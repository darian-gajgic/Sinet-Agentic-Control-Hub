package local

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

func TestWilsonInterval(t *testing.T) {
	// A known case: 8/10 → the 95% Wilson interval is roughly [0.49, 0.94].
	lo, hi := WilsonInterval(8, 10)
	if lo < 0.45 || lo > 0.52 || hi < 0.90 || hi > 0.96 {
		t.Errorf("Wilson(8,10) = [%.3f, %.3f], outside the expected band", lo, hi)
	}
	if lo, hi := WilsonInterval(0, 0); lo != 0 || hi != 1 {
		t.Errorf("Wilson(0,0) = [%v,%v], want [0,1]", lo, hi)
	}
	// Full pass narrows toward 1 but never above it.
	if _, hi := WilsonInterval(30, 30); hi > 1.0 {
		t.Errorf("Wilson upper bound %v exceeds 1", hi)
	}
}

func TestDecidePromotionRule(t *testing.T) {
	// Challenger clearly wins 2 duties (non-overlapping Wilson, faster), ties a
	// third with overlap → promote (≥2 wins).
	inc := []SuiteResult{
		{Duty: "a", WilsonLo: 0.50, WilsonHi: 0.70, TokensPerSec: 40},
		{Duty: "b", WilsonLo: 0.40, WilsonHi: 0.60, TokensPerSec: 40},
		{Duty: "c", WilsonLo: 0.80, WilsonHi: 0.95, TokensPerSec: 40},
	}
	ch := []SuiteResult{
		{Duty: "a", WilsonLo: 0.75, WilsonHi: 0.90, TokensPerSec: 45}, // wins: lo>inc.hi, faster
		{Duty: "b", WilsonLo: 0.65, WilsonHi: 0.85, TokensPerSec: 40}, // wins: lo>inc.hi, equal
		{Duty: "c", WilsonLo: 0.82, WilsonHi: 0.97, TokensPerSec: 45}, // overlaps → not a win
	}
	p := DecidePromotion(inc, ch)
	if !p.Promote || p.Wins != 2 {
		t.Fatalf("expected promote with 2 wins, got %+v", p)
	}
}

func TestDecidePromotionRefusedOnSlowerChallenger(t *testing.T) {
	// Challenger's accuracy wins but it is SLOWER → not a win (equal-or-better
	// tokens/s is required, S12.10).
	inc := []SuiteResult{{Duty: "a", WilsonLo: 0.5, WilsonHi: 0.7, TokensPerSec: 40}, {Duty: "b", WilsonLo: 0.5, WilsonHi: 0.7, TokensPerSec: 40}}
	ch := []SuiteResult{{Duty: "a", WilsonLo: 0.75, WilsonHi: 0.9, TokensPerSec: 30}, {Duty: "b", WilsonLo: 0.75, WilsonHi: 0.9, TokensPerSec: 30}}
	if p := DecidePromotion(inc, ch); p.Promote {
		t.Errorf("a slower challenger must not promote (equal-or-better tokens/s), got %+v", p)
	}
}

func TestDecidePromotionRefusedOnOverlap(t *testing.T) {
	inc := []SuiteResult{{Duty: "a", WilsonLo: 0.5, WilsonHi: 0.8, TokensPerSec: 40}, {Duty: "b", WilsonLo: 0.5, WilsonHi: 0.8, TokensPerSec: 40}}
	ch := []SuiteResult{{Duty: "a", WilsonLo: 0.7, WilsonHi: 0.9, TokensPerSec: 45}, {Duty: "b", WilsonLo: 0.7, WilsonHi: 0.9, TokensPerSec: 45}}
	if p := DecidePromotion(inc, ch); p.Promote {
		t.Errorf("overlapping intervals must not promote, got %+v", p)
	}
}

// swapTestEnv wires a real registry + DB + calstore for the swap-gate tests.
type swapTestEnv struct {
	db    *storage.DB
	reg   *settings.Registry
	log   *eventlog.Log
	store *CalStore
}

