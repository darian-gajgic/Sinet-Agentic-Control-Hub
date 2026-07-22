package verify

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Seed artifacts (Spec S07.4/S07.10, G3 Def.8 seeding discipline): the
// software rubric bundle, the software golden set (planted-defect cases),
// and the entailment calibration set (planted supported/unsupported pairs).
// All three ship in code — versioned, attributable, operator-editable via
// strict-JSON Load overrides (the intake taxonomy precedent, CONVENTIONS
// §14) — and formally enter the 8.3 knowledge gate when Spec S09 lands
// (B3). SEED CONTENT RATIFICATION IS A B2-GATE ITEM; the TPR/TNR
// measurements over these seeds need the B4 local tier — the deferral
// record lives in P3/measurements/ per G3 Def.8.

// ---- Rubric bundle (Spec S07.10) ----

// RubricItem is one axis-2 mini-rubric criterion. The engineering rules are
// structural: BINARY verdict per criterion (pass/fail anchors, no scales),
// ONE construct per criterion, behavioral anchors per level.
type RubricItem struct {
	// ID is the citable criterion id ("axis2/side-effects") — the id a
	// blocker cites when it violates a rubric item rather than an AC.
	ID string `json:"id"`
	// Probe binds the item to its ratified axis-2 probe (Spec S07.5).
	Probe Probe `json:"probe"`
	// Construct is the ONE thing this criterion measures.
	Construct string `json:"construct"`
	// PassAnchor/FailAnchor are the behavioral anchors: what a pass looks
	// like, what a fail looks like — a verdict without its anchor behavior
	// is unusable (Spec S07.10).
	PassAnchor string `json:"pass_anchor"`
	FailAnchor string `json:"fail_anchor"`
}

// RubricBundle is one immutable versioned rubric (Spec S07.10): a named,
// versioned, falsifiable object — imported prefab scores are banned.
type RubricBundle struct {
	ID      string `json:"id"`
	Domain  string `json:"domain"`
	Version int    `json:"version"`
	// VerifiedOn is the P-T06-1 stamp (last planted-defect audit date,
	// ISO date).
	VerifiedOn string `json:"verified_on"`
	// JudgePin pins the judge for this rubric version (P-T06-5): any
	// judge-model change gates on a golden-set re-run before unsupervised
	// judging resumes. At seed time the pin names the ratified CLASS; the
	// concrete engine pin lands with S08 judge selection (B3) and bumps
	// the bundle version.
	JudgePin string `json:"judge_pin"`
	// Axis1Protocol documents the compliance-pass protocol the bundle
	// rides: binary verdict per numbered AC, mandatory extractive evidence
	// quote, Unknown escape (Spec S07.5).
	Axis1Protocol string `json:"axis1_protocol"`
	// Items is the axis-2 mini-rubric (the four probes).
	Items []RubricItem `json:"items"`
	// ExtractiveGrounding attests the extractive-quote rule: a high score
	// is impossible without evidence (Spec S07.10). Validation requires it.
	ExtractiveGrounding bool `json:"extractive_grounding"`
	// LengthBiasNote records the per-judge-model length/style bias
	// (P-T06-3), re-measured on every judge change.
	LengthBiasNote string `json:"length_bias_note"`
	// GoldenSet carries the judge's current golden-set error rates against
	// this rubric (honest "unmeasured" until the B4 run).
	GoldenSet GoldenSetRates `json:"golden_set"`
}

