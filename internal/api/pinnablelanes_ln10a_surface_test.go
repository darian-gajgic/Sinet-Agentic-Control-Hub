package api_test

// pinnablelanes_ln10a_surface_test.go — P3-LN-10a, the family postures of
// GET /api/intake/pinnable-lanes (brief R6, R7, R1).
//
// These three compile against TODAY's tree and are RED for one reason: the
// route is not registered, so the mux answers 404 where each of them wants a
// different, deliberate answer. The row SHAPE and the composed order need
// api.Config.PinnableLanes to exist and land beside the implementation
// (pinnablelanes_ln10a_shape_test.go) — a tests-first commit that does not
// build is a commit whose tests are not red for the right reason (§65 D10).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
)

const pinnableLanesPath = "/api/intake/pinnable-lanes"

// TestLN10aEmptySetIsHonestNotAnError — brief R6.
//
// An intake surface IS wired and the composed set is empty: that is a real
// state of a real platform, and the refusal path already has words for it
// ("none — this platform holds no pinnable lane at all"). It answers 200 with
// an EMPTY LIST, and the list is `[]` rather than `null`: a picker reading
// `null` sees a broken read where the truth is "no lane is pinnable here".
func TestLN10aEmptySetIsHonestNotAnError(t *testing.T) {
	srv := intakeServer(t, &fakeSurface{}, api.Identity{UserID: "alice"}, nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", pinnableLanesPath, nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d (want 200) — a wired surface with no pinnable lane is a real state, not an error: %s",
			rr.Code, rr.Body.String())
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not a JSON object: %v\n%s", err, rr.Body.String())
	}
	if got := string(body["lanes"]); got != "[]" {
		t.Errorf(`"lanes" = %s, want the literal [] — a null reads as a broken read rather than as "no lane is pinnable"`, got)
	}
	// The family's caching posture: a set composed from the running world is
	// never a body a browser may hold onto (writeSurface, R7).
	if got := rr.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want %q like every sibling on this surface", got, "no-store")
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
}

// TestLN10aNotWiredIsTheFamilys503 — brief R7.
//
// A process with NO intake surface must not answer "there are no pinnable
// lanes": Submit itself answers 503 in that process, so an empty list here
// would be a claim about a world nobody can file a task in. It takes the
// family's own not_wired refusal instead — the dishonest-absence class this
// platform refuses everywhere (§65).
func TestLN10aNotWiredIsTheFamilys503(t *testing.T) {
	srv := intakeServer(t, nil, api.Identity{UserID: "alice"}, nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", pinnableLanesPath, nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d (want 503) — a process with no intake surface must refuse, never report an empty set: %s",
			rr.Code, rr.Body.String())
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not a JSON object: %v\n%s", err, rr.Body.String())
	}
	if body.Error != "not_wired" {
		t.Errorf(`"error" = %q, want "not_wired" — the code every sibling on this surface answers with`, body.Error)
	}
}

// TestLN10aReadRequiresASession — brief R1, the fail-closed limb.
//
// The route is registered through `protected`, exactly like POST
// /api/intake/requests beside it: no resolved identity is 401. This server
// wires no Auth and is not in dev posture, so nothing attaches an identity.
func TestLN10aReadRequiresASession(t *testing.T) {
	b := newBackend(t)
	srv := api.New(api.Config{
		Log:      b.log,
		Sessions: b.store,
		Settings: fixedSettings{d: 20 * 1e9},
		HealthFn: func() api.Health { return api.Health{Ready: true} },
		DB:       b.db,
		Intake:   &fakeSurface{},
	})
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", pinnableLanesPath, nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status %d with no session, want 401 — the read sits behind requireIdentity like every sibling", rr.Code)
	}
}
