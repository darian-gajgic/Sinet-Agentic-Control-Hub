package project

// P3-TQ-1 acceptance tests (Spec S02.4d, S13.5; finding TQ-F1): the platform
// snapshot must survive the engine's vanishing atomic-write temp files, stay
// byte-faithful across a retry, and stay LOUD on a genuinely unreadable tree.
//
// The race lives inside ONE git process — `add -A` walks the worktree, lists
// the engine's in-flight `<target>.tmp.<pid>.<12 hex>` temp, the engine's
// rename removes it, and add_file_to_index's lstat dies with "unable to stat"
// (exit 128). No seam between the walk and the add exists in Go code, so the
// smallest seam the code offers is PATH resolution of gitBin (git.go:32): a
// shim `git` ahead of the real one replays the recorded die verbatim on the
// platform's `add -A`, completing the engine's rename exactly as the real
// engine's write did, then hands every later call to the real git.

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// recordedVanish is the path run t-3120e8e3d14591d3 died on (run_events
// stage.finished S-4 / run.state_changed running→crashed, 2026-09-16 20:39Z):
// the engine's Write temp, `<target>.tmp.<pid>.<12 hex>` — claude 2.1.273's
// writer is `.tmp.${process.pid}.${randomBytes(6).toString("hex")}` appended
// to the target path, staged beside it.
const recordedVanish = "src/components/PartGrid.jsx.tmp.107557.477ad8f56e3d"

// snapshotAddBound is the retry bound the brief pins (P3-TQ-1 R5): a
// structural constant, S13.5/S02.4 ratify no key.
const snapshotAddBound = 3

// gitVanishMessage is git's die_errno line for a path that vanished between
// the directory walk and add_file_to_index — byte-for-byte the recorded one.
func gitVanishMessage(path string) string {
	return fmt.Sprintf("fatal: unable to stat '%s': No such file or directory", path)
}

// gitShim is a `git` on PATH that intercepts the platform's `add -A` only:
// the first dieUntil interceptions exit 128 with message on stderr (on the
// first one it also performs renames, worktree-relative temp→target, the
// engine's rename completing under the snapshot); everything else execs the
// real git. Every interception bumps a counter the test reads back.
type gitShim struct {
	counter string
}

func installGitShim(t *testing.T, wt string, dieUntil int, renames map[string]string, message string) *gitShim {
	t.Helper()
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("real git: %v", err)
	}
	dir := t.TempDir()
	counter := filepath.Join(dir, "attempts")
	var b strings.Builder
	b.WriteString("#!/bin/sh\n# P3-TQ-1 test shim: replays git's recorded die on the platform's `add -A`.\n")
	fmt.Fprintf(&b, "REAL='%s'\nCOUNTER='%s'\nDIE_UNTIL=%d\n", real, counter, dieUntil)
	b.WriteString("prev=''\nhit=0\nfor a in \"$@\"; do\n  if [ \"$prev\" = 'add' ] && [ \"$a\" = '-A' ]; then hit=1; fi\n  prev=\"$a\"\ndone\n")
	b.WriteString("if [ \"$hit\" = 1 ]; then\n  n=0\n  if [ -f \"$COUNTER\" ]; then n=$(cat \"$COUNTER\"); fi\n  n=$((n + 1))\n  echo \"$n\" > \"$COUNTER\"\n")
	b.WriteString("  if [ \"$n\" -le \"$DIE_UNTIL\" ]; then\n    if [ \"$n\" = 1 ]; then\n      :\n")
	temps := make([]string, 0, len(renames))
	for temp := range renames {
		temps = append(temps, temp)
	}
	sort.Strings(temps)
	for _, temp := range temps {
		fmt.Fprintf(&b, "      mv -f '%s' '%s'\n", filepath.Join(wt, temp), filepath.Join(wt, renames[temp]))
	}
	fmt.Fprintf(&b, "    fi\n    printf '%%s\\n' '%s' >&2\n    exit 128\n  fi\nfi\n", strings.ReplaceAll(message, "'", `'\''`))
	b.WriteString("exec \"$REAL\" \"$@\"\n")
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(b.String()), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return &gitShim{counter: counter}
}

