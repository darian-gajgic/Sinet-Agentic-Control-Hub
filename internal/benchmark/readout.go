package benchmark

import (
	"context"
	"database/sql"
	"fmt"
)

// readout.go is BENCH-REG §15 and §16: small-n honesty as a product surface,
// and the reporting views over the record.
//
// The rule this file exists to make unbreakable (§15, S14.7): "There is no
// benchmark number without its epistemics." So the package exposes exactly ONE
// published-rate shape, PublishedRate, and it carries the win rate together
// with n, G, the tie and both-bad rates, the decline rate, the guess accuracy,
// the §5 blindness label when one is due, and the §15 honest-claims table
// itself. There is deliberately NO accessor anywhere in this package that
// returns a bare win rate — readout_test.go asserts that by AST, because a
// convenience accessor is exactly how a number ends up on a screen without its
// epistemics.
//
// §16's views are READOUTS. None of them is a decision input: §7 makes G the
// only decision statistic, and the gate and the alarm read it directly.

// Accrual is the raw record count for a (domain, epoch) scope. Ties and
// both-bads are counted and reported but are excluded from m and w and never
// enter the posterior (§8, frozen).
type Accrual struct {
	Domain  string `json:"domain"`
	EpochID string `json:"epoch_id,omitempty"`
	// Answered is every pair the requester answered — verdicts plus declines.
	// It is the decline rate's denominator: selective participation is measured
	// against what people were actually asked (§4.2.5).
	Answered int `json:"answered"`
	Declined int `json:"declined"`
	Wins     int `json:"wins"`
	Losses   int `json:"losses"`
	Ties     int `json:"ties"`
	BothBad  int `json:"both_bad"`
	// Verdicts is wins+losses+ties+both-bad: the pairs that produced a blind
	// pick. It is the tie/both-bad rates' denominator.
	Verdicts     int `json:"verdicts"`
	Guesses      int `json:"guesses"`
	GuessCorrect int `json:"guess_correct"`
}

// NonTied is m: the pairs that enter the posterior (§8).
func (a Accrual) NonTied() int { return a.Wins + a.Losses }

// Accrual counts a domain's recorded pairs. An empty epochID pools ALL epochs
// and is therefore valid ONLY for the §13 done-directly figure and the §16
// history views — never for a decision statistic, which §9 confines to the
// current epoch. The gate and the alarm always pass an epoch.
func (s *Store) Accrual(ctx context.Context, domain, epochID string) (Accrual, error) {
	var a Accrual
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		var err error
		a, err = s.accrualTx(ctx, tx, domain, epochID)
		return err
	})
	return a, err
}

func (s *Store) accrualTx(ctx context.Context, tx *sql.Tx, domain, epochID string) (Accrual, error) {
	a := Accrual{Domain: domain, EpochID: epochID}
	q := `SELECT verdict, guess, position, declined FROM benchmark_pairs
	       WHERE domain = ? AND state IN (?, ?)`
	args := []any{domain, StateRecorded, StateDeclined}
	if epochID != "" {
		q += ` AND epoch_id = ?`
		args = append(args, epochID)
	}
	rows, err := tx.QueryContext(ctx, q, args...)
	if err != nil {
		return a, fmt.Errorf("benchmark: accrual for %q: %w", domain, err)
	}
	defer rows.Close()
	for rows.Next() {
		var verdict, guess, position string
		var declined int
		if err := rows.Scan(&verdict, &guess, &position, &declined); err != nil {
			return a, fmt.Errorf("benchmark: scan accrual: %w", err)
		}
		a.Answered++
		if declined == 1 {
			a.Declined++
			continue
		}
		a.Verdicts++
		switch Choice(verdict) {
		case ChoiceA, ChoiceB:
			platformSide := SideA
			if position == positionPlatformB {
				platformSide = SideB
			}
			if Side(verdict) == platformSide {
				a.Wins++
			} else {
				a.Losses++
			}
		case ChoiceTie:
			a.Ties++
		case ChoiceBothBad:
			a.BothBad++
		}
		if guess != "" {
			a.Guesses++
			platformSide := SideA
			if position == positionPlatformB {
				platformSide = SideB
			}
			if Side(guess) == platformSide {
				a.GuessCorrect++
			}
		}
	}
	return a, rows.Err()
}

