package stage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/worker"
)

// local.go — the S12.1 class-(b) duty-seam adapters (brief R20/R21). The
// low-level duty caller (internal/local) resolves the alias over ⚙ + the
// manifest, shapes the /v1 call (temp-0, engine-enforced json_schema,
// logprobs), and writes the ONE $0 D7 row on the consuming run. These thin
// adapters build the concrete intake/worker duty schema + prompt, call the
// caller on the consuming run, and map the result back to the seam type —
// exactly as router.go bridges worker→intake (B3-3), which is why
// internal/local imports neither intake nor worker (the import wall, R24).
//
// Degradation (R17): the caller's errors (stack absent/unhealthy, alias
// unknown, admissions stopped) surface here and each seam degrades EXACTLY per
// its S12.4/S06 row — classifier fails closed (high tier, the pipeline's job),
// utility falls back to deterministic text, spot-check is skipped, tie-break
// takes the deterministic order — never faked.

// Per-duty length caps — structural constants (S18 ratifies no key; the §7
// sseBatchSize precedent, settings-tab flag, §8 reading 7).
const (
	triageMaxTokens    = 512
	helpMaxTokens      = 700
	spotCheckMaxTokens = 400
	tieBreakMaxTokens  = 200
)

// triage vocabularies — intake's family/tier vocabularies rendered as the
// json_schema enum values (local owns the schema SHAPE, stage the values).
func triageFamilies() []string {
	return []string{
		string(intake.FamilySoftware), string(intake.FamilyResearch),
		string(intake.FamilyContent), string(intake.FamilyData),
		string(intake.FamilyChore), string(intake.FamilyGeneric),
	}
}

func triageTiers() []string {
	return []string{string(intake.TierTrivial), string(intake.TierLow), string(intake.TierStandard), string(intake.TierHigh)}
}

// triageSizes is a compact size-class enum (intake stores SizeClass as a free
// string; the classifier sizes, S10 prices — the estimate stays Known=false).
var triageSizes = []string{"trivial", "small", "medium", "large", "xlarge"}

// triageLabelFields are the routing-decisive intake-triage labels the S12.5
// margin is taken over (R4): family + stakes drive routing; size is a soft
// estimate. The least-confident of these is the case margin.
var triageLabelFields = []string{"family", "stakes"}

// ---- Classifier (intake-triage alias) ----

type localClassifier struct {
	duty    *local.Duty
	recheck *local.ReChecker // R4: low-margin workhorse re-check (nil ⇒ uncalibrated posture, no gate)
}

var _ intake.Classifier = (*localClassifier)(nil)

// NewLocalClassifier wraps the duty caller as the intake Classifier seam. The
// re-check is uncalibrated (no gate) — use NewLocalClassifierWithRecheck to
// wire the S12.5 low-margin workhorse re-check (R4).
func NewLocalClassifier(duty *local.Duty) intake.Classifier {
	return &localClassifier{duty: duty}
}

// NewLocalClassifierWithRecheck wires the classifier with the S12.5 low-margin
// re-check consumer (R4): a below-threshold fast-tier triage re-checks on the
// workhorse ($0, local→local) when a calibrated threshold exists.
func NewLocalClassifierWithRecheck(duty *local.Duty, recheck *local.ReChecker) intake.Classifier {
	return &localClassifier{duty: duty, recheck: recheck}
}

