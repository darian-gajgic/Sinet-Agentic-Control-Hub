// Package benchmark is the Spec S14.7 benchmark practice: the machinery around
// the signed pre-registration Spec/benchmark-preregistration-v1.md (BENCH-REG).
//
// THE PROTOCOL IS BENCH-REG, NOT THIS PACKAGE. S14.7 is explicit: "none of its
// registered numbers is this spec's to change — amendments happen only via
// BENCH-REG §17, and silent drift between that text and the running platform is
// a platform defect (its own clause)". Every frozen value this package needs —
// the gate limbs, the alarm threshold, the done-directly minimum, the labels,
// the §15 honest-claims table — lives in registered.go as marked read-only data
// carrying its BENCH-REG §-ref, is never re-derived, and is never a ⚙ settings
// row (S18.4 R9: "only benchmark.sampling_rate is a live dial"). The package's
// tests are the anti-drift instrument: registered_test.go asserts every value
// against the registration file itself.
//
// What the package owns: the sampling hook at verdict-eligibility (§4), the
// blind-pair machinery (§3, §5, §6), the epoch tracker (§9), the decision
// statistic G (§7) with tie handling (§8), the gate and the alarm (§11, §12),
// the measured done-directly feed (§13), and the honest-claims readout (§15,
// §16). What it does NOT own: rendering and every HTTP verb (Spec S15, B6),
// retention enforcement and history views (B5-8), and the registration itself.
//
// Import wall (importwall_test.go, AST-pinned with a non-tautology probe):
// storage / eventlog / run / scheduler / metering + stdlib. It never imports
// evals, verify, intake, api, settings, worker, stage or adapters — the ⚙ read
// rides this package's own narrow Settings interface, the intake artifact of
// record and the S14.8 floor surfaces ride func seams composed at the shell root
// (internal/shell/benchmark_seams.go), so the evals→verify→intake→adapters graph
// stays out of an observability leaf (CONVENTIONS §33/§34).
//
// The package owns the LOGIC; the shell owns WHEN (CONVENTIONS §31).
package benchmark

import (
	"context"
	"errors"
	"time"
)

// Package errors. Callers branch on these.
var (
	// ErrUnknownPair: no pair row has the given id.
	ErrUnknownPair = errors.New("benchmark: unknown pair")
	// ErrGuessRequired: a verdict was offered without the mandatory arm-guess.
	// BENCH-REG §3.3 is frozen: "No verdict without a guess; the guess is never
	// optional." The Answer type makes a guess-less verdict inexpressible; this
	// error is what the constructor and the record verb return when one is
	// attempted anyway.
	ErrGuessRequired = errors.New("benchmark: a verdict requires the arm-guess in the same form (BENCH-REG §3.3)")
	// ErrPairState: the pair is not in a state that admits the requested act.
	ErrPairState = errors.New("benchmark: pair is not in a state that admits this act")
	// ErrUnregisteredDomain: the domain is not a BENCH-REG §10 registration.
	ErrUnregisteredDomain = errors.New("benchmark: domain is not a registered launch domain (BENCH-REG §10)")
	// ErrNoAlarm: no alarm is standing for the domain.
	ErrNoAlarm = errors.New("benchmark: no alarm is standing")
	// ErrBadInput covers malformed verb inputs.
	ErrBadInput = errors.New("benchmark: invalid input")
)

// Settings is this package's narrow ⚙ reader (CONVENTIONS §2: consumers read
// the registry by dotted key through a per-package interface; *settings.Registry
// satisfies it). The package declares no key and consumes exactly one.
type Settings interface {
	StringMap(key string) (map[string]string, error)
}

// KeySamplingRate is the ONE ⚙ key this package consumes (Spec S14 settings
// table; BENCH-REG §4.1). It is a TypeMap of phase → percent, declared in
// internal/settings/index.go and read LIVE at evaluation time so an operator
// rate change applies to the next draw without a restart.
//
// Referenced but NOT consumed here, named so nothing in this package hardcodes
// or re-derives them:
//   - intake.zero_interaction_cost_usd — the trivial band is the intake
//     pipeline's; eligibility limb 3 reads the tier the pipeline already
//     recorded and never re-prices a task (G1 D1.7).
//   - retention.compaction_horizon — B5-8's; benchmark records are keep-forever
//     regardless (Spec S14.9).
//   - pressure.bg_admit_stop — background admission is metering/scheduler
//     machinery the direct arm RIDES, not machinery this package reads.
const KeySamplingRate = "benchmark.sampling_rate"

