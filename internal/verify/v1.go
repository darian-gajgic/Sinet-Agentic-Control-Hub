package verify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// V1 — domain check packs and plan-conformance checks (Spec S07.3). The
// software launch domain runs the executable ladder per revision in the task
// sandbox with evidence artifacts retained: lint+typecheck+build →
// unit/integration tests → runtime smoke → drive-the-feature e2e. The
// hardened MUST rules of Spec S07.3 are carried here structurally where the
// substrate exists and behind named seams where it is owned elsewhere:
//
//  1. Network off by default; answer-bearing VCS history stripped — the
//     CheckRunner seam composes the per-run sandbox (Spec S11, via
//     adapters.Confiner); history stripping is the verification workspace's
//     contract (WorkspaceProvider, Spec S13/B4).
//  2. Tests read-only to the executor — an executor-side confinement fact
//     (Spec S11 workspace binds); the verification run itself also cannot
//     rewrite checks (the pack is platform data).
//  3. The pass/fail verdict is computed OUTSIDE every agent-accessible
//     environment: the platform derives it from the process exit status in
//     control-plane code — in-band success signals are never trusted.
//  4. Test provenance: acceptance checks originate from the frozen ACs
//     (structured sub-line) and carry their separate-context authoring
//     provenance; doer-written tests pass by construction and are rejected
//     at pack validation.
//  5. Sanctioned challenge instead of edit access: ChallengeCheck raises a
//     CHECK-INTEGRITY card (Spec S07.7).
//  6. Flaky checks are quarantined with an owner and a fix-by date, never
//     retried-until-green: a quarantined check is skipped, recorded, and
//     carded — RunV1 never re-runs a check within a pass.
//  7. Every check suite carries a verified-on stamp and a periodic
//     planted-defect audit (P-T06-1, Spec S07.9): a suite past
//     ⚙ verification.check_audit_interval_days flags the verdict stale;
//     audit failure quarantines via RecordAuditFailure.

// LadderStage is one rung of the software executable ladder (Spec S07.3).
type LadderStage string

const (
	// StageStatic is lint + typecheck + build.
	StageStatic LadderStage = "static"
	// StageUnit is unit/integration tests.
	StageUnit LadderStage = "unit"
	// StageSmoke is the runtime smoke.
	StageSmoke LadderStage = "smoke"
	// StageE2E drives the feature end to end.
	StageE2E LadderStage = "e2e"
)

// ladderOrder is the cheap-first execution order (Spec S07.3).
var ladderOrder = []LadderStage{StageStatic, StageUnit, StageSmoke, StageE2E}

func stageRank(s LadderStage) (int, bool) {
	for i, l := range ladderOrder {
		if l == s {
			return i, true
		}
	}
	return 0, false
}

// ContractState is a stage-contract state (Spec S07.3): PASS / FAIL / N-A /
// UNVERIFIABLE-HERE, with first-upstream-failure attribution.
type ContractState string

const (
	ContractPass ContractState = "PASS"
	ContractFail ContractState = "FAIL"
	ContractNA   ContractState = "N-A"
	// ContractUnverifiable: the contract cannot be decided at this boundary
	// (an upstream failure blocked it, or the deciding substrate is not
	// wired). Definition-of-done includes definition-of-cannot-be-done-here
	// (Spec S07.3).
	ContractUnverifiable ContractState = "UNVERIFIABLE-HERE"
)

// Check is one executable check of a domain pack.
type Check struct {
	ID    string      `json:"id"`
	Stage LadderStage `json:"stage"`
	// Argv is the check command, run inside the verification sandbox.
	Argv []string `json:"argv"`
	// StepID names the PLAN step whose "Done when" contract this check
	// decides at its stage boundary ("" = infrastructure check).
	StepID string `json:"step_id,omitempty"`
	// ACKey names the frozen criterion (structured sub-line) this check
	// executes ("" = not an acceptance check). V2 consumes the outcome as
	// evidence and never re-decides the mechanical fact (Spec S07.5).
	ACKey string `json:"ac_key,omitempty"`
	// Provenance records the separate authoring context of an acceptance
	// check (different session or model from the implementation — Spec
	// S07.3 rule 4). Required when ACKey is set.
	Provenance string `json:"provenance,omitempty"`
	// FindingCategory declares the route-table category a failure of this
	// check escalates under — a stage contract is incomplete unless it
	// declares its finding categories and their escalation routes (Spec
	// S07.3).
	FindingCategory Category `json:"finding_category"`
}

