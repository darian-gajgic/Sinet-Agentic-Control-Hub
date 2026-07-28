package metering

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// pricestore.go — the S10.3 price table's durable home and its live-apply
// read-through (P3-B6-3A, OQ1(a); migration 0019 `price_rows`).
//
// price.go carries the SEMANTICS (effective-dated lookup, the two coverage
// guards) over an in-memory row set. This file is where those rows come from
// and where an operator edit goes: the stored rows are composed into an
// EffectiveDatedTable at boot, and an edit re-composes it in place so a price
// change applies without a restart.
//
// The store lives HERE, in the S10 owner, and not in the API layer, because a
// price table read by the pricing path and a price table read by the settings
// UI must be the same table. Two stores would be two price truths, and the one
// that priced a receipt would be whichever the ledger happened to hold.
//
// WHAT THIS FILE DOES NOT DO: it does not apply anything by itself. The B5-6A
// watchlist lands provider price movements as PROPOSAL drift cards and never
// auto-applies them (G2 Def.16; S10.3 "refresh lands as a proposal, never an
// overwrite"), and nothing here reaches into that path. AddRow is an OPERATOR'S
// act, called from the settings verb with the operator's own identity on it.

// EventPriceRowAdded is the platform-scope event appended for each stored price
// row (Spec S14.2 family 12 via the CONVENTIONS §29 broadened-Platform reading).
//
// It is its own type rather than a settings.changed, and the reason is that
// settings.changed's contract minimum does not describe this act honestly:
// its {actor, key, old, new, reason} mirrors settings_events, whose `key` is a
// DECLARED dotted registry key (there is none here — a price row is identified
// by {model, lane, effective_from}) and whose old→new is a value REPLACEMENT
// (there is none here — an effective-dated append leaves every earlier row in
// force for the dates it covers). Emitting it with a pseudo-key and a fabricated
// `old` would put two lies in the audit trail to avoid registering one type.
const EventPriceRowAdded = "price.row_added"

const priceRowEventSchemaVersion = 1

// emptyTableVersion is the version label of a table with no stored rows — the
// landed B1-2 label, kept verbatim so the empty-ship posture is byte-identical
// to what the platform already had. Every row prices UNPRICED against it, which
// is S10.1's ratified honest behavior, never a silent $0.
const emptyTableVersion = "empty-v0"

// StoredPriceRow is one `price_rows` row: the S10.3 pricing fields plus the
// owner attribution that makes the table its own audit record (S01.10).
type StoredPriceRow struct {
	ID int64 `json:"id"`
	PriceRow
	CreatedBy string    `json:"created_by"`
	CreatedTS time.Time `json:"created_ts"`
	Reason    string    `json:"reason,omitempty"`
}

// MarshalJSON flattens the embedded PriceRow, whose own fields carry no tags.
func (r StoredPriceRow) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID            int64      `json:"id"`
		Model         string     `json:"model"`
		Lane          string     `json:"lane"`
		Prices        UnitPrices `json:"unit_prices"`
		EffectiveFrom time.Time  `json:"effective_from"`
		VerifiedOn    time.Time  `json:"verified_on"`
		Source        string     `json:"source"`
		CreatedBy     string     `json:"created_by"`
		CreatedTS     time.Time  `json:"created_ts"`
		Reason        string     `json:"reason,omitempty"`
	}{r.ID, r.Model, r.Lane, r.Prices, r.EffectiveFrom, r.VerifiedOn, r.Source,
		r.CreatedBy, r.CreatedTS, r.Reason})
}

// MarshalJSON gives UnitPrices its stored shape — the same object the
// `unit_prices` column holds, so what an operator reads back is what is stored.
func (p UnitPrices) MarshalJSON() ([]byte, error) { return json.Marshal(storedPrices(p)) }

// UnmarshalJSON reads the stored shape.
func (p *UnitPrices) UnmarshalJSON(b []byte) error {
	var s storedPricesJSON
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*p = UnitPrices{
		InputUSD: s.Input, OutputUSD: s.Output, CacheReadUSD: s.CacheRead,
		CacheCreationUSD: s.CacheCreation, ServerToolUSD: s.ServerTool,
	}
	return nil
}