// PublishedRate is the ONE shape a benchmark rate is published in. Every field
// below the win rate is the epistemics §15 requires to travel with it: "Every
// published rate carries n, the posterior G, tie/both-bad rates, decline rate,
// and guess accuracy."
type PublishedRate struct {
	Domain  string `json:"domain"`
	EpochID string `json:"epoch_id"`

	// WinRate is w/m over NON-TIED pairs. N is m — the n every other figure is
	// reported at.
	WinRate float64 `json:"win_rate"`
	N       int     `json:"n"`
	W       int     `json:"w"`
	// G is the §7 posterior P(platform better); LossG is 1−G, the §12 alarm's
	// input. G is the only decision statistic.
	G     float64 `json:"g"`
	LossG float64 `json:"loss_g"`

	// TieRate / BothBadRate are over the pairs that produced a blind pick — a
	// tie is a verdict, not a non-event. A sustained high both-bad rate is a
	// product-quality signal with NO automatic decision consequence (§8).
	TieRate     float64 `json:"tie_rate"`
	BothBadRate float64 `json:"both_bad_rate"`
	// DeclineRate is declined ÷ answered: selective participation must be
	// visible, not silent (§4.2.5).
	DeclineRate float64 `json:"decline_rate"`
	// GuessAccuracy is the §5 blindness measurement, at GuessN votes.
	GuessAccuracy float64 `json:"guess_accuracy"`
	GuessN        int     `json:"guess_n"`
	// BlindnessLabel is §5's "partially unblinded (guess accuracy g%)" when
	// guess accuracy beats chance, and empty otherwise. The rate STILL
	// publishes either way — no result is ever suppressed for blindness
	// failure, it is labelled.
	BlindnessLabel string `json:"blindness_label,omitempty"`

	// The §15 obligation rides on the rate itself, so wherever B6 renders a
	// number the honest-claims table renders with it.
	HonestClaims        []HonestClaim `json:"honest_claims"`
	HonestClaimsReadout string        `json:"honest_claims_readout"`
	// ArmParity is the §6 declaration: declared, not hidden.
	ArmParity []string `json:"arm_parity"`
	// ParityNotes are the recorded per-pair parity gaps in scope — flagged, not
	// absorbed (§6). An empty slice is the ordinary case.
	ParityNotes []string `json:"parity_notes,omitempty"`
}

// PublishedRate computes the §15 published shape for a (domain, epoch). An
// empty epochID pools all epochs, which is legitimate ONLY for a §16 history
// view — the gate and alarm never call it that way.
func (p *Practice) PublishedRate(ctx context.Context, domain, epochID string) (PublishedRate, error) {
	a, err := p.Store.Accrual(ctx, domain, epochID)
	if err != nil {
		return PublishedRate{}, err
	}
	notes, err := p.parityNotes(ctx, domain, epochID)
	if err != nil {
		return PublishedRate{}, err
	}
	return publishedRate(a, notes)
}

func publishedRate(a Accrual, notes []string) (PublishedRate, error) {
	post, err := ComputeG(a.Wins, a.NonTied())
	if err != nil {
		return PublishedRate{}, err
	}
	r := PublishedRate{
		Domain: a.Domain, EpochID: a.EpochID,
		WinRate: post.WinRate, N: post.M, W: post.W, G: post.G, LossG: post.LossG,
		TieRate:      ratio(a.Ties, a.Verdicts),
		BothBadRate:  ratio(a.BothBad, a.Verdicts),
		DeclineRate:  ratio(a.Declined, a.Answered),
		GuessN:       a.Guesses,
		HonestClaims: HonestClaims(), HonestClaimsReadout: HonestClaimsReadout,
		ArmParity: ArmParityDeclaration, ParityNotes: notes,
	}
	r.GuessAccuracy = ratio(a.GuessCorrect, a.Guesses)
	if a.Guesses > 0 && r.GuessAccuracy > ChanceAccuracy {
		r.BlindnessLabel = PartiallyUnblindedLabel(r.GuessAccuracy)
	}
	return r, nil
}

func ratio(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return float64(n) / float64(d)
}

// parityNotes collects the §6 parity gaps recorded on the pairs in scope.
func (p *Practice) parityNotes(ctx context.Context, domain, epochID string) ([]string, error) {
	q := `SELECT parity_note FROM benchmark_pairs
	       WHERE domain = ? AND parity_note <> '' AND state IN (?, ?)`
	args := []any{domain, StateRecorded, StateDeclined}
	if epochID != "" {
		q += ` AND epoch_id = ?`
		args = append(args, epochID)
	}
	q += ` ORDER BY recorded_ts, pair_id`
	rows, err := p.Store.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("benchmark: parity notes for %q: %w", domain, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var note string
		if err := rows.Scan(&note); err != nil {
			return nil, err
		}
		out = append(out, note)
	}
	return out, rows.Err()
}

// EpochRate is one epoch's published rate in a history view.
type EpochRate struct {
	Epoch Epoch         `json:"epoch"`
	Rate  PublishedRate `json:"rate"`
}

