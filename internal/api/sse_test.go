package api_test

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
)

// ── S14.3 live-inspection test seams ──

// fixedIdentity authenticates every request as one non-dev user, so the owner
// scope reads that user's real role from the auth store (S01.9, OQ1).
type fixedIdentity struct{ id string }

func (f fixedIdentity) Authenticate(*http.Request) (api.Identity, error) {
	return api.Identity{UserID: f.id}, nil
}

// fakeMeter is the metering seam under test (§4 run counters + §3 fleet lane
// meter). LaneMeter returns a fixed non-zero consumption so the fleet lane
// carries a real meter figure; owner-scoping of the lane LIST is enforced by
// the projection SQL, exercised by TestSSEMemberScopedSnapshots.
type fakeMeter struct {
	tokens int64
	cost   float64
}

func (m fakeMeter) RunMeter(context.Context, string) (api.RunMeter, error) {
	return api.RunMeter{Tokens: m.tokens, APIEquivCostUSD: m.cost, Unpriced: m.cost == 0}, nil
}

func (m fakeMeter) LaneMeter(context.Context, string, string) (api.LaneMeter, error) {
	return api.LaneMeter{WeightedConsumption: 1}, nil
}

func nowTS() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func exec(t *testing.T, b *backend, q string, args ...any) {
	t.Helper()
	if err := b.db.WriteTx(context.Background(), func(tx *sql.Tx) error {
		_, e := tx.ExecContext(context.Background(), q, args...)
		return e
	}); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

// seedUser inserts a person row directly (no auth-event appends), so the event
// sequence stays clean for the owner-scoping assertions. The role is what
// isOperatorRead reads back through the auth store (S01.9, OQ1).
func seedUser(t *testing.T, b *backend, id, role string) {
	t.Helper()
	exec(t, b, `INSERT INTO users (user_id, display_name, role, created_ts) VALUES (?,?,?,?)`,
		id, id, role, nowTS())
}

func seedTask(t *testing.T, b *backend, taskID, owner, title, kanban string) {
	t.Helper()
	exec(t, b, `INSERT INTO tasks (task_id, user_id, title, kanban_status, created_ts) VALUES (?,?,?,?,?)`,
		taskID, owner, title, kanban, nowTS())
}

func seedRun(t *testing.T, b *backend, runID, owner, taskID, state, lane string) {
	t.Helper()
	var tid any
	if taskID != "" {
		tid = taskID
	}
	exec(t, b, `INSERT INTO runs (run_id, user_id, task_id, state, lane, generation, created_ts, updated_ts)
	            VALUES (?,?,?,?,?,0,?,?)`, runID, owner, tid, state, lane, nowTS(), nowTS())
}

func seedAsk(t *testing.T, b *backend, askID, runID, owner, status string) {
	t.Helper()
	exec(t, b, `INSERT INTO asks (ask_id, run_id, user_id, snapshot, status, observed_ts) VALUES (?,?,?,?,?,?)`,
		askID, runID, owner, "{}", status, nowTS())
}

func seedEffect(t *testing.T, b *backend, effectID, runID, owner, class, state string) {
	t.Helper()
	exec(t, b, `INSERT INTO effects (effect_id, run_id, user_id, class, payload, payload_hash, state, created_ts, updated_ts)
	            VALUES (?,?,?,?,?,?,?,?,?)`, effectID, runID, owner, class, "{}", "h", state, nowTS(), nowTS())
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

// appendRun appends one run-scoped event (generation 0, matching a seeded run)
// and returns its event_seq.
func appendRun(t *testing.T, b *backend, owner, runID, typ, payload string) int64 {
	t.Helper()
	seq, err := b.log.Append(context.Background(), eventlog.Append{
		RunID: runID, Generation: 0, UserID: owner, Type: typ, SchemaVersion: 1,
		Payload: json.RawMessage(payload),
	})
	if err != nil {
		t.Fatalf("append run event %s: %v", typ, err)
	}
	return seq
}

// appendPlatform appends one platform-scope event (user_id = platform).
func appendPlatform(t *testing.T, b *backend, owner, typ string) int64 {
	t.Helper()
	seq, err := b.log.Append(context.Background(), eventlog.Append{
		UserID: owner, Type: typ, SchemaVersion: 1, Payload: json.RawMessage(`{"n":1}`),
	})
	if err != nil {
		t.Fatalf("append platform event %s: %v", typ, err)
	}
	return seq
}

// snapFrame is one parsed snapshot frame.
type snapFrame struct {
	topic  string
	cursor int64
	state  map[string]any
}

func readSnapshot(t *testing.T, r *bufio.Reader) snapFrame {
	t.Helper()
	f := readEvent(t, r)
	if f.event != "snapshot" {
		t.Fatalf("want snapshot frame, got event=%q id=%q data=%q", f.event, f.id, f.data)
	}
	var env struct {
		Topic  string          `json:"topic"`
		Cursor int64           `json:"cursor"`
		State  json.RawMessage `json:"state"`
	}
	if err := json.Unmarshal([]byte(f.data), &env); err != nil {
		t.Fatalf("snapshot envelope: %v (%s)", err, f.data)
	}
	var st map[string]any
	if err := json.Unmarshal(env.State, &st); err != nil {
		t.Fatalf("snapshot state: %v", err)
	}
	return snapFrame{topic: env.Topic, cursor: env.Cursor, state: st}
}

// ── Topic multiplexing (rubric 1–3) ──

func TestSSEUnknownTopicRejected(t *testing.T) {
	b := newBackend(t)
	_, ts := newTestServer(t, serverOpts{b: b})
	resp, err := http.Get(ts.URL + "/events?topics=board,bogus")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown topic: status %d, want 400", resp.StatusCode)
	}
}

func TestSSERunTopicRequiresRun(t *testing.T) {
	b := newBackend(t)
	_, ts := newTestServer(t, serverOpts{b: b})
	resp, err := http.Get(ts.URL + "/events?topics=run")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("run topic without ?run: status %d, want 400", resp.StatusCode)
	}
}

