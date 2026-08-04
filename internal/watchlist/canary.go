package watchlist

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// canary.go — the Spec S14.6 ¶3 API canary layer, the sibling half of the
// watchlist executor. The executor watches what providers SAY about themselves
// (pages, feeds, aggregator APIs); the canary layer watches what they actually
// DO on the wire, per lane.
//
// Four canary kinds mint canary.result: auth, behavioral, logprob and
// model-list. The fifth S14.6 bullet — the conformance canaries — is the S14.5
// adapter suites on their weekly schedule, whose recording home is
// conformance.RecordResult (eval.score_recorded, already minted); it surfaces
// dueness here and mints nothing of its own (canary_conformance.go).
//
// HARD INVARIANT — a canary failure raises a CARD, never a kill (S14.4 alert
// discipline / G1 D1.3). No canary terminates, tombstones, parks or gates a
// run; the auth canary's Class-4 verdict is recorded as the scheduler ACTION it
// is, and the dispatch path is what would honour a frozen lane. The invariant
// is AST-proven in canary_invariant_test.go, mirroring the watchdog's own
// no-auto-kill wall.
//
// Cards derive from ONE query. A failing canary raises its card as a
// drift.finding through the existing emitter (S14.6 ¶2), so
// InboxSnapshot.DriftCards, the fingerprint-per-incident-window dedup and the
// S14.8 revalidation edge all apply unchanged and internal/api needs no edit.
//
// The shell owns WHEN (one goroutine); dueness derives from the log — the last
// canary.result per (kind, lane) — so it survives restart and suspend and no
// ticker drives a cadence.

// EventCanaryResult is the S14.2 Drift & canary family type this layer mints
// (Spec S14.6 ¶3). Its payload carries the family's contract-minimum canary
// half verbatim: canary kind + lane + pass/fail/delta.
const EventCanaryResult = "canary.result"

// The four canary kinds (Spec S14.6 ¶3).
const (
	// CanaryAuth is the P-T17-1 auth canary: a scheduled cheap real request
	// per lane, classified by the S10.5 five-class classifier.
	CanaryAuth = "auth"
	// CanaryBehavioral is the scheduled fixed-prompt mini-eval per lane — the
	// ONLY drift detection available to the subscription lanes.
	CanaryBehavioral = "behavioral"
	// CanaryLogprob is the one-token logprob drift tracker, LOCAL LANE ONLY.
	CanaryLogprob = "logprob"
	// CanaryModelList is the P-T17-3 observed-vs-config model-list diff.
	CanaryModelList = "model-list"
)

// Canary results as they appear on the wire (Spec S14.2 "pass/fail").
const (
	CanaryPass = "pass"
	CanaryFail = "fail"
)

// CanaryPurposeTag is the S10.1 purpose_tag every canary call is metered under.
// Canary consumption is metered like everything and itemized on the lane
// owner's meters (D4/D7) — negligible by construction, never exempt, never
// off-ledger. Both v0 paid lanes are flat-rate and metering.MeteredExceptions
// is EMPTY at v0 (G1 P7), so a canary consumes allowance and pressure, never
// dollars — and it is never built on a metered path.
const CanaryPurposeTag metering.PurposeTag = metering.PurposeProbe

// CanaryWorkloadClass is the S10.7 scheduler workload class of a canary call.
const CanaryWorkloadClass scheduler.WorkloadClass = scheduler.ClassProbe

// CanaryArmEnv arms the REAL-REQUEST canary legs (auth, behavioral,
// model-list). The v0 posture is DISARMED: the packet that built this layer
// spent $0, so the legs that dial a provider ship behind an explicit operator
// act with a pre-registered consumption projection and stop line. The logprob
// canary is local-tier and costs no allowance, so it is never gated by this.
const CanaryArmEnv = "SINET_CANARY_ARM"

