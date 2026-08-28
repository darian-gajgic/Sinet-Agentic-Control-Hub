package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/memory"
)

// memory.go — the S15.2 memory family (P3-B6-3C): the scoped browse reads, the
// gate-routed write verbs, the owner's removal rights and the S09.7 conflict
// edges with their resolution.
//
// EVERY WRITE GOES THROUGH THE S09 GATE, AND THERE IS NO WAY AROUND IT. The
// gate is the station-3 human write path (S09.4): it resolves the acting
// person against the users table, applies the D10 house rule, refuses the
// dormant scope and the layer with no writer, and lands the row, the file, the
// conflict edges and the audit event in one transaction. This file constructs
// no knowledge SQL, mints no knowledge event and implements no second copy of
// any of those rules — it maps a request onto a gate verb and serves what the
// gate returns, INCLUDING its refusals, which carry the gate's own reason
// (§17). A source scan in the test file pins that.
//
// VISIBILITY IS A CONTENT LINE, NOT THE OBSERVABILITY LINE (OQ5, ratified). A
// caller sees their own entries across every scope they own (S4.1), house-scope
// entries (house defaults address everyone, S4.5), and project-scope entries of
// the projects they own or are invited to (S09.6 scope membership). The
// operator role bit does NOT open another member's user-scope entries: memory
// is personal CONTENT, and D10 is house authority — the right to curate the
// house scope — not a read into someone's personal store. This deliberately
// diverges from the §30 operator-sees-all rule, which is an OBSERVABILITY rule
// about runs and telemetry; both directions are test-pinned.
//
// WHAT STAYS S09-INTERNAL (R22, checkable negatives): `lesson_proposals` get no
// route — a proposal is not memory and is read into nothing (S09.2); `Adopt`
// stays unrouted, because browse-and-adopt is the v1 sharing surface (S09.4
// activation table); `Store.Search` gets no route here, because knowledge_search
// is the run-scoped ENGINE tool whose scope predicate comes from a run id, not
// from an HTTP caller (S09.3); and there is no L1 write shim — the gate refuses
// the observation kind and that refusal IS the dormancy (S09.4).
//
// SERVED CONTENT IS UNWRAPPED. An entry body, its title and its conflict
// question are S09 product read from S09's OWN tables — content, not an
// observability projection lifted out of a run_events payload (§38 ruling (b);
// §40). Nothing here touches internal/redact, so the enumerated redaction edge
// stays honest at three files.

// memoryListCap bounds the browse list. Structural, not ⚙ (S18 ratifies no such
// key — interim under the standing settings-tab directive), and it is the same
// number as readPageDefault because this is one read surface with one page size
// (the §38/§40 precedent).
const memoryListCap = readPageDefault

// projectRegistryCap bounds the registry read that resolves the caller's
// project membership. The S13.7 registry is household-scale — a handful of
// projects — but expectation is not a bound, and this read shares the control
// plane's one writer connection (S02.1).
const projectRegistryCap = readPageMax

// The scope/kind/status filter vocabularies a `?scope=`/`?kind=`/`?status=`
// may name. Each is the S09.2 stored enum, closed: anything else is a 400
// rather than a silently empty answer.
var (
	memoryScopes = map[string]bool{
		string(memory.ScopeUser): true, string(memory.ScopeProject): true,
		string(memory.ScopeHouse): true, string(memory.ScopeWorkerOverlay): true,
	}
	memoryKinds = map[string]bool{
		string(memory.KindLesson): true, string(memory.KindPreference): true,
		string(memory.KindPlaybook): true, string(memory.KindRubric): true,
		string(memory.KindStyle): true, string(memory.KindExemplar): true,
		string(memory.KindConvention): true, string(memory.KindObservation): true,
		string(memory.KindTaxonomy): true, string(memory.KindTriggerRules): true,
	}
	memoryStatuses = map[string]bool{
		string(memory.StatusProposed): true, string(memory.StatusActive): true,
		string(memory.StatusRetired): true, string(memory.StatusRemoved): true,
	}
)

// The origins a CREATE may name. `human_direct` is the workspace-UI write of
// the S09.4 v0 activation row; `imported` is the N20 house-KB import ceremony,
// where the operator's import IS the approval. The other two S09.5 origins have
// no HTTP door: `adopted_from` is the v1 browse-and-adopt surface, and
// `proposed_from` belongs to the dormant station-2 drafter.
const (
	originHumanDirect = string(memory.OriginHumanDirect)
	originImported    = string(memory.OriginImported)
)

