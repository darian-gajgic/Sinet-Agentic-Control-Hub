package intake

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/ledger"
)

// Delta re-approval (Spec S06.9): every post-approval change to SPEC or
// PLAN — freshness re-validation findings (4.3), sibling-collision
// re-plans (S1.11), contested-card fixes, confinement widening — is
// expressed as an ADDED / MODIFIED / REMOVED delta against the frozen
// artifacts and approved on a delta-only card showing exactly what
// changed. A silently disappearing criterion is structurally impossible:
// removing an AC exists only as a REMOVED item on such a card, and the
// ledger's pinned §1 moves only under the new spec version this path
// assigns. Each card ships the measurement hook (P-T05-2): the event log
// records presented-delta size, time-to-decision, decision, and outcome
// linkage; analysis is the eval practice's (Spec S14).

const deltaDecisionSchemaVersion = 1

// diffPairs computes the delta items between the frozen pair and a
// proposed revision, deterministically on stable keys.
func diffPairs(frozen, next *Pair) []DeltaItem {
	var items []DeltaItem

	// ACs by stable key over the longer numbering.
	n := len(frozen.Spec.ACs)
	if len(next.Spec.ACs) > n {
		n = len(next.Spec.ACs)
	}
	for i := 1; i <= n; i++ {
		key := fmt.Sprintf("AC-%d", i)
		oldAC, newAC := frozen.Spec.AC(key), next.Spec.AC(key)
		switch {
		case oldAC == nil && newAC != nil:
			items = append(items, DeltaItem{Kind: DeltaAdded, Target: key, New: newAC.Plain})
		case oldAC != nil && newAC == nil:
			items = append(items, DeltaItem{Kind: DeltaRemoved, Target: key, Old: oldAC.Plain})
		case oldAC != nil && newAC != nil && (oldAC.Plain != newAC.Plain || oldAC.Structured != newAC.Structured):
			items = append(items, DeltaItem{Kind: DeltaModified, Target: key, Old: oldAC.Plain, New: newAC.Plain})
		}
	}

	// Steps by stable key.
	n = len(frozen.Plan.Steps)
	if len(next.Plan.Steps) > n {
		n = len(next.Plan.Steps)
	}
	for i := 1; i <= n; i++ {
		key := fmt.Sprintf("S-%d", i)
		oldS, newS := frozen.Plan.Step(key), next.Plan.Step(key)
		switch {
		case oldS == nil && newS != nil:
			items = append(items, DeltaItem{Kind: DeltaAdded, Target: key, New: newS.Title})
		case oldS != nil && newS == nil:
			items = append(items, DeltaItem{Kind: DeltaRemoved, Target: key, Old: oldS.Title})
		case oldS != nil && newS != nil:
			if oldS.Title != newS.Title || oldS.DoneWhen != newS.DoneWhen {
				items = append(items, DeltaItem{Kind: DeltaModified, Target: key, Old: oldS.Title, New: newS.Title})
			}
			if oldS.Class != newS.Class {
				// Confinement widening is a plan change requiring delta
				// re-approval (P-T05-1; Spec S06.6).
				items = append(items, DeltaItem{Kind: DeltaModified, Target: "confinement:" + key, Old: oldS.Class, New: newS.Class})
			}
		}
	}

	// Assumptions by text.
	oldSet := map[string]bool{}
	for _, a := range frozen.Spec.Assumptions {
		oldSet[a.Text] = true
	}
	newSet := map[string]bool{}
	for _, a := range next.Spec.Assumptions {
		newSet[a.Text] = true
		if !oldSet[a.Text] {
			items = append(items, DeltaItem{Kind: DeltaAdded, Target: "assumption", New: a.Text})
		}
	}
	for _, a := range frozen.Spec.Assumptions {
		if !newSet[a.Text] {
			items = append(items, DeltaItem{Kind: DeltaRemoved, Target: "assumption", Old: a.Text})
		}
	}
	return items
}

