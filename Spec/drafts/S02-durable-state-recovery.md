## S02 — Durable state, checkpointing & recovery

**Scope:** The authoritative durable-state layer beneath the harness — `platform.db`, checkpoint-per-paid-call, the run-lifecycle FSM, the startup/wake recovery ladder, the two-phase effect journal, and artifact-level project coordination — specifying what MUST survive crash, sleep, and disk death so that no paid work is lost, no outward effect is ever repeated, and every record is owner-attributed from day one.
**Binding inputs:** R08 §4 [G2 D2.1]; G1 D1.1 (run FSM + SQLite-WAL event log); G1 Def.4 (hold-vs-park), G1 Def.5 (freshness fingerprint); G2 Def.1 (`synchronous=FULL`), Def.2 (lease/liveness set), Def.3 (whole-project claim), D2.4 (effect-channel policy), D2.5 (backup posture); SPIKE P2-S2 (systemd harvest matrix + storage seam), SPIKE P2-S4 (serialize-by-deny); D2, D6, D7, 15.6; P-T07-1..5, P-T02-3, P-T01-1.

The vocabulary of this section: **harvest** = recovering the *result* of work that finished during an outage instead of re-running it; **generation** = a per-run monotonic fencing counter, bumped on every takeover or resume, stamped into every event append; **recovery ladder** = the ordered ALIVE / WEDGED / FINISHED-DURING-OUTAGE / DEAD classification run at start and wake; **transcript copy-aside** = a per-checkpoint file copy of a Claude-lane engine transcript segment, kept as harvest/insurance material only. These four terms are coined here.

### S02.1 The state substrate — `platform.db`

One SQLite database file in WAL mode is the authoritative state of the platform [R08 §4.1; G2 D2.1]. The `sinet-control` process **MUST be the sole writer**; dashboards, the CLI, and read-model followers open read-only connections (P-T01-1 discipline at the storage layer) [R08 §4.1].

- **Durability.** `⚙ state.synchronous = FULL` [G2 Def.1] — at Sinet's write rate (a handful of writes per paid call, seconds apart, tens/min at peak fan-out) the per-commit fsync is unmeasurable and it deletes the power-loss window entirely; `NORMAL` is the documented fallback if measurement ever disagrees, and the schema already tolerates a last-commits rollback because every cross-table invariant is written in a single transaction [R08 §2.5.1/§4.1].
- **Write discipline.** Every writing transaction **MUST** use `BEGIN IMMEDIATE` — a deferred read→write upgrade against a moved DB fails `SQLITE_BUSY` *immediately* (the busy handler is deliberately bypassed); `BEGIN IMMEDIATE` honors `⚙ state.busy_timeout = 5 s` and, once it succeeds, returns no `SQLITE_BUSY` through COMMIT [R08 §2.5.2]. `foreign_keys=ON` on every connection.
- **Read hygiene.** No long-lived read transactions — event tailing reads in short batches by `event_seq` cursor; a long reader starves the WAL checkpoint and grows the WAL without bound. Periodic `wal_checkpoint(TRUNCATE)` at `⚙ state.wal_truncate_interval = 1 h`, with a WAL-size watch feeding [XREF:S14] [R08 §2.5.3/§4.1].
- **Integrity.** `PRAGMA integrity_check` at every platform start (cheap at this size); it is also the crash-harness postcondition (S02.9) [R08 §4.1].
- **Migrations.** `PRAGMA user_version` + numbered one-transaction SQL files; event payloads carry a per-type `schema_version` and are upcast on read. **History is NEVER rewritten** — append-only [R08 §2.5.8/§4.1].
- **Event-payload discipline** (P-T07-5): payloads capped at `⚙ state.event_payload_cap = 64 KB`; bulky tool output and artifacts live as files (or engine stores) referenced by path+hash, not inlined; events are validated against their `schema_version` **before** persist — never forward an unvalidated payload into a later turn (the LangGraph corruption class) [R08 §2.2/§4.1].
- **Corruption etiquette**, enforced by convention + lint: nothing ever file-copies the live DB (snapshots go through `VACUUM INTO`, S02.9); the `-wal`/`-shm` sidecars are never deleted or renamed; the DB is never opened read-write by any process but the control plane; WAL is same-host only (never a network filesystem) [R08 §2.5.4/§5].

### S02.2 Schema core — the ~12 owner-attributed table families

