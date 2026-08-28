package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strconv"
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

// OnboardFamilySurface is the ADDITIVE half of the onboarding door (P3-RW-11;
// S15.2): a door that can also carry the owner-declared task family, which is
// what makes intake open that family's question set instead of the generic one.
//
// It is a SECOND interface rather than a sixth parameter on StartOnboarding
// because S15.2's additive rule is the point: OnboardSurface is landed and has
// implementors, and widening it would have broken every one of them for a field
// they may not carry. A door that does not implement this one keeps working and
// simply cannot express a family — and the handler REFUSES a family it cannot
// deliver rather than dropping it silently, because a project onboarded without
// the family its owner asked for would send every one of its tasks to the wrong
// question set with nothing anywhere saying why.
//
// internal/api declares the FIELD and validates nothing about its VALUE: the
// vocabulary lives in internal/project, which refuses an unknown one loudly and
// whose refusal arrives here as the landed ErrBadInput → 400 translation. One
// list, one reader (CONVENTIONS §43).
type OnboardFamilySurface interface {
	StartOnboardingWithFamily(ctx context.Context, owner, projectID, name, remoteURL, family string) (OnboardRefs, error)
}

// ProjectCommandsSurface is the api-facing door to the S13.7 captured command
// set (P3-GF5): the one seam through which this package can cause a registry
// write, composed at the root over internal/project's `EditCommands`.
//
// It exists because the registry is READ here and never written (the §40-B
// wall): `internal/api` imports neither internal/project nor internal/broker,
// so the door's only path to the store is this interface, and the store's
// refusals arrive as the landed sentinel→SurfaceError translation the
// composition root performs (§38: never on message text).
//
// The interface reports whether a NEW capture version was minted rather than
// returning the capture, because that is the one fact the handler needs and
// cannot derive: the trimming the store performs is the store's, so comparing
// submitted bytes here would be a second implementation of it. The state itself
// is read back through the family's own read path, so the write door and the
// detail door describe a project in exactly one way.
type ProjectCommandsSurface interface {
	// SetCommands replaces the project's captured command set as caller,
	// minting a new immutable capture version with everything else carried
	// forward. minted=false is the retry-safe answer: the submitted set is
	// already the captured one, so nothing was written and nothing evented.
	SetCommands(ctx context.Context, caller, projectID string, c ProjectCommands) (minted bool, err error)
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

// ProjectCommandsWritten is the commands door's answer: the entry as it now
// stands, read back through the family's own read path, and an honest sentence
// about what the write did. 200, not 201 — no route on this API answers 201,
// and this is an edit of an existing entry in any case.
type ProjectCommandsWritten struct {
	Project ProjectDetail `json:"project"`
	Detail  string        `json:"detail"`
	Cursor  int64         `json:"cursor"`
}

// The three facts, in the register a member reads them in (P3-GF13 R4). The
// citations: S13.7 sets the own-plus-invited scope, and D10 — house authority
// over what the platform DOES — deliberately opens nothing here, because a
// project's captured content is somebody's work, not platform telemetry
// (CONVENTIONS §40-C's content-vs-telemetry line).
const projectsVisibilityRule = "the projects you own, plus the projects you have been invited to. " +
	"A project you are setting up shows here while it is still waiting for your approval; somebody else's never shows, at any stage. " +
	"Being the household's operator does not open this list any wider: what a project holds is the work of the people in it, and running " +
	"the platform is not the same as reading their work."

// onboardStartedDetail and onboardInFlightDetail are honest about the card.
//
// The approval ask row lands when the SCHEDULER dispatches the onboarding run,
// which has not happened when this response is written. So the answer names the
// reference the card will carry and says where it will appear; claiming an open
// card exists would be a surface asserting something it has not observed.
//
// The in-flight sentence is the same kind of claim and gets the same treatment
// (drain r1 D1): it is served only once the onboarding task has been seen. A
// retry that finds no task is answered with the started sentence instead, which
// is what actually happened — the seam ran again and filed what was missing.
// Both sentences are read by a PERSON, so they name the surface rather than the
// route: the operator's word for where a card lands is the **Inbox**
// (web/src/routes.ts), and an API path is not a place anybody goes. The
// machine-readable reference rides the structured `ask_ref` field beside them,
// so prose and reference stay cleanly split (S15.5: surfaces speak operator
// language).
const onboardStartedDetail = "onboarding started: the entry is registered, its store initialized and scanned, and its drafted " +
	"conventions, commands and danger zones are captured as version 1 — pending your approval, and yours alone as its owner. The approval card lands in your " +
	"Inbox once the onboarding run dispatches; it is not open yet. Answering it there activates the entry, and no other door does."

const onboardInFlightDetail = "this onboarding was already started and is still pending your approval: what follows is the entry that " +
	"exists, not a second onboarding. Nothing was cloned, scanned or filed again — a repeated write returns the state that is already there."

// errNoSuchProject is the ONE refusal an unknown id and an entry the caller
// cannot see SHARE — same status, same body, and no id echoed back. Telling
// them apart would make this door an existence oracle for other people's
// projects, which is the landed noSuchPin discipline (S13.7; §38 404-before-403).
var errNoSuchProject = &SurfaceError{Status: http.StatusNotFound, Code: "not_found", Msg: "project not found"}

// errProjectTaken is the create door's conflict: the registry id namespace is
// one namespace, and an id that is taken cannot be onboarded again whoever
// holds it.
//
// It names no owner, no project name and no content, and it is the ONE answer
// both taken cases get: the caller's own taken id and an id they cannot see
// answer with this identical object, decided at the handler before the seam is
// entered. A refusal that read differently depending on who held the id would
// be an ownership oracle, and a taken-but-invisible id must not so much as
// reach the onboarding seam.
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
	// Family is the owner-declared task family (P3-RW-11): the kind of work
	// this project's tasks are, which decides the question set intake opens for
	// every one of them. Optional — absent means none declared, and a task in a
	// project with no family is ASKED rather than assumed generic. The value is
	// checked by the registry, not here (see OnboardFamilySurface).
	Family string `json:"family,omitempty"`
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
			// 15.6: a project is attributed to a PERSON.
			Msg: "the developer fallback can look around but cannot start a project: a project belongs to the person who owns it, and the fallback is nobody. Sign in as yourself and start it again."})
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
		s.writeSurface(w, nil, badRequest(`missing "name": the entry needs the name you call this project by`))
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
	//
	// The in-flight claim is VERIFIED before it is made (drain r1 D1). The
	// registry half of an onboarding and its run half are two commits, not one,
	// so a pending entry whose onboarding task was never filed is a state the
	// platform can really be in. Answering that state "already started" would
	// name a task and a card that resolve to nothing, and would strand the
	// project pending forever: activation needs the ask, the ask needs the run,
	// and this packet routes no repair verb. So the door asks whether the task
	// is actually there, and when it is not it falls THROUGH to the seam —
	// whose own idempotence is the repair, the registry half returning the
	// existing draft rather than re-cloning while the run half files the task
	// that is missing.
	existing, err := s.visibleProjectRows(r.Context(), id.UserID, projectID)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	if len(existing) == 1 {
		row := existing[0]
		if row.entry.Owner != id.UserID || row.entry.State != projectStatePending {
			s.writeSurfaceErr(w, errProjectTaken)
			return
		}
		refs := s.onboard.OnboardRefs(projectID)
		filed, err := s.onboardTaskFiled(r.Context(), refs.TaskID)
		if err != nil {
			s.writeSurface(w, nil, err)
			return
		}
		if filed {
			s.writeStarted(w, r, row, refs, onboardInFlightDetail)
			return
		}
		// Torn, and the caller's OWN: fall through and let the seam heal it.
	} else {
		// A taken id is taken whoever holds it, and the door must say so BEFORE
		// it starts anything (brief OQ6). The onboarding seam cannot answer this
		// question for us: project.OnboardStart is idempotent by design — for an
		// entry that already exists it returns that entry's current draft rather
		// than re-cloning, which is right for the run's own re-dispatch and would
		// be wrong here, where it would attach this caller's onboarding task to
		// somebody else's project.
		//
		// It is an EXISTENCE question and nothing more: no row, no column and no
		// fact about another person's project reaches the answer, which is the
		// same `already_registered` the caller's own taken id gets. What they
		// learn is only what they must in order to choose another id — and they
		// can already tell whether an id is one of theirs by reading their own
		// list. An id they cannot see never reaches the seam at all.
		taken, err := s.projectIDTaken(r.Context(), projectID)
		if err != nil {
			s.writeSurface(w, nil, err)
			return
		}
		if taken {
			s.writeSurfaceErr(w, errProjectTaken)
			return
		}
	}

	refs, err := s.startOnboarding(r.Context(), id.UserID, projectID, name,
		strings.TrimSpace(body.RemoteURL), strings.TrimSpace(body.Family))
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

