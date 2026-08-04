package benchmark

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/scheduler"
)

// pair.go is BENCH-REG §2/§3: the direct arm, the blind pairing, the one-act
// verdict-plus-guess, and the record that precedes the reveal.
//
// The order of §3 is load-bearing and is enforced by the pair's state machine:
// a pair is dispatched before it is rendered, rendered before it can be voted
// on, and RECORDED before any arm identity is readable. Reveal is a read of the
// committed record (§3.4), never a flag flipped on a mutable row.

// The v0 direct-arm surface for software-development (BENCH-REG §10.1: "the
// requester's own frontier coding surface (consumer subscription chat/CLI),
// single-shot per §2"). In this tree that surface is the wrapped Anthropic CLI
// on the anthropic lane (CONVENTIONS §10). These are STRUCTURAL constants, not
// ⚙ and not registered numbers: they name which of the platform's existing
// substrates IS the requester's frontier surface, and S18 ratifies no key for a
// substrate name. web-research's surface arrives with its v0.1 accrual.
const (
	directSubstrate = "claude-cli"
	directLane      = "anthropic"
)

// Queue is the ordinary admission surface (Spec S10.7), satisfied by
// *scheduler.Scheduler. The direct arm rides it as ClassBackground under the
// REQUESTER's own user_id, so the existing spawn gate, pressure admission (⚙
// pressure.bg_admit_stop) and per-checkpoint gates throttle it exactly like any
// other background work (G2 Def.4; Spec S10.4). No benchmark-special admission
// path, priority or exemption exists anywhere in this package.
type Queue interface {
	Enqueue(ctx context.Context, runID string, class scheduler.WorkloadClass) error
}

// Consumption reads a run's measured consumption and its observed model
// identities from the S10 ledger, inside the caller's transaction so the §14
// record is written from one consistent read. Satisfied by *metering.Ledger.
// The practice RECORDS what metering measured; it prices nothing itself.
type Consumption interface {
	RunConsumptionTx(ctx context.Context, tx *sql.Tx, runID string) (metering.RunConsumption, error)
}

// PlatformText resolves the accepted deliverable's content — the platform arm's
// answer "as accepted at review" (BENCH-REG §10.1). It is a func seam composed
// at the shell root so this package does not import internal/review.
type PlatformText func(ctx context.Context, deliverableID string) (string, error)

// DirectText resolves the direct arm's produced answer for its run. Same seam
// posture as PlatformText.
type DirectText func(ctx context.Context, runID string) (string, error)

// Practice is the S14.7 machinery: the package-level verbs the S15/B6 surfaces
// will call. HTTP endpoints and React rendering are B6's (Spec S15.6; the §31
// precedent) — this packet ships the verbs and the projection data.
type Practice struct {
	Store    *Store
	Log      *eventlog.Log
	Runs     *run.Store
	Queue    Queue
	Ledger   Consumption
	Briefs   BriefSource
	Platform PlatformText
	Direct   DirectText
	Floors   SuiteFloors
	// Rand assigns the §3.2 per-pair position; nil means crypto/rand.
	Rand   Rand
	Logger *slog.Logger
	Now    Clock
}

func (p *Practice) logger() *slog.Logger {
	if p.Logger != nil {
		return p.Logger
	}
	return slog.Default()
}

func (p *Practice) now() time.Time { return p.Now.now() }

func (p *Practice) rand(n int) int {
	if p.Rand != nil {
		return p.Rand(n)
	}
	return cryptoRand(n)
}

// DirectRunID is the direct arm's run id for a pair. It is derived so the link
// is legible in the trace and so a re-dispatch cannot silently create a second
// direct arm: the runs table's primary key refuses the duplicate.
func DirectRunID(pairID string) string { return pairID + ".direct" }

