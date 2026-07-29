package api_test

// apifixtures_test.go — the B6-5 OQ2 golden fixtures.
//
// One canonical response body per read the B6-5 web surfaces consume, committed
// under web/src/fixtures/api/ and asserted HERE against what the real handler
// serves.
//
// ONE SHAPE, TWO CONSUMERS. The vitest fetch doubles import these same files as
// their response bodies, so a Go handler whose JSON drifts from what the views
// were built against fails the GO suite. Without this the two sides agree only
// as long as somebody re-diffs a hand-written TypeScript literal against a Go
// struct by eye — which is the §40-C D3 twin-maintained-copy hazard, in two
// languages instead of one.
//
// COMPARE-ONLY BY DEFAULT. A normal run — every run CI can make — writes
// nothing, so §2's "tests never write outside t.TempDir()" holds. Regenerating
// the fixtures is a deliberate act with the diff in front of you:
//
//	SINET_WRITE_API_FIXTURES=1 go test ./internal/api -run TestWebAPIFixtures
//
// and TestFixtureWriterIsNeverAutomated asserts CI never sets that variable.
//
// The seeded world below is FIXED-CLOCK: every timestamp is a literal, and no
// row is written through a helper that stamps time.Now(). That is what makes
// the bytes stable enough to commit — a fixture that churns on every run is a
// fixture nobody reviews.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"net/http"
	"net/http/httptest"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/history"
)

// fixtureDir is where the committed bodies live: inside the web tree, next to
// the tests that import them.
const fixtureDir = "../../web/src/fixtures/api"

// fixtureWriteEnv gates regeneration. Named rather than a flag so it cannot be
// set by a stray `go test` argument in a script.
const fixtureWriteEnv = "SINET_WRITE_API_FIXTURES"

// The fixture world's clock. Every stamp below is derived from it by hand, so
// the seeded rows carry no wall-clock reading at all.
const (
	fxT0 = "2026-07-20T09:00:00Z"
	fxT1 = "2026-07-20T09:01:00Z"
	fxT2 = "2026-07-20T09:02:00Z"
	fxT3 = "2026-07-20T09:03:00Z"
	fxT4 = "2026-07-20T09:04:00Z"
)

// fixtureMeter is the metering seam for the fixture world. It answers a
// per-run figure so the card face carries a real cost, and REFUSES for one run
// so the committed body also carries the honest nil — the absence the views
// have to render without turning it into a zero (§37).
type fixtureMeter struct{}

func (fixtureMeter) RunMeter(_ context.Context, runID string) (api.RunMeter, error) {
	switch runID {
	case "r-ship":
		return api.RunMeter{Tokens: 184_320, APIEquivCostUSD: 1.42}, nil
	case "r-triage":
		return api.RunMeter{Tokens: 9_100, APIEquivCostUSD: 0.07}, nil
	case "r-audit":
		return api.RunMeter{Tokens: 41_500, APIEquivCostUSD: 0.63}, nil
	}
	return api.RunMeter{}, os.ErrNotExist
}

func (fixtureMeter) LaneMeter(context.Context, string, string) (api.LaneMeter, error) {
	return api.LaneMeter{WeightedConsumption: 12.5, CacheReadWeight: 0.1, Assumed: true}, nil
}