func TestSSETopicFilterAndMultiValuedTags(t *testing.T) {
	b := newBackend(t)
	seedRun(t, b, "r1", "u1", "", "running", "anthropic")
	_, ts := newTestServer(t, serverOpts{b: b})

	// Subscribe board only. A run.state_changed (RunLifecycle, run-scoped) must
	// arrive tagged BOTH board and run; a checkpoint.written (not a board type)
	// must be filtered out of the board tail.
	r, done := stream(t, testCtx(t), ts.URL+"/events?topics=board", nil)
	defer done()
	readSnapshot(t, r) // the board snapshot

	appendRun(t, b, "u1", "r1", "checkpoint.written", `{"n":1}`) // not board → filtered
	stateSeq := appendRun(t, b, "u1", "r1", "run.state_changed", `{"from":"new","to":"running"}`)

	f := readEvent(t, r)
	if f.event != "run.state_changed" || f.id != fmt.Sprint(stateSeq) {
		t.Fatalf("board tail delivered event=%q id=%q, want run.state_changed id=%d (checkpoint must be filtered)",
			f.event, f.id, stateSeq)
	}
	var w struct {
		Topics []string `json:"topics"`
	}
	if err := json.Unmarshal([]byte(f.data), &w); err != nil {
		t.Fatalf("frame data: %v", err)
	}
	if !hasTag(w.Topics, "board") || !hasTag(w.Topics, "run") {
		t.Fatalf("run.state_changed tags = %v, want both board and run", w.Topics)
	}
}

// ── Snapshot-then-tail (rubric 4, 6) ──

func TestSSESnapshotThenTail(t *testing.T) {
	b := newBackend(t)
	seedRun(t, b, "r1", "u1", "", "running", "anthropic")
	_, ts := newTestServer(t, serverOpts{b: b, meter: fakeMeter{tokens: 42, cost: 0.5}})

	r, done := stream(t, testCtx(t), ts.URL+"/events?topics=run&run=r1", nil)
	defer done()

	snap := readSnapshot(t, r)
	if snap.topic != "run" {
		t.Fatalf("first frame topic = %q, want run", snap.topic)
	}
	if snap.state["run_id"] != "r1" {
		t.Fatalf("run card run_id = %v, want r1", snap.state["run_id"])
	}
}

