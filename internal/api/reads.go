package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/redact"
)

// reads.go is the S15.2 REST read side: the `runs` and `tasks` resource
// families, plus the serving-edge helpers the rest of the read surface shares.
// Everything here PROJECTS over machinery that already exists — the S02.3 FSM
// rows, the B5-2 projections, the intake pipeline's own artifact store, the
// 0016 linkage views — and adds no state, no mutation verb and no new ⚙.
//
// Three contract rules bind every route in this file (B6-1 §F):
//
//   - Owner scope is server-side and fail-closed (S01.9): the one role bit
//     decides, a member sees only their own rows, an unresolved scope is an
//     error and never an anonymous downgrade.
//   - Progress semantics carry the checkable NEGATIVE (§30): no percent,
//     fraction, ratio or ETA field exists in any shape below.
//   - Store-raw / serve-redacted (§30 R19): a response that serves run_events
//     PAYLOAD content passes through redact.RedactJSON at serialization, the
//     writeSnapshotFrame precedent. It is NOT blanket middleware — SPEC/PLAN
//     artifacts, receipts and view rows are structurally exempt (§7-C2·2) and
//     go out through writeReadJSON unwrapped.

// Bounds on a REST read. Both are STRUCTURAL constants, not ⚙ (Spec S18
// ratifies no such key; the §7 sseBatchSize precedent, interim under the
// standing settings-tab directive). They mirror history.DefaultRowLimit /
// history.MaxRowLimit deliberately — one read surface, one page size — and
// carry the same two reasons:
//
//   - readPageDefault: a list is read by a person, and a page is the unit a
//     person reads. The bound changes how much arrives at once, never which
//     rows are eligible.
//   - readPageMax: the control plane shares ONE writer connection (S02.1), so a
//     read that materializes an unbounded result set is a liveness risk to
//     everything else on the handle.
const (
	readPageDefault = 200
	readPageMax     = 1000
)

// runDetailRecordCap bounds the spawn/routing record lists on a run detail.
// Structural, with its reason: those lists exist so a reader can see WHICH
// helpers and routings a run made; the exhaustive trace is the event log, which
// /api/events answers. A pathological run must not turn one detail read into an
// unbounded materialization.
const runDetailRecordCap = 500

// The FSM states a `?status=` filter may name (S02.3 stored states — `wedged`
// and `stalled` are DERIVED and are never a stored value, §30 R11).
var runStates = map[string]bool{
	"new": true, "queued": true, "claimed": true, "running": true, "parked": true,
	"draining": true, "completed": true, "crashed": true, "finalized": true,
	"tombstoned": true, "died-at-gate": true,
}

// noProjectBucket is the 0016 honest bucket for work with no registry-resolved
// project. A `?project=` filter naming it selects exactly that bucket — such
// tasks are never dropped from the project altitude (§37).
const noProjectBucket = "(no project)"

// ── serving-edge helpers ────────────────────────────────────────────────────

// readScope resolves the S01.9 owner scope of a read from the request identity.
func (s *Server) readScope(r *http.Request) ownerScope {
	id, _ := IdentityFrom(r.Context()) // present: every read route is requireIdentity
	return ownerScope{UserID: id.UserID, Operator: s.isOperatorRead(r.Context(), id)}
}

// projReady reports the projection handle, or writes the not-wired answer. A
// read cannot be owner-scoped without the DB, so the absence is refused rather
// than served unscoped.
func (s *Server) projReady(w http.ResponseWriter) bool {
	if s.proj == nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusServiceUnavailable, Code: "not_wired",
			Msg: "the read projections need the platform DB, which is not wired in this process"})
		return false
	}
	return true
}

// writeReadRedacted serializes a PAYLOAD-BEARING read response: the marshaled
// body passes through the codor-C2 primitive once, exactly as the SSE snapshot
// frame does. The stored run_events.payload is never touched (R19).
func (s *Server) writeReadRedacted(w http.ResponseWriter, v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		s.logger.Error("read: marshal", "err", err)
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusInternalServerError, Code: "internal", Msg: "response encoding failed"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(redact.RedactJSON(raw))
}

// writeReadJSON serializes a read response that carries no run_events payload
// content — SPEC/PLAN artifacts, receipts, catalog and view rows (§7-C2·2).
func (s *Server) writeReadJSON(w http.ResponseWriter, v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		s.logger.Error("read: marshal", "err", err)
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusInternalServerError, Code: "internal", Msg: "response encoding failed"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(raw)
}

// redactStageProgress applies the codor-C2 primitive to the payload-DERIVED
// strings of the stage story (drain D10). It runs per string VALUE, which is
// how the primitive is defined (§30: RedactJSON deep-walks and calls Redact per
// value), so honest stage ids and duty names are byte-unchanged and a secret
// that reached a payload's `stage`/`phase`/`name` field does not reach the
// wire. The stored row is untouched — store-raw / serve-redacted (R19).
func redactStageProgress(steps []StageStep) {
	for i := range steps {
		steps[i].Stage = redact.Redact(steps[i].Stage)
		steps[i].Kind = redact.Redact(steps[i].Kind)
		steps[i].Outcome = redact.Redact(steps[i].Outcome)
	}
}

// redactTaskLineage applies the same primitive to the one lineage member that
// is payload-DERIVED. Since migration 0022 the task→project edge resolves from
// the registry pin on the task's latest `intake.state` payload when no claim
// has been declared yet, so the served project is no longer always a relational
// column — and "enumerated by key" is not the same property as "cannot carry a
// secret" (the D10 reasoning, one field later). The task list rides
// writeReadRedacted with the rest of its body; the task detail goes out
// UNWRAPPED, so the value passes the primitive here. The stored row is
// untouched — store-raw / serve-redacted (R19).
func redactTaskLineage(lin *TaskLineage) {
	lin.Project = redact.Redact(lin.Project)
}

// redactTaskTriage applies the same primitive to the Stage-0 block, which is
// entirely payload-DERIVED: every value is lifted by key out of an
// `intake.state` body, and a floor reason's `detail` is the freest text of the
// lot — it quotes the plan step or the request phrase that tripped the floor.
// The stored row is untouched — store-raw / serve-redacted (R19).
func redactTaskTriage(tri *TaskTriage) {
	if tri == nil {
		return
	}
	tri.Family = redact.Redact(tri.Family)
	tri.FamilySource = redact.Redact(tri.FamilySource)
	tri.Tier = redact.Redact(tri.Tier)
	tri.FloorTier = redact.Redact(tri.FloorTier)
	for i := range tri.FloorReasons {
		tri.FloorReasons[i].Class = redact.Redact(tri.FloorReasons[i].Class)
		tri.FloorReasons[i].Source = redact.Redact(tri.FloorReasons[i].Source)
		tri.FloorReasons[i].Detail = redact.Redact(tri.FloorReasons[i].Detail)
	}
}

func badRequest(msg string) *SurfaceError {
	return &SurfaceError{Status: http.StatusBadRequest, Code: "bad_request", Msg: msg}
}

// readLimit parses and clamps the caller's page bound.
func readLimit(r *http.Request) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return readPageDefault, nil
	}
	n, err := readLimitValue(raw)
	if err != nil {
		return 0, err
	}
	if n > readPageMax {
		return readPageMax, nil
	}
	return n, nil
}

// readLimitValue is the shared boundary validation for `?limit=`.
func readLimitValue(raw string) (int, error) {
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, badRequest(fmt.Sprintf("bad limit %q: want a positive integer", raw))
	}
	return n, nil
}

// readPerson resolves the `?person=` filter against the caller's scope. Only
// the operator reads across owners; a member naming anyone else is refused
// rather than silently re-scoped, so the answer is never about a subject the
// caller did not ask for.
func readPerson(r *http.Request, scope ownerScope) (string, error) {
	person := strings.TrimSpace(r.URL.Query().Get("person"))
	if person == "" {
		return "", nil
	}
	if !scope.Operator && person != scope.UserID {
		return "", &SurfaceError{Status: http.StatusForbidden, Code: "forbidden",
			// S01.9: cross-person reads are the operator's.
			Msg: "reading another person's work is the household operator's to do"}
	}
	return person, nil
}

