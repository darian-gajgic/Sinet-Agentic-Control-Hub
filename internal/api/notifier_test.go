package api_test

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/push"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/verify"
)

// The S15.11 notifier, driven end to end (B6-9).
//
// EVERYTHING BELOW IS HERMETIC. The sender is the REAL push.Sender — real
// RFC 8291 encryption, real RFC 8292 signing, real audit rows — pointed at a
// loopback TLS httptest server. Nothing dials a real push service, 8481/8482, a
// live unit or the network, and zero pushes leave this machine. Using the real
// sender rather than a recording double is load-bearing: the audit row it
// writes IS the dueness store, so idempotency and restart-safety are properties
// of the landed path and not of a stub written to agree with them.

// pushService is the fake push service. TLS because the platform refuses a
// non-https endpoint at enrolment — every real push service is https — so a
// plain-HTTP fake would have meant loosening a production rule for a test.
type pushService struct {
	*httptest.Server
	got []pushHit
}

type pushHit struct {
	topic   string
	urgency string
	path    string
}

func newPushService(t *testing.T) *pushService {
	t.Helper()
	p := &pushService{}
	p.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		p.got = append(p.got, pushHit{
			topic: r.Header.Get("Topic"), urgency: r.Header.Get("Urgency"), path: r.URL.Path,
		})
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(p.Close)
	return p
}

func (p *pushService) count() int { return len(p.got) }

func (p *pushService) reset() { p.got = nil }

// notifyEnv is the decision-plane world plus a real push channel over the same
// database, with a controllable clock.
type notifyEnv struct {
	*decisionEnv
	t     *testing.T
	ctx   context.Context
	svc   *pushService
	store *push.Store
	dir   string
	now   time.Time
}

func newNotifyEnv(t *testing.T) *notifyEnv {
	t.Helper()
	e := &notifyEnv{
		decisionEnv: newDecisionEnv(t), t: t, ctx: context.Background(),
		svc: newPushService(t),
		dir: filepath.Join(t.TempDir(), "state"),
		now: time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC),
	}
	e.store = e.newStore()
	// asks.run_id is a FOREIGN KEY, so a card needs a real run to hang on. One
	// parked run per owner, which is also the state a task holding an open card
	// is actually in (the intake pipeline parks on a gate — §44-B R1).
	for _, owner := range []string{"alice", "bob", "op"} {
		seedRun(t, e.decisionEnv.b, "r-"+owner, owner, "", "parked", "anthropic")
	}
	return e
}

// newStore builds a push store over the SAME database and state directory. A
// SECOND call is a fresh process: the same rows, the same VAPID key, nothing
// carried over in memory.
func (e *notifyEnv) newStore() *push.Store {
	e.t.Helper()
	st, err := push.New(push.Config{
		DB: e.b.db, Log: e.b.log, StateDir: e.dir,
		Now: func() time.Time { return e.now },
	})
	if err != nil {
		e.t.Fatalf("push.New: %v", err)
	}
	return st
}

// server builds the notifier's server. `store` is passed in so a test can hand
// it a FRESH store and prove dueness comes from the database.
func (e *notifyEnv) server(store *push.Store) *api.Server {
	e.t.Helper()
	return api.New(api.Config{
		Log: e.b.log, Sessions: e.b.store, Auth: fixedIdentity{"op"},
		Settings: approvalSettings(),
		HealthFn: func() api.Health { return api.Health{Ready: true} },
		DB:       e.b.db, Meter: fakeMeter{},
		Intake: e.surface, Effects: e.journal, Cancel: e.cancel,
		Now:        func() time.Time { return e.now },
		Push:       store,
		PushSender: push.NewSender(store, e.svc.Client()),
	})
}

func (e *notifyEnv) evaluate() {
	e.t.Helper()
	if err := e.server(e.store).EvaluatePush(e.ctx); err != nil {
		e.t.Fatalf("EvaluatePush: %v", err)
	}
}

// enrol puts one device on the register for `who`.
func (e *notifyEnv) enrol(who, label string) {
	e.t.Helper()
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		e.t.Fatalf("key: %v", err)
	}
	auth := make([]byte, 16)
	if _, err := rand.Read(auth); err != nil {
		e.t.Fatalf("auth: %v", err)
	}
	b := base64.RawURLEncoding
	if _, _, err := e.store.Enrol(e.ctx, who, push.Enrolment{
		Endpoint: e.svc.URL + "/push/" + who + "/" + label,
		Keys:     push.Keys{P256DH: b.EncodeToString(priv.PublicKey().Bytes()), Auth: b.EncodeToString(auth)},
		Origin:   "https://sinet.example.ts.net",
		Label:    label,
	}); err != nil {
		e.t.Fatalf("enrol %s: %v", who, err)
	}
}

