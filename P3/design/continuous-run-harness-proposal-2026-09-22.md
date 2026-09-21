# Continuous-run harness for P3 — proposal (2026-09-22)

Status: PROPOSAL, awaiting operator ratification. Nothing here is in force yet.
Origin: the 2026-09-16/17 coordinator sitting reached 96% context after nine landings; the operator asked for a way to run the build for days or weeks without restarting sessions by hand and without any one session's context filling.

## 1. What was verified (live, 2026-09-21/22; Claude Code 2.1.278 on this host)

Sources: code.claude.com/docs (headless, sessions, agent-view, sub-agents, scheduled-tasks, goal, hooks, interactive-mode, settings-reference), the official ralph-loop plugin on disk, Anthropic engineering posts, and community harnesses. Items marked REPORTED come from the research agent's sources and were not re-fetched by the coordinator.

| Fact | Consequence for us |
|---|---|
| Interactive sessions at a terminal wait out a claude.ai usage limit and continue on their own (v2.1.234+), but re-arm **at most twice in a row**, never for a reset more than 24 h away, and never for a model-family limit while running another family. | A Claude session cannot be the thing that keeps a multi-day loop alive; after the third limit hit it needs a human. |
| **Background sessions (`claude --bg`) and headless `-p` runs get no usage-limit wait at all.** | Whatever supervises them must detect the limit and sleep until the reset itself. |
| Headless `-p`: JSON result carries `is_error`, `subtype`, `result`, `session_id`, `terminal_reason`, `total_cost_usd`. Exit code is 0 for in-run failures (the failure is printed as the result). `--bare` skips subscription login (needs an API key), so the subscription lane must run without `--bare`. | Classify a sitting from the JSON and the repo state, never from the exit code. Tested on this host: `claude -p … --output-format json` authenticates and returns `is_error:false`. |
| `-p` stays open for background subagents, but only **10 minutes of idle waiting** by default (`CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS`, 0 = no ceiling). | Our stage agents run 20 to 60 min while the coordinator idles. The ceiling must be lifted or every sitting dies mid-packet. |
| `--permission-mode auto --permission-prompts none` (v2.1.259+) runs unattended: anything that would prompt is denied and `AskUserQuestion` is removed. | Gates must be files, not questions. That is the right contract anyway. |
| Subagents: own context window; nesting three deep by default (`CLAUDE_CODE_MAX_SUBAGENT_SPAWN_DEPTH`); `maxTurns` returns partial output; a subagent cut off by a usage limit reports the failure to its parent with its last output; subagents auto-compact (`CLAUDE_AUTOCOMPACT_PCT_OVERRIDE`). | The four-stage pipeline keeps working inside a bounded sitting. Compaction inside an agent is transparent and lossy, so the sitting budget, not compaction, is the control. |
| Background sessions are hosted by a supervisor daemon: they survive closing the terminal and sleep, stop at shutdown, and are scriptable (`claude agents --json`, `claude logs/stop/respawn/rm`). The docs name "another Claude session that supervises background work" as a supported reader of that JSON. | An attachable alternative to `-p` sittings if the operator wants to watch a sitting live. |
| `/goal` is a session-scoped prompt-based Stop hook (Haiku judges the condition after each turn). It pauses on a usage limit and resumes only if the session's auto-wait fires, and it clears on an un-compactable context overflow. `/loop` and cron tasks are session-scoped and expire after 7 days. | Useful inside a sitting, useless as the multi-day driver. |
| The official `ralph-loop` plugin is a Stop hook that re-feeds the same prompt **inside one session**; it does not reset context. | Exactly the failure we are trying to avoid. The original Ralph is an external `while` loop that starts a fresh `claude` each iteration. |
| Stop, SubagentStop, PreCompact, PostCompact, SessionEnd hooks exist. `autoCompactWindow` and `autoCompactEnabled` are settings. | Available rails; none of them replaces an external loop. |
| Anthropic's own long runs (REPORTED): "Effective harnesses for long-running agents" (2025-11-26) = fresh session per feature, `feature_list.json` + progress file + git, "compaction alone is insufficient"; the two-week C-compiler run (2026-02-05) = a bash `while true` spawning ~2,000 fresh sessions across 16 containers, no supervisor session; the scientific-computing run (2026-03-23) = SLURM + tmux. Community harnesses (nightcrawler, agent-build-harness, continuous-claude, frankbria/ralph) all: fresh window per episode, handoff files, external supervisor, limit detection with sleep-until-reset, circuit breakers, a STOP file. | Nobody runs one long session for days. The supervisor is always outside Claude. |
| `--fallback-model` never fires on rate limits (REPORTED, model-config doc). `-p` exits 0 on task failure (REPORTED, community). | Model fallback at a limit must be the loop's decision. |

