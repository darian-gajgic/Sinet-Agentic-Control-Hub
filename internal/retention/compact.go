package retention

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
)

// compact.go — the S14.9 ¶2 scheduled compaction pass.
//
//	"At ⚙ retention.compaction_horizon = 6 months (per-user, 13.4): the
//	 scheduled compaction pass strips bulky event payloads and transcript
//	 copy-asides. Keep-forever set: run summaries, verdicts, decisions,
//	 receipts, routing records, drift events, benchmark records. The pass logs
//	 itself (retention.compacted) — the audit trail records its own compaction."
//
// Three properties hold by construction:
//
//   - NO ROW IS EVER DELETED. event_seq is the sole ordering authority (S14.1)
//     and checkpoints.event_seq is a FK into run_events (0001). The pass elides
//     BODIES. 0015's run_events_no_delete is untouched and still aborts.
//   - THE HORIZON IS PER-USER. ⚙ retention.compaction_horizon is PerUser, so it
//     is read with IntFor for each owner and two owners genuinely get two
//     boundaries. Reading it with Int would silently apply one horizon to the
//     whole household.
//   - THE PASS IS IDEMPOTENT AND RESTART-SAFE. It holds no cursor: what to
//     strip is a pure function of the horizon, the row's ts and its type, and
//     an already-stripped row is excluded by the same predicate that selected
//     it. A second pass over the same window is a no-op. (Migration 0015's
//     trigger also refuses a second strip outright.)
//
// The package owns the LOGIC; the shell owns WHEN. CompactionInterval below is
// the driver's cadence, not a decision this package makes.

// CompactionInterval is the shell driver's tick for the compaction pass — a
// STRUCTURAL CONSTANT, not a ⚙ row (the §35 sampling-loop precedent; S18
// ratifies no cadence key and ⚙ retention.compaction_horizon is a HORIZON in
// months, not an interval). Its reason: the horizon is months, so the tick
// decides only how promptly a months-old payload is stripped — never WHAT gets
// stripped, which is a pure function of the horizon and the row. Daily-
// equivalent. Interim under the standing settings-tab directive.
const CompactionInterval = 24 * time.Hour

// compactBatch bounds one UPDATE batch so a very long-lived database does not
// build one enormous transaction. Structural: it changes only how many
// transactions a pass takes, never its result (the pass is idempotent and its
// predicate is stable across batches).
const compactBatch = 500

// OwnerPass is one owner's leg of a compaction pass.
type OwnerPass struct {
	UserID           string           `json:"user_id"`
	HorizonMonths    int64            `json:"horizon_months"`
	BoundaryTS       string           `json:"boundary_ts"`
	BoundaryEventSeq int64            `json:"boundary_event_seq"`
	EventsStripped   int64            `json:"events_stripped"`
	BytesReclaimed   int64            `json:"bytes_reclaimed"`
	ByFamily         map[string]int64 `json:"by_family,omitempty"`
	// Transcript copy-asides (S02.2 engine_sessions): the file on disk and the
	// row's reference to it are both cleared.
	TranscriptsStripped int64 `json:"transcripts_stripped"`
	TranscriptErrors    int64 `json:"transcript_errors,omitempty"`
}

// PassResult is one whole compaction pass.
type PassResult struct {
	AsOf           string      `json:"as_of"`
	Owners         []OwnerPass `json:"owners"`
	EventsStripped int64       `json:"events_stripped"`
	BytesReclaimed int64       `json:"bytes_reclaimed"`
	// EventSeq is the retention.compacted row this pass logged itself as, or 0
	// when the pass stripped nothing and therefore recorded nothing.
	EventSeq int64 `json:"-"`
}

func (r PassResult) transcriptsStripped() int64 {
	var n int64
	for _, o := range r.Owners {
		n += o.TranscriptsStripped
	}
	return n
}

