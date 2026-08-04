package preview

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/portpool"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/sandbox"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// The preview acceptance harness: a real platform.db, the real event log, the
// real project store (git topology), the real portpool allocator, hermetic git
// fixtures, and a FAKE review store returning a repo-backed deliverable pinned
// at the fixture's HEAD. No network, no live front chain (host hazard).

type mgrFix struct {
	t          *testing.T
	reg        *settings.Registry
	db         *storage.DB
	log        *eventlog.Log
	proj       *project.Store
	ports      *portpool.Allocator
	reviews    *fakeReviews
	projectID  string
	committish string
}

func newMgrFix(t *testing.T, files map[string]string, dtype string, previewOverride ...string) *mgrFix {
	t.Helper()
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	log := eventlog.New(db, reg)
	proj, err := project.New(project.Config{DB: db, Log: log, Root: filepath.Join(t.TempDir(), "projects")})
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	projectID := "proj-1"
	src := fixtureRepo(t, files)
	_, draft, err := proj.Onboard(ctx, project.OnboardInput{ProjectID: projectID, Owner: "u", Name: "demo", Source: src})
	if err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	// Clear the scan-drafted preview command by default so the heuristic lanes
	// are exercised; an explicit override keeps a captured command for the R3
	// registry-precedence path (TestRegistryCommandThroughManager).
	draft.Commands.Preview = ""
	if len(previewOverride) > 0 {
		draft.Commands.Preview = previewOverride[0]
	}
	e, err := proj.Approve(ctx, projectID, "u", &draft)
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	committish := gitOut(t, e.StorePath, "rev-parse", "HEAD")

	// A DISJOINT range from the shell composition test's (R2-1a): both packages
	// run in parallel and the OS loopback port is the shared resource neither
	// file-stub sees; the preview package owns 47700-47719, the shell 47800-47819,
	// and neither touches the structural default 47600-47619.
	ports, err := portpool.New(portpool.Config{Dir: filepath.Join(t.TempDir(), "portpool"), Lo: 47700, Hi: 47719})
	if err != nil {
		t.Fatalf("portpool.New: %v", err)
	}
	reviews := &fakeReviews{
		deliverables: map[string]review.Deliverable{
			"dlv1": {ID: "dlv1", Owner: "u", ProjectID: projectID, Type: dtype, CurrentRevision: 1, State: review.StateInReview},
		},
		revisions: map[string]review.Revision{
			"dlv1/1": {DeliverableID: "dlv1", N: 1, SnapshotSHA: committish, PlatformRef: review.RevisionRef("dlv1", 1)},
		},
		accepted: map[string]review.Revision{},
	}
	return &mgrFix{t: t, reg: reg, db: db, log: log, proj: proj, ports: ports, reviews: reviews, projectID: projectID, committish: committish}
}

// --- fakes ---

type fakeReviews struct {
	deliverables map[string]review.Deliverable
	revisions    map[string]review.Revision
	accepted     map[string]review.Revision
	// acceptedErr, when set, is returned by AcceptedRevision for an id absent
	// from `accepted` INSTEAD of the ErrBadInput-wrapped default — to exercise
	// LaunchComparison's non-ErrBadInput propagate branch (F7).
	acceptedErr error
}

func (f *fakeReviews) Deliverable(_ context.Context, id string) (review.Deliverable, error) {
	d, ok := f.deliverables[id]
	if !ok {
		return review.Deliverable{}, fmt.Errorf("no deliverable %q", id)
	}
	return d, nil
}

func (f *fakeReviews) RevisionAt(_ context.Context, id string, n int) (review.Revision, error) {
	r, ok := f.revisions[fmt.Sprintf("%s/%d", id, n)]
	if !ok {
		return review.Revision{}, fmt.Errorf("no revision %s/%d", id, n)
	}
	return r, nil
}

