package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/worker"
)

// workforce_test.go — GET /api/workforce (B6-8 part B, R11–R14 + R19).
//
// The read is served over the fixture world, because that world is the only one
// where workers exist through their REAL producers: a template's equipment is
// parsed from a hash-verified file on every read, so a hand-seeded roster row
// would point at a file nothing wrote and the S08.3 check would refuse it.

// workforceRead does the read as `who` and decodes it.
func workforceRead(t *testing.T, b *backend, who string) api.WorkforceView {
	t.Helper()
	rr := httptest.NewRecorder()
	fixtureServer(t, b, who).Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/api/workforce", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/workforce as %s: status %d: %s", who, rr.Code, rr.Body.String())
	}
	var v api.WorkforceView
	if err := json.Unmarshal(rr.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode workforce view: %v", err)
	}
	return v
}

func workforceIDs(v api.WorkforceView) []string {
	out := make([]string, 0, len(v.Workers))
	for _, w := range v.Workers {
		out = append(out, w.TemplateID)
	}
	return out
}

func workforceWorkerByID(t *testing.T, v api.WorkforceView, id string) api.WorkforceWorker {
	t.Helper()
	for _, w := range v.Workers {
		if w.TemplateID == id {
			return w
		}
	}
	t.Fatalf("the served roster carries no worker %q (has %v)", id, workforceIDs(v))
	return api.WorkforceWorker{}
}

func workforceVersion(t *testing.T, w api.WorkforceWorker, versionID string) api.WorkforceVersion {
	t.Helper()
	for _, v := range w.Versions {
		if v.VersionID == versionID {
			return v
		}
	}
	t.Fatalf("worker %q carries no version %q", w.TemplateID, versionID)
	return api.WorkforceVersion{}
}

// ── R11: owner scope, three ways ────────────────────────────────────────────

// TestWorkforceRosterScopeIsThreeWay is the leak test, and the MEMBER limb is
// the one it exists for: "operator sees all" is easy to get right and easy to
// over-apply, and the failure mode is one member reading another member's
// personal worker. So all three callers are driven against the same world:
//
//	op     — the whole registry (a worker is audited machinery under D10 acts)
//	alice  — her own personal draft plus the HOUSEHOLD roster
//	bob    — his own personal automation plus the household roster, and NOT
//	         alice's personal draft
//
// The two members' answers are asymmetric in both directions, which is what
// makes this more than a count.
func TestWorkforceRosterScopeIsThreeWay(t *testing.T) {
	b := fixtureWorld(t)

	op := workforceIDs(workforceRead(t, b, "op"))
	for _, want := range []string{fxWorkerNotes, fxWorkerDigest, fxWorkerAudit} {
		if !contains(op, want) {
			t.Errorf("the operator's roster is missing %q: %v", want, op)
		}
	}

	alice := workforceIDs(workforceRead(t, b, "alice"))
	if !contains(alice, fxWorkerNotes) {
		t.Errorf("alice cannot see the HOUSEHOLD worker: %v", alice)
	}
	if !contains(alice, fxWorkerAudit) {
		t.Errorf("alice cannot see her OWN personal draft: %v", alice)
	}
	if contains(alice, fxWorkerDigest) {
		t.Errorf("alice can see BOB's personal worker — the S15.10 identity rule leaked: %v", alice)
	}

	bob := workforceIDs(workforceRead(t, b, "bob"))
	if !contains(bob, fxWorkerNotes) {
		t.Errorf("bob cannot see the household worker: %v", bob)
	}
	if !contains(bob, fxWorkerDigest) {
		t.Errorf("bob cannot see his OWN personal automation: %v", bob)
	}
	if contains(bob, fxWorkerAudit) {
		t.Errorf("bob can see ALICE's personal draft: %v", bob)
	}

	// The scope statement is served rather than inferred, because a member's
	// answer and the operator's are different READINGS and neither may pose as
	// the other.
	if got := workforceRead(t, b, "alice").RosterScope; !strings.Contains(got, "household") {
		t.Errorf("a member's roster does not say what it covers: %q", got)
	}
	if got := workforceRead(t, b, "op").RosterScope; !strings.Contains(got, "every worker") {
		t.Errorf("the operator's roster does not say what it covers: %q", got)
	}
}

