package intake

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/ledger"
)

// PlanSource is the S06-owned implementation of the ledger assembly's
// Plan seam (Spec S05.4 Sources.Plan): stage instructions from the
// approved plan. Selection is deterministic over platform-owned facts —
// the task id and the stage name; a stage matching a plan step key gets
// that step's contract, and every stage gets the plan frame.
type PlanSource struct {
	P *Pipeline
}

var _ ledger.Source = (*PlanSource)(nil)

// Items returns the approved plan's items for the queried stage. Tasks
// without an approved plan contribute nothing (the seam is optional by
// contract).
func (s *PlanSource) Items(ctx context.Context, q ledger.SliceQuery) ([]ledger.Item, error) {
	st, err := s.P.LoadState(ctx, q.TaskID)
	if errors.Is(err, ErrNoState) {
		return nil, nil // no intake state — nothing to contribute
	}
	if err != nil {
		return nil, err
	}
	if st.Phase != PhaseApproved {
		return nil, nil
	}
	pair, err := s.P.loadPair(st)
	if err != nil {
		return nil, err
	}
	version := pair.Plan.SpecPlanVersion()

	var items []ledger.Item
	items = append(items, ledger.Item{
		ItemID:       "plan-frame",
		SourcePath:   st.PlanRef.Path,
		Content:      planFrame(&pair.Plan),
		Version:      version,
		SelectorRule: "approved plan for task " + q.TaskID,
		Precedence:   ledger.PrecedenceTask,
	})
	// A sub-stage of an executed stage split (Spec S05.3) receives its
	// PLANNED stage's step contract: the split never changes what the plan
	// assigned, only how many sessions carry it.
	if step := pair.Plan.Step(ledger.PlannedStage(q.Stage)); step != nil {
		items = append(items, ledger.Item{
			ItemID:       "plan-step-" + step.ID,
			SourcePath:   st.PlanRef.Path,
			Content:      stepContract(step, &pair.Plan),
			Version:      version,
			SelectorRule: fmt.Sprintf("plan step %s matches stage %q", step.ID, q.Stage),
			Precedence:   ledger.PrecedenceStage,
		})
	}
	return items, nil
}

func planFrame(p *Plan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Approved plan %s (%d steps):\n", p.VersionKey(), len(p.Steps))
	for i, s := range p.Steps {
		fmt.Fprintf(&b, "%d. %s: %s\n", i+1, s.ID, s.Title)
	}
	return b.String()
}

func stepContract(s *Step, p *Plan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Current step %s: %s\n", s.ID, s.Title)
	fmt.Fprintf(&b, "Done when: %s\n", s.DoneWhen)
	fmt.Fprintf(&b, "Confinement class: %s (declared at plan time; helpers inherit tighter-only, P-T05-1)\n", s.Class)
	if len(s.WriteSet) > 0 {
		fmt.Fprintf(&b, "Declared write-set: %s\n", strings.Join(s.WriteSet, ", "))
	}
	var acs []string
	for _, key := range sortedACKeys(p.Coverage) {
		for _, sid := range p.Coverage[key] {
			if sid == s.ID {
				acs = append(acs, key)
				break
			}
		}
	}
	if len(acs) > 0 {
		fmt.Fprintf(&b, "Delivers: %s\n", strings.Join(acs, ", "))
	}
	return b.String()
}
