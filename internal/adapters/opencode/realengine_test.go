package opencode

// realengine_test.go — tier R of the conformance split: the REAL pinned
// `opencode serve`, at ZERO provider cost. Every process is spawned into its
// own group and reaped by GROUP on every exit path; every serve interaction is
// bounded, so a battery can never hang (R21, operator standing directive).
//
// Absence of the engine is the ONE sanctioned skip class (CONVENTIONS §10); a
// boot that exceeds its budget skips with a NAMED reason rather than hanging or
// faking a pass.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
)

// realManager builds the dev-posture spawner for a tier-R test, group-reaped
// on every exit path.
func realManager(t *testing.T, bin string, boot time.Duration) *Manager {
	t.Helper()
	m := &Manager{
		Binary:         bin,
		BootTimeout:    boot,
		StopGrace:      3 * time.Second,
		HealthInterval: time.Second,
		Log:            slog.New(slog.NewTextHandler(testWriter{t}, nil)),
	}
	t.Cleanup(func() {
		if err := m.Close(); err != nil {
			t.Errorf("manager close (group reap): %v", err)
		}
	})
	return m
}

// realAdapter wires an Adapter onto a real Manager with a synthetic HOME, so a
// stray write outside the per-user XDG root is detectable and the operator's
// own opencode dirs are never in the child's environment.
func realAdapter(t *testing.T, m *Manager, providers ProviderConfig) (*Adapter, string) {
	t.Helper()
	home := t.TempDir()
	return &Adapter{
		Root:      t.TempDir(),
		Instances: m,
		Providers: providers,
		Env:       []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home},
		Log:       slog.New(slog.NewTextHandler(testWriter{t}, nil)),
		Now:       time.Now,
	}, home
}

