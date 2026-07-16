# Shared research context — read before any topic brief

Every research run consumes this file plus its topic brief (`T##-*.md`). Where this digest and the full spec differ, the spec wins: `Docs/agent-platform-feature-list-v1.md`. Read the spec sections your brief's Scope lists — they are the requirements your findings must serve.

## The project in one paragraph

**Sinet Agentic Control Hub** — a self-hosted platform for supervised AI knowledge work: any task domain, for the operator plus a small trusted household, over home LAN/Tailscale, on the flat-rate consumer AI subscriptions each person already pays for. One maintainer. Must run reliably unattended while the host (a mobile laptop) is up. In one line: *a factory for supervised AI work — dependable enough to run while the host is up, honest enough to audit, and self-extending enough that it grows new workers instead of the operator writing them.* Successor to Nexus (see post-mortem digest below). Currently in research phase: no code exists; these reports feed the core architecture spec.

**Operator context that weighs every trade-off:** solo enterprise software architect maintains everything alone (bus factor 1 is a design input, not an accident to lament); users range from the technical operator to household members with zero IT/AI knowledge; hardware is one laptop — Intel Ultra 9 275HX, RTX 5070 Ti Mobile 12 GB VRAM, 32 GB RAM (expandable 96 GB), Ubuntu 26.04 LTS, optional RTX 3090 24 GB eGPU as a second, separate VRAM pool.

## Fixed constraints D1–D10 (digest — research WITHIN these, never against them)

- **D1 Central topology.** All execution on the operator's machine; members connect over LAN/Tailscale. No runners on member devices.
- **D2 Per-person credentials on the host.** Each person's provider credentials in their own store; every run authenticates as its owner; subscriptions never pooled. No OS-user separation (mutual-trust household): **the sandbox is the load-bearing isolation boundary, and credentials never enter task sandboxes.**
- **D3 Dual execution substrate behind one adapter contract:** start, stream progress, checkpoint, pause, resume, cancel, report usage. Per provider: wrap the provider's first-party tool where subscription terms require it, else call an API directly (including local models).
- **D4 Consumption metering, reactive limits.** Measure consumption exactly; treat provider limit events as normal recoverable scheduling events; never model or predict providers' quota windows.
- **D5 Two currencies.** Between flat-rate options, routing uses consumption pressure — never dollars (marginal cost there is zero). Dollars (from a user-maintained API-equivalent price table) are the reporting currency, and a routing input only for explicitly enabled metered use.
- **D6 Single-coordinator topology with a depth cap.** One coordinator per task, isolated helpers, sub-helpers only within a configurable depth cap (default 2 below the coordinator), **no lateral messaging at any depth**, every spawn logged with its reason.
- **D7 Checkpoint-and-gate invariant.** Progress checkpointed after every paid model call; all non-idempotent outward effects exist only as gated proposals until a human approves.
- **D8 Worker model: template → overlay → instance.** Versioned templates (personal by default; household-shared only via operator approval), per-user overlays (lessons/preferences), per-run instances (notes expire with the task).
- **D9 Per-project git, GitHub as the off-host home.** Accepted work = attributed commits by the accepting user; each user's project repos live under their own GitHub account; platform-state snapshots to one operator-owned private repo (client-side encrypted).
- **D10 Approval principle.** Everyone approves their own objects; promotion of anything to household-shared requires operator approval.

## Operating-reality facts (bind your recommendations)

