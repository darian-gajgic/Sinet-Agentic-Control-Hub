# CI-1 — execute report (light path, test-only)

**Commit:** `c250d9e` CI-1: benchmark driver tests wait on the pair state, not run idleness (test-only)
**Branch:** `worktree-agent-a3c74d1a853902d3e` (not pushed)
**Files:** `internal/shell/benchmark_driver_test.go`, `internal/shell/benchmark_verbs_test.go`. Production code unchanged.

## Verified mechanism

`stage.dispatchDirect` (internal/stage/direct.go) calls `Driver.Drive`, whose `pump` commits the run's terminal transition (`running→completed` / `→died-at-gate`) and returns; only THEN does it call `BenchmarkCapture` → `Store.CaptureDirectText` (a separate `WriteTx` that sets `benchmark_pairs.direct_text`; pair `State` stays `dispatched`). All of it runs in the scheduler's tracked dispatch goroutine. `driverEnv.waitIdle` returns as soon as no run is `claimed`/`running`, i.e. inside that window, so the next `benchmarkDriverPass` sees "arm ended, no capture" and correctly skips the pair (benchmark_seams.go:456, the failed-pair path) — the test then reads `dispatched` and fails; `t.Cleanup(db.Close)` runs while the capture goroutine is still at `BEGIN IMMEDIATE`, which logs `storage: begin immediate: sql: database is closed` through `slog.Default` (stage gets no Logger in `newDriverEnv`; `testLogger` would have discarded it).

## Evidence

- Unmodified code, `GOMAXPROCS=1 go test -count=20 -run '<the two tests>' ./internal/shell/`: green (not reproducible this way on a 24-core host).
- Scratch reproduction (test file only, reverted before commit): `BenchmarkCapture` wrapped with a 50 ms sleep in `newDriverEnv`. Result: 11 tests red, including both CI messages verbatim (`benchmark_driver_test.go:437: the retry did not complete: dispatched`; `fixture pair is dispatched, want rendered`) and the exact ERROR line. Also red: TestDriverWalksASampledPairToRendered (:352, the capture read), TestTruncatedArmYieldsTheParityNote (:464), every `rendered()` user (TestRecordCommits…, TestGuessLess…, TestVerdictIsTheRequesters…, TestPendingForm…, TestTheServedVocabulary…). TestDriverAdvancesNothingSynthetically and TestDispatchIsSingleShotEvenWhenStateIsFlippedBack passed only vacuously (their captures died against the closed DB).
- Same scratch delay WITH the fix: `go test -count=3 ./internal/shell/` green; `-v -count=3` over all 16 benchmark driver/verbs tests: 0 FAIL, 0 `capture failed`/`database is closed` lines.

## Fix (waiting only)

New `driverEnv.captured(t, pairID)` polls `e.bs.Store.CapturedDirectText(DirectRunID(pairID))` with the 10 s deadline / 2 ms step (`ErrNoDirectCapture` → keep polling; any other error → Fatal). Called after the claim in: `rendered()`; TestDriverWalksASampledPairToRendered (before the capture assertion); TestDriverRetriesAFailedRenderWithoutCorruption (so the failing pass really reaches the render rather than skipping); TestTruncatedArmYieldsTheParityNote (empty capture is `("", nil)`, so the wait holds); TestDriverAdvancesNothingSynthetically (before the fixture NULLs `direct_text`, so a late capture cannot re-land behind it); TestDispatchIsSingleShotEvenWhenStateIsFlippedBack (the first walk is whole before it is tampered with — this one removes the leftover teardown ERROR line, no assertion depends on it). `claim`'s doc corrected ("waits for the claimed run to end", not "for the dispatch to finish"). `waitIdle` unchanged and still the run-level wait. TestCrashedArm… needs no wait: the crash path returns before any capture. No assertion added, removed or weakened.

## Verification (after reverting the scratch delay)

1. `GOMAXPROCS=1 go test -count=20 -run 'TestDeclineIsRecordedAndReported|TestDriverRetriesAFailedRenderWithoutCorruption' ./internal/shell/` → ok (40/40 PASS under `-v`).
2. `go test -count=3 ./internal/shell/` → ok (28.5 s).
3. `go test -p 1 -count=1 -skip 'TestLivePhraseAndSummarize|TestLiveIntakeTriageClassifiesWebshop' ./...` (foreground) → exit 0, 48 packages ok, 5 no-test-files, 0 FAIL.
4. `gofmt -l internal/shell/` empty; `go vet ./internal/shell/` clean. No orphan test processes.

## STOP-clause finding — production, NOT patched (coordinator decision)

The CI failure is test-side (the failing tests never run `shell.Run`; the closed DB is the test's own `t.Cleanup`). But the production analogue exists, latent: `shell.Run`'s shutdown (shell.go ~1298–1330) does `StopAdmission` → HTTP shutdown → `CheckpointTruncate` → `db.Close()` and never calls `sched.WaitInFlight()`; `procCtx` (shell.go:1149, from `context.Background()`) is cancelled only by `defer procCancel()` AFTER `db.Close`, and `sched.Run`'s own `wg.Wait()` is not awaited by the shell. So a direct arm whose `Drive` lands its terminal state just before `db.Close` loses its capture with this same ERROR line. Consequence is bounded and honest: on restart the pair has an ended arm and NULL `direct_text`, the driver skips it, the failed-pair card offers decline (one sample lost, nothing corrupted). It is the same gap for every in-flight dispatch's `settleTerminal` (queue row + receipt). Whether shutdown should drain/park in-flight dispatches before close is an S01.6 question, and I left production alone as briefed.

## Draft STATE landing line

CI-1 landed (c250d9e, test-only): internal/shell benchmark driver/verbs tests now wait on the pair's capture (new driverEnv.captured polls Store.CapturedDirectText, 10 s/2 ms) instead of run idleness. stage.dispatchDirect lands the run terminal before BenchmarkCapture, so waitIdle returned early and the test's own db.Close killed the capture. Repro'd with a 50 ms scratch delay (CI messages verbatim). Green: count=20 GOMAXPROCS=1, shell count=3, serial battery. OPEN: shell.Run shutdown closes the DB without sched.WaitInFlight (latent prod race, unpatched).
