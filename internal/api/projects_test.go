package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
)

// projects_test.go — P3-RW-2 acceptance tests for the S13.7/S15.2 projects
// HTTP family, COMMITTED RED at grounding (CONVENTIONS §3 Amendment-A: the
// routes do not exist yet — every test here fails on the mux's own 404 — and
// the packet's implementation commit closes the window).
//
// WHAT BINDS AND WHAT DOES NOT: the ASSERTIONS bind — they are derived from
// Spec S13.7 (registry content + pending→active lifecycle + owner/member
// visibility), S15.2 (server-side authority; the browser is a display) and the
// landed PinForIntake refusal discipline (an unknown id and an invisible entry
// are ONE answer). The response field NAMES are deliberately not pinned — the
// checks are substring-level so the executor owns the wire shapes within the
// brief's proposed routes (P3/briefs/P3-RW-2.md §3). The env builder below is
// a SEAM: the executor extends `server()` with whatever Config field the
// onboard door needs (the IntakeSurface wiring precedent) and may compose the
// real stage skeleton for the POST test — the observable effects asserted
// against the store/DB are the door's contract and may not be faked.

// projEnv is the projects-family fixture: a migrated backend, the four
// identities the visibility rules need (operator, owner, invited member,
// member of nothing), and the real S13.7 registry store.
type projEnv struct {
	t    *testing.T
	b    *backend
	ctx  context.Context
	proj *project.Store
}