// Quarantine is one flake-quarantine record (Spec S07.3 rule 6): owner and
// fix-by date, never retried-until-green.
type Quarantine struct {
	CheckID string    `json:"check_id"`
	Owner   string    `json:"owner"`
	Reason  string    `json:"reason"`
	FixBy   time.Time `json:"fix_by"`
}

// CheckPack is one domain's check suite — a named, versioned, falsifiable
// object (Spec S07.10 "no prefab scores").
type CheckPack struct {
	Domain  string `json:"domain"`
	Version int    `json:"version"`
	// VerifiedOn is the rule-7 stamp: when the suite last passed its
	// planted-defect audit (P-T06-1).
	VerifiedOn time.Time `json:"verified_on"`
	Checks     []Check   `json:"checks"`
	// Quarantines maps check id → active quarantine.
	Quarantines map[string]Quarantine `json:"quarantines,omitempty"`
	// Posture, when set, marks a resolution that is not a runnable suite —
	// today only PostureBootstrap, the registered project holding no
	// executable rung (Spec S07.8 [A14, 2026-08-27]; see bootstrap.go). The
	// flag lives on the pack because the resolver seam answers with a pack:
	// it keeps "this project has nothing to run yet" distinguishable from the
	// (nil, nil) "this domain has no pack machinery" absence without
	// overloading either. A posture-carrying pack is deliberately not a valid
	// one — Validate still refuses a pack without checks.
	Posture Posture `json:"posture,omitempty"`
}

// Validate checks the pack contract (Spec S07.3): known ladder stages,
// declared finding categories with live routes, provenance on acceptance
// checks, a verified-on stamp.
func (p *CheckPack) Validate() error {
	if p.Domain == "" || p.Version < 1 {
		return fmt.Errorf("%w: pack requires domain and version", ErrBadPack)
	}
	if p.VerifiedOn.IsZero() {
		return fmt.Errorf("%w: pack without verified-on stamp (Spec S07.3 rule 7)", ErrBadPack)
	}
	if len(p.Checks) == 0 {
		return fmt.Errorf("%w: pack without checks", ErrBadPack)
	}
	seen := map[string]bool{}
	for _, c := range p.Checks {
		if c.ID == "" || len(c.Argv) == 0 {
			return fmt.Errorf("%w: check requires id and argv", ErrBadPack)
		}
		if seen[c.ID] {
			return fmt.Errorf("%w: duplicate check id %q", ErrBadPack, c.ID)
		}
		seen[c.ID] = true
		if _, ok := stageRank(c.Stage); !ok {
			return fmt.Errorf("%w: check %q stage %q not on the ladder", ErrBadPack, c.ID, c.Stage)
		}
		if _, ok := RouteTable[c.FindingCategory]; !ok {
			return fmt.Errorf("%w: check %q declares no routed finding category (Spec S07.3 stage-contract rule)", ErrBadPack, c.ID)
		}
		if c.ACKey != "" && c.Provenance == "" {
			return fmt.Errorf("%w: acceptance check %q without separate-context provenance (Spec S07.3 rule 4)", ErrBadPack, c.ID)
		}
	}
	for id := range p.Quarantines {
		if !seen[id] {
			return fmt.Errorf("%w: quarantine for unknown check %q", ErrBadPack, id)
		}
	}
	return nil
}

// AuditStale reports whether the suite is past its planted-defect audit
// interval (⚙ verification.check_audit_interval_days): a stale suite flags
// the verdict card stale (P-T06-1, Spec S07.9).
func (p *CheckPack) AuditStale(now time.Time, settings Settings) (bool, error) {
	days, err := settings.Int(keyCheckAuditDays)
	if err != nil {
		return false, fmt.Errorf("verify: read ⚙ %s: %w", keyCheckAuditDays, err)
	}
	return now.Sub(p.VerifiedOn) > time.Duration(days)*24*time.Hour, nil
}

