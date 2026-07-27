package shell

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/retention"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// The Spec S14.9 retention substrate, composed at the root (B5-8A).
//
// internal/retention is a LEAF: storage + eventlog + stdlib. The two edges it
// needs — the run's terminal transition and the local tier's narrative pass —
// are func seams satisfied HERE, exactly as the S14.7 benchmark arms and the
// S14.8 eval surfaces are (CONVENTIONS §28/§33/§34/§35). The shell owns WHEN;
// the package owns the LOGIC.

// retentionSurface holds the composed S14.9/S14.10 machinery.
type retentionSurface struct {
	Store *retention.Store
	// KeepForever counts the allowlist rows seeded at this boot (the 11.3
	// exporter boundary's data).
	KeepForever int
	// NarratorWired records whether the local-tier enrichment leg exists. False
	// is an HONEST state, not a failure: every summary is still written and
	// flagged aggregate-only (S14.9 ¶1 — the local tier enriches, never gates).
	NarratorWired bool
}

// buildRetentionSurface composes the S14.9 retention store, seeds the
// keep-forever allowlist, and installs the run-end summary hook.
//
// The allowlist is seeded unconditionally at every boot (the conformance-
// registry / watch-row posture): it is what makes the 11.3 exporter boundary
// structural, and an unseeded table would strip every payload from the export —
// the safe direction, but not the correct one.
func buildRetentionSurface(
	ctx context.Context,
	db *storage.DB,
	log *eventlog.Log,
	reg *settings.Registry,
	runs *run.Store,
	duty *local.Duty,
	checkpoints *gates.Checkpoints,
	logger *slog.Logger,
) (*retentionSurface, error) {
	narrator := summaryNarrator(duty, advisoryMeter(runs, checkpoints))
	store, err := retention.New(retention.Config{
		DB:       db,
		Log:      log,
		Settings: reg,
		Narrator: narrator,
	})
	if err != nil {
		return nil, err
	}
	seeded, err := store.EnsureKeepForeverSeeded(ctx)
	if err != nil {
		return nil, fmt.Errorf("shell: seed the S14.9 keep-forever allowlist: %w", err)
	}
	// A restore-from-dump leaves the FTS index empty behind a cursor that says
	// otherwise (the dump never carries FTS content). Repair it at boot.
	rebuilt, err := store.ReconcileIndex(ctx)
	if err != nil {
		return nil, fmt.Errorf("shell: reconcile the S14.10 derived index: %w", err)
	}
	if rebuilt {
		logger.Info("retention: derived index rebuilt from the log after a restore (S14.10)")
	}

	// The run-end hook (Spec S14.9 ¶1: "at run end, never later"). One edge
	// covers every terminal transition in the tree.
	if err := runs.SetTerminalHook(func(ctx context.Context, tx *sql.Tx, r run.Run) error {
		written, reason, err := store.WriteAtRunEndTx(ctx, tx, r.ID)
		if err != nil {
			return err
		}
		if !written {
			logger.Debug("retention: no run summary for this ending", "run", r.ID, "reason", reason)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("shell: install the S14.9 run-end hook: %w", err)
	}

	return &retentionSurface{Store: store, KeepForever: seeded, NarratorWired: narrator != nil}, nil
}

// summaryNarrator is the S14.9 ¶1 local-tier leg on duty alias
// `distill-summarize` (S12.4: "L1 distiller; run summary at run end"), with the
// S12.4 shaping that row mandates: grounding instructions, extract-then-abstract,
// EVENT-ID CITATION FORCING, a length cap and a section schema.
//
// It is a DRAFTING duty, not a classification one: no json_schema, no logprobs,
// no abstain member (S12.4 splits those two shapes). The abstain belt does not
// apply and is not asserted.
//
// Metering rides the advisory platform run (the S09.7 contradiction-screen
// precedent, §31 OQ4): the summarized run is TERMINAL by the time this is
// called, so it cannot take a D7 checkpoint, and an unmetered local call is a
// surfaced defect (S12.1 R18). With no meter and no duty the seam is nil and
// every summary stands aggregate-only, flagged.
func summaryNarrator(duty *local.Duty, meter stage.AdvisoryMeter) retention.Narrator {
	if duty == nil {
		return nil
	}
	return func(ctx context.Context, runID string, agg retention.Aggregate) (retention.Narrative, error) {
		meterRun, settle := stage.BeginAdvisory(ctx, meter, "run-summary")
		if settle != nil {
			defer settle()
		}
		res, err := duty.Call(ctx, meterRun, local.DutyRequest{
			Alias:     local.AliasDistillSummarize,
			System:    summaryNarratorSystem,
			User:      summaryNarratorPrompt(agg),
			MaxTokens: summaryNarratorTokens,
		})
		if err != nil {
			return retention.Narrative{}, err
		}
		return retention.Narrative{Text: strings.TrimSpace(res.Content), Model: res.Model}, nil
	}
}

// summaryNarratorTokens is the S12.4 length cap for the narrative pass — a
// structural constant set by the caller, the shape local.DutyRequest documents
// (S18 ratifies no key). Its reason: S14.9 calls the summary "a compact
// structured story", and the deterministic aggregate already carries every
// figure — the narrative adds only the connective prose, which does not grow
// with the length of the trace.
const summaryNarratorTokens = 700

const summaryNarratorSystem = `You write the narrative half of a run summary for an audit log.
GROUNDING: every statement must be supported by the structured record you are given. You have no other knowledge of this run.
EXTRACT THEN ABSTRACT: first note which recorded facts matter, then write the story from those facts only.
CITATIONS: cite the event ids you rely on inline as [#<event_seq>]. A claim with no event id is not allowed.
NEVER invent a stage, a tool, a verdict, a decision, a cost or an outcome that is not in the record. If the record does not say, write that the record does not say.
Write plain prose. No preamble, no headings beyond the sections asked for, no speculation about causes.`

// summaryNarratorPrompt renders the section schema and the grounded facts. The
// deterministic aggregate is passed as the record; the model narrates it and
// never recomputes it (S14.9 ¶1: deterministic aggregation FIRST, the local
// tier second).
func summaryNarratorPrompt(agg retention.Aggregate) string {
	var b strings.Builder
	b.WriteString("Write these sections, in this order, one short paragraph each:\n")
	b.WriteString("WHAT WAS ASKED / WHAT HAPPENED / HOW IT ENDED\n\n")
	b.WriteString("THE RECORD:\n")
	fmt.Fprintf(&b, "run: %s (owner %s)\n", agg.RunID, agg.Owner)
	fmt.Fprintf(&b, "objective (%s): %s\n", agg.ObjectiveSource, orNone(agg.Objective))
	fmt.Fprintf(&b, "final state: %s\n", agg.FinalState)
	fmt.Fprintf(&b, "events: %d (seq %d..%d)\n", agg.EventCount, agg.FirstEventSeq, agg.LastEventSeq)
	b.WriteString("stages:\n")
	for _, st := range agg.Stages {
		fmt.Fprintf(&b, "  [#%d] %s -> %s\n", st.EventSeq, orNone(st.From), st.Name)
	}
	// Counts, never names (S14.9 ¶1 asks for tool-call COUNTS; drain D8).
	fmt.Fprintf(&b, "tool calls: %d across %d distinct tools (%d unnamed)\n",
		agg.ToolCalls.Total, agg.ToolCalls.Distinct, agg.ToolCalls.Unnamed)
	b.WriteString("verdicts:\n")
	for _, v := range agg.Verdicts {
		fmt.Fprintf(&b, "  [#%d] round %d %s %s\n", v.EventSeq, v.Round, v.Verdict, v.RubricID)
	}
	b.WriteString("decisions:\n")
	for _, d := range agg.Decisions {
		fmt.Fprintf(&b, "  [#%d] %s %s by %s\n", d.EventSeq, d.Type, d.Decision, d.Actor)
	}
	fmt.Fprintf(&b, "receipts: %d materialized over %d checkpointed paid calls. %s\n",
		agg.Receipts.Count, agg.Receipts.Checkpoints, agg.Receipts.Note)
	return b.String()
}

func orNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(the record does not say)"
	}
	return s
}

