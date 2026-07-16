# Campaign state — live pointer

> Any session resuming the campaign: read this file first, then `CAMPAIGN.md`, then follow `.claude/skills/research-campaign/SKILL.md`. Update this file before and after every action. This file is the single source of truth for progress.

**Campaign status:** WAVE A1 — T01 pilot running (Mode A, launched 2026-07-16).
**Next action:** on pilot completion: coordinator validates → commits → operator reviews the report (quality/depth/format) before Wave A2 launches.
**Last updated:** 2026-07-16 (setup session, post-G0).
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
| T01 | execution-engines-and-adapters | A1 | FULL | **running** | → Research/01-execution-engines-and-adapters.md | pilot, Mode A, launched 2026-07-16 |
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
| — | — | (none yet) |

## Session log

- **2026-07-16 — setup session (Fable 5):** read all Docs/, probed subagent skill access (deep-research available in subagents → Mode A viable), built campaign plan, 16 briefs, gate template, coordinator skill. G0 presented to operator.
- **2026-07-16 — same session:** G0 answered: pilot first. Max-effort rule recorded (skill §entry-0). T01 launched in Mode A; report target `Research/01-execution-engines-and-adapters.md`.
