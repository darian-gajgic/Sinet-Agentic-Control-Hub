package watchlist_test

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/watchlist"
)

// canary_logprob_test.go — the MEASUREMENT path of the S14.6 ¶3 logprob canary
// (drain D2). Tier F: a local.FakeServer + a real local.Duty, so the whole
// production path runs — duty call, schema round-trip, local.LabelMargin
// extraction, delta vs the log, card + subject — with ZERO network and ZERO
// paid calls. The local tier is $0 by construction.

// logprobEnv wires a fake local stack onto the harness.
type logprobEnv struct {
	*harness
	fake *local.FakeServer
	duty *local.Duty
	clk  *clock
	c    *watchlist.Canaries
}

func newLogprobEnv(t *testing.T) *logprobEnv {
	t.Helper()
	h := newHarness(t)
	fake := local.NewFakeServer()
	t.Cleanup(fake.Close)

	duty := local.NewDuty(local.DutyDeps{
		Registry:    local.NewRegistry(h.reg),
		Client:      local.NewClient(fake.URL),
		Checkpoints: gates.NewCheckpoints(h.db, h.log),
		Events:      h.log,
	})
	clk := &clock{t: mustTime(t, "2026-07-27T09:00:00Z")}
	e := &logprobEnv{harness: h, fake: fake, duty: duty, clk: clk}
	e.c = h.canaries(t, clk)
	e.c.Logprob = watchlist.NewLogprobCanary(duty, e.advisoryMeter())
	return e
}

// advisoryMeter opens a short-lived platform run so the $0 D7 row lands (the
// §28/§31 AdvisoryMeter seam) — the canary is never issued unmetered.
func (e *logprobEnv) advisoryMeter() watchlist.AdvisoryMeter {
	runs := run.NewStore(e.db, e.log)
	return func(ctx context.Context, purpose string) (string, func(), error) {
		id := "platform.advisory." + purpose + "." + strconv.FormatInt(time.Now().UnixNano(), 10)
		if _, err := runs.Create(ctx, run.NewRun{
			ID: id, UserID: run.ActorPlatform, Lane: local.LaneLocal, Substrate: "local",
		}); err != nil {
			return "", nil, err
		}
		for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
			if _, err := runs.Transition(ctx, id, st, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
				return "", nil, err
			}
		}
		return id, func() {
			_, _ = runs.Transition(context.WithoutCancel(ctx), id, run.StateCompleted,
				run.TransitionOptions{Actor: run.ActorPlatform})
		}, nil
	}
}

// seatModel is the model the watchlist-triage alias resolves to — the id the
// canary must carry as its revalidation subject.
func (e *logprobEnv) seatModel(t *testing.T) string {
	t.Helper()
	seat, err := e.duty.ResolveSeat(local.AliasWatchlist)
	if err != nil {
		t.Fatalf("ResolveSeat(%s): %v", local.AliasWatchlist, err)
	}
	return seat.Model
}

// setMargin makes the fake seat emit the canary's schema with a change_class
// label token whose top-2 gap is exactly margin.
func (e *logprobEnv) setMargin(t *testing.T, model string, margin float64) {
	t.Helper()
	e.fake.SetModelResponse(model, local.FakeResponse{
		Content: `{"reason":"the input price moved","change_class":"price","lanes":["local"],"summary":"price moved"}`,
		// The reconstructed text must contain the emitted JSON so
		// local.LabelMargin can locate `"change_class":` and map the first
		// value character back to its token. The decisive token is the one
		// carrying the first character of the enum value.
		Logprobs:     marginTokens(`{"reason":"the input price moved","change_class":"`, "price", margin),
		InputTokens:  40,
		OutputTokens: 12,
	})
}

// marginTokens builds a token stream whose reconstruction is prefix+label+tail
// and whose LABEL token carries a top1−top2 gap of exactly margin. Every other
// token carries a wide, uninteresting gap, so the minimum across label fields
// is unambiguously the label's.
//
// The alternatives type is unexported, so the tokens are built through
// local.TokenLogprobFixture — the constructor internal/local already ships for
// exactly this (no second fixture builder is invented here).
func marginTokens(prefix, label string, margin float64) []local.TokenLogprob {
	return []local.TokenLogprob{
		local.TokenLogprobFixture(prefix, 9.0),
		// The label token: its top1−top2 gap IS the margin under test, and it
		// is the narrowest in the stream, so the minimum across label fields is
		// unambiguously this one.
		local.TokenLogprobFixture(label, margin),
		local.TokenLogprobFixture(`","lanes":["local"],"summary":"price moved"}`, 9.0),
	}
}