// ProposeDelta diffs a proposed revision against the frozen artifacts and
// issues the delta-only card. The run FSM is untouched — post-approval
// pause policy belongs to the machinery that produced the revision (4.3 /
// S1.11), not to intake.
func (p *Pipeline) ProposeDelta(ctx context.Context, taskID, origin string, next Pair) (*State, string, error) {
	st, err := p.LoadState(ctx, taskID)
	if err != nil {
		return nil, "", err
	}
	if st.Phase != PhaseApproved {
		return nil, "", fmt.Errorf("%w: deltas apply to approved plans (phase %s)", ErrPhase, st.Phase)
	}
	if st.OpenAskID != "" {
		return nil, "", fmt.Errorf("%w: %q", ErrGateOpen, st.OpenAskID)
	}
	frozen, err := p.loadPair(st)
	if err != nil {
		return nil, "", err
	}
	items := diffPairs(frozen, &next)
	if len(items) == 0 {
		return nil, "", fmt.Errorf("%w: proposed revision changes nothing", ErrBadAnswer)
	}

	// Stage the proposed pair as draft files at the next versions; they
	// freeze only on delta approval.
	next.Spec.TaskID, next.Spec.Owner = st.TaskID, st.Owner
	next.Plan.TaskID, next.Plan.Owner = st.TaskID, st.Owner
	next.Spec.Version, next.Spec.Status, next.Spec.Tier = st.SpecVersion+1, StatusDraft, st.Tier
	next.Plan.Version, next.Plan.SpecVersion, next.Plan.Status, next.Plan.Tier = st.PlanVersion+1, st.SpecVersion+1, StatusDraft, st.Tier
	if err := next.Validate(); err != nil {
		return nil, "", err
	}
	store := p.store()
	if _, err := store.write(store.specPath(st.TaskID, next.Spec.Version), renderSpecMD(&next.Spec), &next.Spec, next.Spec.Version); err != nil {
		return nil, "", err
	}
	if _, err := store.write(store.planPath(st.TaskID, next.Plan.Version), renderPlanMD(&next.Plan), &next.Plan, next.Plan.Version); err != nil {
		return nil, "", err
	}

	rawItems, err := json.Marshal(items)
	if err != nil {
		return nil, "", fmt.Errorf("intake: marshal delta items: %w", err)
	}
	st.CardVersion++
	askID := fmt.Sprintf("intake:%s:%d", st.TaskID, st.CardVersion)
	card := &Card{
		Kind: CardDelta, TaskID: st.TaskID, RunID: st.RunID,
		Version: st.CardVersion, IssuedTS: p.nowRFC3339(),
		Clearance: st.Clearance, Tier: st.Tier,
		Delta: &DeltaBody{
			Origin:  origin,
			Items:   items,
			Actions: DeltaActions(),
			Help: HelpBlock{
				What:      "The approved plan changed. Only the listed items differ; everything else stays exactly as approved.",
				Wrong:     "A REMOVED item disappears from the contract; a MODIFIED item changes what gets verified.",
				Recommend: "Read each line — the card shows the complete change. Approve to adopt it; reject to keep the frozen plan.",
			},
		},
	}
	st.Deltas = append(st.Deltas, DeltaRecord{
		ID: fmt.Sprintf("delta-%d", len(st.Deltas)+1), Origin: origin, Items: items,
		AskID: askID, PresentedBytes: len(rawItems), IssuedTS: card.IssuedTS,
		NewSpecVersion: next.Spec.Version, NewPlanVersion: next.Plan.Version,
	})
	st.OpenAskID, st.OpenAskKind = askID, CardDelta
	st.CardIssuedTS = card.IssuedTS
	err = p.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		if err := p.insertAskTx(ctx, tx, askID, st, card); err != nil {
			return err
		}
		return p.appendStateTx(ctx, tx, st)
	})
	if err != nil {
		return nil, "", err
	}
	return st, askID, nil
}

