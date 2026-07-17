# GATE-1 — Architecture direction

**Opened:** 2026-07-17 · **Wave covered:** A (A1 + A1b + A2) · **Status:** OPEN
**Reports in scope:** Research/01 (T01 engines/adapters), 02 (T17 provider watchlist), 03 (T05 intake/planning), 04 (T06 verification), 05 (T02 harness/loop), 06 (T03 orchestration), 07 (T04 context engineering)

## Findings digest

### T01 — Execution engines & adapters (`Research/01-…`)
- **Recommendation:** dual substrate behind Sinet's adapter contract — pinned `opencode serve` per user (API lane: Z.AI-class, local, metered exceptions) + wrapped `claude -p` per user (Anthropic lane); Agent SDK held as in-adapter alternative, decided by spike.
- **Key evidence:** spec's "Agent SDK → credits" claim was stale (change paused 2026-06-15 before effect); real risk class = all headless subscription use repriceable on ~30 days' notice; Anthropic + Google now both first-party-lock subscriptions; churn evidence kills deep multi-engine matrices and TUI scraping.
- **Changes to prior assumptions:** engine transcripts are NOT durable checkpoints (P-T01-1); schema drift is permanent, even first-party (P-T01-2); billing shifts are ~30-day ops events (P-T01-3); cancel corrupts JSONL mid-tool-call (P-T01-4).

### T17 — Provider watchlist & onboarding criteria (`Research/02-…`)
- **Recommendation:** xAI = sanctioned third-party-open lane (rides opencode, config-only, EU model-availability caveat); DeepSeek = metered-only candidate; onboarding checklist (§5) + watchlist cadence (§6) become standing platform machinery.
- **Key evidence:** sanction is allowlist-shaped and revocable server-side without notice (P-T17-1) → every lane needs an auth canary distinct from limit handling.
- **Changes:** report 01 recommendation unchanged, strengthened.

### T05 — Intake, planning & spec pipeline (`Research/03-…`)
- **Recommendation:** staged intake (interview → spec → plan → approval) with stakes-tiered ceremony; plan-stage confinement must bind helpers (P-T05-1); delta re-approval cards carry rubber-stamp risk needing in-house measurement (P-T05-2).
- **Key evidence:** models under-ask by default — scaffolds carry elicitation coverage; exactly one rubber-stamping mitigation has an RCT.
- **Operator pre-decided (see table):** dynamic clarity-driven intake with Clearance indicator; AC dual-phrasing rule; zero-interaction band conditions; ≤8B v0 utility models.

