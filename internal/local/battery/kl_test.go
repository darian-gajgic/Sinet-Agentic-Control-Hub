package battery

import (
	"context"
	"math"
	"testing"
)

func TestParseKLOutput(t *testing.T) {
	out := `
perplexity: calculating KL divergence over 250000 tokens
====== KL divergence statistics ======
Mean    KL divergence:      0.012345 ±   0.000456
Maximum KL divergence:      3.456789
99.90%  KL divergence:      0.234567
Median  KL divergence:      0.001234
`
	r, err := ParseKLOutput(out)
	if err != nil {
		t.Fatalf("ParseKLOutput: %v", err)
	}
	if math.Abs(r.MeanKL-0.012345) > 1e-9 {
		t.Errorf("MeanKL = %v, want 0.012345", r.MeanKL)
	}
	if math.Abs(r.MaxKL-3.456789) > 1e-9 {
		t.Errorf("MaxKL = %v, want 3.456789", r.MaxKL)
	}
	if math.Abs(r.MedianKL-0.001234) > 1e-9 {
		t.Errorf("MedianKL = %v, want 0.001234", r.MedianKL)
	}
	if math.Abs(r.P99KL-0.234567) > 1e-9 {
		t.Errorf("P99KL = %v, want 0.234567", r.P99KL)
	}
}

func TestParseKLOutputNoStats(t *testing.T) {
	if _, err := ParseKLOutput("nothing here\nload time: 1s\n"); err == nil {
		t.Error("expected an error when no KL stats are present")
	}
}

func TestRunKLCheckTwoStepFlow(t *testing.T) {
	// Prove the two-step flow (baseline then compare) via an injected runner —
	// no real binary. The first call generates the baseline, the second is
	// parsed for the stats.
	var calls []string
	run := func(_ context.Context, _ string, args []string, _ string) (string, error) {
		calls = append(calls, joinHas(args, "--kl-divergence-base"))
		for _, a := range args {
			if a == "--kl-divergence" {
				return "Mean KL divergence: 0.005 ± 0.001\nMedian KL divergence: 0.002\n", nil
			}
		}
		return "baseline written", nil
	}
	res, err := runKLCheck(context.Background(), KLConfig{
		PerplexityBin: "llama-perplexity", BaselineModel: "fp16.gguf", QuantModel: "q5.gguf", CorpusPath: "corpus.txt",
	}, run)
	if err != nil {
		t.Fatalf("runKLCheck: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 llama-perplexity calls (baseline + compare), got %d", len(calls))
	}
	if math.Abs(res.MeanKL-0.005) > 1e-9 {
		t.Errorf("MeanKL = %v, want 0.005", res.MeanKL)
	}
	if res.QuantModel != "q5.gguf" || res.BaselineModel != "fp16.gguf" {
		t.Errorf("result did not carry the model ids: %+v", res)
	}
}

func TestRunKLCheckMissingArgs(t *testing.T) {
	if _, err := RunKLCheck(context.Background(), KLConfig{}); err == nil {
		t.Error("RunKLCheck should error on missing paths")
	}
}

func joinHas(args []string, want string) string {
	for _, a := range args {
		if a == want {
			return want
		}
	}
	return ""
}
