package intake

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The artifact pair (Spec S06.6): Sinet-owned markdown + YAML frontmatter
// (ids, owner, stakes tier, provenance, status), stable keys,
// git-committable (D9), checkpointed at every stage boundary (D7),
// referenced from the ledger. The markdown file is the artifact of record
// (its sha256 is what the ledger references); a machine-readable JSON
// sidecar is written beside it for lossless reload, and a load verifies
// the markdown still renders byte-identically from the sidecar — one
// truth, two projections.

// Artifact statuses.
const (
	StatusDraft    = "draft"
	StatusApproved = "approved"
)

// AC is one numbered acceptance criterion (AC-1..n, stable keys): plain
// phrasing always; a structured machine-checkable sub-line only where the
// criterion is machine-checkable (G1 P10 dual phrasing — EARS for
// behavioral criteria, GWT where examples communicate better; both
// admissible at v0).
type AC struct {
	N              int    `json:"n"`
	Plain          string `json:"plain"`
	Structured     string `json:"structured,omitempty"`
	StructuredKind string `json:"structured_kind,omitempty"` // "ears" | "gwt" | ""
}

// Key returns the stable AC key ("AC-3").
func (a AC) Key() string { return fmt.Sprintf("AC-%d", a.N) }

// Assumption is one explicit assumption listed on the SPEC — the approval
// card's centerpiece (Spec S06.9).
type Assumption struct {
	Text   string `json:"text"`
	Origin string `json:"origin,omitempty"` // "slot:<id>" | "force_proceed" | "planner" | "band" | "marker"
}

// AssumptionOriginMarker labels an assumption minted from a NEEDS-CLARIFICATION
// marker (P3-GF12 R6/R7): either one the requester answered and the planner
// re-raised — settled from the record, carrying question and answer — or one
// still open when the clarification rounds ran out, converted rather than
// swallowed (S06.6: "each marker is either asked or converted to a listed
// assumption"). Both land on the approval card's centerpiece, where a wrong
// reading is contestable via Re-plan.
const AssumptionOriginMarker = "marker"

// Spec is the SPEC artifact — the contract all later work is measured
// against (feature 1.3; Spec S06.6).
type Spec struct {
	TaskID     string `json:"task_id"`
	Owner      string `json:"owner"`
	Version    int    `json:"version"`
	Status     string `json:"status"`
	Tier       Tier   `json:"tier"`
	Provenance string `json:"provenance"` // model/version provenance

	Restatement string   `json:"restatement"`       // plain-language goal restatement, requester's terms
	Outcome     []string `json:"outcome,omitempty"` // technology-agnostic outcome criteria (SC-style)

	ACs []AC `json:"acs"`

	Constraints    []string       `json:"constraints,omitempty"`
	Assumptions    []Assumption   `json:"assumptions,omitempty"`
	OutOfScope     []string       `json:"out_of_scope,omitempty"`   // the "will NOT do" list
	Supplied       []SuppliedFact `json:"supplied,omitempty"`       // requester-supplied inputs (S06.3)
	Clarifications []string       `json:"clarifications,omitempty"` // open NEEDS-CLARIFICATION markers
}

// VersionKey returns the spec version string used on the ledger header and
// in the freshness fingerprint ("spec-v2").
func (s *Spec) VersionKey() string { return fmt.Sprintf("spec-v%d", s.Version) }

// AC returns the criterion by stable key, or nil.
func (s *Spec) AC(key string) *AC {
	for i := range s.ACs {
		if s.ACs[i].Key() == key {
			return &s.ACs[i]
		}
	}
	return nil
}

