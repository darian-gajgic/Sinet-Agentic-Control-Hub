# Sinet Agentic Control Hub

Self-hosted AI agent platform: a factory for supervised AI work — any knowledge work, for several people, on the flat-rate subscriptions they already pay for. Successor to Nexus (rebuild-with-transplants, not an in-place update).

**Status: research phase.** No implementation exists yet. Before any code is written, dedicated research sessions (run on Fable 5) work through the feature list and produce build research in `Research/`.

## Repository layout

| Path | Contents |
|---|---|
| `Docs/` | Source documents — the inputs to research and build. Treat as read-only reference. |
| `Docs/agent-platform-feature-list-v1.md` | **The spec.** Complete capability list (titled v2 inside; supersedes the earlier draft). Its "Decided constraints" D1–D10 are fixed and must never be re-derived or contradicted. |
| `Docs/nexus-post-mortem.md` | Prior art: what Nexus proved, how bench-02 failed, the three strategic inversions for this rebuild. |
| `Docs/component-harvest-map-proposal-v1.md` | What to ADOPT / PORT / PATTERN / STUDY from Nexus and other systems. |
| `Research/` | Output of the research sessions. See `Research/README.md` for conventions. |
| `CLAUDE.md` | Standing instructions for Claude Code sessions in this repo. |

## Starting a research session

Open a fresh Claude Code session (Fable 5) in this directory and paste:

```
Read CLAUDE.md and Docs/agent-platform-feature-list-v1.md in full. We are in the
research phase: research HOW to build the platform's capabilities — current best
practice, concrete technology choices, trade-offs — before anything is implemented.
Treat the Decided constraints D1–D10 as fixed. Write your findings to Research/
following the conventions in Research/README.md. Start with: <topic or section>.
```

Scope each session to one section or capability cluster of the feature list so findings stay deep rather than broad.

## Build sequence (decided, from the spec)

1. **v0** — software development end-to-end, single-user (operator only)
2. **v0.1** — web research as second full-pipeline domain
3. **Benchmark gate** — the platform must prove it beats using a frontier model directly
4. **v1** — household onboarding, degraded-mode domains, scheduling
