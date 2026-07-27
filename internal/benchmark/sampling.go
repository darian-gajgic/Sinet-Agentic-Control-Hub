package benchmark

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strconv"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
)

// sampling.go is BENCH-REG §4: the hook that fires at verdict-eligibility, the
// frozen five-limb eligibility conjunction, and the uniform-random draw at the
// ⚙ rate.
//
// The hook is DERIVE-FROM-LOG. A task ends in an accepted deliverable, which is
// already a minted event (deliverable.accepted, Spec S13.6), so the sampler
// tails the log rather than making internal/review or internal/accept call it —
// neither package is edited, and the trigger point cannot drift out from under
// the practice (CONVENTIONS §31: the package owns the LOGIC, the shell owns
// WHEN).
//
// The cursor is DURABLE (migration 0014 benchmark_sampler), not bootstrapped at
// the head like a detection tail: sampling is uniform-random among ELIGIBLE
// tasks and that uniformity is FROZEN (§4.1), so an accept skipped because the
// process restarted is a lost draw, not a neutral one. Each event's decision and
// the cursor advance commit in one transaction, so a crash can neither
// double-sample nor silently consume.

// Eligibility limbs, numbered as BENCH-REG §4.2 numbers them. A skipped task
// records WHICH limb failed, so a domain that never accrues can be diagnosed
// without guessing.
const (
	// LimbOptIn (§4.2.1): the requester has the standing benchmark opt-in
	// enabled; duplicate-run consumption comes from that requester's budget.
	LimbOptIn = 1
	// LimbDomainAccepted (§4.2.2): the task belongs to a registered launch
	// domain (§10) and ended in an accepted deliverable.
	LimbDomainAccepted = 2
	// LimbNotTrivial (§4.2.3): the task is not in the zero-interaction trivial
	// band (G1 D1.7) — trivial tasks don't exercise the pipeline under test.
	LimbNotTrivial = 3
	// LimbReviewableDeliverable (§4.2.4): the task's essence is a reviewable
	// deliverable, not a side effect.
	LimbReviewableDeliverable = 4
	// LimbNotDeclined (§4.2.5): the requester did not decline this specific
	// sampled pair.
	LimbNotDeclined = 5
)

// trivialTier is the S06.4 zero-interaction band's tier value, read from the
// intake pipeline's own recorded state. Limb 3 reads the tier the pipeline
// ALREADY classified and never re-prices a task: the band is defined by ⚙
// intake.zero_interaction_cost_usd and owned by internal/intake (G1 D1.7), and a
// second pricing here could disagree with the classification the requester
// actually experienced.
const trivialTier = "trivial"

// Rand returns a uniform integer in [0, n). The default is crypto/rand: the
// UNIFORMITY of sampling is frozen (§4.1), so the draw uses a real uniform
// source rather than a seeded generator whose stream a later reader could
// reconstruct and second-guess. Tests inject a deterministic one.
type Rand func(n int) int

func cryptoRand(n int) int {
	if n <= 0 {
		return 0
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		// crypto/rand failing is a platform fault, not a benchmark condition.
		// n-1 is the most conservative value available: the draw samples only
		// when it lands strictly below the phase rate, so n-1 MISSES at every
		// rate below 100%. At a 100% rate every eligible task is sampled
		// whatever the draw returns, so the fallback still cannot manufacture a
		// sample the rate did not already mandate. A broken entropy source
		// therefore under-accrues visibly rather than polluting the record with
		// draws that were never random.
		return n - 1
	}
	return int(v.Int64())
}

// Sampler is the §4 sampling hook. The shell drives it; it owns the decision.
type Sampler struct {
	Store    *Store
	Log      *eventlog.Log
	Settings Settings
	// Rand is the uniform draw source; nil means crypto/rand.
	Rand Rand
	// Logger records skipped tasks with their failing limb (log level, not a
	// card — an ineligible task is not an operator's problem, §4.2).
	Logger *slog.Logger
	// NewPairID mints pair ids; nil means the default derived id.
	NewPairID func(sourceSeq int64) string
}

func (s *Sampler) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

func (s *Sampler) draw(n int) int {
	if s.Rand != nil {
		return s.Rand(n)
	}
	return cryptoRand(n)
}

func (s *Sampler) pairID(sourceSeq int64) string {
	if s.NewPairID != nil {
		return s.NewPairID(sourceSeq)
	}
	return fmt.Sprintf("bp-%d", sourceSeq)
}

// tailBatch is how many events one sampling pass consumes. It matches the SSE
// short-batch read (CONVENTIONS §7): the pass is cheap, runs on a slow cadence,
// and a backlog drains over consecutive passes rather than in one long
// transaction. Structural, not ⚙ — S18 ratifies no key.
const tailBatch = 256

