package benchmark_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/benchmark"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
)

// registered_test.go is this packet's ANTI-DRIFT INSTRUMENT.
//
// BENCH-REG §17: "Silent drift between this text and the running platform is a
// defect of the platform." A test that asserted the code against itself would
// catch nothing, so every frozen value is asserted against the REGISTRATION
// FILE: the section that registers it is extracted from
// Spec/benchmark-preregistration-v1.md and the value must literally appear
// there. Edit the constant without amending the registration and the build
// fails; amend the registration under §17 and the constant must follow.

// section extracts one numbered section of the registration, from its heading
// up to the next top-level heading.
func section(t *testing.T, text, heading string) string {
	t.Helper()
	i := strings.Index(text, heading)
	if i < 0 {
		t.Fatalf("the registration has no section %q — the file this packet pins against changed shape", heading)
	}
	rest := text[i+len(heading):]
	if j := strings.Index(rest, "\n## "); j >= 0 {
		rest = rest[:j]
	}
	return rest
}

// TestRegisteredValuesAppearInTheirRegisteredSection walks every frozen value
// this package encodes and finds it in the section it cites.
func TestRegisteredValuesAppearInTheirRegisteredSection(t *testing.T) {
	text := registrationText(t)
	cases := []struct {
		heading string
		want    string
		what    string
	}{
		{"## 11.", fmt.Sprintf("m ≥ %d", benchmark.GateMinNonTiedPairs), "gate limb (a), the minimum non-tied pairs"},
		{"## 11.", fmt.Sprintf("G ≥ %.2f", benchmark.GateGFloor), "gate limb (b), the posterior floor"},
		{"## 12.", fmt.Sprintf("1 − G > %.2f", benchmark.AlarmThreshold), "the alarm threshold"},
		{"## 13.", fmt.Sprintf("≥ %d", benchmark.DoneDirectlyMinPairs), "the done-directly measured-median minimum"},
		{"## 13.", benchmark.LabelHeuristic, "the stage-1 done-directly label"},
		{"## 13.", "measured (benchmark n=", "the stage-2 done-directly label"},
		{"## 5.", "partially unblinded (guess accuracy ", "the §5 blindness label"},
		{"## 10.", benchmark.DomainSoftwareDevelopment, "the v0 registered launch domain"},
		{"## 10.", benchmark.DomainWebResearch, "the v0.1 registered launch domain"},
		{"## 1.", benchmark.RegisteredMarker, "the read-only marker every frozen value carries"},
	}
	for _, c := range cases {
		body := section(t, text, c.heading)
		if !strings.Contains(body, c.want) {
			t.Errorf("%s: %q does NOT appear in BENCH-REG %s — the platform has drifted from its own registration (§17)",
				c.what, c.want, strings.TrimSpace(c.heading))
		}
	}
}

// TestTheServedAnswerVocabulariesAreTheSetsTheMachineryValidates is the B6-6
// OQ4 pin: a decision form renders buttons from these three lists, so a list
// that offered a value the package refuses would put a dead control on screen,
// and one that omitted a value the package accepts would hide a legitimate
// answer. Both validators now READ these lists, which makes the property
// structural — this drives it through the exported behaviour anyway, so the
// structure cannot be quietly undone.
func TestTheServedAnswerVocabulariesAreTheSetsTheMachineryValidates(t *testing.T) {
	choices, sides := benchmark.VerdictChoices(), benchmark.GuessSides()
	if len(choices) == 0 || len(sides) == 0 {
		t.Fatalf("an empty vocabulary renders a form with no buttons: %v / %v", choices, sides)
	}
	for _, c := range choices {
		for _, s := range sides {
			a, err := benchmark.NewAnswer(c, s)
			if err != nil {
				t.Errorf("the served verdict %q with the served guess %q is refused by the constructor: %v", c, s, err)
				continue
			}
			if a.Choice() != c || a.Guess() != s {
				t.Errorf("the formed answer is %q/%q, not the values it was built from", a.Choice(), a.Guess())
			}
		}
	}
	// The other direction: a value the list does not carry is refused, so "the
	// list is what validates" is not vacuously true of every string.
	if _, err := benchmark.NewAnswer(benchmark.Choice("maybe"), sides[0]); err == nil {
		t.Error("a verdict outside the served vocabulary was accepted")
	}
	if _, err := benchmark.NewAnswer(choices[0], benchmark.Side("C")); err == nil {
		t.Error("a guess side outside the served vocabulary was accepted")
	}

	// The dispositions are the §12 set, and the register that displays frozen
	// values is where their registration is asserted; here they only have to be
	// the three the package declares, non-empty and distinct.
	dispositions := benchmark.AlarmDispositions()
	seen := map[string]bool{}
	for _, d := range dispositions {
		if d == "" || seen[d] {
			t.Errorf("the disposition set is %v — every entry must be a distinct, non-empty registered value", dispositions)
		}
		seen[d] = true
	}
	for _, want := range []string{
		benchmark.DispositionInvestigate, benchmark.DispositionFixAndAccrue, benchmark.DispositionReregister,
	} {
		if !seen[want] {
			t.Errorf("the served disposition set omits %q, which the alarm gate accepts", want)
		}
	}
}

