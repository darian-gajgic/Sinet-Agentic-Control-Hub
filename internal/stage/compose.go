package stage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/worker"
)

// The S08.6 composition leg: the compose verb answered on a task's no-fit
// card (intake.ActionCompose) launches a `<task>.compose` run — ceremony,
// billed and itemized to the requester (metering derives the purpose from
// the suffix) — whose dispatch runs the ONE-SHOT generation through the
// worker store's Compose verb and feeds the draft INTO the B3-2 battery:
// generation → CreateDraft (composer provenance) → four-station battery
// (schema lint, permission audit, sandboxed dry run through EngineDryRun,
// approval-as-diff via the store's card + Approve verbs). The battery
// gates adoption exactly as for a human-authored draft — the composer
// never sees its results.

// stage markers of the composition sessions (engines.go convention).
const (
	markerCompose = "compose"
	markerDryRun  = "dryrun"
)

// composeOutputSchema is the composer session's JSON output contract.
const composeOutputSchema = `Output EXACTLY one JSON object, nothing else, shaped:
{"template": "the FULL template file draft — markdown with YAML frontmatter per the schema reference",
 "grants": {"tools":[string...], "class":"C0"|"C1"|"C2", "egress":"none"|"registries"|"single-host",
   "egress_hosts":[string...], "gated_tools":[string...], "permission_mode":string,
   "budget_usd":number, "budget_steps":number},
 "sample_task": "one representative dry-run sample task",
 "golden_note": "what a correct outcome of the sample looks like"}
Rules: the template file carries BEHAVIOR only — a guardrail-class field in it is a
structural reject; grants are REQUESTS the permission audit and a human approval
decide; request the smallest set that serves the recurring work.`

// EngineComposer is the production worker.ComposeEngine: ONE generation
// ceremony session on the composition run, at the planning-model duty seat
// (Spec S08.6: drafting is heavyweight ceremony; the duty map resolves the
// class). The jsonSession transport bounce (jsonRetryLimit, the house
// rule) recovers a malformed ENVELOPE only — no draft is ever regenerated
// against validation results, which would be the banned refine loop.
type EngineComposer struct {
	s     *Skeleton
	runID string
}

var _ worker.ComposeEngine = (*EngineComposer)(nil)

func (c *EngineComposer) ComposeOnce(ctx context.Context, req worker.ComposeRequest) (worker.ComposeOutput, error) {
	gapJSON, err := json.MarshalIndent(req.Gap, "", "  ")
	if err != nil {
		return worker.ComposeOutput{}, fmt.Errorf("stage: marshal gap record: %w", err)
	}
	var b strings.Builder
	b.WriteString(stageMarker(markerCompose))
	b.WriteString("You are the worker composer (Spec S08.6): draft ONE worker template for a recurring task family in ONE pass.\n")
	b.WriteString("Your inputs are exactly the policy set below — the task spec + gap record, the composer playbook, the palette, and the platform reference (template schema, lint rules, ceiling table).\n")
	fmt.Fprintf(&b, "\n=== composer playbook (%s@v%d — the current approved version) ===\n%s\n",
		req.Playbook.EntryID, req.Playbook.Version, req.Playbook.Content)
	fmt.Fprintf(&b, "\n=== task spec (task %s) ===\n%s\n", req.TaskID, req.TaskSpec)
	fmt.Fprintf(&b, "\n=== gap record (the recurring no-fit evidence) ===\n%s\n", gapJSON)
	b.WriteString("\n=== palette (options, never defaults) ===\n")
	if len(req.Palette) == 0 {
		b.WriteString("(no palette bundles available)\n")
	} else {
		for _, p := range req.Palette {
			fmt.Fprintf(&b, "- %s: tools [%s]; %s\n", p.Name, strings.Join(p.Tools, ", "), p.Context)
		}
	}
	fmt.Fprintf(&b, "\n=== platform reference ===\n%s\n", req.Reference)
	b.WriteString("\n" + composeOutputSchema + "\n")

	var out worker.ComposeOutput
	if err := c.s.jsonSession(ctx, SessionInput{
		RunID:        c.runID,
		Stage:        "compose",
		Assemble:     false, // inputs by policy replace stage-brief assembly (Spec S08.6)
		Instructions: b.String(),
		Class:        "C1",
	}, &out); err != nil {
		return worker.ComposeOutput{}, fmt.Errorf("composer session: %w", err)
	}
	// The drafting model is a platform fact (the seat the ceremony ran on),
	// never trusted from model output (S08.1 provenance).
	out.Model = c.s.modelFor("")
	return out, nil
}

