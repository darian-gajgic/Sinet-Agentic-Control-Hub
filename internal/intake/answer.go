package intake

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// Answering a card resumes the pipeline in place (4.3; Spec S06.1). Every
// intake card is answered by the requester — the D10 holder; approval in
// particular never self-approves (5.8).

// Answer applies one card answer and drives the pipeline onward until the
// next gate or terminal phase.
func (p *Pipeline) Answer(ctx context.Context, actor, askID string, raw json.RawMessage) (*State, error) {
	card, runID, err := p.openCard(ctx, askID)
	if err != nil {
		return nil, err
	}
	var taskID sql.NullString
	if err := p.DB.QueryRowContext(ctx, `SELECT task_id FROM runs WHERE run_id = ?`, runID).Scan(&taskID); err != nil {
		return nil, fmt.Errorf("intake: read run %q: %w", runID, err)
	}
	st, err := p.LoadState(ctx, taskID.String)
	if err != nil {
		return nil, err
	}
	if st.OpenAskID != askID {
		return nil, fmt.Errorf("%w: %q is not the open intake gate", ErrUnknownAsk, askID)
	}
	if actor != st.Owner {
		return nil, fmt.Errorf("%w: %q", ErrNotRequester, actor)
	}
	ans, err := decodeAnswer(raw)
	if err != nil {
		return nil, err
	}

	switch card.Kind {
	case CardInterview:
		if err := p.applyInterviewAnswer(ctx, st, card, ans); err != nil {
			return nil, err
		}
	case CardClarification:
		if err := p.applyClarificationAnswer(st, card, ans); err != nil {
			return nil, err
		}
	case CardEscalation:
		if ans.Text == "" {
			return nil, fmt.Errorf("%w: escalation needs a text answer", ErrBadAnswer)
		}
		from := st.PendingEscalation
		st.Escalations = append(st.Escalations, EscalationAnswer{
			From: from, Question: card.Questions[0].Text, Answer: ans.Text, TS: p.nowRFC3339(),
		})
		st.PendingEscalation = ""
		switch from {
		case "revise":
			st.Phase = PhaseSpine // PendingRevise is still set; the revise re-runs
		case "critique":
			st.Phase = PhaseCritique
		default:
			st.Phase = PhaseInterview
		}
	case CardFamily:
		if err := p.applyFamilyAnswer(ctx, st, ans); err != nil {
			return nil, err
		}
	case CardCoverage:
		if err := p.applyCoverageAnswer(ctx, st, ans); err != nil {
			return nil, err
		}
	case CardResearch:
		if err := p.applyResearchAnswer(ctx, st, ans); err != nil {
			return nil, err
		}
	case CardSpecDoubt:
		if err := p.applySpecDoubtAnswer(ctx, st, ans); err != nil {
			return nil, err
		}
	case CardApproval:
		return p.applyApprovalAnswer(ctx, st, card, ans, raw)
	case CardDelta:
		return p.applyDeltaAnswer(ctx, st, ans, raw)
	default:
		return nil, fmt.Errorf("%w: kind %q", ErrBadAnswer, card.Kind)
	}

	if err := p.closeAndResume(ctx, st, askID, "answered", raw, ans.Note); err != nil {
		return nil, err
	}
	if st.Phase == PhaseCancelled {
		return st, nil
	}
	return p.advanceLoaded(ctx, st)
}

// closeAndResume closes the ask, resumes the parked run (parked→running —
// the resume bumps the generation, Spec S02.3), and appends the state, in
// one transaction.
//
// cancelReason is the person's own words for a cancel. It is read ONLY on the
// leg below, which is what makes the answer note "ignored outside the two
// cancel-shaped answers" (P3-RW-19 ratified OQ2) a property of this code rather
// than of each caller's care: a note handed in with an answer that resumes the
// pipeline reaches nothing.
func (p *Pipeline) closeAndResume(ctx context.Context, st *State, askID, status string, raw json.RawMessage, cancelReason string) error {
	st.OpenAskID, st.OpenAskKind = "", ""
	return p.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		if err := p.closeAskTx(ctx, tx, askID, status, raw); err != nil {
			return err
		}
		r, err := p.Runs.GetTx(ctx, tx, st.RunID)
		if err != nil {
			return err
		}
		if r.State == run.StateParked {
			to := run.StateRunning
			reason := "intake gate answered: pipeline resumes in place (4.3)"
			opts := run.TransitionOptions{Reason: reason, Actor: run.ActorPlatform}
			if st.Phase == PhaseCancelled {
				// 15.6 attribution: a cancel is a human act, so its transition
				// names the human (the intake's requester — the only principal
				// intake accepts an answer from) and carries the ONE shared
				// structured detail, naming the card it was answered at
				// (P3-RW-18 D1-R1). The resume leg stays platform-attributed:
				// resuming a pipeline is the platform's act, not a decision.
				to = run.StateFinalized
				opts = run.TransitionOptions{
					Reason: "intake cancelled (4.5): finalize-with-card",
					Actor:  st.Owner, Detail: run.CancelDetail(st.Owner, false, askID, cancelReason),
				}
			}
			if _, err := p.Runs.TransitionTx(ctx, tx, st.RunID, to, opts); err != nil {
				return err
			}
		}
		return p.appendStateTx(ctx, tx, st)
	})
}

// ---- Per-kind answer application ----

