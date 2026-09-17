# P3-TQ-1 — the artifact-snapshot race (TQ-F1): grounded brief

Single-use grounding artifact (SKILL.md Stage 1; stamped EXPIRED at landing). Binding sections: **S02.4 (d)**, **S02.3**, **S02.5**, **S13.5**, S02.10; CONVENTIONS §2, §3, §5, §8, §10, §14, §23. Finding: `P3/design/taskquality-webshop-findings-2026-09-16.md` TQ-F1; ratified as a packet 2026-09-17 in `P3/gates/rework-sitting-gate.md` Part C ("conformance defect, no amendment; P3-TQ-1 backend, small"). Grounding branch: `worktree-agent-a1d594b76e5908bc5` (worktree of `main` at `cc8fd85`).

Packet shape: **one production file changes** (`internal/project/workspace.go`, `Store.Snapshot`), five acceptance tests already committed (three RED, two green pins), no FSM change, no adopted-code change, no ⚙.

---

## 0. The defect, with evidence

**The recorded run** (`~/.sinet-rework-sitting/platform.db` run_events for `t-3120e8e3d14591d3.execute`, 2026-09-16T20:39:35Z, read from the DB/WAL with `strings`, verbatim):

```
stage.finished {"stage":"S-4","kind":"execute","outcome":"error","detail":"adapters: artifact snapshot (S02.4d): project: git -c (exit 128): fatal: unable to stat 'src/components/PartGrid.jsx.tmp.107557.477ad8f56e3d': No such file or directory"}
run.state_changed {"from":"running","to":"crashed","reason":"stage dispatch failed","actor":"platform","detail":{"cause":"step S-4 session: adapters: artifact snapshot (S02.4d): project: git -c (exit 128): fatal: unable to stat 'src/components/PartGrid.jsx.tmp.107557.477ad8f56e3d': No such file or directory"}}
```

**The engine's temp-file shape, verified from the engine binary that ran the sitting** (installed `@anthropic-ai/claude-code` 2.1.273, `bin/claude.exe` dated 2026-09-16 03:40; `strings` over the bundle): the Write path builds its temp suffix as `` `.tmp.${process.pid}.${randomBytes(6).toString("hex")}` `` appended to the target path and staged beside it (`Mhn(n.stagingDir, o, w, …)`), and the engine's own recognizer for its transient files is the regex `/\.tmp\.(?:[0-9a-f]{8}|\d+\.[0-9a-f]{12})$/` (a second, 8-hex form exists for other internal writes). The recorded path `PartGrid.jsx.tmp.107557.477ad8f56e3d` = `<target>.tmp.<pid>.<12 hex>`. (The lock pins the engine at 2.1.218 — `components.lock` "claude CLI (engine)"; the installed 2.1.273 is a pin↔installed delta to report per CONVENTIONS §10, outside this packet — see §9 O4.)

**The exact command sequence today and where the failure surfaces** (question 1):

