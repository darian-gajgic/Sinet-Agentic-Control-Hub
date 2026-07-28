package api_test

// settings_test.go — the S15.9 settings family (P3-B6-3A, R1–R6 + the F slice).
//
// Every authority battery here runs in the NON-TAUTOLOGICAL direction as well:
// the OPERATOR's own act must succeed, so a handler that refused everything
// could not pass, and a member's own read must return real content, so an
// empty answer cannot pass for a scoped one.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
)

// devIdentity is the dev-posture fallback: it reads, and every admin verb
// refuses it (§9). It is its own fake because the refusal is a property of the
// IDENTITY, not of the role bit — a dev identity reads as operator-equivalent.
type devIdentity struct{}

func (devIdentity) Authenticate(*http.Request) (api.Identity, error) {
	return api.Identity{UserID: "dev", Dev: true}, nil
}

// fakePrices stands in for the composed price table at the TRANSPORT boundary.
// What is under test here is SCOPE, the empty-table posture and the wire shape;
// the store's real behavior — the append-only trigger, the live re-compose, the
// audit event — is asserted in internal/metering, and the two are driven
// end-to-end over the real adapter in internal/shell.
type fakePrices struct {
	mu      sync.Mutex
	rows    []json.RawMessage
	actors  []string
	version string
	addErr  error
}

func newFakePrices() *fakePrices { return &fakePrices{version: "empty-v0"} }

func (f *fakePrices) PriceRows(context.Context) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.rows) == 0 {
		return json.RawMessage(`[]`), nil
	}
	return json.Marshal(f.rows)
}

func (f *fakePrices) AddPriceRow(_ context.Context, actor, reason string, row json.RawMessage) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.addErr != nil {
		return nil, f.addErr
	}
	f.actors = append(f.actors, actor)
	stored, _ := json.Marshal(map[string]any{
		"id": len(f.rows) + 1, "created_by": actor, "reason": reason, "row": json.RawMessage(row),
	})
	f.rows = append(f.rows, stored)
	f.version = "prices-fixture"
	return stored, nil
}

func (f *fakePrices) TableVersion() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.version
}

func (f *fakePrices) added() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.actors...)
}

// settingsEnv composes the settings family over a REAL attached registry: the
// write path's whole contract (clamp, rules, one-transaction audit) is the
// registry's, and a fake would have tested the fake.
type settingsEnv struct {
	b      *backend
	reg    *settings.Registry
	prices *fakePrices
	bench  *fakeBenchmark
}

func newSettingsEnv(t *testing.T) *settingsEnv {
	t.Helper()
	b := newBackend(t)
	ctx := context.Background()
	if err := b.reg.Attach(ctx, b.db, b.log); err != nil {
		t.Fatalf("attach registry: %v", err)
	}
	if err := b.store.CreateUser(ctx, "", auth.User{ID: "op", DisplayName: "Op", Role: auth.RoleOperator}, fixturePIN); err != nil {
		t.Fatalf("bootstrap operator: %v", err)
	}
	for _, u := range []auth.User{
		{ID: "alice", DisplayName: "Alice", Role: auth.RoleMember},
		{ID: "bob", DisplayName: "Bob", Role: auth.RoleMember},
	} {
		if err := b.store.CreateUser(ctx, "op", u, fixturePIN); err != nil {
			t.Fatalf("create %s: %v", u.ID, err)
		}
	}
	return &settingsEnv{b: b, reg: b.reg, prices: newFakePrices(), bench: &fakeBenchmark{}}
}

func (e *settingsEnv) cfg(auther api.Authenticator) api.Config {
	return api.Config{
		Log: e.b.log, Sessions: e.b.store, Auth: auther,
		Settings: approvalSettings(), Registry: e.reg, Prices: e.prices,
		Benchmark: e.bench,
		HealthFn:  func() api.Health { return api.Health{Ready: true} },
		DB:        e.b.db, Meter: fakeMeter{},
	}
}

func (e *settingsEnv) do(t *testing.T, who, method, path, body string) (int, string) {
	t.Helper()
	return e.doWith(t, fixedIdentity{who}, method, path, body)
}

