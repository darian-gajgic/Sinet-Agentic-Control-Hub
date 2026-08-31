package stage_test

// abort_e2e_test.go — P3-RW-10 T1–T3: an intake beat whose drive dies BECAUSE
// its caller's request context was canceled still leaves a classifiable corpse.
//
// The defect these tests pin (live, task `t-7c22f74e0d0d6c17`, 2026-08-12): the
// requester answered an interview card, `closeAndResume` committed
// parked→running, the plan-draft sessions started INSIDE the answer POST — and
// the page navigated away. The request context died, the drive died with it
// ("planner session: context canceled"), and then the CRASH POSTURE died too:
// `mapDriveErr` read the run's state on the same dead context, the read failed
// ("run: scan: context canceled"), and the beat fell through to a corpse-less
// raw 500. The run sat `running`, driver-less and unscannable, until the
// watchdog silence-parked it 5 minutes later — a park only another human could
// release.
//
// P3-RW-9 established the posture; this battery pins that the posture is
// CONTEXT-INDEPENDENT: the record of the ending outlives the request.
//
// AMENDED 2026-08-31 (P3-GF14 R1): §56's second bullet — "the drive itself
// stays attached, and abort-mid-drive is a CRASH" — is REVERSED, so the drive
// no longer dies with its caller and the caller's cancellation is no longer a
// cause the crash can name. Every other assertion in this file stands
// unchanged, and the corpse-and-heal posture it exists for is untouched: what
// moved is one cause STRING per test, at `callerDied`'s note below.
//
// Zero paid calls: the planning seam is the fork harness's recorder.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
)

// abortPlanner is the browser-abort seam: on its armed call it optionally
// cancels the REQUEST's own context and fails — the shape a page refresh
// mid-answer produces, where the seam dies because the CALLER died. Later calls
// plan normally, so a recovery fork can take the run over.
type abortPlanner struct {
	mu      sync.Mutex
	armed   bool
	cancel  context.CancelFunc              // nil leaves the caller's context alone
	failure func(ctx context.Context) error // the armed call's error
}

func (p *abortPlanner) Draft(ctx context.Context, in intake.DraftInput) (intake.Pair, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.armed {
		return forkPair(in), nil
	}
	p.armed = false
	if p.cancel != nil {
		p.cancel()
	}
	return intake.Pair{}, p.failure(ctx)
}

func (p *abortPlanner) Revise(_ context.Context, in intake.ReviseInput) (intake.Pair, error) {
	return in.Pair, nil
}

// callerDied is the armed failure of the diagnosed shape: a seam that fails
// while its caller's request is already dead.
//
// DATED NOTE 2026-08-31 (P3-GF14 R1, the §56 second-bullet reversal): the drive
// no longer rides the caller's context, so a seam reached after the abort sees
// a LIVE context and cannot report the caller's cancellation as its own cause.
// What these tests stage is unchanged — the request really does die mid-drive —
// and what they pin is unchanged: a drive that fails for a REAL cause past the
// resume commit still leaves a classifiable corpse for the ladder. Only the
// cause SENTENCE moved, because the caller's death is no longer a cause of
// drive death. That the abort alone no longer kills the drive is pinned by
// TestGF14AnswerDriveOutlivesItsCaller and its stage-level companions.
func callerDied(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("planner session: %w", err)
	}
	return errors.New("planner session died mid-drive")
}

// armAbort points the pipeline's planning seam at an abort planner that cancels
// ctx on its next call, and returns the cancelable request context.
func armAbort(h *forkHarness, failure func(context.Context) error) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	h.sk.Pipeline().Planner = &abortPlanner{armed: true, cancel: cancel, failure: failure}
	return ctx, cancel
}

// assertCorpse asserts the run is a classifiable corpse carrying a cause that
// names the failure — the whole point of the posture.
func assertCorpse(t *testing.T, h *forkHarness, runID, wantInCause string) {
	t.Helper()
	if got := h.runState(runID); got != run.StateCrashed {
		t.Fatalf("run %s is %s, want crashed (R1: the posture is context-independent)", runID, got)
	}
	if n := h.crashEvents(runID); n != 1 {
		t.Fatalf("%d crash events on %s, want exactly 1", n, runID)
	}
	cause := crashCause(t, h, runID)
	if cause == "" {
		t.Fatal("the crash carries no cause — an unclassifiable corpse (§8: reason+actor+detail)")
	}
	if !strings.Contains(cause, wantInCause) {
		t.Fatalf("crash cause = %q, want it to name %q", cause, wantInCause)
	}
}

// ---- T1: the answer beat ----