// TestRegisteredValueRegisterIsComplete: every value in the S18.4 R9 display
// register carries the marker and a BENCH-REG reference, and the register names
// all four gate limbs, the alarm, the done-directly minimum and both labels —
// so nothing frozen surfaces without saying it cannot be dialled.
func TestRegisteredValueRegisterIsComplete(t *testing.T) {
	values := benchmark.RegisteredValues()
	if len(values) == 0 {
		t.Fatal("the registered-value register is empty")
	}
	seen := map[string]bool{}
	for _, v := range values {
		if v.Marker != benchmark.RegisteredMarker {
			t.Errorf("registered value %q does not carry the S18.4 R9 marker", v.Name)
		}
		if !strings.HasPrefix(v.Ref, "BENCH-REG §") {
			t.Errorf("registered value %q cites %q, not a BENCH-REG section", v.Name, v.Ref)
		}
		if v.Value == "" {
			t.Errorf("registered value %q has no value to display", v.Name)
		}
		if seen[v.Name] {
			t.Errorf("registered value %q appears twice — each frozen value surfaces exactly once", v.Name)
		}
		seen[v.Name] = true
	}
	for _, want := range []string{
		"gate limb (a) — minimum non-tied pairs",
		"gate limb (b) — posterior floor",
		"gate limb (c) — no active alarm",
		"gate limb (d) — suites green at registered floors",
		"alarm threshold",
		"done-directly measured-median minimum",
		"done-directly stage-1 label",
		"done-directly stage-2 label",
	} {
		if !seen[want] {
			t.Errorf("the registered-value register omits %q", want)
		}
	}
}

// TestHonestClaimsTableIsTheRegisteredOne asserts the §15 table ships VERBATIM:
// every question and every answer must appear in §15 of the registration.
func TestHonestClaimsTableIsTheRegisteredOne(t *testing.T) {
	body := section(t, registrationText(t), "## 15.")
	claims := benchmark.HonestClaims()
	if len(claims) != 5 {
		t.Fatalf("the §15 honest-claims table has %d rows, want the registration's 5", len(claims))
	}
	for _, c := range claims {
		if !strings.Contains(body, c.Question) {
			t.Errorf("honest-claims question %q is not in BENCH-REG §15 — the table must ship verbatim", c.Question)
		}
		if !strings.Contains(body, c.Answer) {
			t.Errorf("honest-claims answer %q is not in BENCH-REG §15 — the table must ship verbatim", c.Answer)
		}
	}
	if !strings.Contains(body, benchmark.HonestClaimsReadout) {
		t.Error("the §15 readout paragraph does not match the registration")
	}
}

// TestArmParityDeclarationIsTheRegisteredOne: §6 is declared VERBATIM, not
// paraphrased and not tightened. Every bullet of the shipped declaration must
// appear in §6 word for word once the file's markdown emphasis is stripped —
// which is the only difference the shipped strings are allowed to have.
func TestArmParityDeclarationIsTheRegisteredOne(t *testing.T) {
	body := plainText(section(t, registrationText(t), "## 6."))
	shipped := benchmark.ArmParityDeclaration
	if len(shipped) != 4 {
		t.Fatalf("the shipped §6 declaration has %d bullets, want the registration's 4", len(shipped))
	}
	for _, bullet := range shipped {
		if !strings.Contains(body, plainText(bullet)) {
			t.Errorf("this §6 bullet is not in the registration VERBATIM — never \"improve\" a registered label (BENCH-REG §17):\n  shipped: %s", bullet)
		}
	}
	// The two parentheticals a paraphrase would be tempted to drop: an example
	// makes the tool clause concrete, and the citation records that the absence
	// of prior art is a FINDING rather than an absence of looking.
	for _, clause := range []string{"(e.g., its own web search)", "(report 12 §2.6, negative finding)"} {
		if !strings.Contains(body, clause) {
			t.Errorf("BENCH-REG §6 no longer contains %q — the shipped declaration is pinned to it", clause)
		}
		if !strings.Contains(strings.Join(shipped, " "), clause) {
			t.Errorf("the shipped §6 declaration elides %q", clause)
		}
	}
}

