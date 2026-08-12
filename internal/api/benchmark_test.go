package api_test

// benchmark_test.go — the B6-2C verdict backend at the TRANSPORT (R20 + the
// part-C slice of H). internal/api may not import internal/benchmark in either
// direction (the reverse import wall), so this battery drives a FAKE surface and
// asserts what the transport itself owns: scope, order, refusal and the negative
// that no registered number lives in this package. The end-to-end over the REAL
// practice is internal/shell's, where the composition root can see both sides.
//
// $0 and hermetic: no engine, no seat, no network.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
)

// ── the fake practice ───────────────────────────────────────────────────────

// fakeBenchmark records the CALL ORDER as well as the calls. The order is the
// point: BENCH-REG §3.4 freezes "record, then reveal", and a transport that
// asked for a reveal first would be asking for arm identity the record had not
// yet earned.
type fakeBenchmark struct {
	mu    sync.Mutex
	calls []string
	// requesters records the scope the pending list was asked for.
	requesters []string
	// verdicts records (pair, verdict, guess) triples.
	verdicts []string
	declines []string
	disposes []string
	optIns   []string
	// recordErr, when set, refuses the verdict — the guess-less case.
	recordErr error
	// optedIn is the STORE's answer to the consent read, and it is MUTABLE so a
	// test can move it behind the handler's back. A response field hardcoded to
	// either position cannot follow it, which is the whole point of the probe.
	optedIn bool
	// optedInErr, when set, is the read that broke — the absence arm, which must
	// never degrade into "not opted in".
	optedInErr error
	// optedInFor records WHO each consent read asked about. It is what proves
	// the served block is the caller's own and never widens with the role bit.
	optedInFor []string
}

func (f *fakeBenchmark) note(s string) {
	f.mu.Lock()
	f.calls = append(f.calls, s)
	f.mu.Unlock()
}

func (f *fakeBenchmark) PendingVerdicts(_ context.Context, requester string) (json.RawMessage, error) {
	f.note("pending")
	f.mu.Lock()
	f.requesters = append(f.requesters, requester)
	f.mu.Unlock()
	// The package's own pre-record shape: no arm, no position, no run, no model.
	return json.RawMessage(`[{"pair_id":"bp-1","user_id":"alice","domain":"D","task_id":"t-1","render_a":"AAA","render_b":"BBB","length_a":3,"length_b":3}]`), nil
}

func (f *fakeBenchmark) RecordVerdict(_ context.Context, pairID, verdict, guess string) error {
	f.note("record")
	if f.recordErr != nil {
		return f.recordErr
	}
	f.mu.Lock()
	f.verdicts = append(f.verdicts, pairID+"/"+verdict+"/"+guess)
	f.mu.Unlock()
	return nil
}

func (f *fakeBenchmark) Reveal(_ context.Context, pairID string) (json.RawMessage, error) {
	f.note("reveal")
	return json.RawMessage(`{"pair_id":"` + pairID + `","platform_side":"A"}`), nil
}

func (f *fakeBenchmark) Decline(_ context.Context, pairID string) error {
	f.note("decline")
	f.mu.Lock()
	f.declines = append(f.declines, pairID)
	f.mu.Unlock()
	return nil
}

func (f *fakeBenchmark) DisposeAlarm(_ context.Context, actor, domain, disposition, reason string) error {
	f.note("dispose")
	f.mu.Lock()
	f.disposes = append(f.disposes, actor+"/"+domain+"/"+disposition)
	f.mu.Unlock()
	return nil
}

func (f *fakeBenchmark) SetOptIn(_ context.Context, actor, subject string, enabled bool) error {
	f.note("optin")
	f.mu.Lock()
	f.optIns = append(f.optIns, actor+"/"+subject)
	f.optedIn = enabled
	f.mu.Unlock()
	return nil
}

func (f *fakeBenchmark) OptedIn(_ context.Context, userID string) (bool, error) {
	f.note("optedin")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.optedInFor = append(f.optedInFor, userID)
	if f.optedInErr != nil {
		return false, f.optedInErr
	}
	return f.optedIn, nil
}

// AnswerVocabulary stands in for the package's registered answer vocabularies.
// The values are STAND-INS on purpose: internal/api names no registered string
// and this test must not become the place one appears. What it proves is that
// the served lists are whatever the seam handed over — the real vocabulary is
// pinned to the constants on the package's own side of the wall.
func (f *fakeBenchmark) AnswerVocabulary(context.Context) (api.BenchmarkVocabulary, error) {
	f.note("vocabulary")
	return api.BenchmarkVocabulary{
		Choices:      []string{"stand-in-choice-1", "stand-in-choice-2"},
		GuessSides:   []string{"stand-in-side"},
		Dispositions: []string{"stand-in-disposition"},
	}, nil
}

