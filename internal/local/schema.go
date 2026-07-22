package local

import (
	"encoding/json"
	"fmt"
	"strings"
)

// schema.go — the S12.4 duty json_schemas (brief R16–R17; drain F5). These are
// the engine-enforced constraint the client sends as `response_format:
// json_schema`. Free-text-then-constrained is REALIZED (F5): every
// classification schema places a REQUIRED `reason` field FIRST, emitted in
// property ORDER (hand-built JSON — a Go map marshals keys alphabetically,
// which would reorder the grammar), so the model's free-text reasoning is the
// leading token region and the constrained labels follow. Every CLASSIFICATION
// schema also carries an explicit abstain/`unclear` member — a model is never
// schema-forced to fabricate a label (S12.4; duty.go's assertAbstain is the
// belt). b10085's json_schema→GBNF PRESERVES property order — CONFIRMED live
// at the tier-L smoke (F5: the real model emitted `reason` first), so no
// two-call fallback is needed. Drafting duties (utility Help) are exempt from
// the abstain rule (F7 ratified — S12.4 scopes it to classification-shaped
// duties).
//
// The enum VALUES are passed in by the caller (the intake families/tiers/sizes
// are intake's vocabulary; the local package owns the SHAPE, not the intake
// constants — the import wall, R24).

// TriageSchema is the intake-triage duty schema (S06.2 family/stakes/size +
// data-bearing) with `reason` first (F5). `abstain:true` means the request
// cannot be classified — the pipeline then fails closed to high stakes
// (S12.4/S06.2), never a fabricated label. The classifier may only ADD the
// data-bearing flag (S06.2 rule 4, enforced by the pipeline).
func TriageSchema(families, tiers, sizes []string) json.RawMessage {
	return orderedObjectSchema([]prop{
		{"reason", strField("brief free-text reasoning — fill this FIRST, before the labels")},
		{"family", enumField(families)},
		{"stakes", enumField(tiers)},
		{"size", enumField(sizes)},
		{"data_bearing", map[string]any{"type": "boolean", "description": "true if the task rests on a live fact (a P47 data-bearing add, S06.2 rule 4)"}},
		{"abstain", map[string]any{"type": "boolean", "description": "true if the request cannot be classified — the pipeline fails closed to high (S12.4)"}},
	}, []string{"reason", "family", "stakes", "size", "data_bearing", "abstain"})
}

// TieBreakSchema is the S08.8 step-2 tie-break duty schema on the utility alias
// (§8 reading 2): reason first, then {pick ∈ candidate ids ∪ "abstain"} (R21).
func TieBreakSchema(candidateIDs []string) json.RawMessage {
	picks := make([]string, 0, len(candidateIDs)+1)
	picks = append(picks, candidateIDs...)
	picks = append(picks, "abstain")
	return orderedObjectSchema([]prop{
		{"reason", strField("one line why (advisory; this seat never decides — S12.4 utility row)")},
		{"pick", enumField(picks)},
	}, []string{"reason", "pick"})
}

// SpotCheckSchema is the S06.7(a) advisory coverage spot-check duty schema:
// reason first, then {uncovered: the AC labels the plan appears not to cover,
// abstain}. Advisory-only, never a gate (R20).
func SpotCheckSchema() json.RawMessage {
	return orderedObjectSchema([]prop{
		{"reason", strField("brief reasoning — fill this FIRST")},
		{"uncovered", map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "acceptance-criteria labels the plan appears not to cover (advisory)",
		}},
		{"abstain", map[string]any{"type": "boolean", "description": "true if coverage cannot be assessed (advisory skip)"}},
	}, []string{"reason", "uncovered", "abstain"})
}

// ContradictionSchema is the S09.7 contradiction-screen (confirm stage) duty
// schema (R16): reason first, then {contradicts ∈ yes/no/unclear, abstain}.
// High-precision advisory — the caller treats only a confident "yes" as a hit
// and refines the question card; the deterministic same-topic_key detection is
// never suppressed by it (S09.7). One-stage per OQ4 (workhorse confirm).
func ContradictionSchema() json.RawMessage {
	return orderedObjectSchema([]prop{
		{"reason", strField("brief reasoning — fill this FIRST")},
		{"contradicts", enumField([]string{"yes", "no", "unclear"})},
		{"abstain", map[string]any{"type": "boolean", "description": "true if the pair cannot be assessed (advisory skip)"}},
	}, []string{"reason", "contradicts", "abstain"})
}