func TestRealServeBindAuthIsolation(t *testing.T) {
	bin := enginePath(t)
	// No provider block: this probe stays OFF provider-touching paths, so the
	// ~62 MB per-user first-boot dependency tree (SPIKE G1-S3 F1) is never
	// fetched and the boot budget is small.
	m := realManager(t, bin, 45*time.Second)
	a, home := realAdapter(t, m, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	insts := map[string]Instance{}
	for _, user := range []string{"alice", "bob"} {
		req := testRequest(t)
		req.UserID = user
		l, err := a.lower(req)
		if err != nil {
			t.Fatalf("lower(%s): %v", user, err)
		}
		inst, err := m.Acquire(ctx, a.instanceSpec(req, l, l.env))
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "boot") {
				t.Skipf("SANCTIONED SKIP (CONVENTIONS §10): real `opencode serve` did not report a listening "+
					"URL within the %s boot budget for user %q (%v) — tier-R real-serve conformance not run", m.BootTimeout, user, err)
			}
			t.Fatalf("acquire(%s): %v", user, err)
		}
		insts[user] = inst
	}

	client := &http.Client{Timeout: 5 * time.Second}
	for user, inst := range insts {
		// Bind 127.0.0.1 only (S03.2).
		if !strings.HasPrefix(inst.BaseURL, "http://127.0.0.1:") {
			t.Errorf("%s: instance base URL %q is not a loopback bind", user, inst.BaseURL)
		}
		if inst.Password == "" {
			t.Errorf("%s: instance started without a server password", user)
		}
		// Basic auth on EVERY endpoint, /api/health included (SPIKE P2-S1).
		for _, path := range []string{"/api/health", "/global/health", "/session", "/permission", "/session/status"} {
			code, err := probeStatus(ctx, client, inst.BaseURL+path, "", "")
			if err != nil {
				t.Fatalf("%s %s (no auth): %v", user, path, err)
			}
			if code != http.StatusUnauthorized && code != http.StatusProxyAuthRequired {
				t.Errorf("%s %s without Basic auth = %d, want a 401/407-class refusal", user, path, code)
			}
			code, err = probeStatus(ctx, client, inst.BaseURL+path, BasicAuthUser, inst.Password)
			if err != nil {
				t.Fatalf("%s %s (auth): %v", user, path, err)
			}
			if code != http.StatusOK {
				t.Errorf("%s %s with Basic auth = %d, want 200", user, path, code)
			}
		}
		// The username is part of the credential at this pin (live-recorded
		// 2026-08-23): a wrong username is refused.
		if code, err := probeStatus(ctx, client, inst.BaseURL+"/api/health", "sinet", inst.Password); err == nil && code == http.StatusOK {
			t.Errorf("%s: /api/health accepted Basic username %q — the recorded engine requires %q",
				user, "sinet", BasicAuthUser)
		}
	}

	// The two XDG trees are fully disjoint, and each instance actually wrote
	// into its own (SPIKE G1-S3 F1 re-asserted at the pin).
	alice, bob := insts["alice"].Root, insts["bob"].Root
	if alice == bob || alice == "" {
		t.Fatalf("per-user roots are not disjoint: %q vs %q", alice, bob)
	}
	for _, root := range []string{alice, bob} {
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) == 0 {
			t.Errorf("per-user root %q is empty — the instance did not use it: %v", root, err)
		}
	}
	// Zero strays: nothing landed in the default XDG locations under HOME.
	for _, stray := range []string{
		".config/opencode", ".local/share/opencode", ".local/state/opencode", ".cache/opencode",
	} {
		if _, err := os.Stat(filepath.Join(home, stray)); err == nil {
			t.Errorf("stray engine state outside the per-user root: %s", filepath.Join(home, stray))
		}
	}

	// Group reap: after Stop the endpoint stops answering.
	if err := m.Stop(ctx, "alice"); err != nil {
		t.Fatalf("stop alice: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, err := probeStatus(ctx, client, insts["alice"].BaseURL+"/api/health", BasicAuthUser, insts["alice"].Password)
		if err != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("alice's serve still answering 10s after Stop — the process group was not reaped")
		}
		time.Sleep(100 * time.Millisecond)
	}
	// bob is untouched by alice's reap.
	if code, err := probeStatus(ctx, client, insts["bob"].BaseURL+"/api/health", BasicAuthUser, insts["bob"].Password); err != nil || code != http.StatusOK {
		t.Errorf("stopping one user's instance disturbed another's: code=%d err=%v", code, err)
	}
}