// Validate checks the SPEC contract: numbered contiguous ACs, plain
// phrasing on every criterion (the requester is never required to read
// notation), a restatement.
func (s *Spec) Validate() error {
	if s.TaskID == "" || s.Owner == "" || s.Version < 1 {
		return fmt.Errorf("%w: spec requires task, owner, version", ErrBadArtifact)
	}
	if s.Status != StatusDraft && s.Status != StatusApproved {
		return fmt.Errorf("%w: spec status %q", ErrBadArtifact, s.Status)
	}
	if s.Restatement == "" {
		return fmt.Errorf("%w: spec without restatement", ErrBadArtifact)
	}
	if len(s.ACs) == 0 {
		return fmt.Errorf("%w: spec without acceptance criteria", ErrBadArtifact)
	}
	for i, ac := range s.ACs {
		if ac.N != i+1 {
			return fmt.Errorf("%w: acceptance criteria must be numbered 1..n contiguously", ErrBadArtifact)
		}
		if ac.Plain == "" {
			return fmt.Errorf("%w: %s without plain phrasing (G1 P10)", ErrBadArtifact, ac.Key())
		}
	}
	return nil
}

// ResearchNode marks a plan step as the required live-research node for a
// data-bearing flag (Spec S06.6; 1.9/P47).
type ResearchNode struct {
	RuleID string `json:"rule_id"`
	StepID string `json:"step_id"`
	Query  string `json:"query,omitempty"`
}

// The [A15] per-step approach carries four STRUCTURAL constants, not ⚙ keys:
// S18 declares none, and none of these changes what the platform decides —
// they bound how much text one step may carry so a card and an artifact stay
// readable and one emission cannot balloon the pair (the maxQuestionsPerCard /
// sseBatchSize precedent). They are flagged to the operator gate under the
// settings-tab directive, like every other constant of this class.
const (
	// approachMaxRunes bounds the plain-words HOW. The approach is a few
	// sentences a non-programmer reads on a card, never an essay; the cap is
	// an abuse guard on card and artifact size, not an editorial rule.
	approachMaxRunes = 1200
	// decisionFieldMaxRunes bounds each Decision / Alternative / Why string.
	// One material decision states itself in a sentence or two; more than that
	// is the approach text's job.
	decisionFieldMaxRunes = 400
	// maxStepDecisions bounds the listed material decisions per step. A step
	// making more than this many separate calls is a step that should be split
	// — which is a planning defect the requester should see as one, not a wall
	// of decisions on one card.
	maxStepDecisions = 8
	// orderingMaxRunes bounds the ordering rationale: one load-bearing
	// sentence or two saying why this step sits where it does.
	orderingMaxRunes = 400
)

// The exported cap aliases exist so the ONE emission contract states its own
// bounds FROM these constants (P3-GF12: the pairSchema prompt interpolates
// them — a second literal would be a second spelling that drifts, the §65 D4
// class of defect). The unexported names stay the validator's; these are the
// prompt's, and they are the same numbers by construction.
//
// The NUMBERS do not move (P3-GF12 R8). Every witnessed overshoot was within
// 1.31× of 1200 — the signature of an author who was never told the budget, not
// of a budget that cannot hold an honest approach; 1200 characters is roughly
// 200 words, which fits S06.6's "plain words, a few sentences" comfortably with
// the decisions carried in their own capped fields; and the cap guards card and
// artifact size, which a raise without the prompt statement would move rather
// than fix. If a told-the-bound planner still overruns routinely, the constant
// moves then, with that as its stated reason.
const (
	ApproachMaxRunes      = approachMaxRunes
	DecisionFieldMaxRunes = decisionFieldMaxRunes
	MaxStepDecisions      = maxStepDecisions
	OrderingMaxRunes      = orderingMaxRunes
)

// StepDecision is one material decision the planner made inside a step
// [A15, 2026-08-27]: what was decided in plain words, the alternatives it
// considered, and why the chosen one won (Spec S06.6 per-step approach;
// operator record r4 §F3). Requester-facing plan content — never a
// machine-check target.
type StepDecision struct {
	Decision     string   `json:"decision"`
	Alternatives []string `json:"alternatives,omitempty"`
	Why          string   `json:"why"`
}

