package worker_test

import (
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/worker"
)

// Station 2 (Spec S08.6): the deterministic permission audit against the
// v0 task-class ceiling table + the Rule-of-Two admission check.

func TestAuditWithinCeilingIsClean(t *testing.T) {
	def, _ := parseFor(t, agenticSrc)
	res := worker.AuditPermissions(def, readGrants(), false)
	if !res.Green() {
		t.Fatalf("audit not green: %+v", res)
	}
	if len(res.FlaggedItems()) != 0 {
		t.Fatalf("unexpected flags: %v", res.FlaggedItems())
	}
	if res.CeilingFamily != worker.FamilyReadAnalyze {
		t.Fatalf("ceiling family = %s", res.CeilingFamily)
	}
}

func TestAuditFlagsAboveCeiling(t *testing.T) {
	def, _ := parseFor(t, agenticSrc) // family read-analyze: max C1, no egress, read-only tools
	req := worker.RequestedGrants{
		Tools:  []string{"Read", "Bash"}, // Bash beyond the read-analyze set
		Class:  "C2",                     // above the C1 ceiling
		Egress: worker.EgressRegistries,  // above none
	}
	res := worker.AuditPermissions(def, req, false)
	flags := res.FlaggedItems()
	want := map[string]bool{"tool:Bash": true, "confinement": true, "egress": true}
	if len(flags) != len(want) {
		t.Fatalf("flags = %v, want %v", flags, want)
	}
	for _, fl := range flags {
		if !want[fl] {
			t.Fatalf("unexpected flag %q", fl)
		}
	}
	// Flags do NOT make the audit structurally red (they gate approval
	// acknowledgement, Spec S08.6 station 2).
	if !res.Green() {
		t.Fatalf("flags should not be a structural refusal: %+v", res)
	}
}

func TestAuditUnmatchedFamilyTakesGenericFallback(t *testing.T) {
	def, _ := parseFor(t, agenticSrc)
	def.Domain = "web-research" // no ceiling row → generic fallback (C1, none, read-only)
	res := worker.AuditPermissions(def, readGrants(), false)
	if res.CeilingFamily != "generic-fallback" {
		t.Fatalf("family = %s, want generic-fallback", res.CeilingFamily)
	}
	if !res.Green() {
		t.Fatalf("read-only request under fallback should be green: %+v", res)
	}
}

func TestAuditRuleOfTwoRefusesThreePropertyAgentic(t *testing.T) {
	// An agentic worker with egress (untrusted input + external), a
	// connector credential (sensitive access), and workspace write: all
	// three properties, no standing supervision → static refusal (Spec
	// S11.6).
	def, _ := parseFor(t, agenticSrc)
	def.Selectors.Family = worker.FamilyImplementFix
	def.Equipment.Connectors = []string{"github"}
	req := worker.RequestedGrants{
		Tools: []string{"Read", "Write", "Edit", "Bash"}, Class: "C2",
		Egress: worker.EgressRegistries,
	}
	res := worker.AuditPermissions(def, req, false)
	if res.RuleOfTwoRefusal == "" {
		t.Fatalf("three-property agentic combination admitted: %+v", res.Props)
	}
	if res.Green() {
		t.Fatalf("Rule-of-Two refusal must be structurally red")
	}
}

func TestAuditRuleOfTwoAdmitsSupervisedAutomation(t *testing.T) {
	// A C0 automation holds all three properties (service payloads,
	// broker credential, outward effects) but its outward effects are
	// ALWAYS gated proposals — the standing supervision basis (Spec
	// S08.9, D7/4.2).
	def := worker.Definition{
		Name: "calendar-digest", Description: "d", Kind: worker.KindAutomation,
		Domain:    "chore",
		Selectors: worker.SelectorSet{Family: worker.FamilyConnectorAutomation},
		Equipment: worker.Equipment{Connectors: []string{"calendar"}},
	}
	req := worker.RequestedGrants{
		Tools: []string{"calendar.list", "calendar.post"}, Class: "C0",
		Egress: worker.EgressSingleHost, EgressHosts: []string{"calendar.example.com"},
	}
	res := worker.AuditPermissions(def, req, false)
	if res.RuleOfTwoRefusal != "" {
		t.Fatalf("supervised automation refused: %s", res.RuleOfTwoRefusal)
	}
	if !res.Supervised {
		t.Fatalf("automation supervision basis not derived")
	}
	if len(res.FlaggedItems()) != 0 {
		t.Fatalf("single-service verbs flagged: %v", res.FlaggedItems())
	}
}

func TestAuditAutomationForeignServiceVerbFlagged(t *testing.T) {
	def := worker.Definition{
		Name: "calendar-digest", Description: "d", Kind: worker.KindAutomation,
		Domain:    "chore",
		Selectors: worker.SelectorSet{Family: worker.FamilyConnectorAutomation},
		Equipment: worker.Equipment{Connectors: []string{"calendar"}},
	}
	req := worker.RequestedGrants{
		Tools: []string{"calendar.list", "email.send"}, Class: "C0",
		Egress: worker.EgressSingleHost,
	}
	res := worker.AuditPermissions(def, req, false)
	found := false
	for _, fl := range res.FlaggedItems() {
		if fl == "verb:email.send" {
			found = true
		}
	}
	if !found {
		t.Fatalf("foreign-service verb not flagged: %v", res.FlaggedItems())
	}
}

func TestGuardrailRejectMakesAuditRed(t *testing.T) {
	def, _ := parseFor(t, agenticSrc)
	res := worker.AuditPermissions(def, readGrants(), true)
	if res.Green() {
		t.Fatalf("guardrail-field reject must be structurally red (S08.6 station 2)")
	}
}
