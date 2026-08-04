package evals_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/evals"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/lockfile"
)

// The pinned runner is exercised hermetically against a STUB binary under
// t.TempDir() (the sandbox-runtime stub precedent): CI has no promptfoo, and
// the adapter's contract — the generated config, the self-hosted invocation,
// the results parse — is behavior we can assert without one.

// stubResults is a results file in promptfoo's schema at the pinned ref
// (recorded from examples/simple-cli/output.json at tag 0.121.19): the parsed
// subset is results.version, results.results[] and results.stats.
const stubResults = `{
  "evalId": "eval-stub",
  "results": {
    "version": 3,
    "timestamp": "2026-07-26T00:00:00.000Z",
    "prompts": [],
    "results": [
      {"success": true,  "score": 1, "testCase": {"metadata": {"case_id": "probe/one"}},
       "gradingResult": {"pass": true, "score": 1, "reason": "All assertions passed"}},
      {"success": false, "score": 0, "testCase": {"metadata": {"case_id": "probe/two"}},
       "gradingResult": {"pass": false, "score": 0, "reason": "Expected output to contain \"65535\""}}
    ],
    "stats": {"successes": 1, "failures": 1, "tokenUsage": {"total": 0}}
  }
}`

// stubPromptfoo writes a stub binary that answers --version and, for an eval,
// records its argv+env and writes results to the -o path.
func stubPromptfoo(t *testing.T, version, results string) (bin, trace string) {
	t.Helper()
	dir := t.TempDir()
	trace = t.TempDir()
	body := filepath.Join(dir, "results-body.json")
	if err := os.WriteFile(body, []byte(results), 0o600); err != nil {
		t.Fatal(err)
	}
	bin = filepath.Join(dir, "promptfoo")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then env > "` + trace + `/env-version"; echo "` + version + `"; exit 0; fi
printf '%s\n' "$@" > "` + trace + `/argv"
env > "` + trace + `/env"
out=""
while [ $# -gt 0 ]; do
  if [ "$1" = "-o" ]; then out="$2"; shift 2; else shift; fi
done
cp "$1" /dev/null 2>/dev/null
cat "` + body + `" > "$out"
exit 100
`
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return bin, trace
}

func (f *fix) readTrace(t *testing.T, trace, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(trace, name))
	if err != nil {
		t.Fatalf("the stub runner was never invoked (%s): %v", name, err)
	}
	return string(b)
}

// ── the fake-runner leg (rubric 4, 5) ──

func TestFakeRunnerParsesOutputAndRecords(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	bin, trace := stubPromptfoo(t, "0.121.19", stubResults)
	t.Setenv(evals.PromptfooPathEnv, bin)

	r, err := evals.FindPromptfoo()
	if err != nil {
		t.Fatalf("FindPromptfoo with the override: %v", err)
	}
	name, version, err := r.Identity(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if name != evals.RunnerPromptfoo || version != "0.121.19" {
		t.Fatalf("identity = %q %q", name, version)
	}

	out, err := r.Run(ctx, evals.RunConfig{
		Suite:    "probe-v1",
		Provider: "anthropic:claude-opus-4-8",
		Cases: []evals.Case{
			{ID: "probe/one", Prompt: "do the thing", Expect: []string{"func"}, Forbid: []string{"TODO"}},
			{ID: "probe/two", Prompt: "do the other", Expect: []string{"65535"}},
		},
	})
	// Exit 100 means "a test failed" — a legitimate red run, never an
	// invocation error (the stub exits 100 to prove exactly that).
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(out.Cases) != 2 || !out.Cases[0].Pass || out.Cases[1].Pass {
		t.Fatalf("parsed cases = %+v", out.Cases)
	}
	if out.Cases[1].ID != "probe/two" {
		t.Errorf("case identity did not survive the round trip: %+v", out.Cases[1])
	}
	if out.Successes != 1 || out.Failures != 1 || out.PassRate() != 0.5 {
		t.Errorf("stats = %d/%d rate %v", out.Successes, out.Failures, out.PassRate())
	}

	// Runner-never-store (P-T11-4): sharing refused on the command line,
	// telemetry and update checks off, and promptfoo's own eval history and
	// cache redirected into per-run scratch that is removed after the run.
	argv := f.readTrace(t, trace, "argv")
	for _, flag := range []string{"eval", "-c", "-o", "--no-share", "--no-progress-bar"} {
		if !strings.Contains(argv, flag) {
			t.Errorf("invocation misses %q: %s", flag, argv)
		}
	}
	if strings.Contains(argv, "--share") && !strings.Contains(argv, "--no-share") {
		t.Error("the invocation must never share results")
	}
	env := f.readTrace(t, trace, "env")
	for _, want := range []string{"PROMPTFOO_DISABLE_TELEMETRY=1", "PROMPTFOO_DISABLE_UPDATE=1",
		"PROMPTFOO_CONFIG_DIR=", "PROMPTFOO_CACHE_PATH="} {
		if !strings.Contains(env, want) {
			t.Errorf("child env misses %q", want)
		}
	}
	var scratch string
	for _, line := range strings.Split(env, "\n") {
		if v, ok := strings.CutPrefix(line, "PROMPTFOO_CONFIG_DIR="); ok {
			scratch = filepath.Dir(v)
		}
	}
	if scratch == "" {
		t.Fatal("no scratch dir was set")
	}
	if _, err := os.Stat(scratch); !os.IsNotExist(err) {
		t.Errorf("the runner's scratch dir survived the run (%s): its state must never be read back as a record", scratch)
	}

	// The parsed outcome lands in platform.db — the store, as opposed to the
	// runner.
	if err := f.store.RecordAsset(ctx, evals.AssetResult{
		AssetKind: evals.AssetEngine, AssetID: "claude-cli", AssetVersion: "2.1.219",
		Suite: "probe-v1", Runner: name, RunnerVersion: version,
		Metrics: map[string]any{"pass_rate": out.PassRate()}, Passed: false,
	}); err != nil {
		t.Fatal(err)
	}
	if f.sweepRow(t).LastResult != "red" {
		t.Error("the parsed red outcome did not land in platform.db")
	}
}

func TestCallerEnvCannotWeakenTheHardening(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	bin, trace := stubPromptfoo(t, "0.121.19", stubResults)
	t.Setenv(evals.PromptfooPathEnv, bin)
	r, err := evals.FindPromptfoo()
	if err != nil {
		t.Fatal(err)
	}
	// A hostile RunConfig tries to re-enable telemetry and update checks and to
	// redirect the runner's state back onto the host. Telemetry-off and the
	// scratch redirection are S16.4 adoption-walk invariants of the entry, not
	// caller conventions, so the hardening must win regardless.
	home := t.TempDir()
	_, err = r.Run(ctx, evals.RunConfig{
		Suite: "s", Provider: "p", Cases: []evals.Case{{ID: "probe/one", Prompt: "x"}},
		Env: []string{
			"PROMPTFOO_DISABLE_TELEMETRY=0",
			"PROMPTFOO_DISABLE_UPDATE=0",
			"PROMPTFOO_CONFIG_DIR=" + home,
			"ANTHROPIC_API_KEY=sentinel", // a legitimate provider entry survives
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	env := f.readTrace(t, trace, "env")
	for _, want := range []string{"PROMPTFOO_DISABLE_TELEMETRY=1", "PROMPTFOO_DISABLE_UPDATE=1", "ANTHROPIC_API_KEY=sentinel"} {
		if !strings.Contains(env, want) {
			t.Errorf("child env misses %q:\n%s", want, env)
		}
	}
	for _, forbidden := range []string{"PROMPTFOO_DISABLE_TELEMETRY=0", "PROMPTFOO_DISABLE_UPDATE=0", "PROMPTFOO_CONFIG_DIR=" + home} {
		if strings.Contains(env, forbidden) {
			t.Errorf("a caller entry defeated the hardening: %q", forbidden)
		}
	}
}

func TestIdentityRunsUnderScratchIsolation(t *testing.T) {
	// The version probe must leave no runner state on the host either: at the
	// B5-gate host install it would otherwise run against the operator's real
	// config dir.
	f := newFix(t)
	bin, trace := stubPromptfoo(t, "0.121.19", stubResults)
	t.Setenv(evals.PromptfooPathEnv, bin)
	r, err := evals.FindPromptfoo()
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	r.WorkDir = work
	if _, _, err := r.Identity(context.Background()); err != nil {
		t.Fatal(err)
	}
	env := f.readTrace(t, trace, "env-version")
	for _, want := range []string{"PROMPTFOO_DISABLE_TELEMETRY=1", "PROMPTFOO_CONFIG_DIR=" + work, "PROMPTFOO_CACHE_PATH=" + work} {
		if !strings.Contains(env, want) {
			t.Errorf("the version probe env misses %q:\n%s", want, env)
		}
	}
	// And it leaves nothing behind.
	entries, err := os.ReadDir(work)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("Identity left scratch state behind: %v", entries)
	}
}

func TestRunnerRefusesAnUnknownResultsSchema(t *testing.T) {
	ctx := context.Background()
	bin, _ := stubPromptfoo(t, "9.9.9", strings.Replace(stubResults, `"version": 3`, `"version": 4`, 1))
	t.Setenv(evals.PromptfooPathEnv, bin)
	r, err := evals.FindPromptfoo()
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Run(ctx, evals.RunConfig{Suite: "s", Provider: "p", Cases: []evals.Case{{ID: "a", Prompt: "x"}}})
	if err == nil || !strings.Contains(err.Error(), "schema version") {
		t.Fatalf("an unexpected results schema must fail loudly, got %v", err)
	}
}

func TestRunnerAbsenceIsNamedNotGuessed(t *testing.T) {
	t.Setenv(evals.PromptfooPathEnv, filepath.Join(t.TempDir(), "does-not-exist"))
	if _, err := evals.FindPromptfoo(); !errors.Is(err, evals.ErrRunnerAbsent) {
		t.Fatalf("missing override target: %v, want ErrRunnerAbsent", err)
	}
}

// ── pin ↔ lock coupling + the real leg (rubric 1, 2, 3, 5) ──

func TestPinMatchesLock(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot, "components.lock"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := lockfile.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	var pf, de *lockfile.Component
	for i := range f.Components {
		switch f.Components[i].Name {
		case "promptfoo":
			pf = &f.Components[i]
		case "DeepEval":
			de = &f.Components[i]
		}
	}
	if pf == nil {
		t.Fatal("components.lock has no promptfoo entry (S16.3 row must materialize at first consumption)")
	}
	if pf.Pin != evals.PromptfooPin {
		t.Errorf("lock pin %q != evals.PromptfooPin %q", pf.Pin, evals.PromptfooPin)
	}
	if pf.Kind != "library" || pf.License.SPDX != "MIT" || pf.License.Checked == "" {
		t.Errorf("promptfoo entry = kind %q license %+v", pf.Kind, pf.License)
	}
	// Not a Go module → NO modules claim (the claude/srt precedent), and no
	// npm-side modules mechanics are invented before B6.
	if len(pf.Modules) != 0 {
		t.Errorf("promptfoo must carry no modules claim: %v", pf.Modules)
	}
	if !strings.Contains(pf.Replacement, "DeepEval") {
		t.Error("promptfoo's funeral plan must name its pre-registered swap")
	}
	if !strings.Contains(pf.Notes, "#10 operator ratification PENDING") {
		t.Error("the S16.4 #10 ratification must be marked gate-pending")
	}
	for i := 1; i <= 9; i++ {
		if !strings.Contains(pf.Notes, "#"+string(rune('0'+i))) {
			t.Errorf("the S16.4 walk misses item #%d", i)
		}
	}
	if de == nil {
		t.Fatal("components.lock has no DeepEval standby entry (S16.2 standby rule)")
	}
	if de.Kind != "standby" || de.License.SPDX != "Apache-2.0" || de.Pin != "4.1.3" {
		t.Errorf("DeepEval standby = kind %q pin %q license %+v", de.Kind, de.Pin, de.License)
	}
	if !strings.Contains(de.Notes, "PYTHON") {
		t.Error("the standby entry must be honest that the swap crosses ecosystems")
	}
}

func TestPromptfooRealBinaryMatchesPin(t *testing.T) {
	// Tier R: auto-runs when promptfoo is installed on the host, sanctioned
	// skip otherwise. The packet installs nothing — the exact-version
	// `npm install -g promptfoo@<pin>` is a B5-gate HOST act.
	t.Setenv(evals.PromptfooPathEnv, "")
	if _, err := exec.LookPath("promptfoo"); err != nil {
		t.Skip("SANCTIONED SKIP (CONVENTIONS §10): promptfoo is not installed on this host")
	}
	r, err := evals.FindPromptfoo()
	if err != nil {
		t.Fatal(err)
	}
	_, version, err := r.Identity(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if version != evals.PromptfooPin {
		t.Errorf("installed promptfoo %s != pin %s — a pin↔installed delta is reported LOUDLY and never silently retargeted (CONVENTIONS §10)",
			version, evals.PromptfooPin)
	}
}
