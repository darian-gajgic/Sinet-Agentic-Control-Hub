package intake

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
)

// The P47 research-trigger list (Spec S06.3, discharging R03-OQ5): tasks
// whose output depends on facts in the world always include live research
// as a required step. The trigger layer is deterministic rules FIRST,
// classifier second — and the classifier may only ADD the data-bearing
// flag, never remove it. The v1 rule file is an operator-editable,
// versioned platform file (`intake/p47-triggers`); this is its seed
// content, the S06.3 table verbatim. Maintenance at v0 is manual operator
// edits; miss-driven auto-proposals wait for the v1 lesson gate (G2 D2.7).

// TriggerRule is one rule row: cue phrases match on word boundaries,
// case-insensitive; patterns are regular expressions for cues that are not
// plain phrases (currency amounts). Rules with neither (P47-6, P47-11)
// are classifier-detected classes, present so the rule file carries the
// full ratified table.
type TriggerRule struct {
	ID       string   `json:"id"`
	Class    string   `json:"class"`
	Cues     []string `json:"cues,omitempty"`
	Patterns []string `json:"patterns,omitempty"`
	Note     string   `json:"note,omitempty"`
}

// TriggerFile is the versioned rule file.
type TriggerFile struct {
	ID      string        `json:"id"`
	Version string        `json:"version"`
	Source  string        `json:"source"`
	Rules   []TriggerRule `json:"rules"`

	compiled []compiledRule
}

type compiledRule struct {
	rule TriggerRule
	res  []*regexp.Regexp
}

// Compile validates the file and prepares its matchers.
func (f *TriggerFile) Compile() error {
	if f.ID == "" || f.Version == "" {
		return fmt.Errorf("%w: trigger file requires id and version", ErrTaxonomy)
	}
	f.compiled = f.compiled[:0]
	seen := make(map[string]bool, len(f.Rules))
	for _, r := range f.Rules {
		if r.ID == "" || r.Class == "" {
			return fmt.Errorf("%w: trigger rule requires id and class", ErrTaxonomy)
		}
		if seen[r.ID] {
			return fmt.Errorf("%w: duplicate trigger rule %q", ErrTaxonomy, r.ID)
		}
		seen[r.ID] = true
		var res []*regexp.Regexp
		for _, cue := range r.Cues {
			re, err := regexp.Compile(`(?i)\b` + regexp.QuoteMeta(cue) + `\b`)
			if err != nil {
				return fmt.Errorf("%w: rule %s cue %q: %v", ErrTaxonomy, r.ID, cue, err)
			}
			res = append(res, re)
		}
		for _, p := range r.Patterns {
			re, err := regexp.Compile(p)
			if err != nil {
				return fmt.Errorf("%w: rule %s pattern %q: %v", ErrTaxonomy, r.ID, p, err)
			}
			res = append(res, re)
		}
		f.compiled = append(f.compiled, compiledRule{rule: r, res: res})
	}
	return nil
}

// Detect runs the deterministic rule layer over the request text: at most
// one hit per rule, carrying the first matching cue. Over-flagging is the
// safe direction — a hit only obliges research, and dismissal is the
// requester's alone (Spec S06.3).
func (f *TriggerFile) Detect(text string) []TriggerHit {
	var hits []TriggerHit
	for _, c := range f.compiled {
		for _, re := range c.res {
			if m := re.FindString(text); m != "" {
				hits = append(hits, TriggerHit{
					RuleID: c.rule.ID, Class: c.rule.Class,
					Cue: strings.TrimSpace(m), Source: "rule",
				})
				break
			}
		}
	}
	return hits
}

// seedClasses maps a seed rule id to its plain Class, built once.
var seedClasses = sync.OnceValue(func() map[string]string {
	m := make(map[string]string, 16)
	for _, r := range SeedTriggers().Rules {
		m[r.ID] = r.Class
	}
	return m
})

