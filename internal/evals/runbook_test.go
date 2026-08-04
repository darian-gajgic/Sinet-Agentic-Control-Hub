package evals_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/conformance"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/evals"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// flagCalls records what the runbook asked the S08.10 flag surfaces to do. The
// real surfaces are worker.Store's; internal/evals reaches them through func
// seams, so the test substitutes recorders.
type flagCalls struct {
	byPin   []string
	byModel []string
	stamps  []evals.Stamp
}

func (c *flagCalls) hooks() evals.Hooks {
	return evals.Hooks{
		FlagByEnginePin: func(_ context.Context, pin, _ string) ([]string, error) {
			c.byPin = append(c.byPin, pin)
			return []string{"wt-flagged-by-pin"}, nil
		},
		FlagByModel: func(_ context.Context, model, _ string) ([]string, error) {
			c.byModel = append(c.byModel, model)
			return []string{"wt-flagged-by-model"}, nil
		},
		Stamp: func(_ context.Context, st evals.Stamp) error {
			c.stamps = append(c.stamps, st)
			return nil
		},
	}
}

// ── the three trigger classes (rubric 11; Spec S14.8 ¶3) ──

func TestRunbookTriggersDriveExistingFlagSurfaces(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	calls := &flagCalls{}
	rb := &evals.Runbook{Store: f.store, Catalog: &evals.Catalog{}, Hooks: calls.hooks()}

	// (ii) an engine bump flags through S08.10(b)'s engine-pin surface.
	got, err := rb.Flag(ctx, evals.Trigger{Kind: evals.TriggerEngineBump, Subject: "2.1.219"})
	if err != nil {
		t.Fatal(err)
	}
	if len(calls.byPin) != 1 || calls.byPin[0] != "2.1.219" || len(got) != 1 {
		t.Fatalf("engine bump did not drive FlagByEnginePin: %v / %v", calls.byPin, got)
	}

	// (iii) a swap CONSUMES the set the S12.10 swap gate already produced before
	// the alias went live — it never re-flags, because that would be a second
	// gate (P-T15-1).
	got, err = rb.Flag(ctx, evals.Trigger{
		Kind: evals.TriggerSeatSwap, Subject: "gemma-3-9b", Flagged: []string{"wt-a", "wt-b"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "wt-a" {
		t.Fatalf("swap flag set = %v, want the swap gate's own output", got)
	}
	if len(calls.byModel) != 0 {
		t.Fatalf("a swap must not re-flag: %v", calls.byModel)
	}

	// (i) a watchlist drift finding naming a model flags by model. The runbook
	// exposes the trigger verb only — no emitter is faked here (B5-6 owns it).
	if _, err = rb.Flag(ctx, evals.Trigger{Kind: evals.TriggerDrift, Subject: "claude-opus-4-8"}); err != nil {
		t.Fatal(err)
	}
	if len(calls.byModel) != 1 || calls.byModel[0] != "claude-opus-4-8" {
		t.Fatalf("drift did not drive FlagByModel: %v", calls.byModel)
	}

	// Exactly three classes: a fourth is refused, never silently absorbed.
	if _, err = rb.Flag(ctx, evals.Trigger{Kind: "price_change", Subject: "x"}); !errors.Is(err, evals.ErrUnknownTrigger) {
		t.Fatalf("unknown trigger: %v, want ErrUnknownTrigger", err)
	}
	// An unwired surface fails loudly rather than silently flagging nothing.
	bare := &evals.Runbook{Store: f.store, Catalog: &evals.Catalog{}}
	if _, err = bare.Flag(ctx, evals.Trigger{Kind: evals.TriggerEngineBump, Subject: "2.1.219"}); !errors.Is(err, evals.ErrNoFlagHook) {
		t.Fatalf("unwired flag surface: %v, want ErrNoFlagHook", err)
	}
}

// ── the pipeline + the dated stamp (rubric 12, 13) ──

func TestRunbookStampsGreenAndKeepsRedFlagged(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.registerFloors(t)
	calls := &flagCalls{}
	cat := &evals.Catalog{}
	rb := &evals.Runbook{Store: f.store, Catalog: cat, Hooks: calls.hooks()}

	rubric := verify.SeedSoftwareRubric()
	obj, err := cat.Rubric(rubric)
	if err != nil {
		t.Fatal(err)
	}
	// The pinned baseline is the rubric bundle's OWN recorded rates, so the
	// runbook and the verdict records can never disagree (Spec S07.11).
	base, err := evals.BaselineFromRubric(rubric)
	if err != nil {
		t.Fatal(err)
	}
	if base.TPR != 1.0 || base.TNR != 0.5 || base.MeasuredOn != "2026-07-22" {
		t.Fatalf("baseline = %+v, want the rider-1 measurement", base)
	}

	rv := evals.Revalidation{
		Trigger:  evals.Trigger{Kind: evals.TriggerEngineBump, Subject: "2.1.219"},
		Object:   obj,
		Baseline: base,
		Measured: evals.Measured{
			PlantedCatch: 1.0, CleanPass: 0.5,
			Runner: evals.RunnerRider1, RunnerVersion: "golden-software@v1",
			RanAt: time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC),
		},
		TemplateID: "wt-1", VersionID: "wv-1", Actor: "alice",
	}
	out, err := rb.Revalidate(ctx, rv)
	if err != nil {
		t.Fatalf("Revalidate: %v", err)
	}
	if !out.Green || !out.Stamped {
		t.Fatalf("no regression must release with a dated stamp: %+v", out)
	}
	if len(calls.stamps) != 1 {
		t.Fatalf("stamps = %d, want 1", len(calls.stamps))
	}
	st := calls.stamps[0]
	if !st.Green || st.VersionID != "wv-1" || st.Actor != "alice" {
		t.Fatalf("stamp = %+v", st)
	}
	// Who / when / which suites / vs which baseline: the suites and the baseline
	// ride the stamp (the date is stamped by the lifecycle verb).
	if !strings.Contains(st.Suites, "golden-software") {
		t.Errorf("stamp does not name the suites that ran: %q", st.Suites)
	}
	if !strings.Contains(st.Baseline, "2026-07-22") {
		t.Errorf("stamp does not name the pinned baseline: %q", st.Baseline)
	}
	if f.sweepRow(t).LastResult != conformance.ResultGreen {
		t.Error("the green revalidation did not record green")
	}

	// The SAME pass with a below-floor catch rate is red: the version stays
	// flagged (the stamp records the red) and the row goes red, which is the
	// owner card's derivation input.
	rv.Measured.PlantedCatch = 0.70
	out, err = rb.Revalidate(ctx, rv)
	if err != nil {
		t.Fatalf("Revalidate (red): %v", err)
	}
	if out.Green {
		t.Fatal("a below-floor catch rate must be red")
	}
	if out.FloorCheck.Green {
		t.Fatal("the floor check must be the thing that reds it")
	}
	if len(calls.stamps) != 2 || calls.stamps[1].Green {
		t.Fatalf("the red pass must stamp NOT-green (the version stays flagged): %+v", calls.stamps)
	}
	if f.sweepRow(t).LastResult != conformance.ResultRed {
		t.Error("the red revalidation did not record red")
	}

	// A rubric asset has no worker-lifecycle row: it records and never stamps.
	rv.VersionID, rv.TemplateID = "", ""
	rv.Measured.PlantedCatch = 1.0
	out, err = rb.Revalidate(ctx, rv)
	if err != nil {
		t.Fatal(err)
	}
	if out.Stamped || len(calls.stamps) != 2 {
		t.Fatalf("an asset with no lifecycle row must not stamp: %+v", out)
	}

	// An unmeasured baseline is refused rather than compared against zero.
	rv.Baseline = evals.Baseline{}
	if _, err = rb.Revalidate(ctx, rv); !errors.Is(err, evals.ErrUnmeasuredBaseline) {
		t.Fatalf("unmeasured baseline: %v, want ErrUnmeasuredBaseline", err)
	}
}

func TestJudgeRemeasurementRecordsTheRiderInstrument(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	cat := &evals.Catalog{}
	obj, err := cat.Rubric(verify.SeedSoftwareRubric())
	if err != nil {
		t.Fatal(err)
	}
	// The P-T06-5 re-measurement path the runbook owns: the committed rider-1
	// harness is the instrument, and its result records honestly under that
	// identity — never under the promptfoo runner's.
	res := evals.JudgeRemeasurement(obj, "claude-opus-4-8",
		evals.Measured{PlantedCatch: 1.0, CleanPass: 0.5}, true)
	if res.Runner != evals.RunnerRider1 {
		t.Fatalf("runner = %q, want the rider-1 harness", res.Runner)
	}
	if err := f.store.RecordAsset(ctx, res); err != nil {
		t.Fatal(err)
	}
	var payload string
	if err := f.db.QueryRowContext(ctx,
		`SELECT payload FROM run_events WHERE type = ? ORDER BY event_seq DESC LIMIT 1`,
		conformance.EventScoreRecorded).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, "claude-opus-4-8") || !strings.Contains(payload, "P-T06-5") {
		t.Errorf("the re-measurement record does not name its seat and probe: %s", payload)
	}
}