// TestSSESnapshotCursorAtomic proves R7: the snapshot captures the head cursor
// H and the tail resumes strictly after H — pre-existing events are NOT
// replayed as frames, and newly appended events arrive exactly once with no
// gap across the snapshot→tail boundary.
func TestSSESnapshotCursorAtomic(t *testing.T) {
	b := newBackend(t)
	seedRun(t, b, "r1", "u1", "", "running", "anthropic")
	// Two pre-existing board events → head = 2 at connect.
	appendRun(t, b, "u1", "r1", "run.state_changed", `{"to":"queued"}`)
	appendRun(t, b, "u1", "r1", "run.state_changed", `{"to":"running"}`)
	srv, ts := newTestServer(t, serverOpts{b: b})

	r, done := stream(t, testCtx(t), ts.URL+"/events?topics=board", nil)
	defer done()

	snap := readSnapshot(t, r)
	if snap.cursor != 2 {
		t.Fatalf("snapshot cursor = %d, want 2 (head at connect)", snap.cursor)
	}

	// Append three more AFTER connect; the tail must deliver exactly 3,4,5.
	var want []int64
	for i := 0; i < 3; i++ {
		want = append(want, appendRun(t, b, "u1", "r1", "run.state_changed", `{"to":"parked"}`))
	}
	srv.Nudge()
	for i, wantSeq := range want {
		f := readEvent(t, r)
		if f.id != fmt.Sprint(wantSeq) {
			t.Fatalf("tail frame %d = id %s, want %d (gap, double, or pre-snapshot replay)", i, f.id, wantSeq)
		}
	}
}

// ── Progress semantics (rubric 8) ──

func TestSSERunCardProgressFieldSet(t *testing.T) {
	b := newBackend(t)
	seedRun(t, b, "r1", "u1", "", "parked", "anthropic")
	seedAsk(t, b, "a1", "r1", "u1", "gate") // open ask → waiting_on_human
	appendRun(t, b, "u1", "r1", "tool.completed", `{"tool":"read_file","args_digest":"sha256:abc"}`)
	seq := appendRun(t, b, "u1", "r1", "run.state_changed", `{"to":"parked","line":"parked on gate"}`)
	// A checkpoint referencing a real event_seq → steps counter.
	exec(t, b, `INSERT INTO checkpoints (run_id, user_id, event_seq, created_ts) VALUES (?,?,?,?)`,
		"r1", "u1", seq, nowTS())
	_, ts := newTestServer(t, serverOpts{b: b, meter: fakeMeter{tokens: 1234, cost: 0.0}})

	r, done := stream(t, testCtx(t), ts.URL+"/events?topics=run&run=r1", nil)
	defer done()
	snap := readSnapshot(t, r)

	for _, k := range []string{"state", "waiting_on_human", "parked_until", "stage", "tool",
		"counters", "last_activity", "wedged", "lane", "generation", "ceilings"} {
		if _, ok := snap.state[k]; !ok {
			t.Fatalf("run card missing field %q (§4 field set)", k)
		}
	}
	if snap.state["waiting_on_human"] != true {
		t.Fatalf("waiting_on_human = %v, want true (parked + open ask)", snap.state["waiting_on_human"])
	}
	counters, _ := snap.state["counters"].(map[string]any)
	for _, k := range []string{"tokens", "api_equiv_cost_usd", "elapsed_s", "steps"} {
		if _, ok := counters[k]; !ok {
			t.Fatalf("counters missing %q", k)
		}
	}
	if tok, _ := counters["tokens"].(float64); tok != 1234 {
		t.Fatalf("counters.tokens = %v, want 1234 (metering seam)", counters["tokens"])
	}
	tool, _ := snap.state["tool"].(map[string]any)
	if tool["name"] != "read_file" {
		t.Fatalf("tool = %v, want name read_file", snap.state["tool"])
	}
}

// TestSSENeverPercentNegative proves R10: NO percent/fraction/ratio/progress/
// pct/complete_fraction/ETA key exists in ANY projection or frame JSON.
func TestSSENeverPercentNegative(t *testing.T) {
	b := newBackend(t)
	seedTask(t, b, "t1", "u1", "Task", "doing")
	seedRun(t, b, "r1", "u1", "t1", "running", "anthropic")
	seedAsk(t, b, "a1", "r1", "u1", "gate")
	appendRun(t, b, "u1", "r1", "run.state_changed", `{"to":"running"}`)
	_, ts := newTestServer(t, serverOpts{b: b, meter: fakeMeter{tokens: 5, cost: 0.1}})

	// A field NAMED as a percent/fraction/ratio/progress/ETA is forbidden
	// (R10). Match keys exactly (so honest fields like "generation", which
	// merely contains the substring "ratio", are not flagged) plus a "percent"
	// substring guard (percent_done etc.).
	forbiddenExact := map[string]bool{
		"percent": true, "percentage": true, "percent_complete": true,
		"fraction": true, "complete_fraction": true, "ratio": true,
		"progress": true, "pct": true, "complete": true, "completion": true,
		"eta": true, "eta_s": true, "eta_seconds": true,
	}
	keys := map[string]bool{}

	r, done := stream(t, testCtx(t), ts.URL+"/events?topics=run,board,fleet,inbox&run=r1", nil)
	defer done()
	for i := 0; i < 4; i++ { // one snapshot per subscribed topic
		snap := readSnapshot(t, r)
		collectKeys(snap.state, keys)
	}
	// Also scan a live tail frame's full JSON.
	appendRun(t, b, "u1", "r1", "run.state_changed", `{"to":"parked"}`)
	f := readEvent(t, r)
	var full map[string]any
	if err := json.Unmarshal([]byte(f.data), &full); err == nil {
		collectKeys(full, keys)
	}

	for k := range keys {
		lk := strings.ToLower(k)
		if forbiddenExact[lk] || strings.Contains(lk, "percent") {
			t.Fatalf("projection exposes forbidden progress key %q — R10 never-percent", k)
		}
	}
}

