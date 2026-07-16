# T07 — Durable state, checkpointing & crash recovery

**Wave:** B1 (consumes the G1 addendum: engine + session-model decisions) · **Depth:** FULL · **Report slug:** `durable-state-checkpointing-recovery`

## Scope
3.8 (runs survive disasters: crash/restart/sleep/limit — finished work recovered, in-progress resumes from checkpoint, re-spend bounded, outward actions never repeated), 4.8/D7 (mechanics), 4.5 (maintenance mode), S1.11 (parallel same-project coordination state), 11.3 (state snapshot mechanics belong to T12; here only what must BE durable).

## Why this gates the spec
"Restarts never lose paid AI work" was Nexus's most valuable verified property, and under subscription economics wasted tokens are wasted *windows*. The spec's state layer — what is persisted, when, and how recovery classifies interrupted work — must be designed before the schema exists, and it depends directly on G1's engine decision (what session state the engine exposes).

## Core question
How should a single-host platform persist run state in mid-2026 so that platform crashes, host sleeps, restarts, and provider-limit interruptions never lose paid AI work — what are current best practices for durable, resumable agent execution (checkpoint content, orphan recovery, idempotent effects, atomic queueing), and which existing machinery (durable-execution frameworks vs DIY-on-SQLite) fits a solo-maintained laptop platform?

## Sub-questions
1. Durable-execution landscape mid-2026 for this scale: Temporal-class engines, lighter-weight durable runtimes (DBOS-class, Inngest-class, others current), plain SQLite state machines — operational cost for ONE maintainer on ONE host vs what each actually buys; do any have first-class agent/LLM-step support now?
2. What a checkpoint IS when an adopted engine holds the session (per G1): engine-session pointer + platform-side event log? Full transcript capture? Artifact snapshots? Recovery fidelity and re-spend bounds per design — "checkpoint after every paid model call" made concrete for the chosen substrate.
3. Orphan recovery: on restart, classifying interrupted work as still-running / finished-during-outage (harvest the result!) / dead — the harvest map's N1 ladder is the reference; find public equivalents and current idioms (process supervision, session-history harvesting, lease/heartbeat designs).
4. Exactly-once outward effects: proposal gating (4.2) + idempotency keys + effect journals — patterns ensuring a crash between "approved" and "executed" never double-sends/pushes/publishes.
5. Atomic work claiming and queues on SQLite: CAS claiming (N2 reference), WAL-mode concurrency realities, lease expiry, per-model slot gates — versus adopting a small job-queue library; current best practice.
6. Event-sourcing vs mutable-state-plus-audit for run lifecycle: which do comparable systems use; replay/debugging value vs complexity for bus-factor-1.
7. Crash-consistency verification culture: kill -9 / power-cut test harnesses for state layers; sleep/wake edge cases on laptops (clock jumps, network changes mid-run).
8. Maintenance mode (4.5): drain semantics (finish in-flight, accept nothing) and how systems implement stop-the-world safely around in-flight paid calls.
9. Multi-task same-project coordination state (S1.11): artifact-claim/lock registries at plan time; freshness re-check triggers when a sibling lands (mechanics; policy lives in T02's 4.3 findings).

## Constraints that bind this topic
D7 (the invariant this topic implements), D3 (checkpointing works through the adapter contract uniformly across substrates), D9 (accepted work exits through git — the state layer holds everything before acceptance), 15.6 (every record owner-attributed from day one).

## Harvest-map items to verdict
N1 (orphan-harvest ladder + control/execution split), N2 (CAS claiming + slot gates), N4 (dispatch state machine, user-facing status orthogonal to machinery status), A5 (Archon's 7-table schema as minimal-set sanity reference).

## Sources to prioritize
Durable-execution project docs/comparisons (current, not 2024 hype), SQLite-in-production engineering posts (WAL, litestream-class replication), agent-platform postmortems on lost work, engine docs on session persistence/resume (per G1 choice).

## Decisions this feeds
G2: state layer design (framework vs DIY, checkpoint content, recovery ladder). Spec: schema core, run lifecycle persistence, recovery procedures (tested, not assumed).