// safetyCard marshals a REAL verify.Card carrying the ratified safety SLA. It
// is the producer's own type, so the `sla.class` key path and the `safety`
// value are the ones internal/verify stamps — a rename there fails here rather
// than silently reclassifying every safety escalation as an ordinary approval.
func safetyCard(t *testing.T, summary string) string {
	t.Helper()
	raw, err := json.Marshal(verify.Card{
		Kind: verify.SinkAlert, Category: verify.CatSafety,
		TaskID: "t-1", RunID: "r-1", IssuedTS: "2026-07-30T09:00:00Z",
		SLA:     verify.SLA{Class: verify.SLASafety, PushImmediately: true, RepingEveryHours: 1},
		Summary: summary, AskID: "ask-safety",
	})
	if err != nil {
		t.Fatalf("marshal card: %v", err)
	}
	return string(raw)
}

func (e *notifyEnv) pushEvents() []map[string]any {
	e.t.Helper()
	rows, err := e.b.db.QueryContext(e.ctx,
		`SELECT payload FROM run_events WHERE type = ? ORDER BY event_seq`, push.EventSent)
	if err != nil {
		e.t.Fatalf("read push.sent: %v", err)
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			e.t.Fatalf("scan: %v", err)
		}
		m := map[string]any{}
		if err := json.Unmarshal([]byte(p), &m); err != nil {
			e.t.Fatalf("payload: %v", err)
		}
		out = append(out, m)
	}
	return out
}

// TestApprovalCardsPushAtTheRatifiedThresholdAndRepeat drives the G1 Def.3
// approval line: nothing at birth, a push once the card has been pending ⚙
// verification.card_push_hours, and every further card_push_hours while it
// stays unanswered (OQ4(i)).
func TestApprovalCardsPushAtTheRatifiedThresholdAndRepeat(t *testing.T) {
	e := newNotifyEnv(t)
	e.enrol("alice", "phone")
	seedCard(t, e.b, "ask-fresh", "r-alice", "alice", approvalCard("medium", "a decision"), e.now)

	// A card born just now is NOT due: 24 h has not passed.
	e.evaluate()
	if n := e.svc.count(); n != 0 {
		t.Fatalf("a fresh card pushed %d times, want 0 before the 24 h threshold", n)
	}

	// Just short of the threshold: still nothing. This is the control that
	// stops the assertion below passing on a notifier that pushes everything.
	e.now = e.now.Add(23*time.Hour + 59*time.Minute)
	e.evaluate()
	if n := e.svc.count(); n != 0 {
		t.Fatalf("pushed %d times one minute before the threshold", n)
	}

	// At the threshold it goes out, exactly once.
	e.now = e.now.Add(time.Minute)
	e.evaluate()
	if n := e.svc.count(); n != 1 {
		t.Fatalf("pushed %d times at the threshold, want 1", n)
	}
	if got := e.svc.got[0].urgency; got != "normal" {
		t.Errorf("approval Urgency = %q, want normal", got)
	}

	// It does NOT repeat before the next cadence.
	e.now = e.now.Add(23 * time.Hour)
	e.evaluate()
	if n := e.svc.count(); n != 1 {
		t.Fatalf("re-pushed after %s, want the 24 h re-nag cadence", 23*time.Hour)
	}
	// And it DOES repeat after it — the "re-nag" half of the ratified letter.
	e.now = e.now.Add(time.Hour)
	e.evaluate()
	if n := e.svc.count(); n != 2 {
		t.Fatalf("pushed %d times after two cadences, want 2", n)
	}
}

