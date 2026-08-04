package api_test

// The B2-4 intake surface transport: identity riding, the S01.9 step-up
// wall (dev identity can never step up; a PIN verifies against the real
// auth store), SurfaceError status mapping, and the not-wired 503.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
)

// fakeSurface records calls and returns scripted results.
type fakeSurface struct {
	lastUser string
	// lastBody + calls are the B6-7 additions: the S15.7 handoff has to be
	// provable to have called the ONE landed ingress, with the requester's
	// identity and the requester-supplied inputs actually on the body.
	lastBody    json.RawMessage
	calls       int
	lastAsk     string
	lastPinOK   bool
	lastAnswer  json.RawMessage
	submitErr   error
	answerErr   error
	taskPayload json.RawMessage
	artifacts   json.RawMessage
}

func (f *fakeSurface) Submit(_ context.Context, userID string, body json.RawMessage) (json.RawMessage, error) {
	f.lastUser, f.lastBody = userID, body
	f.calls++
	if f.submitErr != nil {
		return nil, f.submitErr
	}
	return json.RawMessage(`{"task_id":"t-1"}`), nil
}

func (f *fakeSurface) Answer(_ context.Context, userID, askID string, answer json.RawMessage, pinVerified bool) (json.RawMessage, error) {
	f.lastUser, f.lastAsk, f.lastAnswer, f.lastPinOK = userID, askID, answer, pinVerified
	if f.answerErr != nil {
		return nil, f.answerErr
	}
	return json.RawMessage(`{"ok":true}`), nil
}

func (f *fakeSurface) Task(context.Context, string) (json.RawMessage, error) {
	if f.taskPayload != nil {
		return f.taskPayload, nil
	}
	return json.RawMessage(`{"task_id":"t-1"}`), nil
}

func (f *fakeSurface) Artifacts(context.Context, string) (json.RawMessage, error) {
	if f.artifacts != nil {
		return f.artifacts, nil
	}
	return nil, &api.SurfaceError{Status: http.StatusNotFound, Code: "not_found", Msg: "no drafted artifact pair yet"}
}

func (f *fakeSurface) Receipt(context.Context, string) (json.RawMessage, error) {
	return json.RawMessage(`{"run_id":"r-1"}`), nil
}

func (f *fakeSurface) Advance(_ context.Context, userID, taskID string) (json.RawMessage, error) {
	f.lastUser = userID
	return json.RawMessage(`{"task_id":"` + taskID + `"}`), nil
}

// staticAuth resolves every request to a fixed identity.
type staticAuth struct{ id api.Identity }

func (a staticAuth) Authenticate(*http.Request) (api.Identity, error) { return a.id, nil }

func intakeServer(t *testing.T, surface api.IntakeSurface, id api.Identity, b *backend) *api.Server {
	t.Helper()
	if b == nil {
		b = newBackend(t)
	}
	return api.New(api.Config{
		Log:        b.log,
		Sessions:   b.store,
		DevPosture: true,
		Auth:       staticAuth{id: id},
		Settings:   fixedSettings{d: 20 * time.Second},
		HealthFn:   func() api.Health { return api.Health{Ready: true} },
		// The read routes are owner-scoped in the data layer (B6-1 R6/R17), so
		// the transport tests wire the DB the scope resolves against.
		DB:     b.db,
		Intake: surface,
	})
}

func TestIntakeSubmitRidesIdentity(t *testing.T) {
	f := &fakeSurface{}
	srv := intakeServer(t, f, api.Identity{UserID: "alice"}, nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("POST", "/api/intake/requests",
		strings.NewReader(`{"title":"x","text":"y"}`)))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body)
	}
	if f.lastUser != "alice" {
		t.Fatalf("surface saw user %q, want the authenticated identity", f.lastUser)
	}
}

func TestIntakeAnswerWithoutPINPassesUnverified(t *testing.T) {
	f := &fakeSurface{}
	srv := intakeServer(t, f, api.Identity{UserID: "alice"}, nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("POST", "/api/asks/a-1/answer",
		strings.NewReader(`{"answer":{"action":"approve"}}`)))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body)
	}
	if f.lastPinOK {
		t.Fatal("pinVerified true without a PIN")
	}
	if f.lastAsk != "a-1" || !strings.Contains(string(f.lastAnswer), "approve") {
		t.Fatalf("surface saw ask=%q answer=%s", f.lastAsk, f.lastAnswer)
	}
}

func TestIntakeAnswerDevIdentityCannotStepUp(t *testing.T) {
	f := &fakeSurface{}
	srv := intakeServer(t, f, api.Identity{UserID: "dev", Dev: true}, nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("POST", "/api/asks/a-1/answer",
		strings.NewReader(`{"pin":"1234","answer":{"action":"approve"}}`)))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403 (S01.9: the dev identity is resolution-only)", rr.Code)
	}
	if f.lastAsk != "" {
		t.Fatal("surface reached despite refused step-up")
	}
}

func TestIntakeAnswerPINVerifiesAgainstAuthStore(t *testing.T) {
	b := newBackend(t)
	ctx := context.Background()
	if err := b.store.CreateUser(ctx, "", auth.User{ID: "op", DisplayName: "Op", Role: auth.RoleOperator}, "hunter2hunter"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	f := &fakeSurface{}
	srv := intakeServer(t, f, api.Identity{UserID: "op"}, b)

	// Wrong PIN → 401, surface untouched.
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("POST", "/api/asks/a-1/answer",
		strings.NewReader(`{"pin":"wrong","answer":{"action":"approve"}}`)))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("wrong PIN: status %d, want 401", rr.Code)
	}
	if f.lastAsk != "" {
		t.Fatal("surface reached despite failed PIN")
	}

	// Right PIN → pinVerified rides through.
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("POST", "/api/asks/a-1/answer",
		strings.NewReader(`{"pin":"hunter2hunter","answer":{"action":"approve"}}`)))
	if rr.Code != http.StatusOK {
		t.Fatalf("right PIN: status %d: %s", rr.Code, rr.Body)
	}
	if !f.lastPinOK {
		t.Fatal("pinVerified false after a successful step-up")
	}
}

func TestIntakeSurfaceErrorMapsStatus(t *testing.T) {
	f := &fakeSurface{answerErr: &api.SurfaceError{Status: http.StatusNotFound, Code: "not_found", Msg: "no such ask"}}
	srv := intakeServer(t, f, api.Identity{UserID: "alice"}, nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("POST", "/api/asks/a-x/answer",
		strings.NewReader(`{"answer":{}}`)))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status %d, want mapped 404", rr.Code)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil || body.Error != "not_found" {
		t.Fatalf("body %s", rr.Body)
	}
}

// TestIntakeNotWiredIs503 covers the routes that genuinely need the pipeline
// surface. The S15.2 task DETAIL is not one of them any more: it projects the
// task from the DB and renders the missing artifacts as an honest absence
// (B6-1 R5), so its not-wired case is a task rendered without a spec, not a
// 503 that hides the task.
func TestIntakeNotWiredIs503(t *testing.T) {
	srv := intakeServer(t, nil, api.Identity{UserID: "alice"}, nil)
	for _, route := range []struct{ method, path, body string }{
		{"POST", "/api/intake/requests", `{"title":"x","text":"y"}`},
		{"GET", "/api/runs/r-1/receipt", ""},
	} {
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, httptest.NewRequest(route.method, route.path, strings.NewReader(route.body)))
		if rr.Code != http.StatusServiceUnavailable {
			t.Errorf("%s %s: status %d, want 503 when the surface is not wired", route.method, route.path, rr.Code)
		}
	}
}
