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

## Observation — PENDING

Not executed. The coordinator runs the command above and records here: per-case results, TPR/TNR with Wilson 95% intervals, the point-biserial length bias, cumulative API-equivalent spend against the projection and the stop line, and the installed `claude` version (reported, never retargeted).

## Verdict — PENDING
