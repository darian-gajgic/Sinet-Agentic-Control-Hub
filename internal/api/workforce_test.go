package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker/automation"
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
	// The step-to-step EDGE, which is the one thing S15.10 actually asks the
	// chain for. `post` depends on `fetch` and that dependency is written down in
	// exactly one place — the `$from` reference in its args — so a chain served
	// without it is a sequence with approval flags and no connections at all.
	if got := wf.Steps[1].Needs["digest"]; got != "steps.fetch.summary" {
		t.Errorf("the outward step's edge to %q is missing: needs = %v", "fetch", wf.Steps[1].Needs)
	}
	if got := wf.Steps[0].Needs["day"]; got != "payload.day" {
		t.Errorf("the read step's payload edge is missing: needs = %v", wf.Steps[0].Needs)
	}
	// LITERAL args are still off the wire — a reference is a connection, a
	// literal is a value, and only the first is this surface's to show. The raw
	// body carries no `args` key, which is what the `needs` projection replaced.
	if strings.Contains(workforceRawBody(t, b, "op"), `"args"`) {
		t.Error("the workflow serves raw step args — only $from REFERENCES belong on a view-only map")
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

	// THE CONTROL, AND IT IS ASKED OF THE REAL LEDGER. The claim arm 3 rests on
	// is a claim about PRODUCTION — that `Ledger.RunConsumption` answers a run
	// with no checkpoints successfully, with everything folded to zero — and
	// asking `fixtureMeter{}` about it would only re-read the stub that was
	// written to match the claim. `r-claim` is a real run row in this world with
	// no checkpoints, so the real ledger answers it.
	rc, err := metering.NewLedger(b.db, nil, metering.MeteredExceptions{}, nil).
		RunConsumption(t.Context(), "r-claim")
	if err != nil {
		t.Fatalf("the REAL ledger refuses a run with no checkpoints, so arm 3 is not a production state: %v", err)
	}
	if rc.TotalPricedUSD != 0 || rc.TotalUnpricedCalls != 0 || len(rc.Items) != 0 {
		t.Fatalf("the real ledger does not fold an unmeasured run to zero: %+v", rc)
	}
	// And the discriminator that tells this apart from a measured zero. It is
	// what `api.RunMeter.Calls` carries, and it is zero here precisely because
	// nothing has been metered — a run whose only work priced a true local $0
	// would fold to the same figures with a NON-zero count.
	if rc.TotalCalls != 0 {
		t.Fatalf("an unmeasured run folded %d calls — the empty reading is not empty", rc.TotalCalls)
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

// TestWorkforceStepReferencesKeepEdgesAndDropLiterals is the other half of D3:
// the selection rule is "a reference is a connection, a literal is a value", so
// both directions have to be driven. A definition carrying one of each proves
// the projection selects rather than simply copying or simply dropping.
func TestWorkforceStepReferencesKeepEdgesAndDropLiterals(t *testing.T) {
	wf, err := automation.Parse(`{"dialect":"` + automation.DialectVersion + `","service":"calendar","steps":[
	  {"id":"fetch","verb":"calendar.list","args":{"day":{"$from":"payload.day"},"limit":25,"label":"daily"}},
	  {"id":"post","verb":"calendar.post","args":{"digest":{"$from":"steps.fetch.summary"},"channel":"notes"},"approval":true}
	]}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := map[string]map[string]string{}
	for _, st := range wf.Steps {
		got[st.ID] = worker.StepReferences(st)
	}
	if want := map[string]string{"day": "payload.day"}; !sameRefs(got["fetch"], want) {
		t.Errorf("fetch: got %v, want only the reference %v", got["fetch"], want)
	}
	if want := map[string]string{"digest": "steps.fetch.summary"}; !sameRefs(got["post"], want) {
		t.Errorf("post: got %v, want only the reference %v", got["post"], want)
	}
	// The literals are the control: a projection that copied everything would
	// carry them, and one that dropped everything would carry nothing.
	for _, literal := range []string{"limit", "label", "channel"} {
		for id, refs := range got {
			if _, ok := refs[literal]; ok {
				t.Errorf("step %s serves the LITERAL arg %q — a value is not a connection", id, literal)
			}
		}
	}
	if len(got["fetch"]) == 0 || len(got["post"]) == 0 {
		t.Fatal("no references survived at all, so the literal check above proves nothing")
	}
}

// TestWorkforceStepEdgesUseTheDialectsOwnPredicate is the rest of D3, and it is
// the half the first fix got wrong in BOTH directions at once.
//
// The map's one job here is to say how the stages connect, so "is this argument
// an edge" must be the same question the EXECUTOR asks. It was not: the reader
// ran a stricter decoder of its own. An argument carrying a sibling key beside
// `$from` parses (Step.Args is raw JSON, so the document loader's strictness
// never reaches inside an argument value) and RESOLVES — a genuine step-to-step
// dependency — and the reader showed it as no connection at all. And
// `{"$from":""}`, which the executor passes through as the literal it is,
// showed up as an edge pointing nowhere. Both are false statements about how
// the procedure connects, which is worse than the missing-edges defect D3 fixed.
//
// Driven through the real dialect end to end: the document parses, the executor
// resolves the annotated reference to the earlier step's output, and the
// reader's answer is asserted against that.
func TestWorkforceStepEdgesUseTheDialectsOwnPredicate(t *testing.T) {
	wf, err := automation.Parse(`{"dialect":"` + automation.DialectVersion + `","service":"calendar","steps":[
	  {"id":"fetch","verb":"calendar.list","args":{"day":{"$from":"payload.day"}}},
	  {"id":"note","verb":"calendar.note","args":{
	     "digest":{"$from":"steps.fetch.summary","note":"annotated"},
	     "blank":{"$from":""}}}
	]}`)
	if err != nil {
		t.Fatalf("the dialect refuses a reference carrying a sibling key, so there is nothing to disagree about: %v", err)
	}

	// The executor's own answer, taken from the executor. `note` returns the arg
	// it was handed, so the report says what `digest` resolved to.
	verbs := automation.VerbMap{
		"calendar.list": {Fn: func(context.Context, map[string]json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"summary":"THE DAY"}`), nil
		}},
		"calendar.note": {Fn: func(_ context.Context, args map[string]json.RawMessage) (json.RawMessage, error) {
			return args["digest"], nil
		}},
	}
	rep, err := automation.Execute(t.Context(), automation.ExecInput{
		Workflow: wf, Payload: json.RawMessage(`{"day":"monday"}`), Verbs: verbs, UserID: "alice",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var resolved string
	for _, st := range rep.Steps {
		if st.ID == "note" {
			resolved = string(st.Output)
		}
	}
	if resolved != `"THE DAY"` {
		t.Fatalf("the executor did not resolve the annotated reference (got %s), so this test is not about a real edge", resolved)
	}

	refs := worker.StepReferences(wf.Steps[1])
	if refs["digest"] != "steps.fetch.summary" {
		t.Errorf("the reader shows no connection for an argument the executor resolves from step `fetch`: %v", refs)
	}
	if _, ok := refs["blank"]; ok {
		t.Errorf("`{\"$from\":\"\"}` is a LITERAL to the executor and rendered as an edge to nowhere: %v", refs)
	}
}

func sameRefs(got, want map[string]string) bool {
	if len(got) != len(want) {
		return false
	}
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}

// TestMeterReadingIsOneExpressionAcrossEverySurface is D2's regression, and it
// is deliberately about the SEAM rather than about one view.
//
// `Ledger.RunConsumption` answers a run that has recorded no usage with zeros
// and NO error. Three landed surfaces read that seam — the task list's
// `cost_so_far_usd`, the run card's counters, and the workforce map's routed
// runs — and before this drain each decided independently what `err == nil`
// meant. Two of them got it wrong. The check is that all three now answer the
// same way for the same run.
func TestMeterReadingIsOneExpressionAcrossEverySurface(t *testing.T) {
	b := fixtureWorld(t)

	// r-claim: the seam answers, with nothing recorded. Every surface must
	// render the ABSENCE, and none may print a zero.
	var tasks api.TaskList
	mustDecode(t, fixtureBody(t, b, "op", "/api/tasks"), &tasks)
	var claim *api.TaskListRun
	for _, it := range tasks.Tasks {
		if it.LatestRun != nil && it.LatestRun.RunID == "r-claim" {
			claim = it.LatestRun
		}
	}
	if claim == nil {
		t.Fatal("r-claim is not on the task list, so this asserts nothing")
	}
	if claim.CostSoFarUSD != nil {
		t.Errorf("the task card serves a cost for a run the ledger folds to nothing: %v — that is a fabricated USD %v",
			*claim.CostSoFarUSD, *claim.CostSoFarUSD)
	}

	var detail api.RunDetail
	mustDecode(t, fixtureBody(t, b, "op", "/api/runs/r-claim"), &detail)
	if detail.Card.Counters.APIEquivCostUSD != nil {
		t.Errorf("the run card serves a cost for the same run: %v", *detail.Card.Counters.APIEquivCostUSD)
	}
	// Tokens stay a plain counter beside it: zero consumed IS a true reading.
	if detail.Card.Counters.Tokens != 0 {
		t.Errorf("tokens should read 0 for a run that consumed nothing, got %d", detail.Card.Counters.Tokens)
	}

	notes := workforceWorkerByID(t, workforceRead(t, b, "op"), fxWorkerNotes)
	for _, v := range notes.Versions {
		for _, r := range v.Outcomes.Runs {
			if r.RunID == "r-claim" && r.APIEquivCostUSD != nil {
				t.Errorf("the map serves a cost for the same run: %v", *r.APIEquivCostUSD)
			}
		}
	}

	// The other direction, so this is not just "everything is nil": r-ship has a
	// real reading and every surface that shows one shows it.
	var ship api.RunDetail
	mustDecode(t, fixtureBody(t, b, "op", "/api/runs/r-ship"), &ship)
	if ship.Card.Counters.APIEquivCostUSD == nil {
		t.Fatal("r-ship has a real meter reading and the run card dropped it — the fix over-applied")
	}
	if *ship.Card.Counters.APIEquivCostUSD == 0 {
		t.Error("a real reading rendered as zero")
	}
}

// TestRunCardCarriesTheUnpricedMarkingWithTheFigure is the fabricated-zero class
// one surface over from where this packet kept finding it.
//
// A subscription lane prices UNPRICED, so the seam's cost is 0 and the run card
// printed a bare `USD 0` — a figure that says the run was free when what is true
// is that nobody priced it. The workforce map's routed rows already carried the
// marking; `RunCounters` did not, so the ONE-expression claim `meterReading`
// makes across three surfaces was not true of this one. Additive: the same
// field, the same meaning, beside the same figure.
func TestRunCardCarriesTheUnpricedMarkingWithTheFigure(t *testing.T) {
	read := func(m api.MeterReader) api.RunCounters {
		t.Helper()
		b := newBackend(t)
		seedRun(t, b, "r1", "u1", "", "running", "anthropic")
		_, ts := newTestServer(t, serverOpts{b: b, meter: m})
		resp, err := ts.Client().Get(ts.URL + "/api/runs/r1")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET /api/runs/r1 = %d", resp.StatusCode)
		}
		var detail api.RunDetail
		if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return detail.Card.Counters
	}

	// cost 0 with tokens: fakeMeter marks that unpriced, which is the real
	// subscription-lane shape.
	unpriced := read(fakeMeter{tokens: 1234, cost: 0})
	if unpriced.APIEquivCostUSD == nil {
		t.Fatal("an unpriced reading IS a reading and its figure was withheld — the wrong half went missing")
	}
	if *unpriced.APIEquivCostUSD != 0 {
		t.Fatalf("the unpriced figure is %v, not the 0 the seam served", *unpriced.APIEquivCostUSD)
	}
	if !unpriced.Unpriced {
		t.Error("the card serves USD 0 with nothing saying the lane is unpriced — that reads as free (§37)")
	}

	// The other direction, so the field is a consequence rather than a constant.
	priced := read(fakeMeter{tokens: 900, cost: 0.42})
	if priced.Unpriced {
		t.Error("a priced reading is marked unpriced")
	}
}

