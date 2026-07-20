package intake

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// Interview taxonomies (Spec S06.5): each task family carries a versioned
// must-know taxonomy — the taxonomy decides WHAT must be asked; models only
// phrase. Taxonomies are knowledge objects: versioned, attributable,
// operator-editable, formally entering through the 8.3 knowledge gate when
// Spec S09 lands (B3); until then the seed ships here and its content is a
// B2-gate ratification item (TBD-P3 seed, Spec S06.5 + S19.5).

// Option is one labeled answer option of an interview question (Spec
// S06.5: 2–4 labeled options plus free text; free text is always
// available and is not an Option).
type Option struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Slot is one must-know slot of a family taxonomy. Weight is shipped in
// the taxonomy file and operator-editable (G1 P8).
type Slot struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	MustKnow string   `json:"must_know"`
	Weight   int      `json:"weight"`
	Question string   `json:"question"`
	Options  []Option `json:"options,omitempty"`
}

// Taxonomy is one versioned family question set.
type Taxonomy struct {
	ID      string `json:"id"`
	Family  Family `json:"family"`
	Version string `json:"version"`
	Source  string `json:"source"`
	Slots   []Slot `json:"slots"`
}

// Validate checks the structural rules a question set must satisfy.
func (t *Taxonomy) Validate() error {
	if t.ID == "" || t.Version == "" {
		return fmt.Errorf("%w: taxonomy requires id and version", ErrTaxonomy)
	}
	if len(t.Slots) == 0 {
		return fmt.Errorf("%w: taxonomy %q has no slots", ErrTaxonomy, t.ID)
	}
	seen := make(map[string]bool, len(t.Slots))
	for _, s := range t.Slots {
		switch {
		case s.ID == "":
			return fmt.Errorf("%w: slot without id in %q", ErrTaxonomy, t.ID)
		case seen[s.ID]:
			return fmt.Errorf("%w: duplicate slot %q in %q", ErrTaxonomy, s.ID, t.ID)
		case s.Weight <= 0:
			return fmt.Errorf("%w: slot %q needs a positive weight", ErrTaxonomy, s.ID)
		case s.Question == "":
			return fmt.Errorf("%w: slot %q without a question", ErrTaxonomy, s.ID)
		case len(s.Options) == 1 || len(s.Options) > 4:
			// S06.5: options come as 2–4 labeled choices (or none — free
			// text only).
			return fmt.Errorf("%w: slot %q must carry 0 or 2–4 options", ErrTaxonomy, s.ID)
		}
		seen[s.ID] = true
	}
	return nil
}

// Slot returns the slot by id, or nil.
func (t *Taxonomy) Slot(id string) *Slot {
	for i := range t.Slots {
		if t.Slots[i].ID == id {
			return &t.Slots[i]
		}
	}
	return nil
}

// Clearance computes the deterministic Clearance indicator (0–100, G1 P8):
// 100 × resolved weight / total weight over the active question set.
func (t *Taxonomy) Clearance(resolved map[string]bool) float64 {
	total, res := 0, 0
	for _, s := range t.Slots {
		total += s.Weight
		if resolved[s.ID] {
			res += s.Weight
		}
	}
	if total == 0 {
		return 0
	}
	return 100 * float64(res) / float64(total)
}

// Unresolved returns the unresolved slots, highest weight first (stable
// within equal weights) — the S06.5 card ordering.
func (t *Taxonomy) Unresolved(resolved map[string]bool) []Slot {
	var out []Slot
	for _, s := range t.Slots {
		if !resolved[s.ID] {
			out = append(out, s)
		}
	}
	for i := 1; i < len(out); i++ { // insertion sort: stable, tiny n
		for j := i; j > 0 && out[j].Weight > out[j-1].Weight; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// LoadTaxonomy reads an operator-edited taxonomy file (strict JSON,
// unknown fields rejected — the components.lock discipline for platform
// data files).
func LoadTaxonomy(path string) (*Taxonomy, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("intake: read taxonomy %s: %w", path, err)
	}
	var t Taxonomy
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&t); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrTaxonomy, path, err)
	}
	if err := t.Validate(); err != nil {
		return nil, err
	}
	return &t, nil
}