func (p *Pipeline) applyInterviewAnswer(ctx context.Context, st *State, card *Card, ans Answer) error {
	tax := p.taxonomyFor(st.Family)
	asked := make(map[string]Question, len(card.Questions))
	for _, q := range card.Questions {
		asked[q.ID] = q
	}
	// The family correction is VALIDATED before anything is applied, for the same
	// reason the arm conflict below is: a refused body must change nothing at all
	// (P3-GF7 R9). It is APPLIED last, after the answers have landed against the
	// set that was actually asked.
	correction := Family(ans.Family)
	if ans.Family != "" && !ValidFamily(correction) {
		return fmt.Errorf("%w: task family %q is not one this platform offers — it offers %s",
			ErrBadAnswer, ans.Family, familyVocabularySentence())
	}
	// One slot, ONE arm. The per-entry check below refuses a single entry that
	// carries both a value and a skip; this refuses the same contradiction
	// spread across two entries, which is the shape a surface actually produces
	// (a chip toggled, then the box typed into). Both are the same rule: a body
	// that answers a question and skips it says two different things about it,
	// and the platform must not pick one. It runs BEFORE anything is applied, so
	// a refused body changes nothing at all.
	arm := make(map[string]bool, len(ans.Answers))
	for _, a := range ans.Answers {
		if prior, seen := arm[a.ID]; seen && prior != a.Skip {
			return fmt.Errorf("%w: slot %q is answered in one entry and skipped in another; a skip takes the recommendation instead of an answer", ErrBadAnswer, a.ID)
		}
		arm[a.ID] = a.Skip
	}
	var skipped []SlotResolution
	for _, a := range ans.Answers {
		question, onCard := asked[a.ID]
		if !onCard || tax.Slot(a.ID) == nil {
			return fmt.Errorf("%w: slot %q was not asked", ErrBadAnswer, a.ID)
		}
		if a.Skip {
			// The per-slot skip is S06.5's third resolution arm ("converted to
			// an explicit assumption"), aimed at ONE slot: the requester takes
			// the recommendation for this question and moves on, and the
			// assumption lands on the approval card's centerpiece where it can
			// be contested. It is strictly more conservative than the
			// whole-interview force-proceed, which stays exactly as it was.
			//
			// Skipping is not answering, so the two arms are exclusive: a body
			// carrying both says two different things about the same slot and
			// the platform must not pick one.
			if a.Value != "" {
				return fmt.Errorf("%w: slot %q carries both an answer and a skip; a skip takes the recommendation instead of an answer", ErrBadAnswer, a.ID)
			}
			r := SlotResolution{
				SlotID: a.ID, How: ResolvedAssumption,
				Assumption: skipAssumption(tax.Slot(a.ID), question),
			}
			st.resolveSlot(r)
			skipped = append(skipped, r)
			continue
		}
		if a.Value == "" {
			return fmt.Errorf("%w: empty answer for %q", ErrBadAnswer, a.ID)
		}
		st.resolveSlot(SlotResolution{SlotID: a.ID, How: ResolvedAnswered, Value: a.Value})
	}
	for _, a := range ans.Assume {
		if tax.Slot(a.ID) == nil {
			return fmt.Errorf("%w: unknown slot %q", ErrBadAnswer, a.ID)
		}
		st.resolveSlot(SlotResolution{SlotID: a.ID, How: ResolvedAssumption, Assumption: a.Value})
	}
	if ans.ForceProceed {
		// Force-proceed converts every unresolved must-know slot into a
		// listed assumption on the approval card (S06.5).
		st.ForceProceeded = true
	}
	st.Clearance = tax.Clearance(st.resolvedSet())

	// The guess was wrong and the requester said so. The switch runs AFTER the
	// answers, so what they already told the platform is kept on the record and
	// simply stops counting toward Clearance where the new set has no such slot —
	// applyFamilyTaxonomy re-keys the question set, re-applies the registry's
	// pre-answered slots and re-settles the never-asked ones against THAT set,
	// and the next advance asks the right questions (P3-GF7 R9; H17 REJECT).
	if correction != "" && correction != st.Family {
		st.Family, st.FamilySource = correction, FamilySourceRequester
		fallback := p.applyFamilyTaxonomy(st)
		// The correction is the REQUESTER'S, so it is human-authored in the
		// ledger — the same authorship the family card's own answer carries.
		if _, err := p.Ledger.RecordDecision(ctx, st.RunID, ledger.AuthorHuman, st.Owner, "triage",
			"requester corrected the task family to "+string(correction),
			"interview card (S06.5; the family guess is shown and correctable before it shapes the questions)", 0); err != nil {
			return err
		}
		if err := p.recordTaxonomyFallback(ctx, st, fallback); err != nil {
			return err
		}
	}
	if !st.NeedsDraft {
		// A pair exists (re-interview / TIER-UP): fold the new answers in
		// via a bounded revision, then re-verify.
		merge := &ReviseReq{Reason: ReviseInterview}
		for _, a := range ans.Answers {
			if a.Skip {
				continue // carried by the skip resolutions below
			}
			merge.Resolutions = append(merge.Resolutions, SlotResolution{SlotID: a.ID, How: ResolvedAnswered, Value: a.Value})
		}
		merge.Resolutions = append(merge.Resolutions, skipped...)
		for _, a := range ans.Assume {
			merge.Resolutions = append(merge.Resolutions, SlotResolution{SlotID: a.ID, How: ResolvedAssumption, Assumption: a.Value})
		}
		st.PendingRevise = mergeRevise(st.PendingRevise, merge)
		st.Phase = PhaseSpine
	}
	return nil
}