// ── the wire shapes ─────────────────────────────────────────────────────────

// MemoryEntry is one knowledge row as this family serves it: the S09.2 body,
// the S09.5 provenance, the gate status verbatim and the S09.8 lifecycle
// fields, each grouped by what it answers about the entry.
//
// Content and FileRef are exclusive by construction — the row is the selector
// index and the file is the content (S09.2). A file-backed entry serves its ref
// and its commit rather than the bytes: the file lives in the scope's
// git-versioned knowledge dir, and git is its history.
type MemoryEntry struct {
	EntryID    string           `json:"entry_id"`
	Owner      string           `json:"owner"`
	Scope      string           `json:"scope"`
	ScopeRef   string           `json:"scope_ref,omitempty"`
	Layer      string           `json:"layer"`
	Kind       string           `json:"kind"`
	Title      string           `json:"title"`
	Content    string           `json:"content,omitempty"`
	FileRef    string           `json:"file_ref,omitempty"`
	FileCommit string           `json:"file_commit,omitempty"`
	TopicKey   string           `json:"topic_key,omitempty"`
	Selectors  memory.Selectors `json:"selectors"`
	Status     string           `json:"status"`
	Tombstone  bool             `json:"tombstone"`
	// TombstoneNote is the retained stub's own words after a true deletion
	// (G2 Def.9): id, dates, "removed at owner request", and nothing else.
	TombstoneNote  string             `json:"tombstone_note,omitempty"`
	Provenance     MemoryProvenance   `json:"provenance"`
	Verification   MemoryVerification `json:"verification"`
	LastInjectedTS string             `json:"last_injected_ts,omitempty"`
	CreatedTS      string             `json:"created_ts"`
	UpdatedTS      string             `json:"updated_ts"`
}

// MemoryProvenance is the S09.5 provenance block: where the entry came from,
// who approved it and where it sits in its version chain.
type MemoryProvenance struct {
	Origin        string `json:"origin"`
	OriginRef     string `json:"origin_ref,omitempty"`
	ProposerModel string `json:"proposer_model,omitempty"`
	ApprovedBy    string `json:"approved_by,omitempty"`
	ApprovedTS    string `json:"approved_ts,omitempty"`
	Version       int64  `json:"version"`
	Supersedes    string `json:"supersedes,omitempty"`
}

// MemoryVerification is the S09.8 lifecycle block. A zero interval is a real
// answer — a preference carries no re-verify interval unless a human flags one.
type MemoryVerification struct {
	VerifiedBy           string `json:"verified_by,omitempty"`
	VerifiedTS           string `json:"verified_ts,omitempty"`
	ReverifyIntervalDays int64  `json:"reverify_interval_days,omitempty"`
	ExpiresTS            string `json:"expires_ts,omitempty"`
}

func memoryEntryView(e memory.Entry) MemoryEntry {
	return MemoryEntry{
		EntryID: e.ID, Owner: e.Owner, Scope: string(e.Scope), ScopeRef: e.ScopeRef,
		Layer: string(e.Layer), Kind: string(e.Kind), Title: e.Title,
		Content: e.Content, FileRef: e.FilePath, FileCommit: e.FileCommit,
		TopicKey: e.TopicKey, Selectors: e.Selectors,
		Status: string(e.Status), Tombstone: e.Tombstone, TombstoneNote: e.TombstoneNote,
		Provenance: MemoryProvenance{
			Origin: string(e.Origin), OriginRef: e.OriginRef, ProposerModel: e.ProposerModel,
			ApprovedBy: e.ApprovedBy, ApprovedTS: e.ApprovedTS,
			Version: e.Version, Supersedes: e.Supersedes,
		},
		Verification: MemoryVerification{
			VerifiedBy: e.VerifiedBy, VerifiedTS: e.VerifiedTS,
			ReverifyIntervalDays: e.ReverifyIntervalDays, ExpiresTS: e.ExpiresTS,
		},
		LastInjectedTS: e.LastInjectedTS, CreatedTS: e.CreatedTS, UpdatedTS: e.UpdatedTS,
	}
}

// ── GET /api/memory ─────────────────────────────────────────────────────────

// MemoryList is the browse read with its S15.3 tail cursor.
type MemoryList struct {
	Entries []MemoryEntry `json:"entries"`
	// Visibility states the rule that produced this set, because "why is that
	// entry not here?" is a question a person asks of a memory surface and the
	// honest answer is a sentence rather than a silent omission.
	Visibility string `json:"visibility"`
	// Projects names the project scopes the caller can see, so the answer can be
	// read against the registry membership it rests on.
	Projects  []string `json:"projects"`
	Cursor    int64    `json:"cursor"`
	Truncated bool     `json:"truncated"`
}

