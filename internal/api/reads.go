package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/redact"
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
			Msg: "reading another person's work is the operator's (S01.9)"}
	}
	return person, nil
}

// readTimeRange parses the optional `?since=`/`?until=` RFC3339 bounds.
func readTimeRange(r *http.Request) (since, until string, err error) {
	for _, p := range []struct {
		name string
		dst  *string
	}{{"since", &since}, {"until", &until}} {
		v := strings.TrimSpace(r.URL.Query().Get(p.name))
		if v == "" {
			continue
		}
		if _, perr := time.Parse(time.RFC3339, v); perr != nil {
			return "", "", badRequest(fmt.Sprintf("bad %s %q: want an RFC3339 timestamp", p.name, v))
		}
		*p.dst = v
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
	TaskID       string    `json:"task_id"`
	Owner        string    `json:"owner"`
	Title        string    `json:"title"`
	KanbanStatus string    `json:"kanban_status"`
	Project      string    `json:"project"`
	CreatedTS    time.Time `json:"created_ts"`
	LatestRun    *BoardRun `json:"latest_run"`
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

	// The project linkage is the 0016 view, LEFT JOINed so a task with no
	// registry-resolved project renders in the honest '(no project)' bucket
	// rather than dropping out of the list (§37).
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
		if lr, ok := s.proj.latestRunForTask(r.Context(), out.Tasks[i].TaskID); ok {
			out.Tasks[i].LatestRun = &lr
		}
	}
	s.writeReadRedacted(w, out)
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

// TaskLineage is the task's place in the work: its project (via the 0016
// task_project view over artifact_claims — the only populated relational edge
// at v0, §37) and its follow-up edges in BOTH directions.
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
	s.fillTaskArtifacts(r.Context(), taskID, &detail)

	// Not redacted: SPEC/PLAN artifacts, receipts and the pipeline's own card
	// are S13/S06 product content, structurally exempt from the observability
	// primitive (§7-C2·2). The payload-derived members here are enumerated
	// structural fields — a stage id, a duty name, a sha256, an outcome from a
	// closed vocabulary — never a payload body served verbatim.
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

// taskRuns lists the task's runs with their receipts served verbatim.
func (p *projector) taskRuns(ctx context.Context, taskID string) ([]TaskRunView, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT r.run_id, r.state, r.created_ts, rc.usage_json
		   FROM runs r LEFT JOIN receipts rc ON rc.run_id = r.run_id
		  WHERE r.task_id = ? ORDER BY r.created_ts, r.run_id`, taskID)
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
			v.ReceiptAbsent = "no receipt yet — receipts materialize at the run's terminal transition (S10.1)"
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
// It is served by migration 0015's run_events_run_type_idx (run_id, type,
// event_seq): the index leads with run_id, filters on type, and delivers
// event_seq order, which is the whole shape. No new index is needed and
// migration 0017 is therefore not opened.
func QueryRunEventsByType(types int) string {
	return `SELECT event_seq, type, ts, payload FROM run_events WHERE run_id = ? AND type IN (` +
		placeholders(types) + `) ORDER BY event_seq LIMIT ?`
}
