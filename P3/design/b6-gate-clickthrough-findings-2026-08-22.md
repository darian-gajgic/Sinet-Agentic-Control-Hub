# Operator findings — B6 gate click-through, first sitting (2026-08-22)

The operator began the scripted click-through (`P3/gates/B6-walkthrough.html`, world `~/.sinet-b6-clickthrough` on :8483) and **stopped at the first wall: the Inbox**. Free-text feedback is authoritative per the standing gate rule. The gate remains OPEN; the sitting is PAUSED pending the rework below, then resumes on the same world.

## The wall, in the operator's terms

Created a project, then had to answer questions in the Inbox — took ~5 minutes to even find the right card. "This tab is still a mess." No way to filter, no way to sort; the inbox ignores the selected project. Cards lead with useless internals (row ID, change class, fingerprint, latest seq, seq) with the useful facts (which project, which task) buried in between. The headline is "sign-off" or "question" — meaningless for deciding anything. Everything is displayed in one big card all at once. Verdict: **rework ordered** — filterable by project and task, organized presentation, progressive disclosure.

## Findings (with code triage)

| # | Finding | Triage |
|---|---|---|
| 1 | No filtering or sorting; the inbox does not follow project selection; finding the card that needed answering took ~5 min | Deliberate landed posture ("the server ranks, the client does not — never a filter", `web/src/Inbox.tsx:38-41`, header copy :83, W2-10 lineage) — **overruled by the operator at the gate.** Client-side project/task filtering is presentation-only and sanctioned (S15.12); default order stays the served order |
| 2 | Raw internals on the card face: row ID, change class, fingerprint, latest seq, seq | Confirmed — `RowCard` (`Inbox.tsx:862-878`) dumps every raw projection-row key as a definition list for the watchdog/drift/conformance/alarm kinds. Forward-tolerant for producers, hostile to humans. Internals demote to an expanded "technical details" fold; never the face |
| 3 | Headline is "sign-off" / "question" — "what do I want with that" | Confirmed — `displayClass` (`Inbox.tsx:552-569`) is the card face's lead. The face must lead with the issue in plain words plus project + task; the class label demotes to a chip |
| 4 | Which project a card belongs to is invisible / buried; task ditto | Wire serves `task_id` (api.ts:536, absent honestly) but **no project**; project is joinable client-side from the served tasks read (presentation-only join). Cards with no run/task get the honest "(no project)" bucket (precedent api.ts:156) |
| 5 | Everything renders at once — one big card with the full body inline | Confirmed — `CardBody` renders fully expanded in the list. Rework: compact rows (project · task · one-line issue), press a card to open the full detail |

## The ordered rework (operator words → requirements)

1. **Filter by project and by task**; the inbox honors a selected project (deep-linkable, e.g. from a project page); a sort/order affordance.
2. **Compact main view**: each card = what project, what task, a short plain-words description of the issue. Nothing else on the face.
3. **Press a card → the full detail**: what the issue is, what has to be decided, what information that decision needs, what the platform recommends — then the verbs.
4. **Raw internals out of sight**: row id / change class / fingerprint / seq live only inside an expanded technical fold.

## What stays binding (unchanged)

Served order is the default order; a filter is presentation and must say what it is hiding (every served card reachable — nothing silently dropped); the card is the authority (verbs only from served `actions`, answerability reasons rendered); tier mechanics, batch, PIN step-up, payload-hash answers untouched; escape-by-default (card bodies are model-derived input); honest absence everywhere; no new wire computation — the project comes from a served read, never a guess. Backend untouched — this is a FRONTEND.md builder round. If the builder proves a needed fact genuinely unserved (e.g. the tasks-read join breaks under pagination bounds), it STOPS and flags; the backend micro then runs the four-stage pipeline separately.