// Validate enforces the S07.10 rubric engineering rules.
func (r *RubricBundle) Validate() error {
	if r.ID == "" || r.Domain == "" || r.Version < 1 {
		return fmt.Errorf("%w: rubric requires id, domain, version", ErrBadSeed)
	}
	if r.VerifiedOn == "" {
		return fmt.Errorf("%w: rubric %q without verified-on stamp (P-T06-1)", ErrBadSeed, r.ID)
	}
	if r.JudgePin == "" {
		return fmt.Errorf("%w: rubric %q pins no judge (P-T06-5)", ErrBadSeed, r.ID)
	}
	if !r.ExtractiveGrounding {
		return fmt.Errorf("%w: rubric %q without extractive-quote grounding (Spec S07.10)", ErrBadSeed, r.ID)
	}
	if len(r.Items) == 0 {
		return fmt.Errorf("%w: rubric %q without items", ErrBadSeed, r.ID)
	}
	probes := map[Probe]bool{}
	seen := map[string]bool{}
	for _, it := range r.Items {
		if it.ID == "" || it.Construct == "" {
			return fmt.Errorf("%w: rubric item requires id and ONE construct (Spec S07.10)", ErrBadSeed)
		}
		if seen[it.ID] {
			return fmt.Errorf("%w: duplicate rubric item %q", ErrBadSeed, it.ID)
		}
		seen[it.ID] = true
		if it.PassAnchor == "" || it.FailAnchor == "" {
			return fmt.Errorf("%w: rubric item %q without behavioral anchors per level (Spec S07.10)", ErrBadSeed, it.ID)
		}
		probes[it.Probe] = true
	}
	for _, p := range Probes {
		if !probes[p] {
			return fmt.Errorf("%w: rubric %q misses the ratified probe %q (Spec S07.5)", ErrBadSeed, r.ID, p)
		}
	}
	return nil
}

// SeedSoftwareRubric returns the v1 software rubric bundle (TBD-P3 seed,
// Spec S07.10 "seed rubric drafting session per launch domain" — drafted at
// implementation time, ratification queued for the B2 gate).
func SeedSoftwareRubric() *RubricBundle {
	tpr, tnr := 1.0, 0.5 // measured on opus-4-8, 2026-07-22 (rider 1, P-T06-5)
	return &RubricBundle{
		ID:         "rubric-software",
		Domain:     DomainSoftware,
		Version:    2, // v2 bump (B4-7 rider 1): golden-set rates measured on the ratified opus-4-8 judge; content FLAGGED for gate ratification (S00.9), like the v1 seeds
		VerifiedOn: "2026-07-22",
		JudgePin: "claude-opus-4-8 (the D3-ratified judge seat, applied e06f0a4). The P-T06-5 golden-set re-run ran on it " +
			"2026-07-22 (rider 1) before unsupervised judging resumes; any future judge change re-gates on a fresh re-run.",
		Axis1Protocol: "One BINARY verdict per numbered frozen AC, binding to the structured sub-line where one exists " +
			"(G1 P10); a mandatory extractive evidence quote from the artifact for every PASS; an Unknown escape. " +
			"Sub-lines executed at V1 are consumed as evidence, never re-decided (Spec S07.5).",
		Items: []RubricItem{
			{
				ID: "axis2/reasonable-user", Probe: ProbeReasonableUser,
				Construct:  "Would a reasonable user consider this done and good?",
				PassAnchor: "A user receiving exactly this output would use it as-is: it does the asked thing without surprises, cleanup work, or apology-notes.",
				FailAnchor: "A user would come back within minutes — output needs manual fixing, misses the obvious point of the request, or works only in the demo path.",
			},
			{
				ID: "axis2/implicit-expectations", Probe: ProbeImplicitExpectations,
				Construct:  "What would a well-informed person expect that is absent?",
				PassAnchor: "The unstated-but-expected essentials for this deliverable type are present (errors handled where failure is likely, inputs validated at trust boundaries, docs/tests where the domain convention demands them).",
				FailAnchor: "A domain-obvious expectation is missing — e.g. code that cannot fail gracefully, a migration without rollback notes, output that silently drops edge cases.",
			},
			{
				ID: "axis2/side-effects", Probe: ProbeSideEffects,
				Construct:  "Unrequested changes are failures, not bonuses.",
				PassAnchor: "Every change traces to the SPEC or a numbered finding; nothing else was touched.",
				FailAnchor: "Files, behavior, dependencies, or config changed beyond the request — including 'helpful' refactors, formatting sweeps, or added packages nobody asked for.",
			},
			{
				ID: "axis2/expert-standard", Probe: ProbeExpertStandard,
				Construct:  "Does the work meet what a domain expert would call competent?",
				PassAnchor: "An experienced practitioner would sign off: sound approach, no known-bad patterns (injection-prone string building, secrets in code, O(n²) on unbounded input without need).",
				FailAnchor: "An expert would flag it on first read: a known anti-pattern, a security hole, or a fragile construction a professional would not ship.",
			},
		},
		ExtractiveGrounding: true,
		LengthBiasNote: "MEASURED on opus-4-8 2026-07-22 (rider 1, P-T06-3): point-biserial r = -0.167 of artifact length vs the " +
			"judge's flag decision over the 26-case golden set — WEAK, slightly negative (nowhere near the 0.10–0.76 style-bias " +
			"warning). Re-measure on every judge change.",
		GoldenSet: GoldenSetRates{TPR: &tpr, TNR: &tnr, Measured: true, MeasuredOn: "2026-07-22"},
	}
}

