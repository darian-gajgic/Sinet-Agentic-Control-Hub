# P3-TQ-8 — the judge's input slice for repo-backed deliverables: the artifact plus its diff against the previous revision (S07.5)

Grounded 2026-09-18 on `main` at `b0149ac` (post P3-SIT-1, CONVENTIONS §78). Binding sections read in full: S07.3 (rule 1), S07.5, S07.9 (P-T06-3), S07.11, S05.3, S13.1, S13.2; CONVENTIONS §2–§5, §15, §22, §38, §59, §72–§78. Code read: `internal/verify/{v2.go,pipeline.go,verify.go,review.go,record.go,rework.go,v0.go}`, `internal/stage/{engines.go,skeleton.go,review_sink.go,runner.go,stage.go}`, `internal/review/{compare.go,tree.go,store.go,diff.go}`, `internal/project/{tree.go,workspace.go}`, `internal/shell/project_seams.go`, `internal/ledger/assemble.go`, `internal/worker/routing.go`, `internal/settings/index.go`, the judge harnesses (`verify/v2_test.go`, `verify/judgegate_test.go`, `verify/harness_test.go`, `stage/tq5_judgesession_internal_test.go`, `adapters/claudecli/rider1_golden_test.go`) and the SIT-1 fixtures (`review/sit1_tree_test.go`, `api/sit1_tree_test.go`, `stage/tq2_resume_e2e_test.go`). No prior brief was read. Every line number below is from this grounding's tree.

## 0. The finding, in the code

S07.5: the judge's input slice is "the artifact + its diff against the previous revision [S13] + the frozen ACs + the rubric version + V1 check outcomes as evidence + prior-round findings — never the execution transcript". S07.9 P-T06-3: "judges receive extracted claims/diffs, not presentation, wherever the deliverable type allows". CONVENTIONS §78: a repo-backed revision IS its tree at the snapshot pin; the step report is a companion object, never the deliverable.

What the judge receives today, for a software task whose deliverable is a tree:

- `stage/skeleton.go:1194-1246` `verifyInput`: `Content: content` (:1233) is `readDeliverable(taskID, revision)` — the executor's step report (`deliverable.md`, the S-8 self-report); `Type: "markdown"` (:1231); the pins ride as `SnapshotSHA/BaseSHA` from `RepoFacts` (:1216) and feed only the S07.2 wrote-nothing gate.
- `verify/v2.go:165-217` `BuildJudgeInput`: `extraItem(itemArtifact, d.Content, …)` (:173) and `extraItem(itemDiff, d.Diff, …)` (:174); `Artifact: d.Content` (:208). `stage/engines.go:926` `d.Diff = ""` on every rework revision ("the judge receives the full artifact"); production never sets `Diff` at all. So `verify/diff` is an empty block and `verify/artifact` is the report.
- `stage/engines.go:794-822` `session`: the wire is `in.BriefText + "\n" + instructions` (:812) — the Extra blocks rendered verbatim by `ledger.writeBlock` (`ledger/assemble.go:589-596`, one `=== [stage] <item id> v<version> ===` header per block); the judge seat's window rides `WindowTokens: seat.WindowTokens` (:817, §73).
- `verify/pipeline.go:529` `ValidateAxis1(ax1, input.ACs, v1Facts, d.Content)`: an axis-1 PASS needs an extractive quote (`v2.go:299` `strings.Contains(artifact, v.Evidence)`) — measured against the REPORT. The `axis1Schema` (`engines.go:755-763`) tells the judge "evidence MUST be an EXACT substring of the artifact". The judge is therefore instructed to quote, and validated against, the executor's prose about its work.

That is the webshop defect: the judge never saw that `public/images/` did not exist because the only thing on its wire was the report claiming it. §74 (TQ-3) already bars the report from step-contract decisions and §59's wrote-nothing gate refuses to trust it; V2 is the layer still reading it as the artifact.

