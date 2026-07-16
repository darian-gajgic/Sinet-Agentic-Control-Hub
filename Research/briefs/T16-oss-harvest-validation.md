# T16 — OSS harvest validation & adoptable-component sweep

**Wave:** B2 (deliberately after Wave A — verdicts must reference fresh findings, not anchor them) · **Depth:** FULL · **Report slug:** `oss-harvest-validation`

## Scope
The entire `Docs/component-harvest-map-proposal-v1.md` (operator's explicit ask: "check if this is valid and could be used"), plus a fresh sweep for adoptable components the map misses (operator: "we do not have to design and code everything from scratch"). Feature-list touchpoints: everywhere the map points.

## Why this gates the spec
The map is a *proposal* written before this research campaign, in coordinates from a previous planning generation (it references "slices", "RQ" numbers, "briefing v2.2", "Constitution rules" — none of which exist in this repo). Before the spec adopts anything from it, every load-bearing item needs a current-status check, and every kept item needs re-targeting to feature-list item numbers. Standing rule: **the post-mortem's failure analysis outranks the map's mechanisms** wherever they tension.

## Core question
Item by item, is the component harvest map still valid in mid-2026 — licenses, maintenance status, claimed capabilities, adoption modes — and what adoptable open-source components does it MISS that would materially reduce what Sinet must build from scratch?

## Sub-questions
1. **Per-source status checks (primary sources, dated):**
   - opencode (O1–O4): release cadence, license, maintainer health, the per-session-model claim, server/API surface — cross-check against T01's report if already committed.
   - Archon post-pivot (A1–A7): current state, license, the DAG-builder/dashboard claims, the telemetry flag, the `interactive: true` and `fresh_context` features as they exist NOW.
   - Crush (K1–K2): exact current license terms — confirm patterns-only is legally required, or whether status changed.
   - gh-aw (G1): safe-outputs design current docs.
   - OpenHands, Goose (fallback engines): maintenance status, license — still credible fallbacks?
   - deepagents, n8n starter-kit, awesome-harness-engineering (reference rows): still the right references?
2. **Nexus port items (N1–N22):** these are the operator's own code (no license issues) — validate each against (a) the post-mortem's mechanisms-not-strategy rule and (b) Wave-A findings where committed (e.g., if T02/T07 found the adopted engine or a library provides an equivalent, the port is SUPERSEDED). Flag every port item that fresh evidence makes unnecessary — porting is not free.
3. **Dangling-coordinate repair:** for every KEPT item, re-target its "Target" column to feature-list item numbers (e.g. "Slice 05" → "3.2/D4 scheduler"); list items whose targets have no feature-list home (candidates for dropping).
4. **The NEW-candidates sweep** (the operator's explicit ask — search broadly): approval-inbox / human-in-the-loop libraries (HumanLayer-class and successors), agent control planes / orchestrators that emerged or matured since the map (self-hosted, adoptable-unmodified), durable-execution runtimes with agent support (cross-reference T07 if committed), agent-platform UI kits (task boards, trace viewers, approval UIs), credential-broker / secrets tooling fitting D2, anything covering feature areas with no current donor (intake pipeline, metering, benchmark practice). For each find: license, maintenance, what it would replace, adoption mode (ADOPT/PATTERN/STUDY).
5. **License audit table** for everything recommended ADOPT: license, copyleft/fair-source implications for a private self-hosted platform, NOTICE obligations.
6. **Anti-harvest list review:** does 2026 evidence still support each exclusion (peer-mesh topologies, vector-first memory, standing rosters, metered load-bearing paths)? Strengthen, keep, or soften each — with sources.
7. Verdict discipline: every map item gets exactly one of CONFIRM / REVISE (state the revision) / REJECT (state why) / SUPERSEDED-BY (name the replacement). Every new candidate gets a one-line "what it saves us building".

## Constraints that bind this topic
Adopt-don't-fork (any component needing modification is PATTERN at best), no load-bearing metered paths, D1 (self-hosted only — SaaS components disqualified as dependencies, allowed as pattern references), post-mortem-outranks-map on every conflict.

## Sources to prioritize
The repos themselves (releases, commits, issues, licenses — primary), maintainer announcements, independent comparisons/reviews (2025–2026), license texts (primary).

## Decisions this feeds
G2: the adoption list (what the spec builds ON vs builds fresh), NOTICE/license obligations, updated harvest map (this report effectively supersedes the proposal doc — the spec cites this report, the proposal stays untouched in Docs/).