// fixtureWorld seeds the deterministic world the committed bodies are taken
// from. Rows go in through raw SQL with literal timestamps deliberately: the
// shared seed helpers stamp time.Now(), which no committed byte may depend on.
func fixtureWorld(t *testing.T) *backend {
	t.Helper()
	b := newBackend(t)

	for _, u := range []struct{ id, role string }{
		{"op", auth.RoleOperator}, {"alice", auth.RoleMember}, {"bob", auth.RoleMember},
	} {
		exec(t, b, `INSERT INTO users (user_id, display_name, role, created_ts) VALUES (?,?,?,?)`,
			u.id, strings.ToUpper(u.id[:1])+u.id[1:], u.role, fxT0)
	}

	// Four tasks: two kanban columns with cards, one card in the OQ7
	// forward-tolerant bucket (`moonshot` is not one of the six landed values —
	// a producer string the board has never seen must not vanish a card), and
	// one task in the honest '(no project)' bucket.
	for _, tk := range []struct{ id, owner, title, kanban, created string }{
		{"t-ship", "alice", "Ship the release notes", "executing", fxT0},
		{"t-triage", "alice", "Triage the inbox backlog", "intake", fxT1},
		{"t-audit", "bob", "Audit the price table", "attention", fxT2},
		{"t-archive", "alice", "Archive last quarter", "moonshot", fxT3},
		{"t-notes", "alice", "Write the weekly note", "done", fxT4},
		{"t-stall", "bob", "Re-index the archive", "verifying", fxT4},
		{"t-claim", "alice", "Rebuild the search index", "executing", fxT4},
	} {
		exec(t, b, `INSERT INTO tasks (task_id, user_id, title, kanban_status, created_ts) VALUES (?,?,?,?,?)`,
			tk.id, tk.owner, tk.title, tk.kanban, tk.created)
	}
	// artifact_claims is the only populated project edge at v0 (§37); the tasks
	// with no claim resolve to the honest bucket rather than dropping out.
	exec(t, b, `INSERT INTO artifact_claims (task_id, project, user_id, path_globs, mode, status, created_ts)
	            VALUES (?,?,?,?,?,?,?)`, "t-ship", "release-notes", "alice", "**", "W", "active", fxT0)

	for _, r := range []struct{ id, owner, task, state, lane, created string }{
		{"r-ship", "alice", "t-ship", "running", "anthropic", fxT0},
		{"r-triage", "alice", "t-triage", "queued", "anthropic", fxT1},
		{"r-audit", "bob", "t-audit", "parked", "zai", fxT2},
		{"r-archive", "alice", "t-archive", "queued", "local", fxT3},
		// A finished run, so the recently-finished bucket has a row; a parked
		// run with NO limit marker, so the "parked, no horizon given" absence
		// is a served fact rather than a hand-written case; and a `claimed`
		// run, which none of the five named buckets covers — the catch-all
		// exists so a state the view did not anticipate still appears.
		{"r-notes", "alice", "t-notes", "completed", "anthropic", fxT4},
		{"r-stall", "bob", "t-stall", "parked", "zai", fxT4},
		{"r-claim", "alice", "t-claim", "claimed", "anthropic", fxT4},
	} {
		exec(t, b, `INSERT INTO runs (run_id, user_id, task_id, state, lane, generation, created_ts, updated_ts)
		            VALUES (?,?,?,?,?,0,?,?)`, r.id, r.owner, r.task, r.state, r.lane, r.created, r.created)
	}

	// The parked run is blocked on a person: an unanswered ask is what makes
	// waiting-on-human true (it is derived, never a stored flag).
	exec(t, b, `INSERT INTO asks (ask_id, run_id, user_id, snapshot, status, observed_ts) VALUES (?,?,?,?,?,?)`,
		"ask-audit", "r-audit", "bob", "{}", "gate", fxT2)

	// Two queued runs carry RECORDED drag order; spaced ranks, 1-based, as the
	// board writes them. r-archive is deliberately left at the default 0 — "no
	// hint" — so the committed body shows both states.
	exec(t, b, `INSERT INTO queue (run_id, user_id, status, priority_lane, enqueued_ts, hint_rank)
	            VALUES (?,?,?,?,?,?)`, "r-triage", "alice", "queued", "", fxT1, 10)
	exec(t, b, `INSERT INTO queue (run_id, user_id, status, priority_lane, enqueued_ts, hint_rank)
	            VALUES (?,?,?,?,?,?)`, "r-archive", "alice", "queued", "", fxT3, 0)

	for _, e := range []struct{ owner, run, typ, payload, ts string }{
		// Effort mode with NO disclosed downgrade: the routing reason is
		// mandatory, a downgrade note is not, and the card must show the
		// absence rather than promoting the reason into one.
		{"alice", "r-ship", "routing.decided",
			`{"cause":"selector-match","score":0.91,"model":"claude","lane":"anthropic","effort":"standard","plain_reason":"the release-notes worker matched on both signals","window_tokens":200000}`, fxT0},
		{"alice", "r-ship", "stage.started", `{"stage":"execute","kind":"execute"}`, fxT1},
		// The other direction: a producer that DID disclose one.
		{"alice", "r-triage", "routing.decided",
			`{"cause":"no-fit-generalist","model":"claude","lane":"anthropic","effort":"quick","plain_reason":"no worker matched, so the generalist took it","downgrade_note":"effort dropped from deep to quick: this lane is close to its declared budget","window_tokens":200000}`, fxT1},
		{"bob", "r-audit", "limit.event",
			`{"provider":"zai","parked_until":"2026-07-20T12:00:00Z","reason":"weekly quota reached"}`, fxT2},
		{"alice", "r-archive", "intake.state", `{"stage":"drafting","kind":"intake"}`, fxT3},
	} {
		exec(t, b, `INSERT INTO run_events (run_id, generation, user_id, type, schema_version, payload, ts)
		            VALUES (?,0,?,?,1,?,?)`, e.run, e.owner, e.typ, e.payload, e.ts)
	}

	// One materialized receipt, so the cost views have a row to read and the
	// task detail has a receipt to render (B6-5 part B consumes it).
	exec(t, b, `INSERT INTO receipts (run_id, user_id, usage_json, materialized_ts) VALUES (?,?,?,?)`,
		"r-ship", "alice", fixtureReceipt, fxT4)
	return b
}

