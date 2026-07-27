package benchmark_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/benchmark"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/scheduler"
)

// sampledPair runs the §4 hook once and returns the pair it drew.
func (h *harness) sampledPair(t *testing.T, taskID string) benchmark.Pair {
	t.Helper()
	ctx := context.Background()
	h.eligibleTask(t, taskID)
	if pass, err := h.sampler(0).Tail(ctx); err != nil || pass.Sampled != 1 {
		t.Fatalf("sampling pass = %+v err=%v, want 1 sampled", pass, err)
	}
	pairs, err := h.store.PairsInDomain(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pairs {
		if p.TaskID == taskID {
			return p
		}
	}
	t.Fatalf("no pair was drawn for task %q", taskID)
	return benchmark.Pair{}
}

// TestDirectArmIsOrdinaryBackgroundWorkBilledToTheRequester is acceptance item
// 6: the duplicate rides the EXISTING admission surface as ClassBackground under
// the requester's own user_id. No benchmark-special admission path, priority or
// exemption exists (G2 Def.4; Spec S10.4).
func TestDirectArmIsOrdinaryBackgroundWorkBilledToTheRequester(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	pair := h.sampledPair(t, "t1")

	p := h.practice(t)
	q := &recordingQueue{inner: h.scheduler(t)}
	p.Queue = q
	got, err := p.DispatchDirectArm(ctx, pair.PairID)
	if err != nil {
		t.Fatalf("DispatchDirectArm: %v", err)
	}

	// Enqueued exactly once, as background.
	if len(q.runIDs) != 1 {
		t.Fatalf("the direct arm was enqueued %d times, want exactly one single-shot admission (BENCH-REG §2)", len(q.runIDs))
	}
	if q.class[0] != scheduler.ClassBackground {
		t.Errorf("direct arm enqueued as %q, want %q — it throttles like any background work", q.class[0], scheduler.ClassBackground)
	}
	if q.runIDs[0] != benchmark.DirectRunID(pair.PairID) {
		t.Errorf("enqueued %q, want the pair's direct-arm run", q.runIDs[0])
	}

	// The run row is the requester's, on their own frontier surface.
	r, err := h.runs.Get(ctx, got.DirectRunID)
	if err != nil {
		t.Fatalf("the direct arm has no run row: %v", err)
	}
	if r.UserID != "alice" {
		t.Errorf("direct-arm run owner = %q, want the requester (3.4: the duplicate bills them)", r.UserID)
	}
	if r.TaskID != "t1" {
		t.Errorf("direct-arm run task = %q, want the sampled task", r.TaskID)
	}

	// The queue row carries the same owner — admission is owner-attributed
	// (15.6) and pressure/spawn gating therefore sees an ordinary background
	// item of alice's.
	var owner, status string
	if err := h.db.QueryRowContext(ctx,
		`SELECT user_id, status FROM queue WHERE run_id = ?`, got.DirectRunID).Scan(&owner, &status); err != nil {
		t.Fatalf("the direct arm produced no queue row — it must ride the ordinary enqueue surface: %v", err)
	}
	if owner != "alice" {
		t.Errorf("queue row owner = %q, want alice", owner)
	}
	if got.State != benchmark.StateDispatched {
		t.Errorf("pair state = %q, want %q", got.State, benchmark.StateDispatched)
	}
}

// TestSchedulerSatisfiesTheQueueSeam pins that the seam the direct arm rides is
// the production scheduler's own verb, not a benchmark-side reimplementation.
func TestSchedulerSatisfiesTheQueueSeam(t *testing.T) {
	var _ benchmark.Queue = (*scheduler.Scheduler)(nil)
}

// TestDirectArmIsSingleShotWithNoPlatformMachinery is acceptance item 7's
// structural half (OQ9): ONE engine invocation, no Sinet retry, no follow-up
// turn, no verification round, no worker template and no knowledge injection.
// The package must contain none of the machinery that would add them.
func TestDirectArmIsSingleShotWithNoPlatformMachinery(t *testing.T) {
	code := readPackageCode(t)
	if n := strings.Count(code, "Enqueue"); n == 0 {
		t.Fatal("the code scan sees no Enqueue at all — it would pass vacuously")
	}
	for _, forbidden := range []string{
		"Retry", "retry", "Rerun", "FollowUp", "followUp", "Verifier", "Verify",
		"WorkerTemplate", "workerTemplate", "InjectKnowledge", "Compose",
	} {
		if strings.Contains(code, forbidden+"\n") {
			t.Errorf("the package's code names %q — the direct arm is single-shot and gets NONE of the platform's machinery; the asymmetry IS the treatment under test (BENCH-REG §2/§6)", forbidden)
		}
	}
	// A second dispatch is refused by state, so a caller cannot turn one arm
	// into two by calling twice.
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	pair := h.sampledPair(t, "t1")
	p := h.practice(t)
	if _, err := p.DispatchDirectArm(ctx, pair.PairID); err != nil {
		t.Fatal(err)
	}
	if _, err := p.DispatchDirectArm(ctx, pair.PairID); !errors.Is(err, benchmark.ErrPairState) {
		t.Errorf("a second dispatch returned %v, want ErrPairState — one run, single-shot", err)
	}
}

// TestUniformTemplateStripsTellsAndNeverTruncates is acceptance item 8. The
// template normalizes FORMAT; it never removes content, and "length never
// truncated" is frozen text.
func TestUniformTemplateStripsTellsAndNeverTruncates(t *testing.T) {
	in := "---\ntitle: Sinet deliverable\nattempt: 3\n---\n" +
		"# Result\r\n\r\n\r\n\r\n* first point   \n+ second point\t\n\nbody text\n\n\n"
	got := benchmark.Render(in)

	// Scaffolding stripped: the front-matter metadata block is gone.
	for _, tell := range []string{"title: Sinet deliverable", "attempt: 3"} {
		if strings.Contains(got, tell) {
			t.Errorf("the template left the scaffolding tell %q", tell)
		}
	}
	// Bullet glyphs normalized to one house-neutral form.
	if strings.Contains(got, "* first") || strings.Contains(got, "+ second") {
		t.Errorf("bullet glyphs were not normalized:\n%s", got)
	}
	// CONTENT is intact, in order — nothing truncated.
	for _, want := range []string{"# Result", "- first point", "- second point", "body text"} {
		if !strings.Contains(got, want) {
			t.Errorf("the template lost content %q from:\n%s", want, got)
		}
	}
	if strings.Contains(got, "\r") {
		t.Error("line endings were not normalized")
	}
	if strings.Contains(got, "\n\n\n") {
		t.Error("blank-line runs were not collapsed")
	}

	// The load-bearing negative: a very long input comes back whole. A
	// truncating template would report a length confound it had itself created.
	long := strings.Repeat("word ", 200000)
	if out := benchmark.Render(long); len(out) < len(long)-1 {
		t.Errorf("the template shortened a %d-byte input to %d — length is NEVER truncated (BENCH-REG §3.2)", len(long), len(out))
	}
	// An unclosed leading `---` is a horizontal rule, not front matter.
	if out := benchmark.Render("---\njust a rule\nand text"); !strings.Contains(out, "just a rule") {
		t.Error("an unclosed --- must not be treated as front matter")
	}
}

// TestPositionIsRandomizedAndStoredWithTheLengthConfound is the rest of item 8.
func TestPositionIsRandomizedAndStoredWithTheLengthConfound(t *testing.T) {
	for _, c := range []struct {
		draw     int
		wantSide benchmark.Side
	}{{0, benchmark.SideA}, {1, benchmark.SideB}} {
		h := newHarness(t)
		ctx := context.Background()
		h.user(t, "alice", true)
		pair := h.sampledPair(t, "t1")
		p := h.practice(t)
		p.Rand = func(int) int { return c.draw }
		p.Platform = func(context.Context, string) (string, error) { return "the platform answer, longer", nil }
		p.Direct = func(context.Context, string) (string, error) { return "direct", nil }
		if _, err := p.DispatchDirectArm(ctx, pair.PairID); err != nil {
			t.Fatal(err)
		}
		got, err := p.RenderBlind(ctx, pair.PairID)
		if err != nil {
			t.Fatalf("RenderBlind: %v", err)
		}
		if got.PlatformSide() != c.wantSide {
			t.Errorf("draw %d put the platform on side %s, want %s", c.draw, got.PlatformSide(), c.wantSide)
		}
		// The length confound is RECORDED per arm, never used to trim either.
		if got.LengthPlat != len("the platform answer, longer") || got.LengthDirect != len("direct") {
			t.Errorf("length confound recorded as platform=%d direct=%d, want the rendered lengths",
				got.LengthPlat, got.LengthDirect)
		}
		// Both blind artifacts persist and are referenceable.
		pending, err := h.store.PendingVerdicts(ctx, "alice")
		if err != nil {
			t.Fatal(err)
		}
		if len(pending) != 1 || pending[0].RenderA == "" || pending[0].RenderB == "" {
			t.Fatalf("the rendered blind artifacts did not persist: %+v", pending)
		}
		if benchmark.ArtifactRef(pair.PairID, benchmark.SideA) == benchmark.ArtifactRef(pair.PairID, benchmark.SideB) {
			t.Error("the two blind artifacts must have distinct refs")
		}
	}
}

// TestVerdictRefusesAMissingGuessStructurally is acceptance item 9. §3.3 is
// frozen: "No verdict without a guess; the guess is never optional." The
// refusal is STRUCTURAL — the zero Answer cannot be formed, and NewAnswer
// requires the guess.
func TestVerdictRefusesAMissingGuessStructurally(t *testing.T) {
	// The constructor refuses a guess-less form.
	for _, guess := range []benchmark.Side{"", "maybe"} {
		if _, err := benchmark.NewAnswer(benchmark.ChoiceA, guess); !errors.Is(err, benchmark.ErrGuessRequired) {
			t.Errorf("NewAnswer(A, %q) = %v, want ErrGuessRequired", guess, err)
		}
	}
	// A tie still needs a guess: the blindness measurement is per VOTE.
	if _, err := benchmark.NewAnswer(benchmark.ChoiceTie, ""); !errors.Is(err, benchmark.ErrGuessRequired) {
		t.Error("a tie verdict must also carry the mandatory guess (BENCH-REG §5)")
	}
	// The record verb refuses the ZERO Answer, which is the only Answer a
	// caller can produce without the constructor.
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	pair := h.readyPair(t, "t1")
	p := h.practice(t)
	if _, err := p.RecordVerdict(ctx, pair.PairID, benchmark.Answer{}); !errors.Is(err, benchmark.ErrGuessRequired) {
		t.Errorf("RecordVerdict with an unformed Answer = %v, want ErrGuessRequired", err)
	}
	// And the type has no exported field a caller could set instead.
	rt := reflect.TypeOf(benchmark.Answer{})
	for i := range rt.NumField() {
		if rt.Field(i).IsExported() {
			t.Errorf("Answer.%s is exported — a guess-less verdict must be INEXPRESSIBLE, not merely validated", rt.Field(i).Name)
		}
	}
	// The one act carries both: verdict AND guess.
	a, err := benchmark.NewAnswer(benchmark.ChoiceA, benchmark.SideB)
	if err != nil {
		t.Fatal(err)
	}
	if a.Choice() != benchmark.ChoiceA || a.Guess() != benchmark.SideB || !a.Formed() {
		t.Errorf("the §3.3 form did not carry both halves: %+v", a)
	}
}

// readyPair takes a pair all the way to `rendered` — the state a verdict needs.
func (h *harness) readyPair(t *testing.T, taskID string) benchmark.Pair {
	t.Helper()
	ctx := context.Background()
	pair := h.sampledPair(t, taskID)
	p := h.practice(t)
	if _, err := p.DispatchDirectArm(ctx, pair.PairID); err != nil {
		t.Fatal(err)
	}
	got, err := p.RenderBlind(ctx, pair.PairID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// TestRevealOnlyAfterTheRecord is acceptance item 10: no pre-record read
// surface exposes arm identity, position or model identity, and the reveal is a
// read of the COMMITTED record.
func TestRevealOnlyAfterTheRecord(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	pair := h.readyPair(t, "t1")
	p := h.practice(t)

	// Structural: the pending surface has no field that could carry arm
	// identity. This is the assertion that survives refactors.
	rt := reflect.TypeOf(benchmark.PendingPair{})
	for i := range rt.NumField() {
		name := strings.ToLower(rt.Field(i).Name)
		for _, leak := range []string{"arm", "position", "model", "platform", "direct", "run"} {
			if strings.Contains(name, leak) {
				t.Errorf("PendingPair.%s could reveal arm identity before the record lands (BENCH-REG §3.4)", rt.Field(i).Name)
			}
		}
	}
	pending, err := h.store.PendingVerdicts(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("%d pending verdicts, want 1", len(pending))
	}

	// Before the record there is nothing to reveal.
	if _, err := p.Reveal(ctx, pair.PairID); !errors.Is(err, benchmark.ErrUnknownPair) {
		t.Errorf("Reveal before the record = %v, want a refusal — arm identity is revealed only afterwards", err)
	}

	a, err := benchmark.NewAnswer(benchmark.ChoiceA, benchmark.SideA)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.RecordVerdict(ctx, pair.PairID, a); err != nil {
		t.Fatalf("RecordVerdict: %v", err)
	}

	// After the record, the reveal is a read of the committed event — not a
	// flag on the row. Deleting nothing and mutating nothing: the row is frozen.
	rev, err := p.Reveal(ctx, pair.PairID)
	if err != nil {
		t.Fatalf("Reveal after the record: %v", err)
	}
	if rev.Seq == 0 {
		t.Error("the reveal must come from a committed event row")
	}
	if rev.PlatformSide != benchmark.SideA {
		t.Errorf("revealed platform side = %s, want A (the fixed test draw)", rev.PlatformSide)
	}
	// The pending list is now empty: a recorded pair is no longer awaiting one.
	pending, err = h.store.PendingVerdicts(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Errorf("%d pending verdicts after recording, want 0", len(pending))
	}
	// §17: a recorded pair is frozen — results are never recomputed.
	if _, err := p.RecordVerdict(ctx, pair.PairID, a); !errors.Is(err, benchmark.ErrPairState) {
		t.Errorf("re-recording a pair = %v, want a refusal (BENCH-REG §17)", err)
	}
}

// TestRecordAndStateCommitTogether is acceptance item 10's transactional half
// (R23): the §14 append and the row's move to `recorded` are one WriteTx, so a
// pair is never revealed without its record and never recorded without moving.
func TestRecordAndStateCommitTogether(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	pair := h.readyPair(t, "t1")
	p := h.practice(t)

	// A verdict on a pair that is not rendered is refused BEFORE anything is
	// appended, so a rejected act leaves no orphan record.
	other := h.sampledPair(t, "t2")
	a, err := benchmark.NewAnswer(benchmark.ChoiceB, benchmark.SideA)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.RecordVerdict(ctx, other.PairID, a); !errors.Is(err, benchmark.ErrPairState) {
		t.Fatalf("a verdict on an unrendered pair = %v, want ErrPairState", err)
	}
	var orphans int
	if err := h.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM run_events WHERE type = ? AND json_extract(payload,'$.pair_id') = ?`,
		benchmark.EventPairRecorded, other.PairID).Scan(&orphans); err != nil {
		t.Fatal(err)
	}
	if orphans != 0 {
		t.Errorf("a refused verdict left %d record(s) behind", orphans)
	}

	if _, err := p.RecordVerdict(ctx, pair.PairID, a); err != nil {
		t.Fatal(err)
	}
	got, err := h.store.Pair(ctx, pair.PairID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != benchmark.StateRecorded || got.RecordedTS.IsZero() {
		t.Errorf("the row did not move with the record: %+v", got)
	}
	var records int
	if err := h.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM run_events WHERE type = ? AND json_extract(payload,'$.pair_id') = ?`,
		benchmark.EventPairRecorded, pair.PairID).Scan(&records); err != nil {
		t.Fatal(err)
	}
	if records != 1 {
		t.Errorf("%d records for the pair, want exactly 1", records)
	}
}

// TestRecordIsVerbatimCompleteAgainstSection14 is acceptance item 12: the §14
// record schema minimums, all present.
func TestRecordIsVerbatimCompleteAgainstSection14(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	pair := h.readyPair(t, "t1")
	// Give both arms measured consumption and distinct observed identities.
	h.checkpoint(t, pair.PlatformRunID, "platform-model", 4)
	h.checkpoint(t, pair.DirectRunID, "frontier-a", 2)

	p := h.practice(t)
	a, err := benchmark.NewAnswer(benchmark.ChoiceA, benchmark.SideB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.RecordVerdict(ctx, pair.PairID, a); err != nil {
		t.Fatal(err)
	}
	rec, err := p.Reveal(ctx, pair.PairID)
	if err != nil {
		t.Fatal(err)
	}

	// pair id · domain · task class · timestamps · both arms' exact observed
	// model identities · both arms' measured consumption · refs to both
	// rendered-blind artifacts · position · verdict · platform-guess · decline
	// flag · epoch id · requester id.
	checks := []struct {
		field string
		ok    bool
	}{
		{"pair id", rec.PairID == pair.PairID},
		{"domain", rec.Domain == benchmark.DomainSoftwareDevelopment},
		{"task class", rec.TaskClass == "software/standard"},
		{"timestamps", rec.SampledTS != "" && rec.RecordedTS != ""},
		{"platform model identity", rec.PlatformModel == "platform-model"},
		{"direct model identity", rec.DirectModel == "frontier-a"},
		{"platform consumption", rec.PlatformUSD > 0},
		{"direct consumption", rec.DirectUSD > 0},
		{"artifact A ref", rec.ArtifactARef == benchmark.ArtifactRef(pair.PairID, benchmark.SideA)},
		{"artifact B ref", rec.ArtifactBRef == benchmark.ArtifactRef(pair.PairID, benchmark.SideB)},
		{"position assignment", rec.Position != ""},
		{"verdict", rec.Verdict == string(benchmark.ChoiceA)},
		{"requester's platform-guess", rec.PlatformGuess == string(benchmark.SideB)},
		{"decline flag", !rec.Declined},
		{"epoch id", rec.EpochID != ""},
		{"requester id", rec.RequesterID == "alice"},
	}
	for _, c := range checks {
		if !c.ok {
			t.Errorf("the §14 record minimum %q is missing or wrong: %+v", c.field, rec.RecordPayload)
		}
	}
	// The guess was WRONG (platform was on A, guessed B), and the record scores
	// it — that scoring is the §5 blindness measurement.
	if rec.GuessCorrect {
		t.Error("the guess named the wrong side and must be scored as incorrect")
	}
	// Keep-forever is marked on the payload (Spec S14.9; enforcement is B5-8's).
	if rec.Retention != "keep-forever" {
		t.Errorf("retention class = %q, want keep-forever", rec.Retention)
	}
	if !strings.Contains(rec.ProtocolRef, "benchmark-preregistration-v1.md") {
		t.Errorf("the record does not name the registration it was produced under: %q", rec.ProtocolRef)
	}
}

// TestDeclinedPairRecordsTheFlagAndNoArtifacts is acceptance item 5: a vetoed
// pair still writes its §14 record, with the decline flag and NO verdict, guess
// or artifacts — selective participation must be visible, not silent.
func TestDeclinedPairRecordsTheFlagAndNoArtifacts(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	pair := h.sampledPair(t, "t1")
	p := h.practice(t)

	got, err := p.Decline(ctx, pair.PairID)
	if err != nil {
		t.Fatalf("Decline: %v", err)
	}
	if got.State != benchmark.StateDeclined || !got.Declined {
		t.Errorf("declined pair state = %q declined=%v", got.State, got.Declined)
	}
	rec, err := p.Reveal(ctx, pair.PairID)
	if err != nil {
		t.Fatalf("a declined pair must still have a §14 record: %v", err)
	}
	if !rec.Declined {
		t.Error("the record does not carry the decline flag")
	}
	if rec.Verdict != "" || rec.PlatformGuess != "" {
		t.Errorf("a declined pair recorded a verdict/guess: %q/%q", rec.Verdict, rec.PlatformGuess)
	}
	if rec.ArtifactARef != "" || rec.ArtifactBRef != "" {
		t.Error("a declined pair records no artifact refs — the comparison never happened")
	}
	if rec.EpochID == "" || rec.RequesterID != "alice" {
		t.Errorf("a declined record still carries its epoch and requester: %+v", rec.RecordPayload)
	}
}

// TestParityNoteWhenTheDirectArmWasTruncated is the OQ9 boundary: ordinary
// metering and ceilings apply to the direct arm as platform economics, but a
// ceiling that CUT the arm short is a §6 parity fact recorded on the pair and
// flagged, never absorbed.
func TestParityNoteWhenTheDirectArmWasTruncated(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	pair := h.readyPair(t, "t1")
	p := h.practice(t)

	// The direct arm's run ends other than on its own terms.
	if _, err := h.runs.Transition(ctx, pair.DirectRunID, run.StateClaimed,
		run.TransitionOptions{Reason: "test", Actor: run.ActorPlatform}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.runs.Transition(ctx, pair.DirectRunID, run.StateCrashed,
		run.TransitionOptions{Reason: "ceiling", Actor: run.ActorPlatform}); err != nil {
		t.Fatal(err)
	}

	a, err := benchmark.NewAnswer(benchmark.ChoiceB, benchmark.SideA)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.RecordVerdict(ctx, pair.PairID, a)
	if err != nil {
		t.Fatal(err)
	}
	if got.ParityNote == "" {
		t.Fatal("a direct arm that did not finish on its own terms must record a §6 parity note")
	}
	if !strings.Contains(got.ParityNote, "coordinator") {
		t.Errorf("the parity note must flag coordinator attention: %q", got.ParityNote)
	}
	// It rides the published rate so it is visible wherever the number is.
	rate, err := p.PublishedRate(ctx, benchmark.DomainSoftwareDevelopment, got.EpochID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rate.ParityNotes) != 1 {
		t.Errorf("%d parity notes on the published rate, want 1 — a gap is flagged, never absorbed", len(rate.ParityNotes))
	}
}

// TestCleanDirectArmRecordsNoParityNote: the ordinary case is silent, so the
// note means something when it appears.
func TestCleanDirectArmRecordsNoParityNote(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	pair := h.readyPair(t, "t1")
	for _, s := range []run.State{run.StateClaimed, run.StateRunning, run.StateCompleted} {
		if _, err := h.runs.Transition(ctx, pair.DirectRunID, s,
			run.TransitionOptions{Reason: "test", Actor: run.ActorPlatform}); err != nil {
			t.Fatal(err)
		}
	}
	p := h.practice(t)
	a, err := benchmark.NewAnswer(benchmark.ChoiceA, benchmark.SideA)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.RecordVerdict(ctx, pair.PairID, a)
	if err != nil {
		t.Fatal(err)
	}
	if got.ParityNote != "" {
		t.Errorf("a cleanly finished direct arm recorded a parity note: %q", got.ParityNote)
	}
}