### T06 — Verification & quality loops (`Research/04-…`)
- **Recommendation:** two-axis verification (spec-compliance + outcome-sanity), cheap-first cascades, dissimilar-judge routing where possible, bounded rework (one retry), tested escalation.
- **Key evidence:** style bias inflates judges (0.10–0.76); verifier rot is real (30% of a professional benchmark broken); retrieval-contaminated verification (63% of one model's benchmark wins by lookup); escalation paths die silently → dead-man canary.
- **Changes:** verification is a platform practice with its own failure modes, not a stage; 5 platform problems filed (P-T06-1..5).

### T02 — Agent loop & harness engineering (`Research/05-…`)
- **Recommendation:** Sinet-owned control plane: SQLite-WAL append-only event log + run-lifecycle FSM (stalled = derived, never stored) + platform watchdogs/ceilings/reconcile passes; durable-execution runtimes and LangGraph rejected as spine.
- **Key evidence:** two-tier harness is field consensus; checkpoint-per-paid-call achievable only via platform event log from stream events; `defer` (v2.1.89+) = exit-park primitive with two traps (single-tool-call-only; cleanup-sweep evaporation); **freshness re-validation on stale resume ships nowhere — spec 4.3 is novel**; cost-ceiling granularity is one model call (19x overshoot live-probed on a $0.001 cap) → systemd PID-1 time ceilings are the backstop.
- **Changes:** suspend/wake is a crash-equivalent and double-resume factory (P-T02-3); resume-cache fidelity drifts silently (P-T02-2); 5 problems filed (P-T02-1..5).

### T03 — Orchestration & multi-agent within D6 (`Research/06-…`)
- **Recommendation:** single-agent-first; helpers earned via three logged triggers (T-CTX context protection / T-PAR read fan-out / T-SPEC specialization-quarantine); brief-in/report-out firewall; depth-2 + budgets enforced by the control plane (no engine enforces D6).
- **Key evidence:** the field converged on D6's exact shape (Anthropic Managed Agents depth-1 hard; Cognition writes-single-threaded; Google: centralized contains error amplification 4.4x vs 17.2x independent); designed cost ≈2–4x single-agent — 15x-class only without a context contract; peer-mesh exclusion re-validated stronger (72% fact erasure in agent discussion).
- **Changes:** spawn-reason logging exceeds industry practice — Sinet defines the schema; 4 problems filed (incl. caching-vs-subscription-quota unknown; sibling-failure containment is Sinet-owned).

### T04 — Context engineering (`Research/07-…`)
- **Recommendation:** fresh-context-per-stage on a platform-owned **Task Context Ledger** (pinned objective/AC/constraints; append-only decisions; verified-status state; restorable artifact refs); deterministic registry-driven injection with a trace manifest; compaction demoted to audited safety net; AGENTS.md + CLAUDE.md import shim.
- **Key evidence:** fresh session + consolidated brief restores 95.1% of full-context performance (controlled, multi-model — the strongest number in Wave A); compaction silently drops constraints (violations 0%→30–59%; ≈53pp knowledge loss over 3 cascades); Claude compactor undisableable + version-unstable; subscription cache-quota weighting is publicly unspecified.
- **Changes:** ledger = D7 checkpoint payload = 4.3 freshness input — one artifact serves three spec features; 4 problems filed.

## Decisions required

### Pre-registered operator decisions (recorded during Wave-A review sessions — NOT re-asked)

| # | Decision | Chosen | Date |
|---|---|---|---|
| P1 | R01-OQ1 un-pause response | **Interactive-only demotion** (headless weight → Z.AI-class/local; T08 treats as pre-registered policy) | 07-17 |
| P2 | R01-OQ2 Anthropic compliance posture | **Gray zone as-is** (D2/3.4 only) | 07-17 |
| P3 | R01-OQ5 Codex lane | **Skip** (reopen only if a member holds a ChatGPT plan) | 07-17 |
| P4 | Engine direction nod | **Dual substrate approved** as working direction | 07-17 |
| P5 | R02-OQ5 class-2 gray-zone posture | **As-is, one uniform policy** (headless inside whitelisted tools per vendor docs) | 07-17 |
| P6 | Launch-lane set | **v0 = Anthropic + Z.AI only**; xAI DEFERRED (R02-OQ1); lane-addition must stay config-only | 07-17 |
| P7 | R02-OQ2/3/4/6 (Synthetic/Cerebras, StepFun, BytePlus, DeepSeek ratification) | **PARKED**; metered-exception list EMPTY at v0 (3.10 strictest) | 07-17 |
| P8 | R03-OQ1 question budgets | **Dynamic clarity-driven intake + Clearance indicator (0–100%)**; fixed per-tier caps rejected | 07-17 |
| P9 | R03-OQ3 utility-model duty map | **v0 ≤8B params**; post-v0 ceiling = RTX 3090 24 GB per duty; T15 benchmarks within envelope | 07-17 |
| P10 | R03-OQ2 AC dual-phrasing | **Plain always; structured sub-line only where machine-checkable** (verification binds to it) | 07-17 |
| P11 | R03-OQ4 zero-interaction band | **All-four-conditions rule adopted** (read-only ∧ no data-bearing ∧ cost < threshold ∧ no new tools/workers); threshold number → D1.7 below | 07-17 |
| P12 | R02-OQ7 watchlist owner | Coordinator-resolved → **T11** (brief addendum done) | 07-16 |

### DECISION 1.1 — Ratify the G1 architecture-direction package
- **Context:** The four G1 subjects (CAMPAIGN §4) each have one evidenced recommendation and zero surviving competitors across 7 reports. Package: (a) **engines:** dual substrate per report 01 §4 (pinned opencode v1.x serve per user + wrapped `claude -p` per user; SDK as in-adapter alternative pending spike); (b) **session/stage model:** run-lifecycle FSM + platform-owned SQLite-WAL event log (report 05 §4.1–4.2) + fresh-context-per-stage on the Task Context Ledger (report 07 §4.1–4.2); (c) **orchestration within D6:** single-agent-first, earned helpers via T-CTX/T-PAR/T-SPEC triggers, control-plane-enforced caps/logging (report 06 §4); (d) **verification:** two-axis (spec-compliance + outcome-sanity), cheap-first cascade, bounded rework, tested escalation (report 04 §4).
- **Options:** A) Ratify all four. B) Ratify with named exceptions. C) Hold for discussion.
- **Recommendation:** A — every component is the convergent-evidence winner; B-wave topics (T07 state, T08 metering, T09 sandboxing) all consume this direction.
- **Forecloses:** A → nothing irreversibly (all software choices swappable behind the adapter/contract; G2 revisits substrate specifics) · C → Wave B stays blocked.

