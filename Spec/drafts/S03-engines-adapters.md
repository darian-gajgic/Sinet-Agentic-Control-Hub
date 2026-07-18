## S03 — Execution engines & adapters

**Scope:** The D3 adapter contract and its two v0 substrates — how every provider connection (start, stream progress, checkpoint, pause, resume, cancel, report usage) is delivered by a wrapped `claude` CLI (Anthropic lane) and a pinned `opencode serve` (Z.AI + local lanes), how engines are pinned/bumped/conformance-gated, how gate-parking works per lane, and how a Sinet run is *lowered* so that compiled config is the only config an engine sees and all spawning stays control-plane-owned.

**Binding inputs:** D3, D2, D6, D7, Operating reality (subscription coverage) · [R01 §2–7], [R02 §2–6], [R05 §2–4] · [G1 P1, P2, P5, P6; D1.6; riders 2, 3] · [G2 D2.2; P6] · [G3 D3.2; §Follow-ups R06-OQ2 reminder] · [SPIKE G1-S1] CLI-vs-SDK · [SPIKE G1-S2] defer drill · [SPIKE G1-S3] opencode park + XDG · [SPIKE P2-S1] Z.AI live-auth + calibration · [SPIKE P2-S5] engine lowering · [SPIKE P2-S4 headline] serialize-by-deny · R06-OQ2 (open question, §7). Boundary siblings: [XREF:S02] checkpoint store/recovery · [XREF:S04] helper topology/spawn triggers · [XREF:S10] metering/limit taxonomy · [XREF:S12] local-lane internals · [XREF:S14] canary/eval machinery.

**New term coined here:** **engine lowering** — compiling a Sinet run down to the exact per-lane invocation an engine receives, such that Sinet-compiled config is the *only* config in effect (S03.5). **Config channel** — one independent path (settings, MCP, skills, tools, cross-read instruction files, cwd project-config, env) through which ambient environment could reach a worker; each needs its own closing knob.

### S03.1 The adapter contract (D3 verbs)

Every lane sits behind one Sinet-owned adapter interface. The adapter is the **only** component that touches engine specifics; everything above it — scheduler, watchdogs, the run FSM [XREF:S02], orchestration [XREF:S04], metering [XREF:S10] — speaks this contract and nothing else [R01 §4]. Verbs are shaped as a strict superset of ACP's stable verb set so a future transport convergence is a swap, not a re-architecture [R01 §2.4, §4].

Interface sketch (spec pseudocode; not application code):

```
adapter.start(run_id, compiled_worker, model, owner_cred_ref) -> session_cursor
adapter.stream(session_cursor) -> iter<event>      # event ∈ {message, usage, rate_limit, gate_ask, tool_result, done}
adapter.checkpoint(session_cursor) -> checkpoint_ref   # per paid call (D7); payload/store owned by [XREF:S02]
adapter.pause(session_cursor) -> park_record        # interrupt-at-safe-boundary + park (no true suspend exists)
adapter.resume(park_record) -> session_cursor       # FULL invocation re-supply; freshness-gated by [XREF:S02]
adapter.cancel(session_cursor) -> disposition       # boundary → engine-abort → group-TERM → KILL ladder
adapter.report_usage(event|session) -> usage_row    # per paid call; priced by Sinet's table, never the engine, [XREF:S10]
```

Verb → substrate mapping (updated from [R01 §4] with the spike measurements):