// TestWorkforceRosterScopePredicateCanRefuse is the non-tautological control on
// the test above: with the member predicate DEFEATED — the query run as the
// operator would run it — bob's personal automation reaches alice. Without this
// the three-way test could be passing because the fixture world happens to
// carry what it carries.
func TestWorkforceRosterScopePredicateCanRefuse(t *testing.T) {
	b := fixtureWorld(t)
	st := fixtureWorkforce(t, b)

	scoped, err := st.Roster(t.Context(), worker.RosterQuery{Viewer: "alice", Limit: 50})
	if err != nil {
		t.Fatalf("scoped roster: %v", err)
	}
	unscoped, err := st.Roster(t.Context(), worker.RosterQuery{Viewer: "alice", Operator: true, Limit: 50})
	if err != nil {
		t.Fatalf("unscoped roster: %v", err)
	}
	has := func(es []worker.RosterEntry, id string) bool {
		for _, e := range es {
			if e.Template.ID == id {
				return true
			}
		}
		return false
	}
	if has(scoped, fxWorkerDigest) {
		t.Error("the member predicate admitted another member's personal worker")
	}
	if !has(unscoped, fxWorkerDigest) {
		t.Fatal("dropping the member predicate changes nothing — the scoped test above proves nothing")
	}
	// And a roster read with no viewer is refused rather than answered
	// unscoped: a read that cannot be scoped must not serve.
	if _, err := st.Roster(t.Context(), worker.RosterQuery{Limit: 10}); err == nil {
		t.Error("a viewerless roster read was answered — an unscoped read must fail closed (S01.9)")
	}
}

// TestWorkforceRosterRendersEveryIdentityFact is R11: the identity, status,
// domain MATURITY, active version with its provenance, and the supervised
// first-N counter are all served — and a DRAFT serves the honest absences
// instead of a blank.
func TestWorkforceRosterRendersEveryIdentityFact(t *testing.T) {
	b := fixtureWorld(t)
	v := workforceRead(t, b, "op")

	notes := workforceWorkerByID(t, v, fxWorkerNotes)
	if notes.Name == "" || notes.Owner != "alice" || notes.Scope != string(worker.ScopeHousehold) ||
		notes.Kind != string(worker.KindAgentic) || notes.Status != string(worker.StatusActive) {
		t.Errorf("the household worker's identity block is incomplete: %+v", notes)
	}
	if notes.Domain.Maturity != string(worker.MaturityFull) {
		t.Errorf("the software domain must render FULL, got %q", notes.Domain.Maturity)
	}
	active := workforceVersion(t, notes, fxWorkerNotesV2)
	if !active.Active || active.ApprovedBy == "" || active.ApprovedTS == "" || active.AuthorKind == "" ||
		active.Origin == "" || active.FileSHA256 == "" {
		t.Errorf("the active version's provenance block is incomplete: %+v", active)
	}
	if active.GraduatedTS != "" {
		t.Errorf("nothing graduated this version, so the stamp must be absent: %q", active.GraduatedTS)
	}
	if active.Granted == nil || active.Granted.FirstNRemaining <= 0 {
		t.Fatalf("the supervised first-N counter must be served for a not-yet-graduated version: %+v", active.Granted)
	}

	// The DEGRADED domain is a structural fact, and the S08.7 consequence rides
	// with it: this worker cannot deliver without a person looking.
	digest := workforceWorkerByID(t, v, fxWorkerDigest)
	if digest.Domain.Maturity != string(worker.MaturityDegraded) {
		t.Errorf("the chore domain must render DEGRADED, got %q", digest.Domain.Maturity)
	}
	if !digest.Delivery.RequiresReview {
		t.Error("a degraded-domain worker must render as requiring review (S08.7 is structural, not advisory)")
	}
	if !hasReasonAbout(digest.Delivery.Reasons, "degraded") {
		t.Errorf("the degraded consequence carries no reason: %v", digest.Delivery.Reasons)
	}

	// The draft: no active version, so no definition and nothing granted — each
	// with its reason, because "nothing equipped" and "not read" are different
	// facts (§42 honest absence).
	draft := workforceWorkerByID(t, v, fxWorkerAudit)
	if draft.Definition != nil {
		t.Errorf("a draft has no active version, so it can have no parsed definition: %+v", draft.Definition)
	}
	if draft.DefinitionAbsent == "" {
		t.Error("the draft's missing definition renders with no reason")
	}
	if draft.ActiveVersion != "" {
		t.Errorf("a draft names an active version: %q", draft.ActiveVersion)
	}
	dv := workforceVersion(t, draft, fxWorkerAuditV1)
	if dv.Granted != nil || dv.GrantedAbsent == "" {
		t.Errorf("an unapproved version must serve NO granted block plus the reason: %+v / %q", dv.Granted, dv.GrantedAbsent)
	}
	if dv.Validation != nil || dv.ValidationAbsent == "" {
		t.Errorf("a version whose battery never ran must serve no validation plus the reason: %+v / %q",
			dv.Validation, dv.ValidationAbsent)
	}
	// The composer provenance the draft was born with renders as served.
	if dv.AuthorKind != "composer" || dv.Composer == "" || dv.PlaybookVersion == "" {
		t.Errorf("the composer provenance block is incomplete: %+v", dv)
	}
}

