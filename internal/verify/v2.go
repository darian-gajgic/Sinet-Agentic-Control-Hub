package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
)

// V2 — the dual-axis judge pass (Spec S07.5), behind an engine seam (the
// B2-2 Planner/Critic pattern; real engine sessions wire at B2-4, selection
// mechanics are Spec S08's).
//
// Independence: the judge is a fresh, clean-context platform session —
// never the executor's session. Its input slice is the WHOLE world it sees:
// the artifact + its diff against the previous revision + the frozen ACs +
// the rubric version + V1 check outcomes as evidence + prior-round findings
// — NEVER the execution transcript (the O(M²) prefill trap, and the
// transcript is the executor's frame). The slice is materialized through
// the ledger's clean-context assembly (Spec S05.4 exception): the ledger
// projection reduces to objective_ac, user-overlay items and
// learned_this_task are structurally excluded, and everything else enters
// as labeled Extra items on the manifest.

// Probe names one of the four ratified axis-2 probes (Spec S07.5).
type Probe string

const (
	ProbeReasonableUser       Probe = "reasonable-user"
	ProbeImplicitExpectations Probe = "implicit-expectations"
	ProbeSideEffects          Probe = "side-effects"
	ProbeExpertStandard       Probe = "expert-standard"
)

// Probes is the ratified axis-2 probe set, in prompt order.
var Probes = []Probe{ProbeReasonableUser, ProbeImplicitExpectations, ProbeSideEffects, ProbeExpertStandard}

// ACVerdict is one axis-1 per-criterion verdict: binary with a mandatory
// extractive evidence quote and an Unknown escape (Spec S07.5).
type ACVerdict struct {
	Key string `json:"key"` // "AC-3"
	// Pass is the binary verdict; meaningless when Unknown.
	Pass bool `json:"pass"`
	// Unknown is the ratified escape: the judge could not decide.
	Unknown bool `json:"unknown,omitempty"`
	// BoundTo records which phrasing the verdict bound to: "structured"
	// where a sub-line exists, else "plain" (G1 P10).
	BoundTo string `json:"bound_to"`
	// Evidence is the extractive quote grounding the verdict. A PASS whose
	// evidence is not found verbatim in the artifact is forced to Unknown
	// by validation (Spec S07.10: a high score is impossible without
	// evidence).
	Evidence string `json:"evidence,omitempty"`
	// FromV1 marks a criterion whose structured sub-line executed at V1:
	// the judge consumes the mechanical outcome as evidence and does not
	// re-decide it (Spec S07.5).
	FromV1 bool `json:"from_v1,omitempty"`
	// Disagreement records a judge–check disagreement — a CHECK-INTEGRITY
	// finding, never an override: the V1 fact stands (Spec S07.5).
	Disagreement bool `json:"disagreement,omitempty"`
	// Forced records a validation intervention ("" = none): "unknown:
	// missing verdict" | "unknown: non-extractive evidence".
	Forced string `json:"forced,omitempty"`
}

// Axis1Result is the spec-compliance pass output.
type Axis1Result struct {
	Verdicts []ACVerdict `json:"verdicts"`
	Findings []Finding   `json:"findings,omitempty"`
	// Escalate, non-empty, is the judge's explicit ESCALATE verdict reason.
	Escalate string `json:"escalate,omitempty"`
}

// Axis2Result is the outcome-sanity pass output — separately prompted with
// its own mini-rubric, never folded into the compliance pass (Spec S07.5).
type Axis2Result struct {
	// ProbeNotes carries the per-probe observations; the four probes exist
	// structurally, so an unprompted second axis cannot happen.
	ProbeNotes map[Probe]string `json:"probe_notes"`
	Findings   []Finding        `json:"findings,omitempty"`
	// ReopenSpec, non-empty, is axis 2's unique power: the deliverable-time
	// spec challenge, filed as a decision card and never a spec edit (Spec
	// S07.5; D10).
	ReopenSpec string `json:"reopen_spec,omitempty"`
	// Escalate, non-empty, is the judge's explicit ESCALATE verdict reason.
	Escalate string `json:"escalate,omitempty"`
}

// JudgeMeta identifies the judge engine for the verdict record and the
// receipt flags (Spec S07.5/S07.11; selection mechanics Spec S08).
type JudgeMeta struct {
	Model string `json:"model"`
	// SelfFamily: the judge's model family equals the executor's — always
	// flagged on the receipt (G1 Def.1).
	SelfFamily bool `json:"self_family,omitempty"`
}

