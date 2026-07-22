package stage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/sandbox"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/worker"
)

// helpers.go — the S04 spawn/helper slice landing at B3-3 (the B3 queue
// header reading: S04's spawn mechanics, helper topology, and the D6 shape
// build here where their named consumers land; the ≥50-item mass-mechanical
// fan-out stays parked post-v0 per S19.4).
//
// The D6 shape, structural (Spec S04.1, S04.3):
//   - one coordinator per task; helpers are isolated sibling engine
//     sessions with no shared conversational state — brief down, report up,
//     NOTHING else. The control plane provides no helper-to-helper channel:
//     no lateral verb exists on this API, and helper toolsets are refused
//     if they name the engine-native spawn/messaging family (the only
//     conceivable lateral vector on the lane).
//   - helpers WRITE NOTHING DURABLE themselves: a helper session holds no
//     ledger verbs, no DB reach, no artifact registry — its entire upward
//     surface is the report text; the PLATFORM screens it and lands it in
//     the coordinator's L0 (the run's ledger record: report file +
//     artifact ref + acceptance note), which is where the next stage
//     assembly reads it (Spec S04.3 "reports land"; verbatim relay — the
//     filed report is the helper's words, never a paraphrase).
//   - spawning is control-plane-owned, logged, never gated (Spec S04.2:
//     within budgets it needs no human approval); every spawn writes its
//     record BEFORE the session starts, and every refusal is a logged
//     event naming the failed check (Spec S04.4).
//
// Helper sessions ride the coordinator's RUN (checkpoints per paid call
// land on it — D7; receipts join through metering); helpers are NOT run
// rows: helper death is a scheduling event handled by SALVAGE (Spec S04.5),
// never the S02.5 recovery ladder, and a retry is a FRESH spawn
// (fork-don't-poison), never a fork.

// Spawn triggers — exactly three (Spec S04.2); anything matching none runs
// single-agent.
const (
	TriggerContext        = "T-CTX"  // context protection
	TriggerParallel       = "T-PAR"  // read fan-out (read-only facets)
	TriggerSpecialization = "T-SPEC" // tool/permission/model quarantine
)

// Helper lifecycle states (Spec S04.5).
const (
	HelperSpawned   = "SPAWNED"
	HelperRunning   = "RUNNING"
	HelperReturned  = "RETURNED"
	HelperAccepted  = "ACCEPTED"
	HelperRejected  = "REJECTED"
	HelperSalvaged  = "SALVAGED"
	HelperFailed    = "FAILED"
	HelperEscalated = "ESCALATED"
)

// Event types (provisional pending the S14 event contract at B5 — the
// standing CONVENTIONS §7/§8 naming note). The spawn record is an event-log
// row (Spec S04.4: "spawn records and lifecycle transitions are event-log
// rows in platform.db").
const (
	EventSpawn        = "orchestration.spawn"
	EventSpawnRefused = "orchestration.spawn_refused"
	EventHelper       = "orchestration.helper"
)

const orchestrationSchemaVersion = 1

// ⚙ keys (Spec S04.4 settings table; declared in the S18 registry since
// B0-2, consumed here by dotted key — CONVENTIONS §2).
const (
	keyDepthCap             = "orchestration.depth_cap"
	keyMaxConcurrentHelpers = "orchestration.max_concurrent_helpers"
	keyHelperTurns          = "orchestration.helper_turns"
	keyHelperTokens         = "orchestration.helper_tokens"
	keySpawnBudget          = "orchestration.spawn_budget"
	keyReportTokens         = "orchestration.report_tokens"
	keyBulkOffloadTokens    = "orchestration.bulk_offload_tokens"
	keyHelperRetryLimit     = "orchestration.helper_retry_limit"
)

// keyStaggerIdenticalPrefix (⚙ orchestration.stagger_identical_prefix) is
// DELIBERATELY UNCONSUMED at B3-3 — a named seam, never faked (the B2-4
// recitation precedent): staggering applies to identical-prefix helpers
// launched SIMULTANEOUSLY (Spec S04.3 cache-aware mechanics), and the v0
// spawn surface launches one helper per call — no simultaneous-launch
// batch exists until a fan-out consumer does (the parked ≥50 lane, or a
// live coordinator spawn channel). Consuming the key with no batch to
// stagger would fake the mechanism.
const keyStaggerIdenticalPrefix = "orchestration.stagger_identical_prefix"

