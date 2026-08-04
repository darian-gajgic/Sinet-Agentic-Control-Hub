package worker

import (
	"fmt"
	"sort"
	"strings"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/sandbox"
)

// ceiling.go — station 2 of the S08.6 battery: the deterministic
// permission audit ("a table, not a model") against the task-class ceiling
// table — 4.4's instrument — plus the Rule-of-Two admission check on the
// resulting class combination (Spec S11.6, via sandbox.AdmitRuleOfTwo).
// Requested grants are diffed against the ceiling row for the template's
// (domain, family); any above-ceiling request becomes a flagged line on
// the approval card, and approval refuses unacknowledged flags
// (ErrAboveCeiling — the ceiling stays a ceiling by default; an explicit,
// audit-trailed approver acknowledgement is the only way past a flag).

// The v0 tool vocabulary. Names are the Sinet-granted tool identifiers the
// S03.5 lowering hands to the engine allowlist; at v0 they coincide with
// the Anthropic-lane names (the one agentic lane wired, CONVENTIONS §10).
const (
	ToolRead  = "Read"
	ToolGrep  = "Grep"
	ToolGlob  = "Glob"
	ToolWrite = "Write"
	ToolEdit  = "Edit"
	ToolBash  = "Bash"
)

// KnownTools is the closed v0 vocabulary station 1 resolves tool refs
// against.
var KnownTools = map[string]bool{
	ToolRead: true, ToolGrep: true, ToolGlob: true,
	ToolWrite: true, ToolEdit: true, ToolBash: true,
}

