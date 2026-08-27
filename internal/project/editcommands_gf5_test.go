package project

// editcommands_gf5_test.go — P3-GF5 at the STORE boundary (Spec S13.7: the
// registry holds a project's build/test/lint/run/preview commands, "rows are
// owner-attributed" and "captured content is versioned"; migration 0008's own
// comment anticipates this writer — "the version bumps on every
// re-capture/re-scan/EDIT and stays traceable").
//
// COMMITTED RED (CONVENTIONS §3 Amendment-A): `EditCommands` and the rune cap
// do not exist yet, so this file fails to compile against the pre-GF5 store —
// the packet's implementation commit closes the window. Colocated in a NEW
// file because the packet may not modify pre-existing test files (the
// project_onboard_seam_test.go precedent one package over).
//
// T5 IS THE BRIEF'S PROPERTY-BASED SPEC. Both properties run over a seeded
// generator with stdlib math/rand — no new dependency, a fixed seed so a
// failure is reproducible from the log line alone, and enough generated worlds
// that "changes only Commands" is a claim about the verb rather than about one
// hand-picked capture.

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// ── the generator ───────────────────────────────────────────────────────────

// gf5Alphabet mixes ASCII with multi-byte runes on purpose: the cap counts
// RUNES (a byte count would refuse a shorter sentence written in a language
// with wider characters), and the round-trip through the registry's JSON
// columns has to preserve them byte-equal.
var gf5Alphabet = []rune("abcdefghijklmnopqrstuvwxyz0123456789 ./-_=éüñ漢字—")

// gf5Word builds one generated token of n runes.
func gf5Word(rng *rand.Rand, n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteRune(gf5Alphabet[rng.Intn(len(gf5Alphabet))])
	}
	return b.String()
}

// gf5Commands generates one submitted command set. Slots are filled
// independently (an all-empty set is a legal submission — it returns the
// project to the bootstrap posture, which is the honest recompute), and a
// filled slot is randomly padded with surrounding whitespace so the "round-trips
// byte-equal AFTER TRIM" half of the property is non-vacuous.
func gf5Commands(rng *rand.Rand) Commands {
	slot := func() string {
		if rng.Intn(3) == 0 {
			return ""
		}
		v := gf5Word(rng, 1+rng.Intn(40))
		switch rng.Intn(3) {
		case 0:
			return "  " + v
		case 1:
			return v + "\t "
		}
		return v
	}
	return Commands{Build: slot(), Test: slot(), Lint: slot(), Run: slot(), Preview: slot()}
}

// gf5Trim is what the store is expected to store: each slot trimmed, nothing
// else touched.
func gf5Trim(c Commands) Commands {
	return Commands{
		Build:   strings.TrimSpace(c.Build),
		Test:    strings.TrimSpace(c.Test),
		Lint:    strings.TrimSpace(c.Lint),
		Run:     strings.TrimSpace(c.Run),
		Preview: strings.TrimSpace(c.Preview),
	}
}

// gf5PriorCapture generates the capture the edit must carry forward: arbitrary
// conventions, danger zones, scan hash and family, plus whatever commands the
// world happens to start with.
func gf5PriorCapture(rng *rand.Rand) CaptureInput {
	var conventions []string
	for i := rng.Intn(4); i > 0; i-- {
		conventions = append(conventions, gf5Word(rng, 5+rng.Intn(20)))
	}
	var zones []DangerZone
	for i := rng.Intn(3); i > 0; i-- {
		zones = append(zones, DangerZone{
			Path:       gf5Word(rng, 4+rng.Intn(8)) + "/",
			Rule:       gf5Word(rng, 6+rng.Intn(20)),
			SourceHash: gf5Word(rng, 12),
		})
	}
	families := append(Families(), "")
	return CaptureInput{
		Conventions: conventions,
		Commands:    gf5Trim(gf5Commands(rng)),
		DangerZones: zones,
		ScanHash:    gf5Word(rng, 16),
		Family:      families[rng.Intn(len(families))],
	}
}

// gf5Seed registers, captures and activates one generated world.
func gf5Seed(t *testing.T, f *fix, id, owner string, in CaptureInput) {
	t.Helper()
	ctx := context.Background()
	if _, err := f.store.Register(ctx, RegisterInput{
		ProjectID: id, Owner: owner, Name: id, StorePath: filepath.Join(f.root, id),
	}); err != nil {
		t.Fatalf("Register %s: %v", id, err)
	}
	in.ProjectID, in.By = id, owner
	if _, err := f.store.Capture(ctx, in); err != nil {
		t.Fatalf("Capture %s: %v", id, err)
	}
	if _, err := f.store.Activate(ctx, id, owner); err != nil {
		t.Fatalf("Activate %s: %v", id, err)
	}
}

