// Package battery is the Spec S12.9 T15 acceptance battery for the local tier
// (brief R6–R9): built once at bring-up, versioned in this repo, run against
// the EXACT deployed quant and engine build. It is S12's acceptance instrument
// — the workhorse bakeoff (S12.9 #1), the per-duty confidence calibration
// (S12.9 #6, whose margins ride the same runner), and the input to the S12.10
// swap gate's promotion rule all read its SuiteResults. The runner is
// executor-shaped (a package + the `sinet local` CLI verb — a tool, never a
// mode) and runs standalone NOW, consumable by the S14 pinned eval runner at
// B5 (the seam is noted, no S14 machinery is built here).
//
// Contents (S12.9): ~30–50 labeled cases per registry duty (synthetic seeds +
// real traces as they accumulate) PLUS the quant sanity check (kl.go). Batch
// passes are AC-only: the runner refuses a battery execution when ⚙
// local.batch.ac_only is true and the host is on battery (R7 — the key's first
// consumer; a sleep-safe power-supply observation, never a periodic wake).
//
// Import posture: this package imports internal/local + stdlib only — it drives
// the local client/schemas/margins and produces local.SuiteResult /
// local.LabeledItem, which the swap gate + calibration store consume.
package battery

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
)

// Version versions the committed battery (case data + harness). Bumped when the
// seed suites change.
const Version = "t15-v0"

// Kind marks how a suite is scored.
type Kind string

const (
	// KindClassification: a duty with a label margin (S12.4 classification
	// shape) — scored by label match, and its margins feed calibration (R25).
	KindClassification Kind = "classification"
	// KindOutputContract: a generation-shaped duty (utility, distill) — no
	// label margin; scored by output-contract checks (schema/length/grounding),
	// its quality control is its own S12.4 row (R6).
	KindOutputContract Kind = "output-contract"
)

// Contract is an output-contract check for a generation-shaped case (R6).
type Contract struct {
	// MaxChars caps the output length (0 = no cap).
	MaxChars int `json:"max_chars,omitempty"`
	// MustContainFields are JSON fields the output must carry (schema shape).
	MustContainFields []string `json:"must_contain_fields,omitempty"`
	// Grounding are substrings the output must contain (grounded in the input).
	Grounding []string `json:"grounding,omitempty"`
}

// Case is one labeled battery case.
type Case struct {
	ID     string `json:"id"`
	Prompt string `json:"prompt"`
	// ExpectLabels is the expected label per field (classification cases).
	ExpectLabels map[string]string `json:"expect_labels,omitempty"`
	// ExpectAbstain marks a case whose correct answer IS abstain (a genuinely
	// ambiguous input — proves the model uses the abstain member, S12.4).
	ExpectAbstain bool `json:"expect_abstain,omitempty"`
	// Contract is the output-contract check (generation cases).
	Contract *Contract `json:"contract,omitempty"`
}

// Suite is one duty's labeled case set.
type Suite struct {
	Duty string `json:"duty"`
	Kind Kind   `json:"kind"`
	// Fields are the classification label fields (name + enum) the runner
	// builds its schema from (classification suites).
	Fields []local.LabelField `json:"fields,omitempty"`
	// System is the duty system prompt.
	System string `json:"system"`
	// LabelFields names the routing-decisive labels the margin is taken over
	// (a subset of Fields; the least-confident of these is the case margin, R2).
	LabelFields []string `json:"label_fields,omitempty"`
	Cases       []Case   `json:"cases"`
}

// Validate enforces the suite contract: id/kind present; classification suites
// declare fields + label fields; every case complete for its kind.
func (s *Suite) Validate() error {
	if s.Duty == "" {
		return fmt.Errorf("battery: suite without a duty")
	}
	if len(s.Cases) == 0 {
		return fmt.Errorf("battery: suite %q has no cases", s.Duty)
	}
	seen := map[string]bool{}
	for _, c := range s.Cases {
		if c.ID == "" || c.Prompt == "" {
			return fmt.Errorf("battery: suite %q has an incomplete case", s.Duty)
		}
		if seen[c.ID] {
			return fmt.Errorf("battery: suite %q duplicate case %q", s.Duty, c.ID)
		}
		seen[c.ID] = true
		switch s.Kind {
		case KindClassification:
			if len(c.ExpectLabels) == 0 && !c.ExpectAbstain {
				return fmt.Errorf("battery: classification case %q labels nothing and does not expect abstain", c.ID)
			}
		case KindOutputContract:
			if c.Contract == nil {
				return fmt.Errorf("battery: output-contract case %q carries no contract", c.ID)
			}
		default:
			return fmt.Errorf("battery: suite %q unknown kind %q", s.Duty, s.Kind)
		}
	}
	if s.Kind == KindClassification {
		if len(s.Fields) == 0 {
			return fmt.Errorf("battery: classification suite %q declares no schema fields", s.Duty)
		}
		if len(s.LabelFields) == 0 {
			return fmt.Errorf("battery: classification suite %q declares no label fields (the margin surface)", s.Duty)
		}
	}
	return nil
}

