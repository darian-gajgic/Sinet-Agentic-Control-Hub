package verify

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

// Entailment coverage for research-domain claims (Spec S07.4). v0 status:
// the launch domain is software, so this machinery is specified and SHIPS
// IDLE; it activates with the web-research domain at v0.1 (G1 D1.2; Spec
// S19) — and never before its calibration bar is recorded: thresholds and
// the mandatory-coverage bar are TBD-BRINGUP(entailment calibration set —
// planted supported/unsupported pairs + first real outcomes; pre-registered
// TPR/TNR bar before entailment gates unsupervised). The calibration seed
// lives in seeds.go; its TPR/TNR measurement needs the B4 local tier
// (deferral record in P3/measurements/).

// Claim is one deliverable claim with its citation.
type Claim struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	SourceURL string `json:"source_url"`
	// LoadBearing claims get MANDATORY entailment coverage; the rest are
	// sampled at ⚙ verification.entailment_sample_rate (G1 Def.2). Which
	// claims count as load-bearing is part of the TBD-BRINGUP bar.
	LoadBearing bool `json:"load_bearing,omitempty"`
}

// EntailmentPair is one judged pair. Claims are judged against the FETCHED
// source content, never against the deliverable's own citation text —
// judges are flippable by fabricated authority (Spec S07.4). The type makes
// that structural: there is no citation-text field to judge against.
type EntailmentPair struct {
	Claim Claim `json:"claim"`
	// FetchedContent is the fetch-and-check result body for the claim's
	// source (the mechanical citation pass supplies it, Spec S07.3
	// web-research pack; C3 confinement at v0.1).
	FetchedContent string `json:"fetched_content"`
}

// EntailmentVerdict is the checker's per-pair verdict.
type EntailmentVerdict string

const (
	EntailSupported   EntailmentVerdict = "supported"
	EntailUnsupported EntailmentVerdict = "unsupported"
	// EntailUnverifiable: the fetched content cannot decide the claim
	// (rotted source, paywall, empty fetch).
	EntailUnverifiable EntailmentVerdict = "unverifiable"
)

// EntailmentChecker is the local-tier seat seam (Spec S12; G3 Def.4:
// default seat Granite-Guardian-8B, CPU floor Flan-T5-0.8B for sampled
// checks). Wiring lands with the local tier (B4).
type EntailmentChecker interface {
	Entail(ctx context.Context, pair EntailmentPair) (EntailmentVerdict, error)
}

// EntailmentGate is the activation gate and coverage splitter. It ships
// idle at v0: Active is false for every domain until the web-research
// domain arrives AND the calibration bar is recorded.
type EntailmentGate struct {
	Settings Settings
	Checker  EntailmentChecker
	// Calibrated is set only when the pre-registered TPR/TNR bar has been
	// measured and recorded (TBD-BRINGUP; B4 local tier). Hard false until
	// then — entailment never gates unsupervised uncalibrated (G3 Def.4).
	Calibrated bool
}

// Active reports whether entailment runs for a domain (Spec S07.4): only
// the web-research domain, only calibrated, only with a checker seat.
func (g *EntailmentGate) Active(domain string) bool {
	return g != nil && domain == DomainWebResearch && g.Calibrated && g.Checker != nil
}

// Coverage splits claims into the checked set per the ratified rule:
// load-bearing claims are MANDATORY; the rest are sampled at
// ⚙ verification.entailment_sample_rate (TBD-BRINGUP, default 0). Sampling
// is deterministic per claim id (hash-based), so a re-run checks the same
// set — no RNG state in verification.
func (g *EntailmentGate) Coverage(claims []Claim) (checked []Claim, sampledOut int, err error) {
	rate, err := g.Settings.Float(keyEntailmentSample)
	if err != nil {
		return nil, 0, fmt.Errorf("verify: read ⚙ %s: %w", keyEntailmentSample, err)
	}
	for _, c := range claims {
		if c.LoadBearing {
			checked = append(checked, c)
			continue
		}
		if sampleHash(c.ID) < rate {
			checked = append(checked, c)
		} else {
			sampledOut++
		}
	}
	return checked, sampledOut, nil
}

// sampleHash maps a claim id to [0,1) deterministically.
func sampleHash(id string) float64 {
	sum := sha256.Sum256([]byte(id))
	v := binary.BigEndian.Uint64(sum[:8])
	return float64(v) / float64(^uint64(0))
}

// EntailmentOutcome is one checked pair's result.
type EntailmentOutcome struct {
	ClaimID string            `json:"claim_id"`
	Verdict EntailmentVerdict `json:"verdict"`
	// Mandatory records whether the pair was load-bearing (mandatory
	// coverage) or sampled in.
	Mandatory bool `json:"mandatory"`
}

// Check runs entailment over the covered pairs. Callers gate on Active —
// calling an inactive gate is an error, not a silent pass (Spec S07.4 idle
// posture: the pipeline records "idle", it never fakes coverage).
func (g *EntailmentGate) Check(ctx context.Context, domain string, pairs []EntailmentPair) ([]EntailmentOutcome, error) {
	if !g.Active(domain) {
		return nil, fmt.Errorf("%w: entailment is idle (domain %q; calibrated=%v)", ErrSeamMissing, domain, g != nil && g.Calibrated)
	}
	var out []EntailmentOutcome
	for _, p := range pairs {
		v, err := g.Checker.Entail(ctx, p)
		if err != nil {
			return nil, fmt.Errorf("verify: entailment on claim %q: %w", p.Claim.ID, err)
		}
		out = append(out, EntailmentOutcome{ClaimID: p.Claim.ID, Verdict: v, Mandatory: p.Claim.LoadBearing})
	}
	return out, nil
}
