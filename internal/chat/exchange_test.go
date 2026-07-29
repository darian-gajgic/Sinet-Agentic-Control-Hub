package chat_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/chat"
)

// exchange_test.go — the S15.7 file exchange: owner attribution, the structural
// bounds, the resolve-then-deny confinement probes, and the OQ7 produced-files
// diff. Nothing here dials anything; every byte lives in t.TempDir().

func upload(t *testing.T, e *env, owner, name, body string) chat.File {
	t.Helper()
	f, err := e.store.Upload(e.ctx, owner, name, []byte(body), "", "")
	if err != nil {
		t.Fatalf("upload %q as %s: %v", name, owner, err)
	}
	return f
}

// TestExchangeIsOwnerAttributedAndOwnerScoped: the manifest row carries the
// owner and the identity of the bytes, and the folder is invisible to anyone
// else — in BOTH directions.
func TestExchangeIsOwnerAttributedAndOwnerScoped(t *testing.T) {
	e := newEnv(t)
	body := "id,amount\n1,42\n"
	f := upload(t, e, "alice", "quarterly.csv", body)
	upload(t, e, "bob", "bobs.csv", "x")

	sum := sha256.Sum256([]byte(body))
	if f.Owner != "alice" || f.Name != "quarterly.csv" ||
		f.SizeBytes != int64(len(body)) || f.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("manifest row = %+v", f)
	}
	mine, err := e.store.ListFiles(e.ctx, "alice")
	if err != nil || len(mine) != 1 || mine[0].ID != f.ID {
		t.Fatalf("alice's folder = %+v %v, want exactly her own object", mine, err)
	}
	for _, who := range []string{"bob", "op", ""} {
		if _, err := e.store.FileRow(e.ctx, who, f.ID); err != chat.ErrNotFound {
			t.Errorf("FileRow(%q) on alice's object = %v, want ErrNotFound", who, err)
		}
		res, err := e.store.Delete(e.ctx, who, f.ID)
		if err != nil || res.Applied {
			t.Errorf("Delete(%q) on alice's object = %+v %v, want applied:false", who, res, err)
		}
	}
	if _, err := e.store.FileRow(e.ctx, "alice", f.ID); err != nil {
		t.Fatalf("alice's object must survive a stranger's delete: %v", err)
	}
	if n := e.eventCount(t, chat.EventFileUploaded); n != 2 {
		t.Errorf("chat.file_uploaded rows = %d, want 2", n)
	}
}

// TestExchangeDeleteRemovesRowAndBytes, and is retry-safe.
func TestExchangeDeleteRemovesRowAndBytes(t *testing.T) {
	e := newEnv(t)
	f := upload(t, e, "alice", "notes.txt", "hello")
	before := countFilesOnDisk(t, e.root)
	if before != 1 {
		t.Fatalf("expected one object on disk, found %d", before)
	}
	res, err := e.store.Delete(e.ctx, "alice", f.ID)
	if err != nil || !res.Applied || res.File.Name != "notes.txt" {
		t.Fatalf("delete = %+v %v", res, err)
	}
	if n := countFilesOnDisk(t, e.root); n != 0 {
		t.Errorf("%d objects still on disk after a delete", n)
	}
	if n := e.eventCount(t, chat.EventFileDeleted); n != 1 {
		t.Errorf("chat.file_deleted rows = %d, want 1", n)
	}
	again, err := e.store.Delete(e.ctx, "alice", f.ID)
	if err != nil || again.Applied {
		t.Fatalf("repeat delete = %+v %v, want applied:false", again, err)
	}
	if n := e.eventCount(t, chat.EventFileDeleted); n != 1 {
		t.Errorf("a repeat delete must mint nothing more, got %d rows", n)
	}
}