// DomainReadout is the §16 per-domain view: the current epoch's published rate,
// the per-epoch history side by side, the gate evaluation, the alarm state, the
// §13 done-directly figure, and the registered values that produced them all —
// each marked read-only with its BENCH-REG §-ref.
type DomainReadout struct {
	Domain string `json:"domain"`
	// Phase is the §4.1 sampling phase the domain is in.
	Phase        string             `json:"phase"`
	CurrentEpoch string             `json:"current_epoch"`
	Current      PublishedRate      `json:"current"`
	History      []EpochRate        `json:"history"`
	Gate         GateEvaluation     `json:"gate"`
	Alarm        AlarmState         `json:"alarm"`
	DoneDirectly DirectUseAggregate `json:"done_directly"`
	// Registered is every frozen value this readout depends on, marked
	// "registered — changing this value requires a re-registration commit"
	// (S18.4 R9). None of them is a settings row.
	Registered []RegisteredValue `json:"registered"`
	// LowAccrualNote is BENCH-REG §11's last paragraph, surfaced as a NOTE and
	// nothing more: no low-n alternate gate exists in this code, because
	// pre-committing a weaker gate was considered and rejected as a built-in
	// goalpost move. If accrual lands well below the registered guidance, the
	// honest path is a dated re-registration — an operator act under §17.
	LowAccrualNote string `json:"low_accrual_note,omitempty"`
}

// lowAccrualNote is §11's guidance rendered for the readout. It proposes
// nothing and gates nothing.
const lowAccrualNote = "accrual in this domain is low. BENCH-REG §11: if accrual volume lands well below " +
	"~5 pairs/month per domain, the honest path is a dated re-registration proposing a catastrophic-only " +
	"low-n variant — pre-committing a weaker alternate gate was considered and rejected. No alternate gate " +
	"exists in the platform, and none is proposed here."

// DomainReadout builds the §16 view for one registered domain.
func (p *Practice) DomainReadout(ctx context.Context, domain string) (DomainReadout, error) {
	st, err := p.Store.EnsureDomain(ctx, domain)
	if err != nil {
		return DomainReadout{}, err
	}
	out := DomainReadout{
		Domain: domain, Phase: st.Phase(), CurrentEpoch: st.CurrentEpochID,
		Registered: RegisteredValues(),
	}
	if out.Current, err = p.PublishedRate(ctx, domain, st.CurrentEpochID); err != nil {
		return DomainReadout{}, err
	}
	epochs, err := p.EpochHistory(ctx, domain)
	if err != nil {
		return DomainReadout{}, err
	}
	for _, e := range epochs {
		rate, err := p.PublishedRate(ctx, domain, e.ID)
		if err != nil {
			return DomainReadout{}, err
		}
		out.History = append(out.History, EpochRate{Epoch: e, Rate: rate})
	}
	if out.Gate, err = p.Gate(ctx, domain); err != nil {
		return DomainReadout{}, err
	}
	if out.Alarm, err = p.AlarmState(ctx, domain); err != nil {
		return DomainReadout{}, err
	}
	if out.DoneDirectly, err = p.DoneDirectly(ctx, domain); err != nil {
		return DomainReadout{}, err
	}
	if out.Current.N < GateMinNonTiedPairs {
		out.LowAccrualNote = lowAccrualNote
	}
	return out, nil
}

// Readout is the platform-wide §16 view: every registered v0 launch domain plus
// the 15.3 gate over them and the standing expansion freeze.
type Readout struct {
	Domains []DomainReadout `json:"domains"`
	// PlatformGate is 15.3: it opens when every registered launch domain passes.
	PlatformGate PlatformGate `json:"platform_gate"`
	// Freeze is the §12 expansion freeze, displayed and queryable.
	Freeze FreezeState `json:"freeze"`
	// Protocol names the registration these numbers were produced under.
	Protocol string `json:"protocol"`
	Marker   string `json:"marker"`
}

// Readout builds the whole §16 reporting surface. It is a pure read: it writes
// no state except the §4.1 first-open marker the gate evaluation owns (OQ3),
// and it gates nothing anywhere in the platform.
func (p *Practice) Readout(ctx context.Context) (Readout, error) {
	out := Readout{Protocol: RegistrationRef, Marker: RegisteredMarker}
	for _, d := range V0LaunchDomains() {
		dr, err := p.DomainReadout(ctx, d)
		if err != nil {
			return Readout{}, err
		}
		out.Domains = append(out.Domains, dr)
	}
	var err error
	if out.PlatformGate, err = p.PlatformGate(ctx); err != nil {
		return Readout{}, err
	}
	if out.Freeze, err = p.ExpansionFreeze(ctx); err != nil {
		return Readout{}, err
	}
	return out, nil
}
