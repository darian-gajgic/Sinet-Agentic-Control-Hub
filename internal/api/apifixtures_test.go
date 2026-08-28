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
	"crypto/ecdh"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/accept"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/chat"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/history"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/memory"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/portpool"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/preview"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/push"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/retention"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker/automation"
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

// fxZAIPlanBudget is the fixture world's declared plan budget on the zai lane:
// half the published 28 000-credit rolling allowance, which is what the S10.4
// proposal offers. It is named rather than repeated so the body's `pressure`
// and its `budget.period_units` cannot drift apart.
const fxZAIPlanBudget = 14000.0

// fixtureMeter is the metering seam for the fixture world. It answers a per-run
// figure so the card face carries a real cost, and it produces BOTH honest nils
// the views have to render without turning either into a zero (§37):
//
//   - it REFUSES for an unknown run;
//   - and for `r-claim` it answers a ZERO-VALUED reading with NO error, which is
//     the shape the production ledger actually returns for a run that exists and
//     has recorded no usage yet. `Ledger.RunConsumption` folds a run's
//     checkpoint rows, and a run between its routing decision and its first
//     checkpoint folds to zero tokens, zero cost and zero unpriced calls
//     successfully. Without this arm in the fixture, every consumer that treats
//     `err == nil` as "there is a reading" prints USD 0 in production and passes
//     every test — which is what this arm exists to make impossible. `r-claim`
//     is the right run for it: its state is `claimed`, so nothing has run.
type fixtureMeter struct{}

func (fixtureMeter) RunMeter(_ context.Context, runID string) (api.RunMeter, error) {
	switch runID {
	case "r-ship":
		return api.RunMeter{Tokens: 184_320, APIEquivCostUSD: 1.42}, nil
	case "r-triage":
		return api.RunMeter{Tokens: 9_100, APIEquivCostUSD: 0.07}, nil
	case "r-audit":
		return api.RunMeter{Tokens: 41_500, APIEquivCostUSD: 0.63}, nil
	case "r-claim":
		return api.RunMeter{}, nil
	}
	return api.RunMeter{}, os.ErrNotExist
}

func (fixtureMeter) LaneMeter(_ context.Context, _, lane string) (api.LaneMeter, error) {
	m := api.LaneMeter{
		WeightedConsumption: 12.5, CacheReadWeight: 0.1, Assumed: true,
		// The tier of the token figures above: measured per-call usage.
		Tier: 1,
	}
	// A flat-subscription lane also carries a tier-3 reading in the PLAN's own
	// unit. It is populated for exactly one lane here so the committed body
	// carries the shape both languages read — a member no fixture exercises is
	// a contract neither side is held to.
	//
	// Pressure stays nil: no operator plan budget is declared in this world,
	// and the denominator is Sinet's own budget and never the provider's
	// published allowance (S10.4/D4), which rides along as seed provenance.
	// The kimi lane's plan is the reason per-window units exist: its rolling
	// 5-hour window counts REQUESTS and its 7-day window counts CREDITS, and
	// nobody publishes an allowance for the second one. Both shapes reach the
	// committed body here, because `allowance_unverified` is precisely the
	// member a surface is most likely to render wrongly — a 0 that means
	// "unknown", never "none".
	if lane == "kimi" {
		m.Plan = &api.LanePlanMeter{
			Unit: "requests", Tier: 3, Assumed: true,
			AssumedNote:      "derived plan units, and a LOWER BOUND: the quota is shared across every signed-in device and the API key, so this count is one consumer's, never the pool's",
			Consumed:         4,
			Calls:            4,
			Multiplier:       1,
			MultiplierWindow: "standard",
			BudgetDeclared:   false,
			SeedAllowance:    300,
			SeedQuota:        "rolling-5h",
			VerifiedOn:       "2026-08-24",
			Windows: []api.LanePlanWindow{
				{Name: "rolling-5h", Unit: "requests", Allowance: 300, WindowHours: 5},
				{Name: "weekly", Unit: "credits", WindowHours: 168, AllowanceUnverified: true},
			},
		}
		return m, nil
	}
	if lane == "zai" {
		// The DECLARED-AND-EXPIRED shape (P3-LN-6, corrected at drain r2 R7).
		//
		// This world is FIXED-CLOCK: every timestamp is a literal, which is what
		// makes the bytes stable enough to commit. That has a consequence the
		// first cut of this block got wrong — it served a five-hour budget
		// starting 2026-07-20 WITH a live pressure, which is a state the reading
		// stopped producing the moment period_hours became load-bearing (D6),
		// and no literal instant can ever be inside a five-hour window again.
		//
		// So the committed body carries the state those literals actually
		// produce: the row is declared, the declaration is served, `pressure` is
		// null, and `inapplicable_note` says why — the COHERENT TRIPLE. It
		// exercises one member more than the live shape did, and the live shape
		// is pinned Go-side against the real reading (internal/shell's
		// TestLN6DeclaredPlanBudgetMakesPressureApplicable and
		// TestLN6MetersReadAgreesWithTheRouter) where the clock is real.
		m.Plan = &api.LanePlanMeter{
			Unit: "credits", Tier: 3, Assumed: true,
			AssumedNote:      "derived plan units: the plan publishes no per-request counter, so consumption is a request proxy with the documented multiplier applied",
			Consumed:         3.5,
			Calls:            5,
			Multiplier:       0.5,
			MultiplierWindow: "off-peak",
			Pressure:         nil,
			BudgetDeclared:   true,
			// Byte-identical to what internal/metering writes; moved with that
			// sentence at P3-GF13 drain r1 (F5).
			InapplicableNote: "the declared 5-hour period started 2026-07-20T09:00:00Z and has ended, so it is not a budget " +
				"for the current one; declaring again is what starts the next period, and nothing carries over",
			Budget: &api.LanePlanBudget{
				PeriodUnits: fxZAIPlanBudget, Unit: "credits", Window: "rolling-5h",
				PeriodStart: fxT0, PeriodHours: 5,
				Source: "proposal-seeded", SeededFrom: "rolling-5h", Fraction: 0.5,
				DeclaredBy: "bob", DeclaredTS: fxT0,
			},
			SeedAllowance: 28000,
			SeedQuota:     "rolling-5h",
			VerifiedOn:    "2026-08-23",
			// The declared windows, each with its own unit. Populated here for
			// the same reason the plan block itself is: a member no fixture
			// exercises is a contract neither language is held to.
			//
			// Both windows are credits because this fixture lane IS zai, whose
			// windows share a unit — the shape stays honest rather than
			// demonstrating a variety this lane does not have. The lane that
			// carries a differently-denominated window, and the one unverified
			// allowance, is kimi above.
			Windows: []api.LanePlanWindow{
				{Name: "rolling-5h", Unit: "credits", Allowance: 28000, WindowHours: 5},
				{Name: "weekly", Unit: "credits", Allowance: 140000, WindowHours: 168},
			},
		}
	}
	return m, nil
}

// fixtureWorld seeds the deterministic world the committed bodies are taken
// from. Rows go in through raw SQL with literal timestamps deliberately: the
// shared seed helpers stamp time.Now(), which no committed byte may depend on.
// fixtureRoot names one file-backed store's directory under this world's root.
//
// THE HAZARD IT CLOSES (P3-UI-7 C-1 drain D1). Each of these stores used its own
// `t.TempDir()`, which the test binary removes when it exits. That is invisible
// and harmless while the SAME process both mints and serves — every committed
// fixture is produced that way — and it is fatal the moment a world is seeded
// for a DIFFERENT process to serve: the database keeps the rows and the
// content-addressed bytes are gone, so `compare` and `comments` answer 500 on a
// deliverable whose metadata reads perfectly. This file's own B6-8 note already
// warned about exactly this class one process in; the seed took it one process
// further out.
func fixtureRoot(t *testing.T, b *backend, name string) string {
	t.Helper()
	if b.root == "" {
		return filepath.Join(t.TempDir(), name)
	}
	return filepath.Join(b.root, name)
}

func fixtureWorld(t *testing.T) *backend {
	t.Helper()
	return fixtureWorldOn(t, newBackend(t), t.TempDir())
}

// fixtureWorldOn seeds THIS world onto a caller-supplied backend and knowledge
// root.
//
// The split exists so the committed golden fixtures and the dev-only demo seed
// (seedworld_test.go) drive ONE world rather than two copies that can drift —
// the §40-C twin-maintained-copy hazard, which this file's own header names as
// the reason the fixtures exist at all. `fixtureWorld` keeps its signature and
// its behaviour exactly: a fresh backend over `t.TempDir()`.
func fixtureWorldOn(t *testing.T, b *backend, root string) *backend {
	t.Helper()
	// Every file-backed store in this world roots HERE. Under the fixture
	// writer that is a `t.TempDir()` and nothing changes; under the demo seed it
	// is the throwaway state directory, so the minted bytes outlive the seeding
	// process and the binary that opens the database next can serve them.
	b.root = root
	// The S09 store is composed ONCE, here, so every fixtureServer over this
	// backend shares one knowledge root (see backend.mem).
	store, err := memory.NewStore(b.db, b.log, b.reg, filepath.Join(root, "knowledge"))
	if err != nil {
		t.Fatalf("memory.NewStore: %v", err)
	}
	b.mem, b.memGate = store, memory.NewGate(store)
	// The registry is ATTACHED here, exactly as the shell attaches it: a
	// settings write lands its override cell, its audit row and its
	// settings.changed event in one transaction, and an unattached registry
	// refuses the write rather than losing the audit trail (S01.10).
	// The fixed clock, so a row minted through the REAL write verb is
	// reproducible: settings_events is append-only by trigger, so there is no
	// normalizing a stamp after the fact (the review-store / journal precedent).
	b.reg.Now = func() time.Time { return mustTime(t, fxT4) }
	if err := b.reg.Attach(context.Background(), b.db, b.log); err != nil {
		t.Fatalf("attach the settings registry: %v", err)
	}

	// Users are created through the REAL auth store rather than inserted, so
	// they carry a PIN: a High-tier effect approval re-prompts it (S01.9
	// verify-at-act), and driving that verb for real is the point (drain r1).
	for _, u := range []struct{ id, name, role string }{
		{"op", "Op", auth.RoleOperator}, {"alice", "Alice", auth.RoleMember}, {"bob", "Bob", auth.RoleMember},
	} {
		actor := "op"
		if u.id == "op" {
			actor = ""
		}
		if err := b.store.CreateUser(context.Background(),
			actor, auth.User{ID: u.id, DisplayName: u.name, Role: u.role}, fixturePIN); err != nil {
			t.Fatalf("create %s: %v", u.id, err)
		}
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
		// An OPERATOR-owned task. The operator's own priority hint on it is the
		// one act that mints a TASK-SCOPED decision carrying the D10 operator
		// limb, so it is what the task page can render "(as operator)" from.
		{"t-ops", "op", "Rotate the household keys", "intake", fxT4},
		// The chat-born task is NOT here: it is seeded after driveFixtureChat, by
		// seedFixtureChatBornTask, so no turn can answer with a row the
		// conversation had not yet created.
	} {
		exec(t, b, `INSERT INTO tasks (task_id, user_id, title, kanban_status, created_ts) VALUES (?,?,?,?,?)`,
			tk.id, tk.owner, tk.title, tk.kanban, tk.created)
	}
	// artifact_claims is this world's populated project edge (§37; since 0022 a
	// durable intake-time pin is the other — the seed's intake.state carries no
	// registry key, so claims alone attribute here); the tasks with no claim
	// resolve to the honest bucket rather than dropping out.
	// path_globs is JSON — the shape insertClaimTx writes and claimOverlapTx
	// decodes; a raw glob string here 500s plan approval into this project
	// (checkpoint-3 builder find, 2026-08-06).
	exec(t, b, `INSERT INTO artifact_claims (task_id, project, user_id, path_globs, mode, status, created_ts)
	            VALUES (?,?,?,?,?,?,?)`, "t-ship", "release-notes", "alice", `["**"]`, "W", "active", fxT0)

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
		{"r-ops", "op", "t-ops", "queued", "anthropic", fxT4},
		// A run on the KIMI lane, so the meters projection serves a kimi row
		// and the plan block's per-window units reach the committed body with
		// the shape only this lane has: a requests window beside a credits
		// window whose allowance nobody published (drain r1 F7).
		{"r-kimi", "bob", "t-stall", "queued", "kimi", fxT4},
	} {
		exec(t, b, `INSERT INTO runs (run_id, user_id, task_id, state, lane, generation, created_ts, updated_ts)
		            VALUES (?,?,?,?,?,0,?,?)`, r.id, r.owner, r.task, r.state, r.lane, r.created, r.created)
	}

	// The parked run is blocked on a person: an unanswered ask is what makes
	// waiting-on-human true (it is derived, never a stored flag).
	exec(t, b, `INSERT INTO asks (ask_id, run_id, user_id, snapshot, status, observed_ts) VALUES (?,?,?,?,?,?)`,
		"ask-audit", "r-audit", "bob", "{}", "gate", fxT2)
	// The asks ALICE owns, so the inbox has cards she can actually answer: D10
	// says the owner answers, so with only bob's ask every card read as
	// not-yours and the "yours to answer" branch had no fixture (drain r2 R4b).
	//
	// Their snapshots are marshaled from the REAL intake.Card types (the
	// receipt-fixture precedent), so the committed bodies carry the keys the
	// pipeline actually writes — the S06.9 Layer-1/Layer-2 bodies, the 13.5
	// help block, and each family's own answer vocabulary. A hand-written
	// snapshot would only prove the inbox can read keys somebody imagined,
	// which is the drain-r1 root cause in one sentence.
	for _, a := range []struct{ id, run, status, snapshot string }{
		{"ask-ship", "r-ship", "question", fixtureApprovalCard(t)},
		{"ask-delta", "r-ship", "question", fixtureDeltaCard(t)},
		{"ask-coverage", "r-triage", "question", fixtureCoverageCard(t)},
		// Two trivial-band cards, because "one action answers a SELECTED SET"
		// (S15.6) cannot be driven against a single batchable card.
		{"ask-notes", "r-notes", "question", fixtureTrivialApprovalCard(t, "t-notes", "r-notes",
			"Write this week's household note from the calendar and the task board.")},
		{"ask-claim", "r-claim", "question", fixtureTrivialApprovalCard(t, "t-claim", "r-claim",
			"Rebuild the search index over the household's notes folder.")},
		// A THIRD batchable card whose vocabulary DIFFERS from the other two.
		// Without it every batchable card shared one action list and the OQ10
		// constraint — a batch answers only cards that accept the chosen action
		// — was unobservable: removing the filter changed nothing anybody could
		// see. The three now overlap on `replan` and diverge everywhere else.
		{"ask-sweep", "r-archive", "question", fixtureTrivialCoverageCard(t)},
		// The chat-born card is NOT here either — see seedFixtureChatBornTask.
	} {
		exec(t, b, `INSERT INTO asks (ask_id, run_id, user_id, snapshot, status, observed_ts) VALUES (?,?,?,?,?,?)`,
			a.id, a.run, "alice", a.snapshot, a.status, fxT2)
	}

	// Two queued runs carry RECORDED drag order; spaced ranks, 1-based, as the
	// board writes them. r-archive is deliberately left at the default 0 — "no
	// hint" — so the committed body shows both states.
	exec(t, b, `INSERT INTO queue (run_id, user_id, status, priority_lane, enqueued_ts, hint_rank)
	            VALUES (?,?,?,?,?,?)`, "r-triage", "alice", "queued", "", fxT1, 10)
	exec(t, b, `INSERT INTO queue (run_id, user_id, status, priority_lane, enqueued_ts, hint_rank)
	            VALUES (?,?,?,?,?,?)`, "r-archive", "alice", "queued", "", fxT3, 0)
	exec(t, b, `INSERT INTO queue (run_id, user_id, status, priority_lane, enqueued_ts, hint_rank)
	            VALUES (?,?,?,?,?,?)`, "r-ops", "op", "queued", "", fxT4, 10)

	for _, e := range []struct{ owner, run, typ, payload, ts string }{
		// Effort mode with NO disclosed downgrade: the routing reason is
		// mandatory, a downgrade note is not, and the card must show the
		// absence rather than promoting the reason into one.
		// P3-LN-9: this task carries a per-task LANE PIN (S00.9 A13). The
		// `lane_pin` member is what LN-10's picker binds to, and a member no
		// fixture exercises is a contract nobody agreed to (§63 R3) — so the
		// fixture world has to contain one pinned task, not merely permit one.
		// The reason moves with it: the pin is visible on every surface that
		// renders a reason, which is what makes R1 true with no web/src change.
		{"alice", "r-ship", "routing.decided",
			`{"cause":"selector-match","score":0.91,"model":"claude","lane":"anthropic","lane_pin":"anthropic","effort":"standard","plain_reason":"the release-notes worker matched on both signals; lane \"anthropic\" is pinned on this task, so the pin replaced the consumption-pressure comparison","window_tokens":200000}`, fxT0},
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

	// The S13 orchestrations, composed before anything reads a door.
	fixtureAcceptAndPreview(t, b)

	// A deliverable waiting for a person, with two immutable numbered revisions:
	// the review-ready half of the what-needs-me filter, and the revision list
	// the task detail links into.
	// Two deliverables on the shipping task. `d-notes` is ACCEPTED below
	// through the real verb (so the task detail lists a finished one with its
	// numbered revisions); `d-changelog` stays in-review, so the what-needs-me
	// feed carries genuinely review-ready work. Driving the accept for real is
	// what emptied the in-review feed the first time round — the fixture now
	// says both things because the surface renders both.
	for _, d := range []struct {
		id, subject, state string
		revs               int
	}{
		{"d-notes", "notes/RELEASE.md", "in-review", 2},
		{"d-changelog", "notes/CHANGELOG.md", "in-review", 1},
	} {
		exec(t, b, `INSERT INTO deliverables (deliverable_id, user_id, task_id, project_id, subject_ref, dtype, current_revision, state, created_ts, updated_ts)
		            VALUES (?,?,?,?,?,?,?,?,?,?)`,
			d.id, "alice", "t-ship", "release-notes", d.subject, "text", d.revs, d.state, fxT2, fxT4)
		for n := 1; n <= d.revs; n++ {
			exec(t, b, `INSERT INTO deliverable_revisions (deliverable_id, n, user_id, run_id, pin_kind, content_sha256, platform_ref, created_ts)
			            VALUES (?,?,?,?,?,?,?,?)`,
				d.id, n, "alice", "r-ship", "content", fmt.Sprintf("sha-%s-%d", d.id, n), d.subject, fxT2)
		}
	}

	seedFixtureReviewSurface(t, b)

	// One materialized receipt, so the cost views have a row to read and the
	// task detail has a receipt to render (B6-5 part B consumes it).
	exec(t, b, `INSERT INTO receipts (run_id, user_id, usage_json, materialized_ts) VALUES (?,?,?,?)`,
		"r-ship", "alice", fixtureReceiptJSON(t), fxT4)

	// Two effects on the shipping run, PROPOSED through the real journal so
	// each carries the hash the journal itself computes — a hand-written
	// payload_hash reads as payload DRIFT the moment the approval verb checks
	// it, which is the landed S02.7 protection working (drain r1).
	//
	// One is approved through the landed verb below; the other stays proposed,
	// so the what-needs-me feed carries a genuinely PENDING approval-kind card
	// (drain r1 D9).
	// The journal mints a UUID per effect, which no committed file can carry —
	// so the id (a surrogate) is pinned afterwards while the PAYLOAD and its
	// journal-computed HASH, which are what the approval verb actually checks,
	// stay the producer's own. Same class of concession as the injected clock.
	// e-rotate has NO run: an effect attributed to no run is PLATFORM-LEVEL,
	// which is exactly what makes it need the operator's D10 co-approval.
	for _, id := range []string{"e-publish", "e-notify", "e-rotate"} {
		run := "r-ship"
		if id == "e-rotate" {
			run = ""
		}
		e, err := fixtureJournal(t, b).Propose(context.Background(), gates.Proposal{
			RunID: run, UserID: "alice", Class: gates.ClassC,
			Payload: json.RawMessage(`{"kind":"publish","target":"` + id + `"}`),
		})
		if err != nil {
			t.Fatalf("propose %s: %v", id, err)
		}
		exec(t, b, `UPDATE effects SET effect_id = ?, created_ts = ?, updated_ts = ? WHERE effect_id = ?`,
			id, fxT2, fxT2, e.ID)
	}

	seedFixtureOversightCards(t, b)
	driveFixtureDecisions(t, b)
	driveFixtureMemoryConflict(t, b)
	driveFixtureSettings(t, b)
	driveFixtureChat(t, b)
	seedFixtureChatBornTask(t, b)
	// LAST, deliberately: the S08 verbs append their own worker.* rows, and
	// seeding them here rather than earlier keeps every event_seq the other
	// committed bodies carry exactly where it was. The head cursor still moves —
	// producer-driven workers mean real appended events, which is the whole point
	// of driving them — and it moves in lockstep across every body.
	seedFixtureWorkforce(t, b)
	seedFixturePushDevices(t, b)
	driveFixturePause(t, b)
	driveFixtureMemoryRetire(t, b)
	indexFixtureHistory(t, b)
	return b
}