func knownToolNames() []string {
	out := make([]string, 0, len(KnownTools))
	for t := range KnownTools {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// Task families of the v0 ceiling rows (Spec S08.6 [coordinator-draft];
// further families ship with their domains, S19). Family strings are the
// selector-family vocabulary the S08.8 selector match will consume (B3-3).
const (
	FamilyReadAnalyze         = "read-analyze"
	FamilyImplementFix        = "implement-fix"
	FamilyConnectorAutomation = "connector-automation"
)

// CeilingRow is one task-class ceiling table row: the MAXIMUM tool,
// confinement, and egress powers for a (domain, family) (Spec S08.6
// station 2).
type CeilingRow struct {
	Domain   string
	Family   string
	Tools    []string
	MaxClass sandbox.Class
	Egress   EgressClass
	// ConnectorVerbs marks the automation row: granted "tools" are fixed
	// connector verbs on the one named service, not engine tools (Spec
	// S08.6, S08.9).
	ConnectorVerbs bool
}

// V0CeilingTable is the ratified v0 row set [coordinator-draft], verbatim
// from Spec S08.6 station 2.
var V0CeilingTable = []CeilingRow{
	{Domain: "software", Family: FamilyReadAnalyze,
		Tools: []string{ToolRead, ToolGrep, ToolGlob}, MaxClass: sandbox.C1, Egress: EgressNone},
	{Domain: "software", Family: FamilyImplementFix,
		Tools:    []string{ToolRead, ToolGrep, ToolGlob, ToolWrite, ToolEdit, ToolBash},
		MaxClass: sandbox.C2, Egress: EgressRegistries},
	{Domain: "chore", Family: FamilyConnectorAutomation,
		Tools: nil, MaxClass: sandbox.C0, Egress: EgressSingleHost, ConnectorVerbs: true},
}

// genericFallbackRow is the unmatched-family row: read-only set, C1, no
// egress (Spec S08.6).
var genericFallbackRow = CeilingRow{
	Domain: "", Family: "generic-fallback",
	Tools: []string{ToolRead, ToolGrep, ToolGlob}, MaxClass: sandbox.C1, Egress: EgressNone,
}

// CeilingFor resolves the ceiling row for a (domain, family); an unmatched
// pair takes the generic fallback.
func CeilingFor(domain, family string) CeilingRow {
	for _, row := range V0CeilingTable {
		if row.Domain == domain && row.Family == family {
			return row
		}
	}
	return genericFallbackRow
}

// AuditLine is one requested-vs-ceiling comparison on the approval card's
// powers table (Spec S08.6 station 4).
type AuditLine struct {
	Item      string `json:"item"`
	Requested string `json:"requested"`
	Ceiling   string `json:"ceiling"`
	// Flagged marks an above-ceiling request; approval requires an
	// explicit acknowledgement of every flagged item id.
	Flagged bool `json:"flagged,omitempty"`
}

// AuditResult is station 2's outcome.
type AuditResult struct {
	CeilingFamily string      `json:"ceiling_family"`
	Lines         []AuditLine `json:"lines"`
	// Props is the derived Rule-of-Two property vector; Supervised its
	// standing supervision basis (Spec S11.6).
	Props      sandbox.Props `json:"props"`
	Supervised bool          `json:"supervised"`
	// RuleOfTwoRefusal is set when the admission check statically refuses
	// the combination — a hard error, not a flag (Spec S11.6 "statically
	// refuses").
	RuleOfTwoRefusal string `json:"rule_of_two_refusal,omitempty"`
	// GuardrailReject is set when guardrail-class fields were found in
	// definition files (station 1 detects; station 2 restates the
	// structural reject per Spec S08.6).
	GuardrailReject bool `json:"guardrail_reject,omitempty"`
}

// Green reports a structurally admissible audit (flags allowed — they gate
// approval acknowledgement, not validation).
func (a AuditResult) Green() bool { return a.RuleOfTwoRefusal == "" && !a.GuardrailReject }

// FlaggedItems lists the item ids an approver must acknowledge.
func (a AuditResult) FlaggedItems() []string {
	var out []string
	for _, l := range a.Lines {
		if l.Flagged {
			out = append(out, l.Item)
		}
	}
	return out
}

// AuditPermissions runs station 2 for a definition and its requested
// grants (Spec S08.6). Deterministic — a table, not a model.
func AuditPermissions(def Definition, req RequestedGrants, lintHadGuardrailReject bool) AuditResult {
	row := CeilingFor(def.Domain, def.Selectors.Family)
	res := AuditResult{CeilingFamily: row.Family, GuardrailReject: lintHadGuardrailReject}

	// Tools vs the ceiling set. For the automation row the request must be
	// fixed connector verbs on the one named service (Spec S08.9); for
	// engine rows, any tool outside the row's set is a flagged line.
	if row.ConnectorVerbs {
		service := ""
		if len(def.Equipment.Connectors) == 1 {
			service = def.Equipment.Connectors[0]
		}
		for _, verb := range req.Tools {
			svc, _, ok := strings.Cut(verb, ".")
			line := AuditLine{Item: "verb:" + verb, Requested: verb, Ceiling: "fixed connector verbs on the one named service"}
			if !ok || service == "" || svc != service {
				line.Flagged = true
			}
			res.Lines = append(res.Lines, line)
		}
		if len(def.Equipment.Connectors) != 1 {
			res.Lines = append(res.Lines, AuditLine{
				Item: "connectors", Requested: strings.Join(def.Equipment.Connectors, ", "),
				Ceiling: "exactly one named service (S08.9)", Flagged: true,
			})
		}
	} else {
		allowed := map[string]bool{}
		for _, t := range row.Tools {
			allowed[t] = true
		}
		for _, t := range req.Tools {
			line := AuditLine{Item: "tool:" + t, Requested: t, Ceiling: strings.Join(row.Tools, ", ")}
			if !allowed[t] {
				line.Flagged = true
			}
			res.Lines = append(res.Lines, line)
		}
	}

	// Confinement class vs the ceiling (ladder rank; v0 grants are C0–C2,
	// Spec S11.6).
	reqClass := sandbox.Class(req.Class)
	classLine := AuditLine{Item: "confinement", Requested: req.Class, Ceiling: string(row.MaxClass)}
	if classRank(reqClass) < 0 || classRank(reqClass) > classRank(row.MaxClass) {
		classLine.Flagged = true
	}
	res.Lines = append(res.Lines, classLine)

	// Egress vs the ceiling (reach ranks).
	egressLine := AuditLine{Item: "egress", Requested: string(req.Egress), Ceiling: string(row.Egress)}
	if egressReach(req.Egress) > egressReach(row.Egress) {
		egressLine.Flagged = true
	}
	res.Lines = append(res.Lines, egressLine)

	// Rule-of-Two admission on the resulting combination (Spec S11.6).
	res.Props, res.Supervised = deriveProps(def, req)
	if err := sandbox.AdmitRuleOfTwo(res.Props, res.Supervised); err != nil {
		res.RuleOfTwoRefusal = err.Error()
	}
	return res
}

func classRank(c sandbox.Class) int {
	switch c {
	case sandbox.C0:
		return 0
	case sandbox.C1:
		return 1
	case sandbox.C2:
		return 2
	}
	return -1
}

// deriveProps derives the Rule-of-Two property vector from the declared
// record (Spec S11.6; derivation is a v0-conservative reading, visible on
// the audit result):
//
//   - untrusted input: any egress reach (external content flows in) or a
//     deterministic automation fed service payloads;
//   - sensitive access: any connector — a broker-held credential (D2);
//   - state/external: workspace write (C2), any egress, or an automation
//     (whose purpose is outward effects).
//
// The STANDING supervision basis is the automation posture: every outward
// effect materializes as a gated proposal (Spec S08.9, D7/4.2). First-N
// review and degraded-domain review are temporary states and deliberately
// do NOT count — a three-property agentic worker is refused rather than
// admitted on supervision that later graduates away.
func deriveProps(def Definition, req RequestedGrants) (sandbox.Props, bool) {
	p := sandbox.Props{
		UntrustedInput:  egressReach(req.Egress) > 0 || def.Kind == KindAutomation,
		SensitiveAccess: len(def.Equipment.Connectors) > 0,
		StateOrExternal: req.Class == string(sandbox.C2) || egressReach(req.Egress) > 0 || def.Kind == KindAutomation,
	}
	supervised := def.Kind == KindAutomation
	return p, supervised
}

// String renders one line for cards/tests.
func (l AuditLine) String() string {
	flag := ""
	if l.Flagged {
		flag = "  ⚑ above ceiling"
	}
	return fmt.Sprintf("%-24s requested: %-32s ceiling: %s%s", l.Item, l.Requested, l.Ceiling, flag)
}
