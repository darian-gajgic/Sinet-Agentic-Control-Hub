package shell

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/preview"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/sandbox"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// TestBuildPreviewSurfaceComposition (R19): the S13.8 preview module is driven
// through the SHELL-CONSTRUCTED object — buildPreviewSurface with the production
// stores — proving it is compiled in and reachable through the real wiring. It
// launches a static preview from a repo-backed revision and tears it down. No
// live front chain is touched (routing disabled: no admin endpoint configured).
func TestBuildPreviewSurfaceComposition(t *testing.T) {
	// F8: pin the routing config to empty so a dev-shell export can never make
	// this test dial the LIVE Caddy admin API (host hazard). The test asserts
	// routing is disabled below. R2-1a: pin a DISJOINT port range (47800-47819)
	// from the preview package's (47700-47719) so the two test packages, running
	// in parallel, never contend for the same OS loopback port.
	t.Setenv("SINET_PREVIEW_CADDY_ADMIN", "")
	t.Setenv("SINET_PREVIEW_BASE_HOST", "test.invalid")
	t.Setenv("SINET_PREVIEW_PORT_LO", "47800")
	t.Setenv("SINET_PREVIEW_PORT_HI", "47819")
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	log := eventlog.New(db, reg)
	proj, err := project.New(project.Config{DB: db, Log: log, Root: filepath.Join(t.TempDir(), "projects")})
	if err != nil {
		t.Fatal(err)
	}
	reviewStore := &review.Store{DB: db, Log: log, Settings: reg, Root: filepath.Join(t.TempDir(), "review")}

	// An onboarded project + an in-review deliverable whose revision snapshot is
	// a static site (index.html).
	remote := seedCompRemote(t)
	if _, _, err := proj.Onboard(ctx, project.OnboardInput{ProjectID: "proj", Owner: "u1", Name: "P", Source: remote, RemoteURL: "file://" + remote}); err != nil {
		t.Fatal(err)
	}
	if _, err := proj.Approve(ctx, "proj", "u1", nil); err != nil {
		t.Fatal(err)
	}
	ws, err := proj.EnsureWorkspace(ctx, "proj", "pipe1")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(ws.Path, "index.html"), []byte("<h1>shell-composed</h1>"), 0o600)
	candidate, err := proj.Snapshot(ctx, ws.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := proj.CreateRevisionRef(ctx, "proj", review.RevisionRef("dlv", 1), candidate); err != nil {
		t.Fatal(err)
	}
	// Seed the user/task/run the deliverable + revision reference.
	compSeed(t, ctx, db, log, run.NewStore(db, log))
	if _, err := reviewStore.EnsureDeliverable(ctx, review.EnsureInput{ID: "dlv", Owner: "u1", TaskID: "t1", ProjectID: "proj", Type: "code"}); err != nil {
		t.Fatal(err)
	}
	if _, err := reviewStore.MintRevision(ctx, review.MintInput{
		DeliverableID: "dlv", N: 1, RunID: "r1", AttemptRef: "r1#round-1",
		Files: map[string]string{"index.html": "<h1>shell-composed</h1>"}, SnapshotSHA: candidate,
	}); err != nil {
		t.Fatal(err)
	}

	// THE SHELL WIRING: buildPreviewSurface with the production stores.
	m, err := buildPreviewSurface(previewDeps{
		Reviews: reviewStore, Projects: proj, Sandbox: sandbox.NewComposer(reg, nil),
		Events: log, Settings: reg, StateDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("buildPreviewSurface: %v", err)
	}
	s, err := m.Launch(ctx, preview.LaunchRequest{DeliverableID: "dlv", User: "u1"})
	if err != nil {
		t.Fatalf("Launch through the shell-constructed object: %v", err)
	}
	if s.Lane != preview.LaneStatic || s.State != preview.StateLive {
		t.Fatalf("preview lane/state = %s/%s, want static/live", s.Lane, s.State)
	}
	if s.Routed {
		t.Errorf("routing should be disabled (no admin endpoint configured)")
	}
	if err := m.Stop(ctx, s.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

// TestPreviewInCmdDeps (R19): internal/preview is compiled into the release
// binary — go list -deps ./cmd/sinet includes it, proving the surface is
// reachable and not dead code.
func TestPreviewInCmdDeps(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "list", "-deps", "./cmd/sinet")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "Sinet-Agentic-Control-Hub/internal/preview") {
		t.Fatalf("cmd/sinet deps do not include internal/preview (surface not compiled in)")
	}
}

// TestEnvPortRange pins F18: the structural port-range override is passed
// through from env; malformed/absent env falls back to the portpool default.
func TestEnvPortRange(t *testing.T) {
	t.Setenv("SINET_PREVIEW_PORT_LO", "40000")
	t.Setenv("SINET_PREVIEW_PORT_HI", "40009")
	if lo, hi := envPortRange(); lo != 40000 || hi != 40009 {
		t.Errorf("override = [%d,%d], want [40000,40009]", lo, hi)
	}
	t.Setenv("SINET_PREVIEW_PORT_LO", "notanint")
	if lo, hi := envPortRange(); lo != 0 || hi != 0 {
		t.Errorf("malformed lo → [%d,%d], want [0,0] (default)", lo, hi)
	}
	t.Setenv("SINET_PREVIEW_PORT_LO", "40009")
	t.Setenv("SINET_PREVIEW_PORT_HI", "40000") // hi < lo
	if lo, hi := envPortRange(); lo != 0 || hi != 0 {
		t.Errorf("hi<lo → [%d,%d], want [0,0] (default)", lo, hi)
	}
}
