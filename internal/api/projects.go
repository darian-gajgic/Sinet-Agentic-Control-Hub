package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// projects.go — the S13.7 projects family (P3-RW-2): the read door onto the
// project/repository registry, and the door that starts an onboarding.
//
// THE FAMILY IS ADDITIVE (S15.2). The S15.2 table has no projects row; this
// family lands under that section's contract RULES exactly as `/api/chat` and
// `/api/push` did — owner-scoped server-side, no version prefix, additive-first,
// no outward-effect verb — and the cosmetic table amendment naming the row is
// carried to the gate list rather than written here (S00.9).
//
// THREE DOORS AND NO FOURTH. The read pair serves what the caller may see; the
// create door starts the S13.7 onboarding task. There is NO activation verb
// here: onboarding is approved on the landed `POST /api/asks/{ask}/answer`
// path, which routes an `onboard:` ask to the owner-only D10 activation, and a
// second door onto that act is exactly the double-mint shape in transport form.
// Re-scan on demand (S13.7), member management and deletion are DELIBERATE
// absences of this packet, not oversights.
//
// THE REGISTRY IS READ, NEVER WRITTEN, AND NEVER IMPORTED. internal/api imports
// neither internal/project nor internal/broker (the §40-B wall, AST-pinned):
// the entries are read as ONE bounded SELECT over repo_registry and its capture
// table, and every mutation — the pending entry, its immutable capture, the
// activation, and the `registry.*` events that record them — is performed by
// internal/project through the onboarding seam. This file mints no event and
// writes no registry SQL; the store's own rows and events ARE the audit (§39
// OQ8: an act with a landed canonical row is never double-minted).
//
// VISIBILITY IS THE CONTENT LINE (brief OQ3). A caller sees the projects they
// own and the projects they were invited to, and nobody else's — the landed
// visibleTo/PinForIntake discipline re-expressed once, here, as SQL. The
// operator role bit opens NOTHING: a project's captured conventions, commands
// and danger zones are project CONTENT, the same line the memory family draws
// (§40-C OQ5) and the chat family after it (§44), and the operator's oversight
// rides the telemetry surfaces rather than this door. The predicate takes no
// role parameter at all, so there is no operator limb to forget to omit.
//
// SERVED CONTENT IS UNWRAPPED. A capture is S13 product read from S13's OWN
// tables — content, not an observability projection lifted out of a run_events
// payload (§38 ruling (b)) — so nothing here touches internal/redact and the
// enumerated redaction edge is unchanged.
//
// NO AGGREGATES AND NO MONEY. No task count, no spend figure and no receipt
// join rides any shape below: those are client-side derivations over rows the
// API already serves. The capture summary's two counts are counts of the
// CAPTURE'S OWN content (how many conventions, how many danger zones), which is
// the S15.3 snapshot economy the list read exists for — never a figure about
// work or cost.

// projectsListCap bounds the list. Structural, not ⚙ — S13.7/S18 ratify no such
// key and this packet declares none — and it is the same number as
// readPageDefault because this is one read surface with one page size (the
// §38/§40 precedent, interim under the standing settings-tab directive).
const projectsListCap = readPageDefault

// The S13.7 lifecycle values, as migration 0008's own CHECK constraint stores
// them. They are read here as DATA (the state a row carries), never re-derived:
// pending→active is one-way and only the owner's D10 approval crosses it.
const (
	projectStatePending = "pending"
	projectStateActive  = "active"
)

// ── the onboarding seam ─────────────────────────────────────────────────────

// OnboardRefs names the durable objects one onboarding rides: the task the
// platform performs it as, and the ask its owner-approval card lands on (Spec
// S13.7). Both names belong to the layer that mints them — the transport ASKS
// for them rather than composing them, so one id scheme exists.
type OnboardRefs struct {
	TaskID string `json:"task_id"`
	AskRef string `json:"ask_ref"`
}

