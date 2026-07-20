package stage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/verify"
)

// Engine-session implementations of the pipeline model seams (Spec S06.10
// duty table; Spec S07.5–S07.6 judge/rework wiring). Each is a prompt
// contract + strict parse over ONE fresh stage session — trust stays in
// the platform validation (the intake spine, the judge validators), never
// in the seam (Spec S06.6). Sessions are tool-less (`--tools ""`): the
// duty is to REASON and emit a structured result, and the S06.6 C1
// read-only posture composes when confinement is active.

// Stage marker kinds (the prompt-frame duty header, runner.go).
const (
	markerPlanDraft  = "plan-draft"
	markerPlanRevise = "plan-revise"
	markerCritique   = "critique"
	markerExecute    = "execute"
	markerCompliance = "judge-compliance"
	markerSanity     = "judge-sanity"
	markerRevise     = "revise"
)

// parseJSONOutput parses a session's structured output: the final message
// must be one JSON object (optionally fenced). A parse failure is a seam
// error the pipeline surfaces — never silently absorbed.
func parseJSONOutput(text string, into any) error {
	t := strings.TrimSpace(text)
	if i := strings.Index(t, "```"); i >= 0 {
		t = t[i+3:]
		t = strings.TrimPrefix(t, "json")
		if j := strings.LastIndex(t, "```"); j >= 0 {
			t = t[:j]
		}
	}
	t = strings.TrimSpace(t)
	if i := strings.IndexByte(t, '{'); i > 0 {
		t = t[i:]
	}
	if err := json.Unmarshal([]byte(t), into); err != nil {
		return fmt.Errorf("stage: session output is not the contracted JSON object: %w", err)
	}
	return nil
}

// ---- intake.Planner (Spec S06.10: planning-model duty; wired B2-4) ----

// EnginePlanner drives the Stage-1 drafting/revision sessions on the
// task's intake run. The session input is the ledger-assembled stage brief
// plus the DraftInput serialized as a manifested Extra item (Spec S05.4:
// every injected item logged); the pipeline validates whatever comes back.
type EnginePlanner struct{ s *Skeleton }

var _ intake.Planner = (*EnginePlanner)(nil)

const pairSchema = `Output EXACTLY one JSON object, nothing else, shaped:
{"spec":{"restatement":string, "outcome":[string...],
  "acs":[{"n":1,"plain":string,"structured":string?,"structured_kind":"ears"|"gwt"|""}...],
  "constraints":[string...], "assumptions":[{"text":string,"origin":string}...],
  "out_of_scope":[string...], "clarifications":[string...]},
 "plan":{"steps":[{"id":"S-1","title":string,"done_when":string,"class":"C1"|"C2",
   "write_set":[glob...], "outward_effects":[string...], "new_spend":bool,
   "credential_touch":bool, "shared_asset_write":bool, "research":bool}...],
  "coverage":{"AC-1":["S-1"],...}, "research_nodes":[{"rule_id":string,"step_id":"S-n","query":string}...],
  "risks":[string...], "est":{"size_class":string,"usd":number,"known":bool,"basis":string}}}
Rules: acceptance criteria numbered 1..n contiguously, plain phrasing on every
criterion; steps keyed S-1..n, each with a testable done_when and a confinement
class; EVERY AC key appears in coverage; a research node per open data-bearing
flag; unresolved consequential ambiguities become NEEDS-CLARIFICATION entries
in clarifications (they block approval); prefer the smallest honest plan.`

func (p *EnginePlanner) Draft(ctx context.Context, in intake.DraftInput) (intake.Pair, error) {
	extra, err := draftInputItem(in)
	if err != nil {
		return intake.Pair{}, err
	}
	instructions := stageMarker(markerPlanDraft) + fmt.Sprintf(
		"You are the planning model for task %s (family %s, tier %s): conduct Stage-1 drafting (Spec S06.6).\n"+
			"Read the interview record in the %s block above; honor registry-supplied facts and recorded assumptions.\n"+
			"Emit SPEC version %d and PLAN version %d (plan.spec_version %d).\n%s",
		in.Request.TaskID, in.Family, in.Tier, itemDraftInput, in.SpecVersion, in.PlanVersion, in.SpecVersion, pairSchema)
	return p.pairSession(ctx, in.Request.TaskID, in.Request.UserID, in.Tier, extra, instructions,
		in.SpecVersion, in.PlanVersion)
}

