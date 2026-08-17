package claudecli_test

// P3-RW-16 acceptance test (grounding-committed RED, Amendment-A window):
// the pump's oversized-stream-line posture. NOT the drain-r1 crash's root
// cause (the evidence exonerated the read path — see the brief), but a
// latent defect the investigation surfaced on the same read path: today a
// single stdout line larger than scanBufCap stops the bufio.Scanner cold
// (bufio.ErrTooLong), the rest of the stream — including the terminal
// result envelope — is never read, and the child wedges on its blocked
// stdout write, so cmd.Wait never returns and the session hangs until
// something kills the process.
//
// Contract under test (brief P3/briefs/P3-RW-16.md R5, S03.1
// forward-tolerance MUST): an oversized line is one more skipped-line
// class — discarded through its newline, logged loudly, parsing CONTINUES
// — so the session still completes on the envelope and nothing wedges.

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
)

func TestRW16OversizedStreamLineIsSkippedNotAWedge(t *testing.T) {
	// 12 MiB > the adapter's 10 MiB scanBufCap.
	a := fakeEngineAdapter(t, "rw16-overrun.jsonl",
		"SINET_FAKE_BIGLINE="+strconv.Itoa(12<<20))
	ctx := context.Background()
	sess, err := a.Start(ctx, e2eRequest(t, "rw16-overrun"))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Always run the cancel ladder on the way out: on today's RED behavior
	// the child is blocked mid-write and only a kill unwedges it.
	defer func() { _ = sess.Cancel(context.Background()) }()

	go func() {
		for range sess.Events() {
		}
	}()

	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := sess.Wait(wctx)
	if err != nil {
		t.Fatalf("Wait: %v — the oversized line wedged the session (R5: skip the line, keep reading, never block on the child)", err)
	}
	if out.Kind != adapters.OutcomeCompleted {
		t.Fatalf("outcome %s (%s), want completed: the terminal envelope after the oversized line must still be read (R5)", out.Kind, out.Detail)
	}
	if out.ResultText != `{"ok":true}` {
		t.Fatalf("ResultText %q, want the envelope's result field verbatim (RW-9 precedence unchanged)", out.ResultText)
	}
}
