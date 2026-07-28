package api_test

// approvals_test.go — the S15.6 decision plane (B6-2A): the one risk-ranked
// inbox, the hash-pinned answer verbs, the tier mechanics (batch / individual /
// step-up / co-approval), the OQ2 deny disposition, and the audit obligation.
//
// The two probes that carry the retry-safety claim are adversarial by
// construction: the stale-hash test asserts NOTHING FIRED by re-reading the
// subject afterwards, and the double-submit test drives the SAME request twice
// against machinery that would visibly double-apply if the guard were removed.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
)

// ── fixtures ────────────────────────────────────────────────────────────────

// keySettings serves a different duration per ⚙ key, so the expiry horizon and
// the staleness threshold cannot be confused for one another by accident.
type keySettings map[string]time.Duration

func (k keySettings) Duration(key string) (time.Duration, error) {
	d, ok := k[key]
	if !ok {
		return 0, fmt.Errorf("no such ⚙ key %q", key)
	}
	return d, nil
}

func approvalSettings() keySettings {
	return keySettings{
		"effects.approval_expiry": 7 * 24 * time.Hour,
		"freshness.max_age":       time.Hour,
	}
}

// closingSurface stands in for the landed pipeline surface on the answer path.
// It does what the pipeline does to an answered card and nothing else: the ask
// row CLOSES and the pipeline's own canonical audit row (intake.state) lands —
// which is exactly why the approvals verb mints no decision.recorded for an ask
// (OQ8: an act with a landed canonical row is never double-minted).
type closingSurface struct {
	fakeSurface
	b      *backend
	calls  int
	failed error
}

func (c *closingSurface) Answer(ctx context.Context, userID, askID string, answer json.RawMessage, pinVerified bool) (json.RawMessage, error) {
	c.calls++
	c.lastUser, c.lastAsk, c.lastAnswer, c.lastPinOK = userID, askID, answer, pinVerified
	if c.failed != nil {
		return nil, c.failed
	}
	var owner string
	if err := c.b.db.QueryRowContext(ctx, `SELECT user_id FROM asks WHERE ask_id = ?`, askID).Scan(&owner); err != nil {
		return nil, err
	}
	if err := c.b.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE asks SET status='answered', answered_ts=?, answer=? WHERE ask_id=?`,
			nowTS(), string(answer), askID)
		return err
	}); err != nil {
		return nil, err
	}
	if _, err := c.b.log.Append(ctx, eventlog.Append{
		UserID: owner, Type: "intake.state", SchemaVersion: 1,
		Payload: json.RawMessage(`{"phase":"approved","ask":"` + askID + `"}`),
	}); err != nil {
		return nil, err
	}
	return json.RawMessage(`{"ok":true,"ask":"` + askID + `"}`), nil
}

// decisionEnv is the standing decision-plane fixture: a real migrated DB, a
// real auth store, a real S02.7 journal, and the answer surface above.
type decisionEnv struct {
	b       *backend
	surface *closingSurface
	journal *gates.Journal
	cancel  *fakeCancel
}

// fixturePIN is the PIN every fixture identity carries, so the High-tier
// step-up runs against the REAL auth store rather than a stub of it.
const fixturePIN = "hunter2hunter"

func newDecisionEnv(t *testing.T) *decisionEnv {
	t.Helper()
	b := newBackend(t)
	j, err := gates.NewJournal(gates.JournalConfig{DB: b.db, Settings: settings.New()})
	if err != nil {
		t.Fatalf("journal: %v", err)
	}
	ctx := context.Background()
	// The bootstrap window: the first user must be the operator (S01.9), and
	// every later identity is created BY the operator — which is also what
	// gives each one a real PIN for the step-up path.
	if err := b.store.CreateUser(ctx, "", auth.User{ID: "op", DisplayName: "Op", Role: auth.RoleOperator}, fixturePIN); err != nil {
		t.Fatalf("bootstrap operator: %v", err)
	}
	for _, u := range []auth.User{
		{ID: "alice", DisplayName: "Alice", Role: auth.RoleMember},
		{ID: "bob", DisplayName: "Bob", Role: auth.RoleMember},
		{ID: "carol", DisplayName: "Carol", Role: auth.RoleMember},
		{ID: "opal", DisplayName: "Opal", Role: auth.RoleOperator},
	} {
		if err := b.store.CreateUser(ctx, "op", u, fixturePIN); err != nil {
			t.Fatalf("create %s: %v", u.ID, err)
		}
	}
	return &decisionEnv{b: b, surface: &closingSurface{b: b}, journal: j, cancel: &fakeCancel{}}
}

func (e *decisionEnv) server(t *testing.T, who string) *api.Server {
	t.Helper()
	return api.New(api.Config{
		Log: e.b.log, Sessions: e.b.store, Auth: fixedIdentity{who},
		Settings: approvalSettings(),
		HealthFn: func() api.Health { return api.Health{Ready: true} },
		DB:       e.b.db, Meter: fakeMeter{},
		Intake: e.surface, Effects: e.journal, Cancel: e.cancel,
	})
}

// do issues an authenticated request and returns status + body.
func (e *decisionEnv) do(t *testing.T, who, method, path, body string) (int, string) {
	t.Helper()
	rr := httptest.NewRecorder()
	e.server(t, who).Handler().ServeHTTP(rr, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rr.Code, rr.Body.String()
}

func (e *decisionEnv) mustDo(t *testing.T, who, method, path, body string) string {
	t.Helper()
	code, out := e.do(t, who, method, path, body)
	if code != http.StatusOK {
		t.Fatalf("%s %s as %s: status %d: %s", method, path, who, code, out)
	}
	return out
}

// approvalCard is an ask card snapshot in the S06.9 shape: the tier the inbox
// ranks and gates on, the action vocabulary, and the 13.5 help block.
func approvalCard(tier, restatement string) string {
	return `{"kind":"approval","tier":"` + tier + `","task_id":"t-1","version":1,
	  "approval":{"layer1":{"restatement":"` + restatement + `",
	    "help":{"what":"Approving starts the work exactly as planned.",
	            "wrong":"If an assumption is wrong the result misses your intent.",
	            "recommend":"Read the assumptions; contest via Re-plan."}},
	    "layer2":{"acs":[]},
	    "actions":["approve","replan","reinterview","cancel"]}}`
}

func seedCard(t *testing.T, b *backend, askID, runID, owner, snapshot string, observed time.Time) {
	t.Helper()
	exec(t, b, `INSERT INTO asks (ask_id, run_id, user_id, snapshot, status, observed_ts) VALUES (?,?,?,?,?,?)`,
		askID, runID, owner, snapshot, "open", observed.UTC().Format(time.RFC3339Nano))
}

// seedProposal writes a proposed effect through the REAL journal, so its
// payload_hash is the journal's own pin and not a test-authored string.
func seedProposal(t *testing.T, e *decisionEnv, runID, owner, payload string) string {
	t.Helper()
	eff, err := e.journal.Propose(context.Background(), gates.Proposal{
		RunID: runID, UserID: owner, Class: gates.ClassA, Payload: json.RawMessage(payload),
	})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	return eff.ID
}

func decodeList(t *testing.T, body string) api.ApprovalList {
	t.Helper()
	var out api.ApprovalList
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode approval list: %v: %s", err, body)
	}
	return out
}

func itemByID(list api.ApprovalList, id string) (api.ApprovalItem, bool) {
	for _, it := range list.Items {
		if it.ID == id {
			return it, true
		}
	}
	return api.ApprovalItem{}, false
}

func countEvents(t *testing.T, b *backend, typ string) int {
	t.Helper()
	var n int
	if err := b.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM run_events WHERE type = ?`, typ).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", typ, err)
	}
	return n
}

// ── R1: the one inbox, all seven kinds, landed scoping intact ───────────────

