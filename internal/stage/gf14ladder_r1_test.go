package stage_test

// gf14ladder_r1_test.go — P3-GF14 drain r1 F2: the R1 arm the landing left
// unpinned, and the evaluator was right that it is stageable.
//
// `ladderRetry` commits the granted successor and THEN admits it, and admission
// is the rest of the same act: a caller that dies in that window would leave a
// run in the runs table with no queue row — bought by an answer that already
// committed, and driven by nobody. The `Admitter` is a bindable interface
// (`Skeleton.Bind`), so a wrapper can die exactly there: it cancels the
// caller's request and then honors the context IT was handed. Detached, that
// context is live and the admission lands; attached, it is dead and the
// admission refuses — which is the strand.
//
// $0: no engine, no network.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
)

// cancelOnEnqueue is the browser abort landed in the admission window: it kills
// the request and then does exactly what its own context permits.
type cancelOnEnqueue struct {
	stage.Admitter
	cancel context.CancelFunc
	calls  int
	sawCtx error // what the context it was handed said, after the caller died
}

func (a *cancelOnEnqueue) Enqueue(ctx context.Context, runID string, class scheduler.WorkloadClass) error {
	a.calls++
	a.cancel()
	if err := ctx.Err(); err != nil {
		a.sawCtx = err
		return err
	}
	return a.Admitter.Enqueue(ctx, runID, class)
}

func TestGF14LadderRetryAdmissionOutlivesItsCaller(t *testing.T) {
	const owner = "u-operator"
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	askID, runID := seedTombstonedLineage(t, h, "t-gf14-ladder", owner)
	adm := &cancelOnEnqueue{Admitter: h.sched, cancel: cancel}
	h.sk.Bind(adm)

	// The RESPONSE may die with the caller — the task view behind it is read on
	// the caller's own context, and there is nobody left to send it to. What
	// must not die is the granted attempt.
	_, _ = h.sur.Answer(ctx, owner, askID, json.RawMessage(`{"choice":"retry"}`), false)
	if adm.calls != 1 {
		t.Fatalf("the admission ran %d times, want exactly 1 — this test proves nothing otherwise", adm.calls)
	}
	if ctx.Err() == nil {
		t.Fatal("staging defect: the caller's request was never cancelled")
	}
	if adm.sawCtx != nil {
		t.Fatalf("the admission was handed the caller's dying context (%v) — past the fork commit the "+
			"work belongs to the run, not to the request that asked for it", adm.sawCtx)
	}

	successor := runID + ".g1"
	s, err := h.runs.Get(context.Background(), successor)
	if err != nil {
		t.Fatalf("no successor %s: %v", successor, err)
	}
	if s.State != run.StateQueued {
		t.Fatalf("successor state = %s, want queued", s.State)
	}
	// The strand this closes, stated as the row that would be missing: a run in
	// the table with no queue row is a run nobody claims.
	var queued int
	if err := h.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM queue WHERE run_id = ?`, successor).Scan(&queued); err != nil {
		t.Fatalf("count queue rows: %v", err)
	}
	if queued != 1 {
		t.Fatalf("%d queue rows for the granted attempt, want 1 — the answer bought a run nobody drives", queued)
	}
	// And it really is claimable: the loop picks it up on the next pass.
	h.sk.Bind(h.sched)
	if n := h.tick(context.Background()); n != 1 {
		t.Fatalf("tick dispatched %d, want the granted attempt", n)
	}
}
