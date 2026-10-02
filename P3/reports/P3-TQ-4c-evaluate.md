# P3-TQ-4c — evaluation report

**Evaluated 2026-10-02** on worktree `agent-a08ed49b57e61379b`, branch `worktree-agent-a08ed49b57e61379b`, base `333c556` (main at evaluation time `65dbca2`; `git diff 333c556 65dbca2` touches `P3/` docs only, no code drift). Range: grounding `497b7c2` (brief + red tests) → implementation `19595de`; the executor's report `27bdb02` (report file only) was read and not trusted. Truth read directly: `P3/briefs/P3-TQ-4c.md` (R1–R13, checklist 1–13), `Spec/drafts/S07-verification-quality.md` S07.3/S07.5/S07.7/S07.8 [A16], CONVENTIONS §79, §80 (worktree) and §81 (main).

## VERDICT: PASS

Nothing above nit. All 13 checklist items hold against the diff; the two sanctioned test edits are exactly the named ones; seven mutation probes were all caught by the suite; four novel held-out probes passed or confirmed the behavior the brief describes. Findings F1–F7 are nits (wording, invariant scoping, a spec-reading to state, pre-existing classes).

## Findings

**F1 [nit/high] `internal/verify/v2.go:463` + brief R10(ii) — "exactly one finding at `check:<id>`" is an invariant over PLATFORM mints only.** A judge note carrying the platform note's exact key (`|check:detected:<slot>|AC-BLOCKER`, criterion "") is admitted at round 1 unconditionally (round 1 suppresses nothing) and at round ≥ 2 through `priorKeys` once the platform note existed in an earlier round. The R6 guard (`TestTQ4cAJudgeNoteShapedLikeTheDetectedNoteIsStillSuppressed`) covers only the no-prior-key case. Evidence — probe G4 (detected:build fails both rounds, lint fails round 1 only, judge adds the forged-shape note at call 2): round 2 has **2** findings at `check:detected:build` (judge N=1 "the judge dresses up as the platform", platform N=2), `SuppressedNotes == 0`, SHIP-with-notes; the sink holds 2 rows at that anchor across the two rounds. On the review surface both are kind "finding" (`stage.reviewSink.RecordFindings` maps every non-requester finding to `review.KindFinding`), so a reader cannot tell them apart — the `fromCheck` mark is unexported by design. Severity nit: a note never forces REVISE; the class is pre-existing (a judge can re-raise the owner kill's key as a demoted note the same way, and `isPostureDisclosure` admits a judge note with the disclosure's identity key); no code change belongs in this packet. **Proposed:** §82 states the invariant as "exactly one PLATFORM note per detected FAIL" and names this carve-out beside R6.

**F2 [nit/high] `internal/verify/tq7_kill_test.go:102-104` — the sanctioned edit also rewrote the helper's doc comment.** R12(a) names the filter clause (`f.Severity == verify.SeverityBlocker`); the executor additionally replaced the one-line doc with three lines ("returns the AC-BLOCKER blockers … A failed detected rung's NOTE shares the anchor shape and is not one (P3-TQ-4c)"). Comment-only, inside the named helper, accurate. Recorded per duty 4b; no action.

**F3 [nit/med] Brief R7 and the execute report cite a "What the checker found" section of the deliverable page; no such string exists.** `grep -rn 'What the checker found\|checker found' web/ internal/api/` is empty. The real human-visible sink is `web/src/Deliverable.tsx:891` ("verdict #N — the checker's findings on this revision", comment rows with `data-author="platform-checker"`), fed by `stage.reviewSink.RecordFindings` (`internal/stage/review_sink.go:175`), wired in production at `internal/shell/shell.go:756/871/1116`. The sink exists; only the brief's wording is off. With `Verifier.Review == nil` (dev/test posture) the note lives on the `verify.v1` row, the round record and the `verdict.recorded` row only — the brief accepts that posture explicitly.

**F4 [nit/high] Brief-internal: R1's consequence prose disagrees with R1's formal predicate.** The prose says an owner failure blocks every later rung of either origin "each attributed to the owner's first failure"; the predicate says a blocked DETECTED rung is attributed to `firstFailure`, "the earliest failure of any origin". When a detected rung failed at a LOWER stage than the owner's failure, the blocked detected rung is attributed to the detected id. Implementation, the property test's oracle (`tq4c_mixed_test.go:646-647`) and the executor's §82 draft all follow the predicate. Evidence — probe G1 (detected:lint FAIL static, owner test FAIL unit, detected:smoke, owner e2e): `detected:smoke` → UNVERIFIABLE attributed `detected:lint`; `e2e` → attributed `test`; S-2 → attributed `test`. **Coordinator:** land §82's wording ("past the earliest failure of ANY origin"), not R1's prose.

