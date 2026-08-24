package main

// main_test.go — the hermetic battery for the ceremony's credential tool.
//
// Every case runs against a temp store and the SHIPPED lane documents. Nothing
// here touches the network, the operator's real broker store, or a real key:
// the only secret in the file is a sentinel, and the tests that matter most are
// the ones asserting that sentinel never appears anywhere it should not.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sentinel stands in for a real credential. It is deliberately distinctive so a
// containment scan over the tool's whole output cannot pass by accident.
const sentinel = "sk-SENTINEL-lanekey-hermetic-do-not-use-0123456789"

// shippedLane is the path to a real lane document, so the tool is exercised
// against the documents the ceremony will actually read rather than a fixture
// that could drift away from them.
func shippedLane(t *testing.T, lane string) string {
	t.Helper()
	p := filepath.Join("..", "..", "internal", "adapters", "opencode", "lanedata", lane+".json")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("shipped lane document %s: %v", p, err)
	}
	return p
}

// invoke runs the tool the way main does, capturing both streams.
func invoke(stdin string, args ...string) (code int, stdout, stderr string) {
	var out, errb bytes.Buffer
	code = run(args, strings.NewReader(stdin), &out, &errb)
	return code, out.String(), errb.String()
}

func TestSecretShapedFlagsAreRefusedBeforeAnythingRuns(t *testing.T) {
	for _, name := range secretFlagNames {
		for _, form := range []string{"--" + name + "=" + sentinel, "-" + name} {
			code, stdout, stderr := invoke("", "put", form)
			if code != 2 {
				t.Errorf("%s: exit = %d, want 2 (refusal)", form, code)
			}
			if !strings.Contains(stderr, "REFUSED") {
				t.Errorf("%s: stderr did not refuse by name: %q", form, stderr)
			}
			// The refusal must not itself echo what it refused.
			if strings.Contains(stdout+stderr, sentinel) {
				t.Errorf("%s: the refusal ECHOED the secret it refused", form)
			}
		}
	}
}