func (e *settingsEnv) doWith(t *testing.T, auther api.Authenticator, method, path, body string) (int, string) {
	t.Helper()
	rr := httptest.NewRecorder()
	api.New(e.cfg(auther)).Handler().ServeHTTP(rr, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rr.Code, rr.Body.String()
}

func (e *settingsEnv) mustDo(t *testing.T, who, method, path, body string) string {
	t.Helper()
	code, out := e.do(t, who, method, path, body)
	if code != http.StatusOK {
		t.Fatalf("%s %s as %s: status %d: %s", method, path, who, code, out)
	}
	return out
}

// settingsRead decodes the one registry read.
type settingsRead struct {
	Schema           json.RawMessage  `json:"schema"`
	UISchema         json.RawMessage  `json:"uischema"`
	Values           []settings.Value `json:"values"`
	Domains          []string         `json:"domains"`
	Registered       json.RawMessage  `json:"registered"`
	RegisteredAbsent string           `json:"registered_absent"`
	Editable         bool             `json:"editable"`
	EditableReason   string           `json:"editable_reason"`
}

func (e *settingsEnv) read(t *testing.T, who string) settingsRead {
	t.Helper()
	var out settingsRead
	if err := json.Unmarshal([]byte(e.mustDo(t, who, "GET", "/api/settings", "")), &out); err != nil {
		t.Fatalf("decode settings read: %v", err)
	}
	return out
}

func (e *settingsEnv) valueOf(t *testing.T, who, key string) settings.Value {
	t.Helper()
	for _, v := range e.read(t, who).Values {
		if v.Key == key {
			return v
		}
	}
	t.Fatalf("no value served for %q", key)
	return settings.Value{}
}

// ── R1/R2/R5: the one registry read ─────────────────────────────────────────

// TestSettingsReadServesBothArtifactsAndTheWholeIndex is R1: schema, UISchema,
// every declared key with its effective value, its EFFECTIVE clamp bounds, its
// badges and its 13.5 help.
func TestSettingsReadServesBothArtifactsAndTheWholeIndex(t *testing.T) {
	e := newSettingsEnv(t)
	got := e.read(t, "op")

	if len(got.Values) != 118 {
		t.Fatalf("values block has %d entries, want 118 (S18.5)", len(got.Values))
	}
	if len(got.Domains) != 33 {
		t.Errorf("domains = %d, want 33 (S18.5)", len(got.Domains))
	}
	// Both generated artifacts arrive, and both are the registry's own.
	var schema, ui map[string]any
	if err := json.Unmarshal(got.Schema, &schema); err != nil {
		t.Fatalf("schema is not JSON: %v", err)
	}
	if err := json.Unmarshal(got.UISchema, &ui); err != nil {
		t.Fatalf("uischema is not JSON: %v", err)
	}
	if ui["type"] != "Categorization" {
		t.Errorf("uischema root = %v, want Categorization", ui["type"])
	}
	// The served artifacts ARE the registry's own emissions. They travel as
	// embedded JSON, so the wire form is compacted where the emitter indents —
	// the comparison is therefore over the compacted bytes, which is identity
	// of content rather than of whitespace.
	for _, c := range []struct {
		what string
		got  json.RawMessage
		emit func() ([]byte, error)
	}{
		{"schema", got.Schema, settings.New().JSONSchema},
		{"uischema", got.UISchema, settings.New().UISchema},
	} {
		want, err := c.emit()
		if err != nil {
			t.Fatalf("emit %s: %v", c.what, err)
		}
		var compact bytes.Buffer
		if err := json.Compact(&compact, want); err != nil {
			t.Fatal(err)
		}
		if string(got.Schema) == "" {
			t.Fatalf("no %s served", c.what)
		}
		if compact.String() != string(c.got) {
			t.Errorf("the served %s is not the registry's own emission", c.what)
		}
	}

	bounded, helped, restart := 0, 0, 0
	for _, v := range got.Values {
		if v.Help != "" {
			helped++
		}
		if v.Floor != nil || v.Ceiling != nil {
			bounded++
		}
		if v.Restart {
			restart++
		}
	}
	if helped != 118 {
		t.Errorf("%d keys carry 13.5 help, want all 118", helped)
	}
	if bounded == 0 {
		t.Error("no clamp bounds served — S15.9 says the UI shows each setting's")
	}
	if restart != 2 {
		t.Errorf("restart-required keys = %d, want 2", restart)
	}
}

// TestRegisteredBlockCarriesItsMarkerAndIsAbsentHonestly is R5 (S18.4 R9).
func TestRegisteredBlockCarriesItsMarkerAndIsAbsentHonestly(t *testing.T) {
	e := newSettingsEnv(t)
	got := e.read(t, "op")
	if len(got.Registered) == 0 {
		t.Fatal("no registered block served with a practice composed")
	}
	const marker = "registered — changing this value requires a re-registration commit"
	if !strings.Contains(string(got.Registered), marker) {
		t.Errorf("the registered block does not carry the R9 marker verbatim: %s", got.Registered)
	}
	if got.RegisteredAbsent != "" {
		t.Errorf("a served block also claims an absence: %q", got.RegisteredAbsent)
	}

	// With no practice composed the block is an honest absence WITH its reason,
	// never an empty list that reads as "nothing is registered".
	e.bench = nil
	cfg := e.cfg(fixedIdentity{"op"})
	cfg.Benchmark = nil
	rr := httptest.NewRecorder()
	api.New(cfg).Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/api/settings", nil))
	var absent settingsRead
	if err := json.Unmarshal(rr.Body.Bytes(), &absent); err != nil {
		t.Fatal(err)
	}
	if len(absent.Registered) != 0 {
		t.Error("a block was served with no practice composed")
	}
	if absent.RegisteredAbsent == "" {
		t.Error("the absent registered block carries no reason")
	}
}

// TestNoRegisteredNumberIsNamedInTheAPI extends the §39-C source-scan
// discipline: the frozen values travel from the package that owns them, so a
// display can never disagree with the machinery.
func TestNoRegisteredNumberIsNamedInTheAPI(t *testing.T) {
	// Values from BENCH-REG that must not appear as literals in this package.
	banned := []string{"0.95", "fix-and-continue-accruing", "software-development"}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	scanned := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		scanned++
		for _, b := range banned {
			if strings.Contains(string(raw), b) {
				t.Errorf("%s names the registered value %q — R9 values travel from internal/benchmark, never restated here", name, b)
			}
		}
	}
	if scanned == 0 {
		t.Fatal("the registered-value scan read no files — it would pass vacuously")
	}
	// Non-tautology probe.
	if !strings.Contains("\tconst threshold = 0.95\n", banned[0]) {
		t.Fatal("the registered-value scan cannot detect its own probe")
	}
}

// ── R4/OQ8: authority, three ways ───────────────────────────────────────────

