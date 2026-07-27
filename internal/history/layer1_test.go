package history_test

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/history"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
)

// layer1_test.go — TIER F throughout: a local.FakeServer + a real local.Duty
// (the internal/watchlist canary_logprob_test composition), so the whole
// production path runs — duty call, engine-enforced schema round-trip,
// local.LabelMargin extraction, the calibration gate, the card — with ZERO
// network and ZERO paid calls. The live seat is a bring-up act (SANCTIONED
// SKIP, §10); nothing here dials one.

// stack is a fixture with the fake local tier wired in.
type stack struct {
	*fixture
	fake *local.FakeServer
	cal  *local.CalStore
	seat local.SeatRecord
}

func newStack(t *testing.T, withCal bool) *stack {
	t.Helper()
	fake := local.NewFakeServer()
	t.Cleanup(fake.Close)

	var s stack
	// The local registry needs the fixture's settings, so the duty is wired
	// after the fixture exists and the store is rebuilt around it.
	f := newFixture(t)
	lreg := local.NewRegistry(f.reg)
	duty := local.NewDuty(local.DutyDeps{
		Registry:    lreg,
		Client:      local.NewClient(fake.URL),
		Checkpoints: gates.NewCheckpoints(f.db, f.log),
		Events:      f.log,
	})
	seat, err := duty.ResolveSeat(local.AliasIntentFilling)
	if err != nil {
		t.Fatalf("resolve the intent-filling seat: %v", err)
	}
	if !seat.Servable {
		t.Skipf("SANCTIONED SKIP: the %s seat is not servable in this manifest (%s)", local.AliasIntentFilling, seat.Note)
	}
	var cal *local.CalStore
	if withCal {
		cal = local.NewCalStore(f.db)
	}
	runs := run.NewStore(f.db, f.log)
	advisory := func(ctx context.Context, label string) (string, func()) {
		id := "platform.advisory." + label + "." + strconv.FormatInt(time.Now().UnixNano(), 10)
		if _, err := runs.Create(ctx, run.NewRun{
			ID: id, UserID: run.ActorPlatform, Lane: local.LaneLocal, Substrate: "local",
		}); err != nil {
			return "", nil
		}
		for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
			if _, err := runs.Transition(ctx, id, st, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
				return "", nil
			}
		}
		return id, func() {
			_, _ = runs.Transition(context.WithoutCancel(ctx), id, run.StateCompleted,
				run.TransitionOptions{Actor: run.ActorPlatform})
		}
	}
	st, err := history.New(history.Config{
		DB: f.db, Log: f.log, Duty: duty, Cal: cal, Advisory: advisory,
		Now: func() time.Time { return f.now },
	})
	if err != nil {
		t.Fatalf("history.New: %v", err)
	}
	f.st = st
	s.fixture, s.fake, s.cal, s.seat = f, fake, cal, seat
	return &s
}

// answer sets the fake model's reply for the next call.
func (s *stack) answer(content string, lp []local.TokenLogprob) {
	s.fake.SetResponse(local.FakeResponse{Content: content, Logprobs: lp, InputTokens: 20, OutputTokens: 8})
}

// margins builds a logprob stream whose `query` VALUE token carries a given
// top1−top2 gap — the S12.5 signal local.LabelMargin extracts. It uses
// local.TokenLogprobFixture, the constructor internal/local ships for exactly
// this (the internal/watchlist marginTokens precedent).
func margins(query string, gap float64) []local.TokenLogprob {
	return []local.TokenLogprob{
		local.TokenLogprobFixture(`{"reason":"because","query":"`, 9.0),
		local.TokenLogprobFixture(query, gap),
		local.TokenLogprobFixture(`","abstain":false}`, 9.0),
	}
}

