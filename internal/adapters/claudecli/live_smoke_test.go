package claudecli_test

// Tier L of the conformance split (conformance_test.go): ONE minimal paid
// end-to-end call against the REAL engine, proving spawn → events →
// checkpoint → engine_sessions on the live substrate. Never part of a
// plain `go test`: it runs only under SINET_LIVE_SMOKE=1 (explicitly
// operator/coordinator-invoked, one call, cheapest model) — the paid
// section of the S03.3 bump gate alongside the S14 quality probes.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/claudecli"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
)

func TestLiveSmoke(t *testing.T) {
	if os.Getenv("SINET_LIVE_SMOKE") != "1" {
		t.Skip("SANCTIONED SKIP (CONVENTIONS §10): tier-L live smoke runs only under SINET_LIVE_SMOKE=1 (one paid call)")
	}
	if _, err := exec.LookPath(claudecli.DefaultBinary); err != nil {
		t.Skipf("SANCTIONED SKIP (CONVENTIONS §10): engine binary %q not installed", claudecli.DefaultBinary)
	}

	e := newE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	e.claimedRun(t, "smoke1")

	a := &claudecli.Adapter{
		Settings: settings.New(),
		// No gated tools in the smoke → no hook wiring needed; HookCmd
		// defaults are irrelevant here.
	}
	req := adapters.StartRequest{
		RunID: "smoke1", UserID: "u1",
		// Cheapest model, one call, one turn of output.
		Model:   "haiku",
		Cwd:     t.TempDir(),
		WorkDir: t.TempDir(),
		Worker: adapters.CompiledWorker{
			Prompt:        "Reply with exactly: ok",
			ToolAllowlist: []string{"Read"}, // default-deny; never Task* (S03.5)
		},
		// Platform ceilings; engine belts = ⚙ 2× these (S03.4).
		CeilingCostUSD: 0.05,
		CeilingSteps:   1,
		// OwnerCredRef empty: dev posture — the engine resolves its own
		// per-user config root (~/.claude); the credential is never read
		// or logged by the platform (D2 hygiene).
	}
	out, err := e.drv.Drive(ctx, a, req)
	if err != nil {
		t.Fatalf("Drive: %v", err)
	}
	t.Logf("live outcome: kind=%s detail=%q result=%s", out.Kind, out.Detail, out.Result)
	if out.Kind != adapters.OutcomeCompleted {
		t.Fatalf("outcome %q, want completed", out.Kind)
	}
	if out.Totals == nil {
		t.Fatal("no run-total usage row from the result envelope")
	}
	t.Logf("live totals: engine_cost_usd=%.7f raw=%s", out.Totals.EngineCostUSD, out.Totals.Raw)

	r, err := e.runs.Get(ctx, "smoke1")
	if err != nil || r.State != run.StateCompleted {
		t.Fatalf("run state %s err=%v", r.State, err)
	}
	cps := gates.NewCheckpoints(e.db, e.log)
	cp, ok, err := cps.Last(ctx, "smoke1")
	if err != nil || !ok {
		t.Fatalf("checkpoint: ok=%v err=%v", ok, err)
	}
	t.Logf("live checkpoint: session_id=%s message_index=%d model=%s usage=%s transcript=%s",
		cp.SessionID, cp.MessageIndex, cp.ModelID, cp.Usage, cp.TranscriptPath)
	if cp.SessionID == "" {
		t.Fatal("checkpoint lost the engine-reported session id (S02.4b)")
	}
	if cp.InvocationFingerprint == "" {
		t.Fatal("checkpoint lost the invocation fingerprint (S02.4e)")
	}

	types := liveEventTypes(t, e)
	t.Logf("live event trail: %v", types)
	need := map[string]bool{"engine.message": false, "usage.recorded": false, "checkpoint.written": false, "engine.done": false}
	for _, typ := range types {
		if _, tracked := need[typ]; tracked {
			need[typ] = true
		}
	}
	for typ, seen := range need {
		if !seen {
			t.Errorf("event trail missing %s", typ)
		}
	}

	// engine_sessions row + transcript copy-aside: behavioral check of
	// the derived CWD-keyed transcript location on the REAL engine.
	var sid, copyPath string
	if err := e.db.QueryRowContext(ctx,
		`SELECT engine_session_id, transcript_copy_path FROM engine_sessions WHERE run_id = 'smoke1'`).
		Scan(&sid, &copyPath); err != nil {
		t.Fatalf("engine_sessions: %v", err)
	}
	t.Logf("live engine_sessions: session=%s copy=%s", sid, copyPath)
	if sid != cp.SessionID {
		t.Errorf("engine_sessions id %q ≠ checkpoint %q", sid, cp.SessionID)
	}
	if copyPath != "" {
		if st, err := os.Stat(copyPath); err != nil || st.Size() == 0 {
			t.Errorf("copy-aside missing/empty: %v", err)
		} else {
			t.Logf("live copy-aside: %d bytes at %s", st.Size(), filepath.Base(copyPath))
		}
	} else {
		t.Logf("NOTE: transcript path not derivable on this engine version — copy-aside skipped (P-T01-1: platform rows remain authoritative)")
	}
}

func liveEventTypes(t *testing.T, e *e2eEnv) []string {
	t.Helper()
	rows, err := e.db.QueryContext(context.Background(),
		`SELECT type FROM run_events WHERE run_id = 'smoke1' ORDER BY event_seq`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	var types []string
	for rows.Next() {
		var typ string
		if err := rows.Scan(&typ); err != nil {
			t.Fatalf("scan: %v", err)
		}
		types = append(types, typ)
	}
	return types
}
