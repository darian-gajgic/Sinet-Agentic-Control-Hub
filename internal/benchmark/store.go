package benchmark

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// store.go is the control-plane state of the practice over migration 0014:
// the in-flight pair, the per-domain epoch/first-open bookkeeping, and the
// sampler cursor. Pair identity is immutable and a pair is never deleted
// (triggers); a recorded pair is frozen, because §17 forbids recomputing a
// result under a later registration.
//
// The RECORD is the event. A pair's move to `recorded` and the
// benchmark.pair_recorded append commit in ONE WriteTx (CONVENTIONS §6/§8), and
// the reveal reads that committed event — never this table's mutable state
// (BENCH-REG §3.4).

// Store reads and writes the benchmark tables.
type Store struct {
	db  *storage.DB
	log *eventlog.Log
	// Now is the clock seam; nil means the wall clock.
	Now Clock
}

// NewStore returns a Store over db appending through log.
func NewStore(db *storage.DB, log *eventlog.Log) *Store {
	return &Store{db: db, log: log}
}

func (s *Store) now() time.Time { return s.Now.now() }

func (s *Store) ts() string { return s.now().Format(time.RFC3339Nano) }

// Pair is a full pair row. It carries arm-identifying columns (position, the
// two run ids and, once recorded, the two model identities), so it is the
// package's own working view: no pre-record READ SURFACE returns it. The blind
// voter's view is PendingPair, and the post-record view is the committed record
// (RecordedPair, read from the event log).
type Pair struct {
	PairID        string
	UserID        string
	Domain        string
	DomainSource  string
	TaskID        string
	DeliverableID string
	TaskClass     string
	PlatformRunID string
	Phase         string
	RatePct       int
	SourceSeq     int64
	SampledTS     time.Time

	State        string
	DirectRunID  string
	Position     string
	RenderA      string
	RenderB      string
	LengthPlat   int
	LengthDirect int
	Verdict      string
	Guess        string
	Declined     bool
	EpochID      string

	PlatformModel  string
	DirectModel    string
	PlatformUSD    float64
	DirectUSD      float64
	DirectMeasured bool
	ParityNote     string
	RecordedTS     time.Time
}

// PlatformSide returns which blind side the platform arm was rendered into.
// It is arm-identity data and is used only inside the package's record path.
func (p Pair) PlatformSide() Side {
	if p.Position == positionPlatformB {
		return SideB
	}
	return SideA
}

// GuessedPlatform reports whether the requester's guess named the platform arm
// (BENCH-REG §5: blindness is measured per vote).
func (p Pair) GuessedPlatform() bool { return p.Guess != "" && Side(p.Guess) == p.PlatformSide() }

const pairColumns = `pair_id, user_id, domain, domain_source, task_id, deliverable_id,
	task_class, platform_run_id, phase, rate_pct, source_seq, sampled_ts,
	state, direct_run_id, position, render_a, render_b, length_platform, length_direct,
	verdict, guess, declined, epoch_id, platform_model, direct_model,
	platform_usd, direct_usd, direct_measured, parity_note, COALESCE(recorded_ts, '')`

func scanPair(row interface{ Scan(...any) error }) (Pair, error) {
	var (
		p                   Pair
		sampled, recordedTS string
		declined, measured  int
	)
	err := row.Scan(&p.PairID, &p.UserID, &p.Domain, &p.DomainSource, &p.TaskID, &p.DeliverableID,
		&p.TaskClass, &p.PlatformRunID, &p.Phase, &p.RatePct, &p.SourceSeq, &sampled,
		&p.State, &p.DirectRunID, &p.Position, &p.RenderA, &p.RenderB, &p.LengthPlat, &p.LengthDirect,
		&p.Verdict, &p.Guess, &declined, &p.EpochID, &p.PlatformModel, &p.DirectModel,
		&p.PlatformUSD, &p.DirectUSD, &measured, &p.ParityNote, &recordedTS)
	if err != nil {
		return Pair{}, err
	}
	p.SampledTS, p.RecordedTS = parseTS(sampled), parseTS(recordedTS)
	p.Declined, p.DirectMeasured = declined == 1, measured == 1
	return p, nil
}

// NewPair is a sampled pair to insert (the immutable identity block).
type NewPair struct {
	PairID        string
	UserID        string
	Domain        string
	DomainSource  string
	TaskID        string
	DeliverableID string
	TaskClass     string
	PlatformRunID string
	Phase         string
	RatePct       int
	SourceSeq     int64
}