// gf5CaptureRows reads every stored capture row of a project as raw column
// text, keyed by version. It is what makes "leaves every prior capture row
// byte-immutable" a claim about the BYTES rather than about a decoded struct.
func gf5CaptureRows(t *testing.T, f *fix, projectID string) map[int]string {
	t.Helper()
	rows, err := f.db.QueryContext(context.Background(), `
		SELECT version, conventions, commands, danger_zones, scan_hash, family, captured_by, captured_ts
		  FROM repo_registry_captures WHERE project_id = ? ORDER BY version`, projectID)
	if err != nil {
		t.Fatalf("read capture rows: %v", err)
	}
	defer rows.Close()
	out := map[int]string{}
	for rows.Next() {
		var v int
		var conventions, commands, zones, hash, family, by, ts string
		if err := rows.Scan(&v, &conventions, &commands, &zones, &hash, &family, &by, &ts); err != nil {
			t.Fatalf("scan capture row: %v", err)
		}
		out[v] = strings.Join([]string{conventions, commands, zones, hash, family, by, ts}, "\x1f")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("capture rows: %v", err)
	}
	return out
}

func gf5SameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func gf5SameZones(a, b []DangerZone) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ── T5, property 1: the edit changes ONLY Commands ──────────────────────────

// TestEditCommandsChangesOnlyCommands [PROPERTY, brief T5]: over generated
// prior captures and generated valid command sets, the edit verb
//
//   - mints a NEW version exactly +1 when the command set actually differs, and
//     mints NOTHING when it does not (S15.2: a phone retry can never double-fire);
//   - carries conventions, danger zones, scan hash and family forward byte-equal
//     (the Rescan carry-forward discipline: dropping the scan hash would leave
//     DriftCheck comparing against nothing, and dropping conventions or zones
//     would silently unset owner-approved content with no event behind it);
//   - leaves every PRIOR capture row byte-immutable (S13.7 "captured content is
//     versioned"; migration 0008's own triggers);
//   - round-trips the accepted values byte-equal after trim.
func TestEditCommandsChangesOnlyCommands(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	const seed = 20260827
	rng := rand.New(rand.NewSource(seed))

	for i := 0; i < 48; i++ {
		id := fmt.Sprintf("p-%02d", i)
		gf5Seed(t, f, id, "alice", gf5PriorCapture(rng))

		before, err := f.store.Get(ctx, id)
		if err != nil {
			t.Fatalf("world %d (seed %d): Get before: %v", i, seed, err)
		}
		rowsBefore := gf5CaptureRows(t, f, id)

		submitted := gf5Commands(rng)
		want := gf5Trim(submitted)

		got, minted, err := f.store.EditCommands(ctx, id, "alice", submitted)
		if err != nil {
			t.Fatalf("world %d (seed %d): EditCommands(%+v): %v", i, seed, submitted, err)
		}

		wantMinted := want != before.Capture.Commands
		if minted != wantMinted {
			t.Fatalf("world %d (seed %d): minted = %v, want %v (prior %+v, submitted %+v)",
				i, seed, minted, wantMinted, before.Capture.Commands, want)
		}
		if got.Commands != want {
			t.Fatalf("world %d (seed %d): returned commands %+v, want %+v (byte-equal after trim)", i, seed, got.Commands, want)
		}

		after, err := f.store.Get(ctx, id)
		if err != nil {
			t.Fatalf("world %d (seed %d): Get after: %v", i, seed, err)
		}
		wantVersion := before.CaptureVersion
		if wantMinted {
			wantVersion++
		}
		if after.CaptureVersion != wantVersion {
			t.Fatalf("world %d (seed %d): capture version v%d, want v%d", i, seed, after.CaptureVersion, wantVersion)
		}
		if after.Capture.Commands != want {
			t.Fatalf("world %d (seed %d): stored commands %+v, want %+v", i, seed, after.Capture.Commands, want)
		}
		if wantMinted && after.Capture.CapturedBy != "alice" {
			t.Fatalf("world %d (seed %d): captured_by = %q, want the caller (S13.7 owner-attributed)", i, seed, after.Capture.CapturedBy)
		}

		// Everything that is not commands, carried forward byte-equal.
		if !gf5SameStrings(after.Capture.Conventions, before.Capture.Conventions) {
			t.Fatalf("world %d (seed %d): conventions changed: %v → %v", i, seed, before.Capture.Conventions, after.Capture.Conventions)
		}
		if !gf5SameZones(after.Capture.DangerZones, before.Capture.DangerZones) {
			t.Fatalf("world %d (seed %d): danger zones changed: %v → %v", i, seed, before.Capture.DangerZones, after.Capture.DangerZones)
		}
		if after.Capture.ScanHash != before.Capture.ScanHash {
			t.Fatalf("world %d (seed %d): scan hash changed %q → %q — DriftCheck would compare against nothing",
				i, seed, before.Capture.ScanHash, after.Capture.ScanHash)
		}
		if after.Capture.Family != before.Capture.Family {
			t.Fatalf("world %d (seed %d): family changed %q → %q — every later task would land in the wrong question set",
				i, seed, before.Capture.Family, after.Capture.Family)
		}

		// The carry-forward is BYTE-equal, not merely decode-equal. A stored
		// column that round-trips to the same Go value while holding different
		// bytes (`[]` vs `null`) is a difference nothing in the decoded world
		// notices and that a committed golden fixture reads — which is how this
		// assertion earned its place.
		rowsAfter := gf5CaptureRows(t, f, id)
		if wantMinted {
			was := strings.Split(rowsBefore[before.CaptureVersion], "\x1f")
			now := strings.Split(rowsAfter[after.CaptureVersion], "\x1f")
			// column order: conventions, commands, danger_zones, scan_hash, family, by, ts
			for _, col := range []struct {
				name string
				at   int
			}{{"conventions", 0}, {"danger_zones", 2}, {"scan_hash", 3}, {"family", 4}} {
				if was[col.at] != now[col.at] {
					t.Fatalf("world %d (seed %d): stored %s is not byte-equal across the edit:\n was: %q\n now: %q",
						i, seed, col.name, was[col.at], now[col.at])
				}
			}
		}
		for v, was := range rowsBefore {
			now, ok := rowsAfter[v]
			if !ok {
				t.Fatalf("world %d (seed %d): capture v%d was deleted — capture rows are immutable and never deleted", i, seed, v)
			}
			if now != was {
				t.Fatalf("world %d (seed %d): capture v%d was rewritten:\n was: %q\n now: %q", i, seed, v, was, now)
			}
		}
	}
}

