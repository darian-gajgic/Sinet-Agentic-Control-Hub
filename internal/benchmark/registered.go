package benchmark

import "fmt"

// registered.go holds every BENCH-REG value this package must encode, as MARKED
// READ-ONLY DATA carrying its section reference.
//
// The rule these constants exist to obey (Spec S18.4 R9, verbatim): "Benchmark
// frozen values (gate limbs, alarm threshold, done-directly minimum, labels) —
// surface in the registry marked 'registered — changing this value requires a
// re-registration commit' [BENCH-REG §1; S14]; only benchmark.sampling_rate is a
// live dial."
//
// So: none of these is a ⚙ settings row, none is clamped, none is re-derived
// from anything, and none may be edited here. Changing any of them is a dated
// amendment to Spec/benchmark-preregistration-v1.md under its own §17,
// committed with the registered signing key — an operator act, never a packet's.
// registered_test.go pins each value against the registration FILE, so drift
// between that text and this code fails the build, which is exactly what §17's
// "silent drift between this text and the running platform is a defect of the
// platform" asks for.

// RegisteredMarker is the S18.4 R9 / BENCH-REG §1 marker every frozen value
// carries wherever it surfaces.
const RegisteredMarker = "registered — changing this value requires a re-registration commit"

// RegistrationRef names the registration itself, for display beside the marker.
const RegistrationRef = "Spec/benchmark-preregistration-v1.md (BENCH-REG v1, signed 2026-07-18)"

// ── The gate (BENCH-REG §11, frozen) ─────────────────────────────────────────

// GateMinNonTiedPairs is limb (a): m ≥ 20 non-tied pairs in the current epoch.
const GateMinNonTiedPairs = 20

// GateGFloor is limb (b): G ≥ 0.90.
const GateGFloor = 0.90

// ── The alarm (BENCH-REG §12, frozen) ────────────────────────────────────────

// AlarmThreshold is the §12 trigger: at any per-pair update, 1 − G > 0.95 in a
// launch domain's current epoch. The comparison is STRICTLY greater, exactly as
// registered.
const AlarmThreshold = 0.95

// ── Done directly (BENCH-REG §13, frozen text) ───────────────────────────────

// DoneDirectlyMinPairs is §13.2's registered minimum: the aggregate switches to
// the measured median once a domain has accrued ≥ 10 measured benchmark pairs.
const DoneDirectlyMinPairs = 10

// LabelHeuristic is §13.1's stage-1 label, verbatim. It is the same string
// internal/metering exports as DirectUseLabel — deliberately duplicated across
// the wall rather than imported, because both sites cite the registration
// directly and neither owns the other's copy (the deadManCanaryPrefix
// duplication precedent, CONVENTIONS §31). registered_test.go pins both against
// the registration file.
const LabelHeuristic = "direct-use estimate (heuristic)"

// labelMeasuredFmt renders §13.2's stage-2 label. The registration writes it
// "measured (benchmark n=…)"; n is the count, so the template is the frozen
// form and MeasuredLabel is the only way this package produces it.
const labelMeasuredFmt = "measured (benchmark n=%d)"

// MeasuredLabel renders the §13.2 measured-median label verbatim for n pairs.
func MeasuredLabel(n int) string { return fmt.Sprintf(labelMeasuredFmt, n) }

// ── Blindness (BENCH-REG §5, frozen) ─────────────────────────────────────────

// labelPartiallyUnblindedFmt renders §5's label for a rate published when guess
// accuracy beats chance. No result is ever suppressed for blindness failure; it
// is LABELLED.
const labelPartiallyUnblindedFmt = "partially unblinded (guess accuracy %.1f%%)"

// PartiallyUnblindedLabel renders §5's label at the measured guess accuracy.
// One decimal: a 20-vote sample moves in 5.0% steps, so more digits would
// pretend to precision the n does not have.
func PartiallyUnblindedLabel(accuracy float64) string {
	return fmt.Sprintf(labelPartiallyUnblindedFmt, accuracy*100)
}

