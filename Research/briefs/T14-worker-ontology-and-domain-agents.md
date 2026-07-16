# T14 — Worker ontology & domain-specific agents

**Wave:** C · **Depth:** FULL · **Report slug:** `worker-ontology-and-domain-agents`

## Scope
§7 complete (self-composing workforce: 7.1 composition on demand, 7.2 validation-that-means-something, 7.3 versioned assets + model-change revalidation, 7.4 recurring registration, 7.5 specialization over time, 7.6 honest capability marking, 7.7 accountable routing, 7.8/D8 template→overlay→instance), 2.1/2.3 (domain acceptance with maturity honesty; task-type detection), 5.7 (per-domain judges), D10 (promotion gating), 14.4 (no standing army), 2.6 (deterministic automations born as machinery).

## Why this gates the spec
D8's ontology (template → overlay → instance) is day-one schema (15.6) even though the self-composing workforce matures post-v0. And "the platform builds its own workers following current best practice" (7.1) is the platform's boldest promise — this topic finds out what that credibly means in mid-2026.

## Core question
How should Sinet represent workers as versioned assets (template → overlay → instance, D8), compose new ones automatically when an uncovered task type arrives (7.1), validate them in a way that means something (7.2), and revalidate them across model changes (7.3) — and what does mid-2026 evidence say about domain-specialized agents actually outperforming well-contexted generalists?

## Sub-questions
1. Agent-definition formats mid-2026: markdown+frontmatter subagent definitions (coding-harness style), engine-native agent configs, structured DSLs — convergence, portability, and which format a *platform* should store as its template source of truth (compiling down to engine formats — harvest O3's "configuration over engine" idea).
2. The specialization question, with evidence: do domain-tuned prompts/tools/knowledge measurably beat a strong generalist model given the same context in 2026? Where does specialization pay (tools/knowledge) vs cosplay (persona prompts)?
3. Meta-agents / agent composers: systems that generate agent definitions on demand today — credibility, guardrails, failure modes; what "following the latest state of the art" means operationally (does the composer consult live best-practice sources — P47 applied to worker-building?).
4. Validation of generated workers (7.2 = structural checks + human sign-off, explicitly NOT a quality guarantee): config linting, permission audits (4.4 minimal powers), dry runs on sample tasks — prior art for agent-definition test harnesses; first-N-outputs supervised operation (degraded mode 7.6).
5. Versioning + revalidation (7.3): worker-version ↔ outcome records; model-change triggers flagging every tuned worker; eval-pinned versions (bridge to T11's regression evals); rollback semantics.
6. Overlay design (7.8, 10.1): per-user lessons/preferences layered on shared templates without cross-user leakage — storage and merge patterns (bridge to T10's scopes); instance working notes expiring with the task.
7. Task-type detection & routing to workers (2.3, 7.7): classifier design (cheap, wrong-is-cheap per 2.5), routing rationale records, gap detection ("no worker fits" → compose or advise per 2.7).
8. Deterministic automations as workers (2.6): generated connector-class machinery (C0) — born versioned, supervised first run, minimal permissions, rollback; how current automation platforms govern generated workflows (reviewable definitions — "reviewed like code, managed like a worker").
9. Template promotion & sharing (D10, 12.5): personal-by-default, operator-gated household promotion, provenance on shared templates.

## Constraints that bind this topic
D8 (the ontology is fixed — research fills in its mechanics), D10, 14.2 (workers can NEVER alter their own permissions/budgets/gates), 14.4 (single-agent-first: composition adds machinery only when a task earns it), 7.6 (no unsupervised operation in unverified domains — structural, not advisory).

## Harvest-map items to verdict
N22 (retired specialist prompt bodies as palette raw material), S1/SAW (role palette, pruned — options never defaults), O3 (engine config formats as compile target), anti-harvest "18-specialist standing roster" row.

## Sources to prioritize
Agent-definition format docs (engines, harnesses — primary), meta-agent/composer writeups with results (2025–2026), specialization-vs-generalist evidence (evals, not vibes), automation-platform governance docs, framework blogs on multi-agent role design.

## Decisions this feeds
G3: worker template schema (day-one, 15.6), composition pipeline shape (post-v0 but schema-relevant), validation harness design. Spec: worker ontology section, routing section.
