# P3-TQ-8 — post-cap fix (N6–N9)

Finalizer, fresh context, 2026-10-02, worktree branch `worktree-agent-a0253ce364edb86af` on top of `6c8e52a` (re-check r2). Inputs: the brief, `## Re-check r2` of `P3/reports/P3-TQ-8-evaluate.md`, `internal/stage/review_sink.go`, `internal/verify/judgeslice.go`, the r1/r2 TQ8 tests. Scope: N6, N7, N8, N9 only. N10 (report count nit) and the N3/N2-vs-R9 tension are the coordinator's and were not touched.

## Findings

| N | Status | How |
|---|---|---|
| N6 [med] | FIXED | Reproduced first at the real seam (PR5 shape: rev 3 adds `a/big.go` 2,410 × 68 B lines in two regions split by 10 unchanged lines; rev 4 rewrites both regions and adds `b.txt` 100 KiB). Pre-fix: `a/big.go DiffTruncated=true diff=110726 bytes BodySkipped=false` (the exact 110,726 B charge the re-check measured). Fix: `review_sink.go` new helper `tooLargeToCompare(row, cmp)`, called in pass 1 (diffs up to the cut) AND pass 3 (diffs past the cut) right after `CompareFile`: a `cmp.Truncated` comparison sets `DiffTruncated` + `DiffReason`, leaves `Diff == ""`, charges nothing and never sets `cutAt`/`BodySkipped`; reading continues. The renderer's existing `!BodySkipped && (Diff=="" ‖ DiffTruncated)` arm names it "too large to be compared in one piece" without cutting. Post-fix wire: `b.txt` shown (`DiffsShown=1`, `DiffsOmitted=[a/big.go]`), `a/big.go` named too large, its ~164 KB content "did not fit" against the ~92 KB truly left — the only "did not fit" on the wire. |
| N7 [low] | FIXED (pin added) | PR8 shape at the real seam: `src/app.go` 25 KiB → 10 KiB behind a 160 KiB add `a.txt`; asserts the app.go diff is the cut (`BodySkipped`) and its own 10 KiB content is served and on the wire (`ContentShown=1`). M14 (content pass skips `BodySkipped` rows) now FAILS this test; no code change needed. |
| N8 [low] | FIXED | Reproduced first (PR6: rev 3 adds `huge.txt` 300 KiB + `z.txt` 4 B). Pre-fix: `huge.txt DiffTruncated=false diff=0 BodySkipped=true` (a bound cut). Same fix as N6: review-truncated rows are excluded from the cut logic. Post-fix: `huge.txt` named too large, `z.txt` diff shown, no "did not fit" / "already cut" on the wire. |
| N9 [nit] | FIXED | `judgeslice.go` bound paragraph: "…was left out whole, and so was everything after it." → "…was left out whole, and so were the changes of every file after it." Note: the pre-existing `TestTQ8TheDiffBoundSentenceOnlyWhenTheBoundCutADiff` (`tq8_slicehonesty_test.go:343`) asserted the absence of the OLD phrase and is now vacuous on that line; it was NOT edited (immutability). Its F1/F3 property is re-pinned in the new test with the stem "was left out whole". |

Doc comments updated: the sink's body-order comment in `RevisionChange`, the new helper's doc, and `verify.ChangeSource` ("A diff the store could compare only in part is never served and costs nothing: its row carries DiffTruncated with no Diff, and it is not a cut.").

## New regression tests (all new files; no existing test touched)

- `internal/stage/tq8_sinktoolarge_test.go`: `TestTQ8AReviewTruncatedDiffIsNeverChargedNorACut` (N6, PR5), `TestTQ8AnOverCapAddIsTooLargeNotACut` (N8, PR6), `TestTQ8TheCutRowsOwnContentIsStillShown` (N7, PR8).
- `internal/verify/tq8_boundsentence_test.go`: `TestTQ8TheBoundSentenceScopesTheCutToTheDiffs` (N9 + F1/F3 under the new wording).

Coordinator command:

    go test -p 1 -count=1 -run 'TestTQ8AReviewTruncatedDiffIsNeverChargedNorACut|TestTQ8AnOverCapAddIsTooLargeNotACut|TestTQ8TheCutRowsOwnContentIsStillShown|TestTQ8TheBoundSentenceScopesTheCutToTheDiffs' ./internal/...

## Red-before / mutations (applied to the worktree with a byte-copy backup, restored and `cmp`-verified)

| Mutation | Result |
|---|---|
| no fix (code at `6c8e52a`) | N6 test FAIL, N8 test FAIL (the reproductions above) |
| M14: content pass `if row.Binary \|\| row.BodySkipped \|\| …` | `TestTQ8TheCutRowsOwnContentIsStillShown` FAIL |
| fix off: `tooLargeToCompare` `if !cmp.Truncated` → `if true` | N6 + N8 tests FAIL |
| N9 old sentence restored | `TestTQ8TheBoundSentenceScopesTheCutToTheDiffs` FAIL |

## Battery

`gofmt -l internal/ cmd/` empty; `go vet ./...` exit 0; `go run ./tools/lockgate` OK (40 entries). Packages: stage ok 14.8s, verify ok 4.4s, review ok 2.1s. Full `go test -p 1 -count=1 -skip 'TestLivePhraseAndSummarize|TestLiveIntakeTriageClassifiesWebshop' ./...` in the foreground: exit 0, **48 ok, 5 no test files, 0 FAIL**. Orphan sweep: none.

## Draft STATE landing line

P3-TQ-8 post-cap (<commit>): N6 FIXED (a review-truncated diff is never served nor charged nor a cut, in both sink diff passes; named "too large to be compared"), N8 FIXED (same rule), N7 pinned (cut row's own content; M14 now caught), N9 FIXED (bound sentence scoped to the diffs). PR5/PR6/PR8 reproduced at the real seam and committed as 3 stage tests + 1 verify test; 4 mutations caught. Full ./... 48 ok/5 no tests/0 fail. Coordinator verifies with the -run command in the report.

## §81 wording to change

Bound bullet: after "A budget-skipped row (`BodySkipped`) counts as omitted." add "A diff review could compare only in part (`DiffTruncated`) is never served nor charged, is named 'too large to be compared', and is not a cut: the diffs after it are still shown."
