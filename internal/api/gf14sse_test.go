package api_test

// gf14sse_test.go — P3-GF14 R2.1: the standing proof that a gate-open COMMIT
// reaches a surface that is already listening.
//
// The wedge this pins down (GF9 review M1): a requester sat on a page that said
// "listening" for two minutes while the card they were waiting for had already
// been committed, then reloaded — and the reload killed the drive (R1). The
// gate-open commit writes its rows in ONE transaction (the pipeline's
// `intake.state` and the run's `run.state_changed` park), so this test drives
// the REAL pipeline against a live subscriber and reads the wire. Driving it
// rather than crafting the appends is deliberate (drain r1 F1): the rows are
// read back from the log, so a change to `issueCard`'s commit shape fails this
// test loudly instead of leaving it passing about a shape nothing writes.
//
// Green is the result it is meant to have: the backend delivered, every frame
// carrying `id:` = its event_seq, and M1's residue is the client's. The frames'
// TOPIC TAGS are asserted too, because that is the fact a surface routes on.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// gateOpen drives a REAL intake gate-open on this world and returns the rows it
// actually committed.
//
// The pipeline is the one from internal/intake — no planner and no classifier
// are wired, so the drive reaches the pre-round family question and parks
// there, which is a gate-open with no paid seam in it. Reading the rows back
// from the log rather than crafting them is the point (drain r1 F1): a hand-
// written pair of appends would keep passing after `issueCard`'s commit shape
// moved, and would then be proving something about the test.
func gateOpen(t *testing.T, b *backend, owner, taskID string) (runID string, stateSeq, parkSeq int64) {
	t.Helper()
	ctx := context.Background()
	runs := run.NewStore(b.db, b.log)
	pipe := &intake.Pipeline{
		DB: b.db, Log: b.log, Runs: runs, Ledger: ledger.NewStore(b.db, b.log),
		Settings: b.reg, ArtifactRoot: filepath.Join(t.TempDir(), "artifacts"),
	}
	st, err := pipe.Start(ctx, intake.Request{TaskID: taskID, UserID: owner,
		Title: "GF14 wire proof", Text: "A one-page price list for my candle shop."})
	if err != nil {
		t.Fatalf("intake Start: %v", err)
	}
	// Admission is the scheduler's; the FSM edges are walked directly here, as
	// every dev-mode intake harness does.
	for _, s := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := runs.Transition(ctx, st.RunID, s, run.TransitionOptions{
			Reason: "test admission", Actor: run.ActorPlatform}); err != nil {
			t.Fatalf("admit %s: %v", s, err)
		}
	}
	if st, err = pipe.Advance(ctx, taskID); err != nil {
		t.Fatalf("intake Advance: %v", err)
	}
	if st.OpenAskID == "" {
		t.Fatalf("the drive opened no gate (card %q) — this test needs the gate-open commit", st.OpenAskKind)
	}

	// The two rows the gate-open committed, read back from the log: the last
	// state event of the run, and the park that put it behind the gate.
	seqOf := func(typ, jsonPath, want string) int64 {
		t.Helper()
		var seq int64
		q := `SELECT event_seq FROM run_events WHERE run_id = ? AND type = ?`
		args := []any{st.RunID, typ}
		if jsonPath != "" {
			q += ` AND json_extract(payload, ?) = ?`
			args = append(args, jsonPath, want)
		}
		q += ` ORDER BY event_seq DESC LIMIT 1`
		if err := b.db.QueryRowContext(ctx, q, args...).Scan(&seq); err != nil {
			t.Fatalf("read the %s row the gate-open committed: %v", typ, err)
		}
		return seq
	}
	stateSeq = seqOf(intake.EventState, "", "")
	parkSeq = seqOf(run.EventState, "$.to", string(run.StateParked))
	if stateSeq == 0 || parkSeq == 0 {
		t.Fatal("the gate-open committed neither of the rows this test reads")
	}
	return st.RunID, stateSeq, parkSeq
}

// awaitFrames reads until every wanted event_seq has arrived, failing on the
// stream's own deadline rather than on a sleep. It returns each frame's topics.
func awaitFrames(t *testing.T, r *bufio.Reader, want []int64) map[int64][]string {
	t.Helper()
	got := make(map[int64][]string, len(want))
	pending := make(map[int64]bool, len(want))
	for _, seq := range want {
		pending[seq] = true
	}
	for len(pending) > 0 {
		f := readEvent(t, r)
		var w struct {
			Seq    int64    `json:"seq"`
			Topics []string `json:"topics"`
		}
		if err := json.Unmarshal([]byte(f.data), &w); err != nil {
			t.Fatalf("frame data: %v (%s)", err, f.data)
		}
		if f.id != fmt.Sprint(w.Seq) {
			t.Fatalf("frame id = %q but seq = %d — the id IS the event_seq (S14.3 frame contract)", f.id, w.Seq)
		}
		if pending[w.Seq] {
			delete(pending, w.Seq)
			got[w.Seq] = w.Topics
		}
	}
	return got
}

// TestGF14GateOpenReachesAConnectedSubscriber is R2.1 on the unfiltered relay:
// a committed gate-open cannot be missed server-side while the connection
// stands.
func TestGF14GateOpenReachesAConnectedSubscriber(t *testing.T) {
	b := newBackend(t)
	seedUser(t, b, "op", auth.RoleOperator)
	_, ts := newTestServer(t, serverOpts{b: b, auth: fixedIdentity{id: "op"}})

	r, done := stream(t, testCtx(t), ts.URL+"/events?after_seq=0", nil)
	defer done()

	_, stateSeq, parkSeq := gateOpen(t, b, "op", "t-gf14-sse")
	tags := awaitFrames(t, r, []int64{stateSeq, parkSeq})

	for seq, want := range map[int64]string{stateSeq: "run", parkSeq: "board"} {
		if !hasTag(tags[seq], want) {
			t.Fatalf("event %d arrived tagged %v, want it to carry %q", seq, tags[seq], want)
		}
	}
}

// TestGF14GateOpenReachesTheSubscribedSurface is R2.1 on the topic-subscribed
// connection a surface actually opens. The SPA merges its panes into ONE
// stream, so the subscribe set is the union — and the routing fact this pins is
// that a gate-open rides the BOARD tag: a consumer that filtered strictly on
// `inbox` would never learn that a card had opened.
func TestGF14GateOpenReachesTheSubscribedSurface(t *testing.T) {
	b := newBackend(t)
	seedUser(t, b, "op", auth.RoleOperator)
	_, ts := newTestServer(t, serverOpts{b: b, auth: fixedIdentity{id: "op"}})

	r, done := stream(t, testCtx(t), ts.URL+"/events?topics=board,inbox", nil)
	defer done()
	readSnapshot(t, r) // board
	readSnapshot(t, r) // inbox

	_, stateSeq, parkSeq := gateOpen(t, b, "op", "t-gf14-sse2")
	tags := awaitFrames(t, r, []int64{stateSeq, parkSeq})

	for _, seq := range []int64{stateSeq, parkSeq} {
		if hasTag(tags[seq], "inbox") {
			continue
		}
		if !hasTag(tags[seq], "board") {
			t.Fatalf("event %d reached a board+inbox subscriber tagged %v — it matched neither topic", seq, tags[seq])
		}
	}
}
