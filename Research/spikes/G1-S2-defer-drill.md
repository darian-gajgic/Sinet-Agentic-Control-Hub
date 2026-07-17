# G1-S2 — Defer drill: exit-park end-to-end + parallel-tool-call fallback rate

**Spike battery:** G1 (gate decision D1.6-C) · **Answers:** R05-OQ6, P-T02-1 (retention interplay), P-T02-4 (coverage bound)
**Date:** 2026-07-17 · **CLI:** claude v2.1.212 (pinned, operator Max subscription) · **Host:** bench Linux 7.0.0-27-generic
**Scratch:** `…/scratchpad/spike-s2/` (hooks, settings, fixtures, raw probe outputs in `out/`)

## Purpose

Report 05 established `PreToolUse permissionDecision: "defer"` as the platform-grade exit-park primitive for the Anthropic lane, but left four things open (OQ6): (1) a functional end-to-end drill on the pinned CLI (park → exit → answer → `--resume` → `updatedInput`); (2) the load-bearing number — how often parallel-tool-call turns make defer silently fall back (P-T02-4); (3) whether permission config is faithfully restored across defer→resume; (4) the `cleanupPeriodDays` sweep that would evaporate parked sessions (P-T02-1). Empirical probes only; no application code.

## Method

**Functional drill (items 1, 3)** — model `claude-haiku-4-5` (contract not model-sensitive). Per-invocation `--settings` file gating `Bash` with a state-driven hook; every probe `--max-turns 4 --max-budget-usd 0.05 --output-format json`, cwd = scratch fixtures dir.

Hook (state file `park` → defer; `allow` → allow + modified input), stdout JSON verbatim:

```json
{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"defer","permissionDecisionReason":"SPIKE-S2: parked awaiting operator answer"}}
```
```json
{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","permissionDecisionReason":"SPIKE-S2: operator approved with modified input","updatedInput":{"command":"echo ANSWER-42","description":"operator-approved replacement command"}}}
```

Settings file: `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"<abs path>/func-hook.sh"}]}]}}`. Prompt: *"Use the Bash tool to run exactly this command: echo HELLO — then report its stdout verbatim."* Park run, then:

```
claude -p --resume <session-id> --settings <file> --model claude-haiku-4-5 \
  --max-turns 4 --max-budget-usd 0.05 --output-format json < /dev/null
```

Sequence: F1 park A → inspect persistence → F2 resume A (state=allow). F1b park B → F2b poll-resume B (still parked) → F3 resume B **without** `--settings`. FC1 park C under `--permission-mode acceptEdits` → FC2 resume C without the mode flag (hook logs `permission_mode` per fire).

**Parallel-call rate (item 2)** — model-sensitive, so measured on the **default session model** (`claude-fable-5[1m]` per user settings; probes omit `--model`). Universal gate: same hook shape with `"matcher": "*"`, always defer. Probe prompts: read-3-files, compare-2-files, two-command shell info task. Datum per probe = tool_use count in the first tool-bearing assistant turn, grouped by `message.id` in the transcript (Claude Code splits one API message across JSONL lines — counting lines overstates turns).

