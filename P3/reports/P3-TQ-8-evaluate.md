# P3-TQ-8 evaluation — the judge's input slice for repo-backed deliverables (S07.5)

Evaluated 2026-09-22 on worktree branch `worktree-agent-a0253ce364edb86af` at `86946c7`, range `b0a45b4..86946c7` (c054cc8 grounding, 387de17 implementation, 86946c7 one-token grounding-test fix). No executor report exists (pre-R5); the executor's chat summary was not trusted. Brief `P3/briefs/P3-TQ-8.md` and the cited spec/CONVENTIONS sections (S07.3 rule 1, S07.5, S07.9 P-T06-3, S07.10, S07.11, S05.3/S05.4, S13.1/S13.2; §15, §22, §59, §73, §74, §77, §78) were read in full. Fresh relaunch; the previous evaluation left nothing.

## VERDICT: FAIL

Nothing found breaks the seam, the wire or the record on the paths the acceptance suite drives, and every battery is green. The verdict is FAIL because two findings sit above nit: F1 is an executed honesty defect on the judge's wire and in `JudgeSaw` for a reachable input (a text row whose diff review serves EMPTY), and F2 is a load-bearing R2 rule (the seam's `bodyBudget`) that no test in the range can trip. The coordinator triages the rest (six low, four nit).

## 1. Checklist (brief §6) against the diff