func (p *EnginePlanner) Revise(ctx context.Context, in intake.ReviseInput) (intake.Pair, error) {
	extra, err := reviseInputItem(in)
	if err != nil {
		return intake.Pair{}, err
	}
	instructions := stageMarker(markerPlanRevise) + fmt.Sprintf(
		"You are the planning model revising the SPEC/PLAN pair for task %s (revision reason: %s).\n"+
			"Address EXACTLY the numbered findings/resolutions in the %s block above — criteria do not drift (Spec S06.7(a)/S06.8).\n"+
			"Emit SPEC version %d and PLAN version %d (plan.spec_version %d).\n%s",
		in.Pair.Spec.TaskID, in.Reason, itemReviseInput, in.SpecVersion, in.PlanVersion, in.SpecVersion, pairSchema)
	return p.pairSession(ctx, in.Pair.Spec.TaskID, in.Pair.Spec.Owner, in.Pair.Spec.Tier, extra, instructions,
		in.SpecVersion, in.PlanVersion)
}

func (p *EnginePlanner) pairSession(ctx context.Context, taskID, owner string, tier intake.Tier,
	extra ledger.Item, instructions string, specV, planV int) (intake.Pair, error) {
	res, err := p.s.Session(ctx, SessionInput{
		RunID:        taskID + RunSuffixIntake,
		Stage:        "plan",
		Assemble:     true,
		Sources:      ledger.Sources{}, // knowledge/conventions/worker sources are B3/B4 seams
		Extra:        []ledger.Item{extra},
		Instructions: instructions,
		Class:        "C1", // read-only planning sandbox (Spec S06.6, P-T05-1)
	})
	if err != nil {
		return intake.Pair{}, fmt.Errorf("planner session: %w", err)
	}
	var pair intake.Pair
	if err := parseJSONOutput(res.Text, &pair); err != nil {
		return intake.Pair{}, fmt.Errorf("planner output: %w", err)
	}
	// Platform-owned bookkeeping fields are stamped here, not trusted from
	// the engine (the spine re-validates content; identity/version
	// bookkeeping is the platform's).
	pair.Spec.TaskID, pair.Plan.TaskID = taskID, taskID
	pair.Spec.Owner, pair.Plan.Owner = owner, owner
	pair.Spec.Version, pair.Plan.Version = specV, planV
	pair.Plan.SpecVersion = specV
	pair.Spec.Status, pair.Plan.Status = intake.StatusDraft, intake.StatusDraft
	pair.Spec.Tier, pair.Plan.Tier = tier, tier
	prov := "planning-model session (" + p.s.modelFor("") + ")"
	pair.Spec.Provenance, pair.Plan.Provenance = prov, prov
	if err := pair.Validate(); err != nil {
		return intake.Pair{}, fmt.Errorf("planner output: %w", err)
	}
	return pair, nil
}

// Draft-input item identities on the assembly manifest (Spec S05.4).
const (
	itemDraftInput  = "intake/draft-input"
	itemReviseInput = "intake/revise-input"

	selectorIntake = "intake pipeline stage input (S06.5/S06.6) — interview record / revision findings"
)

func draftInputItem(in intake.DraftInput) (ledger.Item, error) {
	// The serialized interview record: everything Stage 1 receives (Spec
	// S06.5–S06.6). The taxonomy is reduced to slot id → question so the
	// resolutions read meaningfully without the full weight table.
	slots := map[string]string{}
	if in.Taxonomy != nil {
		for _, s := range in.Taxonomy.Slots {
			slots[s.ID] = s.Question
		}
	}
	body, err := json.MarshalIndent(struct {
		Request     intake.Request            `json:"request"`
		Family      intake.Family             `json:"family"`
		Tier        intake.Tier               `json:"tier"`
		Slots       map[string]string         `json:"slots,omitempty"`
		Resolutions []intake.SlotResolution   `json:"resolutions,omitempty"`
		DataHits    []intake.TriggerHit       `json:"data_hits,omitempty"`
		Supplied    []intake.SuppliedFact     `json:"supplied,omitempty"`
		Escalations []intake.EscalationAnswer `json:"escalations,omitempty"`
		Prior       *intake.Pair              `json:"prior,omitempty"`
	}{in.Request, in.Family, in.Tier, slots, in.Resolutions, in.DataHits, in.Supplied, in.Escalations, in.Prior}, "", "  ")
	if err != nil {
		return ledger.Item{}, fmt.Errorf("stage: marshal draft input: %w", err)
	}
	return ledger.Item{
		ItemID:       itemDraftInput,
		SourcePath:   "platform:" + itemDraftInput,
		Content:      string(body),
		Version:      fmt.Sprintf("spec-v%d", in.SpecVersion),
		SelectorRule: selectorIntake,
		Precedence:   ledger.PrecedenceStage,
	}, nil
}