// DispatchDirectArm runs BENCH-REG §2's direct arm for a sampled pair.
//
// §2, frozen: "the same confirmed task statement + the same attachments the
// specification had, run once, single-shot against the requester's own frontier
// surface for that domain (their consumer subscription — chat or CLI — with that
// surface's native defaults; no retries, no follow-up turns, take what comes)."
//
// So: ONE run row, ONE enqueue, and nothing else. This package contains no
// retry, no re-dispatch, no follow-up turn and no verification round — the
// asymmetry between the arms IS the treatment under test (§6), and adding any
// of the platform's machinery to the direct arm would destroy the comparison it
// exists to make. The run carries no worker template and no knowledge injection
// because this package assigns neither.
//
// Ordinary platform economics still apply — metering, per-run ceilings and
// pressure admission are not arm shaping — but if one of them TRUNCATES the
// direct arm, that is a §6 parity fact and is recorded on the pair for the
// coordinator rather than absorbed (recorded at RecordVerdict, where the run's
// outcome is known).
func (p *Practice) DispatchDirectArm(ctx context.Context, pairID string) (Pair, error) {
	pair, err := p.Store.Pair(ctx, pairID)
	if err != nil {
		return Pair{}, err
	}
	if pair.State != StateSampled {
		return Pair{}, fmt.Errorf("%w: pair %q is %s, want %s", ErrPairState, pairID, pair.State, StateSampled)
	}
	if p.Briefs == nil {
		return Pair{}, fmt.Errorf("%w: no intake-artifact seam — the direct arm cannot be run without the frozen task statement (BENCH-REG §2)", ErrBadInput)
	}
	brief, err := p.Briefs(ctx, pair.TaskID)
	if err != nil {
		return Pair{}, fmt.Errorf("benchmark: read the §2 frozen statement for task %q: %w", pair.TaskID, err)
	}
	if strings.TrimSpace(brief.Statement) == "" {
		return Pair{}, fmt.Errorf("%w: task %q has no confirmed statement to put to the direct arm", ErrBadInput, pair.TaskID)
	}

	// The statement source rides the direct arm's BIRTH EVENT, captured at
	// dispatch. It has to be captured here rather than re-resolved at record
	// time: the artifact of record is versioned, and a later bounded revision
	// must never rewrite which text this pair's direct arm actually answered.
	// The event log is the durable home for it — no schema change, and the run
	// becomes self-describing (derive-from-log, CONVENTIONS §31).
	detail, err := json.Marshal(directArmBirth{
		PairID: pairID, StatementSource: brief.StatementSource,
		Attachments: brief.Attachments, StatementBytes: len(brief.Statement),
	})
	if err != nil {
		return Pair{}, err
	}
	runID := DirectRunID(pairID)
	err = p.Store.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := p.Runs.CreateTx(ctx, tx, run.NewRun{
			ID:        runID,
			UserID:    pair.UserID, // 3.4: the duplicate bills the requester
			TaskID:    pair.TaskID,
			Substrate: directSubstrate,
			Lane:      directLane,
		}, run.EventCreated, detail)
		return err
	})
	if err != nil && !errors.Is(err, run.ErrExists) {
		return Pair{}, fmt.Errorf("benchmark: create direct-arm run for %q: %w", pairID, err)
	}
	if err := p.Queue.Enqueue(ctx, runID, scheduler.ClassBackground); err != nil {
		return Pair{}, fmt.Errorf("benchmark: enqueue direct-arm run %q: %w", runID, err)
	}
	if err := p.Store.db.WriteTx(ctx, func(tx *sql.Tx) error {
		return p.Store.updatePairTx(ctx, tx, pairID,
			`state = ?, direct_run_id = ?`, StateDispatched, runID)
	}); err != nil {
		return Pair{}, err
	}
	p.logger().Info("benchmark: direct arm dispatched as ordinary background work",
		"pair", pairID, "run", runID, "owner", pair.UserID,
		"statement_source", brief.StatementSource, "attachments", len(brief.Attachments))
	return p.Store.Pair(ctx, pairID)
}

// directArmBirth is the detail carried on the direct arm's run.created event:
// which frozen text it was given, and where that text came from.
type directArmBirth struct {
	PairID          string   `json:"benchmark_pair_id"`
	StatementSource string   `json:"benchmark_statement_source"`
	Attachments     []string `json:"benchmark_attachments,omitempty"`
	StatementBytes  int      `json:"benchmark_statement_bytes"`
}

// statementSourceTx reads back the statement source captured at dispatch. An
// absent or unreadable birth detail yields "", and the record says so rather
// than guessing which text the arm answered.
func (p *Practice) statementSourceTx(ctx context.Context, tx *sql.Tx, runID string) string {
	if runID == "" {
		return ""
	}
	var payload string
	err := tx.QueryRowContext(ctx,
		`SELECT payload FROM run_events WHERE run_id = ? AND type = ?
		  ORDER BY event_seq LIMIT 1`, runID, run.EventCreated).Scan(&payload)
	if err != nil {
		return ""
	}
	var envelope struct {
		Detail directArmBirth `json:"detail"`
	}
	if json.Unmarshal([]byte(payload), &envelope) != nil {
		return ""
	}
	return envelope.Detail.StatementSource
}