// ChanceAccuracy is the coin-flip baseline a guess accuracy is compared against
// (§5: "If guess accuracy beats chance"). It is the definition of a two-sided
// blind guess, not a tunable.
const ChanceAccuracy = 0.5

// ── Domains (BENCH-REG §10, frozen) ──────────────────────────────────────────

// DomainSoftwareDevelopment is the v0 registered launch domain (§10.1). This is
// the REGISTERED name and the one that goes on every record; the tree's internal
// verification-domain constant is "software" (internal/verify), and codeDomains
// below is the single pinned translation between them.
const DomainSoftwareDevelopment = "software-development"

// DomainWebResearch is registered at §10.2 and accrues from v0.1 (Spec S19). It
// is NOT in the v0 launch roster, so the 15.3 gate does not wait on it at v0.
const DomainWebResearch = "web-research"

// v0LaunchDomains is the roster the 15.3 gate evaluates at v0: "The 15.3 gate
// opens when every registered launch domain passes" (§11), and at v0 the
// accruing roster is software-development alone — web-research accrues from
// v0.1 ship (§10, "Accrual for web-research starts at v0.1 ship").
var v0LaunchDomains = []string{DomainSoftwareDevelopment}

// V0LaunchDomains returns the v0 accruing launch-domain roster.
func V0LaunchDomains() []string {
	out := make([]string, len(v0LaunchDomains))
	copy(out, v0LaunchDomains)
	return out
}

// codeDomains is the ONE pinned translation from the tree's internal
// verification-domain vocabulary (internal/verify: DomainSoftware = "software",
// DomainWebResearch = "web-research") to the BENCH-REG §10 REGISTERED names.
//
// It is a READING, recorded here, not a spec edit: the frozen text decides what
// a record says, and every pair record carries the registered name. The map is
// duplicated across the import wall rather than importing internal/verify (which
// drags intake → adapters into an observability leaf, CONVENTIONS §33); a test
// pins both sides.
var codeDomains = map[string]string{
	"software":     DomainSoftwareDevelopment,
	"web-research": DomainWebResearch,
}

// RegisteredDomain maps an internal code-domain name to its BENCH-REG §10
// registered name, reporting ok=false for anything unregistered.
func RegisteredDomain(codeDomain string) (string, bool) {
	d, ok := codeDomains[codeDomain]
	return d, ok
}

// IsRegisteredDomain reports whether a registered domain name is one BENCH-REG
// §10 declares.
func IsRegisteredDomain(domain string) bool {
	return domain == DomainSoftwareDevelopment || domain == DomainWebResearch
}

// ── The §15 honest-claims table (frozen obligation) ──────────────────────────

// HonestClaim is one row of the BENCH-REG §15 reference table — the small-n
// humility surface that ships wherever a win rate is shown ("wherever a win
// rate is shown, the UI also shows what the current n could even detect").
type HonestClaim struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// honestClaims is the §15 table VERBATIM. It is reference data reproduced from
// the registration, not computed here: computing it would be re-deriving a
// registered number.
var honestClaims = []HonestClaim{
	{"n for 80% power at true 60/40 · 65/35 · 70/30 · 75/25 · 80/20", "158 · 69 · 37 · 23 · 18"},
	{"n=20: fixed-test rejection needs", "≥15/20 (75%)"},
	{"n=20 power vs true 65/35 · 75/25 · 85/15", "0.25 · 0.62 · 0.93"},
	{"Wilson 95% CI at 14/20", "[0.48, 0.85] — includes 50%"},
	{"Beta-Binomial P(p>0.5) at 7/10 · 14/20 · 20/30 · 35/50", "0.89 · 0.96 · 0.97 · 0.998"},
}

// HonestClaimsReadout is §15's closing paragraph, verbatim — the sentence that
// makes the table legible to a household reader.
const HonestClaimsReadout = "a household quarter (n≈20–30) reliably detects only large effects (≥75/25); " +
	"a month detects only catastrophic ones (≥85/15). The predecessor's failure was catastrophic-scale — " +
	"detectable at n≈15–20."

