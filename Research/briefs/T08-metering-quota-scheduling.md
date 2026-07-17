# T08 — Consumption metering, quota handling & scheduling

**Wave:** B1 (consumes G1 addendum) · **Depth:** FULL · **Report slug:** `metering-quota-scheduling`

## Scope
3.1–3.11 complete (exact metering; limit events as scheduling events; automation budgets; per-requester billing; effort modes Eco/Balanced/Smart; honest receipts incl. done-directly comparison; hard ceilings; disaster survival [economics side]; never-fully-offline; opt-in metered; local-resource arbitration), D4, D5, 2.8 (missed-slot policies), S2.5 (cost observability feeds), 3.4 (attribution).

## Why this gates the spec
The whole platform rides on subscription economics (Operating reality): exact self-metering replaces provider quota knowledge that doesn't exist (D4), and routing between flat-rate options must use consumption pressure, never dollars (D5). This is also the layer least represented in public tooling — most of the world meters dollars, Sinet meters *windows* — so finding what exists vs what must be designed is the point.

## Core question
How does Sinet measure consumption exactly — per person, model, task, and period — across heterogeneous substrates (wrapped first-party CLIs, direct APIs, local models), treat provider limit events as routine recoverable scheduling events without ever modeling quota windows, and schedule work under per-person automation budgets with effort modes defined against depletion?

