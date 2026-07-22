# Entailment thresholds (S12.9 #5, R24) + KL quant check (S12.9 batteries, R8/OQ3) — status — 2026-07-22

Both legs' MACHINERY is built + tested; both need a heavier bring-up execution than this packet's budget covered. Recorded honestly per the B4-5 discipline (state per-measurement what ran), NEVER silently skipped. Neither is the v0 default path.

## R24 — entailment thresholds + mandatory-coverage bar + Flan-T5 verdict

**Built + ready (this packet):**
- `verify.EntailmentChecker` LIVE-wired on the Guardian seat (binary supported/unsupported + P(yes) from the verdict-token logprob margin), the `EntailmentGate` honestly IDLE (Active requires web-research domain AND Calibrated); hermetically tested (verdict mapping incl. unverifiable; empty-source → unverifiable; gate idle for software). See `internal/stage/advisory.go`.
- The `entailment-calibration` v1 seed (10 planted supported/unsupported pairs) + the T15 `entailment` suite (12 pairs) exist versioned in-repo.
- **Servability sanity (ran):** the `entailment` classification suite scored **12/12 (100%)** on the workhorse (Qwen3.5-9B) as a proxy — the binary supported/unsupported shape + P(yes) margin works end-to-end through the pinned stack.
- Granite Guardian 3.3-8B re-pulled + sha256'd (the real entailment seat), ready to serve.

**Remaining bring-up leg (flagged):** the full **Guardian-3.3 vs 4.1 vs MiniCheck** measurement on **~200 Sinet-domain claim–citation pairs** (grown from the 10-pair seed; judged against fetched/excerpted source content). Blockers: Granite 4.1 GGUF does not exist at the guessed path (re-pull record); Bespoke-MiniCheck 401-gated (operator); Flan-T5-0.8B is `Servable:false` (seq2seq fact-check shape — its serving-path validation at b10085 is itself the flagged sub-item; sampled-checks fallback posture if unservable). The **TPR/TNR bar + mandatory-coverage bar** are pre-registered in the seed (`CalibrationSet.Bar`) BEFORE entailment ever gates unsupervised (G3 Def.4) — the gate stays idle until measured.
- **⚙ `verification.entailment_sample_rate` (declared default 0):** the SET is an operator-owned `Registry.Set` write (one tx: override row + `settings_events` + `settings.changed`), NEVER a code-default edit. The derived value comes FROM the 200-pair measurement (not yet run), so the value + the write PROCEDURE are recorded and FLAGGED to the gate here — honestly deferred with the measurement, never silently written to a wrong value.

## R8 / OQ3 — KL quant sanity check on the two default-path quants

**Built + ready (this packet):**
- `llama-perplexity` + `llama-bench` built from the SAME pinned b10085 tree (same lock entry, no new adoption) — verified (`version: 1 (b4aa7dd)` == b10085), durable at `~/.sinet-b45/bin/`.
- The KL wrapper (`battery/kl.go`, two-step baseline→compare flow, tolerant output parser — fixture-tested) + the ~250K-token Sinet-domain corpus assembler (`battery/corpus.go`, deterministic from Spec/Research/Docs, sha256-versioned) — all tier-F green.

**Remaining bring-up leg (flagged), OQ3(a) = the two default-path quants (Qwen3.5-9B Q5_K_M + Qwen3.5-4B Q4_K_M):** the FP16/BF16 baseline pulls are SANCTIONED (~26 GB, disk-checked). Two honest hardware constraints: (a) the ~26 GB baseline pull is a further network leg (queued at bring-up); (b) an **FP16 9B is ~18 GB — it does NOT fit the 12 GB pool for `--kl-divergence-base` logit generation**, so the baseline pass runs CPU-offloaded (slow) or on a larger card — a real bring-up-hardware detail, recorded not faked. The KV cache stays fp16/q8_0 (the check validates weights quant only). The harness runs any quant at its promotion moment (documented in the harness); other seats KL-check at their own swap.

## Verdict — machinery COMPLETE + tested; both executions are flagged bring-up legs (honest, not skipped)

The entailment checker + KL machinery + corpus + binaries are all built and hermetically proven; the entailment servability sanity ran 12/12. The full 200-pair entailment measurement (+ the ⚙ set) and the KL FP16-baseline run are the two remaining bring-up executions, each with a concrete honest blocker (Granite-4.1-no-GGUF / Bespoke-401 / Flan-T5-non-servable; FP16-doesn't-fit-12GB + the 26 GB pull). Pre-registered bars are in place BEFORE any unsupervised gating. Flagged to the B4 gate.
