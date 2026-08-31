package metering

// gf14red_test.go — P3-GF14 acceptance test T6, committed RED with the
// grounding brief (P3/briefs/P3-GF14.md §7; Amendment-A carve-out, CONVENTIONS
// §3). Window closes at the P3-GF14 implementation commit.
//
// The defect (GF9 review M8, e5rw2-F2 confirmed): the receipts drawer ships
// the citation soup verbatim to a requester — "no price table loaded at v0
// (UNPRICED, never a silent $0 — S10.1)" and "aggregate switches to the
// measured-median stage per Spec/benchmark-preregistration-v1.md §13 …".
// Requester-facing prose is for the person (CONVENTIONS §59; P3-GF13); the
// GF13 keep ledger holds NO row for these strings, so they go plain-words.
// Two members are pinned the OTHER way: the ratified label (Spec S15.5 /
// G2 D2.8) and the machine formula ref (registered provenance, a keep row for
// it lands with this packet).

import (
	"regexp"
	"strings"
	"testing"
)

// citationShapes are the GF13 widened-scan token forms a served sentence may
// never carry (P3-GF13-keep-ledger.md, deferred item 3): spec-section tokens,
// build-phase tokens, repo paths, and the section sign.
var citationShapes = []*regexp.Regexp{
	regexp.MustCompile(`\bS[0-9]{1,2}\.[0-9]`), // S10.1, S14.7 …
	regexp.MustCompile(`\bB[0-9]\b`),           // B5 …
	regexp.MustCompile(`\bP47\b`),
	regexp.MustCompile(`Spec/`),
	regexp.MustCompile(`§`),
}

// TestGF14ReceiptProseCarriesNoCitations is R5: the done-directly line's
// requester-facing sentences speak plain words, while the ratified label and
// the machine provenance member stay byte-exact.
func TestGF14ReceiptProseCarriesNoCitations(t *testing.T) {
	est := directUse(RunConsumption{}) // no items → the honest UNPRICED posture

	if est.Label != "direct-use estimate (heuristic)" {
		t.Fatalf("the ratified label (Spec S15.5, G2 D2.8) moved: %q", est.Label)
	}
	if DirectUseLabel != est.Label {
		t.Fatalf("DirectUseLabel and the served label diverged: %q vs %q", DirectUseLabel, est.Label)
	}
	if est.FormulaRef != "Spec/benchmark-preregistration-v1.md §13" {
		t.Fatalf("FormulaRef is machine provenance for the registered formula and stays byte-exact "+
			"(GF13 keep row, brief R5.3): %q", est.FormulaRef)
	}
	if !est.Unpriced {
		t.Fatal("an empty consumption must render UNPRICED (S10.1)")
	}
	if strings.TrimSpace(est.Reason) == "" {
		t.Fatal("an absence must still be explained, in plain words (S10.1: never a silent $0)")
	}

	for name, prose := range map[string]string{
		"Reason":            est.Reason,
		"MeasuredStageSeam": est.MeasuredStageSeam,
	} {
		for _, re := range citationShapes {
			if hit := re.FindString(prose); hit != "" {
				t.Errorf("R5: DirectUseEstimate.%s serves %q to a requester — citation token %q; "+
					"plain words only (GF9 review M8; CONVENTIONS §59; GF13 — no keep row exists): %q",
					name, hit, hit, prose)
			}
		}
	}
	// The registered threshold is BENCH-REG's alone (its §17 discipline): no
	// digit-bearing restatement of the pair threshold may appear in prose.
	if regexp.MustCompile(`[0-9]+ +(measured )?pairs`).MatchString(est.MeasuredStageSeam) {
		t.Errorf("the measured-stage sentence restates a registered number: %q", est.MeasuredStageSeam)
	}
}