// ── T5, property 2: the validation companion ────────────────────────────────

// gf5Poison is one unrunnable slot value with the rule it violates. A captured
// command is ONE shell line the network-off C2 verification sandbox will later
// run as `/bin/sh -lc <cmd>` (Spec S07.3 rule 1; S11) — a multi-line value
// smuggles a script body past that shape, a NUL cannot survive the argv, and an
// invalid encoding is not text the platform can vouch for.
var gf5Poisons = []struct{ name, value string }{
	{"multi-line", "make build\nrm -rf /"},
	{"carriage return", "make build\rrm -rf /"},
	{"NUL", "make\x00build"},
	{"invalid utf-8", "make \xff\xfe build"},
	{"oversize", strings.Repeat("x", commandMaxRunes+1)},
}

// gf5Slots names each command slot with a setter, so the property runs over all
// five rather than over whichever one somebody remembered.
var gf5Slots = []struct {
	name string
	set  func(*Commands, string)
}{
	{"build", func(c *Commands, v string) { c.Build = v }},
	{"test", func(c *Commands, v string) { c.Test = v }},
	{"lint", func(c *Commands, v string) { c.Lint = v }},
	{"run", func(c *Commands, v string) { c.Run = v }},
	{"preview", func(c *Commands, v string) { c.Preview = v }},
}

// TestEditCommandsRefusesUnrunnableSlots [PROPERTY, brief T5 companion]: every
// slot × every unrunnable value is ErrBadInput, the refusal NAMES the slot so a
// caller can fix the request without reading source, and nothing is minted.
func TestEditCommandsRefusesUnrunnableSlots(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	gf5Seed(t, f, "p-val", "alice", CaptureInput{ScanHash: "scan-v1", Family: FamilySoftware})

	for _, slot := range gf5Slots {
		for _, poison := range gf5Poisons {
			var cmds Commands
			slot.set(&cmds, poison.value)
			_, minted, err := f.store.EditCommands(ctx, "p-val", "alice", cmds)
			if !errors.Is(err, ErrBadInput) {
				t.Errorf("%s slot with a %s value: err = %v, want ErrBadInput", slot.name, poison.name, err)
			}
			if minted {
				t.Errorf("%s slot with a %s value reported a mint", slot.name, poison.name)
			}
			if err != nil && !strings.Contains(err.Error(), slot.name) {
				t.Errorf("%s slot with a %s value: refusal does not name the slot: %v", slot.name, poison.name, err)
			}
			e, gerr := f.store.Get(ctx, "p-val")
			if gerr != nil {
				t.Fatalf("Get: %v", gerr)
			}
			if e.CaptureVersion != 1 {
				t.Fatalf("%s slot with a %s value minted version v%d — a refused write mints nothing",
					slot.name, poison.name, e.CaptureVersion)
			}
		}
	}
}