// TestIntentSchemaIsGrammarConstrainedReasonFirst — acceptance 28, first half.
// The required `reason` field must be FIRST in PROPERTY ORDER (the F5 finding:
// a Go map marshals alphabetically, which would reorder the grammar), the query
// enum must carry the catalog plus an abstain member, and the duty must be sent
// as a classification (schema + logprobs).
func TestIntentSchemaIsGrammarConstrainedReasonFirst(t *testing.T) {
	names := history.CatalogNames()
	schema := string(history.IntentSchema(names))

	iReason := strings.Index(schema, `"reason"`)
	iQuery := strings.Index(schema, `"query"`)
	iAbstain := strings.Index(schema, `"abstain"`)
	if iReason < 0 || iQuery < 0 || iAbstain < 0 {
		t.Fatalf("intent schema is missing a required member: %s", schema)
	}
	if !(iReason < iQuery && iQuery < iAbstain) {
		t.Errorf("intent schema property order is reason(%d) query(%d) abstain(%d) — reason must be FIRST (S12.4/F5)", iReason, iQuery, iAbstain)
	}
	if !strings.Contains(schema, `"abstain"`) {
		t.Error("no abstain member — a model is never schema-forced to fabricate a label (S12.4)")
	}
	if !strings.Contains(schema, `"additionalProperties":false`) {
		t.Error("the schema is not strict — the engine constraint must be tight")
	}
	for _, n := range names {
		if !strings.Contains(schema, `"`+n+`"`) {
			t.Errorf("catalog query %q is not in the intent enum", n)
		}
	}

	// The slot grammar has the same shape.
	q, _ := history.QueryByName("status.runs_by_state")
	slotSchema := string(history.SlotSchema(q.Slots))
	if strings.Index(slotSchema, `"reason"`) > strings.Index(slotSchema, `"state"`) {
		t.Errorf("slot schema does not put reason first: %s", slotSchema)
	}
	if !strings.Contains(slotSchema, `"abstain"`) {
		t.Error("slot schema has no abstain member")
	}
}

