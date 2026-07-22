# Workhorse bakeoff — Qwen3.5-9B vs Gemma-4-12B-QAT (S12.9 #1, R20) — 2026-07-22

- Instrument: the T15 battery `intake-triage` suite (32 synthetic Sinet-domain seeds; real traces accumulate later — S12.9). Each model served on b10085 sm_120 (ctx 8192/4096, 4 slots, ngl 99). $0 local. On AC.
- Basis stated: SYNTHETIC bring-up seeds (S12.9's "real Sinet-domain traces accumulate later"). Scoring is STRICT 3-field (family+stakes+size all match).

## Pre-registered expectation

The 9B ships as default alias target UNLESS the bakeoff flips it (G3 Def.8). A flip applies ONLY through the S12.10 swap gate promotion rule (R10: challenger wins ≥2 duty suites with NON-overlapping 95% Wilson at equal-or-better tokens/s). The Gemma flip is ADDITIONALLY blocked pending operator ratification of the flagged card-vs-spec license conflict. Running the measurement is sanctioned; applying a flip without ratification is not.

## Observation — FULL grown battery (drain C3/F3: Gemma side across ALL suites, t15-v1 ≥30 cases)

The round-1 bakeoff compared only intake-triage. C3 runs the FULL classification battery on BOTH sides so the ≥2-suite `DecidePromotion` rule is exercisable.

| classification suite | Qwen3.5-9B (incumbent) | Gemma-4-12B-QAT (challenger) | Gemma win? |
|---|---|---|---|
| intake-triage | 7/32 [0.11,0.39] | 8/32 [0.13,0.42] | no (overlap) |
| watchdog-disambiguator | 25/30 [0.66,0.93] | 28/30 [0.79,0.98] | no (0.79 < 0.93 overlap) |
| watchlist-triage | 25/30 [0.66,0.93] | 25/30 [0.66,0.93] | no (tie) |
| intent-filling | 26/30 [0.70,0.95] | 23/30 [0.59,0.88] | no (Gemma lower) |
| entailment | 30/30 [0.89,1.00] | 29/30 [0.83,0.99] | no (Gemma lower) |
| contradiction-screen | 28/30 [0.79,0.98] | 27/30 [0.74,0.97] | no (Gemma lower) |

Throughput: 9B **43–47 t/s** vs Gemma **41–48 t/s** (comparable, Gemma marginally slower at ctx 4096). Model hashes: 9B `dc2a39ae…`, Gemma `93567e57…`.

## Promotion-rule application (R10, the swap gate's `DecidePromotion`)

- Duty suites Gemma wins with NON-overlapping 95% Wilson AT equal-or-better tokens/s: **0** (the rule needs ≥2). Every suite either overlaps or Gemma is lower; on watchdog (Gemma's best) the intervals still overlap (0.79 < 0.93).
- `DecidePromotion(9B, Gemma)` ⇒ **Promote=false, Wins=0**. The ≥2-suite rule is now demonstrably exercisable (full battery on both sides) and is NOT met.

## Verdict — PASS: the 9B STAYS the default; the bakeoff does NOT flip it

Gemma-4-12B-QAT does not clear the S12.10 promotion rule on the FULL grown battery: 0 of 6 duty suites won (overlapping Wilson or Gemma-lower on every one). The swap gate — the SOLE `⚙ local.alias` write surface — therefore does not admit a workhorse flip. Additionally, even had Gemma won, the alias flip stays BLOCKED pending operator ratification of the Gemma card-vs-spec license conflict (`manifest.go`, `License.Conflict`). Qwen3.5-9B Q5_K_M ships as the default workhorse alias target (G3 Def.8 upheld). The measurement was $0 local; no ⚙ write happened; host gate clean.

Note (harness): the strict `size` field depresses both absolute pass rates; the bakeoff's relative verdict is unaffected. A size-agnostic triage seed (growing under governance) would raise both absolute numbers — future work, not a flip trigger.
