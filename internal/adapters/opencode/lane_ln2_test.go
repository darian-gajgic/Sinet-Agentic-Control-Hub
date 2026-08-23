package opencode

// lane_ln2_test.go — LN-2A acceptance suite for the Z.AI/GLM lane (tier F).
//
// $0 ABSOLUTE. Nothing here reaches a provider: every turn terminates on the
// loopback fake serve, every credential is a sentinel, and the seed document's
// endpoint is asserted as DATA rather than dialled.

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
)

const (
	zaiProviderID = "zai-coding-plan"
	zaiCodingBase = "https://api.z.ai/api/coding/paas/v4"
	zaiGeneralBSE = "https://api.z.ai/api/paas/v4"
)

func seedZAI(t *testing.T) LaneConfig {
	t.Helper()
	seeds, err := SeedLaneConfigs()
	if err != nil {
		t.Fatalf("SeedLaneConfigs: %v", err)
	}
	for _, c := range seeds {
		if c.Lane == adapters.LaneZAI {
			return c
		}
	}
	t.Fatalf("no seed lane document for %q (lanes: %d)", adapters.LaneZAI, len(seeds))
	return LaneConfig{}
}

// otherLaneDoc is a DIFFERENT lane document — different endpoint, different
// model ids, different credential. It exists to prove the lane is data: if any
// of the seed's values were compiled in, this document could not flow through
// unchanged.
func otherLaneDoc(t *testing.T) LaneConfig {
	t.Helper()
	c, err := LoadLaneConfig([]byte(`{
	  "lane": "zai",
	  "provider_id": "elsewhere-plan",
	  "npm": "@ai-sdk/openai-compatible",
	  "display_name": "Elsewhere Plan",
	  "verified_on": "2026-08-23",
	  "base_url": "https://api.elsewhere.example/api/coding/paas/v9",
	  "endpoint_marker": "/api/coding/paas/v9",
	  "credential": {"profile": "elsewhere", "env_var": "ELSEWHERE_API_KEY"},
	  "models": [
	    {"id": "mdl-alpha", "name": "Alpha", "verified_on": "2026-08-23", "billing": "flat", "overflow_mode": "park"},
	    {"id": "mdl-beta",  "name": "Beta",  "verified_on": "2026-08-23", "billing": "flat", "overflow_mode": "park"}
	  ],
	  "eco_options": {"thinking": {"type": "disabled"}},
	  "reset_marker": "reset at",
	  "signals": [{"code": "1308", "http_status": 429, "verified_on": "2026-08-23"}]
	}`))
	if err != nil {
		t.Fatalf("LoadLaneConfig(other): %v", err)
	}
	return c
}

// laneAdapter builds an adapter commissioned on one lane document.
func laneAdapter(t *testing.T, c LaneConfig) *Adapter {
	t.Helper()
	a := testAdapter(t)
	a.Providers = c.Providers()
	a.Lanes = []LaneConfig{c}
	return a
}

func laneRequest(t *testing.T, c LaneConfig) adapters.StartRequest {
	t.Helper()
	req := testRequest(t)
	req.Model = c.ProviderID + "/" + c.Models[0].ID
	return req
}

// packageSources lists the package's non-test Go files.
func packageSources(t *testing.T) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	out := map[string]string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		out[name] = string(raw)
	}
	if len(out) == 0 {
		t.Fatal("no non-test sources found — the source scan would pass vacuously")
	}
	return out
}

// ── spec 1 · the provider entry is DATA ──────────────────────────────────────

