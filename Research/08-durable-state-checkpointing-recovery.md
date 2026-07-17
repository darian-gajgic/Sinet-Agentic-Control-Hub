# 08 — Durable state, checkpointing & crash recovery

**Topic:** T07 · **Wave:** B1 · **Depth:** FULL · **Written:** 2026-07-17
**Method:** deep-research harness — 5 fan-out search angles (durable-execution landscape / agent checkpoint & recovery patterns / orphan recovery & leases / exactly-once outward effects / SQLite state layers & crash testing), ~95 extracted findings, primary-source fetching, then a **3-vote adversarial verification pass over the 12 most load-bearing claim bundles: 36 votes — 27 SUPPORT / 9 PARTIAL / 0 REFUTE**; every PARTIAL's precision corrections are folded into this text (two claims were corrected by majority vote: Restate pause/resume is a v1.6.0 feature, and Litestream v0.5.7+ restores v0.3 backups). One verifier live-tested the SQLite busy-timeout bypass and systemd transient-unit reload survival. Engine facts measured by spikes G1-S1/S2/S3 are consumed as settled inputs and cited, never re-researched. All URLs accessed 2026-07-17. Engine/inference spend: $0.

---

## 1. Scope

Feature-list items covered (per brief T07): **3.8** (runs survive disasters — finished work recovered, in-progress resumes from checkpoint, re-spend bounded, outward actions never repeated), **4.8/D7** (checkpoint-and-gate mechanics — the invariant this topic implements at the storage layer), **4.5** (maintenance mode), **4.2** (effect-gating mechanics between "approved" and "executed"), **S1.11** (parallel same-project coordination state), **11.3** (only *what must be durable* and the dump-friendly shape — snapshot scheduling/encryption mechanics belong to T12), **15.6** (every record owner-attributed from day one).

Binding inputs consumed as settled: the G1 addendum (run-lifecycle FSM + platform-owned SQLite-WAL append-only event log; fresh-context-per-stage on the Task Context Ledger; dual substrate with CLI-wrap confirmed at S1; hold-vs-park 10 min, drain grace 15 min, systemd transient units adopted, pause-and-flag containment — all ⚙ operator-editable settings per the G1 rider), the three spike reports (`Research/spikes/G1-S{1,2,3}-*.md`), and reports 01/05/07. Report 05 designed the **harness** (FSM, watchdogs, ceilings, parking policy); this report designs the **state layer beneath it**: what is durable, the schema core, checkpoint content per substrate, the recovery ladder, the effect journal, atomic claiming, and the crash-test practice. Where report 05 already settled a mechanism, it is cited, not re-derived.

---

## 2. Current state of the art (mid-2026)

### 2.1 Durable-execution landscape: the field converged on "persistent memoization" — and, at this scale, on SQLite being enough

The category clarified substantially since report 05's survey, in Sinet's favor — and one of report 05's rejection grounds went stale:

- **Temporal** still lists SQLite persistence as "meant only for development and testing, not production usage" (a Visibility store is additionally required), and still hard-caps a workflow's event history at **51,200 events / 50 MB — the workflow is *terminated* when exceeded** (warnings at 10,240/10 MB; separate 2,000-Update/10,000-Signal limits) [S1, S2, P; 3-vote SUPPORT]. Its 2025–26 AI push is real — OpenAI Agents SDK integration GA 2026-03-23, agentic sandboxes April 2026, LLM calls as journaled Activities [S3, P] — but the integration assumes the agent loop runs *inside* Temporal workers, the exact opposite of Sinet's adopt-engines-unmodified posture.
- **DBOS** — **correction to report 05: the "requires Postgres" rejection ground is stale.** Since dbos-transact-py 1.13.0 (2025-09-02), **SQLite is the default system database for new Python apps** ("By default, DBOS uses SQLite"), with hardening fixes into 2026 (2.9.0 threading, 2.10.0 isolation); DBOS Transact is an MIT library with no server ("entirely contained in this open-source library") [S4, S5, P; 3-vote verified with scope corrections]. Scope, per verification majority: the SQLite default is a **Python-SDK fact** — the TypeScript SDK has no SQLite backend (its docs remain Postgres-first, with at least one page inconsistency), and Go gained SQLite as an opt-in only in mid-2026 (v0.16/0.17, June 2026). Agent support is first-class (OpenAI Agents SDK backed by SQLite, Pydantic AI, Google ADK plugin) [S6, P]. DBOS is therefore the strongest framework challenger and is argued fresh in §3-B — the rejection survives on different grounds.
- **Restate** v1.7.2 (2026-07-06): single binary, RocksDB storage, SQL introspection, BUSL-1.1 license converting to Apache-2.0 after 4 years with internal self-host use expressly permitted. **Manual pause/resume of invocations shipped in v1.6.0 (2026-01-30**, which also made pause-not-kill the default on retry exhaustion**); v1.7 made pause persisted — and abort-immediate**: pausing aborts the in-flight attempt rather than draining to a safe point, so un-journaled work re-executes on resume [S7, S8, P; corrected 2-vs-1]. Still a second always-on stateful service; report 05's verdict stands.
- **Inngest** self-host keeps the queue and state store in an **in-memory Redis with periodic snapshots to SQLite** ("including prior to shutdown") — so a SIGKILL or forced sleep between snapshots loses queue/in-flight state (a sound inference from the documented architecture, not vendor-stated; flagged as such). Its flagship `step.ai.infer()` offloads inference to Inngest's cloud [S9, P]. Rejection confirmed and sharpened: this is precisely the failure class Sinet exists to prevent.
- **The 2026 "you don't need a framework at this scale" current is now mainstream, not contrarian.** Armin Ronacher's **Absurd** (Apache-2.0, launched 2025-11-03, production report 2026-04-04): durable execution as one SQL file of stored procedures — a checkpoint is a stored step result; on failure the task function re-runs and stored results are substituted; **explicitly no deterministic replay** ("you can call Math.random() or datetime.now()"); agent loops are the flagship use case [S10, P]. Simon Willison ported the design to SQLite as a research PoC (2025-12-24) [S11]; Gunnar Morling's Persistasaurus builds a durable-execution engine on SQLite in <1,000 lines and concludes durable execution is "persistent implementation of the memoization pattern" — "if you are building a self-contained AI agent… an embedded database such as SQLite would make for a great store" [S12, P]; Obelisk (single binary, SQLite default, but WASM-component-only execution — cannot wrap native engines) argues "SQLite is All You Need for Durable Workflows" [S13, P]; byteiota states the graduation threshold plainly: SQLite-based durability by default, Temporal-class only for multi-tenant/multi-region coordination [S14, S].
- **The structural gap no framework fills** (and the reason the G1 direction is right regardless of DBOS's SQLite reversal): every surveyed runtime checkpoints **discrete request/response step results**. None models Sinet's actual step — a **long-lived interactive engine subprocess with streaming output and human gates** — as a journaled unit; a 2026 survey of durable agent runtimes makes the same point and adds two design inputs Sinet adopts below: durable approvals must record "who approved, when, what exact artifact they saw, a hash or version… expiry or reuse policy," and checkpoint records should carry model/prompt/tool version fields because version drift breaks replay [S15, S].

### 2.2 What a checkpoint is: the field's answers, their failure records, and the concrete Sinet answer

**Field survey (extends report 05 §2.2 with post-G1 evidence):**

- **LangGraph checkpointers** remain the cautionary tale: ~100 rows per graph invocation with **no TTL/retention machinery** (open request since 2025-04) [S16, P]; a 2026-05 issue (no maintainer response at access date) measures **85.3% storage bloat and 37.8% token overhead** from duplicated channel state, and documents **corrupt payloads being forwarded unvalidated** into subsequent turns; sibling issues show unvalidated writes permanently corrupting checkpoints [S17, P]. Lessons made mechanism in §4: platform-owned retention from day one, validate-before-persist, refs-not-blobs.
- **OpenHands** V1 is fully event-sourced by construction (all components read/append one chronological log; replayable) — and is **silent on workspace recovery**: event-log authority stops at the conversation boundary [S18, P]. The same split Sinet has: the event log carries run state; artifacts need their own snapshot mechanism.
- **The vendor's own doctrine, verbatim** (Claude Agent SDK sessions docs): "Sessions persist the conversation, **not the filesystem**" and "Capture the results you need… as application state and pass into a fresh session's prompt" — and resume **silently returns a fresh session on cwd mismatch** [S19, P]. First-party endorsement of P-T01-1: platform state is the record; engine sessions are a resume optimization.
- **The big-money answer is machine snapshots** — Devin's blockdiff snapshots a 20 GB VM in ~200 ms via XFS reflinks; Factory's Droid keeps filesystem+process memory [S20, P]. The laptop-scale analogue is git-based workspace snapshots: Gemini CLI's checkpoint trio (shadow-git snapshot + conversation + pending tool call, atomically restorable) remains the published reference shape [S21, P]; **jj (Jujutsu)** is the emerging adopt-not-build option — it snapshots the working copy before every operation and keeps an operation log with out-of-order undo, and 2026 tooling embeds it as an agent VCS engine ("jj local, git canonical remote") [S22, P/S].
- **Google ADK** session state is never updated in place — `state_delta` events appended per turn [S23, P]; **OpenAI Agents SDK** serializes full run state including pending approvals and usage (its `SQLiteSession` default is in-memory — a documented trap) [S24, P].
- **War stories new this run:** a public multi-tenant agent gateway's issue tracker documents restart auto-resume racing an inbound message into **duplicate agent instances with every hook and tool firing twice**, zombie sessions delivering into both old and new sessions, and a reset that failed to stop an in-flight run which then published into the reset session [S25, P — single project, flagged]; Claude Code's own 12-bug multi-agent post-mortem (2026-04-28) shows work being lost **without process death** when durable state lives only in the context window (plan → compact → drift; 30+ duplicate task creations with no dedup) [S26, P]. Both reinforce: resume must be fenced against new work, and durable state lives in the platform store, not the conversation.

**The Sinet checkpoint, made concrete (D7 + spike S1/S3 facts):** one platform record per paid model call, written from the adapter's stream events in the same transaction as the run-event append — containing (a) the **per-call usage block** (identical schema on CLI and SDK per S1: input/output tokens, `cache_read`/`cache_creation` with TTL buckets, cost fields); (b) the **session cursor** — substrate, engine session id *as reported by `system/init`* (never the requested id, per S1), message index, cwd key, transcript path; (c) the **Task Context Ledger revision** (content or content-hash — the ledger *is* the D7 context payload per G1/R07); (d) the **artifact snapshot ref** (opencode lane: native per-step snapshot id from its SQLite store, per S3; Claude lane: platform-owned snapshot commit in the run worktree — N9's `snapshot_commit`, with jj as a post-v0 candidate); (e) **version fields**: model id, invocation-config fingerprint (settings content hash + permission mode + model — exactly S2's park-record set), tool/prompt schema versions [per S15's drift warning]. Re-spend after any disaster is then bounded *structurally* by "work since the last paid call" — and *economically* by S1's measured cache cliff (a resume inside the 1 h subscription TTL costs ~1/6–1/16 of a cold one), which the scheduler already consumes via T08.