// Step is one numbered plan step (stable key "S-n") with its per-step
// "Done when" stage contract (consumed by verification, Spec S07) and its
// declarations: confinement class, write-set, and the effect/spend/
// credential/tool facts the Stage-2 floor re-check reads (Spec S06.7(b)).
type Step struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	DoneWhen string `json:"done_when"`

	// Per-step approach [A15, 2026-08-27] — the artifact-schema marker site
	// the S00.9 A15 row requires annotated. Approach states HOW the step
	// will be built, in plain words at non-IT reading level; Decisions are
	// the material decisions with alternatives and why; OrderingRationale
	// says why this step sits where it does, only where the ordering is
	// load-bearing. Rendered under the step with the assumptions treatment
	// (S06.9 Layer 2); verification still binds to the ACs and DoneWhen —
	// the approach informs the Stage-4 human judgment, it is not a new
	// machine-check target. All json-omitempty so PRE-A15 artifacts reload
	// and re-render byte-identically (the CitedEntries precedent).
	Approach          string         `json:"approach,omitempty"`
	Decisions         []StepDecision `json:"decisions,omitempty"`
	OrderingRationale string         `json:"ordering_rationale,omitempty"`

	// Confinement declaration (C0–C2 at v0; Spec S11 owns mechanics). The
	// class declared here flows to every helper, tighter-only; widening is
	// a plan change requiring delta re-approval (P-T05-1).
	Class string `json:"class"`

	WriteSet  []string `json:"write_set,omitempty"`
	ReadSet   []string `json:"read_set,omitempty"`
	Unbounded bool     `json:"unbounded,omitempty"` // cannot bound its write-set (S02.8 whole-project claim)

	OutwardEffects   []string `json:"outward_effects,omitempty"`
	NewSpend         bool     `json:"new_spend,omitempty"`
	CredentialTouch  bool     `json:"credential_touch,omitempty"`
	SharedAssetWrite bool     `json:"shared_asset_write,omitempty"`
	NewTools         []string `json:"new_tools,omitempty"`

	Research bool `json:"research,omitempty"` // this step is a research node
}

// validateApproach enforces the [A15] per-step approach contract at the
// artifact boundary (Spec S06.6 "Per-step approach [A15]"; P3-GF8 R11).
//
// The approach is REQUIRED: a step that names only its outcome cannot be judged
// for approach, which is the one job the 1.6 cheap preview exists to do. A
// listed decision must be COMPLETE — S06.6 says "the material decisions it made
// inside the step with the alternatives it considered and why the chosen one
// won", and a decision with no alternative is not a material decision, it is a
// sentence. The ordering rationale is optional by contract: it is owed only
// where the ordering is load-bearing, and demanding it everywhere would buy
// filler.
//
// Nothing here is a machine-check target: verification still binds to the ACs
// and the "Done when" contracts [A15 "What does NOT change"]. This is a size
// and completeness contract, never a reading-level one.
func (s *Step) validateApproach() error {
	if strings.TrimSpace(s.Approach) == "" {
		return fmt.Errorf("%w: step %s states no approach — a step that names only its outcome cannot be judged for approach (S06.6 [A15])", ErrBadArtifact, s.ID)
	}
	if n := utf8.RuneCountInString(s.Approach); n > approachMaxRunes {
		return fmt.Errorf("%w: step %s approach is %d characters (cap %d)", ErrBadArtifact, s.ID, n, approachMaxRunes)
	}
	if len(s.Decisions) > maxStepDecisions {
		return fmt.Errorf("%w: step %s lists %d material decisions (cap %d — a step making more calls than that is a step that should be split)",
			ErrBadArtifact, s.ID, len(s.Decisions), maxStepDecisions)
	}
	for i, d := range s.Decisions {
		if strings.TrimSpace(d.Decision) == "" {
			return fmt.Errorf("%w: step %s decision %d says nothing (S06.6 [A15])", ErrBadArtifact, s.ID, i+1)
		}
		if strings.TrimSpace(d.Why) == "" {
			return fmt.Errorf("%w: step %s decision %q gives no reason it won (S06.6 [A15])", ErrBadArtifact, s.ID, d.Decision)
		}
		alternatives := 0
		for _, alt := range d.Alternatives {
			if strings.TrimSpace(alt) != "" {
				alternatives++
			}
			if n := utf8.RuneCountInString(alt); n > decisionFieldMaxRunes {
				return fmt.Errorf("%w: step %s decision %q has an alternative of %d characters (cap %d)", ErrBadArtifact, s.ID, d.Decision, n, decisionFieldMaxRunes)
			}
		}
		if alternatives == 0 {
			return fmt.Errorf("%w: step %s decision %q considered no alternative — a material decision with nothing to weigh against is not one (S06.6 [A15])", ErrBadArtifact, s.ID, d.Decision)
		}
		for _, field := range []string{d.Decision, d.Why} {
			if n := utf8.RuneCountInString(field); n > decisionFieldMaxRunes {
				return fmt.Errorf("%w: step %s decision %q carries a %d-character field (cap %d)", ErrBadArtifact, s.ID, d.Decision, n, decisionFieldMaxRunes)
			}
		}
	}
	if n := utf8.RuneCountInString(s.OrderingRationale); n > orderingMaxRunes {
		return fmt.Errorf("%w: step %s ordering rationale is %d characters (cap %d)", ErrBadArtifact, s.ID, n, orderingMaxRunes)
	}
	return nil
}