// TestWorkforceEquipmentKeepsRequestedAndGrantedApart is R12: the S08.2 split
// is the point, so the file's REQUESTED equipment and the version's GRANTED
// guardrails are separate served blocks a surface can render side by side.
func TestWorkforceEquipmentKeepsRequestedAndGrantedApart(t *testing.T) {
	b := fixtureWorld(t)
	notes := workforceWorkerByID(t, workforceRead(t, b, "op"), fxWorkerNotes)
	if notes.Definition == nil {
		t.Fatalf("the active worker serves no definition: %q", notes.DefinitionAbsent)
	}
	req := notes.Definition.RequestedEquipment
	if len(req.Tools) == 0 || len(req.Skills) == 0 || len(req.Knowledge) == 0 {
		t.Errorf("the requested equipment block is incomplete: %+v", req)
	}
	active := workforceVersion(t, notes, fxWorkerNotesV2)
	if active.Granted == nil {
		t.Fatal("the active version serves no granted guardrails")
	}
	if len(active.Granted.GrantedTools) == 0 || active.Granted.ConfinementClass == "" || active.Granted.Egress == "" {
		t.Errorf("the granted permissions block is incomplete: %+v", active.Granted)
	}
	// The two blocks are reachable under DIFFERENT keys on the wire — which is
	// what makes them distinguishable on screen rather than merged into one list.
	raw := workforceRawBody(t, b, "op")
	for _, key := range []string{`"requested_equipment"`, `"requested_grants"`, `"granted"`} {
		if !strings.Contains(raw, key) {
			t.Errorf("the served body carries no %s key — requested and granted would be indistinguishable", key)
		}
	}
	// The equipment vocabulary is the REGISTRY's own. A "helpers" key would be
	// fabricated: the S08.1 schema has no helper field, and helper selection is
	// per-run through the routing pipeline (S08.8 step 5).
	if strings.Contains(raw, `"helpers"`) || strings.Contains(raw, `"helper_roster"`) {
		t.Error("the served body carries a helper list — the S08.1 schema has no helper field, so it was invented")
	}
}

// TestWorkforceAutomationServesItsStepChain is R13: an automation's multi-stage
// chain is the registry's own data, in order, with the explicit approval node
// marked — and an AGENTIC worker serves its selectors and profile instead,
// because those are what its "connections" are.
func TestWorkforceAutomationServesItsStepChain(t *testing.T) {
	b := fixtureWorld(t)
	v := workforceRead(t, b, "op")

	digest := workforceWorkerByID(t, v, fxWorkerDigest)
	if digest.Definition == nil || digest.Definition.Workflow == nil {
		t.Fatalf("the automation serves no parsed workflow: %+v", digest.Definition)
	}
	wf := digest.Definition.Workflow
	if wf.Dialect == "" || wf.Service != "calendar" {
		t.Errorf("the workflow header is incomplete: %+v", wf)
	}
	if len(wf.Steps) != 2 {
		t.Fatalf("the workflow serves %d steps, want the 2 the definition declares", len(wf.Steps))
	}
	if wf.Steps[0].ID != "fetch" || wf.Steps[1].ID != "post" {
		t.Errorf("the step chain is out of order: %+v", wf.Steps)
	}
	if wf.Steps[0].Approval {
		t.Error("the read step is marked as an approval node — the marking would mean nothing if every step carried it")
	}
	if !wf.Steps[1].Approval {
		t.Error("the OUTWARD step's approval node is not marked (S08.9; D7)")
	}
	// Step ARGS are deliberately not on the wire: the map renders how stages
	// connect, and an arg is a value flowing between them.
	if strings.Contains(workforceRawBody(t, b, "op"), `"args"`) {
		t.Error("the workflow serves step args — a view-only map must not surface payload values")
	}

	// The agentic worker's own multi-stage facts: what selects it and how it runs.
	notes := workforceWorkerByID(t, v, fxWorkerNotes)
	if notes.Definition.Workflow != nil {
		t.Error("an agentic worker has no dialect body, so it must serve no workflow")
	}
	if notes.Definition.Selectors.Family == "" || len(notes.Definition.Selectors.TaskClasses) == 0 ||
		notes.Definition.Profile.Duty == "" || notes.Definition.Profile.EffortFloor == "" {
		t.Errorf("the agentic worker serves no selector/profile facts: %+v", notes.Definition)
	}
}

// ── R14: the version→outcome join ───────────────────────────────────────────

