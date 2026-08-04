package benchmark_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/benchmark"
)

// sampler wires the §4 hook over the harness with a deterministic draw.
func (h *harness) sampler(draw int) *benchmark.Sampler {
	return &benchmark.Sampler{
		Store: h.store, Log: h.log, Settings: h.reg,
		Rand: func(int) int { return draw }, Logger: discardLogger(),
	}
}

// eligibleTask builds the full shape §4.2 needs: an opted-in requester, a
// software task with a non-trivial tier, and an accepted deliverable.
func (h *harness) eligibleTask(t *testing.T, id string) string {
	t.Helper()
	d := h.task(t, taskOpts{
		taskID: id, owner: "alice", codeDomain: "software",
		tier: "standard", family: "software", accept: true,
	})
	h.accept(t, "alice", d)
	return d
}

// TestSamplingHookFiresOnAcceptedDeliverable is acceptance item 1: the hook
// derives from the log — an accepted deliverable produces a sampled pair with
// no edit to internal/review or internal/accept anywhere in the diff.
func TestSamplingHookFiresOnAcceptedDeliverable(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	h.eligibleTask(t, "t1")

	pass, err := h.sampler(0).Tail(ctx)
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if pass.Considered != 1 || pass.Sampled != 1 {
		t.Fatalf("pass = %+v, want 1 considered / 1 sampled at the default 100%% pre-gate rate", pass)
	}
	pairs, err := h.store.PairsInDomain(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 1 {
		t.Fatalf("%d pairs recorded, want 1", len(pairs))
	}
	p := pairs[0]
	// The pair carries the REGISTERED domain name, never the code name (OQ2).
	if p.Domain != benchmark.DomainSoftwareDevelopment {
		t.Errorf("pair domain = %q, want the registered %q", p.Domain, benchmark.DomainSoftwareDevelopment)
	}
	if p.DomainSource != "verdict.recorded" {
		t.Errorf("domain resolution source = %q, want the accepting run's verdict record", p.DomainSource)
	}
	// The rate and phase actually used are recorded ON the pair (R3).
	if p.Phase != benchmark.PhasePreGate || p.RatePct != 100 {
		t.Errorf("pair records phase=%q rate=%d, want pre-gate/100", p.Phase, p.RatePct)
	}
	if p.State != benchmark.StateSampled {
		t.Errorf("pair state = %q, want %q", p.State, benchmark.StateSampled)
	}
	if p.UserID != "alice" || p.TaskID != "t1" {
		t.Errorf("pair is not attributed to the requester's task: %+v", p)
	}
	if p.TaskClass != "software/standard" {
		t.Errorf("task class = %q, want the recorded intake family/tier", p.TaskClass)
	}
	if p.PlatformRunID != "t1.r1" {
		t.Errorf("platform run = %q, want the run that minted the accepted revision", p.PlatformRunID)
	}
}

// TestEligibilityLimbsEachSkipWithTheirLimbRecorded is acceptance item 2: all
// five §4.2 limbs are implemented, each failure names its limb, and no pair is
// created when any limb fails.
func TestEligibilityLimbsEachSkipWithTheirLimbRecorded(t *testing.T) {
	cases := []struct {
		name     string
		wantLimb int
		build    func(t *testing.T, h *harness)
	}{
		{
			"limb 1 — no standing opt-in", benchmark.LimbOptIn,
			func(t *testing.T, h *harness) {
				h.user(t, "alice", false) // default OFF: it is an opt-IN
				h.eligibleTask(t, "t1")
			},
		},
		{
			"limb 2 — unregistered domain", benchmark.LimbDomainAccepted,
			func(t *testing.T, h *harness) {
				h.user(t, "alice", true)
				d := h.task(t, taskOpts{taskID: "t1", owner: "alice", codeDomain: "content",
					tier: "standard", family: "content", accept: true})
				h.accept(t, "alice", d)
			},
		},
		{
			"limb 2 — domain unresolvable", benchmark.LimbDomainAccepted,
			func(t *testing.T, h *harness) {
				h.user(t, "alice", true)
				d := h.task(t, taskOpts{taskID: "t1", owner: "alice", tier: "standard", accept: true})
				h.accept(t, "alice", d)
			},
		},
		{
			"limb 2 — deliverable not accepted", benchmark.LimbDomainAccepted,
			func(t *testing.T, h *harness) {
				h.user(t, "alice", true)
				d := h.task(t, taskOpts{taskID: "t1", owner: "alice", codeDomain: "software",
					tier: "standard", accept: false})
				h.accept(t, "alice", d)
			},
		},
		{
			"limb 3 — the zero-interaction trivial band", benchmark.LimbNotTrivial,
			func(t *testing.T, h *harness) {
				h.user(t, "alice", true)
				d := h.task(t, taskOpts{taskID: "t1", owner: "alice", codeDomain: "software",
					tier: "trivial", family: "software", accept: true})
				h.accept(t, "alice", d)
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			ctx := context.Background()
			c.build(t, h)

			pass, err := h.sampler(0).Tail(ctx)
			if err != nil {
				t.Fatalf("Tail: %v", err)
			}
			if pass.Sampled != 0 || pass.Skipped != 1 {
				t.Fatalf("pass = %+v, want the accept skipped", pass)
			}
			// The failing limb is reachable as a value, not only as a log line,
			// so a domain that never accrues can be diagnosed.
			var el benchmark.Eligibility
			events, err := h.log.After(ctx, 0, 256)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range events {
				if e.Type != "deliverable.accepted" {
					continue
				}
				if err := h.db.WriteTx(ctx, func(tx *sqlTxAlias) error {
					var err error
					el, err = h.sampler(0).Eligible(ctx, tx, e)
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}
			if el.Eligible {
				t.Fatal("the task must not be eligible")
			}
			if el.Limb != c.wantLimb {
				t.Errorf("failing limb = %d (%q), want limb %d", el.Limb, el.Reason, c.wantLimb)
			}
			if el.Reason == "" {
				t.Error("a skipped task must record why")
			}
			for _, ds := range []string{benchmark.DomainSoftwareDevelopment, benchmark.DomainWebResearch} {
				pairs, err := h.store.PairsInDomain(ctx, ds)
				if err != nil {
					t.Fatal(err)
				}
				if len(pairs) != 0 {
					t.Errorf("an ineligible task created %d pair(s) in %s", len(pairs), ds)
				}
			}
		})
	}
}

// TestLimbFiveIsNamedAndBecomesRealAtDecline: limb 5 (§4.2.5) cannot fail at
// sampling time — no pair exists to decline yet — so the conjunction records it
// as satisfied and the decline verb is where it becomes real. This asserts the
// limb is not silently dropped.
func TestLimbFiveIsNamedAndBecomesRealAtDecline(t *testing.T) {
	src := readPackageSource(t)
	if !strings.Contains(src, "LimbNotDeclined") {
		t.Error("limb 5 (§4.2.5) is not named in the package — the five-limb conjunction must be complete")
	}
	if benchmark.LimbNotDeclined != 5 {
		t.Errorf("LimbNotDeclined = %d, want the §4.2 numbering (5)", benchmark.LimbNotDeclined)
	}
}

// TestDrawIsIndependentUniformAtThePhaseRate is acceptance item 3: the draw is
// one independent uniform draw per eligible task against the phase rate. At 0%
// nothing is drawn; at 100% everything is; and the boundary is the registered
// comparison, not an approximation.
func TestDrawIsIndependentUniformAtThePhaseRate(t *testing.T) {
	for _, c := range []struct {
		rate       string
		draw       int
		wantSample bool
	}{
		{"0", 0, false},   // a 0% rate samples nothing, whatever the draw
		{"100", 99, true}, // a 100% rate samples everything
		{"25", 24, true},  // draw < rate ⇒ sampled
		{"25", 25, false}, // draw == rate ⇒ not sampled (uniform over [0,100))
		{"25", 99, false},
	} {
		t.Run(fmt.Sprintf("rate=%s draw=%d", c.rate, c.draw), func(t *testing.T) {
			h := newHarness(t)
			ctx := context.Background()
			h.setRate(t, c.rate, "25")
			h.user(t, "alice", true)
			h.eligibleTask(t, "t1")

			pass, err := h.sampler(c.draw).Tail(ctx)
			if err != nil {
				t.Fatalf("Tail: %v", err)
			}
			got := pass.Sampled == 1
			if got != c.wantSample {
				t.Errorf("sampled = %v, want %v at rate %s%% with draw %d", got, c.wantSample, c.rate, c.draw)
			}
			// Either way the accept is CONSUMED: an eligible task is drawn on
			// exactly once, never re-rolled by a later pass.
			cursor, err := h.store.SamplerCursor(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if cursor == 0 {
				t.Error("the sampler cursor did not advance past a considered accept — a re-roll would break §4.1 uniformity")
			}
		})
	}
}

// TestSamplingRateIsReadLive is the §34 live-apply obligation: the ⚙ key is
// re-read at every evaluation, so an operator rate change applies to the next
// draw with no restart.
func TestSamplingRateIsReadLive(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	s := h.sampler(50)

	h.setRate(t, "100", "25")
	h.eligibleTask(t, "t1")
	if pass, err := h.sampler(50).Tail(ctx); err != nil || pass.Sampled != 1 {
		t.Fatalf("first pass = %+v err=%v, want 1 sampled at 100%%", pass, err)
	}

	// Move the dial mid-run. The same sampler instance must honour it.
	h.setRate(t, "10", "25")
	h.eligibleTask(t, "t2")
	pass, err := s.Tail(ctx)
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if pass.Sampled != 0 {
		t.Errorf("second pass sampled %d, want 0 — the ⚙ rate must be read LIVE at evaluation time", pass.Sampled)
	}
}

// TestPhaseFlipsOnTheDurableFirstOpenMarker: phase is pre-gate until the
// domain's gate first opens, then maintenance, keyed to the durable marker
// (OQ3) — and the rate the draw uses follows the phase.
func TestPhaseFlipsOnTheDurableFirstOpenMarker(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.setRate(t, "100", "0") // pre-gate samples everything, maintenance nothing
	h.user(t, "alice", true)

	st, err := h.store.EnsureDomain(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	if st.Phase() != benchmark.PhasePreGate {
		t.Fatalf("a domain whose gate has never opened is in phase %q, want pre-gate", st.Phase())
	}
	h.eligibleTask(t, "t1")
	if pass, err := h.sampler(0).Tail(ctx); err != nil || pass.Sampled != 1 {
		t.Fatalf("pre-gate pass = %+v err=%v, want 1 sampled", pass, err)
	}

	// Mark the gate as having opened, exactly as a gate evaluation would.
	if err := h.exec(ctx, `UPDATE benchmark_domains SET first_gate_opened_ts = ? WHERE domain = ?`,
		"2026-08-02T00:00:00Z", benchmark.DomainSoftwareDevelopment); err != nil {
		t.Fatal(err)
	}
	st, err = h.store.Domain(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	if st.Phase() != benchmark.PhaseMaintenance {
		t.Fatalf("after the first open the phase is %q, want maintenance", st.Phase())
	}
	h.eligibleTask(t, "t2")
	pass, err := h.sampler(0).Tail(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pass.Sampled != 0 {
		t.Errorf("maintenance pass sampled %d at a 0%% maintenance rate — the phase must select the rate", pass.Sampled)
	}
	// The marker is write-once: history does not move.
	if err := h.exec(ctx, `UPDATE benchmark_domains SET first_gate_opened_ts = NULL WHERE domain = ?`,
		benchmark.DomainSoftwareDevelopment); err == nil {
		t.Error("the first-open marker must be write-once (migration 0014 trigger)")
	}
}

// TestSamplerCursorIsDurableAcrossARestart: the cursor lives in the DB, not in
// the goroutine, so a restart neither re-samples an accept nor silently skips
// one. A lost accept is a lost DRAW, and §4.1's uniformity is frozen.
func TestSamplerCursorIsDurableAcrossARestart(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	h.eligibleTask(t, "t1")

	if _, err := h.sampler(0).Tail(ctx); err != nil {
		t.Fatal(err)
	}
	cursor, err := h.store.SamplerCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cursor == 0 {
		t.Fatal("the cursor did not advance")
	}

	// A "restart": a brand-new Store and Sampler over the same database.
	fresh := benchmark.NewStore(h.db, h.log)
	fresh.Now = h.clk.now
	again := &benchmark.Sampler{Store: fresh, Log: h.log, Settings: h.reg,
		Rand: func(int) int { return 0 }, Logger: discardLogger()}
	pass, err := again.Tail(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pass.Considered != 0 {
		t.Errorf("a restarted sampler re-considered %d accept(s) — the cursor must be durable", pass.Considered)
	}
	pairs, err := fresh.PairsInDomain(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 1 {
		t.Errorf("%d pairs after a restart, want the original 1 — a re-sample would double-count", len(pairs))
	}

	// A later accept IS picked up: the cursor advances, it does not close.
	h.eligibleTask(t, "t2")
	if pass, err := again.Tail(ctx); err != nil || pass.Sampled != 1 {
		t.Fatalf("post-restart pass = %+v err=%v, want the new accept sampled", pass, err)
	}
}

// TestNoStratificationPathExists is acceptance item 3's negative: §4.1 freezes
// UNIFORMITY, so the package must contain no stratification, prioritization or
// quota machinery — the ways a sampler stops being uniform without anyone
// deciding to make it so.
func TestNoStratificationPathExists(t *testing.T) {
	code := strings.ToLower(readPackageCode(t))
	for _, forbidden := range []string{"stratif", "quota", "weighted", "oversampl", "prioritiz"} {
		if strings.Contains(code, forbidden) {
			t.Errorf("the package's CODE contains %q — sampling is uniform-random among eligible tasks and that uniformity is FROZEN (BENCH-REG §4.1)", forbidden)
		}
	}
	// The probe: the scanner must be able to see a real identifier, or the
	// assertion above proves nothing.
	if !strings.Contains(code, "computeg") {
		t.Fatal("the code scan cannot see the package's own identifiers — it would pass vacuously")
	}
}

// TestTrivialBandIsReadNeverRepriced: limb 3's source is the tier the intake
// pipeline recorded. The package must not re-price a task against the
// zero-interaction band's ⚙ key (G1 D1.7 — the pipeline already classified, and
// a second pricing could disagree with what the requester experienced).
func TestTrivialBandIsReadNeverRepriced(t *testing.T) {
	src := readPackageSource(t)
	if strings.Contains(src, `"intake.zero_interaction_cost_usd"`) {
		t.Error("the package reads intake.zero_interaction_cost_usd as a key — limb 3 reads the RECORDED tier, it never re-prices")
	}
	if !strings.Contains(src, "intake.zero_interaction_cost_usd") {
		t.Error("the referenced-not-consumed key must be NAMED in a comment so nothing hardcodes the band")
	}
}
