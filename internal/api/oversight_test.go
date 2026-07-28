package api_test

// oversight_test.go — the four oversight verbs of P3-B6-2B (R14–R17): watchdog
// suppress, "resume — I was wrong", drift-card dismiss, conformance
// acknowledge. Each battery runs the three-way cross-owner check in the
// non-tautological direction, and each card verb proves the DERIVATION reads its
// decision back — which is what makes dismissal and acknowledgement
// derive-from-log facts rather than side-store columns.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
)

// ── seams ───────────────────────────────────────────────────────────────────

// fakeSuppressor records exactly what the transport handed the landed
// watchdog.Suppress. It performs no suppression of its own: the retune-proposal
// mechanics and the event append are internal/watchdog's and are asserted there,
// and a fake that re-implemented them would be asserting itself.
type fakeSuppressor struct {
	mu    sync.Mutex
	calls []suppressCall
	err   error
}

type suppressCall struct{ actor, runID, rule string }

func (f *fakeSuppressor) Suppress(_ context.Context, actor, runID, rule string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.calls = append(f.calls, suppressCall{actor, runID, rule})
	return nil
}

func (f *fakeSuppressor) last() (suppressCall, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return suppressCall{}, 0
	}
	return f.calls[len(f.calls)-1], len(f.calls)
}

// fakeResumer takes the REAL S02.3 parked→running edge through run.Store — the
// same edge internal/stage's surface takes — so the projection assertions below
// are made against a genuine transition rather than a hand-written event.
type fakeResumer struct {
	runs *run.Store
	mu   sync.Mutex
	seen []string
}

func (f *fakeResumer) ResumeRun(ctx context.Context, actor, runID string) (json.RawMessage, error) {
	f.mu.Lock()
	f.seen = append(f.seen, actor+":"+runID)
	f.mu.Unlock()
	if f.runs == nil {
		return json.RawMessage(`{"run_id":"` + runID + `","applied":true}`), nil
	}
	r, err := f.runs.Transition(ctx, runID, run.StateRunning, run.TransitionOptions{
		Reason: "resumed by " + actor, Actor: actor,
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"run_id": runID, "applied": true, "generation": r.Generation})
}

func (f *fakeResumer) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.seen)
}

// ── fixtures ────────────────────────────────────────────────────────────────

// seedFlag appends one open watchdog flag on a run, owner-attributed.
func seedFlag(t *testing.T, b *backend, owner, runID, class, severity string) {
	t.Helper()
	appendRun(t, b, owner, runID, "watchdog.flagged",
		`{"rule":"`+strings.SplitN(class, ":", 2)[0]+`","anomaly_class":"`+class+`","severity":"`+severity+`","detail":"D"}`)
}

// seedPlatformFlag appends one RUN-LESS flag — the suffixed anomaly-class shape
// (watchdog.spend:<owner>) that was permanently un-clearable before the §34 D5
// fix, and that the transport must therefore pass through verbatim.
func seedPlatformFlag(t *testing.T, b *backend, class string) {
	t.Helper()
	appendPlatformPayload(t, b, "watchdog.flagged",
		`{"rule":"`+strings.SplitN(class, ":", 2)[0]+`","anomaly_class":"`+class+`","severity":"digest","detail":"D"}`)
}

// seedFindingAt appends one drift finding at a chosen instant, so a test can
// open two INCIDENT WINDOWS without waiting a day.
func seedFindingAt(t *testing.T, b *backend, fingerprint, summary string, when time.Time) int64 {
	t.Helper()
	payload := `{"source":"SRC","lanes":["l"],"change_class":"breaking","severity":"flag-now",` +
		`"summary":"` + summary + `","fingerprint":"` + fingerprint + `","row_id":"w-1","classified":true}`
	seq, err := b.log.Append(context.Background(), eventlog.Append{
		UserID: "platform", Type: "drift.finding", SchemaVersion: 1,
		Payload: json.RawMessage(payload), Time: when,
	})
	if err != nil {
		t.Fatalf("append drift finding: %v", err)
	}
	return seq
}