func TestZAIProviderEntryIsData(t *testing.T) {
	seed := seedZAI(t)
	if seed.ProviderID != zaiProviderID {
		t.Errorf("provider id = %q, want %q (S03.6/R3: the ratified id)", seed.ProviderID, zaiProviderID)
	}
	if seed.BaseURL != zaiCodingBase {
		t.Errorf("endpoint = %q, want the CODING endpoint %q", seed.BaseURL, zaiCodingBase)
	}
	if seed.VerifiedOn == "" {
		t.Error("the endpoint carries no verified-on date")
	}
	wantModels := map[string]bool{"glm-5.3": false, "glm-5-turbo": false, "glm-4.7": false}
	for _, m := range seed.Models {
		if _, ok := wantModels[m.ID]; ok {
			wantModels[m.ID] = true
		}
		if m.VerifiedOn == "" {
			t.Errorf("model %q carries no verified-on date", m.ID)
		}
		if m.Billing != BillingFlat {
			t.Errorf("model %q billing = %q, want flat (the coding plan is flat-rate)", m.ID, m.Billing)
		}
	}
	for id, seen := range wantModels {
		if !seen {
			t.Errorf("seed model %q missing", id)
		}
	}
	// The general endpoint is RECORDED and not wired, so the self-check has
	// the wrong-endpoint case by name.
	var recordedGeneral bool
	for _, e := range seed.RecordedEndpoints {
		if e.BaseURL == zaiGeneralBSE {
			recordedGeneral = true
			if e.Wired {
				t.Error("the general endpoint is marked wired — it must be recorded, never wired")
			}
			if e.VerifiedOn == "" {
				t.Error("the recorded general endpoint carries no verified-on date")
			}
		}
	}
	if !recordedGeneral {
		t.Errorf("the general endpoint %q is not recorded as a dated fact", zaiGeneralBSE)
	}

	// A DIFFERENT endpoint + model set flows end to end unchanged.
	other := otherLaneDoc(t)
	a := laneAdapter(t, other)
	l, err := a.lower(laneRequest(t, other))
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	cfg := decodeConfig(t, l)
	entry, ok := cfg.Provider[other.ProviderID]
	if !ok {
		t.Fatalf("compiled config has no %q provider entry: %s", other.ProviderID, l.configJSON)
	}
	if base, _ := entry.Options["baseURL"].(string); base != other.BaseURL {
		t.Errorf("compiled baseURL = %v, want %q — the endpoint is not flowing from config", entry.Options["baseURL"], other.BaseURL)
	}
	for _, m := range other.Models {
		if _, ok := entry.Models[m.ID]; !ok {
			t.Errorf("compiled config lost model %q: %v", m.ID, entry.Models)
		}
	}
	if _, ok := entry.Models["glm-5.3"]; ok {
		t.Error("a seed model id leaked into a lowering built from a different document")
	}

	// No endpoint and no model id is a Go constant in a non-test file.
	for name, src := range packageSources(t) {
		for _, lit := range []string{"api.z.ai", "glm-5.3"} {
			if strings.Contains(src, lit) {
				t.Errorf("%s contains the literal %q — lane values are DATA with dates, never constants (S03.6)", name, lit)
			}
		}
	}
}

// ── spec 2 · per-user resolution ─────────────────────────────────────────────

func TestZAIProviderResolvesPerUser(t *testing.T) {
	alice, bob := seedZAI(t), otherLaneDoc(t)
	a := testAdapter(t)
	a.Providers = nil
	a.Lanes = []LaneConfig{alice, bob}
	a.ProvidersFor = func(userID string) (ProviderConfig, error) {
		switch userID {
		case "alice":
			return alice.Providers(), nil
		case "bob":
			return bob.Providers(), nil
		}
		return nil, nil
	}

	lowerFor := func(user string, c LaneConfig) *lowered {
		t.Helper()
		req := laneRequest(t, c)
		req.UserID = user
		l, err := a.lower(req)
		if err != nil {
			t.Fatalf("lower(%s): %v", user, err)
		}
		return l
	}
	la, lb := lowerFor("alice", alice), lowerFor("bob", bob)

	ca, cb := decodeConfig(t, la), decodeConfig(t, lb)
	if _, ok := ca.Provider[alice.ProviderID]; !ok {
		t.Errorf("alice's lowering lost her provider entry: %s", la.configJSON)
	}
	if _, ok := ca.Provider[bob.ProviderID]; ok {
		t.Error("alice's lowering carries bob's provider entry — the lane config is not per-user")
	}
	if _, ok := cb.Provider[bob.ProviderID]; !ok {
		t.Errorf("bob's lowering lost his provider entry: %s", lb.configJSON)
	}

	ia := identityOf(a.instanceSpec(withUser(laneRequest(t, alice), "alice"), la, la.env))
	ib := identityOf(a.instanceSpec(withUser(laneRequest(t, bob), "bob"), lb, lb.env))
	if ia == ib {
		t.Error("two users with different provider entries share one instance identity — one person's " +
			"lane config would serve the other (S03.2: one instance per user)")
	}
}

func withUser(req adapters.StartRequest, user string) adapters.StartRequest {
	req.UserID = user
	return req
}

// ── spec 3 · a provider change restarts only that user's serve ───────────────