- **All frontier-AI usage must be covered by each person's flat-rate consumer subscription.** Never pooled. The operator has recorded that Anthropic's Agent SDK is moving to credits-based billing and is therefore ruled out as the engine — T01 verifies this against primary sources; every other topic treats it as given. Pay-per-token is an explicit, deliberately enabled exception — never a default, never a silent fallback.
- At least one major provider permits subscription-covered programmatic use **only through its own first-party tool** (hence D3's wrap-or-API split).
- Providers do not reliably expose remaining-quota state; the platform meters its own consumption and reacts to limit events (D4).
- A local GPU is a **permanent free tier**: the platform's own background intelligence (health watching, change detection, risk-ranking, routine classification) runs on local models only and keeps working when every paid window is empty.
- Human control is a product requirement: defined decision points, answerable from any device. Availability is best-effort while the host is up; schedules have explicit missed-slot behavior.

## Post-mortem lessons (Nexus, the predecessor — failures outrank its mechanisms)

Nexus proved control/execution-split resilience, race-proof scheduling, gate discipline, and honest accounting — and lost its own benchmark: 28/50 quality at ~$241 API-equivalent vs 43–44/50 at ~$14–16 for direct frontier use (n=1). The rebuild's three inversions are standing policy:

1. **Adopt, don't fork.** The engine is a running dependency, configured only — no core modifications, ever.
2. **Route the work, not just the judging.** Mid-2026 subscription economics put frontier *execution* on flat-rate paths; the cheap-executor/frontier-judge split is dead.
3. **Validate before breadth.** A pre-registered added-value benchmark gates every feature beyond the v0 core.

Named traps your findings must not reintroduce: **P45** spec lock-in (verification amplifies whatever objective it's given — every verifier needs a second, spec-independent "is this actually good?" axis); **P46** escalation routes must exist AND be proven by tests (a finding that dies in a log is a platform defect); **P47** data-bearing tasks get live research injected by policy, never left to model initiative; multi-stage pipelines re-sending context per stage produced a measured ~15×-class token multiplication — cost structure is an architectural property, not a tuning knob.

## How to treat prior art

The component harvest map (`Docs/component-harvest-map-proposal-v1.md`) lists candidates from Nexus, opencode, OpenClaw, Archon, Crush, SAW, gh-aw. Your brief names the items in your scope. Treat them as **candidates to evaluate against the current field, never as defaults** — this campaign exists to research fresh, not to ratify last week's plan. Where your evidence contradicts a map item, say so plainly: the map is a proposal; your report is the evidence.

## Sourcing standards (non-negotiable)

- **Live research only** for anything that can drift: pricing, ToS, licensing, maintenance status, model capabilities, library APIs. Model memory is not a source. Cite **URL + access date**.
- Load-bearing claims need **≥2 independent sources**. Single-source claims are flagged as such.
- ToS/pricing/licensing claims need **primary sources** (the provider's/project's own pages) — secondary reporting only as corroboration.
- Prefer sources ≤12 months old (it is mid-2026; the field moves monthly). Older material must be flagged as possibly superseded. Leading-source classes to draw on: vendor engineering blogs (Anthropic, OpenAI, Google), framework blogs (langchain.com/blog and peers), maintainer repos/release notes, respected practitioners (evals/agents), academic surveys, and production war stories (weighted for credibility) — cross-verified, since the operator explicitly distrusts any single source.
- Production evidence outranks demos; demos outrank blog claims; blog claims outrank marketing. Say which tier each key claim rests on.

## Output contract (every report)

Write the report to the exact output path you are given (`Research/NN-<slug>.md`), structured:

1. **Scope** — feature-list items covered (from the brief).
2. **Current state of the art (mid-2026)** — how this problem is solved now, dated.
3. **Candidate approaches** — concrete technologies/designs, trade-offs against the operator context above.
4. **Recommendation for Sinet** — one primary approach, reasoning, and *what would change the decision*.
5. **What NOT to use and why** — rejected candidates and popular-but-wrong-here approaches, each with the reason.
6. **Harvest-map verdicts** — for each item your brief lists: CONFIRM / REVISE (how) / REJECT (why) / SUPERSEDED-BY (what).
7. **Open questions** — answerable questions needing an operator decision or later research, each with a proposed owner. Include any **new platform problem** you discovered (the spec's Known-problems list expects research to keep hunting).
8. **Sources** — URL, access date, one line on what each supports.

Length: whatever decision-grade density requires — typically 300–700 lines. No padding, no generic tutorial prose; every section earns its place by changing what Sinet builds.
