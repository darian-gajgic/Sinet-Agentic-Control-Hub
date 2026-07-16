# T11 — Evals, observability & the benchmark practice

**Wave:** B2 · **Depth:** FULL · **Report slug:** `evals-observability-benchmark`

## Scope
11.1 (full audit trail, retention → compaction), 11.2 (the added-value benchmark: blind A/B vs direct frontier use, pre-registered thresholds, sampled, opt-in budget), S2.1–S2.11 (traceability, live inspection, verdict records, human-decision records, cost observability, routing explainability, self-health-watching on local models, outside-world drift detection, auditable learning, queryable history), 5.7 (rubrics stay falsifiable via the benchmark), 7.3 (model-change revalidation needs eval sets).

## Why this gates the spec
"Validate before breadth" is inversion #3: the benchmark gate (15.3) blocks all expansion, so its methodology must be designed into v0, not bolted on. And a one-maintainer platform lives or dies by observability: problems must announce themselves (S2.7) on the free local tier.

## Core question
What observability and evaluation stack fits a self-hosted, single-maintainer, multi-user agent platform in mid-2026 — full per-run tracing with retention/compaction, live streaming inspection, self-health watching on local models, provider-drift detection, and a standing blind-A/B benchmark practice that keeps "does this platform beat direct model use?" permanently answered?

## Sub-questions
1. Tracing: OTel GenAI semantic conventions status mid-2026 (stable? adopted?), self-hosted trace stores (Langfuse, Phoenix-class, plain-SQLite trace schemas) — operational cost for one maintainer vs what each buys; what the adopted engine (per G1) already emits and how a platform captures it through the adapter.
2. What a *complete* run story requires (S2.1–S2.4): steps, tool uses, artifacts, verdicts with reasons, human decisions, injected knowledge (S4.6) — schema patterns; retention → compaction pipelines (11.1: full traces 6 months default, summaries forever).
3. Live inspection (S2.2): streaming current activity to surfaces (SSE-class patterns), progress semantics beyond started/ended.
4. Health watching on the free tier (S2.7): stall/loop/silence/spend-anomaly detection running on LOCAL models + deterministic monitors — what anomaly classes are cheaply detectable from traces; prior art for watchdog agents; false-positive discipline (alerts must stay rare enough to be read).
5. Provider/outside-world drift watch (S2.8, D3): canary/contract tests against providers and wrapped CLIs (format drift, limit-behavior drift, billing-status changes); who does this today; alerting before failures spread.
6. The benchmark practice (11.2/S2.11): current methodology for pairwise blind human comparison (position/order bias controls, sample sizing at low volume, pre-registered per-domain metrics and thresholds); the direct-arm protocol (same task run against the frontier model directly — using the requester's own subscription, opt-in); statistics honest at household scale (n is small — what claims are supportable?).
7. Regression evals for platform assets: eval sets per worker/rubric so 7.3's model-change revalidation and 5.7's rubric falsifiability are executable; cheap eval harnesses a solo maintainer will actually run; planted-defect tests for verification (does the judge catch known-bad?).
8. Routing explainability (S2.6/7.7): storing the plain-language reason per routing decision and analyzing routing quality over time (harvest N16 reference).
9. Queryable history (S2.10): making all of the above answerable by the conversational assistant (structured queries over traces/receipts) — schema and indexing implications.

## Constraints that bind this topic
Operating reality (health watching costs no allowance — local only), 11.1 (retention windows + compaction are design inputs), 11.3 (traces stay local; only summaries/verdicts/receipts enter snapshots), 3.4/S2.5 (cost observability per person is a first-class query), D5 (receipts report dollars as API-equivalent comparison).

## Harvest-map items to verdict
N21 (benchmark methodology: frozen baselines, blind scoring, pre-registered thresholds — bench-02 direct-arm results as the baseline arm), N16 (routing telemetry + collapse watch), C4 (window surfacing as observed state).

## Sources to prioritize
OTel GenAI spec status + adopters; self-hosted observability project docs and independent comparisons; eval-methodology practitioners (the current canon); LLM-comparison bias literature (2025–2026); postmortems where observability caught (or missed) agent failures.

## Decisions this feeds
G2: observability stack (adopt vs own-schema), watchdog design, benchmark protocol (pre-registered before v0 ships — it gates 15.3). Spec: trace schema, health monitors, benchmark section.