// Pass is one sampling pass's outcome, for the shell's log line.
type Pass struct {
	Considered int
	Sampled    int
	Skipped    int
	Cursor     int64
}

// Tail consumes the accepted-deliverable events past the durable cursor and
// applies the §4 sampling decision to each. It returns after at most one batch;
// the shell calls it again on its own cadence.
func (s *Sampler) Tail(ctx context.Context) (Pass, error) {
	var pass Pass
	cursor, err := s.Store.SamplerCursor(ctx)
	if err != nil {
		return pass, err
	}
	pass.Cursor = cursor
	events, err := s.Log.After(ctx, cursor, tailBatch)
	if err != nil {
		return pass, fmt.Errorf("benchmark: read accepted deliverables: %w", err)
	}
	for _, e := range events {
		if e.Type != acceptedType {
			// Every other event still advances the cursor: the cursor tracks
			// the LOG, so a quiet stretch of unrelated events is not re-read
			// on every pass.
			if err := s.consume(ctx, e.Seq, nil); err != nil {
				return pass, err
			}
			pass.Cursor = e.Seq
			continue
		}
		pass.Considered++
		sampled, err := s.consideration(ctx, e)
		if err != nil {
			return pass, err
		}
		if sampled {
			pass.Sampled++
		} else {
			pass.Skipped++
		}
		pass.Cursor = e.Seq
	}
	return pass, nil
}

// acceptedType is the S13.6 accept event this hook keys on. It is referenced by
// its canonical type STRING rather than by importing internal/review, exactly as
// internal/api reads drift.finding rows without importing internal/watchlist
// (CONVENTIONS §31/§34) — the wall holds and internal/review is untouched.
const acceptedType = "deliverable.accepted"

// consume advances the cursor past one event, optionally inserting a pair in the
// same transaction so the draw and the cursor can never disagree.
func (s *Sampler) consume(ctx context.Context, seq int64, insert func(*sql.Tx) error) error {
	return s.Store.db.WriteTx(ctx, func(tx *sql.Tx) error {
		have, err := s.Store.samplerCursorTx(ctx, tx)
		if err != nil {
			return err
		}
		if have >= seq {
			return nil // another pass already decided this event
		}
		if insert != nil {
			if err := insert(tx); err != nil {
				return err
			}
		}
		return s.Store.advanceCursorTx(ctx, tx, seq)
	})
}

// consideration applies §4.2 eligibility then the §4.1 draw to one accept.
func (s *Sampler) consideration(ctx context.Context, e eventlog.Event) (bool, error) {
	var el Eligibility
	err := s.Store.db.WriteTx(ctx, func(tx *sql.Tx) error {
		var err error
		el, err = s.Eligible(ctx, tx, e)
		return err
	})
	if err != nil {
		return false, err
	}
	if !el.Eligible {
		s.logger().Debug("benchmark: task not eligible for sampling",
			"deliverable", el.DeliverableID, "limb", el.Limb, "reason", el.Reason)
		return false, s.consume(ctx, e.Seq, nil)
	}

	st, err := s.Store.EnsureDomain(ctx, el.Domain)
	if err != nil {
		return false, err
	}
	phase := st.Phase()
	rate, err := s.rate(phase)
	if err != nil {
		return false, err
	}
	// The frozen mechanism: ONE independent uniform draw per eligible task
	// against the phase rate. There is no stratification, no prioritization and
	// no quota-filling anywhere in this package — sampling all eligible tasks is
	// trivially uniform, and the phase schedule changes accrual speed only,
	// never inference validity (§4.1).
	if s.draw(100) >= rate {
		s.logger().Debug("benchmark: eligible task not drawn",
			"deliverable", el.DeliverableID, "domain", el.Domain, "phase", phase, "rate_pct", rate)
		return false, s.consume(ctx, e.Seq, nil)
	}

	n := NewPair{
		PairID: s.pairID(e.Seq), UserID: el.UserID, Domain: el.Domain,
		DomainSource: el.DomainSource, TaskID: el.TaskID, DeliverableID: el.DeliverableID,
		TaskClass: el.TaskClass, PlatformRunID: el.PlatformRunID,
		Phase: phase, RatePct: rate, SourceSeq: e.Seq,
	}
	if err := s.consume(ctx, e.Seq, func(tx *sql.Tx) error {
		_, err := s.Store.CreateTx(ctx, tx, n)
		return err
	}); err != nil {
		return false, err
	}
	s.logger().Info("benchmark: task sampled into a blind pair",
		"pair", n.PairID, "domain", el.Domain, "phase", phase, "rate_pct", rate)
	return true, nil
}