// JudgeInput is the S07.5 input slice — the whole world the judge sees.
// There is no transcript field: transcript access is structurally absent,
// not policy-forbidden.
type JudgeInput struct {
	// Brief is the clean-context assembly (Spec S05.4 exception): ledger
	// projection = objective_ac only; Extra carries the rest of the slice;
	// every item is manifested.
	Brief ledger.Brief
	// BriefText is the deterministic prompt body of the brief.
	BriefText string
	// ACs is the frozen numbered criterion set from the ledger's pinned §1.
	ACs []ledger.AcceptanceCriterion
	// Artifact and Diff are the deliverable revision under judgment.
	Artifact string
	Diff     string
	// RubricID/RubricVersion pin the rubric bundle this pass judges under
	// (Spec S07.10: immutable versioned bundles).
	RubricID      string
	RubricVersion int
	// V1 outcomes enter as evidence (Spec S07.5).
	V1 *V1Result
	// PriorFindings are carried verbatim into re-review rounds (Spec
	// S07.6: prior findings in scope, original criteria).
	PriorFindings []Finding
	Round         int
}

// Judge is the V2 engine seam. Compliance and Sanity are SEPARATE prompted
// calls (Spec S07.5: both axes fit one prompted call each; the second axis
// is never folded into the first). Wiring to real engine sessions is B2-4;
// the judge engine class and pairing rules are Spec S08's.
type Judge interface {
	Compliance(ctx context.Context, in JudgeInput) (Axis1Result, error)
	Sanity(ctx context.Context, in JudgeInput) (Axis2Result, error)
	Meta() JudgeMeta
}

// Judge-input item identities on the assembly manifest (Spec S05.4 entry
// schema; stage-precedence Extra items).
const (
	itemArtifact      = "verify/artifact"
	itemDiff          = "verify/diff"
	itemRubric        = "verify/rubric"
	itemV1Outcomes    = "verify/v1-outcomes"
	itemPriorFindings = "verify/prior-findings"

	selectorVerify = "verification input slice (S07.5) — artifact+diff+rubric+V1+prior findings; never the execution transcript"
)

// BuildJudgeInput assembles the S07.5 input slice through the ledger's
// clean-context assembly, appending the context.manifest event for the
// judge session (Spec S05.4). The ledger store enforces the clean-mode
// firewall (user overlay dropped, learned excluded, objective_ac only);
// this function contributes the S07-owned Extra items.
func BuildJudgeInput(ctx context.Context, store *ledger.Store, d Deliverable, rubric *RubricBundle, v1 *V1Result, prior []Finding, round int) (JudgeInput, error) {
	if store == nil {
		return JudgeInput{}, fmt.Errorf("%w: V2 without the ledger store (clean-context assembly, Spec S05.4)", ErrSeamMissing)
	}
	if rubric == nil {
		return JudgeInput{}, fmt.Errorf("%w: V2 without a rubric bundle (Spec S07.10)", ErrSeamMissing)
	}
	extra := []ledger.Item{
		extraItem(itemArtifact, d.Content, fmt.Sprintf("rev%d", d.Revision)),
		extraItem(itemDiff, d.Diff, fmt.Sprintf("rev%d", d.Revision)),
		extraItem(itemRubric, rubricSlice(rubric), fmt.Sprintf("v%d", rubric.Version)),
	}
	if v1 != nil {
		raw, err := json.Marshal(v1)
		if err != nil {
			return JudgeInput{}, fmt.Errorf("verify: marshal V1 outcomes: %w", err)
		}
		extra = append(extra, extraItem(itemV1Outcomes, string(raw), fmt.Sprintf("pack-v%d", v1.PackVersion)))
	}
	if len(prior) > 0 {
		raw, err := json.Marshal(prior)
		if err != nil {
			return JudgeInput{}, fmt.Errorf("verify: marshal prior findings: %w", err)
		}
		extra = append(extra, extraItem(itemPriorFindings, string(raw), fmt.Sprintf("round%d", round-1)))
	}
	brief, err := store.Assemble(ctx, ledger.AssembleInput{
		RunID: d.RunID,
		Stage: "verify",
		Clean: true,
		Extra: extra,
	})
	if err != nil {
		return JudgeInput{}, fmt.Errorf("verify: assemble judge input: %w", err)
	}
	doc, _, err := currentLedgerDoc(ctx, store, d.TaskID)
	if err != nil {
		return JudgeInput{}, err
	}
	return JudgeInput{
		Brief:         brief,
		BriefText:     ledger.BriefText(brief),
		ACs:           doc.ObjectiveAC.AcceptanceCriteria,
		Artifact:      d.Content,
		Diff:          d.Diff,
		RubricID:      rubric.ID,
		RubricVersion: rubric.Version,
		V1:            v1,
		PriorFindings: prior,
		Round:         round,
	}, nil
}

