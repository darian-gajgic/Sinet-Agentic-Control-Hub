# P3/run — the continuous-run harness (H-1)

Design: `P3/design/continuous-run-harness-proposal-2026-09-22.md` (ratified 2026-09-22). Rules: runbook amendment F in `.claude/skills/p3-implementation/SKILL.md`. Per-sitting contract: `sitting-prompt.md` (appended to every headless sitting's system prompt).

| File | Role |
|---|---|
| `loop.sh` | the supervisor: `flock`, STOP/RESUME files, orphan reap, launch a sitting, classify, act (sleep / switch model / wait for a gate / breakers) |
| `sitting.sh` | one budgeted headless sitting: `claude -p "continue implementation" …` with the wall clock (`timeout --signal=INT`) and `--max-turns` rails; `--smoke` = one-turn launch-line check |
| `lib.sh` | shared library: `classify`, `parse_reset_epoch`, `notify`, `gate_answered`, `reap_orphans` |
| `test-classify.sh` + `fixtures/` | unit tests on canned results (limit text, 429, StopFailure status, status outcomes, crash, api_error, max-turns→CAPPED, reset parsing, gate marker, `--dry-run` decisions) |
| `hooks.proposed.json` + `install-hooks.sh` | the three project hooks (StopFailure / PreCompact / PermissionDenied) and their one-command installer |
| `sitting-prompt.md` | the sitting contract (budget, gate files, `status.json` last) |
| `log/` (gitignored) | `sitting-<ts>.jsonl` stream-json transcript, `.err`, `.meta`; `loop.log` |
| `status.json`, `STOP`, `RESUME`, `loop.lock`, `loop.pid` (gitignored) | runtime signals |

## Run it

```bash
P3/run/test-classify.sh                     # 30 checks, no API calls
P3/run/test-loop.sh                         # breaker tests with a stub claude: crash breaker, stall breaker, LIMIT resets the crash count
P3/run/sitting.sh --smoke                   # one 1-turn call: proves auth, flags, model string, log capture
tmux new -s p3loop 'P3/run/loop.sh --once'  # H-2(a): ONE supervised sitting, then exit
tmux new -s p3loop 'P3/run/loop.sh'         # H-2(b): unattended chain
touch P3/run/STOP                           # stop before the next sitting (Ctrl-C in tmux ends the running one cleanly)
touch P3/run/RESUME                         # end a gate/blocked wait without editing the gate file
```

**Hooks (operator installs once, one command):** `P3/run/install-hooks.sh` merges `hooks.proposed.json` into the project `.claude/settings.json` — `StopFailure` writes `status.json` = LIMIT on a usage/rate limit (the loop then never guesses from prose), `PreCompact` logs every compaction to `log/compactions.log` (a sitting that compacts is over budget — the empirical signal for the 3-packet cap), `PermissionDenied` logs what auto mode blocked to `log/denials.log`. The coordinator cannot install these itself (its edit of settings.json is denied as self-modification).

**After a limit:** the loop sleeps until the parsed reset (or backoff 15 min → 1 h), then sends a one-turn canary (`probe_model`) every 15 min until the limited model answers; the Fable→Opus switch reverses only after a Fable probe completes. Budget rails cut a sitting before its wind-down → `CAPPED` (treated as CONTINUE; the next sitting recovers from git). A non-limit API failure (auth wall, overload) → crash class with backoff; three in a row stop the loop. Note `--system-prompt-snapshot` defaults to on: editing `sitting-prompt.md` mid-sitting has no effect until the next sitting.

Environment knobs (defaults in `lib.sh`): `P3_MODEL_PRIMARY` (`claude-fable-5-1[1m]`), `P3_MODEL_FALLBACK` (`claude-opus-5`), `P3_SITTING_WALL` (`4h`), `P3_SITTING_MAX_TURNS` (`600`), `P3_SITTING_MAX_BUDGET_USD` (empty = no nominal-cost rail on the subscription lane), `P3_PROBE_INTERVAL` (`900`), `P3_PAUSE_MIN`/`P3_PAUSE_MAX` (`120`/`1800`, idle backoff), `P3_CRASH_PAUSE` (`300`), `P3_SWITCH_PAUSE` (`60`), `P3_NOTIFY_URL` (optional ntfy.sh topic for phone push).

## How a sitting is classified (never by exit code)

1. A usage-limit signature in the final `{"type":"result"}` line (`api_error_status` 429, or the limit regex in `result`/`error`) → `LIMIT:<family>:<reset epoch>`. Fable limit while on the primary model → the next sittings run on the fallback until the parsed reset (or 1 h); any other limit → sleep until the reset, else backoff 15 min → 1 h.
2. Else the sitting's own `status.json` (its last act): `CONTINUE` → next after 2 min; `GATE:<file>` → desktop notification, poll the file for `answered: yes|partial` every 10 min; `BLOCKED` → notification, wait for `RESUME`; `DONE` → exit 0.
3. Else `CRASH` (no signature): 3 consecutive → stop + notify. Stall breaker: 3 consecutive sittings without progress beyond STATE/HANDOFF bookkeeping commits → stop + notify; the pause between unproductive sittings grows 2 min → 10 min → 30 min. A LIMIT never counts as a crash or a stall.

Never run an interactive `continue implementation` session while the loop is up: touch `STOP` first, wait for the sitting to end (`tmux attach -t p3loop`), then work interactively.