// The ⚙ benchmark.sampling_rate phase keys, exactly as declared in the settings
// index. Phase is pre-gate until the domain's gate first opens, then
// maintenance (BENCH-REG §4.1). The phase NAMES are part of the declared key's
// shape; the RATES behind them are the operator's dial.
const (
	PhasePreGate     = "pre-gate"
	PhaseMaintenance = "maintenance"
)

// Arm is one side of a pair (BENCH-REG §2).
type Arm string

const (
	// ArmPlatform is the deliverable the platform produced and the requester
	// accepted through the normal pipeline (intake → … → review → accept).
	ArmPlatform Arm = "platform"
	// ArmDirect is the same confirmed task statement + attachments run once,
	// single-shot, against the requester's own frontier surface.
	ArmDirect Arm = "direct"
)

// Side is a blind presentation position: the voter sees A and B and never the
// arms behind them.
type Side string

const (
	SideA Side = "A"
	SideB Side = "B"
)

// Choice is the §3.3 blind pick.
type Choice string

const (
	ChoiceA       Choice = "A"
	ChoiceB       Choice = "B"
	ChoiceTie     Choice = "tie"
	ChoiceBothBad Choice = "both-bad"
)

// validChoice reports whether c is one of the four frozen verdict values.
func validChoice(c Choice) bool {
	switch c {
	case ChoiceA, ChoiceB, ChoiceTie, ChoiceBothBad:
		return true
	}
	return false
}

// NonTied reports whether a choice enters m and w. tie and both-bad are
// excluded from both and never enter the posterior; they are first-class
// recorded signals reported beside every win rate (BENCH-REG §8, frozen).
func (c Choice) NonTied() bool { return c == ChoiceA || c == ChoiceB }

// Answer is the §3.3 verdict form: the blind pick AND, in the same act, the
// requester's guess of which side was the platform's.
//
// The guess is STRUCTURAL, not validated: the fields are unexported and the
// only constructor is NewAnswer, which requires a guess and refuses an empty
// one. A caller outside this package therefore cannot express a guess-less
// verdict at all, and the zero Answer is refused by the record verb — which is
// what BENCH-REG §3.3's "the guess is never optional" means in code.
type Answer struct {
	choice Choice
	guess  Side
	formed bool
}

// NewAnswer builds the one-act §3.3 form. guess is the side the requester
// believes is the platform's; it is mandatory for every choice including tie
// and both-bad (the blindness measurement is per VOTE, §5, and a tie vote is
// still a vote that saw both responses).
func NewAnswer(choice Choice, guess Side) (Answer, error) {
	if !validChoice(choice) {
		return Answer{}, errors.New("benchmark: verdict must be A, B, tie or both-bad (BENCH-REG §3.3)")
	}
	if guess != SideA && guess != SideB {
		return Answer{}, ErrGuessRequired
	}
	return Answer{choice: choice, guess: guess, formed: true}, nil
}

// Choice returns the blind pick.
func (a Answer) Choice() Choice { return a.choice }

// Guess returns the requester's platform-guess.
func (a Answer) Guess() Side { return a.guess }

// Formed reports whether the answer came through NewAnswer. The zero value is
// unformed and is refused, so a guess-less verdict cannot reach the store.
func (a Answer) Formed() bool { return a.formed }

// Pair state values (migration 0014). The §3 flow is
// sampled → declined | dispatched → rendered → recorded.
const (
	StateSampled    = "sampled"
	StateDeclined   = "declined"
	StateDispatched = "dispatched"
	StateRendered   = "rendered"
	StateRecorded   = "recorded"
)

// Position values: which SIDE the platform arm was rendered into for a pair
// (BENCH-REG §3.2, randomized per pair).
const (
	positionPlatformA = "platform-a"
	positionPlatformB = "platform-b"
)

