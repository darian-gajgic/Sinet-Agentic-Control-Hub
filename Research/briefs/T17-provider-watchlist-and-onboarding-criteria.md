# T17 — Provider watchlist & onboarding criteria (T01 extension)

**Wave:** A1b (addendum — authorized during the Wave A2 hold, 2026-07-16) · **Depth:** FULL rigor, narrow scope (bounded fan-out; this extends an existing report, it does not redo it) · **Report slug:** `provider-watchlist-and-onboarding-criteria`

## Scope
2.7 (gap advice needs a *complete and refreshable* subscription-landscape table), Operating reality (subscription-coverage rule), D3 (every new provider = a lane behind the same adapter contract), 3.1/13.4 (per-model flat/metered flags as data), S2.8 (outside-world drift watch). **Extends `Research/01-execution-engines-and-adapters.md` — read it first; do not re-audit providers it already covers except where you find their status changed.** Resolves report 01's open question #8 (refresh cadence/source list for the provider table).

## Why this matters
Operator directive (2026-07-16): the platform must be designed from the beginning to integrate new models/providers cleanly the moment something good becomes available on a subscription basis. The *architecture* for that is settled (report 01 §4: adapter-only coupling, billing regimes as data). What's missing is **coverage** (xAI and DeepSeek are absent from report 01's table; hosted open-weights subscriptions are thin) and the **standing process** (onboarding criteria + watchlist) the spec must encode.

## Core question
Which providers beyond report 01's table — xAI (Grok 4.5 era), DeepSeek, any further credible Chinese providers, and hosted open-weights/flat-subscription services — currently offer (or credibly announce) subscription-covered programmatic use fitting Sinet's lanes; and what onboarding-criteria checklist and watchlist process should the platform encode so any future provider is added as configuration plus an adapter lane, never a re-architecture?

## Sub-questions
1. **xAI audit (primary sources, dated):** current consumer/prosumer plans (SuperGrok-class tiers), what programmatic surfaces exist (first-party CLI/agent tool? API only? OAuth support in third-party tools?), whether any plan covers programmatic/agentic use flat-rate, ToS language on automation and account sharing, and Grok 4.5's actual standing for coding/agent work per independent (non-xAI) evals.
2. **DeepSeek audit:** subscription products vs pure metered API; any flat programmatic lane; tool-ecosystem support (Anthropic/OpenAI-compatible endpoints?); automation ToS; model relevance mid-2026.
3. **Sweep for additional Chinese providers** not in report 01's table (it covers Z.AI, Moonshot/Kimi, MiniMax, Alibaba/Qwen): anything credible with coding-plan-style subscriptions and programmatic access (verify actual sanction, not marketing).
4. **Hosted open-weights / flat-subscription lane:** services serving strong open-weights models with subscription (not per-token) programmatic plans — Cerebras Code status update (report 01: "sold out"), Groq-class, Together/Fireworks-class, and whatever exists mid-2026; for each: plan terms, tool whitelists, protocol compatibility, limits behavior.
5. **Open-weights frontier snapshot (thin — for routing awareness only):** which current open-weights families are competitive for Sinet's work classes per independent evals, as (a) hosted-flat candidates here, (b) own-GPU candidates to flag forward to T15 (do not duplicate T15's depth).
6. **The onboarding criteria checklist (the durable spec artifact):** the tests a candidate provider/plan must pass to become a lane — ToS sanction for programmatic subscription use (primary-source), per-person accounts compatible with D2, usage-reporting fidelity (D4 metering), limit-event behavior observable (3.2), protocol fit (OpenAI/Anthropic-compatible endpoint, first-party tool worth wrapping, or disqualified), no-pooling compliance, exit cost. Express as a checklist the spec can adopt verbatim.
7. **The watchlist process:** which sources move first when a provider changes terms or a new plan launches (the report-01 churn arc as calibration); a concrete refresh cadence + source list for the 2.7 table (resolving report 01 OQ#8); how S2.8's drift watch consumes it.
8. **Cross-check:** does anything found here change report 01's recommendation (dual substrate, opencode API lane + Claude wrap lane)? Explicit yes/no with reasoning.

## Constraints that bind this topic
Same as T01: D2 (per-person, never pooled), D3, D4 (never model quota windows), D5, no load-bearing metered paths (metered = explicit exception only), adopt-don't-fork. A provider with strong models but no sanctioned subscription-programmatic path belongs in "NOT usable (and why)" with its would-change-this trigger — not in the lanes.

## Harvest-map items to verdict
O2 rider only: report 01 marked opencode's provider breadth CONFIRM-with-revisions — verify specifically whether opencode's xAI/Grok OAuth path (named in the map) exists and what it rides on today.

## Sources to prioritize
Provider pricing/ToS/docs pages (primary, dated); first-party tool docs; independent model evals (non-vendor); tool-whitelist pages; credible reporting on plan launches/changes (corroboration only).

## Output contract deltas
Standard 8 sections, plus: (a) an **updated consolidated provider table** — superset of report 01 §2.1's table, same columns, changed rows marked; (b) the **onboarding criteria checklist** as a standalone, spec-ready section; (c) the **watchlist: sources + cadence** section resolving report 01 OQ#8.

## Decisions this feeds
G1 (any change to engine/lane direction — expected none, verify); spec: provider-onboarding section, 2.7 gap-advice data feed, S2.8 watch inputs. T15 (flagged model families), T08 (any new limit-event shapes).