// ---- Golden set (Spec S07.10) ----

// GoldenCase is one human-labeled case: a planted defect (or a clean
// control) with its expected classification. The judge is validated as a
// classifier (TPR/TNR) against the set; clean controls make TNR measurable.
type GoldenCase struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// DefectClass is the planted class: a route-table category,
	// "V0-MALFORMED" (validates the pre-gates — planted defects falsify
	// checks too, P-T06-1), or "clean".
	DefectClass string `json:"defect_class"`
	// ACs are the case's frozen criteria (plain phrasing).
	ACs []string `json:"acs"`
	// Artifact is the deliverable under judgment.
	Artifact string `json:"artifact"`
	// Expected is the labeled outcome: a Verdict, or "MALFORMED" for
	// V0-class cases.
	Expected string `json:"expected"`
	// ExpectedFinding names the criterion/rubric item a correct judge
	// cites ("" on clean cases).
	ExpectedFinding string `json:"expected_finding,omitempty"`
}

// GoldenSet is the per-domain labeled case set: 25–50 cases, refreshed from
// real incidents (Spec S07.10).
type GoldenSet struct {
	ID      string       `json:"id"`
	Domain  string       `json:"domain"`
	Version int          `json:"version"`
	Cases   []GoldenCase `json:"cases"`
	// Provenance records where the cases came from (seed vs incident
	// refresh).
	Provenance string `json:"provenance"`
}

// validExpected is the labeled-outcome vocabulary.
var validExpected = map[string]bool{
	string(VerdictShip): true, string(VerdictShipWithNotes): true,
	string(VerdictRevise): true, string(VerdictEscalate): true,
	string(VerdictReopenSpec): true, "MALFORMED": true,
}

// Validate enforces the golden-set contract: the ratified 25–50 size, both
// defective and clean cases (TPR AND TNR must be measurable), unique ids.
func (g *GoldenSet) Validate() error {
	if g.ID == "" || g.Domain == "" || g.Version < 1 {
		return fmt.Errorf("%w: golden set requires id, domain, version", ErrBadSeed)
	}
	if n := len(g.Cases); n < 25 || n > 50 {
		return fmt.Errorf("%w: golden set has %d cases; the ratified size is 25–50 (Spec S07.10)", ErrBadSeed, n)
	}
	seen := map[string]bool{}
	defective, clean := 0, 0
	for _, c := range g.Cases {
		if c.ID == "" || c.Title == "" || c.Artifact == "" || len(c.ACs) == 0 {
			return fmt.Errorf("%w: golden case %q incomplete", ErrBadSeed, c.ID)
		}
		if seen[c.ID] {
			return fmt.Errorf("%w: duplicate golden case %q", ErrBadSeed, c.ID)
		}
		seen[c.ID] = true
		if !validExpected[c.Expected] {
			return fmt.Errorf("%w: golden case %q expected %q", ErrBadSeed, c.ID, c.Expected)
		}
		if c.DefectClass == "clean" {
			clean++
		} else {
			defective++
			if c.ExpectedFinding == "" {
				return fmt.Errorf("%w: defective case %q names no expected finding", ErrBadSeed, c.ID)
			}
		}
	}
	if defective == 0 || clean == 0 {
		return fmt.Errorf("%w: golden set needs BOTH defective and clean cases (TPR and TNR, Spec S07.10)", ErrBadSeed)
	}
	return nil
}

