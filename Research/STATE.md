# Campaign state — live pointer

> Any session resuming the campaign: read this file first, then `CAMPAIGN.md`, then follow `.claude/skills/research-campaign/SKILL.md`. Update this file before and after every action. This file is the single source of truth for progress.

**Campaign status:** WAVE A2 — **RESUMED on operator instruction 2026-07-17** (same session as 3rd pause). T04 revived via SendMessage with context intact — re-entered at the verifier-launch step, no spend lost (target: report 07). T02 + T03 + T05 + T06 committed + validated (reports 05/06/03/04). Engine-direction nod stands (dual substrate; final at G1).
**Next action:** validate T04 on completion (checklist), commit. Wave A then complete → write GATE G1 memo per gate protocol (template ready; bundles operator backlogs from reports 02 §9, 03 §7, 04 §7, 05 §7, 06 §7 + report-01 residuals (Z.AI tier, CLI-vs-SDK spike plan) + spec operating-reality-bullet amendment, still undecided) and present to operator.
**Last updated:** 2026-07-17 (operator resume; T04 back in flight at verification phase; operator backlog decisions from the parallel review session are recorded below and stand).
**Operating note:** campaign sessions run at **max effort** (operator instruction 2026-07-16); research subagents inherit it.

## Gate log

| Gate | Subject | Status | Decision | Recorded in |
|---|---|---|---|---|
| G0 | Launch: pilot-first vs review-first vs full Wave A | CLOSED 2026-07-16 | **Pilot first** — T01 alone; operator reviews the report before Wave A2 | (this row) |
| G1 | Architecture direction (after Wave A) | pending | — | decisions/GATE-1-*.md |
| G2 | Substrate + adoption list (after Wave B) | pending | — | decisions/GATE-2-*.md |
| G3 | Spec-readiness (after Wave C) | pending | — | decisions/GATE-3-*.md |
| G4 | Spec review → end research phase | pending | — | decisions/GATE-4-*.md |

## Topic board

Statuses: `pending` → `ready` (brief final, wave unblocked) → `running` (agent launched) → `saved` (report written, validating) → `committed` (validated + committed + pushed) → `gated` (reviewed at its gate).

| T | Slug | Wave | Depth | Status | Report file | Notes |
|---|---|---|---|---|---|---|
| T01 | execution-engines-and-adapters | A1 | FULL | **committed + validated** | Research/01-execution-engines-and-adapters.md | pilot PASS — 322 lines, 80 sources/158 URLs; ~48 min, ~266k tokens; commit cfe4dd6 |
| T02 | agent-loop-and-harness-engineering | A2 | FULL | **committed + validated** | Research/05-agent-loop-and-harness-engineering.md | PASS — 342 lines, 81 sources; ~42 min, ~266k tokens; spot-checks 3/3 (one via installed-binary strings probe); freshness re-validation (4.3) confirmed novel |
| T03 | orchestration-and-multiagent | A2 | FULL | **committed + validated** | Research/06-orchestration-and-multiagent.md | PASS — 297 lines, 67 sources; ~36 min, ~184k tokens; spot-checks 3/3; D6 exclusions re-validated stronger |
| T04 | context-engineering | A2 | FULL | **running** | → Research/07-context-engineering.md | resumed 2026-07-17 same-session (SendMessage revive, spend preserved); in verification phase |
| T05 | intake-planning-spec-pipeline | A2 | FULL | **committed + validated** | Research/03-intake-planning-spec-pipeline.md | PASS — 298 lines, 90 sources; survived pause/resume (verifiers re-run); spot-checks pass |
| T06 | verification-and-quality-loops | A2 | FULL | **committed + validated** | Research/04-verification-and-quality-loops.md | PASS — 353 lines, 98 sources; survived pause/resume; spot-checks 3/3 verbatim |
| T07 | durable-state-checkpointing-recovery | B1 | FULL | pending | — | consumes G1 addendum |
| T08 | metering-quota-scheduling | B1 | FULL | pending | — | consumes G1 addendum |
| T09 | sandboxing-confinement | B1 | FULL | pending | — | consumes G1 addendum |
| T10 | memory-and-knowledge-architecture | B2 | FULL | pending | — | |
| T11 | evals-observability-benchmark | B2 | FULL | pending | — | |
| T12 | deliverables-review-git | B2 | FULL | pending | — | |
| T16 | oss-harvest-validation | B2 | FULL | pending | — | deliberately after Wave A |
| T13 | platform-stack-architecture | C | LIGHT | pending | — | |
| T17 | provider-watchlist-and-onboarding-criteria | A1b | FULL (narrow) | **committed + validated** | Research/02-provider-watchlist-and-onboarding-criteria.md | PASS — 336 lines, 72 sources; ~34 min, ~201k tokens; xAI = sanctioned lane, DeepSeek = metered-only; report 01 recommendation unchanged |
| T14 | worker-ontology-and-domain-agents | C | FULL | pending | — | |
| T15 | local-models-layer | C | FULL | pending | — | |

