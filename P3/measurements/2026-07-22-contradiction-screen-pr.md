# Contradiction-screen precision/recall (S12.9 #4, S09.7, R23) — 2026-07-22

- Shipped shape per OQ4(a): **ONE-STAGE** — the workhorse CONFIRM on the `contradiction-screen` duty (the DeBERTa-class pre-screen seat is `Servable:false` at the pin, no GGUF path; the two-stage pre-screen re-enters when a serving path exists). So R23 measures the one-stage workhorse screen (Qwen3.5-9B Q5_K_M, b10085), json_schema + abstain, temp-0.
- Set: 12 synthetic lesson pairs versioned with the battery (`seedContradiction`), **label-balanced 6 contradict / 6 not** (deploy-Fridays-yes/no, tabs-vs-spaces, region defaults, migration order, log level, etc.).

## Pre-registered expectation

Literature (S12.9) predicts **high precision / weak recall with temporal/numeric blind spots**. Record P/R + the operating threshold. Closes `TBD-BRINGUP(contradiction-screen P/R)` (S09.7) for the shipped shape.

## Observation (Qwen3.5-9B, one-stage confirm, b10085)

- Suite pass = **12/12 (100%)**, 95% Wilson **[0.76, 1.00]**, 42.9 t/s.
- Of the 6 genuine contradictions: all 6 correctly `contradicts=yes` → **recall = 1.00** on the seed.
- Of the 6 non-contradictions: all 6 correctly `no`/`unclear` (never a false `yes`) → **precision = 1.00** on the seed.
- Margin threshold (S12.5 fit): 5.65; achieved error 0; meets_bar true (the confidence calibration for this duty is clean).

## Verdict — PASS (one-stage shape; P=R=1.0 on the synthetic seed, with the honest weak-recall caveat)

The one-stage workhorse contradiction screen achieves **perfect precision AND recall on the 12-pair synthetic seed** — consistent with the "high precision" prediction, and the seed did not surface the predicted weak-recall blind spots (subtle temporal/numeric contradictions). This is the EXPECTED limitation of a small synthetic set: the weak-recall caveat is about hard temporal/numeric pairs that a 12-pair balanced seed does not stress. The high-precision advisory posture holds (question-cards only, never silent resolution; the deterministic same-`topic_key` detection runs regardless and is never suppressed by the screen). Closes `TBD-BRINGUP(contradiction-screen P/R)` for the shipped one-stage shape; recall against hard real lesson pairs is the growth item under golden-set governance (real traces sharpen the recall estimate). $0 local; host gate clean.
