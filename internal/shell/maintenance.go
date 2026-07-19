package shell

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Lifecycle modes surfaced on the health endpoint (surfaces stay readable
// throughout maintenance, Spec S01.6).
const (
	ModeRunning     = "running"
	ModeDraining    = "draining"    // maintenance entered, drain grace running
	ModeMaintenance = "maintenance" // grace expired, in-flight runs parked
)

// Maintenance is the one operator switch of Spec S01.6 (feature 4.5):
//
//   - Enter: admission stops — the scheduler claims nothing new; surfaces
//     stay readable.
//   - Drain: in-flight runs continue for ⚙ shell.drain_grace; runs that
//     finish, finish normally.
//   - Grace expiry: still-running runs are parked — never a kill of record.
//   - Exit: admission resumes.
//
// B0-3 ships the switch machinery; the operator surface that flips it
// arrives with the auth stack (B0-5) and the settings/ops endpoints, and the
// runs it will drain arrive at B0-4/B1. Planned restarts and deploys SHOULD
// pass through it (Spec S01.6, S01.11).
type Maintenance struct {
	settings  Settings
	admission Admission
	logger    *slog.Logger

	mu    sync.Mutex
	mode  string
	timer *time.Timer
}

// NewMaintenance builds the switch in ModeRunning.
func NewMaintenance(settings Settings, admission Admission, logger *slog.Logger) *Maintenance {
	return &Maintenance{settings: settings, admission: admission, logger: logger, mode: ModeRunning}
}

// Mode returns the current lifecycle mode.
func (m *Maintenance) Mode() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.mode
}

// Enter flips the switch on: stop admission, then drain for ⚙
// shell.drain_grace before parking what still runs.
func (m *Maintenance) Enter(ctx context.Context) error {
	m.mu.Lock()
	if m.mode != ModeRunning {
		mode := m.mode
		m.mu.Unlock()
		return fmt.Errorf("shell: maintenance already entered (mode %s)", mode)
	}
	grace, err := m.settings.Duration(keyDrainGrace)
	if err != nil {
		m.mu.Unlock()
		return fmt.Errorf("shell: read ⚙ %s: %w", keyDrainGrace, err)
	}
	m.mode = ModeDraining
	m.mu.Unlock()

	if err := m.admission.StopAdmission(ctx); err != nil {
		m.mu.Lock()
		m.mode = ModeRunning
		m.mu.Unlock()
		return fmt.Errorf("shell: maintenance enter: stop admission: %w", err)
	}

	m.mu.Lock()
	if m.mode == ModeDraining { // not exited concurrently
		m.timer = time.AfterFunc(grace, m.expire)
	}
	m.mu.Unlock()
	m.logger.InfoContext(ctx, "maintenance: entered, draining", "grace", grace)
	return nil
}

// expire is the drain-grace terminal: park still-running runs (Spec S01.6
// grace expiry; parked, flagged, resumable — the park policy is Spec S10's).
func (m *Maintenance) expire() {
	m.mu.Lock()
	if m.mode != ModeDraining {
		m.mu.Unlock()
		return
	}
	m.mode = ModeMaintenance
	m.timer = nil
	m.mu.Unlock()

	ctx := context.Background()
	if err := m.admission.ParkInFlightRuns(ctx); err != nil {
		m.logger.Error("maintenance: park in-flight runs", "err", err)
	}
	m.logger.Info("maintenance: drain grace expired, in-flight runs parked")
}

// Exit flips the switch off: admission resumes; parked runs resume per
// scheduler priority (Spec S01.6 — the resume policy is Spec S10's).
func (m *Maintenance) Exit(ctx context.Context) error {
	m.mu.Lock()
	if m.mode == ModeRunning {
		m.mu.Unlock()
		return fmt.Errorf("shell: not in maintenance")
	}
	t := m.timer
	m.timer = nil
	m.mode = ModeRunning
	m.mu.Unlock()

	if t != nil {
		t.Stop()
	}
	if err := m.admission.ResumeAdmission(ctx); err != nil {
		return fmt.Errorf("shell: maintenance exit: resume admission: %w", err)
	}
	m.logger.InfoContext(ctx, "maintenance: exited, admission resumed")
	return nil
}