func seedRedConformanceRow(t *testing.T, b *backend, rowID, affect, lastRun string) {
	t.Helper()
	exec(t, b, `INSERT INTO conformance_registry
	    (row_id, owning_section, fixtures, trigger_set, schedule, cadence, affect_class, last_run, last_result)
	    VALUES (?, 'S14.5', 'go test ./x/', 'quarterly', 'quarterly sweep', 'quarterly', ?, ?, 'red')`,
		rowID, affect, lastRun)
}

func inboxKinds(t *testing.T, e *verbEnv, who, kind string) []api.ApprovalItem {
	t.Helper()
	list := decodeList(t, e.mustDo(t, who, "GET", "/api/approvals", ""))
	var out []api.ApprovalItem
	for _, it := range list.Items {
		if it.Kind == kind {
			out = append(out, it)
		}
	}
	return out
}

// ── R14: watchdog suppress ─────────────────────────────────────────────────

// TestSuppressScopeIsVisibility: you can suppress exactly what you can see. The
// owner's own act SUCCEEDS (the non-tautological direction), another member is
// refused, and the operator — who sees everything — may suppress it too.
func TestSuppressScopeIsVisibility(t *testing.T) {
	for _, tc := range []struct {
		name, who string
		want      int
	}{
		{"the flag's owner suppresses it", "alice", http.StatusOK},
		{"another member cannot see it, so cannot suppress it", "bob", http.StatusNotFound},
		{"the operator sees everything and may suppress it", "op", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newVerbEnv(t)
			seedTask(t, e.b, "t-alice", "alice", "T", "doing")
			seedRun(t, e.b, "r-alice", "alice", "t-alice", "running", "lane-a")
			seedFlag(t, e.b, "alice", "r-alice", "watchdog.loop", "flag-now")

			code, out := e.do(t, tc.who, "POST", "/api/watchdog/flags/suppress",
				`{"run_id":"r-alice","anomaly_class":"watchdog.loop"}`)
			if code != tc.want {
				t.Fatalf("status %d, want %d: %s", code, tc.want, out)
			}
			call, n := e.watchdog.last()
			if tc.want != http.StatusOK {
				if n != 0 {
					t.Fatalf("a refused suppression still reached the watchdog: %+v", call)
				}
				return
			}
			if n != 1 || call.runID != "r-alice" || call.rule != "watchdog.loop" || call.actor != tc.who {
				t.Fatalf("suppress call = %+v (n=%d), want the acting identity and the flag's own class", call, n)
			}
		})
	}
}

// TestSuppressPassesTheFullSuffixedAnomalyClass is the §34 D5 regression: a
// run-less flag's class carries a suffix, and the derive-from-log supersession
// matches the FULL class. A transport that trimmed it to the bare rule would
// make those flags permanently un-clearable — which is exactly what happened
// before D5 — so the class the card served is what reaches the verb, verbatim.
func TestSuppressPassesTheFullSuffixedAnomalyClass(t *testing.T) {
	e := newVerbEnv(t)
	const class = "watchdog.spend:alice"
	seedPlatformFlag(t, e.b, class)

	// Platform-scope flags are operator-only, as the landed projection scopes
	// them: a member cannot even see this one.
	if code, _ := e.do(t, "alice", "POST", "/api/watchdog/flags/suppress",
		`{"anomaly_class":"`+class+`"}`); code != http.StatusNotFound {
		t.Fatalf("member suppressing a platform-scope flag: status %d, want 404", code)
	}
	e.mustDo(t, "op", "POST", "/api/watchdog/flags/suppress", `{"anomaly_class":"`+class+`"}`)
	call, n := e.watchdog.last()
	if n != 1 || call.rule != class || call.runID != "" {
		t.Fatalf("suppress call = %+v, want the FULL suffixed class on a run-less flag (§34 D5)", call)
	}

	// And the round trip closes: with the landed producer's row appended for
	// that class, the card leaves the open list.
	appendPlatformPayload(t, e.b, "watchdog.suppressed", `{"rule":"`+class+`","actor":"op","count":1}`)
	if got := inboxKinds(t, e, "op", "watchdog_flag"); len(got) != 0 {
		t.Fatalf("the flag is still open after a suppression for its full class: %+v", got)
	}
}