// S4.1 (your own scopes), S4.5 (house defaults address everyone), S09.6
// (project scope follows membership) and D10 — house authority is authority
// over the HOUSE scope, never a read into a person's own notes (CONVENTIONS
// §40-C's content-vs-telemetry line). Said plainly for the person reading it.
const memoryVisibilityRule = "everything you have saved, in every place you own it, plus anything saved for the whole household " +
	"(house notes are meant for everyone), plus the notes of projects you own or have been invited to. " +
	"Being the household's operator does not open another person's own notes: what someone writes down is theirs, and running the " +
	"platform is the right to curate the household's shared notes, not to read anybody's."

func (s *Server) handleMemoryList(w http.ResponseWriter, r *http.Request) {
	if !s.memoryReady(w) || !s.projReady(w) {
		return
	}
	limit, err := readLimit(r)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	if limit > memoryListCap {
		limit = memoryListCap
	}
	q := memory.BrowseQuery{Limit: limit + 1} // one beyond, so Truncated is OBSERVED
	for _, f := range []struct {
		name       string
		vocabulary map[string]bool
		dst        *string
		what       string
	}{
		{"scope", memoryScopes, (*string)(&q.Scope), "scope"},
		{"kind", memoryKinds, (*string)(&q.Kind), "kind"},
		{"status", memoryStatuses, (*string)(&q.Status), "status"},
	} {
		v := strings.TrimSpace(r.URL.Query().Get(f.name))
		if v == "" {
			continue
		}
		if !f.vocabulary[v] {
			s.writeSurface(w, nil, badRequest(fmt.Sprintf(
				// S09.2 fixes each of these vocabularies; the set is closed.
				"unknown %s %q: %s is chosen from a fixed set, and this is not one of them", f.what, v, f.what)))
			return
		}
		*f.dst = v
	}

	id, _ := IdentityFrom(r.Context())
	q.Viewer = id.UserID
	projects, err := s.visibleProjects(r.Context(), id.UserID)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	q.Projects = projects
	entries, err := s.memory.ListVisible(r.Context(), q)
	if err != nil {
		s.writeSurface(w, nil, s.memoryErr(err))
		return
	}
	cursor, err := s.head(r.Context())
	if err != nil {
		s.writeSurface(w, nil, fmt.Errorf("read head cursor: %w", err))
		return
	}
	out := MemoryList{
		Entries: []MemoryEntry{}, Visibility: memoryVisibilityRule,
		Projects: projects, Cursor: cursor,
	}
	if out.Projects == nil {
		out.Projects = []string{}
	}
	for _, e := range entries {
		if len(out.Entries) >= limit {
			out.Truncated = true
			break
		}
		out.Entries = append(out.Entries, memoryEntryView(e))
	}
	s.writeReadJSON(w, out)
}

// ── GET /api/memory/{entry} ─────────────────────────────────────────────────

// MemoryEntryDetail is one entry with the S09.7 edges the READER is the
// addressee of. The same shape answers a create and a new version, so a writer
// whose entry raised a question they own sees it without a second read — and a
// writer whose write raised a question addressed to SOMEONE ELSE does not,
// because that card is that person's to see and to answer (drain D2).
//
// The influence list is deliberately NOT here: it is a scan over every run's
// trace manifest (S09.5), which is the removal verb's answer to "what did this
// touch?" rather than something every read of an entry should pay for.
type MemoryEntryDetail struct {
	Entry MemoryEntry `json:"entry"`
	// Conflicts are the OPEN edges naming this entry on either side, surfaced
	// and never silently resolved (S09.7). Resolving one is its own verb.
	Conflicts []memory.Conflict `json:"conflicts"`
	Cursor    int64             `json:"cursor"`
}

func (s *Server) handleMemoryDetail(w http.ResponseWriter, r *http.Request) {
	e, ok := s.memoryScopeRead(w, r)
	if !ok {
		return
	}
	s.writeMemoryDetail(w, r, e)
}

