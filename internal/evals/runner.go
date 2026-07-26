package evals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// The pinned eval runner seam (Spec S14.8 ¶1; S01.3 adoption seam) and its
// adopted promptfoo adapter.
//
// promptfoo is consumed as an UNMODIFIED external Node CLI (adopt-don't-fork,
// S16.1): the adapter generates a config, runs the process, and parses its
// output — no fork, no patch, no Go module (OQ1(a), the sandbox-runtime
// precedent). It is a RUNNER, NEVER A STORE (P-T11-4): every score it produces
// lands in platform.db as eval.score_recorded, its telemetry is disabled at
// invocation, sharing is refused on the command line, and its own on-disk eval
// history is redirected to scratch under the run's temp dir and never read back
// as a record.
//
// B5-6's behavioral canaries reuse this seam; nothing is built for them here.

// PromptfooPin is the adopted pin, coupled to the components.lock entry by test
// (the TestPinMatchesLock pattern, CONVENTIONS §10).
const PromptfooPin = "0.121.19"

// PromptfooPathEnv overrides binary discovery — the operator/activation knob
// and the hermetic test-injection seam (the SINET_SRT_PATH precedent).
const PromptfooPathEnv = "SINET_PROMPTFOO_PATH"

// promptfooBinary is the installed command name (npm bin `promptfoo`).
const promptfooBinary = "promptfoo"

// promptfooExitTestFailure is the documented exit code for "at least one test
// case failed" — a legitimate red run, not an invocation error (exit 1 is the
// error case). Recorded from the pinned version's CLI reference.
const promptfooExitTestFailure = 100

// promptfooResultsVersion is the results-file schema version the parser
// accepts, recorded from the pinned ref (examples/simple-cli/output.json at
// tag 0.121.19).
const promptfooResultsVersion = 3

// ErrRunnerAbsent: no promptfoo binary is installed and no override points at
// one. Callers turn this into the CONVENTIONS §10 sanctioned skip.
var ErrRunnerAbsent = errors.New("evals: promptfoo binary not found (PATH or " + PromptfooPathEnv + ")")

// Case is one runnable eval case: a prompt plus the graded expectations a
// competent answer satisfies.
type Case struct {
	ID     string
	Prompt string
	// Expect are substrings a passing answer contains.
	Expect []string
	// Forbid are substrings a passing answer does NOT contain.
	Forbid []string
}

// RunConfig is one runner invocation: a named suite of cases against one
// provider identity the runner resolves.
type RunConfig struct {
	Suite    string
	Provider string
	Cases    []Case
	// Env are extra environment entries the provider needs. The child env is
	// built explicitly (deny-by-default), never inherited wholesale.
	Env []string
}

// CaseOutcome is one case's graded result.
type CaseOutcome struct {
	ID     string
	Pass   bool
	Score  float64
	Reason string
}

// RunOutcome is one runner invocation's parsed result.
type RunOutcome struct {
	Suite     string
	Provider  string
	Cases     []CaseOutcome
	Successes int
	Failures  int
	Errors    int
}

// PassRate is the fraction of cases that passed; 0 for an empty run.
func (o RunOutcome) PassRate() float64 {
	if len(o.Cases) == 0 {
		return 0
	}
	passed := 0
	for _, c := range o.Cases {
		if c.Pass {
			passed++
		}
	}
	return float64(passed) / float64(len(o.Cases))
}

// Runner executes eval cases against a provider. The promptfoo adapter is the
// v0 implementation; a swap to the pre-registered DeepEval replacement replaces
// exactly this adapter, because every record already lives in platform.db.
type Runner interface {
	// Identity returns the runner name and its version, recorded on every
	// result (Spec S14.2 family 14).
	Identity(ctx context.Context) (name, version string, err error)
	// Run executes cfg and returns the parsed outcome.
	Run(ctx context.Context, cfg RunConfig) (RunOutcome, error)
}

// Promptfoo is the adopted runner adapter.
type Promptfoo struct {
	// Path is the resolved binary; FindPromptfoo fills it.
	Path string
	// WorkDir is the parent of the per-run scratch dir; "" ⇒ os.TempDir.
	WorkDir string
}

var _ Runner = (*Promptfoo)(nil)

