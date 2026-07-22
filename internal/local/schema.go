package local

import "encoding/json"

// schema.go — the S12.4 duty json_schemas (brief R16–R17). These are the
// engine-enforced constraint the client sends as `response_format:
// json_schema` (and the caller ALSO places in the prompt, S12.4). Every
// CLASSIFICATION schema carries an explicit abstain/`unclear` member by
// construction — a model is never schema-forced to fabricate a label (S12.4;
// duty.go's assertAbstain is the belt). The enum VALUES are passed in by the
// caller (the intake families/tiers/sizes are intake's vocabulary; the local
// package owns the SHAPE, not the intake constants — the import wall, R24).

// TriageSchema is the intake-triage duty schema (S06.2 family/stakes/size +
// data-bearing). The caller passes the enum values (intake.Family/Tier/size
// vocabularies). `abstain:true` means the request cannot be classified — the
// pipeline then fails closed to high stakes (S12.4/S06.2), never a fabricated
// label. The classifier may only ADD the data-bearing flag (S06.2 rule 4,
// enforced by the pipeline).
func TriageSchema(families, tiers, sizes []string) json.RawMessage {
	props := map[string]any{
		"family":       enumField(families),
		"stakes":       enumField(tiers),
		"size":         enumField(sizes),
		"data_bearing": map[string]any{"type": "boolean", "description": "true if the task rests on a live fact (a P47 data-bearing add, S06.2 rule 4)"},
		"abstain":      map[string]any{"type": "boolean", "description": "true if the request cannot be classified — the pipeline fails closed to high (S12.4)"},
		"reason":       map[string]any{"type": "string", "description": "one line of free-text reasoning"},
	}
	return objectSchema(props, []string{"family", "stakes", "size", "data_bearing", "abstain"})
}

// TieBreakSchema is the S08.8 step-2 tie-break duty schema on the utility
// alias (§8 reading 2): {pick ∈ candidate ids ∪ "abstain", reason one line}.
// "abstain" (or any error) → the deterministic degraded order (R21).
func TieBreakSchema(candidateIDs []string) json.RawMessage {
	picks := make([]string, 0, len(candidateIDs)+1)
	picks = append(picks, candidateIDs...)
	picks = append(picks, "abstain")
	props := map[string]any{
		"pick":   enumField(picks),
		"reason": map[string]any{"type": "string", "maxLength": 200, "description": "one line why (advisory; this seat never decides — S12.4 utility row)"},
	}
	return objectSchema(props, []string{"pick", "reason"})
}

// SpotCheckSchema is the S06.7(a) advisory coverage spot-check duty schema:
// {uncovered: the AC ids/labels the plan appears not to cover, abstain}.
// Advisory-only, never a gate; abstain or an empty list = no finding (R20).
func SpotCheckSchema() json.RawMessage {
	props := map[string]any{
		"uncovered": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "acceptance-criteria labels the plan appears not to cover (advisory)",
		},
		"abstain": map[string]any{"type": "boolean", "description": "true if coverage cannot be assessed (advisory skip)"},
	}
	return objectSchema(props, []string{"uncovered", "abstain"})
}

// HelpSchema is the utility Help drafting schema (S06.9 13.5 card help). This
// is DRAFTING, not classification (S12.4 utility row: "this seat never
// decides", human-gated downstream) — no forced-label abstain member; the
// duty layer sets Classification=false so no logprobs/abstain assertion runs.
func HelpSchema() json.RawMessage {
	str := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	props := map[string]any{
		"what":      str("what this decision does, non-IT reading level"),
		"wrong":     str("what could go wrong"),
		"recommend": str("what the platform recommends and why"),
	}
	return objectSchema(props, []string{"what", "wrong", "recommend"})
}

// enumField builds a string-enum schema field.
func enumField(values []string) map[string]any {
	return map[string]any{"type": "string", "enum": values}
}

// objectSchema builds a strict object schema (additionalProperties:false so
// the engine constraint is tight — GBNF-expressible).
func objectSchema(props map[string]any, required []string) json.RawMessage {
	b, _ := json.Marshal(map[string]any{
		"type":                 "object",
		"properties":           props,
		"required":             required,
		"additionalProperties": false,
	})
	return b
}
