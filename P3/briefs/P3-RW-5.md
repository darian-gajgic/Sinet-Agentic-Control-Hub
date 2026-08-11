# P3-RW-5 grounded brief — recovery-lease liveness for in-process pipeline work (the interview-survival fix)

**Packet class:** rework (defect, coordinator-ratified root cause, this session).
**Binding spec sections (read in full for this brief):** S02.5 (recovery ladder), S02.3 (run FSM), S02.2 (lease block), S10.7/S10.8 (scheduler claim + never-auto-kill), S14.4 (watchdog suite), S18 registry rows for `recovery.*`.
**The defect, one line:** a live, human-paced interview run was reaped as DEAD by the recovery sweep while it was demonstrably advancing — because the lease is written once at claim time and never renewed, and classification lets the dead lease outrank fresh event-cursor evidence.

The executor and the evaluation work from THIS brief plus the cited spec sections and code. Prior packet briefs are EXPIRED artifacts — do not consult them as truth.

---

## 1. What exists (verified in code this session)

**The incident (coordinator-ratified, DB events + file:line):** a live interview run was killed mid-walk during the post-answer RUNNING advance. It died 11 seconds after its lease expired, while it had written a checkpoint 14 seconds before death — the event cursor was provably fresh at the kill.

**The mechanism, each link verified:**

