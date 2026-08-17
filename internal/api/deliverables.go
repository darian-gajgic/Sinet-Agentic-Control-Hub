package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
)

// deliverables.go — the S15.2 deliverables family (P3-B6-3B): the owner-scoped
// list and detail, the per-type comparison data react-diff-view consumes, and
// the anchored-comment surface.
//
// NOTHING HERE RE-IMPLEMENTS S13. `internal/review` owns what a revision is,
// what a comparison of two revisions looks like per type, where a comment
// anchors after a port, and which of the five anchor statuses that placement
// carries. This file resolves the caller's authority, hands the read or the
// write to its owner, and serves what comes back. The one thing it adds is the
// LIST query, because the review store has no list verb and a list is a
// transport concern (bounded, filtered, newest-first) — the reads.go precedent.
//
// COMMENT LIFECYCLE IS S13.3's, AND IT SHIPS NO UPDATE AND NO DELETE (OQ2).
// Migration 0007 makes a comment immutable — "only the one-way consumption
// stamp moves" — and never deleted (P-T12-2), and consumption happens through
// THE drain alone (Spec S13.4, source-scan-pinned to review/drain.go). So
// S15.2's "comment CRUD (own comments)" grounds to Create + Read under that
// lifecycle: there is no edit verb and no delete verb to route, "own comments"
// is authorship attribution rather than a mutation right, and a person who
// wants to change what they said adds another comment. The route-table negative
// asserting their absence is in the test file.
//
// SERVED CONTENT IS UNWRAPPED. Diff text, comment bodies, revision pins and
// deliverable rows are S13 product read from S13's OWN tables — content, not an
// observability projection lifted out of a run_events payload (§38 ruling (b);
// §40). Nothing in this file touches internal/redact, so the enumerated
// redaction edge stays honest at three files.

// deliverablesListCap bounds the deliverable list. Structural, not ⚙ (S18
// ratifies no such key — the §7 sseBatchSize precedent, interim under the
// standing settings-tab directive), and it is the same number as
// readPageDefault because this is one read surface with one page size.
const deliverablesListCap = readPageDefault

// The deliverable states a `?state=` filter may name (Spec S13.1 — the three
// stored states, closed).
var deliverableStates = map[string]bool{
	review.StateInReview: true, review.StateAccepted: true, review.StateSuperseded: true,
}

// ── the doors: a surface must name the live verbs it has ────────────────────

// The door vocabulary (OQ3 = doors-as-data). Each names a route that EXISTS,
// with the preconditions under which it is live; none of them is a new verb.
// A second endpoint reaching the same landed act would be the double-mint
// shape in transport form — one act, two doors, two audit stories.
const (
	doorAccept          = "accept"
	doorRequestRevision = "request-revision"
	doorFollowUp        = "follow-up"
	doorPreview         = "preview"
	doorPreviewCompare  = "preview-compare"
)

// Door is one live verb on a deliverable, with the preconditions that decide
// whether it is open right now and the reason either way — the D3/D9 honesty
// rule applied to a resource detail: no dead door, and no live door left
// unnamed. A door that is not available still SHIPS, carrying why, so a surface
// can explain the state instead of silently omitting the control.
type Door struct {
	Verb      string `json:"verb"`
	Method    string `json:"method"`
	Route     string `json:"route"`
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
	// PinFrom names the read a caller makes FIRST to obtain the payload_hash
	// its body must quote (S15.2 retry-safety). Empty for doors that answer no
	// card and therefore pin nothing.
	PinFrom string `json:"pin_from,omitempty"`
	// Answer is the answer verb the body must carry when this door answers a
	// card (the S07.7 `revise_with_guidance` limb of OQ3).
	Answer string `json:"answer,omitempty"`
	// AskID + PayloadHash are that card's identity and its current pin.
	AskID       string `json:"ask_id,omitempty"`
	PayloadHash string `json:"payload_hash,omitempty"`
	// Preset is the S13.9 framing this door spawns under (the follow-up limb).
	Preset string `json:"preset,omitempty"`
}

// ── GET /api/deliverables ───────────────────────────────────────────────────

