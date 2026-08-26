package stage_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
)

// lanepin_ln9_test.go — P3-LN-9, the per-task lane pin (backend half).
//
// COMMITTED RED, deliberately, by the grounding agent BEFORE the executor
// starts (the brief's acceptance specification, executable). Both tests below
// COMPILE against today's surface and FAIL for exactly one reason: the
// task-creation verb accepts an unknown `pinned_lane` member and SILENTLY
// DROPS it, so a submission that names a lane the platform cannot honor is
// answered 200 with a task born, instead of refused.
//
// Why they compile: POST /api/intake/requests is decoded from a
// json.RawMessage (internal/stage/surface.go — `submitBody`), so a member the
// struct does not declare is legal JSON that unmarshals to nothing. No Go
// identifier from the unbuilt feature appears here; a tests-first commit that
// does not build is a commit whose tests are not red for the right reason
// (CONVENTIONS §65 D10 records the cost of the other choice).
//
// The contract they pin (brief R3): an operator who names a lane asked for
// THAT lane. A pin the platform cannot honor is refused LOUDLY at the system
// boundary and no task is born believing it got one — never a silent fallback
// to routing's own choice. This is the posture `intake.ErrPinRefused` already
// codifies for Request.Project (internal/intake/intake.go:532-547) and the
// INVERSE of routing's degrade-and-explain posture for uncovered lanes
// (internal/worker/routing.go:699-708), which is correct for a lane the
// PLATFORM chose and wrong for a lane a PERSON chose.
//
// The harness (newProjectHarness, round1_e2e_test.go:111) composes
// stage.Config with no Lane and no CommissionedLanes, so NOTHING is pinnable
// in this world — which is what makes "kimi-cli" an honest refusal case
// without commissioning a credential ($0; no provider call on any path).

func submitRawLanePin(h *projHarness, userID, lane string) error {
	body := fmt.Sprintf(
		`{"title":"lane pin","text":"run the comparison task on the named lane","pinned_lane":%q}`, lane)
	_, err := h.sur.Submit(context.Background(), userID, json.RawMessage(body))
	return err
}

// TestLN9LanePinToAnUnpinnableLaneRefusesAtTheBoundary — brief R3.
//
// A pin naming a lane this world holds no coverage for must be refused at
// Submit, as a mapped *api.SurfaceError, before the task/run/event birth
// transaction — the `Request.Project` refusal shape (internal/intake/
// pipeline.go:216-217: a pinned request never degrades).
//
// RED TODAY because `pinned_lane` is not a member of submitBody: the member is
// dropped, Submit answers 200, and a task is born on whatever lane routing
// would have picked anyway. That is the silent fallback R3 forbids.
func TestLN9LanePinToAnUnpinnableLaneRefusesAtTheBoundary(t *testing.T) {
	h := newProjectHarness(t)

	for _, lane := range []string{"kimi-cli", "kimi", "zai", "no-such-lane"} {
		t.Run(lane, func(t *testing.T) {
			before := countTasks(t, h)

			err := submitRawLanePin(h, "alice", lane)
			if err == nil {
				t.Fatalf("Submit pinned to %q succeeded — a pin the platform cannot honor must refuse "+
					"loudly at the boundary (brief R3), never fall back to routing's own choice", lane)
			}
			var se *api.SurfaceError
			if !errors.As(err, &se) {
				t.Fatalf("err = %v (%T), want a mapped *api.SurfaceError so the refusal reaches HTTP "+
					"with a status and a code (the mapIntakeErr table, internal/stage/surface.go:39)", err, err)
			}
			if se.Status < 400 || se.Status > 499 {
				t.Fatalf("refusal status = %d, want a 4xx — an unhonorable pin is a bad REQUEST, "+
					"never a platform defect (CONVENTIONS §30)", se.Status)
			}
			if se.Msg == "" {
				t.Fatalf("refusal carries no message: the operator must be told which lanes ARE pinnable, " +
					"the way the plan-budget verb names the windows that exist " +
					"(internal/api/meters_verbs.go:407-410)")
			}
			if after := countTasks(t, h); after != before {
				t.Fatalf("a refused pin born %d task row(s) — no task is born believing it got a lane "+
					"it did not get (the Request.Project precedent)", after-before)
			}
		})
	}
}

// TestLN9LanePinToLocalRefusesWithItsOwnReason — brief R8(b).
//
// `local` is NOT a pinnable lane at v0 and its refusal may not borrow the
// subscription-gap wording. It is absent from the lane registry entirely
// (internal/adapters/opencode/lanedata/ holds kimi, kimi-cli, zai), so
// Coverage.laneCovered("local") is false for a reason that has nothing to do
// with flat-rate coverage: the local ENGINE lane has no v0 consumer because no
// local provider entry is commissioned (internal/worker/routing.go:640-665;
// S12.1 class (a)). Honoring such a pin could only mean riding the paid seat —
// the silent fallback R3 forbids — so it refuses, and says why in its own
// words rather than telling the operator to buy a subscription they hold.
//
// RED TODAY for the same one reason as the test above.
func TestLN9LanePinToLocalRefusesWithItsOwnReason(t *testing.T) {
	h := newProjectHarness(t)
	before := countTasks(t, h)

	err := submitRawLanePin(h, "alice", "local")
	if err == nil {
		t.Fatalf("Submit pinned to the local tier succeeded — the local engine lane carries no v0 " +
			"consumer, so honoring the pin could only mean riding the paid seat (brief R8(b))")
	}
	var se *api.SurfaceError
	if !errors.As(err, &se) {
		t.Fatalf("err = %v (%T), want a mapped *api.SurfaceError", err, err)
	}
	if after := countTasks(t, h); after != before {
		t.Fatalf("a refused local pin born %d task row(s)", after-before)
	}
}