// readTimeRange parses the optional `?since=`/`?until=` RFC3339 bounds and
// NORMALIZES them to UTC.
//
// The normalization is load-bearing, not cosmetic (drain D1). Stored `*_ts`
// columns are UTC RFC3339Nano text and the filters compare them
// LEXICOGRAPHICALLY in SQL, so a bound carrying a non-UTC offset compares as
// the characters the caller typed rather than as the instant they meant:
// `2026-06-01T23:59:59+12:00` is 11:59:59Z, but as text it sorts ABOVE a row
// stored at 15:00:00Z and wrongly excludes it. Re-rendering the parsed instant
// in UTC puts both sides of every compare in one representation.
func readTimeRange(r *http.Request) (since, until string, err error) {
	for _, p := range []struct {
		name string
		dst  *string
	}{{"since", &since}, {"until", &until}} {
		v := strings.TrimSpace(r.URL.Query().Get(p.name))
		if v == "" {
			continue
		}
		t, perr := time.Parse(time.RFC3339, v)
		if perr != nil {
			return "", "", badRequest(fmt.Sprintf("bad %s %q: want an RFC3339 timestamp", p.name, v))
		}
		*p.dst = t.UTC().Format(time.RFC3339Nano)
	}
	return since, until, nil
}

// head captures the snapshot-then-tail cursor a live view resumes from (S15.3):
// the head event_seq AT PROJECTION TIME, so a client tails /events from exactly
// where the snapshot it holds stops.
func (s *Server) head(ctx context.Context) (int64, error) { return s.log.Head(ctx) }

// ── GET /api/runs — the owner-scoped run list ───────────────────────────────

// RunListItem summarizes one run from its stored S02.3 FSM row plus the landed
// derives. `wedged` is DERIVED (§30 R11), never a column. There is no percent,
// fraction, ratio or ETA field — and none may be added (R12).
type RunListItem struct {
	RunID          string     `json:"run_id"`
	Owner          string     `json:"owner"`
	TaskID         string     `json:"task_id"`
	State          string     `json:"state"`
	WaitingOnHuman bool       `json:"waiting_on_human"`
	ParkedUntil    *string    `json:"parked_until"`
	Wedged         bool       `json:"wedged"`
	Stage          string     `json:"stage"`
	Lane           string     `json:"lane"`
	Generation     int64      `json:"generation"`
	CreatedTS      time.Time  `json:"created_ts"`
	UpdatedTS      time.Time  `json:"updated_ts"`
	LastActivityTS *time.Time `json:"last_activity_ts"`
}

// RunList is the list response. Cursor is the S15.3 snapshot-then-tail cursor.
type RunList struct {
	Runs      []RunListItem `json:"runs"`
	Cursor    int64         `json:"cursor"`
	Truncated bool          `json:"truncated"`
}

func (s *Server) handleRunList(w http.ResponseWriter, r *http.Request) {
	if !s.projReady(w) {
		return
	}
	scope := s.readScope(r)
	limit, err := readLimit(r)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	person, err := readPerson(r, scope)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	since, until, err := readTimeRange(r)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" && !runStates[status] {
		s.writeSurface(w, nil, badRequest(fmt.Sprintf("unknown status %q: want a stored S02.3 state (wedged is derived, never stored)", status)))
		return
	}

	q := `SELECT run_id, user_id, COALESCE(task_id, ''), state, lane, generation, created_ts, updated_ts FROM runs WHERE 1 = 1`
	args := []any{}
	if !scope.Operator {
		q += ` AND user_id = ?`
		args = append(args, scope.UserID)
	}
	for _, f := range []struct {
		clause string
		val    string
	}{
		{` AND user_id = ?`, person},
		{` AND state = ?`, status},
		{` AND task_id = ?`, strings.TrimSpace(r.URL.Query().Get("task"))},
		{` AND created_ts >= ?`, since},
		{` AND created_ts <= ?`, until},
	} {
		if f.val != "" {
			q += f.clause
			args = append(args, f.val)
		}
	}
	q += ` ORDER BY created_ts DESC, run_id DESC LIMIT ?`
	args = append(args, limit+1) // one beyond, so Truncated is OBSERVED

	cursor, err := s.head(r.Context())
	if err != nil {
		s.writeSurface(w, nil, fmt.Errorf("read head cursor: %w", err))
		return
	}
	rows, err := s.proj.db.QueryContext(r.Context(), q, args...)
	if err != nil {
		s.writeSurface(w, nil, fmt.Errorf("read runs: %w", err))
		return
	}
	out := RunList{Runs: []RunListItem{}, Cursor: cursor}
	for rows.Next() {
		if len(out.Runs) >= limit {
			out.Truncated = true
			break
		}
		var it RunListItem
		var created, updated string
		if err := rows.Scan(&it.RunID, &it.Owner, &it.TaskID, &it.State, &it.Lane, &it.Generation, &created, &updated); err != nil {
			rows.Close()
			s.writeSurface(w, nil, fmt.Errorf("scan runs: %w", err))
			return
		}
		it.CreatedTS, it.UpdatedTS = parseTS(created), parseTS(updated)
		out.Runs = append(out.Runs, it)
	}
	// The cursor is closed EXPLICITLY, not by defer: a truncated page leaves it
	// unexhausted, and the control plane's pool is one connection (S02.1), so
	// the per-row derives below would block on a cursor still holding it.
	rows.Close()
	if err := rows.Err(); err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	for i := range out.Runs {
		s.proj.decorateRunRow(r.Context(), &out.Runs[i])
	}
	s.writeReadRedacted(w, out)
}

