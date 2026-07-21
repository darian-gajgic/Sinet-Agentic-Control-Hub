package intake

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// Pipeline is the S06 intake pipeline over the landed substrate. All model
// duties ride seams (S06.10); everything else is platform code. The
// pipeline operates on one intake run per task: stage work requires the
// run to be running (admission is the scheduler's, Spec S10; dev-mode
// harnesses walk the FSM), gates park it, answers resume it in place.
type Pipeline struct {
	DB       *storage.DB
	Log      *eventlog.Log
	Runs     *run.Store
	Ledger   *ledger.Store
	Settings Settings

	// ArtifactRoot is the durable artifact directory (per-task subdirs).
	ArtifactRoot string

	// Taxonomies are the active per-family question sets; families without
	// one fall back to FamilyGeneric (Spec S06.2: v0 ships software +
	// generic). Nil = SeedTaxonomies().
	Taxonomies map[Family]*Taxonomy
	// Triggers is the P47 rule file. Nil = SeedTriggers().
	Triggers *TriggerFile

	// Seams (Spec S06.10). Planner is required; Critic is required from
	// the standard tier up (critique is mandatory there — ErrSeamMissing
	// otherwise); the rest are optional.
	Planner    Planner
	Critic     Critic
	Classifier Classifier
	Registry   Registry
	Utility    Utility
	SpotCheck  SpotCheck

	// Router is the S08.8 selection seam (B3-3): the approval card carries
	// the selected worker + plain-language reason PRE-execution, with
	// re-route/pin on the answer (route.go). Nil = test-only posture (no
	// routing block; the composition root always wires it).
	Router Router

	// Fingerprint supplies the current freshness fingerprint for the
	// approval staleness check (G1 Def.5; S02 owns the durable set).
	// Optional; nil restricts staleness to the age trigger.
	Fingerprint func(ctx context.Context) (run.Fingerprint, error)

	// Now is the clock seam (tests). Timestamps are recorded, never an
	// ordering authority (P-T07-4).
	Now func() time.Time
}

func (p *Pipeline) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

func (p *Pipeline) nowRFC3339() string { return p.now().UTC().Format(time.RFC3339Nano) }

func (p *Pipeline) store() artifactStore { return artifactStore{root: p.ArtifactRoot} }

func (p *Pipeline) taxonomies() map[Family]*Taxonomy {
	if p.Taxonomies != nil {
		return p.Taxonomies
	}
	return SeedTaxonomies()
}

func (p *Pipeline) triggers() *TriggerFile {
	if p.Triggers != nil {
		return p.Triggers
	}
	return SeedTriggers()
}

func (p *Pipeline) taxonomyFor(family Family) *Taxonomy {
	tax := p.taxonomies()
	if t, ok := tax[family]; ok {
		return t
	}
	return tax[FamilyGeneric]
}

// ---- Start: Stage 0 (Spec S06.2) ----

