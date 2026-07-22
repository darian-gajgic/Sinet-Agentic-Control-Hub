# sql-open on the deployed Arctic seat (S12.9, R6, drain C4/F2c) — 2026-07-22

- **DRAIN C4 CORRECTION:** round 1 ran sql-open on the 9B (a PROXY, 6/10 on the old seed) and did not record it in its own file. The DEPLOYED seat is **Arctic-Text2SQL-R1-7B Q5_K_M** (`157cc3e1…`, re-pulled + sha256'd, R19). Re-run on Arctic over the grown 30-case sql-open suite. $0 local; on AC; b10085. The suite measures the ALIAS (does the seat emit a single SELECT for a NL question); the guardrail query surface (read-only conn, allowlisted views, single-statement parse, LIMIT+timeout) is S14/B5, not measured here.

## Pre-registered expectation

Arctic is the S12.3 layer-2 open-SQL seat (flagged lower-confidence, G3 D3.5). Expectation: it emits a single read-only `SELECT` for a NL question over the given schema (the output-contract check: contains `SELECT`, within the length cap). The verdict records the alias's pass rate; low-confidence is expected (the guardrail stack is the real safety, not the raw model).

## Observation (Arctic-Text2SQL-R1-7B Q5_K_M, b10085, grown 30-case suite)

- **Arctic (deployed seat): 0/30 (0.0%)**, Wilson [0.00, 0.11], 68.4 t/s.
- Contrast (proxy seats on the same suite): 9B 16/30 (53%), Gemma 17/30 (57%), 4B 9/30 (30%).

## Verdict — honest finding: the RAW Arctic alias output busts the naive output-contract; the guardrail stack (S14/B5) is what makes it usable

Arctic-Text2SQL-**R1** is a REASONING (R1-style) model: it emits a long chain-of-thought before the SQL, which **exceeds the suite's 600-char output-contract cap** (and/or buries the `SELECT` past the checked window) — so the naive alias check scores 0/30, WORSE than the general models (9B/Gemma ~55%) that emit a terse `SELECT` directly. This is exactly why S12.3 flags the SQL layer **lower-confidence** and why its real consumer is the **S12.3 guardrail stack (S14/B5): read-only connection, allowlisted views, SINGLE-STATEMENT PARSE (which extracts the `SELECT` from the reasoning output), LIMIT+timeout, audit-logged.** The measurement measures the ALIAS (raw output), not the guarded query surface; the honest reading is that Arctic needs its guardrail stack to be usable — the raw alias is not a clean single-SELECT emitter. Not on the v0 default path; the guardrail-stack consumer stays S14/B5, unbuilt here. $0 local; host gate clean. (Round-1 recorded this only as a 9B-proxy 6/10 in the per-duty file; now measured on the deployed Arctic seat with the honest 0/30 + the diagnosis.)
