## S10 — Metering, budgets, scheduling & limit events

**Scope:** The consumption layer beneath every run — the exact usage ledger (one row per paid call at the D7 checkpoint), the two-currency model (consumption pressure vs the effective-dated price table), per-person automation budgets and interactive headroom, the five-class limit-event taxonomy that drives retry/park/freeze, effort modes as disclosed depletion ladders, the one SQLite run scheduler (priorities, cache-window-aware resumes, missed-slot policy), per-run hard ceilings, local-resource arbitration policy, honest receipts including the done-directly figure, and never-fully-offline behavior — specifying **how** the platform meters exactly, reacts to (never models) provider limits, and schedules everything so automation never starves a person's interactive use.

**Binding inputs:** R09 §1–7 (primary) [G2 D2.1]; **D4, D5, D7**; feature-list 3.1–3.11, 2.8, S2.5, 3.4; G1 P1/P7, D1.7, Def.10, riders 1 & 3; G2 D2.1, D2.4, D2.8, Def.4/5/6/16; G3 D3.1; `Spec/benchmark-preregistration-v1.md` §13 (done-directly formula — cited, numbers never restated); [SPIKE P2-S1] Z.AI calibration + logprob; [SPIKE P2-S3] `anthropic-ratelimit-*` inventory (R09-OQ6); [SPIKE P2-S4] serialize-by-deny cost; [SPIKE G1-S1/S2/S3] usage/park/ceiling wire facts; P-T08-1..5; P-T17-1, P-T01-3, P-T02-2 (shared). Boundary siblings: [XREF:S01] units/slices/timers · [XREF:S02] checkpoint/FSM/queue storage · [XREF:S03] wire mechanics & engine knobs · [XREF:S06] intake cost-gate · [XREF:S08] worker/routing ceiling defaults · [XREF:S11] credential-injection proxy · [XREF:S12] GPU/local internals · [XREF:S14] watchdog alarms & benchmark machinery · [XREF:S15] meter/receipt surfaces · [XREF:S19] v1 schedule/trigger boundary.

The vocabulary of this section: **usage row** = the single measured record written for one paid model call, carrying usage, pricing, purpose, and approximation tier; **approximation tier** = the 1–5 honesty rank of how a row's consumption was obtained (measured → unknown), stamped on every row and shown on receipts; **consumption-pressure gauge** = the per-(person, lane) instrument reading weighted consumption against the operator-declared automation budget (the D5 flat-rate currency, extending the glossary's *consumption pressure*); **limit-event class** = one of the five wire-observable failure kinds (§S10.5) that selects a retry/park/freeze action; **workload class** = a run's scheduling priority band (interactive / human-blocked resume / scheduled / background / probe). These five terms are coined here.

### S10.1 The consumption ledger (3.1, 3.4, D4)

The platform meters its **own** consumption exactly and never trusts a provider to report remaining quota (D4). One **usage row** is written **per paid model call, in the same transaction as its D7 checkpoint** [R09 §4.1; D7; S02.4a] — the checkpoint's usage block *is* the ledger row; the `receipts`/usage table family materializes them per run-end [S02.2]. Storage is S02's; the row's content, pricing, and approximation semantics are specified here.