// TestSSEWedgedDerivedNotWatchdog proves R11: wedged is derived from the
// run.wedged recovery marker (never a stored column), superseded by a later
// transition, and NOT conflated with B5-3's watchdog.flagged.
func TestSSEWedgedDerivedNotWatchdog(t *testing.T) {
	b := newBackend(t)
	seedRun(t, b, "r1", "u1", "", "running", "anthropic")
	appendRun(t, b, "u1", "r1", "run.state_changed", `{"to":"running"}`)
	appendRun(t, b, "u1", "r1", "run.wedged", `{"reason":"stall"}`)
	_, ts := newTestServer(t, serverOpts{b: b})

	// Latest lifecycle marker is run.wedged → wedged true.
	r, done := stream(t, testCtx(t), ts.URL+"/events?topics=run&run=r1", nil)
	if got := readSnapshot(t, r).state["wedged"]; got != true {
		t.Fatalf("wedged = %v, want true (latest marker run.wedged)", got)
	}
	done()

	// A resume transition supersedes it; a later watchdog.flagged must NOT.
	appendRun(t, b, "u1", "r1", "run.state_changed", `{"to":"running"}`)
	appendRun(t, b, "u1", "r1", "watchdog.flagged", `{"rule":"loop"}`)
	r2, done2 := stream(t, testCtx(t), ts.URL+"/events?topics=run&run=r1", nil)
	defer done2()
	if got := readSnapshot(t, r2).state["wedged"]; got != false {
		t.Fatalf("wedged = %v, want false (transition supersedes; watchdog.flagged is not wedged)", got)
	}
}

// ── Owner scoping (rubric 7; OQ1) ──

func TestSSEOwnerScopedTail(t *testing.T) {
	b := newBackend(t)
	seedUser(t, b, "op", auth.RoleOperator)
	seedUser(t, b, "alice", auth.RoleMember)
	seedRun(t, b, "r-alice", "alice", "", "running", "anthropic")
	seedRun(t, b, "r-op", "op", "", "running", "anthropic")

	appendPlatform(t, b, "platform", "settings.changed")                                     // seq 1
	aliceSeq := appendRun(t, b, "alice", "r-alice", "run.state_changed", `{"to":"running"}`) // seq 2
	appendRun(t, b, "op", "r-op", "run.state_changed", `{"to":"running"}`)                   // seq 3

	// Member (alice) sees ONLY her own rows: not platform (seq1), not op (seq3).
	srvA, tsA := newTestServer(t, serverOpts{b: b, auth: fixedIdentity{"alice"}})
	rA, doneA := stream(t, testCtx(t), tsA.URL+"/events?after_seq=0", nil)
	defer doneA()
	if f := readEvent(t, rA); f.id != fmt.Sprint(aliceSeq) {
		t.Fatalf("alice first frame id %s, want %d (member sees only own)", f.id, aliceSeq)
	}
	// Prove seq3 (op's) is skipped: the next delivered frame is a fresh alice event.
	sentinel := appendRun(t, b, "alice", "r-alice", "run.state_changed", `{"to":"parked"}`)
	srvA.Nudge()
	if f := readEvent(t, rA); f.id != fmt.Sprint(sentinel) {
		t.Fatalf("alice next frame id %s, want %d (op's row must be skipped)", f.id, sentinel)
	}

	// Operator sees ALL rows including platform-scope.
	_, tsO := newTestServer(t, serverOpts{b: b, auth: fixedIdentity{"op"}})
	rO, doneO := stream(t, testCtx(t), tsO.URL+"/events?after_seq=0", nil)
	defer doneO()
	var ids []string
	for i := 0; i < 3; i++ {
		ids = append(ids, readEvent(t, rO).id)
	}
	if ids[0] != "1" || ids[1] != "2" || ids[2] != "3" {
		t.Fatalf("operator saw ids %v, want [1 2 3] (operator sees all incl platform)", ids)
	}
}

