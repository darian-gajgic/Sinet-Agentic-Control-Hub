package ledger

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// EventLedgerUpdate is the ledger revision event type. The name is fixed by
// Spec S02.2: revisions are persisted as run_events of type `ledger_update`
// carrying the full content — the ledger is small by design (the
// restorable-reference rule keeps bulk out; ⚙ state.event_payload_cap is
// the structural bound, enforced by the event log).
const EventLedgerUpdate = "ledger_update"

// ledgerUpdateSchemaVersion versions the ledger_update payload.
const ledgerUpdateSchemaVersion = 1

// Session verb names (Spec S05.1 [coordinator-draft naming]) and the
// control-plane change names implied by the S05.1 writers column (the spec
// names only the session verbs; the ledger.* family is extended to the
// control-plane writes for one auditable vocabulary).
const (
	VerbDecide   = "ledger.decide"
	VerbState    = "ledger.state"
	VerbArtifact = "ledger.artifact"
	VerbNote     = "ledger.note"

	VerbObjective   = "ledger.objective_ac"
	VerbConstraints = "ledger.constraints"
	VerbPlanVersion = "ledger.plan_version"
	VerbVerify      = "ledger.verify"
)

// Change describes the accepted write inside a ledger_update payload. The
// event row itself is attributed to the run's owner (15.6, CONVENTIONS §8);
// the acting principal rides here.
type Change struct {
	Verb  string `json:"verb"`
	Actor string `json:"actor"`
	Stage string `json:"stage,omitempty"`
}

// updatePayload is the self-contained D7 payload of one ledger_update
// event: the change descriptor, the revision content hash, and the FULL
// document at this revision (Spec S02.2) — ledger state reconstructs from
// the event stream alone.
type updatePayload struct {
	Change      Change   `json:"change"`
	ContentHash string   `json:"content_hash"`
	Ledger      Document `json:"ledger"`
}

// RevisionRef is the S02.4(c) checkpoint ledger-revision block: the
// ledger_version the checkpoint was built from (Spec S05.1 header rule)
// plus the revision content hash. The content itself is in run_events, so
// the reference is durable against workspace loss (D7 self-containment).
type RevisionRef struct {
	LedgerVersion int64  `json:"ledger_version"`
	SHA256        string `json:"sha256"`
}

// ParseRevisionRef decodes a checkpoint ledger_revision block. An empty
// string is a run without a ledger (ok=false).
func ParseRevisionRef(s string) (RevisionRef, bool, error) {
	if s == "" {
		return RevisionRef{}, false, nil
	}
	var ref RevisionRef
	if err := json.Unmarshal([]byte(s), &ref); err != nil {
		return RevisionRef{}, false, fmt.Errorf("ledger: parse revision ref: %w", err)
	}
	return ref, true, nil
}

// Store reads and writes the Task Context Ledger as ledger_update events
// over the platform event log. All writes follow CONVENTIONS §6: one write
// transaction, AppendTx composition, generation fencing at the event log,
// owner attribution on the row.
type Store struct {
	db  *storage.DB
	log *eventlog.Log

	// Now is the clock seam (tests). Nil = time.Now. Timestamps are
	// recorded, never ordering (P-T07-4).
	Now func() time.Time
}

// NewStore returns a Store over db appending through log.
func NewStore(db *storage.DB, log *eventlog.Log) *Store {
	return &Store{db: db, log: log}
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Store) nowRFC3339() string {
	return s.now().UTC().Format(time.RFC3339Nano)
}

