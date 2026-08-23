package opencode

// drain1_test.go — the P3-LN-1 drain round 1 findings, each pinned by the test
// that would have caught it. Several of these cover code the original suite
// exercised only indirectly, which is exactly how the defects survived it.

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// ─── D1: a recovered error must not crash the turn, and its tokens still bill ──

func TestTransientErrorDoesNotOutrankASuccessfulCompletion(t *testing.T) {
	a := testAdapter(t)
	req := testRequest(t)
	l, err := a.lower(req)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	s := &session{a: a, req: req, low: l, p: newParser(fixtureSessionID, t.Logf)}
	s.cursor = adapters.Cursor{Substrate: adapters.SubstrateOpencode, SessionID: fixtureSessionID}
	var usages []*adapters.Usage
	for _, frame := range fixtureFrames(t, "error-then-success.sse") {
		for _, ev := range s.p.feed(frame) {
			if ev.Kind == adapters.KindUsage && ev.Usage != nil {
				usages = append(usages, ev.Usage)
			}
		}
	}
	out, extra := s.assembleOutcome()

	if out.Kind != adapters.OutcomeCompleted {
		t.Fatalf("outcome = %q (%s), want completed — the engine reported the turn finished, and a "+
			"transient error it recovered from must not be minted into a crash (R19)", out.Kind, out.Detail)
	}
	if out.ResultText != "recovered-answer" {
		t.Errorf("ResultText = %q, want the real answer — a sticky error must not throw it away", out.ResultText)
	}
	// D7 is per PAID call, not per successful one: the errored call was billed.
	if len(usages) != 2 {
		t.Fatalf("usage events = %d, want 2 — the errored call's tokens are paid and must checkpoint", len(usages))
	}
	if usages[0].InputTokens != 8 || usages[0].OutputTokens != 5 {
		t.Errorf("the errored call's token row was lost: %+v", usages[0])
	}
	// Demoted, never erased.
	if s.p.transientErr == "" {
		t.Error("the recovered error vanished entirely — an operator can no longer see the engine misbehaved")
	}
	if len(extra) != 1 || extra[0].Kind != adapters.KindDone {
		t.Fatalf("terminal events = %v", kindsOf(extra))
	}
	if !strings.Contains(string(extra[0].Payload), "transient_error") {
		t.Errorf("the done payload does not carry the demoted error: %s", extra[0].Payload)
	}
	if !strings.Contains(string(extra[0].Payload), "provider returned 503") {
		t.Errorf("the demoted error lost the engine's own words: %s", extra[0].Payload)
	}
}

func TestErrorAfterSuccessStillCrashes(t *testing.T) {
	// The mirror image, so the D1 fix cannot become "errors never crash": an
	// error that arrives AFTER the last success is the turn's real ending.
	a := testAdapter(t)
	req := testRequest(t)
	l, _ := a.lower(req)
	s := &session{a: a, req: req, low: l, p: newParser(fixtureSessionID, t.Logf)}
	for _, frame := range fixtureFrames(t, "happy.sse") {
		s.p.feed(frame)
	}
	for _, frame := range fixtureFrames(t, "crash.sse") {
		s.p.feed(frame)
	}
	out, _ := s.assembleOutcome()
	if out.Kind != adapters.OutcomeCrashed {
		t.Fatalf("outcome = %q, want crashed — the LAST word from the engine was an error", out.Kind)
	}
}

func kindsOf(evs []adapters.Event) []adapters.EventKind {
	out := make([]adapters.EventKind, len(evs))
	for i, ev := range evs {
		out[i] = ev.Kind
	}
	return out
}

// ─── D2a: the boot parse, and the buffer contract it depends on ───────────────

func TestParseListenURLRequiresACompleteLine(t *testing.T) {
	const want = "http://127.0.0.1:41005"
	for name, tc := range map[string]struct {
		in   string
		want string
	}{
		"complete line":            {"opencode server listening on " + want + "\n", want},
		"crlf":                     {"opencode server listening on " + want + "\r\n", want},
		"followed by more output":  {"opencode server listening on " + want + "\nready\n", want},
		"preceded by other output": {"loading config\nopencode server listening on " + want + "\n", want},
		// The reason the rule exists: a port still being written must never be
		// read as a URL, or every later call goes to the wrong port.
		"port still arriving": {"opencode server listening on http://127.0.0.1:41", ""},
		"prefix only":         {"opencode server listening on http://127.0.0.1:", ""},
		// The reason awaitListening must read raw(): TRIMMED, this same complete
		// announcement has lost its terminator and is indistinguishable from a
		// half-written one.
		"complete line, trimmed": {"opencode server listening on " + want, ""},
		"nothing yet":            {"", ""},
		"non-numeric port":       {"opencode server listening on http://127.0.0.1:4x1\n", ""},
	} {
		if got := parseListenURL(tc.in); got != tc.want {
			t.Errorf("%s: parseListenURL(%q) = %q, want %q", name, tc.in, got, tc.want)
		}
	}
}

