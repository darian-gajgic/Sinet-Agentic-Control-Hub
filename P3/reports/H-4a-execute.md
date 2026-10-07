# H-4a execute — the `main` guard + landing tags (gate C2a)

Light-path process packet (no brief, no Go). Spec: the launch prompt, the H-4a row in `P3/STATE.md`, item 2 of `P3/gates/harness-hardening-gate.md` (answer (a)). Worktree `/home/sinep/Sinet-Agentic-Control-Hub/.claude/worktrees/agent-a6d880d498293a5ac`, branch `worktree-agent-a6d880d498293a5ac`, base `93f4ab8`.

## Commits

- `70527ab` H-4a: main guard + landing tag tests, red (gate C2a)
- `914a2ec` H-4a: main guard + landing tags (gate C2a)
- this report

## What shipped

### 1. The CI guard (`loop.sh`, `lib.sh`)

- `ci_state` (lib.sh) runs `gh run list --branch main --limit 1 --json status,conclusion,headSha,url` in `P3_ROOT` (`timeout 60`, `GH_NO_UPDATE_NOTIFIER=1`, `GH_PROMPT_DISABLED=1`, stderr captured apart from the JSON) and prints one line: `GREEN <sha> <url>`, `RUNNING <status> <sha> <url>`, `RED <conclusion> <sha> <url>` or `UNKNOWN <reason>`. Every status other than `completed` is RUNNING (gh 2.95.0 documents `queued`, `in_progress`, `requested`, `waiting`, `pending`). UNKNOWN covers: no gh on PATH, gh exit ≠ 0 (auth, API, timeout; the first 160 chars of stderr are kept), `[]`, unparseable output.
- `ci_clear` (loop.sh) is the `wait_for` predicate. GREEN: log `CI on main: success <sha>`, go. RUNNING: `WAIT_S=P3_CI_POLL`, log, wait. RED: `notify "P3 loop waiting: CI on main <conclusion>" "<url>"` once per run URL (`CI_NOTIFIED`, for the loop's lifetime), log with the RESUME hint, wait `P3_WAIT_POLL`. UNKNOWN: log `CI guard: <reason> — proceeding`, go.
- `wait_for` keeps its STOP (exit 0) and RESUME (consume, return) semantics. Its sleep is now `WAIT_S`, reset to `P3_WAIT_POLL` (default 600, the old literal) before each predicate call; a predicate may shorten it.
- Main loop: `[ "$P3_CI_GUARD" = 0 ] || wait_for ci_clear "CI on main"` right after `reap_orphans`, so before the model probe and before every sitting launch, `--once` included. The `LOOP start` line carries `ci_guard=`.
- Knobs (lib.sh): `P3_CI_GUARD` (`1`), `P3_CI_POLL` (`120`), `P3_WAIT_POLL` (`600`).

### 2. Landing tags

- `tag_landing <log> <status> <before> <after>` (lib.sh) runs after `record_sitting`/`update_cap`/`archive_limit` and before the stall breaker and every exit (`--once`, DONE, breakers). A landing: `.landed` non-empty, or `progress_since before after`. It creates the annotated tag `sitting/<ts>` on `<after>`. `<ts>` comes from `sitting_ts` (the transcript name, now-UTC fallback), which `archive_limit` now uses too (extraction, no behavior change). Message: the landed list joined with `, `; with none, `no landed list; commits beyond bookkeeping <before7>..<after7>`. Push: `GIT_TERMINAL_PROMPT=0 timeout 120 git -C "$P3_ROOT" push -q origin refs/tags/sitting/<ts>`. A tag or push failure logs `TAG … failed: …` / `TAG … created, push failed: …` and returns 0. Success logs `TAG sitting/<ts> → <sha7> pushed (<landed>)`.

### 3. Tests (`test-loop.sh`): 43 → 117 checks

- **Isolation.** Every loop now runs in a temp repo with a temp bare origin. `P3_ROOT` is exported before `lib.sh` is sourced, so `RUN_DIR`, `STATUS_FILE`, `STOP_FILE`, `RESUME_FILE`, `LOOP_LOG` and `loop.lock` all live in `$T`. The harness files (`lib/loop/sitting.sh`, `sitting-prompt.md`, `cap`, `cli-version.pinned`, `fixtures/`) are copied in, plus non-empty placeholders for the preflight files. The tag section refuses to run unless origin is the temp bare repo. This was required: the pre-existing stub fixtures carry `landed` lists (`continue` → P3-TQ-8, `land3` → A, B, C), so with tagging in place the old layout would have tagged the shared repo and pushed to GitHub.
- **Stub `gh`** on the stub PATH, default green, so the 43 existing checks run unchanged and green. A state sequence comes from `$T/ghseq` (the last state sticks): green, red, red2 (a second run), running, queued, none, error (HTTP 401, exit 1), resume/stop (touch RESUME/STOP, answer red). Its argv is logged.
- **gh absent:** a PATH where each directory holding a `gh` is swapped for a symlink copy without it (generic; on this host `~/.local/bin`).
- **New claude stub sittings:** `landcommit` (a commit + landed [P3-X-1, P3-X-2]), `bookkeeping` (a P3/STATE.md commit, landed []), `progress` (a commit, landed []), `crashwork` (a commit, no status → CRASH).
- **Guard cases (46 checks).** Each `→` result is the ordered trail of guard readings and sitting starts in `loop.log`.
  - exact gh argv.
  - green → `success>SITTING`.
  - red red green → `failure>failure>success>SITTING`, 3 calls, one notification with the exact text and URL, re-checks at `P3_WAIT_POLL` (`P3_CI_POLL=7` set so a mix-up would show).
  - red + RESUME → `failure>failure>RESUME>SITTING`, one notification.
  - red + STOP → `failure>failure>STOP`, no sitting.
  - queued running green → `queued>in_progress>success>SITTING`, re-checks at `P3_CI_POLL` (`P3_WAIT_POLL=7` set so a mix-up would show), no notification.
  - gh absent, gh error, no runs → the reason logged, then the sitting runs.
  - `P3_CI_GUARD=0` → 0 gh calls.
  - the guard runs before every sitting of a two-sitting chain.
  - the same red run across two waits → one notification; a newer red run → a second one (timed_out, run 1003).
  - lib.sh defaults are 1/120/600.
- **Tag cases (28 checks).**
  - landing → exactly one new tag, named `sitting/<ts>` with the sitting's transcript ts, annotated, peeling to the post-sitting HEAD (which moved), message `P3-X-1, P3-X-2`, the same tag object in the bare origin, plus the log line.
  - bookkeeping-only → no tag, origin unchanged.
  - progress-only → tagged with the fallback message, pushed.
  - landed list without a commit → tag at the unchanged HEAD.
  - CRASH after a commit → tagged and pushed before the `--once` exit 1.
  - origin unreachable → tag kept locally, `created, push failed` logged, exit 0, no tag in origin.

### 4. README (`P3/run/README.md`)

Changes: file table; `Run it` counts (117 / 53) plus `git tag -n1 -l 'sitting/*'`; the test-isolation note; the **Main guard + landing tags (H-4a, gate C2a)** section (guard, tags, rollback, restart and release-pipeline notes); the knobs; the GATE poll is now `P3_WAIT_POLL`. Per the launch prompt this README section replaces the CONVENTIONS § draft.

## Test evidence

- Red (`70527ab`): 117 checks.
  - The 43 pre-existing checks: all ok under the temp-repo layout and the default-green stub.
  - 40 new checks FAIL: every positive guard and tag behavior.
  - 34 new checks ok: exit codes, negative assertions and plumbing that hold without the feature.
- Green:
  - `P3/run/test-loop.sh`: `---- 117 checks, ALL PASS` (exit 0, about 90 s).
  - `P3/run/test-classify.sh`: `---- 53 checks, ALL PASS`.
  - `bash -n` OK on lib.sh, loop.sh, test-loop.sh.
- The first green run scored 115/117. `tag-push-fails` started in the same second as the previous `--once` stub sitting, so `git tag` refused ("already exists"). The failure was logged, not fatal, and the loop exited 0, which showed the never-fatal path working. Test fix: the tag-section sittings now start at least 1 s apart. The tag name has second resolution, and real sittings are minutes to hours apart. No assertion changed.
- Isolation, checked after all runs:
  - `git tag -l 'sitting/*'` in the real repo: 0.
  - The worktree's `P3/run` has no `status.json`, `STOP`, `RESUME`, `loop.lock` or `loop.pid`, and `log/` holds only my scratch dir.
  - `cap` and `cli-version.pinned` unmodified.
  - The main checkout and its live loop were never touched.
  - No `go test` or vitest was running at any test run (pgrep checked before each).
- Not exercised against the real GitHub API (constraint: never call the API). The JSON fields and status vocabulary come from `gh run list --help` (gh 2.95.0, offline).

## Deviations (declared)

1. **Rollback recipe.** `git revert --no-commit sitting/<a>..sitting/<b>` fails on this repo's history because landings are `--no-ff` merges. Results in a scratch repo on git 2.53.0:

   | Command | Result |
   |---|---|
   | `git revert --no-commit sitting/<a>..sitting/<b>` | exit 128, "commit … is a merge but no -m option was given", a partial revert left staged (`git revert --abort` cleans it) |
   | `git revert --no-commit -m 1 $(git rev-list --first-parent sitting/<a>..sitting/<b>)` | exit 0; the index tree equals the `sitting/<a>` tree (with `<b>` = HEAD) |
   | `-m 1` over the full range | conflicts (the branch commits get reverted twice) |

   The README documents the first-parent form and why.
2. **`P3_WAIT_POLL`** is a new knob (default 600, the old literal). It replaces `wait_for`'s `sleep 600` and is shared by the gate, blocked and red-CI waits. Without it, "re-check every 600 s … proceeds when the stub turns green" could not be tested in less than ten minutes.
3. **"notify ONCE"** means once per red run (by URL), for the loop's lifetime. Re-entering the wait on the same red run (e.g. after a RESUME override) stays silent; a newer red run notifies again.
4. **"queued/in_progress"** is generalized to any status ≠ `completed`: `requested`, `waiting` and `pending` are the same unfinished state.
5. **test-loop.sh restructure.** All loops run in the temp repo and bare origin, including the 43 pre-existing checks; their assertions are unchanged (see Tests). Side effect: test-loop.sh no longer writes `STOP`, `status.json` or `log/` of the checkout it runs from. The old layout, run from main, would have touched a live loop's `STOP` and `status.json` and failed on its lock.
6. **`sitting_ts`** is extracted from `archive_limit` (no behavior change), so "ts as in archive_limit" holds by construction.

## Finding (pre-existing, out of scope, not fixed)

**F1 MED: a gate answered in its file never ends the GATE wait.**
- Cause: `loop.sh` calls `wait_for "gate_answered $GATEF" "gate $GATEF"`, and `wait_for` runs `! "$1"`. The quoted expansion makes bash look up a single command named `gate_answered P3/gates/…`. That fails with 127 "command not found", so the predicate is always false and only RESUME or STOP ends the wait.
- Evidence (scratch probe): quoted `"$1"` with `'pred yes'` gives `pred yes: command not found`; unquoted it gives true.
- Unattended-night impact: if a sitting hits a GATE overnight and the operator answers in the file next morning, the loop does not resume. `touch P3/run/RESUME` still works, and the README already names it.
- One-line fix in loop.sh: `gate_file_answered() { gate_answered "$GATEF"; }` plus `wait_for gate_file_answered "gate $GATEF"` (or run `$1` unquoted in `wait_for`).
- Now that `P3_WAIT_POLL` is a knob, a stub test is cheap: a GATE sitting, write `answered: yes`, expect the next sitting without RESUME.

## Residuals and operator notes

1. **Arming.** The live loop keeps the code it started with, so the guard and tags take effect only after a restart: `touch P3/run/STOP`, let the sitting end, relaunch in tmux. Merging under the running loop is safe. Bash has already parsed the whole `while` body, and its children use nothing H-4a changed: `sitting.sh` sources the new lib.sh but calls none of the new functions.
2. **Fails open by design** (spec): every gh failure proceeds. Examples: the wrong active gh account (404 on the private repo), or a second git remote with no `gh repo set-default` (`GH_PROMPT_DISABLED` turns that prompt into an error). Each case logs `CI guard: … — proceeding` on every sitting.
3. **Freshness.** The guard sees the newest run GitHub has created, so a push whose run does not exist yet is not seen. `headSha` is logged but not compared with `origin/main`; the spec only reads. CI's `cancel-in-progress` marks a superseded run `cancelled`, but the newer run is then the latest, so there is no false red. A manually cancelled latest run reads red (≠ success, per spec).
4. **Push auth.** The tag push authenticates through `gh auth git-credential`, so a 403 means the wrong active gh account. `GIT_TERMINAL_PROMPT=0` and `timeout 120` keep a credential prompt from hanging the loop. No git hooks are installed, and tag/commit signing is not forced.
5. **CI and tags.** CI does not trigger on tags (`on.push.branches: [main]`). The future tag-triggered release pipeline (S01.11) must match only its own tags (e.g. `v*`).
6. **reap_orphans.** test-loop.sh's loops still call `reap_orphans`, a host-wide kill of go test and vitest (unchanged). Run it only when no battery is running; the README says so.
7. **Stale status edge** (pre-existing). If `sitting.sh` exits in preflight before deleting `status.json` (a harness file missing), the previous sitting's status is classified again. Its `.landed` would then re-tag the same HEAD under a new ts, a harmless duplicate.

## Draft STATE landing line (≤600 chars)

H-4a LANDED (<merge>; tests 70527ab, impl 914a2ec; light path): loop.sh reads gh run list --branch main --limit 1 before every sitting incl. --once. Queued/in progress → re-check P3_CI_POLL 120 s; not success → notify once per run, re-check P3_WAIT_POLL 600 s; STOP/RESUME; no gh/error/no runs → log+proceed; P3_CI_GUARD=0 off. Landings tagged sitting/<ts> (annotated, landed list), pushed, never fatal. README rollback: first-parent -m 1 revert. 117/0 + 53/0. Restart the loop to arm. F1 open: gate-file wait.
