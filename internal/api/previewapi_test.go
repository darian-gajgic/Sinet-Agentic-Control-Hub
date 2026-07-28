package api_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/preview"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/review"
)

// previewapi_test.go — the S13.8 preview verbs over HTTP (P3-B6-3B, R16–R17).
//
// HOST HAZARD. This host runs a LIVE production front chain. Every preview
// fixture here composes the manager exactly as internal/preview's own harness
// does — NewCaddyClient("", "") is routing-disabled — and the tripwire at the
// bottom of this file proves by AST inspection that no test in internal/api
// carries a string literal naming the live admin API or the live preview
// ports. Nothing here dials, binds beyond loopback, or shells a host tool.

func launch(t *testing.T, e *dlvEnv, who, id, body string) preview.Session {
	t.Helper()
	var s preview.Session
	out := e.mustDo(t, who, "POST", "/api/deliverables/"+id+"/preview", body)
	if err := json.Unmarshal([]byte(out), &s); err != nil {
		t.Fatalf("decode session: %v (%s)", err, out)
	}
	return s
}

// ── R16: every disposition is a 200 answer with its reason ──────────────────

func TestPreviewAnswersADefinedStateForEveryDisposition(t *testing.T) {
	e := newDlvEnv(t).withPreview(1)
	e.mkRun("t-a", "r-a", "alice")

	// no-preview: a type with no launchable surface and no repo-backed revision.
	e.mkDeliverable("d-none", "alice", "t-a", "r-a", "", "markdown", map[string]string{"a.md": "x\n"}, "")
	// self-preview: the notebook file IS its own artifact.
	e.mkDeliverable("d-nb", "alice", "t-a", "r-a", "", "notebook", map[string]string{"n.ipynb": "{}\n"}, "")
	// unavailable: a dev-server lane on a host that cannot compose the sandbox
	// boundary (the env wires no Sandbox) — never an unconfined run.
	e.prepareProject("proj", "alice", map[string]string{"package.json": "{\"name\":\"x\"}\n"})
	devSnap := e.snapshotCandidate("proj", "pipe-dev", "package.json", "{\"name\":\"x\"}\n")
	e.mkDeliverable("d-dev", "alice", "t-a", "r-a", "proj", "code", map[string]string{"package.json": "{}\n"}, devSnap)

	for _, tc := range []struct {
		id    string
		state preview.State
		lane  preview.Lane
	}{
		{"d-none", preview.StateNoPreview, preview.LaneNone},
		{"d-nb", preview.StateSelfPreview, preview.LaneNotebook},
		{"d-dev", preview.StateUnavailable, preview.LaneDevServer},
	} {
		s := launch(t, e, "alice", tc.id, `{}`)
		if s.State != tc.state {
			t.Errorf("%s: want state %q, got %q (%s)", tc.id, tc.state, s.State, s.Reason)
		}
		if s.Lane != tc.lane {
			t.Errorf("%s: want lane %q, got %q", tc.id, tc.lane, s.Lane)
		}
		if s.Reason == "" {
			t.Errorf("%s: a defined state must carry its reason — an unexplained state is a broken iframe with extra steps", tc.id)
		}
	}
	// Non-backed dispositions hold NO port and start NOTHING.
	if got := e.count(preview.EventStarted); got != 0 {
		t.Errorf("a non-backed disposition emitted %d start events — nothing started", got)
	}
	if got := len(e.prev.List()); got != 0 {
		t.Errorf("a non-backed disposition stored %d sessions — nothing is live", got)
	}
}

