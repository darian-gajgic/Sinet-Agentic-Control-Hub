package claudecli_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters/claudecli"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/verify"
)

// rider1_golden_test.go — D3 rider 1 (P-T06-5, S07.9): the golden-set re-run on
// the ratified opus-4-8 judge, gated by SINET_RIDER1=1 (a PAID leg — the
// packet's pre-ratified obligation). It runs the 26-case golden seed
// (verify.SeedGoldenSet) through the REAL two-axis judge prompts (the S07.5
// Compliance + Sanity shapes, the same instructions stage.EngineJudge sends) on
// claude-opus-4-8 via the committed claudecli adapter, clean context (artifact +
// ACs only, no transcript). Judge-as-classifier: a case is flagged (REVISE) when
// either axis finds a blocking issue; TPR/TNR vs the human labels. Projection
// $2.10, ratified STOP LINE $5.00 — the harness aborts + records if cumulative
// cost reaches it.

const (
	rider1Model    = "claude-opus-4-8"
	rider1StopLine = 5.00
)

// axis-1 (compliance) + axis-2 (sanity) prompt shapes — the S07.5 judge shapes
// (replicated from stage/engines.go's axis schemas; unexported there).
const rider1Axis1 = `You are the spec-compliance judge (S07.5 axis 1). For each numbered acceptance criterion, decide pass/fail over the artifact. A PASS needs an exact substring quote of the artifact as evidence; if you cannot quote it, the verdict is unknown. Output EXACTLY one JSON object:
{"verdicts":[{"key":"AC-1","pass":true,"unknown":false,"evidence":"..."}],"blocker":true}
where "blocker" is true if ANY criterion fails.`

const rider1Axis2 = `You are the outcome-sanity judge (S07.5 axis 2), separately prompted. Probes: would a reasonable user consider this done and good? what would a well-informed person expect that is absent? any unrequested side effects? does it meet the expert standard? Unrequested changes are failures, not bonuses. Output EXACTLY one JSON object:
{"reasonable_user":"...","expert_standard":"...","blocker":true}
where "blocker" is true if there is any genuine outcome/sanity problem a professional would not ship.`

func rider1Judge(t *testing.T, e *e2eEnv, ctx context.Context, runID, instr, artifact string, acs []string) (blocker bool, cost float64, ok bool) {
	t.Helper()
	e.claimedRun(t, runID)
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\nACCEPTANCE CRITERIA:\n", instr)
	for _, ac := range acs {
		fmt.Fprintf(&b, "- %s\n", ac)
	}
	fmt.Fprintf(&b, "\nARTIFACT UNDER JUDGMENT:\n%s\n\nRespond with ONLY the JSON object.", artifact)
	a := &claudecli.Adapter{Settings: settings.New()}
	out, err := e.drv.Drive(ctx, a, adapters.StartRequest{
		RunID: runID, UserID: "u1", Model: rider1Model, Cwd: t.TempDir(), WorkDir: t.TempDir(),
		Worker:         adapters.CompiledWorker{Prompt: b.String(), ToolAllowlist: []string{"Read"}},
		CeilingCostUSD: 0.20, CeilingSteps: 1,
	})
	if err != nil || out.Kind != adapters.OutcomeCompleted {
		t.Logf("rider1 judge call on %s: kind=%v err=%v", runID, out.Kind, err)
		return false, 0, false
	}
	if out.Totals != nil {
		cost = out.Totals.EngineCostUSD
	}
	return parseBlocker(out.ResultText), cost, true
}

// parseBlocker extracts "blocker": true/false from the first JSON object in the
// judge's output (robust to surrounding prose).
func parseBlocker(text string) bool {
	i := strings.Index(text, "{")
	j := strings.LastIndex(text, "}")
	if i < 0 || j <= i {
		return strings.Contains(strings.ToLower(text), "fail") // conservative fallback
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(text[i:j+1]), &m); err == nil {
		if b, ok := m["blocker"].(bool); ok {
			return b
		}
	}
	return strings.Contains(strings.ToLower(text[i:j+1]), "\"pass\":false") || strings.Contains(strings.ToLower(text), "\"blocker\": true")
}

type rider1Case struct {
	id, class, expected string
	defective, flagged  bool
	artLen              int
}

