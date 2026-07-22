package battery

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// kl.go — the S12.9 quant sanity check (brief R8): llama-perplexity
// --kl-divergence over the Sinet-domain corpus against the FP16/BF16 baseline.
// The check validates the WEIGHTS quant only; the KV cache stays fp16/q8_0 and
// is never quantized (S12.3 quant policy). llama-perplexity is an ADDITIONAL
// build target of the SAME pinned llama.cpp tree (b10085) — no new adoption,
// same lock entry (R32). This wrapper is BUILD-half machinery; the actual KL
// run is an EXECUTE-half measurement (OQ3: the two default-path deployed quants
// — Qwen3.5-9B Q5_K_M + Qwen3.5-4B Q4_K_M — with their FP16 baselines).
//
// The llama.cpp KL flow is two-step: generate baseline logits from the FP16
// model (--kl-divergence-base), then compare the quantized model against them
// (--kl-divergence). The output is parsed for the mean/median/max KL — a low
// mean KL means the quant tracks the baseline closely (the quant is faithful).

// KLResult is one quant's KL-divergence-vs-baseline outcome.
type KLResult struct {
	QuantModel    string  `json:"quant_model"`
	BaselineModel string  `json:"baseline_model"`
	MeanKL        float64 `json:"mean_kl"`
	MedianKL      float64 `json:"median_kl"`
	MaxKL         float64 `json:"max_kl"`
	P99KL         float64 `json:"p99_kl"`
	CorpusTokens  int     `json:"corpus_tokens"`
	RawTail       string  `json:"raw_tail"` // captured stats block for the Def.8 file
}

// KLConfig configures a KL run.
type KLConfig struct {
	PerplexityBin  string // path to the built llama-perplexity
	LDLibraryPath  string // the shared-lib dir (the b10085 build's bin/)
	BaselineModel  string // FP16/BF16 baseline GGUF
	QuantModel     string // the deployed quant GGUF
	CorpusPath     string // the assembled Sinet-domain corpus (text)
	BaselineOutput string // scratch path for the baseline logits (.dat)
	GPULayers      int    // -ngl (999 = all on GPU)
}

// runner is the exec seam (injected in tests so the parser is fixture-tested
// without a real binary).
type klRunner func(ctx context.Context, name string, args []string, ldLibraryPath string) (string, error)

func execRunner(ctx context.Context, name string, args []string, ldLibraryPath string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if ldLibraryPath != "" {
		cmd.Env = append(cmd.Environ(), "LD_LIBRARY_PATH="+ldLibraryPath)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// RunKLCheck executes the two-step KL flow and parses the result (R8). The
// baseline logits are generated once from the FP16 model, then the quant is
// compared against them.
func RunKLCheck(ctx context.Context, cfg KLConfig) (KLResult, error) {
	return runKLCheck(ctx, cfg, execRunner)
}

func runKLCheck(ctx context.Context, cfg KLConfig, run klRunner) (KLResult, error) {
	if cfg.PerplexityBin == "" || cfg.BaselineModel == "" || cfg.QuantModel == "" || cfg.CorpusPath == "" {
		return KLResult{}, fmt.Errorf("battery: KL check needs perplexity bin, baseline, quant, and corpus paths")
	}
	base := cfg.BaselineOutput
	if base == "" {
		base = cfg.QuantModel + ".kldbase"
	}
	ngl := cfg.GPULayers
	if ngl == 0 {
		ngl = 999
	}
	// Step 1: baseline logits from the FP16 model.
	baseArgs := []string{"-m", cfg.BaselineModel, "-f", cfg.CorpusPath, "--kl-divergence-base", base, "-ngl", strconv.Itoa(ngl)}
	if _, err := run(ctx, cfg.PerplexityBin, baseArgs, cfg.LDLibraryPath); err != nil {
		return KLResult{}, fmt.Errorf("battery: KL baseline generation (%s): %w", cfg.BaselineModel, err)
	}
	// Step 2: compare the quant against the baseline.
	cmpArgs := []string{"-m", cfg.QuantModel, "-f", cfg.CorpusPath, "--kl-divergence", "--kl-divergence-base", base, "-ngl", strconv.Itoa(ngl)}
	out, err := run(ctx, cfg.PerplexityBin, cmpArgs, cfg.LDLibraryPath)
	if err != nil {
		return KLResult{}, fmt.Errorf("battery: KL comparison (%s): %w\n%s", cfg.QuantModel, err, tail(out, 40))
	}
	res, perr := ParseKLOutput(out)
	if perr != nil {
		return KLResult{}, fmt.Errorf("battery: parse KL output: %w\n%s", perr, tail(out, 40))
	}
	res.QuantModel = cfg.QuantModel
	res.BaselineModel = cfg.BaselineModel
	return res, nil
}

// ParseKLOutput extracts the KL statistics from llama-perplexity's output. It is
// tolerant of label variations across llama.cpp versions (it scans lines
// carrying "KL divergence" and a leading descriptor). Fixture-tested.
func ParseKLOutput(out string) (KLResult, error) {
	var r KLResult
	found := false
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		low := strings.ToLower(line)
		if !strings.Contains(low, "kl divergence") && !strings.Contains(low, "kl-divergence") {
			continue
		}
		val, ok := firstFloat(line)
		if !ok {
			continue
		}
		switch {
		case strings.Contains(low, "mean"):
			r.MeanKL, found = val, true
		case strings.Contains(low, "median"):
			r.MedianKL, found = val, true
		case strings.Contains(low, "maximum") || strings.Contains(low, "max "):
			r.MaxKL, found = val, true
		case strings.Contains(low, "99"):
			r.P99KL, found = val, true
		}
	}
	if !found {
		return KLResult{}, fmt.Errorf("no KL divergence statistics found in output")
	}
	return r, nil
}

// firstFloat returns the first floating-point number on a line (after the
// colon, if any — so "Mean KL divergence: 0.0123 ± 0.0001" yields 0.0123).
func firstFloat(line string) (float64, bool) {
	if i := strings.IndexByte(line, ':'); i >= 0 {
		line = line[i+1:]
	}
	fields := strings.FieldsFunc(line, func(r rune) bool {
		return !(r >= '0' && r <= '9' || r == '.' || r == '-' || r == '+' || r == 'e' || r == 'E')
	})
	for _, f := range fields {
		if v, err := strconv.ParseFloat(f, 64); err == nil {
			return v, true
		}
	}
	return 0, false
}

func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
