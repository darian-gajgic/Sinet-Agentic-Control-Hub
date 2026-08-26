package metering

// planunits.go — the S10.1 tier-3 reading: a flat-rate plan's own units.
//
// A subscription lane has two consumption stories and they are NOT the same
// story. The tokens a call consumed are MEASURED and land on the ledger row at
// tier 1 (metering.go). What the PLAN charged for that call is a different
// quantity in a different unit, published as an allowance with a charging
// multiplier and no per-request counter — so it is DERIVED, at tier 3, from
// the requests actually made (S10.4: "requests-as-proxy with the documented
// multipliers applied as data").
//
// Folding the two together would produce one confident number that is wrong in
// both units. They are therefore two readings, each stamped with its own tier
// and each labeled with what it assumed.
//
// Every number the derivation needs — the multipliers, their window, the
// allowance, the unit itself — is a DATED ROW in plandata/, never a Go
// constant. The plan re-denominated from prompts to credits and moved its
// multipliers inside five weeks of the spec being written; a constant would
// have gone stale invisibly, where a dated row goes stale visibly.

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"
)

// The plan documents ship as a DIRECTORY embed for the same reason the lane
// documents do: dropping a document in must genuinely add a lane's plan.
//
//go:embed plandata/*.json
var planSeeds embed.FS

// ErrPlanDoc reports a plan document that cannot be used as a denominator.
var ErrPlanDoc = errors.New("metering: plan budget document")

// Quota names the seed documents ship. They are KEYS into the data, not
// numbers: the window lengths and allowances live in the rows.
const (
	// PlanQuotaWindow is the plan's short rolling allowance.
	PlanQuotaWindow = "rolling-5h"
	// PlanQuotaWeekly is the plan's long allowance.
	PlanQuotaWeekly = "weekly"
)

// planDateLayout is the verified-on date shape. A mechanic, not a ⚙ value.
const planDateLayout = "2006-01-02"

// PlanDoc is one flat-rate plan's dated commercial shape — the S18.3
// per-person automation-budget data surface for a lane, with no dotted key by
// design (S18.3; the numbers are per-plan facts, not operator preferences).
type PlanDoc struct {
	Lane string `json:"lane"`
	Plan string `json:"plan"`
	// Unit is the plan's OWN unit, named honestly. It is data because it
	// changed: this plan counted prompts when S10.4 was written and counts
	// credits now.
	Unit       string `json:"unit"`
	UnitNote   string `json:"unit_note,omitempty"`
	VerifiedOn string `json:"verified_on"`
	Source     string `json:"source"`
	// AssumedNote is the reason this whole document is labeled "assumed" — it
	// rides every reading derived from it (S10.4; G1 Def.10).
	AssumedNote string `json:"assumed_note"`

	WeeklyReset     string `json:"weekly_reset,omitempty"`
	WeeklyResetNote string `json:"weekly_reset_note,omitempty"`

	// MonthlyPoolNote records a window that exists and is deliberately NOT
	// seeded as a quota row, with the reason.
	MonthlyPoolNote string `json:"monthly_pool_note,omitempty"`
	// TierMultiplierNote records per-tier allowance multipliers that are real
	// and UNVERIFIED at primary-source grade, so nobody later mistakes their
	// absence for an oversight.
	TierMultiplierNote string `json:"tier_multiplier_note,omitempty"`

	// Pool names the shared quota pool this document's allowance IS, when more
	// than one lane draws it; PoolLanes are the lanes that draw it, and
	// PoolNote carries the vendor's own dated words for the claim.
	//
	// A pool is ONE document with several members, never several documents:
	// a second document would BE a second allowance, which is the exact
	// misreading the concept exists to prevent. Consumption is SUMMED across
	// the members — without that each lane reports a half-empty pool and
	// routing steers work onto an allowance that is already spent.
	//
	// Absent members mean the pre-packet behavior exactly: a document with no
	// pool serves its own lane alone (the PlanQuota.Unit precedent), which is
	// why the unpooled plan is expressible unchanged.
	//
	// The document's own Lane stays the pool's CANONICAL lane — the one a
	// budget may be declared against. See PlanPoolRefusal.
	Pool      string   `json:"pool,omitempty"`
	PoolLanes []string `json:"pool_lanes,omitempty"`
	PoolNote  string   `json:"pool_note,omitempty"`

	Multipliers []PlanMultiplier `json:"multipliers"`
	Quotas      []PlanQuota      `json:"quotas"`
}

// CountsLane reports whether consumption on a lane draws THIS document's
// allowance. An unpooled document counts its own lane and nothing else.
func (d PlanDoc) CountsLane(lane string) bool {
	if lane == d.Lane {
		return true
	}
	for _, member := range d.PoolLanes {
		if member == lane {
			return true
		}
	}
	return false
}