// CreateTx inserts a sampled pair inside the caller's transaction, so the draw
// and the sampler-cursor advance commit together and a crash can neither
// double-sample nor silently drop a draw.
func (s *Store) CreateTx(ctx context.Context, tx *sql.Tx, n NewPair) (Pair, error) {
	switch {
	case n.PairID == "":
		return Pair{}, fmt.Errorf("%w: empty pair id", ErrBadInput)
	case n.UserID == "":
		return Pair{}, fmt.Errorf("%w: empty user_id (owner attribution, 15.6)", ErrBadInput)
	case !IsRegisteredDomain(n.Domain):
		return Pair{}, fmt.Errorf("%w: %q", ErrUnregisteredDomain, n.Domain)
	case n.Phase != PhasePreGate && n.Phase != PhaseMaintenance:
		return Pair{}, fmt.Errorf("%w: phase %q is not a declared benchmark.sampling_rate phase", ErrBadInput, n.Phase)
	}
	ts := s.ts()
	_, err := tx.ExecContext(ctx,
		`INSERT INTO benchmark_pairs
		   (pair_id, user_id, domain, domain_source, task_id, deliverable_id, task_class,
		    platform_run_id, phase, rate_pct, source_seq, sampled_ts, state, updated_ts)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.PairID, n.UserID, n.Domain, n.DomainSource, n.TaskID, n.DeliverableID, n.TaskClass,
		n.PlatformRunID, n.Phase, n.RatePct, n.SourceSeq, ts, StateSampled, ts)
	if err != nil {
		return Pair{}, fmt.Errorf("benchmark: create pair %q: %w", n.PairID, err)
	}
	return s.pairTx(ctx, tx, n.PairID)
}

// Pair returns one pair row.
func (s *Store) Pair(ctx context.Context, pairID string) (Pair, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+pairColumns+` FROM benchmark_pairs WHERE pair_id = ?`, pairID)
	p, err := scanPair(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Pair{}, fmt.Errorf("%w: %q", ErrUnknownPair, pairID)
	}
	if err != nil {
		return Pair{}, fmt.Errorf("benchmark: read pair %q: %w", pairID, err)
	}
	return p, nil
}

func (s *Store) pairTx(ctx context.Context, tx *sql.Tx, pairID string) (Pair, error) {
	row := tx.QueryRowContext(ctx, `SELECT `+pairColumns+` FROM benchmark_pairs WHERE pair_id = ?`, pairID)
	p, err := scanPair(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Pair{}, fmt.Errorf("%w: %q", ErrUnknownPair, pairID)
	}
	if err != nil {
		return Pair{}, fmt.Errorf("benchmark: read pair %q: %w", pairID, err)
	}
	return p, nil
}

// PairsInDomain returns every pair of a domain in sampled order. Callers filter
// by epoch themselves, because §9 makes the epoch scope a decision the caller
// must state explicitly: a decision statistic uses the current epoch only, while
// the §13 done-directly figure and the §16 history views deliberately do not.
func (s *Store) PairsInDomain(ctx context.Context, domain string) ([]Pair, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+pairColumns+` FROM benchmark_pairs WHERE domain = ? ORDER BY sampled_ts, pair_id`, domain)
	if err != nil {
		return nil, fmt.Errorf("benchmark: list pairs in %q: %w", domain, err)
	}
	defer rows.Close()
	var out []Pair
	for rows.Next() {
		p, err := scanPair(rows)
		if err != nil {
			return nil, fmt.Errorf("benchmark: scan pair: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// updatePairTx applies a working-state update and refreshes updated_ts. The
// identity columns are never in `set` — the migration's trigger enforces that
// independently of this helper.
func (s *Store) updatePairTx(ctx context.Context, tx *sql.Tx, pairID, set string, args ...any) error {
	args = append(args, s.ts(), pairID)
	res, err := tx.ExecContext(ctx,
		`UPDATE benchmark_pairs SET `+set+`, updated_ts = ? WHERE pair_id = ?`, args...)
	if err != nil {
		return fmt.Errorf("benchmark: update pair %q: %w", pairID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: %q", ErrUnknownPair, pairID)
	}
	return nil
}

// ── The blind read surface (BENCH-REG §3.4) ──────────────────────────────────

// PendingPair is the ONLY pre-record read surface for a pair awaiting its
// verdict. It carries no arm identity, no position mapping and no model
// identity — not because a caller is trusted to ignore them, but because the
// struct has no such field and the query that fills it selects no such column.
// That is BENCH-REG §3.4 ("Arm identity revealed only after the verdict is
// recorded") made structural.
type PendingPair struct {
	PairID    string    `json:"pair_id"`
	UserID    string    `json:"user_id"`
	Domain    string    `json:"domain"`
	TaskID    string    `json:"task_id"`
	SampledTS time.Time `json:"sampled_ts"`
	// RenderA / RenderB are the two deliverables through the ONE uniform
	// presentation template, keyed by SIDE. Which arm is which is exactly what
	// the voter must not know.
	RenderA string `json:"render_a"`
	RenderB string `json:"render_b"`
	// LengthA / LengthB report the §3.2 length confound by SIDE. Length is
	// reported, never "corrected" — and reporting it by side leaks nothing.
	LengthA int `json:"length_a"`
	LengthB int `json:"length_b"`
}

// pendingColumns is the pre-record projection. Adding an arm-identifying column
// here would break §3.4; pair_reveal_test.go asserts the set.
const pendingColumns = `pair_id, user_id, domain, task_id, sampled_ts, render_a, render_b,
	length(render_a), length(render_b)`

// PendingVerdicts returns the pairs awaiting a verdict, scoped to one voter.
// The requester IS the voter (BENCH-REG §2/§3.3), so the card is theirs; the
// operator sees all, as everywhere (Spec S01.9).
func (s *Store) PendingVerdicts(ctx context.Context, userID string) ([]PendingPair, error) {
	q := `SELECT ` + pendingColumns + ` FROM benchmark_pairs WHERE state = ?`
	args := []any{StateRendered}
	if userID != "" {
		q += ` AND user_id = ?`
		args = append(args, userID)
	}
	q += ` ORDER BY sampled_ts, pair_id`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("benchmark: pending verdicts: %w", err)
	}
	defer rows.Close()
	out := []PendingPair{}
	for rows.Next() {
		var (
			p       PendingPair
			sampled string
		)
		if err := rows.Scan(&p.PairID, &p.UserID, &p.Domain, &p.TaskID, &sampled,
			&p.RenderA, &p.RenderB, &p.LengthA, &p.LengthB); err != nil {
			return nil, fmt.Errorf("benchmark: scan pending verdict: %w", err)
		}
		p.SampledTS = parseTS(sampled)
		out = append(out, p)
	}
	return out, rows.Err()
}

// ── Domain state (BENCH-REG §9, §11; migration 0014 benchmark_domains) ───────

// DomainState is a registered domain's practice bookkeeping.
type DomainState struct {
	Domain string `json:"domain"`
	// FirstGateOpened is zero until the domain's gate has opened once. It
	// selects the §4.1 sampling phase and nothing else.
	FirstGateOpened time.Time `json:"first_gate_opened,omitempty"`
	CurrentEpochID  string    `json:"current_epoch_id"`
	EpochStarted    time.Time `json:"epoch_started"`
	// EpochIdentity is the direct-arm model identity this epoch is defined by;
	// empty before the domain's first recorded pair.
	EpochIdentity string `json:"epoch_identity"`
	EpochSeq      int    `json:"epoch_seq"`
}

// Phase returns the §4.1 sampling phase for the domain: pre-gate until its gate
// first opens, maintenance afterwards.
func (d DomainState) Phase() string {
	if d.FirstGateOpened.IsZero() {
		return PhasePreGate
	}
	return PhaseMaintenance
}

// EnsureDomain creates the domain's bookkeeping row if absent, opening its
// first epoch. It is idempotent and safe to call at every boot.
func (s *Store) EnsureDomain(ctx context.Context, domain string) (DomainState, error) {
	if !IsRegisteredDomain(domain) {
		return DomainState{}, fmt.Errorf("%w: %q", ErrUnregisteredDomain, domain)
	}
	var st DomainState
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		var err error
		st, err = s.ensureDomainTx(ctx, tx, domain)
		return err
	})
	return st, err
}

func (s *Store) ensureDomainTx(ctx context.Context, tx *sql.Tx, domain string) (DomainState, error) {
	ts := s.ts()
	if _, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO benchmark_domains
		   (domain, current_epoch_id, epoch_started_ts, epoch_seq)
		 VALUES (?, ?, ?, 1)`, domain, epochID(domain, 1), ts); err != nil {
		return DomainState{}, fmt.Errorf("benchmark: ensure domain %q: %w", domain, err)
	}
	return s.domainTx(ctx, tx, domain)
}