// skipAssumption writes the disclosed assumption a skipped slot carries, in
// words for the PERSON who will read it on the approval card (CONVENTIONS §59).
//
// It says its own reason, because this arm's reason is genuinely its own: the
// band was never asked, the force-proceed was asked and declined the lot, and
// this requester answered the other questions and told the platform to take the
// recommendation on this one. The suggestion comes from the CARD SNAPSHOT — the
// durable record of what was actually offered — never recomputed, so the
// assumption says what the requester saw when they skipped.
func skipAssumption(slot *Slot, asked Question) string {
	name := asked.ID
	if slot != nil && slot.Name != "" {
		name = slot.Name
	}
	if suggestion := strings.TrimSpace(asked.Suggested); suggestion != "" {
		return fmt.Sprintf("%s: you skipped this one, so I am going with what was suggested on the card: %s", name, suggestion)
	}
	return fmt.Sprintf("%s: you skipped this one, so I will pick something sensible and show you what I picked on the plan.", name)
}

func (p *Pipeline) applyClarificationAnswer(st *State, card *Card, ans Answer) error {
	asked := make(map[string]string, len(card.Questions))
	for _, q := range card.Questions {
		asked[q.ID] = q.Text
	}
	req := &ReviseReq{Reason: ReviseMarkers}
	for _, a := range ans.Answers {
		text, ok := asked[a.ID]
		if !ok {
			return fmt.Errorf("%w: marker %q was not asked", ErrBadAnswer, a.ID)
		}
		if a.Value == "" {
			return fmt.Errorf("%w: empty answer for %q", ErrBadAnswer, a.ID)
		}
		req.Resolutions = append(req.Resolutions, SlotResolution{SlotID: a.ID, How: ResolvedAnswered, Value: text + " → " + a.Value})
	}
	if len(req.Resolutions) == 0 {
		return fmt.Errorf("%w: clarification card needs answers", ErrBadAnswer)
	}
	st.PendingRevise = mergeRevise(st.PendingRevise, req)
	st.Phase = PhaseSpine
	return nil
}

// applyFamilyAnswer applies the requester's answer to the S06.5 family question
// (P3-RW-11 R4). The phase does not move: the SAME advance that follows re-reads
// taxonomyFor and asks the chosen family's questions, and Clearance counts only
// slot ids present in the active set, so the swap needs no resolved-slot surgery
// (nothing has been asked yet — the card is pre-round by construction).
func (p *Pipeline) applyFamilyAnswer(ctx context.Context, st *State, ans Answer) error {
	fam := Family(ans.Choice)
	if !ValidFamily(fam) {
		// The card's own vocabulary refuses, and names what it takes. Validating
		// against the PACKAGE's list rather than the answering snapshot's is the
		// §43 discipline: a card issued before a vocabulary change answers exactly
		// as a fresh one does, and there is no third behavior to reason about.
		return fmt.Errorf("%w: task family %q is not one this card offers — it offers %s",
			ErrBadAnswer, ans.Choice, familyVocabularySentence())
	}
	st.Family, st.FamilySource = fam, FamilySourceRequester
	fallback := p.applyFamilyTaxonomy(st)

	// The choice is the REQUESTER'S, so it is human-authored in the ledger — the
	// same authorship a dropped criterion or an accepted gap carries (S06.7).
	if _, err := p.Ledger.RecordDecision(ctx, st.RunID, ledger.AuthorHuman, st.Owner, "triage",
		"requester answered the task family: "+string(fam),
		"family question (S06.5; 1.7 ask-don't-assume — nothing else resolved it)", 0); err != nil {
		return err
	}
	return p.recordTaxonomyFallback(ctx, st, fallback)
}

func (p *Pipeline) applyCoverageAnswer(ctx context.Context, st *State, ans Answer) error {
	uncovered := []string{}
	if st.Spine != nil {
		uncovered = st.Spine.Uncovered
	}
	switch ans.Choice {
	case ChoiceReplan:
		// The requester grants exactly one more revise round beyond the ⚙
		// auto-fix bound — an explicit human action, not an auto loop.
		st.PendingRevise = mergeRevise(st.PendingRevise, &ReviseReq{Reason: ReviseCoverage, Findings: uncovered})
		st.Phase = PhaseSpine
	case ChoiceDropCriterion:
		if len(ans.Criteria) == 0 {
			return fmt.Errorf("%w: drop_criterion needs criteria", ErrBadAnswer)
		}
		// An explicit, recorded requester drop — never silent (1.4). The
		// revision re-emits the spec without the dropped criteria; stable
		// keys renumber pre-approval (nothing froze yet), and the decision
		// entry plus prior artifact versions preserve traceability.
		if _, err := p.Ledger.RecordDecision(ctx, st.RunID, ledger.AuthorHuman, st.Owner, "spine",
			"requester dropped criteria: "+joinKeys(ans.Criteria),
			"coverage decision card (S06.7a)", 0); err != nil {
			return err
		}
		st.PendingRevise = mergeRevise(st.PendingRevise, &ReviseReq{Reason: ReviseDrop, Findings: ans.Criteria})
		st.Phase = PhaseSpine
	case ChoiceProceedUncovered:
		accepted := ans.Criteria
		if len(accepted) == 0 {
			accepted = uncovered
		}
		st.AcceptedUncovered = append(st.AcceptedUncovered, accepted...)
		if _, err := p.Ledger.RecordDecision(ctx, st.RunID, ledger.AuthorHuman, st.Owner, "spine",
			"requester accepted uncovered criteria: "+joinKeys(accepted),
			"coverage decision card (S06.7a); gap listed on the approval card", 0); err != nil {
			return err
		}
		st.Phase = PhaseSpine
	default:
		return fmt.Errorf("%w: coverage choice %q", ErrBadAnswer, ans.Choice)
	}
	return nil
}