// retentionIndexLoop drives the S14.10 ¶4 derived index and the S14.9 ¶1
// local-tier enrichment. The shell owns WHEN; both verbs are idempotent and
// restart-safe from stored state (a durable cursor and a per-row status), so a
// missed tick costs promptness and nothing else.
func retentionIndexLoop(ctx context.Context, rs *retentionSurface, logger *slog.Logger) {
	t := time.NewTicker(retention.IndexInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if res, err := rs.Store.Index(ctx); err != nil {
				logger.Warn("retention: index pass failed", "err", err)
			} else if res.Indexed > 0 || res.Rolled > 0 {
				logger.Info("retention: index pass complete",
					"indexed", res.Indexed, "rolled", res.Rolled, "through_seq", res.ThroughSeq)
			}
			if n, err := rs.Store.EnrichPending(ctx, retentionEnrichBatch); err != nil {
				logger.Warn("retention: summary enrichment failed", "err", err)
			} else if n > 0 {
				logger.Info("retention: run summaries enriched", "count", n)
			}
		}
	}
}

// retentionEnrichBatch bounds how many pending summaries one tick narrates — a
// structural constant. Its reason: each narration is a local model call on the
// workhorse seat, so the batch bounds how long one tick holds that seat; the
// backlog is drained across ticks and nothing is skipped.
const retentionEnrichBatch = 4

// retentionCompactLoop drives the S14.9 ¶2 compaction pass. It runs once at
// start and then on the daily-equivalent tick, so a host that is rebooted more
// often than the interval still compacts (the dead-man lesson: a bare ticker
// resets on restart and freezes across suspend).
func retentionCompactLoop(ctx context.Context, rs *retentionSurface, logger *slog.Logger) {
	runPass := func() {
		res, err := rs.Store.Compact(ctx, time.Time{})
		if err != nil {
			logger.Warn("retention: compaction pass failed", "err", err)
			return
		}
		if res.EventsStripped > 0 {
			logger.Info("retention: compaction pass complete (Spec S14.9)",
				"events_stripped", res.EventsStripped,
				"bytes_reclaimed", res.BytesReclaimed,
				"owners", len(res.Owners))
		}
	}
	runPass()
	t := time.NewTicker(retention.CompactionInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			runPass()
		}
	}
}

// assertStackAbsentMarker pins internal/retention's StackAbsentMarker to the
// real local.ErrStackAbsent text. internal/retention cannot import
// internal/local (import wall), so it classifies an absent stack by that
// substring; if the sentinel is ever reworded, a summary would be flagged
// `failed` instead of `absent` — both honest, but the wrong one. This fails the
// build's test run instead.
func assertStackAbsentMarker() error {
	if !strings.Contains(local.ErrStackAbsent.Error(), retention.StackAbsentMarker) {
		return fmt.Errorf("shell: retention.StackAbsentMarker %q no longer appears in local.ErrStackAbsent %q",
			retention.StackAbsentMarker, local.ErrStackAbsent)
	}
	return nil
}
