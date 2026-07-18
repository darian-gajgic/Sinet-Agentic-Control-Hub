# Spike P2-S5 — engine lowering: config-isolation + no-native-spawn

**Date:** 2026-07-18 · **Closes:** R15-OQ6 (engine-lowering probes) · **Gate:** P2-entry spike battery
**Engines under test (pinned, both installed globally):** Claude Code **2.1.214** (`~/.npm-global/bin/claude`, a compiled ELF `claude.exe`), opencode **1.18.3** (`~/.npm-global/bin/opencode`).
**Spend:** **$0.0915** Claude API-equivalent (3 haiku probe turns under subscription auth; no metered spend). opencode lane **$0.00** (all auth-free). Source-grep subagent **$0.00**.

Relaunch of a paused run (prior run confirmed settings-isolation but stopped mid-verify). Started fresh; this run re-establishes every probe from scratch and extends into the config channels the prior run had not reached.

---

## Scope (R15-OQ6)

Verify, on the pinned versions, that **Sinet-compiled config is the ONLY config an engine sees**, and that **engine-native subagent spawning can be fully disabled** — so D6 orchestration stays Sinet-owned (G1 sole-controller). Five probes: Claude-lane agent-injection path (1) and settings-isolation (2); opencode-lane agent registration (3), `.claude/` cross-read disable (4), and `permission.task` deny-all spawn kill (5). Extends G1-S1 (Claude `--settings` findings) and G1-S3 (opencode XDG isolation); consumes R15 §2.1/§4.1 (guardrail split, compile-down) as settled.

**Headline:** all five CONFIRMED. The load-bearing discovery: on **both** lanes, isolating *settings* is necessary but **not sufficient** — MCP servers, skills, tools, cross-read `.claude/` instruction files, and cwd project-config are **separate channels** that bleed the ambient/operator environment into a worker unless each is explicitly closed. R15 §4.1's "explicit `--settings`/`settingSources` isolation" is correct but under-specified; the full per-lane recipe is in §"Adapter requirements".

---

## Method