// FindPromptfoo resolves the promptfoo binary: the explicit override first,
// then PATH. It returns ErrRunnerAbsent when neither yields one.
func FindPromptfoo() (*Promptfoo, error) {
	if p := os.Getenv(PromptfooPathEnv); p != "" {
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("%w: %s=%q: %w", ErrRunnerAbsent, PromptfooPathEnv, p, err)
		}
		return &Promptfoo{Path: p}, nil
	}
	p, err := exec.LookPath(promptfooBinary)
	if err != nil {
		return nil, ErrRunnerAbsent
	}
	return &Promptfoo{Path: p}, nil
}

// versionPattern extracts the version from `promptfoo --version` output
// tolerantly (the printed form is not a behavioral contract).
var versionPattern = regexp.MustCompile(`\d+\.\d+\.\d+`)

// Identity reports the runner name and the INSTALLED version — the version that
// actually ran, never the pin assumed from the manifest. It runs under the same
// scratch isolation as Run, so even a version probe cannot leave runner state
// on the host.
func (p *Promptfoo) Identity(ctx context.Context) (string, string, error) {
	scratch, err := p.scratchDir()
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(scratch)
	cmd := exec.CommandContext(ctx, p.Path, "--version")
	cmd.Env = p.env(scratch, nil)
	out, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("evals: promptfoo --version: %w", err)
	}
	v := versionPattern.FindString(string(out))
	if v == "" {
		return "", "", fmt.Errorf("evals: promptfoo --version printed no version: %q", strings.TrimSpace(string(out)))
	}
	return RunnerPromptfoo, v, nil
}

// Run generates a config, invokes promptfoo, and parses the results file. The
// process runs entirely inside a scratch dir: config, cache, and promptfoo's
// own eval history live there and die with the run.
func (p *Promptfoo) Run(ctx context.Context, cfg RunConfig) (RunOutcome, error) {
	scratch, err := p.scratchDir()
	if err != nil {
		return RunOutcome{}, err
	}
	defer os.RemoveAll(scratch)

	// The config is emitted as JSON into promptfoo's default .yaml config name:
	// JSON is a subset of YAML, so the pinned loader parses it, and Sinet stays
	// stdlib-only (no YAML adoption for a generated file).
	confPath := filepath.Join(scratch, "promptfooconfig.yaml")
	body, err := json.MarshalIndent(promptfooConfig(cfg), "", "  ")
	if err != nil {
		return RunOutcome{}, fmt.Errorf("evals: marshal promptfoo config: %w", err)
	}
	if err := os.WriteFile(confPath, body, 0o600); err != nil {
		return RunOutcome{}, fmt.Errorf("evals: write promptfoo config: %w", err)
	}
	outPath := filepath.Join(scratch, "results.json")

	cmd := exec.CommandContext(ctx, p.Path, "eval",
		"-c", confPath, "-o", outPath,
		// No shareable URL, no progress bar, no results table: self-hosted
		// only, and stdout stays quiet because the results FILE is the record.
		"--no-share", "--no-progress-bar", "--no-table")
	cmd.Env = p.env(scratch, cfg.Env)
	cmd.Dir = scratch
	runErr := cmd.Run()
	if runErr != nil {
		var exitErr *exec.ExitError
		// Exit 100 means "tests failed" — the results file is still written and
		// a red run is a legitimate outcome, not an invocation failure.
		if !errors.As(runErr, &exitErr) || exitErr.ExitCode() != promptfooExitTestFailure {
			return RunOutcome{}, fmt.Errorf("evals: promptfoo eval: %w", runErr)
		}
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		return RunOutcome{}, fmt.Errorf("evals: read promptfoo results: %w", err)
	}
	return parsePromptfooResults(cfg, raw)
}

// scratchDir creates the per-invocation dir holding everything the runner
// writes — the generated config, its disk cache, and promptfoo's own eval
// history. Callers remove it when the invocation ends, so nothing the runner
// stores can be read back as a record.
func (p *Promptfoo) scratchDir() (string, error) {
	dir, err := os.MkdirTemp(p.WorkDir, "sinet-promptfoo-")
	if err != nil {
		return "", fmt.Errorf("evals: promptfoo scratch dir: %w", err)
	}
	return dir, nil
}