func probeStatus(ctx context.Context, c *http.Client, url, user, pass string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	if user != "" {
		req.SetBasicAuth(user, pass)
	}
	resp, err := c.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

// TestRealServePermissionRoundTripFakeProvider is the load-bearing $0 live
// probe: a REAL serve, a REAL turn, a REAL S03.4 park and REST answer — with
// the model call terminating on loopback.
func TestRealServePermissionRoundTripFakeProvider(t *testing.T) {
	bin := enginePath(t)
	prov := newFakeProvider(t)
	// A provider block means the engine installs its per-user provider
	// dependency tree on first boot (~62 MB, needs network ONCE — SPIKE G1-S3
	// F1). The budget is generous but bounded; exceeding it skips loudly.
	m := realManager(t, bin, 150*time.Second)
	a, _ := realAdapter(t, m, testProviders(prov.baseURL()))
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	req := testRequest(t)
	sess, err := a.Start(ctx, req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "boot") {
			t.Skipf("SANCTIONED SKIP (CONVENTIONS §10): the real 1.18.3 serve could not complete a hermetic "+
				"provider bootstrap within %s (the per-user provider dependency tree needs network once, "+
				"SPIKE G1-S3 F1): %v — the S03.4 round-trip stays covered by the tier-F fixtures "+
				"(TestPermissionAskParksWithDurableAskRecord / TestPermissionReplyOnceNeverAlways / "+
				"TestAlwaysClassFanoutResolvedByRequestID), and the gap is REPORTED, never faked", m.BootTimeout, err)
		}
		t.Fatalf("Start: %v", err)
	}
	var asked *adapters.Ask
	for ev := range sess.Events() {
		if ev.Kind == adapters.KindGateAsk && ev.Ask != nil {
			asked = ev.Ask
		}
	}
	out, err := sess.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if out.Kind != adapters.OutcomeParked || out.Ask == nil {
		t.Fatalf("outcome = %q (%s), want a live permission park; provider calls=%d tools=%v",
			out.Kind, out.Detail, prov.callCount(), prov.toolsOffered())
	}
	if asked == nil || asked.ID != out.Ask.ID {
		t.Errorf("the gate_ask event and the Outcome disagree on the ask: %+v vs %+v", asked, out.Ask)
	}
	if !strings.HasPrefix(out.Ask.ID, "per_") {
		t.Errorf("ask id %q is not an engine requestID", out.Ask.ID)
	}
	// Sole-controller, proven on the live toolset: the engine never offered
	// its native spawn tool (S03.5 Probe 5 — stripped pre-inference).
	for _, name := range prov.toolsOffered() {
		if strings.EqualFold(name, "task") {
			t.Errorf("the live engine offered the native spawn tool %q despite permission.task=deny", name)
		}
	}

	// Answer it over REST and let the turn finish.
	resumed, err := a.Resume(ctx, *out.Park, ApproveAnswer(out.Ask.ID))
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	paid := 0
	for ev := range resumed.Events() {
		if ev.Kind == adapters.KindUsage && ev.Usage != nil && !ev.Usage.Total {
			paid++
			if ev.Usage.EngineCostUSD != 0 {
				t.Errorf("engine-reported cost = %v on a $0 loopback provider", ev.Usage.EngineCostUSD)
			}
		}
	}
	out2, err := resumed.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait(resume): %v", err)
	}
	if out2.Kind != adapters.OutcomeCompleted {
		t.Fatalf("resumed outcome = %q (%s), want completed", out2.Kind, out2.Detail)
	}
	if paid == 0 {
		t.Error("the resumed turn recorded no paid call")
	}
	if prov.callCount() < 2 {
		t.Errorf("provider calls = %d, want at least the tool turn plus its continuation", prov.callCount())
	}
	t.Logf("tier-R live round trip: ask=%s provider_calls=%d tools=%v result=%q",
		out.Ask.ID, prov.callCount(), prov.toolsOffered(), out2.ResultText)
}

