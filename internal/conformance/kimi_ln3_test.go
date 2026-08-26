package conformance_test

// kimi_ln3_test.go — P3-LN-3 §6 specs 25-26 (R02 §5, S03.3, S00.9, S16).
//
// The kimi lane's conformance row, and the ONE sanctioned Spec/ edit this
// packet makes: the S00.9 A11 amendment. No Go test executes a provider call
// here; the spec half is a file read.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/conformance"
)

// ── spec 25 · the kimi lane's conformance row ────────────────────────────────

func TestKimiLaneRowRegistered(t *testing.T) {
	rows := conformance.SeedRows()
	var row conformance.Row
	kimiRows := 0
	// Scoped to the `kimi` LANE's own row. A substring match on "kimi" also
	// catches the kimi-cli substrate and lane rows added at P3-LN-7 — a
	// DIFFERENT lane on a different engine, which this test says nothing about.
	// The one-row invariant is unchanged; what moved is which ids it covers.
	for _, r := range rows {
		if r.ID != "adapter-kimi" {
			continue
		}
		kimiRows++
		row = r
	}
	if kimiRows != 1 {
		t.Fatalf("%d adapter-kimi rows registered, want exactly 1", kimiRows)
	}

	if row.AffectClass != conformance.AffectLane {
		t.Errorf("affect class = %q, want %q — a lane row is lane-affecting and therefore flag-now", row.AffectClass, conformance.AffectLane)
	}
	if row.Cadence != conformance.CadenceWeekly {
		t.Errorf("cadence = %q, want weekly (the adapter-zai sibling's)", row.Cadence)
	}
	if len(row.Fixtures) == 0 {
		t.Fatal("the kimi row names no fixtures")
	}
	triggers := strings.Join(row.TriggerSet, ",")
	for _, want := range []string{conformance.TriggerEngineBump, conformance.TriggerWeekly} {
		if !strings.Contains(triggers, want) {
			t.Errorf("trigger set = %v, missing %q", row.TriggerSet, want)
		}
	}
	// The two-limb bump-gate reference rides every engine_bump row as DATA.
	for _, needle := range []string{"(a)", "(b)", "S14.8", "B5-5"} {
		if !strings.Contains(row.Notes, needle) {
			t.Errorf("the row's notes miss the bump-gate needle %q: %q", needle, row.Notes)
		}
	}
	// The row says what it does NOT prove. No credential exists, so a green row
	// must never be readable as "a paid call on this lane works".
	for _, needle := range []string{"LN-CEREMONY", "$0"} {
		if !strings.Contains(row.Notes, needle) {
			t.Errorf("the kimi row's notes never mention %q — the row must say what it does NOT prove: %q", needle, row.Notes)
		}
	}
	// Tier-prefixed handles, the adapter-zai idiom.
	for _, fx := range row.Fixtures {
		low := strings.ToLower(fx.Handle)
		if !strings.Contains(low, "tier f") && !strings.Contains(low, "tier r") && !strings.Contains(low, "tier l") {
			t.Errorf("fixture handle carries no tier prefix: %q", fx.Handle)
		}
	}

	// (The conformance canary's own lane resolution for this row is pinned
	// where that map lives: internal/watchlist's TestKimiConformanceRowRecords.)
}

// ── spec 26 · the S00.9 A11 amendment, in BOTH copies, byte-identical ────────

const (
	s00Draft  = "Spec/drafts/S00-front-matter.md"
	assembled = "Spec/core-architecture-v1.md"
)