// indexFixtureHistory runs the REAL B5-8A projector over this world's own event
// log, so `GET /api/events/search` is answered from a corpus somebody's
// machinery WROTE (P3-UI-4).
//
// It exists because of the §38 D12 lesson: a search test over an empty index
// passes while proving nothing, and a committed search body whose rows were
// hand-inserted here would be this file's idea of what the indexer writes
// rather than what it writes. Running `retention.Index` instead means the
// bodies, the refs, the kinds and the owner column are all the projector's, and
// the write-time redaction (index.go's `searchBody`) is the real one.
//
// It appends NO event — the pass writes `history_fts`, `run_event_rollup` and
// its own cursor row inside one WriteTx and touches the log only to read it —
// so the head `cursor` every other committed body carries cannot move. That is
// also why its position in the ordering above is free; it runs last so it sees
// every row the drivers before it wrote.
func indexFixtureHistory(t *testing.T, b *backend) {
	t.Helper()
	rs, err := retention.New(retention.Config{
		DB: b.db, Log: b.log, Settings: b.reg,
		Now: func() time.Time { return mustTime(t, fxT4) },
	})
	if err != nil {
		t.Fatalf("retention.New: %v", err)
	}
	if _, err := rs.EnsureKeepForeverSeeded(context.Background()); err != nil {
		t.Fatalf("EnsureKeepForeverSeeded: %v", err)
	}
	res, err := rs.Index(context.Background())
	if err != nil {
		t.Fatalf("index history: %v", err)
	}
	// A pass that indexed nothing would leave the committed search body vacuous
	// in exactly the way D12 is about, and it would do it silently.
	if res.Indexed == 0 {
		t.Fatalf("the history index pass wrote no rows: the search fixture would be answered from an empty corpus")
	}
}

// driveFixtureMemoryRetire retires the two entries driveFixtureMemoryConflict
// wrote through the real gate, so a browse can be committed at all.
//
// The problem it solves is the recorded one: `entry_id` is crypto/rand and
// migration 0004's identity trigger makes it immutable, so a gate-written row
// can never appear in a committed body. The conflict pair works around it with
// SURROGATE copies — and those copies are only reachable in a LIST if the real
// rows they copy are out of the way. Retiring them through the REAL remove verb
// is how: `?status=active` then answers with exactly the two reproducible rows,
// and nothing was hidden by a query this surface would not otherwise make.
//
// It is driven LAST, after the pause flip, for that flip's own stated reason:
// two real `knowledge.remove` rows move the head cursor, and appending them here
// leaves every other committed body's event_seq exactly where it was.
func driveFixtureMemoryRetire(t *testing.T, b *backend) {
	t.Helper()
	ctx := context.Background()
	rows, err := b.db.QueryContext(ctx,
		`SELECT entry_id FROM knowledge_entries
		  WHERE user_id = 'alice' AND entry_id NOT IN (?, ?) ORDER BY entry_id`, fxEntryA, fxEntryB)
	if err != nil {
		t.Fatalf("read the driven entries: %v", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan driven entry: %v", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) != 2 {
		t.Fatalf("found %d gate-written entries to retire, want the conflict pair's 2", len(ids))
	}
	for _, id := range ids {
		rr := httptest.NewRecorder()
		fixtureServer(t, b, "alice").Handler().ServeHTTP(rr,
			httptest.NewRequest("POST", "/api/memory/"+id+"/remove",
				strings.NewReader(`{"reason":"superseded by the committed pair"}`)))
		if rr.Code != http.StatusOK {
			t.Fatalf("retire %s: %d: %s", id, rr.Code, rr.Body.String())
		}
	}
}

// driveFixturePause flips one person's S10.4 automation switch through the REAL
// verb, so `GET /api/meters` carries a genuinely paused position and a
// genuinely open one side by side.
//
// It is the OPERATOR administering a MEMBER's switch (OQ4's own+operator-any
// authority), which is the reading the fleet control's administer path renders
// — and it is driven LAST for the same reason the workforce seeding is: the
// verb appends a real `decision.recorded` row, so every event_seq the other
// committed bodies carry stays exactly where it was and only the head cursor
// moves.
func driveFixturePause(t *testing.T, b *backend) {
	t.Helper()
	rr := httptest.NewRecorder()
	fixtureServer(t, b, "op").Handler().ServeHTTP(rr, httptest.NewRequest("POST", "/api/meters/pause",
		strings.NewReader(`{"person":"bob","paused":true,"reason":"bob asked for his headroom back"}`)))
	if rr.Code != http.StatusOK {
		t.Fatalf("drive the pause switch: %d: %s", rr.Code, rr.Body.String())
	}
}

// The fixture world's VAPID key, derived from a FIXED scalar rather than
// generated.
//
// It is derived here rather than committed as a PEM for the obvious reason —
// no private key belongs in this repository — and it is fixed rather than
// random because `GET /api/push/subscriptions` SERVES the public half, so a
// per-run key would churn the committed body on every regeneration and
// TestWebAPIFixturesAreStable would (correctly) refuse it. Writing it through
// the real PEM file the store reads means the fixture exercises the real
// load path rather than a seam around it.
func fixtureVAPIDKey(t *testing.T, stateDir string) {
	t.Helper()
	scalar := make([]byte, 32)
	for i := range scalar {
		scalar[i] = byte(i + 1)
	}
	key, err := ecdh.P256().NewPrivateKey(scalar)
	if err != nil {
		t.Fatalf("fixture VAPID scalar: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal fixture VAPID key: %v", err)
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "vapid-key.pem"),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write fixture VAPID key: %v", err)
	}
}

// fixturePushStore composes the S15.11 channel over the world, with the fixed
// clock and a fixed id generator so a row minted through the REAL enrol verb is
// reproducible (the internal/chat NewID precedent, §44).
func fixturePushStore(t *testing.T, b *backend) *push.Store {
	t.Helper()
	if b.push != nil {
		return b.push
	}
	dir := fixtureRoot(t, b, "push-state")
	fixtureVAPIDKey(t, dir)
	n := 0
	st, err := push.New(push.Config{
		DB: b.db, Log: b.log, StateDir: dir,
		Now:   func() time.Time { return mustTime(t, fxT4) },
		NewID: func() string { n++; return fmt.Sprintf("push-fixture-%04d", n) },
	})
	if err != nil {
		t.Fatalf("push.New: %v", err)
	}
	b.push = st
	return st
}

// seedFixturePushDevices enrols two devices through the REAL verb, so the
// committed body is what the handler serves over rows a producer wrote.
//
// TWO OWNERS, because the read is scoped three ways and the operator's body has
// to be able to show a household. The endpoints are the shape a real push
// service issues; they are never served back (an endpoint is a capability), so
// what the committed bodies carry is the hash.
func seedFixturePushDevices(t *testing.T, b *backend) {
	t.Helper()
	st := fixturePushStore(t, b)
	ctx := context.Background()
	for _, d := range []struct{ owner, endpoint, label, p256dh, auth string }{
		{"alice", "https://web.push.apple.com/QDzVuUUFuFXY-fixture-alice-phone", "Alice’s phone",
			"BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4",
			"BTBZMqHH6r4Tts7J_aSIgg"},
		{"op", "https://fcm.googleapis.com/fcm/send/fixture-op-laptop", "Op’s laptop",
			"BP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A8",
			"DGv6ra1nlYgDCS1FRnbzlw"},
	} {
		if _, _, err := st.Enrol(ctx, d.owner, push.Enrolment{
			Endpoint: d.endpoint,
			Keys:     push.Keys{P256DH: d.p256dh, Auth: d.auth},
			Origin:   "https://sinet.example.ts.net",
			Label:    d.label,
		}); err != nil {
			t.Fatalf("enrol fixture device for %s: %v", d.owner, err)
		}
	}
}

// ── the S15.8 review surface (B6-8) ─────────────────────────────────────────

// The revision content the line-diff surface is built from. It is written out as
// literal text rather than assembled, because the committed unified diff is what
// react-diff-view parses and a generated body would make the fixture unreadable
// at review time.
//
// Rev 2 makes TWO edits, far enough apart to land in two separate hunks under
// --unified=3, which is what gives the placement ladder something real to do: an
// anchor inside a changed hunk cannot be mapped by line number and falls to the
// text search, while an anchor in the untouched middle maps exactly by the diff's
// own line delta. Both statuses have to exist in the committed body, so the
// distance between the two edits is load-bearing rather than incidental.
const (
	fxSiteRev1 = "import { mount } from './mount'\n" +
		"\n" +
		"export function ReleasePage() {\n" +
		"  const notes = loadNotes()\n" +
		"  if (!notes) throw new Error('no notes to render')\n" +
		"  return render(notes)\n" +
		"}\n" +
		"\n" +
		"export function loadNotes() {\n" +
		"  return fetchChangelog()\n" +
		"}\n" +
		"\n" +
		"export function render(notes) {\n" +
		"  return mount(notes)\n" +
		"}\n" +
		"\n" +
		"export const version = 1\n"
	fxSiteRev2 = "import { mount } from './mount'\n" +
		"import { theme } from './theme'\n" +
		"\n" +
		"export function ReleasePage() {\n" +
		"  const notes = loadNotes()\n" +
		"  if (!notes) throw new Error('no notes to render')\n" +
		"  return render(notes)\n" +
		"}\n" +
		"\n" +
		"export function loadNotes() {\n" +
		"  return fetchChangelog()\n" +
		"}\n" +
		"\n" +
		"export function render(notes) {\n" +
		"  theme.apply()\n" +
		"  return mount(notes)\n" +
		"}\n" +
		"\n" +
		// A MODIFIED line, not just insertions: markEdits pairs a delete with an
		// insert to mark the changed words inside a line, so a diff with no
		// modification anywhere gives its tokenizer nothing to do and the committed
		// body could not exercise the client-side highlighting at all.
		"export const version = 2\n"
	fxSiteReadme = "# Release page\n\nRendered from the changelog.\n"
	fxSiteLegacy = "// superseded by release.tsx\nexport const legacy = true\n"
)

// The S13.5 snapshot-commit pins. They are LITERALS, not real commits: what makes
// a revision repo-backed — and therefore acceptable — is the column being filled,
// and a real `git` sha would carry the wall clock into the committed bodies
// through the commit's own timestamps. The accept orchestration is composed but
// never called by a fixture read, so nothing resolves these against a repo.
const (
	fxSiteSnap1 = "1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d"
	fxSiteSnap2 = "9f8e7d6c5b4a39281706f5e4d3c2b1a09f8e7d6c"
)