// SeedTaxonomies returns the v1 seed question sets (fresh copies): the
// software family seeded from the ClarifyCodeBench 10-type taxonomy and
// the generic fallback set for unmatched requests (Spec S06.5; R03 §2.3).
// Weights are evidence-informed from the same benchmark: the types models
// natively handle (Units, Numerical Precision) weigh least; the types
// every model measurably fails (Collection Semantics, Comparison Rules,
// Ordering & Atomicity) weigh most [R03 §2.3, S40].
func SeedTaxonomies() map[Family]*Taxonomy {
	return map[Family]*Taxonomy{
		FamilySoftware: softwareSeed(),
		FamilyGeneric:  genericSeed(),
	}
}

// softwareSeed is the ClarifyCodeBench 10-type taxonomy (arXiv:2607.00711
// Table 2), seeded per the S06.5 TBD-P3 drafting session at P3-B2-2.
func softwareSeed() *Taxonomy {
	return &Taxonomy{
		ID:      "software",
		Family:  FamilySoftware,
		Version: "v1",
		Source:  "ClarifyCodeBench 10-type taxonomy (arXiv:2607.00711, Table 2); seeded P3-B2-2 per Spec S06.5 TBD-P3; 8.3-gate entry pending S09 (B3)",
		Slots: []Slot{
			{
				ID: "behavior", Name: "Behavior", Weight: 10,
				MustKnow: "The required function, objective, or side effect is underspecified, so the intended behavior is not uniquely determined.",
				Question: "What exactly should this do — what behavior or outcome makes it correct?",
			},
			{
				ID: "terminology", Name: "Terminology", Weight: 10,
				MustKnow: "A domain term, action, or state is undefined, overloaded, or open to multiple interpretations.",
				Question: "Are there terms in the request that need pinning down — what do they mean here?",
			},
			{
				ID: "edge_cases", Name: "Edge Cases", Weight: 10,
				MustKnow: "Boundary or exceptional conditions are not specified, leaving behavior unclear for special inputs.",
				Question: "How should boundary and exceptional inputs be handled (empty, missing, malformed, extreme)?",
				Options: []Option{
					{Label: "Fail loudly on anything unexpected", Value: "fail_loud"},
					{Label: "Handle gracefully with sensible defaults", Value: "graceful"},
					{Label: "I'll specify per case", Value: "specify"},
				},
			},
			{
				ID: "collection_semantics", Name: "Collection Semantics", Weight: 12,
				MustKnow: "A collection, container, or state object is mentioned, but its membership, update rule, or access semantics are underspecified.",
				Question: "For the collections/state involved: what belongs in them, and how are they updated or accessed?",
			},
			{
				ID: "comparison_rules", Name: "Comparison Rules", Weight: 12,
				MustKnow: "The comparison key, tie-breaking rule, or stability requirement is not specified.",
				Question: "Where things are compared, sorted, or deduplicated: by what key, and how are ties broken?",
			},
			{
				ID: "ordering_atomicity", Name: "Ordering & Atomicity", Weight: 12,
				MustKnow: "Temporal order, simultaneity, or indivisible execution assumptions are unclear.",
				Question: "Does order of operations or atomicity matter here — what must happen before what, and what must never interleave?",
			},
			{
				ID: "indices_ranges", Name: "Indices & Ranges", Weight: 8,
				MustKnow: "Index bases, interval boundaries, or inclusion rules are underspecified.",
				Question: "For indices and ranges: zero- or one-based, and are boundaries inclusive or exclusive?",
				Options: []Option{
					{Label: "Zero-based, end-exclusive (language-conventional)", Value: "zero_exclusive"},
					{Label: "One-based, inclusive", Value: "one_inclusive"},
					{Label: "I'll specify", Value: "specify"},
				},
			},
			{
				ID: "output_format", Name: "Output Format", Weight: 8,
				MustKnow: "The required output structure, layout, or presentation rule is missing or unclear.",
				Question: "What shape should the output take?",
				Options: []Option{
					{Label: "Match the existing/conventional format", Value: "conventional"},
					{Label: "Exact format matters — I'll specify", Value: "specify"},
					{Label: "Any clear format is fine", Value: "any"},
				},
			},
			{
				ID: "units", Name: "Units", Weight: 6,
				MustKnow: "A quantity is specified without a clear unit, scale, prefix, or dimensional convention.",
				Question: "Are all quantities' units and scales unambiguous — if not, which convention applies?",
			},
			{
				ID: "numerical_precision", Name: "Numerical Precision", Weight: 6,
				MustKnow: "Precision, rounding, tolerance, or error handling requirements are unclear.",
				Question: "Do precision, rounding, or tolerance requirements apply to numeric results?",
			},
		},
	}
}