func (p *Pipeline) applyResearchAnswer(ctx context.Context, st *State, ans Answer) error {
	missing := []string{}
	if st.Spine != nil {
		missing = st.Spine.MissingResearch
	}
	switch ans.Choice {
	case ChoiceSupplyFact:
		if len(ans.Facts) == 0 {
			return fmt.Errorf("%w: supply_fact needs facts", ErrBadAnswer)
		}
		// Dismissal is the requester's alone: the fact is recorded on the
		// SPEC as a requester-supplied input (S06.3).
		for _, f := range ans.Facts {
			f.TS = p.nowRFC3339()
			st.Supplied = append(st.Supplied, f)
		}
		if _, err := p.Ledger.RecordDecision(ctx, st.RunID, ledger.AuthorHuman, st.Owner, "spine",
			fmt.Sprintf("requester supplied %d fact(s), dismissing research trigger(s)", len(ans.Facts)),
			"research decision card (S06.3)", 0); err != nil {
			return err
		}
		// The supplied inputs belong on the SPEC: fold them in.
		st.PendingRevise = mergeRevise(st.PendingRevise, &ReviseReq{Reason: ReviseMarkers})
		st.Phase = PhaseSpine
	case ChoiceReplan:
		st.PendingRevise = mergeRevise(st.PendingRevise, &ReviseReq{Reason: ReviseResearch, Findings: missing})
		st.Phase = PhaseSpine
	default:
		return fmt.Errorf("%w: research choice %q", ErrBadAnswer, ans.Choice)
	}
	return nil
}

func (p *Pipeline) applySpecDoubtAnswer(ctx context.Context, st *State, ans Answer) error {
	record := func(text string) error {
		_, err := p.Ledger.RecordDecision(ctx, st.RunID, ledger.AuthorHuman, st.Owner, "critique",
			text, "SPEC-DOUBT decision card (S06.8)", 0)
		return err
	}
	switch ans.Choice {
	case ChoiceProceedAnyway:
		if err := record("requester: proceed anyway despite SPEC-DOUBT"); err != nil {
			return err
		}
		st.CritiqueDone = true
		st.Phase = PhaseApproval
	case ChoiceAdjustSpec:
		if err := record("requester: adjust spec after SPEC-DOUBT"); err != nil {
			return err
		}
		st.ReinterviewRequested = true
		st.ForceProceeded = false
		st.CritiqueDone = false
		st.Phase = PhaseInterview
	case ChoiceRethink:
		if err := record("requester: rethink — intake cancelled after SPEC-DOUBT"); err != nil {
			return err
		}
		st.Phase = PhaseCancelled
	default:
		return fmt.Errorf("%w: spec-doubt choice %q", ErrBadAnswer, ans.Choice)
	}
	return nil
}

func (p *Pipeline) applyApprovalAnswer(ctx context.Context, st *State, card *Card, ans Answer, raw json.RawMessage) (*State, error) {
	// S08.8 re-route/pin rides the approval answer — the pre-execution
	// override surface (validated against the card's offered set and
	// recorded with its actor). With Approve it binds the execution; with
	// Re-plan / Re-interview the pin survives into the recomputed card
	// (computeRouting keeps pinned selections).
	if ans.Route != nil {
		switch ans.Action {
		case ActionApprove, ActionRePlan, ActionReInterview:
			if err := applyRouteOverride(st, ans.Route, st.Owner); err != nil {
				return nil, err
			}
			if _, err := p.Ledger.RecordDecision(ctx, st.RunID, ledger.AuthorHuman, st.Owner, "approval",
				fmt.Sprintf("routing override (S08.8 re-route/pin): %s (pin=%v)", st.Routing.PlainReason, st.Routing.Pinned),
				"approval card routing block (S08.8; override recorded with its actor)", 0); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("%w: a route override rides approve, replan, or reinterview", ErrBadAnswer)
		}
	}

	switch ans.Action {
	case ActionCompose:
		// The no-fit compose verb (Spec S08.6 compose-when-earned): valid
		// only while the card offers it. The card stays OPEN — the
		// requester still owes the approval decision; the composition
		// ceremony runs as its own billed run (launched by the stage layer
		// from the recorded request), its product entering service only
		// through the battery + station-4 approval.
		if st.Routing == nil || !st.Routing.ComposeEarned || st.Routing.GapSignature == "" {
			return nil, fmt.Errorf("%w: compose is offered only when the gap has earned it (S08.6)", ErrBadAnswer)
		}
		if st.Compose != nil {
			return nil, fmt.Errorf("%w: a composition for this task was already requested", ErrBadAnswer)
		}
		if _, err := p.Ledger.RecordDecision(ctx, st.RunID, ledger.AuthorHuman, st.Owner, "approval",
			fmt.Sprintf("requester chose compose-a-worker on the no-fit card (gap %s)", st.Routing.GapSignature),
			"compose-when-earned (S08.6); the ceremony bills to the requester, the battery still gates adoption", 0); err != nil {
			return nil, err
		}
		st.Compose = &ComposeState{
			RequestedBy:  st.Owner,
			RequestedTS:  p.nowRFC3339(),
			GapSignature: st.Routing.GapSignature,
		}
		if err := p.appendState(ctx, st); err != nil {
			return nil, err
		}
		return st, nil
	case ActionApprove:
		pair, err := p.ensurePair(st, nil)
		if err != nil {
			return nil, err
		}
		if err := p.approve(ctx, st, pair, st.Owner, false, raw); err != nil {
			return nil, err
		}
		return st, nil
	case ActionRePlan:
		targets, findings, err := replanContest(ans)
		if err != nil {
			return nil, err
		}
		contested := "the plan in their own words"
		if len(targets) > 0 {
			contested = strings.Join(targets, ", ")
		}
		if _, err := p.Ledger.RecordDecision(ctx, st.RunID, ledger.AuthorHuman, st.Owner, "approval",
			"requester contested "+contested+" via Re-plan", "approval card (S06.9)", 0); err != nil {
			return nil, err
		}
		st.PendingRevise = mergeRevise(st.PendingRevise, &ReviseReq{Reason: ReviseContest, Findings: findings})
		st.Phase = PhaseSpine
	case ActionReInterview:
		if _, err := p.Ledger.RecordDecision(ctx, st.RunID, ledger.AuthorHuman, st.Owner, "approval",
			"requester chose Re-interview", "approval card (S06.9); artifacts stay intact", 0); err != nil {
			return nil, err
		}
		st.ReinterviewRequested = true
		st.ForceProceeded = false
		st.Phase = PhaseInterview
	case ActionCancel:
		if _, err := p.Ledger.RecordDecision(ctx, st.RunID, ledger.AuthorHuman, st.Owner, "approval",
			"requester cancelled the intake", "cancel is always available (4.5)", 0); err != nil {
			return nil, err
		}
		st.Phase = PhaseCancelled
	default:
		return nil, fmt.Errorf("%w: approval action %q", ErrBadAnswer, ans.Action)
	}
	if err := p.closeAndResume(ctx, st, st.OpenAskID, "answered", raw, ans.Note); err != nil {
		return nil, err
	}
	if st.Phase == PhaseCancelled {
		return st, nil
	}
	return p.advanceLoaded(ctx, st)
}