// contentHash is the revision hash: sha256 hex over the canonical (compact
// struct-order) JSON marshaling of the document.
func contentHash(doc Document) (string, []byte, error) {
	raw, err := json.Marshal(doc)
	if err != nil {
		return "", nil, fmt.Errorf("ledger: marshal document: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), raw, nil
}

// runRow is the platform-owned run context of a write.
type runRow struct {
	userID     string
	taskID     string
	generation int64
}

func readRun(ctx context.Context, q queryer, runID string) (runRow, error) {
	var (
		r      runRow
		taskID sql.NullString
	)
	err := q.QueryRowContext(ctx,
		`SELECT user_id, task_id, generation FROM runs WHERE run_id = ?`, runID).
		Scan(&r.userID, &taskID, &r.generation)
	if errors.Is(err, sql.ErrNoRows) {
		return runRow{}, fmt.Errorf("ledger: unknown run %q", runID)
	}
	if err != nil {
		return runRow{}, fmt.Errorf("ledger: read run %q: %w", runID, err)
	}
	r.taskID = taskID.String
	if r.taskID == "" {
		return runRow{}, fmt.Errorf("%w: run %q", ErrNoTask, runID)
	}
	return r, nil
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// currentQuery selects the latest ledger_update payload for a task across
// all of the task's runs (fork successors continue the same ledger), by
// event_seq — the sole ordering authority (Spec S02.5).
const currentQuery = `
SELECT e.payload FROM run_events e
  JOIN runs r ON e.run_id = r.run_id
 WHERE r.task_id = ? AND e.type = '` + EventLedgerUpdate + `'
 ORDER BY e.event_seq DESC LIMIT 1`

func currentDoc(ctx context.Context, q queryer, taskID string) (Document, string, bool, error) {
	var payload string
	err := q.QueryRowContext(ctx, currentQuery, taskID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return Document{}, "", false, nil
	}
	if err != nil {
		return Document{}, "", false, fmt.Errorf("ledger: read current revision: %w", err)
	}
	var up updatePayload
	if err := json.Unmarshal([]byte(payload), &up); err != nil {
		return Document{}, "", false, fmt.Errorf("ledger: decode ledger_update payload: %w", err)
	}
	return up.Ledger, up.ContentHash, true, nil
}

// apply runs one accepted write: read the run context and current document,
// mutate, bump ledger_version, and append the ledger_update event — all in
// one write transaction (CONVENTIONS §6). assertedGen, when non-nil, is the
// caller's generation claim appended as-is so the event-log fence rejects a
// superseded stage session (Spec S02.5 step 4); nil appends at the run's
// current generation (control-plane acts).
func (s *Store) apply(ctx context.Context, runID string, assertedGen *int64, mutate func(doc *Document) (Change, error)) (Document, error) {
	var out Document
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		r, err := readRun(ctx, tx, runID)
		if err != nil {
			return err
		}
		doc, _, found, err := currentDoc(ctx, tx, r.taskID)
		if err != nil {
			return err
		}
		if !found {
			// Birth: the first accepted write starts from the empty document
			// (version 0); header identity comes from the task row.
			var owner string
			if err := tx.QueryRowContext(ctx,
				`SELECT user_id FROM tasks WHERE task_id = ?`, r.taskID).Scan(&owner); err != nil {
				return fmt.Errorf("ledger: read task %q: %w", r.taskID, err)
			}
			doc = Document{TaskID: r.taskID, Owner: owner}
		}
		change, err := mutate(&doc)
		if err != nil {
			return err
		}
		doc.LedgerVersion++
		doc.UpdatedAt = s.nowRFC3339()
		hash, _, err := contentHash(doc)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(updatePayload{Change: change, ContentHash: hash, Ledger: doc})
		if err != nil {
			return fmt.Errorf("ledger: marshal ledger_update: %w", err)
		}
		gen := r.generation
		if assertedGen != nil {
			gen = *assertedGen
		}
		if _, err := s.log.AppendTx(ctx, tx, eventlog.Append{
			RunID:         runID,
			Generation:    gen,
			UserID:        r.userID,
			Type:          EventLedgerUpdate,
			SchemaVersion: ledgerUpdateSchemaVersion,
			Payload:       payload,
		}); err != nil {
			return err
		}
		out = doc
		return nil
	})
	if err != nil {
		return Document{}, err
	}
	return out, nil
}

// Current returns the task's current ledger document.
func (s *Store) Current(ctx context.Context, taskID string) (Document, bool, error) {
	doc, _, found, err := currentDoc(ctx, s.db, taskID)
	return doc, found, err
}

// AtVersion returns the task's ledger document at an exact revision — the
// recovery and replay read (Spec S05.2: recovery forks a fresh session from
// the checkpointed ledger).
func (s *Store) AtVersion(ctx context.Context, taskID string, version int64) (Document, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.payload FROM run_events e
		  JOIN runs r ON e.run_id = r.run_id
		 WHERE r.task_id = ? AND e.type = '`+EventLedgerUpdate+`'
		 ORDER BY e.event_seq`, taskID)
	if err != nil {
		return Document{}, fmt.Errorf("ledger: read revisions: %w", err)
	}
	defer rows.Close()
	any := false
	for rows.Next() {
		any = true
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return Document{}, fmt.Errorf("ledger: scan revision: %w", err)
		}
		var up updatePayload
		if err := json.Unmarshal([]byte(payload), &up); err != nil {
			return Document{}, fmt.Errorf("ledger: decode ledger_update payload: %w", err)
		}
		if up.Ledger.LedgerVersion == version {
			return up.Ledger, nil
		}
	}
	if err := rows.Err(); err != nil {
		return Document{}, err
	}
	if !any {
		return Document{}, fmt.Errorf("%w: task %q", ErrNoLedger, taskID)
	}
	return Document{}, fmt.Errorf("%w: task %q version %d", ErrVersionNotFound, taskID, version)
}

// CheckpointRef returns the S02.4(c) ledger-revision block for a run: the
// current revision ref of the run's task ledger, "" when the run has no
// task or the task has no ledger yet. Wired as the adapter Driver's
// LedgerRevision seam.
func (s *Store) CheckpointRef(ctx context.Context, runID string) (string, error) {
	var taskID sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT task_id FROM runs WHERE run_id = ?`, runID).Scan(&taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("ledger: unknown run %q", runID)
	}
	if err != nil {
		return "", fmt.Errorf("ledger: read run %q: %w", runID, err)
	}
	if taskID.String == "" {
		return "", nil
	}
	doc, hash, found, err := currentDoc(ctx, s.db, taskID.String)
	if err != nil {
		return "", err
	}
	if !found {
		return "", nil
	}
	ref, err := json.Marshal(RevisionRef{LedgerVersion: doc.LedgerVersion, SHA256: hash})
	if err != nil {
		return "", fmt.Errorf("ledger: marshal revision ref: %w", err)
	}
	return string(ref), nil
}