// HelperBrief is the S04.3 v0 brief contract — every section MANDATORY
// (vague briefs are the measured failure mode). Context is the slice the
// PLATFORM draws from the Task Context Ledger — never raw coordinator
// history (helpers start with no inherited history, ever).
type HelperBrief struct {
	Objective      string   `json:"objective"`
	OutputContract string   `json:"output_contract"`
	ToolsSources   string   `json:"tools_sources"`
	Boundaries     string   `json:"boundaries"`
	Context        string   `json:"context"`
	Tools          []string `json:"tools,omitempty"`
	Class          string   `json:"class"` // confinement class (tighter-or-equal the coordinator's, P-T05-1)
}

func (b HelperBrief) validate() error {
	missing := []string{}
	for name, v := range map[string]string{
		"objective": b.Objective, "output_contract": b.OutputContract,
		"tools_sources": b.ToolsSources, "boundaries": b.Boundaries, "context": b.Context,
	} {
		if strings.TrimSpace(v) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("brief missing mandatory sections %v (S04.3)", missing)
	}
	return nil
}

// briefText renders the brief with its budget line — the helper's ENTIRE
// context (Spec S04.3).
func briefText(b HelperBrief, turns, tokens, reportTokens, bulkTokens int64) string {
	var sb strings.Builder
	sb.WriteString(stageMarker(markerHelper))
	sb.WriteString("HELPER BRIEF (v0 contract, Spec S04.3)\n")
	fmt.Fprintf(&sb, "objective: %s\n", b.Objective)
	fmt.Fprintf(&sb, "output_contract: %s\n", b.OutputContract)
	fmt.Fprintf(&sb, "  Report at most ~%d tokens with EXACTLY the sections FINDINGS / EVIDENCE (paths, URLs) / GAPS.\n", reportTokens)
	fmt.Fprintf(&sb, "  Bulk raw material (above ~%d tokens) goes to files in your working directory, referenced by path — never into the report.\n", bulkTokens)
	fmt.Fprintf(&sb, "tools_sources: %s\n", b.ToolsSources)
	fmt.Fprintf(&sb, "boundaries: %s\n  Read-only unless stated. At budget, STOP and report partial with the marker %q.\n", b.Boundaries, UnfinishedMarker)
	fmt.Fprintf(&sb, "budget: <= %d turns, <= %d tokens (this spawn's effective values)\n", turns, tokens)
	fmt.Fprintf(&sb, "context:\n%s\n", b.Context)
	return sb.String()
}

// UnfinishedMarker is the explicit partial-output marker (Spec S04.5
// SALVAGED: "partial output carried with an explicit unfinished marker —
// never silent loss").
const UnfinishedMarker = "[UNFINISHED — partial output]"

// markerHelper heads helper-session prompts (runner.go stageMarker frame).
const markerHelper = "helper"

// SpawnRequest is one control-plane spawn (Spec S04.2/S04.4). The spawn API
// is the ONLY way a helper comes to exist (Spec S04.1 structural
// enforcement).
type SpawnRequest struct {
	// RunID is the coordinator's run (the task's execute leg) the helper
	// serves; the spawn record, checkpoints, and the report all land on it.
	RunID string
	// ParentSpawn is "" when the coordinator spawns; a live spawn id when a
	// helper proposes a sub-helper (depth 2). Depth derives from it.
	ParentSpawn string
	// Trigger MUST be one of the exactly-three (Spec S04.2).
	Trigger string
	// Reason is the one human-readable line (7.7).
	Reason string
	Brief  HelperBrief
	// Mechanical marks a mechanical helper duty (local-lane default, Spec
	// S08.8 step 5 — degraded to the paid seat until B4 with the reason
	// recorded).
	Mechanical bool
	// EscalateOnFailure routes the second failure to ESCALATED (a durable
	// human ask) instead of FAILED (Spec S04.5: "second failure → FAILED or
	// ESCALATED"). Either way the gap is never silent.
	EscalateOnFailure bool
}