// EntailmentSchema is the S07.4 entailment duty schema (R17): reason first,
// then {verdict ∈ supported/unsupported, abstain}. Binary; P(yes) rides the
// verdict token's logprob margin (S12.4 "binary + P(yes) from logprobs"). An
// abstain maps to the unverifiable verdict (empty/undecidable source).
func EntailmentSchema() json.RawMessage {
	return orderedObjectSchema([]prop{
		{"reason", strField("brief reasoning — fill this FIRST")},
		{"verdict", enumField([]string{"supported", "unsupported"})},
		{"abstain", map[string]any{"type": "boolean", "description": "true if the source cannot decide the claim (unverifiable — rotted/empty/paywalled)"}},
	}, []string{"reason", "verdict", "abstain"})
}

// ContractSchema builds a strict object schema requiring the given string
// fields — the DRAFTING output-contract shape (S12.4 utility row): a free-text
// drafting duty (utility Help) is Classification:false (no forced-label
// abstain), but it STILL enforces a json_schema at the engine for its output
// STRUCTURE (the production utility seam sends HelpSchema). The T15 battery's
// output-contract suites use this so a schema-shaped contract (MustContainFields)
// is honored at the engine, never left to free prose (F2b).
func ContractSchema(fields []string) json.RawMessage {
	props := make([]prop, 0, len(fields))
	for _, f := range fields {
		props = append(props, prop{f, strField("the " + f + " field")})
	}
	return orderedObjectSchema(props, fields)
}

// HelpSchema is the utility Help drafting schema (S06.9 13.5 card help).
// DRAFTING, not classification (S12.4 utility row: "this seat never decides",
// human-gated downstream) — no forced-label abstain member (F7 ratified); the
// duty layer sets Classification=false so no logprobs/abstain assertion runs.
func HelpSchema() json.RawMessage {
	return orderedObjectSchema([]prop{
		{"what", strField("what this decision does, non-IT reading level")},
		{"wrong", strField("what could go wrong")},
		{"recommend", strField("what the platform recommends and why")},
	}, []string{"what", "wrong", "recommend"})
}

// LabelField is one classification label the T15 battery drives its generic
// schema from (brief R6): a field name plus its enum vocabulary (empty Enum =
// a free string, not a routing label). The battery owns the case data; the
// local package owns the schema SHAPE (the import wall, R24).
type LabelField struct {
	Name string   `json:"name"`
	Enum []string `json:"enum,omitempty"`
}

// BatterySchema builds a reason-first classification schema for a battery suite
// (R6): a required free-text `reason` first (F5), then each label field, then
// an explicit abstain member (S12.4). Enum fields constrain to their vocabulary;
// empty-enum fields are free strings.
func BatterySchema(fields []LabelField) json.RawMessage {
	props := []prop{{"reason", strField("brief free-text reasoning — fill this FIRST, before the labels")}}
	required := []string{"reason"}
	for _, f := range fields {
		if len(f.Enum) > 0 {
			props = append(props, prop{f.Name, enumField(f.Enum)})
		} else {
			props = append(props, prop{f.Name, strField("the " + f.Name + " label")})
		}
		required = append(required, f.Name)
	}
	props = append(props, prop{"abstain", map[string]any{"type": "boolean", "description": "true if the input cannot be classified confidently — never guess a label (S12.4)"}})
	required = append(required, "abstain")
	return orderedObjectSchema(props, required)
}

// prop is one ordered schema property (name → spec).
type prop struct {
	name string
	spec map[string]any
}

func strField(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

// enumField builds a string-enum schema field.
func enumField(values []string) map[string]any {
	return map[string]any{"type": "string", "enum": values}
}

// orderedObjectSchema builds a strict object schema with properties in the
// GIVEN order (F5: reason first). Hand-built at the top level because a Go map
// marshals keys alphabetically — which would reorder the grammar; each
// property's OWN spec is a map (its internal key order is grammar-irrelevant).
// additionalProperties:false keeps the engine constraint tight (GBNF-expressible).
func orderedObjectSchema(props []prop, required []string) json.RawMessage {
	var pb strings.Builder
	pb.WriteByte('{')
	for i, p := range props {
		if i > 0 {
			pb.WriteByte(',')
		}
		key, _ := json.Marshal(p.name)
		val, _ := json.Marshal(p.spec)
		pb.Write(key)
		pb.WriteByte(':')
		pb.Write(val)
	}
	pb.WriteByte('}')
	req, _ := json.Marshal(required)
	return json.RawMessage(fmt.Sprintf(`{"type":"object","properties":%s,"required":%s,"additionalProperties":false}`, pb.String(), req))
}
