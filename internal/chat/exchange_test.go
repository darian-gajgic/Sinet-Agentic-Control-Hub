package chat_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/chat"
)

// exchange_test.go — the S15.7 file exchange: owner attribution, the structural
// bounds, the resolve-then-deny confinement probes, and the OQ7 produced-files
// diff. Nothing here dials anything; every byte lives in t.TempDir().

func upload(t *testing.T, e *env, owner, name, body string) chat.File {
	t.Helper()
	res, err := e.store.Upload(e.ctx, owner, name, []byte(body), "", "")
	if err != nil {
		t.Fatalf("upload %q as %s: %v", name, owner, err)
	}
	if !res.Applied {
		t.Fatalf("upload %q as %s reported applied:false — this helper uploads new objects", name, owner)
	}
	return res.File
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

// TestExchangeCountBoundRefusesAtTheLimit drives MaxFilesPerOwner. Retention at
// v0 is keep-until-deleted with NO sweeper, so this number is the only thing
// between one person's folder and the disk — an enforced-but-undriven bound is
// a bound nobody has watched fire.
func TestExchangeCountBoundRefusesAtTheLimit(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < chat.MaxFilesPerOwner; i++ {
		upload(t, e, "alice", "f"+strconv.Itoa(i)+".txt", "body "+strconv.Itoa(i))
	}
	_, err := e.store.Upload(e.ctx, "alice", "one-too-many.txt", []byte("x"), "", "")
	if err == nil {
		t.Fatalf("the %dth upload was accepted — MaxFilesPerOwner did not fire", chat.MaxFilesPerOwner+1)
	}
	if !errors.Is(err, chat.ErrTooMany) {
		t.Errorf("count refusal = %v, want the too-many class", err)
	}
	if !strings.Contains(err.Error(), strconv.Itoa(chat.MaxFilesPerOwner)) {
		t.Errorf("count refusal %q does not name its bound — a refusal states its reason", err)
	}
	if n := countFilesOnDisk(t, e.root); n != chat.MaxFilesPerOwner {
		t.Errorf("%d objects on disk after the refusal, want %d", n, chat.MaxFilesPerOwner)
	}
	// The bound is PER OWNER, not per folder: somebody else's quota is their own.
	upload(t, e, "bob", "mine.txt", "bob's first")
}

// TestRepeatedUploadAnswersTheAlreadyResolvedRow is the S15.2 retry-safety leg
// for the upload verb. The same owner sending the same bytes under the same name
// is one act arriving twice — a phone retrying a request whose answer it never
// saw — and it must not cost a second manifest row, a second quota slot or a
// second event.
func TestRepeatedUploadAnswersTheAlreadyResolvedRow(t *testing.T) {
	e := newEnv(t)
	const name, body = "numbers.csv", "id,amount\n1,42\n"
	first, err := e.store.Upload(e.ctx, "alice", name, []byte(body), "", "")
	if err != nil || !first.Applied {
		t.Fatalf("first upload = %+v %v", first, err)
	}
	again, err := e.store.Upload(e.ctx, "alice", name, []byte(body), "", "")
	if err != nil {
		t.Fatalf("repeat upload: %v", err)
	}
	if again.Applied {
		t.Error("a repeated identical upload reported applied:true")
	}
	if again.File.ID != first.File.ID {
		t.Errorf("repeat answered %s, want the already-resolved row %s", again.File.ID, first.File.ID)
	}
	if list, err := e.store.ListFiles(e.ctx, "alice"); err != nil || len(list) != 1 {
		t.Errorf("the folder holds %d rows after a repeat, want 1 (%v)", len(list), err)
	}
	if n := e.eventCount(t, chat.EventFileUploaded); n != 1 {
		t.Errorf("chat.file_uploaded rows = %d after a repeat, want 1", n)
	}
	if n := countFilesOnDisk(t, e.root); n != 1 {
		t.Errorf("%d objects on disk after a repeat, want 1", n)
	}

	// The non-tautological controls: it is the (owner, name, sha256) TRIPLE that
	// makes a repeat, so changing any one of the three is a new object.
	other, err := e.store.Upload(e.ctx, "alice", name, []byte(body+"2"), "", "")
	if err != nil || !other.Applied || other.File.ID == first.File.ID {
		t.Fatalf("different bytes under the same name = %+v %v, want a NEW row", other, err)
	}
	renamed, err := e.store.Upload(e.ctx, "alice", "copy.csv", []byte(body), "", "")
	if err != nil || !renamed.Applied || renamed.File.ID == first.File.ID {
		t.Fatalf("the same bytes under a different name = %+v %v, want a NEW row", renamed, err)
	}
	bobs, err := e.store.Upload(e.ctx, "bob", name, []byte(body), "", "")
	if err != nil || !bobs.Applied {
		t.Fatalf("bob's identical upload = %+v %v, want his OWN row", bobs, err)
	}
	if bobs.File.ID == first.File.ID {
		t.Error("bob's upload answered alice's row — dedupe is per owner, never across the household")
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

// TestSymlinkedLeafIsRefused is the resolve-then-deny letter carried to the LEAF
// (S11.7). The directory check cannot see this one: the owner directory is a
// real directory that resolves inside the root, and the link stands where the
// object itself goes. A plain write would follow it and land the bytes outside.
//
// Unreachable in production today — object ids are crypto/rand and nothing else
// writes into the exchange home — which is exactly why the close is worth
// having: the id seam that makes this drivable is the same seam a future caller
// could use to choose one.
func TestSymlinkedLeafIsRefused(t *testing.T) {
	e := newEnv(t)
	const planted = "cf-plantedleafid"
	e.store.NewID = func(string) string { return planted }

	victimDir := t.TempDir()
	victim := filepath.Join(victimDir, "secret.txt")
	if err := os.WriteFile(victim, []byte("original"), 0o600); err != nil {
		t.Fatalf("write victim: %v", err)
	}
	// The layout is opaque, so the owner directory is derived exactly as the
	// store derives it, and the link is planted where the object will go.
	sum := sha256.Sum256([]byte("alice"))
	ownerDir := filepath.Join(e.root, hex.EncodeToString(sum[:])[:32])
	if err := os.MkdirAll(ownerDir, 0o700); err != nil {
		t.Fatalf("mkdir owner dir: %v", err)
	}
	if err := os.Symlink(victim, filepath.Join(ownerDir, planted)); err != nil {
		t.Skipf("SANCTIONED SKIP (CONVENTIONS §10): this filesystem does not support symlinks: %v", err)
	}

	if _, err := e.store.Upload(e.ctx, "alice", "innocent.txt", []byte("overwritten"), "", ""); err == nil {
		t.Error("an upload onto a symlinked LEAF was accepted — the write followed the link")
	}
	if b, err := os.ReadFile(victim); err != nil || string(b) != "original" {
		t.Fatalf("the file outside the exchange root was rewritten through the leaf link: %v %q", err, b)
	}
	if n := e.eventCount(t, chat.EventFileUploaded); n != 0 {
		t.Errorf("a refused upload minted %d events", n)
	}
	// The non-tautological control: with the link gone, the same id writes.
	if err := os.Remove(filepath.Join(ownerDir, planted)); err != nil {
		t.Fatalf("remove the planted link: %v", err)
	}
	if _, err := e.store.Upload(e.ctx, "alice", "innocent.txt", []byte("overwritten"), "", ""); err != nil {
		t.Fatalf("the probe would pass by uploads never working at all: %v", err)
	}
}

// exchangeHomeMarkers are the ways the CHAT EXCHANGE HOME can be named in Go
// source: its owning package's import path, the composition of the store that
// builds the path, and the `<stateDir>/exchange` join itself.
//
// THEY ARE DELIBERATELY NOT THE WORD "EXCHANGE". `sandbox.RWExchange` and
// `adapters.RWExchange` are a DIFFERENT, UNRELATED concept — the engine's
// copy-aside directory bound read-write into a run's sandbox (the S03.4 gate
// ctl dir, internal/adapters/claudecli) — and widening this scan to the word
// would report every one of them as a violation of a rule they have nothing to
// do with. Keying on the chat store's own path construction is what keeps the
// wall about the thing it guards. TestSandboxRWExchangeIsADifferentConcept
// below states the distinction in the test suite's own words.
var exchangeHomeMarkers = []string{
	`internal/chat"`, // the owning package's import path
	"chat.Config{",   // composing the store, which is what fixes Root
	`"exchange"`,     // the <stateDir>/exchange join the store's root is built from
}

// exchangeHomeCallers are the ONLY packages under internal/ allowed to name the
// exchange home: the store that owns it, the transport that serves it, the shell
// that composes it, and the S14.2 event contract — whose declare-once registry
// names every minting package by construction (§29), and which mounts nothing.
// Everything else in the tree is walled off.
var exchangeHomeCallers = map[string]bool{
	"chat": true, "api": true, "shell": true, "eventlog": true,
}

// TestNoSandboxMountsTheExchange is the S11.3 negative, and it walks the WHOLE
// internal/ tree rather than four hand-listed directories — the old scan missed
// internal/adapters entirely and could not see a subpackage at all. The exchange
// is control-plane territory: a file reaches a run as an intake INPUT, never as
// a mount, so no confinement, sandbox, stage, worker or adapter code may so much
// as name its home.
func TestNoSandboxMountsTheExchange(t *testing.T) {
	scanned := 0
	err := filepath.WalkDir("..", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		pkgDir := filepath.Base(filepath.Dir(path))
		if exchangeHomeCallers[pkgDir] {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		scanned++
		for _, marker := range exchangeHomeMarkers {
			if strings.Contains(string(src), marker) {
				t.Errorf("%s names the chat exchange home (%q) — it is control-plane territory and no sandbox may mount it (S11.3 allowlist-only)", path, marker)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal/: %v", err)
	}
	// The scan must have READ something, and it must be reading the packages
	// this wall exists for — a walk that silently found no files would pass.
	if scanned < 100 {
		t.Fatalf("the scan read only %d files — it is not walking the tree", scanned)
	}
	for _, must := range []string{"../sandbox/sandbox.go", "../adapters/claudecli/claudecli.go", "../stage/surface.go"} {
		if _, err := os.Stat(must); err != nil {
			t.Errorf("%s is not where the scan expects it: %v", must, err)
		}
	}
	// The non-tautological control: the ALLOWED callers do name it, so the
	// markers match real source rather than nothing at all.
	for _, caller := range []string{"../shell/shell.go", "../api/chatapi.go"} {
		src, err := os.ReadFile(caller)
		if err != nil {
			t.Fatalf("read %s: %v", caller, err)
		}
		hit := false
		for _, marker := range exchangeHomeMarkers {
			hit = hit || strings.Contains(string(src), marker)
		}
		if !hit {
			t.Errorf("no marker matches %s — the scan above would pass by matching nothing", caller)
		}
	}
}

// TestSandboxRWExchangeIsADifferentConcept records, as an assertion rather than
// a comment, why the scan above is not keyed on the word "exchange".
//
// `RWExchange` is the sandbox's read-write copy-aside directory — the S03.4 gate
// ctl dir the claudecli adapter binds into a run — and it has nothing to do with
// the S15.7 chat exchange home. Both names would match a word scan; neither
// carries a chat-exchange marker. If a future change ever DID route the chat
// exchange into a sandbox, it would have to name the chat store to get the path,
// and the scan above would catch it.
func TestSandboxRWExchangeIsADifferentConcept(t *testing.T) {
	const adapter = "../adapters/claudecli/claudecli.go"
	src, err := os.ReadFile(adapter)
	if err != nil {
		t.Fatalf("read %s: %v", adapter, err)
	}
	if !strings.Contains(string(src), "RWExchange") {
		t.Fatalf("%s no longer populates RWExchange — the distinction this test draws has moved", adapter)
	}
	for _, marker := range exchangeHomeMarkers {
		if strings.Contains(string(src), marker) {
			t.Errorf("%s carries the chat-exchange marker %q — the two concepts have been conflated", adapter, marker)
		}
	}
}

// TestProducedFilesDiffIsAWindowAndAnOriginRef drives BOTH halves of OQ7: what
// appeared while the turn ran, and what arrived afterwards saying which turn
// caused it. It also pins the honest EMPTY case, which at v0 is the common one
// — no platform-side producer writes into the folder yet.
func TestProducedFilesDiffIsAWindowAndAnOriginRef(t *testing.T) {
	e := newEnv(t)
	// THE CLOCK IS FROZEN, and that is the point of this rig rather than an
	// artifact of it. Every *_ts in this family is second-resolution RFC3339, so
	// an upload landing in the same second as a turn's start is indistinguishable
	// from one landing during it — and with a frozen clock EVERY row shares one
	// stamp, which is exactly the fixture world's own condition. A window that
	// still attributes correctly here is a window no timestamp collision can
	// fool.
	e.store.Now = func() time.Time { return time.Date(2026, 7, 20, 9, 2, 0, 0, time.UTC) }
	s, _ := e.store.CreateSession(e.ctx, "alice")

	// An object that ALREADY EXISTS before any turn opens. No turn may ever
	// claim it: a file a turn merely found is not a file it produced, and a chip
	// saying otherwise fabricates authorship.
	before := upload(t, e, "alice", "before.txt", "uploaded before any turn ran")

	// A turn with nothing appearing during it: honestly empty, never fabricated.
	quiet, _, _ := e.store.BeginTurn(e.ctx, "alice", s.ID, chat.KindAsk, "nothing happens here")
	settledQuiet, err := e.store.SettleTurn(e.ctx, "alice", quiet.ID, chat.OutcomeAnswer, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("settle: %v", err)
	}
	if len(settledQuiet.Produced) != 0 {
		t.Fatalf("a turn with no produced files must render empty, got %v (a pre-existing object is not this turn's product)", settledQuiet.Produced)
	}

	// A turn during which an object appears: the WINDOW half. `before` must stay
	// out of it even though its stamp is identical to the turn's.
	turn, _, _ := e.store.BeginTurn(e.ctx, "alice", s.ID, chat.KindAsk, "now something appears")
	during := upload(t, e, "alice", "during.txt", "produced while the turn ran")
	settled, err := e.store.SettleTurn(e.ctx, "alice", turn.ID, chat.OutcomeAnswer, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("settle: %v", err)
	}
	if len(settled.Produced) != 1 || settled.Produced[0] != during.ID {
		t.Fatalf("window diff = %v, want exactly [%s] — %s existed before the turn opened",
			settled.Produced, during.ID, before.ID)
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
	if !contains(reread.Produced, late.File.ID) || !contains(reread.Produced, during.ID) {
		t.Fatalf("produced = %v, missing %s or %s", reread.Produced, late.File.ID, during.ID)
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

// TestOverlappingTurnWindowsAttributeToAtMostOneTurn is the cross-session tail
// of the same fabricated-authorship defect the window fix closed.
//
// The one-turn rule is enforced per SESSION on purpose — a person with two
// conversations open should be able to run a turn in each, and part B renders
// sessions as switchable — so one owner's turn windows CAN overlap. A file that
// lands in the overlap belongs to no turn in particular, and OQ7 says so
// already: the origin ref decides, and where none resolves it the file
// attributes to no turn. Two turns both printing the same chip under different
// authorship is a fabrication, not a duplicate.
func TestOverlappingTurnWindowsAttributeToAtMostOneTurn(t *testing.T) {
	e := newEnv(t)
	e.store.Now = func() time.Time { return time.Date(2026, 7, 20, 9, 2, 0, 0, time.UTC) }
	sa, _ := e.store.CreateSession(e.ctx, "alice")
	sb, _ := e.store.CreateSession(e.ctx, "alice")

	turnA, _, err := e.store.BeginTurn(e.ctx, "alice", sa.ID, chat.KindAsk, "in the first conversation")
	if err != nil {
		t.Fatalf("begin A: %v", err)
	}
	turnB, _, err := e.store.BeginTurn(e.ctx, "alice", sb.ID, chat.KindAsk, "in the second conversation")
	if err != nil {
		t.Fatalf("begin B — a second session must still be able to open a turn: %v", err)
	}

	// One upload, inside BOTH open windows and naming neither.
	ambiguous := upload(t, e, "alice", "dropped-in.csv", "id,amount\n1,42\n")
	// One upload that DOES name a turn: the origin ref is what resolves an
	// otherwise identical ambiguity.
	named, err := e.store.Upload(e.ctx, "alice", "for-b.csv", []byte("x,y\n1,2\n"), sb.ID, turnB.ID)
	if err != nil {
		t.Fatalf("named upload: %v", err)
	}

	settledA, err := e.store.SettleTurn(e.ctx, "alice", turnA.ID, chat.OutcomeAnswer, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("settle A: %v", err)
	}
	settledB, err := e.store.SettleTurn(e.ctx, "alice", turnB.ID, chat.OutcomeAnswer, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("settle B: %v", err)
	}

	claims := 0
	for _, turn := range []chat.Turn{settledA, settledB} {
		if contains(turn.Produced, ambiguous.ID) {
			claims++
		}
	}
	if claims > 1 {
		t.Errorf("%s is claimed by %d turns (A=%v, B=%v) — a file cannot have been produced by two different turns",
			ambiguous.ID, claims, settledA.Produced, settledB.Produced)
	}
	if claims != 0 {
		t.Errorf("%s was claimed by a turn although nothing resolves which one caused it — OQ7 attributes it to NO turn",
			ambiguous.ID)
	}
	// The origin-ref half still resolves its own ambiguity: the named file lands
	// on B and only B, even though A's window spans it just as widely.
	if !contains(settledB.Produced, named.File.ID) {
		t.Errorf("B's produced = %v, missing %s — an origin ref is what decides an overlap",
			settledB.Produced, named.File.ID)
	}
	if contains(settledA.Produced, named.File.ID) {
		t.Errorf("A claimed %s, which names B as its origin", named.File.ID)
	}
	// The non-tautological control: with no overlap, the window still attributes.
	// A third turn, opened after both closed, claims what arrives during it.
	turnC, _, err := e.store.BeginTurn(e.ctx, "alice", sa.ID, chat.KindAsk, "on its own now")
	if err != nil {
		t.Fatalf("begin C: %v", err)
	}
	alone := upload(t, e, "alice", "solo.csv", "a,b\n3,4\n")
	settledC, err := e.store.SettleTurn(e.ctx, "alice", turnC.ID, chat.OutcomeAnswer, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("settle C: %v", err)
	}
	if len(settledC.Produced) != 1 || settledC.Produced[0] != alone.ID {
		t.Fatalf("an unambiguous turn produced %v, want [%s] — the rule above would pass by attributing nothing, ever",
			settledC.Produced, alone.ID)
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
