# Spike 1 — serialize-by-deny reconfirm ≡ parallel-gate fallback-rate measurement

- **Packet:** P3-B1-4 (B1 spike battery), 2026-07-20. Measurement, not feature-building.
- **Spec markers:** `TBD-P3(reconfirm serialize-by-deny on the default worker model)` [S02.8 carry-forward caveat] ≡ `TBD-BRINGUP(parallel-gate fallback rate on the default worker model)` [S03.4; S03 Deferred]. S19.6 sequences these as **one** task.
- **Anchors:** S02.8 (artifact-coordination enforcement = serialize-by-deny; carry-forward caveat), S03.4 (defer exit-park; single-tool-call trap; fallback detection; serialize-by-deny), SPIKE P2-S4 (headline: viable on `claude-haiku-4-5`, ~+1 turn, ~$0.013 cheap lane).
- **Engine:** installed `claude` 2.1.215 vs lock pin 2.1.214 (delta already recorded for the B1 gate as an operator S03.3 bump decision, B1-1). Discipline: assert behavior, never docs (S03.3 rule 4).

## Pre-registered expectation (from the spec rows)

E1 — **single gated tool-call turn → clean exit-park.** The PreToolUse hook returns `defer`; the headless process exits `stop_reason:"tool_deferred"` carrying a `deferred_tool_use{id,name,input}` object; the shipped adapter builds the ask record from the exit JSON alone; the run maps to `OutcomeParked` (S03.4; S02.3 `parked`). Poll-resume while parked is genuinely free ($0, zero tokens) [S03.4].

E2 — **parallel gated tool-call turn → silent defer fallback, detected.** When one turn contains >1 tool call, `defer` is **not** honored and the engine falls back to normal permission flow **silently** (no stderr warning in `-p` json mode). The shipped adapter detects it: >1 PreToolUse fire for one turn + no `tool_deferred` exit ⇒ fallback (`GateFallback` true; `fires > 1`) [S03.4; P-T02-4].

E3 — **serialize-by-deny converts the fallback into ~one extra cheap turn.** With ⚙ `adapter.parallel_gate_fallback = serialize-by-deny` (default), the hook denies every non-first call of the parallel turn with reason "re-issue the gated call alone"; the engine **re-issues the gated call alone**, which then parks cleanly with a *faithful single-call* ask record — ~+1 model turn, no held process (S02.8; P2-S4). Distinct value over always-defer: faithful ask-record fidelity (never re-derives the siblings on resume).

E4 — **fallback rate.** Spec baseline ≈ **20% of gated turns on the default worker model** (8.5% on opus-4-8) [S03.4]. Recorded here as a point estimate under a parallel-inducing prompt (model-dependent; small-sample; NOT the natural mixed-workload 20%).

**PASS** = E1 (clean defer-park) ∧ E2 (fallback detected on parallel) ∧ E3 (serialize-by-deny yields a clean single-call park at ~+1 turn) hold live through the shipped adapter on the reconfirm model. E4 is recorded as context.

## Model designation reading (why PROVISIONAL)

The caveat says serialize-by-deny "MUST be reconfirmed on the default worker model — the model that actually drives the ~20% parallel-batch rate." **The spec does not pin "the default worker model" to any concrete model in any draft** (grep S00–S19): the default worker is fixed by **S08** (worker registry + composer + routing), which is **B3 — not yet built**. Per the packet rule, this reconfirm uses **the B1-1 lane's cheapest model, `--model haiku`** (the model the shipped tier-L live smoke uses; resolves to the `claude-haiku-4-5` family at 2.1.215). This is ≈ the exact model class P2-S4 already demonstrated on — so the reconfirm's added value is that it exercises the **shipped adapter path** (`sinet engine-hook` + fires.log turn-window heuristic + outcome assembly) live on the **currently installed engine**, not the research spike's ad-hoc harness. The substance of the caveat (the model that drives the natural ~20% rate) is **NOT** resolved here. **VERDICT IS THEREFORE PROVISIONAL** and the TBD stays OPEN, to be re-run when S08 fixes the default worker (re-entry: S08/B3).

## Method

Drive the **shipped adapter** `internal/adapters/claudecli` (Start → Events → Wait) — the packet's "shipped adapter path where the design permits." Gate a tool (`GatedTools`), wire the real gate hook `sinet engine-hook --ctl <dir>` via `Adapter.HookCmd` → the scratch-built `sinet` binary; ⚙ `adapter.parallel_gate_fallback = serialize-by-deny` (registry default). Induce single vs parallel gated turns via prompts; N trials for the rate. Separately capture raw stream-json (direct exec of the adapter's own lowered argv + `--include-hook-events`) for the defer/parallel fixture re-record (B1-1 flag). Cheapest model (`haiku`); report summed `total_cost_usd`.

## Observations (live, 2026-07-20, shipped adapter through the real `sinet engine-hook`)

Reconfirm model: `--model haiku` → **`claude-haiku-4-5-20251001`** (init `apiKeySource:none` = subscription auth, S03.2). This is the `claude-haiku-4-5` family — i.e. ≈ the exact model class P2-S4 demonstrated on. Harness: `internal/adapters/claudecli/spike_test.go` (live-gated `SINET_B1_4=1`), driving `Adapter.Start/Events/Wait`.