The minimal durable set that covers every in-scope feature item, ratified as R08 §4.2 [G2 D2.1] and sanity-checked against Archon's 7-table reference (the delta over Archon is exactly the machinery the spec demands and Archon lacks). **Every row carries its owning `user_id` (15.6)** — identity is in the schema from day one even though v0 operates single-user. Names are indicative; exact DDL is the P3 schema workshop's job, jointly with the Ledger schema [XREF:S05].

| Table family | Holds | Anchor |
|---|---|---|
| `users` | identity, per-person credential-store refs (D2) | 15.6, 10.x |
| `tasks` | user-facing task + kanban status, orthogonal to run machinery | 9.1, S1.3 |
| `runs` | FSM `state` (S02.3), owner, task, substrate/lane, **lease** {holder, wall-clock deadline, heartbeat cursor}, **generation** (fencing counter), systemd unit name, workspace/worktree ref, **ceilings** (time/steps/cost — 3.7) | 3.7/3.8, D6 |
| `run_events` | append-only log: `event_seq INTEGER PRIMARY KEY` (the sole ordering authority — never timestamps), run_id, generation, type, `schema_version`, JSON payload, wall-clock ts. **This is the D7 event record and the observability substrate** [XREF:S14] | 4.8, 11.1 |
| `checkpoints` | one row per paid model call (S02.4) | D7, 4.3 |
| `asks` | every gate/question the moment observed: full invocation-reconstruction snapshot, status, observed/answered ts, answer payload, engine-expiry watch (P-T02-1) | 4.2/4.3 |
| `effects` | the two-phase journal (S02.7): proposal payload + normalized `payload_hash`, class A–D, approval record, state, `idempotency_key`, provider window ref, attempts, result | 4.2, 3.8 |
| `queue` / claim columns | CAS claiming: status, `claimed_by`, lease columns, priority lane | 3.3 |
| `lanes` / `slots` | per-(user, model/lane) concurrency caps as data | 3.11, D4 |
| `artifact_claims` | S1.11 registry: task, project, path/glob set, mode R/W, declared-at-plan, status | S1.11 |
| `engine_sessions` | session registry incl. copied-aside transcript path — the reboot-case mining index (P-T07-2) | 3.8 |
| `receipts` / usage | derived from checkpoint usage rows, materialized per run-end (design [XREF:S10]) | 3.6, D5 |

The Task Context Ledger is not a separate table family here: its revisions are persisted as `run_events` (type `ledger_update`, full content — it is small by design), so the D7 checkpoint payload is self-contained in the DB even if the workspace is destroyed; the working copy in the task workspace is a projection [R08 §4.2]. The Ledger's internal schema is [XREF:S05].

### S02.3 Run-lifecycle FSM

The run FSM ratified at G1 D1.1 (R05 §4.1), with the storage-layer precisions R08 adds. The current state is a column on `runs`; **every transition is a `run_events` append in the same transaction as the `runs.state` update** [R08 §4.2].

**Stored states:** `new` → `queued` (admitted to the claim queue) → `claimed` (CAS-claimed, lease held) → `running` (engine subprocess live, event cursor advancing). From `running`: `parked` (suspended on a limit event or an open gate/ask; resumes on provider signal, schedule, or approval — the limit-event taxonomy and park verbs are [XREF:S10] / [XREF:S03]); `draining` (maintenance mode — finishing the current paid call to a checkpoint boundary before parking; 4.5); and the terminals `completed`, `crashed`, `finalized` (finalize-with-card on stale resume), and `tombstoned` (a repeat-offender wedged run). `died-at-gate` is a first-class terminal for the measured case where a budget/ceiling preempts a park before it can resume [R08 §4.2, from SPIKE-measured behavior].

**Derived, NEVER stored** (computed at reconcile from unit liveness + event cursor): **stalled / wedged** — unit active but the event cursor has not advanced past `⚙ recovery.dead_after`. Keeping "stalled" derived rather than stored is a ratified FSM discipline: a stored stall would go stale across a nap and mislabel a live worker [R05 via G1 D1.1; R08 §2.3]. The recovery classes (S02.5) are likewise derived per pass.

