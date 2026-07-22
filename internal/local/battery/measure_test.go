package battery_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local/battery"
)

// measure_test.go — the RUNNABLE-STANDALOW T15 battery driver (brief R6/R9/R20/
// R23/R25): gated by SINET_MEASURE=1, it runs the seed suites against a LIVE
// llama-server /v1 endpoint and prints per-suite results + fitted calibrations
// as JSON, for the coordinator to record into P3/measurements/ Def.8 files. It
// is $0 (local inference only), consumes the exact deployed quant/engine, and
// is the same machinery the S14 pinned eval runner will drive at B5.
//
// Usage (one served model at a time — llama-server serves one model):
//   SINET_MEASURE=1 \
//   SINET_MEASURE_ENDPOINT=http://127.0.0.1:8899 \
//   SINET_MEASURE_MODEL="Qwen3.5-9B" \
//   SINET_MEASURE_SUITES=intake-triage,contradiction-screen,... \
//   go test ./internal/local/battery/ -run TestMeasureSuites -count=1 -v -timeout 30m

func TestMeasureSuites(t *testing.T) {
	if os.Getenv("SINET_MEASURE") != "1" {
		t.Skip("SANCTIONED SKIP (CONVENTIONS §10) — measurement driver, set SINET_MEASURE=1 with a live endpoint")
	}
	endpoint := os.Getenv("SINET_MEASURE_ENDPOINT")
	model := os.Getenv("SINET_MEASURE_MODEL")
	if endpoint == "" || model == "" {
		t.Fatal("SINET_MEASURE_ENDPOINT and SINET_MEASURE_MODEL required")
	}
	modelHash := os.Getenv("SINET_MEASURE_HASH")
	want := os.Getenv("SINET_MEASURE_SUITES") // comma list; empty ⇒ all classification suites

	client := local.NewClient(endpoint)
	runner := battery.NewRunner(client, nil, nil) // AC gate off (driver)
	target := battery.Target{Model: model, ModelHash: modelHash, EngineBuild: local.LlamaCppPin}

	type suiteReport struct {
		Duty         string               `json:"duty"`
		Kind         string               `json:"kind"`
		Passed       int                  `json:"passed"`
		Total        int                  `json:"total"`
		PassRate     float64              `json:"pass_rate"`
		WilsonLo     float64              `json:"wilson_lo"`
		WilsonHi     float64              `json:"wilson_hi"`
		TokensPerSec float64              `json:"tokens_per_s"`
		Calibration  *local.Calibration   `json:"calibration,omitempty"`
		Failures     []battery.CaseResult `json:"failures,omitempty"`
	}
	var reports []suiteReport

	// A loaded suite file (SINET_MEASURE_SUITE_FILE) overrides the seed set —
	// the grown ~200-pair entailment set (C8) runs this way against Guardian.
	suites := battery.SeedSuites()
	if f := os.Getenv("SINET_MEASURE_SUITE_FILE"); f != "" {
		s, err := battery.LoadSuite(f)
		if err != nil {
			t.Fatalf("LoadSuite %s: %v", f, err)
		}
		suites = []*battery.Suite{s}
	}

	for _, s := range suites {
		if want != "" && !contains(want, s.Duty) {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		out, err := runner.RunSuite(ctx, s, target)
		cancel()
		if err != nil {
			t.Logf("suite %s: RUN ERROR %v", s.Duty, err)
			continue
		}
		rep := suiteReport{
			Duty: s.Duty, Kind: string(s.Kind),
			Passed: out.Result.Passed, Total: out.Result.Total, PassRate: out.Result.PassRate(),
			WilsonLo: out.Result.WilsonLo, WilsonHi: out.Result.WilsonHi, TokensPerSec: out.Result.TokensPerSec,
		}
		for _, c := range out.Cases {
			if !c.Pass {
				rep.Failures = append(rep.Failures, c)
			}
		}
		// Per-duty calibration (R25): fit over the suite's labeled margins.
		if s.Kind == battery.KindClassification && len(out.Labeled) >= 4 {
			key := local.CalibrationKey{Duty: s.Duty, ModelHash: modelHash, EngineBuild: local.LlamaCppPin}
			if cal, ferr := local.Fit(key, out.Labeled, 0.15); ferr == nil {
				rep.Calibration = &cal
			}
		}
		reports = append(reports, rep)
		t.Logf("suite %s: %d/%d (%.1f%%) Wilson[%.2f,%.2f] %.1f t/s", s.Duty, rep.Passed, rep.Total, rep.PassRate*100, rep.WilsonLo, rep.WilsonHi, rep.TokensPerSec)
	}
	blob, _ := json.MarshalIndent(struct {
		Model   string        `json:"model"`
		Engine  string        `json:"engine_build"`
		Reports []suiteReport `json:"reports"`
	}{model, local.LlamaCppPin, reports}, "", "  ")
	t.Logf("MEASURE_JSON_BEGIN\n%s\nMEASURE_JSON_END", blob)
}

func contains(csv, want string) bool {
	start := 0
	for i := 0; i <= len(csv); i++ {
		if i == len(csv) || csv[i] == ',' {
			if csv[start:i] == want {
				return true
			}
			start = i + 1
		}
	}
	return false
}