## Report numbering

Reports take the next free `NN` in `Research/` in completion order. Map:

| NN | T | File |
|---|---|---|
| 01 | T01 | 01-execution-engines-and-adapters.md |
| 02 | T17 | 02-provider-watchlist-and-onboarding-criteria.md |
| 03 | T05 | 03-intake-planning-spec-pipeline.md |
| 04 | T06 | 04-verification-and-quality-loops.md |
| 05 | T02 | 05-agent-loop-and-harness-engineering.md |
| 06 | T03 | 06-orchestration-and-multiagent.md |

## Session log

- **2026-07-17 — same session, RESUMED:** operator instructed resume shortly after the pause. T04 revived in the same session via SendMessage (the stopped agent had no active task; resumed from transcript with context intact) — it re-enters exactly at the verifier-launch step, so the pause cost zero research spend. Board back to `running`; next milestone unchanged (T04 validate+commit → G1 memo).

- **2026-07-17 — same session, PAUSED by operator (3rd pause):** operator asked to pause with clean resumability. T04 (only live work) stopped via TaskStop at a clean boundary — its final progress line: "Now launching the 3 adversarial verifiers in parallel, each with the same 34-claim list", i.e. research fan-out complete, verification not yet started. Same-session resume: SendMessage to the stopped T04 agent revives it with context intact (spend preserved — validated mechanism from the T05/T06 pause). Cross-session resume: relaunch T04 per runbook (fresh). All completed work is committed + pushed (reports 01–06); no uncommitted state beyond this file's pause record. Wave A remaining after resume: T04 validate+commit → G1 memo.