**Recovery-fidelity taxonomy** (report 05 §2.2) holds unchanged: conversation ✓ (session cursor + engine store as optimization), workspace ✓ (snapshot ref), accounting ✓ (usage rows + reconcile pass), **in-flight outward effects** — covered by nobody in the field; Sinet covers them with the effect journal (§2.4), which is this report's largest net-new specification.

### 2.3 Orphan recovery: leases, fencing, harvesting — the idioms and their numbers

**The job-queue canon, with defaults** (all primary, current docs):

| System | Liveness signal | Dead-after | Orphan disposition |
|---|---|---|---|
| Sidekiq Pro `super_fetch` | per-process heartbeat 60 s | scan on startup + ≤1/min | re-enqueue; **poison-pill: 3 recoveries/72 h → Dead set** [S27] |
| Rails Solid Queue | registered-process heartbeat 60 s | 5 min (`process_alive_threshold`) | **claimed jobs of a pruned process → FailedExecution (`ProcessPrunedError`) — deliberately failed, not requeued** ("the worker might be crashing *because of* the job"); graceful TERM/QUIT *releases* jobs back to the queue — only unclean death fails them [S28] |
| River (Go/Postgres) | job lease | `max(1 h, JobTimeout+1 h)` rescuer | retry or discard [S29] |
| Oban Lifeline | node heartbeat | `rescue_after` 60 min | rescue only from provably dead nodes; out-of-attempts → discarded [S30] |
| Faktory | pure lease (FETCH reserves) | 1,800 s default | expiry = FAIL [S31] |
| AWS SQS | visibility timeout 30 s default | extendable via heartbeat (`ChangeMessageVisibility`, max 12 h) | redelivery; extensions not sticky across re-receipt [S32] |

