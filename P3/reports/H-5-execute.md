# H-5 — execute report (light path, process packet: harness)

**Commit:** `262ecf9` H-5: loop forwards Ctrl-C to the sitting's process group (job control), bounded grace, then TERM
**Branch:** `worktree-agent-a16008b0f7fa014a8` (not pushed)
**Files:** `P3/run/loop.sh`, `P3/run/sitting.sh`, `P3/run/lib.sh`, `P3/run/test-loop.sh`, `P3/run/README.md`. Not touched: `P3/STATE.md`, `P3/HANDOFF.md`, `P3/CONVENTIONS.md`, `.claude/settings.json`.

## Defect, re-verified

`P3/run/log/loop.log` (main checkout): sitting `20261008-114546` started 11:45:46Z; `signal: forwarding SIGINT to the sitting` at 12:01:56Z and 12:01:57Z; the sitting ended only at 12:02:45Z (`exit=0 subtype=success turns=28`), when SIGINT went to the `claude` pid by hand; `loop exiting on signal` the same second.

## Mechanism, measured on this host 2026-10-08

Dispositions and process groups, read from `/proc/<pid>/status` and `ps` for a sitting.sh-shaped script launched the way loop.sh launched it:

| process | before H-5 | with `set -m` at launch |
|---|---|---|
| sitting.sh | SigIgn `0x6` (INT + QUIT ignored), pgid = the loop's | INT not ignored, pgid = its own pid |
| `timeout` | catches INT, pgid = its own pid (`setpgid` at start) | same |
| `claude` (timeout's child) | SigIgn `0` for INT, pgid = timeout's | same |

So the brief's diagnosis is half right. The sitting.sh shell did ignore SIGINT and could not un-ignore it. But `timeout` installs its own SIGINT handler and moves itself and `claude` into a group of their own. That has three consequences:
- The terminal's Ctrl-C (sent to the loop's foreground group) never reached `claude`.
- `kill -INT -- -<sitting pgid>` alone would not reach it either. sitting.sh has to forward the signal to `timeout`.
- The wall-clock rail was never affected: `claude` starts with default SIGINT. The new `wall-rail` checks pass on the pre-fix scripts too.

The host's `timeout` is **uutils coreutils 0.8.0** (`/usr/lib/cargo/bin/coreutils/timeout`), not GNU. I probed it with a child and grandchild that log every INT/TERM they receive:
- INT to the timeout pid: one INT to the child and one to the grandchild, after about 0.1 s.
- A later TERM to the timeout pid: not forwarded. Only `--kill-after`'s SIGKILL ended the child.
- INT to timeout's whole group: the child and grandchild got it twice, once directly and once again from timeout.
- TERM to the group: delivered directly, and it ends a child that ignores INT within 0.1 s.
- Exit status: 124 after any stop timeout signalled. That held for a clean INT stop where the child exited 42, for the TERM path, and for the wall clock.

## Fix

- **loop.sh:**
  - The sitting is launched with `LAUNCHING=1; set -m; sitting.sh … < /dev/null … & CHILD=$!; set +m; LAUNCHING=0`. Job control applies to this one launch only, and stdin stays `/dev/null` as before.
  - `on_signal <INT|TERM>`: SIGINT to the sitting's group, then a poll for up to `P3_INT_GRACE` s (default 90). If the sitting is still running: SIGTERM to the group, up to 10 s more, then `exit 130`. A second Ctrl-C during the grace is logged and ignored.
  - A signal between the fork and `CHILD=$!` uses `$!`, but only inside the launch window.
  - Every sitting now logs `sitting.sh exit=<n> (informational only)`, which proves `wait` still returns the status under job control.
- **sitting.sh:**
  - `timeout … claude …` runs as a background job plus `wait`, because bash defers traps until a foreground command ends.
  - `fwd INT` sends to the timeout pid (one delivery). `fwd TERM` sends to timeout's group, falling back to its pid.
  - A forwarded signal interrupts `wait`, so the script waits again for timeout's own status.
  - A signal before the launch exits 130 and starts no sitting.
  - fwd logs to `loop.log` only, because stdout's last line must stay the transcript path. INT/TERM are ignored for the milliseconds of bookkeeping after timeout exits.
- **lib.sh:** `P3_INT_GRACE`, `proc_alive` (`ps` stat; zombie = dead).
- **README:** new "Stopping (H-5)" section covering STOP vs Ctrl-C, timings, log lines, the manual fallback `pkill -INT -f '^claude -p .*--name p3-sitting-'`, why the defect happened, and the uutils specifics. The Run-it lines, file table, knob list and check count are updated.

