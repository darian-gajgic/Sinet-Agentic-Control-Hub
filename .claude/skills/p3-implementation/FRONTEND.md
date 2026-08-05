# Frontend workflow — the design-work pipeline (operator-directed 2026-08-05)

**Why this exists:** the UI batch (UI-1…UI-7) was built with the backend four-stage pipeline and was rejected whole by the operator on first real use — mechanically green, humanly unusable. The error analysis and the research behind every rule here are in `P3/design/frontend-workflow-research-2026-08-05.md`; the operator findings are in `P3/design/rework-operator-findings.md`. **Scope: any work whose primary output is presentation or interaction — `web/` views, styling, UX flows, navigation, copy. Backend packets keep the four-stage pipeline in `SKILL.md` unchanged. Mixed work is split so each half runs under its own pipeline.**

The spec still wins on behavior (S-refs, honesty invariants, FC-v1 component picks, content-vs-telemetry — all binding, unchanged). What changes is how presentation gets designed, built, and judged.

## The five rules that replace the four-stage pipeline for frontend work

**1. One author.** A single long-lived Fable agent owns the entire view layer (or, for later smaller features, an entire user journey) in one continuous context — continued across sessions via SendMessage, never re-derived. Subagents may research and review; they never co-author screens. Fresh-context-per-surface authorship is banned: it is the documented cause of incoherent UI (each context makes hundreds of implicit design decisions no other context can see).

**2. Reference over prose.** The builder gets the actual working reference in context — for the rework: the Nexus source files (`~/Nexus-Agentic-Coding-Setup/app/static/style.css`, the relevant `app.js` view functions) and screenshots of the running Nexus app — never a prose survey of them. The operator-ratified art direction (2026-08-05): **recreate the Nexus look** — the violet glass control-room design language, its token values, hierarchy and control-room anatomy — on the current React/Tailwind stack, adapted only where the current backend contract differs. Prose specs of visual design are banned; where direction is genuinely open, render 2–3 variants and let the operator pick.

**3. The product map comes before code, and it is user-shaped.** One page written from the operator's chair: the jobs to be done, the navigation with plain names, per surface "what you see / what you can do", and — for every carried reference pattern — a carry/adapt/drop verdict reconciled against the CURRENT backend contract (`web/src/api.ts` + the spec's behavior sections), so stale reference behavior surfaces here, not in built screens. Work is sliced by user journey, never by spec section. **Operator checkpoint 1: the map is approved before the first component is written.**

**4. The builder looks at every change.** The builder runs the app on the seeded demo world and verifies in real Chrome: full pages and full flows, at desktop and narrow widths — never per-region crops, never jsdom-only. Every increment ends with render → screenshot → self-diff against the reference → iterate; evidence is the screenshot, not an assertion. Goal-shaped build prompts (intent + audience + constraints + the quality modifier "go beyond the basics — fully-featured, polished"); no R-numbered step lists — over-prescription measurably degrades output. **Operator checkpoint 2: rendered screenshots after the shell + first surfaces land; then autonomous until the exit gate.** After two failed corrections on the same point, reset the approach instead of patching the patch.

**5. Judged on pixels by fresh eyes; tests filter, the operator gates.** Landing requires, in order:
   - **Live design review** — a fresh-context agent (browser tools, never the builder) drives the RUNNING app through interaction flows, responsiveness, visual polish against the Nexus bar, and accessibility, grading Design Quality / Originality / Craft / Functionality. Scoped to correctness and requirement gaps plus those four criteria — findings triaged by the coordinator, drain capped at two rounds as ever.
   - **Cold walks** — fresh-context agents given only operator-level knowledge attempt real tasks in the browser ("create a task asking for a picture of a car", "find the judge's feedback on revision 2", "reprioritize the queue") and narrate friction. A failed walk blocks landing.
   - **The machine battery last** (typecheck, vitest, go tests, lockgate, escape scan, zero-drift regen) — binding as a filter, never the definition of done. Behavior-contract tests stay authoritative; presentation-coupled tests are (re)written after the design settles, and screenshot baselines start life only from operator-approved states (ratchet, not judge).
   - **The operator's eyes are the only final gate** — click-through on the seeded world, free-text feedback authoritative.

## Model routing (frontend)

| Role | Model | Notes |
|---|---|---|
| Builder (single author) | **`model: fable`** | strongest dense-screenshot vision for the render loop; goal-shaped prompts only |
| Live design reviewer | fresh `model: fable` + Chrome tools | judge ≥ executor holds (Fable–Fable, fresh context, hazards named in advance) |
| Cold walkers | fresh `model: fable` | operator-persona knowledge only |
| Any classifier trip | lossless `model: opus` relaunch | standing remedy (memory `fable5-safeguard-false-positive`); zero trips across the entire UI batch |

## Banned (each one shipped a defect class in the UI batch)

- Slicing UI work by spec section, layer, or control inventory.
- A fresh context authoring any screen mid-journey.
- Prose descriptions standing in for a visual reference that exists as code or pixels.
- Accepting on rubric/tests without a live-browser look at full pages and flows.
- Per-region-only or jsdom-only visual verification.
- Prescriptive R-numbered briefs for design work.
- "Looks good" without an attached screenshot.

## What carries over from the backend pipeline

STATE.md discipline (update before/after every step), the landing hygiene (coordinator re-runs the battery, spot-diff, commit, push after milestones), the spec-conflict amendment path, adopt-don't-fork + lockgate, ⚙-through-registry, the failure ladder, and the hard boundaries — all unchanged. Only the shape of design work changes: one author, real references, screens judged as screens.