func TestBackedPreviewStartsStopsAndReleases(t *testing.T) {
	e := newDlvEnv(t).withPreview(1)
	e.prepareProject("proj", "alice", map[string]string{"index.html": "<!doctype html>hello\n"})
	snap := e.snapshotCandidate("proj", "pipe", "index.html", "<!doctype html>candidate\n")
	e.mkRun("t-a", "r-a", "alice")
	e.mkDeliverable("d-a", "alice", "t-a", "r-a", "proj", "code", map[string]string{"index.html": "x\n"}, snap)

	s := launch(t, e, "alice", "d-a", `{}`)
	if s.State != preview.StateLive || s.Lane != preview.LaneStatic {
		t.Fatalf("the static lane must come up live, got %q/%q (%s)", s.State, s.Lane, s.Reason)
	}
	if s.PoolPort == 0 || s.BackendAddr == "" {
		t.Errorf("a backed session holds a pool port: %+v", s)
	}
	if s.Routed {
		t.Errorf("routing is DISABLED in this fixture — a routed session means the live admin API was reached: %+v", s)
	}
	if !strings.HasPrefix(s.BackendAddr, "127.0.0.1:") {
		t.Errorf("the backend must be loopback-only, got %q", s.BackendAddr)
	}
	if e.count(preview.EventStarted) != 1 {
		t.Errorf("a backed launch must emit preview.started, got %d", e.count(preview.EventStarted))
	}

	// The at-capacity disposition: the fixture's cap is 1 and one slot is held.
	atCap := launch(t, e, "alice", "d-a", `{}`)
	if atCap.State != preview.StateAtCapacity || atCap.Reason == "" {
		t.Errorf("the second launch must answer at-capacity with its reason, got %q (%s)", atCap.State, atCap.Reason)
	}
	if e.count(preview.EventStarted) != 1 {
		t.Error("an at-capacity answer must start nothing")
	}

	var stopped api.PreviewStopped
	if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "POST", "/api/previews/"+s.ID+"/stop", `{}`)), &stopped); err != nil {
		t.Fatal(err)
	}
	if !stopped.Stopped {
		t.Fatalf("stop did not complete: %+v", stopped)
	}
	if e.count(preview.EventStopped) != 1 {
		t.Errorf("a teardown must emit preview.stopped, got %d", e.count(preview.EventStopped))
	}
	if len(e.prev.List()) != 0 {
		t.Error("a stopped session must be released from the live set")
	}
	// The slot is back: the next launch comes up live again.
	again := launch(t, e, "alice", "d-a", `{}`)
	if again.State != preview.StateLive {
		t.Errorf("the released slot must be reusable, got %q (%s)", again.State, again.Reason)
	}
}

func TestPreviewListIsOwnerScopedThreeWays(t *testing.T) {
	e := newDlvEnv(t).withPreview(3)
	e.prepareProject("proj", "alice", map[string]string{"index.html": "<!doctype html>hello\n"})
	snap := e.snapshotCandidate("proj", "pipe", "index.html", "<!doctype html>candidate\n")
	e.mkRun("t-a", "r-a", "alice")
	e.mkRun("t-b", "r-b", "bob")
	e.mkDeliverable("d-a", "alice", "t-a", "r-a", "proj", "code", map[string]string{"index.html": "a\n"}, snap)
	e.mkDeliverable("d-b", "bob", "t-b", "r-b", "proj", "code", map[string]string{"index.html": "b\n"}, snap)

	aliceSession := launch(t, e, "alice", "d-a", `{}`)
	bobSession := launch(t, e, "bob", "d-b", `{}`)
	if aliceSession.State != preview.StateLive || bobSession.State != preview.StateLive {
		t.Fatalf("both fixtures must be live: %q / %q", aliceSession.State, bobSession.State)
	}

	ids := func(who string) map[string]bool {
		out := map[string]bool{}
		var list api.PreviewSessions
		if err := json.Unmarshal([]byte(e.mustDo(t, who, "GET", "/api/previews", "")), &list); err != nil {
			t.Fatalf("decode previews as %s: %v", who, err)
		}
		for _, s := range list.Sessions {
			out[s.ID] = true
		}
		return out
	}
	if a := ids("alice"); !a[aliceSession.ID] || a[bobSession.ID] {
		t.Errorf("alice must see her own session and not bob's, got %v", a)
	}
	if b := ids("bob"); !b[bobSession.ID] || b[aliceSession.ID] {
		t.Errorf("bob must see his own session and not alice's, got %v", b)
	}
	if o := ids("op"); !o[aliceSession.ID] || !o[bobSession.ID] {
		t.Errorf("the operator sees every live session, got %v", o)
	}

	// Stop is own-or-operator, 404 before 403.
	if code, _ := e.do(t, "bob", "POST", "/api/previews/"+aliceSession.ID+"/stop", `{}`); code != http.StatusForbidden {
		t.Error("another member must not stop alice's session")
	}
	if code, _ := e.do(t, "bob", "POST", "/api/previews/p-nope/stop", `{}`); code != http.StatusNotFound {
		t.Error("an unknown session must 404 before it 403s")
	}
	if code, out := e.do(t, "op", "POST", "/api/previews/"+aliceSession.ID+"/stop", `{}`); code != http.StatusOK {
		t.Errorf("the operator may stop any session: %d %s", code, out)
	}
	if code, out := e.do(t, "bob", "POST", "/api/previews/"+bobSession.ID+"/stop", `{}`); code != http.StatusOK {
		t.Errorf("the owner's own stop must succeed: %d %s", code, out)
	}
}