func TestAnswerAbortMidDriveStillLeavesCorpse(t *testing.T) {
	h := newForkHarness(t)
	bg := context.Background()
	const owner = "u-abort-answer"

	taskID, intakeRun, askID, body := walkToOpenCard(t, h, owner)

	// The answer commits (ask closed, parked→running) and THEN the request is
	// aborted under the drive: the planning seam sees a dead context and says so.
	ctx, cancel := armAbort(h, callerDied)
	defer cancel()

	_, err := h.sur.Answer(ctx, owner, askID, body, false)
	if err == nil {
		t.Fatal("the answer's advance must still fail loudly — the request really did fail")
	}
	if ctx.Err() == nil {
		t.Fatal("the request context is still live — this test is not pinning the abort shape")
	}
	se := surfaceError(t, err)
	if se.Status != http.StatusInternalServerError || se.Code != strandCodeRecovering {
		t.Fatalf("answer error = %d/%s, want 500/%s (honest AND classifiable, even under cancellation)",
			se.Status, se.Code, strandCodeRecovering)
	}

	assertCorpse(t, h, intakeRun, "planner session died mid-drive")
	if got := h.openAskID(taskID); got != "" {
		t.Fatalf("ask %q is still open — the answered card must stay closed", got)
	}

	// The heal is machine-only and bounded: ONE ladder pass forks the corpse, the
	// fork dispatches, rebinds and re-parks on its own card. No watchdog-silence
	// window, no human resume anywhere.
	forkID := h.forkOf(intakeRun)
	if n := h.tick(bg); n != 1 {
		t.Fatalf("tick dispatched %d, want the fork", n)
	}
	if got := h.crashEvents(forkID); got != 0 {
		t.Fatalf("the fork crashed %d times — it must take over, not re-die", got)
	}
	if got := h.runState(forkID); got != run.StateParked {
		t.Fatalf("fork %s is %s, want parked on a fresh gate", forkID, got)
	}
	freshAsk := h.openAskID(taskID)
	if freshAsk == "" {
		t.Fatal("the healed fork left no open card — the task would sit attention-less")
	}
	if got := h.askRunID(freshAsk); got != forkID {
		t.Fatalf("fresh ask run_id = %q, want the fork %q", got, forkID)
	}
}

// ---- T2: the Advance nudge ----

func TestAdvanceAbortMidDriveStillLeavesCorpse(t *testing.T) {
	h := newForkHarness(t)
	bg := context.Background()
	const owner = "u-abort-advance"

	taskID, intakeRun, askID, body := walkToOpenCard(t, h, owner)

	// The window: answer through the PIPELINE (which transitions nothing — the
	// corpse is the stage layer's act, CONVENTIONS §14/§16), leaving the run
	// running with no open gate and nobody driving it.
	h.planner.failFirst = 1
	if _, err := h.sk.Pipeline().Answer(bg, owner, askID, body); err == nil {
		t.Fatal("expected the pipeline answer to fail with the dying planning seam")
	}
	if got := h.runState(intakeRun); got != run.StateRunning {
		t.Fatalf("window: run %s is %s, want running", intakeRun, got)
	}

	// (a) the nudge re-drives and the request is aborted under it.
	ctx, cancel := armAbort(h, callerDied)
	defer cancel()
	_, err := h.sur.Advance(ctx, owner, taskID)
	if err == nil {
		t.Fatal("expected the advance to fail with the aborted request")
	}
	se := surfaceError(t, err)
	if se.Status != http.StatusInternalServerError || se.Code != strandCodeRecovering {
		t.Fatalf("advance error = %d/%s, want 500/%s", se.Status, se.Code, strandCodeRecovering)
	}
	assertCorpse(t, h, intakeRun, "planner session died mid-drive")

	forkID := h.forkOf(intakeRun)
	if n := h.tick(bg); n != 1 {
		t.Fatalf("tick dispatched %d, want the fork", n)
	}
	if got := h.runState(forkID); got != run.StateParked {
		t.Fatalf("fork %s is %s, want parked on a fresh gate", forkID, got)
	}

	if n := h.crashEvents(forkID); n != 0 {
		t.Fatalf("%d crash events on the fork — it must take over, not re-die", n)
	}
	if n := h.crashEvents(intakeRun); n != 1 {
		t.Fatalf("%d crash events on %s, want the original 1", n, intakeRun)
	}

	// (b) the refusal mapping survives cancellation. The fixture is the live
	// wedge itself: a run parked with NO open card (every ask answered, then the
	// watchdog's silence park), which the nudge refuses because `pipe.Advance`
	// demands `running`. Under an ALREADY-dead context it must answer that SAME
	// 409 — never a raw 500 — and mint no corpse: the beat never reached the
	// resume commit, so there is nothing to strand.
	taskB, runB, askB, bodyB := walkToOpenCard(t, h, owner)
	h.sk.Pipeline().Planner = &abortPlanner{armed: true, failure: func(context.Context) error {
		return errors.New("planning session died mid-advance")
	}}
	if _, err := h.sk.Pipeline().Answer(bg, owner, askB, bodyB); err == nil {
		t.Fatal("expected the pipeline answer to fail with the dying planning seam")
	}
	if _, err := h.runs.Transition(bg, runB, run.StateParked, run.TransitionOptions{
		Reason: "watchdog:watchdog.silence", Actor: run.ActorPlatform,
	}); err != nil {
		t.Fatalf("park %s: %v", runB, err)
	}

	dead, cancelDead := context.WithCancel(bg)
	cancelDead()
	for _, tc := range []struct {
		name string
		ctx  context.Context
	}{{"live context", bg}, {"canceled context", dead}} {
		t.Run(tc.name, func(t *testing.T) {
			view, err := h.sur.Advance(tc.ctx, owner, taskB)
			if err == nil {
				t.Fatalf("advancing an ask-less parked run must refuse (got %s)", view)
			}
			se := surfaceError(t, err)
			if se.Status != http.StatusConflict || se.Code != "conflict" {
				t.Fatalf("advance-on-parked = %d/%s, want 409/conflict", se.Status, se.Code)
			}
			if got := h.runState(runB); got != run.StateParked {
				t.Fatalf("run %s is %s, want still parked", runB, got)
			}
			if n := h.crashEvents(runB); n != 0 {
				t.Fatalf("%d crash events — a refused nudge mints no corpse", n)
			}
		})
	}
}