Live probes were budget-capped at N=3 (fable-5 `[1m]` costs $0.11–0.20/probe even cache-warm; see disclosure), so the rate was supplemented by a **zero-cost corpus measurement**: all 270 local transcripts under `~/.claude/projects/` (852 MB, spike-s2 probe sessions excluded), counting tool_use blocks per assistant `message.id`, split by model, overall and conditional on the turn containing a gated tool (`Bash|Write|Edit|NotebookEdit` — the class Sinet's 4.2 gate fires on). Same engine, same system prompt family, real workloads on this host.

**Retention (item 4)** — read-only: settings inspection at all scopes + live docs fetch (code.claude.com/docs/en/settings). No settings changed.

## Findings

### 1. Functional drill: PASS — the park/resume contract, exactly as the adapter must code it

**Park (F1).** Clean exit 0, `subtype: "success"`, `stop_reason: "tool_deferred"`, `terminal_reason: "tool_deferred"`, `result: ""`. **New contract detail beyond report 05:** the exit JSON carries a `deferred_tool_use` object with the full pending call — `{"id":"toolu_01LaHF…","name":"Bash","input":{"command":"echo HELLO",…}}` — so the wrapper can build its durable ask record from the exit JSON alone, no transcript parsing needed.

**Persistence.** Transcript at `~/.claude/projects/<cwd-key>/<session-id>.jsonl` (cwd-derived dir key, `0600`). The parked state is *implicit*: a dangling assistant `tool_use` with no `tool_result`, plus a `last-prompt` record — no explicit "pending" marker anywhere on disk. The pending proposal's durable, explicit form exists only in the exit JSON the wrapper captured.

**Hook input contract (per fire, logged verbatim):** `session_id`, `transcript_path`, `cwd`, `prompt_id`, `permission_mode`, `hook_event_name`, `tool_name`, `tool_input`, `tool_use_id`.

**Resume (F2).** `claude -p --resume <sid> --settings <file>` needs no prompt argument (redirect stdin from `/dev/null` to skip a 3 s stdin wait). PreToolUse **re-fired on the same `tool_use_id` with the original input**; the hook's allow + `updatedInput` replaced the input wholesale — the tool executed `echo ANSWER-42` and the final answer quoted `ANSWER-42` verbatim. `stop_reason: "end_turn"`, **same session id, same `.jsonl` file — deferred resume does not fork the session** (unlike ordinary `--resume` forking noted in report 01). Cost $0.0034.

**Poll-resume while parked (F2b).** Re-fires the hook, re-defers, exits `tool_deferred` again at **`total_cost_usd: 0`, zero tokens — no API call is made**. Sanctioned polling is genuinely free; periodic reconcile sweeps over parked sessions cost nothing.

### 2. Nothing is restored on resume — the wrapper owns full invocation reconstruction

- **Hooks (F3):** resuming *without* `--settings` → the gate never fired (fire-log count unchanged) and the parked `echo HELLO` **executed with its original un-reviewed input** under the ambient user allowlist (`Bash(echo:*)`). Hook config is per-invocation only; nothing of it persists with the parked session. This is P-T02-4's "silently allowed" hazard made concrete — a resume path that forgets `--settings` is an unattended approval.
- **Permission mode (FC1/FC2):** parked under `--permission-mode acceptEdits`; resumed without the flag → hook re-fire saw `permission_mode: "default"`. **Contradicts report 05 [S41]** ("resume restores the prior permission mode"): on 2.1.212, measured, it does **not**. Report 05 §2.3 should be corrected on this point.
- Consequence: the adapter's park record must snapshot the **entire invocation config** (settings file content/fingerprint, permission mode, model, tool restrictions) and re-supply all of it on every resume.

### 3. Parallel-tool-call fallback rate: ~20% of gate moments on the default model

| Measurement | N (tool-bearing turns) | ≥2 parallel calls | Rate |
|---|---|---|---|
| **fable-5, conditional on gated tool in turn (the operative number)** | **7,325** | **1,476** | **20.2%** |
| fable-5, all tool-bearing turns | 9,109 | 1,906 | 20.9% |
| opus-4-8, conditional on gated tool | 5,022 | 426 | 8.5% |
| Live probes, default model (M1–M3) | 3 | 1 | 1/3 |

Distribution on gated turns (fable-5): 1 call 79.8%, 2 calls 14.2%, 3 calls 3.8%, ≥4 calls 2.0% (max 8; max 14 across all turns). Live probes agree in kind: M1 (read-3-files) emitted one API message with **3 parallel Reads**; M2/M3 (compare-files, 2-part shell task) each emitted a single compound Bash call. **Model sensitivity confirmed:** opus-4-8's rate is well under half of fable-5's — the rate must be re-measured whenever the default worker model changes.

**Headline: roughly 1 in 5 gate firings on the current default model cannot exit-park via defer.** The fallback is not an edge case; it is a first-class path.

**The fallback is silent (worse than documented).** Docs say parallel turns fall back "with a warning"; in `-p --output-format json` mode **no warning reached stderr** (M1). Detection contract for the wrapper: >1 PreToolUse fire for one turn + process did not exit `tool_deferred` ⇒ fallback happened (PostToolUse fires confirm execution). The defers were bypassed and the parallel Reads executed under normal permission flow.

### 4. Ceilings pre-empt the park (new, unplanned finding)

When `--max-budget-usd` trips on the same turn the gate fires, the run dies with `subtype: "error_max_budget_usd"`, `is_error: true`, `terminal_reason: "budget_exhausted"`, `stop_reason: "tool_use"` — **no `deferred_tool_use`, no park — even though the hook had already fired and returned defer** (M2/M3: single-call turns, 1 fire each, defer decision returned, budget exit anyway). A defer decision is not durable against a same-turn ceiling. The wrapper must either keep engine ceilings comfortably above per-turn cost (wrapper-side ledger as the real ceiling, engine flag as backstop) or treat budget-exit-after-gate-fire as died-at-gate and rely on its own ask record.

### 5. cleanupPeriodDays (read-only; docs verified live 2026-07-17)

- Default **30 days**, minimum 1; sweeps "session files and other application data older than this period **at startup**", measured by file age; configurable in `settings.json` at any scope. **This host has no override in any scope** (user/project/local checked; no managed settings) → every parked session here is on the default 30-day clock. P-T02-1 stands unmitigated by default.
- Upstream changes since report 05: `cleanupPeriodDays: 0` now **fails validation** (the [S74] silent-persistence-kill footgun is fixed); an unreadable settings file pauses the sweep with a `/status` warning.
- Note: the docs' defer section could not be re-fetched (page truncates in both fetch variants); the empirical drill above measures the contract directly, and [S41]'s 3-vote doc verification stands except for the permission-mode-restoration claim corrected in Finding 2.

## Verdict

**Defer is a sufficient exit-park primitive for v0** — the park/resume contract is clean, machine-readable at both ends (exit JSON `deferred_tool_use` → ask record; `--resume` + hook `updatedInput` → answer injection), resume is exact (same `tool_use_id`, same session id, no fork), and polling is free. But it is sufficient only with four wrapper obligations, all now measured rather than assumed:

1. **Full config re-supply on every resume** (settings + permission mode + model): the engine restores nothing; forgetting it silently executes the parked call (Finding 2).
2. **The fallback path must be first-class, sized at ~20% of gate moments** on the current default model (8.5% on opus-4-8 — recheck per model). Held-process parking below the hold threshold covers it; above threshold, the candidate worth one probe in the next battery is **serialize-by-deny**: on a parallel gated turn, deny all calls with reason "re-issue the gated call alone" and let the model's single-call retry hit defer — converting fallback into one extra cheap turn instead of a held process. (Untested here — budget cap; flag for T07.)
3. **Fallback detection is the wrapper's job** — the engine emits no usable signal in `-p` json mode (Finding 3).
4. **Ceiling ordering**: engine budget flags must not be able to trip on the gate turn, or died-at-gate must be a handled state (Finding 4).

Plus the standing retention rule (P-T02-1, now confirmed live): platform ask/park records authoritative at observation; raise `cleanupPeriodDays` well above the park horizon on the Claude lane.

## Implications for T07/T08 briefs

- **T07 (state & checkpoints):** park record schema needs, verbatim from the measured contract: `session_id`, `transcript_path`, project-dir key (cwd), `deferred_tool_use{id,name,input}`, `permission_mode` at park, settings fingerprint + content, model, park timestamp. Resume = full invocation reconstruction from that record. Same-session-id resume simplifies the session cursor (no fork bookkeeping on the defer path). Free poll-resume permits cheap periodic park-reconcile sweeps. Add the serialize-by-deny probe to the next spike batch.
- **T08 (metering):** `error_max_budget_usd`/`budget_exhausted` are the engine ceiling signals; wrapper ledger must be the primary ceiling with engine flags as ≥2× backstop so parks can't be pre-empted. Zero-cost poll resumes must not count as attempts or spend. Cache-write premium on 1M-window models makes tiny `-p` invocations disproportionately expensive (~$0.13–0.20 each) — probe batteries and short worker runs should pin standard-window models.
- **Adapter spec (P-T02-4):** keep gating in PreToolUse hooks (confirmed: they fire even when defer is bypassed, and even when budget kills the run); detect fallback by fire-count vs exit-reason; correct report 05 §2.3's permission-mode-restoration sentence.

## Probe-cost disclosure

All probes `--max-budget-usd 0.05`, `--max-turns ≤4`; costs are API-equivalent `total_cost_usd` from the result JSON (Max subscription — no marginal cash).

| Probe | Model | What | Cost |
|---|---|---|---|
| P0 smoke | haiku-4-5 | nested `-p` sanity | $0.0184 |
| F1 park A | haiku-4-5 | defer exit | $0.0109 |
| F2 resume A | haiku-4-5 | allow+updatedInput | $0.0034 |
| F1b park B | haiku-4-5 | second park | $0.0032 |
| F2b poll B | haiku-4-5 | re-defer while parked | $0.0000 |
| F3 resume B no-settings | haiku-4-5 | hazard probe | $0.0032 |
| FC1 park C | haiku-4-5 | acceptEdits park | $0.0032 |
| FC2 resume C | haiku-4-5 | mode-restoration check | $0.0038 |
| M1 | fable-5[1m] (default) | read-3-files | $0.2036 |
| M2 | fable-5[1m] (default) | compare-2-files | $0.1297 |
| M3 | fable-5 (std window) | 2-part shell task | $0.1145 |
| Corpus mining, docs fetches, config inspection | — | read-only | $0 |
| **Total** | | **11 paid probes** | **$0.4942** |

## Blocked items

- **Live-probe battery at N≈10 on the default session model** — blocked by the $0.50 spend cap: fable-5 `[1m]` probes cost $0.11–0.20 each even cache-warm (M1–M3), so only N=3 live runs fit. The corpus measurement (N=7,325 gated turns, same model/engine/host) is offered as the load-bearing substitute and is the stronger estimator. If the operator wants the live battery anyway: (1) re-run the 10 prompts in `Method` from `…/scratchpad/spike-s2/fixtures/` with `--settings ../settings-gate.json --max-turns 1 --max-budget-usd 0.25` and a total allowance of ~$2.00; (2) count first-turn tool_use blocks per `message.id` as above. No other blockers.
- **Serialize-by-deny fallback candidate** — designed but unprobed (same budget cap); one haiku-class probe in the next battery settles it.