// applyDeltaAnswer decides a delta card and emits the measurement-hook
// event either way.
func (p *Pipeline) applyDeltaAnswer(ctx context.Context, st *State, ans Answer, raw json.RawMessage) (*State, error) {
	if len(st.Deltas) == 0 {
		return nil, fmt.Errorf("%w: no pending delta", ErrPhase)
	}
	rec := &st.Deltas[len(st.Deltas)-1]
	if rec.AskID != st.OpenAskID || rec.Decision != "" {
		return nil, fmt.Errorf("%w: %q", ErrUnknownAsk, st.OpenAskID)
	}
	askID := st.OpenAskID

	switch ans.Action {
	case ChoiceApproveDelta:
		next, err := p.loadStagedDelta(st, rec)
		if err != nil {
			return nil, err
		}
		st.SpecVersion, st.PlanVersion = rec.NewSpecVersion, rec.NewPlanVersion
		st.Phase = PhaseApproved // stays approved; approve() below re-pins and re-claims
		// Supersede the old claims; approve() writes fresh ones.
		if err := p.supersedeClaims(ctx, st); err != nil {
			return nil, err
		}
		specRef, err := p.store().write(p.store().specPath(st.TaskID, next.Spec.Version), renderSpecMD(&next.Spec), &next.Spec, next.Spec.Version)
		if err != nil {
			return nil, err
		}
		planRef, err := p.store().write(p.store().planPath(st.TaskID, next.Plan.Version), renderPlanMD(&next.Plan), &next.Plan, next.Plan.Version)
		if err != nil {
			return nil, err
		}
		st.SpecRef, st.PlanRef = &specRef, &planRef
		st.OpenAskID = "" // approve() must not treat the delta ask as an approval card
		if err := p.approve(ctx, st, next, st.Owner, false, raw); err != nil {
			return nil, err
		}
		rec.Decision = "approved"
	case ChoiceRejectDelta:
		if _, err := p.Ledger.RecordDecision(ctx, st.RunID, ledger.AuthorHuman, st.Owner, "approval",
			"delta rejected: the frozen plan stands", "delta-only card (S06.9)", 0); err != nil {
			return nil, err
		}
		rec.Decision = "rejected"
	default:
		return nil, fmt.Errorf("%w: delta action %q", ErrBadAnswer, ans.Action)
	}
	rec.DecidedTS = p.nowRFC3339()

	// Measurement hook (P-T05-2): presented size, time-to-decision,
	// decision, outcome linkage.
	issued, err := parseRFC3339(rec.IssuedTS)
	if err != nil {
		return nil, err
	}
	hook := map[string]any{
		"delta_id":           rec.ID,
		"origin":             rec.Origin,
		"presented_items":    len(rec.Items),
		"presented_bytes":    rec.PresentedBytes,
		"time_to_decision_s": p.now().Sub(issued).Seconds(),
		"decision":           rec.Decision,
		"task_id":            st.TaskID,
		"spec_plan_version":  fmt.Sprintf("spec-v%d/plan-v%d", st.SpecVersion, st.PlanVersion),
	}
	payload, err := json.Marshal(hook)
	if err != nil {
		return nil, fmt.Errorf("intake: marshal delta hook: %w", err)
	}
	err = p.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		var gen int64
		if err := tx.QueryRowContext(ctx, `SELECT generation FROM runs WHERE run_id = ?`, st.RunID).Scan(&gen); err != nil {
			return fmt.Errorf("intake: read run: %w", err)
		}
		if _, err := p.Log.AppendTx(ctx, tx, eventlog.Append{
			RunID: st.RunID, Generation: gen, UserID: st.Owner,
			Type: EventDeltaDecision, SchemaVersion: deltaDecisionSchemaVersion, Payload: payload,
		}); err != nil {
			return err
		}
		if rec.Decision == "rejected" {
			st.OpenAskID, st.OpenAskKind = "", ""
			if err := p.closeAskTx(ctx, tx, askID, "answered", raw); err != nil {
				return err
			}
		}
		return p.appendStateTx(ctx, tx, st)
	})
	if err != nil {
		return nil, err
	}
	if rec.Decision == "approved" {
		// approve() left the delta ask open (it only closes st.OpenAskID);
		// close it now.
		err = p.DB.WriteTx(ctx, func(tx *sql.Tx) error {
			return p.closeAskTx(ctx, tx, askID, "answered", raw)
		})
		if err != nil {
			return nil, err
		}
	}
	return st, nil
}

// loadStagedDelta reloads the staged (draft) delta pair by path.
func (p *Pipeline) loadStagedDelta(st *State, rec *DeltaRecord) (*Pair, error) {
	specPath := p.store().specPath(st.TaskID, rec.NewSpecVersion)
	planPath := p.store().planPath(st.TaskID, rec.NewPlanVersion)
	var spec Spec
	if err := readSidecar(specPath, &spec); err != nil {
		return nil, err
	}
	var plan Plan
	if err := readSidecar(planPath, &plan); err != nil {
		return nil, err
	}
	return &Pair{Spec: spec, Plan: plan}, nil
}

func readSidecar(mdPath string, into any) error {
	raw, err := os.ReadFile(sidecarPath(mdPath))
	if err != nil {
		return fmt.Errorf("intake: read staged sidecar: %w", err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return fmt.Errorf("%w: sidecar %s: %v", ErrBadArtifact, mdPath, err)
	}
	return nil
}

// supersedeClaims retires the previous plan version's claims; the fresh
// approval writes the new ones (S02.8: claims track the approved plan).
func (p *Pipeline) supersedeClaims(ctx context.Context, st *State) error {
	if len(st.ClaimIDs) == 0 {
		return nil
	}
	err := p.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		for _, id := range st.ClaimIDs {
			if _, err := tx.ExecContext(ctx,
				`UPDATE artifact_claims SET status = 'superseded' WHERE claim_id = ?`, id); err != nil {
				return fmt.Errorf("intake: supersede claim %d: %w", id, err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	st.ClaimIDs = nil
	st.ClaimStatus = ""
	return nil
}
