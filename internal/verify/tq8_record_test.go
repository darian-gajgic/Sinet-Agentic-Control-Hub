package verify_test

// tq8_record_test.go — P3-TQ-8 R6 on the event type the platform actually
// mints. The verdict row was renamed verify.round → verdict.recorded when the
// S14.2 event contract landed (verify.EventRound; eventlog/contract.go
// "renamed from verify.round"), and nothing writes the old name any more, so
// the record half of R6 is pinned here against verify.EventRound: the
// keep-forever verdict row carries judge_saw verbatim, and a round that never
// judged writes no verdict row at all.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

func TestTQ8JudgeSawRidesTheVerdictRow(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	j := &fakeJudge{}
	v := f.verifier(j, &scriptRunner{}, passPack())
	rc, omitted := tq8WideTree()
	v.Change = &tq8Change{rc: rc}
	d := tq8Deliverable("t1", "r1")
	d.Revision, d.PrevContent = 2, "# report rev 1\n"

	out, err := v.Verify(context.Background(), input(d))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	rows := f.events(verify.EventRound)
	if len(rows) != 1 {
		t.Fatalf("%s rows %d, want 1", verify.EventRound, len(rows))
	}
	var payload struct {
		JudgeSaw *verify.JudgeSaw `json:"judge_saw"`
	}
	if err := json.Unmarshal(rows[0].Payload, &payload); err != nil {
		t.Fatalf("decode %s: %v", verify.EventRound, err)
	}
	saw := payload.JudgeSaw
	if saw == nil {
		t.Fatalf("the verdict row carries no judge_saw (Spec S07.11: a verdict is kept with what was checked)")
	}
	if saw.Kind != "tree" || !saw.Truncated || saw.Files != 7 || saw.DiffsShown != 3 || saw.ContentShown != 1 {
		t.Fatalf("recorded judge_saw = %+v", saw)
	}
	if strings.Join(saw.DiffsOmitted, ",") != strings.Join(omitted, ",") {
		t.Fatalf("recorded DiffsOmitted %v, want %v", saw.DiffsOmitted, omitted)
	}
	// The row says the same as the round record the drain returned.
	rec := out.Rounds[0].JudgeSaw
	if rec == nil {
		t.Fatal("the round record carries no judge_saw")
	}
	wantJSON, _ := json.Marshal(rec)
	gotJSON, _ := json.Marshal(saw)
	if string(wantJSON) != string(gotJSON) {
		t.Fatalf("round record %s differs from the recorded row %s", wantJSON, gotJSON)
	}
}

func TestTQ8ALostPinWritesNoVerdictRow(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	j := &fakeJudge{}
	v := f.verifier(j, &scriptRunner{}, passPack())
	drift := errors.New("content drift: the project store no longer holds the saved files that dlv-t1 pins at s1")
	v.Change = &tq8Change{err: drift}

	if _, err := v.Verify(context.Background(), input(tq8Deliverable("t1", "r1"))); !errors.Is(err, drift) {
		t.Fatalf("Verify err = %v, want the seam's drift error surfaced", err)
	}
	if n := len(f.events(verify.EventRound)); n != 0 {
		t.Fatalf("%d %s rows written for a round that never judged", n, verify.EventRound)
	}
}
