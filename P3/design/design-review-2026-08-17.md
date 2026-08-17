# Live design review — 2026-08-17 (fresh eyes, real Chrome, post-RW-15 tree)

Reviewer: fresh-context agent per FRONTEND.md rule 5. Method: fresh throwaway world (journey walked to the PIN step-up, $0 — never approved) + the builder's kept world `~/.sinet-b6-final` read-only for completed states; desktop ~1440w; narrow ~616w via a same-origin iframe harness (Wayland refused programmatic resize — layout math is width-driven, will reproduce in a real 616px window). Production untouched.

**Grades:** Design Quality **A−** (violet-glass control-room genuinely recreated; narrow topbar + text walls keep it from A) · Originality **B+** (provenance chips, park/resume rail, UNPRICED tables are ownable; template boilerplate on the most important card is the least original writing) · Craft **B−** (excellent state copy/empty states/disabled-reasons, but visible seams ship) · Functionality **B+** (whole journey runs live with correct gating and zero stalls; review-diff and raw-id naming are the blemishes).

**Verdict: SHIP-BAR NOT MET** — blockers #15, #4, #3, #19, #1.

## Findings + coordinator triage (D = builder drain round 1; O = observation/deferred; B = backend investigation)

| # | Finding (surface — essence) | Sev | Triage |
|---|---|---|---|
| 1 | Review surface default diff tells a false story: "pre-task base vs rev 1" renders the repo's Makefile as deleted and the shipped text under `deliverable.md`, contradicting the task's own AC/plan (fileset category error). Fenced surface, but the walkthrough routes the operator here. | HIGH | **D-BLOCKER** — minimal honest cut now (diff basis scoped/labeled truthfully); full rebuild stays step-4. B if the fileset comes wrong off the wire. |
| 2 | One judge comment rendered ×4 (twice in list, twice in "without a place") | med-high | **D** — root-cause data-vs-render first; if wire, report B. |
| 3 | Door-born tasks wear raw `t-…` ids as titles on board + overlay; "names itself otherwise" promise unmet (pairs with RW-14 OQ6 empty tasks.title note) | med-high | **D-BLOCKER** — capture/derive a human title at the door (SPA-side; if the door POST lacks a title field, report B). |
| 4 | PIN step-up renders off-screen after "Approve — start the work" (viewport snaps to card top; field + notice two screenfuls down; no disarm affordance) | med-high | **D-BLOCKER** — scroll-to + visible arming + a "never mind" disarm. |
| 5 | Zero-touch classifier misfiled a deletion/rebuild goal as `chore`, shaping routine-flavored questions; planner self-corrected | med | **O** — model-quality; intake-triage is honestly `calibrated=false`; the landed workhorse re-check activates at bring-up calibration. Recorded. |
| 6 | Phrased vs canonical-fallback questions indistinguishable; fallback unadmitted | med | **D** (recommended) — a small honesty marker when the phraser fell back. |
| 7 | Stale wait-face copy: post-answers waits revert to the birth copy ("The task is born…") on two paths where the correct face exists | med | **D**. |
| 8 | Failed sign-in silent; account-switch clears typed PIN | med | **D**. |
| 9 | Cost one-liner "USD 0 · … API-equivalent figure" contradicts UNPRICED receipts below | med | **D** — truthful wording (priced-total ≠ API-equivalent). |
| 10 | Cancelled task's "why" never in plain words (rail jargon only; HUMAN DECISIONS claims "nobody has decided anything" on an operator-cancelled task) | med-high | **D** — plain-words why + the decision recorded in the decisions block (B if the wire lacks the cancel reason). |
| 11 | Raw Go error chain + absolute host path as CHECK-INTEGRITY card body | med | **D** — humanize (the greyed-verbs pattern already nearby is the model). |
| 12 | Board cost chip bare "USD 0" on UNPRICED-spend tasks | low-med | **D** (cheap, rides #9). |
| 13 | "Answer its open card" verb on a DONE task reads stale/unspecific | low | **O**. |
| 14 | Live drift/watch feeds land whole in the operator queue (Health 5→17 in 90 min; "routine synchronization" as HIGH sign-off cards) | low/obs | **O** — queue-noise triage design, pairs with the deferred drift-flood ledger item (G-2026-08-16-d). |
| 15 | Plan-card assumed-slot boilerplate walls: label rendered twice, 9–13 near-identical "assumed a sensible default" rows that never state the default, the same rows duplicated again under ASSUMPTIONS, raw slot ids leak | med-high | **D-BLOCKER** — dedupe label, collapse the wall, render the ACTUAL assumed value from resolutions; raw ids never surface. B if the assumed value is not on the wire. |
| 16 | Answered open-markers missing from "what was settled" with their chips | low-med | **D** (recommended). |
| 17 | Jargon leaks on rebuilt surfaces: "(ears)"/"(gwt)", spec refs as body text, `intake.state` as rail labels, per-run repeated footnote | low-med | **D** (sweep). |
| 18 | Clearance meter bare number, no unit/help; reads 100 while approval still blocked on markers | low-med | **D** (recommended). |
| 19 | Narrow (~616px) topbar shatters into ~230px of sticky stacked rows on every surface — phone-complete violated on inbox/board/glance | med-high | **D-BLOCKER**. |
| 20 | Narrow header drops stakes chip/family crumb/clearance exactly on the phone | low-med | **D** (rides #19). |
| 21 | Narrow otherwise holds (board bounded, overlay full-screen sheet, forms clean) | + | record |
| 22 | Microtext AA failure: 3.2:1 at 11px on provenance/footnotes; 10px chips | med | **D**. |
| 23 | Review diff near-illegible (faded content, ghost line numbers) | med-high | **D** (rides #1's minimal cut). |
| 24 | Focus rings inconsistent (custom violet vs UA white) — both visible | low | **O**. |
| 25 | Positives on record: honest ticking waits, open-marker gate refusing an under-specified deletion, step-up pre-announcement, UNPRICED-never-silent, park/resume causes, fence banners, disabled-reasons, owner-scope language, deep-link frames, keyboard order + Escape/focus discipline | + | record |

Not covered by the review (for the cold walks / later passes): drag-to-reorder, the tour, Assistant/chat, verify-card Retry path, accept/follow-up/preview verbs, Settings/History/Memory/Specialists interiors.
