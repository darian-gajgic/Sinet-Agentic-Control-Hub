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
// Properties that hold by construction:
//
//   - NO ROW IS EVER DELETED. event_seq is the sole ordering authority (S14.1)
//     and checkpoints.event_seq is a FK into run_events (0001). The pass elides
//     BODIES. 0015's run_events_no_delete is untouched and still aborts.
//   - ONLY BULKY PAYLOADS ARE STRIPPED. S14.9 says "bulky event payloads", and
//     the qualifier is load-bearing: eliding a 40-byte auth record destroys
//     content AND grows the file, because the marker is longer than the body it
//     replaces. See BulkPayloadFloorBytes.
//   - KEEP-FOREVER IS ENFORCED AT THE DB LAYER, not only here. Migration 0015's
//     run_events_payload_compaction_only trigger consults the same allowlist
//     table this predicate joins, so no writer at all can elide a keep-forever
//     body — this predicate is the polite path, not the boundary.
//   - THE HORIZON IS PER-USER, and computed in the DATABASE clock domain. ⚙
//     retention.compaction_horizon is PerUser, so it is read with IntFor for
//     each owner; the boundary is derived by SQLite strftime, the same function
//     family the trigger floor uses, so the pass and the guard cannot disagree
//     about what "one month ago" means.
//   - EVERY COMMITTED STRIP IS COVERED BY A COMMITTED AUDIT EVENT. Each unit of
//     work is ONE transaction containing both the elisions and the
//     retention.compacted row that counts them. There is no window in which
//     bodies are gone and nothing records it.
//   - ONE OWNER'S FAILURE NEVER TOUCHES ANOTHER'S. Owners are isolated; a
//     failure is recorded on that owner's leg and the pass continues.
//   - THE PASS IS IDEMPOTENT AND RESTART-SAFE. It holds no cursor: what to strip
//     is a pure function of the horizon, the row's ts, its size and its type,
//     and an already-stripped row is excluded by the same predicate that
//     selected it.
//
// The package owns the LOGIC; the shell owns WHEN.

// CompactionInterval is the shell driver's tick for the compaction pass — a
// STRUCTURAL CONSTANT, not a ⚙ row (the §35 sampling-loop precedent; S18
// ratifies no cadence key and ⚙ retention.compaction_horizon is a HORIZON in
// months, not an interval). Its reason: the horizon is months, so the tick
// decides only how promptly a months-old payload is stripped — never WHAT gets
// stripped, which is a pure function of the horizon and the row. Daily-
// equivalent. Interim under the standing settings-tab directive.
const CompactionInterval = 24 * time.Hour

// BulkPayloadFloorBytes is the "BULKY" in S14.9 ¶2's "strips bulky event
// payloads" — a STRUCTURAL CONSTANT, not a ⚙ row (S18 ratifies no key; the §35
// precedent).
//
// Its reason, in two parts:
//
//   - ARITHMETIC. The compacted marker is itself ~110 bytes. Eliding a payload
//     smaller than that destroys content AND makes the database bigger, which
//     inverts the purpose of the pass. No threshold below the marker's own
//     length can ever be correct, and a reclaimed-bytes figure computed without
//     one cannot even report the loss.
//   - THE REFS-NOT-BLOBS LINE. internal/adapters caps an embedded trace excerpt
//     at 4096 bytes (ExcerptCap, P-T07-5) and every other producer writes refs,
//     not blobs. A payload at or above 1 KiB is therefore carrying embedded
//     CONTENT — a message, a tool result, a brief — which is what "bulky" names.
//     Below it the payload is a structured record whose entire body is smaller
//     than a quarter of one trace excerpt: an auth event, a settings change, an
//     FSM transition. Those are the audit trail, not the bulk, and they survive
//     past the horizon by construction.
//
// Interim under the standing settings-tab directive.
const BulkPayloadFloorBytes = 1024

// compactBatch bounds one unit of work. Structural: it changes only how many
// transactions a pass takes, never its result — the predicate is stable across
// batches and every batch is individually atomic with its own audit row.
const compactBatch = 500

// OwnerPass is one owner's leg of a compaction pass. Riding a
// retention.compacted payload it carries the counts of the ONE committed unit
// that event covers; riding a PassResult it carries the owner's totals.
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
	// Error records an owner-level failure honestly. The counts above are what
	// this owner COMMITTED before it failed — never rolled back, always audited.
	Error string `json:"error,omitempty"`
}

