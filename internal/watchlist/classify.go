package watchlist

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
)

// classify.go — the S14.6 ¶2 second pass: EVERY hit (page diff, feed entry, API
// diff) is classified on the local tier. It costs no allowance (Operating
// reality) and it never replaces the platform's own derivations: the model
// annotates {relevant lane(s), change class, one-line summary}, while SEVERITY
// is computed mechanically from the change class and the WATCH ROW's lane
// (S14.4 fixes that rule, and the row is server-side truth — a hit's body never
// decides its own lane or severity).
//
// Degradation is honest end to end: an absent local stack (ErrStackAbsent), a
// call error, an abstain or an unparseable answer all yield "unclassified —
// operator reads it". The hit still becomes a card; it is never dropped and a
// label is never fabricated (the B5-3 Tier-1 precedent).

// AdvisoryMeter opens a short-lived platform run for a run-less advisory local
// call and returns its id plus a settle func (the §28/§31 AdvisoryMeter seam).
// A watch hit has no run — the watchlist is not a pipeline stage — so every
// second-pass call rides this seam. With no meter the second pass is skipped
// HONESTLY (the hit is recorded unclassified), never issued unmetered.
type AdvisoryMeter func(ctx context.Context, purpose string) (runID string, settle func(), err error)

// classifyMaxTokens is the per-duty length cap for the watchlist triage duty (a
// structural constant set by the caller — S18 ratifies no key, the §26 reading).
const classifyMaxTokens = 400

// detailCap bounds the hit excerpt handed to the classifier and carried on the
// finding payload (refs-not-blobs, P-T07-5; ⚙ state.event_payload_cap is the
// hard gate above it).
const detailCap = 1500

// Classification is one second-pass verdict.
type Classification struct {
	// Lanes are the lanes the model judged relevant. The watch row's own lane
	// always leads the list — the model can add, never replace.
	Lanes []string
	// Class is one of the S14.2 change classes, or ClassUnclear when the pass
	// abstained or could not run.
	Class string
	// Summary is the one-line summary. When unclassified it is a deterministic
	// platform-authored line, never model output.
	Summary string
	// Classified is false when no usable model verdict was obtained.
	Classified bool
	// Note records WHY a pass is unclassified, so the card can say so.
	Note string
}