// fixtureReceipt is a stored S10.10 receipt body, served VERBATIM by every read
// that carries it. The label strings are the registered ones — the views render
// what is served and never re-type a label of their own (G2 D2.8).
const fixtureReceipt = `{"run_id":"r-ship","owner":"alice","lane":"anthropic",` +
	`"ceremony":{"tokens":12800,"api_equiv_cost_usd":0.11},` +
	`"execution":{"tokens":171520,"api_equiv_cost_usd":1.31},` +
	`"unpriced":false,"pricing_tier":"published",` +
	`"direct_use":{"label":"direct-use estimate (heuristic)","usd":1.42},` +
	`"mode":{"note":"no mode change (S10.6 downgrade ladder lands with routing S08/local tier S12)"},` +
	`"parks":[{"from":"2026-07-20T09:02:00Z","until":"2026-07-20T09:03:00Z","reason":"weekly quota reached"}]}`

// fixtureServer is the fixture world's control plane: the read surface plus the
// S14.10 query layers, with the deterministic meter.
func fixtureServer(t *testing.T, b *backend, who string) *api.Server {
	t.Helper()
	st, err := history.New(history.Config{DB: b.db, Log: b.log})
	if err != nil {
		t.Fatalf("history.New: %v", err)
	}
	return api.New(api.Config{
		Log: b.log, Sessions: b.store, Auth: fixedIdentity{who},
		Settings: fixedSettings{d: 20 * 1e9},
		HealthFn: func() api.Health { return api.Health{Ready: true} },
		DB:       b.db, Meter: fixtureMeter{}, History: st,
	})
}

// webAPIFixtures is the covered set: one entry per read a B6-5 view calls.
// The identity is the operator, because the surfaces this packet builds are
// read at the household altitude — a member's narrower answer is the same
// SHAPE, and owner scope has its own three-way tests (reads_test.go).
var webAPIFixtures = []struct{ name, path string }{
	{"tasks", "/api/tasks"},
	{"runs", "/api/runs"},
	{"meters", "/api/meters"},
	{"history-views", "/api/events/views"},
	{"history-catalog", "/api/events/catalog"},
	{"history-view-answer", "/api/events/views/cost_per_run"},
	{"history-query-answer", "/api/events/query/status.runs_active"},
}