type storedPricesJSON struct {
	Input         float64 `json:"input_usd"`
	Output        float64 `json:"output_usd"`
	CacheRead     float64 `json:"cache_read_usd"`
	CacheCreation float64 `json:"cache_creation_usd"`
	ServerTool    float64 `json:"server_tool_usd"`
}

func storedPrices(p UnitPrices) storedPricesJSON {
	return storedPricesJSON{p.InputUSD, p.OutputUSD, p.CacheReadUSD, p.CacheCreationUSD, p.ServerToolUSD}
}

// AddRowRequest is one operator price-row append.
type AddRowRequest struct {
	Row    PriceRow
	Actor  string
	Reason string
}

// ErrBadPriceRow refuses a row the S10.3 shape does not admit.
var ErrBadPriceRow = errors.New("metering: price row is not admissible")

// PriceStore is the durable S10.3 price table (migration 0019 `price_rows`).
//
// The table is APPEND-ONLY by trigger and by verb: there is no update and no
// delete, because a price change is a new effective-dated row and the row that
// priced a past receipt must stay exactly as it was.
type PriceStore struct {
	db  *storage.DB
	log *eventlog.Log
}

// NewPriceStore returns the store over db. log carries the append's audit event
// in the same transaction as the row.
func NewPriceStore(db *storage.DB, log *eventlog.Log) *PriceStore {
	return &PriceStore{db: db, log: log}
}