**Transitions (summary):** `new→queued→claimed→running`; `running↔parked` (gate/limit, resume); `running→draining→parked` (maintenance drain, past the ⚙15-min grace — policy [XREF:S10]); a paid call appends a checkpoint event and stays `running`; `running→completed`; `running→crashed`, then superseded by **fork-from-last-checkpoint as a new run with `generation+1`** (S02.5); `parked→died-at-gate` when a budget preempts; `{crashed | finished-during-outage}→completed` via harvest; a repeat-offender wedged run → `tombstoned`. Effect sub-states (`executing`/`unknown`) are first-class on `effects`, not on `runs` (S02.7).

### S02.4 Checkpoint-per-paid-call

**D7 invariant, at the storage layer:** a checkpoint row **MUST** be written after every paid model call, in the same transaction as its run-event append; re-spend after any disaster is then bounded *structurally* to "work since the last paid call," and *economically* by the subscription cache TTL a warm resume rides (scheduler input [XREF:S10]) [R08 §2.2/§4.3; D7]. A checkpoint row contains, identically across substrates (measured schema parity):

- **(a) usage block** — input/output tokens, `cache_read`/`cache_creation` by TTL bucket, cost fields;
- **(b) session cursor** — substrate, engine session id *as reported by the engine's `system/init`* (never the requested id), message index, cwd key, transcript path;
- **(c) Ledger revision** — the Task Context Ledger content or content-hash; the D7 context payload [XREF:S05];
- **(d) artifact snapshot ref** — opencode lane: the native per-step snapshot id from its store; Claude lane: a platform-owned snapshot commit in the run worktree (jj is a post-v0 candidate);
- **(e) version fields** — model id, invocation-config fingerprint (settings hash + permission mode + model), tool/prompt schema versions — so recovery can detect that a resume would run under changed assumptions and route to the freshness pass (S02.6) instead of replaying blind.

**Transcript copy-aside** (Claude lane): at each checkpoint the engine transcript segment is copied aside and indexed in `engine_sessions` — cheap insurance against the ~30-day session sweep, cwd-key breakage, and crash-truncated JSONL, and the raw material for reboot-case harvest [R08 §4.3].

**Engine transcripts are NEVER durable checkpoints** (P-T01-1): the platform checkpoint row is authoritative; the engine session store and the copy-aside are a resume optimization and a harvest source, nothing more. The vendor's own doctrine agrees ("sessions persist the conversation, not the filesystem"; a resume silently returns a fresh session on cwd mismatch) [R08 §2.2].

### S02.5 The startup / wake recovery ladder

One level-triggered reconcile pass — the same code at platform start, at wake, and on a periodic sweep `⚙ recovery.sweep_interval = 5 min` — following the kubelet rule: re-list actual state, never trust supervisor memory [R08 §4.4].

**Pre-sleep** (P-T07-1): the control plane holds a logind sleep **delay** inhibitor and, on `PrepareForSleep(true)`, does only an O(1) durable flush (commit open work, `wal_checkpoint`, append a `suspend` event) inside the inhibitor budget. That budget is short and host-configurable — 5 s systemd default, 30 s on the v0 host via an unattended-upgrades drop-in [SPIKE P2-S2] — so **nothing heavy is ever sized to it**: D7 already keeps state durable at all times; all real reconcile is wake-side, enqueued by `PrepareForSleep(false)` [R08 §2.6/§4.4].

**Suspend/resume is a crash-equivalent** and the double-resume factory of P-T02-3: a suspended laptop is the GC-pause scenario made physical. Two mechanisms make it harmless — generation fencing and suspend-aware leases (below). All TCP/SSE connections are presumed dead after wake; a streamed response in flight before the nap may have completed *and been paid for* server-side, so the pass reconciles against the engine session store before any re-spend.

**The pass, per non-terminal run:**

