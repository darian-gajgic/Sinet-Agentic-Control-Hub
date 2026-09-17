package stage_test

// tq8_changesource_test.go — P3-TQ-8 (4): the judge's diff comes from the
// platform-owned project store at the pinned refs, through the stage's
// review sink as verify.ChangeSource over review.Store's tree lane (§78's
// read-only verbs) — never the sandbox or the stripped verification
// workspace (Spec S07.3 rule 1). Committed RED by grounding (Amendment-A
// carve-out, CONVENTIONS §3).
//
// The world is REAL: a project onboarded from a git fixture, its task
// worktree, platform snapshot commits taken by project.Store.Snapshot,
// revisions minted on those pins with their platform refs, and the review
// store's TreeSource wired to the project store through a test adapter that
// does what the shell's projectSeams does (the SIT-1 api pattern). Nothing
// fakes a tree: every inventory row, diff and content the seam returns is
// compared against what the test itself WROTE.

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

const (
	tq8Readme  = "# shop\n"
	tq8AppBase = "package app\n\nfunc Run() {}\n"
	tq8AppRev1 = "package app\n\nfunc Run() {\n\tserve()\n}\n"
	tq8AppRev2 = "package app\n\nfunc Run() {\n\tserve()\n\tlisten()\n}\n"
	tq8Cart    = "package app\n\nfunc Cart() {}\n"
	tq8PNG     = "\x89PNG\r\n\x1a\n\x00\x00IHDR"
)

// tq8NoEngine is an adapter that starts nothing ($0): the skeleton needs one
// registered, and no session is ever driven here.
type tq8NoEngine struct{}

func (tq8NoEngine) Substrate() string { return adapters.SubstrateClaudeCLI }
func (tq8NoEngine) Start(context.Context, adapters.StartRequest) (adapters.Session, error) {
	return nil, errors.New("tq8: no engine")
}
func (tq8NoEngine) Resume(context.Context, adapters.ParkRecord, *adapters.Answer) (adapters.Session, error) {
	return nil, errors.New("tq8: no engine")
}

// tq8Judge is injected so newVerifier's P-T06-5 gate does not apply (the
// composition root's dev/test seam); it is never called.
type tq8Judge struct{}

func (tq8Judge) Compliance(context.Context, verify.JudgeInput) (verify.Axis1Result, error) {
	return verify.Axis1Result{}, errors.New("tq8: judge never called")
}
func (tq8Judge) Sanity(context.Context, verify.JudgeInput) (verify.Axis2Result, error) {
	return verify.Axis2Result{}, errors.New("tq8: judge never called")
}
func (tq8Judge) Meta() verify.JudgeMeta { return verify.JudgeMeta{Model: "tq8-judge"} }

// tq8Seam adapts the project store to review.TreeSource for one project,
// exactly as the shell's projectSeams does.
type tq8Seam struct {
	proj      *project.Store
	projectID string
}

func (s tq8Seam) TreeBase(ctx context.Context, deliverableID string) (string, bool, error) {
	sha, err := s.proj.BaseSHA(ctx, s.projectID, strings.TrimPrefix(deliverableID, "dlv-"))
	if err != nil {
		return "", false, err
	}
	return sha, sha != "", nil
}

func (s tq8Seam) TreeChanges(ctx context.Context, _, oldSHA, newSHA string) ([]review.ChangedFile, error) {
	rows, err := s.proj.TreeChanges(ctx, s.projectID, oldSHA, newSHA)
	if err != nil {
		return nil, err
	}
	out := make([]review.ChangedFile, 0, len(rows))
	for _, r := range rows {
		out = append(out, review.ChangedFile{Path: r.Path, OldPath: r.OldPath, Kind: r.Kind,
			OldSize: r.OldSize, NewSize: r.NewSize, Binary: r.Binary, Additions: r.Additions, Deletions: r.Deletions})
	}
	return out, nil
}

func (s tq8Seam) TreeBlob(ctx context.Context, _, sha, path string, limit int64) ([]byte, int64, bool, error) {
	return s.proj.TreeBlob(ctx, s.projectID, sha, path, limit)
}

type tq8World struct {
	t    *testing.T
	ctx  context.Context
	sk   *stage.Skeleton
	rev  *review.Store
	proj *project.Store
	ws   string
	base string
	pins []string
}

