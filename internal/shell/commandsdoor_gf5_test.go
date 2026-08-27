package shell

// commandsdoor_gf5_test.go — P3-GF5 at the COMPOSITION ROOT: the S13.7
// project-Commands write door, composed the way shell.Run composes it, in
// front of the real project store and the real check-pack projection.
//
// COMMITTED RED (CONVENTIONS §3 Amendment-A): `commandsDoor`, the
// api.ProjectCommandsSurface seam and the store's EditCommands verb do not
// exist yet, so this file fails to compile against the pre-GF5 tree — the
// packet's implementation commit closes the window. New file, because the
// packet may not modify pre-existing test files (the project_onboard_seam_test
// precedent one file over).
//
// WHAT THIS FILE IS FOR, that neither the api battery nor the store battery can
// prove alone: GF4 landed the CONSUMPTION side — the pack is resolved from the
// registry's CURRENT capture on every judged round — and this packet lands the
// door that puts commands there. The claim the operator's r4-F1b finding makes
// is that those two meet: an owner types their build/test/lint commands into
// the product, and the next verification round runs them. Here that is one test
// over the REAL projection (packFromCapture, GF4's own), driven through the
// REAL HTTP handler, with nothing between them faked.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// commandsEnv is the door in its production composition: the real project
// store, the real commandsDoor over it, and the real api.Server in front.
type commandsEnv struct {
	ctx      context.Context
	db       *storage.DB
	log      *eventlog.Log
	reg      *settings.Registry
	root     string
	sessions *auth.Store
	proj     *project.Store
}