func TestZAIProviderChangeRestartsOnlyThatUsersServe(t *testing.T) {
	seed, other := seedZAI(t), otherLaneDoc(t)
	var mu sync.Mutex
	entries := map[string]ProviderConfig{"alice": seed.Providers(), "bob": seed.Providers()}

	a := testAdapter(t)
	a.Providers = nil
	a.Lanes = []LaneConfig{seed, other}
	a.ProvidersFor = func(userID string) (ProviderConfig, error) {
		mu.Lock()
		defer mu.Unlock()
		return entries[userID], nil
	}

	identity := func(user string, c LaneConfig) string {
		t.Helper()
		req := withUser(laneRequest(t, c), user)
		req.Cwd = filepath.Join(t.TempDir(), "cwd-"+user)
		if err := os.MkdirAll(req.Cwd, 0o700); err != nil {
			t.Fatalf("cwd: %v", err)
		}
		l, err := a.lower(req)
		if err != nil {
			t.Fatalf("lower(%s): %v", user, err)
		}
		return identityOf(a.instanceSpec(req, l, l.env))
	}
	aliceBefore, bobBefore := identity("alice", seed), identity("bob", seed)

	mu.Lock()
	entries["alice"] = other.Providers()
	mu.Unlock()

	aliceAfter, bobAfter := identity("alice", other), identity("bob", seed)
	if aliceAfter == aliceBefore {
		t.Error("alice's provider change did not move her instance identity — the engine resolves the " +
			"provider block at STARTUP, so a live serve would keep running the old lane (§61(a))")
	}
	if bobAfter != bobBefore {
		t.Error("bob's serve identity moved when alice's entry changed — one person's lane edit restarts another's engine")
	}
}

// ── spec 4 · the credential appears nowhere (property) ───────────────────────

func TestZAICredentialNeverLeaves(t *testing.T) {
	const sentinel = "SINET-TEST-SECRET-6f1c2a9d-4b77-4b0e-9a11-0c5b7c1e2d33"
	e := newE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	e.claimedRun(t, "r1")

	f := newFakeServe(t)
	f.onPrompt = func(f *fakeServe, n int) { f.publishFixture("happy.sse") }

	seed := seedZAI(t)
	logs := &captureWriter{}
	spy := &spyInstances{inst: f.instance()}
	a := laneAdapter(t, seed)
	a.Instances = spy
	a.Log = slog.New(slog.NewTextHandler(logs, nil))

	req := laneRequest(t, seed)
	req.CredInject = func(base []string) ([]string, error) {
		return append(append([]string(nil), base...), seed.Credential.EnvVar+"="+sentinel), nil
	}
	var payloads []string
	req.OnEvent = func(ev adapters.Event) { payloads = append(payloads, string(ev.Payload)) }

	out, err := e.drv.Drive(ctx, a, req)
	if err != nil {
		t.Fatalf("Drive: %v", err)
	}
	if out.Kind != adapters.OutcomeCompleted {
		t.Fatalf("outcome = %q (%s)", out.Kind, out.Detail)
	}
	// The credential DID reach the engine: the whole point is that it reaches
	// the serve environment and nothing else.
	if len(spy.specs) != 1 {
		t.Fatalf("instance specs = %d, want 1", len(spy.specs))
	}
	if _, ok := envValue(spy.specs[0].Env, seed.Credential.EnvVar); !ok {
		t.Fatalf("the injected credential never reached the serve environment")
	}

	for i, p := range payloads {
		if strings.Contains(p, sentinel) {
			t.Errorf("event payload %d carries the credential: %s", i, p)
		}
	}
	if id := identityOf(spy.specs[0]); strings.Contains(id, sentinel) {
		t.Errorf("the instance identity key carries the credential: %q", id)
	}
	if s := logs.String(); strings.Contains(s, sentinel) {
		t.Error("the ops log carries the credential")
	}
	if p := out.Park; p != nil {
		blob, _ := json.Marshal(p)
		if strings.Contains(string(blob), sentinel) {
			t.Error("the park record carries the credential")
		}
	}
	rows, err := e.db.QueryContext(ctx, `SELECT payload FROM run_events WHERE run_id = 'r1'`)
	if err != nil {
		t.Fatalf("run_events: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if strings.Contains(payload, sentinel) {
			t.Errorf("a run_events payload carries the credential: %s", payload)
		}
	}
	// The whole store, not only the columns this test thought to name.
	if err := e.db.CheckpointTruncate(ctx); err != nil {
		t.Fatalf("checkpoint truncate: %v", err)
	}
	for _, suffix := range []string{"", "-wal"} {
		raw, err := os.ReadFile(e.db.Path() + suffix)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("read db%s: %v", suffix, err)
		}
		if strings.Contains(string(raw), sentinel) {
			t.Errorf("the database file%s contains the credential", suffix)
		}
	}
}

