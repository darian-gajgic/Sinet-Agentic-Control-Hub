package shell

import (
	"context"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/retention"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/stage"
)

// retention_seams_test.go — the composition-root end of the S14.9 ¶1 local-tier
// leg. Tier F: a local.FakeServer + a real local.Duty, so the whole production
// path runs — alias resolution, request shaping, the D7 metering seam — with
// ZERO network and ZERO paid calls. $0 by construction; a live seat is a
// bring-up act (SANCTIONED SKIP, CONVENTIONS §10).

// TestSummaryNarratorRidesTheDistillSummarizeAlias (rubric 4): the narrative
// pass addresses an ALIAS, never a hardcoded model, so an operator retarget is
// honored and the seat is swap-invisible (S12.4 / R15).
func TestSummaryNarratorRidesTheDistillSummarizeAlias(t *testing.T) {
	ctx := context.Background()
	fake := local.NewFakeServer()
	defer fake.Close()
	fake.SetResponse(local.FakeResponse{
		Content: "WHAT WAS ASKED: the record says nothing [#1].", InputTokens: 40, OutputTokens: 12,
	})

	h := newRetentionHarness(t, fake)
	narrator := summaryNarrator(h.duty, h.meter)
	if narrator == nil {
		t.Fatal("the narrator seam is nil with a live duty wired")
	}

	agg := retention.Aggregate{
		RunID: "t1.r1", Owner: "alice", Objective: "Ship the widget",
		ObjectiveSource: "tasks.title", FinalState: "completed",
		FirstEventSeq: 1, LastEventSeq: 9, EventCount: 9,
		Stages:    []retention.Stage{{EventSeq: 4, Name: "running", From: "claimed"}},
		ToolCalls: retention.ToolCalls{Total: 2, Distinct: 1},
		Verdicts:  []retention.VerdictRef{{EventSeq: 7, Round: 1, Verdict: "pass", RubricID: "rubric-software"}},
	}
	nar, err := narrator(ctx, "t1.r1", agg)
	if err != nil {
		t.Fatalf("narrator: %v", err)
	}
	if nar.Text == "" {
		t.Error("the narrator returned no text")
	}
	if nar.Model == "" {
		t.Error("the answering seat must be recorded on the narrative")
	}

	req := fake.LastRequest()
	seat, err := h.duty.ResolveSeat(local.AliasDistillSummarize)
	if err != nil {
		t.Fatalf("resolve %s: %v", local.AliasDistillSummarize, err)
	}
	if req.Model != seat.Model {
		t.Errorf("the call went to model %q; want the seat ⚙ local.alias resolves %q to (%q) — never a hardcoded model",
			req.Model, local.AliasDistillSummarize, seat.Model)
	}

	// S12.4 splits drafting from classification: this is a DRAFTING duty, so no
	// json_schema, no logprobs, and no abstain member is asserted.
	if req.HasJSONSchema {
		t.Error("the narrative pass sent a json_schema; distill-summarize is a drafting duty (S12.4)")
	}
	if req.Logprobs {
		t.Error("the narrative pass requested logprobs; that belt is for classification duties")
	}
	// A length cap rides the request (S12.4 "length caps").
	if req.MaxTokens != summaryNarratorTokens || req.MaxTokens == 0 {
		t.Errorf("max_tokens = %d, want the structural length cap %d", req.MaxTokens, summaryNarratorTokens)
	}
}