// TestSSERunSubscriptionOwnerCheck proves the OQ1 run-detail owner check: a
// member gets 404 for a missing run and 403 for another owner's run, 200 for
// their own; the operator subscribes to any.
func TestSSERunSubscriptionOwnerCheck(t *testing.T) {
	b := newBackend(t)
	seedUser(t, b, "op", auth.RoleOperator)
	seedUser(t, b, "alice", auth.RoleMember)
	seedRun(t, b, "r-alice", "alice", "", "running", "anthropic")
	seedRun(t, b, "r-op", "op", "", "running", "anthropic")

	status := func(a api.Authenticator, run string) int {
		_, ts := newTestServer(t, serverOpts{b: b, auth: a})
		resp, err := http.Get(ts.URL + "/events?topics=run&run=" + run)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if s := status(fixedIdentity{"alice"}, "r-missing"); s != http.StatusNotFound {
		t.Fatalf("alice → missing run: %d, want 404", s)
	}
	if s := status(fixedIdentity{"alice"}, "r-op"); s != http.StatusForbidden {
		t.Fatalf("alice → op's run: %d, want 403", s)
	}
	if s := status(fixedIdentity{"alice"}, "r-alice"); s != http.StatusOK {
		t.Fatalf("alice → own run: %d, want 200", s)
	}
	if s := status(fixedIdentity{"op"}, "r-alice"); s != http.StatusOK {
		t.Fatalf("operator → any run: %d, want 200", s)
	}
}

// ── C2 redaction (rubric 17, 18) ──

// TestSSEStoreRawServeRedacted proves R19: a planted secret is [REDACTED] on
// the wire but survives verbatim in the stored run_events row.
func TestSSEStoreRawServeRedacted(t *testing.T) {
	b := newBackend(t)
	seedRun(t, b, "r1", "u1", "", "running", "anthropic")
	const secret = "sk-ant-api03-PLANTEDsecret1234567_abcDEF"
	appendRun(t, b, "u1", "r1", "tool.completed", `{"out":"leaked `+secret+` here"}`)
	_, ts := newTestServer(t, serverOpts{b: b})

	r, done := stream(t, testCtx(t), ts.URL+"/events?after_seq=0", nil)
	defer done()
	f := readEvent(t, r)
	if strings.Contains(f.data, secret) {
		t.Fatalf("served frame leaks the secret: %s", f.data)
	}
	if !strings.Contains(f.data, "[REDACTED:anthropic_key]") {
		t.Fatalf("served frame not redacted: %s", f.data)
	}

	// The stored row is untouched (store-raw): the secret is verbatim in the DB.
	rows, err := b.log.After(context.Background(), 0, 10)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	found := false
	for _, e := range rows {
		if strings.Contains(string(e.Payload), secret) {
			found = true
		}
	}
	if !found {
		t.Fatalf("stored run_events row was mutated — the secret must survive verbatim (store-raw, R19)")
	}
}

// redactionEdgeFiles ENUMERATES the observability serving edges — no wildcard,
// so a new file cannot join by accident. B6-1 added the REST half of the edge
// Research/18 §7-C2·1 always scoped as "SSE/REST": reads.go serves run_events
// payload content (the run detail's spawn/routing records, the payload-derived
// list rows) and redacts it at serialization, the writeSnapshotFrame precedent.
// This is scope ARRIVING, not a guardrail change — store-raw / serve-redacted
// is unchanged, and SPEC/PLAN artifacts, receipts and view rows stay
// structurally exempt (§7-C2·2) and go out unwrapped.
// B6-2A adds the third: approvals.go serves the watchdog / drift /
// benchmark-alarm inbox cards, whose strings are lifted out of run_events
// payload bodies, and redacts them per string VALUE before serialization. The
// ask snapshots and effect payloads in the same response are S06/S02 decision
// CONTENT and stay unwrapped — the line is CONTENT vs PROJECTION, exactly as
// the task detail draws it (§38 ruling (b)).
// B6-7 adds the fourth: chatapi.go serves a chat turn's stored OUTCOME, which
// for a query-layer turn is a history.Answer whose rows are lifted out of
// run_events — a projection — and redacts it per serialization. The message
// bodies and session titles in the SAME response are chat's own CONTENT, read
// from chat's own tables, and go out unwrapped: the line is CONTENT vs
// PROJECTION, exactly as the task detail and the memory family draw it
// (§38 ruling (b); §40-C).
var redactionEdgeFiles = map[string]string{
	"sse.go":       "the /events frames and snapshot states (B5-2)",
	"reads.go":     "the payload-bearing REST reads (B6-1 R20)",
	"approvals.go": "the payload-derived inbox cards on the S15.6 approvals list (B6-2A R3)",
	"chatapi.go":   "the payload-derived turn outcomes on the S15.7 assistant family (B6-7)",
}

// TestRedactionObservabilityEdgeOnly proves R16/R18/invariant #2: the redaction
// primitive fires ONLY at the enumerated observability serving edges, never as
// blanket middleware, and the deliverable/review/preview serve paths never
// import it.
func TestRedactionObservabilityEdgeOnly(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	uses := map[string]bool{}
	for _, e := range entries {
		n := e.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		src, err := os.ReadFile(n)
		if err != nil {
			t.Fatalf("read %s: %v", n, err)
		}
		if !strings.Contains(string(src), "redact.") {
			continue
		}
		uses[n] = true
		if _, ok := redactionEdgeFiles[n]; !ok {
			t.Fatalf("redact.* is called in %s — the observability edge is the enumerated set only (R16/R18, B6-1 R20)", n)
		}
	}
	// The enumeration stays honest in the other direction too: a listed file
	// that no longer redacts would leave a stale name standing in for a
	// serving edge that has stopped protecting anything.
	for n, why := range redactionEdgeFiles {
		if !uses[n] {
			t.Fatalf("%s is enumerated as a redaction edge for %s but no longer calls the primitive", n, why)
		}
	}
	// The deliverable/review/preview/intake serve paths must NEVER import the
	// primitive — redaction never touches S13 product content (§7-C2·2).
	for _, pkg := range []string{"review", "preview", "accept", "intake"} {
		files, _ := filepath.Glob(filepath.Join("..", pkg, "*.go"))
		for _, f := range files {
			src, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			if strings.Contains(string(src), "internal/redact") {
				t.Fatalf("%s imports internal/redact — redaction must never touch deliverable content (R18)", f)
			}
		}
	}

	// B5-8B extension (§37): the primitive now has TWO declared importers, and
	// the assertion covers both sides of the line.
	//
	//   * internal/retention  — the write side: the FTS corpus is redacted at
	//                           index time, which is where the C2 property has
	//                           to live (§36 drain D4).
	//   * internal/history    — the read side: the S14.10 search QUERY is
	//                           redacted before it is matched and excerpts are
	//                           redacted on the way out (§30's final bullet).
	//
	// Asserting these POSITIVELY is what keeps the negative above non-vacuous:
	// if the primitive stopped being used by the observability path entirely,
	// the deliverable-path negative would pass for the wrong reason.
	for pkg, why := range map[string]string{
		"retention": "the redact-before-index corpus (§36 drain D4)",
		"history":   "redact-before-match search and bounded redacted excerpts (§30, B5-8B)",
	} {
		files, _ := filepath.Glob(filepath.Join("..", pkg, "*.go"))
		found := false
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			src, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			if strings.Contains(string(src), "internal/redact") {
				found = true
			}
		}
		if !found {
			t.Fatalf("internal/%s no longer imports internal/redact — it is a DECLARED importer for %s; the deliverable-path negative above would pass for the wrong reason", pkg, why)
		}
	}
}