## 2. Verdict on the nested-session idea

The operator proposed: big Fable session → bounded Fable implementation session → Opus stage agents, stop after a budget.

- **The middle layer is right and is the unit to build.** A *sitting* = one fresh coordinator session with a hard budget (packets, wall clock, turns) that lands its packets, rewrites HANDOFF/STATE, writes a status file, and exits. Sittings chain; nothing else carries state but disk and git. This is what every verified pattern does.
- **The top layer should not be a Claude session.** Three verified reasons: (1) only an interactive terminal session waits out usage limits, and only twice in a row, so a Claude supervisor needs a human after the third limit hit, which is the "check every few hours" the operator wants gone; (2) a supervisor session's context still grows with every absorbed report and its compaction is lossy, while a script never fills; (3) subagents die with their parent at a limit, so a sitting run as a subagent of a limited supervisor is lost mid-packet (recoverable from disk, but a wasted sitting each time). Anthropic's own two-week run used a bash loop, not a supervisor session.
- A Claude session still has a place **on demand**: the operator opens one to answer a gate in free text, to read the sittings' reports, or to inspect a past sitting (`claude --resume <session-id>`). It is not the thing that keeps the loop alive.

## 3. The design: sitting runner + loop

```
loop.sh (bash, systemd --user or tmux)          ← never fills, never hits a limit
  └─ sitting N: claude -p "continue implementation" …   ← fresh context, budgeted
        ├─ grounding agent   (fresh context)
        ├─ executor agent    (fresh context, opus)
        ├─ evaluation agent  (fresh context)
        └─ drain / landing   (coordinator inline, or finalizer agent)
  status.json → CONTINUE | GATE | BLOCKED | LIMIT | DONE
```

### 3.1 The sitting (one iteration)

- Entry point stays `continue implementation` (the existing skill); the runbook gains the sitting rules R1–R7 from the 2026-09-21 analysis (budget: ≤3 landings or 4 h, never a grounding past that; gates to disk first; delegated spot-checks; on-disk stage prompt templates; capped agent chat reports; agent-drafted landing text; HANDOFF rewritten not appended).
- Launch (subscription lane, no `--bare`):