// OnboardSurface is the api-facing door to the S13.7 onboarding task (the
// IntakeSurface / CancelSurface / ResumeSurface precedent). It is transport
// plumbing over a LANDED capability: the composition root implements it over
// internal/project's registry half and internal/stage's run half, which is why
// internal/api still imports neither.
//
// There is no `source` parameter, and its absence is the point (brief OQ5):
// this door cannot express a clone-from-host-path even by mistake.
type OnboardSurface interface {
	// StartOnboarding runs the platform's onboarding task for a NEW project —
	// register → initialize the store → scan → draft — and launches the run
	// whose durable ask carries the draft for the owner's D10 approval. It is
	// the caller's job never to call it for an onboarding already in flight.
	StartOnboarding(ctx context.Context, owner, projectID, name, remoteURL string) (OnboardRefs, error)
	// OnboardRefs is the pure naming half: what an onboarding of this project is
	// called. It performs nothing, which is what lets a retry answer with the
	// references of the onboarding that is already running.
	OnboardRefs(projectID string) OnboardRefs
}

// ── the wire shapes ─────────────────────────────────────────────────────────

// ProjectEntry is the registry row as this family serves it: the facts a card
// needs to name a project and say where it stands.
//
// `store_path` is served to NOBODY, the owner included (brief OQ4): a host
// filesystem path has no client use, and the guarantee is structural rather
// than a rule somebody remembers — the column is never selected. The remote is
// served as PRESENCE and not as its URL, because R4 names presence and because
// a remote URL can carry an embedded credential.
type ProjectEntry struct {
	ProjectID string   `json:"project_id"`
	Name      string   `json:"name"`
	Owner     string   `json:"owner"`
	Members   []string `json:"members"`
	// State is `pending` while onboarding is in flight and `active` once the
	// owner has approved the draft. A pending entry of the caller's own appears
	// here honestly (brief OQ2): the Projects surface is where the onboarding
	// was started, and an invisible in-flight project is a worse answer than a
	// visible one that says what it is waiting for.
	State          string `json:"state"`
	DefaultBranch  string `json:"default_branch"`
	HasRemote      bool   `json:"has_remote"`
	CaptureVersion int    `json:"capture_version"`
	CreatedTS      string `json:"created_ts"`
	UpdatedTS      string `json:"updated_ts"`
}

// ProjectCommands is the captured build/test/lint/run/preview command set
// (S13.7). The preview slot's consumer is S13.8's; here it is data.
type ProjectCommands struct {
	Build   string `json:"build,omitempty"`
	Test    string `json:"test,omitempty"`
	Lint    string `json:"lint,omitempty"`
	Run     string `json:"run,omitempty"`
	Preview string `json:"preview,omitempty"`
}

// ProjectDangerZone is one captured danger zone: where it is, what it forbids,
// and why. Its stored source hash is the drift check's own input (S13.7) and is
// not part of what a person reads about their project.
type ProjectDangerZone struct {
	Path   string `json:"path"`
	Action string `json:"action,omitempty"`
	Rule   string `json:"rule"`
}

// ProjectCapture is the CURRENT captured-content version, whole (brief OQ4):
// members read the same conventions, commands and danger zones the platform
// injects into the stage brief of every run in the project, including their own
// runs (S13.7 feeds → S05/S06.2). Hiding from a member what the platform is
// already acting on in front of them would be theater.
type ProjectCapture struct {
	Version     int                 `json:"version"`
	Conventions []string            `json:"conventions"`
	Commands    ProjectCommands     `json:"commands"`
	DangerZones []ProjectDangerZone `json:"danger_zones"`
	ScanHash    string              `json:"scan_hash,omitempty"`
	CapturedBy  string              `json:"captured_by,omitempty"`
	CapturedTS  string              `json:"captured_ts,omitempty"`
}

// ProjectCaptureSummary is what a LIST row carries of the capture (brief OQ1):
// its version, how much it holds, and the one command a person recognizes a
// project by. The whole capture is one read away on the detail door — the
// S15.3 snapshot-economy split, not a visibility line: the same people may read
// both.
type ProjectCaptureSummary struct {
	Version     int    `json:"version"`
	Conventions int    `json:"conventions"`
	DangerZones int    `json:"danger_zones"`
	TestCommand string `json:"test_command,omitempty"`
	CapturedBy  string `json:"captured_by,omitempty"`
	CapturedTS  string `json:"captured_ts,omitempty"`
}