// Compact runs one compaction pass as of asOf (zero = the Store clock) and logs
// itself as retention.compacted. Safe to call at any cadence.
//
// The pass event is PLATFORM-SCOPE with per-owner counts in its payload: the
// operator sees the whole pass, and no member's audit view gains another
// member's counts (a per-owner event would put one owner's boundary in another
// owner's stream only by being read there anyway).
func (s *Store) Compact(ctx context.Context, asOf time.Time) (PassResult, error) {
	if asOf.IsZero() {
		asOf = s.now()
	}
	asOf = asOf.UTC()

	owners, err := s.compactionOwners(ctx)
	if err != nil {
		return PassResult{}, err
	}
	res := PassResult{AsOf: asOf.Format(time.RFC3339Nano)}
	for _, owner := range owners {
		leg, err := s.compactOwner(ctx, owner, asOf)
		if err != nil {
			return PassResult{}, err
		}
		res.Owners = append(res.Owners, leg)
		res.EventsStripped += leg.EventsStripped
		res.BytesReclaimed += leg.BytesReclaimed
	}

	// "The pass logs itself — the audit trail records its own compaction"
	// (S14.9 ¶2). A pass that stripped NOTHING compacted nothing, and there is
	// no compaction for the audit trail to record: on a quiet household the
	// daily tick would otherwise write a no-op row every day forever, until the
	// pass's own records were the bulk of the log it exists to bound. So the
	// event is emitted exactly when something was actually elided.
	if res.EventsStripped == 0 && res.transcriptsStripped() == 0 {
		return res, nil
	}

	payload, err := json.Marshal(res)
	if err != nil {
		return PassResult{}, fmt.Errorf("retention: marshal %s payload: %w", EventCompacted, err)
	}
	seq, err := s.log.Append(ctx, eventlog.Append{
		UserID:        platformOwner,
		Type:          EventCompacted,
		SchemaVersion: eventSchemaVersion,
		Payload:       payload,
	})
	if err != nil {
		return PassResult{}, err
	}
	res.EventSeq = seq
	return res, nil
}

