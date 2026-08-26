# P3-LN-7 — the Kimi Code CLI substrate (`internal/adapters/kimicli`) + the `kimi-cli` lane

**Grounded 2026-08-26.** Carries **S00.9 amendment A12**.
Pipeline: exec opus · grounding+eval opus. Strictly sequential before LN-8.

> **Read this brief plus the spec sections it cites. Nothing else.** Prior briefs in `P3/briefs/`
> are EXPIRED and contain statements the code has since overturned. The binding truth is
> (a) `Spec/drafts/S*.md`, (b) the Go tree, (c) `P3/CONVENTIONS.md`. Where this brief and the tree
> disagree, **the tree wins and the disagreement is a finding to report**.

> ## ⛔ THIS PACKET IS GATED. DO NOT START BUILDING.
> The live capture that grounds this brief found a **primary-source Terms finding (F0) that
> disqualifies this lane under S03.6's own ratified taxonomy**, and it fires the A11 audit's
> **pre-registered would-change trigger** — against the EXISTING `kimi` lane as well as the new
> one. It also found that the flag surface the packet was queued against **does not exist** (F1).
> §1 carries both. The technical specification below is complete and ready, and it is
> **conditional on the operator's Gate-A ruling and the coordinator's F0 disposition**.

---

## §0 · The order, and what it actually asks for

The operator's order of **2026-08-26** (chat, authoritative, recorded in `P3/STATE.md`):

> "I have the subscription on Kimi K3 code, so I can use Kimi code via the Kimi code CLI or via
> API, since there is a quota included there … implement Kimi K3 not just via API but also via
> CLI, similar to claude code. When you're done, run the script again so I can input my API keys
> for GLM and include the Kimi K3 code CLI. And then we test what is performing best."

Read precisely, that is three things:

1. A **third D3 substrate** — Moonshot's first-party `kimi` CLI driven headless, the way the
   Anthropic lane drives `claude -p`. S03.2 pins "v0 ships exactly two substrates", so this is a
   **spec change**, executed as **S00.9 amendment A12** on the operator's explicit order, following
   A11's mechanics exactly (§11 carries the byte-exact draft).
2. A **fourth lane**, `kimi-cli`, as a lane **DOCUMENT** — not a second code path. It draws the
   **same Kimi Code membership pool** as the existing `kimi` lane (which rides opencode against
   `https://api.kimi.com/coding/v1`).
3. A **comparison**: after LN-8 wires the door, the operator lands comparable tasks on `kimi` and
   `kimi-cli` and reads the answer off the Lane column. Everything this packet builds has to make
   that reading honest — above all, **one pool must never appear as two**.

**This packet is (1) + (2) + A12.** The door script, the ceremony leg and the comparison walk are
**LN-8** (`P3/gates/lane-test-door.sh`, `lane-key-ceremony.sh` — NOT touched here).
`P3/gates/B6-clickthrough.sh` is never touched by anything in the LN campaign.

---

## §1 · ★ Findings that change the packet

Five findings from the 2026-08-26 capture. Each is quoted verbatim in §2 with its URL. F0 is
**blocking**; F1–F4 change requirements that were already written.

### F0 — BLOCKING · Moonshot explicitly prohibits non-interactive use of a Kimi Code subscription

Source: `https://www.kimi.com/code/docs/en/kimi-code/community-guidelines.html` (read 2026-08-26):

```
Scope of Use

Kimi Code subscriptions are for interactive use only. We're compatible with mainstream coding
tools and agent frameworks (Kimi CLI, VS Code, Claude Code, OpenCode, OpenClaw, etc.), so you can
call Kimi Code's AI capabilities from the tools you already use. For enterprise integrations,
commercial services, or other platform-related inquiries, visit the Kimi Platform to explore
partnership options.
```
```
Don't use Kimi Code for non-interactive automation

Kimi Code subscriptions are for personal interactive use only. Using it for non-interactive
purposes — such as scripted batch execution or data annotation pipelines — goes beyond normal use.
```

**Why this is blocking, in the platform's own ratified words.** S03.6 fixes a three-class ToS
taxonomy as spec vocabulary: *"class 3 explicit interactive-only/automation-banned →
**auto-disqualifying**"*. The phrase *"for interactive use only"* and the heading *"Don't use Kimi
Code for non-interactive automation"* are that class stated verbatim, on a first-party page.

**It is not only about the new lane.** The A11 audit
(`P3/measurements/2026-08-24-kimi-lane-gate-audit.md`) classified this membership **class 2** on
two full ToS reads, and recorded a pre-registered watch trigger, verbatim:

> "**Would-change trigger (for the §6 watchlist):** appearance in either ToS of an interactive-only
> / no-automation / no-non-interactive-batch clause of the kind Alibaba, Tencent and Xiaomi carry
> verbatim → **immediate re-audit and lane freeze before the next run consumes allowance.**"

That trigger has effectively **fired**. The text sits on a page the A11 audit never fetched —
`community-guidelines.html` is not in its 11-source list and is not one of the two ToS documents it
read — so this is a **gap in the audit's coverage**, not a change in Moonshot's position. The
already-commissioned `kimi` lane is affected identically: it is the same subscription.

**The honest reading, both directions, because the operator decides and not this brief.**

- *Disqualifying (the rule as written).* The words are present verbatim; S03.6 makes them
  auto-disqualifying; the pre-registered trigger names exactly this text and prescribes freeze +
  re-audit. Under the rule, both kimi lanes freeze pending the operator.
