# T03 — Orchestration & multi-agent architecture (within D6)

**Wave:** A2 · **Depth:** FULL · **Report slug:** `orchestration-and-multiagent`

## Scope
D6 (single coordinator, isolated helpers, depth cap 2, no lateral messaging, logged spawns), 2.4 (automatic orchestration within the fixed topology), 7.7 (routing accountability), 14.1 (no chatting swarms), 14.4 (no standing army — machinery only when a task earns it).

## Why this gates the spec
The topology is FIXED (D6) — but everything inside it is open: when to spawn helpers at all, how to decompose, how to hand context down and results up, and how to stop multi-agent cost multiplication (the post-mortem measured a ~15×-class overhead) from eating the platform's value. This is where "orchestration engineering / deep agents / dynamic subagents" best practice lands in the spec.

## Core question
Within a fixed hub-and-spoke topology (one coordinator per task, isolated helpers, sub-helper depth ≤ 2, no lateral messaging), what are mid-2026 best practices for **when** to decompose into helpers, **how** to pass context and results without token blowup, and **how** dynamic (per-task, on-demand) helper composition should work — with evidence, not vendor enthusiasm?

## Sub-questions
1. Where has the single-agent-vs-multi-agent debate landed mid-2026? Track the argument's evolution (Cognition's "don't build multi-agents", Anthropic's multi-agent research system, LangChain's deep-agents line and its current form, others) into the present: for which task shapes does delegation demonstrably win, and by how much?
2. The read/write asymmetry: parallel helpers for research/read fan-out vs conflicts in write/code work — current evidence and idioms (e.g. worktree isolation for parallel writers).
3. Deep-agents pattern today (planning artifact + filesystem + subagents + long prompts): current reference implementations, measured results, failure modes.
4. Context passing: brief-in/report-out contracts, shared filesystem state, structured handoffs — what keeps helpers effective without re-sending everything (the 15× trap); context quarantine (helper sees only its slice) as an injection-blast-radius tool.
5. Dynamic composition: how do current systems decide *at runtime* whether to spawn helpers and which specialists to instantiate (vs static pipelines)? Spawn budgeting, depth limiting, spawn-reason logging in the wild.
6. Cost accounting for multi-agent runs: measured multipliers in 2025–2026 systems, and which mitigations (caching, small-helper models, context reuse) actually move the number.
7. Coordinator failure semantics: helper dies / returns junk / exceeds budget — containment and retry patterns that don't cascade.
8. Evidence against peer-mesh/group-chat topologies (the excluded alternative): confirm the exclusion still matches 2026 evidence — cascade errors, cost, debuggability.

## Constraints that bind this topic
D6 is absolute (findings recommending lateral messaging or uncapped depth are out of scope — note them under What-NOT-to-use with reasons). D7 (helper work checkpoints like any paid work), D5 (helper model choice routes by consumption pressure between flat-rate options), 14.4 (single-agent-first: default formation is ONE agent; helpers must be earned per task).

## Harvest-map items to verdict
Anti-harvest "peer-mesh/group-chat" exclusion (re-validate the evidence base), anti-harvest "18-specialist standing roster" (single-agent-first), deepagents reference row (vocabulary/sanity-check status — still redundant as a dependency?), N22 disposition (specialist prompt bodies as palette raw material — sanity only; full worker composition is T14).

## Sources to prioritize
Anthropic engineering (multi-agent research system + successors); Cognition/Devin essays and rebuttals; langchain.com/blog deep-agents line; academic multi-agent evaluations 2025–2026; production reports with token/cost numbers.

## Decisions this feeds
G1: default orchestration policy within D6 (when to spawn, context-passing contract, spawn budget defaults). Spec: coordinator/helper lifecycle. T14 (worker composition rides on this).
