# P3-TQ-8 finalize, drain round 1

Finalized 2026-10-02 on worktree branch `worktree-agent-a0253ce364edb86af`, on top of `14ec042` (evaluate report, VERDICT FAIL, F1-F12). Coordinator triage: F1-F10 apply; F11 and F12 DROPPED to the ledger (not touched). Sanctions used: F3 as an R2 refinement (the sink skips only the body that cannot fit and keeps reading); F5 the one-token edit at `tq8_judgeslice_test.go:450`.

## Inherited state

A previous finalizer died mid-drain (host API error, 2026-09-22) and left uncommitted work: `internal/stage/review_sink.go`, `internal/verify/judgeslice.go`, the `:450` edit, and two new test files (`internal/stage/tq8_sinkbudget_test.go`, `internal/verify/tq8_slicehonesty_test.go`). I judged it against the findings and the spec text. It was green and covered every finding, but it had two defects. I fixed both here:

1. **A served diff was dropped for its content (regression, from the F3 change).** The sink's content loop set `BodySkipped` on rows whose diff it had already served. The rewritten renderer tested `cut || row.BodySkipped` before it looked at `Diff`. So a modified file with a small diff and a big content (a one-line change in a big file) lost its served, fitting diff from the wire. Every later diff was cut with it. Fix: a separate `ChangedFile.ContentSkipped` flag, set only by the sink's content loop. `BodySkipped` again means "the diff body (and so every body) was not served". The renderer's content section treats either flag as omitted. Pinned at the real seam (`TestTQ8AContentThatCannotFitNeverCostsItsFileTheDiff`) and in the renderer (`TestTQ8ASkippedContentLeavesTheServedDiffShown`). Under the inherited code both fail (mutation M9).
2. **The diff paragraph's bound sentence could be false.** One `bound` flag was set by both sections. A content-only cut plus a mode-only omitted diff made the diff paragraph say "what did not fit ... was left out whole, and so was everything after it" when no diff had failed to fit. Fix: `bound` is set only by diff-section cuts (the content section already says its own). Pinned by `TestTQ8TheDiffBoundSentenceOnlyWhenTheBoundCutADiff` (the mutation is caught).

## Findings

| F | Disposition | How / evidence |
|---|---|---|
| F1 | FIXED | Renderer: a non-binary row with an empty served diff and no skip is named in `DiffsOmitted` with its own cause ("too large to be compared in one piece" when `DiffTruncated`, else "same text: mode or path changed"). It does not cut later diffs, and `Truncated` follows. The wire no longer says "all N ... in full" or "no file has text" beside it. A new `DiffsShown == 0` arm. Repro on pre-fix source: `TestTQ8ATextRowWithNoServedDiffIsNamedOmitted` fails 3/3 (`DiffsOmitted = []`). |
| F2 | FIXED | `TestTQ8TheSinkStopsAtTheBodyBudgetAndKeepsReadingPastWhatCannotFit` drives the real git-backed sink over the 192 KiB bound and a 1-byte budget. Mutation M4 (both budget checks disabled) is now caught, and so is M4a (only the new fit check disabled). |
| F3 | FIXED (R2 refinement as sanctioned) | Sink: a body larger than what is left of the budget is not served and costs nothing, and later rows are still read. A diff that does not fit sets `BodySkipped`, a content that does not fit sets `ContentSkipped`. PROBE-5's shape now shows `src/app.go`'s content. Repro on pre-fix source: the sink read 208,429 body bytes for a 196,608 budget and skipped app.go. The renderer keeps R3 contiguity: diffs after a skipped diff are still omitted. |
| F4 | FIXED | A modified/renamed row with empty content (no skip, not truncated) is shown as the empty file, with a "(no text ...)" line and `ContentShown` counting it. Truncated-but-empty content is named omitted. Repro on pre-fix source: `TestTQ8AFileEmptiedInPlaceIsNamedOnTheWire` fails (in neither list). |
| F5 | FIXED | `tq8_judgeslice_test.go:450` `"verify.round"` changed to `verify.EventRound` (one token, sanctioned). The pre-fix assertion was vacuous: `EventRound = "verdict.recorded"` (`record.go:36`). |
| F6 | FIXED (pin) | `TestTQ8TheReportItemIsLabelledClaimsAndSitsAfterTheDiff` asserts manifest and wire order artifact < diff < report < rubric. M6 is caught (`manifest order ... report=1`). |
| F7 | FIXED (pin) | Same test asserts the claims label ("not the work itself", names verify/artifact and verify/diff) precedes the verbatim report. M8 is caught ("rides UNLABELLED"). |
| F8 | FIXED (pin) | `TestTQ8ContentSectionIsAContiguousPathOrderPrefix`: a.go fits, b.go is cut, c.go would fit and must stay omitted. M5 is caught. |
| F9 | FIXED | `TestTQ8TheBoundIsInclusiveAtTheCap` pins exactly-cap shown, cap+1 omitted, and content exactly filling the remainder. The "first 0 of" wording was replaced by an explicit `DiffsShown == 0` arm. M7 is caught. Repro on pre-fix source: fails on "the first 0 of". |
| F10 | FIXED | Review's page sentences (`DiffReason`/`ContentReason`) are no longer rendered. The renderer states causes in its own judge-facing sentences, and the dead "content shown only in part" line is gone. The F1 test asserts "open the file to read it" never reaches the slice. |
| F11, F12 | not touched | Dropped by the coordinator (ledger). |

Test immutability: the only edit to a pre-existing test is the sanctioned `:450` token. New tests sit only in the two new files. Mutations were applied in a scratch copy, or applied in place and restored from a byte-verified backup.

## Battery (serial, foreground)

`go build ./...` exit 0; `gofmt -l internal/ cmd/` empty; `go vet ./...` exit 0; `go run ./tools/lockgate` OK. Package runs: verify ok, stage ok, review ok. Full `go test -p 1 -count=1 -skip 'TestLivePhraseAndSummarize|TestLiveIntakeTriageClassifiesWebshop' ./...`: exit 0, 48 ok, 5 no test files, 0 failed. `web/src` untouched.

## Draft STATE landing line (R6)

P3-TQ-8 finalize r1: F1-F10 FIXED (F11/F12 ledger). The renderer names every empty-served text row with its own cause, shows emptied files, says "nothing shown" rather than "first 0", and keeps review's page sentences off the wire. The sink skips only a body that cannot fit and keeps reading. New `ContentSkipped` keeps a served diff when only content is cut. Pins added for item order, claims label, content contiguity and the cap boundary: M4-M9 all caught. The dead finalizer's work was kept, with 2 defects fixed. Full ./... green, 48 ok.

## CONVENTIONS draft text

No change to the draft section text. One addition for the coordinator's §-entry if it wants it: `ChangedFile` carries two skip flags. `BodySkipped` means the diff (and so the content) was not served; `ContentSkipped` means only the content was not served. A served diff is never dropped for its content.
