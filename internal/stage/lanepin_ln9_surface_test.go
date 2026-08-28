package stage_test

// lanepin_ln9_surface_test.go — P3-LN-9 §9.4 (S15.2, S00.9 A13, §30).
//
// The GREEN successor of the two tests the grounding committed red
// (lanepin_ln9_test.go), extended with what only exists once the refusal class
// does: the exact status and code, the message that names what IS pinnable, and
// — the half a refusal test is worthless without — the INVERSE CONTROL. A
// refusal that refuses everything is not a fix, so the same body that is
// refused in a world holding one lane is ACCEPTED in a world holding two.
//
// The grounding file is left exactly as committed; nothing here weakens it.
//
// $0: no provider call on any path.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
)

// ln9SubmitPin submits a body carrying a lane pin through the production verb.
func ln9SubmitPin(sur interface {
	Submit(context.Context, string, json.RawMessage) (json.RawMessage, error)
}, userID, lane string) error {
	body := fmt.Sprintf(
		`{"title":"lane pin","text":"run the comparison task on the named lane","pinned_lane":%q}`, lane)
	_, err := sur.Submit(context.Background(), userID, json.RawMessage(body))
	return err
}

// TestLN9PinRefusalCarriesItsStatusCodeAndTheLanesThatExist pins the refusal
// CONTRACT: one code for every unhonorable pin, distinct detail sentences, and
// a message an operator can act on without reading the source.
func TestLN9PinRefusalCarriesItsStatusCodeAndTheLanesThatExist(t *testing.T) {
	h := newProjectHarness(t)

	// This world holds exactly the configured lane. Everything else refuses.
	//
	// Two DETAIL sentences under one code, and which one you get says something
	// real. A lane the platform has never heard of is answered by SET
	// MEMBERSHIP, which is the boundary's own job and no domain rule at all. A
	// lane the platform KNOWS and still cannot dispatch to — the local engine
	// lane — is answered by the selection layer's carried verdict, quoted.
	for _, tc := range []struct {
		lane      string
		wantWords []string
	}{
		{"zai", []string{`"zai"`, "Pinnable lanes:", `"anthropic"`}},
		{"kimi", []string{`"kimi"`, "Pinnable lanes:", `"anthropic"`}},
		{"kimi-cli", []string{`"kimi-cli"`, "Pinnable lanes:", `"anthropic"`}},
		{"no-such-lane", []string{`"no-such-lane"`, "Pinnable lanes:", `"anthropic"`}},
		// The local ENGINE lane refuses in its OWN words, citing the absent
		// provider entry rather than a subscription the operator already holds.
		{"local", []string{"nothing is set up", "paid model", `"anthropic"`}},
	} {
		t.Run(tc.lane, func(t *testing.T) {
			before := countTasks(t, h)
			err := ln9SubmitPin(h.sur, "alice", tc.lane)
			if err == nil {
				t.Fatalf("Submit pinned to %q succeeded in a world that holds only the configured lane", tc.lane)
			}
			var se *api.SurfaceError
			if !errors.As(err, &se) {
				t.Fatalf("err = %v (%T), want a mapped *api.SurfaceError", err, err)
			}
			if se.Status != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 — an unhonorable pin is a bad REQUEST, never a platform "+
					"defect (§30)", se.Status)
			}
			if se.Code != "lane_pin_refused" {
				t.Errorf("code = %q, want lane_pin_refused — ONE code for every unhonorable pin, with the "+
					"DETAIL distinguishing the cases (OQ-5)", se.Code)
			}
			for _, want := range tc.wantWords {
				if !strings.Contains(se.Msg, want) {
					t.Errorf("the refusal does not say %q: %q", want, se.Msg)
				}
			}
			if after := countTasks(t, h); after != before {
				t.Fatalf("a refused pin born %d task row(s) — the refusal must land BEFORE the task/run/event "+
					"birth transaction", after-before)
			}
		})
	}

	// The local refusal must not borrow the subscription-gap wording, which
	// would tell an operator to buy something they already hold — and it must
	// not be the same sentence as the unknown-lane one, or the two situations
	// (different remedies, different reader) would read identically.
	var local, unknown *api.SurfaceError
	if !errors.As(ln9SubmitPin(h.sur, "alice", "local"), &local) ||
		!errors.As(ln9SubmitPin(h.sur, "alice", "no-such-lane"), &unknown) {
		t.Fatal("expected both refusals to be mapped surface errors")
	}
	if strings.Contains(local.Msg, "Subscription gap") {
		t.Errorf("the local refusal borrowed the 2.7 subscription-gap wording: %q", local.Msg)
	}
	if local.Msg == unknown.Msg {
		t.Error("the local-lane and unknown-lane refusals are the same sentence — one code, DISTINCT detail (OQ-5)")
	}
	if local.Code != unknown.Code {
		t.Errorf("two codes were minted (%q vs %q) — OQ-5 is ONE code with distinct detail", local.Code, unknown.Code)
	}
}

// TestLN9PinToACoveredLaneIsAccepted is the INVERSE CONTROL. Without it the
// guard above is satisfied by a boundary that refuses every pin ever submitted,
// which would be a worse platform than the one before this packet.
func TestLN9PinToACoveredLaneIsAccepted(t *testing.T) {
	// The configured lane is pinnable even with nothing commissioned: an
	// operator may name the lane they are already on, deliberately.
	h := newProjectHarness(t)
	before := countTasks(t, h)
	if err := ln9SubmitPin(h.sur, "alice", adapters.LaneAnthropic); err != nil {
		t.Fatalf("a pin to the CONFIGURED lane was refused: %v", err)
	}
	if after := countTasks(t, h); after != before+1 {
		t.Fatalf("an accepted pin born %d task rows, want 1", after-before)
	}

	// And in a world that holds a second lane, that lane is pinnable too —
	// which is the world the operator's ordered head-to-head needs.
	c := newLN9Harness(t)
	insertUser(t, c.db, "alice", "member")
	commissionedBefore := countTasks(t, c.projectHarnessShim())
	if err := ln9SubmitPin(c.sur, "alice", adapters.LaneZAI); err != nil {
		t.Fatalf("a pin to the COMMISSIONED lane was refused in a world that holds it: %v", err)
	}
	if after := countTasks(t, c.projectHarnessShim()); after != commissionedBefore+1 {
		t.Fatalf("an accepted pin born %d task rows, want 1", after-commissionedBefore)
	}
	// The same body is still refused for a lane this second world does not
	// hold, so acceptance did not become blanket permission.
	if err := ln9SubmitPin(c.sur, "alice", "kimi-cli"); err == nil {
		t.Error("the commissioned world admitted a pin to a lane it does not hold — a boundary that admits " +
			"everything once anything is commissioned is not a boundary")
	}
}

// projectHarnessShim lets countTasks — which takes the project harness — read
// this harness's database. countTasks is a pre-existing helper and is not
// edited; this adapts to it instead.
func (h *ln9Harness) projectHarnessShim() *projHarness {
	return &projHarness{harness: h.harness}
}