// TestSettingsWritesAreOperatorOnly is the OQ8 battery in every direction: the
// operator's own act SUCCEEDS (so a refuse-everything handler cannot pass), a
// member is refused, and the dev identity is refused even though it reads as
// operator-equivalent.
func TestSettingsWritesAreOperatorOnly(t *testing.T) {
	e := newSettingsEnv(t)
	const key = "orchestration.helper_turns"
	const path = "/api/settings/" + key

	// (a) the operator: succeeds, and the value actually moves.
	e.mustDo(t, "op", "POST", path, `{"value":30,"reason":"more room"}`)
	if v := e.valueOf(t, "op", key); v.Effective != float64(30) {
		t.Fatalf("after the operator's write the effective value is %v, want 30", v.Effective)
	}

	// (b) a member: 403 on every write verb, and nothing moves.
	for _, c := range []struct{ path, body string }{
		{path, `{"value":31}`},
		{path + "/bounds", `{"ceiling":50}`},
		{"/api/settings/prices", `{"row":{"model":"m"}}`},
	} {
		code, out := e.do(t, "alice", "POST", c.path, c.body)
		if code != http.StatusForbidden {
			t.Errorf("member POST %s: status %d, want 403: %s", c.path, code, out)
		}
		if !strings.Contains(out, "operator") {
			t.Errorf("the refusal for %s does not say why: %s", c.path, out)
		}
	}
	if v := e.valueOf(t, "op", key); v.Effective != float64(30) {
		t.Errorf("a refused member write changed the value to %v", v.Effective)
	}
	if got := e.prices.added(); len(got) != 0 {
		t.Errorf("a refused member price edit reached the store: %v", got)
	}

	// (c) the dev identity reads but never administers (§9).
	if code, _ := e.doWith(t, devIdentity{}, "GET", "/api/settings", ""); code != http.StatusOK {
		t.Errorf("dev identity GET /api/settings: %d, want 200 (it reads)", code)
	}
	for _, c := range []struct{ path, body string }{
		{path, `{"value":32}`},
		{path + "/bounds", `{"ceiling":50}`},
		{"/api/settings/prices", `{"row":{"model":"m"}}`},
	} {
		code, out := e.doWith(t, devIdentity{}, "POST", c.path, c.body)
		if code != http.StatusForbidden {
			t.Errorf("dev POST %s: status %d, want 403: %s", c.path, code, out)
		}
	}
	if v := e.valueOf(t, "op", key); v.Effective != float64(30) {
		t.Errorf("a refused dev write changed the value to %v", v.Effective)
	}
}

// TestSettingsReadStatesTheCallersOwnAuthority: the read tells each caller
// whether they may write, so a surface renders a read-only view rather than
// controls that will 403.
func TestSettingsReadStatesTheCallersOwnAuthority(t *testing.T) {
	e := newSettingsEnv(t)
	if op := e.read(t, "op"); !op.Editable || op.EditableReason == "" {
		t.Errorf("operator read: editable=%v reason=%q, want editable with a reason", op.Editable, op.EditableReason)
	}
	member := e.read(t, "alice")
	if member.Editable {
		t.Error("a member's read claims the settings are editable")
	}
	if member.EditableReason == "" {
		t.Error("a member's read does not say why they cannot edit")
	}
	// The member still gets the WHOLE index — non-tautological: an
	// all-refusing handler would serve nothing.
	if len(member.Values) != 118 {
		t.Errorf("a member's read has %d values, want the whole index", len(member.Values))
	}
}

// TestPerUserOverridesAreOwnerScopedOnRead: the declarations are the platform's
// own configuration and everyone reads them, but a per-user override is about a
// PERSON — so a member sees only their own and the operator sees all (§30).
func TestPerUserOverridesAreOwnerScopedOnRead(t *testing.T) {
	e := newSettingsEnv(t)
	const key = "retention.compaction_horizon"
	e.mustDo(t, "op", "POST", "/api/settings/"+key, `{"value":9,"for_user":"alice"}`)
	e.mustDo(t, "op", "POST", "/api/settings/"+key, `{"value":12,"for_user":"bob"}`)

	if op := e.valueOf(t, "op", key); len(op.UserValues) != 2 {
		t.Errorf("the operator sees %d per-user overrides, want both", len(op.UserValues))
	}
	alice := e.valueOf(t, "alice", key)
	if len(alice.UserValues) != 1 || alice.UserValues["alice"] == nil {
		t.Errorf("alice sees %v, want exactly her own override", alice.UserValues)
	}
	if _, leaked := alice.UserValues["bob"]; leaked {
		t.Error("alice's read carries bob's per-user override")
	}
	// Non-tautological: her own value is PRESENT, so an empty answer cannot
	// pass for a scoped one.
	if alice.UserValues["alice"] != float64(9) {
		t.Errorf("alice's own override reads %v, want 9", alice.UserValues["alice"])
	}
	// A member with no override of their own sees none rather than another's.
	if bobSees := e.valueOf(t, "bob", key).UserValues; bobSees["alice"] != nil {
		t.Error("bob's read carries alice's override")
	}
}

// ── R3: the write path's own contract, through HTTP ─────────────────────────