// CanaryArmed reports whether the operator has armed the real-request legs.
func CanaryArmed() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(CanaryArmEnv))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// Canary layer errors. Callers branch on these.
var (
	// ErrCanaryDisarmed: a real-request leg has no probe because the operator
	// has not armed it, or because arming it needs material that only exists
	// after the B5-gate install. It is an honest absence, never a failure, and
	// it always wraps the NAMED reason — a leg that cannot run must say why.
	ErrCanaryDisarmed = errors.New("watchlist: canary leg is disarmed")
	// ErrCanaryStackAbsent: a $0 local-tier canary could not run because the
	// local stack or the advisory metering seam is absent. This is NOT an
	// arming matter — the logprob canary costs no allowance and is never gated
	// by CanaryArmEnv — so the two are distinct errors and the sweep counts
	// them separately (drain D5).
	ErrCanaryStackAbsent = errors.New("watchlist: canary leg has no local stack")
	// ErrLaneNotLocal: the logprob canary was asked to run on a lane that
	// exposes no logprobs (Spec S03.7).
	ErrLaneNotLocal = errors.New("watchlist: the logprob canary runs on the local lane only")
)

// disarmedBecause builds the honest disarmed error for a named reason.
func disarmedBecause(reason string) error {
	return fmt.Errorf("%w: %s", ErrCanaryDisarmed, reason)
}

// DisarmedReasonNotArmed is the reason every real-request leg carries when the
// operator has not armed the layer.
const DisarmedReasonNotArmed = "the real-request legs are disarmed (" + CanaryArmEnv +
	" unset); this build spends $0 until an operator arms them"

// ⚙ dotted keys the canary layer consumes, read live at evaluation time so an
// operator change applies without a restart. Both are DECLARED already
// (internal/settings/index.go) — this layer re-declares nothing.
const (
	keyCanaryAuthInterval       = "canary.auth_interval"
	keyCanaryBehavioralInterval = "canary.behavioral_interval"
)

// logprobCanaryInterval is the logprob canary's cadence. STRUCTURAL, not ⚙:
// S18 ratifies exactly two canary keys and both are named for the other two
// canaries, so consuming either here would misappropriate a ratified clamp
// (canary.behavioral_interval's floor is a day and its ceiling a month — a
// shape chosen for a paid mini-eval, not for a free local probe). Daily is the
// same coarse cadence the dead-man canary uses, and this probe costs no
// allowance, so nothing is bought by dialing it.
const logprobCanaryInterval = 24 * time.Hour

// behavioralDropTolerance is how far a lane's mini-eval pass rate may fall
// before the canary alerts (Spec S14.6 ¶3 "alert on score drop"). STRUCTURAL,
// not ⚙: S18 ratifies no canary-tolerance key. A five-task battery moves in
// steps of 0.2, so a tolerance below 0.2 would alert on a single case flipping
// — noise on a battery this small — and 0.2 makes exactly one lost case the
// alert threshold.
const behavioralDropTolerance = 0.2

// logprobMarginTolerance is how far the label-token margin (Spec S12.5
// top1−top2) may move between runs before the canary alerts. STRUCTURAL, not ⚙.
// The margin is a logprob gap in nats; a full nat of movement on a fixed prompt
// at temperature 0 is a decode-path change, not sampling noise (greedy decoding
// makes the same prompt reproducible, so small drift is quantization/build
// noise and a whole nat is not).
const logprobMarginTolerance = 1.0

// CanarySettings is the canary layer's view of the settings registry — wider
// than the executor's Settings because scheduler.LoadLimitConfig needs the same
// three accessors (CONVENTIONS §2: ⚙ values by dotted key through narrow
// per-package interfaces). *settings.Registry satisfies it, and its method set
// is exactly scheduler.Settings' so it passes straight through.
type CanarySettings interface {
	Int(key string) (int64, error)
	Float(key string) (float64, error)
	Duration(key string) (time.Duration, error)
}