```bash
CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS=0 \
timeout --signal=INT --kill-after=15m 5h \
claude -p "continue implementation" \
  --model claude-fable-5-1 \
  --permission-mode auto --permission-prompts none \
  --append-system-prompt-file P3/run/sitting-prompt.md \
  --output-format stream-json --verbose \
  > "P3/run/log/sitting-$(date +%F-%H%M).jsonl"
```

  `sitting-prompt.md` is the fixed per-sitting contract (Ralph's PROMPT.md): the budget, the status-file contract, "never ask, write a gate file", the wind-down sequence. SIGINT at the wall-clock cap ends the turn cleanly (documented); SIGTERM would leave it unfinished. `--max-turns` is in the CLI reference but not in this build's `--help`; the wall clock is the hard rail, turns are a secondary rail if the flag exists.
- The sitting ends by writing `P3/run/status.json`:

```json
{"outcome":"CONTINUE","landed":["P3-TQ-8"],"next":"P3-TQ-4c executor","gate":null,"ended":"2026-09-22T03:14:00Z"}
```

  `outcome` ∈ `CONTINUE` (more unblocked work), `GATE` (no unblocked work; `gate` names the file), `BLOCKED` (needs the operator for something else, e.g. a host change), `DONE` (queue empty). A sitting killed by a limit or crash leaves no status file; the loop classifies that from the JSON result and the repo.

### 3.2 The loop (supervisor)

Per iteration: refuse to run if `P3/run/STOP` exists; take a `flock`; reap orphans (`pgrep -af 'go test|vitest'`, the runbook's own audit step does the rest); launch the sitting; then classify:

| Signal | Action |
|---|---|
| result text matches `reached your .* limit` / `hit your .* limit` / `rate_limit` | If it is the **Fable** limit: next sitting on `--model claude-opus-5` (lossless per the standing rule), back to Fable once the window passed. Otherwise sleep until the reset time if it can be parsed, else backoff 15 min → 1 h. A weekly limit is just a long sleep. |
| `status.json` = CONTINUE | Next sitting after a short pause (2 min). |
| `status.json` = GATE / BLOCKED | Notify the operator (desktop `notify-send`, optionally a phone push), then poll every 10 min for the gate file's `answered: yes` marker (or a `P3/run/RESUME` touch); on answer, next sitting. Unblocked packets keep the loop working meanwhile; only an empty unblocked queue waits. |
| `status.json` = DONE | Notify, exit 0. |
| no status file and no limit | Crash class: count it; up to 3 consecutive → notify and stop (circuit breaker). |
| three consecutive sittings without a new commit on main | Stall breaker: notify and stop. |

Rules from the evidence that go in: a PID/flock guard (PID reuse skipped runs forever in one harness), classify by JSON + git state not by exit code, capped log sizes, a watched STOP file, and never `Restart=on-failure` on the exit code alone.

### 3.3 Where the loop lives

- First runs: `tmux new -s p3loop 'P3/run/loop.sh'` — visible, stoppable, dies at reboot (the operator restarts).
- Steady state: a `systemd --user` unit with `Restart=always` plus `loginctl enable-linger sinep` so it survives logout and comes back after a reboot. This is a host change and follows the proposed-then-approved rule. `KillUserProcesses` is commented (default no) in `/etc/systemd/logind.conf`, so tmux also survives logout today.

### 3.4 Gates without chat

A gate becomes a file first (`P3/gates/<name>.md`, committed before it is presented) with an `## Answers` section. The operator answers either by editing the file (any editor, from the phone over Tailscale/SSH) or by opening an interactive `claude` session and dictating in free text; that session records the answers into the file and sets `answered: yes`. The loop sees the marker and continues. This keeps the operator's preferred plain-language answering and removes the last reason a session had to stay open.

## 4. Rollout

1. Ratify R1–R7 and this design; write the runbook amendment and `P3/run/sitting-prompt.md`.
2. Build `P3/run/sitting.sh`, `P3/run/loop.sh`, the status/gate contracts, `notify-send`; unit-test the classifier on canned JSON (limit text, crash, CONTINUE, GATE).
3. Supervised dry run: one sitting in tmux with the operator watching; check the idle-ceiling env, the status file, and the log.
4. Unattended overnight run; review in the morning: packets landed per sitting, context per sitting (transcript size), limit handling.
5. Move to the systemd user unit if the operator approves linger.

## 5. Decisions for the operator

- D-a Sitting form: headless `-p` (recommended; deterministic, documented) or `--bg` background sessions (attachable live via `claude agents`, same limit behaviour).
- D-b Notification channel: desktop only, or also phone (ntfy.sh topic or the Claude app push).
- D-c Loop host: tmux now and systemd+linger later (recommended), or systemd from the start.
- D-d Budget numbers: 3 landings / 4 h wall clock / turns if available.
- D-e Model fallback: switch sittings to Opus 5 while the Fable limit is active (recommended per the standing lossless rule) or wait for Fable.
- D-f Gate answering by file edit in addition to chat (recommended).