// replanContest folds the S06.9 structured Re-plan entry into the contested
// targets and the findings ONE bounded delta re-plan is asked to fix
// (P3-GF3-BE1 R10; design note §2.E).
//
// S06.9's "tap the AC, assumption, or step being contested" is a structure, not
// a cardinality of one: a requester who finds three things wrong with a plan
// should say all three in one send and get one revision, instead of three
// rounds of re-plan and re-approval. The plural findings this produces are the
// SAME `ReviseReq.Findings` the landed single contest produced, so nothing
// downstream changes: one Revise call, spine re-run, no second paid critique.
//
// The legacy single `contest` keeps its exact behavior. The optional top-level
// note is the requester's own words about the plan as a whole, and it may stand
// ALONE (§11 OQ-4): a person who cannot point at the step that is wrong, only
// at the result, has still said something a bounded re-plan can act on. What is
// refused is an empty send, which asks for a paid revision that names nothing.
func replanContest(ans Answer) (targets, findings []string, err error) {
	refs := make([]ContestRef, 0, len(ans.Contests)+1)
	if ans.Contest != nil {
		refs = append(refs, *ans.Contest)
	}
	refs = append(refs, ans.Contests...)
	for _, c := range refs {
		if strings.TrimSpace(c.Target) == "" {
			return nil, nil, fmt.Errorf("%w: a contested item needs the target it names (S06.9 structured entry)", ErrBadAnswer)
		}
		targets = append(targets, c.Target)
		finding := c.Target
		if c.Note != "" {
			finding += ": " + c.Note
		}
		findings = append(findings, finding)
	}
	if note := strings.TrimSpace(ans.Note); note != "" {
		findings = append(findings, note)
	}
	if len(findings) == 0 {
		return nil, nil, fmt.Errorf("%w: re-plan needs the contested target (S06.9 structured entry)", ErrBadAnswer)
	}
	return targets, findings, nil
}

// ---- Approve (Spec S06.9 "On Approve" + S02.8 claims) ----

