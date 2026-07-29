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
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"net/http"
	"net/http/httptest"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/history"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/review"
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

	// One proposed effect on the shipping run, so an effect approval has
	// something to be ABOUT: `decision.recorded` carries no run_id, and the
	// effects join is what ties `effect:<id>` back to this task (OQ5).
	exec(t, b, `INSERT INTO effects (effect_id, run_id, user_id, class, payload, payload_hash, state, created_ts, updated_ts)
	            VALUES (?,?,?,?,?,?,?,?,?)`, "e-publish", "r-ship", "alice", "C", "{}", "h1", "approved", fxT2, fxT3)

	// The human decisions this task's page must show — and one that it must
	// NOT, because it is about a different task entirely.
	for _, d := range []struct{ run, payload, ts string }{
		{"", `{"actor":"alice","card_id":"priority_hint:t-ship","card_type":"priority_hint","decision":"reorder","subject":"t-ship","reason":"the release is due","decided_at":"2026-07-20T09:05:00Z","presented_at":"2026-07-20T09:05:00Z"}`, fxT4},
		{"", `{"actor":"op","actor_is_operator":true,"card_id":"effect:e-publish","card_type":"effect","decision":"approve","decided_at":"2026-07-20T09:06:00Z","presented_at":"2026-07-20T09:05:00Z"}`, fxT4},
		{"", `{"actor":"alice","card_id":"priority_hint:t-elsewhere","card_type":"priority_hint","decision":"reorder","subject":"t-elsewhere","decided_at":"2026-07-20T09:07:00Z","presented_at":"2026-07-20T09:07:00Z"}`, fxT4},
	} {
		exec(t, b, `INSERT INTO run_events (run_id, generation, user_id, type, schema_version, payload, ts)
		            VALUES (NULL,NULL,?,?,1,?,?)`, "alice", "decision.recorded", d.payload, d.ts)
	}
	// A run-scoped family row: the accept IS a human decision (S13.6).
	exec(t, b, `INSERT INTO run_events (run_id, generation, user_id, type, schema_version, payload, ts)
	            VALUES (?,0,?,?,1,?,?)`, "r-ship", "alice", "deliverable.accepted",
		`{"actor":"alice","card_id":"deliverable:d-notes","card_type":"deliverable","decision":"accept","deliverable_id":"d-notes","revision_n":2,"decided_at":"2026-07-20T09:08:00Z"}`, fxT4)

	// A deliverable waiting for a person, with two immutable numbered revisions:
	// the review-ready half of the what-needs-me filter, and the revision list
	// the task detail links into.
	exec(t, b, `INSERT INTO deliverables (deliverable_id, user_id, task_id, project_id, subject_ref, dtype, current_revision, state, created_ts, updated_ts)
	            VALUES (?,?,?,?,?,?,?,?,?,?)`,
		"d-notes", "alice", "t-ship", "release-notes", "notes/RELEASE.md", "text", 2, "in-review", fxT2, fxT4)
	for _, rev := range []struct {
		n   int
		sha string
		ts  string
	}{{1, "a1", fxT2}, {2, "b2", fxT4}} {
		exec(t, b, `INSERT INTO deliverable_revisions (deliverable_id, n, user_id, run_id, pin_kind, content_sha256, platform_ref, created_ts)
		            VALUES (?,?,?,?,?,?,?,?)`,
			"d-notes", rev.n, "alice", "r-ship", "content", rev.sha, "notes/RELEASE.md", rev.ts)
	}

	// One materialized receipt, so the cost views have a row to read and the
	// task detail has a receipt to render (B6-5 part B consumes it).
	exec(t, b, `INSERT INTO receipts (run_id, user_id, usage_json, materialized_ts) VALUES (?,?,?,?)`,
		"r-ship", "alice", fixtureReceiptJSON(t), fxT4)
	return b
}

// fixtureReceipt is a stored S10.10 receipt body.
//
// It is MARSHALED FROM THE REAL metering.Receipt rather than hand-written, so
// the committed fixture is the shape the platform actually produces — keys,
// nesting and all — and the view built against it cannot be built against a
// shape that does not exist. (The `items` array carries Go field names because
// metering.LineItem has no json tags; that is the served truth, and the client
// consumes it as served.)
//
// internal/api production code must never import internal/metering — the money
// scan enforces that, and it is the whole point of the MeterReader seam. This
// is a TEST producing a faithful stored body, which is the opposite concern.
func fixtureReceiptJSON(t *testing.T) string {
	t.Helper()
	rc := metering.Receipt{
		RunID:  "r-ship",
		UserID: "alice",
		Items: []metering.LineItem{
			{Model: "claude", Lane: "anthropic", Purpose: metering.PurposeCeremony, Calls: 3,
				PromptTokens: 9_800, BilledOutputTokens: 3_000, PricedUSD: 0.11, PricedCalls: 3, Currency: metering.CurrencyAPIEquiv},
			{Model: "claude", Lane: "anthropic", Purpose: metering.PurposeExecution, Calls: 11,
				PromptTokens: 140_000, BilledOutputTokens: 31_520, PricedUSD: 1.31, PricedCalls: 10, UnpricedCalls: 1,
				Currency: metering.CurrencyAPIEquiv},
		},
		Currency: metering.CurrencyAPIEquiv, TotalPricedUSD: 1.42, TotalCalls: 14, TotalUnpricedCalls: 1,
		// The S10.6 seam note, byte-identical to what internal/metering writes
		// on every real receipt today — the view renders it VERBATIM, so the
		// fixture has to carry the real sentence rather than a placeholder.
		Mode: metering.ModeSummary{Note: "no mode change (S10.6 downgrade ladder lands with routing S08/local tier S12)"},
		ParkHistory: []metering.ParkEpisode{{
			ParkedAt:        mustTime(t, "2026-07-20T09:02:00Z"),
			ResumedAt:       mustTime(t, "2026-07-20T09:03:00Z"),
			DurationSeconds: 60, ParkReason: "weekly quota reached", ResumeCause: "provider signal",
		}},
		DirectUse: metering.DirectUseEstimate{
			Label: metering.DirectUseLabel, FormulaRef: metering.DirectUseFormulaRef,
			HeuristicUSD: 1.42, Currency: metering.CurrencyAPIEquiv,
		},
		MaterializedTS: mustTime(t, fxT4),
	}
	raw, err := json.Marshal(rc)
	if err != nil {
		t.Fatalf("marshal receipt: %v", err)
	}
	return string(raw)
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return ts
}

