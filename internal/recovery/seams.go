package recovery

import (
	"context"
	"encoding/json"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// The ladder's step-1 observation inputs (Spec S02.5: "re-list actual
// state, never trust supervisor memory") are interfaces at B0: real systemd
// probing (live units + --remain-after-exit corpses via
// ExecMainStatus/Result/CPUUsageNSec/MemoryPeak; RemainAfterExit=yes +
// ExitType=cgroup + Type=exec, never --collect) and engine-session-store
// mining arrive with real run units and adapters at B1 (Spec S02.5 step 1,
// S03; SPIKE P2-S2).

// UnitState is the observed liveness of a run's unit.
type UnitState int

const (
	// UnitUnknown: the prober cannot say (treated conservatively: dead only
	// if the lease is also dead — see the DEAD rule in Reconcile).
	UnitUnknown UnitState = iota
	// UnitActive: the unit is active and its process tree is live.
	UnitActive
	// UnitCorpseExit0: a --remain-after-exit corpse whose main process
	// exited 0 — the finished-during-outage trigger (Spec S02.5 step 2).
	UnitCorpseExit0
	// UnitCorpseFailed: a corpse that exited non-zero / Result=failed.
	UnitCorpseFailed
	// UnitGone: no unit exists for the run (never started, or reaped —
	// corpses do not survive reboot, P-T07-2).
	UnitGone
)

// UnitStatus is one probe observation.
type UnitStatus struct {
	State      UnitState
	ExitStatus int    // main-process exit status where known
	Result     string // supervisor result string where known
}

// UnitLiveness probes the actual state of a run's unit (Spec S02.5 step 1).
type UnitLiveness interface {
	Probe(ctx context.Context, r run.Run) (UnitStatus, error)
}

// NoUnits is the B0 unit-liveness default: no run units exist yet (they
// land at B1, Spec S03/S11.8), so nothing can be probed and every run is
// UnitGone — exactly the crash-equivalent posture the ladder needs when the
// control plane restarts with no supervisor to ask.
type NoUnits struct{}

// Probe reports UnitGone for every run.
func (NoUnits) Probe(ctx context.Context, r run.Run) (UnitStatus, error) {
	return UnitStatus{State: UnitGone}, nil
}

// HarvestRecord is one engine-store record newer than the run's last
// checkpoint — the raw material of finished-during-outage harvest,
// deduplicated by (session_id, message_id) at append (Spec S02.5 step 2).
type HarvestRecord struct {
	SessionID string
	MessageID string
	// Terminal marks a record showing a terminal result for the run — the
	// engine-store half of the FINISHED-DURING-OUTAGE trigger.
	Terminal bool
	Payload  json.RawMessage
	Usage    json.RawMessage
}

// HarvestSource mines engine session state for results produced during an
// outage (Spec S02.5 steps 1–2). sinceEventSeq is the run's last checkpoint
// event (0 when none): only strictly newer records are harvest material.
type HarvestSource interface {
	NewerThan(ctx context.Context, r run.Run, sinceEventSeq int64) ([]HarvestRecord, error)
}

// NoHarvest is the B0 harvest default: engine session stores arrive with
// the adapters (B1, Spec S03), so there is nothing to mine.
type NoHarvest struct{}

// NewerThan reports no records.
func (NoHarvest) NewerThan(ctx context.Context, r run.Run, sinceEventSeq int64) ([]HarvestRecord, error) {
	return nil, nil
}
