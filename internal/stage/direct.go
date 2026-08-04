package stage

import (
	"context"
	"errors"
	"fmt"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// direct.go — the BENCH-REG §2 direct-arm run leg (P3-B6-2C, R19).
//
// §2, frozen: "the same confirmed task statement + the same attachments the
// specification had, run ONCE, SINGLE-SHOT against the requester's own frontier
// surface for that domain (their consumer subscription — chat or CLI — with that
// surface's native defaults; NO RETRIES, NO FOLLOW-UP TURNS, TAKE WHAT COMES)."
//
// Everything about this leg is that sentence, and the whole point of keeping it
// in its own file is that the omissions are visible:
//
//   - the prompt is the frozen statement ALONE. No stage brief is assembled, so
//     no ledger projection, no knowledge slice and no pinned-context file reach
//     it; no worker is compiled, so no agent definition and no template L0 reach
//     it; no recitation valve is wired, so no per-turn platform authoring reaches
//     it. The asymmetry between the arms IS the treatment under test (§6) and
//     lending the direct arm any of the platform's machinery would destroy the
//     comparison the practice exists to make.
//   - there is no retry, no re-dispatch, no follow-up turn and no successor of
//     any kind. The leg runs ONE session and reports what came back. A session
//     that ends short of completion is an OUTCOME, not a condition to recover
//     from: §6's parity note records it at verdict time and the arm is never
//     re-run (BENCH-REG §17 — a result is never recomputed).
//   - no stage-split machinery is wired. A split is a SUCCESSOR SESSION on a
//     consolidated ledger, which is exactly the multi-turn platform behaviour §2
//     forbids on this arm — so this leg deliberately does not go through
//     Skeleton.Session, whose budget watcher exists to request one.
//
// What it DOES keep is the platform's ordinary economics, because §2 calls those
// economics and not arm shaping: the adapter Driver drives the session, so every
// paid call takes its D7 checkpoint (Spec S02.4) and metering measures the arm —
// the §14 record needs that consumption, and an unmetered arm would make the §13
// done-directly figure unanswerable. Admission is the ordinary background lane
// the benchmark package enqueued it on, with no special path (G2 Def.4).
//
// The two things this package must NOT know about ride func seams composed at
// the shell root (Config.BenchmarkStatement / Config.BenchmarkCapture, the
// Snapshot/LedgerRevision/Paused precedent): internal/stage never imports
// internal/benchmark, and internal/benchmark never imports internal/stage — the
// §34/§35 seam posture, asserted on the benchmark side by its import wall.

// RunSuffixDirect names the §2 direct-arm run (benchmark.DirectRunID appends it
// to the pair id). It is duplicated across the seam rather than imported,
// because importing internal/benchmark here is exactly the edge the §35 wall
// forbids; the shell — which imports both — pins the two sides together at boot,
// the checkBenchmarkDomainMapping precedent.
const RunSuffixDirect = ".direct"

// roleDirect is the §2 direct-arm leg's dispatch role.
const roleDirect role = "direct"

// IsDirectArmRun reports whether a run id names a BENCH-REG §2 direct arm. It is
// the ONE definition of the convention: Skeleton.Dispatch routes on it, and the
// composition root hands it to the recovery ladder as the NoFork predicate — so
// "which runs are single-shot" is one fact with one home, and the router and the
// fork-refusal can never disagree about a run.
//
// Recovery forks append `.gN` segments, so the check tolerates them: a fork that
// somehow exists must still be recognized as a direct arm rather than routed as
// an unknown role. With the NoFork predicate composed, none should ever exist.
func IsDirectArmRun(runID string) bool {
	rl, ok := runRole(runID)
	return ok && rl == roleDirect
}

// directStage names the leg's per-run work/cwd directory. The direct arm is one
// session, so it has exactly one.
const directStage = "direct"

// errNoBenchmarkSeams reports a `.direct` run dispatched in a process whose
// benchmark seams are not composed. It is LOUD rather than degraded: the only
// thing that could be served without the statement seam is a prompt the platform
// invented, and a direct arm answering a different question than the platform arm
// is the one confound §3.1 exists to forbid.
var errNoBenchmarkSeams = errors.New(
	"stage: no BENCH-REG §2 direct-arm seams composed in this process — the frozen task statement " +
		"and the single-shot capture ride func seams from the composition root, and the arm is never " +
		"run on an invented prompt (BENCH-REG §2/§3.1)")

// dispatchDirect runs the §2 direct arm: ONE engine session on the run's own
// substrate and lane, whose prompt is the frozen task statement and nothing
// else, driven through the ordinary Driver so the run completes under the
// ordinary FSM (claimed→running→terminal) with its checkpoints and its metering
// intact.
func (s *Skeleton) dispatchDirect(ctx context.Context, r run.Run) error {
	if s.cfg.BenchmarkStatement == nil || s.cfg.BenchmarkCapture == nil {
		s.crash(ctx, r.ID, errNoBenchmarkSeams.Error())
		return errNoBenchmarkSeams
	}
	statement, err := s.cfg.BenchmarkStatement(ctx, r.TaskID)
	if err != nil {
		s.crash(ctx, r.ID, "direct-arm statement: "+err.Error())
		return fmt.Errorf("stage: read the BENCH-REG §2 frozen statement for %q: %w", r.TaskID, err)
	}
	if statement == "" {
		s.crash(ctx, r.ID, "direct-arm statement: the artifact of record is empty")
		return fmt.Errorf("stage: task %q has no confirmed statement to put to the direct arm (BENCH-REG §2)", r.TaskID)
	}

	// The run's OWN substrate — the requester's frontier surface for the domain,
	// fixed on the run row by the benchmark package at dispatch. The ceremony
	// substrate is never substituted here: which surface the direct arm ran on is
	// part of what the pair recorded.
	adapter, ok := s.cfg.Adapters[r.Substrate]
	if !ok {
		s.crash(ctx, r.ID, "no adapter for substrate "+r.Substrate)
		return fmt.Errorf("stage: no adapter registered for the direct arm's substrate %q (run %s)", r.Substrate, r.ID)
	}
	workDir, cwd, err := s.runDirs(r.ID, directStage)
	if err != nil {
		s.crash(ctx, r.ID, "direct-arm run dirs: "+err.Error())
		return err
	}

	req := adapters.StartRequest{
		RunID:  r.ID,
		UserID: r.UserID,
		// THE WHOLE INVOCATION. Prompt is the frozen statement; every other
		// CompiledWorker field is deliberately zero — no system-prompt append, no
		// agent definition, no tool allowlist, no gated tools, no permission mode,
		// no SessionStart re-injection path and no recitation valve.
		Worker: adapters.CompiledWorker{Prompt: statement},
		Model:  s.directArmModel(),
		// Ordinary platform machinery, not arm shaping: the owner's own
		// credential (the requester's subscription — §2's "their consumer
		// subscription"), the composed sandbox, and the run's own ceilings.
		OwnerCredRef:   s.ownerCredRef(ctx, r.UserID),
		Confiner:       s.cfg.Confiner,
		Cwd:            cwd,
		WorkDir:        workDir,
		CeilingCostUSD: r.CeilingCostUSD,
		CeilingSteps:   r.CeilingSteps,
	}
	if s.cfg.CredInject != nil {
		req.CredInject = s.cfg.CredInject(r.UserID)
	}

	// Driver.Drive, not DriveStage: this run's ONE session IS the run, so the
	// ordinary claimed→running→terminal mapping is the right FSM story and the
	// leg neither opens nor closes a transition of its own. Nothing registers the
	// session anywhere — no split requester (a split is the multi-turn behaviour
	// §2 forbids) and no cancel handle (the NO-AUTO-KILL wall's caller set stays
	// exactly the three files part A named; a human cancel of a benchmark arm is
	// not a thing this leg gets to offer).
	out, err := s.driver.Drive(ctx, adapter, req)
	if err != nil {
		// The session machinery itself failed (a spawn crash is reported as an
		// Outcome, not an error). Drive has already landed whatever FSM state it
		// could; there is nothing to capture and nothing to retry.
		s.logger().Warn("stage: direct-arm session failed (BENCH-REG §2: no retry, take what comes)",
			"run", r.ID, "err", err)
		return fmt.Errorf("stage: direct-arm session on %q: %w", r.ID, err)
	}

	// TAKE WHAT COMES — but only from an arm that actually ran.
	//
	// A session that ended on its own terms, or that an ordinary ceiling cut
	// short, PRODUCED something (possibly nothing at all, which is a real
	// single-shot outcome), so its text is captured verbatim and the pair goes on
	// to be rendered and voted on; the §6 parity note records at verdict time that
	// the run did not end cleanly.
	//
	// A CRASH is different in kind and is deliberately NOT captured (drain r1). An
	// engine that died — or never spawned — did not answer the frozen statement at
	// all, so an empty capture would render "the platform versus a platform
	// failure" and ask a person to vote on it, which is exactly the fiction the
	// honest-absence rule exists to prevent. The capture stays NULL, the pair
	// becomes the requester-facing failed-pair card, and a human closes it with a
	// §14 decline record. The arm is never re-run either way (§2; §17).
	if out.Kind == adapters.OutcomeCrashed {
		s.logger().Warn("stage: direct arm crashed without producing an answer; the pair cannot complete and its requester is shown a card (BENCH-REG §2)",
			"run", r.ID, "detail", out.Detail)
		return nil
	}
	took, cerr := s.cfg.BenchmarkCapture(ctx, r.ID, out.ResultText)
	if cerr != nil {
		// A failed capture is LOUD and does not fail the run: the run's own
		// disposition already committed, and the pair stays where it is until a
		// capture lands rather than advancing on a text nobody stored.
		s.logger().Error("stage: direct-arm capture failed; the pair stays awaiting its arm",
			"run", r.ID, "err", cerr)
		return nil
	}
	s.logger().Info("stage: direct arm ran once, single-shot (BENCH-REG §2)",
		"run", r.ID, "outcome", string(out.Kind), "captured_bytes", len(out.ResultText), "pair_took_capture", took)
	return nil
}

// directArmModel resolves the model the direct arm runs on.
//
// §2 asks for "that surface's native defaults" and §6 records that the direct
// arm's model identity "cannot be pinned or controlled; it is recorded as
// observed per pair". The lane cannot express "no model": S03.2 makes the model a
// per-invocation config channel and the claude-cli lowering refuses a start
// without one. So the platform passes the lane's OWN default working seat — the
// EXECUTION duty, which is what this household's frontier coding surface runs
// when it is asked to do the work — and the §14 record carries the identity
// metering actually OBSERVED, which is the figure §6 says is authoritative.
// Config.Model stays the dev/test override it is everywhere else.
func (s *Skeleton) directArmModel() string {
	if s.cfg.Model != "" {
		return s.cfg.Model
	}
	return s.seat(worker.DutyExecution).Model
}
