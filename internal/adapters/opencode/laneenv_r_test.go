package opencode

// laneenv_r_test.go — tier R, LN-2A/D13: the credential path's one unmeasured
// assumption, measured.
//
// The compiled config names an ENVIRONMENT VARIABLE (`{env:VAR}`) and never a
// value, because a config body is hashed, logged and inspected. That design
// only works if the engine actually resolves the reference — and nothing in
// the packet had proven it at the pin. This probe proves it for REAL, at $0: a
// real `opencode serve` whose provider entry carries `{env:VAR}`, a sentinel in
// the serve environment, and a loopback fake provider that records the
// Authorization header it was sent.
//
// If the engine does NOT resolve the reference, this test fails loudly. That
// is the point: the fallback would be to put the material in the config body,
// and a silent workaround there is exactly what the containment property
// forbids.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

const laneEnvSentinel = "SINET-TEST-ZAI-KEY-8b41f0c7"

func TestRealServeResolvesTheCredentialEnvReference(t *testing.T) {
	bin := enginePath(t)
	prov := newFakeProvider(t)
	m := realManager(t, bin, 150*time.Second)

	// A lane document shaped exactly like the shipped one, pointed at the
	// loopback fake provider. No real endpoint, no real key.
	lane, err := LoadLaneConfig([]byte(`{
	  "lane": "zai",
	  "provider_id": "probe-plan",
	  "npm": "@ai-sdk/openai-compatible",
	  "display_name": "Probe Plan",
	  "verified_on": "2026-08-23",
	  "base_url": "` + prov.baseURL() + `/v1",
	  "endpoint_marker": "/v1",
	  "credential": {"profile": "probe", "env_var": "SINET_TEST_ZAI_KEY"},
	  "models": [{"id": "fakemodel", "name": "Fake Model", "verified_on": "2026-08-23",
	              "billing": "flat", "overflow_mode": "hard-stop", "region_model_gate": "none"}],
	  "reset_marker": "reset at",
	  "signals": [{"code": "1308", "http_status": 429, "verified_on": "2026-08-23"}]
	}`))
	if err != nil {
		t.Fatalf("lane document: %v", err)
	}
	// The config the engine reads names the VARIABLE, never the value.
	entry := lane.ProviderEntry()
	if got, _ := entry.Options["apiKey"].(string); got != "{env:SINET_TEST_ZAI_KEY}" {
		t.Fatalf("provider entry apiKey = %v, want an env reference", entry.Options["apiKey"])
	}

	a, _ := realAdapter(t, m, lane.Providers())
	a.Lanes = []LaneConfig{lane}

	req := testRequest(t)
	req.Model = lane.ProviderID + "/" + lane.Models[0].ID
	req.CredInject = func(base []string) ([]string, error) {
		return append(append([]string(nil), base...), lane.Credential.EnvVar+"="+laneEnvSentinel), nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()
	sess, err := a.Start(ctx, req)
	if err != nil {
		failOrSkipBoot(t, err)
	}
	for range sess.Events() {
	}
	if _, err := sess.Wait(ctx); err != nil {
		t.Fatalf("Wait: %v", err)
	}

	headers := prov.authHeaders()
	if len(headers) == 0 {
		t.Fatal("the engine never called the provider, so the credential path is unproven")
	}
	var sawSentinel, sawLiteral bool
	for _, h := range headers {
		if strings.Contains(h, laneEnvSentinel) {
			sawSentinel = true
		}
		if strings.Contains(h, "{env:") {
			sawLiteral = true
		}
	}
	if sawLiteral {
		t.Fatalf("the engine sent the env REFERENCE verbatim instead of resolving it: %q — the lane's "+
			"credential design (config names the variable, never the value) does not hold at this pin", headers)
	}
	if !sawSentinel {
		t.Fatalf("no request carried the injected credential: %q — `{env:VAR}` in options.apiKey did not "+
			"resolve from the serve environment at this pin", headers)
	}
	for _, h := range headers {
		if strings.Contains(h, laneEnvSentinel) && !strings.HasPrefix(h, "Bearer ") {
			t.Errorf("the credential was sent as %q, want a Bearer presentation", h)
		}
	}

	// And the containment half, on the real path: the compiled config the
	// engine received names the variable and never the material.
	l, err := a.lower(req)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if strings.Contains(string(l.configJSON), laneEnvSentinel) {
		t.Error("the compiled config carries the credential material")
	}
	var cfg struct {
		Provider map[string]struct {
			Options map[string]json.RawMessage `json:"options"`
		} `json:"provider"`
	}
	if err := json.Unmarshal(l.configJSON, &cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if !strings.Contains(string(cfg.Provider[lane.ProviderID].Options["apiKey"]), lane.Credential.EnvVar) {
		t.Errorf("the compiled config lost the env reference: %s", cfg.Provider[lane.ProviderID].Options["apiKey"])
	}
}