// TestWorkforceVersionOutcomesJoinIsExercisedBothWays is the hazard this test
// exists for: if no run in the world were ever routed to a worker VERSION, the
// join would never run and "empty" would be indistinguishable from "broken". So
// the world carries both — a version WITH routed runs and a version without —
// and both arms are asserted, on the same worker.
func TestWorkforceVersionOutcomesJoinIsExercisedBothWays(t *testing.T) {
	b := fixtureWorld(t)
	notes := workforceWorkerByID(t, workforceRead(t, b, "op"), fxWorkerNotes)

	v2 := workforceVersion(t, notes, fxWorkerNotesV2)
	if len(v2.Outcomes.Runs) != 3 {
		t.Fatalf("v2 serves %d routed runs, want the 3 the world routed to it: %+v", len(v2.Outcomes.Runs), v2.Outcomes.Runs)
	}
	if v2.Outcomes.Absent != "" {
		t.Errorf("a version WITH outcomes must not also carry an absence reason: %q", v2.Outcomes.Absent)
	}
	// Newest routing decision first, so the reading opens on what happened last.
	if v2.Outcomes.Runs[0].RunID != "r-stall" {
		t.Errorf("outcomes are not newest-first: %s", v2.Outcomes.Runs[0].RunID)
	}
	// The recorded routing facts ride each row: the cause and the plain reason
	// are what make a routing decision auditable at all (S08.8 accountability).
	for _, r := range v2.Outcomes.Runs {
		if r.Cause == "" || r.PlainReason == "" || r.RoutedTS == "" || r.Owner == "" {
			t.Errorf("a routed run is missing its recorded routing facts: %+v", r)
		}
	}

	// The SAME worker's superseded version, joined separately: one run, not
	// three. A join keyed on the worker rather than the version would have put
	// all four here.
	v1 := workforceVersion(t, notes, fxWorkerNotesV1)
	if len(v1.Outcomes.Runs) != 1 || v1.Outcomes.Runs[0].RunID != "r-audit" {
		t.Fatalf("v1 serves %+v, want only the one run routed to IT", v1.Outcomes.Runs)
	}

	// The empty arm, with its reason. At v0 this is what most versions carry and
	// it is the truthful render — but a zero-length list saying nothing would
	// read as a measurement somebody took.
	draft := workforceWorkerByID(t, workforceRead(t, b, "op"), fxWorkerAudit)
	dv := workforceVersion(t, draft, fxWorkerAuditV1)
	if len(dv.Outcomes.Runs) != 0 || dv.Outcomes.Absent == "" {
		t.Errorf("a version nothing was routed to must serve an empty list WITH its reason: %+v / %q",
			dv.Outcomes.Runs, dv.Outcomes.Absent)
	}
}

// TestWorkforceOutcomeVerdictsAndMoneyAreServedOrHonestlyAbsent is R14's other
// half: a verdict and a meter reading each render as served, and each renders
// its own absence with a reason rather than as a zero.
func TestWorkforceOutcomeVerdictsAndMoneyAreServedOrHonestlyAbsent(t *testing.T) {
	b := fixtureWorld(t)
	notes := workforceWorkerByID(t, workforceRead(t, b, "op"), fxWorkerNotes)
	v2 := workforceVersion(t, notes, fxWorkerNotesV2)

	var withVerdict, withoutVerdict int
	for _, r := range v2.Outcomes.Runs {
		switch {
		case len(r.Verdicts) > 0:
			withVerdict++
			if r.VerdictsAbsent != "" {
				t.Errorf("run %s carries verdicts AND an absence reason", r.RunID)
			}
			if r.Verdicts[0].Verdict == "" || r.Verdicts[0].Round == 0 || r.Verdicts[0].TS == "" {
				t.Errorf("run %s serves an empty verdict row: %+v", r.RunID, r.Verdicts[0])
			}
		default:
			withoutVerdict++
			if r.VerdictsAbsent == "" {
				t.Errorf("run %s has no verdict and no reason for it", r.RunID)
			}
		}
	}
	if withVerdict == 0 || withoutVerdict == 0 {
		t.Fatalf("both verdict arms must be driven: %d with, %d without", withVerdict, withoutVerdict)
	}

	// Money. The one run the metering seam answers for is on the superseded
	// version; every other routed run gets the honest nil plus its reason. A
	// zero would be a reading nobody took (§37).
	v1 := workforceVersion(t, notes, fxWorkerNotesV1)
	if len(v1.Outcomes.Runs) == 0 {
		t.Fatal("the superseded version serves no routed run, so the money arm below asserts nothing")
	}
	paid := v1.Outcomes.Runs[0]
	if paid.APIEquivCostUSD == nil || paid.Tokens == nil {
		t.Fatalf("the metered run serves no figures: %+v", paid)
	}
	want, err := fixtureMeter{}.RunMeter(t.Context(), paid.RunID)
	if err != nil {
		t.Fatalf("the fixture meter refuses %s, so this asserts nothing: %v", paid.RunID, err)
	}
	if *paid.APIEquivCostUSD != want.APIEquivCostUSD || *paid.Tokens != want.Tokens {
		t.Errorf("the served figures are not the seam's own: got %v/%v want %v/%v",
			*paid.Tokens, *paid.APIEquivCostUSD, want.Tokens, want.APIEquivCostUSD)
	}
	if paid.MeterAbsent != "" {
		t.Errorf("a run WITH a reading also carries an absence reason: %q", paid.MeterAbsent)
	}
	var unmetered int
	for _, r := range v2.Outcomes.Runs {
		if r.APIEquivCostUSD == nil {
			unmetered++
			if r.MeterAbsent == "" {
				t.Errorf("run %s has no meter reading and no reason for it", r.RunID)
			}
		}
	}
	if unmetered == 0 {
		t.Fatal("no routed run lacks a meter reading, so the honest-nil arm is undriven")
	}
}

