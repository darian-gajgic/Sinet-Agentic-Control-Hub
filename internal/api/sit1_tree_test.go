package api_test

// sit1_tree_test.go — P3-SIT-1 R10–R14 at the transport, plus the three
// spec-stated invariants as property tests (R15), committed RED by grounding
// (Amendment-A carve-out, CONVENTIONS §3).
//
// The world is REAL end to end: a project onboarded from a file:// fixture,
// its run-branch worktree, platform snapshot commits taken by
// project.Store.Snapshot, revisions minted with those pins and their
// refs/sinet/deliverable/<id>/rev-<n> refs created — and the review store's
// TreeSource wired to the project store through a test adapter that does what
// the shell's projectSeams does (resolve the deliverable's project, pass the
// git facts through). Nothing here fakes a tree: every served inventory, diff
// and file body is compared against what the test itself WROTE.

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
)

// sit1Seam adapts the project store to review.TreeSource for one project,
// exactly as the shell's projectSeams will: the deliverable id resolves to its
// task (the pipeline id), the project is the store's own, and the git facts
// pass through with the same change-kind words.
type sit1Seam struct {
	e         *dlvEnv
	projectID string
}

func (s sit1Seam) TreeBase(ctx context.Context, deliverableID string) (string, bool, error) {
	sha, err := s.e.proj.BaseSHA(ctx, s.projectID, strings.TrimPrefix(deliverableID, "dlv-"))
	if err != nil {
		return "", false, err
	}
	return sha, sha != "", nil
}