- **Claude lane (live, cheap):** three headless `claude -p` runs, `--model claude-haiku-4-5 --max-turns ≤2 --max-budget-usd 0.10`, `--output-format stream-json --verbose --include-hook-events`. Nested-session env markers (`CLAUDECODE`, `CLAUDE_CODE_*`, `AI_AGENT`, `CLAUDE_EFFORT`, `CLAUDE_PID`) stripped per invocation. **Decoys:** a project `.claude/settings.json` in a scratchpad launch cwd (distinct sentinel SessionStart + PreToolUse-deny hooks writing marker files) and the operator's **real** `~/.claude/settings.json` (SessionStart brief + PreToolUse sudo-gate) used as the natural user-level decoy — never overwritten. An "isolated" settings file with its own sentinel hooks stands in for Sinet-compiled config. Run A = isolated (probe 1 + SessionStart isolation); Run B = isolated with the full flag recipe (probe 2 + tools/MCP/skills lockdown); Run C = ambient control, no isolation flags. Which config fired is proven by (a) marker files, (b) verbatim PreToolUse deny-reason strings, (c) `system/init` `tools`/`mcp_servers`/`slash_commands`, (d) presence/absence of the operator brief.
- **opencode lane (auth-free):** an isolated XDG env (`XDG_{CONFIG,DATA,STATE,CACHE}_HOME` under scratchpad) with a Sinet-owned `opencode.jsonc`; a launch cwd salted with `.claude/agents/`, `.claude/skills/`, a non-`.claude` skill, and (later) an adversarial `opencode.json`. Resolved state read with the binary's own inspectors — `opencode debug config`, `opencode agent list`, `opencode debug skill`, `opencode debug agent <name>`, `opencode debug paths` — plus env-var toggles. No provider credential exists on this host (`opencode auth list` → 0), so no paid turn ran.
- **Source corroboration:** the opencode npm binary is a compiled Bun ELF (no readable TS), so the **`anomalyco/opencode` v1.18.3** source tag (commit `127bdb3…`, `package.json` version = 1.18.3) was fetched read-only and grepped (adopt-don't-fork honored; nothing executed from it). Claude's isolation levers were confirmed by `strings` on `claude.exe`.
- **Hygiene:** user-space only; isolated dirs in scratchpad; no accounts; no secrets printed; no `auth.json` created (verified absent); opencode's CLI auto-spawns an ephemeral serve per command — all were shut down, port free at end.

---

## Findings

### Probe 1 — Claude artifact injection path — **CONFIRMED**

**Mechanism:** a compiled agent is injected **per invocation on the command surface**, no project `.claude/agents/` file required. `claude --help`:

> `--agents <json>  JSON object defining custom agents (e.g. '{"reviewer": {"description": "...", "prompt": "..."}}')`
> `--agent <agent>  Agent for the current session. Overrides the 'agent' setting.`

Run A passed `--agents '{"sinet_probe":{"description":"...","prompt":"You are SINET-PROBE-AGENT. When asked your codename, reply EXACTLY with: KESTREL-9 ..."}}' --agent sinet_probe` and asked for the codename. Evidence:

- `system/init` `agents` = `['claude','Explore','general-purpose','Plan','sinet_probe','statusline-setup']` — the inline agent is registered.
- `result` = `'KESTREL-9'`, `is_error: False` — the injected prompt drove the session (the main session ran **as** the agent). Cost $0.036746.

Supporting levers on the same surface (all in `--help`, `strings`-confirmed): `--append-system-prompt`/`--system-prompt` (prompt body), `--tools`/`--allowedTools`/`--disallowedTools` (toolset), `--mcp-config` (connectors), `--settings` (hooks/permissions as JSON string or file). So a full compiled worker lowers to flags + inline JSON with **zero writes to any shared project dir**. (`--agents` is inline JSON; nothing touches disk.)

### Probe 2 — Claude `--settings` + setting-source isolation — **CONFIRMED (with a critical channel gap)**

**Mechanism:** the CLI equivalent of the SDK's `settingSources: []` is **`--setting-sources <user,project,local>`**; pass it **empty** to load none, and `--settings <file>` to supply the one Sinet file. Verified airtight for the settings/hooks/CLAUDE.md channel — **and** the mirror-image control proves the decoys otherwise bleed in:

| Signal | Run A / B (isolated: `--settings iso.json --setting-sources ""`) | Run C (ambient control, no flags) |
|---|---|---|
| PreToolUse deny reason on `echo hello` | `SINET-ISOLATED-DENY` ×4, zero project-decoy | `SINET-PROJECT-DECOY-DENY` ×4 |
| SessionStart hook that fired | isolated only (`hook_response … 'SINET-ISOLATED-SESSIONSTART…'`) | project decoy + operator brief |
| Operator brief (`AUTO-LOADED OPERATIONAL BRIEF`) injected | **absent** (grep = 0) | present (grep = 1) |
| Marker files written | only `ISOLATED-*` | only `PROJECT-DECOY-*` |

So the planted user- and project-level decoys **did not take effect** under isolation and **did** without it — the prior partial finding reproduces and is now nailed both directions.

**The critical gap (new, load-bearing):** `--setting-sources ""` gates *settings files only*. Run A (settings-isolated but no other flags) still inherited ambient config through **three other channels**: `system/init` showed `mcp_servers` = the operator's `claude.ai Google Drive / Calendar / Gmail` connectors, `slash_commands` count 43 (operator skills), and `tools` count 29 **including `Task`** and non-vanilla harness tools (`CronCreate`, `DesignSync`, `RemoteTrigger`, `Workflow`, `Monitor`, `SendMessage`, …). Run B closed all three with the compound recipe — `--strict-mcp-config` (→ `mcp_servers: []`), `--disable-slash-commands` (→ `slash_commands: 0`), `--tools "Bash,Read"` (→ `tools: ['Bash','Read']`, **`Task` and the whole `Task*` family gone**). `strings claude.exe` confirms `CLAUDE_CONFIG_DIR` is honoured (a clean-room alternative — see §requirements) and shows no `CLAUDE_CODE_EXTRA_TOOLS`-class env inject (the bleed was ambient config, not env). **No-native-spawn (Claude):** the runtime spawn tool is **`Task`** (plus `TaskCreate/Get/List/Output/Stop/Update`); the `--tools` **allowlist** (default-deny) provably excludes it (Run B), which is stronger than enumerating `--disallowedTools`.

### Probe 3 — opencode agent registration per-invocation under isolated XDG — **CONFIRMED**

**Mechanism:** agents are defined **in config keyed by name** and selected per turn **by name reference**. `opencode debug paths` confirmed every path resolved inside the scratchpad XDG tree (`home` stays real). An agent defined in the isolated `opencode.jsonc` (`agent.sinet_probe`, `mode:"primary"`, `permission.task:"deny"`) resolved cleanly: `opencode agent list` → `sinet_probe (primary)`; `opencode debug config` → `agents in resolved config: ['sinet_probe']`. **Per-invocation without a shared file:** `OPENCODE_CONFIG_CONTENT='{…}'` (inline final-merge) injected `sinet_injected` into an otherwise-empty config → `resolved agents: ['sinet_injected']` — the opencode analog to Claude `--agents`.

Source (v1.18.3): config dirs scanned are `~/.config/opencode` (XDG), project `.opencode` (walked up, gated by `OPENCODE_DISABLE_PROJECT_CONFIG`), home `.opencode`, and `OPENCODE_CONFIG_DIR` (`config/paths.ts:23-41`); agents come from the `opencode.json` `agent` map or `{agent,agents}/**/*.md` (`config/agent.ts:11-32`) — **never `.claude`**. The HTTP `POST /session/{id}/prompt_async` body's `agent` is `Schema.optional(Schema.String)` resolved by `agents.get(name)` (`session/prompt.ts:1423,1503`); **there is no inline agent *definition* per request** — it must exist in the config the server loaded at startup (so Sinet injects via the isolated file or `OPENCODE_CONFIG_CONTENT`/`OPENCODE_CONFIG_DIR`, then references by name). Fields: `description` optional, `mode ∈ {primary,subagent,all}`, `tools` **deprecated → `permission`**.

### Probe 4 — opencode `.claude/` cross-read disable — **CONFIRMED (var identified; premise refined)**

R15's search-level `OPENCODE_DISABLE_CLAUDE_CODE` note is **correct that the var exists**, but the premise needed three corrections, all verified:

1. **opencode never reads `.claude/agents`.** The `.claude/agents/claude_decoy.md` decoy did **not** register (`debug config` → `claude_decoy present in agents?: False`). Source: exhaustive grep finds no `.claude/agents` read; the skills test doc says skills-only. So agent-level cross-read is a non-issue.
2. **The cross-read that IS real is skills (and CLAUDE.md).** The `.claude/skills/claude_decoy_skill` decoy **leaked** (`debug skill` → `claude_decoy_skill`), while a non-`.claude` cwd skill did **not** — opencode scans `.claude/skills` specifically, not arbitrary cwd dirs. Source `skill/index.ts:21-23,186-202` reads `~/.claude/skills` **and** project `.claude/skills`; `session/instruction.ts:62,66` reads `~/.claude/CLAUDE.md` **and** project `CLAUDE.md`. Because XDG isolation leaves **HOME unchanged**, the operator's real `~/.claude/CLAUDE.md` (present, 1497 B) would bleed into every worker's instructions absent the switch.
3. **The switch, verbatim (`effect/runtime-flags.ts:23-30`):** `OPENCODE_DISABLE_CLAUDE_CODE` is the **broad** kill for both the CLAUDE.md prompt read and the `.claude/skills` scan; granular `OPENCODE_DISABLE_CLAUDE_CODE_PROMPT` / `OPENCODE_DISABLE_CLAUDE_CODE_SKILLS`; `OPENCODE_DISABLE_EXTERNAL_SKILLS` additionally covers `.agents/skills`. **ENV-gated only** (no `opencode.json` key). Empirically, `debug skill` dropped `claude_decoy_skill` under `OPENCODE_DISABLE_CLAUDE_CODE=1`, `…_SKILLS=1`, and `…_EXTERNAL_SKILLS=1` alike (baseline `['customize-opencode','claude_decoy_skill']` → `['customize-opencode']`).

**Adjacent leak found & closed:** opencode "walks up from cwd" for a project `opencode.json`. A planted adversarial `cwd/opencode.json` (`permission.task:"allow"`, agent `evil_cwd`) **bled in** at baseline (`agents:['evil_cwd'] | permission:{"task":"allow"}` — i.e. it could **re-enable spawning**). `OPENCODE_DISABLE_PROJECT_CONFIG=1` fully ignored it (`agents:[] | permission:null`). Sinet's real combo (`OPENCODE_DISABLE_PROJECT_CONFIG=1` + `OPENCODE_CONFIG_CONTENT` inject + `OPENCODE_DISABLE_CLAUDE_CODE=1`) yields exactly `agents:['sinet_injected'] | permission:{"task":"deny"}` — nothing else.

### Probe 5 — opencode `permission.task` deny-all disables spawning — **CONFIRMED (config + source); live turn BLOCKED on auth**

**Mechanism (blanket `task:"deny"` → tool removed from the model's toolset, pre-inference).** `opencode debug agent` exposes the resolved per-agent `tools` map; `permission.task` deterministically drives `tools.task`:

| agent | `permission.task` | resolved `tools.task` | `tools.bash` |
|---|---|---|---|
| `sinet_deny` | `deny` | **`false`** | `true` |
| `sinet_allow` | `allow` | `true` | `true` |
| `build` (built-in) | (default) | `true` | `true` |
| `sinet_injected` (env-injected) | `deny` | **`false`** | `true` |

So deny is **surgical** — only the spawn tool goes, other tools untouched. Source confirms the path and that it is bypass-proof: a blanket `"*":"deny"` is flagged by `disabled()` (`permission/index.ts:204-214`) and **stripped by `resolveTools` before the LLM request** (`session/llm/request.ts:208-214`) — the model never receives a `task` tool. A *pattern-scoped* deny instead keeps the tool and rejects each disallowed call with a `DeniedError` tool-result ("The user has specified a rule which prevents you from using this specific tool call…", `core/src/v1/permission.ts:24-26`). No model-reachable bypass: `bypassAgentCheck` can't re-add a stripped tool and is not model-settable (`session/prompt.ts:1223,331`); spawned subagents inherit `task:"deny"` (`agent/subagent-permissions.ts:18,25`); and `subagent_depth` defaults to **1** (`tool/task.ts:111-117`).

**Verdict CONFIRMED:** deny-all removes the spawn capability structurally — a stronger guarantee than a behavioral "it didn't spawn," because the tool is provably absent from the toolset the model is handed. The optional live-turn belt-and-suspenders (drive a turn, watch no spawn + observe the tool-result) is **BLOCKED**: this host has 0 opencode provider credentials (operator steps below).

---

## What Sinet's adapter must do per lane (config-isolation + no-native-spawn)

**Claude lane (2.1.214) — per-run headless invocation:**
1. Inject the compiled agent inline: `--agents '<json>'` + `--agent <name>` (prompt body in the agent's `prompt`, or `--append-system-prompt`). No project `.claude/agents/` write.
2. `--settings <sinet-compiled.json>` — the one settings file (hooks/permissions).
3. `--setting-sources ""` — load **no** user/project/local settings.
4. `--strict-mcp-config` (+ `--mcp-config` only for Sinet's own servers) — kill ambient MCP/connectors.
5. `--disable-slash-commands` — kill ambient skills/commands (ship Sinet skills as Agent Skills dirs the worker is pointed at, not via discovery).
6. `--tools "<explicit allowlist>"` — default-deny toolset; **must exclude `Task` and `TaskCreate/TaskGet/TaskList/TaskOutput/TaskStop/TaskUpdate`** → no native subagent spawn.
7. Scrub nested-session env (`CLAUDECODE`, `CLAUDE_CODE_*`, `AI_AGENT`, `CLAUDE_EFFORT`, `CLAUDE_PID`) from the child.
8. Hash body+config together per run (R15 §4.1; the compiled artifact was reproduced here from flags).
9. *Clean-room alternative for the API-key lane:* `CLAUDE_CONFIG_DIR=<per-run dir>` relocates the whole config root (`~/.claude.json` MCP registry + `~/.claude/` skills/plugins/settings). **Not** for the subscription lane — it also relocates credential lookup (keychain/`~/.claude/.credentials.json`); `--bare` is stronger still but forces `ANTHROPIC_API_KEY`-only auth (no OAuth/keychain).

**opencode lane (1.18.3) — per-user `serve` under isolated XDG:**
1. Fully disjoint `XDG_{CONFIG,DATA,STATE,CACHE}_HOME` per user, set on **every** invocation (G1-S3). Bind `127.0.0.1`; set `OPENCODE_SERVER_PASSWORD`; never expose `POST /global/upgrade`.
2. Supply the compiled agent + `permission` map via the isolated `opencode.jsonc` **or** inline `OPENCODE_CONFIG_CONTENT='<json>'` (no shared file); select per turn with the `agent` **name** in `prompt_async`.
3. `OPENCODE_DISABLE_CLAUDE_CODE=1` — kill all `.claude` cross-reads (`~/.claude/CLAUDE.md` + project `CLAUDE.md` instruction reads and `~/.claude/skills` + project `.claude/skills` scans). Required because HOME is unchanged under XDG isolation. (`.claude/agents` is never read.)
4. `OPENCODE_DISABLE_PROJECT_CONFIG=1` — ignore any `opencode.json`/`.opencode` walked up from the launch cwd (an adversarial repo config can otherwise re-enable `task`/add agents/MCP).
5. `OPENCODE_PURE=1` (or `--pure`) — no external plugins; `OPENCODE_DISABLE_DEFAULT_PLUGINS=1` to drop built-ins too.
6. `permission.task: "deny"` (blanket, per-agent and top-level) — removes the `task` tool → no native spawn. (`subagent_depth`=1 and inherited-deny are extra belts.)

**Cross-lane consequence for the spec:** "compiled config is the only config" is a **compound** guarantee — one flag/var per channel (settings, MCP, skills, tools, cross-read instructions, cwd project-config, env). R15 §4.1's guardrail split holds; this spike supplies the exact per-version knobs and adds cwd-project-config + HOME-relative cross-reads as channels the design must name.

---

## Blocked items — operator steps

**Probe 5 live-turn behavioral confirmation** (optional; the structural verdict is already CONFIRMED). Needs one opencode provider credential; sanctioned path is the operator's Z.AI GLM Coding Max plan (whitelisted for opencode, R01). Reuse the G1-S3 provisioning steps, then run under the spike's isolated `XDG_DATA_HOME`:
1. At `https://z.ai` → GLM Coding Plan dashboard → copy the coding-plan API key.
2. `XDG_DATA_HOME=<spike-data-dir> opencode auth login` → select **"Z.AI Coding Plan"** (`zai-coding-plan`) → paste. Verify: `opencode auth list` → `1 credential`.
3. Tell the runner the XDG env used. Test (≤$0.20): start `opencode serve` with `permission.task:"deny"`; prompt a task that would delegate; confirm `agent list`/turn shows no `task` tool offered and no subagent session is created. Delete the `auth.json` afterward.

---

## Cost

| Probe | Lane | Turns | API-equivalent |
|---|---|---|---|
| 1 (Run A) | Claude, haiku | 1 | $0.036746 |
| 2 (Run B, isolated) | Claude, haiku | 2 | $0.024906 |
| 2 (Run C, control) | Claude, haiku | 2 | $0.029806 |
| 3,4,5 | opencode | 0 (auth-free `debug`) | $0.0000 |
| source grep | subagent | — | $0.0000 |
| **Total** | | | **$0.0915** |

Figures are the engine's own `total_cost_usd` under subscription auth; no metered spend occurred. Under the $0.10/probe cap. No `auth.json` created; all opencode serve instances shut down; ports free.