// DeliverableListItem is one deliverable row as the list serves it.
type DeliverableListItem struct {
	DeliverableID   string    `json:"deliverable_id"`
	Owner           string    `json:"owner"`
	TaskID          string    `json:"task_id"`
	ProjectID       string    `json:"project_id"`
	Type            string    `json:"type"`
	CurrentRevision int       `json:"current_revision"`
	State           string    `json:"state"`
	CreatedTS       time.Time `json:"created_ts"`
	UpdatedTS       time.Time `json:"updated_ts"`
}

// DeliverableList is the list response with its S15.3 tail cursor.
type DeliverableList struct {
	Deliverables []DeliverableListItem `json:"deliverables"`
	Cursor       int64                 `json:"cursor"`
	Truncated    bool                  `json:"truncated"`
}

func (s *Server) handleDeliverableList(w http.ResponseWriter, r *http.Request) {
	// The list is gated on the review store even though it reads the table
	// directly: one family, one readiness. A list whose every row's detail,
	// diff and comment link answers 503 is a worse answer than saying the
	// family is not wired in this process.
	if !s.reviewReady(w) || !s.projReady(w) {
		return
	}
	scope := s.readScope(r)
	limit, err := readLimit(r)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	if limit > deliverablesListCap {
		limit = deliverablesListCap
	}
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if state != "" && !deliverableStates[state] {
		s.writeSurface(w, nil, badRequest(fmt.Sprintf(
			"unknown state %q: a deliverable is in-review, accepted or superseded (S13.1)", state)))
		return
	}

	q := `SELECT deliverable_id, user_id, task_id, project_id, dtype, current_revision, state, created_ts, updated_ts
	        FROM deliverables WHERE 1 = 1`
	args := []any{}
	if !scope.Operator {
		q += ` AND user_id = ?`
		args = append(args, scope.UserID)
	}
	for _, f := range []struct {
		clause string
		val    string
	}{
		{` AND project_id = ?`, strings.TrimSpace(r.URL.Query().Get("project"))},
		{` AND state = ?`, state},
		{` AND dtype = ?`, strings.TrimSpace(r.URL.Query().Get("type"))},
		// `?task=` is additive (B6-5 drain r1 D5): the S15.5 task detail lists
		// "every deliverable revision" for ONE task, and without it the surface
		// had no way to ask that question — the first cut rendered follow-up
		// LINEAGE edges under the deliverables heading instead, which is a
		// different fact wearing the same label. Owner scope is untouched: this
		// narrows an already-scoped read.
		{` AND task_id = ?`, strings.TrimSpace(r.URL.Query().Get("task"))},
	} {
		if f.val != "" {
			q += f.clause
			args = append(args, f.val)
		}
	}
	q += ` ORDER BY created_ts DESC, deliverable_id DESC LIMIT ?`
	args = append(args, limit+1) // one beyond, so Truncated is OBSERVED

	cursor, err := s.head(r.Context())
	if err != nil {
		s.writeSurface(w, nil, fmt.Errorf("read head cursor: %w", err))
		return
	}
	rows, err := s.proj.db.QueryContext(r.Context(), q, args...)
	if err != nil {
		s.writeSurface(w, nil, fmt.Errorf("read deliverables: %w", err))
		return
	}
	defer rows.Close()
	out := DeliverableList{Deliverables: []DeliverableListItem{}, Cursor: cursor}
	for rows.Next() {
		if len(out.Deliverables) >= limit {
			out.Truncated = true
			break
		}
		var it DeliverableListItem
		var created, updated string
		if err := rows.Scan(&it.DeliverableID, &it.Owner, &it.TaskID, &it.ProjectID, &it.Type,
			&it.CurrentRevision, &it.State, &created, &updated); err != nil {
			s.writeSurface(w, nil, fmt.Errorf("scan deliverables: %w", err))
			return
		}
		it.CreatedTS, it.UpdatedTS = parseTS(created), parseTS(updated)
		out.Deliverables = append(out.Deliverables, it)
	}
	if err := rows.Err(); err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	s.writeReadJSON(w, out)
}

// ── GET /api/deliverables/{deliverable} ─────────────────────────────────────

