# Campaign state — live pointer

> Any session resuming the campaign: read this file first, then `CAMPAIGN.md`, then follow `.claude/skills/research-campaign/SKILL.md`. Update this file before and after every action. This file is the single source of truth for progress.

**Campaign status:** WAVE A1 COMPLETE — pilot report validated (checklist PASS, 3/3 citation spot-checks confirmed). Awaiting operator review of `Research/01-execution-engines-and-adapters.md` before Wave A2 (per G0).
**Next action:** operator reviews the pilot report → on approval, launch Wave A2 (T02–T06, Mode A, 2 concurrent).
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
| T02 | agent-loop-and-harness-engineering | A2 | FULL | pending | — | unblocks after pilot validated |
| T03 | orchestration-and-multiagent | A2 | FULL | pending | — | |
| T04 | context-engineering | A2 | FULL | pending | — | |
| T05 | intake-planning-spec-pipeline | A2 | FULL | pending | — | |
| T06 | verification-and-quality-loops | A2 | FULL | pending | — | |
| T07 | durable-state-checkpointing-recovery | B1 | FULL | pending | — | consumes G1 addendum |
| T08 | metering-quota-scheduling | B1 | FULL | pending | — | consumes G1 addendum |
| T09 | sandboxing-confinement | B1 | FULL | pending | — | consumes G1 addendum |
| T10 | memory-and-knowledge-architecture | B2 | FULL | pending | — | |
| T11 | evals-observability-benchmark | B2 | FULL | pending | — | |
| T12 | deliverables-review-git | B2 | FULL | pending | — | |
| T16 | oss-harvest-validation | B2 | FULL | pending | — | deliberately after Wave A |
| T13 | platform-stack-architecture | C | LIGHT | pending | — | |
| T14 | worker-ontology-and-domain-agents | C | FULL | pending | — | |
| T15 | local-models-layer | C | FULL | pending | — | |

## Report numbering

Reports take the next free `NN` in `Research/` in completion order. Map:

| NN | T | File |
|---|---|---|
| 01 | T01 | 01-execution-engines-and-adapters.md |

## Session log

- **2026-07-16 — setup session (Fable 5):** read all Docs/, probed subagent skill access (deep-research available in subagents → Mode A viable), built campaign plan, 16 briefs, gate template, coordinator skill. G0 presented to operator.
- **2026-07-16 — same session:** G0 answered: pilot first. Max-effort rule recorded (skill §entry-0). T01 launched in Mode A; report target `Research/01-execution-engines-and-adapters.md`.
- **2026-07-16 — same session, pilot complete:** T01 finished in ~48 min / 51 tool uses / ~266k subagent tokens; deep-research harness ran fully (5-angle fan-out, adversarial pass 4×SUPPORT 1×PARTIAL-REFUTE). Research agent committed its own report (cfe4dd6) — benign runbook deviation; coordinator validated afterwards: checklist PASS, citation spot-checks 3/3 verbatim (Anthropic pause article, opencode server API, opencode V2 break). **Headline:** spec's "Agent SDK → credits ⇒ no option" claim is stale (paused 2026-06-15 before effect) and its implied remedy wrong — real risk class is "all headless subscription use repriceable on ~30 days notice"; recommendation = dual substrate (pinned opencode v1.x serve per user + wrapped `claude -p` per user), Agent SDK kept as in-adapter alternative for a G1 spike. 4 new platform problems filed (P-T01-1..4 → T07/T08/spec). Spec's Operating-reality bullet flagged stale — operator decides whether to amend Docs (read-only for sessions). Mode A validated for Wave A2.