// RenderBlind renders both arms through the ONE uniform presentation template
// and assigns the §3.2 per-pair position, moving the pair to `rendered` — the
// state whose card the requester votes on.
//
// Position is randomized per pair and STORED. It is arm-identity data, so it is
// written here and read only by the record path; the pending-verdict surface
// (PendingPair) has no field for it.
func (p *Practice) RenderBlind(ctx context.Context, pairID string) (Pair, error) {
	pair, err := p.Store.Pair(ctx, pairID)
	if err != nil {
		return Pair{}, err
	}
	if pair.State != StateDispatched {
		return Pair{}, fmt.Errorf("%w: pair %q is %s, want %s", ErrPairState, pairID, pair.State, StateDispatched)
	}
	if p.Platform == nil || p.Direct == nil {
		return Pair{}, fmt.Errorf("%w: both arm-text seams are required to render a blind pair", ErrBadInput)
	}
	platformText, err := p.Platform(ctx, pair.DeliverableID)
	if err != nil {
		return Pair{}, fmt.Errorf("benchmark: read the platform arm of %q: %w", pairID, err)
	}
	directText, err := p.Direct(ctx, pair.DirectRunID)
	if err != nil {
		return Pair{}, fmt.Errorf("benchmark: read the direct arm of %q: %w", pairID, err)
	}
	rp := RenderPair(platformText, directText)

	position := positionPlatformA
	renderA, renderB := rp.Platform, rp.Direct
	if p.rand(2) == 1 {
		position, renderA, renderB = positionPlatformB, rp.Direct, rp.Platform
	}
	if err := p.Store.db.WriteTx(ctx, func(tx *sql.Tx) error {
		return p.Store.updatePairTx(ctx, tx, pairID,
			`state = ?, position = ?, render_a = ?, render_b = ?,
			 length_platform = ?, length_direct = ?`,
			StateRendered, position, renderA, renderB, rp.LengthPlatform, rp.LengthDirect)
	}); err != nil {
		return Pair{}, err
	}
	return p.Store.Pair(ctx, pairID)
}

// Decline records the requester's veto of a sampled pair (BENCH-REG §4.2.5).
//
// A declined pair STILL writes its §14 record, with the decline flag set and no
// verdict, no guess and no artifact refs — because "declines are logged and the
// decline rate is reported next to every win rate: selective participation must
// be visible, not silent". The record and the row's move to `declined` commit in
// one transaction, exactly as a verdict does.
func (p *Practice) Decline(ctx context.Context, pairID string) (Pair, error) {
	return p.record(ctx, pairID, Answer{}, true)
}

// RecordVerdict is BENCH-REG §3.3's ONE act: the blind pick and the arm-guess,
// together. A guess-less verdict cannot be expressed (Answer's fields are
// unexported and NewAnswer requires the guess), and the zero Answer is refused
// here, so the refusal is structural rather than a validation a caller could
// route around.
//
// The §14 record append and the pair's move to `recorded` commit in ONE
// WriteTx (§3.4: "the full §14 record is written before identity reveal
// completes"), together with the epoch assignment and the §12 alarm evaluation.
// A crash mid-record therefore leaves the pair awaiting its verdict, never a
// revealed pair with no record.
func (p *Practice) RecordVerdict(ctx context.Context, pairID string, a Answer) (Pair, error) {
	if !a.Formed() {
		return Pair{}, ErrGuessRequired
	}
	return p.record(ctx, pairID, a, false)
}

