# Campaign state — live pointer

> Any session resuming the campaign: read this file first, then `CAMPAIGN.md`, then follow `.claude/skills/research-campaign/SKILL.md`. Update this file before and after every action. This file is the single source of truth for progress.

**Campaign status:** WAVE A2 — resumed from pause #2 IN-PLACE (operator "continue … where it stopped … quality most important", 2026-07-16): T05 and T06 resumed from their own transcripts via SendMessage — no research spend lost. T05 instructed to re-run any verifier its pause killed (quality over speed); T06 writing its report from completed verification. **T02–T04 still held** for the operator's engine-direction nod.
**Next action:** (1) on T05/T06 completion: validate → commit → digest. (2) On operator's engine-direction nod: launch T02, then T03/T04 as slots free.
**Last updated:** 2026-07-16 (setup session, post-pilot).
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
| T02 | agent-loop-and-harness-engineering | A2 | FULL | **held** | → Research/05-… (assigned at launch) | awaits operator engine-direction nod (leans on report 01) |
| T03 | orchestration-and-multiagent | A2 | FULL | **held** | → (assigned at launch) | same hold as T02 |
| T04 | context-engineering | A2 | FULL | **held** | → (assigned at launch) | same hold (weak coupling; held for cleanliness) |
| T05 | intake-planning-spec-pipeline | A2 | FULL | **running (resumed in-place)** | → Research/03-intake-planning-spec-pipeline.md | transcript-resumed 2026-07-16; finishing verification (re-running killed verifiers) then synthesis |
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
| 03 | T05 | 03-intake-planning-spec-pipeline.md (in progress) |
| 04 | T06 | 04-verification-and-quality-loops.md |

## Session log

- **2026-07-16 — same session, A2 split launch:** operator chose split launch: T05 + T06 launched now (zero coupling to report 01; targets Research/03 + 04); T02–T04 held pending the operator's engine-direction confirmation from their report-01 read (§2.1/§4/§7 suffice). Coupling analysis recorded: T02 real, T03 moderate (sibling-sessions shape), T04 weak, T05/T06 none. If a future session resumes before the nod: keep the hold; only the operator releases T02–T04.

- **2026-07-16 — setup session (Fable 5):** read all Docs/, probed subagent skill access (deep-research available in subagents → Mode A viable), built campaign plan, 16 briefs, gate template, coordinator skill. G0 presented to operator.
- **2026-07-16 — same session:** G0 answered: pilot first. Max-effort rule recorded (skill §entry-0). T01 launched in Mode A; report target `Research/01-execution-engines-and-adapters.md`.
- **2026-07-16 — same session, during hold:** operator directive: also cover xAI/Grok, DeepSeek, further Chinese providers, hosted open-weights subscriptions, and a future-provider onboarding process (xAI + DeepSeek were genuinely absent from report 01's table). Added T17 (brief + CAMPAIGN row), launched it as A1b addendum — Wave A2 hold unchanged. Architectural note recorded: future-provider extensibility is already fixed by D3 + report 01 §4 (adapter-only coupling, billing as data); T17 adds coverage + the standing onboarding/watchlist process.
- **2026-07-16 — same session, pre-A2 refresh:** shared context updated with T01's verified truth (credits claim stale/paused; dual-substrate = standing direction pending G1) + new "Prior campaign reports" section (read, cite, never re-research settled facts). T02/T03 briefs pointed at report 01 as settled input. Wave A2 remains ready-to-launch on operator go.
- **2026-07-16 — same session, T17 complete:** T17 finished ~34 min / 21 tool uses / ~201k tokens; validated PASS (checklist + 3/3 spot-checks: opencode xAI OAuth docs, DeepSeek pricing, Synthetic pricing — all verbatim). Headline: xAI is now a third-party-open sanctioned lane (rides opencode, config-only); DeepSeek has no flat lane (designated metered-exception candidate); report 01 recommendation UNCHANGED, strengthened. New problems P-T17-1/2/3 filed (auth canaries, overflow modes, region-gated model lists → T08/adapter spec). OQ#7 resolved by coordinator: watchlist implementation assigned to T11 (brief addendum). Operator decision backlog rides to G1.
- **2026-07-16 — same session, pilot complete:** T01 finished in ~48 min / 51 tool uses / ~266k subagent tokens; deep-research harness ran fully (5-angle fan-out, adversarial pass 4×SUPPORT 1×PARTIAL-REFUTE). Research agent committed its own report (cfe4dd6) — benign runbook deviation; coordinator validated afterwards: checklist PASS, citation spot-checks 3/3 verbatim (Anthropic pause article, opencode server API, opencode V2 break). **Headline:** spec's "Agent SDK → credits ⇒ no option" claim is stale (paused 2026-06-15 before effect) and its implied remedy wrong — real risk class is "all headless subscription use repriceable on ~30 days notice"; recommendation = dual substrate (pinned opencode v1.x serve per user + wrapped `claude -p` per user), Agent SDK kept as in-adapter alternative for a G1 spike. 4 new platform problems filed (P-T01-1..4 → T07/T08/spec). Spec's Operating-reality bullet flagged stale — operator decides whether to amend Docs (read-only for sessions). Mode A validated for Wave A2.