**E1 — single gated call → clean defer-park. PASS.** One gated `Bash` call → `OutcomeParked`, `stop_reason=terminal_reason=tool_deferred`, `deferred_tool_use={Bash,"echo HELLO"}`; the ask is built from the exit JSON alone; `num_turns=1`; cost **$0.010879**. No fallback (`GateFallback=false`).

**E2 — the silent-fallback trap did NOT reproduce on 2.1.215+haiku (drift finding).** In 6/6 trials the model emitted a *genuine* parallel gated turn — **two `Read` tool_use blocks in one assistant message** (raw stream confirms). But the engine **honors the first `defer` even on a parallel turn**: it exited `stop_reason=tool_deferred` carrying call-1 as the deferred_tool_use, while the serialize-by-deny **deny** of call-2 came back as an error tool_result ("re-issue the gated call alone"). So the S03.4 trap ("defer honored only on single-tool-call turns; parallel turns fall back to normal permission flow **silently** → completion without a park, detected by >1 fire + no `tool_deferred` exit") **did not occur**: every parallel gated turn still parked via `tool_deferred`. Consequently the adapter's `GateFallback` detector (`fires>1` in the *completed* branch) **stayed dormant** — 0/6 — because we always reached the *parked* branch. The observed parallel-batch fallback-to-completion rate on 2.1.215+haiku was **0%**, not the S03.4 ~20% (measured on an earlier engine family). Detection remains a correct **defense-in-depth** net for engines/models that DO silently fall back.

**E3 — serialize-by-deny yields a faithful single-call park at ~+1 turn. PASS.** All 6/6 parallel trials → `OutcomeParked` with a **faithful single-call ask** (only call-1's `Read`, e.g. `{file_path:.../a.txt}`); the sibling was denied and re-issues on the model's next turn (the serialize intent). `num_turns=2` (the ~+1 turn vs the single-call path); ~**$0.0177**/trial (SUMMARY total $0.106338). This is exactly the S02.8/P2-S4 desired end state — no held process, faithful ask-record fidelity.

**Defer→resume→complete round-trip (live).** Park $0.011069 → answer staged into the ctl dir → the re-fired PreToolUse returned `allow(updatedInput)` → `echo APPROVED-42` ran → `OutcomeCompleted`, resume **$0.002394**, `num_turns=1`. The ask round-trips through the shipped adapter. (The $0 *poll*-resume-without-answer is covered by unit tests + G1-S2; not separately re-billed here.)

**Forward-tolerance (S03.1 MUST) held against real drift.** 2.1.215 emits **new** stream event types `command_lifecycle` and `attachment` (+ `system` subtypes `status`/`thinking_tokens`); the shipped parser logs-and-skips them, never fatal. The defer result envelope also carries new fields (`permission_denials`, `iterations`, `context_management`, `fast_mode_state`, `modelUsage.contextWindow`) — parsed tolerantly.

**Fixture re-record (B1-1 flag).** `testdata/defer.jsonl` re-recorded from this live 2.1.215 defer (faithful drift fields); the 4 asserting literals updated (ask id `toolu_01JwN5WHtxKWbHoLyfaRdifh`, session `44baeb68-…`); claudecli package green. `budget.jsonl` re-record **deferred** — reliably inducing a same-turn budget-preempt-*after*-gate-fire (died-at-gate, S03.4 M2/M3) is a targeted probe better scheduled with the S10 ceiling work; the synthetic shape stays unit-tested. `parallel.jsonl` **kept** as the completed-silent-fallback DETECTION fixture (defense-in-depth), now that live parallel turns park instead.

## Verdict

**PASS — PROVISIONAL.** E1 (clean defer-park) and E3 (serialize-by-deny → faithful single-call park at ~+1 turn) reconfirmed **live through the shipped adapter path** (`sinet engine-hook` + fires.log turn-window heuristic + `assembleOutcome`) on the reconfirm model. The turn-window heuristic behaved correctly (defer first fire, deny sibling within the window). **PROVISIONAL** because the spec does not pin "the default worker model" until **S08 (B3)**; this used `haiku` (B1-1 cheapest ≈ P2-S4's model), so the caveat's substance (the model that drives the *natural* parallel-batch rate) is not resolved. Re-run via the committed harness (`SINET_B1_4=1`) once S08 fixes the default worker.

## Spec markers resolved / escalated

- `TBD-P3(reconfirm serialize-by-deny on the default worker model)` ≡ `TBD-BRINGUP(parallel-gate fallback rate on the default worker model)` — **RECONFIRMED PROVISIONALLY; stays OPEN** for the S08-fixed default worker. The shipped serialize-by-deny is validated live on haiku.
- **Escalated to the B1 gate + the S2.8/S14 engine-behavior canary suite (REPORT, not patched):** on installed **2.1.215 + haiku** the engine **honors the first `defer` on parallel gated turns**, so the S03.4 "~20% silent fallback" trap does **not** reproduce and the adapter's `GateFallback` completed-branch detector is dormant here. This is favorable (parallel gated turns park cleanly) but is a **behavior drift from the pinned-family measurement** — it belongs in the per-pin canary (S09.9/S05.7/S03.3 re-check) and should be re-measured at the pin bump 2.1.214→2.1.215 the B1 gate already faces. No feature change made (measurement packet).

## Spend

Live, api-equivalent reported `total_cost_usd`, summed: precond/auth+model $0.018469; E1 $0.010879; parallel 6× $0.106338; resume (park $0.011069 + answer $0.002394); raw-capture (defer $0.010947 + parallel $0.017726). **Spike-1 total ≈ $0.1778.**
