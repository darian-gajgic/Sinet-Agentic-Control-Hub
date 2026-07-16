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
