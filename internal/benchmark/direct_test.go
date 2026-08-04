package benchmark_test

// direct_test.go — the storage half of the BENCH-REG §2 direct-arm capture and
// the pairs-by-state listing the §3 driver reads (P3-B6-2C, migration 0018).
//
// The properties under test are the ones the practice's honesty rests on: an
// absent capture is an ABSENCE (never an empty answer), a captured empty answer
// is an ANSWER (never an absence), and a pair whose §14 record is written can
// never take a late capture — a committed result is not recomputed (§17).

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/benchmark"
)

// TestDirectCaptureAbsentIsAnAbsence: with no capture the read is the package's
// own sentinel, so every caller can branch on it — and the shell's DirectText
// seam turns it into the honest refusal that keeps the pair unrendered.
func TestDirectCaptureAbsentIsAnAbsence(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	pair := h.sampledPair(t, "t-absent")
	if _, err := h.practice(t).DispatchDirectArm(ctx, pair.PairID); err != nil {
		t.Fatalf("DispatchDirectArm: %v", err)
	}
	runID := benchmark.DirectRunID(pair.PairID)

	body, err := h.store.CapturedDirectText(ctx, runID)
	if !errors.Is(err, benchmark.ErrNoDirectCapture) {
		t.Fatalf("a dispatched-but-unanswered arm reads %q, %v — want the absence sentinel", body, err)
	}
	if body != "" {
		t.Errorf("the absent read returned %q — a placeholder arm would make every statistic fiction", body)
	}
	// A run no pair claims is the same absence, not a different error class: a
	// caller must not have to tell "never dispatched" from "never answered" to
	// know it has nothing to render.
	if _, err := h.store.CapturedDirectText(ctx, "bp-nobody.direct"); !errors.Is(err, benchmark.ErrNoDirectCapture) {
		t.Errorf("an unknown direct run reads %v, want the same absence", err)
	}
}

// TestDirectCaptureRoundTripsIncludingSilence: the arm's text is stored VERBATIM
// and an EMPTY answer is a real single-shot outcome, distinguishable from having
// no answer at all. That distinction is the whole reason 0018's column is
// nullable.
func TestDirectCaptureRoundTripsIncludingSilence(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	pair := h.sampledPair(t, "t-capture")
	if _, err := h.practice(t).DispatchDirectArm(ctx, pair.PairID); err != nil {
		t.Fatalf("DispatchDirectArm: %v", err)
	}
	runID := benchmark.DirectRunID(pair.PairID)

	// An empty capture: the arm ran and produced nothing.
	took, err := h.store.CaptureDirectText(ctx, runID, "")
	if err != nil || !took {
		t.Fatalf("CaptureDirectText(empty) = %v, %v — an empty answer is an outcome", took, err)
	}
	body, err := h.store.CapturedDirectText(ctx, runID)
	if err != nil {
		t.Fatalf("a captured silence must read back as an answer, not an absence: %v", err)
	}
	if body != "" {
		t.Errorf("read back %q, want the empty answer that was stored", body)
	}

	// A real answer, with the kind of content a template must never truncate.
	const answer = "# heading\n\nline one\n\n  * a bullet\n\ttab\n" + "long tail: 0123456789"
	took, err = h.store.CaptureDirectText(ctx, runID, answer)
	if err != nil || !took {
		t.Fatalf("CaptureDirectText = %v, %v", took, err)
	}
	body, err = h.store.CapturedDirectText(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if body != answer {
		t.Errorf("the capture is not verbatim:\n got: %q\nwant: %q", body, answer)
	}
	// A capture for no run at all is refused rather than written somewhere.
	if _, err := h.store.CaptureDirectText(ctx, "", "x"); err == nil {
		t.Error("a capture with no run id must be refused")
	}
}

// TestDirectCaptureNeverTouchesAResult: a capture arriving after the pair's §14
// record — a raced leg, a recovery fork, an operator replaying something — takes
// nothing. A committed result is never recomputed under a later registration
// (BENCH-REG §17), and the 0014/0015 freezes are the backstop behind this rule
// rather than the mechanism.
func TestDirectCaptureNeverTouchesAResult(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)

	recorded := h.recordPair(t, pairSpec{
		task: "t-recorded", choice: benchmark.ChoiceA, guess: benchmark.SideA,
		directModel: "m1", units: 1,
	})
	took, err := h.store.CaptureDirectText(ctx, benchmark.DirectRunID(recorded.PairID), "too late")
	if err != nil {
		t.Fatalf("a late capture must be a no-op, not an error: %v", err)
	}
	if took {
		t.Error("a recorded pair took a late capture — its §14 record was computed from something else")
	}

	declined := h.recordPair(t, pairSpec{task: "t-declined", decline: true})
	took, err = h.store.CaptureDirectText(ctx, benchmark.DirectRunID(declined.PairID), "too late")
	if err != nil {
		t.Fatalf("a late capture on a declined pair must be a no-op: %v", err)
	}
	if took {
		t.Error("a declined pair took a late capture — a decline is as final as a record")
	}
}