| # | Item | Result | Evidence |
|---|---|---|---|
| 1 | Repo-backed round: inventory rows, diffs, modified content, report as claims once, binary bytes absent | met | `TestTQ8RepoBackedJudgeSeesTheChangeNotTheReport` green; `judgeslice.go` renderer + `v2.go:184-189` items |
| 2 | Empty tree change at rev 2 stays tree shape | met | same test, sub-case; `renderChangeArtifact` empty arm |
| 3 | Seam once per judged round with `bodyBudget == JudgeArtifactBytesCap`; 1 compliance + 1 sanity | met | `pipeline.go:528-541`; test asserts calls/budget and 1/1 |
| 4 | PASS quoting the report only is forced Unknown; PASS quoting the diff stands | met, load-bearing | `pipeline.go:562 input.Quotable()`; mutation M1 fails the test |
| 5 | Over-bound wide tree: 3 whole diffs, a04.. omitted, content from remainder, JudgeSaw + `judge_saw` row | met on the fake seam | `TestTQ8SliceIsBoundedAtAFileBoundaryAndSaysSo`; see F3 for the real-seam divergence on the content clause |
| 6 | Content-pinned byte-identical with/without the seam; JudgeSaw content + AbsentReason | met | `TestTQ8ContentPinnedSliceIsByteIdenticalAndRecorded`; `contentSlice` |
| 7 | Seam error: wrapped (`errors.Is`), zero judge calls, zero verdict rows | met | `pipeline.go:531-533` `%w`; row assertion pinned by `tq8_record_test.go:78` (the grounding's own row count at `:450` is vacuous, F5) |
| 8 | R9 invariants over 150 random changes | met with blind spots | `TestTQ8PropSliceInvariants`; F4 (empty content never generated as a checked case), F1 (empty diff never generated) |
| 9 | Real composition over a git project store | met, budget unpinned | `TestTQ8StageSinkServesTheChangeFromThePlatformStore`; F2 |
| 10 | `newVerifier` composes `Change` whenever the review store is wired | met, load-bearing | `skeleton.go:1117-1121,1142`; mutation M3 fails the test |
| 11 | `axis1Schema` names the three item ids | met | `engines.go:759-763`; `TestTQ8ComplianceInstructionsNameTheQuotableItemsAndTheReport` |
| 12 | Batteries | green | §3 below |

## 2. Hunt (prompt duty 2)

- Spec contradictions: none found. S07.5's slice (artifact + diff, never the transcript), S07.9 P-T06-3 (claims/diffs not presentation), S07.11 recording, S13.1 (rev 1 vs base, N vs N-1) are implemented as written. `Clean: true` assembly untouched; no transcript field.
- Behavior outside the named seams: none. The only tree read is `verify.ChangeSource` on `stage.reviewSink` over `review.Store.Change/CompareFile/RevisionFile`; `RepoFacts`/`VerificationWorkspace` untouched; `d.SnapshotSHA` is not consulted (PROBE-4 executed). `internal/review`, `internal/project`, `internal/shell`, migrations, settings, `web/` untouched (diff stat).
- Stay-out zones: `pipeline.go` hunks at `:97` and `:516-566` only (not `:287`, `:478`, not `validateFindings`); `engines.go` only the `axis1Schema` hunk.
- ⚙ as constants: `JudgeArtifactBytesCap = 192 << 10` is the brief-sanctioned structural constant, doc comment carries the derivation (window x stage_fit_target x bytes/token) and the settings-tab note. No other new number.
- Dependencies: none new; `go run ./tools/lockgate` OK. Adopted code: untouched (git is the diff authority, unchanged).
- Import wall: `judgeslice.go` imports `context`, `fmt`, `strings` only; verify still never imports review.
- Test edits (duty 4b, every test file in range diffed): `v2_test.go:40` (sanctioned by name), `tq8_judgeslice_test.go:372` `"verify.round"` -> `verify.EventRound` (sanctioned 2026-09-21), `export_test.go` hooks (grounding commit). The executor also ADDED `internal/verify/tq8_record_test.go` (two tests on `verify.EventRound`), a file outside the brief's §4 list; an addition, not a modification of a brief-specified or pre-existing test. No other test was touched.

## 3. Batteries (all run in the worktree, serial)

`go build ./...` exit 0; `gofmt -l internal/ cmd/` empty; `go vet ./...` exit 0; `go run ./tools/lockgate` exit 0.
`go test -p 1 -count=1 ./internal/verify/` ok (4.4s); `./internal/stage/` ok (13.0s, no `-skip`; the two GPU-live tests self-skip with `~/.sinet-b45` absent); `./internal/review/` ok (2.1s).
Full run `go test -p 1 -count=1 ./...` in the foreground: exit 0, 48 packages ok, 5 with no test files, none failed. Logs: `/tmp/tq8-eval/{baseline,mutate,full}.log`.

## 4. Probe log (amendment C)

Scratch files `internal/verify/zz_probe_eval_test.go`, `internal/stage/zz_probe_eval_test.go` were written, run, and deleted; `git status` clean afterwards. Mutations were applied with `sed`/`perl`, run, and restored with `git checkout --` (each restoration verified with `git diff --quiet`).

Held-out probes on the unmodified tree:

- PROBE-1 (composition, renderer): a `modified` non-binary row with `Diff:""`, `DiffTruncated:true`, `DiffReason:"big.go is 2.0 MB, which is too large to compare here — open the file to read it"` (exactly what `review/tree.go:384` serves for a two-sided over-cap file) beside a normal added row. Observed: `DiffsShown:1 DiffsOmitted:[] Truncated:false`; artifact line "The file-by-file changes are in the verify/diff item: all 1 file with text are there in full"; inventory line "its changes are shown only in part: ... open the file to read it". Single-row variant: "No file in this change has text to compare, so the verify/diff item is empty." with `Truncated:false`. FAILS the R9/R3 expectation. -> F1.
- PROBE-2 (boundary): one added row with a diff of exactly `JudgeArtifactBytesCap` bytes -> shown whole, `DiffBytes == cap`; cap+1 -> omitted whole, `DiffsOmitted:[a.txt]`. PASSES. Captured wording when nothing fits: "the first 0 of 1 file with text are there in full". -> F9.
- PROBE-3 (boundary): a `modified` row with `Content:""` (file emptied in place) -> `ContentShown:0 ContentOmitted:[]`. FAILS R9's content clause. -> F4.
- PROBE-4 (real composition, stage): `w.deliverable(2)` with `SnapshotSHA:"stale-copied-pin"`, `BaseSHA:"stale-base"` -> `rc.OldPin/NewPin` equal the store's `pins[0]/pins[1]`. PASSES (R2: the pins come from the store's own rows).
- PROBE-5 (real composition, stage): revision 3 with `README.md` re-added, `a.txt`/`b.txt` 100 KiB added, `src/app.go` modified (37 B). Observed from the real sink: `a.txt diff=104176B`, `b.txt diff=104176B`, `src/app.go skipped=true diff=0B content=0B`; renderer: `DiffsShown:2 DiffsOmitted:[b.txt src/app.go] ContentShown:0 ContentOmitted:[src/app.go] DiffBytes:104295`, 92,313 B of the 196,608 B bound unshown. -> F3.