- *Not disqualifying (the argument on the other side).* The same page blesses agent frameworks —
  *"We're compatible with mainstream coding tools and agent frameworks (Kimi CLI, VS Code, Claude
  Code, OpenCode, OpenClaw, etc.)"* — which is the A3 "named sanctioned tool" limb the A11 audit
  passed on; it says heavy and multi-device use is fine (*"Users with normal patterns … are not
  typically flagged, regardless of volume"*, *"Switching between devices … or different coding
  tools … is a completely normal usage pattern"*); the banned examples are *"scripted batch
  execution or data annotation pipelines"*, which is a fair description of a data pipeline and a
  poor one of a person assigning a task to their own assistant; and the stated enforcement is
  **graduated** (*"limiting concurrent access"*), not termination.
- *What is NOT an argument.* That the CLI's own product docs bless `-p` for scripting and CI. They
  do — see §2(7) — but tool documentation is not the subscription's commercial terms, and the
  guidelines' own escape hatch points elsewhere ("For enterprise integrations, commercial
  services … visit the Kimi Platform").

**Disposition required — OQ-0.** This is a **Gate-A re-audit + an operator decision**, not an
executor decision and not a coordinator decision alone: it is the operator's subscription and the
operator's risk, and S03.6's rule is unambiguous enough that proceeding is a deliberate,
recorded act or it is nothing. It joins the LN gate batch **beside A12's veto**, and it must be
answered **before the operator's door run makes the first live call** — that is what the
pre-registered trigger says.

**One escape route was examined and does NOT work at v0.** The capture's own suggestion — swap to
a Kimi **Open Platform** pay-as-you-go key (`platform.kimi.com`, `api.moonshot.{ai,cn}/v1`) — is a
**different lane**: metered, not flat. G1 P7 keeps the **metered-exception list EMPTY at v0** with
DeepSeek as the sole pre-registered designated exception, and 3.10 forbids metered spending as a
default. Enabling it would need its own amendment and its own Gate A–C audit, and no Open Platform
ToS document was locatable in this capture. **Do not route around F0 this way without a decision.**

### F1 — the flag surface this packet was queued against does not exist

`P3/STATE.md`'s LN-7 row and its acceptance headline name `kimi --print`, `--afk`,
`--input-format`, `--final-message-only`, `--quiet` and a `--session-id`. **None of them exist.** A
`grep -rniE '\-\-afk|\-\-print|final-message|--quiet|dangerously|--max-turns|--session-id|--allowedTools|--permission-mode'`
over all 22 English doc pages returns **no hits**; those are Claude Code / Amp flags. The real
non-interactive flag is `-p` / `--prompt`; there is no `--afk` concept at all, and no
`--input-format` — a prompt is a single argv string, **not** a stream and (undocumented) not stdin.

**Consequence:** every requirement below is written against the REAL table captured in §2(2). The
STATE row's headline is corrected at landing. Nothing in this packet may cite a flag that is not in
§2(2).

### F2 — `--output-format stream-json` is undocumented, and carries no usage or cost numbers

The entire published description of the format is two prose sentences (§2(3)): JSONL on stdout,
OpenAI-chat-shaped Assistant/`tool_calls`/Tool messages, thinking excluded, progress on stderr.
**No `type` enumeration, no example object, no terminal/result message, no error shape, and — the
load-bearing absence — no token or cost fields anywhere.** A grep for `input_tokens|output_tokens|"usage"`
across the doc set hits exactly one line, and it is in the `kimi web` **server API**, not print mode.

**Why that is load-bearing.** S10.1 is normative: *"One usage row is written per paid model call,
in the same transaction as its D7 checkpoint — the checkpoint's usage block IS the ledger row."*
An adapter that cannot emit `adapters.Usage` cannot satisfy D7, cannot feed the S10 ledger, and
cannot support the operator's comparison (tokens are half of "what performed best").

**Resolution: a $0 spike, first, before any adapter code** — see **R0**. The CLI's `KIMI_MODEL_*`
channel accepts an arbitrary OpenAI-compatible base URL, so a **real** `kimi -p` turn can terminate
on a **loopback fake provider** and produce the **real** envelope at **$0** — this is exactly the
ratified tier-R shape §61 already uses on the opencode substrate ("a real `opencode serve` drives a
real turn whose model call terminates on a loopback fake OpenAI-compatible provider"). The spike
settles the envelope, the exit codes, whether usage is present, and whether `-p` runs on env
credentials alone. **The transport decision (OQ-A) is taken on the spike's result, not on
argument.**

### F3 — hooks are FAIL-OPEN, and print mode has no ask/defer at all

Verbatim (§2(5)): *"Even if the script errors or times out, the CLI **will not interrupt your
work** … this 'allow on failure' design is called fail-open"*, and *"Hooks are suitable for alerts
and lightweight interception, but **should not be used as the sole security barrier**."* And
(§2(2)): *"In `-p` mode, no human approval is requested — regular tool calls are handled under the
`auto` permission policy, while **static deny rules remain in effect**."*

**Consequence:** S03.4's exit-park primitive has **no analogue on this engine**. There is no
`defer` and no `permission.asked` round-trip: a hook may `allow` or `deny`, nothing else. So the
gate on this lane is **deny-with-reason, never park**, and the *enforcement* layer is the config's
static `[permission] deny` rules plus the `[tools]` allowlist — **not** hooks. See **R10**, which
makes the adapter refuse a gated-tool worker rather than silently auto-approve it.

### F4 — the pinned version silently self-upgrades, and print mode is unbounded by default

Auto-update is **on**: `tui.toml` `[upgrade].auto_install` defaults `true`, disabled only by
`KIMI_CODE_NO_AUTO_UPDATE`. A pinned 0.38.0 **will not stay 0.38.0** without it — and the package
has published **70 versions in ~3 months** (§2(1)). This is S03.3 rule 2's exact concern
(*"opencode's `POST /global/upgrade` self-upgrade endpoint MUST never be exposed or called"*).

And print mode does not end when the turn ends: `print_background_mode` defaults to `"steer"`,
`print_wait_ceiling_s` defaults to ~24.8 days, `bash_task_timeout_s` defaults to `0` (no timeout),
and `[subagent]`/`[swarm]` `timeout_ms` both default to `0`. **Left at defaults, one `kimi -p`
invocation is unbounded in both turns and wall-clock.** That is the RW-14B runaway class this
household has already paid for once (204 worlds, RAM full — memory `serial-tests-gpu-only`). See
**R12**.

---

## §2 · LIVE CAPTURE — 2026-08-26, verbatim, $0

*This section is the packet's only source of provider facts. Every load-bearing line is quoted from
a primary source read on 2026-08-26. **Zero provider API calls, zero logins, no `npm install`, the
`kimi` binary was never executed, no tarball downloaded** — npm registry metadata, GitHub raw docs
and the vendor's server-rendered doc pages only.*

**Capture note on method, because it changes how much the quotes are worth.** `WebFetch` proved
**unusable for verbatim capture** on this task — its summarizer caps quotes at ~125 characters and
truncates large JSON, and it once reported "no `docs/` directory" from a truncated GitHub tree that
in fact contains 20 English doc pages. Every quote below was taken from **raw bytes** (`curl` +
local read) at `raw.githubusercontent.com/MoonshotAI/kimi-code/main/...` and `www.kimi.com`, not
from a summarizer. An executor re-checking a line should do the same.

### (1) Package facts — the pin

Source: `npm view @moonshot-ai/kimi-code --json`

| Field | Value |
|---|---|
| **Pin (EXACT)** | **`0.38.0`** — `dist-tags.latest`, published **2026-08-20T13:13:41.488Z** |
| license | `MIT` |
| author | `Moonshot AI`; maintainer `yangruizeng <yangruizeng@moonshot.ai>` |
| bin | `{ "kimi": "dist/main.mjs" }` — single bin name **`kimi`** |
| engines | `node >=22.19.0` |
| type / size | ESM; `fileCount` 547; `unpackedSize` 57 552 817 (~57.5 MB) |
| repository | `git+https://github.com/MoonshotAI/kimi-code.git`, directory `apps/kimi-code` |
| provenance | published via **npm trusted publisher / GitHub Actions OIDC** with a SLSA provenance attestation (`_npmUser.name: "GitHub Actions"`, `trustedPublisher.id: "github"`) — first-party, mechanically attested |
| postinstall | `node scripts/postinstall.mjs` — relevant if the install is sandboxed |

**Cadence — the pin is fragile and the funeral plan must say so.** First publish `0.1.0` on
2026-05-21; **70 versions in ~3 months**; the last eight:

```
0.35.0: 2026-08-12T03:47:31.463Z   0.37.0: 2026-08-18T11:23:08.390Z
0.36.0: 2026-08-13T05:50:15.325Z   0.37.1: 2026-08-18T14:32:10.773Z
0.36.1: 2026-08-14T12:53:34.177Z   0.37.2: 2026-08-18T17:40:35.134Z
                                   0.38.0: 2026-08-20T13:13:41.488Z
```

LICENSE — `https://raw.githubusercontent.com/MoonshotAI/kimi-code/main/LICENSE`:
```
MIT License

Copyright (c) 2026 Moonshot AI
```

Install shape — `docs/en/guides/getting-started.md`:
```
Requires Node.js 22.19.0 or later:
...
npm install -g @moonshot-ai/kimi-code
```
The README also offers a non-npm installer (`curl -fsSL https://code.kimi.com/kimi-code/install.sh | bash`).
**Sinet uses the npm path**: it is the one with an exact registry pin, a license at a pinned ref
and a provenance attestation — a `curl | bash` installer is none of those (S16.2 pin discipline).

### (2) The real flag surface

Source: `raw.githubusercontent.com/MoonshotAI/kimi-code/main/docs/en/reference/kimi-command.md`

```
kimi [options]
kimi <subcommand> [options]
```
```
| Option | Short | Description |
| --- | --- | --- |
| `--version` | `-V` | Print the version number and exit |
| `--help` | `-h` | Show help information and exit |
| `--session [id]` | `-S` | Resume a session. With an ID, opens that session directly; without an ID, enters an interactive selector |
| `--continue` | `-c` | Continue the most recent session in the current working directory, without specifying an ID manually |
| `--model <model>` | `-m` | Specify a model alias for this launch. When omitted, new sessions use `default_model` from the config file |
| `--prompt <prompt>` | `-p` | Run a single prompt non-interactively and stream the Assistant output to stdout. This mode does not open the TUI |
| `--output-format <format>` | | Set the non-interactive output format; supports `text` and `stream-json`. Can only be used with `--prompt`; defaults to `text` |
| `--yolo` | `-y` | Auto-approve regular tool calls, skipping approval requests |
| `--auto` | | Start with auto permission mode; tool approvals are handled automatically and the Agent will not ask the user questions |
| `--plan` | | Start a new session in Plan mode — the AI will prioritize read-only tools for exploration and planning |
| `--skills-dir <dir>` | | Load Skills from the specified directory, replacing the automatically discovered user and project directories. Can be repeated |
| `--agent <name>` | | Start a new session with the specified agent as the main Agent. Cannot be combined with `--session`/`--continue` |
| `--agent-file <path>` | | Load a custom agent from a Markdown file for the new session and select it. Cannot be repeated or combined with `--agent`, `--session`, or `--continue` |
| `--add-dir <dir>` | | Add an extra workspace directory for this session. Relative paths resolve against the current working directory. Can be repeated |
```
```
`-r` / `--resume` is a hidden alias for `--session`; `--yes` and `--auto-approve` are hidden
aliases for `--yolo` and are not shown in help output.
```

Conflict rules, verbatim:
```
- `--continue` and `--session` are mutually exclusive — both mean "resume a previous session"
- `--yolo` and `--auto` are mutually exclusive — the two permission modes cannot be combined
- `--prompt` cannot be used with `--yolo`, `--auto`, or `--plan` — non-interactive mode uses `auto` permission by default
- `--output-format` can only be used together with `--prompt`
```
```
In `-p` mode, no human approval is requested — regular tool calls are handled under the `auto`
permission policy, while static deny rules remain in effect.
```

**Recorded doc self-contradiction** (do not resolve it silently — the §64 rule): `docs/en/configuration/overrides.md`
shows `kimi --yolo -p "Batch rename the following files..."`, which the reference above says is
rejected. **The reference wins; the spike (R0) settles it by execution.**

**No `--session-id`.** Sessions receive server-generated ULID-style ids (`01HZ…XYZ` in examples);
the control plane **cannot choose** one, unlike the Anthropic lane. Resume is `--session <id>` /
`-S`, or `--continue` / `-c`.

**No turn or budget flag.** Ceilings are config-only (`docs/en/configuration/config-files.md`):
```
| `print_max_turns` | `integer` | `100000` | In print mode (`kimi -p`) with `print_background_mode = "steer"`, the maximum number of new turns that may be triggered by background-task completions, to keep the steering loop bounded (the default is effectively unbounded) |
| `print_wait_ceiling_s` | `integer` | `2147483` | In print mode (`kimi -p`), the wall-clock ceiling (seconds) for the wait/steer loop when `print_background_mode` is `"drain"` or `"steer"` (the default is ~24.8 days — effectively unbounded). |
| `max_steps_per_turn` | `integer` | — | Maximum steps per turn; unset or `0` means unlimited |
| `max_attempts_per_step` | `integer` | `10` | Maximum total attempts for a failing step, including the initial attempt |
```
`max_steps_per_turn` is env-overridable via `KIMI_LOOP_MAX_STEPS_PER_TURN`. **There is no
cost/budget ceiling flag or setting anywhere in the doc set** — so this lane has **no engine belt
to order against**, and `⚙ adapter.engine_ceiling_backstop_mult` has no consumer here (§5).

Unbounded-by-default print mode, verbatim:
```
In print mode (`kimi -p "<prompt>"`), Kimi Code stays alive after the main agent's turn as long as
background tasks are still pending: each completion is fed back to the main agent as a synthetic
user message, steering it into a new turn (`print_background_mode = "steer"` by default), and the
run exits once a turn ends with nothing pending. ... Background work is never killed by a
wall-clock cap in print mode either: background `Bash` tasks default to no timeout
(`bash_task_timeout_s = 0`), and subagents run without a timeout (`[subagent] timeout_ms` and
`[swarm] timeout_ms` both default to `0` unless explicitly set), so only the model itself stops a
task.
```

Permission and tool control — config, not flags:
```
| `decision` | `string` | Yes | Action on match: `allow` (permit immediately), `deny` (reject immediately), `ask` (prompt each time) |
| `scope` | `string` | No | Rule scope: `turn-override`, `session-runtime`, `project`, `user`; defaults to `user` |
| `pattern` | `string` | Yes | Match pattern in the form `ToolName` or `ToolName(arg-pattern)`, e.g. `Read` or `Bash(rm -rf*)` |
```
```
| `enabled` | `array<string>` | — | Global allowlist: when non-empty, only the listed tools are available; omitting the field or setting an empty array imposes no constraint |
| `disabled` | `array<string>` | — | Global denylist, applied after `enabled` |
```
```
`tools` is the global tool switch: it applies to every agent in all sessions and intersects with
each agent's own `tools` / `disallowedTools` policy.
```

**Native spawn is disablable — and the wildcard is a trap.** The subagent tools are named `Agent`
and `AgentSwarm` (`docs/en/customization/agents.md`: *"the `Agent` tool lists only the sub-agent
types the caller may delegate to, and both `Agent` and `AgentSwarm` re-check the allowlist before
dispatching"*), so `disabled = ["Agent", "AgentSwarm"]` removes them. But:
```
a wildcard outside an `mcp__` pattern (`enabled = ["*"]` disables every tool, `disabled = ["*"]`
disables none)
```
Default recursion is already bounded: *"Built-in sub-agents cannot dispatch further sub-agents."*

**Skills / slash-commands:** no disable flag exists. `--skills-dir <dir>` **replaces** discovery
(point it at an empty platform-owned dir), and built-ins go off via `builtin_product_skills = false`
or `KIMI_CODE_BUILTIN_PRODUCT_SKILLS=0`.

**System prompt / agent:** `--agent <name>`, `--agent-file <path>` (both documented as working in
print mode: *"`--agent` and `--agent-file` select which agent drives a new session, in both print
mode (`kimi -p`) and the interactive TUI"*), plus a persistent full override:
```
To override the main agent's system prompt permanently — without passing `--agent` or
`--agent-file` on every launch — write a `$KIMI_CODE_HOME/SYSTEM.md` file (default:
`~/.kimi-code/SYSTEM.md`; it moves with `KIMI_CODE_HOME`). While the file exists and is non-empty,
it replaces the built-in default main agent's system prompt in full
```

**MCP:** no flags. File-configured at `$KIMI_CODE_HOME/mcp.json` + project `.kimi-code/mcp.json`;
disable per server via `[tools] disabled = ["mcp__<server>__*"]`. Timeouts
`KIMI_MCP_STARTUP_TIMEOUT_MS`, `KIMI_MCP_TOOL_TIMEOUT_MS`.

**Subcommands:** `login`, `acp`, `web`, `doctor`, `export`, `migrate`, `upgrade`, `provider`, `vis`.
`kimi acp` (`docs/en/reference/kimi-acp.md`):
```
Switch Kimi Code CLI to ACP (Agent Client Protocol) mode, communicating with an IDE via JSON-RPC
over stdin/stdout so the editor can directly drive kimi's sessions and tool calls.
```
```
Once started, the command prints no banner and immediately waits for the ACP client to send an
`initialize` request on stdin. Logs are written to stderr (as well as the diagnostic log under
`~/.kimi-code/logs/`), so the ACP channel itself stays clean.
```
`kimi web` carries the product's only `--dangerous*` flag:
```
| `--dangerous-bypass-auth` | Disable bearer-token auth on all REST and WebSocket routes so the web UI connects without a token; only for trusted networks or behind an authenticating proxy |
```

### (3) stream-json — what is and is not documented

The complete published description (`kimi-command.md`):
```
When you need to parse output programmatically, use the `stream-json` format — each line on stdout
is a JSON object:

kimi -p "List changed files" --output-format stream-json

In `stream-json` mode, regular replies produce an Assistant message; when the model calls a tool,
an Assistant message with `tool_calls` is emitted first, followed by the corresponding Tool
message, then subsequent Assistant messages. Thinking content is not written to JSONL; tool
progress and "resuming session" notices are still written to stderr.
```

**Established:** JSONL on stdout; OpenAI-chat-shaped `Assistant` (with `tool_calls`) then `Tool`
messages; thinking excluded; progress on stderr. **NOT FOUND, after all 22 English pages, the zh
mirror and `kimi.com/code/docs`:** the `type` enumeration, any example object, a terminal/result
message, error shapes, **any token or cost field**, any `--input-format`, and any documented stdin
prompt path.

**The documented usage numbers live on `kimi web`'s server API** (`docs/en/reference/server-api.md`)
— the only place `total_cost_usd` appears in the whole doc set:
```
| `usage` | object | Token rollup `{ input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, context_tokens, context_limit?, total_cost_usd?, turn_count? }`; all zeros outside the snapshot endpoint |
```
That reference documents a loopback REST + WebSocket protocol with event types `turn.started` →
`assistant.delta` → `tool.call.started` / `tool.result` → `turn.ended`, cursor/epoch
resubscription, and `GET /api/v1/sessions/{session_id}/snapshot`.

**And it carries something no v0 lane has — a live plan-quota readback:**
```
On success, `data` is `{ kind: "ok", summary, limits, extra_usage }` ... a row is
`{ name?, window?, used, limit, reset_at? }` with `window` as `{ duration, unit }`, `unit` one of
`minute` / `hour` / `day` / `week`. `extra_usage` (nullable) is the pay-as-you-go wallet:
`{ balance_cents, total_cents, monthly_charge_limit_enabled, monthly_charge_limit_cents,
monthly_used_cents, currency }`.
```
This **closes A11 audit item U3** (*"`/usage` response field names and shape — mentioned but never
specified"*) and is a candidate answer to **U4** (the missing reset-time signal, which is why
`plandata`/`lanedata` carry an empty `reset_marker`). It is **provider-signaled observed state**,
D4-clean in exactly S10.4's sense — *"reset times come only from provider signals"*, overlay and
never the pressure denominator. **Recorded, not built here** (R23).

### (4) Per-user isolation

Source: `docs/en/configuration/data-locations.md`

```
Overrides the data root directory; the default is `~/.kimi-code`. Once set, the config file,
sessions, logs, OAuth credentials, and all other data land under the new path
```
```
Once set, **all** Kimi Code data — config, sessions, logs, OAuth credentials, Kimi-specific user
Skills, global `AGENTS.md`, and more — lands under the new path.
```
```
> Make sure the directory is writable. Multiple `kimi` instances sharing the same `KIMI_CODE_HOME`
> will share config and credential files.
```

**`KIMI_CODE_HOME` is the sole isolation knob. No XDG support is documented** — and it is a clean
knob, but it has one hole:
```
**Generic `.agents` resources** stay under the real OS home so they can be shared across tools. For
example, user-level generic Skills remain at `~/.agents/skills/`, while Kimi-specific user Skills
move with `KIMI_CODE_HOME` as `$KIMI_CODE_HOME/skills/`.
```
`HOME` is also documented as *"used to resolve the default data path"*. **So HOME must be bounded
as well as `KIMI_CODE_HOME`** — the same lesson the opencode lock entry already records at cost.

Default layout, verbatim:
```
$KIMI_CODE_HOME  (default: ~/.kimi-code)
├── config.toml             # User configuration
├── tui.toml                # Terminal UI preferences (including auto-update toggle)
├── AGENTS.md               # Global Kimi-specific agent instructions (optional)
├── mcp.json                # User-level MCP server declarations (optional)
├── skills/                 # Kimi-specific user-level Skills (optional)
├── plugins/
│   ├── installed.json      # Installed plugin records and enabled state
│   └── managed/            # Plugin copies installed from zip/local paths
├── session_index.jsonl     # Session index
├── credentials/            # OAuth credentials (dir 0700, files 0600)
│   ├── <name>.json
│   └── mcp/
│       └── <key>-<suffix>.json
├── sessions/               # Session data (see below)
│   └── <workDirKey>/<sessionId>/
├── bin/
│   ├── rg                  # managed ripgrep binary for Grep (rg.exe on Windows)
│   └── fd                  # managed fd binary for file references (fd.exe on Windows)
├── logs/
│   └── kimi-code.log       # Global diagnostic log
├── updates/
│   ├── latest.json
│   ├── install.json
│   ├── install.lock
│   └── rollout.log
└── user-history/
    └── <md5(workDir)>.jsonl
```
```
Each session's data is stored under `sessions/<workDirKey>/<sessionId>/`, and a top-level
`session_index.jsonl` index is maintained (one record per line, each containing `sessionId`,
`sessionDir`, and `workDir`). `workDirKey` is a bucket name derived from the working directory
path, in the format `wd_<slug>_<first-12-chars-of-sha256>`.
```
```
Do not manually edit files inside the `sessions/` directory — doing so may prevent sessions from
being restored correctly.
```
Per-session files include `state.json` and `agents/main/wire.jsonl` (*a full replayable record*) —
i.e. this engine **does** keep a JSONL transcript, so `Cursor.TranscriptPath` and the copy-aside
are reachable here (unlike opencode).

**Config files read:** `$KIMI_CODE_HOME/config.toml`, `tui.toml`, `SYSTEM.md`, `AGENTS.md`,
`mcp.json`, and **`<project-root>/.kimi-code/local.toml`**:
```
In addition to the user-level files under `~/.kimi-code`, Kimi Code reads a project-local
configuration file at `<project-root>/.kimi-code/local.toml`.
```
**No walked-up discovery**, stated explicitly:
```
The CLI currently reads a single user-level config file and has no project-level config file
mechanism. To isolate config between different projects, point `KIMI_CODE_HOME` at different data
directories
```

**Instruction files (the `CLAUDE.md` analogue) are `AGENTS.md`:**
```
Global Kimi-specific instructions can live at `$KIMI_CODE_HOME/AGENTS.md` (default:
`~/.kimi-code/AGENTS.md`). When you relocate the data root with `KIMI_CODE_HOME`, this global
instruction file moves with it. Generic cross-tool instructions can still live under
`~/.agents/AGENTS.md` in the real OS home, and project-level instructions remain under the project
tree, for example `.kimi-code/AGENTS.md` or `AGENTS.md`.
```
**Whether `AGENTS.md` ingestion can be disabled: NOT FOUND** — no flag, no config key. The
`SYSTEM.md` template exposes `${agents_md}` (*"Content of the workspace instruction files (such as
`AGENTS.md`)"*), so a `SYSTEM.md` omitting that placeholder **may** exclude it — **an inference,
not a doc statement**, and one the spike must settle (R0-Q6).

**Untrusted-repo takeover channel, verbatim** — directly S11.7 P-T09-1 and the S03.5 cwd channel:
```
A project-scoped file can take over a built-in agent entirely: naming it `agent.md` with
`override: true` replaces the **default main agent's whole system prompt** ... Review
`.kimi-code/agents/` and `.agents/agents/` in unfamiliar repositories with the same caution you
would apply to scripts, before running Kimi Code inside them.
```

**Env vars** (`docs/en/configuration/env-vars.md`, ~60 total). Build-relevant beyond the above:
`KIMI_DISABLE_TELEMETRY`, `KIMI_CODE_NO_AUTO_UPDATE`, `KIMI_LOG_LEVEL`, `KIMI_CODE_INFINITE_RETRY`,
`KIMI_CODE_IDENTITY_NAME`/`_SLUG`, `KIMI_DISABLE_CRON`, `KIMI_SUBAGENT_TIMEOUT_MS`,
`KIMI_CODE_SWARM_TIMEOUT_MS`, `KIMI_LOOP_MAX_STEPS_PER_TURN`, `KIMI_MODEL_MAX_COMPLETION_TOKENS`,
`KIMI_CODE_LEGACY_FLAG`, plus `HTTP_PROXY`/`HTTPS_PROXY`/`ALL_PROXY`/`NO_PROXY`, `NO_COLOR`, `CI`.

### (5) Hooks

Source: `docs/en/customization/hooks.md`. Configured as a `[[hooks]]` array **in `config.toml`**:
```
| `event` | `string` | Yes | Trigger event name; must be one of the entries in the "Event Reference" table below |
| `matcher` | `string` | No | A regular expression to filter event targets; if omitted, matches all |
| `command` | `string` | Yes | The shell command to run when triggered |
| `timeout` | `integer` | No | Timeout in seconds, range 1–600; defaults to 30 seconds |
```
```
`[[hooks]]` only allows these four fields; extra fields will cause the config file to fail to load.
```
```
**When multiple rules match the same event**, all matching hooks run in parallel; multiple rules
with identical `command` values run only once.
```
stdin contract:
```json
{
  "hook_event_name": "PreToolUse",
  "session_id": "session_abc",
  "session_title": "Fix the login page",
  "client_type": "kimi_code_cli",
  "cwd": "/path/to/project"
}
```
```
Specific events will also include additional fields (such as tool name and command content); see
the event reference below. All field names use snake_case.
```
Return contract:
```
| Exit code | Meaning | CLI behavior |
| `0` | Normal exit, allow | Continue execution; stdout content (if any) may be appended to context |
| `2` | Intentional block | Stop the current operation; stderr content (printed via `console.error`) is used as the reason for blocking |
| Other non-zero | Script error | Default allow (fail-open) |
| Timeout or crash | Script exception | Default allow (fail-open) |
```
```json
{
  "hookSpecificOutput": {
    "permissionDecision": "deny",
    "permissionDecisionReason": "Please use rg instead of grep"
  }
}
```
```
| `PreToolUse` | Tool name | ✓ | Triggered before a tool call (before permission checks); the tool will not execute if blocked |
```
```
Only **blockable events** (`PreToolUse`, `Stop`, `UserPromptSubmit`) have return values that affect
the main flow. All other events are **observation-only events**
```
**Fail-open, verbatim** (F3):
```
Even if the script errors or times out, the CLI **will not interrupt your work** as a result — this
"allow on failure" design is called fail-open, preventing hook errors from becoming blockers.
```
```
Precisely because of fail-open, Hooks are suitable for alerts and lightweight interception, but
**should not be used as the sole security barrier**. For truly high-risk operations, rely on
permission approvals and manual confirmation.
```
Events: `UserPromptSubmit`✓, `UserPromptQueued`, `PreToolUse`✓, `Stop`✓, `TurnStarted`,
`PostToolUse`, `PostToolUseFailure`, `PermissionRequest`, `PermissionResult`, `SessionStart`,
`SessionEnd`, `SessionHeartbeat`, `SubagentStart`, `SubagentStop`, `TaskStarted`, `StopFailure`,
`Interrupt`, `PreCompact`, `PostCompact`, `Notification`. (`SessionStart` and `PreCompact` exist —
the S05.4 re-injection and the A4 PreCompact-veto machinery have analogues here, **not this
packet's scope**, recorded so a later packet does not re-derive it.)

### (6) AUTH — the decisive question, answered

**A shell `KIMI_API_KEY` does NOT authenticate this CLI.** Verbatim (`docs/en/configuration/env-vars.md`):
```
::: warning Important: API keys are not configured here
Credential variables such as `KIMI_API_KEY`, `ANTHROPIC_API_KEY`, and `OPENAI_API_KEY` are **not**
read automatically from shell environment variables. Running `export KIMI_API_KEY=xxx` in the
terminal does not give any provider its key — they must be written in `config.toml` under
`[providers.<name>]` or the `[providers.<name>.env]` sub-table.

The only exception is the `KIMI_MODEL_*` family, which is an explicit channel that *does* read
credentials from the shell
:::
```
Restated in `overrides.md`, and the `[providers.<name>.env]` sub-table is **a TOML table that
merely reuses env-var names**, not the environment:
```
The `[providers.<name>.env]` sub-table is just a TOML section in the config file — it does not
write anything into the shell environment.
```

**So: headless env-key auth IS possible, through `KIMI_MODEL_*` — a DIFFERENT variable name from
the API lane's.**
```sh
export KIMI_MODEL_NAME="kimi-for-coding"
export KIMI_MODEL_API_KEY="YOUR_API_KEY"
export KIMI_MODEL_BASE_URL="https://api.example.com/v1"
```
```
| `KIMI_MODEL_NAME` | Yes (also the enable switch) | Model id sent to the API | — |
| `KIMI_MODEL_API_KEY` | Yes | API key | — |
| `KIMI_MODEL_PROVIDER_TYPE` | No | Provider type: `kimi`, `anthropic`, `openai` | `kimi` |
| `KIMI_MODEL_BASE_URL` | No | API base URL | Each type has its own default |
```
```
When `KIMI_MODEL_NAME` is set, the CLI synthesizes a temporary provider and model alias from the
`KIMI_MODEL_*` variables in memory — nothing is written back to the config file. These variables
take priority over `default_model` in `config.toml`, but the `-m <alias>` option at startup still
has the highest priority.
```
```
If `KIMI_MODEL_NAME` is set but a required variable is missing, startup fails immediately with a
clear error message.
```
Two properties this design depends on and gets for free: **nothing is written back to the config
file** (the secret never lands on disk), and **a missing variable fails at startup** (fail-closed,
never a silent unauthenticated call).

**A membership Console key is a static API key against the subscription endpoint** — no OAuth
needed (`docs/en/kimi-code/error-reference.html`):
```
Kimi Code and the Kimi Open Platform are two separate systems — keys and Base URLs are not
interchangeable:
Kimi Code: get your key from the Console, Base URL is https://api.kimi.com/coding/v1 (OpenAI
protocol) or https://api.kimi.com/coding/ (Anthropic protocol)
Open Platform: get your key from platform.kimi.com, Base URL is https://api.moonshot.cn/v1
```
Corroborated by `docs/en/third-party-tools/claude-code.html`: *"Make sure you have subscribed to
Kimi membership and activated Kimi Code benefits. Obtain an API Key: Go to the Kimi Code Console;
Click 'Create API Key' …"*.

**Base-URL variables are three, and they are not interchangeable:**
```
| `KIMI_CODE_BASE_URL` | Managed API base URL used after OAuth login | `https://api.kimi.com/coding/v1` |
```
```
::: warning
`KIMI_CODE_BASE_URL` (OAuth-managed service, targeting `kimi.com`) and `KIMI_BASE_URL` (direct API
key connection, targeting `moonshot.ai`) are two distinct variables.
:::
```
`KIMI_MODEL_BASE_URL` is the shell channel for the synthesized provider. Also
`KIMI_CODE_OAUTH_HOST` / `KIMI_OAUTH_HOST` (default `https://auth.kimi.com`).

**The interactive path exists and is not needed.** `kimi login` runs an RFC 8628 device-code flow
non-interactively-*ish* (prints a URL and code to stderr, polls) but still requires a human at a
browser once. The credential lands at `$KIMI_CODE_HOME/credentials/<name>.json`:
```
**`credentials/`**: OAuth credential directory, with permissions `0o700` (directory) / `0o600`
(files), readable and writable only by the current user. Managed provider credentials are stored as
`credentials/<name>.json` ... Credentials are written using an atomic flow (tmp → fsync → rename)
to prevent corruption.
```
**Lifetime / expiry / refresh cadence: NOT FOUND** — only *"the built-in authentication toolchain
automatically writes and refreshes credentials"*. Device unbinding IS documented: *"devices
inactive for over 30 days are automatically unbound and can be restored by running `/login` again."*

**★ THE SHARED-POOL PROOF** (`https://www.kimi.com/code/docs/en/kimi-code/membership.html`):
```
Kimi Code is the developer-focused AI coding service within Kimi membership benefits, provided
together with a Kimi membership subscription and sharing the same quota. Requests from the CLI, VS
Code, and third-party tools all count toward that quota.
```
```
All logged-in devices and API Keys share the same quota; devices inactive for over 30 days are
automatically unbound and can be restored by running /login again.
```
```
Kimi Code's quota refreshes automatically every 7 days from your subscription date; unused quota
does not roll over. Beyond the weekly quota, there is also a rolling 5-hour rate window — even with
quota remaining, too many requests in a short time trigger rate limiting, which recovers
automatically once the window rolls over.
```
```
Kimi Code shares quota with your Kimi membership plan: if your Kimi membership's monthly total is
reached, Kimi Code quota is frozen until the monthly quota resets or you upgrade.
```
```
Extra Usage: ... Kimi on the web and Kimi Code share the same Extra Usage balance
```
This is the **direct primary-source statement** that the CLI and `api.kimi.com/coding/v1` draw one
pool — sharper than the line `plandata/kimi.json` currently carries, and it names the CLI
explicitly. It grounds **R21**.

**AMBIGUOUS, and the spike settles it (R0-Q4):** no doc line states that `-p` works with
`KIMI_MODEL_*` alone and no `config.toml`. The inference is strong (the channel is documented as
resolved at startup, in memory, ahead of `default_model`) but it is an inference.

### (7) ToS / unattended use — see F0

The prohibition is §1 F0, quoted verbatim there. The countervailing lines, also verbatim:
```
Q3: I use Kimi Code across multiple devices and tools at the same time. Will that get me suspended?
No. Switching between devices (e.g., work laptop, personal machine) or different coding tools (Kimi
CLI, VS Code, Claude Code) is a completely normal usage pattern. You won't be mistakenly flagged as
long as you don't deliberately spoof or alter your client identity.
```
```
Q1: I'm a heavy user with high daily usage. Will I get flagged by mistake?
No. Our risk detection considers a wide range of signals. Users with normal patterns across IP
address, conversation behavior, and usage metrics are not typically flagged, regardless of volume.
```
Enforcement:
```
If your usage doesn't align with the guidelines above, we'll review the situation first and take
appropriate action—such as limiting concurrent access—based on the severity. You'll then see a
You've reached your concurrent request limit error
```
Two further prohibitions that bind a **household** platform:
```
Don't resell your account or API access
```
```
Don't spoof or alter client identity information
We rely on this information to maintain service quality and security.
```
⚠️ The last one **forbids** `KIMI_CODE_IDENTITY_NAME` / `KIMI_CODE_IDENTITY_SLUG` (which rewrite
the User-Agent product token) against a membership credential. See **R14**.

The CLI's own product docs do bless `-p` for scripting (*"When running a single prompt in a script
or CI environment, use `-p`"*, *"Skip approval prompts — suitable for batch tasks that are known to
be safe"*), and `KIMI_CODE_INFINITE_RETRY` is documented as *"Intended for long-running unattended
evaluations"*. **Tool documentation is not the subscription's commercial terms** — that tension is
F0's substance, not its resolution.

**No formal ToS document was locatable** for either Kimi Code or the Open Platform in this capture —
only the Community Guidelines. The A11 audit's two ToS reads
(`platform.kimi.ai/docs/agreement/modeluse`, `kimi.com/user/agreement/modelUse`) remain the ToS
evidence and are unchanged; this page is a **third, previously unread** first-party usage-policy
surface.

### (8) Operational facts a Go adapter needs

**Exit codes — sparse.** Only documented: `kimi login` (`0`/`1`), `kimi doctor` (`0`/`1`),
`kimi server` (deprecated, `1`), and `/goal` in print mode (*"Prompt mode exits with code `0` when
the goal completes, `3` when it blocks, and `6` when it pauses."*). **General `kimi -p` exit codes:
NOT FOUND** — no documented code for model error, quota exhaustion or tool failure. **Do not branch
on exit codes beyond 0/non-zero without the spike (R0-Q3).**

**stdout/stderr split — never merge them:**
```
Output uses a transcript style: thinking content and Assistant text are both prefixed with `• `,
and wrapped lines are indented by two spaces. Assistant text goes to stdout; thinking, tool
progress, and "resuming session" notices go to stderr.
```

**Error taxonomy** (`docs/en/kimi-code/error-reference.html`), verbatim messages and statuses:
```
You've reached your 5-hour usage limit                              403
You've reached your weekly (7-day) usage limit                      403
You've reached your monthly usage limit for this billing cycle      403
Your credit balance is insufficient                                 403
You've reached your concurrent request limit                        403
We're receiving too many requests                                   429
The engine is currently overloaded                                  429
unable to verify your membership benefits                           402
The API Key appears to be invalid or may have expired               401
total message size N exceeds limit 2097152                          400
Your request exceeded model token limit: 262144                     400
```
```
Note
If you are using a third-party client such as OpenCode or Claude Code, the client may transform or
re-wrap error codes, so the code you see may differ from what is documented here. In that case,
focus on the text content of the error message and match keywords in the quick lookup table below.
```
```
The request is correctly formatted and identity is verified, but the current account's subscription
tier does not include the requested capability (such as K3, 1M context, or the HighSpeed model).
Retrying is pointless
```
**This confirms the message-keyed taxonomy §64 built** — and it adds strings the shipped
`lanedata/kimi.json` table does not carry, plus one **trap**: see **R16**.

**Engine retry semantics:**
```
Retries only apply to transient failures — connection errors, timeouts, HTTP 429 rate limits, and
5xx server errors. A 429 caused by an exhausted quota or insufficient account balance is not
retried and fails immediately, since it cannot succeed until the account is recharged.
```
```
Retry every failed LLM request indefinitely — turn steps and background operations such as
compaction alike — instead of failing the task; waits use exponential backoff (capped at 32 s) and
honor the server's `Retry-After` header ... Intended for long-running unattended evaluations
against endpoints that may fail temporarily
```
(S10.5 is explicit that *"Engine-native retry is NEVER the policy layer"* — so `KIMI_CODE_INFINITE_RETRY`
is **never set**, and the engine's own retry stays minimal while the wrapper classifies.)

**Telemetry:** `KIMI_DISABLE_TELEMETRY=1` (also `true`/`yes`/`y`) plus `telemetry = false` in
`config.toml`; the package depends on `@moonshot-ai/kimi-telemetry`.

**Auto-update (F4):**
```
| `KIMI_CODE_NO_AUTO_UPDATE` | Fully disable the update preflight — no check, background install, or prompt. Legacy alias `KIMI_CLI_NO_AUTO_UPDATE` is also honored | Truthy: `1`/`true`/`yes`/`on` |
```
plus `tui.toml` `[upgrade].auto_install`, default `true`.

**Two engines exist; v2 is default:**
```
| `KIMI_CODE_LEGACY_FLAG` | Use the legacy `agent-core` engine for `kimi`, `kimi -p`, `kimi doctor`, `kimi acp`, `kimi export`, and `kimi provider`; these commands use `agent-core-v2` by default |
```
Several settings (`KIMI_CODE_INFINITE_RETRY`, `KIMI_CODE_IDENTITY_*`, `builtin_product_skills`) are
**ignored** under the legacy flag. The pin therefore pins *two* engines; conformance asserts v2.

**Network egress to expect:** `api.kimi.com`, `auth.kimi.com`, `api.moonshot.ai`,
`models.dev/api.json` (provider catalogue), `code.kimi.com` (installer, plugin marketplace, update
checks) — plus first-use binary downloads:
```
The first time the `Grep` tool needs ripgrep, the CLI can automatically download `rg` and cache it
at `bin/rg` ... File-reference completion in the terminal UI uses `fd`; the CLI downloads and
caches it at `bin/fd`
```
Proxy support is complete (`HTTP_PROXY`/`HTTPS_PROXY`/`ALL_PROXY` SOCKS/`NO_PROXY`, loopback always
bypassed) — which matters for the S11.5 injection proxy when that packet lands.

**Model ids** (`https://www.kimi.com/code/docs/en/kimi-code/models.html`):
```
Fill in the Model ID, not the model version name: when calling a model, use one of the Model IDs
from the table above (k3, k3-256k, kimi-for-coding, kimi-for-coding-highspeed).
```
**This is new primary-source corroboration for two ids** the shipped `lanedata/kimi.json` flags
`seed-only-pending-observation` because the opencode 1.18.3 embedded record lacks them. It settles
that `k3-256k` and `kimi-for-coding` are **real vendor model ids**; it does **not** settle what the
operator's ACCOUNT serves, which stays P-T17-3's job. See **R15**.

**Misc:** `Bash` tool *"stdin is always closed — interactive commands receive EOF immediately"*;
termination is *"SIGTERM → 5-second grace period → SIGKILL"*; `kimi web` binds loopback, prints a
bearer token, stores it at `~/.kimi-code/server.token`, rotatable via `kimi web rotate-token`,
default port `58628`/`58627` with `+1` retry.

### Capture gaps, named

| # | Gap | Closes at |
|---|---|---|
| G1 | The stream-json envelope: type set, terminal message, error shape, **usage/token fields** | **R0 spike, $0** |
| G2 | General `kimi -p` exit codes | R0 spike |
| G3 | Does `-p` run on `KIMI_MODEL_*` alone, with no `config.toml`? | R0 spike |
| G4 | Prompt via stdin / pipe into `-p` (no `--input-format` exists) | R0 spike |
| G5 | OAuth credential JSON schema, token lifetime, refresh cadence | not needed if R13 holds; else ceremony |
| G6 | Any switch disabling `AGENTS.md` ingestion | R0 spike (the `${agents_md}` inference) |
| G7 | A formal Open Platform ToS | **blocks the F0 escape route**; not this packet |
| G8 | `--yolo -p` — reference says rejected, guide shows it | R0 spike |
| G9 | `.cn` vs `.ai` regional split for the Open Platform base URL (`GET /api/v1/oauth/region`, *"the default is `mainland-cn`"*) | ceremony, per account |
| G10 | Per-tier weekly allowance figures | still UNVERIFIED (A11 audit U2) — operator console at bring-up |

---

## §3 · Requirements

Each carries its spec reference. "MUST" is binding. A requirement the executor cannot satisfy is a
**finding to report**, never a thing to quietly drop or fake.

**R-GATE (S03.6, S00.9) · Nothing in this section starts until OQ-0 is dispositioned.** If the
operator's Gate-A ruling is anything other than an explicit, recorded "proceed", this packet does
not build: it becomes a re-audit + freeze packet instead, and A12 is not applied.

### R0 · The $0 capture spike, FIRST

**R0 (S03.3 rule 4, §61 tier-R precedent) · Before any adapter code, run a hermetic $0 spike and
commit its result file.** `KIMI_MODEL_PROVIDER_TYPE=openai` + `KIMI_MODEL_BASE_URL=http://127.0.0.1:<port>/v1`
+ a sentinel key points a **real** `kimi -p` at a **loopback fake OpenAI-compatible provider**. No
provider call, no quota, no login. Result file:
`P3/measurements/2026-08-XX-kimi-cli-print-mode-spike.md`. **Pre-registered questions, answered by
execution and not by argument:**

| Q | Question | Why it is load-bearing |
|---|---|---|
| Q1 | The complete `--output-format stream-json` envelope: every message shape emitted across a plain turn, a tool-calling turn, an error turn and a cancelled turn | G1; the tier-F fixtures ARE this capture |
| Q2 | **Does any frame carry token counts?** Field paths, verbatim | D7/S10.1 — decides OQ-A |
| Q3 | Exit codes for: clean completion, model error, tool failure, cancellation | G2; the Outcome mapping |
| Q4 | Does `-p` run with `KIMI_MODEL_*` alone and **no** `config.toml`? | G3; the credential design |
| Q5 | Does the process **exit** with `print_background_mode = "exit"` and explicit timeouts? Measure wall-clock | F4/R12 — the runaway guard |
| Q6 | Is `AGENTS.md` ingestion suppressible via a `SYSTEM.md` omitting `${agents_md}`? | G6; an S03.5 channel with no documented knob |
| Q7 | Is `--yolo -p` accepted or rejected? | G8; a doc self-contradiction |
| Q8 | With `[tools] disabled = ["Agent","AgentSwarm"]`, is the tool **absent from the offered toolset** (pre-inference) or merely refused at call time? | R9 — structural vs behavioral, the sole-controller rider's whole substance |
| Q9 | Does a `[permission] deny` rule hold in `-p` mode, and does a `PreToolUse` hook returning `permissionDecision: "deny"` stop the call? | R10 |
| Q10 | Is `session_index.jsonl` + `sessions/<workDirKey>/<sessionId>/` populated by a `-p` run, and does `--session <id>` resume it? | R4 resume; `Cursor` |

**The spike's answers are the packet's ground.** A requirement below that the spike contradicts is
**corrected by the spike**, and the correction is reported — the S03.3 rule 4 posture: suites assert
behavior, never docs.

### The substrate

**R1 (S03.2, A12) · A third substrate constant, named beside the other two.**
`internal/adapters/adapters.go` gains `SubstrateKimiCLI = "kimi-cli"` in the same `const` block as
`SubstrateClaudeCLI`/`SubstrateOpencode`. The block's leading comment — currently *"v0 ships exactly
two substrates carrying three lanes"* — is corrected to the post-A12 truth and cites A12 by name.
This is a **marker site A12 claims**, so a test asserts the annotation (T-A12).

**R2 (S03.2) · `LaneKimiCLI = "kimi-cli"`**, beside `LaneKimi`, its comment stating the shared-pool
fact and pointing at the lane document.

**R3 (S03.1, §10) · One new package, `internal/adapters/kimicli`, behind the EXISTING seam.**
Implements `adapters.Adapter`/`adapters.Session` and nothing else. Imports are `internal/adapters`
+ stdlib **only** — never `internal/sandbox` (§12: sandbox → adapters is the only direction) and
**never `internal/adapters/opencode`** (R16: the lane document reaches this package as a func seam).
Layout mirrors `claudecli`: `kimicli.go`, `lower.go`, `parse.go`, `hook.go`, `conformance_test.go`,
`live_smoke_test.go`, `testdata/`.

**R4 (S03.1) · The seven D3 verbs, over the transport OQ-A settles.** Against the REAL surface:
- **start** — `kimi -p "<prompt>" --output-format stream-json`, `-m <model>`, `--agent-file`, the
  lowered `KIMI_CODE_HOME`, own process group. **The control plane CANNOT choose the session id**
  (no `--session-id`): `Cursor.SessionID` is the engine-REPORTED id, harvested from the stream or
  from `session_index.jsonl`, exactly as the contract already requires ("the session id AS REPORTED
  by the engine (never the requested id)"). Nothing may fabricate one.
- **stream** — stdout JSONL only. **stderr is never merged** (§2(8)): it carries thinking, progress
  and "resuming session" notices, and merging it corrupts the JSONL. Capture stderr separately,
  bounded, for ops.
- **checkpoint** — per-paid-call `Usage` events. **This is the requirement OQ-A exists to make
  satisfiable**; if the spike finds no usage in `-p`, the lane cannot ship on `-p` (R0-Q2).
- **pause** — interrupt at a completed message boundary, **never mid-tool-call** (P-T01-4).
- **resume** — `--session <id>` with **FULL invocation re-supply** from the park record (S03.4:
  the engine restores nothing; a resume built from less silently executes a parked call unreviewed).
  On this engine the config lives in the run's own `KIMI_CODE_HOME`, so re-supply means the whole
  lowered home is rebuilt and re-verified, not assumed intact.
- **cancel** — the ladder: safe boundary → process-group TERM → KILL, spawned as group leader.
  There is no cooperative abort on the `-p` surface.
- **report_usage** — whatever the spike finds, **priced by Sinet's table and never the engine** (D5).

**R5 (S03.1, P-T01-2) · Forward-tolerant parsing is a MUST.** Unknown engine frames are logged and
skipped — never fatal, **never minted as a new platform kind** outside
`{message, usage, rate_limit, gate_ask, tool_result, done}`. Excerpts bound at
`adapters.ExcerptCap`. Pinned as a **property** over random unknown frames, not one example. This
matters more here than anywhere: the envelope is undocumented and moves weekly.

**R6 (S10.1) · One paid call is exactly one `Usage` event.** Group by message id. The Anthropic
accounting normalization (`total prompt = cache_read + cache_creation + input_tokens`) is owed
**only if** the spike shows the same field semantics — do not assume it; a mis-normalized row
double- or under-counts (P-T08-1). `EngineCostUSD` is a **cross-check only**.

**R7 (§60, S03.1) · `Outcome.ResultText` follows the ratified order verbatim.** (1) the terminal
envelope's own result text when it carries anything; (2) otherwise the stream's final assistant
message **VERBATIM**; (3) otherwise empty. Never repair, complete or fabricate; never mint a crash
for an outcome the engine reported as success. On this engine, if the spike shows no terminal
envelope at all, (2) is the whole story and the code says so.

### Lowering, containment, the sole-controller rider

**R8 (S03.5) · The compound guarantee: one knob per config channel.** Compiled config is the ONLY
config this engine sees. The lowering table for this substrate, from §2:

| Config channel | Knob at pin 0.38.0 |
|---|---|
| settings / hooks / ceilings | the run's own `config.toml`, inside a **per-run `KIMI_CODE_HOME`** (R11) |
| instruction files (`AGENTS.md`) | moves with `KIMI_CODE_HOME`; the **`~/.agents/` leg does NOT** ⇒ HOME must be bounded; a `SYSTEM.md` omitting `${agents_md}` is the candidate suppressor — **R0-Q6 settles it, and if it fails this is a channel with NO knob and is reported by name** |
| system prompt | `$KIMI_CODE_HOME/SYSTEM.md` (full replacement) and/or `--agent-file` |
| MCP / connectors | no server declared in the run's `mcp.json`; `[tools] disabled = ["mcp__*"]` as the belt |
| skills / slash-commands | `--skills-dir <platform-owned-empty-dir>` (**replaces** discovery) + `builtin_product_skills = false` / `KIMI_CODE_BUILTIN_PRODUCT_SKILLS=0` |
| tools incl. native spawn | `[tools] enabled = [<allowlist>]` + `[tools] disabled = ["Agent","AgentSwarm"]` (R9) |
| cwd project-config | `<cwd>/.kimi-code/local.toml`, `<cwd>/.kimi-code/agents/*`, `<cwd>/.agents/agents/*`, `AGENTS.md` — the run cwd is platform-owned and isolated, and the lowering MUST assert none of these exists before spawn (the `agent.md`+`override:true` takeover is quoted in §2(4)) |
| plugins | `plugins/installed.json` absent in a fresh per-run home ⇒ none loaded; assert it |
| telemetry / update | `KIMI_DISABLE_TELEMETRY=1`, `KIMI_CODE_NO_AUTO_UPDATE=1` (R13) |
| engine selection | `KIMI_CODE_LEGACY_FLAG` unset ⇒ `agent-core-v2`; asserted |
| nested-session env | scrub the ambient `KIMI_*` / `KIMI_CODE_*` / `AI_AGENT` family, exactly as `claudecli` scrubs `ANTHROPIC_*`/`CLAUDE_*` |

Each row is enforced as an **attempt-the-leak** conformance entry asserting the decoy **did not take
effect** (S03.5). A channel with no available knob is reported **by name**, never silently open.

**R9 (S03.5, G1 rider 2) · Native subagents disabled, structurally.** `[tools] disabled =
["Agent","AgentSwarm"]`. **R0-Q8 decides whether that is structural (stripped pre-inference, the
ratified bar) or merely refused at call time**; if only the latter, that is a **finding for the
gate**, because the rider's whole substance is that the guarantee is structural. The adapter
**refuses loudly** (`ErrNativeSpawnTool`, the §61 precedent) any input that would re-admit them: an
allowlist naming `Agent`/`AgentSwarm` in any case, a **wildcard allowlist** (`enabled = ["*"]`
disables everything and `disabled = ["*"]` disables **nothing** — quoted in §2(2); a caller writing
either almost certainly meant the opposite), a gated-tools entry, or a config fragment re-enabling
them. Refuse; never silently overwrite.

**R10 (S03.4, F3) · The permission story, resolved honestly: DENY, never park.** On this engine
`-p` requests no human approval, hooks are **fail-open**, and there is **no ask/defer**. Therefore:
- The **enforcement** layer is the config's static `[permission] deny` rules + the `[tools]`
  allowlist — *not* hooks (the docs say so themselves: *"should not be used as the sole security
  barrier"*). Hooks are **audit and denial only**.
- A `CompiledWorker` carrying `GatedTools` **MUST be refused by name** on this substrate
  (`ErrGateParkUnsupported`), not auto-approved. §12's fail-closed rule is explicit: a decision the
  substrate cannot express is refused loudly, because *the only default available is consent*.
  Silently running a gated call under `auto` is the S03.4 failure the park exists to prevent.
- `Outcome.Park`/`Ask` therefore stay **unset with the reason in code** — the §61 posture for a
  structurally unreachable contract shape, never faked. `Outcome.GateFallback` likewise: the
  Anthropic defer trap has no analogue here.
- `--yolo`, `--auto`, `--yes`, `--auto-approve` and `--dangerous-bypass-auth` go in a
  `forbiddenFlags` list (the `claudecli` precedent) and are **asserted never emitted**.
- The honest consequence is written in three places: a doc comment, the lane document, and the
  conformance row's Notes. **It is not hidden behind a hook that cannot hold it.**

**R11 (S03.2, S03.5, S11.7) · A per-RUN `KIMI_CODE_HOME` under a per-USER root.**
`<stateDir>/engines/kimi-cli/<user>/` at 0700 per person (the `opencodeRoot` precedent), with the
run's own home beneath it. **Per-run, not per-user**, and the reason is structural: there is exactly
one `config.toml` per `KIMI_CODE_HOME`, there is **no `--config` flag**, and the docs state
plainly *"Multiple `kimi` instances sharing the same `KIMI_CODE_HOME` will share config and
credential files"* and *"To isolate config between different projects, point `KIMI_CODE_HOME` at
different data directories"*. A per-user home would make two concurrent runs share one lowered
config — S03.5's guarantee lost outright. Consequences, all named rather than discovered:
- **`HOME` is bounded too** (the `~/.agents/` leg, §2(4)).
- **The run's home must OUTLIVE the process while the run is parked** — the same reason
  `⚙ adapter.claude.cleanup_period_days` is raised above the park horizon (P-T02-1). Reaping is
  tied to the run's own lifecycle, plus a stale-home sweep; **never** a fixed timer that can
  evaporate a parked run's session store.
- **First-use cost is per home**: `bin/rg` and `bin/fd` download on first `Grep`/reference use
  (§2(8)). Either pre-seed `bin/` from a per-user cache or keep those tools off the allowlist;
  **measure and report** rather than assert (the opencode ~62 MB precedent, whose lock note records
  this lesson at cost).
- Assert **zero cross-leak and zero strays** between two users and between two runs.

**R12 (F4, S02.3, S10.8) · The run is BOUNDED, or it does not spawn.** In the lowered `config.toml`:
`print_background_mode = "exit"` (or `"drain"` with an explicit ceiling), an explicit
`print_max_turns`, an explicit `print_wait_ceiling_s`, an explicit `bash_task_timeout_s > 0`, and
explicit `[subagent] timeout_ms` / `[swarm] timeout_ms`. `KIMI_CODE_INFINITE_RETRY` is **never
set** (S10.5: *"Engine-native retry is NEVER the policy layer"*). **R0-Q5 measures that the process
actually exits.** A spawn whose lowered config lacks any of these is refused before the process
exists. This is the one requirement whose absence has already cost this household a session.

**R13 (S03.3 rule 2, F4) · The pin cannot self-upgrade.** `KIMI_CODE_NO_AUTO_UPDATE=1` on every
invocation, `[upgrade].auto_install = false` in the lowered `tui.toml`, `KIMI_DISABLE_TELEMETRY=1`,
and the `upgrade` subcommand unreachable from the adapter **by construction** — not guarded,
absent. The tier-R suite asserts the installed version equals `Pin` and **reports a delta LOUDLY**;
reconciling is the operator's S03.3 deliberate-bump procedure, never a silent retarget.

**R14 (F0, §2(7)) · Client identity is never altered.** `KIMI_CODE_IDENTITY_NAME` and
`KIMI_CODE_IDENTITY_SLUG` are **never set** and are scrubbed from the ambient environment — the
guidelines forbid spoofing client identity, and doing it while F0 is unresolved would convert a
policy question into a policy violation. A source scan asserts neither name appears in a non-test
file.

### The credential

**R15 (D2, S11.5, S01.6, §2(6)) · The credential channel is `KIMI_MODEL_API_KEY`, and only that.**
- The lane document's `credential` block is `{profile: "kimi-code", env_var: "KIMI_MODEL_API_KEY"}`.
  **Same broker profile as lane `kimi`, different variable name** — a shell `KIMI_API_KEY` does
  **not** authenticate this CLI (§2(6), quoted). Getting this wrong produces a silent
  unauthenticated startup failure that looks like a broken lane.
- `KIMI_MODEL_NAME` (= `StartRequest.Model`) and `KIMI_MODEL_BASE_URL` (= the lane document's
  `base_url`) are **LOWERING**, not credential — they carry no secret and belong in the adapter's
  env construction.
- `KIMI_MODEL_PROVIDER_TYPE` is set explicitly from the lane document rather than defaulted; which
  value pairs with `https://api.kimi.com/coding/v1` is **R0/ceremony**, not a guess.
- Two properties come free and are asserted: the CLI **writes nothing back to the config file**
  when the `KIMI_MODEL_*` channel is used, and a **missing required variable fails at startup** —
  fail-closed, never a silent unauthenticated call.
- **No OAuth path is used.** `kimi login`, `/login`, and `$KIMI_CODE_HOME/credentials/` are not part
  of this design. If the ceremony ever needs one, it is an operator-typed act in their own terminal
  with the credential file outside the repo, presence-only reads, never logged and never committed
  — but **it is not needed**, and a design that does not need an interactive login should not grow
  one.
- **Containment is a property, not a review** (the §65 shape): a sentinel reaches the lowered engine
  environment **exactly once** and appears in no event payload, no `run_events` row, no park record,
  no digest input, no ops log line, and nowhere in the SQLite file **or its WAL**.

**R15a · Two lanes, one broker resolution per spawn.** `laneCredInjector` composes one injector per
commissioned lane and applies them left to right. `kimi` and `kimi-cli` share profile `kimi-code`
but name **different variables**, so the broker is dialled twice for one secret. Resolve each
**profile** at most once per spawn and fan the material out to each distinct variable. Test both
directions, and pin the single-lane world byte-identical to before.

### The lane, as a document

**R16 (S03.6, §64) · `lanedata/kimi-cli.json`, in the EXISTING corpus.** Loaded by the same
directory embed, validated by the same `LoadLaneConfig` gates, declaring `"substrate": "kimi-cli"`.
- **Why the existing corpus.** `LaneConfig` is already substrate-agnostic — it carries a
  `substrate` field *precisely* so which engine serves a lane is DATA (its own doc comment says so),
  and every platform-side consumer (`Commission`, `CommissionedLanes`, `CommissionedSubstrates`,
  `CommissionedSeats`, `laneConfiguredModels`, `laneCredInjector`) walks **all** documents,
  substrate-blind by design. Splitting the corpus forks all of them. The package NAME is where the
  machinery was born, not what it owns — correct its package comment to say so. Moving it to a
  neutral `internal/adapters/lanedoc` is a real option with a wide diff and no behavior gain: **OQ-1**.
- **`provider_id` MUST be distinct** (`loadLaneDocs` refuses a duplicate BY NAME, and is right to).
  Suggested `kimi-code-cli`, with a `provider_id_note` stating honestly that on a non-opencode
  substrate this field is a **document-identity key**, not a provider key inside any engine config.
- **`npm` MUST be empty** with a note: this substrate has no ai-sdk provider package; the engine is
  the pinned CLI itself, recorded in `components.lock`. (`validate()` does not require `npm` —
  verified in the tree.) Declaring `@ai-sdk/anthropic` here would be a **false dated claim in a
  document whose entire value is that its claims are dated** (§64's most instructive lesson).
- **The signal table is carried, extended and corrected from §2(8)** — this is the packet's second
  most consequential piece of data work after the pool:
  - The shipped `lanedata/kimi.json` table was `DOCUMENTED-NOT-OBSERVED` from the 2026-08-24 error
    reference. This capture **re-reads the same page** and finds **strings it does not carry**:
    `403 "You've reached your 5-hour usage limit"`, `403 "You've reached your weekly (7-day) usage
    limit"`, `403 "Your credit balance is insufficient"`, `403 "You've reached your concurrent
    request limit"`, `402 "unable to verify your membership benefits"`, and the two `400` size
    limits. Add them to **both** documents, dated 2026-08-26, each classed deliberately.
  - **The trap, and it is a real one.** `403 "You've reached your concurrent request limit"` is
    **both** an ordinary concurrency shed **and the vendor's stated enforcement signal for a terms
    violation** — verbatim: *"we'll … take appropriate action—such as limiting concurrent
    access—based on the severity. You'll then see a You've reached your concurrent request limit
    error."* Classing it `transient` would make the platform **retry silently through an
    enforcement action against the operator's account**. It therefore carries **no
    `documented_class`** and falls through to the Class-4 status rule (freeze + operator alert),
    which is the safe half of P-T08-2. Say exactly that in the row's note.
  - `403 "Your credit balance is insufficient"` is a **balance** event, and the closed exemption
    vocabulary has **no `balance` member by design** — no class, it freezes. Correct.
  - `402 "unable to verify your membership benefits"` — no class; 402 freezes on the status rule.
  - The 5-hour / weekly / monthly `403`s are `depletion`; the `400`s are surfaced, not parked.
  - **Both documents' tables must be pinned EQUAL by test**, not by eyeball: same membership, same
    wire, and a divergence would make one lane freeze where the other parks.
- **The C5 `data_policy` rider is carried**, `enforced: false`, same citation, same
  `enforcement_note` — the constraint is a property of the provider, not the client path.
- **`reset_marker` stays deliberately empty** on both, for the unchanged reason (A11 audit U4), with
  the new `GET /api/v1/oauth/usage` finding recorded as the candidate that could fill it later (R23).
- **The wire signal seam:** `kimicli.Adapter` takes
  `Signals func(bodyText string, httpStatus int) (json.RawMessage, bool)`, wired at the composition
  root from the `kimi-cli` document's `ExtractSignal` + `MarshalJSON`. **The payload IS the
  contract** (`scheduler.SignalFromPayload` decodes it) — no shared type crosses, no sibling import
  exists, and the adapter classifies nothing. A nil seam forwards raw facts with no documented
  class: the honest degrade. `EndpointVerified` is established at **lowering**, where the config is,
  and forwarded as data — never looked up inside `Classify`, which is pure and total.

**R17 (S03.6) · The opencode adapter must not be handed a provider entry for an engine it does not
drive.** `Commission` composes entries for every commissionable document and `engineAdapters` hands
the map straight to `opencode.Adapter.ProvidersFor`. Filter by substrate at that boundary, with the
pure helper beside the lane documents (§65: the composer lives in the package owning both input and
output types). Without it opencode compiles a provider block for the CLI lane: harmless to
execution, a **lie in a config body that gets hashed, logged and inspected**, and a spurious serve
restart. Assert both directions — opencode's entries unchanged, the platform-wide map carrying it.

**R18 (§64, P-T17-3) · Record the model-id corroboration, do not over-claim it.** `kimi-code/models.html`
names `k3`, `k3-256k`, `kimi-for-coding`, `kimi-for-coding-highspeed` as the Model IDs to send. That
is **new primary-source evidence** for two ids `lanedata/kimi.json` flags
`seed-only-pending-observation` because the opencode embedded record lacks them. Update that grade
honestly — *documented at primary source 2026-08-26; still absent from the pinned opencode engine
record; the ACCOUNT's observed list still decides* — and **do not** promote them to observed. The
`kimi-cli` document carries the same ids at the same grades. `default_model` = `k3`.

### Commissioning and routing

**R19 (S03.6, S11.5, §65) · One placed credential ⇒ BOTH kimi lanes routable at startup.** Both
documents name profile `kimi-code`, so `commissionEngineLanes` commissions both the moment that
profile holds an `engine-cred` — **verify by test, do not assume**. Commissioning stays
**STARTUP-BOUND**: no watcher, no SIGHUP, no rescan; a key placed while the control plane runs is
picked up at its next start, and the startup line says so.

**R20 (S14.1, §65) · The commissioning INFO line names both lanes** for a person holding the
profile, and nothing secret appears.

**R21 (S03.2, §63 D5 / §65) · Dispatch is proven at the REAL call sites, never at the resolver.**
`laneSubstrates["kimi-cli"] == "kimi-cli"`, and a `kimi-cli`-seated decision reaches the **kimicli**
adapter through the production fill and the real dispatch sites (helper spawn and revise — the two
that dropped the lane at §63 drain r2), against recording adapters, with a nothing-placed control
taking the unchanged `claude-cli` path. **Mutation-verify**: neutering the fill must collapse the
commissioned case into the control and fail the guard.

**R22 (S08.8) · The seat comes from the document.** `default_model` on the document; **no model id
or endpoint is a Go constant** in `internal/adapters/opencode`, `internal/worker`, `internal/shell`
or `internal/stage` — the existing no-lane-constants source scan stays green. Seats stay
**execution-only**: no Kimi model has been measured against the S07.5 bars and seating one as
verified would be inventing a ratification.

### ★ The shared pool

**R23 (S10.1, S10.4, §64) · One pool, declared ONCE.** Grounded on the verbatim membership line
(§2(6)): *"provided together with a Kimi membership subscription and sharing the same quota.
Requests from the CLI, VS Code, and third-party tools all count toward that quota."*

- **(a)** `internal/metering/plandata/` gains **NO second document**. `SeedPlanDocs()` still returns
  **2**; a test asserts the count **and** that no document declares an allowance for `kimi-cli`
  independently.
- **(b)** `plandata/kimi.json` gains an explicit **pool identity** and the lanes drawing it
  (e.g. `"pool": "kimi-code-membership"`, `"pool_lanes": ["kimi","kimi-cli"]`), dated.
  `PlanDocFor(lane)` resolves **either** lane to that one document. Absent members mean the
  pre-packet behavior exactly (a document with no pool serves its own lane alone), so the `zai`
  document is expressible **unchanged** — the `PlanQuota.Unit` precedent.
- **(c)** The plan **reading** for a pooled lane counts consumption across **every lane in the
  pool**. `planUnits` currently filters `effectiveLane(runLane, usage) != lane`; that becomes a
  membership test over the pool's lane set. Without it each lane reports a half-empty pool and
  routing steers work onto an allowance that is already spent. The reading **names the pool and the
  lanes it aggregated**, so nobody reads the figure as one lane's own.
- **(d)** Both LANE documents carry the shared-pool statement with the new verbatim quote, and a
  test asserts they name the **same** pool. `plandata/kimi.json`'s `assumed_note` — *"Sinet is ONE
  consumer of a pool the operator also draws on interactively, so Sinet's own count is a LOWER
  BOUND"* — is now true across three consumers and the note is extended to say so.
- **(e)** **Budgets must not double-declare.** `plan_budgets` is keyed `(user_id, lane, window)`,
  so an operator could declare against one allowance twice. **OQ-2** carries the disposition; the
  executor implements the coordinator's answer at the **three layers LN-6 established** (store /
  HTTP boundary / `ReadPlanUnits`), because a rule spelled once is a rule that can be walked around.
- **(f) Recorded, not built:** `GET /api/v1/oauth/usage` on `kimi web` returns
  `{name?, window:{duration,unit}, used, limit, reset_at?}` plus the Extra-Usage wallet — a genuine
  **provider-signaled observed overlay**, D4-clean in S10.4's exact sense, and a candidate to close
  A11 audit U4 and populate `reset_marker`. Record it on the lane documents as a dated fact with
  `wired: false`. **Do not build it here**: the denominator stays the operator's budget, never an
  inferred provider window (D4), and inventing an overlay consumer is the class §63 D2 exists to
  close.

**R24 (S10.3) · Pricing is unchanged, and its honest state is stated.** Price rows are
operator-loaded at runtime (`internal/metering/pricestore.go`); **no kimi price row is seeded
anywhere in the tree** (grep-verified 2026-08-26). A `kimi-cli` run renders **UNPRICED** exactly as
a `kimi` run does today — never a silent $0 (P-T08-1). If the lookup keys on lane, the ceremony
owes rows for **both** lanes; name that for LN-8 rather than inventing a seeding path.
**Never meter dollars from the engine** (D5) — and note §2(3) shows `total_cost_usd` *does* exist on
the `kimi web` surface, which makes the rule live rather than theoretical here.

### Watch, conformance, the rail

**R25 (S16.4 item 9, §64) · Watch rows for the new component, and a classifier that can name the
lane.** 70 releases in 3 months makes the CLI's own changelog a **tier-1 row**; the
`community-guidelines.html` page becomes a tier-1 row too — **it is the page F0 came from, and the
A11 audit not watching it is precisely how F0 stayed invisible for two days.**
`internal/watchlist/classify.go`'s lane enum is a **constrained-decoding grammar**: until `kimi-cli`
is in it the classifier cannot emit the lane, so no hit could ever be attributed. Add it. Every
pinned count that moves, moves to an **exact number, never an inequality**, and the executor names
every site.

**R26 (S03.3, §10) · Conformance tiers F + R + L, with named skips.**
- **Tier F** — replay fixtures, always run; **the fixtures ARE R0's capture**, with its provenance
  and date on them.
- **Tier R** — real CLI, **zero cost**, auto-runs when the binary is installed; absence-skips print
  exactly `SANCTIONED SKIP (CONVENTIONS §10)`. Asserts installed-version-vs-`Pin`, argv
  acceptance/rejection, the isolation probes, the native-spawn-disable probe, and the
  bounded-exit probe — all terminating on the loopback fake provider, never a provider path.
- **Tier L** — paid, one minimal call, behind the **existing** `SINET_LIVE_SMOKE=1` (**no second env
  name is minted**) plus a commissioned lane; structurally unreachable at landing, printing its
  named skip. **Under F0 it stays unreachable regardless.**
- Suites **assert behavior, never docs or help text** (S03.3 rule 4).
- The conformance **registry** gains a **substrate** row and a **lane** row, each honest about both
  halves of its scope. Seed rows **15 → 17**, `engine_bump` **9 → 11**. **Verify against the tree;
  a count that disagrees is a finding, not a number to overwrite.**

**R27 (S16.2, S16.4, §4) · The pin lands through the rail, in the same commit as the package.**
A `components.lock` entry with every mandatory S16.2 field, pin **`0.38.0` EXACT** — never `latest`,
never a range. License **path-scoped, read verbatim at the pinned ref**, with a carve-out scan
(`enterprise/`, `ee/`, dual-license trailers) — the `opencode-ai` entry is the reference shape.
**No `modules` claim** (not a Go module) and **no `npm_packages` claim** (a global CLI, not a `web/`
dependency). `kimicli.Pin` exported and coupled by `TestPinMatchesLock`. `lockgate` stays green and
the entry count moves 39 → 40 (verify). The `notes` field records, at minimum: the SLSA/OIDC
provenance, the 70-versions-in-3-months cadence, the auto-update disable (R13), the per-run
`KIMI_CODE_HOME` posture, the `bin/rg`+`bin/fd` first-use network cost, the fail-open hook finding,
and **F0's disposition**. `replacement` names the concrete path — the existing `kimi` lane on the
opencode substrate already serves the same membership, which is this component's own funeral plan.

### Receipts and the amendment

**R28 (S02.2, S10.1) · Every run carries lane + substrate.** `runs.lane` = `kimi-cli`,
`runs.substrate` = `kimi-cli`. No new surface is invented for the comparison — LN-8 reads the
existing Lane column. If a `web/src` fixture moves mechanically, regenerate **only** through the
sanctioned `SINET_WRITE_API_FIXTURES=1` path, leave `web/src` application source untouched, and
**the frontend battery (vitest) is owed at landing** (standing rule).

**R29 (S00.9) · A12 applied byte-verified, in both copies — and only after OQ-0.**
`Spec/drafts/S00-front-matter.md` gains the A12 row (§11) appended after A11, currently the file's
last line (191). `Spec/core-architecture-v1.md` is then **regenerated, never hand-edited**:

```
cd Spec && awk 'FNR==1&&NR>1{print""}{print}' drafts/S[0-9]*.md > core-architecture-v1.md
```

`TestAssembledSpecReproducesFromDrafts` stays green, and a sibling of
`TestA11AmendmentLandedInBothCopies` asserts A12 in both copies byte-identically plus every marker
site it claims. **No ⚙ default or clamp moves ⇒ no S18 re-sweep**; the S18 tally stays
**118 keys / 33 domains** and the S18.3 data-surface count stays **3**, both asserted against the
spec text, never assumed.

**R30 (S03.6, §64) · A11's Gate-A record is corrected, whatever OQ-0 decides.** The audit file
`P3/measurements/2026-08-24-kimi-lane-gate-audit.md` records a class-2 PASS reached without reading
`community-guidelines.html`. A dated addendum records the omission, the text found, and the ruling.
**A record whose value is that its claims are dated cannot carry a verdict its own trigger has
fired against** — the §64 lesson, applied to the audit that taught it.

---

## §4 · Seams to respect, and the stub for each phase that has not come

| Seam | Posture at LN-7 |
|---|---|
| `StartRequest.Confiner` (S11) | **Honored** — a per-RUN process substrate can, unlike opencode's per-user server. Build the `*exec.Cmd` through it (the `claudecli.buildCmd` shape), populating `SpawnSpec{Argv,Env,Workspace,ROConfig,RWExchange,EnginePrefix}`. `EnginePrefix` matters: a global npm CLI lives outside `/usr` and must be reachable inside the sandbox — **and so must Node ≥22.19**. |
| `StartRequest.CredInject` (S11.5) | **Honored**, resolved fresh per spawn (R15). |
| The S11.5 **injection proxy** | **Deferred**, unchanged. Record this lane's wire facts on an exported constant beside `opencode.CredentialInjectionFacts` — including that the CLI honors `HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY` with loopback bypassed, which is what that packet will need. Do not build it. |
| `OnEvent` / `OnSession` (S05.3) | Wired as on the existing substrates; observers never block and persistence never depends on them. |
| `BridgeGrant` (S12.6) | **Untouched**, wired-but-dormant. |
| `Driver` (S02.4) | **Unchanged.** This engine DOES keep a per-session JSONL (`agents/main/wire.jsonl`), so `Cursor.TranscriptPath` and the copy-aside are reachable — set them if the spike confirms the path, and remember the engine's own warning that the store must not be hand-edited. Engine transcripts are still **never durable state** (P-T01-1). |
| S05.4 re-injection / A4 PreCompact veto | `SessionStart` and `PreCompact` hooks exist here (§2(5)). **Not this packet.** Recorded so a later packet does not re-derive it. |
| Routing-policy / per-lane data policy | **Still no enforcement point in the tree.** The C5 rider stays RECORDED-NOT-ENFORCED on both documents. |
| 13.4 settings UI | Not this packet (§66). |

---

## §5 · ⚙ settings

**This packet CONSUMES existing machinery and adds NO ⚙ key.** Expected at landing: **118 dotted
keys across 33 domains**, **3 data-valued settings surfaces** — a fourth lane document is another
**row** of an existing surface, never a new surface (§64). Assert both against the S18 draft's prose.

- `adapter.engine_ceiling_backstop_mult` — **no consumer here.** §2(2) establishes there is no cost
  or turn ceiling *flag*, and no budget setting at all; the ceilings that exist
  (`print_max_turns`, `print_wait_ceiling_s`) are turn/time, not spend. Say so in a doc comment in
  `lower.go`; **do not fake a consumer** (the §61 precedent, where the same honesty was a finding).
  Whether `CeilingSteps` should scale `print_max_turns` by this key is **OQ-3**.
- `adapter.parallel_gate_fallback` — no analogue; this engine has no defer trap because it has no
  defer (R10). Doc-comment the reason.
- `adapter.claude.cleanup_period_days` — Claude-lane retention, **not** this lane's. The analogous
  problem (a parked run's home must outlive the process) is real here; whether it wants its own key
  is **OQ-3**.
- Legitimately readable **by dotted name**: `limit.*`, `budget.background_window_fraction`,
  `canary.*`, `pressure.cache_read_weight`.

**New numbers land as structural constants with their reason in a doc comment**, flagged to the gate
under the `sseBatchSize`/`cancelGrace` precedent (§7/§9/§11/§61) — `cancelGrace`, `scanBufCap`,
stderr capture cap, the lowered `print_max_turns`/`print_wait_ceiling_s`/`bash_task_timeout_s`
defaults. Making any of them operator-tunable is an S00.9 amendment adding S18 rows and is **not
minted here**. A number that genuinely wants a registry row is an **OQ**, never an invented key.

---

## §6 · Files expected to change

**New** — `internal/adapters/kimicli/` (`kimicli.go`, `lower.go`, `parse.go`, `hook.go`,
`conformance_test.go`, `live_smoke_test.go`, `testdata/`);
`internal/adapters/opencode/lanedata/kimi-cli.json`;
`P3/measurements/2026-08-XX-kimi-cli-print-mode-spike.md`.

**Edited** — `internal/adapters/adapters.go`; `internal/adapters/opencode/lane.go` (substrate filter
+ package-comment correction); `internal/adapters/opencode/lanedata/kimi.json` (pool + the new
signal rows + the model-id regrade); `internal/shell/engineadapters.go` (third adapter, substrate
filter, `kimicli` root, `Signals` seam, cred dedupe); `internal/shell/shell.go` (registry/close
wiring); `internal/metering/planunits.go` + `plandata/kimi.json` (pool identity + pooled read);
`internal/metering/planbudgets.go` + the `internal/api` boundary (double-declaration refusal, per
OQ-2); `internal/watchlist/classify.go`, `seed.go`, `canary.go` (PaidLanes **only** per OQ-4);
`internal/conformance/registry.go` + `conformance_test.go`; `components.lock`;
`Spec/drafts/S00-front-matter.md`, `S03-engines-adapters.md`, `S16-adoption-manifest.md`;
`Spec/core-architecture-v1.md` (**regenerated only**);
`P3/measurements/2026-08-24-kimi-lane-gate-audit.md` (the R30 addendum); `P3/CONVENTIONS.md` (a new
§67); `P3/STATE.md` (queue row + counters + the F1 correction); `web/src/fixtures/api/*.json` **only
if mechanically forced**.

**Never touched:** `Docs/`, `Research/`, `P3/gates/lane-test-door.sh`,
`P3/gates/lane-key-ceremony.sh`, `P3/gates/B6-clickthrough.sh`, any adopted engine's source.

---

## §7 · Adopted component

One new adoption: **`@moonshot-ai/kimi-code@0.38.0`** — an external CLI, spawned per run, consumed
**UNMODIFIED** (S16.1 adopt-don't-fork: never vendored, never linked, never patched). S16.4
checklist, with §2 as its evidence:

| # | Check | Status |
|---|---|---|
| 1 | Niche & seam | PASS — a single engine behind the D3 seam (R3) |
| 2 | Organ-grade screen | PASS — no orchestration, session-ownership or store-of-record opinion is adopted; Sinet's store stays authoritative |
| 3 | Health check, primary sources | PASS with a **flag**: first-party (OIDC/SLSA-attested, moonshot.ai maintainer), MIT, actively developed — **70 versions in ~3 months** is a high-churn bus risk and the pin is fragile |
| 4 | License, path-scoped at the ref | **OWED BY THE EXECUTOR** — the root LICENSE is MIT at `main`; the scan MUST be redone **at tag/ref for 0.38.0** with a carve-out sweep, verbatim, dated |
| 5 | Capability claims verified behaviorally | **R0 spike + tier R** — and this checklist item is why the spike is not optional |
| 6 | Adopt-unmodified | PASS — config, env and argv only |
| 7 | Runner-never-store | PASS — `platform.db` stays the system of record |
| 8 | Exact pin + funeral plan | `0.38.0`; replacement = the `kimi` lane on opencode, same membership |
| 9 | Watch registration | R25 |
| 10 | Lock entry + approval | R27; approval = the 2026-08-26 order, **subject to OQ-0** |

---

## §8 · Acceptance checklist

| # | Check |
|---|---|
| A0 | **OQ-0 dispositioned and recorded** before any build step; A11's audit corrected (R30) |
| A1 | The R0 spike ran at **$0** against a loopback fake provider, its result file committed, all ten questions answered by execution |
| A2 | `@moonshot-ai/kimi-code@0.38.0` in `components.lock`, all S16.2 fields, license path-scoped **at the 0.38.0 ref** + dated, no `modules`/`npm_packages` claim, `lockgate` green, entry count exact |
| A3 | `kimicli.Pin` coupled to the entry by `TestPinMatchesLock`; tier R reports a pin↔installed delta LOUDLY |
| A4 | `SubstrateKimiCLI`/`LaneKimiCLI` exist; the "exactly two substrates" comment corrected and citing A12 |
| A5 | `internal/adapters/kimicli` implements `adapters.Adapter`; imports `internal/adapters` + stdlib only (source-scanned); no `opencode` import |
| A6 | Seven D3 verbs over the OQ-A transport; **stderr never merged into the JSONL**; the session id is the engine-REPORTED one and none is fabricated |
| A7 | Unknown frames logged-and-skipped as a **property**; one paid call = one `Usage`; `ResultText` follows the §60 order |
| A8 | Resume re-supplies the ENTIRE invocation, home rebuilt and re-verified; a resume that forgets any part is caught by test |
| A9 | The S03.5 channel table exists in code, each channel closed by a named knob, each proven by an attempt-the-leak test asserting the decoy did NOT take effect; the cwd `.kimi-code`/`.agents` takeover channel asserted absent before spawn; any channel with no knob reported by name |
| A10 | `[tools] disabled = ["Agent","AgentSwarm"]`; **R0-Q8's structural-vs-behavioral answer recorded**; wildcard/case/gated-tools/config-fragment re-admission refused by a named error, pinned as a property |
| A11 | `GatedTools` refused by name (`ErrGateParkUnsupported`), never auto-approved; `Park`/`Ask`/`GateFallback` left unset **with the reason in code**; `--yolo`/`--auto`/`--yes`/`--auto-approve`/`--dangerous-bypass-auth` never emitted |
| A12 | Per-RUN `KIMI_CODE_HOME` under a per-user 0700 root; HOME bounded; cross-run and cross-user leak/stray probes assert zero; parked-run homes outlive the process; `bin/` first-use cost measured and reported |
| A13 | Bounded run: `print_background_mode`, `print_max_turns`, `print_wait_ceiling_s`, `bash_task_timeout_s`, subagent/swarm timeouts all explicit; **R0-Q5 measured the process actually exits**; `KIMI_CODE_INFINITE_RETRY` never set; a spawn missing any of these is refused |
| A14 | `KIMI_CODE_NO_AUTO_UPDATE=1` + `auto_install=false` + `KIMI_DISABLE_TELEMETRY=1`; `upgrade` unreachable by construction; `agent-core-v2` asserted; `KIMI_CODE_IDENTITY_*` never set and scrubbed |
| A15 | Credential is `KIMI_MODEL_API_KEY` from profile `kimi-code`; `KIMI_MODEL_NAME`/`_BASE_URL`/`_PROVIDER_TYPE` are lowering, not credential; sentinel appears **exactly once** in the lowered env and nowhere in events, `run_events`, park records, digests, ops logs, the SQLite file **or its WAL**; one broker resolution per profile per spawn (R15a) |
| A16 | `lanedata/kimi-cli.json` loads and validates; substrate `kimi-cli`; distinct `provider_id`; empty `npm` with its note; **signal tables of both documents pinned EQUAL**; the five new strings added with the concurrent-limit trap carrying **no** documented class; C5 rider carried; no model id or endpoint is a Go constant |
| A17 | opencode's `ProvidersFor` **unchanged** by the new document while the platform-wide map DOES carry it — both directions |
| A18 | One placed `kimi-code` engine-cred ⇒ **both** kimi lanes commissioned at startup; the INFO line names both; nothing placed is byte-identical to the pre-packet world |
| A19 | `laneSubstrates["kimi-cli"] == "kimi-cli"`; a `kimi-cli` decision reaches the **kimicli** adapter at the REAL call sites, mutation-verified |
| A20 | **One pool:** `SeedPlanDocs()` still 2; `PlanDocFor("kimi-cli")` resolves to the `kimi` document; the reading aggregates across the pool and names the lanes; both lane documents declare the same pool; double-declaration handled per OQ-2 at all three layers; `oauth/usage` recorded `wired:false` and built on by nothing |
| A21 | Tiers F + R + L; skips print exactly `SANCTIONED SKIP (CONVENTIONS §10)`; tier L behind the existing `SINET_LIVE_SMOKE=1`, unreachable at landing; suites assert behavior, never help text |
| A22 | Registry rows added (substrate + lane), honest about both halves; row-count pins exact at **every** site |
| A23 | Watch rows for the CLI changelog **and `community-guidelines.html`**; `classify.go` lane enum carries `kimi-cli`; every moved count exact |
| A24 | A12 applied; assembled spec **regenerated** by the awk concat and byte-verified; marker sites annotated; ⚙ 118/33 and data surfaces 3 asserted unchanged |
| A25 | **$0 throughout** — no live provider call on any code or test path, no credential material committed, tier L never armed, canary legs never armed |
| A26 | Batteries green **serial** (`-p 1`): `gofmt`/`go vet`/`go build`, full `go test`, `lockgate`; vitest run if any `web/src` fixture moved |

---

## §9 · The CONVENTIONS constraints that bind

- **§10** — package map; `engine.<kind>` events through the S14.2 contract; paid-call bookkeeping in
  ONE `WriteTx`; three conformance tiers and the **one sanctioned skip class**; pin discipline (a
  delta is reported LOUDLY, never silently retargeted); credential hygiene; the ctl-dir airlock for
  any gate hook; the dev-mode confinement seam, which **nothing may widen**.
- **§12** — sandbox → adapters is the only import direction; **fail-closed** is the house rule (an
  inexpressible decision is refused loudly, never degraded to consent). R10 rests on this.
- **§61** — what a substrate package owes; structural constants flagged with reasons; contract
  shapes that are structurally unreachable are **left unset with the reason in code, never faked**;
  `-race` matters (an event-carried pointer belongs to its consumer the moment it is sent);
  ⚙-consumed-none is a **finding**, not a gap to paper over.
- **§62** — a lane is a DOCUMENT and the document is the unit of honesty; no endpoint or model id as
  a Go constant in a non-test file; credential-reference resolution is **measured, not assumed**;
  the endpoint self-check is an INPUT to the classifier, never a lookup inside it; registering an
  adapter changes posture and is **stated rather than implied**.
- **§63/§64** — exact numbers, never inequalities; a fixture claim that overstates its own innocence
  is the kind a reviewer stops checking, so **name every moved value**; DOCUMENTED-NOT-OBSERVED is a
  label the code wears; a record fetched *by* a tool is not the record *of* that tool; the closed
  documented-class vocabulary carries **no `auth` and no `balance` member by design**, and a data
  edit must never be able to suppress a lane freeze. R16's concurrent-limit ruling is this rule
  applied to a new string.
- **§65** — commissioning reads **disk truth**, never a startup probe; `broker.OpenStore`,
  `broker.NewServer` and `.Resolve(` stay banned from `internal/shell`'s non-test sources;
  STARTUP-BOUND with no reload seam invented; **selection and dispatch proven at the real call
  sites**, mutation-verified.
- **§66** — the honest absence pinned as a **property**, not a case; no dollars on a plan-unit path;
  a rule spelled twice drifts, so a refusal predicate is computed once and carried across seams.
- **Standing rule** — a backend packet moving `web/src` fixtures owes the frontend battery.
- **Serial tests, GPU-only inference** — `-p 1`; paid tests need explicit opt-in env; reap orphans.
  **R12 is this rule expressed as a lowering requirement.**

---

## §10 · Acceptance-test specifications

Written before any implementation exists. **They are specified, not committed as red tests, and the
reason is stated rather than left to be discovered:** the packet is gated on OQ-0 and its transport
is undecided pending R0, so a red test written against an argv the spike may overturn would fail
for the **wrong** reason — the one thing a red test must never do. The executor writes them red,
watches them fail for the right reason, then makes them pass.

| ID | Test | Setup | Assertions |
|---|---|---|---|
| T1 | `TestSubstrateConstantsAndA12Marker` | read `adapters.go` source | `SubstrateKimiCLI=="kimi-cli"`, `LaneKimiCLI=="kimi-cli"`; the source no longer claims "exactly two substrates" and cites `A12` |
| T2 | `TestPinMatchesLock` | parse `components.lock` | the `@moonshot-ai/kimi-code` entry's pin `== kimicli.Pin == "0.38.0"`; every S16.2 field non-empty; `license.checked` a valid date; no `modules`/`npm_packages` |
| T3 | `TestKimiCLILaneDocumentValidates` | `LoadLaneConfig(embedded kimi-cli.json)` | loads; `Substrate=="kimi-cli"`; `ProviderID != "kimi-for-coding"`; `NPM==""`; `Credential=={"kimi-code","KIMI_MODEL_API_KEY"}`; `DefaultModel=="k3"`; `EndpointMarker=="/coding/v1"` |
| T4 | `TestBothKimiLaneSignalTablesAreEqual` | both documents | the `(http_status, message_contains, documented_class)` triples are set-equal; a mutation to either fails the test |
| T5 | `TestConcurrentLimitRowFreezesRatherThanRetries` | `ExtractSignal` + `scheduler.Classify` on `403 "You've reached your concurrent request limit"` | verdict is Class 4 (freeze), **not** transient/depletion; the row carries **no** `documented_class`; a control (`403 "You've reached your 5-hour usage limit"`) still parks as depletion |
| T6 | `TestNoLaneConstantsInGoSources` (extend existing) | source scan | no endpoint or model id literal in the four scanned packages |
| T7 | `TestOpencodeEntriesUnchangedByCLILane` | production fill, one person holding `kimi-code` | `opencode.Adapter.ProvidersFor(who)` byte-identical to the pre-packet result; the platform-wide commissioned map DOES contain the CLI lane's provider id |
| T8 | `TestOnePlacedKeyCommissionsBothKimiLanes` | `t.TempDir()` broker store, one `engine-cred` under `kimi-code`, empty ciphertext | `commissionedLanes` contains `kimi` **and** `kimi-cli`; `laneSubstrates` maps each to its own substrate; the INFO line names both; nothing-placed control byte-identical to before |
| T9 | `TestCredResolvedOncePerProfilePerSpawn` | recording broker | exactly **one** `Resolve("kimi-code")`; the env carries `KIMI_API_KEY` **and** `KIMI_MODEL_API_KEY`, each **once** |
| T10 | `TestKimiCLIDispatchAtRealCallSites` | stage composed from the production fill; recording adapters; helper-spawn and revise sites | a `kimi-cli`-seated decision reaches the kimicli adapter; nothing-placed control reaches `claude-cli`; **mutation-verified** — neutering the fill collapses the two and fails |
| T11 | `TestGatedToolsRefusedOnKimiCLI` | `CompiledWorker{GatedTools:["Bash"]}` | `Start` returns `ErrGateParkUnsupported`; **no process is spawned**; the error text names the S03.4 reason |
| T12 | `TestNativeSpawnReadmissionRefused` | property over generated allowlists | any input naming `Agent`/`AgentSwarm` in any case, `enabled=["*"]`, `disabled=["*"]`, or a config fragment re-enabling them ⇒ `ErrNativeSpawnTool`; never a silent overwrite |
| T13 | `TestForbiddenFlagsNeverEmitted` | property over random `StartRequest`s | argv contains none of `--yolo`,`--auto`,`--yes`,`--auto-approve`,`--dangerous-bypass-auth`; `--output-format stream-json` present; `-p` present |
| T14 | `TestLoweredRunIsBounded` | inspect the lowered `config.toml` | `print_background_mode != "steer"`; `print_max_turns`, `print_wait_ceiling_s`, `bash_task_timeout_s`, subagent/swarm timeouts all explicitly set and finite; `KIMI_CODE_INFINITE_RETRY` absent; a spawn missing any ⇒ refusal before any process exists |
| T15 | `TestAutoUpdateAndTelemetryDisabled` | lowered env + `tui.toml` | `KIMI_CODE_NO_AUTO_UPDATE=1`, `KIMI_DISABLE_TELEMETRY=1`, `[upgrade].auto_install=false`; `KIMI_CODE_LEGACY_FLAG` absent; `KIMI_CODE_IDENTITY_*` absent from env **and** from non-test sources |
| T16 | `TestPerRunHomeIsolation` | two runs, two users | four distinct `KIMI_CODE_HOME`s; `HOME` bounded; after both, no file written outside them; no cross-read of a decoy planted in the other's home or in the real `$HOME/.agents/` |
| T17 | `TestConfigChannelDecoysDoNotTakeEffect` | one decoy per §2/R8 channel (project `.kimi-code/local.toml`, `.kimi-code/agents/agent.md` with `override:true`, `~/.agents/AGENTS.md`, a skills dir, an `mcp.json`, a plugin record) | each decoy is absent from the effective config, or the spawn refuses; **each assertion names its channel** |
| T18 | `TestUnknownFramesAreSkippedNotFatal` | property: random unknown `type` values injected into a fixture stream | no event minted outside the closed kind set; the session still reaches its Outcome; nothing fatal |
| T19 | `TestOnePaidCallOneUsageEvent` | fixture stream re-emitting one completed message N times | exactly one `Usage` with `Total==false`; run-total rides `engine.done`, never a checkpoint |
| T20 | `TestCredentialContainmentProperty` | full run against fixtures with a sentinel key | sentinel appears **exactly once** in the lowered env; scan every event payload, every `run_events` row, the park record, every digest input, the ops log, the SQLite file **and its WAL** — zero hits |
| T21 | `TestPlanDocResolvesBothKimiLanes` | `SeedPlanDocs()` | length **2**; `PlanDocFor("kimi")` and `PlanDocFor("kimi-cli")` return the **same** document; no document declares a `kimi-cli`-only allowance |
| T22 | `TestPooledConsumptionIsSummedNotSplit` | checkpoints on both kimi lanes for one person | the reading for **either** lane reports the **combined** consumption, names the pool and both lanes; the `zai` lane's reading is byte-identical to before (property over 200 fixed-seed draws) |
| T23 | `TestPoolBudgetDoubleDeclarationRefused` | declare on `kimi`, then on `kimi-cli`, same window | refused per OQ-2's disposition at **all three** layers (store, HTTP boundary, `ReadPlanUnits`), each naming the pool; a planted row does not steer dispatch |
| T24 | `TestBothLaneDocumentsDeclareTheSamePool` | both documents | same pool identity; both carry the verbatim shared-quota quote and its date |
| T25 | `TestKimiCLIConformanceTiers` | run the suite with and without the binary, with and without `SINET_LIVE_SMOKE` | tier F always runs; tier R absence prints exactly `SANCTIONED SKIP (CONVENTIONS §10)`; tier L prints its named skip and never spends |
| T26 | `TestConformanceSeedRowCounts` | `conformance.Seed()` | exactly 17 rows; `BumpGatingRows()` exactly 11; both new rows carry `engine_bump` |
| T27 | `TestKimiCLIWatchRowsVerified` | watchlist seed | the CLI changelog row and the `community-guidelines.html` row exist, carry a lane and a `verified_on`; the pinned lane-row count is exact |
| T28 | `TestClassifierGrammarCarriesKimiCLI` | `classify.go` source | the lane enum contains `kimi-cli` |
| T29 | `TestA12AmendmentLandedInBothCopies` | `S00` draft + assembled | the `\| A12 \|` row present in both **byte-identically**; carries `2026-08-26`, `kimi-cli`, `A11`, `no S18 re-sweep`, `118`; marker sites annotated in `S03` and `adapters.go`; S18 tally prose unmoved |
| T30 | `TestAssembledSpecReproducesFromDrafts` (existing) | — | still green after the regeneration |

---

## §11 · The A12 draft text

Appended to the S00.9 post-G4 changelog table in `Spec/drafts/S00-front-matter.md`, immediately
after the A11 row (currently line 191, the file's last line), as **one table row on one line** —
A11's exact form. `<OPERATOR GATE-A RULING>` is the **only** placeholder and is replaced with the
operator's own words from OQ-0 before the row is written; the row is not applied while it is empty.

```
| A12 | 2026-08-26 | **S03.2's "v0 ships exactly two substrates carrying three lanes" grows a THIRD substrate — `kimi-cli`, Moonshot's first-party Kimi Code CLI (`@moonshot-ai/kimi-code`, npm, MIT, pin 0.38.0 EXACT) — on explicit operator order (2026-08-26).** The operator holds a Kimi Code membership that serves BOTH the API endpoint `https://api.kimi.com/coding/v1` (lane `kimi`, opencode substrate, added at A11) AND the first-party CLI, **from one shared quota pool** — vendor verbatim, captured 2026-08-26: *"Kimi Code is the developer-focused AI coding service within Kimi membership benefits, provided together with a Kimi membership subscription and sharing the same quota. Requests from the CLI, VS Code, and third-party tools all count toward that quota."* The order is to run K3 by both paths so the operator can measure which performs better, which is a comparison the platform can only make honestly if the second path is a real substrate rather than a relabelled lane. **The rider-3 mechanism is NOT what governs here and the difference is stated rather than blurred:** S03.6's checklist adds LANES config-only and explicitly "never a new substrate", so a third substrate is a genuine amendment to S03.2's pin, not an onboarding. What rides the checklist is the lane `kimi-cli` (a lane DOCUMENT: substrate `kimi-cli`, the same `kimi-code` broker profile, credential variable `KIMI_MODEL_API_KEY` — the CLI does not read `KIMI_API_KEY` from the shell — and the same membership pool, declared ONCE and never as two). **Gate A was RE-AUDITED at this amendment and the re-audit is the reason this entry exists in the form it does:** the 2026-08-24 audit passed Gate A as class 2 without reading `https://www.kimi.com/code/docs/en/kimi-code/community-guidelines.html`, which states verbatim *"Kimi Code subscriptions are for interactive use only"* and *"Don't use Kimi Code for non-interactive automation"* — S03.6's class-3 language, which its own pre-registered would-change trigger names and answers with "immediate re-audit and lane freeze". That text binds the EXISTING `kimi` lane identically. Operator ruling, 2026-08-26: <OPERATOR GATE-A RULING>. Evidence: `P3/measurements/2026-08-24-kimi-lane-gate-audit.md` (with its 2026-08-26 Gate-A addendum) and `P3/measurements/2026-08-26-kimi-cli-print-mode-spike.md` (the $0 loopback capture that establishes the engine's real flag surface, stream envelope, boundedness and native-spawn disable — the 2026-08-26 coordinator sweep's `--print`/`--afk`/`--input-format`/`--final-message-only` list does not exist at this pin and is superseded). Sole-controller posture UNCHANGED and structural: `[tools] disabled = ["Agent","AgentSwarm"]`; S03.4 parking is **structurally unavailable** on this substrate (print mode requests no approval and hooks are fail-open), so a gated-tool worker is REFUSED by name rather than auto-approved, and that limitation is written on the lane document and the conformance row. The metered-exception list stays **EMPTY** [G1 P7]. No ⚙ setting's default or clamp is touched — the lane's numbers live on the S18.3 data surfaces with no dotted key → **no S18 re-sweep**; the S18 tally stays **118 keys / 33 domains** and the data-surface count stays **3**. Marker sites annotated: S03.2 substrate count, S03.6 lane roadmap, the `adapters.go` substrate-const comment, and the S16 lane-onboarding manifest (second row). | operator, 2026-08-26 order; Gate-A ruling same date; presented for veto at the LN gate batch |
```

**Marker-site edits A12 claims** (each asserted by T29):
1. `Spec/drafts/S03-engines-adapters.md` §S03.2 — the sentence *"v0 ships exactly two substrates
   carrying three lanes"* gains its A12 annotation and the third substrate.
2. `Spec/drafts/S03-engines-adapters.md` §S03.6 — the lane roadmap gains `kimi-cli`, noting that a
   lane addition normally never forces a substrate and that this one did, by amendment.
3. `internal/adapters/adapters.go` — the substrate-const block comment (R1).
4. `Spec/drafts/S16-adoption-manifest.md` — a second row in the lane-onboarding record table.

---

## §12 · Open questions — coordinator disposition required

**OQ-0 · BLOCKING · Does this lane proceed at all, given F0?** The operator's Gate-A ruling on
*"Kimi Code subscriptions are for interactive use only"* / *"Don't use Kimi Code for non-interactive
automation"*, which under S03.6 is class 3 (auto-disqualifying) and fires the A11 audit's own
pre-registered freeze trigger — **against the already-commissioned `kimi` lane as well as this new
one**. Both readings are laid out in §1 F0. The named alternatives: **(i)** proceed on a recorded
operator acceptance of the gray zone (the posture already taken for the Anthropic lane's own
gray-zone note at G1 P2), **(ii)** freeze both kimi lanes pending clarification from Moonshot,
**(iii)** narrow to interactive-only use (the operator drives Kimi Code themselves; Sinet does not
route to it), **(iv)** an Open Platform metered lane — which needs its own amendment, its own Gate
A–C audit and a G1 P7 exception, and is **not** available as a quiet substitution.
**Grounding's recommendation: present at the LN gate batch beside A12's veto, before the door run
makes any live call, and let the operator rule.** This brief takes no position on their risk.

**OQ-A · Which transport?** Three exist and the spec is silent because it never anticipated this
engine. **(1) `kimi -p --output-format stream-json`** — per-run process, matches D3's per-run shape
and the operator's "similar to claude code", honors a per-run `Confiner`; but the envelope is
undocumented and **may carry no usage numbers**, which D7/S10.1 require. **(2) `kimi acp`** —
JSON-RPC over stdin/stdout, per-run, and **the protocol S03.1 explicitly shapes the D3 verbs after**
(*"a strict superset of ACP's stable verb set so a future transport convergence is a swap, not a
re-architecture"*); whether ACP carries usage is unknown and would need a second capture.
**(3) `kimi web`** — per-user loopback HTTP+WebSocket with a **documented** event protocol,
**documented usage numbers**, and a live quota readback; architecturally the opencode shape, so §61's
lessons transfer wholesale — but it re-imports §61's per-user-server tension (config startup-bound,
per-run `Confiner` unsupported, the cancel ladder ending the user's instance).
**Recommended disposition: (1), conditional on R0-Q2.** If the spike finds usage in the `-p` stream,
(1) wins on every other axis. If it does not, **(3)** is the fallback and the reason is not
preference but D7: a lane that cannot write a usage row cannot write a checkpoint. Decide on the
spike, not on this paragraph.

**OQ-1 · Where does the lane-document machinery live?** Recommended: **keep it in
`internal/adapters/opencode`** and correct the package comment to say it serves every substrate —
the type is already substrate-agnostic and every consumer is substrate-blind, so moving it is a wide
diff with no behavior gain. Alternative: extract to `internal/adapters/lanedoc` with thin aliases,
which reads better and touches a lot. Named so the executor does not decide a package boundary alone.

**OQ-2 · How is a double-declared pool budget refused?** `plan_budgets` is keyed
`(user_id, lane, window)`; two lanes now share one allowance. Recommended: **refuse the second
declaration, naming the pool's canonical lane**, enforced at the three layers LN-6 established (the
`PlanBudgetWindowRefusal` precedent — the store computes the verdict, the boundary carries it, and
`ReadPlanUnits` refuses to apply a planted row). Alternatives: **(i)** upsert both lanes' rows to
one value (invisible coupling, and the operator cannot tell why their edit moved a row they did not
touch); **(ii)** key budgets by pool rather than lane (correct, and a migration on a table that
landed five days ago). The recommendation keeps the honest half and names the consumer.

**OQ-3 · Two numbers that may want registry rows, neither minted here.** (a) Should
`CeilingSteps × ⚙ adapter.engine_ceiling_backstop_mult` drive the lowered `print_max_turns`? It is
the closest thing this engine has to an engine belt, and using it would give S03.4's ceiling ordering
a real meaning here — but `print_max_turns` counts background-steered turns, not model calls, so the
mapping is not obviously the same quantity. (b) A parked run's `KIMI_CODE_HOME` retention horizon is
the same problem `adapter.claude.cleanup_period_days` solves on the Anthropic lane; does it want its
own key, or is a structural constant honest? Both would be S00.9 amendments adding S18 rows.
**Recommended: structural constants with their reasons, flagged to the gate** (the §61 precedent).

**OQ-4 · Does `kimi-cli` join `watchlist.PaidLanes()`?** It is a hardcoded list of three; a fourth
takes the disarmed-leg count from **9 to 12** at four pinned sites and, when armed, **doubles the
real-request canary spend on one shared pool** for answers that are properties of the membership
(auth sanction, the account's model list), not of the client path. **Recommended: NO at LN-7** — the
`kimi-cli` document records that its canary coverage is the `kimi` lane's, by name. The counter-case
is real and conditional on the credential design: if the CLI path could ever be entitled or revoked
independently of the API key, it needs its own auth canary. Under R15 (one profile, one Console key)
it cannot be. Revisit if that changes.

**OQ-5 · Is `kimi web`'s `GET /api/v1/oauth/usage` worth a follow-up packet?** It would give this
platform its **first provider-signaled observed overlay outside the Anthropic wire** — per-window
`used`/`limit`/`reset_at`, D4-clean, and the thing that could finally populate the deliberately-empty
`reset_marker` and close A11 audit U4. **Not this packet** (S10.4's denominator stays the operator's
budget, and building an overlay with no consumer is the class §63 D2 exists to close). Named so the
finding is not lost.

---

## §13 · What must NOT land

- **No build before OQ-0.** The gate is the first requirement, not a formality.
- **No flag from the STATE row's fabricated list** (F1). If it is not in §2(2), it does not exist.
- **No door, ceremony or walk changes** — LN-8 owns them; `B6-clickthrough.sh` is never touched.
- **No hook-based security barrier.** Hooks are fail-open by design and the vendor says so; the
  brake is `[permission] deny` + the `[tools]` allowlist.
- **No silent auto-approval of a gated tool.** Refuse by name.
- **No `KIMI_CODE_IDENTITY_*`, ever** — the guidelines forbid altering client identity.
- **No `KIMI_CODE_INFINITE_RETRY`** — engine-native retry is never the policy layer (S10.5).
- **No Open Platform metered key as a quiet route around F0** — that is a different lane, a G1 P7
  exception and its own audit.
- **No S11.5 injection proxy** — deferred; record the wire facts and stop.
- **No per-lane data-policy enforcement** — the C5 rider stays RECORDED-NOT-ENFORCED and still needs
  an **affirmative operator yes**, not merely an absence of objection.
- **No new ⚙ key**, no new registry row, no S18 sweep.
- **No seeded price rows, no invented allowance figure** — a number this platform cannot source is
  the denominator D4 forbids by name, and an operator would accept it *because it looked sourced*.
- **No promotion of an UNVERIFIED row to verified** by copying it between documents.
- **No engine patch, no vendoring, no fork** — ever (S16.1).
- **No paid call.** The R0 spike is $0 against a loopback fake; the operator's own door run stays the
  first live call, and under F0 it may not happen at all.
