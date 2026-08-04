package watchdog

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// tier1_test.go — the local disambiguator (brief R9–R14, rubric 6–9). All $0
// via local.FakeServer — NO paid call is ever implied. A real-model exercise
// (tier R/L) would be a SANCTIONED SKIP when the engine is absent; this suite is
// the always-run tier-F fake.

func TestSchemaCarriesUnclearAbstainMember(t *testing.T) {
	s := string(WatchdogSchema())
	if !strings.Contains(s, `"unclear"`) {
		t.Errorf("watchdog verdict schema missing the mandatory `unclear` abstain member (S12.4): %s", s)
	}
	// reason MUST lead the properties (F5 ordered schema).
	ri, vi := strings.Index(s, `"reason"`), strings.Index(s, `"verdict"`)
	if ri < 0 || vi < 0 || ri > vi {
		t.Errorf("schema is not reason-first (F5): reason@%d verdict@%d", ri, vi)
	}
	// local.Duty's abstain belt accepts it (the belt enforces the member for a
	// Classification call).
	e := newEnv(t).withDuty()
	e.runningRun("t.execute", "alice")
	e.fake.SetModelResponse(e.seatModel(), local.FakeResponse{
		Content:     `{"reason":"stuck","verdict":"loop","confidence":0.9,"evidence":"same Bash 5×"}`,
		InputTokens: 40, OutputTokens: 8,
	})
	if _, err := e.duty.Call(context.Background(), "t.execute", local.DutyRequest{
		Alias: local.AliasWatchdog, Schema: WatchdogSchema(), Name: "watchdog-disambiguator",
		System: "s", User: "u", MaxTokens: 64, Classification: true,
	}); err != nil {
		t.Fatalf("abstain belt rejected the watchdog schema: %v", err)
	}
}

func TestTier1AnnotatesAndShapesTheCall(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t).withDuty()
	w := e.wd()
	e.runningRun("t.execute", "alice")
	e.fake.SetModelResponse(e.seatModel(), local.FakeResponse{
		Content:     `{"reason":"repeats the same call","verdict":"loop","confidence":0.88,"evidence":"Bash d1 ×5"}`,
		InputTokens: 60, OutputTokens: 12,
		Logprobs: []local.TokenLogprob{{Token: "loop", Logprob: -0.1}},
	})

	for i := 0; i < 5; i++ {
		e.toolEvent("t.execute", "Bash", "d1", false)
	}
	if err := w.EvaluateRun(ctx, "t.execute"); err != nil {
		t.Fatalf("EvaluateRun: %v", err)
	}

	// watchdog.annotated recorded with the verdict.
	anns := e.eventsOfType(EventAnnotated)
	if len(anns) != 1 {
		t.Fatalf("want 1 watchdog.annotated, got %d", len(anns))
	}
	var ann AnnotationPayload
	mustUnmarshal(t, anns[0], &ann)
	if ann.Verdict != VerdictLoop || ann.Confidence != 0.88 || ann.Evidence == "" {
		t.Errorf("annotation = %+v, want verdict=loop confidence=0.88 evidence set", ann)
	}

	// The flag references the annotation.
	fs := e.flagsFor("t.execute")
	if len(fs) != 1 || fs[0].AnnotationRef == nil {
		t.Fatalf("flag missing annotation_ref: %+v", fs)
	}

	// The call was shaped per S12.4 (greedy, json_schema at the engine, logprobs).
	lr := e.fake.LastRequest()
	if lr.Temperature != 0 || !lr.HasJSONSchema || lr.SchemaName != "watchdog-disambiguator" || !lr.Logprobs {
		t.Errorf("disambiguator call not shaped per S12.4: %+v", lr)
	}

	// EXACTLY one $0 zero-allowance local line on the WATCHED run (OQ4 active).
	led := metering.NewLedger(e.db, nil, metering.NoMeteredExceptions(), e.reg)
	rc, err := led.RunConsumption(ctx, "t.execute")
	if err != nil {
		t.Fatalf("RunConsumption: %v", err)
	}
	local0 := 0
	for _, li := range rc.Items {
		if li.Lane == metering.LaneLocal {
			local0++
			if li.Unpriced() || li.PricedUSD != 0 || !li.ZeroAllowance() {
				t.Errorf("Tier-1 local line not TRUE $0 zero-allowance: %+v", li)
			}
		}
	}
	if local0 != 1 {
		t.Errorf("want exactly 1 local $0 line on the watched run, got %d", local0)
	}
}

// TestTier1AnnotatesNeverGates is the R12 invariant: the Tier-1 verdict NEVER
// changes whether Tier-2 parks. Even a `productive` verdict (the disambiguator
// saying "not a loop") leaves the run PARKED — Tier-2 is ALWAYS on a Tier-0
// trigger.
func TestTier1AnnotatesNeverGates(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t).withDuty()
	w := e.wd()
	e.runningRun("t.execute", "bob")
	e.fake.SetModelResponse(e.seatModel(), local.FakeResponse{
		Content:     `{"reason":"actually making progress","verdict":"productive","confidence":0.95,"evidence":"varied outputs"}`,
		InputTokens: 40, OutputTokens: 8,
	})
	for i := 0; i < 5; i++ {
		e.toolEvent("t.execute", "Bash", "d1", false)
	}
	if err := w.EvaluateRun(ctx, "t.execute"); err != nil {
		t.Fatalf("EvaluateRun: %v", err)
	}
	if e.state("t.execute") != run.StateParked {
		t.Fatalf("a `productive` verdict prevented the park — Tier-1 must NEVER gate Tier-2 (R12)")
	}
	anns := e.eventsOfType(EventAnnotated)
	var ann AnnotationPayload
	mustUnmarshal(t, anns[0], &ann)
	if ann.Verdict != VerdictProductive {
		t.Errorf("verdict = %q, want productive (recorded honestly, still parked)", ann.Verdict)
	}
}