Mutations (expected outcome in brackets):

| M | Edit | Targeted run | Result |
|---|---|---|---|
| M1 | `pipeline.go:562` quotable text += report | `TestTQ8QuoteFromTheReportDoesNotSatisfyAPass` | FAIL (caught) |
| M2 | `record.go` drop `JudgeSaw: rec.JudgeSaw` | `TestTQ8*` verify | FAIL x2 (caught: `tq8_judgeslice_test.go:384`, `tq8_record_test.go:47`) |
| M3 | `skeleton.go` `sink, change = rs, nil` | `TestTQ8NewVerifierWiresTheChangeSeam` | FAIL (caught) |
| M4 | `review_sink.go:337,360` `if read >= bodyBudget` -> `if read < 0` | `TestTQ8*` stage | PASS (uncaught) -> F2; PROBE-5 flips to PASS under it |
| M5 | `judgeslice.go:253` drop `cut \|\|` (content contiguity) | `TestTQ8*` verify | PASS (uncaught) -> F8 |
| M6 | `v2.go:184-189` report item placed before the diff item | `TestTQ8*\|TestJudgeInputSliceCleanContext` | PASS (uncaught) -> F6 |
| M7 | `judgeslice.go:231` `>` -> `>=` | `TestTQ8*` verify; then PROBE-2 | PASS (uncaught); PROBE-2 FAILS (trips it) -> F9 |
| M8 | `v2.go:258` `executorClaims` returns the raw report | `TestTQ8*` verify | PASS (uncaught) -> F7 |

## 5. Findings

**F1 [med / high] `internal/verify/judgeslice.go:221-228, 262, 286-292, 346`** — A non-binary row whose served diff is EMPTY is neither shown nor named omitted, and the wire lies about it. Reachable inputs: review's two-sided over-cap arm (`review/tree.go:384` serves `Unified=""`, `Truncated=true`, a reason) and a mode-only change (`gitDiff` returns "" for identical text). The renderer skips such a row at `:221-228` unless `BodySkipped`; `diffable` (`:286`) excludes it, so the artifact says "No file in this change has text to compare" (`:290`) when it is the only text row or "all N files with text are there in full" (`:292`) beside others; `saw.Truncated` stays false (`:262`), so the S07.11 record says nothing was cut; and the inventory line says "its changes are shown only in part" (`:346`) although zero bytes of it are shown. Contradicts R3 ("never silent"), R9 ("every diffable row is either shown whole or named as omitted"), §78 ("honest about the bound"). Evidence: PROBE-1 output above. Fix shape: treat `Diff == "" && (DiffTruncated || Kind != deleted...)` as omitted with the review reason, or count text rows from the inventory rather than from what was shown.

**F2 [med / high] `internal/stage/review_sink.go:337-338, 360-361`** — The seam's `bodyBudget` rule (R2: "body reads stop once bodyBudget bytes are read ... rows past it carry BodySkipped") is pinned by no test: mutation M4 disables both budget checks and the entire stage TQ8 battery stays green (checklist 9 asserts only `!BodySkipped` on a small change). The verify-side tests use a fake seam that ignores the budget. A suite no probe can trip on a load-bearing rule is itself a finding (amendment C).

**F3 [low / high] `internal/stage/review_sink.go:341-346, 360-369` with `internal/verify/judgeslice.go:231, 247-251`** — READ budget vs SHOWN budget: the sink's cumulative `read` counts a diff body the renderer later omits (the first diff over the bound), so every later row is `BodySkipped` even when the renderer has room. PROBE-5 on the real store: a 37-byte modified `src/app.go` after two 104 KB added files is `BodySkipped` (no diff, no content) with 92,313 B of the 196,608 B bound unshown. Brief-conformant to R2/R3's letter, but checklist 5's "the small modified file's content still shown from the remainder" holds only with the fake seam; the same wide tree through the real seam yields `ContentShown 0 / ContentOmitted [m01.go]`. Honest on the wire (named as omitted), so low; the judge simply sees less than the bound allows. Candidate refinement for the coordinator: the sink should not count a diff it cannot fit (`len(cmp.Unified) > bodyBudget-read`), or read modified-file contents before oversized diffs.