// TestMeteredZeroIsAReadingAndUnmeteredIsNot is the sign-flipped half of the
// packet's headline defect, and it is the one that withholds a TRUE figure.
//
// `meterReading` keyed on folded MAGNITUDES, and the ledger deliberately prices
// a local duty call a true $0 on the permanent free tier (the zero-allowance
// row, S12.1). So a run whose recorded work was local folds to zero tokens,
// zero cost and zero unpriced calls — a real measurement, indistinguishable at
// the seam from a run nobody has touched, and served as "no reading". The
// discriminator is how many rows the ledger FOLDED, which the seam now carries.
//
// Both halves are driven, and the first is asked of the REAL ledger because the
// claim being relied on is a claim about production.
func TestMeteredZeroIsAReadingAndUnmeteredIsNot(t *testing.T) {
	b := newBackend(t)
	seedRun(t, b, "r-local", "u1", "", "running", "anthropic")
	seedRun(t, b, "r-untouched", "u1", "", "running", "anthropic")
	seq := appendRun(t, b, "u1", "r-local", "run.state_changed", `{"to":"running"}`)
	// A local duty call's checkpoint: the wire contract internal/local writes,
	// with no tokens on it. This is the row that folds to a true $0.
	exec(t, b, `INSERT INTO checkpoints (run_id, user_id, event_seq, usage_json, session_substrate, session_id, model_id, created_ts)
	            VALUES (?,?,?,?,'claude-cli',?,?,?)`,
		"r-local", "u1", seq,
		`{"input_tokens":0,"output_tokens":0,"local":{"lane":"local","duty":"summarize","model":"qwen","model_sha256":"sha","engine_build":"b"}}`,
		"sid-local", "qwen", nowTS())

	ledger := metering.NewLedger(b.db, nil, metering.MeteredExceptions{}, nil)
	local, err := ledger.RunConsumption(t.Context(), "r-local")
	if err != nil {
		t.Fatalf("real ledger, local run: %v", err)
	}
	// The shape that defeats a magnitude test: measured, and it came to zero.
	if local.TotalCalls != 1 {
		t.Fatalf("the local row was not folded (%d calls) — this test is not about a measured run", local.TotalCalls)
	}
	if local.TotalPricedUSD != 0 || local.TotalUnpricedCalls != 0 {
		t.Fatalf("the local row did not price a true $0: %+v", local)
	}
	untouched, err := ledger.RunConsumption(t.Context(), "r-untouched")
	if err != nil {
		t.Fatalf("real ledger, untouched run: %v", err)
	}
	if untouched.TotalCalls != 0 {
		t.Fatalf("a run with no checkpoints folded %d calls", untouched.TotalCalls)
	}

	// And the seam's answer for each, through the card. `callsMeter` reports
	// exactly what the shell's projMeter reports off the numbers above.
	card := func(runID string, m api.MeterReader) api.RunCounters {
		t.Helper()
		_, ts := newTestServer(t, serverOpts{b: b, meter: m})
		resp, err := ts.Client().Get(ts.URL + "/api/runs/" + runID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		defer resp.Body.Close()
		var detail api.RunDetail
		if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return detail.Card.Counters
	}
	measured := card("r-local", callsMeter{calls: local.TotalCalls})
	if measured.APIEquivCostUSD == nil {
		t.Error("a MEASURED zero was served as an absence — the run's local work really did cost nothing and the card will not say so")
	} else if *measured.APIEquivCostUSD != 0 {
		t.Errorf("the measured zero came back as %v", *measured.APIEquivCostUSD)
	}
	// The other direction, unchanged: nothing folded, so there is no reading.
	if none := card("r-untouched", callsMeter{calls: untouched.TotalCalls}); none.APIEquivCostUSD != nil {
		t.Errorf("an unmeasured run serves a figure again: %v — that is the fabricated zero back", *none.APIEquivCostUSD)
	}
}

// callsMeter answers with a fold's CALL COUNT and nothing else — the shape
// projMeter produces for a run whose every row priced a true $0.
type callsMeter struct{ calls int64 }

func (m callsMeter) RunMeter(context.Context, string) (api.RunMeter, error) {
	return api.RunMeter{Calls: m.calls}, nil
}

func (callsMeter) LaneMeter(context.Context, string, string) (api.LaneMeter, error) {
	return api.LaneMeter{}, nil
}

func mustDecode(t *testing.T, raw []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("decode: %v (%s)", err, raw)
	}
}