// ── spec 5 · a missing credential is a NAMED state ───────────────────────────

func TestZAIMissingCredentialIsNamedState(t *testing.T) {
	seed := seedZAI(t)
	f := newFakeServe(t)
	spy := &spyInstances{inst: f.instance()}
	a := laneAdapter(t, seed)
	a.Instances = spy

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// No CredInject at all: the lane is configured and NOT commissioned.
	_, err := a.Start(ctx, laneRequest(t, seed))
	if err == nil {
		t.Fatal("Start succeeded without a credential — an unauthenticated call is not a success")
	}
	if !strings.Contains(err.Error(), "not commissioned") {
		t.Errorf("error = %v, want a named not-commissioned state", err)
	}
	if !strings.Contains(err.Error(), seed.Credential.Profile) {
		t.Errorf("error = %v, want the auth-profile name so an operator knows what to place", err)
	}
	if len(spy.specs) != 0 {
		t.Errorf("a serve instance was acquired for an uncommissioned lane (%d specs)", len(spy.specs))
	}
	if n := len(f.requests()); n != 0 {
		t.Errorf("the uncommissioned lane made %d engine requests, want 0", n)
	}

	// A provider entry missing altogether is the same named state.
	a2 := laneAdapter(t, seed)
	a2.Providers = ProviderConfig{}
	a2.Instances = spy
	if _, err := a2.Start(ctx, laneRequest(t, seed)); err == nil ||
		!strings.Contains(err.Error(), "not commissioned") {
		t.Errorf("missing provider entry: err = %v, want a named not-commissioned state", err)
	}
}

// ── spec 6 · the Eco lever ───────────────────────────────────────────────────

func TestThinkingDisabledOnEcoOnly(t *testing.T) {
	seed := seedZAI(t)
	a := laneAdapter(t, seed)

	optionsFor := func(effort string) map[string]any {
		t.Helper()
		req := laneRequest(t, seed)
		req.EffortMode = effort
		l, err := a.lower(req)
		if err != nil {
			t.Fatalf("lower(%s): %v", effort, err)
		}
		var cfg struct {
			Provider map[string]struct {
				Models map[string]struct {
					Options map[string]any `json:"options"`
				} `json:"models"`
			} `json:"provider"`
		}
		if err := json.Unmarshal(l.configJSON, &cfg); err != nil {
			t.Fatalf("decode config: %v", err)
		}
		return cfg.Provider[seed.ProviderID].Models[seed.Models[0].ID].Options
	}

	eco := optionsFor(adapters.EffortEco)
	thinking, ok := eco["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("Eco produced no thinking lever: %v", eco)
	}
	if thinking["type"] != "disabled" {
		t.Errorf("Eco thinking.type = %v, want disabled (S10.6)", thinking["type"])
	}
	for _, effort := range []string{adapters.EffortSmart, adapters.EffortBalanced, ""} {
		opts := optionsFor(effort)
		if _, present := opts["thinking"]; present {
			t.Errorf("effort %q carries the Eco thinking lever: %v", effort, opts)
		}
	}
	// reasoning_effort is a recorded dated fact, deliberately NOT wired (OQ-4).
	for _, effort := range []string{adapters.EffortEco, adapters.EffortSmart} {
		if _, present := optionsFor(effort)["reasoning_effort"]; present {
			t.Errorf("effort %q wired reasoning_effort, which no named section sanctions (OQ-4)", effort)
		}
	}
	if seed.ReasoningEffort.Wired {
		t.Error("the seed marks reasoning_effort wired")
	}
	if seed.ReasoningEffort.VerifiedOn == "" {
		t.Error("reasoning_effort is not recorded as a DATED fact")
	}

	// Disclosure: the active mode is a state on the run's own record, not a
	// log line (S10.6: "a mode change is a visible state").
	req := laneRequest(t, seed)
	req.EffortMode = adapters.EffortEco
	s := &session{a: a, req: req, low: mustLower(t, a, req), p: newParser(fixtureSessionID, t.Logf)}
	var done struct {
		Effort string `json:"effort"`
	}
	if err := json.Unmarshal(s.donePayload(adapters.Outcome{Kind: adapters.OutcomeCompleted}), &done); err != nil {
		t.Fatalf("done payload: %v", err)
	}
	if done.Effort != adapters.EffortEco {
		t.Errorf("done payload effort = %q, want %q — the mode must be a disclosed state", done.Effort, adapters.EffortEco)
	}
}

