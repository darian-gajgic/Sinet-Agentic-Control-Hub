package shell

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/buildinfo"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
)

// Settings is the shell-facing view of the settings registry (Spec S01.10):
// effective ⚙ values by dotted key. *settings.Registry satisfies it.
type Settings interface {
	Duration(key string) (time.Duration, error)
}

// Dotted ⚙ keys the shell consumes.
const (
	// keyDrainGrace is the maintenance-mode drain grace (Spec S01.6, ⚙
	// shell.drain_grace, G1 Def.7).
	keyDrainGrace = "shell.drain_grace"
	// keyWALTruncateInterval schedules the periodic wal_checkpoint
	// (TRUNCATE); the interval is owned by Spec S02 (⚙
	// state.wal_truncate_interval), the scheduling duty is the shell's
	// (Spec S02.1 read hygiene).
	keyWALTruncateInterval = "state.wal_truncate_interval"
)

// RecoveryLadder is the S01.6 step-3 seam: one level-triggered reconcile
// pass over runs, effects, and leases — the same code at platform start, at
// wake, and on the periodic sweep (Spec S02.5). The ladder itself is
// P3-B0-4's; the shell only owns when it runs.
type RecoveryLadder interface {
	Reconcile(ctx context.Context) error
}

// stubLadder is the B0-3 stand-in: the startup sequence invokes the seam,
// and nothing exists to reconcile until the run FSM lands (Spec S02, B0-4).
type stubLadder struct {
	logger *slog.Logger
}

func (s stubLadder) Reconcile(ctx context.Context) error {
	s.logger.InfoContext(ctx, "recovery ladder: stub — no run machinery until B0-4 (Spec S02.5)")
	return nil
}

// Admission is the scheduler admission seam the lifecycle drives (Spec
// S01.6): resumed at startup step 5 and on maintenance exit, stopped at
// shutdown and on maintenance enter, with parked-at-grace-expiry as the
// drain terminal. scheduler.StubAdmission implements it until B1 (Spec S10).
// Implementations must not call back into the shell.
type Admission interface {
	ResumeAdmission(ctx context.Context) error
	StopAdmission(ctx context.Context) error
	ParkInFlightRuns(ctx context.Context) error
}

// PlatformUserID attributes platform-scope lifecycle events. Every event row
// carries an owning user_id (feature 15.6); acts of the platform itself are
// owned by this fixed identity. The authenticated person identities arrive
// with the auth stack (Spec S01.9, B0-5) and never collide with it.
const PlatformUserID = "platform"

// Platform lifecycle event types. Minted provisionally at B0-3: Spec S02.5
// establishes platform-lifecycle appends (the pre-sleep flush appends a
// `suspend` event) and the S01.6 shutdown flush is "identical to the
// pre-sleep path"; the start/stop analogs are the same class. The S14 event
// contract (B5) owns final event-type naming.
const (
	EventPlatformStarted  = "platform.started"
	EventPlatformStopping = "platform.stopping"
)

// lifecycleEventSchemaVersion versions the lifecycle payloads.
const lifecycleEventSchemaVersion = 1

// appendLifecycle appends one platform-scope lifecycle event (run_id NULL,
// Spec S02.2).
func appendLifecycle(ctx context.Context, log *eventlog.Log, eventType string) (int64, error) {
	payload, err := json.Marshal(map[string]string{"version": buildinfo.Version()})
	if err != nil {
		return 0, fmt.Errorf("shell: marshal %s payload: %w", eventType, err)
	}
	seq, err := log.Append(ctx, eventlog.Append{
		UserID:        PlatformUserID,
		Type:          eventType,
		SchemaVersion: lifecycleEventSchemaVersion,
		Payload:       payload,
	})
	if err != nil {
		return 0, fmt.Errorf("shell: append %s: %w", eventType, err)
	}
	return seq, nil
}
