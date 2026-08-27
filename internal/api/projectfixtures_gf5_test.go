package api_test

// projectfixtures_gf5_test.go — P3-GF5 R11 (CONVENTIONS §42/§42-B): the golden
// response bodies GF6's vitest doubles import.
//
// COMMITTED RED (CONVENTIONS §3 Amendment-A): the commands door and its Config
// seam do not exist yet, so this file fails to compile against the pre-GF5
// tree; the packet's implementation commit closes the window and the four
// bodies land with it.
//
// A NEW FILE, NOT AN EXTENSION OF apifixtures_test.go: the packet may not
// modify pre-existing test files, and the projects family needs a world of its
// own anyway — the B6-5 world registers no project at all, and its covered set
// is GET-only while the headline body here is a WRITE response.
//
// PRODUCER-MINTED, NEVER AUTHOR-IMAGINED (§42-B). Every row below goes in
// through the REAL verbs — project.Register/Capture/Activate, the real HTTP
// commands door, review.MintRevision, verify.Recorder.RecordRound — over a
// fixed clock, and each body is what the real handler served. The one thing
// mirrored rather than composed is the composition-root adapter
// (gf5CommandsSeam), exactly as projects_test.go mirrors the onboarding door:
// an api test cannot import internal/shell, and the shell's own battery
// (commandsdoor_gf5_test.go) drives the production adapter.
//
// COMPARE-ONLY BY DEFAULT. A normal run writes nothing. Regenerating is a
// deliberate act with the diff in front of you:
//
//	SINET_WRITE_API_FIXTURES=1 go test ./internal/api -run TestGF5ProjectFixtures
//
// and TestGF5FixtureWriterIsNeverAutomated extends the CI-never-writes
// assertion to these bodies.

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// The GF5 fixture world's clock. Every stamp in every committed body below is
// derived from it, so the bytes are stable enough to commit and review.
const (
	gf5FxT0 = "2026-08-27T09:00:00Z"
	gf5FxT1 = "2026-08-27T09:01:00Z"
)

// gf5CommandsSeam is the composition root's commandsDoor, mirrored — the
// onboardSeam precedent (projects_test.go). It performs NOTHING itself: the
// real store verb does the work and its sentinels are translated here exactly
// as internal/shell translates them.
type gf5CommandsSeam struct{ proj *project.Store }

var _ api.ProjectCommandsSurface = gf5CommandsSeam{}

func (s gf5CommandsSeam) SetCommands(ctx context.Context, caller, projectID string, c api.ProjectCommands) (bool, error) {
	_, minted, err := s.proj.EditCommands(ctx, projectID, caller, project.Commands{
		Build: c.Build, Test: c.Test, Lint: c.Lint, Run: c.Run, Preview: c.Preview,
	})
	if err != nil {
		return false, gf5CommandsRefusal(err)
	}
	return minted, nil
}

// gf5CommandsRefusal is the shell's sentinel translation (project_seams.go):
// the store's refusals cross the wall as TYPED transport errors, never as text
// this or any other layer matches on (§38).
func gf5CommandsRefusal(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, project.ErrNotFound):
		return &api.SurfaceError{Status: http.StatusNotFound, Code: "not_found", Msg: "project not found"}
	case errors.Is(err, project.ErrNotOwner):
		return &api.SurfaceError{Status: http.StatusForbidden, Code: "not_owner", Msg: err.Error()}
	case errors.Is(err, project.ErrNotActive):
		return &api.SurfaceError{Status: http.StatusConflict, Code: "not_active", Msg: err.Error()}
	case errors.Is(err, project.ErrBadInput):
		return &api.SurfaceError{Status: http.StatusBadRequest, Code: "bad_request", Msg: err.Error()}
	default:
		return err
	}
}

// gf5Fixtures is the covered set of this packet: one body per read GF6's
// surfaces call.
type gf5Fixtures struct {
	projects        []byte
	projectDetail   []byte
	projectCommands []byte
	deliverableBoot []byte
}