// startOnboarding routes to whichever half of the door the composition wired.
// A family the composed door cannot carry is REFUSED, not dropped: the caller
// asked for a project whose tasks get a particular question set, and answering
// "started" while silently onboarding it without one would be a lie the person
// only discovers task by task.
func (s *Server) startOnboarding(ctx context.Context, owner, projectID, name, remoteURL, family string) (OnboardRefs, error) {
	if family == "" {
		return s.onboard.StartOnboarding(ctx, owner, projectID, name, remoteURL)
	}
	door, ok := s.onboard.(OnboardFamilySurface)
	if !ok {
		return OnboardRefs{}, &SurfaceError{Status: http.StatusBadRequest, Code: "family_unsupported",
			Msg: `"family" is not accepted by this deployment's onboarding door: omit it and the project is registered without one (its tasks are then asked what kind of work they are)`}
	}
	return door.StartOnboardingWithFamily(ctx, owner, projectID, name, remoteURL, family)
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

// onboardTaskFiled answers whether the onboarding task the seam names for this
// project has actually been filed. It is what turns the create door's in-flight
// answer from a claim into an observation (drain r1 D1): a `200 already started`
// naming references that resolve to nothing is worse than starting again.
//
// It reads the task row by its id and nothing else — no column, no owner, no
// title — and it is asked only about the CALLER'S OWN pending entry, so it
// discloses nothing an owner does not already hold.
func (s *Server) onboardTaskFiled(ctx context.Context, taskID string) (bool, error) {
	var one int
	err := s.proj.db.QueryRowContext(ctx, `SELECT 1 FROM tasks WHERE task_id = ?`, taskID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read onboarding task: %w", err)
	}
	return true, nil
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

// ── POST /api/projects/{project}/commands ───────────────────────────────────

// projectCommandsBody is the write as it arrives (P3-GF5 R1).
//
// FULL REPLACEMENT, AND THE OBJECT IS THE UNIT. The submitted `commands` object
// becomes the capture's whole command member: a slot the caller omits is
// CLEARED, not left alone. Per-slot patching would need a way to say "leave
// this one" that is distinguishable from "clear this one", and JSON absence
// cannot carry both meanings — so the door takes the whole set, which is also
// what an editor renders and submits.
//
// THE MEMBER IS A POINTER, AND THAT IS THE DIFFERENCE BETWEEN CLEARING AND
// DESTROYING (drain r1 F2). Full replacement means an absent slot is erased —
// which is right when the caller SENT a set, and catastrophic when they sent
// nothing: readBody promotes an empty body to `{}`, so a request that lost its
// payload in transit, or a client with a typo'd envelope, decoded to the zero
// value and wiped every captured command with a 200. Absence of the `commands`
// key is now a malformed request; `{"commands":{}}` is the explicit clear, and
// it stays reachable because R4's all-empty set really is a legitimate act.
//
// The OWNER is not a field, and neither is the project: authority is the
// session identity and the entry is the path (S15.2 — the browser is a
// display). There is no conventions, danger-zone or family member: this door
// orders commands, and a capture editor is a later packet's act (brief OQ5).
type projectCommandsBody struct {
	Commands *json.RawMessage `json:"commands"`
}

// commandSlotNames is the wire vocabulary of the commands object, derived from
// ProjectCommands' OWN json tags. Deriving rather than repeating is what keeps
// the accepted set and the served set from drifting: a slot added to the struct
// is accepted here the moment it exists, and one nobody added cannot be.
var commandSlotNames = func() map[string]bool {
	out := map[string]bool{}
	t := reflect.TypeOf(ProjectCommands{})
	for i := 0; i < t.NumField(); i++ {
		if name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ","); name != "" && name != "-" {
			out[name] = true
		}
	}
	return out
}()

// readCommandSlots decodes the commands object and REFUSES a key it does not
// know (drain r1 F2).
//
// Unknown keys are refused HERE and nowhere else on this API, and the narrowness
// is the justification: elsewhere an unknown member is forward-compatible slack
// that costs a caller nothing, while here it is silent destruction — a `biuld`
// typo drops the build command the caller meant to send AND erases the one that
// was captured, under a 200 that says the write succeeded. The refusal names the
// unknown slot and the set that exists, so the fix is one edit away.
func readCommandSlots(raw json.RawMessage) (ProjectCommands, error) {
	var slots map[string]json.RawMessage
	if err := json.Unmarshal(raw, &slots); err != nil {
		return ProjectCommands{}, badRequest(`"commands" must be an object of command slots, e.g. {"commands":{"test":"go test ./..."}}: ` + err.Error())
	}
	var unknown []string
	for name := range slots {
		if !commandSlotNames[name] {
			unknown = append(unknown, strconv.Quote(name))
		}
	}
	if len(unknown) > 0 {
		// Sorted, so the same bad request always reads the same way.
		sort.Strings(unknown)
		known := make([]string, 0, len(commandSlotNames))
		for name := range commandSlotNames {
			known = append(known, name)
		}
		sort.Strings(known)
		return ProjectCommands{}, badRequest(fmt.Sprintf(
			"unknown command slot %s: a project captures %s. Nothing was changed — this write replaces the whole set, so a slot the platform does not know would have been dropped and the commands you meant to keep erased with it",
			strings.Join(unknown, ", "), strings.Join(known, ", ")))
	}
	var cmds ProjectCommands
	if err := json.Unmarshal(raw, &cmds); err != nil {
		return ProjectCommands{}, badRequest(`"commands" slots must be strings: ` + err.Error())
	}
	return cmds, nil
}

func (s *Server) handleProjectCommands(w http.ResponseWriter, r *http.Request) {
	if !s.projReady(w) || !s.projCommandsReady(w) {
		return
	}
	id, _ := IdentityFrom(r.Context())
	if id.Dev {
		// Browsing is not filing (§9; the create door's own rule): a capture is
		// attributed to the person who typed it, and the dev-posture fallback is
		// nobody. Checked BEFORE the entry is resolved, so the answer does not
		// depend on which projects the fallback identity happens not to own.
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusForbidden, Code: "dev_identity",
			// 15.6: a capture is attributed to a PERSON.
			Msg: "the developer fallback can look around but cannot set commands: a project's commands are recorded against the person who set them, and the fallback is nobody. Sign in as yourself and set them again."})
		return
	}
	projectID := r.PathValue("project")
	rows, err := s.visibleProjectRows(r.Context(), id.UserID, projectID)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	if len(rows) == 0 {
		// An unknown id and an entry the caller cannot see are ONE answer, so
		// this door cannot become an existence oracle for another person's
		// projects (§38 404-before-403; the noSuchPin discipline).
		s.writeSurfaceErr(w, errNoSuchProject)
		return
	}
	row := rows[0]
	if row.entry.Owner != id.UserID {
		// A VISIBLE member is told the truth, including who to ask: they can
		// already read `owner` on the detail, so naming it discloses nothing and
		// saves them guessing (brief OQ3 — widening to members is a later
		// ratified act, not an omission here).
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusForbidden, Code: "not_owner",
			Msg: fmt.Sprintf("only the owner sets a project's commands, and this project is owned by %q. "+
				"They decide what gets run to check everybody's work in it — ask them to set it.", row.entry.Owner)})
		return
	}
	if row.entry.State != projectStateActive {
		// The pending draft's door is the onboarding approval card, where the
		// owner edits the whole draft and answering activates the entry. A
		// second write path onto that draft would be one act with two audit
		// stories — the "three doors and no fourth" shape.
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusConflict, Code: "not_active",
			Msg: "this project is still waiting for your approval, so its drafted commands are edited on the onboarding card in your Inbox — " +
				"answering it there captures the draft you approve and activates the entry. This door serves an active project."})
		return
	}

	raw, ok := s.readBody(w, r)
	if !ok {
		return
	}
	var body projectCommandsBody
	if err := json.Unmarshal(raw, &body); err != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_body", Msg: err.Error()})
		return
	}
	if body.Commands == nil {
		// A body with no `commands` member is malformed, not an instruction to
		// erase (drain r1 F2): an empty POST body decodes to `{}`, so accepting
		// absence would let a request that lost its payload wipe every captured
		// command and answer 200.
		// S07.8: an empty capture returns the project to the bootstrap posture.
		s.writeSurface(w, nil, badRequest(`missing "commands": this door replaces the whole captured set, so the object is required. `+
			`To clear every command — which leaves the platform with nothing to check the work with, so your review decides it — send {"commands":{}} and mean it.`))
		return
	}
	commands, err := readCommandSlots(*body.Commands)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	// The VALUES are checked by the registry, not here: the one-line/encoding/
	// length rules live with the store that will hand them to the verification
	// sandbox, and its refusal arrives as the landed ErrBadInput → 400
	// translation naming the rule and the slot. One list, one reader (§43).
	minted, err := s.projCommands.SetCommands(r.Context(), id.UserID, projectID, commands)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	// Read back through the family's own read path, so the write door and the
	// detail door describe a project in exactly one way (the create door's
	// precedent).
	rows, err = s.visibleProjectRows(r.Context(), id.UserID, projectID)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	if len(rows) == 0 {
		s.logger.Error("projects: commands written but the entry could not be read back", "project", projectID, "owner", id.UserID)
		s.writeSurface(w, nil, fmt.Errorf("the commands were captured but the entry could not be read back"))
		return
	}
	cursor, err := s.head(r.Context())
	if err != nil {
		s.writeSurface(w, nil, fmt.Errorf("read head cursor: %w", err))
		return
	}
	s.writeReadJSON(w, ProjectCommandsWritten{
		Project: rows[0].detail(),
		Detail:  commandsDetail(minted, rows[0].capture),
		Cursor:  cursor,
	})
}

