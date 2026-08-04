package benchmark_test

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/benchmark"
)

// readout_test.go pins BENCH-REG §15/§16 and S14.7's flat rule: "There is no
// benchmark number without its epistemics."

// TestPublishedRateCarriesTheFullEpistemicsBlock is acceptance item 18: §15's
// "Every published rate carries n, the posterior G, tie/both-bad rates, decline
// rate, and guess accuracy", plus the honest-claims table itself.
func TestPublishedRateCarriesTheFullEpistemicsBlock(t *testing.T) {
	rt := reflect.TypeOf(benchmark.PublishedRate{})
	for _, field := range []string{
		"WinRate", "N", "W", "G", "LossG",
		"TieRate", "BothBadRate", "DeclineRate", "GuessAccuracy", "GuessN",
		"HonestClaims", "ArmParity",
	} {
		if _, ok := rt.FieldByName(field); !ok {
			t.Errorf("PublishedRate has no %s — every published rate carries its epistemics (BENCH-REG §15)", field)
		}
	}

	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	p := h.practice(t)

	// A record with every signal in it: wins, losses, a tie, a both-bad and a
	// decline, all in one epoch.
	h.recordPair(t, pairSpec{task: "w1", choice: benchmark.ChoiceA, guess: benchmark.SideA, directModel: "frontier-a", units: 1})
	h.recordPair(t, pairSpec{task: "w2", choice: benchmark.ChoiceA, guess: benchmark.SideB, directModel: "frontier-a", units: 1})
	h.recordPair(t, pairSpec{task: "l1", choice: benchmark.ChoiceB, guess: benchmark.SideB, directModel: "frontier-a", units: 1})
	h.recordPair(t, pairSpec{task: "x1", choice: benchmark.ChoiceTie, guess: benchmark.SideB, directModel: "frontier-a", units: 1})
	h.recordPair(t, pairSpec{task: "x2", choice: benchmark.ChoiceBothBad, guess: benchmark.SideB, directModel: "frontier-a", units: 1})
	h.recordPair(t, pairSpec{task: "d1", decline: true})

	st, err := h.store.Domain(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	rate, err := p.PublishedRate(ctx, benchmark.DomainSoftwareDevelopment, st.CurrentEpochID)
	if err != nil {
		t.Fatal(err)
	}

	// tie and both-bad are EXCLUDED from m and w (BENCH-REG §8, frozen) …
	if rate.N != 3 || rate.W != 2 {
		t.Errorf("w/m = %d/%d, want 2/3 — tie and both-bad never enter the posterior", rate.W, rate.N)
	}
	// … and REPORTED beside the win rate, at the same accrual.
	if rate.TieRate != 0.2 || rate.BothBadRate != 0.2 {
		t.Errorf("tie rate %.3f / both-bad rate %.3f, want 0.2 / 0.2 over the five verdicts",
			rate.TieRate, rate.BothBadRate)
	}
	// The decline rate ships beside the rate: selective participation must be
	// visible, not silent (§4.2.5).
	if want := 1.0 / 6.0; rate.DeclineRate != want {
		t.Errorf("decline rate = %.4f, want %.4f (1 decline of 6 answered)", rate.DeclineRate, want)
	}
	// Guess accuracy is measured, at the same n as the votes.
	if rate.GuessN != 5 {
		t.Errorf("guess n = %d, want the 5 votes", rate.GuessN)
	}
	if rate.G <= 0 || rate.G >= 1 {
		t.Errorf("G = %v is not a posterior probability", rate.G)
	}
	// The §15 table rides along, so wherever B6 renders the number it renders
	// with it.
	if len(rate.HonestClaims) != 5 || rate.HonestClaimsReadout == "" {
		t.Error("the §15 honest-claims table must ride on the published rate")
	}
	if len(rate.ArmParity) == 0 {
		t.Error("the §6 arm-parity declaration must ride on the published rate")
	}
}

// TestBlindnessLabelAppearsWhenGuessAccuracyBeatsChance is acceptance item 11:
// the rate STILL publishes, labelled — no result is ever suppressed for
// blindness failure.
func TestBlindnessLabelAppearsWhenGuessAccuracyBeatsChance(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	p := h.practice(t)

	// Four votes, all guessing correctly: the platform is on side A (the fixed
	// draw) and every guess names A.
	for _, task := range []string{"g1", "g2", "g3", "g4"} {
		h.recordPair(t, pairSpec{task: task, choice: benchmark.ChoiceA, guess: benchmark.SideA,
			directModel: "frontier-a", units: 1})
	}
	st, err := h.store.Domain(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	rate, err := p.PublishedRate(ctx, benchmark.DomainSoftwareDevelopment, st.CurrentEpochID)
	if err != nil {
		t.Fatal(err)
	}
	if rate.GuessAccuracy != 1 {
		t.Fatalf("guess accuracy = %v, want 1 (every guess named the platform's side)", rate.GuessAccuracy)
	}
	if rate.BlindnessLabel != benchmark.PartiallyUnblindedLabel(1) {
		t.Errorf("blindness label = %q, want %q", rate.BlindnessLabel, benchmark.PartiallyUnblindedLabel(1))
	}
	// The rate is NOT suppressed: n, G and the win rate are all still there.
	if rate.N != 4 || rate.WinRate != 1 || rate.G == 0 {
		t.Error("a partially-unblinded rate still publishes in full — it is labelled, never withheld")
	}

	// At chance, no label. The label means something because it is not always on.
	h2 := newHarness(t)
	h2.user(t, "alice", true)
	p2 := h2.practice(t)
	h2.recordPair(t, pairSpec{task: "c1", choice: benchmark.ChoiceA, guess: benchmark.SideA, directModel: "frontier-a", units: 1})
	h2.recordPair(t, pairSpec{task: "c2", choice: benchmark.ChoiceA, guess: benchmark.SideB, directModel: "frontier-a", units: 1})
	st2, err := h2.store.Domain(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	rate2, err := p2.PublishedRate(ctx, benchmark.DomainSoftwareDevelopment, st2.CurrentEpochID)
	if err != nil {
		t.Fatal(err)
	}
	if rate2.GuessAccuracy != 0.5 || rate2.BlindnessLabel != "" {
		t.Errorf("at chance accuracy (%v) there must be no unblinding label, got %q",
			rate2.GuessAccuracy, rate2.BlindnessLabel)
	}
}

// TestWinsFollowThePositionNotTheLetter: a "B" vote is a platform WIN when the
// platform was rendered into side B. Getting this backwards would invert every
// statistic in the practice, silently.
func TestWinsFollowThePositionNotTheLetter(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	p := h.practice(t)

	h.recordPair(t, pairSpec{task: "t1", choice: benchmark.ChoiceB, guess: benchmark.SideB,
		directModel: "frontier-a", units: 1, platformOnB: true})
	st, err := h.store.Domain(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	rate, err := p.PublishedRate(ctx, benchmark.DomainSoftwareDevelopment, st.CurrentEpochID)
	if err != nil {
		t.Fatal(err)
	}
	if rate.W != 1 || rate.N != 1 {
		t.Errorf("a B vote with the platform on side B is a platform WIN; got w=%d m=%d", rate.W, rate.N)
	}
	if rate.GuessAccuracy != 1 {
		t.Errorf("a B guess with the platform on side B is CORRECT; got accuracy %v", rate.GuessAccuracy)
	}
}

// TestNoBareRateAccessorExists is acceptance item 18's negative, asserted
// structurally: no exported function or method in the package returns a rate
// without its epistemics. A convenience accessor is exactly how a number ends
// up on a screen naked.
func TestNoBareRateAccessorExists(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, filepath.Join(repoRoot, "internal/benchmark"), func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, pkg := range pkgs {
		for name, file := range pkg.Files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || !fn.Name.IsExported() {
					continue
				}
				checked++
				lower := strings.ToLower(fn.Name.Name)
				if !strings.Contains(lower, "rate") && !strings.Contains(lower, "winrate") {
					continue
				}
				// An exported name mentioning a rate must return the ONE
				// published shape, never a naked number.
				if fn.Type.Results == nil {
					continue
				}
				for _, res := range fn.Type.Results.List {
					if id, ok := res.Type.(*ast.Ident); ok && (id.Name == "float64" || id.Name == "float32") {
						t.Errorf("%s: exported %s returns a bare %s — every published rate travels inside PublishedRate with its epistemics (BENCH-REG §15)",
							filepath.Base(name), fn.Name.Name, id.Name)
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("the accessor scan found no exported functions — it would pass vacuously")
	}
	// The probe: the scan DOES see PublishedRate, so the rule above is live.
	found := false
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "PublishedRate" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("the scan cannot see PublishedRate — the assertion would be vacuous")
	}
}

// TestReadoutIsTheSixteenthSectionsViews: per-domain and per-epoch summaries,
// epoch history, the gate, the alarm, the done-directly figure, and the
// registered values that produced them — each marked read-only.
func TestReadoutIsTheSixteenthSectionsViews(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	p := h.practice(t)
	p.Floors = greenFloors
	h.recordWins(t, "a", 3, 3, "frontier-a")
	h.recordWins(t, "b", 2, 1, "frontier-b")

	out, err := p.Readout(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Domains) != len(benchmark.V0LaunchDomains()) {
		t.Fatalf("%d domain readouts, want the v0 roster's %d", len(out.Domains), len(benchmark.V0LaunchDomains()))
	}
	d := out.Domains[0]
	if d.Phase != benchmark.PhasePreGate {
		t.Errorf("phase = %q, want pre-gate before the gate has opened", d.Phase)
	}
	if len(d.History) != 2 {
		t.Errorf("%d epochs in the history view, want 2 side by side (§16)", len(d.History))
	}
	if d.Current.EpochID != d.CurrentEpoch {
		t.Error("the current published rate must be scoped to the current epoch")
	}
	if len(d.Registered) == 0 || d.Registered[0].Marker != benchmark.RegisteredMarker {
		t.Error("the readout must surface the registered values with their read-only marker (S18.4 R9)")
	}
	if d.DoneDirectly.HeuristicLabel != benchmark.LabelHeuristic {
		t.Error("the readout must carry the §13 done-directly figure")
	}
	if out.Protocol == "" || out.Marker != benchmark.RegisteredMarker {
		t.Error("the readout must name the registration it was produced under")
	}
	if out.PlatformGate.Open {
		t.Error("the 15.3 gate is shut at this accrual")
	}
	// Every historical epoch's rate carries the same epistemics — a history
	// view is not a place where the rules relax.
	for _, e := range d.History {
		if len(e.Rate.HonestClaims) == 0 {
			t.Errorf("epoch %s's rate has no honest-claims table", e.Epoch.ID)
		}
	}
}

// TestDeclineRateIsOverSampledNotAnswered is the brief R5 formula: the decline
// rate is declined ÷ SAMPLED, per (domain, epoch). The denominator is every
// pair drawn — including ones still awaiting an answer — because a requester
// who is asked and never answers is also selective participation, and measuring
// against answered pairs alone would hide exactly that (§4.2.5).
func TestDeclineRateIsOverSampledNotAnswered(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	p := h.practice(t)

	// One decline, one verdict, and two pairs still in flight.
	h.recordPair(t, pairSpec{task: "d1", decline: true})
	h.recordPair(t, pairSpec{task: "w1", choice: benchmark.ChoiceA, guess: benchmark.SideA,
		directModel: "frontier-a", units: 1})
	h.sampledPair(t, "f1")
	h.sampledPair(t, "f2")

	st, err := h.store.Domain(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	a, err := h.store.Accrual(ctx, benchmark.DomainSoftwareDevelopment, st.CurrentEpochID)
	if err != nil {
		t.Fatal(err)
	}
	if a.Answered != 2 {
		t.Errorf("answered = %d, want the 2 pairs that got an answer", a.Answered)
	}
	if a.Sampled != 4 {
		t.Errorf("sampled = %d, want all 4 drawn pairs (2 answered + 2 in flight)", a.Sampled)
	}
	rate, err := p.PublishedRate(ctx, benchmark.DomainSoftwareDevelopment, st.CurrentEpochID)
	if err != nil {
		t.Fatal(err)
	}
	if want := 1.0 / 4.0; rate.DeclineRate != want {
		t.Errorf("decline rate = %.4f, want %.4f (1 declined ÷ 4 sampled) — the R5 denominator is SAMPLED, not answered",
			rate.DeclineRate, want)
	}

	// A CLOSED epoch counts only its own answered pairs: an in-flight pair
	// belongs to the epoch it will land in, which is the current one.
	h.recordPair(t, pairSpec{task: "b1", choice: benchmark.ChoiceB, guess: benchmark.SideA,
		directModel: "frontier-b", units: 1})
	closed, err := h.store.Accrual(ctx, benchmark.DomainSoftwareDevelopment, st.CurrentEpochID)
	if err != nil {
		t.Fatal(err)
	}
	if closed.Sampled != closed.Answered {
		t.Errorf("a closed epoch counted %d sampled against %d answered — in-flight pairs belong to the CURRENT epoch",
			closed.Sampled, closed.Answered)
	}
}

// TestGuessAccuracyIsPerVoteAndPublishesItsOwnN is the D3 declared reading:
// §5's measurement is per VOTE, so a tie still measures blindness, and the
// sample it rests on is published as GuessN beside the win rate's n.
func TestGuessAccuracyIsPerVoteAndPublishesItsOwnN(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	p := h.practice(t)

	h.recordPair(t, pairSpec{task: "w1", choice: benchmark.ChoiceA, guess: benchmark.SideA, directModel: "frontier-a", units: 1})
	h.recordPair(t, pairSpec{task: "t1", choice: benchmark.ChoiceTie, guess: benchmark.SideA, directModel: "frontier-a", units: 1})
	h.recordPair(t, pairSpec{task: "t2", choice: benchmark.ChoiceBothBad, guess: benchmark.SideB, directModel: "frontier-a", units: 1})

	st, err := h.store.Domain(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	rate, err := p.PublishedRate(ctx, benchmark.DomainSoftwareDevelopment, st.CurrentEpochID)
	if err != nil {
		t.Fatal(err)
	}
	// The win rate rests on one non-tied pair; the blindness measurement rests
	// on all three votes — and BOTH n's are published, so neither is assumed.
	if rate.N != 1 {
		t.Errorf("n = %d, want 1 non-tied pair (ties never enter m, §8)", rate.N)
	}
	if rate.GuessN != 3 {
		t.Errorf("guess n = %d, want all 3 votes — a tie vote still saw both responses (§5)", rate.GuessN)
	}
	if want := 2.0 / 3.0; rate.GuessAccuracy != want {
		t.Errorf("guess accuracy = %.4f, want %.4f over the 3 votes", rate.GuessAccuracy, want)
	}
}