// attempts reports how many times the platform's `add -A` reached the shim.
func (s *gitShim) attempts(t *testing.T) int {
	t.Helper()
	body, err := os.ReadFile(s.counter)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(body)))
	if err != nil {
		t.Fatalf("shim counter %q: %v", body, err)
	}
	return n
}

// blobBytes reads a blob VERBATIM (no trim — byte identity is the claim).
func blobBytes(t *testing.T, dir, spec string) string {
	t.Helper()
	cmd := exec.Command("git", "cat-file", "blob", spec)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("cat-file blob %s: %v", spec, err)
	}
	return string(out)
}

func treeNames(f *fix, dir string) []string {
	return strings.Split(f.git(dir, "ls-tree", "-r", "--name-only", "HEAD"), "\n")
}

func hasName(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

// R1/R2 — the recorded race, replayed: the snapshot survives the engine's
// vanishing temp by retrying the stage, and the commit holds the produced
// file (the rename target) byte-exact with no transient in it.
func TestTQ1SnapshotSurvivesVanishingUntrackedFile(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.activeProject("shop", "alice", "shop", map[string]string{"main.go": "package main\n"})
	ws, err := f.store.EnsureWorkspace(ctx, "shop", "t-3120e8e3d14591d3")
	if err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	const produced = "src/components/PartGrid.jsx"
	const partGrid = "export function PartGrid({ parts }) {\n  return parts.length\n}\n"
	const app = "export default function App() {}\n"
	// The tree as the walk saw it: one finished file, one in-flight atomic
	// write whose temp the engine renames away before git lstat's it.
	f.writeFiles(ws.Path, map[string]string{recordedVanish: partGrid, "src/App.jsx": app})
	shim := installGitShim(t, ws.Path, 1, map[string]string{recordedVanish: produced}, gitVanishMessage(recordedVanish))

	sha, err := f.store.Snapshot(ctx, ws.Path)
	if err != nil {
		t.Fatalf("Snapshot must survive the engine's vanishing temp file (TQ-F1): %v", err)
	}
	if sha == "" || sha == ws.Base {
		t.Fatalf("snapshot sha %q did not advance past the base %q", sha, ws.Base)
	}
	names := treeNames(f, ws.Path)
	if !hasName(names, produced) || !hasName(names, "src/App.jsx") {
		t.Fatalf("snapshot tree is not the produced tree:\n%s", strings.Join(names, "\n"))
	}
	for _, n := range names {
		if strings.Contains(n, ".tmp.") {
			t.Fatalf("a vanished transient landed in the snapshot: %s", n)
		}
	}
	if got := blobBytes(t, ws.Path, "HEAD:"+produced); got != partGrid {
		t.Fatalf("produced file not byte-exact:\n got %q\nwant %q", got, partGrid)
	}
	if got := blobBytes(t, ws.Path, "HEAD:src/App.jsx"); got != app {
		t.Fatalf("finished file not byte-exact:\n got %q\nwant %q", got, app)
	}
	if n := shim.attempts(t); n != 2 {
		t.Fatalf("add -A attempts = %d, want 2 (the recorded die, then one retry)", n)
	}
}

// R2 (invariant, property) — a snapshot's content is the produced tree, byte
// identical across a retry: the raced snapshot's TREE id equals a control
// snapshot of the same final files taken with no race at all. Tree ids are
// content-addressed (paths + bytes + modes), so equality is byte identity.
func TestTQ1SnapshotIsByteFaithfulAcrossRetry(t *testing.T) {
	rng := rand.New(rand.NewSource(0x5413120e8e3))
	randomText := func() string {
		n := rng.Intn(64)
		var b strings.Builder
		for i := 0; i < n; i++ {
			b.WriteByte(" \n\tabcdefghijklmnopqrstuvwxyz{}();=/*"[rng.Intn(36)])
		}
		return b.String()
	}
	for iter := 0; iter < 5; iter++ {
		t.Run(fmt.Sprintf("iter%d", iter), func(t *testing.T) {
			ctx := context.Background()
			k := 1 + rng.Intn(5)
			final := map[string]string{}   // the produced tree
			onDisk := map[string]string{}  // what the walk saw
			renames := map[string]string{} // temp → target (the engine's rename)
			for i := 0; i < k; i++ {
				target := fmt.Sprintf("src/f%d.js", i)
				content := randomText()
				final[target] = content
				if i == 0 || rng.Intn(2) == 0 {
					temp := fmt.Sprintf("%s.tmp.%d.%012x", target, 1000+rng.Intn(900000), rng.Uint64()&0xffffffffffff)
					renames[temp] = target
					onDisk[temp] = content
					if rng.Intn(2) == 0 {
						onDisk[target] = "stale content the rename replaces\n"
					}
				} else {
					onDisk[target] = content
				}
			}
			temps := make([]string, 0, len(renames))
			for temp := range renames {
				temps = append(temps, temp)
			}
			sort.Strings(temps)

			// Control: the final tree, snapshotted with no race.
			ctrl := newFix(t)
			ctrl.activeProject("shop", "alice", "shop", map[string]string{"main.go": "package main\n"})
			cws, err := ctrl.store.EnsureWorkspace(ctx, "shop", "t-ctrl")
			if err != nil {
				t.Fatalf("control EnsureWorkspace: %v", err)
			}
			ctrl.writeFiles(cws.Path, final)
			if _, err := ctrl.store.Snapshot(ctx, cws.Path); err != nil {
				t.Fatalf("control Snapshot: %v", err)
			}
			want := ctrl.git(cws.Path, "rev-parse", "HEAD^{tree}")

			// Raced: the walk sees the in-flight temps; the engine finishes
			// its renames under the add; git dies once; the retry stages the
			// produced tree.
			f := newFix(t)
			f.activeProject("shop", "alice", "shop", map[string]string{"main.go": "package main\n"})
			ws, err := f.store.EnsureWorkspace(ctx, "shop", "t-raced")
			if err != nil {
				t.Fatalf("EnsureWorkspace: %v", err)
			}
			f.writeFiles(ws.Path, onDisk)
			shim := installGitShim(t, ws.Path, 1, renames, gitVanishMessage(temps[0]))
			if _, err := f.store.Snapshot(ctx, ws.Path); err != nil {
				t.Fatalf("Snapshot under the race: %v", err)
			}
			if got := f.git(ws.Path, "rev-parse", "HEAD^{tree}"); got != want {
				t.Fatalf("raced snapshot tree %s != control tree %s (files %v, temps %v)", got, want, final, temps)
			}
			if n := shim.attempts(t); n != 2 {
				t.Fatalf("add -A attempts = %d, want 2", n)
			}
		})
	}
}

// R3/R5 — the retry is BOUNDED and the failure past the bound is LOUD: git's
// own stderr survives verbatim, the bound is stated, nothing is committed.
func TestTQ1SnapshotRetryIsBoundedAndLoud(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.activeProject("shop", "alice", "shop", map[string]string{"main.go": "package main\n"})
	ws, err := f.store.EnsureWorkspace(ctx, "shop", "task-1")
	if err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	f.writeFiles(ws.Path, map[string]string{"src/App.jsx": "export default function App() {}\n"})
	before := f.git(ws.Path, "rev-parse", "HEAD")
	shim := installGitShim(t, ws.Path, 1<<30, nil, gitVanishMessage(recordedVanish))

	_, err = f.store.Snapshot(ctx, ws.Path)
	if err == nil {
		t.Fatal("a tree git cannot stage on every attempt must fail LOUD, never succeed")
	}
	if !strings.Contains(err.Error(), "unable to stat '"+recordedVanish+"'") {
		t.Fatalf("the error must carry git's own stderr verbatim: %v", err)
	}
	if !strings.Contains(err.Error(), "attempts") {
		t.Fatalf("the error must state the exhausted bound: %v", err)
	}
	if n := shim.attempts(t); n != snapshotAddBound {
		t.Fatalf("add -A attempts = %d, want exactly the bound %d", n, snapshotAddBound)
	}
	if after := f.git(ws.Path, "rev-parse", "HEAD"); after != before {
		t.Fatalf("a failed snapshot committed: %s -> %s", before, after)
	}
}

// R3 (pin, real git, no shim) — a GENUINELY unreadable tree stays loud on
// this host git, leaves no index.lock behind (the worktree stays usable for
// the engine), commits nothing, and snapshots normally once readable again.
func TestTQ1SnapshotUnreadableTreeStaysLoud(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.activeProject("shop", "alice", "shop", map[string]string{"main.go": "package main\n"})
	ws, err := f.store.EnsureWorkspace(ctx, "shop", "task-1")
	if err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	f.writeFiles(ws.Path, map[string]string{"locked/secret.txt": "x\n", "src/App.jsx": "ok\n"})
	locked := filepath.Join(ws.Path, "locked")
	// Listable but not searchable: the walk lists locked/secret.txt, lstat
	// on it fails — git's die path, for a reason no retry can clear.
	if err := os.Chmod(locked, 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
	before := f.git(ws.Path, "rev-parse", "HEAD")

	_, err = f.store.Snapshot(ctx, ws.Path)
	if err == nil {
		t.Fatal("an unreadable tree must fail LOUD (CONVENTIONS §14/§23: never faked)")
	}
	if !strings.Contains(err.Error(), "unable to stat 'locked/secret.txt'") {
		t.Fatalf("expected this host git's die shape, got: %v", err)
	}
	lock := f.git(ws.Path, "rev-parse", "--git-path", "index.lock")
	if !filepath.IsAbs(lock) {
		lock = filepath.Join(ws.Path, lock)
	}
	if _, statErr := os.Stat(lock); statErr == nil {
		t.Fatalf("index.lock left behind after the loud failure: %s", lock)
	}
	if after := f.git(ws.Path, "rev-parse", "HEAD"); after != before {
		t.Fatalf("a failed snapshot committed: %s -> %s", before, after)
	}
	if err := os.Chmod(locked, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Snapshot(ctx, ws.Path); err != nil {
		t.Fatalf("once readable again the snapshot must succeed: %v", err)
	}
	if names := treeNames(f, ws.Path); !hasName(names, "locked/secret.txt") || !hasName(names, "src/App.jsx") {
		t.Fatalf("recovered snapshot is not the produced tree:\n%s", strings.Join(names, "\n"))
	}
}

// R4 (pin, real git, no shim) — a transient that STAYS (an orphaned in-flight
// write) is captured, never hidden: the snapshot is the tree as it is, and
// the reviewer sees the orphan beside its possibly-stale target.
func TestTQ1SnapshotStayingTransientIsCapturedNotHidden(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.activeProject("shop", "alice", "shop", map[string]string{"main.go": "package main\n"})
	ws, err := f.store.EnsureWorkspace(ctx, "shop", "task-1")
	if err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	const orphan = "src/x.jsx.tmp.4242.0123456789ab"
	const half = "export function X() {\n  // the engine died here\n"
	f.writeFiles(ws.Path, map[string]string{orphan: half, "src/x.jsx": "export function X() {}\n"})

	if _, err := f.store.Snapshot(ctx, ws.Path); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	names := treeNames(f, ws.Path)
	if !hasName(names, orphan) || !hasName(names, "src/x.jsx") {
		t.Fatalf("a staying transient was dropped from the snapshot without a record:\n%s", strings.Join(names, "\n"))
	}
	if got := blobBytes(t, ws.Path, "HEAD:"+orphan); got != half {
		t.Fatalf("orphan not byte-exact:\n got %q\nwant %q", got, half)
	}
}