| step | file:line | what runs |
|---|---|---|
| snapshot stage | `internal/project/workspace.go:155` | `git -c core.quotepath=false -c gc.auto=0 -c core.excludesFile=<root>/platform-excludes add -A` (via `gitRaw`, `git.go:95-114`; hermetic env `git.go:53-73`) |
| error shape | `internal/project/git.go:86` | `project: git %s (exit %d): %s` with `args[0]` = `-c` → the recorded `git -c (exit 128)` |
| no-change probe | `workspace.go:160` | `git diff --cached --quiet` (exit 0 → return prior tip; 1 → commit) |
| commit | `workspace.go:176` | `git commit --no-verify --no-gpg-sign -m "sinet: platform snapshot (Spec S13.5)"` |
| caller (per paid call) | `internal/adapters/driver.go:339-344` | `Driver.checkpoint`: `d.Snapshot(ctx, r.ID)` BEFORE the checkpoint WriteTx; error wrapped `adapters: artifact snapshot (S02.4d): %w` and returned |
| seam | `internal/shell/project_seams.go:615-620` | `projectSeams.Snapshot` → existing worktree path → `proj.Snapshot(ctx, path)` |
| session end | `driver.go:217-229` (`DriveStage`) | the pump keeps draining the engine and remembers the FIRST persistence error; after `sess.Wait` (the engine's own `end_turn`) it returns `(out, pumpErr)` |
| stage error | `internal/stage/runner.go:299-304` | `err != nil` → `appendStageFinished(…, StageOutcomeError, err.Error())`; returns the error |
| crash | `internal/stage/skeleton.go:730-732` → `skeleton.go:448` | `dispatchExecute`: `s.crash(ctx, r.ID, "step S-4 session: …")` → `Runs.Transition(StateCrashed, reason "stage dispatch failed")` |

Two further `cfg.Snapshot` callers exist and ride the same `Store.Snapshot`: the execute-leg stage close (`skeleton.go:781-785`, best-effort: logged, never a crash) and the review sink (`review_sink.go:112-113`).

**Why git dies, mechanically.** `git add -A` first walks the worktree (`fill_directory`, applying `.gitignore` + `core.excludesFile`), collecting the untracked paths to add; then `add_file_to_index` does `lstat()` on each collected path and `die_errno("unable to stat '%s'")` on failure (exit 128). A tracked file that vanishes is staged as a deletion (no die); only an UNTRACKED path that vanishes between the walk and its lstat kills the process. The engine's atomic write (create temp → write → rename over the target) puts an untracked temp on disk for milliseconds; a per-paid-call checkpoint (S02.4) runs while the engine is live and writing, so this race is structural to checkpoint-per-paid-call on the Claude lane, and the probability compounds with session length — the finding's "kills any long task probabilistically".

**Blast radius of the false crash** (S02.5): the skeleton's own `crash` leaves a `crashed` corpse; the ladder classifies DEAD, forks `execute.g1` from the last checkpoint (S02.5 step 2), and — until P3-TQ-2 lands — re-drives the plan from S-1 over the dirty worktree (TQ-F2): $1.19 discarded, ~10 min. Each false crash also burns one of `⚙ recovery.max_attempts = 3` (lineage-cumulative `runs.recovery_attempts`, CONVENTIONS §8): three such races on one task TOMBSTONE it.

---

## 1. Requirements

**R1 — the snapshot survives a vanishing untracked path (S02.4d; S13.5 "snapshot commits … written at checkpoint boundaries").** `Store.Snapshot` retries the staging step (`git add -A`, identical arguments) when the invocation exits **128** (git's `die` code), up to `snapshotAddAttempts` total attempts, then proceeds unchanged. The retry is the SAME command on the tree as it is at that instant — no pathspec narrowing, no stderr-derived path exclusion, no change to the excludes file. A spawn/OS error from `gitRaw` (code −1 with `err`, e.g. git absent or ctx cancelled) is never retried (loud immediately, the existing posture `git.go:29-31`). No sleep/backoff: the vanish window has closed by the time git has died.

**R2 — content faithfulness is byte-exact (S13.5 tree-level; S02.4d "artifact snapshot ref"; CONVENTIONS §23 TREE-LEVEL).** A snapshot taken under the race is byte-identical to a snapshot of the same final tree taken with no race: identical git TREE id (paths + bytes + modes). The transient that vanished is absent because it is gone from the tree, not because anything filtered it. The produced file (the rename target) is captured with the bytes the engine wrote.

**R3 — a real failure stays loud (CONVENTIONS §14 "never faked", §23 "a repo-backed snapshot FAILURE is loud at the sink"; S02.4 D7).** Past the bound, or on any non-128 exit, `Snapshot` returns an error that (a) wraps the last git error with `%w` so git's stderr survives verbatim (`unable to stat '<path>': …`), (b) contains the word `attempts` with the count, and (c) has committed nothing (HEAD unchanged); a spawn/OS error is returned as today, unwrapped by the loop (`git.go:113`). The retry can never turn a genuinely unreadable tree into a success: each attempt is the same `add -A`, and a persistent cause fails every time (pinned by `TestTQ1SnapshotUnreadableTreeStaysLoud` on this host git 2.53.0 with a listable-but-unsearchable directory: die shape `unable to stat 'locked/secret.txt'`, no `index.lock` left behind, snapshot succeeds once the tree is readable again).

**R4 — a transient that STAYS is captured, never hidden (S13.5 junk rules unchanged; §23 "junk rules are STRUCTURAL").** The platform ignore list (`internal/project/ignore.go:20-47`) is NOT extended for the engine's temp shape. An orphaned `<target>.tmp.<pid>.<hash>` (the engine died mid-write) is real tree content: the snapshot carries it beside its possibly-stale target, so the reviewer and the S13.1 diff see the evidence that the target may be incomplete. Nothing is dropped, so no separate record is needed. (Pinned by `TestTQ1SnapshotStayingTransientIsCapturedNotHidden`.)

**R5 — the bound is a structural constant, not ⚙ (S01.10; S13.5/S02.4 ratify no key; §23 ⚙ posture; §7 `sseBatchSize` / §14 `maxQuestionsPerCard` precedent).** `const snapshotAddAttempts = 3` in `internal/project/workspace.go`, unexported, with a doc comment giving the reason: one retry clears any single vanish (the file is gone by the second walk); the third attempt covers a second independent vanish during a multi-file write burst; the bound exists so a pathologically churning tree fails LOUD instead of looping. Flagged to the next gate under the standing settings-tab directive (memory `settings-tab-see-change-everything`), exactly as §23 flagged the junk rules. No S18 sweep; no `settings` change; the settings-index tally test stays green untouched.

**R6 — no change to stage endings, the FSM, or the error path (S02.3; S02.5; CONVENTIONS §8).** `driver.go`, `runner.go`, `skeleton.go`, `review_sink.go`, `project_seams.go` are untouched. The fix lives entirely inside `Store.Snapshot`; all three callers benefit without change. `Store.Snapshot`'s signature and `""`/prior-tip semantics (`workspace.go:142-150`) are unchanged.

**R7 — hermetic git discipline holds (CONVENTIONS §23; §10 rule 4).** The retry uses the existing `gitRaw` (hermetic env, `-c gc.auto=0`, `core.excludesFile`); no new git flags, no `stash`/`--amend`/`--force`/`push`/`jj` strings (`conformance_test.go:62-83` tripwire); no new dependency; behaviour asserted by fixture tests on this host git, never parsed from `--help`.

---

## 2. Decision record — the five questions

**(1) Sequence and surfacing:** §0 table. The failure is a persistence error inside the per-paid-call checkpoint (`driver.go:339-344`), remembered by `DriveStage` while the engine finishes normally, then turned into `stage.finished outcome=error` at `runner.go:302` and `running→crashed` at `skeleton.go:730-732`/`448`.

**(2) The right fix — bounded retry of the identical `add -A`.** Compared against the spec's own words (S13.5: tree-level, captures bash side effects, junk-excluded via platform ignore rules; S02.4d: a faithful artifact ref per paid call):

- *Exclude `*.tmp.<pid>.<hash>` via the platform ignore rules* — rejected. It is engine-specific (a claude 2.1.273 naming; the Kimi Code CLI lane — CONVENTIONS §67 — is a third engine whose temp naming is unverified, and any pin bump can change it); it does not cover bash-side vanishers (package managers, test runners, build tools write and remove scratch files during a live session — the same die); a static glob is not *provably* content-preserving (a produced file can carry any name); and it would HIDE an orphaned temp that is review evidence (R4). It also silently changes S13.5's ratified junk set, a structural code change with no spec words asking for it.
- *Quiesce before add* — impossible without redesign: S02.4 checkpoints per paid call while the engine is live and its tools run; there is no quiescence signal, and moving the snapshot to a quiet moment changes checkpoint timing (FSM/D7 territory, out of scope).
- *Bounded retry* — chosen. Provably never a content alteration (the retry is the same `git add -A` on the tree as it is; equality to a no-race snapshot is asserted as tree-id identity). Provably never masks a real failure (bounded; the last git error is returned verbatim; a persistent cause fails every attempt). Engine- and lane-agnostic: covers every vanishing-untracked-path instance, whatever wrote it. One loop in one function. Gate = exit 128 (git's `die`) rather than stderr text: stderr parsing is brittle and adds no safety once the loop is bounded, and the sibling live-engine race (`index.lock` contention from the engine's own git use in its cwd, also exit 128) rides the same gate for free — this is not extra handling, it is the absence of a narrower gate. Non-128 exits and spawn errors are not retried.

**(3) Invariants pinned as property tests:** `TestTQ1SnapshotIsByteFaithfulAcrossRetry` (5 seeded random iterations: 1–5 produced files, some finished, some mid-atomic-write with the temp on disk and the target absent or stale; the shim completes the engine's renames under the first `add -A` and dies; the raced snapshot's tree id must equal a control snapshot of the final files with no race; exactly 2 attempts). `TestTQ1SnapshotStayingTransientIsCapturedNotHidden`: a temp that never vanishes is in the snapshot byte-exact — the honest behaviour is capture, not exclusion (R4). `TestTQ1SnapshotRetryIsBoundedAndLoud`: every attempt dies → exactly `snapshotAddAttempts` (3) attempts, error carries `unable to stat '<path>'` verbatim and the word `attempts`, nothing committed.

**(4) May the platform's own bookkeeping mark a stage `error` when the engine succeeded?** What the spec implies: S02.3's `running→crashed` edge is for the engine dying; S02.5's classes are ALIVE / WEDGED / FINISHED-DURING-OUTAGE / DEAD, and FINISHED-DURING-OUTAGE exists precisely so that "a terminal result newer than the last checkpoint" is HARVESTED, never redone. A completed engine session with an incomplete platform checkpoint is a bookkeeping gap on the platform's side, not a dead engine; recording it as a stage `error` + `crashed` and forking is a conformance gap against S02.3/S02.5, and the honest record would be the engine's outcome on `stage.finished` plus a platform checkpoint-failure event carrying the cause. That is a redesign of stage endings and of what `DriveStage` returns (and it touches `internal/stage`, under concurrent edit); **it is out of this packet**. This packet removes the transient trigger inside `Snapshot`; the residual — a GENUINELY unreadable tree — still fails loud and still reaches today's error path, which is correct for now: S02.4 (d) cannot be honoured, and writing a checkpoint row with an empty ref would be the NULL-indistinguishable fake §23 forbids. Recorded for coordinator triage in §9 (O1, O2), alongside TQ-F8.

**(5) ⚙:** none new, none consumed. R5's bound is a structural constant with its reason; no S18 matter, no STOP.

---

## 3. Seams to respect

- `func (s *Store) Snapshot(ctx context.Context, worktree string) (string, error)` — signature, prior-tip/no-`--allow-empty` semantics and `""`-for-unborn unchanged (`workspace.go:142-180`).
- `projectSeams.Snapshot` (`project_seams.go:615`), `Driver.Snapshot` (`driver.go:46`), `stage.Config.Snapshot` (`stage.go:378`) — untouched; the driver's wrap text `adapters: artifact snapshot (S02.4d): ` untouched.
- `gitBin` stays a `const` (`git.go:32`); PATH resolution is the test seam — add NO production hook for tests.
- `internal/stage/{engines.go,skeleton.go,surface.go}`, `internal/worker`, `internal/metering/receipt.go` are under concurrent edit on `main`: this packet must not touch `internal/stage` at all (the snapshot fix does not need to).
- `ignore.go` / `platformIgnorePatterns` unchanged (R4).

## 4. Files expected to change

- `internal/project/workspace.go` — `Snapshot`: the bounded retry around the `add -A` invocation (switch that one call from `s.git` to `s.gitRaw` so the exit code is visible; keep `-c core.excludesFile=…`), the `snapshotAddAttempts` constant with its doc comment, the function's doc comment extended by one sentence naming the retry and its bound.
- `internal/project/tq1_snapshot_race_test.go` — committed by grounding (RED); the executor may not modify it.
- Nothing else. No `components.lock` change (host git consumed unmodified, no new flags).

Reference shape (non-binding; the tests are the contract):

```go
var addErr error
for attempt := 1; attempt <= snapshotAddAttempts; attempt++ {
    code, _, stderr, err := s.gitRaw(ctx, worktree, id, "-c", "core.excludesFile="+s.excludes, "add", "-A")
    if err != nil { return "", err }            // spawn/OS failure: loud, never retried
    if code == 0 { addErr = nil; break }
    addErr = fmt.Errorf("project: git add -A (exit %d): %s", code, strings.TrimSpace(stderr))
    if code != 128 { break }                   // only git's die is the transient class
}
if addErr != nil {
    return "", fmt.Errorf("project: snapshot staging failed after %d attempts: %w", attempts, addErr)
}
```

## 5. Adopted components touched

- **host git CLI** (`components.lock` "git (host CLI)"): consumed unmodified, same subcommands and flags; the retry re-invokes the same command.
- **claude CLI (engine)**: not touched; its temp naming is evidence only. Pin delta 2.1.218 (lock) vs 2.1.273 (installed) reported in §9 O4.

## 6. CONVENTIONS constraints that bind

§2 (errors wrap with `%w`; no ⚙ as a constant — R5 is a structural constant by the §7/§14/§23 precedent, stated as such in its doc comment); §3 (stdlib `testing` only; `t.TempDir()` only; the amendment-A red window: grounding commits RED tests, `go build ./...` stays green, the executor's implementation commit closes it); §5 (subject `P3-TQ-1: <summary> (S02.4, S13.5 refs)`; explicit pathspec staging; no push); §8 (no FSM change); §10 rule 4 (fixture-asserted git behaviour); §14 (never faked); §23 (hermetic per-invocation git; TREE-LEVEL `add -A`; never `--allow-empty`/`stash`/`--amend`/`--force`/`push`/`jj`; no ⚙ for snapshot mechanics; loud at the sink).

## 7. Acceptance checklist (the evaluation rubric)

1. `go test -p 1 -count=1 -run 'TestTQ1' ./internal/project/` → 5/5 PASS (the three RED tests turn green; the two pins stay green).
2. `go test -p 1 -count=1 ./internal/project/` fully green (existing `TestSnapshot*`, `TestNoBannedGitMechanisms`, `TestRW14*`, accept tests untouched and passing); `go build ./...`, `go vet ./...`, `gofmt -l` clean; `go run ./tools/lockgate` green.
3. The diff touches `internal/project/workspace.go` only (plus any NEW test files the executor adds); zero changes under `internal/stage`, `internal/adapters`, `internal/shell`, `internal/project/ignore.go`, `components.lock`, `Spec/`.
4. Retry semantics exactly R1: same arguments each attempt; gate = exit code 128 of the `add` invocation only; spawn errors (`err != nil` from `gitRaw`) not retried; no sleep; no stderr parsing; at most 3 attempts.
5. Failure past the bound: error text contains `attempts` and, via `%w`, the last git stderr verbatim (`unable to stat '…'`); HEAD unchanged; no `index.lock` left (R3).
6. `snapshotAddAttempts` (value 3) is an unexported `const` in `workspace.go` with a doc comment stating the reason and that it is structural, not ⚙ (S13.5/S02.4 ratify no key); no new settings key anywhere.
7. `Snapshot`'s doc comment names the retry and its bound; no research narration or changelog prose in code.
8. Held-out probes the evaluator should run (amendment C): (a) delete the loop (single attempt) → `TestTQ1SnapshotSurvivesVanishingUntrackedFile` and the property test fail; (b) bound → 1 → `TestTQ1SnapshotRetryIsBoundedAndLoud` fails on the count; (c) gate widened to "any non-zero exit including −1" → the spawn-error posture regresses (probe with a `git` that is not executable, or by asserting a ctx-cancelled call returns without three attempts); (d) an executor "fix" that excludes `*.tmp.*` instead → `TestTQ1SnapshotStayingTransientIsCapturedNotHidden` fails.

## 8. Acceptance-test specifications (committed: `internal/project/tq1_snapshot_race_test.go`)

Seam used (stated per amendment A): the race is inside one git process, so no Go-level seam exists between the walk and the add; the smallest seam the code offers is PATH resolution of `gitBin` (`git.go:32`, resolved by `exec.CommandContext` from the parent's PATH, which `gitRaw` also copies into the child env). A shim `git` (installed ahead of the real `/usr/bin/git` via `t.Setenv("PATH", …)`) intercepts only an argument pair `add` `-A`, counts interceptions, on the first one performs the engine's rename (temp → target, `mv -f`) and exits 128 with git's verbatim die line, and `exec`s the real git for everything else. No production code was changed for the tests. Run command: `go test -p 1 -count=1 -run 'TestTQ1' ./internal/project/`.

| test | setup | assertions | state at grounding |
|---|---|---|---|
| `TestTQ1SnapshotSurvivesVanishingUntrackedFile` (R1, R2) | active project, workspace `t-3120e8e3d14591d3`; on disk `src/components/PartGrid.jsx.tmp.107557.477ad8f56e3d` (content P) + `src/App.jsx` (content A); shim dies once with the recorded message after renaming the temp onto `src/components/PartGrid.jsx` | `Snapshot` returns nil error and a sha ≠ base; tree has `src/components/PartGrid.jsx` and `src/App.jsx`, no name containing `.tmp.`; `cat-file blob` of both == P / A byte-exact; shim counter == 2 | **RED**: `Snapshot must survive the engine's vanishing temp file (TQ-F1): project: git -c (exit 128): fatal: unable to stat 'src/components/PartGrid.jsx.tmp.107557.477ad8f56e3d': No such file or directory` |
| `TestTQ1SnapshotIsByteFaithfulAcrossRetry` (R2 property) | seed `0x5413120e8e3`, 5 iterations; k∈[1,5] produced files, each either finished on disk or mid-write (temp on disk, target absent or stale); control fix snapshots the final files with no race → `HEAD^{tree}`; raced fix + shim (renames all temps, dies once) | raced `HEAD^{tree}` == control tree id; counter == 2 | **RED** 5/5 iterations, same reason (`unable to stat 'src/f0.js.tmp.<pid>.<hash>'`) |
| `TestTQ1SnapshotRetryIsBoundedAndLoud` (R3, R5) | shim dies on EVERY `add -A`; one produced file on disk | error non-nil; contains `unable to stat 'src/components/PartGrid.jsx.tmp.107557.477ad8f56e3d'`; contains `attempts`; counter == 3; HEAD unchanged | **RED**: `the error must state the exhausted bound: project: git -c (exit 128): fatal: unable to stat '…'` (today: one attempt, no bound stated) |
| `TestTQ1SnapshotUnreadableTreeStaysLoud` (R3 pin, real git) | `locked/secret.txt` + `src/App.jsx`; `chmod 0400 locked` (listable, not searchable) | error non-nil containing `unable to stat 'locked/secret.txt'`; `rev-parse --git-path index.lock` absent; HEAD unchanged; after `chmod 0700` `Snapshot` succeeds and the tree has both files | **GREEN pin** (proves this host git's die shape and lock hygiene; must stay green after the retry) |
| `TestTQ1SnapshotStayingTransientIsCapturedNotHidden` (R4 pin, real git) | `src/x.jsx.tmp.4242.0123456789ab` (never vanishes) + `src/x.jsx` | `Snapshot` succeeds; both names in the tree; orphan bytes exact | **GREEN pin** (guards against the rejected exclude alternative) |

Observed at grounding (this session, serial run): 3 FAIL / 2 PASS exactly as above; the package's pre-existing `TestSnapshot*|TestNoBannedGitMechanisms|TestRW14*|TestAccept*` selection stays green with the new file present.

## 9. Out-of-scope observations for coordinator triage (not for the executor)

- **O1 — a paid call's usage event is lost when the checkpoint fails.** `driver.go:339-344` runs the snapshot BEFORE the WriteTx at `driver.go:355`; on a snapshot error the `engine.usage` append and the checkpoint row are both skipped, so the paid call has neither its D7 row nor its metering event (S02.4 "MUST be written after every paid model call"; S10). This packet removes the transient trigger; the residual (a genuine snapshot failure) still drops the usage event. Candidate follow-up with (4).
- **O2 — the stage-close snapshot is log-only.** `skeleton.go:781-785` swallows a stage-close snapshot failure into a Warn line; the verify mint then reads a stale HEAD via `SnapshotAndBase` with no durable record. §14-adjacent (a silent degrade); pairs with TQ-F8's run-surface honesty.
- **O3 — `commit` / `diff --cached` steps** can meet `index.lock` contention from the engine's own git use in its cwd (also exit 128); no such failure is in the record, so no retry is specified for those steps (no handling for the unevidenced).
- **O4 — engine pin delta:** lock 2.1.218 vs installed 2.1.273 (the binary that ran the sitting). CONVENTIONS §10: report LOUDLY, never silently retarget; the operator's S03.3 bump procedure.
- **O5 — recovery-budget erosion:** every false crash consumed one of `⚙ recovery.max_attempts` (lineage-cumulative); with R1 the class no longer reaches the crash path, but any other platform-side persistence failure on a live session still does (see (4)).
