package local_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/metering"
)

// gpubroker_test.go — hermetic tests of the S12.6 sandboxed plane (brief
// R3–R5/R17/R18): the per-run token registry and the /v1 bridge handler. No v0
// consumer mints a token (BINDING), so this is the designed seam under test —
// never a live path.

// TestTokenRegistry proves mint→resolve→owner + run-lifetime revoke, and that
// C0 connectors are NEVER granted the bridge (R3/R6).
func TestTokenRegistry(t *testing.T) {
	tr := local.NewTokenRegistry()
	tok, err := tr.Mint(local.SandboxGrant{RunID: "r1", Owner: "alice", Class: "C1", ModelAllowlist: []string{"utility"}})
	if err != nil {
		t.Fatalf("Mint C1: %v", err)
	}
	g, ok := tr.Resolve(tok)
	if !ok || g.RunID != "r1" || g.Owner != "alice" {
		t.Fatalf("Resolve = %+v ok=%v, want run r1/owner alice (token→run→owner)", g, ok)
	}
	tr.RevokeRun("r1")
	if _, ok := tr.Resolve(tok); ok {
		t.Error("token survived RevokeRun — grants are run-lifetime (R3)")
	}
	if _, err := tr.Mint(local.SandboxGrant{RunID: "r2", Class: "C0", ModelAllowlist: []string{"utility"}}); err == nil {
		t.Error("C0 connector was granted the bridge — C0 never gets it (S12.6/R6)")
	}
}

func newBroker(t *testing.T) (*local.SandboxBroker, *local.TokenRegistry, *dutyEnv) {
	t.Helper()
	e := newDutyEnv(t)
	tr := local.NewTokenRegistry()
	b := local.NewSandboxBroker(local.SandboxBrokerDeps{
		Tokens: tr, Registry: local.NewRegistry(e.reg), Client: local.NewClient(e.fake.URL),
		Checkpoints: e.cps, Events: e.log, Settings: e.reg,
	})
	return b, tr, e
}