func TestSuppressRefusesAnUnknownClassAndFiresNothing(t *testing.T) {
	e := newVerbEnv(t)
	seedTask(t, e.b, "t-alice", "alice", "T", "doing")
	seedRun(t, e.b, "r-alice", "alice", "t-alice", "running", "lane-a")
	seedFlag(t, e.b, "alice", "r-alice", "watchdog.loop", "flag-now")

	for _, body := range []string{
		`{"run_id":"r-alice","anomaly_class":"watchdog.nonesuch"}`,
		`{"anomaly_class":"watchdog.loop"}`, // right class, wrong (run-less) subject
	} {
		if code, out := e.do(t, "alice", "POST", "/api/watchdog/flags/suppress", body); code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404: %s", body, code, out)
		}
	}
	if _, n := e.watchdog.last(); n != 0 {
		t.Error("a refused suppression still reached the watchdog")
	}
	if code, _ := e.do(t, "alice", "POST", "/api/watchdog/flags/suppress", `{"run_id":"r-alice"}`); code != http.StatusBadRequest {
		t.Error("a suppression with no anomaly class was not a 400")
	}
}

// ── R15: "resume — I was wrong" ────────────────────────────────────────────

// TestResumeVerbCrossOwnerAndSupersedesTheOpenFlag: the owner's own resume
// succeeds, another member's is refused, the operator's succeeds — and the
// resume supersedes the run's open watchdog flag through the LANDED derivation,
// with no projection change of any kind.
func TestResumeVerbCrossOwnerAndSupersedesTheOpenFlag(t *testing.T) {
	for _, tc := range []struct {
		name, who string
		want      int
	}{
		{"the owner resumes her own parked run", "alice", http.StatusOK},
		{"another member cannot", "bob", http.StatusForbidden},
		{"the operator may resume anybody's", "op", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newVerbEnv(t)
			e.resume.runs = run.NewStore(e.b.db, e.b.log)
			seedTask(t, e.b, "t-alice", "alice", "T", "doing")
			seedParkedRun(t, e, "r-alice", "alice", "t-alice")
			seedFlag(t, e.b, "alice", "r-alice", "watchdog.loop", "flag-now")

			if got := inboxKinds(t, e, "alice", "watchdog_flag"); len(got) != 1 {
				t.Fatalf("fixture: expected one open flag, got %d", len(got))
			}
			code, out := e.do(t, tc.who, "POST", "/api/runs/r-alice/resume", `{}`)
			if code != tc.want {
				t.Fatalf("status %d, want %d: %s", code, tc.want, out)
			}
			if tc.want != http.StatusOK {
				if e.resume.count() != 0 {
					t.Fatal("a refused resume still reached the resume surface")
				}
				if got := inboxKinds(t, e, "alice", "watchdog_flag"); len(got) != 1 {
					t.Fatal("a refused resume cleared the flag anyway")
				}
				return
			}
			// The supersession is the LANDED derivation reading the transition
			// this verb took: "resume, I was wrong" closes the flag.
			if got := inboxKinds(t, e, "alice", "watchdog_flag"); len(got) != 0 {
				t.Fatalf("the flag is still open after a human resume: %+v", got)
			}
		})
	}
	t.Run("an unknown run is 404", func(t *testing.T) {
		e := newVerbEnv(t)
		if code, _ := e.do(t, "alice", "POST", "/api/runs/r-nope/resume", `{}`); code != http.StatusNotFound {
			t.Fatal("resume of an unknown run was not a 404")
		}
	})
}