// SeedGoldenSet returns the v1 software golden set: 26 planted cases (20
// defective across the route-table classes + 6 clean controls),
// TBD-BRINGUP(golden-set seed per launch domain) — refreshed from real
// incidents as they accrue; content ratification is a B2-gate item.
func SeedGoldenSet() *GoldenSet {
	acSum := []string{"AC-1: sum(a, b) returns the arithmetic sum of its two integer arguments"}
	acCLI := []string{"AC-1: the CLI exits 0 on success and 1 on any error", "AC-2: --json prints machine-readable output"}
	acParse := []string{"AC-1: parseAge rejects non-numeric input with a clear error"}
	return &GoldenSet{
		ID: "golden-software", Domain: DomainSoftware, Version: 1,
		Provenance: "P3-B2-3 seed (planted defects; Spec S07.10). Refresh from real incidents; ratification = B2 gate.",
		Cases: []GoldenCase{
			// ---- Clean controls (TNR) ----
			{ID: "g-01", Title: "correct sum", DefectClass: "clean", ACs: acSum,
				Artifact: "func Sum(a, b int) int { return a + b }",
				Expected: string(VerdictShip)},
			{ID: "g-02", Title: "correct validation", DefectClass: "clean", ACs: acParse,
				Artifact: "func ParseAge(s string) (int, error) {\n\tn, err := strconv.Atoi(s)\n\tif err != nil {\n\t\treturn 0, fmt.Errorf(\"age must be a number, got %q\", s)\n\t}\n\treturn n, nil\n}",
				Expected: string(VerdictShip)},
			{ID: "g-03", Title: "correct CLI exit paths", DefectClass: "clean", ACs: acCLI,
				Artifact: "func main() {\n\tif err := run(); err != nil {\n\t\tfmt.Fprintln(os.Stderr, err)\n\t\tos.Exit(1)\n\t}\n}",
				Expected: string(VerdictShip)},
			{ID: "g-04", Title: "correct dedup helper", DefectClass: "clean",
				ACs:      []string{"AC-1: Dedup returns the input order-preserving with duplicates removed"},
				Artifact: "func Dedup(in []string) []string {\n\tseen := map[string]bool{}\n\tvar out []string\n\tfor _, s := range in {\n\t\tif !seen[s] {\n\t\t\tseen[s] = true\n\t\t\tout = append(out, s)\n\t\t}\n\t}\n\treturn out\n}",
				Expected: string(VerdictShip)},
			{ID: "g-05", Title: "correct config default", DefectClass: "clean",
				ACs:      []string{"AC-1: missing config file falls back to defaults without error"},
				Artifact: "func Load(path string) Config {\n\traw, err := os.ReadFile(path)\n\tif err != nil {\n\t\treturn Defaults()\n\t}\n\treturn parse(raw)\n}",
				Expected: string(VerdictShip)},
			{ID: "g-06", Title: "correct retry with cap", DefectClass: "clean",
				ACs:      []string{"AC-1: fetch retries at most 3 times with backoff"},
				Artifact: "for attempt := 0; attempt < 3; attempt++ {\n\tif res, err = fetch(url); err == nil {\n\t\tbreak\n\t}\n\ttime.Sleep(time.Second << attempt)\n}",
				Expected: string(VerdictShip)},

			// ---- AC blockers (axis 1) ----
			{ID: "g-07", Title: "wrong operation", DefectClass: "AC-BLOCKER", ACs: acSum,
				Artifact: "func Sum(a, b int) int { return a * b }",
				Expected: string(VerdictRevise), ExpectedFinding: "AC-1"},
			{ID: "g-08", Title: "missing --json mode", DefectClass: "AC-BLOCKER", ACs: acCLI,
				Artifact: "func main() {\n\tif err := run(); err != nil {\n\t\tos.Exit(1)\n\t}\n\t// human-readable output only\n\tfmt.Println(report())\n}",
				Expected: string(VerdictRevise), ExpectedFinding: "AC-2"},
			{ID: "g-09", Title: "always exits 0", DefectClass: "AC-BLOCKER", ACs: acCLI,
				Artifact: "func main() {\n\t_ = run() // errors ignored\n\tfmt.Println(\"done\")\n}",
				Expected: string(VerdictRevise), ExpectedFinding: "AC-1"},
			{ID: "g-10", Title: "accepts garbage age", DefectClass: "AC-BLOCKER", ACs: acParse,
				Artifact: "func ParseAge(s string) (int, error) {\n\tn, _ := strconv.Atoi(s)\n\treturn n, nil\n}",
				Expected: string(VerdictRevise), ExpectedFinding: "AC-1"},
			{ID: "g-11", Title: "off-by-one pagination", DefectClass: "AC-BLOCKER",
				ACs:      []string{"AC-1: Page(n) returns items 25n..25n+24"},
				Artifact: "func Page(n int) []Item { return items[25*n+1 : 25*n+26] }",
				Expected: string(VerdictRevise), ExpectedFinding: "AC-1"},
			{ID: "g-12", Title: "partial AC: sum works, overflow silently wraps despite AC naming it", DefectClass: "AC-BLOCKER",
				ACs:      []string{"AC-1: sum(a, b) returns the sum or an error on int64 overflow"},
				Artifact: "func Sum(a, b int64) (int64, error) { return a + b, nil }",
				Expected: string(VerdictRevise), ExpectedFinding: "AC-1"},

			// ---- Sanity blockers (axis 2) ----
			{ID: "g-13", Title: "compliant but destroys data on error", DefectClass: "SANITY-BLOCKER",
				ACs:      []string{"AC-1: import loads the CSV into the table"},
				Artifact: "func Import(path string) error {\n\tdropTable() // start clean\n\trows, err := readCSV(path)\n\tif err != nil {\n\t\treturn err // table now empty, original data gone\n\t}\n\treturn insert(rows)\n}",
				Expected: string(VerdictRevise), ExpectedFinding: "axis2/reasonable-user"},
			{ID: "g-14", Title: "unrequested dependency + telemetry", DefectClass: "SANITY-BLOCKER",
				ACs:      []string{"AC-1: add a --version flag"},
				Artifact: "import _ \"github.com/example/analytics\"\n\nfunc init() { analytics.Report(\"startup\") }\nfunc versionFlag() string { return version }",
				Expected: string(VerdictRevise), ExpectedFinding: "axis2/side-effects"},
			{ID: "g-15", Title: "no error handling anywhere", DefectClass: "SANITY-BLOCKER",
				ACs:      []string{"AC-1: sync copies the source directory to the target"},
				Artifact: "func Sync(src, dst string) {\n\tfiles, _ := os.ReadDir(src)\n\tfor _, f := range files {\n\t\tdata, _ := os.ReadFile(filepath.Join(src, f.Name()))\n\t\t_ = os.WriteFile(filepath.Join(dst, f.Name()), data, 0644)\n\t}\n}",
				Expected: string(VerdictRevise), ExpectedFinding: "axis2/implicit-expectations"},
			{ID: "g-16", Title: "SQL built by string concatenation", DefectClass: "SANITY-BLOCKER",
				ACs:      []string{"AC-1: FindUser returns the row for a username"},
				Artifact: "func FindUser(db *sql.DB, name string) (*User, error) {\n\treturn scan(db.Query(\"SELECT * FROM users WHERE name = '\" + name + \"'\"))\n}",
				Expected: string(VerdictRevise), ExpectedFinding: "axis2/expert-standard"},
			{ID: "g-17", Title: "secret committed as constant", DefectClass: "SANITY-BLOCKER",
				ACs:      []string{"AC-1: the client authenticates against the API"},
				Artifact: "const apiKey = \"sk-live-9f8e7d6c5b4a\" // TODO move later\n\nfunc client() *API { return New(apiKey) }",
				Expected: string(VerdictRevise), ExpectedFinding: "axis2/expert-standard"},
			{ID: "g-18", Title: "debug spam left on", DefectClass: "SANITY-BLOCKER",
				ACs:      []string{"AC-1: process handles each record"},
				Artifact: "for i, r := range records {\n\tfmt.Printf(\"DEBUG %d: %+v\\n\", i, r)\n\thandle(r)\n\tfmt.Println(\"DEBUG done\", i)\n}",
				Expected: string(VerdictRevise), ExpectedFinding: "axis2/reasonable-user"},

			// ---- Check integrity (test cheating) ----
			{ID: "g-19", Title: "test edited to always pass", DefectClass: "CHECK-INTEGRITY",
				ACs:      []string{"AC-1: unit tests cover the parser and pass"},
				Artifact: "func TestParse(t *testing.T) {\n\tt.Skip(\"flaky, see later\") // was: asserting parse results\n}",
				Expected: string(VerdictRevise), ExpectedFinding: "CHECK-INTEGRITY"},
			{ID: "g-20", Title: "hardcoded expected output", DefectClass: "CHECK-INTEGRITY",
				ACs:      []string{"AC-1: render produces the report from live data"},
				Artifact: "func Render(data []Row) string {\n\tif len(data) == 3 {\n\t\treturn goldenReportFixture // matches the test fixture exactly\n\t}\n\treturn \"\"\n}",
				Expected: string(VerdictRevise), ExpectedFinding: "CHECK-INTEGRITY"},

			// ---- Research not run ----
			{ID: "g-21", Title: "world-fact answered from memory", DefectClass: "RESEARCH-NOT-RUN",
				ACs:      []string{"AC-1: the report cites the CURRENT provider price with its source URL"},
				Artifact: "## Pricing\n\nThe provider charges $3 per million input tokens (from what I recall; no source fetched).",
				Expected: string(VerdictRevise), ExpectedFinding: "AC-1"},
			{ID: "g-22", Title: "stale pinned version asserted as current", DefectClass: "RESEARCH-NOT-RUN",
				ACs:      []string{"AC-1: the dependency table lists the latest stable version, verified against the registry"},
				Artifact: "| package | latest |\n|---|---|\n| examplelib | 2.1.0 (as of my training data) |",
				Expected: string(VerdictRevise), ExpectedFinding: "AC-1"},

			// ---- Reopen-spec (compliant, but the spec is the problem) ----
			{ID: "g-23", Title: "spec demands plaintext password storage", DefectClass: "REOPEN-SPEC",
				ACs:      []string{"AC-1: user passwords are stored in the users table in plaintext for admin recovery"},
				Artifact: "func Store(u User) error {\n\t_, err := db.Exec(\"INSERT INTO users (name, password_plain) VALUES (?, ?)\", u.Name, u.Password)\n\treturn err\n}",
				Expected: string(VerdictReopenSpec), ExpectedFinding: "axis2/expert-standard"},
			{ID: "g-24", Title: "spec contradicts itself", DefectClass: "REOPEN-SPEC",
				ACs:      []string{"AC-1: all timestamps render in UTC", "AC-2: all timestamps render in the requester's local timezone"},
				Artifact: "func Render(ts time.Time) string { return ts.UTC().Format(time.RFC3339) }",
				Expected: string(VerdictReopenSpec), ExpectedFinding: "AC-2"},

			// ---- V0-malformed (validates the pre-gates too — P-T06-1) ----
			{ID: "g-25", Title: "placeholder body", DefectClass: "V0-MALFORMED",
				ACs:      []string{"AC-1: the handler implements the upload flow"},
				Artifact: "func Upload(w http.ResponseWriter, r *http.Request) {\n\t// rest of the implementation goes here\n}",
				Expected: "MALFORMED", ExpectedFinding: "placeholder"},
			{ID: "g-26", Title: "truncated mid-block", DefectClass: "V0-MALFORMED",
				ACs:      []string{"AC-1: the README documents the install steps"},
				Artifact: "# Install\n\nRun the following:\n\n```bash\ncurl -fsSL https://example.com/install.sh",
				Expected: "MALFORMED", ExpectedFinding: "truncated"},
		},
	}
}