// CanaryResult is one canary run's outcome, before it is recorded.
type CanaryResult struct {
	Kind string
	Lane string
	Pass bool
	// Delta is the movement since this canary's previous run on this lane —
	// the S14.2 contract minimum's third canary field. Its meaning is the
	// canary's: a pass-rate delta (behavioral), a margin delta (logprob), a
	// count of changed model ids (model-list), or 0 (auth, which is a verdict
	// and not a measurement).
	Delta float64
	// Baseline marks a FIRST observation: there was no prior run to have
	// drifted from, so Delta is not meaningful and no card is raised. The same
	// rule the feed and aggregator tiers already apply (S14.6 ¶T2/T3).
	Baseline bool
	// Summary is the one-line summary carried onto the card.
	Summary string
	// Detail is the bounded evidence.
	Detail string
	// ChangeClass is the S14.2 drift class a FAILURE raises its card under.
	ChangeClass string
	// Subjects are the model ids a failure names, if any. A model id is the
	// only subject that can carry a finding into the S14.8 revalidation
	// runbook (OQ4(a)); the model-list and logprob canaries are the producers
	// that have one.
	Subjects []string
	// Action is the scheduler action an auth-canary signal classified to
	// (Spec S10.5), recorded as DATA. Recording it is the whole point: the
	// canary never acts on it.
	Action string
	// LimitClass is the S10.5 class number the classifier returned.
	LimitClass int
	// Measurement is the raw scalar this run observed (a pass rate, a label
	// margin), recorded so the NEXT run can compute its delta from the log
	// rather than from a side store. Nil for a canary that measures nothing.
	Measurement *float64
	// Observed is the raw list this run saw (the model-list canary's), stored
	// for the same reason. It is bounded before it reaches the payload.
	Observed []string
	// Err records a probe that could not complete. A probe that did not answer
	// is not proof of a policy revocation, so it fails the canary honestly and
	// routes to the digest rather than to flag-now.
	Err string
}

// CanaryPayload is the canary.result payload. The first four fields are the
// S14.2 FamilyDriftCanary contract minimum's canary half verbatim: canary kind
// + lane + pass/fail/delta.
type CanaryPayload struct {
	CanaryKind string  `json:"canary_kind"`
	Lane       string  `json:"lane"`
	Result     string  `json:"result"`
	Delta      float64 `json:"delta"`

	Baseline    bool     `json:"baseline,omitempty"`
	Summary     string   `json:"summary"`
	Detail      string   `json:"detail,omitempty"`
	ChangeClass string   `json:"change_class,omitempty"`
	Subjects    []string `json:"subjects,omitempty"`
	// Action / LimitClass carry the S10.5 classifier verdict for the auth
	// canary. They are a RECORD of what the taxonomy said, never an act.
	Action     string `json:"action,omitempty"`
	LimitClass int    `json:"limit_class,omitempty"`
	// PurposeTag / WorkloadClass state the S10.1/S10.7 metering posture on the
	// record itself, so a receipt reader never has to infer it.
	PurposeTag    string `json:"purpose_tag"`
	WorkloadClass string `json:"workload_class"`
	// Measurement / Observed are this run's raw observation, kept so the next
	// run derives its delta from the log instead of a side store.
	Measurement *float64 `json:"measurement,omitempty"`
	Observed    []string `json:"observed,omitempty"`
	// ObservedCount is the FULL size of the observation, which differs from
	// len(Observed) when the recorded list was truncated.
	ObservedCount int `json:"observed_count,omitempty"`
	// ObservedTruncated marks a recorded list bounded by observedListCap.
	ObservedTruncated bool `json:"observed_truncated,omitempty"`
	// ObservedDigest is a stable hash over the FULL observation, so drift is
	// still detectable against a truncated baseline even though the individual
	// ids cannot be named from it.
	ObservedDigest string `json:"observed_digest,omitempty"`
	// VerifiedOn is when this observation was made — the S03.6 "verified-on
	// date" the observed model list is consumed with.
	VerifiedOn time.Time `json:"verified_on"`
	Error      string    `json:"error,omitempty"`
}