// QuarantineCheck records a flake quarantine (Spec S07.3 rule 6). The card
// (CHECK-INTEGRITY route) is the Escalator's; this mutates the pack record.
func (p *CheckPack) QuarantineCheck(q Quarantine) error {
	if q.CheckID == "" || q.Owner == "" || q.FixBy.IsZero() {
		return fmt.Errorf("%w: quarantine requires check, owner, fix-by (Spec S07.3 rule 6)", ErrBadPack)
	}
	found := false
	for _, c := range p.Checks {
		if c.ID == q.CheckID {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("%w: quarantine for unknown check %q", ErrBadPack, q.CheckID)
	}
	if p.Quarantines == nil {
		p.Quarantines = map[string]Quarantine{}
	}
	p.Quarantines[q.CheckID] = q
	return nil
}

// CheckOutcomeState is a per-check execution state.
type CheckOutcomeState string

const (
	CheckPassed CheckOutcomeState = "PASS"
	CheckFailed CheckOutcomeState = "FAIL"
	// CheckUnverifiable: not run because an upstream ladder stage failed.
	CheckUnverifiable CheckOutcomeState = "UNVERIFIABLE-HERE"
	// CheckQuarantined: skipped under an active flake quarantine.
	CheckQuarantined CheckOutcomeState = "QUARANTINED"
	// CheckRunnerFailed: the runner itself failed (tool failure, not a
	// verdict) — recorded, escalates rather than approves.
	CheckRunnerFailed CheckOutcomeState = "RUNNER-FAILURE"
)

// CheckOutcome is one executed (or skipped) check with its retained
// evidence ref (refs-not-blobs, P-T07-5).
type CheckOutcome struct {
	CheckID      string            `json:"check_id"`
	Stage        LadderStage       `json:"stage"`
	StepID       string            `json:"step_id,omitempty"`
	ACKey        string            `json:"ac_key,omitempty"`
	State        CheckOutcomeState `json:"state"`
	ExitCode     int               `json:"exit_code,omitempty"`
	EvidenceRef  string            `json:"evidence_ref,omitempty"`
	EvidenceSHA  string            `json:"evidence_sha256,omitempty"`
	AttributedTo string            `json:"attributed_to,omitempty"` // first upstream failure (check id)
	Detail       string            `json:"detail,omitempty"`
}

// StepContract is one PLAN step's "Done when" contract decided at its stage
// boundary (Spec S07.3), with its declared finding category and escalation
// route.
type StepContract struct {
	StepID       string        `json:"step_id"`
	DoneWhen     string        `json:"done_when"`
	State        ContractState `json:"state"`
	AttributedTo string        `json:"attributed_to,omitempty"` // first upstream failure
	Category     Category      `json:"category,omitempty"`
	Route        Sink          `json:"route,omitempty"`
}

// V1Result is the V1 layer outcome.
type V1Result struct {
	Checks []CheckOutcome `json:"checks"`
	Steps  []StepContract `json:"steps,omitempty"`
	// StaleAudit flags a suite past its audit interval (P-T06-1): the
	// verdict card renders stale.
	StaleAudit bool `json:"stale_audit,omitempty"`
	// Findings carries the platform-raised V1 findings (quarantine skips,
	// runner failures) into the round record.
	Findings []Finding `json:"findings,omitempty"`
	// PackVersion/PackVerifiedOn identify the suite that ran (recording,
	// Spec S07.11).
	PackVersion    int       `json:"pack_version"`
	PackVerifiedOn time.Time `json:"pack_verified_on"`
}

// ACOutcomes maps the frozen-criterion keys executed at V1 to their
// mechanical outcome — the facts V2 consumes as evidence and never
// re-decides (Spec S07.5).
func (r *V1Result) ACOutcomes() map[string]CheckOutcomeState {
	m := map[string]CheckOutcomeState{}
	for _, c := range r.Checks {
		if c.ACKey != "" {
			m[c.ACKey] = c.State
		}
	}
	return m
}

// CheckRequest is one check execution request.
type CheckRequest struct {
	RunID string
	Check Check
	// Workspace is the verification workspace (Spec S13 seam: the revision
	// materialized with answer-bearing VCS history stripped, P-T06-2).
	Workspace string
	// EvidenceDir receives the retained per-check output artifact.
	EvidenceDir string
}

// CheckResult is the runner's raw result. Pass is computed by the PLATFORM
// from the exit status — never by anything inside the sandbox (Spec S07.3
// rule 3).
type CheckResult struct {
	ExitCode    int
	EvidenceRef string
	EvidenceSHA string
}

// CheckRunner executes one check inside the network-off verification
// sandbox (Spec S07.3 rule 1; sandbox mechanics Spec S11 behind the
// adapters.Confiner seam). Implementations must not interpret the check's
// output — the verdict derivation happens in RunV1.
type CheckRunner interface {
	RunCheck(ctx context.Context, req CheckRequest) (CheckResult, error)
}

// SandboxCheckRunner is the production CheckRunner: it composes the per-run
// sandbox through the adapters.Confiner seam (internal/sandbox.Composer in
// production) and runs the check argv inside it.
//
// Network posture (Spec S07.3 rule 1, P-T06-2): class C1 composes with an
// empty netns (structurally network-off); C2 (writable workspace, needed for
// build/test rungs) also composes network-off today — egress-needing classes
// ride an empty netns until the deferred host egress substrate lands
// (CONVENTIONS §12). When that substrate lands, the verification profile
// MUST stay network-off — a flagged B4 obligation on the S11/S13 wiring,
// recorded in the packet report; nothing in this package may widen it.
type SandboxCheckRunner struct {
	Confiner adapters.Confiner
	// Class is the confinement class for check runs ("C2" default: the
	// build/test rungs write to the workspace overlay; C2 is network-off
	// until the egress substrate lands, see above).
	Class string
	// WorkDir is the platform-owned scratch dir handed to the composer
	// (holds the srt config when the srt path is active).
	WorkDir string
	// Env is the minimal check environment (never the engine's lowered
	// env; no credentials by construction — the sandbox starts empty-env).
	Env []string
}

// RunCheck implements CheckRunner. The exit status is read by the platform
// from the process wait status — the in-sandbox process cannot fake a
// wait(2) result — and the combined output is retained as the evidence
// artifact.
func (r *SandboxCheckRunner) RunCheck(ctx context.Context, req CheckRequest) (CheckResult, error) {
	if r.Confiner == nil {
		return CheckResult{}, fmt.Errorf("%w: SandboxCheckRunner without a Confiner (Spec S11 seam)", ErrSeamMissing)
	}
	class := r.Class
	if class == "" {
		class = "C2"
	}
	cmd, cleanup, err := r.Confiner.Confine(
		adapters.StartRequest{RunID: req.RunID, Class: class, WorkDir: r.WorkDir},
		adapters.SpawnSpec{Argv: req.Check.Argv, Env: r.Env, Workspace: req.Workspace},
	)
	if err != nil {
		return CheckResult{}, fmt.Errorf("verify: compose check sandbox: %w", err)
	}
	if cleanup != nil {
		defer cleanup()
	}
	if err := os.MkdirAll(req.EvidenceDir, 0o700); err != nil {
		return CheckResult{}, fmt.Errorf("verify: evidence dir: %w", err)
	}
	evidence := filepath.Join(req.EvidenceDir, req.Check.ID+".log")
	out, err := os.Create(evidence)
	if err != nil {
		return CheckResult{}, fmt.Errorf("verify: evidence file: %w", err)
	}
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.Dir = req.Workspace
	runErr := cmd.Run()
	if cerr := out.Close(); cerr != nil {
		return CheckResult{}, fmt.Errorf("verify: close evidence: %w", cerr)
	}
	code := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) {
			return CheckResult{}, fmt.Errorf("verify: run check %q: %w", req.Check.ID, runErr)
		}
		code = exitErr.ExitCode()
	}
	raw, err := os.ReadFile(evidence)
	if err != nil {
		return CheckResult{}, fmt.Errorf("verify: read evidence: %w", err)
	}
	sum := sha256.Sum256(raw)
	return CheckResult{ExitCode: code, EvidenceRef: evidence, EvidenceSHA: hex.EncodeToString(sum[:])}, nil
}