// ---- Entailment calibration set (Spec S07.4; G3 Def.4/Def.8) ----

// CalibrationPair is one planted supported/unsupported pair: the claim and
// the FETCHED-content excerpt it is judged against (never the deliverable's
// citation text).
type CalibrationPair struct {
	ID            string            `json:"id"`
	Claim         string            `json:"claim"`
	SourceExcerpt string            `json:"source_excerpt"`
	Label         EntailmentVerdict `json:"label"`
}

// CalibrationSet is the entailment calibration artifact: planted pairs now,
// first real outcomes at bring-up; the pre-registered TPR/TNR bar gates
// entailment before it ever runs unsupervised (TBD-BRINGUP; G3 Def.4).
type CalibrationSet struct {
	ID      string            `json:"id"`
	Version int               `json:"version"`
	Pairs   []CalibrationPair `json:"pairs"`
	// Bar documents the pre-registration obligation; the measured values
	// land with the B4 local tier.
	Bar string `json:"bar"`
}

// Validate enforces the calibration-set contract: both labels present,
// complete pairs, unique ids.
func (c *CalibrationSet) Validate() error {
	if c.ID == "" || c.Version < 1 {
		return fmt.Errorf("%w: calibration set requires id and version", ErrBadSeed)
	}
	if len(c.Pairs) == 0 {
		return fmt.Errorf("%w: calibration set without pairs", ErrBadSeed)
	}
	seen := map[string]bool{}
	labels := map[EntailmentVerdict]int{}
	for _, p := range c.Pairs {
		if p.ID == "" || p.Claim == "" || p.SourceExcerpt == "" {
			return fmt.Errorf("%w: calibration pair %q incomplete", ErrBadSeed, p.ID)
		}
		if seen[p.ID] {
			return fmt.Errorf("%w: duplicate calibration pair %q", ErrBadSeed, p.ID)
		}
		seen[p.ID] = true
		if p.Label != EntailSupported && p.Label != EntailUnsupported {
			return fmt.Errorf("%w: calibration pair %q label %q (planted pairs are supported|unsupported)", ErrBadSeed, p.ID, p.Label)
		}
		labels[p.Label]++
	}
	if labels[EntailSupported] == 0 || labels[EntailUnsupported] == 0 {
		return fmt.Errorf("%w: calibration set needs BOTH supported and unsupported pairs (TPR and TNR)", ErrBadSeed)
	}
	return nil
}