func mustLower(t *testing.T, a *Adapter, req adapters.StartRequest) *lowered {
	t.Helper()
	l, err := a.lower(req)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	return l
}

// ── spec 7 · wire extraction ─────────────────────────────────────────────────

func zaiBody(code, message string) string {
	blob, err := json.Marshal(map[string]any{"error": map[string]any{"code": code, "message": message}})
	if err != nil {
		panic(err)
	}
	return string(blob)
}

func TestZAIErrorBodyExtractsLimitSignal(t *testing.T) {
	seed := seedZAI(t)
	const flush = "2026-08-23T18:00:00Z"
	cases := []struct {
		name       string
		body       string
		wantCode   string
		wantStatus int
		wantReset  bool
	}{
		{"1302 rate limit", zaiBody("1302", "Rate limit reached for requests"), "1302", 429, false},
		{"1305 overloaded", zaiBody("1305", "The service may be temporarily overloaded, please try again later"), "1305", 429, false},
		{"1308 usage limit", zaiBody("1308", "Usage limit reached for `20000` `credits`. Your limit will reset at `"+flush+"`"), "1308", 429, true},
		{"1310 weekly", zaiBody("1310", "Weekly/Monthly Limit Exhausted. Your limit will reset at `"+flush+"`"), "1310", 429, true},
		{"1113 balance", zaiBody("1113", "Insufficient balance or no resource package. Please recharge."), "1113", 429, false},
		{"1220 permission", zaiBody("1220", "You do not have permission to access chat"), "1220", 403, false},
		{"wrapped in engine prose", "AI_APICallError: provider said " + zaiBody("1308", "Usage limit reached for `1` `credit`. Your limit will reset at `"+flush+"`") + " (status 429)", "1308", 429, true},
	}
	want, err := time.Parse(time.RFC3339, flush)
	if err != nil {
		t.Fatalf("fixture time: %v", err)
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sig, ok := seed.ExtractSignal(c.body, 0)
			if !ok {
				t.Fatalf("no signal extracted from %s", c.body)
			}
			if sig.Lane != adapters.LaneZAI {
				t.Errorf("lane = %q, want %q", sig.Lane, adapters.LaneZAI)
			}
			if sig.ErrorCode != c.wantCode {
				t.Errorf("error code = %q, want %q (the wire carries it as a STRING)", sig.ErrorCode, c.wantCode)
			}
			if sig.HTTPStatus != c.wantStatus {
				t.Errorf("http status = %d, want %d", sig.HTTPStatus, c.wantStatus)
			}
			if c.wantReset && !sig.ResetAt.Equal(want) {
				t.Errorf("reset = %v, want the parsed next_flush_time %v", sig.ResetAt, want)
			}
			if !c.wantReset && !sig.ResetAt.IsZero() {
				t.Errorf("reset = %v, want zero — no resume time is signalled", sig.ResetAt)
			}
			if !sig.Known {
				t.Errorf("code %q is in the dated signal table but was reported unknown", c.wantCode)
			}
		})
	}
	// An observed status wins over the table's.
	if sig, _ := seed.ExtractSignal(zaiBody("1308", "x"), 503); sig.HTTPStatus != 503 {
		t.Errorf("observed status ignored: %d", sig.HTTPStatus)
	}
	// A numeric code is tolerated; an unparseable body yields no signal.
	if sig, ok := seed.ExtractSignal(`{"error":{"code":1308,"message":"x"}}`, 0); !ok || sig.ErrorCode != "1308" {
		t.Errorf("numeric code not tolerated: %+v ok=%v", sig, ok)
	}
	if _, ok := seed.ExtractSignal("connection reset by peer", 0); ok {
		t.Error("a non-provider error produced a lane signal")
	}
}

// ── spec 8 · an unknown code is forwarded, never fatal ───────────────────────

func zaiErrorFrame(t *testing.T, providerID, modelID, body string) []byte {
	t.Helper()
	frame, err := json.Marshal(map[string]any{
		"type": "message.updated",
		"properties": map[string]any{
			"sessionID": fixtureSessionID,
			"info": map[string]any{
				"id": "msg_err_1", "sessionID": fixtureSessionID, "role": "assistant",
				"modelID": modelID, "providerID": providerID,
				"time":  map[string]any{"created": 1, "completed": 2},
				"error": map[string]any{"name": "AI_APICallError", "data": map[string]any{"message": body}},
			},
		},
	})
	if err != nil {
		t.Fatalf("frame: %v", err)
	}
	return frame
}