func readSpec(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

func TestA11AmendmentLandedInBothCopies(t *testing.T) {
	draft := readSpec(t, s00Draft)
	full := readSpec(t, assembled)

	// The A11 row itself, located in the draft and required VERBATIM in the
	// assembled copy — the A4-A8 byte-discipline precedent.
	var a11 string
	for _, line := range strings.Split(draft, "\n") {
		if strings.HasPrefix(line, "| A11 |") {
			a11 = line
			break
		}
	}
	if a11 == "" {
		t.Fatalf("%s carries no A11 row in the S00.9 post-G4 changelog", s00Draft)
	}
	if !strings.Contains(full, a11) {
		t.Errorf("the assembled spec does not carry the A11 row byte-identically — draft and assembled must not diverge")
	}
	for _, needle := range []string{
		"2026-08-24",                         // the amendment's date
		"2026-08-23",                         // the operator order that is the D10 approval
		"Kimi",                               // the subject
		"rider 3",                            // the mechanism followed, not bypassed
		"2026-08-24-kimi-lane-gate-audit.md", // the evidence
		"opt-in-credits",                     // the C1 finding
		"no-household-personal-data",         // the C5 rider
		"118",                                // the settings tally
		"no S18 re-sweep",                    // no ⚙ default or clamp is touched
	} {
		if !strings.Contains(a11, needle) {
			t.Errorf("the A11 row does not carry %q: %s", needle, a11)
		}
	}
	// The metered-exception list stays EMPTY (G1 P7).
	if !strings.Contains(a11, "EMPTY") && !strings.Contains(a11, "empty") {
		t.Errorf("the A11 row does not record that the metered-exception list stays empty: %s", a11)
	}

	// The marker sites A11 claims are annotated, actually are.
	s03 := readSpec(t, "Spec/drafts/S03-engines-adapters.md")
	if !strings.Contains(s03, "A11") {
		t.Error("S03 carries no A11 annotation — the amendment claims the lane roadmap and the deferred post-v0 row are annotated")
	}
	roadmap := sectionAround(s03, "### S03.6 Lane roadmap & onboarding", 6)
	if !strings.Contains(roadmap, "Kimi") {
		t.Errorf("the S03.6 lane roadmap does not name the Kimi lane: %q", roadmap)
	}
	if !strings.Contains(s03, "Post-v0 lanes") {
		t.Fatal("the S03.6 deferred Post-v0 lanes row vanished")
	}
	parked := lineContaining(s03, "Post-v0 lanes")
	if !strings.Contains(parked, "Kimi") || !strings.Contains(parked, "A11") {
		t.Errorf("the deferred Post-v0 lanes row does not record that Kimi left it early under A11: %q", parked)
	}
	s16 := readSpec(t, "Spec/drafts/S16-adoption-manifest.md")
	for _, needle := range []string{"Kimi", "opt-in-credits", "@ai-sdk/anthropic", "2026-08-24", "no-household-personal-data", "$3.00"} {
		if !strings.Contains(s16, needle) {
			t.Errorf("the S16 onboarding manifest record does not carry %q", needle)
		}
	}

	// The ⚙ tally is UNCHANGED, asserted rather than assumed.
	s18 := readSpec(t, "Spec/drafts/S18-settings-registry.md")
	if !strings.Contains(s18, "**118 dotted registry keys** across **33 domains**") {
		t.Error("the S18 tally prose moved — this packet declares no ⚙ key and must not touch it")
	}
	if !strings.Contains(s18, "**3 data-valued settings surfaces**") {
		t.Error("the S18.3 data-surface count moved — a SECOND lane document is another row of an existing surface, never a new surface")
	}
}

// TestAssembledSpecReproducesFromDrafts is the byte-check the amendment
// mechanics require: the assembled document is exactly what the verified awk
// concat produces from the drafts, so nobody can edit one copy alone.
func TestAssembledSpecReproducesFromDrafts(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join(repoRoot, "Spec/drafts/S[0-9]*.md"))
	if err != nil {
		t.Fatalf("glob the drafts: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("the draft glob matched nothing — the byte-check would pass vacuously")
	}
	var b strings.Builder
	for i, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		s := string(raw)
		if !strings.HasSuffix(s, "\n") {
			s += "\n"
		}
		if i > 0 {
			// awk 'FNR==1&&NR>1{print""}' — one blank line between files.
			b.WriteString("\n")
		}
		b.WriteString(s)
	}
	got := readSpec(t, assembled)
	if got != b.String() {
		t.Errorf("%s is NOT the concatenation of its drafts (assembled %d bytes, concat %d bytes) — regenerate with:\n"+
			"  cd Spec && awk 'FNR==1&&NR>1{print\"\"}{print}' drafts/S[0-9]*.md > core-architecture-v1.md",
			assembled, len(got), b.Len())
	}
}

// sectionAround returns the n lines following a heading.
func sectionAround(src, heading string, n int) string {
	lines := strings.Split(src, "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, heading) {
			end := i + n
			if end > len(lines) {
				end = len(lines)
			}
			return strings.Join(lines[i:end], "\n")
		}
	}
	return ""
}

func lineContaining(src, needle string) string {
	for _, l := range strings.Split(src, "\n") {
		if strings.Contains(l, needle) {
			return l
		}
	}
	return ""
}