// TestAskResolvesIntentAndNamesTheQuery — the happy path, end to end through a
// real duty call against the fake seat.
func TestAskResolvesIntentAndNamesTheQuery(t *testing.T) {
	s := newStack(t, false)
	seedTwoOwners(t, s.fixture)
	s.answer(`{"reason":"the asker wants live runs","query":"status.runs_active","abstain":false}`,
		margins("status.runs_active", 4.0))

	a, err := s.st.Ask(s.ctx, "what is running right now?", opScope(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if a.Card != nil {
		t.Fatalf("a resolvable question produced a card: %+v", a.Card)
	}
	if a.Query != "status.runs_active" {
		t.Errorf("answer names %q, want status.runs_active (R26)", a.Query)
	}
	if a.Layer != history.LayerCanned || a.Confidence != history.ConfidenceCanned {
		t.Errorf("layer/confidence = %d/%q", a.Layer, a.Confidence)
	}
	if len(a.Rows) == 0 {
		t.Error("the resolved query returned nothing — the seeded active run should appear")
	}

	// The duty was shaped as a CLASSIFICATION: engine-enforced schema and
	// logprobs requested (S12.4).
	req := s.fake.LastRequest()
	if !req.HasJSONSchema {
		t.Error("the intent duty was sent without a json_schema — the grammar constraint is engine-enforced")
	}
	if !req.Logprobs {
		t.Error("the intent duty was sent without logprobs — the S12.5 margin has nothing to read")
	}
	if req.SchemaName == "" {
		t.Error("the intent duty carried no schema name")
	}
}

// TestAbstainProducesADisambiguationCardNeverAGuess — acceptance 28, second
// half. S12.4's intent-filling row fixes the failure mode: "below threshold ⇒ a
// 'which of these did you mean?' card — never a guess."
func TestAbstainProducesADisambiguationCardNeverAGuess(t *testing.T) {
	s := newStack(t, false)
	seedTwoOwners(t, s.fixture)
	s.answer(`{"reason":"nothing in the catalog answers this","query":"abstain","abstain":true}`,
		margins("abstain", 6.0))

	a, err := s.st.Ask(s.ctx, "what is the weather in Oslo?", opScope(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if a.Card == nil {
		t.Fatal("an abstain did not produce a disambiguation card — the model must never be turned into a guess")
	}
	if len(a.Rows) != 0 {
		t.Errorf("a card answer carried %d rows — nothing was run, so nothing may be shown", len(a.Rows))
	}
	if len(a.Card.Choices) == 0 {
		t.Error("the card offers no choices — 'which of these did you mean?' needs the 'these'")
	}
	for _, c := range a.Card.Choices {
		if _, ok := history.QueryByName(c.Query); !ok {
			t.Errorf("the card offers %q, which is not a catalog query", c.Query)
		}
		if c.Description == "" {
			t.Errorf("the card offers %q with no description", c.Query)
		}
	}
	if !strings.Contains(a.Card.Reason, "abstain") {
		t.Errorf("the card does not say why it appeared: %q", a.Card.Reason)
	}
}

// TestBelowThresholdProducesACard — the S12.5 margin gate. `intent-filling@4B`
// is a calibrated duty, so the threshold is real platform data; a below-
// threshold label is a card, not a guess.
func TestBelowThresholdProducesACard(t *testing.T) {
	s := newStack(t, true)
	seedTwoOwners(t, s.fixture)

	// Calibrate the duty at a threshold the answer below will not clear.
	if err := s.cal.SaveCalibration(s.ctx, local.Calibration{
		Key: local.CalibrationKey{
			Duty: local.AliasIntentFilling, ModelHash: s.seat.ModelHash(), EngineBuild: local.LlamaCppPin,
		},
		Threshold: 3.0, AcceptanceBar: 0.05, LabeledN: 40, MeetsBar: true,
	}); err != nil {
		t.Fatalf("SaveCalibration: %v", err)
	}

	// A confident answer clears it and runs.
	s.answer(`{"reason":"live runs","query":"status.runs_active","abstain":false}`,
		margins("status.runs_active", 9.0))
	ok, err := s.st.Ask(s.ctx, "what is running right now?", opScope(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if ok.Card != nil {
		t.Fatalf("a confident answer was carded: %+v", ok.Card)
	}

	// The SAME answer below the threshold is a card. This is the control that
	// makes the gate non-tautological: only the margin changed.
	s.answer(`{"reason":"live runs","query":"status.runs_active","abstain":false}`,
		margins("status.runs_active", 0.2))
	low, err := s.st.Ask(s.ctx, "what is running right now?", opScope(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if low.Card == nil {
		t.Fatal("a below-threshold label was accepted as an answer — S12.4 requires a card, never a guess")
	}
	if len(low.Rows) != 0 {
		t.Error("a carded answer carried rows")
	}
	if !strings.Contains(low.Card.Reason, "threshold") {
		t.Errorf("the card does not name the reason it appeared: %q", low.Card.Reason)
	}
	if len(low.Card.Choices) == 0 {
		t.Error("the low-margin card offers no alternatives")
	}
}

// TestUnfilledSlotProducesACard — the second grammar's failure mode. A query
// that needs a value the question never supplied is a card, never an invented
// filter.
func TestUnfilledSlotProducesACard(t *testing.T) {
	s := newStack(t, false)
	seedTwoOwners(t, s.fixture)
	// The intent resolves to a slotted query, then the filler abstains.
	s.fake.SetResponse(local.FakeResponse{
		Content:  `{"reason":"they want one task","query":"status.runs_for_task","abstain":false}`,
		Logprobs: margins("status.runs_for_task", 8.0), InputTokens: 10, OutputTokens: 4,
	})
	first, err := s.st.Ask(s.ctx, "show me the runs for that task", opScope(), 20)
	if err != nil {
		t.Fatal(err)
	}
	// The fake returns the SAME body for the slot call, which carries no
	// `task_id` member — an unfilled required slot.
	if first.Card == nil {
		t.Fatal("a query whose slot could not be filled was run anyway — a missing filter widens the answer")
	}
	if !strings.Contains(first.Card.Reason, "task_id") && !strings.Contains(first.Card.Reason, "value") {
		t.Errorf("the card does not say what was missing: %q", first.Card.Reason)
	}
}

// TestAskWithoutALocalStackStillAnswersWithTheFloor — the local tier ENRICHES
// the query surface; it never gates it. With no stack the catalog is still the
// reliability floor and the card is how it is offered.
func TestAskWithoutALocalStackStillAnswersWithTheFloor(t *testing.T) {
	f := newFixture(t) // no Duty, no Advisory
	seedTwoOwners(t, f)

	a, err := f.st.Ask(f.ctx, "what is running right now?", opScope(), 20)
	if err != nil {
		t.Fatalf("an absent local stack made the question FAIL: %v", err)
	}
	if a.Card == nil {
		t.Fatal("no card with the stack absent")
	}
	if !strings.Contains(a.Card.Reason, "local tier") {
		t.Errorf("the card does not state the honest reason: %q", a.Card.Reason)
	}
	if len(a.Card.Choices) == 0 {
		t.Error("the card offers nothing — the catalog is always available")
	}
	// And the named query still runs, with no model anywhere in the path.
	direct, err := f.st.RunQuery(f.ctx, "status.runs_active", nil, opScope(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(direct.Rows) == 0 {
		t.Error("the model-free floor returned nothing")
	}
}

// TestLayer2IsNotReachedBySilentFallthrough — S14.10 ¶3: "canned queries remain
// the floor; Layer 2 is escalation, never default." No Layer-1 path may produce
// a Layer-2 answer, and the unresolved path produces a CARD rather than an
// escalation.
func TestLayer2IsNotReachedBySilentFallthrough(t *testing.T) {
	s := newStack(t, false)
	seedTwoOwners(t, s.fixture)
	s.answer(`{"reason":"no match","query":"abstain","abstain":true}`, margins("abstain", 7.0))

	a, err := s.st.Ask(s.ctx, "something the catalog does not cover", opScope(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if a.Layer == history.LayerOpenSQL {
		t.Fatal("an unresolved Layer-1 question fell through to Layer 2 — escalation is never a default (S14.10 ¶3)")
	}
	if a.Card == nil {
		t.Fatal("the unresolved path did not produce a card")
	}
}