1. **The lease is written ONCE, at scheduler claim time.** `internal/scheduler/scheduler.go:504-547` `claimOne`: CAS-claims the queue row, transitions queued→claimed, and calls `runs.SetLeaseTx(ctx, tx, c.runID, claimHolder, deadline, 0)` (line 537) with `deadline = now + ⚙recovery.dead_after` (`keyLeaseTTL = "recovery.dead_after"`, line 165; TTL read at lines 505-513). This is the **sole production caller** of `SetLeaseTx` (swept, §2). Nothing anywhere renews a lease afterward. The comment at scheduler.go:163 even states the contract — "a claimed run must heartbeat before recovery.dead_after or the recovery ladder reclaims it" — but no code implements the heartbeat.
2. **`SetLeaseTx` is unfenced.** `internal/run/run.go:441-470`: `UPDATE runs SET lease_holder=?, lease_deadline_ts=?, heartbeat_event_seq=? WHERE run_id=?` — no generation guard, no holder guard, no state guard. Its doc (run.go:433-440) says the lease "carries no run_events append". The `heartbeat_event_seq` column exists (S02.2's "heartbeat cursor"; run.go:185) but the scheduler passes 0 and **nothing ever reads it for classification**.
3. **The shell wires no Units seam.** `internal/shell/shell.go:187-212` composes `recovery.New` without `Units`/`Harvest`, so both default to the B0 probes (`recovery.New`, recovery.go:104-109). `NoUnits.Probe` returns `UnitGone` for every run (`internal/recovery/seams.go:53-58`).
4. **Classification lets the lease outrank the cursor.** `internal/recovery/recovery.go:266-336` `reconcileRun`: `lastActivity` (the newest `run_events` row's wall-clock ts, by `event_seq` — lines 289, 349-369) is computed for every run, but is only consulted in the two `UnitActive` arms (ALIVE line 303, WEDGED line 309). Under `UnitGone` neither can fire, so the switch falls to `case status.State == UnitCorpseFailed, l.leaseDead(now, grace, r):` (lines 321-322) — **DEAD on a dead/zero lease alone, cursor ignored**. `leaseDead` (lines 342-347): zero deadline = dead; else `now > deadline + grace`. The DEAD arm's own comment quotes S02.5 — "unit gone/failed with nothing newer to harvest" — while checking only the engine-store harvest probe (`NoHarvest` at B0-B4-in-process: always empty), never the cursor. A checkpoint 14 s ago is "something newer" by any honest reading.
5. **⚙`recovery.heartbeat` has ZERO consumers.** Swept (§2): it exists in the registry (`internal/settings/index.go:78-84`, default 60 s, clamp 15 s–5 min, "How often a live run refreshes its lease heartbeat", Ratified G2 Def.2) and in the cross-check `recovery.dead_after >= 2x recovery.heartbeat` (index.go:920-925). No runtime code reads it.
6. **Parked runs are never scanned.** `recovery.go:197`: the pass scans `StateClaimed, StateRunning, StateDraining` only — so open-ask pauses (intake parks at every card, `internal/intake/pipeline.go:878-901` `issueCard`) are safe indefinitely. **The kill window is the post-answer RUNNING advance**: `internal/intake/answer.go:90-127` (`closeAndResume` un-parks parked→running, then `advanceLoaded` drives in-process planner/critic engine calls) and `answer.go:497-535` (`approve`'s own un-park). Any advance later than claim+`dead_after` is at most one sweep (⚙`recovery.sweep_interval`, 5 min) from death.
7. **Every advance already leaves cursor evidence.** `run.Store.TransitionTx` (run.go:372-431) appends a `run.state` event **in the same tx** as every FSM edge — including both un-park edges — and parked→running bumps the generation (run.go:111-113, 380-383). `gates.Checkpoints.WriteTx` (checkpoints.go:126-196) appends a `run.checkpoint` event in the same tx as the row; its sole production caller is `internal/adapters/driver.go:343` — the engine driver the stage layer composes for ALL in-process engine calls (`internal/stage/skeleton.go:112-126`), and the driver's pump appends engine events at the running generation throughout a live call (driver.go:97-98). Intake appends `intake.state` events per step (`appendStateTx`). **So the event cursor is fresh at the exact moments the ladder kills; only the lease is stale.**
8. **The watchdog reads the OTHER evidence and would disagree.** `internal/watchdog/sweep.go:58-109` `sweepSilence`: newest-event recency vs ⚙`watchdog.silence_budget.<run_type>` seeded from `recovery.dead_after`; watch set = running/draining unless waiting-on-human, parked only with a PAST resume-time (sweep.go:20-33). Its response is Tier-2 `parkAndFlag` (`internal/watchdog/tier2.go:55-73`) — park + card, **never kill** (§31 importwall AST-proves no kill path exists). Today the watchdog would call the incident run healthy (event 14 s old) in the same minute the ladder reaps it: the two liveness observers read different evidence and disagree fatally.

**The FSM/spec frame this must live in (S02.5, S02.3, S14.4 — read in full):**
- The ladder (S02.5 step 2): **ALIVE** = unit active, cursor advancing → reattach. **WEDGED** = unit active, cursor stalled past ⚙`recovery.dead_after` → **pause-and-flag, never auto-kill** (D1.3). **FINISHED-DURING-OUTAGE** → harvest. **DEAD** = "unit gone/failed **with nothing newer to harvest**" → crashed + fork-from-last-checkpoint at generation+1.
- S02.3: stalled/wedged is **derived, never stored** — "unit active but the event cursor has not advanced past ⚙recovery.dead_after". The registered semantics of `dead_after` are cursor-silence: the S18 row's own help text reads "How long a run's **event cursor may sit still** before the recovery pass treats the run as stalled" (index.go:86-91).
- S02.5 step 4 + S02.2: the lease block is `{holder, wall-clock deadline, heartbeat cursor}`; deadlines evaluated suspend-aware with ⚙`recovery.wake_grace`; the G2 Def.2 set ratifies **heartbeat 60 s** alongside dead-after 5 min, with the registry clamp `dead_after ≥ 2× heartbeat` — the shape only makes sense if something renews at heartbeat cadence and expiry tolerates at least one missed beat.
- S10.7: the scheduler CAS-claims ("claimed, lease held"). S10.8: loop and silence detection are **scheduling actions, not kills** — pause-and-flag, NEVER auto-kill.
- S14.4: the watchdog's silence check is the event log; Tier-2 is always a pause. **Watchdog and ladder both observe liveness; they must agree because they must read the same evidence (the cursor). Only the ladder reaps, and only when there is no holder and nothing newer.**

**Does the spec assign the lease-renewal duty to anyone today? NO — that is the gap.** S02.2 defines the lease block (including the heartbeat cursor), G2 Def.2 ratifies the cadence, the S18 help text says "a live run refreshes its lease heartbeat", and scheduler.go:163's comment presumes the duty — but no S-section names the renewing component, and no code performs it. This is a spec *gap*, not a spec conflict: no ratified sentence contradicts assigning the duty to the platform component actively driving the run (this brief's R3). No S00.9 amendment is required; record the assignment at the phase gate.

**Is there a WEDGED classification for the genuinely-stuck case, distinct from DEAD? YES.** S02.5's WEDGED is exactly "the holder is alive but progress stopped" — the ladder pauses-and-flags (once per stall episode, `flagWedged` recovery.go:373-390, `run.wedged` observation event, no state change) and never kills; tombstoning a repeat offender is an operator/policy act. Its in-process translation (no unit to be "active"): **live lease (a heartbeating holder) + stale cursor = WEDGED-analog**, and the watchdog's silence rule independently parks-and-cards the same condition. DEAD is reserved for the holderless: lease dead/absent AND cursor stale AND nothing to harvest — or a failed unit corpse (the body outranks; note P-T07-2's wrapper exit-record append means a corpse case can show *recent* cursor activity that is death evidence, not life — which is why cursor freshness gates DEAD only under `UnitGone`/`UnitUnknown`).

## 2. Consumer inventory (swept this session)

**Lease columns (`runs.lease_holder`, `runs.lease_deadline_ts`, `runs.heartbeat_event_seq`):**
- Writer: `run.Store.SetLeaseTx` (run.go:441-470) — **sole writer**; sole production caller `scheduler.claimOne` (scheduler.go:537, heartbeatSeq always 0).
- Readers: `recovery.Ladder.leaseDead` (recovery.go:342-347) — **the sole decision consumer**; the `run.Store` row scan (run.go:570-620) materializes them onto `run.Run`.
- `queue.lease_deadline_ts`: written at claim (scheduler.go:519) and **read by nothing** — write-only today. Out of scope; do not touch.

**⚙`recovery.dead_after` consumers:** recovery ladder (recovery.go:37, classification threshold), scheduler (scheduler.go:165, the claim-lease TTL — documented CONVENTIONS §11 cross-cutting consumption), watchdog (watchdog.go:104, silence-budget seed per S14.4). **⚙`recovery.heartbeat` consumers: none** (registry definition + clamp cross-check only). **⚙`recovery.wake_grace` consumers:** recovery ladder only (recovery.go:38, 159-191).

**parked→running / to-running transition sites (the option-(a) inventory):** intake/answer.go:113 (`closeAndResume`), intake/answer.go:527 (`approve`); stage/answer.go:120, 169; stage/resume.go:85; stage/onboard.go:139, 222; stage/compose.go:193; stage/skeleton.go:311, 419, 606; adapters/driver.go:100 (`Drive`, claimed→running), 143 (`DriveResume`). All ride `run.Store.Transition{,Tx}` — **one chokepoint**. Park sites (for completeness): intake/pipeline.go:893, stage/onboard.go:165, stage/skeleton.go:753, stage/pause.go:76, watchdog/tier2.go:64, scheduler.go:272.

**Cursor-advance chokepoints:** `run.Store.TransitionTx` (every FSM edge), `gates.Checkpoints.WriteTx` (every paid call, via adapters/driver.go:343), the driver's engine-event pump, intake `appendStateTx` — all funnel through `eventlog.Log.AppendTx` (generation-fenced).

## 3. Requirements (numbered, each with its S-ref)

- **R1 — Evidence outranks the timer under an unknowable body.** [S02.5 step 2: ALIVE = "cursor advancing", DEAD = "nothing newer to harvest"; S02.3 derived-stalled; S18 `dead_after` help text] When the unit probe reports `UnitGone` or `UnitUnknown`, a non-terminal scanned run whose last `run_events` append is within `dead_after` (+ the pass's wake grace) of the pass MUST NOT be classified DEAD, regardless of lease state. It classifies ALIVE (counted in `Report.Alive`).
- **R2 — DEAD becomes a conjunction (in-process form).** [S02.5 step 2 DEAD; CONVENTIONS §8 "a live lease is always honored" — preserved and strengthened] Under `UnitGone`/`UnitUnknown`, DEAD requires: lease dead or absent (suspend-aware per S02.5 step 4, `wake_grace` applied exactly as today) AND cursor stale past `dead_after`(+grace) AND no terminal harvest. `UnitCorpseFailed` remains sufficient for DEAD regardless of cursor recency (the body is conclusive; P-T07-2's wrapper exit-record append is itself recent-but-dead cursor activity). The existing bound decision (crash + fork/tombstone/finalize in ONE tx, recovery.go:461-524) is untouched.
- **R3 — The holder renews the lease at ⚙`recovery.heartbeat` cadence.** [S02.2 lease block; G2 Def.2 ratified set + the `dead_after ≥ 2× heartbeat` clamp; S18 help "how often a live run refreshes its lease heartbeat"; scheduler.go:163's stated contract] While the control plane is actively driving a run's in-process advance — the scheduler dispatch leg (scheduler.go:553+), the intake post-answer advance (`Answer`/`advanceLoaded`), the stage answer/resume drains, and the adapter driver's pump — the platform renews `runs.lease_deadline_ts = now + dead_after` every `recovery.heartbeat`, starting immediately on taking the run (so the un-park instant is covered), stopping when the advance ends (park, terminal, or error). Both ⚙ read by dotted key at use (CONVENTIONS §2), never cached across a live-apply.
- **R4 — Renewal is fenced.** [S02.5 step 4 generation fencing, P-T02-3] Lease renewal carries the holder's generation and applies only `WHERE generation = ?` and state ∈ {claimed, running, draining}; a stale-generation or post-park renewal is a silent no-op (never an error that kills the healthy path). Implemented as a NEW fenced method (e.g. `RenewLeaseTx`) — `SetLeaseTx`'s claim-time semantics stay byte-identical.
- **R5 — Resume leaves no expired-lease window.** [S02.3 resume edge; the root-caused kill window] Every parked→running resume results in a run whose lease is live before the next sweep can observe it: either renewed in the resume transaction itself or by the holder's immediate first heartbeat (R3 "starting immediately"). Whichever mechanic the coordinator dispositions (OQ1), this property is non-negotiable and is pinned by test T1/T6.
- **R6 — The in-process WEDGED analog never dies.** [S02.5 WEDGED; D1.3; S10.8] A run with a LIVE lease and a stale cursor under `UnitGone`/`UnitUnknown` is never DEAD. Disposition of whether the ladder additionally flags it `run.wedged` (once per stall episode, exactly the existing `flagWedged` discipline) is OQ2; killing it is forbidden under every disposition. The watchdog's silence rule (park + card) remains the containment owner and is NOT modified (§31 importwall).
- **R7 — Watchdog/ladder evidence coherence.** [S14.4; S02.5] After this packet, both observers read the same cursor: on a healthy world, no run within its silence budget is ladder-reapable. Achieved entirely on the recovery side; zero watchdog changes.
- **R8 — No new ⚙ keys; no migration.** [S18; CONVENTIONS §2/§6] The packet consumes only ratified keys, all verified present in `internal/settings/index.go`: `recovery.heartbeat` (line 79 — this packet is its FIRST consumer; it is the natural renewal cadence, not a dead key), `recovery.dead_after` (line 86), `recovery.wake_grace` (line 93), plus the ladder's existing `recovery.sweep_interval`/`max_attempts`/`stale_finalize`. The schema already holds every needed column (`lease_holder`, `lease_deadline_ts`, `heartbeat_event_seq`). **If any schema change appears necessary during implementation: STOP and flag the coordinator — do not migrate.**
- **R9 — Claim-time behavior unchanged.** [S10.7; CONVENTIONS §11] `claimOne`'s initial lease (deadline = now + `dead_after`, heartbeatSeq 0) stays as-is: the first heartbeat must simply arrive within `dead_after`. The write-only `queue.lease_deadline_ts` is not touched.
- **R10 — Parked stays unscanned; scanned-state set unchanged.** [S02.5 "per non-terminal run"; recovery.go:197] The pass continues to scan exactly {claimed, running, draining}; open-ask pauses remain structurally safe (pinned by T6). Draining runs get heartbeat coverage like running ones (S10.8's drain grace of 15 min exceeds `dead_after` — a draining holder must beat).
- **R11 — CONVENTIONS records the settled reading.** [process] The executor appends a P3-RW-5 conventions entry: the DEAD conjunction (refining §8's compressed "or a dead/absent lease" sentence — the live-lease-honored half is unchanged), the renewal duty assignment (holder = the driving component), and the fenced-renewal rule.

## 4. Seams to respect

- `recovery.UnitLiveness` / `recovery.HarvestSource` stay the B0 no-ops at v0 (seams.go:49-58, 82-87). **Do NOT wire a fake in-process Units seam that reports advance-goroutines as `UnitActive`** — an in-memory holder map is supervisor memory, and S02.5's kubelet rule ("re-list actual state, never trust supervisor memory") forbids exactly that; the DB lease + cursor ARE the re-listable actuals. (Rejected alternative, recorded.)
- `run.Store` transition discipline (CONVENTIONS §8): every `runs.state` change through `Transition{,Tx}` in ONE WriteTx with its event append; renewal is a lease-column update, NOT a state change, and appends no event (`SetLeaseTx` doc, run.go:433-440 — keep that property for `RenewLeaseTx`; the cursor must reflect progress, not heartbeats).
- The watchdog importwall (§31): `internal/watchdog` is untouched; `internal/recovery` must not import it, nor it recovery.
- Scheduler ↔ engine seam (§11): the scheduler never imports adapters; any shared lease-keeper helper lives in `internal/run` (imported by scheduler, intake, stage, adapters already).
- Single-writer + `BEGIN IMMEDIATE` posture (S02.1): heartbeat writes are tiny single-row UPDATEs through `storage.DB.WriteTx`; at 60 s cadence per active advance this is noise, but never hold a WriteTx open across a tick.
- Eventlog generation fencing is standing behavior (recovery.go:194-196); renewal fencing (R4) complements it — it does not replace it.

## 5. ⚙ settings (registry names, all pre-existing — verified `internal/settings/index.go`)

| key | role in this packet | line |
|---|---|---|
| `recovery.heartbeat` | renewal cadence (FIRST consumer) | 79 |
| `recovery.dead_after` | renewal deadline extent; cursor-staleness threshold; claim TTL (unchanged) | 86 |
| `recovery.wake_grace` | suspend-aware expiry evaluation (unchanged) | 93 |
| `recovery.sweep_interval`, `recovery.max_attempts`, `recovery.stale_finalize` | existing ladder consumption, unchanged | 100+ |

No new keys. No clamp changes. The `dead_after ≥ 2× heartbeat` cross-check (index.go:920-925) becomes load-bearing: a holder survives at least one missed beat.

## 6. Files expected to change (executor; exact set narrows per OQ1 disposition)

- `internal/recovery/recovery.go` — classification (R1/R2/R6); comment updates quoting S02.5 honestly.
- `internal/run/run.go` — fenced `RenewLeaseTx` (R4); the lease-keeper helper (start/stop heartbeat around an advance) if OQ4 lands there.
- `internal/scheduler/scheduler.go` — dispatch-leg heartbeat hold (R3).
- `internal/intake/answer.go` (+ possibly `pipeline.go`) — heartbeat hold across the post-answer advance (R3/R5).
- `internal/stage/answer.go`, `internal/stage/resume.go` (+ skeleton drive paths as needed) — heartbeat hold across answer/resume drains (R3).
- `internal/adapters/driver.go` — heartbeat hold across `Drive`/`DriveResume`/pump (R3); under a checkpoint-renewal disposition, `internal/gates/checkpoints.go` instead.
- `P3/CONVENTIONS.md` — the R11 entry.
- Colocated `_test.go` files for §8's specs.

No migrations directory changes. No `internal/watchdog` changes. No `Spec/` changes.

## 7. Adopted components

None. stdlib only (CONVENTIONS §2); no `components.lock` motion.

## 8. Acceptance-test SPECIFICATIONS (written here per the standing PROCESS EXCEPTION — the grounding round commits NO tests and touches NO Go file; the executor materializes these)

All in `internal/recovery` unless noted; fake clock through the ladder's `Now` seam; settings via the test registry; DB per the package's existing test harness. "Grace-neutral" setup = run one throwaway pass first and keep clock deltas < 2× `sweep_interval` so `wake_grace` is not applied (recovery.go:184-189).

- **T1 `TestReconcile_AdvanceOutlivesClaimLease_Regression` — reproduces the death.** Setup: run claimed with lease deadline D = claim+300 s (heartbeatSeq 0, exactly `claimOne`'s write); transition parked→running (or claimed→running); grace-neutral; advance clock to D+11 s; append a `run.checkpoint` event at D−3 s (i.e. 14 s before the pass). Pass. Assert: run state still `running`; `Report.Alive == 1`, `Forked == 0`; no `run.state`→crashed event; no successor row. (Without the fix this exact setup crashes+forks — the incident.)
- **T2 `TestReconcile_StillReapsTheDead`.** Setup: running run, `UnitGone`, no harvest; lease deadline expired > grace; newest event older than `dead_after`+grace. Pass (grace-neutral). Assert: crashed + forked successor `<id>.g1` at generation+1 committed in one pass; `Report.Forked == 1`. Variant (b): `lease_deadline_ts` NULL/zero with the same stale cursor → same outcome (a leaseless corpse row is still reaped).
- **T3 `TestReconcile_ReapBound`.** Same as T2 but sweep the clock: assert the run is NOT dead on a pass at `lastEvent + dead_after − ε` and IS dead on the first pass after `max(leaseDeadline, lastEvent + dead_after)` + grace — the S02.5 bound, explicit.
- **T4 `TestRenewLeaseTx_Fenced`** (`internal/run`). Renewal with the current generation + running state extends the deadline and (per OQ3) stamps `heartbeat_event_seq`; renewal with a stale generation is a no-op (row unchanged, nil error); renewal on a parked/terminal run is a no-op; `SetLeaseTx` claim semantics unchanged.
- **T5 `TestLeaseKeeper_HeartbeatCadence`** (home = wherever OQ4 lands). Fake clock; hold a run; assert a renewal lands immediately on start, then per `recovery.heartbeat` tick, each setting deadline = now+`dead_after`; assert renewals stop after release; assert a mid-hold generation bump (simulated fork/park) stops effective renewal (T4's fence observed end-to-end).
- **T6 `TestReconcile_HumanPacedInterviewSurvivesIndefinitely`.** Simulate ≥ 3 full cycles over hours of fake clock: claim → running (holder heartbeating; include one single advance segment lasting > `dead_after` with NO event appends — only heartbeats) → park at an ask (heartbeat released; lease left to expire while parked) → hours pass → sweep repeatedly (assert parked run untouched — the R10 pin) → un-park → advance → checkpoint → park… Assert: across every sweep the run never leaves {claimed, running, parked} except by its own completion; zero crash/fork events.
- **T7 `TestReconcile_WakeGraceStillHolds`.** Existing suspend-aware behavior preserved: first pass of a process (or clock-jump > 2× `sweep_interval`) applies `wake_grace`; a run whose lease expired within the grace and whose cursor is stale within `dead_after`+grace is `Pending`, not DEAD.
- **T8 `TestReconcile_CorpseFailedOutranksFreshCursor`.** `UnitCorpseFailed` with a recent event append (the wrapper exit record, P-T07-2) → still DEAD (crash+bound). Guards against over-rotating R1 into "any fresh event blocks DEAD".
- **T9 `TestReconcile_WedgedInProcess`** (shape per OQ2 disposition). Live lease (freshly renewed), cursor stale past `dead_after`, `UnitGone`. Assert: never crashed; per disposition either `run.wedged` appended once per episode + `Report.Wedged` counted, or `Report.Pending` counted — the not-DEAD half is binding either way.

## 9. Acceptance checklist (the headline, decomposed — the evaluation rubric)

Headline: **a human-paced interview (arbitrary pauses at ask cards, multi-minute in-process advances) survives indefinitely under the sweep on a healthy world, while a genuinely dead run (no lease, no activity, no body) is still reaped within the S02.5 bounds.**

1. [ ] Arbitrary ask-card pauses: parked runs unscanned (T6, R10) — hours/days at a card cause nothing.
2. [ ] The un-park instant is safe: no post-resume window where an expired claim lease can meet a sweep (T1, T6; R5).
3. [ ] Multi-minute in-process advances are safe, including a single advance segment > `dead_after` with zero event appends (T6's long segment; R3).
4. [ ] The incident replay passes: lease-expired-by-11 s + checkpoint-14 s-ago = ALIVE (T1; R1).
5. [ ] A genuinely dead run (lease dead/absent + cursor stale + no body/harvest) is crashed and forked/bounded on the first sweep past `max(lease, lastEvent+dead_after)`+grace (T2, T3; R2).
6. [ ] A failed unit corpse is still DEAD regardless of cursor recency (T8; R2).
7. [ ] Suspend-awareness unchanged: wake grace still protects across naps (T7).
8. [ ] Renewal is generation-fenced and state-guarded; a zombie holder cannot keep a superseded run alive (T4, T5; R4).
9. [ ] Live-lease + stale-cursor is never DEAD (T9; R6); the watchdog remains the containment owner, untouched (R7).
10. [ ] No new ⚙, no migration, no new dependency; `recovery.heartbeat` consumed by dotted key (R8).
11. [ ] `go build ./... && go test ./...` green at the implementation commit; the concurrent light-path packet's battery never sees red from this packet.
12. [ ] CONVENTIONS entry recorded (R11).

## 10. Binding CONVENTIONS constraints

- §2: stdlib-first (no new modules); ⚙ by dotted key at use, never a hardcoded constant; `gofmt`/`go vet` clean; doc comments cite spec sections.
- §3: stdlib `testing` only; colocated `_test.go`; `t.TempDir()` discipline. **This packet does NOT use the Amendment-A red-tests carve-out** — see §12.
- §5: explicit pathspec staging; packet sessions never push; subject format per the coordinator's exact message.
- §8: transition discipline (one WriteTx, event append same tx); "a live lease is always honored" (preserved); DEAD-needs-proof refined per R2/R11; crash+bound one tx; checkpoints only in running/draining.
- §11: `recovery.dead_after` as claim TTL is documented cross-cutting consumption — kept (R9).
- §31: watchdog untouched; its importwall and no-auto-kill invariants bound this packet from the outside.

## 11. Open questions — NOT resolved here; the coordinator dispositions each

- **OQ1 — the mechanics: (a) renew-at-activity-sites, (b) classification-side evidence, or (c) both.** Grounding's weighing:
  - **(a) alone** (renew at un-park + checkpoint append + state append + engine-event harvest): fixes the incident, keeps the lease meaningful. Cost: the discrete-site inventory is ~12 transition sites + 3 append families (§2); done at the chokepoints (`TransitionTx` to=running, `Checkpoints.WriteTx`, the driver pump, intake `appendStateTx`) it shrinks to ~4, but **misses any advance segment longer than `dead_after` with no appends** (a single long engine call whose stream goes quiet — the headline's "multi-minute advances" is not fully covered), and the cost of missing one site is that path's advances silently regain the death window. Site-renewal also conflates *progress* with *holder liveness*.
  - **(b) alone** (under `UnitGone`/`UnitUnknown`, cursor advance within `dead_after` = ALIVE): a one-file fix that matches S02.5's DEAD text ("nothing newer") and the S18 `dead_after` semantics; fixes the incident. Cost: the lease becomes permanently dead on every long run (never renewed → expiry by construction), i.e. a schema field and a ratified G2 Def.2 mechanism become decorative, scheduler.go:163's stated contract stays false, and the same long-quiet-advance segment still dies (no appends AND dead lease). False-ALIVE risk is nil-to-acceptable: a chatty wedge lives past the ladder — which is CORRECT (D1.3/S10.8: a wedge is pause-and-flag, and the watchdog's loop/ping-pong/error-loop/silence rules park it); a silent wedge whose holder died goes cursor-stale and is reaped.
  - **(c) — grounding's lean:** the semantically honest split. Lease = **holder liveness**, renewed at ⚙`recovery.heartbeat` cadence by whoever drives the advance (covers quiet long calls; R3/R5); cursor = **progress evidence**, honored by classification under an unknowable body (covers any renewal-gap seam; R1/R2). Each mechanism alone fixes the incident; together each backstops the other's blind spot, and both ratified artifacts (`recovery.heartbeat`, the S02.2 heartbeat cursor) stop being dead letters.
- **OQ2 — the ladder's WEDGED analog.** Should live-lease + stale-cursor + `UnitGone` be flagged `run.wedged` (once per episode, `Report.Wedged`), or left as `Pending` with the watchdog's silence rule as the sole flagger? Lean: flag it — it is S02.5's WEDGED semantics translated (holder alive, progress stopped), costs ~10 lines reusing `flagWedged`, and the ladder's `run.wedged` (observation event) vs the watchdog's `watchdog.flagged` (park + card) are distinct, non-conflicting duties. Against: dual flags for one condition is noise the alert-dedup discipline (S14.4) must absorb.
- **OQ3 — stamp the heartbeat cursor?** Should renewal write `heartbeat_event_seq` = the run's newest `event_seq` (S02.2: "last event_seq the holder asserted")? Lean: yes — cheap, makes the lease row self-describing for `tools/dbpeek` forensics, and gives the column its first honest writer; classification continues to read `run_events` directly either way.
- **OQ4 — where the heartbeat holder lives.** One `run.LeaseKeeper` helper (start/stop, owns the ticker goroutine, calls `RenewLeaseTx`) used by the four driving layers (scheduler dispatch, intake advance, stage drains, adapter pump) — vs per-layer ad-hoc tickers. Lean: the single helper in `internal/run` (already imported by all four; no new import edges), so the fence and cadence logic exist once.

## 12. Process notes (coordinator-facing)

- **PROCESS EXCEPTION (standing, coordinator-logged):** this grounding round commits ONLY this brief — no failing tests, no Go files touched; a light-path packet runs concurrently and the battery must stay green. The executor materializes §8 red-first per its own packet instructions.
- Commit: `P3-RW-5 grounding: recovery-lease liveness brief (S02.5, S02.3, S10.7)` — explicit pathspec, no push.
- **Spec conflicts found: none.** The drift is code-vs-spec (the DEAD arm ignores "nothing newer to harvest") and one compressed CONVENTIONS §8 sentence; S02.5, S02.3, S10.7/S10.8, S14.4 and the S18 rows are mutually consistent and jointly determine the fix's shape. The lease-renewal duty is a spec *gap* (unassigned), settled here as R3 and recorded per R11.
- Incident facts (11 s past lease, checkpoint 14 s prior) are the coordinator-ratified root-cause record of this session; every mechanism link was independently re-verified against code this session at the file:line cites in §1.