func newSwapEnv(t *testing.T) *swapTestEnv {
	t.Helper()
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	log := eventlog.New(db, reg)
	if err := reg.Attach(ctx, db, log); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if err := db.ReapplySettings(ctx); err != nil {
		t.Fatalf("reapply: %v", err)
	}
	return &swapTestEnv{db: db, reg: reg, log: log, store: NewCalStore(db)}
}

// recordingAliasWriter captures the alias map the gate writes (and proves the
// gate is the write surface).
type recordingAliasWriter struct {
	last  map[string]string
	calls int
}

func (w *recordingAliasWriter) SetAliasMap(_ context.Context, m map[string]string, _, _ string) error {
	w.last = m
	w.calls++
	return nil
}

func TestSwapGateP_T15_1_RefusesUncalibratedTarget(t *testing.T) {
	env := newSwapEnv(t)
	ctx := context.Background()
	writer := &recordingAliasWriter{}
	gate := NewSwapGate(SwapDeps{Settings: env.reg, Store: env.store, Writer: writer, Events: env.log})

	// The workhorse-alternate (Gemma) has a recorded model hash? No — it is not
	// pulled/hashed, so use the fast seat (Qwen3.5-4B, hashed) as a servable
	// target with a known hash. Retarget intake-triage → fast with NO calibration.
	_, err := gate.Retarget(ctx, "intake-triage", "fast", "op1", "test flip")
	if !errors.Is(err, ErrUncalibratedTarget) {
		t.Fatalf("expected ErrUncalibratedTarget, got %v", err)
	}
	if writer.calls != 0 {
		t.Error("no ⚙ write may happen on a refused retarget (P-T15-1)")
	}

	// Add calibration but NOT the battery suite → still refused (both required).
	seat, _ := SeatByKey("fast")
	key := CalibrationKey{Duty: "intake-triage", ModelHash: seat.ModelHash(), EngineBuild: LlamaCppPin}
	if err := env.store.SaveCalibration(ctx, Calibration{Key: key, Threshold: 1.0, AcceptanceBar: 0.2, LabeledN: 40, MeetsBar: true}); err != nil {
		t.Fatalf("SaveCalibration: %v", err)
	}
	_, err = gate.Retarget(ctx, "intake-triage", "fast", "op1", "test flip")
	if !errors.Is(err, ErrBatterySuiteNotRun) {
		t.Fatalf("expected ErrBatterySuiteNotRun with calibration but no suite, got %v", err)
	}
	if writer.calls != 0 {
		t.Error("still no ⚙ write until BOTH calibration and battery suite exist")
	}
}

