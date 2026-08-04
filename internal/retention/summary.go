package retention

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/redact"
)

// summary.go — the S14.9 ¶1 run summary.
//
//	"At run end (never later): the run summary — a compact structured story
//	 (objective, stages, tool-call counts, verdicts, decisions, receipts,
//	 final state) from deterministic aggregation + the local tier. Incremental
//	 per-run is the documented safe shape; bulk summarization over months of
//	 trace is the documented failure shape."
//
// Two legs, in this order and never the other:
//
//  1. The DETERMINISTIC AGGREGATE is computed from run_events + checkpoints +
//     receipts inside the run's terminal transaction, written to run_summaries,
//     and appended as run.summary_written in that same transaction (S02.3 /
//     CONVENTIONS §6, §8). This aggregate IS the record.
//  2. The LOCAL TIER enriches it afterwards, out of band, through the Narrator
//     seam. It never gates: with no stack the summary stands aggregate-only and
//     says so.
//
// There is NO batch or backfill summarizer here. EnrichPending works over rows
// whose OWN run has already ended and whose narrative is still pending — one
// run at a time, keyed on that run — never over a window of historical trace.

// Stage is one step of the run's progression.
//
// D8: `Name` and `From` are S02.3 FSM STATE VALUES — a closed set the runs
// CHECK constraint enforces — and an unrecognized value is dropped rather than
// carried. The transition's free-text `reason` is deliberately NOT here: the
// summary crosses the 11.3 export boundary (it is keep-forever), so it must not
// smuggle unbounded trace-derived prose past a boundary the trace itself does
// not cross. The reason stays in the run.state_changed payload, which is trace
// and is stripped past the horizon exactly as S14.9's closed keep-forever list
// prescribes.
type Stage struct {
	EventSeq int64  `json:"event_seq"`
	Name     string `json:"name"`
	From     string `json:"from,omitempty"`
	TS       string `json:"ts"`
}

// ToolCalls is the S14.9 "tool-call COUNTS" leg — counts, and nothing else.
//
// D8: tool NAMES are deliberately absent. S14.9 ¶1 asks for tool-call counts,
// and a name is trace-derived free text that would ride the summary across the
// 11.3 export boundary the trace itself never crosses. Distinct preserves the
// shape of the run's tool use without naming anything.
type ToolCalls struct {
	Total int64 `json:"total"`
	// Distinct is how many distinct tool names appeared — a count, not a list.
	Distinct int64 `json:"distinct"`
	// Unnamed counts tool.completed rows whose payload carried no recognizable
	// tool name — counted honestly rather than bucketed under a guess.
	Unnamed int64 `json:"unnamed,omitempty"`
}

// VerdictRef is one S2.3 verdict record, by reference.
type VerdictRef struct {
	EventSeq int64  `json:"event_seq"`
	Round    int    `json:"round,omitempty"`
	Verdict  string `json:"verdict,omitempty"`
	RubricID string `json:"rubric_id,omitempty"`
}

// DecisionRef is one S2.4 human decision, by reference.
type DecisionRef struct {
	EventSeq int64  `json:"event_seq"`
	Type     string `json:"type"`
	Decision string `json:"decision,omitempty"`
	Actor    string `json:"actor,omitempty"`
}

// ReceiptsRef is the S14.9 "receipts" leg. Receipts are materialized from
// checkpoint usage rows by S10; at the instant a run ends they may not exist
// yet, so the paid-call count (checkpoints) is reported beside them and the
// absence is visible rather than rendered as zero cost.
type ReceiptsRef struct {
	ReceiptIDs  []int64 `json:"receipt_ids,omitempty"`
	Count       int64   `json:"count"`
	Checkpoints int64   `json:"checkpoints"`
	// Note records the honest reading when there are paid calls but no
	// materialized receipt yet (S10 materializes per run-end, S02.2).
	Note string `json:"note,omitempty"`
}

