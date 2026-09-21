# Continuous-run harness: 2026 state of the art

Research date: **2026-09-22**. Host: Linux, Claude Code **2.1.278**, subscription (OAuth) login.
Every claim is tagged **VERIFIED** (primary source URL + fetched today, or empirically reproduced on this host),
**REPORTED** (secondary source), or **NOT FOUND**.

---

## 0. Executive answer to the design

The ratified design (external bash loop -> chain of fresh budgeted `claude -p` sittings -> classify from
JSON + git, never exit code -> sleep to reset -> gates as committed markdown -> circuit breakers) is
**confirmed by current primary sources on every load-bearing point**, with six refinements:

| # | Refinement | Why |
|---|---|---|
| R1 | **Add `--max-turns` and `--max-budget-usd` as hard rails.** Both exist and work in 2.1.278; `--max-turns` is *undocumented in `--help`* but accepted. | §1.2 |
| R2 | **Classify on `terminal_reason` + `is_error`, never `subtype`.** An auth/API failure returns `subtype:"success"` **with** `is_error:true`. Reproduced on this host. | §1.3, §4.2 |
| R3 | **Never pass `--bare`.** It hard-disables OAuth: the run returns `"Not logged in · Please run /login"`. Reproduced. | §1.5 |
| R4 | **Set `CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS`.** A `-p` run that spawns background subagents self-terminates after 10 min of idle waiting and **drops the partial result**. The four-stage packet pipeline is exactly this shape. | §1.6 |
| R5 | **Add a `StopFailure` hook with matchers `rate_limit` / `usage_limit`** so the sitting writes `status.json=LIMIT` itself instead of the loop guessing from prose. Undocumented-but-present hook event. | §4.3 |
| R6 | **Per-sitting budget of ~3 packets / 4 h is in the right band**, corroborated twice: Anthropic's V2 harness (3 h 50 min run, builder coherent >2 h) and `ralph-orchestrator`'s shipped `max_runtime_seconds: 14400`. Keep the 4 h wall clock. | §5.5, §2.7 |
| R7 | **A LIMIT must never increment the 3-crash breaker.** The best pure-bash implementation in the field zeroes the error counter explicitly on the rate-limit path. | §2.3 |
| R8 | **"A new commit exists" is not evidence of progress** — the coordinator commits `STATE.md` every sitting. Exclude `P3/STATE.md`, `P3/HANDOFF.md` and gate files from the no-progress check, or the breaker can never fire. | §2.5 |
| R9 | **Add a diminishing-returns breaker and an exponential idle backoff.** One surveyed harness measured 21 of 100 successful sessions shipping nothing, at \$0.399 each — "about \$38/day, forever, and nothing in the system could notice." | §2.4, §2.5 |
| R10 | **Never persist a guessed reset time**, and keep every status/monitor command strictly read-only. Three documented silent livelocks come from violating exactly these two rules. | §2.5 |

**A supervising interactive Claude session is still NOT viable** (R-critical): usage-limit auto-wait is
interactive-only and explicitly ends when the conversation is handed to a background session. §5.1.

---

## 1. Anthropic official guidance + Claude Code 2.1.x headless contract

### 1.1 The two canonical posts

**VERIFIED** — *Effective harnesses for long-running agents*, Justin Young, **2025-11-26**,
<https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents> (fetched 2026-09-22).
Two-part harness: an **initializer agent** (writes `init.sh`, `claude-progress.txt`, first git commit,
`feature_list.json`) and a **coding agent** run repeatedly with fresh context. Verbatim findings:

- "**Compaction isn't sufficient**" for multi-session coherence.
- Two named failure modes: **over-ambitious execution** (exhausts context mid-feature, leaves undocumented
  half-work) and **premature completion** (a later instance declares the project finished on partial progress).
- The feature list is **JSON on purpose**: "the model is less likely to inappropriately change or overwrite
  JSON files compared to Markdown files." Each entry: category, description, step-by-step test, `passes` bool.
- The Claude.ai-clone example had **over 200 features**.
- Fixed per-session sequence: `pwd` -> read git log + progress file -> pick highest-priority incomplete
  feature -> `init.sh` -> smoke test -> **implement exactly one feature** -> test -> commit -> update progress.
- Hard instruction language: "It is unacceptable to remove or edit tests because this could lead to missing or
  buggy functionality."
- Git is dual-purpose: progress record **and** recovery (revert bad changes).
- **No file-size or token budget is specified.** NOT FOUND.

**VERIFIED** — *Harness Design for Long-Running Application Development*, Prithvi Rajasekaran (Anthropic Labs),
**2026-03-24**, <https://www.anthropic.com/engineering/harness-design-long-running-apps> (fetched 2026-09-22).
This is the **2026 successor** and is the more decision-relevant of the two.

- **Three-agent (GAN-shaped) V1 harness**: planner (1-4 sentence prompt -> product spec) / generator /
  evaluator (drives the running app via **Playwright MCP**, grades against criteria).
- **Context reset > compaction**, verbatim: "context resets — clearing the context window entirely and starting
  a fresh agent, combined with a **structured handoff that carries the previous agent's state and the next
  steps** — addresses both these issues." On compaction: "it doesn't give the agent a clean slate, which means
  context anxiety can still persist."
- **"Context anxiety"** named as a failure mode: agents "begin wrapping up work prematurely as they approach
  what they believe is their context limit." Sonnet 4.5 exhibited it strongly enough that compaction alone failed.
- **Self-evaluation bias**: without a separate evaluator, "agents tend to respond by confidently praising the
  work — even when, to a human observer, the quality is obviously mediocre."
- **Sprint contracts**: generator and evaluator negotiate "what 'done' looked like for that chunk of work
  before any code was written." Sprint 3 of the example had **27 criteria**.
- **Agents communicate via files**: "one agent would write a file, another agent would read it and respond
  either within that file or with a new file that the previous agent would read in turn."
- **Measured runs**: retro game maker — solo agent 20 min / \$9 vs full harness **6 h / \$200** (16-feature spec,
  10 sprints). DAW on simplified V2 — **3 h 50 min / \$124.70** (planner 4.7 min, builds ~3.5 h), with the
  builder "running coherently for over two hours."
- **V2 removed the sprint construct** once Opus 4.6 shipped; grading moved to a single pass at the end.
  Governing principle: "every component in a harness encodes an assumption about what the model can't do on its
  own, and those assumptions are worth stress testing."
- **Outer loop script, handoff file format/size, append-vs-rewrite, explicit budget rails: NOT FOUND in the post.**

**VERIFIED** — <https://github.com/anthropics/cwc-long-running-agents> (fetched 2026-09-22, ~677 stars,
Apache-2.0, labelled "example ingredients, not a turnkey harness", demo from Code with Claude 2026).
Three shipped primitives, each directly mappable onto the Sinet design:

| Primitive | Files | What it does |
|---|---|---|
| **Default-FAIL contract** | `hooks/track-read.sh`, `hooks/verify-gate.sh`, `test-results.json`, `.claude/.evidence-reads` | A `PreToolUse` hook **blocks a write to the results file unless the agent first opened the evidence file**. Stops false "passing" claims. |
| **Fresh-context evaluator** | `agents/evaluator.md` | A subagent **with no Write/Edit tools**, run as `claude --agent evaluator -p "<review prompt>"`, returns `PASS` / `NEEDS_WORK`. |
| **Agent-maintained handoff** | `PROGRESS.md`, `hooks/commit-on-stop.sh` | Agent writes `PROGRESS.md` as it works and re-reads it on restart; a Stop hook auto-commits as a backstop. |

Operator controls shipped: **`hooks/kill-switch.sh`** (halts all tool calls while an `AGENT_STOP` file exists —
this is exactly the STOP-file breaker) and **`hooks/steer.sh`** (surfaces `STEER.md` once, mid-run redirect).
The README's own multi-session loop:

```bash
while grep -q '"passes": false' test-results.json; do
  claude -p "Read PROGRESS.md and build the next unfinished feature per CLAUDE.md."
  VERDICT=$(claude --agent evaluator -p "Review the most recent commit against its spec.")
  [ "$(echo "$VERDICT" | head -1)" = "PASS" ] || echo "$VERDICT" > NEXT_FINDINGS.md
done
```

Monitoring recipe it recommends: `watch -n 5 'git log --oneline -8'`, `watch -n 2 'tail -20 PROGRESS.md'`.
It also names advanced patterns it does **not** ship, sourced to the Mar-2026 post: "**Unattended loop**: outer
script caps session length and chains restarts", planner agent, sprint contracts, grading rubrics,
browser-verified evaluator, and "**re-simplify on upgrades**".

### 1.2 Flags — what actually exists in 2.1.278

All rows **VERIFIED empirically on this host today** unless marked.

