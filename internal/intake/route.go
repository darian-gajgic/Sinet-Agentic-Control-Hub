package intake

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// route.go — the S08.8 boundary as intake sees it: the plan declares
// REQUIREMENTS (confinement class, tools, family, research nodes, effort
// mode — Spec S06); selection of worker, model, and lane is Spec S08.8's,
// reached through the Router seam below (wired by the composition layer to
// the worker-store router; the seam keeps intake free of the worker
// package, the S06.10 Planner/Critic seam pattern).
//
// The selection is VISIBLE AND OVERRIDABLE pre-execution (Spec S08.8): it
// rides the Stage-4 approval card (which also carries the S06 restatement —
// the no-fit flow's stage 1 "task interpretation confirmed" surface), and
// the approval answer may re-route or pin before anything executes;
// overrides are recorded with their actor.

// RouteQuery carries the plan-declared requirements to the router.
type RouteQuery struct {
	Requester string `json:"requester"`
	TaskID    string `json:"task_id"`
	// RunID is the CONSUMING run of any work the selection performs (the
	// tie-break's $0 D7 row rides it) — the run the pipeline is driving now,
	// which after a recovery-fork rebind is the fork, never the superseded
	// parent (CONVENTIONS §16 run-id rule; the worker.RouteQuery.RunID shape).
	RunID    string   `json:"run_id,omitempty"`
	TaskText string   `json:"task_text"`
	Family   string   `json:"family"`
	Classes  []string `json:"classes,omitempty"`
	Tools    []string `json:"tools,omitempty"`
	Research bool     `json:"research,omitempty"`
	// Writes reports that the plan durably declares it writes the workspace
	// (any non-empty step write_set, or an unbounded whole-project claim —
	// the fact the S02.8 W claim is minted from). The plan DECLARES the
	// requirement; S08.8 selection enforces it (P3-RW-14 R8).
	Writes bool `json:"writes,omitempty"`
	// PinnedLane carries the task's declared lane pin to selection (S00.9
	// A13). It is re-read from the REQUEST on every recompute rather than
	// frozen onto the block, which is why a lane pin survives re-planning
	// with none of the worker pin's freeze mechanic.
	PinnedLane string `json:"pinned_lane,omitempty"`
}

// RouteCandidate is one re-route target offered on the card.
type RouteCandidate struct {
	TemplateID string  `json:"template_id"`
	Name       string  `json:"name"`
	VersionID  string  `json:"version_id"`
	Score      float64 `json:"score"`
	Reason     string  `json:"reason"`
}

// RouteBlock is the selection as surfaced on the plan/approval card and
// stored in the intake state (Spec S08.8: worker + plain-language reason
// visible pre-execution; the no-fit two-stage card content; override
// bookkeeping with its actor).
type RouteBlock struct {
	Cause        string   `json:"cause"`
	Score        float64  `json:"score,omitempty"`
	Signals      []string `json:"signals,omitempty"`
	TemplateID   string   `json:"worker,omitempty"`
	TemplateName string   `json:"worker_name,omitempty"`
	VersionID    string   `json:"version,omitempty"`
	Generalist   bool     `json:"generalist,omitempty"`
	Model        string   `json:"model"`
	Lane         string   `json:"lane"`
	Effort       string   `json:"effort,omitempty"`
	WindowTokens int64    `json:"window_tokens"`
	PlainReason  string   `json:"plain_reason"`
	Degraded     bool     `json:"degraded,omitempty"`

	Candidates []RouteCandidate `json:"candidates,omitempty"`

	// No-fit stage-2 card content (Spec S08.8): the generalist default is
	// the pre-picked choice; ComposeEarned marks the compose-a-worker offer
	// (S08.6 compose-when-earned); GapAdvice the 2.7 model-gap leg.
	GapSignature  string `json:"gap_signature,omitempty"`
	ComposeEarned bool   `json:"compose_earned,omitempty"`
	GapAdvice     string `json:"gap_advice,omitempty"`

	// LanePin mirrors worker.Decision.LanePin: the lane this task pinned at
	// creation, empty when it pinned none (S00.9 A13). It is the structured
	// member LN-10's picker binds to; the plain reason is what makes the pin
	// visible on every surface that already renders a reason.
	//
	// It is deliberately NOT Pinned below. That flag is the WORKER axis and
	// means one specific thing — freeze the worker choice against a re-plan
	// recompute — and setting it for a lane pin would freeze a choice nobody
	// asked to freeze.
	LanePin string `json:"lane_pin,omitempty"`

	// Override bookkeeping (Spec S08.8 "overrides are recorded with their
	// actor"). Pinned selections survive re-planning recomputes.
	Pinned       bool   `json:"pinned,omitempty"`
	OverriddenBy string `json:"overridden_by,omitempty"`
}