// sevenKinds seeds one card of every landed kind, disjoint by owner so a
// dropped predicate reads as a leak rather than a coincidence.
func sevenKinds(t *testing.T, e *decisionEnv) {
	t.Helper()
	b := e.b
	seedTask(t, b, "t-alice", "alice", "Alice Task", "doing")
	seedRun(t, b, "r-alice", "alice", "t-alice", "parked", "lane-a")
	seedTask(t, b, "t-bob", "bob", "Bob Task", "doing")
	seedRun(t, b, "r-bob", "bob", "t-bob", "parked", "lane-b")

	seedCard(t, b, "ask-alice", "r-alice", "alice", approvalCard("high", "ALICE-ASK"), time.Now())
	seedCard(t, b, "ask-bob", "r-bob", "bob", approvalCard("low", "BOB-ASK"), time.Now())
	seedProposal(t, e, "r-alice", "alice", `{"kind":"send","to":"ALICE-EFFECT"}`)
	seedProposal(t, e, "r-bob", "bob", `{"kind":"send","to":"BOB-EFFECT"}`)

	// Watchdog flag (member-visible), conformance + drift + alarm (operator-only),
	// benchmark verdict (REQUESTER-owned — a member card the operator does not own).
	appendRun(t, b, "alice", "r-alice", "watchdog.flagged",
		`{"rule":"watchdog.loop","anomaly_class":"watchdog.loop","severity":"flag-now","detail":"ALICE-FLAG"}`)
	exec(t, b, `INSERT INTO conformance_registry
	    (row_id, owning_section, fixtures, trigger_set, schedule, cadence, affect_class, last_run, last_result)
	    VALUES (?, 'S14.5', 'go test ./x/', 'quarterly', 'quarterly sweep', 'quarterly', 'lane', ?, 'red')`,
		"CONF-ROW", nowTS())
	appendPlatformPayload(t, b, "drift.finding",
		`{"source":"DRIFT-SRC","lanes":["l"],"change_class":"breaking","severity":"flag-now","summary":"DRIFT-CARD","fingerprint":"fp-1","row_id":"w-1","classified":true}`)
	appendPlatformPayload(t, b, "benchmark.alarm",
		`{"action":"raise","domain":"ALARM-DOMAIN","epoch_id":"e1","severity":"flag-now","summary":"ALARM-CARD","loss_g":0.96,"threshold":0.95,"expansion_freeze":true}`)
	exec(t, b, `INSERT INTO benchmark_pairs
	    (pair_id, user_id, domain, task_id, deliverable_id, phase, rate_pct, sampled_ts, state, render_a, render_b, updated_ts)
	    VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		"VERDICT-PAIR", "alice", "software-development", "t-alice", "d-1", "pre-gate", 100,
		nowTS(), "rendered", "aaa", "bbb", nowTS())
}

func appendPlatformPayload(t *testing.T, b *backend, typ, payload string) {
	t.Helper()
	if _, err := b.log.Append(context.Background(), eventlog.Append{
		UserID: "platform", Type: typ, SchemaVersion: 1, Payload: json.RawMessage(payload),
	}); err != nil {
		t.Fatalf("append %s: %v", typ, err)
	}
}

func TestApprovalListServesAllSevenKindsWithLandedScoping(t *testing.T) {
	e := newDecisionEnv(t)
	sevenKinds(t, e)

	op := decodeList(t, e.mustDo(t, "op", "GET", "/api/approvals", ""))
	kinds := map[string]int{}
	for _, it := range op.Items {
		kinds[it.Kind]++
	}
	for _, k := range []string{"ask", "effect", "watchdog_flag", "conformance_card", "drift_card", "benchmark_verdict", "benchmark_alarm"} {
		if kinds[k] == 0 {
			t.Errorf("the operator's inbox is missing kind %q (all seven landed kinds serve through one endpoint, R1)", k)
		}
	}

	alice := e.mustDo(t, "alice", "GET", "/api/approvals", "")
	bob := e.mustDo(t, "bob", "GET", "/api/approvals", "")
	// Member-visible, own only.
	for _, tok := range []string{"ALICE-ASK", "ALICE-EFFECT", "ALICE-FLAG", "VERDICT-PAIR"} {
		if !strings.Contains(alice, tok) {
			t.Errorf("alice's inbox is missing her own %s — an empty answer is not a scoped one", tok)
		}
		if strings.Contains(bob, tok) {
			t.Errorf("bob's inbox LEAKS alice's %s", tok)
		}
	}
	if !strings.Contains(bob, "BOB-ASK") || !strings.Contains(bob, "BOB-EFFECT") {
		t.Error("bob's inbox is missing his own cards")
	}
	// Operator-only kinds stay operator-only.
	for _, tok := range []string{"CONF-ROW", "DRIFT-CARD", "ALARM-CARD"} {
		if strings.Contains(alice, tok) || strings.Contains(bob, tok) {
			t.Errorf("a platform-scope card %q reached a member's inbox (S01.9)", tok)
		}
		if !strings.Contains(e.mustDo(t, "op", "GET", "/api/approvals", ""), tok) {
			t.Errorf("the operator's inbox is missing platform-scope card %q", tok)
		}
	}
	// The benchmark verdict is the member-owned card the operator does not own:
	// it is still visible to the operator (who sees all) but its owner is the
	// requester, which is the landed BENCH-REG §3.3 property.
	if it, ok := itemByID(op, "benchmark_verdict:VERDICT-PAIR"); !ok || it.Owner != "alice" {
		t.Errorf("the blind-pair verdict card must stay REQUESTER-owned; got %+v", it)
	}
}

func TestApprovalListIsRiskRankedAndAskCardsCarryTierAndHelp(t *testing.T) {
	e := newDecisionEnv(t)
	seedTask(t, e.b, "t-1", "alice", "T", "doing")
	seedRun(t, e.b, "r-1", "alice", "t-1", "parked", "lane")
	seedCard(t, e.b, "ask-low", "r-1", "alice", approvalCard("low", "LOW"), time.Now())
	seedCard(t, e.b, "ask-high", "r-1", "alice", approvalCard("high", "HIGH"), time.Now())

	list := decodeList(t, e.mustDo(t, "alice", "GET", "/api/approvals", ""))
	if len(list.Items) < 2 {
		t.Fatalf("want both cards, got %d", len(list.Items))
	}
	if list.Items[0].Tier != "high" {
		t.Errorf("the inbox must arrive ranked by risk (S15.6): first item tier %q", list.Items[0].Tier)
	}
	if list.Cursor == 0 {
		t.Error("the inbox must carry its snapshot-then-tail cursor (S15.3)")
	}
	high, _ := itemByID(list, "ask:ask-high")
	switch {
	case high.Tier != "high":
		t.Errorf("tier = %q, want high (read from the stored card)", high.Tier)
	case !high.StepUpRequired:
		t.Error("a High-tier card must demand the PIN re-prompt (S15.6)")
	case high.Batchable:
		t.Error("a High-tier card is NEVER batchable (S15.6)")
	case !strings.Contains(string(high.Card), "Approving starts the work"):
		t.Error("the card must carry the 13.5 help block from the stored snapshot")
	case len(high.Actions) == 0 || high.Actions[0] != "approve":
		t.Errorf("the card's OWN action vocabulary must ride verbatim; got %v", high.Actions)
	}
	low, _ := itemByID(list, "ask:ask-low")
	if !low.Batchable || low.StepUpRequired {
		t.Errorf("a Low-tier card batches and never steps up; got batchable=%v stepup=%v", low.Batchable, low.StepUpRequired)
	}
}

// TestPayloadHashIsTheOneCanonicalization proves R2's pin comes from the ONE
// canonicalization function rather than a second implementation.
func TestPayloadHashIsTheOneCanonicalization(t *testing.T) {
	e := newDecisionEnv(t)
	seedTask(t, e.b, "t-1", "alice", "T", "doing")
	seedRun(t, e.b, "r-1", "alice", "t-1", "parked", "lane")
	snapshot := approvalCard("medium", "PIN")
	seedCard(t, e.b, "ask-1", "r-1", "alice", snapshot, time.Now())
	effectID := seedProposal(t, e, "r-1", "alice", `{"kind":"send","body":"x"}`)

	list := decodeList(t, e.mustDo(t, "alice", "GET", "/api/approvals", ""))
	ask, _ := itemByID(list, "ask:ask-1")
	want, err := gates.CanonicalHash(json.RawMessage(snapshot))
	if err != nil {
		t.Fatalf("canonical hash: %v", err)
	}
	if ask.PayloadHash != want {
		t.Errorf("ask pin = %q, want the gates canonical hash %q", ask.PayloadHash, want)
	}
	eff, _ := itemByID(list, "effect:"+effectID)
	stored, err := e.journal.Get(context.Background(), effectID)
	if err != nil {
		t.Fatalf("get effect: %v", err)
	}
	if eff.PayloadHash != stored.PayloadHash {
		t.Errorf("effect pin = %q, want the journal's own pinned hash %q served verbatim", eff.PayloadHash, stored.PayloadHash)
	}
	if eff.Tier != "high" {
		t.Errorf("every S02.7 effect approval is High by definition (S15.6); got %q", eff.Tier)
	}
}

// TestStalenessDerivesAtReadWithoutAnySweep: the time-based flag flips on an
// aged fixture with nothing sweeping and no ticker anywhere — it is a function
// of the row's own observed time and the ⚙ horizon, read at read time.
func TestStalenessDerivesAtReadWithoutAnySweep(t *testing.T) {
	e := newDecisionEnv(t)
	seedTask(t, e.b, "t-1", "alice", "T", "doing")
	seedRun(t, e.b, "r-1", "alice", "t-1", "parked", "lane")
	fresh := time.Now()
	aged := fresh.Add(-3 * time.Hour) // ⚙ freshness.max_age = 1h in this fixture
	seedCard(t, e.b, "ask-fresh", "r-1", "alice", approvalCard("medium", "FRESH"), fresh)
	seedCard(t, e.b, "ask-aged", "r-1", "alice", approvalCard("medium", "AGED"), aged)

	list := decodeList(t, e.mustDo(t, "alice", "GET", "/api/approvals", ""))
	freshItem, _ := itemByID(list, "ask:ask-fresh")
	agedItem, _ := itemByID(list, "ask:ask-aged")
	if freshItem.Stale {
		t.Error("a card inside ⚙ freshness.max_age must not be flagged stale")
	}
	if !agedItem.Stale || len(agedItem.StaleReasons) == 0 {
		t.Errorf("a card pending past ⚙ freshness.max_age must flag stale WITH its reason; got %+v", agedItem)
	}
	// The flag never blocks answering (S06.9 / S15.6): the aged card is still
	// answerable and still carries its one-click re-plan action.
	if !agedItem.Answerable || !containsAction(agedItem.Actions, "replan") {
		t.Errorf("a stale card stays answerable with re-plan offered; got %+v", agedItem)
	}
	// Expiry countdown = observed + ⚙ effects.approval_expiry.
	if agedItem.ExpiryAt == nil || agedItem.ExpiryAt.Sub(agedItem.ObservedTS) != 7*24*time.Hour {
		t.Errorf("expiry_at must be observed + ⚙ effects.approval_expiry; got %v", agedItem.ExpiryAt)
	}
}

func containsAction(actions []string, want string) bool {
	for _, a := range actions {
		if a == want {
			return true
		}
	}
	return false
}

// ── R4: retry-safety, per verb ──────────────────────────────────────────────

func TestAnswerStaleHashFiresNothing(t *testing.T) {
	e := newDecisionEnv(t)
	seedTask(t, e.b, "t-1", "alice", "T", "doing")
	seedRun(t, e.b, "r-1", "alice", "t-1", "parked", "lane")
	seedCard(t, e.b, "ask-1", "r-1", "alice", approvalCard("medium", "STALE"), time.Now())
	effectID := seedProposal(t, e, "r-1", "alice", `{"kind":"send"}`)
	before := countEvents(t, e.b, "intake.state")

	code, body := e.do(t, "alice", "POST", "/api/approvals/ask:ask-1/answer",
		`{"payload_hash":"not-the-hash","answer":{"action":"approve"}}`)
	if code != http.StatusConflict || !strings.Contains(body, "stale_payload") {
		t.Fatalf("stale ask hash: status %d body %s, want a 409 stale_payload", code, body)
	}
	if !strings.Contains(body, `"current"`) || !strings.Contains(body, "STALE") {
		t.Error("a stale-hash refusal must carry the FRESH card back (S15.2)")
	}
	// Side-effect probe: nothing fired.
	if e.surface.calls != 0 {
		t.Errorf("the pipeline was reached on a stale hash (%d calls) — nothing may fire", e.surface.calls)
	}
	if got := countEvents(t, e.b, "intake.state"); got != before {
		t.Errorf("a stale-hash answer appended %d audit rows; want none", got-before)
	}

	code, body = e.do(t, "alice", "POST", "/api/approvals/effect:"+effectID+"/answer",
		`{"payload_hash":"not-the-hash","pin":"x","answer":{"action":"approve"}}`)
	if code != http.StatusConflict || !strings.Contains(body, "stale_payload") {
		t.Fatalf("stale effect hash: status %d body %s", code, body)
	}
	eff, _ := e.journal.Get(context.Background(), effectID)
	if eff.State != gates.EffectProposed {
		t.Errorf("a stale-hash effect answer moved the journal row to %s — nothing may fire", eff.State)
	}
	if n := countEvents(t, e.b, api.EventDecisionRecorded); n != 0 {
		t.Errorf("a stale-hash refusal recorded %d decisions; want none", n)
	}
}

func TestDoubleSubmitCannotDoubleFire(t *testing.T) {
	e := newDecisionEnv(t)
	seedTask(t, e.b, "t-1", "alice", "T", "doing")
	seedRun(t, e.b, "r-1", "alice", "t-1", "parked", "lane")
	snapshot := approvalCard("medium", "RETRY")
	seedCard(t, e.b, "ask-1", "r-1", "alice", snapshot, time.Now())
	hash, _ := gates.CanonicalHash(json.RawMessage(snapshot))
	body := `{"payload_hash":"` + hash + `","answer":{"action":"approve"}}`

	first := e.mustDo(t, "alice", "POST", "/api/approvals/ask:ask-1/answer", body)
	if !strings.Contains(first, `"applied":true`) {
		t.Fatalf("first answer must apply; got %s", first)
	}
	second := e.mustDo(t, "alice", "POST", "/api/approvals/ask:ask-1/answer", body)
	if !strings.Contains(second, `"applied":false`) || !strings.Contains(second, `"state":"answered"`) {
		t.Fatalf("a repeat must return the already-resolved state; got %s", second)
	}
	if e.surface.calls != 1 {
		t.Fatalf("the answer verb ran %d times on a double submit — a phone retry double-fired", e.surface.calls)
	}
	if n := countEvents(t, e.b, "intake.state"); n != 1 {
		t.Errorf("the double submit produced %d audit rows; one act is one row", n)
	}

	// (c) a DIFFERENT answer on a resolved item is a conflict, never a second
	// application.
	code, out := e.do(t, "alice", "POST", "/api/approvals/ask:ask-1/answer",
		`{"payload_hash":"`+hash+`","answer":{"action":"cancel"}}`)
	if code != http.StatusConflict || !strings.Contains(out, "already_answered") {
		t.Fatalf("conflicting answer on a resolved card: status %d body %s", code, out)
	}
	if e.surface.calls != 1 {
		t.Errorf("a conflicting answer reached the pipeline (%d calls)", e.surface.calls)
	}
}

func TestEffectApproveDoubleSubmitCannotDoubleFire(t *testing.T) {
	e := newDecisionEnv(t)
	seedTask(t, e.b, "t-1", "carol", "T", "doing")
	seedRun(t, e.b, "r-1", "carol", "t-1", "running", "lane")
	effectID := seedProposal(t, e, "r-1", "carol", `{"kind":"send","to":"x"}`)
	stored, _ := e.journal.Get(context.Background(), effectID)
	body := `{"payload_hash":"` + stored.PayloadHash + `","pin":"` + fixturePIN + `","answer":{"action":"approve"}}`

	first := e.mustDo(t, "carol", "POST", "/api/approvals/effect:"+effectID+"/answer", body)
	if !strings.Contains(first, `"state":"approved"`) {
		t.Fatalf("first approve must approve; got %s", first)
	}
	second := e.mustDo(t, "carol", "POST", "/api/approvals/effect:"+effectID+"/answer", body)
	if !strings.Contains(second, `"applied":false`) || !strings.Contains(second, `"state":"approved"`) {
		t.Fatalf("a repeat must read back the resolved state; got %s", second)
	}
	if n := countEvents(t, e.b, api.EventDecisionRecorded); n != 1 {
		t.Errorf("the double submit recorded %d decisions; one act is one row", n)
	}
	eff, _ := e.journal.Get(context.Background(), effectID)
	if eff.Attempts != 0 {
		t.Errorf("approving executed the effect (attempts=%d): approval and execution are different facts (D7)", eff.Attempts)
	}
}

// ── R5: tier mechanics ──────────────────────────────────────────────────────

func TestHighTierAnswerDemandsTheSameRequestPIN(t *testing.T) {
	e := newDecisionEnv(t)
	ctx := context.Background()
	seedTask(t, e.b, "t-1", "carol", "T", "doing")
	seedRun(t, e.b, "r-1", "carol", "t-1", "running", "lane")
	effectID := seedProposal(t, e, "r-1", "carol", `{"kind":"send"}`)
	stored, _ := e.journal.Get(ctx, effectID)
	pinned := `"payload_hash":"` + stored.PayloadHash + `"`

	code, body := e.do(t, "carol", "POST", "/api/approvals/effect:"+effectID+"/answer",
		`{`+pinned+`,"answer":{"action":"approve"}}`)
	if code != http.StatusUnauthorized || !strings.Contains(body, "pin_required") {
		t.Fatalf("High tier without a PIN: status %d body %s, want pin_required", code, body)
	}
	failedBefore := countEvents(t, e.b, auth.EventRepromptFailed)
	code, body = e.do(t, "carol", "POST", "/api/approvals/effect:"+effectID+"/answer",
		`{`+pinned+`,"pin":"wrong-pin","answer":{"action":"approve"}}`)
	if code != http.StatusUnauthorized || !strings.Contains(body, "pin_rejected") {
		t.Fatalf("High tier with a wrong PIN: status %d body %s", code, body)
	}
	if countEvents(t, e.b, auth.EventRepromptFailed) != failedBefore+1 {
		t.Error("a failed step-up must land on the audit log (auth.reprompt_failed)")
	}
	if eff, _ := e.journal.Get(ctx, effectID); eff.State != gates.EffectProposed {
		t.Errorf("a refused step-up moved the effect to %s — nothing may fire", eff.State)
	}
	out := e.mustDo(t, "carol", "POST", "/api/approvals/effect:"+effectID+"/answer",
		`{`+pinned+`,"pin":"`+fixturePIN+`","answer":{"action":"approve"}}`)
	if !strings.Contains(out, `"state":"approved"`) {
		t.Fatalf("High tier with the right PIN must apply; got %s", out)
	}
}

// TestPlatformLevelEffectNeedsBothApprovals is the D10 co-approval limb: a
// platform-level effect (one attributed to no run — nobody's task) reaches
// gates.Approve only after BOTH the owner and the operator have said yes, and
// both states are visible on the card the whole time.
func TestPlatformLevelEffectNeedsBothApprovals(t *testing.T) {
	e := newDecisionEnv(t)
	ctx := context.Background()
	effectID := seedProposal(t, e, "", "carol", `{"kind":"rotate","scope":"household"}`)
	stored, _ := e.journal.Get(ctx, effectID)
	body := `{"payload_hash":"` + stored.PayloadHash + `","pin":"` + fixturePIN + `","answer":{"action":"approve"}}`

	list := decodeList(t, e.mustDo(t, "carol", "GET", "/api/approvals", ""))
	card, ok := itemByID(list, "effect:"+effectID)
	if !ok || card.Approvals == nil || !card.Approvals.PlatformLevel {
		t.Fatalf("a run-less effect must read as platform-level on the card; got %+v", card.Approvals)
	}

	out := e.mustDo(t, "carol", "POST", "/api/approvals/effect:"+effectID+"/answer", body)
	if !strings.Contains(out, `"state":"proposed"`) || !strings.Contains(out, `"owner_approved":true`) {
		t.Fatalf("the owner's approval alone must NOT approve a platform-level effect; got %s", out)
	}
	if eff, _ := e.journal.Get(ctx, effectID); eff.State != gates.EffectProposed {
		t.Fatalf("gates.Approve fired on one approval; effect is %s", eff.State)
	}
	// Both states are on the card between the two signatures.
	list = decodeList(t, e.mustDo(t, "opal", "GET", "/api/approvals", ""))
	card, _ = itemByID(list, "effect:"+effectID)
	if card.Approvals == nil || !card.Approvals.OwnerApproved || card.Approvals.OperatorApproved {
		t.Fatalf("both approver states must be visible on the card; got %+v", card.Approvals)
	}

	out = e.mustDo(t, "opal", "POST", "/api/approvals/effect:"+effectID+"/answer", body)
	if !strings.Contains(out, `"state":"approved"`) {
		t.Fatalf("the operator's co-approval must complete it; got %s", out)
	}
	if n := countEvents(t, e.b, api.EventDecisionRecorded); n != 2 {
		t.Errorf("each approval is its own decision row; got %d, want 2", n)
	}
	eff, _ := e.journal.Get(ctx, effectID)
	if eff.ApprovedBy != "opal" {
		t.Errorf("the single approved_by column records the FINAL approver; got %q", eff.ApprovedBy)
	}
}

// TestOwnerScopedEffectRefusesTheOperator: the role bit decides VISIBILITY,
// never authorship of another person's decision. A run-attributed effect is its
// owner's own call (D10).
func TestOwnerScopedEffectRefusesTheOperator(t *testing.T) {
	e := newDecisionEnv(t)
	seedTask(t, e.b, "t-1", "alice", "T", "doing")
	seedRun(t, e.b, "r-1", "alice", "t-1", "running", "lane")
	effectID := seedProposal(t, e, "r-1", "alice", `{"kind":"send"}`)
	stored, _ := e.journal.Get(context.Background(), effectID)
	body := `{"payload_hash":"` + stored.PayloadHash + `","pin":"x","answer":{"action":"approve"}}`

	if code, _ := e.do(t, "op", "POST", "/api/approvals/effect:"+effectID+"/answer", body); code != http.StatusForbidden {
		t.Errorf("the operator answering an owner-scoped effect: status %d, want 403 (D10)", code)
	}
	if code, _ := e.do(t, "bob", "POST", "/api/approvals/effect:"+effectID+"/answer", body); code != http.StatusForbidden {
		t.Errorf("another member answering: status %d, want 403", code)
	}
	if code, _ := e.do(t, "alice", "POST", "/api/approvals/effect:missing/answer", body); code != http.StatusNotFound {
		t.Errorf("an unknown effect: status %d, want 404 (404-before-403)", code)
	}
	if eff, _ := e.journal.Get(context.Background(), effectID); eff.State != gates.EffectProposed {
		t.Errorf("a refused answer moved the effect to %s", eff.State)
	}
}

// TestAskAnswerCrossOwnerThreeWay — the non-tautological direction: the
// member's OWN answer must SUCCEED, so an all-refusing handler cannot pass.
func TestAskAnswerCrossOwnerThreeWay(t *testing.T) {
	e := newDecisionEnv(t)
	seedTask(t, e.b, "t-1", "alice", "T", "doing")
	seedRun(t, e.b, "r-1", "alice", "t-1", "parked", "lane")
	snapshot := approvalCard("medium", "SCOPE")
	seedCard(t, e.b, "ask-1", "r-1", "alice", snapshot, time.Now())
	hash, _ := gates.CanonicalHash(json.RawMessage(snapshot))
	body := `{"payload_hash":"` + hash + `","answer":{"action":"approve"}}`

	if code, out := e.do(t, "bob", "POST", "/api/approvals/ask:ask-1/answer", body); code != http.StatusForbidden {
		t.Errorf("another member answering alice's card: status %d body %s, want 403", code, out)
	}
	// D10: the ask's OWNER answers; the operator reads it but does not decide it.
	if code, _ := e.do(t, "op", "POST", "/api/approvals/ask:ask-1/answer", body); code != http.StatusForbidden {
		t.Errorf("the operator answering another person's card: status %d, want 403 (D10)", code)
	}
	if code, out := e.do(t, "alice", "POST", "/api/approvals/ask:unknown/answer", body); code != http.StatusNotFound {
		t.Errorf("an unknown card: status %d body %s, want 404", code, out)
	}
	if e.surface.calls != 0 {
		t.Fatalf("a refused answer reached the pipeline (%d calls)", e.surface.calls)
	}
	if code, out := e.do(t, "alice", "POST", "/api/approvals/ask:ask-1/answer", body); code != http.StatusOK {
		t.Fatalf("the OWNER's own answer must succeed: status %d body %s", code, out)
	}
}

// ── R6: the Low-tier batch ──────────────────────────────────────────────────

func TestLowTierBatchAppliesIndividuallyAndRefusesPerItem(t *testing.T) {
	e := newDecisionEnv(t)
	seedTask(t, e.b, "t-1", "alice", "T", "doing")
	seedRun(t, e.b, "r-1", "alice", "t-1", "parked", "lane")
	lowA, lowB := approvalCard("low", "LOW-A"), approvalCard("low", "LOW-B")
	high := approvalCard("high", "HIGH-IN-BATCH")
	med := approvalCard("standard", "MEDIUM-IN-BATCH")
	seedCard(t, e.b, "ask-a", "r-1", "alice", lowA, time.Now())
	seedCard(t, e.b, "ask-b", "r-1", "alice", lowB, time.Now())
	seedCard(t, e.b, "ask-h", "r-1", "alice", high, time.Now())
	seedCard(t, e.b, "ask-m", "r-1", "alice", med, time.Now())
	ha, _ := gates.CanonicalHash(json.RawMessage(lowA))
	hb, _ := gates.CanonicalHash(json.RawMessage(lowB))
	hh, _ := gates.CanonicalHash(json.RawMessage(high))
	hm, _ := gates.CanonicalHash(json.RawMessage(med))

	body := `{"items":[
	  {"id":"ask:ask-a","payload_hash":"` + ha + `","answer":{"action":"approve"}},
	  {"id":"ask:ask-h","payload_hash":"` + hh + `","answer":{"action":"approve"}},
	  {"id":"ask:ask-m","payload_hash":"` + hm + `","answer":{"action":"approve"}},
	  {"id":"ask:ask-b","payload_hash":"` + hb + `","answer":{"action":"approve"}}]}`
	out := e.mustDo(t, "alice", "POST", "/api/approvals/answer-batch", body)

	var res api.ApprovalBatchResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("decode batch: %v: %s", err, out)
	}
	if len(res.Outcomes) != 4 {
		t.Fatalf("every item must be individually listed; got %d outcomes", len(res.Outcomes))
	}
	// Medium is individual and High is never batched (S15.6): both are refused
	// per item, and the refusal names the tier that refused.
	for _, i := range []int{1, 2} {
		if res.Outcomes[i].Code != "not_batchable" {
			t.Errorf("outcome %d must be refused per item; got %+v", i, res.Outcomes[i])
		}
	}
	for _, i := range []int{0, 3} {
		if res.Outcomes[i].Status != http.StatusOK || res.Outcomes[i].Result == nil || !res.Outcomes[i].Result.Applied {
			t.Errorf("Low-tier item %d must still apply while a sibling is refused; got %+v", i, res.Outcomes[i])
		}
	}
	// Individually APPLIED and individually logged: two applications, two
	// canonical audit rows — never one merged act.
	if e.surface.calls != 2 {
		t.Errorf("the batch applied %d answers; want 2 individual applications", e.surface.calls)
	}
	if n := countEvents(t, e.b, "intake.state"); n != 2 {
		t.Errorf("the batch produced %d audit rows; want one per applied item", n)
	}
	// The other half of the ratified ruling (drain D2): an act with a landed
	// canonical row is NEVER double-minted, so a Low-tier batch of asks
	// co-produces NO family-5 row at all (OQ8; §39).
	if n := countEvents(t, e.b, api.EventDecisionRecorded); n != 0 {
		t.Errorf("the batch minted %d decision.recorded rows; asks carry their own canonical row (OQ8)", n)
	}
	// The refused item is untouched.
	var status string
	if err := e.b.db.QueryRowContext(context.Background(), `SELECT status FROM asks WHERE ask_id='ask-h'`).Scan(&status); err != nil {
		t.Fatalf("read refused ask: %v", err)
	}
	if status != "open" {
		t.Errorf("the refused High-tier card was touched (status %q)", status)
	}
	// A batch is bounded.
	items := make([]string, 0, answerBatchOverflow)
	for i := 0; i < answerBatchOverflow; i++ {
		items = append(items, `{"id":"ask:ask-a","payload_hash":"x","answer":{}}`)
	}
	code, _ := e.do(t, "alice", "POST", "/api/approvals/answer-batch", `{"items":[`+strings.Join(items, ",")+`]}`)
	if code != http.StatusBadRequest {
		t.Errorf("an over-cap batch: status %d, want 400 (the bound is structural, with its reason)", code)
	}
}

// answerBatchOverflow is one past the transport's structural batch cap.
const answerBatchOverflow = 101

// ── R5/OQ2: the deny disposition ────────────────────────────────────────────

func TestEffectDenyIsTerminalNeverExecutedAndLeavesTheInbox(t *testing.T) {
	e := newDecisionEnv(t)
	ctx := context.Background()
	seedTask(t, e.b, "t-1", "carol", "T", "doing")
	seedRun(t, e.b, "r-1", "carol", "t-1", "running", "lane")
	effectID := seedProposal(t, e, "r-1", "carol", `{"kind":"send","to":"nobody"}`)
	stored, _ := e.journal.Get(ctx, effectID)

	out := e.mustDo(t, "carol", "POST", "/api/approvals/effect:"+effectID+"/answer",
		`{"payload_hash":"`+stored.PayloadHash+`","pin":"`+fixturePIN+`","answer":{"action":"deny","reason":"not this one"}}`)
	if !strings.Contains(out, `"state":"failed"`) {
		t.Fatalf("a denied effect settles terminally as failed (OQ2); got %s", out)
	}
	eff, _ := e.journal.Get(ctx, effectID)
	switch {
	case eff.State != gates.EffectFailed:
		t.Errorf("state = %s, want failed", eff.State)
	case eff.Attempts != 0:
		t.Errorf("a denied effect was executed (attempts=%d)", eff.Attempts)
	case eff.IdempotencyKey != "":
		t.Errorf("a denied effect got an idempotency key %q — it never reached execution", eff.IdempotencyKey)
	case !strings.Contains(string(eff.Result), `"denied":true`),
		!strings.Contains(string(eff.Result), `"actor":"carol"`),
		!strings.Contains(string(eff.Result), "not this one"):
		t.Errorf("the denial result must be machine-readable {denied, actor, reason}; got %s", eff.Result)
	}
	if n := countEvents(t, e.b, api.EventDecisionRecorded); n != 1 {
		t.Errorf("the denial must be audited exactly once; got %d rows", n)
	}
	// It leaves the inbox: the derivation lists `proposed` effects only.
	if strings.Contains(e.mustDo(t, "carol", "GET", "/api/approvals", ""), effectID) {
		t.Error("a denied effect still appears in the inbox")
	}
	// And a repeat reads the resolved state back rather than re-denying.
	repeat := e.mustDo(t, "carol", "POST", "/api/approvals/effect:"+effectID+"/answer",
		`{"payload_hash":"`+stored.PayloadHash+`","pin":"`+fixturePIN+`","answer":{"action":"deny","reason":"not this one"}}`)
	if !strings.Contains(repeat, `"applied":false`) {
		t.Errorf("a repeated denial must read back; got %s", repeat)
	}
}

// ── R3/R19: redaction at the serving edge ───────────────────────────────────

func TestApprovalsRedactPayloadDerivedCardsAndServeContentUnwrapped(t *testing.T) {
	e := newDecisionEnv(t)
	const secret = "sk-ant-api03-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	seedTask(t, e.b, "t-1", "alice", "T", "doing")
	seedRun(t, e.b, "r-1", "alice", "t-1", "parked", "lane")
	appendRun(t, e.b, "alice", "r-1", "watchdog.flagged",
		`{"rule":"watchdog.loop","anomaly_class":"watchdog.loop","severity":"flag-now","detail":"leaked `+secret+`"}`)
	// The ask snapshot is S06 decision CONTENT: it goes out unwrapped, exactly
	// as the task detail's own title does (§38 ruling (b)).
	seedCard(t, e.b, "ask-1", "r-1", "alice", approvalCard("medium", "Deploy the api03 report"), time.Now())

	body := e.mustDo(t, "alice", "GET", "/api/approvals", "")
	if strings.Contains(body, secret) {
		t.Fatal("a payload-derived inbox card leaked a secret to the wire (R3/§30)")
	}
	if !strings.Contains(body, "[REDACTED") {
		t.Fatal("the payload-derived card must carry the redaction marker")
	}
	if !strings.Contains(body, "Deploy the api03 report") {
		t.Error("the ask snapshot is decision CONTENT and must go out unwrapped (§38 ruling (b))")
	}
	// Store-raw: the stored row is untouched.
	var stored string
	if err := e.b.db.QueryRowContext(context.Background(),
		`SELECT payload FROM run_events WHERE type='watchdog.flagged'`).Scan(&stored); err != nil {
		t.Fatalf("read stored row: %v", err)
	}
	if !strings.Contains(stored, secret) {
		t.Error("the stored run_events payload was mutated — store-raw / serve-redacted (§30 R19)")
	}
}

// ── R7/R18: the audit walk over every new route ─────────────────────────────

// TestEveryDecisionRouteLandsOnTheEventLog walks each mutation this packet adds
// and asserts its audit row — the family-5 decision.recorded where the act has
// no canonical row of its own, and the landed canonical row where it does.
// Nothing is double-minted (OQ8).
func TestEveryDecisionRouteLandsOnTheEventLog(t *testing.T) {
	e := newDecisionEnv(t)
	ctx := context.Background()
	seedTask(t, e.b, "t-1", "carol", "T", "doing")
	seedRun(t, e.b, "r-1", "carol", "t-1", "parked", "lane")
	snapshot := approvalCard("medium", "AUDIT")
	seedCard(t, e.b, "ask-1", "r-1", "carol", snapshot, time.Now())
	hash, _ := gates.CanonicalHash(json.RawMessage(snapshot))

	// (1) an ask answer: the pipeline's own canonical row, and NO second row.
	e.mustDo(t, "carol", "POST", "/api/approvals/ask:ask-1/answer",
		`{"payload_hash":"`+hash+`","answer":{"action":"approve"}}`)
	if n := countEvents(t, e.b, "intake.state"); n != 1 {
		t.Errorf("the ask answer must land on its own canonical row; got %d", n)
	}
	if n := countEvents(t, e.b, api.EventDecisionRecorded); n != 0 {
		t.Errorf("an act with a landed canonical row must NOT be double-minted (OQ8); got %d decision rows", n)
	}

	// (2) an effect approve: no journal row reaches run_events, so this is the
	// act decision.recorded exists to cover.
	effectID := seedProposal(t, e, "r-1", "carol", `{"kind":"send"}`)
	stored, _ := e.journal.Get(ctx, effectID)
	e.mustDo(t, "carol", "POST", "/api/approvals/effect:"+effectID+"/answer",
		`{"payload_hash":"`+stored.PayloadHash+`","pin":"`+fixturePIN+`","answer":{"action":"approve"}}`)

	var payload string
	if err := e.b.db.QueryRowContext(ctx,
		`SELECT payload FROM run_events WHERE type = ? ORDER BY event_seq DESC LIMIT 1`,
		api.EventDecisionRecorded).Scan(&payload); err != nil {
		t.Fatalf("read decision row: %v", err)
	}
	var d map[string]any
	if err := json.Unmarshal([]byte(payload), &d); err != nil {
		t.Fatalf("decode decision payload: %v", err)
	}
	// The S14.2 family-5 contract minimum, verbatim — including the latency
	// datum P-T05-2 needs.
	for _, k := range []string{"actor", "card_id", "card_type", "decision", "presented_at", "decided_at", "latency_s"} {
		if _, ok := d[k]; !ok {
			t.Errorf("decision.recorded payload is missing the family-5 required field %q", k)
		}
	}
	if d["card_type"] != "effect" || d["decision"] != "approve" || d["actor"] != "carol" {
		t.Errorf("decision payload = %v", d)
	}
	// Owner-attributed (15.6).
	var owner string
	if err := e.b.db.QueryRowContext(ctx,
		`SELECT user_id FROM run_events WHERE type = ? ORDER BY event_seq DESC LIMIT 1`,
		api.EventDecisionRecorded).Scan(&owner); err != nil {
		t.Fatalf("read decision owner: %v", err)
	}
	if owner != "carol" {
		t.Errorf("the decision row must be owner-attributed; got %q", owner)
	}
}

// TestTheDecisionPlaneIsUnversionedAndOutwardEffectFree is the route-table
// review as an assertion: no /v1 anywhere, and no route performs an outward
// act — outward effects still exit only through the S02.7 journal (D7).
func TestTheDecisionPlaneIsUnversionedAndOutwardEffectFree(t *testing.T) {
	e := newDecisionEnv(t)
	for _, path := range []string{
		"/v1/api/approvals", "/api/v1/approvals",
	} {
		if code, _ := e.do(t, "alice", "GET", path, ""); code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404: the API is unversioned at v0 (S15.2)", path, code)
		}
	}
	// The one journal verb this plane can reach is Approve/Deny — never
	// BeginExecute, never a provider call. Proven structurally: an approved
	// effect has attempts 0 and no idempotency key until the executor runs.
	seedTask(t, e.b, "t-1", "alice", "T", "doing")
	seedRun(t, e.b, "r-1", "alice", "t-1", "running", "lane")
	id := seedProposal(t, e, "r-1", "alice", `{"kind":"send"}`)
	eff, _ := e.journal.Get(context.Background(), id)
	if eff.Attempts != 0 || eff.IdempotencyKey != "" {
		t.Fatalf("fixture invariant broken: %+v", eff)
	}
}

// ── the not-wired posture ───────────────────────────────────────────────────

func TestDecisionPlaneNotWiredIs503(t *testing.T) {
	b := newBackend(t)
	seedUser(t, b, "alice", auth.RoleMember)
	srv := api.New(api.Config{
		Log: b.log, Sessions: b.store, Auth: fixedIdentity{"alice"},
		Settings: approvalSettings(),
		HealthFn: func() api.Health { return api.Health{Ready: true} },
		DB:       b.db,
	})
	for _, route := range []struct{ method, path, body string }{
		{"POST", "/api/runs/r-1/cancel", "{}"},
		{"POST", "/api/tasks/t-1/cancel", "{}"},
		{"POST", "/api/deliverables/d-1/follow-up", "{}"},
	} {
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, httptest.NewRequest(route.method, route.path, strings.NewReader(route.body)))
		if rr.Code != http.StatusServiceUnavailable {
			t.Errorf("%s %s: status %d, want 503 when the surface is not wired", route.method, route.path, rr.Code)
		}
	}
}

// TestDecisionShapesNeverPercent extends the §30 progress-semantics NEGATIVE
// over every shape this packet ships: the inbox, an answer result, a batch
// result, a cancel outcome and a spawn record. There is no percent, fraction,
// ratio or ETA key in any of them, and no undeclared ratio-shaped key either.
func TestDecisionShapesNeverPercent(t *testing.T) {
	e := newDecisionEnv(t)
	sevenKinds(t, e)
	snapshot := approvalCard("medium", "SCAN")
	seedCard(t, e.b, "ask-scan", "r-alice", "alice", snapshot, time.Now())
	hash, _ := gates.CanonicalHash(json.RawMessage(snapshot))

	bodies := []string{
		e.mustDo(t, "op", "GET", "/api/approvals", ""),
		e.mustDo(t, "alice", "POST", "/api/approvals/ask:ask-scan/answer",
			`{"payload_hash":"`+hash+`","answer":{"action":"approve"}}`),
		e.mustDo(t, "alice", "POST", "/api/approvals/answer-batch",
			`{"items":[{"id":"ask:ask-bob","payload_hash":"x","answer":{"action":"approve"}}]}`),
		e.mustDo(t, "alice", "POST", "/api/runs/r-alice/cancel", "{}"),
	}
	keys := map[string]bool{}
	for _, body := range bodies {
		var v any
		if err := json.Unmarshal([]byte(body), &v); err != nil {
			t.Fatalf("decode: %v: %s", err, body)
		}
		collectKeys(v, keys)
	}
	if len(keys) == 0 {
		t.Fatal("collected no keys — the scan proves nothing")
	}
	forbidden := map[string]bool{
		"percent": true, "percentage": true, "percent_complete": true,
		"fraction": true, "complete_fraction": true, "ratio": true,
		"progress": true, "pct": true, "complete": true, "completion": true,
		"eta": true, "eta_s": true, "eta_seconds": true,
	}
	for k := range keys {
		lk := strings.ToLower(k)
		if forbidden[lk] || strings.Contains(lk, "percent") {
			t.Fatalf("a decision-plane shape exposes forbidden progress key %q (§30)", k)
		}
		for _, shape := range []string{"_pct", "_ratio", "_fraction", "utiliz"} {
			if strings.Contains(lk, shape) {
				t.Errorf("undeclared ratio-shaped key %q reached a decision-plane shape (§30 R10)", k)
			}
		}
	}
}

// TestApprovalListIsBounded: the inbox is a page a person works through, and
// the bound is OBSERVED (one row beyond the cap) rather than assumed.
func TestApprovalListIsBounded(t *testing.T) {
	e := newDecisionEnv(t)
	seedTask(t, e.b, "t-1", "alice", "T", "doing")
	seedRun(t, e.b, "r-1", "alice", "t-1", "parked", "lane")
	for i := 0; i < 205; i++ {
		seedCard(t, e.b, fmt.Sprintf("ask-%03d", i), "r-1", "alice",
			approvalCard("low", fmt.Sprintf("CARD-%03d", i)), time.Now())
	}
	list := decodeList(t, e.mustDo(t, "alice", "GET", "/api/approvals", ""))
	if !list.Truncated {
		t.Error("an over-cap inbox must say it was truncated, never silently drop")
	}
	if len(list.Items) > 200 {
		t.Errorf("the inbox served %d items past its structural bound", len(list.Items))
	}
}

// TestEveryNewRouteHasANamedAuditRow is the route-table inventory: each verb
// this packet adds is listed WITH the canonical audit row it lands on, and the
// list is asserted against the routes the server actually answers. A route
// added later without an audit row named here fails the walk.
func TestEveryNewRouteHasANamedAuditRow(t *testing.T) {
	inventory := []struct {
		method, path, audit, why string
	}{
		{"GET", "/api/approvals", "", "a read: it changes nothing and audits nothing"},
		{"POST", "/api/approvals/{id}/answer", "intake.state | decision.recorded",
			"an ask answer lands the pipeline's own canonical row; an effect answer lands the family-5 row (OQ8)"},
		{"POST", "/api/approvals/answer-batch", "intake.state (one per applied item)",
			"a batch is individually applied, so it is individually audited"},
		{"POST", "/api/runs/{run}/cancel", "run.state_changed", "the cancel rides its transition (OQ8)"},
		{"POST", "/api/tasks/{task}/cancel", "run.state_changed", "one per non-terminal run"},
		{"POST", "/api/deliverables/{deliverable}/follow-up", "intake.followup_spawned",
			"the landed spawn row; no decision.recorded is double-minted"},
	}
	e := newDecisionEnv(t)
	for _, r := range inventory {
		if r.audit == "" && r.method != "GET" {
			t.Errorf("mutating route %s %s names no audit row", r.method, r.path)
		}
		// Each listed route must actually exist: a probe on a concrete instance
		// must not answer 404 for a MISSING ROUTE (405 or a handler status is
		// fine — the point is that the pattern is registered).
		concrete := strings.NewReplacer("{id}", "ask:none", "{run}", "r-none",
			"{task}", "t-none", "{deliverable}", "d-none").Replace(r.path)
		code, _ := e.do(t, "alice", r.method, concrete, "{}")
		if code == http.StatusMethodNotAllowed {
			t.Errorf("%s %s is not routed", r.method, r.path)
		}
	}
	// And the two answer acts that DO mint the family-5 row are exactly the two
	// effect verbs — nothing else in this packet co-produces it (OQ8).
	if n := countEvents(t, e.b, api.EventDecisionRecorded); n != 0 {
		t.Errorf("the inventory walk itself recorded %d decisions; every probe was refused", n)
	}
}

// ── drain D3: the co-approval derivation is CYCLE-SCOPED ────────────────────

// TestExpiredApprovalDoesNotCarryIntoTheNextCycle: the journal sends an effect
// back to `proposed` when its approval window lapses (S02.7). A signature from
// the cycle before must not still count — otherwise a re-approval would dedup
// against it and fire gates.Approve with no fresh audit row, and on a
// platform-level effect the operator alone could complete a conjunction the
// owner signed a fortnight ago.
func TestExpiredApprovalDoesNotCarryIntoTheNextCycle(t *testing.T) {
	e := newDecisionEnv(t)
	ctx := context.Background()
	effectID := seedProposal(t, e, "", "carol", `{"kind":"rotate","scope":"household"}`)
	stored, _ := e.journal.Get(ctx, effectID)
	cardID := "effect:" + effectID
	body := `{"payload_hash":"` + stored.PayloadHash + `","pin":"` + fixturePIN + `","answer":{"action":"approve"}}`

	// A signature from the PREVIOUS approval cycle: given for this very payload,
	// but 8 days ago — past ⚙ effects.approval_expiry, which is exactly when the
	// journal reverts the row to `proposed`.
	appendAgedDecision(t, e, "carol", "carol", cardID, stored.PayloadHash, false, 8*24*time.Hour)

	list := decodeList(t, e.mustDo(t, "opal", "GET", "/api/approvals", ""))
	card, _ := itemByID(list, cardID)
	if card.Approvals == nil || card.Approvals.OwnerApproved {
		t.Fatalf("an EXPIRED owner signature still counts on the card: %+v", card.Approvals)
	}
	// The operator alone must NOT complete the conjunction now.
	out := e.mustDo(t, "opal", "POST", "/api/approvals/"+cardID+"/answer", body)
	if !strings.Contains(out, `"state":"proposed"`) {
		t.Fatalf("the operator alone completed a conjunction whose owner-yes had expired; got %s", out)
	}
	if eff, _ := e.journal.Get(ctx, effectID); eff.State != gates.EffectProposed {
		t.Fatalf("gates.Approve fired on an expired signature; effect is %s", eff.State)
	}
	// Carol re-approves in the NEW cycle. The dedup runs on the SAME cycle
	// predicate, so this is not deduped against the expired row: a FRESH row is
	// minted and only then does the conjunction complete.
	out = e.mustDo(t, "carol", "POST", "/api/approvals/"+cardID+"/answer", body)
	if !strings.Contains(out, `"state":"approved"`) {
		t.Fatalf("the re-approval did not complete the conjunction; got %s", out)
	}
	if n := countEvents(t, e.b, api.EventDecisionRecorded); n != 3 {
		t.Errorf("decision rows = %d, want 3 (the expired one + a fresh operator + a fresh owner) — a post-expiry re-approval must mint a FRESH row", n)
	}
}

// TestDriftedPayloadDropsPriorSignatures: a drift reversion changes the pinned
// hash, so signatures given for the OLD payload stop counting — nobody's
// approval silently transfers to content they never saw.
func TestDriftedPayloadDropsPriorSignatures(t *testing.T) {
	e := newDecisionEnv(t)
	ctx := context.Background()
	effectID := seedProposal(t, e, "", "carol", `{"kind":"rotate","target":"before"}`)
	stored, _ := e.journal.Get(ctx, effectID)
	e.mustDo(t, "carol", "POST", "/api/approvals/effect:"+effectID+"/answer",
		`{"payload_hash":"`+stored.PayloadHash+`","pin":"`+fixturePIN+`","answer":{"action":"approve"}}`)

	list := decodeList(t, e.mustDo(t, "carol", "GET", "/api/approvals", ""))
	if card, _ := itemByID(list, "effect:"+effectID); card.Approvals == nil || !card.Approvals.OwnerApproved {
		t.Fatal("fixture invariant broken: the owner signature must count before the drift")
	}

	// The stored payload drifts (S02.7's drift case) — the row is re-pinned.
	newPayload := `{"kind":"rotate","target":"AFTER"}`
	newHash, _ := gates.CanonicalHash(json.RawMessage(newPayload))
	exec(t, e.b, `UPDATE effects SET payload = ?, payload_hash = ? WHERE effect_id = ?`,
		newPayload, newHash, effectID)

	list = decodeList(t, e.mustDo(t, "carol", "GET", "/api/approvals", ""))
	card, _ := itemByID(list, "effect:"+effectID)
	if card.Approvals == nil || card.Approvals.OwnerApproved {
		t.Fatalf("a signature given for the OLD payload still counts after drift: %+v", card.Approvals)
	}
	if card.PayloadHash != newHash {
		t.Errorf("the card must serve the CURRENT pin; got %q", card.PayloadHash)
	}
	_ = ctx
}

// appendAgedDecision writes a decision.recorded row as it would have been
// written in a PAST approval cycle: backdated both on the wire (the row's ts)
// and in the payload (decided_at). It is an APPEND — run_events is append-only
// by trigger and this fixture respects that rather than reaching around it.
func appendAgedDecision(t *testing.T, e *decisionEnv, actor, owner, cardID, payloadHash string, isOperator bool, age time.Duration) {
	t.Helper()
	when := time.Now().UTC().Add(-age)
	payload, err := json.Marshal(map[string]any{
		"actor": actor, "card_id": cardID, "card_type": "effect", "decision": "approve",
		"presented_at": when.Format(time.RFC3339Nano), "decided_at": when.Format(time.RFC3339Nano),
		"latency_s": 0, "payload_hash": payloadHash, "actor_is_operator": isOperator,
	})
	if err != nil {
		t.Fatalf("marshal aged decision: %v", err)
	}
	if _, err := e.b.log.Append(context.Background(), eventlog.Append{
		UserID: owner, Type: api.EventDecisionRecorded, SchemaVersion: 1,
		Payload: payload, Time: when,
	}); err != nil {
		t.Fatalf("append aged decision: %v", err)
	}
}

// ── drain D9: `answerable` is the CALLER's, not the kind's ──────────────────

// TestAnswerableMatchesTheAuthorityTheVerbEnforces: the list and the verb read
// the same rule, so a surface never renders a control whose use would 403.
func TestAnswerableMatchesTheAuthorityTheVerbEnforces(t *testing.T) {
	e := newDecisionEnv(t)
	seedTask(t, e.b, "t-1", "alice", "T", "doing")
	seedRun(t, e.b, "r-1", "alice", "t-1", "running", "lane")
	seedCard(t, e.b, "ask-1", "r-1", "alice", approvalCard("medium", "MINE"), time.Now())
	owned := seedProposal(t, e, "r-1", "alice", `{"kind":"send"}`)   // run-attributed: alice's own
	platform := seedProposal(t, e, "", "alice", `{"kind":"rotate"}`) // run-less: platform-level

	cases := []struct {
		who, id    string
		answerable bool
		why        string
	}{
		{"alice", "ask:ask-1", true, "the owner answers their own card"},
		{"op", "ask:ask-1", false, "D10: the operator reads it but does not decide it"},
		{"alice", "effect:" + owned, true, "the owner answers their own effect"},
		{"op", "effect:" + owned, false, "an owner-scoped effect is not the operator's to answer"},
		{"alice", "effect:" + platform, true, "the owner signs a platform-level effect"},
		{"op", "effect:" + platform, true, "the operator co-approves a platform-level effect (D10)"},
	}
	for _, c := range cases {
		list := decodeList(t, e.mustDo(t, c.who, "GET", "/api/approvals", ""))
		it, ok := itemByID(list, c.id)
		if !ok {
			t.Fatalf("%s cannot see %s at all", c.who, c.id)
		}
		if it.Answerable != c.answerable {
			t.Errorf("%s sees %s answerable=%v, want %v (%s)", c.who, c.id, it.Answerable, c.answerable, c.why)
		}
		if !it.Answerable && it.NotAnswerableReason == "" {
			t.Errorf("%s sees %s un-answerable with no reason — a dead control with no explanation", c.who, c.id)
		}
		// The card and the verb must AGREE: what the card says is answerable
		// must not 403, and what it says is not must not 200.
		code, _ := e.do(t, c.who, "POST", "/api/approvals/"+c.id+"/answer",
			`{"payload_hash":"deliberately-stale","answer":{"action":"approve"}}`)
		refused := code == http.StatusForbidden
		if refused == c.answerable {
			t.Errorf("%s on %s: card says answerable=%v but the verb answered %d — the button and the door disagree",
				c.who, c.id, c.answerable, code)
		}
	}
}

// ── drain D10: `failed` is reached by two different acts ────────────────────

// TestDenyAndExecutionFailureAreDifferentAnswers: a denial is terminal and is
// never read back as an approval, and an execution failure after an approval is
// not read back as a denial.
func TestDenyAndExecutionFailureAreDifferentAnswers(t *testing.T) {
	e := newDecisionEnv(t)
	ctx := context.Background()
	seedTask(t, e.b, "t-1", "carol", "T", "doing")
	seedRun(t, e.b, "r-1", "carol", "t-1", "running", "lane")

	denied := seedProposal(t, e, "r-1", "carol", `{"kind":"send","to":"a"}`)
	dh, _ := e.journal.Get(ctx, denied)
	e.mustDo(t, "carol", "POST", "/api/approvals/effect:"+denied+"/answer",
		`{"payload_hash":"`+dh.PayloadHash+`","pin":"`+fixturePIN+`","answer":{"action":"deny","reason":"no"}}`)

	// deny-after-deny reads back at 200.
	out := e.mustDo(t, "carol", "POST", "/api/approvals/effect:"+denied+"/answer",
		`{"payload_hash":"`+dh.PayloadHash+`","pin":"`+fixturePIN+`","answer":{"action":"deny","reason":"no"}}`)
	if !strings.Contains(out, `"applied":false`) {
		t.Errorf("deny-after-deny must read back; got %s", out)
	}
	// approve-after-deny is a CONFLICT naming the denial — never "already approved".
	code, body := e.do(t, "carol", "POST", "/api/approvals/effect:"+denied+"/answer",
		`{"payload_hash":"`+dh.PayloadHash+`","pin":"`+fixturePIN+`","answer":{"action":"approve"}}`)
	if code != http.StatusConflict {
		t.Fatalf("approve-after-deny: status %d, want 409", code)
	}
	if !strings.Contains(body, "DENIED") {
		t.Errorf("the conflict must say the effect was denied, not that it was approved; got %s", body)
	}

	// The other `failed`: an approved effect whose EXECUTION failed. An approve
	// on it reads back (the approval did stand); a deny is a conflict.
	exec := seedProposal(t, e, "r-1", "carol", `{"kind":"send","to":"b"}`)
	eh, _ := e.journal.Get(ctx, exec)
	if _, err := e.journal.Approve(ctx, exec, "carol"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, err := e.journal.BeginExecute(ctx, exec); err != nil {
		t.Fatalf("begin execute: %v", err)
	}
	if _, err := e.journal.Fail(ctx, exec, json.RawMessage(`{"provider":"500"}`)); err != nil {
		t.Fatalf("fail: %v", err)
	}
	out = e.mustDo(t, "carol", "POST", "/api/approvals/effect:"+exec+"/answer",
		`{"payload_hash":"`+eh.PayloadHash+`","pin":"`+fixturePIN+`","answer":{"action":"approve"}}`)
	if !strings.Contains(out, `"applied":false`) || !strings.Contains(out, `"state":"failed"`) {
		t.Errorf("approve on an execution-failed effect must read the approval back; got %s", out)
	}
	if code, _ := e.do(t, "carol", "POST", "/api/approvals/effect:"+exec+"/answer",
		`{"payload_hash":"`+eh.PayloadHash+`","pin":"`+fixturePIN+`","answer":{"action":"deny"}}`); code != http.StatusConflict {
		t.Errorf("deny on an already-executed effect: status %d, want 409", code)
	}
}

// TestDenyRecordsItsDecisionOnlyAfterTheJournalActSucceeds (drain D6): a deny
// that the journal refuses leaves NO audit row claiming a refusal that never
// took effect.
func TestDenyRecordsItsDecisionOnlyAfterTheJournalActSucceeds(t *testing.T) {
	e := newDecisionEnv(t)
	ctx := context.Background()
	seedTask(t, e.b, "t-1", "carol", "T", "doing")
	seedRun(t, e.b, "r-1", "carol", "t-1", "running", "lane")
	id := seedProposal(t, e, "r-1", "carol", `{"kind":"send"}`)
	stored, _ := e.journal.Get(ctx, id)
	// Approve it out from under the deny: Deny wants `proposed` and will refuse.
	if _, err := e.journal.Approve(ctx, id, "carol"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	before := countEvents(t, e.b, api.EventDecisionRecorded)
	code, _ := e.do(t, "carol", "POST", "/api/approvals/effect:"+id+"/answer",
		`{"payload_hash":"`+stored.PayloadHash+`","pin":"`+fixturePIN+`","answer":{"action":"deny"}}`)
	if code == http.StatusOK {
		t.Fatal("denying an approved effect must not succeed")
	}
	if got := countEvents(t, e.b, api.EventDecisionRecorded); got != before {
		t.Errorf("a refused deny left %d orphan audit rows (drain D6)", got-before)
	}
}
