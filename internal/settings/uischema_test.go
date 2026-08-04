package settings_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
)

// uischema_test.go — the S15.9 UISchema emitter (P3-B6-3A R2).
//
// The load-bearing property is COMPLETENESS AGAINST THE INDEX: a key declared
// without a control would be a setting the generated UI cannot show, which is
// exactly the drift S01.10's one-artifact-set mandate exists to prevent. It is
// pinned against Decls() rather than against a golden, so adding a key to
// index.go later fails the build unless the emitter covers it.

// uiNode is the emitted document, decoded for structural assertions.
type uiNode struct {
	Type     string          `json:"type"`
	Label    string          `json:"label"`
	Scope    string          `json:"scope"`
	Elements []uiNode        `json:"elements"`
	Options  map[string]any  `json:"options"`
	Rule     json.RawMessage `json:"rule"`
}

func emitUI(t *testing.T, reg *settings.Registry) (uiNode, []byte) {
	t.Helper()
	raw, err := reg.UISchema()
	if err != nil {
		t.Fatalf("UISchema: %v", err)
	}
	var root uiNode
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("UISchema is not valid JSON: %v", err)
	}
	return root, raw
}

// controls walks the tree and returns every Control in document order.
func controls(n uiNode) []uiNode {
	var out []uiNode
	if n.Type == "Control" {
		return []uiNode{n}
	}
	for _, e := range n.Elements {
		out = append(out, controls(e)...)
	}
	return out
}

// TestUISchemaCoversEveryDeclaredKeyExactlyOnce is R2's completeness pin: every
// one of the S18.5 keys has exactly one Control, and no Control names a key the
// index does not declare.
func TestUISchemaCoversEveryDeclaredKeyExactlyOnce(t *testing.T) {
	reg := settings.New()
	root, _ := emitUI(t, reg)
	if root.Type != "Categorization" {
		t.Fatalf("root type = %q, want Categorization (FC-v1 §4)", root.Type)
	}

	seen := map[string]int{}
	for _, c := range controls(root) {
		seen[scopeToKey(c.Scope)]++
	}
	decls := reg.Decls()
	for _, d := range decls {
		switch seen[d.Key] {
		case 1:
		case 0:
			t.Errorf("declared key %q has NO Control — the generated settings UI could not show it (S15.9)", d.Key)
		default:
			t.Errorf("declared key %q has %d Controls, want exactly 1", d.Key, seen[d.Key])
		}
	}
	declared := map[string]bool{}
	for _, d := range decls {
		declared[d.Key] = true
	}
	for key := range seen {
		if !declared[key] {
			t.Errorf("UISchema controls %q, which the index does not declare", key)
		}
	}
	if len(seen) != len(decls) {
		t.Errorf("UISchema controls %d distinct keys, want %d (S18.5)", len(seen), len(decls))
	}
}

// scopeToKey inverts controlScope: #/properties/a/properties/b → a.b.
func scopeToKey(scope string) string {
	return strings.ReplaceAll(strings.TrimPrefix(scope, "#/properties/"), "/properties/", ".")
}