// compactionOwners lists every owner with rows in the log, in a deterministic
// order. Derived from the log itself rather than from the users table: a run's
// owner may have been created before or after any other bookkeeping, and the
// pass must reach every row that exists.
func (s *Store) compactionOwners(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT user_id FROM run_events ORDER BY user_id`)
	if err != nil {
		return nil, fmt.Errorf("retention: list compaction owners: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// compactOwner strips one owner's trace past THEIR horizon.
func (s *Store) compactOwner(ctx context.Context, userID string, asOf time.Time) (OwnerPass, error) {
	months, err := s.horizonFor(userID)
	if err != nil {
		return OwnerPass{}, err
	}
	boundary := asOf.AddDate(0, -int(months), 0).Format(time.RFC3339Nano)
	leg := OwnerPass{
		UserID:        userID,
		HorizonMonths: months,
		BoundaryTS:    boundary,
		ByFamily:      map[string]int64{},
	}

	// The boundary event_seq: the highest seq at or below the boundary for this
	// owner. Reported, never used as the predicate — ts is the horizon's unit
	// and event_seq is only an ordering authority (P-T07-4).
	var maxSeq sql.NullInt64
	if err := s.db.QueryRowContext(ctx,
		`SELECT max(event_seq) FROM run_events WHERE user_id = ? AND ts <= ?`,
		userID, boundary).Scan(&maxSeq); err != nil {
		return OwnerPass{}, fmt.Errorf("retention: boundary seq for %q: %w", userID, err)
	}
	leg.BoundaryEventSeq = maxSeq.Int64

	for {
		n, err := s.stripBatch(ctx, userID, boundary, &leg)
		if err != nil {
			return OwnerPass{}, err
		}
		if n == 0 {
			break
		}
	}

	if err := s.stripTranscripts(ctx, userID, boundary, &leg); err != nil {
		return OwnerPass{}, err
	}
	if len(leg.ByFamily) == 0 {
		leg.ByFamily = nil
	}
	return leg, nil
}

// stripBatch elides one batch of payload bodies and returns how many it moved.
// Selection and update happen in ONE transaction so the counts recorded are the
// counts committed.
func (s *Store) stripBatch(ctx context.Context, userID, boundary string, leg *OwnerPass) (int, error) {
	moved := 0
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		// The predicate: this owner, at or below the boundary, not keep-forever,
		// not already compacted. The keep-forever test is the SEEDED ALLOWLIST
		// (one source of truth with the export view and with the Go family
		// registry that seeds it) — never a second hand-written type list.
		rows, err := tx.QueryContext(ctx,
			`SELECT e.event_seq, e.type, length(e.payload)
			   FROM run_events e
			   LEFT JOIN retention_keep_forever k ON k.type = e.type
			  WHERE e.user_id = ?
			    AND e.ts <= ?
			    AND k.type IS NULL
			    AND e.payload <> ?
			  ORDER BY e.event_seq
			  LIMIT ?`,
			userID, boundary, CompactedPayload, compactBatch)
		if err != nil {
			return fmt.Errorf("retention: select compactable rows for %q: %w", userID, err)
		}
		type target struct {
			seq   int64
			typ   string
			bytes int64
		}
		var targets []target
		for rows.Next() {
			var t target
			if err := rows.Scan(&t.seq, &t.typ, &t.bytes); err != nil {
				rows.Close()
				return err
			}
			targets = append(targets, t)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()

		for _, t := range targets {
			if _, err := tx.ExecContext(ctx,
				`UPDATE run_events SET payload = ? WHERE event_seq = ?`, CompactedPayload, t.seq); err != nil {
				return fmt.Errorf("retention: strip payload at seq %d: %w", t.seq, err)
			}
			fam, known := eventlog.Classify(t.typ)
			name := string(fam)
			if !known {
				// Forward tolerance: an unregistered type is trace by default
				// and is counted under a visible label, never silently.
				name = "unregistered"
			}
			leg.ByFamily[name]++
			leg.EventsStripped++
			if delta := t.bytes - int64(len(CompactedPayload)); delta > 0 {
				leg.BytesReclaimed += delta
			}
			moved++
		}
		return nil
	})
	return moved, err
}

// stripTranscripts removes the S02.2 engine_sessions transcript copy-asides
// past the boundary: the FILE on disk and the row's reference to it. The row
// itself stays — it is the reboot-case mining index and its session identity is
// still a fact.
//
// A file that is already gone is not an error (the strip is idempotent and a
// snapshot restore or an operator cleanup may have preceded it); any other
// filesystem error is COUNTED and the reference is left in place, so the next
// pass retries rather than orphaning a file nobody can find again.
func (s *Store) stripTranscripts(ctx context.Context, userID, boundary string, leg *OwnerPass) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT session_key, transcript_copy_path FROM engine_sessions
		  WHERE user_id = ? AND updated_ts <= ? AND transcript_copy_path <> ''
		  ORDER BY session_key`, userID, boundary)
	if err != nil {
		return fmt.Errorf("retention: list transcript copy-asides for %q: %w", userID, err)
	}
	type aside struct {
		key  int64
		path string
	}
	var asides []aside
	for rows.Next() {
		var a aside
		if err := rows.Scan(&a.key, &a.path); err != nil {
			rows.Close()
			return err
		}
		asides = append(asides, a)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, a := range asides {
		if err := s.remove(a.path); err != nil && !os.IsNotExist(err) {
			leg.TranscriptErrors++
			continue
		}
		if err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx,
				`UPDATE engine_sessions SET transcript_copy_path = '' WHERE session_key = ?`, a.key)
			return err
		}); err != nil {
			return fmt.Errorf("retention: clear transcript ref %d: %w", a.key, err)
		}
		leg.TranscriptsStripped++
	}
	return nil
}

// osRemove is the default RemoveFile seam.
func osRemove(path string) error { return os.Remove(path) }

// KeepForeverSeeded returns the seeded allowlist as stored, sorted — the
// queryable form of the boundary for the S14.10 surfaces and for tests that
// assert the DB and the registry agree.
func (s *Store) KeepForeverSeeded(ctx context.Context) ([]KeepForeverEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT type, family, note FROM retention_keep_forever ORDER BY type`)
	if err != nil {
		return nil, fmt.Errorf("retention: read keep-forever allowlist: %w", err)
	}
	defer rows.Close()
	var out []KeepForeverEntry
	for rows.Next() {
		var e KeepForeverEntry
		var fam string
		if err := rows.Scan(&e.Type, &fam, &e.Note); err != nil {
			return nil, err
		}
		e.Family = eventlog.Family(fam)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Type < out[j].Type })
	return out, nil
}
