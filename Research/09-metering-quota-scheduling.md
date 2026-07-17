# T08 — Consumption metering, quota handling & scheduling

**Report date:** 2026-07-17 · **Depth:** FULL (Wave B1) · **Method:** deep-research harness — 5 fan-out search angles (provider usage/limit signals · park-resume/retry/ceilings · budgets/admission/effort modes · price tables/receipts · single-host scheduling/GPU arbitration), primary-source fetching, 3-agent adversarial verification pass over the 22 most load-bearing claims (verdicts: **15 SUPPORT, 6 PARTIAL with corrections folded in, 1 REFUTE** — the refuted claim was this brief's own novelty conjecture, narrowed in §2.9). All URLs accessed 2026-07-17 unless noted. Binding inputs consumed, not re-researched: `Research/decisions/GATE-1-architecture-direction.md`, spikes `G1-S1/S2/S3`, reports 01 (engines) and 02 (providers). Environment caveats: freedesktop.org and help.openai.com 403 the fetcher (Debian/Arch man-page mirrors and OpenAI's own forum quotes used, marked); x.ai unreachable as in report 02.

---

## 1. Scope

Feature-list items covered (per brief T08): **3.1–3.11** complete (exact metering; limit events as scheduling events; automation budgets; per-requester billing; effort modes; honest receipts incl. done-directly comparison; hard ceilings; disaster economics; never-fully-offline; opt-in metered; local-resource arbitration), **D4**, **D5**, **2.8** (missed-slot policies), **S2.5** (cost observability), **3.4** (attribution).

Constraints researched within: D4 (observe, never model quota windows), D5 (two currencies), 3.10 (metered = explicit flag; exception list EMPTY at v0 per G1/P7), D2 (per-person metering), adopt-don't-fork. G1-ratified facts treated as fixed: v0 lanes = **Anthropic Max + Z.AI GLM Coding Max only**; dual substrate (pinned `opencode serve` + wrapped `claude -p` per user); **wrapper ledger = primary ceiling, engine flags ≥2× backstop**; zero-cost poll-resumes never count as attempts or spend; cache-read pressure weighting ⚙0.1× "assumed"; stage budgets ⚙50 %/70 %; zero-interaction threshold ⚙$0.50; hold-vs-park ⚙10 min; P1 pre-registered un-pause response.

---

## 2. Current state of the art (mid-2026)

### 2.1 Usage extraction per substrate — exact metering is achievable on every v0 lane

**Anthropic lane (wrapped `claude -p`).** Spike S1 measured the load-bearing facts: every `assistant` event carries complete per-call usage (`input_tokens`, `output_tokens`, `cache_creation_input_tokens`, `cache_read_input_tokens`, TTL buckets), and the result envelope carries `total_cost_usd` + per-model `modelUsage.costUSD` under subscription auth, identically on CLI and SDK [S1 F1/F3]. This report adds the API-level accounting rule the ledger must encode: **`input_tokens` counts only tokens after the last cache breakpoint** — total prompt = `cache_read + cache_creation + input_tokens` [1]. Newer usage fields exist and should be stored pass-through: `output_tokens_details.thinking_tokens` ("`output_tokens` remains the inclusive, authoritative total used for billing"), `server_tool_use` (web search/fetch request counts — priced per use, outside token math), `service_tier`, `inference_geo` [3].

Two hard caveats, both verified from Anthropic's own docs (adversarial verdicts SUPPORT):

- **Engine dollars are estimates, not truth.** "The `total_cost_usd` and `costUSD` fields are client-side estimates, not authoritative billing data. The SDK computes them locally from a price table bundled at build time… Do not bill end users or trigger financial decisions from these fields" [4]. Sinet's own price table (§2.6) is the D5 reporting authority; engine figures are cross-checks.
- **The result `usage` field undercounts under nesting**: it "counts only the top-level agent loop" while `modelUsage`/`total_cost_usd` include subagents; and parallel tool calls emit multiple assistant messages sharing one message `id` with identical usage — **deduplicate by id** [4]. (Engine-native subagents are disabled per the G1 sole-controller rider, which removes most of this hazard; the dedup rule still applies.)

