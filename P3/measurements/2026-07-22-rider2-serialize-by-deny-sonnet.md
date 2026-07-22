# Rider 2 — serialize-by-deny E3 leg on the sonnet-5 executor (S12/D3, R27) — 2026-07-22

**PAID LEG (pre-ratified at the B3 gate; the packet's only paid legs with rider 1). Pre-registered BEFORE any paid call.**

## Why this runs

The D3 advisor split set the EXECUTION seat to **sonnet-5** (`worker.DefaultDutyMap()`, applied `e06f0a4`). The B1-4 serialize-by-deny spike (parallel-gate fallback) was measured on haiku; the per-pin canary re-confirms it on the ratified sonnet-5 executor. A favorable-drift change (e.g. first-defer-honored-on-parallel persisting) is a FINDING for the per-pin canary, not a failure of the rider.

## Design (pre-registered) — EXACTLY the committed B1-4 harness

- Harness: `internal/adapters/claudecli/spike_test.go`, `SINET_B1_4=1`, real `sinet engine-hook`, `SINET_B1_4_MODEL=<the DefaultDutyMap execution seat = sonnet-5>`.
- Scale (the established B1-4/B3-3 scale): **E1 ×1 (clean defer-park), parallel E3 ×6 (faithful single-call parks), defer→resume round-trip ×1.**
- Expectations (verbatim from the predecessor files 2026-07-20 + 2026-07-21):
  - E1: clean defer-park.
  - E3: 6/6 faithful single-call parks at ~+1 turn under ⚙ `adapter.parallel_gate_fallback = serialize-by-deny`.
  - E2-context: does first-defer-honored-on-parallel persist on sonnet-5? The favorable drift is model/engine-dependent — a fallback-to-completion on sonnet-5 is a FINDING for the per-pin canary, not a rider failure.

## Spend projection + STOP LINE (pre-registered)

- sonnet-5 pricing (claude-api skill): **$3.00 / 1M input ($2.00 intro through 2026-08-31), $15.00 / 1M output ($10.00 intro).**
- B3-3 measured **$0.1304** on haiku (haiku $1/$5). sonnet-5 ≈ 3× haiku per-token weight (per the B3 D3 record).
- Projection: 0.1304 × 3 ≈ **$0.40** (the ratified projection). Spend reads from the harness' reported `total_cost_usd` (the B1-4/B3-3 shape, CONVENTIONS §11).
- **STOP LINE = $1.50** (coordinator-ratified 2026-07-22). If cumulative `total_cost_usd` reaches $1.50: STOP + record the partial + the stop.

## Observation (2026-07-22, claude 2.1.217 installed vs lock pin 2.1.216 — reported per §10, never retargeted)

**Model-string note:** the DefaultDutyMap execution seat is the FULL id `claude-sonnet-5` (the brief's "sonnet-5" is shorthand). A first run with the bare `sonnet-5` alias returned `api_error` at $0 (the CLI rejected the alias) — no spend, well under the stop line; re-ran with `claude-sonnet-5`.

- **E1 (single gated defer):** outcome=**parked**, gate_fallback=false, num_turns=1, cost=$0.037442. Clean defer-park ✓ (matches the expectation).
- **E3 (parallel ×6):** **6/6 clean_park**, gate_fallback=false, parallel_fallback_detected=0, fires(final-window)=2, num_turns=2 (~+1 turn), ask=true on every trial. Total $0.226171.
- **Defer→resume round-trip:** park (ask, $0.037679) → resume outcome=**completed** ($0.005587), num_turns=1 ✓.
- **Total spend = $0.306879 (≈ $0.31)** — under the $0.40 projection, well under the $1.50 STOP LINE. Stop line never approached.

## Verdict — PASS (with the expected per-pin canary finding)

All three legs PASS. **Per-pin canary FINDING (E2-context):** on **claude-sonnet-5**, parallel gated tools yield **6/6 CLEAN single-call parks at ~+1 turn WITHOUT the serialize-by-deny fallback firing** (`gate_fallback=false`, `parallel_fallback_detected=0`) — i.e. the FAVORABLE first-defer-honored-on-parallel behavior PERSISTS on sonnet-5. This is model/engine-dependent drift and is a per-pin canary finding, NOT a rider failure (exactly the pre-registered E2-context reading). The serialize-by-deny fallback (⚙ `adapter.parallel_gate_fallback`) remains the safety net if a future pin regresses; on this pin it isn't needed because the engine already honors the first defer. TBD-BRINGUP(parallel-gate fallback) reconfirmed on the ratified sonnet-5 executor. Host gate clean; only the two riders' paid calls.