// TestSSEMemberScopedSnapshots proves the OQ1 owner scope on the SNAPSHOT
// projections (D1): a member's board/fleet/inbox snapshots contain NONE of
// another owner's rows, while the operator sees all. Non-tautological — it
// fails if any snapshot's `user_id = ?` predicate is dropped.
func TestSSEMemberScopedSnapshots(t *testing.T) {
	b := newBackend(t)
	seedUser(t, b, "op", auth.RoleOperator)
	seedUser(t, b, "alice", auth.RoleMember)
	seedTask(t, b, "t-alice", "alice", "Alice Task", "doing")
	seedTask(t, b, "t-bob", "op", "Bob Task", "doing")
	seedRun(t, b, "r-alice", "alice", "t-alice", "running", "lane-alice")
	seedRun(t, b, "r-bob", "op", "t-bob", "running", "lane-bob")
	seedAsk(t, b, "ask-alice", "r-alice", "alice", "gate")
	seedAsk(t, b, "ask-bob", "r-bob", "op", "gate")
	seedEffect(t, b, "eff-alice", "r-alice", "alice", "A", "proposed")
	seedEffect(t, b, "eff-bob", "r-bob", "op", "A", "proposed")

	bobRows := []string{"t-bob", "Bob Task", "r-bob", "lane-bob", "ask-bob", "eff-bob"}
	aliceRows := []string{"t-alice", "r-alice", "lane-alice", "ask-alice", "eff-alice"}

	// Member alice: her three snapshots carry her rows and none of bob's.
	_, tsA := newTestServer(t, serverOpts{b: b, auth: fixedIdentity{"alice"}, meter: fakeMeter{}})
	rA, doneA := stream(t, testCtx(t), tsA.URL+"/events?topics=board,fleet,inbox", nil)
	defer doneA()
	aliceAll := ""
	for i := 0; i < 3; i++ {
		aliceAll += mustJSON(t, readSnapshot(t, rA).state)
	}
	for _, tok := range bobRows {
		if strings.Contains(aliceAll, tok) {
			t.Fatalf("member snapshot leaks another owner's row %q: %s", tok, aliceAll)
		}
	}
	for _, tok := range aliceRows {
		if !strings.Contains(aliceAll, tok) {
			t.Fatalf("member snapshot missing own row %q (empty ≠ scoped): %s", tok, aliceAll)
		}
	}

	// Operator: sees every owner's rows.
	_, tsO := newTestServer(t, serverOpts{b: b, auth: fixedIdentity{"op"}, meter: fakeMeter{}})
	rO, doneO := stream(t, testCtx(t), tsO.URL+"/events?topics=board,fleet,inbox", nil)
	defer doneO()
	opAll := ""
	for i := 0; i < 3; i++ {
		opAll += mustJSON(t, readSnapshot(t, rO).state)
	}
	for _, tok := range append(append([]string{}, bobRows...), aliceRows...) {
		if !strings.Contains(opAll, tok) {
			t.Fatalf("operator snapshot missing %q — operator must see all owners: %s", tok, opAll)
		}
	}
}