// ProjectListItem is one row of the list read.
type ProjectListItem struct {
	ProjectEntry
	Capture ProjectCaptureSummary `json:"capture"`
}

// ProjectDetail is one entry with its full current capture and the S13.6
// protected-ref policy the broker reads (data here, per brief OQ4).
type ProjectDetail struct {
	ProjectEntry
	ProtectedRefs []string       `json:"protected_refs"`
	Capture       ProjectCapture `json:"capture"`
}

// ProjectList is the scoped list with its S15.3 tail cursor.
type ProjectList struct {
	Projects []ProjectListItem `json:"projects"`
	// Visibility states the rule that produced this set, because "why is that
	// project not here?" is a question a person asks of this surface and the
	// honest answer is a sentence rather than a silent omission (the memory
	// family's precedent).
	Visibility string `json:"visibility"`
	Cursor     int64  `json:"cursor"`
	Truncated  bool   `json:"truncated"`
}

// ProjectDetailResponse is one entry read.
type ProjectDetailResponse struct {
	Project ProjectDetail `json:"project"`
	Cursor  int64         `json:"cursor"`
}

// ProjectStarted is the create door's answer: the drafted entry, the references
// its approval will arrive on, and an honest sentence about what exists yet.
// It is served 200, not 201 (brief OQ6): no route on this API answers 201, and
// a family that invented one would be describing the same kind of act in a
// second vocabulary.
type ProjectStarted struct {
	Project ProjectDetail `json:"project"`
	TaskID  string        `json:"task_id"`
	AskRef  string        `json:"ask_ref"`
	Detail  string        `json:"detail"`
	Cursor  int64         `json:"cursor"`
}

const projectsVisibilityRule = "the projects you own, plus the projects you have been invited to as a member (S13.7). " +
	"Your own onboardings appear while they are still pending your approval; another person's never appear, at any state. " +
	"The operator role bit opens nothing here: a project's captured content is project content, and D10 is authority over what the platform DOES, " +
	"not a read into another person's work."

// onboardStartedDetail and onboardInFlightDetail are honest about the card.
//
// The approval ask row lands when the SCHEDULER dispatches the onboarding run,
// which has not happened when this response is written. So the answer names the
// reference the card will carry and says where it will appear; claiming an open
// card exists would be a surface asserting something it has not observed.
const onboardStartedDetail = "onboarding started: the entry is registered, its store initialized and scanned, and its drafted " +
	"conventions, commands and danger zones are captured as version 1 — pending your approval (D10). The approval card appears in " +
	"/api/approvals once the onboarding run dispatches; it is not open yet. Answering it there activates the entry, and no other door does."

const onboardInFlightDetail = "this onboarding was already started and is still pending your approval: what follows is the entry that " +
	"exists, not a second onboarding. Nothing was cloned, scanned or filed again (S15.2: a repeated write returns the already-resolved state)."

// errNoSuchProject is the ONE refusal an unknown id and an entry the caller
// cannot see SHARE — same status, same body, and no id echoed back. Telling
// them apart would make this door an existence oracle for other people's
// projects, which is the landed noSuchPin discipline (S13.7; §38 404-before-403).
var errNoSuchProject = &SurfaceError{Status: http.StatusNotFound, Code: "not_found", Msg: "project not found"}

// errProjectTaken is the create door's conflict: the registry id namespace is
// one namespace, and an id that is taken cannot be onboarded again whoever
// holds it.
//
// It names no owner, no project name and no content. The taken-and-visible case
// answers here and the taken-but-invisible case answers with the STORE's own
// refusal through the seam (§40-C: a refusal carries the words of the layer
// that made it) — both 409 `already_registered`, and the difference in wording
// discloses nothing, because a caller can already tell whether an id is one of
// theirs by reading their own list.
var errProjectTaken = &SurfaceError{Status: http.StatusConflict, Code: "already_registered",
	Msg: "that project id is already registered; if it is yours, read it rather than starting it again"}

// ── the ONE registry read, and the ONE visibility predicate ─────────────────

