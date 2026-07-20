package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/sandbox"
)

// compile.go — Spec S08.3: at spawn the control plane compiles (template
// version + granted guardrails + the requester's overlay slice + instance
// refs) through S03's engine lowering, so Sinet-compiled config is the
// ONLY config the engine sees. Properties realized here:
//
//   - Fresh every run: CompileInvocation is a pure function callers run
//     per spawn; nothing it produces persists as engine-side state.
//   - Hash-pinned, body + config together: ConfigHash covers the whole
//     compiled unit — template file hash, guardrails, overlay slice,
//     instance refs, and every compiled field — one unit (7.7's audit
//     extended to configuration). No half is runtime-editable.
//   - Sole-controller preserved: the granted toolset is refused if it
//     names the engine's native-spawn family (belt here; the S03.5
//     lowering validates again and the compiled artifact disables native
//     spawning by construction).
//
// The compiled output feeds adapters.StartRequest verbatim: Worker (the
// S03.5 CompiledWorker), Class, and the platform ceilings. Assembly of the
// compiled equipment into the stage brief, and the trace manifest, are
// S05's; model/lane selection is S08.8's (B3-3) — the Model field of the
// StartRequest stays the caller's.

// OverlayItem is one worker-overlay L2 lesson entering the instance
// compile (Spec S08.4). At v0 the slice is structurally empty: the S09
// gate refuses worker-overlay writes and the scope's injection activates
// at v1 (Spec S09.4 activation table) — the machinery is day-one, the
// content arrives with activation.
type OverlayItem struct {
	EntryID string
	Title   string
	Content string
	Version int64
}

// OverlaySource is the S09-machinery seam the compile reads the
// requester's overlay slice through (per user × template). The production
// implementation wraps memory.Store.OverlaySlice at the composition root;
// nil is the dormant v0 default (an empty slice, exactly what the S09
// dormancy yields).
type OverlaySource interface {
	OverlaySlice(ctx context.Context, owner, templateID string) ([]OverlayItem, error)
}

// InstanceRefs are the per-run instance identifiers entering the compile
// and its hash (Spec S08.3). Instances are per-run: L0 scratch + the run
// workspace, expiring with the task (Spec S08.4); the instance's L0 ledger
// section reaches the engine through the S05 stage brief, not through the
// agent definition.
type InstanceRefs struct {
	RunID  string `json:"run_id"`
	TaskID string `json:"task_id,omitempty"`
	Stage  string `json:"stage,omitempty"`
	// PinnedContextPath wires the lane's deterministic re-injection
	// channel (CompiledWorker.SessionStartContextPath; Spec S05.7).
	PinnedContextPath string `json:"pinned_context_path,omitempty"`
}

// CompileInput is one per-invocation compile request. Def and FileSHA256
// come from the pinned version file (the store verifies the on-disk file
// against the row hash before parsing — the S08.3 tamper check);
// Guardrails from the version's control-plane row; Overlay through the
// OverlaySource; Instance from the spawning run.
type CompileInput struct {
	TemplateID string
	VersionID  string
	Def        Definition
	FileSHA256 string
	Guardrails Guardrails
	Overlay    []OverlayItem
	Instance   InstanceRefs
	// Skills are the resolved static skill dirs (skill.go validation
	// already passed at battery time); their content hashes pin the skill
	// half of the unit.
	Skills []CompiledSkillRef
}

// CompiledSkillRef is one resolved skill entering the compiled unit.
type CompiledSkillRef struct {
	Name   string `json:"name"`
	Dir    string `json:"dir"`
	SHA256 string `json:"sha256"`
}

// Compiled is the compiled invocation unit.
type Compiled struct {
	// TemplateID/VersionID identify the compiled definition (the S08.3
	// recorded-on-the-run join: which compiled configuration produced which
	// outcome).
	TemplateID string
	VersionID  string
	// Worker is the S03.5 compile target the adapter lowers.
	Worker adapters.CompiledWorker
	// Class/Egress are the compiled confinement data (Spec S11.6: carried
	// declaratively from the control-plane record into StartRequest.Class
	// and the Confiner's egress policy).
	Class       string
	Egress      EgressClass
	EgressHosts []string
	// Ceilings for StartRequest (3.7 platform ceilings).
	CeilingCostUSD float64
	CeilingSteps   int64
	// SkillDirs are the resolved static skill directories (pointed-at
	// dirs, S03.5 skill channel). Engine-side placement is the S05
	// assembly/B3-3 wiring; the dirs' content hashes are inside
	// ConfigHash, so the compiled unit pins them.
	SkillDirs []string
	// ConfigHash is the S08.3 hash over the whole unit, recorded on the
	// run by the dispatch wiring (B3-3).
	ConfigHash string
}

// nativeSpawnFamily is the engine-native spawn tool family every compiled
// artifact must exclude (G1 rider 2; Spec S03.5). The adapter refuses
// again at lowering; the compile refuses first.
var nativeSpawnFamily = map[string]bool{
	"Task": true, "TaskCreate": true, "TaskGet": true, "TaskList": true,
	"TaskOutput": true, "TaskStop": true, "TaskUpdate": true,
}