// commandsDetail says what the write actually did, in plain words for the
// person who pressed the button (S15.5: surfaces speak operator language).
//
// The three sentences are three different FACTS, and collapsing them would make
// the surface claim something it did not observe: a repeated submission is the
// S15.2 retry answer and captured nothing, an emptied set really does return
// the project to Spec S07.8's bootstrap posture, and only the third case can
// promise the ladder.
func commandsDetail(minted bool, c ProjectCapture) string {
	version := fmt.Sprintf("version %d", c.Version)
	if !minted {
		return "nothing changed: these are already the commands captured as " + version +
			", so no new version was recorded and nothing was added to the audit trail. A repeated write returns the state that is already there."
	}
	// S07.8's bootstrap posture and S07.3's check ladder, said for the person
	// who just pressed the button (P3-GF13 R6).
	if c.Commands == (ProjectCommands{}) {
		return "the commands are cleared, captured as " + version +
			". With no build, test or lint command the platform has nothing to run against this project's work, so checking falls back to " +
			"bootstrap mode: every check is recorded as unproven, and your review is what decides whether the work is right. " +
			"The previous version is kept, never overwritten."
	}
	return "captured as " + version +
		": from the next round of checking on, work in this project is checked by running these commands, and the note saying the verdict is " +
		"advisory only drops away. Nothing was run just now — a captured command runs only inside the sealed checking sandbox. " +
		"The previous version is kept, never overwritten."
}

// projCommandsReady reports the commands seam, or writes the not-wired answer.
// The READ doors deliberately do not gate on it, for the onboardReady reason: a
// process composed without the write half still answers honestly about the
// projects that exist.
func (s *Server) projCommandsReady(w http.ResponseWriter) bool {
	if s.projCommands == nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusServiceUnavailable, Code: "not_wired",
			Msg: "the S13.7 project-commands door is not wired in this process: capturing a project's commands is a registry write, and this process composes no registry writer"})
		return false
	}
	return true
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