// projectRow is one repo_registry row with its CURRENT capture, as read.
type projectRow struct {
	entry         ProjectEntry
	protectedRefs []string
	capture       ProjectCapture
}

// registryRead is the SQL of this family: every column the two doors serve,
// joined to the capture the entry points at. `store_path` is deliberately
// absent from the select list (brief OQ4).
const registryRead = `
	SELECT r.project_id, r.user_id, r.name, r.remote_url, r.default_branch, r.members,
	       r.protected_refs, r.state, r.capture_version, r.created_ts, r.updated_ts,
	       COALESCE(c.conventions, ''), COALESCE(c.commands, ''), COALESCE(c.danger_zones, ''),
	       COALESCE(c.scan_hash, ''), COALESCE(c.captured_by, ''), COALESCE(c.captured_ts, '')
	  FROM repo_registry r
	  LEFT JOIN repo_registry_captures c
	    ON c.project_id = r.project_id AND c.version = r.capture_version
	 WHERE (? = '' OR r.project_id = ?)
	 ORDER BY r.project_id
	 LIMIT ?`

// visibleProjectRows reads the S13.7 registry and applies THE visibility rule.
// An empty projectID reads the caller's whole visible set; a non-empty one reads
// at most that entry, and an entry the caller may not see comes back as NO ROW —
// which is what makes the detail door's refusal structurally identical to the
// unknown-id one rather than identical by two pieces of code agreeing.
//
// The registry row is read as bounded SQL rather than through internal/project,
// which internal/api imports in neither direction (the §40-B wall): the columns
// this needs are ordinary data, and importing the git topology to read them
// would widen the one wall that keeps the outward path single.
//
// The cursor is drained and CLOSED before anything is decoded or derived. The
// control plane shares ONE writer connection (S02.1), so a handler that leaves a
// cursor open across further work deadlocks everything on that handle (§38 —
// found live once, by a hung test).
func (s *Server) visibleProjectRows(ctx context.Context, viewer, projectID string) ([]projectRow, error) {
	type raw struct {
		e                                ProjectEntry
		remote, members, protected       string
		conventions, commands, zones     string
		scanHash, capturedBy, capturedTS string
	}
	rows, err := s.proj.db.QueryContext(ctx, registryRead, projectID, projectID, projectRegistryCap)
	if err != nil {
		return nil, fmt.Errorf("read project registry: %w", err)
	}
	var scanned []raw
	for rows.Next() {
		var r raw
		if err := rows.Scan(&r.e.ProjectID, &r.e.Owner, &r.e.Name, &r.remote, &r.e.DefaultBranch,
			&r.members, &r.protected, &r.e.State, &r.e.CaptureVersion, &r.e.CreatedTS, &r.e.UpdatedTS,
			&r.conventions, &r.commands, &r.zones, &r.scanHash, &r.capturedBy, &r.capturedTS); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan project registry: %w", err)
		}
		scanned = append(scanned, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("read project registry: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("read project registry: %w", err)
	}

	out := make([]projectRow, 0, len(scanned))
	for _, r := range scanned {
		members := s.decodeStrings(r.e.ProjectID, "members", r.members)
		if !projectVisibleTo(r.e.Owner, members, viewer) {
			continue
		}
		r.e.Members = members
		r.e.HasRemote = r.remote != ""
		if r.e.Members == nil {
			r.e.Members = []string{}
		}
		row := projectRow{
			entry:         r.e,
			protectedRefs: s.decodeStrings(r.e.ProjectID, "protected_refs", r.protected),
			capture: ProjectCapture{
				Version: r.e.CaptureVersion, ScanHash: r.scanHash,
				CapturedBy: r.capturedBy, CapturedTS: r.capturedTS,
				Conventions: s.decodeStrings(r.e.ProjectID, "conventions", r.conventions),
			},
		}
		if row.protectedRefs == nil {
			row.protectedRefs = []string{}
		}
		if row.capture.Conventions == nil {
			row.capture.Conventions = []string{}
		}
		if r.commands != "" {
			if err := json.Unmarshal([]byte(r.commands), &row.capture.Commands); err != nil {
				s.logger.Error("projects: captured commands do not decode", "project", r.e.ProjectID, "err", err)
			}
		}
		row.capture.DangerZones = []ProjectDangerZone{}
		if r.zones != "" {
			if err := json.Unmarshal([]byte(r.zones), &row.capture.DangerZones); err != nil {
				s.logger.Error("projects: captured danger zones do not decode", "project", r.e.ProjectID, "err", err)
				row.capture.DangerZones = []ProjectDangerZone{}
			}
		}
		out = append(out, row)
	}
	return out, nil
}

// projectVisibleTo is THE visibility rule of this family, written once: the
// owning user, or somebody they invited (S13.7 "owning user, invited members").
// The list door and the detail door share this ONE expression through the read
// above, so they cannot disagree about what a person may see — structurally,
// rather than by two implementations being kept in step.
//
// It takes no role bit (brief OQ3), which is why there is no operator limb to
// omit by accident, and no lifecycle state (brief OQ2): a pending entry is as
// visible to its own people, and as invisible to everyone else, as an active
// one. That the entry is not yet usable is what `state` says.
func projectVisibleTo(owner string, members []string, viewer string) bool {
	if owner == viewer {
		return true
	}
	for _, m := range members {
		if m == viewer {
			return true
		}
	}
	return false
}

// decodeStrings reads one of the registry's JSON string-list columns. A column
// this transport cannot read is not a reason to widen what a caller sees: an
// unreadable membership list means NOT a member, and the ops log carries the
// corruption.
func (s *Server) decodeStrings(projectID, column, stored string) []string {
	if stored == "" || stored == "[]" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(stored), &out); err != nil {
		s.logger.Error("projects: registry column does not decode", "project", projectID, "column", column, "err", err)
		return nil
	}
	return out
}