// TestSSESnapshotStoreRawServeRedacted proves D2: writeSnapshotFrame redacts
// snapshot-projected strings (a run-card last-activity line AND a board task
// title), while the DB rows hold the secret verbatim. Fails if snapshot
// redaction is removed.
func TestSSESnapshotStoreRawServeRedacted(t *testing.T) {
	b := newBackend(t)
	const secret = "sk-ant-api03-SNAPSHOTsecret1234567_xyz"
	seedTask(t, b, "t1", "u1", "title with "+secret, "doing")
	seedRun(t, b, "r1", "u1", "t1", "running", "anthropic")
	appendRun(t, b, "u1", "r1", "run.state_changed", `{"to":"running","line":"activity `+secret+` seen"}`)
	_, ts := newTestServer(t, serverOpts{b: b, meter: fakeMeter{}})

	r, done := stream(t, testCtx(t), ts.URL+"/events?topics=run,board&run=r1", nil)
	defer done()
	runState := mustJSON(t, readSnapshot(t, r).state) // run snapshot (last_activity line)
	if strings.Contains(runState, secret) {
		t.Fatalf("run snapshot leaks the planted secret (snapshot redaction removed?): %s", runState)
	}
	if !strings.Contains(runState, "[REDACTED:anthropic_key]") {
		t.Fatalf("run snapshot last-activity line not redacted: %s", runState)
	}
	boardState := mustJSON(t, readSnapshot(t, r).state) // board snapshot (task title)
	if strings.Contains(boardState, secret) {
		t.Fatalf("board snapshot leaks the planted secret in a task title: %s", boardState)
	}

	// Store-raw: the DB rows still hold the secret verbatim.
	var title string
	if err := b.db.QueryRowContext(context.Background(),
		`SELECT title FROM tasks WHERE task_id = 't1'`).Scan(&title); err != nil {
		t.Fatalf("read title: %v", err)
	}
	if !strings.Contains(title, secret) {
		t.Fatalf("stored task title was mutated — store-raw violated (R19)")
	}
	rows, err := b.log.After(context.Background(), 0, 10)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	raw := false
	for _, e := range rows {
		if strings.Contains(string(e.Payload), secret) {
			raw = true
		}
	}
	if !raw {
		t.Fatalf("stored run_events payload was mutated — store-raw violated (R19)")
	}
}

