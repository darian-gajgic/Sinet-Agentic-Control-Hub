package settings

import "fmt"

// The declaration index: every ⚙ key of Spec S18.2, one Decl per dotted
// key, grouped by owning section. Defaults and clamps are the ratified
// values, canonicalized (durations → seconds, sizes → bytes); keys whose
// name carries a unit suffix (_s, _days, _hours, _mb, ...) stay integers in
// that unit, per the S18.4 R10 note that unit heterogeneity is cosmetic
// under typed declarations. The package tests pin this index to the S18.5
// tallies (118 keys, 33 domains, per-owner counts).

func f(x float64) *float64 { return &x }

func index() []Decl {
	return []Decl{
		// ─── S01 — shell (S18.2) ───
		{
			Key: "shell.drain_grace", Section: "S01", Type: TypeDuration, Unit: "seconds",
			Default: int64(900), Min: f(60), Max: f(86400),
			Title:    "Maintenance drain grace",
			Help:     "How long in-flight runs may keep working after maintenance mode starts before they are parked at their last checkpoint.",
			Ratified: "G1 Def.7",
		},
		{
			Key: "shell.watchdog_sec", Section: "S01", Type: TypeInt, Unit: "seconds",
			Default: int64(30), Min: f(10), Max: f(300),
			Title:    "Control-plane watchdog interval",
			Help:     "The systemd WatchdogSec= budget for sinet-control's heartbeat; missing it lets systemd restart the control plane.",
			Ratified: "R17 §4.1 (⚙ flagged unnumbered)",
		},
		{
			Key: "shell.inhibit_delay_max", Section: "S01", Type: TypeDuration, Unit: "seconds",
			Default: int64(30), Min: f(5), Max: f(60), Restart: true,
			Title:    "Sleep-inhibitor window",
			Help:     "The logind delay-inhibitor window for the pre-sleep flush, applied as host configuration; the flush itself is sized to fit even the stock 5 seconds.",
			Ratified: "R17 §4.9; SPIKE P2-S2",
		},
		{
			Key: "shell.journal_max_use", Section: "S01", Type: TypeInt, Unit: "bytes",
			Default: int64(4294967296), Min: f(536870912), Max: f(34359738368), Restart: true,
			Title:    "Journal disk cap",
			Help:     "Total disk space journald may use for the ops logs; the audit record lives in platform.db, never here.",
			Ratified: "R17 §4.8 (⚙ flagged unnumbered)",
		},

		// ─── S02 — state (S18.2) ───
		{
			Key: "state.synchronous", Section: "S02", Type: TypeEnum,
			Enum: []string{"FULL", "NORMAL"}, Default: "FULL",
			Title:    "SQLite synchronous mode",
			Help:     "FULL deletes the power-loss window entirely at Sinet's write rate; NORMAL is the documented fallback if measurement ever disagrees.",
			Ratified: "G2 Def.1",
		},
		{
			Key: "state.busy_timeout", Section: "S02", Type: TypeDuration, Unit: "seconds",
			Default: int64(5), Min: f(5),
			Title:    "SQLite busy timeout",
			Help:     "How long a write waits for the database lock before failing; honored by the BEGIN IMMEDIATE write discipline.",
			Ratified: "R08 §4.1",
		},
		{
			Key: "state.wal_truncate_interval", Section: "S02", Type: TypeDuration, Unit: "seconds",
			Default: int64(3600), Min: f(300), Max: f(86400),
			Title:    "WAL truncation interval",
			Help:     "How often the write-ahead log is checkpointed and truncated so it cannot grow without bound.",
			Ratified: "R08 §4.1",
		},
		{
			Key: "state.event_payload_cap", Section: "S02", Type: TypeInt, Unit: "bytes",
			Default: int64(65536), Min: f(4096), Max: f(1048576),
			Title:    "Event payload cap",
			Help:     "Maximum size of one event-log payload; bulky output lives as files referenced by path and hash, never inline.",
			Ratified: "R08 §4.1 / P-T07-5",
		},

		// ─── S02 — recovery ───
		{
			Key: "recovery.heartbeat", Section: "S02", Type: TypeDuration, Unit: "seconds",
			Default: int64(60), Min: f(15), Max: f(300),
			Title:    "Run heartbeat interval",
			Help:     "How often a live run refreshes its lease heartbeat.",
			Ratified: "G2 Def.2",
		},
		{
			Key: "recovery.dead_after", Section: "S02", Type: TypeDuration, Unit: "seconds",
			Default:  int64(300),
			Title:    "Silence-before-dead threshold",
			Help:     "How long a run's event cursor may sit still before the recovery pass treats the run as stalled; must stay at least twice the heartbeat.",
			Ratified: "G2 Def.2 (seeds watchdog.silence_budget.*)",
		},
		{
			Key: "recovery.wake_grace", Section: "S02", Type: TypeDuration, Unit: "seconds",
			Default: int64(120), Min: f(30), Max: f(600),
			Title:    "Post-wake lease grace",
			Help:     "Grace applied to every lease deadline right after the host wakes, so a nap never falsely kills a live run.",
			Ratified: "G2 Def.2",
		},
		{
			Key: "recovery.max_attempts", Section: "S02", Type: TypeInt,
			Default: int64(3), Min: f(1), Max: f(5),
			Title:    "Recovery attempts per interruption",
			Help:     "How many recovery attempts one interruption gets before the run is tombstoned for review.",
			Ratified: "G2 Def.2",
		},
		{
			Key: "recovery.stale_finalize", Section: "S02", Type: TypeDuration, Unit: "seconds",
			Default: int64(86400), Min: f(3600),
			Title:    "Stale-run finalize horizon",
			Help:     "A run interrupted longer than this is finalized with a card instead of blindly resumed; the freshness pass fires first regardless.",
			Ratified: "G2 Def.2",
		},
		{
			Key: "recovery.sweep_interval", Section: "S02", Type: TypeDuration, Unit: "seconds",
			Default: int64(300), Min: f(60), Max: f(1800),
			Title:    "Recovery sweep interval",
			Help:     "How often the level-triggered reconcile pass re-lists actual run state outside of start and wake.",
			Ratified: "R08 §4.4",
		},

		// ─── S02 — effects ───
		{
			Key: "effects.approval_expiry", Section: "S02", Type: TypeDuration, Unit: "seconds",
			Default: int64(604800), Min: f(3600), Max: f(2592000),
			Title:    "Effect approval expiry",
			Help:     "How long an approved outward effect stays valid before drift or age sends it back for re-approval.",
			Ratified: "G2 Def.2 / R08 §4.5",
		},

		// ─── S02 — claims ───
		{
			Key: "claims.default_write", Section: "S02", Type: TypeEnum,
			Enum: []string{"declared-set", "whole-project"}, Default: "whole-project",
			Title:    "Default write claim",
			Help:     "What a plan claims when it cannot bound its write-set: the whole project serializes more but never overwrites; declared-set is the parallel-friendlier alternative.",
			Ratified: "G2 Def.3",
		},

		// ─── S02 — freshness ───
		{
			Key: "freshness.max_age", Section: "S02", Type: TypeDuration, Unit: "seconds",
			Default: int64(86400), Min: f(3600),
			Title:    "Staleness horizon",
			Help:     "A parked run or pending approval older than this re-validates its plan against current reality before spending anything significant; also folded alias of intake.approval_stale_hours (S18.4 R2).",
			Ratified: "G1 Def.5; feature 4.3 (cross-cutting; consumers S03/S06/S10)",
		},
		{
			Key: "freshness.hold_vs_park", Section: "S02", Type: TypeDuration, Unit: "seconds",
			Default: int64(600), Min: f(60), Max: f(3600),
			Title:    "Hold-versus-park threshold",
			Help:     "Pauses shorter than this hold the process for a cheap resume; anything longer parks the run and subjects the resume to the freshness pass.",
			Ratified: "G1 Def.4 (consumer S10)",
		},

		// ─── S03 — adapter (S18.2) ───
		{
			Key: "adapter.claude.cleanup_period_days", Section: "S03", Type: TypeInt, Unit: "days",
			Default: int64(365), Min: f(30),
			Title:    "Claude session cleanup period",
			Help:     "How long the Claude engine keeps local session data before its own sweep; must exceed the effect-approval expiry so an answerable ask never outlives its session.",
			Ratified: "P-T02-1 mitigation; SPIKE G1-S2 F5",
		},
		{
			Key: "adapter.engine_ceiling_backstop_mult", Section: "S03", Type: TypeFloat, Unit: "multiplier",
			Default: float64(2), Min: f(2.0),
			Title:    "Engine ceiling backstop multiplier",
			Help:     "The engine-internal turn ceiling is set to this multiple of the platform ceiling so the platform gate always fires first.",
			Ratified: "SPIKE G1-S2 F4 (consumer S10)",
		},
		{
			Key: "adapter.parallel_gate_fallback", Section: "S03", Type: TypeEnum,
			Enum: []string{"serialize-by-deny", "hold-process"}, Default: "serialize-by-deny",
			Title:    "Parallel-gate fallback strategy",
			Help:     "How a gated call inside a parallel batch is serialized; serialize-by-deny makes the engine re-issue the gated call alone for a faithful ask record.",
			Ratified: "SPIKE G1-S2 §Verdict; P2-S4 headline; [coordinator-draft]",
		},

		// ─── S04 — orchestration (S18.2; (value, floor, ceiling) stored per S04.4) ───
		{
			Key: "orchestration.depth_cap", Section: "S04", Type: TypeInt,
			Default: int64(2), Min: f(0), Max: f(2),
			Title:    "Helper depth cap",
			Help:     "Maximum helper nesting depth; raising the ceiling past 2 is a deliberate operator bounds change, never routine.",
			Ratified: "D6; G1 Def.8",
		},
		{
			Key: "orchestration.max_concurrent_helpers", Section: "S04", Type: TypeInt,
			Default: int64(4), Min: f(0), Auto: true,
			Title:    "Concurrent helpers per task",
			Help:     "How many helpers one task may run at once across all depths; automation may move this within the operator's bounds.",
			Ratified: "G1 Def.8 + rider 1",
		},
		{
			Key: "orchestration.helper_turns", Section: "S04", Type: TypeInt, Unit: "turns",
			Default: int64(20), Min: f(1), Auto: true,
			Title:    "Turns per helper",
			Help:     "Turn budget one helper gets before it must report; automation may move this within the operator's bounds.",
			Ratified: "G1 Def.8 + rider 1",
		},
		{
			Key: "orchestration.helper_tokens", Section: "S04", Type: TypeInt, Unit: "tokens",
			Default: int64(80000), Min: f(1), Auto: true,
			Title:    "Token budget per helper",
			Help:     "Token budget one helper gets; automation may move this within the operator's bounds.",
			Ratified: "G1 Def.8 + rider 1",
		},
		{
			Key: "orchestration.spawn_budget", Section: "S04", Type: TypeInt,
			Default: int64(8), Min: f(0),
			Title:    "Spawn budget per task",
			Help:     "Total helper spawns one task gets, including sub-helpers and retries; overrunning it takes an operator-visible gate.",
			Ratified: "G1 Def.8; R06 §4.3",
		},
		{
			Key: "orchestration.report_tokens", Section: "S04", Type: TypeInt, Unit: "tokens",
			Default: int64(2000), Min: f(1),
			Title:    "Helper report size",
			Help:     "Token cap on one helper report, enforced by the report screen.",
			Ratified: "G1 Def.8 (screen-enforced, S04.5)",
		},
		{
			Key: "orchestration.bulk_offload_tokens", Section: "S04", Type: TypeInt, Unit: "tokens",
			Default: int64(20000), Min: f(1),
			Title:    "Bulk-offload threshold",
			Help:     "Content larger than this is offloaded to a helper instead of being pulled into the orchestrating context.",
			Ratified: "R06 §4.2; G1 D1.1",
		},
		{
			Key: "orchestration.helper_retry_limit", Section: "S04", Type: TypeInt,
			Default: int64(1), Min: f(0),
			Title:    "Helper retries",
			Help:     "How many times one failed helper is retried before the failure surfaces.",
			Ratified: "R06 §4.4; G1 D1.1",
		},
		{
			Key: "orchestration.stagger_identical_prefix", Section: "S04", Type: TypeBool,
			Default:  true,
			Title:    "Stagger identical-prefix helpers",
			Help:     "Launch helpers sharing an identical prompt prefix staggered so the provider cache is warm for the followers.",
			Ratified: "R06 §4.2; G1 D1.1",
		},

		// ─── S05 — context (S18.2) ───
		{
			Key: "context.stage_fit_target", Section: "S05", Type: TypeFloat, Unit: "ratio",
			Default: 0.50, Min: f(0.20), Max: f(0.70), Auto: true,
			Title:    "Stage fit target",
			Help:     "Fraction of the lane's context window a stage is packed toward; recalibrated per model generation, and always below the overflow threshold.",
			Ratified: "G1 Def.11 (per-generation recalibration, S05.2)",
		},
		{
			Key: "context.stage_overflow_threshold", Section: "S05", Type: TypeFloat, Unit: "ratio",
			Default: 0.70, Max: f(0.85), Auto: true,
			Title:    "Stage overflow threshold",
			Help:     "Fraction of the lane's context window past which a stage overflows and splits; stays at least 0.05 above the fit target.",
			Ratified: "G1 Def.11 (same recalibration)",
		},
		{
			Key: "context.conventions_max_lines", Section: "S05", Type: TypeInt, Unit: "lines",
			Default: int64(150), Min: f(50), Max: f(400),
			Title:    "Conventions file line cap",
			Help:     "Maximum lines of the injected conventions file before it must be tightened.",
			Ratified: "R07 §4.4",
		},
		{
			Key: "context.recitation_interval_turns", Section: "S05", Type: TypeInt, Unit: "turns",
			Default:  int64(10),
			Title:    "Objective recitation interval",
			Help:     "Every this-many turns the run recites its objective to fight drift; 0 turns recitation off, otherwise 5 to 50.",
			Ratified: "[coordinator-draft]",
		},

		// ─── S06 — intake (S18.2) ───
		{
			Key: "intake.zero_interaction_cost_usd", Section: "S06", Type: TypeFloat, Unit: "USD",
			Default: 0.50, Min: f(0), PerUser: true,
			Title:    "Zero-interaction cost band",
			Help:     "Tasks estimated under this API-equivalent cost may run without interview questions; per person.",
			Ratified: "G1 P11 + D1.7 (⚙ per-user at the gate)",
		},
		{
			Key: "intake.clearance_floor.low", Section: "S06", Type: TypeInt, Unit: "score",
			Default: int64(60), Min: f(0), Max: f(100),
			Title:    "Clearance floor — low stakes",
			Help:     "Interviewing continues while the spec's clearance score sits below this floor on low-stakes tasks.",
			Ratified: "G1 P8 mechanism; default [coordinator-draft]",
		},
		{
			Key: "intake.clearance_floor.standard", Section: "S06", Type: TypeInt, Unit: "score",
			Default: int64(75), Min: f(0), Max: f(100),
			Title:    "Clearance floor — standard stakes",
			Help:     "Interviewing continues while the spec's clearance score sits below this floor on standard-stakes tasks.",
			Ratified: "G1 P8 mechanism; default [coordinator-draft]",
		},
		{
			Key: "intake.clearance_floor.high", Section: "S06", Type: TypeInt, Unit: "score",
			Default: int64(90), Min: f(0), Max: f(100),
			Title:    "Clearance floor — high stakes",
			Help:     "Interviewing continues while the spec's clearance score sits below this floor on high-stakes tasks.",
			Ratified: "G1 P8 mechanism; default [coordinator-draft]",
		},
		{
			Key: "intake.size_recheck_factor", Section: "S06", Type: TypeFloat, Unit: "multiplier",
			Default: 2.0, Min: f(1.0), MinExclusive: true,
			Title:    "Size re-check factor",
			Help:     "If the plan-derived estimate exceeds the intake guess by this factor, size and tier are re-decided before expensive work.",
			Ratified: "feature 2.5; R03 §4 Stage 2(c); default [coordinator-draft]",
		},
		{
			Key: "intake.coverage_autofix_rounds", Section: "S06", Type: TypeInt, Unit: "rounds",
			Default: int64(1), Min: f(0), Max: f(2),
			Title:    "Coverage auto-fix rounds",
			Help:     "Bounded planner rounds to fix an uncovered acceptance criterion before a decision card goes to the requester.",
			Ratified: "R03 §4 Stage 2(a) (\"bounded\")",
		},
		{
			Key: "intake.critique_revise_rounds", Section: "S06", Type: TypeInt, Unit: "rounds",
			Default: int64(1), Min: f(0), Max: f(2),
			Title:    "Critique revise rounds",
			Help:     "Bounded revise rounds after the plan self-attack before the result ships as-is with the critique attached.",
			Ratified: "R03 §4 Stage 3 (\"fixes once\")",
		},

		// ─── S07 — verification (S18.2) ───
		{
			Key: "verification.rework_rounds", Section: "S07", Type: TypeInt, Unit: "rounds",
			Default: int64(3), Min: f(0),
			Title:    "Rework rounds",
			Help:     "Maximum fix-and-re-verify rounds a deliverable gets before the remaining findings surface to a human.",
			Ratified: "R04 §4 via G1 D1.1(d); S07 resolution note",
		},
		{
			Key: "verification.convergence_patience_rounds", Section: "S07", Type: TypeInt, Unit: "rounds",
			Default: int64(2), Min: f(1),
			Title:    "Convergence patience",
			Help:     "How many rework rounds may pass without the finding count shrinking before rework stops early; never more than the rework rounds themselves.",
			Ratified: "R04 §2.6/§3-D via G1 D1.1(d)",
		},
		{
			Key: "verification.sanity_stakes_floor", Section: "S07", Type: TypeEnum,
			Enum: []string{"trivial", "low", "standard", "high"}, Default: "standard",
			Title:    "Sanity-pass stakes floor",
			Help:     "The lowest stakes tier (S06.4 tiers) at which the independent sanity pass always runs.",
			Ratified: "G1 D1.2 mechanism; default [coordinator-draft]",
		},
		{
			Key: "verification.entailment_sample_rate", Section: "S07", Type: TypeFloat, Unit: "ratio",
			Default: float64(0), Min: f(0), Max: f(1),
			Title:    "Entailment sampling rate",
			Help:     "Fraction of non-load-bearing claims sampled into the entailment check; 0 until the bring-up calibration set fixes the ratified value (TBD-BRINGUP).",
			Ratified: "G1 Def.2; G3 Def.4/Def.8; default TBD-BRINGUP(entailment calibration set)",
		},
		{
			Key: "verification.research_rerun_limit", Section: "S07", Type: TypeInt, Unit: "rounds",
			Default: int64(1), Min: f(0), Max: f(2),
			Title:    "Research re-run limit",
			Help:     "How many times a failed research verification may re-run its research node before carding.",
			Ratified: "feature 1.9; R03 §4 Stage 2(d) via S06.6",
		},
		{
			Key: "verification.check_audit_interval_days", Section: "S07", Type: TypeInt, Unit: "days",
			Default: int64(90), Min: f(1),
			Title:    "Check-the-checker audit interval",
			Help:     "How often the verification checks themselves are audited against ground truth.",
			Ratified: "P-T06-1; R04 §4; default [coordinator-draft]",
		},
		{
			Key: "verification.canary_interval_hours", Section: "S07", Type: TypeInt, Unit: "hours",
			Default: int64(24), Min: f(1),
			Title:    "Escalation-path canary interval",
			Help:     "Cadence of the dead-man canary that proves the escalation path can still reach a human; silence is itself the alarm.",
			Ratified: "G1 Def.3 (superseded set: \"canary daily\"); S18.4 R5",
		},
		{
			Key: "verification.drill_interval_days", Section: "S07", Type: TypeInt, Unit: "days",
			Default: int64(90), Min: f(1),
			Title:    "Escalation drill interval",
			Help:     "Cadence of the full escalation drill (distinct from the backup restore drill, S18.4 R6).",
			Ratified: "G1 Def.3 (superseded set: \"drill quarterly\"); S18.4 R6",
		},
		{
			Key: "verification.card_remind_hours", Section: "S07", Type: TypeInt, Unit: "hours",
			Default: int64(4), Min: f(1),
			Title:    "Card reminder cadence",
			Help:     "Inbox-wide: how long an unanswered card waits before its first reminder.",
			Ratified: "G1 Def.3 (superseded set) — inbox-wide",
		},
		{
			Key: "verification.card_push_hours", Section: "S07", Type: TypeInt, Unit: "hours",
			Default: int64(24), Min: f(1),
			Title:    "Card push cadence",
			Help:     "Inbox-wide: how long an unanswered card waits before a push notification goes out.",
			Ratified: "G1 Def.3 (superseded set) — inbox-wide",
		},
		{
			Key: "verification.safety_reping_hours", Section: "S07", Type: TypeInt, Unit: "hours",
			Default: int64(1), Min: f(1),
			Title:    "Safety re-ping cadence",
			Help:     "Inbox-wide: re-ping interval for safety-relevant cards.",
			Ratified: "G1 Def.3 (superseded set) — inbox-wide",
		},

		// ─── S08 — workers (S18.2) ───
		{
			Key: "workers.first_n", Section: "S08", Type: TypeInt,
			Default: int64(3), Min: f(1),
			Title:    "Supervised first runs",
			Help:     "A new or changed worker's first N runs are supervised; the count resets when its body or equipment version changes.",
			Ratified: "G3 D3.4",
		},
		{
			Key: "workers.gap_proposal_count", Section: "S08", Type: TypeInt,
			Default: int64(2), Min: f(2),
			Title:    "Gap occurrences before proposal",
			Help:     "How many times the same capability gap must occur before a worker proposal is raised.",
			Ratified: "G3 D3.4 (\"second occurrence\")",
		},
		{
			Key: "workers.persona_lines_max", Section: "S08", Type: TypeInt, Unit: "lines",
			Default: int64(2), Min: f(0),
			Title:    "Persona line cap",
			Help:     "Warn threshold of the station-1 lint on persona prose in a worker definition.",
			Ratified: "R15 §4.2 (⚙ there)",
		},
		{
			Key: "workers.dryrun_cost_cap_usd", Section: "S08", Type: TypeFloat, Unit: "USD",
			Default: 0.50, Min: f(0), MinExclusive: true,
			Title:    "Worker dry-run cost cap",
			Help:     "API-equivalent budget one composer dry-run may spend.",
			Ratified: "R15 §4.3 (⚙ there); anchor G1 D1.7 [coordinator-draft]",
		},

		// ─── S09 — memory (S18.2) ───
		{
			Key: "memory.l1_ttl_days", Section: "S09", Type: TypeInt, Unit: "days",
			Default: int64(90), Min: f(7), Max: f(365),
			Title:    "L1 memory TTL",
			Help:     "Days an L1 observation lives before expiry; the L1 scope activates at v1.",
			Ratified: "G2 Def.7 (L1 scope activates v1)",
		},
		{
			Key: "memory.reverify_lessons_days", Section: "S09", Type: TypeInt, Unit: "days",
			Default: int64(90), Min: f(30), Max: f(365),
			Title:    "Lesson re-verify interval",
			Help:     "Days before a stored lesson must be re-verified against reality.",
			Ratified: "G2 Def.7",
		},
		{
			Key: "memory.reverify_house_days", Section: "S09", Type: TypeInt, Unit: "days",
			Default: int64(180), Min: f(90), Max: f(730),
			Title:    "House-knowledge re-verify interval",
			Help:     "Days before a house-scope knowledge entry must be re-verified.",
			Ratified: "G2 Def.7",
		},
		{
			Key: "memory.proposals_per_task_max", Section: "S09", Type: TypeInt,
			Default: int64(2), Min: f(0), Max: f(5),
			Title:    "Memory proposals per task",
			Help:     "Maximum memory-write proposals one task may raise through the write gate.",
			Ratified: "G2 Def.7",
		},
		{
			Key: "memory.digest_interval_days", Section: "S09", Type: TypeInt, Unit: "days",
			Default: int64(7), Min: f(1), Max: f(30),
			Title:    "Memory digest interval",
			Help:     "Days between memory digests presented for review.",
			Ratified: "G2 Def.7",
		},
		{
			Key: "memory.distill_threshold_lessons", Section: "S09", Type: TypeInt,
			Default: int64(3), Min: f(2), Max: f(10),
			Title:    "Distillation threshold",
			Help:     "How many related lessons accumulate before distillation into a single entry is proposed.",
			Ratified: "G2 Def.7",
		},
		{
			Key: "memory.injection_budget_tokens.house", Section: "S09", Type: TypeInt, Unit: "tokens",
			Default: int64(2000), Min: f(500), Max: f(10000),
			Title:    "Injection budget — house scope",
			Help:     "Token budget for house-scope memory injected into a run's context.",
			Ratified: "G2 Def.7; numbers [coordinator-draft]",
		},
		{
			Key: "memory.injection_budget_tokens.project", Section: "S09", Type: TypeInt, Unit: "tokens",
			Default: int64(3000), Min: f(500), Max: f(10000),
			Title:    "Injection budget — project scope",
			Help:     "Token budget for project-scope memory injected into a run's context.",
			Ratified: "G2 Def.7; numbers [coordinator-draft]",
		},
		{
			Key: "memory.injection_budget_tokens.user", Section: "S09", Type: TypeInt, Unit: "tokens",
			Default: int64(1500), Min: f(500), Max: f(10000),
			Title:    "Injection budget — user scope",
			Help:     "Token budget for user-scope memory injected into a run's context.",
			Ratified: "G2 Def.7; numbers [coordinator-draft]",
		},
		{
			Key: "memory.injection_budget_tokens.worker_overlay", Section: "S09", Type: TypeInt, Unit: "tokens",
			Default: int64(1500), Min: f(500), Max: f(10000),
			Dormant:  "worker-overlay scope activates v1 (S18.5)",
			Title:    "Injection budget — worker overlay",
			Help:     "Token budget for worker-overlay memory; the scope activates at v1.",
			Ratified: "G2 Def.7; numbers [coordinator-draft]",
		},
		{
			Key: "memory.vector_gate.task_miss_rate", Section: "S09", Type: TypeFloat, Unit: "ratio",
			Default: 0.05, Min: f(0.01), Max: f(0.25),
			Dormant:  "pre-registered trigger, evaluated post-15.3 (S18.5)",
			Title:    "Vector gate — miss-rate trigger",
			Help:     "Pre-registered retrieval miss rate that, once exceeded, opens the vector-search gate; evaluated only after the 15.3 benchmark gate.",
			Ratified: "G2 Def.8",
		},
		{
			Key: "memory.vector_gate.corpus_entries", Section: "S09", Type: TypeInt,
			Default: int64(5000), Min: f(1000), Max: f(50000),
			Dormant:  "pre-registered trigger, evaluated post-15.3 (S18.5)",
			Title:    "Vector gate — corpus-size trigger",
			Help:     "Pre-registered corpus size that, once exceeded, opens the vector-search gate.",
			Ratified: "G2 Def.8",
		},

		// ─── S10 — pressure / budget / meter / limit / scheduler / arbitration (S18.2) ───
		{
			Key: "pressure.cache_read_weight", Section: "S10", Type: TypeFloat, Unit: "ratio",
			Default: 0.1, Min: f(0), Max: f(1),
			Title:    "Cache-read pressure weight",
			Help:     "How strongly cache reads count into consumption pressure; labeled assumed until the provider publishes quota semantics.",
			Ratified: "G1 Def.10 (consumer S05; dedup S18.4 R3)",
		},
		{
			Key: "pressure.bg_admit_stop", Section: "S10", Type: TypeFloat, Unit: "ratio",
			Default: 0.7, Min: f(0.1), Max: f(0.95),
			Title:    "Background admission stop",
			Help:     "Consumption-pressure level past which background work stops being admitted.",
			Ratified: "G2 Def.4",
		},
		{
			Key: "budget.background_window_fraction", Section: "S10", Type: TypeFloat, Unit: "ratio",
			Default: 0.5, Min: f(0), Max: f(1),
			Title:    "Background window fraction",
			Help:     "Fraction of a subscription window background work may consume; labeled assumed.",
			Ratified: "G2 Def.4/5",
		},
		{
			Key: "meter.value_divergence_alarm", Section: "S10", Type: TypeFloat, Unit: "percent",
			Default: float64(20), Min: f(5), Max: f(100),
			Title:    "Meter divergence alarm",
			Help:     "Alarm threshold for divergence between the platform's meter and the provider's reported usage.",
			Ratified: "R09 §4.1 / P-T08-1",
		},
		{
			Key: "limit.retry_cap", Section: "S10", Type: TypeInt,
			Default: int64(3), Min: f(1), Max: f(5),
			Title:    "Limit-event retry cap",
			Help:     "Maximum retries after a provider limit event before the run parks.",
			Ratified: "G2 D2.1 / R09 §4.3",
		},
		{
			Key: "limit.retry_budget_ratio", Section: "S10", Type: TypeFloat, Unit: "ratio",
			Default: 0.10, Min: f(0.01), Max: f(0.5),
			Title:    "Retry budget ratio",
			Help:     "Fraction of a run's budget retries may consume.",
			Ratified: "G2 D2.1 / R09 §4.3",
		},
		{
			Key: "limit.probe_interval_max", Section: "S10", Type: TypeDuration, Unit: "seconds",
			Default: int64(1800), Min: f(60), Max: f(7200),
			Title:    "Limit probe interval cap",
			Help:     "Ceiling on the backoff between probes of a closed provider window.",
			Ratified: "G2 D2.1 / R09 §4.3",
		},
		{
			Key: "scheduler.missed_slot_default", Section: "S10", Type: TypeEnum,
			Enum: []string{"run-once-late", "skip", "notify-only"}, Default: "run-once-late",
			Dormant:  "ships v0, consumer surface v1 (S18.5)",
			Title:    "Missed-slot default",
			Help:     "What a schedule slot missed during suspend does by default; the user-facing schedule surface arrives at v1.",
			Ratified: "R09 §4.7",
		},
		{
			Key: "arbitration.background_cpuweight", Section: "S10", Type: TypeString,
			Default:  "idle",
			Title:    "Background CPU weight",
			Help:     "systemd CPUWeight for the background slice: idle, or an integer weight from 1 to 10000.",
			Ratified: "R09 §4.9 (slice mechanism S01)",
		},

		// ─── S11 — sandbox (S18.2) ───
		{
			Key: "sandbox.egress_deny_cidrs", Section: "S11", Type: TypeList,
			Default:  []string{"169.254.169.254/32", "169.254.0.0/16", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7"},
			Title:    "Egress deny list",
			Help:     "CIDRs sandboxed runs may never reach; the cloud-metadata address is a non-removable floor.",
			Ratified: "R10 §2.3/§3.3",
		},
		{
			Key: "sandbox.block_outbound_doh", Section: "S11", Type: TypeBool,
			Default:  true,
			Title:    "Block outbound DoH",
			Help:     "Block DNS-over-HTTPS from sandboxes so egress policy cannot be tunneled around; turning this off requires a recorded reason.",
			Ratified: "R10 §2.3",
		},
		{
			Key: "sandbox.c2_registry_allowlist", Section: "S11", Type: TypeList,
			Default:  []string{},
			Title:    "Package-registry allowlist",
			Help:     "Curated package-registry and CDN hosts C2-confined runs may fetch from (the Copilot/Codex-preset shape; npm/pypi/crates/apt/go/maven/nuget/rubygems). The concrete curated preset is materialized by the S11 sandbox packet; consumers treat an empty list as preset-not-installed. Data, like the price table.",
			Ratified: "R10 §3.3",
		},
		{
			Key: "sandbox.model_egress_tls_terminate", Section: "S11", Type: TypeBool,
			Default:  true,
			Title:    "Model egress TLS termination",
			Help:     "Terminate TLS on model egress per lane; off means that lane falls back to pattern-2 scoped egress.",
			Ratified: "SPIKE P2-S3 / R10 §3.4",
		},

		// ─── S12 — local (S18.2) ───
		{
			Key: "local.ttl.fast_s", Section: "S12", Type: TypeInt, Unit: "seconds",
			Default: int64(120), Min: f(0), Max: f(3600),
			Title:    "Fast-tier idle TTL",
			Help:     "Idle seconds before the fast-tier local model is unloaded and its VRAM returns to zero.",
			Ratified: "R16 §4.1",
		},
		{
			Key: "local.ttl.workhorse_s", Section: "S12", Type: TypeInt, Unit: "seconds",
			Default: int64(300), Min: f(0), Max: f(7200),
			Title:    "Workhorse idle TTL",
			Help:     "Idle seconds before the workhorse local model is unloaded.",
			Ratified: "R16 §4.1",
		},
		{
			Key: "local.ttl.egpu_s", Section: "S12", Type: TypeInt, Unit: "seconds",
			Default: int64(1800), Min: f(0), Max: f(86400),
			Dormant:  "dormant until pool24 (S18.5)",
			Title:    "eGPU idle TTL",
			Help:     "Idle seconds before an eGPU-pool model is unloaded; dormant until the eGPU pool is enrolled.",
			Ratified: "R16 §4.1",
		},
		{
			Key: "local.vram.guard_band_mb", Section: "S12", Type: TypeInt, Unit: "MB",
			Default: int64(512), Min: f(128),
			Title:    "VRAM guard band",
			Help:     "VRAM headroom the GPU broker keeps free when admitting a model load.",
			Ratified: "R16 §4.4 step 5",
		},
		{
			Key: "local.unload.term_grace_s", Section: "S12", Type: TypeInt, Unit: "seconds",
			Default: int64(5), Min: f(1), Max: f(30),
			Title:    "Unload termination grace",
			Help:     "Seconds a local-model backend gets to exit cleanly before it is killed on unload.",
			Ratified: "R16 §4.5",
		},
		{
			Key: "local.gamemode_hook", Section: "S12", Type: TypeBool,
			Default:  true,
			Title:    "GameMode hook",
			Help:     "Pause local-lane admissions and eager-unload models when GameMode reports a game session.",
			Ratified: "G2 Def.6",
		},
		{
			Key: "local.battery.gpu_admission", Section: "S12", Type: TypeEnum,
			Enum: []string{"never", "urgent-only", "always"}, Default: "urgent-only",
			Title:    "GPU admission on battery",
			Help:     "Whether GPU model loads are admitted while the host runs on battery.",
			Ratified: "R16 §4.6 (name [coordinator-draft])",
		},
		{
			Key: "local.batch.ac_only", Section: "S12", Type: TypeBool,
			Default:  true,
			Title:    "Batch work on AC only",
			Help:     "Restrict local batch workloads to when the host is on mains power.",
			Ratified: "R16 §4.6",
		},
		{
			Key: "local.broker.sandbox_logprobs", Section: "S12", Type: TypeBool,
			Default:  false,
			Title:    "Sandbox logprobs exposure",
			Help:     "Whether sandboxed callers of the local-inference bridge may request logprobs (per-template; off reduces side channels).",
			Ratified: "R16 §4.7",
		},
		{
			Key: "local.reeval.cadence_months", Section: "S12", Type: TypeInt, Unit: "months",
			Default: int64(6), Min: f(1), Max: f(12),
			Title:    "Local model re-evaluation cadence",
			Help:     "Months between re-evaluations of the local model set against current releases.",
			Ratified: "R16 §4.10",
		},
		{
			Key: "local.alias", Section: "S12", Type: TypeMap,
			Default: map[string]string{
				"utility":                "workhorse",
				"intake-triage":          "fast",
				"watchdog-disambiguator": "fast",
				"watchlist-triage":       "fast",
				"intent-filling":         "fast",
				"sql-open":               "Arctic-Text2SQL-R1-7B",
				"entailment":             "Granite Guardian 8B",
				"contradiction-screen":   "DeBERTa-class NLI cross-encoder",
				"distill-summarize":      "workhorse",
				"embedder":               "Qwen3-Embedding-0.6B",
			},
			Title:    "Duty-alias map",
			Help:     "Maps each platform duty alias to its serving local model or seat (S12.3/S12.4); changes land only through the S12.10 swap gate.",
			Ratified: "R16 §4.7; G3 Def.8 (map key ⚙ local.alias.<duty>)",
		},

		// ─── S13 — review / backup / preview (S18.2) ───
		{
			Key: "review.anchor_drift_lines", Section: "S13", Type: TypeInt, Unit: "lines",
			Default: int64(2), Min: f(0), Max: f(10),
			Title:    "Review anchor drift tolerance",
			Help:     "How many lines a review comment's anchor may drift before it detaches (the carried N15 behavior).",
			Ratified: "FC-v1 §2",
		},
		{
			Key: "backup.interval", Section: "S13", Type: TypeDuration, Unit: "seconds",
			Default: int64(86400), Min: f(21600), Max: f(604800),
			Title:    "Snapshot interval",
			Help:     "How often the durable-set snapshot pipeline runs.",
			Ratified: "G2 D2.5",
		},
		{
			Key: "backup.keep", Section: "S13", Type: TypeInt, Unit: "snapshots",
			Default: int64(30), Min: f(7), Max: f(365),
			Title:    "Snapshots kept",
			Help:     "How many snapshots the rotation keeps.",
			Ratified: "G2 D2.5",
		},
		{
			Key: "backup.repo_rotation", Section: "S13", Type: TypeInt, Unit: "months",
			Default: int64(12), Min: f(6), Max: f(24),
			Title:    "Snapshot repo rotation",
			Help:     "Months before the snapshot repository is rotated.",
			Ratified: "G2 D2.5 (annual)",
		},
		{
			Key: "backup.drill_interval", Section: "S13", Type: TypeInt, Unit: "months",
			Default: int64(3), Min: f(1), Max: f(12),
			Title:    "Restore drill interval",
			Help:     "Months between scheduled verified-restore drills (distinct from the escalation drill, S18.4 R6).",
			Ratified: "R13 §4.9 (⚙ flagged unnumbered); S18.4 R6",
		},
		{
			Key: "preview.idle_stop", Section: "S13", Type: TypeDuration, Unit: "seconds",
			Default: int64(900), Min: f(60), Max: f(86400),
			Title:    "Preview idle stop",
			Help:     "Idle time before a running preview is stopped and its port returns to the pool.",
			Ratified: "R13 §4.8 (⚙ flagged unnumbered)",
		},
		{
			Key: "preview.max_concurrent", Section: "S13", Type: TypeInt,
			Default: int64(3), Min: f(1), Max: f(10),
			Title:    "Concurrent previews",
			Help:     "Maximum previews running at once.",
			Ratified: "feature 3.11 posture [coordinator-draft]",
		},

		// ─── S14 — obs / watchdog / watchlist / canary / retention / eval / benchmark (S18.2) ───
		{
			Key: "obs.sse_keepalive", Section: "S14", Type: TypeDuration, Unit: "seconds",
			Default: int64(20), Min: f(15), Max: f(30),
			Title:    "SSE keepalive interval",
			Help:     "Comment-frame keepalive cadence on the one SSE endpoint so idle proxies never drop the stream.",
			Ratified: "R12 §4.3",
		},
		{
			Key: "watchdog.loop_repeat", Section: "S14", Type: TypeInt,
			Default: int64(5), Min: f(3), Max: f(10),
			Title:    "Loop-repeat trigger",
			Help:     "How many near-identical repeats of the same action trip the loop watchdog.",
			Ratified: "G2 Def.10",
		},
		{
			Key: "watchdog.pingpong_cycles", Section: "S14", Type: TypeInt,
			Default: int64(6), Min: f(3), Max: f(12),
			Title:    "Ping-pong trigger",
			Help:     "How many alternating A/B cycles trip the ping-pong watchdog.",
			Ratified: "G2 Def.10",
		},
		{
			Key: "watchdog.error_loop", Section: "S14", Type: TypeInt,
			Default: int64(3), Min: f(2), Max: f(10),
			Title:    "Error-loop trigger",
			Help:     "How many identical errors in a row trip the error-loop watchdog.",
			Ratified: "G2 Def.10",
		},
		{
			Key: "watchdog.silence_budget", Section: "S14", Type: TypeMap,
			Default:  map[string]string{},
			Title:    "Silence budgets by run type",
			Help:     "Per-run-type silence budgets in seconds before the silence watchdog flags; empty entries fall back to the recovery dead-after seed until the bring-up calibration (TBD-BRINGUP) fills them.",
			Ratified: "G2 Def.10 (map key ⚙ watchdog.silence_budget.<run_type>; seed = recovery.dead_after)",
		},
		{
			Key: "watchdog.spend_median_mult", Section: "S14", Type: TypeFloat, Unit: "multiplier",
			Default: float64(3), Min: f(1.5),
			Title:    "Spend spike trigger",
			Help:     "Multiple of the trailing-14-day median spend that trips the spend watchdog.",
			Ratified: "G2 Def.10",
		},
		{
			Key: "watchdog.spend_arm_days", Section: "S14", Type: TypeInt, Unit: "days",
			Default: int64(14), Min: f(7),
			Title:    "Spend watchdog arming period",
			Help:     "Days of history required before the spend watchdog arms.",
			Ratified: "G2 Def.10",
		},
		{
			Key: "watchdog.flag_now_target", Section: "S14", Type: TypeInt, Unit: "per-day",
			Default: int64(2), Min: f(1), Max: f(10),
			Title:    "Flag-now target rate",
			Help:     "Target daily rate of immediate watchdog flags used to judge threshold health.",
			Ratified: "G2 Def.10",
		},
		{
			Key: "watchdog.suppress_retune_count", Section: "S14", Type: TypeInt,
			Default: int64(2), Min: f(1), Max: f(5),
			Title:    "Suppressions before retune",
			Help:     "How many operator suppressions of the same flag propose a threshold retune; the retune lands as a proposal card, never an auto-move.",
			Ratified: "G2 Def.10",
		},
		{
			Key: "watchlist.fetch_fail_streak", Section: "S14", Type: TypeInt, Unit: "cycles",
			Default: int64(3), Min: f(2), Max: f(10),
			Title:    "Watchlist fetch-fail streak",
			Help:     "Consecutive failed fetch cycles before a watchlist source is flagged.",
			Ratified: "R12 §4.5 / P-T11-3",
		},
		{
			Key: "canary.auth_interval", Section: "S14", Type: TypeDuration, Unit: "seconds",
			Default: int64(86400), Min: f(21600), Max: f(604800),
			Title:    "Auth canary interval",
			Help:     "Cadence of the provider auth-revocation canary (daily by default; between four times daily and weekly).",
			Ratified: "G1 Def.3 set; P-T17-1; S18.4 R5",
		},
		{
			Key: "canary.behavioral_interval", Section: "S14", Type: TypeDuration, Unit: "seconds",
			Default: int64(604800), Min: f(86400), Max: f(2592000),
			Title:    "Behavioral canary interval",
			Help:     "Cadence of the behavioral drift canary (weekly by default; between daily and monthly).",
			Ratified: "R12 §4.5 [coordinator-draft]",
		},
		{
			Key: "retention.compaction_horizon", Section: "S14", Type: TypeInt, Unit: "months",
			Default: int64(6), Min: f(1), PerUser: true,
			Title:    "Trace compaction horizon",
			Help:     "Months of full event payloads kept before compaction; per person.",
			Ratified: "G2 Def.11; feature 13.4",
		},
		{
			Key: "eval.sweep_interval", Section: "S14", Type: TypeInt, Unit: "months",
			Default: int64(3), Min: f(1), Max: f(6),
			Title:    "Eval sweep interval",
			Help:     "Months between full regression-eval sweeps.",
			Ratified: "R12 §4.7",
		},
		{
			Key: "benchmark.sampling_rate", Section: "S14", Type: TypeMap,
			Default:  map[string]string{"pre-gate": "100", "maintenance": "25"},
			Title:    "Benchmark sampling rates",
			Help:     "Percent of eligible tasks sampled into the benchmark per phase; uniformity is frozen and registered values change only via a re-registration commit (BENCH-REG §4.1; S18.4 R9).",
			Ratified: "BENCH-REG §4.1 (registered-number rule)",
		},

		// ─── S15 — frontend (S18.2) ───
		{
			Key: "frontend.dependency_pass_interval", Section: "S15", Type: TypeInt, Unit: "days",
			Default: int64(90), Min: f(30), Max: f(365),
			Title:    "Frontend dependency pass interval",
			Help:     "Days between SPA dependency passes; co-scheduled with the components.lock review (S18.4 R7).",
			Ratified: "G3 D3.3 rider; R17 §4.3; S18.4 R7",
		},

		// ─── S16 — adoption (S18.2) ───
		{
			Key: "adoption.dependency_pass_months", Section: "S16", Type: TypeInt, Unit: "months",
			Default: int64(3), Min: f(1), Max: f(6),
			Title:    "Dependency pass cadence",
			Help:     "Months between quarterly passes over every components.lock entry.",
			Ratified: "G3 D3.3 rider; R17 §4.8; S18.4 R7",
		},
		{
			Key: "adoption.abandonment_months", Section: "S16", Type: TypeInt, Unit: "months",
			Default: int64(6), Min: f(2), Max: f(24),
			Title:    "Default abandonment horizon",
			Help:     "Months of upstream inactivity that propose executing an entry's funeral plan; overridable per entry in components.lock.",
			Ratified: "R17 §4.8 (⚙ flagged unnumbered); P-T16-1",
		},
		{
			Key: "adoption.price_data_stale_days", Section: "S16", Type: TypeInt, Unit: "days",
			Default: int64(60), Min: f(14), Max: f(180),
			Title:    "Price data staleness horizon",
			Help:     "Days without upstream price-table updates, while providers reprice, before the fallback source is proposed.",
			Ratified: "R14 §4.2",
		},
	}
}

// rules returns the relational clamps the S18.2 tables state between keys,
// plus structural validity of the data-valued keys.
func rules() []rule {
	intInRange := func(s string, min, max int64) bool {
		if s == "" {
			return false
		}
		var n int64
		for i := 0; i < len(s); i++ {
			c := s[i]
			if c < '0' || c > '9' {
				return false
			}
			n = n*10 + int64(c-'0')
		}
		return n >= min && n <= max
	}
	return []rule{
		{
			name: "recovery.dead_after >= 2x recovery.heartbeat (G2 Def.2)",
			keys: []string{"recovery.dead_after", "recovery.heartbeat"},
			check: func(v *view) error {
				if v.float("recovery.dead_after") < 2*v.float("recovery.heartbeat") {
					return fmt.Errorf("dead_after %vs < 2x heartbeat %vs", v.float("recovery.dead_after"), v.float("recovery.heartbeat"))
				}
				return nil
			},
		},
		{
			name: "context.stage_overflow_threshold >= stage_fit_target + 0.05 (G1 Def.11)",
			keys: []string{"context.stage_fit_target", "context.stage_overflow_threshold"},
			check: func(v *view) error {
				fit, over := v.float("context.stage_fit_target"), v.float("context.stage_overflow_threshold")
				if over < fit+0.05-1e-9 {
					return fmt.Errorf("overflow threshold %v < fit target %v + 0.05", over, fit)
				}
				return nil
			},
		},
		{
			name: "verification.convergence_patience_rounds <= rework_rounds (R04 §2.6)",
			keys: []string{"verification.convergence_patience_rounds", "verification.rework_rounds"},
			check: func(v *view) error {
				if v.float("verification.convergence_patience_rounds") > v.float("verification.rework_rounds") {
					return fmt.Errorf("patience %v > rework rounds %v", v.float("verification.convergence_patience_rounds"), v.float("verification.rework_rounds"))
				}
				return nil
			},
		},
		{
			name: "adapter.claude.cleanup_period_days must exceed effects.approval_expiry (S18.2 S03 row)",
			keys: []string{"adapter.claude.cleanup_period_days", "effects.approval_expiry"},
			check: func(v *view) error {
				if v.float("adapter.claude.cleanup_period_days")*86400 <= v.float("effects.approval_expiry") {
					return fmt.Errorf("cleanup period %v d does not exceed approval expiry %v s", v.float("adapter.claude.cleanup_period_days"), v.float("effects.approval_expiry"))
				}
				return nil
			},
		},
		{
			name: "sandbox.egress_deny_cidrs keeps the metadata-IP floor (S18.2 S11 row)",
			keys: []string{"sandbox.egress_deny_cidrs"},
			check: func(v *view) error {
				for _, c := range v.strings("sandbox.egress_deny_cidrs") {
					if c == "169.254.169.254/32" {
						return nil
					}
				}
				return fmt.Errorf("169.254.169.254/32 is a non-removable floor")
			},
		},
		{
			name: "context.recitation_interval_turns is 0 (off) or 5-50 (S18.2 S05 row)",
			keys: []string{"context.recitation_interval_turns"},
			check: func(v *view) error {
				n := v.float("context.recitation_interval_turns")
				if n != 0 && (n < 5 || n > 50) {
					return fmt.Errorf("%v is neither 0 (off) nor within 5-50", n)
				}
				return nil
			},
		},
		{
			name: "arbitration.background_cpuweight is idle or 1-10000 (systemd CPUWeight)",
			keys: []string{"arbitration.background_cpuweight"},
			check: func(v *view) error {
				s := v.str("arbitration.background_cpuweight")
				if s == "idle" || intInRange(s, 1, 10000) {
					return nil
				}
				return fmt.Errorf("%q is neither \"idle\" nor an integer 1-10000", s)
			},
		},
		{
			name: "benchmark.sampling_rate values are percentages 0-100 (BENCH-REG §4.1)",
			keys: []string{"benchmark.sampling_rate"},
			check: func(v *view) error {
				for phase, val := range v.stringMap("benchmark.sampling_rate") {
					if !intInRange(val, 0, 100) {
						return fmt.Errorf("phase %q rate %q is not an integer percent 0-100", phase, val)
					}
				}
				return nil
			},
		},
		{
			name: "watchdog.silence_budget values are seconds >= 60 (G2 Def.10: >= 1 min)",
			keys: []string{"watchdog.silence_budget"},
			check: func(v *view) error {
				for runType, val := range v.stringMap("watchdog.silence_budget") {
					if !intInRange(val, 60, 1<<40) {
						return fmt.Errorf("run type %q budget %q is not a whole number of seconds >= 60", runType, val)
					}
				}
				return nil
			},
		},
		{
			name: "local.alias targets are non-empty (S12.4; changes ride the S12.10 swap gate)",
			keys: []string{"local.alias"},
			check: func(v *view) error {
				for duty, target := range v.stringMap("local.alias") {
					if duty == "" || target == "" {
						return fmt.Errorf("empty duty or target in the alias map")
					}
				}
				return nil
			},
		},
	}
}