func (row projectRow) detail() ProjectDetail {
	return ProjectDetail{ProjectEntry: row.entry, ProtectedRefs: row.protectedRefs, Capture: row.capture}
}

func (row projectRow) listItem() ProjectListItem {
	return ProjectListItem{ProjectEntry: row.entry, Capture: ProjectCaptureSummary{
		Version:     row.capture.Version,
		Conventions: len(row.capture.Conventions),
		DangerZones: len(row.capture.DangerZones),
		TestCommand: row.capture.Commands.Test,
		CapturedBy:  row.capture.CapturedBy,
		CapturedTS:  row.capture.CapturedTS,
	}}
}

// ── GET /api/projects ───────────────────────────────────────────────────────

func (s *Server) handleProjectList(w http.ResponseWriter, r *http.Request) {
	if !s.projReady(w) {
		return
	}
	limit, err := readLimit(r)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	if limit > projectsListCap {
		limit = projectsListCap
	}
	rows, err := s.visibleProjectRows(r.Context(), s.callerID(r), "")
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	cursor, err := s.head(r.Context())
	if err != nil {
		s.writeSurface(w, nil, fmt.Errorf("read head cursor: %w", err))
		return
	}
	out := ProjectList{Projects: []ProjectListItem{}, Visibility: projectsVisibilityRule, Cursor: cursor}
	for _, row := range rows {
		if len(out.Projects) >= limit {
			// Observed, not predicted: the flag says a page ended, never that
			// rows were dropped for any other reason.
			out.Truncated = true
			break
		}
		out.Projects = append(out.Projects, row.listItem())
	}
	s.writeReadJSON(w, out)
}

// ── GET /api/projects/{project} ─────────────────────────────────────────────

func (s *Server) handleProjectDetail(w http.ResponseWriter, r *http.Request) {
	row, ok := s.projectRead(w, r)
	if !ok {
		return
	}
	cursor, err := s.head(r.Context())
	if err != nil {
		s.writeSurface(w, nil, fmt.Errorf("read head cursor: %w", err))
		return
	}
	s.writeReadJSON(w, ProjectDetailResponse{Project: row.detail(), Cursor: cursor})
}