// Domain returns a domain's bookkeeping row.
func (s *Store) Domain(ctx context.Context, domain string) (DomainState, error) {
	row := s.db.QueryRowContext(ctx, domainQuery, domain)
	return scanDomain(row, domain)
}

func (s *Store) domainTx(ctx context.Context, tx *sql.Tx, domain string) (DomainState, error) {
	row := tx.QueryRowContext(ctx, domainQuery, domain)
	return scanDomain(row, domain)
}

const domainQuery = `SELECT domain, COALESCE(first_gate_opened_ts, ''), current_epoch_id,
	epoch_started_ts, epoch_identity, epoch_seq FROM benchmark_domains WHERE domain = ?`

func scanDomain(row interface{ Scan(...any) error }, domain string) (DomainState, error) {
	var (
		st              DomainState
		opened, started string
	)
	err := row.Scan(&st.Domain, &opened, &st.CurrentEpochID, &started, &st.EpochIdentity, &st.EpochSeq)
	if errors.Is(err, sql.ErrNoRows) {
		return DomainState{}, fmt.Errorf("%w: %q has no bookkeeping row", ErrUnregisteredDomain, domain)
	}
	if err != nil {
		return DomainState{}, fmt.Errorf("benchmark: read domain %q: %w", domain, err)
	}
	st.FirstGateOpened, st.EpochStarted = parseTS(opened), parseTS(started)
	return st, nil
}

