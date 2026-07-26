package watchlist

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// drift.go — the drift.finding emitter (Spec S14.6 ¶2).
//
// Findings are PLATFORM-SCOPE (UserID "platform", no RunID — a watch hit has no
// run) at schema_version 1, and their payload satisfies the S14.2 Drift & canary
// contract minimum verbatim: source, lane(s), change class, severity, one-line
// summary, incident fingerprint.
//
// Cards DERIVE from the log. There is no card table and no bare `asks` row
// (asks.run_id is NOT NULL): internal/api projects the open drift cards out of
// these rows with fingerprint-per-incident-window dedup — one card per storm
// (S14.6 ¶2; S10.5 once-per-storm logging).

// Hit is one detected change, before classification. Everything on it is
// platform-derived: the row supplies lane and source, so a hit's own content
// never decides its lane or severity.
type Hit struct {
	RowID string
	Kind  string
	// Source is what changed — the url|feed of the row, or the row id for a
	// sourceless obligation.
	Source string
	// Lane is the watch row's lane|candidate (server-side truth).
	Lane string
	// Title is the one-line subject (a feed entry's title, a diff headline).
	Title string
	// Detail is the observed change, bounded before it reaches the payload.
	Detail string
	// Subject, when set, names the concrete asset the change is about — a MODEL
	// ID for model-list drift. It is the only thing that can carry a finding
	// into the S14.8 revalidation runbook (see the OQ4 disposition below).
	Subject string
	// Class, when set, marks a PLATFORM-AUTHORED finding (the decay meta-watch,
	// the price-refresh proposal): the second pass is skipped because the
	// platform, not a model, knows what happened.
	Class string
	// Summary overrides the classifier summary on a platform-authored finding.
	Summary string
	// DecayStage / MigrateBias ride the card so a decayed source announces
	// itself with its P-T11-3 posture attached.
	DecayStage  int
	MigrateBias bool
	// Proposal, when set, carries the R14 price-refresh proposal.
	Proposal *PriceProposal
}

// Revalidation records what the S14.8 revalidation edge did — always, including
// when it did NOTHING. Coordinator disposition OQ4(a): the runbook fires ONLY
// for change class `models` carrying a model-id subject, because the only wired
// hook is worker.Store.FlagByModel, which matches a MODEL ID against
// validation_records.model and the template's Profile.ModelPin. Every other
// class raises its card and records on the card that no revalidation was
// triggered — the residual gap is made VISIBLE, never silently dropped, and no
// flag is ever faked.
type Revalidation struct {
	Triggered bool   `json:"triggered"`
	Subject   string `json:"subject,omitempty"`
	Reason    string `json:"reason"`
	// Flagged lists the template ids the runbook flagged (empty is a legitimate
	// answer — the v0 worker set may be empty).
	Flagged []string `json:"flagged,omitempty"`
	// Error records a runbook failure. A failed revalidation never suppresses
	// the card.
	Error string `json:"error,omitempty"`
}

// FindingPayload is the drift.finding payload. The first six fields are the
// S14.2 FamilyDriftCanary contract minimum verbatim.
type FindingPayload struct {
	Source      string   `json:"source"`
	Lanes       []string `json:"lanes"`
	ChangeClass string   `json:"change_class"`
	Severity    string   `json:"severity"`
	Summary     string   `json:"summary"`
	Fingerprint string   `json:"fingerprint"`

	RowID string `json:"row_id"`
	Kind  string `json:"kind"`
	// Classified is false when the second pass could not produce a verdict; the
	// card then reads "unclassified — operator reads it".
	Classified     bool   `json:"classified"`
	ClassifierNote string `json:"classifier_note,omitempty"`
	Detail         string `json:"detail,omitempty"`
	DecayStage     int    `json:"decay_stage,omitempty"`
	// MigrateBias surfaces the P-T11-3 standing bias to migrate this scrape
	// target to a feed/API the moment one exists.
	MigrateBias  bool           `json:"migrate_bias,omitempty"`
	Revalidation Revalidation   `json:"revalidation"`
	Proposal     *PriceProposal `json:"price_proposal,omitempty"`
}