// Aggregate is the deterministic story: every S14.9 ¶1 field, computed by
// aggregation over tables that exist. It is the summary; the narrative is an
// addition to it, never a replacement for it.
type Aggregate struct {
	RunID  string `json:"run_id"`
	Owner  string `json:"owner"`
	TaskID string `json:"task_id,omitempty"`

	Objective string `json:"objective"`
	// ObjectiveSource names where the objective came from, so an empty
	// objective reads as a genuine absence and not as a blank field.
	ObjectiveSource string `json:"objective_source"`

	Stages []Stage `json:"stages"`
	// StagesSource records the producer the stage list was derived from. At v0
	// the S15/9.1 stage.started/finished producers do not exist (declare-only,
	// B6), so the run's own S02.3 lifecycle transitions ARE its stages; when
	// those producers land the list picks them up without a schema change.
	StagesSource string `json:"stages_source"`

	ToolCalls  ToolCalls     `json:"tool_calls"`
	Verdicts   []VerdictRef  `json:"verdicts"`
	Decisions  []DecisionRef `json:"decisions"`
	Receipts   ReceiptsRef   `json:"receipts"`
	FinalState string        `json:"final_state"`

	// The trace window the aggregate was computed over.
	FirstEventSeq int64 `json:"first_event_seq"`
	LastEventSeq  int64 `json:"last_event_seq"`
	EventCount    int64 `json:"event_count"`
	// TypeCounts is the whole trace by type — the compact shape of what the
	// compaction pass will later strip.
	TypeCounts map[string]int64 `json:"type_counts,omitempty"`
}

// Summary is one stored run_summaries row.
type Summary struct {
	RunID           string
	UserID          string
	EventSeq        int64
	FinalState      string
	Objective       string
	Aggregate       Aggregate
	InputsDigest    string
	Narrative       string
	NarrativeStatus string
	NarrativeNote   string
	NarrativeModel  string
	WrittenTS       time.Time
}

// Narrative statuses (migration 0015 CHECK).
const (
	NarrativePending = "pending"
	NarrativeWritten = "written"
	NarrativeAbsent  = "absent"
	NarrativeFailed  = "failed"
)

// platformOwner is the platform's own user_id (value-identical to
// run.ActorPlatform / shell.PlatformUserID; this package imports neither).
const platformOwner = "platform"

// ErrNoSummary is returned when a run has no summary row.
var ErrNoSummary = errors.New("retention: no run summary")

