package stage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/worker"
)

// SessionInput names one stage engine session on an already-RUNNING run
// (Spec S05.3 fresh-context-per-stage).
type SessionInput struct {
	RunID string
	// Stage names the pipeline stage; for execute sessions it is the plan
	// step id, so the ledger assembly's Plan source injects that step's
	// contract (Spec S05.4 Sources.Plan; intake.PlanSource selection).
	Stage string

	// Assemble builds the stage brief through the ledger (Spec S05.4).
	// When false the prompt is Instructions alone — the artifact-only
	// sessions (Stage-3 critique, Spec S06.8: "the signature admits
	// nothing but the pair"; the S07.6 retry package: "exactly" its four
	// members) whose input contracts EXCLUDE the ledger projection.
	Assemble bool
	Clean    bool
	// LedgerVersion pins the assembly to a revision (recovery path); 0 =
	// current.
	LedgerVersion int64
	Sources       ledger.Sources
	Extra         []ledger.Item

	// PlaceBrief places an ALREADY-assembled brief's pinned body for the
	// SessionStart re-injection channel without re-assembling (the judge
	// path: verify.BuildJudgeInput ran the clean-context assembly and its
	// manifest already — Spec S07.5/S05.4). Ignored when Assemble is true.
	PlaceBrief *ledger.Brief

	// Instructions is the stage instruction block appended after the
	// assembled brief (or the whole prompt when Assemble is false). It
	// starts with the stage marker line (stageMarker) so the session's
	// duty is explicit in the prompt frame.
	Instructions string

	// Class is the confinement class compiled for this session (Spec
	// S11.6); Tools the default-deny allowlist (empty = tool-less).
	Class string
	Tools []string

	// PermissionMode is the engine-side permission posture for the session
	// ("" = the lane default, under which a headless session's tool
	// permission prompts auto-deny). Part of the S03.4 invocation snapshot,
	// lowered by the adapter. This is the cooperation layer only — the
	// enforcement boundary is the confinement class (Spec S11.1), never
	// engine consent.
	PermissionMode string

	// Model is the session model ("" = the planning duty seat — ceremony
	// sessions; execute sessions receive the S08.8 selection explicitly).
	Model string

	// WindowTokens is the selected model's context-window record (the
	// worker.Seat row fact the stage-fit budget measures against, Spec
	// S05.3); 0 = the duty-map default.
	WindowTokens int64

	// Compiled is the S08.3 per-invocation compiled unit for worker-backed
	// sessions (CompileForRun output): its CompiledWorker carries the agent
	// definition, granted toolset, gated tools and permission mode; the
	// stage brief still arrives through Prompt assembly (Spec S08.3: the
	// instance's L0 reaches the engine through the S05 stage brief, not the
	// agent definition). Nil = the generalist path (Tools/PermissionMode
	// above apply).
	Compiled *worker.Compiled

	// PriorOverflows counts context.overflow proposals earlier sessions of
	// this PLANNED stage already raised: a crossing with a prior overflow
	// escalates to a re-plan proposal (Spec S05.3 "a second overflow
	// within one planned stage").
	PriorOverflows int
}

// SessionResult is one completed stage session.
type SessionResult struct {
	Outcome adapters.Outcome
	// Text is the session's full final result text (the parse input for
	// structured stage output).
	Text string
	// Brief is the assembled stage brief (zero value when !Assemble).
	Brief ledger.Brief
	// Budget is the watcher's session summary.
	Budget BudgetReport
	// Split: the session ended at a checkpoint boundary because an
	// auto-accepted stage-split proposal executed (Spec S05.3) — not a
	// completion and not an error. The caller consolidates to the ledger
	// and continues the planned stage in a successor sub-stage session; the
	// engine park record is deliberately DISCARDED: the successor is a
	// fresh brief from the updated ledger, never a resumed transcript
	// (Spec S05.3 fresh-context rule, G1 D1.1).
	Split bool
}

// ErrStageSession reports a stage session that ended short of completion.
type stageError struct {
	Kind   adapters.OutcomeKind
	Detail string
}

func (e *stageError) Error() string {
	return fmt.Sprintf("stage: session ended %s: %s", e.Kind, e.Detail)
}

// stageMarker heads every stage-session prompt with the session's duty —
// the stable prompt frame stage instructions hang off (and the marker the
// dev-mode fake engine keys its canned streams on).
func stageMarker(kind string) string { return "SINET-STAGE: " + kind + "\n" }

