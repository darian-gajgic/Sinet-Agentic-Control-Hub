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
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

//go:embed plandata/zai.json
var zaiPlanSeed []byte

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

	Multipliers []PlanMultiplier `json:"multipliers"`
	Quotas      []PlanQuota      `json:"quotas"`
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
	Name        string  `json:"name"`
	Units       float64 `json:"units"`
	WindowHours float64 `json:"window_hours"`
	VerifiedOn  string  `json:"verified_on"`
	Source      string  `json:"source,omitempty"`
	Note        string  `json:"note,omitempty"`
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

	// Quota names the allowance window this reading measured against.
	Quota       string
	QuotaUnits  float64
	WindowHours float64

	// Calls is the number of requests in the window — the proxy's input.
	// Consumed is those requests with each one's multiplier applied.
	Calls    int64
	Consumed float64

	// Multiplier and MultiplierWindow are the rate in force AT the reading's
	// instant, reported so a person can see why the number moves.
	Multiplier       float64
	MultiplierWindow string

	// Pressure is Consumed/QuotaUnits; BackgroundCeiling is
	// ⚙ budget.background_window_fraction of the allowance — background work's
	// own ceiling on this lane (S10.4).
	Pressure          float64
	BackgroundCeiling float64
	Applicable        bool
}

// SeedPlanDocs returns the plan documents that ship with the platform. They are
// seed DATA with dates — a starting point an operator's own rows replace — and
// never the authority.
func SeedPlanDocs() ([]PlanDoc, error) {
	d, err := LoadPlanDoc(zaiPlanSeed)
	if err != nil {
		return nil, err
	}
	return []PlanDoc{d}, nil
}

// PlanDocFor returns the seed plan document for a lane, if one ships.
func PlanDocFor(lane string) (PlanDoc, bool) {
	docs, err := SeedPlanDocs()
	if err != nil {
		return PlanDoc{}, false
	}
	for _, d := range docs {
		if d.Lane == lane {
			return d, true
		}
	}
	return PlanDoc{}, false
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
		case q.Units <= 0:
			return fmt.Errorf("%w (lane %q): quota %q declares %v units — an allowance of nothing is not an allowance", ErrPlanDoc, d.Lane, q.Name, q.Units)
		case q.WindowHours <= 0:
			return fmt.Errorf("%w (lane %q): quota %q declares a %v-hour window", ErrPlanDoc, d.Lane, q.Name, q.WindowHours)
		}
		if err := planDate(d.Lane, "quota "+q.Name, q.VerifiedOn); err != nil {
			return err
		}
		seen[q.Name] = true
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

// ReadPlanUnits derives the tier-3 plan-unit reading for (userID, lane) over a
// named allowance window ending at now.
//
// The proxy is the REQUEST, because that is what the plan meters and what the
// platform can count exactly; each request is charged at the multiplier in
// force when it was made, not at the one in force now, because a window that
// closed mid-run charged what it charged.
func (g *PressureGauge) ReadPlanUnits(ctx context.Context, userID, lane string, doc PlanDoc, quota string, now time.Time) (PlanReading, error) {
	q, ok := doc.Quota(quota)
	if !ok {
		return PlanReading{}, fmt.Errorf("%w (lane %q): no quota row %q", ErrPlanDoc, doc.Lane, quota)
	}
	bgFraction, err := g.settings.Float(keyBgWindowFraction)
	if err != nil {
		return PlanReading{}, fmt.Errorf("metering: read ⚙ %s: %w", keyBgWindowFraction, err)
	}
	since := now.Add(-time.Duration(q.WindowHours * float64(time.Hour)))
	calls, consumed, err := g.planUnits(ctx, userID, lane, doc, since, now)
	if err != nil {
		return PlanReading{}, err
	}
	factor, window := doc.MultiplierAt(now)
	r := PlanReading{
		UserID: userID, Lane: lane, Plan: doc.Plan,
		Unit:        doc.Unit,
		Tier:        TierDerived,
		Assumed:     true,
		AssumedNote: doc.AssumedNote,
		VerifiedOn:  doc.VerifiedOn,
		Source:      doc.Source,

		Quota: q.Name, QuotaUnits: q.Units, WindowHours: q.WindowHours,
		Calls: calls, Consumed: consumed,
		Multiplier: factor, MultiplierWindow: window,
	}
	if q.Units > 0 {
		r.Applicable = true
		r.Pressure = consumed / q.Units
		r.BackgroundCeiling = bgFraction * q.Units
	}
	return r, nil
}

// planUnits counts the person's requests on the lane inside the window and
// charges each at its own instant's rate.
func (g *PressureGauge) planUnits(ctx context.Context, userID, lane string, doc PlanDoc, since, until time.Time) (int64, float64, error) {
	rows, err := g.db.QueryContext(ctx,
		`SELECT c.usage_json, r.lane, c.created_ts FROM checkpoints c JOIN runs r ON r.run_id = c.run_id
		  WHERE r.user_id = ? AND c.created_ts >= ? AND c.created_ts <= ?`,
		userID, since.UTC().Format(time.RFC3339Nano), until.UTC().Format(time.RFC3339Nano))
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
		if effectiveLane(runLane, json.RawMessage(usage)) != lane {
			continue
		}
		at, perr := time.Parse(time.RFC3339Nano, ts)
		if perr != nil {
			// An unreadable stamp charges at the standard rate rather than at
			// the discount: the safe direction for a consumption gauge is to
			// over-report, never to under-report an allowance.
			at = until
		}
		factor, _ := doc.MultiplierAt(at)
		calls++
		consumed += factor
	}
	return calls, consumed, rows.Err()
}