// TestTier1LowMarginIsUnclear: an unparseable / no-verdict model output degrades
// to `unclear` — annotates, never a fabricated label (R12).
func TestTier1LowMarginIsUnclear(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t).withDuty()
	w := e.wd()
	e.runningRun("t.execute", "carol")
	// The FakeServer default response is `{"abstain":true}` — no verdict field.
	for i := 0; i < 5; i++ {
		e.toolEvent("t.execute", "Bash", "d1", false)
	}
	if err := w.EvaluateRun(ctx, "t.execute"); err != nil {
		t.Fatalf("EvaluateRun: %v", err)
	}
	anns := e.eventsOfType(EventAnnotated)
	if len(anns) != 1 {
		t.Fatalf("want 1 annotation, got %d", len(anns))
	}
	var ann AnnotationPayload
	mustUnmarshal(t, anns[0], &ann)
	if ann.Verdict != VerdictUnclear || !ann.Abstained {
		t.Errorf("no-verdict output must degrade to unclear/abstained, got %+v", ann)
	}
	if e.state("t.execute") != run.StateParked {
		t.Fatalf("run must still park on an unclear verdict (Tier-2 always)")
	}
}

// TestTier1TerminalRunUsesAdvisoryMeter (OQ4): a suspicious-completion (terminal
// watched run) is not checkpointable, so the $0 D7 row lands on an AdvisoryMeter
// platform run — never on the terminal run, never a silent unmetered call. The
// watchdog.annotated event still attributes to the WATCHED run.
func TestTier1TerminalRunUsesAdvisoryMeter(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t).withDuty()
	w := e.wd()
	e.fake.SetResponse(local.FakeResponse{
		Content:     `{"reason":"ended with no work","verdict":"unclear","confidence":0.3,"evidence":"no tools"}`,
		InputTokens: 30, OutputTokens: 6,
	})
	e.runningRun("silent.execute", "dave")
	e.completeRun("silent.execute") // terminal, not checkpointable
	if err := w.EvaluateRun(ctx, "silent.execute"); err != nil {
		t.Fatalf("EvaluateRun: %v", err)
	}

	// The annotation is attributed to the watched run.
	var annOwner, annRun string
	if err := e.db.QueryRowContext(ctx,
		`SELECT COALESCE(run_id,''), user_id FROM run_events WHERE type=? LIMIT 1`, EventAnnotated).
		Scan(&annRun, &annOwner); err != nil {
		t.Fatalf("read annotation scope: %v", err)
	}
	if annRun != "silent.execute" || annOwner != "dave" {
		t.Errorf("annotation scope = (%q,%q), want the watched run (silent.execute, dave)", annRun, annOwner)
	}

	// The terminal run has NO local checkpoint; a platform advisory run carries
	// the $0 row instead.
	led := metering.NewLedger(e.db, nil, metering.NoMeteredExceptions(), e.reg)
	rc, _ := led.RunConsumption(ctx, "silent.execute")
	for _, li := range rc.Items {
		if li.Lane == metering.LaneLocal {
			t.Errorf("the terminal watched run must not carry a local checkpoint (not checkpointable)")
		}
	}
	var advisoryRuns int
	if err := e.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM runs WHERE run_id LIKE 'platform.advisory.watchdog-disambiguator.%'`).Scan(&advisoryRuns); err != nil {
		t.Fatalf("count advisory runs: %v", err)
	}
	if advisoryRuns != 1 {
		t.Errorf("want 1 AdvisoryMeter platform run for the terminal Tier-1 call, got %d", advisoryRuns)
	}
}

// TestTier1SkipsWhenTerminalAndNoMeter: duty wired but no AdvisoryMeter and the
// watched run is terminal ⇒ the annotation is skipped HONESTLY (logged), and the
// flag still fires (Tier-2 always) — never a silent unmetered call.
func TestTier1SkipsHonestlyWithoutMeter(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t).withDuty()
	w := e.wd(func(d *Deps) { d.Meter = nil }) // duty set, no advisory meter
	e.runningRun("silent.execute", "erin")
	e.completeRun("silent.execute")
	if err := w.EvaluateRun(ctx, "silent.execute"); err != nil {
		t.Fatalf("EvaluateRun: %v", err)
	}
	if len(e.eventsOfType(EventAnnotated)) != 0 {
		t.Errorf("annotation produced without a checkpointable run/meter")
	}
	if fs := e.flagsFor("silent.execute"); len(fs) != 1 {
		t.Fatalf("flag must still fire when Tier-1 is skipped (Tier-2 always): %+v", fs)
	}
}

func mustUnmarshal(t *testing.T, raw []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}