func reviseInputItem(in intake.ReviseInput) (ledger.Item, error) {
	body, err := json.MarshalIndent(in, "", "  ")
	if err != nil {
		return ledger.Item{}, fmt.Errorf("stage: marshal revise input: %w", err)
	}
	return ledger.Item{
		ItemID:       itemReviseInput,
		SourcePath:   "platform:" + itemReviseInput,
		Content:      string(body),
		Version:      fmt.Sprintf("plan-v%d", in.PlanVersion),
		SelectorRule: selectorIntake,
		Precedence:   ledger.PrecedenceStage,
	}, nil
}

// ---- intake.Critic (Spec S06.8: planning-model class, fresh context,
// artifact-only input) ----

// EngineCritic drives the Stage-3 self-attack session. ARTIFACT-ONLY: the
// prompt is built from the pair alone — no ledger assembly, no interview
// record, no transcript; the fresh session IS the context separation
// (Spec S06.8; the seam signature admits nothing else).
type EngineCritic struct{ s *Skeleton }

var _ intake.Critic = (*EngineCritic)(nil)

const verdictSchema = `Output EXACTLY one JSON object:
{"kind":"pass"|"revise"|"spec_doubt"|"tier_up",
 "findings":[string...],   // numbered blocker findings (revise)
 "doubt":string,           // plain-language why (spec_doubt)
 "proposed_tier":"low"|"standard"|"high"}  // (tier_up)`

func (c *EngineCritic) Critique(ctx context.Context, pair intake.Pair) (intake.Verdict, error) {
	return c.session(ctx, pair, nil)
}

func (c *EngineCritic) Recheck(ctx context.Context, pair intake.Pair, findings []string) (intake.Verdict, error) {
	return c.session(ctx, pair, findings)
}

func (c *EngineCritic) session(ctx context.Context, pair intake.Pair, recheck []string) (intake.Verdict, error) {
	pairJSON, err := json.MarshalIndent(pair, "", "  ")
	if err != nil {
		return intake.Verdict{}, fmt.Errorf("stage: marshal pair: %w", err)
	}
	var b strings.Builder
	b.WriteString(stageMarker(markerCritique))
	b.WriteString("You are the plan critic (Spec S06.8): fresh context, artifact-only input — attack this SPEC/PLAN pair.\n")
	b.WriteString("Hunt: unmet spec intent, untestable done-when contracts, hidden effects/spend, wrong stakes tier, plans that cannot work.\n")
	b.WriteString("SPEC-DOUBT is mandatory when the specification itself is the problem — never soften it into a note.\n")
	if len(recheck) > 0 {
		fmt.Fprintf(&b, "RE-CHECK ONLY these prior findings (judge nothing new): %s\n", strings.Join(recheck, "; "))
	}
	b.WriteString("\n=== the pair ===\n")
	b.Write(pairJSON)
	b.WriteString("\n\n" + verdictSchema + "\n")

	res, err := c.s.Session(ctx, SessionInput{
		RunID:        pair.Spec.TaskID + RunSuffixIntake,
		Stage:        "critique",
		Assemble:     false, // artifact-only (Spec S06.8)
		Instructions: b.String(),
		Class:        "C1",
	})
	if err != nil {
		return intake.Verdict{}, fmt.Errorf("critic session: %w", err)
	}
	var v intake.Verdict
	if err := parseJSONOutput(res.Text, &v); err != nil {
		return intake.Verdict{}, fmt.Errorf("critic output: %w", err)
	}
	switch v.Kind {
	case intake.VerdictPass, intake.VerdictRevise, intake.VerdictSpecDoubt, intake.VerdictTierUp:
	default:
		return intake.Verdict{}, fmt.Errorf("critic output: unknown verdict kind %q", v.Kind)
	}
	return v, nil
}

// ---- verify.Judge (Spec S07.5: two separately prompted axes; wired B2-4) ----

// EngineJudge drives the V2 judge sessions on the task's verify run. The
// input slice is EXACTLY JudgeInput — built by verify.BuildJudgeInput
// through the ledger's clean-context assembly; there is no transcript
// anywhere (Spec S07.5). Selection/pairing mechanics are Spec S08's (B3):
// at B2-4 the judge is the configured dev model, honestly reported
// self-family (executor and judge share the one wired lane).
type EngineJudge struct{ s *Skeleton }

var _ verify.Judge = (*EngineJudge)(nil)

const axis1Schema = `Output EXACTLY one JSON object:
{"verdicts":[{"key":"AC-1","pass":bool,"unknown":bool,"evidence":string}...],
 "findings":[{"severity":"blocker"|"note","category":"ac-blocker","criterion":"AC-n","anchor":string,"text":string}...],
 "escalate":string}  // non-empty ONLY to escalate instead of another round
Rules: one verdict per numbered AC; evidence MUST be an EXACT substring of the
artifact (extractive quote) — a PASS without one is forced Unknown; every
blocker cites the frozen criterion it violates.`