// plainText strips the registration's markdown emphasis so a shipped string can
// be compared to the file's prose. It touches nothing else: the comparison is
// still word for word.
func plainText(s string) string {
	return strings.NewReplacer("**", "", "*", "").Replace(s)
}

// TestDoneDirectlyLabelsAgreeAcrossThePackages: the §13.1 heuristic label exists
// in TWO places — internal/metering (stage 1, on every receipt) and this package
// (stage 2's companion). They are duplicated across the import wall by design,
// so this asserts they have not diverged from each other OR from §13.
func TestDoneDirectlyLabelsAgreeAcrossThePackages(t *testing.T) {
	if benchmark.LabelHeuristic != metering.DirectUseLabel {
		t.Errorf("the §13.1 label differs between packages: benchmark %q vs metering %q",
			benchmark.LabelHeuristic, metering.DirectUseLabel)
	}
	if !strings.Contains(metering.DirectUseFormulaRef, "benchmark-preregistration-v1.md") {
		t.Errorf("metering's formula ref %q no longer points at the registration", metering.DirectUseFormulaRef)
	}
	if got := benchmark.MeasuredLabel(23); got != "measured (benchmark n=23)" {
		t.Errorf("MeasuredLabel(23) = %q, want the §13.2 form", got)
	}
}

// TestDomainMappingIsPinnedBothWays: the ONE translation from the tree's
// internal domain vocabulary to the registered names, asserted in both
// directions, plus the refusal of anything unregistered. The pair record always
// carries the REGISTERED name.
func TestDomainMappingIsPinnedBothWays(t *testing.T) {
	if d, ok := benchmark.RegisteredDomain("software"); !ok || d != benchmark.DomainSoftwareDevelopment {
		t.Errorf(`RegisteredDomain("software") = %q,%v; want %q,true (the tree's verify.DomainSoftware)`,
			d, ok, benchmark.DomainSoftwareDevelopment)
	}
	if d, ok := benchmark.RegisteredDomain("web-research"); !ok || d != benchmark.DomainWebResearch {
		t.Errorf(`RegisteredDomain("web-research") = %q,%v; want %q,true`, d, ok, benchmark.DomainWebResearch)
	}
	if _, ok := benchmark.RegisteredDomain("software-development"); ok {
		t.Error("the mapping's input side is the CODE vocabulary; the registered name is its output, never its input")
	}
	if _, ok := benchmark.RegisteredDomain("content"); ok {
		t.Error("an unregistered domain must not map — BENCH-REG §10 registers exactly two")
	}
	if !benchmark.IsRegisteredDomain(benchmark.DomainSoftwareDevelopment) ||
		!benchmark.IsRegisteredDomain(benchmark.DomainWebResearch) {
		t.Error("both §10 registrations must be recognized")
	}
	if benchmark.IsRegisteredDomain("software") {
		t.Error("the code name is not a registered domain name")
	}
	// v0's accruing roster is software-development alone: web-research accrues
	// from v0.1 ship, so the 15.3 gate does not wait on it at v0.
	roster := benchmark.V0LaunchDomains()
	if len(roster) != 1 || roster[0] != benchmark.DomainSoftwareDevelopment {
		t.Errorf("the v0 launch roster is %v, want [%s]", roster, benchmark.DomainSoftwareDevelopment)
	}
}

// TestNoRegisteredNumberIsASettingsKey (S18.4 R9): "only
// benchmark.sampling_rate is a live dial". The package consumes exactly one ⚙
// key and declares none, so no frozen value can be reached through the settings
// registry.
func TestNoRegisteredNumberIsASettingsKey(t *testing.T) {
	src := readPackageSource(t)
	if !strings.Contains(src, `"benchmark.sampling_rate"`) {
		t.Error("the package must consume benchmark.sampling_rate by dotted key")
	}
	// No frozen value may be spelled as a dotted ⚙ key anywhere in the package:
	// a key is how a number becomes dialable, and these must not be.
	for _, forbidden := range []string{
		"benchmark.gate_", "benchmark.alarm_threshold", "benchmark.min_pairs",
		"benchmark.g_floor", "benchmark.done_directly", "benchmark.labels",
	} {
		if strings.Contains(src, forbidden) {
			t.Errorf("the package names %q — no registered number is a ⚙ row and this package declares no key (S18.4 R9)", forbidden)
		}
	}
	// The one consumed key is read through the package's own narrow interface,
	// so internal/settings is never imported (asserted structurally by the
	// import wall; asserted here as the reason).
	if strings.Contains(src, `internal/settings"`) {
		t.Error("⚙ values ride the package's own Settings interface, never an internal/settings import")
	}
}