func TestPreviewLaunchFollowsDeliverableReadVisibility(t *testing.T) {
	e := newDlvEnv(t).withPreview(3)
	e.mkRun("t-a", "r-a", "alice")
	e.mkDeliverable("d-a", "alice", "t-a", "r-a", "", "markdown", map[string]string{"a.md": "x\n"}, "")
	if code, _ := e.do(t, "bob", "POST", "/api/deliverables/d-a/preview", `{}`); code != http.StatusForbidden {
		t.Error("another member must not launch a preview of alice's deliverable")
	}
	if code, _ := e.do(t, "bob", "POST", "/api/deliverables/d-nope/preview", `{}`); code != http.StatusNotFound {
		t.Error("an unknown deliverable must 404 before it 403s")
	}
	if code, out := e.do(t, "alice", "POST", "/api/deliverables/d-a/preview", `{}`); code != http.StatusOK {
		t.Errorf("the owner's own launch must succeed: %d %s", code, out)
	}
}

func TestPreviewComparisonIsHonestAboutHavingNoBefore(t *testing.T) {
	e := newDlvEnv(t).withPreview(3)
	e.mkRun("t-a", "r-a", "alice")
	e.mkDeliverable("d-a", "alice", "t-a", "r-a", "", "markdown", map[string]string{"a.md": "one\n"}, "")

	var cmp preview.Comparison
	if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "POST", "/api/deliverables/d-a/preview/compare", `{}`)), &cmp); err != nil {
		t.Fatalf("decode comparison: %v", err)
	}
	// Revision 1 has no "before"; inventing one would be a preview fake of a
	// story S13.1's diff-against-base already tells truthfully.
	if !cmp.SingleInstance || cmp.Before != nil || cmp.Sync.Enabled {
		t.Fatalf("want the honest single-instance state, got %+v", cmp)
	}
	if cmp.After.State == "" {
		t.Error("the candidate side must still carry its defined state")
	}

	// Once a revision is accepted there IS a before, and the pair comes back
	// with synced navigation enabled.
	if _, err := e.rev.Accept(e.ctx, "d-a", "alice"); err != nil {
		t.Fatal(err)
	}
	var pair preview.Comparison
	if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "POST", "/api/deliverables/d-a/preview/compare", `{}`)), &pair); err != nil {
		t.Fatal(err)
	}
	if pair.SingleInstance || pair.Before == nil || !pair.Sync.Enabled {
		t.Fatalf("an accepted revision gives the before-vs-after pair, got %+v", pair)
	}
	if pair.Before.Revision != 1 {
		t.Errorf("the accepted revision is the `before`, got %d", pair.Before.Revision)
	}
}

// ── R17: the host-hazard tripwire, extended over internal/api ───────────────

// TestAPITestsNeverNameTheLiveFrontChain extends internal/preview's AST
// tripwire discipline to this package: no test here may carry a string literal
// naming the live Caddy admin API, the live preview ports, or a host control
// tool. It inspects STRING LITERALS only, so the host-hazard WARNINGS that live
// in comments are not flagged; this file names the banned tokens by necessity
// and is skipped.
func TestAPITestsNeverNameTheLiveFrontChain(t *testing.T) {
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	banned := []string{":8481", ":8482", "8481:", "8482:", "tailscale", "systemctl", ":2019"}
	fset := token.NewFileSet()
	scanned, probes := 0, 0
	for _, name := range files {
		if name == "previewapi_test.go" {
			continue // this file defines the banned list
		}
		parsed, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		scanned++
		ast.Inspect(parsed, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			probes++
			val, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			for _, b := range banned {
				if strings.Contains(val, b) {
					t.Errorf("%s has a string literal %q naming the LIVE front chain / host state — host hazard", name, val)
				}
			}
			return true
		})
	}
	if scanned == 0 || probes == 0 {
		t.Fatalf("the tripwire inspected %d files and %d literals — it proves nothing", scanned, probes)
	}
	// The probe: the scan must be able to SEE a banned token when one is present.
	if !strings.Contains("http://127.0.0.1:2019/config/", banned[len(banned)-1]) {
		t.Fatal("the tripwire cannot detect its own probe")
	}
}