// SpawnResult is one terminal spawn outcome. Err is nil even for
// FAILED/SALVAGED outcomes — a helper's death NEVER fails the caller
// (sibling-failure containment, Spec S04.5); the coordinator proceeds with
// what it has.
type SpawnResult struct {
	SpawnID string
	Outcome string // ACCEPTED | SALVAGED | FAILED | ESCALATED
	// Report is the accepted (or salvaged-partial) report text — the
	// helper's words verbatim (Spec S04.3 relay-verbatim).
	Report string
	// ReportPath is the durable per-spawn report file in the task
	// workspace ([coordinator-draft] per-spawn subpath).
	ReportPath string
	// Annotations are advisory instruction-pattern screen hits (report-as-
	// data, Spec S04.3: they annotate, never reword).
	Annotations []string
	// Depth, Model, Lane document what ran (the spawn record has the full
	// set).
	Depth int
	Model string
	Lane  string
}

// helperCounter tracks live helpers per coordinator run across all depths
// (⚙ orchestration.max_concurrent_helpers "counting all live helpers of
// the task across depths") plus the process-lifetime spawn-lineage
// registry (spawn id → depth) sub-helper proposals resolve against. Both
// are in-process state: helper sessions die with the process (death =
// salvage, S04.5), and a sub-helper is proposed by its live parent.
type helperCounter struct {
	mu    sync.Mutex
	live  map[string]int64
	depth map[string]int
}

func (h *helperCounter) tryAcquire(runID string, max int64) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.live == nil {
		h.live = map[string]int64{}
	}
	if h.live[runID] >= max {
		return false
	}
	h.live[runID]++
	return true
}

func (h *helperCounter) release(runID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.live[runID] > 0 {
		h.live[runID]--
	}
}

func (h *helperCounter) registerDepth(spawnID string, d int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.depth == nil {
		h.depth = map[string]int{}
	}
	h.depth[spawnID] = d
}

func (h *helperCounter) parentDepth(spawnID string) (int, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	d, ok := h.depth[spawnID]
	return d, ok
}

// spawnRecord is the S04.4 spawn-log row (event payload; OTel-mappable
// shape — Sinet defines its own schema, R06 §2.5).
type spawnRecord struct {
	SpawnID       string `json:"spawn_id"`
	TS            string `json:"ts"`
	TaskID        string `json:"task_id"`
	Owner         string `json:"owner"`
	ParentSession string `json:"parent_session"` // "coordinator" or the parent spawn id
	ChildSession  string `json:"child_session"`  // the helper session's stage name
	Depth         int    `json:"depth"`
	Trigger       string `json:"trigger"`
	Reason        string `json:"reason"`
	BriefHash     string `json:"brief_hash"`
	Model         string `json:"model"`
	Lane          string `json:"lane"`
	Budget        struct {
		Turns  int64 `json:"turns"`
		Tokens int64 `json:"tokens"`
	} `json:"budget"`
	Retry int `json:"retry,omitempty"` // 0 = first spawn; a retry is a FRESH spawn record
}