// writeMemoryDetail serves an entry with the open conflict edges THIS caller is
// the addressee of.
//
// The filter is the drain-D2 correction: a house or project entry is visible to
// many people, but an edge against it carries the id, kind, version and question
// text OF THE OTHER ENTRY, which is frequently somebody's personal one. Serving
// the edge to every viewer of the shared entry would leak that person's
// metadata to readers who cannot act on it anyway — the resolve verb is the
// affected owner's alone. Same predicate, both places (mayAnswerConflict), so
// the card and the door agree by construction (the §39 D9 shape).
func (s *Server) writeMemoryDetail(w http.ResponseWriter, r *http.Request, e memory.Entry) {
	all, err := s.memory.EntryConflicts(r.Context(), e.ID)
	if err != nil {
		s.writeSurface(w, nil, s.memoryErr(err))
		return
	}
	conflicts := []memory.Conflict{}
	for _, c := range all {
		if mayAnswerConflict(c, s.callerID(r)) {
			conflicts = append(conflicts, c)
		}
	}
	cursor, err := s.head(r.Context())
	if err != nil {
		s.writeSurface(w, nil, fmt.Errorf("read head cursor: %w", err))
		return
	}
	s.writeReadJSON(w, MemoryEntryDetail{
		Entry: memoryEntryView(e), Conflicts: conflicts, Cursor: cursor,
	})
}

// ── POST /api/memory + POST /api/memory/{entry}/new-version ─────────────────

// memoryWriteBody is one human-supplied entry as it arrives. It is the S09.4
// station-3 draft: the gate validates the scope, the kind, the title and the
// body with its own rules and its own messages, and this transport adds only
// what is genuinely the transport's — which origin ceremony is being performed,
// and whether the caller may perform it.
type memoryWriteBody struct {
	Scope      string           `json:"scope"`
	ScopeRef   string           `json:"scope_ref,omitempty"`
	Kind       string           `json:"kind"`
	Title      string           `json:"title"`
	Content    string           `json:"content"`
	TopicKey   string           `json:"topic_key,omitempty"`
	Selectors  memory.Selectors `json:"selectors,omitempty"`
	FileBacked bool             `json:"file_backed,omitempty"`
	FileName   string           `json:"file_name,omitempty"`
	// Origin is empty (= human_direct) or "imported" for the N20 house-KB
	// import, which is the operator's ceremony and nobody else's.
	Origin    string `json:"origin,omitempty"`
	OriginRef string `json:"origin_ref,omitempty"`
}

// draft maps the body onto the gate's own input type.
func (b memoryWriteBody) draft() memory.Draft {
	return memory.Draft{
		Scope: memory.Scope(b.Scope), ScopeRef: b.ScopeRef, Kind: memory.Kind(b.Kind),
		Title: b.Title, Content: b.Content, TopicKey: b.TopicKey, Selectors: b.Selectors,
		FileBacked: b.FileBacked, FileName: b.FileName,
	}
}

// TIER READING (recorded): an own-store write is Medium (S09.4 station 3, S15.2
// memory row). Medium demands no PIN step-up — that is High's, and High is the
// outward-effect tier — and it is never batched (S15.6), which is why no batch
// verb exists here. The manual human-direct write IS the individual owner
// approval: the person is at the surface, deciding, and the write records them
// as both approver and verifier at that instant (the operator-import-is-the-
// approval precedent, S09.4).
func (s *Server) handleMemoryCreate(w http.ResponseWriter, r *http.Request) {
	// The DB handle is required for the same reason the reads require it: the
	// project-scope limb of this family's authority is a registry read, and a
	// write whose scope cannot be resolved is refused rather than allowed
	// through unscoped (the projReady precedent).
	if !s.memoryReady(w) || !s.projReady(w) {
		return
	}
	body, actor, ok := s.memoryWriteRequest(w, r)
	if !ok {
		return
	}
	var (
		e   memory.Entry
		err error
	)
	if body.Origin == originImported {
		e, err = s.memGate.Import(r.Context(), actor, body.draft(), body.OriginRef)
	} else {
		e, err = s.memGate.CreateManual(r.Context(), actor, body.draft())
	}
	if err != nil {
		s.writeSurface(w, nil, s.memoryErr(err))
		return
	}
	// Nothing is minted here: the gate already landed the row, its file, any
	// conflict edge and the canonical `knowledge.write` event in ONE
	// transaction, and an act with a landed canonical row is never double-minted
	// (§39 OQ8).
	s.writeMemoryDetail(w, r, e)
}