// TestSafetyCardsPushImmediatelyAndRePingHourly drives the other ratified line,
// including the class derivation: the card is safety-class ONLY because its own
// stamped snapshot says so.
func TestSafetyCardsPushImmediatelyAndRePingHourly(t *testing.T) {
	e := newNotifyEnv(t)
	e.enrol("alice", "phone")
	seedCard(t, e.b, "ask-safety", "r-alice", "alice", safetyCard(t, "confinement violation"), e.now)

	e.evaluate()
	if n := e.svc.count(); n != 1 {
		t.Fatalf("a safety card pushed %d times at birth, want 1 — it pushes IMMEDIATELY", n)
	}
	if got := e.svc.got[0].urgency; got != "high" {
		t.Errorf("safety Urgency = %q, want high", got)
	}

	e.now = e.now.Add(59 * time.Minute)
	e.evaluate()
	if n := e.svc.count(); n != 1 {
		t.Fatalf("re-pinged inside the hour: %d", n)
	}
	e.now = e.now.Add(time.Minute)
	e.evaluate()
	if n := e.svc.count(); n != 2 {
		t.Fatalf("did not re-ping at the hour: %d", n)
	}

	// THE CONTROL FOR THE CLASS DERIVATION: the same world with an ordinary
	// approval card, born at the same instant, pushes NOTHING at birth. Without
	// it, a notifier that treated every card as safety would pass above.
	e2 := newNotifyEnv(t)
	e2.enrol("alice", "phone")
	seedCard(t, e2.b, "ask-plain", "r-alice", "alice", approvalCard("medium", "a decision"), e2.now)
	e2.evaluate()
	if n := e2.svc.count(); n != 0 {
		t.Fatalf("an unstamped card was treated as safety-class: %d pushes at birth", n)
	}
}

// TestTwoBackToBackEvaluationsSendOnce: the evaluation is idempotent, and it is
// idempotent BECAUSE the first pass's audit row is what the second one reads.
func TestTwoBackToBackEvaluationsSendOnce(t *testing.T) {
	e := newNotifyEnv(t)
	e.enrol("alice", "phone")
	seedCard(t, e.b, "ask-safety", "r-alice", "alice", safetyCard(t, "urgent"), e.now)

	e.evaluate()
	e.evaluate()
	e.evaluate()
	if n := e.svc.count(); n != 1 {
		t.Fatalf("three back-to-back evaluations sent %d pushes, want 1", n)
	}
	if rows := e.pushEvents(); len(rows) != 1 {
		t.Fatalf("push.sent rows = %d, want 1", len(rows))
	}
}

// TestAFreshNotifierOverTheSameDatabaseAnswersIdentically is the restart/suspend
// proof (R8), driven rather than asserted. It also covers the second half: a
// card BORN during the downtime is picked up on the next pass, which a
// wall-clock countdown could never do.
func TestAFreshNotifierOverTheSameDatabaseAnswersIdentically(t *testing.T) {
	e := newNotifyEnv(t)
	e.enrol("alice", "phone")
	seedCard(t, e.b, "ask-safety", "r-alice", "alice", safetyCard(t, "urgent"), e.now)
	e.evaluate()
	if n := e.svc.count(); n != 1 {
		t.Fatalf("first pass sent %d", n)
	}

	// The process dies. A brand-new store and a brand-new server come up over
	// the same database — nothing about the first pass survives in memory.
	fresh := e.newStore()
	srv := e.server(fresh)

	// Still inside the re-ping cadence: the fresh notifier must NOT re-send.
	e.now = e.now.Add(30 * time.Minute)
	if err := srv.EvaluatePush(e.ctx); err != nil {
		t.Fatalf("fresh EvaluatePush: %v", err)
	}
	if n := e.svc.count(); n != 1 {
		t.Fatalf("a fresh notifier re-sent inside the cadence: %d pushes — dueness is not coming from stored state", n)
	}

	// A card BORN while the platform was down (backdated past its threshold) is
	// picked up on the very next pass.
	seedCard(t, e.b, "ask-downtime", "r-alice", "alice",
		approvalCard("medium", "born while the host slept"), e.now.Add(-30*time.Hour))
	if err := srv.EvaluatePush(e.ctx); err != nil {
		t.Fatalf("fresh EvaluatePush: %v", err)
	}
	if n := e.svc.count(); n != 2 {
		t.Fatalf("a card born during downtime was not picked up: %d pushes", n)
	}
	// Past the re-ping the safety card resumes on the fresh process too.
	e.now = e.now.Add(31 * time.Minute)
	if err := srv.EvaluatePush(e.ctx); err != nil {
		t.Fatalf("fresh EvaluatePush: %v", err)
	}
	if n := e.svc.count(); n != 3 {
		t.Fatalf("the fresh notifier did not resume the re-ping: %d pushes", n)
	}
}