// DeliverableDetail is the S15.2 deliverable resource: the row, its full
// immutable lineage of revisions, the task's follow-up lineage, and the doors.
type DeliverableDetail struct {
	Deliverable review.Deliverable `json:"deliverable"`
	// Revisions is the lineage 1..N, never compressed (Spec S13.1), each with
	// its content pin, its minting run/attempt and its verification verdict ref.
	Revisions []review.Revision `json:"revisions"`
	Lineage   TaskLineage       `json:"lineage"`
	Doors     []Door            `json:"doors"`
	Cursor    int64             `json:"cursor"`
}

func (s *Server) handleDeliverableDetail(w http.ResponseWriter, r *http.Request) {
	if !s.reviewReady(w) || !s.projReady(w) {
		return
	}
	d, ok := s.deliverableScope(w, r)
	if !ok {
		return
	}
	revs, err := s.review.Revisions(r.Context(), d.ID)
	if err != nil {
		s.writeSurface(w, nil, s.reviewErr(err))
		return
	}
	if revs == nil {
		revs = []review.Revision{}
	}
	cursor, err := s.head(r.Context())
	if err != nil {
		s.writeSurface(w, nil, fmt.Errorf("read head cursor: %w", err))
		return
	}
	lineage, err := s.proj.taskLineage(r.Context(), d.TaskID)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	s.writeReadJSON(w, DeliverableDetail{
		Deliverable: d,
		Revisions:   revs,
		Lineage:     lineage,
		Doors:       s.doorsFor(r.Context(), d, revs),
		Cursor:      cursor,
	})
}

// doorsFor names every live verb on this deliverable with its preconditions
// (OQ3 = doors-as-data). NO new verb is minted here: the accept is R14's route,
// the two request-revision limbs are the LANDED `revise_with_guidance` answer
// (B2-5, routed by B6-2A) and the LANDED follow-up spawn with its `revision`
// preset (B6-2A), and the preview doors are R16's.
func (s *Server) doorsFor(ctx context.Context, d review.Deliverable, revs []review.Revision) []Door {
	base := "/api/deliverables/" + d.ID
	doors := []Door{s.acceptDoor(ctx, d, revs, base)}
	doors = append(doors, s.reviseDoor(ctx, d, base))
	doors = append(doors, Door{
		Verb: doorFollowUp, Method: http.MethodPost, Route: base + "/follow-up",
		Available: d.CurrentRevision >= 1,
		Reason: followUpReason(d.CurrentRevision >= 1) +
			" A successor task links to a numbered revision of this deliverable (S13.9); the framings are presets over that one link.",
	})
	previewOK := s.preview != nil && d.CurrentRevision >= 1
	previewWhy := "a preview launches from a minted revision and answers a defined state for every type — no-preview and requires-container-tier are answers, not failures (S13.8)"
	switch {
	case s.preview == nil:
		previewWhy = "no preview surface is composed in this process"
	case d.CurrentRevision < 1:
		previewWhy = "no revision is minted yet, so there is nothing to preview (S13.1)"
	}
	doors = append(doors,
		Door{Verb: doorPreview, Method: http.MethodPost, Route: base + "/preview", Available: previewOK, Reason: previewWhy},
		Door{Verb: doorPreviewCompare, Method: http.MethodPost, Route: base + "/preview/compare", Available: previewOK,
			Reason: previewWhy + "; with no accepted revision yet the pair answers the honest single-instance state (S13.8 S1.9)"},
	)
	return doors
}

func followUpReason(ok bool) string {
	if ok {
		return "open:"
	}
	return "closed until a revision is minted:"
}