// TestSetIsValidatedClampedAndLiveApplied drives the landed registry semantics
// over the transport: the clamp refuses with the registry's own message, an
// undeclared key is 404, and an accepted write is in force on the next read
// with no restart.
func TestSetIsValidatedClampedAndLiveApplied(t *testing.T) {
	e := newSettingsEnv(t)
	const key = "orchestration.helper_turns"

	// Live-apply.
	e.mustDo(t, "op", "POST", "/api/settings/"+key, `{"value":25}`)
	if v := e.valueOf(t, "op", key); v.Effective != float64(25) || !v.Overridden {
		t.Fatalf("after a set the read serves %v (overridden=%v), want 25/true", v.Effective, v.Overridden)
	}

	// The operator narrows the bounds, then the clamp refuses past them with
	// the REGISTRY's message.
	e.mustDo(t, "op", "POST", "/api/settings/"+key+"/bounds", `{"ceiling":40}`)
	if v := e.valueOf(t, "op", key); v.Ceiling == nil || *v.Ceiling != 40 {
		t.Fatalf("effective ceiling = %v, want 40", v.Ceiling)
	}
	code, out := e.do(t, "op", "POST", "/api/settings/"+key, `{"value":41}`)
	if code != http.StatusBadRequest {
		t.Errorf("out-of-clamp set: status %d, want 400: %s", code, out)
	}
	if !strings.Contains(out, "ceiling") {
		t.Errorf("the clamp refusal does not carry the registry's own message: %s", out)
	}

	// Type and enum refusals are the registry's too.
	if code, _ := e.do(t, "op", "POST", "/api/settings/"+key, `{"value":"twenty"}`); code != http.StatusBadRequest {
		t.Errorf("a wrongly typed value: status %d, want 400", code)
	}

	// An undeclared key is 404 on every verb.
	for _, c := range []struct{ method, path, body string }{
		{"POST", "/api/settings/no.such_key", `{"value":1}`},
		{"POST", "/api/settings/no.such_key/bounds", `{"ceiling":1}`},
		{"GET", "/api/settings/no.such_key/history", ""},
	} {
		if code, out := e.do(t, "op", c.method, c.path, c.body); code != http.StatusNotFound {
			t.Errorf("%s %s: status %d, want 404: %s", c.method, c.path, code, out)
		}
	}
}

// TestResetDeletesTheOverride: a null value is reset-to-default, which DELETES
// the row (S01.10), and the next read serves the default again.
func TestResetDeletesTheOverride(t *testing.T) {
	e := newSettingsEnv(t)
	const key = "orchestration.helper_turns"
	e.mustDo(t, "op", "POST", "/api/settings/"+key, `{"value":25}`)
	before := e.valueOf(t, "op", key)

	out := e.mustDo(t, "op", "POST", "/api/settings/"+key, `{"value":null,"reason":"back to default"}`)
	if !strings.Contains(out, `"reset":true`) {
		t.Errorf("the reset response does not report itself as a reset: %s", out)
	}
	after := e.valueOf(t, "op", key)
	if after.Overridden {
		t.Error("the override row survived a reset")
	}
	if after.Effective != after.Default {
		t.Errorf("after reset effective = %v, want the default %v", after.Effective, after.Default)
	}
	if before.Effective == after.Effective {
		t.Error("the fixture never actually moved the value — the reset proves nothing")
	}

	// The stored override cell is really gone.
	var n int
	if err := e.b.db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM settings WHERE key = ? AND user_id = '' AND value IS NOT NULL`, key).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("%d override cells remain after a reset", n)
	}
}

// TestAnInternalWriteFailureIs500NotBadRequest is drain D1: a mid-write
// storage/event-log failure is a PLATFORM fault, and answering it 400
// "bad_request" would tell the operator their input was wrong when it was not.
//
// The path is reached by removing the audit table out from under the write: the
// override cell upserts, the settings_events INSERT fails, and the registry
// wraps that as "settings: audit row: ..." — the exact prefix the pre-drain
// transport matched on to produce a 400. Nothing in production drops a table;
// this is the smallest lever that reaches the real failure branch.
func TestAnInternalWriteFailureIs500NotBadRequest(t *testing.T) {
	e := newSettingsEnv(t)
	ctx := context.Background()
	const key = "orchestration.helper_turns"

	// The control FIRST, so the fixture is proven to work before it is broken:
	// an all-500 handler could not pass this test.
	e.mustDo(t, "op", "POST", "/api/settings/"+key, `{"value":25}`)

	if err := e.b.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DROP TABLE settings_events`)
		return err
	}); err != nil {
		t.Fatalf("drop the audit table: %v", err)
	}

	code, out := e.do(t, "op", "POST", "/api/settings/"+key, `{"value":26}`)
	if code != http.StatusInternalServerError {
		t.Fatalf("a mid-write audit failure answered %d: %s\nwant 500 — the caller's request was fine and the PLATFORM failed", code, out)
	}
	if !strings.Contains(out, `"internal"`) {
		t.Errorf("the failure is not coded as internal: %s", out)
	}
	if strings.Contains(out, "bad_request") {
		t.Errorf("a platform failure answered as the caller's mistake: %s", out)
	}
	// The body stays generic: the caller cannot act on it, and the real cause
	// belongs in the ops log rather than on the wire.
	if strings.Contains(out, "settings_events") || strings.Contains(out, "audit row") {
		t.Errorf("the 500 body leaks the internal cause: %s", out)
	}
	// And the caller-fault classes are still 400 on the same broken store —
	// they are refused BEFORE the write is attempted, so the store's state
	// cannot change how a bad request is classified.
	if code, out := e.do(t, "op", "POST", "/api/settings/"+key, `{"value":"twenty"}`); code != http.StatusBadRequest {
		t.Errorf("a wrongly typed value answered %d, want 400: %s", code, out)
	}
	if code, _ := e.do(t, "op", "POST", "/api/settings/no.such_key", `{"value":1}`); code != http.StatusNotFound {
		t.Errorf("an undeclared key answered %d, want 404", code)
	}
}