// TestWorkforceOutcomeScopeFollowsTheRUNsOwner is the second leak limb, and it
// is a DIFFERENT rule from the roster's: a member reads the household worker,
// and the runs routed to it still belong to whoever launched them (D2 — the
// fleet's "accounts are never summed" applied to a version's history). So
// alice's reading of the SHARED worker carries her runs and not bob's.
func TestWorkforceOutcomeScopeFollowsTheRUNsOwner(t *testing.T) {
	b := fixtureWorld(t)

	mine := workforceVersion(t, workforceWorkerByID(t, workforceRead(t, b, "alice"), fxWorkerNotes), fxWorkerNotesV2)
	all := workforceVersion(t, workforceWorkerByID(t, workforceRead(t, b, "op"), fxWorkerNotes), fxWorkerNotesV2)
	if len(all.Outcomes.Runs) <= len(mine.Outcomes.Runs) {
		t.Fatalf("the operator's reading (%d runs) is not wider than the member's (%d) — the scope is not applied",
			len(all.Outcomes.Runs), len(mine.Outcomes.Runs))
	}
	for _, r := range mine.Outcomes.Runs {
		if r.Owner != "alice" {
			t.Errorf("alice's reading carries %s's run %s", r.Owner, r.RunID)
		}
	}
	// And the version she CAN see but whose only run is somebody else's renders
	// the honest absence rather than an unexplained empty.
	v1 := workforceVersion(t, workforceWorkerByID(t, workforceRead(t, b, "alice"), fxWorkerNotes), fxWorkerNotesV1)
	if len(v1.Outcomes.Runs) != 0 || v1.Outcomes.Absent == "" {
		t.Errorf("a version whose only run is another owner's must render an explained absence: %+v / %q",
			v1.Outcomes.Runs, v1.Outcomes.Absent)
	}
	if got := workforceRead(t, b, "alice").OutcomeScope; !strings.Contains(got, "your own runs") {
		t.Errorf("a member's reading does not say whose runs it covers: %q", got)
	}
	if got := workforceRead(t, b, "op").OutcomeScope; !strings.Contains(got, "every owner") {
		t.Errorf("the operator's reading does not say whose runs it covers: %q", got)
	}
}

// ── R19: the checkable negatives ────────────────────────────────────────────

// TestWorkforceRouteIsReadOnly is S15.10's "the v0 surface has no mutation
// affordances", enforced at the transport: /api/workforce answers GET and
// nothing else, and no worker-mutating route exists anywhere on the API.
func TestWorkforceRouteIsReadOnly(t *testing.T) {
	b := fixtureWorld(t)
	srv := fixtureServer(t, b, "op")
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, httptest.NewRequest(method, "/api/workforce", strings.NewReader(`{}`)))
		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /api/workforce: want 405, got %d: %s", method, rr.Code, rr.Body.String())
		}
	}
	// The shapes a worker-mutation verb would most likely take, refused by name.
	// This is a NAMED-SHAPE check and not an enumeration of the route table: a
	// 404 satisfies it as surely as a 405 does, which is the point — none of
	// these paths exists, and editing through the map is parked to 15.5.
	for _, p := range []string{
		"/api/workforce/wt-notes", "/api/workforce/wt-notes/approve",
		"/api/workers", "/api/workers/wt-notes",
	} {
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, httptest.NewRequest("POST", p, strings.NewReader(`{}`)))
		if rr.Code == http.StatusOK {
			t.Errorf("POST %s answered 200 — the workforce map performs no act", p)
		}
	}
}