// Session runs one fresh stage engine session and returns its result. The
// run must be running (DriveStage enforces it); paid calls checkpoint
// against the run with the live ledger-revision block (Spec S02.4c via
// Driver.LedgerRevision).
func (s *Skeleton) Session(ctx context.Context, in SessionInput) (SessionResult, error) {
	r, err := s.cfg.Runs.Get(ctx, in.RunID)
	if err != nil {
		return SessionResult{}, err
	}
	// Substrate: the run's own where set (execute/verify runs, created by
	// the skeleton); the configured ceremony substrate otherwise — the
	// intake run predates engine selection (Spec S06.10 puts the ceremony
	// duty map on the platform, per-person overrides at S08/B3).
	substrate := r.Substrate
	if substrate == "" {
		substrate = s.cfg.Substrate
	}
	adapter, ok := s.cfg.Adapters[substrate]
	if !ok {
		return SessionResult{}, fmt.Errorf("stage: no adapter registered for substrate %q (run %s)", substrate, r.ID)
	}

	prompt := in.Instructions
	var brief ledger.Brief
	sessionStartPath := ""
	workDir, cwd, err := s.runDirs(in.RunID, in.Stage)
	if err != nil {
		return SessionResult{}, err
	}
	// Repo-backed runs work in the project worktree (Spec S02.10/S13.5, R34):
	// the plain run cwd is replaced by the run-branch worktree of the
	// registered project store; a non-project run keeps the plain dir. The
	// platform-owned work dir (lowered config + ctl dir) stays separate
	// (S03.5). Idempotent: the seam creates the worktree once and returns it.
	if s.cfg.WorkspaceCwd != nil {
		if wpath, ok, werr := s.cfg.WorkspaceCwd(ctx, in.RunID); werr == nil && ok && wpath != "" {
			cwd = wpath
		} else if werr != nil {
			s.logger().Warn("stage: project workspace unavailable; using the plain run dir",
				"run", in.RunID, "err", werr)
		}
	}
	if in.Assemble {
		brief, err = s.cfg.Ledger.Assemble(ctx, ledger.AssembleInput{
			RunID:         in.RunID,
			Stage:         in.Stage,
			Clean:         in.Clean,
			LedgerVersion: in.LedgerVersion,
			Sources:       in.Sources,
			Extra:         in.Extra,
		})
		if err != nil {
			return SessionResult{}, fmt.Errorf("stage: assemble brief: %w", err)
		}
		// Pinned survival across compaction/resume: the platform-placed
		// pinned-context file + the SessionStart re-injection hook (Spec
		// S05.4/S05.7 step 3; B1-4 spike channel).
		path, _, err := ledger.PlacePinned(workDir, brief)
		if err != nil {
			return SessionResult{}, err
		}
		sessionStartPath = path
		prompt = ledger.BriefText(brief) + "\n" + in.Instructions
	} else if in.PlaceBrief != nil {
		brief = *in.PlaceBrief
		path, _, err := ledger.PlacePinned(workDir, brief)
		if err != nil {
			return SessionResult{}, err
		}
		sessionStartPath = path
	}

	watcher := s.newBudgetWatcher(ctx, r, in.Stage, in.PriorOverflows, in.WindowTokens)
	// Stage-split execution (Spec S05.3): a first overflow's stage-split
	// proposal is auto-accepted (risk-free platform-internal scheduling,
	// fully logged by the context.overflow event) and executes at the next
	// checkpoint boundary — the watcher observes the persisted usage event
	// (the checkpoint's own boundary) and requests the S03.1 pause verb, an
	// interrupt at the engine's next safe message boundary (never
	// mid-tool-call, P-T01-4). Only ledger-backed working sessions split
	// (Assemble && !Clean): artifact-only and clean-context sessions have
	// input contracts that exclude the ledger projection (Spec S06.8/S07.5–
	// S07.6, CONVENTIONS §14/§15), so no successor brief could carry their
	// consolidated state — for them the overflow event stays a proposal
	// record.
	split := &splitRequester{s: s, runID: in.RunID, stage: in.Stage}
	if in.Assemble && !in.Clean {
		watcher.onStageSplit = split.request
	}
	cw := adapters.CompiledWorker{
		Prompt:                  prompt,
		ToolAllowlist:           in.Tools,
		PermissionMode:          in.PermissionMode,
		SessionStartContextPath: sessionStartPath,
	}
	ceilUSD, ceilSteps := r.CeilingCostUSD, r.CeilingSteps
	if in.Compiled != nil {
		// Worker-backed session (Spec S08.3): the compiled unit IS the
		// enforcement surface — agent definition, granted toolset, gated
		// tools, permission mode all recompiled from control-plane rows;
		// the stage brief rides Prompt assembly on top.
		cw = in.Compiled.Worker
		cw.Prompt = prompt
		cw.SessionStartContextPath = sessionStartPath
		ceilUSD = tighterCeilingF(ceilUSD, in.Compiled.CeilingCostUSD)
		ceilSteps = tighterCeilingI(ceilSteps, in.Compiled.CeilingSteps)
		// The per-spawn compiled unit is recorded on the run (Spec S08.3
		// "recorded on the run"; the version→outcome join's config half).
		s.recordCompiled(ctx, r, in.Stage, in.Compiled)
	}
	// Recitation (Spec S05.3; Research/18 §7-C1): ledger-backed working
	// sessions get the per-turn channel — the reciter computes dueness and
	// authors, the compiled PostToolUse valve delivers (recite.go).
	recite := s.newReciter(ctx, in, brief, workDir, cw.ToolAllowlist)
	cw.Recitation = recite != nil
	onEvent := watcher.observe
	if recite != nil {
		onEvent = func(ev adapters.Event) {
			watcher.observe(ev)
			recite.observe(ev)
		}
	}
	req := adapters.StartRequest{
		RunID:          in.RunID,
		UserID:         r.UserID,
		Worker:         cw,
		Model:          s.modelFor(in.Model),
		OwnerCredRef:   s.ownerCredRef(ctx, r.UserID),
		Class:          in.Class,
		Confiner:       s.cfg.Confiner,
		Cwd:            cwd,
		WorkDir:        workDir,
		CeilingCostUSD: ceilUSD,
		CeilingSteps:   ceilSteps,
		OnEvent:        onEvent,
		OnSession:      split.capture,
	}
	if s.cfg.CredInject != nil {
		req.CredInject = s.cfg.CredInject(r.UserID)
	}

	out, err := s.driver.DriveStage(ctx, adapter, req)
	if err != nil {
		return SessionResult{}, err
	}
	// Recitation deliveries become manifest events for EVERY session
	// disposition — a delivery into a session that then failed still
	// happened and must not vanish from the trace (§7-C1 condition 3).
	if recite != nil {
		s.recordRecitations(ctx, in)
	}
	res := SessionResult{Outcome: out, Text: out.ResultText, Brief: brief, Budget: watcher.report()}
	switch {
	case split.requested && out.Kind == adapters.OutcomeParked && out.Ask == nil:
		// The stage split executed: the boundary interrupt ended the session
		// (Spec S05.3 "end session"); the caller consolidates and continues
		// in a successor sub-stage. The park record is discarded — a
		// resumed transcript is never the mechanism for crossing a stage
		// boundary (Spec S05.3, G1 D1.1).
		res.Split = true
		s.logger().Info("stage: stage split executed at checkpoint boundary",
			"run", in.RunID, "stage", in.Stage, "overflow_seq", res.Budget.OverflowSeq)
		// Mid-session pinned re-injections that happened before the split
		// still happened — manifested like every injection (Spec S05.4).
		s.recordReinjections(ctx, in, brief, workDir)
		return res, nil
	case out.Kind != adapters.OutcomeCompleted:
		return res, &stageError{Kind: out.Kind, Detail: out.Detail}
	case split.requested:
		// The pause raced completion and lost (the adapter's race rule):
		// the engine finished the stage first, so the split is moot — the
		// overflow event remains the proposal record.
		s.logger().Info("stage: stage-split pause raced completion; result stands",
			"run", in.RunID, "stage", in.Stage)
	}
	// Mid-stage pinned re-injections (SessionStart fires on resume/compact)
	// become manifest entries (Spec S05.4); best-effort read of the ctl
	// fires log the hook appended.
	s.recordReinjections(ctx, in, brief, workDir)
	return res, nil
}