// seedFixtureReviewSurface builds the world the S15.8 review surface renders,
// entirely through S13's own verbs (§42's producer-fidelity rule: a fixture is
// only worth what the ROW under it is worth).
//
//	d-site    code    two content revisions with real files → the line-diff
//	                  surface, a REAL unified diff, and the five placement
//	                  statuses across the revision hop
//	d-hero    image   two object revisions with recorded image/png types → the
//	                  image-pair surface the S13.2 trio renders
//	d-bundle  binary  two object revisions → the metadata cards + hash verdict
//	d-brief   pdf     bytes that are not a PDF → the extraction-failure DEGRADE,
//	                  labeled, which is a defined answer and never a refusal
//
// Everything hangs off the shipping task and its run, so no task or run row is
// added: the deliverables belong to work that already exists, and the accept
// card's trailers derive from the routing decision and engine session that run
// already recorded.
func seedFixtureReviewSurface(t *testing.T, b *backend) {
	t.Helper()
	ctx := context.Background()
	rev := fixtureReview(t, b)

	// The OPEN REWORK CARD behind the request-revision door's live limb. The door
	// is doors-as-data: with no rework card open it names NO route and carries the
	// narrative instead, which `d-site` shows — so without a real card in the world
	// the open limb had no ground at all, and the surface's only driven direction
	// would have been the closed one.
	//
	// The snapshot is a real `verify.Card` for the reason every other ask snapshot
	// here is: the door reads the card's own `choices` (§40-B), and a hand-written
	// snapshot would only prove the door can read keys somebody imagined. The
	// category is the one whose landed vocabulary carries `revise_with_guidance`.
	card, err := json.Marshal(verify.Card{
		Kind: verify.SinkDecisionCard, Category: verify.CatACBlocker,
		TaskID: "t-brief", RunID: "r-brief", IssuedTS: fxT3,
		Summary: "AC-2 is unmet after two rounds: the brief still has no plain-language summary.",
		Detail:  []string{"round 2 verdict: red on AC-2", "the requester decides whether another round is worth it"},
		Choices: []string{"accept_best_effort", "revise_with_guidance", "cancel"},
		AskID:   "ask-brief",
	})
	if err != nil {
		t.Fatalf("marshal verify.Card: %v", err)
	}
	exec(t, b, `INSERT INTO tasks (task_id, user_id, title, kanban_status, created_ts) VALUES (?,?,?,?,?)`,
		"t-brief", "alice", "Rewrite the onboarding brief", "attention", fxT3)
	exec(t, b, `INSERT INTO runs (run_id, user_id, task_id, state, lane, generation, created_ts, updated_ts)
	            VALUES (?,?,?,?,?,0,?,?)`, "r-brief", "alice", "t-brief", "parked", "anthropic", fxT3, fxT3)
	exec(t, b, `INSERT INTO asks (ask_id, run_id, user_id, snapshot, status, observed_ts) VALUES (?,?,?,?,?,?)`,
		"ask-brief", "r-brief", "alice", string(card), "question", fxT3)

	// The two provenance facts the accept card renders trailers from. The routing
	// decision is already in the world; the engine session's substrate is added so
	// the card takes the PRIMARY path (substrate recorded) rather than the lane
	// fallback — production records both, and a fixture that only exercised the
	// fallback would teach the surface the wrong shape.
	exec(t, b, `INSERT INTO engine_sessions (run_id, user_id, substrate, engine_session_id, created_ts, updated_ts)
	            VALUES (?,?,?,?,?,?)`, "r-ship", "alice", "claude-cli", "r-ship-sess", fxT0, fxT1)
	// The S13.7 registry row behind `protected_ref`: an accept pushes to a branch,
	// and the card names which one BEFORE the act.
	exec(t, b, `INSERT INTO repo_registry (project_id, user_id, name, store_path, default_branch, state, created_ts, updated_ts)
	            VALUES (?,?,?,?,?,?,?,?)`,
		"release-notes", "alice", "release-notes", "projects/release-notes.git", "main", "active", fxT0, fxT0)

	ensure := func(id, task, dtype, subject string) {
		t.Helper()
		if _, err := rev.EnsureDeliverable(ctx, review.EnsureInput{
			ID: id, Owner: "alice", TaskID: task, ProjectID: "release-notes", Type: dtype, SubjectRef: subject,
		}); err != nil {
			t.Fatalf("EnsureDeliverable %s: %v", id, err)
		}
	}
	mint := func(runID string, in review.MintInput) {
		t.Helper()
		in.RunID = runID
		in.AttemptRef = fmt.Sprintf("%s#round-%d", runID, in.N)
		if _, err := rev.MintRevision(ctx, in); err != nil {
			t.Fatalf("MintRevision %s/%d: %v", in.DeliverableID, in.N, err)
		}
	}
	comment := func(in review.CommentInput) {
		t.Helper()
		in.DeliverableID, in.Author = "d-site", "alice"
		if _, err := rev.AddComment(ctx, in); err != nil {
			t.Fatalf("AddComment %q: %v", in.Body, err)
		}
	}

	// ── d-site: the line-diff surface and the comment loop ──────────────────
	ensure("d-site", "t-ship", "code", "site/release.tsx")
	mint("r-ship", review.MintInput{DeliverableID: "d-site", N: 1, SnapshotSHA: fxSiteSnap1, Files: map[string]string{
		"site/release.tsx": fxSiteRev1, "site/README.md": fxSiteReadme, "site/legacy.tsx": fxSiteLegacy,
	}})

	// ONE comment before the rework, so the drain has something to consume and the
	// committed body carries both halves of the S13.3 lifecycle. It is drained
	// BEFORE revision 2 is minted, which is the real order of events: the rework
	// receives the numbered points and then produces the next revision.
	comment(review.CommentInput{
		RevisionN: 1, Severity: review.SeverityBlocker,
		Body:      "The page mounts before the theme is applied, so the first paint is unstyled.",
		Anchor:    &review.AnchorRecord{FilePath: "site/release.tsx", Side: review.SideNew, LineNo: 14, LineText: "  return mount(notes)"},
		Suggested: "  theme.apply()\n  return mount(notes)",
	})
	drained, err := rev.Drain(ctx, review.DrainRequest{
		DeliverableID: "d-site", AttemptRef: "r-ship#round-2", RunID: "r-ship",
	})
	if err != nil {
		t.Fatalf("drive review.Drain: %v", err)
	}
	if len(drained) != 1 || drained[0].Number != 1 {
		t.Fatalf("the drain must number the one open blocker [F1], got %+v", drained)
	}

	mint("r-ship", review.MintInput{DeliverableID: "d-site", N: 2, SnapshotSHA: fxSiteSnap2, Files: map[string]string{
		"site/release.tsx": fxSiteRev2, "site/README.md": fxSiteReadme,
	}})

	// The five placement statuses, each produced by the ladder rather than
	// asserted: the surface has to render every one of them, and a body missing a
	// status is a render nobody checked.
	//
	//  mapped  — anchored in rev 1 in the UNTOUCHED middle of the file, so the
	//            diff's own line delta moves it and the quote confirms there.
	//  drifted — an OLD-side anchor of revision 1, whose old side is the pre-task
	//            base the S13.5 topology has not materialized, so the ladder skips
	//            the map and finds the quote near the claimed position instead.
	//  orphan  — anchored in a file revision 2 deleted: no live location anywhere.
	//  file    — a file-level comment, first-class and positionless by choice.
	//  exact   — made on revision 2 itself, at the line it names.
	comment(review.CommentInput{
		RevisionN: 1,
		Body:      "loadNotes() can return an empty list — is that a render or an error?",
		Anchor:    &review.AnchorRecord{FilePath: "site/release.tsx", Side: review.SideNew, LineNo: 9, LineText: "export function loadNotes() {"},
	})
	comment(review.CommentInput{
		RevisionN: 1, Severity: review.SeverityBlocker,
		Body:   "This component has no error boundary above it.",
		Anchor: &review.AnchorRecord{FilePath: "site/release.tsx", Side: review.SideOld, LineNo: 3, LineText: "export function ReleasePage() {"},
	})
	comment(review.CommentInput{
		RevisionN: 1,
		Body:      "Worth checking nothing still imports this before it goes.",
		Anchor:    &review.AnchorRecord{FilePath: "site/legacy.tsx", Side: review.SideNew, LineNo: 2, LineText: "export const legacy = true"},
	})
	comment(review.CommentInput{
		RevisionN: 2, FileLevel: "site/README.md",
		Body: "The README should say where the notes come from.",
	})
	comment(review.CommentInput{
		RevisionN: 2,
		Body:      "Applying the theme here is the right place.",
		Anchor:    &review.AnchorRecord{FilePath: "site/release.tsx", Side: review.SideNew, LineNo: 15, LineText: "  theme.apply()"},
	})

	// One VERIFICATION FINDING, through the other ingress of the same schema
	// (S13.1/S13.3: one comment schema, two ingresses). Its anchor is a section
	// reference rather than a position, so it records file-level with the original
	// string kept verbatim in origin_anchor — the field the surface renders as the
	// finding's own claim of where it applies.
	if _, err := rev.AddFindings(ctx, "d-site", 2, []review.FindingInput{{
		Author: "alice", RunID: "r-ship", Kind: review.KindFinding, Severity: review.SeverityNote,
		Category: "accessibility", Criterion: "AC-2",
		Body:      "The page sets no document title, so a screen reader announces the URL.",
		RawAnchor: "section:accessibility",
	}}); err != nil {
		t.Fatalf("drive review.AddFindings: %v", err)
	}

	// All five statuses are the POINT of this world, so their presence is
	// ASSERTED here rather than hoped for. A content edit that quietly collapsed
	// two of them would leave a render nobody checks while the committed body
	// still looked plausible — an absence failing silently, which is worse than
	// the ambiguity the five statuses exist to remove.
	_, placements, err := rev.PlacedComments(ctx, "d-site", 2)
	if err != nil {
		t.Fatalf("read placements: %v", err)
	}
	seen := map[review.AnchorStatus]bool{}
	for _, p := range placements {
		seen[p.Status] = true
	}
	for _, want := range []review.AnchorStatus{
		review.AnchorExact, review.AnchorMapped, review.AnchorDrifted, review.AnchorFile, review.AnchorOrphan,
	} {
		if !seen[want] {
			t.Fatalf("the review fixture world produces no %q placement — the surface would have no ground for it (placements: %+v)",
				want, placements)
		}
	}

	// ── the object surfaces ─────────────────────────────────────────────────
	// A content-pinned deliverable of a type with NO rich comparison: the honest
	// extracted-text FALLBACK, labeled. Without it the surface's fallback-diff lane
	// had no committed ground at all — the PDF lane degrades past it to the cards,
	// so the two are different renders rather than one.
	ensure("d-notebook", "t-ship", "notebook", "analysis/report.ipynb")
	for n, body := range []string{"cells: 3\nsummary: draft\n", "cells: 4\nsummary: reviewed\n"} {
		mint("r-ship", review.MintInput{DeliverableID: "d-notebook", N: n + 1,
			Files: map[string]string{"analysis/report.ipynb": body}})
	}

	ensure("d-hero", "t-ship", "image", "site/hero.png")
	ensure("d-bundle", "t-ship", "binary", "dist/site.tar")
	// The PDF hangs off the task whose rework card is open, so the deliverable
	// whose request-revision door is LIVE is a real one rather than a construction.
	ensure("d-brief", "t-brief", "pdf", "docs/brief.pdf")
	for n, side := range []string{"OLD", "NEW"} {
		mint("r-ship", review.MintInput{DeliverableID: "d-hero", N: n + 1,
			Objects: map[string][]byte{"site/hero.png": []byte("\x89PNG\r\n\x1a\n" + side)},
			Types:   map[string]string{"site/hero.png": "image/png"}})
		mint("r-ship", review.MintInput{DeliverableID: "d-bundle", N: n + 1,
			Objects: map[string][]byte{"dist/site.tar": []byte("site-bundle-" + side)},
			Types:   map[string]string{"dist/site.tar": "application/x-tar"}})
		// Bytes that are NOT a PDF, which is the degrade path a corrupt or
		// unsupported document takes: the surface falls back to the metadata cards
		// with the reason on the label, never a refusal (S13.2).
		mint("r-brief", review.MintInput{DeliverableID: "d-brief", N: n + 1,
			Objects: map[string][]byte{"docs/brief.pdf": []byte("not-a-pdf-" + side)},
			Types:   map[string]string{"docs/brief.pdf": "application/pdf"}})
	}
}

// seedFixtureChatBornTask puts the task the S15.7 handoff gives birth to into
// the world: the task, its intake run, and the OPEN INTAKE CARD as a real
// `asks` row served through the real projection (drain r1 D2 — the chat feed
// answers that card in place through the LANDED approvals verb, which pins an
// answer to the card's own payload hash, so the widget needs a queue row where
// the hash, the derived action vocabulary and `answerable` are the REAL
// projector's). The snapshot is the SAME fixtureChatBornCard the handoff view
// carries: one card value, two renderings, so the queue and the feed cannot
// drift.
//
// WHY THESE ROWS ARE SEEDED AT ALL, precisely. This world's intake seam is
// fixtureIntake — a double, because internal/api never speaks the pipeline's
// vocabulary — so no handoff turn births anything here. In PRODUCTION the born
// rows arrive in two instalments, and neither one produces an ask at handoff
// time: `stage.Surface.Submit` calls `intake.Pipeline.Start`, which inserts the
// task and its `<task>.intake` run and NOTHING ELSE (intake/pipeline.go's
// birth transaction — task, run, first state event), and the interview card is
// issued LATER by `intake`'s issueCard, which inserts the ask row and
// TRANSITIONS THE RUN TO `parked` in the same transaction ("gates wait",
// S06.1). So a task holding an open interview card has a PARKED intake run —
// `running` beside an open ask is a state the intake pipeline cannot produce,
// and TestFixtureBornTaskStateIsOneTheIntakePipelineProduces reads that rule
// out of the pipeline's own source rather than trusting this comment.
//
// WHY AFTER driveFixtureChat. Nothing in the chat drive reads these rows — the
// handoff answers off the fixtureIntake seam, not the DB — and the ask is only
// needed when the committed BODIES are read, which is after fixtureWorld
// returns. Seeded at the top of the world instead, the born run appeared in
// turn 1's `cost_per_run` answer: the conversation's FIRST answer reported the
// run of a task the conversation gives birth to two turns later. Served order
// is unaffected either way (`/api/runs` orders by created_ts, run_id).
func seedFixtureChatBornTask(t *testing.T, b *backend) {
	t.Helper()
	exec(t, b, `INSERT INTO tasks (task_id, user_id, title, kanban_status, created_ts) VALUES (?,?,?,?,?)`,
		"t-chatborn", "alice", "Draft the release notes", "intake", fxT2)
	exec(t, b, `INSERT INTO runs (run_id, user_id, task_id, state, lane, generation, created_ts, updated_ts)
	            VALUES (?,?,?,?,?,0,?,?)`,
		"t-chatborn.intake", "alice", "t-chatborn", "parked", "anthropic", fxT2, fxT2)
	exec(t, b, `INSERT INTO asks (ask_id, run_id, user_id, snapshot, status, observed_ts) VALUES (?,?,?,?,?,?)`,
		"ask-chatborn-1", "t-chatborn.intake", "alice", fixtureChatBornCard(t), "question", fxT2)
}

