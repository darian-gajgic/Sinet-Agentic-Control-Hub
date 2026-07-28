package settings

import (
	"encoding/json"
	"fmt"
	"strings"
)

// uischema.go — the JSON Forms UISchema companion to JSONSchema() (P3-B6-3A).
//
// Spec S15.9 binds the settings UI to "the registry-emitted JSON Schema +
// UISchema (S01.10)", and S01.10 (b) is why the emission lives HERE rather than
// in the API layer: one artifact set drives validation, the UI and the docs, so
// they cannot drift apart. FC-v1 §4 fixes the vocabulary as JSON Forms' CORE,
// theme-independent elements — Categorization/Category (the tabs),
// Group (sub-grouping), Control (one per setting) — rendered by the Sinet-owned
// renderer set, which is B6-6's. Nothing here renders anything.
//
// The emitter is a SIBLING of schema.go: index.go is byte-unchanged and the
// declarations never move. Both emitters read the same *Decl, and the tests pin
// them to agree leaf-for-leaf, so a key can never appear in one and not the
// other.
//
// v0 EMITS NO RULE, and that is a decision rather than an omission. JSON Forms'
// rule vocabulary (SHOW/HIDE/ENABLE/DISABLE against a condition over another
// value) needs a registry datum that says "this setting is only meaningful when
// that one is set thus". The registry has no such datum: `restartRequired`,
// `auto`, `perUser` and `dormant` are BADGES — facts about a setting that a
// person reads — not visibility conditions, and the S18.2 relational rules
// (S18 clamps) are validated on write by the registry itself, never by hiding a
// control. Fabricating a rule to demonstrate the vocabulary would put a
// condition on the wire that no ratified fact backs. The capability is present
// structurally (the shape is JSON Forms' own, so a later packet with a real
// condition adds a `rule` member and nothing else moves), and this comment plus
// TestUISchemaEmitsNoFabricatedRule are the recorded reason.

// uiElement is one JSON Forms UISchema node. The member set is exactly the core
// vocabulary FC-v1 §4 admits; `omitempty` keeps each node to the members that
// element type actually uses.
type uiElement struct {
	Type     string         `json:"type"`
	Label    string         `json:"label,omitempty"`
	Scope    string         `json:"scope,omitempty"`
	Elements []uiElement    `json:"elements,omitempty"`
	Options  map[string]any `json:"options,omitempty"`
}

// UISchema emits the registry as one JSON Forms UISchema document: a
// Categorization whose Categories are the S18 domains in declared order, each
// holding one Control per declared key of that domain, with a Group wherever
// the key index itself declares a shared sub-namespace.
//
// The emission is deterministic — two calls produce byte-identical output —
// because every ordering comes from the declaration index and no map is
// iterated.
func (r *Registry) UISchema() ([]byte, error) {
	root := uiElement{Type: "Categorization", Elements: []uiElement{}}
	subgroups := r.subgroupCounts()
	catAt := make(map[string]int, len(r.decls))
	grpAt := make(map[string]int, len(subgroups))

	for _, key := range r.order {
		d := r.decls[key]
		segs := strings.Split(d.Key, ".")
		if len(segs) < 2 {
			// Unreachable behind validateDecl's domain-prefix check; kept as a
			// named failure rather than an index panic if that ever loosens.
			return nil, fmt.Errorf("settings: %q has no domain prefix", d.Key)
		}
		domain := segs[0]
		ci, ok := catAt[domain]
		if !ok {
			root.Elements = append(root.Elements, uiElement{
				Type:     "Category",
				Label:    domainTitle(domain, d.Section),
				Elements: []uiElement{},
			})
			ci = len(root.Elements) - 1
			catAt[domain] = ci
		}
		ctrl := uiElement{
			Type:    "Control",
			Scope:   controlScope(segs),
			Label:   d.Title,
			Options: controlOptions(d),
		}
		prefix := strings.Join(segs[:len(segs)-1], ".")
		if subgroups[prefix] < 2 {
			root.Elements[ci].Elements = append(root.Elements[ci].Elements, ctrl)
			continue
		}
		gi, ok := grpAt[prefix]
		if !ok {
			root.Elements[ci].Elements = append(root.Elements[ci].Elements, uiElement{
				Type:     "Group",
				Label:    strings.Join(segs[1:len(segs)-1], "."),
				Elements: []uiElement{},
			})
			gi = len(root.Elements[ci].Elements) - 1
			grpAt[prefix] = gi
		}
		g := &root.Elements[ci].Elements[gi]
		g.Elements = append(g.Elements, ctrl)
	}
	return json.MarshalIndent(root, "", "  ")
}

// domainTitle is the Category label, built exactly as schema.go builds the
// domain node's title so the two artifacts name the same tab the same way. A
// domain has one owning section (S18.4 R10: no domain prefix straddles two
// owners), pinned by TestEveryDomainHasOneOwningSection.
func domainTitle(domain, section string) string { return domain + " (" + section + ")" }

// controlScope is the JSON Pointer from the UISchema Control to its leaf in the
// emitted JSON Schema. Dotted keys are nested objects there (schema.go), so each
// segment is one `properties` hop.
func controlScope(segs []string) string {
	return "#/properties/" + strings.Join(segs, "/properties/")
}

// controlOptions carries the S15.9 badge data onto the Control: the
// restart-required badge ("restart-required settings are badged"), the G1
// rider 1 auto flag, the per-user scope qualifier, the S18.5 dormant note and
// the declared unit.
//
// These are the same five facts schema.go writes into `x-sinet`, from the same
// *Decl — the UISchema carries them because JSON Forms hands a renderer its
// UISchema node's `options` directly, and TestUISchemaOptionsAgreeWithSchema
// pins the two projections to agree for every key, so there is one truth with
// two readers rather than two truths.
func controlOptions(d *Decl) map[string]any {
	o := map[string]any{}
	if d.Unit != "" {
		o["unit"] = d.Unit
	}
	if d.Auto {
		o["auto"] = true
	}
	if d.PerUser {
		o["perUser"] = true
	}
	if d.Restart {
		o["restartRequired"] = true
	}
	if d.Dormant != "" {
		o["dormant"] = d.Dormant
	}
	if len(o) == 0 {
		return nil
	}
	return o
}

// subgroupCounts counts declared keys per full parent path (everything but the
// last segment) for keys below the second level.
//
// A Group is emitted only where TWO OR MORE keys share a parent — the sub-
// namespace is then something the key index itself declares (`local.ttl.*`,
// `memory.injection_budget_tokens.*`), not a threshold this emitter invented. A
// lone deep key (`adapter.claude.cleanup_period_days`) sits directly in its
// Category, because a Group of one is a box drawn around nothing.
func (r *Registry) subgroupCounts() map[string]int {
	out := map[string]int{}
	for _, key := range r.order {
		segs := strings.Split(key, ".")
		if len(segs) < 3 {
			continue
		}
		out[strings.Join(segs[:len(segs)-1], ".")]++
	}
	return out
}