// acceptDoor states whether the accept is live — and it ASKS `acceptable()`
// (accept.go), which is the one authority on the accept's preconditions, rather
// than mirroring them here (P3-RW-17 finish).
//
// THE DOOR MAY SUMMARIZE THE AUTHORITY; IT MAY NEVER CONTRADICT IT. A hand-copy
// of the preconditions is exactly what went stale: this limb still refused a
// payload-pinned revision as "not repo-backed … an accept pushes a commit" long
// after 5.8/D10/S13.1 opened the accept for every deliverable, so the detail
// introduced a High-tier press with the reason it could not be made while the
// same deliverable's card opened and the verb worked. Delegating leaves ONE
// place where a precondition can change.
//
// What stays door-shaped is only the OPEN sentence, because the door's job
// differs from the card's: the card carries the pin and the trailers, and the
// door points at the card. That sentence is per-ARM for the same reason the
// card's tier statement is — the person is about to make one of two different
// acts, and being told about the other one is being told something untrue
// (S13.1/S13.6).
func (s *Server) acceptDoor(ctx context.Context, d review.Deliverable, revs []review.Revision, base string) Door {
	door := Door{
		Verb: doorAccept, Method: http.MethodPost, Route: base + "/accept",
		PinFrom: "GET " + base + "/accept-card",
	}
	rev := currentRevision(revs, d.CurrentRevision)
	// The trailer facts are read only when the act is actually in play. The
	// guard mirrors `acceptable()`'s own earlier limbs — not its arm split — so
	// the door never assumes WHICH arm needs provenance, only that a settled
	// refusal needs none.
	var prov AcceptProvenance
	if s.accept != nil && d.State == review.StateInReview && rev.N >= 1 {
		prov = s.acceptProvenance(ctx, rev)
	}
	door.Available, door.Reason = acceptable(s.accept != nil, d, rev, prov)
	if !door.Available {
		return door
	}
	if rev.SnapshotSHA == "" {
		door.Reason = "open: nothing is pushed — revision " + strconv.Itoa(rev.N) +
			" pins its content, so accepting records your decision against that immutable pin and files the work as accepted. " +
			"Read the card for the pin, then accept with your PIN in the same request (High tier, S13.1/S15.6)"
		return door
	}
	door.Reason = "open: read the card for the trailers and the pin, then accept with your PIN in the same request (High tier, S15.6)"
	return door
}

// reviseDoor is OQ3's disposition in one place: which LANDED door a bounded
// revision goes through depends on the deliverable's state, and the detail says
// which one and what it needs.
//
//   - parked on a rework-family card ⇒ the S07.7 answer verb, naming the open
//     ask, the verb, and the pin the answer must quote;
//   - accepted (or otherwise finished) ⇒ the S13.9 follow-up with the `revision`
//     preset, which is the same landed spawn under its revision framing.
//
// Neither is a new endpoint. A convenience verb dispatching between them would
// be a second route to acts that already have one apiece.
func (s *Server) reviseDoor(ctx context.Context, d review.Deliverable, base string) Door {
	door := Door{Verb: doorRequestRevision}
	if d.State == review.StateInReview {
		if ask, hash, ok := s.openReworkCard(ctx, d.TaskID); ok {
			door.Method, door.Route = http.MethodPost, "/api/approvals/"+approvalKindAsk+":"+ask+"/answer"
			door.Available, door.Answer, door.AskID, door.PayloadHash = true, verbReviseWithGuidance, ask, hash
			door.PinFrom = "GET /api/approvals"
			door.Reason = "open: this deliverable is parked on a rework-family card, so a bounded revision is that card's " +
				verbReviseWithGuidance + " answer — the guidance lands as durable requester comments and enters the next attempt through THE drain (S07.7/S13.4)"
			return door
		}
		// No concrete target exists yet, so the door names NO route: the verb it
		// will use is an answer to a card that has not been issued, and the
		// follow-up route belongs to the finished door alone. A route naming a
		// verb that cannot open from this state is the dead-door shape the D3/D9
		// honesty rule exists to prevent — the reason carries the narrative
		// instead.
		door.Reason = "closed for now: the work is in review and no rework card is open. A bounded revision travels as the " +
			verbReviseWithGuidance + " answer once the platform asks; until then, comments on the revision are what the next round drains (S13.3/S13.4)"
		return door
	}
	door.Method, door.Route, door.Preset = http.MethodPost, base+"/follow-up", string(intake.PresetRevision)
	door.Available = d.CurrentRevision >= 1
	door.Reason = "open: this deliverable is finished, so a bounded revision is a successor task under the S13.9 `revision` framing over the one successor link"
	if !door.Available {
		door.Reason = "closed: there is no minted revision for a successor to link to (S13.9)"
	}
	return door
}

// verbReviseWithGuidance is the S07.7 answer verb this door names. It is the
// transport's own reference to a vocabulary internal/verify owns and validates;
// internal/api does not import the verification pipeline, exactly as the run
// record types in reads.go are literals rather than imports.
const verbReviseWithGuidance = "revise_with_guidance"

