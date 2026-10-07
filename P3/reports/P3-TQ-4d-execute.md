# P3-TQ-4d — execute report

Light-path copy packet: user-facing copy and its pins, no brief. Spec: the launch prompt, the P3-TQ-4d row in `P3/STATE.md`, the last bullet of `P3/reports/P3-TQ-4c-execute.md` §"§79 / §80 amendment wording", S07.7, S07.8 [A16], CONVENTIONS §82. Worktree `/home/sinep/Sinet-Agentic-Control-Hub/.claude/worktrees/agent-a3db20427ec3542d9`, branch `worktree-agent-a3db20427ec3542d9`, base `1d34e75` (= origin/main). Local main's H-4a merge `05393cd` touches only `P3/run/*`, `P3/STATE-HISTORY.md` and a report, so it is disjoint from this packet.

## Commits
- `de96f75` P3-TQ-4d: BootstrapPostureNote claims pin, red (S07.8 A16, S07.7). Opens the amendment-A red window.
- `d52fa25` P3-TQ-4d: BootstrapPostureNote copy post-A16 (S07.8 A16, S07.7). Closes it.
- This report's commit.

## What shipped
- `internal/verify/bootstrap.go:54`, const `BootstrapPostureNote`: the middle sentence only. "every check rung is recorded as unverifiable here," became "the project's own checks are recorded as unverifiable here, anything the platform ran from the project's files is evidence only,". The rest of the sentence is unchanged. A script compared the constant at base and at `d52fa25`: sentences 1 and 3 are byte-identical, and the middle sentence is byte-equal to the launch prompt's text. The note grows from 377 to 455 bytes. The doc comment is untouched.
- `internal/verify/tq4d_posture_note_test.go` (new, `verify_test`): `TestTQ4dBootstrapPostureNoteNamesEachLane` asserts the three claims by fragment ("the project's own checks are recorded as unverifiable here", "anything the platform ran from the project's files is evidence only", "your review is what decides this work") and asserts that "every check rung is recorded as unverifiable" is absent (S07.8 [A16]: detected rungs run at bootstrap and can pass or fail).
- Packet diff vs base: these 2 files, +33/−1. `web/` and every pre-existing test file are byte-unchanged.

## Pins (sanction unused)
- No pin of the old literal exists. I swept Go sources and tests, `testdata/`, goldens and `web/src` (fixtures included) for the whole old sentence and 10 fragments of it. Hits are only the constant itself and P3 history docs (briefs, reports, walk findings, STATE), which are records and were not edited.
- Tests use the constant by symbol: `internal/api/deliverableposture_gf5_test.go:69,133` and `internal/api/projectfixtures_gf5_test.go:189`. Others check fragments of the unchanged last sentence ("restores the full ladder"): `internal/verify/bootstrap_gf4_test.go` (8 sites), `internal/verify/tq3_contracts_test.go:315`, `internal/stage/bootstrap_gf4_test.go:141,263` and `internal/api/deliverableposture_gf5_test.go:107`. No web fixture carries a posture note, and no 64-hex digest is pinned in the verify/stage/api/shell/review tests. Zero sanctioned edits were used.
- Identity: the posture finding's key is `criterion|anchor|category` (`verify.go:119`) and excludes the text. `isPostureDisclosure` therefore still matches rounds recorded under the old wording, so no migration is needed. No length bound applies to finding Text or card Detail on these paths.

## Test evidence
- Red, before `d52fa25`: `go test -p 1 -count=1 -run TestTQ4d ./internal/verify/` FAIL with 3 errors (two claims missing, the stale claim present; review-decides already held). `go build ./...` was green.
- Green: the same run PASSes. `-run TestGF13 ./internal/api/ ./internal/intake/`: 6/6 PASS.
- Package batteries, serial with the live skip: `internal/verify` ok 4.9 s, `internal/api` ok 101.6 s, `internal/intake` ok 7.1 s, `internal/stage` ok 14.8 s. The verify package includes its own GF13 files.
- `gofmt -l internal/verify/` is empty and `go vet ./internal/verify/` is clean.
- Full battery: `go test -p 1 -count=1 -skip 'TestLivePhraseAndSummarize|TestLiveIntakeTriageClassifiesWebshop' ./...` in the foreground, after pgrep showed no foreign run. Exit 0 in 3m35s: 48 packages ok, 5 with no test files, 0 FAIL and no reaper signals. No `web/src` change, so no frontend battery was needed. No orphans remain.

## Deviations / notes
- None. No `web/` copy, no other wording, no refactor.
- Ledger (found during the sweep; out of scope under "nothing else"). The same post-A16 staleness appears in three places:
  (1) `internal/api/projects.go:1043-1046`, the `commandsDetail` cleared-commands answer: "the platform has nothing to run against this project's work … every check is recorded as unproven". This is server-authored requester copy, and only the GF13 jargon-class test covers it; nothing pins its wording.
  (2) `web/src/Deliverable.tsx:335-336`, the bootstrap box: "the platform could not run a build, tests or lint on the work". Frontend lane.
  (3) `internal/verify/bootstrap.go:16-23`, a package comment that still gives the A14-only reading ("every executable-ladder rung recording UNVERIFIABLE-HERE"). Comment only, LOW.

## Draft STATE landing line
P3-TQ-4d LANDED (`d52fa25` on red `de96f75`): `BootstrapPostureNote`'s middle sentence now names each lane (own checks unverifiable here · platform-run evidence only · judge advisory · review decides); first/last sentences byte-identical, finding key unchanged. New `TestTQ4dBootstrapPostureNoteNamesEachLane` pins the claims + the stale "every check rung" absent. No pin of the old literal existed: zero sanctioned edits. 3 red → green; GF13 6/6; full battery 48 ok. Ledger: same stale claim in `commandsDetail` (projects.go:1044) and Deliverable.tsx:336.

## Draft CONVENTIONS §82 amendment bullet
Append to §82. In its Process record, drop "`BootstrapPostureNote` wording (below);" from the ledger.

- **The disclosure names each lane (P3-TQ-4d, 2026-10-07; S07.8 A16, S07.7).** `BootstrapPostureNote`'s middle sentence says the project's own checks are recorded as unverifiable here, anything the platform ran from the project's files is evidence only, the judge is advisory, and your review decides. Bootstrap copy never claims every rung is unverifiable, because detected rungs run and can pass or fail. Pinned by `TestTQ4dBootstrapPostureNoteNamesEachLane`. Ledger: the same claim is in `commandsDetail`'s cleared answer (projects.go:1044) and the web bootstrap box (Deliverable.tsx:336).
