package worker

import (
	"fmt"
	"regexp"
	"strings"
)

// lint.go — station 1 of the S08.6 validation battery (deterministic):
// frontmatter schema validation, name/selector conventions, unresolved
// tool/skill references, the persona-length warning (⚙
// workers.persona_lines_max), the guardrail-field structural lint (Spec
// S08.2 — a REJECT, never a warning), and the instruction-pattern screen
// over body and skill files (Spec S08.10, P-T14-2: worker definitions are
// a prompt-injection carrier class).

// Finding codes.
const (
	// FindingSchema: frontmatter outside the closed schema or wrong shape.
	FindingSchema = "schema"
	// FindingGuardrail: a guardrail-class field in a definition file — the
	// S08.2 structural reject.
	FindingGuardrail = "guardrail-field"
	// FindingScreen: an instruction-pattern screen hit (Spec S08.6 station
	// 1; S08.10).
	FindingScreen = "instruction-pattern"
	// FindingReference: an unresolved tool/skill/knowledge reference.
	FindingReference = "unresolved-reference"
	// FindingConvention: name/selector convention violation.
	FindingConvention = "convention"
	// FindingPersona: persona lines beyond the ⚙ cap (a warning — the
	// persona budget is a tone lever, Spec S08.5).
	FindingPersona = "persona-length"
	// FindingDialect: a kind=automation body failing the dialect parse
	// (Spec S08.9).
	FindingDialect = "dialect"
)

// Finding is one lint/screen observation.
type Finding struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// StationResult is one battery station's outcome. Errors reject; warnings
// ride the approval card (Spec S08.6).
type StationResult struct {
	Errors   []Finding `json:"errors,omitempty"`
	Warnings []Finding `json:"warnings,omitempty"`
}

// Green reports a pass (no errors; warnings allowed).
func (r StationResult) Green() bool { return len(r.Errors) == 0 }

// guardrailExact are normalized (lowercased, "-"/"_" stripped) key names
// that assert enforcement state — the S08.2 guardrail split enumeration
// plus the engine guardrail-class vocabulary a vendor strips at trust
// boundaries (R15 §2.1: hooks, mcpServers, permissionMode) and the S03.5
// config channels (env, settings). Conservative toward reject: this is the
// enforcement channel, and fail-closed is the posture.
var guardrailExact = map[string]bool{
	"granted": true, "grantedtools": true, "grants": true,
	"permission": true, "permissions": true, "permissionmap": true, "permissionmode": true,
	"confinement": true, "confinementclass": true, "class": true,
	"egress": true, "egressclass": true, "egresshosts": true,
	"budget": true, "budgets": true, "ceiling": true, "ceilings": true, "costcap": true,
	"gatepolicy": true, "gates": true, "gatedtools": true,
	"firstn": true, "firstnremaining": true,
	"schedule": true, "scheduleattachable": true,
	"hooks": true, "mcpservers": true,
	"allowedtools": true, "disallowedtools": true,
	"credentials": true, "secrets": true, "apikey": true, "apikeys": true,
	"env": true, "settings": true, "sandbox": true, "network": true,
}

// guardrailPrefixes catch spelled variants of the same families.
var guardrailPrefixes = []string{
	"budget", "ceiling", "egress", "permission", "confinement",
	"grant", "gate", "hook", "mcp", "schedule", "credential", "secret", "sandbox",
}

// IsGuardrailField reports whether a frontmatter key names guardrail-class
// enforcement state (Spec S08.2). Matching is on the normalized whole key
// (never substrings), so legitimate schema keys like task_classes do not
// trip it.
func IsGuardrailField(key string) bool {
	n := strings.ToLower(key)
	n = strings.ReplaceAll(n, "-", "")
	n = strings.ReplaceAll(n, "_", "")
	if guardrailExact[n] {
		return true
	}
	for _, p := range guardrailPrefixes {
		if strings.HasPrefix(n, p) {
			return true
		}
	}
	return false
}

// classifyUnknownKey files an off-schema key as either the S08.2
// structural reject or a plain schema finding.
func classifyUnknownKey(path string, addf func(code, msg string)) {
	leaf := path
	if i := strings.LastIndexByte(path, '.'); i >= 0 {
		leaf = path[i+1:]
	}
	if IsGuardrailField(leaf) {
		addf(FindingGuardrail, fmt.Sprintf("%s: guardrail-class field in a definition file — enforcement state lives exclusively in control-plane rows (S08.2)", path))
		return
	}
	addf(FindingSchema, fmt.Sprintf("%s: unknown key (the template schema is closed)", path))
}