func TestRider1GoldenSetOpus(t *testing.T) {
	if os.Getenv("SINET_RIDER1") != "1" {
		t.Skip("SANCTIONED SKIP (CONVENTIONS §10): rider 1 (P-T06-5 golden set on opus-4-8) is a PAID leg, runs only under SINET_RIDER1=1")
	}
	if _, err := exec.LookPath(claudecli.DefaultBinary); err != nil {
		t.Skip("SANCTIONED SKIP: no claude engine installed")
	}
	gs := verify.SeedGoldenSet()
	if err := gs.Validate(); err != nil {
		t.Fatalf("golden set: %v", err)
	}

	var results []rider1Case
	var totalCost float64
	tpN, tpHit, tnN, tnHit := 0, 0, 0, 0

	limit := len(gs.Cases)
	if v := os.Getenv("SINET_RIDER1_LIMIT"); v != "" {
		fmt.Sscanf(v, "%d", &limit)
	}
	for ci, c := range gs.Cases {
		if ci >= limit {
			break
		}
		if totalCost >= rider1StopLine {
			t.Logf("STOP LINE $%.2f reached at $%.4f — halting (recorded partial)", rider1StopLine, totalCost)
			break
		}
		e := newE2E(t)
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		b1, cost1, ok1 := rider1Judge(t, e, ctx, "g1-"+c.ID, rider1Axis1, c.Artifact, c.ACs)
		b2, cost2, ok2 := rider1Judge(t, e, ctx, "g2-"+c.ID, rider1Axis2, c.Artifact, c.ACs)
		cancel()
		totalCost += cost1 + cost2
		if !ok1 && !ok2 {
			t.Logf("case %s: both axes failed to run — skipped", c.ID)
			continue
		}
		flagged := b1 || b2
		defective := c.DefectClass != "clean"
		results = append(results, rider1Case{c.ID, c.DefectClass, c.Expected, defective, flagged, len(c.Artifact)})
		if defective {
			tpN++
			if flagged {
				tpHit++
			}
		} else {
			tnN++
			if !flagged {
				tnHit++
			}
		}
		t.Logf("case %-5s class=%-16s defective=%v flagged=%v (a1=%v a2=%v) cost=$%.4f cum=$%.4f", c.ID, c.DefectClass, defective, flagged, b1, b2, cost1+cost2, totalCost)
	}

	tpr, tnr := ratio(tpHit, tpN), ratio(tnHit, tnN)
	// Statistically-corrected view: TPR/TNR ARE computed vs ground-truth human
	// labels (the golden set), i.e. the judge-as-classifier confusion matrix —
	// the S07.11 correction (never take judge self-reports raw). Wilson 95% CIs.
	tprLo, tprHi := wilson(tpHit, tpN)
	tnrLo, tnrHi := wilson(tnHit, tnN)
	// Length bias (P-T06-3): point-biserial correlation of artifact length with
	// the judge's flag decision (a proxy — does length sway the verdict?).
	lb := lengthBiasCorr(results)

	t.Logf("RIDER1_RESULT judge=%s cases=%d TPR=%.3f[%.2f,%.2f] (%d/%d) TNR=%.3f[%.2f,%.2f] (%d/%d) length_bias_r=%.3f total_cost=$%.4f stop=$%.2f",
		rider1Model, len(results), tpr, tprLo, tprHi, tpHit, tpN, tnr, tnrLo, tnrHi, tnHit, tnN, lb, totalCost, rider1StopLine)
	if totalCost > rider1StopLine {
		t.Errorf("spend $%.4f exceeded the ratified stop line $%.2f", totalCost, rider1StopLine)
	}
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

func wilson(k, n int) (lo, hi float64) {
	if n == 0 {
		return 0, 1
	}
	const z = 1.959963984540054
	fn, p := float64(n), float64(k)/float64(n)
	z2 := z * z
	den := 1 + z2/fn
	c := (p + z2/(2*fn)) / den
	h := (z / den) * math.Sqrt(p*(1-p)/fn+z2/(4*fn*fn))
	lo, hi = c-h, c+h
	if lo < 0 {
		lo = 0
	}
	if hi > 1 {
		hi = 1
	}
	return
}

// lengthBiasCorr is the point-biserial correlation between artifact length and
// the binary flag decision over the judged cases.
func lengthBiasCorr(rs []rider1Case) float64 {
	n := len(rs)
	if n < 3 {
		return 0
	}
	var sx, sy float64
	for _, r := range rs {
		sx += float64(r.artLen)
		if r.flagged {
			sy += 1
		}
	}
	mx, my := sx/float64(n), sy/float64(n)
	var sxy, sxx, syy float64
	for _, r := range rs {
		dx := float64(r.artLen) - mx
		dy := b2f(r.flagged) - my
		sxy += dx * dy
		sxx += dx * dx
		syy += dy * dy
	}
	if sxx == 0 || syy == 0 {
		return 0
	}
	return sxy / math.Sqrt(sxx*syy)
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
