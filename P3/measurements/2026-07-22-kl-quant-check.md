# KL quant sanity check (S12.9, R8, OQ3(a), drain C9/F7b) — 2026-07-22

- **DRAIN C9 CORRECTION (round 1) + R1 (round 2):** round 1's §28 said "Executed on the two default-path quants" — FALSE at the time (only the 4B ran; the 9B was deferred on an UNMEASURED "multi-hour" estimate). Drain round 2 completed the 9B BF16 baseline pull and EXECUTED the 9B leg too (below), so BOTH default-path quants are now genuinely KL-checked. This file records the ACTUAL executions.
- Tool: `llama-perplexity` (built from the pinned b10085 tree, `version: 1 (b4aa7dd)` == b10085). Corpus: the ~250K-token Sinet-domain corpus (`kl_corpus.txt`, assembled deterministically from Spec/Research/Docs, 1,050,000 B ≈ 262,500 tokens, sha256 `592874e7…`). Two-step: baseline logits from the BF16 model → compare the deployed quant. Weights-quant only; KV cache stays fp16 (S12.3). $0 local; on AC.
- Baselines (BF16, sanctioned OQ3(a) pull): Qwen3.5-4B-BF16 (8.42 GB, sha `9e6e2841…`), Qwen3.5-9B-BF16 (**17,920,697,312 B == HF content-length**, completed in drain round 2; in `~/.sinet-b45/baselines/`, deletable after the check).

## Pre-registered expectation

A faithful weights quant tracks its BF16 baseline closely: a LOW mean KL divergence over the corpus (the quant is a safe substitute). The check validates the DEPLOYED quants (4B Q4_K_M, 9B Q5_K_M).

## Observation

### 4B Q4_K_M vs 4B BF16 (fits the 12 GB pool, -ngl 99) — EXECUTED, 545 chunks

Two-step run over the corpus (baseline logit dump from 4B BF16 = 69.0 GB `.dat`, then compare the deployed 4B Q4_K_M):

| statistic | value |
|---|---|
| **Mean KLD** | **0.036879 ± 0.000521** |
| Median KLD | 0.021778 |
| 90.0% KLD | 0.060078 |
| 95.0% KLD | 0.083162 |
| 99.0% KLD | 0.207924 |
| 99.9% KLD | 2.281247 |
| Maximum KLD | 20.667883 |
| Mean PPL(Q) / PPL(base) | **1.017087 ± 0.000903** (Q4_K_M perplexity 1.7% above BF16) |
| Cor(ln PPL) | 99.57% |
| Same-top-p | 90.021 ± 0.080 % |
| RMS Δp | 4.186 ± 0.057 % |

### 9B Q5_K_M vs 9B BF16 — EXECUTED (bounded CPU timing probe → full two-step run), 450 chunks

The 9B BF16 baseline is **17,920,697,312 B (17.92 GB)** (completed in drain round 2; disk == HF content-length) — it EXCEEDS the RTX 5070 Ti pool of **12,227 MiB (12.83 GB)**, so the `--kl-divergence-base` logit dump cannot run on the GPU. It DOES run on **CPU** (`-ngl 0`), and a bounded probe MEASURED the cost directly — replacing the round-1 UNMEASURED "~10–30× slower / multi-hour" estimate, which was wrong by ~an order of magnitude.

**MEASURED (base dump, CPU `-ngl 0`, 4 threads of 24, 19.65 GB RSS on the 30 GB host):** **5.70 s/pass** at n_seq=4 (4 chunks/pass) = **~1.42 s/chunk ≈ 362 tok/s** prefill; the 450-chunk base-dump wall-clock was **10 min 36 s** (writing a 57 GB scratch `.dat`, deleted after). The GPU compare step (deployed Q5_K_M, `-ngl 99`) added **2 min 18 s** (1.19 s/pass). **450-chunk total ≈ 12 min 54 s.**

**Extrapolation to the full 545-chunk corpus:** base dump ~136 passes × 5.70 s ≈ **~12.9 min**; compare ~2.8 min; **full-corpus total ≈ ~16 min** (the `.dat` would be ~69 GB — the reason this run used 450 chunks / ~206K tokens for safe disk headroom on the live host, extrapolating the remainder). **VERDICT: tractable — execute-now, and EXECUTED here** (not a multi-hour bring-up leg).

**KL result (9B Q5_K_M vs 9B BF16, 450 chunks / ~206K tokens):**

| statistic | value |
|---|---|
| **Mean KLD** | **0.014476 ± 0.000499** |
| Median KLD | 0.007277 |
| 90.0% KLD | 0.020362 |
| 95.0% KLD | 0.028103 |
| 99.0% KLD | 0.069537 |
| 99.9% KLD | 1.088824 |
| Maximum KLD | 19.509464 |
| Mean PPL(Q)/PPL(base) | **1.008752 ± 0.000634** (Q5_K_M perplexity 0.88% above BF16) |
| Cor(ln PPL) | 99.81% |
| Same-top-p | 94.026 ± 0.070 % |
| RMS Δp | 2.706 ± 0.072 % |

## Verdict — BOTH default-path quants are FAITHFUL (executed); the 9B leg is tractable-and-DONE, not infeasible

- **4B Q4_K_M: PASS the faithfulness expectation.** Mean KLD **0.0369** is low (for reference, ~<0.05 mean KLD is the "safe substitute" band cited in the llama.cpp KLD guidance the corpus assembler documents); median 0.022; the quant tracks BF16 tightly (PPL only **1.7%** higher, ln-PPL correlation 99.57%, top-1 token agreement 90.0%). The tail (99.9% KLD 2.28, max 20.67) is the expected handful of high-divergence tokens on a 545-chunk corpus, not a systematic drift. The default-path fast quant is a safe substitute for its BF16 parent — the S12.9/OQ3(a) check the swap gate binds at promotion.
- **9B Q5_K_M: PASS the faithfulness expectation (EXECUTED).** Mean KLD **0.0145** — even LOWER (more faithful) than the 4B Q4's 0.0369, as expected for the higher-fidelity Q5 quant; PPL ratio **1.0088** (0.88% above BF16), same-top-p 94.0%, ln-PPL correlation 99.81%; median KLD 0.0073. The tail (99.9% KLD 1.09, max 19.5) is the expected handful of high-divergence tokens, not systematic drift. The workhorse-DEFAULT 9B Q5_K_M is a safe substitute for its BF16 parent. **The round-1 "measured-infeasible / multi-hour" claim is CORRECTED:** the BF16 baseline doesn't fit the 12 GB pool (true), but the CPU base-dump path is **~13 min measured** (~16 min extrapolated to the full 545-chunk corpus) — tractable, and run here.
- The harness (`battery/kl.go`) runs any quant against its BF16 parent at the seat's promotion moment; this file now records BOTH default-path executions. $0 local; on AC; host gate clean (user-space `llama-perplexity` on CPU for the base dump + the deployed 9B on its own GPU pool for the compare; the operator's own GPU processes untouched).