// Spawn validates, records, routes, runs, screens, and lands one helper —
// the whole S04.5 lifecycle, containment included. It returns an error ONLY
// for platform defects (a bad request is a logged refusal + error to the
// proposer; a helper failure is a terminal outcome, never an error).
func (s *Skeleton) Spawn(ctx context.Context, req SpawnRequest) (SpawnResult, error) {
	r, err := s.cfg.Runs.Get(ctx, req.RunID)
	if err != nil {
		return SpawnResult{}, err
	}

	// ── Validation (Spec S04.2/S04.4) — every refusal is a logged event
	// naming the failed check (refusals are evidence).
	depthCap, err := s.cfg.Settings.Int(keyDepthCap)
	if err != nil {
		return SpawnResult{}, fmt.Errorf("stage: read ⚙ %s: %w", keyDepthCap, err)
	}
	maxHelpers, err := s.cfg.Settings.Int(keyMaxConcurrentHelpers)
	if err != nil {
		return SpawnResult{}, fmt.Errorf("stage: read ⚙ %s: %w", keyMaxConcurrentHelpers, err)
	}
	spawnBudget, err := s.cfg.Settings.Int(keySpawnBudget)
	if err != nil {
		return SpawnResult{}, fmt.Errorf("stage: read ⚙ %s: %w", keySpawnBudget, err)
	}

	refuse := func(check, detail string) (SpawnResult, error) {
		s.refuseSpawn(ctx, r, req, check, detail)
		return SpawnResult{}, fmt.Errorf("stage: spawn refused (%s): %s", check, detail)
	}

	switch req.Trigger {
	case TriggerContext, TriggerParallel, TriggerSpecialization:
	default:
		return refuse("trigger", fmt.Sprintf("trigger %q is not one of the exactly-three (S04.2)", req.Trigger))
	}
	if strings.TrimSpace(req.Reason) == "" {
		return refuse("reason", "a spawn needs its one-line reason (D6; 7.7)")
	}
	if err := req.Brief.validate(); err != nil {
		return refuse("brief", err.Error())
	}
	// Depth derives from the durable-in-process spawn lineage: helpers run
	// at depth 1; a sub-helper proposed through the same API runs at its
	// parent's depth + 1 (Spec S04.1) — a depth over ⚙ cap is refused.
	depth := 1
	if req.ParentSpawn != "" {
		pd, ok := s.helpers.parentDepth(req.ParentSpawn)
		if !ok {
			return refuse("parent", fmt.Sprintf("unknown parent spawn %q (sub-helpers are proposed by their live parent)", req.ParentSpawn))
		}
		depth = pd + 1
	}
	if int64(depth) > depthCap {
		return refuse("depth", fmt.Sprintf("depth %d > ⚙ %s = %d (S04.1: depth 1 is the operating norm)", depth, keyDepthCap, depthCap))
	}
	for _, tool := range req.Brief.Tools {
		if isNativeSpawnTool(tool) {
			// The engine-native spawn family is the sole-controller
			// exclusion (G1 rider 2, S03.5) AND the only conceivable
			// helper-to-helper channel on the lane — no lateral edge exists
			// (Spec S04.1: the control plane provides NO lateral channel;
			// the conformance battery asserts this refusal).
			return refuse("lateral-or-native-spawn", fmt.Sprintf("tool %q is engine-native spawn (S03.5 sole-controller; D6 no-lateral)", tool))
		}
	}
	// Helper confinement: inherit-and-tighten only (P-T05-1; the S11
	// per-axis check). The coordinator envelope at v0 is the execute leg's
	// class posture; helpers declare their class in the brief.
	coordClass := sandbox.C2 // the execute leg's loosest v0 class
	helperClass := sandbox.Class(req.Brief.Class)
	if err := sandbox.AdmitHelperClass(
		sandbox.Confinement{Class: coordClass, WorkspaceMode: "rw"},
		confinementFor(helperClass),
	); err != nil {
		return refuse("confinement", err.Error())
	}
	// Spawn budget: per task, INCLUDING sub-helpers and retries (a retry is
	// a fresh spawn) — counted from the durable spawn records.
	spawned, err := s.countSpawns(ctx, r.TaskID)
	if err != nil {
		return SpawnResult{}, err
	}
	if spawned >= spawnBudget {
		// Overrun happens only through an operator-visible gate (Spec
		// S04.4) — the refusal is the budget stop; the overrun approval
		// card belongs to the surface that grants budget raises (⚙
		// ceilings, S18/G1 rider 1).
		return refuse("spawn-budget", fmt.Sprintf("spawn budget exhausted: %d/%d (⚙ %s; overrun only via an operator-visible gate)", spawned, spawnBudget, keySpawnBudget))
	}
	if !s.helpers.tryAcquire(req.RunID, maxHelpers) {
		return refuse("concurrency", fmt.Sprintf("live helpers at ⚙ %s = %d", keyMaxConcurrentHelpers, maxHelpers))
	}
	defer s.helpers.release(req.RunID)

	// ── Routing: helpers ride the SAME S08.8 pipeline with the spawn
	// trigger as an input (Spec S08.8 step 5; S04.1 boundary "S08 owns
	// which worker/model/lane a helper runs on").
	decision := s.routeHelper(ctx, r, req)
	if err := s.emitRoutingDecided(ctx, r, executeRouting{Decision: decision}); err != nil {
		return SpawnResult{}, err
	}

	return s.runHelper(ctx, r, req, decision, depth, 0)
}