// CompileInvocation compiles one invocation (Spec S08.3). Agentic only: a
// kind=automation body never reaches an engine — no model in the loop
// (Spec S08.9; ErrKindMismatch).
func CompileInvocation(in CompileInput) (Compiled, error) {
	if in.Def.Kind != KindAgentic {
		return Compiled{}, fmt.Errorf("%w: compile targets engines; kind=%s does not (S08.9 no model in the loop)", ErrKindMismatch, in.Def.Kind)
	}
	if in.Instance.RunID == "" {
		return Compiled{}, fmt.Errorf("%w: instance refs need a run id (S08.3 per-invocation)", ErrInvalid)
	}
	g := in.Guardrails
	switch sandbox.Class(g.Class) {
	case sandbox.C1, sandbox.C2:
	case sandbox.C0:
		return Compiled{}, fmt.Errorf("%w: C0 is the automation class — no engine invocation compiles to it (S08.9)", ErrInvalid)
	default:
		return Compiled{}, fmt.Errorf("%w: confinement class %q is not a v0 engine class (S11.6)", ErrInvalid, g.Class)
	}
	for _, tool := range g.GrantedTools {
		if nativeSpawnFamily[tool] {
			return Compiled{}, fmt.Errorf("%w: granted tool %q is engine-native spawn — structurally excluded (S03.5 sole-controller)", ErrInvalid, tool)
		}
	}

	// The agent prompt: template body (base) + the overlay L2 slice,
	// concatenated deterministically, most-specific-last under the 8.9
	// precedence (template baseline before personal overlay — Spec S08.4).
	// The template file is never mutated; overlay items with
	// guardrail-shaped fields or screen hits are refused (S08.4 zero
	// guardrail fields; belt to the write-time machinery).
	var prompt strings.Builder
	prompt.WriteString(in.Def.Body)
	for _, line := range in.Def.Persona {
		prompt.WriteString("\n")
		prompt.WriteString(line)
	}
	for i, item := range in.Overlay {
		if findings := ScreenOverlayContent(fmt.Sprintf("overlay[%d] %s", i, item.EntryID), item.Content); len(findings) > 0 {
			for _, f := range findings {
				if f.Code == FindingGuardrail {
					return Compiled{}, fmt.Errorf("%w: %s", ErrGuardrailField, f.Message)
				}
			}
			return Compiled{}, fmt.Errorf("%w: %s", ErrLintReject, findings[0].Message)
		}
		if i == 0 {
			prompt.WriteString("\n\n## Personal overlay (precedence: personal overlay > template baseline — 8.9)\n")
		}
		fmt.Fprintf(&prompt, "\n[overlay lesson %s v%d] %s\n%s\n", item.EntryID, item.Version, item.Title, item.Content)
	}

	agentName := in.Def.Name
	agents := map[string]map[string]string{
		agentName: {"description": in.Def.Description, "prompt": prompt.String()},
	}
	agentsJSON, err := json.Marshal(agents)
	if err != nil {
		return Compiled{}, fmt.Errorf("worker: marshal agent definition: %w", err)
	}

	out := Compiled{
		TemplateID: in.TemplateID,
		VersionID:  in.VersionID,
		Worker: adapters.CompiledWorker{
			AgentsJSON:              agentsJSON,
			AgentName:               agentName,
			ToolAllowlist:           append([]string(nil), g.GrantedTools...),
			GatedTools:              append([]string(nil), g.GatedTools...),
			PermissionMode:          g.PermissionMode,
			SessionStartContextPath: in.Instance.PinnedContextPath,
		},
		Class:          g.Class,
		Egress:         g.Egress,
		EgressHosts:    append([]string(nil), g.EgressHosts...),
		CeilingCostUSD: g.BudgetUSD,
		CeilingSteps:   g.BudgetSteps,
	}
	for _, sk := range in.Skills {
		out.SkillDirs = append(out.SkillDirs, sk.Dir)
	}

	hash, err := configHash(in, out)
	if err != nil {
		return Compiled{}, err
	}
	out.ConfigHash = hash
	return out, nil
}

// configHash hashes the compiled unit — body + config together, one unit
// (Spec S08.3) — as canonical JSON.
func configHash(in CompileInput, c Compiled) (string, error) {
	overlay := make([]map[string]any, 0, len(in.Overlay))
	for _, o := range in.Overlay {
		overlay = append(overlay, map[string]any{
			"id": o.EntryID, "version": o.Version, "sha256": sha256Hex([]byte(o.Content)),
		})
	}
	unit := map[string]any{
		"template_id":   in.TemplateID,
		"version_id":    in.VersionID,
		"file_sha256":   in.FileSHA256,
		"guardrails":    in.Guardrails,
		"overlay":       overlay,
		"instance":      in.Instance,
		"agents_json":   string(c.Worker.AgentsJSON),
		"agent_name":    c.Worker.AgentName,
		"tools":         c.Worker.ToolAllowlist,
		"gated_tools":   c.Worker.GatedTools,
		"permission":    c.Worker.PermissionMode,
		"session_start": c.Worker.SessionStartContextPath,
		"class":         c.Class,
		"egress":        map[string]any{"class": c.Egress, "hosts": c.EgressHosts},
		"ceilings":      map[string]any{"usd": c.CeilingCostUSD, "steps": c.CeilingSteps},
		"skills":        in.Skills,
	}
	raw, err := json.Marshal(unit)
	if err != nil {
		return "", fmt.Errorf("worker: hash compiled unit: %w", err)
	}
	return sha256Hex(raw), nil
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
