# Operator findings — the UI batch rejection (2026-08-05)

The operator's first real use of the restyled app (informal walk, before the scripted click-through). Verdict: **complete rework**; piecemeal fixing rejected ("if I would go through one by one, I would take days"). These findings seed the rework's product map (FRONTEND.md stage: checkpoint 1). Free-text feedback is authoritative per the standing gate rule.

## The verdict, in the operator's terms

Unusable, unintuitive, unreadable, "not understandable or recognizable for a human"; on the level of early-2025 model output, "made without any prototype". Stopped testing out of anger — feedback per-surface is pointless until the structure is humane. The Nexus frontend is the explicitly named quality bar ("most of this was already good in the Nexus frontend"), with the caveat that not everything carries — backend logic changed, so every carried pattern needs a carry/adapt/drop verdict.

## Specific findings (with triage where the session established it)

| # | Finding | Triage |
|---|---|---|
| 1 | Deliverables tab: style errors; the side-by-side comparison unreadable | Broken presentation |
| 2 | The 2-up / swipe / onion boxes: clicking them writes text over text, unreadable; purpose of the controls not communicated | Confirmed defect — the S13.2 image trio overlays the ENTIRE figure (caption + text fallback included) absolutely at 50% opacity (`web/src/Deliverable.tsx:1326-1364`); GitHub's pattern overlays only aligned bitmaps. Also a labeling failure: three unexplained jargon radio boxes |
| 3 | Workforce tab: style errors | Broken presentation |
| 4 | Kanban board "missing" | **Exists** (`web/src/Board.tsx`, nav item "Board") but unrecognizable as a Kanban board — structure AND style; findability/legibility failure |
| 5 | Assistant/chat: unintuitive, errors, "un-understandable feedback" | Broken behavior + presentation; needs reproduction on the seeded world during the rework |
| 6 | Task creation: attempted "I need a picture of a car" to create a task → "garbage messy output with errors"; operator expected a Nexus-style questionnaire intake | Behavior + UX; current design routes intake through chat (spec D3) — the product map must reconcile the questionnaire expectation against the spec's intake contract (carry/adapt verdict, amendment if needed) |
| 7 | Judges' feedback: can't find where to see it | Exists on deliverable/inbox/workforce surfaces; findability failure |
| 8 | The loop (findings → retry): can't find it | Findability failure |
| 9 | Planning / "nice organized structure" from Nexus: missing | Product map item — identify which Nexus planning surfaces carry |
| 10 | Buttons that don't work and are not described, so their function is unknowable | Broken interaction + labeling failure, app-wide |

## Ratified rework decisions (operator, 2026-08-05)

1. **Visual target: recreate the Nexus look** — dark violet glass control-room design language on the current React/Tailwind stack; structure and look from Nexus, adapted only where the new backend logic differs.
2. **Checkpoints: two** — operator approves the product map before code; sees rendered screenshots after shell + first surfaces; then autonomous until the final click-through.
3. **Workflow first**: the rework runs under `.claude/skills/p3-implementation/FRONTEND.md` (created this date); the four-stage pipeline remains for backend work only.
