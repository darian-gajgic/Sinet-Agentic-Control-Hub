package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/sandbox"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/worker/automation"
)

// battery.go — the four-station validation battery (Spec S08.6): 7.2's
// "validation = structural checks passed plus human sign-off" made into
// stations. Station 1 (schema lint + instruction-pattern screen) and
// station 2 (permission audit vs the task-class ceiling table +
// Rule-of-Two) are deterministic and free; station 3 is the ⚙-capped
// sandboxed dry run on the requester's lane (agentic — through the
// DryEngine seam) or the sample-payload test execution (automation — every
// outward effect a gated proposal, D7 makes it free); station 4 is the
// approval-as-diff card the store's Approve verb consumes. Every pass
// writes a validation_records row per (version × model × engine pin);
// validation asserts structure, powers, and a witnessed run — NOTHING
// about output quality (7.2; quality is S07's).

// DryRunRequest is one station-3 dry run: the compiled draft invocation on
// an effects-impossible sandbox — C1, or the worker's class capped at C2
// (Spec S08.6 station 3) — capped at ⚙ workers.dryrun_cost_cap_usd.
type DryRunRequest struct {
	Compiled   Compiled
	SampleTask string
	CostCapUSD float64
	Class      string
}

// DryRunResult is a witnessed dry run; transcript and output attach to the
// approval card.
type DryRunResult struct {
	Completed     bool    `json:"completed"`
	TranscriptRef string  `json:"transcript_ref,omitempty"`
	Output        string  `json:"output,omitempty"`
	CostUSD       float64 `json:"cost_usd,omitempty"`
	Detail        string  `json:"detail,omitempty"`
}

// DryEngine runs a compiled draft against the requester's lane inside the
// composed sandbox. A named engine seam: the production implementation is
// the stage layer's EngineDryRun (B3-5) — one stage session on the
// composition ceremony run through the S03 adapters, the compiled draft as
// the invocation, ceilings and class from the battery's DryRunRequest;
// tests use fakes (zero paid calls).
type DryEngine interface {
	DryRun(ctx context.Context, req DryRunRequest) (DryRunResult, error)
}

// BatteryInput parameterizes one battery pass.
type BatteryInput struct {
	// Actor is the requester driving validation (recorded on events).
	Actor string
	// SampleTask is the station-3 sample (composer-proposed, requester-
	// curated on the card — Spec S08.6; for automations it is the sample
	// payload, JSON).
	SampleTask string
	// Engine runs agentic dry runs; required for kind=agentic.
	Engine DryEngine
	// Model + EnginePin key the validation record for kind=agentic (the
	// lane the dry run witnessed). For kind=automation both are ignored:
	// the record keys on the dialect version (Spec S08.1, S08.9).
	Model     string
	EnginePin string
	// Verbs is the automation verb registry; required for
	// kind=automation.
	Verbs automation.Verbs
	// Journal receives the outward steps of an automation test execution
	// as gated proposals (Spec S08.9 station 3).
	Journal *gates.Journal
}

// BatteryResult is one full pass.
type BatteryResult struct {
	VersionID string        `json:"version_id"`
	Model     string        `json:"model,omitempty"`
	EnginePin string        `json:"engine_pin"`
	Lint      StationResult `json:"lint"`
	Audit     AuditResult   `json:"audit"`
	DryRun    *DryRunResult `json:"dry_run,omitempty"`
	// GoldenSeed is the dry-run pair that seeds the worker's golden set on
	// approval, labeled unverified until the first accepted real outcome
	// confirms it (Spec S08.6 station 3, discharges R15-OQ4).
	GoldenSeed *GoldenSeed `json:"golden_seed,omitempty"`
	Green      bool        `json:"green"`
	RecordID   int64       `json:"record_id"`
}

// GoldenSeed is the sample task + witnessed output pair.
type GoldenSeed struct {
	SampleTask   string `json:"sample_task"`
	OutputSHA256 string `json:"output_sha256,omitempty"`
	Verified     bool   `json:"verified"` // false at birth (S08.6)
}