// TestWorkforceUndeclaredCeilingIsAnAbsenceNotAZero is D1 at the seam rather
// than at the render. `worker_guardrails` stores both ceilings NOT NULL with a
// >= 0 check and `Approve` copies the version's REQUEST verbatim, so a version
// that asked for no ceiling is granted 0 — and the registry's own request type
// says so, carrying `omitempty` on both fields. Serving that 0 put `USD 0` on
// screen for a ceiling nobody set (S10.1, the meters precedent).
func TestWorkforceUndeclaredCeilingIsAnAbsenceNotAZero(t *testing.T) {
	b := fixtureWorld(t)
	notes := workforceWorkerByID(t, workforceRead(t, b, "op"), fxWorkerNotes)

	// v2 asked for ceilings, so they are served as the figures they are.
	v2 := workforceVersion(t, notes, fxWorkerNotesV2)
	if v2.Granted == nil || v2.Granted.BudgetUSD == nil || v2.Granted.BudgetSteps == nil {
		t.Fatalf("a version that REQUESTED ceilings serves none: %+v", v2.Granted)
	}
	if *v2.Granted.BudgetUSD != 12.5 || *v2.Granted.BudgetSteps != 400 {
		t.Errorf("the granted ceilings are not the requested ones: %v / %v",
			*v2.Granted.BudgetUSD, *v2.Granted.BudgetSteps)
	}

	// v1 asked for none, so there is nothing to serve — and a 0 would be a
	// ceiling nobody set.
	v1 := workforceVersion(t, notes, fxWorkerNotesV1)
	if v1.Granted == nil {
		t.Fatal("v1 has no guardrails row, so this asserts nothing")
	}
	if v1.Granted.BudgetUSD != nil {
		t.Errorf("an undeclared dollar ceiling was served as %v — a figure nobody set (S10.1)", *v1.Granted.BudgetUSD)
	}
	if v1.Granted.BudgetSteps != nil {
		t.Errorf("an undeclared step ceiling was served as %v", *v1.Granted.BudgetSteps)
	}
	// And it is genuinely 0 in the row, so the nil above is a DECISION about a
	// stored zero rather than a column that happened to be empty.
	g, err := fixtureWorkforce(t, b).Guardrails(t.Context(), fxWorkerNotesV1)
	if err != nil {
		t.Fatalf("read the guardrails row: %v", err)
	}
	if g.BudgetUSD != 0 || g.BudgetSteps != 0 {
		t.Fatalf("v1's stored ceilings are not zero (%v/%v), so the absence above is not the case under test",
			g.BudgetUSD, g.BudgetSteps)
	}
}