// genericSeed is the generic fallback set covering unmatched requests
// (Spec S06.2/S06.5). Drafted at P3-B2-2 (the TBD-P3 seed session);
// content is a B2-gate ratification item, 8.3-gate entry pending S09.
func genericSeed() *Taxonomy {
	return &Taxonomy{
		ID:      "generic",
		Family:  FamilyGeneric,
		Version: "v1",
		Source:  "Generic fallback must-know set; drafted P3-B2-2 per Spec S06.5 TBD-P3; 8.3-gate entry pending S09 (B3)",
		Slots: []Slot{
			{
				ID: "goal", Name: "Goal", Weight: 12,
				MustKnow: "The outcome that makes the task done is not uniquely determined.",
				Question: "What outcome makes this done — what will you look at to say it worked?",
			},
			{
				ID: "deliverable", Name: "Deliverable", Weight: 10,
				MustKnow: "The form the result should take is unspecified.",
				Question: "What form should the result take?",
				Options: []Option{
					{Label: "A document or write-up", Value: "document"},
					{Label: "A working change (code, config, files)", Value: "change"},
					{Label: "A recommendation with reasoning", Value: "recommendation"},
					{Label: "Something else — I'll describe", Value: "other"},
				},
			},
			{
				ID: "scope", Name: "Scope", Weight: 10,
				MustKnow: "What is in and out of scope is unstated.",
				Question: "Where are the edges — what should this explicitly include, and what should it not touch?",
			},
			{
				ID: "inputs", Name: "Inputs & sources", Weight: 8,
				MustKnow: "Which materials, sources, or prior work the task should build on is unclear.",
				Question: "What should this be based on — specific files, sources, or prior work?",
			},
			{
				ID: "constraints", Name: "Constraints", Weight: 8,
				MustKnow: "Hard constraints (tools, style, budget, privacy, compatibility) are unstated.",
				Question: "Any hard constraints — tools to use or avoid, style, budget, privacy?",
			},
			{
				ID: "audience", Name: "Audience", Weight: 6,
				MustKnow: "Who the result is for, and where it will be used, is unclear.",
				Question: "Who is this for, and where will it be used?",
			},
			{
				ID: "quality_bar", Name: "Quality bar", Weight: 6,
				MustKnow: "The intended thoroughness of the work is unstated.",
				Question: "How thorough should this be?",
				Options: []Option{
					{Label: "Quick and rough is fine", Value: "quick"},
					{Label: "Solid, normal quality", Value: "normal"},
					{Label: "As thorough as it takes", Value: "thorough"},
				},
			},
			{
				ID: "deadline", Name: "Timing", Weight: 4,
				MustKnow: "Whether timing matters is unstated.",
				Question: "Is there a deadline or preferred timing?",
			},
		},
	}
}
