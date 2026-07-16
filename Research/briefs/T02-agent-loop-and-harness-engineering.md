# T02 — Agent loop & harness engineering

**Wave:** A2 · **Depth:** FULL · **Report slug:** `agent-loop-and-harness-engineering`

## Scope
D7 (checkpoint-and-gate), 4.3 (blocked ≠ failed; freshness-checked resume), 4.5 (a stop that stops; maintenance mode), 4.6 (nothing fails silently), 3.7 (hard ceilings, circle/silence detection), 3.8 (runs survive disasters — loop-level view; storage mechanics live in T07).

## Why this gates the spec
Sinet's reliability promises (pause for hours on a human gate, resume after crashes and quota storms, never lose paid work, never act outward ungated) are properties of the **harness around the engine**, not the engine itself. This topic decides what the platform layer must implement vs what a mid-2026 engine already provides — the boundary that killed Nexus (it forked the engine to get harness features).

## Core question
What does a state-of-the-art agent harness look like in mid-2026 — the control layer wrapped around an adopted engine — and specifically: how do the best systems implement checkpoint-per-paid-call, human-gate pauses that resume hours later, freshness re-validation on stale resumes, runaway/stall detection, hard ceilings, and clean cancellation, **without modifying the engine**?

## Sub-questions
1. Harness engineering as a discipline mid-2026: what layers do leading harnesses (Claude Code and peers) own — hooks, permission callbacks, compaction, tool policies, lifecycle events — and which of those does a wrapping platform get for free vs re-own?
2. Checkpoint semantics when the engine holds session state: what *is* a checkpoint (session id + message index? transcript snapshot? artifact state?) in real wrapper platforms; recovery fidelity per approach.
3. Pause/resume across hours on human input: patterns for parking a run (blocked-on-approval) without holding processes/context open; what Archon's `interactive: true` persistence actually does (validate harvest A3); what others do.
4. Freshness re-validation on resume (spec 4.3: default 24 h threshold; repo moved on, sources changed): does ANY current system do this? If none, propose the mechanism from adjacent evidence (cheap re-plan validation passes) and say it's novel.
5. Runaway detection: loop/circle detection (same-state revisiting, no-progress heuristics), silence/stall detection, spend-anomaly triggers — what's proven, what's snake oil.
6. Ceilings enforcement (time/steps/cost) — enforcement points that actually bound damage when the engine is a black-box subprocess vs an API session.
7. Clean cancellation: killing in-flight tool calls and subprocesses without orphaned side effects; maintenance-mode draining (finish in-flight, accept nothing).
8. Failure-mode taxonomy for harnesses (what breaks in production: SSE stalls, half-written state, double resumes) and the defensive idioms.

## Constraints that bind this topic
D3 (harness sits above the adapter contract), D7 (its central invariant), adopt-don't-fork (harness features must be achievable via wrapping/config/hooks only — any candidate pattern requiring engine surgery is disqualified), 4.2 (outward effects only as proposals — harness collects, gate releases).

## Harvest-map items to verdict
A3 (`interactive: true` pause/resume), A4 (`fresh_context` per iteration — loop-hygiene half; context half is T04's), N1 (orphan-harvest ladder — as *pattern to understand now*, port decision belongs to T07/G2), K1 (shared-workspace permission queue pattern).

## Sources to prioritize
Anthropic engineering posts on harness/agent design; engine docs (hooks/permissions/events APIs); durable-agent-runtime writeups; framework blogs (LangChain/LangGraph interrupts & human-in-the-loop, and peers); production postmortems of long-running agent systems.

## Decisions this feeds
G1: platform-vs-engine responsibility boundary; session model direction. Spec: run lifecycle state machine, harness layer. T07 (state storage), T08 (limit-event handling hooks).