// Plan is the PLAN artifact (Spec S06.6).
type Plan struct {
	TaskID      string `json:"task_id"`
	Owner       string `json:"owner"`
	Version     int    `json:"version"`
	SpecVersion int    `json:"spec_version"` // the SPEC this plan implements
	Status      string `json:"status"`
	Tier        Tier   `json:"tier"`
	Provenance  string `json:"provenance"`

	Steps []Step `json:"steps"`

	// Coverage is the AC coverage map: every AC key → owning step id(s) —
	// the 1.4 traceability substrate (Spec S06.7(a)).
	Coverage map[string][]string `json:"coverage"`

	ResearchNodes []ResearchNode `json:"research_nodes,omitempty"`
	Risks         []string       `json:"risks,omitempty"`

	// CitedEntries lists the project-truth knowledge entries this plan's
	// assumptions cite (Spec S09.6). Their versions join the freshness
	// fingerprint at approval — a ratified extension of the G1 Def.5 set (R30).
	// Additive and optional: a plan without citations renders byte-identically
	// to today (the omitempty + guarded section preserve the S06.6 re-render
	// integrity).
	CitedEntries []string `json:"cited_entries,omitempty"`

	// Est is the plan-derived size/cost estimate, explicitly compared to
	// the Stage-0 guess by the spine — surfaced, never silent (2.5).
	Est Estimate `json:"est"`
}

// VersionKey returns the plan version string ("plan-v1").
func (p *Plan) VersionKey() string { return fmt.Sprintf("plan-v%d", p.Version) }

// SpecPlanVersion is the combined version that joins the freshness
// fingerprint at approval (G1 Def.5; Spec S06.9).
func (p *Plan) SpecPlanVersion() string {
	return fmt.Sprintf("spec-v%d/plan-v%d", p.SpecVersion, p.Version)
}

// Step returns the step by id, or nil.
func (p *Plan) Step(id string) *Step {
	for i := range p.Steps {
		if p.Steps[i].ID == id {
			return &p.Steps[i]
		}
	}
	return nil
}

// Validate checks the PLAN contract: numbered steps with Done-when and a
// valid confinement class, a coverage map over known keys.
func (p *Plan) Validate() error {
	if p.TaskID == "" || p.Owner == "" || p.Version < 1 || p.SpecVersion < 1 {
		return fmt.Errorf("%w: plan requires task, owner, versions", ErrBadArtifact)
	}
	if p.Status != StatusDraft && p.Status != StatusApproved {
		return fmt.Errorf("%w: plan status %q", ErrBadArtifact, p.Status)
	}
	if len(p.Steps) == 0 {
		return fmt.Errorf("%w: plan without steps", ErrBadArtifact)
	}
	for i, st := range p.Steps {
		if st.ID != fmt.Sprintf("S-%d", i+1) {
			return fmt.Errorf("%w: steps must carry stable keys S-1..n", ErrBadArtifact)
		}
		if st.Title == "" || st.DoneWhen == "" {
			return fmt.Errorf("%w: step %s requires title and done-when", ErrBadArtifact, st.ID)
		}
		switch st.Class {
		case "C0", "C1", "C2":
		default:
			return fmt.Errorf("%w: step %s confinement class %q (C0–C2 at v0, Spec S11)", ErrBadArtifact, st.ID, st.Class)
		}
		if err := st.validateApproach(); err != nil {
			return err
		}
	}
	for key, steps := range p.Coverage {
		for _, sid := range steps {
			if p.Step(sid) == nil {
				return fmt.Errorf("%w: coverage %s cites unknown step %s", ErrBadArtifact, key, sid)
			}
		}
	}
	for _, rn := range p.ResearchNodes {
		if p.Step(rn.StepID) == nil {
			return fmt.Errorf("%w: research node %s cites unknown step %s", ErrBadArtifact, rn.RuleID, rn.StepID)
		}
	}
	return nil
}