func TestWebAPIFixtures(t *testing.T) {
	b := fixtureWorld(t)
	write := os.Getenv(fixtureWriteEnv) != ""
	if write {
		if err := os.MkdirAll(fixtureDir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", fixtureDir, err)
		}
	}
	for _, fx := range webAPIFixtures {
		t.Run(fx.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			fixtureServer(t, b, "op").Handler().ServeHTTP(rr, httptest.NewRequest("GET", fx.path, nil))
			if rr.Code != http.StatusOK {
				t.Fatalf("GET %s: status %d: %s", fx.path, rr.Code, rr.Body.String())
			}
			got := canonicalJSON(t, rr.Body.Bytes())
			path := filepath.Join(fixtureDir, fx.name+".json")
			if write {
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatalf("write %s: %v", path, err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v\n\nRegenerate with:\n  %s=1 go test ./internal/api -run TestWebAPIFixtures",
					path, err, fixtureWriteEnv)
			}
			if string(got) != string(want) {
				t.Errorf("%s drifted from what %s serves.\n\nThe web views are built against the committed file, so this is a "+
					"CONTRACT change, not a test failure to silence. Review the diff, then regenerate with:\n  %s=1 go test ./internal/api -run TestWebAPIFixtures\n\n"+
					"served:\n%s\ncommitted:\n%s", path, fx.path, fixtureWriteEnv, got, want)
			}
		})
	}
}

// TestWebAPIFixturesAreStable is the property the whole mechanism rests on: the
// same seeded world serves the same bytes twice. A body carrying a wall-clock
// reading would pass the comparison above on the run that wrote it and fail on
// every run after, so the instability is caught HERE, naming the read.
func TestWebAPIFixturesAreStable(t *testing.T) {
	for _, fx := range webAPIFixtures {
		first := canonicalJSON(t, fixtureBody(t, fixtureWorld(t), fx.path))
		second := canonicalJSON(t, fixtureBody(t, fixtureWorld(t), fx.path))
		if string(first) != string(second) {
			t.Errorf("%s is not byte-stable across two identical seedings — it carries a live reading, "+
				"so it cannot be a committed fixture:\n%s\n%s", fx.path, first, second)
		}
	}
}

// TestFixtureWriterIsNeverAutomated pins the half of "compare-only" that lives
// outside this file: the regeneration gate must not be set anywhere the suite
// runs by itself.
func TestFixtureWriterIsNeverAutomated(t *testing.T) {
	if os.Getenv(fixtureWriteEnv) != "" {
		t.Skipf("SANCTIONED SKIP (CONVENTIONS §10, tier-R): %s is set, which is the deliberate regeneration act", fixtureWriteEnv)
	}
	for _, path := range []string{"../../.github/workflows/ci.yml", "../../Makefile"} {
		src, err := os.ReadFile(path)
		if err != nil {
			continue // an absent file cannot set the variable
		}
		if strings.Contains(string(src), fixtureWriteEnv) {
			t.Errorf("%s names %s — regeneration is an operator act with the diff in front of them, never something CI does",
				path, fixtureWriteEnv)
		}
	}
}

func fixtureBody(t *testing.T, b *backend, path string) []byte {
	t.Helper()
	rr := httptest.NewRecorder()
	fixtureServer(t, b, "op").Handler().ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: status %d: %s", path, rr.Code, rr.Body.String())
	}
	return rr.Body.Bytes()
}

// canonicalJSON re-indents a served body. The committed files are indented
// because a fixture nobody can read is a fixture nobody reviews; indenting is
// the only transformation applied, so every KEY, VALUE and ORDER in the file is
// the handler's own.
func canonicalJSON(t *testing.T, raw []byte) []byte {
	t.Helper()
	var buf strings.Builder
	var v json.RawMessage
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("served body is not JSON: %v: %s", err, raw)
	}
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	// Encoding the RAW message re-emits the handler's own bytes with whitespace
	// added — it never re-marshals through a Go type, so no field order or
	// omitempty decision is re-made here.
	if err := enc.Encode(v); err != nil {
		t.Fatalf("indent: %v", err)
	}
	return []byte(buf.String())
}
