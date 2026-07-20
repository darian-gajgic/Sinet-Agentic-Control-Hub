# Serialize-by-deny reconfirm on the S08-chosen default worker (spike-1 re-run)

- **Packet:** P3-B3-3, 2026-07-21. The pre-registered PAID leg of the ratified B3 queue (queue row: "serialize-by-deny reconfirm on the chosen default worker → measurement file closes TBD-BRINGUP(parallel-gate fallback) or escalates").
- **Spec markers:** `TBD-P3(reconfirm serialize-by-deny on the default worker model)` [S02.8 carry-forward caveat] ≡ `TBD-BRINGUP(parallel-gate fallback rate on the default worker model)` [S03.4; S03 Deferred] — the ONE marker B1-4 left OPEN pending S08's model designation.
- **Prior run:** `P3/measurements/2026-07-20-serialize-by-deny-reconfirm.md` (P3-B1-4, PASS-PROVISIONAL): verdict provisional solely because "the spec does not pin 'the default worker model' until S08 (B3)" — the mechanics passed on `haiku` as a stand-in.
- **What changed since:** S08.8 selection landed (this packet). **The default worker model is now FIXED by the platform: `claude-haiku-4-5`** — the `worker.DefaultDutyMap()` execution seat (the v0 recommended platform-wide duty map, S06.10 shape; the model every no-fit generalist and default execution session runs on). The designation reading is recorded in `internal/worker/routing.go` (DefaultDutyMap doc) and flagged for the B3 gate.

## Pre-registered expectation (unchanged from spike-1; the designation is what this run settles)

- **E1** — single gated tool-call turn → clean defer exit-park (`OutcomeParked`, ask from exit JSON alone, no fallback).
- **E3** — parallel gated turn under ⚙ `adapter.parallel_gate_fallback = serialize-by-deny` (registry default) → faithful SINGLE-call park at ~+1 turn, no held process, sibling denied with "re-issue the gated call alone".
- **E2 context** — the S03.4 silent-fallback trap (~20% fallback-to-completion, measured on an earlier engine family): B1-4 found it does NOT reproduce on 2.1.215+haiku (the engine honors first-defer on parallel turns); expectation: the favorable behavior persists; the `GateFallback` completed-branch detector stays as defense-in-depth.
- **Round-trip** — park → answer staged in ctl dir → resume → `allow(updatedInput)` → completed.

**PASS** = E1 ∧ E3 on the S08-designated default worker model through the shipped adapter; the designation caveat thereby discharged. Stop rule: projected cost > ~$0.50 → STOP and report (projection from B1-4 same-family figures: ≈ $0.14 → proceed).

## Method

Exactly the committed B1-4 harness (`internal/adapters/claudecli/spike_test.go`, live-gated `SINET_B1_4=1`, real `sinet engine-hook` via `SINET_HOOK_BIN`), re-run at the established B1-4 scale (E1 single ×1; parallel ×6; defer→resume ×1) with the model now passed explicitly: `SINET_B1_4_MODEL=claude-haiku-4-5` (the harness gained the `spikeModel()` env parameter this packet; default remains `haiku`). Raw-fixture capture leg not re-run (fixtures re-recorded at B1-4; no flag outstanding).

**Engine:** installed `claude` **2.1.216** vs lock pin **2.1.215** — a pin↔installed delta REPORTED per CONVENTIONS §10 (the npm-global install moved; same situation B1-4 faced at 2.1.214→2.1.215). Discipline held: behavior asserted live, pin never silently retargeted; reconciling is an operator S03.3 deliberate-bump decision (flagged in the packet report — P-T14-1 note: the revalidation trigger fires against a worker set that is empty in production).

## Observations (live, 2026-07-21, shipped adapter through the real `sinet engine-hook`)

- **E1 — PASS.** Single gated `Bash` → `OutcomeParked`, no fallback, `num_turns=1`, ask faithful from exit JSON (`{"command":"echo HELLO",…}`), cost **$0.011056**.
- **E3 — PASS, 6/6.** Every parallel gated turn (2 `Read` calls in one assistant message; 2 PreToolUse fires in the final turn window per trial) → `OutcomeParked` with a FAITHFUL single-call ask (call-1's `Read {a.txt}` only), `num_turns=2` (the ~+1 serialize turn), per-trial cost $0.017425–$0.017875, total **$0.106060**.
- **E2 context — reconfirmed at 2.1.216.** Fallback-to-completion **0/6**; the engine honors first-defer on parallel gated turns (the favorable drift B1-4 recorded at 2.1.215 persists at 2.1.216). `GateFallback` detector: dormant, retained as defense-in-depth for engines/models that DO silently fall back.
- **Round-trip — PASS.** Park $0.011006 → staged answer → re-fired PreToolUse `allow(updatedInput)` → `echo APPROVED-42` ran → `OutcomeCompleted`, resume **$0.002258**, `num_turns=1`.
- **Forward-tolerance held (S03.1 MUST):** 2.1.216 still emits `command_lifecycle`/`attachment` stream types; the shipped parser logs-and-skips, never fatal.

## Verdict

**PASS — the provisional qualifier is DISCHARGED.** E1 and E3 hold live through the shipped adapter path on **the S08-designated default worker model** (`claude-haiku-4-5`, the DefaultDutyMap execution seat — no longer a stand-in: the designation is platform data as of P3-B3-3). Serialize-by-deny remains the validated ⚙ default; ask-record fidelity and the ~+1-turn economics match both the P2-S4 spike and the B1-4 run.

## Spec markers resolved / escalated

- `TBD-P3(reconfirm serialize-by-deny on the default worker model)` ≡ `TBD-BRINGUP(parallel-gate fallback rate on the default worker model)` — **CLOSED.** The caveat's substance (the model that drives the natural parallel-batch rate) is resolved by designation + live reconfirm on that designation. Spec-text bookkeeping of the closed marker is the coordinator's/gate's (S00.9 mechanics; packets never edit `Spec/`).
- **Reported, not patched (B3 gate item):** installed engine 2.1.216 vs pin 2.1.215 — operator S03.3 bump decision pending; behavior at 2.1.216 matches the 2.1.215 measurements everywhere this harness touches (defer-park, first-defer-honored on parallel, resume, stream tolerance). Production worker set is empty, so the P-T14-1 mass-revalidation event a bump implies is again empty (the two test-fixture workers of the battery are not production rows).
- **Standing note carried forward:** the natural-workload ~20% parallel-batch figure of S03.4 was measured on an earlier engine family; on the current pin family parallel gated turns PARK CLEANLY, so no fallback-rate figure exists to re-measure — the per-pin canary (S2.8/S14, B5) re-checks at every future bump.

## Spend

Live, api-equivalent reported `total_cost_usd`, summed: E1 $0.011056 + parallel 6× $0.106060 + resume ($0.011006 + $0.002258) = **$0.130380** (projection $0.14; stop line $0.50 — not approached). Zero additional paid calls anywhere else in the packet (fake-engine batteries throughout).