// ---- Session verb surface (Spec S05.1) ----

// Verbs is the platform tool surface a stage session mutates the ledger
// through (Spec S05.1: ledger.decide / ledger.state / ledger.artifact /
// ledger.note). Writes to the pinned sections and to the verified flag are
// structurally impossible from here — those APIs simply do not exist on
// this type. Constructed per stage session with the session's asserted
// generation, so a superseded (forked-over) session's writes are fenced
// out at the event log (Spec S02.5 step 4). The engine-facing tool wiring
// that calls this surface is the pipeline's (Spec S06/S03.4, B2-2+).
type Verbs struct {
	store      *Store
	runID      string
	stage      string
	generation int64
}

// SessionVerbs returns the verb surface for one stage session of a run.
func (s *Store) SessionVerbs(runID, stage string, generation int64) *Verbs {
	return &Verbs{store: s, runID: runID, stage: stage, generation: generation}
}

func (v *Verbs) apply(ctx context.Context, mutate func(doc *Document) (Change, error)) (Document, error) {
	gen := v.generation
	return v.store.apply(ctx, v.runID, &gen, mutate)
}

// Decide appends a §3 decision (author: coordinator). supersedes = 0 for a
// plain decision; a reversal cites the superseded seq and is a NEW entry —
// decisions are never edited (Spec S05.1 §3 append-only rule).
func (v *Verbs) Decide(ctx context.Context, text, reason string, supersedes int64) (Document, error) {
	return v.apply(ctx, func(doc *Document) (Change, error) {
		return appendDecision(doc, v.store.nowRFC3339(), v.stage, AuthorCoordinator, text, reason, supersedes)
	})
}