func (p *Practice) record(ctx context.Context, pairID string, a Answer, declined bool) (Pair, error) {
	var out Pair
	err := p.Store.db.WriteTx(ctx, func(tx *sql.Tx) error {
		pair, err := p.Store.pairTx(ctx, tx, pairID)
		if err != nil {
			return err
		}
		if err := admitsRecord(pair, declined); err != nil {
			return err
		}
		pair.Declined = declined
		if !declined {
			pair.Verdict, pair.Guess = string(a.Choice()), string(a.Guess())
		}

		// Both arms' EXACT OBSERVED model identities and measured consumption
		// (§14). The direct arm's surface cannot be pinned, so its identity is
		// whatever metering observed for its run — recorded as observed (§6).
		plat := p.armReading(ctx, tx, pair.PlatformRunID)
		direct := armReading{}
		if !declined {
			direct = p.armReading(ctx, tx, pair.DirectRunID)
			pair.ParityNote = p.parityNote(ctx, tx, pair.DirectRunID)
		}
		pair.PlatformModel, pair.PlatformUSD = plat.model, plat.usd
		pair.DirectModel, pair.DirectUSD = direct.model, direct.usd
		pair.DirectMeasured = direct.measured
		statementSource := ""
		if !declined {
			statementSource = p.statementSourceTx(ctx, tx, pair.DirectRunID)
		}

		// The epoch is decided by the DIRECT arm's observed identity and by
		// nothing else — a platform-arm model change is recorded per pair and
		// never splits an epoch (§9, frozen).
		epoch, err := p.assignEpochTx(ctx, tx, pair.Domain, direct.model)
		if err != nil {
			return err
		}
		pair.EpochID = epoch
		pair.RecordedTS = p.now()

		payload, err := json.Marshal(recordPayload(pair, statementSource))
		if err != nil {
			return err
		}
		if _, err := p.Log.AppendTx(ctx, tx, eventlog.Append{
			UserID: pair.UserID, Type: EventPairRecorded, SchemaVersion: eventSchemaVersion,
			Payload: payload, Time: pair.RecordedTS,
		}); err != nil {
			return err
		}
		state := StateRecorded
		if declined {
			state = StateDeclined
		}
		if err := p.Store.updatePairTx(ctx, tx, pairID,
			`state = ?, verdict = ?, guess = ?, declined = ?, epoch_id = ?,
			 platform_model = ?, direct_model = ?, platform_usd = ?, direct_usd = ?,
			 direct_measured = ?, parity_note = ?, recorded_ts = ?`,
			state, pair.Verdict, pair.Guess, boolInt(declined), pair.EpochID,
			pair.PlatformModel, pair.DirectModel, pair.PlatformUSD, pair.DirectUSD,
			boolInt(pair.DirectMeasured), pair.ParityNote,
			pair.RecordedTS.Format(time.RFC3339Nano)); err != nil {
			return err
		}
		// §12: the alarm is evaluated at any per-pair update, in the same
		// transaction, so the record and the alarm it implies can never disagree.
		if err := p.evaluateAlarmTx(ctx, tx, pair.Domain, pair.EpochID); err != nil {
			return err
		}
		out, err = p.Store.pairTx(ctx, tx, pairID)
		return err
	})
	if err != nil {
		return Pair{}, err
	}
	if out.ParityNote != "" {
		p.logger().Warn("benchmark: arm-parity note recorded on a pair — coordinator attention (BENCH-REG §6)",
			"pair", pairID, "note", out.ParityNote)
	}
	return out, nil
}

// admitsRecord enforces the §3 order. A declined pair may be vetoed from any
// pre-record state (the requester can decline the moment they are asked); a
// verdict needs a rendered pair, because a verdict without both rendered arms
// would not be a blind comparison.
func admitsRecord(pair Pair, declined bool) error {
	switch pair.State {
	case StateRecorded, StateDeclined:
		return fmt.Errorf("%w: pair %q is already %s (BENCH-REG §17: a result is never recomputed)", ErrPairState, pair.PairID, pair.State)
	}
	if !declined && pair.State != StateRendered {
		return fmt.Errorf("%w: pair %q is %s, want %s — a verdict needs both arms rendered blind", ErrPairState, pair.PairID, pair.State, StateRendered)
	}
	return nil
}

// armReading is one arm's observed identity and measured consumption.
type armReading struct {
	model    string
	usd      float64
	measured bool
}

// armReading reads what metering measured for an arm's run. An absent or
// unmeasured run yields measured=false and an empty identity — the record says
// so rather than guessing, and only measured pairs feed the §13 median.
func (p *Practice) armReading(ctx context.Context, tx *sql.Tx, runID string) armReading {
	if runID == "" || p.Ledger == nil {
		return armReading{}
	}
	rc, err := p.Ledger.RunConsumptionTx(ctx, tx, runID)
	if err != nil {
		return armReading{}
	}
	return armReading{
		model:    observedModels(rc),
		usd:      rc.TotalPricedUSD,
		measured: rc.TotalCalls > 0,
	}
}