// epochID is the stable per-domain epoch identity carried on every record
// (BENCH-REG §14 "epoch id"). It is derived from the domain and the epoch
// sequence rather than from a model name, because the direct arm's identity
// string is the consumer surface's to change and an id must not.
func epochID(domain string, seq int) string { return fmt.Sprintf("%s#e%d", domain, seq) }

// ── The sampler cursor ───────────────────────────────────────────────────────

// samplerCursorTx reads the durable sampling cursor inside a transaction.
func (s *Store) samplerCursorTx(ctx context.Context, tx *sql.Tx) (int64, error) {
	var seq int64
	err := tx.QueryRowContext(ctx, `SELECT last_seq FROM benchmark_sampler WHERE row_id = 'sampler'`).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("benchmark: read sampler cursor: %w", err)
	}
	return seq, nil
}

// SamplerCursor reports how far the sampling hook has consumed the event log.
func (s *Store) SamplerCursor(ctx context.Context) (int64, error) {
	var seq int64
	err := s.db.QueryRowContext(ctx, `SELECT last_seq FROM benchmark_sampler WHERE row_id = 'sampler'`).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("benchmark: read sampler cursor: %w", err)
	}
	return seq, nil
}

// advanceCursorTx moves the durable cursor forward. It never moves backwards:
// a stale pass cannot un-consume events another pass already decided on.
func (s *Store) advanceCursorTx(ctx context.Context, tx *sql.Tx, seq int64) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE benchmark_sampler SET last_seq = ?, updated_ts = ?
		  WHERE row_id = 'sampler' AND last_seq < ?`, seq, s.ts(), seq)
	if err != nil {
		return fmt.Errorf("benchmark: advance sampler cursor: %w", err)
	}
	return nil
}

// ── The standing opt-in (BENCH-REG §4.2.1) ───────────────────────────────────

// optedInTx reports whether a user has the standing benchmark opt-in enabled.
// A user row that does not exist is not opted in — the honest reading of an
// opt-IN.
func (s *Store) optedInTx(ctx context.Context, tx *sql.Tx, userID string) (bool, error) {
	var v int
	err := tx.QueryRowContext(ctx, `SELECT benchmark_opt_in FROM users WHERE user_id = ?`, userID).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("benchmark: read opt-in for %q: %w", userID, err)
	}
	return v == 1, nil
}

// OptedIn reports whether a user has the standing benchmark opt-in enabled.
func (s *Store) OptedIn(ctx context.Context, userID string) (bool, error) {
	var v int
	err := s.db.QueryRowContext(ctx, `SELECT benchmark_opt_in FROM users WHERE user_id = ?`, userID).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("benchmark: read opt-in for %q: %w", userID, err)
	}
	return v == 1, nil
}

// parseTS parses a stored RFC3339 timestamp, returning the zero time for an
// empty or malformed value (timestamps are recorded, never an ordering
// authority — P-T07-4).
func parseTS(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC()
	}
	return time.Time{}
}