// StateUpdate is the ledger.state argument: item upserts plus optional
// replacement of the current/next_actions/blockers fields (nil = leave
// unchanged).
type StateUpdate struct {
	Upserts     []WorkItem
	Current     *string
	NextActions *[]string
	Blockers    *[]string
}

// State applies a §4 state update under the session write rules: a session
// may claim at most done_unverified (ErrVerifiedFromSession otherwise), and
// an item the control plane has verified is closed to session writes
// (ErrVerifiedItemProtected).
func (v *Verbs) State(ctx context.Context, u StateUpdate) (Document, error) {
	return v.apply(ctx, func(doc *Document) (Change, error) {
		for _, item := range u.Upserts {
			if item.ID == "" {
				return Change{}, fmt.Errorf("%w: work item without id", ErrInvalidWrite)
			}
			if item.Status == StatusVerified {
				return Change{}, fmt.Errorf("%w: item %q", ErrVerifiedFromSession, item.ID)
			}
			if item.Status != "" && !validStatus[item.Status] {
				return Change{}, fmt.Errorf("%w: item %q status %q", ErrInvalidWrite, item.ID, item.Status)
			}
			if err := validACRefs(doc, item.ACRefs); err != nil {
				return Change{}, fmt.Errorf("item %q: %w", item.ID, err)
			}
			cur := doc.item(item.ID)
			switch {
			case cur == nil:
				if item.Summary == "" {
					return Change{}, fmt.Errorf("%w: new item %q without summary", ErrInvalidWrite, item.ID)
				}
				if item.Status == "" {
					item.Status = StatusPending
				}
				doc.State.Items = append(doc.State.Items, item)
			case cur.Status == StatusVerified:
				return Change{}, fmt.Errorf("%w: item %q", ErrVerifiedItemProtected, item.ID)
			default:
				if item.Summary != "" {
					cur.Summary = item.Summary
				}
				if item.ACRefs != nil {
					cur.ACRefs = item.ACRefs
				}
				if item.Status != "" {
					cur.Status = item.Status
				}
				if item.EvidenceRef != "" {
					cur.EvidenceRef = item.EvidenceRef
				}
			}
		}
		if u.Current != nil {
			if *u.Current != "" && doc.item(*u.Current) == nil {
				return Change{}, fmt.Errorf("%w: current %q", ErrUnknownItem, *u.Current)
			}
			doc.State.Current = *u.Current
		}
		if u.NextActions != nil {
			doc.State.NextActions = *u.NextActions
		}
		if u.Blockers != nil {
			doc.State.Blockers = *u.Blockers
		}
		return Change{Verb: VerbState, Actor: AuthorCoordinator, Stage: v.stage}, nil
	})
}

// Artifact appends a §5 restorable reference. The hash is optional at the
// verb (Spec S05.1: ledger.artifact(ref, description, hash?)); the control
// plane records/verifies hashes at checkpoint (Spec S02).
func (v *Verbs) Artifact(ctx context.Context, ref, kind, description, sha256hex string) (Document, error) {
	return v.apply(ctx, func(doc *Document) (Change, error) {
		if ref == "" {
			return Change{}, fmt.Errorf("%w: artifact without ref", ErrInvalidWrite)
		}
		if err := oneLine("artifact description", description); err != nil {
			return Change{}, err
		}
		if sha256hex != "" {
			if len(sha256hex) != 64 {
				return Change{}, fmt.Errorf("%w: artifact hash must be sha256 hex", ErrInvalidWrite)
			}
			if _, err := hex.DecodeString(sha256hex); err != nil {
				return Change{}, fmt.Errorf("%w: artifact hash must be sha256 hex", ErrInvalidWrite)
			}
		}
		doc.Artifacts = append(doc.Artifacts, Artifact{
			ID:  "a" + strconv.Itoa(len(doc.Artifacts)+1),
			Ref: ref, Kind: kind, Description: description,
			SHA256: sha256hex, ProducingStage: v.stage,
		})
		return Change{Verb: VerbArtifact, Actor: AuthorCoordinator, Stage: v.stage}, nil
	})
}