**Usage row (indicative fields; exact DDL is the P3 schema workshop's, jointly with S02):**

| Group | Fields |
|---|---|
| identity | `run_id, task_id, requester (D2 owner), owner_lane, substrate, model, ts` |
| usage | `input, output, cache_read, cache_creation (+ TTL buckets), thinking_tokens, server_tool_use{count}` — stored pass-through [R09 §2.1; SPIKE G1-S1 F1] |
| pricing | `engine_cost_estimate` (cross-check only), `priced_cost{table_version, effective_date}` (§S10.3), `currency ∈ {api-equivalent, real}` |
| classification | `purpose_tag ∈ {ceremony, execution, verification, probe}`, `approximation_tier ∈ {1..5}` |

**Metering rules (normative):**
- **Accounting math is encoded, not assumed.** On the Anthropic surface `input_tokens` counts only tokens after the last cache breakpoint — total prompt = `cache_read + cache_creation + input_tokens` [R09 §2.1]; thinking tokens are billed as output; server tools (web search/fetch) are priced per use, outside token math [R09 §2.6]; the batch flag halves token prices. A row that fails to normalize these double- or under-counts (P-T08-1).
- **Dedup parallel-tool-call messages by message `id`** — one message id can emit multiple assistant messages with identical usage [R09 §2.1]. Engine-native subagents are disabled (sole-controller rider [G1 rider 2; S03.5]), which removes the nested-`usage`-undercount hazard, but the `modelUsage`/`total_cost_usd` cross-check and the dedup rule still apply.
- **Engine dollars are cross-checks, never billing truth.** `total_cost_usd`/`costUSD` are vendor-labeled client-side estimates with a live ~10× bug record [R09 §2.1/§2.6]; they are stored and cross-checked, never authoritative. Sinet prices from its own table (§S10.3, D5).
- **No row silently prices to $0.** An unpriceable row renders **UNPRICED** on the receipt and raises a drift alert; the LiteLLM silent-$0 incident is the cautionary record [R09 §2.1]. This is the load-bearing guard against the no-load-bearing-metered-paths rule at the metering layer.

**Aggregations** per person × model × task × day/week/month are **queries over the ledger, not new state** [R09 §4.1]. Every run **bills its requester** (3.4), ceremony itemized separately from execution (1.10); the `/usage` per-capability-percentage breakdown is the presentation model [R09 §2.6; XREF:S15]. Trigger-/webhook-registered tasks bill the **registrant** — a v1 surface, since triggers are v1 [3.4; XREF:S19].

**The honest approximation hierarchy** (best-first; every row labeled) [R09 §2.1]:

1. **Measured per-call usage** from the substrate stream/DB — both v0 subscription lanes and local, the normal case; exact on every surface, confirmed live incl. per-message cache detail [SPIKE P2-S1 §Impl-metering-1].
2. **Engine-computed aggregates** — cross-check tier; alarm when tier-1 × price table diverges from the engine estimate past `⚙ meter.value_divergence_alarm` (catches both price staleness and engine bugs; machinery [XREF:S14]).
3. **Derived plan units** — the Z.AI GLM Coding plan is denominated in "prompts" (~15–20 model calls each) but exposes **no per-prompt counter, no usage endpoint, and no rate-limit headers on the wire** (all `…/usage` variants 404; response body carries only per-request token `usage`) [SPIKE P2-S1 Probe 7; S03.7]. Prompt-unit consumption therefore stays **tier-3 derived** (requests-as-prompt-proxy with documented peak 3× / off-peak 2× multipliers applied as data), reconcilable only against the operator dashboard by `request_id` — the calibration recipe is `TBD-OPERATOR(Z.AI dashboard prompt-unit calibration)`, the 5-step dashboard procedure [SPIKE P2-S1 §Blocked].
4. **Tokenizer estimates** — pre-run cost gates only (the 2.5 size guess and the $0.50 zero-interaction band [G1 D1.7; XREF:S06]), never a ledger fact.
5. **Unknown** — flagged UNPRICED (tier-5), never a silent zero (P-T08-1).

### S10.2 Two currencies (D5, 3.10)

Between flat-rate options, scheduling and routing use **consumption pressure**, never dollars (marginal cost is zero there); **dollars are the reporting currency** and a routing input only for explicitly enabled metered use [D5].

- **Flat/metered is declared per model by each user** (3.1) as a data flag on the price table (§S10.3); a flip is a rehearsed data change with visibly changing receipt currency, not an architecture change [P-T01-3; S03.3].
- **Metered spending is opt-in only** (3.10): per-token billing is enterable only by an explicit per-use flag — never a default, never a silent fallback [14.3]. The **metered-exception list is EMPTY at v0** [G1 P7]; DeepSeek is the pre-registered designated exception should one ever be enabled [S03.6]. A model whose `overflow_mode` is `auto-metered` without a proven disable/zero-balance is rejected at onboarding [S03.6; 3.10].
- **Local lane has no pressure and no budget** — it is arbitrated, not budgeted (§S10.9); its rows price at $0 with utilization noted [R09 §4.2].

Dollar-based routing *between* flat-rate lanes is a **D5 violation and NEVER done** — marginal dollars are zero on both v0 subscription lanes, so any dollar-ranked choice between them is noise [R09 §5].

### S10.3 The price table (3.1, D5) — user-maintained, effective-dated

Dollars come only from an **effective-dated price table** [D5; R09 §4.6]. Shipped defaults are the **pinned genai-prices `data.json`, vendored** [G2 Def.16; D2.2 Layer B] — this **supersedes R09 §2.6's models.dev lean** (the gate examined the price-table area report 09 left as a follow-up and ratified genai-prices); models.dev and the LiteLLM map are retained only as CI cross-checks. Semantics:

- **Rows carry `{model, lane, unit_prices…, effective_from, verified_on, source}` with future-dated rows first-class** — a table without effective dates is *guaranteed* wrong on a known future date (e.g., a pre-announced list-price flip), and a promotional "limited-time free" cache rate is the same shape unbounded (P-T08-3). The provider-watch cadence diffs announced *future* prices, not only current ones [S03.6; XREF:S14].
- **Never price a flat-rate lane's usage from a $0 flat-rate row.** The vendored defaults must be reconciled at ship so Z.AI usage prices API-equivalent from the **metered per-token `zai` rows**, never the zero-rate coding-plan rows, which would render every receipt $0 [SPIKE P2-S1 §Impl-metering-3; R09 §4.6]. Never meter dollars from opencode (it prices the coding plan at $0).
- **Never quote aggregator/cheapest-host prices** — the D5 table quotes the provider actually serving the lane, from primary pages or reconciled genai-prices rows [R09 §5].
- **User-editable per 13.4.** Refresh lands as a **proposal, never an overwrite**; the user overlay is never clobbered [G2 Def.16]. **Price-table drift is a first-class staleness cause** — it re-prices the remaining plan, so any change fires the freshness re-validation trigger [G1 Def.5; S02.6].

### S10.4 Consumption pressure, budgets & interactive headroom (3.3, D5)

The **consumption-pressure gauge** per (person, lane) = weighted consumption ÷ the **operator-declared automation budget** for a Sinet-defined period, overlaid with provider-signaled observed events [R09 §4.2, §3-D2]. This is D4-clean: the denominator is Sinet's own budget (a 3.3 construct the operator edits, a 13.4 setting), **never an inferred provider window**; reset times come only from provider signals (§S10.5).

- **Weighting.** Anthropic lane: tokens with cache-read weighted `⚙ pressure.cache_read_weight = 0.1×`, labeled **"assumed"** until subscription quota semantics are published [G1 Def.10]. This is the single most impactful gauge assumption on cache-heavy work — live sampling saw cache reads dominate (≈8320/8668 tokens on one Z.AI turn) [SPIKE P2-S1 §Impl-metering-1] — and the gauge inherits it (P-T08-5). Z.AI lane: requests-as-prompt-proxy with the documented multipliers applied as data (tier-3). Every gauge reading carries its approximation tier.
- **Budget denominators seed from plan marketing shape** at a conservative fraction — e.g., the GLM Coding Max ~1,600 prompts/5 h shape [G1 D1.5] — labeled **"assumed"**, which fires sooner than a calibration week and stays honest [G2 Def.5]. Background budgets default to `⚙ budget.background_window_fraction = 0.5` (≤50% of the advertised window, "assumed") [G2 Def.4/5].
- **Headroom rule (SRE-shaped).** Interactive use is CRITICAL_PLUS — **the platform's own consumption NEVER blocks a person's interactive use** (3.3); background classes shed lowest-first as pressure rises, and **background admission stops at `⚙ pressure.bg_admit_stop = 0.7`**, leaving the remainder as interactive headroom (aligned with the ratified 70% stage-overflow event) [G2 Def.4; R09 §4.4].
- **Observed-state overlay (D4-clean, measured).** On the Anthropic subscription wire the model-egress request returns the **`anthropic-ratelimit-unified-*` family (16 headers: 5h / 7d / 7d_oi × reset/utilization/status, plus overage/fallback fields)** — the classic per-request/token buckets are **absent** on the subscription surface [SPIKE P2-S3 §4; R09-OQ6 confirmed]. These are provider-*signaled* facts (the provider telling us, not us modeling), harvested at the **credential-injection proxy** [XREF:S11] — one choke point, two purposes, mirroring the D7 "one write, two purposes" shape. They (a) supply Class-2 park resume times (`-reset`, §S10.5), (b) enrich the fleet/consumption meters as an observed overlay (`-utilization`, `-status`, `-overage-status`) [XREF:S15], and (c) feed the P-T08-5 quarterly cache-weight calibration. The **denominator stays the operator budget** regardless — utilization headers are overlay and cross-check, never the pressure denominator (D4). Z.AI exposes no such headers; its limit state rides error-code bodies (§S10.5) [SPIKE P2-S3 §4].
- **Enforcement is two-gated** [R09 §4.4; C2]: a **spawn gate** (estimated cost vs remaining budget + pressure band; the $0.50 zero-interaction band rides this estimate [G1 D1.7; XREF:S06]) and a **per-checkpoint gate** (a breach parks the run `blocked_budget` at the checkpoint — the finest granularity that exists anywhere — resuming when the period rolls or the owner raises the budget; both are inbox cards). Admission against a post-call-updated counter is *not* a hard ceiling — the per-checkpoint gate is what makes budgets bind (§S10.8). Notification-only budgets are NEVER used — a budget that only emails is not a budget [R09 §5].
- **One-switch pause-my-automation** stops the person's background admission and parks in-flight runs at their next checkpoint (halt-with-finish-current-stage option), and **MUST preserve everything queued and parked**. Destroying parked/queued work on pause is the named anti-behavior with a mandatory test (P-T08-4) [R09 §4.4/§5].

### S10.5 The five-class limit-event taxonomy (3.2, D4)

Limit events are **normal, recoverable scheduling events**, not errors (D4). The wire's signals sort into five **limit-event classes**; each selects a fixed action. **The classifier is a TESTED component** with per-lane fixtures — budget, policy, and auth events masquerade as one another on the wire (budget-as-HTTP-401, policy-ban on valid credentials, Z.AI 1113 on endpoint misconfig), and misclassifying an auth event as depletion (retry-parking a revoked lane) is the named worst case (P-T08-2) [R09 §2.2/§4.3; XREF:S14]. Adapters forward the raw signals as data [S03.1]; the taxonomy and scheduling live here.

| Class | Wire signals (per lane) | Action |
|---|---|---|
| **1 Transient shed** | Anthropic 529 / subscriber transient-429; Z.AI 1302/1305 | **Retry in place**: full jitter, per-request cap `⚙ limit.retry_cap = 3`, per-lane retry budget `⚙ limit.retry_budget_ratio = 0.10`, circuit breaker on repeat [R09 §4.3]. Never park; never count against quota. |
| **2 Depletion + signal** | `rate_limit_event.resetsAt`; `anthropic-ratelimit-unified-*-reset` [P2-S3]; Z.AI 1308/1310 `next_flush_time`; opencode `retry.next` | Park **`blocked_quota`** at checkpoint with the **provider-signaled** resume time; auto-resume; prefer resuming inside the 1 h cache window when pressure allows (§S10.7). |
| **3 Depletion − signal** | Z.AI 1113 (after endpoint self-check); undocumented concurrency caps | Park with a jittered probe schedule (interval cap `⚙ limit.probe_interval_max = 30 min`); probe resumes are zero-cost and never count as attempts or spend [SPIKE G1-S2]. |
| **4 Auth / policy** | 401/402/403; policy-ban text on *valid* credentials | **Lane freeze + operator alert** via the P-T17-1 auth canary [S03.6]; **NEVER retry-park** — a policy event that dies in a retry loop is a platform defect (P-T08-2). |
| **5 Engine ceiling** | `error_max_budget_usd` / `budget_exhausted` | Died-at-gate handling — Sinet's own backstop tripping; the wrapper ask-record is authoritative [S02.3]; engine flags sized ≥2× so this class stays rare (§S10.8). |

**Park economics and discipline.** Parking is cheap and lossless on both lanes — defer-park exits carry the full pending call and poll-resumes cost $0 [SPIKE G1-S2 F1]; opencode sessions survive restart in SQLite, with ask-loss contained by Sinet's authoritative ask-record [SPIKE P2-S1 Probe 2; S03.4]. The run **parks at its checkpoint losing nothing and resumes without anyone babysitting** (3.2/3.8) — from the provider signal where available, else on the probe schedule. **Once-per-storm logging**: one drift/park card per storm per lane, not per attempt; interrupted verification rounds never count as rework rounds [R09 §6 (N3)]. Engine-native retry is NEVER the policy layer — Claude Code hard-stops on window exhaustion and opencode retries unboundedly at ~1 s; engines get minimal retry config and the wrapper classifies [R09 §5; S03.3].

### S10.6 Effort modes as disclosed depletion ladders (3.5)

Effort modes are policy bundles (duty-map model + vendor effort param + verification depth + retry allowance + lane/park preference), each a **depletion ladder whose steps are disclosed states** [R09 §4.5; D5]:

- **Eco** — an adequate result at the lowest consumption: smallest capable model, effort *low*, mandatory-only verification, no retries, off-hours placement. The Z.AI **`thinking:{type:disabled}` lever (~50× token reduction on trivial work) is an Eco/Balanced rung** on that lane [SPIKE P2-S1 Probe 7; S03.7].
- **Balanced** — the best result per unit of consumption: duty-map default model, effort *medium*, cheap-first verification cascade, one retry.
- **Smart** — the best result within the person's automation budgets: frontier lane, effort *high/max*, full two-axis verification, retries within ceilings, and **parking on depletion rather than degrading below its quality floor**.

**The downgrade ladder** — flat-lane switch by pressure → in-lane model demotion → effort-param reduction → local fallback (3.9) → park with resume time — moves **only with disclosure as state** (3.5 "downgrades gracefully **and says so**"): the task card shows the active mode/degradation like a Battery-Saver indicator, and the receipt carries a mode-change line (§S10.10). A **mode change is a visible state, not a log line** — the documented silent Opus→Sonnet fallback is the exact failure mode this prevents [R09 §2.5/§4.5]. Routing between flat lanes uses the pressure gauge, effort mode, and task classification — **never dollars, never keywords** [R09 §6 (N16); D5].

### S10.7 The one run scheduler (2.2, 2.8, 3.3)

A **single SQLite-backed run scheduler** in the control plane owns run admission, priority, park/resume timing, and budget gating [R09 §4.7; G2 D2.1]. It reads and claims the S02 `queue`/`lanes`/`slots` tables (CAS claiming, per-lane/per-model slot gates from *observed* concurrency, not documented promises); storage and claim mechanics are S02's [S02.2], the policy is here.

- **Priority ladder by workload class** [R09 §4.7]: **interactive > human-blocked resumes (answered gate/ask parks, resumes-due) > scheduled-due > background > probes**, with **aging so background never starves**. Interactive is never starved by automation (3.3).
- **Cache-window-aware resume timing.** An in-TTL resume outranks same-priority cold work — a cold resume costs ~6–16× a cached one, and the subscription cache TTL is ~1 h, which is why the `⚙ freshness.hold_vs_park = 10 min` default sits below that cliff [SPIKE G1-S1 F3; G1 Def.4; S02.6].
- **Missed-slot policy (Quartz-style vocabulary):** `run-once-late` (default — fire-once-now + coalesce) / `skip` / `notify-only` (a card, not a run) [R09 §4.7]. **At v0 the only schedules are platform-internal timers**, which run as systemd `Persistent=true` calendar timers owned by [XREF:S01] (snapshots, canaries, quarterly passes); the **user-facing recurring-schedule and event-trigger surface that consumes this policy is v1** [2.8, 15.4; XREF:S19]. `⚙ scheduler.missed_slot_default = run-once-late` ships now as the ratified default, activated at v1. systemd is NEVER the task-schedule authority (no per-schedule missed-slot policy, no billing integration, `WakeSystem` unavailable to user units) — it starts Sinet and enforces `RuntimeMaxSec` transient-scope ceilings only [R09 §5; S01.2].
- **The scheduler is needed even single-user** for limit events (parks/resumes/budgets) — it is v0 machinery regardless of the v1 schedule surface [S00.2].

### S10.8 Hard ceilings & anomaly-as-scheduling (3.7)

**No run can quietly burn a week's allowance** (3.7). Per-run **time / steps / cost ceilings are columns on `runs`** [S02.2]; their default values are worker/stakes-derived and operator-editable [XREF:S08; G1 rider 1]. Enforcement facts:

- **Ceiling granularity is one model call — everywhere.** A ledger check runs at every paid-call checkpoint (the finest granularity that exists); a run admitted under budget can still overshoot by up to one call. The live evidence is the **19× overshoot on a $0.001 cap** [R09 §2.4/§4.8; R05]. The platform ledger is the *real* ceiling.
- **Engine-side ceilings are backstops, ordered outside the platform ceiling.** The wrapped-CLI `--max-budget-usd`/`--max-turns` sit at `⚙ adapter.engine_ceiling_backstop_mult ≥ 2×` the platform remainder so a same-turn engine trip does not pre-empt a park before the ledger acts; ordering and mechanics are S03's [S03.4].
- **systemd PID-1 time ceilings are the outermost backstop** — run units carry `RuntimeMaxSec` and cgroup accounting as the time-ceiling backstop the ledger cannot provide mid-call [S01.2; G1 Def.6].
- **Loop and silence detection are scheduling actions, not kills.** A flagged run is **contained → parked at its checkpoint → surfaced as a card** (pause-and-flag, **NEVER auto-kill** — the Gemini-CLI/OpenHands false-positive record is the evidence base) [D1.3; R09 §4.8/§5]. The card always offers "resume, I was wrong." Detection thresholds (loop 5×, ping-pong 6, error-loop 3×, silence budgets) and abnormal-spend alarms are the watchdog's [G2 Def.10; XREF:S14]; the park response is the scheduler's.

Re-spend after any disaster is bounded *structurally* to "work since the last paid call" (D7) and *economically* by the cache TTL a warm resume rides — the disaster economics of 3.8, whose durable mechanism is [XREF:S02].

### S10.9 Local arbitration (3.11) & never fully offline (3.9)

Local GPU/VRAM, RAM, and CPU are shared between sandboxes, local inference, and the operator's own interactive use; the platform arbitrates them, and **the operator's interactive use always wins** (3.11) [R09 §4.9; G2 Def.6]. This section owns the **admission policy**; the systemd-slice mechanism is [XREF:S01] and the GPU broker / VRAM-ledger internals are [XREF:S12].

- **CPU/RAM/IO:** systemd slices in priority order — operator session (uresourced-style active-session memory guarantee + CPU boost) > `sinet-control` > local inference > sandbox batch at `⚙ arbitration.background_cpuweight = idle` with `MemoryHigh` fences; **PSI pressure triggers pause background admission** before the desktop notices [R09 §2.8/§4.9; mechanism XREF:S01].
- **GPU:** no partitioning exists on consumer NVIDIA (MIG datacenter-only; MPS is an opt-in allocation cap, not isolation) — serialization is at the inference server, and **Sinet admission-checks a local run against the VRAM ledger** (measured model footprints + measured compositor headroom) as the **POLICY hook** before dispatch; the ledger/broker mechanics are [XREF:S12] [R09 §4.9/§5]. Operator-wins at v0 = a **manual eager-unload switch + GameMode start/end hook**; idle-detection auto-pause is post-v0 novel work with no prior art [G2 Def.6].
- **Never fully offline (3.9).** With every paid allowance exhausted, **local-feasible work continues on local models and the rest parks with resume times** [R09 §4.9]. The local watchdog/intelligence floor's residency mechanics are [XREF:S12] — **R16 revised R09's always-resident-slot sketch** (an always-resident model and deep GPU sleep are mutually exclusive; the ratified shape is TTL-warm-on-AC / CPU-tier-on-battery, so the floor is never dark and never cloud). This section binds only the policy: exhaustion demotes to local, never to silence.

### S10.10 Honest receipts (3.6, S2.5)

Every job ends with an account of what it consumed [3.6]. Receipt content (the surface is [XREF:S15]):

- **Consumed units per model × purpose**, ceremony itemized separately from execution and verification (1.10/3.4) — the `/usage` per-capability breakdown is the pattern [R09 §2.6].
- **Currency = API-equivalent for flat-rate** (from the §S10.3 table, labeled), **real dollars for metered** — the currency **visibly flips** for any metered-flagged run (P-T01-3 rehearsed flip) [R09 §4.6].
- **Mode/degradation lines** (§S10.6) and **park history** ("parked 2 h 14 m, resumed on provider signal").
- **The done-directly figure** — the honesty keystone (3.6): "what this would have cost done directly, without the platform." It follows the **two-stage formula** (per-run heuristic → per-domain measured median once ≥10 benchmark pairs exist) [G2 D2.8], whose **registered, non-retunable text lives in `Spec/benchmark-preregistration-v1.md` §13** — this spec cites that form and **does not restate its numbers** [CONTRACT §binding-layer-3]. No shipped tool presents this counterfactual; it is Sinet's to compute against the registered formula [R09 §2.6]. Every altitude of cost (run/task/project/person/period) is observable, always including this figure [S2.5; XREF:S15].

---

**Settings introduced (⚙):** (all operator-editable with audit trail per G1 rider 1; auto-adjust only within operator ceilings; every number visible on receipts)

| ⚙ setting | default | clamp / range | ratified by |
|---|---|---|---|
| `pressure.cache_read_weight` | 0.1× | 0–1 (labeled "assumed") | G1 Def.10 |
| `pressure.bg_admit_stop` | 0.7 | 0.1 – 0.95 | G2 Def.4 |
| `budget.background_window_fraction` | 0.5 | 0 – 1 (labeled "assumed") | G2 Def.4/5 |
| `meter.value_divergence_alarm` | 20 % | 5 – 100 % | R09 §4.1 / P-T08-1 |
| `limit.retry_cap` | 3 | 1 – 5 | G2 D2.1 / R09 §4.3 |
| `limit.retry_budget_ratio` | 0.10 | 0.01 – 0.5 | G2 D2.1 / R09 §4.3 |
| `limit.probe_interval_max` | 30 min | 1 – 120 min | G2 D2.1 / R09 §4.3 |
| `scheduler.missed_slot_default` † | run-once-late | {run-once-late, skip, notify-only} | R09 §4.7 |
| `arbitration.background_cpuweight` ‡ | idle | systemd CPUWeight | R09 §4.9 |

† ships now as the ratified default; the user-facing schedule surface that consumes it activates at v1 [XREF:S19]. ‡ policy value; the slice mechanism is [XREF:S01].

Per-person **automation budgets** (per lane, per period) are operator-declared 13.4 settings seeded from plan marketing shape [G2 Def.5]; they are data, not a single default. Cross-cutting settings consumed here but owned elsewhere (deduped by [XREF:S18]): `freshness.hold_vs_park`/`freshness.max_age` [G1 Def.4/5; S02/S06], `adapter.engine_ceiling_backstop_mult` [S03], the $0.50 zero-interaction band [G1 D1.7; S06], per-run `runs.ceilings` [S02.2], `RuntimeMaxSec` [S01.2].

**Known problems owned here:**
- **P-T08-1** — engine usage/cost *values* drift and break, not just schemas (3–8× usage duplication, 10× cost inflation on the primary lane within 12 months) → ledger sanity bounds + tier-1-vs-tier-2 divergence alarm (`⚙ meter.value_divergence_alarm`); conformance suites assert *values* on known fixtures, not only field presence [XREF:S14]. Extends P-T01-2.
- **P-T08-2** — budget, policy, and auth events masquerade as each other on the wire → the five-class classifier (§S10.5) is a tested component with per-lane fixtures; misclassifying Class-4 as 2/3 (retry-parking a revoked lane) is the named worst case; auth-shaped → lane-freeze [S03.6].
- **P-T08-3** — prices carry effective dates, not just values → price-table `effective_from` + scheduled-change support; the watcher diffs announced future prices [S03.6; XREF:S14].
- **P-T08-4** — pause semantics can destroy deferred work (the Zapier drop-on-expiry shape) → "pause preserves queued + parked work" is a tested invariant of one-switch pause and maintenance mode.
- **P-T08-5** — the pressure gauge inherits the `⚙0.1×` cache-weighting assumption → keep the "assumed" label; add a quarterly calibration check using *observed depletion events* (never window modeling) — measured window exhaustion vs gauge prediction at exhaustion — and alarm on systematic divergence; joins P-T02-2's cache-fidelity alarm suite [XREF:S14].
- Shared/referenced: **P-T17-1** (auth-shaped sanction → Class-4 lane freeze, never retry-park; owned S03.6) · **P-T01-3** (billing-regime flip → data-not-architecture currency flip; mechanics land in §S10.2/§S10.10, owned S03.3) · **P-T02-2** (cache-fidelity drift alarm; shared suite [XREF:S14]).

**Deferred / parked:**
- `TBD-OPERATOR(Z.AI dashboard prompt-unit calibration)` — the 5-step dashboard recipe closing the request→prompt ratio + multiplier-posting question; re-entry: operator convenience [SPIKE P2-S1 §Blocked; S03.7].
- `TBD-BRINGUP(anthropic unified-header utilization scale + 7d_oi semantics)` — P2-S3 confirmed the header *names*; the value scale and the weekly-Opus-input sub-limit behavior need one bring-up observation before the overlay is trusted for park timing [SPIKE P2-S3 §4].
- Idle-detection GPU auto-pause → post-v0 novel work; re-entry: post-v0 [G2 Def.6].
- OmniRoute-class OSS matures a D2/D4-compatible per-person pressure router → re-run adopt-vs-build for the routing layer only; the ledger/scheduler stay Sinet-owned (D7/D9-entangled) [R09 §4].
- Metered-exception lane (DeepSeek pre-registered designate) → parked while the exception list is empty at v0; re-entry: operator enables a metered exception [G1 P7; S03.6].
- systemd `Persistent=` suspend catch-up sanity test → folded into S01.7's suspend-session bring-up probe, not duplicated [XREF:S01].

**Coverage:** (Scope → subsection)

| feature-list item | subsection |
|---|---|
| 3.1 exact consumption metering | S10.1, S10.3 |
| 3.2 limit events as scheduling events | S10.5, S10.7 |
| 3.3 automation never starves the human | S10.4, S10.7 |
| 3.4 billed to requester (ceremony itemized) | S10.1 |
| 3.5 effort modes as disclosed depletion | S10.6 |
| 3.6 honest receipts + done-directly figure | S10.10 |
| 3.7 hard ceilings + anomaly | S10.8 |
| 3.8 disaster economics (bounded re-spend) | S10.1, S10.5; mechanism [XREF:S02] |
| 3.9 never fully offline | S10.9 |
| 3.10 metered spending opt-in only | S10.2 |
| 3.11 local-resource arbitration | S10.9 |
| 2.8 missed-slot policies | S10.7 (v1 surface [XREF:S19]) |
| D4 measure-not-model; reactive limits | S10.1, S10.4, S10.5 |
| D5 two currencies | S10.2, S10.3 |
| S2.5 cost observable at every altitude | S10.10 (surface [XREF:S15]) |

**Open items for G4:** none remaining.
- *Resolved inline (recorded per CONTRACT, not left open):* price-table seed source — R09 §2.6 leaned models.dev; **the gate ratified genai-prices** [G2 Def.16; D2.2], which binds (§S10.3); models.dev/LiteLLM demoted to CI cross-checks. The never-fully-offline local-floor residency — R09's always-resident-slot sketch is **superseded by R16's TTL-warm/CPU-tier shape** [XREF:S12] (§S10.9). Both are gate/later-report supersessions, not live tensions.