### DECISION 1.2 — Outcome-sanity axis scope at launch (R04-OQ2)
- **Context:** Axis 2 (does the deliverable make sense, beyond meeting its AC?) catches what compliance checking misses, but its marginal value at household scale is unmeasured until 11.2 runs.
- **Options:** A) Every deliverable in the two launch domains, stakes-gated in degraded domains (report's proposal). B) Stakes-gated everywhere from day one.
- **Recommendation:** A — launch domains are where quality trust is built; the cost is one extra judge pass on a flat-rate/local lane.
- **Forecloses:** B → blind spots in exactly the domains being evaluated for trust; A → slightly higher ceremony until 11.2 data justifies gating.

### DECISION 1.3 — Runaway containment default for unattended runs (R05-OQ3)
- **Context:** When loop/no-progress detectors flag an unattended run, the platform must act without a human present.
- **Options:** A) Pause-and-flag to inbox (run parks at checkpoint; human decides). B) Auto-kill after N flags.
- **Recommendation:** A — consistent with 4.6 ("contained and surfaced") and the false-positive record of every shipped detector; ceilings already bound damage.
- **Forecloses:** B → false-positive kills of legitimate long work (the documented Gemini/OpenHands failure mode).

### DECISION 1.4 — Spec operating-reality bullet amendment
- **Context:** `Docs/agent-platform-feature-list-v1.md` line 64 still carries the stale claim ("Agent SDK will be moved to credits soon, so this is no option"). T01 disproved it; briefs carry the correction, but the spec is the master document and Docs/ is operator-gated.
- **Options:** A) Amend now (coordinator edits with dated note citing report 01 §2.1 + pre-registered P1 response; operator previews wording). B) Leave flagged; fold into P2 spec-drafting.
- **Recommendation:** A — one sentence of drift in the master spec compounds as more sessions read it; the correction is fully evidenced and its policy response (P1) is already decided.
- **Forecloses:** nothing either way; B risks a future session anchoring on the stale claim.

### DECISION 1.5 — Z.AI plan tier at v0 (R01-OQ4)
- **Context:** v0 lanes are Anthropic + Z.AI (P6). Z.AI tiers: Lite ~$18/mo (~80 prompts/5h, ~1 concurrent), Pro ~$72 (~400, 1–2), Max ~$160 (~1600, 2+); peak-hour 3x / off-peak 2x deduction. v0 is operator-only; prompts batch 15–20 model calls each.
- **Options:** A) Lite now; D4 meters decide upgrade. B) Pro now. C) Decide at first purchase after spec (no plan needed during research).
- **Recommendation:** C, with A as the pre-registered starting tier — nothing in the research phase needs the plan yet; when purchased, start Lite and let measured consumption pressure justify upgrades (D5 discipline).
- **Forecloses:** nothing — tier changes are provider-side, config-invisible.

### DECISION 1.6 — G1 spike battery: timing and scope
- **Context:** Reports queued engine-empirical spikes that sharpen Wave-B research and the spec: (S1) CLI-wrap vs Agent SDK conformance (R01-OQ3; cache fidelity data point P-T02-2 favors CLI today); (S2) defer end-to-end drill incl. parallel-tool-call fallback rate (R05-OQ6, P-T02-4); (S3) opencode park conformance + per-user XDG isolation (R05-OQ7, R01-OQ6); (S4) engine-native micro-fanout observability (R06-OQ2); (S5) PreCompact blocking + Claude-lane injection mechanics (R07-OQ3/5). All are config/probe-level (no app code) — same class as the live probes already run in T01/T02.
- **Options:** A) Dedicated spike session right after G1; results feed T07–T09 briefs as addendum #2. B) Defer all spikes to P3 implementation start. C) Split: S1–S3 (adapter-critical, feed Wave B) now; S4–S5 at P3.
- **Recommendation:** C — S1–S3 answer questions T07/T08/T09 will otherwise research blind; S4/S5 only matter at build time.
- **Forecloses:** B → Wave B carries avoidable unknowns; A → spends a session on S4/S5 answers nobody consumes yet.

