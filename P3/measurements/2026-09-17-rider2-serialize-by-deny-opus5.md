# Rider 2 — serialize-by-deny E3 leg on the claude-opus-5 executor (S02.8 carry-forward; B7) — 2026-09-17

**PAID LEG. Pre-registered BEFORE any paid call (CONVENTIONS §28). Executed by the COORDINATOR after rider 1, between packet P3-TQ-5's commit 1 and commit 2.**

## Why this runs

The 2026-09-17 gate record (`P3/gates/rework-sitting-gate.md` item B7 + §Answers) moves the seat that actually executes work on this host to **`claude-opus-5`**: K3 on `kimi-cli` is the configured first choice and K3 on `kimi` the second, but no Kimi credential is placed, so the third choice — `claude-opus-5` on the anthropic lane — is what runs. The parallel-gate fallback is a **per-pin canary**: measured on haiku 2026-07-20/21, re-confirmed on `claude-sonnet-5` 2026-07-22, and owed again on the seat that replaces it.

**Honest scope, stated up front:** the E3 leg is a **claude-CLI PreToolUse-hook mechanism**. The K3/`kimi-cli` executor has no such leg — its print-mode gating was spiked at LN-7 (`P3/measurements/2026-08-26-kimi-cli-print-mode-spike.md`). This rider therefore covers the **claude seat only**, and says so rather than implying coverage of a lane it cannot reach.

## Design (pre-registered) — EXACTLY the committed B1-4 harness, no code change

- Harness: `internal/adapters/claudecli/spike_test.go` — `TestSpikeSingleGatedDefer` (E1), `TestSpikeParallelSerializeByDeny` (E3), `TestSpikeDeferResumeRoundTrip`. Gates: `SINET_B1_4=1`, `SINET_HOOK_BIN=<a built sinet>`, model via `SINET_B1_4_MODEL` (`spike_test.go:53-58`). The harness takes the model from the environment, so nothing in the tree changes for this rider.
- Scale (the established B1-4/B3-3 scale): **E1 ×1, parallel E3 ×6, defer→resume round-trip ×1.**
- Model string: the FULL id **`claude-opus-5`** (live-verified 2026-09-17 against https://platform.claude.com/docs/en/models/overview). The 2026-07-22 run lost a first attempt to a bare alias; the full id is the only form the CLI accepts.

## Command

```
SINET_B1_4=1 SINET_HOOK_BIN=<built sinet> SINET_B1_4_MODEL=claude-opus-5 \
  go test -p 1 -count=1 -run 'TestSpikeSingleGatedDefer|TestSpikeParallelSerializeByDeny|TestSpikeDeferResumeRoundTrip' \
  -v ./internal/adapters/claudecli/
```

Subscription `claude` login: **$0 cash**; the API-equivalent is read from the harness's reported `total_cost_usd` (the B1-4/B3-3 shape, CONVENTIONS §11). Serial (`-p 1`).

## Expectations (pre-registered)

- **E1:** clean defer-park.
- **E3:** 6/6 faithful single-call parks at ~+1 turn under ⚙ `adapter.parallel_gate_fallback = serialize-by-deny`.
- **Defer→resume round-trip:** park (ask) → resume completes.
- **E2-context:** does first-defer-honored-on-parallel persist on `claude-opus-5`? It persisted on `claude-sonnet-5`. Either answer is a **per-pin canary FINDING**, not a rider failure — the fallback is the safety net, and a pin that does not need it is a favourable drift, not a pass condition.

## Spend projection + STOP LINE (pre-registered)

- B3-3 measured **$0.1304** on haiku ($1 / $5 per MTok). `claude-opus-5` is **$5 / $25** — 5× haiku on both rates (live-verified 2026-09-17).
- Projection: 0.1304 × 5 ≈ **$0.65 API-equivalent**.
- **STOP LINE = $2.50** (the 2026-07-22 ratio of ≈ 3.8× the projection, carried forward). At cumulative `total_cost_usd` ≥ $2.50: STOP, record the partial and the stop.

## Observation — EXECUTED 2026-09-17 (coordinator; raw log beside this file: `2026-09-17-rider2-serialize-by-deny-opus5.log`)

Run exactly as pre-registered: `SINET_B1_4=1 SINET_HOOK_BIN=<sinet built from main at 7a21c73> SINET_B1_4_MODEL=claude-opus-5 go test -p 1 -count=1 -run '…' -v ./internal/adapters/claudecli/`; serial; subscription `claude` login ($0 cash). Rider 1 had completed before this run started.

- **Installed engine vs pin (CONVENTIONS §10, reported, not retargeted):** `claude --version` = **2.1.274** at run time; `components.lock` / `claudecli.Pin` = **2.1.218**. The delta is gate item B12 in `P3/gates/rework-sitting-gate.md` (S03.3 bump proposed).
- **E1 (`TestSpikeSingleGatedDefer`):** outcome=**parked**, `gate_fallback=false`, `num_turns=1`, cost $0.037220; the ask was the gated `Bash echo HELLO` call. PASS (3.09 s).
- **E3 (`TestSpikeParallelSerializeByDeny`), 6 trials:** every trial outcome=**parked**, `gate_fallback=false`, `fires(final-window)=2`, `num_turns=2`, `ask=true`, cost $0.0371–$0.0374 each; **SUMMARY: trials=6 parallel_fallback_detected=0 clean_park=6 completed=0 total_cost=$0.223794**. PASS (22.70 s).
- **Defer→resume round-trip (`TestSpikeDeferResumeRoundTrip`):** park cost $0.037881 → resume outcome=**completed**, `num_turns=1`, cost $0.008989. PASS (8.31 s).
- **E2-context (pre-registered canary question):** first-defer-honored-on-parallel **persists on `claude-opus-5`** — 6/6 parallel gated turns parked on the first deferred call at +1 turn with the serialize-by-deny fallback never engaged (`parallel_fallback_detected=0`), the same favourable drift measured on `claude-sonnet-5` 2026-07-22 and on 2.1.217. A per-pin canary finding in the favourable direction; the fallback remains the safety net.
- **Spend:** cumulative API-equivalent **≈ $0.308** ($0.0372 + $0.2238 + $0.0469) vs the $0.65 projection and the **$2.50 STOP LINE** (never approached). Package wall 34.1 s.

## Verdict — PASS

All three legs met their pre-registered expectations on the `claude-opus-5` executor seat with the installed 2.1.274 engine: E1 clean defer-park, E3 6/6 faithful single-call parks at +1 turn under ⚙ `adapter.parallel_gate_fallback = serialize-by-deny` with the fallback unneeded, round-trip park→resume completed. Scope as stated: the claude seat only; the K3/`kimi-cli` lane has no E3 leg. What this changes in the tree: nothing beyond the record — the rider is a canary, not a gate for P3-TQ-5's commit 2 (rider 1 is that gate, and it passed).