// RegisteredValues stands in for the package's S18.4 R9 block. It carries the
// real marker string because that is the property the settings surface must
// serve verbatim; the value itself is a stand-in, because internal/api names no
// registered number and this test must not become the place one appears.
func (f *fakeBenchmark) RegisteredValues(context.Context) (json.RawMessage, error) {
	f.note("registered")
	return json.RawMessage(`[{"name":"stand-in","value":"stand-in","ref":"BENCH-REG §1",` +
		`"marker":"registered — changing this value requires a re-registration commit"}]`), nil
}

func (f *fakeBenchmark) order() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.calls...)
}

// benchEnv is a real migrated DB (the pair rows and the alarm events the
// transport scopes against) plus the fake practice behind the seam.
type benchEnv struct {
	b    *backend
	fake *fakeBenchmark
}

func newBenchEnv(t *testing.T) *benchEnv {
	t.Helper()
	b := newBackend(t)
	seedUser(t, b, "op", auth.RoleOperator)
	seedUser(t, b, "alice", auth.RoleMember)
	seedUser(t, b, "bob", auth.RoleMember)
	seedTask(t, b, "t-alice", "alice", "Alice Task", "doing")
	exec(t, b, `INSERT INTO benchmark_pairs
	    (pair_id, user_id, domain, task_id, deliverable_id, phase, rate_pct, sampled_ts, state, render_a, render_b, updated_ts)
	    VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		"bp-alice", "alice", "software-development", "t-alice", "d-1", "pre-gate", 100,
		nowTS(), "rendered", "aaa", "bbb", nowTS())
	return &benchEnv{b: b, fake: &fakeBenchmark{}}
}

func (e *benchEnv) do(t *testing.T, who, method, path, body string) (int, string) {
	t.Helper()
	srv := api.New(api.Config{
		Log: e.b.log, Sessions: e.b.store, Auth: fixedIdentity{who},
		Settings: fixedSettings{d: 20 * 1e9},
		HealthFn: func() api.Health { return api.Health{Ready: true} },
		DB:       e.b.db, Benchmark: e.fake,
	})
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rr.Code, rr.Body.String()
}

func (e *benchEnv) mustDo(t *testing.T, who, method, path, body string) string {
	t.Helper()
	code, out := e.do(t, who, method, path, body)
	if code < 200 || code > 299 {
		t.Fatalf("%s %s as %s: status %d: %s", method, path, who, code, out)
	}
	return out
}

// ── the §3.4 order at the transport (rubric 17) ─────────────────────────────

// TestTransportRecordsBeforeItReveals: the handler asks for the record FIRST and
// the reveal only after it returned — so a reveal can never precede the record
// that entitles it. Asserted on the call ORDER rather than on the response,
// because the response would look identical either way.
func TestTransportRecordsBeforeItReveals(t *testing.T) {
	e := newBenchEnv(t)
	out := e.mustDo(t, "alice", "POST", "/api/approvals/benchmark_verdict:bp-alice/verdict",
		`{"verdict":"A","guess":"B"}`)

	if got := e.fake.order(); len(got) != 2 || got[0] != "record" || got[1] != "reveal" {
		t.Fatalf("call order %v — BENCH-REG §3.4 freezes record-then-reveal", got)
	}
	if len(e.fake.verdicts) != 1 || e.fake.verdicts[0] != "bp-alice/A/B" {
		t.Fatalf("the verdict did not travel verbatim: %v", e.fake.verdicts)
	}
	if !strings.Contains(out, "platform_side") {
		t.Errorf("the response does not carry the committed reveal: %s", out)
	}
}

// TestRefusedVerdictRevealsNothing: a refused verdict — the guess-less case
// above all — fires nothing AND asks for no reveal. There is no record, so there
// is nothing arm identity could be read out of.
func TestRefusedVerdictRevealsNothing(t *testing.T) {
	e := newBenchEnv(t)
	e.fake.recordErr = &api.SurfaceError{Status: http.StatusBadRequest, Code: "guess_required",
		Msg: "a verdict requires the arm-guess in the same form (BENCH-REG §3.3)"}

	code, out := e.do(t, "alice", "POST", "/api/approvals/benchmark_verdict:bp-alice/verdict", `{"verdict":"A"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("guess-less verdict: status %d, want 400: %s", code, out)
	}
	if !strings.Contains(out, "guess_required") {
		t.Errorf("the refusal does not name the rule: %s", out)
	}
	if got := e.fake.order(); len(got) != 1 || got[0] != "record" {
		t.Fatalf("a refused verdict produced calls %v — no reveal may follow a refusal", got)
	}
}

// ── scope, three ways per verb (R21) ────────────────────────────────────────

// TestBenchmarkVerbsAreScopedThreeWays drives every new pair-scoped verb as the
// owner, another member and the operator, in the non-tautological direction: the
// OWNER's own act must succeed, or an all-refusing handler would pass.
func TestBenchmarkVerbsAreScopedThreeWays(t *testing.T) {
	for _, verb := range []string{"verdict", "decline"} {
		t.Run(verb, func(t *testing.T) {
			body := `{}`
			if verb == "verdict" {
				body = `{"verdict":"A","guess":"B"}`
			}
			path := "/api/approvals/benchmark_verdict:bp-alice/" + verb

			// The owner: SUCCEEDS.
			e := newBenchEnv(t)
			e.mustDo(t, "alice", "POST", path, body)

			// Another member: refused, and nothing fired.
			e = newBenchEnv(t)
			if code, _ := e.do(t, "bob", "POST", path, body); code != http.StatusForbidden {
				t.Errorf("another member's %s: status %d, want 403", verb, code)
			}
			if got := e.fake.order(); len(got) != 0 {
				t.Errorf("a refused %s reached the practice: %v", verb, got)
			}

			// The operator: refused too — the vote is not delegable.
			e = newBenchEnv(t)
			if code, _ := e.do(t, "op", "POST", path, body); code != http.StatusForbidden {
				t.Errorf("the operator's %s: status %d, want 403", verb, code)
			}
			if got := e.fake.order(); len(got) != 0 {
				t.Errorf("a refused operator %s reached the practice: %v", verb, got)
			}

			// An unknown pair is 404, never an existence oracle.
			e = newBenchEnv(t)
			if code, _ := e.do(t, "alice", "POST", "/api/approvals/benchmark_verdict:bp-nobody/"+verb, body); code != http.StatusNotFound {
				t.Errorf("unknown pair %s: status %d, want 404", verb, code)
			}
		})
	}
}

// TestPendingListIsRequesterScoped: a member is asked for by name; the operator
// is asked for with an empty scope, which is the landed inbox derivation's own
// rule (they see all, they own none).
func TestPendingListIsRequesterScoped(t *testing.T) {
	e := newBenchEnv(t)
	e.mustDo(t, "alice", "GET", "/api/benchmark/verdicts", "")
	e.mustDo(t, "op", "GET", "/api/benchmark/verdicts", "")
	if len(e.fake.requesters) != 2 || e.fake.requesters[0] != "alice" || e.fake.requesters[1] != "" {
		t.Fatalf("pending list scopes = %q — a member is scoped by name, the operator sees all", e.fake.requesters)
	}
}

// TestServedOptInIsTheCallersOwnInBothPostures: the pair list widens for the
// operator and the consent block does NOT. Consent is self-only in both
// directions (§4.2.1), so a role bit that opens every pending pair still opens
// nobody's consent but the reader's — asserted on WHO the read was asked about,
// which is the thing a widening would have to change.
func TestServedOptInIsTheCallersOwnInBothPostures(t *testing.T) {
	e := newBenchEnv(t)
	e.mustDo(t, "alice", "GET", "/api/benchmark/verdicts", "")
	e.mustDo(t, "op", "GET", "/api/benchmark/verdicts", "")
	if len(e.fake.optedInFor) != 2 || e.fake.optedInFor[0] != "alice" || e.fake.optedInFor[1] != "op" {
		t.Fatalf("consent read for %q — each caller reads their OWN, and the operator's read is not widened", e.fake.optedInFor)
	}
}

// TestServedOptInFollowsTheStoreAndNotTheVerb is the probe that a hardcoded
// field cannot pass: the position is written BEHIND THE HANDLER'S BACK and the
// next read has to follow it.
//
// This is the §48 lesson applied to a second control. A block that only ever
// reported what the last flip said would be a memory of an act, and the one
// thing it could never tell you is what is true now — which is precisely what a
// standing consent has to say.
func TestServedOptInFollowsTheStoreAndNotTheVerb(t *testing.T) {
	e := newBenchEnv(t)

	// Nothing has been flipped: the served position is OFF, and it is a
	// position rather than an absence.
	var before api.BenchmarkVerdictForms
	decodeBody(t, e.mustDo(t, "alice", "GET", "/api/benchmark/verdicts", ""), &before)
	if before.OptIn.Enabled == nil || *before.OptIn.Enabled {
		t.Fatalf("the default consent did not read as an explicit OFF: %+v", before.OptIn)
	}
	if before.OptIn.Absent != "" {
		t.Errorf("a readable position carried an absence too: %q", before.OptIn.Absent)
	}

	// Moved in the store, with no verb involved.
	e.fake.mu.Lock()
	e.fake.optedIn = true
	e.fake.mu.Unlock()

	var after api.BenchmarkVerdictForms
	decodeBody(t, e.mustDo(t, "alice", "GET", "/api/benchmark/verdicts", ""), &after)
	if after.OptIn.Enabled == nil || !*after.OptIn.Enabled {
		t.Fatalf("the served consent did not follow the store: %+v — this block is a READING, not a memory of the last flip", after.OptIn)
	}
}

// TestUnreadableOptInIsAnAbsenceAndNeverAnOff: a read that broke must not
// degrade into "not opted in". Consent nobody gave is the one answer this block
// may never invent.
func TestUnreadableOptInIsAnAbsenceAndNeverAnOff(t *testing.T) {
	e := newBenchEnv(t)
	e.fake.optedInErr = errors.New("the users table is unreadable")

	var got api.BenchmarkVerdictForms
	decodeBody(t, e.mustDo(t, "alice", "GET", "/api/benchmark/verdicts", ""), &got)
	if got.OptIn.Enabled != nil {
		t.Fatalf("a failed read still served a consent position: %+v", got.OptIn)
	}
	if !strings.Contains(got.OptIn.Absent, "the users table is unreadable") {
		t.Errorf("the absence does not carry the read's own reason: %q", got.OptIn.Absent)
	}
	// The rest of the response is unaffected: one member's absence does not take
	// the pending pairs down with it.
	if len(got.Pairs) == 0 {
		t.Error("an unreadable consent suppressed the pairs, which are a different read")
	}
}

// decodeBody reads a served response into its own wire type, so an assertion
// is about the SHAPE production serves rather than about a substring.
func decodeBody(t *testing.T, raw string, into any) {
	t.Helper()
	if err := json.Unmarshal([]byte(raw), into); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
}

// TestPendingFormShapeCarriesNoArmIdentity: the served shape is the package's
// own, so the blindness property travels intact. The scan is over the WIRE.
func TestPendingFormShapeCarriesNoArmIdentity(t *testing.T) {
	e := newBenchEnv(t)
	out := e.mustDo(t, "alice", "GET", "/api/benchmark/verdicts", "")
	// The scan is over the PAIRS document, not the response prose: the wrapper's
	// explanatory detail necessarily talks about arms and guesses, and scanning
	// English would make the assertion about wording rather than about data.
	var resp struct {
		Pairs json.RawMessage `json:"pairs"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("decode: %v: %s", err, out)
	}
	pairs := string(resp.Pairs)
	for _, forbidden := range []string{"position", "platform-a", "platform-b", "direct_run", "platform_run", "_model", "\"arm"} {
		if strings.Contains(pairs, forbidden) {
			t.Errorf("the blind form leaks %q (BENCH-REG §3.4): %s", forbidden, pairs)
		}
	}
	for _, required := range []string{"render_a", "render_b"} {
		if !strings.Contains(pairs, required) {
			t.Fatalf("the form is missing %q — the scan above would pass vacuously", required)
		}
	}
	if !strings.Contains(out, "guess_required") {
		t.Fatal("the form does not carry the frozen mandatory-guess fact")
	}
}

// TestAlarmDisposeIsOperatorOnly: alarms are platform-scope, so a member's own
// derivation returns none and asking it is both the authority check and the
// existence check — the same shape the part-B card verbs use.
func TestAlarmDisposeIsOperatorOnly(t *testing.T) {
	e := newBenchEnv(t)
	appendPlatformPayload(t, e.b, "benchmark.alarm",
		`{"action":"raise","domain":"software-development","epoch_id":"e1","severity":"flag-now","summary":"S","loss_g":0.96,"threshold":0.95,"expansion_freeze":true}`)
	const path = "/api/approvals/benchmark_alarm:software-development#e1/dispose"

	if code, _ := e.do(t, "alice", "POST", path, `{"disposition":"investigate"}`); code != http.StatusNotFound {
		t.Errorf("a member dispositioning an alarm: status %d, want 404", code)
	}
	if got := e.fake.order(); len(got) != 0 {
		t.Fatalf("a refused disposition reached the practice: %v", got)
	}
	e.mustDo(t, "op", "POST", path, `{"disposition":"investigate","reason":"why"}`)
	if len(e.fake.disposes) != 1 || e.fake.disposes[0] != "op/software-development/investigate" {
		t.Fatalf("the disposition did not travel verbatim with its actor: %v", e.fake.disposes)
	}
	// A card id of the wrong kind is a 400, not a mystery 404.
	if code, _ := e.do(t, "op", "POST", "/api/approvals/drift_card:fp-1/dispose", `{"disposition":"investigate"}`); code != http.StatusBadRequest {
		t.Errorf("dispose on a drift card: status %d, want 400", code)
	}
	// The alarm card id carries a `#` (domain#epoch, as the landed projection
	// mints it), so a real client percent-encodes it. That form must route and
	// resolve to the same domain — otherwise the card the inbox renders would be
	// unusable from a browser and only from a test.
	e2 := newBenchEnv(t)
	appendPlatformPayload(t, e2.b, "benchmark.alarm",
		`{"action":"raise","domain":"software-development","epoch_id":"e1","severity":"flag-now","summary":"S","loss_g":0.96,"threshold":0.95,"expansion_freeze":true}`)
	e2.mustDo(t, "op", "POST", "/api/approvals/benchmark_alarm:software-development%23e1/dispose",
		`{"disposition":"investigate"}`)
	if len(e2.fake.disposes) != 1 || e2.fake.disposes[0] != "op/software-development/investigate" {
		t.Fatalf("the percent-encoded card id did not resolve: %v", e2.fake.disposes)
	}
}

// TestOptInIsSelfOnlyAtTheTransport: consent is not delegable, and the operator
// is refused by the same sentence as anybody else (OQ10; §35).
func TestOptInIsSelfOnlyAtTheTransport(t *testing.T) {
	e := newBenchEnv(t)
	if code, out := e.do(t, "op", "POST", "/api/benchmark/opt-in", `{"person":"alice","enabled":true}`); code != http.StatusForbidden {
		t.Fatalf("the operator setting another person's consent: status %d, want 403: %s", code, out)
	}
	if code, _ := e.do(t, "alice", "POST", "/api/benchmark/opt-in", `{"person":"bob","enabled":true}`); code != http.StatusForbidden {
		t.Errorf("a member setting another person's consent: status %d, want 403", code)
	}
	if got := e.fake.order(); len(got) != 0 {
		t.Fatalf("a refused consent flip reached the practice: %v", got)
	}
	// Your own consent works — both named explicitly and left implicit.
	e.mustDo(t, "alice", "POST", "/api/benchmark/opt-in", `{"enabled":true}`)
	e.mustDo(t, "alice", "POST", "/api/benchmark/opt-in", `{"person":"alice","enabled":false}`)
	if len(e.fake.optIns) != 2 {
		t.Fatalf("own-consent flips reached the practice %d times, want 2", len(e.fake.optIns))
	}
	for _, got := range e.fake.optIns {
		if got != "alice/alice" {
			t.Errorf("the consent flip's actor and subject disagree: %q", got)
		}
	}
}

// ── audit (rubric 18) ───────────────────────────────────────────────────────

// TestBenchmarkVerbsMintNoTransportDecisionRow: every act on these routes has a
// landed canonical row of its own — the §14 pair record for a verdict and a
// decline, and the family-5 rows DisposeAlarm and SetOptIn write INSIDE their
// own transactions. So the transport mints nothing (OQ8): a second row would
// make one decision look like two.
func TestBenchmarkVerbsMintNoTransportDecisionRow(t *testing.T) {
	e := newBenchEnv(t)
	appendPlatformPayload(t, e.b, "benchmark.alarm",
		`{"action":"raise","domain":"software-development","epoch_id":"e1","severity":"flag-now","summary":"S","loss_g":0.96,"threshold":0.95,"expansion_freeze":true}`)

	before := len(decisionRows(t, e.b, ""))
	e.mustDo(t, "alice", "POST", "/api/approvals/benchmark_verdict:bp-alice/verdict", `{"verdict":"A","guess":"B"}`)
	e.mustDo(t, "alice", "POST", "/api/approvals/benchmark_verdict:bp-alice/decline", `{}`)
	e.mustDo(t, "op", "POST", "/api/approvals/benchmark_alarm:software-development#e1/dispose", `{"disposition":"investigate"}`)
	e.mustDo(t, "alice", "POST", "/api/benchmark/opt-in", `{"enabled":true}`)
	e.mustDo(t, "alice", "GET", "/api/benchmark/verdicts", "")

	if after := len(decisionRows(t, e.b, "")); after != before {
		t.Fatalf("the transport minted %d decision.recorded rows — every part-C act carries its own canonical row (OQ8)",
			after-before)
	}
	// The whole route set was exercised, so the assertion above is not vacuous.
	//
	// MOVED 7 → 8 (P3-UI-3): the verdicts GET makes a THIRD seam call, for the
	// caller's own standing consent. It is a READ and mints nothing, which is
	// what the assertion above proves about it — but the walk has to know it
	// happened, or a read that silently stopped being made would still pass.
	if got := len(e.fake.order()); got != 8 {
		t.Fatalf("the audit walk drove %d practice calls, want 8 (record+reveal, decline, dispose, optin, pending+vocabulary+optedin)", got)
	}
}

// TestBenchmarkRoutesAreUnwiredHonestly: with no practice composed, every route
// says so rather than pretending.
func TestBenchmarkRoutesAreUnwiredHonestly(t *testing.T) {
	b := newBackend(t)
	seedUser(t, b, "alice", auth.RoleMember)
	srv := api.New(api.Config{
		Log: b.log, Sessions: b.store, Auth: fixedIdentity{"alice"},
		Settings: fixedSettings{d: 20 * 1e9},
		HealthFn: func() api.Health { return api.Health{Ready: true} },
		DB:       b.db, // Benchmark deliberately nil
	})
	for _, r := range []struct{ method, path, body string }{
		{"GET", "/api/benchmark/verdicts", ""},
		{"POST", "/api/benchmark/opt-in", `{"enabled":true}`},
		{"POST", "/api/approvals/benchmark_verdict:bp-alice/verdict", `{"verdict":"A","guess":"B"}`},
		{"POST", "/api/approvals/benchmark_verdict:bp-alice/decline", `{}`},
		{"POST", "/api/approvals/benchmark_alarm:D#e1/dispose", `{"disposition":"investigate"}`},
	} {
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, httptest.NewRequest(r.method, r.path, strings.NewReader(r.body)))
		if rr.Code != http.StatusServiceUnavailable {
			t.Errorf("%s %s unwired: status %d, want 503", r.method, r.path, rr.Code)
		}
	}
}

// ── the negative that §35 turns on (rubric 17) ──────────────────────────────

// TestNoRegisteredNumberIsRestatedInTheAPI: BENCH-REG's registered values are
// read-only data in internal/benchmark, pinned to the registration file itself.
// A copy anywhere in internal/api would be a second place a frozen number could
// drift — and drift between the registration and the running platform is a
// platform defect by BENCH-REG's own clause (§17). The scan is over every
// non-test source file of this package.
func TestNoRegisteredNumberIsRestatedInTheAPI(t *testing.T) {
	// The registered values and labels, by their exact rendered forms. They are
	// written here — in a TEST — precisely because the point is that they must
	// not appear in the package's own source.
	banned := []string{
		"0.90", "0.95",
		"m >= 20", "m ≥ 20", "n=20",
		"direct-use estimate (heuristic)",
		"partially unblinded",
		"benchmark-preregistration-v1",
		"software-development", "web-research",
		"investigate", "fix-and-continue-accruing", "re-register",
		"both-bad",
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	scanned := 0
	for _, ent := range entries {
		n := ent.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		src, err := os.ReadFile(n)
		if err != nil {
			t.Fatal(err)
		}
		scanned++
		for _, bad := range banned {
			if strings.Contains(string(src), bad) {
				t.Errorf("internal/api/%s contains the registered value/label %q — registered numbers live in internal/benchmark's registered.go and nowhere else (BENCH-REG §17; §35)", n, bad)
			}
		}
	}
	if scanned == 0 {
		t.Fatal("the scan read no files — it would pass vacuously")
	}
	// Non-tautological: the scan's matcher must catch a planted restatement.
	planted := "const alarmThreshold = 0.95"
	hit := false
	for _, bad := range banned {
		if strings.Contains(planted, bad) {
			hit = true
		}
	}
	if !hit {
		t.Fatal("the matcher does not catch a planted registered value — the scan guarantees nothing")
	}
}

// TestPartCShapesNeverPercent extends the standing §30 R12 scan over the shapes
// this packet adds: no percent, fraction, ratio or ETA field anywhere in them.
func TestPartCShapesNeverPercent(t *testing.T) {
	e := newBenchEnv(t)
	appendPlatformPayload(t, e.b, "benchmark.alarm",
		`{"action":"raise","domain":"software-development","epoch_id":"e1","severity":"flag-now","summary":"S","loss_g":0.96,"threshold":0.95,"expansion_freeze":true}`)

	for _, body := range []string{
		e.mustDo(t, "alice", "GET", "/api/benchmark/verdicts", ""),
		e.mustDo(t, "alice", "POST", "/api/approvals/benchmark_verdict:bp-alice/verdict", `{"verdict":"A","guess":"B"}`),
		e.mustDo(t, "alice", "POST", "/api/approvals/benchmark_verdict:bp-alice/decline", `{}`),
		e.mustDo(t, "op", "POST", "/api/approvals/benchmark_alarm:software-development#e1/dispose", `{"disposition":"investigate"}`),
		e.mustDo(t, "alice", "POST", "/api/benchmark/opt-in", `{"enabled":true}`),
	} {
		var any map[string]json.RawMessage
		if err := json.Unmarshal([]byte(body), &any); err != nil {
			t.Fatalf("decode %s: %v", body, err)
		}
		for key := range any {
			k := strings.ToLower(key)
			// "eta" is matched as a WHOLE token, not as a substring: a field named
			// `detail` contains the letters and means nothing of the kind.
			if k == "eta" || strings.HasSuffix(k, "_eta") || strings.HasPrefix(k, "eta_") {
				t.Errorf("part-C shape carries an ETA field %q (§30 R12)", key)
			}
			for _, bad := range []string{"_pct", "percent", "_ratio", "_fraction", "utiliz", "progress"} {
				if strings.Contains(k, bad) {
					t.Errorf("part-C shape carries %q — no percent, fraction or ETA field may exist (§30 R12)", key)
				}
			}
		}
	}
}

// ── counters (rubric 20) ───────────────────────────────────────────────────

// TestPartCCountersArePinned holds the tally this packet is allowed to move and
// the ones it is not: the ⚙ registry is byte-unchanged (118/33), and exactly ONE
// migration is added — 0018 — with user_version following it.
//
// The ADOPTION LOCK is deliberately NOT asserted here (drain r1, D4). `go run
// ./tools/lockgate` is the real gate and it checks coverage in both directions;
// a hand-counted "21" in this file would be a second, weaker authority that
// could pass while the real gate failed. The count belongs to lockgate.
func TestPartCCountersArePinned(t *testing.T) {
	reg := settings.New()
	decls := reg.Decls()
	if len(decls) != 118 {
		t.Errorf("⚙ index has %d keys, want 118 — B6-2C adds no key (R24)", len(decls))
	}
	domains := map[string]bool{}
	for _, d := range decls {
		domains[d.Domain()] = true
	}
	if len(domains) != 33 {
		t.Errorf("⚙ index has %d domains, want 33", len(domains))
	}
	b := newBackend(t)
	v, err := b.db.UserVersion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v != 24 {
		t.Errorf("user_version = %d, want 24 (0001–0017 untouched, 0018 is this packet's, 0019 is B6-3A's, 0020 is B6-7's, 0021 is B6-9's, 0022 is P3-RW-3's, 0023 is P3-RW-7's, 0024 is P3-RW-11's)", v)
	}
	// 0018 is the ONLY migration this packet adds; 0019 is B6-3A's (the S10.3
	// price table's durable home) and moves this sentinel in lockstep, as do
	// 0020/0021, P3-RW-3's 0022 (the pre-approval project-attribution view
	// re-create), P3-RW-7's 0023 (the onboarding arm on the same edge) and
	// P3-RW-11's 0024 (the capture's task family).
	sqls, err := filepath.Glob("../storage/migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range sqls {
		if filepath.Base(p) > "0024_zzz" {
			t.Errorf("unexpected migration %q — part C adds exactly 0018, B6-3A exactly 0019, B6-7 exactly 0020, P3-RW-3 exactly 0022, P3-RW-7 exactly 0023 and P3-RW-11 exactly 0024", filepath.Base(p))
		}
	}
}

// ── drain r1: the eighth kind (D2) ──────────────────────────────────────────

// TestTerminalRunStatesAgreeWithTheFSM: the failed-pair derivation needs the
// terminal-state set inside a SQL-shaped filter, so internal/api mirrors it. The
// mirror is CROSS-CHECKED against run.IsTerminal here — the one definition of
// record — over the whole stored vocabulary, so an FSM edge added later fails
// loudly instead of silently changing which pairs read as failed.
func TestTerminalRunStatesAgreeWithTheFSM(t *testing.T) {
	stored := []run.State{
		run.StateNew, run.StateQueued, run.StateClaimed, run.StateRunning, run.StateParked,
		run.StateDraining, run.StateCompleted, run.StateCrashed, run.StateFinalized,
		run.StateTombstoned, run.StateDiedAtGate,
	}
	mirrored := map[string]bool{}
	for _, s := range api.TerminalRunStatesForTest() {
		mirrored[s] = true
	}
	for _, st := range stored {
		if want, got := run.IsTerminal(st), mirrored[string(st)]; want != got {
			t.Errorf("state %q: run.IsTerminal=%v but the api mirror says %v", st, want, got)
		}
	}
	// `crashed` is deliberately excluded on BOTH sides: recovery owns a crashed
	// run's disposition, so a crashed arm's pair is still being decided.
	if mirrored[string(run.StateCrashed)] {
		t.Error("the mirror treats `crashed` as terminal — recovery still owns that run")
	}
}

// TestFailedPairCardIsRequesterOwnedAndOffersTheDecline: the eighth kind is a
// pure derivation over existing rows and carries the same authority rule as the
// verdict card it replaces — the requester closes it, the operator sees it.
func TestFailedPairCardIsRequesterOwnedAndOffersTheDecline(t *testing.T) {
	e := newBenchEnv(t)
	// A dispatched pair whose direct arm ended terminal with NO capture.
	exec(t, e.b, `INSERT INTO benchmark_pairs
	    (pair_id, user_id, domain, task_id, deliverable_id, phase, rate_pct, sampled_ts, state, direct_run_id, updated_ts)
	    VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		"bp-stranded", "alice", "software-development", "t-alice", "d-2", "pre-gate", 100,
		nowTS(), "dispatched", "bp-stranded.direct", nowTS())
	seedRun(t, e.b, "bp-stranded.direct", "alice", "t-alice", "finalized", "anthropic")

	card := findCard(t, e, "alice", "benchmark_verdict:bp-stranded")
	if !card.Answerable {
		t.Error("the requester cannot close their own stranded pair")
	}
	if len(card.Actions) != 1 || card.Actions[0] != "decline" {
		t.Errorf("the card offers %v, want only the landed decline", card.Actions)
	}
	if card.Tier != "low" {
		t.Errorf("the card is tier %q — nothing is at risk and nothing is blocked", card.Tier)
	}
	if card.Batchable || card.StepUpRequired {
		t.Error("closing a failed pair claims a batch or a step-up neither verb implements")
	}
	if !strings.Contains(string(card.Card), "finalized") {
		t.Errorf("the card does not carry the arm's own terminal state: %s", card.Card)
	}
	// The operator SEES it and cannot close it; another member sees nothing.
	if op := findCard(t, e, "op", "benchmark_verdict:bp-stranded"); op.Answerable {
		t.Error("the operator's copy reads answerable — closing it is the requester's act")
	}
	for _, it := range listItems(t, e, "bob") {
		if it.ID == "benchmark_verdict:bp-stranded" {
			t.Error("another member sees alice's stranded pair")
		}
	}
	// A pair whose arm is still CRASHED is NOT a card yet: recovery owns it.
	exec(t, e.b, `UPDATE runs SET state = 'crashed' WHERE run_id = ?`, "bp-stranded.direct")
	for _, it := range listItems(t, e, "alice") {
		if it.ID == "benchmark_verdict:bp-stranded" {
			t.Error("a pair whose arm is still crashed shows a failed card — recovery has not dispositioned it yet")
		}
	}
}

func listItems(t *testing.T, e *benchEnv, who string) []api.ApprovalItem {
	t.Helper()
	var list api.ApprovalList
	if err := json.Unmarshal([]byte(e.mustDo(t, who, "GET", "/api/approvals", "")), &list); err != nil {
		t.Fatalf("decode approvals: %v", err)
	}
	return list.Items
}

func findCard(t *testing.T, e *benchEnv, who, id string) api.ApprovalItem {
	t.Helper()
	for _, it := range listItems(t, e, who) {
		if it.ID == id {
			return it
		}
	}
	t.Fatalf("%s sees no card %q", who, id)
	return api.ApprovalItem{}
}