func (c *localClassifier) Classify(ctx context.Context, req intake.Request, reg *intake.RegistrySlice) (intake.TriageProposal, error) {
	schema := local.TriageSchema(triageFamilies(), triageTiers(), triageSizes)
	runID := req.TaskID + RunSuffixIntake
	in := local.DutyRequest{
		Alias:          local.AliasIntakeTriage,
		System:         "You are a task triage classifier for a personal automation platform. Output ONLY the JSON matching the schema — put your brief reasoning in the leading \"reason\" field, then the labels (free-text-then-constrained). Set abstain=true if you cannot classify confidently — never guess a label.",
		User:           triagePrompt(req, reg, schema),
		Schema:         schema,
		Name:           "intake-triage",
		MaxTokens:      triageMaxTokens,
		Classification: true,
	}
	res, err := c.duty.Call(ctx, runID, in)
	if err != nil {
		// Fail closed: the pipeline treats a classify error as high-stakes,
		// unknown estimate, no band membership (S06.2). Never faked.
		return intake.TriageProposal{}, err
	}
	// R4: low-margin re-check on the workhorse ($0, local→local). Uncalibrated
	// ⇒ no gate (the fast answer stands). The re-check never hard-fails the
	// classification — a failed re-check leaves the fast answer.
	if c.recheck != nil {
		if rc, rerr := c.recheck.MarginRecheck(ctx, runID, local.AliasIntakeTriage, in, res, triageLabelFields); rerr == nil {
			res = rc.Result
		}
	}
	return parseTriage(res.Content)
}

func triagePrompt(req intake.Request, reg *intake.RegistrySlice, schema json.RawMessage) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Request title: %s\n\nRequest text:\n%s\n", req.Title, req.Text)
	if reg != nil && reg.Project != "" {
		fmt.Fprintf(&b, "\nMatched project: %s (conventions/commands are known to the platform).\n", reg.Project)
	}
	fmt.Fprintf(&b, "\nClassify family, stakes, size, and whether the task rests on a live fact (data_bearing). Respond with JSON matching this schema:\n%s\n", schema)
	return b.String()
}

func parseTriage(content string) (intake.TriageProposal, error) {
	// Fields mirror the TriageSchema exactly (F12: no unreachable fields —
	// band inputs ReadOnly/NewNeeds are the pipeline's deterministic layer's,
	// not requested from the classifier). `reason` is the leading free-text
	// region (F5); it is not consumed into the proposal.
	var out struct {
		Family      string `json:"family"`
		Stakes      string `json:"stakes"`
		Size        string `json:"size"`
		DataBearing bool   `json:"data_bearing"`
		Abstain     bool   `json:"abstain"`
	}
	if err := json.Unmarshal([]byte(content), &out); err != nil {
		return intake.TriageProposal{}, fmt.Errorf("stage: decode intake-triage output: %w", err)
	}
	if out.Abstain {
		// The model could not classify — fail closed to high (S06.2).
		return intake.TriageProposal{}, fmt.Errorf("stage: intake-triage abstained (S12.4); pipeline fails closed to high (S06.2)")
	}
	fam := intake.Family(out.Family)
	if !validFamily(fam) {
		fam = intake.FamilyGeneric
	}
	tier := intake.Tier(out.Stakes)
	if !intake.ValidTier(tier) {
		tier = intake.TierHigh // conservative
	}
	p := intake.TriageProposal{
		Family: fam,
		Tier:   tier,
		Est:    intake.Estimate{SizeClass: out.Size, Known: false, Basis: "local intake-triage size class (S10 prices)"},
	}
	if out.DataBearing {
		// The classifier may only ADD the data-bearing flag (S06.2 rule 4);
		// the pipeline enforces add-only.
		p.DataHits = []intake.TriggerHit{{RuleID: "classifier", Class: "data_bearing", Cue: "classifier flag", Source: "classifier"}}
	}
	return p, nil
}

func validFamily(f intake.Family) bool {
	switch f {
	case intake.FamilySoftware, intake.FamilyResearch, intake.FamilyContent,
		intake.FamilyData, intake.FamilyChore, intake.FamilyGeneric:
		return true
	}
	return false
}

// ---- Utility (utility alias, drafting) ----

type localUtility struct{ duty *local.Duty }

var _ intake.Utility = (*localUtility)(nil)

// NewLocalUtility wraps the duty caller as the intake Utility seam (optional;
// a caller error degrades to the pipeline's deterministic help text).
func NewLocalUtility(duty *local.Duty) intake.Utility { return &localUtility{duty} }