// Event types minted by this package. Both are registered in the S14.2 contract
// under FamilyBenchmarkEval / FamilyHumanDecision (internal/eventlog/contract.go,
// CONVENTIONS §29).
const (
	// EventPairRecorded is the BENCH-REG §14 pair record — the practice's
	// artifact of record, keep-forever (Spec S14.9). Its payload carries the
	// §14 minimums verbatim.
	EventPairRecorded = "benchmark.pair_recorded"
	// EventAlarm is the BENCH-REG §12 alarm, raised when 1−G passes the
	// registered threshold in a launch domain's current epoch and cleared only
	// by an operator disposition. Alarm history is keep-forever.
	EventAlarm = "benchmark.alarm"
	// EventDecision is the S14.2 canonical generic human-decision type. This
	// package produces it for the two operator/requester acts the practice
	// needs: the §12 alarm disposition and the §4.2.1 standing-opt-in consent
	// flip.
	EventDecision = "decision.recorded"
)

// eventSchemaVersion versions this package's payloads (Spec S14.2 rule 2).
const eventSchemaVersion = 1

// PlatformScope is the user_id of the practice's platform-scope events (15.6).
// Pair records are attributed to the REQUESTER (the pair is theirs); alarms and
// gate bookkeeping are platform-scope.
const PlatformScope = "platform"

// Alarm actions on the EventAlarm payload.
const (
	AlarmRaise = "raise"
	AlarmClear = "clear"
)

// Clock is the package's time seam; tests move it rather than sleeping.
type Clock func() time.Time

// now resolves the clock, defaulting to the wall clock.
func (c Clock) now() time.Time {
	if c == nil {
		return time.Now().UTC()
	}
	return c().UTC()
}

// SuiteFloors is the BENCH-REG §10.1(d) / Spec S14.8 seam: whether a domain's
// registered regression suites are green at their registered floors. It is a
// FUNC SEAM composed at the shell root over internal/evals, so this package
// never imports evals (which drags verify → intake → adapters, CONVENTIONS
// §33). A nil seam means limb (d) is honestly "not evaluable" — never a fake
// green, and therefore the gate cannot open (BENCH-REG §10.1(d)).
type SuiteFloors func(ctx context.Context, domain string) (FloorReport, error)

// FloorReport is what the S14.8 surfaces say about a domain's suites.
type FloorReport struct {
	// Evaluable is false when the domain has no registered suites, an
	// unratified or unfloored version, or no sweep has ever run — the gate
	// "cannot be evaluated — and therefore cannot open" (BENCH-REG §10.1(d)).
	Evaluable bool
	Green     bool
	// Reason states the verdict in one line, for the card and the readout.
	Reason string
}

// TaskBrief is the §2 direct-arm input: the SAME confirmed task statement and
// the SAME attachments the specification had, read from the S06 intake artifact
// of record. It rides a func seam composed at the shell root for the same
// import-wall reason as SuiteFloors.
//
// Eligibility does NOT use this seam: the S06.4 stakes tier limb 3 reads is
// already in the event log (intake.state), so the eligibility check derives it
// from the log rather than depending on an artifact read (the derive-from-log
// precedent, CONVENTIONS §31).
type TaskBrief struct {
	TaskID string
	// Statement is the CONFIRMED task statement, frozen — both arms answer this
	// identical text (BENCH-REG §3.1/§6). Its source is the S06 artifact of
	// record, NOT the arriving request: the platform arm executed from the
	// confirmed specification, and bounded revisions during intake may have
	// moved that text away from what the requester first typed. Handing the
	// direct arm the raw ask would silently break §3.1 — the two arms would be
	// answering different questions and every comparison after it would be
	// measuring the intake pipeline rather than the platform.
	Statement string
	// StatementSource names the artifact the statement was drawn from
	// (document + content hash), so a record can be audited years later against
	// the exact text both arms answered. It is recorded on the pair.
	StatementSource string
	// Attachments are the refs the specification carried, passed through
	// unchanged. Refs, never bodies (P-T07-5).
	Attachments []string
}

// BriefSource resolves a task's §2 direct-arm brief (the intake artifact of
// record). A nil seam makes every task ineligible for dispatch, honestly — the
// direct arm cannot be run without the frozen statement.
type BriefSource func(ctx context.Context, taskID string) (TaskBrief, error)
