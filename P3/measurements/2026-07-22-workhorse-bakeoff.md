# Workhorse bakeoff — Qwen3.5-9B vs Gemma-4-12B-QAT (S12.9 #1, R20) — 2026-07-22

- Instrument: the T15 battery `intake-triage` suite (32 synthetic Sinet-domain seeds; real traces accumulate later — S12.9). Each model served on b10085 sm_120 (ctx 8192/4096, 4 slots, ngl 99). $0 local. On AC.
- Basis stated: SYNTHETIC bring-up seeds (S12.9's "real Sinet-domain traces accumulate later"). Scoring is STRICT 3-field (family+stakes+size all match).

## Pre-registered expectation

The 9B ships as default alias target UNLESS the bakeoff flips it (G3 Def.8). A flip applies ONLY through the S12.10 swap gate promotion rule (R10: challenger wins ≥2 duty suites with NON-overlapping 95% Wilson at equal-or-better tokens/s). The Gemma flip is ADDITIONALLY blocked pending operator ratification of the flagged card-vs-spec license conflict. Running the measurement is sanctioned; applying a flip without ratification is not.

## Observation

| model | intake-triage pass | 95% Wilson | tokens/s | model hash (sha256) |
|---|---|---|---|---|
| **Qwen3.5-9B Q5_K_M** (incumbent/default) | 7/32 (21.9%) | **[0.11, 0.39]** | **46.2** | dc2a39ae… |
| Gemma-4-12B-QAT int4 (challenger) | 8/32 (25.0%) | **[0.13, 0.42]** | 41.2 | 93567e57… |

Both score low on strict 3-field triage — the `size` label is genuinely subjective (see the per-duty calibration file); the RELATIVE comparison is what the bakeoff decides.

## Promotion-rule application (R10, the swap gate's `DecidePromotion`)

- intake-triage: Gemma Wilson-lo **0.13** does NOT clear the 9B Wilson-hi **0.39** → **OVERLAPPING intervals** (not a win). AND Gemma **41.2 t/s < 46.2 t/s** → NOT equal-or-better tokens/s (also disqualifying).
- Duty suites won by Gemma: **0** (the rule needs ≥2).

## Verdict — PASS: the 9B STAYS the default; the bakeoff does NOT flip it

Gemma-4-12B-QAT does not clear the S12.10 promotion rule on the measured battery: 0 duty suites won (overlapping Wilson AND slower on the one contested suite). The swap gate — the SOLE `⚙ local.alias` write surface — therefore does not admit a workhorse flip. Additionally, even had Gemma won, the alias flip stays BLOCKED pending operator ratification of the Gemma card-vs-spec license conflict (`manifest.go`, `License.Conflict`). Qwen3.5-9B Q5_K_M ships as the default workhorse alias target (G3 Def.8 upheld). The measurement was $0 local; no ⚙ write happened; host gate clean.

Note (harness): the strict `size` field depresses both absolute pass rates; the bakeoff's relative verdict is unaffected. A size-agnostic triage seed (growing under governance) would raise both absolute numbers — future work, not a flip trigger.