// Start receives a request: it creates the task and its intake run,
// runs triage (deterministic rules first, local classifier second,
// fail-closed), persists the intake record, and emits the Stage-0 ledger
// writes. No paid model runs here.
//
// Stage-0 floor observation: the spec pins the five floor classes, not a
// request-text cue lexicon — Stage-0 floors arrive from the registry and
// the classifier (add-only), and Stage 2(b) re-checks deterministically
// against the PLAN's declared contents, with D7 gating every outward
// effect regardless of what intake decided (Spec S06.2).
func (p *Pipeline) Start(ctx context.Context, req Request) (*State, error) {
	if req.UserID == "" {
		return nil, fmt.Errorf("intake: request without user (15.6 owner attribution)")
	}
	if req.Text == "" {
		return nil, fmt.Errorf("intake: empty request")
	}
	if req.TaskID == "" {
		req.TaskID = "t-" + randomHex(8)
	}

	st := &State{
		Phase:      PhaseInterview,
		TaskID:     req.TaskID,
		RunID:      req.TaskID + ".intake",
		Owner:      req.UserID,
		Req:        req,
		NeedsDraft: true,
		Tier:       TierHigh, // fail-closed baseline until classified (S06.2)
		Family:     FamilyGeneric,
		Guess:      Estimate{Known: false, Basis: "unclassified"},
	}

	// Registry match (S1.6): injected so the interview never asks what the
	// platform already knows; danger zones feed the stakes floor.
	if p.Registry != nil {
		if slice, ok, err := p.Registry.Match(ctx, req); err == nil && ok {
			st.Registry = &slice
			if slice.Family != "" {
				st.Family = slice.Family
			}
		}
	}

	// Deterministic P47 rule layer FIRST (S06.3).
	st.addHits(p.triggers().Detect(req.Title + "\n" + req.Text))

	// Local classifier second — proposals only; failure fails closed to
	// high stakes (S06.2).
	classified := false
	if p.Classifier != nil {
		if prop, err := p.Classifier.Classify(ctx, req, st.Registry); err == nil {
			classified = true
			if prop.Family != "" {
				st.Family = prop.Family
			}
			if ValidTier(prop.Tier) {
				st.Tier = prop.Tier
			}
			st.Guess = prop.Est
			st.addHits(prop.DataHits) // add-only
			st.addFloors(validFloors(prop.Floors))
			// Zero-interaction band (S06.4): all four conditions,
			// evaluated conservatively; the cost bound is per-user ⚙.
			if st.FloorTier == "" && prop.Tier == TierTrivial && prop.ReadOnly && !prop.NewNeeds && len(st.openDataBearing()) == 0 {
				if capUSD, err := p.Settings.FloatFor(keyZeroInteractionCost, st.Owner); err == nil && prop.Est.Known && prop.Est.USD < capUSD {
					st.Band = true
				}
			}
			if prop.Tier == TierTrivial && !st.Band {
				// Trivial IS the band; a trivial proposal failing the band
				// conditions lands at low (S06.4).
				st.Tier = maxTier(TierLow, st.FloorTier)
			}
		}
	}
	if !classified {
		st.Tier = TierHigh
	}

	tax := p.taxonomyFor(st.Family)
	st.TaxonomyID, st.TaxonomyVersion = tax.ID, tax.Version

	// Registry-supplied slots resolve before any question is asked.
	if st.Registry != nil {
		for slotID, val := range st.Registry.ResolvedSlots {
			if tax.Slot(slotID) != nil {
				st.resolveSlot(SlotResolution{SlotID: slotID, How: ResolvedRegistry, Value: val})
			}
		}
	}
	st.Clearance = tax.Clearance(st.resolvedSet())

	// Intake record — the durable Stage-0 artifact (S06.1 Stage 0).
	record := map[string]any{
		"family": st.Family, "stakes_tier": st.Tier, "floor_reasons": st.FloorReasons,
		"size_cost_guess": st.Guess, "data_bearing": st.DataBearing,
		"registry_slice_ref": registryRef(st.Registry), "band": st.Band,
		"taxonomy": st.TaxonomyID + "@" + st.TaxonomyVersion, "ts": p.nowRFC3339(),
	}
	recordJSON, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("intake: marshal intake record: %w", err)
	}
	ref, err := p.store().write(p.store().recordPath(st.TaskID), append(recordJSON, '\n'), nil, 1)
	if err != nil {
		return nil, err
	}
	st.RecordRef = &ref

	// Task + run + first state event, one transaction: the task exists in
	// the D7 record from its first instant (CONVENTIONS §8 precedent).
	err = p.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO tasks (task_id, user_id, title, kanban_status, created_ts)
			 VALUES (?, ?, ?, 'intake', ?)`,
			st.TaskID, st.Owner, req.Title, p.nowRFC3339()); err != nil {
			return fmt.Errorf("intake: insert task: %w", err)
		}
		if _, err := p.Runs.CreateTx(ctx, tx, run.NewRun{
			ID: st.RunID, UserID: st.Owner, TaskID: st.TaskID,
		}, run.EventCreated, json.RawMessage(`{"reason":"intake run (S06.1)"}`)); err != nil {
			return err
		}
		return p.appendStateTx(ctx, tx, st)
	})
	if err != nil {
		return nil, err
	}

	// Stage-0 ledger emissions (S06.1: intake record → artifacts +
	// decisions). Sequential after the birth transaction; the state event
	// and record file are the durable Stage-0 record either way.
	gen, err := p.currentGen(ctx, st.RunID)
	if err != nil {
		return nil, err
	}
	verbs := p.Ledger.SessionVerbs(st.RunID, "triage", gen)
	if _, err := verbs.Artifact(ctx, ref.Path, "intake-record", "Stage-0 intake record (S06.2)", ref.SHA256); err != nil {
		return nil, err
	}
	if _, err := p.Ledger.RecordDecision(ctx, st.RunID, ledger.AuthorPlatform, run.ActorPlatform, "triage",
		fmt.Sprintf("triage: family=%s tier=%s band=%v data-bearing=%d clearance=%.0f%%",
			st.Family, st.Tier, st.Band, len(st.DataBearing), st.Clearance),
		"Stage-0 triage classification (S06.2)", 0); err != nil {
		return nil, err
	}
	return st, nil
}

func validFloors(in []FloorReason) []FloorReason {
	valid := map[string]bool{
		FloorOutwardEffect: true, FloorNewSpend: true, FloorCredentialTouch: true,
		FloorSharedAssetWrite: true, FloorRegulatedDomain: true,
	}
	var out []FloorReason
	for _, f := range in {
		if valid[f.Class] {
			out = append(out, f)
		}
	}
	return out
}

func registryRef(r *RegistrySlice) string {
	if r == nil {
		return ""
	}
	return r.Ref
}

// ---- Advance: the stage driver ----

// Advance drives the pipeline until it blocks on a gate or reaches a
// terminal phase, and returns the current state. Stage work requires the
// intake run to be running (ErrNotRunning otherwise).
func (p *Pipeline) Advance(ctx context.Context, taskID string) (*State, error) {
	st, err := p.LoadState(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return p.advanceLoaded(ctx, st)
}

func (p *Pipeline) advanceLoaded(ctx context.Context, st *State) (*State, error) {
	if st.Phase == PhaseApproved || st.Phase == PhaseCancelled {
		return st, nil
	}
	if st.OpenAskID != "" {
		return st, nil // a gate is open — gates wait (S06.1)
	}
	r, err := p.Runs.Get(ctx, st.RunID)
	if err != nil {
		return nil, err
	}
	if r.State != run.StateRunning {
		return nil, fmt.Errorf("%w: run %s is %s", ErrNotRunning, st.RunID, r.State)
	}

	var pair *Pair
	for guard := 0; guard < 64; guard++ {
		var blocked bool
		var err error
		switch st.Phase {
		case PhaseInterview:
			blocked, pair, err = p.phaseInterview(ctx, st, pair)
		case PhaseSpine:
			blocked, pair, err = p.phaseSpine(ctx, st, pair)
		case PhaseCritique:
			blocked, pair, err = p.phaseCritique(ctx, st, pair)
		case PhaseApproval:
			blocked, pair, err = p.phaseApproval(ctx, st, pair)
		default:
			return st, nil
		}
		if err != nil {
			return nil, err
		}
		if blocked || st.Phase == PhaseApproved || st.Phase == PhaseCancelled {
			return st, nil
		}
	}
	return nil, fmt.Errorf("intake: pipeline did not settle (programming error)")
}

// phaseInterview runs the Stage-1 interview loop and drafting (Spec
// S06.5–S06.6).
func (p *Pipeline) phaseInterview(ctx context.Context, st *State, pair *Pair) (bool, *Pair, error) {
	tax := p.taxonomyFor(st.Family)
	resolved := st.resolvedSet()
	st.Clearance = tax.Clearance(resolved)

	// Explicit Re-interview (S06.9): one full card of the top-weight
	// slots, answered values overwrite.
	if st.ReinterviewRequested {
		st.ReinterviewRequested = false
		qs := make([]Question, 0, maxQuestionsPerCard)
		for _, s := range tax.Slots {
			if len(qs) == maxQuestionsPerCard {
				break
			}
			qs = append(qs, Question{ID: s.ID, Text: s.Question, Options: s.Options, Weight: s.Weight})
		}
		return true, pair, p.issueCard(ctx, st, &Card{Kind: CardInterview, Questions: qs})
	}

	// The interview: continues while Clearance is below the tier floor and
	// unresolved slots remain; no fixed question caps (G1 P8). The band
	// never interviews (S06.4); force-proceed stops it.
	if !st.Band && !st.ForceProceeded {
		floor, err := p.clearanceFloor(st.Tier)
		if err != nil {
			return false, pair, err
		}
		unresolved := tax.Unresolved(resolved)
		if st.Clearance < floor && len(unresolved) > 0 {
			qs := make([]Question, 0, maxQuestionsPerCard)
			for _, s := range unresolved {
				if len(qs) == maxQuestionsPerCard {
					break
				}
				qs = append(qs, Question{ID: s.ID, Text: s.Question, Options: s.Options, Weight: s.Weight})
			}
			return true, pair, p.issueCard(ctx, st, &Card{Kind: CardInterview, Questions: qs})
		}
	}

	if !st.NeedsDraft {
		// Pair exists (TIER-UP re-entry or re-interview): floor satisfied
		// again — re-verify through the spine.
		st.Phase = PhaseSpine
		return false, pair, p.appendState(ctx, st)
	}

	// Unresolved slots convert to explicit assumptions where the spec says
	// so: the band auto-lists all of them; force-proceed converts all of
	// them; a floor reached leaves the low-weight tail to the planner's
	// own listed assumptions (S06.4, S06.5).
	if st.Band || st.ForceProceeded {
		origin := "band"
		if st.ForceProceeded {
			origin = "force_proceed"
		}
		for _, s := range tax.Unresolved(resolved) {
			st.resolveSlot(SlotResolution{
				SlotID: s.ID, How: ResolvedAssumption,
				Assumption: fmt.Sprintf("(%s) %s — assumed: proceeding without an answer", origin, s.Name),
			})
		}
		st.Clearance = tax.Clearance(st.resolvedSet())
	}

	// Draft (Stage-1 emission).
	in := DraftInput{
		Request: st.Req, Family: st.Family, Tier: st.Tier,
		Registry: st.Registry, Taxonomy: tax,
		Resolutions: st.Resolutions, DataHits: st.openDataBearing(),
		Supplied: st.Supplied, Escalations: st.Escalations,
		SpecVersion: st.SpecVersion + 1, PlanVersion: st.PlanVersion + 1,
	}
	if st.SpecRef != nil {
		prior, err := p.loadPair(st)
		if err != nil {
			return false, pair, err
		}
		in.Prior = prior
	}
	next, err := p.Planner.Draft(ctx, in)
	if blocked, herr := p.handleEscalation(ctx, st, err, "draft"); blocked || herr != nil {
		return blocked, pair, herr
	}
	newPair, err := p.acceptEmission(ctx, st, in.Prior, &next)
	if err != nil {
		return false, pair, err
	}
	st.NeedsDraft = false
	st.Phase = PhaseSpine
	return false, newPair, p.appendState(ctx, st)
}

// phaseSpine applies pending revisions and runs the Stage-2 deterministic
// spine (Spec S06.7).
func (p *Pipeline) phaseSpine(ctx context.Context, st *State, pair *Pair) (bool, *Pair, error) {
	pair, err := p.ensurePair(st, pair)
	if err != nil {
		return false, pair, err
	}

	if st.PendingRevise != nil {
		req := *st.PendingRevise
		in := ReviseInput{
			Pair: *pair, Reason: req.Reason, Findings: req.Findings,
			Resolutions: req.Resolutions, Escalations: st.Escalations,
			SpecVersion: st.SpecVersion + 1, PlanVersion: st.PlanVersion + 1,
		}
		next, err := p.Planner.Revise(ctx, in)
		if blocked, herr := p.handleEscalation(ctx, st, err, "revise"); blocked || herr != nil {
			return blocked, pair, herr
		}
		pair, err = p.acceptEmission(ctx, st, pair, &next)
		if err != nil {
			return false, pair, err
		}
		st.PendingRevise = nil
		if err := p.appendState(ctx, st); err != nil {
			return false, pair, err
		}
	}

	// Advisory semantic spot-check (S06.7(a)) — advisory-only.
	var advisory []string
	if p.SpotCheck != nil {
		if notes, err := p.SpotCheck.Check(ctx, *pair); err == nil {
			advisory = notes
		} else {
			advisory = []string{"spot-check unavailable: " + err.Error()}
		}
	}

	res, err := p.runSpine(ctx, st, pair, advisory)
	if err != nil {
		return false, pair, err
	}
	st.Spine = res
	if _, err := p.Ledger.RecordDecision(ctx, st.RunID, ledger.AuthorPlatform, run.ActorPlatform, "spine",
		"spine: "+res.summary(), "Stage-2 deterministic spine (S06.7)", 0); err != nil {
		return false, pair, err
	}

	// (a) Coverage: bounded auto-fix, then a decision card — an agreed
	// criterion never disappears silently.
	if len(res.Uncovered) > 0 {
		rounds, err := p.Settings.Int(keyCoverageAutofixRounds)
		if err != nil {
			return false, pair, fmt.Errorf("intake: read ⚙ %s: %w", keyCoverageAutofixRounds, err)
		}
		if int64(st.CoverageRounds) < rounds {
			st.CoverageRounds++
			st.PendingRevise = &ReviseReq{Reason: ReviseCoverage, Findings: res.Uncovered}
			return false, pair, p.appendState(ctx, st)
		}
		card := &Card{Kind: CardCoverage, Decision: &DecisionBody{
			Summary: fmt.Sprintf("The plan does not cover: %s. Auto-fix rounds are exhausted (⚙ %s).", joinKeys(res.Uncovered), keyCoverageAutofixRounds),
			Detail:  res.Uncovered,
			Choices: []Option{
				{Label: "Re-plan once more", Value: ChoiceReplan},
				{Label: "Drop the criterion (recorded, visible)", Value: ChoiceDropCriterion},
				{Label: "Proceed with the gap listed on the approval card", Value: ChoiceProceedUncovered},
			},
			Help: HelpBlock{
				What:      "An agreed acceptance criterion has no plan step delivering it.",
				Wrong:     "Proceeding with the gap means that criterion will not be worked on or verified.",
				Recommend: "Re-plan once more; drop the criterion only if you no longer want it.",
			},
		}}
		return true, pair, p.issueCard(ctx, st, card)
	}

	// (d) Research-node presence: bounce once, then card (S06.7(d)).
	if len(res.MissingResearch) > 0 {
		if !st.ResearchBounced {
			st.ResearchBounced = true
			st.PendingRevise = &ReviseReq{Reason: ReviseResearch, Findings: res.MissingResearch}
			return false, pair, p.appendState(ctx, st)
		}
		card := &Card{Kind: CardResearch, Decision: &DecisionBody{
			Summary: "The task depends on facts in the world, but the plan still lacks required research steps for: " + joinKeys(res.MissingResearch),
			Detail:  res.MissingResearch,
			Choices: []Option{
				{Label: "I'll supply the fact myself", Value: ChoiceSupplyFact},
				{Label: "Re-plan to add the research step", Value: ChoiceReplan},
			},
			Help: HelpBlock{
				What:      "Live research is required by policy for data-bearing tasks (1.9).",
				Wrong:     "Without it the result may rest on stale or invented facts.",
				Recommend: "Let the plan research it — or supply the fact if you already own it.",
			},
		}}
		return true, pair, p.issueCard(ctx, st, card)
	}

	// Open NEEDS-CLARIFICATION markers cannot reach approval (S06.6).
	if len(res.OpenMarkers) > 0 {
		qs := make([]Question, 0, maxQuestionsPerCard)
		for i, m := range res.OpenMarkers {
			if len(qs) == maxQuestionsPerCard {
				break
			}
			qs = append(qs, Question{ID: fmt.Sprintf("marker-%d", i+1), Text: m})
		}
		return true, pair, p.issueCard(ctx, st, &Card{Kind: CardClarification, Questions: qs})
	}

	if tierRank[st.Tier] >= tierRank[TierStandard] && !st.CritiqueDone {
		st.Phase = PhaseCritique
	} else {
		st.Phase = PhaseApproval
	}
	return false, pair, p.appendState(ctx, st)
}

// phaseCritique is Stage 3 (Spec S06.8): fresh-context self-attack,
// artifact-only input, mandatory from the standard tier up.
func (p *Pipeline) phaseCritique(ctx context.Context, st *State, pair *Pair) (bool, *Pair, error) {
	if tierRank[st.Tier] < tierRank[TierStandard] || st.CritiqueDone {
		st.Phase = PhaseApproval
		return false, pair, p.appendState(ctx, st)
	}
	if p.Critic == nil {
		return false, pair, fmt.Errorf("%w: critique is mandatory at %s tier (S06.4) and no Critic is configured", ErrSeamMissing, st.Tier)
	}
	pair, err := p.ensurePair(st, pair)
	if err != nil {
		return false, pair, err
	}

	var v Verdict
	if len(st.OpenFindings) > 0 {
		// Re-critique checks only the named findings (S06.8 REVISE).
		v, err = p.Critic.Recheck(ctx, *pair, st.OpenFindings)
	} else {
		v, err = p.Critic.Critique(ctx, *pair)
	}
	if blocked, herr := p.handleEscalation(ctx, st, err, "critique"); blocked || herr != nil {
		return blocked, pair, herr
	}

	round := len(st.Verdicts) + 1
	st.Verdicts = append(st.Verdicts, VerdictRecord{
		Round: round, Kind: v.Kind, Findings: v.Findings, Doubt: v.Doubt,
		Proposed: v.ProposedTier, TS: p.nowRFC3339(),
	})
	// The verdict is written to the ledger; the findings artifact joins §5
	// (S06.1 Stage 3: decisions + artifacts).
	md := renderCritiqueMD(st.TaskID, st.PlanVersion, round, v)
	ref, err := p.store().write(p.store().critiquePath(st.TaskID, st.PlanVersion, round), md, nil, round)
	if err != nil {
		return false, pair, err
	}
	gen, err := p.currentGen(ctx, st.RunID)
	if err != nil {
		return false, pair, err
	}
	verbs := p.Ledger.SessionVerbs(st.RunID, "critique", gen)
	if _, err := verbs.Artifact(ctx, ref.Path, "critique", fmt.Sprintf("Stage-3 critique round %d (S06.8)", round), ref.SHA256); err != nil {
		return false, pair, err
	}
	if _, err := verbs.Decide(ctx,
		fmt.Sprintf("critique verdict: %s (round %d, %d findings)", v.Kind, round, len(v.Findings)),
		"Stage-3 plan self-attack (S06.8)", 0); err != nil {
		return false, pair, err
	}

	switch v.Kind {
	case VerdictPass:
		st.CritiqueDone = true
		st.OpenFindings = nil
		st.Phase = PhaseApproval

	case VerdictRevise:
		rounds, err := p.Settings.Int(keyCritiqueReviseRounds)
		if err != nil {
			return false, pair, fmt.Errorf("intake: read ⚙ %s: %w", keyCritiqueReviseRounds, err)
		}
		if int64(st.CritiqueRounds) < rounds {
			st.CritiqueRounds++
			st.OpenFindings = v.Findings
			st.PendingRevise = &ReviseReq{Reason: ReviseCritique, Findings: v.Findings}
			st.Phase = PhaseSpine // the spine runs after every revision (S06.7)
		} else {
			// One pass, never loops: proceed with the critique attached —
			// the open findings ride the approval card (S06.8).
			st.CritiqueDone = true
			st.OpenFindings = v.Findings
			st.Phase = PhaseApproval
		}

	case VerdictSpecDoubt:
		// Mandatory decision card — never absorbed (S06.8, P45).
		card := &Card{Kind: CardSpecDoubt, Decision: &DecisionBody{
			Summary: "The critique concludes the specification itself may be a bad idea: " + v.Doubt,
			Choices: []Option{
				{Label: "Proceed anyway", Value: ChoiceProceedAnyway},
				{Label: "Adjust the spec", Value: ChoiceAdjustSpec},
				{Label: "Rethink (cancel this intake)", Value: ChoiceRethink},
			},
			Help: HelpBlock{
				What:      "A fresh reviewer doubts the goal as specified, not just the plan.",
				Wrong:     "Proceeding anyway may execute a well-built plan for the wrong goal.",
				Recommend: "Read the doubt; adjust the spec if it rings true.",
			},
		}}
		return true, pair, p.issueCard(ctx, st, card)

	case VerdictTierUp:
		raised := ValidTier(v.ProposedTier) && tierRank[v.ProposedTier] > tierRank[st.Tier]
		if raised {
			st.Tier = maxTier(st.Tier, v.ProposedTier)
			st.exitBand("TIER-UP (S06.8)")
			st.CritiqueDone = false
			st.OpenFindings = nil
			st.Phase = PhaseInterview // re-enter at the new tier's requirements
		} else {
			// A TIER-UP that cannot raise (already at the proposed tier or
			// higher) degenerates to proceed-with-verdict-attached; the
			// requester sees it on the card.
			st.CritiqueDone = true
			st.Phase = PhaseApproval
		}

	default:
		return false, pair, fmt.Errorf("intake: unknown verdict %q", v.Kind)
	}
	return false, pair, p.appendState(ctx, st)
}

// phaseApproval is Stage 4 (Spec S06.9).
func (p *Pipeline) phaseApproval(ctx context.Context, st *State, pair *Pair) (bool, *Pair, error) {
	pair, err := p.ensurePair(st, pair)
	if err != nil {
		return false, pair, err
	}
	if len(pair.Spec.Clarifications) > 0 {
		return false, pair, ErrMarkersOpen
	}
	// S08.8 selection runs BEFORE the card is built so the selected worker
	// and its plain-language reason are visible pre-execution (the no-fit
	// two-stage offer rides the same card; a pinned selection survives
	// re-planning recomputes).
	if err := p.computeRouting(ctx, st, pair); err != nil {
		return false, pair, err
	}
	if st.Band {
		// Zero-interaction band: the pipeline still ran — SPEC and
		// degenerate PLAN exist in the ledger, ungated; no blocking gate,
		// completion card only (delivered at completion, S06.4).
		if err := p.approve(ctx, st, pair, run.ActorPlatform, true, nil); err != nil {
			return false, pair, err
		}
		return false, pair, nil
	}
	card, err := p.buildApprovalCard(ctx, st, pair)
	if err != nil {
		return false, pair, err
	}
	if p.Fingerprint != nil {
		if fp, err := p.Fingerprint(ctx); err == nil {
			fp.SpecPlanVersion = pair.Plan.SpecPlanVersion()
			st.StoredFingerprint = &fp
		}
	}
	return true, pair, p.issueCard(ctx, st, card)
}

// ---- Emission acceptance (draft/revise) ----

// acceptEmission validates a Stage-1 emission, assigns versions (a spec
// version moves only when spec content changed — stable keys, no drift),
// guarantees converted-slot assumptions are listed, persists the artifact
// files, and appends the ledger §5 refs via the stage session verbs.
func (p *Pipeline) acceptEmission(ctx context.Context, st *State, prior *Pair, next *Pair) (*Pair, error) {
	// The platform guarantees requester-supplied inputs are recorded on
	// the SPEC (S06.3), regardless of what the model listed.
	for _, f := range st.Supplied {
		found := false
		for _, have := range next.Spec.Supplied {
			if have.RuleID == f.RuleID && have.Fact == f.Fact {
				found = true
				break
			}
		}
		if !found {
			next.Spec.Supplied = append(next.Spec.Supplied, f)
		}
	}

	// The platform guarantees visibility of converted slots regardless of
	// what the model listed (S06.5 force-proceed contract).
	tax := p.taxonomyFor(st.Family)
	for _, r := range st.Resolutions {
		if r.How != ResolvedAssumption {
			continue
		}
		origin := "slot:" + r.SlotID
		found := false
		for _, a := range next.Spec.Assumptions {
			if a.Origin == origin {
				found = true
				break
			}
		}
		if !found {
			text := r.Assumption
			if s := tax.Slot(r.SlotID); s != nil && text == "" {
				text = s.Name + ": assumed"
			}
			next.Spec.Assumptions = append(next.Spec.Assumptions, Assumption{Text: text, Origin: origin})
		}
	}

	// Version assignment: plan bumps on every emission; spec bumps only on
	// content change.
	specV, planV := st.SpecVersion+1, st.PlanVersion+1
	if prior != nil && specContentEqual(&prior.Spec, &next.Spec) {
		specV = prior.Spec.Version
	}
	next.Spec.TaskID, next.Spec.Owner = st.TaskID, st.Owner
	next.Plan.TaskID, next.Plan.Owner = st.TaskID, st.Owner
	next.Spec.Version, next.Spec.Status, next.Spec.Tier = specV, StatusDraft, st.Tier
	next.Plan.Version, next.Plan.SpecVersion, next.Plan.Status, next.Plan.Tier = planV, specV, StatusDraft, st.Tier
	if err := next.Validate(); err != nil {
		return nil, err
	}

	store := p.store()
	specRef, err := store.write(store.specPath(st.TaskID, specV), renderSpecMD(&next.Spec), &next.Spec, specV)
	if err != nil {
		return nil, err
	}
	planRef, err := store.write(store.planPath(st.TaskID, planV), renderPlanMD(&next.Plan), &next.Plan, planV)
	if err != nil {
		return nil, err
	}
	st.SpecVersion, st.PlanVersion = specV, planV
	st.SpecRef, st.PlanRef = &specRef, &planRef

	// §5 artifact refs land through the plan stage session (S05.1 writers).
	gen, err := p.currentGen(ctx, st.RunID)
	if err != nil {
		return nil, err
	}
	verbs := p.Ledger.SessionVerbs(st.RunID, "plan", gen)
	if _, err := verbs.Artifact(ctx, specRef.Path, "spec", fmt.Sprintf("SPEC %s (S06.6)", next.Spec.VersionKey()), specRef.SHA256); err != nil {
		return nil, err
	}
	if _, err := verbs.Artifact(ctx, planRef.Path, "plan", fmt.Sprintf("PLAN %s (S06.6)", next.Plan.VersionKey()), planRef.SHA256); err != nil {
		return nil, err
	}
	return next, nil
}

// specContentEqual compares spec content modulo version/status.
func specContentEqual(a, b *Spec) bool {
	ca, cb := *a, *b
	ca.Version, cb.Version = 0, 0
	ca.Status, cb.Status = "", ""
	ca.Tier, cb.Tier = "", ""
	ja, _ := json.Marshal(ca)
	jb, _ := json.Marshal(cb)
	return string(ja) == string(jb)
}

// handleEscalation routes a seam call's Escalation into a single-question
// card (1.7: the run blocks-not-fails and resumes on answer). Returns
// (true, nil) when a card was issued; (false, err) on a real error;
// (false, nil) when there was no error at all.
func (p *Pipeline) handleEscalation(ctx context.Context, st *State, err error, from string) (bool, error) {
	if err == nil {
		return false, nil
	}
	var esc *Escalation
	if !errors.As(err, &esc) {
		return false, err
	}
	st.PendingEscalation = from
	card := &Card{Kind: CardEscalation, Questions: []Question{{ID: "escalation", Text: esc.Question}}}
	if ierr := p.issueCard(ctx, st, card); ierr != nil {
		return false, ierr
	}
	return true, nil
}

// ---- Cards & gates ----

// issueCard persists the ask row, parks the run, and appends the state —
// one transaction. Gates wait; answering resumes the pipeline in place
// (Spec S06.1).
func (p *Pipeline) issueCard(ctx context.Context, st *State, card *Card) error {
	st.CardVersion++
	askID := fmt.Sprintf("intake:%s:%d", st.TaskID, st.CardVersion)
	card.TaskID, card.RunID = st.TaskID, st.RunID
	card.Version = st.CardVersion
	card.IssuedTS = p.nowRFC3339()
	card.Clearance = st.Clearance
	card.Tier = st.Tier
	st.OpenAskID, st.OpenAskKind = askID, card.Kind
	st.CardIssuedTS = card.IssuedTS
	st.StaleFlag, st.StaleReasons = false, nil
	return p.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		if err := p.insertAskTx(ctx, tx, askID, st, card); err != nil {
			return err
		}
		if _, err := p.Runs.TransitionTx(ctx, tx, st.RunID, run.StateParked, run.TransitionOptions{
			Reason: fmt.Sprintf("intake gate open: %s — gates wait (S06.1)", card.Kind),
			Actor:  run.ActorPlatform,
		}); err != nil {
			return err
		}
		return p.appendStateTx(ctx, tx, st)
	})
}

// buildApprovalCard renders the S06.9 card content contract from the
// artifacts.
func (p *Pipeline) buildApprovalCard(ctx context.Context, st *State, pair *Pair) (*Card, error) {
	help := defaultHelp(pair)
	if p.Utility != nil {
		if h, err := p.Utility.Help(ctx, *pair); err == nil {
			help = h
		}
	}
	steps := make([]string, 0, len(pair.Plan.Steps))
	for _, s := range pair.Plan.Steps {
		steps = append(steps, s.Title)
	}
	costTime := "UNPRICED (no price table; measured usage will appear on the receipt)"
	if pair.Plan.Est.Known {
		costTime = fmt.Sprintf("~%.2f USD (API-equivalent, D5)", pair.Plan.Est.USD)
	}
	l1 := ApprovalLayer1{
		Restatement: pair.Spec.Restatement,
		Deliverable: pair.Spec.Outcome,
		Steps:       steps,
		WillNotDo:   pair.Spec.OutOfScope,
		Assumptions: pair.Spec.Assumptions, // the centerpiece (R03 §2.4)
		Risks:       pair.Plan.Risks,
		CostTime:    costTime,
		Clearance:   st.Clearance,
		SizeClass:   pair.Plan.Est.SizeClass,
		Help:        help,
		Uncovered:   st.AcceptedUncovered,
		OpenFinds:   st.OpenFindings,
	}
	if st.Spine != nil && st.Spine.SizeFinding != "" {
		l1.SizeNote = st.Spine.SizeDetail // stakes-gated display: non-trivial shows it (2.5)
	}
	l2 := ApprovalLayer2{
		ACs: pair.Spec.ACs, Steps: pair.Plan.Steps, Coverage: pair.Plan.Coverage,
		Verdicts: st.Verdicts, ResearchNodes: pair.Plan.ResearchNodes,
		Estimate: pair.Plan.Est, SpecRef: st.SpecRef, PlanRef: st.PlanRef,
	}
	actions := []string{ActionApprove, ActionRePlan, ActionReInterview, ActionCancel}
	if st.Routing != nil && st.Routing.ComposeEarned && st.Compose == nil {
		// The no-fit stage-2 compose offer is an ANSWERABLE verb exactly
		// when composition is earned (Spec S08.6 compose-when-earned) and
		// no composition ran for this task yet.
		actions = append(actions, ActionCompose)
	}
	return &Card{Kind: CardApproval, Approval: &ApprovalBody{
		Layer1: l1,
		Layer2: l2,
		// The S08.8 selection block: worker + plain-language reason, the
		// re-route candidates, and the no-fit two-stage offer — visible and
		// overridable pre-execution.
		Routing: st.Routing,
		Actions: actions,
	}}, nil
}

// ---- Shared helpers ----

func (p *Pipeline) clearanceFloor(t Tier) (float64, error) {
	var key string
	switch t {
	case TierLow:
		key = keyClearanceFloorLow
	case TierStandard:
		key = keyClearanceFloorStd
	case TierHigh:
		key = keyClearanceFloorHigh
	default:
		return 0, nil // trivial: no interview (S06.4)
	}
	v, err := p.Settings.Int(key)
	if err != nil {
		return 0, fmt.Errorf("intake: read ⚙ %s: %w", key, err)
	}
	return float64(v), nil
}

// ensurePair loads the current pair from the durable artifacts when the
// caller does not hold it (resume-from-artifact, Spec S06.1).
func (p *Pipeline) ensurePair(st *State, pair *Pair) (*Pair, error) {
	if pair != nil {
		return pair, nil
	}
	return p.loadPair(st)
}

func (p *Pipeline) loadPair(st *State) (*Pair, error) {
	spec, err := p.store().loadSpec(st.SpecRef)
	if err != nil {
		return nil, err
	}
	plan, err := p.store().loadPlan(st.PlanRef)
	if err != nil {
		return nil, err
	}
	return &Pair{Spec: spec, Plan: plan}, nil
}

// CurrentPair loads a task's CURRENT drafted artifact pair regardless of
// approval — the read seam for consumers of the specification content
// itself (the S08.6 composer's task-spec policy input reads it while the
// approval card is still open). Nothing EXECUTES from this read (D10:
// execution still demands ApprovedPair); the artifact loads carry the same
// sha256 + re-render integrity checks.
func (p *Pipeline) CurrentPair(ctx context.Context, taskID string) (*Pair, error) {
	st, err := p.LoadState(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return p.loadPair(st)
}

// ApprovedPair loads a task's APPROVED artifact pair plus its pipeline
// state — the read seam execution and verification build from (B2-4
// walking skeleton): work proceeds from the durable approved artifacts,
// never from conversation (D7; Spec S06.1), and the load re-verifies
// sha256 + byte-identical re-render (resume-from-artifact integrity, Spec
// S06.6). ErrPhase when no approved plan exists — D10: nothing executes
// unapproved.
func (p *Pipeline) ApprovedPair(ctx context.Context, taskID string) (Pair, *State, error) {
	st, err := p.LoadState(ctx, taskID)
	if err != nil {
		return Pair{}, nil, err
	}
	if st.Phase != PhaseApproved {
		return Pair{}, nil, fmt.Errorf("%w: task %s is %s, not approved (D10)", ErrPhase, taskID, st.Phase)
	}
	pair, err := p.loadPair(st)
	if err != nil {
		return Pair{}, nil, err
	}
	return *pair, st, nil
}

func (p *Pipeline) currentGen(ctx context.Context, runID string) (int64, error) {
	r, err := p.Runs.Get(ctx, runID)
	if err != nil {
		return 0, err
	}
	return r.Generation, nil
}

func joinKeys(keys []string) string {
	out := ""
	for i, k := range keys {
		if i > 0 {
			out += ", "
		}
		out += k
	}
	return out
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failure is unrecoverable
	}
	return hex.EncodeToString(b)
}