// poolMembers reports the lanes drawing this document's allowance, canonical
// lane first, or nothing when the document is unpooled.
func (d PlanDoc) poolMembers() []string {
	if d.Pool == "" {
		return nil
	}
	out := make([]string, 0, len(d.PoolLanes)+1)
	out = append(out, d.Lane)
	for _, member := range d.PoolLanes {
		if member != d.Lane {
			out = append(out, member)
		}
	}
	return out
}

// PlanMultiplier is one charging-rate window. Exactly one row is the DEFAULT:
// the rate that applies to every instant no named window claims. Without it a
// gap in the windows would silently charge nothing.
type PlanMultiplier struct {
	Name   string  `json:"name"`
	Factor float64 `json:"factor"`
	// Days are the weekday abbreviations the window covers ("Mon".."Sun");
	// empty means every day.
	Days []string `json:"days,omitempty"`
	// FromHour/ToHour bound the window [from, to) in the provider's own
	// timezone, expressed as UTCOffsetMinutes.
	FromHour         int  `json:"from_hour,omitempty"`
	ToHour           int  `json:"to_hour,omitempty"`
	UTCOffsetMinutes int  `json:"utc_offset_minutes,omitempty"`
	Default          bool `json:"default,omitempty"`

	VerifiedOn string `json:"verified_on"`
	Source     string `json:"source,omitempty"`
	Note       string `json:"note,omitempty"`
}

// PlanQuota is one allowance window: how many plan units the account holds
// over how long.
type PlanQuota struct {
	Name string `json:"name"`
	// Unit is THIS WINDOW's own unit, empty to inherit the document's.
	//
	// It is per-quota because one lane's windows are denominated differently:
	// the Kimi Code plan's rolling 5-hour window counts REQUESTS and its 7-day
	// window counts CREDITS. A single lane-wide scalar cannot describe that,
	// and rendering both gauges in one unit would be a quiet lie about what
	// was counted. Empty inherits, so a document whose windows share a unit
	// (the zai plan) is expressible exactly as it was.
	Unit        string  `json:"unit,omitempty"`
	Units       float64 `json:"units"`
	WindowHours float64 `json:"window_hours"`
	// AllowanceUnverified says this window's SHAPE is published and its
	// ALLOWANCE is not — so Units is 0 and means "nobody published one",
	// never "the allowance is nothing".
	//
	// It exists because that is the honest state of Kimi's 7-day window: the
	// cycle is published verbatim and the per-tier figures circulate only on
	// secondary aggregators. The alternative was to seed a number this
	// platform cannot source, and a budget proposed from an invented allowance
	// is the inferred provider window S10.4/D4 forbids by name.
	AllowanceUnverified bool   `json:"allowance_unverified,omitempty"`
	VerifiedOn          string `json:"verified_on"`
	Source              string `json:"source,omitempty"`
	Note                string `json:"note,omitempty"`
}

// PlanWindow is one declared allowance window as a READING reports it, with its
// own unit, so no surface can render a credit window in requests.
type PlanWindow struct {
	Name                string
	Unit                string
	Allowance           float64
	WindowHours         float64
	AllowanceUnverified bool
}

// PlanReading is one tier-3 plan-unit reading for a (person, lane). It is
// deliberately a DIFFERENT type from Gauge: the two readings share a run and
// share nothing else, and giving them one struct is how they get added
// together by somebody who did not read this file.
type PlanReading struct {
	UserID string
	Lane   string
	Plan   string
	// Unit is the plan's own unit, carried onto every surface so a reader is
	// never left to guess what was counted.
	Unit string
	// Tier is always TierDerived: this reading is a proxy, by construction.
	Tier ApproximationTier
	// Assumed and AssumedNote carry the S10.4 label and its reason.
	Assumed     bool
	AssumedNote string
	VerifiedOn  string
	Source      string

	// SeedQuota / SeedAllowance / SeedWindowHours record the PROVIDER's
	// published allowance this lane's budget was proposed from. They are
	// PROVENANCE and never the denominator: S10.4 is explicit that the
	// denominator is Sinet's own operator-declared budget and "NEVER an
	// inferred provider window (D4)". Reporting them lets a person see what a
	// budget was seeded from without the reading resting on it.
	SeedQuota       string
	SeedAllowance   float64
	SeedWindowHours float64

	// Budget is the operator-declared denominator. Undeclared ⇒ Applicable is
	// false and Pressure is 0, exactly as the token gauge behaves.
	Budget PlanBudget

	// Calls is the number of requests in the budget's period — the proxy's
	// input. Consumed is those requests with each one's multiplier applied.
	Calls    int64
	Consumed float64

	// Multiplier and MultiplierWindow are the rate in force AT the reading's
	// instant, reported so a person can see why the number moves.
	Multiplier       float64
	MultiplierWindow string

	// Windows are the plan's declared allowance windows, each with its OWN
	// unit. A lane whose windows are denominated differently cannot be
	// rendered from one scalar, and a surface that tried would print credits
	// under a "requests" label.
	Windows []PlanWindow

	// Pressure is Consumed/Budget.PeriodUnits, valid only when Applicable;
	// BackgroundCeiling is ⚙ budget.background_window_fraction of the declared
	// budget — background work's own ceiling on this lane (S10.4).
	// Pool and PoolLanes name the shared allowance this reading covers, when
	// the lane draws one. Lane above stays the REQUESTING lane: the reading is
	// about that lane, taken against a pool it shares. Both empty on an
	// unpooled lane, which is every lane that predates the concept.
	Pool      string
	PoolLanes []string

	Pressure          float64
	BackgroundCeiling float64
	Applicable        bool

	// InapplicableNote says why a DECLARED budget still produced no ratio.
	// Empty when the budget applied, and empty when none was declared at all —
	// that absence has its own answer and needs no sentence here. It exists
	// because a stored row that cannot denominate this reading must be visibly
	// refused rather than silently skipped: a row somebody inserted by hand
	// must never steer dispatch, and a reader must be able to see that it did
	// not (P3-LN-6 drain r1 D1).
	InapplicableNote string
}

