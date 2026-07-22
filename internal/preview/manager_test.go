package preview

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/sandbox"
)

// manager builds a Manager over the fixture, returning it and the recording
// event sink. Options tune Caddy/Sandbox/Settings/Lookup.
func (f *mgrFix) manager(opts ...func(*Config)) (*Manager, *recordingEvents) {
	f.t.Helper()
	rec := &recordingEvents{log: f.log}
	cfg := Config{
		Reviews: f.reviews, Projects: f.proj, Ports: f.ports,
		Events: rec, Settings: f.reg, BaseHost: "sinet.example.ts.net",
		Scratch: f.t.TempDir(), // clones under a test-owned root, never system temp (F16)
	}
	for _, o := range opts {
		o(&cfg)
	}
	m, err := New(cfg)
	if err != nil {
		f.t.Fatalf("preview.New: %v", err)
	}
	// Tear down any live preview at test end so a leaked static server never
	// holds an OS port into the next test (the range is process-wide).
	f.t.Cleanup(func() {
		for _, s := range m.List() {
			_ = m.Stop(context.Background(), s.ID)
		}
	})
	return m, rec
}

func withCaddy(endpoint string) func(*Config) {
	return func(c *Config) { c.Caddy = NewCaddyClient(endpoint, "srv0") }
}
func withSandbox(s Sandbox) func(*Config)   { return func(c *Config) { c.Sandbox = s } }
func withSettings(s Settings) func(*Config) { return func(c *Config) { c.Settings = s } }
func withLookup(l ToolLookup) func(*Config) { return func(c *Config) { c.Lookup = l } }
func withPorts(p Ports) func(*Config)       { return func(c *Config) { c.Ports = p } }

// flakyPorts fails the next Release once, then delegates — to exercise Stop's
// retryable partial-teardown path (F21).
type flakyPorts struct {
	inner    Ports
	failNext bool
}

func (f *flakyPorts) Allocate(owner string) (int, error) { return f.inner.Allocate(owner) }
func (f *flakyPorts) Release(port int) error {
	if f.failNext {
		f.failNext = false
		return errorString("release boom")
	}
	return f.inner.Release(port)
}

type errorString string

func (e errorString) Error() string { return string(e) }

func stubLookup(tools map[string]string) ToolLookup {
	return func(name string) (string, bool) { p, ok := tools[name]; return p, ok }
}