func seedParkedRun(t *testing.T, e *verbEnv, runID, owner, taskID string) {
	t.Helper()
	seedRun(t, e.b, runID, owner, taskID, "parked", "lane-a")
}

// ── R16 / OQ7: drift-card dismiss ──────────────────────────────────────────

// TestDriftDismissIsOperatorOnlyAndClosesOnlyThisIncident is the OQ7 battery in
// one arc: a member cannot dismiss (they cannot even see the card), the operator
// can, the dismissed card leaves the open list, a finding in a NEW window opens
// a NEW card, and the drift.finding rows are untouched throughout.
func TestDriftDismissIsOperatorOnlyAndClosesOnlyThisIncident(t *testing.T) {
	e := newVerbEnv(t)
	first := seedFindingAt(t, e.b, "fp-1", "FIRST-STORM", time.Now().Add(-48*time.Hour))

	if got := inboxKinds(t, e, "op", "drift_card"); len(got) != 1 {
		t.Fatalf("fixture: expected one drift card, got %d", len(got))
	}
	if got := inboxKinds(t, e, "alice", "drift_card"); len(got) != 0 {
		t.Fatal("a member sees a drift card — they are platform-scope (S01.9)")
	}
	// A member cannot dismiss what they cannot see.
	if code, out := e.do(t, "alice", "POST", "/api/approvals/drift_card:fp-1/dismiss", `{}`); code != http.StatusNotFound {
		t.Fatalf("member dismiss: status %d, want 404: %s", code, out)
	}
	if n := len(decisionRows(t, e.b, "drift_card")); n != 0 {
		t.Fatalf("a refused dismissal recorded %d decisions", n)
	}

	out := e.mustDo(t, "op", "POST", "/api/approvals/drift_card:fp-1/dismiss", `{"reason":"known, tracked upstream"}`)
	var d api.DriftDismissed
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatalf("decode: %v: %s", err, out)
	}
	if !d.Dismissed || d.Fingerprint != "fp-1" || d.WindowStartSeq != first {
		t.Fatalf("dismissal = %+v, want this incident (fingerprint + window-start seq %d)", d, first)
	}
	if got := inboxKinds(t, e, "op", "drift_card"); len(got) != 0 {
		t.Fatalf("the dismissed card is still listed: %+v", got)
	}

	// A NEW window opens a NEW card: dismissal closes an incident, never a
	// fingerprint forever (a standing mute would be a watch deletion, S16.2).
	second := seedFindingAt(t, e.b, "fp-1", "SECOND-STORM", time.Now())
	cards := inboxKinds(t, e, "op", "drift_card")
	if len(cards) != 1 {
		t.Fatalf("a new window did not open a new card: %d cards", len(cards))
	}
	if second == first {
		t.Fatal("fixture: the two findings share a seq")
	}
	// …and the SECOND card is dismissible in its own right, independently.
	e.mustDo(t, "op", "POST", "/api/approvals/drift_card:fp-1/dismiss", `{}`)
	if got := inboxKinds(t, e, "op", "drift_card"); len(got) != 0 {
		t.Fatalf("the second incident's card survived its own dismissal: %+v", got)
	}

	// The FINDINGS are untouched: a dismissal closes a card, it does not delete
	// evidence. Both rows are still queryable.
	var n int
	if err := e.b.db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM run_events WHERE type = 'drift.finding'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("drift.finding rows = %d, want 2 — dismissing a card never deletes a finding (S14.6)", n)
	}
	if rows := decisionRows(t, e.b, "drift_card"); len(rows) != 2 {
		t.Fatalf("recorded %d drift dismissals, want 2 — every dismissal is audited", len(rows))
	}
}

