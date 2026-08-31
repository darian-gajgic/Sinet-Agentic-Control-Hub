package api_test

// gf14sse_test.go — P3-GF14 R2.1: the standing proof that a gate-open COMMIT
// reaches a surface that is already listening.
//
// The wedge this pins down (GF9 review M1): a requester sat on a page that said
// "listening" for two minutes while the card they were waiting for had already
// been committed, then reloaded — and the reload killed the drive (R1). The
// gate-open commit writes exactly two event rows in ONE transaction (the
// pipeline's `intake.state` and the run's `run.state_changed` park), so this
// test drives that commit shape against a live subscriber and reads the wire.
//
// Green is the result it is meant to have: the backend delivered, every frame
// carrying `id:` = its event_seq, and M1's residue is the client's. The frames'
// TOPIC TAGS are asserted too, because that is the fact a surface routes on.

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
)

// commitGateOpen writes the two rows an intake gate-open commits, in one
// transaction, exactly as internal/intake's issueCard does.
func commitGateOpen(t *testing.T, b *backend, owner, runID string) (stateSeq, parkSeq int64) {
	t.Helper()
	ctx := context.Background()
	if err := b.db.WriteTx(ctx, func(tx *sql.Tx) error {
		var err error
		stateSeq, err = b.log.AppendTx(ctx, tx, eventlog.Append{
			RunID: runID, Generation: 0, UserID: owner, Type: "intake.state", SchemaVersion: 1,
			Payload: json.RawMessage(`{"phase":"interview","open_ask_kind":"interview"}`),
		})
		if err != nil {
			return err
		}
		parkSeq, err = b.log.AppendTx(ctx, tx, eventlog.Append{
			RunID: runID, Generation: 0, UserID: owner, Type: "run.state_changed", SchemaVersion: 1,
			Payload: json.RawMessage(`{"from":"running","to":"parked"}`),
		})
		return err
	}); err != nil {
		t.Fatalf("gate-open commit: %v", err)
	}
	return stateSeq, parkSeq
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
	seedTask(t, b, "t-gf14-sse", "op", "GF14 wire proof", "doing")
	seedRun(t, b, "t-gf14-sse.intake", "op", "t-gf14-sse", "running", "anthropic")
	_, ts := newTestServer(t, serverOpts{b: b, auth: fixedIdentity{id: "op"}})

	r, done := stream(t, testCtx(t), ts.URL+"/events?after_seq=0", nil)
	defer done()

	stateSeq, parkSeq := commitGateOpen(t, b, "op", "t-gf14-sse.intake")
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
	seedTask(t, b, "t-gf14-sse2", "op", "GF14 wire proof", "doing")
	seedRun(t, b, "t-gf14-sse2.intake", "op", "t-gf14-sse2", "running", "anthropic")
	_, ts := newTestServer(t, serverOpts{b: b, auth: fixedIdentity{id: "op"}})

	r, done := stream(t, testCtx(t), ts.URL+"/events?topics=board,inbox", nil)
	defer done()
	readSnapshot(t, r) // board
	readSnapshot(t, r) // inbox

	stateSeq, parkSeq := commitGateOpen(t, b, "op", "t-gf14-sse2.intake")
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