// Note appends a §6 learned_this_task entry.
func (v *Verbs) Note(ctx context.Context, text string) (Document, error) {
	return v.apply(ctx, func(doc *Document) (Change, error) {
		if text == "" {
			return Change{}, fmt.Errorf("%w: empty note", ErrInvalidWrite)
		}
		doc.LearnedThisTask = append(doc.LearnedThisTask, Learned{
			Note: text, TS: v.store.nowRFC3339(), Stage: v.stage,
		})
		return Change{Verb: VerbNote, Actor: AuthorCoordinator, Stage: v.stage}, nil
	})
}

// ---- Control-plane write surface (Spec S05.1 writers column) ----

// SetObjective writes §1 from the confirmed specification (Spec S06 owns
// how the spec enters; this is the ledger-side seam). The section is
// pinned: once set it changes only under a NEW spec version through
// re-approval, which bumps spec_version + ledger_version.
func (s *Store) SetObjective(ctx context.Context, runID, actor string, o ObjectiveAC) (Document, error) {
	return s.apply(ctx, runID, nil, func(doc *Document) (Change, error) {
		if o.Objective == "" || o.SpecVersion == "" {
			return Change{}, fmt.Errorf("%w: objective and spec version required", ErrInvalidWrite)
		}
		if len(o.AcceptanceCriteria) == 0 {
			return Change{}, fmt.Errorf("%w: numbered acceptance criteria required", ErrInvalidWrite)
		}
		for i, ac := range o.AcceptanceCriteria {
			if ac.N != i+1 {
				return Change{}, fmt.Errorf("%w: acceptance criteria must be numbered 1..n contiguously", ErrInvalidWrite)
			}
			if ac.Plain == "" {
				return Change{}, fmt.Errorf("%w: criterion %d without plain phrasing (G1 P10)", ErrInvalidWrite, ac.N)
			}
		}
		if doc.ObjectiveAC.Objective != "" && o.SpecVersion == doc.SpecVersion {
			return Change{}, fmt.Errorf("%w: objective_ac at spec %q", ErrPinnedImmutable, doc.SpecVersion)
		}
		doc.ObjectiveAC = o
		doc.SpecVersion = o.SpecVersion
		return Change{Verb: VerbObjective, Actor: actor}, nil
	})
}

// SetConstraints writes §2 (task constraints + the registry danger-zone
// snapshot) under the header's current spec version; a re-set requires the
// spec version to have moved (§2 mutation rule: same as §1).
func (s *Store) SetConstraints(ctx context.Context, runID, actor string, c ConstraintsDangerZones) (Document, error) {
	return s.apply(ctx, runID, nil, func(doc *Document) (Change, error) {
		if doc.SpecVersion == "" {
			return Change{}, fmt.Errorf("%w: no spec version on the ledger (set objective_ac first)", ErrInvalidWrite)
		}
		if c.SpecVersion != doc.SpecVersion {
			return Change{}, fmt.Errorf("%w: got %q, header %q", ErrSpecVersionMismatch, c.SpecVersion, doc.SpecVersion)
		}
		for _, z := range c.DangerZones {
			if z.Path == "" || z.Rule == "" {
				return Change{}, fmt.Errorf("%w: danger zone requires path and rule", ErrInvalidWrite)
			}
		}
		set := len(doc.Constraints.Constraints) > 0 || len(doc.Constraints.DangerZones) > 0 || doc.Constraints.SpecVersion != ""
		if set && doc.Constraints.SpecVersion == c.SpecVersion {
			return Change{}, fmt.Errorf("%w: constraints_danger_zones at spec %q", ErrPinnedImmutable, c.SpecVersion)
		}
		doc.Constraints = c
		return Change{Verb: VerbConstraints, Actor: actor}, nil
	})
}

