package metering_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// pricestore_test.go — the S10.3 price table's durable home (P3-B6-3A, OQ1).
//
// The properties under test are the ones a billing surface rests on: the table
// SHIPS EMPTY and prices UNPRICED until an operator fills it, a stored row is
// immutable, an edit is live without a restart, and the version moves exactly
// when the row set does.

func priceFixture(t *testing.T) (*metering.LivePriceTable, *storage.DB, *eventlog.Log) {
	t.Helper()
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	log := eventlog.New(db, reg)
	if err := reg.Attach(ctx, db, log); err != nil {
		t.Fatalf("attach: %v", err)
	}
	live := metering.NewLivePriceTable(metering.NewPriceStore(db, log))
	if err := live.Reload(ctx); err != nil {
		t.Fatalf("reload: %v", err)
	}
	return live, db, log
}

func sampleRow(model string, from time.Time) metering.PriceRow {
	return metering.PriceRow{
		Model: model, Lane: "anthropic",
		Prices:        metering.UnitPrices{InputUSD: 0.000003, OutputUSD: 0.000015},
		EffectiveFrom: from,
		VerifiedOn:    from.Add(-24 * time.Hour),
		Source:        "provider pricing page",
	}
}

// TestPriceTableShipsEmptyAndPricesUnpriced pins the ratified v0 posture: no
// seed, no rows, and every usage row prices UNPRICED rather than $0 (S10.1;
// P-T08-1; CONVENTIONS §11). The B1-gate posture must survive this packet.
func TestPriceTableShipsEmptyAndPricesUnpriced(t *testing.T) {
	live, _, _ := priceFixture(t)
	ctx := context.Background()

	rows, err := live.Rows(ctx)
	if err != nil {
		t.Fatalf("Rows: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("a freshly migrated platform holds %d price rows, want 0 — nothing seeds a price", len(rows))
	}
	if v := live.Version(); v != "empty-v0" {
		t.Errorf("empty table version = %q, want the landed empty-v0 label", v)
	}
	cost := live.Price("claude-sonnet-4-5", "anthropic", time.Now(),
		metering.Accounting{PromptTokens: 1000, BilledOutputTokens: 500}, metering.CurrencyAPIEquiv)
	if !cost.Unpriced {
		t.Fatalf("an empty table priced a row at %v — it must render UNPRICED, never a silent $0 (P-T08-1)", cost.CostUSD)
	}
	if cost.Reason == "" {
		t.Error("an UNPRICED cost carries no reason — the label has to say why")
	}
}

// TestAddRowIsLiveWithoutARestart is the OQ1 read-through: the composed table
// re-builds in place, so the next priced call uses the operator's edit.
func TestAddRowIsLiveWithoutARestart(t *testing.T) {
	live, _, _ := priceFixture(t)
	ctx := context.Background()
	at := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	before := live.Version()
	stored, err := live.AddRow(ctx, metering.AddRowRequest{
		Row: sampleRow("claude-sonnet-4-5", at), Actor: "op", Reason: "list price",
	})
	if err != nil {
		t.Fatalf("AddRow: %v", err)
	}
	if stored.ID == 0 || stored.CreatedBy != "op" || stored.CreatedTS.IsZero() {
		t.Errorf("stored row is not owner-attributed: %+v", stored)
	}

	// Live: no restart, no reload call by the caller.
	cost := live.Price("claude-sonnet-4-5", "anthropic", at.Add(time.Hour),
		metering.Accounting{PromptTokens: 1000, BilledOutputTokens: 500}, metering.CurrencyAPIEquiv)
	if cost.Unpriced {
		t.Fatalf("the row is stored but the live table still prices UNPRICED (%s) — the read-through did not re-compose", cost.Reason)
	}
	if cost.Source != "provider pricing page" {
		t.Errorf("priced from source %q, want the stored row's", cost.Source)
	}
	if live.Version() == before {
		t.Error("the table version did not move when a row landed — the S02.6 freshness member would never fire (S10.3; G1 Def.5)")
	}
	if cost.TableVersion != live.Version() {
		t.Errorf("priced cost stamped %q but the table is at %q", cost.TableVersion, live.Version())
	}

	// A usage timestamp BEFORE the effective date still prices UNPRICED:
	// effective dating is real, not decorative (S10.3; P-T08-3).
	early := live.Price("claude-sonnet-4-5", "anthropic", at.Add(-time.Hour),
		metering.Accounting{PromptTokens: 1000}, metering.CurrencyAPIEquiv)
	if !early.Unpriced {
		t.Error("a future-dated row priced a usage row from before its effective date")
	}
}

// TestStoredRowIsImmutableByTrigger probes the append-only posture at the
// SCHEMA, with raw SQL — the level a bug in a verb could not reach around.
func TestStoredRowIsImmutableByTrigger(t *testing.T) {
	live, db, _ := priceFixture(t)
	ctx := context.Background()
	if _, err := live.AddRow(ctx, metering.AddRowRequest{
		Row: sampleRow("m1", time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)), Actor: "op",
	}); err != nil {
		t.Fatalf("AddRow: %v", err)
	}

	for _, c := range []struct{ what, sql string }{
		{"update a price", `UPDATE price_rows SET unit_prices = '{"input_usd":9}'`},
		{"update the effective date", `UPDATE price_rows SET effective_from = '2020-01-01T00:00:00Z'`},
		{"update the attribution", `UPDATE price_rows SET created_by = 'somebody-else'`},
		{"delete the row", `DELETE FROM price_rows`},
	} {
		err := db.WriteTx(ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, c.sql)
			return err
		})
		if err == nil {
			t.Errorf("raw SQL was allowed to %s — the append-only trigger did not fire", c.what)
		}
	}

	// The row survived every attempt, unchanged.
	rows, err := live.Rows(ctx)
	if err != nil {
		t.Fatalf("Rows: %v", err)
	}
	if len(rows) != 1 || rows[0].CreatedBy != "op" || rows[0].Prices.InputUSD != 0.000003 {
		t.Errorf("the row changed under a refused write: %+v", rows)
	}
}