// ---- T3: the continuation legs ----

// abortingAdmitter is the S16.6 ingress refusing, then dying WITH ITS CALLER.
// Refused first so the composition never launches, which is what leaves the
// compose leg of `afterIntake` still to do on the approval — and that leg runs
// BEFORE the approved run is completed, the one continuation window that can
// strand a still-running run.
type abortingAdmitter struct {
	stage.Admitter
	cancel context.CancelFunc
	refuse bool
	armed  bool
}

func (a *abortingAdmitter) Enqueue(ctx context.Context, runID string, class scheduler.WorkloadClass) error {
	switch {
	case a.armed:
		a.armed = false
		a.cancel()
		return fmt.Errorf("admission for %s died with its caller: %w", runID, ctx.Err())
	case a.refuse:
		return fmt.Errorf("admission refused for %s (test)", runID)
	}
	return a.Admitter.Enqueue(ctx, runID, class)
}

func TestContinuationAbortStillLeavesCorpse(t *testing.T) {
	h := newForkHarness(t)
	bg := context.Background()
	const owner = "u-abort-continuation"

	h.sk.Pipeline().Router = composeEarnedRouter{}
	taskID, intakeRun, askID, body := walkToOpenCard(t, h, owner)

	// Answer card 1 for real: the pipeline drafts, critiques and parks on the
	// approval card, which now offers the compose verb.
	if _, err := h.sur.Answer(bg, owner, askID, body, false); err != nil {
		t.Fatalf("Answer(card 1): %v", err)
	}
	approvalAsk := h.openAskID(taskID)
	if approvalAsk == "" || approvalAsk == askID {
		t.Fatalf("expected a fresh approval card, got %q", approvalAsk)
	}

	// The compose verb is chosen and its launch is refused: the request is
	// RECORDED on the state, the card stays open and the run stays parked — so
	// the composition is still owed when the approval resumes the run.
	ctx, cancel := context.WithCancel(bg)
	defer cancel()
	admitter := &abortingAdmitter{Admitter: h.sched, cancel: cancel, refuse: true}
	h.sk.Bind(admitter)
	if _, err := h.sur.Answer(bg, owner, approvalAsk, json.RawMessage(`{"action":"compose"}`), true); err == nil {
		t.Fatal("expected the composition launch to fail under refused admission")
	}
	if got := h.runState(intakeRun); got != run.StateParked {
		t.Fatalf("run %s is %s, want still parked on its approval card", intakeRun, got)
	}

	// Approving RESUMES the run; the composition launch then dies with the
	// request that asked for it, on a run that is RUNNING — the strand.
	admitter.refuse, admitter.armed = false, true
	_, err := h.sur.Answer(ctx, owner, approvalAsk, json.RawMessage(`{"action":"approve"}`), true)
	if err == nil {
		t.Fatal("expected the approved continuation to fail with the aborted request")
	}
	if ctx.Err() == nil {
		t.Fatal("the request context is still live — this test is not pinning the abort shape")
	}
	se := surfaceError(t, err)
	if se.Status != http.StatusInternalServerError || se.Code != strandCodeRecovering {
		t.Fatalf("continuation error = %d/%s, want 500/%s", se.Status, se.Code, strandCodeRecovering)
	}
	assertCorpse(t, h, intakeRun, "continuation")
	if got := h.openAskID(taskID); got != "" {
		t.Fatalf("ask %q is still open — the answered approval card must stay closed", got)
	}

	// And the corpse heals the same way the drive's does.
	forkID := h.forkOf(intakeRun)
	if n := h.tick(bg); n != 1 {
		t.Fatalf("tick dispatched %d, want the fork", n)
	}
	if got := h.runState(forkID); got == run.StateCrashed {
		t.Fatalf("fork %s re-died instead of taking over", forkID)
	}
}