// Router is the S08.8 selection seam. Nil = no routing wired (a TEST-only
// posture: the composition root always wires it at B3-3; cards then carry
// no routing block and execution falls to the duty-map default downstream —
// loudly logged there, never silent).
type Router interface {
	RouteTask(ctx context.Context, q RouteQuery) (RouteBlock, error)
}

// RouteOverride is the approval-answer re-route/pin entry (Spec S08.8:
// "the requester can re-route or pin before execution").
type RouteOverride struct {
	// Target is "generalist" or a template id offered in
	// RouteBlock.Candidates (or the selected worker itself, for a pure
	// pin).
	Target string `json:"target"`
	// Pin freezes the (possibly re-routed) selection: re-planning
	// recomputes routing EXCEPT when pinned.
	Pin bool `json:"pin,omitempty"`
}

// RouteTargetGeneralist is the override target selecting the generalist
// default.
const RouteTargetGeneralist = "generalist"

// routeQueryFor derives the router inputs from the intake state + plan —
// platform facts only (Spec S08.1 selector discipline).
func routeQueryFor(st *State, pair *Pair) RouteQuery {
	q := RouteQuery{
		Requester: st.Owner,
		TaskID:    st.TaskID,
		// The consuming run, read from the state — the fork after a rebind (R7).
		RunID:    st.RunID,
		TaskText: st.Req.Title + " " + st.Req.Text,
		Family:   string(st.Family),
		Research: len(pair.Plan.ResearchNodes) > 0,
		// Read FRESH from the request on every recompute (S00.9 A13). That is
		// what makes the lane pin survive a re-plan without borrowing the
		// worker pin's freeze: the pin is a fact about the TASK, so a
		// recompute that re-reads the task re-reads the pin.
		PinnedLane: st.Req.PinnedLane,
	}
	// The write posture is a REQUIREMENT, exactly like the class and tools
	// below it: a plan that declares it writes cannot be given to equipment
	// that cannot write (Spec S08.8 step 1; S02.8 — the claim is the durable
	// knowledge that the task writes).
	globs, unbounded := pair.Plan.WriteGlobs()
	q.Writes = len(globs) > 0 || unbounded
	seen := map[string]bool{}
	for _, s := range pair.Plan.Steps {
		if s.Class != "" && !seen["c:"+s.Class] {
			seen["c:"+s.Class] = true
			q.Classes = append(q.Classes, s.Class)
		}
		for _, tool := range s.NewTools {
			if tool != "" && !seen["t:"+tool] {
				seen["t:"+tool] = true
				q.Tools = append(q.Tools, tool)
			}
		}
	}
	return q
}

// refuseLanePin answers whether a submitted lane pin may be admitted (S00.9
// A13), returning an ErrLanePinRefused-wrapped refusal when it may not.
//
// Layer 1 of three. The VERDICT is the selection layer's, carried across the
// seam as data and quoted verbatim; what happens here is set membership and
// message composition, never a second derivation of the domain rule (§66 D1:
// the boundary never re-derives a domain verdict). The message names the lanes
// that ARE pinnable, the way the plan-budget verb names the windows that exist.
func (p *Pipeline) refuseLanePin(lane string) error {
	if lane == "" {
		return nil
	}
	for _, opt := range p.PinnableLanes {
		if opt.Lane != lane {
			continue
		}
		if opt.Pinnable {
			return nil
		}
		// A lane the platform KNOWS and still cannot dispatch to. Its own
		// words, computed where the rule lives — borrowing the general
		// sentence would answer the wrong question.
		return fmt.Errorf("%w: %s", ErrLanePinRefused, opt.NotPinnable)
	}
	return fmt.Errorf("%w: lane %q is not one this platform can pin a task to. Pinnable lanes: %s",
		ErrLanePinRefused, lane, p.pinnableLaneNames())
}