**F5 [nit/med] S07.7 sink enumeration vs the note's sink — a reading to state, not a defect.** S07.7: "a decision card, an operator alert, or a worker-flaw ticket. No other sink type exists". The note's sink is a review comment, which rests on S07.6/S13.4 ("notes ride along to the requester as review comments") and the §70 precedent (the posture disclosure already reaches the person through `reviewable`). The brief declares this reading binding. §82 should state it in one sentence so the next packet does not re-derive it.

**F6 [nit/low] Real-sink re-raise semantics (pre-existing, not this packet's seam).** `reviewSink.RecordFindings` adds one comment row per finding per round at the judged revision; a detected rung failing in rounds 1 and 2 yields two open rows (revision 1 and 2), exactly as every re-raised finding does (owner kill, judge notes). Observed through the fake sink in G4 (2 rows across both rounds). The brief's "same note each round" (R5) is about the KEY, which holds. Carried; no action.

**F7 [nit/low] `internal/verify/verify.go:107` — one over-long comment line in the rewritten `fromCheck` doc** ("RunV1 and bootstrapV1. It needs to survive none of those round trips: validation runs"). gofmt-clean; style only.

**Carried ledger (confirmed, from the brief/executor):** `BootstrapPostureNote` (`bootstrap.go:54`) "every check rung is recorded as unverifiable here" is stale post-A16 — coordinator's wording amendment with pinned tests; B2-3 N2 (owner runner error → integrity card AND SHIP with items verified) stays open. The §79/§80 sentence amendments the executor drafted are accurate to the landed code.

## Checklist (brief §5) against the diff

| # | Item | Result | Evidence |
|---|---|---|---|
| 1 | build/gofmt/vet; TestTQ4c green; verify+shell green; stage green under skips | HOLDS | `go build ./...` ok; `gofmt -l internal/` empty; `go vet` clean; `-run TestTQ4c ./internal/verify/ ./internal/shell/` ok; `./internal/verify/` ok 4.8s; `./internal/shell/` ok 9.4s; `./internal/stage/` ok 14.4s with the two live skips |
| 2 | detected FAIL leaves a later owner rung running, `AttributedTo ""`, contract decided from it | HOLDS | `v1.go:493-496` lane selection; `TestTQ4cDetectedFailureNeverBlocksAnOwnerRung` ×3 green; mutation A (any lane for owners) fails all three sub-tests |
| 3 | owner FAIL blocks both origins, attributed to the owner's first; blocked detected mints nothing | HOLDS | `v1.go:555-563` owner lane set only under `c.Origin != ProvenanceDetected`; `TestTQ4cOwnerFailureBlocksLaterRungsOfBothOrigins` green (also under mutation A) |
| 4 | note shape: note, undemoted, declared cat, criterion "", anchor `check:<id>`, key, text, tail, plain words | HOLDS | `detectedNote` `v1.go:763-781`; `assertDetectedNote` passes in every TQ4c test; mutation G (criterion `check:<id>`) fails on the criterion assertion AFTER the severity assertion passed → severity stays note |
| 5 | drain with sink: SHIP-with-notes, no card, no integrity card, `[w1]` verified, sink/v1-row/verdict-row carry it, judge once, `detected:test` → `detected:build` | HOLDS | `TestTQ4cDetectedFailureInAGraduatedPackIsAVisibleNote` green; `ComputeVerdict` (`v2.go:506-525`) byte-unchanged; integrity raiser `pipeline.go:658` blockers only |
| 6 | round-2 note itemized, `SuppressedNotes 0`, sink at rev 2; judge-shape note still suppressed | HOLDS | `v2.go:463` one clause; both tests green; mutation B (drop clause) and C (drop mark) each fail the round-2 test while the guard stays green |
| 7 | bootstrap: disclosure AND note, SHIP-with-notes, `ReviewMandatory`, no verified items, passing rung mints nothing | HOLDS | `bootstrap.go:167-169` seeds the disclosure first, appends notes; test green; mutation F (drop the append) fails it; probe G2 extends it to round 2 |
| 8 | property over 84 mixed packs: oracle, no crossing, FAIL ↔ note, owner FAIL ↔ kill | HOLDS | green; fails under mutations A, D, E; the oracle is an independent two-lane walk (`tq4c_mixed_test.go:635-691`) |
| 9 | drain property over 4 subsets × tail: SHIP-with-notes iff a detected rung that RAN failed | HOLDS | green |
| 10 | 8×8 composition proof in shell | HOLDS | `packSlots` has exactly three slots (`project_seams.go:580-584`); the ONLY `"detected:"` id mint site is `project_seams.go:641`; `TestTQ4cASlotIsTheOwnersOrDetectedNeverBoth` green; `ladderCheck` declares `CatACBlocker` for every rung, detected included, so the bootstrap note's category is declared, never empty |
| 11 | R12's two edits exact; every other pre-existing test untouched; `TestTQ4DetectedRungInAGraduatedPackMintsNoFinding` green unedited | HOLDS (F2 nit) | `git diff 333c556..19595de -- '*_test.go'`: `tq7_kill_test.go` (+filter clause, +doc comment), `tq7drain_test.go` (absence → "no BLOCKER", closing comment), plus the grounding's two new files; the tq4drain test is unedited and green (fails under mutation E) |
| 12 | R11 docs; no ⚙/constant/card/route/event/category; `pipeline.go`, `review.go`, `rework.go`, shell, stage, web byte-unchanged | HOLDS | implementation commit touches `bootstrap.go`, `v1.go`, `v2.go`, `verify.go` + the two tests only; all seven R11 doc sites corrected (`Check.Origin`, `CheckPack.Provenance`, `V1Result.Findings`, `Finding.fromCheck`, `RunV1`, `bootstrapV1`, `checkFinding`) |
| 13 | evaluator probes | DONE | see below: every predicted failure materialized |

## Hunt list (launch prompt)

- **Attribution leak in a composition the 8×8 proof does not cover:** the proof covers the only composition root (three slots, one mint site). Attribution itself is pinned by the property over random 4-stage packs of both origins with quarantines, runner errors and step bindings, and by probe G1's four-stage case. No leak found; an owner outcome's `AttributedTo` can only ever be `ownerFirstFailure`, which is set only under the owner guard.
- **Detected failure still kills or mints a card in a graduated pack:** no. `ComputeVerdict` counts a note → SHIP-with-notes; the integrity raiser fires on CHECK-INTEGRITY blockers only; `stop.observe` keys blockers only; `VerifyLedgerItems` runs. Pinned by checklist 5 and 9.
- **`fromCheck` notes suppressed or duplicated across rounds:** not suppressed (R6, mutations B/C); one note per FAIL per round (property); re-raised each round under the same key by design. The only "two at one anchor" case is a judge note sharing the key (F1, nit).
- **Bootstrap note without the disclosure beside it:** `bootstrapV1` always seeds the disclosure first; at round 2 both survive `validateFindings` (disclosure by identity, note by mark) — probe G2.
- **Output-tail bounds:** `detectedNote` re-bounds through `boundedTail` exactly as `checkFinding` does; probe G3 — 5000 bytes no newline → 2048; 3000x+`\n`+1500y → the 1500-y line; whitespace-only → "It printed nothing."; `a\n`+2047z → the 2047-z line.
- **S07.7 no sink lost:** no path lost a sink; owner rungs that used to be silenced by a detected failure now run and decide (gained sinks). The brief's sink wording is off (F3) and the reading should be stated (F5).
- **§80 not weakened:** every owner FAIL still mints `checkFinding` (blocker, `check:<id>` criterion, `fromCheck`); owner rungs are never blocked by a detected failure, so they can only kill MORE often than before; `TestTQ7*` all green including under mutation C; `tq4cCheckBlockers` and the property's "owner FAIL ↔ exactly one blocker citing the check" pin it.

## Probe log (amendment C)

Held-out probes lived in `internal/verify/zz_tq4c_probe_test.go` (package `verify_test`), run with `-run TestZZProbe`, and the file was deleted before the final battery (`git status` empty).

- **G1 — four-stage mixed composition with step bindings** (`detected:lint` static FAIL; owner `test` unit FAIL bound S-1; `detected:smoke`; owner `e2e` bound S-2): runner calls `[detected:lint test]`; `detected:smoke` UNVERIFIABLE←`detected:lint`; `e2e` UNVERIFIABLE←`test`; S-1 FAIL←`test`; S-2 UNVERIFIABLE←`test`; findings = one note `|check:detected:lint|AC-BLOCKER`, one kill `check:test|check:test|AC-BLOCKER`, one contract blocker `AC-1|step:S-1|AC-BLOCKER`. **PASS.**
- **G2 — bootstrap round 2** (judge blocker on AC-1 at round 1 → REVISE; `detected:test` script `{0,1}`; permissive sink): round 1 REVISE, no note; round 2 posture bootstrap, disclosure present, note present with tail, `SuppressedNotes 0`, SHIP-with-notes, `ReviewMandatory`, no verified items, sink holds 1 note at `check:detected:test`. **PASS.** (First attempt failed only because the probe had not wired the sink — probe bug, fixed.)
- **G3 — tail bound in four cut shapes:** 2048 / 1500 (line boundary) / "printed nothing" / 2047. **PASS.**
- **G4 — judge piggyback on a prior platform key:** 2 findings at `check:detected:build` at round 2 (judge N=1, platform N=2), 0 suppressed, SHIP-with-notes; 2 sink rows across rounds. **Behavior confirmed → F1 (nit).**

Mutation probes (applied with perl, targeted tests run, production files restored with `git checkout --` each time; final `git status --short internal/` clean):

| Mutation | Edit | Predicted (brief 13) | Observed |
|---|---|---|---|
| A | owner rungs read the ANY lane (`laneStage, laneFailure := failedStage, firstFailure`) | 2 and 8 fail | `TestTQ4cDetectedFailureNeverBlocksAnOwnerRung` ×3 sub-tests FAIL, property FAIL (`c2 recorded UNVERIFIABLE-HERE/"detected:c3", want PASS`); guard GREEN — **caught** |
| B | drop `&& !f.fromCheck` (`v2.go:463`) | 6 fails | round-2 test FAIL (`findings anchored check:detected:build = 0`); judge-shape guard GREEN; round-1 test GREEN — **caught** |
| C | drop `fromCheck: true` from `detectedNote` | 6 fails, guard passes | round-2 test FAIL; guard GREEN; all `TestTQ7*` GREEN — **caught** |
| D | `stepContracts(res.Checks, steps, firstFailure)` | 2's quarantine sub-test and 8 fail | exactly those two FAIL (`contract S-1 attributed to "detected:build"`; `step S-1 attributed to detected detected:c1`) — **caught** |
| E | runner-failure arm also mints `detectedNote` for detected | tq4drain test fails | first attempt: perl pattern matched nothing (empty diff, void result); redone with the correct indentation: `TestTQ4DetectedRungInAGraduatedPackMintsNoFinding/a_detected_rung's_runner_failure_mints_nothing` FAIL and property FAIL (`RUNNER-FAILURE carries notes=1`) — **caught** |
| F | `bootstrapV1` drops the notes (`_ = notes`) | 7 fails | `TestTQ4cBootstrapDetectedFailureMintsTheSameNote` FAIL (only the disclosure left) — **caught** |
| G | `detectedNote` cites `check:<id>` as criterion | 4 fails; severity stays note | both shape tests FAIL on the criterion assertion, which runs after the severity assertion passed → severity stayed note; `TestTQ7MixedPackKillsOnTheOwnersRungOnly` GREEN — **caught** |

## Battery

Serial throughout, one package at a time, no foreign battery live (checked before each run; a foreign shell wait-loop PID 241056 was present and is not a test process). `go build ./...` ok; `gofmt -l internal/` empty; `go vet ./internal/verify/ ./internal/shell/` clean. `-run TestTQ4c ./internal/verify/ ./internal/shell/` ok. `./internal/verify/` ok (4.8s); `./internal/shell/` ok (9.4s); `./internal/stage/` ok (14.4s) under `-skip 'TestLivePhraseAndSummarize|TestLiveIntakeTriageClassifiesWebshop'`. Final foreground `go test -p 1 -count=1 -skip '…' ./...`: exit 0, **48 ok, 0 FAIL**, 5 packages without tests. No orphan test processes after the run.

## Hygiene

Worktree only; no production or test file modified (every mutation restored; scratch probe deleted); nothing pushed.

## Draft STATE landing line (≤600 chars)

P3-TQ-4c EVALUATED PASS (19595de on 497b7c2): all 13 checklist items hold; R12's two sanctioned test edits exact (+ a doc comment on the same helper, nit); 4 held-out probes (4-stage mixed composition, bootstrap round-2 note beside the disclosure, tail bound 2048/1500/none/2047, judge piggyback on a prior platform key → 2 notes at one anchor) and 7 mutations all caught; nits only: R10(ii) is a platform-mint invariant, R1 prose vs predicate, S07.7 sink reading to state in §82, brief's "What the checker found" wording; build/vet/gofmt clean; full ./... 48 ok.