func TestUnknownZAIErrorCodeIsForwardedNotFatal(t *testing.T) {
	seed := seedZAI(t)
	logs := &captureWriter{}
	a := laneAdapter(t, seed)
	a.Log = slog.New(slog.NewTextHandler(logs, nil))
	req := laneRequest(t, seed)
	s := &session{a: a, req: req, low: mustLower(t, a, req), p: newParser(fixtureSessionID, t.Logf)}
	s.p.lane = &seed
	s.p.logf = a.logf

	body := zaiBody("9999", "something nobody has documented")
	var limits []adapters.Event
	for _, ev := range s.p.feed(zaiErrorFrame(t, seed.ProviderID, seed.Models[0].ID, body)) {
		if ev.Kind == adapters.KindRateLimit {
			limits = append(limits, ev)
		}
	}
	if len(limits) != 1 {
		t.Fatalf("rate-limit events = %d, want 1 (forwarded as data, never dropped)", len(limits))
	}
	var got LaneSignal
	if err := json.Unmarshal(limits[0].Payload, &got); err != nil {
		t.Fatalf("payload: %v (%s)", err, limits[0].Payload)
	}
	if got.ErrorCode != "9999" {
		t.Errorf("error code = %q, want 9999", got.ErrorCode)
	}
	if got.Known {
		t.Error("an undocumented code was reported as known")
	}
	if !strings.Contains(logs.String(), "9999") {
		t.Errorf("the unknown code was absorbed silently; log = %q", logs.String())
	}
	// Never fatal, and never minted as a new platform kind.
	out, _ := s.assembleOutcome()
	if out.Kind == "" {
		t.Error("the session produced no outcome after an unknown provider code")
	}
	for _, ev := range limits {
		if ev.Kind != adapters.KindRateLimit {
			t.Errorf("event kind = %q — the closed platform set gained a member", ev.Kind)
		}
	}
}

// ── spec 9 · opencode's own cost is never a price ────────────────────────────

func TestOpenCodeCostNeverPricesZAI(t *testing.T) {
	seed := seedZAI(t)
	a := laneAdapter(t, seed)
	req := laneRequest(t, seed)
	s := &session{a: a, req: req, low: mustLower(t, a, req), p: newParser(fixtureSessionID, t.Logf)}
	s.p.lane = &seed

	frame, err := json.Marshal(map[string]any{
		"type": "message.updated",
		"properties": map[string]any{
			"sessionID": fixtureSessionID,
			"info": map[string]any{
				"id": "msg_cost_1", "sessionID": fixtureSessionID, "role": "assistant",
				"modelID": seed.Models[0].ID, "providerID": seed.ProviderID,
				"cost":   1.23,
				"tokens": map[string]any{"input": 11, "output": 7, "cache": map[string]any{"read": 3}},
				"finish": "stop",
				"time":   map[string]any{"created": 1, "completed": 2},
			},
		},
	})
	if err != nil {
		t.Fatalf("frame: %v", err)
	}
	var usage *adapters.Usage
	var payloads []string
	for _, ev := range s.p.feed(frame) {
		payloads = append(payloads, string(ev.Payload))
		if ev.Usage != nil {
			usage = ev.Usage
		}
	}
	if usage == nil {
		t.Fatal("no usage event")
	}
	if usage.EngineCostUSD != 1.23 {
		t.Errorf("engine cost = %v, want it carried VERBATIM as cross-check material", usage.EngineCostUSD)
	}
	if usage.InputTokens != 11 || usage.OutputTokens != 7 || usage.CacheReadTokens != 3 {
		t.Errorf("measured tokens lost: %+v", usage)
	}
	for _, p := range payloads {
		if strings.Contains(p, "priced") {
			t.Errorf("an adapter payload carries a priced figure: %s", p)
		}
	}
	// The adapter derives no dollar figure at all: pricing is Sinet's own
	// table (D5, S10.3), and opencode rates a flat-rate plan at $0.
	for name, src := range packageSources(t) {
		for _, sym := range []string{"priced_cost", "PricedCost", "PriceUSD", "pricePerToken"} {
			if strings.Contains(src, sym) {
				t.Errorf("%s references %q — this adapter must never meter dollars (S03.7)", name, sym)
			}
		}
	}
}