// routeHelper resolves the helper's worker/model/lane through the S08.8
// router (generalist duty-map default in the no-router test posture,
// loudly logged — never silent).
func (s *Skeleton) routeHelper(ctx context.Context, r run.Run, req SpawnRequest) worker.Decision {
	if s.router != nil {
		d, err := s.router.Route(ctx, worker.RouteQuery{
			Requester: r.UserID,
			TaskID:    r.TaskID,
			// The consuming run of any tie-break D7 write is the coordinator's
			// execute run r.ID — the intake run may be terminal at helper-spawn
			// time (drain F2; D6/§19).
			RunID:        r.ID,
			TaskText:     req.Brief.Objective,
			Kind:         worker.KindAgentic,
			Classes:      []string{req.Brief.Class},
			Tools:        req.Brief.Tools,
			SpawnTrigger: req.Trigger,
			Mechanical:   req.Mechanical,
		})
		if err == nil {
			d.Cause = "helper-spawn"
			return d
		}
		s.logger().Error("stage: helper routing failed; duty-map generalist fallback", "err", err)
	} else {
		s.logger().Warn("stage: helper spawn without a router (test-only posture) — duty-map generalist")
	}
	seat := s.seat(worker.DutyExecution)
	model := seat.Model
	if s.cfg.Model != "" {
		model = s.cfg.Model
	}
	reason := "Helper on the generalist execution seat."
	if req.Mechanical {
		reason = "Mechanical helper duty prefers the local free tier; its engine lane carries no v0 consumer (S12.1 class (a), B4-5), so it rides the paid execution seat. " + reason
	}
	return worker.Decision{
		Cause: "helper-spawn", Generalist: true, Model: model, Lane: seat.Lane,
		WindowTokens: seat.WindowTokens, PlainReason: reason,
		Signals: []string{"spawn_trigger=" + req.Trigger},
	}
}