// Completer is the /v1 completion surface the runner drives — *local.Client
// satisfies it (against the live pinned stack, or the shared FakeServer in
// tests). Narrow by design so a suite runs standalone.
type Completer interface {
	Chat(ctx context.Context, spec local.ChatSpec) (local.Completion, error)
}

// Runner runs suites against a target seat, honouring the AC gate (R7).
type Runner struct {
	comp     Completer
	settings local.Settings
	power    local.PowerReader
}

// NewRunner wires a battery runner. power/settings back the AC gate (R7); a nil
// power reader disables the gate (a non-battery host / test).
func NewRunner(comp Completer, settings local.Settings, power local.PowerReader) *Runner {
	return &Runner{comp: comp, settings: settings, power: power}
}

// ErrOnBattery refuses a batch battery run on battery when ⚙ local.batch.ac_only
// is set (R7).
type batteryError struct{ msg string }

func (e batteryError) Error() string { return e.msg }

// ErrOnBattery is returned when the AC gate refuses a run (R7).
var ErrOnBattery = batteryError{"battery: refused — ⚙ local.batch.ac_only is set and the host is on battery (S12.8)"}

// CanRun applies the AC gate (R7): when ⚙ local.batch.ac_only is true and the
// host is on battery, a batch battery execution is refused (a sleep-safe
// power-supply observation, never a periodic wake). No settings/power ⇒ allow.
func (r *Runner) CanRun() (bool, error) {
	if r.settings == nil || r.power == nil {
		return true, nil
	}
	acOnly, err := r.settings.Bool("local.batch.ac_only")
	if err != nil {
		return false, fmt.Errorf("battery: read ⚙ local.batch.ac_only: %w", err)
	}
	if !acOnly {
		return true, nil
	}
	onBat, err := r.power.OnBattery()
	if err != nil {
		return false, fmt.Errorf("battery: read host power state: %w", err)
	}
	return !onBat, nil
}

// Target is the seat a suite runs against — its model id keys the /v1 call, its
// hash + the engine build key the recorded SuiteResult (R6/R9).
type Target struct {
	Model       string
	ModelHash   string
	EngineBuild string
}

// TargetFromSeat builds a Target from a manifest seat + engine build.
func TargetFromSeat(seat local.SeatRecord, engineBuild string) Target {
	return Target{Model: seat.Model, ModelHash: seat.ModelHash(), EngineBuild: engineBuild}
}

// Outcome is a suite run: the SuiteResult (for the swap gate + persistence) and
// the per-case labeled items (margin + correctness — the calibration input,
// R25). Failures carry the per-case detail for the Def.8 file.
type Outcome struct {
	Result  local.SuiteResult   `json:"result"`
	Labeled []local.LabeledItem `json:"labeled"`
	Cases   []CaseResult        `json:"cases"`
}

// CaseResult is one case's outcome.
type CaseResult struct {
	ID     string  `json:"id"`
	Pass   bool    `json:"pass"`
	Margin float64 `json:"margin,omitempty"`
	Detail string  `json:"detail,omitempty"`
}

