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

## Observation — status (2026-07-22)

**Built + ready:** the `golden-software` v1 seed (26 cases: 20 planted defects + 6 clean controls, `SeedGoldenSet()`); the REAL V2 judge seam `verify.Judge` = `stage.EngineJudge` (Compliance axis-1 + Sanity axis-2, the S07.5 clean-context input slice, no transcript); the rubric bundle's `GoldenSetRates`/`LengthBiasNote` slots ready for the `Measured:true` v2 bump. Auth is available (claude 2.1.217 authenticated, subscription).

**Remaining paid leg (flagged, NOT run — $0 opus-4-8 spend on this rider):** a gated harness to run the 26 cases through `EngineJudge` on the **opus-4-8** seat does not exist as a committed test. `EngineJudge` drives its axes through the full stage `Skeleton.jsonSession` (clean-context ledger assembly + the claude adapter + verify-run choreography), so running the golden set requires wiring a complete stage Skeleton with the claude adapter targeting opus-4-8, creating a verify run per case, and building the judge input slice — a full-pipeline integration, plus the S07.11 statistical rate-correction and the rubric-bundle **v2 bump** (gate-ratified like the v1 seeds). This is the remaining bring-up execution of the pre-registered design; projection $2.10 / stop line $5.00 stand for when it runs.

## Verdict — pre-registered + machinery ready; execution FLAGGED (no paid call made)

The P-T06-5 obligation is pre-registered (this file, before any paid call) and every component exists (golden set + real judge seam + rubric v2 slots). The paid opus-4-8 golden-set run itself needs a judge-on-opus harness (full stage Skeleton + claude adapter) beyond this packet's execution budget — flagged to the B4 gate as the one remaining rider execution, NOT silently skipped. **Zero opus-4-8 spend on rider 1.** (Rider 2 executed + recorded: `2026-07-22-rider2-serialize-by-deny-sonnet.md`, $0.31.) The rubric bundle stays v1 `GoldenSet{Measured:false}` until this runs (honest — no fabricated rates); unsupervised judging on the opus-4-8 seat stays gated on the P-T06-5 re-run per S07.9.