const axis2Schema = `Output EXACTLY one JSON object:
{"probe_notes":{"reasonable-user":string,"implicit-expectations":string,"side-effects":string,"expert-standard":string},
 "findings":[{"severity":"blocker"|"note","category":"sanity-blocker","criterion":string,"anchor":string,"text":string}...],
 "reopen_spec":string,  // non-empty ONLY when the SPECIFICATION itself is wrong (files a human decision card)
 "escalate":string}
Rules: all four probes answered; unrequested changes are failures, not bonuses.`

func (j *EngineJudge) Compliance(ctx context.Context, in verify.JudgeInput) (verify.Axis1Result, error) {
	instructions := stageMarker(markerCompliance) +
		"You are the spec-compliance judge (Spec S07.5 axis 1): one binary verdict per numbered AC over the input slice above.\n" +
		axis1Schema + "\n"
	var out verify.Axis1Result
	if err := j.session(ctx, in, instructions, &out); err != nil {
		return verify.Axis1Result{}, err
	}
	return out, nil
}

func (j *EngineJudge) Sanity(ctx context.Context, in verify.JudgeInput) (verify.Axis2Result, error) {
	instructions := stageMarker(markerSanity) +
		"You are the outcome-sanity judge (Spec S07.5 axis 2), separately prompted with your own mini-rubric.\n" +
		"Probes: would a reasonable user consider this done and good? what would a well-informed person expect that is absent? " +
		"any unrequested side effects? does it meet the expert standard?\n" +
		axis2Schema + "\n"
	var out verify.Axis2Result
	if err := j.session(ctx, in, instructions, &out); err != nil {
		return verify.Axis2Result{}, err
	}
	return out, nil
}

func (j *EngineJudge) session(ctx context.Context, in verify.JudgeInput, instructions string, out any) error {
	// BuildJudgeInput already ran the clean-context assembly (manifest
	// appended); the brief is re-used for pinned placement so a compaction
	// inside the judge session re-injects the frozen ACs (Spec S05.7).
	brief := in.Brief
	res, err := j.s.Session(ctx, SessionInput{
		RunID:        in.Brief.TaskID + RunSuffixVerify,
		Stage:        "verify",
		Assemble:     false,
		PlaceBrief:   &brief,
		Instructions: in.BriefText + "\n" + instructions,
		Class:        "C1",
	})
	if err != nil {
		return fmt.Errorf("judge session: %w", err)
	}
	if err := parseJSONOutput(res.Text, out); err != nil {
		return fmt.Errorf("judge output: %w", err)
	}
	return nil
}

// Meta implements verify.Judge. SelfFamily is honestly TRUE at B2-4: the
// one wired lane serves executor and judge alike, and the flag rides the
// receipt (G1 Def.1); the dissimilar-lane swap is S08 selection (B3).
func (j *EngineJudge) Meta() verify.JudgeMeta {
	return verify.JudgeMeta{Model: j.s.modelFor(""), SelfFamily: true}
}

// ---- verify.Revise (Spec S07.6: fresh-session rework executor) ----

// engineRevise is the fresh-session executor seam: each round builds a NEW
// session from EXACTLY the retry package (fork-don't-poison; the package
// members are the whole world — no ledger assembly, no history).
func (s *Skeleton) engineRevise(ctx context.Context, pkg verify.RetryPackage) (verify.Deliverable, error) {
	pkgJSON, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return verify.Deliverable{}, fmt.Errorf("stage: marshal retry package: %w", err)
	}
	instructions := stageMarker(markerRevise) + fmt.Sprintf(
		"You are a fresh executor session for rework round %d (Spec S07.6).\n"+
			"The retry package below is your ENTIRE input: the original SPEC, the frozen ACs, the numbered findings with anchors, and the current deliverable.\n"+
			"Fix EXACTLY the named problems; never regenerate blind; change nothing already correct.\n"+
			"Output ONLY the complete corrected deliverable content — no commentary, no JSON wrapper.\n\n=== retry package ===\n%s\n",
		pkg.Round, pkgJSON)
	res, err := s.Session(ctx, SessionInput{
		RunID:        pkg.Deliverable.TaskID + RunSuffixVerify,
		Stage:        fmt.Sprintf("revise-r%d", pkg.Round),
		Assemble:     false,
		Instructions: instructions,
		Class:        "C1",
	})
	if err != nil {
		return verify.Deliverable{}, fmt.Errorf("revise session: %w", err)
	}
	d := pkg.Deliverable
	d.PrevContent = d.Content
	d.Content = res.Text
	d.Revision = pkg.Deliverable.Revision + 1
	d.Diff = "" // revision diff mechanics are Spec S13's (B4); the judge receives the full artifact
	return d, nil
}