| Verb | Anthropic lane (wrapped `claude -p`) | Z.AI / local lane (`opencode serve`) | Adapter obligation |
|---|---|---|---|
| **start** | spawn process, control-plane-chosen `--session-id <uuid>` (honored exactly, [SPIKE G1-S1 F1]) | `POST /session` + `POST /session/{id}/prompt_async` (per-message `model:{providerID,modelID}`, `agent` by name) [SPIKE G1-S3] | per-run owner credential in the engine process, never the sandbox (D2) |
| **stream progress** | stdout JSONL `stream-json` (identical schema to the SDK; carries `assistant.usage`, `rate_limit_event`, `result`) [SPIKE G1-S1 F1] | v1 SSE `GET /event` for parks; both event generations exist — parse tolerantly [SPIKE P2-S1 Probe 6-B] | **forward-tolerant parsing MUST**: unknown event types logged and skipped, never fatal (GitButler lesson, [R01 §2.3]) |
| **checkpoint** | `result`/`assistant` usage rows per call; JSONL transcript is a *resume optimization only* | per-message token/cost rows; SQLite `event`/`event_sequence` durable log | Sinet's store is authoritative (P-T01-1); payload + store = [XREF:S02] |
| **pause** | PreToolUse `defer` (exit-park) below/above the hold threshold; message-boundary interrupt otherwise | `permission.asked` parks the turn at **zero cost** on an in-process deferred; abort + re-prompt as the restart path | never interrupt mid-tool-call (P-T01-4); durable ask-record on observation (S03.4) |
| **resume** | `--resume <id>` (same id on the defer path, no fork — [SPIKE G1-S2 F2]); `--fork-session` for retry-without-poisoning | re-prompt the surviving session id (a paid turn) | **re-supply the entire invocation** (S03.4); freshness-gate first [XREF:S02] |
| **cancel** | process-group TERM→KILL (spawn each engine as its own group leader); health-probe before reuse | `POST /session/{id}/abort` (cooperative) then group kill | boundary-first; fork-from-checkpoint is the standard recovery, not transcript repair (P-T01-4) |
| **report usage** | `result.total_cost_usd`/`usage`/`modelUsage` — emitted under subscription auth (D5 reporting figure) [SPIKE G1-S1 F1] | per-message `.tokens.{input,output,reasoning,cache.*}`; opencode's `.cost` is **$0** (flat-rate) [SPIKE P2-S1 Probe 7] | Sinet meters independently (D4) and prices from its own table (D5); engine figures are cross-checks [XREF:S10] |

Limit signals ride this contract as data (D4): Anthropic emits in-band `rate_limit_event` carrying `resetsAt`, `rateLimitType`, `overageStatus` [SPIKE G1-S1 F1]; opencode surfaces `GET /session/status` `retry` variant (`attempt`, `next`, provider `action`) [SPIKE G1-S3]. The adapter forwards them; the five-class taxonomy and park scheduling are [XREF:S10]. **Multi-host awareness (12.7/12.8):** every substrate surface is already network-transparent; if Sinet ever grows past one host the adapter contract survives unchanged [R01 §2.4] — no v0 design work.

### S03.2 The v0 lanes

v0 ships exactly two substrates carrying three lanes [G1 P6; rider 3; G2 D2.2]; additions are config-only (S03.6).

