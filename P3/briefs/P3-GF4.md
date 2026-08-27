> **EXPIRED at P3-GF4 landing (2026-08-27).** Single-use artifact — drain fixes moved the code past this text; never read as truth.

# P3-GF4 — bootstrap verification posture for command-less launch-domain tasks (Amendment A14)

**Contract:** `Spec/drafts/S07-verification-quality.md` §S07.8 bootstrap bullet (line 120, carries the A14 tag) + the A14 changelog row `Spec/drafts/S00-front-matter.md` line 194. Siblings this packet touches: S07.3 (stage contracts + the ratified vocabulary), S07.5/S07.11 (verdict/recording/receipt), S07.7 (route table, refusal terminals), S06.4 (stakes tiers, trivial band), S13.7 (the registry capture that feeds packs). Operator record: `P3/design/b6-gate-operator-findings-r4-2026-08-23.md` §F1/F1a ("Fix it for good now" — the order IS the order to execute).

**Scope guardrail (verbatim from the packet order):** GF4 is the verification-posture change ONLY. The project-Commands write route and its UI door are GF5/GF6 — out of scope. The card/receipt copy MAY say "capturing the project's commands restores the full ladder"; BUILDING the capture door is not this packet. No new features, no refactors, no abstractions beyond what A14 requires; validate at system boundaries only.

---

## 1. The defect, as it stands in the code today (all verified this session)

A launch-domain (software) deliverable whose registered project's S13.7 capture holds no build/test/lint commands dies before the drain starts:

- `internal/shell/project_seams.go:404-426` `packFromCapture`: zero checks → error wrapping `verify.ErrNoCheckPack` with the text: *"no build, test or lint commands are captured for project %q, so there is nothing to check this work with — capture them for the project (its Commands), then retry"*. (`packChecks`, :438-463, maps lint/build → `StageStatic`, test → `StageUnit`; run/preview are deliberately NOT checks.)
- `internal/verify/verify.go:229` `ErrNoCheckPack` = *"verify: launch-domain deliverable without a domain check pack (Spec S07.8)"*.
- `internal/stage/skeleton.go:1009-1021` `checkPack` marks it `PreambleRefusal`; `drainOrCard` (:906-932) converts it to `verify.InfrastructureEscalation` (`internal/verify/escalate.go:222-239`): Summary = *"verification cannot run — "* + cause; Detail includes *"The checks that decide whether this work is correct are missing, so the platform stopped instead of guessing"* and *"Fix what the summary names, then answer `retry` to run verification again; `cancel` ends the task."*; choices `retry`/`cancel`; `Quarantined: "domain check pack: software"`; `Infrastructure: true`.
- The run PARKS on the card (`verifyTerminal`, skeleton.go:1125-1133). The frontend renders (`web/src/Inbox.tsx:2358-2362`): *"Verification never ran — nothing was judged, nothing was delivered, and the work is parked on this card. Answering it is what moves the task."*
- `retry` can never succeed (GF5's capture door does not exist), which is the operator's twice-hit wall (r4 §F1).
- Same-class refusals that are NOT this packet's target: task attached to no project (`project_seams.go:262-263`, door: register + attach exists today — see OQ1); `ErrBadPack` for commands-with-unreadable-capture-date (:412-414); `RunV1` pack-validation refusal (`v1.go:392-401`); nil Judge / unvalidated judge seat (`pipeline.go:242-243`, skeleton.go:955-961).

**S07.3 vocabulary — exists, exactly as A14 assumes:** `ContractState` = `PASS / FAIL / N-A / UNVERIFIABLE-HERE` (`internal/verify/v1.go:75-88`, `ContractUnverifiable = "UNVERIFIABLE-HERE"`); per-check `CheckOutcomeState` adds `QUARANTINED` and `RUNNER-FAILURE` (:209-222). `stepContracts` (:479-520) gives `N-A` when no check decides a step, `UNVERIFIABLE-HERE` when blocked/undecidable.

**V2 advisory marking — NOT expressible today:** neither `RoundRecord` (`rework.go:82-105`) nor `roundPayload` (`record.go:57-93`) nor `Axis2Result` (`v2.go:78-89`) nor the `Card` (`escalate.go:105-139`) has any posture/advisory member. The only posture-shaped precedents are `Axis2Skipped` (a recorded skip reason) and `Entailment` (a posture string on the round payload).

**V3 / trivial band — the wire today:** SHIP → run completes, kanban `done` (`skeleton.go:1114-1124`); the deliverable sits `in-review` until the S13.6 Accept (`internal/review/accept.go:41`, idempotent; served as a High-tier PIN act, `internal/api/accept.go`). There is NO tier-conditioned behavior anywhere post-verification and NO auto-accept; the S06.4 trivial-band "non-blocking completion card" is not a code object at v0 (the band's only landed effect is intake-side: `internal/intake/pipeline.go:1307-1314` skips the approval gate). So "V3 blocks at every stakes tier" is structurally satisfied at v0 — GF4's obligation is to RECORD the mandatory-review fact durably so no later completion/auto-deliver packet can skip it silently (R8), not to build a blocking surface.

**Receipt today:** `metering.Receipt` (`internal/metering/receipt.go:27-67`) has no verification member; served raw at `GET /api/runs/{run}/receipt` (`internal/api/api.go:601`); golden fixture `web/src/fixtures/api/receipt.json`.

---

## 2. Requirements (each with its S-ref)

**R1 — Bootstrap entry, never a refusal [A14; S07.8 bootstrap bullet].** A launch-domain deliverable whose REGISTERED project's current S13.7 capture holds no build/test/lint command (empty capture, or run/preview-only — `packChecks` produces zero checks) never yields `ErrNoCheckPack`, never mints the infrastructure card, never parks. The drain runs under the **bootstrap posture**. The condition is exactly "no executable rung exists": one captured build OR test OR lint command = a real (partial) pack = the full posture, not bootstrap.

**R2 — V0 unchanged [S07.8: "V0 runs unchanged"].** All pre-gates including the wrote-nothing gate run exactly as today; a V0 hard kill still costs one regeneration (S07.2).

**R3 — Bootstrap V1 is present and honest [S07.3; S07.8].** The round record carries a V1 result (`record.V1 != nil`) and a `verify.v1` event is recorded, in which:
- every executable-ladder rung that would need the missing commands records **`UNVERIFIABLE-HERE`** — never absent (silent-skip), never `PASS`. Concretely: the rungs that captured commands would populate (static, unit at minimum; the executor may record all four ladder rungs) each appear as an outcome in state `UNVERIFIABLE-HERE` with a Detail naming the missing capture in plain words;
- every PLAN step's "Done when" contract records **`UNVERIFIABLE-HERE`** (not `N-A` — the deciding substrate is absent, not inapplicable; `ContractUnverifiable`'s own doc: "the deciding substrate is not wired") with attribution naming the missing pack (e.g. `AttributedTo` = a stable marker such as `check-pack:absent`);
- no evidence artifacts are fabricated (no `EvidenceRef` on unrun rungs); no check IDs are invented as executable checks (the red test pins `len(pack.Checks) == 0` for whatever pack-like object exists).

**R4 — Bootstrap records never force a park [A14: "never parks the run"].** The bootstrap rung/contract records must not enter the round as blocker findings (a permanent blocker would make REVISE→cap→park unconditional, defeating A14). The posture disclosure enters as **note-severity** finding(s) with a stable finding key (survives note-suppression across rounds: `validateFindings` suppresses only NEW note keys after round 1, `v2.go:354-356`). SHIP under bootstrap therefore lands as SHIP-with-notes at best — acceptable and honest.

**R5 — V2 runs, advisory and visibly non-authoritative [S07.8: "the degraded-mode treatment above"].** Both axes run (launch domain ⇒ axis 2 always, S07.8 first bullet; `axis2Required` already returns true for launch domains). The verdict is marked advisory + non-authoritative durably: new members on `RoundRecord` and `roundPayload` (omitempty — e.g. `posture: "bootstrap"` plus a plain-words statement), so progression across revisions shows exactly which rounds were advisory. Axis-1 verdicts consume no V1 facts (there are none: `ACOutcomes` over zero executed checks is empty) — the judge decides ACs from the artifact, which is precisely the non-authoritative part the marking names.

**R6 — Requester review mandatory; V3 blocks at every tier including trivial [S07.8; S06.4].** No code path may auto-accept, auto-deliver, or mark-verified a bootstrap-verified deliverable without a human act. At v0 this is: (a) the deliverable stays `in-review` with the accept door the only exit (already structural — pin it by test); (b) the mandatory-review fact is durably recorded on the round record/verdict payload so any later completion-card or auto-deliver packet inherits it as data, not folklore; (c) `Outcome`/terminal handling stays SHIP→completed (the drain is paid for; parking on SHIP would contradict S02.3's park semantics — parked = suspended on an OPEN ask, and there is no ask).

**R7 — The verdict card and receipt name the posture in plain words, including that capturing the project's commands restores the full ladder [A14; S07.8; S07.11].**
- *Verdict card:* every drain card terminal raised under bootstrap (CAP-HIT / ESCALATE / REOPEN-SPEC / RESEARCH-NOT-RUN) carries a Detail line stating the posture; the per-round posture statement also reaches the requester-visible verdict surface. Server-authored plain strings through EXISTING wire shapes (Card.Detail, finding text → findings-as-comments → the `VerdictReader` fold in `web/src/Deliverable.tsx:779+`) so the landed frontend renders them with zero web changes.
- *Receipt:* the S10.10 receipt body gains a verification-posture statement for runs whose drain ran bootstrap rounds (omitempty member; see OQ5 for the composition home). Plain words, naming what capturing commands restores.
- The phrase may reference the project's Commands; it must NOT promise a UI door that does not exist yet (GF5/GF6) — mirror the pre-GF4 card's factual register without the dead-end imperative.

**R8 — Posture computed per revision from the registry's CURRENT capture [A14: "no residue"].** Every judged round resolves the posture fresh through the seam (rework rounds mint revisions within one drain: `MintCandidate`/`fixupRevision`). Once the capture holds a build/test/lint command, the next judged revision runs the full ladder and the advisory marking drops — zero bootstrap residue on post-capture rounds. The resume entry (`ResumeWithGuidance`) and every fresh `Verify` already construct from the seam's current answer; the in-drain rounds are the part to add. `AuditStale`/quarantine logic applies only when a real pack runs.

**R9 — Refusal terminals remain only for S07.7 integrity cases [A14].** Unchanged and still loud: `ErrBadPack` (unusable capture), nil-Judge / unvalidated judge seat, `RunV1` pack-validation refusal, and — per OQ1's recommendation, pending evaluator/operator confirmation — the task-attached-to-no-project refusal. A nil `Verifier.Pack` in a launch domain WITHOUT an explicit bootstrap resolution stays `ErrNoCheckPack` (a wiring defect is not bootstrap; bootstrap is computed from the registry, never a default — honest absence fails loud). `TestLaunchDomainRequiresPack` (`pipeline_test.go:170`) therefore SURVIVES.

**R10 — Recording [S07.11].** Bootstrap rounds record like any round: `verify.v1` + keep-forever `verdict.recorded` rows, now carrying the posture members. Additive payload members on already-minted event types need NO new type registration (`internal/eventlog/contract.go:456` registers the type; verify the S14.2 conformance tests pin no exact field set before landing — if the registration note's field list is asserted anywhere, update it in the same commit).

**R11 — A14 marker site [S00.9; A14 row: "the implementing code annotates internal/verify at its packet"].** The implementing site(s) in `internal/verify` carry the `[A14, 2026-08-27]` tag in their doc comments (the S07.8 bullet carries the spec-side tag already). Standard Go doc style — no research narration, no changelog prose (CONVENTIONS §2).

**R12 — Ledger verified-status under an advisory verdict [S05.1; S07.11]:** per OQ2's recommendation, an advisory SHIP does NOT `SetVerified` ledger items (they stay `done_unverified`; `Outcome.VerifiedItems` empty under bootstrap). The keep-forever verdict row is still written. If the evaluator/operator overrules OQ2, the round payload must then mark the verified flip advisory-sourced.

**R13 — No new ⚙ [A14: "No ⚙ setting's default or clamp is touched → no S18 re-sweep; tally stays 118/33"].** Consumed keys, by registry name, all pre-existing: `verification.rework_rounds`, `verification.convergence_patience_rounds`, `verification.sanity_stakes_floor` (not read on the launch-domain path), `verification.check_audit_interval_days` (full posture only), `verification.card_remind_hours` / `verification.card_push_hours` (card SLAs), `verification.research_rerun_limit` (research gate, unchanged). Any temptation to mint a posture toggle, advisory threshold, or bootstrap cap is OUT OF SCOPE — flag it, don't build it. Non-⚙ constants follow the CONVENTIONS §15 precedent (documented, settings-tab directive noted).

**R14 — Walls hold.** `internal/verify` imports gain nothing (never `internal/gates`, never `internal/project`); `internal/stage` keeps func seams over project (CONVENTIONS §23); `internal/metering` does not import `internal/verify` (see OQ5). `go build ./...` green at every commit; the red window is already open (see §6).

---

## 3. Seams to respect, and the one seam that changes

- **`stage.Config.CheckPackFor`** (`internal/stage/stage.go:253-265`) + **`shell.projectSeams.CheckPackFor`** (`project_seams.go:253-270`): THE seam this packet changes. Its answer set grows from {pack, nil-nil, error} to also express "bootstrap: registered project, no executable commands" — encoding is the executor's (a small resolution struct, or a flagged pack type; do NOT overload `(nil, nil)`, which already means "no pack machinery / non-launch degraded mode", and do NOT make a zero-check `CheckPack` pass `Validate()` — its `len(Checks)==0` refusal is a contract test). `skeleton.checkPack` (:1009-1021) forwards the distinction; `newVerifier` (:944-996) wires it into the Verifier.
- **`Verifier.Pack` / `Runner`** (`pipeline.go:55-61`, `validateInput` :238-256): the launch-domain nil-pack refusal at :249-250 gains the bootstrap branch; the wiring-defect refusal stays (R9). For R8's per-round recomputation the drain needs a resolver (an optional func field falling back to the static `Pack` — the static form remains the test posture).
- **Seams NOT touched:** `Judge` (S08), `WorkspaceProvider`/`VerificationWorkspace` (S13 — bootstrap runs no checks, so no workspace materialization is needed on bootstrap rounds; do not "fix" its honest absences), `ReviewSink` (S13.4 drain), `Research`/`ResearchRerun` (1.9 gate unchanged), `Tickets`, `Entailment` (idle), the recovery ladder, the S07.7 answer path verbs (`answer.go` — `retry` on an EXISTING pre-GF4 infra card re-enters the drain and now lands in bootstrap: that is the fix working, not a new verb; no new card kind, no new choices).
- **Stubs for phases not yet here:** GF5 (Commands write route) / GF6 (UI door) — the copy references capturing commands, nothing more. No stub code needed.

## 4. Files expected to change

- `internal/verify/pipeline.go` (bootstrap branch in `validateInput`; per-round posture resolution; posture on the round; R12 gating), `v1.go` (bootstrap V1 result builder — rungs + step contracts UNVERIFIABLE-HERE), `verify.go` (posture type/constants if introduced), `rework.go` (`RoundRecord` members), `record.go` (`roundPayload` members), `escalate.go` (card Detail line under bootstrap), matching `*_test.go`.
- `internal/stage/stage.go` (seam contract + doc), `skeleton.go` (`checkPack`, `newVerifier` wiring), tests.
- `internal/shell/project_seams.go` (`CheckPackFor`/`packFromCapture` bootstrap answer), `checkpack_gf4_test.go` (committed red — flips green), `checkpack_rw14_test.go` (superseded pins rewritten, see §6).
- Receipt home per OQ5: `internal/metering/receipt.go` (omitempty member) + the serving composition (`internal/stage/surface.go` or `internal/api`), tests.
- `web/src/fixtures/api/*.json`: expected ZERO drift if all new members are omitempty and the fixture world stays bootstrap-free (fixture regeneration is compare-only, CONVENTIONS §42). **If any fixture body moves, the vitest battery is owed at landing** (typecheck + vitest run recorded in the packet report), per the golden-fixture tie.
- NOT changed: `Spec/**` (A14 already landed), `Docs/**`, `P3/STATE.md`, `internal/settings/index.go` (byte-unchanged — no ⚙), `components.lock`, `web/src/Inbox.tsx` (the infra card remains for the surviving refusals; its copy stays true).

## 5. Adopted components touched

None. (Expect none; confirm none in the packet report.)

---

## 6. Existing tests this packet supersedes (rewrite, don't delete silently)

- `internal/shell/checkpack_rw14_test.go` `TestRW14NoCapturedCommandsNamesWhatToCapture` (:98-115): pins the refusal A14 abolishes — rewrite to pin the bootstrap answer's content (registered project named, no invented checks). Its sibling pins SURVIVE: `TestRW14CapturedCommandsBecomeTheLadderRungs`, `TestRW14NonLaunchDomainKeepsItsDegradedMode`, `TestRW14SoftwareTaskWithNoProjectRefusesWithTheDoor` (per OQ1).
- `internal/verify/pipeline_test.go` `TestLaunchDomainRequiresPack` (:170): SURVIVES as the wiring-defect refusal (R9).
- `internal/stage/verifyoutage_rw14_test.go` / `verifywedge_rw14_e2e_test.go`: the park-on-card machinery stays for surviving refusal classes; where a fake seam's cause string says "no commands captured", reword to a surviving cause (no-project or bad-pack) so the test names a world that still exists.

**Red window (CONVENTIONS §3 Amendment-A):** `internal/shell/checkpack_gf4_test.go` is committed FAILING by this grounding commit (2 tests red against `packFromCapture`; `go build ./...` green; scope = the packet's own paths). The executor's implementation commit closes the window.

---

## 7. Acceptance checklist (the evaluator's rubric)

1. Command-less registered project, software task, full pipeline: run does NOT park on an infrastructure card; a drain outcome exists. [R1]
2. The committed red tests (`checkpack_gf4_test.go`) are green; the superseded RW-14 pin is rewritten, not deleted-silently. [R1, §6]
3. Bootstrap round record: `V0` present and unchanged in behavior; `V1` present with every would-need-commands rung `UNVERIFIABLE-HERE` (never PASS, never absent) and every step contract `UNVERIFIABLE-HERE` with attribution; a `verify.v1` event row exists. [R2, R3]
4. No blocker findings originate from pack absence; the posture disclosure is note-class with a stable key; a bootstrap drain with a clean artifact reaches SHIP-with-notes (never parks on pack absence). [R4]
5. Both judge axes ran; the round record + `verdict.recorded` payload carry the advisory/bootstrap marking; a full-posture round carries none. [R5]
6. Trivial-tier bootstrap SHIP: run completes, deliverable state `in-review`, accept door available, NO auto-accept, `VerifiedItems` empty (per OQ2), ledger items still `done_unverified`. [R6, R12]
7. Card terminals raised under bootstrap carry the plain-words posture Detail including "capturing the project's commands restores the full ladder" (no dead-end imperative); the receipt body for a bootstrap run carries the posture statement. [R7]
8. Capture commands mid-lifecycle → the NEXT judged revision (in-drain round or a fresh drain entry) runs the executable ladder with zero bootstrap residue (no advisory mark, no bootstrap note). [R8]
9. Surviving refusals still refuse loudly and still park on the answerable card: no-project (per OQ1), bad pack, nil-pack-without-bootstrap-resolution, unvalidated judge. [R9]
10. `internal/verify` carries the A14 annotation; no new ⚙ (index.go byte-unchanged, tally 118/33); no adopted component; import walls hold; `gofmt`/`vet` clean; `go build ./...` + `go test -p 1 ./...` green at the implementation commit; zero fixture drift OR the vitest battery run and recorded. [R10, R11, R13, R14]

---

## 8. Acceptance-test specifications (write before implementation; names may be adjusted, assertions may not)

**Committed red in this grounding commit** (internal/shell):
- `TestGF4CommandlessProjectIsNeverARefusal` — registered project, `project.Commands{}`; `packFromCapture(DomainSoftware, e)`: asserts NOT `errors.Is(err, ErrNoCheckPack)`, `err == nil`, and no invented executable checks.
- `TestGF4RunPreviewOnlyCaptureIsStillBootstrap` — same with `{Run, Preview}` only.

**To write with the implementation** (spec'd here because they need the new surface):

- *internal/verify* `TestGF4BootstrapDrainShipsWithNotes`: launch-domain deliverable, bootstrap posture wired, clean artifact, passing fake judge; assert Outcome verdict `SHIP-with-notes`, `Card == nil`, round 1 record: `V1 != nil`, every rung outcome `UNVERIFIABLE-HERE` with non-empty detail naming the missing capture, every step contract `UNVERIFIABLE-HERE` with the attribution marker, posture member set, ≥1 note-class posture finding whose text contains the restores-the-full-ladder sentence, zero blocker findings from pack absence, `VerifiedItems` empty (OQ2).
- *internal/verify* `TestGF4BootstrapNeverParksOnPackAbsence`: same but the judge returns a blocker each round (REVISE to cap); assert the CAP-HIT card's cause is the blocker, its Detail carries the posture line, and no round's findings contain a pack-absence blocker.
- *internal/verify* `TestGF4PostureRecomputedPerRevision`: resolver returns bootstrap for round 1, a real 1-check pack from round 2; Revise seam wired; assert round 1 marked bootstrap, round 2 unmarked with the check executed (`PASS`/`FAIL` from the runner), and no bootstrap note on round 2. [R8]
- *internal/verify* `TestGF4WiringDefectStillRefuses`: launch domain, nil pack, NO bootstrap resolution → `ErrNoCheckPack` (keeps `TestLaunchDomainRequiresPack` company; both must hold). [R9]
- *internal/verify* record test: `verdict.recorded` payload for a bootstrap round unmarshals with the posture members; a full round omits them (omitempty proven both directions). [R5, R10]
- *internal/stage* `TestGF4BootstrapEndToEnd` (the wedge test's mirror, harness of `verifyoutage_rw14_test.go`): seam answers bootstrap → run reaches a drain terminal (completed on SHIP / parked on a REAL drain card), never the infrastructure card; kanban not `attention`-via-infra-park on SHIP. [R1, R6]
- *internal/stage* `TestGF4RetryOnPreGF4CardLandsInBootstrap`: seed a parked run with an open pre-GF4 infra ask (the `InfraAskPrefix` shape), answer `retry` with the project still command-less; assert the resumed drain produces a bootstrap outcome instead of re-parking on the identical refusal. [R1 — the operator's actual wall]
- *receipt home (per OQ5)*: bootstrap run's served receipt contains the posture statement; non-bootstrap run's receipt byte-identical to today's shape. [R7]
- *fixtures*: `TestWebAPIFixtures` regeneration shows zero drift (or the moved bodies + vitest run recorded). [§4]

**Property-based (mark as such; stdlib testing with a seeded generator or exhaustive enumeration — no new deps):**
- P1 [R1]: over all 2^5 subsets of captured {build,test,lint,run,preview}: bootstrap ⇔ build∧test∧lint all blank (whitespace counts as blank — `packChecks` trims); otherwise a valid pack with exactly the non-blank build/test/lint rungs.
- P2 [R3]: for arbitrary step lists (0..n steps, arbitrary IDs), the bootstrap V1 result: no outcome in state `PASS`, every produced state = `UNVERIFIABLE-HERE`, step-contract count == step count, every contract attributed.
- P3 [R4]: for arbitrary bootstrap rounds, no finding minted by the posture machinery has `SeverityBlocker`.

---

## 9. Open questions (recorded, not silently resolved)

**OQ1 — the no-project software task.** A14's text presupposes a project ("whose project has no captured check pack"; "capturing the project's commands restores the full ladder"; "computed per revision from the registry's current capture"). A task attached to NO project has no capture to compute from and no commands to restore — but A14 also says refusal terminals remain "never for pack absence". **Recommendation:** the no-project case stays a refusal card (its door — register the project, attach the task — exists today, so its `retry` CAN succeed; the r4 record's "retry cannot succeed" indictment does not apply), reading A14's "pack absence" as "a registered project's pack absence". A one-line seam change if the operator rules otherwise. The surviving RW-14 test pins this reading.

**OQ2 — does an advisory SHIP flip ledger items to verified?** `SetVerified` is "the only path to verified" (S05.1/S07.11) and the bootstrap V2 verdict is explicitly non-authoritative; the §16 `accept_best_effort` precedent keeps items `done_unverified` when no authoritative verdict exists. **Recommendation:** bootstrap SHIP does not `SetVerified`; the mandatory V3 accept is the human decision; the keep-forever verdict row still records everything. Risk to name: nothing at v0 consumes verified-status downstream in a way that would strand the task (accept works on the deliverable state, not the ledger flag) — verified in `review/accept.go` + `api/accept.go` this session.

**OQ3 — category of the posture disclosure note.** The S07.7 category enum is closed (a NEW category would need an amendment — the A14 row's own reasoning at RW-14 OQ1). **Recommendation:** `SeverityNote` + `CatCheckIntegrity`, the quarantine-skip precedent (`v1.go:432-436` — a suite fact, note-class); the integrity CARD raiser only fires on blocker-severity, so no card spam.

**OQ4 — S06.4's trivial-band completion card.** Not a code object at v0 (verified §1). GF4 records the mandatory-review fact durably (R6b) and pins no-auto-accept by test; building any completion-card surface is out of scope. Flag forward: whichever packet builds it must consume the posture member as its blocking condition.

**OQ5 — the receipt's posture home.** `internal/metering` must not import `internal/verify`, and the receipt materializes from the metering ledger at run-end. **Recommendation:** `metering.Receipt` gains a neutral omitempty member (shape owner; plain strings, no verify types); the value is composed where verify facts are already in reach — the stage/serve side (`stage` implements the intake surface's `Receipt` read; it may read the run's `verdict.recorded` posture and fill the member at materialization or serve time). Executor's choice within the walls; whichever home, checklist item 7 is the bar.

**OQ6 — spec-conflict check: none found.** A14's text, S07.8's surrounding bullets, S07.3's vocabulary, S07.7's route table, S06.4's band rule and the landed code admit a consistent landing as specified above; the only interpretive calls are OQ1–OQ5, each with a recommendation. No S18 sweep is triggered (no ⚙ touched — A14 row confirms, tally 118/33).

---

## 10. CONVENTIONS constraints that bind (read against `P3/CONVENTIONS.md`)

- **§3 tests:** stdlib testing only; colocated; `t.TempDir()` only; build+test green per commit — EXCEPT the declared Amendment-A red window opened by this grounding commit (scope: this packet's paths; closed by the implementation commit). Serial execution: `go test -p 1 ./...` (operator directive after the RW-14B runaway — one package at a time).
- **§15 verification conventions:** verify imports NEVER `internal/gates` (conformance-tested); verify events owner-attributed control-plane appends; escalation cards written in one tx with their event; CHECK-INTEGRITY never enters the REVISE drain; non-⚙ constants documented under the settings-tab directive.
- **§16 resume-in-place:** parked-on-card = blocked-not-failed; answers close ask + transition in one WriteTx; round numbering continues across carried history. GF4 must not fork a second resume substrate — bootstrap outcomes ride the same drain/park/answer machinery.
- **§22-§24:** S13 owns revision minting/comments/accept; findings land as comments through the ReviewSink; accept stays the S13.6 one-action broker-mediated verb.
- **§23:** stage/shell reach `internal/project` only through func seams wired at the composition root.
- **§42 golden fixtures:** fixtures are compare-only; regeneration via `SINET_WRITE_API_FIXTURES=1`; a Go-side shape change that moves a fixture body owes the vitest battery in the same landing.
- **§2 doc comments:** cite spec sections where they clarify; NO research narration, no changelog prose in code (the A14 tag cite of R11 is a spec citation, not narration).
- **Honest-absence-fails-loud (§14/§30/§40 lineage):** bootstrap is an EXPLICIT computed posture, never a default for an unwired seam (R9); absences are recorded (`UNVERIFIABLE-HERE`), never faked as PASS and never silently skipped.
- **Content-vs-telemetry (§30/§40-C/§44):** the posture statement is requester-facing CONTENT (card/verdict/receipt bodies — S13/S06-class product content, structurally exempt from the redaction edge); the event payload members are the durable record. Serve plain words, not error chains (the C2-13 raw-error ban; the `humanizeVerifySummary` precedent).
- **§5 commit/process:** stage files explicitly; never touch `Docs/`, `Spec/`, `Research/`, `P3/STATE.md`; packet sessions never push.

---

*Grounding commit: `P3-GF4 grounding: brief + red acceptance tests (S07.8 A14)` — this brief + `internal/shell/checkpack_gf4_test.go` (2 tests, committed RED; `go build ./...` green; window closes at the implementation commit).*
