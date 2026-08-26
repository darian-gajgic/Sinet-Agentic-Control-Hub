package stage_test

// pinnablelanes_ln10a_test.go — P3-LN-10a, the ONE-SET property, end to end.
//
// The whole reason this read exists is that the lane picker must enumerate from
// the running world rather than hardcode a second spelling. That is only true
// if the set the transport SERVES and the set the boundary REFUSES against are
// one value, so this drives both halves of a composed skeleton and compares
// them by behavior: every lane the served body marks pinnable is a lane Submit
// admits, and every lane it marks not-pinnable is a lane Submit refuses.
//
// The world is the LN-9 harness — anthropic configured, zai commissioned, the
// local engine lane appended — because a pin means nothing in a world with one
// lane. $0: the claude-cli fake engine and a recording adapter that starts
// nothing; no submission here is ever dispatched.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
)

type ln10aRow struct {
	Lane        string `json:"lane"`
	Pinnable    bool   `json:"pinnable"`
	NotPinnable string `json:"not_pinnable"`
}

// TestLN10aServedSetIsTheSetTheBoundaryHonors — brief R2, the property the
// picker rests on.
func TestLN10aServedSetIsTheSetTheBoundaryHonors(t *testing.T) {
	h := newLN9Harness(t)
	ctx := context.Background()

	// The transport is handed the SKELETON's composed value, exactly as the
	// composition root hands it (internal/shell): no second call to the
	// producer anywhere on this path.
	composed := h.sk.PinnableLaneOptions()
	srv := api.New(api.Config{
		Log:           h.log,
		Sessions:      auth.New(h.db, h.log),
		DevPosture:    true,
		HealthFn:      func() api.Health { return api.Health{Ready: true} },
		DB:            h.db,
		Intake:        h.sur,
		PinnableLanes: composed,
	})
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/api/intake/pinnable-lanes", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/intake/pinnable-lanes: status %d: %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Lanes []ln10aRow `json:"lanes"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not the object envelope: %v\n%s", err, rr.Body.String())
	}

	// The served rows ARE the composed rows: same lanes, same verdicts, same
	// order. A body that agreed on membership but not on order would still be
	// a second spelling of something the composition owns.
	var served, want [][2]any
	for _, r := range body.Lanes {
		served = append(served, [2]any{r.Lane, r.Pinnable})
	}
	for _, o := range composed {
		want = append(want, [2]any{o.Lane, o.Pinnable})
	}
	if !reflect.DeepEqual(served, want) {
		t.Fatalf("the served set is not the skeleton's composed set.\nserved: %v\ncomposed: %v", served, want)
	}

	// NON-VACUITY, both directions: a world where nothing was pinnable, or
	// where nothing was refused, would satisfy the loop below for the wrong
	// reason.
	var pinnable, refused int
	for _, r := range body.Lanes {
		if r.Pinnable {
			pinnable++
		} else {
			refused++
		}
	}
	if pinnable == 0 || refused == 0 {
		t.Fatalf("the world serves %d pinnable and %d not-pinnable rows — both halves are needed "+
			"for the comparison below to mean anything: %v", pinnable, refused, body.Lanes)
	}

	// And now the property itself, stated where a person meets it: the SUBMIT
	// verb. The picker offering a lane the boundary would refuse — or refusing
	// one it would honor — is the drift this read exists to make impossible.
	for _, r := range body.Lanes {
		t.Run(r.Lane, func(t *testing.T) {
			body := fmt.Sprintf(
				`{"title":"lane pin","text":"Write a short appreciation note about the SQLite database engine.","pinned_lane":%q}`,
				r.Lane)
			_, err := h.sur.Submit(ctx, "alice", json.RawMessage(body))
			if r.Pinnable {
				if err != nil {
					t.Fatalf("the read offers %q as pinnable and the boundary refused the submission: %v", r.Lane, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("the read marks %q not pinnable and the boundary ADMITTED it — the picker and the "+
					"submit verb disagree about the same set", r.Lane)
			}
			var se *api.SurfaceError
			if !errors.As(err, &se) {
				t.Fatalf("refusal for %q = %v (%T), want a mapped *api.SurfaceError", r.Lane, err, err)
			}
			if se.Status < 400 || se.Status > 499 {
				t.Fatalf("refusal status for %q = %d, want a 4xx", r.Lane, se.Status)
			}
			// The words a person reads at the boundary are the words the read
			// handed the picker. The boundary's message wraps that sentence in
			// its error class, so what is asserted is VERBATIM CARRIAGE rather
			// than byte-equality of the whole message: a paraphrase on either
			// side breaks it.
			if r.NotPinnable == "" {
				t.Fatalf("row %q is not pinnable and carries no reason", r.Lane)
			}
			if !strings.Contains(se.Msg, r.NotPinnable) {
				t.Errorf("the refusal a submitter reads does not carry the reason the picker was given.\n"+
					"boundary: %s\n    read: %s", se.Msg, r.NotPinnable)
			}
		})
	}
}