// PlanBudget is the operator-declared automation budget for one (person, lane)
// in the PLAN's own unit — the D5 flat-rate denominator of the tier-3 reading,
// exactly as Budget is for the tier-1 token gauge.
//
// It exists because the provider's published allowance is the WRONG
// denominator, and S10.4 says so by name: the denominator is Sinet's own
// budget, "NEVER an inferred provider window (D4)". Dividing by the vendor's
// number would produce a pressure figure that moves when the vendor changes
// its marketing, and would quietly re-import the provider's own opinion of how
// much a person may work — the exact import D4 refuses.
//
// The allowance still has a job: it SEEDS a proposal (ProposePlanBudget) at a
// conservative fraction of itself, which an operator accepts, edits or
// ignores. Until somebody declares one, the reading is honestly inapplicable.
type PlanBudget struct {
	// PeriodUnits is the budget in the plan's own unit — NOT the provider's
	// published allowance.
	PeriodUnits float64
	// PeriodStart is the instant consumption is summed from; PeriodHours is
	// the length the operator declared it for, and the reading STOPS APPLYING
	// once it has elapsed — a budget whose period ran out is not a budget for
	// the current one, and re-declaring is the act that starts the next
	// (planPeriodEnded; nothing rolls one over). This is Sinet's period, not
	// the provider's cycle: the provider's weekly window is order-anchored per
	// account and stays a recorded dated fact, never a period used here.
	PeriodStart time.Time
	PeriodHours float64
	Declared    bool

	// Lane is the lane the row was DECLARED on, which is not always the lane a
	// reading is taken for: a pooled allowance is declared once against the
	// pool's canonical lane and read by every member. It is on the value so the
	// reading can refuse a row PLANTED on a non-canonical lane while still
	// applying the canonical one — a distinction the requesting lane alone
	// cannot make.
	Lane string

	// SeededFrom names WHICH allowance window this budget denominates, and
	// Fraction records what share of that window's published allowance a
	// proposal took.
	//
	// Fraction is provenance for a person reading their own budget and is never
	// an input to the arithmetic. SeededFrom is NOT merely provenance and this
	// comment used to say it was (corrected 2026-08-25, drain r1 D1): the
	// reading resolves the seed allowance through it AND refuses to apply a
	// budget whose window cannot denominate this lane's consumption through it
	// (PlanBudgetWindowRefusal). It names a window; it is not a number that
	// enters a division.
	SeededFrom string
	Fraction   float64
}

// UndeclaredPlanBudget is the v0 posture: no operator plan budget (S10.4).
func UndeclaredPlanBudget() PlanBudget { return PlanBudget{} }

// SeedPlanDocs returns the plan documents that ship with the platform. They are
// seed DATA with dates — a starting point an operator's own rows replace — and
// never the authority.
func SeedPlanDocs() ([]PlanDoc, error) {
	return loadPlanDocs(planSeeds, "plandata")
}

// loadPlanDocs walks a directory of plan documents, validates each, and returns
// them SORTED BY LANE NAME. A duplicate lane is refused BY NAME: PlanDocFor
// takes the first match, so a second document for one lane would be silently
// unreachable.
func loadPlanDocs(fsys fs.FS, dir string) ([]PlanDoc, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("%w: read %s: %w", ErrPlanDoc, dir, err)
	}
	var out []PlanDoc
	byLane := map[string]string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		path := dir + "/" + name
		raw, err := fs.ReadFile(fsys, path)
		if err != nil {
			return nil, fmt.Errorf("%w: read %s: %w", ErrPlanDoc, path, err)
		}
		d, err := LoadPlanDoc(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		for _, member := range d.poolMembers() {
			if first, dup := byLane[member]; dup && first != path {
				return nil, fmt.Errorf("%w: %s and %s both count lane %q — the plan lookup takes the FIRST match, so "+
					"one of them would be silently unreachable", ErrPlanDoc, first, path, member)
			}
			byLane[member] = path
		}
		if first, dup := byLane[d.Lane]; dup && first != path {
			return nil, fmt.Errorf("%w: %s and %s both declare lane %q — the plan lookup takes the FIRST match, "+
				"so the second document would be silently unreachable", ErrPlanDoc, first, path, d.Lane)
		}
		byLane[d.Lane] = path
		out = append(out, d)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: %s holds no plan documents", ErrPlanDoc, dir)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Lane < out[j].Lane })
	return out, nil
}