// driveFixtureChat seeds the S15.7 assistant world THROUGH ITS REAL VERBS
// (B6-7 R18): a real session created, a real upload, real turns opened and
// settled by the real transport, and one turn left RUNNING so the widget has a
// committed body for the in-flight state it has to re-attach to.
//
// Nothing here inserts a chat row by hand. That is the whole discipline: a
// fixture hand-written from an imagined payload passes its own test and serves
// nothing (the B6-5 lesson), so every byte below came out of the same handler
// production runs.
//
// The store's clock AND its id generator are pinned, because a golden fixture
// cannot carry a wall-clock stamp or a fresh random id and still be reviewable.
func driveFixtureChat(t *testing.T, b *backend) {
	t.Helper()
	ctx := context.Background()
	n := 0
	store, err := chat.New(chat.Config{
		DB: b.db, Log: b.log, Root: fixtureRoot(t, b, "exchange"),
		Now: func() time.Time { return mustTime(t, fxT2) },
		NewID: func(prefix string) string {
			n++
			return fmt.Sprintf("%s%016d", prefix, n)
		},
	})
	if err != nil {
		t.Fatalf("chat.New: %v", err)
	}
	b.chat = store
	// duringTurn models a SECOND REQUEST arriving while a turn is in flight. The
	// turn verb is synchronous — begin, act, settle in one handler call — so the
	// only honest way a single-threaded fixture can interleave another request is
	// from inside the seam the turn is blocked on. What it models is ordinary at
	// v0: uploads are the only thing that writes the exchange folder (OQ7), and a
	// person can drop a file into the sidebar while a turn is still running.
	var duringTurn func()
	srv := func(who string) *api.Server { return fixtureChatServer(t, b, who, duringTurn) }

	post := func(who, path, body string) string {
		t.Helper()
		rr := httptest.NewRecorder()
		srv(who).Handler().ServeHTTP(rr, httptest.NewRequest("POST", path, strings.NewReader(body)))
		if rr.Code != http.StatusOK {
			t.Fatalf("POST %s as %s: %d: %s", path, who, rr.Code, rr.Body.String())
		}
		return rr.Body.String()
	}

	var created struct {
		Session chat.Session `json:"session"`
	}
	if err := json.Unmarshal([]byte(post("alice", "/api/chat/sessions", "")), &created); err != nil {
		t.Fatalf("decode created session: %v", err)
	}
	sid := created.Session.ID

	// A real upload, so the file sidebar has a real manifest row to render.
	post("alice", "/api/chat/files?name=quarterly-numbers.csv", "run_id,usd\nr-ship,1.42\n")

	// Turn 1 — a Layer-0 view: the deterministic answer, served as the store
	// made it.
	post("alice", "/api/chat/sessions/"+sid+"/turns",
		`{"kind":"view","view":"cost_per_run","text":"what did each run cost?"}`)
	// Turn 2 — the NL floor with nothing resolvable: the disambiguation card,
	// which is the honest refusal a neither-verb turn renders.
	post("alice", "/api/chat/sessions/"+sid+"/turns",
		`{"kind":"ask","text":"write me a poem about the sea"}`)
	// Turn 3 — the S06 handoff: the born task with its OPEN intake card, which
	// is what the feed renders in place (OQ8(i)). It is ALSO the fixture's
	// non-empty produced-files case: a real upload lands through the real handler
	// strictly between this turn's BeginTurn and its SettleTurn, so the window
	// diff (`seq > watermark`) honestly attributes it — the exact mechanism the
	// chips render. Turns 1 and 2 stay honestly EMPTY: sparse chips are the v0
	// truth and both renders now have a committed body behind them.
	duringTurn = func() {
		post("alice", "/api/chat/files?name=release-notes-draft.md",
			"# Release notes\n\n- the merged changes, one plain line each\n")
	}
	post("alice", "/api/chat/sessions/"+sid+"/turns",
		`{"kind":"task","title":"Draft the release notes","text":"pull the merged PRs and draft release notes"}`)
	duringTurn = nil

	// Turn 4 — the Layer-2 ESCALATION, reached only because the caller named
	// `open_sql` (OQ3: reaching open SQL is an act). It is the committed body for
	// the render G3 D3.5 exists for: a layer-2 answer carrying `lower-confidence`
	// with its audit block, at 200. This world composes internal/history with no
	// read-only handle and no duty caller, so the honest outcome is `unavailable`
	// — a LADDER DEGRADATION, which is exactly one of the two facts part B has to
	// render as the answer it is rather than as an error banner.
	post("alice", "/api/chat/sessions/"+sid+"/turns",
		`{"kind":"open_sql","text":"which runs cost the most last week?"}`)

	// The last turn is left RUNNING: the in-flight state has to have a committed
	// body, because "the turn survives navigation" is a render over served state
	// and a render with no fixture is a render nobody drove.
	if _, _, err := store.BeginTurn(ctx, "alice", sid, chat.KindAsk, "and how much did last week cost?"); err != nil {
		t.Fatalf("seed the in-flight turn: %v", err)
	}

	// A second session, left empty and unnamed — the honest untitled state the
	// list has to render without inventing a placeholder.
	post("alice", "/api/chat/sessions", "")
}

// fixtureChatServer is the fixture world's chat-serving stack. It is separate
// from fixtureServer only because the chat store is composed inside
// driveFixtureChat (it needs the pinned clock and id seam) and the intake seam
// answers the handoff. `during`, when set, runs inside the handoff act — see
// duringTurn in driveFixtureChat.
func fixtureChatServer(t *testing.T, b *backend, who string, during func()) *api.Server {
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
		Intake: fixtureIntake{t: t, during: during}, Chat: b.chat,
		Now: func() time.Time { return mustTime(t, fxT4) },
	})
}

// driveFixtureSettings puts the settings surface into a state worth rendering,
// through the REAL registry verbs (B6-6 part B): a platform value override, a
// per-user override, and an operator bounds edit — so the committed bodies
// carry the `overridden` flag, a populated `user_values`, and EFFECTIVE bounds
// that differ from the ratified clamp. A registry read of an untouched index
// would render every one of those as its default and prove none of them.
func driveFixtureSettings(t *testing.T, b *backend) {
	t.Helper()
	post := func(path, body string) {
		rr := httptest.NewRecorder()
		fixtureServer(t, b, "op").Handler().ServeHTTP(rr, httptest.NewRequest("POST", path, strings.NewReader(body)))
		if rr.Code != http.StatusOK {
			t.Fatalf("drive settings %s: %d: %s", path, rr.Code, rr.Body.String())
		}
	}
	// A platform value, twice, so the key's audit history has more than one row
	// and a reader can see what changed from what.
	post("/api/settings/freshness.max_age", `{"value":43200,"reason":"plans go stale faster than a day here"}`)
	post("/api/settings/freshness.max_age", `{"value":21600,"reason":"faster still while the release is in flight"}`)
	// The operator's bounds edit — the G1 rider 1 split, which is the only way
	// an EFFECTIVE bound differs from the ratified clamp.
	post("/api/settings/freshness.max_age/bounds", `{"floor":7200,"ceiling":172800,"reason":"narrowed for this household"}`)
	// A per-user override on a PerUser key, set by the operator FOR a member —
	// the registry admits no member actor, so this is the only way one exists.
	post("/api/settings/intake.zero_interaction_cost_usd",
		`{"value":0.25,"for_user":"alice","reason":"alice's trivial band is tighter"}`)

}

// seedFixtureOversightCards puts ONE card of every remaining inbox kind in the
// world, so the committed inbox body is the whole ranked queue rather than two
// of its nine kinds (B6-6 R1). Rows go in with literal stamps, like everything
// else here.
//
// The two watchdog shapes are both present on purpose: a RUN-SCOPED flag, which
// carries the S14.4 resume door beside suppress (B6-6 OQ9(b)), and a RUN-LESS
// platform flag, which carries only suppress because there is no run to resume.
// A fixture with one of them could not show that difference.
func seedFixtureOversightCards(t *testing.T, b *backend) {
	t.Helper()
	event := func(owner, run, typ, payload, ts string) {
		if run == "" {
			exec(t, b, `INSERT INTO run_events (run_id, generation, user_id, type, schema_version, payload, ts)
			            VALUES (NULL, NULL, ?,?,1,?,?)`, owner, typ, payload, ts)
			return
		}
		exec(t, b, `INSERT INTO run_events (run_id, generation, user_id, type, schema_version, payload, ts)
		            VALUES (?,0,?,?,1,?,?)`, run, owner, typ, payload, ts)
	}

	event("alice", "r-ship", "watchdog.flagged",
		`{"rule":"watchdog.loop","anomaly_class":"watchdog.loop","severity":"flag-now",`+
			`"detail":"the same tool call repeated 12 times with no change in its arguments"}`, fxT3)
	// The suffixed class of a run-less flag, verbatim — trimming it to the bare
	// rule is what made such flags un-clearable before the §34 D5 fix.
	event("platform", "", "watchdog.flagged",
		`{"rule":"watchdog.spend","anomaly_class":"watchdog.spend:alice","severity":"digest",`+
			`"detail":"this week's spend is above the usual band for this account"}`, fxT3)

	exec(t, b, `INSERT INTO conformance_registry
	    (row_id, owning_section, fixtures, trigger_set, schedule, cadence, affect_class, last_run, last_result)
	    VALUES (?, 'S14.5', 'go test ./internal/api/', 'quarterly', 'quarterly sweep', 'quarterly', 'lane', ?, 'red')`,
		"api-read-surface", fxT2)

	event("platform", "", "drift.finding",
		`{"source":"anthropic-changelog","lanes":["anthropic"],"change_class":"breaking","severity":"flag-now",`+
			`"summary":"the messages endpoint deprecates a parameter this platform sends","fingerprint":"fp-anthropic-1",`+
			`"row_id":"w-anthropic","classified":true}`, fxT3)
	event("platform", "", "benchmark.alarm",
		`{"action":"raise","domain":"blind-pairs","epoch_id":"e1","severity":"flag-now",`+
			`"summary":"the platform arm is losing its own blind comparison in this domain",`+
			`"loss_g":0.96,"threshold":0.95,"expansion_freeze":true}`, fxT3)

	// Two recorded suite results — the D4(b) surface's own rows. The payload key
	// set is internal/conformance's scorePayload verbatim, and the two paths
	// differ honestly: the runbook registers a floor inside `metrics`, the sweep
	// does not, so a null floor is a fact about the path rather than a zero.
	event("platform", "", "eval.score_recorded",
		`{"suite_id":"routing-regression","suite_version":"v3","asset_id":"selector-v7","asset_version":"7",`+
			`"runner":"internal/evals","runner_version":"v1","result":"green",`+
			`"metrics":{"floor":0.82,"floor_green":true,"floor_registered":true}}`, fxT2)
	event("platform", "", "eval.score_recorded",
		`{"suite_id":"prompt-sweep","suite_version":"v1","asset_id":"worker-notes","asset_version":"2",`+
			`"runner":"internal/evals","runner_version":"v1","result":"red",`+
			`"metrics":{"assets_evaluated":4,"assets_red":1}}`, fxT3)

	// A pair waiting for its blind verdict, and a pair whose direct arm ended
	// without producing anything — the EIGHTH kind, which rides the same id
	// space at Low tier with decline as its only act.
	pair := `INSERT INTO benchmark_pairs
	    (pair_id, user_id, domain, task_id, deliverable_id, phase, rate_pct, sampled_ts, state,
	     direct_run_id, render_a, render_b, direct_text, updated_ts)
	    VALUES (?,?,?,?,?,'pre-gate',100,?,?,?,?,?,?,?)`
	exec(t, b, pair, "bp-notes", "alice", "blind-pairs", "t-ship", "d-notes", fxT2, "rendered",
		"", "Release notes, draft one: three entries, one line each.",
		"Release notes: three entries with a short line each.", "the direct arm's own answer", fxT4)
	exec(t, b, pair, "bp-archive", "alice", "blind-pairs", "t-archive", "d-notes", fxT3, "dispatched",
		"r-notes", "", "", nil, fxT4)
}