// runDirs materializes the per-run cwd and per-stage work dir under
// RunRoot. The work dir is per-STAGE so each fresh session gets its own
// lowered config + ctl dir (Spec S03.5: platform-owned, never inside Cwd);
// the cwd is per-run so execute steps share a workspace.
func (s *Skeleton) runDirs(runID, stg string) (workDir, cwd string, err error) {
	base := filepath.Join(s.cfg.RunRoot, sanitizePathComponent(runID))
	cwd = filepath.Join(base, "cwd")
	workDir = filepath.Join(base, "work", sanitizePathComponent(stg))
	for _, d := range []string{cwd, workDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return "", "", fmt.Errorf("stage: run dirs: %w", err)
		}
	}
	return workDir, cwd, nil
}

func sanitizePathComponent(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		default:
			return '_'
		}
	}, s)
}

// modelFor resolves a session model: an explicit selection wins (execute
// sessions carry the S08.8 decision), the Config.Model dev/test override
// next, then the PLANNING duty seat — ceremony sessions (planner, critic,
// judge, revise) are S06.10 planning-model duties; the judge in particular
// is the ratified planning-model class (Spec S07.5). Selection of the
// EXECUTION model never lands here — dispatch passes it explicitly.
func (s *Skeleton) modelFor(override string) string {
	if override != "" {
		return override
	}
	if s.cfg.Model != "" {
		return s.cfg.Model
	}
	return s.seat(worker.DutyPlanning).Model
}

