package stage_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
)

// P3-RW-1 (brief §8, OQ1 = REJECT LOUDLY): the surface-level refusal contract.
// An invalid pin is a 4xx through the mapIntakeErr table and NO task is born —
// the requester asked for a specific project; a task that silently lost it is
// the exact failure the Projects-tab door exists to kill (S15.2: the browser is
// a display, never an authority). The unknown and not-visible classes must be
// INDISTINGUISHABLE in the response; a visible-but-inactive entry may say so.
//
// Fixture: pinFixture (projectpin_e2e_test.go) — shop ACTIVE (owner alice,
// member bob), attic ACTIVE (owner dana), draft PENDING (owner alice).

func countTasks(t *testing.T, h *projHarness) int {
	t.Helper()
	var n int
	if err := h.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM tasks`).Scan(&n); err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	return n
}

func submitRawPin(h *projHarness, userID, pin string) error {
	body := fmt.Sprintf(`{"title":"wishlist","text":"add a wishlist option for signed-in shoppers","project":%q}`, pin)
	_, err := h.sur.Submit(context.Background(), userID, json.RawMessage(body))
	return err
}

// TestProjectPinRefusalIsLoud pins the status/code of each refusal class and
// proves a refused Submit births nothing.
func TestProjectPinRefusalIsLoud(t *testing.T) {
	h := newProjectHarness(t)
	pinFixture(t, h)

	cases := []struct {
		name, user, pin, code string
		status                int
	}{
		{"unknown_project", "alice", "no-such-project", "not_found", http.StatusNotFound},
		{"cross_user_project", "alice", "attic", "not_found", http.StatusNotFound},
		{"pending_project", "alice", "draft", "project_not_active", http.StatusConflict},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before := countTasks(t, h)
			err := submitRawPin(h, c.user, c.pin)
			if err == nil {
				t.Fatalf("Submit with an invalid pin %q succeeded — OQ1 is reject-loudly", c.pin)
			}
			var se *api.SurfaceError
			if !errors.As(err, &se) {
				t.Fatalf("err = %v (%T), want a mapped *api.SurfaceError", err, err)
			}
			if se.Status != c.status || se.Code != c.code {
				t.Fatalf("mapped refusal = %d/%q, want %d/%q", se.Status, se.Code, c.status, c.code)
			}
			if after := countTasks(t, h); after != before {
				t.Fatalf("a refused Submit born %d task row(s) — no task is born from a refused pin", after-before)
			}
		})
	}
}

// TestProjectPinRefusalLeaksNoExistence: the unknown-id and the not-mine
// refusals must be one response shape — otherwise Submit is a project-existence
// oracle for other people's projects.
func TestProjectPinRefusalLeaksNoExistence(t *testing.T) {
	h := newProjectHarness(t)
	pinFixture(t, h)

	var unknown, foreign *api.SurfaceError
	if err := submitRawPin(h, "alice", "no-such-project"); !errors.As(err, &unknown) {
		t.Fatalf("unknown pin: err = %v, want a SurfaceError", err)
	}
	if err := submitRawPin(h, "alice", "attic"); !errors.As(err, &foreign) {
		t.Fatalf("cross-user pin: err = %v, want a SurfaceError", err)
	}
	if unknown.Status != foreign.Status || unknown.Code != foreign.Code {
		t.Fatalf("distinguishable refusals: unknown=%d/%q foreign=%d/%q",
			unknown.Status, unknown.Code, foreign.Status, foreign.Code)
	}
	a := strings.ReplaceAll(unknown.Msg, "no-such-project", "PIN")
	b := strings.ReplaceAll(foreign.Msg, "attic", "PIN")
	if a != b {
		t.Fatalf("refusal messages distinguish existence:\n  unknown: %s\n  foreign: %s", a, b)
	}
}

// TestProjectPinIsDurablyRecorded reads the EVENT ROW (not LoadState): the pin
// rides Request onto every intake.state payload, so the durable Stage-0 record
// carries what the requester submitted — with no schema change and no new
// column (CONVENTIONS §14 event-sourced state; the B6-7 Inputs precedent).
func TestProjectPinIsDurablyRecorded(t *testing.T) {
	h := newProjectHarness(t)
	pinFixture(t, h)
	ctx := context.Background()

	latestState := func(taskID string) string {
		t.Helper()
		var payload string
		if err := h.db.QueryRowContext(ctx, `
			SELECT payload FROM run_events
			 WHERE run_id = ? AND type = 'intake.state'
			 ORDER BY event_seq DESC LIMIT 1`, taskID+".intake").Scan(&payload); err != nil {
			t.Fatalf("read intake.state event: %v", err)
		}
		return payload
	}
	// The request object of the event payload, decoded loosely so an ABSENT
	// key is distinguishable from an empty one (the additive proof).
	requestObj := func(payload string) map[string]any {
		t.Helper()
		var ev struct {
			Request map[string]any `json:"request"`
		}
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			t.Fatalf("decode intake.state payload: %v", err)
		}
		return ev.Request
	}

	// Pinned: the submitted id is on the record.
	st, err := submitPin(t, h, "alice", "shop")
	if err != nil {
		t.Fatalf("Submit(pinned): %v", err)
	}
	payload := latestState(st.TaskID)
	if got := requestObj(payload)["project"]; got != "shop" {
		t.Fatalf("intake.state request.project = %v, want shop\npayload: %s", got, payload)
	}

	// Unpinned: byte-additive — the field is absent from the record entirely
	// (omitempty), so an existing consumer of the payload sees today's shape.
	raw, err := h.sur.Submit(ctx, "alice", json.RawMessage(
		`{"title":"checkout","text":"fix the shop backend checkout flow"}`))
	if err != nil {
		t.Fatalf("Submit(unpinned): %v", err)
	}
	v := decodeView(t, raw)
	unpinned := latestState(v.TaskID)
	if _, present := requestObj(unpinned)["project"]; present {
		t.Fatalf("an unpinned submission recorded a request.project key — the field must be omitempty:\n%s", unpinned)
	}
}