// TestCallerFaultsAreMarkedNotMatchedOnText is D1's other half at the registry
// boundary: the classes a transport must answer 400 for are marked as caller
// faults, and the internal wrap is NOT — asserted on the error TYPE, which is
// what the transport now reads.
func TestCallerFaultsAreMarkedNotMatchedOnText(t *testing.T) {
	e := newSettingsEnv(t)
	ctx := context.Background()

	for _, c := range []struct {
		what string
		req  settings.SetRequest
	}{
		{"a wrongly typed value", settings.SetRequest{
			Key: "orchestration.helper_turns", Value: json.RawMessage(`"twenty"`),
			Actor: settings.Actor{Kind: settings.ActorOperator, ID: "op"}}},
		{"a fractional integer", settings.SetRequest{
			Key: "orchestration.helper_turns", Value: json.RawMessage(`20.5`),
			Actor: settings.Actor{Kind: settings.ActorOperator, ID: "op"}}},
		{"a per-user scope on a platform-scope key", settings.SetRequest{
			Key: "orchestration.helper_turns", ForUser: "alice", Value: json.RawMessage(`20`),
			Actor: settings.Actor{Kind: settings.ActorOperator, ID: "op"}}},
	} {
		err := e.reg.Set(ctx, c.req)
		if err == nil {
			t.Errorf("%s was accepted", c.what)
			continue
		}
		if !errors.Is(err, settings.ErrInvalidRequest) {
			t.Errorf("%s is not marked a caller fault: %v", c.what, err)
		}
	}
	// A bounds edit on a key that carries none, and an inverted pair.
	floor, ceiling := 9.0, 1.0
	for _, c := range []struct {
		what string
		req  settings.SetBoundsRequest
	}{
		{"bounds on a non-numeric key", settings.SetBoundsRequest{
			Key: "orchestration.stagger_identical_prefix", Ceiling: &ceiling,
			Actor: settings.Actor{Kind: settings.ActorOperator, ID: "op"}}},
		{"an inverted floor/ceiling", settings.SetBoundsRequest{
			Key: "orchestration.helper_turns", Floor: &floor, Ceiling: &ceiling,
			Actor: settings.Actor{Kind: settings.ActorOperator, ID: "op"}}},
	} {
		err := e.reg.SetBounds(ctx, c.req)
		if err == nil {
			t.Errorf("%s was accepted", c.what)
			continue
		}
		if !errors.Is(err, settings.ErrInvalidRequest) {
			t.Errorf("%s is not marked a caller fault: %v", c.what, err)
		}
	}
	// The other direction: the internal wrap must NOT be marked, or the
	// transport would answer 400 for a platform failure again.
	if err := e.b.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DROP TABLE settings_events`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	err := e.reg.Set(ctx, settings.SetRequest{
		Key: "orchestration.helper_turns", Value: json.RawMessage(`26`),
		Actor: settings.Actor{Kind: settings.ActorOperator, ID: "op"}})
	if err == nil {
		t.Fatal("a write with no audit table succeeded — an unaudited write must not exist")
	}
	if errors.Is(err, settings.ErrInvalidRequest) {
		t.Errorf("a storage failure is marked as the caller's fault: %v", err)
	}
	if !strings.HasPrefix(err.Error(), "settings: ") {
		t.Fatalf("the internal failure lost the prefix that made the old string match look safe: %v", err)
	}
}

// TestAMemberCannotEditTheirOwnPerUserOverride is drain D2 — OQ8's most
// distinctive consequence, and the one a reader is most likely to assume the
// other way. The registry's actor model admits `operator` and `automation` and
// no member kind, so there is no actor a member's write could be attributed to:
// the operator administers a member's own per-user value via `for_user`.
func TestAMemberCannotEditTheirOwnPerUserOverride(t *testing.T) {
	e := newSettingsEnv(t)
	const key = "retention.compaction_horizon"
	const path = "/api/settings/" + key

	if d, ok := settings.New().Decl(key); !ok || !d.PerUser {
		t.Fatalf("fixture assumption broken: %s is not a per-user key", key)
	}

	// Both write forms, aimed by the member at their OWN override.
	for _, c := range []struct{ what, body string }{
		{"set", `{"value":9,"for_user":"alice"}`},
		{"set without naming themselves", `{"value":9}`},
		{"reset", `{"value":null,"for_user":"alice"}`},
	} {
		code, out := e.do(t, "alice", "POST", path, c.body)
		if code != http.StatusForbidden {
			t.Errorf("member %s of their own per-user override: status %d, want 403: %s", c.what, code, out)
		}
		if !strings.Contains(out, "operator") {
			t.Errorf("the refusal does not name the authority: %s", out)
		}
	}
	// Nothing landed for her.
	if v := e.valueOf(t, "op", key); len(v.UserValues) != 0 {
		t.Errorf("a refused member write stored an override: %v", v.UserValues)
	}
	// The non-tautological control: the OPERATOR administering the same value
	// for the same member succeeds, and she can then READ her own.
	e.mustDo(t, "op", "POST", path, `{"value":9,"for_user":"alice"}`)
	if got := e.valueOf(t, "alice", key).UserValues["alice"]; got != float64(9) {
		t.Errorf("alice reads her administered override as %v, want 9", got)
	}
}

// TestPerUserScopeIsRefusedOnANonPerUserKey: `for_user` targets a per-user
// key, and the registry refuses it anywhere else.
func TestPerUserScopeIsRefusedOnANonPerUserKey(t *testing.T) {
	e := newSettingsEnv(t)
	code, out := e.do(t, "op", "POST", "/api/settings/orchestration.helper_turns", `{"value":25,"for_user":"alice"}`)
	if code != http.StatusBadRequest {
		t.Errorf("for_user on a platform-scope key: status %d, want 400: %s", code, out)
	}
	// The control: the same body on a per-user key works.
	e.mustDo(t, "op", "POST", "/api/settings/retention.compaction_horizon", `{"value":9,"for_user":"alice"}`)
}

// ── R3: the audit history route ─────────────────────────────────────────────

func TestHistoryRouteServesTheAuditTrailNewestFirst(t *testing.T) {
	e := newSettingsEnv(t)
	const key = "orchestration.helper_turns"
	for _, v := range []string{`21`, `22`, `23`} {
		e.mustDo(t, "op", "POST", "/api/settings/"+key, `{"value":`+v+`,"reason":"step"}`)
	}
	var got struct {
		Key     string                `json:"key"`
		Entries []settings.AuditEntry `json:"entries"`
		Limit   int                   `json:"limit"`
	}
	if err := json.Unmarshal([]byte(e.mustDo(t, "op", "GET", "/api/settings/"+key+"/history", "")), &got); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(got.Entries) != 3 {
		t.Fatalf("history has %d entries, want 3", len(got.Entries))
	}
	if string(got.Entries[0].New) != "23" {
		t.Errorf("newest entry = %s, want 23", got.Entries[0].New)
	}
	if got.Entries[0].Actor == "" || got.Entries[0].TS == "" {
		t.Errorf("audit row is missing its actor/ts: %+v", got.Entries[0])
	}
	// Values are CONTENT and go out unwrapped — the old/new sides are the
	// registry's canonical JSON verbatim (§38 ruling (b)).
	if string(got.Entries[0].Old) != "22" {
		t.Errorf("old side = %s, want 22", got.Entries[0].Old)
	}
	// Bounded by the caller's limit.
	var short struct {
		Entries []settings.AuditEntry `json:"entries"`
	}
	if err := json.Unmarshal([]byte(e.mustDo(t, "op", "GET", "/api/settings/"+key+"/history?limit=2", "")), &short); err != nil {
		t.Fatal(err)
	}
	if len(short.Entries) != 2 {
		t.Errorf("bounded history returned %d, want 2", len(short.Entries))
	}
}

// ── R6: the price surface ───────────────────────────────────────────────────

type priceRead struct {
	Rows     json.RawMessage `json:"rows"`
	Version  string          `json:"version"`
	Posture  string          `json:"posture"`
	Deferred []string        `json:"deferred"`
	Editable bool            `json:"editable"`
}

// TestPriceSurfaceShipsEmptyWithItsPostureAndItsDeferrals is R6: the empty
// table is a stated posture, not an empty array, and the two S18.3 surfaces
// this packet did not build are named WITH their reason.
func TestPriceSurfaceShipsEmptyWithItsPostureAndItsDeferrals(t *testing.T) {
	e := newSettingsEnv(t)
	var got priceRead
	if err := json.Unmarshal([]byte(e.mustDo(t, "op", "GET", "/api/settings/prices", "")), &got); err != nil {
		t.Fatalf("decode prices: %v", err)
	}
	if strings.TrimSpace(string(got.Rows)) != "[]" {
		t.Errorf("rows = %s, want the empty set (nothing seeds a price)", got.Rows)
	}
	if got.Posture == "" {
		t.Fatal("an empty price table serves no posture — the honest answer to \"why is nothing priced?\" is a sentence")
	}
	if !strings.Contains(got.Posture, "UNPRICED") {
		t.Errorf("the posture does not name the UNPRICED behavior: %q", got.Posture)
	}
	if len(got.Deferred) != 2 {
		t.Fatalf("deferred surfaces = %d, want the 2 S18.3 rows this packet did not build", len(got.Deferred))
	}
	for _, d := range got.Deferred {
		if !strings.Contains(d, "deferred:") {
			t.Errorf("a deferred surface is named without its reason: %q", d)
		}
	}
	if !strings.Contains(strings.Join(got.Deferred, " "), "flat/metered") {
		t.Error("the deferred list does not name the S10.2 flat/metered flag")
	}
	if !got.Editable {
		t.Error("the operator's price read says the table is not editable")
	}
	// A member reads the table (it is platform configuration) but is told they
	// cannot edit it.
	var asMember priceRead
	if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "GET", "/api/settings/prices", "")), &asMember); err != nil {
		t.Fatal(err)
	}
	if asMember.Editable {
		t.Error("a member's price read claims the table is editable")
	}
}

// TestOperatorPriceEditReachesTheStoreAndBumpsTheVersion: the operator's own
// act succeeds (the non-tautological direction) and the served version moves,
// which is what feeds the S02.6 freshness member.
func TestOperatorPriceEditReachesTheStoreAndBumpsTheVersion(t *testing.T) {
	e := newSettingsEnv(t)
	before := e.mustDo(t, "op", "GET", "/api/settings/prices", "")

	out := e.mustDo(t, "op", "POST", "/api/settings/prices",
		`{"row":{"model":"m1","lane":"anthropic","unit_prices":{"input_usd":0.000003},`+
			`"effective_from":"2026-08-01T00:00:00Z","verified_on":"2026-07-28T00:00:00Z","source":"provider page"},`+
			`"reason":"announced flip"}`)
	if !strings.Contains(out, `"version"`) {
		t.Errorf("the append response carries no table version: %s", out)
	}
	if got := e.prices.added(); len(got) != 1 || got[0] != "op" {
		t.Fatalf("the store saw actors %v, want exactly the operator", got)
	}
	after := e.mustDo(t, "op", "GET", "/api/settings/prices", "")
	if before == after {
		t.Error("the price read is unchanged after an append")
	}
	var got priceRead
	if err := json.Unmarshal([]byte(after), &got); err != nil {
		t.Fatal(err)
	}
	if got.Posture != "" {
		t.Errorf("a non-empty table still serves the empty posture: %q", got.Posture)
	}
	if got.Version == "empty-v0" {
		t.Error("the version did not move off the empty label")
	}

	// A body with no row is the caller's mistake, not a platform fault.
	if code, _ := e.do(t, "op", "POST", "/api/settings/prices", `{"reason":"oops"}`); code != http.StatusBadRequest {
		t.Errorf("an append with no row: status %d, want 400", code)
	}
}

// ── the F slice: cross-cutting negatives ────────────────────────────────────

// TestSettingsMutationsMintTheirCanonicalRowAndNothingElse is R26's walk over
// this packet's verbs: each write lands the row its owner mints — the registry's
// one-transaction {cell + settings_events + settings.changed} — and NOTHING
// mints decision.recorded, because an act with a landed canonical row is never
// double-minted (§39 OQ8).
func TestSettingsMutationsMintTheirCanonicalRowAndNothingElse(t *testing.T) {
	e := newSettingsEnv(t)
	ctx := context.Background()
	const key = "orchestration.helper_turns"

	count := func(typ string) int {
		t.Helper()
		var n int
		if err := e.b.db.QueryRowContext(ctx, `SELECT count(*) FROM run_events WHERE type = ?`, typ).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	auditRows := func() int {
		t.Helper()
		var n int
		if err := e.b.db.QueryRowContext(ctx, `SELECT count(*) FROM settings_events`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	if count(settings.EventSettingsChanged) != 0 || auditRows() != 0 {
		t.Fatal("the fixture is not clean")
	}

	// Three mutations: a set, a bounds edit, a reset.
	e.mustDo(t, "op", "POST", "/api/settings/"+key, `{"value":25,"reason":"a"}`)
	e.mustDo(t, "op", "POST", "/api/settings/"+key+"/bounds", `{"ceiling":40,"reason":"b"}`)
	e.mustDo(t, "op", "POST", "/api/settings/"+key, `{"value":null,"reason":"c"}`)

	if got := count(settings.EventSettingsChanged); got != 3 {
		t.Errorf("settings.changed rows = %d, want 3 (one per write)", got)
	}
	if got := auditRows(); got != 3 {
		t.Errorf("settings_events rows = %d, want 3 — the audit row and the event land together (S01.10)", got)
	}
	if got := count("decision.recorded"); got != 0 {
		t.Errorf("%d decision.recorded rows minted — a settings write has its own canonical row and is never double-minted (§39 OQ8)", got)
	}

	// A REFUSED write mints nothing at all.
	e.do(t, "alice", "POST", "/api/settings/"+key, `{"value":26}`)
	e.do(t, "op", "POST", "/api/settings/"+key, `{"value":41}`) // out of clamp
	if got := count(settings.EventSettingsChanged); got != 3 {
		t.Errorf("a refused write was audited: settings.changed = %d, want still 3", got)
	}
	if got := auditRows(); got != 3 {
		t.Errorf("a refused write left an audit row: %d, want still 3", got)
	}
}

// TestSettingsShapesCarryNoPercentOrETA extends the never-percent negative
// (§30) over every shape this packet adds.
func TestSettingsShapesCarryNoPercentOrETA(t *testing.T) {
	e := newSettingsEnv(t)
	e.mustDo(t, "op", "POST", "/api/settings/orchestration.helper_turns", `{"value":25}`)
	e.mustDo(t, "op", "POST", "/api/settings/prices",
		`{"row":{"model":"m1","lane":"l","unit_prices":{"input_usd":1},"effective_from":"2026-08-01T00:00:00Z","verified_on":"2026-07-28T00:00:00Z","source":"s"}}`)

	forbidden := map[string]bool{
		"percent": true, "pct": true, "fraction": true, "ratio": true,
		"progress": true, "eta": true, "complete_fraction": true, "percent_complete": true,
	}
	bodies := []string{
		e.mustDo(t, "op", "GET", "/api/settings", ""),
		e.mustDo(t, "op", "GET", "/api/settings/prices", ""),
		e.mustDo(t, "op", "GET", "/api/settings/orchestration.helper_turns/history", ""),
	}
	checked := 0
	for _, body := range bodies {
		var v any
		if err := json.Unmarshal([]byte(body), &v); err != nil {
			t.Fatalf("decode: %v", err)
		}
		var walk func(any)
		walk = func(n any) {
			switch x := n.(type) {
			case map[string]any:
				for k, sub := range x {
					checked++
					lk := strings.ToLower(k)
					// The registry's own declared keys ride the values block as
					// DATA (a dotted key is not a response field), so only the
					// shape's own member names are judged.
					if strings.Contains(lk, ".") {
						continue
					}
					if forbidden[lk] || strings.Contains(lk, "percent") {
						t.Errorf("a settings shape exposes forbidden progress key %q", k)
					}
					walk(sub)
				}
			case []any:
				for _, sub := range x {
					walk(sub)
				}
			}
		}
		walk(v)
	}
	if checked == 0 {
		t.Fatal("the never-percent walk checked nothing — it would pass vacuously")
	}
	if !forbidden["percent"] {
		t.Fatal("the never-percent walk cannot detect its own probe")
	}
}

// TestSettingsRouteTableHasNoUpdateOrDeleteVerbAndNoVersionPrefix is the route
// negative: a price row is append-only and there is no verb to change one, and
// the API stays unversioned (S15.2 additive-first).
func TestSettingsRouteTableHasNoUpdateOrDeleteVerbAndNoVersionPrefix(t *testing.T) {
	e := newSettingsEnv(t)
	for _, c := range []struct{ method, path string }{
		{"DELETE", "/api/settings/prices"},
		{"PUT", "/api/settings/prices"},
		{"PATCH", "/api/settings/prices"},
		{"DELETE", "/api/settings/orchestration.helper_turns"},
		{"PUT", "/api/settings/orchestration.helper_turns"},
		{"GET", "/v1/api/settings"},
		{"GET", "/api/v1/settings"},
	} {
		code, _ := e.do(t, "op", c.method, c.path, "")
		if code == http.StatusOK {
			t.Errorf("%s %s answered 200 — no such verb exists (a price change is a new effective-dated row; the API is unversioned)", c.method, c.path)
		}
	}
	// The non-tautological control: the verbs that DO exist answer.
	e.mustDo(t, "op", "GET", "/api/settings/prices", "")
	e.mustDo(t, "op", "POST", "/api/settings/orchestration.helper_turns", `{"value":25}`)
}

// TestPriceRouteCannotBeShadowedByAKey pins the mux precedence the routing
// comment rests on: `prices` is a literal segment, every declared key is
// dotted, so no key can ever route to the price handler or be shadowed by it.
func TestPriceRouteCannotBeShadowedByAKey(t *testing.T) {
	for _, d := range settings.New().Decls() {
		if !strings.Contains(d.Key, ".") {
			t.Errorf("declared key %q has no dot — it could collide with a literal route segment", d.Key)
		}
		if d.Key == "prices" {
			t.Errorf("a key named %q would shadow the price route", d.Key)
		}
	}
	e := newSettingsEnv(t)
	// The price path routes to the price handler, not to the key handler.
	var got priceRead
	if err := json.Unmarshal([]byte(e.mustDo(t, "op", "GET", "/api/settings/prices", "")), &got); err != nil {
		t.Fatalf("/api/settings/prices did not serve the price shape: %v", err)
	}
	// And a dotted key still reaches the key handler.
	if code, _ := e.do(t, "op", "GET", "/api/settings/orchestration.helper_turns/history", ""); code != http.StatusOK {
		t.Errorf("the key history route stopped answering: %d", code)
	}
}

// TestSettingsRoutesRequireASession is the fail-closed limb (§9): every route
// in this family sits behind requireIdentity.
func TestSettingsRoutesRequireASession(t *testing.T) {
	e := newSettingsEnv(t)
	srv := api.New(api.Config{
		Log: e.b.log, Sessions: e.b.store,
		Settings: approvalSettings(), Registry: e.reg, Prices: e.prices,
		HealthFn: func() api.Health { return api.Health{Ready: true} },
		DB:       e.b.db, Meter: fakeMeter{},
	})
	for _, c := range []struct{ method, path, body string }{
		{"GET", "/api/settings", ""},
		{"GET", "/api/settings/prices", ""},
		{"POST", "/api/settings/prices", `{"row":{}}`},
		{"GET", "/api/settings/orchestration.helper_turns/history", ""},
		{"POST", "/api/settings/orchestration.helper_turns", `{"value":1}`},
		{"POST", "/api/settings/orchestration.helper_turns/bounds", `{"ceiling":1}`},
	} {
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, httptest.NewRequest(c.method, c.path, strings.NewReader(c.body)))
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with no session: status %d, want 401", c.method, c.path, rr.Code)
		}
	}
}

// TestSettingsRoutesRefuseHonestlyWhenUnwired: a process with no registry or no
// price table says so rather than serving an empty surface that looks real.
func TestSettingsRoutesRefuseHonestlyWhenUnwired(t *testing.T) {
	e := newSettingsEnv(t)
	cfg := e.cfg(fixedIdentity{"op"})
	cfg.Registry, cfg.Prices = nil, nil
	srv := api.New(cfg)
	for _, c := range []struct{ method, path, body string }{
		{"GET", "/api/settings", ""},
		{"GET", "/api/settings/prices", ""},
		{"POST", "/api/settings/prices", `{"row":{}}`},
		{"POST", "/api/settings/orchestration.helper_turns", `{"value":1}`},
	} {
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, httptest.NewRequest(c.method, c.path, strings.NewReader(c.body)))
		if rr.Code != http.StatusServiceUnavailable {
			t.Errorf("%s %s unwired: status %d, want 503", c.method, c.path, rr.Code)
		}
		if !strings.Contains(rr.Body.String(), "not_wired") {
			t.Errorf("%s %s unwired: %s", c.method, c.path, rr.Body.String())
		}
	}
}
