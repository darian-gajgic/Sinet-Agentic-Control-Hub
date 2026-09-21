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
| R6 | **Per-sitting budget of ~3 packets / 4 h is in the right band**, corroborated by Anthropic's own V2 harness (3 h 50 min total run, builder coherent >2 h). Consider raising the *packet* cap and keeping the 4 h wall clock. | §5.4 |

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

## 2. External agent loops, 2026

*(Verified repo-level detail for the individual projects — Ralph/ralph-loop, continuous-claude, nightcrawler,
agent-build-harness, celesteanders/harness, Codex/Copilot equivalents — is in the companion section appended
below. The Anthropic-side loop material is in §1.1.)*

What is already **VERIFIED** from Anthropic primaries:

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

Secondary (REPORTED):
- <https://github.com/anthropics/claude-code/issues/79500> (open, filed 2026-07-20)
- <https://github.com/anthropics/claude-code/issues/78476> (open feature request: headless usage exposure)
- <https://github.com/anthropics/claude-plugins-official/tree/main/plugins/ralph-loop>
- <https://github.com/anthropics/claude-code/tree/main/plugins/ralph-wiggum>

Empirical (reproduced on this host, Claude Code 2.1.278, 2026-09-22): all flag-acceptance tests, the
`--max-turns` / `--max-budget-usd` / auth-failure result payloads and exit codes, the result-message key set,
the `--bare` OAuth failure, the `--bg`+`-p` conflict message, and the `subtype` / `terminal_reason` enum
extraction from the CLI binary.
