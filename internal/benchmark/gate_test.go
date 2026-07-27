package benchmark_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/benchmark"
)

// greenFloors is a limb-(d) seam reporting a domain's suites green at their
// registered floors — the shell composes the real one over internal/evals.
func greenFloors(_ context.Context, domain string) (benchmark.FloorReport, error) {
	return benchmark.FloorReport{Evaluable: true, Green: true,
		Reason: "the " + domain + " rubric + golden sets are green at their registered floors"}, nil
}

// TestGateIsTheFourRegisteredLimbsOverTheCurrentEpoch is acceptance item 17.
func TestGateIsTheFourRegisteredLimbsOverTheCurrentEpoch(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)

	p := h.practice(t)
	p.Floors = greenFloors

	// Nothing accrued: limbs (a) and (b) fail, the gate is shut.
	ev, err := p.Gate(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	if len(ev.Limbs) != 4 {
		t.Fatalf("%d limbs evaluated, want the four registered ones", len(ev.Limbs))
	}
	for i, want := range []string{"a", "b", "c", "d"} {
		if ev.Limbs[i].Limb != want {
			t.Errorf("limb %d is %q, want %q", i, ev.Limbs[i].Limb, want)
		}
		if !strings.HasPrefix(ev.Limbs[i].Ref, "BENCH-REG §11") {
			t.Errorf("limb %s cites %q, not §11", want, ev.Limbs[i].Ref)
		}
	}
	if ev.Open {
		t.Error("an empty record must not open the gate")
	}

	// The registered minimal clearing record: 13 wins of 20 non-tied pairs.
	h.recordWins(t, "w", 20, 13, "frontier-a")
	ev, err = p.Gate(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Rate.N != 20 || ev.Rate.W != 13 {
		t.Fatalf("gate saw w=%d m=%d, want 13/20", ev.Rate.W, ev.Rate.N)
	}
	if !ev.Open {
		t.Errorf("13/20 with green suites and no alarm must open the gate; limbs: %+v", ev.Limbs)
	}
	if ev.Marker != benchmark.RegisteredMarker {
		t.Error("the gate evaluation must carry the read-only marker")
	}

	// The durable first-open marker landed, and the sampling phase follows it.
	st, err := h.store.Domain(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	if st.FirstGateOpened.IsZero() {
		t.Error("the first-open marker was not recorded (OQ3) — the phase schedule keys on it")
	}
	if st.Phase() != benchmark.PhaseMaintenance {
		t.Errorf("phase after the first open = %q, want maintenance", st.Phase())
	}
}

// TestGateShutsOnTwelveOfTwenty is the registered boundary, read through the
// gate rather than through the statistic: one win fewer and it does not open.
func TestGateShutsOnTwelveOfTwenty(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	p := h.practice(t)
	p.Floors = greenFloors

	h.recordWins(t, "w", 20, 12, "frontier-a")
	ev, err := p.Gate(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Open {
		t.Errorf("12/20 (G=%.4f) opened the gate — the registration says it does not clear", ev.Rate.G)
	}
	if ev.Limbs[1].Pass {
		t.Error("limb (b) must fail at 12/20")
	}
	if !ev.Limbs[0].Pass {
		t.Error("limb (a) passes at m=20 — only limb (b) should fail here")
	}
	st, err := h.store.Domain(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	if !st.FirstGateOpened.IsZero() {
		t.Error("a shut gate must not record a first-open marker")
	}
}

// TestLimbDIsHonestlyNotEvaluable is acceptance item 17's honesty clause:
// an absent seam, a seam error, an unfloored suite or a red one all yield "not
// evaluable — the gate cannot open", never a fake green.
func TestLimbDIsHonestlyNotEvaluable(t *testing.T) {
	cases := []struct {
		name       string
		floors     benchmark.SuiteFloors
		wantEvalbl bool
		wantPass   bool
	}{
		{"no seam composed", nil, false, false},
		{"the seam errors", func(context.Context, string) (benchmark.FloorReport, error) {
			return benchmark.FloorReport{}, errors.New("registry unreachable")
		}, false, false},
		{"an unfloored version", func(context.Context, string) (benchmark.FloorReport, error) {
			return benchmark.FloorReport{Evaluable: false,
				Reason: "rubric-software v2 has no registered floor — falsifiability is not evaluable"}, nil
		}, false, false},
		{"a red suite", func(context.Context, string) (benchmark.FloorReport, error) {
			return benchmark.FloorReport{Evaluable: true, Green: false,
				Reason: "planted-defect catch 0.70 BELOW registered floor 0.84"}, nil
		}, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			ctx := context.Background()
			h.user(t, "alice", true)
			p := h.practice(t)
			p.Floors = c.floors
			h.recordWins(t, "w", 20, 13, "frontier-a")

			ev, err := p.Gate(ctx, benchmark.DomainSoftwareDevelopment)
			if err != nil {
				t.Fatal(err)
			}
			d := ev.Limbs[3]
			if d.Evaluable != c.wantEvalbl || d.Pass != c.wantPass {
				t.Errorf("limb (d) evaluable=%v pass=%v, want %v/%v", d.Evaluable, d.Pass, c.wantEvalbl, c.wantPass)
			}
			if d.Reason == "" {
				t.Error("limb (d) must name its reason")
			}
			if !c.wantEvalbl && !strings.Contains(d.Reason, "not evaluable") {
				t.Errorf("an unevaluable limb (d) must say so: %q", d.Reason)
			}
			// The statistics cleared; the gate is still shut. That is the point.
			if ev.Open {
				t.Error("the gate opened despite limb (d) — BENCH-REG §10.1(d): it cannot open while a suite is unfloored or red")
			}
		})
	}
}

// TestGateEvaluationSetsNothingOutsideThePackage is S14.7's SET-nothing clause,
// asserted as a property: a full gate evaluation writes only the benchmark
// bookkeeping row and leaves every other table exactly as it was.
func TestGateEvaluationSetsNothingOutsideThePackage(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	p := h.practice(t)
	p.Floors = greenFloors
	h.recordWins(t, "w", 20, 13, "frontier-a")

	watched := []string{"runs", "queue", "asks", "effects", "settings", "settings_events",
		"conformance_registry", "watch_rows", "eval_floors", "deliverables", "run_events",
		"benchmark_pairs", "checkpoints", "users"}
	before := map[string]int{}
	for _, table := range watched {
		before[table] = h.count(t, table)
	}

	if _, err := p.Gate(ctx, benchmark.DomainSoftwareDevelopment); err != nil {
		t.Fatal(err)
	}
	if _, err := p.PlatformGate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Readout(ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range watched {
		if after := h.count(t, table); after != before[table] {
			t.Errorf("evaluating the gate changed %s (%d → %d) — S14.7: this section evaluates and displays, it SETS NOTHING",
				table, before[table], after)
		}
	}
}

// TestAlarmRaisesAtTheRegisteredThreshold is acceptance item 16: 1−G > 0.95 in
// the current epoch raises ONE flag-now card carrying the full record, plus a
// standing, queryable expansion freeze — and kills nothing.
func TestAlarmRaisesAtTheRegisteredThreshold(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	p := h.practice(t)

	// Three losses: 1−G = 0.938, below the registered threshold.
	h.recordWins(t, "l", 3, 0, "frontier-a")
	st, err := p.AlarmState(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	if st.Standing {
		t.Fatalf("an alarm raised at 3 losses (1−G below %.2f)", benchmark.AlarmThreshold)
	}

	// The fourth loss is 0/4: 1−G = 0.969, the registration's earliest
	// alarm-zone record.
	h.recordPair(t, pairSpec{task: "l3", choice: benchmark.ChoiceB, guess: benchmark.SideA,
		directModel: "frontier-a", units: 1})
	st, err = p.AlarmState(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Standing {
		t.Fatal("0/4 must raise the alarm — it is the registration's earliest alarm-zone record")
	}
	if st.Severity != benchmark.AlarmSeverity {
		t.Errorf("alarm severity = %q, want the flag-now class §12 names", st.Severity)
	}
	if round3(st.LossG) != 0.969 || st.Threshold != benchmark.AlarmThreshold {
		t.Errorf("the alarm records 1−G=%.4f against threshold %.2f, want 0.969 / %.2f",
			st.LossG, st.Threshold, benchmark.AlarmThreshold)
	}
	// It carries "the full record" §12 asks for.
	if st.Rate.N != 4 || st.Rate.W != 0 || len(st.Rate.HonestClaims) == 0 {
		t.Errorf("the alarm card does not carry the full record: %+v", st.Rate)
	}

	// A further losing pair does NOT raise a second card: the alarm STANDS.
	h.recordPair(t, pairSpec{task: "l4", choice: benchmark.ChoiceB, guess: benchmark.SideA,
		directModel: "frontier-a", units: 1})
	if n := h.countAlarms(t, benchmark.AlarmRaise); n != 1 {
		t.Errorf("%d alarm raises, want exactly 1 — the alarm stands while the condition holds", n)
	}

	// The expansion freeze is standing and queryable, and says what it is.
	freeze, err := p.ExpansionFreeze(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !freeze.Standing || len(freeze.Domains) != 1 {
		t.Fatalf("the §12 expansion freeze is not standing: %+v", freeze)
	}
	if !strings.Contains(freeze.Summary, "Nothing is auto-killed") {
		t.Errorf("the freeze must state that it kills nothing: %q", freeze.Summary)
	}
	// Limb (c) closes the gate while it stands.
	p.Floors = greenFloors
	ev, err := p.Gate(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Limbs[2].Pass {
		t.Error("limb (c) must fail while an alarm stands")
	}

	// NOTHING was killed or parked: every run is where the suites left it.
	var parked int
	if err := h.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM runs WHERE state IN ('parked','tombstoned','died-at-gate')`).Scan(&parked); err != nil {
		t.Fatal(err)
	}
	if parked != 0 {
		t.Errorf("%d runs were parked or killed by the alarm — nothing is auto-killed and running work is untouched (D1.3)", parked)
	}
}

// TestAlarmRequiresAnOperatorDispositionToClear is acceptance item 16's second
// half: clearing is an operator act logged as a decision event.
func TestAlarmRequiresAnOperatorDispositionToClear(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	h.operator(t, "op")
	h.user(t, "mallory", false)
	p := h.practice(t)
	h.recordWins(t, "l", 4, 0, "frontier-a")

	if st, err := p.AlarmState(ctx, benchmark.DomainSoftwareDevelopment); err != nil || !st.Standing {
		t.Fatalf("precondition: the alarm must be standing (%+v, %v)", st, err)
	}
	// The condition ceasing to hold does not clear it: the disposition is
	// required, which is the conservative reading of §12.
	h.recordWins(t, "w", 10, 10, "frontier-a")
	st, err := p.AlarmState(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Standing {
		t.Error("the alarm cleared itself when the record recovered — §12 requires an operator disposition")
	}

	// Only a registered disposition clears it.
	if err := p.DisposeAlarm(ctx, "op", benchmark.DomainSoftwareDevelopment, "ignore", "nah"); !errors.Is(err, benchmark.ErrBadInput) {
		t.Errorf("an unregistered disposition = %v, want a refusal (the §12 set is closed)", err)
	}
	if err := p.DisposeAlarm(ctx, "", benchmark.DomainSoftwareDevelopment, benchmark.DispositionInvestigate, ""); !errors.Is(err, benchmark.ErrBadInput) {
		t.Error("a disposition without its acting principal must be refused")
	}
	// And only the OPERATOR clears it. §12 calls the disposition an operator
	// act, and the alarm exists precisely because the platform may be losing:
	// a member — even the pairs' own requester — must not be able to silence it.
	for _, actor := range []string{"alice", "mallory", "nobody"} {
		if err := p.DisposeAlarm(ctx, actor, benchmark.DomainSoftwareDevelopment,
			benchmark.DispositionInvestigate, "please stop"); !errors.Is(err, benchmark.ErrBadInput) {
			t.Errorf("DisposeAlarm as %q = %v, want a refusal — §12 requires an OPERATOR disposition", actor, err)
		}
	}
	if st, err := p.AlarmState(ctx, benchmark.DomainSoftwareDevelopment); err != nil || !st.Standing {
		t.Fatal("a refused disposition must leave the alarm standing")
	}
	if err := p.DisposeAlarm(ctx, "op", benchmark.DomainSoftwareDevelopment,
		benchmark.DispositionFixAndAccrue, "rubric bug found and fixed"); err != nil {
		t.Fatalf("DisposeAlarm: %v", err)
	}
	st, err = p.AlarmState(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	if st.Standing {
		t.Error("a dispositioned alarm must clear")
	}

	// The disposition is logged as a DECISION event carrying the S14.2
	// human-decision minimums, and alarm history keeps both events forever.
	var decisions int
	if err := h.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM run_events WHERE type = ? AND json_extract(payload,'$.card_type') = 'benchmark.alarm'`,
		benchmark.EventDecision).Scan(&decisions); err != nil {
		t.Fatal(err)
	}
	if decisions != 1 {
		t.Errorf("%d decision events for the disposition, want 1", decisions)
	}
	if raises, clears := h.countAlarms(t, benchmark.AlarmRaise), h.countAlarms(t, benchmark.AlarmClear); raises != 1 || clears != 1 {
		t.Errorf("alarm history = %d raise / %d clear, want 1/1 kept forever", raises, clears)
	}
	// No alarm standing ⇒ no freeze.
	freeze, err := p.ExpansionFreeze(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if freeze.Standing {
		t.Error("the expansion freeze must lift with the alarm")
	}
	// And there is nothing to dispose twice.
	if err := p.DisposeAlarm(ctx, "op", benchmark.DomainSoftwareDevelopment,
		benchmark.DispositionInvestigate, ""); !errors.Is(err, benchmark.ErrNoAlarm) {
		t.Errorf("disposing a cleared alarm = %v, want ErrNoAlarm", err)
	}
}

// TestPlatformGateSpansTheV0Roster: 15.3 opens when every registered launch
// domain passes, and the 14.5 hold is stated while it does not.
func TestPlatformGateSpansTheV0Roster(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	p := h.practice(t)
	p.Floors = greenFloors

	pg, err := p.PlatformGate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pg.Open {
		t.Error("the 15.3 gate must not open with no accrual")
	}
	if !strings.Contains(pg.Held, "14.5 breadth restrictions stand") {
		t.Errorf("a shut 15.3 gate must state the 14.5 hold: %q", pg.Held)
	}
	if len(pg.Domains) != len(benchmark.V0LaunchDomains()) {
		t.Errorf("the 15.3 gate evaluated %d domains, want the v0 roster's %d",
			len(pg.Domains), len(benchmark.V0LaunchDomains()))
	}

	h.recordWins(t, "w", 20, 13, "frontier-a")
	pg, err = p.PlatformGate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !pg.Open {
		t.Errorf("the v0 roster is one domain; with it passing the 15.3 gate opens. limbs: %+v", pg.Domains[0].Limbs)
	}
	if pg.Held != "" {
		t.Error("an open gate holds nothing")
	}
}

// TestLowAccrualIsANoteNotMachinery is acceptance item's §11-last-paragraph
// clause: no low-n alternate gate exists in code; the recorded path is a dated
// re-registration.
func TestLowAccrualIsANoteNotMachinery(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	p := h.practice(t)
	p.Floors = greenFloors
	h.recordWins(t, "w", 4, 4, "frontier-a")

	dr, err := p.DomainReadout(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	if dr.LowAccrualNote == "" {
		t.Fatal("a low-accrual domain must carry the §11 note")
	}
	if !strings.Contains(dr.LowAccrualNote, "re-registration") {
		t.Errorf("the note must name the honest path: %q", dr.LowAccrualNote)
	}
	if dr.Gate.Open {
		t.Error("4/4 must not open the gate: limb (a) needs m ≥ 20 and no alternate gate exists")
	}
	// And there is genuinely no alternate machinery.
	code := strings.ToLower(readPackageCode(t))
	for _, forbidden := range []string{"lownlimb", "alternategate", "catastrophiconly", "relaxedgate"} {
		if strings.Contains(code, forbidden) {
			t.Errorf("the package's code names %q — pre-committing a weaker gate was considered and REJECTED (BENCH-REG §11)", forbidden)
		}
	}
}

func (h *harness) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	if err := h.db.QueryRowContext(context.Background(),
		fmt.Sprintf(`SELECT COUNT(*) FROM %s`, table)).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func (h *harness) countAlarms(t *testing.T, action string) int {
	t.Helper()
	var n int
	if err := h.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM run_events WHERE type = ? AND json_extract(payload,'$.action') = ?`,
		benchmark.EventAlarm, action).Scan(&n); err != nil {
		t.Fatalf("count alarms: %v", err)
	}
	return n
}
