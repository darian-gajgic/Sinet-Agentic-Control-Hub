package shell

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/history"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// The Spec S14.10 query surface, composed at the root (B5-8B).
//
// internal/history is a LEAF: storage + eventlog + local + redact + stdlib. The
// ONE edge it cannot own — the advisory run a Layer-1 duty call meters against
// — is a func seam satisfied here, exactly as the S14.9 narrator and the S14.6
// logprob canary are (CONVENTIONS §33/§34/§35/§36).
//
// There is NO LOOP here, and that is the point: the query surface has no
// periodic work at all. Layer 0's views are evaluated on read, Layer 1 runs
// when someone asks, and the FTS corpus it searches is maintained by B5-8A's
// indexer. The shell owns WHEN; here, WHEN is "when a question is asked".

// buildHistorySurface composes the S14.10 three-layer query store.
//
// duty and cal may be nil (no local stack, or the uncalibrated posture): Layer
// 0 and the named Layer-1 verbs are unaffected — they have no model in the path
// — and natural-language Ask degrades to the disambiguation card that lists the
// catalog. The reliability floor is the catalog, and the catalog is always
// there.
func buildHistorySurface(
	ctx context.Context,
	db *storage.DB,
	log *eventlog.Log,
	runs *run.Store,
	checkpoints *gates.Checkpoints,
	duty *local.Duty,
	cal *local.CalStore,
	logger *slog.Logger,
) (*history.Store, func() error, error) {
	// The Layer-2 read-only handle (S14.10 ¶3, R29). It is opened HERE and not
	// inside internal/history because opening the database is the storage
	// seam's business and composition is the shell's — and because it must be
	// opened after the writing handle has created the file and its WAL
	// sidecars, which composing it at the root guarantees.
	//
	// A failure to open it is NOT fatal. Layer 2 is the escalation surface;
	// Layers 0 and 1 are the floor and do not need it, so the platform comes up
	// with an honest absence rather than not coming up.
	var ro *storage.ReadOnly
	closeRO := func() error { return nil }
	ro, err := db.OpenReadOnly(ctx)
	if err != nil {
		logger.Warn("history: the Layer-2 read-only handle could not be opened — open SQL is unavailable, the canned catalog is unaffected",
			"error", err)
		ro = nil
	} else {
		closeRO = ro.Close
	}

	st, err := history.New(history.Config{
		DB:       db,
		ReadOnly: ro,
		Log:      log,
		Duty:     duty,
		Cal:      cal,
		Advisory: historyAdvisory(advisoryMeter(runs, checkpoints)),
	})
	if err != nil {
		_ = closeRO()
		return nil, nil, fmt.Errorf("shell: wire the S14.10 query surface: %w", err)
	}
	logger.Info("history: S14.10 query surface wired (B5-8B)",
		"views", len(history.Views()), "catalog", len(history.Catalog()),
		"intent_wired", duty != nil, "calibrated_gate", cal != nil,
		"open_sql_wired", ro != nil && duty != nil)
	return st, closeRO, nil
}

// historyAdvisory adapts the §31 OQ4 AdvisoryMeter to internal/history's seam.
//
// The S12.1 R18 rule is that every local duty call writes ONE D7 checkpoint on
// a consuming run, and a question asked of the history surface has no run of
// its own — so it rides a short-lived platform-scope advisory run, the same
// shape the S09.7 contradiction screen and the S14.9 narrator use. A failure to
// open one yields an empty run id, which the duty layer refuses; internal/history
// turns that refusal into the disambiguation card rather than into an error,
// because an unavailable classifier must never make a question unanswerable.
func historyAdvisory(meter stage.AdvisoryMeter) history.AdvisoryRun {
	return func(ctx context.Context, label string) (string, func()) {
		return stage.BeginAdvisory(ctx, meter, label)
	}
}
