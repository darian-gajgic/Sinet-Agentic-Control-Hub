package shell

// benchmark_verbs_test.go — the B6-2C verdict backend over the REAL practice
// (P3-B6-2C, R20). internal/api may not import internal/benchmark in either
// direction, so its own battery runs on fakes; the composition root is the only
// place the HTTP verb, the adapter and the registered protocol can be driven
// together, and the §3.4 ordering property is only real here.
//
// $0 and hermetic: no engine, no network, no seat.

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/benchmark"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
)

// do issues an authenticated request as one identity against a server composed
// exactly as shell.Run composes it — the real seam over the real practice.
func (e *driverEnv) do(t *testing.T, who, method, path, body string) (int, string) {
	t.Helper()
	srv := api.New(api.Config{
		Log: e.log, Sessions: auth.New(e.db, e.log),
		Auth: fixedShellIdentity{who}, Settings: settings.New(),
		HealthFn:  func() api.Health { return api.Health{Ready: true} },
		DB:        e.db,
		Benchmark: benchmarkVerbSeam(e.bs),
	})
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rr.Code, rr.Body.String()
}

func (e *driverEnv) mustDo(t *testing.T, who, method, path, body string) string {
	t.Helper()
	code, out := e.do(t, who, method, path, body)
	if code < 200 || code > 299 {
		t.Fatalf("%s %s as %s: status %d: %s", method, path, who, code, out)
	}
	return out
}

// operator promotes a fixture user to the one S01.9 role bit, which is what the
// package's §12 check reads out of the data layer.
func (e *driverEnv) operator(t *testing.T, id string) {
	t.Helper()
	ctx := context.Background()
	if err := e.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO users (user_id, display_name, role, created_ts) VALUES (?,?,?,?)
			 ON CONFLICT (user_id) DO UPDATE SET role = excluded.role`,
			id, id, "operator", time.Now().UTC().Format(time.RFC3339Nano))
		return err
	}); err != nil {
		t.Fatalf("seed operator: %v", err)
	}
}

// rendered drives one pair all the way to the state its verdict form is served
// from, through the real driver and the real dispatcher.
func (e *driverEnv) rendered(t *testing.T, pairID, taskID, owner string) benchmark.Pair {
	t.Helper()
	p := e.seedPair(t, pairID, taskID, owner)
	e.pass(t)
	e.claim(t)
	e.pass(t)
	got := e.pair(t, p.PairID)
	if got.State != benchmark.StateRendered {
		t.Fatalf("fixture pair is %s, want rendered", got.State)
	}
	return got
}

func (e *driverEnv) recordedEvents(t *testing.T, typ, pairID string) int {
	t.Helper()
	var n int
	if err := e.db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM run_events WHERE type = ? AND json_extract(payload, '$.pair_id') = ?`,
		typ, pairID).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", typ, err)
	}
	return n
}

// ── the §3.4 order (rubric 17) ──────────────────────────────────────────────