// QuotaUnit is the unit a named window counts in — its OWN when it declares one,
// the document's otherwise (OQ2).
//
// The fallback is what keeps a single-unit plan expressible exactly as it was:
// a document whose windows share a unit declares it once and no row repeats it.
func (d PlanDoc) QuotaUnit(name string) string {
	if q, ok := d.Quota(name); ok && q.Unit != "" {
		return q.Unit
	}
	return d.Unit
}

// windows renders the declared allowance windows with their own units, for a
// reading to carry onto every surface.
func (d PlanDoc) windows() []PlanWindow {
	if len(d.Quotas) == 0 {
		return nil
	}
	out := make([]PlanWindow, 0, len(d.Quotas))
	for _, q := range d.Quotas {
		out = append(out, PlanWindow{
			Name: q.Name, Unit: d.QuotaUnit(q.Name), Allowance: q.Units,
			WindowHours: q.WindowHours, AllowanceUnverified: q.AllowanceUnverified,
		})
	}
	return out
}

// PlanDocFor returns the seed plan document for a lane, if one ships.
func PlanDocFor(lane string) (PlanDoc, bool) {
	docs, err := SeedPlanDocs()
	if err != nil {
		return PlanDoc{}, false
	}
	for _, d := range docs {
		// Membership, not equality: a pooled lane resolves to the pool's own
		// document. This single predicate reaches every production caller —
		// the store's Declare, the router's pressure read, the meters surface,
		// the boundary's window list and the proposal — so the pool cannot be
		// honored in one place and forgotten in another.
		if d.CountsLane(lane) {
			return d, true
		}
	}
	return PlanDoc{}, false
}

// PoolBudgetLane maps a lane to the lane its plan budget is DECLARED on.
//
// For an unpooled lane that is itself. For a pooled one it is the pool's
// canonical lane, because one allowance carries one budget row: reading a
// sibling lane's own key would find nothing and report a lane with no budget as
// a lane with no consumption to bound.
func PoolBudgetLane(lane string) string {
	doc, ok := PlanDocFor(lane)
	if !ok || doc.Pool == "" {
		return lane
	}
	return doc.Lane
}

// PlanPoolRefusal reports why a plan budget may not be declared on a lane,
// naming the pool's canonical lane. Empty means the declaration is allowed.
//
// plan_budgets is keyed (user_id, lane, window), so two lanes sharing one
// allowance could carry two independent rows against it — and under max-binds
// either could bind the lane, with the operator unable to tell why the number
// they did not touch moved. The rule is: a pooled allowance is declared ONCE,
// against the document's own lane.
//
// It is a predicate rather than a branch because it is enforced at three
// layers — the store refuses the write, the HTTP boundary carries the same
// verdict rather than re-deriving it, and the reading refuses to apply a row
// inserted by any other route. A rule spelled three times drifts; a rule
// computed once and carried does not.
func PlanPoolRefusal(doc PlanDoc, lane string) string {
	if doc.Pool == "" || lane == doc.Lane || !doc.CountsLane(lane) {
		return ""
	}
	return fmt.Sprintf("lane %q draws the shared %q allowance, which is declared ONCE against lane %q — "+
		"declaring it here as well would put two independent budget rows against one allowance, and either "+
		"could bind the lane (S10.4)", lane, doc.Pool, doc.Lane)
}

// LoadPlanDoc parses and VALIDATES one plan document. A denominator nobody can
// date is a denominator nobody can audit, so the dates are gates rather than
// decoration.
func LoadPlanDoc(raw []byte) (PlanDoc, error) {
	var d PlanDoc
	if err := json.Unmarshal(raw, &d); err != nil {
		return PlanDoc{}, fmt.Errorf("%w: %w", ErrPlanDoc, err)
	}
	if err := d.validate(); err != nil {
		return PlanDoc{}, err
	}
	return d, nil
}

