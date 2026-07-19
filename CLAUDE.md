# Sinet Agentic Control Hub — project instructions

Self-hosted AI agent platform for the operator plus a small trusted household, over LAN/Tailscale. Successor to Nexus (`Docs/nexus-post-mortem.md`). One maintainer, personal infrastructure, must run reliably unattended while the host is up.

## Current phase: P3 — IMPLEMENTATION (research phase ended at G4, 2026-07-19)

The research campaign is complete: 17 topics, spike batteries, gates G0–G4 all closed. **The binding build contract is `Spec/core-architecture-v1.md` (v1, frozen at G4 — git tag `spec-v1`)**, assembled from `Spec/drafts/S00…S19` (drafts canonical), plus binding siblings `Spec/benchmark-preregistration-v1.md` (signed; its registered numbers change only via its own §17) and `Spec/frontend-components-v1.md`.

- **Build from the spec, not from memory or the reports.** P3 sessions implement per the S19.5 build order (B0 spine → B1 substrate → B2 pipeline → B3 workforce/memory → B4 deliverables/local tier → B5 observability → B6 frontend) and the S19.6 bring-up measurement sequence. Every load-bearing statement in the spec carries a provenance tag; the spec wins over any report.
- **Post-freeze spec changes** follow S00.9 amendment mechanics (dated changelog entry + operator approval); any amendment touching a ⚙ setting re-runs the S18 sweep.
- `Research/` is the closed evidence archive (`Research/STATE.md` is historical; gate records in `Research/decisions/`).
- **Implementation process:** coordinator runbook `.claude/skills/p3-implementation/SKILL.md` + live state `P3/STATE.md`. Any session continues the build via "continue implementation" (or /p3-implementation); work runs in spec-referenced packets executed by fresh-context subagents, with operator gates at each B-phase boundary.

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