// TestWorkforceEmptyOutcomeSentencesDifferByReading is D6 at the seam. The
// operator's reading covers every owner, so its empty answer is about the
// VERSION; a member's covers their own runs, so its empty answer is about what
// THEY ran — which is also the most it may honestly say, because naming another
// owner's run would disclose exactly what the scope exists to withhold.
func TestWorkforceEmptyOutcomeSentencesDifferByReading(t *testing.T) {
	b := fixtureWorld(t)

	// The version whose only run belongs to bob: alice can see the VERSION
	// (the worker is household) and none of its runs.
	mine := workforceVersion(t, workforceWorkerByID(t, workforceRead(t, b, "alice"), fxWorkerNotes), fxWorkerNotesV1)
	if len(mine.Outcomes.Runs) != 0 {
		t.Fatalf("alice sees runs on v1, so the empty sentence is not under test: %+v", mine.Outcomes.Runs)
	}
	// A version nobody has ever routed to, read as the operator.
	never := workforceVersion(t, workforceWorkerByID(t, workforceRead(t, b, "op"), fxWorkerAudit), fxWorkerAuditV1)
	if len(never.Outcomes.Runs) != 0 {
		t.Fatal("the draft's version has routed runs, so the empty sentence is not under test")
	}

	if mine.Outcomes.Absent == never.Outcomes.Absent {
		t.Errorf("two different facts serve one sentence: %q", mine.Outcomes.Absent)
	}
	if !strings.Contains(mine.Outcomes.Absent, "of yours") {
		t.Errorf("a member's empty answer does not say whose reading it is: %q", mine.Outcomes.Absent)
	}
	// The non-disclosure limb: the member's wording must not reveal that
	// somebody else's run exists here.
	for _, leak := range []string{"another", "other owner", "cannot see", "hidden", "bob"} {
		if strings.Contains(strings.ToLower(mine.Outcomes.Absent), leak) {
			t.Errorf("the member's absence discloses another owner's run (%q): %q", leak, mine.Outcomes.Absent)
		}
	}
}

