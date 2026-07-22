package intake

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// S13.9 follow-ups: any finished deliverable spawns a successor task in ONE
// action, carrying a successor_of link to (deliverable, revision) with lineage
// visible in BOTH directions. The successor enters normal intake with the
// project's inherited context plus the predecessor deliverable ref as brief
// material. Revision / extension / counterpart framings are intake PRESETS over
// the SAME link, never schema variants (S13.9). Concurrent follow-ups in one
// project are coordinated by the EXISTING S02.8 claims machinery (the plan-time
// write-set claims built at B2-2) — this file adds NO claims code; and an
// accepted sibling fires the freshness trigger through the accept-success
// producer (Spec S02.8, internal/accept).

// Preset frames a follow-up's intake (S13.9). It is a framing seed over the one
// successor_of link, NOT a schema variant.
type Preset string

const (
	PresetPlain       Preset = "plain"       // a plain follow-up
	PresetRevision    Preset = "revision"    // revise the accepted deliverable
	PresetExtension   Preset = "extension"   // extend it
	PresetCounterpart Preset = "counterpart" // a counterpart ("now the English version")
)

func validPreset(p Preset) bool {
	switch p {
	case PresetPlain, PresetRevision, PresetExtension, PresetCounterpart, "":
		return true
	}
	return false
}

// EventFollowUp records a spawned follow-up (provisional pending S14).
const EventFollowUp = "intake.followup_spawned"

// FollowUp spawns and resolves S13.9 successor links.
type FollowUp struct {
	DB  *storage.DB
	Log *eventlog.Log
	// Start, when set, enters the successor into normal intake with the built
	// Request (S13.9 "the successor enters normal intake"). The composition
	// root wires it to the intake Pipeline.Start; nil leaves the successor task
	// + link created for a later start.
	Start func(ctx context.Context, req Request) error
	Now   func() time.Time
}

// FollowUpInput describes a follow-up spawned from a finished deliverable.
type FollowUpInput struct {
	NewTaskID     string // "" generates one
	Owner         string // the requester (D10 approver, 15.6)
	DeliverableID string
	Revision      int
	ProjectID     string // the inherited project context
	Preset        Preset
	PresetDetail  string // e.g. "now the English version"
	Objective     string // the follow-up's own ask text
	Title         string
	// PredecessorRef is the deliverable ref carried as brief material (S13.9);
	// defaults to the platform revision ref namespace.
	PredecessorRef string
}

func (fu *FollowUp) now() time.Time {
	if fu.Now != nil {
		return fu.Now()
	}
	return time.Now()
}

// Spawn creates the successor task and the successor_of link in ONE action
// (S13.9), then (if a Start seam is wired) enters it into normal intake with the
// inherited context + predecessor ref + preset framing. The composite foreign
// key makes the source (deliverable, revision) referentially real — a follow-up
// can only link to a revision that exists. Returns the intake Request built for
// the successor.
func (fu *FollowUp) Spawn(ctx context.Context, in FollowUpInput) (Request, error) {
	if in.Owner == "" || in.DeliverableID == "" || in.Revision < 1 {
		return Request{}, fmt.Errorf("intake: follow-up needs an owner, deliverable and revision >= 1")
	}
	if !validPreset(in.Preset) {
		return Request{}, fmt.Errorf("intake: unknown follow-up preset %q", in.Preset)
	}
	taskID := in.NewTaskID
	if taskID == "" {
		taskID = "t-" + randomHex(8)
	}
	predecessorRef := in.PredecessorRef
	if predecessorRef == "" {
		predecessorRef = fmt.Sprintf("refs/sinet/deliverable/%s/rev-%d", in.DeliverableID, in.Revision)
	}
	req := Request{
		TaskID: taskID, UserID: in.Owner, Title: in.Title,
		Text: presetFraming(in, predecessorRef),
	}

	now := fu.now().UTC().Format(time.RFC3339Nano)
	err := fu.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO tasks (task_id, user_id, title, created_ts) VALUES (?, ?, ?, ?)`,
			taskID, in.Owner, in.Title, now); err != nil {
			return fmt.Errorf("intake: create successor task: %w", err)
		}
		// The composite FK to deliverable_revisions rejects a link to a
		// non-existent (deliverable, revision).
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO task_successor_of (task_id, deliverable_id, revision_n, user_id, created_ts)
			 VALUES (?, ?, ?, ?, ?)`,
			taskID, in.DeliverableID, in.Revision, in.Owner, now); err != nil {
			return fmt.Errorf("intake: link successor: %w", err)
		}
		payload, err := json.Marshal(map[string]any{
			"task_id": taskID, "deliverable_id": in.DeliverableID, "revision": in.Revision,
			"preset": string(in.Preset), "project_id": in.ProjectID,
		})
		if err != nil {
			return err
		}
		_, err = fu.Log.AppendTx(ctx, tx, eventlog.Append{
			UserID: in.Owner, Type: EventFollowUp, SchemaVersion: 1, Payload: payload, Time: fu.now(),
		})
		return err
	})
	if err != nil {
		return Request{}, err
	}
	// The successor enters normal intake like any task (S13.9), on the same
	// substrate — the predecessor ref rides as brief material through the
	// existing channels.
	if fu.Start != nil {
		if err := fu.Start(ctx, req); err != nil {
			return Request{}, fmt.Errorf("intake: start successor intake: %w", err)
		}
	}
	return req, nil
}

// PredecessorOf resolves a task's predecessor (deliverable, revision) —
// lineage in the task -> source direction (S13.9). ok=false when the task is
// not a follow-up.
func (fu *FollowUp) PredecessorOf(ctx context.Context, taskID string) (deliverableID string, revision int, ok bool, err error) {
	err = fu.DB.QueryRowContext(ctx,
		`SELECT deliverable_id, revision_n FROM task_successor_of WHERE task_id = ?`, taskID).
		Scan(&deliverableID, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, fmt.Errorf("intake: read predecessor of %q: %w", taskID, err)
	}
	return deliverableID, revision, true, nil
}

// SuccessorsOf resolves a (deliverable, revision)'s successor task ids —
// lineage in the source -> tasks direction (S13.9). Both directions are
// resolvable, the S13.9 "visible in both directions" invariant.
func (fu *FollowUp) SuccessorsOf(ctx context.Context, deliverableID string, revision int) ([]string, error) {
	rows, err := fu.DB.QueryContext(ctx,
		`SELECT task_id FROM task_successor_of WHERE deliverable_id = ? AND revision_n = ? ORDER BY task_id`,
		deliverableID, revision)
	if err != nil {
		return nil, fmt.Errorf("intake: read successors: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// presetFraming seeds the successor's intake framing from the preset over the
// SAME link (S13.9: presets, not schema variants). The predecessor ref rides as
// brief material; the objective is the follow-up's own ask.
func presetFraming(in FollowUpInput, predecessorRef string) string {
	var head string
	switch in.Preset {
	case PresetRevision:
		head = "Revision of the accepted deliverable"
	case PresetExtension:
		head = "Extension of the accepted deliverable"
	case PresetCounterpart:
		head = "Counterpart of the accepted deliverable"
	default:
		head = "Follow-up to the accepted deliverable"
	}
	msg := head
	if in.PresetDetail != "" {
		msg += " (" + in.PresetDetail + ")"
	}
	msg += ".\nPredecessor: " + predecessorRef + " [brief material]."
	if in.ProjectID != "" {
		msg += "\nProject: " + in.ProjectID + " [inherited context]."
	}
	if in.Objective != "" {
		msg += "\n\n" + in.Objective
	}
	return msg
}
