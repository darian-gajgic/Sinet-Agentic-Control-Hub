# G1-S1 — CLI-wrap vs Agent SDK for the Anthropic-lane adapter (empirical spike)

**Spike date:** 2026-07-17 · **Answers:** Research/01 open question 3 (R01-OQ3) · **Status:** COMPLETE, no blocked items · **Total measured spend:** $0.1043 API-equivalent (cap was $0.50)

---

## 1. Purpose

R01 §4 settled the Anthropic lane as a first-party wrap but held one implementation choice open inside the adapter (R01 §3-D, OQ3): **(A) wrapped `claude -p` CLI vs (B) Claude Agent SDK**, to be decided on spike evidence. R05 §2.2/§4.12 added the load-bearing empirical question: **resume-cache fidelity** — issue #39732 ("prompt caching disabled for SDK `query()`/V2 sessions", closed not-planned) was cited as the live data point favoring the CLI (P-T02-2). This spike measures both surfaces against the adapter-contract needs from R01 §4's verb table: structured per-paid-call usage (D7 checkpoint rows), session resume, cache fidelity across resume, PreToolUse gating, and — gate condition — whether the SDK rides subscription auth at all.

## 2. Method

- **Versions:** `claude` CLI v2.1.212 (`~/.npm-global/bin/claude`); `@anthropic-ai/claude-agent-sdk` v0.3.212, npm-installed **locally** in the spike scratchpad (never `-g`). The SDK pulled `@anthropic-ai/claude-agent-sdk-linux-x64` v2.1.212 as its engine (see F6). Date: 2026-07-17.
- **Environment:** operator's existing subscription login only; **no `ANTHROPIC_API_KEY` present in the environment** (verified before each SDK probe with `env -u ANTHROPIC_API_KEY`); nested-session env markers (`CLAUDECODE`, `CLAUDE_CODE_ENTRYPOINT`, `CLAUDE_CODE_SESSION_ID`, `CLAUDE_CODE_CHILD_SESSION`, `CLAUDE_CODE_EXECPATH`, `AI_AGENT`, `CLAUDE_EFFORT`) stripped from all probe invocations. Probes ran from scratchpad cwds (`…/spike-s1/work` and `…/spike-s1/work-sdk`), not the repo. No global settings touched; hooks injected per-invocation only.
- **Budget rules applied:** every probe `--max-budget-usd 0.05` (CLI) / `maxBudgetUsd: 0.05` (SDK), `--max-turns ≤ 2`, tiny prompts, `--model claude-haiku-4-5` throughout (cache read/creation fields are API-level and model-independent; the default model was not needed).

**Exact commands (representative; full logs in spike scratchpad `spike-s1/logs/`):**

```bash
# CLI probe 1 — headless, control-plane-chosen session id
claude -p "Reply with exactly: OK" --output-format stream-json --verbose \
  --model claude-haiku-4-5 --max-turns 1 --max-budget-usd 0.05 --session-id <uuid>

# CLI probe 2 — resume + continuity + cache
claude -p "What exact word did I ask you to reply with in my first message? Answer with that word only." \
  --output-format stream-json --verbose --model claude-haiku-4-5 --max-turns 1 \
  --max-budget-usd 0.05 --resume <uuid>

# CLI probe 3 — PreToolUse gate via per-invocation settings file
claude -p "Run the bash command 'echo hello' and report its output." \
  --output-format stream-json --verbose --model claude-haiku-4-5 --max-turns 2 \
  --max-budget-usd 0.05 --settings settings-deny.json --session-id <uuid>
# settings-deny.json: {"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"deny.sh"}]}]}}
# deny.sh logs stdin JSON, emits {"hookSpecificOutput":{"hookEventName":"PreToolUse",
#   "permissionDecision":"deny","permissionDecisionReason":"Sinet spike S1 gate: denied by control-plane hook"}}
```

```js
// SDK probes 4/5/6 — query() with: {cwd, model:'claude-haiku-4-5', maxTurns, maxBudgetUsd:0.05},
// probe 5 adds resume:<session-id>; probe 6 adds in-process hooks:
hooks: { PreToolUse: [{ matcher: 'Bash', hooks: [async (input, toolUseID) => ({
  hookSpecificOutput: { hookEventName: 'PreToolUse', permissionDecision: 'deny',
                        permissionDecisionReason: 'Sinet spike S1 gate: denied by in-process SDK hook' }})]}] }
```

## 3. Findings

### F1 — Headless invocation & structured output: schema parity is total