// Canaries is the S14.6 ¶3 canary layer. Each canary is nil-safe: an absent one
// is simply not scheduled, which is the honest v0 state for every leg that
// needs a real request or a runner binary.
type Canaries struct {
	db  *storage.DB
	log *eventlog.Log

	// Settings reads the two declared ⚙ canary intervals plus the three S10.5
	// limit ⚙ the classifier needs.
	Settings CanarySettings
	// Emitter raises the card for a failing canary. Nil ⇒ results are recorded
	// and no card is raised (the shell always wires one).
	Emitter *Emitter

	Auth        *AuthCanary
	Behavioral  *BehavioralCanary
	Logprob     *LogprobCanary
	ModelList   *ModelListCanary
	Conformance *ConformanceCanary

	Logger *slog.Logger
	// Now is the test clock seam; nil ⇒ time.Now.
	Now func() time.Time
}

// NewCanaries builds the canary layer over the event log it records into.
func NewCanaries(db *storage.DB, log *eventlog.Log, s CanarySettings) *Canaries {
	return &Canaries{db: db, log: log, Settings: s}
}

func (c *Canaries) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Canaries) logger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}

// CanarySweep is the outcome of one canary sweep, for the shell's log line.
type CanarySweep struct {
	Ran      int
	Failures int
	// Disarmed counts due REAL-REQUEST legs that had no probe: either the
	// operator has not armed them, or arming needs gate-time material. A
	// disarmed leg is still counted and still names its reason — it is never
	// silently skipped (drain D1).
	Disarmed int
	// StackAbsent counts due $0 LOCAL-tier legs that could not run for want of
	// a local stack or an advisory meter. Distinct from Disarmed: no arming
	// decision is involved (drain D5).
	StackAbsent int
	// Reasons names, once per leg kind, why a leg did not run this sweep.
	Reasons []string
	// Cards counts the conformance-dueness cards raised this sweep.
	Cards int
}

// note records a skip reason once per leg kind.
func (s *CanarySweep) note(kind, reason string) {
	line := kind + ": " + reason
	for _, existing := range s.Reasons {
		if existing == line {
			return
		}
	}
	s.Reasons = append(s.Reasons, line)
}

// skipReason strips the sentinel prefix so the sweep line carries the NAMED
// reason rather than repeating the sentinel text.
func skipReason(err, sentinel error) string {
	reason := strings.TrimPrefix(err.Error(), sentinel.Error())
	reason = strings.TrimPrefix(strings.TrimSpace(reason), ":")
	if r := strings.TrimSpace(reason); r != "" {
		return r
	}
	return err.Error()
}

// canaryJob is one due canary run.
type canaryJob struct {
	kind string
	lane string
	run  func(ctx context.Context) (CanaryResult, error)
}

// RunDue runs every canary whose cadence has elapsed and records each result.
// One canary failing never aborts the sweep: a lane that cannot be probed must
// not silence the lanes that can.
func (c *Canaries) RunDue(ctx context.Context) (CanarySweep, error) {
	var p CanarySweep

	jobs, err := c.due(ctx)
	if err != nil {
		return p, err
	}
	for _, j := range jobs {
		res, err := j.run(ctx)
		if err != nil {
			// A leg that could not run is ACCOUNTED FOR and names its reason;
			// it is never silently dropped (drain D1/D5).
			switch {
			case errors.Is(err, ErrCanaryDisarmed):
				p.Disarmed++
				p.note(j.kind, skipReason(err, ErrCanaryDisarmed))
				continue
			case errors.Is(err, ErrCanaryStackAbsent):
				p.StackAbsent++
				p.note(j.kind, skipReason(err, ErrCanaryStackAbsent))
				continue
			}
			// A probe that could not complete is recorded as an honest failure
			// rather than dropped — an unreachable lane is a fact worth a card.
			res = CanaryResult{
				Kind: j.kind, Lane: j.lane,
				Summary:     fmt.Sprintf("%s canary could not complete on lane %s", j.kind, j.lane),
				ChangeClass: ClassUnclear,
				Err:         err.Error(),
			}
		}
		res.Kind, res.Lane = j.kind, j.lane
		if err := c.record(ctx, res); err != nil {
			return p, err
		}
		p.Ran++
		if !res.Pass && !res.Baseline {
			p.Failures++
		}
	}

	if c.Conformance != nil {
		cards, err := c.Conformance.SurfaceDue(ctx, c)
		if err != nil {
			c.logger().Warn("watchlist: conformance-dueness surfacer failed", "err", err)
		}
		p.Cards += cards
	}
	return p, nil
}

