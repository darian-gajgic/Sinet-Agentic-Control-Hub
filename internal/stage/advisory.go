package stage

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/verify"
)

// advisory.go — the composition-bridge adapter that takes the S07.4 entailment
// seam LIVE against the entailment duty alias (brief R17), plus the shared
// advisory metering helpers. verify already lives on stage's import graph, so
// the entailment adapter belongs here (the router.go/local.go bridge precedent,
// §26/R31). The S09.7 contradiction screen (R16) needs internal/memory, which
// stage MUST NOT import (§17 wall) — so THAT adapter lives in internal/shell
// (the composition root), reusing AdvisoryMeter + BeginAdvisory from here.
//
// Both advisory adapters are advisory local calls, so both meter a $0 D7 row
// (R18). The screen fires at a memory-gate write, which at v0 is a run-less
// human act — so an AdvisoryMeter seam provides a short-lived platform run per
// call, keeping every local call honestly metered (never a silently unmetered
// call). A nil meter falls back to a fixed advisory run id (Duty.Call then
// surfaces the honest unmetered-defect if that run is absent — the §26 contract).

// AdvisoryMeter provides a metering run for a run-less advisory local call. It
// returns the run id + a settle func (run-lifetime bookkeeping) + an error. The
// shell satisfies it over the run store; nil ⇒ the fixed-run fallback.
type AdvisoryMeter func(ctx context.Context, purpose string) (runID string, settle func(), err error)

// AdvisoryRunID is the fallback metering run when no AdvisoryMeter is wired.
const AdvisoryRunID = "platform.local-advisory"

// BeginAdvisory resolves a metering run for an advisory call (shared by the
// entailment checker here and the contradiction screen in the shell).
func BeginAdvisory(ctx context.Context, meter AdvisoryMeter, purpose string) (runID string, settle func()) {
	if meter == nil {
		return AdvisoryRunID, func() {}
	}
	id, done, err := meter(ctx, purpose)
	if err != nil || id == "" {
		return AdvisoryRunID, func() {}
	}
	if done == nil {
		done = func() {}
	}
	return id, done
}

// TruncateText bounds a string for a prompt (shared with the shell screen).
func TruncateText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ---- EntailmentChecker (entailment alias — R17) ----

type entailmentChecker struct {
	duty  *local.Duty
	meter AdvisoryMeter
}

var _ verify.EntailmentChecker = (*entailmentChecker)(nil)

// NewEntailmentChecker wraps the duty caller as the S07.4 entailment seat seam
// (R17): the Guardian seat, binary supported/unsupported + P(yes) from the
// verdict-token logprob margin. LIVE-WIRED but the verify.EntailmentGate stays
// IDLE (Active requires the web-research domain AND Calibrated) — this checker
// is what activates at v0.1. Nil-safe.
func NewEntailmentChecker(duty *local.Duty, meter AdvisoryMeter) verify.EntailmentChecker {
	if duty == nil {
		return nil
	}
	return &entailmentChecker{duty: duty, meter: meter}
}

// Entail judges a claim against its FETCHED source content (never the citation
// text — the type has no citation field, S07.4). An abstain/empty-source maps
// to unverifiable; a supported/unsupported verdict maps through (R17).
func (e *entailmentChecker) Entail(ctx context.Context, pair verify.EntailmentPair) (verify.EntailmentVerdict, error) {
	if pair.FetchedContent == "" {
		return verify.EntailUnverifiable, nil // empty fetch cannot decide (S07.4)
	}
	runID, settle := BeginAdvisory(ctx, e.meter, "entailment")
	defer settle()
	schema := local.EntailmentSchema()
	res, err := e.duty.Call(ctx, runID, local.DutyRequest{
		Alias:          local.AliasEntailment,
		System:         "You are a strict grounding checker. A claim is 'supported' ONLY if the SOURCE text entails it; otherwise 'unsupported'. Judge against the source ONLY, never outside knowledge. Reason briefly first, then the verdict.",
		User:           entailmentPrompt(pair, schema),
		Schema:         schema,
		Name:           "entailment",
		MaxTokens:      300,
		Classification: true,
	})
	if err != nil {
		return "", err
	}
	var out struct {
		Verdict string `json:"verdict"`
		Abstain bool   `json:"abstain"`
	}
	if err := json.Unmarshal([]byte(res.Content), &out); err != nil {
		return "", fmt.Errorf("stage: decode entailment output: %w", err)
	}
	if out.Abstain {
		return verify.EntailUnverifiable, nil
	}
	switch out.Verdict {
	case "supported":
		return verify.EntailSupported, nil
	case "unsupported":
		return verify.EntailUnsupported, nil
	default:
		return verify.EntailUnverifiable, nil
	}
}

func entailmentPrompt(pair verify.EntailmentPair, schema json.RawMessage) string {
	return fmt.Sprintf("CLAIM: %s\n\nSOURCE CONTENT:\n%s\n\nIs the claim supported by the source? Respond with JSON matching this schema:\n%s\n",
		pair.Claim.Text, TruncateText(pair.FetchedContent, 4000), schema)
}