1. **Observe actuals** — DB rows ⇄ systemd units (live + `--remain-after-exit` corpses, reading `ExecMainStatus`/`Result`/`CPUUsageNSec`/`MemoryPeak`; the lane recipe is `RemainAfterExit=yes` + `ExitType=cgroup` + `Type=exec`, **never `--collect`** — the sandbox binary exiting ≠ the process tree being dead) ⇄ engine session state ⇄ worktrees [SPIKE P2-S2; unit model [XREF:S01]/[XREF:S03]].
2. **Classify (the ladder):** **ALIVE** (unit active, cursor advancing) → reattach streams, re-arm watchdogs. **WEDGED** (unit active, cursor stalled past threshold) → pause-and-flag, **never auto-kill** (D1.3). **FINISHED-DURING-OUTAGE** (unit corpse exited 0, or the engine store shows a terminal result newer than the last checkpoint) → **harvest**: mine the result/session store, append the missing events/usage **deduplicated by (session_id, message_id)**, deliver the produced result instead of redoing the work, mark `completed`. **DEAD** (unit gone/failed with nothing newer to harvest) → mark `crashed`, then supersede by fork-from-last-checkpoint as a new run with `generation+1`.
3. **Bound recovery:** attempts reuse **one durable dispatch id** per interruption (an ambiguous failure cannot double-start); `⚙ recovery.max_attempts = 3` with backoff; repeat offenders are `tombstoned`; a run interrupted longer than `⚙ recovery.stale_finalize = 24 h` is finalized-with-card rather than blindly resumed (the freshness pass, S02.6, fires first regardless).
4. **Leases + fencing:** lease deadlines are wall-clock DB columns; expiry evaluation is **suspend-aware** — on wake, apply `⚙ recovery.wake_grace = 120 s` before declaring any lease dead (the BOOTTIME/MONOTONIC trap, P-T07-4); every event append carries the run's `generation`, and the writer **rejects stale-generation appends** — the fencing that renders double-resume inert. Ordering comes only from `event_seq`; no single clock is trusted (P-T07-4).
5. **Asks reconcile:** platform `asks` rows are authoritative (engine-side asks are volatile in memory); re-hydrate the engine where it still holds the ask, re-prompt or re-issue full reconstruction where it lost it — the ask row's invocation snapshot is the resume input.
6. **Effects reconcile:** any `executing` row is in-doubt → per-class resolution (S02.7): replay with the idempotency key (A/B), search-before-retry (C), or flip to `unknown` + card (D). **This closes 3.8's "outward actions never repeated" across the crash window.**
7. **GC:** unit corpses `reset-failed` after harvest; orphan worktrees flagged, **never auto-deleted** while they hold uncommitted work; engine session files past retention; tombstone-review cards.