func (u *localUtility) Help(ctx context.Context, pair intake.Pair) (intake.HelpBlock, error) {
	schema := local.HelpSchema()
	res, err := u.duty.Call(ctx, pair.Spec.TaskID+RunSuffixIntake, local.DutyRequest{
		Alias:          local.AliasUtility,
		System:         "You draft plain-language help for a non-technical reader. Be concrete and brief.",
		User:           helpPrompt(pair, schema),
		Schema:         schema,
		Name:           "utility-help",
		MaxTokens:      helpMaxTokens,
		Classification: false, // drafting, not classification — no forced-label abstain
	})
	if err != nil {
		return intake.HelpBlock{}, err // pipeline falls back to deterministic text
	}
	var out struct{ What, Wrong, Recommend string }
	if err := json.Unmarshal([]byte(res.Content), &out); err != nil {
		return intake.HelpBlock{}, fmt.Errorf("stage: decode utility help output: %w", err)
	}
	return intake.HelpBlock{What: out.What, Wrong: out.Wrong, Recommend: out.Recommend}, nil
}

func helpPrompt(pair intake.Pair, schema json.RawMessage) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Goal: %s\n", pair.Spec.Restatement)
	if len(pair.Spec.Outcome) > 0 {
		fmt.Fprintf(&b, "Outcomes: %s\n", strings.Join(pair.Spec.Outcome, "; "))
	}
	fmt.Fprintf(&b, "The plan has %d step(s).\n", len(pair.Plan.Steps))
	fmt.Fprintf(&b, "\nDraft: what this decision does, what could go wrong, what you recommend and why. Respond with JSON matching this schema:\n%s\n", schema)
	return b.String()
}

// ---- SpotCheck (utility alias — R20; advisory coverage) ----

type localSpotCheck struct{ duty *local.Duty }

var _ intake.SpotCheck = (*localSpotCheck)(nil)

// NewLocalSpotCheck wraps the duty caller as the intake SpotCheck seam. It
// rides the `intake-triage` alias (drain F8): S12.4's registry lists the
// advisory coverage spot-check in the intake-triage row (the fast seat); the
// utility row omits it — the spec wins over the brief's R20 mis-citation.
func NewLocalSpotCheck(duty *local.Duty) intake.SpotCheck { return &localSpotCheck{duty} }

func (s *localSpotCheck) Check(ctx context.Context, pair intake.Pair) ([]string, error) {
	schema := local.SpotCheckSchema()
	res, err := s.duty.Call(ctx, pair.Spec.TaskID+RunSuffixIntake, local.DutyRequest{
		Alias:          local.AliasIntakeTriage,
		System:         "You perform an advisory semantic coverage check. Put brief reasoning in the leading \"reason\" field, then list in \"uncovered\" the acceptance-criteria labels the plan appears NOT to cover. Advisory only — never a gate.",
		User:           spotCheckPrompt(pair, schema),
		Schema:         schema,
		Name:           "coverage-spotcheck",
		MaxTokens:      spotCheckMaxTokens,
		Classification: true,
	})
	if err != nil {
		return nil, err // optional duty: the pipeline skips it, never faked
	}
	var out struct {
		Uncovered []string `json:"uncovered"`
		Abstain   bool     `json:"abstain"`
	}
	if err := json.Unmarshal([]byte(res.Content), &out); err != nil {
		return nil, fmt.Errorf("stage: decode coverage spot-check output: %w", err)
	}
	if out.Abstain {
		return nil, nil // advisory skip
	}
	return out.Uncovered, nil
}

