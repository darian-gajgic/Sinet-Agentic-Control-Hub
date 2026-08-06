# Checkpoint 2 — FAILED (operator verdict 2026-08-06) — findings ledger + root-cause analysis

**Operator verdict (free text, authoritative): "It is still bad… maybe 10 percent better than last time."** The operator walked the FULL app on the seeded world (:8483), not only the two rebuilt surfaces. Direct instruction: **analyze why the result is bad BEFORE fixing anything.** The builder HOLDS. This file is the analysis of record; STATE points here.

## 1. What the operator actually walked through

At checkpoint 2 exactly **2 of 15 surfaces were rebuilt** (shell + Home; map §7 step 1 of 7). Of the 15 tabs the operator clicked: 2 rebuilt, 7 honest-but-empty placeholders (steps 2–7), ~6 surfaces still the REJECTED UI batch (board, inbox, task detail, chat/assistant, workforce, memory/fleet/settings), plus the documented-broken plain-ask flow (rework finding 5). Nothing in the app marked which was which. The operator judged the product — the only way a real user ever judges an app.

## 2. Findings ledger (operator verbatim → coordinator verification, live on :8483)

| # | Operator finding | Verified | Class |
|---|---|---|---|
| C2-1 | Projects tab: can't create a project, no projects listed with details — "what is the point?" | ✓ placeholder page only | Placeholder-at-checkpoint + **MAP GAP: no create-project door in map §3 at all** |
| C2-2 | Describe a goal must NOT be a tab — it's a button/function (from a project) | ✓ nav entry per approved map §2 | **MAP DEFECT** — prose approval encoded the wrong shape; operator override supersedes map |
| C2-3 | Wants project → task → subtasks structure, subtasks visible on the Kanban ("Nexus workflows = tasks, tasks = subtasks") | model mismatch confirmed | **PRODUCT-MODEL GAP** — map v2.1 "no separate pipelines object" was approved as prose but does not carry the operator's real expectation: watching the decomposition |
| C2-4 | Board is not a recognizable Kanban: no real columns/structure/filters/priorities/tags, cards on empty background, wraps at zoom | ✓ screenshot: text-label headers, no column containers, label-value data dumps as cards, CSS grid wraps | Old surface (step 3 scope); map §3 already promises exactly what the operator asks |
| C2-5 | First column "intake" should be "backlog" | ✓ `kanban.ts` label "Intake" (display-only, producers untouched) | Trivial label fix, step 3 |
| C2-6 | "Cancelled" and "moonshot" sections confusing — "we have to talk about this" | ✓ Cancelled = declared column, always rendered even empty; **"moonshot" = a seeded-world task with a deliberately unknown status exercising the honest-unknown-column rule — a test artifact shown to a real user** | Board design (step 3) + seed hygiene |
| C2-7 | Inbox: "opt in / opt out" labels unclear; "no idea what this tab is supposed to do… what decisions I have to make" | ✓ old surface; benchmark verdict cards in jargon | Old surface (step 4 scope); validates map's plain-words row anatomy |
| C2-8 | Reviews/Deliverables tab: "just some stupid text, rest is empty" | ✓ placeholder | Placeholder-at-checkpoint (step 5) |
| C2-9 | Specialists: text breaks a line per letter — "same error as the previous version, how can this happen?" | ✓ screenshot: "Confinement C/0", "Ceilings n" in a collapsed-width container | Old REJECTED surface untouched (step 7 scope); the rework never re-screenshot it — the checkpoint let the operator re-find a known-rejected defect |
| C2-10 | Assistant: no styling — "radio buttons, text on a blank screen, 3 borders… beginner work" | ✓ screenshot: naked file input, bare Rename/Delete links, broken radio row with colliding labels | Old surface (step 7 scope) |
| C2-11 | Interview: filled the boxes, "no button to go further, not working" | ✓ live: task t-2aa79cba8f813bb1 WAS born; the "Answer here" button renders greyed-out; mode-picker radios collide | **Finding 5 cluster, already on the books (step 2 scope)** — operator re-hit a documented defect cold |
| C2-12 | Assistant has no AI behind it — wants Jarvis: answer questions about Sinet, control it by text, give briefs | partially structural | Layers 0/1 (canned views + catalog queries) exist; free conversation/briefing needs engines wired = **bring-up (deferred D2 sweep + local tier)**; styling/flow fixable now, the brain arrives at bring-up |
| C2-13 | Task click: wants a Nexus-style structured window card (details, deliverables, side-by-side), not a full-page of "unorganized messy text" | ✓ screenshot: wall-of-text page, RAW internal error string rendered ("not_found: intake: task has no intake state"), "1500991 s elapsed" raw seconds | Old surface (step 3 scope) + **presentation decision: overlay card over context** (operator words) |