**F4 [low / high] `internal/verify/judgeslice.go:247-251`** — A `modified`/`renamed` row whose new-side content is empty (a file emptied in place; also `RevisionFile` answering `Binary=true` for a row numstat called text) is in neither `ContentShown` nor `ContentOmitted` and no line names it; R9's content clause fails. PROBE-3 executed. The property test guards `if row.Content != ""` (`tq8_judgeslice_test.go:543`) and its generator can produce empty content (`rng.Intn(20<<10)` at `:492`), so the case is generated and never checked.

**F5 [low / high] `internal/verify/tq8_judgeslice_test.go:450`** — `TestTQ8ALostPinFailsTheRoundLoudly` still counts rows named `"verify.round"`, which nothing writes (`record.go:36 EventRound = "verdict.recorded"`); the assertion is vacuous (always 0). The sanctioned fix in 86946c7 corrected `:372` only. The requirement itself IS pinned by the executor's new `tq8_record_test.go:78` (`TestTQ8ALostPinWritesNoVerdictRow`), so this is a leftover, not a gap; the same one-token sanction should cover `:450`. For the record: `tq8_record_test.go` is a new test file outside the brief's §4 list (an addition, not a modification).

**F6 [low / high] `internal/verify/v2.go:184-189`** — R5's manifest order ("`verify/executor-report` placed after `verify/diff` and before `verify/rubric`") is unpinned: mutation M6 moves the report item before the diff item and `TestTQ8*` plus `TestJudgeInputSliceCleanContext` stay green.

**F7 [low / high] `internal/verify/v2.go:258-262`** — R1's "labelled claims" item is unpinned: mutation M8 makes `executorClaims` return the raw report (no label) and the battery stays green. The acceptance test checks the item id and the once-on-the-wire count, never the label.

**F8 [low / high] `internal/verify/judgeslice.go:253`** — R3's "same rule" (a contiguous path-order prefix) for the CONTENT section is unpinned: mutation M5 drops `cut ||` and the battery stays green; R9 states only shown-or-omitted for content, so the property test cannot see non-contiguity.

**F9 [nit / high] `internal/verify/judgeslice.go:231, 295`** — The bound's inclusivity at exactly the cap is unpinned (M7 `>` -> `>=` green; PROBE-2 trips it). When nothing fits the artifact reads "the first 0 of 1 file with text are there in full" (`:295`); review's `partialChangeReason` has a `shown == 0` arm, the renderer does not.

**F10 [nit / med] `internal/verify/judgeslice.go:346, 349`; `internal/review/tree.go:384, 391`** — Review's per-file truncation reasons are review-page sentences addressed to a person ("open the file to read it", "the text below covers only the start of it") and are forwarded verbatim to a judge that can open nothing; "its content is shown only in part" for a 1 MiB-cut content is also never reachable under the 192 KiB bound (such content is always omitted), so `:349` is dead in practice.

**F11 [nit / high] `internal/verify/pipeline.go:528-541`** — The seam is asked after V1 has run and recorded its row; a lost pin fails the round after the ladder ran. R2 requires only "before any judge call" (met, $0). Asking right after the mint (`:424` region) would fail faster; ledger item.

**F12 [nit / high] `internal/stage/review_sink.go:341`** — Each `CompareFile` recomputes the whole inventory (`treeCompare` -> `Change` -> `TreeChanges`: `diff-tree --raw`, `--numstat`, `ls-tree`) on top of the blob reads: N+1 inventory computations per judged round, $0 and budget-bounded. The brief's §9 already names the N+1 reads.

## 6. Draft STATE landing line (rule R6)