// TestWorkforceServesRunsRoutedAndVerdictOutcomes is D11: R14 names three
// per-version readings and two of them were missing. A count of rows is a
// reading somebody can take; money is still never summed, and the check below
// asserts that too.
func TestWorkforceServesRunsRoutedAndVerdictOutcomes(t *testing.T) {
	b := fixtureWorld(t)
	notes := workforceWorkerByID(t, workforceRead(t, b, "op"), fxWorkerNotes)

	v2 := workforceVersion(t, notes, fxWorkerNotesV2)
	if v2.Outcomes.RunsRouted != len(v2.Outcomes.Runs) {
		t.Errorf("runs_routed (%d) disagrees with the rows served (%d)", v2.Outcomes.RunsRouted, len(v2.Outcomes.Runs))
	}
	if v2.Outcomes.RunsRouted != 3 {
		t.Errorf("runs_routed = %d, want the 3 the world routed to this version", v2.Outcomes.RunsRouted)
	}
	if len(v2.Outcomes.VerdictTally) != 1 || v2.Outcomes.VerdictTally[0].Verdict != "SHIP" ||
		v2.Outcomes.VerdictTally[0].Rounds != 1 {
		t.Errorf("the verdict tally is not the recorded rounds: %+v", v2.Outcomes.VerdictTally)
	}

	// A version with routed runs but NO verdicts tallies nothing rather than
	// carrying a zero row for a verdict nobody recorded.
	v1 := workforceVersion(t, notes, fxWorkerNotesV1)
	if v1.Outcomes.RunsRouted != 1 {
		t.Errorf("v1 runs_routed = %d, want 1", v1.Outcomes.RunsRouted)
	}
	if len(v1.Outcomes.VerdictTally) != 0 {
		t.Errorf("v1 tallies verdicts it has none of: %+v", v1.Outcomes.VerdictTally)
	}

	// The member's reading counts HER runs, so the count follows the scope
	// rather than reporting a total she may not read.
	mine := workforceVersion(t, workforceWorkerByID(t, workforceRead(t, b, "alice"), fxWorkerNotes), fxWorkerNotesV2)
	if mine.Outcomes.RunsRouted != 2 {
		t.Errorf("the member's runs_routed = %d, want the 2 runs she owns", mine.Outcomes.RunsRouted)
	}
	if mine.Outcomes.RunsRouted >= v2.Outcomes.RunsRouted {
		t.Error("the member's count is not narrower than the operator's — the count escaped the scope")
	}
}

