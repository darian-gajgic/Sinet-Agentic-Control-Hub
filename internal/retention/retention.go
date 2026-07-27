// Package retention is the Spec S14.9 retention substrate: the run summary
// written at run end, the scheduled compaction pass that strips bulky trace
// payloads past ⚙ retention.compaction_horizon, the keep-forever allowlist that
// makes the 11.3 exporter boundary structural, and the S14.10 ¶4 index layer
// (FTS5 + rollups) over what remains.
//
// The package owns the LOGIC; the SHELL owns WHEN (CONVENTIONS §31/§34/§35).
// Every verb here is idempotent and restart-safe from stored state, so a driver
// is a loop with no memory of its own.
//
// It is a LEAF (migration 0015; import wall in importwall_test.go): storage +
// eventlog + stdlib, with the local-tier narrator and the clock riding func
// seams composed at the shell root. ⚙ values ride the package's own narrow
// Settings interface by dotted key, so internal/settings is not imported either
// — the §33/§34/§35 reason holds, an observability leaf stays out of the
// producer graph.
package retention

import (
	"context"
	"fmt"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// Event types minted here (registered in internal/eventlog/contract.go under
// exactly one family each — CONVENTIONS §29).
const (
	// EventSummaryWritten is the S14.9 ¶1 run summary, appended in the same
	// transaction as the run's terminal state change. Payload is the
	// FamilyRunSummary contract minimum: summary artifact ref + generation
	// inputs digest.
	EventSummaryWritten = "run.summary_written"
	// EventCompacted is the S14.9 ¶2 compaction pass logging ITSELF — "the
	// audit trail records its own compaction". Platform-scope, one row per
	// pass, per-owner counts in the payload.
	EventCompacted = "retention.compacted"
)

const eventSchemaVersion = 1

// CompactedPayload is the exact body a stripped run_events payload carries.
// Migration 0015's run_events_payload_compaction_only trigger holds the same
// literal, and TestCompactedMarkerMatchesMigration pins the two byte-equal:
// the trigger permits this body and no other, so the Go side cannot drift into
// writing something the DB would silently accept as "compacted".
const CompactedPayload = `{"compacted":true,"reason":"trace payload stripped past the retention compaction horizon (Spec S14.9)"}`

// ExportStrippedPayload is the body the run_events_export view substitutes for
// a non-keep-forever payload. Declared here so a test can assert it never
// appears with a real body beside it; the view in migration 0015 is the
// authority and TestExportMarkerMatchesMigration pins them equal.
const ExportStrippedPayload = `{"exported":false,"reason":"raw trace payload body stays local (Spec S14.9 11.3 boundary, S13.10)"}`

// keyCompactionHorizon is the one ⚙ key this package consumes, by dotted key
// (CONVENTIONS §2). It is PerUser (S18: Int, months, default 6, Min 1, no Max),
// so it is ALWAYS read through IntFor for the owner being compacted — never
// Int, which would silently apply the platform default to everyone.
const keyCompactionHorizon = "retention.compaction_horizon"

// Settings is the narrow ⚙ seam (the internal/eventlog, internal/storage and
// internal/watchlist house pattern): satisfied by *settings.Registry without
// this package importing it.
type Settings interface {
	IntFor(key, userID string) (int64, error)
}

// Narrator is the S14.9 ¶1 local-tier leg: the narrative pass over an already
// written deterministic aggregate. It rides duty alias `distill-summarize`
// (S12.4 "L1 distiller; run summary at run end"), composed at the shell root so
// this package never imports internal/local.
//
// It ENRICHES, never gates. It is called strictly AFTER the run has ended and
// its summary is committed, never inside the terminal transaction — a model
// call inside a write transaction would hold the single writer for the length
// of an inference, and a run must never fail to end because the local tier is
// down.
type Narrator func(ctx context.Context, runID string, agg Aggregate) (Narrative, error)

// Narrative is one narrator result. Model records which seat answered, so a
// reader can tell whose prose it is.
type Narrative struct {
	Text  string
	Model string
}

// Config wires a Store.
type Config struct {
	DB       *storage.DB
	Log      *eventlog.Log
	Settings Settings
	// Narrator is the local-tier enrichment seam. nil = no narrator wired: a
	// summary is written aggregate-only and honestly flagged `absent`.
	Narrator Narrator
	// Now is the clock seam (hermetic tests drive the horizon). nil = time.Now.
	Now func() time.Time
	// RemoveFile deletes a transcript copy-aside from disk during compaction.
	// nil = os.Remove. Seamed so a test can observe the filesystem leg without
	// a temp-file dance in every case.
	RemoveFile func(path string) error
	// AuditFault is a FAULT-INJECTION seam over the compaction pass's audit
	// append, and exists for one reason: to make the S14.9 ¶2 atomicity
	// OBSERVABLE (drain r2 R3).
	//
	// Every unit of compaction work is one transaction holding both the payload
	// elisions and the retention.compacted row that counts them, so a committed
	// strip is never unaudited. That coupling is invisible to a test that only
	// checks the happy path — the audit append could be moved into a follow-up
	// transaction and every other assertion would still pass. With this seam a
	// test can fail the append INSIDE the unit tx and assert the strips rolled
	// back, so a future decoupling edit fails loudly instead of silently.
	//
	// nil in production, always: nothing sets it outside a test.
	AuditFault func() error
}

// Store is the S14.9/S14.10 retention surface.
type Store struct {
	db       *storage.DB
	log      *eventlog.Log
	settings Settings
	narrator Narrator
	now      func() time.Time
	remove   func(path string) error
	// auditFault is the R3 atomicity-guard seam; nil in production.
	auditFault func() error
}

// New builds a Store. DB, Log and Settings are required; the rest default.
func New(cfg Config) (*Store, error) {
	if cfg.DB == nil || cfg.Log == nil || cfg.Settings == nil {
		return nil, fmt.Errorf("retention: DB, Log and Settings are required")
	}
	s := &Store{
		db:         cfg.DB,
		log:        cfg.Log,
		settings:   cfg.Settings,
		narrator:   cfg.Narrator,
		now:        cfg.Now,
		remove:     cfg.RemoveFile,
		auditFault: cfg.AuditFault,
	}
	if s.now == nil {
		s.now = func() time.Time { return time.Now().UTC() }
	}
	if s.remove == nil {
		s.remove = osRemove
	}
	return s, nil
}

// horizonFor reads ⚙ retention.compaction_horizon for one owner, LIVE at
// evaluation time (so an operator change applies on the next pass without a
// restart) and per-user (so two owners genuinely get two boundaries).
func (s *Store) horizonFor(userID string) (int64, error) {
	months, err := s.settings.IntFor(keyCompactionHorizon, userID)
	if err != nil {
		return 0, fmt.Errorf("retention: read ⚙ %s for %q: %w", keyCompactionHorizon, userID, err)
	}
	return months, nil
}