// rate reads ⚙ benchmark.sampling_rate LIVE at evaluation time and returns the
// percent for a phase (CONVENTIONS §2: by dotted key, never hardcoded). An
// absent phase key is 0 — the honest reading of "the operator removed this
// phase's rate" is "sample nothing", never a fabricated default.
func (s *Sampler) rate(phase string) (int, error) {
	if s.Settings == nil {
		return 0, fmt.Errorf("benchmark: no settings reader for %s", KeySamplingRate)
	}
	m, err := s.Settings.StringMap(KeySamplingRate)
	if err != nil {
		return 0, fmt.Errorf("benchmark: read %s: %w", KeySamplingRate, err)
	}
	raw, ok := m[phase]
	if !ok {
		return 0, nil
	}
	pct, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("benchmark: %s[%s] = %q is not an integer percent", KeySamplingRate, phase, raw)
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return pct, nil
}

// Eligibility is the outcome of the §4.2 five-limb conjunction for one accept.
type Eligibility struct {
	Eligible bool
	// Limb is the FAILING limb's §4.2 number, 0 when eligible.
	Limb   int
	Reason string

	UserID        string
	DeliverableID string
	TaskID        string
	PlatformRunID string
	Domain        string
	DomainSource  string
	TaskClass     string
}

func ineligible(limb int, reason string, base Eligibility) Eligibility {
	base.Eligible, base.Limb, base.Reason = false, limb, reason
	return base
}

// Eligible applies BENCH-REG §4.2's frozen conjunction to one
// deliverable.accepted event. Every limb has a concrete source and every failure
// names its limb.
//
// Limb 4 (§4.2.4, "its essence is a reviewable deliverable, not a side effect —
// tasks whose point is an effect (deploy, send, publish) have no comparable
// single-shot direct arm and are excluded") is satisfied BY CONSTRUCTION at v0
// and the reading is recorded here rather than faked as a check: this hook fires
// only on deliverable.accepted, which is the S13.6 accept of a reviewed
// deliverable revision. An outward EFFECT at v0 is not a deliverable — it is an
// effects-journal row gated at D7 (Spec S02.7) and never produces a
// deliverable.accepted. So at v0 the trigger point IS the limb; if a later
// packet ever mints an accept for an effect-shaped task, this limb needs a real
// discriminator and that is a coordinator flag, not a silent pass.
func (s *Sampler) Eligible(ctx context.Context, tx *sql.Tx, e eventlog.Event) (Eligibility, error) {
	var accepted struct {
		DeliverableID string `json:"deliverable_id"`
		Revision      int    `json:"revision"`
	}
	if err := json.Unmarshal(e.Payload, &accepted); err != nil || accepted.DeliverableID == "" {
		// A non-conformant payload is skipped, never fatal (S14.2 rule 3).
		return ineligible(LimbDomainAccepted, "accept payload names no deliverable", Eligibility{}), nil
	}
	base := Eligibility{UserID: e.UserID, DeliverableID: accepted.DeliverableID}

	// Limb 2, first half: the deliverable exists and is ACCEPTED. The event is
	// the accept, so the row is read for its task and its state rather than
	// trusted from the payload.
	var taskID, state string
	err := tx.QueryRowContext(ctx,
		`SELECT task_id, state FROM deliverables WHERE deliverable_id = ?`,
		accepted.DeliverableID).Scan(&taskID, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return ineligible(LimbDomainAccepted, "deliverable row is absent", base), nil
	}
	if err != nil {
		return Eligibility{}, fmt.Errorf("benchmark: read deliverable %q: %w", accepted.DeliverableID, err)
	}
	base.TaskID = taskID
	if state != "accepted" {
		return ineligible(LimbDomainAccepted, "deliverable is "+state+", not accepted", base), nil
	}

	// Limb 1: the standing opt-in. Default OFF, so a household member is never
	// sampled without having said yes (§4.2.1).
	optedIn, err := s.Store.optedInTx(ctx, tx, e.UserID)
	if err != nil {
		return Eligibility{}, err
	}
	if !optedIn {
		return ineligible(LimbOptIn, "requester has no standing benchmark opt-in", base), nil
	}

	// Limb 2, second half: a REGISTERED launch domain (§10).
	code, source, err := resolveCodeDomain(ctx, tx, taskID)
	if err != nil {
		return Eligibility{}, err
	}
	if code == "" {
		return ineligible(LimbDomainAccepted, "task's domain could not be resolved from its verdict or routing records", base), nil
	}
	domain, ok := RegisteredDomain(code)
	if !ok {
		return ineligible(LimbDomainAccepted, "domain "+code+" is not a BENCH-REG §10 registration", base), nil
	}
	base.Domain, base.DomainSource = domain, source

	// Limb 3: not the zero-interaction trivial band. The source is the tier the
	// intake pipeline recorded, never a re-priced estimate.
	tier, family, err := taskTier(ctx, tx, taskID)
	if err != nil {
		return Eligibility{}, err
	}
	if tier == trivialTier {
		return ineligible(LimbNotTrivial, "task is in the zero-interaction trivial band", base), nil
	}
	base.TaskClass = taskClass(family, tier)

	// Limb 5: the requester has not declined THIS pair. At sampling time no pair
	// exists yet, so the limb is trivially satisfied here and becomes real at
	// Decline (pair.go), which records the decline flag on the pair and feeds the
	// §4.2.5 decline rate. It is named here so the conjunction is complete and
	// nothing silently drops a limb.
	base.PlatformRunID = acceptedRevisionRun(ctx, tx, accepted.DeliverableID)
	base.Eligible, base.Limb = true, 0
	base.Reason = "all five BENCH-REG §4.2 limbs satisfied"
	return base, nil
}