// handleMemoryNewVersion is the S4.1 correction right: an edit is a NEW version
// carrying supersedes_id, and the superseded row is retired in the same
// transaction. In-place mutation is not offered here because it does not exist
// — the migration triggers abort it even for a raw-SQL holder (S09.4 station 4;
// §17).
func (s *Server) handleMemoryNewVersion(w http.ResponseWriter, r *http.Request) {
	// The supersession target is resolved under the read scope first, so an
	// unknown id 404s before anything else and another person's entry can never
	// become an existence oracle.
	old, ok := s.memoryScopeRead(w, r)
	if !ok {
		return
	}
	body, actor, ok := s.memoryWriteRequest(w, r)
	if !ok {
		return
	}
	e, err := s.memGate.NewVersion(r.Context(), actor, old.ID, body.draft())
	if err != nil {
		s.writeSurface(w, nil, s.memoryErr(err))
		return
	}
	s.writeMemoryDetail(w, r, e)
}

// memoryWriteRequest reads and validates the parts of a write that belong to
// the transport: the body parses, the origin is one this surface performs, and
// a project-scope write names a project the caller belongs to.
func (s *Server) memoryWriteRequest(w http.ResponseWriter, r *http.Request) (memoryWriteBody, string, bool) {
	raw, ok := s.readBody(w, r)
	if !ok {
		return memoryWriteBody{}, "", false
	}
	var body memoryWriteBody
	if err := json.Unmarshal(raw, &body); err != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_body", Msg: err.Error()})
		return memoryWriteBody{}, "", false
	}
	id, _ := IdentityFrom(r.Context())
	switch body.Origin {
	case "", originHumanDirect:
		body.Origin = originHumanDirect
	case originImported:
		// The N20 house-KB import: the operator's import IS the approval
		// (S09.4/S09.10), so nobody else performs it and nobody else stamps an
		// entry with an origin that says a curator vouched for it.
		if !s.readScope(r).Operator {
			s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusForbidden, Code: "forbidden",
				// S09.4: the operator's import IS the approval; a member's own
				// entry is human_direct.
				Msg: "only the household's operator can mark an entry as imported — importing it is itself the act of vouching for it. An entry you write yourself is recorded as written by you."})
			return memoryWriteBody{}, "", false
		}
	default:
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_request",
			// S09.4's origin vocabulary; adopted_from/proposed_from arrive later.
			Msg: fmt.Sprintf("unknown origin %q: this door writes %q (something you wrote) or %q (the operator's import). "+
				"Adopting somebody else's entry and having the platform propose one are not built yet",
				body.Origin, originHumanDirect, originImported)})
		return memoryWriteBody{}, "", false
	}
	if body.Scope == string(memory.ScopeProject) {
		if !s.callerInProject(w, r, body.ScopeRef) {
			return memoryWriteBody{}, "", false
		}
	}
	return body, id.UserID, true
}

// callerInProject refuses a project-scope write into a project the caller does
// not belong to. This is scope MAPPING, not a second authority implementation:
// the gate owns whether a write is permitted at all (D10 house promotion, the
// humans-only rule, the dormant scope), and the transport owns whether the
// caller has any relationship to the subject they named — the same fail-closed
// question authorizeOwner answers everywhere else on this API.
//
// It matters because project-scope knowledge injects into every run of that
// project (S05.4 selection is by scope_ref), so an unscoped write would be a
// way to steer other people's runs.
func (s *Server) callerInProject(w http.ResponseWriter, r *http.Request, projectID string) bool {
	if strings.TrimSpace(projectID) == "" {
		// The gate refuses a project write with no project, with its own
		// message; the membership question does not arise.
		return true
	}
	id, _ := IdentityFrom(r.Context())
	projects, err := s.visibleProjects(r.Context(), id.UserID)
	if err != nil {
		s.writeSurface(w, nil, err)
		return false
	}
	for _, p := range projects {
		if p == projectID {
			return true
		}
	}
	s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusForbidden, Code: "forbidden",
		// S09.6: project knowledge is shared by scope membership.
		Msg: fmt.Sprintf("project %q is not one you own or have been invited to. What is saved on a project is shared with everyone in it "+
			"and goes into every piece of work it does", projectID)})
	return false
}

// ── POST /api/memory/{entry}/remove ─────────────────────────────────────────

// MemoryRemoved is the removal outcome with its influence list.
type MemoryRemoved struct {
	EntryID string `json:"entry_id"`
	Status  string `json:"status"`
	// Influence is the S2.9 forward trace: the runs whose trace manifest
	// contains this entry, computed as a join over what was already logged at
	// injection — not a system, and not a side store (S09.5).
	Influence []memory.InfluenceRef `json:"influence"`
	Detail    string                `json:"detail"`
}

