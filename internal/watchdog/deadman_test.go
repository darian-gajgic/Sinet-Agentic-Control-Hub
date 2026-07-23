package watchdog

import (
	"context"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
)

// deadman_test.go — the dead-man canary V2 end-to-end (brief R28-DMC, rubric
// 21): a synthetic anomaly is detected + annotated + flagged, a $0 local
// liveness call answers, and a failure raises the alarm.

func TestDeadManPassNoAlarm(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t).withDuty()
	w := e.wd()
	e.fake.SetResponse(local.FakeResponse{
		Content:     `{"reason":"synthetic loop","verdict":"loop","confidence":0.9,"evidence":"repeat"}`,
		InputTokens: 20, OutputTokens: 5,
	})
	if err := w.DeadManProbe(ctx); err != nil {
		t.Fatalf("DeadManProbe: %v", err)
	}
	// No alarm (no platform dead-man flag).
	for _, f := range e.flagsFor("") {
		if f.Rule == RuleDeadMan {
			t.Fatalf("dead-man alarm raised on a healthy probe: %+v", f)
		}
	}
	// Detection alive: exactly one synthetic canary run exists and it FLAGGED a
	// loop (the end-to-end Tier-0→flag path fired) and is terminated (not
	// lingering).
	var canaryID string
	if err := e.db.QueryRowContext(ctx,
		`SELECT run_id FROM runs WHERE run_id LIKE 'platform.deadman.%' LIMIT 1`).Scan(&canaryID); err != nil {
		t.Fatalf("no canary run created: %v", err)
	}
	if fs := e.flagsFor(canaryID); len(fs) != 1 || fs[0].Rule != RuleLoop {
		t.Fatalf("the canary did not detect its synthetic loop end-to-end: %+v", fs)
	}
	if !run.IsTerminal(e.state(canaryID)) {
		t.Errorf("canary run left lingering in state %s (must be terminated)", e.state(canaryID))
	}
	// A quiet pass produces no new event TYPE beyond the ones already declared.
	if got := e.eventsOfType("canary.result"); len(got) != 0 {
		t.Errorf("dead-man produced canary.result — that stays B5-6's (OQ5)")
	}
}

func TestDeadManAlarmWhenLocalDark(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t).withDuty()
	w := e.wd()
	// The local floor is dark: the fake returns 500 for every call, so the $0
	// liveness call errors.
	e.fake.SetResponse(local.FakeResponse{Status: 500})
	if err := w.DeadManProbe(ctx); err != nil {
		t.Fatalf("DeadManProbe: %v", err)
	}
	var found bool
	for _, f := range e.flagsFor("") {
		if f.Rule == RuleDeadMan {
			found = true
			if f.Severity != SeverityFlagNow {
				t.Errorf("dead-man alarm severity = %q, want flag-now (OQ5)", f.Severity)
			}
			if !strings.Contains(f.Detail, "local") {
				t.Errorf("dead-man alarm detail should name the local-floor failure: %q", f.Detail)
			}
		}
	}
	if !found {
		t.Fatalf("no dead-man alarm raised when the local floor was dark")
	}
}

func TestDeadManNoLocalTierStillPasses(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t) // no duty — no local tier configured
	w := e.wd()
	if err := w.DeadManProbe(ctx); err != nil {
		t.Fatalf("DeadManProbe: %v", err)
	}
	// Detection (model-free) works; the local probe is an honest skip (alive),
	// so no alarm.
	for _, f := range e.flagsFor("") {
		if f.Rule == RuleDeadMan {
			t.Fatalf("dead-man alarm raised with no local tier (the local probe is an honest skip): %+v", f)
		}
	}
}