// runHelper executes one spawn attempt to a terminal outcome (retry
// recursion is a FRESH spawn record — fork-don't-poison, Spec S04.5).
func (s *Skeleton) runHelper(ctx context.Context, r run.Run, req SpawnRequest, decision worker.Decision, depth, retry int) (SpawnResult, error) {
	turns, err := s.cfg.Settings.Int(keyHelperTurns)
	if err != nil {
		return SpawnResult{}, fmt.Errorf("stage: read ⚙ %s: %w", keyHelperTurns, err)
	}
	tokens, err := s.cfg.Settings.Int(keyHelperTokens)
	if err != nil {
		return SpawnResult{}, fmt.Errorf("stage: read ⚙ %s: %w", keyHelperTokens, err)
	}
	reportTokens, err := s.cfg.Settings.Int(keyReportTokens)
	if err != nil {
		return SpawnResult{}, fmt.Errorf("stage: read ⚙ %s: %w", keyReportTokens, err)
	}
	bulkTokens, err := s.cfg.Settings.Int(keyBulkOffloadTokens)
	if err != nil {
		return SpawnResult{}, fmt.Errorf("stage: read ⚙ %s: %w", keyBulkOffloadTokens, err)
	}
	retryLimit, err := s.cfg.Settings.Int(keyHelperRetryLimit)
	if err != nil {
		return SpawnResult{}, fmt.Errorf("stage: read ⚙ %s: %w", keyHelperRetryLimit, err)
	}

	spawnID := newSpawnID()
	brief := briefText(req.Brief, turns, tokens, reportTokens, bulkTokens)
	sum := sha256.Sum256([]byte(brief))

	parent := req.ParentSpawn
	if parent == "" {
		parent = "coordinator"
	}
	rec := spawnRecord{
		SpawnID: spawnID, TS: rfcNow(s), TaskID: r.TaskID, Owner: r.UserID,
		ParentSession: parent, ChildSession: "helper-" + spawnID, Depth: depth,
		Trigger: req.Trigger, Reason: req.Reason, BriefHash: hex.EncodeToString(sum[:]),
		Model: decision.Model, Lane: decision.Lane, Retry: retry,
	}
	rec.Budget.Turns, rec.Budget.Tokens = turns, tokens

	// The spawn record is written BEFORE the helper session starts — no
	// code path creates a helper without one (Spec S04.4).
	if err := s.appendOrch(ctx, r, EventSpawn, rec); err != nil {
		return SpawnResult{}, err
	}
	s.helpers.registerDepth(spawnID, depth)
	s.helperState(ctx, r, spawnID, HelperSpawned, "")
	s.helperState(ctx, r, spawnID, HelperRunning, "")

	res, sessErr := s.Session(ctx, SessionInput{
		RunID:        r.ID,
		Stage:        rec.ChildSession,
		Assemble:     false, // the brief IS the whole context — no coordinator history, ever (S04.3)
		Instructions: brief,
		Class:        req.Brief.Class,
		Tools:        req.Brief.Tools,
		Model:        decision.Model,
		WindowTokens: decision.WindowTokens,
	})

	if sessErr != nil {
		// Budget exhaustion, engine death, mid-run kill: SALVAGE — the
		// partial output is carried with the explicit unfinished marker,
		// never silent loss; sibling helpers and the task are untouched
		// (Spec S04.5 containment; helper death costs a resume, never spent
		// tokens — checkpoints per paid call already landed via the
		// Driver).
		partial := strings.TrimSpace(res.Text)
		marked := UnfinishedMarker + "\n" + partial
		path, annotations := s.landReport(ctx, r, spawnID, marked, "salvaged partial (screened AS partial, S04.5)")
		s.helperState(ctx, r, spawnID, HelperSalvaged, sessErr.Error())
		return SpawnResult{SpawnID: spawnID, Outcome: HelperSalvaged, Report: marked,
			ReportPath: path, Annotations: annotations, Depth: depth, Model: decision.Model, Lane: decision.Lane}, nil
	}

	s.helperState(ctx, r, spawnID, HelperReturned, "")

	// Acceptance screen — conformance-only at v0 (G1 Def.9 as amended; G3
	// Def.3): form, not truth. A local plausibility screen is NOT part of
	// v0; content correctness is S07's at the deliverable level.
	verdictErr := screenReport(res.Text, reportTokens)
	if verdictErr == nil {
		path, annotations := s.landReport(ctx, r, spawnID, res.Text, "accepted (conformance screen passed)")
		s.helperState(ctx, r, spawnID, HelperAccepted, "")
		return SpawnResult{SpawnID: spawnID, Outcome: HelperAccepted, Report: res.Text,
			ReportPath: path, Annotations: annotations, Depth: depth, Model: decision.Model, Lane: decision.Lane}, nil
	}

	s.helperState(ctx, r, spawnID, HelperRejected, verdictErr.Error())
	if int64(retry) < retryLimit {
		// Retry as a FRESH helper with a revised brief (fork-don't-poison):
		// a new spawn record, the rejection folded into boundaries — never
		// a continued session.
		revised := req
		revised.Brief.Boundaries += fmt.Sprintf(" PRIOR ATTEMPT REJECTED by the acceptance screen: %s. Fix exactly that.", verdictErr)
		return s.runHelper(ctx, r, revised, decision, depth, retry+1)
	}

	// Second failure → FAILED or ESCALATED (Spec S04.5). Either way the
	// dropped facet is recorded in the coordinator's own record — never
	// silent (1.4 spirit).
	if req.EscalateOnFailure {
		s.escalateHelper(ctx, r, spawnID, req, verdictErr)
		s.helperState(ctx, r, spawnID, HelperEscalated, verdictErr.Error())
		return SpawnResult{SpawnID: spawnID, Outcome: HelperEscalated, Depth: depth,
			Model: decision.Model, Lane: decision.Lane}, nil
	}
	s.recordFailedFacet(ctx, r, spawnID, req, verdictErr)
	s.helperState(ctx, r, spawnID, HelperFailed, verdictErr.Error())
	return SpawnResult{SpawnID: spawnID, Outcome: HelperFailed, Depth: depth,
		Model: decision.Model, Lane: decision.Lane}, nil
}