Both surfaces emit the **identical stream-json event schema** (the SDK yields the same objects as typed values instead of JSONL lines): `system{hook_started, hook_response, init, thinking_tokens}`, `assistant` (full per-call `usage`), `user` (tool_results), `rate_limit_event`, `result`. Observed on both:

- `system/init` carries `session_id`, `model`, `permissionMode`, `cwd`, and **`apiKeySource`** (see F5).
- **Every `assistant` message carries complete per-call usage** — `input_tokens`, `output_tokens`, `cache_creation_input_tokens`, `cache_read_input_tokens`, and the TTL breakdown `cache_creation: {ephemeral_5m_input_tokens, ephemeral_1h_input_tokens}`. This is exactly the D7 checkpoint-per-paid-call row, available identically on both surfaces.
- `result` envelope keys (verbatim, identical on both): `api_error_status, duration_api_ms, duration_ms, fast_mode_state, is_error, modelUsage, num_turns, permission_denials, result, session_id, stop_reason, subtype, terminal_reason, time_to_request_ms, total_cost_usd, ttft_ms, ttft_stream_ms, type, usage, uuid`. `usage` includes an `iterations[]` array (per-model-call breakdown within the turn); `modelUsage` carries per-model `costUSD`. `total_cost_usd` is emitted under subscription auth on both (D5's reporting currency, confirming R01 §2.3).
- `rate_limit_event` (both surfaces, verbatim): `{"rate_limit_info": {"status": "allowed", "resetsAt": 1784270400, "rateLimitType": "five_hour", "overageStatus": "rejected", "overageDisabledReason": "org_level_disabled", "isUsingOverage": false}}` — a richer 3.2 park-event input than R01 recorded (reset timestamp + window type + overage state, in-band).
- `--session-id <uuid>` is honored exactly (control plane can choose ids, per R01 §4 verb table "start").

### F2 — Session resume: continuity verified on both; same session id kept

- CLI: `-p --resume <id>` continued the **same** session id (init and result both reported the original id — no forced fork at v2.1.212); the model correctly recalled turn-1 content ("OK"). SDK: `resume: <id>` behaved identically (recalled "PANDA", same id).
- **Session-file behavior identical:** both surfaces persist `~/.claude/projects/<cwd-encoded-path>/<session-id>.jsonl` — same substrate, CWD-keyed lookup confirmed live (P-T01-1's rename hazard applies equally to both).
- **Neither surface's sessions appear in the newer `~/.claude/sessions/` index** — R01's "`-p` sessions unindexed; resume-by-id still works" holds for the SDK too. Resume-by-id worked on both.

### F3 — CACHE FIDELITY (the load-bearing measurement): healthy on BOTH surfaces

| Measurement | CLI turn 1 | CLI resume | SDK turn 1 | SDK resume |
|---|---|---|---|---|
| `input_tokens` | 10 | 10 | 10 | 10 |
| `cache_creation_input_tokens` | 8,238 | **57** | 19,169 | **59** |
| `cache_read_input_tokens` | 17,418¹ | **25,656** | 0 | **19,169** |
| TTL bucket | 1h | 1h | 1h | 1h |
| `total_cost_usd` | $0.0184 | $0.0030 | $0.0386 | $0.0024 |

¹ CLI turn 1 already read 17,418 cached tokens (shared prefix with other same-host sessions); SDK turn 1 read 0 because its default system prompt is a different prefix. Within-surface comparison is the valid one.

Verbatim, SDK resumed turn (the decisive datum):

```json
"usage": {"input_tokens": 10, "cache_creation_input_tokens": 59, "cache_read_input_tokens": 19169,
          "cache_creation": {"ephemeral_5m_input_tokens": 0, "ephemeral_1h_input_tokens": 59}, "output_tokens": 4}
```

Readout:

- **Both surfaces resume with a full cache hit**: `cache_read` = exactly the entire prior context (CLI 25,656 = 17,418 + 8,238; SDK 19,169 = turn-1's write), new writes ≈ the new turn only (57–59 tokens). No re-ingestion on either surface.
- **The #39732 state ("prompt caching disabled for SDK `query()`", closed not-planned) is empirically NOT the current behavior at SDK 0.3.212** — R05 §2.2's "the CLI wrap currently has the healthier caching story" is stale as of today. P-T02-2's premise (this class drifts silently and has regressed twice) still stands — the §4.12 standing alarm remains mandatory — but cache fidelity no longer discriminates between the two surfaces.
- **Subscription auth requested the 1-hour TTL on both** (`ephemeral_1h` bucket, live confirmation of R05 §2.2). Corollary for the harness: a park longer than ~1h re-pays the whole context on resume — in these probes a cold turn cost ~6–16x a cached resume turn ($0.0184–0.0386 vs $0.0024–0.0030). Feeds the hold-vs-park threshold economics (R05 OQ1) and T08.
- Healthy-baseline calibration for the §4.12 alarm (T08): healthy = `cache_read ≈ full prior context, cache_creation ≈ new-turn tokens`; regression signature = `cache_read ≈ 0` with `cache_creation ≈ full context` on a resume turn inside TTL.

### F4 — Permission gating: full parity, two delivery mechanisms

- **CLI:** PreToolUse hook injected via **per-invocation `--settings` file** (no global config touched) worked as documented: the hook subprocess received the full JSON payload on stdin (`session_id`, `transcript_path`, `cwd`, `tool_name`, `tool_input`); its `permissionDecision: "deny"` blocked the Bash call; the reason string surfaced to the model as an `is_error` tool_result; the result envelope carried a structured audit: `"permission_denials": [{"tool_name": "Bash", "tool_use_id": "toolu_01…", "tool_input": {"command": "echo hello", …}}]`.
- **SDK:** in-process async hook callback (`options.hooks.PreToolUse`) received the **byte-equivalent payload** (plus `toolUseID` as an argument) and produced identical downstream behavior, including the `permission_denials` audit. No subprocess, no settings file.
- The SDK's typed `HookPermissionDecision` is `'allow' | 'deny' | 'ask' | 'defer'` — **`defer` (the R05 §2.3 exit-park primitive) is typed on the SDK surface too.** (Defer end-to-end + parallel-tool-call fallback rate were not exercised — that is R05 OQ6's separate drill.)
- SDK also exposes `canUseTool` (with the documented shadowing caveat — R05 §2.1's "gate must live in PreToolUse hooks" is unchanged and holds on both surfaces), `settingSources: []` (isolation mode), and a restrictive `managedSettings` policy tier.
- **Hygiene finding:** the SDK's default `settingSources` loads user/project/local settings ("matches CLI defaults" per its typings) — the operator's global SessionStart hook demonstrably fired inside SDK probes. Sinet's adapter must pass explicit `settingSources` (SDK) / `--settings` + settings flags (CLI) so worker runs don't inherit operator-level hooks or CLAUDE.md.

### F5 — Subscription auth: SDK rides the operator's login — no disqualifier

With no `ANTHROPIC_API_KEY` anywhere in the environment, `query()` executed successfully and `system/init` reported **`apiKeySource: "none"`** — identical to the CLI probes. The SDK reuses Claude Code's credential store. The gate condition ("API-key demand alone disqualifies") did **not** trigger; all SDK probes proceeded. (This is mechanism, not policy: R01 §2.1's gray-zone posture for personal SDK-on-subscription is unchanged by this measurement.)

### F6 — Packaging difference (unrequested but adapter-relevant)

The SDK npm install pulls a **platform-specific engine binary package** (`@anthropic-ai/claude-agent-sdk-linux-x64`, ~252 MB, plus ~247 MB musl variant) whose manifest pins engine version **2.1.212 with per-platform SHA-256 checksums**. The SDK did **not** execute the operator's `~/.npm-global` CLI. Consequences: (a) pinning the SDK in a lockfile pins a checksummed engine — arguably the cleanest pin in the field; (b) SDK 0.3.212 ↔ CLI 2.1.212 lockstep confirmed at the artifact level (same-day manifest, `buildDate: 2026-07-16`); (c) ~500 MB disk per pinned install; (d) an SDK-based adapter still spawns an engine subprocess — the process-model difference from CLI-wrap is smaller than it looks.

## 4. Verdict

**Keep CLI-wrap as the v0 Anthropic adapter (R01 §4's start-assumption stands) — but the margin is now policy and dependency-shape, not mechanism. The SDK is verified as a near-drop-in fallback.** Confidence: **moderate-high** for the v0 choice; **high** for "both surfaces are mechanically adequate against the adapter contract."

Reasoning:

- Every mechanical criterion measured came out **at parity**: identical event schema and usage fields (D7 rows), identical session substrate and resume behavior, healthy cache fidelity on both, equivalent gating with the same decision grammar (incl. `defer`), budget/turn belts on both (`maxBudgetUsd` typed in the SDK), subscription auth on both.
- What still separates them is exactly what R01 §2.1/§3-D recorded: **policy posture** (CLI `-p` + `setup-token` is the surface Anthropic's docs explicitly bless for scripts; the SDK is the discretionary gray zone) and **drift surface** (the SDK adds a second pre-1.0 API layer — its own breaking-change record, e.g. 0.3.142 — on top of the same engine; the CLI wrap exposes only the stream-json schema, which the adapter must parse forward-tolerantly anyway per P-T01-2).
- The spike **removes** the two candidate hard disqualifiers for the SDK (API-key demand: no; cache infidelity: no). Since the adapter contract maps 1:1 onto both surfaces (F1–F4), building against Sinet's own contract keeps the swap cost low — precisely R01's "implementation detail inside the Anthropic adapter, not an architecture fork."

**What would flip it to the SDK:**

1. A written Anthropic personal-use safe harbor for SDK-on-subscription (R01 §4's pre-registered flip condition) — the typed in-process hooks, `interrupt()`, and checksummed engine pinning (F6) then become decisive advantages.
2. Stream-json schema churn on the CLI outpacing the SDK's typed layer (i.e., the conformance suite starts failing on JSONL parsing more often than on SDK API changes).
3. The R05 OQ6 defer drill finding that held-process parking (where the SDK's in-process callbacks and `interrupt()` are strictly more ergonomic) must carry more of the gate load than exit-parking.
4. A cache or `defer` regression appearing on the CLI surface only (the §4.12 alarm would surface it).

## 5. Implications for T07/T08 briefs

- **T07 (checkpoint store):** checkpoint-row schema can be **surface-agnostic** — per-call `usage` (incl. `cache_read`/`cache_creation` + TTL buckets + `iterations[]`), `session_id`, and `result` envelope are identical on both surfaces. Record the session id from `system/init` on every run rather than assuming the requested id or id stability across resumes (same-id observed today; R01 recorded fork-on-resume behavior historically — treat as unspecified). Both surfaces' JSONL files are CWD-keyed and absent from the sessions index: P-T01-1 (Sinet's store is authoritative; engine sessions are a resume optimization) applies with equal force to both.
- **T08 (metering & cache alarm):** `total_cost_usd` + per-model `modelUsage.costUSD` are emitted under subscription auth on both surfaces — D5's API-equivalent receipts need no surface-specific handling. Calibrate the §4.12 cache-fidelity alarm with this spike's healthy baseline (resume turn: `cache_read ≈ full prior context`, `cache_creation ≈ new-turn-only`; regression signature: `cache_read ≈ 0` on an in-TTL resume). `rate_limit_event` carries `resetsAt`, `rateLimitType`, `overageStatus` — the 3.2 park scheduler gets a machine-readable resume time for free. The 1h-TTL economics (cold resume ≈ 6–16x a cached one) is a direct input to the hold-vs-park threshold (R05 OQ1) and to scheduling resumes inside the cache window when queue pressure allows.
- **Adapter spec (T07-adjacent):** mandate explicit `settingSources`/`--settings` on every managed invocation (F4 hygiene finding — worker runs must not inherit operator-level hooks/CLAUDE.md); keep the conformance suite dual-target (both surfaces stay green so the fallback stays warm — the cost of this is low given F1's schema identity).

## 6. Probe-cost disclosure

| # | Surface | Probe | Model | Reported `total_cost_usd` |
|---|---|---|---|---|
| 1 | CLI `-p` | headless stream-json, `--session-id` | haiku-4-5 | $0.0184178 |
| 2 | CLI `-p --resume` | resume + continuity + cache | haiku-4-5 | $0.0030096 |
| 3 | CLI `-p --settings` | PreToolUse log-and-deny | haiku-4-5 | $0.0217016 |
| 4 | SDK `query()` | headless, no API key (auth gate) | haiku-4-5 | $0.0385680 |
| 5 | SDK `query()` resume | resume + continuity + cache | haiku-4-5 | $0.0023549 |
| 6 | SDK in-process hook | PreToolUse log-and-deny | haiku-4-5 | $0.0202578 |
| | | | **Total** | **$0.1043** |

All probes under the $0.05/probe cap and ≤2 turns; total 21% of the $0.50 spike budget. Figures are the engines' own API-equivalent reports under subscription auth (no metered spend occurred).

**Caveats:** single-run probes (no repetition for variance); haiku-only (cache fields are API-level; a default-model replication was judged not worth the spend); not exercised — `defer` end-to-end, `--fork-session`, `interrupt()`, parallel-tool-call fallback rate (R05 OQ6's drill), and `canUseTool` latency comparison (R01 OQ3 mentioned it; PreToolUse-hook parity made it non-load-bearing for this verdict).

## 7. Blocked items

None. All five mandated probe areas completed.