## Evidence

- `bash -n`: lib, loop, sitting, test-loop, test-classify, battery, install-hooks all OK.
- `test-loop.sh`: 147 checks, ALL PASS. The baseline before H-5 was 120, all pass; the brief's "43" is out of date. The 27 new checks:
  - `sigint-pid` (stub `sleep 60`, SIGINT to the loop pid): exit 130 after 1.09 s. Nothing left in the stub's or the sitting's group. sitting.sh and claude each have their own group. The stub's SIGINT is default. Trail `INT→group>fwd INT>ended 0`. Meta 124.
  - `sigint-ignored` (stub `trap '' INT; sleep 60`, `P3_INT_GRACE=3`): exit 130 after 4.23 s, at least the grace. Gone. Trail `INT→group>fwd INT>TERM→group>fwd TERM>ended 0`.
  - `ctrl-c-terminal` (SIGINT to the loop's whole group, the incident's shape; the stub ends its turn on INT): exit 130 after 1.07 s. Exactly one INT reached the stub, and the transcript has its result line.
  - `sigterm-loop`: SIGTERM to the loop takes the same path. Exit 130 after 1.18 s.
  - `wall-rail` (`P3_SITTING_WALL=2s`): CRASH:no result line, meta 124, fast, gone.
  - `drift-fail-waitrc` / `ci-green-waitrc`: `sitting.sh exit=3` / `exit=0` reach the loop.
- **Control:** the new test-loop.sh run against the pre-fix loop.sh/sitting.sh fails 20 of 147 checks: every stop-path check plus the two waitrc checks. The signalled loop had to be SIGKILLed after 10 s (exit 137). `timeout` and the stub were left running. `sitting:shared(554701/554677 vs loop 554677)` reproduces the incident's process layout.
- `test-classify.sh`: 117 checks, ALL PASS (unchanged).
- **End to end under a real terminal:** the real loop.sh (temp repo, stub claude/gh) ran as the command of a pane in a private tmux server (own socket), and a real `C-c` keypress was sent. The loop was the session leader and foreground group. sitting.sh and claude were in their own groups and not stopped, so there was no SIGTTIN/SIGTTOU from `set -m`. Result: pane dead with status 130 after 1.10 s, one INT delivered, a result line written, nothing left, no job-control noise in the pane.
- test-loop.sh was run with a scratch PATH shim (not committed) that makes `reap_orphans`' pgrep match nothing, so other agents' `go test` on the host could not be killed. No check covers reaping.

## Deviations from the brief

1. A group kill alone does not reach claude (`timeout` has its own group), so sitting.sh also traps and forwards. INT goes to the timeout pid and TERM to its group, both chosen from the uutils measurements.
2. The suspected wall-clock-rail gap does not exist (claude's SigIgn is 0). It is now covered by `wall-rail`.
3. Additions: `P3_INT_GRACE` knob (the tests use 3 s), the `sitting.sh exit=` log line, the launch-window race guards, and two cases beyond the two required (terminal-shaped group Ctrl-C, SIGTERM to the loop) plus the wall-rail check.

## Not changed (noted)

- A sitting stopped by Ctrl-C is not classified, recorded in the ledger or tagged; that was true before too. The next sitting recovers from git.
- During a model probe or CI read (each under its own `timeout`), a Ctrl-C takes effect only after it returns: at most 120 s for a probe, 60 s for a CI read.
- Loop lock fd 9 is inherited by the sitting and everything it starts. A sitting that outlives its loop keeps the lock, so a new loop refuses to start. That is protective, and pre-existing.
- claude still starts with SIGQUIT ignored (uutils timeout does not catch QUIT). This is pre-existing and harmless.
- The harness now depends on uutils timeout's forwarding behaviour. If coreutils changes, re-run test-loop.sh; the README says so.

## Operator

No loop is running (pid 463066 exited 12:02:45Z). The next `tmux new -s p3loop 'P3/run/loop.sh'` after the merge has H-5.

## Draft STATE landing line

H-5 landed (`262ecf9`): Ctrl-C/SIGTERM on loop.sh now stops the running sitting. Job control at launch gives the sitting its own process group. The loop sends SIGINT to it, and sitting.sh forwards to timeout (uutils 0.8.0: INT to its pid, so one delivery; TERM to its group). After 90 s (`P3_INT_GRACE`) the loop sends SIGTERM, then exits 130. test-loop 147/147 (+27; 20 fail on the old scripts), test-classify 117/117, real-tmux C-c: exit 130 in 1.1 s, one INT. The wall-clock rail was never affected.