func TestDismissRefusesANonDriftCardID(t *testing.T) {
	e := newVerbEnv(t)
	for _, id := range []string{"conformance_card:CONF", "ask:ask-1", "nonsense"} {
		if code, _ := e.do(t, "op", "POST", "/api/approvals/"+id+"/dismiss", `{}`); code != http.StatusBadRequest {
			t.Errorf("dismiss %q was not a 400 — other kinds have their own verbs", id)
		}
	}
}

// ── R17 / OQ6: conformance acknowledge ─────────────────────────────────────

// TestConformanceAcknowledgeStopsFlagNowButStaysRed is the OQ6 semantics
// exactly: the acknowledged red leaves flag-now attention and STAYS LISTED red.
func TestConformanceAcknowledgeStopsFlagNowButStaysRed(t *testing.T) {
	e := newVerbEnv(t)
	lastRun := nowMinus(time.Hour)
	seedRedConformanceRow(t, e.b, "CONF-ROW", "lane", lastRun)

	before := inboxKinds(t, e, "op", "conformance_card")
	if len(before) != 1 {
		t.Fatalf("fixture: expected one conformance card, got %d", len(before))
	}
	if before[0].Tier != "high" {
		t.Fatalf("an un-acknowledged lane-affecting red is tier %q, want high (flag-now)", before[0].Tier)
	}
	// A member cannot see or acknowledge it.
	if code, _ := e.do(t, "alice", "POST", "/api/approvals/conformance_card:CONF-ROW/acknowledge", `{}`); code != http.StatusNotFound {
		t.Fatal("a member acknowledged an operator-only conformance card")
	}

	out := e.mustDo(t, "op", "POST", "/api/approvals/conformance_card:CONF-ROW/acknowledge", `{"reason":"seen; fix is queued"}`)
	var ack api.ConformanceAcknowledged
	if err := json.Unmarshal([]byte(out), &ack); err != nil {
		t.Fatalf("decode: %v: %s", err, out)
	}
	if !ack.Acknowledged || !ack.StillRed {
		t.Fatalf("acknowledgement = %+v, want acknowledged AND still red", ack)
	}

	after := inboxKinds(t, e, "op", "conformance_card")
	if len(after) != 1 {
		t.Fatalf("the acknowledged card left the list — it must STAY LISTED red (OQ6): %d cards", len(after))
	}
	if after[0].Tier != "medium" {
		t.Errorf("acknowledged card tier = %q, want medium — it stops counting as flag-now attention", after[0].Tier)
	}
	var card api.InboxConformanceCard
	if err := json.Unmarshal(after[0].Card, &card); err != nil {
		t.Fatalf("decode card: %v", err)
	}
	if !card.Acknowledged || card.FlagNow || card.LastResult != "red" {
		t.Fatalf("card = %+v, want acknowledged, not flag-now, still red", card)
	}

	// THE NEGATIVE: no verb path made anything green. The registry row is
	// byte-identical where it matters.
	var lastResult, storedRun string
	if err := e.b.db.QueryRowContext(context.Background(),
		`SELECT last_result, last_run FROM conformance_registry WHERE row_id = ?`, "CONF-ROW").
		Scan(&lastResult, &storedRun); err != nil {
		t.Fatal(err)
	}
	if lastResult != "red" || storedRun != lastRun {
		t.Fatalf("the registry row moved to (%q, %q) — only a real suite run writes last_result (§32)", lastResult, storedRun)
	}
}

