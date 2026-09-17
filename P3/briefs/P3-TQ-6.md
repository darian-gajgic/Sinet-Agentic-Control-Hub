# P3-TQ-6 — a ladder step-contract FAIL must route, and a judge verdict that contradicts a V1 contract is CHECK-INTEGRITY

**Grounded 2026-09-17.** Single-use brief for the executor and the evaluator. Truth is the spec (`Spec/drafts/S07-verification-quality.md` S07.3/S07.5/S07.6/S07.7/S07.8/S07.11, `S06-intake-pipeline.md` S06.6) plus the code at `eb7594b` with this grounding commit's inert surface applied (line numbers below are post-grounding); every claim carries its file:line. No prior brief was read as truth. The just-landed P3-TQ-3 record is `P3/CONVENTIONS.md` §74 and its code is on `main` (`internal/verify/contract.go`, `bootstrap.go`, `pipeline.go`).

**The two findings this packet closes** (P3-TQ-3 evaluation, `P3/STATE.md` 2026-09-17 cont. 20 and queue row P3-TQ-6):

1. **Pre-existing P46-class gap on the check-pack path.** `stepContracts` decides every PLAN step's Done-when contract from check outcomes keyed by `StepID` (PASS / FAIL / UNVERIFIABLE-HERE / N-A), but nothing reads `V1Result.Steps` except the `verify.v1` event row and the judge's evidence slice; `ComputeVerdict` reads findings only. A ladder contract FAIL therefore has no verdict effect and reaches no human sink: the "finding that died in a log" S07.7 forbids. The bootstrap path (TQ-3) already materializes ONE AC-BLOCKER finding per FAIL via `contractFinding`. Reuse it.
2. **S07.5's disagreement sentence is realized only for `ACKey`-bound check outcomes.** `ValidateAxis1` keys on `CheckOutcome.ACKey`. A judge axis-1 PASS on AC-k while the V1 contract of the step that COVERS AC-k is FAIL (either path) raises nothing. TQ-3's T8 pins only that the judge cannot flip the contract; the disagreement itself must become a CHECK-INTEGRITY finding routed to the CHECK-INTEGRITY card and never into the REVISE drain (CONVENTIONS §15).

**Coordinator rulings that bind:** OQ-1 of TQ-3 stands: `decideFromTree` is NOT applied to the ladder path's N-A steps (the ladder's N-A = "a suite ran and no check covered this; the judge weighs it at V2"). Routing + disagreement only. No new card kind, no new route-table category, no ⚙, no migration.

---

## 1. Code facts (what is actually there)