func (d PlanDoc) validate() error {
	switch {
	case d.Lane == "":
		return fmt.Errorf("%w: no lane", ErrPlanDoc)
	case d.Unit == "":
		return fmt.Errorf("%w (lane %q): no unit — a reading whose unit is unknown cannot be labeled honestly (S10.4)", ErrPlanDoc, d.Lane)
	case d.AssumedNote == "":
		return fmt.Errorf("%w (lane %q): no assumed-note — the S10.4 label must carry its reason", ErrPlanDoc, d.Lane)
	case len(d.Quotas) == 0:
		return fmt.Errorf("%w (lane %q): no quota rows — there is no denominator", ErrPlanDoc, d.Lane)
	case len(d.Multipliers) == 0:
		return fmt.Errorf("%w (lane %q): no multiplier rows", ErrPlanDoc, d.Lane)
	}
	if err := planDate(d.Lane, "document", d.VerifiedOn); err != nil {
		return err
	}
	defaults := 0
	for _, m := range d.Multipliers {
		if m.Name == "" {
			return fmt.Errorf("%w (lane %q): a multiplier row has no name", ErrPlanDoc, d.Lane)
		}
		if m.Factor <= 0 {
			return fmt.Errorf("%w (lane %q): multiplier %q declares factor %v — a charging rate is positive, and a zero rate would report a lane that consumes nothing",
				ErrPlanDoc, d.Lane, m.Name, m.Factor)
		}
		if err := planDate(d.Lane, "multiplier "+m.Name, m.VerifiedOn); err != nil {
			return err
		}
		if m.Default {
			defaults++
			continue
		}
		if m.FromHour < 0 || m.ToHour > 24 || m.FromHour >= m.ToHour {
			return fmt.Errorf("%w (lane %q): multiplier %q declares the window [%d,%d), which covers no time",
				ErrPlanDoc, d.Lane, m.Name, m.FromHour, m.ToHour)
		}
	}
	// Exactly one default: with none, an instant outside every named window
	// would be charged at no rate at all and the gauge would under-report the
	// plan; with two, which one applied would depend on row order.
	if defaults != 1 {
		return fmt.Errorf("%w (lane %q): %d multiplier rows are marked default, want exactly 1 — every instant no named window claims needs a rate (S10.4)",
			ErrPlanDoc, d.Lane, defaults)
	}
	seen := map[string]bool{}
	for _, q := range d.Quotas {
		switch {
		case q.Name == "":
			return fmt.Errorf("%w (lane %q): a quota row has no name", ErrPlanDoc, d.Lane)
		case seen[q.Name]:
			return fmt.Errorf("%w (lane %q): quota %q is listed twice", ErrPlanDoc, d.Lane, q.Name)
		case q.Units <= 0 && !q.AllowanceUnverified:
			return fmt.Errorf("%w (lane %q): quota %q declares %v units — an allowance of nothing is not an allowance; "+
				"a window whose allowance nobody published says so with allowance_unverified", ErrPlanDoc, d.Lane, q.Name, q.Units)
		case q.Units > 0 && q.AllowanceUnverified:
			// Both at once is a contradiction, and the dangerous half is the
			// number: a figure carried under an "unverified" flag is the one a
			// surface would print anyway.
			return fmt.Errorf("%w (lane %q): quota %q declares %v units AND allowance_unverified — one of the two is "+
				"a lie, and a budget proposed from a number nobody published is the inferred provider window D4 forbids",
				ErrPlanDoc, d.Lane, q.Name, q.Units)
		case q.WindowHours <= 0:
			return fmt.Errorf("%w (lane %q): quota %q declares a %v-hour window", ErrPlanDoc, d.Lane, q.Name, q.WindowHours)
		}
		if err := planDate(d.Lane, "quota "+q.Name, q.VerifiedOn); err != nil {
			return err
		}
		seen[q.Name] = true
	}
	return d.validatePool()
}

// validatePool refuses a pool that cannot be read honestly. Each gate closes a
// way the concept could look declared and mean nothing.
func (d PlanDoc) validatePool() error {
	if d.Pool == "" {
		if len(d.PoolLanes) > 0 {
			return fmt.Errorf("%w (lane %q): pool_lanes %v with no pool name — the members would be summed into an "+
				"allowance nothing identifies", ErrPlanDoc, d.Lane, d.PoolLanes)
		}
		return nil
	}
	if len(d.PoolLanes) < 2 {
		return fmt.Errorf("%w (lane %q): pool %q declares %d member lanes — a pool with fewer than two members is "+
			"an ordinary allowance wearing a pool's name", ErrPlanDoc, d.Lane, d.Pool, len(d.PoolLanes))
	}
	if d.PoolNote == "" {
		return fmt.Errorf("%w (lane %q): pool %q carries no note — a shared-quota claim with no source is an "+
			"assertion, and this one decides whether two lanes read as one allowance or two", ErrPlanDoc, d.Lane, d.Pool)
	}
	seen := map[string]bool{}
	canonical := false
	for _, member := range d.PoolLanes {
		switch {
		case member == "":
			return fmt.Errorf("%w (lane %q): pool %q lists an empty member lane", ErrPlanDoc, d.Lane, d.Pool)
		case seen[member]:
			return fmt.Errorf("%w (lane %q): pool %q lists lane %q twice", ErrPlanDoc, d.Lane, d.Pool, member)
		}
		seen[member] = true
		if member == d.Lane {
			canonical = true
		}
	}
	// The document's own lane must be a member, because it is the CANONICAL
	// one: PlanPoolRefusal points every declaration at it, and a pool whose
	// canonical lane does not draw it would send budgets to a lane that spends
	// nothing.
	if !canonical {
		return fmt.Errorf("%w (lane %q): pool %q does not list its own document's lane among %v — the canonical "+
			"lane is where a budget for this allowance is declared", ErrPlanDoc, d.Lane, d.Pool, d.PoolLanes)
	}
	return nil
}

