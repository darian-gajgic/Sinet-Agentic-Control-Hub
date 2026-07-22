# CPU-tier throughput floor (S12.9 #3, R22) — 2026-07-22

- systemd/host: Intel Core Ultra 9 275HX, 24 cores; RTX 5070 Ti Laptop 12 GB; on AC (`power_supply` online=1); GPU `runtime_status` active.
- Tool: `llama-bench` built from the pinned llama.cpp b10085 tree (R8 build target), `LD_LIBRARY_PATH` = the b10085 build's `bin/` (shared-lib build).
- Basis: the deployed fast/CPU seat Qwen3.5-4B Q4_K_M (the S12.3 3–4B CPU-viable class). `-ngl 0` runs every transformer layer on the CPU (weights in RAM); `-ngl 99` offloads all to the GPU for the contrast.

## Pre-registered expectation

The ~20–35 t/s figure in S12.9 is a class estimate with "no citable laptop source" — this measurement REPLACES it. Expectation: a 4B Q4 generates in the low-tens of t/s on a modern laptop CPU; the GPU is several× faster; the verdict decides whether the S12.8 battery-mode CPU fallback is real and whether a 2B downshift is needed (the parked re-entry).

## Observation (llama-bench, 2 reps)

| model | backend | ngl | test | t/s |
|---|---|---|---|---|
| Qwen3.5-4B Q4_K_M | CUDA | 0 (CPU) | pp128 | 556.26 ± 26.75 |
| Qwen3.5-4B Q4_K_M | CUDA | 0 (CPU) | **tg128 (generation)** | **11.86 ± 0.67** |
| Qwen3.5-4B Q4_K_M | CUDA | 99 (GPU) | pp128 | 3454.12 ± 735.15 |
| Qwen3.5-4B Q4_K_M | CUDA | 99 (GPU) | tg128 (generation) | 101.84 ± 1.26 |

CPU generation floor = **~11.9 t/s** (4B Q4); GPU = ~102 t/s (≈8.6× faster). CPU prompt processing (556 t/s) stays fast because the CPU handles the batched prefill well; generation is the memory-bandwidth-bound floor.

## Verdict — PASS (measured; below the class estimate)

The CPU floor is **~12 t/s for a 4B Q4**, BELOW the ~20–35 t/s class estimate. Decision (R22):

- **The S12.8 battery-mode CPU fallback IS real** but slow: ~12 t/s generation. Adequate for the CLASSIFICATION duties (short JSON labels — a 30-token label emits in ~2.5 s) that dominate the free-tier registry; slow for long free-text drafting.
- **A 2B CPU downshift IS worth keeping as the parked re-entry** for interactive CPU-only / on-battery operation: a 2B model would roughly double CPU generation (~24 t/s, closer to the class estimate) at some quality cost. Not needed at v0 (the 4B floor serves the classification duties acceptably), flagged as the battery/CPU-only optimization when the GPU is unavailable.
- The ~8.6× GPU speedup confirms the GPU tier is the default and the CPU tier is the battery/asleep-GPU fallback (S12.8), exactly as the battery-admission policy assumes.

Host gate clean: user-level binary, no driver/system change, on AC.