// ResearchSubject names, in plain words, the KIND of outside fact a P47 rule
// covers — the seed table's own Class ("Prices & costs" for P47-1, and so on).
// Every requester-facing sentence about a research obligation says THIS and
// never the rule id: the id is a machine value and stays in machine members
// (P3-GF13). The classes are distinct per rule, which is what keeps two rules
// landing on one step two visibly different facts once the id has left the
// sentence (the P3-RW-18 D3-R2 lesson).
//
// An id the seed table does not carry — the local classifier's own flag, or a
// rule an operator added to the file — has no class to name here, so the caller
// gets "" and says the honest generic thing rather than a confidently wrong
// specific one.
func ResearchSubject(ruleID string) string { return seedClasses()[ruleID] }

// Rule returns the rule by id, or nil.
func (f *TriggerFile) Rule(id string) *TriggerRule {
	for i := range f.Rules {
		if f.Rules[i].ID == id {
			return &f.Rules[i]
		}
	}
	return nil
}

// LoadTriggers reads an operator-edited rule file (strict JSON) and
// compiles it.
func LoadTriggers(path string) (*TriggerFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("intake: read trigger file %s: %w", path, err)
	}
	var f TriggerFile
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrTaxonomy, path, err)
	}
	if err := f.Compile(); err != nil {
		return nil, err
	}
	return &f, nil
}

// SeedTriggers returns the compiled v1 seed — the Spec S06.3 table. Cue
// lists carry the table's non-exhaustive cue examples as deterministic
// word-boundary phrases; tuning them is an operator file edit.
func SeedTriggers() *TriggerFile {
	f := &TriggerFile{
		ID:      "p47-triggers",
		Version: "v1",
		Source:  "Spec S06.3 seed table (P47-1..P47-11)",
		Rules: []TriggerRule{
			{
				ID: "P47-1", Class: "Prices & costs",
				Cues:     []string{"price", "prices", "pricing", "cost", "costs", "fee", "fees", "tariff", "subscription", "cheapest"},
				Patterns: []string{`[$€£¥]\s?\d`, `(?i)\b\d+(?:\.\d+)?\s?(?:usd|eur|gbp|chf)\b`},
			},
			{
				ID: "P47-2", Class: "Products & vendors",
				Cues: []string{"availability", "in stock", "best", "alternatives", "vendor", "vendors"},
				Note: "named commercial products, models, SKUs, and services are classifier-detected",
			},
			{
				ID: "P47-3", Class: "Laws, regulation & terms",
				Cues: []string{"legal", "law", "laws", "regulation", "regulations", "tax", "taxes", "licensing", "compliance", "terms of service", "tos", "gdpr"},
			},
			{
				ID: "P47-4", Class: "Technical interfaces & versions",
				Cues: []string{"api", "sdk", "library", "libraries", "package", "packages", "version", "versions", "deprecated", "deprecation"},
			},
			{
				ID: "P47-5", Class: "Schedules & external events",
				Cues: []string{"schedule", "timetable", "opening hours", "deadline", "deadlines"},
				Note: "dates and deadlines set by the requester do not count; requester-set timing resolves via the interview",
			},
			{
				ID: "P47-6", Class: "Churn-prone named entities",
				Note: "organizations, people-in-role, and live services are classifier-detected (fast-changing class per R03 §2.7)",
			},
			{
				ID: "P47-7", Class: "Temporal cues",
				Cues: []string{"current", "latest", "today", "now", "recent", "recently", "this year", "this month"},
			},
			{
				ID: "P47-8", Class: "Locality cues",
				Cues: []string{"near me", "nearby", "in my area", "shipping", "regional availability"},
			},
			{
				ID: "P47-9", Class: "Explicit lookup requests",
				Cues: []string{"look up", "check online", "verify", "find out", "search for"},
			},
			{
				ID: "P47-10", Class: "Prior art & external corpora",
				Cues: []string{"existing solutions", "competitors", "state of the art", "prior art"},
			},
			{
				ID: "P47-11", Class: "Requester-asserted external facts",
				Note: "a checkable requester claim the plan depends on is classifier-detected; verify node at standard tier and above (false-premise guard)",
			},
		},
	}
	if err := f.Compile(); err != nil {
		// The seed is compile-tested; a failure here is a programming error.
		panic(err)
	}
	return f
}