// TestWorkforceSurfaceCallsNoWorkerVerb is the same negative read off the
// SOURCE, because a route table proves what is registered and not what the
// handler calls. internal/api may hold the worker store, and the one thing it
// may not do is write through it.
func TestWorkforceSurfaceCallsNoWorkerVerb(t *testing.T) {
	// The worker store's mutating verbs, by name (internal/worker: lifecycle.go,
	// store.go, battery.go, composer.go).
	verbs := []string{
		"CreateDraft(", "NewVersion(", "Approve(", "Promote(", "Repoint(", "Retire(",
		"FlagByModel(", "FlagByEnginePin(", "Revalidate(", "RunBattery(", "Compose(",
		"CreateDomain(", "SetDomainMaturity(", "RecordGap(", "SetGapDisposition(",
		"InstallSkill(", "RunAutomation(",
	}
	src := readSourceFile(t, "workforce.go")
	for _, v := range verbs {
		if strings.Contains(src, v) {
			t.Errorf("workforce.go calls %s — the map is view-only (S15.10 parks editing to 15.5)", v)
		}
	}
	// The probe: the scan can fail. Without it "no hits" would prove only that
	// the list of names is unreachable from this file.
	if !strings.Contains("g, err := s.workforce.Approve(ctx, actor, id, worker.ApproveOpts{})", verbs[2]) {
		t.Fatal("the verb scan cannot detect its own probe — it would pass vacuously")
	}
	// And the whole of internal/api holds the store for READING only: the field
	// is reachable from exactly the one file that serves the read.
	files, err := filepathGlobGo(t)
	if err != nil {
		t.Fatal(err)
	}
	var holders []string
	for _, name := range files {
		if name == "workforce.go" || name == "api.go" {
			continue
		}
		if strings.Contains(readSourceFile(t, name), "s.workforce") {
			holders = append(holders, name)
		}
	}
	if len(holders) > 0 {
		t.Errorf("the worker store is reached from %v — it has exactly one consumer, the read", holders)
	}
}

// TestWorkforceMoneyIsReadNeverComputed extends the R8 negative onto this
// surface specifically. The package-wide scan (meters_test.go) already forbids
// token arithmetic and the metering import; what this adds is the shape a
// version→outcome view invites — a per-version TOTAL, which is a sum the client
// asked for and the server must refuse to invent.
func TestWorkforceMoneyIsReadNeverComputed(t *testing.T) {
	src := readSourceFile(t, "workforce.go")
	// No accumulation of a money figure, in any of the three shapes Go writes it.
	sums := regexp.MustCompile(`(?i)(total|sum|spend|aggregate)[a-z_]*\s*(\+=|=\s*[a-z_.]*\s*\+)`)
	if m := sums.FindString(src); m != "" {
		t.Errorf("workforce.go accumulates a figure (%q) — money is READ per run and never totalled", m)
	}
	if !sums.MatchString("totalUSD += m.APIEquivCostUSD") {
		t.Fatal("the summing scan cannot detect its own probe — it would pass vacuously")
	}
	// And no total field exists to fill, checked over a REAL served body rather
	// than over a zero-valued struct: a total added anywhere — to the view, to a
	// worker, to a version or to a run — would appear here, and a zero struct
	// with `omitempty` fields would have hidden every one of them.
	b := fixtureWorld(t)
	raw := strings.ToLower(workforceRawBody(t, b, "op"))
	if !strings.Contains(raw, "api_equiv_cost_usd") {
		t.Fatal("the served body carries no money key at all, so the scan below passes vacuously")
	}
	// KEY position only. A worker's task class is legitimately "summarize" and a
	// scan that read a VALUE as a field name would have to be turned off within
	// a week — the §42 lesson about a scan that cannot tell a path from a
	// division, in a second shape.
	aggregate := regexp.MustCompile(`"[a-z_]*(total|sum|average|mean|aggregate)[a-z_]*"\s*:`)
	if m := aggregate.FindString(raw); m != "" {
		t.Errorf("the served body carries the field %s — a cross-run figure is arithmetic nobody performed", m)
	}
	// Probes in BOTH directions, so the key/value distinction stays deliberate.
	if !aggregate.MatchString(`{"runs":[],"total_usd":1.5}`) {
		t.Fatal("the aggregate scan cannot detect its own probe — it would pass vacuously")
	}
	if aggregate.MatchString(`{"task_classes":["review","summarize"]}`) {
		t.Fatal("the aggregate scan reads a VALUE as a field name — it would be noise, not a check")
	}
	if !strings.Contains(src, "RunMeter(ctx") {
		t.Error("the outcome rows do not read the metering seam — every dollar figure here must come off RunMeter as served")
	}
}