func countFilesOnDisk(t *testing.T, root string) int {
	t.Helper()
	n := 0
	err := filepath.WalkDir(root, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			n++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return n
}

// TestExchangeBoundsRefuseWithTheirReasons: size and count are structural
// constants, and both refuse loudly rather than truncating or evicting.
func TestExchangeBoundsRefuseWithTheirReasons(t *testing.T) {
	e := newEnv(t)
	if _, err := e.store.Upload(e.ctx, "alice", "huge.bin",
		make([]byte, chat.MaxFileBytes+1), "", ""); err == nil {
		t.Fatal("an over-size upload must be refused")
	} else if !strings.Contains(err.Error(), "too large") {
		t.Errorf("size refusal = %v, want the too-large class", err)
	}
	if _, err := e.store.Upload(e.ctx, "alice", "empty.txt", nil, "", ""); err == nil {
		t.Error("an empty upload must be refused")
	}
	if n := countFilesOnDisk(t, e.root); n != 0 {
		t.Errorf("a refused upload wrote %d objects to disk", n)
	}
	if n := e.eventCount(t, chat.EventFileUploaded); n != 0 {
		t.Errorf("a refused upload minted %d events", n)
	}
}

// TestTraversalProbesFailClosed is the R6 confinement battery. Three shapes —
// a parent reference, an absolute path, and a planted SYMLINK — and none of
// them writes, reads or unlinks a byte outside the exchange root.
func TestTraversalProbesFailClosed(t *testing.T) {
	e := newEnv(t)
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	secret := filepath.Join(outside, "passwd")
	if err := os.WriteFile(secret, []byte("root:x:0:0"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	// (a) parent references and (b) absolute paths are refused as NAMES, so
	// they never reach the filesystem at all.
	for _, name := range []string{
		"../../etc/passwd", "..", ".", "a/b.txt", `a\b.txt`,
		"/etc/passwd", "sub/../../escape.txt", "no\x00null.txt", "bell\x07.txt",
	} {
		if _, err := e.store.Upload(e.ctx, "alice", name, []byte("payload"), "", ""); err == nil {
			t.Errorf("upload with name %q was ACCEPTED — a filename may never carry a path", name)
		}
	}
	if _, err := chat.SafeName("ok-name.csv"); err != nil {
		t.Fatalf("the non-tautological control: a plain filename must be accepted, got %v", err)
	}
	if n := countFilesOnDisk(t, e.root); n != 0 {
		t.Fatalf("the traversal probes wrote %d objects", n)
	}
	if _, err := os.Stat(filepath.Join(outside, "payload")); err == nil {
		t.Fatal("a traversal probe wrote outside the exchange root")
	}
	if b, err := os.ReadFile(secret); err != nil || string(b) != "root:x:0:0" {
		t.Fatalf("the file outside the root was touched: %v %q", err, b)
	}

	// (c) a SYMLINK planted where the owner's directory goes: resolve-then-deny
	// must refuse it even though nothing about the name is suspicious.
	e2 := newEnv(t)
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.MkdirAll(victim, 0o700); err != nil {
		t.Fatalf("mkdir victim: %v", err)
	}
	dirs, err := os.ReadDir(e2.root)
	if err != nil {
		t.Fatalf("read exchange root: %v", err)
	}
	if len(dirs) != 0 {
		t.Fatalf("expected an empty exchange root, found %d entries", len(dirs))
	}
	// The layout is opaque, so the owner directory's name is derived exactly as
	// the store derives it — planting the link where the store will look.
	sum := sha256.Sum256([]byte("alice"))
	ownerDir := filepath.Join(e2.root, hex.EncodeToString(sum[:])[:32])
	if err := os.Symlink(victim, ownerDir); err != nil {
		t.Skipf("SANCTIONED SKIP (CONVENTIONS §10): this filesystem does not support symlinks: %v", err)
	}
	if _, err := e2.store.Upload(e2.ctx, "alice", "innocent.txt", []byte("payload"), "", ""); err == nil {
		t.Error("an upload through a symlinked owner directory was ACCEPTED — resolve-then-deny did not fire")
	}
	if n := countFilesOnDisk(t, victim); n != 0 {
		t.Errorf("the symlink probe wrote %d objects outside the exchange root", n)
	}
}

// TestNoSandboxMountsTheExchange is the S11.3 negative: no confinement or
// sandbox code names the exchange root. The exchange is control-plane
// territory; a file reaches a run as an intake INPUT, never as a mount.
func TestNoSandboxMountsTheExchange(t *testing.T) {
	roots := []string{"../sandbox", "../stage", "../preview", "../worker"}
	for _, dir := range roots {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, ent := range entries {
			if !strings.HasSuffix(ent.Name(), ".go") {
				continue
			}
			src, err := os.ReadFile(filepath.Join(dir, ent.Name()))
			if err != nil {
				t.Fatalf("read %s: %v", ent.Name(), err)
			}
			if strings.Contains(string(src), `"exchange"`) || strings.Contains(string(src), "internal/chat") {
				t.Errorf("%s/%s names the exchange — no sandbox may mount it (S11.3 allowlist-only)", dir, ent.Name())
			}
		}
	}
}

// TestProducedFilesDiffIsAWindowAndAnOriginRef drives BOTH halves of OQ7: what
// appeared while the turn ran, and what arrived afterwards saying which turn
// caused it. It also pins the honest EMPTY case, which at v0 is the common one
// — no platform-side producer writes into the folder yet.
func TestProducedFilesDiffIsAWindowAndAnOriginRef(t *testing.T) {
	e := newEnv(t)
	s, _ := e.store.CreateSession(e.ctx, "alice")

	// A turn with nothing appearing during it: honestly empty, never fabricated.
	quiet, _, _ := e.store.BeginTurn(e.ctx, "alice", s.ID, chat.KindAsk, "nothing happens here")
	settledQuiet, err := e.store.SettleTurn(e.ctx, "alice", quiet.ID, chat.OutcomeAnswer, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("settle: %v", err)
	}
	if len(settledQuiet.Produced) != 0 {
		t.Fatalf("a turn with no produced files must render empty, got %v", settledQuiet.Produced)
	}

	// A turn during which an object appears: the WINDOW half.
	turn, _, _ := e.store.BeginTurn(e.ctx, "alice", s.ID, chat.KindAsk, "now something appears")
	during := upload(t, e, "alice", "during.txt", "produced while the turn ran")
	settled, err := e.store.SettleTurn(e.ctx, "alice", turn.ID, chat.OutcomeAnswer, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("settle: %v", err)
	}
	if len(settled.Produced) != 1 || settled.Produced[0] != during.ID {
		t.Fatalf("window diff = %v, want [%s]", settled.Produced, during.ID)
	}

	// A LATE arrival naming the turn as its origin: the origin-ref half.
	late, err := e.store.Upload(e.ctx, "alice", "late.txt", []byte("arrived afterwards"), s.ID, turn.ID)
	if err != nil {
		t.Fatalf("late upload: %v", err)
	}
	reread, err := e.store.Turn(e.ctx, "alice", turn.ID)
	if err != nil {
		t.Fatalf("re-read turn: %v", err)
	}
	if len(reread.Produced) != 2 {
		t.Fatalf("produced after a late arrival = %v, want the window object plus the origin-attributed one", reread.Produced)
	}
	if !contains(reread.Produced, late.ID) || !contains(reread.Produced, during.ID) {
		t.Fatalf("produced = %v, missing %s or %s", reread.Produced, late.ID, during.ID)
	}
	// And it did NOT leak onto the quiet turn — attribution is per turn.
	quietAgain, _ := e.store.Turn(e.ctx, "alice", quiet.ID)
	if len(quietAgain.Produced) != 0 {
		t.Errorf("the quiet turn gained %v — a late arrival must attribute to the turn that caused it", quietAgain.Produced)
	}

	// A cross-owner origin ref is refused: nobody labels their upload as having
	// come out of somebody else's turn.
	if _, err := e.store.Upload(e.ctx, "bob", "forged.txt", []byte("x"), s.ID, turn.ID); err != chat.ErrNotFound {
		t.Errorf("cross-owner origin ref = %v, want ErrNotFound", err)
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// TestNoTickerOrWatcherInThisPackage is the §32 negative: the produced-files
// diff is a QUERY over stored rows, run once at settle. A ticker or a
// filesystem watcher would be a second source of truth that drifts.
func TestNoTickerOrWatcherInThisPackage(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	banned := []string{"time.Tick", "time.NewTicker", "fsnotify", "inotify", "time.AfterFunc"}
	seen := false
	for _, ent := range entries {
		n := ent.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		seen = true
		src, err := os.ReadFile(n)
		if err != nil {
			t.Fatalf("read %s: %v", n, err)
		}
		for _, b := range banned {
			if strings.Contains(string(src), b) {
				t.Errorf("%s uses %s — the produced-files diff derives from stored rows, never from a clock or a watcher (§32)", n, b)
			}
		}
	}
	if !seen {
		t.Fatal("the scan read no source files — it would pass vacuously")
	}
}
