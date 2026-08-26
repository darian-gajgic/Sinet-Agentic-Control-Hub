package conformance_test

// kimicli_ln7_test.go — P3-LN-7 §10 specs T26, T29 (S03.3, S00.9 A12, S16).
//
// The third substrate's conformance rows, and the ONE Spec/ edit this packet
// makes. No provider call anywhere: the spec half is a file read and the
// registry half is a pure data function.

import (
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/conformance"
)

// ── T26 · the registry gains a SUBSTRATE row and a LANE row ──────────────────

// TestConformanceSeedRowCounts pins exact numbers, never inequalities (§63/§64).
// Two rows land, not one: the substrate and the lane are different claims with
// different scopes, and one row asserting both would be honest about neither.
func TestConformanceSeedRowCounts(t *testing.T) {
	rows := conformance.SeedRows()
	if len(rows) != 17 {
		t.Errorf("SeedRows returned %d rows, want 17 (15 before this packet + the kimi-cli substrate row + the kimi-cli lane row)", len(rows))
	}

	byID := map[string]conformance.Row{}
	for _, r := range rows {
		byID[r.ID] = r
	}
	for _, id := range []string{"adapter-kimi-cli", "lane-kimi-cli"} {
		row, ok := byID[id]
		if !ok {
			t.Errorf("no %q row in the registry", id)
			continue
		}
		if row.AffectClass != conformance.AffectLane {
			t.Errorf("%s: affect class = %q, want %q — lane-affecting rows are flag-now", id, row.AffectClass, conformance.AffectLane)
		}
		if row.Cadence != conformance.CadenceWeekly {
			t.Errorf("%s: cadence = %q, want weekly", id, row.Cadence)
		}
		if len(row.Fixtures) == 0 {
			t.Errorf("%s names no fixtures", id)
		}
		if !strings.Contains(strings.Join(row.TriggerSet, ","), conformance.TriggerEngineBump) {
			t.Errorf("%s: trigger set = %v, missing engine_bump — the pin published 70 versions in ~3 months", id, row.TriggerSet)
		}
		// The two-limb bump-gate reference rides every engine_bump row as DATA.
		for _, needle := range []string{"(a)", "(b)", "S14.8", "B5-5"} {
			if !strings.Contains(row.Notes, needle) {
				t.Errorf("%s: notes miss the bump-gate needle %q", id, needle)
			}
		}
		// Every fixture handle declares its tier, so a reader can tell what
		// actually ran from what was skipped.
		for _, f := range row.Fixtures {
			h := strings.ToLower(f.Handle)
			if !strings.Contains(h, "tier f") && !strings.Contains(h, "tier r") && !strings.Contains(h, "tier l") {
				t.Errorf("%s: fixture %q does not declare its tier", id, f.Handle)
			}
		}
	}

	// The two rows are honest about BOTH halves of their scope.
	sub := byID["adapter-kimi-cli"]
	for _, needle := range []string{"SUBSTRATE", "does NOT prove"} {
		if !strings.Contains(sub.Notes, needle) {
			t.Errorf("the substrate row's notes miss %q — a row that does not say what it fails to prove is read as proving everything", needle)
		}
	}
	lane := byID["lane-kimi-cli"]
	for _, needle := range []string{"pool", "does NOT prove"} {
		if !strings.Contains(lane.Notes, needle) {
			t.Errorf("the lane row's notes miss %q", needle)
		}
	}
	// The gray-zone Gate-A posture is recorded where a person reading the row
	// will see it, not only in the audit file.
	if !strings.Contains(lane.Notes, "gray zone") && !strings.Contains(lane.Notes, "Gate A") {
		t.Error("the kimi-cli lane row does not record the 2026-08-26 Gate-A ruling")
	}
}

func TestBumpGatingRowsMovedToEleven(t *testing.T) {
	rows := conformance.SeedRows()
	n := 0
	for _, r := range rows {
		if strings.Contains(strings.Join(r.TriggerSet, ","), conformance.TriggerEngineBump) {
			n++
		}
	}
	if n != 11 {
		t.Errorf("%d rows carry engine_bump, want 11 (9 before this packet + the two kimi-cli rows)", n)
	}
}

// ── T29 · A12 landed in BOTH copies, byte-identically ────────────────────────

func TestA12AmendmentLandedInBothCopies(t *testing.T) {
	draft := readSpec(t, s00Draft)
	full := readSpec(t, assembled)

	var a12 string
	for _, line := range strings.Split(draft, "\n") {
		if strings.HasPrefix(line, "| A12 |") {
			a12 = line
			break
		}
	}
	if a12 == "" {
		t.Fatalf("%s carries no A12 row in the S00.9 post-G4 changelog", s00Draft)
	}
	if !strings.Contains(full, a12) {
		t.Error("the assembled spec does not carry the A12 row byte-identically — draft and assembled must not diverge")
	}
	for _, needle := range []string{
		"2026-08-26",       // the amendment's date and the operator order
		"kimi-cli",         // the subject
		"A11",              // the audit whose verdict it corrects
		"no S18 re-sweep",  // no ⚙ default or clamp is touched
		"118",              // the settings tally
		"THIRD substrate",  // what actually changed in S03.2
		"sharing the same", // the verbatim shared-quota line that grounds the pool
	} {
		if !strings.Contains(a12, needle) {
			t.Errorf("the A12 row does not carry %q: %s", needle, a12)
		}
	}
	// The Gate-A ruling is IN the row. An amendment that rests on an operator
	// decision and does not record it is an amendment nobody can audit.
	if strings.Contains(a12, "<OPERATOR GATE-A RULING>") {
		t.Error("the A12 row still carries its placeholder — the row is not applied while the ruling is empty")
	}
	if !strings.Contains(a12, "interactive") {
		t.Error("the A12 row does not record the interactive-only clause its Gate-A re-audit turned on")
	}
	// The metered-exception list stays EMPTY (G1 P7).
	if !strings.Contains(a12, "EMPTY") && !strings.Contains(a12, "empty") {
		t.Error("the A12 row does not record that the metered-exception list stays empty")
	}

	// The four marker sites A12 claims are annotated, actually are.
	s03 := readSpec(t, "Spec/drafts/S03-engines-adapters.md")
	if strings.Contains(s03, "exactly two substrates") {
		t.Error("S03.2 still says \"exactly two substrates\" — A12 names that sentence as a marker site")
	}
	if !strings.Contains(s03, "A12") {
		t.Error("S03 carries no A12 annotation")
	}
	roadmap := sectionAround(s03, "### S03.6 Lane roadmap & onboarding", 6)
	if !strings.Contains(roadmap, "kimi-cli") {
		t.Errorf("the S03.6 lane roadmap does not name the kimi-cli lane: %q", roadmap)
	}
	s16 := readSpec(t, "Spec/drafts/S16-adoption-manifest.md")
	for _, needle := range []string{"kimi-cli", "0.38.0", "2026-08-26"} {
		if !strings.Contains(s16, needle) {
			t.Errorf("the S16 lane-onboarding manifest does not carry %q — A12 claims a second row there", needle)
		}
	}

	// The ⚙ tally is UNCHANGED, asserted rather than assumed. A fourth lane
	// document is another ROW of an existing surface, never a new surface.
	s18 := readSpec(t, "Spec/drafts/S18-settings-registry.md")
	if !strings.Contains(s18, "**118 dotted registry keys** across **33 domains**") {
		t.Error("the S18 tally prose moved — this packet declares no ⚙ key and must not touch it")
	}
	if !strings.Contains(s18, "**3 data-valued settings surfaces**") {
		t.Error("the S18.3 data-surface count moved")
	}
}