func newTQ8World(t *testing.T) *tq8World {
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
	runs := run.NewStore(db, log)
	root := t.TempDir()
	rev := &review.Store{DB: db, Log: log, Settings: reg, Root: filepath.Join(root, "review")}
	proj, err := project.New(project.Config{DB: db, Log: log, Root: filepath.Join(root, "projects")})
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	sk, err := stage.New(stage.Config{
		DB: db, Log: log, Runs: runs, Checkpoints: gates.NewCheckpoints(db, log),
		Ledger: ledger.NewStore(db, log), Settings: reg,
		Substrate: adapters.SubstrateClaudeCLI, Lane: adapters.LaneAnthropic,
		Adapters:     map[string]adapters.Adapter{adapters.SubstrateClaudeCLI: tq8NoEngine{}},
		DutyMap:      worker.DefaultDutyMap(),
		ArtifactRoot: filepath.Join(root, "artifacts"),
		RunRoot:      filepath.Join(root, "runs"),
		Review:       rev,
		Judge:        tq8Judge{},
	})
	if err != nil {
		t.Fatalf("stage.New: %v", err)
	}
	const owner = "alice"
	if err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		for _, id := range []string{"t-shop", "t-doc"} {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO tasks (task_id, user_id, title, created_ts) VALUES (?, ?, ?, ?)`,
				id, owner, "tq8 "+id, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("create tasks: %v", err)
	}
	for _, id := range []string{"t-shop.execute", "t-shop.verify", "t-doc.verify"} {
		task := strings.SplitN(id, ".", 2)[0]
		if _, err := runs.Create(ctx, run.NewRun{ID: id, UserID: owner, TaskID: task,
			Substrate: adapters.SubstrateClaudeCLI, Lane: adapters.LaneAnthropic}); err != nil {
			t.Fatalf("create run %s: %v", id, err)
		}
	}
	src := gitFixture(t, map[string]string{"README.md": tq8Readme, "src/app.go": tq8AppBase})
	if _, err := proj.OnboardStart(ctx, project.OnboardInput{ProjectID: "shop", Owner: owner, Name: "shop", Source: src}); err != nil {
		t.Fatalf("OnboardStart: %v", err)
	}
	if err := proj.OnboardApprove(ctx, "shop", owner, nil); err != nil {
		t.Fatalf("OnboardApprove: %v", err)
	}
	ws, err := proj.EnsureWorkspace(ctx, "shop", "t-shop")
	if err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	base, err := proj.BaseSHA(ctx, "shop", "t-shop")
	if err != nil || base == "" {
		t.Fatalf("BaseSHA: %q %v", base, err)
	}
	if _, err := rev.EnsureDeliverable(ctx, review.EnsureInput{
		ID: "dlv-t-shop", Owner: owner, TaskID: "t-shop", ProjectID: "shop", Type: "code",
	}); err != nil {
		t.Fatalf("EnsureDeliverable: %v", err)
	}
	w := &tq8World{t: t, ctx: ctx, sk: sk, rev: rev, proj: proj, ws: ws.Path, base: base}
	// Revision 1: cart.go added, app.go modified. Revision 2: app.go modified
	// again, README.md deleted, a binary logo added.
	w.mintTree(map[string]string{"README.md": tq8Readme, "src/app.go": tq8AppRev1, "src/cart.go": tq8Cart})
	w.mintTree(map[string]string{"src/app.go": tq8AppRev2, "src/cart.go": tq8Cart, "assets/logo.png": tq8PNG})
	rev.Tree = tq8Seam{proj: proj, projectID: "shop"}
	return w
}

// mintTree makes the worktree hold EXACTLY files, takes the platform
// snapshot, mints the next revision on it and creates its platform ref —
// the execute stage-close + verify-handoff sequence, minus the engine.
func (w *tq8World) mintTree(files map[string]string) string {
	w.t.Helper()
	entries, err := os.ReadDir(w.ws)
	if err != nil {
		w.t.Fatal(err)
	}
	for _, ent := range entries {
		if ent.Name() == ".git" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(w.ws, ent.Name())); err != nil {
			w.t.Fatal(err)
		}
	}
	for rel, body := range files {
		p := filepath.Join(w.ws, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			w.t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			w.t.Fatal(err)
		}
	}
	sha, err := w.proj.Snapshot(w.ctx, w.ws)
	if err != nil {
		w.t.Fatalf("Snapshot: %v", err)
	}
	n := len(w.pins) + 1
	if _, err := w.rev.MintRevision(w.ctx, review.MintInput{
		DeliverableID: "dlv-t-shop", N: n, RunID: "t-shop.verify", AttemptRef: verify.MintRef("t-shop.verify", n),
		ProducedBy:  "t-shop.execute",
		Files:       map[string]string{stage.DeliverableFileName: "# report rev " + string(rune('0'+n)) + "\n"},
		SnapshotSHA: sha,
	}); err != nil {
		w.t.Fatalf("MintRevision %d: %v", n, err)
	}
	if err := w.proj.CreateRevisionRef(w.ctx, "shop", review.RevisionRef("dlv-t-shop", n), sha); err != nil {
		w.t.Fatalf("CreateRevisionRef %d: %v", n, err)
	}
	w.pins = append(w.pins, sha)
	return sha
}

func (w *tq8World) deliverable(n int) verify.Deliverable {
	return verify.Deliverable{
		TaskID: "t-shop", RunID: "t-shop.verify", Domain: verify.DomainSoftware, Type: "markdown",
		Revision: n, Content: "# report\n", SnapshotSHA: w.pins[n-1], BaseSHA: w.base, WriteClaimed: true,
	}
}

func tq8Rows(files []verify.ChangedFile) map[string]verify.ChangedFile {
	out := map[string]verify.ChangedFile{}
	for _, f := range files {
		out[f.Path] = f
	}
	return out
}

// TestTQ8StageSinkServesTheChangeFromThePlatformStore — the stage's review
// sink IS the verify.ChangeSource, and what it returns is the real tree
// change at the pinned refs: revision 1 against the recorded pre-task base,
// revision N against N−1, an added file's diff being its content, a
// modified file carrying its full new-side content, a binary row with no
// bodies, a content-pinned revision an honest absence, and a lost pin
// content drift — never the report.
func TestTQ8StageSinkServesTheChangeFromThePlatformStore(t *testing.T) {
	w := newTQ8World(t)
	ctx := w.ctx
	cs, ok := stage.ChangeSourceOf(w.sk)
	if !ok {
		t.Fatal("the stage review sink does not implement verify.ChangeSource (P3-TQ-8: the judge's tree slice must be read through the S13 adapter)")
	}

	// Revision 1 against the pre-task base.
	rc, err := cs.RevisionChange(ctx, w.deliverable(1), verify.JudgeArtifactBytesCap)
	if err != nil {
		t.Fatalf("RevisionChange rev 1: %v", err)
	}
	if rc.AbsentReason != "" {
		t.Fatalf("rev 1 is repo-backed; absent reason %q", rc.AbsentReason)
	}
	if !rc.OldIsBase || rc.OldN != 0 || rc.NewN != 1 || rc.OldPin != w.base || rc.NewPin != w.pins[0] {
		t.Fatalf("rev 1 pins: %+v (base %s, pin %s)", rc, w.base, w.pins[0])
	}
	rows := tq8Rows(rc.Files)
	if len(rows) != 2 || rows["src/app.go"].Kind != verify.KindModified || rows["src/cart.go"].Kind != verify.KindAdded {
		t.Fatalf("rev 1 inventory: %+v", rc.Files)
	}
	if !strings.Contains(rows["src/cart.go"].Diff, "+func Cart() {}") || rows["src/cart.go"].Content != "" {
		t.Fatalf("added file: diff must be its content and Content empty: %+v", rows["src/cart.go"])
	}
	if !strings.Contains(rows["src/app.go"].Diff, "+\tserve()") || rows["src/app.go"].Content != tq8AppRev1 {
		t.Fatalf("modified file: hunk + full new-side content: %+v", rows["src/app.go"])
	}
	for _, r := range rc.Files {
		if r.BodySkipped || r.DiffTruncated || r.ContentTruncated {
			t.Fatalf("small change reported as bounded: %+v", r)
		}
	}

	// Revision 2 against revision 1.
	rc, err = cs.RevisionChange(ctx, w.deliverable(2), verify.JudgeArtifactBytesCap)
	if err != nil {
		t.Fatalf("RevisionChange rev 2: %v", err)
	}
	if rc.OldIsBase || rc.OldN != 1 || rc.NewN != 2 || rc.OldPin != w.pins[0] || rc.NewPin != w.pins[1] {
		t.Fatalf("rev 2 pins: %+v", rc)
	}
	rows = tq8Rows(rc.Files)
	if len(rows) != 3 || rows["README.md"].Kind != verify.KindDeleted || rows["src/app.go"].Kind != verify.KindModified || rows["assets/logo.png"].Kind != verify.KindAdded {
		t.Fatalf("rev 2 inventory: %+v", rc.Files)
	}
	if !strings.Contains(rows["README.md"].Diff, "-# shop") {
		t.Fatalf("deleted file's diff: %+v", rows["README.md"])
	}
	if !strings.Contains(rows["src/app.go"].Diff, "+\tlisten()") || rows["src/app.go"].Content != tq8AppRev2 {
		t.Fatalf("rev 2 app.go: %+v", rows["src/app.go"])
	}
	logo := rows["assets/logo.png"]
	if !logo.Binary || logo.Diff != "" || logo.Content != "" {
		t.Fatalf("binary row must carry no bodies: %+v", logo)
	}
	// Path order, as the inventory is served.
	for i := 1; i < len(rc.Files); i++ {
		if rc.Files[i-1].Path >= rc.Files[i].Path {
			t.Fatalf("inventory not in path order: %+v", rc.Files)
		}
	}

	t.Run("a content-pinned revision is an honest absence", func(t *testing.T) {
		if _, err := w.rev.EnsureDeliverable(ctx, review.EnsureInput{ID: "dlv-t-doc", Owner: "alice", TaskID: "t-doc", Type: "markdown"}); err != nil {
			t.Fatal(err)
		}
		if _, err := w.rev.MintRevision(ctx, review.MintInput{DeliverableID: "dlv-t-doc", N: 1, RunID: "t-doc.verify",
			AttemptRef: verify.MintRef("t-doc.verify", 1), Files: map[string]string{stage.DeliverableFileName: "# doc\n"}}); err != nil {
			t.Fatal(err)
		}
		rc, err := cs.RevisionChange(ctx, verify.Deliverable{TaskID: "t-doc", RunID: "t-doc.verify", Domain: verify.DomainSoftware,
			Revision: 1, Content: "# doc\n"}, verify.JudgeArtifactBytesCap)
		if err != nil {
			t.Fatalf("RevisionChange: %v", err)
		}
		if rc.AbsentReason == "" || len(rc.Files) != 0 {
			t.Fatalf("content-pinned revision must answer an absence, got %+v", rc)
		}
	})

	t.Run("a lost pin is content drift, never a fall-through to the report", func(t *testing.T) {
		const gone = "0123456789abcdef0123456789abcdef01234567"
		if _, err := w.rev.MintRevision(ctx, review.MintInput{DeliverableID: "dlv-t-shop", N: 3, RunID: "t-shop.verify",
			AttemptRef: verify.MintRef("t-shop.verify", 3), ProducedBy: "t-shop.execute",
			Files: map[string]string{stage.DeliverableFileName: "# report rev 3\n"}, SnapshotSHA: gone}); err != nil {
			t.Fatal(err)
		}
		d := w.deliverable(2)
		d.Revision, d.SnapshotSHA = 3, gone
		_, err := cs.RevisionChange(ctx, d, verify.JudgeArtifactBytesCap)
		if !errors.Is(err, review.ErrContentDrift) {
			t.Fatalf("lost pin: err = %v, want review.ErrContentDrift", err)
		}
	})
}

// TestTQ8NewVerifierWiresTheChangeSeam — the composition: newVerifier hands
// the drain the Change seam beside the review sink whenever the review store
// is wired, so a production verify leg reads the tree slice by construction.
func TestTQ8NewVerifierWiresTheChangeSeam(t *testing.T) {
	w := newTQ8World(t)
	wired, err := stage.VerifierChangeSeamWired(w.ctx, w.sk, verify.DomainSoftware, "t-shop")
	if err != nil {
		t.Fatalf("newVerifier: %v", err)
	}
	if !wired {
		t.Fatal("newVerifier composes no verify.ChangeSource although the review store is wired (P3-TQ-8)")
	}
}