// driveFixtureMemoryConflict produces the NINTH kind through the REAL write
// path (B6-6 OQ1): two entries that share a topic key, posted through
// POST /api/memory, so the conflict row under the card is one the S09.7
// detection actually minted rather than a row somebody imagined.
//
// The question is addressed to the owner of the entry that was ALREADY THERE,
// and detection runs over what the new entry's author can SEE — so both entries
// are alice's own. Two of her lessons about one topic that say opposite things
// is the case S09.7 exists for, and it is the shape that lands the card on the
// person who can actually settle it.
func driveFixtureMemoryConflict(t *testing.T, b *backend) {
	t.Helper()
	post := func(who, body string) {
		rr := httptest.NewRecorder()
		fixtureServer(t, b, who).Handler().ServeHTTP(rr,
			httptest.NewRequest("POST", "/api/memory", strings.NewReader(body)))
		if rr.Code != http.StatusOK {
			t.Fatalf("seed memory entry as %s: %d: %s", who, rr.Code, rr.Body.String())
		}
	}
	post("alice", `{"scope":"user","kind":"lesson","title":"how I write release notes",`+
		`"content":"one line per merged change, newest first","topic_key":"release-notes-style"}`)
	post("alice", `{"scope":"user","kind":"lesson","title":"release notes, second thoughts",`+
		`"content":"group the notes by project, oldest first","topic_key":"release-notes-style"}`)
	ctx := context.Background()
	conflicts, err := b.mem.OpenConflictsFor(ctx, "alice")
	if err != nil {
		t.Fatalf("read the seeded conflict: %v", err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("the two same-topic entries produced %d open conflicts, want 1 — the ninth kind has no fixture", len(conflicts))
	}

	// The knowledge store mints a RANDOM entry id per entry (crypto/rand), and
	// no committed file can carry one — the same concession the effect
	// journal's UUID forced (drain r1). The ids are SURROGATES pinned after the
	// real write path ran: the detection, the conflict row and the gate's own
	// question sentence are all the producer's, and only the two opaque
	// identifiers inside them are made reproducible. Two surrogate entries go
	// in first so the conflict's foreign keys stay satisfied at every step; the
	// real pair STAYS, because a knowledge entry is never deleted (the S09.5
	// audit trigger refuses it, which is the store protecting its own record).
	// The extra rows reach no fixture — the conflict card names the surrogates,
	// and no committed body reads the entry list.
	c := conflicts[0]
	for i, id := range []string{fxEntryA, fxEntryB} {
		real := []string{c.EntryID, c.OtherEntryID}[i]
		var title, content, approvedBy, verifiedBy string
		if err := b.db.QueryRowContext(ctx,
			`SELECT title, coalesce(content,''), approved_by, verified_by
			   FROM knowledge_entries WHERE entry_id = ?`, real).
			Scan(&title, &content, &approvedBy, &verifiedBy); err != nil {
			t.Fatalf("read seeded entry %s: %v", real, err)
		}
		// The S09.5 provenance and the S09.8 verification stamps are copied FROM
		// the gate's own row rather than typed here (P3-UI-3): the manual write
		// records its actor as both approver and verifier at that instant, and a
		// committed body that dropped them would have made the surface's
		// provenance block a render nobody had checked against a real one. Only
		// the two wall-clock instants are pinned, for the same reason the id is.
		exec(t, b, `INSERT INTO knowledge_entries
		    (entry_id, user_id, scope, layer, kind, title, content, topic_key, status, version, origin,
		     approved_by, approved_ts, verified_by, verified_ts, created_ts, updated_ts)
		    VALUES (?,?,'user','L2','lesson',?,?,'release-notes-style','active',1,'human_direct',?,?,?,?,?,?)`,
			id, "alice", title, content, approvedBy, fxT2, verifiedBy, fxT2, fxT2, fxT2)
	}
	exec(t, b, `UPDATE knowledge_conflicts
	       SET entry_id = ?, other_entry_id = ?,
	           question = replace(replace(question, ?, ?), ?, ?),
	           detected_ts = ?
	     WHERE conflict_id = ?`,
		fxEntryA, fxEntryB, c.EntryID, fxEntryA, c.OtherEntryID, fxEntryB, fxT2, c.ID)
}

// The surrogate knowledge-entry ids the committed conflict card names.
const (
	fxEntryA = "k-fixture-release-notes-a"
	fxEntryB = "k-fixture-release-notes-b"
)

// driveFixtureDecisions produces the Human-decision rows through the REAL
// verbs — the OQ2 fidelity promise, and the root cause of drain r1.
//
// A hand-written payload proves only that the derive can read the keys its
// author imagined. Driving the producers is what proved that a real
// `deliverable.accepted` is PLATFORM-scoped (so the run-scoped half could
// never see it) and that `intake.delta_decision` names no actor at all.
func driveFixtureDecisions(t *testing.T, b *backend) {
	t.Helper()
	// Driven as the card's OWNER: a High-tier effect is the owner's to approve
	// (D10), and the operator is not excepted — the 403 that this returns for
	// anyone else is the landed rule, not a fixture inconvenience.
	srv := fixtureServer(t, b, "alice")

	// 1. A REAL effect approval, read-then-answer exactly as a client does:
	//    the card carries the pinned payload hash, and answering with anything
	//    else is a 409 stale_payload. Driving it this way is what makes the
	//    resulting `decision.recorded` a real producer's row rather than a
	//    shape somebody imagined.
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/api/approvals", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("read approvals: %d: %s", rr.Code, rr.Body.String())
	}
	var inbox struct {
		Items []struct {
			ID          string `json:"id"`
			PayloadHash string `json:"payload_hash"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &inbox); err != nil {
		t.Fatalf("decode approvals: %v", err)
	}
	var hash string
	for _, it := range inbox.Items {
		if it.ID == "effect:e-publish" {
			hash = it.PayloadHash
		}
	}
	if hash == "" {
		t.Fatalf("the effect card is not in the inbox: %s", rr.Body.String())
	}
	body := fmt.Sprintf(`{"payload_hash":%q,"pin":%q,"answer":{"action":"approve","reason":"the release is due"}}`,
		hash, fixturePIN)
	ans := httptest.NewRecorder()
	srv.Handler().ServeHTTP(ans, httptest.NewRequest("POST", "/api/approvals/effect:e-publish/answer", strings.NewReader(body)))
	if ans.Code != http.StatusOK {
		t.Fatalf("drive effect answer: %d: %s", ans.Code, ans.Body.String())
	}

	// 1b. The D10 CO-APPROVAL, driven for real: the owner signs, then the
	//     OPERATOR signs the same platform-level card. Two rows, one card, and
	//     the second carries actor_is_operator.
	answerEffect(t, b, "alice", "e-rotate")
	answerEffect(t, b, "op", "e-rotate")

	// 1c. The operator's own priority hint. `answerEffect` above proves the
	//     operator limb on the APPROVALS family; this proves it where a TASK
	//     page can show it — a platform-level effect is attributed to no run,
	//     so by construction it is nobody's task decision, while a hint names
	//     its task as the subject.
	hint := httptest.NewRequest("POST", "/api/tasks/t-ops/priority-hint",
		strings.NewReader(`{"rank":10,"reason":"key rotation is due"}`))
	hr := httptest.NewRecorder()
	fixtureServer(t, b, "op").Handler().ServeHTTP(hr, hint)
	if hr.Code != http.StatusOK {
		t.Fatalf("drive operator priority hint: %d: %s", hr.Code, hr.Body.String())
	}

	// 2. A REAL deliverable accept through review.Accept — platform-scoped,
	//    payload {deliverable_id, revision, project_id, accepted_by, superseded},
	//    nothing naming the task.
	if _, err := fixtureReview(t, b).Accept(context.Background(), "d-notes", "alice"); err != nil {
		t.Fatalf("drive review.Accept: %v", err)
	}

	// 3. A REAL intake delta decision: internal/intake's own measurement-hook
	//    payload, byte-for-byte the key set delta.go marshals — no actor key,
	//    which is precisely why the derive falls back to the row's owner.
	exec(t, b, `INSERT INTO run_events (run_id, generation, user_id, type, schema_version, payload, ts)
	            VALUES (?,0,?,?,1,?,?)`, "r-ship", "alice", "intake.delta_decision",
		`{"delta_id":"delta-1","origin":"verify-findings","presented_items":3,"presented_bytes":812,`+
			`"time_to_decision_s":94,"decision":"accept","task_id":"t-ship","spec_plan_version":"spec-v2/plan-v2"}`, fxT4)

	// 4. The negative: a decision about ANOTHER task, minted the same way, so
	//    "it does not appear" is a fact about the derive rather than about the
	//    fixture being thin.
	exec(t, b, `INSERT INTO run_events (run_id, generation, user_id, type, schema_version, payload, ts)
	            VALUES (NULL,NULL,?,?,1,?,?)`, "alice", "decision.recorded",
		`{"actor":"alice","card_id":"priority_hint:t-elsewhere","card_type":"priority_hint","decision":"reorder",`+
			`"subject":"t-elsewhere","presented_at":"2026-07-20T09:07:00Z","decided_at":"2026-07-20T09:07:00Z"}`, fxT4)
}

// ── the stored ask snapshots, from the REAL intake.Card types ───────────────
//
// internal/api production code never imports internal/intake — the pipeline
// rides the IntakeSurface seam, and the whole point of the seam is that the
// transport does not speak the pipeline's vocabulary. This is a TEST producing
// faithful stored bodies, exactly as the receipt fixture marshals a real
// metering.Receipt.

func fixtureCardJSON(t *testing.T, card intake.Card) string {
	t.Helper()
	raw, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("marshal %s card: %v", card.Kind, err)
	}
	return string(raw)
}

// fixtureChatBornCard is the S06.5 batched option card a task born FROM A CHAT
// TURN opens with (B6-7 OQ8(i)). It is in the fixture set because the chat feed
// renders it INLINE: the conversation continues in place against the same card
// the task surface shows, answered through the LANDED ask verbs — same
// vocabulary, no second answer path — so the render needs a committed body.
// fixtureChatBornClearance is the born task's clearance. It is stated ONCE and
// read by both the card and the task view, because they are two renderings of
// one number and a fixture where they disagree teaches the widget a world that
// cannot happen.
const fixtureChatBornClearance = 0.48

func fixtureChatBornCard(t *testing.T) string {
	t.Helper()
	return fixtureCardJSON(t, intake.Card{
		Kind: intake.CardInterview, TaskID: "t-chatborn", RunID: "t-chatborn.intake",
		Version: 1, IssuedTS: fxT2, Clearance: fixtureChatBornClearance, Tier: intake.TierStandard,
		Questions: []intake.Question{
			{
				ID: "audience", Text: "Who are these release notes for?", Weight: 3,
				Options: []intake.Option{
					{Label: "The household", Value: "household"},
					{Label: "Just me", Value: "self"},
				},
			},
			{
				ID: "range", Text: "Which changes should it cover?", Weight: 2,
				Options: []intake.Option{
					{Label: "Since the last release", Value: "since_last_release"},
					{Label: "This month", Value: "this_month"},
				},
			},
		},
	})
}

// fixtureApprovalCard is the S06.9 Stage-4 card: one phone screen with the
// assumptions as its centerpiece, the 13.5 help block, the expandable Layer 2,
// and the card's own Approve · Re-plan · Re-interview vocabulary. It carries a
// STORED staleness flag as well, so a surface sees both sources of the flag —
// the one the card was issued with and the one derived from its age at read.
func fixtureApprovalCard(t *testing.T) string {
	t.Helper()
	return fixtureCardJSON(t, intake.Card{
		Kind: intake.CardApproval, TaskID: "t-ship", RunID: "r-ship", Version: 2,
		IssuedTS: fxT2, Clearance: 0.82, Tier: intake.TierStandard,
		Approval: &intake.ApprovalBody{
			Layer1: intake.ApprovalLayer1{
				Restatement: "Write the release notes for this cycle from the merged changes, and publish them where the household reads them.",
				Deliverable: []string{"a release-notes document with one entry per merged change"},
				Steps: []string{
					"Collect every merged change since the last release",
					"Write one plain-language line per change",
					"Publish the notes to the household's notes folder",
				},
				WillNotDo: []string{"translate the notes", "post anything outside the household"},
				Assumptions: []intake.Assumption{
					{Text: "the changelog is the source of truth for what merged", Origin: "slot:source"},
					{Text: "one line per change is enough detail", Origin: "band"},
				},
				Risks:     []string{"the changelog may be incomplete for the last two days"},
				CostTime:  "about 4 minutes of model time; no outward effect until you approve one",
				Clearance: 0.82,
				SizeClass: "small",
				Help: intake.HelpBlock{
					What:      "Approving starts the work exactly as planned; nothing runs before you approve.",
					Wrong:     "If an assumption below is wrong, the result will miss what you actually wanted.",
					Recommend: "Read the restatement and the assumptions. If they match your intent, approve; if anything is off, contest it via Re-plan.",
				},
			},
			Layer2: intake.ApprovalLayer2{
				ACs: []intake.AC{
					{N: 1, Plain: "Every merged change since the last release is listed once."},
					{N: 2, Plain: "Each entry says what changed in plain language.",
						Structured: "WHEN a reader opens the notes THEN each entry reads as a sentence", StructuredKind: "gwt"},
				},
				// The [A15] per-step approach and the r5 §B.1 understanding
				// fields the card now serves (P3-GF8 R4/R17): GF9 draws these,
				// so the committed body carries the real shapes.
				Steps: []intake.Step{
					{ID: "S-1", Title: "Collect the merged changes", DoneWhen: "every merge since the last tag is listed",
						Approach: "I read the merge log since the last release tag and list every entry once, newest first.",
						Decisions: []intake.StepDecision{{
							Decision:     "read the merge log rather than the commit log",
							Alternatives: []string{"walk every commit", "diff the two release tags"},
							Why:          "one line per merged change is what the notes list, and the merge log already says that",
						}},
						OrderingRationale: "nothing can be written up before the list of what changed exists"},
					{ID: "S-2", Title: "Write one line per change", DoneWhen: "each listed change has a plain sentence",
						Approach: "I write one plain sentence per listed change, saying what a reader would notice."},
				},
				Coverage:    map[string][]string{"AC-1": {"S-1"}, "AC-2": {"S-2"}},
				Constraints: []string{"no external publishing"},
				Supplied:    []intake.SuppliedFact{{RuleID: "P47-7", Fact: "the last release tag is v2.3.0, cut on 2026-07-06", TS: fxT1}},
				Estimate:    intake.Estimate{SizeClass: "small", USD: 0.12, Known: true, Basis: "median of the last five notes runs"},
			},
			Actions:      []string{intake.ActionApprove, intake.ActionRePlan, intake.ActionReInterview},
			StaleFlag:    true,
			StaleReasons: []string{"the changelog gained three commits since this plan was drafted"},
		},
	})
}

// fixtureTrivialApprovalCard is the zero-interaction band (S06.4 trivial), which
// folds onto the inbox's LOW tier — the only tier that batches. Two of them make
// "one action answers a selected set" a thing a fixture can drive.
func fixtureTrivialApprovalCard(t *testing.T, taskID, runID, restatement string) string {
	t.Helper()
	return fixtureCardJSON(t, intake.Card{
		Kind: intake.CardApproval, TaskID: taskID, RunID: runID, Version: 1,
		IssuedTS: fxT2, Clearance: 0.97, Tier: intake.TierTrivial,
		Approval: &intake.ApprovalBody{
			Layer1: intake.ApprovalLayer1{
				Restatement: restatement,
				Deliverable: []string{"one short note in the household folder"},
				Steps:       []string{"Read the week's finished tasks", "Write the note"},
				Assumptions: []intake.Assumption{{Text: "the note stays under a page", Origin: "band"}},
				CostTime:    "under a minute of model time",
				Clearance:   0.97,
				Help: intake.HelpBlock{
					What:      "Approving starts the work exactly as planned; nothing runs before you approve.",
					Wrong:     "If the week's task list is wrong, the note will describe a week that did not happen.",
					Recommend: "This is a small, reversible piece of work — approve it if the week looks right.",
				},
			},
			Layer2: intake.ApprovalLayer2{
				ACs: []intake.AC{{N: 1, Plain: "The note names every task finished this week."}},
				Steps: []intake.Step{{ID: "S-1", Title: "Write the note", DoneWhen: "the note exists in the household folder",
					Approach: "I read this week's finished tasks and write one short paragraph naming each."}},
				Coverage: map[string][]string{"AC-1": {"S-1"}},
				Estimate: intake.Estimate{SizeClass: "tiny", Known: false, Basis: "no comparable run yet"},
			},
			Actions: []string{intake.ActionApprove, intake.ActionRePlan},
		},
	})
}

// fixtureDeltaCard is the post-approval delta-only card: exactly what changed
// against the frozen artifacts, in the ADDED / MODIFIED / REMOVED vocabulary.
//
// It carries the producer's OWN two-verb vocabulary (Approve · Reject), which
// the drain added at issuance: before it, a real delta card declared none, so a
// pending delta was unanswerable from any surface that renders controls from
// the card — while it held the task's OpenAskID open. The committed body is what
// makes the fix a checkable fact rather than an assertion.
func fixtureDeltaCard(t *testing.T) string {
	t.Helper()
	return fixtureCardJSON(t, intake.Card{
		Kind: intake.CardDelta, TaskID: "t-ship", RunID: "r-ship", Version: 3,
		IssuedTS: fxT2, Clearance: 0.82, Tier: intake.TierStandard,
		Delta: &intake.DeltaBody{
			Origin:  "freshness_revalidation",
			Actions: intake.DeltaActions(),
			Items: []intake.DeltaItem{
				{Kind: intake.DeltaAdded, Target: "AC-3", New: "The notes link each entry to its merge."},
				{Kind: intake.DeltaModified, Target: "S-2",
					Old: "Write one line per change", New: "Write one line per change, newest first"},
				{Kind: intake.DeltaRemoved, Target: "assumption:one line per change is enough detail"},
			},
			Help: intake.HelpBlock{
				What:      "The approved plan changed. Only the listed items differ; everything else stays exactly as approved.",
				Wrong:     "A REMOVED item disappears from the contract; a MODIFIED item changes what gets verified.",
				Recommend: "Read each line — the card shows the complete change.",
			},
		},
	})
}

// fixtureTrivialCoverageCard is a decision card in the zero-interaction band,
// so it folds onto the inbox's LOW tier and batches — with the DECISION family's
// answer vocabulary rather than the approval family's. It is what makes the
// OQ10 mixed-vocabulary constraint a thing a test can watch work.
func fixtureTrivialCoverageCard(t *testing.T) string {
	t.Helper()
	return fixtureCardJSON(t, intake.Card{
		Kind: intake.CardCoverage, TaskID: "t-archive", RunID: "r-archive", Version: 1,
		IssuedTS: fxT2, Clearance: 0.91, Tier: intake.TierTrivial,
		Decision: &intake.DecisionBody{
			Summary: "The archive plan does not cover: AC-1. Auto-fix rounds are exhausted.",
			Detail:  []string{"AC-1"},
			Choices: []intake.Option{
				{Label: "Re-plan once more", Value: intake.ChoiceReplan},
				{Label: "Drop the criterion (recorded, visible)", Value: intake.ChoiceDropCriterion},
			},
			Help: intake.HelpBlock{
				What:      "An agreed acceptance criterion has no plan step delivering it.",
				Wrong:     "Proceeding with the gap means that criterion will not be worked on or verified.",
				Recommend: "Re-plan once more; drop the criterion only if you no longer want it.",
			},
		},
	})
}

// fixtureCoverageCard is the S06.7(a) decision card, and the reason its answer
// vocabulary is in the fixture set at all: its choices are []Option{Label,
// Value}, and the inbox derives the card's actions from VALUE. Until B6-6 it
// read a key no producer writes, so every card of this family served a list of
// empty strings — the committed body is what pins the fix.
func fixtureCoverageCard(t *testing.T) string {
	t.Helper()
	return fixtureCardJSON(t, intake.Card{
		Kind: intake.CardCoverage, TaskID: "t-triage", RunID: "r-triage", Version: 1,
		IssuedTS: fxT2, Clearance: 0.55, Tier: intake.TierStandard,
		Decision: &intake.DecisionBody{
			Summary: "The plan does not cover: AC-2. Auto-fix rounds are exhausted.",
			Detail:  []string{"AC-2"},
			Choices: []intake.Option{
				{Label: "Re-plan once more", Value: intake.ChoiceReplan},
				{Label: "Drop the criterion (recorded, visible)", Value: intake.ChoiceDropCriterion},
				{Label: "Proceed with the gap listed on the approval card", Value: intake.ChoiceProceedUncovered},
			},
			Help: intake.HelpBlock{
				What:      "An agreed acceptance criterion has no plan step delivering it.",
				Wrong:     "Proceeding with the gap means that criterion will not be worked on or verified.",
				Recommend: "Re-plan once more; drop the criterion only if you no longer want it.",
			},
		},
	})
}

// fixturePrices is the S18.3 stored-price seam for the fixture world.
//
// The row is MARSHALED FROM THE REAL metering.StoredPriceRow (the receipt
// precedent, and B6-6 OQ6's own requirement): the price editor composes exactly
// these fields, so pinning the committed body to the owner's struct is what
// stops the form drifting from the shape the store accepts. The APPEND is not
// driven here — a GET fixture never calls it, and the store's validation is the
// authority the UI renders refusals from.
type fixturePrices struct{}

func (fixturePrices) PriceRows(context.Context) (json.RawMessage, error) {
	rows := []metering.StoredPriceRow{{
		ID: 1,
		PriceRow: metering.PriceRow{
			Model: "claude-opus-5", Lane: "anthropic",
			Prices:        metering.UnitPrices{InputUSD: 0.000015, OutputUSD: 0.000075, CacheReadUSD: 0.0000015},
			EffectiveFrom: fixturePriceDate("2026-07-01"),
			VerifiedOn:    fixturePriceDate("2026-07-18"),
			Source:        "the provider's published pricing page, read on the verified-on date",
		},
		CreatedBy: "op", CreatedTS: fixturePriceDate("2026-07-20"),
		Reason: "first row: the household's own lane",
	}}
	return json.Marshal(rows)
}

func (fixturePrices) AddPriceRow(_ context.Context, _, _ string, _ json.RawMessage) (json.RawMessage, error) {
	return nil, &api.SurfaceError{Status: http.StatusNotImplemented, Code: "not_implemented",
		Msg: "the fixture world serves the price READ; the append is the store's own battery"}
}

func (fixturePrices) TableVersion() string { return "prices/2026-07-01#1" }

// fixturePriceDate parses a fixed date. A price row's dates are the operator's
// own data, so they are literals like every other stamp in this world.
func fixturePriceDate(day string) time.Time {
	ts, err := time.Parse("2006-01-02", day)
	if err != nil {
		panic("fixture price date: " + err.Error())
	}
	return ts
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
		// Moved with the sentence at P3-GF13 drain r1 (F5): a museum seed that
		// keeps the retired copy alive is how a purged citation comes back.
		Mode: metering.ModeSummary{Note: "no change of mode during this work"},
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
type fixtureIntake struct {
	t *testing.T
	// during runs inside Submit, while the calling turn is still open. It is the
	// fixture's only way to interleave a second request with an in-flight turn.
	during func()
}

// Submit answers the S15.7 handoff with a faithful born-task view (B6-7). The
// pipeline rides the IntakeSurface seam and internal/api never speaks its
// vocabulary, so this is a TEST producing the body the real taskView produces —
// exactly as fixtureCardJSON produces the stored ask snapshots and the receipt
// fixture marshals a real metering.Receipt.
//
// It carries the born task's OPEN INTAKE CARD, because that is the whole of
// OQ8(i): the conversation continues IN PLACE against the same card the task
// surface would show, answered through the LANDED ask verbs. A fixture without
// the card would leave that render undriven.
// fixtureTaskView and fixtureRunSummary MIRROR internal/stage's taskView and
// runSummary field for field, tag for tag (surface.go). The pipeline rides the
// IntakeSurface seam and internal/api never speaks its vocabulary, so the shape
// cannot be reached by import — but a hand-built payload that drifts from the
// real one is exactly the B6-5 root cause (imagined keys serving a world that
// does not exist), so TestFixtureHandoffMatchesTheRealTaskView reads the real
// definition out of its source and pins these against it.
type fixtureTaskView struct {
	TaskID string `json:"task_id"`
	Title  string `json:"title"`
	Kanban string `json:"kanban_status"`
	Owner  string `json:"owner"`

	Phase     string  `json:"phase,omitempty"`
	Tier      string  `json:"tier,omitempty"`
	Family    string  `json:"family,omitempty"`
	Clearance float64 `json:"clearance,omitempty"`

	OpenAskID string          `json:"open_ask_id,omitempty"`
	OpenCard  json.RawMessage `json:"open_card,omitempty"`

	Runs []fixtureRunSummary `json:"runs"`
}

type fixtureRunSummary struct {
	RunID string `json:"run_id"`
	Role  string `json:"role"`
	State string `json:"state"`
	// HasReceipt is NOT omitempty in the real runSummary, so production emits it
	// on every run — including the false that says a run has produced no receipt
	// yet, which is the whole fact a freshly born task has to render.
	HasReceipt bool `json:"has_receipt"`
}

func (f fixtureIntake) Submit(_ context.Context, userID string, body json.RawMessage) (json.RawMessage, error) {
	if f.during != nil {
		f.during()
	}
	var in struct {
		Title string `json:"title"`
	}
	_ = json.Unmarshal(body, &in)
	view := fixtureTaskView{
		TaskID: "t-chatborn", Title: in.Title, Kanban: "intake", Owner: userID,
		Phase: "interview", Tier: "medium", Family: "content",
		// The real view carries clearance straight off the intake state, and the
		// born card this handoff serves declares its own — one task, one number.
		Clearance: fixtureChatBornClearance,
		OpenAskID: "ask-chatborn-1",
		OpenCard:  json.RawMessage(fixtureChatBornCard(f.t)),
		// `parked`, because this view carries an OPEN interview card and the
		// pipeline parks the run in the same transaction that issues one
		// (intake's issueCard: "gates wait", S06.1) — so the view and the seeded
		// world state agree, and neither is a state the pipeline cannot reach.
		//
		// A FIDELITY GAP, and the correction of what this comment used to claim
		// about it: the REAL Submit returns BEFORE any card is issued (Start
		// births the task and run with no ask; the card and the run's park land
		// in a later transaction), so production's first handoff response carries
		// no open card at all. It said "the feed would meet it on a re-read" —
		// which was FALSE, and demonstrably: a settled turn's outcome has exactly
		// one writer and is served verbatim apart from redaction, so no re-read
		// can ever add a card to it. The widget therefore no longer reads the
		// card from the outcome at all — it reads the person's own decision
		// queue, which is where the card actually appears and where its pin
		// already lived (post-cap RES-1, §44-B). This body keeps its card because
		// this seam's double returns one, and the render is driven BOTH ways: with
		// it and, in the production shape, without it.
		Runs: []fixtureRunSummary{
			{RunID: "t-chatborn.intake", Role: "intake", State: "parked", HasReceipt: false},
		},
	}
	return json.Marshal(view)
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

// fixtureReview and fixtureJournal carry the FIXED clock, so a row minted
// through a real producer is reproducible and can therefore be committed.
// answerEffect approves one effect card as one identity, reading the card for
// its pinned hash exactly as a client does.
func answerEffect(t *testing.T, b *backend, who, effectID string) {
	t.Helper()
	srv := fixtureServer(t, b, who)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/api/approvals", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("read approvals as %s: %d: %s", who, rr.Code, rr.Body.String())
	}
	var inbox struct {
		Items []struct {
			ID          string `json:"id"`
			PayloadHash string `json:"payload_hash"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &inbox); err != nil {
		t.Fatalf("decode approvals: %v", err)
	}
	var hash string
	for _, it := range inbox.Items {
		if it.ID == "effect:"+effectID {
			hash = it.PayloadHash
		}
	}
	if hash == "" {
		t.Fatalf("%s cannot see effect %s: %s", who, effectID, rr.Body.String())
	}
	body := fmt.Sprintf(`{"payload_hash":%q,"pin":%q,"answer":{"action":"approve","reason":"agreed"}}`, hash, fixturePIN)
	ans := httptest.NewRecorder()
	srv.Handler().ServeHTTP(ans,
		httptest.NewRequest("POST", "/api/approvals/effect:"+effectID+"/answer", strings.NewReader(body)))
	if ans.Code != http.StatusOK {
		t.Fatalf("answer %s as %s: %d: %s", effectID, who, ans.Code, ans.Body.String())
	}
}

// fixtureReview returns the world's ONE review store.
//
// Sharing it is load-bearing rather than tidy (B6-8): `Root` is where minted
// revision bytes live, and the store used to be constructed per server over a
// fresh t.TempDir — so a revision minted through the real verb was invisible to
// the next request, and the review reads could only be fixtured off hand-written
// SQL rows with invented content hashes. One store per world is what lets
// `MintRevision`, `AddComment` and `Drain` be the producers of the committed
// bodies. A caller with no composed world still gets a throwaway root.
func fixtureReview(t *testing.T, b *backend) *review.Store {
	t.Helper()
	if b.rev == nil {
		b.rev = &review.Store{
			DB: b.db, Log: b.log, Settings: b.reg, Root: fixtureRoot(t, b, "review"),
			Now: func() time.Time { return mustTime(t, fxT4) },
		}
	}
	return b.rev
}

// fixtureAcceptAndPreview composes the two S13 orchestrations the review surface
// renders doors for. Neither is CALLED by any committed read: the accept card is
// a read that only needs the Accepter to exist (`acceptable()` asks whether the
// orchestration is composed, not what it would do), and no fixture launches a
// preview. Composing them is what makes the served doors say what a real process
// says instead of "not composed in this process" — the doors are data the surface
// renders controls from, so a fixture where every door is closed would exercise
// exactly one of the two directions.
//
// The host-hazard posture is internal/preview's own: NewCaddyClient("", "") is
// routing-disabled, the port range is this package's 47900-47919 (disjoint from
// internal/preview's and internal/shell's), and nothing binds a port because
// nothing launches.
func fixtureAcceptAndPreview(t *testing.T, b *backend) {
	t.Helper()
	rev := fixtureReview(t, b)
	proj, err := project.New(project.Config{DB: b.db, Log: b.log, Root: fixtureRoot(t, b, "projects")})
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	acc, err := accept.New(accept.Config{
		Project: proj, Journal: fixtureJournal(t, b), Push: &fakePusher{}, Review: rev,
		Freshness: b.reg, Now: func() time.Time { return mustTime(t, fxT4) },
	})
	if err != nil {
		t.Fatalf("accept.New: %v", err)
	}
	ports, err := portpool.New(portpool.Config{
		Dir: fixtureRoot(t, b, "portpool"), Lo: 47900, Hi: 47919,
		Now: func() time.Time { return mustTime(t, fxT4) },
	})
	if err != nil {
		t.Fatalf("portpool.New: %v", err)
	}
	prev, err := preview.New(preview.Config{
		Reviews: rev, Projects: proj, Ports: ports,
		Caddy: preview.NewCaddyClient("", ""), Events: b.log,
		Settings: dlvPreviewSettings{cap: 2}, Scratch: fixtureRoot(t, b, "preview-clones"),
		Now: func() time.Time { return mustTime(t, fxT4) },
	})
	if err != nil {
		t.Fatalf("preview.New: %v", err)
	}
	b.acc, b.prev = acc, prev
}

func fixtureJournal(t *testing.T, b *backend) *gates.Journal {
	t.Helper()
	j, err := gates.NewJournal(gates.JournalConfig{
		DB: b.db, Settings: settings.New(), Now: func() time.Time { return mustTime(t, fxT4) },
	})
	if err != nil {
		t.Fatalf("gates.NewJournal: %v", err)
	}
	return j
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
		`"steps":[{"id":"S-1","title":"Collect the merged changes",` +
		`"approach":"I read the merge log since the last release tag and list every entry once, newest first.",` +
		`"decisions":[{"decision":"read the merge log rather than the commit log",` +
		`"alternatives":["walk every commit","diff the two release tags"],` +
		`"why":"one line per merged change is what the notes are supposed to list, and the merge log already says that"}],` +
		`"ordering_rationale":"nothing can be written up before the list of what changed exists"},` +
		`{"id":"S-2","title":"Write one line per change",` +
		`"approach":"I write one plain sentence per listed change, saying what a reader would notice."}],` +
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
		// The scheduler's STORAGE seam is the landed double: constructing a real
		// scheduler is out of proportion here, and what this fixture is proving
		// is the decision ROW the verb mints, which recordDecision writes
		// itself. The verb, its D10 authorization and its row are all real; the
		// queue row below carries the rank the real scheduler would persist.
		// The S10.4 pause switch, REAL — `metering.Pause` over this world's own
		// users table, which is the store the pause verb writes and the meters
		// read now reads. Its position is a committed served fact rather than a
		// scripted one.
		//
		// Its SIBLING is deliberately absent: no Budgets store is wired here,
		// because the lane gauge in this world is `fixtureMeter` and a declared
		// budget would make one response contradict itself — the Layer-0 budget
		// view reporting a declaration while the gauge block beside it reported
		// none, which is exactly the defect §39-B drain D2 fixed. The declared
		// half's real tie lives where the real gauge is: internal/shell's
		// HTTP-level test declares through the verb and requires the two halves
		// of one response to agree.
		Pause:      metering.NewPause(b.db),
		Hints:      newFakeHints("r-ops"),
		Review:     fixtureReview(t, b),
		Accept:     b.acc,
		Preview:    b.prev,
		Effects:    fixtureJournal(t, b),
		Memory:     b.mem,
		MemoryGate: b.memGate,
		Registry:   b.reg,
		Prices:     fixturePrices{},
		Benchmark:  fixtureBenchmark{},
		Chat:       b.chat,
		Workforce:  fixtureWorkforce(t, b),
		Push:       fixturePushStore(t, b),
		// The S00.9 A13 lane-pin set behind GET /api/intake/pinnable-lanes
		// (P3-LN-10a), for the lanes this fixture world already speaks.
		PinnableLanes: fixturePinnableLanes(),
		Now:           func() time.Time { return mustTime(t, fxT4) },
	})
}