// screenReport is the v0 acceptance screen (Spec S04.5): mandatory sections
// present, size within ⚙ orchestration.report_tokens (the structural
// bytes/4 estimate — the §17 precedent), unfinished marker consistent with
// the exit path (a clean return must not claim to be a salvage partial).
// Boundary respect is STRUCTURAL, not screened: the toolset and confinement
// class enforced them during the session.
func screenReport(text string, reportTokens int64) error {
	up := strings.ToUpper(text)
	for _, section := range []string{"FINDINGS", "EVIDENCE", "GAPS"} {
		if !strings.Contains(up, section) {
			return fmt.Errorf("report missing mandatory section %s (S04.3 contract)", section)
		}
	}
	if est := int64(len(text)) / 4; est > reportTokens {
		return fmt.Errorf("report ~%d tokens exceeds ⚙ %s = %d (bulk belongs in workspace files by path)", est, keyReportTokens, reportTokens)
	}
	if strings.Contains(text, UnfinishedMarker) {
		return fmt.Errorf("clean return carries the unfinished marker — inconsistent with the exit path (S04.5)")
	}
	return nil
}

// landReport writes the report to the task workspace under the per-spawn
// subpath and registers it in the coordinator's L0 — the run's ledger
// record: artifact ref + acceptance note (Spec S04.3 "reports land in the
// coordinator's L0"; the PLATFORM writes, the helper never does). The
// instruction-pattern screen runs report-as-data: hits ANNOTATE (marked in
// the note), never reword (Spec S04.3; advisory by design).
func (s *Skeleton) landReport(ctx context.Context, r run.Run, spawnID, text, disposition string) (string, []string) {
	var annotations []string
	for _, f := range worker.ScreenInstructions("helper report "+spawnID, text) {
		annotations = append(annotations, f.Message)
	}

	dir := filepath.Join(s.cfg.RunRoot, sanitizePathComponent(r.ID), "cwd", "helpers", sanitizePathComponent(spawnID))
	path := filepath.Join(dir, "report.md")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		s.logger().Error("stage: helper report dir", "err", err)
		return "", annotations
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		s.logger().Error("stage: write helper report", "err", err)
		return "", annotations
	}
	sum := sha256.Sum256([]byte(text))

	verbs := s.cfg.Ledger.SessionVerbs(r.ID, "helper-"+spawnID, r.Generation)
	desc := fmt.Sprintf("helper report %s: %s (S04.3; report-as-data)", spawnID, disposition)
	if len(annotations) > 0 {
		desc += fmt.Sprintf("; instruction-pattern annotations: %s", strings.Join(annotations, " | "))
	}
	if _, err := verbs.Artifact(ctx, path, "helper-report", desc, hex.EncodeToString(sum[:])); err != nil {
		s.logger().Error("stage: register helper report", "run", r.ID, "err", err)
	}
	return path, annotations
}

// recordFailedFacet records a FAILED helper's gap in the coordinator's own
// output record (Spec S04.5: a dropped facet is never silent).
func (s *Skeleton) recordFailedFacet(ctx context.Context, r run.Run, spawnID string, req SpawnRequest, cause error) {
	if _, err := s.cfg.Ledger.RecordDecision(ctx, r.ID, ledger.AuthorPlatform, run.ActorPlatform, "helper-"+spawnID,
		fmt.Sprintf("helper facet ABANDONED (spawn %s, trigger %s): %s — objective was: %s", spawnID, req.Trigger, cause, req.Brief.Objective),
		"S04.5 FAILED: the gap is recorded in the coordinator's own output", 0); err != nil {
		s.logger().Error("stage: record failed facet", "run", r.ID, "err", err)
	}
}

