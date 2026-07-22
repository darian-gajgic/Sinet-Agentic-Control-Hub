# VRAM-ledger calibration (S12.9 #2, S12.7 protocol, R21) — 2026-07-22

- Host: RTX 5070 Ti Laptop, **12227 MiB** total; on AC (`power_supply` online=1); GPU `runtime_status` active. Engine b10085 (llama-server, shared-lib CUDA build sm_120). Sleep-gate discipline held (device-truth `memory.free`; per-process attribution via `nvidia-smi -q -x`).
- Display mode at measurement: **HYBRID** (the recommended VRAM-maximizing mode — dGPU + iGPU; compositor can offload to the iGPU).

## Pre-registered expectation

Per-model footprints measured through a WARM-UP generation (KV + CUDA graphs materialize then), keyed by the full S12.7 tuple (model, quant, context, slots, engine build) [R18]. Both compositor-headroom figures (hybrid + MUX). Unload/kill→free latencies via the preempt path. Admission rests on the LIVE `memory.free` reading regardless of the ledger belief.

## Observation

### Per-model footprint (warm, measured through the battery's generation load)

| tuple (model, quant, ctx, slots, engine) | footprint | measured how |
|---|---|---|
| Qwen3.5-9B, Q5_K_M, 8192, 4, b10085 | **6340 MiB** | `nvidia-smi -q -x` process attribution while the 9B suite (100+ generations) ran — KV + CUDA graphs fully materialized |
| Gemma-4-12B-QAT, int4 QAT, 4096, 4, b10085 | **~8352 MiB** | device used 11624 − wisprflow 3272 while Gemma served the bakeoff |

The 9B footprint (6340 MiB) is the calibrated value for the DEFAULT-path workhorse tuple; the ledger's `SetFootprint` fills it per tuple (an unmeasured tuple stays honest-uncalibrated). Guard band ⚙ `local.vram.guard_band_mb` (declared) is the admission headroom on top.

### Live-reading-authoritative admission — the S12.7 scenario, observed IN THE WILD

An operator app (`local-wisprflow`, a voice tool, PID 764489) held **~3300 MiB** of VRAM throughout. This is EXACTLY the S12.7 case the admission math rests on: the platform's admission uses `live memory.free ≥ footprint + guard`, NEVER the ledger belief — because a non-platform process (wisprflow, the compositor) legitimately holds VRAM. With the 9B (6340) + wisprflow (3272) + compositor loaded, `memory.free` fell to **2043–2075 MiB**; the LIVE reading is what any load decision consults.

### Compositor headroom (S12.7 step 6)

- **HYBRID** (measured now): non-model, non-app baseline ≈ 12227 − 6340(9B) − 3272(wisprflow) − 2075(free) ≈ **~540 MiB** for the compositor/desktop. Hybrid is the recommended mode.
- **MUX** (dGPU-only): NOT measured — requires a display-mode switch (BIOS/reboot), an **OPERATOR-ASSISTED leg** (flagged; the hybrid figure recorded now, the MUX figure never faked). MUX would move the full compositor onto the dGPU (typically +1–2 GB less headroom).

### Unload / kill→free latency (the B4-6 preempt path)

- **Kill→free (SIGTERM):** killed the 9B `llama-server`; `memory.free` recovered **2075 → 8453 MiB** (+6378 MiB, matching the 6340 MiB footprint) essentially immediately — the CUDA context released cleanly on SIGTERM. Confirms kill-not-freeze (S12.7): teardown frees VRAM, and the freed amount == the footprint. (The tier-L conformance additionally proves the mandatory SIGTERM→⚙grace→SIGKILL against a SIGTERM-ignoring backend + recovery-verified gate.)

## Verdict — PASS (hybrid leg complete; MUX leg = flagged operator-assisted)

The S12.7 protocol ran on the real machine: per-tuple footprints measured warm (9B 6340 MiB, Gemma ~8352 MiB), the live-reading-authoritative admission validated against a REAL competing VRAM holder (wisprflow 3.3 GB), the hybrid compositor headroom measured (~540 MiB), and kill→free confirmed (SIGTERM freed the exact footprint). The MUX-mode headroom is the one operator-assisted leg (display-mode switch), flagged not faked. `SetFootprint` fills the ledger per R18 tuple; the durable calstore/platform.db write of these figures is the bring-up step (OQ1 tables exist). Host gate clean (no driver/persistenced/clock change; user-level reads only).