- **2026-07-17 — operator backlog review (discussion session, continues report-02 review):** operator worked through the cross-report decision backlog. Decisions pre-registered for the G1 memo (record, don't re-ask):
  - **Launch-lane set: v0 runs on Anthropic + Z.AI only.** Stability first, lanes later. **Report-02 OQ#1 (xAI lane) = DEFERRED** — same treatment as the Codex skip: revisit when Grok 4.5 is actually EU-available and a member holds SuperGrok/X Premium+. Standing requirement re-affirmed by operator: architecture must keep lane-addition trouble-free config-only (D3 + report-02 §5 checklist), so parked lanes can join post-v0 without rework.
  - **Report-02 OQ#2 (Synthetic vs Cerebras), OQ#3 (StepFun experiment), OQ#4 (BytePlus spike), OQ#6 (DeepSeek metered-exception ratification) = PARKED.** Metered-exception list stays EMPTY at v0 (3.10 strictest form); watch-only items stay on the §6 watchlist; revisit post-v0 on concrete need.
  - **Report-03 OQ#1 (question budgets) = fixed per-tier caps REJECTED; dynamic clarity-driven intake adopted.** Intake asks only while understanding is incomplete — stop early when clear, continue past any nominal count when not. UI: a **"Clearance" indicator (0–100% model-estimated understanding)**; the user may answer until 100% or stop at their own threshold (e.g. 85%), remaining gaps proceed as listed assumptions. Feeds the T05 spec section (intake §4.x) and UI design; note for T11: the clearance estimate is a calibration target (relates to P-T05-4 stakes-classifier eval).
  - **Report-03 OQ#3 (utility-model duty map) = v0 utility models capped at ≤8B params;** post-v0 ceiling: whatever fits RTX 3090 24 GB VRAM, chosen per duty ("best for the assigned task"). T15 benchmarks duties within that envelope (8B-class first, 24 GB-class second); the local-vs-frontier interview-phrasing sub-question stays open pending T15 numbers.
  - Report-03 OQ#2 (AC dual-phrasing): recommendation given (plain always + structured sub-line only where behavioral), operator confirmation pending. Report-03 OQ#4 (zero-interaction band): explained, operator decision pending.

- **2026-07-17 — same session, T02 complete:** T02 finished ~42 min / 25 tool uses / ~266k tokens; validated PASS (checklist; brief cross-check exact — SQ1–8 ↔ §2.1–2.8, harvest A3/A4/N1/K1 all verdicted, engine direction not reopened; spot-checks 3/3: #34629 resume-cache ~20x regression verbatim incl. exact cost table; opencode pending-permission in-memory `Map` verified at source; PreToolUse `defer` verified via docs decision-table + strings probe of the installed v2.1.211 binary — `tool_deferred` ×13, `tool_deferred_unavailable` ×8). Method disclosure: the research agent ran two sub-cent live headless probes on the operator subscription (~$0.02 API-equivalent total) to verify `--max-budget-usd`/`--max-turns` enforcement — precedent: T01's local CLI probe. **Headline:** two-tier harness is field consensus (engines own loop/permissions/compaction; durable checkpoints/inbox/ceilings/watchdogs/receipts are outer-harness = Sinet); engine state expires/corrupts/regresses → platform store authoritative (P-T01-1 reinforced); `defer` = the exit-park primitive with two traps (single-tool-call-only, cleanup-sweep evaporation); **freshness re-validation on stale resume ships nowhere — spec 4.3 is genuinely novel**; recommendation = Sinet-owned SQLite-WAL event log + run FSM, durable-execution runtimes + LangGraph rejected as spine. 5 operator items + 3 G1 spikes → G1 backlog; P-T02-1..5 filed. T04 (last A2 topic) still running.

- **2026-07-17 — same session, T03 complete:** T03 finished ~36 min / 28 tool uses / ~184k tokens; validated PASS (checklist + spot-checks 3/3: Google scaling-study headline range verbatim on the arXiv abstract with body numbers attested by the report's 3-vote adversarial pass; opencode #18100 — 47-session/20-level runaway, depth limit closed-not-planned — verbatim; Cognition 2026-04 revision all four claims verbatim). **Headline:** the field converged on D6's exact shape; delegation wins only on context-protection / read-fan-out / clean-context review; designed cost ≈2–4x single-agent (15x-class only without a context contract); **no engine enforces D6** → control-plane spawn API with trigger rubric (T-CTX/T-PAR/T-SPEC), brief/report firewall, spawn_log schema — ⚙ defaults proposed for G1 ratification. 3 operator items → G1 backlog; 4 new platform problems filed (caching-vs-subscription-quota semantics, sibling-failure containment as tested behavior, D6 conformance-by-violation tests, helper-report injection surface). T04 launched into the freed slot (target report 07).

- **2026-07-17 — operator report-02 review (discussion session):** operator read and discussed report 02. **Report-02 OQ#5 decided: class-2 gray-zone posture = gray zone as-is, one uniform policy** — class-2 lanes (Z.AI, Kimi, MiniMax, StepFun) run headless inside their whitelisted tools exactly as the vendors' own integration docs demonstrate; residual ambiguity accepted; no attended-only restriction. Generalizes the report-01 OQ#2 decision; write into the spec as a single household policy applied uniformly to all class-2 plans. Remaining report-02 §9 items (xAI lane, Synthetic-vs-Cerebras, StepFun experiment, BytePlus spike, DeepSeek metered-exception ratification) stay on the G1 backlog — all purchase/policy decisions, none block Wave A2.

- **2026-07-17 — same session, nod given + batch 2 launch:** operator approved the engine direction after the report-01 discussion (reasoning noted: an Anthropic un-pause hits `claude -p` and Agent SDK identically, so the SDK stays in research as the in-adapter alternative — matches report 01 approach D). Coordinator checked reports 02/03/04 operator backlogs before launch: all items are G1-or-later by design, nothing blocks Wave A2. T02 + T03 launched Mode A (background subagents, max effort, targets Research/05 + 06); T04 released to `ready`, launches when a slot frees. Spec operating-reality-bullet amendment added to the G1 agenda (operator hasn't decided; briefs already carry the corrected facts).

- **2026-07-17 — operator report-01 review (discussion session):** operator read and discussed report 01 in depth. **T02–T04 hold MAINTAINED** — no engine-direction nod yet; only the operator releases. Two report-01 G1-backlog items resolved early by explicit operator decision: **OQ#1 (un-pause response) = (b) interactive-only demotion** — if Anthropic revives the credit split, the Anthropic lane demotes to interactive-only and headless weight shifts to Z.AI-class/local lanes (T08 brief must treat this as pre-registered policy; note it raises the load-bearing-ness of the third-party-open lane). **OQ#2 (compliance posture) = gray zone as-is** — rely on D2/3.4 only; no additional household-sharing constraint. **OQ#5 (Codex lane) reaffirmed: skip** (matches report default; reopens only if a member holds a ChatGPT plan). Spec operating-reality bullet amendment: explained to operator, decision still open (Docs/ remains untouched). Side breadcrumb for T14/D8: SAW (github.com/bybren-llc/safe-agentic-workflow) logged in operator memory as STUDY input + post-build seed candidate for the software-dev domain.

- **2026-07-16 — same session, A2 split launch:** operator chose split launch: T05 + T06 launched now (zero coupling to report 01; targets Research/03 + 04); T02–T04 held pending the operator's engine-direction confirmation from their report-01 read (§2.1/§4/§7 suffice). Coupling analysis recorded: T02 real, T03 moderate (sibling-sessions shape), T04 weak, T05/T06 none. If a future session resumes before the nod: keep the hold; only the operator releases T02–T04.

- **2026-07-16 — setup session (Fable 5):** read all Docs/, probed subagent skill access (deep-research available in subagents → Mode A viable), built campaign plan, 16 briefs, gate template, coordinator skill. G0 presented to operator.
- **2026-07-16 — same session:** G0 answered: pilot first. Max-effort rule recorded (skill §entry-0). T01 launched in Mode A; report target `Research/01-execution-engines-and-adapters.md`.
- **2026-07-16 — same session, during hold:** operator directive: also cover xAI/Grok, DeepSeek, further Chinese providers, hosted open-weights subscriptions, and a future-provider onboarding process (xAI + DeepSeek were genuinely absent from report 01's table). Added T17 (brief + CAMPAIGN row), launched it as A1b addendum — Wave A2 hold unchanged. Architectural note recorded: future-provider extensibility is already fixed by D3 + report 01 §4 (adapter-only coupling, billing as data); T17 adds coverage + the standing onboarding/watchlist process.
- **2026-07-16 — same session, pre-A2 refresh:** shared context updated with T01's verified truth (credits claim stale/paused; dual-substrate = standing direction pending G1) + new "Prior campaign reports" section (read, cite, never re-research settled facts). T02/T03 briefs pointed at report 01 as settled input. Wave A2 remains ready-to-launch on operator go.
- **2026-07-16 — same session, T17 complete:** T17 finished ~34 min / 21 tool uses / ~201k tokens; validated PASS (checklist + 3/3 spot-checks: opencode xAI OAuth docs, DeepSeek pricing, Synthetic pricing — all verbatim). Headline: xAI is now a third-party-open sanctioned lane (rides opencode, config-only); DeepSeek has no flat lane (designated metered-exception candidate); report 01 recommendation UNCHANGED, strengthened. New problems P-T17-1/2/3 filed (auth canaries, overflow modes, region-gated model lists → T08/adapter spec). OQ#7 resolved by coordinator: watchlist implementation assigned to T11 (brief addendum). Operator decision backlog rides to G1.
- **2026-07-16 — same session, pilot complete:** T01 finished in ~48 min / 51 tool uses / ~266k subagent tokens; deep-research harness ran fully (5-angle fan-out, adversarial pass 4×SUPPORT 1×PARTIAL-REFUTE). Research agent committed its own report (cfe4dd6) — benign runbook deviation; coordinator validated afterwards: checklist PASS, citation spot-checks 3/3 verbatim (Anthropic pause article, opencode server API, opencode V2 break). **Headline:** spec's "Agent SDK → credits ⇒ no option" claim is stale (paused 2026-06-15 before effect) and its implied remedy wrong — real risk class is "all headless subscription use repriceable on ~30 days notice"; recommendation = dual substrate (pinned opencode v1.x serve per user + wrapped `claude -p` per user), Agent SDK kept as in-adapter alternative for a G1 spike. 4 new platform problems filed (P-T01-1..4 → T07/T08/spec). Spec's Operating-reality bullet flagged stale — operator decides whether to amend Docs (read-only for sessions). Mode A validated for Wave A2.