// fixturePinnableLanes is the committed world's lane-pin set, produced by the
// REAL producer over a real Coverage — the composition root's own adaptation
// (stage.lanePinOptions), inlined because internal/api holds the set as data.
//
// The three-field copy is harness; the SENTENCE is the rule and is never typed
// by hand. Both row KINDS reach the committed body deliberately: a pinnable row
// (which must carry no `not_pinnable` key at all) and the local engine lane's
// refusal (whose words the picker renders verbatim) — a member no fixture
// exercises is a contract nobody agreed to (§63-R3).
//
// The lanes are the ones this world already speaks: anthropic configured-first,
// kimi and zai commissioned, the local engine lane appended last.
func fixturePinnableLanes() []intake.LanePinOption {
	cov := worker.Coverage{
		FlatRateLanes: []string{adapters.LaneAnthropic, adapters.LaneKimi, adapters.LaneZAI},
		LocalLane:     adapters.LaneLocal,
	}
	pinnable := worker.PinnableLanes(cov)
	out := make([]intake.LanePinOption, 0, len(pinnable))
	for _, p := range pinnable {
		out = append(out, intake.LanePinOption{Lane: p.Lane, Pinnable: p.Pinnable, NotPinnable: p.NotPinnable})
	}
	return out
}

// fixtureBenchmark is the practice seam for the fixture world.
//
// It carries NO import of internal/benchmark: the reverse import wall
// (benchmark/importwall_test.go) scans every .go file under internal/api, tests
// included, and cards derive from the log rather than from the producer. So the
// served vocabulary is written out here, and the tie that keeps it honest lives
// where both vocabularies are legitimately visible — internal/shell's adapter
// test compares THIS committed fixture against the package's own constants, so
// a registration change fails the build instead of quietly leaving the form
// offering last year's buttons.
//
// The ACTS are not exercised by a GET fixture and answer as unwired rather than
// pretending: this is a test producing faithful bodies, not a stand-in for the
// practice, whose own battery proves the pre-record shape carries no arm.
type fixtureBenchmark struct{}

// The pending pair, in the package's pre-record shape: keyed by SIDE, with no
// arm, no position, no run and no model — the fields a blind voter may see, and
// no field that could leak which arm is which.
const fixturePendingPairs = `[{"pair_id":"bp-notes","user_id":"alice","domain":"blind-pairs",` +
	`"task_id":"t-ship","sampled_ts":"2026-07-20T09:02:00Z",` +
	`"render_a":"Release notes, draft one: three entries, one line each.",` +
	`"render_b":"Release notes: three entries with a short line each.",` +
	`"length_a":55,"length_b":52}]`

func (fixtureBenchmark) PendingVerdicts(_ context.Context, requester string) (json.RawMessage, error) {
	if requester != "" && requester != "alice" {
		return json.RawMessage(`[]`), nil
	}
	return json.RawMessage(fixturePendingPairs), nil
}

func (fixtureBenchmark) AnswerVocabulary(context.Context) (api.BenchmarkVocabulary, error) {
	return api.BenchmarkVocabulary{
		Choices:      []string{"A", "B", "tie", "both-bad"},
		GuessSides:   []string{"A", "B"},
		Dispositions: []string{"investigate", "fix-and-continue-accruing", "re-register"},
	}, nil
}

func (fixtureBenchmark) RegisteredValues(context.Context) (json.RawMessage, error) {
	return nil, notWiredInFixture()
}

func notWiredInFixture() error {
	return &api.SurfaceError{Status: http.StatusNotImplemented, Code: "not_implemented",
		Msg: "the fixture world serves the benchmark READS the committed bodies need; its acts are driven by the package's own battery"}
}

func (fixtureBenchmark) RecordVerdict(context.Context, string, string, string) error {
	return notWiredInFixture()
}

func (fixtureBenchmark) Reveal(context.Context, string) (json.RawMessage, error) {
	return nil, notWiredInFixture()
}
func (fixtureBenchmark) Decline(context.Context, string) error { return notWiredInFixture() }
func (fixtureBenchmark) DisposeAlarm(context.Context, string, string, string, string) error {
	return notWiredInFixture()
}
func (fixtureBenchmark) SetOptIn(context.Context, string, string, bool) error {
	return notWiredInFixture()
}

// OptedIn is a READ and therefore answers, unlike the acts above: the committed
// verdict body has to carry a real consent position or the surface that renders
// it would be probed against a shape production never serves. Alice is opted IN
// because she has a sampled pair — a pending pair is only reachable for somebody
// whose standing consent is on (BENCH-REG §4.1/§4.2.1), so the alternative would
// be a world that contradicts itself.
func (fixtureBenchmark) OptedIn(_ context.Context, userID string) (bool, error) {
	return userID == "alice", nil
}