**Field drift is a value problem, not just a schema problem.** Report 01 established schema drift (P-T01-2). The 12-month record on the *cost/usage values*: v1.0.9 removed `costUSD` from session JSONL, breaking downstream tools [13]; the SDK package/namespace break (late 2025); live correctness bugs — stream-json duplicating usage stats across content blocks ("3–8×" inflation, #6805), `total_cost_usd` ~10× too high on Haiku-class (#53371), and truncated `-p` output dropping the result message entirely (fixed 2.1.208) [5, 17]. Three naming conventions coexist today across adjacent surfaces (`total_cost_usd` / `costUSD` / `cost.total_cost_usd`). Consequence: the ledger needs **sanity bounds and cross-check alarms on values**, not only tolerant parsing (new problem P-T08-1, §7).

**Z.AI lane (opencode serve).** opencode 1.18.3 stores typed cost/token columns (input/output/reasoning/cache read+write) as first-class SQLite session fields [S3 F3] and reports per-message tokens over the API (report 01 §2.3). The Anthropic-/OpenAI-compatible Z.AI endpoints return standard usage objects. What Z.AI does **not** expose: any per-prompt quota counter — the plan is denominated in "prompts" (~15–20 model calls each, G1 data) but the wire carries only per-request token usage. Sinet can meter requests and tokens exactly; **prompt-unit consumption is approximation tier 2** (see hierarchy below), calibratable only against the subscription dashboard (OQ4).

**Local lane.** Exact metering is native everywhere: llama.cpp server returns OpenAI-compat `usage` incl. `prompt_tokens_details.cached_tokens`, supports `stream_options.include_usage`, and a `timings` object with `prompt_n`/`predicted_n` and per-second rates [9]; Ollama returns `prompt_eval_count`/`eval_count` plus nanosecond durations [10]; vLLM supports streamed usage and Prometheus `/metrics` [11]. Local tokens cost no allowance (Operating reality) but feed 3.11 arbitration (§2.8) and receipts (at $0, with utilization noted).

**The honest approximation hierarchy** (for the 3.1 ledger, best-first — each row labeled on receipts):

1. **Measured per-call usage** from the substrate's stream/DB (both v0 lanes + local — the normal case).
2. **Engine-computed aggregates** (`total_cost_usd`, opencode session cost) — cross-check tier; alarm when tier 1 × price table diverges >20 % from tier 2 (catches both price-table staleness and engine bugs).
3. **Derived plan units** (Z.AI prompts ≈ requests; peak/off-peak multipliers applied as documented data) — labeled "derived"; calibrated against the provider dashboard on a schedule.
4. **Tokenizer estimates** (only for pre-run cost gates where no measurement exists yet — the 2.5 size guess).
5. **Unknown — flagged, never silent-zero.** The LiteLLM record is the cautionary tale: unknown models silently show $0.00 in dashboards, and a 2026-01-27 malformed-price-map incident silently zeroed cost tracking [48, 49]. A Sinet ledger row that cannot be priced renders as "UNPRICED" on the receipt and raises a drift alert — never $0.

### 2.2 Limit-event taxonomy — five classes, all observable on the wire

The field's signals sort into five classes. Everything below is primary-sourced; this taxonomy is the N3 generalization the brief asked for.

**Class 1 — transient shed (retry in place).** Anthropic API: 529 `overloaded_error` is documented as distinct from 429 ("can occur when the API experiences high traffic across all users") [2]; "acceleration limits" can 429 sharp ramps even below steady-state limits [1]. Claude Code ≥2.1.199 (subscription): "Transient server rate-limit errors (429s **unrelated to your usage limit**) are now retried automatically with backoff for subscribers" — official confirmation that subscription 429s arrive in two distinguishable flavors [5]. Z.AI: codes **1302** ("Rate limit reached for requests") and **1305** ("service may be temporarily overloaded") — community-measured to recover on 1 s retries [6, 24]. Action: bounded in-place retry with full jitter (§2.3); never a park.

**Class 2 — depletion with reset signal (park until signal).** Anthropic API: 429 + `retry-after` (seconds) + `anthropic-ratelimit-*` headers (requests/tokens/input/output × limit/remaining/reset, reset in RFC 3339; token bucket, continuously replenished) [1]. Anthropic subscription surface: the in-band `rate_limit_event` with `resetsAt`/`rateLimitType`/`overageStatus` [S1 F1] — the park scheduler's machine-readable resume time. Z.AI: **1308 "Usage limit reached… Resets at {next_flush_time}"** and **1310 "Weekly/Monthly Limit Exhausted… reset at {next_flush_time}"** — reset times in-band, verified from the official error table [6]; plus 1316–1321 (5-hour/7-day/spend-cap variants). opencode surfaces provider limit state as `GET /session/status` `retry` variant with `attempt`/`next`/provider-`action` [S3 F2]. Action: park with provider-signaled resume time.

**Class 3 — depletion without signal (park on probe schedule).** Z.AI **1113** ("Insufficient balance or no resource package") persists until window reset but carries no time — and is **ambiguous**: it also fires when a valid coding-plan key hits the wrong base URL [24]. Undocumented concurrency throttles (community: GLM Pro ~1 concurrent, surfacing as generic "Too Many Requests" at 4 % of quota [14 in angle sources; single-ecosystem]) look like depletion but are really admission caps. Action: park with jittered probe schedule + endpoint-config self-check before believing "depleted".

**Class 4 — auth/policy events (freeze lane, alert operator — never retry-park).** Anthropic API separates cleanly: 401 revoked/expired, 402 billing, 403 permission, 429 limits [2]. But the two documented policy-enforcement arcs ride *valid* credentials: Anthropic's OAuth ban returns "This credential is only authorized for use with Claude Code…" on an otherwise-valid token [19, 20], and xAI's allowlist gating returns plain 403 on an active paid plan (report 02, P-T17-1). LiteLLM even returns budget exhaustion as HTTP 401 ("Authentication Error, ExceededTokenBudget…") [44] — proof the ecosystem conflates these classes. Sinet's classifier must not: auth-shaped errors → P-T17-1 canary path (lane freeze + operator alert), with message-text matching as a secondary signal (new problem P-T08-2).

**Class 5 — engine ceilings (died-at-gate handling).** `error_max_budget_usd`/`budget_exhausted` from the wrapped CLI; a same-turn budget trip pre-empts a defer park even after the gate fired [S2 F4]. These are Sinet's own backstops tripping, not provider events — handled by the wrapper-primary ceiling rule (§4.8).

One negative finding worth recording: Anthropic's **monthly spend-cap** rejection has no documented HTTP status or error type ("API usage pauses until the next month") [1, verified] — if metered exceptions are ever enabled, the adapter must discover this empirically.

### 2.3 Park-and-resume & retry discipline — nobody ships subscription-window parking; the primitives all exist

**The gap is real.** Claude Code hard-stops when the subscription window exhausts — no auto-wait, no auto-resume; the standing feature request (#35744, open; #36320 closed as its duplicate) documents "Claude Code hard-stops with no recovery path", and third-party wait-and-continue wrappers exist as workarounds [20, 21]. opencode's 429 handling is worse for unattended use: retries ~every 1 s indefinitely, SDK-internal retries stacking under session retries, no max count (#30510) — plus separate reports of hanging forever or terminating instead of retrying [22, 23]. **Neither v0 engine can own limit handling; the wrapper must** — confirming report 01's adapter split and pricing in the S3 finding that opencode pending asks are in-memory-volatile.

**The primitives are solved elsewhere.** Temporal's durable timers are the canonical park pattern ("Timers are persisted… a Workflow can sleep for months"; parking on external signals = timer-vs-signal race) [8 in angle-2 list]; conventional queues (Sidekiq: `retry_count⁴ + 15 + jitter`, 25 retries then Dead set; BullMQ exponential + jitter) show the delayed-retry shape [15, 16]. Retry discipline standards: Retry-After in both delay-seconds and HTTP-date forms [28]; AWS full jitter `sleep = random(0, min(cap, base·2^attempt))` with the verdict that jittered backoff "should be considered a standard approach" [29]; Google SRE retry budgets — per-request cap (~3), per-client retry ratio ≤10 %, retry only at the layer immediately above the rejection [30]; circuit breaker for repeated failure (closed→open→half-open) [31]. **gh-aw is the closest agent-native prior art for layered guardrails**: 20-min default agent timeout, `stop-after` evaluated before execution, and a daily AI-credit threshold (default ~$50/day) that fails a run *at spawn* if the trailing-24 h spend exceeds it [37, verified].

**Interaction with checkpointing (T07 bridge).** Parking is cheap and lossless on both lanes: defer-park exits carry the full pending call in `deferred_tool_use`, resume re-fires the same `tool_use_id`, and poll-resumes cost $0 [S2 F1/F2b]; opencode sessions survive restarts in SQLite [S3 F3]. The economics are measured: subscription cache TTL is 1 h and a cold resume costs ~6–16× a cached one [S1 F3] — so the park scheduler should prefer resuming inside the cache window when queue pressure allows, and the ⚙10-min hold-vs-park default (G1 Def.4) is consistent with that cliff.

### 2.4 Budget enforcement & admission control — the field's budgets are dollar-shaped and leaky

**LiteLLM proxy** is the richest budget feature set (key/user/team hierarchies, `budget_duration` aligned resets, `soft_budget` alerts at 85 %/95 %, and a *projection* event `projected_limit_exceeded`) [44, 47] — and a documented multi-issue enforcement-gap history: global budget limiter instantiated but never registered (#27381), end-user budgets allowing unlimited spending (#11083), org budgets never resetting (#20532); all now fixed, but the family establishes the difficulty class: **admission checks against post-call-updated counters are not hard ceilings** — the docs' own opt-in `fail_closed_budget_enforcement` flag concedes the default hot path can overshoot [44, 48]. This matches Sinet's own measured granularity fact (one model call; R05's 19× overshoot on a $0.001 cap) — ceilings are only as fine as one call anywhere.

**OpenAI removed hard budget limits**: current primary docs describe only notification thresholds; project budgets are "soft spending thresholds" — "API requests will continue to be processed without interruption" (verified against OpenAI's own pages + forum) [51, 52]. Budgets that only notify are not budgets. **OpenRouter** has real admission-time per-key credit caps (`limit`/`limit_remaining`/`limit_reset`, 402 on exhaustion) [53]. **Anthropic Console** workspace spend limits exist but have no Admin API (the new Spend Limits API is Enterprise-only and per-seat) [14, 15, 16].

**Headroom reservation prior art.** Google SRE's criticality × quota is the canonical citation for 3.3: CRITICAL_PLUS/CRITICAL/SHEDDABLE_PLUS/SHEDDABLE, with "a backend task will only reject requests of a given criticality if it's already rejecting all requests of all lower criticalities" when a customer exhausts quota [30, verified]. LiteLLM's `priority_reservation` + `saturation_threshold` (guaranteed share per priority class, borrowing allowed only under low saturation) is the same idea over model TPM/RPM [45]. Kubernetes ResourceQuota (admission) + PriorityClass preemption is the systems analogy [54]. **Nobody applies this to subscription windows** — that composition is Sinet's to build (§2.9).

**One-switch pause semantics** — three shapes exist in the wild, and they disagree: Home Assistant halts in-flight by default (`stop_actions` defaults true, with a finish-option flag) [59]; GitHub Actions disable is future-only (docs silent on in-flight) [60]; Zapier **drops** pending delayed work whose timer expires while off ("won't continue… and won't resume") [61, verified with corrected wording]. The Zapier shape is a data-loss footgun 3.3 must design against: pause must preserve queued and parked work.

### 2.5 Effort modes & disclosed degradation — vendors converged on effort enums and quota-triggered demotion; disclosure is the weak point

All three majors now ship an effort knob: OpenAI `reasoning.effort` (none→xhigh) [56]; Anthropic's `output_config` effort (low/medium/high/max; `budget_tokens` deprecated on 4.6-class) — notably it "controls overall token spend for text responses and tool calls", a general consumption lever, not just thinking depth [13]; Gemini `thinkingBudget`/`thinkingLevel` [57]. Product-level depletion demotion exists inside single plans: ChatGPT switches to mini-model fallbacks when usage limits are reached ("won't appear in the model picker") [58]; Claude Code auto-falls-back Opus→Sonnet at a usage threshold with a one-line notice — and issue #3434 ("Claude **silently** falls back… breaks workflows… Please allow opt-out") documents exactly the UX failure Sinet's 3.5 "downgrades gracefully **and says so**" must avoid [12, 17]. The consumer-OS patterns (Android Data Saver's per-app background restriction with persistent status-bar disclosure; Battery Saver's depletion-triggered degradation with explicit mode indicator) are the right disclosure prior art: **a mode change is a visible state, not a log line** [55].

### 2.6 Price tables & receipts — a solved input problem, an open output niche

**Machine-readable price sources, compared** (full audit in angle sources; verification closed the gaps):

| Source | Fit for shipped defaults | Key facts |
|---|---|---|
| **models.dev** (`anomalyco/models.dev`) | **Best** | MIT (GitHub API-verified); TOML per model, CI-validated community PRs; `cost.input/output/cache_read/cache_write/reasoning` (+ `context_over_200k`, `tiers`) in **USD/MTok**; single `api.json` (3.2 MB, 167 providers). **Z.AI present as 4 providers — but `zai-coding-plan` rows are all-zero** (flat-rate encoding); the metered `zai` rows carry real prices matching first-party docs exactly [68] |
| LiteLLM `model_prices_and_context_window.json` | Cross-check | Widest coverage, per-token units; documented staleness/$0.00 failure modes + the Jan 2026 silent-fallback incident [48, 49, 50] |
| OpenRouter `/api/v1/models` | No | Marketplace/route prices, not first-party list prices; string per-token values [69] |
| Artificial Analysis API | No (as source) | 1,000 req/day free tier, **attribution required** — awkward to redistribute [70] |
| Langfuse model definitions | Pattern, not data | The design template: shipped defaults + user-editable overrides + **conditional pricing tiers** (e.g., >200 K premium); defaults maintained by hand in a JSON [71] |
| Aggregators (pricepertoken etc.) | No | Quote cheapest-third-party-host prices: GLM-5 "$0.60" vs first-party $1.00 — the silent-mixing trap [74] |

**Current first-party prices for the v0 table defaults** (primary, 2026-07-17; USD/MTok in/out): Anthropic — Fable 5 $10/$50, Opus 4.8 $5/$25, Sonnet 5 **$2/$10 introductory through 2026-08-31, then $3/$15**, Haiku 4.5 $1/$5; cache write 1.25× (5 m) / 2× (1 h), cache read 0.1×, Batch −50 %, multipliers stack; web search $10/1k [8]. Z.AI — GLM-5.2 $1.4/$4.4 (cached input $0.26), GLM-5 $1.0/$3.2/$0.20, GLM-4.7 $0.6/$2.2/$0.11; cache storage currently "Limited-time Free" (a promotional zero, not an absent concept) [23]. Drift in 12 months: a 3× Opus cut, a **pre-announced future-dated price change** (Sonnet 5, 2026-09-01), fast-mode pricing introduced then partially killed, GLM flagship price raised twice. Consequence: the price table needs **effective-date rows** (new problem P-T08-3) and the report-02 §6 watch cadence covers refresh.

**Cost-engine pitfall checklist** (each independently documented): Anthropic `input_tokens` excludes cache tokens while OpenAI-style `prompt_tokens` includes them — normalize or double/under-count (Langfuse normalizes at ingestion) [71]; thinking tokens billed as output [4]; server tools priced per-use outside token math [3, 8]; batch flag halves token prices [8]; TTL-blind cache-write pricing is a known approximation (ccusage's documented fallback) [73]; unknown model ⇒ never $0 (§2.1 tier 5).

**Receipts.** The nearest itemization prior art is Claude Code `/usage`: per-model token/cost breakdown **plus attribution of usage to skills, subagents, and MCP servers as percentages** — per-purpose itemization exists in the field [7]. ccusage computes API-equivalent dollars from subscription JSONL (auto/calculate/display modes) but ships **no value-vs-plan-price comparison** [73]. Adversarial search found no tool anywhere presenting a per-run "what this would have cost done directly" counterfactual — CI cost tooling (BuildBuddy/Depot) doesn't do per-run receipts either. **Spec 3.6's done-directly figure is an open niche**: buildable, but the formula must be defined and pre-registered by Sinet (OQ3).

### 2.7 Scheduling machinery & missed slots — the vocabulary exists; the host profile decides the shape

**Missed-slot policy prior art maps 1:1 onto spec 2.8.** Quartz's misfire instructions are the richest published vocabulary [79, verified]: fire-once-now (= **run-once-late**: "a per-minute cleaner missed 20 times should run once, not 20 times"), do-nothing (= **skip**: stale data worse than a gap), ignore-policy/replay-all (Sinet doesn't need it). APScheduler adds `misfire_grace_time` + `coalesce` [80]; systemd `Persistent=` is hardwired coalesced fire-once-now, and calendar timers elapsing during sleep are caught up on resume, coalescing multiple missed elapses into one activation [76]. The one reported catch-up failure across suspend (#24984) was **closed as a fixed regression in 2022** [77] — the semantics are trustworthy on a current Ubuntu; a one-hour sanity test on the actual laptop remains cheap insurance. `WakeSystem=` requires system-level privileges (user timers cannot wake the machine) [76]. **Notify-only** has no direct prior art — it is a card instead of a run, trivially Sinet-side. anacron is the classic run-once-late daemon (day granularity); cron's no-catch-up default is the skip shape [78].

**Queue engineering.** SQLite-backed job queues are established practice at far beyond household scale (Solid Queue supports SQLite — "writes are sequential" so SKip-Locked degrades harmlessly; goqite/litequeue show the visibility-timeout and mark-locked-UPDATE patterns; one implementation claims 15 k jobs/s) [81, 82]. Nexus's own N2 (atomic CAS claiming + per-model slot gates, ~300 lines) is the same pattern already debugged against this exact workload. Fairness at small scale: priority starvation is real; the standard mitigations — aging, weighted-fair shares, lottery — port directly to dequeue policy [91].

### 2.8 Local resource arbitration — "operator always wins" is mainstream desktop practice; the GPU arbiter is the inference server

**CPU/RAM/IO: cgroups v2 via systemd is the whole answer.** `CPUWeight` (range 1–10000; special value **"idle"** — CPU only when nothing else wants it — the batch-sandbox setting), `MemoryHigh` as "the main mechanism to control memory usage" (throttle + reclaim) with `MemoryMax` as "the last line of defense", `IOWeight` [83, verified]. **uresourced is direct prior art for D-operator-wins**: Fedora allocates the *active* user's session a protected memory guarantee (250 MiB, capped at 10 % RAM) and a ~5× CPU-share boost, rationale verbatim: "Graphical sessions should always be responsive, even when the machine is doing a lot of work" [84]. PSI (`cpu/memory/io.pressure`, some/full, userspace triggers) is the arbitration signal for shedding background load before the desktop notices [85]. This composes with the G1-ratified systemd transient units per engine process (Def.6): Sinet places sandboxes/inference in weighted slices and reacts to PSI.

**GPU/VRAM: no partitioning on consumer NVIDIA — serialize at the server.** MIG is datacenter-only; MPS memory limits (`CUDA_MPS_PINNED_DEVICE_MEM_LIMIT`) are enforced at allocation time but opt-in and not hardware isolation (verification corrected "advisory") [86]; NVIDIA time-slicing provides no memory isolation [87]. The practical arbiter is the inference server's own queue: **Ollama** queues requests until idle models unload ("as prior models become idle, one or more will be unloaded to make room"), with `keep_alive` (default 5 min; −1 pin / 0 immediate unload), `OLLAMA_MAX_LOADED_MODELS` (3×GPU count), `OLLAMA_NUM_PARALLEL` (1), `OLLAMA_MAX_QUEUE` (512) [26, verified]; **llama.cpp server** slots + deferred-request queue (note: current default is `--parallel -1` auto with a unified KV buffer — the old fixed per-slot context split applies only when slots are explicit) [25, verified]; **vLLM** preempts (V1 default RECOMPUTE) when KV cache is insufficient, queue depth observable via `vllm:num_requests_waiting` [27]. Model swap costs are seconds-scale (community figures: ~2–5 s for 7B from NVMe; wide variance — measure on the actual hardware), and desktop compositor VRAM headroom is real but undocumented (community: hundreds of MB to ~1–2 GB on Wayland) [89, 90]. "Pause local inference when the operator is active/gaming" has **no published turnkey implementation** — GameMode's custom start/end scripts + `ollama stop`/`keep_alive 0` are the documented primitives it would be glued from [88].

### 2.9 Consumption-pressure routing between flat-rate lanes — prior art EXISTS; the brief's novelty guess was wrong, narrowly

The brief conjectured this layer is "likely novel." Adversarial verification **refuted the conjecture as written**: two publicly documented systems route proactively on subscription-window consumption. **OmniRoute** (README + `docs/routing/QUOTA_SHARE.md`, v3.8.40, updated 2026-06-28) ships routing strategies `headroom` ("pick the target with the most remaining quota") and `reset-window`, plus a Quota-Share layer: "Deficit-Round-Robin scheduling… multi-window usage buckets (5 h / 7 d / per-model)… proactive saturation from upstream token-usage headers", enforced "in the hot path **before** the request leaves OmniRoute", with a soft-deprioritize factor (0.7) [62]. **teamclaude** rotates between Claude accounts at a configurable threshold (default 98 %) of the 5 h/7 d window using `anthropic-ratelimit-unified-*` headers [63]. (An earlier fetch of OmniRoute's USER_GUIDE.md — which genuinely omits Quota-Share — produced the wrong "reactive-only" read; resolved by fetching the README and the dedicated doc.)

Three readouts:

1. **What remains novel for Sinet** — the workload-class policy layer: reserved *interactive* headroom plus disclosed *background* demotion, per person. Neither system has it: OmniRoute arbitrates between team members; teamclaude between accounts. The SRE criticality×quota model [30] is the design template for exactly this composition.
2. **The prior art is partially D-incompatible and study-only.** teamclaude's purpose is multi-account rotation — precisely the pooling D2 bans. OmniRoute's DRR-over-window-buckets works by *tracking window state from upstream headers and window shapes* — it sails close to D4's line; Sinet takes the observed-signals half (headers, reset messages) and leaves the window-bucket modeling (§5). Both are STUDY inputs for mechanism, never dependencies.
3. **A single-source lead worth one probe:** teamclaude's README implies `anthropic-ratelimit-unified-*` response headers exist on subscription-authenticated traffic. If real, that is additional *provider-signaled* observed state (D4-clean). Unverified anywhere else — flag for the implementation-phase probe battery.

---

## 3. Candidate approaches

**A. Metering architecture.**
- **A1 — Wrapper-ledger-first (chosen):** Sinet's control plane writes one usage row per paid call into the T07 SQLite-WAL event log, from stream events (Anthropic lane) and session polling/SSE (opencode lane); engine aggregates stored as cross-checks. This is D7's checkpoint row carrying usage — one write, two purposes [S1/S3]. Cost: Sinet owns parsing drift (already priced into P-T01-2's conformance suites).
- **A2 — Gateway-in-the-middle (LiteLLM-class proxy meters all lanes):** rejected. The Anthropic lane cannot ride a proxy (first-party wrap required, report 01); D2 would demand one proxy per person; the budget-enforcement gap record [48] and budget-as-401 error shape [44] make it cautionary art; and it adds a dependency where the substrates already emit exact usage.
- **A3 — Observability platform (Langfuse-class) as the ledger:** rejected as system-of-record (another service to run; receipts and budgets are platform features, not dashboards) — but its model-definition/override/tiered-pricing design is the adopted *pattern* for the price table [71].

**B. Limit-event handling.**
- **B1 — Engine-native retries:** rejected on evidence (Claude Code hard-stop; opencode unbounded 1 s retries [20, 22]). Engine retry knobs are set to minimum/off where configurable; the wrapper owns classification.
- **B2 — Wrapper-owned five-class taxonomy (chosen):** §2.2's classes with per-class actions (§4.3). Cost: a per-lane classifier table that drifts with providers — maintained via the report-02 §6 watchlist and the conformance suite.

**C. Budget enforcement.**
- **C1 — Admission-only (gh-aw shape):** too coarse alone — a long run admitted under budget can far exceed it.
- **C2 — Admission + per-checkpoint enforcement (chosen):** spawn-time gate on estimated cost vs remaining budget and pressure band, plus a ledger check at every paid-call checkpoint (the finest granularity that exists anywhere); breach ⇒ park-at-checkpoint as `blocked_budget`, a normal scheduling state (N4 vocabulary). Engine `--max-budget-usd` set at ≥2× the wrapper remainder as backstop [S2].
- **C3 — Post-hoc accounting with alerts (LiteLLM default / OpenAI current):** rejected — notification-only budgets are not budgets [51].

**D. Pressure & routing between flat lanes.**
- **D1 — Window-bucket modeling (OmniRoute Quota-Share shape):** rejected on D4 (below, §5).
- **D2 — Sinet-defined budget denominators + provider-signaled overlays (chosen):** pressure per (person, lane) = weighted consumption ÷ **operator-declared automation budget** for a Sinet-defined period (defaults seeded from plan marketing shape at a conservative fraction, labeled "assumed"), overlaid with observed events: Class-2/3 depletion snaps the lane's pressure to 1.0 until the signaled/probed reset; Class-1 shed adds a temporary penalty; observed concurrency rejections cap the lane's slot gate. This is D4-clean: the denominator is Sinet's own budget (a 3.3 construct the operator edits), never an inferred provider window; reset times only ever come from provider signals.
- **D3 — Dollar-cost routing between flat lanes:** forbidden by D5; rejected.

**E. Effort modes.** Policy bundles (model duty map + vendor effort param + verification depth + retry allowance + lane/park preferences), with a depletion ladder whose steps are disclosed states (§4.5). Alternative "modes = model choice only" rejected: the measured levers (effort params, verification cascade depth, batch/off-hours placement) move more consumption than model identity alone.

**F. Price table.** models.dev api.json snapshot as shipped seed (metered `zai` + `anthropic` rows), reconciled against first-party pages at ship; user-editable rows with `verified-on` and **effective-date** support; Langfuse-style override semantics. LiteLLM map as CI cross-check only. Receipts price flat-rate usage at API-equivalent from this table (D5), flipping currency visibly for any metered-flagged run (P-T17-2).

**G. Scheduler.**
- **G1 — systemd timers as the schedule authority:** rejected for task schedules (missed-slot policy is per-schedule and richer than `Persistent=`; the control plane must own catch-up state to honor run-once-late/skip/notify-only and billing). systemd retains two roles: starting Sinet itself (and optional future WakeSystem, system-level), and `RuntimeMaxSec` transient-scope ceilings (Def.6).
- **G2 — Sinet-owned queue in the T07 store (chosen):** single SQLite priority queue, CAS claiming, per-lane/per-model slot gates, aging; scheduler ticks while the control plane runs and executes an anacron-style catch-up scan at startup/wake applying each schedule's missed-slot policy.

**H. Local arbitration.** systemd slices per class (interactive session > platform services > local inference > sandbox batch at `CPUWeight=idle`), `MemoryHigh` fences, PSI triggers to pause batch admission; GPU arbitration delegated to one inference server per GPU pool (Ollama or llama.cpp server per T15's outcome) with Sinet-side admission on a VRAM ledger (model footprints + measured compositor headroom); operator-wins hooks: eager-unload switch (manual + GameMode script hook at v0; idle-detection glue is post-v0 — no prior art exists).

---

## 4. Recommendation for Sinet

**One integrated design: meter every paid call into the platform's own ledger; convert consumption to pressure against operator-declared budgets; treat every provider signal as observed state; schedule everything — parks, budgets, ceilings, anomalies — as queue transitions in one SQLite-backed scheduler.**

**4.1 The ledger (3.1, S2.5, 3.4).** One row per paid model call, written at the D7 checkpoint: `{run_id, task_id, requester, owner_lane, substrate, model, usage{input, output, cache_read, cache_creation(+TTL buckets), thinking, server_tool_use…}, engine_cost_estimate, priced_cost{table_version, effective_date}, purpose_tag(ceremony|execution|verification|probe), approximation_tier(1–5), ts}`. Aggregations per person × model × task × day/week/month are queries, not new state. Rules: dedup parallel-tool-call messages by message id [4]; subagent accounting moot at v0 (sole-controller rider) but the dedup+`modelUsage` cross-check stays; engine dollar figures stored but never authoritative [4]; **no row silently prices to $0** — unpriceable rows render UNPRICED and alert (P-T08-1 guard: alarm when tier-1×table diverges >20 % from engine estimates). Every run bills its requester with ceremony itemized (3.4); the `/usage` per-capability-percentage pattern [7] is the presentation model.

**4.2 Two currencies, operationalized (D5).** Between flat lanes: the **pressure gauge** per (person, lane) from §3-D2 — weighted units (Anthropic: tokens with cache-read ⚙0.1× "assumed" [G1 Def.10]; Z.AI: requests as prompt-proxy with documented 3×-peak/2×-off-peak multipliers applied as data; both labeled with approximation tier) ÷ operator-declared automation budget per Sinet period, overlaid with observed limit events. Dollars: only the price table (4.6) for receipts and for the empty-at-v0 metered exceptions. Local lane has no pressure — it is arbitrated (4.9), not budgeted.

**4.3 Limit-event policy (3.2, D4) — the five classes map to actions:**

| Class | Signals (per §2.2) | Action |
|---|---|---|
| 1 Transient shed | 529; CC transient-429; Z.AI 1302/1305 | In-place retry: full jitter, per-request cap ~3, per-lane retry budget ≤10 %, circuit breaker on repeat [29, 30, 31]; never park, never count vs quota-storm |
| 2 Depletion + signal | `rate_limit_event.resetsAt`; `retry-after`/ratelimit headers; Z.AI 1308/1310 `next_flush_time`; opencode `retry.next` | Park `blocked_quota` with provider resume time; auto-resume; prefer resume inside the 1 h cache window when pressure allows [S1] |
| 3 Depletion − signal | Z.AI 1113 (after endpoint self-check); undocumented concurrency caps | Park with jittered probe schedule (cap ~30 min interval); probe resumes are zero-cost and never count [S2] |
| 4 Auth/policy | 401/402/403; policy text on valid creds | Lane freeze + operator alert (P-T17-1 canary); **never** retry-park |
| 5 Engine ceiling | `error_max_budget_usd`/`budget_exhausted` | Died-at-gate handling per S2: wrapper ask-record is authoritative; engine flags sized ≥2× so this class stays rare |

Once-per-storm logging (N3) carries forward: one drift/park card per storm per lane, not per attempt. Interrupted verification rounds never count as rework rounds (N3, now under T06's bounded-rework rule).

**4.4 Budgets & headroom (3.3).** Per-person, per-lane, per-period automation budgets as ⚙ settings (operator rider applies: defaults conservative — e.g., background ≤50 % of a plan's advertised window capacity, labeled "assumed" — auto-scaling only within operator ceilings). Enforcement: **spawn gate** (estimated cost vs remainder + pressure band; the $0.50 zero-interaction band [G1 D1.7] rides the same estimate) and **per-checkpoint gate** (breach ⇒ park `blocked_budget` at the checkpoint — resume when the period rolls or the owner raises the budget; both are cards). **Headroom rule (SRE-shaped):** interactive use is CRITICAL_PLUS — never blocked by automation budgets (3.3's "the platform's own consumption never blocks a person's interactive use"); background classes shed lowest-first as pressure rises; background admission stops at pressure ⚙0.7 (aligned with the ratified 70 % stage-overflow event), leaving the remainder as interactive headroom. **One-switch pause:** stops the person's background admission, parks in-flight runs at their next checkpoint (HA-style halt with a finish-current-stage option), and **preserves everything queued and parked** — the Zapier data-loss shape is a named anti-behavior with a test (P-T08-4).

**4.5 Effort modes as depletion policies (3.5).** Eco = adequate at minimum consumption (smallest capable model, effort low, mandatory-only verification, no retries, off-hours placement). Balanced = best per unit (duty-map default, effort medium, cheap-first cascade, one retry). Smart = best within budgets (frontier lane, effort high/max, full two-axis verification, retries within ceilings, **parks on depletion rather than degrading below its quality floor**). The downgrade ladder — flat-lane switch by pressure → in-lane model demotion → effort-param reduction → local fallback (3.9) → park with resume time — moves only with **disclosure as state**: the task card shows the active mode/degradation like a Battery-Saver indicator, and the receipt carries a mode-change line. The Opus→Sonnet silent-switch complaint record [17] is the documented failure mode this prevents.

**4.6 Price table & receipts (3.1, 3.6).** Ship models.dev-seeded defaults (Anthropic + metered `zai` rows — **never the zero-cost `zai-coding-plan` rows**, which would render every receipt $0 [68]); rows carry `{model, lane, unit_prices…, effective_from, verified_on, source}` with future-dated rows first-class (Sonnet 5's 2026-09-01 flip ships in the default table). User-editable per 13.4; refresh via the report-02 §6 cadence plus a monthly models.dev diff. Receipts: consumed units per model × purpose (ceremony/execution/verification itemized), currency = API-equivalent for flat-rate (labeled), real dollars for metered (currency visibly flips, P-T17-2), mode/degradation lines, park history ("parked 2 h 14 m, resumed on provider signal"), and the **done-directly figure** — recommended formula: the final accepted attempt's execution-stage usage priced at list, as "direct-use estimate", labeled a heuristic until 11.2 benchmark pairs replace it with measured comparisons (pre-register before receipts ship; OQ3).

**4.7 Scheduler (2.2, 2.8, 3.3).** One SQLite-backed queue in the T07 store: CAS claiming, per-lane/per-model slot gates (observed concurrency, not documented promises), priority ladder `interactive > answered-parks/resumes-due > scheduled-due > background > probes`, with aging so background never starves. Cache-window-aware resume ordering: an in-TTL resume outranks same-priority cold work (6–16× economics [S1]). Schedules carry `{cadence, missed_slot_policy: run-once-late(default)|skip|notify-only, off_hours_window?, owner}`; catch-up scan at startup/wake applies the policy Quartz-style (run-once-late = fire-once-now + coalesce) [79]; off-hours batching = admission window per user config. systemd's role: start Sinet, and Def.6 transient scopes with `RuntimeMaxSec` as the PID-1 ceiling backstop.

**4.8 Ceilings & anomaly (3.7).** Per-run time/step/cost ceilings enforced by the wrapper at call boundaries (the only granularity that exists); engine flags at ≥2× the wrapper remainder [S2]; systemd time ceilings outermost. Loop detection (repeated-action signatures + no-artifact-progress) and silence detection (heartbeat timeout on stream events, WatchdogSec-shaped) are **scheduling actions**: contain (stop admitting the next call) → park at checkpoint → surface a card — pause-and-flag per D1.3, never auto-kill; the Gemini-CLI and OpenHands false-positive records [41, 40] are the evidence base for conservative thresholds and for always offering "resume, I was wrong" on the card.

**4.9 Local arbitration (3.11).** systemd slices: operator session (uresourced-style protection — active-session memory guarantee + CPU boost) > Sinet control plane > local inference > sandbox batch (`CPUWeight=idle`, `MemoryHigh` fenced). PSI triggers pause batch admission under sustained pressure. GPU: one inference server per VRAM pool owns the queue (Ollama's queue-until-unload or llama.cpp slots per T15); Sinet admission-checks against a VRAM ledger (measured model footprints + measured compositor headroom — both numbers are per-machine measurements, not documented constants [89, 90]); operator-wins = an eager-unload switch (manual + GameMode start/end script hook) at v0; automatic idle-detection glue is post-v0 novel work. Background intelligence (health watching, classification) reserves a small always-resident model slot so 3.9 "never fully offline" holds even under arbitration.

**What would change this decision:**
- **Anthropic un-pauses the credits change** → pre-registered P1 executes (interactive-only demotion; headless weight → Z.AI/local); the pressure router treats it as lane-config change + price-table currency flip — rehearsed 3.10 kill-switch (P-T01-3), no architecture change.
- **Providers ship real remaining-quota APIs on subscriptions** (e.g., `anthropic-ratelimit-unified-*` headers proving out, §2.9) → richer *observed* state for meters and park timing; D4 unchanged (still no prediction), C4 meters get better data.
- **OmniRoute-class OSS matures a D2/D4-compatible per-person pressure router** → re-run adopt-vs-build for the routing layer only; the ledger/scheduler stay Sinet-owned regardless (they are D7/D9-entangled).
- **models.dev goes stale or dies** → LiteLLM map (with its documented failure modes guarded) or manual table maintenance; the schema doesn't change.
- **Z.AI ends the cache-storage "Limited-time Free"** or changes multipliers → price-table + pressure-weight data change, watched by the §6 cadence; no design change.

---

## 5. What NOT to use and why

- **Quota-window modeling in any form** — OmniRoute's multi-window usage buckets (5 h/7 d) and teamclaude's window-percentage rotation reconstruct provider windows client-side; community "remaining quota" trackers (Dicklesworthstone's usage tracker, ccusage plan bars) estimate window state for display. As *control inputs* all of these are the D4 anti-pattern: provider windows are server-controlled, reset fleet-wide at will (Anthropic has manually reset all users' windows at least once), and are explicitly undocumented on some lanes. Sinet displays measured consumption vs its own budgets plus provider-signaled facts — nothing else. *(Study their signal extraction; never their window math.)*
- **teamclaude-shaped multi-account rotation** — its entire purpose is pooling across accounts; D2 bans it. Mechanism-study only.
- **LiteLLM proxy as the budget/metering layer** — enforcement-gap history (#27381/#11083/#20532, all fixed but the class is demonstrated), budget-exceeded surfacing as HTTP 401, silent $0.00 for unknown models, silent stale-fallback incident; plus the Anthropic lane can't ride a proxy at all and D2 would need one proxy per person [44, 48, 49]. Its budget-hierarchy and projection-alert *concepts* are worth keeping.
- **Engine-native retry/limit handling as the policy layer** — Claude Code hard-stops on window exhaustion (no resume); opencode retries 429s unboundedly at ~1 s with stacking SDK retries [20, 22]. Engines get minimal retry config; the wrapper classifies and schedules.
- **Notification-only budgets (the current OpenAI shape)** — "requests continue to be processed without interruption" after the threshold [51, 52]. 3.3's budgets must block admission and park runs, not email.
- **Dollar-based routing between flat-rate lanes** — D5 violation; also concretely present in tools (OmniRoute "Cost Optimized" strategy) — marginal dollars are zero on both v0 lanes, so any dollar-ranked choice between them is noise.
- **Aggregator price feeds as table ground truth** — pricepertoken-class sites quote cheapest-third-party-host prices for open-weight models (GLM-5 $0.60 vs first-party $1.00) [74]; the D5 table quotes the provider actually serving the lane, from primary pages, or models.dev rows reconciled against them.
- **Engine cost figures as billing truth** — documented client-side estimates with a live 10× bug record [4, 17]; cross-check tier only.
- **Auto-kill anomaly responses** — Gemini CLI's loop detector "renders [the CLI] unusable and interrupts legitimate repetitive tasks" per its own issue tracker; OpenHands' detector killed agents waiting on long builds [41, 40]. D1.3 pause-and-flag stands.
- **MPS/MIG-based GPU partitioning on this hardware** — MIG is datacenter-only; MPS limits are opt-in allocation caps, not isolation, and add operational complexity for zero gain over inference-server serialization on 12/24 GB pools [86].
- **cron/systemd timers as the task-schedule authority** — no per-schedule missed-slot policy, no billing/attribution integration, `WakeSystem` unavailable to user units; fine for starting Sinet itself, wrong for 2.2/2.8 schedules [76].
- **Zapier-shaped pause semantics** — pending deferred work dropped when the switch is off [61]. Pause must preserve queued+parked work; this is a tested invariant (P-T08-4).

---

## 6. Harvest-map verdicts

| Item | Verdict | Detail |
|---|---|---|
| **N3** Quota-storm handling (shed-vs-real distinction; fallback retry; `blocked_quota` + backoff; once-per-storm logging; interrupted judge runs never count) | **CONFIRM the lesson, REVISE the mechanics** | The core distinction is now multi-provider-verified wire fact: Z.AI 1302/1305 (transient) vs 1113/1308/1310/1316–1321 (depletion, some with in-band reset times) [6]; Anthropic's own changelog splits subscriber 429s into the same two flavors [5]. Nexus's Z.AI-specific lesson generalizes to the five-class taxonomy (§2.2), which adds two classes Nexus lacked: **auth/policy** (P-T17-1 — never retry-park) and **engine-ceiling** (S2). Mechanics revised: retry-once-on-fallback-model becomes the effort-mode downgrade ladder (disclosed, §4.5); backoff gains full jitter + retry budgets + circuit breaker [29, 30, 31]; park resume times now come from richer signals (`resetsAt`, `next_flush_time`, `retry.next`) than Nexus had. Once-per-storm logging and interrupted-rounds-don't-count carry forward unchanged. |
| **N8** Frontier ledger / honest accounting (dollars from `claude -p` envelope; blended-rate flat pricing; API-equivalent as comparison-never-bill) | **CONFIRM the posture, REVISE the mechanism** | API-equivalent-as-comparison-never-bill is now the field's own posture — Anthropic labels its figures "client-side estimates… do not bill from these fields" [4], and Claude Code's `/usage` carries the same disclaimer [7]. Mechanism revised: envelope dollars demote to cross-check (documented 10×-class value bugs [17]); blended-rate flat pricing is superseded by per-call usage × the effective-dated price table (§4.6), which prices cache tiers/thinking/server-tools correctly where a blended rate cannot. Added beyond Nexus: per-purpose itemization (ceremony/execution/verification — `/usage`'s capability-attribution pattern [7]) and the done-directly counterfactual, which no shipped tool has (§2.6). |
| **C4** Quota-window surfacing (fleet meters showing live window state per account/model) | **CONFIRM as observed-state PATTERN — prediction stays banned** | Consistent with report 01's C4 verdict, now with the full signal inventory: meters show measured consumption vs the person's own budgets, plus provider-signaled facts only — `rate_limit_event` fields, ratelimit headers where they exist, Z.AI reset times, "parked until {provider-signaled time}". Community window-*estimating* trackers (§5) are the anti-pattern boundary. The teamclaude `anthropic-ratelimit-unified-*` lead (§2.9, single-source) would enrich this surface if a probe confirms it. |
| **N16** Routing telemetry + collapse watch (keyword routing; rationale stored per task; `routing_outcomes`; threshold-tweak proposals as admin cards) | **CONFIRM the habit, REVISE the router** | Rationale-storage is ratified spec behavior (7.7/S2.6) and this report adds the field evidence for *why*: the Opus→Sonnet silent-fallback complaint record [17] shows undisclosed routing decisions destroy trust even when technically sane. Keep: per-decision plain-language rationale, `routing_outcomes` telemetry, periodic no-LLM threshold-tweak proposals as admin cards (they slot into the ⚙-settings + receipts regime from the G1 operator rider). Superseded: the deterministic keyword router itself — routing inputs are now the pressure gauge (§4.2), effort mode, and the T05/T06 task classification, not keywords. |

---

## 7. Open questions

**Operator decisions needed:**

1. **Pressure thresholds & headroom fractions.** Background-admission stop at ⚙0.7 pressure and background-budget defaults at ⚙≤50 % of advertised window capacity are proposed to align with the ratified stage-budget numbers — ratify or adjust (per-person 13.4 settings either way). *Owner: operator at G2.*
2. **Budget denominator seeding.** Seed automation-budget defaults from plan marketing shape (Max ~1,600 prompts/5 h, labeled "assumed") vs starting minimal and raising from a calibration week of measured consumption. Recommendation: seed-from-plan-shape at the conservative fraction — it fires sooner and the label is honest. *Owner: operator.*
3. **Done-directly formula ratification (3.6).** Pre-register the counterfactual: final-accepted-attempt execution usage at list price ("direct-use estimate", heuristic label) until 11.2 benchmark pairs supply measured comparisons — or ship without the figure until 11.2 exists. The figure is the platform's honesty keystone; its formula must not float. *Owner: operator; T11 consumes.*
4. **Z.AI prompt-unit calibration.** Requests-as-prompt-proxy needs one live-key calibration run against the subscription dashboard (does 1 request ≈ 1 prompt? how do multipliers post?). Extends the S3 blocked-items battery (≤$0.50 class). *Owner: implementation-phase spike (operator provisions the key — S3 steps already written).*
5. **GPU operator-wins scope at v0.** Manual eager-unload switch + GameMode hook (recommended v0) vs also building idle-detection auto-pause (novel glue, no prior art). *Owner: operator; T15 bridge.*

**Later research / spikes:**

6. **`anthropic-ratelimit-unified-*` header probe** — single-source lead (teamclaude README); if subscription responses carry machine-readable window headers, C4 meters and Class-2 parks get richer observed state. One cheap probe alongside the next battery. *Owner: implementation-phase probe.*
7. **systemd `Persistent=` sanity test on the target laptop** — the catch-up-across-suspend regression is fixed upstream (2022) but a one-hour test against this Ubuntu build is cheap insurance for the platform-restart path (task schedules themselves are Sinet-owned, so exposure is limited to Sinet's own startup timer). *Owner: implementation phase.*
8. **VRAM ledger calibration** — measure per-model footprints and compositor headroom on the actual 12 GB + 24 GB pools; community numbers vary too widely to ship [89, 90]. *Owner: T15.*

**New platform problems discovered (for the spec's Known-problems list):**

- **P-T08-1 — Engine usage/cost values drift and break, not just schemas.** 3–8× usage duplication, 10× cost inflation, missing result envelopes — all on the primary lane within 12 months [17]. → Ledger sanity bounds + tier-1-vs-tier-2 divergence alarms (>20 %); conformance suites assert *values* on known fixtures, not just field presence. *(Extends P-T01-2.)*
- **P-T08-2 — Budget, policy, and auth events masquerade as each other on the wire.** LiteLLM budgets → HTTP 401; Anthropic policy bans → valid-credential auth-shaped errors; Z.AI depletion code 1113 also fires on endpoint misconfig [44, 19, 24]. → The five-class classifier is a tested component with per-lane fixtures; misclassification of Class 4 as Class 2/3 (retry-parking a revoked lane) is the named worst case (P46-adjacent: a policy event that dies in a retry loop is a platform defect).
- **P-T08-3 — Prices carry effective dates, not just values.** Sonnet 5's pre-announced 2026-09-01 flip means a table without effective-date rows is *guaranteed* wrong on a known future date; Z.AI's cache-storage "Limited-time Free" is the same shape unbounded [8, 23]. → Price-table schema gets `effective_from` + scheduled-change support; the §6 watcher diffs announced future prices, not just current ones.
- **P-T08-4 — Pause semantics can destroy deferred work.** The Zapier drop-on-expiry shape [61]. → "Pause preserves queued + parked work" is a tested invariant of the one-switch pause and maintenance mode.
- **P-T08-5 — The pressure gauge inherits the cache-weighting assumption.** The ⚙0.1× cache-read weight is "assumed" (G1 Def.10); if subscription quota accounting weights cache reads differently, pressure systematically misleads routing on the heaviest-cached workloads. → Keep the label; add a quarterly calibration check (measured window exhaustion vs gauge prediction at exhaustion events — using *observed depletion events*, not window modeling); alarm on systematic divergence. *(Joins P-T02-2's cache-fidelity alarm in the same suite.)*

---

## 8. Sources

Internal (settled inputs, cited as such): `Research/decisions/GATE-1-architecture-direction.md` · `Research/spikes/G1-S1-cli-wrap-vs-sdk.md` [S1] · `Research/spikes/G1-S2-defer-drill.md` [S2] · `Research/spikes/G1-S3-opencode-park-xdg.md` [S3] · `Research/01-execution-engines-and-adapters.md` · `Research/02-provider-watchlist-and-onboarding-criteria.md` · `Docs/component-harvest-map-proposal-v1.md` · `Docs/agent-platform-feature-list-v1.md`.

Web sources — all accessed 2026-07-17. Tier P = primary (provider/project's own page or repo), S = secondary, C = community.

**Anthropic — API, subscription, Claude Code**
1. P — https://platform.claude.com/docs/en/api/rate-limits — 429/`retry-after`, `anthropic-ratelimit-*` header set (RFC 3339 resets), token bucket, acceleration limits, spend caps, cache-aware ITPM.
2. P — https://platform.claude.com/docs/en/api/errors — 401/402/403/429/529 error-type mapping; error body shape.
3. P — https://platform.claude.com/docs/en/api/messages — usage object fields incl. `output_tokens_details.thinking_tokens`, `server_tool_use`, `service_tier`, `inference_geo`.
4. P — https://code.claude.com/docs/en/agent-sdk/cost-tracking — `total_cost_usd`/`costUSD` = client-side estimates; `usage` top-level-only vs `modelUsage` incl. subagents; dedup-by-message-id.
5. P — https://raw.githubusercontent.com/anthropics/claude-code/main/CHANGELOG.md — 2.1.199 subscriber transient-429 auto-retry; 2.1.208 truncated `-p` output fix; `/usage` fixes.
6. P — https://docs.z.ai/api-reference/api-code — full Z.AI error-code table (1113/1302/1305/1308/1310/1316–1321; 1000-class auth); streaming errors via `finish_reason`.
7. P — https://code.claude.com/docs/en/costs — `/usage` session receipt, per-capability attribution percentages, estimate disclaimer.
8. P — https://platform.claude.com/docs/en/about-claude/pricing — current per-model prices; cache 1.25×/2×/0.1×; batch −50 %; stacking multipliers; Sonnet 5 introductory pricing through 2026-08-31.
9. P — https://github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md (+ server-schema.cpp) — timings/usage fields, `cached_tokens`, `include_usage`, slots & deferred queue, `--parallel -1`/unified KV default.
10. P — https://raw.githubusercontent.com/ollama/ollama/main/docs/api.md — `prompt_eval_count`/`eval_count`, nanosecond durations.
11. P — https://docs.vllm.ai/en/latest/serving/openai_compatible_server/ + /en/latest/configuration/optimization.html + /en/stable/design/metrics/ — stream usage, preemption (V1 RECOMPUTE), `vllm:num_requests_waiting`.
12. P — https://support.claude.com/en/articles/14552983 + https://code.claude.com/docs/en/model-config — Opus→Sonnet usage-threshold fallback and its notice string.
13. P/C — https://github.com/ryoppippi/ccusage/issues/4 — v1.0.9 removed `costUSD` from session JSONL (drift event).
14. P — https://platform.claude.com/docs/en/manage-claude/workspaces — workspace spend limits ≤ org limits.
15. P — https://platform.claude.com/docs/en/manage-claude/spend-limits-api — Enterprise-only, per-seat; not Console workspaces.
16. C — https://github.com/anthropics/claude-quickstarts/issues/371 — no Admin API for workspace spend/rate limits.
17. C — https://github.com/anthropics/claude-code/issues/6805 (usage duplication 3–8×), /issues/53371 (`total_cost_usd` ~10×), /issues/3434 (silent Opus→Sonnet fallback, no opt-out), /issues/35744 (open: auto-continue after limit reset), /issues/36320 (closed dup; documents headless hard-stop).
18. P — https://support.claude.com/en/articles/11647753 — one usage pool across surfaces; usage-limit presentation.
19. S — https://www.theregister.com/2026/02/20/anthropic_clarifies_ban_third_party_claude_access/ — OAuth-ban ToS clarification timeline.
20. S — https://www.mindstudio.ai/blog/anthropic-openclaw-ban-oauth-authentication — verbatim policy-ban error text on valid credentials (corroborated multi-outlet).
21. S — https://x.com/AnthropicAI/status/1949898502688903593 + https://techcrunch.com/2025/07/28/anthropic-unveils-new-rate-limits-to-curb-claude-code-power-users/ — weekly-limit introduction (>12 mo old; flagged; limits since evolved).

**Z.AI plan & community wire evidence**
22. P — https://docs.z.ai/devpack/faq — prompts/5 h per tier, depletion behavior, peak/off-peak multipliers, weekly cap.
23. P — https://docs.z.ai/guides/overview/pricing — GLM per-MTok prices incl. cached-input; cache storage "Limited-time Free".
24. C — https://github.com/anomalyco/opencode/issues/8618 (undocumented ~1-concurrency on GLM Pro; single-source), /issues/14535 (1302 fast-recovering transient); https://github.com/letta-ai/letta-code/issues/1394 (1113 on endpoint misconfig with valid plan key; single-source).

**Local inference** — see 9, 10, 11 above; 25. P — https://docs.ollama.com/faq — keep_alive defaults/semantics, MAX_LOADED_MODELS/NUM_PARALLEL/MAX_QUEUE, queue-until-VRAM-frees (verified verbatim).

**Retry, parking, ceilings, anomaly**
26. P — https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Retry-After — both value forms; 429/503 applicability (RFC 9110 fetch truncated; MDN + secondaries consistent).
27. P — https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/ — full-jitter formula; jittered backoff as standard (full-vs-decorrelated verdict "less clear" — nuance noted).
28. P — https://sre.google/sre-book/handling-overload/ — criticality tiers; quota×criticality shed-lowest-first; retry budgets (3 per request, ≤10 % per client); adaptive throttling (verified verbatim).
29. P/S — https://martinfowler.com/bliki/CircuitBreaker.html — breaker states (corroborated by Azure Architecture Center; search-verified tier).
30. P — https://docs.temporal.io/develop/typescript/workflows/timers (+ /encyclopedia/retry-policies, search-verified) — durable timers persist across process death; timer-vs-signal parking.
31. P — https://github.com/sidekiq/sidekiq/wiki/Error-Handling + https://docs.bullmq.io/guide/retrying-failing-jobs — delayed-retry formulas + jitter options (search-verified tier).
32. C — https://github.com/anomalyco/opencode/issues/30510 (+ /8203, /16994) — unbounded ~1 s 429 retries, stacked SDK retries, hang/terminate failure modes.
33. C — https://github.com/Aider-AI/aider/issues/861 — inadequate backoff on Anthropic 429 (single-source).
34. P — https://github.github.com/gh-aw/reference/rate-limiting-controls/ — 20-min agent timeout, stop-after, daily AI-credit spawn-time guardrail (verified; sibling-doc discrepancy #21663 noted).
35. P — https://swe-agent.com/latest/reference/model_config/ — per-instance cost limits; cost as primary knob; per-call check granularity (granularity inference flagged).
36. P — https://docs.langchain.com/oss/python/langgraph/errors/GRAPH_RECURSION_LIMIT — recursion_limit 25 default (search-verified tier).
37. P — https://docs.openhands.dev/sdk/guides/agent-stuck-detector + https://raw.githubusercontent.com/OpenHands/software-agent-sdk/main/openhands-sdk/openhands/sdk/conversation/types.py — stuck-pattern taxonomy, defaults 4/3/3/6, on-by-default (verified in source); config-doc default discrepancy (100 vs 500 iterations) flagged.
38. C — https://github.com/All-Hands-AI/OpenHands/issues/5355 — stuck detector kills long-running-process waits.
39. P — https://raw.githubusercontent.com/google-gemini/gemini-cli/main/packages/core/src/services/loopDetectionService.ts + PR #8231 — thresholds (5/10/30-turn LLM check); per-session disable added under false-positive pressure.
40. C — https://github.com/google-gemini/gemini-cli/issues/8237, /5761, /11002, /14887 — loop-detection false-positive record (maintainer-acknowledged; #14887 possibly a true positive, noted).
41. P — https://manpages.debian.org/bookworm/systemd/systemd.service.5.en.html (+ sd_notify(3)) — RuntimeMaxSec, WatchdogSec heartbeat semantics (freedesktop.org 403; mirror text identical).
42. P — https://code.claude.com/docs/en/cli-reference — `--max-turns`/`--max-budget-usd` semantics, print-mode-only.

**Budgets, admission, effort modes, pause**
43. P — https://docs.litellm.ai/docs/proxy/users — budget hierarchy, aligned resets, ExceededTokenBudget-as-401, `fail_closed_budget_enforcement`.
44. P — https://docs.litellm.ai/docs/proxy/dynamic_rate_limit — priority_reservation + saturation_threshold (guaranteed shares, borrow-under-low-saturation).
45. P — https://docs.litellm.ai/docs/proxy/alerting (+ /docs/scheduler) — budget_crossed/threshold_crossed/projected_limit_exceeded events; beta priority queue.
46. C — https://github.com/BerriAI/litellm/issues/27381, /11083, /20532, /12905, /31292, /26672, /20324 — budget-enforcement gap family (all closed; cited as documented history).
47. P — https://docs.litellm.ai/blog/model-cost-map-incident (+ issues /22609, /22646; /docs/troubleshoot/cost_discrepancy) — 2026-01-27 silent stale-fallback incident; unknown models → $0.00.
48. P — https://developers.openai.com/api/docs/guides/production-best-practices — notification thresholds only; no hard-cap mechanism in current docs.
49. S/C — https://grafient.ai/blog/openai-removed-hard-budget-limits + https://community.openai.com/t/1193635 + /t/1300634 + https://news.ycombinator.com/item?id=45589628 — hard-limit removal, "soft spending thresholds… without interruption" (help-center text quoted in OpenAI's own forum; primary page 403 to fetcher).
50. P — https://openrouter.ai/docs/api_reference/limits + /docs/features/provisioning-api-keys — per-key `limit`/`limit_remaining`/`limit_reset`, 402 on exhaustion (pre-provider rejection inferred, flagged).
51. P — https://kubernetes.io/docs/concepts/scheduling-eviction/pod-priority-preemption/ — admission quota + priority preemption analogy.
52. P — https://source.android.com/docs/core/data/data-saver — background-restriction-with-disclosure UX pattern.
53. P — https://developers.openai.com/api/docs/guides/reasoning — reasoning.effort enum.
54. P — https://platform.claude.com/docs/en/build-with-claude/effort — effort low/medium/high/max; `budget_tokens` deprecation; "controls overall token spend".
55. P — https://ai.google.dev/gemini-api/docs/thinking — thinkingBudget/thinkingLevel.
56. P — https://openai.com/index/introducing-gpt-5/ + https://help.openai.com/en/articles/11909943 — router; mini fallback at usage limits ("won't appear in the model picker").
57. P — https://www.home-assistant.io/docs/automation/services/ — `stop_actions` default true (halt-in-flight with finish option).
58. P — https://docs.github.com/en/actions/how-tos/manage-workflow-runs/disable-and-enable-workflows — disable = future triggers only.
59. P — https://help.zapier.com/hc/en-us/articles/8496061204621 — delayed tasks expiring while off are dropped (wording verified).

**Pressure-routing prior art (novelty check)**
60. C — https://raw.githubusercontent.com/diegosouzapw/OmniRoute/main/README.md + /docs/routing/QUOTA_SHARE.md (+ USER_GUIDE.md, which omits the feature) — headroom/reset-window strategies; DRR over 5 h/7 d buckets; soft-deprioritize 0.7; hot-path pre-send enforcement (v3.8.40, 2026-06-28).
61. C — https://github.com/KarpelesLab/teamclaude — proactive multi-account rotation at 98 % of 5 h/7 d window via `anthropic-ratelimit-unified-*` headers (single source for those headers; D2-incompatible purpose).
62. C — https://knightli.com/en/2026/05/08/9router-ai-coding-router-token-saver/ — reactive tiered failover (contrast case).
63. C — https://github.com/tashfeenahmed/freellmapi — proactive under-cap routing across free tiers (nearest per-request analogue).
64. C — https://github.com/Dicklesworthstone/coding_agent_usage_tracker — window-state display tool (anti-pattern boundary for D4).

**Price tables & receipts**
65. P — https://models.dev/api.json + https://github.com/anomalyco/models.dev (+ README) — MIT (GitHub API-verified); cost field schema USD/MTok; CI-validated PRs; `zai` real prices vs `zai-coding-plan` all-zero rows (fetched and inspected).
66. P — https://openrouter.ai/docs/guides/overview/models — /api/v1/models pricing object (string per-token values; marketplace prices).
67. P — https://artificialanalysis.ai/data-api/docs — 1,000 req/day free tier; attribution required.
68. P — https://langfuse.com/docs/observability/features/token-and-cost-tracking (+ github.com/orgs/langfuse/discussions/9696) — model definitions, usage-type pricing, conditional tiers, inclusive-vs-exclusive token normalization; defaults hand-maintained.
69. S — https://github.com/Helicone/helicone/blob/main/packages/cost/README.md + https://www.helicone.ai/llm-cost — open cost registry (search-summary tier, flagged).
70. P — https://ccusage.com/guide/cost-modes — auto/calculate/display modes; LiteLLM-backed prices; TTL-blind cache-write fallback; no plan-price comparison.
71. S — https://pricepertoken.com/pricing-page/model/z-ai-glm-5 + https://openrouter.ai/z-ai/glm-5 — aggregator-vs-first-party price divergence (the trap exhibit).
72. C — https://github.com/pydantic/genai-prices — additional maintained open price DB (unexamined; follow-up candidate).

**Scheduling & host arbitration**
73. P — https://manpages.debian.org/testing/systemd/systemd.timer.5.en.html — Persistent=, sleep catch-up + coalescing, WakeSystem privileges, RandomizedDelaySec (freedesktop 403; identical upstream text).
74. P/C — https://github.com/systemd/systemd/issues/24984 — suspend catch-up regression, closed-completed 2022-10-20.
75. S — https://www.cloudns.net/blog/cron-vs-anacron-a-comprehensive-guide/ (+ oneuptime anacron guide) — anacron run-once-late vs cron skip.
76. P/S — https://www.quartz-scheduler.org/api/2.3.0/org/quartz/CronTrigger.html + https://nurkiewicz.com/2012/04/quartz-scheduler-misfire-instructions.html — misfire instruction vocabulary + design guidance (verified against javadoc).
77. P — https://apscheduler.readthedocs.io/en/3.x/userguide.html — misfire_grace_time + coalesce.
78. P — https://github.com/rails/solid_queue/blob/main/README.md (+ github.com/maragudk/goqite, github.com/litements/litequeue, github.com/justplainstuff/plainjob) — SQLite queue patterns; SKIP LOCKED no-op on SQLite; throughput existence proofs (plainjob benchmark unverified, flagged).
79. P — https://man.archlinux.org/man/systemd.resource-control.5.en — CPUWeight (incl. "idle"), MemoryHigh-main/MemoryMax-last-resort, IOWeight, MemoryPressureWatch (verified verbatim).
80. P — https://fedoraproject.org/wiki/Changes/Reserve_resources_for_active_user_WS — uresourced: active-user memory guarantee + CPU boost ("operator always wins" prior art; verified).
81. P — https://docs.kernel.org/accounting/psi.html (+ facebookmicrosites PSI overview) — some/full pressure metrics, userspace triggers.
82. P — https://docs.nvidia.com/datacenter/tesla/mig-user-guide/supported-gpus.html + https://docs.nvidia.com/deploy/mps/ — MIG datacenter-only; MPS mem limits enforced-at-allocation, opt-in, not isolation (verified with correction).
83. S — https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/gpu-sharing.html — time-slicing has no memory isolation.
84. P — https://github.com/FeralInteractive/gamemode — start/end custom-script hooks (operator-wins glue point).
85. C — https://www.dedoimedo.com/computers/wayland-fedora-gnome-kde-neon-amd-graphics-benchmark.html + https://forums.developer.nvidia.com/t/307939 + https://www.runaihome.com/blog/ollama-model-keeps-reloading-vram-fix-2026/ — compositor VRAM behavior + model-load latencies (community tier; measure-on-target flagged).
86. C — https://third-bit.com/2026/06/05/priority-starvation/ — aging-burst oscillation note.
87. S — https://itnext.io/building-a-carbon-and-price-aware-kubernetes-scheduler-f305cd3df0f1 — window-deferred batch pattern (off-hours analogue).

*(Source count: 87 numbered web entries — several bundling issue families — plus 8 internal settled inputs. Single-source claims are flagged inline where they occur: GLM Pro concurrency [24], `anthropic-ratelimit-unified-*` headers [61], plainjob throughput [78], Helicone details [69], compositor headroom & load latencies [85].)*