// TestWorkforceRunsRoutedCountsRunsAndTheTallyIsExactOrSaysItIsBounded is the
// rest of D11, and both halves are the packet's own headline class in a
// different currency: a number whose NAME does not describe what it counts.
//
// `runs_routed` counted routing DECISIONS. A run can be routed to one version
// more than once — an `override` re-route is a cause this world already uses —
// so the field reported more runs than exist, and the render beside it ("N
// routed in this reading") said runs too. The verdict tally had the same defect
// squared: the duplicated row re-counted that run's rounds, so ONE recorded SHIP
// tallied as two. And the tally was SILENTLY bounded — rounds past the per-run
// bound simply never reached it, with nothing on screen to warn a reader that
// the number is a floor.
func TestWorkforceRunsRoutedCountsRunsAndTheTallyIsExactOrSaysItIsBounded(t *testing.T) {
	b := fixtureWorld(t)
	// The same run, routed to the same version a second time. Nothing exotic:
	// it is the fixture world's own `override` cause on a run it already has.
	exec(t, b, `INSERT INTO run_events (run_id, generation, user_id, type, schema_version, payload, ts)
	            VALUES (?,0,?,?,1,?,?)`, "r-notes", "alice", "routing.decided",
		`{"cause":"override","score":0.88,"worker":"`+fxWorkerNotes+`","worker_name":"release-notes-writer",`+
			`"version":"`+fxWorkerNotesV2+`","model":"claude","lane":"anthropic","effort":"deep",`+
			`"plain_reason":"alice re-routed the same run to the same version","window_tokens":200000}`, fxT4)

	v2 := workforceVersion(t, workforceWorkerByID(t, workforceRead(t, b, "op"), fxWorkerNotes), fxWorkerNotesV2)
	if v2.Outcomes.RunsRouted != 3 {
		t.Errorf("runs_routed = %d for 3 distinct runs — a field named after runs counted decisions",
			v2.Outcomes.RunsRouted)
	}
	// The decisions are all still served, which is the point of keeping them
	// separate numbers rather than deduplicating the rows.
	if len(v2.Outcomes.Runs) != 4 {
		t.Errorf("the re-route's own decision row was dropped: %d rows", len(v2.Outcomes.Runs))
	}
	ship := 0
	for _, tally := range v2.Outcomes.VerdictTally {
		if tally.Verdict == "SHIP" {
			ship = tally.Rounds
		}
	}
	if ship != 1 {
		t.Errorf("the tally reports SHIP: %d for ONE recorded SHIP round — the duplicated row re-counted it", ship)
	}
	if v2.Outcomes.VerdictTallyTruncated {
		t.Error("an exact tally claims to be bounded, so the marker means nothing")
	}

	// The other half: past the per-run bound the tally is a floor, and it says
	// so where the number is. 25 rounds, all newer than the SHIP, so the bound
	// keeps 20 REWORKs and the SHIP falls out.
	for i := 1; i <= 25; i++ {
		exec(t, b, `INSERT INTO run_events (run_id, generation, user_id, type, schema_version, payload, ts)
		            VALUES (?,0,?,?,1,?,?)`, "r-notes", "alice", "verdict.recorded",
			fmt.Sprintf(`{"round":%d,"verdict":"REWORK","domain":"software","retention":"keep-forever","golden_set":{}}`, i),
			fxT4)
	}
	bounded := workforceVersion(t, workforceWorkerByID(t, workforceRead(t, b, "op"), fxWorkerNotes), fxWorkerNotesV2)
	rework := 0
	for _, tally := range bounded.Outcomes.VerdictTally {
		if tally.Verdict == "REWORK" {
			rework = tally.Rounds
		}
	}
	if rework != 20 {
		t.Fatalf("REWORK tallied %d rounds, want the 20 the per-run bound serves — the bound is not under test", rework)
	}
	if !bounded.Outcomes.VerdictTallyTruncated {
		t.Error("25 recorded rounds tally as 20 with nothing saying the number is a floor — a silently bounded count")
	}
}