// RunV1 executes the pack ladder cheap-first over the verification
// workspace: quarantined checks are skipped (rule 6), the first failing
// stage stops later stages (their checks and contracts become
// UNVERIFIABLE-HERE with first-upstream-failure attribution), and every
// verdict derivation happens here, platform-side (rule 3).
func RunV1(ctx context.Context, pack *CheckPack, runner CheckRunner, req CheckRequest, steps []intake.Step, now time.Time, settings Settings) (V1Result, error) {
	if err := pack.Validate(); err != nil {
		// An invalid pack is a SCREEN THAT CANNOT RUN, wherever it is caught:
		// re-running it produces the identical refusal, so the ladder could only
		// fork it to a tombstone. Marked as the preamble refusal class so it takes
		// the S07.7 decision-card door (P3-RW-14A drain D3), which matters the
		// moment a pack source appears that this package cannot validate at
		// resolve time — today the composition root validates before handing one
		// over, so this is the latent path, not the live one.
		return V1Result{}, NewPreambleRefusal(err)
	}
	if runner == nil {
		return V1Result{}, fmt.Errorf("%w: V1 without a CheckRunner", ErrSeamMissing)
	}
	stale, err := pack.AuditStale(now, settings)
	if err != nil {
		return V1Result{}, err
	}
	res := V1Result{StaleAudit: stale, PackVersion: pack.Version, PackVerifiedOn: pack.VerifiedOn}

	// Execute in ladder order; within a stage, pack order.
	firstFailure := ""
	failedStage := -1
	for _, stage := range ladderOrder {
		rank, _ := stageRank(stage)
		for _, c := range pack.Checks {
			if c.Stage != stage {
				continue
			}
			switch {
			case failedStage >= 0 && rank > failedStage:
				res.Checks = append(res.Checks, CheckOutcome{
					CheckID: c.ID, Stage: c.Stage, StepID: c.StepID, ACKey: c.ACKey,
					State: CheckUnverifiable, AttributedTo: firstFailure,
				})
			case pack.Quarantines[c.ID].CheckID != "":
				q := pack.Quarantines[c.ID]
				res.Checks = append(res.Checks, CheckOutcome{
					CheckID: c.ID, Stage: c.Stage, StepID: c.StepID, ACKey: c.ACKey,
					State: CheckQuarantined, Detail: fmt.Sprintf("quarantined by %s until %s: %s", q.Owner, q.FixBy.Format("2006-01-02"), q.Reason),
				})
				res.Findings = append(res.Findings, Finding{
					Severity: SeverityNote, Category: CatCheckIntegrity,
					Criterion: "", Anchor: "check:" + c.ID,
					Text: fmt.Sprintf("check %q is quarantined (owner %s, fix by %s) — its verdict is missing from this pass", c.ID, q.Owner, q.FixBy.Format("2006-01-02")),
				})
			default:
				r := req
				r.Check = c
				out, err := runner.RunCheck(ctx, r)
				if err != nil {
					// Runner tool failure: recorded loudly; the check is
					// undecided — a screen outage escalates rather than
					// approves (Spec S07.2 principle, applied at V1).
					res.Checks = append(res.Checks, CheckOutcome{
						CheckID: c.ID, Stage: c.Stage, StepID: c.StepID, ACKey: c.ACKey,
						State: CheckRunnerFailed, Detail: err.Error(),
					})
					res.Findings = append(res.Findings, Finding{
						Severity: SeverityBlocker, Category: CatCheckIntegrity,
						Criterion: string(CatCheckIntegrity), Anchor: "check:" + c.ID,
						Text: fmt.Sprintf("check %q runner failure (not a verdict): %v", c.ID, err),
					})
					continue
				}
				state := CheckPassed
				if out.ExitCode != 0 {
					state = CheckFailed
					if failedStage < 0 || rank < failedStage {
						failedStage = rank
					}
					if firstFailure == "" {
						firstFailure = c.ID
					}
				}
				res.Checks = append(res.Checks, CheckOutcome{
					CheckID: c.ID, Stage: c.Stage, StepID: c.StepID, ACKey: c.ACKey,
					State: state, ExitCode: out.ExitCode,
					EvidenceRef: out.EvidenceRef, EvidenceSHA: out.EvidenceSHA,
				})
			}
		}
	}

	res.Steps = stepContracts(res.Checks, steps, firstFailure)
	return res, nil
}