// pinnableLaneNames lists the admissible lanes for a refusal message. Lane
// names ship in the lane documents and are not secret, so unlike the project
// pin there is no existence-oracle concern and this may enumerate freely.
func (p *Pipeline) pinnableLaneNames() string {
	names := make([]string, 0, len(p.PinnableLanes))
	for _, opt := range p.PinnableLanes {
		if opt.Pinnable {
			names = append(names, strconv.Quote(opt.Lane))
		}
	}
	if len(names) == 0 {
		return "none — this platform holds no pinnable lane at all"
	}
	return strings.Join(names, ", ")
}

// computeRouting runs selection for the approval card (pre-execution
// visibility). A PINNED prior selection is kept (revalidated against the
// fresh candidate set when worker-backed); everything else recomputes —
// re-planning changed the requirements the selection reads.
func (p *Pipeline) computeRouting(ctx context.Context, st *State, pair *Pair) error {
	if p.Router == nil {
		return nil
	}
	q := routeQueryFor(st, pair)
	fresh, err := p.Router.RouteTask(ctx, q)
	if err != nil {
		return fmt.Errorf("intake: route selection: %w", err)
	}
	if st.Routing != nil && st.Routing.Pinned {
		prior := st.Routing
		if prior.Generalist {
			fresh.Generalist, fresh.TemplateID, fresh.TemplateName, fresh.VersionID = true, "", "", ""
			fresh.Cause = "pinned"
			fresh.PlainReason = "Pinned to the generalist default by " + prior.OverriddenBy + ". " + fresh.PlainReason
			fresh.Pinned, fresh.OverriddenBy = true, prior.OverriddenBy
			st.Routing = &fresh
			return nil
		}
		for _, c := range fresh.Candidates {
			if c.TemplateID == prior.TemplateID {
				kept := fresh
				kept.Cause = "pinned"
				kept.TemplateID, kept.TemplateName, kept.VersionID = c.TemplateID, c.Name, c.VersionID
				kept.Generalist = false
				kept.Score = c.Score
				kept.PlainReason = fmt.Sprintf("Pinned to %q by %s. %s", c.Name, prior.OverriddenBy, c.Reason)
				kept.Pinned, kept.OverriddenBy = true, prior.OverriddenBy
				st.Routing = &kept
				return nil
			}
		}
		// The pinned worker no longer survives the filters (retired,
		// flagged, or the re-planned requirements exceed it): the pin
		// cannot silently widen powers — fall to the fresh selection with
		// the loss recorded (never silent, 14.3).
		fresh.PlainReason = fmt.Sprintf("Previously pinned worker %q no longer fits the re-planned requirements; pin dropped. %s",
			prior.TemplateName, fresh.PlainReason)
	}
	st.Routing = &fresh
	return nil
}

// applyRouteOverride applies the approval-answer re-route/pin (Spec S08.8;
// validated against the card's offered set — an override can never widen
// beyond what selection admitted).
func applyRouteOverride(st *State, ov *RouteOverride, actor string) error {
	if ov == nil {
		return nil
	}
	if st.Routing == nil {
		return fmt.Errorf("%w: no routing block on this card to override", ErrBadAnswer)
	}
	r := *st.Routing
	switch {
	case ov.Target == "" && ov.Pin:
		// Pure pin of the presented selection.
	case ov.Target == RouteTargetGeneralist:
		r.TemplateID, r.TemplateName, r.VersionID = "", "", ""
		r.Generalist = true
		r.Cause = "override"
		r.PlainReason = fmt.Sprintf("Re-routed to the generalist default by %s (override recorded, S08.8). %s", actor, r.PlainReason)
	case ov.Target != "":
		found := false
		for _, c := range r.Candidates {
			if c.TemplateID == ov.Target {
				r.TemplateID, r.TemplateName, r.VersionID = c.TemplateID, c.Name, c.VersionID
				r.Generalist = false
				r.Score = c.Score
				r.Cause = "override"
				r.PlainReason = fmt.Sprintf("Re-routed to %q by %s (override recorded, S08.8). %s", c.Name, actor, c.Reason)
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%w: re-route target %q is not among the card's candidates", ErrBadAnswer, ov.Target)
		}
	default:
		return fmt.Errorf("%w: empty route override", ErrBadAnswer)
	}
	r.Pinned = r.Pinned || ov.Pin
	r.OverriddenBy = actor
	st.Routing = &r
	return nil
}