// approve freezes the ACs under their numbers, pins objective + frozen ACs
// + constraints/danger zones into the ledger (G1 Def.12), records the
// spec+plan version for the freshness fingerprint, writes the approval
// record, writes the S02.8 write-set claim, and freezes the artifact
// files. Every ledger pin is conditional on its current value, so a
// crash-interrupted approval re-runs idempotently.
func (p *Pipeline) approve(ctx context.Context, st *State, pair *Pair, approver string, ungated bool, raw json.RawMessage) error {
	if len(pair.Spec.Clarifications) > 0 {
		return ErrMarkersOpen
	}
	specVer := pair.Spec.VersionKey()
	doc, found, err := p.Ledger.Current(ctx, st.TaskID)
	if err != nil {
		return err
	}

	author := ledger.AuthorHuman
	actor := approver
	if ungated {
		author = ledger.AuthorPlatform
		actor = run.ActorPlatform
	}

	// Pin §1 (objective + frozen ACs) — later verification judges against
	// exactly these numbers (5.4; Spec S07).
	if !found || doc.SpecVersion != specVer {
		acs := make([]ledger.AcceptanceCriterion, 0, len(pair.Spec.ACs))
		for _, ac := range pair.Spec.ACs {
			acs = append(acs, ledger.AcceptanceCriterion{N: ac.N, Plain: ac.Plain, Structured: ac.Structured})
		}
		if _, err := p.Ledger.SetObjective(ctx, st.RunID, actor, ledger.ObjectiveAC{
			Objective:          pair.Spec.Restatement,
			AcceptanceCriteria: acs,
			SpecVersion:        specVer,
		}); err != nil {
			return err
		}
		doc, _, _ = p.Ledger.Current(ctx, st.TaskID)
	}

	// Pin §2 (constraints + registry danger-zone snapshot).
	if doc.Constraints.SpecVersion != specVer {
		var zones []ledger.DangerZone
		if st.Registry != nil {
			for _, z := range st.Registry.DangerZones {
				zones = append(zones, ledger.DangerZone{Path: z.Path, Rule: z.Rule, SourceHash: z.SourceHash})
			}
		}
		if _, err := p.Ledger.SetConstraints(ctx, st.RunID, actor, ledger.ConstraintsDangerZones{
			Constraints: pair.Spec.Constraints,
			DangerZones: zones,
			SpecVersion: specVer,
		}); err != nil {
			return err
		}
	}

	// The spec+plan version joins the freshness fingerprint (G1 Def.5).
	spv := pair.Plan.SpecPlanVersion()
	if doc.PlanVersion != spv {
		if _, err := p.Ledger.SetPlanVersion(ctx, st.RunID, actor, spv); err != nil {
			return err
		}
	}

	// Freeze the artifacts: approved renderings are NEW immutable files
	// (draft files never mutate — the crash window between file writes and
	// the closing transaction stays consistent).
	store := p.store()
	pair.Spec.Status, pair.Plan.Status = StatusApproved, StatusApproved
	specRef, err := store.write(approvedPath(store.specPath(st.TaskID, pair.Spec.Version)), renderSpecMD(&pair.Spec), &pair.Spec, pair.Spec.Version)
	if err != nil {
		return err
	}
	planRef, err := store.write(approvedPath(store.planPath(st.TaskID, pair.Plan.Version)), renderPlanMD(&pair.Plan), &pair.Plan, pair.Plan.Version)
	if err != nil {
		return err
	}

	// Approval record → ledger §3 (S06.1 Stage 4: who/when/card version).
	note := "gated"
	if ungated {
		note = "zero-interaction band, ungated (S06.4)"
	}
	if _, err := p.Ledger.RecordDecision(ctx, st.RunID, author, actor, "approval",
		fmt.Sprintf("plan approved (%s): %s; spec sha256=%s; plan sha256=%s", note, spv, specRef.SHA256, planRef.SHA256),
		fmt.Sprintf("approval record: card v%d (S06.9)", st.CardVersion), 0); err != nil {
		return err
	}

	// Claims + ask close + resume + terminal state — one transaction. The
	// collision decision entry is recorded after commit (the ledger opens
	// its own transaction; the single-connection pool forbids nesting).
	askID := st.OpenAskID
	st.SpecRef, st.PlanRef = &specRef, &planRef
	st.Approval = &ApprovalRecord{
		Who: approver, When: p.nowRFC3339(), CardVersion: st.CardVersion,
		Choices: raw, Ungated: ungated,
	}
	st.Phase = PhaseApproved
	st.OpenAskID, st.OpenAskKind = "", ""
	var collisionHolder string
	err = p.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		holder, err := p.writeClaimsTx(ctx, tx, st, &pair.Plan)
		if err != nil {
			return err
		}
		collisionHolder = holder
		if askID != "" {
			if err := p.closeAskTx(ctx, tx, askID, "answered", raw); err != nil {
				return err
			}
			r, err := p.Runs.GetTx(ctx, tx, st.RunID)
			if err != nil {
				return err
			}
			if r.State == run.StateParked {
				// Approval is a ratified resume trigger (Spec S02.3); the
				// resume bumps the generation. What runs next is B2-4's.
				if _, err := p.Runs.TransitionTx(ctx, tx, st.RunID, run.StateRunning, run.TransitionOptions{
					Reason: "plan approved: pipeline resumes (S02.3 approval resume)", Actor: run.ActorPlatform,
				}); err != nil {
					return err
				}
			}
		}
		return p.appendStateTx(ctx, tx, st)
	})
	if err != nil {
		return err
	}
	// The approval resume hands the run onward rather than driving it inline,
	// so there is no advance to bracket — but the resume still has to leave a
	// LIVE lease behind it, or the next sweep meets an un-parked run holding
	// the expired claim lease and reads it as death (R5). One fenced beat at
	// the resumed generation does exactly that.
	p.Leases.Beat(ctx, st.RunID, leaseHolderIntake)
	if collisionHolder != "" {
		if _, err := p.Ledger.RecordDecision(ctx, st.RunID, ledger.AuthorPlatform, run.ActorPlatform, "approval",
			fmt.Sprintf("write-set overlaps active claim of task %s: sequenced behind the holder (S02.8)", collisionHolder),
			"claim collision surfaced at plan time (S1.11)", 0); err != nil {
			return err
		}
	}
	return nil
}

func approvedPath(draftPath string) string {
	return draftPath[:len(draftPath)-len(".md")] + ".approved.md"
}