// escalateHelper opens the durable human ask for an unresolvable helper
// (Spec S04.5 ESCALATED: "the escalation route is test-proven, because a
// finding that dies in a log is a platform defect").
func (s *Skeleton) escalateHelper(ctx context.Context, r run.Run, spawnID string, req SpawnRequest, cause error) {
	askID := "ask-helper-" + spawnID
	snapshot, err := json.Marshal(map[string]any{
		"kind": "helper-escalation", "spawn_id": spawnID, "task_id": r.TaskID,
		"trigger": req.Trigger, "reason": req.Reason, "objective": req.Brief.Objective,
		"cause": cause.Error(),
	})
	if err != nil {
		s.logger().Error("stage: marshal helper escalation", "err", err)
		return
	}
	err = s.cfg.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO asks (ask_id, run_id, user_id, snapshot, status, observed_ts)
			 VALUES (?, ?, ?, ?, 'open', ?)`,
			askID, r.ID, r.UserID, string(snapshot), rfcNow(s)); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		s.logger().Error("stage: helper escalation ask", "ask", askID, "err", err)
	}
}

// helperState appends one lifecycle transition (Spec S04.5: every
// transition a logged event).
func (s *Skeleton) helperState(ctx context.Context, r run.Run, spawnID, state, detail string) {
	if err := s.appendOrch(ctx, r, EventHelper, map[string]any{
		"spawn_id": spawnID, "state": state, "detail": detail,
	}); err != nil {
		s.logger().Error("stage: helper lifecycle event", "spawn", spawnID, "state", state, "err", err)
	}
}

func (s *Skeleton) refuseSpawn(ctx context.Context, r run.Run, req SpawnRequest, check, detail string) {
	if err := s.appendOrch(ctx, r, EventSpawnRefused, map[string]any{
		"check": check, "detail": detail, "trigger": req.Trigger, "reason": req.Reason,
		"parent": req.ParentSpawn,
	}); err != nil {
		s.logger().Error("stage: spawn refusal event", "check", check, "err", err)
	}
}

func (s *Skeleton) appendOrch(ctx context.Context, r run.Run, typ string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("stage: marshal %s: %w", typ, err)
	}
	if _, err := s.cfg.Log.Append(ctx, eventlog.Append{
		RunID: r.ID, Generation: r.Generation, UserID: r.UserID,
		Type: typ, SchemaVersion: orchestrationSchemaVersion, Payload: raw,
	}); err != nil {
		return fmt.Errorf("stage: append %s: %w", typ, err)
	}
	return nil
}

// countSpawns counts the task's durable spawn records across its runs
// (spawn budget is per TASK including sub-helpers and retries, Spec S04.4).
func (s *Skeleton) countSpawns(ctx context.Context, taskID string) (int64, error) {
	var n int64
	err := s.cfg.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM run_events WHERE type = ? AND run_id IN (SELECT run_id FROM runs WHERE task_id = ?)`,
		EventSpawn, taskID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("stage: count spawns: %w", err)
	}
	return n, nil
}

// confinementFor builds the helper's declared confinement for the P-T05-1
// per-axis check (S11 owns the mechanics; which class attaches is policy).
func confinementFor(c sandbox.Class) sandbox.Confinement {
	conf := sandbox.Confinement{Class: c}
	if c == sandbox.C2 {
		conf.WorkspaceMode = "rw"
	} else if c == sandbox.C1 {
		conf.WorkspaceMode = "ro"
	}
	return conf
}

func isNativeSpawnTool(tool string) bool {
	switch tool {
	case "Task", "TaskCreate", "TaskGet", "TaskList", "TaskOutput", "TaskStop", "TaskUpdate":
		return true
	}
	return false
}

func rfcNow(s *Skeleton) string { return s.now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00") }

func newSpawnID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("stage: crypto/rand: %v", err))
	}
	return "sp-" + hex.EncodeToString(b[:])
}