// RecordDecision appends a §3 entry authored by the control plane: a human
// answer/approval (AuthorHuman) or a platform adjustment such as the 4.3
// adjust-with-note outcome (AuthorPlatform, Spec S05.6).
func (s *Store) RecordDecision(ctx context.Context, runID, author, actor, stage, text, reason string, supersedes int64) (Document, error) {
	if author != AuthorHuman && author != AuthorPlatform {
		return Document{}, fmt.Errorf("%w: control-plane decision author must be human or platform", ErrInvalidWrite)
	}
	return s.apply(ctx, runID, nil, func(doc *Document) (Change, error) {
		ch, err := appendDecision(doc, s.nowRFC3339(), stage, author, text, reason, supersedes)
		if err != nil {
			return Change{}, err
		}
		ch.Actor = actor
		return ch, nil
	})
}

// SetVerified flips a done_unverified item to verified from a recorded
// verification verdict — the ONLY path to verified status (Spec S05.1 §4,
// G1 Def.12). evidenceRef names the verdict record; the verification
// machinery producing it is Spec S07's (B2-3).
func (s *Store) SetVerified(ctx context.Context, runID, actor, itemID, evidenceRef string) (Document, error) {
	return s.apply(ctx, runID, nil, func(doc *Document) (Change, error) {
		if evidenceRef == "" {
			return Change{}, fmt.Errorf("%w: verified requires a verdict evidence ref (Spec S07)", ErrInvalidWrite)
		}
		it := doc.item(itemID)
		if it == nil {
			return Change{}, fmt.Errorf("%w: %q", ErrUnknownItem, itemID)
		}
		if it.Status != StatusDoneUnverified {
			return Change{}, fmt.Errorf("%w: item %q is %s", ErrNotVerifiable, itemID, it.Status)
		}
		it.Status = StatusVerified
		it.EvidenceRef = evidenceRef
		return Change{Verb: VerbVerify, Actor: actor}, nil
	})
}

// SetPlanVersion records the approved plan version on the header (Spec
// S05.1 header; how the plan enters the ledger is Spec S06's, B2-2).
func (s *Store) SetPlanVersion(ctx context.Context, runID, actor, planVersion string) (Document, error) {
	return s.apply(ctx, runID, nil, func(doc *Document) (Change, error) {
		if planVersion == "" {
			return Change{}, fmt.Errorf("%w: empty plan version", ErrInvalidWrite)
		}
		doc.PlanVersion = planVersion
		return Change{Verb: VerbPlanVersion, Actor: actor}, nil
	})
}

// appendDecision is the shared §3 append: entries are never edited or
// summarized; seq is dense and monotonic.
func appendDecision(doc *Document, ts, stage, author, text string, reason string, supersedes int64) (Change, error) {
	if text == "" {
		return Change{}, fmt.Errorf("%w: empty decision text", ErrInvalidWrite)
	}
	if err := oneLine("decision reason", reason); err != nil {
		return Change{}, err
	}
	if supersedes != 0 && !doc.decision(supersedes) {
		return Change{}, fmt.Errorf("%w: %d", ErrUnknownSupersedes, supersedes)
	}
	doc.Decisions = append(doc.Decisions, Decision{
		Seq: int64(len(doc.Decisions)) + 1, TS: ts, Stage: stage,
		Author: author, Text: text, Reason: reason, Supersedes: supersedes,
	})
	return Change{Verb: VerbDecide, Actor: author, Stage: stage}, nil
}

func validACRefs(doc *Document, refs []int) error {
	for _, n := range refs {
		found := false
		for _, ac := range doc.ObjectiveAC.AcceptanceCriteria {
			if ac.N == n {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%w: ac_ref %d not in objective_ac", ErrInvalidWrite, n)
		}
	}
	return nil
}
