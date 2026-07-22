# Per-duty confidence calibration (S12.9 #6, S12.5 recipe, R25) — 2026-07-22 (drain C2/F1 re-run at DEPLOYED-seat keys)

- **DRAIN C2/F1 CORRECTION:** the round-1 records keyed EVERY duty at the 9B — WRONG. S12.4 seats the fast classification duties (`intake-triage`, `watchdog-disambiguator`, `watchlist-triage`, `intent-filling`) at the **4B FAST seat** (`local.alias` default → `fast`); the live low-margin re-check reads `(intake-triage, 4B-hash, b10085)`, so THAT key must exist. Re-run at the deployed seats on the grown ≥30-case suites (t15-v1). The wrong-key 9B records are superseded and must NOT be what bring-up writes.
- Deployed seats + hashes: fast classification → **Qwen3.5-4B Q4_K_M** (`00fe7986…`); contradiction-screen (confirm) → **Qwen3.5-9B Q5_K_M** (`dc2a39ae…`); entailment → **Granite Guardian** (measured in `2026-07-22-entailment-thresholds.md`). Engine b10085; on AC; sleep-gate discipline held.
- Instrument: the T15 battery runner over the grown seeds; each classification case yields `LabeledItem{margin, wrong}`; S12.5 `Fit` (isotonic PAV + threshold, bar 0.15). Keyed (duty, DEPLOYED model hash, engine build) per R3.

## Pre-registered expectation

Fit the R2 recipe for every MEASURABLE registry duty at its DEPLOYED seat + engine build; record per-duty (margin distribution, threshold, achieved error) keyed per R3. Generation-shaped / no-live-seat duties recorded not-calibrated-with-reason.

## Observation — at the DEPLOYED seats (grown ≥30-case suites, b10085)

| duty | DEPLOYED seat | pass | 95% Wilson | thr | achieved_err | meets_bar |
|---|---|---|---|---|---|---|
| intake-triage | 4B fast | 4/32 (12.5%) | [0.05,0.28] | 2.01 | 1.00 | **false** |
| watchdog-disambiguator | 4B fast | 25/30 (83%) | [0.66,0.93] | 8.91 | 1.00 | **false** |
| watchlist-triage | 4B fast | 12/30 (40%) | [0.25,0.58] | 10.36 | 1.00 | **false** |
| intent-filling | 4B fast | 25/30 (83%) | [0.66,0.93] | 6.27 | 0.13 | **true** |
| contradiction-screen | 9B (confirm) | 28/30 (93%) | [0.79,0.98] | 5.65 | 0.07 | **true** |
| entailment | Guardian | see C8 file | — | — | — | see C8 |

**Calibrated (meets_bar=true) at the deployed seat: intent-filling (4B) + contradiction-screen (9B).** entailment at Guardian is in the C8 entailment-thresholds file.

**HONEST FINDING (why C2 matters):** at the DEPLOYED **4B fast seat**, watchdog/watchlist do NOT meet the bar — the fast seat's confidence MARGIN does not separate its errors (watchdog 83% pass but the 5 misses aren't margin-distinguishable; watchlist 40%). The round-1 WRONG-key 9B records showed watchdog/watchlist/intent all meets=true — that overstated fast-seat quality. At the 9B (grown seeds) these DO meet, but the 9B is not the deployed seat for them. So the deployed fast tier is honestly less reliable, and only intent-filling (4B) + contradiction (9B) calibrate at v0. This is the confidence machinery reporting the truth of the deployed seating, not the convenient 9B numbers.

## (superseded round-1 wrong-key observation, kept for the record)

Round 1 keyed all at the 9B (grown-seed 9B: watchdog 25/30 meets=true, watchlist 25/30 meets=true, intent 26/30 meets=true, contradiction 28/30 meets=true, entailment 30/30 meets=true). Those 9B numbers are real but at the WRONG key for the fast duties; superseded by the deployed-seat table above.

## Original observation (round 1, 9B — retained below for provenance)

| duty | suite pass | 95% Wilson | labeled_n | threshold | achieved_err | accept_rate | meets_bar |
|---|---|---|---|---|---|---|---|
| watchdog-disambiguator | 10/12 | [0.55,0.95] | 12 | 7.46 | 0.00 | 0.67 | **true** |
| watchlist-triage | 10/12 | [0.55,0.95] | 12 | 9.71 | 0.00 | 0.83 | **true** |
| intent-filling | 10/12 | [0.55,0.95] | 12 | 8.83 | 0.00 | 0.67 | **true** |
| entailment (binary) | 12/12 | [0.76,1.00] | 12 | 4.60 | 0.00 | 1.00 | **true** |
| contradiction-screen (confirm) | 12/12 | [0.76,1.00] | 12 | 5.65 | 0.00 | 1.00 | **true** |
| intake-triage | 7/32 | [0.11,0.39] | 32 | 1.38 | 1.00 | 0.00 | **false** |