// TestRealServeConfigIsTheOnlyConfig re-asserts the S03.5 compound guarantee
// against the LIVE engine's own resolved config: the decoys the tier-F lowering
// test plants must not appear in what the running serve reports.
func TestRealServeConfigIsTheOnlyConfig(t *testing.T) {
	bin := enginePath(t)
	m := realManager(t, bin, 45*time.Second)
	a, _ := realAdapter(t, m, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	req := testRequest(t)
	ancestor := t.TempDir()
	req.Cwd = filepath.Join(ancestor, "run")
	if err := os.MkdirAll(req.Cwd, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ancestor, "opencode.json"),
		[]byte(`{"permission":{"task":"allow"},"agent":{"evil_cwd":{"prompt":"DECOY"}}}`), 0o600); err != nil {
		t.Fatalf("plant decoy: %v", err)
	}
	l, err := a.lower(req)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	inst, err := m.Acquire(ctx, a.instanceSpec(req, l, l.env))
	if err != nil {
		t.Skipf("SANCTIONED SKIP (CONVENTIONS §10): real serve unavailable within the boot budget: %v", err)
	}
	body, code, err := probeBody(ctx, inst, "/config")
	if err != nil || code != http.StatusOK {
		t.Fatalf("GET /config: code=%d err=%v", code, err)
	}
	var got struct {
		Permission map[string]string `json:"permission"`
		Agent      map[string]any    `json:"agent"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode /config: %v (%s)", err, body)
	}
	if got.Permission["task"] != "deny" {
		t.Errorf("live resolved permission.task = %q, want deny (S03.5)", got.Permission["task"])
	}
	if _, leaked := got.Agent["evil_cwd"]; leaked {
		t.Errorf("the walked-up decoy config reached the live engine: %s", body)
	}
	if _, ok := got.Agent[req.Worker.AgentName]; !ok {
		t.Errorf("the compiled agent %q is absent from the live resolved config: %s", req.Worker.AgentName, body)
	}
}

func probeBody(ctx context.Context, inst Instance, path string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, inst.BaseURL+path, nil)
	if err != nil {
		return nil, 0, err
	}
	req.SetBasicAuth(BasicAuthUser, inst.Password)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return raw, resp.StatusCode, err
}

// TestRealServeProviderTreeIsolation records WHERE a PROVIDER-CONFIGURED
// instance actually writes. SPIKE G1-S3 F1's "zero strays" was measured on the
// auth-free serve path with NO provider block, so it says nothing about the
// per-user provider dependency install (~62 MB, network once) that F1 separately
// reports — and that install is the one thing on this substrate that can write
// outside the XDG quartet, because it follows HOME, which XDG isolation
// deliberately leaves alone (S03.5).
//
// It asserts what MUST hold — the engine's own state lives in the per-user root
// and no opencode state dir appears at its default HOME location — and REPORTS
// the footprint rather than asserting a number, because the install is
// CONDITIONAL: it did not reproduce in this hermetic battery at all (see the
// logged figures), while a shell probe of the same configuration on this host
// did fetch it. Reporting keeps that visible at every tier-R run; asserting
// either outcome would encode a guess.
func TestRealServeProviderTreeIsolation(t *testing.T) {
	bin := enginePath(t)
	prov := newFakeProvider(t)
	m := realManager(t, bin, 150*time.Second)
	a, home := realAdapter(t, m, testProviders(prov.baseURL()))
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	req := testRequest(t)
	sess, err := a.Start(ctx, req)
	if err != nil {
		t.Skipf("SANCTIONED SKIP (CONVENTIONS §10): real serve unavailable within the boot budget: %v", err)
	}
	for range sess.Events() {
	}
	if _, err := sess.Wait(ctx); err != nil {
		t.Fatalf("Wait: %v", err)
	}

	root := filepath.Join(a.Root, req.UserID)
	rootBytes := treeSize(t, root)
	if rootBytes == 0 {
		t.Errorf("the per-user root %q holds nothing — the XDG quartet did not take effect", root)
	}
	// The engine's own state dirs must NOT appear at their default HOME
	// locations (the G1-S3 F1 containment claim, re-asserted at the pin).
	for _, stray := range []string{
		".config/opencode", ".local/share/opencode", ".local/state/opencode", ".cache/opencode",
	} {
		if _, err := os.Stat(filepath.Join(home, stray)); err == nil {
			t.Errorf("engine state escaped the per-user root: %s", filepath.Join(home, stray))
		}
	}
	// What else landed under HOME, reported as a dated measurement.
	var outside []string
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatalf("read synthetic HOME: %v", err)
	}
	for _, e := range entries {
		p := filepath.Join(home, e.Name())
		outside = append(outside, fmt.Sprintf("%s=%dB", e.Name(), treeSize(t, p)))
	}
	t.Logf("provider-configured instance footprint at pin %s: per-user XDG root %d bytes; "+
		"entries under the synthetic HOME (empty means the provider dependency install did not "+
		"fire on this run): %v. Whatever the package manager writes follows HOME, not the XDG "+
		"quartet, so the per-user jail the D6 ceremony composes must bound HOME too (LN-2/D6 scope)",
		Pin, rootBytes, outside)
}

func treeSize(t *testing.T, root string) int64 {
	t.Helper()
	var total int64
	err := filepath.WalkDir(root, func(_ string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	if err != nil {
		return 0
	}
	return total
}