// EngineDryRun is the production worker.DryEngine (the B3-2 named seam):
// the station-3 witness run executes the COMPILED draft — requested grants
// as if granted, so the witness runs the real configuration — as one stage
// session on the composition run, in the effects-impossible class the
// battery computed (C1, or the requested class capped at C2), under the ⚙
// workers.dryrun_cost_cap_usd ceiling the compiled unit carries.
type EngineDryRun struct {
	s     *Skeleton
	runID string
}

var _ worker.DryEngine = (*EngineDryRun)(nil)

func (e *EngineDryRun) DryRun(ctx context.Context, req worker.DryRunRequest) (worker.DryRunResult, error) {
	compiled := req.Compiled
	res, err := e.s.Session(ctx, SessionInput{
		RunID:    e.runID,
		Stage:    "dryrun",
		Assemble: false,
		Instructions: stageMarker(markerDryRun) +
			"This is a sandboxed validation dry run (Spec S08.6 station 3). Perform the sample task; output the complete result as your final message.\n\n=== sample task ===\n" +
			req.SampleTask + "\n",
		Class:    req.Class,
		Compiled: &compiled,
	})
	if err != nil {
		var se *stageError
		if errors.As(err, &se) {
			// The witnessed run ended short of completion — a RED station 3,
			// not a mechanical platform error; the record carries it.
			return worker.DryRunResult{Completed: false, Detail: se.Error()}, nil
		}
		return worker.DryRunResult{}, err
	}
	dr := worker.DryRunResult{
		Completed: true,
		Output:    res.Text,
		// The transcript ref points at the run whose engine_sessions rows +
		// copy-aside hold the full witnessed transcript (refs-not-blobs).
		TranscriptRef: "run:" + e.runID + "/dryrun",
	}
	if res.Outcome.Totals != nil {
		dr.CostUSD = res.Outcome.Totals.EngineCostUSD
	}
	return dr, nil
}

// ensureComposeRun launches the task's composition run from a recorded
// compose request — exactly once per task (ErrExists tolerated; a row in
// any post-queued state means the ceremony already ran or runs). Enqueue is
// the sole ingress (Spec S16.6); the requester is waiting on the card, so
// the run rides the interactive class.
func (s *Skeleton) ensureComposeRun(ctx context.Context, st *intake.State) error {
	if s.sched == nil {
		return errors.New("stage: no scheduler bound (Bind was not called)")
	}
	runID := st.TaskID + RunSuffixCompose
	_, err := s.cfg.Runs.Create(ctx, run.NewRun{
		ID:        runID,
		UserID:    st.Compose.RequestedBy,
		TaskID:    st.TaskID,
		Substrate: s.cfg.Substrate,
		Lane:      s.cfg.Lane,
	})
	switch {
	case errors.Is(err, run.ErrExists):
		r, gerr := s.cfg.Runs.Get(ctx, runID)
		if gerr != nil {
			return gerr
		}
		if r.State != run.StateNew {
			return nil // already admitted (or already ran) — one composition per task
		}
	case err != nil:
		return fmt.Errorf("stage: create compose run: %w", err)
	}
	if err := s.sched.Enqueue(ctx, runID, scheduler.ClassInteractive); err != nil {
		return fmt.Errorf("stage: enqueue compose run: %w", err)
	}
	s.logger().Info("stage: launched composition run (S08.6)", "run", runID, "requester", st.Compose.RequestedBy)
	return nil
}