// TestWorkforceScheduleBarIsAServedFactOnEveryGrantedBlock is D5's server half,
// which until now was pinned only by golden drift, plus the forward-tolerance
// half the first fix left client-side.
//
// S08.7's third consequence — a degraded domain closes auto-accepting schedules
// — is a POLICY consequence, so it is decided where the maturity vocabulary is
// defined and served as a fact. Deriving it in the browser from
// `maturity === "degraded"` leaves any future maturity that also bars schedules
// silently unmarked, and a flag passed in by a caller gets forgotten: the
// superseded-version block rendered "granted" with no bar at all.
func TestWorkforceScheduleBarIsAServedFactOnEveryGrantedBlock(t *testing.T) {
	b := fixtureWorld(t)
	view := workforceRead(t, b, "op")

	// The degraded domain's automation REQUESTED attachability, so the grant and
	// the bar are two different facts on one row.
	digest := workforceVersion(t, workforceWorkerByID(t, view, fxWorkerDigest), fxWorkerDigestV1)
	if digest.Granted == nil {
		t.Fatal("the automation has no granted block, so there is no row to bar")
	}
	if !digest.Granted.ScheduleAttachable {
		t.Fatal("the automation was not granted schedule attachability, so the bar has nothing to contradict")
	}
	if !digest.Granted.ScheduleBarred {
		t.Error("a worker in a degraded domain is granted a schedule with no bar served (S08.7)")
	}

	// The control: a FULL domain grants no bar, so the field is a consequence
	// rather than a constant — and EVERY version carries its own, including the
	// superseded one, which is where a caller-passed flag had been dropped.
	notes := workforceWorkerByID(t, view, fxWorkerNotes)
	if notes.Domain.Maturity != "full" {
		t.Fatalf("the control worker's domain is %q, not full", notes.Domain.Maturity)
	}
	seen := 0
	for _, v := range notes.Versions {
		if v.Granted == nil {
			continue
		}
		seen++
		if v.Granted.ScheduleBarred {
			t.Errorf("version %s in a FULL domain carries a schedule bar", v.VersionID)
		}
	}
	if seen < 2 {
		t.Fatalf("only %d granted blocks under test — the superseded version is not covered", seen)
	}
}