// WriteGlobs returns the union of declared step write-sets and whether any
// step left its write-set unbounded.
func (p *Plan) WriteGlobs() (globs []string, unbounded bool) {
	seen := map[string]bool{}
	for _, st := range p.Steps {
		if st.Unbounded {
			unbounded = true
		}
		for _, g := range st.WriteSet {
			if !seen[g] {
				seen[g] = true
				globs = append(globs, g)
			}
		}
	}
	return globs, unbounded
}

// ReadGlobs returns the union of declared step read-sets.
func (p *Plan) ReadGlobs() []string {
	seen := map[string]bool{}
	var globs []string
	for _, st := range p.Steps {
		for _, g := range st.ReadSet {
			if !seen[g] {
				seen[g] = true
				globs = append(globs, g)
			}
		}
	}
	return globs
}

// Pair is the Stage-1 artifact pair.
type Pair struct {
	Spec Spec `json:"spec"`
	Plan Plan `json:"plan"`
}

// Validate checks pair coherence.
func (p *Pair) Validate() error {
	if err := p.Spec.Validate(); err != nil {
		return err
	}
	if err := p.Plan.Validate(); err != nil {
		return err
	}
	if p.Plan.SpecVersion != p.Spec.Version {
		return fmt.Errorf("%w: plan implements spec-v%d but spec is v%d", ErrBadArtifact, p.Plan.SpecVersion, p.Spec.Version)
	}
	for key := range p.Plan.Coverage {
		if p.Spec.AC(key) == nil {
			return fmt.Errorf("%w: coverage cites unknown criterion %s", ErrBadArtifact, key)
		}
	}
	return nil
}

// ErrBadArtifact covers artifact contract violations.
var ErrBadArtifact = fmt.Errorf("intake: invalid artifact")

// ArtifactRef references one durable artifact file: path + sha256 of the
// markdown of record, per the ledger's restorable-reference rule.
type ArtifactRef struct {
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Version int    `json:"version"`
}

// artifactStore reads and writes the per-task artifact directory.
type artifactStore struct{ root string }

func (a artifactStore) dir(taskID string) string { return filepath.Join(a.root, taskID) }

func (a artifactStore) specPath(taskID string, v int) string {
	return filepath.Join(a.dir(taskID), fmt.Sprintf("SPEC-v%d.md", v))
}

func (a artifactStore) planPath(taskID string, v int) string {
	return filepath.Join(a.dir(taskID), fmt.Sprintf("PLAN-v%d.md", v))
}

func (a artifactStore) critiquePath(taskID string, planV, round int) string {
	return filepath.Join(a.dir(taskID), fmt.Sprintf("critique-plan-v%d-r%d.md", planV, round))
}

// recordPath names one VERSION of the Stage-0 intake record. The record is
// written twice — v1 is the honest pre-classification baseline written at
// birth, v2 the classified values — and the version rides the filename so the
// two are distinct immutable files rather than one file overwritten in place
// (S06.6: a draft artifact never mutates, and the ledger's earlier sha256 must
// keep resolving).
func (a artifactStore) recordPath(taskID string, v int) string {
	return filepath.Join(a.dir(taskID), fmt.Sprintf("intake-record-v%d.json", v))
}