func spotCheckPrompt(pair intake.Pair, schema json.RawMessage) string {
	var b strings.Builder
	b.WriteString("Acceptance criteria:\n")
	for _, ac := range pair.Spec.ACs {
		covered := len(pair.Plan.Coverage[ac.Key()]) > 0
		fmt.Fprintf(&b, "- %s: %s [plan lists coverage: %v]\n", ac.Key(), ac.Plain, covered)
	}
	b.WriteString("\nPlan steps:\n")
	for i, st := range pair.Plan.Steps {
		fmt.Fprintf(&b, "%d. %s\n", i+1, st.Title)
	}
	fmt.Fprintf(&b, "\nList the AC labels the plan appears NOT to cover. Respond with JSON matching this schema:\n%s\n", schema)
	return b.String()
}

// ---- TieBreaker (utility alias — §8 reading 2) ----

type localTieBreaker struct{ duty *local.Duty }

var _ worker.TieBreaker = (*localTieBreaker)(nil)

// NewLocalTieBreaker wraps the duty caller as the S08.8 step-2 tie-break seam
// on the utility alias (§8 reading 2 — flagged). The pick is ADVISORY input to
// a human-visible card (S08.8 re-route/pin); "this seat never decides" holds.
func NewLocalTieBreaker(duty *local.Duty) worker.TieBreaker { return &localTieBreaker{duty} }

func (t *localTieBreaker) Break(ctx context.Context, q worker.RouteQuery, candidates []worker.Candidate) (int, string, error) {
	ids := make([]string, len(candidates))
	for i, c := range candidates {
		ids[i] = c.TemplateID
	}
	schema := local.TieBreakSchema(ids)
	// The consuming run is the caller's (drain F2): intake-time routing rides
	// <task>.intake; helper-spawn routing rides the coordinator's execute run
	// (q.RunID), which may differ from the now-terminal intake run.
	runID := q.RunID
	if runID == "" {
		runID = q.TaskID + RunSuffixIntake
	}
	res, err := t.duty.Call(ctx, runID, local.DutyRequest{
		Alias:          local.AliasUtility,
		System:         "You pick the best-fit worker for a task, or abstain. Put your one-line reasoning in the leading \"reason\" field, then the pick. Advisory — a human confirms and may re-route.",
		User:           tieBreakPrompt(q, candidates, schema),
		Schema:         schema,
		Name:           "tie-break",
		MaxTokens:      tieBreakMaxTokens,
		Classification: true,
	})
	// abstain OR any error → the deterministic degraded order (pick 0) with the
	// reason recorded (R21) — never a route failure. The duty caller already
	// surfaced any D7-metering defect loudly (R18); the tie-break, being
	// advisory, degrades regardless.
	if err != nil {
		return 0, fmt.Sprintf("tie-break duty unavailable (%s); deterministic order: score, then name", degradeReason(err)), nil
	}
	var out struct{ Pick, Reason string }
	if err := json.Unmarshal([]byte(res.Content), &out); err != nil {
		return 0, "tie-break duty returned unparseable output; deterministic order: score, then name", nil
	}
	if out.Pick == "" || out.Pick == "abstain" {
		return 0, "tie-break duty abstained; deterministic order: score, then name", nil
	}
	for i, c := range candidates {
		if c.TemplateID == out.Pick {
			reason := "local tie-break picked " + c.Name
			if out.Reason != "" {
				reason += ": " + out.Reason
			}
			return i, reason, nil
		}
	}
	return 0, "tie-break pick did not match a candidate; deterministic order: score, then name", nil
}

func tieBreakPrompt(q worker.RouteQuery, candidates []worker.Candidate, schema json.RawMessage) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Task: %s\n\nCandidate workers:\n", q.TaskText)
	for _, c := range candidates {
		fmt.Fprintf(&b, "- id=%s name=%q why=%s\n", c.TemplateID, c.Name, c.Reason)
	}
	fmt.Fprintf(&b, "\nPick the best-fit worker id, or \"abstain\". Respond with JSON matching this schema:\n%s\n", schema)
	return b.String()
}

// degradeReason trims a local-duty error to a one-line degradation reason.
func degradeReason(err error) string {
	s := err.Error()
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}