// stepContracts decides every PLAN step's "Done when" contract from the
// check outcomes (Spec S07.3): PASS when all its checks passed, FAIL on any
// failure, UNVERIFIABLE-HERE when blocked upstream or undecidable, N-A when
// no check decides it mechanically (the judge weighs it at V2).
func stepContracts(checks []CheckOutcome, steps []intake.Step, firstFailure string) []StepContract {
	byStep := map[string][]CheckOutcome{}
	for _, c := range checks {
		if c.StepID != "" {
			byStep[c.StepID] = append(byStep[c.StepID], c)
		}
	}
	var out []StepContract
	for _, s := range steps {
		sc := StepContract{StepID: s.ID, DoneWhen: s.DoneWhen, State: ContractNA}
		outcomes := byStep[s.ID]
		if len(outcomes) > 0 {
			sc.State = ContractPass
			for _, c := range outcomes {
				sc.Category = checkCategory(c, sc.Category)
				switch c.State {
				case CheckFailed:
					sc.State = ContractFail
					sc.AttributedTo = c.CheckID
				case CheckUnverifiable, CheckQuarantined, CheckRunnerFailed:
					if sc.State != ContractFail {
						sc.State = ContractUnverifiable
						sc.AttributedTo = c.AttributedTo
						if sc.AttributedTo == "" {
							sc.AttributedTo = firstFailure
						}
					}
				}
			}
		}
		if sc.Category == "" {
			sc.Category = CatACBlocker
		}
		sc.Route = RouteTable[sc.Category].Sink
		out = append(out, sc)
	}
	return out
}