| Flag | Present in 2.1.278? | Notes |
|---|---|---|
| `-p` / `--print` | yes | |
| `--output-format text\|json\|stream-json` | yes | print-mode only |
| `--json-schema <schema>` | yes | print-mode only; result lands in `structured_output`. Invalid schema -> `Error: --json-schema is not a valid JSON Schema`, exit 1. `format` is accepted but **not enforced** (annotation only). |
| **`--max-turns`** | **yes, but ABSENT from `claude -p --help`** | Verified: `--max-turns 1` on a multi-tool prompt returned `subtype:"error_max_turns"`, `terminal_reason:"max_turns"`, `is_error:true`, **exit 1**. Docs (<https://code.claude.com/docs/en/cli-reference>) do document it: "Limit the number of agentic turns (print mode only). Exits with an error when the limit is reached. No limit by default." **The `--help` omission is a docs/CLI drift — do not conclude from `--help` that it is gone.** |
| `--max-budget-usd <amount>` | yes | Verified: `--max-budget-usd 0.0001` returned `subtype:"error_max_budget_usd"`, `terminal_reason:"budget_exhausted"`, exit 1. Docs: subagent spend counts toward the cap; at the cap, spawning another subagent fails with `Budget limit reached` and **running background subagents are stopped** (needs >= v2.1.217). |
| `--permission-mode auto` | yes | choices: `acceptEdits`, `auto`, `bypassPermissions`, `manual`, `dontAsk`, `plan`. |
| `--permission-prompts none` | yes | requires >= v2.1.259. Removes `AskUserQuestion` entirely; anything that would prompt is denied and **Claude is told nobody can approve and not to retry**. Denials appear as `permission_denied` system messages and in `permission_denials` on the result. |
| `--append-system-prompt-file <path>` | yes | Verified: a bad path fails fast with `Error: Append system prompt file not found: …`. Good for a launch-time preflight. |
| `--no-session-persistence` | yes | print-mode only; sessions not saved to disk, **cannot be resumed**. |
| `--autocompact <auto\|tokens>` | yes | "auto, or 100k-1M tokens"; sets the auto-compact window for the session (>= v2.1.221). |
| `--bg` / `--background` | yes, but **rejected with `-p`** | Verified error text: "`--bg` and `--print` conflict: `--print` never starts the interactive session that `claude agents` attaches to, so the job would be unattachable." |
| `--fallback-model <a,b>` | yes | comma-separated; **re-tries the primary at the start of each user turn**. Useful for the Fable-limit -> Opus switch *within* a sitting. |
| `--session-id <uuid>` | yes | lets the loop pre-assign and log the sitting id. |
| `--effort low\|medium\|high\|xhigh\|max` | yes | |
| `--bare` | yes — **do not use**, see §1.5 | |
| `--restricted`, `--safe-mode`, `--worktree`, `--agents <json>`, `--settings`, `--tools`, `--disallowed-tools` | yes | |

### 1.3 The `result` message — real field set on 2.1.278

**VERIFIED** (captured from a live `--output-format json` run on this host). Top-level keys:

```
duration_api_ms  duration_ms  errors  fast_mode_disabled_reason  fast_mode_state
is_error  modelUsage  num_turns  permission_denials  queued_turn_count  result_index
session_id  stop_reason  subagent_stats  subtype  terminal_reason  total_cost_usd
type  usage  uuid
```

`subagent_stats` is notable for this design — it reports `spawned`, `completed`, `failed`,
`killed{parent,user,system}`, `refused{depth_limit,concurrency_limit,budget}`, `max_depth`. **The drain stage
can read `subagent_stats.refused.budget` to detect that stages were silently starved by the cost cap.**

Observed field values (this host, this version):

| Outcome | `subtype` | `is_error` | `terminal_reason` | exit |
|---|---|---|---|---|
| normal completion | `success` | `false` | `completed` | 0 |
| `--max-turns` hit | `error_max_turns` | `true` | `max_turns` | 1 |
| `--max-budget-usd` hit | `error_max_budget_usd` | `true` | `budget_exhausted` | 1 |
| **auth / API failure** | **`success`** | **`true`** | `api_error` | 1 |
| unknown CLI flag | — (no JSON) | — | — | 1 |

Full enum sets, extracted from the 2.1.278 binary (**VERIFIED**, `strings` on
`~/.npm-global/lib/node_modules/@anthropic-ai/claude-code/bin/claude.exe`):

- `subtype`: `success`, `error_max_turns`, `error_max_budget_usd`, `error_during_execution`,
  `error_max_structured_output_retries`.
  *(The TypeScript SDK docs say `error_max_budget`; the CLI actually emits **`error_max_budget_usd`**. Match the
  CLI string, not the docs string.)*
- `terminal_reason`: `completed`, `max_turns`, `budget_exhausted`, `api_error`, `error_during_execution`,
  `refusal`, `user_abort`, `cancelled`, `interrupted`, `permission_denied`, `max_tokens`.

### 1.4 Exit codes and signals

- **VERIFIED**: exit 0 on success; exit 1 on unknown flag, `--max-turns`, `--max-budget-usd`, auth failure.
- **VERIFIED (docs)** <https://code.claude.com/docs/en/headless>: "Claude Code exits with code 0 on success and
  a non-zero code when the run fails." **But** "When a failure happens inside the run, such as missing
  authentication, Claude Code prints the failure as the result on stdout" — i.e. an in-run failure is
  *content*, not a distinct code. **The design's rule "classify from JSON + git, never the exit code" is correct.**
- **VERIFIED (docs)**: SIGTERM -> **exit 143**, the in-flight turn is left unfinished with **no result recorded**,
  Bash process trees are killed, `SessionEnd` hooks run, and the unfinished turn resumes on `--resume`.
  To end the turn cleanly instead, send **SIGINT** first. *The loop's watchdog should SIGINT, wait, then SIGTERM.*
- **VERIFIED (docs)**: piped stdin is capped at **10 MB**; over the cap is a clean error + non-zero.
- **VERIFIED (docs)**: with `stream-json`, if the consumer reads slowly Claude Code drains queued output before
  exiting, capped at **30 s** (was ~2 s before v2.1.214 — which truncated large responses).

### 1.5 `--bare` is incompatible with subscription login — do not use it

**VERIFIED empirically.** `claude --bare -p … --model sonnet` returned:
`{"subtype":"success","is_error":true,"terminal_reason":"api_error","result":"Not logged in · Please run /login"}`.
Docs confirm the mechanism: "In bare mode, Claude Code never reads OAuth credentials or the system keychain…
set `ANTHROPIC_API_KEY`". The docs elsewhere call `--bare` "the recommended mode for scripted and SDK calls"
and say it "will become the default for `-p` in a future release" — **that is a live risk for this design**: if
`-p` defaults to bare, a subscription-auth harness breaks. Pin the CLI version or watch that release note.

Note `--bare` would also skip hooks, skills, subagents, plugins, auto-memory and `CLAUDE.md` — i.e. everything
the p3-implementation runbook depends on. It is doubly wrong here.

### 1.6 `CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS` — the silent-truncation trap

**VERIFIED (docs, <https://code.claude.com/docs/en/headless>, fetched today)**:

> If Claude starts a background subagent or workflow, `claude -p` instead **stays open until that work
> completes**… By default the wait ends after **10 minutes of continuous idle waiting**… At that point Claude
> Code **stops whatever is still running and drops its partial result**. To change the limit, set
> `CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS`, or set it to `0` to wait without one.

**VERIFIED (binary)**: the terminating message is literally
`…s; terminating. Set CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS=0 to wait indefinitely.`, and there is a telemetry
event `print_budget_halt`.

⚠️ The env-vars docs page gives a **contradictory** description of this variable ("Maximum milliseconds Claude
Code waits before printing a message about backgrounded subagents. Default `5000`"). The headless page and the
binary agree with each other; **trust the headless page**. Treat the env-vars page entry as stale.

Also **VERIFIED (docs)**: a plain background *Bash* task (dev server, watch build) is killed ~5 s after the
final result. And a `Monitor` watch is waited on "until it times out or the ten-minute cap ends the wait";
a Monitor watch defaults to timing out **5 min** after Claude starts it.

**Consequence for Sinet**: the coordinator sitting runs four subagent stages per packet. If any stage goes
quiet for 10 minutes (a long build, a deep read, a stalled eval), the sitting is killed and the stage's result
is **discarded, not reported**. Set `CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS` to the sitting wall-clock budget
(e.g. `14400000` for 4 h) or `0`, and let `--max-budget-usd` + the external timeout be the real rails.

### 1.7 Other env vars worth setting

**VERIFIED (docs, env-vars page)** unless noted.

| Var | Effect |
|---|---|
| `CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS` | §1.6. Set explicitly. |
| `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE` | "percentage (1-100) of the auto-compact window at which auto-compaction triggers". **VERIFIED present in binary.** |
| `CLAUDE_CODE_GOAL_CHECKIN_MINUTES` | controls `/goal` check-ins and automatic retries; `0` turns both off. (Docs text on this page says default 15; the `/goal` page says the background-work check-in interval is 30 min. Treat the `/goal` page as authoritative.) |
| `CLAUDE_CODE_SUBAGENT_MODEL` | model subagents use; unset = main session's model. **Directly useful**: run the sitting coordinator on one model and the four stages on another, without editing prompts. |
| `ANTHROPIC_DEFAULT_HAIKU_MODEL` | the small fast model; also used by `/goal` evaluation and background summarization. |
| `CLAUDE_CODE_DISABLE_CRON=1` | disables `/loop` and all scheduled triggers — a cheap way to stop a headless sitting scheduling anything. |
| `CLAUDE_CODE_SESSION_DIR` | where session files live (default `~/.claude/sessions`). |
| `CLAUDE_CODE_FORWARD_SUBAGENT_TEXT` | same as `--forward-subagent-text`. |
| `CLAUDE_AFK_TIMEOUT_MS` | idle time before an unanswered `AskUserQuestion` auto-continues. Moot under `--permission-prompts none` (the tool is removed). |

### 1.8 Hooks relevant to the harness

**VERIFIED (docs <https://code.claude.com/docs/en/hooks> + binary)**. Full event list in 2.1.278:
`PreToolUse`, `PostToolUse`, `Stop`, **`StopFailure`**, `SubagentStart`, `SubagentStop`, `SessionStart`,
`SessionEnd`, `PreCompact`, `PostCompact`, `Notification`, `UserPromptSubmit`, `PermissionRequest`,
`PermissionDenied`, `Setup`, `Elicitation`.

- **`Stop`** — fires at end of turn. Input: `last_assistant_message`, `stop_reason`
  (`end_turn`|`tool_use`|`max_tokens`), `stop_hook_active`, `permission_mode`. **Exit 2 blocks the stop and
  continues the conversation.** Can request a `modelSwitch`. Always check `stop_hook_active` to avoid recursion;
  there is a **block cap** that stops the loop if Claude answers without tool use for several turns.
- **`StopFailure`** — see §4.3. This is the one the design is missing.
- **`SubagentStop`** — input includes `agent_id`, `agent_type`, `last_assistant_message`, `stop_reason`,
  `stop_hook_active`; **exit 2 keeps the subagent running**. Useful to enforce that a stage wrote its report file.
- **`SessionEnd`** — `reason` ∈ `clear`|`resume`|`logout`|`prompt_input_exit`|`other`. Cannot block. **Runs on
  SIGTERM.** This is the right place to write a last-gasp `status.json` and flush `STATE.md`.
- **`PreCompact`** / **`PostCompact`** — `reason` ∈ `manual`|`auto`. Cannot block. A `PreCompact` firing in a
  sitting is a **direct signal that the sitting budget is too large** — log it.
- **`PermissionRequest`** / **`PermissionDenied`** — `PermissionDenied` receives the exact `tool_input`, which
  the on-screen denial notice omits. **This is the only programmatic way to capture what auto mode blocked.**
  Set `hookSpecificOutput.retry: true` to let the model retry.
- `Stop`/`SubagentStop` default hook timeouts are 120 s (binary: `StopFailure:120,SubagentStop:120`).

### 1.9 `/goal`, `/loop`, `ralph-loop`, background sessions

**`/goal`** — **VERIFIED** <https://code.claude.com/docs/en/goal> (fetched today). Highly relevant.

- It is "a wrapper around a **session-scoped prompt-based Stop hook**". After each turn the condition +
  conversation go to the **small fast model** (Haiku by default), which returns *not yet met* / *met* /
  *impossible*, each with a reason. Evaluation cost is "typically negligible".
- **The evaluator does not call tools.** "It doesn't run commands or read files independently, so write the
  condition as something Claude's own output can demonstrate." ⚠️ For Sinet this means a `/goal` condition
  cannot itself verify `status.json` or the git log — it only judges what the coordinator *said*.
- Condition limit: **4,000 characters**. One goal per session.
- **It works in `-p`**: "Setting a goal with `-p` runs the loop to completion in a single invocation:
  `claude -p "/goal CHANGELOG.md has an entry for every PR merged this week"`". With default text output
  nothing prints until the end — use `--output-format stream-json --verbose`.
- **To bound it, put the bound in the condition**: "include a turn or time clause in the condition, such as
  `or stop after 20 turns`."
- **Runaway rails**: if Claude answers the evaluator without tool use for several turns, Claude Code stops the
  loop, warns, and returns control with the goal still set.
- **Failure handling, directly relevant to limit detection**: four failures **clear** the goal — auth failure,
  exhausted credit balance, a context overflow auto-compaction couldn't clear, and an unavailable model. Any
  other failure keeps it set. **An API rate limit or a claude.ai usage limit *pauses* the goal** with a notice
  starting `Goal paused`. Retries are capped at three, then it pauses.
- **Requires hooks to be enabled** — unavailable under `disableAllHooks` or `allowManagedHooksOnly`.

**`/loop`** — **VERIFIED** <https://code.claude.com/docs/en/scheduled-tasks>. Session-scoped only
("Requires open session: Yes"), min interval 1 min, **recurring tasks auto-expire after 7 days**, no catch-up
for missed fires, a session holds at most 50 tasks, jitter up to 30 min on recurring fires. `loop.md` (project
`.claude/loop.md` beats `~/.claude/loop.md`) replaces the built-in maintenance prompt; **content beyond 25,000
bytes is truncated**. Not a substitute for an external loop — it cannot survive the session exiting.

**`ralph-loop`** — **REPORTED** (search results, 2026-09-22): shipped in
`anthropics/claude-plugins-official/plugins/ralph-loop`, with a `ralph-wiggum` variant in `anthropics/claude-code`.
Reported invocation `/ralph-loop "<prompt>" --max-iterations 10 --completion-promise "DONE"`; it "intercepts
session exits via a **stop hook** and automatically re-feeds your prompt while preserving all file
modifications and git history between iterations." **This is an in-session loop, not a fresh-context loop** —
it re-feeds the prompt into the *same* context, which is precisely what the Mar-2026 post says is insufficient.
It is not a replacement for the external chain. (See §2 for the verified repo detail.)

**Background sessions (`--bg`, `claude agents`)** — **VERIFIED** <https://code.claude.com/docs/en/agent-view>.
A **supervisor process** keeps them running with no terminal open; they survive closing the shell and machine
sleep. **`--bg` cannot be combined with `-p`** (verified on host, §1.2). Machine-readable state via
`claude agents --json --all` with `state` ∈ `working` | `blocked` | `done` | `failed` | `stopped`, plus
`claude logs <id>`, `claude stop|respawn|rm <id>`, `claude daemon status`. Each session draws subscription quota
independently. **Sessions finished and unattached for ~an hour are stopped by the supervisor** to free
resources (conversation preserved). Usage-limit behaviour in a background session is **NOT FOUND** in the docs —
and §5.1 shows auto-wait explicitly *ends* when a conversation is handed to a background session.

---

## 2. External agent loops, 2026 — surveyed projects

All repo facts below fetched live 2026-09-22 (GitHub API + raw.githubusercontent). Stars/dates as of that date.

> **Correction to one delegated finding.** The survey reported "there is NO `--max-turns` flag" from
> `claude --help | grep -c max-turns` → 0. **That inference is wrong.** I tested the flag directly on this host:
> `--max-turns 1` is accepted and returns `subtype:"error_max_turns"` / `terminal_reason:"max_turns"` / exit 1.
> It is **absent from `--help` but functional and documented in the CLI reference**. Use it. (§1.2)

### 2.1 Comparison table

| Project | Stars | Last push | Fresh ctx/iter | Budget rails | Limit handling | Circuit breakers | Human gate |
|---|---|---|---|---|---|---|---|
| `ghuntley/how-to-ralph-wiggum` | 1,758 | 2026-01-11 | **yes** (new process) | iteration cap only | **none** | **none** | none in-loop |
| `anthropics/.../ralph-loop` (= `ralph-wiggum`) | 36,595 / 147,464 (repos) | 2026-09-21 | **no — in-session Stop hook** | `--max-iterations` | none | corruption guards, fail-open | `/cancel-ralph` |
| `AnandChowdhary/continuous-claude` | 1,381 | 2026-08-24 | **yes, strictly** | runs / cost / duration / calls-per-hour | **best pure-bash sleep-to-reset** | error-threshold 3, hourly throttle | `--stall-threshold` |
| `thebasedcapital/nightcrawler` | 18 | 2026-02-23 | **yes** (episodes) | `--max-budget-usd` per episode + 8 stop conditions | none | PID lock + stale reap, diminishing returns | `STOP` file |
| `aditya-029/agent-build-harness` | 0 | 2026-09-19 | **no** — one orchestrator, **rotated at 120k tok** | cost/credit caps, usage floors, breaker | **3-bucket classification + parks to `resets_at`** | full breaker ladder | `harness chat` / `blocked.md` |
| `celesteanders/harness` | 183 | 2026-04-03 | **yes**, 2 sessions/task (gen + skeptical eval) | `MAX_RETRIES=2` | none | none | none |
| `frankbria/ralph-claude-code` | **9,641** | 2026-09-19 | configurable, **defaults to resumed** | calls/hr, per-call timeout | sleeps to top of hour (own limiter) | **CLOSED/HALF_OPEN/OPEN state machine** | permission-denied branch |
| `mikeyobrien/ralph-orchestrator` | 3,152 | 2026-09-10 | **yes** | `max_iterations 100`, `max_runtime_seconds 14400` (**4 h**), `idle_timeout 1800` | none | orphan-process reaping tests | **Telegram `human.interact`** |
| `li0nel/claude-loop` | 25 | 2025-11-03 (stale) | yes | iters 1000 / 12 h / \$100 | none | none | `--interactive` |

### 2.2 The two official Anthropic plugins are NOT external loops

**VERIFIED.** `anthropics/claude-plugins-official/plugins/ralph-loop` and
`anthropics/claude-code/plugins/ralph-wiggum` are **the same plugin**; `ralph-loop` (author "Anthropic") is the
newer maintained fork of `ralph-wiggum` (author Daisy Hollman). Both are **in-session `Stop` hooks** — the
README says outright: *"The loop happens **inside your current session** - you don't need external bash loops."*

- State: `.claude/ralph-loop.local.md`, YAML frontmatter `active / iteration / session_id / max_iterations /
  completion_promise / started_at` + the prompt body.
- Flags: `--max-iterations N` (0 = unlimited, the default) and `--completion-promise TEXT`.
- Completion = **exact literal** match of a `<promise>…</promise>` tag pulled out with `perl -0777`, compared
  with `=` not `==` (a shipped comment explains `==` in `[[ ]]` glob-matches and breaks on `*`, `?`, `[`).
- Continuation = `jq -n '{"decision":"block","reason":$prompt,"systemMessage":$msg}'` — it blocks the Stop and
  **re-injects the same prompt into the same context**. That is precisely what Anthropic's own Mar-2026 post
  says is insufficient. **Not a substitute for the fresh-sitting chain.**
- Every corruption guard **fails open** (`rm` the state file, `exit 0`) so the loop stops rather than wedges.
- **Anti-lying instruction, verbatim** — worth copying into the Sinet drain stage:
  > *"If a completion promise is set, you may ONLY output it when the statement is completely and unequivocally
  > TRUE. Do not output false promises to escape the loop, even if you think you're stuck… If the loop should
  > stop, the promise statement will become true naturally. Do not force it by lying."*
- The `ralph-wiggum` → `ralph-loop` diff is **three real bug fixes**, all relevant:
  1. **Cross-session Stop-hook interference** — the state file is project-scoped but the Stop hook fires in
     *every* session in that project; fixed by comparing `session_id` in the frontmatter against the hook input.
  2. **Wrong transcript parsing** — `tail -1` of assistant lines broke on a turn ending in a tool call
     ("Claude Code writes each content block as its own JSONL line, all with role=assistant"); fixed with
     `tail -n 100` + `jq -rs 'map(.message.content[]? | select(.type=="text") | .text) | last // ""'`.
  3. **`$?` clobbering under `set -e`** after a command substitution.
- README limitation, verbatim: *"`--completion-promise` uses exact string matching, so you cannot use it for
  multiple completion conditions (like "SUCCESS" vs "BLOCKED"). **Always rely on `--max-iterations` as your
  primary safety mechanism.**"* ⇒ **A single sentinel cannot express CONTINUE|GATE|BLOCKED|LIMIT|DONE. The
  Sinet `status.json` file is the right shape; a promise string is not.**

### 2.3 `continuous-claude` — the reference sleep-until-reset implementation

**VERIFIED**, 1,381★, MIT, v0.24.8 (2026-07-13), `continuous_claude.sh` = 3,537 lines.
Strictly fresh context: `grep -E '--continue|--resume|session[-_]id'` over the whole script → **zero hits**.

- State: `SHARED_TASK_NOTES.md` injected each iteration under `## CONTEXT FROM PREVIOUS ITERATION`, plus an
  optional `--knowledge-file` under `## DURABLE PROJECT KNOWLEDGE`; one branch + one PR per iteration.
- Rails (at least one of the first three is **required**): `--max-runs`, `--max-cost` (summed from each run's
  `total_cost_usd`), `--max-duration`, `--max-calls-per-hour` (token bucket over 3600 s),
  `--error-threshold 3`, `--completion-threshold 3`.
- **Limit detection** matches `rate limit | rate_limit_error | too many requests | 429 | overloaded_error |
  temporarily overloaded | limit reached`, then in order: parse Anthropic's own *"resets at 3pm
  (America/Los_Angeles)"* wording via `sed -nE`, else `retry-after`, else a `try again in N minutes` regex,
  else a **300 s default backoff**; with a roll-over guard `if wait<=0 then wait+=86400`.
- **The single most important line for Sinet**, verbatim from `maybe_sleep_for_rate_limit`:
  ```bash
  sleep "$wait_seconds"
  error_count=0          # ← rate limits do NOT count toward the crash counter
  ```
  ⇒ **A LIMIT must never increment the 3-crash breaker.** Check this in the Sinet loop explicitly.
- Human gate worth copying: `--stall-threshold N` writes `## Health pause - <ts>` + iteration + consecutive
  failures + the last 80 lines of diagnostics into the notes file, then **`exit 1` if the shell is
  non-interactive** ("so a human can intervene") rather than silently retrying.
- Honest documented failure mode: a failed iteration **closes the PR and discards the work**.

### 2.4 `nightcrawler` — episodic; the closest structural match to the Sinet design

**VERIFIED**, 18★, MIT, TypeScript, 2026-02-23 (one-day repo, low adoption, high design value).
README: *"A 60-minute episode with a clean context window produces better work than minute 300 of a continuous
session."* Exact spawn:

```ts
const args = ["-p","--dangerously-skip-permissions","--model",config.model,
              "--max-budget-usd",String(config.budget_per_episode_usd),
              "--output-format","text", prompt];
spawn(CLAUDE_BIN,args,{cwd:process.env.HOME, timeout:config.episode_timeout_seconds*1000,
  env:(()=>{const env={...process.env,TERM:"dumb"}; delete env.CLAUDECODE; return env;})()});
```

Note `delete env.CLAUDECODE` (stops the child believing it is nested) and `TERM:"dumb"`. **This is the only
surveyed harness using the native `--max-budget-usd` per-session cap.** Its state files are almost exactly the
Sinet set: `STATE.json`, `HANDOFF.md`, `PROGRESS.jsonl`, `tasks.json`, `LOCK`, `STOP`,
`checkpoints/episode-NNN.json`, `MISSION.md`.

- Config: `max_duration_hours 12`, `max_episodes 24`, `max_budget_usd 50`, `budget_per_episode_usd 5`,
  `episode_timeout_seconds 3600`, `error_threshold 10`, `diminishing_returns_lookback 3`,
  `cooldown_between_episodes_seconds 10`.
- **Eight termination conditions in order**: STOP file → agent-declared → episode limit → duration → budget →
  error threshold → fatal error → **diminishing returns** (avg tasks completed over last 3 episodes < 0.5).
  **The diminishing-returns arm is the one the Sinet design is missing** — it catches "the loop is running but
  achieving nothing", which "3 sittings without a new commit" only partially covers.
- **Lock file done correctly** (PID + liveness + stale reap):
  ```ts
  if (fileExists(LOCK_PATH)) {
    try { const pid = parseInt(readText(LOCK_PATH).trim()); process.kill(pid,0); return false; }
    catch { log("STALE_LOCK | Removing stale lockfile"); }
  }
  writeFileSync(LOCK_PATH,String(process.pid));
  ```
  plus `process.on("exit"|"SIGTERM"|"SIGINT", releaseLock)`.
  ⚠️ **PID-reuse caveat**: `kill(pid,0)` cannot distinguish a reused PID — there is no start-time/boot-id check.
  **`flock` (already in the Sinet design, and present on this host) is strictly better and avoids this whole
  class.** Keep `flock`; do not switch to a PID file.
- **Immutable task tracker** — `tasks.json` is generated from `MISSION.md` checkboxes and the prompt says:
  *"You may ONLY change the `passes` field from `false` to `true`. Do NOT delete, reorder, or rewrite tasks."*
  README: *"This prevents a known failure mode where autonomous agents rewrite their own success criteria to
  declare premature victory."*
- **Anti-hallucinated-progress**: the orchestrator injects `git diff --stat HEAD~1` and `git log --oneline -10`
  into every episode prompt — *"The next episode can cross-check what the previous handoff claims against what
  the git history shows. If they diverge, the agent knows to distrust the handoff."*
- Orchestrator re-reads `STATE.json` after each episode but **preserves its own tracking fields**
  (`episode_history`, `budget_spent_usd`, `errors`) so the agent cannot rewrite them.
- Supervision via macOS **launchd** with `KeepAlive{Crashed:true,SuccessfulExit:false}`, `ThrottleInterval 30`,
  `TimeOut 43200`. README: *"A shell loop dies when the terminal closes, the SSH session drops, or the machine
  sleeps."* **On this Linux host the equivalent is a systemd user unit with `Restart=on-failure` +
  `RestartSec=30` — worth doing rather than a bare `nohup` bash loop.**
- 🐛 **Verified real bug, and the best cautionary tale in the survey**: `state.budget_spent_usd` is **never
  incremented** — it is declared, initialised to 0, read by `shouldContinue`, and round-tripped
  (`previousBudget = state.budget_spent_usd` → `state.budget_spent_usd = previousBudget`). **The `budget_limit`
  circuit breaker is dead code and reported spend is always \$0.00.**
  ⇒ **Add a test that asserts every Sinet breaker counter actually moves.** A gate reading a counter nothing
  writes is indistinguishable from no gate.

### 2.5 `agent-build-harness` — the richest postmortems (read this one)

**VERIFIED**, 0★ but `src/harness.mjs` is 2,554 lines and the README says *"Every threshold carries a comment
recording what it was, what it is, and what measurement changed it. The interesting parts of this repo are
those comments."* That is accurate.

- **Dissents on fresh context**: keeps **one orchestrator conversation**, resumed each tick, and **rotates it
  past `rotateCtxTokens: 120000`**. Measured: `tick 1 new session $0.0277 / 2 turns` vs
  `tick 2 resumed $0.0063 / 1 turn`. ⇒ **The cheap-resume option exists, but only with a hard rotation
  threshold.** For Sinet, the sitting *is* the rotation, so this mostly confirms the design.
- Invocation: `-p <prompt>`, `--session-id <id>` (fresh) or `--resume <id>`, `--permission-mode auto`
  (default, **not** `--dangerously-skip-permissions`), `--fallback-model`, `--output-format stream-json
  --verbose --strict-mcp-config --forward-subagent-text`, `--include-hook-events`.
- **Three-bucket exit classification** (the field's best answer to Q4), verbatim table:
  | Bucket | Signature | Response |
  |---|---|---|
  | `ok` | no error | run again next interval |
  | `window_limit` | HTTP 429 **and** a rejected `rate_limit_event` | resume at that window's own `resetsAt`. **Not a fault** |
  | `transient` | `api_error`, **no HTTP status**, transport-shaped message | short retry (120 s) |
  | `error` | anything else | 30-min backoff |
  > *"That third bucket was missing for weeks. Five runs died to `Connection closed mid-response` and each
  > bought a full 30-minute park — about 2.5 hours of dead scheduler time for a fault a retry clears in
  > seconds. `isTransientFailure()` is deliberately narrow: it requires the absence of any HTTP status at all,
  > so a 401 or a 500 keeps the long backoff."*
- **Three documented livelocks in the reset logic** — each one is a trap the Sinet loop can fall into:
  1. **Mixing sources**: took utilisation from a live reading but kept the cache's expired `resets_at`.
     *"That composite described a moment that never existed — 100% used, reset time in the past — which the
     staleness branch read as 'the window rolled over' and cleared a doomed session on every tick."*
     ⇒ **Take each window's reading whole; never compose a reset time from two sources.**
  2. **Inventing a reset time**: *"The old code invented `now + 1800` here and returned it as authoritative.
     That is a verdict that renews itself: the harness parks, so no session runs, so no newer reading is ever
     written, so thirty minutes later the same figure re-parks it. Permanent, and silent."*
     ⇒ **Never persist a guessed reset time. Use a live canary probe instead (§4.3).**
  3. **Observer with a side effect**: *"`harness ui` polls twice a second, so leaving the dashboard open
     rewrote `last_probe` continuously and the scheduler's gate could never become true — the harness would
     silently stop building for as long as the dashboard was open."* ⇒ **Any Sinet status/monitor command must
     be strictly read-only.**
- **Lock-file self-deadlock, documented**: *"A lock held by THIS process is not a concurrent tick — it is our
  own from a previous pass. Only an exit handler used to clear it, which is correct for the one-shot path but
  **self-deadlocks the moment cmdTick is called in a loop: pass 2 reads our own live PID and skips forever.**"*
- **Breaker ladder** `healthy → steering → constrained → stopped`, **one rung per beat, de-escalating one rung
  per healthy beat, `hardStop:false` by default**. Arms: same tool call 6× in a row; 5 consecutive API errors;
  session USD cap; no distinct tool call for 10 min; output velocity ≥ 2500 tok/min over N beats. Three stated
  "easy to get wrong": **velocity must be the diff of two cumulative samples, never one sample read as an
  increment**; **compaction must be exempt** (*"their own auto-compact tripped their own breaker"*); and
  **enforcement below `stopped` is a message, not a kill**:
  `[supervisor] Something looks stuck: <reason>. If you are making progress, say so in one line and carry on.`
  Also: *"null, not 0 — 0 is a legitimate timestamp and using it as the 'unset' sentinel made the no-progress
  arm silently dead."*
- **The strongest empirical result in the survey — idle cost.** From 143 recorded runs: *"100 succeeded and 21
  of them did nothing at all: 21 idle sessions · \$8.38 · 87,632 output tokens spent to say 'there is nothing
  to do' — \$0.399 each, against \$2.351 for a session that shipped something… an empty backlog costs about
  \$38/day, forever, and nothing in the system could notice."* Fix: exponential idle backoff 15 m → 30 m → 1 h
  → 2 h → 4 h, **capped at 6 h**, reset by the first session that ships.
  **Work is measured from the git tree, not the agent's report**, with a `bookkeepingPaths` exclusion list
  (e.g. `logs/journal.jsonl`) — *"the loop requires every session to commit a journal line, so a HEAD check
  alone would never fire."* ⚠️ **This applies directly to Sinet: the coordinator commits `STATE.md` every
  sitting, so "a new commit exists" is NOT evidence of progress. Exclude `P3/STATE.md`, `P3/HANDOFF.md` and
  the gate files from the progress check.**
- **Metering honesty**: *"token counts come from the settled `result` event, not from streaming snapshots —
  summing those undercounted output by ~87× while double-counting cached input by 2.3×."* And *"When telemetry
  is unavailable, status says **unavailable**, never `$0`."*
- **Compaction defence**: *"a summary is a claim, not evidence; the non-negotiables are re-injected through
  `PreToolUse`, which actually reaches a running session"* — e.g. `"Never delete or weaken a failing test to go
  green."` (the "agents delete tests" failure mode).
- **Explicitly rejects the Stop-hook loop**: *"Its design doc presents this as the autonomous loop; its shipped
  code has it disabled, with the reason in a comment — it 'could spend credits while a user was answering a
  question'."*
- Stated scope limit: *"It should never be the thing that performs an outward-facing, irreversible action —
  sending, publishing, purchasing, submitting."*

### 2.6 `frankbria/ralph-claude-code` — the most-adopted external loop (9,641★)

- **Defaults to CONTINUED context** (`SESSION_CONTINUITY=true`, `SESSION_EXPIRY_HOURS=24`).
- **🔑 `--continue` hijacking (Issue #151), verbatim source comment:**
  > `# IMPORTANT: Use --resume with explicit session ID instead of --continue`
  > `# --continue resumes the "most recent session in current directory" which`
  > `# can hijack active Claude Code sessions.`
  ⇒ **If the Sinet loop ever resumes, it must use `--resume <explicit id>` (or pre-assign `--session-id`), never
  `--continue`** — otherwise a sitting can steal the operator's own interactive session in the same repo.
- Uses `--allowedTools` rather than `--dangerously-skip-permissions`, explicitly *"to preserve the permission
  denial circuit breaker (Issue #101)"*.
- **Circuit breaker is a real `CLOSED / HALF_OPEN / OPEN` state machine** persisted as JSON:
  `CB_NO_PROGRESS_THRESHOLD=3`, `CB_SAME_ERROR_THRESHOLD=5`, `CB_OUTPUT_DECLINE_THRESHOLD=70%`,
  `CB_PERMISSION_DENIAL_THRESHOLD=2`, `CB_COOLDOWN_MINUTES=30` (OPEN→HALF_OPEN), `CB_AUTO_RESET=false`
  (*"WARNING: Reduces circuit breaker safety for unattended operation"*). Handles a **corrupt state file**
  (`jq '.' || rm`) and **clock skew** (*"If elapsed_minutes < 0 (clock skew), stay OPEN safely"*).
- **🔑 Anti-self-destruction (Issue #149)**: `validate_ralph_integrity()` runs at the top of every loop and
  halts if `.ralph/`, `PROMPT.md`, `fix_plan.md`, `AGENT.md` or `.ralphrc` has gone missing, because
  *"broad `Bash(git *)` allows destructive commands"* — `git clean` / `git rm` / `git reset` had deleted the
  harness's own control files. ⇒ **Sinet equivalent: verify `P3/STATE.md`, `P3/HANDOFF.md`, the runbook SKILL,
  the gate directory and the loop scripts still exist at the top of every sitting, and abort if not.**
- **Exit detection is two-of-two plus a net**: completion indicators ≥ 2 **AND** an explicit `EXIT_SIGNAL=true`;
  plus *"🚨 SAFETY CIRCUIT BREAKER: Force exit after 5 consecutive EXIT_SIGNAL=true responses"* — added because
  the normal path could fail to fire while the agent kept insisting it was done.
- Known CLI bug it works around (Issue #243): compound Bash commands (pipes, `2>&1`, `;`, `&&`) may not match
  `Bash(cmd *)` permission patterns; it warns and continues rather than halting.
- **Lock/PID: NOT FOUND** — nothing stops two instances in one repo.

### 2.7 `celesteanders/harness` (183★) and `ralph-orchestrator` (3,152★)

- **celesteanders**: fresh context, and **two sessions per task — a generator and a separate skeptical
  evaluator** (*"agents grade their own work too generously. A skeptical evaluator in a fresh context provides
  honest feedback"*). Plans are **JSON not Markdown**: *"structured state survives context resets and is
  machine-parseable."* `MAX_RETRIES = 2` evaluator→generator cycles per task, then exit. Invocation uses
  `--allowedTools Bash,Read,Edit,Write,Glob,Grep`, no permission bypass. Its `docs/research/` holds **verbatim
  mirrors** of the Huntley, Anthropic (Nov-2025 and Mar-2026) and OpenAI harness papers.
- **ralph-orchestrator**: 11 backends; fresh context per iteration is a guardrail injected into every prompt
  (*"Fresh context each iteration - save learnings to memories for next time"*). Rails, verbatim:
  `max_iterations: 100`, **`max_runtime_seconds: 14400` (4 hours)**, `idle_timeout_secs: 1800`,
  `completion_promise: "LOOP_COMPLETE"`. **Independent corroboration of a 4-hour sitting budget.**
  Human gate is the most developed of any project: Telegram, *"agents emit `human.interact` events; the loop
  blocks until a response arrives or times out"*, with parallel-loop routing by reply-to or `@loop-id`.
  Confidence protocol worth stealing: *"score decisions 0-100. >80 proceed autonomously; 50-80 proceed +
  document; <50 choose safe default + document."* Has explicit **orphan-process cleanup tests**
  (`acp_process_cleanup.rs`, `omp_process_cleanup.rs`, `omp_timeout.rs`) — directly relevant to the operator's
  RW-14B runaway directive.

### 2.8 The exit-code misclassification anti-pattern, in the wild

**REPORTED** — two widely-copied scripts get this exactly wrong and should not be imitated:

- A dev.to "46-line script" (2026-08-23) classifies **any non-zero exit as a rate limit**, asserting
  *"Claude Code returns exit 0 on normal completion and non-zero on rate limits or abnormal termination"*, then
  blind-retries 20 × 5 min with no reset parsing.
- The workaround in `anthropics/claude-code` issue **#36320** (the "no rate-limit exit code" issue, closed as a
  duplicate with nothing shipped) is the same shape:
  `while true; do claude "$@"; [[ $? -eq 0 ]] && break; sleep "${CLAUDE_POLL_INTERVAL:-300}"; done`.

Both conflate a crash, a max-turns stop and a usage limit. **This is the failure the Sinet design already
avoids by classifying from the JSON + git.** Keep that rule prominent in the runbook.

Also **REPORTED**: `li0nel/claude-loop` opens with `set -e` while the `claude` call sits in an unguarded command
substitution — a usage limit's non-zero exit **kills the whole loop**. ⇒ **Do not run the Sinet loop under
`set -e` around the sitting invocation**; capture the status explicitly.

### 2.9 OpenAI Codex — loop pattern (⚠️ mirrored-primary, not direct-primary)

**ESCALATION.** `WebFetch` returns **HTTP 403** on `openai.com/index/harness-engineering/` and
`openai.com/index/unrolling-the-codex-agent-loop/`. Per the operator's blocked-tool rule this was **not**
silently substituted at full confidence: the content below comes from a **verbatim mirror** committed at
`celesteanders/harness/docs/research/260211_openai_harness_engineering_codex.md` (fetched via
raw.githubusercontent, includes byline and canonical link). **Evidence grade: mirrored-primary.** If direct
primary is required, enable Chrome browser control (`--chrome`) and re-fetch.

*"Harness engineering: leveraging Codex in an agent-first world"* — OpenAI, **2026-02-11**, Ryan Lopopolo.

- OpenAI **names the Ralph loop** as its PR-completion pattern: *"…iterate in a loop until all agent reviewers
  are satisfied (effectively this is a Ralph Wiggum Loop)."*
- **No external bash loop is published.** The loop is *inside* one long run: *"We regularly see single Codex
  runs work on a single task for upwards of six hours (often while the humans are sleeping)."* Completion is
  gated by **agent reviewers being satisfied**, not an iteration counter.
- **`AGENTS.md` is deliberately ~100 lines — a table of contents, not an encyclopedia**, pointing into a
  structured `docs/` system of record. Documented failure of the alternative, verbatim:
  *"We tried the 'one big AGENTS.md' approach. It failed in predictable ways: Context is a scarce resource…
  Too much guidance becomes non-guidance… **It rots instantly**… It's hard to verify."*
  ⇒ **Direct support for keeping `HANDOFF.md` short and making it an index into the spec and the packet briefs.**
- **Backpressure is mechanical, not prompt-based**: custom linters + structural tests enforce layering, and
  *"because the lints are custom, we write the error messages to inject remediation instructions into agent
  context."* A recurring **doc-gardening agent** opens fix-up PRs for stale docs.
- **Documented failure mode — entropy / "AI slop"**: *"Codex replicates patterns that already exist in the
  repository—even uneven or suboptimal ones. Over time, this inevitably leads to drift… Our team used to spend
  every Friday (20% of the week) cleaning up 'AI slop.' Unsurprisingly, that didn't scale."* Fix: golden
  principles encoded in-repo + background scans that open targeted refactoring PRs — *"like garbage collection."*
- Codex CLI surface (**VERIFIED from harness source, not OpenAI docs**): `codex exec --json --sandbox
  workspace-write -C <repo> <prompt>`, `codex exec resume --json <threadId> <prompt>`,
  `codex queue --thread <id> --message <msg>` for live steering, `codex exec --yolo`.
  **Codex exposes no account-window or USD-cost telemetry through `codex exec --json`** — cost gates don't
  exist on that provider. Claude Code's `total_cost_usd` is a real advantage here.
- **GitHub Copilot: NOT FOUND.** No GitHub-authored external-loop pattern exists. Copilot CLI appears only as
  one backend among eleven in `ralph-orchestrator`.

### 2.10 Fourteen cross-cutting lessons

1. **Fresh session per iteration is the majority position** (ghuntley, continuous-claude, nightcrawler,
   ralph-orchestrator, celesteanders). The one dissenter rotates at 120k tokens anyway.
2. **State on disk + git, never in the loop process.** Names in use: `IMPLEMENTATION_PLAN.md`, `AGENTS.md`,
   `SHARED_TASK_NOTES.md`, `HANDOFF.md`, `STATE.json`, `tasks.json`, `PROGRESS.jsonl`, `journal.jsonl`.
3. **Measure progress from the git tree, not the agent's report** — with an exclusion list for bookkeeping
   paths, or the check never fires.
4. **Make the success criteria immutable** (`passes: false→true` only) — the documented cure for premature done.
5. **Completion needs two independent signals**, never one, plus a hard iteration cap as the primary rail.
6. **Exit-code classification is the #1 correctness trap.** Classify from the stream/JSON, not `$?`.
   Never let a rate limit increment the crash counter.
7. **Lock files: PID + liveness + stale reaping, and never skip on your own PID.** `flock` avoids the class.
8. **Read-only observers must have no side effects** — a polling dashboard silently wedged a scheduler.
9. **Cap in-flight, not just pre-flight** — a breaker ladder that steers before it kills, with a compaction
   exemption and velocity measured as a diff of cumulative samples.
10. **Budget counters must actually be written** — add a test that asserts the counter moves.
11. **Doing nothing is its own cost class** — exponential idle backoff capped at ~6 h.
12. **Use `--max-budget-usd` per sitting** plus a harness-wide cap from a stamped baseline.
13. **Prefer `--resume <explicit id>` over `--continue`** — `--continue` hijacks the most recent session in the cwd.
14. **Protect the harness's own control files from the agent** — `git clean`/`rm`/`reset` have deleted them.

### 2.11 Delta against the ratified Sinet design

| Already in the design | Confirmed by | Note |
|---|---|---|
| fresh sitting per iteration | majority of the field + both Anthropic posts | strongest consensus point |
| classify from JSON + git, never exit code | agent-build-harness 3-bucket; the two anti-patterns in §2.8 | keep prominent |
| sleep to reset | continuous-claude (best impl) | never persist a *guessed* reset time |
| `flock` | nightcrawler's PID lock has the reuse hole `flock` avoids | keep `flock` |
| STOP file | nightcrawler `STOP`, cwc `AGENT_STOP` | enforce via `PreToolUse` so it halts **mid**-sitting |
| 3 crashes / 3 sittings without a commit | frankbria `CB_NO_PROGRESS_THRESHOLD=3`, continuous-claude `--error-threshold 3` | exclude bookkeeping paths from "new commit" |
| 4 h sitting | ralph-orchestrator `max_runtime_seconds: 14400`; Anthropic V2 run 3 h 50 m | independently corroborated twice |
| gate = committed file | cwc `PROGRESS.md`/`AGENT_STOP`; continuous-claude `--stall-threshold` | make the non-interactive path **exit 1**, not retry |

| **Missing / to add** | Source |
|---|---|
| **A LIMIT must not increment the crash counter** (explicit `error_count=0` on the limit path) | continuous-claude |
| **Diminishing-returns breaker** (avg packets landed over last N sittings < threshold) | nightcrawler |
| **Exponential idle backoff** when a sitting ships nothing, capped ~6 h | agent-build-harness (\$38/day finding) |
| **Exclude `STATE.md`/`HANDOFF.md`/gate files from the "new commit" progress check** | agent-build-harness `bookkeepingPaths` |
| **Harness self-integrity check** at the top of every sitting | frankbria Issue #149 |
| **Assert breaker counters actually increment** (a test) | nightcrawler's dead `budget_spent_usd` |
| **Inject `git diff --stat` + `git log --oneline -10` into every sitting prompt** so the sitting can distrust the handoff | nightcrawler |
| **Never compose a reset time from two sources; never persist a guessed one** | agent-build-harness livelocks 1 & 2 |
| **Any status/monitor command must be strictly read-only** | agent-build-harness livelock 3 |
| **Don't wrap the sitting call in `set -e`** | li0nel/claude-loop bug |
| **`--resume <explicit id>`, never `--continue`** | frankbria Issue #151 |
| **`delete env.CLAUDECODE` + `TERM=dumb`** when spawning | nightcrawler |
| **systemd user unit** (`Restart=on-failure`, `RestartSec=30`) rather than a bare `nohup` loop | nightcrawler's launchd rationale |
| **Immutable packet ledger** — a sitting may flip `status` forward only, never delete or rewrite packets | nightcrawler + Anthropic |
| **Anti-lying clause in the drain-stage prompt** | ralph-loop plugin |

### 2.12 Anthropic-primary loop facts (from §1.1)

**VERIFIED** from the Anthropic sources:

- **Fresh context per iteration is the consensus**, not `--continue`. Both Anthropic posts and the
  `cwc-long-running-agents` loop start a **new `claude -p`** each iteration.
- **State is carried in files + git**, never in the conversation: `feature_list.json` / `test-results.json`
  (machine-readable contract, JSON chosen because models overwrite Markdown more readily),
  `PROGRESS.md` / `claude-progress.txt` (human-readable narrative), `NEXT_FINDINGS.md` (evaluator output fed to
  the next iteration), and descriptive git commits as the audit trail.
- **The loop's termination condition is a file predicate**, not the agent's self-report:
  `while grep -q '"passes": false' test-results.json`.
- **Operator control is file-based**: `AGENT_STOP` (kill switch, enforced by a `PreToolUse` hook so it halts
  *within* a running sitting, not just between sittings) and `STEER.md` (surfaced once).
- **A `Stop` hook commits uncommitted work as a backstop** (`commit-on-stop.sh`) so a crashed sitting never
  leaves the tree dirty — this is a cheap, strong addition to the Sinet drain stage.
- **Documented failure modes (Anthropic, verified)**: premature completion; over-ambitious scoping exhausting
  context mid-feature; features marked done without end-to-end verification; agents editing or deleting tests
  to make things pass; self-evaluation bias (confident praise of mediocre work); context anxiety.
  A tooling-specific one: Claude "cannot see browser-native alert modals through Puppeteer MCP, resulting in
  buggier features reliant on such modals" — i.e. **an evaluator blind spot produces confidently wrong PASSes**.
- **PID reuse / exit-code misclassification / lock-file staleness: NOT FOUND** in any Anthropic primary. The
  exit-code hazard is documented instead as the `is_error`-vs-`subtype` contradiction (§4.2).

---

## 3. Handoff and memory files between fresh sessions

### 3.1 What the primaries say

- **VERIFIED**: "Compaction isn't sufficient" (Nov 2025) and "context resets… combined with a structured
  handoff that carries the previous agent's state and the next steps" (Mar 2026). **A reset + handoff beats a
  compact.** The Sinet split — `STATE.md` (live truth) + `HANDOFF.md` (orientation snapshot) + per-packet briefs
  — is the shape both posts converge on.
- **VERIFIED**: the machine-readable contract should be **JSON**, explicitly because "the model is less likely
  to inappropriately change or overwrite JSON files compared to Markdown files." ⚠️ **Sinet's `STATE.md` is
  Markdown and is the single source of truth.** Consider moving the *machine-checkable* part (packet ledger:
  id, phase, status, commit sha, gate id) into a `state.json` the loop can `jq`, leaving `STATE.md` as the
  narrative. This also lets the external loop classify without parsing prose.
- **VERIFIED**: a per-session contract of "what done looks like, agreed before any code was written"
  (sprint contracts, 27 criteria for one sprint) — the Sinet grounding brief already does this; the missing
  half is that the **evaluator should co-sign the criteria before the executor starts**, not after.
- **VERIFIED**: the **default-FAIL contract** — an agent may not write a PASS into the results file unless a
  `PreToolUse` hook has seen it read the evidence file first. This is the strongest available answer to
  "the evaluation stage rubber-stamped the executor."
- **Published size limits on orientation docs: NOT FOUND.** Neither Anthropic post gives one. The only hard
  number in the docs ecosystem is `loop.md`'s **25,000-byte truncation**, which is a reasonable empirical
  ceiling to borrow for a `HANDOFF.md`.

### 3.2 Context-rot evidence (the reason to keep them small)

**VERIFIED** — *Context Rot: How Increasing Input Tokens Impacts LLM Performance*, Kelly Hong, Anton Troynikov,
Jeff Huber (Chroma), **published 2025-07-14**, <https://www.trychroma.com/research/context-rot> (fetched today).
18 models incl. Claude Opus 4, Sonnet 4/3.7/3.5, Haiku 3.5, o3, GPT-4.1 family, Gemini 2.5 Pro/Flash, Qwen3.

- **Degradation is non-uniform and starts well before the window is full** — models do not process the
  10,000th token as reliably as the 100th.
- **Distractors compound**: a single topically-related distractor measurably lowers performance vs the
  needle-only baseline; four distractors degrade it further, and *which* distractor matters (non-uniform).
- **Needle-question similarity dominates**: low-similarity needle/question pairs (0.445-0.775 on Paul Graham
  essays, 0.521-0.829 on arXiv) degrade far more steeply with length than high-similarity pairs.
  *Implication: a `HANDOFF.md` written in the same vocabulary as the next sitting's task survives length better
  than one written in prose that only loosely matches.*
- **LongMemEval**: focused prompts (**~300 tokens**) scored significantly higher than full prompts
  (**~113k tokens**) on the same task, and **"Claude models demonstrated the largest gaps"** between the focused
  and full conditions, with conservative abstentions under ambiguity. **This is the single most decision-relevant
  number for the handoff-file design.**
- **Repeated-words**: 1,090 variations, 25→10,000 words; performance degraded consistently with length.
  Opus 4 refused the task 2.89% of the time, GPT-4.1 2.55%.
- **Shuffled haystacks beat logically coherent ones** across all 18 models — structural coherence did not help.

**REPORTED** (secondary, 2026): RULER cross-checks put effective context at **50-65% of advertised**; a 200K
window can show significant degradation by **50K**; LLM safety monitors miss dangerous actions **2-30x** more
often after 800k tokens of benign activity. Treat as directional, not primary.

**Verdict**: the ~9-packet context fill is not a budget problem, it is a *quality* problem — output quality was
already degrading long before the window filled. **Sittings should be capped well below the context limit, and
the orientation docs should be short and vocabulary-matched to the next task.**

---

## 4. Structured status/outcome contracts from a headless run

### 4.1 `--json-schema` vs a file the agent writes

**VERIFIED**: `--json-schema` is real, print-mode only, validated, and the payload arrives in
`structured_output`; an invalid schema is a hard fail (exit 1) since v2.1.205, and `format` keywords are
accepted but unenforced. There is an `error_max_structured_output_retries` subtype (binary), so schema
conformance is retried and can exhaust.

**Recommendation: use both, with the file as the source of truth.**

- The **file** (`status.json`, committed) survives a SIGTERM/SIGKILL, a `PRINT_BG_WAIT_CEILING` truncation, a
  crash mid-stream, and a lost stdout pipe. `--json-schema` does not — if the process dies, there is no result
  message at all.
- The **`--json-schema` result** is a cheap cross-check: if `structured_output.verdict` disagrees with
  `status.json`, the sitting died between writing one and the other — classify `BLOCKED`, not `CONTINUE`.
- Write `status.json` from a **`SessionEnd` hook** as well as from the drain stage, so a SIGTERM'd sitting still
  leaves a verdict (SessionEnd is the one hook that runs during SIGTERM shutdown).

### 4.2 Distinguishing "usage limit" vs "crash" vs "done"

**The contradiction is real and is the core hazard.** **VERIFIED on this host**: an API/auth failure returns
`subtype:"success"` **and** `is_error:true`. **REPORTED**, <https://github.com/anthropics/claude-code/issues/79500>
(filed 2026-07-20, v2.1.49, still **open** as of today): a rate-limited headless run returned
`{"subtype":"success","is_error":true,"result":"API Error: Rate limit reached","stop_reason":"stop_sequence",
"duration_api_ms":0,"total_cost_usd":0,"modelUsage":{}}` with **exit code 0**.

On 2.1.278 the exit code is now **1** for that class and `terminal_reason:"api_error"` is present, so the
situation is better than the issue describes — but `subtype` is still wrong. Classification rules for the loop:

```
result=$(jq -c . < out.json)          # last stream-json line, or the json result
subtype=$(jq -r .subtype        <<<"$result")
iserr=$(jq -r .is_error         <<<"$result")
treason=$(jq -r .terminal_reason<<<"$result")
text=$(jq -r '.result // ""'    <<<"$result")

# 1. hard budget stops -> CONTINUE (the sitting did its job and was capped)
[ "$treason" = max_turns ] || [ "$treason" = budget_exhausted ] && verdict=CONTINUE

# 2. usage / rate limit -> LIMIT   (NEVER trust subtype here)
[ "$treason" = api_error ] && grep -qiE 'usage limit|rate limit|limit reached' <<<"$text" && verdict=LIMIT

# 3. any other api_error, or is_error with no verdict file -> BLOCKED
[ "$iserr" = true ] && verdict=${verdict:-BLOCKED}

# 4. clean finish -> read status.json, cross-check against git
[ "$treason" = completed ] && verdict=$(jq -r .verdict status.json)
```

Additional discriminators available on 2.1.278 and worth using:
- `duration_api_ms == 0` and `total_cost_usd == 0` and `modelUsage == {}` ⇒ **rejected before inference** ⇒ a
  limit/auth wall, not a crash. (This is the issue's own recommended heuristic and it holds.)
- `permission_denials` non-empty ⇒ `--permission-prompts none` blocked something ⇒ likely `BLOCKED`, and the
  entries name the tool.
- `subagent_stats.refused.budget > 0` ⇒ stages were starved by `--max-budget-usd`.
- `errors` (top-level array) ⇒ inspect.
- **git state remains the independent check the design already specifies** — a new commit since the sitting
  started is the only evidence that is not self-reported.

### 4.3 Detecting the reset time — use the `StopFailure` hook

**VERIFIED (binary + docs)**: `StopFailure` fires **when a turn ends due to an API error**. Its hook input
schema in 2.1.278 is `{hook_event_name:"StopFailure", error, error_details?, last_assistant_message?}`.
Matcher values present in the binary: `rate_limit`, **`usage_limit`**, `overloaded`, `authentication_failed`,
`oauth_org_not_allowed`, `account_on_hold`, `billing_error`, `invalid_request`, `model_not_found`,
`server_error`, `max_output_tokens`, `cloud_credential_error`, `unknown`.
*(Note: the docs' matcher list omits `usage_limit`; the binary has it. Match on both `rate_limit` and
`usage_limit`.)* Default hook timeout 120 s. `StopFailure` **discards hook output** (no exit-2 blocking) — it is
observational, which is exactly what is wanted: it can write a file.

**This is the clean answer to "how does the sitting tell the loop it hit a limit":**

```json
{"hooks": {"StopFailure": [
  {"matcher": "rate_limit|usage_limit",
   "hooks": [{"type": "command",
              "command": "jq -c '{verdict:\"LIMIT\",error:.error,detail:.error_details,at:now}' > \"$CLAUDE_PROJECT_DIR/P3/harness/status.json\""}]}
]}}
```

**A machine-readable reset timestamp is NOT FOUND in the `-p` result JSON.** What exists:
- **VERIFIED (binary)**: the HTTP response headers carry it —
  `anthropic-ratelimit-unified-reset`, `anthropic-ratelimit-unified-overage-reset`,
  `anthropic-ratelimit-unified-slow-budget-reset`, plus `retry-after`. These are **not surfaced** on the
  result message.
- **VERIFIED (binary)**: internal state keys `continuableWallResetsAt`, `_autoContinueResetsAt`,
  `rateLimitGraceResetsAt`, `dismissedWallResetsAt` — these back the interactive auto-wait and are not exposed.
- **VERIFIED (docs)**: `system/api_retry` stream events carry `error: "rate_limit"`, `retry_delay_ms`,
  `error_status`, `attempt`, `max_retries`. **With `--output-format stream-json` the loop can read
  `retry_delay_ms` live** — this is the only first-party numeric hint available to a script.
- **REPORTED**: <https://github.com/anthropics/claude-code/issues/78476> is an **open feature request** to
  "expose subscription usage percentages headlessly (`stream-json` rate_limit_event or `claude usage --json`)".
  **VERIFIED**: there is no `claude usage` subcommand in 2.1.278 (`claude --help` lists `agents`, `attach`,
  `auth`, `auto-mode`, `doctor`, `gateway`, `import`, `install`, `logs`, `mcp`, `plugin`, `project`, `respawn`,
  `rm`, `setup-token`, `stop`, `ultrareview`, `update` — no `usage`).

**Practical recommendation**: parse the human reset time out of the `result` prose as a *hint*, but make the
loop's sleep robust without it — sleep to the next 5-hour boundary (or a configured retry cadence) and
re-probe with a ~\$0.01 `claude -p --max-turns 1 "ok"` canary; if the canary returns `terminal_reason:completed`,
resume. That is deterministic and does not depend on an unstable string.

---

## 5. What contradicts or refines the design

### 5.1 A supervising Claude session is still NOT viable — auto-wait is interactive-only

**VERIFIED** <https://code.claude.com/docs/en/interactive-mode> §"Wait for a usage limit to reset" (fetched today):

> Automatic continue is on by default **in interactive sessions** signed in with a claude.ai subscription.
> Requires Claude Code v2.1.234 or later.

And the wait **ends without continuing** when "**You exit Claude Code**: the wait doesn't restart when you
resume the session" and when "**The conversation changes hands**: …or hand the session to Claude Desktop, **a
background session**, or the cloud." Also: it re-arms at most **twice in a row**, then stops with
`Automatic continue stopped after repeated usage-limit hits`; and it does not start on its own for a reset
**more than 24 hours away** (a weekly limit), nor in Remote Control / agent-team teammate sessions.

⇒ **The external sleep-to-reset loop is required.** A `-p` sitting will not wait; it returns an error result and
exits. Even a background session is explicitly excluded. Do not redesign around a supervising session.

### 5.2 `--max-turns` exists — add it (this was an open question)

Answered in §1.2: it exists, works, is **missing from `--help`** but present in the docs and accepted by the
binary. It is the cheapest per-sitting runaway rail, and it produces an unambiguous
`terminal_reason:"max_turns"` that maps to `CONTINUE`.

### 5.3 There is a first-party "run until done" mode — `/goal` — but it does not replace the loop

`/goal` works in `-p` and "runs the loop to completion in a single invocation" (§1.9). It is genuinely useful
**inside** a sitting (e.g. `/goal three packets are landed and committed, or stop after 40 turns`). It does
**not** replace the external loop, for three verified reasons:

1. The evaluator **cannot call tools** — it cannot read `status.json` or `git log`, only what the coordinator
   said. Self-report is exactly what the design refuses to trust.
2. On a usage limit the goal **pauses**, it does not wait — and in `-p` there is nothing to un-pause it.
3. It runs in **one context window**, so it makes the context-fill problem worse, not better. That is the
   problem the design exists to solve.

Best use: `/goal` as the *in-sitting* stop condition with an explicit turn clause, external loop as the
*cross-sitting* driver. Note it requires hooks to be enabled.

### 5.4 Scheduled-task / cloud features do NOT replace a local loop

**VERIFIED** <https://code.claude.com/docs/en/routines> (research preview): routines run on **Anthropic-managed
cloud**, each run **clones a GitHub repo** — "**Access to local files: No (fresh clone)**". **Minimum interval
one hour**; there is a **daily cap on runs per account**; runs draw down normal subscription usage; Claude
pushes only to `claude/`-prefixed branches by default. A green run status "does **not** mean the task in your
prompt succeeded." ⇒ **Cannot drive a local build on this host.** Not a replacement.

**VERIFIED** <https://code.claude.com/docs/en/desktop-scheduled-tasks>: desktop tasks *do* run locally with
local file access, min interval 1 min, survive restarts, optional per-run git worktree. But they **require the
Claude Desktop app open and the computer awake**, they **stall indefinitely on a permission prompt in Manual
mode**, and a run is **skipped when "the previous run was still in progress."** A 4 h sitting would therefore
suppress every scheduled fire under it. ⇒ Usable at most as a *trigger* for the bash loop, not as the loop.

**`/loop`** requires an open session and expires after 7 days (§1.9). ⇒ Not a replacement.

**Conclusion: the external bash loop remains the only mechanism that satisfies "local repo + unattended +
multi-hour + survives session exit."**

### 5.5 Per-sitting budget: 3 packets / 4 h is defensible; the binding constraint is quality, not tokens

- **VERIFIED**: Anthropic's V2 harness ran **3 h 50 min / \$124.70** end to end with the builder "running
  coherently for over two hours"; the V1 three-agent run was **6 h / \$200**. A 4 h sitting sits inside the
  band Anthropic itself measured.
- **VERIFIED (Chroma)**: quality degrades continuously with length; the focused-vs-full gap (~300 tok vs ~113k
  tok) was **largest for Claude models**. So the 4 h cap should be enforced *and* the sitting should not be
  allowed to coast to the context limit.
- **Refinement**: make the cap **multi-dimensional and whichever-first** — `--max-turns N`,
  `--max-budget-usd X`, an external `timeout 4h`, and a packet counter. Log every `PreCompact` firing: **a
  sitting that auto-compacts has exceeded its useful budget**, and that is the empirical signal to lower N.
- **On raising the packet cap**: cost is the argument for more packets per sitting (each fresh sitting re-pays
  the orientation read), quality is the argument for fewer. The `PreCompact` counter settles it empirically
  after ~5 sittings — start at 3, raise to 4-5 only if no sitting compacts.

### 5.6 Smaller corrections to the design as briefed

| Design element | Status | Correction |
|---|---|---|
| `--permission-mode auto --permission-prompts none` | ✅ correct | `--permission-prompts none` needs >= v2.1.259 (have 2.1.278). Add `permissions.ask` rules for `Bash(git push *)` if a push checkpoint is wanted — ask rules are evaluated **before** the classifier and cannot be auto-approved. Add a `PermissionDenied` hook to capture `tool_input`; nothing else records it. |
| classify from JSON + git, never exit code | ✅ correct and load-bearing | See §4.2. Also never classify on `subtype`. |
| sleep until usage-limit reset | ✅ required | No reset timestamp in the result JSON; use the `StopFailure` hook + a canary probe (§4.3). |
| switch sitting model to Opus while Fable limit is active | ✅ supported | `--model opus` per sitting; **also** consider `--fallback-model` (retries the primary at the start of each user turn) and `CLAUDE_CODE_SUBAGENT_MODEL` to move only the stages. |
| gate = committed markdown with `## Answers` + `answered: yes` | ✅ matches SOTA | Anthropic's file-mediated agent handoff is the same pattern. Add the `AGENT_STOP` kill switch as a `PreToolUse` hook so a STOP halts **mid-sitting**, not just between sittings. |
| circuit breakers: 3 crashes, 3 sittings without a commit, STOP file, flock | ✅ correct | `flock` and `jq` both verified present on this host. Add a 4th: **`PreCompact` fired** (budget too big). Watchdog should SIGINT before SIGTERM (§1.4). |
| stage prompt templates on disk | ✅ correct | `--append-system-prompt-file` verified, and fails fast on a missing path — good preflight. Note `--system-prompt-snapshot` defaults to `on`: the system prompt is recorded on the conversation's first request and **reused verbatim on every later request and resume**, so editing a template mid-sitting has no effect. Pass `--system-prompt-snapshot off` while iterating on templates. |
| capped agent chat reports + full report to file | ✅ matches SOTA | Reinforce with a `SubagentStop` hook (exit 2) that refuses to let a stage finish until its report file exists. |
| `STATE.md` as single source of truth | ⚠️ refine | Anthropic explicitly prefers **JSON** for the machine-checked contract because models overwrite Markdown more readily. Split: `state.json` (packet ledger, `jq`-able by the loop) + `STATE.md` (narrative). |
| evaluation stage | ⚠️ refine | Adopt the **default-FAIL contract**: a `PreToolUse` hook that blocks writing a PASS until the evidence file was read. Have the evaluator **co-sign the packet's done-criteria before the executor starts** (sprint contract), not only after. |
| 4-stage pipeline with background subagents | ⚠️ **hazard** | `CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS` default kills the sitting after 10 min idle and **drops the partial result**. Set it explicitly. §1.6. |
| `--bare` | ❌ do not use | Breaks OAuth; also skips hooks/skills/CLAUDE.md. Watch for the announced change making `--bare` the `-p` default. §1.5. |

---

## 6. Source list

Primary (fetched 2026-09-22):
- <https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents> (2025-11-26, Justin Young)
- <https://www.anthropic.com/engineering/harness-design-long-running-apps> (2026-03-24, Prithvi Rajasekaran)
- <https://github.com/anthropics/cwc-long-running-agents> (Apache-2.0, ~677 stars)
- <https://code.claude.com/docs/en/headless>
- <https://code.claude.com/docs/en/cli-reference>
- <https://code.claude.com/docs/en/hooks>
- <https://code.claude.com/docs/en/goal>
- <https://code.claude.com/docs/en/scheduled-tasks>
- <https://code.claude.com/docs/en/routines>
- <https://code.claude.com/docs/en/desktop-scheduled-tasks>
- <https://code.claude.com/docs/en/interactive-mode>
- <https://code.claude.com/docs/en/agent-view>
- <https://code.claude.com/docs/en/auto-mode-config>
- <https://code.claude.com/docs/en/env-vars> (⚠️ its `CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS` entry contradicts
  the headless page and the binary; trust the headless page)
- <https://code.claude.com/docs/en/agent-sdk/typescript>
- <https://code.claude.com/docs/en/errors>
- <https://www.trychroma.com/research/context-rot> (2025-07-14, Hong / Troynikov / Huber)

External-loop projects (repo facts fetched live 2026-09-22 via GitHub API / raw.githubusercontent):
- <https://github.com/ghuntley/how-to-ralph-wiggum> (1,758*)
- <https://github.com/anthropics/claude-plugins-official/tree/main/plugins/ralph-loop>
- <https://github.com/anthropics/claude-code/tree/main/plugins/ralph-wiggum>
- <https://github.com/AnandChowdhary/continuous-claude> (1,381*, MIT, v0.24.8)
- <https://github.com/thebasedcapital/nightcrawler> (18*, MIT)
- <https://github.com/aditya-029/agent-build-harness> (0*, MIT - richest postmortems)
- <https://github.com/celesteanders/harness> (183*; holds verbatim mirrors of the Anthropic and OpenAI papers)
- <https://github.com/frankbria/ralph-claude-code> (9,641*)
- <https://github.com/mikeyobrien/ralph-orchestrator> (3,152*)
- <https://github.com/li0nel/claude-loop> (25*, stale)
- <https://www.humanlayer.dev/blog/brief-history-of-ralph> (2026-01-06)

Mirrored-primary (direct source 403-blocked - see the escalation in Sec 2.9):
- OpenAI, "Harness engineering: leveraging Codex in an agent-first world", 2026-02-11, Ryan Lopopolo -
  canonical <https://openai.com/index/harness-engineering/> (403 on WebFetch), read via the verbatim mirror at
  `celesteanders/harness/docs/research/260211_openai_harness_engineering_codex.md`.

Secondary (REPORTED):
- <https://github.com/anthropics/claude-code/issues/79500> (open, filed 2026-07-20)
- <https://github.com/anthropics/claude-code/issues/36320> (no rate-limit exit code; closed as duplicate)
- <https://github.com/anthropics/claude-code/issues/78476> (open feature request: headless usage exposure)
- <https://github.com/anthropics/claude-plugins-official/tree/main/plugins/ralph-loop>
- <https://github.com/anthropics/claude-code/tree/main/plugins/ralph-wiggum>

Empirical (reproduced on this host, Claude Code 2.1.278, 2026-09-22): all flag-acceptance tests, the
`--max-turns` / `--max-budget-usd` / auth-failure result payloads and exit codes, the result-message key set,
the `--bare` OAuth failure, the `--bg`+`-p` conflict message, and the `subtype` / `terminal_reason` enum
extraction from the CLI binary.
