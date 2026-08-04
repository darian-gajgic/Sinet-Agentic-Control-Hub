package shell

// price_seams_test.go — the S10.3 price surface end to end over the REAL
// composition (P3-B6-3A): the HTTP verb → priceSeam → metering.LivePriceTable →
// migration 0019, and back out through the read.
//
// This is the level the package-level batteries cannot see. internal/metering
// proved the store is append-only and re-composes live; internal/api proved the
// verb is operator-scoped and serves the empty posture; only here do the two
// meet, and only here can "the row an operator added is the row the platform
// prices against" be asserted. It follows the drain-D2 precedent that caught the
// budget seam contradicting itself.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

type priceEnv struct {
	db        *storage.DB
	log       *eventlog.Log
	live      *metering.LivePriceTable
	seams     *projectSeams
	freshness run.FreshnessSettings
	srv       func(who string) *api.Server
}

func newPriceEnv(t *testing.T) *priceEnv {
	t.Helper()
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	log := eventlog.New(db, reg)
	if err := reg.Attach(ctx, db, log); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	sessions := auth.New(db, log)
	if err := sessions.CreateUser(ctx, "",
		auth.User{ID: "op", DisplayName: "Op", Role: auth.RoleOperator}, "hunter2hunter"); err != nil {
		t.Fatalf("bootstrap operator: %v", err)
	}
	if err := sessions.CreateUser(ctx, "op",
		auth.User{ID: "alice", DisplayName: "Alice", Role: auth.RoleMember}, "hunter2hunter"); err != nil {
		t.Fatalf("create member: %v", err)
	}

	// Composed exactly as shell.Run composes it.
	live := metering.NewLivePriceTable(metering.NewPriceStore(db, log))
	if err := live.Reload(ctx); err != nil {
		t.Fatalf("compose price table: %v", err)
	}
	e := &priceEnv{db: db, log: log, live: live, freshness: reg,
		seams: &projectSeams{db: db, prices: live}}
	e.srv = func(who string) *api.Server {
		return api.New(api.Config{
			Log: log, Sessions: sessions, Auth: fixedShellIdentity{who},
			Settings: reg, Registry: reg,
			Prices:   priceSurfaceSeam(live),
			HealthFn: func() api.Health { return api.Health{Ready: true} },
			DB:       db,
		})
	}
	return e
}