func do(t *testing.T, srv *httptest.Server, method, path, token, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// TestSandboxedPlaneSurface proves the R4/R5 surface: exact routes, token auth,
// allowlist-only /v1/models, alias-in-allowlist enforcement, logprobs refusal,
// absent management verbs — and the $0 D7 write on the SANDBOXED run (R17).
func TestSandboxedPlaneSurface(t *testing.T) {
	ctx := context.Background()
	b, tr, e := newBroker(t)
	e.runningRun(t, "sbx.run")
	e.fake.SetModelResponse("Qwen3.5-4B", local.FakeResponse{Content: `{"abstain":true}`, InputTokens: 20, OutputTokens: 4})

	srv := httptest.NewServer(b.Handler())
	t.Cleanup(srv.Close)

	tok, err := tr.Mint(local.SandboxGrant{
		RunID: "sbx.run", Owner: "alice", Class: "C1",
		ModelAllowlist: []string{local.AliasIntakeTriage}, MaxRequestBytes: 1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}

	// No token ⇒ 401.
	if r := do(t, srv, http.MethodPost, "/v1/chat/completions", "", `{"model":"intake-triage"}`); r.StatusCode != http.StatusUnauthorized {
		t.Errorf("no token: status %d, want 401", r.StatusCode)
	}
	// A bare token WITHOUT the "Bearer " scheme ⇒ 401 (strict parse, F4).
	bare, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/models", nil)
	bare.Header.Set("Authorization", tok)
	if r, err := srv.Client().Do(bare); err != nil {
		t.Fatal(err)
	} else if r.StatusCode != http.StatusUnauthorized {
		t.Errorf("bare token (no Bearer scheme): status %d, want 401 (strict RFC 6750 parse)", r.StatusCode)
	}
	// Allowed alias ⇒ 200.
	if r := do(t, srv, http.MethodPost, "/v1/chat/completions", tok, `{"model":"intake-triage","messages":[{"role":"user","content":"x"}]}`); r.StatusCode != http.StatusOK {
		t.Fatalf("allowed alias: status %d, want 200", r.StatusCode)
	}
	// A raw model id / an alias outside the allowlist ⇒ 403.
	if r := do(t, srv, http.MethodPost, "/v1/chat/completions", tok, `{"model":"utility"}`); r.StatusCode != http.StatusForbidden {
		t.Errorf("disallowed alias: status %d, want 403", r.StatusCode)
	}
	// Logprobs with ⚙ off ⇒ 403 (side-channel reduction, R5).
	if r := do(t, srv, http.MethodPost, "/v1/chat/completions", tok, `{"model":"intake-triage","logprobs":true}`); r.StatusCode != http.StatusForbidden {
		t.Errorf("logprobs with ⚙ off: status %d, want 403", r.StatusCode)
	}
	// /v1/models returns ONLY the allowlist.
	r := do(t, srv, http.MethodGet, "/v1/models", tok, "")
	if r.StatusCode != http.StatusOK {
		t.Fatalf("/v1/models: status %d", r.StatusCode)
	}
	var models struct {
		Data []struct{ ID string } `json:"data"`
	}
	decode(t, r, &models)
	if len(models.Data) != 1 || models.Data[0].ID != local.AliasIntakeTriage {
		t.Errorf("/v1/models = %+v, want ONLY the run's allowlist [intake-triage]", models.Data)
	}
	// Management verbs are ABSENT by construction (R2).
	if r := do(t, srv, http.MethodPost, "/api/models/unload", tok, ""); r.StatusCode != http.StatusNotFound {
		t.Errorf("management verb /api/models/unload: status %d, want 404 (absent on the sandboxed plane)", r.StatusCode)
	}
	// /v1/embeddings exists but the embedder is post-gate (R4).
	if r := do(t, srv, http.MethodPost, "/v1/embeddings", tok, `{"model":"embedder"}`); r.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("/v1/embeddings: status %d, want 503 (post-gate)", r.StatusCode)
	}

	// R17: the allowed call wrote ONE $0 zero-allowance local line on the
	// SANDBOXED run, reusing the B4-5 marker (no metering-package change).
	led := metering.NewLedger(e.db, nil, metering.NoMeteredExceptions(), e.reg)
	rc, err := led.RunConsumption(ctx, "sbx.run")
	if err != nil {
		t.Fatalf("RunConsumption: %v", err)
	}
	var localLine *metering.LineItem
	for i := range rc.Items {
		if rc.Items[i].Lane == metering.LaneLocal {
			localLine = &rc.Items[i]
		}
	}
	if localLine == nil {
		t.Fatalf("no local-lane line on the sandboxed run (the marker override did not fire, R17); items=%+v", rc.Items)
	}
	if localLine.Unpriced() || !localLine.ZeroAllowance() {
		t.Error("sandboxed $0 line must be TRUE $0 zero-allowance, never UNPRICED (R17)")
	}
}

func decode(t *testing.T, r *http.Response, v any) {
	t.Helper()
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

// TestSandboxedMarkerUsesRequestedAlias proves the D7 marker records the
// REQUESTED alias, never the grant's first alias (F1): with a multi-alias grant
// [utility, intake-triage], a call to intake-triage (NOT first) must record a
// consistent (duty=intake-triage, model=Qwen3.5-4B) pair — not (utility, 4B).
func TestSandboxedMarkerUsesRequestedAlias(t *testing.T) {
	ctx := context.Background()
	b, tr, e := newBroker(t)
	e.runningRun(t, "sbx.multi")
	e.fake.SetModelResponse("Qwen3.5-4B", local.FakeResponse{Content: `{"abstain":true}`, InputTokens: 7, OutputTokens: 2})

	srv := httptest.NewServer(b.Handler())
	t.Cleanup(srv.Close)

	tok, err := tr.Mint(local.SandboxGrant{
		RunID: "sbx.multi", Owner: "alice", Class: "C1",
		ModelAllowlist: []string{local.AliasUtility, local.AliasIntakeTriage}, MaxRequestBytes: 1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if r := do(t, srv, http.MethodPost, "/v1/chat/completions", tok, `{"model":"intake-triage","messages":[{"role":"user","content":"x"}]}`); r.StatusCode != http.StatusOK {
		t.Fatalf("multi-alias call: status %d, want 200", r.StatusCode)
	}

	var usageJSON string
	if err := e.db.QueryRowContext(ctx, `SELECT usage_json FROM checkpoints WHERE run_id = ?`, "sbx.multi").Scan(&usageJSON); err != nil {
		t.Fatalf("read checkpoint usage: %v", err)
	}
	var marker struct {
		Local local.LocalMarker `json:"local"`
	}
	if err := json.Unmarshal([]byte(usageJSON), &marker); err != nil {
		t.Fatalf("parse marker: %v", err)
	}
	if marker.Local.Duty != local.AliasIntakeTriage {
		t.Errorf("D7 marker duty = %q, want the REQUESTED alias %q (not the grant's first alias)", marker.Local.Duty, local.AliasIntakeTriage)
	}
	if marker.Local.Model != "Qwen3.5-4B" {
		t.Errorf("D7 marker model = %q, want Qwen3.5-4B (the requested alias's seat)", marker.Local.Model)
	}
}