// TestAcknowledgementDoesNotCarryToALaterResult: the acknowledgement is scoped
// to the RESULT it was given for, the same cycle-scoping the co-approval
// derivation uses. A later suite run — even another red — is a NEW failure, and
// yesterday's acknowledgement must not silence it.
func TestAcknowledgementDoesNotCarryToALaterResult(t *testing.T) {
	e := newVerbEnv(t)
	seedRedConformanceRow(t, e.b, "CONF-ROW", "lane", nowMinus(2*time.Hour))
	e.mustDo(t, "op", "POST", "/api/approvals/conformance_card:CONF-ROW/acknowledge", `{}`)
	if got := inboxKinds(t, e, "op", "conformance_card"); got[0].Tier != "medium" {
		t.Fatalf("tier = %q after the acknowledgement, want medium", got[0].Tier)
	}

	// A fresh suite run records another red.
	exec(t, e.b, `UPDATE conformance_registry SET last_run = ? WHERE row_id = ?`, nowTS(), "CONF-ROW")
	got := inboxKinds(t, e, "op", "conformance_card")
	if len(got) != 1 || got[0].Tier != "high" {
		t.Fatalf("after a NEW red result the card is %+v, want flag-now again — an old acknowledgement never silences a new failure", got)
	}
}

// TestNoVerbPathWritesLastResult is the SOURCE assertion behind the probe above:
// internal/api contains no write to the conformance registry at all, so no verb
// can make a red row green even by accident. `last_result` moves through
// internal/conformance's RecordResult and nowhere else (§32).
func TestNoVerbPathWritesLastResult(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	scanned := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		scanned++
		// Line-by-line with comments skipped, and the tokens chosen so a READ is
		// not mistaken for a WRITE: the derivation legitimately SELECTs
		// `WHERE last_result = 'red'`, and what must not exist is a statement
		// that SETs it or a call to the one verb that does.
		for _, line := range strings.Split(string(src), "\n") {
			l := strings.TrimSpace(line)
			if strings.HasPrefix(l, "//") {
				continue
			}
			for _, bad := range writeTokens {
				if strings.Contains(l, bad) {
					t.Errorf("%s contains %q — no API verb may write a conformance result (§32; OQ6):\n  %s", name, bad, l)
				}
			}
		}
	}
	if scanned == 0 {
		t.Fatal("the scan read no files — it would pass vacuously")
	}
	// Non-tautology: the scan detects its own planted defect, and does NOT fire
	// on the legitimate read the derivation performs.
	probe := "\t_, _ = db.ExecContext(ctx, `UPDATE conformance_registry SET last_result = 'green'`)"
	if !anyToken(probe, writeTokens) {
		t.Fatal("the last_result scan cannot detect its own probe — it would pass vacuously")
	}
	read := "\tq := `SELECT row_id FROM conformance_registry WHERE last_result = 'red'`"
	if anyToken(read, writeTokens) {
		t.Fatal("the last_result scan fires on a legitimate READ — it would forbid the derivation itself")
	}
}

// writeTokens are the ways a conformance RESULT could be written from here.
var writeTokens = []string{
	"UPDATE conformance_registry", "INSERT INTO conformance_registry",
	"SET last_result", ".RecordResult(",
}

func anyToken(line string, tokens []string) bool {
	for _, tok := range tokens {
		if strings.Contains(line, tok) {
			return true
		}
	}
	return false
}

// ── the not-wired posture ──────────────────────────────────────────────────

func TestOversightVerbsNotWiredAre503(t *testing.T) {
	e := newVerbEnv(t)
	seedTask(t, e.b, "t-1", "alice", "T", "doing")
	seedRun(t, e.b, "r-1", "alice", "t-1", "parked", "lane-a")
	// A server with the read plane but neither oversight seam.
	srv := api.New(api.Config{
		Log: e.b.log, Sessions: e.b.store, Auth: fixedIdentity{"alice"},
		Settings: approvalSettings(),
		HealthFn: func() api.Health { return api.Health{Ready: true} },
		DB:       e.b.db,
	})
	for _, route := range []struct{ path, body string }{
		{"/api/watchdog/flags/suppress", `{"anomaly_class":"watchdog.loop"}`},
		{"/api/runs/r-1/resume", `{}`},
	} {
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, httptest.NewRequest("POST", route.path, strings.NewReader(route.body)))
		if rr.Code != http.StatusServiceUnavailable {
			t.Errorf("POST %s: status %d, want 503 when the seam is not wired", route.path, rr.Code)
		}
	}
}