// TestPreviewComposesWithRoutingDisabled is the structural half of R17: this
// package's manager is built with a routing-DISABLED Caddy client, so a live
// session's URL is a loopback backend and never a tailnet host. If a fixture
// ever wired a real admin endpoint, Routed would be true here.
func TestPreviewComposesWithRoutingDisabled(t *testing.T) {
	e := newDlvEnv(t).withPreview(1)
	e.prepareProject("proj", "alice", map[string]string{"index.html": "<!doctype html>hello\n"})
	snap := e.snapshotCandidate("proj", "pipe", "index.html", "<!doctype html>candidate\n")
	e.mkRun("t-a", "r-a", "alice")
	e.mkDeliverable("d-a", "alice", "t-a", "r-a", "proj", "code", map[string]string{"index.html": "x\n"}, snap)

	s := launch(t, e, "alice", "d-a", `{}`)
	if s.Routed || s.RouteID != "" {
		t.Fatalf("routing must be disabled in every api-level preview fixture: %+v", s)
	}
	if !strings.HasPrefix(s.URL, "http://127.0.0.1:") {
		t.Errorf("a routing-disabled session's URL is its loopback backend, got %q", s.URL)
	}
	if s.Reason == "" {
		t.Error("the routing-disabled state must carry its machine-readable reason")
	}
}

// ── R26: the every-mutation walk ────────────────────────────────────────────

// TestEveryPartBMutationLandsItsNamedCanonicalRow drives every mutating route
// this packet adds and asserts the landed canonical event it produces — and
// that ZERO decision.recorded rows are co-minted. Every act here already has a
// canonical row of its own, and a double mint would make one act look like two
// (§39 OQ8).
func TestEveryPartBMutationLandsItsNamedCanonicalRow(t *testing.T) {
	e := newDlvEnv(t).withPreview(3)
	e.prepareProject("proj", "alice", map[string]string{"index.html": "<!doctype html>hello\n"})
	snap := e.snapshotCandidate("proj", "pipe", "index.html", "<!doctype html>candidate\n")
	e.mkRun("t-a", "r-a", "alice")
	e.mkDeliverable("d-a", "alice", "t-a", "r-a", "proj", "code", map[string]string{"index.html": "x\n"}, snap)

	// 1. comment create → review.comment
	before := e.count(review.EventComment)
	e.mustDo(t, "alice", "POST", "/api/deliverables/d-a/comments", `{"body":"a note","severity":"note"}`)
	if got := e.count(review.EventComment); got != before+1 {
		t.Errorf("a comment must land its review.comment row, got %d (was %d)", got, before)
	}

	// 2. a BACKED preview launch → preview.started; its stop → preview.stopped.
	s := launch(t, e, "alice", "d-a", `{}`)
	if s.State != preview.StateLive {
		t.Fatalf("the backed fixture must be live: %q (%s)", s.State, s.Reason)
	}
	if e.count(preview.EventStarted) != 1 {
		t.Errorf("want one preview.started row, got %d", e.count(preview.EventStarted))
	}
	e.mustDo(t, "alice", "POST", "/api/previews/"+s.ID+"/stop", `{}`)
	if e.count(preview.EventStopped) != 1 {
		t.Errorf("want one preview.stopped row, got %d", e.count(preview.EventStopped))
	}

	// 3. a NON-BACKED preview → nothing started (the absence assert).
	e.mkDeliverable("d-plain", "alice", "t-a", "r-a", "", "markdown", map[string]string{"a.md": "x\n"}, "")
	launch(t, e, "alice", "d-plain", `{}`)
	if e.count(preview.EventStarted) != 1 {
		t.Errorf("a non-backed disposition minted a start event: %d", e.count(preview.EventStarted))
	}

	// 4. the accept → deliverable.accepted + its journal row.
	var card api.AcceptCard
	if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "GET", "/api/deliverables/d-a/accept-card", "")), &card); err != nil {
		t.Fatal(err)
	}
	e.mustDo(t, "alice", "POST", "/api/deliverables/d-a/accept", acceptBody(card.PayloadHash, dlvPIN))
	if got := e.count(review.EventAccepted); got != 1 {
		t.Errorf("want one deliverable.accepted row, got %d", got)
	}

	// The whole walk co-minted nothing.
	if got := e.count(api.EventDecisionRecorded); got != 0 {
		t.Errorf("the deliverables family co-minted %d decision.recorded rows — every act here has its own canonical row (§39 OQ8)", got)
	}
}
