package stage_test

// zombiedispatch_test.go — P3-RW-6 §7 packet B: an unroutable dispatch must
// leave a CORPSE, never a claimed zombie (T-B1, checklist 10), plus the
// exported routability predicate's fork property (R15).
//
// `Skeleton.Dispatch` refused a run id carrying no skeleton role BEFORE any
// transition, so the run stayed `claimed` — holding its cap-1 (user, lane) slot
// — until a recovery sweep noticed it up to ⚙ recovery.dead_after later. The
// ratified edge for exactly this is claimed→crashed (run.go: "implied by S02.5
// step 2"), and the crash-for-the-ladder precedent is already in this file's
// package: a dispatch leg that cannot proceed leaves a classifiable corpse.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
)

func TestUnroutableDispatchLeavesCorpse(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	const owner = "u-zombie"

	// Two roleless runs on ONE (user, lane) slot bucket: the second can only be
	// claimed if the first released its slot, which is the whole point.
	for _, id := range []string{"r-zombie", "r-zombie-2"} {
		if _, err := h.runs.Create(ctx, run.NewRun{
			ID: id, UserID: owner, Lane: "anthropic", Substrate: "claude-cli",
		}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
		if err := h.sched.Enqueue(ctx, id, scheduler.ClassInteractive); err != nil {
			t.Fatalf("enqueue %s: %v", id, err)
		}
	}

	if n := h.tick(ctx); n != 1 {
		t.Fatalf("first tick dispatched %d, want 1 (the lane cap is 1)", n)
	}

	// The run committed CRASHED together with its state-change event, and the
	// event names the cause — read in ONE query so a state without its event (or
	// the reverse) cannot pass.
	var state, payload string
	if err := h.db.QueryRowContext(ctx, `
		SELECT r.state, e.payload FROM runs r JOIN run_events e ON e.run_id = r.run_id
		 WHERE r.run_id = ? AND e.type = ? AND json_extract(e.payload, '$.to') = 'crashed'`,
		"r-zombie", run.EventState).Scan(&state, &payload); err != nil {
		t.Fatalf("no committed crash for the unroutable run (it is a claimed zombie): %v", err)
	}
	if state != string(run.StateCrashed) {
		t.Fatalf("run state = %q, want crashed", state)
	}
	var ev struct {
		Detail struct {
			Cause string `json:"cause"`
		} `json:"detail"`
	}
	if err := json.Unmarshal([]byte(payload), &ev); err != nil {
		t.Fatalf("decode crash event: %v", err)
	}
	if !strings.Contains(ev.Detail.Cause, "r-zombie") {
		t.Fatalf("crash detail cause = %q, want the roleless run id named", ev.Detail.Cause)
	}

	// The slot freed IMMEDIATELY: the sibling is claimed on the very next pass,
	// with no recovery sweep in between.
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("second tick dispatched %d, want 1 — the corpse never released its (user, lane) slot", n)
	}
	var active int
	if err := h.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM runs WHERE user_id = ? AND state IN ('claimed','running','draining')`,
		owner).Scan(&active); err != nil {
		t.Fatalf("count active: %v", err)
	}
	if active != 0 {
		t.Fatalf("%d runs still occupy a slot, want 0 (both are corpses)", active)
	}
}

// TestRoutablePropertyOverForkChains is the exported half of the §7 property:
// routability is a property of the run's ROLE, so it survives any chain of
// recovery-fork suffixes — `Routable(fork(id)) == Routable(id)` — which is what
// lets the shell compose "no role ⇒ never fork" without the predicate flipping
// between generations of one lineage.
func TestRoutablePropertyOverForkChains(t *testing.T) {
	routable := []string{"t-x.intake", "t-x.execute", "t-x.verify", "t-x.compose",
		"onboard-shop.onboard", "bp-1.direct"}
	roleless := []string{"r-zombie", "platform.deadman.1754870000000000000",
		"t-x", "t-x.gadget", "t-x.executes", "t-g1"}
	chains := []string{"", ".g1", ".g2.g3", ".g12", ".g4.g5.g6"}

	for _, base := range routable {
		for _, chain := range chains {
			if !stage.Routable(base + chain) {
				t.Errorf("Routable(%q) = false, want true (a fork routes as its base does)", base+chain)
			}
		}
	}
	for _, base := range roleless {
		for _, chain := range chains {
			if stage.Routable(base + chain) {
				t.Errorf("Routable(%q) = true, want false (no skeleton role)", base+chain)
			}
		}
	}
}
