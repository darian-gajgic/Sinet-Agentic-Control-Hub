# P3/run — the continuous-run harness (H-1, hardened by H-3)

Design: `P3/design/continuous-run-harness-proposal-2026-09-22.md` (ratified 2026-09-22). Rules: runbook amendment F in `.claude/skills/p3-implementation/SKILL.md`. Per-sitting contract: `sitting-prompt.md` (appended to every headless sitting's system prompt). H-3 (deterministic limits, update guard, measured cap): `P3/reports/H-3-execute.md`.

| File | Role |
|---|---|
| `loop.sh` | the supervisor: `flock`, STOP/RESUME files, orphan reap, launch a sitting, classify, record the ledger row + apply the cap rule, archive LIMIT evidence, act (sleep / switch model / wait for a gate / breakers) |
| `sitting.sh` | one budgeted headless sitting: `claude -p "continue implementation" …` with the wall clock (`timeout --signal=INT`) and `--max-turns` rails, `DISABLE_AUTOUPDATER=1`, the CLI update guard, and the measured cap in the launch prompt; `--smoke` = one-turn launch-line check |
| `lib.sh` | shared library: `classify`, `parse_reset_epoch`, `limit_retry_json`/`retry_reset_epoch`/`limit_epoch`, `notify`, `gate_answered`, `reap_orphans`, `archive_limit`, `read_cap`/`record_sitting`/`update_cap` |
| `test-classify.sh` + `fixtures/` | unit tests on canned results (limit text, 429, StopFailure status, status outcomes, crash, api_error, max-turns→CAPPED, reset parsing, gate marker, `--dry-run` decisions, `system/api_retry` limits, the StopFailure hook command run in isolation + its matcher) |
| `test-loop.sh` | loop/sitting tests with a stub `claude` against temp state: breakers, LIMIT archive, update guard (drift → probe fail/pass), measured cap (down, up, broken streak, clamps, prompt injection) |
| `fixtures/observed/` (committed dir) | raw evidence of every LIMIT-classified sitting, `<sitting ts>/{result.json,status.json,api_retry.jsonl,class.txt}` — real observations to promote into tests |
| `cap` (committed) | the measured packet cap, default `3`, range 1..5; the loop rewrites it (shows as a working-tree change) |
| `cli-version.pinned` (committed) | the Claude Code version the harness last passed a probe on; the update guard rewrites it after a passed drift probe |
| `hooks.proposed.json` + `install-hooks.sh` | the three project hooks (StopFailure / PreCompact / PermissionDenied) and their one-command installer |
| `sitting-prompt.md` | the sitting contract (budget, gate files, `status.json` last; the launch prompt's cap overrides its default) |
| `log/` (gitignored) | `sitting-<ts>.jsonl` stream-json transcript, `.err`, `.meta`; `loop.log`; `sittings.tsv` (the cap ledger); `compactions.log`, `denials.log` (hooks) |
| `status.json`, `STOP`, `RESUME`, `loop.lock`, `loop.pid` (gitignored) | runtime signals |

## Run it

```bash
P3/run/test-classify.sh                     # 53 checks, no API calls
P3/run/test-loop.sh                         # 43 checks with a stub claude (breakers, LIMIT archive, update guard, measured cap)
P3/run/sitting.sh --smoke                   # one 1-turn call: proves auth, flags, model string, log capture
tmux new -s p3loop 'P3/run/loop.sh --once'  # H-2(a): ONE supervised sitting, then exit
tmux new -s p3loop 'P3/run/loop.sh'         # H-2(b): unattended chain
touch P3/run/STOP                           # stop before the next sitting (Ctrl-C in tmux ends the running one cleanly)
touch P3/run/RESUME                         # end a gate/blocked wait without editing the gate file
```

**Hooks (operator installs once, one command):** `P3/run/install-hooks.sh` merges `hooks.proposed.json` into the project `.claude/settings.json` — `StopFailure` (matcher `rate_limit|usage_limit|.*limit.*`: the two documented kinds explicitly plus any future error type containing `limit`) writes `status.json` = LIMIT (the loop then never guesses from prose), `PreCompact` logs every compaction to `log/compactions.log` (a sitting that compacts is over budget — the input of the measured cap), `PermissionDenied` logs what auto mode blocked to `log/denials.log`. The coordinator cannot install these itself (its edit of settings.json is denied as self-modification). After changing `hooks.proposed.json`, re-run the installer.

**After a limit:** the loop sleeps until the reset (or backoff 15 min → 1 h), then sends a one-turn canary (`probe_model`) every 15 min until the limited model answers; the Fable→Opus switch reverses only after a Fable probe completes. Every LIMIT-classified sitting's result line, `status.json` and `api_retry` events are copied to `fixtures/observed/<ts>/`. Budget rails cut a sitting before its wind-down → `CAPPED` (treated as CONTINUE; the next sitting recovers from git). A non-limit API failure (auth wall, overload) → crash class with backoff; three in a row stop the loop. Note `--system-prompt-snapshot` defaults to on: editing `sitting-prompt.md` mid-sitting has no effect until the next sitting.

**Update guard (H-3b):** every sitting runs with `DISABLE_AUTOUPDATER=1`. Before launch, `sitting.sh` compares `claude --version` with `cli-version.pinned`. On drift it runs the one-turn probe on the sitting's model: probe fails → `status.json` = `{"outcome":"BLOCKED","note":"CLI drift <old>→<new>: smoke failed"}`, exit 3 (the loop notifies and waits for `RESUME`); probe passes → log, one notification, the pin is updated, the sitting continues.

**Measured cap (H-3c):** after every sitting the loop appends one row to `log/sittings.tsv`: `ts model duration_s turns landed compactions transcript_bytes outcome cap` (landed = `status.json` `.landed` length; compactions = `compactions.log` entries inside the sitting's start/end window, interactive ones excluded; `cap` = the cap that sitting ran under, so the rule survives loop restarts and `--once`). Rule: any compaction → `cap` − 1 (min 1); the last five rows all ran at the current cap, landed it, and compacted zero times → `cap` + 1 (max 5). A preflight-blocked sitting (no transcript) writes no row. `sitting.sh` puts "land at most <cap> packets this sitting" in the launch prompt; the contract says that overrides its default of 3.

Environment knobs (defaults in `lib.sh`): `P3_MODEL_PRIMARY` (`claude-fable-5-1[1m]`), `P3_MODEL_FALLBACK` (`claude-opus-5`), `P3_SITTING_WALL` (`4h`), `P3_SITTING_MAX_TURNS` (`600`), `P3_SITTING_MAX_BUDGET_USD` (empty = no nominal-cost rail on the subscription lane), `P3_PROBE_INTERVAL` (`900`), `P3_PAUSE_MIN`/`P3_PAUSE_MAX` (`120`/`1800`, idle backoff), `P3_CRASH_PAUSE` (`300`), `P3_SWITCH_PAUSE` (`60`), `P3_NOTIFY_URL` (optional ntfy.sh topic for phone push). State-file paths (the tests point them at a temp dir): `P3_CAP_FILE` (`P3/run/cap`), `P3_SITTINGS_TSV` (`log/sittings.tsv`), `P3_COMPACTIONS_LOG` (`log/compactions.log`, must match the PreCompact hook's path in real runs), `P3_CLI_PIN` (`P3/run/cli-version.pinned`), `P3_OBSERVED_DIR` (`P3/run/fixtures/observed`).

## How a sitting is classified (never by exit code)

1. A usage-limit signature in the final `{"type":"result"}` line (`api_error_status` 429, or the limit regex in `result`/`error`) → `LIMIT:<family>:<reset epoch>`. The reset epoch is the prose time when one parses, else the last limit-kind `system/api_retry` event's time + `retry_delay_ms` (the event's `timestamp` if present, else the transcript's mtime), else 0. Fable limit while on the primary model → the next sittings run on the fallback until the reset (or 1 h); any other limit → sleep until the reset, else backoff 15 min → 1 h.
2. Else the sitting's own `status.json` (its last act, or the StopFailure hook's LIMIT record, with the same epoch fallback): `CONTINUE` → next after 2 min; `GATE:<file>` → desktop notification, poll the file for `answered: yes|partial` every 10 min; `BLOCKED` → notification, wait for `RESUME`; `DONE` → exit 0.
3. Else, if the sitting ended while still retrying a limit (its last assistant/`api_retry` event is an `api_retry` whose `error` kind contains `limit`) → `LIMIT` (covers a wall-clock kill mid-retry with no result line). A limit retry the CLI recovered from does not count.
4. Else `CAPPED` (budget rail) or `CRASH` (no signature): 3 consecutive crashes → stop + notify. Stall breaker: 3 consecutive sittings without progress beyond STATE/HANDOFF bookkeeping commits → stop + notify; the pause between unproductive sittings grows 2 min → 10 min → 30 min. A LIMIT never counts as a crash or a stall.

Never run an interactive `continue implementation` session while the loop is up: touch `STOP` first, wait for the sitting to end (`tmux attach -t p3loop`), then work interactively.