// InstructionPatterns is the conservative station-1 screen lexicon over
// body and skill content (Spec S08.6 station 1; S08.10 P-T14-2 —
// definitions are untrusted content). Deliberately small and
// high-precision: the screen is one mitigation among four (static-only
// skills, review-as-diff, and the guardrail split are the others); a hit
// is a hard error the author resolves by editing the definition. Exported
// like verify.PlaceholderMarkers so the operator can see exactly what is
// screened.
var InstructionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bignore\s+(all\s+|any\s+|your\s+)?(previous|prior|earlier|system)\s+(instructions|prompts?|messages|rules)`),
	regexp.MustCompile(`(?i)\bdisregard\s+(all\s+|your\s+)?(previous|prior|earlier|system)\s+(instructions|prompts?|messages|rules)`),
	regexp.MustCompile(`(?i)\byou\s+are\s+no\s+longer\b`),
	regexp.MustCompile(`(?i)\breveal\b.{0,40}\b(system\s+prompt|hidden\s+instructions)`),
	regexp.MustCompile(`(?i)\bdo\s+not\s+(tell|inform|alert|notify)\s+the\s+(user|operator|requester|human)`),
	regexp.MustCompile(`(?i)\bhide\s+(this|these|the\s+following)\s+from\b`),
	regexp.MustCompile(`(?i)\bexfiltrat`),
	regexp.MustCompile(`(?i)curl\s+[^\n|]*\|\s*(ba)?sh\b`),
	regexp.MustCompile(`(?i)wget\s+[^\n|]*\|\s*(ba)?sh\b`),
	regexp.MustCompile(`(?i)base64\s+(-d|--decode)\s*[^\n]*\|`),
	regexp.MustCompile(`(?i)ANTHROPIC_API_KEY|CLAUDE_CONFIG_DIR`),
	regexp.MustCompile(`~/\.claude\b|\$HOME/\.claude\b`),
}

// ScreenInstructions runs the pattern screen over one text.
func ScreenInstructions(label, text string) []Finding {
	var out []Finding
	for _, re := range InstructionPatterns {
		if loc := re.FindString(text); loc != "" {
			out = append(out, Finding{
				Code:    FindingScreen,
				Message: fmt.Sprintf("%s: instruction-pattern screen hit %q (S08.10 P-T14-2)", label, excerpt(loc, 80)),
			})
		}
	}
	return out
}

// guardrailLineRe matches a "key: …" line so overlay prose can be screened
// for guardrail-shaped fields without false-positives on ordinary text.
var guardrailLineRe = regexp.MustCompile(`(?m)^\s*([A-Za-z0-9_-]+)\s*:`)

// ScreenOverlayContent screens overlay lesson content: overlays carry ZERO
// guardrail fields (Spec S08.4 — "a lesson can say 'prefer British
// spelling', never 'skip the review gate'"; the guardrail split makes the
// latter structurally homeless) and are instruction-bearing content under
// the same S08.10 screen. The S09 write gate refuses worker-overlay writes
// entirely at v0 (Spec S09.4); this is the v1-activation write-time check
// AND the compile-time belt (compile.go refuses steered overlay items).
func ScreenOverlayContent(label, content string) []Finding {
	var out []Finding
	for _, m := range guardrailLineRe.FindAllStringSubmatch(content, -1) {
		if IsGuardrailField(m[1]) {
			out = append(out, Finding{
				Code:    FindingGuardrail,
				Message: fmt.Sprintf("%s: guardrail-shaped field %q in overlay content (S08.4: overlays carry zero guardrail fields)", label, m[1]),
			})
		}
	}
	out = append(out, ScreenInstructions(label, content)...)
	return out
}

// nameRe is the template/skill name convention: short, lowercase,
// dash-separated (station-1 "name/selector conventions", Spec S08.6).
var nameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$`)

// LintSettings is the station's view of the settings registry.
type LintSettings interface {
	Int(key string) (int64, error)
}

const keyPersonaLinesMax = "workers.persona_lines_max"

// SkillResolver resolves a skill ref to its loaded skill (skill.go); the
// store's skill dir is the production resolver.
type SkillResolver interface {
	ResolveSkill(name string) (Skill, error)
}

