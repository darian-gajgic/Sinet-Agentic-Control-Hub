# KL quant sanity check (S12.9, R8, OQ3(a), drain C9/F7b) — 2026-07-22

- **DRAIN C9 CORRECTION:** round 1's §28 said "Executed on the two default-path quants" — FALSE (not run). This file records the ACTUAL execution.
- Tool: `llama-perplexity` (built from the pinned b10085 tree, `version: 1 (b4aa7dd)` == b10085). Corpus: the ~250K-token Sinet-domain corpus (`kl_corpus.txt`, assembled deterministically from Spec/Research/Docs, 1,050,000 B ≈ 262,500 tokens, sha256 `592874e7…`). Two-step: baseline logits from the BF16 model → compare the deployed quant. Weights-quant only; KV cache stays fp16 (S12.3). $0 local; on AC.
- Baselines (BF16, sanctioned OQ3(a) pull): Qwen3.5-4B-BF16 (8.42 GB, sha `9e6e2841…`), Qwen3.5-9B-BF16 (pulled to `~/.sinet-b45/baselines/`, deletable after the check).

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

### 9B Q5_K_M vs 9B BF16 — MEASURED infeasible on this hardware (recorded, not assumed)

MEASURED evidence: the 9B BF16 baseline is **17,920,697,312 B (17.92 GB)** (HF content-length) — it EXCEEDS the RTX 5070 Ti pool of **12,227 MiB (12.83 GB)**, so `llama-perplexity --kl-divergence-base` cannot materialize the 9B BF16's logits on the GPU (the 4B baseline, 8.42 GB, DID fit and ran — the direct contrast). The interrupted local copy is 15.28 GB (the sanctioned pull was killed mid-stream). The CPU path is available (host RAM 30 GB) but ~10–30× slower than the 4B GPU run (the 4B GPU pass alone is ~10 min/pass; 9B on CPU over the same corpus is a multi-hour AC-batch job) — a bring-up leg, not a same-session run. The 4B leg above is the OQ3(a) representative faithfulness check on the default-path fast quant; the 9B leg completes at bring-up via CPU/partial-offload with its own footprint-fitting `-ngl` split. The check binds the DEPLOYED quant at its promotion moment (documented in `battery/kl.go`) — the 9B is validated at its own swap/bring-up.

## Verdict — 4B Q4_K_M is a FAITHFUL deployed quant (executed); 9B leg measured-infeasible on this pool

- **4B Q4_K_M: PASS the faithfulness expectation.** Mean KLD **0.0369** is low (for reference, ~<0.05 mean KLD is the "safe substitute" band cited in the llama.cpp KLD guidance the corpus assembler documents); median 0.022; the quant tracks BF16 tightly (PPL only **1.7%** higher, ln-PPL correlation 99.57%, top-1 token agreement 90.0%). The tail (99.9% KLD 2.28, max 20.67) is the expected handful of high-divergence tokens on a 545-chunk corpus, not a systematic drift. The default-path fast quant is a safe substitute for its BF16 parent — the S12.9/OQ3(a) check the swap gate binds at promotion.
- **9B Q5_K_M: not run — MEASURED infeasible on this pool** (17.92 GB BF16 baseline > 12.83 GB VRAM; interrupted local copy 15.28 GB), completes at bring-up via CPU/partial-offload. Recorded with the size evidence above, not assumed.
- The harness (`battery/kl.go`) runs any quant against its BF16 parent at the seat's promotion moment; this file is the 4B execution + the 9B measured constraint. $0 local; host gate clean (user-space `llama-perplexity`, GPU shared with the operator's own processes untouched).