// gf5DriveFixtures seeds the world and drives every body, in a FIXED order —
// the commands write comes last, so the list and the detail above it show the
// fresh project in the state GF6's door renders for (no commands captured) and
// the write body shows the state after.
func gf5DriveFixtures(t *testing.T) gf5Fixtures {
	t.Helper()
	ctx := context.Background()
	b := newBackend(t)
	root := t.TempDir()

	for _, u := range []struct{ id, name, role string }{
		{"op", "Op", auth.RoleOperator}, {"alice", "Alice", auth.RoleMember}, {"bob", "Bob", auth.RoleMember},
	} {
		actor := "op"
		if u.id == "op" {
			actor = ""
		}
		if err := b.store.CreateUser(ctx, actor,
			auth.User{ID: u.id, DisplayName: u.name, Role: u.role}, fixturePIN); err != nil {
			t.Fatalf("create %s: %v", u.id, err)
		}
	}

	proj, err := project.New(project.Config{
		DB: b.db, Log: b.log, Root: filepath.Join(root, "projects"),
		Now: func() time.Time { return mustTime(t, gf5FxT0) },
	})
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	// p-shop is the SETTLED shape: an active project whose capture already
	// holds build/test/lint, which is what the editor reads back.
	gf5SeedProject(t, proj, root, "p-shop", "Shop backend", "alice", []string{"bob"}, project.CaptureInput{
		Conventions: []string{"tabs, not spaces", "every change ships with a test"},
		Commands: project.Commands{
			Build: "go build ./...", Test: "go test ./...", Lint: "gofmt -l .",
			Run: "go run ./cmd/shop", Preview: "go run ./cmd/shop --port 8080",
		},
		DangerZones: []project.DangerZone{{Path: "deploy/", Action: "never", Rule: "deployment manifests are changed by hand", SourceHash: "6f1c2b"}},
		ScanHash:    "scan-shop-v1",
		Family:      project.FamilySoftware,
	})
	// p-fresh is the r4-F1 shape the door exists for: a scaffold with NOTHING
	// captured, whose tasks verify under the bootstrap posture until an owner
	// types their commands.
	gf5SeedProject(t, proj, root, "p-fresh", "Car webshop", "alice", nil, project.CaptureInput{
		ScanHash: "scan-fresh-v1",
		Family:   project.FamilySoftware,
	})

	// The bootstrap deliverable: one revision whose verdict row carries the
	// S07.8 posture, pinned to the revision the way the stage's review sink
	// pins it.
	rev := &review.Store{
		DB: b.db, Log: b.log, Settings: b.reg, Root: filepath.Join(root, "review"),
		Now: func() time.Time { return mustTime(t, gf5FxT1) },
	}
	gf5Exec(t, b, `INSERT INTO tasks (task_id, user_id, title, kanban_status, created_ts) VALUES (?, ?, ?, 'executing', ?)`,
		"t-fresh", "alice", "Scaffold the car webshop", gf5FxT0)
	gf5Exec(t, b, `INSERT INTO runs (run_id, user_id, task_id, state, lane, generation, created_ts, updated_ts)
	                VALUES (?, ?, ?, 'completed', 'anthropic', 0, ?, ?)`,
		"t-fresh.verify", "alice", "t-fresh", gf5FxT0, gf5FxT1)
	gf5Exec(t, b, `INSERT INTO task_project (task_id, project_id, project_choices) VALUES (?, ?, 1)`,
		"t-fresh", "p-fresh")
	if _, err := rev.EnsureDeliverable(ctx, review.EnsureInput{
		ID: "dlv-t-fresh", Owner: "alice", TaskID: "t-fresh", ProjectID: "p-fresh", Type: "code",
	}); err != nil {
		t.Fatalf("EnsureDeliverable: %v", err)
	}
	if _, err := rev.MintRevision(ctx, review.MintInput{
		DeliverableID: "dlv-t-fresh", N: 1, RunID: "t-fresh.verify", AttemptRef: "t-fresh.verify#round-1",
		Files: map[string]string{"main.go": "package main\n\nfunc main() {}\n"},
	}); err != nil {
		t.Fatalf("MintRevision: %v", err)
	}
	seq, err := (&verify.Recorder{DB: b.db, Log: b.log}).RecordRound(ctx, "t-fresh.verify",
		verify.Deliverable{TaskID: "t-fresh", RunID: "t-fresh.verify", Domain: verify.DomainSoftware, Revision: 1},
		verify.RoundRecord{
			Round: 1, Verdict: verify.VerdictShip, Revision: 1,
			Posture: verify.PostureBootstrap, PostureNote: verify.BootstrapPostureNote,
			ReviewMandatory: true, ContentSHA: "sha-fresh-1",
		},
		verify.JudgeMeta{Model: "gf5-judge-1", SelfFamily: true}, verify.GoldenSetRates{},
		"rubric-software", 1, []string{"AC-1"}, "idle")
	if err != nil {
		t.Fatalf("RecordRound: %v", err)
	}
	if err := rev.SetVerdictRef(ctx, "dlv-t-fresh", 1, seq); err != nil {
		t.Fatalf("SetVerdictRef: %v", err)
	}

	srv := func(who string) *api.Server {
		return api.New(api.Config{
			Log: b.log, Sessions: b.store, Auth: fixedIdentity{who},
			Settings: fixedSettings{d: 20 * 1e9},
			HealthFn: func() api.Health { return api.Health{Ready: true} },
			DB:       b.db, Review: rev,
			ProjectCommands: gf5CommandsSeam{proj: proj},
			Now:             func() time.Time { return mustTime(t, gf5FxT1) },
		})
	}
	body := func(who, method, path, in string) []byte {
		t.Helper()
		rr := httptest.NewRecorder()
		srv(who).Handler().ServeHTTP(rr, httptest.NewRequest(method, path, strings.NewReader(in)))
		if rr.Code != http.StatusOK {
			t.Fatalf("%s %s as %s: status %d: %s", method, path, who, rr.Code, rr.Body.String())
		}
		return canonicalJSON(t, rr.Body.Bytes())
	}

	return gf5Fixtures{
		projects:        body("alice", http.MethodGet, "/api/projects", ""),
		projectDetail:   body("alice", http.MethodGet, "/api/projects/p-shop", ""),
		deliverableBoot: body("alice", http.MethodGet, "/api/deliverables/dlv-t-fresh", ""),
		projectCommands: body("alice", http.MethodPost, "/api/projects/p-fresh/commands",
			`{"commands":{"build":"npm run build","test":"npm test","lint":"npm run lint"}}`),
	}
}