// LintTemplate runs station 1 over a parsed definition, its parse
// findings, and its resolved skills (Spec S08.6 station 1). It is
// deterministic and free: schema + conventions + references + guardrail
// lint + instruction-pattern screen + the ⚙ persona warning.
func LintTemplate(def Definition, parseFindings []Finding, skills SkillResolver, settings LintSettings) (StationResult, error) {
	var res StationResult
	for _, f := range parseFindings {
		res.Errors = append(res.Errors, f)
	}
	addErr := func(code, msg string) { res.Errors = append(res.Errors, Finding{Code: code, Message: msg}) }
	addWarn := func(code, msg string) { res.Warnings = append(res.Warnings, Finding{Code: code, Message: msg}) }

	// Identity/conventions.
	if def.Name == "" {
		addErr(FindingConvention, "name: required")
	} else if !nameRe.MatchString(def.Name) {
		addErr(FindingConvention, fmt.Sprintf("name %q: must be lowercase dash-separated (≤ 64 chars)", def.Name))
	}
	if strings.TrimSpace(def.Description) == "" {
		addErr(FindingConvention, "description: required — it is the routing selector text (S08.1)")
	}
	switch def.Kind {
	case KindAgentic, KindAutomation:
	case "":
		addErr(FindingSchema, "kind: required (agentic | automation)")
	default:
		addErr(FindingSchema, fmt.Sprintf("kind %q: must be agentic or automation", def.Kind))
	}
	if def.Domain == "" {
		addErr(FindingSchema, "domain: required — the domains row drives 7.6 marking (S08.1, S08.7)")
	}
	if def.Kind == KindAgentic && def.Body == "" {
		addErr(FindingSchema, "body: an agentic template needs a prompt body (S08.1)")
	}
	if def.Profile.ModelPin != "" && strings.TrimSpace(def.Profile.ModelPinReason) == "" {
		addErr(FindingConvention, "profile.model_pin: a concrete model pin needs a recorded reason (S08.1; 7.3 flags the pin, S08.10)")
	}

	// Persona budget (⚙ workers.persona_lines_max): warn beyond the cap —
	// persona prose is a tone lever only (Spec S08.5).
	if settings == nil {
		return res, fmt.Errorf("worker: lint without a settings registry (⚙ discipline, S01.10)")
	}
	personaMax, err := settings.Int(keyPersonaLinesMax)
	if err != nil {
		return res, fmt.Errorf("worker: read ⚙ %s: %w", keyPersonaLinesMax, err)
	}
	if int64(len(def.Persona)) > personaMax {
		addWarn(FindingPersona, fmt.Sprintf("persona: %d lines exceed the ⚙ cap of %d — long personas measured as accuracy harm (S08.5)", len(def.Persona), personaMax))
	}

	// References: tools against the known vocabulary, skills against the
	// store (errors — structural equipment); knowledge topic keys resolve
	// at injection time by conservative selector match, so an unmatched
	// key is a warning surfaced on the card, not a structural break.
	if def.Kind == KindAgentic {
		for _, tool := range def.Equipment.Tools {
			if !KnownTools[tool] {
				addErr(FindingReference, fmt.Sprintf("equipment.tools: unknown tool %q (v0 vocabulary: %s)", tool, strings.Join(knownToolNames(), ", ")))
			}
		}
	}
	for _, ref := range def.Equipment.Skills {
		if skills == nil {
			addErr(FindingReference, fmt.Sprintf("equipment.skills: %q cannot resolve (no skill store)", ref))
			continue
		}
		sk, err := skills.ResolveSkill(ref)
		if err != nil {
			addErr(FindingReference, fmt.Sprintf("equipment.skills: %q: %v", ref, err))
			continue
		}
		// Skill files are screened like the body: same carrier class
		// (Spec S08.10; static-only packaging is skill.go's).
		res.Errors = append(res.Errors, sk.Findings...)
		res.Errors = append(res.Errors, ScreenInstructions("skill "+ref, sk.Body)...)
	}
	for _, k := range def.Equipment.Knowledge {
		if strings.TrimSpace(k) == "" {
			addErr(FindingReference, "equipment.knowledge: empty topic key")
			continue
		}
		addWarn(FindingReference, fmt.Sprintf("equipment.knowledge: %q resolves at injection time — verify an L2 entry exists for it", k))
	}

	// Instruction-pattern screen over the body (kind=agentic; the
	// automation body is a dialect document validated by its own strict
	// parser — battery.go).
	if def.Kind == KindAgentic {
		res.Errors = append(res.Errors, ScreenInstructions("body", def.Body)...)
	}
	return res, nil
}

func excerpt(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