func planDate(lane, what, on string) error {
	if on == "" {
		return fmt.Errorf("%w (lane %q): %s carries no verified-on date — these numbers move, so an undated one is unusable", ErrPlanDoc, lane, what)
	}
	if _, err := time.Parse(planDateLayout, on); err != nil {
		return fmt.Errorf("%w (lane %q): %s verified-on %q is not a %s date", ErrPlanDoc, lane, what, on, planDateLayout)
	}
	return nil
}

// Quota returns the named allowance row.
func (d PlanDoc) Quota(name string) (PlanQuota, bool) {
	for _, q := range d.Quotas {
		if q.Name == name {
			return q, true
		}
	}
	return PlanQuota{}, false
}

// MultiplierAt reports the charging rate in force at an instant, and the name
// of the window that set it. The first named window that claims the instant
// wins; otherwise the default row does.
func (d PlanDoc) MultiplierAt(t time.Time) (float64, string) {
	for _, m := range d.Multipliers {
		if m.Default || !m.covers(t) {
			continue
		}
		return m.Factor, m.Name
	}
	for _, m := range d.Multipliers {
		if m.Default {
			return m.Factor, m.Name
		}
	}
	// validate() makes this unreachable for a loaded document; a hand-built
	// zero value charges at the standard rate rather than at nothing.
	return 1, ""
}