// observedModels renders the distinct model identities a run actually consumed,
// in stable order. A run that used more than one model records all of them:
// §14 asks for the exact OBSERVED identities, and collapsing them to a
// representative would be a claim the measurement does not support.
func observedModels(rc metering.RunConsumption) string {
	seen := map[string]bool{}
	var models []string
	for _, it := range rc.Items {
		if it.Model == "" || seen[it.Model] {
			continue
		}
		seen[it.Model] = true
		models = append(models, it.Model)
	}
	sort.Strings(models)
	return strings.Join(models, "+")
}

// cleanTerminals are the run outcomes that mean "the direct arm finished on its
// own terms". Anything else means the platform intervened in the arm's life —
// which is exactly the §6 parity fact worth recording. They are matched by
// VALUE against the runs row rather than through a transition ident, because
// this package never moves a run between states.
var cleanTerminals = map[string]bool{"completed": true, "finalized": true}

// parityNote records a §6 arm-parity note when an ordinary platform mechanism
// truncated the direct arm. Ordinary metering and ceilings apply to the direct
// arm as platform economics, not as arm shaping — but a ceiling that CUT the arm
// short changed what "take what comes" produced, and §6 is explicit that parity
// gaps beyond its declaration are new registration material, never silently
// absorbed. The note rides the pair record and is surfaced on the readout.
func (p *Practice) parityNote(ctx context.Context, tx *sql.Tx, runID string) string {
	if runID == "" {
		return ""
	}
	var state string
	if err := tx.QueryRowContext(ctx, `SELECT state FROM runs WHERE run_id = ?`, runID).Scan(&state); err != nil {
		return ""
	}
	if cleanTerminals[state] {
		return ""
	}
	return fmt.Sprintf("direct-arm run ended in state %q rather than on its own terms — "+
		"an ordinary platform ceiling or interruption may have truncated the single-shot arm; "+
		"parity gap beyond the BENCH-REG §6 declaration, flagged for the coordinator", state)
}

// RecordPayload is the BENCH-REG §14 record, verbatim in its minimums:
// pair id · domain · task class · timestamps · both arms' exact observed model
// identities/versions · both arms' measured consumption · refs to both
// rendered-blind artifacts · position assignment · verdict · requester's
// platform-guess · decline flag · epoch id · requester id.
//
// Fields beyond that list are ADDITIVE — §14 registers minimums, and the extra
// provenance (phase, rate, run links, the domain-resolution source) is what
// makes a record auditable years later. Nothing additive is a decision input.
type RecordPayload struct {
	PairID      string `json:"pair_id"`
	Domain      string `json:"domain"`
	TaskClass   string `json:"task_class"`
	SampledTS   string `json:"sampled_ts"`
	RecordedTS  string `json:"recorded_ts"`
	RequesterID string `json:"requester_id"`
	EpochID     string `json:"epoch_id"`

	PlatformModel string  `json:"platform_model"`
	DirectModel   string  `json:"direct_model"`
	PlatformUSD   float64 `json:"platform_consumption_usd"`
	DirectUSD     float64 `json:"direct_consumption_usd"`
	// DirectMeasured is false when the direct arm produced no measured usage;
	// only measured pairs feed the §13 done-directly median.
	DirectMeasured bool `json:"direct_measured"`

	ArtifactARef string `json:"artifact_a_ref"`
	ArtifactBRef string `json:"artifact_b_ref"`
	Position     string `json:"position"`

	Verdict string `json:"verdict"`
	// PlatformGuess is the requester's guess of which SIDE was the platform's;
	// GuessCorrect is that guess scored, which is the §5 blindness measurement.
	PlatformGuess string `json:"platform_guess"`
	GuessCorrect  bool   `json:"guess_correct"`
	Declined      bool   `json:"declined"`

	// Additive provenance.
	TaskID        string `json:"task_id"`
	DeliverableID string `json:"deliverable_id"`
	PlatformRunID string `json:"platform_run_id"`
	DirectRunID   string `json:"direct_run_id"`
	DomainSource  string `json:"domain_source"`
	// StatementSource names the S06 artifact of record the §2 frozen statement
	// was drawn from — the document both arms answered, pinned by content hash.
	// Empty on a declined pair (no direct arm ran) and on a pair whose birth
	// detail is unreadable; it is never guessed.
	StatementSource string `json:"statement_source,omitempty"`
	Phase           string `json:"phase"`
	RatePct         int    `json:"rate_pct"`
	LengthPlatform  int    `json:"length_platform"`
	LengthDirect    int    `json:"length_direct"`
	ParityNote      string `json:"parity_note,omitempty"`
	// Retention names the record's class on the payload itself: benchmark
	// records are keep-forever (BENCH-REG §14; Spec S14.9 keep-forever set).
	// ENFORCEMENT of the class is B5-8's compaction pass; this marks it.
	Retention string `json:"retention"`
	// ProtocolRef names the registration this record was produced under, so a
	// later amendment (§17) can never make an old record ambiguous: results
	// computed under a prior registration are never retroactively recomputed.
	ProtocolRef string `json:"protocol_ref"`
}

