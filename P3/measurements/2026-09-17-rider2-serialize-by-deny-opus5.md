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

## Observation — PENDING

Not executed. The coordinator runs the command above and records here: the three legs' outcomes with `gate_fallback` / `parallel_fallback_detected` / `num_turns` per trial, the cumulative API-equivalent spend against the projection and the stop line, and the **installed `claude` version against the `components.lock` pin** — reported per CONVENTIONS §10, never retargeted.

## Verdict — PENDING