// PassResult is one whole compaction pass.
type PassResult struct {
	AsOf           string      `json:"as_of"`
	Owners         []OwnerPass `json:"owners"`
	EventsStripped int64       `json:"events_stripped"`
	BytesReclaimed int64       `json:"bytes_reclaimed"`
	// FailedOwners names the owners whose leg errored. The pass continues past
	// each one, so this is a list, not a stop.
	FailedOwners []string `json:"failed_owners,omitempty"`
	// AuditEventSeqs are the retention.compacted rows this pass committed — one
	// per committed unit of work, never fewer.
	AuditEventSeqs []int64 `json:"-"`
}

// Compact runs one compaction pass as of asOf (zero = the Store clock).
//
// It returns an error only for a PASS-level failure (owner enumeration, the
// durable stamp). A single owner's failure is recorded on that owner's leg and
// in FailedOwners, and every other owner is compacted normally.
func (s *Store) Compact(ctx context.Context, asOf time.Time) (PassResult, error) {
	if asOf.IsZero() {
		asOf = s.now()
	}
	asOf = asOf.UTC()
	res := PassResult{AsOf: asOf.Format(time.RFC3339Nano)}

	owners, err := s.compactionOwners(ctx)
	if err != nil {
		// The stamp records the failure rather than going stale — a stale stamp
		// reads as a dead driver, which is a different and worse diagnosis.
		_ = s.recordPassState(ctx, res, err)
		return res, err
	}
	for _, owner := range owners {
		leg, legErr := s.compactOwner(ctx, owner, asOf, &res)
		if legErr != nil {
			leg.Error = legErr.Error()
			res.FailedOwners = append(res.FailedOwners, owner)
		}
		res.Owners = append(res.Owners, leg)
		res.EventsStripped += leg.EventsStripped
		res.BytesReclaimed += leg.BytesReclaimed
	}
	if err := s.recordPassState(ctx, res, nil); err != nil {
		return res, err
	}
	return res, nil
}

// compactionOwners lists every owner with rows in the log, in a deterministic
// order. Derived from the log itself rather than from the users table: the pass
// must reach every row that exists.
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

// boundaryFor derives one owner's horizon boundary IN THE DATABASE CLOCK
// DOMAIN. The trigger floor in migration 0015 is `strftime(..., 'now', '-1
// month', '-1 minute')`; computing the pass's boundary with the same function
// family on the same database means the two cannot disagree about calendar
// arithmetic, and the floor's one minute of slack absorbs the residual
// sub-second difference at horizon = 1, where the two coincide.
func (s *Store) boundaryFor(ctx context.Context, asOf time.Time, months int64) (string, error) {
	var boundary string
	err := s.db.QueryRowContext(ctx,
		`SELECT strftime('%Y-%m-%dT%H:%M:%SZ', ?, ?)`,
		asOf.Format(time.RFC3339Nano), fmt.Sprintf("-%d months", months)).Scan(&boundary)
	if err != nil {
		return "", fmt.Errorf("retention: derive horizon boundary (-%d months): %w", months, err)
	}
	if boundary == "" {
		return "", fmt.Errorf("retention: horizon boundary (-%d months) resolved empty", months)
	}
	return boundary, nil
}

// compactOwner strips one owner's trace past THEIR horizon. Every unit it
// commits is already audited by the time it returns, so a partial leg is still
// a complete record of what it did.
func (s *Store) compactOwner(ctx context.Context, userID string, asOf time.Time, res *PassResult) (OwnerPass, error) {
	leg := OwnerPass{UserID: userID, ByFamily: map[string]int64{}}

	months, err := s.horizonFor(userID)
	if err != nil {
		return leg, err
	}
	leg.HorizonMonths = months
	boundary, err := s.boundaryFor(ctx, asOf, months)
	if err != nil {
		return leg, err
	}
	leg.BoundaryTS = boundary

	// The boundary event_seq: the highest seq at or below the boundary for this
	// owner. Reported, never used as the predicate — ts is the horizon's unit
	// and event_seq is only an ordering authority (P-T07-4).
	var maxSeq sql.NullInt64
	if err := s.db.QueryRowContext(ctx,
		`SELECT max(event_seq) FROM run_events WHERE user_id = ? AND ts <= ?`,
		userID, boundary).Scan(&maxSeq); err != nil {
		return leg, fmt.Errorf("retention: boundary seq for %q: %w", userID, err)
	}
	leg.BoundaryEventSeq = maxSeq.Int64

	for {
		moved, seq, err := s.stripUnit(ctx, &leg, asOf)
		if err != nil {
			return leg, err
		}
		if moved == 0 {
			break
		}
		res.AuditEventSeqs = append(res.AuditEventSeqs, seq)
	}

	if err := s.stripTranscripts(ctx, &leg, asOf, res); err != nil {
		return leg, err
	}
	if len(leg.ByFamily) == 0 {
		leg.ByFamily = nil
	}
	return leg, nil
}