// TestStaticLaneLifecycleE2E: launch → GET over loopback → teardown, fully
// hermetic (Spec S13.8; R11/R12/R16). The static server serves the read-only
// checkout on 127.0.0.1:<pool port>; the route add/remove hits the fake admin;
// the port returns to the pool; the checkout is released; events are recorded.
func TestStaticLaneLifecycleE2E(t *testing.T) {
	fx := newMgrFix(t, map[string]string{"index.html": "<h1>preview-me</h1>"}, "code")
	fa, srv := newFakeAdmin()
	defer srv.Close()
	m, rec := fx.manager(withCaddy(srv.URL))
	ctx := context.Background()

	s, err := m.Launch(ctx, LaunchRequest{DeliverableID: "dlv1", User: "u"})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if s.Lane != LaneStatic || s.State != StateLive {
		t.Fatalf("lane/state = %s/%s, want static/live", s.Lane, s.State)
	}
	lo, hi := fx.ports.Range()
	if s.PoolPort < lo || s.PoolPort > hi {
		t.Fatalf("pool port %d out of range [%d,%d]", s.PoolPort, lo, hi)
	}
	if !s.Routed || s.RouteID == "" {
		t.Fatalf("expected a Caddy route, got routed=%v id=%q", s.Routed, s.RouteID)
	}

	// The backend actually serves the read-only checkout over loopback.
	resp, err := http.Get("http://" + s.BackendAddr + "/")
	if err != nil {
		t.Fatalf("GET backend: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(body) != "<h1>preview-me</h1>" {
		t.Fatalf("backend served %d %q", resp.StatusCode, body)
	}

	if got := len(m.List()); got != 1 {
		t.Fatalf("List() = %d live sessions, want 1", got)
	}
	if len(rec.byType(EventStarted)) != 1 {
		t.Fatalf("expected one preview.started event")
	}
	rid := s.RouteID // Stop clears it on success (F21) — capture before teardown

	// Teardown: port released, route removed, checkout released, event recorded.
	if err := m.Stop(ctx, s.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := m.Stop(ctx, s.ID); err != nil {
		t.Fatalf("Stop (idempotent): %v", err)
	}
	if got := len(m.List()); got != 0 {
		t.Fatalf("List() = %d after stop, want 0", got)
	}
	res, _ := fx.ports.List()
	if len(res) != 0 {
		t.Fatalf("port not returned to the pool: %+v", res)
	}
	if len(rec.byType(EventStopped)) != 1 {
		t.Fatalf("expected one preview.stopped event")
	}
	// The route was added on launch and removed on teardown.
	var adds, dels int
	for _, mth := range fa.methods() {
		switch {
		case mth == "POST /config/apps/http/servers/srv0/routes":
			adds++
		case mth == "DELETE /id/"+rid:
			dels++
		}
	}
	if adds < 1 || dels < 1 {
		t.Fatalf("expected route add + remove admin calls, got adds=%d dels=%d (%v)", adds, dels, fa.methods())
	}
	// The backend is really down after teardown.
	if _, err := http.Get("http://" + s.BackendAddr + "/"); err == nil {
		t.Fatalf("backend still serving after teardown")
	}
}

// TestLowTierEventAttribution: preview lifecycle events are platform-scope
// (run_id NULL, no generation — previews are not runs) and owner-attributed to
// the launching user (Spec S13.8; R22). Low-tier: no effect journal / approval /
// step-up exists in the flow (the import wall proves the machinery is unreachable).
func TestLowTierEventAttribution(t *testing.T) {
	fx := newMgrFix(t, map[string]string{"index.html": "x"}, "code")
	m, rec := fx.manager()
	s, err := m.Launch(context.Background(), LaunchRequest{DeliverableID: "dlv1", User: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if s.State != StateLive {
		t.Fatalf("state = %s", s.State)
	}
	// F22/R12: routing disabled (no Caddy admin endpoint) is recorded as a
	// machine-readable reason on the live session.
	if s.Routed || !strings.Contains(s.Reason, "routing disabled") {
		t.Errorf("routing-disabled live session lacks the reason: routed=%v reason=%q", s.Routed, s.Reason)
	}
	got := rec.byType(EventStarted)
	if len(got) != 1 {
		t.Fatalf("want 1 started event, got %d", len(got))
	}
	a := got[0]
	if a.RunID != "" || a.Generation != 0 {
		t.Errorf("event is not platform-scope: run_id=%q generation=%d", a.RunID, a.Generation)
	}
	if a.UserID != "alice" {
		t.Errorf("event owner = %q, want alice (15.6)", a.UserID)
	}
}

// TestMaxConcurrentCap: a launch beyond ⚙ preview.max_concurrent is refused with
// the honest at-capacity state — no port allocated (Spec S13.8; R15).
func TestMaxConcurrentCap(t *testing.T) {
	fx := newMgrFix(t, map[string]string{"index.html": "x"}, "code")
	m, _ := fx.manager(withSettings(fakeSettings{idle: 900 * time.Second, cap: 1}))
	ctx := context.Background()
	s1, err := m.Launch(ctx, LaunchRequest{DeliverableID: "dlv1", User: "u"})
	if err != nil {
		t.Fatalf("first launch: %v", err)
	}
	if s1.State != StateLive {
		t.Fatalf("first launch state = %v", s1.State)
	}
	s2, err := m.Launch(ctx, LaunchRequest{DeliverableID: "dlv1", User: "u"})
	if err != nil {
		t.Fatal(err)
	}
	if s2.State != StateAtCapacity {
		t.Fatalf("second launch state = %s, want at-capacity", s2.State)
	}
	if s2.PoolPort != 0 {
		t.Fatalf("at-capacity launch allocated a port %d", s2.PoolPort)
	}
}

// TestPerTypeStatesThroughManager: every non-runnable type gets a defined,
// checkout-released state — no refusal, no broken iframe (Spec S13.8; R4).
func TestPerTypeStatesThroughManager(t *testing.T) {
	cases := []struct {
		files map[string]string
		dtype string
		state State
	}{
		{map[string]string{"analysis.ipynb": "{}"}, "notebook", StateSelfPreview},
		{map[string]string{"docker-compose.yml": "services: {}"}, "code", StateRequiresContainer},
		{map[string]string{"README.md": "hi"}, "text", StateNoPreview},
	}
	for _, c := range cases {
		fx := newMgrFix(t, c.files, c.dtype)
		m, _ := fx.manager()
		s, err := m.Launch(context.Background(), LaunchRequest{DeliverableID: "dlv1", User: "u"})
		if err != nil {
			t.Fatalf("%s: %v", c.dtype, err)
		}
		if s.State != c.state {
			t.Errorf("%v → state %s, want %s", c.files, s.State, c.state)
		}
		if s.Reason == "" {
			t.Errorf("%v → state carries no machine-readable reason", c.files)
		}
		if s.PoolPort != 0 {
			t.Errorf("%v → non-backed state holds a port", c.files)
		}
	}
}

// TestDevServerUnavailableWithoutCaps: a dev-server lane on a host that cannot
// compose the boundary is the honest unavailable state — NEVER an unconfined run
// (Spec S13.8; R7).
func TestDevServerUnavailableWithoutCaps(t *testing.T) {
	fx := newMgrFix(t, map[string]string{"package.json": "{}"}, "code")
	// Sandbox nil ⇒ unavailable.
	m, _ := fx.manager()
	s, err := m.Launch(context.Background(), LaunchRequest{DeliverableID: "dlv1", User: "u"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Lane != LaneDevServer || s.State != StateUnavailable {
		t.Fatalf("nil sandbox: lane/state = %s/%s, want dev-server/unavailable", s.Lane, s.State)
	}
	// A host that probes uncomposable is likewise unavailable.
	m2, _ := fx.manager(withSandbox(unavailableSandbox{}), withLookup(stubLookup(map[string]string{"npm": "/x/npm"})))
	s2, _ := m2.Launch(context.Background(), LaunchRequest{DeliverableID: "dlv1", User: "u"})
	if s2.State != StateUnavailable {
		t.Fatalf("uncomposable host: state = %s, want unavailable", s2.State)
	}
}

// TestDevServerRunnerToolAbsent: an absent runner tool is the honest no-preview
// state — NEVER an install (Spec S13.8; R2).
func TestDevServerRunnerToolAbsent(t *testing.T) {
	fx := newMgrFix(t, map[string]string{"package.json": "{}", "pnpm-lock.yaml": ""}, "code")
	// Composable host, but the pnpm tool is absent (empty lookup).
	m, _ := fx.manager(withSandbox(sandbox.NewComposer(fx.reg, nil)), withLookup(stubLookup(nil)))
	s, err := m.Launch(context.Background(), LaunchRequest{DeliverableID: "dlv1", User: "u"})
	if err != nil {
		t.Fatal(err)
	}
	if s.State != StateNoPreview {
		t.Fatalf("absent pnpm: state = %s, want no-preview", s.State)
	}
	if s.Reason == "" || !strings.Contains(s.Reason, "pnpm") {
		t.Fatalf("reason %q should name the absent tool", s.Reason)
	}
}

// TestDevServerComposesThroughSandbox: a dev-server lane on a composable host
// composes through the EXISTING sandbox surface (proving R7's read-only
// consumer) and reports the honest deferred state; the throwaway clone is
// disposed and the checkout released (Spec S13.8; R6/R7).
func TestDevServerComposesThroughSandbox(t *testing.T) {
	caps := sandbox.Probe()
	if !caps.Available() {
		t.Skip("SANCTIONED SKIP (S16.3): host cannot compose the sandbox boundary")
	}
	fx := newMgrFix(t, map[string]string{"package.json": "{}"}, "code")
	stub := filepath.Join(t.TempDir(), "npm")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	m, _ := fx.manager(withSandbox(sandbox.NewComposer(fx.reg, nil)), withLookup(stubLookup(map[string]string{"npm": stub})))
	s, err := m.Launch(context.Background(), LaunchRequest{DeliverableID: "dlv1", User: "u"})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if s.State != StateUnavailable || !strings.Contains(s.Reason, "composes") {
		t.Fatalf("state=%s reason=%q, want a composes/deferred unavailable state", s.State, s.Reason)
	}
	if s.PoolPort != 0 {
		t.Fatalf("deferred dev-server held a port %d", s.PoolPort)
	}
	// allowedHosts injection was recorded on the session (R13).
	if !hasEnv(s.HostInjections, "SINET_PREVIEW_HOST", "preview-"+s.ID+".sinet.example.ts.net") {
		t.Fatalf("host injection missing: %+v", s.HostInjections)
	}
}

// TestRegistryCommandThroughManager: an active project's captured preview
// command drives the runner and takes precedence over heuristics (Spec S13.7/
// S13.8; R3) — proven end-to-end through the manager, not just Detect.
func TestRegistryCommandThroughManager(t *testing.T) {
	fx := newMgrFix(t, map[string]string{"index.html": "x"}, "code", "myserver --dev --port 8000")
	m, _ := fx.manager()
	s, err := m.Launch(context.Background(), LaunchRequest{DeliverableID: "dlv1", User: "u"})
	if err != nil {
		t.Fatal(err)
	}
	// index.html would heuristically be static; the registry command overrides.
	if s.Lane != LaneDevServer {
		t.Fatalf("lane = %s, want dev-server (registry command wins over the static heuristic)", s.Lane)
	}
	if s.Runner == nil || s.Runner.Source != "registry" {
		t.Fatalf("runner = %v, want registry-sourced", s.Runner)
	}
	if len(s.Runner.Argv) == 0 || s.Runner.Argv[0] != "myserver" {
		t.Fatalf("runner argv = %v, want the captured command", s.Runner.Argv)
	}
}

// TestZeroMutation wraps the REAL Manager lifecycle (F3/R6): a dev-server launch
// stages a checkout, clones it, composes (writing into the clone), and tears
// down — and the project's committed tree AND a separate "reviewed run worktree"
// stay byte-identical (symlinks included) across the whole path. Caps-gated: the
// dev-server lane requires composition (this host has caps).
func TestZeroMutation(t *testing.T) {
	caps := sandbox.Probe()
	if !caps.Available() {
		t.Skip("SANCTIONED SKIP (S16.3): host cannot compose the sandbox boundary")
	}
	fx := newMgrFix(t, map[string]string{"src/app.js": "console.log(1)", "package.json": "{}"}, "code")
	ctx := context.Background()

	// A SEPARATE reviewed run worktree the preview is NEVER handed + the project
	// store base — both must be untouched after the lifecycle.
	reviewed, err := fx.proj.AddUtilityCheckout(ctx, fx.projectID, fx.committish)
	if err != nil {
		t.Fatal(err)
	}
	e, _ := fx.proj.Get(ctx, fx.projectID)
	beforeReviewed := hashTree(t, reviewed.Path)
	beforeStore := hashTree(t, e.StorePath)

	// A stub runner whose composeCheck writes into the clone via a hook, so the
	// "preview writes" happen inside the real Manager launch→compose→teardown.
	stub := filepath.Join(t.TempDir(), "npm")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	m, _ := fx.manager(withSandbox(sandbox.NewComposer(fx.reg, nil)), withLookup(stubLookup(map[string]string{"npm": stub})))
	// Simulate the dev server's writes landing in the clone, and capture the
	// revision utility checkout so the direct leg (R2-2) can assert it separately.
	var capturedCheckout string
	m.cloneWriteHook = func(s *Session) {
		capturedCheckout = s.checkout
		_ = os.WriteFile(filepath.Join(s.clone, "node_modules_marker"), []byte("installed"), 0o600)
		_ = os.WriteFile(filepath.Join(s.clone, "src", "app.js"), []byte("MUTATED"), 0o600)
	}
	s, err := m.Launch(ctx, LaunchRequest{DeliverableID: "dlv1", User: "u"})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if s.Lane != LaneDevServer {
		t.Fatalf("lane = %s, want dev-server", s.Lane)
	}

	if hashTree(t, reviewed.Path) != beforeReviewed {
		t.Errorf("the reviewed run worktree was mutated by the preview (R6)")
	}
	if hashTree(t, e.StorePath) != beforeStore {
		t.Errorf("the project store tree was mutated by the preview (R6)")
	}
	// DIRECT leg (R2-2): the revision utility checkout (under <root>/utility/,
	// outside both trees hashed above) was released CLEAN by teardown — a
	// manager-side write into s.checkout would dirty it, ReleaseUtilityCheckout
	// would ErrDirty, and the worktree dir would REMAIN. Assert it is gone.
	if capturedCheckout == "" {
		t.Fatal("clone hook did not fire — revision checkout not captured")
	}
	if _, err := os.Stat(capturedCheckout); !os.IsNotExist(err) {
		t.Errorf("revision checkout %q not released clean after teardown — a manager-side write dirtied it (R2-2/R6)", capturedCheckout)
	}
	// The reviewed worktree releases cleanly (a dirtied one would ErrDirty).
	if err := fx.proj.ReleaseUtilityCheckout(ctx, fx.projectID, reviewed.Path); err != nil {
		t.Errorf("clean reviewed worktree should release, got %v", err)
	}
}

// TestComparisonBeforeVsAfter: the synced-navigation DATA contract carries both
// sides when an accepted revision exists, and the honest single-instance state
// when it does not (Spec S13.8; R17/R18).
func TestComparisonBeforeVsAfter(t *testing.T) {
	fx := newMgrFix(t, map[string]string{"index.html": "v"}, "code")
	fa, srv := newFakeAdmin()
	defer srv.Close()
	m, _ := fx.manager(withCaddy(srv.URL))
	ctx := context.Background()

	// No accepted revision → single-instance (candidate only).
	cmp, err := m.LaunchComparison(ctx, "dlv1", 0, "u")
	if err != nil {
		t.Fatal(err)
	}
	if !cmp.SingleInstance || cmp.Before != nil || cmp.Sync.Enabled {
		t.Fatalf("no-accepted → want single-instance, got %+v", cmp)
	}
	if cmp.After.Role != RoleAfter {
		t.Fatalf("after side role = %s", cmp.After.Role)
	}
	_ = fa

	// Now the deliverable has an accepted revision → both sides.
	fx.reviews.accepted["dlv1"] = fx.reviews.revisions["dlv1/1"]
	cmp2, err := m.LaunchComparison(ctx, "dlv1", 0, "u")
	if err != nil {
		t.Fatal(err)
	}
	if cmp2.SingleInstance || cmp2.Before == nil || !cmp2.Sync.Enabled {
		t.Fatalf("accepted → want dual-instance, got %+v", cmp2)
	}
	if cmp2.Before.Role != RoleBefore || cmp2.After.Role != RoleAfter {
		t.Fatalf("roles = %s/%s", cmp2.Before.Role, cmp2.After.Role)
	}
	// The contract is JSON-serializable DATA (no HTML) for the S15 widget (B6).
	blob, err := json.Marshal(cmp2)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"before"`, `"after"`, `"sync"`, `"role":"before"`} {
		if !strings.Contains(string(blob), want) {
			t.Errorf("contract JSON lacks %s: %s", want, blob)
		}
	}
}

// TestLaunchCLILane covers the ttyd CLI lane through the Manager (F10): absent
// ttyd → honest no-preview; present ttyd on a composable host → the deferred
// state; the read-only checkout is composed via a CLONE, never writable (F2).
func TestLaunchCLILane(t *testing.T) {
	ctx := context.Background()

	// Absent ttyd → honest CLI no-preview (never installed, R20/R21).
	fx := newMgrFix(t, map[string]string{"main.go": "package main"}, "cli", "myctl serve")
	m, _ := fx.manager(withSandbox(sandbox.NewComposer(fx.reg, nil)), withLookup(stubLookup(nil)))
	s, err := m.Launch(ctx, LaunchRequest{DeliverableID: "dlv1", User: "u"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Lane != LaneCLI || s.State != StateNoPreview || !strings.Contains(s.Reason, "ttyd") {
		t.Fatalf("absent ttyd: lane/state/reason = %s/%s/%q, want cli/no-preview/ttyd", s.Lane, s.State, s.Reason)
	}

	// Present ttyd on a composable host → deferred state; checkout composed via a
	// clone (F2), nothing held live.
	caps := sandbox.Probe()
	if !caps.Available() {
		t.Skip("SANCTIONED SKIP (S16.3): host cannot compose the sandbox boundary")
	}
	fx2 := newMgrFix(t, map[string]string{"main.go": "package main"}, "cli", "myctl serve")
	stub := filepath.Join(t.TempDir(), "ttyd")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	m2, _ := fx2.manager(withSandbox(sandbox.NewComposer(fx2.reg, nil)), withLookup(stubLookup(map[string]string{"ttyd": stub})))
	s2, err := m2.Launch(ctx, LaunchRequest{DeliverableID: "dlv1", User: "u"})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if s2.Lane != LaneCLI || s2.State != StateUnavailable || !strings.Contains(s2.Reason, "ttyd CLI") {
		t.Fatalf("present ttyd: lane/state/reason = %s/%s/%q", s2.Lane, s2.State, s2.Reason)
	}
	if s2.PoolPort != 0 {
		t.Errorf("deferred CLI held a port %d", s2.PoolPort)
	}
}

// TestCheckoutFSRefusesSymlinkEscape pins F13: the static server's filesystem
// refuses a committed symlink that resolves outside the checkout, so unsandboxed
// platform code never serves host files to the tailnet; in-tree files still open.
func TestCheckoutFSRefusesSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "secret")
	if err := os.WriteFile(outside, []byte("SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "evil")); err != nil {
		t.Skipf("symlinks unsupported here: %v", err)
	}
	fsys := checkoutFS{root: root}
	f, err := fsys.Open("/index.html")
	if err != nil {
		t.Errorf("in-tree file should open, got %v", err)
	} else {
		f.Close()
	}
	if _, err := fsys.Open("/evil"); err == nil {
		t.Errorf("a symlink escaping the checkout must be refused (F13)")
	}
}

// TestRenderUnitsReadsIdleSetting pins F1: the production unit-rendering path
// reads ⚙ preview.idle_stop by dotted key through the Settings interface — the
// value is not a constant.
func TestRenderUnitsReadsIdleSetting(t *testing.T) {
	fx := newMgrFix(t, map[string]string{"index.html": "x"}, "code")
	m, _ := fx.manager(withSettings(fakeSettings{idle: 120 * time.Second, cap: 3}))
	_, service, err := m.RenderUnits(47600, "127.0.0.1:5173")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(service.Content, "--exit-idle-time=120s") {
		t.Fatalf("⚙ idle value did not flow into the rendered unit:\n%s", service.Content)
	}
	// The real registry default (900 s) flows through too.
	m2, _ := fx.manager()
	_, svc2, err := m2.RenderUnits(47600, "127.0.0.1:5173")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(svc2.Content, "--exit-idle-time=900s") {
		t.Fatalf("registry default idle did not render:\n%s", svc2.Content)
	}
}

// TestBwrapOverlayFlags behavior-asserts the bwrap overlay flags on THIS host's
// bwrap (F15/§10): --overlay-src + --tmp-overlay expose the lowerdir, proving the
// production overlay-upper substrate is real (never --help parsing). Caps-gated.
func TestBwrapOverlayFlags(t *testing.T) {
	caps := sandbox.Probe()
	if !caps.Available() {
		t.Skip("SANCTIONED SKIP (S16.3): host cannot compose the sandbox boundary")
	}
	lower := t.TempDir()
	if err := os.WriteFile(filepath.Join(lower, "f.txt"), []byte("lower"), 0o600); err != nil {
		t.Fatal(err)
	}
	const dest = "/overlay-probe"
	cmd := exec.Command(sandbox.SystemBwrap, "--unshare-user", "--unshare-net",
		"--ro-bind", "/usr", "/usr", "--ro-bind-try", "/bin", "/bin",
		"--ro-bind-try", "/lib", "/lib", "--ro-bind-try", "/lib64", "/lib64",
		"--proc", "/proc", "--dev", "/dev",
		"--overlay-src", lower, "--tmp-overlay", dest,
		"--", "/bin/cat", dest+"/f.txt")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bwrap %s rejected --overlay-src/--tmp-overlay (the overlay substrate claim is unproven): %v\n%s", caps.BwrapVersion, err, out)
	}
	if strings.TrimSpace(string(out)) != "lower" {
		t.Fatalf("overlay did not expose the lowerdir file: %q", out)
	}
}

// TestStopRetryableOnPartialFailure pins F21: a Release failure during Stop
// returns the joined error and KEEPS the session (retryable); a retry completes
// and forgets it, and only then is the stop event emitted.
func TestStopRetryableOnPartialFailure(t *testing.T) {
	fx := newMgrFix(t, map[string]string{"index.html": "x"}, "code")
	fp := &flakyPorts{inner: fx.ports}
	m, rec := fx.manager(withPorts(fp))
	ctx := context.Background()
	s, err := m.Launch(ctx, LaunchRequest{DeliverableID: "dlv1", User: "u"})
	if err != nil || s.State != StateLive {
		t.Fatalf("launch state=%v err=%v", s.State, err)
	}
	// First Stop: Release fails → error returned, session retained.
	fp.failNext = true
	if err := m.Stop(ctx, s.ID); err == nil {
		t.Fatalf("expected a partial-teardown error")
	}
	if len(m.List()) != 1 {
		t.Fatalf("session dropped on partial failure — not retryable (F21)")
	}
	if len(rec.byType(EventStopped)) != 0 {
		t.Fatalf("stopped event emitted before full teardown")
	}
	// Retry: Release now succeeds → session forgotten, event emitted.
	if err := m.Stop(ctx, s.ID); err != nil {
		t.Fatalf("retry Stop: %v", err)
	}
	if len(m.List()) != 0 {
		t.Fatalf("session not forgotten after successful retry")
	}
	if len(rec.byType(EventStopped)) != 1 {
		t.Fatalf("expected one stopped event after full teardown, got %d", len(rec.byType(EventStopped)))
	}
	res, _ := fx.ports.List()
	if len(res) != 0 {
		t.Fatalf("port not released after retry: %+v", res)
	}
}

// TestStaticLaneAdvancesPastBoundPort pins R2-1(b): when a pool port is bound
// EXTERNALLY (a squatter, or a parallel test package sharing the range), the OS
// bind view diverges from the file-stub view; launchStatic advances to the next
// port rather than hard-erroring on EADDRINUSE. Deterministic: pre-bind the
// lowest range port, then assert the session lands on a later one and serves.
func TestStaticLaneAdvancesPastBoundPort(t *testing.T) {
	fx := newMgrFix(t, map[string]string{"index.html": "advance"}, "code")
	lo, hi := fx.ports.Range()
	squat, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(lo)))
	if err != nil {
		t.Skipf("could not pre-bind %d: %v", lo, err)
	}
	defer squat.Close()

	m, _ := fx.manager()
	s, err := m.Launch(context.Background(), LaunchRequest{DeliverableID: "dlv1", User: "u"})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if s.State != StateLive {
		t.Fatalf("state = %s, want live", s.State)
	}
	if s.PoolPort == lo {
		t.Fatalf("landed on the externally-bound port %d — did not advance (R2-1)", lo)
	}
	if s.PoolPort <= lo || s.PoolPort > hi {
		t.Fatalf("advanced port %d out of range [%d,%d]", s.PoolPort, lo, hi)
	}
	resp, err := http.Get("http://" + s.BackendAddr + "/")
	if err != nil {
		t.Fatalf("GET advanced backend: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("advanced backend status %d", resp.StatusCode)
	}
}

// TestComparisonPropagatesRealError pins F7's propagate branch: a NON-ErrBadInput
// error from AcceptedRevision (e.g. a storage failure) propagates out of
// LaunchComparison rather than being swallowed as the honest single-instance
// state.
func TestComparisonPropagatesRealError(t *testing.T) {
	fx := newMgrFix(t, map[string]string{"index.html": "x"}, "code")
	fx.reviews.acceptedErr = errorString("storage boom") // not review.ErrBadInput
	m, _ := fx.manager()
	_, err := m.LaunchComparison(context.Background(), "dlv1", 0, "u")
	if err == nil {
		t.Fatalf("expected LaunchComparison to propagate a real store error (F7)")
	}
	if errors.Is(err, review.ErrBadInput) {
		t.Fatalf("a real error must not be reported as ErrBadInput/single-instance (F7)")
	}
}