Two design readings for Sinet: (1) **lease durations derive from declared run timeouts plus margin**, not fixed guesses (River's formula); (2) the field's mature systems **do not blindly requeue orphaned non-idempotent work** — Solid Queue's fail-and-inspect posture is the queue-world expression of Sinet's own ratified pause-and-flag (G1 D1.3), and Sinet's recovery ladder goes one better by *harvesting* first (below).

**Fencing.** Kleppmann's fencing-token argument remains the unsuperseded canon (a lock/lease alone cannot guarantee mutual exclusion; the resource must check a monotonically increasing token and reject stale writers) [S33, P]; 2026 agent literature has not re-derived it — the closest industrial forms are Temporal's task tokens and attempt counts (attempt-fencing is documented verbatim for the Reset operation; the general only-latest-attempt-accepted behavior is community/issue-documented rather than a single doc sentence — treated here as design precedent, not doc-quote) [S34, P/S]. **A suspended laptop is the GC-pause scenario made physical**: report 05's P-T02-3 (wake = double-resume factory) is exactly the case fencing exists for.

**Harvesting finished-during-outage work — the public state of the art has caught up to N1.** Two verified findings:

- **systemd is a designed harvest mechanism**: transient units run with `--remain-after-exit` stay loaded after the process exits precisely so a supervisor can later read `ExecMainStatus`/`ExecMainCode`/`Result` ("useful to collect runtime information about the service after it finished running"); failed units linger until `reset-failed` (CollectMode default `inactive`); transient units **survive `daemon-reload`** (live-tested by a verifier on systemd 259; mechanism: unit files in `/run/systemd/transient/` — indirectly documented via the unit search path) **but not reboot** (released "…or the system is rebooted", org.freedesktop.systemd1(5)) [S35, P + live test]. `ExitType=cgroup` (since v250) keeps a unit "running" while *any* process in the cgroup lives — authoritative process-tree tracking without PID files; `Type=oneshot` accepts `Restart=on-failure` (since v244) but rejects `always`/`on-success` [S36, P]. **Consequence (new platform problem P-T07-2):** the unit corpse is a restart-grade harvest source but a **reboot destroys it** — so the run wrapper must write its own exit record to disk (an event-log append) as the durable evidence, with unit corpses and engine session stores as enrichment.
- **OpenClaw's gateway restart-recovery doc is the closest public equivalent of the N1 ladder, verified verbatim by all three verifiers** [S37, P]: the claim is recorded "in one SQLite transaction before model … execution"; on boot the gateway "scans session stores for sessions that still claim to be running but have no live owner"; "if a final reply had already been produced but not delivered, its text is included so the agent can **deliver it instead of redoing the work**"; recovery retries ≤3 with exponential backoff, "every retry reuses **one durable dispatch identifier**, so an ambiguous connection failure cannot start the same recovery twice"; owner-disappeared runs are "marked lost after a grace period"; repeatedly failing sessions are "**tombstoned as wedged** so recovery cannot loop forever"; runs interrupted >2 h are finalized, not resumed; drain budget 5 min. Every one of these refinements is adopted into §4.4.
- **The reconcile-loop canon** (Kubernetes): level-triggered desired-vs-actual reconciliation, queue holds keys not events, the same function serves steady state and cold start; the kubelet's restart behavior is the reference — containers keep running, the restarted kubelet **re-lists actual state from the runtime** and reconciles, never trusting its own memory [S38, P/S]. Nomad's `RecoverTask` (persist a reattach handle at spawn; recover → reattach; error → mark LOST) is the named driver-level pattern [S39, P]. No public system articulates a *three-way* still-running/finished/dead classification — N1 remains ahead of the field in articulation, but every ingredient is now standard practice (§6).

**Liveness and the fourth state.** Temporal's heartbeat regime (throttle = min(0.8 × heartbeat timeout, 30 s default/60 s max); **heartbeat payloads carry progress cursors** that the next attempt reads) [S34, P] maps directly onto Sinet: the run heartbeat payload is the event-log cursor (last event seq), and "process alive but cursor stalled" is a distinct **wedged** classification — derived, never stored, per the ratified FSM discipline (report 05 §4.1).

### 2.4 Exactly-once outward effects: journal + idempotency + payload-pinned approvals

The canon is unambiguous: exactly-once *delivery* is impossible; exactly-once *effect* is achievable via at-least-once attempts + dedup at the effect boundary [S40, S]. How the durable-execution frameworks phrase their own guarantees is the cleanest articulation — DBOS, verbatim: "Steps are tried **at least once** but are never re-executed after they complete"; "Transactions commit **exactly once**" [S41, P; 3-vote SUPPORT]. Translated to Sinet: **journal writes are exactly-once (transactional with state); provider calls are at-least-once with idempotency; the journal is what makes the difference invisible.**

- **The crash window is textbook**: the transactional-outbox literature names exactly the approved-but-not-yet-executed / executed-but-not-yet-recorded windows Sinet's D7 gate creates [S42, P]. The single-node answer is a **two-phase effect journal**: intent row written in the same transaction that flips the proposal to `executing` (before the provider call), outcome row after; restart scans in-doubt (`executing`) rows.
- **Idempotency keys are provider-specific, and no standard exists.** Stripe (the reference): keys replay the first result **including errors** for 24 h; "we save results only after the execution of an endpoint begins — if incoming parameters fail validation, or the request conflicts with another request that's executing concurrently, we don't save the idempotent result… you can retry these requests"; same key + different params errors [S43, P; 3-vote SUPPORT with the precision that rate limits are *not* named in the carve-out]. Divergence across providers is wide (Adyen ≥7 d; PayPal ≤45 d; Square requires a body param; Svix replays *successes only* for 12 h) [S44, P], and the IETF `Idempotency-Key` header draft **expired in 2026 with no successor** (-07, 2025-10-15, "no longer active") [S45, P] — so Sinet needs a **per-provider idempotency registry** (window, mechanism, replay semantics), not a uniform header.
- **Effect classes** (the field's de-facto taxonomy, assembled from primary sources): **(A) natively idempotent** — `git push` is idempotent at the ref level with atomic ref updates and `--force-with-lease` as CAS [S46, P]: blind retry is safe. **(B) key-dedupable** — Stripe-class APIs; notably **email is solvable at the provider level** (Resend exposes idempotency keys on send; AgentMail similar) [S47, P]. **(C) query-before-retry** — GitHub comments/issues have *no* idempotency mechanism and duplicate on retry; the pattern is a deterministic marker (effect id embedded in the body) + search-before-retry [S48, P]. **(D) no idempotency** — plain SMTP has carried a documented crash-duplicate window since RFC 1047 (1988): at-most-once execution + surface "effect status unknown" to a human [S49, P]. OWASP's AI-agent guidance states the two-track rule: "make high-impact actions idempotent where possible and require explicit duplicate confirmation when idempotency is not possible" [S50, P].
- **Approval must be pinned to the exact payload.** Terraform's saved-plan flow is the shipped canon — `plan -out` + `apply <plan>` means "approve exactly this artifact," and apply refuses a stale plan when state moved [S51, P]; 2026 agent-security guidance (OWASP; academic work on approval-view fidelity attacks) converges on binding the approval record to a **normalized-payload hash + expiry**, with the executor re-verifying the hash inside the transaction that flips `approved → executing` [S50, S52, P/S]. This composes with the ratified 4.3 freshness machinery (drift invalidates, GitHub-stale-approval style — report 05 §2.4).
- **Agent frameworks demonstrably do not solve this**: crewAI (open 2026-05) re-executes already-executed tools on task retry; LangGraph Cloud has the same class, production-confirmed by multiple parties [S53, P]. The closest shipped proposal-queue precedent is **gh-aw safe-outputs** (now at github/gh-aw): agents run read-only and request actions via structured output; separate permission-controlled jobs execute them after a threat-detection gate; per-type max counts; a `staged` mode previews instead of executing [S54, P] — D7's shape, at CI granularity. **MCP tool annotations (`destructiveHint` default *true*, `idempotentHint` default *false*) are explicitly untrusted hints — the current spec says clients "MUST consider tool annotations to be untrusted unless they come from trusted servers"** — usable as classification *input*, never as a gate bypass [S55, P]. (MCP's next revision is due 2026-07-28 — Tasks/Extensions land there; the S2.8 watch from report 05 OQ9 stands.)

### 2.5 SQLite as the state substrate: the verified operating rules

All of the following survived 3-vote verification against sqlite.org primary text:

1. **Durability dial.** In WAL mode, `synchronous=NORMAL` is always *consistent* but "a transaction committed… might roll back following a power loss or system crash"; **"transactions are durable across application crashes regardless of the synchronous setting"**; `FULL` "is atomic, consistent, isolated, and durable (ACID)" [S56, P]. Design consequence in §4.1: at Sinet's write rate (a handful of writes per paid call, i.e., seconds apart — not thousands/sec) the fsync cost of FULL is noise, and it deletes the power-loss window entirely.
2. **Write discipline.** A deferred transaction upgrading read→write against a moved database fails `SQLITE_BUSY` (`SQLITE_BUSY_SNAPSHOT` in WAL) **immediately — the busy handler is deliberately bypassed** (deadlock avoidance, c3ref/busy_handler; live-tested: 0.000 s failure with a 5 s busy_timeout set). The fix is **`BEGIN IMMEDIATE` on every writing transaction**, which honors busy_timeout and carries a documented guarantee: once it succeeds, no operation through COMMIT returns SQLITE_BUSY [S57, P + live test].
3. **Checkpoint starvation.** Long/overlapping read transactions block WAL checkpoints and the WAL "will grow without bound" — the platform's own event-tail followers are the hazard; use short read transactions and periodic `wal_checkpoint(TRUNCATE)` [S56, P].
4. **Corruption etiquette** (howtocorrupt.html, the items that matter on one host): never file-copy a live DB mid-transaction; never delete/rename `-wal`/`-shm`; beware POSIX `close()` cancelling all advisory locks process-wide (no casual second open of the DB file in-process); consumer drives lie about sync [S58, P]. Same-host-only: WAL does not work over network filesystems.
5. **Queue-on-SQLite is settled practice.** The consensus claim is a single atomic `UPDATE … SET status/lease WHERE id = (SELECT … WHERE status='queued' … LIMIT 1) RETURNING *` (SQLite ≥3.35) — the single-writer lock makes claim races structurally impossible; lease-expiry columns give SQS-style redelivery; polling (~0.1–1 s) is what everyone ships [S59, S/P]. Rails 8's Solid Queue supports SQLite for exactly this scale; goqite/litequeue/plainjob demonstrate the pattern with plainjob benchmarking ~15k jobs/s — three orders of magnitude above Sinet's need [S28, S60, P]. **N2's design is field-validated** (§6).
6. **Production precedent.** Tailscale's coordination server is a single Go process on SQLite + Litestream; Bluesky's PDS is per-tenant SQLite; PocketBase runs WAL SQLite as its sole store [S61, P/S].
7. **Backup shape.** `VACUUM INTO` is "transactional… a consistent snapshot of the original database," works on a live DB, leaves the original untouched, and purges deleted content (a privacy bonus for off-host snapshots); `.recover` is a salvage tool, not a backup mechanism [S62, P]. **Litestream** was rewritten on the LTX format in v0.5.0 (2025-09-30) and is actively maintained (v0.5.14, 2026-07-06); early 0.5 instability was fixed by ~0.5.2; **v0.5.7 (2026-02) added transparent v0.3-backup restore** (only Age-encrypted v0.3 backups remain unrestorable — Age support was removed); one open bug (#1083, silent replication failure on WAL space reuse in v0.5.6+) argues for pinning + the restore drill below [S63, P; corrected by verification]. LiteFS is effectively frozen (beta, last release 2025-04; Cloud sunset) — wrong shape anyway [S64, P].
8. **Migrations.** `PRAGMA user_version` + numbered SQL files, one transaction each, is the accepted solo pattern (Tailscale published `squibble` for exactly this); event payloads carry a per-type `schema_version` and are upcast on read — history is never rewritten [S65, P/S].

### 2.6 Crash-consistency verification culture — and the laptop's specific hazards

- **The layer below the app is SQLite's problem, and SQLite tests it harder than anyone**: its own harness crashes a child process mid-write under a VFS that "randomly reorders and corrupts the unsynchronized write operations," TH3 injects power-loss-characteristic damage, and every crash test ends with `PRAGMA integrity_check` [S66, P]. Academic tooling (ALICE, CrashMonkey/ACE, dm-log-writes) is 2017–19 vintage and disproportionate for one maintainer [S67, S]. **The realistic solo harness** — and the one this report mandates as a platform conformance-suite entry (§4.7) — is a kill-loop: run the platform under synthetic load with a mock adapter, `kill -9` control plane and run units at random points (including mid-claim and mid-journal-append), restart, and assert *application* invariants: integrity_check ok, no per-run event-seq gaps, every non-terminal run classified by one reconcile pass, no mock effect executed twice, ask records ≥ engine pending asks. With `synchronous=FULL` the kill -9 harness covers the whole loss model; power-cut testing adds nothing SQLite hasn't already tested below the app.
- **Suspend is filesystem-safe by kernel design; the hooks are not what report 05 assumed.** The suspend transition freezes user space and syncs filesystems before devices power down (default-on, tunable via `/sys/power/sync_on_suspend` / `CONFIG_SUSPEND_SKIP_SYNC`) [S68, P; citation corrected by verification to the sysfs-power ABI doc]. But the **sanctioned pre-sleep hook is a logind *delay* inhibitor lock + the `PrepareForSleep` D-Bus signal — and `InhibitDelayMaxSec` defaults to 5 seconds** [S69, P]. The `/usr/lib/systemd/system-sleep/` scripts report 05 §4.9 named are officially "intended for local use only and should be considered hacks," and **`user.slice` is frozen while they run** — fatal if Sinet runs as a user service [S70, P]. **Correction to report 05 recorded in §7.** Consequence (P-T07-1): pre-sleep work must be an O(1) durable flush (state is already durable per D7 — there is nothing heavy to do); the real work happens **wake-side** on `PrepareForSleep(false)`, which enqueues the same reconcile pass as startup.
- **Clocks are the top laptop trap.** `CLOCK_MONOTONIC` "does not count time that the system is suspended"; `CLOCK_BOOTTIME` does [S71, P]. A MONOTONIC-based lease silently *extends* across a nap (dead worker looks alive); a BOOTTIME-based lease mass-expires on wake (alive workers look dead). NTP steps jump `CLOCK_REALTIME` [S72, S/P]. The idiom adopted in §4.4: **ordering comes only from the event-log sequence; deadlines are wall-clock columns in the DB; expiry evaluation is suspend-aware** (wake detected via `PrepareForSleep(false)` and the BOOTTIME−MONOTONIC delta) with a ⚙ wake grace before any lease is declared dead. All TCP/SSE connections are presumed dead after wake; an in-flight streamed response from before the nap may have completed — and been paid for — server-side: reconcile against the engine session store before any re-spend (the harvest step again).

### 2.7 Event sourcing vs mutable-state-plus-audit at bus factor 1

The practitioner temperature in 2025–26 is consistent: full event sourcing (events as the *only* truth + projections + replay machinery) is overkill for small systems; the recommended middle path is **mutable current-state rows + an append-only event log** — traceability without projection/replay infrastructure [S73, S]. The standard SQLite idiom: one append-only table, `INTEGER PRIMARY KEY` as the ordering authority (never timestamps), JSON payload, wall-clock column for humans [S74, S/P]. OpenHands proves full ES *works* when one SDK owns everything [S18]; Sinet doesn't own the engines, so events are observations, not commands — replay could never reconstruct engine-side state anyway. **The G1-ratified hybrid (FSM rows + append-only event log) is confirmed as the field's recommended shape, not a compromise.** One addition from the LangGraph record: the event log needs its own growth policy (payload caps, refs-not-blobs, retention) from day one — filed as P-T07-5.

### 2.8 Maintenance-mode drain

Report 05 §2.7/§4.11's canon re-verified unchanged (Sidekiq TSTP quiet → TERM → push-back; Temporal worker drain; K8s preStop/grace; Trigger.dev pause = queue-but-don't-run vs Inngest's pause = drop). New precision from this run: **no system ships "drain to checkpoint boundary + park" as a named feature** — OpenClaw's 5-minute drain budget then lost-marking is the nearest public behavior [S37]; Solid Queue's graceful-TERM-releases vs unclean-death-fails distinction [S28] is the queue-level version of the same idea; and Restate's v1.7 pause pointedly *aborts* rather than drains [S8] — evidence that "pause" and "drain to a safe point" are different operations that must not be conflated. Sinet's composition stands: admission gate keeps queueing → in-flight runs finish the current paid call and checkpoint → past the ⚙15-min grace (ratified), boundary-interrupt + park. The per-run drain hook maps onto the transient unit's `ExecStop=` (runs only if the unit started cleanly; TERM→KILL per `TimeoutStopSec`) [S36].

### 2.9 Parallel same-project coordination state (S1.11)

- **Worktree-per-task is the 2026 consensus isolation unit** (Vibe Kanban, Cursor background agents — cap 25/machine, Conductor-class tools); the dissent (container-per-task) argues worktrees share the host env [S75, P/S]. Sinet already has N9 (worktree+branch per pipeline) and S5 sandboxes — both, composed.
- **Plan-time artifact claims exist nowhere as an enforced product feature.** The closest public practice is advisory: disjoint-file-set assignment with an "overlap zone" registry of high-risk files (practitioner methodology); task-level claim files; declared read/write sets in harness papers (paper-stage) [S76, S]. Everything shipped resolves conflicts **at merge time**: "parallel generation, sequential merging," with the first merge-queue gate being applies-cleanly-on-latest-target, and rebase-on-main as the de-facto freshness re-check [S77, S]. **Consequence: S1.11's plan-time claim registry is, like 4.3's freshness pass, a Sinet-built novelty composed of standard parts** — a claims table, conservative write-set declaration at plan approval, overlap detection by path/glob intersection, and the ratified sibling-accept freshness trigger (G1 Def.5). Mechanics in §4.9; *policy* (sequence vs branch) stays with T02's 4.3 findings as briefed.

---

## 3. Candidate approaches

**A. Sinet-owned SQLite state layer — the G1-ratified direction, fully specified (§4).** One WAL database owned by the control plane (sole writer); mutable FSM rows + append-only event log; checkpoint rows per paid call; durable ask records; two-phase effect journal; CAS work claiming with leases + fencing generations; reconcile ladder on start/wake. Everything in §2 is achievable with documented SQLite semantics; the shape now has explicit expert consensus at this scale (§2.1) and production precedent (§2.5-6). Cost: Sinet owns ~12 tables and the reconcile/journal logic — bounded, testable components, all of which any framework alternative would leave Sinet building anyway (see B).

**B. DBOS-on-SQLite as the state spine — the strongest challenger, argued fresh** (its old rejection ground is stale, §2.1). What it buys: step memoization ("never re-executed after they complete"), queues, retries — solid, MIT, no server. Why it still loses for Sinet: (1) **it wants to own the control flow** — durability attaches to decorated workflow *functions* that DBOS re-invokes on recovery; Sinet's unit of work is a long-lived interactive engine subprocess with streaming events, human gates that park for hours, and fencing requirements — wrapping that in a re-invocable function reintroduces exactly the workflow-shaped indirection the G1 session model avoided; (2) **it solves the already-small part**: of this report's specification, DBOS covers step memoization and queueing — the ask records, effect journal with payload-pinned approvals, engine-session mining, artifact claims, suspend-aware leases, and owner attribution are all outside its model and would be DIY tables beside its system tables anyway; (3) **maturity asymmetry**: the SQLite backend is Python-only-default (TS none; Go opt-in June 2026) and was still receiving threading/isolation fixes in 2026 [S4]; (4) observability (Conductor) is paid for production. Verdict: **rejected as spine; promoted to named design donor and the pre-registered fallback** if Sinet's steps ever become discrete request/response calls (e.g., a future API-lane-only mode). Its docs' guarantee vocabulary (§2.4) is adopted regardless.

**C. Restate as the parking/journal service.** Improved since report 05 (v1.6 pause/resume; retention knobs; honest BUSL), and its durable-promise model matches gate parking well. Still a second always-on stateful service (RocksDB) whose journal would duplicate what the engines + platform log already hold, still abort-on-pause rather than drain — and Sinet's parking already costs $0 while parked (S2: poll-resume is free; opencode sessions idle on disk). Rejected; re-evaluate only at multi-host scale (12.7).

**D. A small queue library (goqite/litequeue/plainjob-class).** The claim/lease pattern is ~50 lines of SQL that must live inside Sinet's own transaction boundaries (claim + FSM transition + event append atomically); a library dependency would fragment the transactional core to save trivial code, and none carries per-model slot gates, owner attribution, or priority lanes. Pattern donors only.

**E. Full event sourcing.** Rejected per §2.7: replay cannot reconstruct engine-held state; projections add machinery bus-factor-1 doesn't need; the hybrid delivers the audit value (11.1) without it.

**F. Temporal / Inngest.** Rejection confirmed with sharpened evidence (§2.1): Temporal — dev-only SQLite, history caps that agent-length runs would hit, loop-inside-workers model; Inngest — in-memory queue state with snapshot gaps on exactly Sinet's crash classes, cloud-coupled AI steps.

---

## 4. Recommendation for Sinet

**Build approach A: one platform-owned SQLite-WAL database as the authoritative state layer, with the following specified mechanics.** Numbers marked ⚙ are operator-editable settings with these defaults (G1 rider), for ratification at G2.

### 4.1 Store configuration

- One database file (`platform.db`), WAL mode, **`synchronous=FULL`** ⚙ — rationale: Sinet's write rate is per-paid-call/per-transition (seconds apart, tens per minute at peak fan-out), so FULL's per-commit fsync is unmeasurable here and buys zero-loss on power cut (§2.5.1); NORMAL is the documented fallback if measurement ever disagrees, accepting a last-commits rollback window that the schema already tolerates (every cross-table invariant is written in one transaction).
- **Control plane is the sole writer process** (P-T01-1 discipline at the storage layer); dashboards/CLI open read-only connections. Every writing transaction is **`BEGIN IMMEDIATE`**; `busy_timeout` ≥ 5 s ⚙ on all connections; `foreign_keys=ON`.
- Read hygiene: no long-lived read transactions (event tailing reads in short batches by seq cursor); periodic `wal_checkpoint(TRUNCATE)` ⚙ hourly + WAL-size watch feeding S2.7 (checkpoint starvation is a silent disk-eater, §2.5.3).
- `PRAGMA integrity_check` at every platform start (cheap at this size) — it is also the crash-harness postcondition.
- Migrations: `user_version` + numbered SQL files, one transaction each (squibble-class tooling optional); event payloads carry `schema_version`, upcast-on-read; **history is never rewritten** (append-only + Manus cache-alignment rule from report 05).
- Event-payload discipline (the LangGraph lesson, P-T07-5): payloads capped ⚙ 64 KB; bulky tool output/artifacts live as files (or engine stores) referenced by path+hash; validate events against their schema version before persisting — never forward unvalidated payloads (the #7714 failure class).
- Corruption etiquette enforced by convention + lint: nothing ever file-copies the live DB (snapshots go through `VACUUM INTO`, §4.7); nothing but the control plane opens it read-write; the `-wal`/`-shm` files are radioactive.

### 4.2 Schema core (the durable set — every row owner-attributed, 15.6)

The minimal set that covers every spec item in scope, sanity-checked against Archon's 7-table reference (§6, A5). Names indicative; exact DDL is the spec workshop's job (with the ledger schema, per G1 Def.12 and R07 OQ4):

| Table | Holds | Spec anchor |
|---|---|---|
| `users` | identity, credential-store refs (D2) | 15.6, 10.x |
| `tasks` | user-facing task + kanban status (orthogonal to run machinery — N4) | 9.1, S1.3 |
| `runs` | FSM state (R05 §4.1 states + `died-at-gate` from S2), owner, task, substrate/lane, **lease** (holder, wall-clock deadline, heartbeat cursor), **generation** (fencing counter, bumped per takeover/resume), systemd unit name, workspace/worktree ref, ceilings | 3.7/3.8, D6 |
| `run_events` | append-only log: `event_seq INTEGER PRIMARY KEY`, run_id, generation, type, `schema_version`, JSON payload, wall-clock ts. The D7 record. | 4.8, 11.1 |
| `checkpoints` | one row per paid call, written in the same txn as its event: session cursor {substrate, engine session id from `system/init`, message index, cwd key, transcript path}, usage block, **ledger revision ref/hash**, artifact snapshot ref, config fingerprint {settings hash, permission mode, model} | D7, 4.3 |
| `asks` | every gate/question the moment observed (S2 exit-JSON `deferred_tool_use`; S3 `permission.asked`/`question.asked`): full **invocation-reconstruction snapshot** (S2's measured park-record set), status, observed/answered ts, answer payload, engine-expiry watch | 4.2/4.3, P-T02-1 |
| `effects` | the journal: proposal payload + normalized **payload_hash**, class A–D, approval {who, when, expires ⚙}, state `proposed→approved→executing→succeeded/failed/unknown`, **idempotency_key** (= effect UUID), provider + provider-window ref, attempts, result/error | 4.2, 3.8 "never repeated" |
| `queue` (or claim columns on `runs`) | CAS claiming: status, `claimed_by`, lease columns, priority lane | 3.3, N2 |
| `lanes`/`slots` | per-(user, model/lane) concurrency caps — N2's slot gates as data | 3.11, D4 |
| `artifact_claims` | S1.11 registry: task, project, path/glob set, mode R/W, declared-at-plan, status | S1.11 |
| `engine_sessions` | session registry incl. copied-aside transcript path (§4.3) — the reboot-case mining index | 3.8, P-T07-2 |
| `receipts`/usage | derived from checkpoint usage rows; materialized per run-end (design owned by T08) | 3.6, D5 |

Ledger note: Task Context Ledger revisions are persisted as `run_events` (type `ledger_update`, full content — it is small by design), making the D7 checkpoint payload self-contained in the DB even if the workspace is destroyed; the working copy in the task workspace is a projection.

### 4.3 Checkpoint content and the transcript copy-aside

As specified in §2.2 (fields a–e), identical across substrates (S1's schema parity). Two additions: (1) **copy-aside the engine transcript segment at each checkpoint** on the Claude lane (cheap file copy, indexed in `engine_sessions`) — insurance against the 30-day sweep, cwd-key breakage, and crash-truncated JSONL (war-story class §2.2), and the raw material for reboot-case harvesting; (2) checkpoint rows carry the version fields (model id, config fingerprint, tool schema hash) so recovery can detect that a resume would run under different assumptions — feeding the 4.3 freshness pass rather than replaying blind [S15].

### 4.4 The recovery ladder (startup/wake reconcile — N1 generalized, OpenClaw-hardened)

**Triggers:** platform start; `PrepareForSleep(false)` (wake); periodic sweep ⚙ 5 min (free on the Claude lane — S2's $0 poll-resume; cheap `GET /session/status` on opencode).

**Pre-sleep** (P-T07-1): hold a logind sleep **delay** inhibitor; on `PrepareForSleep(true)` do only an O(1) flush — commit any open work, `wal_checkpoint`, mark a `suspend` event — within the ~5 s `InhibitDelayMaxSec` budget. Nothing heavier: D7 means state is already durable at all times.

**The pass**, per non-terminal run (level-triggered; same code for start, wake, and sweep — the kubelet rule: re-list actual state, never trust supervisor memory):

1. **Observe actuals**: DB rows ⇄ systemd units (live + `--remain-after-exit` corpses: `ExecMainStatus`/`Result`) ⇄ OS cgroups ⇄ engine session state (opencode `/session/status`, `permission.list`, `question.list`; Claude lane: exit records + transcript copy-aside + JSONL) ⇄ worktrees.
2. **Classify** (the ladder): **ALIVE** — unit active *and* event cursor advancing → reattach streams, re-arm watchdogs. **WEDGED** (derived, 4th class) — unit active, cursor stalled past threshold → pause-and-flag (D1.3), never auto-kill. **FINISHED-DURING-OUTAGE** — unit corpse exited 0, or engine store shows a terminal result newer than the last checkpoint → **harvest**: parse the result envelope / mine the session store, append the missing events/usage **deduplicated by (session_id, message_id)** (R05 §4.9), deliver the produced result instead of redoing the work (OpenClaw's rule), mark completed. **DEAD** — unit gone/failed with nothing newer to harvest → mark crashed; supersede by **fork-from-last-checkpoint as a new attempt with an incremented generation**.
3. **Bound recovery** (OpenClaw + Sidekiq): recovery attempts reuse **one durable dispatch id** per interruption (an ambiguous failure cannot double-start); ≤3 ⚙ attempts with backoff; repeat offenders **tombstoned as wedged** → decision card; runs interrupted longer than ⚙ 24 h are finalized-with-card rather than blindly resumed (the 4.3 freshness pass fires first regardless — ratified fingerprint triggers).
4. **Leases + fencing**: lease deadlines are wall-clock columns; expiry evaluation is suspend-aware — on wake, apply ⚙ 120 s grace before declaring leases dead (the BOOTTIME/MONOTONIC trap, §2.6); every event append carries the run's generation, and the writer rejects stale-generation appends — the fencing that makes double-resume (P-T02-3) harmless.
5. **Asks reconcile**: platform ask records are authoritative (S3, source-verified volatility); re-hydrate engine-side asks where the engine still holds them, re-prompt the same session id where it lost them (a paid turn, opencode lane), re-issue `--resume` full-reconstruction on the Claude lane (S2's measured contract: the engine restores *nothing* — the ask row's invocation snapshot is the resume input).
6. **Effects reconcile**: any `executing` row is in-doubt → per-class resolution (§4.5): replay with idempotency key (B), query-before-retry (C), or flip to `unknown` + card (D). This closes 3.8's "outward actions never repeated" across the crash window.
7. **GC** (four layers, R05 §4.9): unit corpses `reset-failed` after harvest; orphan worktrees flagged (never auto-delete uncommitted work); engine session files past platform retention; tombstone review cards.

**Reboot asymmetry** (P-T07-2): after a reboot the unit corpses are gone — classification then rests on the run wrapper's own exit records (written to the event log as its last act) and engine-session mining. The wrapper-written exit record is therefore mandatory, not an optimization.

### 4.5 The effect journal (exactly-once outward effects, 4.2 + 3.8)

- **Guarantee vocabulary** (DBOS wording, adopted): journal/state writes are exactly-once (same transaction as the FSM transition); provider calls are at-least-once (classes A–C) or at-most-once (class D); dedup lives at the effect boundary.
- **Lifecycle**: `proposed` (created by the gate, payload normalized + hashed) → `approved` (approver, timestamp, **payload_hash pinned, expiry ⚙ 7 d** — Terraform saved-plan semantics; drift or expiry → back to re-approval, integrating the ratified freshness triggers) → `executing` (own transaction, **written before the provider call**, hash re-verified inside this transaction, idempotency key = effect UUID) → `succeeded` / `failed` / `unknown`.
- **Class registry** (per-provider, data not code — like the price table): **A** natively idempotent (git push/`--force-with-lease`) → blind retry; **B** key-dedupable → retry with the stored key, respecting the provider window registry (Stripe 24 h incl. error replay with the validation/concurrency carve-out; Adyen ≥7 d; PayPal ≤45 d; Svix success-only 12 h); **C** query-before-retry (GitHub comments/issues) → deterministic effect-id marker embedded in the payload + search-before-retry; **D** no idempotency (plain SMTP) → at-most-once, crash-window → `unknown` + a decision card that shows the human what to check (5.6; the OWASP two-track rule).
- **Policy consequence** (P-T07-3, operator decision OQ3): channel choice is a durability decision — prefer idempotent-capable providers for email-class effects (Resend-class keys) so class D stays rare.

### 4.6 Queue and claiming (N2 ported)

Atomic claim = the consensus statement (§2.5.5): `UPDATE … SET status='claimed', claimed_by=?, lease_deadline=?, generation=generation+1 WHERE id=(SELECT id FROM queue WHERE status='queued' AND lane_has_slot(...) ORDER BY priority LIMIT 1) RETURNING *` inside `BEGIN IMMEDIATE`. Slot gates: counted capacity per (user, lane/model) checked in the same transaction (N2's `_slot_ok` as SQL). Priority ladder preserved from N2: **resume > queued > retry-blocked > new**. Polling ⚙ 500 ms; no notification bus (field norm, §2.5.5). Graceful drain **releases** claims back to the queue; unclean death goes through the recovery ladder — **harvest-first, never blind requeue** (Solid Queue's rationale + D1.3, §2.3).

### 4.7 Crash-test practice and the durable-set snapshot (tested, not assumed)

- **The kill -9 harness is a platform conformance-suite entry** (same standing as the adapter suites, P-T01-2): synthetic load on a mock adapter; random SIGKILL of control plane and run units, biased to the nasty windows (mid-claim, mid-journal-append, between paid call and checkpoint); restart; assert: `integrity_check` ok; no event-seq gaps; one reconcile pass classifies every non-terminal run; zero double-executed mock effects; ask records ⊇ engine pending. Plus a **suspend-cycle test** (fake `PrepareForSleep` signals + clock-delta injection) asserting the lease-grace behavior.
- **What must BE durable for 11.3** (mechanics to T12): the §4.2 table set is the snapshot payload; the shape is `VACUUM INTO` a temp file (consistent against the live DB) → `.dump` (text-first, diffable, deleted-content-purged) → client-side encrypt → snapshot repo. Raw `run_events` payload bodies past the 11.1 compaction horizon are excluded (traces stay local). **A verified-restore drill** — rebuild from the dump, `integrity_check`, invariant checks — is itself a ⚙ scheduled platform task (11.3's "restore procedure is tested, not assumed" made concrete). Litestream v0.5.x (pinned) is the recommended *continuous* local/off-host replication addition, with bug #1083 on the watchlist — but the dump-based snapshot is the load-bearing 11.3 mechanism.

### 4.8 Maintenance mode

Unchanged from report 05 §4.11 + G1 (admission gate keeps queueing; drain to next checkpoint boundary; ⚙ 15-min grace then boundary-interrupt + checkpoint + park), now with storage-layer mechanics: `draining` is an FSM state; the per-run drain hook is the transient unit's `ExecStop=` with `TimeoutStopSec` = the grace; platform shutdown ends with the O(1) flush of §4.4 plus a final `wal_checkpoint(TRUNCATE)`. "Pause" and "drain" are distinct verbs in the adapter contract (the Restate v1.7 lesson: pause-that-aborts re-executes un-journaled work).

### 4.9 S1.11 artifact-claim registry (mechanics; policy stays with T02/4.3 findings)

At plan approval, the plan declares its **write-set** (paths/globs; conservative default ⚙ = whole-project write claim when the plan cannot bound it) and optional read-set (for freshness precision). The registry check is glob-intersection against active claims in the same project: disjoint → run; overlap → **sequence** (queue behind the holder) or **explicit branch** as a decision card — surfaced at plan time, which no shipped product does (§2.9; novel like 4.3). Sibling-accept fires the ratified freshness trigger (G1 Def.5) as an event to all active runs in the project. At accept time, the first gate is applies-cleanly-on-current-HEAD (the merge-queue idiom); a collision surfaces as a reviewable merge card — S1.11's "never silently overwritten," verbatim. Claims are rows, not locks: the enforcement point is the scheduler (control plane), consistent with the sole-controller posture.

**What would change the decision:**

1. **DBOS shipping an external-process/step-attachment mode + TS/SQLite parity** — would make it a genuine spine candidate; re-evaluate at G2's successor or on its 3.x. (Watch, §7.)
2. **Engines adopting the MCP Tasks extension** (final spec 2026-07-28) as durable task handles — could converge parking/cancel onto protocol handles; standing S2.8 watch (carried from R05 OQ9).
3. **Multi-host growth (12.7)** — SQLite's same-host WAL constraint is the hard wall; the migration is Postgres + River/DBOS-class machinery, and the schema above ports (it is deliberately framework-agnostic SQL).
4. **SQLite corruption or Litestream instability in practice** — corruption: escalate via the restore drill (the state layer is rebuildable from snapshots + engine mining); Litestream: drop to dump-snapshots-only until stable.
5. **A shipped plan-time claim/lock product** in the parallel-agent space — adopt its semantics if they beat §4.9's registry.

---

## 5. What NOT to use and why

- **Temporal as the state layer** — SQLite persistence officially non-production; 51,200-event/50 MB history termination is undersized for agent-run journals; the AI integrations assume the loop lives inside their workers. Patterns (heartbeat cursors, drain states) harvested, dependency rejected.
- **Inngest self-host** — queue/state in in-memory Redis with periodic SQLite snapshots: a designed crash-loss window on exactly Sinet's failure classes; cloud-coupled `step.ai`.
- **DBOS as the spine** — *not* for the stale Postgres reason (corrected herein): rejected because it owns control flow via re-invocable functions (wrong shape for parked interactive subprocesses), covers only the small solved part of this design, and its SQLite backend is Python-only-default and freshly hardened. Design donor + pre-registered fallback.
- **Restate** — improved (v1.6 pause/resume) but still a second stateful service duplicating what engines + the platform log hold; pause aborts rather than drains.
- **LangGraph checkpointers** — no retention machinery, measured 85% storage bloat class, unvalidated persistence, schema-evolution breakage. Cautionary reference only.
- **Full event sourcing** — replay cannot reconstruct engine-held state; projection machinery unjustified at bus factor 1. Hybrid ratified.
- **Queue/job libraries as dependencies** (goqite/litequeue/plainjob/Solid-Queue-class) — the claim SQL must live inside Sinet's own transactions; libraries fragment the atomic core to save ~50 lines.
- **Engine stores as the authoritative record** — settled (P-T01-1) and re-confirmed by the vendor's own docs ("sessions persist the conversation, not the filesystem"; fresh-session-on-cwd-mismatch) and S3's in-memory asks. Resume optimization + harvest source only.
- **Network filesystems for the DB** (WAL same-host requirement) and **LiteFS** (frozen beta, FUSE multi-node shape).
- **File-copy backups of the live DB / `.recover` as a backup path** — the documented corruption/salvage traps; `VACUUM INTO` + dump only.
- **`/usr/lib/systemd/system-sleep/` scripts as the reconcile hook** — officially "hacks," user.slice frozen during execution; inhibitor delay locks + `PrepareForSleep` instead (correction to R05 §4.9).
- **CLOCK_MONOTONIC (or any single clock) for lease math** — the suspend trap cuts both ways; DB wall-clock deadlines + suspend-aware evaluation + event-seq ordering.
- **PID files** — cgroup/unit identity (`ExitType=cgroup`) is the authoritative process-tree tracking.
- **Blind auto-requeue of orphaned non-idempotent work** — the field's mature systems deliberately fail-and-inspect; Sinet harvests first, then supersedes with a card.
- **MCP tool annotations as a gating input** — `destructiveHint`/`idempotentHint` are spec-mandated-untrusted hints; classification input at most, never a D7 bypass.
- **Uniform idempotency assumptions across providers** — the IETF header draft is dead; windows and replay semantics diverge (12 h success-only to 45 d); the per-provider registry is mandatory.

---

## 6. Harvest-map verdicts

| Item | Verdict | Detail |
|---|---|---|
| **N1** — orphan-harvest ladder + control/execution split | **CONFIRM + WIDEN — and no longer unique** | The ladder is production-proven (Nexus) and now has a **verified public sibling**: OpenClaw's restart-recovery doc implements claim-in-one-transaction-before-the-call, boot scan for ownerless "running" sessions, **deliver-the-produced-reply-instead-of-redoing**, bounded retries on one durable dispatch id, lost-after-grace, and wedged-tombstones [S37] — independently converging on N1's shape. Port N1 as §4.4 with the OpenClaw hardenings plus: a **fourth derived class (wedged)**, suspend-aware lease grace, generation fencing, and the **reboot asymmetry fix** (wrapper-written exit records; unit corpses and engine stores as enrichment). The harvest step doubles as the accounting reconciler (R05 §4.9), unchanged. |
| **N2** — atomic CAS claiming + per-model slot gates | **CONFIRM — field-validated** | The 2025–26 queue-on-SQLite consensus is exactly N2's design (single-statement CAS claim + lease columns; Solid Queue ships it on SQLite in Rails 8; `UPDATE…RETURNING` makes it one statement). Port with the priority ladder intact and two additions Nexus never needed as a single always-on process: **wall-clock leases with suspend-aware expiry** and a **fencing generation** stamped into every event append (the wake/double-resume hazard is Sinet-specific). Slot gates become rows (`lanes`), checked in the claim transaction. |
| **N4** — dispatch state machine, user-facing status orthogonal | **CONFIRM — already ratified, extended** | G1 ratified R05's FSM, which is N4's descendant. This report adds the storage-layer precisions: `died-at-gate` (S2's measured budget-preempts-park state), `wedged` as derived-only, effect states (`executing`/`unknown`) as first-class, and the tasks-vs-runs split in the schema (kanban status on `tasks`, machinery state on `runs`) — the orthogonality N4 prescribed, now as DDL. |
| **A5** — Archon's 7-table schema as minimal-set sanity reference | **CONFIRM as reference — sanity check passes** | Mapping: Archon codebases→`tasks`/project registry; sessions/conversations→`runs`+`engine_sessions`; workflow runs→`runs`; messages/events→`run_events`; isolation environments→worktree/sandbox refs on `runs`. Sinet's core lands at ~12 tables; the **delta over Archon is exactly the machinery Archon doesn't have and the spec demands**: `checkpoints` (D7), `asks` (4.2/4.3 durability, P-T02-1), `effects` (exactly-once outward), `lanes`+`queue` (D4/3.3), `artifact_claims` (S1.11), receipts (D5). Nothing in §4.2 lacks a spec anchor — the reference did its job as an austerity check. |

---

## 7. Open questions

**Operator decisions (G2):**

1. **`synchronous=FULL` as the default** ⚙ — negligible cost at Sinet's write rate, deletes the power-loss window. Ratify or choose NORMAL+documented-window. *Owner: operator (13.4 setting).*
2. **Lease/liveness numbers** ⚙ — heartbeat 60 s, dead-after 5 min (field-aligned), wake grace 120 s, recovery attempts ≤3, stale-park finalize-with-card at 24 h, ask-approval expiry 7 d. *Owner: operator ratifies the set.*
3. **Effect-channel policy (P-T07-3)** — require idempotent-capable providers for email-class outward effects (class D stays rare, unknown-outcome cards exceptional) vs allow plain SMTP with routine unknown-cards. *Owner: operator.*
4. **Artifact-claim conservatism** ⚙ — whole-project write claim as the default when a plan can't bound its write-set (safe, serializes more) vs required-explicit sets (parallel-friendlier, riskier). *Owner: operator; policy interplay with T02's 4.3 findings.*
5. **Backup posture now** — adopt pinned Litestream v0.5.x alongside the dump-snapshot drill, or dump-snapshots only until T12 designs 11.3 fully. *Owner: operator; mechanics → T12.*

**Spikes (carried + new):**

6. **serialize-by-deny** fallback probe (carried from S2 — sizes the 20% parallel-tool-call gate fallback). *Owner: next spike battery.*
7. **opencode live-auth park round-trip** (carried from S3; Z.AI key steps listed there). *Owner: next spike battery.*
8. **systemd harvest matrix on this host** — user-vs-system service decision (user.slice freeze makes system-service likely), `--remain-after-exit` + `daemon-reload` + reboot behavior on Ubuntu 26.04's systemd, `InhibitDelayMaxSec` actual value, `ExitType=cgroup` with sandboxed process trees. *Owner: implementation spike.*
9. **kill -9 harness prototype** — build the §4.7 loop against the schema draft before the schema is final; it is the cheapest way to falsify the design. *Owner: implementation start; becomes a standing conformance-suite entry.*

**Watches:**

10. **DBOS TS/SQLite parity + any external-process step mode** — the pre-registered flip condition for candidate B. *Owner: S2.8 watch; revisit at G2's successor.*
11. **MCP 2026-07-28 final (Tasks/Extensions)** — durable task handles as a future parking/cancel convergence (carried from R05 OQ9). *Owner: S2.8 watch.*
12. **Litestream #1083** (silent replication failure class) — gates decision 5. *Owner: S2.8 watch.*

**Contradiction records (explicit, per campaign rules):**

- **Report 05 §3-B/§5 ("DBOS — requires Postgres")**: stale as of dbos-transact-py 1.13.0 (2025-09-02, SQLite default for new Python apps). The rejection **stands on re-argued grounds** (§3-B: control-flow ownership, coverage, maturity asymmetry) — recorded so no future session cites the dead reason.
- **Report 05 §4.9 ("system-sleep hook")**: refined, not reversed — the sanctioned mechanism is a logind **delay inhibitor lock + `PrepareForSleep`**, with a ~5 s default budget (`InhibitDelayMaxSec`); system-sleep scripts are documented "hacks" and run with user.slice frozen. Wake-side reconcile design (§4.4) absorbs the correction.

**New platform problems (for the spec's Known-problems list):**

- **P-T07-1 — The pre-sleep budget is ~5 seconds.** `InhibitDelayMaxSec` defaults to 5 s; no checkpoint pass fits there. → D7's always-durable property is the design answer: pre-sleep is an O(1) flush; all real work is wake-side. *(Feeds run-lifecycle spec.)*
- **P-T07-2 — Reboot destroys the finished-during-outage evidence that restart preserves.** systemd unit corpses survive control-plane death and daemon-reload but not reboot. → The run wrapper's own exit record (event-log append as its last act) is mandatory durable evidence; unit corpses and engine session stores are enrichment; the ladder must classify correctly from records+mining alone. *(Feeds §4.4 spec + conformance suite.)*
- **P-T07-3 — Idempotency-less effect channels leave an irreducible unknown-outcome window.** Plain SMTP-class effects cannot be made exactly-once by any journal. → `unknown` is a first-class effect state with a human card; **channel/provider choice is a durability design input** (prefer key-capable providers); per-provider idempotency registry as data. *(Feeds 4.2 spec + operator decision 3.)*
- **P-T07-4 — No clock on a sleeping laptop can be trusted for expiry or ordering.** MONOTONIC freezes across suspend (dead-looks-alive), BOOTTIME mass-expires on wake (alive-looks-dead), REALTIME steps under NTP. → Ordering exclusively from `event_seq`; deadlines as wall-clock DB columns evaluated suspend-aware (wake grace); chrony step-limits noted in ops docs. *(Feeds run-lifecycle + scheduler specs; extends P-T02-3.)*
- **P-T07-5 — The platform's own event log is a growth/bloat failure mode.** LangGraph's measured 85%-bloat/no-TTL record is the cautionary tale. → Payload caps, refs-not-blobs, validate-before-persist, and an explicit event-log retention/size watch distinct from 11.1's trace retention. *(Feeds schema spec + S2.7 watch.)*

No other contradictions with prior reports found: P-T01-1/-2/-4, P-T02-1/-3/-4, and the R07 ledger design are all reinforced; the ratified FSM, drain grace, hold-vs-park, and freshness-fingerprint decisions are consumed unchanged.

---

## 8. Sources

All accessed 2026-07-17. P = primary (project docs, repo/source, issue tracker, kernel/systemd man pages, live probe), S = secondary. Single-source claims flagged inline. Internal inputs: `Research/spikes/G1-S1|S2|S3`, `Research/decisions/GATE-1-architecture-direction.md`, Reports 01/05/07.

**Durable-execution frameworks**
- [S1] P — docs.temporal.io/temporal-service/persistence — SQLite "only for development and testing".
- [S2] P — docs.temporal.io/workflow-execution/limits (+ /event, self-hosted-guide/defaults) — 51,200-event/50 MB termination; 10,240/10 MB warnings; Update/Signal caps.
- [S3] P — temporal.io/blog/announcing-openai-agents-sdk-integration + change-log + ai-cookbook — agents GA 2026-03-23; LLM calls as Activities.
- [S4] P — docs.dbos.dev/python/tutorials/database-connection + /python/reference/configuration — "By default, DBOS uses SQLite"; Postgres-for-production rationale is multi-server.
- [S5] P — github.com/dbos-inc/dbos-transact-py releases + LICENSE — SQLite default in 1.13.0 (2025-09-02); 2.9.0/2.10.0 hardening; MIT; TS/Go coverage per dbos-transact-ts/-golang releases (Go SQLite v0.16, 2026-06).
- [S6] P — dbos.dev blog (March/June 2026) + ai.pydantic.dev/durable_execution/dbos — agent integrations on SQLite; Conductor self-host paid-for-production.
- [S7] P — github.com/restatedev/restate release-notes v1.6.0/v1.7.0 + releases — pause/resume added v1.6.0 (2026-01-30); v1.7 persisted pause, abort-immediate; v1.7.2 current.
- [S8] P — raw restate LICENSE + docs.restate.dev — BUSL-1.1 → Apache-2.0 (4 y); internal-use grant; SQL introspection.
- [S9] P — inngest.com/docs/self-hosting + /docs/features/…/step-ai-orchestration — in-memory Redis + periodic SQLite snapshots "including prior to shutdown"; step.ai.infer offload.
- [S10] P — lucumr.pocoo.org/2025/11/3/absurd-workflows + /2026/4/4/absurd-in-production + github.com/earendil-works/absurd (LICENSE: Apache-2.0) — stored-procedure durable execution; no deterministic replay; agent loops.
- [S11] P/S — simonwillison.net/2025/Dec/24/absurd-in-sqlite — SQLite port PoC.
- [S12] P — morling.dev/blog/building-durable-execution-engine-with-sqlite — "persistent memoization"; SQLite endorsement.
- [S13] P — github.com/obeli-sk/obelisk + obeli.sk blog "SQLite is All You Need" (2026-05-29) — single binary, SQLite default, WASM-only execution, AGPL-3.0.
- [S14] S — byteiota.com/sqlite-durable-workflows-skip-temporal (2026-05-30) — graduation threshold.
- [S15] S — zylos.ai/research/2026-04-24-durable-execution-agent-runtimes — journaled step boundaries; approval artifact-hash binding; version-drift fields; no framework models interactive subprocess steps.

**Agent checkpoint/recovery patterns**
- [S16] P — github.com/langchain-ai/langgraphjs #1138 — ~100 rows/invocation; no TTL since 2025-04.
- [S17] P — github.com/langchain-ai/langgraph #7714 (+ #6491) — 85.3% bloat, 37.8% token overhead, unvalidated corrupt payloads [maintainer-unanswered at access].
- [S18] P — arXiv 2511.03690 (OpenHands SDK) — event-sourced, replayable; silent on workspace recovery.
- [S19] P — code.claude.com/docs/en/agent-sdk/sessions — "persist the conversation, not the filesystem"; fresh-session advice; cwd-mismatch fresh-session behavior.
- [S20] P — cognition.ai blockdiff engineering post; Factory Droid docs — VM-snapshot resume units.
- [S21] P — Gemini CLI checkpointing docs — shadow-git + conversation + pending-call trio (off by default).
- [S22] P/S — jj docs (operation log, working-copy snapshots) + AgentJJ (2026-02) — adoptable workspace-restore machinery.
- [S23] P — Google ADK sessions docs — state_delta append-only.
- [S24] P — OpenAI Agents SDK sessions docs — run-state serialization incl. approvals; SQLiteSession in-memory default.
- [S25] P — public agent-gateway issue tracker (#45456, #45325, #8523) — restart auto-resume duplicate instances; zombie sessions; reset vs in-flight run [single project, flagged].
- [S26] P — github.com/anthropics/claude-code #54393 — 12-bug multi-agent post-mortem; loss-without-process-death.

**Orphan recovery, leases, supervision**
- [S27] P — Sidekiq Pro reliability wiki — super_fetch, 60 s heartbeat, 3-recoveries/72 h poison-pill.
- [S28] P — github.com/rails/solid_queue README + issue #422 + PR #277 (+ #589, #591) — 60 s/5 min defaults; ProcessPrunedError failed-not-requeued rationale; graceful-TERM releases; SQLite support.
- [S29] P — River docs — RescueStuckJobsAfter = max(1 h, JobTimeout+1 h).
- [S30] P — Oban Lifeline docs — rescue_after 60 min; dead-nodes-only.
- [S31] P — Faktory wiki — FETCH lease 1,800 s default.
- [S32] P — AWS SQS docs — visibility timeout, ChangeMessageVisibility heartbeat.
- [S33] P — martin.kleppmann.com "How to do distributed locking" — fencing tokens.
- [S34] P/S — docs.temporal.io/encyclopedia/detecting-activity-failures + /activity-operations + community/issue #2538 — heartbeat throttle (0.8×, 30 s/60 s); attempt fencing documented for Reset; zombie-completion rejection community-documented.
- [S35] P + live test — man systemd-run(1), systemctl(1), systemd.unit(5) (CollectMode; /run/systemd/transient in search path), org.freedesktop.systemd1(5) — remain-after-exit harvest; reset-failed; daemon-reload survival (live-tested, systemd 259); reboot loss.
- [S36] P — man systemd.service(5) + systemd PR #13754 — ExitType=cgroup (v250); oneshot Restart=on-failure (v244); ExecStop/TimeoutStopSec.
- [S37] P — docs.openclaw.ai/gateway/restart-recovery — one-txn claim; boot scan; deliver-instead-of-redo; one durable dispatch id ≤3 retries; lost-after-grace; wedged tombstones; 5-min drain; >2 h finalize. [3-vote verbatim SUPPORT]
- [S38] P/S — Kubernetes controller concepts + kubelet architecture — level-triggered reconcile; re-list actuals on restart.
- [S39] P — Nomad plugin docs — RecoverTask/reattach-or-LOST.

**Exactly-once effects**
- [S40] S — exactly-once delivery/processing canon (Two Generals treatments).
- [S41] P — docs.dbos.dev/python/tutorials/workflow-tutorial — "Steps are tried at least once…"; "Transactions commit exactly once."
- [S42] P — microservices.io/patterns/data/transactional-outbox — the crash windows.
- [S43] P — docs.stripe.com/api/idempotent_requests — 24 h; error replay; execution-begins carve-out (validation/concurrency); param-mismatch error.
- [S44] P — Adyen/PayPal/Square/Svix idempotency docs — divergent windows/semantics.
- [S45] P — datatracker.ietf.org draft-ietf-httpapi-idempotency-key-header — -07 expired 2025-10-15, archived.
- [S46] P — git-scm docs (push, --force-with-lease, --atomic) — ref-level idempotency/CAS.
- [S47] P — resend.com docs (idempotency keys) + AgentMail — idempotent email send.
- [S48] P — GitHub REST docs — no idempotency mechanism for comments/issues; tag-exists dedup on releases.
- [S49] P — RFC 1047 — SMTP crash-duplicate window (1988; foundational, flagged old).
- [S50] P — OWASP AI Agent Security Cheat Sheet — idempotent-or-confirm rule; approval binding fields.
- [S51] P — Terraform docs (plan -out / apply saved plan) — approve-exactly-this-artifact; stale-plan refusal.
- [S52] S — arXiv 2607.05744 (approval-view fidelity attacks) + practitioner payload-hash-pinning posts.
- [S53] P — github.com/crewAIInc/crewAI #5802; langgraph cloud #7417 — retries re-execute completed tools (open, production-confirmed).
- [S54] P — github.github.com/gh-aw/reference/safe-outputs + threat-detection (project moved to github/gh-aw) — read-only default, gated writes, staged mode, per-type maxes.
- [S55] P — modelcontextprotocol.io spec 2025-11-25 (tools + schema.ts) + MCP blog (2026-07-28 RC) — destructiveHint default true / idempotentHint false; annotations MUST be treated untrusted; Tasks/Extensions pending.

**SQLite substrate, backup, crash testing, laptop hazards**
- [S56] P — sqlite.org/pragma.html#pragma_synchronous + sqlite.org/wal.html — NORMAL/FULL semantics verbatim; app-crash durability; checkpoint starvation; same-host constraint.
- [S57] P + live test — sqlite.org/lang_transaction.html + rescode.html + c3ref/busy_handler.html — deferred-upgrade BUSY; busy-handler deadlock bypass (live-tested 0.000 s); BEGIN IMMEDIATE no-BUSY-through-COMMIT guarantee.
- [S58] P — sqlite.org/howtocorrupt.html — file-copy/journal-deletion/close()-lock-cancellation/lying-drives.
- [S59] S/P — docsaid.org sqlite-job-queue-atomic-claim (2025-09); medium.com arthurpro SQLite queue (2026-06); dev.to minnzen durable queue for agents — UPDATE…RETURNING claim consensus.
- [S60] P — github.com/maragudk/goqite, litements/litequeue, justplainstuff/plainjob (15 k jobs/s self-benchmark) — pattern libraries.
- [S61] P/S — tailscale.com/blog/database-for-2022; pocketbase.io/faq; Bluesky PDS discussions — SQLite primary-store precedents.
- [S62] P — sqlite.org/lang_vacuum.html + recovery.html + cli.html — VACUUM INTO consistency; .recover = salvage only; interrupted-VACUUM-INTO caveat.
- [S63] P — github.com/benbjohnson/litestream releases + litestream.io/docs/migration + fly.io v0.5 post + mtlynch.io notes — LTX rewrite (v0.5.0, 2025-09-30); v0.5.7 transparent v0.3 restore (Age excepted); v0.5.14 (2026-07-06); bug #1083.
- [S64] P — fly.io/docs/litefs + community status thread — frozen beta; Cloud sunset.
- [S65] P/S — github.com/tailscale/squibble; dbmate; sqlite user_version practice posts — solo migration discipline.
- [S66] P — sqlite.org/testing.html — crash-simulation VFS; TH3 power-loss damage; integrity_check postcondition.
- [S67] S — github.com/utsaslab/crashmonkey + CrashMonkey/ACE paper + arXiv 2503.01390 — academic crash-consistency tooling (heavyweight).
- [S68] P — docs.kernel.org suspend-flows + freezing-of-tasks + Documentation/ABI/testing/sysfs-power + kernel/power/Kconfig — freeze/sync ordering; sync_on_suspend default-on, disableable.
- [S69] P — systemd.io/INHIBITOR_LOCKS + man logind.conf(5) — delay locks + PrepareForSleep; InhibitDelayMaxSec "Defaults to 5."
- [S70] P — man systemd-suspend.service(8) — system-sleep scripts "considered hacks"; user.slice frozen.
- [S71] P — man clock_gettime(2) + timerfd_create(2) — MONOTONIC excludes suspend; BOOTTIME includes; TFD_TIMER_CANCEL_ON_SET.
- [S72] S/P — cockroachlabs.com clock-management post; oneuptime event-timestamps (2026-01) — NTP step/slew practice; monotonic-id ordering.
- [S73] S — arkency audit-log-vs-event-sourcing; leapcell single-table event log; medium event-sourcing-audit-logs — "mutable state + append-only log" consensus.
- [S74] S/P — sqliteforum event-sourcing-with-sqlite; mattbishop/sql-event-store; eventsourcing lib SQLite recorder — INTEGER PRIMARY KEY ordering idiom.

**Parallel same-project coordination**
- [S75] P/S — Cursor background-agent/worktree docs (25-worktree cap); Vibe Kanban; Sculptor container dissent — worktree-per-task consensus.
- [S76] S — Autonoma disjoint-file-set + overlap-zone practice; task-claim files; MPAC-class declared read/write sets (paper-stage) — plan-time claims advisory-only in the field.
- [S77] S — parallel-generation-sequential-merging practitioner posts; merge-queue first-gate idiom — merge-time reality.