// projectRead resolves the path's entry under the one predicate, with the one
// refusal. Nothing about an entry the caller cannot see reaches the response —
// not a status that differs, not a message that differs, and no captured
// content.
func (s *Server) projectRead(w http.ResponseWriter, r *http.Request) (projectRow, bool) {
	if !s.projReady(w) {
		return projectRow{}, false
	}
	rows, err := s.visibleProjectRows(r.Context(), s.callerID(r), r.PathValue("project"))
	if err != nil {
		s.writeSurface(w, nil, err)
		return projectRow{}, false
	}
	if len(rows) == 0 {
		s.writeSurfaceErr(w, errNoSuchProject)
		return projectRow{}, false
	}
	return rows[0], true
}

// ── POST /api/projects ──────────────────────────────────────────────────────

// projectCreateBody is the onboarding request as it arrives (brief OQ9). The
// OWNER is not a field: it is the session identity, because 15.6 attributes a
// project to the person who started it and a body-supplied owner would be a
// browser claiming authority (S15.2 — the browser is a display).
//
// MEMBERS ARE NOT A FIELD EITHER, and that is a stated absence rather than an
// omission: the landed onboarding seam takes no member list, so a project is
// created with its owner alone and inviting somebody is a later verb's act.
// Accepting the field here and dropping it would be worse than not offering it.
type projectCreateBody struct {
	// ProjectID is the caller's to choose (brief OQ9): it is what every later
	// read, intake pin and injection names the project by, and the store maps
	// any id shape onto a collision-free path of its own.
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	// Source is refused unless empty (brief OQ5). Over HTTP a clone source would
	// be a host-filesystem READ primitive: any path the platform user can read
	// would become cloneable into a project store and then readable back through
	// this family's own read door. So every onboarding here initializes a FRESH
	// store (S13.7: "a v0 project without a repo gets one created/registered by
	// the onboarding task"), and importing an existing local repo is a stated
	// deliberate absence — a later packet's, with its own confinement decision.
	Source string `json:"source,omitempty"`
	// RemoteURL is accepted and STORED as data. Nothing dials it, here or
	// anywhere: the registry records the remote and the platform's git is
	// hermetic and local-transport-only (§23). Cloning FROM a remote is the
	// other stated deliberate absence — it needs the credential broker, which
	// makes it a brokered packet rather than a parameter.
	RemoteURL string `json:"remote_url,omitempty"`
}

