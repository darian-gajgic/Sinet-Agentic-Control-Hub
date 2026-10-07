# H-4b execute — evidence-gated evaluation (gate C1a)

Light-path process packet (no brief, no Go). Spec: the launch prompt, the H-4b row in `P3/STATE.md`, item 1 of `P3/gates/harness-hardening-gate.md` (answer: all three parts). Worktree `/home/sinep/Sinet-Agentic-Control-Hub/.claude/worktrees/agent-a6a9004408591ed38`, branch `worktree-agent-a6a9004408591ed38`. Base: fast-forwarded from `1d34e75` to `05393cd` (H-4a merged) at the start, then rebased onto `7fae46d` (TQ-4d landed) before the final runs, per the coordinator's base note. No conflicts; H-4a's README section is intact.

## Commits

- `97d19ee` H-4b: verdict gate + battery tests, red (gate C1a)
- `1769cca` H-4b: evidence-gated evaluation — battery.sh + PreToolUse verdict gate + signed contract (gate C1a)
- `a6c1c19` H-4b: battery logs under TMPDIR, so test-classify.sh leaves nothing in /tmp
- this report

## What shipped

### 1. `P3/run/battery.sh <worktree>`

- Resolves the worktree root, HEAD, branch (`detached` without one) and the main checkout (the parent of `git rev-parse --path-format=absolute --git-common-dir`).
- **Wait.** While another `go test` runs on the host, it polls `pgrep -f '[g]o test '` every `P3_BATTERY_POLL` s (30) and prints the PIDs and their command lines. Its own ancestor PIDs are excluded (D4).
- **Legs** run in CI order from the worktree root and never stop at a red one:
  - `clean`: no tracked change and no untracked file, `P3/reports/` excepted (D3).
  - `web-install` (`npm ci --ignore-scripts`), `web-typecheck`, `web-test`, `web-build`: only when `git diff --quiet main...HEAD -- web/src` fails, or when no `main` branch exists.
  - `gofmt` over the tracked Go files (what CI's `gofmt -l .` sees on a fresh checkout), `vet`, `build`, `test` (`go test -p 1 -count=1 -skip "$P3_BATTERY_SKIP" ./...`; the default skips the two GPU-live tests), `lockgate` (`go run ./tools/lockgate`).
  - `stable`: HEAD did not move during the run (D3).
- **Evidence** goes to `<main>/P3/run/log/evidence/<branch>-<sha>.json`, written atomically (tmp file + `mv`). A `/` in the branch name becomes `_` (D5). Fields:
  - `branch`, `head`, `ok` (every leg green), `worktree`, `web`, `skip`;
  - `legs[]`: `name`, `ok`, `rc`, `secs`; a red leg adds `tail` (its last 20 lines); the test leg adds `pkgs_ok`, `pkgs_notest`, `pkgs_fail[]` and `tests_failed[]`;
  - `logs`: the full leg output, under `${TMPDIR:-/tmp}/p3-battery.*`;
  - `times`: `started`, `finished`, `waited_s`, `total_s`.
- **Stdout**: one line per leg (plus the tail of a red one), then `battery: GREEN|RED <branch>@<sha7> → <file>`. Exit 0 iff `ok`, 1 when red, 2 on a usage error or a path outside any worktree.

### 2. The verdict gate: `P3/run/hooks/verdict-gate.sh` + `hooks.proposed.json`

- **Hook entry.** `PreToolUse`, matcher `Write|Edit|Bash`, `timeout` 30, command `"$CLAUDE_PROJECT_DIR"/P3/run/hooks/verdict-gate.sh` (shell form, placeholder in double quotes as the docs require).
- **Fast path.** Stdin that never mentions `-evaluate.md` exits 0 at once.
- **Write/Edit arm.** Fires when `tool_input.file_path` matches `(^|/)P3/reports/.*-evaluate\.md$` AND the written text (`content` for Write, `new_string` for Edit) matches `VERDICT[*_` ]*:[*_` ]*PASS`, case-insensitive (D2).
- **Bash arm (D1).** Fires when the `command` matches the PASS pattern AND names one or more `…P3/reports/…-evaluate.md` paths. A relative path resolves against the input's `cwd`, or against a leading `cd <dir> &&`.
- **The check.** For each report:
  - the report's worktree is the `git rev-parse --show-toplevel` of its nearest existing directory;
  - that worktree's HEAD must have `<main>/P3/run/log/evidence/<branch>-<sha>.json` with `ok == true` and `head == <sha>`;
  - otherwise the hook exits 2. Its stderr reads `VERDICT GATE: no battery evidence for <branch>@<sha7> (<file>)` (or `… is red`), then the exact battery command to run;
  - an evaluate report outside any git worktree is refused.
- **Everything else** exits 0 with empty stdout. A FAIL verdict needs no evidence.
- **`install-hooks.sh`**: header comment only.
  - Its top-level merge adds the `PreToolUse` key. The project settings have none today (checked read-only), and the user-level PreToolUse hook (`sudo-timestamp-gate.sh`) lives in `~/.claude/settings.json`, which the merge leaves alone.
  - Dry run of the merge on a /tmp copy of the main checkout's settings: the hook keys become `PermissionDenied, PreCompact, PreToolUse, StopFailure`, and every non-hook setting stays byte-identical.

### 3. Templates, runbook, READMEs

- **`spotcheck.md`**, new step 7: on PASS, append `## Acceptance contract (signed <YYYY-MM-DD>)` to the brief (the checklist verbatim) and commit the brief alone. The reply carries the signing commit; on FAIL nothing changes.
- **`evaluate.md`**:
  - the rubric is the signed contract, and any change after the signing commit (found with `git log -S`, shown with `git diff`) is a finding;
  - the full run is `battery.sh`, with probe files deleted first;
  - a new section, "PASS is evidence-gated";
  - the report opens with the verdict line and `Evidence: <file> (green|red)`;
  - a re-check re-runs the battery.
- **`SKILL.md`**: new **Amendment G**.
  - G1: the signed contract.
  - G2: the evidence-gated PASS, including the coordinator's landing check.
  - G3: a falsified DROP records its command and output in the evaluate report's `## Triage`.
  - The Stage 1b, 3 and 4 text, the table row and the landing checklist now point to it.
- **`P3/run/README.md`**: file table, `Run it`, the hooks paragraph and the knobs (`P3_BATTERY_SKIP`, `P3_BATTERY_POLL`), plus the section **Evidence-gated evaluation (H-4b, gate C1a)**. Per the launch prompt, that section replaces the CONVENTIONS § draft.
- **`P3/prompts/README.md`** and **`P3/briefs/README.md`** carry amendment G.

### 4. Tests (`test-classify.sh`): 53 → 117 checks

**The gate in isolation (32 checks).** The exact command string from `hooks.proposed.json` runs through `sh -c` on temp git repos, fed stdin shaped like the documented (and observed) PreToolUse JSON.
- No evidence → blocked, and the remedy names `battery.sh <worktree>`.
- Green evidence for HEAD → allowed, for both Write and Edit.
- Red evidence → blocked. A stale sha after a commit → blocked.
- Allowed: a FAIL verdict, an `-execute.md` report, an evaluate-named file outside `P3/reports/`, a `.bak` suffix.
- `**Verdict:** PASS` → blocked.
- Edit arm: blocked without evidence, allowed with it; an Edit without PASS → allowed.
- Bash arm:
  - a heredoc write: blocked without evidence, allowed with it;
  - allowed with evidence: `cd <repo> &&` from another cwd, and an absolute path inside `python3 -`;
  - the wrong cwd → blocked;
  - Bash with no report path, or a read-only `grep VERDICT` → allowed.
- Worktree: evidence in the worktree's own `log/` → blocked; in the main checkout's → allowed.
- A non-git path → blocked.
- Matcher: Write, Edit and Bash match; Read, NotebookEdit, WebFetch and Agent do not. Timeout is 30.

**`battery.sh` on temp Go modules with a stub `tools/lockgate` (32 checks).**
- Green: exit code, summary line, fields, leg list, test-leg counts, skip, web skipped, times, logs under `TMPDIR`, admission by the gate.
- An untracked report draft is excepted.
- A dirty tracked file: `clean` goes red and names the file, and a PASS is blocked.
- A red commit: the gofmt, test and lockgate legs go red; `pkgs_fail` and `tests_failed` are parsed; the unformatted file is named; the summary says RED.
- A `web/src` change on a branch → the web legs run, in CI order.
- A worktree on the slash branch `p3/slash` → evidence in the main checkout as `p3_slash-<sha>.json`, nothing in the worktree, PASS admitted.
- A fake foreign `go test` (`exec -a`) → waited for at least 2 s, and named.
- An ancestor shell whose command line names `go test` → not waited for, GREEN.
- A usage error and a path outside any worktree → exit 2.

## Hook contract: docs + live CLI

**Docs**: Claude Code 2.1.292 hooks reference, `https://code.claude.com/docs/en/hooks.md`, fetched 2026-10-07; quotes verbatim.
- "In addition to the common input fields, PreToolUse hooks receive `tool_name`, `tool_input`, and `tool_use_id`." The common fields include `cwd`.
- "For the file tools `Write`, `Edit`, and `Read`, `tool_input.file_path` is always absolute."
- The exit-2 table row for `PreToolUse`: "Blocks the tool call".
- "A hook that blocks by exiting 2 routes the same way as `"deny"`: Claude sees the stderr message as the denial reason."
- "If your hook is meant to enforce a policy, use `exit 2`." Exit 1 does not block.
- Timeouts: "A timed-out `command`, `http`, or `mcp_tool` hook doesn't block the tool call."
- A missing script exits 127 with a notice, and "a mistyped path in `settings.json` leaves the gate silently disabled".
- Hooks from settings "also run inside subagents".
- Worktrees: "`${CLAUDE_PROJECT_DIR}` stays put … `cwd` follows Claude".
- The shell form runs under `sh -c`; "In shell form, wrap each placeholder in double quotes."
- "Direct edits to hooks in settings files are normally picked up automatically by the file watcher."

**Live check.** The real CLI 2.1.292 ran as `claude -p --model haiku --setting-sources project --tools … --permission-mode acceptEdits` with `DISABLE_AUTOUPDATER=1`, in the throwaway repo `/tmp/h4b-live.7I9CZP` (a trivial Go module plus a stub lockgate). That repo held the exact `PreToolUse` entry from `hooks.proposed.json` and a stdin-capture hook beside it.

| Run | Call | Evidence | Outcome |
|---|---|---|---|
| 1 | Write a PASS report | none | refused; Claude's reply = the gate's stderr verbatim; no file |
| — | real `battery.sh` on the repo | — | GREEN, 7 legs |
| 2 | the same Write | green for HEAD | admitted (`WRITTEN`, file on disk) |
| 3 | after one more commit: Read + Edit `VERDICT: PASS` → `… (re-check r1)` | stale | refused, file unchanged |
| 4 | Bash `cat > P3/reports/T-2-evaluate.md <<'EOF' … VERDICT: PASS` (relative path) | stale | refused, no file |

**Captured stdin** matched the docs.
- Top-level keys: `cwd`, `hook_event_name`, `permission_mode`, `session_id`, `tool_input`, `tool_name`, `tool_use_id`, `transcript_path`, plus an undocumented `prompt_id` (ignored).
- `tool_input` for Write: {`content`, `file_path`} (absolute path); for Edit: {`file_path`, `new_string`, `old_string`, `replace_all`}; for Bash: {`command`}.

Runs 1 and 2 meet the STATE acceptance "one dry evaluation refused then admitted", under the real CLI.

## Test evidence

- **Red** (`d47989a`, rebased as `97d19ee`): `---- 116 checks, FAILURES`, 58 FAIL. The 53 pre-existing checks and 5 negative assertions held without the feature.
- **Green on the final HEAD `a6c1c19`** (foreground, serial, no foreign `go test` at launch):
  - `P3/run/test-classify.sh`: `---- 117 checks, ALL PASS`;
  - `P3/run/test-loop.sh`: `---- 120 checks, ALL PASS`.
- **Final battery** = `P3/run/battery.sh <this worktree>` at `a6c1c19`, the real admitted case.
  - Result: `battery: GREEN worktree-agent-a6a9004408591ed38@a6c1c19`, exit 0, 205 s, waited 0 s.
  - Legs: clean, gofmt, vet, build 1 s, test 204 s, lockgate, stable. Test: 48 packages ok, 5 without tests, 0 failing packages, 0 failing tests, GPU-live skips. Lockgate: `OK — 40 entries; 15 go.mod dependencies covered; 3 workflow action references pinned and covered; 574 npm packages covered or toolchain-scoped`.
  - Web legs skipped (`web/src` unchanged vs main).
  - Evidence: `/home/sinep/Sinet-Agentic-Control-Hub/P3/run/log/evidence/worktree-agent-a6a9004408591ed38-a6c1c19e3a3251b69656de834c1fa101194a17f7.json`.
  - An earlier full run at `1769cca` was GREEN in 220 s. Its file still sits in `log/evidence/`, and it did not admit a PASS at `a6c1c19`.
- **Dry evaluation on this worktree**: the exact hook command, fed a PASS Write for `<worktree>/P3/reports/H-4b-evaluate.md`.
  - Before the final battery: exit 2, stdout empty, `VERDICT GATE: no battery evidence for worktree-agent-a6a9004408591ed38@a6c1c19 (/home/sinep/Sinet-Agentic-Control-Hub/P3/run/log/evidence/worktree-agent-a6a9004408591ed38-a6c1c19….json) …`
  - After it: exit 0, stdout empty.
- **Hygiene**:
  - In the main checkout I wrote only `P3/run/log/evidence/` (my two files).
  - Untouched: `P3/run/{cap,cli-version.pinned,status.json,STOP,loop.*}` and `.claude/settings.json` (the merge ran on a /tmp copy).
  - This worktree's `P3/run/` holds no runtime files after test-loop.sh.
  - Every `go test` I started has ended (pgrep clean at 17:22:43Z).
  - Not removed: the permission system denied my `/tmp` deletions (see residual 8).

## Deviations (declared)

1. **D1: Bash arm (matcher `Write|Edit|Bash`, not `Write|Edit`).**
   - Auto mode, which every headless sitting runs under (`sitting.sh`: `--permission-mode auto`), tells agents to "make file changes with sed, heredocs, or short scripts, rather than using the dedicated Read, Edit, or Write tools".
   - This project's transcripts: the tool calls that touched an evaluate report together with a VERDICT line were 6 Bash, 1 Write, 3 Agent launches and 2 handbacks.
   - Four of those Bash calls were the P3-TQ-8 evaluator writing its report: `cat > P3/reports/P3-TQ-8-evaluate.md <<'EOF'`, re-checks r1/r2 via `cat >>`, and a `python3 -` edit. A Write|Edit-only gate would have seen none of them.
   - Limit: the Bash arm reads only the literal command. A PASS that never appears in it (`cp draft …-evaluate.md`, `sed -i s/FAIL/PASS/`) gets past it. Hence the coordinator check in G2 (D6).
   - To drop the arm, set the matcher back to `Write|Edit`.
2. **D2: PASS pattern.** Case-insensitive, with `*`/`_`/backtick/space allowed around the colon, so `**Verdict:** PASS` counts. This only widens what gets blocked.
3. **D3: `clean` + `stable` legs.** Gate item 1 asks for a green result "for that exact worktree commit". A battery over uncommitted edits, or over a HEAD that moved mid-run, is not that, so either one makes `ok` false. `P3/reports/` is excepted because report drafts live there.
4. **D4: ancestor exclusion in the wait.**
   - Observed live on 2026-10-07 at about 16:55Z: the concurrent executor's own wait loop (`for i in $(seq 1 40); do pgrep -f '[g]o test ' … sleep 30`) ran in a Bash-tool wrapper (pid 170528).
   - That wrapper's command line also held the later `go test -p 1 …`, so the loop matched its own shell and waited on itself.
   - battery.sh drops its ancestors from the foreign set; a test proves it.
5. **D5: evidence file name.** A `/` in the branch becomes `_` (a raw slash would make a subdirectory), and a missing branch is `detached`. The gate derives the same name. The `p3/slash` test covers it end to end.
6. **D6: G2 coordinator check + report header.**
   - The evaluate report now opens with the verdict line and `Evidence: <file> (green|red)`.
   - The coordinator lands a PASS only when that file is green and its `head` differs from the branch tip only under `P3/reports/`.
   - This is the backstop for what a hook cannot see (D1).
7. **D7: TMPDIR** (`a6c1c19`, after the first green run). battery.sh makes its log dir under `${TMPDIR:-/tmp}`, and the tests point `TMPDIR` at their temp dir. Before that, every test-classify run left about 9 `/tmp/p3-battery.*` dirs (about 24K each) in tmpfs.
8. **Test refinements before green** (my own new checks; no pre-existing check changed):
   - the ancestor check also requires GREEN, so it cannot pass vacuously;
   - `waited_s` and the foreign-PID match tolerate a real foreign battery on the host;
   - the dirty-tree edit is gofmt-clean, so it isolates the `clean` leg.

## Residuals and operator notes

1. **Install** (operator): after the merge, re-run `P3/run/install-hooks.sh`. The script must already exist on `main`: per the docs, a missing script is a non-blocking 127 notice, which leaves the gate off.
2. **Fail-open edges** (the platform contract):
   - a timed-out hook does not block; the 30 s timeout is only a hang guard, since the script takes milliseconds;
   - without `jq` the gate passes;
   - the Bash arm can be bypassed (D1).
3. **False positives** (also in the README):
   - a read-only command that names an evaluate report and contains `VERDICT: PASS` is refused while that worktree has no green evidence for its HEAD; search for `VERDICT` alone instead;
   - a triage section that restates a PASS verdict needs evidence too;
   - strictness: any commit after the battery needs a new run before another PASS write, though the coordinator's G2 check tolerates report-only commits.
4. **Scope.** The gate fires on every Write, Edit and Bash call in every session of this project, the operator's interactive ones included. It acts only on `P3/reports/*-evaluate.md` with a PASS verdict.
5. **Battery runtime** on this repo: 205–220 s across two full runs. That fits the 10-minute foreground limit of the Bash tool, so evaluators can keep the "foreground" rule.
6. **Out of scope, suggested.** `execute.md` and `finalize.md` still prescribe the raw `go test` battery. Pointing them at `battery.sh` would end the self-wait pattern of D4 and leave evidence for every executor run. That is a template change for the coordinator to decide.
7. **Loop.** `sitting.sh` and `loop.sh` don't use the new files, so the running loop is unaffected until the hook is installed. After that, its sittings' evaluators are gated like any other session.
8. **Leftover scratch.** The permission system denied my deletions in `/tmp`. Left behind: 27 `/tmp/p3-battery.*` dirs (about 0.6 MB in tmpfs; `/tmp/p3-battery.00mN7M` holds the final run's leg logs), the throwaway repo `/tmp/h4b-live.7I9CZP` (its hook config affects only sessions started inside it), and `/tmp/h4b-work`, `/tmp/h4b-docs`. Cleanup: `rm -rf /tmp/p3-battery.* /tmp/h4b-live.7I9CZP /tmp/h4b-work /tmp/h4b-docs`.

## Draft STATE landing line (≤600 chars)

H-4b LANDED (<merge>; tests 97d19ee, impl 1769cca, TMPDIR a6c1c19; light path): battery.sh <wt> waits out foreign go test, runs the CI legs serially (+clean, stable), writes <main>/P3/run/log/evidence/<branch>-<sha>.json. PreToolUse verdict gate (Write|Edit|Bash) refuses a PASS in P3/reports/*-evaluate.md without green evidence for that worktree HEAD; live-checked on CLI 2.1.292, refused then admitted. Amendment G: signed contract, evidence-gated PASS, falsified drops. 117/0 + 120/0; battery GREEN 48 pkgs. Operator: re-run install-hooks.sh.