P3-TQ-8 evaluate (86946c7): VERDICT FAIL, 12 findings (2 med, 6 low, 4 nit); seam, wire and record intact on the driven paths. Batteries green: build/gofmt/vet/lockgate, verify/stage/review, full ./... (48 ok). 5 held-out probes + 8 mutations: M1-M3 caught; M4 (sink bodyBudget), M5 (content contiguity), M6 (item order), M7 (cap boundary), M8 (claims label) uncaught. Med: F1 an empty-served-diff text row vanishes with "all shown"/"no text" wire statements and Truncated=false; F2 the seam's budget is unpinned. Test edits: 2 sanctioned + new tq8_record_test.go; :450 legacy event name vacuous.

## Re-check r1

Re-checked 2026-10-02 on worktree branch `worktree-agent-a0253ce364edb86af` at `83e071b` (drain r1, range `14ec042..83e071b`). Inputs read in full: the brief, the first evaluation above, the finalizer report `P3/reports/P3-TQ-8-finalize-r1.md` (not trusted; every claim below was re-derived), the drain diff, the two new test files, `review/tree.go` (fileDiff, RevisionFile, caps), `review/diff.go` (gitDiff), `v2.go`, `record.go`. Fresh context; no finalizer state reused.

### VERDICT: FAIL