// RunBattery runs stations 1–3 for a version and writes the validation
// record (Spec S08.6). Green = station 1 has no errors AND station 2 is
// structurally admissible (no Rule-of-Two refusal, no guardrail-field
// reject) AND the witnessed run completed; above-ceiling FLAGS do not
// block validation — they gate approval acknowledgement (station 4). A red
// pass still writes its record and leaves the template in draft.
func (s *Store) RunBattery(ctx context.Context, versionID string, in BatteryInput) (BatteryResult, error) {
	if _, err := s.person(ctx, in.Actor); err != nil {
		return BatteryResult{}, err
	}
	v, err := s.VersionByID(ctx, versionID)
	if err != nil {
		return BatteryResult{}, err
	}
	t, err := s.Template(ctx, v.TemplateID)
	if err != nil {
		return BatteryResult{}, err
	}
	src, err := s.readTemplateFile(v)
	if err != nil {
		return BatteryResult{}, err
	}
	def, parseFindings, err := ParseTemplate(src)
	if err != nil {
		return BatteryResult{}, err
	}

	res := BatteryResult{VersionID: versionID}

	// Station 1 — schema lint + instruction-pattern screen (deterministic).
	res.Lint, err = LintTemplate(def, parseFindings, s, s.settings)
	if err != nil {
		return BatteryResult{}, err
	}
	guardrailReject := false
	for _, f := range res.Lint.Errors {
		if f.Code == FindingGuardrail {
			guardrailReject = true
		}
	}
	// kind=automation: the body must parse in the pinned dialect and every
	// outward step must carry its approval node (Spec S08.9 — station 1
	// for the workflow definition).
	var wf automation.Workflow
	if t.Kind == KindAutomation {
		wf, err = automation.Parse(def.Body)
		if err != nil {
			res.Lint.Errors = append(res.Lint.Errors, Finding{Code: FindingDialect, Message: err.Error()})
		} else if in.Verbs != nil {
			if err := automation.Validate(wf, in.Verbs); err != nil {
				res.Lint.Errors = append(res.Lint.Errors, Finding{Code: FindingDialect, Message: err.Error()})
			}
		} else {
			res.Lint.Errors = append(res.Lint.Errors, Finding{Code: FindingDialect,
				Message: "no verb registry supplied — automation verbs cannot resolve (S08.9 fixed connector verbs)"})
		}
	}

	// Station 2 — permission audit vs the ceiling table + Rule-of-Two
	// (deterministic — a table, not a model).
	res.Audit = AuditPermissions(def, v.Requested, guardrailReject)

	// Station 3 — witnessed run, only when the structural stations stand.
	structuralGreen := res.Lint.Green() && res.Audit.Green()
	if structuralGreen {
		switch t.Kind {
		case KindAgentic:
			dr, err := s.agenticDryRun(ctx, t, v, def, in)
			if err != nil {
				return BatteryResult{}, err
			}
			res.DryRun = &dr
			res.EnginePin, res.Model = in.EnginePin, in.Model
		case KindAutomation:
			dr, err := automationDryRun(ctx, wf, in)
			if err != nil {
				return BatteryResult{}, err
			}
			res.DryRun = &dr
			res.EnginePin = automation.DialectVersion
		}
		if res.DryRun.Completed {
			res.GoldenSeed = &GoldenSeed{
				SampleTask:   in.SampleTask,
				OutputSHA256: sha256Hex([]byte(res.DryRun.Output)),
				Verified:     false,
			}
		}
	} else if t.Kind == KindAutomation {
		res.EnginePin = automation.DialectVersion
	} else {
		res.EnginePin, res.Model = in.EnginePin, in.Model
	}
	if res.EnginePin == "" {
		return BatteryResult{}, fmt.Errorf("%w: an engine pin (or dialect version) keys the validation record (S08.1)", ErrInvalid)
	}

	res.Green = structuralGreen && res.DryRun != nil && res.DryRun.Completed

	// The record (Spec S08.6: each pass writes a row) + status flip on
	// green (draft → validated; approval is station 4's).
	lintJSON, err := json.Marshal(res.Lint)
	if err != nil {
		return BatteryResult{}, err
	}
	auditJSON, err := json.Marshal(res.Audit)
	if err != nil {
		return BatteryResult{}, err
	}
	dryJSON := ""
	if res.DryRun != nil {
		ref := map[string]any{"dry_run": res.DryRun, "golden_seed": res.GoldenSeed}
		raw, err := json.Marshal(ref)
		if err != nil {
			return BatteryResult{}, err
		}
		dryJSON = string(raw)
	}
	now := rfc3339(s.now())
	err = s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx,
			`INSERT INTO validation_records (version_id, model, engine_pin, lint_result, audit_result, dryrun_ref, green, created_ts)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			versionID, res.Model, res.EnginePin, string(lintJSON), string(auditJSON), dryJSON,
			boolInt(res.Green), now)
		if err != nil {
			return fmt.Errorf("worker: write validation record: %w", err)
		}
		res.RecordID, err = r.LastInsertId()
		if err != nil {
			return err
		}
		if res.Green && t.Status == StatusDraft {
			if _, err := tx.ExecContext(ctx,
				`UPDATE worker_templates SET status = ?, updated_ts = ? WHERE template_id = ?`,
				string(StatusValidated), now, t.ID); err != nil {
				return fmt.Errorf("worker: mark validated: %w", err)
			}
		}
		_, err = s.log.AppendTx(ctx, tx, s.event(in.Actor, evValidated, map[string]any{
			"template": t.ID, "version": versionID, "green": res.Green,
			"model": res.Model, "engine_pin": res.EnginePin, "record": res.RecordID,
		}))
		return err
	})
	if err != nil {
		return BatteryResult{}, err
	}
	return res, nil
}

// agenticDryRun compiles the DRAFT (requested grants as if granted — the
// witness run witnesses the real configuration) and runs it through the
// DryEngine seam in the effects-impossible class: C1, or the requested
// class capped at C2 (Spec S08.6 station 3), capped at ⚙
// workers.dryrun_cost_cap_usd.
func (s *Store) agenticDryRun(ctx context.Context, t Template, v Version, def Definition, in BatteryInput) (DryRunResult, error) {
	if in.Engine == nil {
		return DryRunResult{}, fmt.Errorf("%w: an agentic battery needs a DryEngine (S08.6 station 3)", ErrInvalid)
	}
	cap, err := s.settings.Float(keyDryrunCostCap)
	if err != nil {
		return DryRunResult{}, fmt.Errorf("worker: read ⚙ %s: %w", keyDryrunCostCap, err)
	}
	class := string(sandbox.C1)
	if v.Requested.Class == string(sandbox.C2) {
		class = string(sandbox.C2)
	}
	draft := Guardrails{
		VersionID: v.ID, GrantedTools: v.Requested.Tools, GatedTools: v.Requested.GatedTools,
		PermissionMode: v.Requested.PermissionMode, Class: class,
		Egress: v.Requested.Egress, EgressHosts: v.Requested.EgressHosts,
		BudgetUSD: cap, BudgetSteps: v.Requested.BudgetSteps,
	}
	var skills []CompiledSkillRef
	for _, name := range def.Equipment.Skills {
		ref, err := s.skillRefFor(name)
		if err != nil {
			return DryRunResult{}, err
		}
		skills = append(skills, ref)
	}
	compiled, err := CompileInvocation(CompileInput{
		TemplateID: t.ID, VersionID: v.ID, Def: def, FileSHA256: v.FileSHA256,
		Guardrails: draft,
		Instance:   InstanceRefs{RunID: "dryrun." + v.ID, TaskID: "dryrun", Stage: "dryrun"},
		Skills:     skills,
	})
	if err != nil {
		return DryRunResult{}, err
	}
	return in.Engine.DryRun(ctx, DryRunRequest{
		Compiled: compiled, SampleTask: in.SampleTask, CostCapUSD: cap, Class: class,
	})
}

// automationDryRun is station 3 for kind=automation: test execution on the
// sample payload with every outward effect materializing as a gated
// proposal — no model, no cost (Spec S08.9).
func automationDryRun(ctx context.Context, wf automation.Workflow, in BatteryInput) (DryRunResult, error) {
	payload := json.RawMessage(in.SampleTask)
	if strings.TrimSpace(in.SampleTask) == "" {
		payload = json.RawMessage(`{}`)
	}
	rep, err := automation.Execute(ctx, automation.ExecInput{
		Workflow: wf, Payload: payload, Verbs: in.Verbs,
		Journal: in.Journal, UserID: in.Actor,
	})
	if err != nil {
		return DryRunResult{Completed: false, Detail: err.Error()}, nil
	}
	raw, err := json.Marshal(rep)
	if err != nil {
		return DryRunResult{}, err
	}
	return DryRunResult{Completed: true, Output: string(raw)}, nil
}

// LatestValidation loads the newest validation record of a version.
func (s *Store) LatestValidation(ctx context.Context, versionID string) (ValidationRecord, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT record_id, version_id, model, engine_pin, lint_result, audit_result,
		        dryrun_ref, green, approver, coalesce(approved_ts, ''), created_ts
		 FROM validation_records WHERE version_id = ? ORDER BY record_id DESC LIMIT 1`, versionID)
	var rec ValidationRecord
	var lintJSON, auditJSON string
	var green int64
	err := row.Scan(&rec.ID, &rec.VersionID, &rec.Model, &rec.EnginePin, &lintJSON,
		&auditJSON, &rec.DryRunRef, &green, &rec.Approver, &rec.ApprovedTS, &rec.CreatedTS)
	if err == sql.ErrNoRows {
		return rec, fmt.Errorf("%w: no validation record for version %q", ErrNotValidated, versionID)
	}
	if err != nil {
		return rec, fmt.Errorf("worker: load validation record: %w", err)
	}
	rec.Green = green == 1
	if err := json.Unmarshal([]byte(lintJSON), &rec.Lint); err != nil {
		return rec, fmt.Errorf("worker: decode lint result: %w", err)
	}
	if err := json.Unmarshal([]byte(auditJSON), &rec.Audit); err != nil {
		return rec, fmt.Errorf("worker: decode audit result: %w", err)
	}
	return rec, nil
}