// RunSuite runs a suite against a target, honouring the AC gate. It returns the
// SuiteResult (pass count, 95% Wilson, tokens/s) + labeled items + per-case
// detail. The AC gate is checked ONCE at the top (a batch pass).
func (r *Runner) RunSuite(ctx context.Context, s *Suite, t Target) (Outcome, error) {
	if err := s.Validate(); err != nil {
		return Outcome{}, err
	}
	ok, err := r.CanRun()
	if err != nil {
		return Outcome{}, err
	}
	if !ok {
		return Outcome{}, ErrOnBattery
	}

	var schema json.RawMessage
	if s.Kind == KindClassification {
		schema = local.BatterySchema(s.Fields)
	}

	var out Outcome
	passed := 0
	var totalOutTokens int64
	start := time.Now()
	for _, c := range s.Cases {
		cr, item, outTok := r.runCase(ctx, s, schema, c, t.Model)
		out.Cases = append(out.Cases, cr)
		totalOutTokens += outTok
		if cr.Pass {
			passed++
		}
		if s.Kind == KindClassification && item != nil {
			out.Labeled = append(out.Labeled, *item)
		}
	}
	elapsed := time.Since(start).Seconds()
	// tokens/s over the suite's total output tokens — a coarse but honest
	// throughput figure for the promotion rule's equal-or-better test (R10).
	tokPerSec := 0.0
	if elapsed > 0 {
		tokPerSec = float64(totalOutTokens) / elapsed
	}
	out.Result = local.NewSuiteResult(s.Duty, t.ModelHash, t.EngineBuild, Version, passed, len(s.Cases), tokPerSec)
	return out, nil
}

// runCase runs one case and scores it. For classification it also returns the
// labeled item (margin + wrong) for calibration (R25). outTok is the case's
// output token count (for the suite tokens/s).
func (r *Runner) runCase(ctx context.Context, s *Suite, schema json.RawMessage, c Case, model string) (CaseResult, *local.LabeledItem, int64) {
	spec := local.ChatSpec{
		Model:     model,
		System:    s.System,
		User:      c.Prompt,
		MaxTokens: 512,
	}
	if s.Kind == KindClassification {
		spec.Schema = schema
		spec.Name = s.Duty
		spec.Logprobs = true
		spec.NoThink = true
	}
	comp, err := r.comp.Chat(ctx, spec)
	if err != nil {
		return CaseResult{ID: c.ID, Pass: false, Detail: "call error: " + err.Error()}, nil, 0
	}
	if s.Kind == KindOutputContract {
		pass, detail := checkContract(comp.Content, c.Contract)
		return CaseResult{ID: c.ID, Pass: pass, Detail: detail}, nil, comp.OutputTokens
	}
	pass, detail := checkClassification(comp.Content, c)
	margin, hasMargin := local.LabelMargin(comp.Logprobs, s.LabelFields)
	cr := CaseResult{ID: c.ID, Pass: pass, Detail: detail}
	if hasMargin {
		cr.Margin = margin
		return cr, &local.LabeledItem{Margin: margin, Wrong: !pass}, comp.OutputTokens
	}
	return cr, nil, comp.OutputTokens
}

// checkClassification scores a classification case: the parsed JSON must match
// every expected label (or the abstain member when the case expects abstain).
func checkClassification(content string, c Case) (bool, string) {
	var out map[string]any
	if err := json.Unmarshal([]byte(content), &out); err != nil {
		return false, "unparseable output: " + truncate(content, 80)
	}
	if c.ExpectAbstain {
		if b, _ := out["abstain"].(bool); b {
			return true, ""
		}
		return false, "expected abstain, got a label"
	}
	if b, _ := out["abstain"].(bool); b {
		return false, "abstained on a labeled case"
	}
	for field, want := range c.ExpectLabels {
		got, _ := out[field].(string)
		if !strings.EqualFold(strings.TrimSpace(got), want) {
			return false, fmt.Sprintf("field %q = %q, want %q", field, got, want)
		}
	}
	return true, ""
}

// checkContract scores an output-contract case (R6).
func checkContract(content string, ct *Contract) (bool, string) {
	if ct == nil {
		return true, ""
	}
	if ct.MaxChars > 0 && len(content) > ct.MaxChars {
		return false, fmt.Sprintf("output %d chars > cap %d", len(content), ct.MaxChars)
	}
	if len(ct.MustContainFields) > 0 {
		var out map[string]any
		if err := json.Unmarshal([]byte(content), &out); err != nil {
			return false, "output-contract expects JSON, got unparseable content"
		}
		for _, f := range ct.MustContainFields {
			if _, ok := out[f]; !ok {
				return false, "missing required field " + f
			}
		}
	}
	for _, g := range ct.Grounding {
		if !strings.Contains(content, g) {
			return false, "output not grounded — missing " + truncate(g, 40)
		}
	}
	return true, ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