// fixtureIntake is the artifact + receipt seam for the fixture world.
//
// It serves an APPROVED spec/plan pair for one task and a DRAFT pair for
// another, so the §38 ruling-(a) display contract is exercised in both
// directions from SERVED data: the view has to label a draft as a draft, and it
// cannot be tested on that with only one of the two states on the wire.
// Everything else is the honest absence a task before drafting really has.
type fixtureIntake struct{ t *testing.T }

func (fixtureIntake) Submit(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return nil, &api.SurfaceError{Status: http.StatusNotImplemented, Code: "not_implemented", Msg: "fixture"}
}

func (fixtureIntake) Answer(context.Context, string, string, json.RawMessage, bool) (json.RawMessage, error) {
	return nil, &api.SurfaceError{Status: http.StatusNotImplemented, Code: "not_implemented", Msg: "fixture"}
}

func (fixtureIntake) Advance(context.Context, string, string) (json.RawMessage, error) {
	return nil, &api.SurfaceError{Status: http.StatusNotImplemented, Code: "not_implemented", Msg: "fixture"}
}

func (fixtureIntake) Task(context.Context, string) (json.RawMessage, error) {
	return nil, &api.SurfaceError{Status: http.StatusNotFound, Code: "not_found", Msg: "no pipeline view"}
}

func (fixtureIntake) Artifacts(_ context.Context, taskID string) (json.RawMessage, error) {
	switch taskID {
	case "t-ship":
		return json.RawMessage(fixturePairApproved), nil
	case "t-triage":
		return json.RawMessage(fixturePairDraft), nil
	}
	return nil, errors.New("this task has no drafted spec/plan pair yet")
}

func (f fixtureIntake) Receipt(_ context.Context, runID string) (json.RawMessage, error) {
	if runID == "r-ship" {
		return json.RawMessage(fixtureReceiptJSON(f.t)), nil
	}
	return nil, &api.SurfaceError{Status: http.StatusNotFound, Code: "not_found", Msg: "no receipt for this run"}
}

func fixturePair(status string, n int) string {
	return `{"spec":{"task_id":"t-x","owner":"alice","version":` + strconv.Itoa(n) + `,"status":"` + status + `",` +
		`"tier":"standard","provenance":"claude/2026-07","restatement":"Publish the release notes for this cycle.",` +
		`"outcome":["the notes are published where the household reads them"],` +
		`"acs":[{"n":1,"plain":"Every merged change since the last release is listed once."},` +
		`{"n":2,"plain":"Each entry says what changed in plain language.","structured":"WHEN a reader opens the notes THEN each entry reads as a sentence","structured_kind":"gwt"}],` +
		`"constraints":["no external publishing"],` +
		`"assumptions":[{"text":"the changelog is the source of truth","basis":"stated by the requester"}],` +
		`"out_of_scope":["translating the notes"]},` +
		`"plan":{"task_id":"t-x","owner":"alice","version":` + strconv.Itoa(n) + `,"spec_version":` + strconv.Itoa(n) +
		`,"status":"` + status + `","tier":"standard","provenance":"claude/2026-07",` +
		`"steps":[{"id":"S-1","title":"Collect the merged changes"},{"id":"S-2","title":"Write one line per change"}],` +
		`"coverage":{"AC-1":["S-1"],"AC-2":["S-2"]},"risks":["the changelog may be incomplete"]}}`
}

var (
	fixturePairApproved = fixturePair("approved", 2)
	fixturePairDraft    = fixturePair("draft", 1)
)

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
		DB:       b.db, Meter: fixtureMeter{}, History: st, Intake: fixtureIntake{t: t},
		Review: &review.Store{DB: b.db, Log: b.log, Settings: b.reg, Root: t.TempDir()},
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
	{"task-detail", "/api/tasks/t-ship"},
	{"task-detail-draft", "/api/tasks/t-triage"},
	{"task-detail-bare", "/api/tasks/t-archive"},
	{"receipt", "/api/runs/r-ship/receipt"},
	{"deliverables-in-review", "/api/deliverables?state=in-review"},
	{"approvals", "/api/approvals"},
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
