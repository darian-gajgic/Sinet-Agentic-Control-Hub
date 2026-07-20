package worker

import (
	"fmt"
	"sort"
	"strings"
)

// template.go — the Sinet template file format (Spec S08.1): markdown +
// YAML frontmatter carrying the BEHAVIORAL definition only. The file may
// request equipment; it can never carry enforcement state — a
// guardrail-class field here is a structural lint reject (Spec S08.2,
// S08.6 station 1; lint.go).

// SelectorSet is the S08.1 identity/purpose selector block: deterministic
// rule inputs for routing, never agent-supplied lookups.
type SelectorSet struct {
	Family      string   `json:"family,omitempty"`
	TaskClasses []string `json:"task_classes,omitempty"`
	Triggers    []string `json:"triggers,omitempty"`
}

// ExecutionProfile is the S08.1 execution profile: duty class + effort
// floor, lane-agnostic; a concrete model pin only with a recorded reason —
// that pin is what 7.3 flags (Spec S08.10).
type ExecutionProfile struct {
	Duty           string `json:"duty,omitempty"`
	EffortFloor    string `json:"effort_floor,omitempty"`
	ModelPin       string `json:"model_pin,omitempty"`
	ModelPinReason string `json:"model_pin_reason,omitempty"`
}

// Equipment is the S08.1 REQUESTED equipment block: tool list, skill refs,
// knowledge selectors (L2 topic keys resolved by the injection manifest at
// run time), connector/MCP NAMES — broker-resolved references, never
// secrets (D2).
type Equipment struct {
	Tools      []string `json:"tools,omitempty"`
	Skills     []string `json:"skills,omitempty"`
	Knowledge  []string `json:"knowledge,omitempty"`
	Connectors []string `json:"connectors,omitempty"`
}

// EvalHooks are the S08.1 eval hook refs.
type EvalHooks struct {
	GoldenSetRef     string `json:"golden_set_ref,omitempty"`
	PlantedDefectRef string `json:"planted_defect_ref,omitempty"`
}

// Definition is a parsed template file. Persona lines are STRUCTURED
// frontmatter (persona:) rather than free prose so the ⚙
// workers.persona_lines_max cap is mechanically checkable (Spec S08.1,
// S08.5: "at most ⚙ persona_lines_max tone/behavior lines").
type Definition struct {
	Name        string
	Description string
	Kind        Kind
	Domain      string
	Selectors   SelectorSet
	Profile     ExecutionProfile
	Equipment   Equipment
	Eval        EvalHooks
	Persona     []string
	// Body is the prompt body (kind=agentic: procedural instructions —
	// objective template, method, boundaries, output contract, escalation
	// triggers) or the automation workflow document (kind=automation,
	// Spec S08.9 dialect).
	Body string
}

// templateSchema is the closed frontmatter schema: top-level keys → the
// nested keys each block accepts (nil = scalar or list leaf). Closed
// schema is part of the fail-closed posture: unknown keys are findings,
// and guardrail-named keys among them are structural rejects (lint.go).
var templateSchema = map[string]map[string]bool{
	"name":        nil,
	"description": nil,
	"kind":        nil,
	"domain":      nil,
	"selectors":   {"family": true, "task_classes": true, "triggers": true},
	"profile":     {"duty": true, "effort_floor": true, "model_pin": true, "model_pin_reason": true},
	"equipment":   {"tools": true, "skills": true, "knowledge": true, "connectors": true},
	"eval":        {"golden_set_ref": true, "planted_defect_ref": true},
	"persona":     nil,
}