- **Anthropic lane — wrapped `claude` CLI, pinned, one process per user.** Invocation core: `claude -p --output-format stream-json --input-format stream-json --verbose --include-partial-messages`, per-user `CLAUDE_CONFIG_DIR`, auth via each member's own `claude setup-token` (1-year, inference-scoped — the surface Anthropic's docs bless for scripts) [R01 §2.1, §4]. Model per invocation (`--model`); resume by id / fork by `--fork-session`; ceilings `--max-turns` + `--max-budget-usd` as belts [R05 §2.6]; **never** `--dangerously-skip-permissions`. Gate via PreToolUse hooks (S03.4). Compliance posture: gray-zone as-is, D2/3.4 only [G1 P2] — self-hosted, per-person-authenticated through Anthropic's own tool sits closest to the encouraged personal-use category [R01 §2.1].
- **Z.AI lane — pinned `opencode serve`, one instance per user, per-user XDG isolation.** Pin `opencode-ai@1.18.3` **exact** (npm `latest` is still 1.x but flips at 2.0 GA; `@1` floats) [SPIKE G1-S3]. Each instance runs under fully disjoint `XDG_{CONFIG,DATA,STATE,CACHE}_HOME` — measured to fully contain auth.json, the SQLite DB, config, cache and logs with **zero cross-leak and zero strays** on the serve path (#18633 did not reproduce) [SPIKE G1-S3 F1]. Bind `127.0.0.1`; set a per-instance `OPENCODE_SERVER_PASSWORD` (enforced as HTTP **Basic** auth on *every* endpoint including `/api/health` — the adapter client and health-watcher both send it) [SPIKE P2-S1]. Provider `zai-coding-plan`, endpoint `https://api.z.ai/api/coding/paas/v4`; the operator holds GLM Coding Max [G1 D1.5]. Per-user first boot pulls a ~62 MB provider dependency tree and needs network once [SPIKE G1-S3 F1]. Z.AI is a class-2 (tool-scoped, no unattended-use ban) plan run inside a whitelisted tool — accepted as-is under one uniform policy [G1 P5; R02 §2.4].
- **Local lane — local models exposed to the same `opencode serve` adapter as OpenAI-compatible providers** (llama-swap → llama-server). Model set, GPU arbitration, and platform-internal local duties (ceremony, watchdogs, utility models) are [XREF:S12]. Adapter-relevant fact: it is the **only** v0 lane exposing logprobs, hence the only one with a logprob drift-canary; both subscription lanes fall back to behavioral evals [SPIKE P2-S1 Probe 8; XREF:S14]. Otherwise it inherits the opencode adapter wholesale.

**Agent-SDK-as-in-adapter-alternative.** The Anthropic adapter is built against Sinet's own contract; the *mechanism* is a v0 sub-choice, not an architecture fork [R01 §3-D]. Spike verdict: keep **CLI-wrap** for v0; the Agent SDK is a verified near-drop-in fallback kept documented inside the same adapter [SPIKE G1-S1 §4]. Every mechanical criterion measured at parity — identical stream-json schema and per-call usage (D7 rows), identical session substrate/resume, **healthy cache fidelity on both** (the #39732 "SDK caching disabled" state is not current — [R05 §2.2]'s "CLI has the healthier caching story" is stale), equivalent gating incl. typed `defer`, and subscription auth on both (`apiKeySource:"none"`, no API-key demand) [SPIKE G1-S1 F3–F5]. The residual margin is **policy** (CLI+`setup-token` is the blessed scripts surface) and **drift-surface** (the SDK adds a second pre-1.0 API layer over the same engine). Flip conditions, pre-registered: a written Anthropic personal-use safe harbor; CLI stream-json churn outpacing the SDK's typed layer; the defer economics favoring held-process parking; or a cache/`defer` regression on the CLI surface only [SPIKE G1-S1 §4].

### S03.3 Engine version policy & drift

Both substrates are **pinned versioned dependencies**; schema and behavior drift is *when*, not *if* — 321 `claude` releases/yr on one lane, an announced V1→V2 API break on the other [R01 §2.3, P-T01-2]. Pins live in the adoption manifest / `components.lock` [XREF:S16]; the bump procedure lives here:

1. **Pin exact** (`claude` 2.1.214, `opencode-ai@1.18.3` today); never track `latest` [SPIKE G1-S3, P2-S5].
2. A **deliberate bump** is an operator-gated release event, never automatic. `opencode`'s `POST /global/upgrade` self-upgrade endpoint MUST never be exposed or called [SPIKE G1-S3 F2].
3. **Conformance gate before a bump lands:** the per-substrate conformance suite MUST pass on the candidate version *and* a before/after **quality** probe on a small task battery must show no regression — the Apr-2026 Claude-Code postmortem class (silent effort/context-handling drift) is invisible to schema tests and uptime checks [R05 §2.8, P-T02-5]. Suite/probe machinery is [XREF:S14]; the gate policy is normative here.
4. Conformance suites **assert behavior, never docs** — the vendor's own docs run months stale (Ollama logprobs; opencode defer-page truncation) [G3/R16 finding; SPIKE G1-S2 §5].
5. A bump is a **worker-revalidation trigger**: every worker version tuned against the bumped engine is flagged for revalidation before further unsupervised use, and Sinet's bump *is* the announcement clock — no provider clock announces it (P-T14-1) [XREF:S08].

Four adapter-facing drift facts bind the design:

- **Engine transcripts are not durable state (P-T01-1).** Claude JSONL is CWD-keyed, absent from the sessions index for `-p`, corruptible, and swept after `cleanupPeriodDays`; opencode keeps sessions in SQLite but **not** pending asks. → Sinet's store is authoritative; engine sessions are a resume optimization; copy-aside is [XREF:S02]; `⚙ adapter.claude.cleanup_period_days` is raised well above the park horizon [SPIKE G1-S2 F5].
- **Schema drift is permanent even first-party (P-T01-2 / P-T02-2).** → Forward-tolerant parsing (S03.1) + conformance suites + a standing cache-fidelity alarm (`cache_read` stuck flat across an in-TTL resume; healthy baseline calibrated in [SPIKE G1-S1 F3]) whose machinery is [XREF:S10/S14].
- **Billing-regime shifts are ~30-day ops events (P-T01-3).** Handled as *data* — per-model flat/metered flags with a rehearsed flip and receipts that visibly change currency [XREF:S10]. The Anthropic un-pause response is **pre-registered [G1 P1]**: the Anthropic lane demotes to interactive-only and headless weight shifts to Z.AI-class/local lanes — a policy/data flip, **no architecture change**; detection is the S2.8 watch [XREF:S14].
- **Cancel corrupts JSONL mid-tool-call (P-T01-4).** → The cancel ladder prefers message-boundary interrupts, runs a session-health probe before any reuse, and treats fork-from-checkpoint as the standard recovery (opencode's mid-abort corruption family was fixed 2026-04; Claude's is canon) [R05 §2.7].

### S03.4 Gate parking & defer mechanics per lane

Proposals (4.2) are collected at the gate and the run parks losing nothing (4.3). No engine offers true mid-run suspension; "pause" = interrupt-at-safe-boundary + park + external-event resume [R01 §2.3]. The **durable ask-record is a platform record from the moment the ask is observed** on *both* lanes — engine-side park state is volatile (P-T01-1, P-T02-1), confirmed at source and live.