func newProjEnv(t *testing.T) *projEnv {
	t.Helper()
	ctx := context.Background()
	b := newBackend(t)
	if err := b.store.CreateUser(ctx, "", auth.User{ID: "op", DisplayName: "Op", Role: auth.RoleOperator}, dlvPIN); err != nil {
		t.Fatalf("create operator: %v", err)
	}
	for _, id := range []string{"alice", "bob", "carol"} {
		if err := b.store.CreateUser(ctx, "op", auth.User{ID: id, DisplayName: id, Role: auth.RoleMember}, dlvPIN); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	proj, err := project.New(project.Config{DB: b.db, Log: b.log, Root: filepath.Join(t.TempDir(), "projects")})
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	return &projEnv{t: t, b: b, ctx: ctx, proj: proj}
}

// server builds the per-caller server. EXECUTOR SEAM: wire the onboard-door
// surface (and anything else the family's Config needs) HERE, in one place.
func (e *projEnv) server(who string) *api.Server {
	e.t.Helper()
	return api.New(api.Config{
		Log:      e.b.log,
		Sessions: e.b.store,
		Auth:     fixedIdentity{who},
		Settings: approvalSettings(),
		HealthFn: func() api.Health { return api.Health{Ready: true, Mode: "running", Version: "test"} },
		DB:       e.b.db,
		Meter:    fakeMeter{},
	})
}

func (e *projEnv) do(t *testing.T, who, method, path, body string) (int, string) {
	t.Helper()
	rr := httptest.NewRecorder()
	e.server(who).Handler().ServeHTTP(rr, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rr.Code, rr.Body.String()
}

// seedActive registers, captures and activates one entry at the data layer
// (the pin_test.go precedent — Register → Capture → Activate needs no git and
// takes MEMBERS, which the git-backed Onboard path does not).
func (e *projEnv) seedActive(t *testing.T, id, owner string, members []string, in project.CaptureInput) {
	t.Helper()
	if _, err := e.proj.Register(e.ctx, project.RegisterInput{
		ProjectID: id, Owner: owner, Name: id,
		StorePath: filepath.Join(e.t.TempDir(), id), Members: members,
	}); err != nil {
		t.Fatalf("Register %s: %v", id, err)
	}
	in.ProjectID, in.By = id, owner
	if _, err := e.proj.Capture(e.ctx, in); err != nil {
		t.Fatalf("Capture %s: %v", id, err)
	}
	if _, err := e.proj.Activate(e.ctx, id, owner); err != nil {
		t.Fatalf("Activate %s: %v", id, err)
	}
}

// TestProjectsListIsCallerScoped — Spec S13.7 ("owning user, invited members")
// + S15.2 (visibility enforced server-side): GET /api/projects answers each
// caller with the entries they own or belong to, and NEVER with anybody
// else's. Both directions are non-tautological: the caller's own entry must be
// PRESENT (an empty answer cannot pass for a scoped one, §38).
func TestProjectsListIsCallerScoped(t *testing.T) {
	e := newProjEnv(t)
	e.seedActive(t, "p-alpha", "alice", []string{"bob"}, project.CaptureInput{
		Conventions: []string{"p-alpha-convention: tabs, never spaces"},
		Commands:    project.Commands{Test: "go test ./..."},
		DangerZones: []project.DangerZone{{Path: "deploy/", Rule: "never touch deploy"}},
	})
	e.seedActive(t, "p-gamma", "carol", nil, project.CaptureInput{
		Conventions: []string{"p-gamma-convention"},
	})

	for _, tc := range []struct {
		who       string
		sees      string
		neverSees string
	}{
		{"alice", "p-alpha", "p-gamma"}, // owner
		{"bob", "p-alpha", "p-gamma"},   // invited member
		{"carol", "p-gamma", "p-alpha"}, // owner of the other; member of nothing here
	} {
		code, out := e.do(t, tc.who, "GET", "/api/projects", "")
		if code != http.StatusOK {
			t.Fatalf("GET /api/projects as %s: status %d (want 200): %s", tc.who, code, out)
		}
		if !strings.Contains(out, tc.sees) {
			t.Errorf("as %s the list omits %s — the caller's own entry must be present: %s", tc.who, tc.sees, out)
		}
		if strings.Contains(out, tc.neverSees) {
			t.Errorf("as %s the list leaks %s — visibility is owner-or-member, server-side (S15.2): %s", tc.who, tc.neverSees, out)
		}
	}
}

// TestProjectDetailServesCapturedContentAndRefusesWithOneAnswer — Spec S13.7:
// the entry's captured conventions, commands and danger zones are the detail a
// card renders (product map §3 Projects); the OWNER sees the capture. The
// refusal discipline is the landed PinForIntake / §38 shape: an id the caller
// cannot see answers exactly like an id that does not exist (404, no
// existence oracle), and no captured content rides a refusal.
func TestProjectDetailServesCapturedContentAndRefusesWithOneAnswer(t *testing.T) {
	e := newProjEnv(t)
	e.seedActive(t, "p-alpha", "alice", []string{"bob"}, project.CaptureInput{
		Conventions: []string{"p-alpha-convention: tabs, never spaces"},
		Commands:    project.Commands{Test: "go test ./..."},
		DangerZones: []project.DangerZone{{Path: "deploy/", Rule: "never touch deploy"}},
	})

	// The owner reads the full S13.7 capture. (Member DEPTH is brief OQ4; the
	// member's 200 on the ENTRY is asserted, its capture depth is not.)
	code, out := e.do(t, "alice", "GET", "/api/projects/p-alpha", "")
	if code != http.StatusOK {
		t.Fatalf("owner detail: status %d (want 200): %s", code, out)
	}
	for _, want := range []string{"p-alpha-convention", "go test ./...", "deploy/"} {
		if !strings.Contains(out, want) {
			t.Errorf("owner detail omits %q — the S13.7 capture is the card's content: %s", want, out)
		}
	}
	if code, out := e.do(t, "bob", "GET", "/api/projects/p-alpha", ""); code != http.StatusOK || !strings.Contains(out, "p-alpha") {
		t.Errorf("invited member must read the entry (S13.7 members): status %d: %s", code, out)
	}

	// One refusal for stranger and for unknown — told apart by nothing.
	strangerCode, strangerOut := e.do(t, "carol", "GET", "/api/projects/p-alpha", "")
	unknownCode, _ := e.do(t, "carol", "GET", "/api/projects/p-nosuch", "")
	if strangerCode != http.StatusNotFound || unknownCode != http.StatusNotFound {
		t.Errorf("stranger=%d unknown=%d — both must be 404: an id that answers differently is an existence oracle (S15.2)", strangerCode, unknownCode)
	}
	if strings.Contains(strangerOut, "p-alpha-convention") {
		t.Errorf("a refusal carries captured content: %s", strangerOut)
	}
}

// TestOnboardStartDoorDraftsAPendingEntryWithItsOnboardingTask — Spec S13.7:
// "onboarding a repository is itself a task the platform performs" — the door
// runs register → clone/init → scan → draft over the EXISTING StartOnboarding
// seam and answers with the drafted entry and its approval reference. The
// observable contract asserted here is the door's real effect: a PENDING
// registry entry with its first capture, and the onboarding task attributed to
// the CALLER (identity from the session, never from the body — 15.6).
func TestOnboardStartDoorDraftsAPendingEntryWithItsOnboardingTask(t *testing.T) {
	e := newProjEnv(t)
	code, out := e.do(t, "alice", "POST", "/api/projects", `{"project_id":"p-new","name":"New Proj"}`)
	if code != http.StatusOK {
		t.Fatalf("POST /api/projects: status %d (want 200): %s", code, out)
	}
	if !strings.Contains(out, "p-new") {
		t.Errorf("the answer does not name the drafted entry: %s", out)
	}
	entry, err := e.proj.Get(e.ctx, "p-new")
	if err != nil {
		t.Fatalf("the door answered 200 but registered nothing: %v", err)
	}
	if entry.State != project.StatePending {
		t.Errorf("state %q — the entry activates only on the owner's D10 approval (S13.7)", entry.State)
	}
	if entry.CaptureVersion < 1 {
		t.Errorf("capture_version %d — the scan's draft is the entry's first capture", entry.CaptureVersion)
	}
	if entry.Owner != "alice" {
		t.Errorf("owner %q — the caller is the owner, from the session (15.6)", entry.Owner)
	}
	var taskOwner string
	if err := e.b.db.QueryRowContext(e.ctx,
		`SELECT user_id FROM tasks WHERE task_id = ?`, "onboard-p-new").Scan(&taskOwner); err != nil {
		t.Fatalf("no onboarding task row — onboarding is a task the platform performs (S13.7): %v", err)
	}
	if taskOwner != "alice" {
		t.Errorf("onboarding task owner %q, want the caller", taskOwner)
	}
}