func TestBoundedBufferRawKeepsTheTerminatorStringStrips(t *testing.T) {
	// The exact contract the 5cada25 regression violated. If String() and raw()
	// ever agree on a trailing newline, awaitListening's guarantee is gone.
	b := &boundedBuffer{cap: 1 << 10}
	if _, err := b.Write([]byte("opencode server listening on http://127.0.0.1:41005\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !strings.HasSuffix(b.raw(), "\n") {
		t.Errorf("raw() lost the line terminator: %q", b.raw())
	}
	if strings.HasSuffix(b.String(), "\n") {
		t.Errorf("String() kept the terminator; raw() is then redundant and the distinction rots: %q", b.String())
	}
}

// TestAwaitListeningSeesACompleteAnnouncement is the regression test for the
// tier-R-disabling defect: reintroduce the trimmed-buffer read and this FAILS
// (it times out on an announcement that is plainly present) rather than turning
// every real-engine probe into a silent skip.
func TestAwaitListeningSeesACompleteAnnouncement(t *testing.T) {
	m := &Manager{BootTimeout: 2 * time.Second}
	p := &instanceProc{out: &boundedBuffer{cap: 1 << 10}, exited: make(chan struct{})}
	if _, err := p.out.Write([]byte("opencode server listening on http://127.0.0.1:41005\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	url, err := m.awaitListening(context.Background(), p)
	if err != nil {
		t.Fatalf("awaitListening did not see an announcement that is IN the buffer: %v", err)
	}
	if url != "http://127.0.0.1:41005" {
		t.Errorf("url = %q", url)
	}
}

func TestAwaitListeningDistinguishesTimeoutFromCancellation(t *testing.T) {
	m := &Manager{BootTimeout: 200 * time.Millisecond}
	p := &instanceProc{out: &boundedBuffer{cap: 1 << 10}, exited: make(chan struct{})}
	_, err := m.awaitListening(context.Background(), p)
	if !errors.Is(err, ErrBootTimeout) {
		t.Fatalf("a budget that ran out = %v, want ErrBootTimeout", err)
	}

	// A caller walking away is NOT the engine failing to boot; callers branch on
	// the type, so the two must not share a name.
	m2 := &Manager{BootTimeout: time.Minute}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p2 := &instanceProc{out: &boundedBuffer{cap: 1 << 10}, exited: make(chan struct{})}
	_, err = m2.awaitListening(ctx, p2)
	if errors.Is(err, ErrBootTimeout) {
		t.Fatalf("caller cancellation reported as a boot timeout — it would take the sanctioned skip: %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v, want a wrapped context.Canceled", err)
	}
}

// ─── D3: the R16 seams, and the credential that must actually arrive ──────────

func TestConfinerRefusedByName(t *testing.T) {
	f := newFakeServe(t)
	a := fakeAdapter(t, f)
	req := testRequest(t)
	req.Confiner = refusingConfiner{}
	req.Class = "C1"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := a.Start(ctx, req); !errors.Is(err, ErrConfinementUnsupported) {
		t.Fatalf("Start error = %v, want ErrConfinementUnsupported (S11.1: the jail is per-USER)", err)
	}
	rec := parkRecordFixture(t, a)
	rec.Start.Confiner = refusingConfiner{}
	if _, err := a.Resume(ctx, rec, ApproveAnswer(fixtureAskID)); !errors.Is(err, ErrConfinementUnsupported) {
		t.Fatalf("Resume error = %v, want ErrConfinementUnsupported", err)
	}
	if len(f.requests()) != 0 {
		t.Errorf("a confined request still reached the engine: %+v", f.requests())
	}
}

type refusingConfiner struct{}

func (refusingConfiner) Confine(adapters.StartRequest, adapters.SpawnSpec) (*exec.Cmd, func(), error) {
	panic("the adapter must refuse before composing anything")
}

func TestResumeWithoutAnEngineSessionIsNamed(t *testing.T) {
	f := newFakeServe(t)
	a := fakeAdapter(t, f)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rec := parkRecordFixture(t, a)
	rec.Cursor.SessionID = ""
	if _, err := a.Resume(ctx, rec, ApproveAnswer(fixtureAskID)); !errors.Is(err, ErrNoEngineSession) {
		t.Fatalf("Resume error = %v, want ErrNoEngineSession", err)
	}
}

func TestCredInjectTransformsTheServeEnvironment(t *testing.T) {
	f := newFakeServe(t)
	f.onPrompt = func(f *fakeServe, n int) { f.publishFixture("happy.sse") }
	a := testAdapter(t)
	a.Providers = testProviders(f.srv.URL)
	spy := &spyInstances{inst: f.instance()}
	a.Instances = spy

	req := testRequest(t)
	called := 0
	req.CredInject = func(base []string) ([]string, error) {
		called++
		return append(append([]string(nil), base...), "SINET_TEST_BROKER_CRED=resolved-at-spawn"), nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	sess, err := a.Start(ctx, req)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	for range sess.Events() {
	}
	if _, err := sess.Wait(ctx); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if called != 1 {
		t.Fatalf("CredInject called %d times, want exactly once per spawn (S01.6: resolved FRESH, never stored)", called)
	}
	if len(spy.specs) != 1 {
		t.Fatalf("instance specs = %d", len(spy.specs))
	}
	if _, ok := envValue(spy.specs[0].Env, "SINET_TEST_BROKER_CRED"); !ok {
		t.Errorf("the injected credential never reached the serve environment: %v", spy.specs[0].Env)
	}
	// The secret is per-spawn wiring, never park state (json:"-" on the shared
	// struct); nothing here may persist it.
	if req.CredInject == nil {
		t.Error("CredInject was cleared on the request")
	}
	blob, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal StartRequest: %v", err)
	}
	if strings.Contains(string(blob), "resolved-at-spawn") {
		t.Errorf("the injected credential is serializable off the request: %s", blob)
	}
}

// TestCredentialRotationRestartsTheInstance is the latent trap the missing
// CredInject coverage was hiding: with the environment outside the instance
// identity, a freshly resolved credential would never reach an already-live
// server, and the engine would keep serving on a stale one.
func TestCredentialRotationRestartsTheInstance(t *testing.T) {
	a := testAdapter(t)
	req := testRequest(t)
	l, err := a.lower(req)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	base := a.instanceSpec(req, l, l.env)
	rotated := a.instanceSpec(req, l, append(append([]string(nil), l.env...), "BROKER_TOKEN=v2"))

	if identityOf(base) == identityOf(rotated) {
		t.Fatal("a rotated credential does not change the instance identity — the live serve would keep " +
			"running on the old one (S11.5: credentials are delivered at spawn)")
	}
	// A digest, never the raw env: identities are compared, logged around and
	// held in memory, and a secret belongs in none of those.
	if strings.Contains(identityOf(rotated), "BROKER_TOKEN") || strings.Contains(identityOf(rotated), "v2") {
		t.Errorf("the identity key carries raw environment content: %q", identityOf(rotated))
	}
	if identityOf(base) != identityOf(a.instanceSpec(req, l, l.env)) {
		t.Error("identity is not stable for an unchanged spec — every run would restart the serve")
	}
}

type spyInstances struct {
	inst  Instance
	specs []InstanceSpec
}

func (s *spyInstances) Acquire(_ context.Context, spec InstanceSpec) (Instance, error) {
	s.specs = append(s.specs, spec)
	return s.inst, nil
}
func (s *spyInstances) Stop(context.Context, string) error { return nil }

// ─── D8: the model rides prompt_async, so it must not restart a healthy serve ─

func TestModelChangeDoesNotRestartTheInstance(t *testing.T) {
	a := testAdapter(t)
	req := testRequest(t)
	l1, err := a.lower(req)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	other := testRequest(t)
	other.Cwd, other.WorkDir = req.Cwd, req.WorkDir
	other.Model = "sinetfake/another-model"
	l2, err := a.lower(other)
	if err != nil {
		t.Fatalf("lower(other model): %v", err)
	}
	if identityOf(a.instanceSpec(req, l1, l1.env)) != identityOf(a.instanceSpec(other, l2, l2.env)) {
		t.Fatal("a model change restarts the user's serve — the model rides per-message on prompt_async, " +
			"so the restart buys nothing and kills every ask the instance was holding (S03.4)")
	}
	// The S02.4(e) invocation fingerprint is a different question and DOES move:
	// the checkpoint must still see that the invocation config changed.
	if l1.fingerprint == l2.fingerprint {
		t.Error("the S02.4e fingerprint went blind to the model — freshness gating depends on it")
	}
	// A config change still restarts, or startup-bound state could never move.
	gated := testRequest(t)
	gated.Cwd, gated.WorkDir = req.Cwd, req.WorkDir
	gated.Worker.GatedTools = []string{"bash", "read"}
	l3, err := a.lower(gated)
	if err != nil {
		t.Fatalf("lower(gated): %v", err)
	}
	if identityOf(a.instanceSpec(gated, l3, l3.env)) == identityOf(a.instanceSpec(req, l1, l1.env)) {
		t.Error("a changed permission map does NOT restart the serve — the engine resolves it at startup")
	}
}

// ─── D4 / D7: what may ride an answer, and which way encoding fails ───────────

func TestDecisionEnvelopeRefusesSmuggledInput(t *testing.T) {
	for name, updated := range map[string]string{
		"approve carrying edited input": `{"sinet.decision":"approve","command":"rm -rf /"}`,
		"reject carrying edited input":  `{"sinet.decision":"reject","message":"no","command":"echo x"}`,
		"decision plus unknown member":  `{"sinet.decision":"approve","updatedInput":{"a":1}}`,
		"raw replacement input":         `{"command":"echo EDITED"}`,
		"unencodable sentinel":          `{"sinet.decision":"unencodable"}`,
	} {
		_, _, err := decodeDecision(&adapters.Answer{AskID: fixtureAskID, UpdatedInput: json.RawMessage(updated)})
		if !errors.Is(err, ErrUpdatedInputUnsupported) {
			t.Errorf("%s: decodeDecision(%s) error = %v, want ErrUpdatedInputUnsupported — honoring the "+
				"decision while dropping the edit executes a call the human did not approve", name, updated, err)
		}
	}
	// The two legitimate shapes still decode.
	if reply, msg, err := decodeDecision(ApproveAnswer(fixtureAskID)); err != nil || reply != replyOnce || msg != "" {
		t.Errorf("approve = (%q, %q, %v)", reply, msg, err)
	}
	if reply, msg, err := decodeDecision(RejectAnswer(fixtureAskID, "not allowed")); err != nil || reply != replyReject || msg != "not allowed" {
		t.Errorf("reject = (%q, %q, %v)", reply, msg, err)
	}
}

func TestRejectAnswerFailsClosed(t *testing.T) {
	// Migrated at LN-2A with the decision envelope it was written against: the
	// guarantee is unchanged and now structural. A verdict this substrate
	// cannot express must never decode as an APPROVE — and since the typed
	// member cannot fail to marshal, the encoding failure the old sentinel
	// stood in for no longer exists at all.
	unencodable := &adapters.Answer{AskID: fixtureAskID, Decision: adapters.AnswerDecision("unencodable")}
	reply, _, err := decodeDecision(unencodable)
	if err == nil {
		t.Fatalf("an inexpressible decision decoded to reply %q — a refusal became consent", reply)
	}
	if reply == replyOnce {
		t.Fatal("an inexpressible decision approved")
	}
	if RejectAnswer(fixtureAskID, "no").Decision != adapters.DecisionReject {
		t.Fatal("RejectAnswer no longer carries a refusal")
	}
}

// ─── D5: a parallel turn is not interruptible until every call is done ────────

func TestParallelToolBoundaryWaitsForEverySibling(t *testing.T) {
	p := newParser(fixtureSessionID, t.Logf)
	frames := fixtureFrames(t, "parallel-tools.sse")
	running := func() int { return len(p.runningTools) }

	for i, frame := range frames {
		p.feed(frame)
		switch i {
		case 3: // all three dispatched
			if running() != 3 {
				t.Fatalf("after three dispatches, running = %d, want 3", running())
			}
		case 4, 5: // one, then two, have finished — the turn is STILL mid-work
			if running() == 0 {
				t.Fatalf("frame %d: a sibling finishing declared a safe boundary while %d call(s) were "+
					"still executing — Pause would abort mid-tool-call (P-T01-4)", i, 3-(i-3))
			}
		}
	}
	if running() != 0 {
		t.Fatalf("after every sibling completed, running = %d, want 0", running())
	}
}

func TestPauseHoldsUntilTheLastSiblingFinishes(t *testing.T) {
	f := newFakeServe(t)
	a := fakeAdapter(t, f)
	req := testRequest(t)
	l, err := a.lower(req)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	s := a.newSession(req, l, newClient(f.instance(), nil), fixtureSessionID, fixtureDirectory)
	ctx := context.Background()

	frames := fixtureFrames(t, "parallel-tools.sse")
	aborts := func() int {
		n := 0
		for _, r := range f.requests() {
			if strings.HasSuffix(r.Path, "/abort") {
				n++
			}
		}
		return n
	}
	// Get all three calls in flight FIRST — a pause requested before any tool
	// starts is legitimately at a boundary already, which is not what this
	// pins.
	for _, frame := range frames[:4] {
		s.p.feed(frame)
		s.checkPauseBoundary(ctx)
	}
	if err := s.Pause(ctx); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if aborts() != 0 {
		t.Fatalf("Pause aborted immediately with three tool calls in flight (P-T01-4)")
	}
	for i, frame := range frames[4:6] { // the first two siblings finish
		s.p.feed(frame)
		s.checkPauseBoundary(ctx)
		if aborts() != 0 {
			t.Fatalf("sibling %d finishing declared a safe boundary while others were still executing "+
				"— the pause would land mid-tool-call (P-T01-4)", i+1)
		}
	}
	s.p.feed(frames[6]) // the last sibling completes
	s.checkPauseBoundary(ctx)
	if aborts() != 1 {
		t.Fatalf("aborts after the last sibling finished = %d, want 1 — a pause must land at the boundary", aborts())
	}
}

// ─── D6: a malformed pending entry must not wedge the run ────────────────────

func TestMalformedPermissionEntryIsSkippedNotEmitted(t *testing.T) {
	p := newParser(fixtureSessionID, t.Logf)
	// The pull-based enumeration path: an entry with no requestID is
	// unanswerable, and the Driver refuses an ask with no id — mid-pump that
	// refusal returns before the FSM transition and wedges the run in `running`.
	evs := p.emitAsk(permissionRequest{SessionID: fixtureSessionID, Permission: "bash"}, nil)
	if len(evs) != 0 {
		t.Fatalf("an ask with no requestID was emitted to the Driver: %s", evs[0].Payload)
	}
	if p.ask != nil {
		t.Errorf("an id-less entry became the turn's park: %+v", p.ask)
	}
	if p.unknownFrames == 0 {
		t.Error("the malformed entry was dropped silently rather than logged and counted")
	}
	// A well-formed sibling in the same enumeration still lands.
	evs = p.emitAsk(permissionRequest{ID: fixtureAskID, SessionID: fixtureSessionID, Permission: "bash"}, nil)
	if len(evs) != 1 || evs[0].Ask == nil || evs[0].Ask.ID != fixtureAskID {
		t.Fatalf("the valid entry did not survive the malformed one: %v", kindsOf(evs))
	}
}

func TestMalformedEnumerationDoesNotWedgeTheDrivenRun(t *testing.T) {
	e := newE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	e.claimedRun(t, "r-malformed")

	f := newFakeServe(t)
	f.onPrompt = func(f *fakeServe, n int) {
		// The serve answers the enumeration with one usable ask and one entry
		// missing its id — the shape that used to reach the Driver.
		f.setPermissions(json.RawMessage(`{"sessionID":"`+fixtureSessionID+`","permission":"bash","patterns":[],"metadata":{},"always":[]}`),
			fixtureAsk())
		f.publishFixture("gate.sse")
	}
	a := fakeAdapter(t, f)
	req := testRequest(t)
	req.RunID = "r-malformed"

	out, err := e.drv.Drive(ctx, a, req)
	if err != nil {
		t.Fatalf("Drive: %v — a malformed pending entry must never fail the pump", err)
	}
	if out.Kind != adapters.OutcomeParked {
		t.Fatalf("outcome = %q (%s), want parked", out.Kind, out.Detail)
	}
	// The run reached its terminal state rather than being abandoned mid-pump.
	r, err := e.runs.Get(ctx, "r-malformed")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if r.State != run.StateParked {
		t.Fatalf("run state = %s, want parked — an aborted pump leaves it wedged in running", r.State)
	}
	var asks int
	if err := e.db.QueryRowContext(ctx, `SELECT count(*) FROM asks WHERE run_id = 'r-malformed'`).Scan(&asks); err != nil {
		t.Fatalf("count asks: %v", err)
	}
	if asks != 1 {
		t.Errorf("durable ask rows = %d, want exactly the one answerable ask", asks)
	}
}