// Revalidate is the S14.8 revalidation-runbook seam, composed at the shell root
// (internal/watchlist never imports internal/evals — the FlagByModelFunc /
// eval-seams precedent, CONVENTIONS §28/§33). The shell satisfies it with
// evals.Runbook.Flag over an evals.Trigger{Kind: TriggerDrift}. Nil ⇒ the edge
// is honestly unwired and every finding records that.
type Revalidate func(ctx context.Context, modelID, reason string) ([]string, error)

// SecondPass is the S14.6 ¶2 classification seam. *Classifier is the production
// implementation; the interface exists so the emitter's own derivations —
// severity routing, the lane fallback, the OQ4 gate — are drivable end to end
// without a local stack. A nil seam means every hit is unclassified, which is a
// first-class honest state, not a test-only one.
type SecondPass interface {
	Classify(ctx context.Context, h Hit) Classification
}

// Emitter mints drift.finding events.
type Emitter struct {
	db  *storage.DB
	log *eventlog.Log
	// Classifier runs the second pass; nil ⇒ every hit is unclassified.
	Classifier SecondPass
	// Revalidate is the S14.8 edge; nil ⇒ unwired, recorded on every card.
	Revalidate Revalidate
	Logger     *slog.Logger
	// Now is the test clock seam; nil ⇒ time.Now.
	Now func() time.Time
}

// NewEmitter builds the finding emitter.
func NewEmitter(db *storage.DB, log *eventlog.Log) *Emitter {
	return &Emitter{db: db, log: log}
}

func (e *Emitter) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

func (e *Emitter) logger() *slog.Logger {
	if e.Logger != nil {
		return e.Logger
	}
	return slog.Default()
}

// Fingerprint is the incident fingerprint: a stable derivation over
// {source, lane, change class} (R17). Repeats inside one incident window fold
// into the open card rather than raising a new one.
func Fingerprint(source, lane, class string) string {
	return shortHash(source, lane, class)
}

// Emit classifies a hit (unless it is platform-authored), computes its severity
// and fingerprint, fires the S14.8 revalidation edge per the OQ4 disposition,
// and appends one drift.finding.
//
// It returns the payload and whether a finding was actually appended. A hit the
// second pass classifies `none` is NOT a finding: S14.6 ¶2 says RELEVANT hits
// become drift cards, and the second pass answering "irrelevant" IS that filter.
// The judgment is logged so the trace of having looked is not lost, but it never
// reaches the log or the inbox.
func (e *Emitter) Emit(ctx context.Context, h Hit) (FindingPayload, bool, error) {
	var cl Classification
	if h.Class != "" {
		// Platform-authored: the platform knows what happened, so no model is
		// consulted and none is faked.
		cl = Classification{
			Lanes:      laneList(h.Lane, nil),
			Class:      h.Class,
			Summary:    oneLine(firstNonEmpty(h.Summary, fallbackSummary(h))),
			Classified: true,
			Note:       "platform-authored finding (no model verdict was needed)",
		}
	} else if e.Classifier == nil {
		cl = Classification{
			Lanes: laneList(h.Lane, nil), Class: ClassUnclear, Summary: fallbackSummary(h),
			Note: "no second pass is wired — the hit is recorded unclassified for the operator to read",
		}
	} else {
		cl = e.Classifier.Classify(ctx, h)
	}

	if cl.Class == ClassNone {
		e.logger().Info("watchlist: hit classified irrelevant — no card raised (S14.6 ¶2: RELEVANT hits become drift cards)",
			"row", h.RowID, "source", h.Source, "summary", cl.Summary)
		return FindingPayload{ChangeClass: ClassNone, Summary: cl.Summary, RowID: h.RowID}, false, nil
	}

	// Severity derives SERVER-SIDE from the change class and the lane set
	// (S14.4's two alert-routing classes) — never from the hit body.
	//
	// The watch ROW's lane is authoritative when it has one. A row with NO lane
	// is the case that matters: report-02 calls the cross-provider repo canaries
	// the strongest signal precisely because they are not lane-scoped, and
	// deriving from an empty lane would route a confirmed active-lane change to
	// the daily digest while the card itself displayed that lane — the card and
	// its severity disagreeing. So an empty row lane falls back to the lanes the
	// second pass identified.
	laneSet := []string{h.Lane}
	if h.Lane == "" {
		laneSet = cl.Lanes
	}
	severity := severityFor(cl.Class, laneSet)

	payload := FindingPayload{
		Source:         h.Source,
		Lanes:          cl.Lanes,
		ChangeClass:    cl.Class,
		Severity:       severity,
		Summary:        cl.Summary,
		Fingerprint:    Fingerprint(h.Source, h.Lane, cl.Class),
		RowID:          h.RowID,
		Kind:           h.Kind,
		Classified:     cl.Classified,
		ClassifierNote: cl.Note,
		Detail:         clip(h.Detail, detailCap),
		DecayStage:     h.DecayStage,
		MigrateBias:    h.MigrateBias,
		Proposal:       h.Proposal,
	}
	payload.Revalidation = e.revalidate(ctx, cl.Class, h.Subject, payload.Summary)

	body, err := json.Marshal(payload)
	if err != nil {
		return payload, false, fmt.Errorf("watchlist: marshal drift finding: %w", err)
	}
	if _, err := e.log.Append(ctx, eventlog.Append{
		UserID:        platformUser,
		Type:          EventDriftFinding,
		SchemaVersion: eventSchemaVersion,
		Payload:       body,
		Time:          e.now(),
	}); err != nil {
		return payload, false, fmt.Errorf("watchlist: append drift finding: %w", err)
	}
	return payload, true, nil
}

