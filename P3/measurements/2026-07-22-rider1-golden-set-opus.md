# Rider 1 — P-T06-5 golden-set run on the opus-4-8 judge + length bias (S07.9, R26) — 2026-07-22

**PAID LEG (pre-ratified at the B3 gate; the packet's only paid legs with rider 2). Pre-registered BEFORE any paid call.**

## Why this fires (S07.9 P-T06-5)

The D3 advisor split (applied `e06f0a4`) set the judge seat to **opus-4-8** — a judge-model change. P-T06-5: any judge-model change gates on a golden-set re-run (TPR/TNR re-measured) BEFORE unsupervised judging resumes. This is the S07.9 gate, NOT a BENCH-REG evaluation (R28: no registered-number edit; §7 win-rate math not invoked).

## Design (pre-registered)

- Set: the `golden-software` v1 seed — **26 cases** (20 planted defects across the route-table classes + 6 clean controls), `SeedGoldenSet()`.
- Path: each case through the REAL verify judge seam (`Compliance` axis-1 + `Sanity` axis-2, the S07.5 input slice, clean context) on the **opus-4-8** seat via the claude adapter. Judge-as-classifier.
- Metric: TPR (defects correctly flagged REVISE/ESCALATE/REOPEN) and TNR (clean controls correctly SHIP), computed vs the human labels. **Judge-reported rates statistically corrected, never taken raw** (S07.11 / deferral record).
- Length bias (P-T06-3): score correlation vs response length over the set → `RubricBundle.LengthBiasNote` measured.
- On success: rates land in the rubric bundle's `GoldenSetRates` (`Measured:true`) — an in-code seed **v2 bump**, gate-ratified like the v1 seeds; forward on `verify.round` rows.

## Spend projection + STOP LINE (pre-registered)

- opus-4-8 pricing (claude-api skill, 2026-06-24): **$5.00 / 1M input, $25.00 / 1M output.**
- Scale: 26 cases × 2 axes = **52 opus-4-8 calls** on SHORT planted artifacts (the golden cases are ~5–15 lines each).
- Per call: ~1.5–2.5K input (artifact + rubric + ACs + system) + ~0.3–0.8K output; adaptive thinking on opus-4-8 adds thinking tokens (billed as output).
- Projection: input ≈ 52 × 2.0K = 104K × $5/1M = **$0.52**; output (incl. thinking) ≈ 52 × 1.2K = 62K × $25/1M = **$1.56**. **Total projection ≈ $2.10** (the B3-3 precedent was $0.14/haiku; opus-4-8 is ~5× the input / 5× the output rate, on a bigger judge slice).
- **STOP LINE = $5.00** (coordinator-ratified 2026-07-22). If cumulative `total_cost_usd` reaches $5.00: STOP + record the partial + the stop, do not continue.

## Observation — EXECUTED 2026-07-22 (drain D1/F8)

Harness: `internal/adapters/claudecli/rider1_golden_test.go` (`SINET_RIDER1=1`, the committed claudecli-adapter Drive pattern from `live_smoke_test.go`), the 26-case `SeedGoldenSet()` through **simplified S07.5-shaped two-axis judge prompts** (Compliance + Sanity — replicas that collapse the axis taxonomy to a single `blocker` boolean, NOT the byte-identical findings/escalate/reopen_spec schema `stage.EngineJudge` sends; sufficient for the judge-as-classifier P-T06-5 signal, and a byte-identical-schema re-run is at the gate's discretion), clean context (artifact + ACs only, no transcript), on **claude-opus-4-8**. Judge-as-classifier: a case is flagged (REVISE) when EITHER axis returns a blocker.

- **TPR = 1.000, 95% Wilson [0.84, 1.00] (20/20)** — every planted defect (AC-blockers, sanity-blockers, check-integrity, research-not-run, reopen-spec, V0-malformed) correctly flagged.
- **TNR = 0.500, 95% Wilson [0.19, 0.81] (3/6)** — only 3 of 6 clean controls correctly passed; opus-4-8 FALSE-flagged 3 simple-but-correct artifacts (over-strict sanity/expert-standard on minimal code).
- **Length bias (P-T06-3):** point-biserial r = **−0.167** (artifact length vs flag decision) — WEAK, slightly negative (longer ≠ more-flagged); nowhere near the 0.10–0.76 style-bias warning. Measured, re-measure on every judge change.
- **Statistical correction (S07.11):** TPR/TNR are computed vs the ground-truth human labels (the judge-as-classifier confusion matrix) — never the judge's raw self-report. Wilson 95% CIs shown.
- **Spend = $1.8717** (full 26-case run), under the $2.10 projection and well under the **$5.00 STOP LINE** (never approached). Plus ~$0.21 from a failed-config first run ($0), a 1-case verify ($0.07), and a truncated 2-case partial ($0.14). **Rider-1 total ≈ $2.08.**

## Verdict — PASS with a genuine P-T06-5 finding

The opus-4-8 judge has **perfect recall (TPR 1.0)** on the planted defects but is **over-strict on clean controls (TNR 0.50)** — it flags simple, correct code (a conservative judge favouring false-positives over false-negatives, which is the SAFE direction for a quality gate but inflates rework on clean deliverables). This is the honest P-T06-5 signal for the ratified judge-model change; unsupervised judging resumes with this calibration on record (S07.9). Rates land in the rubric bundle **v2** (`GoldenSetRates{Measured:true}` + measured `LengthBiasNote`) — an in-code seed bump, content FLAGGED for gate ratification (D1). (Rider 2: `2026-07-22-rider2-serialize-by-deny-sonnet.md`, $0.31.) **Both riders executed; combined paid spend ≈ $2.39, both under their stop lines.** Host gate clean.