// TestPairsInStateIsAReadInDrawOrder: the driver's dueness is the STORED state,
// so the listing is exactly that read — oldest draw first, bounded, and moving
// nothing.
func TestPairsInStateIsAReadInDrawOrder(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	var ids []string
	for _, task := range []string{"t-1", "t-2", "t-3"} {
		p := h.sampledPair(t, task)
		ids = append(ids, p.PairID)
		h.clk.advance(time.Minute)
	}

	sampled, err := h.store.PairsInState(ctx, benchmark.StateSampled, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(sampled) != 3 {
		t.Fatalf("listed %d sampled pairs, want 3", len(sampled))
	}
	for i, p := range sampled {
		if p.PairID != ids[i] {
			t.Errorf("position %d is %q, want %q — the listing is draw order so the bound cannot starve a pair",
				i, p.PairID, ids[i])
		}
	}
	// Bounded, and a skipped pair is simply the next pass's first item.
	first, err := h.store.PairsInState(ctx, benchmark.StateSampled, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || first[0].PairID != ids[0] {
		t.Fatalf("the bound did not hold in draw order: %+v", first)
	}
	// It advances nothing: the states are unchanged after two reads.
	again, err := h.store.PairsInState(ctx, benchmark.StateSampled, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 3 {
		t.Errorf("a read moved a pair: %d sampled pairs remain, want 3", len(again))
	}
	// And a dispatched pair leaves the sampled list for the dispatched one — the
	// two halves of the driver's walk never see the same pair twice.
	if _, err := h.practice(t).DispatchDirectArm(ctx, ids[0]); err != nil {
		t.Fatal(err)
	}
	sampled, _ = h.store.PairsInState(ctx, benchmark.StateSampled, 0)
	dispatched, _ := h.store.PairsInState(ctx, benchmark.StateDispatched, 0)
	if len(sampled) != 2 || len(dispatched) != 1 || dispatched[0].PairID != ids[0] {
		t.Errorf("after one dispatch: %d sampled / %d dispatched, want 2 / 1", len(sampled), len(dispatched))
	}
	if dispatched[0].DirectRunID != benchmark.DirectRunID(ids[0]) {
		t.Errorf("the listed pair carries direct run %q — the driver keys the capture on it", dispatched[0].DirectRunID)
	}
}

// TestDirectCaptureIsNeverAnEventBody: refs-not-blobs, asserted where it can
// actually be violated. The arm's text is a durable artifact of the practice and
// lives on the pair row; a model's full answer in an event payload is precisely
// the bulky body ⚙ state.event_payload_cap exists to keep out of the log.
func TestDirectCaptureIsNeverAnEventBody(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	pair := h.sampledPair(t, "t-blob")
	if _, err := h.practice(t).DispatchDirectArm(ctx, pair.PairID); err != nil {
		t.Fatal(err)
	}
	const marker = "UNMISTAKABLE-ARM-BODY-MARKER"
	if _, err := h.store.CaptureDirectText(ctx, benchmark.DirectRunID(pair.PairID),
		strings.Repeat(marker+" ", 64)); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := h.db.QueryRowContext(ctx,
		`SELECT count(*) FROM run_events WHERE payload LIKE ?`, "%"+marker+"%").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("%d event payload(s) carry the arm's body — the capture is a row, never an event (P-T07-5)", n)
	}
}