// due builds the job list for this sweep. Dueness derives from STORED state —
// the last canary.result per (kind, lane) in the event log — so a restart or a
// wall-clock jump can neither skip nor double-fire a run, and no ticker drives
// a cadence. Intervals are read live at evaluation time.
func (c *Canaries) due(ctx context.Context) ([]canaryJob, error) {
	now := c.now()
	var jobs []canaryJob

	add := func(kind, lane string, every time.Duration, run func(context.Context) (CanaryResult, error)) error {
		last, ok, err := c.LastRunAt(ctx, kind, lane)
		if err != nil {
			return err
		}
		if ok && now.Sub(last) < every {
			return nil
		}
		jobs = append(jobs, canaryJob{kind: kind, lane: lane, run: run})
		return nil
	}

	if c.Auth != nil || c.ModelList != nil {
		// S14.6 ¶3 gives the model-list diff no interval of its own — it
		// "rides the same schedule" as the other per-account cheap real
		// request, which is the auth canary's. Reading the ratified
		// canary.auth_interval for both is what that sentence asks for;
		// inventing a constant S18 never ratified would contradict it.
		every, err := c.Settings.Duration(keyCanaryAuthInterval)
		if err != nil {
			return nil, fmt.Errorf("watchlist: read ⚙ %s: %w", keyCanaryAuthInterval, err)
		}
		if c.Auth != nil {
			for _, lane := range c.Auth.Lanes {
				if err := add(CanaryAuth, lane, every, c.Auth.runner(c, lane)); err != nil {
					return nil, err
				}
			}
		}
		if c.ModelList != nil {
			for _, lane := range c.ModelList.Lanes {
				if err := add(CanaryModelList, lane, every, c.ModelList.runner(c, lane)); err != nil {
					return nil, err
				}
			}
		}
	}

	if c.Behavioral != nil {
		every, err := c.Settings.Duration(keyCanaryBehavioralInterval)
		if err != nil {
			return nil, fmt.Errorf("watchlist: read ⚙ %s: %w", keyCanaryBehavioralInterval, err)
		}
		for _, lane := range c.Behavioral.Lanes {
			if err := add(CanaryBehavioral, lane, every, c.Behavioral.runner(c, lane)); err != nil {
				return nil, err
			}
		}
	}

	if c.Logprob != nil {
		if err := add(CanaryLogprob, LaneLocal, logprobCanaryInterval, c.Logprob.runner(c, LaneLocal)); err != nil {
			return nil, err
		}
	}
	return jobs, nil
}

// record appends one canary.result and, for a failure, raises the card.
//
// The card is a drift.finding — the SAME type the executor half raises — so it
// derives through the existing InboxSnapshot.DriftCards query with the existing
// fingerprint-per-incident-window dedup, and the S14.8 revalidation edge fires
// for a models-class failure naming a model id (OQ4(a)). internal/api needs no
// edit for a canary card, which is a checkable negative.
func (c *Canaries) record(ctx context.Context, res CanaryResult) error {
	now := c.now()
	result := CanaryFail
	if res.Pass {
		result = CanaryPass
	}
	recorded, truncated, digest := boundObserved(res.Observed)
	body, err := json.Marshal(CanaryPayload{
		CanaryKind:    res.Kind,
		Lane:          res.Lane,
		Result:        result,
		Delta:         res.Delta,
		Baseline:      res.Baseline,
		Summary:       res.Summary,
		Detail:        clip(res.Detail, detailCap),
		ChangeClass:   res.ChangeClass,
		Subjects:      res.Subjects,
		Action:        res.Action,
		LimitClass:    res.LimitClass,
		PurposeTag:    string(CanaryPurposeTag),
		WorkloadClass: string(CanaryWorkloadClass),
		Measurement:   res.Measurement,
		Observed:      recorded,
		ObservedCount: len(res.Observed),
		// A degenerate catalogue must not push the payload past
		// ⚙ state.event_payload_cap: an aborted Append would mean the lane
		// never baselines and the canary silently stops working (drain D6).
		ObservedTruncated: truncated,
		ObservedDigest:    digest,
		VerifiedOn:        now,
		Error:             res.Err,
	})
	if err != nil {
		return fmt.Errorf("watchlist: marshal canary result: %w", err)
	}
	if _, err := c.log.Append(ctx, eventlog.Append{
		UserID:        platformUser,
		Type:          EventCanaryResult,
		SchemaVersion: eventSchemaVersion,
		Payload:       body,
		Time:          now,
	}); err != nil {
		return fmt.Errorf("watchlist: append canary result: %w", err)
	}

	if res.Pass || res.Baseline || c.Emitter == nil {
		return nil
	}
	return c.raiseCard(ctx, res)
}

