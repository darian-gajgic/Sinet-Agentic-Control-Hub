package stage

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// router.go — the composition glue between Spec S08.8 selection (the
// worker-package Router) and its two consumers here: the intake approval
// card (pre-execution visibility + re-route/pin, through the intake.Router
// seam) and the execute dispatch (which consumes the recorded selection,
// emits the settled routing.decided event per run, and compiles the worker
// per invocation — Spec S08.3/S08.8; 7.7 accountability).

// skeletonRouter adapts the worker Router to the intake seam.
type skeletonRouter struct{ s *Skeleton }

var _ intake.Router = (*skeletonRouter)(nil)

func (sr *skeletonRouter) RouteTask(ctx context.Context, q intake.RouteQuery) (intake.RouteBlock, error) {
	// Intake-time routing: the consuming run of any tie-break D7 write is the
	// intake run — the one the pipeline is driving NOW (drain F2), which the
	// query carries. On a recovery fork the suffix composition would name the
	// superseded parent, whose crashed state makes that D7 row unwritable
	// (P3-RW-6 R7). An unset id keeps the birth composition (test posture).
	runID := q.RunID
	if runID == "" {
		runID = sr.s.currentIntakeRun(ctx, q.TaskID)
	}
	d, err := sr.s.router.Route(ctx, worker.RouteQuery{
		Requester: q.Requester,
		TaskID:    q.TaskID,
		RunID:     runID,
		TaskText:  q.TaskText,
		Family:    q.Family,
		Domain:    domainFor(intake.Family(q.Family)),
		Kind:      worker.KindAgentic,
		Classes:   q.Classes,
		Tools:     q.Tools,
		Research:  q.Research,
		Writes:    q.Writes,
		// The task's own lane pin, carried to selection (S00.9 A13). A
		// correct resolver reached through a dropped argument is a broken
		// feature, so this is the site the pin guards drive.
		PinnedLane: q.PinnedLane,
	})
	if err != nil {
		return intake.RouteBlock{}, err
	}
	return blockFromDecision(d), nil
}

func blockFromDecision(d worker.Decision) intake.RouteBlock {
	b := intake.RouteBlock{
		Cause:         d.Cause,
		Score:         d.Score,
		Signals:       d.Signals,
		TemplateID:    d.TemplateID,
		TemplateName:  d.TemplateName,
		VersionID:     d.VersionID,
		Generalist:    d.Generalist,
		Model:         d.Model,
		Lane:          d.Lane,
		Effort:        d.Effort,
		WindowTokens:  d.WindowTokens,
		PlainReason:   d.PlainReason,
		Degraded:      d.Degraded,
		GapSignature:  d.GapSignature,
		ComposeEarned: d.ComposeEarned,
		GapAdvice:     d.GapAdvice,
		LanePin:       d.LanePin,
	}
	for _, c := range d.Candidates {
		b.Candidates = append(b.Candidates, intake.RouteCandidate{
			TemplateID: c.TemplateID, Name: c.Name, VersionID: c.VersionID,
			Score: c.Score, Reason: c.Reason,
		})
	}
	return b
}

func decisionFromBlock(b *intake.RouteBlock) worker.Decision {
	return worker.Decision{
		Cause:         b.Cause,
		Score:         b.Score,
		Signals:       b.Signals,
		TemplateID:    b.TemplateID,
		TemplateName:  b.TemplateName,
		VersionID:     b.VersionID,
		Generalist:    b.Generalist,
		Model:         b.Model,
		Lane:          b.Lane,
		Effort:        b.Effort,
		WindowTokens:  b.WindowTokens,
		PlainReason:   b.PlainReason,
		Degraded:      b.Degraded,
		GapSignature:  b.GapSignature,
		ComposeEarned: b.ComposeEarned,
		GapAdvice:     b.GapAdvice,
		LanePin:       b.LanePin,
	}
}

// executeRouting resolves the selection an execute run consumes: the
// approval-card decision (with any recorded re-route/pin) is authoritative
// — selection was VISIBLE pre-execution there (Spec S08.8). A run without
// one (the test-only no-router posture, or a pre-B3-3 task) falls to the
// duty-map generalist default, loudly logged and honestly labeled in the
// event — never a silent switch (14.3).
type executeRouting struct {
	Decision worker.Decision
	Override struct {
		Pinned bool
		Actor  string
	}
}