**Reboot asymmetry** (P-T07-2): unit corpses survive control-plane restart, `daemon-reload`, and `daemon-reexec` (measured on the v0 host's systemd 259) but **not reboot** — the journal is persistent and survives reboot, but corpses do not [SPIKE P2-S2]. Therefore the run wrapper **MUST** write its own exit record (a `run_events` append as its last act) as the durable evidence; corpses and engine stores are enrichment, and the ladder must classify correctly from records + mining alone.

**Lease/liveness numbers** (all `⚙`, ratified as a set) [G2 Def.2]: heartbeat 60 s, dead-after 5 min, wake grace 120 s, ≤3 recovery attempts, stale-finalize 24 h, approval expiry 7 d.

### S02.6 Freshness re-validation on stale resume (4.3)

"Blocked is not failed" — an answered run resumes from its checkpoint; but if it slept past a threshold or its target moved, the remaining plan is re-validated against current reality at low cost **before spending anything significant**, then continues, adjusts-with-a-note, or escalates as an explicit decision. S02 owns the durable *inputs* to that decision; the low-cost re-plan *action* is [XREF:S06], and the hold-vs-park mechanics are [XREF:S03].

- **Freshness fingerprint (the durable set)** [G1 Def.5]: `{repo HEAD, source content hashes/ETags, spec+plan version, price-table version}` — plus, when a plan's assumptions cite project-truth knowledge entries, the **cited entry versions** (ratified extension [R11 §4.7; XREF:S09]). The checkpoint's version fields (S02.4e) plus these are what a resume compares against.
- **Triggers:** age `> ⚙ freshness.max_age = 24 h` **OR** any fingerprint drift **OR** a sibling-accept event in the project (S02.8). **Price-table drift alone triggers** — it re-prices the remaining plan, so a changed price table is a first-class staleness cause, not a cosmetic one [G1 Def.5].
- **Hold-vs-park:** a pause shorter than `⚙ freshness.hold_vs_park = 10 min` holds the process (cheap resume, no staleness); past it the run parks and is therefore subject to the freshness pass on resume [G1 Def.4].

`freshness.max_age` and `freshness.hold_vs_park` are G1-ratified and cross-cutting (fed here by checkpoint version fields, enforced by intake/adapter); they are listed in S02's settings table for completeness and deduped by [XREF:S18].

### S02.7 The two-phase effect journal (exactly-once outward effects)

The largest net-new specification in this section and the storage-layer realization of D7's gate (4.2) and 3.8's "outward actions never repeated." Guarantee vocabulary (adopted from DBOS wording): **journal/state writes are exactly-once** (same transaction as the FSM transition); **provider calls are at-least-once** (classes A–C) or at-most-once (class D); dedup lives at the effect boundary and is what makes the difference invisible [R08 §2.4/§4.5].

**Lifecycle** (`effects` row): `proposed` (created by the gate; payload normalized + hashed) → `approved` (approver, timestamp, **`payload_hash` pinned**, `⚙ effects.approval_expiry = 7 d`; Terraform saved-plan semantics — drift or expiry sends it back to re-approval, integrating the S02.6 triggers) → `executing` (own transaction, **written before the provider call**, `payload_hash` re-verified inside that transaction, `idempotency_key` = the effect UUID) → `succeeded` / `failed` / `unknown`. A restart scans in-doubt `executing` rows (S02.5 step 6).

**Per-provider idempotency registry** — data, not code (like the price table): the IETF `Idempotency-Key` draft expired with no successor and provider windows/semantics diverge widely, so uniform assumptions are forbidden [R08 §2.4/§5].

| Class | Example | Strategy | Crash-window disposition |
|---|---|---|---|
| **A** natively idempotent | `git push --force-with-lease` (ref-level CAS) | blind retry | safe |
| **B** key-dedupable | Stripe-class APIs; **Resend-class email** | retry with the stored key, respecting the provider's window (Stripe 24 h incl. error replay; Svix success-only 12 h; Adyen ≥7 d; PayPal ≤45 d) | deduped by provider |
| **C** query-before-retry | GitHub comments/issues (no idempotency) | deterministic effect-id marker embedded in the payload + search-before-retry | deduped by search |
| **D** no idempotency | plain SMTP | at-most-once execution | flip to `unknown` + a decision card showing the human what to check (5.6) |

**Idempotent-capable-channel policy** [G2 D2.4]: email-class outward effects **require** idempotent-capable providers (Resend-class keys); plain SMTP is admissible **only** as an explicit per-channel exception. Channel choice is thereby a durability design input, and the class-D `unknown` card path stays exceptional rather than routine (P-T07-3). MCP `destructiveHint`/`idempotentHint` are spec-mandated-untrusted hints — classification input at most, never a gate bypass [R08 §2.4].

### S02.8 Artifact-level coordination inside a project (S1.11)

Concurrent follow-up tasks in one project proceed in parallel when they touch disjoint artifacts and are sequenced when they would collide — detected at planning time, never resolved by silent overwrite [S1.11; R08 §4.9].

- **Write-set claims at plan time.** At plan approval a plan declares its write-set (paths/globs) and optional read-set into `artifact_claims`. The registry check is glob-intersection against active claims in the same project: disjoint → run; overlap → **sequence** (queue behind the holder) or **explicit branch** as a decision card — surfaced at plan time, which no shipped product does.
- **Whole-project claim when unbounded** [G2 Def.3]: `⚙ claims.default_write = whole-project` — when a plan cannot bound its write-set it claims the whole project (serializes more, never overwrites). The conservative default; `declared-set` is the parallel-friendlier alternative.
- **Enforcement is the control plane, not an OS lock** (claims are rows, consistent with the sole-controller posture). The ratified enforcement primitive at the tool-call gate is **serialize-by-deny** [SPIKE P2-S4]: a control-plane PreToolUse deny with the reason "re-issue the gated call alone" makes the engine re-issue the contended call by itself, which then parks cleanly with a *faithful single-call* ask record. P2-S4 demonstrated it VIABLE end-to-end (deny the batch → single serialized retry → clean park, ~+1 model turn, ~$0.013 on the cheap lane); the same primitive serializes a run behind an active write-claim. Its distinct value over the default always-defer park ([XREF:S03] owns the park default) is the faithful ask-record fidelity — always-defer parks 1-of-N and re-derives the siblings on resume.
  - **Detection binding:** classify a parallel park by **post-park transcript reconstruction**, never by reading the transcript at fire time (a flush-lag hazard that misfires); the adapter-side detection contract is [XREF:S03].
  - **Carry-forward caveat:** serialize-by-deny was demonstrated only on `claude-haiku-4-5`; it **MUST** be reconfirmed on the default worker model — the model that actually drives the ~20% parallel-batch rate — before it is relied upon. `TBD-P3(reconfirm serialize-by-deny on the default worker model)` [SPIKE P2-S4 caveat].
- **Sibling-accept** fires the ratified freshness trigger (S02.6) as an event to all active runs in the project. **At accept time**, the first gate is applies-cleanly-on-current-HEAD (the merge-queue idiom); a collision surfaces as a reviewable merge card — S1.11's "never silently overwritten," verbatim.

### S02.9 Crash-test practice and the durable-set snapshot

**The kill-9 harness is a standing conformance-suite entry**, same standing as the adapter suites [R08 §4.7; P-T01-2]: synthetic load on a mock adapter; random `kill -9` of the control plane and run units biased to the nasty windows (mid-claim, mid-journal-append, between paid call and checkpoint); restart; then assert *application* invariants — `integrity_check` ok; no per-run `event_seq` gaps; one reconcile pass classifies every non-terminal run; **zero double-executed mock effects**; `asks` ⊇ engine pending. With `synchronous=FULL` this covers the whole loss model (SQLite tests the layer below the app harder than anyone). A companion **suspend-cycle test** injects fake `PrepareForSleep` signals + clock deltas and asserts the wake-grace/fencing behavior.

**What must BE durable for 11.3** (state survives disk death): the S02.2 table set is the snapshot payload; the shape is `VACUUM INTO` a temp file (consistent against the live DB) → text-first `.dump` (diffable, deleted-content-purged) → client-side encrypt → snapshot repo, with raw `run_events` payload bodies past the 11.1 compaction horizon excluded (traces stay local). **Snapshot/backup hooks live here, but the pipeline — encryption, keep-N, rotation, and the scheduled verified-restore drill — is [XREF:S13]** [G2 D2.5]. Pinned Litestream v0.5.x is the recommended *continuous* replication addition, deferred to implementation pending bug #1083; the dump-based snapshot is the load-bearing mechanism.

### S02.10 Workspace storage seam

Each run works in an isolated workspace (a clone/worktree of the registered project store); parallel jobs cannot interfere with each other or with live files [4.1, S1.6]. The v0 host imposes a hard constraint the spec must route around, measured directly: the project volume is a **single ~420 GB ext4 root, ~91% full (~39 GB free), with no reflink support** — so copy-on-write per-run workspaces (the Devin-class fast-clone shape) are unavailable as-is; the `xfs.ko`/`btrfs.ko` modules ship, but there is no free space or partition to host a reflink-capable volume without repartitioning (the Windows NTFS partitions are the only reclaimable space) or a loopback image [SPIKE P2-S2]. A workspace/snapshot-heavy design needs disk headroom regardless of which path is chosen. **DECIDED — operator, 2026-07-18 (at S02 review): v0 workspaces use git-worktree + overlayfs on the existing ext4 volume** (lowerdir = shared project base, upperdir = per-run writes; workspace GC is a platform duty). **Loopback-XFS is the pre-registered measured upgrade** (trigger: workspace-creation latency or disk pressure becomes a real problem); **repartition is reserved for a host rebuild.** Disk-headroom management is a standing platform duty on the ~91%-full volume. (Sandbox/confinement of the workspace is [XREF:S11].)

---

**Settings introduced (⚙):** (all operator-editable with audit trail per G1 rider 1; auto-adjust only within operator ceilings)

| ⚙ setting | default | clamp / range | ratified by |
|---|---|---|---|
| `state.synchronous` | `FULL` | {FULL, NORMAL} | G2 Def.1 |
| `state.busy_timeout` | 5 s | ≥ 5 s | R08 §4.1 |
| `state.wal_truncate_interval` | 1 h | 5 min – 24 h | R08 §4.1 |
| `state.event_payload_cap` | 64 KB | 4 KB – 1 MB | R08 §4.1 / P-T07-5 |
| `recovery.heartbeat` | 60 s | 15 s – 5 min | G2 Def.2 |
| `recovery.dead_after` | 5 min | ≥ 2× heartbeat | G2 Def.2 |
| `recovery.wake_grace` | 120 s | 30 s – 10 min | G2 Def.2 |
| `recovery.max_attempts` | 3 | 1 – 5 | G2 Def.2 |
| `recovery.stale_finalize` | 24 h | ≥ 1 h | G2 Def.2 |
| `recovery.sweep_interval` | 5 min | 1 – 30 min | R08 §4.4 |
| `effects.approval_expiry` | 7 d | 1 h – 30 d | G2 Def.2 / R08 §4.5 |
| `claims.default_write` | whole-project | {declared-set, whole-project} | G2 Def.3 |
| `freshness.max_age` † | 24 h | ≥ 1 h | G1 Def.5 |
| `freshness.hold_vs_park` † | 10 min | 1 – 60 min | G1 Def.4 |

† G1-ratified, cross-cutting (4.3); consumed here (fed by checkpoint version fields), enforced via [XREF:S06]/[XREF:S03]; listed for completeness and deduped by [XREF:S18].

**Known problems owned here:**
- **P-T07-1** — pre-sleep inhibitor budget is short and host-configurable (5 s systemd default; 30 s on the v0 host [SPIKE P2-S2]) → pre-sleep is an O(1) durable flush; all reconcile is wake-side (D7 keeps state durable always).
- **P-T07-2** — reboot destroys the finished-during-outage unit-corpse evidence that restart preserves → the run wrapper's own exit-record append is mandatory durable evidence; corpses + engine stores are enrichment; journald survives reboot, corpses do not [SPIKE P2-S2].
- **P-T07-3** — idempotency-less channels leave an irreducible unknown-outcome window → `unknown` is a first-class effect state with a human card; channel choice is a durability input (D2.4).
- **P-T07-4** — no single clock is trustworthy on a sleeping laptop → ordering only from `event_seq`; deadlines are wall-clock DB columns evaluated suspend-aware (wake grace).
- **P-T07-5** — the event log is itself a growth/bloat failure mode → payload caps, refs-not-blobs, validate-before-persist, and an event-log size watch distinct from 11.1 trace retention [XREF:S14].
- **P-T02-3** (filed by T02, *mitigated here*) — suspend/wake = double-resume factory → generation fencing (stale-generation appends rejected) + suspend-aware lease grace make it inert.
- **P-T01-1** (filed by T01, *enforced here*) — engine transcripts are not durable checkpoints → platform checkpoint rows are authoritative; the transcript copy-aside is enrichment/harvest only.

**Deferred / parked:**
- serialize-by-deny on the default worker model → `TBD-P3` re-measurement before reliance [SPIKE P2-S4 caveat].
- Litestream continuous replication → implementation phase, once bug #1083 is triaged [G2 D2.5].
- jj as the workspace-snapshot engine → post-v0 workspace-restore candidate [R08 §2.2].
- DBOS-on-SQLite as the state spine → pre-registered fallback if steps ever become discrete request/response; watch TS/SQLite parity + an external-process step mode [R08 §3-B].
- MCP Tasks/Extensions (final 2026-07-28) → durable task-handle convergence for parking/cancel; S2.8 watch [XREF:S14].
- Postgres + River/DBOS-class migration → only at multi-host growth (12.7); SQLite's same-host WAL is the hard wall; the schema is deliberately portable SQL.

**Coverage:** (Scope → subsection)
| feature-list item | subsection |
|---|---|
| 3.7 run ceilings (silent/looping detection) | S02.2 (`runs.ceilings`); detection [XREF:S14] |
| 3.8 runs survive disasters | S02.4, S02.5, S02.7 |
| 4.2 effect-gating between approved & executed | S02.7 |
| 4.3 blocked-not-failed + freshness | S02.6 |
| 4.5 maintenance mode | S02.3 (`draining`); policy/grace [XREF:S10] |
| 4.8 / D7 checkpoint-and-gate | S02.4, S02.7 |
| S1.11 parallel same-project coordination | S02.8 |
| 11.3 state survives disk death (durable set) | S02.2, S02.9; pipeline [XREF:S13] |
| 15.6 owner-attributed records | S02.2 |
| D6 lease / generation per run | S02.2, S02.5 |

**Open items for G4:** none remaining.
1. ~~Workspace-storage sub-choice~~ **DECIDED — operator, 2026-07-18 (presented at S02 review from the P2-S2 storage-seam finding): (C) git-worktree + overlayfs at v0.** Rejected-for-now with re-entry: (B) loopback-XFS image — pre-registered measured upgrade on workspace-latency/disk-pressure evidence; (A) repartition to native XFS/btrfs — reserved for a host rebuild. Bound in S02.10; G4 reviews as settled.