| # | Fact | Evidence |
|---|---|---|
| C1 | `RunV1` runs the ladder cheap-first; a `CheckFailed` outcome is recorded with exit code and evidence ref and **mints no finding**. Only a quarantine skip (note, CHECK-INTEGRITY) and a runner failure (blocker, CHECK-INTEGRITY, criterion `CHECK-INTEGRITY`, anchor `check:<id>`) append to `V1Result.Findings`. | `internal/verify/v1.go:407-493` (failure branch `:472-486`; quarantine `:448-452`; runner failure `:465-469`) |
| C2 | `stepContracts(checks, steps, firstFailure)` emits exactly ONE `StepContract` per plan step, in plan order: PASS when every bound check passed, FAIL on any `CheckFailed` (`AttributedTo` = the failing check id), UNVERIFIABLE-HERE when a bound check is blocked/quarantined/runner-failed and none failed, N-A when no check binds the step. `Category` derives from the outcome state via `checkCategory` (defaults `CatACBlocker`), `Route = RouteTable[Category].Sink`. **`Detail` is never set on this path.** | `v1.go:499-548`; `StepContract.Detail` at `:258-262` |
| C3 | `Check.FindingCategory` ("the route-table category a failure of this check escalates under") is validated against `RouteTable` and never consumed anywhere else. | `v1.go:107-111, :170-172`; `grep FindingCategory` non-test hits only `Validate` |
| C4 | `bootstrapV1` decides each contract from the tree and, for every FAIL, appends `contractFinding(sc, s, coverage)` to `V1Result.Findings` **before** the drain records the `verify.v1` row. This is the landed materialization site and shape for the bootstrap path. | `internal/verify/bootstrap.go:127-152` (`:147-149`); `pipeline.go:478-486` |
| C5 | `contractFinding` = `{Severity: blocker, Category: AC-BLOCKER, Criterion: coveringCriterion(step.ID, coverage), Anchor: "step:"+sc.StepID, Text: "Step %s of the approved plan is not finished. It was agreed done when: %s. %s The approved plan is what this work is measured against, so it is recorded as not done rather than passed."}` with `%s` #3 = `sc.Detail`. `coveringCriterion` returns the lowest-numbered `AC-<n>` whose coverage entry names the step, `""` when none. Key = `criterion|anchor|category` (`Finding.Key`). | `internal/verify/contract.go:665-718`; `verify.go:103-105` |
| C6 | The drain's pack branch: `RunV1(ctx, pack, v.Runner, CheckRequest{...}, in.Steps, in.Coverage, v.now(), v.Settings)` → `record.V1`, `rec.RecordV1`. (The `coverage` parameter and its pass-through are this grounding's inert surface; `RunV1` ignores it today.) | `pipeline.go:487-506`; `v1.go:407` |
| C7 | Judge pass: `BuildJudgeInput` (V1 rides the slice as `verify/v1-outcomes`) → `Compliance` → `v1Facts = v1res.ACOutcomes()` (keyed by `ACKey` only) → `verdicts, integrity := ValidateAxis1(...)`. `ValidateAxis1` forces an AC executed at V1 (mech PASS/FAIL) to the mechanical outcome, sets `FromV1`, and on a judge disagreement sets `Disagreement=true` and raises `{blocker, CHECK-INTEGRITY, Criterion: AC-k, Anchor: "check:AC-k"}`. | `pipeline.go:510-524`; `v2.go:250-296` (`:269-286`); `v1.go:284-292` |
| C8 | Raw findings = `ax1.Findings` + `UnknownEscapes` + `integrity` + `ax2.Findings` + `v1res.Findings` + round-1 research notes + requester comments → `validateFindings` (numbers `[F1..Fn]`; demotes any blocker whose `Criterion` is not `CHECK-INTEGRITY`, a pinned `AC-k`, or a rubric item id; suppresses NEW notes after round 1 except the posture disclosure) → `ComputeVerdict` (**CHECK-INTEGRITY excluded from the blocker/note count**). | `pipeline.go:544-576`; `v2.go:347-378` (`valid` at `:348`), `:389-416` (`:398`) |
| C9 | CHECK-INTEGRITY raiser: for every blocker-severity CHECK-INTEGRITY finding of the round, once per finding key, `raiseIntegrity` → `esc.Raise(CatCheckIntegrity, Summary "check-integrity: "+f.Text, Findings [f], Quarantined)`; the drain continues. `raiseIntegrity` quarantines a pack check **only when `c.ACKey == f.Criterion`**. Cards land in `out.IntegrityCards` and as durable `asks` rows with a `verify.escalation` event. | `pipeline.go:601-618, :904-929` (`:906-921`); `escalate.go:295-384` |
| C10 | `reviewable` strips every CHECK-INTEGRITY finding except the posture disclosure from the S13 review stream; the retry package with the S13 sink comes from `DrainOpen` (review comments), without it from `BuildRetryPackage` = `blockers(findings)` (severity only, category-blind). `stop.observe` keys convergence on `keySet(blockers(roundFindings))`, also category-blind. | `review.go:76-85`; `pipeline.go:714-726`; `rework.go:37-45, :133-156`; `verify.go:108-116` |
| C11 | Route table: `CatCheckIntegrity` → decision card, approval SLA, raisers include "judge–check disagreement"; choices `fix_suite | waive_check | cancel`. Total over 8 categories; no new category admissible without S00.9. | `escalate.go:72-88` (`:83`), `:158-165` (`:161`) |
| C12 | `VerifyInput.Coverage` (AC key → owning step ids) is the approved PLAN's map, built at ONE production site (`skeleton.go:1124`); `intake.Plan.Coverage` at `artifact.go:303`. Every harness `input()` in `internal/verify/*_test.go` leaves it nil unless a test sets it (`tq3Input`). | `pipeline.go:136-139`; `internal/stage/skeleton.go:1124`; `internal/intake/artifact.go:301-303`; `harness_test.go:247-252` |
| C13 | **Production packs bind no step and no AC.** `packChecks` maps captured lint/build/test to `{ID, Stage, Argv, FindingCategory: AC-BLOCKER}` only: "None of them is an ACCEPTANCE check". Every landed production `Check` therefore has `StepID == ""`, so `stepContracts` marks every plan step N-A on the live pack path. Fact C1 then means **a failing captured build today reaches the judge's evidence slice and the `verify.v1` row, and nothing else**: with a pass-all judge the round is SHIP and `VerifyLedgerItems` verifies. See §9 OQ-1. | `internal/shell/project_seams.go:499-534` (`:505-508`, `:527-530`); `v1.go:499-508`; `pipeline.go:620-636` |
| C14 | Recording: `RecordV1` marshals the `V1Result` by value at call time (so a finding appended AFTER it is absent from the `verify.v1` row); `RecordRound` carries `rec.Findings` and `rec.Axis1` (Pass/Failed/Unknown lists) verbatim. | `record.go:156-158, :162-198`; `rework.go:80-119` |
| C15 | No non-test code outside `v1.go`/`bootstrap.go` reads `V1Result.Checks` or `CheckOutcome`; no consumer outside `internal/verify` parses a `step:` anchor (review's `ParseFindingAnchor` treats a non-`path:NNN` anchor as file-level and never refuses it). | `grep` over `internal/`; `internal/review/anchor.go:135-161` |

## 2. Requirements (each with its S-ref)

| # | Requirement | S-ref |
|---|---|---|
| R1 | **Pack-path materialization.** In `RunV1`, immediately after `res.Steps = stepContracts(...)` and before return, every `StepContract` in state FAIL appends exactly ONE `contractFinding(sc, step, coverage)` to `res.Findings`, where `step` is the `intake.Step` with `ID == sc.StepID`. Same helper, same shape, same key as the bootstrap path (C4/C5): `AC-<lowest covering>|step:<id>|AC-BLOCKER`. The finding is therefore on the `verify.v1` row (C14) and in the round's raw findings (C8). | S07.3 stage contracts + declared routes (line 49); S07.7 sink rule (line 91); S07.5 blocker citation (line 71); S06.6 coverage map (line 106); §74 |
| R2 | **Nothing else mints.** PASS, UNVERIFIABLE-HERE and N-A contracts produce no finding of any category anchored `step:<id>`. The pre-existing quarantine and runner-failure findings (anchored `check:<id>`) are untouched. | S07.3 ("definition-of-cannot-be-done-here"); S07.7 |
| R3 | **A ladder FAIL says why.** `stepContracts` sets `StepContract.Detail` on a FAIL contract to plain words naming every check bound to the step whose outcome is `CheckFailed`, e.g. `The automated check "lint" did not pass.` (two: `The automated checks "lint" and "unit" did not pass.`). No internal tokens (`check:`, `step:`, `S07`, `Spec`, `§`, category names). `contractFinding` already embeds `Detail` in the finding text (C5), so the requester reads which check failed. UNVERIFIABLE-HERE and N-A contracts on the ladder path stay as they are (OQ-1 of TQ-3 stands). | S07.11 recorded with reasons (line 146); CONVENTIONS §38 plain words |
| R4 | **The disagreement detector.** `ContractDisagreements(verdicts []ACVerdict, contracts []StepContract, coverage map[string][]string) ([]ACVerdict, []Finding)` (the grounding stub at `v2.go:298-305` becomes real). For every verdict with `!Unknown && Pass && !FromV1` (the judge's OWN pass) and every step id in `coverage[verdict.Key]` whose contract state is FAIL, emit ONE finding: `Severity: blocker, Category: CHECK-INTEGRITY, Criterion: "CHECK-INTEGRITY", Anchor: "step:<id>/<AC key>"`, plain-words text per §4(7). The returned verdicts are the input verdicts with `Disagreement = true` on every affected criterion and NOTHING else changed (Pass/Unknown/FromV1/BoundTo/Evidence/Forced untouched, order and length preserved). | S07.5 disagreement sentence (line 71); S07.7 route table row (line 100); CONVENTIONS §15 |
| R5 | **Placement.** The drain calls it once per judged round, after `ValidateAxis1` (`pipeline.go:523`) and before the raw-findings assembly (`:544`): `verdicts, contractIntegrity := ContractDisagreements(verdicts, v1Steps, in.Coverage)` where `v1Steps` = `v1res.Steps` (nil when `v1res == nil`); `record.Axis1 = verdicts` after it; the findings are appended to `raw` alongside `integrity`. It thereby runs BEFORE `validateFindings` (which keeps it: `CHECK-INTEGRITY` is a valid criterion, C8) and `ComputeVerdict` (which excludes it, C8), and the existing raiser (C9) cards it once per key. It applies to BOTH V1 branches (bootstrap and pack) because both populate `v1res.Steps`. | S07.5; S07.7; §15 |
| R6 | **Never the drain, never an override, never a quarantine.** The disagreement finding causes no REVISE by itself (C8 exclusion), is stripped from the review stream (C10), and quarantines nothing (`raiseIntegrity` matches `ACKey == "CHECK-INTEGRITY"` against no check, C9; the card's `Quarantined` is `""`). The judge's `Pass` is NOT rewritten: the mechanical fact is the STEP contract, which stands as the AC-BLOCKER of R1 and drives REVISE; there is no AC-level mechanical fact to substitute. | S07.5 ("not an override"); S07.7; §15 ("the mechanical fact stands and V3 still gates") |
| R7 | **No guessing without a coverage map.** With `Coverage` nil/empty (the pre-A16 seed worlds; every harness `input()`), `ContractDisagreements` returns no finding and marks nothing; the R1 contract finding cites `""` and is demoted to a note by `validateFindings` (C8), reaching the requester as a review comment with no verdict effect: the bootstrap precedent (TQ-3 T7). A criterion is never defaulted. | S07.5 citation rule; §74 |
| R8 | **Round-stable keys.** The contract finding key on the pack path is `AC-k|step:<id>|AC-BLOCKER` in every round (no round number, no check id, no exit code in criterion/anchor), so a check that keeps failing across rework rounds recurs unresolved and trips the S07.6 convergence stop into the CAP-HIT card. The disagreement key `CHECK-INTEGRITY|step:<id>/AC-k|CHECK-INTEGRITY` is likewise round-stable so the raiser cards it once per drain (C9). | S07.6 finding key (line 85); §74 |
| R9 | **Byte-unchanged:** `contract.go` (all of it: the §74 decision rules and `contractFinding`), `bootstrap.go`, `escalate.go` (`RouteTable`, `cardChoices`, `Card`), `record.go`, `rework.go`, `review.go`, `verify.go`; the landed tests `tq3_contracts_test.go`, `bootstrap_gf4_test.go`, `bootstrapv1_gf4_internal_test.go`, `contract_internal_test.go`, `contract_coordinator_internal_test.go`, `escalate_test.go`, `rework_test.go`, `v2_test.go`, `pipeline_test.go`, `review_seam_test.go`, `harness_test.go`. The five `RunV1` call sites in `v1_test.go` were already updated at grounding (mechanical `nil` coverage argument, no assertion touched); the executor edits no pre-existing test. | SKILL amendment A; CONVENTIONS §5 |
| R10 | **Plain words on every requester-facing string** this packet adds (R3 Detail, R4 text): no spec ids, no `§`, no category names, no anchor prefixes. | CONVENTIONS §38; TQ-3 T6 vocabulary |
| R11 | **No ⚙, no bound, no migration, no new dependency, nothing outside `internal/verify`.** The detector is O(|ACs| × |steps|) over already-loaded plan data; no cap exists to declare. | S18 (no key); CONVENTIONS §2/§4 |
| R12 | **The judge still sees the contracts.** `BuildJudgeInput` is untouched; the V1 slice (`verify/v1-outcomes`) continues to carry `Steps` and now also carries the R1 finding inside `V1Result.Findings` (as bootstrap's already does). | S07.5 input slice |

## 3. The mechanics, exactly

### 3.1 Pack-path materialization (R1, R2) — `v1.go`, inside `RunV1` after line 491

```go
res.Steps = stepContracts(res.Checks, steps, firstFailure)
byID := make(map[string]intake.Step, len(steps))
for _, s := range steps { byID[s.ID] = s }
for _, sc := range res.Steps {
    if sc.State == ContractFail {
        res.Findings = append(res.Findings, contractFinding(sc, byID[sc.StepID], coverage))
    }
}
return res, nil
```

`contractFinding` reads only `step.ID` and `sc.{StepID, DoneWhen, Detail}` (C5), so the lookup by id is exact. `stepContracts` emits one contract per step (C2); do not rely on index alignment. `V1Result.Findings`'s doc comment (`v1.go:272-274`) gains "contract FAILs".

### 3.2 The ladder FAIL Detail (R3) — `v1.go`, inside `stepContracts`

Collect the ids of bound checks with `c.State == CheckFailed` in ladder order; on FAIL set `sc.Detail` to the plain sentence of R3 (one or many). Keep `AttributedTo` semantics as they are (pre-existing; the last failing check wins, not this packet's to change). Do not set `Detail` on the other three states (OQ-1 of TQ-3 stands; a later packet may).

### 3.3 The disagreement detector (R4, R6, R7) — `v2.go`, replacing the stub at `:297-305`

```
out := append([]ACVerdict(nil), verdicts...)   // never mutate the caller's slice
byStep := map[stepID]StepContract over contracts
for i, v := range out:
    if v.Unknown || !v.Pass || v.FromV1 { continue }
    for _, sid := range coverage[v.Key]:
        sc, ok := byStep[sid]; if !ok || sc.State != ContractFail { continue }
        out[i].Disagreement = true
        findings = append(findings, Finding{
            Severity: SeverityBlocker, Category: CatCheckIntegrity, Criterion: string(CatCheckIntegrity),
            Anchor: "step:" + sid + "/" + v.Key,
            Text: <§4(7) sentence with sid, the criterion number, sc.Detail>,
        })
return out, findings
```

Criterion number: `acNumber(v.Key)` (`contract.go:708`) → "criterion 1"; a non-`AC-<n>` key (cannot occur from intake, C12) prints the raw key. Two findings for the same (step, AC) pair are impossible (coverage lists a step once per AC by construction; if a plan repeats a step id in one entry, dedupe by anchor). Pure function; no I/O; no settings.

### 3.4 Placement in the drain (R5) — `pipeline.go:523-524`

```go
verdicts, integrity := ValidateAxis1(ax1, input.ACs, v1Facts, d.Content)
var v1Steps []StepContract
if v1res != nil { v1Steps = v1res.Steps }
verdicts, contractIntegrity := ContractDisagreements(verdicts, v1Steps, in.Coverage)
record.Axis1 = verdicts
...
raw = append(raw, integrity...)
raw = append(raw, contractIntegrity...)
```

Everything downstream (`validateFindings`, `ComputeVerdict`, the raiser at `:608-618`, `reviewable`, the verdict switch) is unchanged and already does the right thing (C8, C9, C10). Note the ordering consequence stated honestly in §4(2): the finding rides `blockers(findings)` into the pre-S13 in-memory retry package and into `stop.observe`'s key set exactly as the landed `ACKey` disagreement does (C10). Not this packet's to change.

### 3.5 Signatures (fixed by the committed RED tests)

- `func RunV1(ctx context.Context, pack *CheckPack, runner CheckRunner, req CheckRequest, steps []intake.Step, coverage map[string][]string, now time.Time, settings Settings) (V1Result, error)` (landed at grounding; behavior pending).
- `func ContractDisagreements(verdicts []ACVerdict, contracts []StepContract, coverage map[string][]string) ([]ACVerdict, []Finding)` (stub landed at grounding).
- No other exported surface changes.

## 4. Answers the coordinator asked for

1. **Where the pack-path FAIL materializes, and how it relates to the check's own finding.** Inside `RunV1` right after `stepContracts` (§3.1), mirroring `bootstrapV1:147-149`, both calling the one `contractFinding` (C5), so the two paths cannot diverge (T1 and T3 pin the identical key shape). There is **no duplicate to dedupe**: a plain `CheckFailed` outcome mints NO finding of its own today (C1); the only V1 findings are the quarantine note and the runner-failure blocker, both anchored `check:<id>` and both leaving the contract UNVERIFIABLE-HERE, never FAIL (C2), so they cannot coexist with a contract finding on the same step unless another bound check failed, and then they are companions with distinct keys (`CHECK-INTEGRITY|check:<id>|CHECK-INTEGRITY` vs `AC-k|step:<id>|AC-BLOCKER`). Spec-honest reading: the CheckOutcome says WHAT failed (recorded on the `verify.v1` row with exit code and evidence ref), the contract finding says WHICH promise is broken and cites the criterion; R3's Detail carries the "what" into the finding text. UNVERIFIABLE-HERE and N-A stay findings-free (R2, T1, T5).
2. **The detector.** `ContractDisagreements` (§3.3), called at `pipeline.go:523+` between `ValidateAxis1` and the raw assembly, so it precedes `validateFindings` and `ComputeVerdict` (§3.4). Finding: blocker, category CHECK-INTEGRITY, criterion `CHECK-INTEGRITY`, anchor `step:<id>/AC-k`, plain text (7). Route: the existing raiser cards it once per key (C9) as a durable ask; `ComputeVerdict` ignores it; `reviewable` strips it; no quarantine (R6). The judge's verdict is marked `Disagreement` and otherwise kept (R6). Honest note: in the pre-S13 in-memory channel it rides `blockers()` into the retry package and into the convergence key set, exactly like the landed `ACKey` disagreement; in production (S13 sink wired) the retry comes from `DrainOpen` where `reviewable` has already stripped it. Unchanged here (§9 OQ-2).
3. **Coverage absent.** No disagreement can be computed and none is guessed; the contract finding cites nothing and is demoted to a note (R7, T4). The round record shows `Disagreement=false` everywhere. State on the receipt: nothing new (the demoted note already reaches review).
4. **Property invariants (committed as T5, T6).** (a) For any `RunV1` run: `{anchors of AC-BLOCKER findings with prefix step:} == {"step:"+id : contract FAIL}` and each cites the lowest covering AC (T5). (b) For any verdict vector, contract vector and coverage: a CHECK-INTEGRITY finding exists for exactly the pairs (AC judge-PASS ∧ ¬Unknown ∧ ¬FromV1, step ∈ coverage[AC] ∧ contract FAIL); verdicts unchanged except `Disagreement` (T6). (c) UNVERIFIABLE-HERE/N-A/PASS never produce a `step:`-anchored finding (T5's set equality, T1's stray check, T2's `step:S-2/AC-2` stray check).
5. **Byte-unchanged:** R9's list. `contract.go`'s decision rules are not touched (§74); `RouteTable`, `cardChoices`, `Card` kinds unchanged; the bootstrap tests unchanged.
6. **⚙:** none. No bound anywhere (R11).
7. **Copy (plain words, §38).** Detail (R3): `The automated check "lint" did not pass.` Disagreement text (R4): `The automated check found step S-1's promise unmet, but the review marked criterion 1 as met. The automated check "lint" did not pass. The checks and the review disagree, so a person has to look.` (i.e. `The automated check found step %s's promise unmet, but the review marked criterion %s as met. %s The checks and the review disagree, so a person has to look.` with `%s` #3 = the contract's `Detail`; on the bootstrap path that Detail is `failDetail`'s sentence, e.g. "The files this work produced do not show this step finished: the line names public/images/** and no file it produced matches that."). The card summary is the existing `"check-integrity: " + f.Text` (C9).

## 5. Seams, settings, files, components, landed-test edits

- **Seams:** none new. `Verifier.Judge`, `Runner`, `Workspace`, `Review` untouched. The S13 review sink's `reviewable` strip is relied on, not modified.
- **⚙ settings consumed:** none new; the existing `verification.card_remind_hours`/`card_push_hours` are consumed by `Escalator.Raise` for the card as today.
- **Files expected to change:** `internal/verify/v1.go` (R1, R3), `internal/verify/v2.go` (R4), `internal/verify/pipeline.go` (R5: ~5 lines). Tests: `internal/verify/tq6_routing_test.go` (committed RED; executor must not edit). Nothing under `internal/stage`, `internal/shell`, `internal/review`, `internal/api`, `internal/intake`, `internal/memory`, `web/`.
- **Adopted components touched:** none.
- **Landed-test edits:** already made at grounding and declared in its commit: `internal/verify/v1_test.go` five `verify.RunV1(` call sites gained a `nil` coverage argument (`:80, :136, :167, :175, :221`). No further pre-existing test edit is sanctioned.

## 6. CONVENTIONS constraints that bind

- §2 stdlib-first, gofmt/vet clean, `%w` wrapping (no new errors expected), doc comments cite spec sections only where clarifying. No ⚙ as a constant (none exist here).
- §3 stdlib `testing` only; the RED window is this grounding's (declared in its commit) and the implementation commit closes it; `go build ./...` stays green throughout.
- §5 commit subject `P3-TQ-6: <summary> (S07.3, S07.5, S07.7 refs)`; trailer names the authoring model; stage by explicit pathspec; never push; never edit `Spec/`, `Docs/`, `Research/`, `P3/STATE.md`.
- §15 CHECK-INTEGRITY findings never enter the REVISE drain: suite defects route to card; the mechanical fact stands; V3 still gates. `ComputeVerdict`'s exclusion and `reviewable`'s strip are the landed mechanisms; do not add a third.
- §38 plain words on requester-facing copy; classification never by error text.
- §58/§59 unchanged: the check pack shape (`packChecks`) and the stripped verification workspace are not touched.
- §70 the bootstrap posture disclosure exemptions (`isPostureDisclosure`) are identity-keyed and untouched; the new CHECK-INTEGRITY finding must NOT be exempted from the review strip.
- §74 `contract.go` byte-unchanged; the bootstrap path's materialization is the precedent, not a site to refactor.
- Executor scope guardrail: no features beyond R1–R12, no refactor of `stepContracts`'s attribution, no handling for a coverage entry naming an unknown step beyond skipping it (§3.3 `ok` check: a plan's coverage names plan steps by construction, `intake` validates it).

## 7. Acceptance checklist (each item is a test or a grep)

- [ ] T1–T7 (`go test -p 1 -count=1 -run 'TestTQ6' ./internal/verify/`) all PASS.
- [ ] TQ-3 and GF4 batteries unchanged and green: `-run 'TestTQ3|TestGF4'`; `git diff --stat main -- internal/verify/tq3_contracts_test.go internal/verify/bootstrap_gf4_test.go internal/verify/contract.go internal/verify/bootstrap.go` is empty.
- [ ] `go test -p 1 -count=1 ./internal/verify/` green; `go build ./...`, `go vet ./...`, `gofmt -l internal/` empty; `go run ./tools/lockgate` green. (Never run `./internal/stage/` unfiltered: its two live tests spin up a GPU llama-swap.)
- [ ] `grep -n 'contractFinding(' internal/verify/*.go` shows exactly two call sites: `bootstrap.go` and `v1.go`.
- [ ] `grep -n 'ContractDisagreements(' internal/verify/pipeline.go` shows exactly one call, between the `ValidateAxis1` call and `var raw []Finding`.
- [ ] `grep -rn 'verification\.' internal/verify/v1.go internal/verify/v2.go` shows no new ⚙ key; `git diff main -- internal/settings/` empty.
- [ ] Every string literal added under R3/R4 passes `assertPlainWords` (T1, T2, T3 assert it on the produced text).
- [ ] No file outside `internal/verify/` and `P3/` changes in the packet's range.

## 8. Acceptance-test specifications — `internal/verify/tq6_routing_test.go` (package `verify_test`), COMMITTED RED

All seven run through the landed harness (`harness_test.go`: real `platform.db`, real event log and ledger, scripted judge and runner) and the landed helpers (`ladderPack`/`ladderSteps`/`v1req`/`regSettings` from `v1_test.go`; `bootstrapPack`/`tq3Input`/`webshopSteps`/`writeTree`/`stepByID`/`findingAt` from the TQ-3/GF4 files). Observed RED at grounding, each for the stated cause:

| Test | Binds | Observed RED cause |
|---|---|---|
| T1 `TestTQ6LadderContractFailReachesTheRequesterAsACBlocker` | R1, R2, R3, R8, R12 | `S-1 FAIL detail ""` (no Detail; and no `step:S-1` finding) |
| T2 `TestTQ6JudgePassOverLadderContractFailIsCheckIntegrity` | R4, R5, R6, R8 | no CHECK-INTEGRITY finding anchored `step:S-1/AC-1` |
| T3 `TestTQ6DisagreementIsRaisedOnTheBootstrapPathToo` | R4, R5 (bootstrap branch), key parity | no CHECK-INTEGRITY finding anchored `step:S-2/AC-1` (the landed `step:S-2` AC-BLOCKER is present) |
| T4 `TestTQ6NoCoverageMapMeansNoDisagreementAndANote` | R7 | no finding anchored `step:S-1` without a coverage map |
| T5 `TestTQ6PropStepAnchoredBlockersAreExactlyTheFailContracts` | R1, R2 invariant (a)+(c), `RunV1` direct | `step-anchored blockers [] want [step:S-1]` (seed 1 n=1) |
| T6 `TestTQ6PropDisagreementIffJudgePassOverAFailContract` | R4, R6, R7 invariant (b), pure | `disagreements [] want [step:S-1/AC-1 step:S-3/AC-1]` (seed 2 m=2 n=4) |
| T7 `TestTQ6PersistingLadderFailTripsTheConvergenceStop` | R8 | `rounds 1, want 3` (the round is SHIP today) |

Both property tests carry a vacuity guard (`t.Fatal` if the generator never produced a FAIL / a disagreement pair). Sibling batteries verified green under the inert surface at grounding (`-run 'TestTQ3|TestGF4|TestLadder|TestQuarantine|TestRunner|TestAudit|TestPack|TestPlanted|TestConvergence|TestShip|TestValidateAxis1|TestUnknown|TestReviewSeam|TestNotes|TestCapHit|TestRetry'` → `ok`).

## 9. Open questions for the coordinator (none block R1–R12; OQ-1 decides the packet's LIVE value)

- **OQ-1 (ruling required; STOP-candidate for scope, not for execution).** The live production pack binds no step (C13): captured lint/build/test are unbound checks, so `stepContracts` marks every step N-A on the pack path and **R1 has no effect on any production project today**; it is a structural guarantee for step-binding packs (the harness packs, a future A16 walk, a hand-edited pack). The LIVE instance of the same P46 class is the unbound failed check: a captured `build` that fails is recorded on the `verify.v1` row and in the judge's slice and nowhere else; with a pass-all judge the round SHIPs and `VerifyLedgerItems` verifies. `packChecks` declares its route as "an AC-blocker's route (rework round, then a requester decision card at cap)" (`project_seams.go:527-530`) and nothing realizes it. Routing it needs a reading of the S07.5 citation rule: a blocker must cite a frozen criterion, and a failed build violates no single AC (the seed rubric carries only axis-2 items, `seeds.go:137-155`, so no rubric citation exists either). Candidate readings, none decided here: (a) an unbound FAIL mints an AC-BLOCKER anchored `check:<id>` that cites nothing and is demoted to a note (honest, toothless: the build fails and the round can still SHIP); (b) `packChecks` binds every captured check to every code-writing step (a shell-side change; touches a file this packet may not specify, and multiplies checks); (c) admit the check's declared `FindingCategory` as a citable criterion for platform-raised V1 blockers (a change to the citation rule's `valid` set; goalpost-fixing is not at stake because the pack is the project's own frozen bar, but it is an S07.5 reading the operator should see); (d) treat an unbound FAIL at rung k as the first upstream failure of every step's contract (S07.3 attribution) and decide FAIL for steps with a write set. Recommendation: a separate small packet after a coordinator/operator reading; do not fold it into TQ-6 silently.
- **OQ-2 (observation, pre-existing).** `stop.observe` and `BuildRetryPackage` are category-blind (C10): a CHECK-INTEGRITY blocker counts in the convergence key set and, in the pre-S13 channel only, enters the retry package. The landed `ACKey` disagreement has the same property; this packet adds a second finding with it. Filtering `CatCheckIntegrity` out of `blockers()` consumers would be a one-line §15 tightening with convergence consequences (a judge that flips to agreement in round 2 would no longer reset the recurrence counter). Not specified here.
- **OQ-3 (observation, pre-existing).** `raiseIntegrity` passes no `Detail`, so integrity cards raised under the bootstrap posture do not carry `BootstrapPostureNote` (`postureDetail` is applied on the other terminals only). T3 raises such a card. Not specified here.

## 10. STOP conditions

- None for R1–R12: no spec conflict, no ⚙, no dependency, no migration, no adopted component, no touch outside `internal/verify`.
- If the executor finds that any landed test other than the sanctioned five `RunV1` call sites must change to go green, STOP and report the deviation rather than editing it.

## 11. Grounding run record

- Base `eb7594b` on branch `worktree-agent-afe7b6badfa5a1196`. Inert surface: `RunV1` coverage parameter + pass-through (`v1.go:407`, `pipeline.go:494`), `ContractDisagreements` stub (`v2.go:298-305`), five `v1_test.go` call sites. `go build ./...` green, `go vet ./internal/verify/` green, `gofmt -l internal/verify/` empty.
- `go test -p 1 -count=1 -run 'TestTQ6' ./internal/verify/` → 7 FAIL, each for the cause in §8. Sibling filter → `ok`.
- No world or server started; `./internal/stage/` never run.