// TestEditCommandsIsOwnerOnlyAndActiveOnly — S13.7 rows are owner-attributed and
// D10 is authority over one's own object: an invited member is refused on the
// sentinel, an unknown project is ErrNotFound, and a PENDING entry is refused
// distinguishably (its draft is edited at the onboarding approval card, which is
// the one door onto a pending draft).
func TestEditCommandsIsOwnerOnlyAndActiveOnly(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	gf5Seed(t, f, "p-own", "alice", CaptureInput{Family: FamilySoftware})

	if _, _, err := f.store.EditCommands(ctx, "p-own", "bob", Commands{Test: "go test ./..."}); !errors.Is(err, ErrNotOwner) {
		t.Errorf("member edit: err = %v, want ErrNotOwner", err)
	}
	if _, _, err := f.store.EditCommands(ctx, "p-nope", "alice", Commands{Test: "go test ./..."}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown project: err = %v, want ErrNotFound", err)
	}
	if _, err := f.store.Register(ctx, RegisterInput{
		ProjectID: "p-pending", Owner: "alice", Name: "p-pending", StorePath: filepath.Join(f.root, "p-pending"),
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, err := f.store.Capture(ctx, CaptureInput{ProjectID: "p-pending", By: "alice"}); err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if _, _, err := f.store.EditCommands(ctx, "p-pending", "alice", Commands{Test: "go test ./..."}); !errors.Is(err, ErrNotActive) {
		t.Errorf("pending entry: err = %v, want ErrNotActive — the draft's door is the onboarding approval card", err)
	}
	if n := gf5CaptureRows(t, f, "p-own"); len(n) != 1 {
		t.Errorf("p-own capture rows = %d, want 1: no refused caller may have minted a version", len(n))
	}
}

// ── T9: the race ────────────────────────────────────────────────────────────

// gf5DeclaredSentinel reports whether err carries one of the package's DECLARED
// refusals. It is what makes "never a raw constraint abort" checkable without
// matching error text (§38): a caller translates on sentinels, so an error that
// carries none of them is one no transport can turn into an answer.
func gf5DeclaredSentinel(err error) bool {
	for _, sentinel := range []error{
		ErrNotFound, ErrNotActive, ErrNotOwner, ErrBadInput, ErrAlreadyRegistered,
		ErrNotFastForward, ErrDirty, ErrUnprotectedRevision,
	} {
		if errors.Is(err, sentinel) {
			return true
		}
	}
	return false
}

// TestGF5ConcurrentWritesNeverSurfaceAConstraintAbort [brief T9]: two writers
// racing the same project's next capture version. The landed Capture read the
// entry's version BEFORE opening its write transaction, so the loser died on the
// (project_id, version) primary key — a raw constraint abort with no meaning for
// the person who pressed the button. Either both writes land (versions 2 and 3)
// or one comes back as a refusal a caller can act on; a SQLite constraint string
// is neither.
func TestGF5ConcurrentWritesNeverSurfaceAConstraintAbort(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	gf5Seed(t, f, "p-race", "alice", CaptureInput{ScanHash: "scan-v1", Family: FamilySoftware})

	writes := []Commands{
		{Build: "go build ./...", Test: "go test ./..."},
		{Lint: "gofmt -l .", Test: "go test -race ./..."},
	}
	errs := make([]error, len(writes))
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i, cmds := range writes {
		wg.Add(1)
		go func(i int, cmds Commands) {
			defer wg.Done()
			<-start
			_, _, errs[i] = f.store.EditCommands(ctx, "p-race", "alice", cmds)
		}(i, cmds)
	}
	close(start)
	wg.Wait()

	landed := 0
	for i, err := range errs {
		if err == nil {
			landed++
			continue
		}
		low := strings.ToLower(err.Error())
		for _, raw := range []string{"constraint", "unique", "sqlite"} {
			if strings.Contains(low, raw) {
				t.Fatalf("writer %d surfaced a raw storage abort: %v", i, err)
			}
		}
		if !gf5DeclaredSentinel(err) {
			t.Fatalf("writer %d failed with an unactionable error: %v (a lost race is a DECLARED refusal a caller can act on, or a clean second version)", i, err)
		}
	}
	e, err := f.store.Get(ctx, "p-race")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if e.CaptureVersion != 1+landed {
		t.Fatalf("capture version v%d after %d landed writes, want v%d", e.CaptureVersion, landed, 1+landed)
	}
	rows := gf5CaptureRows(t, f, "p-race")
	if len(rows) != 1+landed {
		t.Fatalf("capture rows = %d after %d landed writes, want %d", len(rows), landed, 1+landed)
	}
}