// TestAnsweredAndExpiredCardsAreNeverPushed: the derivation only carries OPEN
// cards, and an expired one is dropped by the notifier because the countdown it
// displayed has run out.
func TestAnsweredAndExpiredCardsAreNeverPushed(t *testing.T) {
	e := newNotifyEnv(t)
	e.enrol("alice", "phone")

	// approvalSettings sets effects.approval_expiry to 7 days; a card observed
	// eight days ago is past it AND past the 24 h push threshold, so only the
	// expiry check can be what stops it.
	seedCard(t, e.b, "ask-expired", "r-alice", "alice",
		approvalCard("medium", "long gone"), e.now.Add(-8*24*time.Hour))
	e.evaluate()
	if n := e.svc.count(); n != 0 {
		t.Fatalf("an expired card pushed %d times", n)
	}

	// The control: the same card one day younger is INSIDE its expiry and past
	// the threshold, so it pushes. Without this the assertion above could pass
	// on a notifier that pushes nothing at all.
	seedCard(t, e.b, "ask-live", "r-alice", "alice",
		approvalCard("medium", "still live"), e.now.Add(-6*24*time.Hour))
	e.evaluate()
	if n := e.svc.count(); n != 1 {
		t.Fatalf("a live card past its threshold pushed %d times, want 1", n)
	}

	// An ANSWERED card leaves the derivation entirely.
	e.svc.reset()
	exec(t, e.b, `UPDATE asks SET status='answered', answered_ts=? WHERE ask_id=?`, nowTS(), "ask-live")
	e.now = e.now.Add(48 * time.Hour)
	e.evaluate()
	if n := e.svc.count(); n != 0 {
		t.Fatalf("an answered card was re-nagged %d times", n)
	}
}

// TestPushReachesOnlySomebodyWhoCanAnswer is the D10 routing reading, driven in
// both directions over ONE world: the operator's ranked list carries alice's
// card and the operator is not pushed it; alice is.
func TestPushReachesOnlySomebodyWhoCanAnswer(t *testing.T) {
	e := newNotifyEnv(t)
	e.enrol("alice", "phone")
	e.enrol("op", "operator-laptop")
	seedCard(t, e.b, "ask-alice", "r-alice", "alice",
		approvalCard("medium", "alice's own decision"), e.now.Add(-30*time.Hour))

	e.evaluate()
	if n := e.svc.count(); n != 1 {
		t.Fatalf("pushes = %d, want exactly 1 (alice's device only)", n)
	}
	if !strings.Contains(e.svc.got[0].path, "/alice/") {
		t.Errorf("the push went to %q, want alice's own device", e.svc.got[0].path)
	}

	// The non-tautological control: the operator's own read DOES carry the card
	// — so "the operator was not pushed" is about answerability and not about a
	// scope that never returned it.
	body := e.mustDo(t, "op", http.MethodGet, "/api/approvals", "")
	item, ok := itemByID(decodeList(t, body), "ask:ask-alice")
	if !ok {
		t.Fatal("the operator's inbox does not carry alice's card at all; the control is vacuous")
	}
	if item.Answerable {
		t.Fatal("the operator can answer alice's card, so this test proves nothing about routing")
	}
}

// TestBadgeIsTheInboxsOwnRankedList is OQ5's property: glance agreement. The
// badge is not computed to a plausible number — it is the length of the very
// list `GET /api/approvals` serves that identity.
func TestBadgeIsTheInboxsOwnRankedList(t *testing.T) {
	e := newNotifyEnv(t)
	e.enrol("alice", "phone")
	// Four of alice's cards past the threshold, one of bob's (which must not
	// count toward her badge), and one fresh one of hers (which is not DUE but
	// IS pending, so it counts).
	for _, id := range []string{"a1", "a2", "a3", "a4"} {
		seedCard(t, e.b, "ask-"+id, "r-alice", "alice", approvalCard("medium", id), e.now.Add(-30*time.Hour))
	}
	seedCard(t, e.b, "ask-b1", "r-bob", "bob", approvalCard("medium", "bob's"), e.now.Add(-30*time.Hour))
	seedCard(t, e.b, "ask-a5", "r-alice", "alice", approvalCard("medium", "fresh"), e.now)

	e.evaluate()

	served := decodeList(t, e.mustDo(t, "alice", http.MethodGet, "/api/approvals", ""))
	if served.Truncated {
		t.Fatal("the served list is truncated; the agreement below would be comparing different things")
	}
	rows := e.pushEvents()
	if len(rows) == 0 {
		t.Fatal("nothing was pushed, so the badge assertion is vacuous")
	}
	for i, r := range rows {
		if got := int(r["badge"].(float64)); got != len(served.Items) {
			t.Errorf("push %d carried badge %d, but alice's inbox serves %d items — the badge and the queue disagree at a glance",
				i, got, len(served.Items))
		}
	}
	// The badge counts what is PENDING, not what is due: alice's fresh card is
	// in the list and in the count, and it was not pushed.
	if len(served.Items) != 5 {
		t.Fatalf("alice's inbox serves %d items, want her own 5", len(served.Items))
	}
	if len(rows) != 4 {
		t.Fatalf("pushes = %d, want 4 (the fresh card is pending but not due)", len(rows))
	}
	// And bob's card is neither in her queue nor in her badge.
	if _, ok := itemByID(served, "ask:ask-b1"); ok {
		t.Error("alice's inbox carries bob's card")
	}
}