// WatchlistSchema is the S14.6 second-pass schema, built HERE in
// internal/watchlist (internal/local stays a read-only consumer — the B5-3 OQ6
// precedent). It follows the local/schema.go SHAPE: a REQUIRED free-text
// `reason` FIRST so the model's reasoning leads the token region, then the
// constrained fields, with `unclear` as the mandatory abstain member on the
// change class (local's assertAbstain belt confirms it for a Classification
// call). Properties are emitted in ORDER — a Go map would marshal them
// alphabetically and reorder the grammar.
func WatchlistSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{` +
		`"reason":{"type":"string","description":"brief free-text reasoning — fill this FIRST, before the labels"},` +
		`"change_class":{"type":"string","enum":["price","terms","limits","models","endpoints","billing-regime","none","unclear"],"description":"what kind of outside-world change this is; none = irrelevant to Sinet; unclear = cannot tell (abstain — NEVER guess)"},` +
		`"lanes":{"type":"array","items":{"type":"string","enum":["anthropic","zai","local"]},"description":"the Sinet lanes this change affects; empty when it affects none"},` +
		`"summary":{"type":"string","description":"one-line summary of the change, plain language"}` +
		`},"required":["reason","change_class","lanes","summary"],"additionalProperties":false}`)
}

// Classifier runs the second pass on the local tier.
type Classifier struct {
	// Duty is the local duty caller; nil (or an unconfigured stack) degrades to
	// unclassified.
	Duty *local.Duty
	// Meter opens the platform run the $0 D7 row lands on; nil skips the pass.
	Meter AdvisoryMeter
}

// Classify runs the second pass for one hit.
func (c *Classifier) Classify(ctx context.Context, h Hit) Classification {
	base := Classification{
		Lanes:   laneList(h.Lane, nil),
		Class:   ClassUnclear,
		Summary: fallbackSummary(h),
	}
	if c == nil || c.Duty == nil {
		base.Note = "no local tier configured — the hit is recorded unclassified for the operator to read"
		return base
	}
	if c.Meter == nil {
		base.Note = "no advisory meter wired — a $0 D7 row could not be written, so the second pass was skipped rather than issued unmetered"
		return base
	}
	runID, settle, err := c.Meter(ctx, "watchlist")
	if err != nil {
		base.Note = "advisory meter unavailable: " + err.Error()
		return base
	}
	if settle != nil {
		defer settle()
	}

	schema := WatchlistSchema()
	res, err := c.Duty.Call(ctx, runID, local.DutyRequest{
		Alias: local.AliasWatchlist,
		System: "You classify a detected change on an AI provider's page, feed or API. " +
			"Reason briefly FIRST, then answer. change_class ∈ {price, terms, limits, models, endpoints, billing-regime, none, unclear}. " +
			"Answer `unclear` when you cannot tell and `none` when the change is cosmetic or irrelevant — NEVER guess a label. " +
			"This is advisory: you annotate, you never stop, freeze or approve anything.",
		User:           classifyPrompt(h, schema),
		Schema:         schema,
		Name:           "watchlist-triage",
		MaxTokens:      classifyMaxTokens,
		Classification: true,
	})
	if err != nil {
		base.Note = "local classification unavailable: " + err.Error()
		return base
	}

	var out struct {
		Reason      string   `json:"reason"`
		ChangeClass string   `json:"change_class"`
		Lanes       []string `json:"lanes"`
		Summary     string   `json:"summary"`
	}
	if err := json.Unmarshal([]byte(res.Content), &out); err != nil {
		base.Note = "local classification returned unparseable output: " + err.Error()
		return base
	}
	if !knownClass(out.ChangeClass) || out.ChangeClass == ClassUnclear {
		base.Note = "local classifier abstained (change_class=" + out.ChangeClass + ")"
		if s := strings.TrimSpace(out.Summary); s != "" {
			base.Summary = oneLine(s)
		}
		return base
	}
	return Classification{
		Lanes:      laneList(h.Lane, out.Lanes),
		Class:      out.ChangeClass,
		Summary:    oneLine(firstNonEmpty(out.Summary, fallbackSummary(h))),
		Classified: true,
		Note:       oneLine(out.Reason),
	}
}

func classifyPrompt(h Hit, schema json.RawMessage) string {
	var b strings.Builder
	b.WriteString("A watchlist source changed. Classify the change.\n\n")
	fmt.Fprintf(&b, "source: %s\n", h.Source)
	fmt.Fprintf(&b, "watch kind: %s\n", h.Kind)
	if h.Lane != "" {
		fmt.Fprintf(&b, "watched lane or candidate: %s\n", h.Lane)
	}
	if h.Title != "" {
		fmt.Fprintf(&b, "headline: %s\n", h.Title)
	}
	b.WriteString("\nobserved change:\n")
	b.WriteString(clip(h.Detail, detailCap))
	b.WriteString("\n\nAnswer as JSON matching this schema exactly:\n")
	b.Write(schema)
	return b.String()
}

// fallbackSummary is the deterministic platform-authored one-liner used when no
// model verdict is available. It describes what was OBSERVED and never asserts
// a judgment.
func fallbackSummary(h Hit) string {
	if h.Title != "" {
		return oneLine(h.Title)
	}
	return fmt.Sprintf("%s watch %s changed", h.Kind, h.Source)
}

func knownClass(c string) bool {
	switch c {
	case ClassPrice, ClassTerms, ClassLimits, ClassModels, ClassEndpoints,
		ClassBillingRegime, ClassNone, ClassUnclear:
		return true
	}
	return false
}

// laneList puts the watch row's own lane first (server-side truth) and appends
// any additional lanes the model named, deduped.
func laneList(rowLane string, extra []string) []string {
	out := []string{}
	seen := map[string]bool{}
	if rowLane != "" {
		out, seen[rowLane] = append(out, rowLane), true
	}
	for _, l := range extra {
		if l != "" && !seen[l] {
			out, seen[l] = append(out, l), true
		}
	}
	return out
}

func oneLine(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " "))
	return clip(s, 300)
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