// ---- the invariant, swept over the failure space ----

// The spec-stated invariant (S02.3 + S02.5 step 2, this packet's headline): a
// beat that took a run to `running` ALWAYS leaves either a live driver or a
// classifiable corpse — whatever the drive's error looks like and whatever the
// caller's context did. The named tests above pin the diagnosed shapes; this
// sweeps the space, because the drive calls duty seams that are free to fail
// any way at all.
func TestIntakeBeatPostureHoldsOverFailureShapes(t *testing.T) {
	shapes := []struct {
		name    string
		cancels bool
		failure func(ctx context.Context) error
	}{
		{"plain error, live context", false, func(context.Context) error {
			return errors.New("planning session returned nothing usable")
		}},
		{"classified sentinel, live context", false, func(context.Context) error {
			return fmt.Errorf("planning session returned an unusable pair: %w", intake.ErrBadAnswer)
		}},
		{"the caller's own context error", true, callerDied},
		{"plain error under a dead context", true, func(context.Context) error {
			return errors.New("planning session returned nothing usable")
		}},
		{"classified sentinel under a dead context", true, func(context.Context) error {
			return fmt.Errorf("planning session returned an unusable pair: %w", intake.ErrBadAnswer)
		}},
		{"deadline exceeded", true, func(context.Context) error {
			return fmt.Errorf("planner session: %w", context.DeadlineExceeded)
		}},
	}
	for _, shape := range shapes {
		t.Run(shape.name, func(t *testing.T) {
			h := newForkHarness(t)
			const owner = "u-sweep"
			taskID, intakeRun, askID, body := walkToOpenCard(t, h, owner)

			ctx := context.Background()
			if shape.cancels {
				var cancel context.CancelFunc
				ctx, cancel = armAbort(h, shape.failure)
				defer cancel()
			} else {
				h.sk.Pipeline().Planner = &abortPlanner{armed: true, failure: shape.failure}
			}

			_, err := h.sur.Answer(ctx, owner, askID, body, false)
			if err == nil {
				t.Fatal("the dying drive must fail the request loudly")
			}
			se := surfaceError(t, err)
			if se.Status != http.StatusInternalServerError || se.Code != strandCodeRecovering {
				t.Fatalf("answer error = %d/%s, want 500/%s — the run's STATE outranks every classification",
					se.Status, se.Code, strandCodeRecovering)
			}
			if got := h.runState(intakeRun); got == run.StateRunning {
				t.Fatal("the run is still running with nobody driving it — the strand this packet ends")
			}
			assertCorpse(t, h, intakeRun, "answer")
			if got := h.openAskID(taskID); got != "" {
				t.Fatalf("ask %q is still open on a crashed run", got)
			}
		})
	}
}

// The other half of the cut line: a beat REFUSED before the resume commit keeps
// its 4xx and mints no corpse — including under a context that is already dead
// when the request arrives (an aborted retry of a stale card).
func TestPreResumeRefusalsSurviveCancellation(t *testing.T) {
	h := newForkHarness(t)
	bg := context.Background()
	const owner = "u-sweep-refusals"

	taskID, intakeRun, askID, body := walkToOpenCard(t, h, owner)
	dead, cancel := context.WithCancel(bg)
	cancel()

	for _, tc := range []struct {
		name string
		ctx  context.Context
		ask  string
		body json.RawMessage
	}{
		{"unknown ask, live context", bg, "intake:" + taskID + ":99", body},
		{"unknown ask, dead context", dead, "intake:" + taskID + ":99", body},
		{"open card, dead context", dead, askID, body},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.sur.Answer(tc.ctx, owner, tc.ask, tc.body, false)
			if err == nil {
				t.Fatal("refusal expected")
			}
			var se *api.SurfaceError
			if !errors.As(err, &se) {
				t.Fatalf("error %v is not a *api.SurfaceError — a raw 500 for an untouched gate", err)
			}
			if se.Status < 400 || se.Status >= 500 {
				t.Fatalf("refusal answered %d/%s, want a 4xx (gates wait, S06.1)", se.Status, se.Code)
			}
			if got := h.runState(intakeRun); got != run.StateParked {
				t.Fatalf("run %s is %s, want still parked", intakeRun, got)
			}
			if got := h.openAskID(taskID); got != askID {
				t.Fatalf("open ask = %q, want the untouched card %q", got, askID)
			}
			if n := h.crashEvents(intakeRun); n != 0 {
				t.Fatalf("%d crash events — a refused answer mints no corpse", n)
			}
		})
	}
}
