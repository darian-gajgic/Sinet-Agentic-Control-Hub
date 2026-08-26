package conformance_test

// lanepin_ln9_test.go — P3-LN-9 R11 (S00.9 A13, S08.8, S15.2, S18).
//
// The ONE Spec/ edit this packet makes, held in both copies and at all three
// marker sites — plus the tally assertions R11 requires be ASSERTED against the
// spec text rather than assumed. A packet that declares no ⚙ key and quietly
// moves one would be indistinguishable from this one at review time.
//
// No provider call: every leg here is a file read.

import (
	"strings"
	"testing"
)

func TestA13AmendmentLandedInBothCopies(t *testing.T) {
	draft := readSpec(t, s00Draft)
	full := readSpec(t, assembled)

	var a13 string
	for _, line := range strings.Split(draft, "\n") {
		if strings.HasPrefix(line, "| A13 |") {
			a13 = line
			break
		}
	}
	if a13 == "" {
		t.Fatalf("%s carries no A13 row in the S00.9 post-G4 changelog", s00Draft)
	}
	// ONE ROW ON ONE LINE, and byte-identical in the assembled copy — the
	// A11/A12 discipline. Neither copy may be edited alone.
	if !strings.Contains(full, a13) {
		t.Error("the assembled spec does not carry the A13 row byte-identically — the draft is canonical and the " +
			"assembled file is REGENERATED, never hand-edited")
	}
	// It sits immediately after A12: the changelog is chronological and a row
	// out of order is a row whose ordering means nothing.
	lines := strings.Split(draft, "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "| A13 |") {
			if i == 0 || !strings.HasPrefix(lines[i-1], "| A12 |") {
				t.Errorf("the A13 row does not follow A12; the line above it is %q", lines[max0(i-1)])
			}
			break
		}
	}

	for _, needle := range []string{
		"2026-08-26",                // the date and the operator order
		"PINNED LANE",               // the subject
		"task CREATION",             // the (b) half — the new expression point
		"the pinnable AXIS",         // the (a) half — the axis being named
		"POST /api/intake/requests", // the create verb as it exists at v0
		"no S18 re-sweep",           // no ⚙ default or clamp is touched
		"118",                       // the settings tally
		"D5 is untouched",           // the money claim
		"S12.1 class (a)",           // the local tier's own refusal ground
		"direct arm",                // the benchmark exemption
		"EMPTY",                     // the metered-exception list
	} {
		if !strings.Contains(a13, needle) {
			t.Errorf("the A13 row does not carry %q", needle)
		}
	}
	// The ruling is IN the row. An amendment resting on an operator decision
	// that does not record it is an amendment nobody can audit.
	if strings.Contains(a13, "<OPERATOR RULING>") {
		t.Error("the A13 row still carries its placeholder — the row is not applied while the ruling is empty")
	}
	if !strings.Contains(a13, "coordinator-drafted option text, operator-selected, in-session") {
		t.Error("the A13 row does not carry the ruling's attribution in the A12 phrasing")
	}

	// ── The three marker sites A13 claims are annotated, actually are ──────
	//
	// Scoped to the SITES rather than file-wide: a file-wide match is satisfied
	// by the tag appearing anywhere, including in a sentence about something
	// else (the §67 lesson about vacuous scans).
	s08 := readSpec(t, "Spec/drafts/S08-workers-composition-routing.md")
	visible := lineContaining(s08, "**Visible and overridable:**")
	if visible == "" {
		t.Fatal("S08.8 carries no \"Visible and overridable\" paragraph — A13 names it as a marker site")
	}
	for _, needle := range []string{"[S00.9 A13]", "LANE", "task CREATION", "REFUSED"} {
		if !strings.Contains(visible, needle) {
			t.Errorf("the S08.8 marker site does not carry %q: %q", needle, visible)
		}
	}
	// Coverage still binds and still outranks the pin, said at the site.
	if !strings.Contains(visible, "coverage") {
		t.Errorf("the S08.8 annotation does not restate that coverage binds: %q", visible)
	}

	s15 := readSpec(t, "Spec/drafts/S15-frontend-api.md")
	tasks := lineContaining(s15, "| tasks | `/api/tasks` |")
	if tasks == "" {
		t.Fatal("S15.2 carries no `tasks` family row — A13 names it as a marker site")
	}
	if !strings.Contains(tasks, "[S00.9 A13]") || !strings.Contains(tasks, "lane pin") {
		t.Errorf("the S15.2 tasks row's mutating-verbs cell does not note the optional lane pin: %q", tasks)
	}

	// The code-side marker (the A12 `adapters.go` precedent): the routing head
	// comment enumerates the pin as a sanctioned step-3 override input.
	routing := readSpec(t, "internal/worker/routing.go")
	head := routing[:strings.Index(routing, "// EventRoutingDecided")]
	for _, needle := range []string{"[S00.9 A13]", "PINNED LANE", "REFUSED"} {
		if !strings.Contains(head, needle) {
			t.Errorf("the internal/worker/routing.go head comment does not carry %q", needle)
		}
	}

	// ── The tallies, ASSERTED rather than assumed ─────────────────────────
	//
	// The pinnable set is derived from what an operator has actually placed and
	// has no dotted key, so nothing here may move. A packet that says "⚙: none"
	// and moves one reads identically to this one without these two lines.
	s18 := readSpec(t, "Spec/drafts/S18-settings-registry.md")
	if !strings.Contains(s18, "**118 dotted registry keys** across **33 domains**") {
		t.Error("the S18 tally prose moved — this packet declares no ⚙ key and must not touch it")
	}
	if !strings.Contains(s18, "**3 data-valued settings surfaces**") {
		t.Error("the S18.3 data-surface count moved — a per-task pin is not a settings surface")
	}
}

func max0(i int) int {
	if i < 0 {
		return 0
	}
	return i
}
