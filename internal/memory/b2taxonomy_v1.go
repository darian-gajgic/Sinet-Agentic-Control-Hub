package memory

import (
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// b2taxonomy_v1.go — the software interview taxonomy EXACTLY as the B2 gate
// ratified it (P3/gates/B2-report.md §7 D3, operator, 2026-07-20).
//
// It is a frozen SNAPSHOT, and that is the point. b2Seeds() used to marshal
// whatever intake.SeedTaxonomies() returned today, which made the B2 entry a
// live pointer rather than a record: the moment P3-RW-12 revised the software
// set to v2, a fresh world's first boot would have written v2 content under
// the B2 gate's provenance — a governed row claiming a ratification that never
// happened — while an existing world's governed v1 file quietly diverged from
// the runtime seed. Both halves are closed: this snapshot keeps the B2 entry
// honest in a fresh world, and EnsureRW12TaxonomyGovernance supersedes it to
// the current content under ITS OWN provenance in every world (S09.8: a new
// version, never an in-place edit).
//
// The rule this establishes, for every seed governed hereafter: governance
// content is a snapshot of what was ratified. A ratification record that
// follows the code is not a record.
//
// The generic set has no snapshot here because P3-RW-12 did not change it —
// the live seed IS the ratified content. TestB2GovernedTaxonomyIsTheRatifiedSnapshot
// pins that, so a later packet that edits generic fails loudly here rather
// than silently re-dating the B2 gate.

// b2SoftwareTaxonomyV1 returns the B2-ratified software question set.
func b2SoftwareTaxonomyV1() *intake.Taxonomy {
	return &intake.Taxonomy{
		ID:      "software",
		Family:  intake.FamilySoftware,
		Version: "v1",
		Source:  "ClarifyCodeBench 10-type taxonomy (arXiv:2607.00711, Table 2); seeded P3-B2-2 per Spec S06.5 TBD-P3; 8.3-gate entry pending S09 (B3)",
		Slots: []intake.Slot{
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
				Options: []intake.Option{
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
				Options: []intake.Option{
					{Label: "Zero-based, end-exclusive (language-conventional)", Value: "zero_exclusive"},
					{Label: "One-based, inclusive", Value: "one_inclusive"},
					{Label: "I'll specify", Value: "specify"},
				},
			},
			{
				ID: "output_format", Name: "Output Format", Weight: 8,
				MustKnow: "The required output structure, layout, or presentation rule is missing or unclear.",
				Question: "What shape should the output take?",
				Options: []intake.Option{
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