// writeClaimsTx writes the S02.8 write-set claim at plan approval: the
// declared write-set (paths/globs) and optional read-set into
// artifact_claims. A plan that cannot bound its write-set claims per
// ⚙ claims.default_write — whole-project serializes more, never
// overwrites. Overlap with an active claim in the same project surfaces
// NOW (status waiting; the returned holder gets a decision entry after
// commit), at plan time, never by silent overwrite; the queueing itself
// is the scheduler's.
func (p *Pipeline) writeClaimsTx(ctx context.Context, tx *sql.Tx, st *State, plan *Plan) (string, error) {
	globs, unbounded := plan.WriteGlobs()
	if unbounded {
		mode, err := p.Settings.String(keyClaimsDefaultWrite)
		if err != nil {
			return "", fmt.Errorf("intake: read ⚙ %s: %w", keyClaimsDefaultWrite, err)
		}
		// declared-set trusts the declared globs where any exist; an
		// unbounded plan with nothing declared has no boundable set either
		// way and takes the whole project.
		if mode == "whole-project" || len(globs) == 0 {
			globs = []string{"**"}
		}
	}
	project := ""
	if st.Registry != nil {
		project = st.Registry.Project
	}
	status := "active"
	var collisionHolder string
	if len(globs) > 0 {
		holder, overlap, err := p.claimOverlapTx(ctx, tx, st.TaskID, project, globs)
		if err != nil {
			return "", err
		}
		if overlap {
			status = "waiting"
			collisionHolder = holder
		}
		id, err := p.insertClaimTx(ctx, tx, st, plan, project, globs, "W", status)
		if err != nil {
			return "", err
		}
		st.ClaimIDs = append(st.ClaimIDs, id)
		st.ClaimStatus = status
	}
	if reads := plan.ReadGlobs(); len(reads) > 0 {
		id, err := p.insertClaimTx(ctx, tx, st, plan, project, reads, "R", "active")
		if err != nil {
			return "", err
		}
		st.ClaimIDs = append(st.ClaimIDs, id)
	}
	return collisionHolder, nil
}