// TestWorkforceMeterBudgetIsSpentNewestFirstAndDeterministically is D7. Past
// `workforceMeterCap` distinct runs, WHICH rows carry a figure and which say
// "not read in this reading" has to be the same answer every time — a served
// body that differs between two identical requests is the determinism family —
// and it has to be the NEWEST rows, like the surface's three other bounds.
//
// It needs more routed runs than the cap to be observable at all, so the world
// grows its own here rather than in the committed fixture.
func TestWorkforceMeterBudgetIsSpentNewestFirstAndDeterministically(t *testing.T) {
	b := fixtureWorld(t)
	// 140 runs routed to the SUPERSEDED version, all newer than anything the
	// fixture seeded, and all owned by alice so the operator and she both see
	// them. The meter refuses every one of them (they are not in fixtureMeter's
	// table), which is fine: what is under test is WHICH rows the budget reached,
	// and "not read in this reading" is distinguishable from every other absence.
	for i := 0; i < 140; i++ {
		run := fmt.Sprintf("r-bulk-%03d", i)
		exec(t, b, `INSERT INTO runs (run_id, user_id, task_id, state, lane, generation, created_ts, updated_ts)
		            VALUES (?,?,?,?,?,0,?,?)`, run, "alice", "t-ship", "completed", "anthropic", fxT4, fxT4)
		exec(t, b, `INSERT INTO run_events (run_id, generation, user_id, type, schema_version, payload, ts)
		            VALUES (?,0,?,?,1,?,?)`, run, "alice", "routing.decided",
			`{"cause":"selector-match","worker":"`+fxWorkerNotes+`","version":"`+fxWorkerNotesV1+`",`+
				`"model":"claude","lane":"anthropic","plain_reason":"bulk","window_tokens":200000}`, fxT4)
	}

	read := func() []api.WorkforceRoutedRun {
		notes := workforceWorkerByID(t, workforceRead(t, b, "op"), fxWorkerNotes)
		return workforceVersion(t, notes, fxWorkerNotesV1).Outcomes.Runs
	}
	first, second := read(), read()
	// A LITERAL, not `api.WorkforceRunsPerVersion`: comparing the served length
	// against the constant that produced it changes both sides together, so the
	// check cannot fail and pins nothing. 20 is the bound's declared value and
	// moving it is a deliberate act that should have to move this line too.
	if len(first) != 20 {
		t.Fatalf("the per-version bound served %d rows, want 20 — the budget is not under test", len(first))
	}
	// The last assertion in this test reasons "20 rows is far inside the 100-run
	// meter cap". That is a RELATION between two independent bounds, and if it
	// ever stopped holding the assertion would start failing for a reason that
	// has nothing to do with what it checks. Said out loud so it is checked.
	if api.WorkforceRunsPerVersion >= api.WorkforceMeterCap {
		t.Fatalf("the per-version bound (%d) no longer sits inside the meter cap (%d) — the budget check below is testing the cap, not the order",
			api.WorkforceRunsPerVersion, api.WorkforceMeterCap)
	}
	// Byte-identical across two identical requests. Under map iteration this is
	// what varies.
	a, _ := json.Marshal(first)
	c, _ := json.Marshal(second)
	if string(a) != string(c) {
		t.Errorf("two identical requests served different bodies:\n%s\n%s", a, c)
	}

	// Newest-first: the rows served are the highest event_seq ones, which are
	// the bulk runs rather than the fixture's original r-audit.
	for _, r := range first {
		if !strings.HasPrefix(r.RunID, "r-bulk-") {
			t.Errorf("the per-version bound kept an older row (%s) over a newer one", r.RunID)
		}
	}
	// And every served row was reached by the meter budget, because 20 rows is
	// far inside the 100-run cap — so nothing here says "not read", which is
	// what proves the budget follows the same order rather than a random one.
	for _, r := range first {
		if strings.Contains(r.MeterAbsent, "not read in this reading") {
			t.Errorf("run %s fell outside the meter budget while inside the per-version bound", r.RunID)
		}
	}
}