// seat reads a duty-map row (fallback: the execution seat — the duty map
// always carries one; worker.DefaultDutyMap is the shipped default).
func (s *Skeleton) seat(duty string) worker.Seat {
	if st, ok := s.dutyMap[duty]; ok {
		return st
	}
	return s.dutyMap[worker.DutyExecution]
}

// tighterCeiling* compose the run ceiling with a worker guardrail ceiling —
// the tighter bound wins; zero means "no bound from this side".
func tighterCeilingF(a, b float64) float64 {
	switch {
	case a == 0:
		return b
	case b == 0:
		return a
	case b < a:
		return b
	}
	return a
}

func tighterCeilingI(a, b int64) int64 {
	switch {
	case a == 0:
		return b
	case b == 0:
		return a
	case b < a:
		return b
	}
	return a
}

// recordCompiled appends the S08.3 per-spawn compile record (config hash on
// the run — worker.EventWorkerCompiled; best-effort: a failed append is
// loud, the session still runs on the compiled unit).
func (s *Skeleton) recordCompiled(ctx context.Context, r run.Run, stg string, c *worker.Compiled) {
	payload, err := json.Marshal(struct {
		Template   string `json:"template"`
		Version    string `json:"version"`
		Stage      string `json:"stage"`
		ConfigHash string `json:"config_hash"`
	}{c.TemplateID, c.VersionID, stg, c.ConfigHash})
	if err != nil {
		s.logger().Error("stage: marshal worker.compiled", "err", err)
		return
	}
	if _, err := s.cfg.Log.Append(ctx, eventlog.Append{
		RunID: r.ID, Generation: r.Generation, UserID: r.UserID,
		Type: worker.EventWorkerCompiled, SchemaVersion: worker.RoutingSchemaVersion, Payload: payload,
	}); err != nil {
		s.logger().Error("stage: append worker.compiled", "run", r.ID, "err", err)
	}
}

// ownerCredRef reads the run owner's credential-store ref (Spec S02.2
// users family; D2: a PATH handed through env, never material). Empty =
// the engine's own default resolution — the dev posture.
func (s *Skeleton) ownerCredRef(ctx context.Context, userID string) string {
	var ref string
	err := s.cfg.DB.QueryRowContext(ctx,
		`SELECT credential_store_ref FROM users WHERE user_id = ?`, userID).Scan(&ref)
	if err != nil {
		return ""
	}
	return ref
}

// recordReinjections turns SessionStart hook fires into re-injection
// manifest events (Spec S05.4 mid-stage entries; B2-1 machinery). Fires
// are read from the session work dir's gate-ctl exchange; absence is
// normal (most sessions never compact or resume).
func (s *Skeleton) recordReinjections(ctx context.Context, in SessionInput, brief ledger.Brief, workDir string) {
	fires, err := readSessionStartFires(filepath.Join(workDir, "gate-ctl"))
	if err != nil || len(fires) == 0 {
		return
	}
	for _, f := range fires {
		// The startup fire is the initial injection already covered by the
		// assembly manifest; resume/compact fires are the mid-stage ones.
		if f.Source == "startup" {
			continue
		}
		if _, err := s.cfg.Ledger.AppendReinjectionManifest(ctx, in.RunID, in.Stage,
			brief.LedgerVersion, in.Clean, f.Source, f.SessionID); err != nil {
			s.logger().Warn("stage: reinjection manifest", "run", in.RunID, "err", err)
		}
	}
}