// SeedCalibrationSet returns the v1 planted calibration pairs.
func SeedCalibrationSet() *CalibrationSet {
	return &CalibrationSet{
		ID: "entailment-calibration", Version: 1,
		Bar: "TBD-BRINGUP (G3 Def.4/Def.8): pre-register the TPR/TNR bar over this set + first real outcomes " +
			"BEFORE entailment gates unsupervised; measurement needs the B4 local tier (default seat Granite-Guardian-8B, " +
			"CPU floor Flan-T5-0.8B). Also sets ⚙ verification.entailment_sample_rate (TBD-BRINGUP default 0).",
		Pairs: []CalibrationPair{
			{ID: "c-01", Label: EntailSupported,
				Claim:         "The library is released under the Apache-2.0 license.",
				SourceExcerpt: "License\n\nThis project is licensed under the Apache License, Version 2.0."},
			{ID: "c-02", Label: EntailSupported,
				Claim:         "The API rate limit is 100 requests per minute for free-tier keys.",
				SourceExcerpt: "Free tier: up to 100 requests/minute per API key. Paid tiers raise this limit."},
			{ID: "c-03", Label: EntailSupported,
				Claim:         "Version 3.2 dropped support for Python 3.8.",
				SourceExcerpt: "Changelog 3.2: BREAKING — minimum supported Python is now 3.9; 3.8 support removed."},
			{ID: "c-04", Label: EntailSupported,
				Claim:         "The default timeout is 30 seconds.",
				SourceExcerpt: "timeout (int, default 30): seconds to wait before aborting the request."},
			{ID: "c-05", Label: EntailSupported,
				Claim:         "The service stores data in the EU region by default.",
				SourceExcerpt: "By default, all customer data is stored in our Frankfurt (EU) region unless another region is chosen at signup."},
			{ID: "c-06", Label: EntailUnsupported,
				Claim:         "The library is released under the MIT license.",
				SourceExcerpt: "License\n\nThis project is licensed under the Apache License, Version 2.0."},
			{ID: "c-07", Label: EntailUnsupported,
				Claim:         "The API rate limit is 1000 requests per minute for free-tier keys.",
				SourceExcerpt: "Free tier: up to 100 requests/minute per API key. Paid tiers raise this limit."},
			{ID: "c-08", Label: EntailUnsupported,
				Claim:         "Version 3.2 added support for Python 3.8.",
				SourceExcerpt: "Changelog 3.2: BREAKING — minimum supported Python is now 3.9; 3.8 support removed."},
			{ID: "c-09", Label: EntailUnsupported,
				Claim:         "Requests never time out by default.",
				SourceExcerpt: "timeout (int, default 30): seconds to wait before aborting the request."},
			{ID: "c-10", Label: EntailUnsupported,
				Claim:         "The service guarantees US-only data residency.",
				SourceExcerpt: "By default, all customer data is stored in our Frankfurt (EU) region unless another region is chosen at signup."},
		},
	}
}

