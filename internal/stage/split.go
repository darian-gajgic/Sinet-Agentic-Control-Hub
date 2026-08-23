package stage

import (
	"context"
	"fmt"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// Stage-split execution (Spec S05.3): the overflow→split contract landed.
// The budget watcher emits the context.overflow proposal (B2-4); a
// stage-split proposal is auto-accepted — risk-free platform-internal
// scheduling, fully logged — and executes at the next checkpoint boundary:
// consolidate-to-ledger (stage-close gate) → end session → successor
// sub-stage brief from the UPDATED ledger, with the stage-fit budget
// re-checked per sub-stage. This file owns the execute-leg realization; the
// session-level pause machinery is runner.go's.

// runPlannedStage runs one plan step's stage sessions (Spec S05.3
// execute-N): normally one fresh session; when an overflow stage-split
// executes, the planned stage continues in successor sub-stage sessions.
// Overflow scope is the PLANNED stage ("a second overflow within one
// planned stage escalates to a re-plan proposal"): the first crossing
// proposes and executes the split; a successor sub-stage carries the prior
// count, so its own crossing emits the re-plan proposal and never splits
// again — the loop is structurally bounded at one split per planned stage.
func (s *Skeleton) runPlannedStage(ctx context.Context, r run.Run, er executeRouting, step intake.Step) (SessionResult, error) {
	prior := 0
	for sub := 1; ; sub++ {
		stageName := ledger.SubStageName(step.ID, sub)
		in := SessionInput{
			RunID:          r.ID,
			Stage:          stageName,
			Assemble:       true,
			Sources:        ledger.Sources{Plan: &intake.PlanSource{P: s.pipe}, Knowledge: s.cfg.Knowledge},
			Instructions:   executeInstructions(r.TaskID, step, stageName, sub),
			Kind:           markerExecute,
			Class:          step.Class,
			Tools:          execTools,
			PermissionMode: execPermissionMode,
			PriorOverflows: prior,
			Model:          er.Decision.Model,
			Lane:           er.Decision.Lane,
			WindowTokens:   er.Decision.WindowTokens,
		}
		if er.Decision.TemplateID != "" && s.cfg.Workers != nil {
			// Worker-backed execution: per-invocation hash-pinned compile
			// (Spec S08.3 via CompileForRun) — fresh for EVERY sub-stage
			// session, instance refs carrying the concrete sub-stage. The
			// session runs at the TIGHTER of the step's declared class and
			// the worker's granted class — neither the approved plan
			// envelope nor the approved guardrails ever widen.
			compiled, err := s.cfg.Workers.CompileForRun(ctx, er.Decision.TemplateID, r.UserID, worker.InstanceRefs{
				RunID: r.ID, TaskID: r.TaskID, Stage: stageName,
			})
			if err != nil {
				return SessionResult{}, fmt.Errorf("compile worker %s: %w", er.Decision.TemplateID, err)
			}
			in.Compiled = &compiled
			in.Class = worker.TighterClass(step.Class, compiled.Class)
			in.Tools = nil // the compiled unit carries the granted toolset
			in.PermissionMode = ""
		}
		// The S10.4 pause boundary INSIDE a planned stage (pause.go): a stage
		// that splits runs several sessions, and each successor boundary is a
		// place the run can park. Without this, a long split chain would ignore
		// the switch until the whole step finished.
		if s.pauseParkPoint(ctx, r, stageName) {
			return SessionResult{}, errPausedPark
		}
		res, err := s.Session(ctx, in)
		if err != nil {
			return res, err
		}
		if res.Budget.Overflowed {
			prior++
		}
		if !res.Split {
			return res, nil
		}
		if err := s.consolidateSplit(ctx, r, step.ID, stageName, sub, res); err != nil {
			return res, err
		}
	}
}

// executeInstructions is the execute-session instruction block; a successor
// sub-stage (sub ≥ 2) gets the continuation frame — its brief already
// carries the consolidated ledger state, and the per-run workspace persists
// across the split.
func executeInstructions(taskID string, step intake.Step, stageName string, sub int) string {
	if sub <= 1 {
		return stageMarker(markerExecute) + fmt.Sprintf(
			"You are executing plan step %s of task %s: %s\n"+
				"Done when: %s\n"+
				"Work in your working directory. When the step is complete, output the step's "+
				"complete deliverable content as your final message — full content, no commentary wrapper.\n",
			step.ID, taskID, step.Title, step.DoneWhen)
	}
	return stageMarker(markerExecute) + fmt.Sprintf(
		"You are continuing plan step %s of task %s after a stage split (sub-stage %s): %s\n"+
			"Earlier work of this step is consolidated in the ledger sections above and persists in your working directory.\n"+
			"Done when: %s\n"+
			"Work in your working directory. When the step is complete, output the step's "+
			"complete deliverable content as your final message — full content, no commentary wrapper.\n",
		step.ID, taskID, stageName, step.Title, step.DoneWhen)
}

// consolidateSplit is the ledger half of an executed stage split (Spec
// S05.3 "consolidate-to-ledger (stage-close gate) → end session → successor
// stage brief from the updated ledger"): the platform records the
// acceptance as a §3 decision (author: platform — the fully-logged
// auto-accept), hands the in-flight work item forward to the successor
// sub-stage through state.next_actions (the one ledger structure naming
// successor work, Spec S05.1), and enforces the stage-close gate on the
// ended sub-stage. The successor's brief then assembles from this updated
// ledger at its bumped ledger_version — real sub-stage briefs via the
// ledger. (At v0 the platform holds the session verbs — the engine-facing
// verb TOOL channel is later wiring, CONVENTIONS §14 — so consolidation
// writes are platform acts on the session's stage.)
func (s *Skeleton) consolidateSplit(ctx context.Context, r run.Run, stepID, stageName string, sub int, res SessionResult) error {
	successor := ledger.SubStageName(stepID, sub+1)
	if _, err := s.cfg.Ledger.RecordDecision(ctx, r.ID, ledger.AuthorPlatform, run.ActorPlatform, stageName,
		fmt.Sprintf("stage split executed: session %s ended at its checkpoint boundary; step %s continues as %s",
			stageName, stepID, successor),
		fmt.Sprintf("compaction.anomaly stage-split auto-accepted (S05.3; event seq %d)", res.Budget.OverflowSeq),
		0); err != nil {
		return fmt.Errorf("record stage split: %w", err)
	}
	verbs := s.cfg.Ledger.SessionVerbs(r.ID, stageName, r.Generation)
	next := []string{stepID}
	doc, err := verbs.State(ctx, ledger.StateUpdate{
		Upserts:     []ledger.WorkItem{{ID: stepID, Status: ledger.StatusInProgress}},
		NextActions: &next,
	})
	if err != nil {
		return fmt.Errorf("hand split step forward: %w", err)
	}
	if err := ledger.CheckStageClose(doc, []string{stepID}); err != nil {
		return fmt.Errorf("split stage-close gate: %w", err)
	}
	return nil
}