// CompactableRowsQuery is the pass's predicate, exported so the query-plan test
// asserts against the VERBATIM production text rather than a paraphrase of it.
const CompactableRowsQuery = `SELECT e.event_seq, e.type, length(e.payload)
	   FROM run_events e
	   LEFT JOIN retention_keep_forever k ON k.type = e.type
	  WHERE e.user_id = ?
	    AND e.ts <= ?
	    AND k.type IS NULL
	    AND e.payload <> ?
	    AND length(e.payload) >= ?
	  ORDER BY e.event_seq
	  LIMIT ?`

// stripUnit elides one batch of BULKY payload bodies AND appends the
// retention.compacted row that counts them, in ONE transaction. It returns how
// many rows it moved and the audit row's seq (0, 0 when nothing is left).
//
// The atomicity is the point: there is no ordering of failures in which bodies
// are committed and their audit record is not.
func (s *Store) stripUnit(ctx context.Context, leg *OwnerPass, asOf time.Time) (int64, int64, error) {
	var moved, auditSeq int64
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		moved, auditSeq = 0, 0
		// The predicate: this owner, at or below the boundary, BULKY, not
		// keep-forever, not already compacted. The keep-forever test is the
		// SEEDED ALLOWLIST — the same table the 0015 trigger and the export view
		// consult, never a second hand-written type list.
		rows, err := tx.QueryContext(ctx, CompactableRowsQuery,
			leg.UserID, leg.BoundaryTS, CompactedPayload, int64(BulkPayloadFloorBytes), compactBatch)
		if err != nil {
			return fmt.Errorf("retention: select compactable rows for %q: %w", leg.UserID, err)
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
		if len(targets) == 0 {
			return nil
		}

		unit := OwnerPass{
			UserID: leg.UserID, HorizonMonths: leg.HorizonMonths,
			BoundaryTS: leg.BoundaryTS, BoundaryEventSeq: leg.BoundaryEventSeq,
			ByFamily: map[string]int64{},
		}
		for _, t := range targets {
			if _, err := tx.ExecContext(ctx,
				`UPDATE run_events SET payload = ? WHERE event_seq = ?`, CompactedPayload, t.seq); err != nil {
				return fmt.Errorf("retention: strip payload at seq %d: %w", t.seq, err)
			}
			fam, known := eventlog.Classify(t.typ)
			name := string(fam)
			if !known {
				// Forward tolerance: an unregistered type is trace by default and
				// is counted under a visible label, never silently.
				name = "unregistered"
			}
			unit.ByFamily[name]++
			unit.EventsStripped++
			// Every strip reclaims by construction: BulkPayloadFloorBytes is
			// asserted greater than the marker's own length.
			unit.BytesReclaimed += t.bytes - int64(len(CompactedPayload))
			moved++
		}
		seq, err := s.appendCompactedTx(ctx, tx, asOf, unit)
		if err != nil {
			return err
		}
		auditSeq = seq

		leg.EventsStripped += unit.EventsStripped
		leg.BytesReclaimed += unit.BytesReclaimed
		for k, v := range unit.ByFamily {
			leg.ByFamily[k] += v
		}
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	return moved, auditSeq, nil
}

// appendCompactedTx writes the S14.9 ¶2 audit row for ONE committed unit,
// inside that unit's own transaction. Platform-scope: the operator sees the
// whole pass and no member's audit view gains another member's counts.
func (s *Store) appendCompactedTx(ctx context.Context, tx *sql.Tx, asOf time.Time, unit OwnerPass) (int64, error) {
	payload, err := json.Marshal(PassResult{
		AsOf:           asOf.Format(time.RFC3339Nano),
		Owners:         []OwnerPass{unit},
		EventsStripped: unit.EventsStripped,
		BytesReclaimed: unit.BytesReclaimed,
	})
	if err != nil {
		return 0, fmt.Errorf("retention: marshal %s payload: %w", EventCompacted, err)
	}
	return s.log.AppendTx(ctx, tx, eventlog.Append{
		UserID:        platformOwner,
		Type:          EventCompacted,
		SchemaVersion: eventSchemaVersion,
		Payload:       payload,
	})
}

// stripTranscripts removes the S02.2 engine_sessions transcript copy-asides
// past the boundary: the FILE on disk and the row's reference to it. The row
// itself stays — it is the reboot-case mining index and its session identity is
// still a fact.
//
// A file that is already gone is not an error (a snapshot restore or an
// operator cleanup may have preceded the pass); any other filesystem error is
// COUNTED and the reference is left in place, so the next pass retries rather
// than orphaning a file nobody can find again. The reference clears and its
// audit row commit in one transaction, exactly as a payload unit does.
func (s *Store) stripTranscripts(ctx context.Context, leg *OwnerPass, asOf time.Time, res *PassResult) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT session_key, transcript_copy_path FROM engine_sessions
		  WHERE user_id = ? AND updated_ts <= ? AND transcript_copy_path <> ''
		  ORDER BY session_key`, leg.UserID, leg.BoundaryTS)
	if err != nil {
		return fmt.Errorf("retention: list transcript copy-asides for %q: %w", leg.UserID, err)
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

	var cleared []int64
	for _, a := range asides {
		if err := s.remove(a.path); err != nil && !os.IsNotExist(err) {
			leg.TranscriptErrors++
			continue
		}
		cleared = append(cleared, a.key)
	}
	if len(cleared) == 0 {
		return nil
	}

	var auditSeq int64
	err = s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		for _, key := range cleared {
			if _, err := tx.ExecContext(ctx,
				`UPDATE engine_sessions SET transcript_copy_path = '' WHERE session_key = ?`, key); err != nil {
				return fmt.Errorf("retention: clear transcript ref %d: %w", key, err)
			}
		}
		unit := OwnerPass{
			UserID: leg.UserID, HorizonMonths: leg.HorizonMonths,
			BoundaryTS: leg.BoundaryTS, BoundaryEventSeq: leg.BoundaryEventSeq,
			TranscriptsStripped: int64(len(cleared)),
		}
		seq, err := s.appendCompactedTx(ctx, tx, asOf, unit)
		auditSeq = seq
		return err
	})
	if err != nil {
		return err
	}
	leg.TranscriptsStripped += int64(len(cleared))
	res.AuditEventSeqs = append(res.AuditEventSeqs, auditSeq)
	return nil
}

// osRemove is the default RemoveFile seam.
func osRemove(path string) error { return os.Remove(path) }

// ── The durable liveness stamp (migration 0015 retention_pass_state) ─────────

// PassState is the compaction pass's durable record of having run. The pass is
// deliberately cursor-less, and a pass that compacted nothing writes no event,
// so without this row a quiet year and a driver goroutine that died at boot
// look identical. Liveness must be a READ, not an inference.
type PassState struct {
	LastRunTS   time.Time
	LastEvents  int64
	LastBytes   int64
	LastError   string
	RunsTotal   int64
	EventsTotal int64
	// Ran reports whether the pass has ever run (a fresh row has not).
	Ran bool
}

// recordPassState stamps the pass, including a no-op or a failed one. It is
// bookkeeping OF the pass and never an input to it.
func (s *Store) recordPassState(ctx context.Context, res PassResult, passErr error) error {
	msg := ""
	if passErr != nil {
		msg = passErr.Error()
	} else if len(res.FailedOwners) > 0 {
		msg = fmt.Sprintf("%d owner(s) failed: %v", len(res.FailedOwners), res.FailedOwners)
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	return s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE retention_pass_state
			    SET last_run_ts = ?, last_events = ?, last_bytes = ?, last_error = ?,
			        runs_total = runs_total + 1, events_total = events_total + ?, updated_ts = ?
			  WHERE row_id = 'compaction'`,
			res.AsOf, res.EventsStripped, res.BytesReclaimed, msg,
			res.EventsStripped, now)
		if err != nil {
			return fmt.Errorf("retention: stamp compaction pass state: %w", err)
		}
		return nil
	})
}

// PassState reads the durable liveness stamp — the READ that distinguishes a
// quiet household from a driver that died at boot.
func (s *Store) PassState(ctx context.Context) (PassState, error) {
	var (
		st      PassState
		lastRun string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT last_run_ts, last_events, last_bytes, last_error, runs_total, events_total
		   FROM retention_pass_state WHERE row_id = 'compaction'`).
		Scan(&lastRun, &st.LastEvents, &st.LastBytes, &st.LastError, &st.RunsTotal, &st.EventsTotal)
	if err != nil {
		return PassState{}, fmt.Errorf("retention: read compaction pass state: %w", err)
	}
	if lastRun != "" {
		if st.LastRunTS, err = time.Parse(time.RFC3339Nano, lastRun); err != nil {
			return PassState{}, fmt.Errorf("retention: parse pass last_run_ts %q: %w", lastRun, err)
		}
		st.Ran = true
	}
	return st, nil
}

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