func currentLedgerDoc(ctx context.Context, store *ledger.Store, taskID string) (ledger.Document, bool, error) {
	doc, found, err := store.Current(ctx, taskID)
	if err != nil {
		return ledger.Document{}, false, fmt.Errorf("verify: read task ledger: %w", err)
	}
	if !found {
		return ledger.Document{}, false, fmt.Errorf("%w: task %q has no ledger (frozen ACs live in §1)", ErrBadInput, taskID)
	}
	return doc, true, nil
}

func extraItem(id, content, version string) ledger.Item {
	return ledger.Item{
		ItemID:       id,
		SourcePath:   "platform:" + id,
		Content:      content,
		Version:      version,
		SelectorRule: selectorVerify,
		Precedence:   ledger.PrecedenceStage,
	}
}

// rubricSlice renders the judge-facing rubric slice.
func rubricSlice(r *RubricBundle) string {
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		// RubricBundle is a plain struct; marshal cannot fail on it.
		return fmt.Sprintf("<rubric render error: %v>", err)
	}
	return string(raw)
}

// ValidateAxis1 applies the platform-side axis-1 rules over a judge result
// (Spec S07.5), returning the corrected verdict set plus any
// CHECK-INTEGRITY findings from judge–check disagreements:
//
//   - every frozen criterion carries exactly one verdict — a missing one is
//     forced to Unknown (recorded), never silently passed;
//   - a PASS whose evidence quote is not found verbatim in the artifact is
//     forced to Unknown (extractive grounding, Spec S07.10);
//   - a criterion executed at V1 takes the mechanical outcome as its
//     verdict; a judge disagreement is recorded and raised CHECK-INTEGRITY,
//     never an override.
func ValidateAxis1(res Axis1Result, acs []ledger.AcceptanceCriterion, v1 map[string]CheckOutcomeState, artifact string) ([]ACVerdict, []Finding) {
	byKey := map[string]ACVerdict{}
	for _, v := range res.Verdicts {
		byKey[v.Key] = v
	}
	var (
		out       []ACVerdict
		integrity []Finding
	)
	for _, ac := range acs {
		key := fmt.Sprintf("AC-%d", ac.N)
		v, ok := byKey[key]
		if !ok {
			v = ACVerdict{Key: key, Unknown: true, Forced: "unknown: missing verdict"}
		}
		v.BoundTo = "plain"
		if ac.Structured != "" {
			v.BoundTo = "structured"
		}
		if mech, executed := v1[key]; executed && (mech == CheckPassed || mech == CheckFailed) {
			mechPass := mech == CheckPassed
			if ok && !v.Unknown && v.Pass != mechPass {
				v.Disagreement = true
				integrity = append(integrity, Finding{
					Severity:  SeverityBlocker,
					Category:  CatCheckIntegrity,
					Criterion: key,
					Anchor:    "check:" + key,
					Text: fmt.Sprintf("judge–check disagreement on %s: V1 mechanical outcome %s, judge said pass=%v — the mechanical fact stands; the suite (or the judge) needs a human look",
						key, mech, v.Pass),
				})
			}
			v.Pass = mechPass
			v.Unknown = false
			v.FromV1 = true
			out = append(out, v)
			continue
		}
		if !v.Unknown && v.Pass && (v.Evidence == "" || !strings.Contains(artifact, v.Evidence)) {
			v.Pass = false
			v.Unknown = true
			v.Forced = "unknown: non-extractive evidence"
		}
		out = append(out, v)
	}
	return out, integrity
}

