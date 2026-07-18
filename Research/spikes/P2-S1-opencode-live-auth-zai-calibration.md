# Spike P2-S1 — opencode live-auth park battery + Z.AI calibration/logprob probes

**Date:** 2026-07-18 (15:19–15:25 UTC) · **Substrate:** `opencode serve` v1.18.3 (pinned), `claude` 2.1.214 present (unused)
**Lane under test:** Z.AI **GLM Coding Max** subscription, endpoint `https://api.z.ai/api/coding/paas/v4`, opencode provider `zai-coding-plan`
**Answers:** R05-OQ7 live half (park round-trip, restart loss, drills, fan-out, finalizer, #16367/#15172) · R09-OQ4 (Z.AI prompt-unit calibration) · R12 §2.5/§4.5(d) logprob-canary applicability (coordinator-labeled R12-OQ7)
**Spend:** $0.00 cash (flat-rate subscription). **~10 paid model calls** metered against the plan (see §Consumption). This is a relaunch of a stopped run; all figures re-measured from scratch.

---

## Scope / open questions

Spike G1-S3 validated the opencode park machinery **auth-free** (OpenAPI shapes, SQLite persistence, source-level ask volatility) and listed six live-only confirmations plus two calibration questions that need a real Z.AI credential. This spike provisions the live key into an **isolated** opencode instance and runs all eight probes end-to-end, then reports wire-side calibration facts for the T07 adapter and T09 metering briefs.

Probes: (1) live `permission.asked` park round-trip; (2) pending-ask loss across restart (reproduce #15172); (3) #16367 serve+attach hang under `ask` — present or fixed in 1.18.3; (4) abort→health→fork during a **running** turn; (5) SIGTERM finalizer visibility; (6) parallel-ask fan-out; (7) Z.AI prompt-unit calibration (R09-OQ4); (8) logprob support on the Z.AI coding endpoint (R12).

---

## Method

- **Isolation (secret hygiene):** all four XDG dirs (`DATA/CONFIG/STATE/CACHE`) under a spike-only scratchpad tree, `chmod 0700`; the operator's real `~/.local/share|config|state|cache/opencode` were never in the env of any opencode invocation. Key read only via `$(cat …)`; the `PUT /auth` body was built by `jq` from an env var and piped over stdin (never in argv or a file). `auth.json` **shredded** at the end; isolated data dir wiped; scratchpad verified key-free by `grep -rlF`.
- **Provisioning (non-interactive, S3 Option B):** launched `opencode serve --hostname 127.0.0.1 --port 43117` detached with the isolated env + a random per-instance `OPENCODE_SERVER_PASSWORD`; provisioned via `PUT /auth/zai-coding-plan {"type":"api","key":…}` → HTTP 200; verified with `opencode auth list` (same XDG).
- **Config:** wrote `opencode.jsonc` with `permission: {bash|edit|write|webfetch: "ask"}`; restarted serve to load it; confirmed via `GET /config`.
- **Instrumentation:** two background SSE captures (`GET /event` v1 and `GET /api/event` v2) to files, byte-offset-marked per probe so only new events are read. Turns driven by `POST /session/{id}/prompt_async` on **glm-4.7** (reliable tool-calling); waits done with a bounded `timeout tail` poll loop (no `sleep`). Direct wire probes (7,8) used `curl` to the OpenAI-compatible `/chat/completions` on **glm-4.5-air / glm-4.6** to isolate the raw API from opencode.
- Issue characterization (#16367/#15172) cross-checked by a research subagent against the current `anomalyco/opencode` tracker; conclusions reconciled with what I measured.

**Server-password mechanism (new, load-bearing for T09):** `OPENCODE_SERVER_PASSWORD` is enforced as **HTTP Basic auth** — `-u <anyuser>:<pw>` → 200; `Authorization: Bearer` and custom headers → **401**; and **`/api/health` is NOT exempt** (401 without auth). Every adapter call, including health probes, must carry Basic auth.

---

## Findings

### Provisioning — PASS
`PUT /auth/zai-coding-plan` → **HTTP 200**. `opencode auth list` → `Z.AI Coding Plan · api · 1 credentials` at the isolated `auth.json`. `GET /config/providers` → `zai-coding-plan` **connected**; opencode-exposed models: `glm-4.5-air, glm-4.7, glm-5-turbo, glm-5.1, glm-5.2, glm-5v-turbo`. Raw OpenAI-compat `GET /models` lists a superset: `glm-4.5, glm-4.5-air, glm-4.6, glm-4.7, glm-5, glm-5-turbo, glm-5.1, glm-5.2`.

### Probe 1 — Live `permission.asked` park round-trip: **PASS**
Prompt "use the bash tool to run `echo hello-from-probe1`" parked after ~6 s. `GET /permission` (permission.list), verbatim:
```json
[{"id":"per_f75dc566b001pAjo4Tq5LOPVcZ","sessionID":"ses_08a23c029ffetDFXmOtS8Ml1N2",
  "permission":"bash","patterns":["echo hello-from-probe1"],
  "metadata":{"command":"echo hello-from-probe1"},"always":["echo *"],
  "tool":{"messageID":"msg_f75dc3fe80014Xx1bpLmYZuYTO","callID":"tool-f48e6db75cbe49a3bc68e12f7cbe8f1d"}}]
```
Status while parked: `{"type":"busy"}` — the turn awaits the in-process deferred, **zero additional model calls = zero cost** (confirms S3's source read live). v1 `/event`:
```json
{"id":"evt_...","type":"permission.asked","properties":{ ...identical fields as above... }}
```
Answered with `POST /permission/{id}/reply {"reply":"once"}` → `true`. Turn resumed to idle; permission.list emptied. v1 `/event`:
```json
{"type":"permission.replied","properties":{"sessionID":"ses_...","requestID":"per_...","reply":"once"}}
```
Command genuinely executed — transcript tool part: `state.status:"completed"`, `input.command:"echo hello-from-probe1"`, `output:"hello-from-probe1\n"`, `metadata.exit:0`, exec ~4 ms.

### Probe 2 — Pending-ask loss across restart (#15172 / #36347): **REPRODUCED (loss confirmed) + recovery confirmed**
Parked a real `pwd` ask (`pending:1`), then `SIGTERM` the server and restarted with the same XDG:
- **After restart: `permission.list` = `{"pending":0}` — the ask is gone.** #15172-class behavior reproduced live.
- **Session survives** (`session_survived:true`; 6 sessions listed) — SQLite intact.
- The orphaned tool call is frozen at `{"cmd":"pwd","status":"running"}` with no output; session is `idle` (turn abandoned). Note: in 1.18.3 over HTTP the bash tool part stayed **`running`**, i.e. it was **not** auto-overwritten to `"Tool execution aborted"` (the source claim in the #15172 thread concerns the *question* tool / TUI-load path).
- **Recovery path works:** re-prompting the surviving session (`prompt_async`) produced a clean turn (`assistant: reasoning,step-finish` → text `recovery-ok`) with **no error**, despite the dangling `tool_use` in history. opencode reconciles the incomplete tool call at request-build time; its stored transcript keeps the part cosmetically `running`.
- Precision (issue research): #15172 as-filed is the **question** tool; the permission-specific restart-loss is **#36347** ("V2: preserve permission and question waits across server restart", **open**). Both unfixed in 1.18.3; fix PRs **#15603 and #20675 closed unmerged**. → **Sinet must persist every ask on `permission.asked`; a serve restart means "ask gone, session intact, re-prompt to recover (a paid turn)".** (P-T02-1 confirmed live.)

### Probe 3 — #16367 serve+attach hang under `ask`: **PRESENT / OPEN in 1.18.3, but orthogonal to Sinet (HTTP path empirically immune)**
Issue **open, not fixed** in 1.18.3 (candidate PR #20675 closed unmerged). Root cause is a **client-relay gap, not a server bug**: `opencode attach <url>` (interactive TUI against a running server) does not relay/render the server's `permission.asked`, so the deferred is never answered and the agent spins "thinking" forever. The server is correctly parked. Sinet does **not** use the attach-TUI — it drives serve over HTTP, subscribing to `/event` and replying via REST — and **probes 1 and 6 prove that path never hangs** (every park resolved and every turn resumed). The interactive TUI was not driven (needs a TTY); verdict rests on the tracker plus the measured HTTP-path immunity. Related and relevant: **#36835** — permissions from *TUI-initiated* turns can't be answered over HTTP (separate pending stores); so Sinet must originate **all** turns over HTTP and never share a serve with a TUI client, or some asks become unanswerable via REST.

### Probe 4 — abort → health → fork during a **RUNNING** turn: **PASS**
With a long-generation turn confirmed `{"type":"busy"}`: `abort` → `true`; `/api/health` → `{"healthy":true}`; `fork` → new session `"probe4-abort (fork #1)"`. The drill S3 could only test idle works **mid-turn**. (After abort the session drops out of the busy map → idle.)

### Probe 5 — SIGTERM finalizer visibility: **NEGATIVE (rejection is not observable)**
`SIGTERM` with one real pending ask outstanding emitted **no event** on v1 `/event`, **no event** on v2 `/api/event`, and **nothing** to stdout/stderr. The graceful-shutdown `RejectedError` (source-confirmed in S3) does **not** surface as a `permission.replied` or session-error event a client can see — the SSE sockets simply close. **The adapter cannot detect park-rejection from the bus; it must infer it** from its own ask-record plus the ask's disappearance from `permission.list` after restart.

### Probe 6 — Parallel-ask fan-out: **PASS** (+ two bonus findings)
glm-4.7 emitted **three parallel bash calls** in one turn; opencode parked all three **simultaneously** as a flat array — three independent `per_…` ids, shared `sessionID`, each `always:["echo *"]`. Replying `{"reply":"always"}` to **only the first** dropped `permission.list` to 0 and the bus emitted **three** `permission.replied` events (all `reply:"always"`, one per `requestID`) — a single pattern-matching `always` **cascades to siblings**. All three echoes ran.
- **Bonus A — v1 `always` rules are server-wide across sessions:** the `echo *` rule granted here later auto-approved an `echo` ask in a *different* session (my first restart-probe attempt didn't park because of it). Combined with S3's source note, v1 `always` is **in-memory + global to the serve process** and cleared on restart.
- **Bonus B — permission events ride v1 `/event` only:** across the whole run, **5 `permission.asked` + 4 `permission.replied`** landed on v1 `/event`; **zero** on the global v2 `/api/event`. The `PermissionV2*` schemas exist but the global v2 stream doesn't emit them in 1.18.3.

### Probe 7 — Z.AI prompt-unit calibration (R09-OQ4): **measured wire-side; prompt-unit denomination needs the dashboard**
Per-request `usage` is exact and detailed. glm-4.5-air (thinking default):
```json
{"prompt_tokens":12,"completion_tokens":720,
 "completion_tokens_details":{"reasoning_tokens":717},
 "prompt_tokens_details":{"cached_tokens":0},"total_tokens":732}
```
glm-4.6 with `"thinking":{"type":"disabled"}`:
```json
{"prompt_tokens":12,"completion_tokens":2,"completion_tokens_details":{"reasoning_tokens":0},"total_tokens":14}
```
- **No per-prompt / quota counter exists on the wire.** Response body keys are exactly `[choices, created, id, model, object, request_id, usage]`; response headers carry only `x-log-id` (a trace id) + HSTS — **no `x-ratelimit-*`, no remaining-prompts field**. Confirms report 09.
- **No wire-side usage endpoint:** `GET …/usage`, `…/user/usage`, `…/dashboard/usage`, `…/v1/usage` all **404**. The 5-hour window % is not retrievable via the coding API — the dashboard is the only source (operator step below).
- **opencode's own accounting** exposes per-message `.tokens.{total,input,output,reasoning,cache.{read,write}}`; one sampled turn: `total 8668` with `cache.read 8320` (heavy prompt caching in play — the ⚙0.1× cache-read weight, Def.10, is material here). opencode's `.cost` is **$0** for every call — models.dev prices the `zai-coding-plan` rows at zero (flat rate).
- **`thinking:{type:disabled}` is a large consumption lever** (732→14 tokens on the same trivial task): a real Z.AI-lane Eco-mode knob for the effort ladder.
- `request_id` in every response is the correlation handle for later dashboard/support reconciliation.

### Probe 8 — Logprob probe (R12 §2.5/§4.5(d)): **NEGATIVE — no logprobs on the Z.AI coding endpoint**
`logprobs:true, top_logprobs:5` on **two** models (glm-4.5-air; glm-4.6 thinking-disabled) → **HTTP 200** but `choices[0]` has **no `logprobs` key** (silently ignored, endpoint-wide). Therefore the **Z.AI coding lane gets no ~free logprob drift-canary → behavioral-eval-only.** Cross-referencing report 16 (local llama.cpp/Ollama logprobs confirmed, not re-run here): among v0 lanes, **only the local lane** gets the logprob canary; **both subscription lanes** (Z.AI here; Anthropic Max is CLI-wrapped) fall back to scheduled fixed-prompt behavioral evals (report 12 §4.5 b/c).

---

## Implications for the T07 adapter spec

1. **Event subscription = v1 `GET /event`** for `permission.asked`/`permission.replied` (the global v2 `/api/event` does not carry them in 1.18.3). Parse both generations tolerantly, but subscribe to v1 for parks.
2. **Reply via `POST /permission/{id}/reply {reply, message?}`** (confirmed: returns `true`, emits `permission.replied`; keeps reject-with-feedback per S3). **Never send `"always"` through opencode** — v1 `always` rules are in-memory + server-wide (Bonus A/B): they leak approvals across a person's sessions and vanish on restart. The adapter should emit only `once`/`reject` and keep persistent allow-policy in Sinet's own layer.
3. **Ask persistence is mandatory and now live-confirmed** (Probe 2): persist on `permission.asked`; treat restart as "ask gone (#15172/#36347, unfixed), session intact, re-prompt to recover." Re-prompt **works** despite a dangling `tool_use`, so recovery is a normal paid turn — but the adapter should reconcile *its own* checkpoint (mark the orphaned call cancelled); do **not** trust opencode's transcript, which leaves the part cosmetically `running`.
4. **Shutdown rejection is silent** (Probe 5): don't wait for an event to learn a park died; diff the ask-record against `permission.list` after any serve restart/health blip.
5. **Fan-out** (Probe 6): enumerate **all** entries in the flat `permission.list` array; expect one reply to fan out to N `permission.replied` events — key everything by `requestID`.
6. **Drills validated mid-turn** (Probe 4): health = `GET /api/health` (**needs Basic auth**), abort = `POST /session/{id}/abort`, fork = `POST /session/{id}/fork` — all work while `busy`.
7. **#16367 is orthogonal** (Probe 3): the HTTP-driven adapter is immune; the constraint is architectural — originate every turn over HTTP, never attach a TUI to a Sinet-owned serve (#36835).
8. **Auth:** `OPENCODE_SERVER_PASSWORD` ⇒ HTTP **Basic** on *every* endpoint including health; the adapter's client and health-watcher must both send it.

## Implications for T09 metering

1. **Tier-1 metering works on both surfaces** — exact `usage` on the raw wire and exact per-message `.tokens` (incl. cache read/write) in opencode. Cache reads dominate (8320/8668 in the sample); the ⚙0.1× cache-read weight (Def.10) is the single most impactful pressure-gauge assumption on this lane.
2. **Prompt-unit consumption stays approximation tier 3 (derived), dashboard-calibrated** — there is no per-prompt counter and no usage endpoint on the wire (Probe 7). R09-OQ4 is *partially* answered: everything measurable wire-side is now measured; the request→prompt ratio and multiplier posting genuinely require the operator dashboard (step below).
3. **Never meter dollars from opencode** — it prices `zai-coding-plan` at $0. The ledger must price Z.AI usage from Sinet's table using the **metered `zai` rows** for API-equivalent (report 09 §4.6), never the zero-rate coding-plan rows.
4. **`thinking:{type:disabled}` is a first-class Z.AI effort knob** — ~50× token reduction on trivial work; wire it into the Eco/Balanced ladder (report 09 §4.5) as a Z.AI-lane lever alongside model choice.
5. **`request_id`** is the reconciliation key between Sinet's ledger rows and the z.ai dashboard/support.

**Drift-canary (report 12):** Z.AI coding lane = **behavioral-eval-only** (no logprobs). Only the local lane gets the logprob canary among v0 lanes.

---

## Blocked items — literal operator steps

**R09-OQ4 residual — Z.AI prompt-unit ↔ request/token mapping (needs the z.ai dashboard; ~2 min):**
1. In a browser, log in at `https://z.ai` and open the **GLM Coding Plan** usage/dashboard page showing the **5-hour window** consumption (the "prompts used / % of window" meter).
2. Record the current window figure (e.g., "X% used" or "N prompts used"). Timestamp it (UTC).
3. Tell the spike runner a green light; it will fire a **known, counted batch** (e.g., exactly 20 `glm-4.7` requests, thinking-on) against the coding endpoint and record token totals from each `usage` object.
4. Refresh the dashboard; record the new window figure and timestamp.
5. Report both deltas to the runner: `Δprompts` (dashboard) vs `Δrequests`=20 and `Δtokens` (wire). This yields **prompts-per-request** and shows whether tier/peak **multipliers** post as extra prompts or extra tokens. (Reference: the prior stopped run saw ~11 requests ≈ 67,575 tokens move the window 0%→1%, reset ~21:40 UTC — re-confirm the reset cadence too.)

**Optional (not required for Sinet):** to witness the #16367 hang first-hand, run `opencode serve` then `opencode attach http://127.0.0.1:<port>` in a real terminal and send a tool-triggering prompt under `ask` — it will spin forever. Sinet's HTTP path is immune, so this is illustrative only.

---

## Cost / consumption disclosure

- **Cash: $0.00** (flat-rate subscription; opencode-priced cost $0.00 on every call).
- **Paid model calls: ~10** metered against the GLM Coding Max plan — 2 direct completions (measured **exactly 732 + 14 = 746 tokens**) + 8 opencode-driven turns (Probe 4 aborted early; Probe 1 initial+continuation; Probe 6 initial+continuation; Probe 2 echo-turn; Probe 2b pwd-turn SIGTERM'd before continuation; Probe 2 recovery). Tool-bearing turns are 2 model calls each. One sampled turn ran 8,668 tokens (mostly cache-read).
- **Magnitude:** comparable to the prior run's 11 calls ≈ 67,575 tokens ≈ **~1% of the 5-hour window**; exact % pending the dashboard step. GET metadata calls (`/models`, usage-endpoint 404 probes) are free (not prompts).
- **Hygiene verified:** isolated XDG (0700); `auth.json` **shredded**; isolated data dir wiped; scratchpad confirmed key-free (`grep -rlF`); operator's real opencode dirs show **0 files modified** since spike start (all four); serve down, **port 43117 free**, all capture curls killed. No git changes; nothing in `Docs/` or engine source touched.