func checkCategory(c CheckOutcome, cur Category) Category {
	if cur != "" {
		return cur
	}
	switch c.State {
	case CheckQuarantined, CheckRunnerFailed:
		return CatCheckIntegrity
	default:
		return CatACBlocker
	}
}

// ---- Did-research-actually-run (1.9; Spec S07.3) ----

// ResearchUsage is the S10 metering seam this check reads: per-step
// search/research-tool invocation counters. The per-step counter substrate
// lands when stage sessions become real engine runs (B2-4+); until then
// implementations report known=false and the check records
// UNVERIFIABLE-HERE loudly — never a fake pass, never a false card.
type ResearchUsage interface {
	ResearchInvocations(ctx context.Context, taskID, stepID string) (n int64, known bool, err error)
}

// ResearchOutcome is one research node's check outcome.
type ResearchOutcome struct {
	Node        intake.ResearchNode `json:"node"`
	State       ContractState       `json:"state"`
	Invocations int64               `json:"invocations"`
	// RerunRequested asks the pipeline to re-run the node once in a fresh
	// session (⚙ verification.research_rerun_limit).
	RerunRequested bool `json:"rerun_requested,omitempty"`
	// NeedsCard: the rerun budget is exhausted and the node still shows
	// zero invocations → RESEARCH-NOT-RUN decision card (Spec S07.7).
	NeedsCard bool   `json:"needs_card,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

// CheckResearch runs the deterministic post-step check for every research
// node in the PLAN: ≥1 research-tool invocation required; zero → the node
// re-runs once (⚙ verification.research_rerun_limit), then a
// RESEARCH-NOT-RUN decision card — correctness on world-facts is never left
// to model initiative or memory (1.9; Spec S07.3). reruns carries the
// per-node re-runs already consumed.
func CheckResearch(ctx context.Context, usage ResearchUsage, taskID string, nodes []intake.ResearchNode, reruns map[string]int, settings Settings) ([]ResearchOutcome, error) {
	if len(nodes) == 0 {
		return nil, nil
	}
	limit, err := settings.Int(keyResearchRerun)
	if err != nil {
		return nil, fmt.Errorf("verify: read ⚙ %s: %w", keyResearchRerun, err)
	}
	var out []ResearchOutcome
	for _, node := range nodes {
		ro := ResearchOutcome{Node: node}
		if usage == nil {
			ro.State = ContractUnverifiable
			// The S10 per-step usage seam is not wired yet (B2-4); said plainly.
			ro.Detail = "the platform does not yet keep a per-step record of what was looked up, so this could not be checked either way — recorded, not silently passed"
			out = append(out, ro)
			continue
		}
		n, known, err := usage.ResearchInvocations(ctx, taskID, node.StepID)
		if err != nil {
			return nil, fmt.Errorf("verify: research counters for %s: %w", node.StepID, err)
		}
		switch {
		case !known:
			ro.State = ContractUnverifiable
			ro.Detail = "per-step usage counters unavailable for this step — recorded, not silently passed"
		case n >= 1:
			ro.State = ContractPass
			ro.Invocations = n
		case int64(reruns[node.RuleID]) < limit:
			ro.State = ContractFail
			ro.RerunRequested = true
			ro.Detail = fmt.Sprintf("zero research-tool invocations; re-run %d/%d requested (fresh session)", reruns[node.RuleID]+1, limit)
		default:
			ro.State = ContractFail
			ro.NeedsCard = true
			ro.Detail = "zero research-tool invocations after the re-run budget — RESEARCH-NOT-RUN"
		}
		out = append(out, ro)
	}
	return out, nil
}
