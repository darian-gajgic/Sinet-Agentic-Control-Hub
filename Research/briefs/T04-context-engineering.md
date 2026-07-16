# T04 — Context engineering

**Wave:** A2 · **Depth:** FULL · **Report slug:** `context-engineering`

## Scope
8.4/S4.6 (the right knowledge arrives by itself — assembly side; storage side is T10), 4.3 (freshness), S1.6 (repo conventions capture), known-problem "context rot", post-mortem cost finding (per-stage context re-send ≈ the 15× trap).

## Why this gates the spec
Context assembly is where quality and cost meet: what each stage/worker sees determines both its output quality and its token bill. The spec must fix the platform's context architecture — session-per-stage vs continuous sessions, what survives compaction, how knowledge slices are selected and injected — and those choices shape the schema, the adapter usage, and the orchestration contract.

## Core question
What is state-of-the-art context engineering for long-running, multi-stage agent work in mid-2026 — assembling the right context slice per stage/worker, mitigating context rot, and avoiding per-stage re-send cost blowup — and which of these mechanisms belong in Sinet's platform layer vs its engine's native features?

## Sub-questions
1. The discipline's current shape: canonical guidance (Anthropic's context-engineering line and successors, framework blogs, practitioner consensus) — what are the agreed principles as of mid-2026, and what remains contested?
2. Context rot: current understanding (long-window degradation, distractor sensitivity — measured, not folklore) and the mitigation hierarchy (compaction, fresh sessions, retrieval, structured notes).
3. Fresh-context-per-stage vs continuous-session: evidence for each; handoff artifacts that make fresh stages work (plan files, structured state docs, progress ledgers); where Archon's `fresh_context` pattern (A4, context half) sits against current practice.
4. Compaction: engine-native compaction quality/controls; what a platform should persist OUTSIDE the window so compaction is safe (decisions, acceptance criteria, file paths).
5. Deterministic context injection (S4.6): selecting the relevant slice (project conventions, playbook, danger zones) per task — rules-based vs retrieval-based selection; provenance ("what was injected" must appear in the trace — auditable memory use).
6. Repo/project convention files (AGENTS.md-class): mid-2026 conventions, what measurably helps, size discipline, generation and upkeep (S1.6's onboarding task).
7. Prompt/context caching: engine and provider caching behavior relevant to wrapped-CLI and API substrates; what caching means under flat-rate subscriptions (window pressure, latency) vs metered (dollars) — D5 lens.
8. Context budgets: per-stage token budgeting as an architectural control; who does this and how it's enforced.
9. Measuring context quality: context-relevance evals, ablations — anything practical for a solo maintainer.

## Constraints that bind this topic
D6 (helpers get sliced context by design — quarantine doubles as injection containment), D7 (checkpoints must capture enough context state to resume), 11.1/S2.1 (whatever is injected must be traceable), adopt-don't-fork (compaction/caching only via engine config/APIs).

## Harvest-map items to verdict
A4 (`fresh_context` — context half), N18 (repo onboarding template — against current AGENTS.md-class practice), deepclaude/"Design Space" reference row (harness internals we deliberately don't rebuild — confirm the boundary).

## Sources to prioritize
Anthropic engineering (context engineering, compaction, memory tooling); langchain.com/blog and peer framework posts; long-context degradation studies (2025–2026); practitioner essays with ablations; engine docs on compaction/caching.

## Decisions this feeds
G1: session/stage model (with T02), context-assembly architecture. Spec: framing/injection design, trace requirements. T10 (memory storage that feeds assembly).