// UnknownEscapes synthesizes the round's Unknown-escape findings (Spec
// S07.5): every criterion whose validated verdict is Unknown becomes a
// blocker-class AC-BLOCKER finding citing that criterion. An undecided
// criterion never dissolves into SHIP — every verification finding
// terminates in a human-visible sink (Spec S07.7), and an agreed criterion
// never disappears silently, at intake or ever after (Spec S06). The
// finding key (criterion + anchor + category) is round-independent: a
// criterion the judge can never decide recurs unresolved and trips the
// S07.6 convergence stop into a CAP-HIT card, while a rework that makes it
// decidable resolves the key. (Rule added at the B2 gate demo, 2026-07-20:
// an all-Unknown round had computed a clean SHIP.)
func UnknownEscapes(verdicts []ACVerdict) []Finding {
	var fs []Finding
	for _, v := range verdicts {
		if !v.Unknown {
			continue
		}
		reason := v.Forced
		if reason == "" {
			reason = "judge returned unknown"
		}
		fs = append(fs, Finding{
			Severity:  SeverityBlocker,
			Category:  CatACBlocker,
			Criterion: v.Key,
			Anchor:    "unknown:" + v.Key,
			Text: fmt.Sprintf("%s could not be verified (%s): an undecided criterion cannot ship (Spec S07.5 Unknown escape) — make the deliverable decidable against it, or the drain escalates",
				v.Key, reason),
		})
	}
	return fs
}

// validateFindings numbers a round's findings [F1..Fn] and enforces the
// citation rule (Spec S07.5): a blocker must cite a frozen criterion or an
// axis-2 rubric item; one that cites none (or cites outside the frozen set)
// is DEMOTED to note, recorded as demoted — goalposts stay structurally
// fixed. After round 1, NEW note-class finding keys are suppressed to a
// count (goalpost-drift suppression, Spec S07.6).
func validateFindings(round int, fs []Finding, acs []ledger.AcceptanceCriterion, rubric *RubricBundle, priorKeys map[FindingKey]bool) (kept []Finding, suppressed int) {
	valid := map[string]bool{string(CatCheckIntegrity): true}
	for _, ac := range acs {
		valid[fmt.Sprintf("AC-%d", ac.N)] = true
	}
	if rubric != nil {
		for _, item := range rubric.Items {
			valid[item.ID] = true
		}
	}
	n := 0
	for _, f := range fs {
		f.Round = round
		if f.Severity == SeverityBlocker && (f.Criterion == "" || !valid[f.Criterion]) {
			f.Severity = SeverityNote
			f.Demoted = true
		}
		// The bootstrap posture disclosure is exempt: it is not a new goalpost
		// but a statement about how this round was verified, and a capture
		// that regresses mid-drain (or a resume whose carried history is all
		// full-posture) would otherwise mint it at a later round and have it
		// swallowed — the requester would never learn nothing was checked.
		if round > 1 && f.Severity == SeverityNote && !priorKeys[f.Key()] && !isPostureDisclosure(f) {
			suppressed++
			continue
		}
		n++
		f.N = n
		kept = append(kept, f)
	}
	return kept, suppressed
}

// ComputeVerdict combines the validated axis results into the round verdict
// (Spec S07.5): REOPEN-SPEC (axis 2's power) and explicit ESCALATE dominate;
// blockers → REVISE; notes only → SHIP-with-notes; clean → SHIP.
//
// CHECK-INTEGRITY findings are excluded from the blocker/note count: they
// are SUITE defects, not deliverable defects — their ratified route is a
// decision card + quarantine (Spec S07.7), never the rework drain, because
// regenerating the deliverable cannot fix a broken check. They still ride
// the round record and their own card.
func ComputeVerdict(ax2 *Axis2Result, findings []Finding, ax1Escalate string) Verdict {
	if ax2 != nil && ax2.ReopenSpec != "" {
		return VerdictReopenSpec
	}
	if ax1Escalate != "" || (ax2 != nil && ax2.Escalate != "") {
		return VerdictEscalate
	}
	hasBlocker, hasNote := false, false
	for _, f := range findings {
		if f.Category == CatCheckIntegrity {
			continue
		}
		switch f.Severity {
		case SeverityBlocker:
			hasBlocker = true
		case SeverityNote:
			hasNote = true
		}
	}
	switch {
	case hasBlocker:
		return VerdictRevise
	case hasNote:
		return VerdictShipWithNotes
	default:
		return VerdictShip
	}
}