// TestRecordCommitsBeforeAnyRevealIsReadable is the ordering §3.4 freezes, made
// a transport property. It is probed in BOTH directions: a reveal asked for
// before the record fails, and after the verdict POST the record row and the
// benchmark.pair_recorded event both exist — so the reveal in the response is a
// read of something committed, never a flag flipped on a mutable row.
func TestRecordCommitsBeforeAnyRevealIsReadable(t *testing.T) {
	e := newDriverEnv(t, &driverEngine{text: driverArmAnswer})
	pair := e.rendered(t, "bp-10", "t-10", "alice")
	ctx := context.Background()

	// BEFORE: there is nothing to reveal, and the package says so.
	if _, err := e.bs.Practice.Reveal(ctx, pair.PairID); err == nil {
		t.Fatal("a pair with no committed record revealed its arms")
	}
	if n := e.recordedEvents(t, benchmark.EventPairRecorded, pair.PairID); n != 0 {
		t.Fatalf("%d record events exist before the vote", n)
	}

	out := e.mustDo(t, "alice", "POST", "/api/approvals/benchmark_verdict:"+pair.PairID+"/verdict",
		`{"verdict":"A","guess":"A"}`)

	// AFTER: the row moved and the record event exists — both committed by the
	// one WriteTx the package uses — and only then does the response carry a
	// reveal.
	if got := e.pair(t, pair.PairID); got.State != benchmark.StateRecorded {
		t.Fatalf("the pair is %s after the vote, want recorded", got.State)
	}
	if n := e.recordedEvents(t, benchmark.EventPairRecorded, pair.PairID); n != 1 {
		t.Fatalf("%d record events after one vote, want exactly 1", n)
	}
	var resp struct {
		Recorded bool            `json:"recorded"`
		Reveal   json.RawMessage `json:"reveal"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("decode: %v: %s", err, out)
	}
	if !resp.Recorded || len(resp.Reveal) == 0 {
		t.Fatalf("the response does not carry the committed reveal: %s", out)
	}
	if !strings.Contains(string(resp.Reveal), "platform_side") {
		t.Errorf("the reveal does not name which side was the platform's: %s", resp.Reveal)
	}
	// The reveal is a READ of that record: asking again returns the same thing.
	rec, err := e.bs.Practice.Reveal(ctx, pair.PairID)
	if err != nil {
		t.Fatalf("Reveal after the record: %v", err)
	}
	if rec.PairID != pair.PairID || rec.Verdict != "A" {
		t.Errorf("the committed record does not match the vote: %+v", rec)
	}
}

// TestGuessLessVerdictCannotBeExpressed: §3.3 is frozen — "No verdict without a
// guess; the guess is never optional." The refusal is STRUCTURAL (NewAnswer will
// not build the argument), so the transport cannot allow one even by accident,
// and NOTHING is recorded when it is attempted.
func TestGuessLessVerdictCannotBeExpressed(t *testing.T) {
	e := newDriverEnv(t, &driverEngine{text: driverArmAnswer})
	pair := e.rendered(t, "bp-11", "t-11", "alice")

	for _, body := range []string{
		`{"verdict":"A"}`,
		`{"verdict":"A","guess":""}`,
		`{"verdict":"tie","guess":"C"}`,
	} {
		code, out := e.do(t, "alice", "POST", "/api/approvals/benchmark_verdict:"+pair.PairID+"/verdict", body)
		if code != http.StatusBadRequest {
			t.Fatalf("guess-less vote %s: status %d, want 400: %s", body, code, out)
		}
		if !strings.Contains(out, "guess_required") {
			t.Errorf("the refusal does not name the frozen rule: %s", out)
		}
	}
	// Nothing fired: the pair still awaits its verdict and no record exists.
	if got := e.pair(t, pair.PairID); got.State != benchmark.StateRendered {
		t.Errorf("a refused vote moved the pair to %s", got.State)
	}
	if n := e.recordedEvents(t, benchmark.EventPairRecorded, pair.PairID); n != 0 {
		t.Errorf("%d records were written by a refused vote", n)
	}
	// A verdict outside the registered vocabulary is refused by the PACKAGE
	// against the registration, not by a copy of the vocabulary in the API.
	code, _ := e.do(t, "alice", "POST", "/api/approvals/benchmark_verdict:"+pair.PairID+"/verdict",
		`{"verdict":"maybe","guess":"A"}`)
	if code != http.StatusBadRequest {
		t.Errorf("an unregistered verdict value: status %d, want 400", code)
	}
}

// TestVerdictIsTheRequestersAndNotDelegable: the operator SEES every pending
// card (S01.9) and may vote NONE of them. Visibility is not authorship.
func TestVerdictIsTheRequestersAndNotDelegable(t *testing.T) {
	e := newDriverEnv(t, &driverEngine{text: driverArmAnswer})
	e.operator(t, "op")
	pair := e.rendered(t, "bp-12", "t-12", "alice")
	e.seedPair(t, "bp-13", "t-13", "bob") // another member, never rendered

	// The operator reads the pending list and sees alice's card...
	out := e.mustDo(t, "op", "GET", "/api/benchmark/verdicts", "")
	if !strings.Contains(out, pair.PairID) {
		t.Fatalf("the operator's pending list is missing the household's card: %s", out)
	}
	// ...and is refused the vote.
	code, body := e.do(t, "op", "POST", "/api/approvals/benchmark_verdict:"+pair.PairID+"/verdict",
		`{"verdict":"A","guess":"A"}`)
	if code != http.StatusForbidden {
		t.Fatalf("operator vote: status %d, want 403: %s", code, body)
	}
	// Another member is refused the same way — 403, not 404, because the PAIR
	// exists and the house rule is 404-for-missing / 403-for-another-owner
	// (`authorizeOwner`'s shape). It is their READ that is scoped, asserted just
	// below: the pending list does not carry the pair at all.
	code, _ = e.do(t, "bob", "POST", "/api/approvals/benchmark_verdict:"+pair.PairID+"/verdict",
		`{"verdict":"A","guess":"A"}`)
	if code != http.StatusForbidden {
		t.Errorf("another member's vote: status %d, want 403", code)
	}
	mine := e.mustDo(t, "bob", "GET", "/api/benchmark/verdicts", "")
	if strings.Contains(mine, pair.PairID) {
		t.Errorf("a member's pending list carries another member's pair: %s", mine)
	}
	// The non-tautological direction: the requester's own vote SUCCEEDS.
	e.mustDo(t, "alice", "POST", "/api/approvals/benchmark_verdict:"+pair.PairID+"/verdict",
		`{"verdict":"B","guess":"A"}`)
	if got := e.pair(t, pair.PairID); got.State != benchmark.StateRecorded {
		t.Errorf("the requester's own vote did not land: %s", got.State)
	}
	// A second vote on a recorded pair is a conflict, never a second record.
	code, _ = e.do(t, "alice", "POST", "/api/approvals/benchmark_verdict:"+pair.PairID+"/verdict",
		`{"verdict":"A","guess":"A"}`)
	if code != http.StatusConflict {
		t.Errorf("re-voting a recorded pair: status %d, want 409", code)
	}
	if n := e.recordedEvents(t, benchmark.EventPairRecorded, pair.PairID); n != 1 {
		t.Errorf("%d records exist after a re-vote attempt, want 1", n)
	}
}

// TestPendingFormCarriesNoArmIdentity: the blind form serves the package's own
// pre-record shape, which has no field for an arm, a position, a run or a model
// — so the transport cannot leak what the vote must not know.
func TestPendingFormCarriesNoArmIdentity(t *testing.T) {
	e := newDriverEnv(t, &driverEngine{text: driverArmAnswer})
	pair := e.rendered(t, "bp-14", "t-14", "alice")

	out := e.mustDo(t, "alice", "GET", "/api/benchmark/verdicts", "")
	for _, forbidden := range []string{
		"platform-a", "platform-b", // the stored position
		"\"position\"", "\"direct_run_id\"", "\"platform_run_id\"",
		"\"direct_model\"", "\"platform_model\"", "\"arm\"",
		benchmark.DirectRunID(pair.PairID),
	} {
		if strings.Contains(out, forbidden) {
			t.Errorf("the pending form leaks %q — arm identity is revealed only after the record (BENCH-REG §3.4)", forbidden)
		}
	}
	// Non-tautological: the form DOES carry what a voter needs.
	for _, required := range []string{pair.PairID, driverArmAnswer, driverPlatformText, "guess_required"} {
		if !strings.Contains(out, required) {
			t.Fatalf("the pending form is missing %q — the scan above would pass vacuously", required)
		}
	}
}

// TestDeclineIsRecordedAndReported: a decline is a §14 record with the decline
// flag, not a silence. Selective participation must be visible.
func TestDeclineIsRecordedAndReported(t *testing.T) {
	e := newDriverEnv(t, &driverEngine{text: driverArmAnswer})
	pair := e.rendered(t, "bp-15", "t-15", "alice")

	out := e.mustDo(t, "alice", "POST", "/api/approvals/benchmark_verdict:"+pair.PairID+"/decline", `{}`)
	if !strings.Contains(out, `"declined":true`) {
		t.Fatalf("the decline response does not say so: %s", out)
	}
	if got := e.pair(t, pair.PairID); got.State != benchmark.StateDeclined {
		t.Fatalf("the pair is %s after a decline", got.State)
	}
	if n := e.recordedEvents(t, benchmark.EventPairRecorded, pair.PairID); n != 1 {
		t.Fatalf("a decline wrote %d records, want exactly 1 — declines are logged", n)
	}
	// Cross-owner: a decline is the requester's own act too.
	other := e.rendered(t, "bp-16", "t-16", "bob")
	code, _ := e.do(t, "alice", "POST", "/api/approvals/benchmark_verdict:"+other.PairID+"/decline", `{}`)
	if code != http.StatusForbidden {
		t.Errorf("declining another member's pair: status %d, want 403", code)
	}
}

// TestAlarmDispositionIsTheOperators: §12's disposition is an operator act, and
// the transport's refusal and the package's own data-layer check both hold. The
// disposition VOCABULARY is the registration's — the API never restates it, so
// the value travels through and the package validates it.
func TestAlarmDispositionIsTheOperators(t *testing.T) {
	e := newDriverEnv(t, &driverEngine{text: driverArmAnswer})
	e.operator(t, "op")
	e.seedPair(t, "bp-17", "t-17", "alice") // gives `alice` a users row
	raiseAlarm(t, e, benchmark.DomainSoftwareDevelopment, "e1")

	const card = "/api/approvals/benchmark_alarm:" + "software-development#e1"
	// A member cannot see a platform-scope card, so it does not exist for them.
	code, _ := e.do(t, "alice", "POST", card+"/dispose", `{"disposition":"investigate"}`)
	if code != http.StatusNotFound {
		t.Errorf("a member dispositioning an alarm: status %d, want 404", code)
	}
	// An unregistered disposition is refused by the PACKAGE against §12.
	code, body := e.do(t, "op", "POST", card+"/dispose", `{"disposition":"ignore-it"}`)
	if code != http.StatusBadRequest {
		t.Errorf("an unregistered disposition: status %d, want 400: %s", code, body)
	}
	// The operator's real disposition lands, with its clear kept as history.
	out := e.mustDo(t, "op", "POST", card+"/dispose", `{"disposition":"investigate","reason":"looking"}`)
	if !strings.Contains(out, `"cleared":true`) {
		t.Fatalf("the disposition response does not say so: %s", out)
	}
	var clears int
	if err := e.db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM run_events WHERE type = ? AND json_extract(payload,'$.action') = 'clear'`,
		benchmark.EventAlarm).Scan(&clears); err != nil {
		t.Fatal(err)
	}
	if clears != 1 {
		t.Errorf("%d alarm clears recorded, want 1 — alarm history is keep-forever", clears)
	}
	// And it is logged as a human decision by the VERB, not double-minted by the
	// transport (OQ8).
	var decisions int
	if err := e.db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM run_events WHERE type = ? AND json_extract(payload,'$.card_type') = 'benchmark.alarm'`,
		"decision.recorded").Scan(&decisions); err != nil {
		t.Fatal(err)
	}
	if decisions != 1 {
		t.Errorf("%d decision rows for one disposition, want exactly 1", decisions)
	}
}

func raiseAlarm(t *testing.T, e *driverEnv, domain, epoch string) {
	t.Helper()
	payload := `{"action":"raise","domain":"` + domain + `","epoch_id":"` + epoch +
		`","severity":"flag-now","summary":"S","loss_g":0.96,"threshold":0.95,"expansion_freeze":true}`
	if _, err := e.log.Append(context.Background(), eventlog.Append{
		UserID: "platform", Type: benchmark.EventAlarm, SchemaVersion: 1,
		Payload: json.RawMessage(payload),
	}); err != nil {
		t.Fatalf("raise alarm: %v", err)
	}
}

// TestOptInIsSelfOnlyIncludingTheOperator: consent is not delegable (§4.2.1).
// The refusal is stated at the transport AND inside the verb, and neither is the
// only statement of it.
func TestOptInIsSelfOnlyIncludingTheOperator(t *testing.T) {
	e := newDriverEnv(t, &driverEngine{text: driverArmAnswer})
	e.operator(t, "op")
	e.seedPair(t, "bp-18", "t-18", "alice")
	ctx := context.Background()

	// The default is OFF and nothing turned it on.
	if in, err := e.bs.Store.OptedIn(ctx, "alice"); err != nil || in {
		t.Fatalf("the opt-in default is not OFF: %v, %v", in, err)
	}
	// The operator is refused on somebody else's consent.
	code, body := e.do(t, "op", "POST", "/api/benchmark/opt-in", `{"person":"alice","enabled":true}`)
	if code != http.StatusForbidden {
		t.Fatalf("the operator set another person's consent: status %d: %s", code, body)
	}
	if in, _ := e.bs.Store.OptedIn(ctx, "alice"); in {
		t.Fatal("a refused delegation still flipped the flag")
	}
	// A member naming somebody else is refused by the same sentence.
	code, _ = e.do(t, "alice", "POST", "/api/benchmark/opt-in", `{"person":"op","enabled":true}`)
	if code != http.StatusForbidden {
		t.Errorf("a member set another person's consent: status %d", code)
	}
	// The non-tautological direction: your own consent works, and the verb logs
	// it as a human decision itself (OQ8 — the transport mints nothing).
	e.mustDo(t, "alice", "POST", "/api/benchmark/opt-in", `{"enabled":true}`)
	if in, err := e.bs.Store.OptedIn(ctx, "alice"); err != nil || !in {
		t.Fatalf("the requester's own opt-in did not land: %v, %v", in, err)
	}
	var decisions int
	if err := e.db.QueryRowContext(ctx,
		`SELECT count(*) FROM run_events WHERE type = ? AND json_extract(payload,'$.card_type') = 'benchmark.opt_in'`,
		"decision.recorded").Scan(&decisions); err != nil {
		t.Fatal(err)
	}
	if decisions != 1 {
		t.Errorf("%d consent decisions recorded for one flip, want exactly 1", decisions)
	}
	// And it is reversible by its owner.
	e.mustDo(t, "alice", "POST", "/api/benchmark/opt-in", `{"enabled":false}`)
	if in, _ := e.bs.Store.OptedIn(ctx, "alice"); in {
		t.Error("opting back out did not take")
	}
}