What is now computable host-side (§78): `review.Store.Change(ctx, id, oldN, newN)` (`review/tree.go:202-260`) — the whole file inventory between two pins, revision 1 against the recorded pre-task base, never truncated; `CompareFile(ctx, id, oldN, newN, path)` (:443-467) — one file's unified diff by the one diff authority (`review.gitDiff`, `diff.go:33`), cut at a hunk boundary under `TreeFileDiffBytesCap`; `RevisionFile(ctx, id, n, path)` (:469-540) — full file content at the pin under `TreeFileBytesCap`. All three are READS over the platform-owned store at the pinned refs through `Store.Tree` (`review.TreeSource`, `tree.go:29`), wired by the shell (`shell/project_seams.go:723 wireReviewStore`; `projectSeams.TreeChanges` :757 over `project.Store.TreeChanges` `project/tree.go:107`, `TreeBlob` :313, `BaseSHA` :87). A pin the store no longer holds is `ErrContentDrift` (`tree.go:160-171 pinDrift`), never a fall-through to the companion report (§78 F1).

## 1. Requirements

Each requirement names the spec section that binds it.

**R1 — For a repo-backed revision the judge's artifact is the tree's CHANGE, and the report is claims.** [S07.5 "the artifact + its diff against the previous revision"; S07.9 P-T06-3; S13.1 "revision 1 is presented against the pre-task state; revision N against N−1"; §78]
The `verify/artifact` item becomes the change inventory (every row: kind, path, old path for a rename, sizes, +/− counts, binary flag — the table of contents, never truncated) plus the FULL new-side content of each modified/renamed text file that fits the bound (the diff alone shows hunks; the judge must be able to quote the file), plus the honest omitted lists (R3). The `verify/diff` item becomes the shown per-file unified diffs in path order, concatenated with no separators (the §22 rule: the headers name the file). An added file's diff IS its content and is not repeated in the artifact item; a deleted file appears as its `-` diff; a binary file has an inventory row and no bodies — its bytes never reach the judge. The step report rides as a NEW Extra item `verify/executor-report` ("what the executor says it did — claims, never the artifact") carried on `JudgeInput.Report` (`v2.go:125`, inert); it is never part of the quotable text. Nothing rides the wire twice (the diff of an added file appears exactly once; the report appears exactly once).

**R2 — The diff comes from the platform-owned store at the pinned refs, through one new seam; never the sandbox.** [S07.3 rule 1 (the verification workspace is stripped of answer-bearing history — it is not a diff source); S13.1 content pin; §59 "reading a fact must not create it"; §22 verify never imports review; §78 read-only verbs]
`verify.ChangeSource` (`verify/judgeslice.go`, inert at grounding): `RevisionChange(ctx, d Deliverable, bodyBudget int) (RevisionChange, error)`. `Verifier.Change ChangeSource` (`pipeline.go:105`, inert) — nil keeps today's slice. The stage's `reviewSink` (`stage/review_sink.go:76`) implements it (it already owns the task → deliverable identity `TaskDeliverableID`, :33) over `rs.store()`: `Change(ctx, id, d.Revision-1, d.Revision)` (old 0 = the pre-task base) for the inventory and the pins — the pins come from the review store's OWN revision rows, `d.SnapshotSHA` is never consulted (the revise path copies a stale pin, `engines.go:918-926`); then, in path order, `CompareFile` per non-binary row for `Diff/DiffTruncated/DiffReason`, then `RevisionFile` per modified/renamed non-binary row for `Content/ContentTruncated/ContentReason`; body reads stop once `bodyBudget` bytes are read (rows past it keep their inventory data and carry `BodySkipped`); the inventory is always whole. `AbsentReason` (a revision that pins no snapshot; no base recorded; no tree source composed — review's own sentences from `tree.go:212-245`) is an ANSWER: the slice keeps today's content shape and the reason is recorded (R6). Any seam error — `ErrContentDrift` on a lost pin included — fails the round BEFORE any judge call and writes no `verify.round` row: never a silent fall-through to the report. `newVerifier` (`skeleton.go:1086-1140`) wires `Change: sink` beside `Review: sink` (:1132) whenever the review store is composed; the drain asks the seam once per judged round, after the mint (`pipeline.go:424`; the round's revision and its pin exist in the store from that point) and before `BuildJudgeInput` (:516). `VerificationWorkspace` (`skeleton.go:1136`) and `RepoFacts` stay what they are and are not diff sources.