// TestUISchemaScopesResolveAgainstTheSchema walks every Control's scope as a
// JSON Pointer into the emitted JSON Schema. A scope that does not resolve is a
// control bound to nothing — JSON Forms would render it blank, and the two
// emitters would have silently drifted apart.
func TestUISchemaScopesResolveAgainstTheSchema(t *testing.T) {
	reg := settings.New()
	rawSchema, err := reg.JSONSchema()
	if err != nil {
		t.Fatalf("JSONSchema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(rawSchema, &schema); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	root, _ := emitUI(t, reg)
	walked := 0
	for _, c := range controls(root) {
		leaf, ok := resolvePointer(schema, c.Scope)
		if !ok {
			t.Errorf("Control scope %q does not resolve in the emitted JSON Schema", c.Scope)
			continue
		}
		walked++
		// The resolved node must be a LEAF (a declared setting), not a group
		// node: a control pointed at a group would edit an object.
		if _, isGroup := leaf["properties"]; isGroup {
			t.Errorf("Control scope %q resolves to a group node, not a setting", c.Scope)
		}
		if _, ok := leaf["x-sinet"]; !ok {
			t.Errorf("Control scope %q resolves to a node with no x-sinet — not a declared setting", c.Scope)
		}
	}
	if walked == 0 {
		t.Fatal("the scope-resolution walk checked nothing — it would pass vacuously")
	}
	// Non-tautology probe: a scope that does NOT exist must fail to resolve.
	if _, ok := resolvePointer(schema, "#/properties/shell/properties/no_such_key"); ok {
		t.Fatal("the resolver accepts a nonexistent scope — the walk proves nothing")
	}
}

// resolvePointer walks a #/properties/... scope into the schema document.
func resolvePointer(schema map[string]any, scope string) (map[string]any, bool) {
	node := schema
	for _, seg := range strings.Split(strings.TrimPrefix(scope, "#/"), "/") {
		next, ok := node[seg].(map[string]any)
		if !ok {
			return nil, false
		}
		node = next
	}
	return node, true
}

// TestUISchemaIsDeterministic pins byte-identical output across emissions. The
// artifact is generated on every read, so an unstable one would make the
// settings response churn for no reason and would make any golden useless.
func TestUISchemaIsDeterministic(t *testing.T) {
	reg := settings.New()
	_, a := emitUI(t, reg)
	_, b := emitUI(t, reg)
	if string(a) != string(b) {
		t.Fatal("two UISchema emissions differ — the emitter iterates something unordered")
	}
	// A second registry over the same index must agree too: the order comes
	// from the declarations, not from an instance.
	_, c := emitUI(t, settings.New())
	if string(a) != string(c) {
		t.Fatal("two registries emit different UISchemas from one index")
	}
}

// TestUISchemaEmitsNoFabricatedRule is the recorded honest-absence check (R2):
// v0 emits NO SHOW/HIDE rule, because no registry datum says a setting is only
// meaningful under a condition on another. The badges (restart/auto/per-user/
// dormant) are FACTS a person reads, not visibility conditions.
func TestUISchemaEmitsNoFabricatedRule(t *testing.T) {
	reg := settings.New()
	root, raw := emitUI(t, reg)
	if strings.Contains(string(raw), `"rule"`) {
		t.Fatal(`the UISchema carries a "rule" — v0 fabricates none (R2: no ratified fact backs a conditional-visibility condition)`)
	}
	var walk func(uiNode)
	walk = func(n uiNode) {
		if len(n.Rule) != 0 {
			t.Fatalf("element %q (%s) carries a rule", n.Label, n.Type)
		}
		for _, e := range n.Elements {
			walk(e)
		}
	}
	walk(root)
	// Non-tautology probe: the scan must SEE a rule when one is present.
	probe := `{"type":"Control","rule":{"effect":"HIDE"}}`
	if !strings.Contains(probe, `"rule"`) {
		t.Fatal("the no-rule scan cannot detect its own probe — it would pass vacuously")
	}
	var probed uiNode
	if err := json.Unmarshal([]byte(probe), &probed); err != nil || len(probed.Rule) == 0 {
		t.Fatal("the no-rule decoder does not see a rule it is given — the walk proves nothing")
	}
}

// TestUISchemaTabsAreTheS18Domains pins the Categorization to the domain
// structure S18.2 indexes by, and pins each tab's label to the title the SCHEMA
// emitter gives the same domain — so the two artifacts name one tab one way.
func TestUISchemaTabsAreTheS18Domains(t *testing.T) {
	reg := settings.New()
	root, _ := emitUI(t, reg)
	domains := reg.Domains()

	var labels []string
	for _, cat := range root.Elements {
		if cat.Type != "Category" {
			t.Fatalf("Categorization holds a %q, want Category only", cat.Type)
		}
		labels = append(labels, cat.Label)
	}
	if len(labels) != len(domains) {
		t.Fatalf("UISchema has %d Categories, want %d domains (S18.5)", len(labels), len(domains))
	}

	// The label is "<domain> (<owning section>)", the same string schema.go
	// writes as the domain node's title.
	rawSchema, err := reg.JSONSchema()
	if err != nil {
		t.Fatalf("JSONSchema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(rawSchema, &schema); err != nil {
		t.Fatal(err)
	}
	props, _ := schema["properties"].(map[string]any)
	byLabel := map[string]bool{}
	for _, l := range labels {
		byLabel[l] = true
	}
	for _, dom := range domains {
		node, ok := props[dom].(map[string]any)
		if !ok {
			t.Errorf("schema has no node for domain %q", dom)
			continue
		}
		title, _ := node["title"].(string)
		if title == "" {
			t.Errorf("schema domain node %q carries no title", dom)
			continue
		}
		if !byLabel[title] {
			t.Errorf("no UISchema Category is labelled %q — the two emitters name domain %q differently", title, dom)
		}
	}
}

// TestEveryDomainHasOneOwningSection pins the assumption the tab label rests
// on (S18.4 R10: no domain prefix straddles two owners). If a later key broke
// it, the Category label would silently name one of two owners.
func TestEveryDomainHasOneOwningSection(t *testing.T) {
	owner := map[string]string{}
	for _, d := range settings.New().Decls() {
		if prev, ok := owner[d.Domain()]; ok && prev != d.Section {
			t.Errorf("domain %q is owned by both %s and %s (S18.4 R10 says one)", d.Domain(), prev, d.Section)
		}
		owner[d.Domain()] = d.Section
	}
	if len(owner) != 33 {
		t.Errorf("saw %d domains, want 33 (S18.5)", len(owner))
	}
}

// TestUISchemaGroupsOnlyWhereTheIndexDeclaresASubNamespace: a Group appears
// exactly where two or more declared keys share a parent path, and never
// around one lone key.
func TestUISchemaGroupsOnlyWhereTheIndexDeclaresASubNamespace(t *testing.T) {
	reg := settings.New()
	shared := map[string]int{}
	for _, d := range reg.Decls() {
		segs := strings.Split(d.Key, ".")
		if len(segs) < 3 {
			continue
		}
		shared[strings.Join(segs[:len(segs)-1], ".")]++
	}

	root, _ := emitUI(t, reg)
	groups := 0
	for _, cat := range root.Elements {
		for _, el := range cat.Elements {
			if el.Type != "Group" {
				continue
			}
			groups++
			if len(el.Elements) < 2 {
				t.Errorf("Group %q holds %d controls — a group of one is a box around nothing", el.Label, len(el.Elements))
			}
			for _, c := range el.Elements {
				if c.Type != "Control" {
					t.Errorf("Group %q holds a %q; only Controls nest here", el.Label, c.Type)
				}
			}
		}
	}
	want := 0
	for _, n := range shared {
		if n >= 2 {
			want++
		}
	}
	if groups != want {
		t.Errorf("UISchema emits %d Groups, want %d (one per shared sub-namespace in the index)", groups, want)
	}
	if want == 0 {
		t.Fatal("the index declares no shared sub-namespace — this test would prove nothing")
	}
}

// TestUISchemaOptionsAgreeWithSchema is the one-truth-two-readers pin: every
// badge a Control carries says the same thing the schema leaf's x-sinet says,
// for every key, in both directions.
func TestUISchemaOptionsAgreeWithSchema(t *testing.T) {
	reg := settings.New()
	rawSchema, err := reg.JSONSchema()
	if err != nil {
		t.Fatalf("JSONSchema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(rawSchema, &schema); err != nil {
		t.Fatal(err)
	}
	root, _ := emitUI(t, reg)
	badges := []string{"unit", "auto", "perUser", "restartRequired", "dormant"}

	restartSeen, autoSeen, perUserSeen := 0, 0, 0
	for _, c := range controls(root) {
		leaf, ok := resolvePointer(schema, c.Scope)
		if !ok {
			t.Errorf("scope %q does not resolve", c.Scope)
			continue
		}
		x, _ := leaf["x-sinet"].(map[string]any)
		for _, b := range badges {
			want, hasWant := x[b]
			got, hasGot := c.Options[b]
			if hasWant != hasGot {
				t.Errorf("%s: badge %q present in x-sinet=%v but in options=%v", scopeToKey(c.Scope), b, hasWant, hasGot)
				continue
			}
			if hasWant && want != got {
				t.Errorf("%s: badge %q is %v in x-sinet and %v in options", scopeToKey(c.Scope), b, want, got)
			}
		}
		if c.Options["restartRequired"] == true {
			restartSeen++
		}
		if c.Options["auto"] == true {
			autoSeen++
		}
		if c.Options["perUser"] == true {
			perUserSeen++
		}
	}
	// The badges are non-empty in fact, so an emitter that dropped options
	// entirely could not pass by agreeing about nothing.
	if restartSeen != 2 {
		t.Errorf("restart-required badges = %d, want 2 (shell.inhibit_delay_max, shell.journal_max_use)", restartSeen)
	}
	if autoSeen != 5 {
		t.Errorf("auto badges = %d, want 5 (G1 rider 1 auto-adjustable keys)", autoSeen)
	}
	if perUserSeen == 0 {
		t.Error("no per-user badge was emitted, but per-user keys are declared")
	}
}