// TestWorkforceRefusesWhenNotWired is the not-wired posture: the registry is nil
// under injected admission, and the route says so rather than serving an empty
// roster — "no workers" and "no registry" are different facts.
func TestWorkforceRefusesWhenNotWired(t *testing.T) {
	b := newBackend(t)
	seedUser(t, b, "op", "operator")
	srv := api.New(api.Config{
		Log: b.log, Sessions: b.store, Auth: fixedIdentity{"op"}, Settings: approvalSettings(),
		HealthFn: func() api.Health { return api.Health{Ready: true} }, DB: b.db, Meter: fakeMeter{},
		// Workforce deliberately nil.
	})
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/api/workforce", nil))
	if rr.Code != http.StatusServiceUnavailable || !strings.Contains(rr.Body.String(), "not_wired") {
		t.Errorf("with no registry wired: want 503 not_wired, got %d: %s", rr.Code, rr.Body.String())
	}
	// Session-required and unversioned, like every other read family.
	closed := api.New(api.Config{
		Log: b.log, Sessions: b.store, Settings: approvalSettings(),
		HealthFn: func() api.Health { return api.Health{Ready: true} }, DB: b.db, Meter: fakeMeter{},
	})
	rr = httptest.NewRecorder()
	closed.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/api/workforce", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("with no identity: want 401, got %d", rr.Code)
	}
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/api/v1/workforce", nil))
	if rr.Code != http.StatusNotFound {
		t.Errorf("/api/v1/workforce: a version prefix must not exist, got %d", rr.Code)
	}
}