// S09.5: selection is deterministic, so removal takes effect exactly and at
// once; true deletion stays the owner's separate right.
const removedDetail = "removed: from now on this entry goes into nothing the platform does — the choice of what to include is exact, so " +
	"removal is exact too. Its text stays in the project's history and its record stays for the audit trail; erasing it for good is a " +
	"separate right, and it is yours. The list below names the work this entry was fed into."

// handleMemoryRemove routes Gate.Remove. Removal is owner-or-operator (§17
// reading 4) and the gate enforces that itself; the influence list comes back
// from the same call and is served verbatim.
func (s *Server) handleMemoryRemove(w http.ResponseWriter, r *http.Request) {
	e, ok := s.memoryScopeRead(w, r)
	if !ok {
		return
	}
	body, ok := s.memoryReason(w, r)
	if !ok {
		return
	}
	influence, err := s.memGate.Remove(r.Context(), s.callerID(r), e.ID, body.Reason)
	if err != nil {
		s.writeSurface(w, nil, s.memoryErr(err))
		return
	}
	if influence == nil {
		influence = []memory.InfluenceRef{}
	}
	s.writeReadJSON(w, MemoryRemoved{
		EntryID: e.ID, Status: string(memory.StatusRemoved),
		Influence: influence, Detail: removedDetail,
	})
}

// ── POST /api/memory/{entry}/delete ─────────────────────────────────────────

// MemoryDeleted is the true-deletion outcome: the tombstone, and the honest
// limits of what a purge can reach.
type MemoryDeleted struct {
	EntryID   string `json:"entry_id"`
	Tombstone bool   `json:"tombstone"`
	Note      string `json:"tombstone_note"`
	Detail    string `json:"detail"`
	// Limits are the S09.5 honest limits, stated rather than implied: what
	// deletion does not and cannot undo.
	Limits []string `json:"limits"`
}

const deletedDetail = "content purged: the working-tree file is gone and the row's content, title, topic key and selectors are cleared. " +
	"What remains is the tombstone stub — id, dates, \"removed at owner request\" (G2 Def.9)."

var deletionLimits = []string{
	"the knowledge file's git history still holds the content until a git-history purge is run, which is an S13 operation on the scope's knowledge dir",
	// S02.1 snapshot mechanics, D9/S09.5 history integrity, S09.5's
	// retrievable-text-only shape (nothing is ever trained into a model).
	"the database gives the freed space back at its next scheduled snapshot",
	"work you already accepted while this entry was live stays exactly as it was — the platform does not rewrite what happened",
	"nothing has to be un-learned from a model: nothing here is ever trained into one, so an entry only ever existed as text that could be looked up, handed over, and removed",
}

// handleMemoryDelete routes Gate.TrueDelete — STRICTLY the owner's right (S4.1;
// G2 Def.9). The gate refuses everyone else INCLUDING the operator, and this
// transport does not soften that by adding an operator limb: the house
// authority bit is not a right to purge another person's memory.
func (s *Server) handleMemoryDelete(w http.ResponseWriter, r *http.Request) {
	e, ok := s.memoryScopeRead(w, r)
	if !ok {
		return
	}
	if err := s.memGate.TrueDelete(r.Context(), s.callerID(r), e.ID); err != nil {
		s.writeSurface(w, nil, s.memoryErr(err))
		return
	}
	// Read back so the answer describes the stub that now exists rather than the
	// request that produced it (the repeated call is idempotent in the gate: an
	// already-purged entry is already a tombstone).
	stub, err := s.memory.Get(r.Context(), e.ID)
	if err != nil {
		s.writeSurface(w, nil, s.memoryErr(err))
		return
	}
	s.writeReadJSON(w, MemoryDeleted{
		EntryID: stub.ID, Tombstone: stub.Tombstone, Note: stub.TombstoneNote,
		Detail: deletedDetail, Limits: deletionLimits,
	})
}

// ── POST /api/memory/conflicts/{conflict}/resolve ───────────────────────────

// MemoryConflictResolved is the closed question.
type MemoryConflictResolved struct {
	Conflict memory.Conflict `json:"conflict"`
	Detail   string          `json:"detail"`
}

// S09.7: closing the question changes neither entry — a correction is its own
// recorded write, which is what stops a conflict being settled silently.
const conflictResolvedDetail = "the question is closed. Closing it records who answered and when; it does not change either entry — " +
	"correcting one, retiring it, or replacing it with a newer one is a separate change you make on purpose, and that is what keeps " +
	"a disagreement from being settled quietly."