func (s *Skeleton) executeRouting(ctx context.Context, r run.Run) executeRouting {
	var out executeRouting
	if st, err := s.pipe.LoadState(ctx, r.TaskID); err == nil && st.Routing != nil {
		out.Decision = decisionFromBlock(st.Routing)
		out.Override.Pinned = st.Routing.Pinned
		out.Override.Actor = st.Routing.OverriddenBy
		if s.cfg.Model != "" {
			// Dev/test model override wins over the seat everywhere,
			// recorded honestly.
			out.Decision.Model = s.cfg.Model
			out.Decision.Signals = append(out.Decision.Signals, "model_override=config")
		}
		return out
	}
	seat := s.seat(worker.DutyExecution)
	model := seat.Model
	if s.cfg.Model != "" {
		model = s.cfg.Model
	}
	s.logger().Warn("stage: no recorded selection for execute run — duty-map generalist default (test-only posture; the composition root wires the router at B3-3)",
		"run", r.ID)
	out.Decision = worker.Decision{
		Cause:        "no-fit-generalist",
		Generalist:   true,
		Model:        model,
		Lane:         seat.Lane,
		WindowTokens: seat.WindowTokens,
		PlainReason:  "No recorded selection (router unwired — test posture); generalist on the execution duty seat.",
	}
	return out
}

// taskLanePin reads the task's declared lane pin off the recorded intake state
// — the same read executeRouting already performs (S00.9 A13).
//
// It reads the REQUEST rather than the recorded routing block deliberately: the
// pin is a fact about the task, so re-reading it is what makes it survive a
// re-plan recompute without the worker pin's freeze mechanic. A task with no
// state, or no pin, answers "" and every downstream path is byte-unchanged.
func (s *Skeleton) taskLanePin(ctx context.Context, taskID string) string {
	st, err := s.pipe.LoadState(ctx, taskID)
	if err != nil || st == nil {
		return ""
	}
	return st.Req.PinnedLane
}

// pinnedExecutionSeat resolves the pinned lane's EXECUTION seat from the
// composed alternates, for the degraded path that never reaches the router
// (S00.9 A13).
//
// It is the same seat set the router resolves a pin against, read from the
// same composed value — so a pin honored by selection and a pin honored by
// the fallback name the same model, and neither invents one. Not found means
// this platform seats no model for that lane, which the caller must treat as
// a refusal rather than as permission to seat something else.
func (s *Skeleton) pinnedExecutionSeat(lane string) (worker.Seat, bool) {
	if lane == "" {
		return worker.Seat{}, false
	}
	if seat := s.seat(worker.DutyExecution); seat.Lane == lane {
		return seat, true
	}
	for _, alt := range s.cfg.AlternateSeats[worker.DutyExecution] {
		if alt.Lane == lane {
			if alt.WindowTokens == 0 {
				alt.WindowTokens = worker.DefaultWindowTokens
			}
			return alt, true
		}
	}
	return worker.Seat{}, false
}

// emitRoutingDecided appends the settled routing.decided event on the run
// (Spec S08.8: every worker/model/effort decision emits it; worker +
// version per run is the version→outcome join key). Failure to record the
// accountability event fails the dispatch — routing without its record
// would be silent routing (7.7; 14.3).
func (s *Skeleton) emitRoutingDecided(ctx context.Context, r run.Run, er executeRouting) error {
	type override struct {
		Pinned bool   `json:"pinned,omitempty"`
		Actor  string `json:"actor,omitempty"`
	}
	payload, err := json.Marshal(struct {
		worker.Decision
		Override *override `json:"override,omitempty"`
	}{er.Decision, func() *override {
		if er.Override.Actor == "" && !er.Override.Pinned {
			return nil
		}
		return &override{Pinned: er.Override.Pinned, Actor: er.Override.Actor}
	}()})
	if err != nil {
		return fmt.Errorf("stage: marshal routing.decided: %w", err)
	}
	if _, err := s.cfg.Log.Append(ctx, eventlog.Append{
		RunID: r.ID, Generation: r.Generation, UserID: r.UserID,
		Type: worker.EventRoutingDecided, SchemaVersion: worker.RoutingSchemaVersion, Payload: payload,
	}); err != nil {
		return fmt.Errorf("stage: append routing.decided: %w", err)
	}
	return nil
}