func (e *priceEnv) do(t *testing.T, who, method, path, body string) (int, string) {
	t.Helper()
	rr := httptest.NewRecorder()
	e.srv(who).Handler().ServeHTTP(rr, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rr.Code, rr.Body.String()
}

const validPriceRow = `{"row":{"model":"claude-sonnet-4-5","lane":"anthropic",` +
	`"unit_prices":{"input_usd":0.000003,"output_usd":0.000015,"cache_read_usd":0.0000003,` +
	`"cache_creation_usd":0.00000375,"server_tool_usd":0},` +
	`"effective_from":"2026-08-01T00:00:00Z","verified_on":"2026-07-28T00:00:00Z",` +
	`"source":"provider pricing page"},"reason":"announced list price"}`

// TestPriceEditIsLiveAndAuditedEndToEnd: an operator's HTTP edit reaches the
// stored table, the composed table prices against it immediately, and the
// append lands its own canonical audit row.
func TestPriceEditIsLiveAndAuditedEndToEnd(t *testing.T) {
	e := newPriceEnv(t)
	ctx := context.Background()
	at := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	acct := metering.Accounting{PromptTokens: 1_000_000, BilledOutputTokens: 100_000}

	// Before: the ratified empty posture, priced UNPRICED.
	if cost := e.live.Price("claude-sonnet-4-5", "anthropic", at, acct, metering.CurrencyAPIEquiv); !cost.Unpriced {
		t.Fatal("the shipped table priced something — it must be empty")
	}
	versionBefore := e.live.Version()

	code, out := e.do(t, "op", "POST", "/api/settings/prices", validPriceRow)
	if code != http.StatusOK {
		t.Fatalf("operator price edit: status %d: %s", code, out)
	}

	// The composed table prices against it with no restart and no reload call.
	cost := e.live.Price("claude-sonnet-4-5", "anthropic", at, acct, metering.CurrencyAPIEquiv)
	if cost.Unpriced {
		t.Fatalf("the stored row did not reach the composed table: %s", cost.Reason)
	}
	if cost.Source != "provider pricing page" {
		t.Errorf("priced from %q, want the row's own source", cost.Source)
	}
	if e.live.Version() == versionBefore {
		t.Error("the table version did not move")
	}

	// The read serves the stored row back, with its owner attribution.
	_, read := e.do(t, "op", "GET", "/api/settings/prices", "")
	for _, want := range []string{"claude-sonnet-4-5", "provider pricing page", `"created_by":"op"`, "announced list price"} {
		if !strings.Contains(read, want) {
			t.Errorf("the price read does not carry %q: %s", want, read)
		}
	}
	if strings.Contains(read, `"posture"`) {
		t.Error("a filled table still serves the empty-table posture")
	}

	// One canonical audit row, and NOT a settings.changed.
	var typ, userID string
	if err := e.db.QueryRowContext(ctx,
		`SELECT type, user_id FROM run_events ORDER BY event_seq DESC LIMIT 1`).Scan(&typ, &userID); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if typ != metering.EventPriceRowAdded {
		t.Errorf("the append minted %q, want %q", typ, metering.EventPriceRowAdded)
	}
	if userID != "op" {
		t.Errorf("the audit row is attributed to %q, want the acting operator", userID)
	}
	var changed, decisions int
	if err := e.db.QueryRowContext(ctx,
		`SELECT count(*) FROM run_events WHERE type = ?`, settings.EventSettingsChanged).Scan(&changed); err != nil {
		t.Fatal(err)
	}
	if err := e.db.QueryRowContext(ctx,
		`SELECT count(*) FROM run_events WHERE type = 'decision.recorded'`).Scan(&decisions); err != nil {
		t.Fatal(err)
	}
	if changed != 0 || decisions != 0 {
		t.Errorf("a price append co-minted settings.changed=%d decision.recorded=%d, want neither", changed, decisions)
	}
}

// TestPriceEditIsOperatorOnlyEndToEnd: the member is refused at the transport
// and nothing reaches the store — with the operator's own act succeeding in the
// same fixture, so a refuse-everything path could not pass.
func TestPriceEditIsOperatorOnlyEndToEnd(t *testing.T) {
	e := newPriceEnv(t)
	ctx := context.Background()

	if code, out := e.do(t, "alice", "POST", "/api/settings/prices", validPriceRow); code != http.StatusForbidden {
		t.Fatalf("member price edit: status %d, want 403: %s", code, out)
	}
	rows, err := e.live.Rows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("a refused member edit stored %d rows", len(rows))
	}
	// The fixture's own user-creation rows are expected; what must be absent is
	// any record of a price act, because none happened.
	var events int
	if err := e.db.QueryRowContext(ctx,
		`SELECT count(*) FROM run_events WHERE type = ?`, metering.EventPriceRowAdded).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 0 {
		t.Errorf("a refused edit appended %d price events", events)
	}
	// The control.
	if code, out := e.do(t, "op", "POST", "/api/settings/prices", validPriceRow); code != http.StatusOK {
		t.Fatalf("operator price edit: status %d: %s", code, out)
	}
	if rows, _ = e.live.Rows(ctx); len(rows) != 1 {
		t.Errorf("the operator's own edit stored %d rows, want 1", len(rows))
	}
}