// dispatchCompose runs the composition ceremony: assemble the
// inputs-by-policy (task spec + gap record from the intake state; the
// composer playbook through the Config seam — the current approved S09.10
// house object; palette empty at v0; platform reference from the store),
// take the one shot, and drive the battery. A rejected draft or a red
// battery still COMPLETES the run — the ceremony happened, its receipt
// stands, and the outcome is recorded on the task ledger; mechanical
// failures crash for the recovery ladder as usual.
func (s *Skeleton) dispatchCompose(ctx context.Context, r run.Run) error {
	if _, err := s.cfg.Runs.Transition(ctx, r.ID, run.StateRunning, run.TransitionOptions{
		Reason: "composition ceremony (S08.6)", Actor: run.ActorPlatform,
	}); err != nil {
		return err
	}
	if s.cfg.Workers == nil {
		s.crash(ctx, r.ID, "compose run without a worker store")
		return errors.New("stage: compose run without a worker store")
	}
	st, err := s.pipe.LoadState(ctx, r.TaskID)
	if err != nil {
		s.crash(ctx, r.ID, "compose: load intake state: "+err.Error())
		return err
	}
	if st.Compose == nil || st.Compose.GapSignature == "" {
		s.crash(ctx, r.ID, "compose run without a recorded compose request")
		return errors.New("stage: compose run without a recorded compose request")
	}
	if s.cfg.ComposerPlaybook == nil {
		// The playbook is a REQUIRED policy input (Spec S08.6): an unwired
		// seam refuses loudly, never a silent skip (the §14 absent-seam
		// discipline for required duties).
		s.crash(ctx, r.ID, "composer playbook seam unwired (S08.6 inputs-by-policy)")
		return errors.New("stage: composer playbook seam unwired")
	}
	playbook, err := s.cfg.ComposerPlaybook(ctx)
	if err != nil {
		s.crash(ctx, r.ID, "compose: read composer playbook: "+err.Error())
		return err
	}
	pair, err := s.pipe.CurrentPair(ctx, r.TaskID)
	if err != nil {
		s.crash(ctx, r.ID, "compose: load task spec: "+err.Error())
		return err
	}
	specInput, err := composeSpecInput(pair)
	if err != nil {
		s.crash(ctx, r.ID, "compose: render task spec: "+err.Error())
		return err
	}

	tpl, v, out, err := s.cfg.Workers.Compose(ctx, st.Compose.RequestedBy, worker.ComposeInput{
		TaskID:       r.TaskID,
		TaskSpec:     specInput,
		GapSignature: st.Compose.GapSignature,
		Playbook:     playbook,
	}, &EngineComposer{s: s, runID: r.ID})
	if err != nil {
		if errors.Is(err, worker.ErrComposeRejected) {
			// The one shot produced an unusable draft: not retried (S08.6
			// one-shot rule). The ceremony completes with the rejection on
			// the task record; the gap keeps its standing disposition and
			// the still-open card's other choices remain.
			return s.finishCompose(ctx, r, fmt.Sprintf("composition rejected: %v", err))
		}
		s.crash(ctx, r.ID, "compose generation: "+err.Error())
		return err
	}

	if s.cfg.EnginePin == "" {
		s.crash(ctx, r.ID, "compose: Config.EnginePin unset — validation records key on (version × model × engine pin), S08.1")
		return errors.New("stage: Config.EnginePin unset")
	}
	battery, err := s.cfg.Workers.RunBattery(ctx, v.ID, worker.BatteryInput{
		Actor:      st.Compose.RequestedBy,
		SampleTask: out.SampleTask,
		Engine:     &EngineDryRun{s: s, runID: r.ID},
		Model:      out.Model,
		EnginePin:  s.cfg.EnginePin,
	})
	if err != nil {
		s.crash(ctx, r.ID, "compose battery: "+err.Error())
		return err
	}
	return s.finishCompose(ctx, r, fmt.Sprintf(
		"composed worker draft %q (template %s, version %s) from gap %s; battery green=%v — approval-as-diff card ready (S08.6 station 4)",
		tpl.Name, tpl.ID, v.ID, st.Compose.GapSignature, battery.Green))
}

// finishCompose lands the ceremony outcome: a platform decision on the
// task ledger (the requester's visible record on the still-open card's
// task), run completion, and the settle that materializes the requester's
// ceremony receipt.
func (s *Skeleton) finishCompose(ctx context.Context, r run.Run, outcome string) error {
	if _, err := s.cfg.Ledger.RecordDecision(ctx, r.ID, ledger.AuthorPlatform, run.ActorPlatform, "compose",
		outcome, "composition ceremony outcome (S08.6)", 0); err != nil {
		s.logger().Error("stage: record compose outcome", "run", r.ID, "err", err)
	}
	if _, err := s.cfg.Runs.Transition(ctx, r.ID, run.StateCompleted, run.TransitionOptions{
		Reason: "composition ceremony finished (S08.6)", Actor: run.ActorPlatform,
	}); err != nil {
		return err
	}
	s.settle(ctx, r.ID)
	s.logger().Info("stage: composition ceremony finished", "run", r.ID, "outcome", outcome)
	return nil
}

// composeSpecInput renders the task-spec policy input from the drafted
// pair: the specification side only — restatement, acceptance criteria,
// constraints, out-of-scope (what the recurring work IS; plan mechanics
// are not a composer input).
func composeSpecInput(pair *intake.Pair) (string, error) {
	raw, err := json.MarshalIndent(struct {
		Restatement string      `json:"restatement"`
		ACs         []intake.AC `json:"acs"`
		Constraints []string    `json:"constraints,omitempty"`
		OutOfScope  []string    `json:"out_of_scope,omitempty"`
	}{pair.Spec.Restatement, pair.Spec.ACs, pair.Spec.Constraints, pair.Spec.OutOfScope}, "", "  ")
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
