> **EXPIRED 2026-09-17 — landed on main as merge `9322167` (grounding `5731d0f`, impl `b8dd07a`, fixture `61bc53e`, drain r1 `ed449ce`, drain r2 `07fe192`, coordinator inline `989a6d7`; landing battery 48 pkgs `-p 1` 0 FAIL 0 SKIP; CONVENTIONS §74). Single-use artifact: never read as truth; code + spec are the truth. SUPERSEDED at drain: §3.2's span rule (the landed rule is CONVENTIONS §74's); the removal guard is word-bounded with an asymmetric boundary; a symlinked/non-directory root is `workspace:unreadable`; absolute/root write globs are `plan:write-set`; the one sanctioned fixture edit (`internal/stage/e2e_test.go`, the fake executor writes `note.md`).**

# P3-TQ-3 — bootstrap stage contracts decided from the tree (TQ-F4)

**Grounded 2026-09-17.** Single-use brief for the executor and the evaluator. Truth is the spec (`Spec/drafts/S07-verification-quality.md`, `S00-front-matter.md` row A16, `S06-intake-pipeline.md`, `S12-local-models-tier.md`) plus the code at `ea22f89`; every claim below carries its file:line. No prior brief was read.

**The finding (TQ-F4, `P3/design/taskquality-webshop-findings-2026-09-16.md:21`; ratified `P3/gates/rework-sitting-gate.md:137`, mechanism at `:99`):** the webshop PLAN step S-2 contracted photos "downloaded into `public/images/**`"; the executor shipped hotlinked placeholders and no `public/images/` at all; nothing checked delivery against the plan's Done-when / Writes rows.

**The clause this packet implements, and ONLY this clause** — S07.8 bootstrap bullet, A16 sentence (S07 draft line 120; S00 row A16 line 196, item (3)): *"only rungs that genuinely need a missing command record UNVERIFIABLE-HERE — every Done-when contract decidable from the tree (write-set globs, named files, structural facts) is DECIDED at V1 by a deterministic or fresh-context model read of the tree, never from the executor's report."* The rescan-capture and the platform-authored browser walk (A16 items (1) and (2)) are P3-TQ-4, not this packet.

---

## 1. Code facts (what is actually there)