// conflictStatusOpen is the stored status a resolvable edge carries (S09.7; the
// 0004 CHECK admits `open` and `resolved`).
const conflictStatusOpen = "open"

const conflictAlreadyClosedDetail = "this question was already closed, by the person and at the time the edge records. Nothing was written again."

// mayAnswerConflict is the ONE expression of who a question card belongs to: the
// affected owner it was addressed to (S09.7). It decides both whether the edge
// SHOWS on an entry detail and whether the resolve verb accepts it, so a
// surfaced card and a working door can never diverge — and the operator has no
// limb, for the same reason they have none on true deletion (OQ5).
func mayAnswerConflict(c memory.Conflict, caller string) bool { return c.Affected == caller }

// handleMemoryConflictResolve routes Gate.ResolveConflict, scoped to the
// AFFECTED OWNER: the S09.7 card is a question addressed to one person, and it
// is theirs to answer. The operator has no limb here for the same reason they
// have none on true deletion — house authority is not access to another
// person's memory decisions (OQ5).
//
// The act's audit is the conflict row itself, which records resolved_by and
// resolved_ts. Nothing is co-minted on top of it: the durable record of who
// closed the question already exists, and a second one would be the double-mint
// shape (§39 OQ8).
func (s *Server) handleMemoryConflictResolve(w http.ResponseWriter, r *http.Request) {
	if !s.memoryReady(w) {
		return
	}
	raw := r.PathValue("conflict")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		s.writeSurface(w, nil, badRequest(fmt.Sprintf("bad conflict id %q: a conflict is identified by its row number", raw)))
		return
	}
	c, err := s.memory.Conflict(r.Context(), id)
	found := !errors.Is(err, memory.ErrNotFound)
	if err != nil && found {
		s.writeSurface(w, nil, s.memoryErr(err))
		return
	}
	// 404 before 403: an unknown id must not become an existence oracle.
	if !found {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusNotFound, Code: "not_found", Msg: "conflict not found"})
		return
	}
	if !mayAnswerConflict(c, s.callerID(r)) {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusForbidden, Code: "forbidden",
			// S09.7: the affected entry's owner answers the conflict.
			Msg: "this question is addressed to the person whose entry it is about; it is theirs to answer"})
		return
	}
	// Resolved first: a repeat serves the already-closed question back rather
	// than a refusal, so a retried tap can never read as a failure (S15.2).
	if c.Status != conflictStatusOpen {
		s.writeReadJSON(w, MemoryConflictResolved{Conflict: c, Detail: conflictAlreadyClosedDetail})
		return
	}
	if err := s.memGate.ResolveConflict(r.Context(), s.callerID(r), id); err != nil {
		s.writeSurface(w, nil, s.memoryErr(err))
		return
	}
	resolved, err := s.memory.Conflict(r.Context(), id)
	if err != nil {
		s.writeSurface(w, nil, s.memoryErr(err))
		return
	}
	s.writeReadJSON(w, MemoryConflictResolved{Conflict: resolved, Detail: conflictResolvedDetail})
}

// ── shared scope, readiness + error mapping ─────────────────────────────────

// memoryReasonBody carries the optional free-text reason a removal records.
type memoryReasonBody struct {
	Reason string `json:"reason,omitempty"`
}

func (s *Server) memoryReason(w http.ResponseWriter, r *http.Request) (memoryReasonBody, bool) {
	raw, ok := s.readBody(w, r)
	if !ok {
		return memoryReasonBody{}, false
	}
	var body memoryReasonBody
	if len(strings.TrimSpace(string(raw))) == 0 {
		return body, true
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_body", Msg: err.Error()})
		return memoryReasonBody{}, false
	}
	return body, true
}

func (s *Server) callerID(r *http.Request) string {
	id, _ := IdentityFrom(r.Context())
	return id.UserID
}

// memoryReady reports both handles, or writes the not-wired answer. Reads and
// writes are gated together on the one-family-one-readiness rule (§40-B): a
// browse list whose every entry's verbs answer 503 is a worse answer than
// saying the family is not composed in this process.
func (s *Server) memoryReady(w http.ResponseWriter) bool {
	if s.memory == nil || s.memGate == nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusServiceUnavailable, Code: "not_wired",
			Msg: "the S09 memory store and its write gate are not wired in this process"})
		return false
	}
	return true
}

