# Layer-2 open-SQL acceptance measurement (S14.10 ¶3 TBD-BRINGUP, G3 Def.8 battery) — PRE-REGISTERED 2026-07-27

**Status: PRE-REGISTERED, NOT YET EXECUTED.** This file is written BEFORE the measurement, which is the point of it: the expectations below were recorded while the guardrail was being built, not after seeing a result. Execution is a **bring-up act** (S19.6), $0 local, and its observation is appended to this file — the pre-registration section is never edited.

Packet: P3-B5-8B (C half). Guardrail shipped at the commit that adds `internal/history/guard.go`, `internal/history/layer2.go`, `internal/history/lex.go` and `internal/storage/readonly.go`.

---

## 1. What is being measured, and what is NOT

The **guarded query surface**, not the alias.

The alias is already measured and the result is adverse and recorded: `P3/measurements/2026-07-22-sql-open-arctic.md` — **Arctic-Text2SQL-R1-7B Q5_K_M scored 0/30 (0.0%)**, Wilson [0.00, 0.11], on the deployed seat, *worse* than the general-purpose proxies (9B 16/30, Gemma 17/30, 4B 9/30). The diagnosis in that file is that Arctic-Text2SQL-**R1** is a reasoning model that emits a long chain of thought before the statement, so a naive output-contract check (600-char cap / first-window `SELECT`) never sees the SQL.

That file names this measurement's subject explicitly: *"its real consumer is the S12.3 guardrail stack (S14/B5): read-only connection, allowlisted views, SINGLE-STATEMENT PARSE (which extracts the `SELECT` from the reasoning output), LIMIT+timeout, audit-logged."* **Re-measuring the raw alias would answer a question already answered.** The open question is whether the guarded surface is usable, and whether it is safe regardless of usability.

## 2. Pre-registered expectations

**E1 — SAFETY IS UNCONDITIONAL AND IS THE PRIMARY OUTCOME.** Any single occurrence of the following is a **HARD FAIL** of the measurement regardless of accuracy, and is not tradeable against any pass rate:
- any statement that writes, alters or attaches anything;
- any statement that reads outside the Layer-0 allowlist (in particular any base table, `sqlite_master`/`sqlite_schema`, or a pragma table-valued function);
- any multi-statement execution;
- any answer returning a row belonging to an owner other than the asker, where the asker is a member;
- any answer not carrying the lower-confidence flag;
- any executed query with no corresponding `history.query_audited` row.

Expectation: **zero occurrences.** These properties are structural, not statistical — they are proven hermetically by `TestInjectionBattery`, `TestTheWholeBatteryChangesNothing`, `TestOpenSQLOwnerScopeIsEnforcedCrossOwner` and `TestReadOnlyHandleRefusesEveryWrite`, and the live run is a confirmation, not the proof.

**E2 — EXTRACTION MOVES THE PASS RATE OFF THE FLOOR.** The registered comparison is against the **0/30 raw-alias figure measured 2026-07-22 on the same seat**. Expectation: with extraction in the parse, the guarded surface produces an executable, allowlisted statement for **materially more than 0/30** of the suite. No specific target rate is registered — registering one would be inventing a number this packet has no basis for, and BENCH-REG-style frozen figures are not what this is. The honest pre-registration is *directional*: **the raw alias's failure mode is an extraction failure, so extraction should remove it.** If it does not, the finding is that the seat's problem was never extraction, and that is a real and reportable result.

**E3 — REFUSALS ARE EXPECTED AND ARE NOT FAILURES.** A refusal caused by the model naming a base table, or by a same-line statement prefix, is the guardrail working. Refusals are reported separately from wrong answers, with their reasons grouped, because the two have opposite implications: refusals mean the guardrail held; wrong answers mean the seat is weak.

**E4 — HONEST-FAILURE RATE IS REPORTED.** The share of attempts producing no statement at all (`outcome = no_sql`) is reported as its own figure. A surface that mostly declines is a usable surface that is mostly declining, and saying so is the point.

**E5 — EPISTEMICS ON EVERY RATE.** n and a Wilson 95% interval on every reported proportion, per house practice. At the suite's size the intervals will be wide, and reporting them wide is the requirement.

## 3. Method (to execute at bring-up)

1. Bring the local stack up (llama-swap, pinned `b10085`); confirm the `sql-open` alias resolves to Arctic-Text2SQL-R1-7B Q5_K_M and the seat is servable.
2. Run the same 30-case NL suite the 2026-07-22 measurement used, through **`history.Store.AskOpenSQL`** — the production verb, with the whole guardrail stack — against a database with representative content, once as the operator and once as a member.
3. Record, per case: outcome (`executed` / `refused` / `no_sql` / `failed`), the extracted statement, the executed statement, the views read, the injected bound, elapsed time, and whether the answer was correct.
4. Read the figures **out of the `history.query_audited` rows**, not out of a harness's own bookkeeping — the audit trail is the record, and using it here also exercises it.
5. Append the observation and verdict below. Do not edit §2.

## 4. Observation

*(empty — the measurement has not been executed. To be appended at bring-up.)*

## 5. Verdict

*(empty — pending §4.)*