// ParseTemplate parses a template document. Structural failures (no
// frontmatter, malformed YAML subset) return an error; schema-level
// problems (unknown keys, wrong shapes, guardrail-class fields) return
// FINDINGS so station 1 can report them all at once (lint.go classifies
// them; a guardrail finding is a reject, Spec S08.6 station 1).
func ParseTemplate(src string) (Definition, []Finding, error) {
	var def Definition
	fmLines, body, err := splitFrontmatter(src)
	if err != nil {
		return def, nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	fm, err := parseFrontmatter(fmLines)
	if err != nil {
		return def, nil, fmt.Errorf("%w: frontmatter: %v", ErrInvalid, err)
	}
	def.Body = strings.TrimSpace(body)

	var findings []Finding
	addf := func(code, msg string) { findings = append(findings, Finding{Code: code, Message: msg}) }

	scalar := func(key string) string {
		v, ok := fm.get(key)
		if !ok {
			return ""
		}
		if !v.IsScalar {
			addf(FindingSchema, fmt.Sprintf("%s: expected a scalar", key))
			return ""
		}
		return v.Scalar
	}
	list := func(m *fmMap, path, key string) []string {
		v, ok := m.get(key)
		if !ok {
			return nil
		}
		switch {
		case v.IsList:
			return v.List
		case v.IsScalar && v.Scalar != "":
			return []string{v.Scalar}
		default:
			addf(FindingSchema, fmt.Sprintf("%s%s: expected a list", path, key))
			return nil
		}
	}
	nested := func(key string) *fmMap {
		v, ok := fm.get(key)
		if !ok {
			return nil
		}
		if v.Map == nil {
			addf(FindingSchema, fmt.Sprintf("%s: expected a block of keys", key))
			return nil
		}
		return v.Map
	}

	// Unknown / guardrail keys over the whole tree (Spec S08.2, S08.6
	// station 1). The walk covers every key path, so a guardrail-class
	// field can not hide in a nested block.
	for _, path := range fm.keyPaths("") {
		topKey, subKey, hasSub := strings.Cut(path, ".")
		sub, known := templateSchema[topKey]
		switch {
		case !known:
			classifyUnknownKey(path, addf)
		case hasSub && (sub == nil || !sub[subKey]):
			classifyUnknownKey(path, addf)
		}
	}

	def.Name = scalar("name")
	def.Description = scalar("description")
	def.Kind = Kind(scalar("kind"))
	def.Domain = scalar("domain")
	if v, ok := fm.get("persona"); ok {
		switch {
		case v.IsList:
			def.Persona = v.List
		case v.IsScalar && v.Scalar != "":
			def.Persona = []string{v.Scalar}
		default:
			addf(FindingSchema, "persona: expected a list of lines")
		}
	}
	if m := nested("selectors"); m != nil {
		if v, ok := m.get("family"); ok && v.IsScalar {
			def.Selectors.Family = v.Scalar
		}
		def.Selectors.TaskClasses = list(m, "selectors.", "task_classes")
		def.Selectors.Triggers = list(m, "selectors.", "triggers")
	}
	if m := nested("profile"); m != nil {
		get := func(k string) string {
			if v, ok := m.get(k); ok && v.IsScalar {
				return v.Scalar
			}
			return ""
		}
		def.Profile.Duty = get("duty")
		def.Profile.EffortFloor = get("effort_floor")
		def.Profile.ModelPin = get("model_pin")
		def.Profile.ModelPinReason = get("model_pin_reason")
	}
	if m := nested("equipment"); m != nil {
		def.Equipment.Tools = list(m, "equipment.", "tools")
		def.Equipment.Skills = list(m, "equipment.", "skills")
		def.Equipment.Knowledge = list(m, "equipment.", "knowledge")
		def.Equipment.Connectors = list(m, "equipment.", "connectors")
	}
	if m := nested("eval"); m != nil {
		if v, ok := m.get("golden_set_ref"); ok && v.IsScalar {
			def.Eval.GoldenSetRef = v.Scalar
		}
		if v, ok := m.get("planted_defect_ref"); ok && v.IsScalar {
			def.Eval.PlantedDefectRef = v.Scalar
		}
	}
	return def, findings, nil
}

// RenderTemplate renders a Definition to its canonical file form — the
// byte-stable serialization that is hashed, diffed, and committed (Spec
// S08.3 hash discipline; S08.6 station 4 approval-as-diff). Parse ∘ Render
// is the identity on the schema fields.
func RenderTemplate(def Definition) string {
	var b strings.Builder
	b.WriteString("---\n")
	wScalar := func(k, v string) {
		if v != "" {
			fmt.Fprintf(&b, "%s: %s\n", k, renderScalar(v))
		}
	}
	wList := func(k string, vs []string, indent string) {
		if len(vs) == 0 {
			return
		}
		quoted := make([]string, len(vs))
		for i, v := range vs {
			quoted[i] = renderScalar(v)
		}
		fmt.Fprintf(&b, "%s%s: [%s]\n", indent, k, strings.Join(quoted, ", "))
	}
	wScalar("name", def.Name)
	wScalar("description", def.Description)
	wScalar("kind", string(def.Kind))
	wScalar("domain", def.Domain)
	if def.Selectors.Family != "" || len(def.Selectors.TaskClasses) > 0 || len(def.Selectors.Triggers) > 0 {
		b.WriteString("selectors:\n")
		if def.Selectors.Family != "" {
			fmt.Fprintf(&b, "  family: %s\n", renderScalar(def.Selectors.Family))
		}
		wList("task_classes", def.Selectors.TaskClasses, "  ")
		wList("triggers", def.Selectors.Triggers, "  ")
	}
	if def.Profile != (ExecutionProfile{}) {
		b.WriteString("profile:\n")
		if def.Profile.Duty != "" {
			fmt.Fprintf(&b, "  duty: %s\n", renderScalar(def.Profile.Duty))
		}
		if def.Profile.EffortFloor != "" {
			fmt.Fprintf(&b, "  effort_floor: %s\n", renderScalar(def.Profile.EffortFloor))
		}
		if def.Profile.ModelPin != "" {
			fmt.Fprintf(&b, "  model_pin: %s\n", renderScalar(def.Profile.ModelPin))
		}
		if def.Profile.ModelPinReason != "" {
			fmt.Fprintf(&b, "  model_pin_reason: %s\n", renderScalar(def.Profile.ModelPinReason))
		}
	}
	eq := def.Equipment
	if len(eq.Tools)+len(eq.Skills)+len(eq.Knowledge)+len(eq.Connectors) > 0 {
		b.WriteString("equipment:\n")
		wList("tools", eq.Tools, "  ")
		wList("skills", eq.Skills, "  ")
		wList("knowledge", eq.Knowledge, "  ")
		wList("connectors", eq.Connectors, "  ")
	}
	if def.Eval != (EvalHooks{}) {
		b.WriteString("eval:\n")
		if def.Eval.GoldenSetRef != "" {
			fmt.Fprintf(&b, "  golden_set_ref: %s\n", renderScalar(def.Eval.GoldenSetRef))
		}
		if def.Eval.PlantedDefectRef != "" {
			fmt.Fprintf(&b, "  planted_defect_ref: %s\n", renderScalar(def.Eval.PlantedDefectRef))
		}
	}
	wList("persona", def.Persona, "")
	b.WriteString("---\n")
	if def.Body != "" {
		b.WriteString(def.Body)
		b.WriteString("\n")
	}
	return b.String()
}

// renderScalar quotes a scalar when the plain form would be ambiguous in
// the subset.
func renderScalar(s string) string {
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, "#:[]{}\"'\n") || s != strings.TrimSpace(s) || strings.HasPrefix(s, "- ") {
		return `"` + strings.ReplaceAll(s, `"`, `'`) + `"`
	}
	return s
}

// equipmentEqual reports whether two definitions carry the same equipment
// — one half of the first-N reset rule (Spec S08.4/G3 D3.4: first-N resets
// when the diff touches body or equipment).
func equipmentEqual(a, b Equipment) bool {
	eq := func(x, y []string) bool {
		if len(x) != len(y) {
			return false
		}
		xs, ys := append([]string(nil), x...), append([]string(nil), y...)
		sort.Strings(xs)
		sort.Strings(ys)
		for i := range xs {
			if xs[i] != ys[i] {
				return false
			}
		}
		return true
	}
	return eq(a.Tools, b.Tools) && eq(a.Skills, b.Skills) &&
		eq(a.Knowledge, b.Knowledge) && eq(a.Connectors, b.Connectors)
}