// gf5SeedProject registers, captures and activates one entry through the real
// verbs.
func gf5SeedProject(t *testing.T, proj *project.Store, root, id, name, owner string, members []string, in project.CaptureInput) {
	t.Helper()
	ctx := context.Background()
	if _, err := proj.Register(ctx, project.RegisterInput{
		ProjectID: id, Owner: owner, Name: name, StorePath: filepath.Join(root, "stores", id), Members: members,
	}); err != nil {
		t.Fatalf("Register %s: %v", id, err)
	}
	in.ProjectID, in.By = id, owner
	if _, err := proj.Capture(ctx, in); err != nil {
		t.Fatalf("Capture %s: %v", id, err)
	}
	if _, err := proj.Activate(ctx, id, owner); err != nil {
		t.Fatalf("Activate %s: %v", id, err)
	}
}

func gf5Exec(t *testing.T, b *backend, q string, args ...any) {
	t.Helper()
	if err := b.db.WriteTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(), q, args...)
		return err
	}); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

// gf5FixtureFiles pairs each committed file with the body it holds.
func gf5FixtureFiles(fx gf5Fixtures) []struct {
	name string
	body []byte
} {
	return []struct {
		name string
		body []byte
	}{
		{"projects", fx.projects},
		{"project-detail", fx.projectDetail},
		{"project-commands", fx.projectCommands},
		{"deliverable-detail-bootstrap", fx.deliverableBoot},
	}
}

// TestGF5ProjectFixtures [brief T10] compares each committed body against what
// the real handlers serve, and regenerates them under the env gate.
func TestGF5ProjectFixtures(t *testing.T) {
	for _, fx := range gf5FixtureFiles(gf5DriveFixtures(t)) {
		t.Run(fx.name, func(t *testing.T) { compareOrWriteFixture(t, fx.name, fx.body) })
	}
}

// TestGF5ProjectFixturesAreStable is the property the whole mechanism rests on:
// the same seeded world, driven in the same order, serves the same bytes twice.
// A body carrying a wall-clock reading passes the comparison on the run that
// wrote it and fails on every run after, so the instability is caught HERE.
func TestGF5ProjectFixturesAreStable(t *testing.T) {
	first := gf5FixtureFiles(gf5DriveFixtures(t))
	second := gf5FixtureFiles(gf5DriveFixtures(t))
	for i := range first {
		if string(first[i].body) != string(second[i].body) {
			t.Errorf("%s is not byte-stable across two identical seedings — it carries a live reading, "+
				"so it cannot be a committed fixture:\n%s\n%s", first[i].name, first[i].body, second[i].body)
		}
	}
}

// TestGF5FixtureWriterIsNeverAutomated extends the CI-never-writes assertion to
// these bodies: regeneration is an operator act with the diff in front of them.
func TestGF5FixtureWriterIsNeverAutomated(t *testing.T) {
	if os.Getenv(fixtureWriteEnv) != "" {
		t.Skipf("SANCTIONED SKIP (CONVENTIONS §10, tier-R): %s is set, which is the deliberate regeneration act", fixtureWriteEnv)
	}
	for _, path := range []string{"../../.github/workflows/ci.yml", "../../Makefile"} {
		src, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if strings.Contains(string(src), fixtureWriteEnv) {
			t.Errorf("%s names %s — regeneration is an operator act, never something CI does", path, fixtureWriteEnv)
		}
	}
	// And the committed bodies must actually exist: a fixture nobody committed
	// would make the comparison above vacuous on a fresh checkout.
	for _, name := range []string{"projects", "project-detail", "project-commands", "deliverable-detail-bootstrap"} {
		if _, err := os.Stat(filepath.Join(fixtureDir, name+".json")); err != nil {
			t.Errorf("committed fixture %s.json is missing: %v", name, err)
		}
	}
}

// TestGF5CommandsFixtureCarriesTheCapturedCommands is the non-tautological
// control on the write body: the committed bytes have to describe the state the
// store actually holds, not a shape somebody typed.
func TestGF5CommandsFixtureCarriesTheCapturedCommands(t *testing.T) {
	fx := gf5DriveFixtures(t)
	for _, want := range []string{"npm run build", "npm test", "npm run lint"} {
		if !strings.Contains(string(fx.projectCommands), want) {
			t.Errorf("the write body does not carry the captured command %q:\n%s", want, fx.projectCommands)
		}
	}
	if !strings.Contains(string(fx.deliverableBoot), `"posture"`) {
		t.Errorf("the bootstrap deliverable body carries no posture member:\n%s", fx.deliverableBoot)
	}
}