func TestPutRoundTripsThroughTheBrokerAndNeverPrintsTheSecret(t *testing.T) {
	store := t.TempDir()
	lane := shippedLane(t, "zai")

	code, stdout, stderr := invoke(sentinel+"\n", "put", "--store-dir", store, "--user", "u1", "--lane", lane)
	if code != 0 {
		t.Fatalf("put: exit = %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "MATCH") || !strings.Contains(stdout, "PASS") {
		t.Errorf("put did not report a round-trip match:\n%s", stdout)
	}
	if strings.Contains(stdout+stderr, sentinel) {
		t.Fatalf("THE TOOL PRINTED THE CREDENTIAL — stdout: %q stderr: %q", stdout, stderr)
	}

	// verify is the re-runnable half: it must pass against what put placed.
	code, stdout, stderr = invoke("", "verify", "--store-dir", store, "--user", "u1", "--lane", lane)
	if code != 0 {
		t.Fatalf("verify: exit = %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "PASS") {
		t.Errorf("verify did not pass:\n%s", stdout)
	}
	if strings.Contains(stdout+stderr, sentinel) {
		t.Fatalf("verify PRINTED THE CREDENTIAL: %q %q", stdout, stderr)
	}

	// The record on disk is ciphertext: the store is the one place the secret
	// lives, and it must not live there in the clear.
	walkStore(t, store, func(path string, blob []byte) {
		if bytes.Contains(blob, []byte(sentinel)) {
			t.Fatalf("%s holds the credential in PLAINTEXT", path)
		}
	})
}

func TestFingerprintIsSaltedPerRunSoItProvesNothingAcrossRuns(t *testing.T) {
	store := t.TempDir()
	lane := shippedLane(t, "zai")
	if code, _, e := invoke(sentinel+"\n", "put", "--store-dir", store, "--user", "u1", "--lane", lane); code != 0 {
		t.Fatalf("put: %s", e)
	}
	_, first, _ := invoke("", "verify", "--store-dir", store, "--user", "u1", "--lane", lane)
	_, second, _ := invoke("", "verify", "--store-dir", store, "--user", "u1", "--lane", lane)
	if first == second {
		t.Error("two verifies of the SAME credential printed the same fingerprint — the salt is not per-run, " +
			"so the printed value is a stable hash of the key and comparable outside its run")
	}
}

func TestVerifyFailsWhenNothingWasPlaced(t *testing.T) {
	code, _, stderr := invoke("", "verify", "--store-dir", t.TempDir(), "--user", "u1", "--lane", shippedLane(t, "kimi"))
	if code == 0 {
		t.Fatal("verify PASSED against an empty store — an uncommissioned lane must not report itself commissioned")
	}
	if !strings.Contains(stderr, "kimi-code") {
		t.Errorf("the failure does not name the profile it looked for: %q", stderr)
	}
}

func TestStdinIsTheOnlyChannelAndItValidates(t *testing.T) {
	store := t.TempDir()
	lane := shippedLane(t, "zai")
	for name, in := range map[string]string{
		"empty":        "",
		"newline only": "\n",
		"has a space":  "sk-abc def\n",
		"has a tab":    "sk-abc\tdef\n",
	} {
		code, _, stderr := invoke(in, "put", "--store-dir", store, "--user", "u1", "--lane", lane)
		if code == 0 {
			t.Errorf("%s: put ACCEPTED it", name)
		}
		if !strings.Contains(stderr, "nothing was written") {
			t.Errorf("%s: the refusal does not say the store was left alone: %q", name, stderr)
		}
	}
	// Nothing was placed by any of those attempts.
	if _, err := os.Stat(filepath.Join(store, "u1", "zai-coding-plan.cred")); !os.IsNotExist(err) {
		t.Errorf("a refused put still wrote a record (stat err = %v)", err)
	}
}

func TestShowDerivesEveryLaneFactFromTheDocument(t *testing.T) {
	for lane, want := range map[string][]string{
		"zai":  {"profile=zai-coding-plan", "env_var=ZAI_API_KEY", "protocol=openai-compatible", "endpoint_marker=/api/coding/paas/v4"},
		"kimi": {"profile=kimi-code", "env_var=KIMI_API_KEY", "protocol=anthropic", "endpoint_marker=/coding/v1"},
	} {
		code, stdout, stderr := invoke("", "show", "--lane", shippedLane(t, lane))
		if code != 0 {
			t.Fatalf("%s: show exit = %d (%s)", lane, code, stderr)
		}
		for _, w := range want {
			if !strings.Contains(stdout, w) {
				t.Errorf("%s: show did not derive %q:\n%s", lane, w, stdout)
			}
		}
	}
}

func TestProbeRequestSpeaksTheProtocolTheDocumentDeclares(t *testing.T) {
	zai, err := loadLane(shippedLane(t, "zai"), "")
	if err != nil {
		t.Fatalf("load zai: %v", err)
	}
	url, _, hdr := probeRequest(zai, invalidProbeKey)
	if !strings.HasSuffix(url, "/chat/completions") {
		t.Errorf("zai probe url = %q, want the OpenAI-compatible completions path", url)
	}
	if hdr["Authorization"] != "Bearer "+invalidProbeKey {
		t.Errorf("zai probe did not present a bearer token: %q", hdr["Authorization"])
	}
	if _, ok := hdr["anthropic-version"]; ok {
		t.Error("zai probe sent an Anthropic-protocol header")
	}

	kimi, err := loadLane(shippedLane(t, "kimi"), "")
	if err != nil {
		t.Fatalf("load kimi: %v", err)
	}
	url, _, hdr = probeRequest(kimi, invalidProbeKey)
	if !strings.HasSuffix(url, "/messages") {
		t.Errorf("kimi probe url = %q, want the Anthropic messages path", url)
	}
	if hdr["x-api-key"] != invalidProbeKey || hdr["anthropic-version"] == "" {
		t.Errorf("kimi probe did not speak the Anthropic protocol: %+v", hdr)
	}
}

// TestWire401CarriesOnlyTheLiteralInvalidKey is the structural guarantee behind
// the capture step: the probe key is a compile-time constant, so no code path
// exists from a placed credential to the wire capture.
func TestWire401CarriesOnlyTheLiteralInvalidKey(t *testing.T) {
	if !strings.Contains(invalidProbeKey, "INVALID") {
		t.Errorf("the probe key %q does not announce itself as invalid", invalidProbeKey)
	}
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read own source: %v", err)
	}
	body := string(src)
	start := strings.Index(body, "func cmdWire401(")
	if start < 0 {
		t.Fatal("cmdWire401 not found")
	}
	end := strings.Index(body[start:], "\nfunc ")
	if end < 0 {
		t.Fatal("could not bound cmdWire401")
	}
	if strings.Contains(body[start:start+end], "OpenStore") {
		t.Error("cmdWire401 opens the broker store — the capture must have no path to a real credential")
	}
}

func walkStore(t *testing.T, root string, check func(path string, blob []byte)) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, "u1"))
	if err != nil {
		t.Fatalf("read store: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("the store is empty after a successful put")
	}
	for _, e := range entries {
		p := filepath.Join(root, "u1", e.Name())
		blob, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		check(p, blob)
	}
}