| # | Fact | Evidence |
|---|---|---|
| C1 | `bootstrapV1(pack, steps)` marks EVERY step contract `UNVERIFIABLE-HERE` attributed `check-pack:absent`; it takes no tree, reads no file. | `internal/verify/bootstrap.go:121-148` |
| C2 | The drain's bootstrap branch calls it with no workspace ("No workspace is materialized — there is nothing to materialize it for"). The pack branch, three lines later, resolves the §59 stripped verification workspace via `v.workspace(ctx, d, in.Workspace)`. | `internal/verify/pipeline.go:462-472` vs `:473-481`; resolver `:1033-1054` |
| C3 | `v.workspace` returns the S13 seam's materialized revision (`Verifier.Workspace`, wired by `internal/stage/skeleton.go:1042-1049` → `internal/shell/project_seams.go:349`), else the caller's dev path; `""` with no seam dir is `ErrBadInput`. | `pipeline.go:1035-1053` |
| C4 | Production `VerifyInput` is built ONLY at `internal/stage/skeleton.go:1082-1104`: `Steps: pair.Plan.Steps` (the approved plan, not the ledger), `Workspace: execCwd` (`RunRoot/<execute run>/cwd`, `:1063`), `EvidenceDir`. The plan's `Coverage` map is NOT passed. | `skeleton.go:1082-1104`; `grep verify.VerifyInput{` finds no other constructor |
| C5 | `intake.Step` carries `DoneWhen`, `WriteSet []string`, `Unbounded bool`; `Plan.Coverage map[string][]string` is "AC key → owning step id(s)". The wrote-nothing gate reads the write claim as "any non-empty step write_set OR unbounded" (`Plan.WriteGlobs()`). | `internal/intake/artifact.go:195-230, 301-303, 385-399`; `skeleton.go:1073, 1096`; `internal/verify/v0.go:129-147` |
| C6 | `StepContract{StepID, DoneWhen, State, AttributedTo, Category, Route}` has NO detail/reason field; `CheckOutcome` has `Detail`. | `internal/verify/v1.go:251-258` vs `:236-249` |
| C7 | NOTHING reads `V1Result.Steps` except the `verify.v1` event row and the judge slice (whole `V1Result` marshalled as Extra item `verify/v1-outcomes`). `ComputeVerdict` reads findings only. So a contract `FAIL` — in EITHER branch — has no verdict effect today. | `grep '\.Steps\b'` non-test: `bootstrap.go:138`, `pipeline.go:467,480`, `v1.go:484`; `internal/verify/v2.go:154-172, 380-407`; `record.go:156-158` |
| C8 | The ladder's `stepContracts` (`v1.go:492-524`) decides from check outcomes keyed by `StepID`: PASS when all bound checks passed (prose beyond them is the judge's), FAIL on any failure, UNVERIFIABLE-HERE when blocked upstream, **N-A when no check decides it**. Category defaults to `CatACBlocker`, route `RouteTable[CatACBlocker].Sink` = `decision_card`. | `v1.go:492-524`; `internal/verify/escalate.go:72-73` |
| C9 | `validateFindings` DEMOTES any blocker whose `Criterion` is not `CHECK-INTEGRITY`, an `AC-k` in the ledger's pinned set, or a rubric item id (`Demoted: true`, severity note). Round > 1 new notes are suppressed except the posture disclosure. | `v2.go:338-369` |
| C10 | `ValidateAxis1`: an AC executed at V1 takes the mechanical outcome; a judge disagreement is a CHECK-INTEGRITY blocker, never an override. That is the only "judge does not re-decide" mechanism, and it is keyed by `ACKey` on `CheckOutcome`, which bootstrap never sets. | `v2.go:239-291`; `v1.go:279-288` |
| C11 | A bootstrap round: `ReviewMandatory=true` + `PostureNote` on the record (`pipeline.go:400-406`); `SHIP` downgraded to `SHIP-with-notes` (`:560-562`); advisory SHIP never `SetVerified`s (`:609-616`); every card raised under the posture carries `BootstrapPostureNote` via `postureDetail` (`:449, 628, 652, 679, 867`). A blocker under bootstrap already reaches the CAP-HIT card (landed test `bootstrap_gf4_test.go:149-190`). | as cited |
| C12 | Runner-failure precedent at V1: a screen outage is recorded `RUNNER-FAILURE` + a CHECK-INTEGRITY blocker (card), never a verdict. `CheckResearch` precedent when its substrate is absent: `UNVERIFIABLE-HERE` + plain-words `Detail`, no finding. | `v1.go:437-452`; `v1.go:583-590` |
| C13 | The only `**`-aware glob code in the tree is intake's conservative claim intersection (`globsIntersect`: `**` intersects everything; literal prefix otherwise). No stdlib doublestar matcher exists; `filepath.Glob`/`path.Match` do not span `/`. | `internal/intake/spine.go:222-239`; `grep filepath.Glob` hits only `adapters/kimicli` |
| C14 | Local tier: the S12.4 alias registry is `AliasUtility, AliasIntakeTriage, AliasWatchdog, AliasWatchlist, AliasIntentFilling, AliasSQLOpen, AliasEntailment, AliasContradiction, AliasDistillSummarize, AliasEmbedder` — no verify-side duty. Adapters live in `internal/stage/local.go` (the SpotCheck seam rides `AliasIntakeTriage`, `:504-535`); a nil `Duty` is `ErrStackAbsent` and each seam degrades per its row (`internal/local/duty.go:88-92, 168-171`). | `internal/local/local.go:99-108`; CONVENTIONS §26 |
| C15 | The verify package's only import wall is "never `internal/gates`" (`effects_test.go:104-124`); nothing forbids stdlib `os`/`io/fs`/`path` reads. | as cited |
| C16 | The landed GF4 tests assert, for a step with DoneWhen `"checks pass"` and no write set: `State == UNVERIFIABLE-HERE`, `AttributedTo == BootstrapAttribution`, no blocker findings, `Workspace: "unused-dev-workspace"` (a path that does not exist). The internal property tests call `bootstrapV1(pack, steps)` over generated prose-only steps. | `bootstrap_gf4_test.go:34-147`; `harness_test.go:277-282`; `bootstrapv1_gf4_internal_test.go:20-110` |

## 2. Requirements (each with its S-ref)