func (s *Server) handleProjectCreate(w http.ResponseWriter, r *http.Request) {
	if !s.projReady(w) || !s.onboardReady(w) {
		return
	}
	id, _ := IdentityFrom(r.Context())
	if id.Dev {
		// The dev-posture fallback identity reads, and never files (brief OQ8;
		// §9's standing rule, and the settings family's own dev limb). A project
		// is attributed to a PERSON (15.6), and unlike the memory gate the
		// registry has no users-table wall of its own to refuse `dev` with, so
		// the refusal belongs here. Reads stay dev-accessible: browsing is not
		// filing.
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusForbidden, Code: "dev_identity",
			Msg: "the dev-posture identity browses but never onboards: a project is attributed to the person who owns it (15.6). Sign in as a person and start it again."})
		return
	}
	raw, ok := s.readBody(w, r)
	if !ok {
		return
	}
	var body projectCreateBody
	if err := json.Unmarshal(raw, &body); err != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_body", Msg: err.Error()})
		return
	}
	projectID := strings.TrimSpace(body.ProjectID)
	name := strings.TrimSpace(body.Name)
	if projectID == "" {
		s.writeSurface(w, nil, badRequest(`missing "project_id": the registry id is the caller's to choose and is what every later read, pin and injection names`))
		return
	}
	if name == "" {
		s.writeSurface(w, nil, badRequest(`missing "name": the registry entry needs the name a person calls the project by (S13.7)`))
		return
	}
	if strings.TrimSpace(body.Source) != "" {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_source",
			Msg: `"source" must be empty: this door creates and initializes a fresh project store, and it will not read a path off the host on a caller's say-so. Importing an existing local repository is not built here.`})
		return
	}

	// Retry-safety, before anything is dispatched (brief OQ7; S15.2 "a repeated
	// answer returns the already-resolved state — a phone retry can never
	// double-fire"). A second POST for an onboarding of the caller's own that is
	// still pending answers with the entry that already exists: nothing is
	// re-cloned, no second task or run is filed, and the drafted capture is left
	// exactly as it stands.
	existing, err := s.visibleProjectRows(r.Context(), id.UserID, projectID)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	if len(existing) == 1 {
		row := existing[0]
		if row.entry.Owner == id.UserID && row.entry.State == projectStatePending {
			s.writeStarted(w, r, row, s.onboard.OnboardRefs(projectID), onboardInFlightDetail)
			return
		}
		s.writeSurfaceErr(w, errProjectTaken)
		return
	}
	// A taken id is taken whoever holds it, and the door must say so BEFORE it
	// starts anything (brief OQ6). The onboarding seam cannot answer this
	// question for us: project.OnboardStart is idempotent by design — for an
	// entry that already exists it returns that entry's current draft rather
	// than re-cloning, which is right for the run's own re-dispatch and would be
	// wrong here, where it would attach this caller's onboarding task to
	// somebody else's project.
	//
	// It is an EXISTENCE question and nothing more: no row, no column and no
	// fact about another person's project reaches the answer, which is the same
	// `already_registered` the caller's own taken id gets. What they learn is
	// only what they must in order to choose another id — and they can already
	// tell whether an id is one of theirs by reading their own list.
	taken, err := s.projectIDTaken(r.Context(), projectID)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	if taken {
		s.writeSurfaceErr(w, errProjectTaken)
		return
	}

	refs, err := s.onboard.StartOnboarding(r.Context(), id.UserID, projectID, name, strings.TrimSpace(body.RemoteURL))
	if err != nil {
		// Statuses come off the seam's TYPED error (§38's ban on matching error
		// text): the composition root translates the project store's sentinels,
		// and anything unmarked is a platform fault, which writeSurface logs and
		// answers 500.
		s.writeSurface(w, nil, err)
		return
	}
	// Read the drafted entry back through the read path above, so the create
	// door and the detail door describe a project in exactly one way.
	rows, err := s.visibleProjectRows(r.Context(), id.UserID, projectID)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	if len(rows) == 0 {
		s.logger.Error("projects: onboarding reported success but registered no visible entry", "project", projectID, "owner", id.UserID)
		s.writeSurface(w, nil, fmt.Errorf("onboarding started but the entry could not be read back"))
		return
	}
	s.writeStarted(w, r, rows[0], refs, onboardStartedDetail)
}

func (s *Server) writeStarted(w http.ResponseWriter, r *http.Request, row projectRow, refs OnboardRefs, detail string) {
	cursor, err := s.head(r.Context())
	if err != nil {
		s.writeSurface(w, nil, fmt.Errorf("read head cursor: %w", err))
		return
	}
	s.writeReadJSON(w, ProjectStarted{
		Project: row.detail(), TaskID: refs.TaskID, AskRef: refs.AskRef,
		Detail: detail, Cursor: cursor,
	})
}

// projectIDTaken answers whether a registry id is in use. It reads NO column of
// the row and is used at exactly one place — the create door, above, where the
// alternative is starting an onboarding onto an id that belongs to somebody
// else. Every READ door goes through the visibility predicate instead.
func (s *Server) projectIDTaken(ctx context.Context, projectID string) (bool, error) {
	var one int
	err := s.proj.db.QueryRowContext(ctx, `SELECT 1 FROM repo_registry WHERE project_id = ?`, projectID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read project registry: %w", err)
	}
	return true, nil
}

// onboardReady reports the onboarding seam, or writes the not-wired answer.
//
// The READ doors deliberately do not gate on it: the registry is readable from
// the platform DB alone, and a process composed without the run substrate still
// answers honestly about the projects that exist. That is a considered
// divergence from the memory family's one-family-one-readiness rule, where the
// store and its write gate are two halves of ONE subsystem; here they are two
// organs, and the create door is the only one that needs the second.
func (s *Server) onboardReady(w http.ResponseWriter) bool {
	if s.onboard == nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusServiceUnavailable, Code: "not_wired",
			Msg: "the S13.7 onboarding task surface is not wired in this process: starting a project is a task the platform performs, and this process runs none"})
		return false
	}
	return true
}
