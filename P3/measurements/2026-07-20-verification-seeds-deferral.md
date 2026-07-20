# Deferral record — verification seed artifacts and their TPR/TNR measurements

**Packet:** P3-B2-3 (S07 verification) · **Date:** 2026-07-20 · **Discipline:** G3 Def.8 (seeded now, measured at the named phase; pre-registered bars before unsupervised reliance)

## What was seeded (versioned, operator-editable, in `internal/verify/seeds.go`)

| Artifact | ID / version | Content | Spec anchor |
|---|---|---|---|
| Entailment calibration set | `entailment-calibration` v1 | 10 planted claim/source pairs (5 supported, 5 unsupported), each judged against fetched-content excerpts | S07.4; G3 Def.4/Def.8 |
| Software golden set | `golden-software` v1 | 26 human-labeled cases: 20 planted defects across AC-BLOCKER / SANITY-BLOCKER / CHECK-INTEGRITY / RESEARCH-NOT-RUN / REOPEN-SPEC / V0-MALFORMED + 6 clean controls (TNR needs true negatives); the ratified 25–50 size is enforced by `Validate` | S07.10 |
| Software rubric bundle | `rubric-software` v1 | Axis-1 protocol (binary per AC + extractive quote + Unknown) + the four axis-2 probes with behavioral pass/fail anchors; judge pinned as the ratified planning-model CLASS (concrete engine pin lands with S08 selection, B3, as a version bump gated on a golden-set re-run per P-T06-5); length-bias note honestly UNMEASURED (P-T06-3); `golden_set.measured=false` | S07.10, S07.5 |

All three load via strict JSON (`LoadCalibrationSet`/`LoadGoldenSet`/`LoadRubric`, unknown fields rejected) and export via `WriteSeed` for gate review. A conformance test proves the planted V0 cases actually trip the real pre-gates and the clean controls do not.

## What measurement is due, at which phase

| Measurement | Needs | Due at | Pre-registered obligation |
|---|---|---|---|
| Entailment TPR/TNR over the calibration set (+ first real outcomes) | the B4 local tier (default seat Granite-Guardian-8B; CPU floor Flan-T5-0.8B for sampled checks — G3 Def.4) | **B4** | the TPR/TNR bar is pre-registered BEFORE entailment ever gates unsupervised; the same run sets `⚙ verification.entailment_sample_rate` (TBD-BRINGUP, default 0) and the mandatory-coverage (load-bearing) bar. Until then the machinery ships idle (software launch domain; activation additionally requires the web-research domain, v0.1). |
| Judge-as-classifier TPR/TNR over the golden set | a concrete judge engine (S08 selection, B3) + eval runners (S14, B5); measurable earliest at **B4** per the packet scoping call | **B4** | rates land in the rubric bundle (`golden_set`) and on every `verify.round` row; judge-reported rates are statistically corrected, never taken raw (S07.11). Every later judge-model change re-runs this gate before unsupervised judging resumes (P-T06-5). |
| Per-judge-model length-bias note | same golden-set run | **B4** | recorded in the rubric bundle; re-measured on every judge change (P-T06-3). |

## Flags

- **Seed CONTENT ratification is a B2-gate item** (operator reviews the three artifacts; `WriteSeed` dumps them for the gate).
- Canary scheduling + silence-alert wiring and the quarterly-drill scheduling/record are **S14 (B5)**; the shapes and checks landed at B2-3.
- The dead-man canary upgrades from the zero-model RESEARCH-NOT-RUN plant to the full V2 leg when the local tier lands (**B4**).