## 3. Root causes, ranked

1. **The checkpoint exposed the whole app when 13% of it was rebuilt, with no in-app boundary.** Old rejected surfaces sat unlabeled beside the new shell; the coordinator's own presentation invited a free click-through ("click around the real thing"). An operator will always click every tab — the process had no rule making not-yet-reworked surfaces visibly declare themselves. Known defects (finding 5, the Specialists letter-wrap) were reachable with no warning, so documented issues were re-discovered as fresh failures.
2. **Checkpoint 1 validated prose, not pixels — approval of the map did not mean what STATE recorded it to mean.** "Describe a goal | /new | NEW route" read fine in a table and is nonsense to the operator as a rendered tab. "Workflows → the plan inside each task" compressed away the thing the operator actually wants: watching the task decompose into subtasks on the board. The reference-over-prose lesson (FRONTEND.md rule 2) was applied to visual design but NOT to product structure. Structure needs rendered variants/wireframes for approval, same as looks.
3. **Step 1 delivered chrome, not a journey.** Shell + Home let the operator DO nothing new — every job they care about dead-ends into a placeholder or an old surface. The first visible milestone of a rework that exists because "mechanically green, humanly unusable" was itself unusable-by-design. Journey slicing should have led with a doable journey on a minimal shell.
4. **Map coverage gaps existed even on its own terms**: no create-project door anywhere in §3 (backend has S13.7 onboarding; which HTTP door exists needs grounding); task-detail presentation form never specified; board filter vocabulary never reconciled against what the backend can honestly filter (priority = own-queue hint rank; effort classes; no tags at v0).
5. **What did NOT recur (evidence, not defense):** on the two surfaces actually rebuilt, the new workflow behaved — the builder's live loop caught five real defects pre-checkpoint, and the operator's message raises no finding against the shell/Home visuals themselves ("You inserted a project tab. Good, but…"). The signal is thin — checkpoint asks the operator to confirm or deny the direction explicitly — but the failure this time is checkpoint design + map fidelity + sequencing, not (so far) the single-author screenshot loop.

## 4. Process corrections (bind from now)

- **P1 — Fence rule:** at every operator touchpoint on a live build, every not-yet-reworked surface carries a visible in-app banner ("old version — rebuild scheduled: <step>"); known defects are listed to the operator UP FRONT at the touchpoint.
- **P2 — Structure-as-pixels:** any navigation/product-structure decision gets rendered variants (cheap wireframes or live toggles) for operator pick BEFORE building — prose tables never again stand in for structure approval.
- **P3 — Journey-first milestones:** every checkpoint demonstrates something the operator can DO end-to-end on the seeded world. Next checkpoint = the working journey: create/see projects → describe a goal from a project (button, not tab) → interview → plan → the task live on a real Kanban → structured task card.
- **P4 — Seed hygiene:** the seeded world must not surface test artifacts (the "moonshot" unknown-status task) in an operator walk; artifact rows are dev-only or clearly labeled.

## 5. Operator decisions needed before the builder resumes (free text)

- **D-A (hierarchy):** Sinet's backend today: a task carries a numbered plan (steps with "done when") + per-stage progress + follow-up tasks under the same project. Proposal: board cards expand to show plan-step/stage progress as sub-items (honest today, no backend change); literal first-class subtask cards would need a backend packet (grounded first). Is the expandable-sub-items reading of "subtasks on the Kanban" acceptable at v0, or ground the backend packet?
- **D-B (board verbs):** proposed: first column labeled "Backlog"; columns as real bounded columns with headers/counts, horizontal scroll (never wrap); Cancelled (and optionally Done) behind a "show finished" toggle; filters limited to what is real at v0 (person, project, effort class, waiting-on-you, own-queue priority rank). Veto/amend freely.
- **D-C (shell + Home):** are the rebuilt sidebar/topbar/Home themselves the right direction (their look raised no finding in the verdict), or also wrong? Decides continue vs reset for the design language.
- **Adopted without asking (operator words were explicit):** Describe-a-goal becomes a button (Projects/Home/Board), not a tab; task click opens a structured overlay card (deep-link URL preserved); inbox rows in plain words; "backlog" naming.

## 6. Standing scope facts restated (so expectations stay honest)

- The assistant's conversational brain (Jarvis behavior) arrives at **bring-up** (engines + local tier + D2 paid sweep — currently operator-deferred). The rework fixes its form, flow, styling, and the broken submit; it cannot give the assistant a model before bring-up.
- GPU/VRAM + CPU/RAM meters stay honest placeholders per the operator's own deferral (map §6).