func (p *Pipeline) insertClaimTx(ctx context.Context, tx *sql.Tx, st *State, plan *Plan, project string, globs []string, mode, status string) (int64, error) {
	raw, err := json.Marshal(globs)
	if err != nil {
		return 0, fmt.Errorf("intake: marshal claim globs: %w", err)
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO artifact_claims (task_id, project, user_id, path_globs, mode, declared_at_plan, status, created_ts)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		st.TaskID, project, st.Owner, string(raw), mode, plan.VersionKey(), status, p.nowRFC3339())
	if err != nil {
		return 0, fmt.Errorf("intake: insert claim: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("intake: claim id: %w", err)
	}
	return id, nil
}

// claimOverlapTx glob-intersects against active write claims of OTHER
// tasks in the same project (Spec S02.8).
func (p *Pipeline) claimOverlapTx(ctx context.Context, tx *sql.Tx, taskID, project string, globs []string) (string, bool, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT task_id, path_globs FROM artifact_claims
		  WHERE project = ? AND mode = 'W' AND status = 'active' AND task_id <> ?`,
		project, taskID)
	if err != nil {
		return "", false, fmt.Errorf("intake: read claims: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var holder, rawGlobs string
		if err := rows.Scan(&holder, &rawGlobs); err != nil {
			return "", false, fmt.Errorf("intake: scan claim: %w", err)
		}
		var held []string
		if err := json.Unmarshal([]byte(rawGlobs), &held); err != nil {
			return "", false, fmt.Errorf("intake: decode claim globs: %w", err)
		}
		if anyGlobsIntersect(globs, held) {
			return holder, true, nil
		}
	}
	return "", false, rows.Err()
}

// ---- Cancel / tier / staleness ----

// Cancel cancels the intake (4.5: cancel is always available). The intake
// run finalizes (parked, via closeAndResume) or completes (running).
//
// reason is the requester's own words for why, empty when they gave none. The
// verb is UNROUTED today — nothing in production calls it, so no affordance can
// supply one — and it takes the parameter regardless, because mint parity is
// the one constructor's contract: all four cancel paths hand it the same shape,
// and a path that could not carry a motive would be the one that quietly serves
// a blank why the day it is routed (P3-RW-19 R13/OQ4).
func (p *Pipeline) Cancel(ctx context.Context, actor, taskID, reason string) (*State, error) {
	st, err := p.LoadState(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if actor != st.Owner {
		return nil, fmt.Errorf("%w: %q", ErrNotRequester, actor)
	}
	if st.Phase == PhaseApproved || st.Phase == PhaseCancelled {
		return nil, fmt.Errorf("%w: %s", ErrPhase, st.Phase)
	}
	if _, err := p.Ledger.RecordDecision(ctx, st.RunID, ledger.AuthorHuman, st.Owner, string(st.Phase),
		"requester cancelled the intake", "cancel is always available (4.5)", 0); err != nil {
		return nil, err
	}
	st.Phase = PhaseCancelled
	if st.OpenAskID != "" {
		if err := p.closeAndResume(ctx, st, st.OpenAskID, "cancelled", nil, reason); err != nil {
			return nil, err
		}
		return st, nil
	}
	err = p.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		r, err := p.Runs.GetTx(ctx, tx, st.RunID)
		if err != nil {
			return err
		}
		if r.State == run.StateRunning {
			// 15.6 attribution, as in closeAndResume's cancel leg: the requester
			// who cancelled is the actor, and the shared structured detail is
			// what the S02.2 reconstruction reads (P3-RW-18 D1-R1). No card was
			// open on this leg, so no ask id rides along.
			if _, err := p.Runs.TransitionTx(ctx, tx, st.RunID, run.StateCompleted, run.TransitionOptions{
				Reason: "intake cancelled (4.5)", Actor: st.Owner,
				Detail: run.CancelDetail(st.Owner, false, "", reason),
			}); err != nil {
				return err
			}
		}
		return p.appendStateTx(ctx, tx, st)
	})
	if err != nil {
		return nil, err
	}
	return st, nil
}

// LowerTier is the one downward tier move: an explicit requester action,
// never automatic, and never below a deterministic floor (Spec S06.4).
func (p *Pipeline) LowerTier(ctx context.Context, actor, taskID string, to Tier) (*State, error) {
	st, err := p.LoadState(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if actor != st.Owner {
		return nil, fmt.Errorf("%w: %q", ErrNotRequester, actor)
	}
	if !ValidTier(to) {
		return nil, fmt.Errorf("%w: tier %q", ErrBadAnswer, to)
	}
	if st.Phase == PhaseApproved || st.Phase == PhaseCancelled {
		return nil, fmt.Errorf("%w: %s", ErrPhase, st.Phase)
	}
	if st.FloorTier != "" && tierRank[to] < tierRank[st.FloorTier] {
		return nil, fmt.Errorf("%w: floor %s (%s)", ErrBelowFloor, st.FloorTier, floorClasses(st.FloorReasons))
	}
	if tierRank[to] >= tierRank[st.Tier] {
		return nil, fmt.Errorf("%w: %q does not lower %q", ErrBadAnswer, to, st.Tier)
	}
	if to == TierTrivial {
		// The band is rule-decided, never re-entered by hand (S06.4).
		return nil, fmt.Errorf("%w: the zero-interaction band is rule-decided (S06.4)", ErrBadAnswer)
	}
	st.Tier = to
	if _, err := p.Ledger.RecordDecision(ctx, st.RunID, ledger.AuthorHuman, st.Owner, string(st.Phase),
		"requester lowered the stakes tier to "+string(to), "explicit requester action (S06.4)", 0); err != nil {
		return nil, err
	}
	return st, p.appendState(ctx, st)
}

func floorClasses(reasons []FloorReason) string {
	seen := map[string]bool{}
	out := ""
	for _, r := range reasons {
		if seen[r.Class] {
			continue
		}
		seen[r.Class] = true
		if out != "" {
			out += ","
		}
		out += r.Class
	}
	return out
}

// CheckApprovalStale runs the S06.9 pending-approval freshness check: a
// card pending past ⚙ freshness.max_age (the folded alias of
// intake.approval_stale_hours, S18.4 R2), or whose fingerprint drifted,
// or whose project accepted a sibling task, is flagged "assumptions may be
// stale" with a one-click re-plan. The flag never blocks approving.
func (p *Pipeline) CheckApprovalStale(ctx context.Context, taskID string, siblingAccept bool) (*State, error) {
	st, err := p.LoadState(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if st.OpenAskKind != CardApproval || st.OpenAskID == "" {
		return nil, fmt.Errorf("%w: no pending approval card", ErrPhase)
	}
	stored := run.Fingerprint{}
	current := run.Fingerprint{}
	if st.StoredFingerprint != nil {
		stored = *st.StoredFingerprint
		current = stored // fingerprint drift only measurable with a probe
	}
	if p.Fingerprint != nil {
		if fp, err := p.Fingerprint(ctx, projectOf(st.Registry)); err == nil {
			fp.SpecPlanVersion = stored.SpecPlanVersion
			current = fp
		}
	}
	// Cited project-truth entries (Spec S09.6, R32): the probe cannot know a
	// plan's citations, so the SAME cited keys' current versions resolve
	// through the dedicated seam. A superseded/removed/retired entry no longer
	// resolves active → its key vanishes from the current map → mapDrift fires
	// "no longer observable". A nil seam is not measurable → carry stored
	// forward (no false drift), the same posture as the base probe.
	current.CitedEntryVersions = stored.CitedEntryVersions
	if len(stored.CitedEntryVersions) > 0 && p.CitedEntryVersions != nil {
		keys := make([]string, 0, len(stored.CitedEntryVersions))
		for k := range stored.CitedEntryVersions {
			keys = append(keys, k)
		}
		if cur, err := p.CitedEntryVersions(ctx, keys); err == nil {
			current.CitedEntryVersions = cur
		}
	}
	issued, err := parseRFC3339(st.CardIssuedTS)
	if err != nil {
		return nil, err
	}
	fresh, err := run.EvaluateFreshness(p.Settings, run.FreshnessInput{
		CheckpointTime: issued,
		Stored:         stored,
		Current:        current,
		SiblingAccept:  siblingAccept,
	}, p.now())
	if err != nil {
		return nil, err
	}
	if fresh.Fresh {
		return st, nil
	}
	st.StaleFlag, st.StaleReasons = true, fresh.Reasons
	card, _, err := p.openCard(ctx, st.OpenAskID)
	if err != nil {
		return nil, err
	}
	if card.Approval != nil {
		card.Approval.StaleFlag = true
		card.Approval.StaleReasons = fresh.Reasons
	}
	err = p.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		if err := p.updateAskSnapshotTx(ctx, tx, st.OpenAskID, card); err != nil {
			return err
		}
		return p.appendStateTx(ctx, tx, st)
	})
	if err != nil {
		return nil, err
	}
	return st, nil
}

func mergeRevise(cur, add *ReviseReq) *ReviseReq {
	if cur == nil {
		return add
	}
	cur.Reason = add.Reason
	cur.Findings = append(cur.Findings, add.Findings...)
	cur.Resolutions = append(cur.Resolutions, add.Resolutions...)
	return cur
}

func parseRFC3339(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("intake: parse card timestamp: %w", err)
	}
	return t, nil
}