- **R1 (S07.8 A16 (3); S07.3 stage contracts).** At a bootstrap-posture round, V1 reads the produced TREE — the same workspace the pack branch would run its ladder on (C2/C3: the §59 stripped revision when the S13 seam answers, else the caller's cwd) — and decides every PLAN step's Done-when contract that the tree can decide. The executor's report/transcript/deliverable text is never an input to the decision (the wrote-nothing gate's discipline, `v0.go:129-142`).
- **R2 (S07.3; A16 "write-set globs").** Class W: a step declaring a bounded write set (`len(WriteSet) > 0 && !Unbounded`) whose globs, as a union, match NO file in the tree is `FAIL` — the step promised writes and the tree holds none of them. This is the wrote-nothing gate's own reading of globs (union-level, C5), applied per step. A tree that satisfies every glob never yields FAIL from this class. Per-glob strictness is deliberately NOT applied: over-declared claims are the S02.8 safe direction (`spine.go:222-226`), and punishing them would teach planners to under-declare.
- **R3 (S07.3; A16 "named files").** Class N: a path-shaped span quoted in backticks in the Done-when line must be present in the tree: a glob → ≥1 matching file; a directory → ≥1 file under it (recursively); a file → exists and is non-empty (the A16 "structural fact"). Any absent one is `FAIL`. Parsed conservatively (§3.2): unquoted paths, bare filenames, stack names, versions and removal contracts are NOT decided.
- **R4 (S07.3 "N-A / UNVERIFIABLE-HERE"; S07.8 A14).** A contract no class applies to (no bounded write set, no path-shaped span, or a removal-worded line) stays `UNVERIFIABLE-HERE` attributed `check-pack:absent` — exactly today's state — but now with a plain-words `Detail` saying WHY it is not decidable from the files. Never `PASS`, never `N-A` (N-A is the ladder's word for "a suite ran and no check covered this"; at bootstrap no suite ran).
- **R5 (S07.3 PASS precedent, C8).** A contract with ≥1 applying class and no refutation is `PASS`, `AttributedTo ""`, with a `Detail` that names the tree facts decided and says in plain words that anything the line asks beyond those files stays with the judge and the requester. This mirrors the ladder's `stepContracts` PASS ("all its checks passed" — prose beyond the mechanical facts is the judge's, C8).
- **R6 (S07.2 "a screen outage escalates rather than approves", applied at V1; C12 CheckResearch precedent).** A tree the platform cannot read (walk error) makes every contract of an applying class `UNVERIFIABLE-HERE` attributed `workspace:unreadable` with the error in `Detail`; contracts of no class are unchanged (R4). No fabricated PASS, no fabricated FAIL, no crash, no finding (the pack branch's equivalent, a runner failure, is a mechanical outage of a RUN; here nothing ran).
- **R7 (S07.3 "declares its finding categories and their escalation routes"; S07.7 route table).** Every contract keeps `Category: CatACBlocker`, `Route: RouteTable[CatACBlocker].Sink` (`decision_card`), as today. `StepContract` gains `Detail string \`json:"detail,omitempty"\`` (additive; pre-existing rows decode unchanged — the `CheckOutcome.Detail` shape, C6). **[inert field ADDED at grounding so the red tests compile.]**
- **R8 (S07.7 "every verification finding terminates in a human-visible sink"; S07.5 blocker citation rule; C7/C9).** A `FAIL` contract materializes ONE blocker finding in `V1Result.Findings`: `Severity: SeverityBlocker`, `Category: CatACBlocker`, `Criterion`: the lowest-numbered AC whose coverage entry names this step (`VerifyInput.Coverage`, R9), or `""` when none does; `Anchor: "step:<StepID>"`; `Text`: plain words for the requester (§38: no citations, no spec ids, no tokens) — the step's id and Done-when line, what the files failed to show, and that the approved plan is the contract. The finding key `criterion|step:<id>|AC-BLOCKER` is round-stable, so a FAIL that persists across rework rounds recurs unresolved and trips the S07.6 convergence stop into the CAP-HIT card. A step no AC covers yields the same finding, which `validateFindings` demotes to a note (`Demoted: true`) — S07.5: "a finding that cites none can only be a note" — and it still reaches the requester as a review comment (`reviewable` keeps AC-BLOCKER notes, `review.go:76-85`).
- **R9 (S06.6 coverage map; C4/C9).** `VerifyInput` gains `Coverage map[string][]string` (AC key → step ids). **[inert field ADDED at grounding.]** The ONE line in `internal/stage/skeleton.go`'s builder (`:1098-1103`) is `Coverage: pair.Plan.Coverage,` — the executor adds exactly that line and nothing else in that file (other agents edit it concurrently).
- **R10 (S07.8 bootstrap: "V3 blocks at every stakes tier"; S07.7 AC-BLOCKER route; C11).** A FAIL contract therefore blocks exactly as the S07.7 AC-BLOCKER route says and as C11 already realizes for any blocker under bootstrap: `ComputeVerdict → REVISE` → a rework round with the finding in the retry package → at cap or with no `Revise` seam → the CAP-HIT decision card carrying the finding and `BootstrapPostureNote`; advisory SHIP never happens on a FAIL round; `SetVerified` is never reached. No new card kind, no new route, no receipt change (the receipt's verification line is the posture disclosure; per-finding copy is the review surface's and the card's — `internal/metering/receipt.go:66-69`, being edited by others, untouched).
- **R11 (S07.5 judge input slice; C7/C10).** The judge consumes the contracts as evidence exactly as today: the whole `V1Result` (now with `Detail`) rides the `verify/v1-outcomes` Extra item (`v2.go:166-172`) and `JudgeInput.V1`. The judge cannot flip a contract: no code path writes `V1Result.Steps` after V1, and the round record's `V1.Steps` is what V1 decided regardless of the judge's verdicts. The assertion to pin (T8): a judge that passes every AC leaves `Rounds[0].V1.Steps[S-2].State == FAIL`, `JudgeInput.V1.Steps` carries the FAIL, and the verdict is not SHIP.
- **R12 (S07.11 cost shape).** No model call at V1 in this packet: the decision is deterministic and free (§4 (2)). Zero receipt lines added.
- **R13 (S01.10 / S18; CONVENTIONS §2).** No ⚙ key is consumed or introduced. Structural constants with reasons, flagged to the settings-tab ledger: `contractDetailMaxExamples = 3` (example paths named in a `Detail` line — a rendering cap for a sentence a person reads). No tree-walk cap: the tree is the snapshot checkout the S07.3 ladder already reads whole, and a partial read would need a contract state the spec does not have.
- **R14 (S07.11 recording).** The `verify.v1` event row and `RoundRecord.V1` carry the decided contracts with `Detail` (they already carry `V1Result` whole, C7) — no schema or migration change.

## 3. The deterministic decision, exactly

### 3.1 Tree index
`indexTree(root string) (*treeIndex, error)`: `filepath.WalkDir(root)`; skip any directory named `.git` (the §59 workspace has none; the dev cwd may); regular files only (symlinks are not followed and not listed — §59 drops them anyway); paths recorded relative to `root`, slash-separated, sorted; directories recorded as a set. A walk error (root missing, unreadable) is returned, not swallowed (R6).

### 3.2 Parsing the Done-when line (class N), conservatively
Backtick-quoted spans only. A span (trimmed) is path-shaped iff ALL hold: no whitespace; does not contain `://`, `(`, `)`, `{`, `}`, `$`, `=`; contains `/` or any of `*?[`; its first byte is one of `[a-z0-9./*_]`. Everything else is prose and is not decided. **Removal guard:** if the line contains any of `remov`, `delet`, `no longer`, `gone`, `drop`, `unused` (case-insensitive), class N does not apply at all — presence cannot decide a removal, and a false FAIL costs a rework round while a false UNVERIFIABLE-HERE costs nothing the round did not already have. Normalization for both classes: trim; strip leading `./` and `/`; a trailing `/` means `<p>/**`.

### 3.3 Glob matching (stdlib only)
Segment-wise on `/`: a `**` segment matches zero or more path segments; any other segment matches one path segment through `path.Match`. A metachar-free pattern that names a directory in the index matches every file under it. `path.ErrBadPattern` on any segment → the pattern is malformed → its class records `UNVERIFIABLE-HERE` attributed `plan:write-set` (or `plan:done-when`) with the pattern named — plan text is a boundary input and is never allowed to crash the drain.

### 3.4 State resolution per step (precedence top-down)
1. any refutation (W union empty; N span absent/empty) → `FAIL`, `AttributedTo: "tree:" + <first refuted pattern>`, `Detail` naming each refuted pattern in plain words.
2. tree unreadable and (W or N applies) → `UNVERIFIABLE-HERE`, `AttributedTo: "workspace:unreadable"`.
3. malformed pattern and nothing refuted → `UNVERIFIABLE-HERE`, `AttributedTo: "plan:..."`.
4. ≥1 class applied, none refuted → `PASS`, `AttributedTo: ""`, `Detail` = facts (counts + up to 3 example paths per pattern) + the "beyond these files" sentence.
5. no class applies → `UNVERIFIABLE-HERE`, `AttributedTo: BootstrapAttribution`, `Detail` = why (no write set and no file or folder named / the line speaks of removal).

Category/Route fixed per R7. The four ladder rungs stay as today (`bootstrap.go:126-136`).

### 3.5 Signatures
```go
// bootstrap.go
func bootstrapV1(pack *CheckPack, steps []intake.Step, coverage map[string][]string, tree string) V1Result
// contract.go (new file)
func indexTree(root string) (*treeIndex, error)
func decideFromTree(step intake.Step, idx *treeIndex, walkErr error) StepContract
func namedPaths(doneWhen string) (paths []string, removal bool)
func contractFinding(sc StepContract, step intake.Step, coverage map[string][]string) Finding
```
`pipeline.go:462-472` bootstrap branch becomes: resolve `ws, cleanup := v.workspace(ctx, d, in.Workspace)` exactly as the pack branch does (same error path — one behavior, no special case; production always sets `Workspace`, C4), call `bootstrapV1(pack, in.Steps, in.Coverage, ws)`, run `cleanup`, record as today.

## 4. Answers the coordinator asked for

1. **Decidable classes and their deterministic decisions:** §3 — W (bounded write set vs tree, union-level, empty = FAIL; precedent `v0.go:143` / `WriteGlobs()`), N (backtick-quoted path-shaped spans: glob ≥1 file / dir ≥1 file / file non-empty), removal-guarded, conservative parser; precedence §3.4.
2. **Model read on the local tier: NOT in this packet — deterministic classes only; prose contracts stay UNVERIFIABLE-HERE with the honest reason.** Grounds: S12.4's cut line — "verification review (5.x) … run on paid frontier-class models and are NEVER assigned to local aliases" (S12 draft line 71) — and S12.4's registry carries no verify-side duty (C14); a new duty is owned by its section (S12.4: "Which duties exist is owned by their sections") and S07 names none, so adding one is S00.9 amendment territory, not a packet's; the S07.11 free-tier bound would be satisfiable in principle (a local alias is $0, `LineItem.ZeroAllowance()`, §26) but there is no ratified alias to ride, and riding `intake-triage` off-registry would be an invention. The smaller correct cut is deterministic-only. If the coordinator later ratifies a verify duty alias, the seam is `decideFromTree`'s step-5 branch (prose only) and the adapter belongs in `internal/stage/local.go`, degrading to UNVERIFIABLE-HERE attributed to the absent tier, never PASS.
3. **FAIL → requester:** R8 finding (`AC-BLOCKER`, sink `decision_card` per `RouteTable`), realized through the existing machinery: REVISE → rework → CAP-HIT card (`Findings` + `BootstrapPostureNote` in `Detail`) or, with an uncovered step, a demoted note on the review surface. Plain-words copy in R8; the receipt is unchanged. V3 is already mandatory at bootstrap (C11); an advisory SHIP cannot occur on a FAIL round because the blocker forces REVISE (R10).
4. **Judge consumes, never re-decides:** R11 — the assertion is T8 (`JudgeInput.V1.Steps` carries the FAIL; a pass-all judge leaves `Rounds[0].V1.Steps` FAIL and the verdict off SHIP).
5. **Property to pin:** T9 — for generated plans whose steps declare `d<k>/**` and generated trees, the recorded contract state equals the predicate "no file under `d<k>/` ⇒ FAIL, else PASS", independent of the deliverable's own success claims; a tree satisfying every glob never yields FAIL.
6. **⚙:** none consumed, none introduced, no S18 matter, no STOP. Structural constant `contractDetailMaxExamples = 3` flagged to the settings-tab ledger; no walk cap (R13).
7. **Non-bootstrap rounds:** the A16 sentence lives inside the S07.8 bootstrap bullet and opens "Restated as A14 already means it" — its scope is the bootstrap posture. The ladder path's N-A ("no check decides it; the judge weighs it at V2", C8) is the general S07.3 rule as landed, and applying the tree predicate there is a widening the text does not state. **Recommendation: bootstrap-only in this packet.** `decideFromTree` is written branch-agnostic so that, if the coordinator ratifies it, the ladder's N-A steps take the same call in `stepContracts` (OQ-1). A second pre-existing gap surfaced by C7 — a ladder contract FAIL (a failed non-AC check bound to a step) routes nowhere today, which is the "finding that died in a log" S07.7 forbids — is OQ-2 for the coordinator, not this packet.

## 5. Seams, settings, files, components, landed-test edits

- **Seams respected:** `Verifier.Workspace` (S13 seam, `pipeline.go:31-37, 71`) is consumed, not changed. `internal/project/workspace.go`, `internal/stage/{engines,skeleton,surface}.go`, `internal/worker`, `internal/metering/receipt.go` are being edited by other agents: the only touch is the ONE `Coverage:` line in `skeleton.go` (R9).
- **⚙ consumed:** none. **⚙ introduced:** none. Structural constant: `contractDetailMaxExamples` (R13).
- **Files expected to change:** `internal/verify/bootstrap.go` (signature + tree decision + findings), new `internal/verify/contract.go` (§3), `internal/verify/pipeline.go` (bootstrap branch resolves the workspace, passes coverage), `internal/verify/v1.go` (`StepContract.Detail` — done at grounding), `internal/stage/skeleton.go` (one line), `internal/verify/bootstrapv1_gf4_internal_test.go` (signature: `bootstrapV1(pack, steps, nil, t.TempDir())`; its "universally unverifiable" comment narrowed to "steps declaring no write set and naming no path" — assertions unchanged, the generator already produces exactly that shape), `P3/CONVENTIONS.md` (new § for this packet), `P3/STATE.md` (coordinator).
- **Landed tests that keep passing without edits:** `bootstrap_gf4_test.go` — its steps have no applying class and `"unused-dev-workspace"` is unreadable → R4/R6 leave every assertion (C16) true.
- **Adopted components touched:** none. Zero new Go modules (stdlib `io/fs`, `path`, `path/filepath`, `os`, `sort`, `strings`).
- **Migrations:** none. `web/`: none. `internal/local`: none.

## 6. CONVENTIONS constraints that bind

- §2 stdlib-first, `gofmt`/`go vet` clean; no ⚙ as constants (none here); errors wrapped `%w`.
- §3 stdlib `testing` only; `t.TempDir()` only; amendment-A red window declared in the commit message and closed by the implementation commit; `go build ./...` green throughout.
- §5 subject `P3-TQ-3: …`; explicit pathspec staging; never touch `Spec/`, `Docs/`, `Research/`, `P3/STATE.md` (executor).
- §15 verify never imports `internal/gates` (`effects_test.go`); verdicts touch no effect row; V1 outcomes reach the judge only via the clean-context `Assemble` Extra items; CHECK-INTEGRITY findings never enter the REVISE drain (this packet mints AC-BLOCKER, not CHECK-INTEGRITY).
- §26 the local tier is not touched; the honest-degradation posture is cited for the (declined) model-read seam.
- §58 no new card kind, no new route-table category; the card carries the ratified sink.
- §59 the verification workspace is the stripped revision; reading a fact must not create it (the walk is read-only; nothing is written to the tree); requester-facing prose is for the person.
- §70 bootstrap is computed, never defaulted; the posture disclosure stays identity-keyed and note-class; an advisory verdict releases nothing.
- §38 (via §46 note): no citations or internal tokens in requester-facing text.
- Test-run discipline (`serial-tests-gpu-only`): `go test -p 1 -count=1 -run 'TestTQ3' ./internal/verify/` only; never the full battery from this packet's sessions.

## 7. Acceptance checklist (each item is a test or a grep)

- [ ] A1 A step with `WriteSet ["public/images/**"]` and a tree holding nothing under it records `FAIL`, `AttributedTo` prefixed `tree:`, non-empty `Detail`, category `AC-BLOCKER`, route `decision_card`; the deliverable text claiming success changes nothing (T1).
- [ ] A2 The TQ-F4 shape — Done-when naming `` `public/images/**` `` in backticks, write set elsewhere satisfied — records `FAIL` attributed `tree:public/images/**` (T2).
- [ ] A3 A tree satisfying the write set and every named path records `PASS` with a `Detail` naming the facts; the round is `SHIP-with-notes`, no card (T3).
- [ ] A4 A prose-only contract records `UNVERIFIABLE-HERE` attributed `check-pack:absent` with a non-empty `Detail` (T4).
- [ ] A5 A removal-worded line with the path absent is never `FAIL`; its `Detail` says why (T5).
- [ ] A6 A FAIL on a step that covers `AC-1` yields a blocker `{AC-1, step:S-2, AC-BLOCKER}` with no citation tokens in its text, forces REVISE, and with no `Revise` seam lands the CAP-HIT card carrying the finding and the posture note; nothing is verified (T6).
- [ ] A7 A FAIL on a step no AC covers yields the same finding demoted to a note; verdict `SHIP-with-notes`; no card (T7).
- [ ] A8 The judge's input carries the FAIL; a pass-all judge cannot flip it; verdict is not SHIP (T8).
- [ ] A9 Property: recorded state == predicate over the tree for generated plans/trees; satisfied globs never FAIL (T9).
- [ ] A10 An unreadable workspace records `UNVERIFIABLE-HERE` attributed `workspace:unreadable`, no blocker, no card (T10).
- [ ] A11 `verify.v1` event payload carries `"detail"` for the decided step (T1 asserts).
- [ ] A12 `grep -n "Coverage: pair.Plan.Coverage" internal/stage/skeleton.go` hits exactly once; no other diff in that file.
- [ ] A13 `go test -p 1 -count=1 -run 'TestGF4' ./internal/verify/` green with `bootstrap_gf4_test.go` byte-unchanged.
- [ ] A14 No new ⚙ key (`go run ./tools/...` settings tally unchanged: 118); no `internal/gates` import in verify; no new module in `go.mod`.

## 8. Acceptance-test specifications — `internal/verify/tq3_contracts_test.go` (package `verify_test`), COMMITTED RED

Shared setup: `newFix(t)`, `f.seedTask("t1","r1")` (ledger ACs AC-1/AC-2), `f.verifier(&fakeJudge{}, nil, bootstrapPack())`, `input(deliverable("t1","r1"))` with `Steps`, `Coverage`, `Workspace` overridden; trees written under `t.TempDir()` by a `writeTree` helper. Real entry point `v.Verify`. Red cause today, for every test: `bootstrapV1` records `UNVERIFIABLE-HERE`/`check-pack:absent` with empty `Detail` for every step and mints no finding.

- **T1 `TestTQ3WriteSetRefutedByTreeIsFAIL`** — S-2 `WriteSet ["public/images/**"]`, DoneWhen prose; tree `{src/data/products.json}`; `Content` = "Verified by construction: images downloaded into public/images/". Assert `Rounds[0].V1.Steps` has S-2 `State == FAIL`, `strings.HasPrefix(AttributedTo, "tree:")`, `Detail != ""`, `Category == CatACBlocker`, `Route == RouteTable[CatACBlocker].Sink`; the last `verify.v1` payload contains `"step_id":"S-2"` and `"state":"FAIL"` and `"detail"`.
- **T2 `TestTQ3NamedPathInDoneWhenRefutedIsFAIL`** — S-2 DoneWhen "representative free stock photos downloaded into `public/images/**` and product data written to `src/data/products.json`", `WriteSet ["src/data/**"]`; tree `{src/data/products.json: "[...]"}`. Assert S-2 `FAIL`, `AttributedTo == "tree:public/images/**"`.
- **T3 `TestTQ3TreeSatisfyingContractIsPASSWithFacts`** — same plan as T2, tree adds `public/images/a.jpg` (non-empty). Assert S-2 `PASS`, `AttributedTo == ""`, `Detail` contains `public/images/**`; `out.Verdict == SHIP-with-notes`; `out.Card == nil`.
- **T4 `TestTQ3ProseOnlyContractStaysUnverifiableWithReason`** — S-3 DoneWhen "the price ranges are realistic and the data is believable", no write set. Assert `UNVERIFIABLE-HERE`, `AttributedTo == BootstrapAttribution`, `Detail != ""`.
- **T5 `TestTQ3RemovalWordingIsNeverDecidedByPresence`** — S-4 DoneWhen "the old `legacy/` folder is removed and nothing imports it", tree without `legacy/`. Assert `State != FAIL` and `Detail` contains "remov".
- **T6 `TestTQ3FailContractReachesTheRequesterAsACBlocker`** — T1's plan, `Coverage {"AC-1": {"S-2"}}`, judge passes all, `Revise == nil`. Assert `out.Card != nil`, `Category == CatCapHit`, `!Infrastructure`; a finding in `out.Card.Findings` with `Severity == SeverityBlocker`, `Category == CatACBlocker`, `Criterion == "AC-1"`, `Anchor == "step:S-2"`, `!Demoted`, `Text` free of `S07`, `Spec`, `§`, `AC-BLOCKER`, `check-pack:`; `strings.Join(out.Card.Detail, "\n")` contains "restores the full ladder"; `len(out.VerifiedItems) == 0`; `Rounds[0].Verdict == VerdictRevise`.
- **T7 `TestTQ3FailOnUncoveredStepIsANoteNotAGoalpost`** — as T6 with `Coverage {"AC-1": {"S-1"}}`. Assert a finding with `Anchor == "step:S-2"`, `Severity == SeverityNote`, `Demoted`; `out.Verdict == SHIP-with-notes`; `out.Card == nil`.
- **T8 `TestTQ3JudgeConsumesContractsAsEvidenceAndCannotFlipThem`** — T6's setup. Assert `j.inputs[0].V1 != nil` with S-2 `FAIL` in `.Steps`; `Rounds[0].V1.Steps` S-2 still `FAIL` after the pass-all judge; `out.Verdict` is neither SHIP nor SHIP-with-notes.
- **T9 `TestTQ3PropContractDecisionIsThePredicateOverTheTree`** — seeds 1..3, n = 1..4 steps `S-k` with `WriteSet ["d<k>/**"]`, prose DoneWhen; tree writes 0..2 files under `d<k>/` per step (seeded rand); `Content` alternates between a success claim and a failure claim. Expected per step: `FAIL` iff zero files under `d<k>/`, else `PASS`. Assert equality for every step and that the all-satisfied tree yields no FAIL. `Coverage nil` (findings demote → no card).
- **T10 `TestTQ3UnreadableWorkspaceIsUnverifiableNeverPass`** — T1's plan, `Workspace = filepath.Join(t.TempDir(), "missing")`. Assert S-2 `UNVERIFIABLE-HERE`, `AttributedTo == "workspace:unreadable"`; no blocker finding in `Rounds[0].Findings`; `out.Card == nil`.

Run: `go test -p 1 -count=1 -run 'TestTQ3' ./internal/verify/` — all ten FAIL at grounding (recorded in §10).

## 9. Open questions for the coordinator (none block the executor)

- **OQ-1** Apply `decideFromTree` to the ladder path's N-A steps (`stepContracts`, C8)? Recommendation: not in this packet (§4 (7)); one call if ratified.
- **OQ-2** A ladder contract `FAIL` (failed non-AC check bound to a step) routes nowhere today (C7) — the P46 class S07.7 forbids. Pre-existing; `contractFinding` is reusable there. Recommendation: a separate conformance packet.
- **OQ-3** R5's PASS semantics (mechanical facts hold ⇒ PASS, prose beyond them is the judge's) follows the landed ladder precedent (C8). The stricter alternative — facts hold ⇒ stay `UNVERIFIABLE-HERE` with the facts in `Detail` — was considered and not chosen because A16 says DECIDED and the precedent already reads PASS that way. Coordinator ratifies.

## 10. STOP conditions

None found. No paid call at V1, no new ⚙, no new dependency, no amendment needed.

## 11. Grounding run record

`go test -p 1 -count=1 -run 'TestTQ3' ./internal/verify/` at grounding: **10 of 10 FAIL**, each for the stated cause — T1/T2/T9 `S-2 contract "UNVERIFIABLE-HERE", want FAIL`; T3 `want PASS (detail "")`; T4/T5 empty `Detail`; T6 `a FAIL contract raised no card`; T7 `no finding anchored to step:S-2` (only the posture disclosure exists); T8 judge input carries S-2 `UNVERIFIABLE-HERE`; T10 attributed `check-pack:absent`, want `workspace:unreadable`. `gofmt -l internal/verify/` empty, `go build ./...` green, `go vet ./internal/verify/` green, `go test -p 1 -count=1 -run 'TestGF4' ./internal/verify/` green with the two inert fields in place (A13 holds before the executor starts).