func (s sit1Seam) TreeChanges(ctx context.Context, _, oldSHA, newSHA string) ([]review.ChangedFile, error) {
	rows, err := s.e.proj.TreeChanges(ctx, s.projectID, oldSHA, newSHA)
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

func (s sit1Seam) TreeBlob(ctx context.Context, _, sha, path string, limit int64) ([]byte, int64, bool, error) {
	return s.e.proj.TreeBlob(ctx, s.projectID, sha, path, limit)
}

// sit1World: project "shop" seeded with README.md + src/app.go, task t-shop
// with its execute and verify runs, and the repo-backed deliverable dlv-t-shop
// (dtype code) at revision 1 = base + src/cart.go added + app.go modified, and
// revision 2 = app.go modified again, README.md deleted, assets/logo.png
// added. Each revision carries the companion report and its platform ref.
type sit1World struct {
	e    *dlvEnv
	ws   string
	base string
	pins []string // pins[n-1] = revision n's snapshot sha
}

const (
	sit1Readme  = "# shop\n"
	sit1AppBase = "package app\n\nfunc Run() {}\n"
	sit1AppRev1 = "package app\n\nfunc Run() {\n\tserve()\n}\n"
	sit1AppRev2 = "package app\n\nfunc Run() {\n\tserve()\n\tlisten()\n}\n"
	sit1Cart    = "package app\n\nfunc Cart() {}\n"
	sit1PNG     = "\x89PNG\r\n\x1a\n\x00\x00IHDR"
)

func newSIT1World(t *testing.T) *sit1World {
	t.Helper()
	e := newDlvEnv(t)
	w := &sit1World{e: e}
	e.prepareProject("shop", "alice", map[string]string{"README.md": sit1Readme, "src/app.go": sit1AppBase})
	w.base = e.baseSHA
	e.mkRun("t-shop", "t-shop.execute", "alice")
	e.mkRun("t-shop", "t-shop.verify", "alice")
	ws, err := e.proj.EnsureWorkspace(e.ctx, "shop", "t-shop")
	if err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	w.ws = ws.Path
	if _, err := e.rev.EnsureDeliverable(e.ctx, review.EnsureInput{
		ID: "dlv-t-shop", Owner: "alice", TaskID: "t-shop", ProjectID: "shop", Type: "code",
	}); err != nil {
		t.Fatalf("EnsureDeliverable: %v", err)
	}
	w.mintTree(t, map[string]string{"README.md": sit1Readme, "src/app.go": sit1AppRev1, "src/cart.go": sit1Cart})
	w.mintTree(t, map[string]string{"src/app.go": sit1AppRev2, "src/cart.go": sit1Cart, "assets/logo.png": sit1PNG})
	e.rev.Tree = sit1Seam{e: e, projectID: "shop"}
	return w
}

// mintTree makes the worktree hold EXACTLY files, takes the platform snapshot,
// mints the next revision on it and creates its platform ref — the execute
// stage-close + verify-handoff sequence, minus the engine.
func (w *sit1World) mintTree(t *testing.T, files map[string]string) string {
	t.Helper()
	e := w.e
	entries, err := os.ReadDir(w.ws)
	if err != nil {
		t.Fatal(err)
	}
	for _, ent := range entries {
		if ent.Name() == ".git" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(w.ws, ent.Name())); err != nil {
			t.Fatal(err)
		}
	}
	for rel, body := range files {
		p := filepath.Join(w.ws, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sha, err := e.proj.Snapshot(e.ctx, w.ws)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	n := len(w.pins) + 1
	if _, err := e.rev.MintRevision(e.ctx, review.MintInput{
		DeliverableID: "dlv-t-shop", N: n, RunID: "t-shop.verify", AttemptRef: fmt.Sprintf("t-shop.verify#round-%d", n),
		ProducedBy:  "t-shop.execute",
		Files:       map[string]string{"deliverable.md": fmt.Sprintf("# report rev %d\n", n)},
		SnapshotSHA: sha,
	}); err != nil {
		t.Fatalf("MintRevision %d: %v", n, err)
	}
	if err := e.proj.CreateRevisionRef(e.ctx, "shop", review.RevisionRef("dlv-t-shop", n), sha); err != nil {
		t.Fatalf("CreateRevisionRef %d: %v", n, err)
	}
	w.pins = append(w.pins, sha)
	return sha
}

func sit1Decode(t *testing.T, body string, into any) {
	t.Helper()
	if err := json.Unmarshal([]byte(body), into); err != nil {
		t.Fatalf("decode: %v\n%s", err, body)
	}
}

func sit1KindsOf(files []review.ChangedFile) map[string]string {
	out := map[string]string{}
	for _, f := range files {
		out[f.Path] = f.Kind
	}
	return out
}

func sit1FilesPath(id string, n int, path string) string {
	return "/api/deliverables/" + id + "/files?revision=" + fmt.Sprint(n) + "&path=" + url.QueryEscape(path)
}

// TestSIT1DetailServesTheChangeInventory — R10. The deliverable detail carries
// the CURRENT revision's default change (N vs N−1; 1 vs the pre-task base):
// the whole file inventory with kinds, sizes and the two pins — the S15.3
// snapshot, with file bodies one read away. A content-pinned deliverable
// serves no `change` key at all.
func TestSIT1DetailServesTheChangeInventory(t *testing.T) {
	w := newSIT1World(t)
	e := w.e
	var detail struct {
		Change *review.Change `json:"change"`
	}
	sit1Decode(t, e.mustDo(t, "alice", "GET", "/api/deliverables/dlv-t-shop", ""), &detail)
	if detail.Change == nil {
		t.Fatal("the detail of a repo-backed deliverable served no change inventory (SIT-F1: the code has to be in the deliverable)")
	}
	ch := detail.Change
	if ch.OldN != 1 || ch.NewN != 2 || ch.OldIsBase || ch.OldPin != w.pins[0] || ch.NewPin != w.pins[1] {
		t.Fatalf("detail change pair = %d→%d base=%v pins %q→%q, want 1→2 on %q→%q", ch.OldN, ch.NewN, ch.OldIsBase, ch.OldPin, ch.NewPin, w.pins[0], w.pins[1])
	}
	want := map[string]string{"README.md": review.KindDeleted, "assets/logo.png": review.KindAdded, "src/app.go": review.KindModified}
	if got := sit1KindsOf(ch.Files); !equalStringMaps(got, want) {
		t.Fatalf("detail inventory = %v, want %v (what the test wrote)", got, want)
	}
	for _, row := range ch.Files {
		switch row.Path {
		case "src/app.go":
			if row.OldSize != int64(len(sit1AppRev1)) || row.NewSize != int64(len(sit1AppRev2)) || row.Binary {
				t.Errorf("app.go row = %+v", row)
			}
		case "assets/logo.png":
			if !row.Binary || row.NewSize != int64(len(sit1PNG)) {
				t.Errorf("logo.png row = %+v, want binary with its size", row)
			}
		}
	}

	// A content-pinned deliverable: no `change` key — the lane is untouched.
	e.mkRun("t-note", "r-note", "alice")
	e.mkDeliverable("dlv-t-note", "alice", "t-note", "r-note", "", "markdown", map[string]string{"deliverable.md": "note\n"}, "")
	var raw map[string]json.RawMessage
	sit1Decode(t, e.mustDo(t, "alice", "GET", "/api/deliverables/dlv-t-note", ""), &raw)
	if _, present := raw["change"]; present {
		t.Fatalf("a content-pinned detail grew a change key: %s", raw["change"])
	}
}

func equalStringMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// TestSIT1CompareRoundOverRoundAndOnDemand — R11. The default compare is
// N vs N−1 over the TREE; old=0 is the pre-task base; the whole inventory
// rides the comparison and the unified text is the per-file diffs.
func TestSIT1CompareRoundOverRoundAndOnDemand(t *testing.T) {
	w := newSIT1World(t)
	e := w.e
	var cmp review.Comparison
	sit1Decode(t, e.mustDo(t, "alice", "GET", "/api/deliverables/dlv-t-shop/compare", ""), &cmp)
	if cmp.OldN != 1 || cmp.NewN != 2 || cmp.Surface != review.SurfaceLineDiff || cmp.Change == nil {
		t.Fatalf("default compare = %d→%d surface %q change %v", cmp.OldN, cmp.NewN, cmp.Surface, cmp.Change != nil)
	}
	if !strings.Contains(cmp.Unified, "diff --git a/src/app.go b/src/app.go") || !strings.Contains(cmp.Unified, "+\tlisten()") || !strings.Contains(cmp.Unified, "-# shop") {
		t.Fatalf("default compare is not the tree diff:\n%s", cmp.Unified)
	}
	if strings.Contains(cmp.Unified, "report rev") {
		t.Fatalf("the companion report leaked into the tree diff:\n%s", cmp.Unified)
	}
	var base review.Comparison
	sit1Decode(t, e.mustDo(t, "alice", "GET", "/api/deliverables/dlv-t-shop/compare?old=0&new=1", ""), &base)
	if base.Change == nil || !base.Change.OldIsBase || base.Change.OldPin != w.base {
		t.Fatalf("old=0 must be the recorded pre-task base %q: %+v", w.base, base.Change)
	}
	want := map[string]string{"src/app.go": review.KindModified, "src/cart.go": review.KindAdded}
	if got := sit1KindsOf(base.Change.Files); !equalStringMaps(got, want) {
		t.Fatalf("base→rev1 inventory = %v, want %v (README.md unchanged from the base is not a change)", got, want)
	}
	var pair review.Comparison
	sit1Decode(t, e.mustDo(t, "alice", "GET", "/api/deliverables/dlv-t-shop/compare?old=0&new=2", ""), &pair)
	if pair.Change == nil || len(pair.Change.Files) != 4 {
		t.Fatalf("0→2 on demand: %+v", pair.Change)
	}
}

// TestSIT1PerFileDiffAndFileContentReads — R12/R13. `?path=` narrows the
// compare to one file; the files read serves one file's bytes at the pin
// (byte-identical to what was written), flags a binary and serves no bytes
// for it, reaches the companion report by name, and answers every bad input
// with a 4xx — never a 500.
func TestSIT1PerFileDiffAndFileContentReads(t *testing.T) {
	w := newSIT1World(t)
	e := w.e
	var one review.Comparison
	sit1Decode(t, e.mustDo(t, "alice", "GET", "/api/deliverables/dlv-t-shop/compare?path=src%2Fapp.go", ""), &one)
	if one.Change == nil || len(one.Change.Files) != 1 || one.Change.Files[0].Path != "src/app.go" {
		t.Fatalf("?path= inventory = %+v, want the one row", one.Change)
	}
	if !strings.HasPrefix(one.Unified, "diff --git a/src/app.go b/src/app.go") || strings.Contains(one.Unified, "README") {
		t.Fatalf("?path= diff is not that file alone:\n%s", one.Unified)
	}
	if code, _ := e.do(t, "alice", "GET", "/api/deliverables/dlv-t-shop/compare?path=src%2Fcart.go", ""); code != http.StatusNotFound {
		t.Fatalf("?path= on a file the change does not cover: %d, want 404", code)
	}

	var fc review.FileContent
	sit1Decode(t, e.mustDo(t, "alice", "GET", sit1FilesPath("dlv-t-shop", 2, "src/cart.go"), ""), &fc)
	if fc.Content != sit1Cart || fc.Size != int64(len(sit1Cart)) || fc.Binary || fc.Truncated || fc.Pin != w.pins[1] || fc.RevisionN != 2 {
		t.Fatalf("files read = %+v, want the written bytes at pin %q", fc, w.pins[1])
	}
	sit1Decode(t, e.mustDo(t, "alice", "GET", sit1FilesPath("dlv-t-shop", 1, "README.md"), ""), &fc)
	if fc.Content != sit1Readme {
		t.Fatalf("rev 1 README = %q", fc.Content)
	}
	sit1Decode(t, e.mustDo(t, "alice", "GET", sit1FilesPath("dlv-t-shop", 2, "assets/logo.png"), ""), &fc)
	if !fc.Binary || fc.Content != "" || fc.Size != int64(len(sit1PNG)) {
		t.Fatalf("binary files read = %+v, want binary, no inline bytes, the real size", fc)
	}
	sit1Decode(t, e.mustDo(t, "alice", "GET", sit1FilesPath("dlv-t-shop", 2, "deliverable.md"), ""), &fc)
	if fc.Content != "# report rev 2\n" {
		t.Fatalf("companion report by name = %q", fc.Content)
	}
	// The revision defaults to the current one.
	sit1Decode(t, e.mustDo(t, "alice", "GET", "/api/deliverables/dlv-t-shop/files?path=src%2Fapp.go", ""), &fc)
	if fc.RevisionN != 2 || fc.Content != sit1AppRev2 {
		t.Fatalf("files read without ?revision= = rev %d %q, want the current revision", fc.RevisionN, fc.Content)
	}

	for _, c := range []struct {
		path string
		want int
	}{
		{sit1FilesPath("dlv-t-shop", 2, "README.md"), http.StatusNotFound},       // deleted at rev 2
		{sit1FilesPath("dlv-t-shop", 2, "nope.txt"), http.StatusNotFound},        // never existed
		{sit1FilesPath("dlv-t-shop", 9, "src/app.go"), http.StatusNotFound},      // no such revision
		{sit1FilesPath("dlv-t-shop", 2, "../etc/passwd"), http.StatusBadRequest}, // not a tree path
		{sit1FilesPath("dlv-t-shop", 2, "/src/app.go"), http.StatusBadRequest},   // absolute
		{"/api/deliverables/dlv-t-shop/files?revision=2", http.StatusBadRequest}, // no path
		{"/api/deliverables/dlv-t-shop/files?revision=x&path=a", http.StatusBadRequest},
		{"/api/deliverables/dlv-t-shop/compare?old=1&new=2&path=..%2Fx", http.StatusBadRequest},
	} {
		code, body := e.do(t, "alice", "GET", c.path, "")
		if code != c.want {
			t.Errorf("GET %s: %d, want %d (4xx, never 500): %s", c.path, code, c.want, body)
		}
	}
}

// TestSIT1FilesRouteInheritsTheFamilyScope — R14. The new read joins the
// deliverables family under deliverableScope: 404 before 403 for another
// owner, operator reads, no session is 401, no review store is 503 not_wired,
// and no /v1 exists.
func TestSIT1FilesRouteInheritsTheFamilyScope(t *testing.T) {
	w := newSIT1World(t)
	e := w.e
	p := sit1FilesPath("dlv-t-shop", 2, "src/app.go")
	if code, _ := e.do(t, "bob", "GET", p, ""); code != http.StatusForbidden {
		t.Errorf("another member reading alice's file: %d, want 403", code)
	}
	if code, _ := e.do(t, "op", "GET", p, ""); code != http.StatusOK {
		t.Errorf("operator reading a member's file: %d, want 200", code)
	}
	if code, _ := e.do(t, "bob", "GET", sit1FilesPath("dlv-none", 1, "x"), ""); code != http.StatusNotFound {
		t.Errorf("unknown deliverable for a non-owner: %d, want 404 before 403", code)
	}
	if code, _ := e.do(t, "alice", "GET", "/api/deliverables/dlv-t-shop/compare?path=src%2Fapp.go", ""); code != http.StatusOK {
		t.Errorf("owner per-file compare: %d, want 200", code)
	}
	closed := api.New(api.Config{
		Log: e.b.log, Sessions: e.b.store, Settings: approvalSettings(),
		HealthFn: func() api.Health { return api.Health{Ready: true} }, DB: e.b.db, Meter: fakeMeter{},
		Review: e.rev, Accept: e.acc,
	})
	rr := httptest.NewRecorder()
	closed.Handler().ServeHTTP(rr, httptest.NewRequest("GET", p, nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("no identity: %d, want 401", rr.Code)
	}
	b := newBackend(t)
	seedUser(t, b, "op", auth.RoleOperator)
	unwired := api.New(api.Config{
		Log: b.log, Sessions: b.store, Auth: fixedIdentity{"op"}, Settings: approvalSettings(),
		HealthFn: func() api.Health { return api.Health{Ready: true} }, DB: b.db, Meter: fakeMeter{},
	})
	rr = httptest.NewRecorder()
	unwired.Handler().ServeHTTP(rr, httptest.NewRequest("GET", p, nil))
	if rr.Code != http.StatusServiceUnavailable || !strings.Contains(rr.Body.String(), "not_wired") {
		t.Errorf("review store not wired: %d %s, want 503 not_wired", rr.Code, rr.Body.String())
	}
	if code, _ := e.do(t, "op", "GET", "/api/v1"+p, ""); code != http.StatusNotFound {
		t.Errorf("/api/v1 prefix must not exist: %d", code)
	}
}

// ── R15: the three spec-stated invariants, as properties over random trees ──
//
// For ANY two pinned trees A → B (random files over a fixed path pool, random
// line contents, some binary):
//
//	(i)   the served inventory is exactly the set of paths whose blobs differ
//	      (a rename row covers its old and new path) — never the executor's
//	      report;
//	(ii)  the served per-file unified diff applied to A's content yields B's
//	      content (git apply, outside any repository) for every modified
//	      text file;
//	(iii) the served content of every text file at B is byte-identical to
//	      what was written into the tree that B pins.

var sit1PathPool = []string{"README.md", "src/app.go", "src/cart.go", "src/lib/util.go", "docs/notes.txt", "assets/logo.png", "tests/cart_test.go", "config.json"}

var sit1Words = []string{"alpha", "beta", "gamma", "delta", "serve()", "listen()", "return nil", "// note", "\tif ok {", "}", "x := 1", "package app"}

func sit1RandomTree(r *rand.Rand) map[string]string {
	files := map[string]string{}
	for _, p := range sit1PathPool {
		if r.Intn(3) == 0 {
			continue // absent
		}
		if strings.HasSuffix(p, ".png") && r.Intn(2) == 0 {
			files[p] = sit1PNG + string(rune('A'+r.Intn(26)))
			continue
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "// %s\n", p)
		for n := r.Intn(12); n > 0; n-- {
			sb.WriteString(sit1Words[r.Intn(len(sit1Words))])
			sb.WriteByte('\n')
		}
		if r.Intn(4) == 0 {
			sb.WriteString("no newline at end") // exercise the \ No newline marker
		}
		files[p] = sb.String()
	}
	return files
}

func sit1IsBinary(s string) bool { return strings.ContainsRune(s, 0) }

// sit1ExpectedChange is the test's own answer, computed from the two maps it
// wrote — independent of git.
func sit1ExpectedChange(a, b map[string]string) map[string]bool {
	out := map[string]bool{}
	for p := range a {
		if bc, ok := b[p]; !ok || bc != a[p] {
			out[p] = true
		}
	}
	for p := range b {
		if _, ok := a[p]; !ok {
			out[p] = true
		}
	}
	return out
}

// sit1ApplyDiff applies a unified diff to base content with the host git,
// outside any repository, and returns the result.
func sit1ApplyDiff(t *testing.T, e *dlvEnv, path, base, unified string) string {
	t.Helper()
	dir := t.TempDir()
	target := filepath.Join(dir, path)
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(base), 0o600); err != nil {
		t.Fatal(err)
	}
	patch := filepath.Join(dir, "sit1.patch")
	if err := os.WriteFile(patch, []byte(unified), 0o600); err != nil {
		t.Fatal(err)
	}
	e.git("-C", dir, "apply", "--unsafe-paths", patch)
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	return string(got)
}

func TestSIT1PropInventoryDiffAndContentInvariants(t *testing.T) {
	w := newSIT1World(t)
	e := w.e
	r := rand.New(rand.NewSource(20260917))
	prev := map[string]string{"src/app.go": sit1AppRev2, "src/cart.go": sit1Cart, "assets/logo.png": sit1PNG} // rev 2's tree
	const cases = 14
	for i := 0; i < cases; i++ {
		next := sit1RandomTree(r)
		w.mintTree(t, next)
		n := len(w.pins)
		expect := sit1ExpectedChange(prev, next)

		// (i) the inventory is the differing-blob set.
		var cmp review.Comparison
		sit1Decode(t, e.mustDo(t, "alice", "GET", fmt.Sprintf("/api/deliverables/dlv-t-shop/compare?old=%d&new=%d", n-1, n), ""), &cmp)
		if cmp.Change == nil {
			t.Fatalf("case %d: no inventory served", i)
		}
		served := map[string]bool{}
		for _, row := range cmp.Change.Files {
			served[row.Path] = true
			if row.Kind == review.KindRenamed {
				served[row.OldPath] = true
			} else if row.OldPath != "" {
				t.Errorf("case %d: old_path on a non-rename row %+v", i, row)
			}
			if _, inNext := next[row.Path]; row.Kind == review.KindDeleted && inNext {
				t.Errorf("case %d: %s reported deleted but present at the pin", i, row.Path)
			}
			if _, inPrev := prev[row.Path]; row.Kind == review.KindAdded && inPrev {
				t.Errorf("case %d: %s reported added but present on the old side", i, row.Path)
			}
			if b := sit1IsBinary(prev[row.Path]) || sit1IsBinary(next[row.Path]); row.Binary != b {
				t.Errorf("case %d: %s binary=%v, want %v", i, row.Path, row.Binary, b)
			}
		}
		if !sit1SameSet(served, expect) {
			t.Errorf("case %d: inventory %v, want the differing paths %v", i, sit1Keys(served), sit1Keys(expect))
		}
		if strings.Contains(cmp.Unified, "report rev") {
			t.Errorf("case %d: the companion report is in the tree diff", i)
		}

		// (ii) per-file diff round-trips; (iii) served content is the written bytes.
		for _, row := range cmp.Change.Files {
			if row.Binary || row.Kind == review.KindRenamed {
				continue
			}
			if row.Kind == review.KindModified {
				var one review.Comparison
				sit1Decode(t, e.mustDo(t, "alice", "GET", fmt.Sprintf("/api/deliverables/dlv-t-shop/compare?old=%d&new=%d&path=%s", n-1, n, url.QueryEscape(row.Path)), ""), &one)
				if one.Truncated {
					t.Fatalf("case %d: a small diff was truncated: %s", i, one.TruncationReason)
				}
				if got := sit1ApplyDiff(t, e, row.Path, prev[row.Path], one.Unified); got != next[row.Path] {
					t.Errorf("case %d: applying the served diff of %s to the old content does not yield the new content:\n--- got\n%s\n--- want\n%s\n--- diff\n%s",
						i, row.Path, got, next[row.Path], one.Unified)
				}
			}
			if row.Kind != review.KindDeleted {
				var fc review.FileContent
				sit1Decode(t, e.mustDo(t, "alice", "GET", sit1FilesPath("dlv-t-shop", n, row.Path), ""), &fc)
				if fc.Content != next[row.Path] || fc.Truncated || fc.Pin != w.pins[n-1] {
					t.Errorf("case %d: served content of %s at rev %d differs from the written bytes (truncated=%v pin=%q)", i, row.Path, n, fc.Truncated, fc.Pin)
				}
			}
		}
		// Every unchanged text file is served verbatim too.
		for p, body := range next {
			if expect[p] || sit1IsBinary(body) {
				continue
			}
			var fc review.FileContent
			sit1Decode(t, e.mustDo(t, "alice", "GET", sit1FilesPath("dlv-t-shop", n, p), ""), &fc)
			if fc.Content != body {
				t.Errorf("case %d: unchanged file %s at rev %d served differently", i, p, n)
			}
		}
		prev = next
	}
}

func sit1SameSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

func sit1Keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