// TestMalformedPriceRowIsTheCallersMistake: a bad body is a 400 naming the
// field, never a 500 — and never a silently zeroed rate, which is exactly the
// figure S10.3's coverage guard exists to keep off a receipt.
func TestMalformedPriceRowIsTheCallersMistake(t *testing.T) {
	e := newPriceEnv(t)
	for _, c := range []struct{ name, body string }{
		{"no dates", `{"row":{"model":"m","lane":"l","source":"s"}}`},
		{"an unparseable date", `{"row":{"model":"m","lane":"l","source":"s","effective_from":"tomorrow","verified_on":"2026-07-28T00:00:00Z"}}`},
		{"a misspelled rate field", `{"row":{"model":"m","lane":"l","source":"s","effective_from":"2026-08-01T00:00:00Z","verified_on":"2026-07-28T00:00:00Z","inpt_usd":3}}`},
		{"no source", `{"row":{"model":"m","lane":"l","effective_from":"2026-08-01T00:00:00Z","verified_on":"2026-07-28T00:00:00Z"}}`},
		{"a row that is not an object", `{"row":"a price"}`},
	} {
		code, out := e.do(t, "op", "POST", "/api/settings/prices", c.body)
		if code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400: %s", c.name, code, out)
		}
	}
	rows, err := e.live.Rows(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("a malformed row landed anyway: %+v", rows)
	}
}

// TestFingerprintCarriesThePriceTableVersion: the digest reaches the S02.6
// freshness fingerprint, so a price change re-validates parked plans (S10.3
// "any change fires the freshness re-validation trigger"; G1 Def.5).
//
// The members nothing can observe still stay empty — the never-faked rule is
// unchanged; what changed is that this member became observable when the table
// got a durable home.
func TestFingerprintCarriesThePriceTableVersion(t *testing.T) {
	e := newPriceEnv(t)
	ctx := context.Background()

	before, err := e.seams.Fingerprint(ctx, "")
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	if before.PriceTableVersion != e.live.Version() {
		t.Fatalf("fingerprint carries %q, want the composed table's %q", before.PriceTableVersion, e.live.Version())
	}
	if before.SpecPlanVersion != "" || len(before.SourceHashes) != 0 || len(before.CitedEntryVersions) != 0 {
		t.Errorf("fingerprint fabricated an unobservable member: %+v", before)
	}

	if code, out := e.do(t, "op", "POST", "/api/settings/prices", validPriceRow); code != http.StatusOK {
		t.Fatalf("price edit: %d %s", code, out)
	}
	after, err := e.seams.Fingerprint(ctx, "")
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	if after.PriceTableVersion == before.PriceTableVersion {
		t.Fatal("the fingerprint's price member did not move with the table — a price change would never fire freshness")
	}

	// And the LANDED comparison reports it as drift on its own: price-table
	// drift alone triggers the S02.6 re-validation pass.
	got, err := run.EvaluateFreshness(e.freshness, run.FreshnessInput{
		Stored: before, Current: after,
	}, time.Now())
	if err != nil {
		t.Fatalf("EvaluateFreshness: %v", err)
	}
	if got.Fresh {
		t.Fatalf("a moved price table still read as fresh: %+v", got)
	}
	found := false
	for _, r := range got.Reasons {
		if strings.Contains(r, "price-table") {
			found = true
		}
	}
	if !found {
		t.Errorf("the staleness reasons do not name the price table: %v", got.Reasons)
	}

	// The control: an UNMOVED table is fresh, so the trigger is not simply
	// always-on.
	same, err := run.EvaluateFreshness(e.freshness, run.FreshnessInput{
		Stored: after, Current: after,
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !same.Fresh {
		t.Errorf("an unchanged fingerprint read as stale: %v", same.Reasons)
	}
}

// TestPriceSeamIsNilWhenNoTableIsComposed: an unwired process refuses honestly
// rather than serving an empty table that looks like a real one.
func TestPriceSeamIsNilWhenNoTableIsComposed(t *testing.T) {
	if s := priceSurfaceSeam(nil); s != nil {
		t.Error("the price seam is non-nil with no table composed")
	}
}