func TestSwapGateAdmitsCalibratedTargetAndFires7_3(t *testing.T) {
	env := newSwapEnv(t)
	ctx := context.Background()
	writer := &recordingAliasWriter{}
	var flagArgs []string // every model/alias the 7.3 seam was fired with
	flag := func(_ context.Context, model, reason string) ([]string, error) {
		flagArgs = append(flagArgs, model)
		return []string{"v-" + model}, nil // one fake worker version per target
	}
	gate := NewSwapGate(SwapDeps{Settings: env.reg, Store: env.store, Writer: writer, Flag: flag, Events: env.log})

	// Retarget intake-triage from its default target ("fast" = Qwen3.5-4B) to the
	// workhorse ("workhorse" = Qwen3.5-9B) — a REAL old≠new swap.
	newSeat, _ := SeatByKey("workhorse")
	oldSeat, _ := SeatByKey("fast") // the current default target
	key := CalibrationKey{Duty: "intake-triage", ModelHash: newSeat.ModelHash(), EngineBuild: LlamaCppPin}
	if err := env.store.SaveCalibration(ctx, Calibration{Key: key, Threshold: 1.0, AcceptanceBar: 0.2, LabeledN: 40, MeetsBar: true}); err != nil {
		t.Fatalf("SaveCalibration: %v", err)
	}
	if err := env.store.SaveBatterySuite(ctx, NewSuiteResult("intake-triage", newSeat.ModelHash(), LlamaCppPin, "t15-v0", 28, 30, 20)); err != nil {
		t.Fatalf("SaveBatterySuite: %v", err)
	}
	res, err := gate.Retarget(ctx, "intake-triage", "workhorse", "op1", "bakeoff flip")
	if err != nil {
		t.Fatalf("Retarget: %v", err)
	}
	if writer.calls != 1 || writer.last["intake-triage"] != "workhorse" {
		t.Fatalf("gate must write the alias map through the ONE write surface; got calls=%d last=%v", writer.calls, writer.last)
	}
	// F4: the 7.3 trigger fires against the SWAPPED ALIAS + the OLD target's
	// model — NEVER the new target (nothing was validated against it yet).
	joined := " " + strings.Join(flagArgs, " ") + " "
	if !strings.Contains(joined, " intake-triage ") {
		t.Errorf("7.3 must fire against the swapped alias 'intake-triage'; fired with %v", flagArgs)
	}
	if !strings.Contains(joined, " "+oldSeat.Model+" ") {
		t.Errorf("7.3 must fire against the OLD target model %q; fired with %v", oldSeat.Model, flagArgs)
	}
	if strings.Contains(joined, " "+newSeat.Model+" ") {
		t.Errorf("7.3 must NOT fire against the NEW target model %q (F4); fired with %v", newSeat.Model, flagArgs)
	}
	if len(res.FlaggedVersions) == 0 {
		t.Errorf("expected the flagged versions to ride the result, got %v", res.FlaggedVersions)
	}
}

func TestSwapGateRefusesUnknownTarget(t *testing.T) {
	env := newSwapEnv(t)
	gate := NewSwapGate(SwapDeps{Settings: env.reg, Store: env.store, Writer: &recordingAliasWriter{}, Events: env.log})
	if _, err := gate.Retarget(context.Background(), "intake-triage", "no-such-seat", "op1", "x"); !errors.Is(err, ErrTargetNotServable) {
		t.Errorf("unknown seat should be ErrTargetNotServable, got %v", err)
	}
}

func TestCalStoreRoundTrip(t *testing.T) {
	env := newSwapEnv(t)
	ctx := context.Background()
	key := CalibrationKey{Duty: "entailment", ModelHash: "hh", EngineBuild: "b10085"}
	c := Calibration{Key: key, Threshold: 1.5, AcceptanceBar: 0.1, AchievedError: 0.05, AcceptRate: 0.7, LabeledN: 200, MeetsBar: true,
		Isotonic: IsotonicMap{X: []float64{0.5, 2.0}, Y: []float64{0.4, 0.05}}, Margins: MarginSummary{N: 200, Min: 0, Max: 3, Mean: 1.4}}
	if err := env.store.SaveCalibration(ctx, c); err != nil {
		t.Fatalf("SaveCalibration: %v", err)
	}
	got, ok, err := env.store.Calibration(ctx, key)
	if err != nil || !ok {
		t.Fatalf("Calibration read: ok=%v err=%v", ok, err)
	}
	if got.Threshold != 1.5 || got.LabeledN != 200 || len(got.Isotonic.Y) != 2 {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	has, _ := env.store.HasCalibration(ctx, key)
	if !has {
		t.Error("HasCalibration should be true after save")
	}
	// Upsert (latest-wins).
	c.Threshold = 2.0
	if err := env.store.SaveCalibration(ctx, c); err != nil {
		t.Fatalf("re-save: %v", err)
	}
	got, _, _ = env.store.Calibration(ctx, key)
	if got.Threshold != 2.0 {
		t.Errorf("upsert should replace: threshold %v want 2.0", got.Threshold)
	}
}