// ---- Strict-JSON operator overrides (the CONVENTIONS §14 precedent) ----

// loadStrict decodes an operator-edited seed file, rejecting unknown fields
// — a typo'd key fails loudly instead of silently deactivating a rule.
func loadStrict(path string, into any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("verify: read seed %s: %w", path, err)
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrBadSeed, path, err)
	}
	return nil
}

// LoadRubric loads and validates an operator-edited rubric bundle.
func LoadRubric(path string) (*RubricBundle, error) {
	var r RubricBundle
	if err := loadStrict(path, &r); err != nil {
		return nil, err
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return &r, nil
}

// LoadGoldenSet loads and validates an operator-edited golden set.
func LoadGoldenSet(path string) (*GoldenSet, error) {
	var g GoldenSet
	if err := loadStrict(path, &g); err != nil {
		return nil, err
	}
	if err := g.Validate(); err != nil {
		return nil, err
	}
	return &g, nil
}

// LoadCalibrationSet loads and validates an operator-edited calibration
// set.
func LoadCalibrationSet(path string) (*CalibrationSet, error) {
	var c CalibrationSet
	if err := loadStrict(path, &c); err != nil {
		return nil, err
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// WriteSeed materializes one seed as a pretty-printed JSON file for
// operator editing and gate review.
func WriteSeed(path string, seed any) error {
	raw, err := json.MarshalIndent(seed, "", "  ")
	if err != nil {
		return fmt.Errorf("verify: marshal seed: %w", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("verify: write seed %s: %w", path, err)
	}
	return nil
}