// sessionStartFire mirrors the adapter's fires-log line shape (the ctl-dir
// exchange contract, CONVENTIONS §13); read here without importing the
// substrate package — the log is a platform-side artifact.
type sessionStartFire struct {
	SessionID string `json:"session_id"`
	Source    string `json:"source"`
}

func readSessionStartFires(ctlDir string) ([]sessionStartFire, error) {
	raw, err := os.ReadFile(filepath.Join(ctlDir, "session_start.fires.log"))
	if err != nil {
		return nil, err
	}
	var out []sessionStartFire
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var f sessionStartFire
		if err := json.Unmarshal([]byte(line), &f); err != nil {
			continue
		}
		out = append(out, f)
	}
	return out, nil
}

// ---- Stage-split execution (Spec S05.3) ----

// splitRequester carries the auto-accepted stage-split execution for one
// session: it captures the live engine session at spawn (the OnSession
// control seam) and, when the budget watcher's stage-split proposal lands,
// requests the S03.1 pause verb — an interrupt at the engine's next safe
// message boundary. All calls happen on the one DriveStage goroutine
// (OnSession before the pump, the watcher callback inside it, the reads
// after it), so plain fields suffice.
type splitRequester struct {
	s     *Skeleton
	runID string
	stage string

	sess      adapters.Session
	requested bool
}

// capture receives the live session (adapters.StartRequest.OnSession).
func (sr *splitRequester) capture(sess adapters.Session) { sr.sess = sess }

// request executes the auto-accepted split: pause at the next safe
// boundary. Idempotent; the disposition arrives through the session's
// Outcome (a pause that races completion loses, per the adapter contract —
// the overflow event then stays a proposal record).
func (sr *splitRequester) request() {
	if sr.requested || sr.sess == nil {
		return
	}
	sr.requested = true
	if err := sr.sess.Pause(context.Background()); err != nil {
		sr.s.logger().Error("stage: stage-split pause request", "run", sr.runID, "stage", sr.stage, "err", err)
		return
	}
	sr.s.logger().Info("stage: stage-split accepted — pause requested at safe boundary (S05.3)",
		"run", sr.runID, "stage", sr.stage)
}

// ---- Stage-fit budgets & overflow (Spec S05.3) ----

// BudgetReport is one session's context-budget summary.
type BudgetReport struct {
	// WindowTokens is the context window measured against.
	WindowTokens int64
	// MaxPromptTokens is the largest observed per-call TRUE prompt size
	// (cache_read + cache_creation + input — the adapter's usage
	// accounting, Spec S05.3/S10.1).
	MaxPromptTokens int64
	// FitExceeded: the FIRST call's footprint already exceeded ⚙
	// context.stage_fit_target — a plan-shape defect (brief cannot fit the
	// target), raised as a re-plan proposal.
	FitExceeded bool
	// Overflowed: the session crossed ⚙ context.stage_overflow_threshold;
	// OverflowSeq is the context.overflow event's seq (0 = none).
	Overflowed  bool
	OverflowSeq int64
	// Proposal is what the overflow event proposed: "stage-split" or
	// "re-plan".
	Proposal string
}

// budgetWatcher consumes a session's usage events and emits the S05.3
// overflow machinery: run-scoped context.overflow events proposing a stage
// split (auto-accepted platform-internal scheduling; onStageSplit executes
// it at the next checkpoint boundary — the split executor, this packet) or
// a re-plan (second overflow in one planned stage / an overweight brief).
type budgetWatcher struct {
	s     *Skeleton
	ctx   context.Context
	runID string
	gen   int64
	owner string
	stage string

	window     int64
	fitTokens  int64
	overTokens int64
	misread    bool

	// onStageSplit, when set, executes an emitted stage-split proposal
	// (Spec S05.3 auto-accept). Nil = the session class does not split; the
	// event stays a proposal record.
	onStageSplit func()

	firstSeen bool
	rep       BudgetReport
	prior     int
}