// TestAChangeIsANewRowAndThePastStaysPriceable is the edit model: appending a
// later-dated row leaves the earlier one pricing its own dates, so a receipt
// charged last month can still be reproduced.
func TestAChangeIsANewRowAndThePastStaysPriceable(t *testing.T) {
	live, _, _ := priceFixture(t)
	ctx := context.Background()
	july := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	sept := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	old := sampleRow("m1", july)
	if _, err := live.AddRow(ctx, metering.AddRowRequest{Row: old, Actor: "op"}); err != nil {
		t.Fatalf("AddRow old: %v", err)
	}
	newer := sampleRow("m1", sept)
	newer.Prices.InputUSD = 0.000006
	newer.Source = "announced flip"
	if _, err := live.AddRow(ctx, metering.AddRowRequest{Row: newer, Actor: "op"}); err != nil {
		t.Fatalf("AddRow new: %v", err)
	}

	acct := metering.Accounting{PromptTokens: 1_000_000}
	past := live.Price("m1", "anthropic", july.Add(24*time.Hour), acct, metering.CurrencyAPIEquiv)
	future := live.Price("m1", "anthropic", sept.Add(24*time.Hour), acct, metering.CurrencyAPIEquiv)
	if past.Unpriced || future.Unpriced {
		t.Fatalf("a two-row table left a date unpriced: past=%+v future=%+v", past, future)
	}
	if !(future.CostUSD > past.CostUSD) {
		t.Errorf("the later row did not take effect: past %v, future %v", past.CostUSD, future.CostUSD)
	}
	if !past.EffectiveOn.Equal(july) {
		t.Errorf("the past priced against %v, want the July row — the past must stay reproducible", past.EffectiveOn)
	}
	if rows, _ := live.Rows(ctx); len(rows) != 2 {
		t.Errorf("the table holds %d rows, want 2 — a change is an append, never a rewrite", len(rows))
	}
}