**Anthropic lane — `defer` (PreToolUse) is the exit-park primitive** [R05 §2.3; SPIKE G1-S2]. The headless process exits `stop_reason:"tool_deferred"` carrying a `deferred_tool_use{id,name,input}` object — the adapter builds its ask-record from the exit JSON alone, no transcript parsing [SPIKE G1-S2 F1]. `--resume` re-fires PreToolUse on the same `tool_use_id`; the hook returns allow + `updatedInput` (the human's answer) with no fork; poll-resume while parked is genuinely free ($0, zero tokens) [SPIKE G1-S2 F1]. Two traps and two obligations bind the adapter:

- **Trap — single-tool-call-only.** `defer` is honored only when the turn contains one tool call; parallel-tool-call turns fall back to normal permission flow **silently** (no stderr warning in `-p` json mode). Measured fallback rate ≈ **20% of gated turns on the default worker model** (8.5% on opus-4-8) — a first-class path, not an edge case (P-T02-4) [SPIKE G1-S2 F3]. Detection is the adapter's job: >1 PreToolUse fire for one turn + no `tool_deferred` exit ⇒ fallback happened. Chosen fallback: **serialize-by-deny** — deny all calls of a parallel gated turn with reason "re-issue the gated call alone", converting fallback into ~one extra cheap turn rather than a held process; measured viable on haiku (~+1 turn) [SPIKE P2-S4 headline] `[coordinator-draft]`, with `⚙ adapter.parallel_gate_fallback` and a bring-up reconfirm on the default worker model, TBD-BRINGUP(parallel-gate fallback rate on default worker model).
- **Trap — cleanup-sweep evaporation.** A deferred session is still subject to the `cleanupPeriodDays` startup sweep (default 30 d) — a parked call silently evaporates past retention (P-T02-1) [SPIKE G1-S2 F5]. → `⚙ adapter.claude.cleanup_period_days` raised above the park horizon; the platform ask-record is authoritative regardless.
- **Obligation — full config re-supply on every resume.** The engine restores **nothing** — not hooks, and (measured, correcting [R05 §2.3 S41]) **not permission mode**; a resume that forgets `--settings` silently executes the parked call unreviewed [SPIKE G1-S2 F2/F3]. The park-record snapshots the entire invocation (settings content+fingerprint, permission mode, model, tool allowlist) and re-supplies all of it. Schema = [XREF:S02].
- **Obligation — ceiling ordering.** A same-turn `--max-budget-usd` trip pre-empts the park (dies `error_max_budget_usd`, no `deferred_tool_use`), so the engine ceiling MUST sit ≥ `⚙ adapter.engine_ceiling_backstop_mult` × the platform ceiling; the platform ledger is the real ceiling [SPIKE G1-S2 F4; XREF:S10].

**Z.AI / local lane — `permission.asked` + REST answer** [SPIKE G1-S3, P2-S1]. Live-confirmed round-trip: the turn parks `busy` at zero cost on an in-process deferred; enumerate **all** entries of the flat `GET /permission` array (`permission.list`); answer via `POST /permission/{id}/reply {reply, message?}` (keeps reject-with-feedback `CorrectedError`, unlike the legacy `permission.respond`); key everything by `requestID` — one `always`-class reply fans out to N `permission.replied` events [SPIKE P2-S1 Probes 1, 6]. Binding constraints:

- **Subscribe to v1 `GET /event`** for `permission.asked`/`permission.replied`; the global v2 `/api/event` does not carry them in 1.18.3 [SPIKE P2-S1 Probe 6-B].
- **Never send `"always"` through opencode** — v1 `always` rules are in-memory **and server-wide**, leaking approvals across a person's sessions and vanishing on restart; the adapter emits only `once`/`reject` and keeps persistent allow-policy in Sinet's own layer [SPIKE P2-S1 Probe 6-A; impl. #2].
- **Restart-ask-loss + containment.** Pending permission *and* question asks are in-memory `Map`s, rejected on graceful shutdown and lost on crash — source-confirmed and reproduced live (#36347, unfixed; PRs closed unmerged) [SPIKE G1-S3 F4; P2-S1 Probe 2]. Containment: persist on `permission.asked`; treat a serve restart as "ask gone, session intact, **re-prompt the surviving session to recover** (a normal paid turn)"; the adapter marks the orphaned call cancelled in its **own** checkpoint — opencode leaves the transcript part cosmetically `running` and must not be trusted [SPIKE P2-S1 Probe 2]. Shutdown rejection is **silent** (no bus event); the adapter infers a dead park by diffing its ask-record against `permission.list` after any restart/health blip [SPIKE P2-S1 Probe 5].
- **HTTP-only origination.** Originate every turn over HTTP and never attach a TUI to a Sinet-owned serve — #16367 (attach hang under `ask`) is orthogonal to the HTTP path (measured immune), but #36835 makes TUI-initiated asks unanswerable over REST [SPIKE P2-S1 Probe 3].

### S03.5 Engine lowering & the sole-controller posture

**Engine lowering** — the run is compiled down to the exact per-lane invocation, and **compiled config is the ONLY config an engine sees** [SPIKE P2-S5; R15 §4.1]. The load-bearing spike finding: isolating *settings* is necessary but **not sufficient** — MCP servers, skills, tools, HOME-relative `.claude/` cross-reads, and walked-up cwd project-config are **separate config channels** that each bleed the ambient/operator environment into a worker unless explicitly closed. The guarantee is therefore **COMPOUND: one knob per channel**, enforced as a per-lane conformance-suite checklist entry (attempt-the-leak tests that assert the decoy did *not* take effect) [SPIKE P2-S5 §"Adapter requirements"; XREF:S14].

| Config channel | Anthropic lane knob (2.1.214) | Z.AI/local lane knob (1.18.3) |
|---|---|---|
| settings / hooks / CLAUDE.md | `--settings <sinet.json>` + `--setting-sources ""` | isolated `opencode.jsonc` **or** inline `OPENCODE_CONFIG_CONTENT`; per-user disjoint XDG |
| MCP / connectors | `--strict-mcp-config` (+ `--mcp-config` for Sinet servers only) | (opencode MCP configured only in the isolated file) |
| skills / slash-commands | `--disable-slash-commands` (ship Sinet skills as pointed-at dirs, not discovery) | — |
| **tools incl. native spawn** | `--tools "<allowlist>"` (default-deny; **MUST exclude `Task` + `TaskCreate/Get/List/Output/Stop/Update`**) | `permission.task:"deny"` (blanket) |
| HOME-relative cross-reads | (closed by `--setting-sources ""`) | `OPENCODE_DISABLE_CLAUDE_CODE=1` — kills `~/.claude/CLAUDE.md` + project `CLAUDE.md` + `.claude/skills` scans (HOME is unchanged under XDG, so required) |
| cwd project-config | (isolated cwd; no project `.claude` write) | `OPENCODE_DISABLE_PROJECT_CONFIG=1` — an adversarial walked-up `opencode.json` can otherwise re-enable `task:"allow"` (leak reproduced) |
| plugins | (none loaded) | `OPENCODE_PURE=1` + `OPENCODE_DISABLE_DEFAULT_PLUGINS=1` |
| agent / prompt body | `--agents '<json>' --agent <name>` (+ `--append-system-prompt`); inline, no disk write | agent map in the isolated config, selected per turn by **name** in `prompt_async` (no inline per-request agent *definition*) |
| nested-session env | scrub `CLAUDECODE`, `CLAUDE_CODE_*`, `AI_AGENT`, `CLAUDE_EFFORT`, `CLAUDE_PID` | (XDG env set on every invocation, G1-S3) |

The compiled worker body+config is hash-pinned per run [R15 §4.1]. D8 worker templates compile to these targets; the guardrail split (behavioral content in files, all enforcement state in control-plane tables) is [XREF:S08].

**Sole-controller posture [G1 rider 2].** All agent spawning is control-plane-owned: helpers are control-plane-spawned sibling sessions, briefed-in and reported-out, with D6's depth-cap/spawn-logging/no-lateral enforced in `sinet-control` — **no engine enforces D6** [R06 §5; XREF:S04]. Engine-native subagent features are **disabled on every substrate**, and the spike proved this is *structural*, not merely behavioral: on the Anthropic lane the `--tools` allowlist excludes the whole `Task*` family; on the opencode lane `permission.task:"deny"` **strips the tool from the model's toolset pre-inference** (bypass-proof: not model-settable, inherited by any subagent, `subagent_depth` defaults to 1) [SPIKE P2-S5 Probes 2, 5]. R06-OQ2 (re-enabling native micro-fanout) is *deferred, not open*, and is surfaced for the operator at G4 with the current binding posture (disabled) — see Open items.

### S03.6 Lane roadmap & onboarding

v0 = Anthropic + Z.AI (+ local); every other provider is **parked/deferred and joins post-v0 config-only** via the report-02 §5 onboarding checklist [G1 P6, P7; rider 3; R02 §5]. Adding a lane is a provider entry per user plus billing flags — never a new substrate: sanctioned providers ride opencode as OpenAI- or Anthropic-compatible config (xAI via native OAuth, Synthetic/StepFun/etc. via endpoints) [R02 §4]. The onboarding checklist's manifest home is [XREF:S16].

- **Three-class ToS taxonomy as spec vocabulary** [R02 §2.4]: class 1 open-programmatic → pass; class 2 tool-scoped without an unattended-use ban → pass with a gray-zone note only if run inside a Sinet-run tool; class 3 explicit interactive-only/automation-banned → **auto-disqualifying**. A lane must never force adopting a new engine.
- **Auth canary is distinct from limit handling per lane (P-T17-1)** [R02 §9; XREF:S14]. Sanction is allowlist-shaped and revocable server-side without notice; an auth-shaped failure classifies as *policy-revocation-suspected* → operator alert + lane freeze, **never** an infinite 3.2 retry-park. The limit-event taxonomy that the canary is distinct *from* is [XREF:S10]; the canary's scheduled machinery is [XREF:S14].
- **Two required per-model attributes** learned in research [R02 §4]: `overflow_mode` (`hard-stop` / `opt-in-credits` / `auto-metered` — the last acceptable only with a proven disable/zero-balance, else reject; 3.10) and `region_model_gate` — the adapter periodically **diffs each account's *observed* model list against config** (P-T17-3); routing and 2.7 gap advice consume the observed list with `verified-on` dates. The metered-exception list is **empty at v0** [G1 P7]; DeepSeek is the pre-registered designated exception should one ever be enabled [R02 §4].

### S03.7 Z.AI adapter-binding specifics

Two Z.AI facts, measured live, bind the adapter and cascade to other sections by XREF [SPIKE P2-S1 Probes 7, 8]:

- **No wire-side usage endpoint, no per-prompt counter.** Response bodies carry only `[choices, created, id, model, object, request_id, usage]`; headers carry no `x-ratelimit-*`; `…/usage`, `…/user/usage`, `…/dashboard/usage`, `…/v1/usage` all 404. Exact per-request token `usage` (incl. cached-token detail) is available, but the 5-hour-window prompt-unit % is retrievable **only from the operator dashboard** → prompt-unit consumption stays derived/approximation-tier and dashboard-calibrated; consequence lands in [XREF:S10]. `request_id` is the reconciliation key to the dashboard. Corollary: **never meter dollars from opencode** (it prices `zai-coding-plan` at $0) — price from Sinet's metered `zai` rows [XREF:S10].
- **No logprobs on the Z.AI coding endpoint** (`logprobs:true` → 200 but no `logprobs` key, endpoint-wide) → the Z.AI lane is **behavioral-eval-only** for drift detection; only the local lane gets the logprob canary [XREF:S14; S12].
- **`thinking:{type:disabled}` is a first-class Z.AI effort knob** (~50× token reduction on trivial work); the adapter exposes it as a Z.AI-lane lever for the Eco/Balanced effort ladder [XREF:S10].

---

**Settings introduced (⚙):**

| Name | Default | Clamp/range | Ratified by |
|---|---|---|---|
| `adapter.claude.cleanup_period_days` | 365 | integer ≥ 30; MUST exceed the ask-expiry horizon (7 d, G2 Def.2) | P-T02-1 mitigation; [SPIKE G1-S2 F5]; G1 rider 1 |
| `adapter.engine_ceiling_backstop_mult` | 2 | ≥ 2.0 | [SPIKE G1-S2 F4]; G1 rider 1 |
| `adapter.parallel_gate_fallback` | `serialize-by-deny` | {`serialize-by-deny`, `hold-process`} | [SPIKE G1-S2 §Verdict; P2-S4 headline]; `[coordinator-draft]` |

Engine version pins (`claude` 2.1.214, `opencode-ai@1.18.3`) are operator-editable via the deliberate-bump procedure (S03.3) but their manifest home and audit trail are `components.lock` [XREF:S16], not duplicated here. Every ⚙ number ships as an operator-editable setting with audit trail (G1 rider 1).

**Known problems owned here:** P-T01-1 (transcripts not durable → authoritative platform store + copy-aside [XREF:S02]; `cleanup_period_days` raised) · P-T01-2 (schema drift → pinning + forward-tolerant parsing + conformance suites) · P-T01-3 (billing shifts → data-not-architecture; pre-registered un-pause response [G1 P1]; flip mechanics [XREF:S10]) · P-T01-4 (cancel corrupts JSONL → boundary-interrupt + fork-from-checkpoint) · P-T02-1 (parked state has engine expiry → durable ask-records authoritative on observation) · P-T02-4 (defer single-tool-call bound → serialize-by-deny fallback; PreToolUse-hook gating, never `canUseTool`) · P-T02-5 (engine harness is a quality-drift source → bump = release event with before/after quality probes [XREF:S14]) · P-T17-1 (allowlist-revocable sanction → auth canary distinct from limit handling) · P-T17-3 (region/account-dependent model list → observed-vs-config diff; `overflow_mode`/`region_model_gate`). Shared/referenced: P-T02-2 (cache-fidelity drift; alarm [XREF:S10/S14]), P-T14-1 (engine-pin bumps = mass revalidation events [XREF:S08]).

**Deferred / parked:**
- TBD-P3(PreCompact-blocking + Claude-lane injection-mechanics spike) — the removed G1 S5 spike; re-entry: P3 implementation start [G1 D1.6].
- TBD-P3(Claude-lane auto-memory containment, R11-OQ6) — re-entry: adapter build at P3; engine memory features join the S2.8 canary suite [XREF:S14; G2 §Follow-ups].
- TBD-BRINGUP(parallel-gate fallback rate on the default worker model) — reconfirms the serialize-by-deny default; re-entry: bring-up battery [SPIKE P2-S4 carry-forward].
- TBD-OPERATOR(Z.AI dashboard prompt-unit calibration) — 5-step recipe feeds [XREF:S10]; re-entry: operator convenience [SPIKE P2-S1 §Blocked].
- Post-v0 lanes (xAI, Synthetic, StepFun, DeepSeek metered exception, …) — parked; re-entry: operator holds a sanctioned plan → onboarding checklist (S03.6).

**Coverage:**

| Feature-list item | Subsection |
|---|---|
| D3 dual substrate behind one adapter contract | S03.1, S03.2 |
| D2 per-run owner credential in engine process, never sandbox | S03.1 (start), S03.2, S03.5 |
| D6 sole-controller / no engine-native spawn | S03.5 + Open items (R06-OQ2) |
| Operating reality subscription coverage; un-pause response | S03.2, S03.3 (P-T01-3; [G1 P1]) |
| 3.2 limit events surfaced at the adapter (taxonomy [XREF:S10]) | S03.1 |
| 2.7 gap-advice / lane roadmap | S03.6 |
| S2.8 adapter behavior-drift watch (machinery [XREF:S14]) | S03.3, S03.6 |
| 12.7/12.8 multi-host awareness | S03.1 (note) |

**Open items for G4:**

> ### OPERATOR DECISION (standing G1-rider-2 reminder): engine-native micro-fanout
> *The coordinator raises this explicitly at G4 per the operator's standing request [G1 rider 2; G2 §Follow-ups; G3 §Follow-ups]. It is **deferred, not open** — the surrounding spec (S03.5) already binds the CURRENT posture: disabled. This box asks only whether to keep it that way.*
>
> **What it is.** "Native micro-fanout" = letting a coordinator's *engine session* spawn its own subagents through the engine's built-in task tool (opencode `task` / Claude Code `Task`) at depth 1 for a small parallel read fan-out — instead of `sinet-control` spawning, briefing, and harvesting separate sibling sessions [R06 §3-D].
>
> **What re-enabling it inside one adapter call would buy.** The engine handles a tiny 2–4-facet read fan-out inline within a single turn — lower orchestration overhead and latency than the control-plane sibling-session path (spawn → brief → stream → report round-trip) for the smallest fan-outs; the model decides the fan-out itself.
>
> **What it costs / what sole-control loses by keeping it off.** The verifying observability spike (G1 **S4**) was **removed** at [G1 D1.6] when the operator adopted the sole-controller posture — so there is no measured guarantee that native spawns are loggable at spawn; they would be *reconstructed* from event streams, not enforced [R06 §3-D, §5]. The engines don't enforce D6: opencode's guard is silently disableable — a walked-up cwd `opencode.json` re-enabling `task:"allow"` **was reproduced live** [SPIKE P2-S5 Probe 4] — and its documented runaway is 47 sessions / 20 levels [R06 §5]; Claude Code's native cap is 5 and non-configurable. Re-enabling moves D6's spawn-logging and depth-cap guarantees from control-plane-enforced to engine-config-trusted, which the evidence shows is unreliable. Keeping it **off** costs only a modest latency/overhead saving on micro-fanouts — and on flat-rate lanes marginal helper cost is consumption pressure, not dollars [R06 §4.5]; the control-plane sibling path already delivers the two cheapest wins (context protection, read fan-out) with D6's guarantees in Sinet-owned code [XREF:S04].
>
> **Recommendation (consistent with ratified evidence): keep DISABLED.** It is the ratified stance [G1 rider 2], and P2-S5 proved native spawn is cleanly, structurally disablable (tool stripped pre-inference) [SPIKE P2-S5 Probe 5]. Precondition for any future revisit: an S4-class observability spike passing **and** an engine config that guarantees every native spawn is logged-at-spawn and depth-capped — which no v0-pinned engine currently provides.