// TestMarginTokensIsNotTautological proves the fixture itself: the token stream
// really does yield the requested margin through the PRODUCTION extractor, so a
// test asserting a margin is asserting the real computation.
func TestMarginTokensIsNotTautological(t *testing.T) {
	for _, want := range []float64{0.5, 2.0, 7.25} {
		got, ok := local.LabelMargin(marginTokens(`{"reason":"r","change_class":"`, "price", want),
			[]string{"change_class"})
		if !ok {
			t.Fatalf("the fixture yields no margin at all for %v", want)
		}
		if diff := got - want; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("LabelMargin = %v, want %v — the fixture does not drive the real extractor", got, want)
		}
	}
}

// TestLogprobCanaryBaselineThenDrift is the D2 headline: the real measurement
// path establishes a baseline quietly, then a margin move past the tolerance
// raises a models-class card carrying the SEAT MODEL as its revalidation
// subject — the local lane's advantage over a subscription lane.
func TestLogprobCanaryBaselineThenDrift(t *testing.T) {
	ctx := context.Background()
	e := newLogprobEnv(t)
	model := e.seatModel(t)

	var flagged []string
	e.em.Revalidate = func(ctx context.Context, modelID, reason string) ([]string, error) {
		flagged = append(flagged, modelID)
		return []string{"tmpl-local"}, nil
	}

	// Run 1: margin 6.0 — the baseline.
	e.setMargin(t, model, 6.0)
	if _, err := e.c.RunDue(ctx); err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	got := e.canaryEvents(t)
	if len(got) != 1 {
		t.Fatalf("canary.result rows = %d, want 1", len(got))
	}
	if !got[0].Baseline || got[0].Result != watchlist.CanaryPass {
		t.Fatalf("first run should be a passing baseline, got %+v", got[0])
	}
	if got[0].Measurement == nil || *got[0].Measurement < 5.99 || *got[0].Measurement > 6.01 {
		t.Fatalf("baseline measurement = %v, want ≈6.0 from the real extractor", got[0].Measurement)
	}
	if len(e.findings(t)) != 0 {
		t.Fatal("the baseline raised a card")
	}
	if len(flagged) != 0 {
		t.Fatalf("the baseline flagged %v", flagged)
	}

	// Run 2, a day later: margin collapses to 1.0 — a 5.0 move, well past the
	// 1.0 tolerance.
	e.setMargin(t, model, 1.0)
	e.clk.advance(25 * time.Hour)
	if _, err := e.c.RunDue(ctx); err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	got = e.canaryEvents(t)
	if len(got) != 2 {
		t.Fatalf("canary.result rows = %d, want 2", len(got))
	}
	if got[1].Result != watchlist.CanaryFail {
		t.Fatalf("a 5.0-nat margin collapse did not fail the canary: %+v", got[1])
	}
	if got[1].Delta > -4.9 {
		t.Errorf("delta = %v, want ≈-5.0", got[1].Delta)
	}

	cards := e.findings(t)
	if len(cards) != 1 {
		t.Fatalf("drift.finding rows = %d, want 1", len(cards))
	}
	if cards[0].ChangeClass != watchlist.ClassModels {
		t.Errorf("change class = %q, want models", cards[0].ChangeClass)
	}
	if !cards[0].Revalidation.Triggered {
		t.Error("the local lane HAS a model id, so the S14.8 runbook must fire")
	}
	if cards[0].Revalidation.Subject != model {
		t.Errorf("revalidation subject = %q, want the seat model %q", cards[0].Revalidation.Subject, model)
	}
	if len(flagged) != 1 || flagged[0] != model {
		t.Errorf("runbook subjects = %v, want [%s]", flagged, model)
	}
}