// write persists one artifact: the markdown of record plus the JSON
// sidecar, atomically (tmp + rename, same directory). Returns the ref over
// the markdown bytes.
func (a artifactStore) write(path string, md []byte, machine any, version int) (ArtifactRef, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return ArtifactRef{}, fmt.Errorf("intake: artifact dir: %w", err)
	}
	if err := atomicWrite(path, md); err != nil {
		return ArtifactRef{}, err
	}
	if machine != nil {
		raw, err := json.MarshalIndent(machine, "", "  ")
		if err != nil {
			return ArtifactRef{}, fmt.Errorf("intake: marshal sidecar: %w", err)
		}
		if err := atomicWrite(sidecarPath(path), append(raw, '\n')); err != nil {
			return ArtifactRef{}, err
		}
	}
	sum := sha256.Sum256(md)
	return ArtifactRef{Path: path, SHA256: hex.EncodeToString(sum[:]), Version: version}, nil
}

// loadSpec reloads a SPEC from its sidecar and verifies the markdown of
// record still renders byte-identically (resume-from-artifact integrity,
// Spec S06.1).
func (a artifactStore) loadSpec(ref *ArtifactRef) (Spec, error) {
	var s Spec
	if err := loadVerified(ref, &s, func() []byte { return renderSpecMD(&s) }); err != nil {
		return Spec{}, err
	}
	return s, nil
}

// loadPlan mirrors loadSpec for the PLAN.
func (a artifactStore) loadPlan(ref *ArtifactRef) (Plan, error) {
	var p Plan
	if err := loadVerified(ref, &p, func() []byte { return renderPlanMD(&p) }); err != nil {
		return Plan{}, err
	}
	return p, nil
}