// TestWorkforceEmptyRegistryRendersAsAnAnswer is the v0 reality: a fresh host
// has NO workers (compose-when-earned, S08.6), so the read has to answer with an
// empty roster and its scope statement — never a refusal, and never a nil array
// a client would have to guess about.
func TestWorkforceEmptyRegistryRendersAsAnAnswer(t *testing.T) {
	b := newBackend(t)
	seedUser(t, b, "alice", "member")
	st, err := worker.NewStore(worker.Config{
		DB: b.db, Log: b.log, Settings: b.reg, Root: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("worker.NewStore: %v", err)
	}
	srv := api.New(api.Config{
		Log: b.log, Sessions: b.store, Auth: fixedIdentity{"alice"}, Settings: approvalSettings(),
		HealthFn: func() api.Health { return api.Health{Ready: true} }, DB: b.db, Meter: fakeMeter{},
		Workforce: st,
	})
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/api/workforce", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("an empty registry must answer 200: got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"workers":[]`) {
		t.Errorf("an empty roster must serve [] rather than null: %s", rr.Body.String())
	}
	var v api.WorkforceView
	if err := json.Unmarshal(rr.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v.RosterScope == "" || v.OutcomeScope == "" {
		t.Error("an empty answer still has to say what it covered")
	}
}

// ── helpers ─────────────────────────────────────────────────────────────────

func workforceRawBody(t *testing.T, b *backend, who string) string {
	t.Helper()
	rr := httptest.NewRecorder()
	fixtureServer(t, b, who).Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/api/workforce", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/workforce as %s: %d", who, rr.Code)
	}
	return rr.Body.String()
}

func contains(hay []string, needle string) bool {
	for _, s := range hay {
		if s == needle {
			return true
		}
	}
	return false
}

func hasReasonAbout(reasons []string, word string) bool {
	for _, r := range reasons {
		if strings.Contains(r, word) {
			return true
		}
	}
	return false
}

// filepathGlobGo lists this package's non-test sources, so a scan over "every
// file in internal/api" is a checked list rather than a hand-kept one.
func filepathGlobGo(t *testing.T) ([]string, error) {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("read the package dir: %w", err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		out = append(out, e.Name())
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no sources found — a scan over them would pass vacuously")
	}
	return out, nil
}

// TestWorkforceMeterEmptyReadingIsAnAbsenceNotAZero is the production-shape
// test, and it exists because the fixture world could not have caught this on
// its own.
//
// `Ledger.RunConsumption` folds a run's checkpoint rows. A run that exists and
// has recorded no usage yet — every run between its routing decision and its
// first checkpoint — folds to zero tokens, zero cost and zero unpriced calls
// with NO error, so a consumer treating `err == nil` as "there is a reading"
// serves `USD 0` for work nobody has measured. The fixture meter now answers
// exactly that shape for `r-claim`, which is why the three arms below are three
// DIFFERENT served states rather than two.
func TestWorkforceMeterEmptyReadingIsAnAbsenceNotAZero(t *testing.T) {
	b := fixtureWorld(t)
	notes := workforceWorkerByID(t, workforceRead(t, b, "op"), fxWorkerNotes)
	runs := map[string]api.WorkforceRoutedRun{}
	for _, v := range notes.Versions {
		for _, r := range v.Outcomes.Runs {
			runs[r.RunID] = r
		}
	}

	// Arm 1 — a real reading, served as served.
	if r, ok := runs["r-audit"]; !ok || r.APIEquivCostUSD == nil {
		t.Fatalf("the metered run serves no figure: %+v", r)
	}
	// Arm 2 — the seam refused: no reading exists.
	if r, ok := runs["r-notes"]; !ok || r.APIEquivCostUSD != nil ||
		!strings.Contains(r.MeterAbsent, "no meter reading") {
		t.Errorf("a refused meter read must serve nil plus its reason: %+v", runs["r-notes"])
	}
	// Arm 3 — the seam ANSWERED with nothing recorded. This is the one that
	// would have shipped a zero.
	empty, ok := runs["r-claim"]
	if !ok {
		t.Fatal("r-claim is not among the routed runs, so the empty-reading arm is undriven")
	}
	if empty.Tokens != nil || empty.APIEquivCostUSD != nil {
		t.Errorf("an empty meter reading was served as a FIGURE (%v / %v) — that is a measurement nobody took (§37)",
			empty.Tokens, empty.APIEquivCostUSD)
	}
	if !strings.Contains(empty.MeterAbsent, "no usage recorded") {
		t.Errorf("the empty reading does not say what it is: %q", empty.MeterAbsent)
	}
	// The three reasons are DISTINCT, so a reader can tell the three states
	// apart rather than seeing one word for three different facts.
	if runs["r-notes"].MeterAbsent == empty.MeterAbsent {
		t.Error("a refusal and an empty reading serve the same reason — they are different facts")
	}

	// The non-tautological control: the seam really does answer this shape
	// without an error, so the arm is a real production state and not a
	// fixture-only curiosity.
	m, err := fixtureMeter{}.RunMeter(t.Context(), "r-claim")
	if err != nil {
		t.Fatalf("the fixture meter refuses r-claim, so arm 3 is testing the refusal path: %v", err)
	}
	if m.Tokens != 0 || m.APIEquivCostUSD != 0 || m.Unpriced {
		t.Fatalf("the fixture meter does not answer the empty shape: %+v", m)
	}
}

// TestWorkforceVerdictScanIsBoundedPerRunNotPerRequest is what makes "no verdict
// recorded" a statement a reader can trust.
//
// A global `LIMIT n` over a multi-run scan lets one talkative run consume the
// budget, and the runs it starves then render an absence the READING invented.
// The scan is partitioned per run instead, so every run's newest rounds are
// reachable regardless of what its siblings carry. Driven by giving one run more
// rounds than the bound and asserting a sibling with ONE round still gets it —
// which is exactly the row a global bound would have dropped, since all the
// noisy rounds are newer.
func TestWorkforceVerdictScanIsBoundedPerRunNotPerRequest(t *testing.T) {
	b := fixtureWorld(t)
	// r-notes already carries one verdict (event_seq low). r-claim gets more
	// rounds than the per-run bound, all of them newer.
	for i := 1; i <= 25; i++ {
		exec(t, b, `INSERT INTO run_events (run_id, generation, user_id, type, schema_version, payload, ts)
		            VALUES (?,0,?,?,1,?,?)`, "r-claim", "alice", "verdict.recorded",
			fmt.Sprintf(`{"round":%d,"verdict":"REWORK","domain":"software","retention":"keep-forever","golden_set":{}}`, i),
			fxT4)
	}
	notes := workforceWorkerByID(t, workforceRead(t, b, "op"), fxWorkerNotes)
	byRun := map[string]api.WorkforceRoutedRun{}
	for _, v := range notes.Versions {
		for _, r := range v.Outcomes.Runs {
			byRun[r.RunID] = r
		}
	}

	// The starved run under a global bound. It has ONE round, and every one of
	// r-claim's 25 is newer.
	quiet := byRun["r-notes"]
	if len(quiet.Verdicts) != 1 {
		t.Fatalf("r-notes lost its verdict to another run's rows — the scan is not partitioned: %+v", quiet)
	}
	if quiet.VerdictsAbsent != "" {
		t.Errorf("a run WITH a verdict also carries an absence reason: %q", quiet.VerdictsAbsent)
	}

	// The talkative run: bounded, newest rounds kept, and it says it was cut.
	loud := byRun["r-claim"]
	if len(loud.Verdicts) != 20 {
		t.Fatalf("the per-run bound served %d rounds, want 20", len(loud.Verdicts))
	}
	if !loud.VerdictsTruncated {
		t.Error("a run whose rounds were cut does not say so — its newest reads as all of them")
	}
	if loud.Verdicts[0].Round != 6 || loud.Verdicts[19].Round != 25 {
		t.Errorf("the bound kept the wrong rounds: first %d last %d, want 6 and 25",
			loud.Verdicts[0].Round, loud.Verdicts[19].Round)
	}
	// A run at or under the bound is NOT marked cut, so the flag means something.
	if quiet.VerdictsTruncated {
		t.Error("a run with one round is marked truncated")
	}
}
