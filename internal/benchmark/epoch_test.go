package benchmark_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/benchmark"
)

// epoch_test.go pins BENCH-REG §9 — the rule that decides which records may be
// added together at all.

// TestDirectArmIdentityChangeEndsTheEpoch is acceptance item 13's first half.
func TestDirectArmIdentityChangeEndsTheEpoch(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)

	first := h.recordPair(t, pairSpec{task: "t1", choice: benchmark.ChoiceA,
		guess: benchmark.SideA, directModel: "frontier-a", units: 1})
	second := h.recordPair(t, pairSpec{task: "t2", choice: benchmark.ChoiceA,
		guess: benchmark.SideA, directModel: "frontier-a", units: 1})
	if first.EpochID != second.EpochID {
		t.Fatalf("two pairs on the same observed identity landed in different epochs: %q vs %q",
			first.EpochID, second.EpochID)
	}

	// The consumer surface auto-upgrades: a new observed identity ends the epoch.
	third := h.recordPair(t, pairSpec{task: "t3", choice: benchmark.ChoiceA,
		guess: benchmark.SideA, directModel: "frontier-b", units: 1})
	if third.EpochID == first.EpochID {
		t.Fatalf("a changed direct-arm identity did not end the epoch (both %q)", third.EpochID)
	}
	st, err := h.store.Domain(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	if st.CurrentEpochID != third.EpochID || st.EpochIdentity != "frontier-b" {
		t.Errorf("the domain's current epoch is %q/%q, want the new one %q/frontier-b",
			st.CurrentEpochID, st.EpochIdentity, third.EpochID)
	}
	if st.EpochSeq != 2 {
		t.Errorf("epoch sequence = %d, want 2 after one split", st.EpochSeq)
	}
}

// TestPlatformArmChangeNeverSplitsAnEpoch is acceptance item 13's second half:
// "Platform-arm model changes are recorded per pair but do not split epochs
// (the platform evolving is the treatment)."
func TestPlatformArmChangeNeverSplitsAnEpoch(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)

	first := h.recordPair(t, pairSpec{task: "t1", choice: benchmark.ChoiceA, guess: benchmark.SideA,
		directModel: "frontier-a", platformModel: "platform-model", units: 1})
	second := h.recordPair(t, pairSpec{task: "t2", choice: benchmark.ChoiceA, guess: benchmark.SideA,
		directModel: "frontier-a", platformModel: "frontier-b", units: 1})

	if first.EpochID != second.EpochID {
		t.Errorf("a PLATFORM-arm model change split the epoch (%q → %q) — only the direct arm's identity may (BENCH-REG §9)",
			first.EpochID, second.EpochID)
	}
	// It is still RECORDED per pair.
	if second.PlatformModel != "frontier-b" {
		t.Errorf("the platform arm's changed identity was not recorded: %q", second.PlatformModel)
	}
	st, err := h.store.Domain(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	if st.EpochSeq != 1 {
		t.Errorf("epoch sequence = %d, want 1 — no split should have happened", st.EpochSeq)
	}
}

// TestNoDecisionStatisticPoolsAcrossEpochs is acceptance item 13's third half:
// G, the gate and the alarm evaluate over the CURRENT epoch only, while the §16
// history view shows the epochs side by side.
func TestNoDecisionStatisticPoolsAcrossEpochs(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)

	// Epoch 1: three platform wins on frontier-a.
	for i, task := range []string{"a1", "a2", "a3"} {
		_ = i
		h.recordPair(t, pairSpec{task: task, choice: benchmark.ChoiceA, guess: benchmark.SideA,
			directModel: "frontier-a", units: 1})
	}
	// Epoch 2: one platform loss on frontier-b.
	last := h.recordPair(t, pairSpec{task: "b1", choice: benchmark.ChoiceB, guess: benchmark.SideA,
		directModel: "frontier-b", units: 1})

	current, err := h.practice(t).PublishedRate(ctx, benchmark.DomainSoftwareDevelopment, last.EpochID)
	if err != nil {
		t.Fatal(err)
	}
	if current.N != 1 || current.W != 0 {
		t.Errorf("the current epoch's record is w=%d m=%d, want 0/1 — the three earlier wins belong to a closed epoch",
			current.W, current.N)
	}
	// The gate reads the same scope, so a closed epoch's evidence can never
	// carry a domain over the line.
	ev, err := h.practice(t).Gate(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	if ev.EpochID != last.EpochID || ev.Rate.N != 1 {
		t.Errorf("the gate evaluated epoch %q at m=%d, want the current epoch at m=1", ev.EpochID, ev.Rate.N)
	}

	// The history view DOES show both, side by side — §16 permits exactly that.
	epochs, err := h.practice(t).EpochHistory(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	if len(epochs) != 2 {
		t.Fatalf("%d epochs in the history view, want 2", len(epochs))
	}
	if !epochs[1].Current || epochs[0].Current {
		t.Errorf("the history view mislabels which epoch is current: %+v", epochs)
	}
	if epochs[0].Identity != "frontier-a" || epochs[1].Identity != "frontier-b" {
		t.Errorf("epoch identities = %q, %q; want frontier-a then frontier-b", epochs[0].Identity, epochs[1].Identity)
	}
}

// TestDeclinedPairNeverSplitsAnEpoch: an unobserved identity is not a CHANGED
// identity. Splitting on absence would shred the record into one-pair epochs
// every time a reading was missing.
func TestDeclinedPairNeverSplitsAnEpoch(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)

	first := h.recordPair(t, pairSpec{task: "t1", choice: benchmark.ChoiceA, guess: benchmark.SideA,
		directModel: "frontier-a", units: 1})
	declined := h.recordPair(t, pairSpec{task: "t2", decline: true})
	if declined.EpochID != first.EpochID {
		t.Errorf("a declined pair opened a new epoch (%q vs %q) — it observed no identity to differ from",
			declined.EpochID, first.EpochID)
	}
	st, err := h.store.Domain(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	if st.EpochIdentity != "frontier-a" || st.EpochSeq != 1 {
		t.Errorf("the domain's epoch moved on a decline: %+v", st)
	}
}

// TestBenchTwoIsStructurallyUnpoolable is acceptance item 13's last clause:
// "Nexus bench-02 results are a closed historical epoch — recorded for context,
// never pooled into any Sinet statistic." There is nothing to exclude because
// nothing can enter: the package has no import path, no seed and no migration
// column for predecessor data, and the ONLY way an epoch comes into existence is
// from a direct-arm identity observed on a Sinet run.
func TestBenchTwoIsStructurallyUnpoolable(t *testing.T) {
	code := strings.ToLower(readPackageCode(t))
	for _, forbidden := range []string{"bench02", "nexus", "backfill", "seedpair", "historicalepoch"} {
		if strings.Contains(code, forbidden) {
			t.Errorf("the package's code names %q — no predecessor-data path may exist (BENCH-REG §9)", forbidden)
		}
	}
	// The reading is RECORDED, per the requirement that it be documented.
	if !strings.Contains(readPackageSource(t), "bench-02") {
		t.Error("the bench-02 clause must be recorded in a doc comment")
	}
	// And the sole epoch creator is the tracker: no other site writes
	// current_epoch_id.
	if n := strings.Count(readPackageSource(t), "current_epoch_id = ?"); n != 1 {
		t.Errorf("%d sites write current_epoch_id, want exactly 1 (the §9 tracker)", n)
	}
}