func recordPayload(p Pair, statementSource string) RecordPayload {
	rp := RecordPayload{
		StatementSource: statementSource,
		PairID:          p.PairID, Domain: p.Domain, TaskClass: p.TaskClass,
		SampledTS: p.SampledTS.Format(time.RFC3339Nano), RecordedTS: p.RecordedTS.Format(time.RFC3339Nano),
		RequesterID: p.UserID, EpochID: p.EpochID,
		PlatformModel: p.PlatformModel, DirectModel: p.DirectModel,
		PlatformUSD: p.PlatformUSD, DirectUSD: p.DirectUSD, DirectMeasured: p.DirectMeasured,
		Position: p.Position, Verdict: p.Verdict, PlatformGuess: p.Guess,
		GuessCorrect: p.GuessedPlatform(), Declined: p.Declined,
		TaskID: p.TaskID, DeliverableID: p.DeliverableID,
		PlatformRunID: p.PlatformRunID, DirectRunID: p.DirectRunID,
		DomainSource: p.DomainSource, Phase: p.Phase, RatePct: p.RatePct,
		LengthPlatform: p.LengthPlat, LengthDirect: p.LengthDirect,
		ParityNote: p.ParityNote,
		Retention:  "keep-forever",
		// The protocol is the registration, not this package.
		ProtocolRef: RegistrationRef,
	}
	// A declined pair records no verdict, no guess and NO ARTIFACTS: the
	// requester vetoed before the comparison happened, so there is nothing
	// blind to point at (§4.2.5).
	if !p.Declined {
		rp.ArtifactARef = ArtifactRef(p.PairID, SideA)
		rp.ArtifactBRef = ArtifactRef(p.PairID, SideB)
	}
	return rp
}

// ── The reveal (BENCH-REG §3.4) ──────────────────────────────────────────────

// RecordedPair is the post-record view: the committed §14 record, arms and all.
type RecordedPair struct {
	Seq int64 `json:"seq"`
	RecordPayload
	// PlatformSide is the reveal itself — which blind side was the platform's.
	PlatformSide Side `json:"platform_side"`
}

// Reveal returns a pair's committed §14 record, which is the only place arm
// identity is readable. It reads the EVENT LOG, not the pair row: "arm identity
// revealed only after the verdict is recorded" is implemented as "the reveal is
// a read of the record", so there is no mutable reveal state to get out of step
// with the record, and a pair whose record has not landed simply has nothing to
// reveal.
func (p *Practice) Reveal(ctx context.Context, pairID string) (RecordedPair, error) {
	var (
		seq     int64
		payload string
	)
	err := p.Store.db.QueryRowContext(ctx,
		`SELECT event_seq, payload FROM run_events
		  WHERE type = ? AND json_extract(payload, '$.pair_id') = ?
		  ORDER BY event_seq DESC LIMIT 1`, EventPairRecorded, pairID).Scan(&seq, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return RecordedPair{}, fmt.Errorf("%w: %q has no committed record — arm identity is revealed only after the record is written (BENCH-REG §3.4)", ErrUnknownPair, pairID)
	}
	if err != nil {
		return RecordedPair{}, fmt.Errorf("benchmark: read record for %q: %w", pairID, err)
	}
	out := RecordedPair{Seq: seq}
	if err := json.Unmarshal([]byte(payload), &out.RecordPayload); err != nil {
		return RecordedPair{}, fmt.Errorf("benchmark: decode record for %q: %w", pairID, err)
	}
	out.PlatformSide = SideA
	if out.Position == positionPlatformB {
		out.PlatformSide = SideB
	}
	return out, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