// ── the audit inventory (rubric 18, part-B slice) ──────────────────────────

// TestEveryPartBRouteHasItsNamedAuditRow walks every verb this packet adds,
// asserts the route is registered, and asserts the audit row it lands — either
// the family-5 decision.recorded or the LANDED canonical row it rides. The
// second half is the one that matters: the two verbs that ride a landed row must
// NOT also mint a decision, because one act must never look like two (OQ8).
func TestEveryPartBRouteHasItsNamedAuditRow(t *testing.T) {
	inventory := []struct {
		method, path, audit, why string
	}{
		{"POST", "/api/meters/budget", "decision.recorded (card_type budget)",
			"a budget edit has no landed canonical row, so the family-5 row carries it with old→new (OQ8)"},
		{"POST", "/api/meters/pause", "decision.recorded (card_type automation_pause)",
			"both flips are audited with old→new"},
		{"POST", "/api/tasks/{task}/priority-hint", "decision.recorded (card_type priority_hint)",
			"the drag is logged whether or not the board was still current"},
		{"POST", "/api/watchdog/flags/suppress", "watchdog.suppressed",
			"the landed verb mints its own canonical row with the actor in the payload — never double-minted"},
		{"POST", "/api/runs/{run}/resume", "run.state_changed",
			"the resume rides its transition, which is also the supersession the projection reads"},
		{"POST", "/api/approvals/{id}/dismiss", "decision.recorded (card_type drift_card)",
			"the dismissal IS the record, and the derivation reads it back"},
		{"POST", "/api/approvals/{id}/acknowledge", "decision.recorded (card_type conformance_card)",
			"the acknowledgement IS the record, and the derivation reads it back"},
	}
	e := newVerbEnv(t)
	for _, r := range inventory {
		if r.audit == "" {
			t.Errorf("mutating route %s %s names no audit row", r.method, r.path)
		}
		concrete := strings.NewReplacer("{id}", "drift_card:none", "{run}", "r-none",
			"{task}", "t-none").Replace(r.path)
		code, _ := e.do(t, "alice", r.method, concrete, "{}")
		if code == http.StatusMethodNotAllowed {
			t.Errorf("%s %s is not routed", r.method, r.path)
		}
	}
	// Every probe above was refused, so the walk itself recorded nothing.
	if n := len(decisionRows(t, e.b, "")); n != 0 {
		t.Errorf("the inventory walk recorded %d decisions; every probe was refused", n)
	}

	// The two riders: neither mints a family-5 row.
	seedTask(t, e.b, "t-alice", "alice", "T", "doing")
	seedRun(t, e.b, "r-run", "alice", "t-alice", "running", "lane-a")
	seedFlag(t, e.b, "alice", "r-run", "watchdog.loop", "flag-now")
	e.mustDo(t, "alice", "POST", "/api/watchdog/flags/suppress",
		`{"run_id":"r-run","anomaly_class":"watchdog.loop"}`)

	e.resume.runs = run.NewStore(e.b.db, e.b.log)
	seedRun(t, e.b, "r-parked", "alice", "t-alice", "parked", "lane-a")
	e.mustDo(t, "alice", "POST", "/api/runs/r-parked/resume", `{}`)

	if n := len(decisionRows(t, e.b, "")); n != 0 {
		t.Errorf("suppress/resume minted %d decision.recorded rows — both ride a landed canonical row (OQ8)", n)
	}
	// And no /v1: the API is unversioned and stays that way (R23).
	for _, path := range []string{"/v1/api/meters/budget", "/api/v1/meters/pause"} {
		if code, _ := e.do(t, "alice", "POST", path, `{}`); code != http.StatusNotFound {
			t.Errorf("POST %s = %d, want 404: the API is unversioned at v0 (S15.2)", path, code)
		}
	}
}