// TestSummaryNarratorCarriesTheS124Shaping: the S12.4 distill-summarize row
// mandates "grounding instructions, extract-then-abstract, event-id
// citation-forcing, length caps, section schema". Each limb is in the request.
func TestSummaryNarratorCarriesTheS124Shaping(t *testing.T) {
	ctx := context.Background()
	fake := local.NewFakeServer()
	defer fake.Close()
	fake.SetResponse(local.FakeResponse{Content: "narrative", InputTokens: 10, OutputTokens: 2})

	h := newRetentionHarness(t, fake)
	agg := retention.Aggregate{
		RunID: "t1.r1", Owner: "alice", FinalState: "completed",
		ObjectiveSource: "absent", LastEventSeq: 3,
		Decisions: []retention.DecisionRef{{EventSeq: 3, Type: "decision.recorded", Decision: "approved", Actor: "alice"}},
	}
	if _, err := summaryNarrator(h.duty, h.meter)(ctx, "t1.r1", agg); err != nil {
		t.Fatal(err)
	}
	req := fake.LastRequest()
	var system, user string
	for _, m := range req.Messages {
		switch m.Role {
		case "system":
			system = m.Content
		case "user":
			user = m.Content
		}
	}
	for _, limb := range []struct{ name, needle, in string }{
		{"grounding", "GROUNDING", system},
		{"extract-then-abstract", "EXTRACT THEN ABSTRACT", system},
		{"event-id citation forcing", "[#<event_seq>]", system},
		{"never invent", "NEVER invent", system},
		{"section schema", "WHAT WAS ASKED / WHAT HAPPENED / HOW IT ENDED", user},
		{"the grounded record", "THE RECORD:", user},
	} {
		if !strings.Contains(limb.in, limb.needle) {
			t.Errorf("the %s limb of the S12.4 distill-summarize shaping is missing (%q)", limb.name, limb.needle)
		}
	}
	// The record is passed as FACTS with their event ids — the model narrates
	// the deterministic aggregate, it never recomputes it.
	if !strings.Contains(user, "[#3]") {
		t.Error("the recorded facts must carry their event ids so the citation forcing is satisfiable")
	}
	// An honest absence is rendered as an absence, never as a blank the model
	// might fill in.
	if !strings.Contains(user, "the record does not say") {
		t.Error("an absent objective must render as an explicit absence")
	}
}

// TestSummaryNarratorIsNilWithNoDuty: with no local stack the seam is nil and
// every summary stands aggregate-only, honestly flagged (S14.9 ¶1).
func TestSummaryNarratorIsNilWithNoDuty(t *testing.T) {
	if summaryNarrator(nil, nil) != nil {
		t.Error("the narrator seam must be nil with no duty wired")
	}
}

// TestStackAbsentMarkerIsPinnedToTheSentinel: internal/retention classifies an
// absent stack by substring because the import wall keeps it a leaf. The boot
// path asserts the two agree; this is that assertion, run in CI.
func TestStackAbsentMarkerIsPinnedToTheSentinel(t *testing.T) {
	if err := assertStackAbsentMarker(); err != nil {
		t.Error(err)
	}
	if !strings.Contains(local.ErrStackAbsent.Error(), retention.StackAbsentMarker) {
		t.Errorf("retention.StackAbsentMarker %q no longer appears in %q",
			retention.StackAbsentMarker, local.ErrStackAbsent)
	}
}

// TestRetentionDriverIntervalsAreStructuralConstants: neither driver cadence is
// a ⚙ row (S18 ratifies none; ⚙ retention.compaction_horizon is a HORIZON in
// months, not an interval), and each carries a real value.
func TestRetentionDriverIntervalsAreStructuralConstants(t *testing.T) {
	if retention.CompactionInterval <= 0 || retention.IndexInterval <= 0 {
		t.Fatal("both driver intervals must be real durations")
	}
	if retention.CompactionInterval <= retention.IndexInterval {
		t.Error("the compaction tick is daily-equivalent and the index tick is prompt; the ordering is inverted")
	}
	if retentionEnrichBatch <= 0 {
		t.Error("the enrichment batch must bound how long one tick holds the workhorse seat")
	}
}

// retentionHarness is the tier-F composition: a fake serving stack behind a
// real local.Duty, plus the advisory metering seam the narrator rides (the
// summarized run is terminal, so it cannot take a D7 checkpoint).
type retentionHarness struct {
	duty  *local.Duty
	meter stage.AdvisoryMeter
}

func newRetentionHarness(t *testing.T, fake *local.FakeServer) *retentionHarness {
	t.Helper()
	db, log, reg := watchlistTestDeps(t)
	checkpoints := gates.NewCheckpoints(db, log)
	duty := local.NewDuty(local.DutyDeps{
		Registry:    local.NewRegistry(reg),
		Client:      local.NewClient(fake.URL),
		Checkpoints: checkpoints,
		Events:      log,
	})
	return &retentionHarness{duty: duty, meter: advisoryMeter(run.NewStore(db, log), checkpoints)}
}
