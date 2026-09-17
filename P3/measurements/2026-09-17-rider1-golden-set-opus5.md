# Rider 1 — P-T06-5 golden-set run on the claude-opus-5 judge + length bias (S07.9, P-T06-3) — 2026-09-17

**PAID LEG. Pre-registered BEFORE any paid call (CONVENTIONS §28: measurements are Def.8 probe files, pre-registered). Executed by the COORDINATOR between packet P3-TQ-5's commit 1 (the mechanism) and commit 2 (the data).**

## Why this fires (S07.9 P-T06-5)

The 2026-09-17 gate record (`P3/gates/rework-sitting-gate.md` item B7 + §Answers) retires `claude-opus-4-8` from the judge seat and seats **`claude-opus-5`** — a judge-model change. S07.9 P-T06-5: the judge model is pinned per rubric version, and any judge-model change gates on a golden-set re-run with TPR/TNR re-measured **before unsupervised judging resumes**. P-T06-3 adds the per-judge-model length bias to the same re-measurement.

This is the **S07.9 gate, not a BENCH-REG evaluation**: no registered number moves, and `Spec/benchmark-preregistration-v1.md` is read-only here.

There is no advisory window in the code. `stage.newVerifier` gates every production judge construction on `verify.UnsupervisedJudgingGate`, which refuses an unmeasured or mismatched pin and turns the refusal into an S07.7 verification-infrastructure decision card on every verification. So the measurement precedes the seat flip: rubric-software **v3 lands only together with these numbers**, and the seat flip lands with it.

## Design (pre-registered — identical to the 2026-07-22 run, only the seat moved)

- **Set:** the `golden-software` v1 seed — **26 cases** (20 planted defects across the route-table classes + 6 clean controls), `verify.SeedGoldenSet()`. Unchanged.
- **Path:** each case through `TestRider1GoldenSetOpus` on **`claude-opus-5`** via the committed claudecli adapter, clean context (artifact + ACs only, no transcript), through the **simplified S07.5-shaped two-axis prompts** (Compliance + Sanity) — replicas that collapse the axis taxonomy to a single `blocker` boolean, NOT the byte-identical findings/escalate/reopen_spec schema `stage.EngineJudge` sends. The judge-as-classifier signal P-T06-5 requires (does either axis flag a blocker?) is preserved, so the measurement stands as the S07.9 gate; a byte-identical-schema re-run is at the gate's discretion. Repeating the 2026-07-22 design byte for byte is what makes the two results comparable.
- **Metric:** TPR (planted defects correctly flagged) and TNR (clean controls correctly passed) **vs the human labels**, never the judge's raw self-report (S07.11 statistical correction), with **Wilson 95%** intervals.
- **Length bias (P-T06-3):** point-biserial correlation of artifact length against the flag decision.

## Command

```
SINET_RIDER1=1 go test -p 1 -count=1 -run TestRider1GoldenSetOpus -v ./internal/adapters/claudecli/
```

Subscription `claude` login (setup-token): **$0 cash**; the API-equivalent is read from the harness's `total_cost_usd`. Serial (`-p 1`), one package at a time, per the operator's serial-tests directive.

## Spend projection + STOP LINE (pre-registered)

- `claude-opus-5` pricing, live-verified 2026-09-17 against https://platform.claude.com/docs/en/models/overview: **$5.00 / 1M input, $25.00 / 1M output** — the same rate card as `claude-opus-4-8`, so the 2026-07-22 projection carries over unchanged.
- Scale: 26 cases × 2 axes = **52 calls** on short planted artifacts (~5–15 lines each).
- Projection: input ≈ 52 × 2.0K = 104K × $5/1M = **$0.52**; output incl. thinking ≈ 52 × 1.2K = 62K × $25/1M = **$1.56**. **Total ≈ $2.10 API-equivalent.**
- **STOP LINE = $5.00** — the 2026-07-22 line, unchanged. At cumulative `total_cost_usd` ≥ $5.00 the harness halts and the partial is recorded.
- **A partial run does NOT clear the gate.** Rubric v3 stays unlanded and packet commit 2 does not land.

## Pre-registered readings (what each number will mean, decided before seeing any)

