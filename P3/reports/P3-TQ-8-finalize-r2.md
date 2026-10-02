# P3-TQ-8 — finalize, drain round 2

Inputs: brief `P3/briefs/P3-TQ-8.md` (R2, R3, R9), `P3/reports/P3-TQ-8-evaluate.md` §Re-check r1 (N1–N5), range `b0a45b4..ddc4351`. N5 dropped to the ledger by the coordinator and was not touched as such (see the N1 side effect below).

## N1 [med] — FIXED

**Reproduced first** at the real seam (git-backed `newTQ8World`, the re-check's PR1 shape: rev 3 `src/app.go` 25 KB; rev 4 adds README.md, a.txt 100 KiB, b.txt 100 KiB, c.txt 70 KiB and appends one line to app.go). New test `internal/stage/tq8_sinkorder_test.go: TestTQ8ADiffPastTheCutNeverCostsAShownContentItsPlace`, run with the two source files stashed back to `ddc4351`:
`tq8_sinkorder_test.go:65: src/app.go's 25 KB content fits beside the shown diffs and must be served: ContentSkipped=true content=0 bytes` (FAIL).

**Constraint:** the drain-r1 test `tq8_sinkbudget_test.go:93,103` (immutable) pins that the sink still SERVES a diff past the cut (`src/app.go` after the skipped `b.txt`: `BodySkipped=false`, diff present) and that the renderer omits it. So "never read past-cut diffs" (re-check shape a, literally) would break it; dropping diff contiguity (shape b) breaks R3/R9 and the same test's `:103`.

**Fix** (`internal/stage/review_sink.go` `RevisionChange`): the sink reads bodies in the order the judge is SHOWN them: (1) diffs in path order until the first that does not fit (`BodySkipped`, cut); (2) contents of modified/renamed rows, including rows whose diff is not shown, as a contiguous prefix: after the first content that does not fit, the rest are marked `ContentSkipped` unread; (3) last, the diffs past the cut, from the leftover only (served if they fit, else `BodySkipped`). Every body the judge is shown is now priced against exactly the bytes shown before it, so the sink's fit decision equals the renderer's and "did not fit the 192 KB" is true wherever it appears. Past-cut diffs only consume room nothing shown needs. The renderer (`internal/verify/judgeslice.go`) now omits a content only on `ContentSkipped` (the sink sets it on every modified/renamed text row it does not serve), so a past-cut row whose content was served keeps it. Contract docs on `ChangeSource` / `ChangedFile.BodySkipped/ContentSkipped` updated.

After: PR1 shape gives `DiffsShown=2 (README.md, a.txt) DiffsOmitted=[b.txt c.txt src/app.go] ContentShown=1`; app.go's content is on the wire; "did not fit" appears only for b.txt (+ the bound paragraph), and the test asserts b.txt really exceeds the room left.

Side effect (not an N5 fix, a consequence): the cut row's content is now actually measured, so P-G's "content did not fit, never measured" can no longer occur from the sink.

## N2 [low] — FIXED

Reproduced at the real seam: `TestTQ8AFileAddedEmptyIsSaidToBeEmpty` on `ddc4351` code: `omitted [empty.txt] truncated true`. Fix: `emptyFile(row)` (added with NewSize 0 / deleted with OldSize 0, no diff, not skipped, not truncated) gets its own inventory note ("it has no changes to show: the file was added empty" / "...was empty when it was removed"), is not counted as a file with text, is in neither omitted list, and leaves `Truncated` false. Renderer test `internal/verify/tq8_sliceempty_test.go: TestTQ8AnEmptyFileAddedOrRemovedIsNotACut` (added, removed, only-empty) fails on the old renderer, passes now.

## N3 [nit] — DECLINED (behaviour), docs corrected

Making a mode-only / pure-rename row `Truncated=false` contradicts an immutable test and the brief: `internal/verify/tq8_slicehonesty_test.go:98` asserts `saw.Truncated` for every case of `TestTQ8ATextRowWithNoServedDiffIsNamedOmitted`, including "a change to the file mode alone has no diff text either" (which also pins the row in `DiffsOmitted`), and R9 / `TestTQ8PropSliceInvariants:562` pin `Truncated ⇔ something is named omitted`. Changing it needs a coordinator-sanctioned edit of that test case and of R9's wording. Done instead: the `JudgeSaw.DiffsOmitted` and `Truncated` doc comments now say what they hold (cut at the bound, too large to compare, or no text changed; Truncated ⇔ anything named, mode-only included). No regression test added since behaviour is unchanged (the existing `:98` case is the pin).

## N4 [nit] — FIXED

`partialDiffNote` and its arm removed. A row whose diff the source served only in part (`DiffTruncated`, non-empty diff) is now never shown (R3 "never a diff of a partially read file") and is named with `noDiffTooLarge`; it is not a bound cut, so later files still show. Stale "the first N of M" doc comment on `RenderChangeSlice` rewritten. Test `TestTQ8ADiffServedOnlyInPartIsNeverShown` fails on the old renderer (partial hunk shown, nothing omitted), passes now.

## Test immutability

No existing test file modified. New files: `internal/stage/tq8_sinkorder_test.go`, `internal/verify/tq8_sliceempty_test.go`.

## Battery (worktree, serial, foreground)

`gofmt -l internal/ cmd/` empty; `go vet ./...` ok; `go run ./tools/lockgate` OK (40 entries); package runs verify ok, stage TQ8 ok; full `go test -p 1 -count=1 -skip 'TestLivePhraseAndSummarize|TestLiveIntakeTriageClassifiesWebshop' ./...`: exit 0, **48 ok, 5 no test files, 0 FAIL**. No orphan test processes afterwards.

## Draft STATE landing line

P3-TQ-8 drain r2 (<sha>): N1 FIXED (sink reads bodies in shown order: prefix diffs, then contents incl. rows whose diff is cut, then past-cut diffs from leftover; renderer omits content only on ContentSkipped; PR1 shape now ships app.go's content, "did not fit" only where true); N2 FIXED (empty added/deleted file: own note, not omitted, Truncated false); N3 DECLINED (slicehonesty_test:98 + R9 pin Truncated for mode-only; docs corrected); N4 FIXED (partial diff never shown, dead note gone). 3 new tests, red before/green after. Full ./... 48 ok/5 no tests/0 fail.