// covers reports whether a named window claims an instant, in the provider's
// own timezone.
func (m PlanMultiplier) covers(t time.Time) bool {
	local := t.In(time.FixedZone("plan", m.UTCOffsetMinutes*60))
	if len(m.Days) > 0 {
		day := local.Format("Mon")
		ok := false
		for _, d := range m.Days {
			if strings.EqualFold(d, day) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	h := local.Hour()
	return h >= m.FromHour && h < m.ToHour
}

// PlanBudgetWindowRefusal reports why a window cannot carry an automation
// budget, or "" when it can. It is the ONE definition of that rule: the store
// refuses a row by it, the reading refuses to apply one by it, and the HTTP
// boundary refuses the input by it through the seam — one predicate rather than
// three spellings that can drift apart (the §65 D4 lesson).
//
// Two conditions, and both are about the ARITHMETIC rather than about taste.
//
//  1. The window must be one the document declares. A budget for a window the
//     plan does not have denominates nothing.
//
//  2. The window must count what CONSUMPTION IS COUNTED IN, which is the
//     document's own unit: planUnits sums each request's charging multiplier,
//     and those multipliers are expressed in the document's unit. One shipped
//     plan counts REQUESTS and publishes its 7-day allowance in CREDITS, so a
//     weekly budget there would divide requests by credits and report the
//     answer labeled "credits" — a number wrong in both units. Under the
//     most-constrained-window rule that incoherent ratio could BIND the lane
//     and steer dispatch, which is why this is refused at every layer rather
//     than documented as a caveat.
//
// The window still rides every reading and every meters payload with its OWN
// unit (PlanReading.Windows): reporting an allowance is not denominating
// against it, and this rule narrows only the second.
func PlanBudgetWindowRefusal(doc PlanDoc, window string) string {
	q, ok := doc.Quota(window)
	if !ok {
		return fmt.Sprintf("the %s plan declares no %q window, so a budget for it denominates nothing", doc.Lane, window)
	}
	if unit := doc.QuotaUnit(q.Name); unit != doc.Unit {
		return fmt.Sprintf("the %s plan's %q window is denominated in %s while this lane's consumption is counted in %s, "+
			"so a budget on it would divide %s by %s and report the answer in neither — the window's allowance is still "+
			"reported, it just cannot be a denominator (S10.4/D4)",
			doc.Lane, q.Name, unit, doc.Unit, doc.Unit, unit)
	}
	return ""
}

// planPeriodEnded reports whether a declared period has run out at `now`.
//
// period_hours is the length the operator declared the budget FOR, and a budget
// whose period has elapsed is not a budget for the current one: counting a
// month of consumption against a five-hour allowance would report a pressure
// that only rises, shed background work forever, and call that headroom.
// Nothing rolls the period over — that would be a timer, and dueness comes from
// stored state, never from a ticker (CONVENTIONS §34) — so an expired row is
// honestly inapplicable until the operator declares the next period, exactly as
// migration 0017 records for the token budget.
func planPeriodEnded(budget PlanBudget, now time.Time) bool {
	if budget.PeriodStart.IsZero() || budget.PeriodHours <= 0 || now.IsZero() {
		return false
	}
	return now.After(budget.PeriodStart.Add(time.Duration(budget.PeriodHours * float64(time.Hour))))
}

// ProposePlanBudget turns a provider allowance row into a PROPOSED per-person
// automation budget: a conservative ⚙ budget.background_window_fraction of the
// published allowance, in the plan's own unit, labeled assumed by the document
// it came from.
//
// This is the only sanctioned use of the published allowance, and it is a
// PROPOSAL. Nothing on a read path calls it: the platform never declares a
// budget on somebody's behalf, because a denominator nobody chose is exactly
// the inferred provider window D4 bars. An operator accepts, edits or ignores
// it through the 13.4 surface, and until one is declared every reading is
// honestly inapplicable.
func (g *PressureGauge) ProposePlanBudget(doc PlanDoc, quota string, start time.Time) (PlanBudget, error) {
	q, ok := doc.Quota(quota)
	if !ok {
		return PlanBudget{}, fmt.Errorf("%w (lane %q): no quota row %q", ErrPlanDoc, doc.Lane, quota)
	}
	if q.AllowanceUnverified {
		// There is nothing to take a fraction OF. Proposing from a window whose
		// allowance nobody published would mint the denominator out of thin air,
		// which is worse than having none: an operator would accept a number
		// that looks sourced (S10.4/D4).
		return PlanBudget{}, fmt.Errorf("%w (lane %q): quota %q publishes no allowance at primary-source grade, so no "+
			"budget can be proposed from it — the operator's own account console closes it (P-T17-3)", ErrPlanDoc, doc.Lane, quota)
	}
	fraction, err := g.settings.Float(keyBgWindowFraction)
	if err != nil {
		return PlanBudget{}, fmt.Errorf("metering: read ⚙ %s: %w", keyBgWindowFraction, err)
	}
	return PlanBudget{
		PeriodUnits: q.Units * fraction,
		PeriodStart: start,
		PeriodHours: q.WindowHours,
		Declared:    true,
		SeededFrom:  q.Name,
		Fraction:    fraction,
	}, nil
}

// ReadPlanUnits derives the tier-3 plan-unit reading for (userID, lane)
// against the operator's declared plan budget.
//
// The denominator is the BUDGET and never the provider's published allowance
// (S10.4, verbatim: "never an inferred provider window (D4)"). The allowance
// rides the reading as provenance so a person can see what their budget was
// proposed from; with no budget declared the reading reports its consumption
// and says Applicable=false, which is what the token gauge does one file over
// and for the same reason.
//
// The proxy is the REQUEST, because that is what the plan meters and what the
// platform can count exactly; each request is charged at the multiplier in
// force when it was made, not at the one in force now, because a window that
// closed mid-run charged what it charged.
func (g *PressureGauge) ReadPlanUnits(ctx context.Context, userID, lane string, doc PlanDoc, budget PlanBudget, now time.Time) (PlanReading, error) {
	bgFraction, err := g.settings.Float(keyBgWindowFraction)
	if err != nil {
		return PlanReading{}, fmt.Errorf("metering: read ⚙ %s: %w", keyBgWindowFraction, err)
	}
	calls, consumed, err := g.planUnits(ctx, userID, lane, doc, budget.PeriodStart, budget.Declared, now)
	if err != nil {
		return PlanReading{}, err
	}
	factor, window := doc.MultiplierAt(now)
	// The reading's unit is the unit CONSUMPTION IS COUNTED IN — the document's
	// own, which the charging multipliers are expressed in. It used to follow
	// the budget's window instead, which was only ever the same answer for a
	// window that can actually denominate this reading, and was a MISLABEL for
	// one that cannot: a credit-denominated weekly budget on a request-counting
	// plan made this say "credits" over a
	// requests count (corrected 2026-08-25, P3-LN-6 drain r1 D1). Each declared
	// window still rides Windows with its own unit, which is where a surface
	// reads what each allowance counts.
	unit := doc.Unit
	r := PlanReading{
		UserID: userID, Lane: lane, Plan: doc.Plan,
		Unit:        unit,
		Windows:     doc.windows(),
		Tier:        TierDerived,
		Assumed:     true,
		AssumedNote: doc.AssumedNote,
		VerifiedOn:  doc.VerifiedOn,
		Source:      doc.Source,

		Budget: budget,
		Calls:  calls, Consumed: consumed,
		Multiplier: factor, MultiplierWindow: window,

		// A combined figure that does not say what it combined invites exactly
		// the wrong reading: somebody sees a lane at 80% and goes looking for
		// 80% of that lane's own traffic. Lane stays the REQUESTING lane;
		// these name the pool the number actually covers.
		Pool:      doc.Pool,
		PoolLanes: doc.poolMembers(),
	}
	// The allowance the budget was proposed from, reported as provenance.
	if q, ok := doc.Quota(budget.SeededFrom); ok {
		r.SeedQuota, r.SeedAllowance, r.SeedWindowHours = q.Name, q.Units, q.WindowHours
	}
	if budget.Declared && budget.PeriodUnits > 0 {
		// A declared budget still has to be one this reading can divide by. Both
		// refusals are DEFENCE IN DEPTH: the store and the verb refuse the same
		// rows, and this leg is what stops a row inserted by any other means
		// from steering dispatch (drain r1 D1/D6).
		switch refusal, poolRefusal := PlanBudgetWindowRefusal(doc, budget.SeededFrom), PlanPoolRefusal(doc, budget.Lane); {
		case poolRefusal != "":
			// A row planted on a pooled lane other than the canonical one, by
			// whatever route. Refusing it only at the write would leave the
			// rule walkable-around; this is the leg that stops it steering
			// dispatch.
			r.InapplicableNote = poolRefusal
		case refusal != "":
			r.InapplicableNote = refusal
		case planPeriodEnded(budget, now):
			r.InapplicableNote = fmt.Sprintf("the declared %v-hour period started %s and has ended, so it is not a budget for the "+
				"current one; re-declaring is the act that starts the next period (S10.4; nothing rolls one over)",
				budget.PeriodHours, budget.PeriodStart.UTC().Format(time.RFC3339))
		default:
			r.Applicable = true
			r.Pressure = consumed / budget.PeriodUnits
			r.BackgroundCeiling = bgFraction * budget.PeriodUnits
		}
	}
	return r, nil
}

// planUnits counts the person's requests on the lane in the budget's period
// and charges each at its own instant's rate.
//
// The period filter runs in GO, not in SQL. A string comparison on created_ts
// silently drops any row whose stamp is not the expected shape — the call
// simply vanishes from the total, which UNDER-reports consumption, the one
// direction a consumption gauge must never fail in. Parsing every candidate
// costs a scan of one person's rows and cannot lose one.
func (g *PressureGauge) planUnits(ctx context.Context, userID, lane string, doc PlanDoc, since time.Time, sincePinned bool, now time.Time) (int64, float64, error) {
	rows, err := g.db.QueryContext(ctx,
		`SELECT c.usage_json, r.lane, c.created_ts FROM checkpoints c JOIN runs r ON r.run_id = c.run_id
		  WHERE r.user_id = ?`, userID)
	if err != nil {
		return 0, 0, fmt.Errorf("metering: read plan-unit consumption for %q/%s: %w", userID, lane, err)
	}
	defer rows.Close()
	var (
		calls    int64
		consumed float64
	)
	for rows.Next() {
		var usage, runLane, ts string
		if err := rows.Scan(&usage, &runLane, &ts); err != nil {
			return 0, 0, fmt.Errorf("metering: scan plan-unit consumption: %w", err)
		}
		// Pool MEMBERSHIP, not lane equality. The SQL already selects every
		// lane for this person, so the whole pool test lives here: a lane that
		// shares an allowance must count what its siblings spent, or each side
		// reports a half-empty pool and routing steers work onto an allowance
		// that is already gone. An unpooled document counts its own lane and
		// nothing else, exactly as before.
		if !doc.CountsLane(effectiveLane(runLane, json.RawMessage(usage))) {
			continue
		}
		at, perr := time.Parse(time.RFC3339Nano, ts)
		if perr != nil {
			// An unreadable stamp cannot be placed in or out of the period, so
			// the call is COUNTED and charged at the plan's standard (most
			// expensive) rate. Both halves are the safe direction: excluding it
			// would hide consumption, and charging it at the discount would
			// under-report it — and the discount applies to most of the week,
			// so "whatever rate is in force now" is the wrong default.
			calls++
			consumed += doc.StandardMultiplier()
			continue
		}
		if sincePinned && !since.IsZero() && at.Before(since) {
			continue
		}
		// The period has an UPPER bound too. Moving the filter into Go for the
		// unreadable-stamp case dropped the one SQL was enforcing, and a
		// future-dated row — a clock skew, a restored backup, a bad import —
		// would then count against a period it does not belong to and inflate
		// the current reading (drain r2 R2).
		if !now.IsZero() && at.After(now) {
			continue
		}
		factor, _ := doc.MultiplierAt(at)
		calls++
		consumed += factor
	}
	return calls, consumed, rows.Err()
}

// StandardMultiplier is the plan's most expensive published rate — the rate a
// discount is expressed as a fraction OF. It is derived from the rows rather
// than assumed to be 1.0, because which row is "standard" is the document's
// fact and not this file's.
func (d PlanDoc) StandardMultiplier() float64 {
	standard := 0.0
	for _, m := range d.Multipliers {
		if m.Factor > standard {
			standard = m.Factor
		}
	}
	if standard == 0 {
		return 1
	}
	return standard
}