// ApprovalCard is station 4's content (Spec S08.6, D10, 13.5): the
// definition as a readable diff, the requested-vs-ceiling powers table,
// the dry-run result, and the provenance block. The owner approves
// personal workers; the operator approves household promotion.
type ApprovalCard struct {
	TemplateID string      `json:"template_id"`
	VersionID  string      `json:"version_id"`
	Diff       string      `json:"diff"`
	Powers     AuditResult `json:"powers"`
	DryRunRef  string      `json:"dry_run_ref,omitempty"`
	Provenance Provenance  `json:"provenance"`
	FirstN     int64       `json:"first_n"`
	Flagged    []string    `json:"flagged,omitempty"`
}

// BuildApprovalCard assembles the card for a version from its stored
// pieces (deterministic — no extra state; the UI renders it at B6).
func (s *Store) BuildApprovalCard(ctx context.Context, versionID string) (ApprovalCard, error) {
	v, err := s.VersionByID(ctx, versionID)
	if err != nil {
		return ApprovalCard{}, err
	}
	rec, err := s.LatestValidation(ctx, versionID)
	if err != nil {
		return ApprovalCard{}, err
	}
	newSrc, err := s.readTemplateFile(v)
	if err != nil {
		return ApprovalCard{}, err
	}
	oldSrc := ""
	if v.Supersedes != "" {
		prev, err := s.VersionByID(ctx, v.Supersedes)
		if err != nil {
			return ApprovalCard{}, err
		}
		oldSrc, err = s.readTemplateFile(prev)
		if err != nil {
			return ApprovalCard{}, err
		}
	}
	firstN, err := s.settings.Int(keyFirstN)
	if err != nil {
		return ApprovalCard{}, fmt.Errorf("worker: read ⚙ %s: %w", keyFirstN, err)
	}
	return ApprovalCard{
		TemplateID: v.TemplateID, VersionID: v.ID,
		Diff: unifiedDiff(oldSrc, newSrc), Powers: rec.Audit, DryRunRef: rec.DryRunRef,
		Provenance: Provenance{AuthorKind: v.AuthorKind, Composer: v.Composer,
			PlaybookVer: v.PlaybookVer, EvidenceRef: v.EvidenceRef,
			Origin: v.Origin, OriginRef: v.OriginRef},
		FirstN: firstN, Flagged: rec.Audit.FlaggedItems(),
	}, nil
}

// unifiedDiff renders a minimal line diff (LCS) — the readable-diff body
// of the approval card. Template files are small; O(n·m) is fine.
func unifiedDiff(oldSrc, newSrc string) string {
	if oldSrc == newSrc {
		return ""
	}
	a := strings.Split(strings.TrimRight(oldSrc, "\n"), "\n")
	b := strings.Split(strings.TrimRight(newSrc, "\n"), "\n")
	if oldSrc == "" {
		a = nil
	}
	// LCS table.
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	var out strings.Builder
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			fmt.Fprintf(&out, "  %s\n", a[i])
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			fmt.Fprintf(&out, "- %s\n", a[i])
			i++
		default:
			fmt.Fprintf(&out, "+ %s\n", b[j])
			j++
		}
	}
	for ; i < len(a); i++ {
		fmt.Fprintf(&out, "- %s\n", a[i])
	}
	for ; j < len(b); j++ {
		fmt.Fprintf(&out, "+ %s\n", b[j])
	}
	return out.String()
}