// webAPIFixtures is the covered set: one entry per read a B6-5 view calls.
// The identity is the operator, because the surfaces this packet builds are
// read at the household altitude — a member's narrower answer is the same
// SHAPE, and owner scope has its own three-way tests (reads_test.go).
var webAPIFixtures = []struct{ name, path, who string }{
	{"tasks", "/api/tasks", ""},
	// The S15.11 device register, read BOTH ways: `scope` and the row set are
	// computed per caller, so the operator's household reading and a member's
	// own are two SERVED bodies rather than two renders of one.
	{"push-subscriptions", "/api/push/subscriptions", ""},
	{"push-subscriptions-member", "/api/push/subscriptions", "alice"},
	{"runs", "/api/runs", ""},
	{"meters", "/api/meters", ""},
	// The same read as a MEMBER. The lanes were already owner-scoped, but the
	// pause switch's position is a per-person fact a control acts on, so "a
	// member is offered their own switch and nobody else's" has to be a claim
	// about a served body rather than about a render.
	{"meters-member", "/api/meters", "alice"},
	{"history-views", "/api/events/views", ""},
	{"history-catalog", "/api/events/catalog", ""},
	{"history-view-answer", "/api/events/views/cost_per_run", ""},
	{"history-query-answer", "/api/events/query/status.runs_active", ""},
	// The two S14.10 layers P3-UI-4 gives a surface, both answered by the REAL
	// handlers over this world rather than scripted:
	//
	//   ask    — no local tier is wired here (`history.New` above takes no Duty
	//            and no Advisory), so this is the honest degraded posture the
	//            SPA meets in every test process and in dev: the layer answers
	//            with its disambiguation CARD and its own reason, at 200, with
	//            real catalog choices. $0, deterministic, and the one answer a
	//            client can be built against without a model in the loop.
	//   search — over the corpus `indexFixtureHistory` had the real projector
	//            write, so the rows, refs, kinds, owner column and excerpts are
	//            all the indexer's own (§38 D12: an empty corpus proves nothing).
	{"history-ask-answer", "/api/events/ask?q=" + url.QueryEscape("what did the release notes deployment cost?"), ""},
	//
	// The search question is chosen to reach BOTH indexed kinds this world
	// carries — a recorded verdict and a drift finding, owned by two different
	// people — because a one-row body would let the surface's list render, its
	// per-row ref and its owner column all be right by accident.
	{"history-search-answer", "/api/events/search?q=" +
		url.QueryEscape("did anything ship, and what changed on the anthropic side?"), ""},
	{"task-detail", "/api/tasks/t-ship", ""},
	{"task-detail-draft", "/api/tasks/t-triage", ""},
	{"task-detail-bare", "/api/tasks/t-archive", ""},
	{"task-detail-ops", "/api/tasks/t-ops", ""},
	{"receipt", "/api/runs/r-ship/receipt", ""},
	{"deliverables-in-review", "/api/deliverables?state=in-review", ""},
	{"deliverables-of-task", "/api/deliverables?task=t-ship", ""},
	{"deliverable-detail", "/api/deliverables/d-notes", ""},
	{"run-detail", "/api/runs/r-ship", ""},
	{"approvals", "/api/approvals", ""},
	// The same read as the person whose cards they are: `answerable` is
	// computed PER CALLER (D10), so the "yours to answer" branch only exists in
	// a body read by an owner. It is also the only body that carries the NINTH
	// kind, whose card reaches its addressee and nobody else — not even the
	// operator, deliberately (B6-6 OQ1).
	{"approvals-mine", "/api/approvals", "alice"},
	// The blind-pair form's data, read as the requester: the two renders, the
	// length figures, and the registered answer vocabularies the form's buttons
	// come from (B6-6 OQ4).
	{"benchmark-verdicts", "/api/benchmark/verdicts", "alice"},
	// The S09 memory family, read BOTH ways — and these two are different
	// ANSWERS rather than one body computed per caller, which is the whole point
	// of committing both. `memory` is ALICE's live set: her own two entries,
	// narrowed to `?status=active` because the rows the real gate wrote carry
	// crypto/rand ids no file can hold (see driveFixtureMemoryRetire).
	// `memory-operator` is the OPERATOR's, and it is EMPTY — the content line
	// showing through a served body rather than through a render: the role bit
	// opens house scope and project membership, and neither of those is another
	// person's user-scope store.
	{"memory", "/api/memory?status=active", "alice"},
	{"memory-operator", "/api/memory", "op"},
	// One entry with the S09.7 edge the caller is the ADDRESSEE of, both minted
	// by the real gate: the question text is the detection's own sentence.
	{"memory-entry", "/api/memory/" + fxEntryA, "alice"},
	// The S15.9 settings surface, read BOTH ways: the operator's body carries
	// `editable:true` and every per-user override, a member's carries the served
	// refusal reason and only their own. The whole write surface renders from
	// that flag, so both bodies have to exist for both renders to be driven.
	{"settings", "/api/settings", ""},
	{"settings-member", "/api/settings", "alice"},
	{"settings-history", "/api/settings/freshness.max_age/history", ""},
	{"prices", "/api/settings/prices", ""},
	// The price table's `editable` is computed per caller too, so the read-only
	// posture is a served body rather than a second render.
	{"prices-member", "/api/settings/prices", "alice"},
	// The S15.7 assistant (B6-7). Chat is OWNER-ONLY, so every one of these is
	// read as alice: there is no operator body to commit, and that absence is
	// itself the disposition (the operator does not read a member's
	// transcripts). The session detail carries the whole render — the
	// transcript, the settled turns with their answers, the disambiguation
	// card, the born task's OPEN intake card, and the RUNNING turn the widget
	// re-attaches to after a navigation.
	{"chat-sessions", "/api/chat/sessions", "alice"},
	{"chat-session", "/api/chat/sessions/cs-0000000000000001", "alice"},
	// The SECOND session, empty and untitled. It is not a spare: one turn at a
	// time is a per-session rule, so the rich body above — which deliberately
	// holds a RUNNING turn — is a conversation the composer correctly refuses to
	// send a second turn into. The empty body is where a driven send belongs, and
	// it is also the committed ground for the nothing-said-yet render.
	{"chat-session-empty", "/api/chat/sessions/cs-0000000000000014", "alice"},
	{"chat-files", "/api/chat/files", "alice"},
	// The D4(b) unlock: the recorded suite results, through the LANDED audited
	// query route.
	{"eval-scores", "/api/events/query/verdicts.eval_scores", ""},
	// The S15.8 review surface (B6-8), read as the OWNER — which is who reviews.
	// A non-owner's render is presentation over the same body (the accept form is
	// the owner's; the card is readable by the operator and acceptable by nobody
	// else), so it needs no second body: the two renders come from two SESSIONS
	// over one served detail.
	{"deliverable-review", "/api/deliverables/d-site", "alice"},
	// The default compare read takes NO parameters: round-over-round IS the
	// server's default (new = current, old = new−1), and the committed body is
	// what proves the surface is not sending a pair it made up.
	{"compare-line-diff", "/api/deliverables/d-site/compare", "alice"},
	// old=0 is the PRE-TASK BASE (S13.1) — the one navigation target that is not a
	// revision, and the reason the revision picker offers a zero at all.
	{"compare-base", "/api/deliverables/d-site/compare?old=0&new=2", "alice"},
	// The three non-diff surfaces, each a defined answer for its type: per-side
	// object refs and the by-hash verdict for images and binaries, and the PDF's
	// extraction-failure DEGRADE with its reason on the label.
	{"compare-image-pair", "/api/deliverables/d-hero/compare", "alice"},
	{"compare-binary-cards", "/api/deliverables/d-bundle/compare", "alice"},
	{"compare-pdf-degrade", "/api/deliverables/d-brief/compare", "alice"},
	// The honest FALLBACK diff: a real unified diff under a label that says it is
	// not a rich surface for this type.
	{"compare-extracted-text", "/api/deliverables/d-notebook/compare", "alice"},
	// The SECOND deliverable detail, and it exists for one door: this one's task is
	// parked on a rework card, so `request-revision` is LIVE and carries the ask,
	// the answer verb and the card's own pin. The body above shows the closed limb;
	// no single deliverable can serve both, because the door's state IS the
	// deliverable's state.
	{"deliverable-rework", "/api/deliverables/d-brief", "alice"},
	// Every comment of the deliverable with where each one anchors in the CURRENT
	// revision — all five placement statuses, an open set beside a consumed one,
	// and a verification finding under the same schema as the human comments.
	{"placed-comments", "/api/deliverables/d-site/comments", "alice"},
	// The High-tier decision data, shown BEFORE the act (S13.6 step 3): the pin,
	// the protected ref, the trailers byte-for-byte with their provenance sources,
	// the secret-free signing posture and the tier statement.
	{"accept-card", "/api/deliverables/d-site/accept-card", "alice"},
	// The owner's live preview sessions. Empty is the truthful answer in a world
	// that launches nothing, and "no session is running" is a render of its own.
	{"previews", "/api/previews", "alice"},
	// The S15.10 workforce map (B6-8 part B), read BOTH ways — and unlike the
	// review surface, the two bodies are genuinely different ANSWERS rather than
	// two renders of one:
	//
	//   - the operator's carries the whole registry, including another member's
	//     PERSONAL automation, and the outcome figures of every owner's runs;
	//   - alice's carries her own personal workers plus the HOUSEHOLD roster, and
	//     the outcome figures of her own runs only — bob's personal automation is
	//     absent from it entirely, which is the limb that leaks if it is wrong.
	//
	// Both have to be committed, because "the member sees less" is a claim about
	// a served body and not about a render.
	{"workforce", "/api/workforce", ""},
	{"workforce-member", "/api/workforce", "alice"},
	// The S00.9 A13 lane-pin set (P3-LN-10a): the lanes a task-creation pin may
	// name, each with the platform's own verdict. Read at the household
	// altitude because the set is PROCESS-WIDE by construction — coverage is the
	// union across the people who placed a credential, and selection re-checks.
	{"intake-pinnable-lanes", "/api/intake/pinnable-lanes", ""},
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
			who := fx.who
			if who == "" {
				who = "op"
			}
			rr := httptest.NewRecorder()
			fixtureServer(t, b, who).Handler().ServeHTTP(rr, httptest.NewRequest("GET", fx.path, nil))
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
		who := fx.who
		if who == "" {
			who = "op"
		}
		first := canonicalJSON(t, fixtureBody(t, fixtureWorld(t), who, fx.path))
		second := canonicalJSON(t, fixtureBody(t, fixtureWorld(t), who, fx.path))
		if string(first) != string(second) {
			t.Errorf("%s is not byte-stable across two identical seedings — it carries a live reading, "+
				"so it cannot be a committed fixture:\n%s\n%s", fx.path, first, second)
		}
	}
}

// TestFixtureBornTaskStateIsOneTheIntakePipelineProduces guards the state
// seedFixtureChatBornTask puts the born task in — against the rule the pipeline
// ENFORCES, not against a value somebody remembered.
//
// The B6-5 root cause was a fixture world that cannot exist, and the shape it
// takes here is a run state. `issueCard` inserts the ask row and parks the run
// in ONE transaction (what the check below can actually see is that ONE FUNCTION
// does both — the transaction boundary is read by a person, the coupling is read
// by the test), so a task whose intake card is open and whose run reads
// `running` is a world production can never reach — and it is not a harmless
// wrongness: `waiting_on_human` is derived as parked AND an open ask
// (projection.go), so the impossible row would sit in mission control's running
// bucket and answer the `running` filter while the task actually waits on a
// person.
func TestFixtureBornTaskStateIsOneTheIntakePipelineProduces(t *testing.T) {
	// The rule, read out of the pipeline's own source. If intake ever stops
	// parking on a gate, this fails HERE — naming the fixture that would have
	// gone quietly wrong — rather than leaving this file's comment to be trusted.
	src, err := os.ReadFile("../intake/pipeline.go")
	if err != nil {
		t.Fatalf("read the intake pipeline: %v", err)
	}
	const marker = "func (p *Pipeline) issueCard("
	from := strings.Index(string(src), marker)
	if from < 0 {
		t.Fatalf("%s is gone from internal/intake — the rule this test reads has moved", marker)
	}
	body := string(src)[from:]
	if end := strings.Index(body[len(marker):], "\nfunc "); end >= 0 {
		body = body[:len(marker)+end]
	}
	if !strings.Contains(body, "insertAskTx") || !strings.Contains(body, "run.StateParked") {
		t.Fatalf("issueCard no longer inserts the ask AND parks the run in one place — re-read S06.1 before trusting the fixture:\n%s", body)
	}

	b := fixtureWorld(t)
	var runID, state string
	if err := b.db.QueryRowContext(context.Background(),
		`SELECT r.run_id, r.state FROM asks a JOIN runs r ON r.run_id = a.run_id
		  WHERE a.ask_id = ? AND a.answered_ts IS NULL`, "ask-chatborn-1").Scan(&runID, &state); err != nil {
		t.Fatalf("read the born card's run: %v", err)
	}
	if state != "parked" {
		t.Errorf("%s holds an OPEN intake card in state %q — the pipeline parks a run when it issues a card, so this world cannot exist",
			runID, state)
	}
	// And the handoff view the chat feed renders says the same thing, so the
	// transcript and the world do not contradict each other.
	handoff, err := fixtureIntake{t: t}.Submit(context.Background(), "alice", json.RawMessage(`{"title":"Draft the release notes"}`))
	if err != nil {
		t.Fatalf("fixture Submit: %v", err)
	}
	var view struct {
		Runs []struct {
			RunID string `json:"run_id"`
			State string `json:"state"`
		} `json:"runs"`
	}
	decodeInto(t, string(handoff), &view)
	for _, r := range view.Runs {
		if r.RunID == runID && r.State != state {
			t.Errorf("the handoff view calls %s %q while the world stores %q", runID, r.State, state)
		}
	}
}