// memoryScopeRead resolves the path's entry and the caller's visibility over it
// in the ONE landed shape: 404 before 403 (§38).
func (s *Server) memoryScopeRead(w http.ResponseWriter, r *http.Request) (memory.Entry, bool) {
	if !s.memoryReady(w) || !s.projReady(w) {
		return memory.Entry{}, false
	}
	e, err := s.memory.Get(r.Context(), r.PathValue("entry"))
	found := !errors.Is(err, memory.ErrNotFound)
	if err != nil && found {
		s.writeSurface(w, nil, s.memoryErr(err))
		return memory.Entry{}, false
	}
	if !found {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusNotFound, Code: "not_found", Msg: "memory entry not found"})
		return memory.Entry{}, false
	}
	visible, err := s.memoryVisible(r.Context(), e, s.callerID(r))
	if err != nil {
		s.writeSurface(w, nil, err)
		return memory.Entry{}, false
	}
	if !visible {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusForbidden, Code: "forbidden", Msg: "forbidden"})
		return memory.Entry{}, false
	}
	return e, true
}

// memoryVisible answers the OQ5 predicate for ONE entry by DELEGATING to the
// store's single-entry read, which shares its SQL fragment with the set read.
// The transport therefore holds no copy of the rule: a detail read and a list
// cannot disagree about what a person may see, structurally rather than by two
// implementations being written to match (drain D3).
func (s *Server) memoryVisible(ctx context.Context, e memory.Entry, viewer string) (bool, error) {
	projects, err := s.visibleProjects(ctx, viewer)
	if err != nil {
		return false, err
	}
	return s.memory.Visible(ctx, e.ID, viewer, projects)
}

// visibleProjects names the S13.7 projects a person owns or is an invited
// member of — the scope-membership input to the project-scope limb above.
//
// It DELEGATES to the projects family's registry read (projects.go), which is
// the same bounded SELECT this function used to hold plus the columns that
// family serves, and — the point of the delegation — the same single expression
// of who may see a project. Two copies of that rule in one package is one copy
// too many: a list and a scope check that disagree is a leak, and the way to
// make them agree is to have one of them (P3-RW-2 R2, the drain-D3 rule).
//
// The registry row is still read as bounded SQL rather than through
// internal/project, which internal/api imports in neither direction (the
// accept's protected-ref read is the landed precedent, §40-B).
func (s *Server) visibleProjects(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.visibleProjectRows(ctx, userID, "")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, row := range rows {
		out = append(out, row.entry.ProjectID)
	}
	return out, nil
}

// memoryErr maps the gate's and the store's refusals to statuses ON THE ERROR'S
// TYPE, never on its message text (§38's ban on error-string matching).
//
// Each caller-fault limb carries the GATE'S OWN message, because the gate knows
// why it refused and a paraphrase here would be a second, worse explanation of
// a wall this transport does not own. An unmarked error is a platform fault —
// the safe default — and answers 500 with the real cause in the ops log.
func (s *Server) memoryErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, memory.ErrNotFound):
		return &SurfaceError{Status: http.StatusNotFound, Code: "not_found", Msg: err.Error()}
	case errors.Is(err, memory.ErrNotHuman),
		errors.Is(err, memory.ErrOperatorRequired),
		errors.Is(err, memory.ErrNotOwner):
		// The three authority walls of S09.1/S09.4: the actor must hold a users
		// row (so the dev-posture identity can never write memory), house scope
		// demands the operator (D10), and a verb reserved to the owner is the
		// owner's.
		return &SurfaceError{Status: http.StatusForbidden, Code: "forbidden", Msg: err.Error()}
	case errors.Is(err, memory.ErrScopeDormant),
		errors.Is(err, memory.ErrLayerForbidden),
		errors.Is(err, memory.ErrInvalidEntry):
		// Dormancy and the absent L1 writer are the S09.4 activation table
		// showing through, and they read as what they are: a request for
		// something that does not exist at v0, with the reason.
		return &SurfaceError{Status: http.StatusBadRequest, Code: "bad_request", Msg: err.Error()}
	case errors.Is(err, memory.ErrNotActive):
		// The entry is not in a state this verb applies to — a removed entry
		// removed again, a tombstone superseded. The caller resolves it by
		// re-reading, which is what a conflict status says.
		return &SurfaceError{Status: http.StatusConflict, Code: "conflict", Msg: err.Error()}
	}
	s.logger.Error("memory: the gate refused for a reason it did not class as the caller's", "err", err)
	return &SurfaceError{Status: http.StatusInternalServerError, Code: "internal",
		Msg: "the memory operation did not complete"}
}