// TestLogprobCanaryToleranceBoundary: a move JUST INSIDE the tolerance passes
// and raises nothing. Paired with the drift test above, this pins the boundary
// from both sides — a tolerance nothing can cross is not a tolerance.
func TestLogprobCanaryToleranceBoundary(t *testing.T) {
	ctx := context.Background()
	e := newLogprobEnv(t)
	model := e.seatModel(t)

	e.setMargin(t, model, 6.0)
	if _, err := e.c.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	// 0.9 of a nat: inside the 1.0 tolerance.
	e.setMargin(t, model, 5.1)
	e.clk.advance(25 * time.Hour)
	if _, err := e.c.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	got := e.canaryEvents(t)
	if len(got) != 2 {
		t.Fatalf("canary.result rows = %d, want 2", len(got))
	}
	if got[1].Result != watchlist.CanaryPass {
		t.Fatalf("a 0.9-nat move (inside the 1.0 tolerance) failed the canary: %+v", got[1])
	}
	if cards := e.findings(t); len(cards) != 0 {
		t.Errorf("a within-tolerance move raised %d cards, want 0", len(cards))
	}
}

// TestLogprobCanaryHonestWhenNoMarginIsAvailable: a seat that returns no top-2
// alternatives yields NO margin. That is recorded as an honest failure carrying
// the abstain class — never as a margin of 0, which would read as a coin-flip
// the model never made.
func TestLogprobCanaryHonestWhenNoMarginIsAvailable(t *testing.T) {
	ctx := context.Background()
	e := newLogprobEnv(t)
	model := e.seatModel(t)

	// A schema-valid answer with logprobs carrying only ONE alternative each:
	// no gap is defined anywhere.
	// Tokens with NO alternatives at all: local.TokenMargin is undefined when a
	// token carries fewer than two, so no gap exists anywhere in the stream.
	e.fake.SetModelResponse(model, local.FakeResponse{
		Content: `{"reason":"r","change_class":"price","lanes":["local"],"summary":"s"}`,
		Logprobs: []local.TokenLogprob{
			{Token: `{"reason":"r","change_class":"`, Logprob: -0.01},
			{Token: "price", Logprob: -0.05},
			{Token: `","lanes":["local"],"summary":"s"}`, Logprob: -0.01},
		},
		InputTokens: 10, OutputTokens: 4,
	})
	if _, err := e.c.RunDue(ctx); err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	got := e.canaryEvents(t)
	if len(got) != 1 {
		t.Fatalf("canary.result rows = %d, want 1", len(got))
	}
	if got[0].Result != watchlist.CanaryFail {
		t.Errorf("result = %q, want fail — an unavailable margin is not a pass", got[0].Result)
	}
	if got[0].Measurement != nil {
		t.Errorf("measurement = %v, want none recorded — a fabricated 0 would read as a coin-flip", *got[0].Measurement)
	}
	if got[0].ChangeClass != watchlist.ClassUnclear {
		t.Errorf("change class = %q, want unclear (the abstain member)", got[0].ChangeClass)
	}
	if got[0].Error == "" {
		t.Error("the honest branch recorded no reason")
	}
	// It must NOT poison the next run's baseline: with no measurement recorded,
	// a later real margin is still a first observation.
	if _, ok, err := e.c.LastMeasurement(ctx, watchlist.CanaryLogprob, watchlist.LaneLocal); err != nil || ok {
		t.Errorf("LastMeasurement ok = %v (err %v), want no measurement recorded", ok, err)
	}
}

// TestLogprobCanaryIsMeteredOrSkipped: with a Duty but NO advisory meter the
// canary is skipped honestly rather than issued unmetered, and the skip is a
// STACK absence, not an arming matter (drain D5).
func TestLogprobCanaryIsMeteredOrSkipped(t *testing.T) {
	ctx := context.Background()
	e := newLogprobEnv(t)
	e.c.Logprob = watchlist.NewLogprobCanary(e.duty, nil)

	sweep, err := e.c.RunDue(ctx)
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if sweep.Ran != 0 || sweep.StackAbsent != 1 || sweep.Disarmed != 0 {
		t.Fatalf("sweep = %+v, want one stack-absent skip", sweep)
	}
	if len(sweep.Reasons) != 1 || !strings.Contains(sweep.Reasons[0], "unmetered") {
		t.Errorf("reasons = %v, want the skip to say the probe was not issued unmetered", sweep.Reasons)
	}
	if got := e.canaryEvents(t); len(got) != 0 {
		t.Errorf("an unmetered canary recorded %d results, want 0", len(got))
	}
}