// WriteAtRunEndTx writes the run's summary INSIDE the caller's terminal
// transition transaction: the deterministic aggregate lands in run_summaries
// and run.summary_written is appended, both committing with the state change or
// not at all. Composed at the shell root onto run.Store's terminal hook, so
// every terminal transition in the tree is covered by one seam rather than ten
// call sites.
//
// It returns written=false, with its reason, for the two cases where a summary
// would be noise or worse:
//
//   - a PLATFORM-scope run. The platform's own short-lived machinery runs (the
//     advisory metering vehicle a local duty call rides, the dead-man canary)
//     have no objective, no task and no receipts. Summarizing them would also
//     be a FEEDBACK LOOP: the narrator's own call rides an advisory run, whose
//     completion would mint another summary, whose narration would ride another
//     advisory run, without end.
//   - a run that already has a summary. Terminal is terminal, and the record is
//     written once (S14.9 "at run end, never later").
func (s *Store) WriteAtRunEndTx(ctx context.Context, tx *sql.Tx, runID string) (written bool, reason string, err error) {
	var owner, finalState, taskID string
	var generation int64
	row := tx.QueryRowContext(ctx,
		`SELECT user_id, state, COALESCE(task_id, ''), generation FROM runs WHERE run_id = ?`, runID)
	switch err := row.Scan(&owner, &finalState, &taskID, &generation); {
	case errors.Is(err, sql.ErrNoRows):
		return false, "", fmt.Errorf("retention: summary for unknown run %q", runID)
	case err != nil:
		return false, "", fmt.Errorf("retention: read run %q: %w", runID, err)
	}
	if owner == platformOwner {
		return false, "platform-scope run: platform machinery carries no run story, and summarizing the narrator's own advisory run would not terminate", nil
	}

	var exists int
	if err := tx.QueryRowContext(ctx,
		`SELECT count(*) FROM run_summaries WHERE run_id = ?`, runID).Scan(&exists); err != nil {
		return false, "", fmt.Errorf("retention: check existing summary %q: %w", runID, err)
	}
	if exists > 0 {
		return false, "a summary is already written for this run (S14.9: at run end, never later)", nil
	}

	agg, digest, err := s.aggregateTx(ctx, tx, runID, owner, taskID, finalState)
	if err != nil {
		return false, "", err
	}
	aggJSON, err := json.Marshal(agg)
	if err != nil {
		return false, "", fmt.Errorf("retention: marshal aggregate for %q: %w", runID, err)
	}

	status := NarrativePending
	note := ""
	if s.narrator == nil {
		status = NarrativeAbsent
		note = "no local-tier narrator is wired: this summary is the deterministic aggregate alone (S14.9 ¶1)"
	}

	payload, err := json.Marshal(struct {
		SummaryRef    string `json:"summary_ref"`
		InputsDigest  string `json:"inputs_digest"`
		FinalState    string `json:"final_state"`
		AggregateOnly bool   `json:"aggregate_only"`
	}{
		SummaryRef:    SummaryRef(runID),
		InputsDigest:  digest,
		FinalState:    finalState,
		AggregateOnly: status == NarrativeAbsent,
	})
	if err != nil {
		return false, "", fmt.Errorf("retention: marshal summary payload for %q: %w", runID, err)
	}
	seq, err := s.log.AppendTx(ctx, tx, eventlog.Append{
		RunID:         runID,
		Generation:    generation,
		UserID:        owner,
		Type:          EventSummaryWritten,
		SchemaVersion: eventSchemaVersion,
		Payload:       payload,
	})
	if err != nil {
		return false, "", err
	}

	now := s.now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO run_summaries (run_id, user_id, event_seq, final_state, objective,
		                            aggregate_json, inputs_digest, narrative_status,
		                            narrative_note, written_ts, updated_ts)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		runID, owner, seq, finalState, agg.Objective, string(aggJSON), digest, status, note, now, now); err != nil {
		return false, "", fmt.Errorf("retention: write summary %q: %w", runID, err)
	}
	return true, "", nil
}

// SummaryRef is the S14.2 contract-minimum "summary artifact ref" — a stable
// reference into the durable set, resolvable by Get. The summary is a
// control-plane row, not a file: there is no artifact to dangle.
func SummaryRef(runID string) string { return "run_summary:" + runID }