func (f *fakeReviews) AcceptedRevision(_ context.Context, id string) (review.Revision, error) {
	if r, ok := f.accepted[id]; ok {
		return r, nil
	}
	if f.acceptedErr != nil {
		return review.Revision{}, f.acceptedErr // a REAL error → must propagate (F7)
	}
	// Wrap review.ErrBadInput exactly as review.Store.AcceptedRevision does for a
	// not-accepted deliverable, so LaunchComparison's F7 branch is exercised
	// (single-instance) vs a real error (propagate).
	return review.Revision{}, fmt.Errorf("%w: deliverable %q is not accepted", review.ErrBadInput, id)
}

type fakeSettings struct {
	idle time.Duration
	cap  int64
}

func (f fakeSettings) Int(key string) (int64, error) {
	if key == keyMaxConcurrent {
		return f.cap, nil
	}
	return 0, fmt.Errorf("unexpected key %q", key)
}

func (f fakeSettings) Duration(key string) (time.Duration, error) {
	if key == keyIdleStop {
		return f.idle, nil
	}
	return 0, fmt.Errorf("unexpected key %q", key)
}

// recordingEvents wraps the real log and records the Append structs so the R22
// Low-tier attributes (platform-scope, owner-attributed) are assertable.
type recordingEvents struct {
	log     *eventlog.Log
	mu      sync.Mutex
	appends []eventlog.Append
}

func (r *recordingEvents) Append(ctx context.Context, a eventlog.Append) (int64, error) {
	r.mu.Lock()
	r.appends = append(r.appends, a)
	r.mu.Unlock()
	return r.log.Append(ctx, a)
}

func (r *recordingEvents) byType(typ string) []eventlog.Append {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []eventlog.Append
	for _, a := range r.appends {
		if a.Type == typ {
			out = append(out, a)
		}
	}
	return out
}

// unavailableSandbox reports a host that cannot compose the boundary (empty
// caps ⇒ Available()==false) — the honest unavailable-state path (R7).
type unavailableSandbox struct{}

func (unavailableSandbox) Caps() sandbox.Capabilities { return sandbox.Capabilities{} }

func (unavailableSandbox) EgressPolicyFor(sandbox.Class, []string) (sandbox.EgressPolicy, error) {
	return sandbox.EgressPolicy{}, nil
}

// --- git fixtures (hermetic; the project-harness posture) ---

func gitEnv() []string {
	env := []string{
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0", "GIT_ALLOW_PROTOCOL=file",
		"HOME=/nonexistent",
		"GIT_AUTHOR_NAME=fixture", "GIT_AUTHOR_EMAIL=fixture@test.invalid",
		"GIT_COMMITTER_NAME=fixture", "GIT_COMMITTER_EMAIL=fixture@test.invalid",
	}
	if p := os.Getenv("PATH"); p != "" {
		env = append(env, "PATH="+p)
	}
	return env
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func fixtureRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "fixture-repo")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	gitOut(t, dir, "init", "-q", "--initial-branch=main", dir)
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	gitOut(t, dir, "add", "-A")
	gitOut(t, dir, "commit", "-q", "-m", "fixture")
	return dir
}

// hashTree fingerprints a directory tree (sorted relpath+content, .git skipped)
// — the zero-mutation before/after byte-identity check (R6).
func hashTree(t *testing.T, dir string) string {
	t.Helper()
	h := sha256.New()
	var rels []string
	contents := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		if d.Name() == ".git" {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil // git-worktree .git FILE — skip just it, not siblings
		}
		if d.Type()&fs.ModeSymlink != 0 { // F3: symlink mutations must be visible
			target, lerr := os.Readlink(path)
			if lerr != nil {
				return lerr
			}
			rels = append(rels, rel)
			contents[rel] = "symlink\x00" + target
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rels = append(rels, rel)
		contents[rel] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(rels)
	for _, r := range rels {
		fmt.Fprintf(h, "%s\x00%s\x00", r, contents[r])
	}
	return hex.EncodeToString(h.Sum(nil))
}