## Sub-questions
1. Usage extraction per substrate (per G1's engine direction): what usage/cost data the wrapped CLI actually emits today (fields, fidelity, drift history), API usage-reporting fields per provider, token accounting for local models — and where exact metering is impossible, the honest approximation hierarchy.
2. Limit-event taxonomy mid-2026 per major provider: how limits present (429s, load-shed vs hard-window signals, retry-after or reset hints, in-band messages in first-party tools); distinguishing transient shedding from real depletion (harvest N3's Z.AI lesson — generalize it); what resume-time signals exist.
3. Park-and-resume scheduling: patterns for parking limit-blocked runs losing nothing and resuming unattended (provider signal where available, else retry schedules) — who does this well; interaction with checkpointing (T07).
4. Budget enforcement: automation budgets per person/model/period enforced at spawn time AND mid-run; reserving interactive headroom ("automation never starves the human", 3.3); one-switch pause semantics.
5. Effort modes against depletion (3.5): Eco/Balanced/Smart as consumption policies, graceful downgrade with disclosure; prior art for consumption-pressure routing between flat-rate options (D5's no-dollars rule between flat rates) — likely novel; design it from adjacent evidence if so.
6. API-equivalent price table (3.1, D5): maintaining a user-editable table with sane shipped defaults; where credible current per-model API prices come from; receipt generation (3.6) incl. the done-directly comparison figure.
7. Scheduling machinery: queue/priority design for mixed interactive + background + scheduled work on one host; missed-slot policies (2.8: run-once-late / skip / notify-only) implementation patterns; off-hours batching per user config.
8. Local resource arbitration (3.11): GPU/VRAM/RAM/CPU sharing between local inference, sandboxes, and the operator's interactive use (operator always wins) — practical single-host arbitration (cgroups-class controls, inference-server queueing, VRAM management), bridge to T15.
9. Ceilings + anomaly cutoffs (3.7): per-run time/step/cost limits and circles/silence detection as *scheduling* actions (contain, park, surface).

## Constraints that bind this topic
D4 (never predict windows — designs that model provider quotas are What-NOT-to-use material), D5 (two currencies discipline), 3.10 (metered spend requires an explicit per-use flag), 3.4 (every run billed to its requester incl. ceremony itemization), D2 (per-person substrates — metering is inherently per-owner).

## Harvest-map items to verdict
N3 (quota-storm handling), N8 (frontier ledger / honest accounting incl. API-equivalent figures), C4 (quota-window surfacing UX — as *observed state*, not prediction), N16 (routing telemetry — the rationale-storage habit).

## Sources to prioritize
Provider docs/help pages on plan limits and programmatic use (primary, dated); first-party CLI output-format docs and change logs; community measurements of subscription limits (credibility-weighted); scheduler/queue engineering for single-host systems; GPU-sharing practice for inference hosts.

## Decisions this feeds
G2: metering design per substrate, limit-event handling policy, budget/effort-mode semantics, scheduler shape. Spec: metering/ledger schema, scheduler, receipts.

## G1 addendum (gate closed 2026-07-17 — binding input; cite `Research/decisions/GATE-1-architecture-direction.md`, not conversation)
G1 ratified: **dual substrate** (pinned `opencode serve` + wrapped `claude -p` per user; SDK in-adapter alternative, S1 spike pending) · run FSM + SQLite-WAL event log · fresh-context-per-stage on the Task Context Ledger · spawning control-plane-owned, engine-native subagents disabled (operator rider) · two-axis verification. Metering-relevant ratified inputs (all operator-editable settings — operator rider; auto-scaling only within operator ceilings, auto-raises on receipts): **v0 lanes = Anthropic Max + Z.AI GLM Coding Max ONLY (operator already holds both — Max tier is real data: ~1,600 prompts/5 h, 2+ concurrent, prompt≈15–20 calls, peak 3×/off-peak 2× multipliers)**; metered-exception list EMPTY at v0 (3.10 strictest); pre-registered P1 policy: Anthropic un-pause ⇒ interactive-only demotion, headless → Z.AI/local; cache-read pressure weighting ⚙0.1× labeled "assumed"; stage budgets ⚙50 % target / ⚙70 % overflow-event; zero-interaction band cost threshold $0.50 API-equiv; helper ceilings 4 concurrent / ~20 turns / ~80k tok / spawn budget 8 / reports ≤2k tok; escalation SLAs: canary daily, approval cards remind 4 h + push 24 h, safety immediate + hourly, drill quarterly; hold-vs-park 10 min. Known-problems routed here: P-T01-3 (billing flips = ops events), P-T17-1/2/3 (auth canary, overflow_mode, region model gates), P-T02-2 (cache-fidelity alarm), lane-heterogeneous consumption units (R07). **Spike addendum #2 (esp. S1 CLI-vs-SDK cache fidelity) follows before launch — read `Research/spikes/` if present.**

## Spike addendum #2 (G1 battery complete 2026-07-17 — measured facts, cite `Research/spikes/G1-S{1,2,3}-*.md`)
**S1:** `total_cost_usd` + per-model `modelUsage.costUSD` emitted under subscription auth on both Anthropic surfaces → D5 API-equivalent receipts need no surface-specific handling. Cache-fidelity alarm baseline measured: healthy resume = `cache_read ≈ full prior context, cache_creation ≈ new-turn-only`; regression signature = `cache_read ≈ 0` on an in-TTL resume. Subscription requests 1h TTL (live-confirmed); **cold resume costs ~6–16× a cached one** → schedule resumes inside the cache window when queue pressure allows; direct input to the 10-min hold-vs-park economics. `rate_limit_event` carries `resetsAt`/`rateLimitType`/`overageStatus` in-band → the 3.2 park scheduler gets a machine-readable resume time for free. **S2:** engine ceiling signals are `error_max_budget_usd`/`budget_exhausted`; a same-turn budget trip pre-empts a defer park even after the gate fired → **wrapper ledger is the primary ceiling, engine flags a ≥2× backstop**; zero-cost poll-resumes ($0 measured) must not count as attempts or spend; 1M-window cache-write premium makes tiny `-p` runs disproportionately expensive (~$0.13–0.20) → pin standard-window models for probe batteries/short runs. **S3:** opencode 1.18.3 session rows carry typed cost/token columns (input/output/reasoning/cache read+write) as first-class SQLite fields — cross-check material for D5; `GET /session/status` `retry` variant carries `attempt`/`next` + provider-`action` — a machine-readable 429/limit signal for D4 park semantics.