// TestEveryPushIsAuditedWithItsCardAndClass pins the audit row's own duties: it
// is the record of the act AND the dueness store, and it carries no
// notification text and no endpoint.
func TestEveryPushIsAuditedWithItsCardAndClass(t *testing.T) {
	e := newNotifyEnv(t)
	e.enrol("alice", "phone")
	e.enrol("alice", "tablet")
	seedCard(t, e.b, "ask-safety", "r-alice", "alice", safetyCard(t, "urgent"), e.now)
	e.evaluate()

	// One row PER DEVICE: each send is its own act with its own outcome.
	rows := e.pushEvents()
	if len(rows) != 2 {
		t.Fatalf("push.sent rows = %d, want one per enrolled device", len(rows))
	}
	seen := map[string]bool{}
	for _, r := range rows {
		if r["card"] != "ask:ask-safety" {
			t.Errorf("audit row names card %v", r["card"])
		}
		if r["class"] != push.ClassSafety {
			t.Errorf("audit row class = %v, want the class it went out under", r["class"])
		}
		if r["outcome"] != push.OutcomeSent {
			t.Errorf("audit row outcome = %v", r["outcome"])
		}
		h, _ := r["endpoint_hash"].(string)
		if len(h) != 64 {
			t.Errorf("audit row endpoint_hash = %q", h)
		}
		seen[h] = true
		raw, _ := json.Marshal(r)
		if strings.Contains(string(raw), "urgent") || strings.Contains(string(raw), e.svc.URL) {
			t.Errorf("the audit row carries notification text or an endpoint: %s", raw)
		}
	}
	if len(seen) != 2 {
		t.Errorf("the two rows name %d distinct subscriptions, want 2", len(seen))
	}
	// The two devices are ONE dueness clock: a second pass sends nothing.
	e.svc.reset()
	e.evaluate()
	if n := e.svc.count(); n != 0 {
		t.Errorf("a second pass re-sent to %d devices", n)
	}
}

// TestCadencesAreReadLiveFromSettings: the ⚙ keys are read AT EVALUATION, so an
// operator changing one moves the whole inbox — which is what the registry's own
// "inbox-wide" help text means. The stamped SLA block stays display data.
func TestCadencesAreReadLiveFromSettings(t *testing.T) {
	e := newNotifyEnv(t)
	e.enrol("alice", "phone")
	seedCard(t, e.b, "ask-1", "r-alice", "alice", approvalCard("medium", "a decision"), e.now)

	live := approvalSettings()
	srv := func() *api.Server {
		return api.New(api.Config{
			Log: e.b.log, Sessions: e.b.store, Auth: fixedIdentity{"op"},
			Settings: live,
			HealthFn: func() api.Health { return api.Health{Ready: true} },
			DB:       e.b.db, Meter: fakeMeter{},
			Intake: e.surface, Effects: e.journal, Cancel: e.cancel,
			Now:        func() time.Time { return e.now },
			Push:       e.store,
			PushSender: push.NewSender(e.store, e.svc.Client()),
		})
	}
	e.now = e.now.Add(2 * time.Hour)
	if err := srv().EvaluatePush(e.ctx); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if n := e.svc.count(); n != 0 {
		t.Fatalf("pushed at 2 h against a 24 h threshold: %d", n)
	}
	// The operator dials the threshold down. The SAME card, the SAME age.
	live.ints["verification.card_push_hours"] = 1
	if err := srv().EvaluatePush(e.ctx); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if n := e.svc.count(); n != 1 {
		t.Fatalf("the cadence change did not take effect: %d pushes", n)
	}
}

