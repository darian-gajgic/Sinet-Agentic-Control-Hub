package worker_test

import (
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/worker"
)

// The Sinet template file format (Spec S08.1): strict YAML-subset
// frontmatter + markdown body, canonical render, fail-closed parsing.

func TestParseRenderRoundTrip(t *testing.T) {
	def, findings, err := worker.ParseTemplate(agenticSrc)
	if err != nil {
		t.Fatalf("ParseTemplate: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("unexpected findings: %+v", findings)
	}
	if def.Name != "code-reviewer" || def.Kind != worker.KindAgentic || def.Domain != "software" {
		t.Fatalf("identity mismatch: %+v", def)
	}
	if def.Selectors.Family != "read-analyze" || len(def.Selectors.Triggers) != 2 {
		t.Fatalf("selectors mismatch: %+v", def.Selectors)
	}
	if got := def.Equipment.Tools; len(got) != 3 || got[0] != "Read" {
		t.Fatalf("tools mismatch: %v", got)
	}
	if len(def.Persona) != 1 {
		t.Fatalf("persona mismatch: %v", def.Persona)
	}
	if !strings.Contains(def.Body, "file:line anchors") {
		t.Fatalf("body mismatch: %q", def.Body)
	}

	canonical := worker.RenderTemplate(def)
	def2, findings2, err := worker.ParseTemplate(canonical)
	if err != nil {
		t.Fatalf("re-parse canonical: %v", err)
	}
	if len(findings2) != 0 {
		t.Fatalf("canonical findings: %+v", findings2)
	}
	if worker.RenderTemplate(def2) != canonical {
		t.Fatalf("canonical render is not a fixed point")
	}
}

func TestFrontmatterSubsetRejections(t *testing.T) {
	cases := map[string]string{
		"no frontmatter":  "just a body\n",
		"unterminated":    "---\nname: x\n",
		"tab":             "---\nname:\tx\n---\nb",
		"duplicate key":   "---\nname: a\nname: b\n---\nb",
		"flow map":        "---\nname: {a: b}\n---\nb",
		"deep nesting":    "---\nselectors:\n  family:\n    deep: x\n---\nb",
		"bad indentation": "---\nselectors:\n   family: x\n---\nb",
		"invalid key":     "---\nna me: x\n---\nb",
	}
	for name, src := range cases {
		if _, _, err := worker.ParseTemplate(src); err == nil {
			t.Errorf("%s: parse accepted, want reject", name)
		}
	}
}

func TestUnknownKeyIsFindingNotGuardrail(t *testing.T) {
	src := "---\nname: x\ndescription: y\nkind: agentic\ndomain: software\nfrobnicate: z\n---\nbody"
	_, findings, err := worker.ParseTemplate(src)
	if err != nil {
		t.Fatalf("ParseTemplate: %v", err)
	}
	if len(findings) != 1 || findings[0].Code != worker.FindingSchema {
		t.Fatalf("want one schema finding, got %+v", findings)
	}
}

func TestGuardrailFieldClassification(t *testing.T) {
	// Whole-key normalized matching: legitimate schema keys never trip it.
	for _, ok := range []string{"task_classes", "tools", "knowledge", "domain", "kind", "triggers"} {
		if worker.IsGuardrailField(ok) {
			t.Errorf("%q wrongly classified as guardrail field", ok)
		}
	}
	for _, bad := range []string{
		"permissionMode", "permission_map", "confinement", "class", "egress",
		"budget_usd", "ceiling", "first_n", "schedule_attachable", "hooks",
		"mcpServers", "allowed-tools", "granted_tools", "gate_policy", "env",
		"settings", "sandbox", "network", "credentials",
	} {
		if !worker.IsGuardrailField(bad) {
			t.Errorf("%q not classified as guardrail field", bad)
		}
	}
}

func TestGuardrailFieldAnywhereInTreeIsReject(t *testing.T) {
	// Top level and nested block both reject (Spec S08.2: "appearing in
	// any definition file" — a structural lint reject, not a warning).
	for name, src := range map[string]string{
		"top level": "---\nname: x\ndescription: y\nkind: agentic\ndomain: software\npermissionMode: bypassPermissions\n---\nbody",
		"nested":    "---\nname: x\ndescription: y\nkind: agentic\ndomain: software\nprofile:\n  hooks: evil\n---\nbody",
	} {
		_, findings, err := worker.ParseTemplate(src)
		if err != nil {
			t.Fatalf("%s: ParseTemplate: %v", name, err)
		}
		found := false
		for _, f := range findings {
			if f.Code == worker.FindingGuardrail {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: no guardrail finding in %+v", name, findings)
		}
	}
}