// decorateRunRow fills the derived members of a list row from the landed
// per-run derives (the same reads the run card uses — one derivation, two
// surfaces).
func (p *projector) decorateRunRow(ctx context.Context, it *RunListItem) {
	var openAsks int64
	if err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM asks WHERE run_id = ? AND answered_ts IS NULL`, it.RunID).Scan(&openAsks); err == nil {
		it.WaitingOnHuman = it.State == "parked" && openAsks > 0
	}
	if pay, ok := p.latestPayload(ctx, it.RunID, "limit.event", "engine.rate_limit", "run.parked"); ok {
		if v := firstString(pay, "parked_until", "parked-until", "resets_at", "resets-at", "reset_at"); v != "" {
			it.ParkedUntil = &v
		}
	}
	if pay, ok := p.latestPayloadType(ctx, it.RunID, &it.Stage, "stage.started", "stage.finished", "intake.state"); ok {
		if v := firstString(pay, "stage", "kind", "name"); v != "" {
			it.Stage = v
		}
	}
	it.Wedged = p.derivedWedged(ctx, it.RunID)
	if a, ok := p.lastActivity(ctx, it.RunID); ok {
		ts := a.TS
		it.LastActivityTS = &ts
	}
}

// ── GET /api/runs/{run} — run detail ────────────────────────────────────────

// RunEventRecord is one run_events row served as a record: its identity, its
// type and its payload. The payload is REDACTED at serialization (R19) — the
// stored row keeps the original bytes.
type RunEventRecord struct {
	Seq     int64           `json:"seq"`
	Type    string          `json:"type"`
	TS      time.Time       `json:"ts"`
	Payload json.RawMessage `json:"payload"`
}

// LiveActivityRefs is what a client needs to go live on this run (S15.3
// snapshot-then-tail): the cursor the snapshot was taken at, the topic and run
// to subscribe with, and the last-activity line the snapshot already knows.
type LiveActivityRefs struct {
	Cursor int64     `json:"cursor"`
	Topic  string    `json:"topic"`
	RunID  string    `json:"run_id"`
	Last   *Activity `json:"last"`
}

// RunDetail is the S15.2 run resource: the run card, the live-activity refs,
// the S04.4 spawn records and the S2.6 routing records.
type RunDetail struct {
	Card           RunCard          `json:"card"`
	LiveActivity   LiveActivityRefs `json:"live_activity"`
	SpawnRecords   []RunEventRecord `json:"spawn_records"`
	RoutingRecords []RunEventRecord `json:"routing_records"`
	Cursor         int64            `json:"cursor"`
}

// The record type sets served on a run detail. They are literals rather than
// imports because internal/api imports no producer package (the derive-from-log
// discipline the inbox projections already follow).
var (
	spawnRecordTypes   = []string{"helper.spawned", "spawn.refused", "orchestration.helper"}
	routingRecordTypes = []string{"routing.decided"}
)

func (s *Server) handleRunDetail(w http.ResponseWriter, r *http.Request) {
	if !s.projReady(w) {
		return
	}
	runID := r.PathValue("run")
	scope := s.readScope(r)
	card, owner, found, err := s.proj.runCard(r.Context(), runID)
	if err != nil {
		s.writeSurface(w, nil, fmt.Errorf("run card: %w", err))
		return
	}
	if code, cerr := authorizeOwner(scope, owner, found, "run"); cerr != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: code, Code: httpCode(code), Msg: cerr.Error()})
		return
	}
	cursor, err := s.head(r.Context())
	if err != nil {
		s.writeSurface(w, nil, fmt.Errorf("read head cursor: %w", err))
		return
	}
	detail := RunDetail{
		Card:   card,
		Cursor: cursor,
		LiveActivity: LiveActivityRefs{
			Cursor: cursor, Topic: topicRun, RunID: runID, Last: card.LastActivity,
		},
		SpawnRecords:   []RunEventRecord{},
		RoutingRecords: []RunEventRecord{},
	}
	if detail.SpawnRecords, err = s.proj.eventRecords(r.Context(), runID, spawnRecordTypes); err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	if detail.RoutingRecords, err = s.proj.eventRecords(r.Context(), runID, routingRecordTypes); err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	s.writeReadRedacted(w, detail)
}

// authorizeOwner is the S01.9 read check in the authorizeRun shape: 404 when
// the subject does not exist, 403 when it belongs to another owner, and any
// row for the operator. 404-before-403 is deliberate — an unknown id must not
// become an existence oracle for another owner's work.
func authorizeOwner(scope ownerScope, owner string, found bool, what string) (int, error) {
	if !found {
		return http.StatusNotFound, fmt.Errorf("%s not found", what)
	}
	if !scope.Operator && owner != scope.UserID {
		return http.StatusForbidden, errors.New("forbidden")
	}
	return 0, nil
}

func httpCode(status int) string {
	if status == http.StatusNotFound {
		return "not_found"
	}
	return "forbidden"
}

// eventRecords reads one run's rows of the named types, bounded.
func (p *projector) eventRecords(ctx context.Context, runID string, types []string) ([]RunEventRecord, error) {
	q := QueryRunEventsByType(len(types))
	args := append([]any{runID}, toAny(types)...)
	args = append(args, runDetailRecordCap)
	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("projection: run records %q: %w", runID, err)
	}
	defer rows.Close()
	out := []RunEventRecord{}
	for rows.Next() {
		var rec RunEventRecord
		var ts, payload string
		if err := rows.Scan(&rec.Seq, &rec.Type, &ts, &payload); err != nil {
			return nil, fmt.Errorf("projection: run record scan: %w", err)
		}
		rec.TS = parseTS(ts)
		rec.Payload = json.RawMessage(payload)
		out = append(out, rec)
	}
	return out, rows.Err()
}

// ── GET /api/tasks — the owner-scoped task list ─────────────────────────────

// TaskListItem is one board card: the task row, its resolved project and its
// latest run summarized (the BoardSnapshot machinery, reused).
type TaskListItem struct {
	TaskID       string       `json:"task_id"`
	Owner        string       `json:"owner"`
	Title        string       `json:"title"`
	KanbanStatus string       `json:"kanban_status"`
	Project      string       `json:"project"`
	CreatedTS    time.Time    `json:"created_ts"`
	LatestRun    *TaskListRun `json:"latest_run"`
}

// TaskListRun is the task list's latest-run summary at the CARD grain: every
// BoardRun field (embedded, so the served JSON keeps them at the same level)
// plus the S1.3 card-face facts no landed list read carried (B6-5 OQ4).
//
// Why the face is derived HERE rather than joined by the client: stage,
// waiting-on-human and parked-until live on the per-run derive, cost-so-far
// lives behind the S10.4 meter seam and the effort mode lives inside a
// routing.decided payload — the last two are reachable from no list read at
// all, so a client assembling the face would need one request per card and
// still could not answer two of the six elements (S1.3; S15.5).
//
// It is a distinct type from BoardRun on purpose: the `board` topic snapshot
// shares BoardRun and stays byte-for-byte as it was (its consumers are the SSE
// projections, which this packet does not touch).
type TaskListRun struct {
	BoardRun
	// Stage is the FSM/pipeline stage marker — the control plane's own value,
	// never a client-invented status.
	Stage          string  `json:"stage"`
	WaitingOnHuman bool    `json:"waiting_on_human"`
	ParkedUntil    *string `json:"parked_until"`
	// CostSoFarUSD is READ from the S10.4 seam and never computed here (§37,
	// the money-is-a-read rule). nil is the honest absence — no meter reading
	// for this run — and is never rendered as a zero.
	CostSoFarUSD *float64 `json:"cost_so_far_usd"`
	// EffortMode and DowngradeNote come from the latest routing.decided payload
	// (3.5 effort modes + disclosed downgrade note). Both are payload-DERIVED
	// strings and redact at the serving edge with the rest of this response.
	//
	// The note is read from an explicit DISCLOSURE key: the landed routing
	// payload has no downgrade discriminator (S10.6's ladder lands with the
	// local tier — the receipt's own mode line says so in the same words), so
	// there is nothing to infer a downgrade FROM. An absent key is an absent
	// note: the platform disclosed none, and the face says so rather than
	// dressing the mandatory plain-language routing reason up as one.
	EffortMode    string `json:"effort_mode"`
	DowngradeNote string `json:"downgrade_note,omitempty"`
	// QueueHintRank is the recorded S15.5 drag order of the task's queued runs
	// (queue.hint_rank, 0 = no hint). Read-only: it exists so a board that
	// reloads renders the order that was RECORDED rather than falling back to
	// creation order and silently losing the drag (B6-5 OQ6). nil = the task
	// has no queued run, so there is no recorded position at all.
	QueueHintRank *int64 `json:"queue_hint_rank"`
}

// TaskList is the list response with its S15.3 tail cursor.
type TaskList struct {
	Tasks     []TaskListItem `json:"tasks"`
	Cursor    int64          `json:"cursor"`
	Truncated bool           `json:"truncated"`
}

func (s *Server) handleTaskList(w http.ResponseWriter, r *http.Request) {
	if !s.projReady(w) {
		return
	}
	scope := s.readScope(r)
	limit, err := readLimit(r)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	person, err := readPerson(r, scope)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	since, until, err := readTimeRange(r)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}

	// The project linkage is the task_project view (0016, re-created by 0022 and
	// again by 0023 over the completed edge: the claims declared at plan
	// approval, else the registry pin the intake recorded at triage, else — for
	// an onboarding task, which has neither — the registry entry its own
	// `onboard-<project>` id names, so an onboarding task is attributed from
	// birth too). LEFT JOINed, so a task with no resolved project
	// renders in the honest '(no project)' bucket rather than dropping out of
	// the list (§37). The served column and the `?project=` clause below are
	// the SAME expression, so they cannot disagree.
	q := `SELECT t.task_id, t.user_id, t.title, t.kanban_status, t.created_ts,
	             COALESCE(tp.project_id, ?) AS project
	        FROM tasks t
	        LEFT JOIN task_project tp ON tp.task_id = t.task_id
	       WHERE 1 = 1`
	args := []any{noProjectBucket}
	if !scope.Operator {
		q += ` AND t.user_id = ?`
		args = append(args, scope.UserID)
	}
	for _, f := range []struct {
		clause string
		val    string
	}{
		{` AND t.user_id = ?`, person},
		{` AND t.kanban_status = ?`, strings.TrimSpace(r.URL.Query().Get("status"))},
		{` AND COALESCE(tp.project_id, ?) = ?`, strings.TrimSpace(r.URL.Query().Get("project"))},
		{` AND t.created_ts >= ?`, since},
		{` AND t.created_ts <= ?`, until},
	} {
		if f.val == "" {
			continue
		}
		q += f.clause
		if strings.Count(f.clause, "?") == 2 {
			args = append(args, noProjectBucket)
		}
		args = append(args, f.val)
	}
	q += ` ORDER BY t.created_ts DESC, t.task_id DESC LIMIT ?`
	args = append(args, limit+1)

	cursor, err := s.head(r.Context())
	if err != nil {
		s.writeSurface(w, nil, fmt.Errorf("read head cursor: %w", err))
		return
	}
	rows, err := s.proj.db.QueryContext(r.Context(), q, args...)
	if err != nil {
		s.writeSurface(w, nil, fmt.Errorf("read tasks: %w", err))
		return
	}
	out := TaskList{Tasks: []TaskListItem{}, Cursor: cursor}
	for rows.Next() {
		if len(out.Tasks) >= limit {
			out.Truncated = true
			break
		}
		var it TaskListItem
		var created string
		if err := rows.Scan(&it.TaskID, &it.Owner, &it.Title, &it.KanbanStatus, &created, &it.Project); err != nil {
			rows.Close()
			s.writeSurface(w, nil, fmt.Errorf("scan tasks: %w", err))
			return
		}
		it.CreatedTS = parseTS(created)
		out.Tasks = append(out.Tasks, it)
	}
	// Closed explicitly for the same reason as the run list: one connection.
	rows.Close()
	if err := rows.Err(); err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	for i := range out.Tasks {
		if lr, ok := s.proj.taskListRun(r.Context(), out.Tasks[i].TaskID); ok {
			out.Tasks[i].LatestRun = &lr
		}
	}
	s.writeReadRedacted(w, out)
}

// taskListRun derives one task's latest run at the S1.3 card-face grain.
//
// The five facts the board card shares with the run list come from
// decorateRunRow — the SAME derive /api/runs runs, called rather than copied,
// so the two surfaces can never disagree about what stage a run is in. The
// three that are new to this grain (cost, effort, recorded drag order) are each
// one bounded read of a landed surface.
func (p *projector) taskListRun(ctx context.Context, taskID string) (TaskListRun, bool) {
	var it RunListItem
	if err := p.db.QueryRowContext(ctx,
		`SELECT run_id, state FROM runs WHERE task_id = ? ORDER BY created_ts DESC, run_id DESC LIMIT 1`, taskID).
		Scan(&it.RunID, &it.State); err != nil {
		return TaskListRun{}, false
	}
	p.decorateRunRow(ctx, &it)
	out := TaskListRun{
		BoardRun:       BoardRun{RunID: it.RunID, State: it.State, Wedged: it.Wedged, LastActivityTS: it.LastActivityTS},
		Stage:          it.Stage,
		WaitingOnHuman: it.WaitingOnHuman,
		ParkedUntil:    it.ParkedUntil,
	}
	// Cost so far: READ through the S10.4 seam, the run-card precedent. No
	// arithmetic happens here and none may (§37). The nil-able field is what
	// makes the absence expressible, and `meterReading` is what decides there is
	// one — a run the ledger folds to zero has no price, and serving that as 0
	// put a fabricated `USD 0` on this very card.
	if m, priced := p.meterReading(ctx, it.RunID); priced {
		cost := m.APIEquivCostUSD
		out.CostSoFarUSD = &cost
	}
	if pay, ok := p.latestPayload(ctx, it.RunID, "routing.decided"); ok {
		out.EffortMode = firstString(pay, "effort", "effort_mode")
		out.DowngradeNote = firstString(pay, "downgrade_note", "downgrade", "effort_downgrade")
	}
	// A mode change is a DISCLOSED STATE, not a log line (S10.6), so the card
	// must show the mode the engine actually ran under. The routing decision
	// is the usual source, but a lane can be dispatched without one — and an
	// adapter that stamps the effort on its own terminal event is then the
	// only honest witness. It fills in, and never overwrites: where a routing
	// decision exists it is the record of what was chosen.
	if out.EffortMode == "" {
		if pay, ok := p.latestPayload(ctx, it.RunID, "engine.done"); ok {
			out.EffortMode = firstString(pay, "effort", "effort_mode")
		}
	}
	// The recorded drag order. The JOIN is read-only: the scheduler owns every
	// write to this column and its claim-loop semantics are untouched.
	var rank int64
	if err := p.db.QueryRowContext(ctx,
		`SELECT q.hint_rank FROM queue q JOIN runs r ON r.run_id = q.run_id
		  WHERE r.task_id = ? AND q.status = 'queued'
		  ORDER BY q.enqueued_ts, q.queue_id LIMIT 1`, taskID).Scan(&rank); err == nil {
		out.QueueHintRank = &rank
	}
	return out, true
}

// ── GET /api/tasks/{task} — the enriched task detail ────────────────────────

// StageStep is one entry of the per-stage story, DERIVED from the log
// (`stage.started`/`stage.finished` from internal/stage, `intake.state` from
// the intake pipeline FSM) — there is no side store and no percentage.
type StageStep struct {
	RunID   string    `json:"run_id"`
	Seq     int64     `json:"seq"`
	Type    string    `json:"type"`
	Stage   string    `json:"stage"`
	Kind    string    `json:"kind"`
	Outcome string    `json:"outcome,omitempty"`
	TS      time.Time `json:"ts"`
}

// TaskSuccessor is one edge of the S13.9 follow-up lineage.
type TaskSuccessor struct {
	TaskID        string    `json:"task_id"`
	DeliverableID string    `json:"deliverable_id"`
	RevisionN     int64     `json:"revision_n"`
	CreatedTS     time.Time `json:"created_ts"`
}

// TaskLineage is the task's place in the work: its project (via the
// task_project view — the approved claims, else the intake's registry pin, else
// the registry entry an onboarding task's own id names, since runs and tasks
// carry no project column at v0, §37) and its follow-up edges in BOTH
// directions.
type TaskLineage struct {
	Project string `json:"project"`
	// ProjectChoices > 1 means the task claimed more than one project: the
	// ambiguity is rendered, never collapsed silently (§37).
	ProjectChoices int64 `json:"project_choices"`
	// Succeeds is what THIS task was spawned from; SucceededBy is what was
	// spawned from its deliverables.
	Succeeds    []TaskSuccessor `json:"succeeds"`
	SucceededBy []TaskSuccessor `json:"succeeded_by"`
}

// TaskRunView is one of the task's runs with its materialized receipt served
// VERBATIM from receipts.usage_json (S10.10). A run with no receipt yet is an
// honest absence, not an error and not a zero.
type TaskRunView struct {
	RunID         string          `json:"run_id"`
	State         string          `json:"state"`
	CreatedTS     time.Time       `json:"created_ts"`
	Receipt       json.RawMessage `json:"receipt,omitempty"`
	ReceiptAbsent string          `json:"receipt_absent,omitempty"`
}

// TaskTriage is the S06 Stage-0 record as the owner reads it: the family and
// WHAT RESOLVED IT, the stakes tier, and the deterministic floors that were
// tripped with the reason each one gives (P3-RW-17 R8).
//
// Every value is lifted by key out of the task's latest `intake.state` payload —
// the same ordering authority `intake.Pipeline.LoadState` and migration 0022's
// pin edge use — so it redacts at this serving edge with the rest of the
// payload-derived strings on this response (the drain-D10 reasoning). Nothing
// here is derived, recomputed or re-decided: a tier this block disagreed with
// would be a second answer to a question the pipeline already answered.
type TaskTriage struct {
	Family       string `json:"family"`
	FamilySource string `json:"family_source,omitempty"`
	Tier         string `json:"tier"`
	// FloorTier is the deterministic floor ("" = none tripped).
	FloorTier    string               `json:"floor_tier,omitempty"`
	FloorReasons []intake.FloorReason `json:"floor_reasons"`
}

// TaskDetail is the S15.2 task resource: spec + numbered ACs, plan, stage
// progress, lineage and the per-run receipt view.
type TaskDetail struct {
	TaskID       string    `json:"task_id"`
	Owner        string    `json:"owner"`
	Title        string    `json:"title"`
	KanbanStatus string    `json:"kanban_status"`
	CreatedTS    time.Time `json:"created_ts"`
	Cursor       int64     `json:"cursor"`

	Spec *intake.Spec `json:"spec"`
	Plan *intake.Plan `json:"plan"`
	// ArtifactsAbsent states WHY there is no spec/plan — a task before its
	// drafting stage has none, and that is a fact about the task, not an error
	// that hides it.
	ArtifactsAbsent string `json:"artifacts_absent,omitempty"`

	StageProgress []StageStep   `json:"stage_progress"`
	Lineage       TaskLineage   `json:"lineage"`
	Runs          []TaskRunView `json:"runs"`

	// Triage is the Stage-0 record when the task HAS one, and is absent
	// otherwise — a task whose intake never classified anything (or whose state
	// event recorded no triage facts) has no triage to show, and an empty block
	// would claim one that never happened. The absence is a fact about the task,
	// exactly as `pipeline` is absent when no pipeline surface is wired.
	Triage *TaskTriage `json:"triage,omitempty"`

	// Decisions is every human decision along this task's way (S2.4), derived
	// server-side because no landed read served the Human-decision family and
	// `decision.recorded` carries NO run_id — its subject travels in the
	// payload, so "which task was this about" is a question only a join can
	// answer (B6-5 OQ5).
	//
	// WHAT IT COVERS: the Human-decision-family rows scoped to this task's runs
	// (an accepted deliverable, an intake delta answered), plus the
	// `decision.recorded` rows whose payload subject or card id names this
	// task, one of its runs, or an EFFECT of one of its runs.
	//
	// WHAT IT MISSES, deliberately and by construction:
	//
	//   - A decision keyed to none of those subjects is not a task decision. A
	//     person-level automation pause and a per-person budget edit are the
	//     live examples — real, recorded, and about the PERSON rather than any
	//     one task, so they render on the surfaces that are about a person.
	//     Widening this to "everything the owner ever decided" would put
	//     another task's approvals on this task's page.
	// WHAT IT ALSO COVERS, since P3-RW-18 D1: the CANCEL. A cold walk proved
	// the earlier posture wrong for the reader — S15.5 binds this list to
	// "every human decision along the way", and cancelling the work is the
	// most consequential one a person takes. The producers still mint no
	// decision row for it (the approvals-plane rule that cancels are never
	// double-minted is untouched); this derive RECONSTRUCTS it from the
	// records they do write (S02.2): the structured `run.state_changed`
	// detail, and — for runs cancelled before mint parity landed — the
	// human-authored `ledger.decide` entry. One human act serves as one row.
	//
	// WHAT IT STILL MISSES, deliberately: an ordinary lifecycle transition
	// (scheduler admit, park, stage completion) is run lifecycle, not a
	// decision, and appears in `runs[].state` alone. Cancels are also visible
	// in the `cancelled` kanban column (internal/stage/skeleton.go sets it)
	// but never in `stage_progress` — that derive reads only
	// stage.started/stage.finished/intake.state, and stageOutcome's
	// vocabulary is {completed, split, error}, so a cancelled queued run
	// appears in no stage row at all.
	Decisions []TaskDecision `json:"decisions"`

	// Pipeline is the intake pipeline's own task view (phase, tier, the open
	// card), passed through from the surface that owns it. Absent when no
	// pipeline surface is wired in this process.
	Pipeline json.RawMessage `json:"pipeline,omitempty"`
}

func (s *Server) handleTask(w http.ResponseWriter, r *http.Request) {
	if !s.projReady(w) {
		return
	}
	taskID := r.PathValue("task")
	scope := s.readScope(r)

	var (
		detail  TaskDetail
		created string
	)
	detail.TaskID = taskID
	err := s.proj.db.QueryRowContext(r.Context(),
		`SELECT user_id, title, kanban_status, created_ts FROM tasks WHERE task_id = ?`, taskID).
		Scan(&detail.Owner, &detail.Title, &detail.KanbanStatus, &created)
	found := !errors.Is(err, sql.ErrNoRows)
	if err != nil && found {
		s.writeSurface(w, nil, fmt.Errorf("read task: %w", err))
		return
	}
	if code, cerr := authorizeOwner(scope, detail.Owner, found, "task"); cerr != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: code, Code: httpCode(code), Msg: cerr.Error()})
		return
	}
	detail.CreatedTS = parseTS(created)
	if detail.Cursor, err = s.head(r.Context()); err != nil {
		s.writeSurface(w, nil, fmt.Errorf("read head cursor: %w", err))
		return
	}

	if detail.Runs, err = s.proj.taskRuns(r.Context(), taskID); err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	if detail.StageProgress, err = s.proj.stageProgress(r.Context(), detail.Runs); err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	if detail.Lineage, err = s.proj.taskLineage(r.Context(), taskID); err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	if detail.Decisions, err = s.proj.taskDecisions(r.Context(), taskID, detail.Owner, detail.Runs); err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	detail.Triage = s.proj.taskTriage(r.Context(), taskID)
	s.fillTaskArtifacts(r.Context(), taskID, &detail)

	// The task detail is NOT wrapped as a whole: SPEC/PLAN artifacts, receipts,
	// the pipeline's own card and the task's stored title are S13/S06 product
	// and task content, structurally exempt from the observability primitive
	// (§7-C2·2) — a task object is not an observability projection.
	//
	// Its stage-progress entries ARE run_events payload content, though: they
	// are lifted out of payload bodies by key, and "enumerated by key" is not
	// the same property as "cannot carry a secret" (drain D10). They redact at
	// this serving edge, per R20, before the unwrapped body goes out.
	redactStageProgress(detail.StageProgress)
	redactTaskLineage(&detail.Lineage)
	redactTaskDecisions(detail.Decisions)
	redactTaskTriage(detail.Triage)
	s.writeReadJSON(w, detail)
}

// fillTaskArtifacts loads the SPEC/PLAN pair through the pipeline surface that
// owns the artifact store. Every failure renders as an honest absence WITH ITS
// REASON: the task itself still reads.
func (s *Server) fillTaskArtifacts(ctx context.Context, taskID string, detail *TaskDetail) {
	if s.intake == nil {
		detail.ArtifactsAbsent = "no intake pipeline surface is wired in this process"
		return
	}
	if view, err := s.intake.Task(ctx, taskID); err == nil {
		detail.Pipeline = view
	}
	raw, err := s.intake.Artifacts(ctx, taskID)
	if err != nil {
		detail.ArtifactsAbsent = err.Error()
		return
	}
	var pair intake.Pair
	if err := json.Unmarshal(raw, &pair); err != nil {
		s.logger.Warn("read: decode task artifacts", "task", taskID, "err", err)
		detail.ArtifactsAbsent = "the stored artifact pair could not be decoded"
		return
	}
	detail.Spec, detail.Plan = &pair.Spec, &pair.Plan
}

// taskRuns lists the task's runs with their receipts served verbatim, bounded
// by the same page cap as the list surface (drain D8): a task's runs are its
// intake/compose/execute/verify legs plus any recovery successors, which is a
// handful — but an unbounded read on the shared single connection is a
// liveness risk whatever the expected shape, and expectation is not a bound.
func (p *projector) taskRuns(ctx context.Context, taskID string) ([]TaskRunView, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT r.run_id, r.state, r.created_ts, rc.usage_json
		   FROM runs r LEFT JOIN receipts rc ON rc.run_id = r.run_id
		  WHERE r.task_id = ? ORDER BY r.created_ts, r.run_id LIMIT ?`, taskID, readPageDefault)
	if err != nil {
		return nil, fmt.Errorf("projection: task runs %q: %w", taskID, err)
	}
	defer rows.Close()
	out := []TaskRunView{}
	for rows.Next() {
		var v TaskRunView
		var created string
		var usage sql.NullString
		if err := rows.Scan(&v.RunID, &v.State, &created, &usage); err != nil {
			return nil, fmt.Errorf("projection: task run scan: %w", err)
		}
		v.CreatedTS = parseTS(created)
		if usage.Valid && usage.String != "" {
			v.Receipt = json.RawMessage(usage.String)
		} else {
			// S10.1: a receipt materializes at the run's terminal transition.
			v.ReceiptAbsent = "no receipt yet — one is written when the work finishes"
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// stageProgress derives the per-stage story from the log across the task's
// runs: the stage boundaries internal/stage mints plus the intake pipeline's
// own phase rows. Derive-from-log, no side store (D6).
func (p *projector) stageProgress(ctx context.Context, runs []TaskRunView) ([]StageStep, error) {
	out := []StageStep{}
	for _, rv := range runs {
		q := QueryRunEventsByType(len(stageProgressTypes))
		args := append([]any{rv.RunID}, toAny(stageProgressTypes)...)
		args = append(args, runDetailRecordCap)
		rows, err := p.db.QueryContext(ctx, q, args...)
		if err != nil {
			return nil, fmt.Errorf("projection: stage progress %q: %w", rv.RunID, err)
		}
		for rows.Next() {
			st := StageStep{RunID: rv.RunID}
			var ts, payload string
			if err := rows.Scan(&st.Seq, &st.Type, &ts, &payload); err != nil {
				rows.Close()
				return nil, fmt.Errorf("projection: stage progress scan: %w", err)
			}
			st.TS = parseTS(ts)
			pay := json.RawMessage(payload)
			st.Stage = firstString(pay, "stage", "phase", "name")
			st.Kind = firstString(pay, "kind")
			st.Outcome = firstString(pay, "outcome")
			out = append(out, st)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// stageProgressTypes are the family-1 markers the per-stage story reads: the
// engine stage boundaries and the intake pipeline's own phases.
var stageProgressTypes = []string{"stage.started", "stage.finished", "intake.state"}

// taskTriage derives the S06 Stage-0 record from the task's LATEST
// `intake.state` event (P3-RW-17 R8) — the ordering authority
// `intake.Pipeline.LoadState` and migration 0022's pin edge both use, for the
// same reason: the whole State is marshaled onto every state event, so the
// newest one is the record of what triage resolved.
//
// The block is served only when the record HOLDS triage facts. A task whose
// intake never classified anything, and a state event written before a family or
// tier existed, get no block at all: an empty one would render a triage that
// never happened, which is worse than an absence a surface can say nothing
// about. Nothing here re-decides anything — a tier this read disagreed with
// would be a second answer to a question S06 already answered.
func (p *projector) taskTriage(ctx context.Context, taskID string) *TaskTriage {
	var payload string
	// No row, or an unreadable one, is an ABSENT block rather than a failed task
	// read — the landed latestPayload posture for every payload-derived enrichment
	// on this response: the task itself still reads.
	if err := p.db.QueryRowContext(ctx, QueryTaskIntakeState, taskID).Scan(&payload); err != nil {
		return nil
	}
	var st struct {
		Family       string               `json:"family"`
		FamilySource string               `json:"family_source"`
		Tier         string               `json:"tier"`
		FloorTier    string               `json:"floor_tier"`
		FloorReasons []intake.FloorReason `json:"floor_reasons"`
	}
	if err := json.Unmarshal([]byte(payload), &st); err != nil {
		return nil
	}
	if st.Family == "" && st.Tier == "" && st.FloorTier == "" && len(st.FloorReasons) == 0 {
		return nil
	}
	tri := &TaskTriage{
		Family: st.Family, FamilySource: st.FamilySource, Tier: st.Tier,
		FloorTier: st.FloorTier, FloorReasons: st.FloorReasons,
	}
	if tri.FloorReasons == nil {
		tri.FloorReasons = []intake.FloorReason{}
	}
	return tri
}

// The S02.2 Human-decision family, split by HOW ITS PRODUCERS ACTUALLY MINT
// (drain r1 D1/D2/D3 — the first cut assumed one shape and missed two).
// Literals for the same reason the spawn and routing record sets are:
// internal/api imports no producer package.
//
//   - runScopedDecisionTypes carry a run_id, so they are found BY RUN.
//     `intake.delta_decision` is the one real member: internal/intake appends
//     it on the run whose delta card was answered.
//   - platformDecisionTypes carry NO run_id and name their subject in the
//     payload, so each needs its own way back to a task. `decision.recorded`
//     names it as a subject/card-id; `deliverable.accepted` names a
//     DELIVERABLE, whose own row carries the task linkage.
//
// A CANCEL mints no member of this family and never will — internal/api/
// approvals.go's rule stands: "cancels (carried by run.state_changed) are
// never double-minted". It is served all the same, by the fourth read below
// (cancelDecisions), which RECONSTRUCTS the decision from the durable cancel
// records rather than asking for a row nobody writes (P3-RW-18 D1; S02.2).
var (
	runScopedDecisionTypes  = []string{"intake.delta_decision", "decision.recorded"}
	platformDecisionTypes   = []string{"decision.recorded", "deliverable.accepted"}
	deliverableAcceptedType = "deliverable.accepted"
)

// The cancel-reconstruction literals. Cross-package literals for the same
// reason every other record set here is one: internal/api imports no producer
// package. Each has exactly one producer-side definition, cited so drift is
// findable from either end.
//
//   - runStateChangedType / humanCancelCause: internal/run's EventState and
//     CancelCauseHuman (internal/run/canceldetail.go). All four cancel mint
//     paths write the cause through run.CancelDetail — the one constructor.
//   - ledgerUpdateType / ledgerDecideVerb: internal/ledger's EventLedgerUpdate
//     and VerbDecide (internal/ledger/store.go).
//   - cancelLedgerPrefixes: the opening words of the three HUMAN-authored
//     ledger cancel mints — internal/stage/answer.go's answerCancel,
//     internal/stage/ladderanswer.go's ladderCancel, and internal/intake/
//     answer.go's Cancel and rethink legs. `ledger.decide` also carries
//     retries, accepts and revisions, so the entry text is what separates a
//     cancel from the rest of the family. Each prefix is pinned at its
//     producer by a mint-side test (cancelmint_rw18_test.go in stage and intake),
//     so a reworded mint fails there rather than silently emptying this leg
//     (P3-RW-18 OQ1; the durable fix is a typed `kind` on ledger decisions,
//     a ledger schema change left to a future packet).
const (
	runStateChangedType = "run.state_changed"
	humanCancelCause    = "human cancel (4.5)"
	ledgerUpdateType    = "ledger_update"
	ledgerDecideVerb    = "ledger.decide"
	ledgerAuthorHuman   = "human"
)

var cancelLedgerPrefixes = []string{"requester cancelled", "requester: rethink"}

// TaskDecision is one human decision on this task's way: who decided what,
// about which card, and when (S2.4 — "actor, card id + type, decision,
// presented-at → decided-at"). Every string is lifted out of a payload by key
// and REDACTS at the serving edge, exactly as the stage story does.
type TaskDecision struct {
	Seq             int64     `json:"seq"`
	Type            string    `json:"type"`
	TS              time.Time `json:"ts"`
	RunID           string    `json:"run_id,omitempty"`
	Actor           string    `json:"actor"`
	ActorIsOperator bool      `json:"actor_is_operator,omitempty"`
	CardID          string    `json:"card_id"`
	CardType        string    `json:"card_type"`
	Decision        string    `json:"decision"`
	Subject         string    `json:"subject,omitempty"`
	Reason          string    `json:"reason,omitempty"`
	// HumanReason is the person's own words for a cancel, lifted from the one
	// constructor's `$.detail.reason` (P3-RW-19 R7). Absence is STRUCTURAL: no
	// key at all when no reason was given, so the view draws its honest line
	// from the absence rather than from a blank that would claim a motive
	// nobody gave. Reason (the record's own mechanical sentence) is untouched
	// — this field is additive.
	HumanReason string `json:"human_reason,omitempty"`
	DecidedAt   string `json:"decided_at,omitempty"`
}

// redactTaskDecisions applies the codor-C2 primitive per lifted VALUE, the
// redactStageProgress precedent: these fields are run_events payload content on
// a response that goes out unwrapped, and "enumerated by key" is not the same
// property as "cannot carry a secret" (§38 D10).
func redactTaskDecisions(rows []TaskDecision) {
	for i := range rows {
		rows[i].Actor = redact.Redact(rows[i].Actor)
		rows[i].CardID = redact.Redact(rows[i].CardID)
		rows[i].CardType = redact.Redact(rows[i].CardType)
		rows[i].Decision = redact.Redact(rows[i].Decision)
		rows[i].Subject = redact.Redact(rows[i].Subject)
		rows[i].Reason = redact.Redact(rows[i].Reason)
		rows[i].HumanReason = redact.Redact(rows[i].HumanReason)
	}
}

// taskDecisions derives the S2.4 story.
//
// THREE reads, because the family's producers mint three different ways and a
// single shape misses two of them (drain r1 D1/D2):
//
//  1. run-scoped rows, found by run — `intake.delta_decision` is the real one;
//  2. `decision.recorded`, platform-scoped with its subject in the payload,
//     matched against everything this task IS (itself, its runs, its effects);
//  3. `deliverable.accepted`, platform-scoped and naming a DELIVERABLE rather
//     than a task or a run — so its way back is the deliverables table's own
//     task linkage. Nothing in its payload names the task, which is why the
//     first cut of this derive silently returned nothing for every real accept.
//
// FOUR now: the cancel, which mints no decision row at all and is reconstructed
// per run from the records the cancel itself wrote (cancelDecisions below,
// P3-RW-18 D1). It is the one leg that reads a lifecycle event, and only ever
// through a discriminator no ordinary transition carries.
func (p *projector) taskDecisions(ctx context.Context, taskID, owner string, runs []TaskRunView) ([]TaskDecision, error) {
	out := []TaskDecision{}
	for _, rv := range runs {
		q := QueryRunEventsByTypeOwned(len(runScopedDecisionTypes))
		args := append([]any{rv.RunID}, toAny(runScopedDecisionTypes)...)
		args = append(args, runDetailRecordCap)
		rows, err := p.db.QueryContext(ctx, q, args...)
		if err != nil {
			return nil, fmt.Errorf("projection: task decisions %q: %w", rv.RunID, err)
		}
		for rows.Next() {
			d, serr := scanDecision(rows)
			if serr != nil {
				rows.Close()
				return nil, serr
			}
			d.RunID = rv.RunID
			out = append(out, d)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}

		cancels, cerr := p.cancelDecisions(ctx, rv.RunID)
		if cerr != nil {
			return nil, cerr
		}
		out = append(out, cancels...)
	}

	subjects, err := p.taskSubjects(ctx, taskID, runs)
	if err != nil {
		return nil, err
	}
	set := placeholders(len(subjects))
	// The card id is "<kind>:<native-id>", so the native half is matched too —
	// an effect approval names `effect:<effect_id>`, and the effect is what
	// ties it to this task's run.
	q := `SELECT event_seq, type, ts, payload, user_id FROM run_events
	       WHERE type = ? AND user_id = ? AND run_id IS NULL
	         AND (json_extract(payload, '$.subject') IN (` + set + `)
	           OR json_extract(payload, '$.card_id') IN (` + set + `)
	           OR substr(json_extract(payload, '$.card_id'),
	                     instr(json_extract(payload, '$.card_id'), ':') + 1) IN (` + set + `))
	       ORDER BY event_seq LIMIT ?`
	args := []any{EventDecisionRecorded, owner}
	for i := 0; i < 3; i++ {
		args = append(args, toAny(subjects)...)
	}
	args = append(args, runDetailRecordCap)
	if err := p.appendDecisions(ctx, &out, q, args, "decision subjects", taskID); err != nil {
		return nil, err
	}

	// The accepts. internal/review appends these platform-scoped, with the
	// deliverable id in the payload and NOTHING naming the task — so the join
	// back is the deliverable's own row. `accepted_by` is the "who".
	acceptQ := `SELECT e.event_seq, e.type, e.ts, e.payload, e.user_id
	              FROM run_events e
	              JOIN deliverables d
	                ON d.deliverable_id = json_extract(e.payload, '$.deliverable_id')
	             WHERE e.type = ? AND e.run_id IS NULL AND d.task_id = ? AND d.user_id = ?
	             ORDER BY e.event_seq LIMIT ?`
	if err := p.appendDecisions(ctx, &out,
		acceptQ, []any{deliverableAcceptedType, taskID, owner, runDetailRecordCap}, "task accepts", taskID); err != nil {
		return nil, err
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	if len(out) > runDetailRecordCap {
		out = out[:runDetailRecordCap]
	}
	return out, nil
}

// appendDecisions runs one decision query and folds its rows in.
func (p *projector) appendDecisions(ctx context.Context, out *[]TaskDecision, q string, args []any, what, taskID string) error {
	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("projection: task %s %q: %w", what, taskID, err)
	}
	defer rows.Close()
	for rows.Next() {
		d, serr := scanDecision(rows)
		if serr != nil {
			return serr
		}
		*out = append(*out, d)
	}
	return rows.Err()
}

// cancelDecisions reconstructs one run's human cancel (P3-RW-18 D1-R2/R3/R4).
//
// The S02.2 posture: the act is already durably recorded — twice over on some
// paths — so the decision is DERIVED from those records rather than a producer
// minting a second row for the reader's benefit. Nothing here writes anything.
//
// TWO legs, because the four cancel mint paths leave two record shapes and the
// walk world holds one of each:
//
//	A. the structured transition — a `run.state_changed` whose detail names the
//	   cause (run.CancelDetail, written by every mint path since D1-R1). Who,
//	   why and which card all come off the record: detail.actor, the
//	   transition's own reason, and detail.ask_id where a card was answered.
//	B. the ledger record — a `ledger.decide` update whose NEWEST entry is
//	   authored `human` and opens with a cancel's words. This is what serves
//	   runs cancelled before mint parity landed, the live walk's own
//	   t-fe5ff6c325967c3e among them, without rewriting a single row of
//	   history.
//
// ONE ACT, ONE ROW: a post-parity card cancel writes BOTH records, so leg B is
// read only for runs leg A found nothing on. The structured record wins because
// it names the actor at the transition itself.
//
// Neither leg fills card_type. The transition detail records the ask id, not
// the kind of card it was — and a cancel taken through the verbs was answered
// at no card at all. Naming a card type here would be this derive inventing
// one, which is the failure the derive exists to end.
//
// The derive is PER RUN, so a task-level cancel that ended three live runs
// serves three rows, one per run, each naming its run. That is deliberate:
// each transition is its own record of work that was stopped, and collapsing
// them would mean this derive deciding which recorded endings the reader may
// see. "One act, one row" is a rule about the two records of ONE run's cancel,
// not about the several runs one verb ends.
func (p *projector) cancelDecisions(ctx context.Context, runID string) ([]TaskDecision, error) {
	structured, err := p.structuredCancels(ctx, runID)
	if err != nil {
		return nil, err
	}
	if len(structured) > 0 {
		return structured, nil
	}
	return p.ledgerCancels(ctx, runID)
}

// structuredCancels is leg A. The discriminator is `$.detail.cause`, a value
// only run.CancelDetail writes — an ordinary lifecycle transition (admit,
// claim, park, stage completion) carries no detail.cause at all, so no amount
// of ordinary run traffic can surface here.
func (p *projector) structuredCancels(ctx context.Context, runID string) ([]TaskDecision, error) {
	const q = `SELECT event_seq, ts,
	                  COALESCE(json_extract(payload, '$.detail.actor'), json_extract(payload, '$.actor'), '') AS actor,
	                  COALESCE(json_extract(payload, '$.reason'), '')                                        AS reason,
	                  COALESCE(json_extract(payload, '$.detail.ask_id'), '')                                 AS ask_id,
	                  COALESCE(json_extract(payload, '$.detail.reason'), '')                                 AS human_reason
	             FROM run_events
	            WHERE run_id = ? AND type = ? AND json_extract(payload, '$.detail.cause') = ?
	            ORDER BY event_seq LIMIT ?`
	rows, err := p.db.QueryContext(ctx, q, runID, runStateChangedType, humanCancelCause, runDetailRecordCap)
	if err != nil {
		return nil, fmt.Errorf("projection: run cancels %q: %w", runID, err)
	}
	defer rows.Close()
	var out []TaskDecision
	for rows.Next() {
		var (
			d                  TaskDecision
			ts, actor, reason  string
			askID, humanReason string
		)
		if err := rows.Scan(&d.Seq, &ts, &actor, &reason, &askID, &humanReason); err != nil {
			return nil, fmt.Errorf("projection: run cancel scan %q: %w", runID, err)
		}
		d.Type, d.TS, d.RunID = runStateChangedType, parseTS(ts), runID
		d.Actor, d.Reason, d.CardID, d.Decision = actor, reason, askID, "cancel"
		d.HumanReason = humanReason
		out = append(out, d)
	}
	return out, rows.Err()
}

// ledgerCancels is leg B: the pre-parity record, and the only place a cancel
// taken before D1-R1 says who took it.
//
// Each `ledger.decide` update carries the whole ledger, so the entry this
// update ADDED is the last one — the same reading internal/ledger's own
// current-document read takes. An entry qualifies only when it is authored by
// a human AND opens with one of the three mints' cancel words; `ledger.decide`
// is also how retries, accepts and revisions are recorded, and none of those
// are cancels.
//
// This leg serves NO human reason (P3-RW-19 R7). A pre-parity record holds no
// motive — nothing captured one — and the "— note: …" suffix some of these
// entries carry is prose, not a field. Scraping a motive out of a sentence
// would be this derive inventing structure, which is the failure it exists to
// end; the absent key is the honest answer.
func (p *projector) ledgerCancels(ctx context.Context, runID string) ([]TaskDecision, error) {
	const q = `SELECT event_seq, ts, payload FROM run_events
	            WHERE run_id = ? AND type = ? AND json_extract(payload, '$.change.verb') = ?
	            ORDER BY event_seq LIMIT ?`
	rows, err := p.db.QueryContext(ctx, q, runID, ledgerUpdateType, ledgerDecideVerb, runDetailRecordCap)
	if err != nil {
		return nil, fmt.Errorf("projection: run ledger cancels %q: %w", runID, err)
	}
	defer rows.Close()
	var out []TaskDecision
	seen := map[string]bool{}
	for rows.Next() {
		var (
			seq     int64
			ts, raw string
		)
		if err := rows.Scan(&seq, &ts, &raw); err != nil {
			return nil, fmt.Errorf("projection: run ledger cancel scan %q: %w", runID, err)
		}
		var pay struct {
			Change struct {
				Actor string `json:"actor"`
			} `json:"change"`
			Ledger struct {
				Decisions []struct {
					Seq    int64  `json:"seq"`
					TS     string `json:"ts"`
					Author string `json:"author"`
					Text   string `json:"text"`
					Reason string `json:"reason"`
				} `json:"decisions"`
			} `json:"ledger"`
		}
		// An undecodable payload is not this derive's to report: the ledger
		// surfaces own that error, and a task page must not 500 because one
		// event body is odd. It simply carries no cancel.
		if err := json.Unmarshal([]byte(raw), &pay); err != nil {
			continue
		}
		if len(pay.Ledger.Decisions) == 0 {
			continue
		}
		e := pay.Ledger.Decisions[len(pay.Ledger.Decisions)-1]
		if e.Author != ledgerAuthorHuman || !isCancelLedgerText(e.Text) {
			continue
		}
		// The same entry can ride more than one update payload (every later
		// snapshot carries it too, and a re-answer re-appends). One entry is
		// one act, keyed by the entry's own append seq.
		key := fmt.Sprintf("%d|%s|%s", e.Seq, e.TS, e.Text)
		if seen[key] {
			continue
		}
		seen[key] = true
		d := TaskDecision{
			Seq: seq, Type: ledgerUpdateType, RunID: runID,
			Actor: pay.Change.Actor, Decision: "cancel", Reason: e.Reason,
		}
		if d.TS = parseTS(e.TS); d.TS.IsZero() {
			d.TS = parseTS(ts)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// isCancelLedgerText reports whether a human ledger entry's text is one of the
// cancel mints (see cancelLedgerPrefixes for the producers and the tests that
// pin them).
func isCancelLedgerText(text string) bool {
	t := strings.ToLower(strings.TrimSpace(text))
	for _, p := range cancelLedgerPrefixes {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return false
}

// taskSubjects is everything a decision could name to be ABOUT this task: the
// task, its runs, and the effects those runs proposed.
func (p *projector) taskSubjects(ctx context.Context, taskID string, runs []TaskRunView) ([]string, error) {
	subjects := []string{taskID}
	if len(runs) == 0 {
		return subjects, nil
	}
	ids := make([]string, 0, len(runs))
	for _, rv := range runs {
		subjects = append(subjects, rv.RunID)
		ids = append(ids, rv.RunID)
	}
	rows, err := p.db.QueryContext(ctx,
		`SELECT effect_id FROM effects WHERE run_id IN (`+placeholders(len(ids))+`) ORDER BY effect_id LIMIT ?`,
		append(toAny(ids), runDetailRecordCap)...)
	if err != nil {
		return nil, fmt.Errorf("projection: task effects %q: %w", taskID, err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("projection: task effect scan: %w", err)
		}
		subjects = append(subjects, id)
	}
	return subjects, rows.Err()
}

func scanDecision(rows *sql.Rows) (TaskDecision, error) {
	var (
		d               TaskDecision
		ts, payloa, who string
	)
	if err := rows.Scan(&d.Seq, &d.Type, &ts, &payloa, &who); err != nil {
		return d, fmt.Errorf("projection: task decision scan: %w", err)
	}
	d.TS = parseTS(ts)
	pay := json.RawMessage(payloa)
	// "Who" comes from the payload where a producer names an actor, and falls
	// back to the ROW's owner attribution where none does (drain r1 D2):
	// `intake.delta_decision` carries a measurement hook with no actor key at
	// all, and rendering an empty "who" on the one real run-scoped producer
	// would have been the family's whole point going missing.
	d.Actor = firstString(pay, "actor", "approved_by", "accepted_by")
	if d.Actor == "" {
		d.Actor = who
	}
	d.CardID = firstString(pay, "card_id")
	d.CardType = firstString(pay, "card_type")
	d.Decision = firstString(pay, "decision", "answer", "verdict")
	d.Subject = firstString(pay, "subject")
	// Where the payload does not name the act, the TYPE does — and naming it
	// from the type is reading the log, not inventing (drain r1 D1/D2). A real
	// `deliverable.accepted` carries {deliverable_id, revision, project_id,
	// accepted_by, superseded} and no card/decision keys at all, so without
	// this the family's own terminal act renders as three blanks.
	switch d.Type {
	case deliverableAcceptedType:
		if id := firstString(pay, "deliverable_id"); id != "" {
			if d.CardID == "" {
				d.CardID = "deliverable:" + id
			}
			if d.Subject == "" {
				d.Subject = id
			}
		}
		if d.CardType == "" {
			d.CardType = "deliverable"
		}
		if d.Decision == "" {
			d.Decision = "accept"
		}
	case "intake.delta_decision":
		if id := firstString(pay, "delta_id"); id != "" && d.CardID == "" {
			d.CardID = "delta:" + id
		}
		if d.CardType == "" {
			d.CardType = "intake_delta"
		}
		if d.Subject == "" {
			d.Subject = firstString(pay, "task_id")
		}
	}
	d.Reason = firstString(pay, "reason")
	d.DecidedAt = firstString(pay, "decided_at")
	d.ActorIsOperator = firstBool(pay, "actor_is_operator")
	return d, nil
}

// taskLineage reads the project linkage and both directions of the S13.9
// follow-up edges.
func (p *projector) taskLineage(ctx context.Context, taskID string) (TaskLineage, error) {
	lin := TaskLineage{Project: noProjectBucket, ProjectChoices: 1, Succeeds: []TaskSuccessor{}, SucceededBy: []TaskSuccessor{}}
	var project string
	var choices int64
	err := p.db.QueryRowContext(ctx,
		`SELECT project_id, project_choices FROM task_project WHERE task_id = ?`, taskID).Scan(&project, &choices)
	switch {
	case err == nil:
		lin.Project, lin.ProjectChoices = project, choices
	case errors.Is(err, sql.ErrNoRows):
		// The honest bucket: no registry-resolved project (§37).
	default:
		return lin, fmt.Errorf("projection: task project %q: %w", taskID, err)
	}

	// What this task succeeded: its own row in task_successor_of.
	var (
		deliverable string
		revision    int64
		created     string
	)
	err = p.db.QueryRowContext(ctx,
		`SELECT deliverable_id, revision_n, created_ts FROM task_successor_of WHERE task_id = ?`, taskID).
		Scan(&deliverable, &revision, &created)
	switch {
	case err == nil:
		lin.Succeeds = append(lin.Succeeds, TaskSuccessor{
			TaskID: taskID, DeliverableID: deliverable, RevisionN: revision, CreatedTS: parseTS(created)})
	case errors.Is(err, sql.ErrNoRows):
	default:
		return lin, fmt.Errorf("projection: task predecessor %q: %w", taskID, err)
	}

	// What succeeded it: rows pointing at revisions of THIS task's
	// deliverables (the index runs both directions, 0009).
	rows, err := p.db.QueryContext(ctx,
		`SELECT s.task_id, s.deliverable_id, s.revision_n, s.created_ts
		   FROM task_successor_of s
		   JOIN deliverables d ON d.deliverable_id = s.deliverable_id
		  WHERE d.task_id = ? ORDER BY s.created_ts, s.task_id`, taskID)
	if err != nil {
		return lin, fmt.Errorf("projection: task successors %q: %w", taskID, err)
	}
	defer rows.Close()
	for rows.Next() {
		var suc TaskSuccessor
		var ts string
		if err := rows.Scan(&suc.TaskID, &suc.DeliverableID, &suc.RevisionN, &ts); err != nil {
			return lin, fmt.Errorf("projection: task successor scan: %w", err)
		}
		suc.CreatedTS = parseTS(ts)
		lin.SucceededBy = append(lin.SucceededBy, suc)
	}
	return lin, rows.Err()
}

// ── the receipt read, owner-scoped ──────────────────────────────────────────

// GET /api/runs/{run}/receipt — the per-run materialized receipt (S10.10),
// served by the pipeline surface verbatim and now scoped to its owner. The
// B2-4 walking-skeleton form of this route answered any authenticated identity;
// that was a pre-multi-owner placeholder (§30 OQ1 precedent) and this closes it.
func (s *Server) handleRunReceipt(w http.ResponseWriter, r *http.Request) {
	if !s.projReady(w) || !s.surfaceReady(w) {
		return
	}
	runID := r.PathValue("run")
	var owner string
	err := s.proj.db.QueryRowContext(r.Context(), `SELECT user_id FROM runs WHERE run_id = ?`, runID).Scan(&owner)
	found := !errors.Is(err, sql.ErrNoRows)
	if err != nil && found {
		s.writeSurface(w, nil, fmt.Errorf("read run owner: %w", err))
		return
	}
	if code, cerr := authorizeOwner(s.readScope(r), owner, found, "run"); cerr != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: code, Code: httpCode(code), Msg: cerr.Error()})
		return
	}
	payload, err := s.intake.Receipt(r.Context(), runID)
	s.writeSurface(w, payload, err)
}

// ── The new run_events derive shape, as DATA (the §36 D7 discipline) ────────

// QueryRunEventsByType is the per-run, type-filtered record shape the run
// detail and the stage-progress derivation run. It is exported so the EXPLAIN
// test asserts the PRODUCTION text rather than a paraphrase of it — a
// paraphrase cannot catch a production query edited into a scan.
//
// WHICH index serves it depends on the type count, and the first cut of this
// comment got it wrong (drain D3). Measured by EXPLAIN QUERY PLAN:
//
//   - the ONE-type form seeks through migration 0015's run_events_run_type_idx
//     (run_id=? AND type=?) — both columns are equality-usable;
//   - the MULTI-type form (`type IN (?,?,?)`, which is what the production
//     spawn-record and stage-progress reads use) seeks through migration
//     0001's run_events_run_idx (run_id=?), with the type set as a residual
//     filter inside the one run's rows.
//
// Both SEEK, which is the property that matters, and no new index is needed —
// so migration 0017 is not opened. Crediting 0015 for the multi-type form
// would have been a false attribution of the kind §36 D7 exists to prevent,
// and the index test now pins FULL index names so a misattribution fails.
func QueryRunEventsByType(types int) string {
	return `SELECT event_seq, type, ts, payload FROM run_events WHERE run_id = ? AND type IN (` +
		placeholders(types) + `) ORDER BY event_seq LIMIT ?`
}

// QueryRunEventsByTypeOwned is the same shape carrying the row's OWNER
// attribution as well — the decision derive needs it because a producer that
// names no actor in its payload still records whose act it was (drain r1 D2).
// Same index behavior as above; the extra column is a projection, not a
// predicate.
func QueryRunEventsByTypeOwned(types int) string {
	return `SELECT event_seq, type, ts, payload, user_id FROM run_events WHERE run_id = ? AND type IN (` +
		placeholders(types) + `) ORDER BY event_seq LIMIT ?`
}