// TestTheNotifierFailsLoudlyWithoutItsCadences: a missing ⚙ key is a
// composition defect, not a runtime condition to notify through on an invented
// schedule.
func TestTheNotifierFailsLoudlyWithoutItsCadences(t *testing.T) {
	e := newNotifyEnv(t)
	e.enrol("alice", "phone")
	broken := approvalSettings()
	delete(broken.ints, "verification.safety_reping_hours")
	srv := api.New(api.Config{
		Log: e.b.log, Sessions: e.b.store, Auth: fixedIdentity{"op"},
		Settings: broken,
		HealthFn: func() api.Health { return api.Health{Ready: true} },
		DB:       e.b.db, Meter: fakeMeter{},
		Now:  func() time.Time { return e.now },
		Push: e.store, PushSender: push.NewSender(e.store, e.svc.Client()),
	})
	if err := srv.EvaluatePush(e.ctx); err == nil {
		t.Fatal("a missing cadence key was notified through")
	} else if !strings.Contains(err.Error(), "verification.safety_reping_hours") {
		t.Errorf("the failure does not name the key: %v", err)
	}
	if n := e.svc.count(); n != 0 {
		t.Errorf("%d pushes went out on an invented schedule", n)
	}
}

// TestNoSubscriptionsMeansNoWork: a household that has enrolled nothing costs
// one query and sends nothing.
func TestNoSubscriptionsMeansNoWork(t *testing.T) {
	e := newNotifyEnv(t)
	seedCard(t, e.b, "ask-1", "r-alice", "alice", approvalCard("medium", "x"), e.now.Add(-30*time.Hour))
	e.evaluate()
	if n := e.svc.count(); n != 0 {
		t.Fatalf("%d pushes with nothing enrolled", n)
	}
	if rows := e.pushEvents(); len(rows) != 0 {
		t.Fatalf("%d audit rows with nothing enrolled", len(rows))
	}
}

// TestDuenessHasNoSideTableAndNoTimerState is the §32 structural check: the
// schema gained exactly one table and it holds subscriptions, not countdowns;
// and neither package that runs the notifier carries per-card timer state.
func TestDuenessHasNoSideTableAndNoTimerState(t *testing.T) {
	e := newNotifyEnv(t)
	rows, err := e.b.db.QueryContext(e.ctx,
		`SELECT name FROM sqlite_master WHERE type='table' AND name LIKE '%push%' ORDER BY name`)
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		tables = append(tables, n)
	}
	if len(tables) != 1 || tables[0] != "push_subscriptions" {
		t.Errorf("push tables = %v, want exactly [push_subscriptions] — the audit rows ARE the last-pushed store", tables)
	}

	// notifier.go drives no schedule of its own: WHEN is the shell's, dueness
	// is stored state, and a timer here would be a competing clock.
	raw, err := os.ReadFile("notifier.go")
	if err != nil {
		t.Fatalf("read notifier.go: %v", err)
	}
	for _, banned := range []string{"time.NewTicker", "time.NewTimer", "time.Tick(", "time.AfterFunc"} {
		if strings.Contains(string(raw), banned) {
			t.Errorf("notifier.go carries %s", banned)
		}
	}
	if !strings.Contains(string(raw), "func pushDue(") {
		t.Fatal("the scan read the wrong file")
	}
}

// TestPushDueIsAPureFunctionOfStoredState drives the SLA table exhaustively
// without a database, which is what makes it reviewable: every row is a
// statement about the ratified set.
func TestPushDueIsAPureFunctionOfStoredState(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	const day, hour = 24 * time.Hour, time.Hour
	var zero time.Time
	cases := []struct {
		name       string
		class      string
		born, last time.Time
		want       bool
	}{
		{"safety, never pushed, born this instant", push.ClassSafety, now, zero, true},
		{"safety, pushed 59 minutes ago", push.ClassSafety, now.Add(-2 * hour), now.Add(-59 * time.Minute), false},
		{"safety, pushed an hour ago", push.ClassSafety, now.Add(-2 * hour), now.Add(-hour), true},
		{"approval, never pushed, one hour old", push.ClassApproval, now.Add(-hour), zero, false},
		{"approval, never pushed, 23 hours old", push.ClassApproval, now.Add(-23 * hour), zero, false},
		{"approval, never pushed, 24 hours old", push.ClassApproval, now.Add(-day), zero, true},
		{"approval, pushed 23 hours ago", push.ClassApproval, now.Add(-5 * day), now.Add(-23 * hour), false},
		{"approval, pushed 24 hours ago", push.ClassApproval, now.Add(-5 * day), now.Add(-day), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := api.PushDueForTest(c.class, c.born, c.last, now, day, hour); got != c.want {
				t.Errorf("pushDue = %v, want %v", got, c.want)
			}
		})
	}
}
