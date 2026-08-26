package api_test

// P3-LN-10a: the pinnable-lanes read — committed RED by the grounding session
// (2026-08-27), turned green by the executor packet. The LN-10 lane picker
// must enumerate the lanes a task may pin FROM THE RUNNING WORLD, and no API
// serves that set: it is composed once at startup into
// intake.Pipeline.PinnableLanes (skeleton.go:199) and consumed only by the
// boundary refusal (route.go:174). The ratified read serves that value
// VERBATIM under the intake family, so the set the picker offers IS the set
// the boundary honors (§65/§66: one spelling, one predicate).
//
// This file asserts only what compiles against the CURRENT tree — the route
// exists and answers the family's envelope. The full contract (row shape,
// composed order, verbatim not_pinnable sentence, honest empty, not-wired
// 503, the golden fixture) is specified in P3/briefs/P3-LN-10a.md and lands
// with the executor's own tests beside this one.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
)

// TestLN10aPinnableLanesRouteIsServed is RED until the executor lands
// GET /api/intake/pinnable-lanes. It rides the family's own harness
// (intakeServer, intake_handlers_test.go): an authenticated session against a
// server whose intake surface IS wired must be answered — today the mux has
// no such route and this fails on status, which is the right reason.
func TestLN10aPinnableLanesRouteIsServed(t *testing.T) {
	srv := intakeServer(t, &fakeSurface{}, api.Identity{UserID: "alice"}, nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/api/intake/pinnable-lanes", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/intake/pinnable-lanes: status %d (want 200) — the pinnable-lanes read is not served: %s",
			rr.Code, rr.Body.String())
	}
	// The envelope is an object holding "lanes", never a bare array
	// (additive-first, S15.2): a top-level array can never grow a sibling key.
	var body struct {
		Lanes *json.RawMessage `json:"lanes"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not a JSON object: %v\n%s", err, rr.Body.String())
	}
	if body.Lanes == nil {
		t.Fatalf(`response carries no "lanes" member: %s`, rr.Body.String())
	}
}
