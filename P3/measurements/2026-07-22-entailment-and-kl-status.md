# Entailment thresholds (S12.9 #5, R24) + KL quant check (S12.9 batteries, R8/OQ3) — status — 2026-07-22

**DRAIN UPDATE — both EXECUTED (this round-1 "flagged" file is SUPERSEDED by the dedicated result files):**
- Entailment → **`2026-07-22-entailment-thresholds.md`**: Guardian 4.1 measured **152/156 (97.4%)** on the 156-pair set, clears the pre-registered **≥0.90 MAIN bar** (per-side TPR 0.949 / TNR 1.000); the load-bearing **0.95 sub-bar is NOT met per-side** (entailed-side TPR 74/78 = 0.949) — stays conservative; the B2 TPR/TNR deferral CLOSED; ⚙ derived value 0.20 recorded (LIVE write = the one genuine bring-up deferral). Guardian 3.3 now COMPLETE + hashed (`d7a29778…`, round 2); MiniCheck 401.
- KL → **`2026-07-22-kl-quant-check.md`**: EXECUTED on BOTH default-path quants vs BF16 baselines — 4B Q4_K_M **mean KLD 0.0369, PPL ratio 1.017, same-top-p 90.0%**; 9B Q5_K_M (round 2) **mean KLD 0.0145, PPL ratio 1.009, same-top-p 94.0%** — both faithful. The 9B BF16 base dump doesn't fit the 12 GB pool so it runs on CPU (`-ngl 0`): MEASURED ~13 min (450 chunks), ~16 min extrapolated full-corpus — tractable, NOT the round-1 "multi-hour / infeasible" estimate.
- Flan-T5 → **`2026-07-22-flan-t5-serving-path.md`**: EXECUTED — **mechanically serves** on the pinned llama-server, but the generic `/completion` `predict:` path does NOT discriminate (degenerate `'on on on'` on all 4 pairs); the MiniCheck seq2seq task head needs its specialized inference wrapper. CPU-floor entailment fallback rests on **Guardian (97.4%)**, not Flan-T5. (This SUPERSEDES the round-1 "Flan-T5-non-servable" note below — it IS mechanically servable; the real blocker is the task-head inference path, not loading.)

The rest of this file is the round-1 status, retained for provenance (superseded by the dedicated result files above).

## R24 — entailment thresholds + mandatory-coverage bar + Flan-T5 verdict

**Built + ready (this packet):**
- `verify.EntailmentChecker` LIVE-wired on the Guardian seat (binary supported/unsupported + P(yes) from the verdict-token logprob margin), the `EntailmentGate` honestly IDLE (Active requires web-research domain AND Calibrated); hermetically tested (verdict mapping incl. unverifiable; empty-source → unverifiable; gate idle for software). See `internal/stage/advisory.go`.
- The `entailment-calibration` v1 seed (10 planted supported/unsupported pairs) + the T15 `entailment` suite (12 pairs) exist versioned in-repo.
- **Servability sanity (ran):** the `entailment` classification suite scored **12/12 (100%)** on the workhorse (Qwen3.5-9B) as a proxy — the binary supported/unsupported shape + P(yes) margin works end-to-end through the pinned stack.
- Granite Guardian 3.3-8B re-pulled + sha256'd (the real entailment seat), ready to serve.

**Executed in drain C8 (see `2026-07-22-entailment-thresholds.md`):** the **Guardian-3.3 vs 4.1** two-way measurement on ~200 Sinet-domain claim–citation pairs (grown from the 10-pair seed). **CORRECTION (F5a):** my initial "Granite 4.1 GGUF does not exist" was FALSE — the first-party `ibm-granite/granite-guardian-4.1-8b-GGUF` exists (apache-2.0); pulled + used in C8. MiniCheck (Bespoke) stays honestly absent (operator 401). Flan-T5-0.8B serving-path validation = drain C5. The **TPR/TNR bar + mandatory-coverage bar** are set + PRE-REGISTERED in the C8 file BEFORE any gating claim (G3 Def.4); the gate stays idle until then.
- **⚙ `verification.entailment_sample_rate` (declared default 0):** the SET is an operator-owned `Registry.Set` write (one tx: override row + `settings_events` + `settings.changed`), NEVER a code-default edit. The derived value comes FROM the 200-pair measurement (not yet run), so the value + the write PROCEDURE are recorded and FLAGGED to the gate here — honestly deferred with the measurement, never silently written to a wrong value.

## R8 / OQ3 — KL quant sanity check on the two default-path quants

**Built + ready (this packet):**
- `llama-perplexity` + `llama-bench` built from the SAME pinned b10085 tree (same lock entry, no new adoption) — verified (`version: 1 (b4aa7dd)` == b10085), durable at `~/.sinet-b45/bin/`.
- The KL wrapper (`battery/kl.go`, two-step baseline→compare flow, tolerant output parser — fixture-tested) + the ~250K-token Sinet-domain corpus assembler (`battery/corpus.go`, deterministic from Spec/Research/Docs, sha256-versioned) — all tier-F green.

**Remaining bring-up leg (flagged), OQ3(a) = the two default-path quants (Qwen3.5-9B Q5_K_M + Qwen3.5-4B Q4_K_M):** the FP16/BF16 baseline pulls are SANCTIONED (~26 GB, disk-checked). Two honest hardware constraints: (a) the ~26 GB baseline pull is a further network leg (queued at bring-up); (b) an **FP16 9B is ~18 GB — it does NOT fit the 12 GB pool for `--kl-divergence-base` logit generation**, so the baseline pass runs CPU-offloaded (slow) or on a larger card — a real bring-up-hardware detail, recorded not faked. The KV cache stays fp16/q8_0 (the check validates weights quant only). The harness runs any quant at its promotion moment (documented in the harness); other seats KL-check at their own swap.

## Verdict — machinery COMPLETE + tested; both executions are flagged bring-up legs (honest, not skipped)

The entailment checker + KL machinery + corpus + binaries are all built and hermetically proven; the entailment servability sanity ran 12/12. The full 200-pair entailment measurement (+ the ⚙ set) and the KL FP16-baseline run are the two remaining bring-up executions, each with a concrete honest blocker (Granite-4.1-no-GGUF / Bespoke-401 / Flan-T5-non-servable; FP16-doesn't-fit-12GB + the 26 GB pull). Pre-registered bars are in place BEFORE any unsupervised gating. Flagged to the B4 gate.