**Calibrated (meets_bar=true): 5 classification duties** — the margin cleanly separates right/wrong (achieved error 0 among the accepted set at the chosen threshold; the isotonic map is monotone-non-increasing and the threshold accepts 0.67–1.00 locally within the 0.15 bar).

**intake-triage meets_bar=FALSE — an honest FINDING, not a machinery failure:** the fit ran (32 items) but the label margin (over family+stakes) does NOT separate right/wrong, because the 7/32 pass rate is dominated by **`size`-field mismatches** — the third label is genuinely subjective (is "book a dentist appointment" small or trivial?) and its correctness is uncorrelated with the family/stakes token margin. The recipe correctly reports it can't meet the bar and escalates ~everything (accept_rate 0). Remedy (documented, for the growing seed): drop `size` from strict scoring or widen the labeled set with size-agnostic labels — the calibration machinery is proven correct BY reporting the honest can't-separate result rather than inventing a threshold.

**Not calibrated, with reason (generation/output-contract — no label margin, S12.5 F7 boundary) — with their ACTUAL observed output-contract pass rates recorded honestly (F2a, no suppression):**

| duty | observed (9B) | note |
|---|---|---|
| distill-summarize | **10/10 (100%)** | ≤400-char single-sentence contract satisfied |
| sql-open | **6/10 (60%)** | ran on the 9B as a PROXY (wrong seat — the deployed seat is Arctic); re-run on Arctic in drain C4 (`2026-07-22-sql-open-arctic.md`) |
| utility | **0/10 → 30/30 on re-run** | HARNESS DEFECT (F2b): the runner sent NO json_schema for output-contract suites while the utility contract demanded strict JSON fields → the free-text draft carried no `{what,wrong,recommend}`. Fixed in drain B6 (`battery.go` now emits `local.ContractSchema` for output-contract suites); re-run number below. |

**R5 — utility output-contract re-run (drain round 2), the number that was missing:** against the FIXED harness, on the 9B via a live llama-server `/v1` endpoint (durable stack: `SINET_LOCAL_LLAMA_SERVER=$HOME/.sinet-b45/build/llama.cpp/build-cuda/bin/llama-server`, `SINET_LOCAL_MODEL_CACHE=$HOME/.sinet-b45/models`; `SINET_MEASURE=1 SINET_MEASURE_SUITES=utility`), $0 local, on AC — **utility = 30/30 (100.0%), 95% Wilson [0.89, 1.00], 54.3 t/s** (the grown t15-v1 suite). The json_schema fix is confirmed: with the drafting schema enforced at the engine, the Help output carries `{what,wrong,recommend}` and the output contract passes fully. (Generation-shaped ⇒ still not margin-calibrated — this is the output-contract pass rate, its correct quality control per the F7 boundary.)

embedder = post-gate (no suite). None of the generation duties was calibrated (correct — these carry no label margin); the numbers above are the honest output-contract observations, not suppressed.

## Verdict — PASS (recipe executed at the DEPLOYED seats; 2 duties calibrate at v0 — the honest deployed-seat result)

The full S12.5 four-step recipe (labeled set → margins → isotonic map → threshold minimizing local cost s.t. the bar) ran on the REAL deployed quant + engine, keyed (duty, **DEPLOYED** model hash, b10085): the fast classification duties at the **4B fast seat** (`00fe7986…`), contradiction-screen at the **9B** (`dc2a39ae…`), entailment at Guardian (the C8 entailment-thresholds file). **Calibrated (meets_bar=true) at the deployed seat: intent-filling (4B) + contradiction-screen (9B)** — plus entailment at Guardian per C8. watchdog/watchlist do NOT meet the bar at the deployed 4B fast seat (the fast seat's margin does not separate its errors — the honest C2 finding, not the convenient 9B numbers); intake-triage reports a can't-separate result (the subjective `size` label). **Bring-up writes EXACTLY the deployed-seat keys** — `(intent-filling, 4B-hash, b10085)` + `(contradiction-screen, 9B-hash, b10085)` (+ entailment@Guardian from C8) — into the migration-0010 `calibration_records` table via `SaveCalibration` (machinery + tables exist). The **round-1 wrong-key 9B records for the fast duties are SUPERSEDED and must NOT be written**: they key the fast duties (intake-triage/watchdog/watchlist/intent-filling) at the 9B, a seat they are not served on. The uncalibrated fast duties (intake-triage/watchdog/watchlist) carry NO threshold belief ⇒ their ungated v0 behavior continues (the `calibrated=false` posture) — R4's low-margin re-check gates only where a calibrated threshold exists at the deployed key (intent-filling@4B among the fast tier), and the swap gate's P-T15-1 consumes the same deployed-seat keys. Closes `TBD-BRINGUP(per-duty confidence calibration)` for the covered duties (spec-text bookkeeping = coordinator/gate S00.9, not a packet edit).