**R3 — The slice is bounded at a FILE boundary, the bound is visible to the judge, and it is never silent.** [S05.3 stage-fit budget (⚙ `context.stage_fit_target`, the brief's own footprint counts inside it); S07.11 recording; §78 "bounded, and honest about the bound"]
`verify.JudgeArtifactBytesCap = 192 << 10` (`judgeslice.go`, inert) bounds the BODIES on the judge's wire — diff bodies plus content bodies together. Reason (in the constant's doc comment): the judge seat's window is 200k tokens (`worker/routing.go:140 DefaultWindowTokens`, `:186 DutyJudge`) × ⚙ `context.stage_fit_target` 0.50 (`settings/index.go:245`; consumed by the budget watcher `stage/runner.go:645`) = 100k tokens for the whole prompt; 192 KiB of code-shaped text ≈ 65k tokens at ~3 bytes/token (conservative), leaving ≥ 35k for the ACs, rubric, V1 outcomes, prior findings, the report, the instructions and the answer. The §78 caps (`review/tree.go:137-139`: 512 KiB whole change / 256 KiB per file / 1 MiB content) are wire caps for a person reading one page; the judge's is its own structural bound with its reason, NOT a ⚙ (S18 ratifies no key; the §78 sibling caps' precedent; settings-tab ledger — see §8). Rule, rendered by the pure `verify.RenderChangeSlice(rc) (artifact, diff string, saw JudgeSaw)` (`judgeslice.go`, stub at grounding): the inventory first, whole; then diffs in path order, each whole or omitted — the first diff that does not fit stops the diff section and every later diff is omitted (a contiguous path-order prefix, the `review.changeDiff` shape `tree.go:303-343`, so "the first N of M" is literally true); then the content of modified/renamed files in path order under the REMAINING budget, same rule. A `BodySkipped` row counts as omitted. Every omitted file is NAMED on the artifact item beside its inventory row, with a plain-words line saying how many of how many are shown and that the judge's bound cut it (§38 plain words; no raw byte counts in requester-facing prose — this is judge-facing, a "192 KB" figure is fine). Never a mid-file cut, never a diff of a partially read file. The S05.3 budget watcher stays the measured backstop (`runner.go:668 FitExceeded`, recorded).

**R4 — Axis-1 evidence is validated against the tree slice: the quotable text is the artifact item plus the diff item; the report never satisfies a PASS.** [S07.5 "mandatory extractive evidence quote"; S07.10; P-T06-3]
`ValidateAxis1` (`v2.go:261`) is unchanged in body; its 4th argument at `pipeline.go:529` changes from `d.Content` to the judge input's quotable text: `Artifact` when `Diff == ""`, else `Artifact + "\n" + Diff` (add `func (in JudgeInput) Quotable() string` in `judgeslice.go`). For a content-pinned deliverable with an empty diff (production: always) this is byte-identical to today. A PASS whose quote exists only in `Report` is forced `Unknown` with `Forced: "unknown: non-extractive evidence"` (:299-303 already does this once the artifact argument is right). The `axis1Schema` (`engines.go:755-763`) names the rule the same way: the evidence must be an EXACT substring of the `verify/artifact` or `verify/diff` items; the `verify/executor-report` item is the executor's own account and never counts as evidence. (Pinned by `TestTQ8ComplianceInstructionsNameTheQuotableItemsAndTheReport` on the item ids — the brief's block headers, the one stable spelling.)

**R5 — `BuildJudgeInput` takes the slice; the manifest stays the S15-clean assembly.** [S05.4 clean-context exception; §15 judge seam: "the input slice is built ONLY through the ledger's clean-context `Assemble` with the S07-owned Extra items"]
Add `type JudgeSlice struct { Artifact, Diff, Report string; Saw JudgeSaw }` (`judgeslice.go`) and change `BuildJudgeInput(ctx, store, d, rubric, v1, prior, round)` (`v2.go:165`) to `BuildJudgeInput(ctx, store, d, slice JudgeSlice, rubric, v1, prior, round)`: `verify/artifact` ← `slice.Artifact`, `verify/diff` ← `slice.Diff`, and — ONLY when `slice.Report != ""` — a `verify/executor-report` Extra item (id constant `itemExecutorReport` beside `itemArtifact` at `v2.go:151`), placed after `verify/diff` and before `verify/rubric`; `JudgeInput.Artifact/Diff/Report` ← the slice. The drain builds the slice: repo-backed (seam answered without `AbsentReason`) → `RenderChangeSlice(rc)` with `Report = d.Content`; otherwise `JudgeSlice{Artifact: d.Content, Diff: d.Diff, Saw: JudgeSaw{Kind: "content", AbsentReason: rc.AbsentReason, ArtifactBytes: len(d.Content)}}`. Callers to move: `pipeline.go:516` and the one test call `verify/v2_test.go:40` (mechanical — §73's lesson: a test that reads a signature the packet changes moves with it). No transcript field appears anywhere; `Clean: true` stays.

**R6 — The round record says what the judge saw.** [S07.11 "every verdict is recorded with its reasons, per round … what was checked"; G2 Def.11 keep-forever]
`RoundRecord.JudgeSaw *JudgeSaw` (`rework.go:119`, inert) is set EVERY judged round: kind `tree` with pins, `Files`, `DiffsShown/DiffsOmitted` (paths), `ContentShown/ContentOmitted`, `DiffBytes/ContentBytes/ArtifactBytes/ReportBytes`, `Truncated`; kind `content` (with `AbsentReason` when the seam said so; without it on a drain with no seam) and `ArtifactBytes = len(Content)`. `roundPayload` (`record.go:58-98`) gains `JudgeSaw *JudgeSaw \`json:"judge_saw,omitempty"\`` and `RecordRound` (:162-183) copies `rec.JudgeSaw` into it, so the `verify.round` row (keep-forever, :182) carries it verbatim.

**R7 — The cost shape does not move.** [S07.11 "≤ rounds × 2 judge calls"]
Still exactly one `Compliance` and at most one `Sanity` call per judged round; the seam's reads are host-side git ($0); a bigger prompt is allowance under R3, never a new call.

**R8 — Content-pinned deliverables are byte-identical on the wire; the golden set stays valid.** [S07.10; P-T06-5]
Every non-project task (the walking-skeleton content lane, composer definitions), every bare `Verifier` (no `Change`), and every seam answer with `AbsentReason`: `Artifact == d.Content`, `Diff == d.Diff`, `Report == ""`, no `verify/executor-report` item, `BriefText` byte-identical to a drain without the seam. The rider-1 golden harness builds its own prompt (`adapters/claudecli/rider1_golden_test.go:60-68`) and never calls `BuildJudgeInput`: untouched. The verify harness's `passAll` (`harness_test.go:169`) quotes `in.Artifact`, a substring of the quotable text on both lanes: still valid.

**R9 — Property invariants (§7 of the packet).** [S07.5; S13.1; §78]
For any `RevisionChange`: the artifact names every inventory row; every diffable row is either shown whole in the diff item or named as omitted (never both, never cut mid-file); the shown diffs are a contiguous path-order prefix; every modified/renamed row's content is either shown whole or named as omitted; `DiffBytes + ContentBytes ≤ JudgeArtifactBytesCap`; binary bytes never appear; `saw` counts equal what was rendered; `Truncated` ⇔ something was omitted; `Kind == "tree"`. For any content-pinned deliverable: the slice is byte-identical to today's.

## 2. Seams

| Seam | Status in this packet |
|---|---|
| `verify.ChangeSource` → `stage.reviewSink` (NEW; `judgeslice.go`, `review_sink.go`) | the ONE way the judge's tree slice is read; nil = today's slice; wired in `newVerifier` beside `Review` |
| `verify.ReviewSink` (`verify/review.go`) | untouched — `MintCandidate` at the round top is what makes the round's revision readable by the new seam |
| `review.TreeSource` → `shell.projectSeams` (§78) | untouched; the seam reaches the store through `review.Store.Tree` (§23: stage and review never import `internal/project`) |
| `stage.Config.RepoFacts` / `VerificationWorkspace` (§59) | untouched; NOT diff sources (S07.3 rule 1) |
| `ledger.Store.Assemble` (`Clean: true`, §15) | untouched; one more S07-owned Extra item (`verify/executor-report`) on the same manifest |
| judge seat + window (`engines.go:806-817`, §73) | untouched; the S05.3 budget watcher is the measured backstop for R3 |

## 3. ⚙ settings

None consumed, none new. `JudgeArtifactBytesCap` is a structural constant with its reason (R3), the §78 caps' precedent and §15's non-⚙ list. **Settings-tab ledger entry (the operator's standing directive):** `verify.JudgeArtifactBytesCap` (192 KiB) — a window-derived bound (`WindowTokens × stage_fit_target × bytes/token`) is the refinement path once the judge's window is visible to verify; until then the S05.3 watcher records any over-fit prompt. No STOP condition: the constant's reason is the ratified S05.3 budget, and a ⚙ default here would be a new S18 key the spec does not ratify.

## 4. Files expected to change (executor)

- `internal/verify/judgeslice.go` — fill `RenderChangeSlice`; add `JudgeSlice`, `JudgeInput.Quotable()`; remove the INERT notices.
- `internal/verify/v2.go` — `:151` add `itemExecutorReport = "verify/executor-report"`; `:165-217` `BuildJudgeInput` takes `slice JudgeSlice`, emits the report item only when non-empty, sets `Artifact/Diff/Report` from the slice; the `JudgeInput` doc (:104-127) loses the inert notice. **Stay out of `validateFindings` (:417+, P3-TQ-7).**
- `internal/verify/pipeline.go` — inside the V2 block only (`:514-529`): call the seam (when `v.Change != nil`), build the slice, pass it to `BuildJudgeInput`, pass `input.Quotable()` to `ValidateAxis1`, set `record.JudgeSaw`. The `Verifier.Change` doc (:100-105) loses the inert notice. **Stay out of `:287` and `:478` (P3-TQ-4a).**
- `internal/verify/record.go` — `roundPayload.JudgeSaw` + the copy in `RecordRound`.
- `internal/verify/rework.go` — the `JudgeSaw` field doc loses the inert notice.
- `internal/stage/review_sink.go` — `func (rs reviewSink) RevisionChange(...)` per R2.
- `internal/stage/skeleton.go:1132` — `Change: sink,` beside `Review: sink,`.
- `internal/stage/engines.go:755-763` — `axis1Schema` names the quotable items and the report item (R4). No other engines.go change.
- `internal/verify/v2_test.go:40` — the one `BuildJudgeInput` call site moves with the signature (pass `verify.JudgeSlice{Artifact: d.Content, Diff: d.Diff}`); its assertions are unchanged.
- `P3/CONVENTIONS.md` — §79 (coordinator, at landing).

Not touched: `internal/review/*`, `internal/project/*`, `internal/shell/*`, migrations, settings, `web/`, `verify/v1.go`, `verify/bootstrap.go`, `verify/verify.go`, `verify/v0.go`, the retry package (`rework.go:37 BuildRetryPackage` still carries `Deliverable.Content` — the executor's rework input is S07.6's, not the judge's), `Deliverable.SHA256()` (`verify.go:177`, still over the report — the content pin of the artifact-store copy), `stop.observe` (`pipeline.go:695`, still over the report — pre-existing, see §9).

## 5. Adopted components touched

None. `git` (host CLI, os-mechanism lock entry, §22) is already the diff authority; no new module (`go run ./tools/lockgate`: OK at grounding).

## 6. Acceptance checklist

1. Repo-backed round: `JudgeInput.Artifact` names every inventory row; `Diff` holds the added file's diff (its content), the modified file's hunk and the deleted file's `-` lines; `Artifact` holds the modified file's full content; the report is on `Report` and on the `verify/executor-report` manifest item, and in neither `Artifact` nor `Diff`; the added file's content and the report each ride `BriefText` exactly once; binary bytes absent. → `TestTQ8RepoBackedJudgeSeesTheChangeNotTheReport`
2. An empty tree change at revision 2 is still the tree shape (`JudgeSaw.Files == 0`, kind `tree`), never the report as artifact. → same test, sub-case
3. Seam called once per judged round with `bodyBudget == JudgeArtifactBytesCap`; one compliance + one sanity call. → same test
4. A PASS quoting the report only → forced Unknown `"unknown: non-extractive evidence"`; a PASS quoting the added file's diff stands. → `TestTQ8QuoteFromTheReportDoesNotSatisfyAPass`
5. Over-bound change: the first three 50 KiB diffs shown whole, the fourth and everything after omitted (no mid-file cut), the small modified file's content still shown from the remainder; `DiffBytes == 3 × 50 KiB`; every omitted path appears ≥ 2× in the artifact (inventory + named as omitted); `JudgeSaw{Truncated, Files 7, DiffsShown 3, DiffsOmitted [a04,a05,a06,m01.go], ContentShown 1}`; `ArtifactBytes == len(Artifact)`, `ReportBytes == len(report)`; the `verify.round` payload's `judge_saw` says the same. → `TestTQ8SliceIsBoundedAtAFileBoundaryAndSaysSo`
6. Content-pinned: `Artifact == Content`, `Diff == ""`, `Report == ""`, no report item, `BriefText` byte-identical with and without the seam; `JudgeSaw{Kind: content, AbsentReason: <seam's>, ArtifactBytes: len(Content)}`. → `TestTQ8ContentPinnedSliceIsByteIdenticalAndRecorded`
7. Seam error → `Verify` returns it (wrapped, `errors.Is`), zero judge calls, zero `verify.round` rows. → `TestTQ8ALostPinFailsTheRoundLoudly`
8. R9 invariants over 150 random changes through `RenderChangeSlice`. → `TestTQ8PropSliceInvariants`
9. Real composition (stage over a real git project store): `stage.ChangeSourceOf(sk)` is the sink; rev 1 vs the recorded base and rev 2 vs rev 1 return exactly the written trees (kinds, hunks, added-file diff with empty `Content`, modified-file `Content`, binary row without bodies, path order, nothing truncated); a content-pinned revision answers `AbsentReason`; a revision minted on a sha the store never held answers `review.ErrContentDrift`. → `TestTQ8StageSinkServesTheChangeFromThePlatformStore`
10. `newVerifier` composes `Change` whenever the review store is wired. → `TestTQ8NewVerifierWiresTheChangeSeam`
11. `axis1Schema` names `verify/artifact`, `verify/diff`, `verify/executor-report`. → `TestTQ8ComplianceInstructionsNameTheQuotableItemsAndTheReport`
12. Batteries: `go build ./...`, `gofmt -l`, `go vet`, `go run ./tools/lockgate` green; `go test -p 1 -count=1 ./internal/verify/` green; `go test -p 1 -count=1 -skip 'TestLivePhraseAndSummarize|TestLiveIntakeTriageClassifiesWebshop' ./internal/stage/` green (§77: the whole package, never a `-run` allowlist); `./internal/review/ ./internal/shell/ ./internal/api/` unchanged and green.

## 7. CONVENTIONS constraints that bind

- §2 stdlib-first; structural constant with its reason, no inline ⚙; `%w` wrapping (the seam error surfaces with `errors.Is`).
- §3 Amendment-A: this grounding commits the nine tests RED with inert type surface (`judgeslice.go`, `Verifier.Change`, `RoundRecord.JudgeSaw`, `JudgeInput.Report`, the two `export_test.go` hooks); `go build ./...` stays green; the red window is declared in the commit message and closed by the executor's implementation commit. Nothing else red — proven: full verify battery 6 red (all `TestTQ8*`), full stage battery 3 red (all `TestTQ8*`).
- §5 subject `P3-TQ-8: …`; explicit pathspec staging; packet sessions never push, never edit `Spec/`, `Docs/`, `Research/`, `P3/STATE.md`.
- §15 the judge slice is built ONLY through `Assemble(Clean: true)` with S07-owned Extra items; there is no transcript field; `Compliance`/`Sanity` stay separate calls.
- §22/§23 `internal/verify` never imports `internal/review`; `stage`/`review` never import `internal/project`; the adapter (`reviewSink`) owns the identity mapping; `review.Store.Drain` remains the one consumption path (untouched).
- §59 reading a fact must not create it: the seam is reads only (`Change`/`CompareFile`/`RevisionFile` over the pinned refs); no snapshot is taken by verify; the stripped workspace is not a diff source.
- §73 the judge session runs on the judge seat with its window; the S05.3 watcher's `FitExceeded` is recorded, never silent.
- §74/§77 the report never decides anything — here it is a labelled claims item; TQ-6's disagreement detector consumes `ValidateAxis1`'s output unchanged.
- §78 the lane is keyed on the PIN (the review store's revision rows), the inventory is never truncated, a lost pin is `ErrContentDrift`, absences are answers (`AbsentReason`), plain words on the wire.

## 8. Answers to the packet's eight questions (with evidence)

1. **What `Artifact`/`Diff` become:** tree → `Artifact` = inventory + full content of modified/renamed files that fit + omitted lists; `Diff` = shown per-file unified diffs (added files ARE their diff); the report → `JudgeInput.Report` + `verify/executor-report` item, never quotable. Content-pinned → unchanged (`v2.go:173-174,208` today; `engines.go:926` empties `Diff`).
2. **The bound:** `JudgeArtifactBytesCap = 192 KiB` over bodies, file-boundary cut, contiguous path-order prefix, inventory whole, omitted named on the wire, recorded in `JudgeSaw`; derived from `DefaultWindowTokens` 200k (`worker/routing.go:140,186`) × `context.stage_fit_target` 0.50 (`settings/index.go:245`; `runner.go:645`). The §78 caps (`tree.go:137-139`) stay the wire caps.
3. **Axis-1 quotes:** validated against `Quotable()` = artifact item + diff item; the report is outside it. Proven red: today AC-1's report quote stands and AC-2's diff quote is forced Unknown (`TestTQ8QuoteFromTheReportDoesNotSatisfyAPass`, inverted at `pipeline.go:529`).
4. **Source:** `review.Store` on the platform-owned store at the pinned refs (`tree.go:202,443,469` over `Store.Tree` → `shell/project_seams.go:757` → `project/tree.go:107,313,87`), reached through the NEW `verify.ChangeSource` seam implemented by `stage.reviewSink`, wired in `newVerifier` (`skeleton.go:1113-1132`). Not `RepoFacts` (`skeleton.go:1216` — pins only), not `VerificationWorkspace` (`skeleton.go:1136` — stripped, S07.3 rule 1).
5. **Cost shape/golden set:** unchanged — same two calls per round (`pipeline.go:520,543`); rider 1 builds its own prompt (`rider1_golden_test.go:60-68`); content-pinned slices byte-identical (checklist 6 — its wire half already PASSES today, only the record is red).
6. **Recording:** `RoundRecord.JudgeSaw` (`rework.go:119`) → `roundPayload.judge_saw` in `RecordRound` (`record.go:162-183`), keep-forever (:182).
7. **Invariants:** R9, `TestTQ8PropSliceInvariants` (150 random changes) + checklist 6.
8. **⚙:** none; structural constant with reason; settings-tab ledger entry; no STOP.

## 9. Ledger (not this packet; stated honestly)

- Convergence similarity (`stop.observe`, `pipeline.go:695`) still compares REPORTS across rounds for repo-backed tasks; the tree's own sameness (equal pins across rounds) would be the honest measure — carried to the coordinator.
- The report item is unbounded on the wire (today's posture, unchanged); reports are KBs in practice.
- Modified files ride twice (hunks in the diff item, full content in the artifact item) by design, so the judge can quote outside the hunks; contents are the first thing the bound drops.
- N+1 host-side git reads per judged round (one `Change` + one `CompareFile` per text file + one `RevisionFile` per modified file), bounded by `bodyBudget` and the §78 per-file caps; $0.
- A window-derived cap for smaller-window judge seats (a future K3 judge) needs the seat's window visible to verify — the settings-tab ledger entry above.
- `skeleton.go:1231` still types the deliverable `"markdown"` for the V0 shape check of the report — correct for what it checks (§78); the review row's dtype is decided at the mint.
- The revise path's copied `d.SnapshotSHA` (`engines.go:918-926`) is stale on rework revisions; harmless here because the seam reads pins from the store rows, noted so nobody builds on that field.

## 10. Acceptance-test specifications (committed RED in this grounding)

Run with `go test -p 1 -count=1 -run 'TestTQ8' ./internal/verify/` and `go test -p 1 -count=1 -run 'TestTQ8' ./internal/stage/` to see the red reasons; the packet's landing battery is checklist 12.

**`internal/verify/tq8_judgeslice_test.go`** (package `verify_test`; fake `ChangeSource` `tq8Change` over an in-memory change, the `sit1Trees` pattern; the real verify drain harness `newFix/seedTask/verifier/input`)

| Test | Setup | Assertions | Red reason observed |
|---|---|---|---|
| `TestTQ8RepoBackedJudgeSeesTheChangeNotTheReport` | `tq8Tree()` (README deleted, logo.png added binary, app.go modified with content, cart.go added), deliverable `Content = tq8Report`, pins `s1/s0`, `WriteClaimed` | checklist 1–3; `JudgeSaw{tree, Files 4, DiffsShown 3, ContentShown 1, !Truncated, OldIsBase, NewPin s1}`; sub-case: empty change at rev 2 | `inventory row "README.md" missing from the judge's artifact` — the artifact is the report |
| `TestTQ8QuoteFromTheReportDoesNotSatisfyAPass` | build-only pack (no AC bound at V1); judge PASSes AC-1 with "wired the header" (report only) and AC-2 with "func Cart() {}" (added file's diff) | AC-1 Unknown + `Forced == "unknown: non-extractive evidence"`; AC-2 Pass, not FromV1 | AC-1 stands (`Pass:true Forced:""`) — validation reads the report |
| `TestTQ8SliceIsBoundedAtAFileBoundaryAndSaysSo` | `tq8WideTree()`: a01..a06 added at 50 KiB each + m01.go modified (1 KiB diff, 10 KiB content); rev 2 | checklist 5, incl. the `verify.round` payload's `judge_saw` | `JudgeSaw not recorded` |
| `TestTQ8ContentPinnedSliceIsByteIdenticalAndRecorded` | plain `deliverable()` twice on separate fixtures, seam absent vs seam answering `AbsentReason` | checklist 6 | wire assertions PASS today; `JudgeSaw = <nil>` |
| `TestTQ8ALostPinFailsTheRoundLoudly` | seam returns a drift error | checklist 7 | `Verify err = <nil>` — the seam is not consulted |
| `TestTQ8PropSliceInvariants` | 150 seeded-random `RevisionChange`s (1..12 files, kinds/binary/over-half-cap sizes mixed) through `RenderChangeSlice` | R9 | `iter 0: JudgeSaw.Files 0, want 5` — the renderer is a stub |

**`internal/stage/tq8_changesource_test.go`** (package `stage_test`; REAL world: `project.New` + `OnboardStart/OnboardApprove` from `gitFixture`, `EnsureWorkspace`, `project.Store.Snapshot` pins, `review.MintRevision` + `CreateRevisionRef`, `rev.Tree = tq8Seam{proj}` doing what `projectSeams` does — the SIT-1 api pattern; `stage.New` with a no-engine adapter and an injected `Judge`)

| Test | Assertions | Red reason observed |
|---|---|---|
| `TestTQ8StageSinkServesTheChangeFromThePlatformStore` | checklist 9 | `the stage review sink does not implement verify.ChangeSource` |
| `TestTQ8NewVerifierWiresTheChangeSeam` | checklist 10 | `newVerifier composes no verify.ChangeSource although the review store is wired` |

**`internal/stage/tq8_prompt_internal_test.go`** (package `stage`): `TestTQ8ComplianceInstructionsNameTheQuotableItemsAndTheReport` — checklist 11; red: `axis-1 instructions do not name "verify/artifact"` (and the other two ids).

**Test-only hooks** (`internal/stage/export_test.go`): `ChangeSourceOf(*Skeleton) (verify.ChangeSource, bool)` — the sink under type assertion; `VerifierChangeSeamWired(ctx, *Skeleton, domain, taskID) (bool, error)` — drives the real `newVerifier`.

**Executor leave-alone (beyond §4):** `verify/v2_test.go` assertions (only the :40 call moves), `verify/review_seam_test.go`'s `fakeSink` (the `ReviewSink` interface is unchanged — the new seam is a separate interface on purpose), every `internal/stage` e2e harness (they are non-project: `Snapshot` seam nil → the mint pins nothing → `AbsentReason` → content shape, byte-identical), `rider1_golden_test.go`.
