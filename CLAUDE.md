# Sinet Agentic Control Hub — project instructions

Self-hosted AI agent platform for the operator plus a small trusted household, over LAN/Tailscale. Successor to Nexus (`Docs/nexus-post-mortem.md`). One maintainer, personal infrastructure, must run reliably unattended while the host is up.

## Current phase: RESEARCH — do not implement

No application code exists and none may be written yet. The order is:

1. Research sessions work through `Docs/agent-platform-feature-list-v1.md` and write findings to `Research/`.
2. Implementation starts only after the operator explicitly ends the research phase.

If asked to do something implementation-shaped during the research phase, flag it and confirm before proceeding.

## Source documents (`Docs/` — read-only reference)

- `agent-platform-feature-list-v1.md` — **the spec.** The complete capability list. Written so a session with no other context can research how to build each capability independently.
- `nexus-post-mortem.md` — what the predecessor proved and how it failed (bench-02), plus the three strategic inversions.
- `component-harvest-map-proposal-v1.md` — item-by-item what to ADOPT / PORT / PATTERN / STUDY from prior systems.

Do not edit files in `Docs/` unless the operator explicitly asks.

## Hard rules (from the spec and post-mortem)

- **D1–D10 in the spec are fixed.** Treat them as given; never re-derive, relitigate, or contradict them. Everything else is open for research.
- **Research fresh, don't anchor.** The spec exists so capabilities are researched against current best practice — not against Nexus's answers or any prior design. Use the harvest map for what's worth studying, but the post-mortem's failure analysis outranks its mechanisms.
- **Adopt, don't fork:** no modifications to any adopted engine, ever.
- **Real-world facts need live research.** Anything resting on provider behavior, pricing, ToS, model capabilities, or library status must be verified against current sources (mid-2026), never answered from memory.

## Research output conventions

Findings go in `Research/` — one file per topic, named `NN-topic-slug.md` in session order. See `Research/README.md`.

## Git

- Default branch: `main`. Remote: private GitHub repo `dariannixda-eng/Sinet-Agentic-Control-Hub`.
- Commit research findings as they complete; push after milestone commits.