F1–F10 are RESOLVED in the code (F3 per the coordinator's sanctioned R2 refinement). The verdict is FAIL because one new finding sits above nit: N1, a residual of F3's mechanism that the refinement left in place — after the first diff the sink cannot fit, it keeps reading and CHARGING diffs the renderer then omits for R3 contiguity, so a later modified file's content is `ContentSkipped` while the renderer has 92 KB of room, and the wire says "it did not fit the 192 KB" (false). Executed at the real seam (PR1 below). Four nits and one low follow.

### Per-finding verification

| F | Status | How verified (independently) |
|---|---|---|
| F1 | RESOLVED | Real seam PR4: a mode-only change (`chmod` on `src/cart.go`, inventory `modified 28→28`, `gitDiff` "") is named in `DiffsOmitted` with "both versions hold the same text — what changed is the file's mode or its path", `Truncated=true`, content shown; renderer P-A: review's two-sided over-cap row (`DiffTruncated`, `Diff ""`) is named with "too large to be compared in one piece", the artifact says "The verify/diff item is EMPTY: not one of the 1 file with text is shown" / "1 of 2 files", never "all N in full", never "shown only in part", and review's "open the file to read it" is absent. Mutation M11 (F1 arm disabled at `judgeslice.go:248`) fails `TestTQ8ATextRowWithNoServedDiffIsNamedOmitted`. |
| F2 | RESOLVED | M4-all (all four sink budget checks disabled) fails `TestTQ8TheSinkStopsAtTheBodyBudgetAndKeepsReadingPastWhatCannotFit` ("read 208662 body bytes for a 196608-byte budget") and `TestTQ8AContentThatCannotFitNeverCostsItsFileTheDiff`; M4a (diff fit check only) and M4b (content fit check only) each fail one of them. Both tests drive the real git-backed sink. |
| F3 | RESOLVED (as sanctioned) | `review_sink.go:352-355, 379-384`: a body larger than `bodyBudget-read` is not served and costs nothing; later rows are still read. The PROBE-5 shape is the new test (`src/app.go` content shown after a skipped `b.txt`). Residual re-filed as N1. |
| F4 | RESOLVED | Real seam PR3: `src/app.go` emptied in place (`modified 47→0`, `TreeBlob` serves the empty blob, no ErrNotFound fall-through) → `ContentShown=1`, the content section prints the "(no text: …)" line; renderer P-B same. M12 (pre-fix skip re-inserted) fails `TestTQ8AFileEmptiedInPlaceIsNamedOnTheWire`. |
| F5 | RESOLVED | Diff `14ec042..83e071b` on `tq8_judgeslice_test.go` is exactly the `:450` token `"verify.round"` → `verify.EventRound`; `record.go:36 EventRound = "verdict.recorded"`, `RecordRound` appends it (`:203`), so the zero-rows assertion is live. |
| F6 | RESOLVED | M6 (report item before the diff item in `v2.go`) fails `TestTQ8TheReportItemIsLabelledClaimsAndSitsAfterTheDiff` ("manifest order artifact=1 diff=3 report=2 rubric=4"). |
| F7 | RESOLVED | M8 (`executorClaims` returns the raw report) fails the same test ("the report rides UNLABELLED"). |
| F8 | RESOLVED | M5 (`cut ||` dropped at `judgeslice.go:296`) fails `TestTQ8ContentSectionIsAContiguousPathOrderPrefix` ("shown 2 omitted [b.go], want 1 shown and b.go,c.go omitted"). |
| F9 | RESOLVED | M7 (`>` → `>=` at `:258`) fails `TestTQ8TheBoundIsInclusiveAtTheCap`; `grep 'the first'` finds no wire wording left (only the stale comment, N4). |
| F10 | RESOLVED | `grep DiffReason\|ContentReason internal/verify/*.go` hits only the struct fields (`:68-75, :82`); P-A shows no review-page sentence on the wire; the `:349` dead line is gone. |

### Test-file tamper check (duty 4b)

`git diff --name-status 14ec042..83e071b`: `M internal/verify/tq8_judgeslice_test.go` (1 insertion, 1 deletion — the sanctioned `:450` token, verified above), `A internal/stage/tq8_sinkbudget_test.go`, `A internal/verify/tq8_slicehonesty_test.go` (new files), plus `review_sink.go`, `judgeslice.go`, the finalizer report. No other test file touched; no brief-specified or pre-existing assertion weakened; no skips added. The untouched R9 property test (`TestTQ8PropSliceInvariants`) still passes under the new semantics; its generator still never produces an empty diff, an empty content on a checked row, or `ContentSkipped` (its blind spots are now covered by the two new files, not by it).

### The finalizer's two self-reported fixes to the dead finalizer's WIP

1. `ContentSkipped` flag: `review_sink.go:372, 382` set only in the content loop; `:365` skips `BodySkipped` rows only; renderer `:253` tests `cut || row.BodySkipped` (not `ContentSkipped`) in the diff section and `:283` treats either flag as content-omitted. Correct. Pinned at the real seam (M9: sink sets `BodySkipped` instead → `TestTQ8AContentThatCannotFitNeverCostsItsFileTheDiff` fails "src/big.go's one-line diff fits and must be served: BodySkipped=true diff=328 bytes") and in the renderer (M13: diff section also tests `ContentSkipped` → `TestTQ8ASkippedContentLeavesTheServedDiffShown` fails).
2. `bound` flag set only by diff-section cuts (`:255, :259`): M10 (content cut also sets `bound`, `:285`) fails `TestTQ8TheDiffBoundSentenceOnlyWhenTheBoundCutADiff`. Correct and covered.

### Probe log

Scratch files `internal/verify/zz_probe_recheck_test.go`, `internal/stage/zz_probe_recheck_test.go` were written, run, and deleted (`git status` clean afterwards). Renderer probes (fake rows):

- P-A (F1): over-cap two-sided row alone and beside an added file — see F1 row above. PASS.
- P-B (F4): `modified 120→0`, `Content ""` → `ContentShown=1`, "(no text …)" line. PASS.
- P-C: added EMPTY file and deleted EMPTY file (`Diff ""`, size 0) beside an added file → both in `DiffsOmitted` with "both versions hold the same text — what changed is the file's mode or its path"; `Truncated=true`. FAILS honesty → N2 (confirmed at the real seam, PR2).
- P-D: the sink's output shape after the refinement (a.txt 104 KiB served, b.txt `BodySkipped`, c.txt 73 KiB served, m.go small diff + `ContentSkipped`) → `DiffsShown=1 DiffBytes=106496`, room 90,112 B, m.go: "its whole content is NOT shown: it did not fit the 192 KB of file text this judge reads under". FAILS → N1 (real seam, PR1).
- P-E: a one-sided over-cap diff as review serves it (256 KiB, `DiffTruncated`) → omitted at the bound; `partialDiffNote` never printed → N4.
- P-F: pure rename (`Diff ""`, content served) → named omitted with the mode/path wording (correct), `Truncated=true`, content shown → N3.
- P-G: a `BodySkipped` modified row → two notes, "its changes … did not fit" and "its whole content … did not fit" (the content was never measured) → folded into N5.

Real-seam probes (`newTQ8World`, git-backed project store, `stage.ChangeSourceOf`):

- PR1 (N1): rev 3 `src/app.go` = 25,612 B; rev 4 adds `README.md`, `a.txt` 100 KiB, `b.txt` 100 KiB, `c.txt` 70 KiB and appends one line to `src/app.go`. Sink rows: `README.md diff=119`, `a.txt diff=104155`, `b.txt BodySkipped`, `c.txt diff=72947` (served and charged), `src/app.go diff=486 content=0 ContentSkipped=T` (25,617 B > the sink's remaining 18,901 B). Renderer: `DiffsShown=2 DiffsOmitted=[b.txt c.txt src/app.go] DiffBytes=104274 ContentShown=0 ContentOmitted=[src/app.go]`, renderer room 92,334 B. Wire for `src/app.go`: "its changes are NOT shown: the slice was already cut at an earlier file in this list …" + "its whole content is NOT shown: it did not fit the 192 KB of file text this judge reads under". The judge receives no body at all for the only in-place change.
- PR2 (N2): rev 3 adds `empty.txt` (""): inventory `added bin=F new=0 diff=0 dtr=F` → `DiffsOmitted=[empty.txt] Truncated=true`, note "both versions hold the same text — what changed is the file's mode or its path", paragraph "The verify/diff item is EMPTY: not one of the 1 file with text is shown."
- PR3 (F4): rev 4 empties `src/app.go` → `modified old=47 new=0 diff=170 content=0 ctr=F` → `ContentShown=1`, "(no text …)" line. PASS.
- PR4 (F1): rev 5 `chmod 700 src/cart.go` → `modified old=28 new=28 diff=0 content=28` → `DiffsOmitted=[src/cart.go]`, mode/path note, content shown. PASS.

### Mutations (all applied in a scratch copy of the tree, each restored and verified byte-identical; targeted run `go test -p 1 -count=1 -run TestTQ8 <pkg>`)

| M | Edit | Result |
|---|---|---|
| M4-all | `review_sink.go`: both `read >= bodyBudget` → `read < 0`, both fit checks → `false` | FAIL (caught) ×2 |
| M4a | diff fit check → `false` | FAIL (caught) |
| M4b | content fit check → `false` | FAIL (caught) |
| M9 | content fit check sets `BodySkipped` | FAIL (caught) |
| M5 | `:296` drop `cut \|\|` | FAIL (caught) ×2 |
| M6 | `v2.go` report item before diff item | FAIL (caught) |
| M7 | `:258` `>` → `>=` | FAIL (caught) |
| M8 | `executorClaims` returns raw report | FAIL (caught) |
| M10 | `:285` content cut sets `bound` | FAIL (caught) |
| M11 | `:248` F1 arm disabled | FAIL (caught) ×2 |
| M12 | pre-fix empty-content skip re-inserted after `:281` | FAIL (caught) |
| M13 | `:253` diff section also cuts on `ContentSkipped` | FAIL (caught) ×2 |

12/12 caught; M4–M8 all confirmed by me, not only by the finalizer.

### Batteries (worktree, serial)

`go build ./...` exit 0; `gofmt -l internal/ cmd/` empty; `go vet ./...` exit 0; `go run ./tools/lockgate` OK (40 entries). Package runs: verify ok (4.3s), stage ok (13.3s), review ok (2.0s). Full `go test -p 1 -count=1 -skip 'TestLivePhraseAndSummarize|TestLiveIntakeTriageClassifiesWebshop' ./...` in the foreground: exit 0, **48 ok, 5 no test files, 0 FAIL**. Orphan sweep after: none. Logs in the session scratchpad (`tq8r1/{baseline,full,unmutated-stage-tq8,mut-*}.log`).

### New findings

**N1 [med / high] `internal/stage/review_sink.go:339-358` with `internal/verify/judgeslice.go:253-257, 283-287`** — F3 residual. After the first diff that cannot fit, the sink keeps reading AND CHARGING later diffs (`read += len(cmp.Unified)`) that the renderer then omits for R3 contiguity ("already cut at an earlier file"). The sink's `read` and the renderer's shown bytes diverge, so a later modified file's content is `ContentSkipped` against the sink's remainder (PR1: 25,617 B > 18,901 B) while the renderer's remainder is 92,334 B, and the wire states "it did not fit the 192 KB of file text this judge reads under" — false for the reader, who sees 104 KB of diffs. R3's content clause ("under the REMAINING budget") is not met; the judge gets no body for the one in-place change, exactly the case the refinement's own comment says must not happen ("One 100 KB file in the middle of a change must not cost a 40-byte fix at the end its place on the wire"). Fix shapes (coordinator decision): (a) the sink mirrors R3 — once a diff is skipped, later diffs are marked past-cut without `CompareFile` and are not charged, while contents are still read; or (b) the renderer drops diff contiguity now that the "first N of M" sentence is gone and every row carries its own reason. Either way the sink's `read` and the renderer's `DiffBytes+ContentBytes` should count the same bytes.

**N2 [low / high] `internal/verify/judgeslice.go:248-252, 347-352`** — An added or deleted EMPTY text file (`gitDiff` returns "" for two identical blobs, `review/diff.go:33`; real seam PR2 `added new=0 diff=0`) is named in `DiffsOmitted` with `noDiffSameText` ("both versions hold the same text — what changed is the file's mode or its path"), which is false for a file that was created or removed; the record says `Truncated=true` and the paragraph says the one "file with text" is not shown although the file has no text. Fix: an added/deleted row with size 0 has nothing to show and should say that (and arguably not count as omitted).

**N3 [nit / high] `internal/verify/judgeslice.go:307`; `JudgeSaw` docs `:151-152, :165-166`** — A mode-only change or pure rename (PR4, P-F) sets `Truncated=true` and lands in `DiffsOmitted` (documented as "omitted at the bound") though nothing was cut and the judge saw everything there was; the S07.11 record conflates "no text to show" with "cut". The first report's F1 fix shape asked for exactly this, so it is the coordinator's semantic call; the doc comments at least should say what the lists now hold.

**N4 [nit / high] `internal/verify/judgeslice.go:324 (partialDiffNote), :208-212`** — `partialDiffNote` ("only the START of its changes is shown") is unreachable from the real seam: review cuts a one-sided over-cap diff at `TreeFileDiffBytesCap` 256 KiB > `JudgeArtifactBytesCap` 192 KiB, so such a diff never fits and is always named "did not fit" (P-E). A dead arm, the same class as F10's dead line. The `RenderChangeSlice` doc comment still says "(a contiguous prefix, so 'the first N of M' is literally true)" — wording the drain removed from the wire.

**N5 [nit / med] `internal/stage/review_sink.go:344-347, 371-374`** — The `read >= bodyBudget` pre-checks are redundant now that the fit checks hold the budget, except at exact equality, where a 0-byte diff row (mode-only) is marked `BodySkipped` ("did not fit") instead of being served as the honest empty; the sink also still runs `CompareFile` on diffs the renderer will omit (PR1: `c.txt` 72,947 B read for nothing), $0 and F12-adjacent. P-G: a `BodySkipped` modified row carries both "its changes … did not fit" and "its whole content … did not fit" although its content was never measured.

### Hygiene notes

Three of my Bash calls were terminated by the harness (exit 144) while running in-place mutation commands in the worktree (one was my own `pkill -f` matching its calling shell); each time the tree was restored with `git checkout --` and verified, and the mutation battery was then moved to a scratch copy of the tree so the worktree never carried a mutation at any commit. Aside, out of scope: a user systemd timer `sinet-probe-timer.timer` (every 15 min, appends a date to `~/sinet-probe-timer.log`) is live on the host — a leftover from some earlier probe, not this packet.

### Draft STATE landing line (R6)

P3-TQ-8 re-check r1 (83e071b): VERDICT FAIL. F1–F10 RESOLVED (F3 as sanctioned); tamper clean (:450 token only); finalizer's two self-fixes correct and pinned. 12 mutations (M4-all/M4a/M4b/M5–M13) all caught; 7 renderer + 4 real-seam probes. Full ./... 48 ok / 5 no tests / 0 fail. New: N1 [med] after the first skipped diff the sink keeps charging diffs the renderer omits, so a later modified file's content is ContentSkipped with 92 KB of room and the wire says "did not fit" (F3 residual, R3 content clause); N2 [low] added/deleted empty file named "same text: mode or path changed"; N3–N5 nits.