// TestDigestIsDeterministicAndContentAddressed: the version is a function of
// the row SET. Two identical tables agree; a changed rate disagrees.
func TestDigestIsDeterministicAndContentAddressed(t *testing.T) {
	at := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	a := metering.StoredPriceRow{ID: 1, PriceRow: sampleRow("m1", at), CreatedBy: "op"}
	b := metering.StoredPriceRow{ID: 2, PriceRow: sampleRow("m2", at), CreatedBy: "op"}

	if d := metering.PriceRowsDigest(nil); d != "empty-v0" {
		t.Errorf("empty digest = %q, want empty-v0", d)
	}
	one := metering.PriceRowsDigest([]metering.StoredPriceRow{a, b})
	if one != metering.PriceRowsDigest([]metering.StoredPriceRow{a, b}) {
		t.Error("the digest is not deterministic")
	}
	if one != metering.PriceRowsDigest([]metering.StoredPriceRow{b, a}) {
		t.Error("the digest depends on row ORDER — it must be a function of the set")
	}
	if one == "empty-v0" {
		t.Error("a non-empty table digested to the empty label")
	}
	// A moved rate moves the digest — the whole point of the freshness member.
	moved := a
	moved.Prices.InputUSD = 0.000009
	if metering.PriceRowsDigest([]metering.StoredPriceRow{moved, b}) == one {
		t.Error("a changed rate left the digest unchanged — a price move would never fire freshness (S10.3; G1 Def.5)")
	}
	// The row's own id and who added it are NOT part of the price identity: two
	// platforms with the same prices price the same.
	reid := a
	reid.ID, reid.CreatedBy = 99, "someone-else"
	if metering.PriceRowsDigest([]metering.StoredPriceRow{reid, b}) != one {
		t.Error("the digest changed when only the row id / author changed — the version tracks PRICES")
	}
}

// TestAddRowLandsItsAuditEventInTheSameTransaction: the row and the record of
// the row commit together (the S01.10 audit pattern), and the event is the
// registered type with its own contract-minimum fields.
func TestAddRowLandsItsAuditEventInTheSameTransaction(t *testing.T) {
	live, db, _ := priceFixture(t)
	ctx := context.Background()
	at := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	if _, err := live.AddRow(ctx, metering.AddRowRequest{
		Row: sampleRow("m1", at), Actor: "op", Reason: "announced flip",
	}); err != nil {
		t.Fatalf("AddRow: %v", err)
	}

	var (
		typ, userID string
		payload     string
	)
	err := db.QueryRowContext(ctx,
		`SELECT type, user_id, payload FROM run_events ORDER BY event_seq DESC LIMIT 1`).
		Scan(&typ, &userID, &payload)
	if err != nil {
		t.Fatalf("read event: %v", err)
	}
	if typ != metering.EventPriceRowAdded {
		t.Fatalf("last event is %q, want %q", typ, metering.EventPriceRowAdded)
	}
	if typ == "settings.changed" {
		t.Fatal("a price append minted settings.changed — its contract minimum names a declared key and an old value, and this act has neither")
	}
	if userID != "op" {
		t.Errorf("event owner = %q, want the acting operator", userID)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &got); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}
	for _, k := range []string{"row_id", "model", "lane", "unit_prices", "effective_from", "verified_on", "source", "actor", "reason"} {
		if _, ok := got[k]; !ok {
			t.Errorf("payload is missing contract-minimum key %q", k)
		}
	}
	if _, ok := got["old"]; ok {
		t.Error(`payload carries an "old" — an effective-dated append replaces nothing, and a fabricated old side would be a lie in the audit trail`)
	}
	if _, ok := got["key"]; ok {
		t.Error(`payload carries a "key" — a price row has no declared registry key, and a pseudo-key is the semantic stretch this type exists to avoid`)
	}

	// The registered contract knows the type (the §29 declare-once rule).
	if fam, known := eventlog.Classify(metering.EventPriceRowAdded); !known || fam != eventlog.FamilyPlatform {
		t.Errorf("Classify(%q) = %q,%v; want the platform family", metering.EventPriceRowAdded, fam, known)
	}
}