- The v3 **catch floor** = the **Wilson 95% lower bound of the observed TPR at n = 20** (`internal/evals/floors.go` `rubricSoftwareCatchFloor`; S14.8 ¶2/¶5 floors per (asset, VERSION), measurement-derived).
- The **TNR** Wilson lower bound is carried **report-only** (`rubricSoftwareTNR`) — a companion reading, never a gate.
- **|r| ≥ 0.10** on the length bias is a **P-T06-3 FINDING presented at the gate**, not a rider failure.
- A **TPR lower bound below the registered v2 floor 0.84** is a FINDING presented at the gate **before** v3 lands.

## What lands afterwards (packet P3-TQ-5 commit 2)

`verify.SeedSoftwareRubric()` → **v3** (`JudgePin` naming `claude-opus-5`, `GoldenSet{TPR, TNR, Measured: true, MeasuredOn}`, `LengthBiasNote` measured on `claude-opus-5`, the four axis-2 items VERBATIM from v2); `internal/evals/floors.go` → the v3 floor row with its basis naming `claude-opus-5`, this file and "Wilson", `Ratified: false`; the `internal/evals/evals_test.go` probe literals only if the Wilson bound moved off `0.84`. Optionally the S14.8 record `evals.JudgeRemeasurement(...)`. The v2 floor row already registered in live databases is untouched (`EnsureFloorsRegistered` is insert-once).

## Observation — EXECUTED 2026-09-17 (coordinator; installed `claude` 2.1.274, reported per CONVENTIONS §10, never retargeted)

Raw log: `P3/measurements/2026-09-17-rider1-golden-set-opus5.log` (26 per-case lines). Full set, no partial, wall 414.8 s. Harness line: `RIDER1_RESULT judge=claude-opus-5 cases=26 TPR=1.000[0.84,1.00] (20/20) TNR=0.500[0.19,0.81] (3/6) length_bias_r=-0.167 total_cost=$2.3714 stop=$5.00`.

- **TPR = 1.000, Wilson 95% [0.84, 1.00] (20/20)** — every planted defect flagged, across all six classes: AC-BLOCKER (g-07…g-12), SANITY-BLOCKER (g-13…g-18), CHECK-INTEGRITY (g-19, g-20), RESEARCH-NOT-RUN (g-21, g-22), REOPEN-SPEC (g-23, g-24), V0-MALFORMED (g-25, g-26).
- **TNR = 0.500, Wilson 95% [0.19, 0.81] (3/6)** — clean controls g-01, g-02, g-04 passed; **g-03, g-05, g-06 over-flagged, by axis 2 alone in all three cases** (`a1=false a2=true`). The same over-strictness, in the same direction, as the 2026-07-22 run on the previous judge seat.
- **Length bias (P-T06-3): point-biserial r = −0.167** — WEAK and slightly negative (longer artifacts are not more flagged), **identical to the 2026-07-22 value**.
- **Statistical correction (S07.11):** TPR/TNR are computed against the ground-truth human labels, never the judge's self-report; Wilson 95% intervals shown.
- **Spend = $2.3714 API-equivalent** (per-case $0.080–$0.106) vs the $2.10 projection and the **$5.00 STOP LINE — never approached**. $0 cash (subscription lane).

**Pre-registered readings, applied as written:**

- Catch floor = the TPR Wilson lower bound at n = 20 = **0.84**. It lands exactly on the registered v2 floor, so the `internal/evals/evals_test.go` `0.84` literal and its `0.90`/`0.70` probes **do not move**.
- TNR lower bound **0.19**, carried **report-only**.
- **|r| ≥ 0.10 → a FINDING for the gate, not a rider failure.** r = −0.167 clears that bar, so it is recorded as a finding: it is the same value the B4 gate already accepted as weak and in the safe direction, reproduced on a different judge seat, which is evidence the measurement is stable rather than evidence of a new problem.

## Verdict — PASS

Full 26-case run, under the stop line, on the pinned seat. The claude-opus-5 judge has **perfect recall on planted defects and is over-strict on clean controls** — conservative, which is the safe direction for a quality gate and inflates rework rather than letting defects through. The gate that P-T06-5 requires is cleared, so unsupervised judging resumes under **rubric-software v3** (`JudgePin` = claude-opus-5, `GoldenSet{Measured: true, MeasuredOn: 2026-09-17}`) and the seeded floor is re-keyed to v3 with this run as its basis. Operator ratification of the v3 numbers is flagged to the packet's gate; the registered floor row ships `ratified=false`.