// reworkCardCap bounds the open-ask scan behind the request-revision door.
// Structural, with its reason: a task has a handful of open cards at most, but
// expectation is not a bound and this read shares the one writer connection
// (S02.1).
const reworkCardCap = 50

// openReworkCard finds the task's open rework-family card and pins it. The
// card's own `choices` decide: a card that offers revise_with_guidance is a
// rework card, which is the producer's word for it rather than a second
// classification of the same fact (internal/verify's escalation Card is what
// lands in asks.snapshot). The pin is the ONE canonicalization — the same
// gates.CanonicalHash the answer verb checks the quote against — so the door
// hands over a hash the door's own route will accept.
func (s *Server) openReworkCard(ctx context.Context, taskID string) (askID, payloadHash string, ok bool) {
	if taskID == "" {
		return "", "", false
	}
	rows, err := s.proj.db.QueryContext(ctx,
		`SELECT a.ask_id, a.snapshot FROM asks a JOIN runs r ON r.run_id = a.run_id
		  WHERE r.task_id = ? AND a.answered_ts IS NULL
		  ORDER BY a.observed_ts DESC LIMIT ?`, taskID, reworkCardCap)
	if err != nil {
		s.logger.Warn("deliverables: read open rework cards", "task", taskID, "err", err)
		return "", "", false
	}
	defer rows.Close()
	for rows.Next() {
		var id, snapshot string
		if err := rows.Scan(&id, &snapshot); err != nil {
			s.logger.Warn("deliverables: scan open rework card", "task", taskID, "err", err)
			return "", "", false
		}
		var card struct {
			Choices []string `json:"choices"`
		}
		if json.Unmarshal([]byte(snapshot), &card) != nil {
			continue
		}
		for _, c := range card.Choices {
			if c != verbReviseWithGuidance {
				continue
			}
			hash, err := gates.CanonicalHash(json.RawMessage(snapshot))
			if err != nil {
				s.logger.Warn("deliverables: pin rework card", "ask", id, "err", err)
				return "", "", false
			}
			return id, hash, true
		}
	}
	if err := rows.Err(); err != nil {
		s.logger.Warn("deliverables: open rework cards", "task", taskID, "err", err)
	}
	return "", "", false
}

func currentRevision(revs []review.Revision, n int) review.Revision {
	for _, r := range revs {
		if r.N == n {
			return r
		}
	}
	return review.Revision{}
}

// ── GET /api/deliverables/{deliverable}/compare ─────────────────────────────

// The comparison is served VERBATIM as review.Comparison. For the line-diff
// surface that means the host-side UNIFIED GIT DIFF text and nothing else: no
// HTML, no tokenized form, no server-rendered highlight. react-diff-view parses
// unified diffs natively with gitdiff-parser and highlights client-side through
// its own tokenizer, and no server-side highlighter exists in the platform
// (FC-v1 §2; S15.4). Shipping a pre-rendered form would be inventing one.

func (s *Server) handleDeliverableCompare(w http.ResponseWriter, r *http.Request) {
	if !s.reviewReady(w) {
		return
	}
	d, ok := s.deliverableScope(w, r)
	if !ok {
		return
	}
	if d.CurrentRevision < 1 {
		s.writeSurface(w, nil, badRequest("this deliverable has no minted revision to compare (S13.1)"))
		return
	}
	// The FC-v1 §2 round-over-round view is the DEFAULT read: new = current,
	// old = new−1. Revision-over-revision navigation is the same read with an
	// explicit pair — a schema capability, not a second endpoint (S13.1).
	newN, err := revisionParam(r, "new", d.CurrentRevision)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	oldN, err := revisionParam(r, "old", newN-1)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	if newN < 1 || oldN < 0 || oldN == newN {
		s.writeSurface(w, nil, badRequest(fmt.Sprintf(
			"compare needs distinct revisions (old %d, new %d); old=0 is the pre-task base (S13.1)", oldN, newN)))
		return
	}
	cmp, err := s.review.Compare(r.Context(), d.ID, oldN, newN)
	if err != nil {
		s.writeSurface(w, nil, s.reviewErr(err))
		return
	}
	s.writeReadJSON(w, cmp)
}