// TestChatTranscriptNeverAnswersWithARowItHasNotYetCreated pins the ORDER the
// fixture world is seeded in, from the outside.
//
// A transcript reads forward. Turn 1 is a Layer-0 `cost_per_run` answer over
// every run in the world and turn 3 is the handoff that gives birth to
// `t-chatborn`, so seeding the born rows before the chat drive made the
// conversation's FIRST answer report the run of a task the conversation had not
// created — a causality inversion in the flagship committed transcript. The
// answers are real reads, so the only fix is the seeding order, and this is what
// keeps it.
func TestChatTranscriptNeverAnswersWithARowItHasNotYetCreated(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(fixtureDir, "chat-session.json"))
	if err != nil {
		t.Fatalf("read the committed session body: %v", err)
	}
	var body struct {
		Turns []chat.Turn `json:"turns"`
	}
	decodeInto(t, string(raw), &body)
	born := false
	for i, turn := range body.Turns {
		// The handoff turn is where the birth belongs: its own outcome IS the born
		// task. Every turn before it must not know the name.
		if turn.Kind == chat.KindTask {
			born = true
		}
		if !born && strings.Contains(string(turn.Outcome), "t-chatborn") {
			t.Errorf("turn %d (%s, kind %q) already names t-chatborn, and no earlier turn gave birth to it",
				i+1, turn.ID, turn.Kind)
		}
	}
	if !born {
		t.Error("the committed transcript has no handoff turn — this test would then be vacuous")
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

func fixtureBody(t *testing.T, b *backend, who, path string) []byte {
	t.Helper()
	rr := httptest.NewRecorder()
	fixtureServer(t, b, who).Handler().ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
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

// ── the S15.10 workforce map (B6-8 part B) ──────────────────────────────────

// The template documents the map renders. They are real files that go in
// through CreateDraft → RunBattery → Approve, because the roster's equipment
// block is PARSED from the hash-verified file on every read (S08.3): a
// hand-written row would point at a file nothing wrote and the tamper check
// would refuse it, so a hand-written roster fixture cannot exist at all.
//
// The v2 body differs from v1 in the PROMPT BODY only, which is what makes v1 a
// superseded version with its own history rather than a duplicate.
const (
	fxWorkerAgenticV1 = `---
name: release-notes-writer
description: Writes and revises the household release notes from the merged changelog
kind: agentic
domain: software
selectors:
  family: read-analyze
  task_classes: [review, summarize]
  triggers: [write the release notes, summarize the changelog]
profile:
  duty: execution
  effort_floor: standard
equipment:
  tools: [Read, Grep, Glob]
  skills: [release-notes-house-style]
  knowledge: [release-notes/conventions]
eval:
  golden_set_ref: golden/release-notes
  planted_defect_ref: planted/release-notes
persona: [Terse and concrete.]
---
Read the changelog since the last release. Write one line per merged change,
in plain language, and cite the change it came from. Escalate when a change
has no description to read.
`
	fxWorkerAgenticV2 = `---
name: release-notes-writer
description: Writes and revises the household release notes from the merged changelog
kind: agentic
domain: software
selectors:
  family: read-analyze
  task_classes: [review, summarize]
  triggers: [write the release notes, summarize the changelog]
profile:
  duty: execution
  effort_floor: standard
equipment:
  tools: [Read, Grep, Glob]
  skills: [release-notes-house-style]
  knowledge: [release-notes/conventions]
eval:
  golden_set_ref: golden/release-notes
  planted_defect_ref: planted/release-notes
persona: [Terse and concrete.]
---
Read the changelog since the last release. Write one line per merged change,
in plain language, and cite the change it came from. Group the lines by the
area of the system they touch. Escalate when a change has no description to
read.
`
	// The automation body is a dialect document (S08.9): a read step feeding an
	// OUTWARD step that carries its explicit approval node. The marked node is
	// what R13 renders as the D7 fact it is, so the chain has to contain a real
	// one — and an outward step's approval marking is only honest when the verb
	// really is outward.
	fxWorkerAutomation = `---
name: calendar-digest
description: Posts a daily digest of the household calendar to the notes channel
kind: automation
domain: chore
selectors:
  family: connector-automation
equipment:
  connectors: [calendar]
---
{"dialect":"sinet-automation/1","service":"calendar","steps":[
  {"id":"fetch","verb":"calendar.list","args":{"day":{"$from":"payload.day"}}},
  {"id":"post","verb":"calendar.post","args":{"digest":{"$from":"steps.fetch.summary"}},"approval":true}
]}
`
	// The DRAFT: a real template document whose battery has never run, so it has
	// no active version, no granted enforcement state and no validation record.
	// That is the honest state of a composed-but-unapproved worker, and it is a
	// render the map owes (an empty roster is not the only truthful absence).
	fxWorkerDraft = `---
name: spend-auditor
description: Reads the weekly receipts and reports where the household spend moved
kind: agentic
domain: software
selectors:
  family: read-analyze
  task_classes: [review]
  triggers: [audit the spend]
profile:
  duty: execution
  effort_floor: quick
equipment:
  tools: [Read, Grep]
---
Read the receipts for the period. Report which lanes moved and by how much,
citing the receipt each figure came from.
`
)

// The workforce fixture's stable ids. Template and version ids are MINTED by
// the store, so the world pins them through the NewID seam rather than
// rewriting six tables and a file path afterwards — the same concession the
// journal clock and review clock already make, and for the same reason.
const (
	fxWorkerNotes     = "wt-notes"
	fxWorkerDigest    = "wt-digest"
	fxWorkerAudit     = "wt-audit"
	fxWorkerNotesV1   = "wtv-notes-1"
	fxWorkerNotesV2   = "wtv-notes-2"
	fxWorkerDigestV1  = "wtv-digest-1"
	fxWorkerAuditV1   = "wtv-audit-1"
	fxWorkerDigestEff = "e-digest"
)

// fixtureWorkforce composes the S08 store ONCE per world. Like the review
// store it has to be shared: its Root is where the template FILES live, and a
// per-server root would put a version's definition where the next server
// cannot hash-verify it.
func fixtureWorkforce(t *testing.T, b *backend) *worker.Store {
	t.Helper()
	if b.work == nil {
		ids := fixtureIDs(t, fxWorkerNotes, fxWorkerNotesV1, fxWorkerNotesV2,
			fxWorkerDigest, fxWorkerDigestV1, fxWorkerAudit, fxWorkerAuditV1)
		st, err := worker.NewStore(worker.Config{
			DB: b.db, Log: b.log, Settings: b.reg, Root: fixtureRoot(t, b, "workers"),
			Now: func() time.Time { return mustTime(t, fxT4) }, NewID: ids,
		})
		if err != nil {
			t.Fatalf("worker.NewStore: %v", err)
		}
		b.work = st
	}
	return b.work
}

// fixtureIDs hands out the pinned ids in mint order. It FAILS rather than
// falling back to a random id when the world mints more than it was given: a
// silent fallback would make one added version quietly un-commitable, which is
// the failure this seam exists to prevent.
func fixtureIDs(t *testing.T, want ...string) func(string) string {
	t.Helper()
	next := 0
	return func(prefix string) string {
		if next >= len(want) {
			t.Fatalf("the workforce fixture minted more ids than it pinned (prefix %q, %d pinned)", prefix, len(want))
			return ""
		}
		id := want[next]
		next++
		if !strings.HasPrefix(id, prefix+"-") {
			t.Fatalf("pinned id %q does not carry the %q prefix the store asked for", id, prefix)
		}
		return id
	}
}

// fxCalendarVerbs is the connector verb registry the automation's station 3
// runs against: one read verb that executes, one OUTWARD verb that journals a
// gated proposal and never calls anything (S08.9; D7 makes this free).
func fxCalendarVerbs() automation.VerbMap {
	return automation.VerbMap{
		"calendar.list": {Fn: func(_ context.Context, args map[string]json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"summary":"3 events on ` + strings.Trim(string(args["day"]), `"`) + `"}`), nil
		}},
		"calendar.post": {Outward: true, Class: gates.ClassC},
	}
}

// seedFixtureWorkforce builds the roster the S15.10 map renders, entirely
// through S08's own verbs.
//
//	wt-notes   agentic, software (FULL), promoted to HOUSEHOLD by the operator,
//	           two versions: v1 superseded, v2 active. Alice's, and therefore
//	           the one a member reads through the household limb.
//	wt-digest  automation, chore (DEGRADED — a new domain is born degraded),
//	           BOB's and PERSONAL. It renders the multi-stage chain with its
//	           marked approval node, the degraded-domain marking, and — because
//	           it is another member's personal worker — it is what must NOT
//	           appear in Alice's roster.
//	wt-audit   agentic, alice's, PERSONAL, DRAFT: the battery never ran, so it
//	           has no active version, no granted guardrails and no validation.
//
// Then the version→outcome join gets something real to join over: four routing
// decisions naming a worker VERSION, across two owners and two versions, plus
// one recorded verdict. That is what makes both arms of the join driven — a
// version WITH outcomes beside a version with none — and what makes the
// member/operator outcome scopes differ observably rather than by assertion.
func seedFixtureWorkforce(t *testing.T, b *backend) {
	t.Helper()
	ctx := context.Background()
	st := fixtureWorkforce(t, b)

	// The skill the agentic definition REFERENCES. Station 1 resolves skill refs
	// against the store's own skill root, so an unresolved ref is a lint error
	// and the battery would be red: installing it for real is what makes the
	// equipment block resolvable rather than aspirational.
	if _, err := st.InstallSkill(ctx, "alice", "release-notes-house-style", map[string][]byte{
		"SKILL.md": []byte("---\nname: release-notes-house-style\n" +
			"description: How this household writes release notes\n---\n" +
			"One line per change. Plain language. Cite the change.\n"),
	}); err != nil {
		t.Fatalf("InstallSkill: %v", err)
	}

	// `chore` does not exist in the 0005 day-one rows, and CreateDomain makes it
	// DEGRADED by construction (S08.7 maturity honesty) — which is exactly the
	// structural fact the automation's card has to render.
	if err := st.CreateDomain(ctx, "op", "chore"); err != nil {
		t.Fatalf("CreateDomain(chore): %v", err)
	}

	// wt-notes v1 → validated → approved → active.
	if _, _, err := st.CreateDraft(ctx, "alice", fxWorkerAgenticV1,
		worker.RequestedGrants{Tools: []string{"Read", "Grep", "Glob"}, Class: "C1", Egress: worker.EgressNone},
		worker.Provenance{AuthorKind: "human", Origin: worker.OriginHumanWritten,
			EvidenceRef: "gap:release-notes/2026-07"}); err != nil {
		t.Fatalf("CreateDraft(notes): %v", err)
	}
	fxApproveWorker(t, st, "alice", fxWorkerNotesV1)

	// v2: every edit is a new immutable version row (S08.4). The body moved, so
	// first-N resets — the supervised counter the map renders is the real one.
	// v2 REQUESTS ceilings and v1 does not, so both arms of the budget render are
	// in the committed bodies and neither is a synthesized body: `Approve` copies
	// requested → granted verbatim (S08.2), so a version that asked for nothing
	// is granted 0 — which is the absence, not a ceiling of zero.
	if _, err := st.NewVersion(ctx, "alice", fxWorkerNotes, fxWorkerAgenticV2,
		worker.RequestedGrants{Tools: []string{"Read", "Grep", "Glob"}, Class: "C1", Egress: worker.EgressNone,
			BudgetUSD: 12.5, BudgetSteps: 400},
		worker.Provenance{AuthorKind: "human", Origin: worker.OriginHumanWritten,
			EvidenceRef: "review:release-notes/round-2"}); err != nil {
		t.Fatalf("NewVersion(notes v2): %v", err)
	}
	fxApproveWorker(t, st, "alice", fxWorkerNotesV2)

	// Promotion to household-shared is an OPERATOR approval (D10, S08.4), and it
	// is what puts this worker in a member's roster through the shared limb
	// rather than through ownership.
	if err := st.Promote(ctx, "op", fxWorkerNotes); err != nil {
		t.Fatalf("Promote(notes): %v", err)
	}

	// wt-digest: bob's personal automation. Station 3 is a sample-payload
	// execution whose outward step journals a gated proposal — the world's own
	// consequence of validating an automation, so the proposal is left standing
	// and its id pinned (the e-publish precedent: the journal mints a UUID no
	// committed file can carry, while the payload and its journal-computed hash
	// stay the producer's own).
	// ScheduleAttachable is REQUESTED here, and Approve copies requested →
	// granted unclamped (lifecycle.go), so the guardrails row says `true` while
	// the worker's domain is degraded — and S08.7's second consequence is that a
	// degraded-domain worker "cannot attach to any schedule whose results
	// auto-accept". The fixture reaches that contradiction on purpose: a card
	// that renders "attachable: yes" without stating the bar is the render this
	// surface must not produce, and with all three workers at `false` it was
	// undriveable.
	if _, _, err := st.CreateDraft(ctx, "bob", fxWorkerAutomation,
		worker.RequestedGrants{Tools: []string{"calendar.list", "calendar.post"}, Class: "C0",
			Egress: worker.EgressSingleHost, EgressHosts: []string{"calendar.example.com"},
			ScheduleAttachable: true},
		worker.Provenance{AuthorKind: "human", Origin: worker.OriginHumanWritten}); err != nil {
		t.Fatalf("CreateDraft(digest): %v", err)
	}
	res, err := st.RunBattery(ctx, fxWorkerDigestV1, worker.BatteryInput{
		Actor: "bob", SampleTask: `{"day":"2026-07-20"}`,
		Verbs: fxCalendarVerbs(), Journal: fixtureJournal(t, b),
	})
	if err != nil {
		t.Fatalf("RunBattery(digest): %v", err)
	}
	if !res.Green {
		t.Fatalf("digest battery red: lint=%+v audit=%+v", res.Lint, res.Audit)
	}
	fxPinDryRunProposal(t, b, fxWorkerDigestEff)
	if _, err := st.Approve(ctx, "bob", fxWorkerDigestV1, worker.ApproveOpts{}); err != nil {
		t.Fatalf("Approve(digest): %v", err)
	}

	// wt-audit: the draft nobody validated.
	if _, _, err := st.CreateDraft(ctx, "alice", fxWorkerDraft,
		worker.RequestedGrants{Tools: []string{"Read", "Grep"}, Class: "C1", Egress: worker.EgressNone},
		worker.Provenance{AuthorKind: "composer", Composer: "claude/2026-07",
			PlaybookVer: "composer-playbook/1", Origin: worker.OriginComposed,
			EvidenceRef: "gap:spend-audit/2026-07"}); err != nil {
		t.Fatalf("CreateDraft(audit): %v", err)
	}

	// The S08.4 version→outcome join's own ground. Every row names a worker AND
	// a version, which is what makes it joinable at all; the runs are ones the
	// world already has, so no task or run row is added.
	for _, e := range []struct{ run, owner, version, cause, effort, reason, ts string }{
		{"r-notes", "alice", fxWorkerNotesV2, "selector-match", "standard",
			"the release-notes writer matched on both signals", fxT4},
		{"r-claim", "alice", fxWorkerNotesV2, "selector-match", "quick",
			"the release-notes writer matched the index rebuild's summarize class", fxT4},
		{"r-stall", "bob", fxWorkerNotesV2, "override", "standard",
			"bob re-routed the re-index to the release-notes writer", fxT4},
		{"r-audit", "bob", fxWorkerNotesV1, "selector-match", "standard",
			"the release-notes writer v1 matched the price-table audit", fxT2},
	} {
		exec(t, b, `INSERT INTO run_events (run_id, generation, user_id, type, schema_version, payload, ts)
		            VALUES (?,0,?,?,1,?,?)`, e.run, e.owner, "routing.decided",
			`{"cause":"`+e.cause+`","score":0.88,"worker":"`+fxWorkerNotes+`","worker_name":"release-notes-writer",`+
				`"version":"`+e.version+`","model":"claude","lane":"anthropic","effort":"`+e.effort+`",`+
				`"plain_reason":"`+e.reason+`","window_tokens":200000}`, e.ts)
	}
	// ONE recorded verdict, so a routed run WITH a verdict renders beside routed
	// runs without one. Both arms have to be in the committed body: "no verdict
	// yet" and "SHIP" are different facts and neither may be inferred.
	exec(t, b, `INSERT INTO run_events (run_id, generation, user_id, type, schema_version, payload, ts)
	            VALUES (?,0,?,?,1,?,?)`, "r-notes", "alice", "verdict.recorded",
		`{"round":1,"verdict":"SHIP","ac_ids":["AC-1","AC-2"],"passed":["AC-1","AC-2"],`+
			`"domain":"software","revision":2,"content_sha256":"sha-d-notes-2","retention":"keep-forever",`+
			`"golden_set":{}}`, fxT4)
}

// fxApproveWorker drives one version through station 3 and station 4 for real:
// a green battery is what Approve requires, and Approve is the ONLY writer of
// the guardrails row the map renders as the permissions block (S08.2).
func fxApproveWorker(t *testing.T, st *worker.Store, actor, versionID string) {
	t.Helper()
	res, err := st.RunBattery(context.Background(), versionID, worker.BatteryInput{
		Actor: actor, SampleTask: "write the release notes for the current cycle",
		Engine: fxDryEngine{}, Model: "claude-haiku-4-5", EnginePin: "claude-cli@2.1.215",
	})
	if err != nil {
		t.Fatalf("RunBattery(%s): %v", versionID, err)
	}
	if !res.Green {
		t.Fatalf("battery red for %s: lint=%+v audit=%+v dry=%+v", versionID, res.Lint, res.Audit, res.DryRun)
	}
	if _, err := st.Approve(context.Background(), actor, versionID, worker.ApproveOpts{}); err != nil {
		t.Fatalf("Approve(%s): %v", versionID, err)
	}
}

// fxDryEngine is station 3's sandboxed dry run. NO engine is spawned and no
// paid call is made — the established fake-engine posture.
type fxDryEngine struct{}

func (fxDryEngine) DryRun(context.Context, worker.DryRunRequest) (worker.DryRunResult, error) {
	return worker.DryRunResult{Completed: true, TranscriptRef: "fixture://dry-run/release-notes",
		Output: "one line per merged change, grouped by area", CostUSD: 0.01}, nil
}

// fxPinDryRunProposal pins the id of the effect the automation's station 3
// journaled. The journal mints a UUID, which no committed body can carry; the
// PAYLOAD and its journal-computed hash — the things the approval verb checks —
// stay the producer's own.
//
// It selects by the proposal's own IDENTITY (service + step + verb, which the
// dialect makes unique within a workflow) rather than by insertion order, so it
// pins the row it means the same way `e-publish` does. `ORDER BY rowid DESC`
// would fail safely today and silently pin the wrong effect the moment a second
// automation joins this world.
func fxPinDryRunProposal(t *testing.T, b *backend, id string) {
	t.Helper()
	var minted string
	if err := b.db.QueryRowContext(context.Background(),
		`SELECT effect_id FROM effects
		  WHERE json_extract(payload, '$.kind') = ?
		    AND json_extract(payload, '$.service') = ?
		    AND json_extract(payload, '$.step') = ?
		    AND json_extract(payload, '$.verb') = ?`,
		"automation-step", "calendar", "post", "calendar.post").Scan(&minted); err != nil {
		t.Fatalf("read the automation dry-run proposal by identity: %v", err)
	}
	exec(t, b, `UPDATE effects SET effect_id = ?, created_ts = ?, updated_ts = ? WHERE effect_id = ?`,
		id, fxT4, fxT4, minted)
}