// env builds the child environment explicitly (deny-by-default, the S11.1
// posture): the caller's provider entries, then the hardening block. The
// hardening block is applied LAST and any caller entry that collides with one
// of its keys is DROPPED — telemetry-off, update-checks-off and the scratch
// redirection are S16.4 adoption-walk invariants of this entry, not caller
// conventions, so no RunConfig can weaken them.
func (p *Promptfoo) env(scratch string, extra []string) []string {
	hardened := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"PROMPTFOO_DISABLE_TELEMETRY=1",
		"PROMPTFOO_DISABLE_UPDATE=1",
		"NO_COLOR=1",
		"PROMPTFOO_CONFIG_DIR=" + filepath.Join(scratch, "state"),
		"PROMPTFOO_CACHE_PATH=" + filepath.Join(scratch, "cache"),
	}
	reserved := make(map[string]bool, len(hardened))
	for _, e := range hardened {
		key, _, _ := strings.Cut(e, "=")
		reserved[key] = true
	}
	env := make([]string, 0, len(extra)+len(hardened))
	for _, e := range extra {
		if key, _, ok := strings.Cut(e, "="); ok && reserved[key] {
			continue
		}
		env = append(env, e)
	}
	return append(env, hardened...)
}

// promptfooConfig renders a RunConfig as promptfoo's config schema at the
// pinned version: one prompt template driven by a per-case var, with the
// expectations as contains / not-contains assertions.
func promptfooConfig(cfg RunConfig) map[string]any {
	tests := make([]map[string]any, 0, len(cfg.Cases))
	for _, c := range cfg.Cases {
		asserts := make([]map[string]any, 0, len(c.Expect)+len(c.Forbid))
		for _, e := range c.Expect {
			asserts = append(asserts, map[string]any{"type": "icontains", "value": e})
		}
		for _, f := range c.Forbid {
			asserts = append(asserts, map[string]any{"type": "not-icontains", "value": f})
		}
		tests = append(tests, map[string]any{
			"vars":     map[string]any{"task": c.Prompt},
			"metadata": map[string]any{"case_id": c.ID},
			"assert":   asserts,
		})
	}
	return map[string]any{
		"description": cfg.Suite,
		"providers":   []string{cfg.Provider},
		"prompts":     []string{"{{task}}"},
		"tests":       tests,
	}
}

// promptfooResults is the parsed subset of the results file. The full document
// carries much more; only these fields are contract for us, so an upstream
// addition never breaks the parse (the S03.1 forward-tolerance posture).
type promptfooResults struct {
	Results struct {
		Version int `json:"version"`
		Results []struct {
			Success  bool    `json:"success"`
			Score    float64 `json:"score"`
			TestCase struct {
				Metadata struct {
					CaseID string `json:"case_id"`
				} `json:"metadata"`
			} `json:"testCase"`
			GradingResult struct {
				Reason string `json:"reason"`
			} `json:"gradingResult"`
		} `json:"results"`
		Stats struct {
			Successes int `json:"successes"`
			Failures  int `json:"failures"`
			Errors    int `json:"errors"`
		} `json:"stats"`
	} `json:"results"`
}

// parsePromptfooResults maps the results file onto a RunOutcome.
func parsePromptfooResults(cfg RunConfig, raw []byte) (RunOutcome, error) {
	var doc promptfooResults
	if err := json.Unmarshal(raw, &doc); err != nil {
		return RunOutcome{}, fmt.Errorf("evals: parse promptfoo results: %w", err)
	}
	if doc.Results.Version != promptfooResultsVersion {
		return RunOutcome{}, fmt.Errorf("evals: promptfoo results schema version %d, expected %d (re-verify the parser on a pin bump)",
			doc.Results.Version, promptfooResultsVersion)
	}
	out := RunOutcome{
		Suite: cfg.Suite, Provider: cfg.Provider,
		Successes: doc.Results.Stats.Successes,
		Failures:  doc.Results.Stats.Failures,
		Errors:    doc.Results.Stats.Errors,
	}
	for _, r := range doc.Results.Results {
		out.Cases = append(out.Cases, CaseOutcome{
			ID: r.TestCase.Metadata.CaseID, Pass: r.Success, Score: r.Score,
			Reason: r.GradingResult.Reason,
		})
	}
	return out, nil
}