// revisionParam parses a revision query bound, defaulting when absent.
func revisionParam(r *http.Request, name string, def int) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, badRequest(fmt.Sprintf("bad %s %q: want a revision number (0 = the pre-task base)", name, raw))
	}
	return n, nil
}

// ── GET /api/deliverables/{deliverable}/comments ────────────────────────────

// PlacedComments is the review surface's comment data for ONE revision: every
// comment of the deliverable, plus where each one anchors in that revision.
//
// Placements are recomputed server-side on EVERY render (Spec S13.3; FC-v1 §2)
// — a client's positions are never placement authority — and a stored placement
// that disagrees with the recomputation is a loud error inside the store rather
// than a silent mis-anchor. Comments whose placement is `file` or `orphan` are
// still HERE, carrying their original quote and their original-revision link:
// no comment may exist without a render location (P-T12-2), and the strip that
// renders them is S15's.
type PlacedComments struct {
	DeliverableID string             `json:"deliverable_id"`
	RevisionN     int                `json:"revision_n"`
	Comments      []review.Comment   `json:"comments"`
	Placements    []review.Placement `json:"placements"`
	Cursor        int64              `json:"cursor"`
}

func (s *Server) handleCommentList(w http.ResponseWriter, r *http.Request) {
	if !s.reviewReady(w) {
		return
	}
	d, ok := s.deliverableScope(w, r)
	if !ok {
		return
	}
	n, err := revisionParam(r, "revision", d.CurrentRevision)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	if n < 1 {
		s.writeSurface(w, nil, badRequest("this deliverable has no minted revision to place comments against (S13.1)"))
		return
	}
	comments, placements, err := s.review.PlacedComments(r.Context(), d.ID, n)
	if err != nil {
		s.writeSurface(w, nil, s.reviewErr(err))
		return
	}
	if comments == nil {
		comments = []review.Comment{}
	}
	if placements == nil {
		placements = []review.Placement{}
	}
	cursor, err := s.head(r.Context())
	if err != nil {
		s.writeSurface(w, nil, fmt.Errorf("read head cursor: %w", err))
		return
	}
	s.writeReadJSON(w, PlacedComments{
		DeliverableID: d.ID, RevisionN: n,
		Comments: comments, Placements: placements, Cursor: cursor,
	})
}

// ── POST /api/deliverables/{deliverable}/comments ───────────────────────────

// commentBody is one human review comment (6.2) as it arrives.
//
// The claimed anchor is exactly that — CLAIMED. It is validated server-side at
// birth against the revision's own content and lands with whatever status the
// ladder gives it (Spec S13.3; ⚙ review.anchor_drift_lines is read inside the
// store, per port). A client that names a line the quote is not on gets a
// drifted or file-level placement, never the position it asserted.
type commentBody struct {
	RevisionN int                  `json:"revision"`
	Body      string               `json:"body"`
	Severity  string               `json:"severity"`
	Anchor    *review.AnchorRecord `json:"anchor,omitempty"`
	FileLevel string               `json:"file_level,omitempty"`
	Suggested string               `json:"suggested_change,omitempty"`
}