// HonestClaims returns the §15 table. It rides on every published rate (R19), so
// wherever B6 renders a number the table renders with it — there is no benchmark
// number without its epistemics.
func HonestClaims() []HonestClaim {
	out := make([]HonestClaim, len(honestClaims))
	copy(out, honestClaims)
	return out
}

// ── The §6 arm-parity declaration (frozen) ───────────────────────────────────

// ArmParityDeclaration is BENCH-REG §6's declaration, carried on every readout:
// declared, not hidden. A parity gap beyond it is new registration material and
// is flagged to the operator, never silently absorbed.
var ArmParityDeclaration = []string{
	"The platform arm carries household memory, knowledge injection, multi-turn pipeline, tool use, " +
		"and verification loops. That asymmetry is the treatment under test, not a confound.",
	"The direct arm runs on a consumer surface whose model identity cannot be pinned or controlled; " +
		"it is recorded as observed per pair. The direct arm has no household memory — arguably the thing being tested.",
	"Both arms answer the identical frozen task statement with identical attachments; the direct arm's " +
		"surface-native tools are allowed as-is — the baseline is \"what the requester's subscription actually does\", " +
		"not a stripped model.",
	"No published prior art exists for \"platform vs the user's own subscription surface\" — parity gaps beyond " +
		"this declaration are treated as new registration material, never silently absorbed.",
}

// ── The registered-value register (S18.4 R9 surface) ─────────────────────────

// RegisteredValue is one frozen BENCH-REG value as it surfaces for display: the
// value, the section that registers it, and the marker that says it cannot be
// dialled. This is the S18.4 R9 "surface in the registry marked …" obligation
// discharged on the benchmark side — the values are NOT settings rows.
type RegisteredValue struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Ref    string `json:"ref"`
	Marker string `json:"marker"`
}

// RegisteredValues returns every frozen value this package encodes, each with
// its BENCH-REG §-ref and the read-only marker, for the readout to display.
// Each value appears here exactly once and is read from the same constant the
// machinery uses, so a display can never disagree with a decision.
func RegisteredValues() []RegisteredValue {
	v := func(name, value, ref string) RegisteredValue {
		return RegisteredValue{Name: name, Value: value, Ref: ref, Marker: RegisteredMarker}
	}
	return []RegisteredValue{
		v("gate limb (a) — minimum non-tied pairs", fmt.Sprintf("m ≥ %d", GateMinNonTiedPairs), "BENCH-REG §11(a)"),
		v("gate limb (b) — posterior floor", fmt.Sprintf("G ≥ %.2f", GateGFloor), "BENCH-REG §11(b)"),
		v("gate limb (c) — no active alarm", "no alarm standing in the domain", "BENCH-REG §11(c)"),
		v("gate limb (d) — suites green at registered floors", "regression suites green at their registered per-version floors", "BENCH-REG §11(d), §10.1(d)"),
		v("alarm threshold", fmt.Sprintf("1 − G > %.2f", AlarmThreshold), "BENCH-REG §12"),
		v("done-directly measured-median minimum", fmt.Sprintf("≥ %d measured pairs", DoneDirectlyMinPairs), "BENCH-REG §13.2"),
		v("done-directly stage-1 label", LabelHeuristic, "BENCH-REG §13.1"),
		v("done-directly stage-2 label", MeasuredLabel(0), "BENCH-REG §13.2"),
		v("partially-unblinded label", PartiallyUnblindedLabel(0), "BENCH-REG §5"),
		v("registered launch domains", DomainSoftwareDevelopment+" (v0) · "+DomainWebResearch+" (v0.1)", "BENCH-REG §10"),
		v("decision statistic", "G := P(p > 0.5 | w, m), p ~ Beta(1+w, 1+(m−w)); the ONLY decision input", "BENCH-REG §7"),
		v("tie handling", "tie and both-bad are excluded from m and w and never enter the posterior", "BENCH-REG §8"),
		v("epoch rule", "an epoch ends when the direct arm's observed model identity changes; no decision statistic pools across epochs", "BENCH-REG §9"),
	}
}