func loadVerified(ref *ArtifactRef, into any, render func() []byte) error {
	if ref == nil {
		return fmt.Errorf("%w: no artifact ref", ErrBadArtifact)
	}
	raw, err := os.ReadFile(sidecarPath(ref.Path))
	if err != nil {
		return fmt.Errorf("intake: read artifact sidecar: %w", err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return fmt.Errorf("%w: sidecar %s: %v", ErrBadArtifact, ref.Path, err)
	}
	md, err := os.ReadFile(ref.Path)
	if err != nil {
		return fmt.Errorf("intake: read artifact: %w", err)
	}
	sum := sha256.Sum256(md)
	if got := hex.EncodeToString(sum[:]); got != ref.SHA256 {
		return fmt.Errorf("%w: %s hash drift (have %s, ref %s)", ErrBadArtifact, ref.Path, got[:12], ref.SHA256[:12])
	}
	if !bytes.Equal(render(), md) {
		return fmt.Errorf("%w: %s markdown does not match its sidecar", ErrBadArtifact, ref.Path)
	}
	return nil
}

func sidecarPath(mdPath string) string { return strings.TrimSuffix(mdPath, ".md") + ".json" }

func atomicWrite(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("intake: write %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("intake: rename %s: %w", path, err)
	}
	return nil
}

// ---- Markdown rendering (deterministic; the artifact of record) ----

func fm(b *strings.Builder, kv ...string) {
	b.WriteString("---\n")
	for i := 0; i+1 < len(kv); i += 2 {
		fmt.Fprintf(b, "%s: %s\n", kv[i], kv[i+1])
	}
	b.WriteString("---\n\n")
}

func section(b *strings.Builder, title string, lines []string) {
	if len(lines) == 0 {
		return
	}
	fmt.Fprintf(b, "## %s\n\n", title)
	for _, l := range lines {
		fmt.Fprintf(b, "- %s\n", l)
	}
	b.WriteString("\n")
}

// renderSpecMD renders the SPEC markdown of record (Spec S06.6 contract).
func renderSpecMD(s *Spec) []byte {
	var b strings.Builder
	fm(&b,
		"kind", "spec",
		"task", s.TaskID,
		"owner", s.Owner,
		"version", fmt.Sprint(s.Version),
		"status", s.Status,
		"tier", string(s.Tier),
		"provenance", s.Provenance,
	)
	fmt.Fprintf(&b, "# Specification — %s %s\n\n", s.TaskID, s.VersionKey())
	fmt.Fprintf(&b, "## Restatement\n\n%s\n\n", s.Restatement)
	section(&b, "Outcome criteria", s.Outcome)
	b.WriteString("## Acceptance criteria\n\n")
	for _, ac := range s.ACs {
		fmt.Fprintf(&b, "%d. **%s**: %s\n", ac.N, ac.Key(), ac.Plain)
		if ac.Structured != "" {
			kind := ac.StructuredKind
			if kind == "" {
				kind = "structured"
			}
			fmt.Fprintf(&b, "   - (%s) %s\n", kind, ac.Structured)
		}
	}
	b.WriteString("\n")
	section(&b, "Constraints", s.Constraints)
	if len(s.Assumptions) > 0 {
		b.WriteString("## Assumptions\n\n")
		for _, a := range s.Assumptions {
			if a.Origin != "" {
				fmt.Fprintf(&b, "- [%s] %s\n", a.Origin, a.Text)
			} else {
				fmt.Fprintf(&b, "- %s\n", a.Text)
			}
		}
		b.WriteString("\n")
	}
	section(&b, "Out of scope — will NOT do", s.OutOfScope)
	if len(s.Supplied) > 0 {
		b.WriteString("## Requester-supplied inputs\n\n")
		for _, f := range s.Supplied {
			fmt.Fprintf(&b, "- [%s] %s\n", f.RuleID, f.Fact)
		}
		b.WriteString("\n")
	}
	if len(s.Clarifications) > 0 {
		b.WriteString("## NEEDS-CLARIFICATION\n\n")
		for _, c := range s.Clarifications {
			fmt.Fprintf(&b, "- %s\n", c)
		}
		b.WriteString("\n")
	}
	return []byte(b.String())
}

// renderPlanMD renders the PLAN markdown of record.
func renderPlanMD(p *Plan) []byte {
	var b strings.Builder
	fm(&b,
		"kind", "plan",
		"task", p.TaskID,
		"owner", p.Owner,
		"version", fmt.Sprint(p.Version),
		"spec_version", fmt.Sprint(p.SpecVersion),
		"status", p.Status,
		"tier", string(p.Tier),
		"provenance", p.Provenance,
	)
	fmt.Fprintf(&b, "# Plan — %s %s (implements %s)\n\n", p.TaskID, p.VersionKey(), fmt.Sprintf("spec-v%d", p.SpecVersion))
	b.WriteString("## Steps\n\n")
	for i, st := range p.Steps {
		fmt.Fprintf(&b, "%d. **%s**: %s\n", i+1, st.ID, st.Title)
		fmt.Fprintf(&b, "   - Done when: %s\n", st.DoneWhen)
		// The [A15] per-step approach, on the artifact of record and not only
		// in the sidecar: the markdown IS the plan (D9), so a plan whose HOW
		// lived only in a JSON projection would still be a black box to
		// anyone reading the artifact. Every line is GUARDED — the
		// CitedEntries precedent — so a pre-A15 plan re-renders byte-
		// identically and keeps loading (S06.6 resume integrity).
		if st.Approach != "" {
			fmt.Fprintf(&b, "   - Approach: %s\n", st.Approach)
		}
		for _, d := range st.Decisions {
			line := "   - Decision: " + d.Decision
			if len(d.Alternatives) > 0 {
				line += " (considered instead: " + strings.Join(d.Alternatives, ", ") + ")"
			}
			if d.Why != "" {
				line += "; why: " + d.Why
			}
			b.WriteString(line + "\n")
		}
		if st.OrderingRationale != "" {
			fmt.Fprintf(&b, "   - Why this step sits here: %s\n", st.OrderingRationale)
		}
		fmt.Fprintf(&b, "   - Confinement: %s\n", st.Class)
		if len(st.WriteSet) > 0 {
			fmt.Fprintf(&b, "   - Writes: %s\n", strings.Join(st.WriteSet, ", "))
		}
		if st.Unbounded {
			b.WriteString("   - Writes: UNBOUNDED (whole-project claim, S02.8)\n")
		}
		if len(st.ReadSet) > 0 {
			fmt.Fprintf(&b, "   - Reads: %s\n", strings.Join(st.ReadSet, ", "))
		}
		if len(st.OutwardEffects) > 0 {
			fmt.Fprintf(&b, "   - Outward effects: %s\n", strings.Join(st.OutwardEffects, ", "))
		}
		if st.NewSpend {
			b.WriteString("   - New spend\n")
		}
		if st.CredentialTouch {
			b.WriteString("   - Credential touch\n")
		}
		if st.SharedAssetWrite {
			b.WriteString("   - Shared-asset write\n")
		}
		if len(st.NewTools) > 0 {
			fmt.Fprintf(&b, "   - New tools: %s\n", strings.Join(st.NewTools, ", "))
		}
		if st.Research {
			b.WriteString("   - Research node\n")
		}
	}
	b.WriteString("\n## Coverage map\n\n")
	for _, key := range sortedACKeys(p.Coverage) {
		fmt.Fprintf(&b, "- %s → %s\n", key, strings.Join(p.Coverage[key], ", "))
	}
	b.WriteString("\n")
	if len(p.ResearchNodes) > 0 {
		b.WriteString("## Research nodes\n\n")
		for _, rn := range p.ResearchNodes {
			line := fmt.Sprintf("- [%s] → %s", rn.RuleID, rn.StepID)
			if rn.Query != "" {
				line += ": " + rn.Query
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}
	if len(p.CitedEntries) > 0 {
		// Cited project-truth knowledge (Spec S09.6); only rendered when
		// present, so plans without citations re-render byte-identically.
		b.WriteString("## Cited knowledge\n\n")
		for _, c := range p.CitedEntries {
			fmt.Fprintf(&b, "- %s\n", c)
		}
		b.WriteString("\n")
	}
	section(&b, "Risks", p.Risks)
	b.WriteString("## Estimate\n\n")
	if p.Est.Known {
		fmt.Fprintf(&b, "- Size %s, ~%.2f USD (API-equivalent, D5)\n", orDash(p.Est.SizeClass), p.Est.USD)
	} else {
		fmt.Fprintf(&b, "- Size %s, UNPRICED (no price table — honest posture, S10)\n", orDash(p.Est.SizeClass))
	}
	if p.Est.Basis != "" {
		fmt.Fprintf(&b, "- Basis: %s\n", p.Est.Basis)
	}
	b.WriteString("\n")
	return []byte(b.String())
}

// renderCritiqueMD renders the Stage-3 findings artifact.
func renderCritiqueMD(taskID string, planV, round int, v Verdict) []byte {
	var b strings.Builder
	fm(&b,
		"kind", "critique",
		"task", taskID,
		"plan_version", fmt.Sprint(planV),
		"round", fmt.Sprint(round),
		"verdict", string(v.Kind),
	)
	fmt.Fprintf(&b, "# Critique — %s plan-v%d round %d\n\n**Verdict: %s**\n\n", taskID, planV, round, strings.ToUpper(string(v.Kind)))
	if v.Doubt != "" {
		fmt.Fprintf(&b, "## Why this specification may be a bad idea\n\n%s\n\n", v.Doubt)
	}
	if len(v.Findings) > 0 {
		b.WriteString("## Findings\n\n")
		for i, f := range v.Findings {
			fmt.Fprintf(&b, "%d. %s\n", i+1, f)
		}
		b.WriteString("\n")
	}
	if v.ProposedTier != "" {
		fmt.Fprintf(&b, "## Proposed tier\n\n- %s\n\n", v.ProposedTier)
	}
	return []byte(b.String())
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// sortedACKeys orders coverage keys by their numeric suffix (AC-1, AC-2,
// …) so rendering is deterministic.
func sortedACKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		ni, _ := strconv.Atoi(strings.TrimPrefix(keys[i], "AC-"))
		nj, _ := strconv.Atoi(strings.TrimPrefix(keys[j], "AC-"))
		if ni != nj {
			return ni < nj
		}
		return keys[i] < keys[j]
	})
	return keys
}