### DECISION 1.7 — Zero-interaction cost threshold (completes P11)
- **Context:** The trivial band's fourth condition needs a number: estimated run cost (API-equivalent USD, D5 currency) below which — with all other conditions met — a task may run with zero interaction.
- **Options:** A) $0.50. B) $0.25. C) $1.00. (13.4 setting; per-user; changeable anytime.)
- **Recommendation:** A — a Wave-A research subagent runs ~$2–4 API-equivalent; $0.50 keeps the band to genuinely small read-only work while not being so low it never fires.
- **Forecloses:** nothing — a settings knob with an audit trail.

### Defaults unless objected (adopted silently at gate close if unflagged)

1. **Judge-lane pairing (R04-OQ1):** utility model = default judge; when executor family = judge family and another flat-rate lane is connected, D5 routing may swap to the dissimilar lane; self-family judging always flagged on the receipt.
2. **Research-domain entailment coverage (R04-OQ3):** mandatory claim–citation entailment for load-bearing claims + sampled for the rest (PwC tri-rubric shape); revisit after T15's local-entailment spike.
3. **Escalation liveness (R04-OQ4):** dead-man canary daily; full escalation drill quarterly; approval cards re-nag at 24h, escalation cards push immediately + re-nag at 1h.
4. **Hold-vs-park threshold (R05-OQ1):** 10 minutes.
5. **Freshness fingerprint (R05-OQ2):** {repo HEAD, source content hashes/ETags, spec+plan version, price-table version}; re-validate at age >24h OR any drift OR sibling-accept; price-table drift alone DOES trigger (it re-prices the remaining plan).
6. **systemd transient units per engine process (R05-OQ4):** adopt (PID-1 time ceilings + cgroup accounting).
7. **Maintenance-mode drain grace (R05-OQ5):** 15 minutes.
8. **Orchestration numbers (R06-OQ1):** max 4 concurrent helpers/task; ~20 turns + ~80k tokens per helper; spawn budget 8/task incl. sub-helpers; helper reports ≤2k tokens; depth cap 2 (D6) with depth-1 the norm.
9. **Helper acceptance-check depth (R06-OQ3):** contract-conformance + local-model plausibility screen (free tier; advisory, never auto-kill).
10. **Cache-read weighting in consumption pressure (R07-OQ1):** 0.1× (receipts keep raw counts; labeled "assumed" until Anthropic publishes subscription quota semantics).
11. **Stage-fit budgets (R07-OQ2):** target ≤50% of lane window at stage start; overflow event at 70% proposes a stage split.
12. **Task Context Ledger v0 sections (R07-OQ4):** objective+AC (pinned) / constraints+danger zones (pinned) / decisions (append-only) / state (verified-status) / artifacts (restorable refs) / learned-this-task (expires); exact schema finalized in the T07 spec workshop.

## Decisions taken (filled at close)

| # | Decision | Chosen | By | Date | Notes |
|---|---|---|---|---|---|
| P1–P12 | (pre-registered, above) | as tabled | operator | 07-17 | recorded from review sessions |

## Follow-ups spawned

- **Platform problems P-T01-1..4, P-T17-1..3, P-T05-1..4, P-T06-1..5, P-T02-1..5, R06×4, R07×4** → spec Known-problems list; routing per each report's §7 (T07: checkpoint/cancel; T08: metering/limits/billing-flip; T09: confinement inheritance; T11: eval practice; S2.8: canary suites).
- **Wave-B brief addenda:** T07 consumes ledger-as-checkpoint-payload (R07 §4.2) + FSM (R05 §4.1); T08 consumes P1 pre-registration + lane-heterogeneous consumption units (R07) + cache-alarm calibration (R05-OQ8); T09 consumes plan-stage confinement inheritance (P-T05-1) + sandbox-as-cleanup-boundary (R05 §2.7).
- **Spike results** (per D1.6 choice) append to T07–T09 briefs as addendum #2.
- **R03-OQ5** (P47 trigger list): spec-now item, coordinator drafts at P2. **R03-OQ6** (high-stakes friction): v1 pilot decision, parked.
- **SAW** (bybren-llc/safe-agentic-workflow): STUDY input for T14/D8; post-build seed-pack evaluation (operator memory + T14 brief note).