// aggregateTx computes the deterministic story and its generation-inputs
// digest from the run's own trace. Every field of S14.9's list is derived from
// tables that exist; nothing is estimated.
func (s *Store) aggregateTx(ctx context.Context, tx *sql.Tx, runID, owner, taskID, finalState string) (Aggregate, string, error) {
	agg := Aggregate{
		RunID:           runID,
		Owner:           owner,
		TaskID:          taskID,
		FinalState:      finalState,
		ObjectiveSource: "absent",
		StagesSource:    "run.state_changed (S02.3 lifecycle; the S15/9.1 stage.started/finished producers are B6's)",
		TypeCounts:      map[string]int64{},
		Stages:          []Stage{},
		Verdicts:        []VerdictRef{},
		Decisions:       []DecisionRef{},
	}

	if taskID != "" {
		var title string
		switch err := tx.QueryRowContext(ctx,
			`SELECT title FROM tasks WHERE task_id = ?`, taskID).Scan(&title); {
		case err == nil:
			agg.Objective, agg.ObjectiveSource = safeTitle(title), "tasks.title"
		case errors.Is(err, sql.ErrNoRows):
			// The FK is nullable and a run can outlive nothing here; an absent
			// task row is recorded, never invented.
			agg.ObjectiveSource = "absent (task row not found)"
		default:
			return Aggregate{}, "", fmt.Errorf("retention: read task %q: %w", taskID, err)
		}
	}

	// One ordered pass over the run's trace. The digest is folded as we go, so
	// it is a digest of exactly the rows the aggregate saw.
	h := sha256.New()
	fmt.Fprintf(h, "run=%s owner=%s task=%s final=%s\n", runID, owner, taskID, finalState)

	rows, err := tx.QueryContext(ctx,
		`SELECT event_seq, type, payload, ts FROM run_events WHERE run_id = ? ORDER BY event_seq`, runID)
	if err != nil {
		return Aggregate{}, "", fmt.Errorf("retention: read trace of %q: %w", runID, err)
	}
	byTool := map[string]int64{}
	for rows.Next() {
		var (
			seq            int64
			typ, pay, ts   string
			payloadDecoded map[string]any
		)
		if err := rows.Scan(&seq, &typ, &pay, &ts); err != nil {
			rows.Close()
			return Aggregate{}, "", err
		}
		fmt.Fprintf(h, "%d\t%s\n", seq, typ)
		if agg.FirstEventSeq == 0 {
			agg.FirstEventSeq = seq
		}
		agg.LastEventSeq = seq
		agg.EventCount++
		agg.TypeCounts[typ]++
		// Forward-tolerant: an unparseable payload is counted by type and
		// otherwise skipped, never fatal (S03.1 / S14.2 rule 3).
		_ = json.Unmarshal([]byte(pay), &payloadDecoded)

		switch typ {
		case "run.state_changed", "run.state":
			// Enumerated only: an unrecognized state value is DROPPED, so a
			// malformed or hostile payload cannot inject free text here.
			to, from := fsmState(str(payloadDecoded, "to")), fsmState(str(payloadDecoded, "from"))
			if to != "" {
				agg.Stages = append(agg.Stages, Stage{EventSeq: seq, Name: to, From: from, TS: ts})
			}
		case "stage.started", "stage.finished":
			// Forward-tolerant: when B6 mints these, they are the better source
			// and the list carries them alongside the lifecycle transitions. A
			// stage id is an identifier, not prose — redacted and bounded, never
			// carried raw across the export boundary.
			agg.Stages = append(agg.Stages, Stage{
				EventSeq: seq,
				Name:     safeLabel(firstStr(payloadDecoded, "stage", "stage_id", "name")),
				TS:       ts,
			})
		case "tool.completed", "engine.tool_result":
			agg.ToolCalls.Total++
			// The same tolerant key set internal/watchdog reads (tier0.go) — the
			// engine payload names it differently across lanes.
			if name := firstStr(payloadDecoded, "tool", "name", "tool_name"); name != "" {
				byTool[name]++
			} else {
				agg.ToolCalls.Unnamed++
			}
		case "verdict.recorded", "verify.round":
			agg.Verdicts = append(agg.Verdicts, VerdictRef{
				EventSeq: seq,
				Round:    num(payloadDecoded, "round"),
				Verdict:  safeLabel(str(payloadDecoded, "verdict")),
				RubricID: safeLabel(str(payloadDecoded, "rubric_id")),
			})
		case "decision.recorded", "intake.delta_decision":
			agg.Decisions = append(agg.Decisions, DecisionRef{
				EventSeq: seq,
				Type:     typ,
				Decision: safeLabel(firstStr(payloadDecoded, "decision", "choice")),
				Actor:    safeLabel(str(payloadDecoded, "actor")),
			})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Aggregate{}, "", err
	}
	rows.Close()
	agg.ToolCalls.Distinct = int64(len(byTool))

	if err := tx.QueryRowContext(ctx,
		`SELECT count(*) FROM checkpoints WHERE run_id = ?`, runID).Scan(&agg.Receipts.Checkpoints); err != nil {
		return Aggregate{}, "", fmt.Errorf("retention: count checkpoints of %q: %w", runID, err)
	}
	rrows, err := tx.QueryContext(ctx,
		`SELECT receipt_id FROM receipts WHERE run_id = ? ORDER BY receipt_id`, runID)
	if err != nil {
		return Aggregate{}, "", fmt.Errorf("retention: read receipts of %q: %w", runID, err)
	}
	for rrows.Next() {
		var id int64
		if err := rrows.Scan(&id); err != nil {
			rrows.Close()
			return Aggregate{}, "", err
		}
		agg.Receipts.ReceiptIDs = append(agg.Receipts.ReceiptIDs, id)
	}
	if err := rrows.Err(); err != nil {
		rrows.Close()
		return Aggregate{}, "", err
	}
	rrows.Close()
	agg.Receipts.Count = int64(len(agg.Receipts.ReceiptIDs))
	if agg.Receipts.Count == 0 && agg.Receipts.Checkpoints > 0 {
		agg.Receipts.Note = "paid calls are checkpointed but no receipt is materialized yet (S10 materializes per run-end); the checkpoint count is the honest figure here"
	}
	fmt.Fprintf(h, "checkpoints=%d receipts=%v\n", agg.Receipts.Checkpoints, agg.Receipts.ReceiptIDs)

	return agg, "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// Get returns one run's summary.
func (s *Store) Get(ctx context.Context, runID string) (Summary, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT run_id, user_id, event_seq, final_state, objective, aggregate_json,
		        inputs_digest, narrative, narrative_status, narrative_note,
		        narrative_model, written_ts
		   FROM run_summaries WHERE run_id = ?`, runID)
	var (
		sum     Summary
		aggJSON string
		written string
	)
	err := row.Scan(&sum.RunID, &sum.UserID, &sum.EventSeq, &sum.FinalState, &sum.Objective,
		&aggJSON, &sum.InputsDigest, &sum.Narrative, &sum.NarrativeStatus,
		&sum.NarrativeNote, &sum.NarrativeModel, &written)
	if errors.Is(err, sql.ErrNoRows) {
		return Summary{}, fmt.Errorf("%w: %q", ErrNoSummary, runID)
	}
	if err != nil {
		return Summary{}, fmt.Errorf("retention: read summary %q: %w", runID, err)
	}
	if err := json.Unmarshal([]byte(aggJSON), &sum.Aggregate); err != nil {
		return Summary{}, fmt.Errorf("retention: decode aggregate of %q: %w", runID, err)
	}
	if sum.WrittenTS, err = time.Parse(time.RFC3339Nano, written); err != nil {
		return Summary{}, fmt.Errorf("retention: parse written_ts of %q: %w", runID, err)
	}
	return sum, nil
}

// EnrichPending runs the local-tier narrative pass over up to limit summaries
// whose narrative is still pending — INCREMENTAL PER-RUN, one ended run at a
// time (S14.9 ¶1: bulk summarization over months of trace is the documented
// failure shape, so no window, no date range, and no re-summarization ever
// enters here). It returns how many rows it moved off `pending`.
//
// It never returns an error for a narrator failure: a failure is RECORDED on
// the row (status `failed` or `absent`, with the reason) and the aggregate
// stands. The only errors are storage defects.
func (s *Store) EnrichPending(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 8
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT run_id FROM run_summaries WHERE narrative_status = ? ORDER BY event_seq LIMIT ?`,
		NarrativePending, limit)
	if err != nil {
		return 0, fmt.Errorf("retention: list pending summaries: %w", err)
	}
	var pending []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		pending = append(pending, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	moved := 0
	for _, runID := range pending {
		sum, err := s.Get(ctx, runID)
		if err != nil {
			return moved, err
		}
		status, text, model, note := NarrativeAbsent, "", "", "no local-tier narrator is wired: this summary is the deterministic aggregate alone (S14.9 ¶1)"
		if s.narrator != nil {
			nar, nerr := s.narrator(ctx, runID, sum.Aggregate)
			switch {
			case nerr == nil && nar.Text != "":
				status, text, model, note = NarrativeWritten, nar.Text, nar.Model, ""
			case nerr == nil:
				status, note = NarrativeAbsent, "the narrator returned no text; the summary stands on its deterministic aggregate"
			case isStackAbsent(nerr):
				status, note = NarrativeAbsent, "the local stack is absent: "+nerr.Error()
			default:
				status, note = NarrativeFailed, nerr.Error()
			}
		}
		if err := s.setNarrative(ctx, runID, status, text, model, note); err != nil {
			return moved, err
		}
		moved++
	}
	return moved, nil
}

func (s *Store) setNarrative(ctx context.Context, runID, status, text, model, note string) error {
	now := s.now().UTC().Format(time.RFC3339Nano)
	return s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE run_summaries
			    SET narrative = ?, narrative_status = ?, narrative_model = ?,
			        narrative_note = ?, updated_ts = ?
			  WHERE run_id = ?`,
			text, status, model, note, now, runID)
		if err != nil {
			return fmt.Errorf("retention: record narrative for %q: %w", runID, err)
		}
		return nil
	})
}

// StackAbsentMarker is the text of internal/local's ErrStackAbsent
// ("local: serving stack is not configured"). It is matched here rather than
// compared with errors.Is because the import wall keeps this package a leaf and
// the narrator is a func seam, not a typed dependency. The classification only
// chooses between two HONEST labels — an absent stack and a failure — so a miss
// degrades to `failed` carrying the real error text, never to a fabricated
// narrative. A local_test in internal/shell pins the two strings together.
const StackAbsentMarker = "serving stack is not configured"

func isStackAbsent(err error) bool {
	return err != nil && strings.Contains(err.Error(), StackAbsentMarker)
}

// ── D8: what may cross the 11.3 export boundary ─────────────────────────────
//
// The run summary is keep-forever, so it LEAVES THE HOST in every snapshot
// (S14.9 ¶3 / S13.10) while the trace it was computed from does not. Anything
// the aggregate carries out of a payload is therefore a hole in that boundary
// unless it is bounded and scrubbed. Three rules, applied at the one place the
// aggregate is built:
//
//   - a state is ENUMERATED (fsmState): the closed S02.3 set or nothing;
//   - a label is REDACTED and BOUNDED (safeLabel): identifiers and short
//     enumerated-ish fields, never prose;
//   - the objective is REDACTED and BOUNDED (safeTitle) at a title's length —
//     it is the operator's own task title, which already travels in the tasks
//     table, but it is scrubbed here too rather than trusted twice.
//
// Free-text trace fields (a transition's reason, a tool name) are not carried
// at all. See Stage and ToolCalls.

// fsmStates is the closed S02.3 stored-state set (the runs CHECK constraint's
// vocabulary, held by value here — internal/retention cannot import
// internal/run, and a state string is data either way).
var fsmStates = map[string]bool{
	"new": true, "queued": true, "claimed": true, "running": true,
	"parked": true, "draining": true, "completed": true, "crashed": true,
	"finalized": true, "tombstoned": true, "died-at-gate": true,
}

// fsmState passes through a recognized S02.3 state and drops anything else.
func fsmState(s string) string {
	if fsmStates[s] {
		return s
	}
	return ""
}

// labelCap bounds an identifier-shaped field carried into the summary. A
// STRUCTURAL CONSTANT: these are ids, verdict words and actor names, not prose,
// and 96 bytes is generous for every one of them in the tree. Its reason is the
// boundary, not aesthetics — an unbounded field is an unbounded leak.
const labelCap = 96

// titleCap bounds the objective. A task title is one line by construction
// (internal/intake), so 200 bytes carries every honest one whole.
const titleCap = 200

func safeLabel(s string) string { return boundRunes(redact.Redact(s), labelCap) }
func safeTitle(s string) string { return boundRunes(redact.Redact(s), titleCap) }

// boundRunes truncates on a rune boundary and marks that it truncated, so a cut
// field never reads as a complete one.
func boundRunes(s string, cap int) string {
	r := []rune(s)
	if len(r) <= cap {
		return s
	}
	return string(r[:cap]) + "…"
}

// ── small payload readers (forward-tolerant, never fatal) ───────────────────

func str(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func firstStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v := str(m, k); v != "" {
			return v
		}
	}
	return ""
}

func num(m map[string]any, key string) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return 0
}