// TestSSEFleetLaneMeterFields proves D3: the fleet lane carries a real meter
// snapshot (weighted_consumption from the metering seam + the utilization/
// budget_remaining/burn_rate fields), not just a run count.
func TestSSEFleetLaneMeterFields(t *testing.T) {
	b := newBackend(t)
	seedRun(t, b, "r1", "u1", "", "running", "anthropic")
	_, ts := newTestServer(t, serverOpts{b: b, meter: fakeMeter{}})
	r, done := stream(t, testCtx(t), ts.URL+"/events?topics=fleet", nil)
	defer done()
	snap := readSnapshot(t, r)
	lanes, _ := snap.state["lanes"].([]any)
	if len(lanes) == 0 {
		t.Fatalf("fleet snapshot has no lanes: %v", snap.state)
	}
	lane0, _ := lanes[0].(map[string]any)
	for _, k := range []string{"owner", "lane", "active_runs", "weighted_consumption", "utilization", "budget_remaining", "burn_rate"} {
		if _, ok := lane0[k]; !ok {
			t.Fatalf("fleet lane missing meter field %q (§3 meter snapshot): %v", k, lane0)
		}
	}
	if wc, _ := lane0["weighted_consumption"].(float64); wc != 1 {
		t.Fatalf("fleet lane weighted_consumption = %v, want 1 (a real meter read, not a run count)", lane0["weighted_consumption"])
	}
}

// TestRunCardArgsDigestNeverRawArgs proves D4: when a tool event carries only
// raw args (no digest field), args_digest is a sha256 fingerprint — NEVER the
// raw args verbatim (§4 "digest, never raw args").
func TestRunCardArgsDigestNeverRawArgs(t *testing.T) {
	b := newBackend(t)
	seedRun(t, b, "r1", "u1", "", "running", "anthropic")
	appendRun(t, b, "u1", "r1", "tool.completed", `{"tool":"bash","args":"rm -rf /secret/path"}`)
	_, ts := newTestServer(t, serverOpts{b: b, meter: fakeMeter{}})
	r, done := stream(t, testCtx(t), ts.URL+"/events?topics=run&run=r1", nil)
	defer done()
	tool, _ := readSnapshot(t, r).state["tool"].(map[string]any)
	dig, _ := tool["args_digest"].(string)
	if strings.Contains(dig, "rm -rf") || strings.Contains(dig, "/secret/path") {
		t.Fatalf("args_digest surfaced RAW args %q — §4 promises digest, never raw args", dig)
	}
	if !strings.HasPrefix(dig, "sha256:") {
		t.Fatalf("args_digest = %q, want a sha256: fingerprint of the raw args", dig)
	}
}

// ── No-engine-SSE-replay standing conformance (rubric 13; R14 / S14.5) ──
//
// This standing assertion is registered in the S14.5 conformance_registry as
// row "no-engine-sse-replay" (internal/conformance SeedRows; migration 0011,
// B5-4). This test IS its named fixture — /events resume reconstructs the exact
// event sequence from run_events ALONE, with no engine/adapter in the loop.
// opencode ignores Last-Event-ID and the fix was declined upstream, so the
// durable log is the only replay source Sinet ever relies on (S14.3 ¶3, R12
// §7.9).
func TestConformance_NoEngineSSEReplay_ResumeFromLogAlone(t *testing.T) {
	b := newBackend(t)
	want := appendEvents(t, b.log, "e.one", "e.two", "e.three", "e.four", "e.five")
	_, ts := newTestServer(t, serverOpts{b: b}) // dev posture: operator-read, sees all

	// Full replay from ?after_seq=0 — the exact sequence, from run_events only.
	r, done := stream(t, testCtx(t), ts.URL+"/events?after_seq=0", nil)
	for i, wantSeq := range want {
		if f := readEvent(t, r); f.id != fmt.Sprint(wantSeq) {
			t.Fatalf("full replay frame %d id %s, want %d", i, f.id, wantSeq)
		}
	}
	done()

	// Mid-stream resume via Last-Event-ID reconstructs the exact tail.
	r2, done2 := stream(t, testCtx(t), ts.URL+"/events", map[string]string{"Last-Event-ID": fmt.Sprint(want[1])})
	defer done2()
	for _, wantSeq := range want[2:] {
		if f := readEvent(t, r2); f.id != fmt.Sprint(wantSeq) {
			t.Fatalf("resume frame id %s, want %d (durable log is the sole replay source)", f.id, wantSeq)
		}
	}
}

// ── helpers ──

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}

func collectKeys(v any, into map[string]bool) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			into[k] = true
			collectKeys(val, into)
		}
	case []any:
		for _, val := range t {
			collectKeys(val, into)
		}
	}
}