// revalidate implements coordinator disposition OQ4(a) exactly. The gate is
// structural: a non-`models` class, or a `models` class with no model-id
// subject, does not reach the hook at all — and says so on the card.
func (e *Emitter) revalidate(ctx context.Context, class, subject, reason string) Revalidation {
	if class != ClassModels {
		return Revalidation{Reason: fmt.Sprintf(
			"no revalidation triggered: the S14.8 runbook's only wired hook flags by MODEL ID, and change class %q names no model (OQ4(a) — the residual gap is recorded, never faked)", class)}
	}
	if subject == "" {
		return Revalidation{Reason: "no revalidation triggered: change class models carried no model-id subject, and a non-model subject would flag nothing (OQ4(a))"}
	}
	if e.Revalidate == nil {
		return Revalidation{Subject: subject, Reason: "no revalidation triggered: the S14.8 runbook seam is not wired at this composition root"}
	}
	flagged, err := e.Revalidate(ctx, subject, "S14.8 revalidation trigger drift_finding: "+reason)
	if err != nil {
		e.logger().Warn("watchlist: S14.8 revalidation runbook failed (the drift card still stands)", "subject", subject, "err", err)
		return Revalidation{Subject: subject, Reason: "revalidation attempted and failed; the drift card stands", Error: err.Error()}
	}
	return Revalidation{
		Triggered: true, Subject: subject, Flagged: flagged,
		Reason: "S14.8 revalidation runbook fired for a models-class drift finding naming a model id (OQ4(a))",
	}
}

// LastObservedChangeAt returns the time of the most recent drift.finding for a
// watch row that recorded an OBSERVED CHANGE, derived from the log — no side
// store. ok is false when the row has never produced one.
//
// Staleness findings are excluded by construction. They are the row reporting
// that NOTHING changed, so counting one as an observation would reset the very
// clock that produced it: the row would report stale, restart its horizon, and
// re-report one horizon later forever, instead of staying stale until upstream
// actually moves.
func (e *Emitter) LastObservedChangeAt(ctx context.Context, rowID string) (time.Time, bool, error) {
	var ts string
	err := e.db.QueryRowContext(ctx,
		`SELECT ts FROM run_events
		  WHERE type = ? AND json_extract(payload, '$.row_id') = ?
		    AND COALESCE(json_extract(payload, '$.price_proposal.stale'), '') = ''
		  ORDER BY event_seq DESC LIMIT 1`, EventDriftFinding, rowID).Scan(&ts)
	if err == sql.ErrNoRows {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("watchlist: read last observed change for %q: %w", rowID, err)
	}
	t, perr := time.Parse(time.RFC3339Nano, ts)
	if perr != nil {
		return time.Time{}, false, nil
	}
	return t, true, nil
}
