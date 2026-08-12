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

// b2GenericTaxonomyV1 returns the B2-ratified generic fallback question set,
// frozen for the same reason the software one is: b2Seeds() records what the
// B2 gate approved, and a record that follows the code is not a record. It is
// byte-identical to the pre-RW-12 seed (verified against 9484ce1 at drain r1,
// F3). P3-RW-12 reviewed the generic set and changed nothing, so this and the
// live seed agree TODAY — which is exactly the moment to freeze it, because
// the next packet to edit generic must not silently re-date the B2 gate.
func b2GenericTaxonomyV1() *intake.Taxonomy {
	return &intake.Taxonomy{
		ID:      "generic",
		Family:  intake.FamilyGeneric,
		Version: "v1",
		Source:  "Generic fallback must-know set; drafted P3-B2-2 per Spec S06.5 TBD-P3; 8.3-gate entry pending S09 (B3)",
		Slots: []intake.Slot{
			{
				ID: "goal", Name: "Goal", Weight: 12,
				MustKnow: "The outcome that makes the task done is not uniquely determined.",
				Question: "What outcome makes this done — what will you look at to say it worked?",
			},
			{
				ID: "deliverable", Name: "Deliverable", Weight: 10,
				MustKnow: "The form the result should take is unspecified.",
				Question: "What form should the result take?",
				Options: []intake.Option{
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
				Options: []intake.Option{
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