func newCommandsEnv(t *testing.T) *commandsEnv {
	t.Helper()
	ctx := context.Background()
	db, log, reg := seamDB(t)
	if err := reg.Attach(ctx, db, log); err != nil {
		t.Fatalf("settings Attach: %v", err)
	}
	sessions := auth.New(db, log)
	if err := sessions.CreateUser(ctx, "",
		auth.User{ID: "op", DisplayName: "Op", Role: auth.RoleOperator}, "hunter2hunter"); err != nil {
		t.Fatalf("bootstrap operator: %v", err)
	}
	for _, id := range []string{"alice", "bob"} {
		if err := sessions.CreateUser(ctx, "op",
			auth.User{ID: id, DisplayName: id, Role: auth.RoleMember}, "hunter2hunter"); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	root := filepath.Join(t.TempDir(), "projects")
	e := &commandsEnv{ctx: ctx, db: db, log: log, reg: reg, root: root, sessions: sessions}
	e.proj = e.store(t, nil)
	return e
}

// store opens another handle onto the SAME registry with its own clock. The
// clock is a construction-time seam (project.Config.Now), which is what lets
// the verified-on test below write two captures at two different times without
// any test-only mutator existing on the production store.
func (e *commandsEnv) store(t *testing.T, now func() time.Time) *project.Store {
	t.Helper()
	s, err := project.New(project.Config{DB: e.db, Log: e.log, Root: e.root, Now: now})
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	return s
}

// srv composes the api.Server exactly as shell.Run composes it. A nil door is
// the unwired posture, which must answer 503 rather than pretend.
func (e *commandsEnv) srv(who string, door api.ProjectCommandsSurface) *api.Server {
	return api.New(api.Config{
		Log: e.log, Sessions: e.sessions, Auth: fixedShellIdentity{who},
		Settings: e.reg, Registry: e.reg,
		HealthFn:        func() api.Health { return api.Health{Ready: true} },
		DB:              e.db,
		ProjectCommands: door,
	})
}

func (e *commandsEnv) do(t *testing.T, who, method, path, body string) (int, string) {
	t.Helper()
	rr := httptest.NewRecorder()
	e.srv(who, commandsDoor{proj: e.proj}).Handler().
		ServeHTTP(rr, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rr.Code, rr.Body.String()
}

// seedCommandless registers, captures (with NO command) and activates one
// project — the r4-F1 fresh-scaffold shape, whose tasks verify under the GF4
// bootstrap posture until this door captures commands.
func (e *commandsEnv) seedCommandless(t *testing.T, id, owner string, members ...string) {
	t.Helper()
	if _, err := e.proj.Register(e.ctx, project.RegisterInput{
		ProjectID: id, Owner: owner, Name: id, StorePath: filepath.Join(t.TempDir(), id), Members: members,
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, err := e.proj.Capture(e.ctx, project.CaptureInput{
		ProjectID: id, By: owner, Conventions: []string{"tabs, not spaces"},
		DangerZones: []project.DangerZone{{Path: "deploy/", Rule: "never touched by tasks", SourceHash: "abc123"}},
		ScanHash:    "scanhash-v1", Family: project.FamilySoftware,
	}); err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if _, err := e.proj.Activate(e.ctx, id, owner); err != nil {
		t.Fatalf("Activate: %v", err)
	}
}

// TestGF5EscapeFromBootstrapThroughTheDoor [brief T7]: r4's acceptance headline
// at the seam that decides it. BEFORE the write the registered project holds no
// executable rung, so the GF4 projection answers the bootstrap posture; the
// owner posts their commands over HTTP; AFTER it the SAME projection answers a
// real executing pack whose rungs are the captured commands — with no
// verify-side change of any kind between the two calls (R10).
func TestGF5CommandsWriteFlipsTheCheckPackResolution(t *testing.T) {
	e := newCommandsEnv(t)
	e.seedCommandless(t, "p-esc", "alice")

	before, err := e.proj.Get(e.ctx, "p-esc")
	if err != nil {
		t.Fatalf("Get before: %v", err)
	}
	pack, err := packFromCapture(verify.DomainSoftware, before)
	if err != nil {
		t.Fatalf("pack before the write: %v", err)
	}
	if pack == nil || pack.Posture != verify.PostureBootstrap {
		t.Fatalf("a command-less project resolves %+v, want the bootstrap posture (Spec S07.8 A14)", pack)
	}

	code, body := e.do(t, "alice", http.MethodPost, "/api/projects/p-esc/commands",
		`{"commands":{"build":"go build ./...","test":"go test ./...","lint":"gofmt -l ."}}`)
	if code != http.StatusOK {
		t.Fatalf("owner commands write = %d; body: %s", code, body)
	}

	after, err := e.proj.Get(e.ctx, "p-esc")
	if err != nil {
		t.Fatalf("Get after: %v", err)
	}
	pack, err = packFromCapture(verify.DomainSoftware, after)
	if err != nil {
		t.Fatalf("pack after the write: %v", err)
	}
	if pack == nil {
		t.Fatal("no pack after the write — the door captured nothing the ladder can run")
	}
	if pack.Posture == verify.PostureBootstrap {
		t.Fatal("the pack is STILL the bootstrap posture after the write — the advisory marking never drops and r4-F1b is not fixed")
	}
	if len(pack.Checks) != 3 {
		t.Fatalf("pack has %d rungs, want the three captured commands: %+v", len(pack.Checks), pack.Checks)
	}
	// The rungs ARE the captured commands, each run later as ONE shell line
	// inside the network-off C2 sandbox — nothing was resolved, dialed or
	// executed when they were captured.
	want := map[string]string{"build": "go build ./...", "test": "go test ./...", "lint": "gofmt -l ."}
	for _, c := range pack.Checks {
		cmd, ok := want[c.ID]
		if !ok {
			t.Fatalf("unexpected rung %q", c.ID)
		}
		if len(c.Argv) != 3 || c.Argv[0] != "/bin/sh" || c.Argv[1] != "-lc" || c.Argv[2] != cmd {
			t.Fatalf("rung %q argv = %v, want the captured command as one shell line", c.ID, c.Argv)
		}
	}
	if err := pack.Validate(); err != nil {
		t.Fatalf("the pack the door produced does not satisfy its own contract: %v", err)
	}
}

// TestEditedCaptureRefreshesTheVerifiedOnStamp [brief T6]: S07.3 rule 7 /
// S07.9 P-T06-1 stated as a test — a suite is exactly as fresh as the capture
// it came from, so a capture written at a later clock yields a pack whose
// VerifiedOn is the NEW capture's timestamp and whose AuditStale is measured
// from there. Landed semantics, unchanged by this packet; the point is that an
// owner's edit therefore un-stales a suite rather than inheriting the old
// scan's date.
func TestEditedCaptureRefreshesTheVerifiedOnStamp(t *testing.T) {
	e := newCommandsEnv(t)
	e.seedCommandless(t, "p-stamp", "alice")

	old := time.Date(2025, 8, 27, 9, 0, 0, 0, time.UTC)
	now := old.AddDate(1, 0, 0)
	thenStore := e.store(t, func() time.Time { return old })
	nowStore := e.store(t, func() time.Time { return now })

	if _, _, err := thenStore.EditCommands(e.ctx, "p-stamp", "alice", project.Commands{Test: "go test ./..."}); err != nil {
		t.Fatalf("first edit: %v", err)
	}
	stale, err := e.proj.Get(e.ctx, "p-stamp")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	stalePack, err := packFromCapture(verify.DomainSoftware, stale)
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	if !stalePack.VerifiedOn.Equal(old) {
		t.Fatalf("verified-on = %s, want the capture's own timestamp %s", stalePack.VerifiedOn, old)
	}
	wasStale, err := stalePack.AuditStale(now, e.reg)
	if err != nil {
		t.Fatalf("AuditStale: %v", err)
	}
	if !wasStale {
		t.Fatal("a year-old suite is not flagged stale — the audit interval is not measured from the capture")
	}

	if _, _, err := nowStore.EditCommands(e.ctx, "p-stamp", "alice", project.Commands{Test: "go test -race ./..."}); err != nil {
		t.Fatalf("second edit: %v", err)
	}
	fresh, err := e.proj.Get(e.ctx, "p-stamp")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if fresh.CaptureVersion != 3 {
		t.Fatalf("capture version v%d after two edits on the seeded v1, want v3", fresh.CaptureVersion)
	}
	freshPack, err := packFromCapture(verify.DomainSoftware, fresh)
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	if !freshPack.VerifiedOn.Equal(now) {
		t.Fatalf("verified-on = %s after the edit, want the new capture's timestamp %s", freshPack.VerifiedOn, now)
	}
	stillStale, err := freshPack.AuditStale(now, e.reg)
	if err != nil {
		t.Fatalf("AuditStale: %v", err)
	}
	if stillStale {
		t.Fatal("the freshly captured suite is still flagged stale — the stamp did not move with the capture")
	}
}

// TestCommandsDoorTranslatesRefusalsOnSentinels [R8]: the composition-root seam
// maps the store's refusals to statuses ON THE SENTINEL and never on message
// text (§38) — the onboardRefusal discipline one door over.
func TestCommandsDoorTranslatesRefusalsOnSentinels(t *testing.T) {
	e := newCommandsEnv(t)
	e.seedCommandless(t, "p-ref", "alice", "bob")
	const body = `{"commands":{"test":"go test ./..."}}`

	if code, out := e.do(t, "bob", http.MethodPost, "/api/projects/p-ref/commands", body); code != http.StatusForbidden {
		t.Errorf("member write = %d (want 403 on ErrNotOwner); body: %s", code, out)
	}
	if code, out := e.do(t, "alice", http.MethodPost, "/api/projects/p-gone/commands", body); code != http.StatusNotFound {
		t.Errorf("unknown project = %d (want the one 404); body: %s", code, out)
	}
	if code, out := e.do(t, "alice", http.MethodPost, "/api/projects/p-ref/commands",
		`{"commands":{"build":"make build\nrm -rf /"}}`); code != http.StatusBadRequest {
		t.Errorf("multi-line command = %d (want 400 on ErrBadInput); body: %s", code, out)
	}
}

// TestCommandsDoorNotWiredIs503 [R8]: a process composed without the seam
// answers the not-wired refusal rather than pretending — the onboardReady
// precedent — and mints nothing.
func TestCommandsDoorNotWiredIs503(t *testing.T) {
	e := newCommandsEnv(t)
	e.seedCommandless(t, "p-unwired", "alice")

	rr := httptest.NewRecorder()
	e.srv("alice", nil).Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost,
		"/api/projects/p-unwired/commands", strings.NewReader(`{"commands":{"test":"go test ./..."}}`)))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("unwired commands door = %d, want 503 not_wired; body: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "not_wired") {
		t.Fatalf("unwired refusal does not carry the not_wired code: %s", rr.Body.String())
	}
	e2, err := e.proj.Get(e.ctx, "p-unwired")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if e2.CaptureVersion != 1 {
		t.Fatalf("capture version v%d after an unwired write, want the seeded v1", e2.CaptureVersion)
	}
}
