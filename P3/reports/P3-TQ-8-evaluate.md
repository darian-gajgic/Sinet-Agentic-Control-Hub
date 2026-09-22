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