// TestAddRowRefusesAnInadmissibleRow: the store's floor. A row without a source
// or a date would put an untraceable number into the billing path.
func TestAddRowRefusesAnInadmissibleRow(t *testing.T) {
	live, db, _ := priceFixture(t)
	ctx := context.Background()
	at := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	good := sampleRow("m1", at)
	for _, c := range []struct {
		what string
		req  metering.AddRowRequest
	}{
		{"no actor", metering.AddRowRequest{Row: good}},
		{"no model", metering.AddRowRequest{Row: metering.PriceRow{Lane: "l", Source: "s", EffectiveFrom: at, VerifiedOn: at}, Actor: "op"}},
		{"no lane", metering.AddRowRequest{Row: metering.PriceRow{Model: "m", Source: "s", EffectiveFrom: at, VerifiedOn: at}, Actor: "op"}},
		{"no source", metering.AddRowRequest{Row: metering.PriceRow{Model: "m", Lane: "l", EffectiveFrom: at, VerifiedOn: at}, Actor: "op"}},
		{"no effective date", metering.AddRowRequest{Row: metering.PriceRow{Model: "m", Lane: "l", Source: "s", VerifiedOn: at}, Actor: "op"}},
		{"no verified date", metering.AddRowRequest{Row: metering.PriceRow{Model: "m", Lane: "l", Source: "s", EffectiveFrom: at}, Actor: "op"}},
		// The B6-6 drain: a row whose DECLARED unit prices are not real positive
		// numbers. This is the store defending the UNPRICED posture from the
		// inside — with no row, usage prices UNPRICED and says so; with a row
		// priced at nothing, the lane is "priced" and every call is charged $0
		// with no trace that anything was missing (S10.1; P-T08-1). A blank
		// field in an operator's form is exactly how such a row gets written.
		{"no unit price at all", metering.AddRowRequest{
			Row: metering.PriceRow{Model: "m", Lane: "l", Source: "s", EffectiveFrom: at, VerifiedOn: at}, Actor: "op"}},
		{"a negative unit price", metering.AddRowRequest{
			Row: metering.PriceRow{Model: "m", Lane: "l", Source: "s", EffectiveFrom: at, VerifiedOn: at,
				Prices: metering.UnitPrices{InputUSD: 1e-6, OutputUSD: -5e-6}}, Actor: "op"}},
		{"a NaN unit price", metering.AddRowRequest{
			Row: metering.PriceRow{Model: "m", Lane: "l", Source: "s", EffectiveFrom: at, VerifiedOn: at,
				Prices: metering.UnitPrices{InputUSD: 1e-6, OutputUSD: math.NaN()}}, Actor: "op"}},
		{"an infinite unit price", metering.AddRowRequest{
			Row: metering.PriceRow{Model: "m", Lane: "l", Source: "s", EffectiveFrom: at, VerifiedOn: at,
				Prices: metering.UnitPrices{InputUSD: math.Inf(1)}}, Actor: "op"}},
	} {
		if _, err := live.AddRow(ctx, c.req); !errors.Is(err, metering.ErrBadPriceRow) {
			t.Errorf("AddRow with %s: %v, want ErrBadPriceRow", c.what, err)
		}
	}
	// Nothing landed, and nothing was audited.
	var rows, events int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM price_rows`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM run_events WHERE type = ?`, metering.EventPriceRowAdded).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if rows != 0 || events != 0 {
		t.Errorf("a refused row left %d rows and %d events behind", rows, events)
	}
	// The non-tautological control: a good row still works.
	if _, err := live.AddRow(ctx, metering.AddRowRequest{Row: good, Actor: "op"}); err != nil {
		t.Fatalf("a well-formed row was refused: %v", err)
	}
}

// TestNoCodePathAutoAppliesAPriceRow re-runs the §34 assertion over the new
// surface. G2 Def.16 and S10.3 make a refresh a PROPOSAL: the watchlist raises
// drift cards and the operator applies them. So the store's append verb must be
// reachable only from the HTTP verb's adapter — never from the watchlist, never
// from a loop, never from anything that could fire on its own.
func TestNoCodePathAutoAppliesAPriceRow(t *testing.T) {
	allowed := map[string]bool{
		filepath.Join("..", "metering", "pricestore.go"): true, // the machinery itself
		filepath.Join("..", "shell", "price_seams.go"):   true, // the operator verb's adapter
	}
	callers := map[string]bool{}
	root := ".."
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		fset := token.NewFileSet()
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return nil
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if sel.Sel.Name == "AddRow" {
				callers[path] = true
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(callers) == 0 {
		t.Fatal("the auto-apply scan found no AddRow call at all — it would pass vacuously")
	}
	for path := range callers {
		if !allowed[path] {
			t.Errorf("%s calls AddRow — a price row is applied by the OPERATOR, and a refresh is a proposal (G2 Def.16; S10.3)", path)
		}
	}
	// The watchlist in particular still proposes and never approves.
	for _, f := range mustGlob(t, filepath.Join("..", "watchlist", "*.go")) {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, rerr := os.ReadFile(f)
		if rerr != nil {
			t.Fatal(rerr)
		}
		for _, banned := range []string{"AddRow(", "NewPriceStore(", "NewLivePriceTable("} {
			if strings.Contains(string(src), banned) {
				t.Errorf("%s names %q — the watchlist proposes price changes, it never applies them (§34)", f, banned)
			}
		}
	}
}

func mustGlob(t *testing.T, pattern string) []string {
	t.Helper()
	out, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob %s: %v", pattern, err)
	}
	if len(out) == 0 {
		t.Fatalf("glob %s matched nothing — the scan would pass vacuously", pattern)
	}
	return out
}