// resolveCodeDomain resolves a task's internal domain name (OQ2). The primary
// source is the accepting task's own verification record — the S07.11
// verdict.recorded payload carries the domain the deliverable was actually
// judged under, which is the domain whose pipeline the pair tests. The fallback
// is the routing/worker-template domain: the worker that produced the work.
// Both are read by canonical type STRING from run_events, so the import wall
// holds (internal/verify and internal/worker are untouched and unimported).
func resolveCodeDomain(ctx context.Context, tx *sql.Tx, taskID string) (domain, source string, err error) {
	var payload string
	err = tx.QueryRowContext(ctx, `
		SELECT e.payload FROM run_events e
		  JOIN runs r ON e.run_id = r.run_id
		 WHERE r.task_id = ? AND e.type = 'verdict.recorded'
		 ORDER BY e.event_seq DESC LIMIT 1`, taskID).Scan(&payload)
	if err == nil {
		var v struct {
			Domain string `json:"domain"`
		}
		if json.Unmarshal([]byte(payload), &v) == nil && v.Domain != "" {
			return v.Domain, "verdict.recorded", nil
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return "", "", fmt.Errorf("benchmark: read verdict domain for %q: %w", taskID, err)
	}

	err = tx.QueryRowContext(ctx, `
		SELECT e.payload FROM run_events e
		  JOIN runs r ON e.run_id = r.run_id
		 WHERE r.task_id = ? AND e.type = 'routing.decided'
		 ORDER BY e.event_seq DESC LIMIT 1`, taskID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("benchmark: read routing record for %q: %w", taskID, err)
	}
	var r struct {
		TemplateID string `json:"worker"`
	}
	if json.Unmarshal([]byte(payload), &r) != nil || r.TemplateID == "" {
		return "", "", nil
	}
	err = tx.QueryRowContext(ctx,
		`SELECT domain FROM worker_templates WHERE template_id = ?`, r.TemplateID).Scan(&domain)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("benchmark: read worker-template domain: %w", err)
	}
	return domain, "worker-template", nil
}

// taskTier reads the tier and family the S06 intake pipeline recorded for a
// task, from its latest intake.state row (the same derive-from-log read the
// pipeline itself uses to reload state).
func taskTier(ctx context.Context, tx *sql.Tx, taskID string) (tier, family string, err error) {
	var payload string
	e := tx.QueryRowContext(ctx, `
		SELECT e.payload FROM run_events e
		  JOIN runs r ON e.run_id = r.run_id
		 WHERE r.task_id = ? AND e.type = 'intake.state'
		 ORDER BY e.event_seq DESC LIMIT 1`, taskID).Scan(&payload)
	if errors.Is(e, sql.ErrNoRows) {
		return "", "", nil
	}
	if e != nil {
		return "", "", fmt.Errorf("benchmark: read intake tier for %q: %w", taskID, e)
	}
	var st struct {
		Tier   string `json:"tier"`
		Family string `json:"family"`
	}
	if json.Unmarshal([]byte(payload), &st) != nil {
		return "", "", nil
	}
	return st.Tier, st.Family, nil
}

// taskClass renders the BENCH-REG §14 "task class" from the intake family and
// tier the pipeline recorded. It is a label on the record, never an input to a
// statistic.
func taskClass(family, tier string) string {
	switch {
	case family == "" && tier == "":
		return ""
	case family == "":
		return tier
	case tier == "":
		return family
	}
	return family + "/" + tier
}

// acceptedRevisionRun returns the run that minted the deliverable's current
// revision — the platform arm's run (BENCH-REG §2). An absent link leaves the
// field empty and the record says so; it is never guessed.
func acceptedRevisionRun(ctx context.Context, tx *sql.Tx, deliverableID string) string {
	var runID string
	err := tx.QueryRowContext(ctx, `
		SELECT r.run_id FROM deliverable_revisions r
		  JOIN deliverables d ON d.deliverable_id = r.deliverable_id
		 WHERE r.deliverable_id = ? AND r.n = d.current_revision`, deliverableID).Scan(&runID)
	if err != nil {
		return ""
	}
	return runID
}