// raiseCard emits the drift.finding(s) for a failed canary. A failure naming
// model ids emits one finding per subject — they share a fingerprint, so they
// fold into ONE card, while each carries its own model id into the revalidation
// runbook.
func (c *Canaries) raiseCard(ctx context.Context, res CanaryResult) error {
	base := Hit{
		RowID:   canarySource(res.Kind, res.Lane),
		Kind:    res.Kind,
		Source:  canarySource(res.Kind, res.Lane),
		Lane:    res.Lane,
		Class:   res.ChangeClass,
		Summary: res.Summary,
		Detail:  res.Detail,
	}
	if len(res.Subjects) == 0 {
		_, _, err := c.Emitter.Emit(ctx, base)
		return err
	}
	subjects := res.Subjects
	if len(subjects) > canarySubjectCap {
		c.logger().Warn("watchlist: canary named more model ids than the per-run subject cap; the rest ride the card detail",
			"kind", res.Kind, "lane", res.Lane, "subjects", len(subjects), "cap", canarySubjectCap)
		subjects = subjects[:canarySubjectCap]
	}
	for _, s := range subjects {
		h := base
		h.Subject = s
		if _, _, err := c.Emitter.Emit(ctx, h); err != nil {
			return err
		}
	}
	return nil
}

// observedListCap bounds how many ids of an observation are RECORDED on the
// payload. STRUCTURAL, not ⚙. ⚙ state.event_payload_cap (default 64 KiB) is a
// hard Append gate: an oversized payload is refused, and a refused append means
// the lane never baselines and the canary silently stops working — the exact
// failure shape this codebase treats as worse than a loud one. 250 ids of a
// realistic length sit around 8 KiB, comfortably inside the cap, and no real
// provider catalogue approaches it.
const observedListCap = 250

// boundObserved bounds a recorded observation. It always returns a digest over
// the FULL list, so drift stays DETECTABLE against a truncated baseline even
// though the individual ids cannot be named from it — the honest degradation is
// "something moved and I cannot name it", never "nothing moved".
func boundObserved(observed []string) (recorded []string, truncated bool, digest string) {
	if len(observed) == 0 {
		return nil, false, ""
	}
	digest = shortHash(observed...)
	if len(observed) <= observedListCap {
		return observed, false, digest
	}
	return observed[:observedListCap], true, digest
}

// canarySubjectCap bounds how many model-id findings one canary run raises.
// STRUCTURAL, not ⚙. A provider rotating its whole catalogue in one window is
// possible and would otherwise fire an unbounded number of revalidation flags
// from a single observation; the card still names the full count in its detail,
// so nothing is hidden — only the per-run flag fan-out is bounded.
const canarySubjectCap = 25

// canarySource is the stable card source for a canary lane. The incident
// fingerprint derives over {source, lane, change class}, so every failure of
// one canary on one lane folds into one card per incident window.
func canarySource(kind, lane string) string {
	return "canary:" + kind + ":" + lane
}

