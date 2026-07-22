package verify_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/verify"
)

// S07.10/S07.4 seed acceptance: the three versioned operator-editable
// artifacts validate against their ratified contracts; the golden set's
// planted V0 cases actually trip the real pre-gates (a plant that doesn't
// plant proves nothing); strict-JSON loading rejects drift.

func TestSeedRubricValid(t *testing.T) {
	r := verify.SeedSoftwareRubric()
	if err := r.Validate(); err != nil {
		t.Fatalf("seed rubric invalid: %v", err)
	}
	// v2 (B4-7 rider 1): the golden-set rates are measured on the ratified
	// opus-4-8 judge (P-T06-5); content flagged for gate ratification (D1).
	if r.Domain != verify.DomainSoftware || r.Version != 2 {
		t.Fatalf("rubric identity: %+v", r)
	}
	if !r.GoldenSet.Measured || r.GoldenSet.TPR == nil || r.GoldenSet.TNR == nil {
		t.Fatal("v2 rubric must carry measured golden-set rates (rider 1 P-T06-5 run)")
	}
	if !strings.Contains(r.JudgePin, "opus-4-8") {
		t.Fatalf("judge pin must name the ratified opus-4-8 judge seat: %q", r.JudgePin)
	}
	if !strings.Contains(r.LengthBiasNote, "MEASURED") {
		t.Fatalf("length-bias note must be measured (P-T06-3): %q", r.LengthBiasNote)
	}
	// Engineering rules bite: a rubric without anchors or with a scale is
	// rejected.
	broken := verify.SeedSoftwareRubric()
	broken.Items[0].FailAnchor = ""
	if err := broken.Validate(); !errors.Is(err, verify.ErrBadSeed) {
		t.Fatalf("anchorless rubric accepted: %v", err)
	}
	broken = verify.SeedSoftwareRubric()
	broken.ExtractiveGrounding = false
	if err := broken.Validate(); !errors.Is(err, verify.ErrBadSeed) {
		t.Fatalf("rubric without extractive grounding accepted: %v", err)
	}
}

func TestSeedGoldenSetValid(t *testing.T) {
	g := verify.SeedGoldenSet()
	if err := g.Validate(); err != nil {
		t.Fatalf("seed golden set invalid: %v", err)
	}
	if n := len(g.Cases); n < 25 || n > 50 {
		t.Fatalf("golden set size %d outside the ratified 25–50", n)
	}
	classes := map[string]int{}
	for _, c := range g.Cases {
		classes[c.DefectClass]++
	}
	for _, want := range []string{"clean", "AC-BLOCKER", "SANITY-BLOCKER", "CHECK-INTEGRITY", "RESEARCH-NOT-RUN", "REOPEN-SPEC", "V0-MALFORMED"} {
		if classes[want] == 0 {
			t.Fatalf("golden set misses class %q: %v", want, classes)
		}
	}
	// Size guard is enforced.
	g.Cases = g.Cases[:10]
	if err := g.Validate(); !errors.Is(err, verify.ErrBadSeed) {
		t.Fatalf("undersized golden set accepted: %v", err)
	}
}

func TestGoldenV0PlantsActuallyTrip(t *testing.T) {
	// The planted V0 cases must trip the REAL pre-gates, and the clean
	// cases must not — planted defects falsify the checks too (P-T06-1).
	for _, c := range verify.SeedGoldenSet().Cases {
		d := verify.Deliverable{Domain: verify.DomainSoftware, Revision: 1, Content: c.Artifact}
		res := verify.RunV0(verify.DefaultPreGates(nil), d)
		switch c.DefectClass {
		case "V0-MALFORMED":
			if !res.Malformed {
				t.Fatalf("case %s: planted V0 defect did not trip the gates", c.ID)
			}
		case "clean":
			if res.Malformed {
				t.Fatalf("case %s: clean control killed by V0: %v", c.ID, res.Reasons)
			}
		}
	}
}

func TestSeedCalibrationValid(t *testing.T) {
	c := verify.SeedCalibrationSet()
	if err := c.Validate(); err != nil {
		t.Fatalf("seed calibration set invalid: %v", err)
	}
	sup, unsup := 0, 0
	for _, p := range c.Pairs {
		if p.Label == verify.EntailSupported {
			sup++
		} else {
			unsup++
		}
	}
	if sup == 0 || unsup == 0 {
		t.Fatalf("calibration set needs both labels: %d/%d", sup, unsup)
	}
	if !strings.Contains(c.Bar, "TBD-BRINGUP") {
		t.Fatalf("calibration bar must carry the TBD-BRINGUP obligation: %q", c.Bar)
	}
}

func TestSeedLoadRoundTripAndStrictness(t *testing.T) {
	dir := t.TempDir()

	// Round-trip all three.
	rp := filepath.Join(dir, "rubric.json")
	gp := filepath.Join(dir, "golden.json")
	cp := filepath.Join(dir, "calibration.json")
	if err := verify.WriteSeed(rp, verify.SeedSoftwareRubric()); err != nil {
		t.Fatalf("WriteSeed rubric: %v", err)
	}
	if err := verify.WriteSeed(gp, verify.SeedGoldenSet()); err != nil {
		t.Fatalf("WriteSeed golden: %v", err)
	}
	if err := verify.WriteSeed(cp, verify.SeedCalibrationSet()); err != nil {
		t.Fatalf("WriteSeed calibration: %v", err)
	}
	if _, err := verify.LoadRubric(rp); err != nil {
		t.Fatalf("LoadRubric: %v", err)
	}
	if _, err := verify.LoadGoldenSet(gp); err != nil {
		t.Fatalf("LoadGoldenSet: %v", err)
	}
	if _, err := verify.LoadCalibrationSet(cp); err != nil {
		t.Fatalf("LoadCalibrationSet: %v", err)
	}

	// Unknown fields fail loudly (operator typo ≠ silent deactivation).
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{"id":"x","version":1,"pairs":[],"bar":"b","typo_field":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := verify.LoadCalibrationSet(bad); !errors.Is(err, verify.ErrBadSeed) {
		t.Fatalf("unknown field accepted: %v", err)
	}
}