func (s *Server) handleCommentCreate(w http.ResponseWriter, r *http.Request) {
	if !s.reviewReady(w) {
		return
	}
	d, ok := s.deliverableScope(w, r)
	if !ok {
		return
	}
	raw, ok := s.readBody(w, r)
	if !ok {
		return
	}
	var body commentBody
	if err := json.Unmarshal(raw, &body); err != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_body", Msg: err.Error()})
		return
	}
	// Caller input is validated HERE, at the boundary, so a failure from the
	// store below can only be the platform's own (the §39 drain-D7 principle).
	if strings.TrimSpace(body.Body) == "" {
		s.writeSurface(w, nil, badRequest(`missing "body": a comment says something (S13.3)`))
		return
	}
	if body.Severity != "" && body.Severity != review.SeverityBlocker && body.Severity != review.SeverityNote {
		s.writeSurface(w, nil, badRequest(fmt.Sprintf(
			"unknown severity %q: the S13.4 vocabulary is %q (another round) or %q (polish travels along)",
			body.Severity, review.SeverityBlocker, review.SeverityNote)))
		return
	}
	if body.RevisionN < 0 {
		s.writeSurface(w, nil, badRequest("bad revision: a comment anchors to a numbered revision (0 = the current one)"))
		return
	}
	if body.Anchor != nil && body.Anchor.Side != "" &&
		body.Anchor.Side != review.SideOld && body.Anchor.Side != review.SideNew {
		s.writeSurface(w, nil, badRequest(fmt.Sprintf(
			"unknown anchor side %q: an anchor is on the %q or the %q side of the diff (FC-v1 §2)",
			body.Anchor.Side, review.SideOld, review.SideNew)))
		return
	}
	// The comment is attributed to the AUTHENTICATED identity (15.6): whoever
	// says it owns having said it. RunID stays empty, which is what makes the
	// review.comment event platform-scope — this is the human surface, not run
	// machinery recording findings.
	id, _ := IdentityFrom(r.Context())
	c, err := s.review.AddComment(r.Context(), review.CommentInput{
		DeliverableID: d.ID,
		RevisionN:     body.RevisionN,
		Author:        id.UserID,
		Body:          body.Body,
		Severity:      body.Severity,
		Anchor:        body.Anchor,
		FileLevel:     body.FileLevel,
		Suggested:     body.Suggested,
	})
	if err != nil {
		s.writeSurface(w, nil, s.reviewErr(err))
		return
	}
	// Nothing is minted here: AddComment already landed the comment row, its
	// birth placement and the canonical `review.comment` event in ONE
	// transaction, and an act with a landed canonical row is never double-minted
	// (§39 OQ8).
	s.writeReadJSON(w, c)
}

// ── shared readiness + error mapping ────────────────────────────────────────

// reviewReady reports the review store handle, or writes the not-wired answer.
func (s *Server) reviewReady(w http.ResponseWriter) bool {
	if s.review == nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusServiceUnavailable, Code: "not_wired",
			Msg: "the S13 deliverable/review store is not wired in this process"})
		return false
	}
	return true
}

// deliverableScope resolves the path's deliverable and the caller's authority
// over it in the ONE landed shape: 404 before 403, so an unknown id can never
// become an existence oracle for another owner's work (S01.9).
func (s *Server) deliverableScope(w http.ResponseWriter, r *http.Request) (review.Deliverable, bool) {
	d, err := s.review.Deliverable(r.Context(), r.PathValue("deliverable"))
	found := !errors.Is(err, review.ErrNotFound)
	if err != nil && found {
		s.writeSurface(w, nil, fmt.Errorf("read deliverable: %w", err))
		return review.Deliverable{}, false
	}
	if code, cerr := authorizeOwner(s.readScope(r), d.Owner, found, "deliverable"); cerr != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: code, Code: httpCode(code), Msg: cerr.Error()})
		return review.Deliverable{}, false
	}
	return d, true
}

// reviewErr maps the review store's refusals to statuses ON THE ERROR'S TYPE,
// never on its message text (§38's ban on error-string matching). An unmarked
// error is a platform fault — the safe default — and answers 500 with the real
// cause in the ops log.
func (s *Server) reviewErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, review.ErrNotFound):
		return &SurfaceError{Status: http.StatusNotFound, Code: "not_found", Msg: err.Error()}
	case errors.Is(err, review.ErrBadInput):
		return &SurfaceError{Status: http.StatusBadRequest, Code: "bad_request", Msg: err.Error()}
	case errors.Is(err, review.ErrRevisionImmutable):
		return &SurfaceError{Status: http.StatusConflict, Code: "conflict", Msg: err.Error()}
	case errors.Is(err, review.ErrContentDrift):
		// The stored bytes no longer match the revision's pin. That is a
		// platform integrity failure, not something the caller can fix by
		// asking differently, and the surface says so rather than serving
		// content it cannot vouch for.
		s.logger.Error("review: revision content does not match its pin", "err", err)
		return &SurfaceError{Status: http.StatusInternalServerError, Code: "content_drift", Msg: err.Error()}
	}
	s.logger.Error("review", "err", err)
	return &SurfaceError{Status: http.StatusInternalServerError, Code: "internal", Msg: err.Error()}
}