// Rows returns every stored row in a deterministic order: by effective date,
// then by {model, lane}, then by insertion. Ordering is by stored fields rather
// than by rowid alone so the digest below is a function of the TABLE's content,
// not of the order rows happened to arrive in.
func (s *PriceStore) Rows(ctx context.Context) ([]StoredPriceRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT price_row_id, model, lane, unit_prices, effective_from, verified_on,
		        source, created_by, created_ts, reason
		   FROM price_rows
		  ORDER BY effective_from, model, lane, price_row_id`)
	if err != nil {
		return nil, fmt.Errorf("metering: read price rows: %w", err)
	}
	defer rows.Close()
	out := []StoredPriceRow{}
	for rows.Next() {
		var (
			r                               StoredPriceRow
			prices, effFrom, verOn, created string
		)
		if err := rows.Scan(&r.ID, &r.Model, &r.Lane, &prices, &effFrom, &verOn,
			&r.Source, &r.CreatedBy, &created, &r.Reason); err != nil {
			return nil, fmt.Errorf("metering: scan price row: %w", err)
		}
		if err := json.Unmarshal([]byte(prices), &r.Prices); err != nil {
			return nil, fmt.Errorf("metering: price row %d unit_prices: %w", r.ID, err)
		}
		for _, p := range []struct {
			raw  string
			dst  *time.Time
			what string
		}{{effFrom, &r.EffectiveFrom, "effective_from"}, {verOn, &r.VerifiedOn, "verified_on"}, {created, &r.CreatedTS, "created_ts"}} {
			t, err := time.Parse(time.RFC3339Nano, p.raw)
			if err != nil {
				return nil, fmt.Errorf("metering: price row %d %s %q: %w", r.ID, p.what, p.raw, err)
			}
			*p.dst = t.UTC()
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("metering: read price rows: %w", err)
	}
	return out, nil
}

// AddRow stores one effective-dated row and appends its audit event, in ONE
// transaction — the S01.10 "same audit pattern" as a registry write: the change
// and the record of the change commit together or not at all.
func (s *PriceStore) AddRow(ctx context.Context, req AddRowRequest) (StoredPriceRow, error) {
	if err := validatePriceRow(req); err != nil {
		return StoredPriceRow{}, err
	}
	prices, err := json.Marshal(storedPrices(req.Row.Prices))
	if err != nil {
		return StoredPriceRow{}, fmt.Errorf("metering: encode unit prices: %w", err)
	}
	stored := StoredPriceRow{
		PriceRow: PriceRow{
			Model:         req.Row.Model,
			Lane:          req.Row.Lane,
			Prices:        req.Row.Prices,
			EffectiveFrom: req.Row.EffectiveFrom.UTC(),
			VerifiedOn:    req.Row.VerifiedOn.UTC(),
			Source:        req.Row.Source,
		},
		CreatedBy: req.Actor,
		CreatedTS: time.Now().UTC(),
		Reason:    req.Reason,
	}
	err = s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO price_rows (model, lane, unit_prices, effective_from, verified_on,
			                         source, created_by, created_ts, reason)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			stored.Model, stored.Lane, string(prices),
			stored.EffectiveFrom.Format(time.RFC3339Nano),
			stored.VerifiedOn.Format(time.RFC3339Nano),
			stored.Source, stored.CreatedBy,
			stored.CreatedTS.Format(time.RFC3339Nano), stored.Reason)
		if err != nil {
			return fmt.Errorf("metering: insert price row: %w", err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("metering: price row id: %w", err)
		}
		stored.ID = id
		payload, err := json.Marshal(struct {
			RowID         int64            `json:"row_id"`
			Model         string           `json:"model"`
			Lane          string           `json:"lane"`
			UnitPrices    storedPricesJSON `json:"unit_prices"`
			EffectiveFrom string           `json:"effective_from"`
			VerifiedOn    string           `json:"verified_on"`
			Source        string           `json:"source"`
			Actor         string           `json:"actor"`
			Reason        string           `json:"reason"`
		}{
			stored.ID, stored.Model, stored.Lane, storedPrices(stored.Prices),
			stored.EffectiveFrom.Format(time.RFC3339Nano),
			stored.VerifiedOn.Format(time.RFC3339Nano),
			stored.Source, req.Actor, req.Reason,
		})
		if err != nil {
			return fmt.Errorf("metering: encode %s payload: %w", EventPriceRowAdded, err)
		}
		if _, err := s.log.AppendTx(ctx, tx, eventlog.Append{
			UserID:        req.Actor,
			Type:          EventPriceRowAdded,
			SchemaVersion: priceRowEventSchemaVersion,
			Payload:       payload,
		}); err != nil {
			return fmt.Errorf("metering: emit %s: %w", EventPriceRowAdded, err)
		}
		return nil
	})
	if err != nil {
		return StoredPriceRow{}, err
	}
	return stored, nil
}

// validatePriceRow enforces the S10.3 row shape. It is the STORE's floor, not
// the boundary check — the HTTP verb owns the caller's input (CONVENTIONS §30)
// — but a store that admitted a row with no source or no date would put a
// number into the billing path that nobody could trace.
func validatePriceRow(req AddRowRequest) error {
	switch {
	case req.Actor == "":
		return fmt.Errorf("%w: a price row is owner-attributed and carries no anonymous edits (S01.10)", ErrBadPriceRow)
	case req.Row.Model == "":
		return fmt.Errorf("%w: a row prices a named model", ErrBadPriceRow)
	case req.Row.Lane == "":
		return fmt.Errorf("%w: a row prices a named lane — the same model on two lanes is two rows (S10.3)", ErrBadPriceRow)
	case req.Row.Source == "":
		return fmt.Errorf("%w: a row names where its price came from; D5 quotes the provider actually serving the lane, never an aggregator (S10.3)", ErrBadPriceRow)
	case req.Row.EffectiveFrom.IsZero():
		return fmt.Errorf("%w: a row is effective-dated — a table without effective dates is guaranteed wrong on a known future price flip (S10.3; P-T08-3)", ErrBadPriceRow)
	case req.Row.VerifiedOn.IsZero():
		return fmt.Errorf("%w: a row records the day it was verified against the provider (S10.3)", ErrBadPriceRow)
	}
	return nil
}

// PriceRowsDigest is the deterministic version of a stored row set: the table
// version stamped on every priced cost (S10.1 `priced_cost.table_version`) and
// the input to the S02.6 freshness fingerprint's PriceTableVersion member — a
// price change re-prices the remaining plan, so it fires the freshness
// re-validation trigger (S10.3; G1 Def.5).
//
// An EMPTY row set digests to the landed `empty-v0` label rather than to the
// hash of nothing: the empty table is the shipped posture, and naming it says
// so where a hex string would only look like a version.
func PriceRowsDigest(rows []StoredPriceRow) string {
	if len(rows) == 0 {
		return emptyTableVersion
	}
	lines := make([]string, 0, len(rows))
	for _, r := range rows {
		p := storedPrices(r.Prices)
		lines = append(lines, fmt.Sprintf("%s\x1f%s\x1f%s\x1f%s\x1f%g\x1f%g\x1f%g\x1f%g\x1f%g",
			r.Model, r.Lane,
			r.EffectiveFrom.UTC().Format(time.RFC3339Nano),
			r.Source,
			p.Input, p.Output, p.CacheRead, p.CacheCreation, p.ServerTool))
	}
	// Sorted so the digest is a function of the row SET, not of read order.
	sort.Strings(lines)
	h := sha256.New()
	for _, l := range lines {
		h.Write([]byte(l))
		h.Write([]byte{'\n'})
	}
	return "prices-" + hex.EncodeToString(h.Sum(nil))[:16]
}

// LivePriceTable is the PriceTable the platform actually prices against: an
// EffectiveDatedTable composed from the stored rows, swapped in place when the
// operator adds one.
//
// This is the OQ1 read-through. Without it a price edit would need a restart to
// take effect, which is precisely the "live-apply" property the settings surface
// is supposed to have — and worse, the stored table and the priced table would
// disagree in the window between.
type LivePriceTable struct {
	store *PriceStore
	mu    sync.RWMutex
	cur   *EffectiveDatedTable
}

// NewLivePriceTable returns a live table over the store, composed EMPTY. Call
// Reload to build it from storage — the shell does that once at boot, and every
// AddRow does it again.
func NewLivePriceTable(store *PriceStore) *LivePriceTable {
	return &LivePriceTable{store: store, cur: NewEffectiveDatedTable(emptyTableVersion)}
}

// Reload re-composes the table from the stored rows. It is the whole of
// "live-apply": build a fresh table, then swap it in under the lock, so a
// concurrent Price call sees either the old table or the new one and never a
// half-built one.
func (t *LivePriceTable) Reload(ctx context.Context) error {
	rows, err := t.store.Rows(ctx)
	if err != nil {
		return err
	}
	next := NewEffectiveDatedTable(PriceRowsDigest(rows))
	for _, r := range rows {
		next.Add(r.PriceRow)
	}
	t.mu.Lock()
	t.cur = next
	t.mu.Unlock()
	return nil
}

// AddRow stores a row and re-composes the live table, so the edit is in force
// for the next priced call. The store's append is the durable act; the reload
// is what makes it live.
func (t *LivePriceTable) AddRow(ctx context.Context, req AddRowRequest) (StoredPriceRow, error) {
	row, err := t.store.AddRow(ctx, req)
	if err != nil {
		return StoredPriceRow{}, err
	}
	if err := t.Reload(ctx); err != nil {
		// The row is committed; only the in-memory view is stale. Say so
		// exactly, because "the edit failed" would be false and would invite a
		// retry that appends a second identical row.
		return row, fmt.Errorf("metering: price row %d is stored but the live table did not re-compose: %w", row.ID, err)
	}
	return row, nil
}

// Rows reads the stored rows (the settings surface's read).
func (t *LivePriceTable) Rows(ctx context.Context) ([]StoredPriceRow, error) {
	return t.store.Rows(ctx)
}

// Price implements PriceTable over the currently composed table.
func (t *LivePriceTable) Price(model, lane string, ts time.Time, a Accounting, cur Currency) PricedCost {
	t.mu.RLock()
	tbl := t.cur
	t.mu.RUnlock()
	return tbl.Price(model, lane, ts, a, cur)
}

// Version implements PriceTable: the digest of the composed row set.
func (t *LivePriceTable) Version() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.cur.Version()
}