// LastRunAt returns when this canary last ran on this lane, derived from the
// log — no side table. ok is false when it has never run.
func (c *Canaries) LastRunAt(ctx context.Context, kind, lane string) (time.Time, bool, error) {
	var ts string
	err := c.db.QueryRowContext(ctx,
		`SELECT ts FROM run_events
		  WHERE type = ?
		    AND json_extract(payload, '$.canary_kind') = ?
		    AND json_extract(payload, '$.lane') = ?
		  ORDER BY event_seq DESC LIMIT 1`, EventCanaryResult, kind, lane).Scan(&ts)
	if err == sql.ErrNoRows {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("watchlist: read last %s canary run on %q: %w", kind, lane, err)
	}
	t, perr := time.Parse(time.RFC3339Nano, ts)
	if perr != nil {
		return time.Time{}, false, nil
	}
	return t, true, nil
}

// LastMeasurement returns the scalar this canary last measured on this lane —
// the baseline the current run computes its delta against, derived from the log
// with no side store. ok is false when it has never measured one, which is the
// first-observation case: a baseline is established and no card is raised.
func (c *Canaries) LastMeasurement(ctx context.Context, kind, lane string) (float64, bool, error) {
	var value sql.NullFloat64
	err := c.db.QueryRowContext(ctx,
		`SELECT json_extract(payload, '$.measurement') FROM run_events
		  WHERE type = ?
		    AND json_extract(payload, '$.canary_kind') = ?
		    AND json_extract(payload, '$.lane') = ?
		    AND json_extract(payload, '$.measurement') IS NOT NULL
		  ORDER BY event_seq DESC LIMIT 1`, EventCanaryResult, kind, lane).Scan(&value)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("watchlist: read last %s canary measurement on %q: %w", kind, lane, err)
	}
	return value.Float64, value.Valid, nil
}

// Observation is one recorded model-list observation read back from the log.
type Observation struct {
	// List is the recorded id list. It is the FULL observation unless
	// Truncated, in which case it is a bounded prefix and naming ids from it
	// would invent removals.
	List []string
	// Truncated marks a list bounded by observedListCap.
	Truncated bool
	// Digest hashes the FULL observation, so an oversized catalogue is still
	// comparable even when its ids are not recoverable.
	Digest string
}

// LastObserved returns the observation this canary last recorded on this lane,
// derived from the log. ok is false when it has never recorded one.
func (c *Canaries) LastObserved(ctx context.Context, kind, lane string) (Observation, bool, error) {
	var raw, digest sql.NullString
	var truncated sql.NullBool
	err := c.db.QueryRowContext(ctx,
		`SELECT json_extract(payload, '$.observed'),
		        json_extract(payload, '$.observed_truncated'),
		        json_extract(payload, '$.observed_digest')
		   FROM run_events
		  WHERE type = ?
		    AND json_extract(payload, '$.canary_kind') = ?
		    AND json_extract(payload, '$.lane') = ?
		    AND json_extract(payload, '$.observed') IS NOT NULL
		  ORDER BY event_seq DESC LIMIT 1`, EventCanaryResult, kind, lane).Scan(&raw, &truncated, &digest)
	if err == sql.ErrNoRows {
		return Observation{}, false, nil
	}
	if err != nil {
		return Observation{}, false, fmt.Errorf("watchlist: read last %s canary observation on %q: %w", kind, lane, err)
	}
	var list []string
	if err := json.Unmarshal([]byte(raw.String), &list); err != nil {
		return Observation{}, false, nil
	}
	return Observation{List: list, Truncated: truncated.Bool, Digest: digest.String}, true, nil
}

// LaneLocal / the paid lanes, named here so the canary layer states its own
// lane set rather than importing one (Spec S03.2).
const (
	LaneAnthropic = "anthropic"
	LaneZAI       = "zai"
	LaneLocal     = "local"
)

// PaidLanes are the two v0 lanes a real-request canary probes. Both are
// FLAT-RATE subscription lanes (metering.MeteredExceptions is EMPTY at v0,
// G1 P7), so canary consumption is allowance and pressure, never dollars.
func PaidLanes() []string { return []string{LaneAnthropic, LaneZAI} }
