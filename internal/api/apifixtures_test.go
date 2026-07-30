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
	"fmt"
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
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/chat"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/history"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/memory"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
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
	// The S09 store is composed ONCE, here, so every fixtureServer over this
	// backend shares one knowledge root (see backend.mem).
	store, err := memory.NewStore(b.db, b.log, b.reg, filepath.Join(t.TempDir(), "knowledge"))
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
		{"r-ops", "op", "t-ops", "queued", "anthropic", fxT4},
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
	return b
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
		DB: b.db, Log: b.log, Root: filepath.Join(t.TempDir(), "exchange"),
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
		var title, content string
		if err := b.db.QueryRowContext(ctx,
			`SELECT title, coalesce(content,'') FROM knowledge_entries WHERE entry_id = ?`, real).
			Scan(&title, &content); err != nil {
			t.Fatalf("read seeded entry %s: %v", real, err)
		}
		exec(t, b, `INSERT INTO knowledge_entries
		    (entry_id, user_id, scope, layer, kind, title, content, topic_key, status, version, origin, created_ts, updated_ts)
		    VALUES (?,?,'user','L2','lesson',?,?,'release-notes-style','active',1,'human_direct',?,?)`,
			id, "alice", title, content, fxT2, fxT2)
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
				Steps: []intake.Step{
					{ID: "S-1", Title: "Collect the merged changes", DoneWhen: "every merge since the last tag is listed"},
					{ID: "S-2", Title: "Write one line per change", DoneWhen: "each listed change has a plain sentence"},
				},
				Coverage: map[string][]string{"AC-1": {"S-1"}, "AC-2": {"S-2"}},
				Estimate: intake.Estimate{SizeClass: "small", USD: 0.12, Known: true, Basis: "median of the last five notes runs"},
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
				ACs:      []intake.AC{{N: 1, Plain: "The note names every task finished this week."}},
				Steps:    []intake.Step{{ID: "S-1", Title: "Write the note", DoneWhen: "the note exists in the household folder"}},
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

func fixtureReview(t *testing.T, b *backend) *review.Store {
	t.Helper()
	return &review.Store{
		DB: b.db, Log: b.log, Settings: b.reg, Root: t.TempDir(),
		Now: func() time.Time { return mustTime(t, fxT4) },
	}
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
		// The scheduler's STORAGE seam is the landed double: constructing a real
		// scheduler is out of proportion here, and what this fixture is proving
		// is the decision ROW the verb mints, which recordDecision writes
		// itself. The verb, its D10 authorization and its row are all real; the
		// queue row below carries the rank the real scheduler would persist.
		Hints:      newFakeHints("r-ops"),
		Review:     fixtureReview(t, b),
		Effects:    fixtureJournal(t, b),
		Memory:     b.mem,
		MemoryGate: b.memGate,
		Registry:   b.reg,
		Prices:     fixturePrices{},
		Benchmark:  fixtureBenchmark{},
		Chat:       b.chat,
		Now:        func() time.Time { return mustTime(t, fxT4) },
	})
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

// webAPIFixtures is the covered set: one entry per read a B6-5 view calls.
// The identity is the operator, because the surfaces this packet builds are
// read at the household altitude — a member's narrower answer is the same
// SHAPE, and owner scope has its own three-way tests (reads_test.go).
var webAPIFixtures = []struct{ name, path, who string }{
	{"tasks", "/api/tasks", ""},
	{"runs", "/api/runs", ""},
	{"meters", "/api/meters", ""},
	{"history-views", "/api/events/views", ""},
	{"history-catalog", "/api/events/catalog", ""},
	{"history-view-answer", "/api/events/views/cost_per_run", ""},
	{"history-query-answer", "/api/events/query/status.runs_active", ""},
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
