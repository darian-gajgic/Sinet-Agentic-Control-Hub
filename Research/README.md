# Research output

Findings from the pre-implementation research sessions. One file per topic, prefixed with a two-digit sequence number in the order they were produced:

> **Campaign machinery (added 2026-07-16):** the research runs as a coordinated campaign. `CAMPAIGN.md` is the master plan (topics, waves, gates), `STATE.md` the live progress pointer, `briefs/` the per-topic research briefs (with `00-shared-context.md` as the shared payload), `decisions/` the gate memos with operator decisions. Any Claude Code session resumes it by reading `STATE.md` — or via the `/research-campaign` skill. Topic reports keep the `NN-` numbering convention below; `STATE.md` maps topic IDs (T##) to report numbers.

```
Research/
  01-execution-adapters.md
  02-consumption-metering.md
  03-sandboxing-confinement-ladder.md
  ...
```

## What a research file must contain

1. **Scope** — which feature-list items (e.g. "3.1–3.6, D4, D5") this covers.
2. **Current state of the art** — how this problem is solved in mid-2026, with sources and dates. Live-researched, not from model memory.
3. **Candidate approaches** — concrete technologies/designs with trade-offs against the operator context (solo maintainer, one laptop, subscription economics).
4. **Recommendation** — one primary approach with reasoning, plus what would change the decision.
5. **Open questions** — anything that needs an operator decision or further research, stated as answerable questions.

## Rules

- Decided constraints D1–D10 from the spec are fixed inputs — research *within* them, never against them.
- Cite sources with URLs and access dates for anything that can drift (pricing, ToS, library maintenance status).
- If research surfaces a new problem the platform must solve, add it to the file's Open questions section with a proposed owner — the spec's "Known problems" list expects research to keep hunting.