func (s *Skeleton) newBudgetWatcher(ctx context.Context, r run.Run, stg string, prior int, window int64) *budgetWatcher {
	if window <= 0 {
		// The duty-map seat row carries the model's window record (Spec
		// S05.3 "the lane's pinned-model context window" — the retired B2-4
		// constant's home, worker.Seat.WindowTokens).
		window = s.seat(worker.DutyPlanning).WindowTokens
		if window <= 0 {
			window = worker.DefaultWindowTokens
		}
	}
	w := &budgetWatcher{
		s: s, ctx: ctx, runID: r.ID, gen: r.Generation, owner: r.UserID,
		stage: stg, window: window, prior: prior,
	}
	fit, err1 := s.cfg.Settings.Float(keyStageFitTarget)
	over, err2 := s.cfg.Settings.Float(keyStageOverflow)
	if err1 != nil || err2 != nil {
		// The keys are declared (Spec S18); failure is a build defect —
		// log loudly and disable the watcher rather than invent numbers.
		s.logger().Error("stage: read stage-budget ⚙", "fit_err", err1, "over_err", err2)
		w.misread = true
		return w
	}
	w.fitTokens = int64(fit * float64(w.window))
	w.overTokens = int64(over * float64(w.window))
	w.rep.WindowTokens = w.window
	return w
}

// observe is the StartRequest.OnEvent hook: usage accounting per paid call.
func (w *budgetWatcher) observe(ev adapters.Event) {
	if w.misread || ev.Kind != adapters.KindUsage || ev.Usage == nil || ev.Usage.Total {
		return
	}
	u := ev.Usage
	prompt := u.CacheReadTokens + u.CacheCreationTokens + u.InputTokens
	footprint := prompt + u.OutputTokens
	if prompt > w.rep.MaxPromptTokens {
		w.rep.MaxPromptTokens = prompt
	}
	if !w.firstSeen {
		w.firstSeen = true
		if prompt > w.fitTokens {
			// The stage brief's own footprint counts inside the budget; a
			// brief that cannot fit the target is a plan-shape defect →
			// re-plan proposal (Spec S05.3).
			w.rep.FitExceeded = true
			w.emit("re-plan", "stage-start footprint exceeds ⚙ "+keyStageFitTarget, prompt)
		}
	}
	if !w.rep.Overflowed && footprint > w.overTokens {
		w.rep.Overflowed = true
		proposal := "stage-split"
		if w.prior > 0 || w.rep.FitExceeded {
			proposal = "re-plan" // second overflow within one planned stage (S05.3)
		}
		w.emit(proposal, "footprint crossed ⚙ "+keyStageOverflow, footprint)
	}
}

func (w *budgetWatcher) emit(proposal, why string, tokens int64) {
	payload, err := json.Marshal(struct {
		Stage          string `json:"stage"`
		Proposal       string `json:"proposal"` // stage-split | re-plan
		Why            string `json:"why"`
		MeasuredTokens int64  `json:"measured_tokens"`
		WindowTokens   int64  `json:"window_tokens"`
		FitTokens      int64  `json:"fit_target_tokens"`
		OverflowTokens int64  `json:"overflow_tokens"`
		PriorOverflows int    `json:"prior_overflows,omitempty"`
		// A stage-split proposal is auto-accepted and executes at the next
		// checkpoint boundary: consolidate-to-ledger (stage-close gate) →
		// end session → successor sub-stage brief from the updated ledger
		// (Spec S05.3; the split executor is the stage runner). A re-plan
		// proposal stays a record for the re-plan machinery.
		ExecutesAt string `json:"executes_at"`
	}{w.stage, proposal, why, tokens, w.window, w.fitTokens, w.overTokens, w.prior,
		"next checkpoint boundary (auto-accepted; split executor, S05.3)"})
	if err != nil {
		w.s.logger().Error("stage: marshal context.overflow", "err", err)
		return
	}
	seq, err := w.s.cfg.Log.Append(w.ctx, eventlog.Append{
		RunID:         w.runID,
		Generation:    w.gen,
		UserID:        w.owner,
		Type:          EventContextOverflow,
		SchemaVersion: contextOverflowSchemaVersion,
		Payload:       payload,
	})
	if err != nil {
		w.s.logger().Error("stage: append context.overflow", "run", w.runID, "err", err)
		return
	}
	w.rep.OverflowSeq = seq
	w.rep.Proposal = proposal
	w.s.logger().Warn("stage: context overflow", "run", w.runID, "stage", w.stage,
		"proposal", proposal, "tokens", tokens, "window", w.window)
	// Auto-accepted execution (Spec S05.3): the proposal event is durably
	// logged FIRST, then the split executes — fully logged scheduling.
	if proposal == "stage-split" && w.onStageSplit != nil {
		w.onStageSplit()
	}
}

func (w *budgetWatcher) report() BudgetReport { return w.rep }
