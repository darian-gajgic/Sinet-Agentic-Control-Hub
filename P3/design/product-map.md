# Sinet product map v3 — APPROVED (the rework's contract)

**Status: APPROVED by the operator 2026-08-05 (v2), amended to v3 2026-08-06 from the checkpoint-2 verdict** (findings + root cause: `P3/design/rework-checkpoint2-findings-2026-08-06.md`; operator answers recorded in §6; v3 changes marked ▲ and listed in §8). This page is the binding product contract for the frontend rework under `FRONTEND.md`. Every verdict is grounded against the live backend (`web/src/api.ts`, `internal/api`, `internal/project`, `internal/intake`) and the spec's behavior sections. Visual direction stays as ratified: recreate the Nexus violet-glass look from the real source, reference-over-prose.

## 1. The jobs this app does for you

1. **Give it work** — "Describe a goal" in plain words; it asks its questions like a form; you trim/approve its plan before it spends anything.
2. **See what's happening** — one glance: what's running, what's stuck waiting on you, what it costs, who's burning what, how the machine itself is doing.
3. **Work by project** — tasks live in projects; a new task in a project builds on the project's prior work (§5); pick a project and the app shows that project's world.
4. **Decide things** — everything that needs a human sits in one inbox; answering resumes the work in place.
5. **Judge the work** — see what came back and what the judges said; **run the program it built**; send it back with comments; accept when right.
6. **Make it smarter** — wins and lessons it reads on future work; memory; specialists and their measured quality.
7. **Ask about anything** — the assistant + a real history/observability instrument.

## 2. Navigation (plain names, Nexus-style grouped sidebar)

| Group | Entry | Route | Status |
|---|---|---|---|
| **Work** | Home | `/` | exists |
| | Projects | `/projects` | **NEW route** |
| | Board | `/board` | exists |
| | Inbox (glowing pending badge) | `/inbox` | exists |
| **Results** | Reviews | `/reviews` | **NEW route** |
| | Wins & Lessons | `/lessons` | **NEW route** |
| **Intelligence** | Assistant (pinned, Nexus-style) | `/chat` | exists |
| | History | `/history` | **NEW route** |
| | Memory | `/memory` | exists |
| **System** | Specialists | `/workforce` (title change only — URLs never rename) | exists |
| | Fleet | `/fleet` | exists |
| | Health & evals (issues badge) | `/health` | **NEW route** |
| | Settings | `/settings` | exists |
| | Manual | `/manual` | **NEW route** |

▲ **v3: "Describe a goal" is a BUTTON, never a nav tab** (operator, checkpoint 2). The give-work door is reached by buttons on Projects (into that project), Home, and Board; the `/new` route stays as the door's deep-linkable address only. The sidebar has **14 entries**.

**Topbar: the global project selector** — the Nexus focus-context chips, carried properly. Pick a project (or "(no project)" — a real served bucket) and every surface whose data carries a project dimension scopes to it: Home, Board, Reviews, History, task lists. Surfaces without that dimension (Fleet, Settings, Health) say so instead of pretending. **Sidebar footer: CPU · MEM · GPU · GPU-MEM micro-meters render as honest "not wired yet" placeholders at v0** — the wiring is operator-deferred (§6).

All published deep-link routes stay (`/tasks/:id`, `/inbox/:id`, `/deliverables/:id`, `/memory/:id`, `/login`).

## 3. Surfaces — what you see / what you can do

**Home.** *See:* hero, four stat tiles (running · waiting-on-you · parked-until · spend today), the live roster, per-person burn, a slim recent-activity feed. *Do:* drill anywhere; personal filters (what-needs-me · mine · running · finished-today). The deep history instrument lives in History; Home stays a glance.

**Projects.** *See:* every project as a card — active tasks by stage, waiting-on-you count, spend, recent deliverables, follow-up lineage; "(no project)" as its own bucket. *Do:* open one → the global selector scopes the app to it; **"Describe a goal" straight into this project** (a button — the task is pinned to it via the landed P3-RW-1 door and plans on its prior work, §5); ▲ v3: **create/onboard a project here** (the S13.7 register→clone→scan→draft→owner-approval flow; HTTP doors served by packet P3-RW-2 — no project list/create endpoint existed before it); jump to its board/reviews/history pre-filtered. *v0 limits:* no move-task-between-projects (operator-deferred, §6).

**Describe a goal** (▲ v3: the give-work DOOR, opened by button — never a tab; `/new` is its deep-link address only). *See:* one plain ask box (literally the Nexus phrase), then the interview as a **form, not a chat**: cards of up to 4 questions, 2–4 labeled options + free text each; a live Clearance meter; the stakes/size guess. **Opened from a project, the door says whose world the task will build on, and the interview visibly skips what the project record already answers.** *Do:* answer or force-proceed (open questions become listed assumptions); then the plan card — what I understood · what you'll get · numbered steps · **what I will NOT do** · assumptions front-and-center · cost/time — **Approve / Re-plan / Re-interview / Cancel**. Nexus's "untick optional stages" carries as Re-plan's structured entry: tap the step you're contesting. Trivial read-only tasks skip ceremony. *(No spec amendment anywhere here: the interview is specified as a questionnaire (S06.5) and `POST /api/intake/requests` is already served; the rejected UI never called it. Chat stays the secondary door, same interview inline.)*

**Board.** *See:* a **recognizable Kanban** — ▲ v3 (operator, checkpoint 2): real BOUNDED columns **Backlog · Executing · Verifying · Needs attention · Done** (display labels; producers untouched — "Backlog" renders the stored `intake` status) with glowing-dot headers + count pills, horizontally scrolling, NEVER wrapping at zoom/narrow; **no Cancelled column** — a cancelled task renders in Backlog bearing a clear "cancelled" sign with why (details on the card/overlay); **Done stays visible** (operator wants to see what is finished); cards = what/whose/stage/effort/cost-so-far/waiting-on-human + priority rail, and ▲ **expand to show the task's plan steps / stage progress as sub-items with live status** (decision D-A: the honest v0 reading of "subtasks on the Kanban" — plan steps are not separate task objects); filters limited to what is real at v0 (person, project, effort class, waiting-on-you); grouped by project when unscoped. *Do:* drag to reorder **your own queued** tasks (priority hint — columns are the machine's state, never writable by drag); per-card cancel; click through (▲ opens the overlay card, not a page swap).

**Task detail** (▲ v3: a **structured overlay card** over the surface you came from — Nexus-style window, never a full-page swap; `/tasks/:id` stays as its deep-link address and renders the card standalone). *See:* the confirmed spec with numbered acceptance criteria, the plan as numbered steps each with "done when", live per-stage progress, every human decision, every revision, the receipt (ceremony vs execution, honest labels), lineage (project + follow-ups), the deliverables it produced with a door into their review/compare. ▲ Raw internal errors are NEVER rendered as body text — absent data gets a plain-words honest-absence line; durations render human-readable, never raw seconds. *Do:* state-computed action bar — only what this state allows (cancel · follow-up · answer the open card · jump to review).

**Inbox.** *See:* risk-ranked cards — what's being approved · mono provenance line · plain "what to check first" · jump to the thing · expiry countdown; stale plans auto-flag. *Do:* Approve / Deny / Answer / Re-plan; Low tier batchable; High tier re-prompts PIN; answering resumes work in place. Blind-pair benchmark verdict forms arrive here too.

**Reviews.** *See:* deliverables as numbered immutable revisions; diffs side-by-side/inline with line-anchored comments; **the judge's verdict + numbered findings per round**; round-over-round "what changed since I sent it back" as the default rework view; images as a proper visual compare (Side by side · Swipe · Overlay — overlay aligns bitmaps only, never text); binaries as hash + download. *Do:* **Run it** — for a program/site deliverable one click starts it live in the before/after preview (the served dual-iframe try-it stack); **every previous revision navigable, previewable, downloadable**; comment at the line; request a bounded revision (that IS the loop); accept in one action; spawn a follow-up task.

**Wins & Lessons.** *See:* the Nexus feedback ledger on Sinet memory: WINS (measured successes) and LESSONS (flop + its correction) as browsable shelves — real reads over memory entries (`kind`, scope person/project/house, provenance). *Do:* log a win/lesson from a task or deliverable (pre-linked); browse by project/person; retire. **These feed future work today** — the S09 influence machinery injects matching entries into runs at v0. *v0 limit:* auto-distilled lessons from rejections arrive with the v1 proposal pipeline; logging is manual, reading is automatic.

**Assistant.** *See:* persistent sessions, streamed turns, file sidebar (drag-drop up, produced-files chips down), the same intake questionnaire inline when you hand it work. *Do:* ask about the platform and its history (open-SQL answers visibly carry their lower-confidence flag); hand over work; exchange files; stop a turn; navigation never kills a running turn. Finding 5's broken plain-ask is reproduced on the seeded world and fixed inside this journey.

**History.** *See:* the observability instrument, findable at last: the filterable event stream (project/person/task/date), the served query registry (views → canned queries → open SQL), answers with audit attached; refusals and disambiguations rendered as honest non-answers; marked not-live (a query instrument, not a feed). *Do:* ask, search, drill from any answer to the task/run it names.

**Memory.** *See:* every entry scoped person/project/house with provenance, gate status, verification state; conflicts surfaced. *Do:* browse/read/create/new-version/retire/delete-own; resolve conflicts; notes live here as plain entries; promotion to house via operator approval.

**Specialists** (the workforce map, plain-named). *See:* every worker: equipment (tools, knowledge, permissions, helpers), how multi-stage procedures connect, and **measured quality/cost per version**. *Do:* view-only at v0 (editing parked by spec — said on the surface, no dead controls). *v0 limit:* per-specialist learned-lesson overlays exist in the schema but stay dormant until v1 writers arrive.

**Fleet.** *See:* who burns what — per-person/per-lane meters, donut gauges, budget bars with hot state, burn rates, limit events with "parked until…"; accounts always distinguished. Local-seat/GPU blocks render as honest not-wired placeholders (operator-deferred, §6). *Do:* edit own budgets; pause-my-automation; filter.

**Health & evals.** *See:* alerts-first, Nexus-monitor style: **Known issues** on top (open watchdog flags, drift records, conformance failures, alarms, parked runs — each with what/why/what-to-do), then benchmark state (opt-in, blind-pair verdict history, what the numbers mean), canary status (disarmed at v0 — displayed honestly). Badge = open issue count. Host meters join when the deferred wiring lands (§6). *Do:* suppress a flag (with reason), dismiss drift, acknowledge conformance, dispose an alarm, resume a parked run, set benchmark opt-in — every verb already served.

**Settings.** *See:* every setting with bounds, plain help, per-setting audit history; the price table; push devices; **Household** — member list with add-member (served: `GET/POST /api/auth/users`), PIN posture, push enrolment. *Do:* edit values within clamps (operator edits bounds); onboard a household member end-to-end. The standing "see and change everything incl. today's constants" directive remains queued as its own amendment; the surface is built so those rows slot in.

**Manual.** *See:* the Nexus manual carried: a plain-words page per surface — what it's for, how to use it, what the words mean — plus the first-run tour launcher. Frontend-only. *Do:* start any surface's guided tour (rebuilt so the spotlight never swallows clicks).

**Login.** Tailnet + PIN, unchanged behavior, styled with the rest.

**Cross-cutting rules (bind everywhere):** every control labeled in plain words and either works or isn't rendered; disabled controls say why; connection pill always visible, stale never poses as live; empty states say what will appear; phone-complete = inbox + decisions + filters + board/fleet glance + task status + chat + push.

## 4. The full Nexus tab reconciliation (all 23 views)

| Nexus tab | Verdict | Where it lands in Sinet |
|---|---|---|
| dashboard | **CARRY** | Home (hero → tiles → gauges/roster → feed hierarchy) |
| projects (+ Describe a goal) | **CARRY/ADAPT** | Projects tab + `/new` door pinned to the project (§5) |
| workflows (pipeline stages) | **ADAPT** | The plan inside each task (numbered steps + done-when); big goals = follow-up lineage under one project — no separate pipelines object at v0 |
| kanban | **CARRY/ADAPT** | Board; drag = own-queue priority hint only, columns are FSM truth |
| deliverables | **CARRY** | Reviews (+ Run-it previews, revision history) |
| meetings | **DROP** | No backend concept; its job (watch agents work) is task detail's live progress |
| agents (live roster) | **CARRY** | Home roster + Fleet |
| decisions | **CARRY** | Inbox (one queue for everything needing a human) |
| agentic (approvals) | **CARRY** | Inbox — row anatomy carried ~1:1 (what · provenance · check-first · jump · verbs) |
| specialists | **CARRY/ADAPT** | Specialists (`/workforce`); view-only v0, learning overlays dormant until v1 |
| memory | **CARRY** | Memory (scoped entries, provenance, conflicts) |
| skills | **ADAPT** | Inside a specialist's detail: its equipment (tools/knowledge/permissions/helpers) |
| monitor (CPU/MEM/GPU/VRAM) | **DEFERRED** | Health & evals + sidebar meters render honest placeholders; wiring/amendment operator-deferred (§6) |
| tools | **ADAPT** | Specialist equipment view (same home as skills) |
| programs | **ADAPT** | Reviews' Run-it previews — a "program" is a deliverable you can run |
| guardian | **ADAPT** | Health & evals (watchdogs, canaries, drift, conformance) |
| usage | **CARRY** | Fleet + per-task receipts |
| observability | **CARRY** | History (event stream + query layers with audit) |
| issues (known issues) | **ADAPT** | Health & evals' Known-issues section + nav badge; platform-detected issues — no user-filed tracker at v0 |
| notes | **ADAPT** | Memory entries (person scope) |
| settings | **CARRY** | Settings (registry-generated, every setting + bounds + audit) |
| manual | **CARRY** | Manual tab + per-surface tours |
| jarvis (assistant) | **CARRY/ADAPT** | Assistant (persistent sessions, files, intake handoff); voice/avatar parked v1+ by spec |
| — global focus context (topbar) | **CARRY** | The global project selector (§2) |
| — sidebar CPU/MEM/GPU/GPU-MEM footer | **DEFERRED** | Placeholder at v0 (§6) |
| — wins & lessons ledger | **CARRY** | Its own nav entry on memory kinds |

Visual-pattern verdicts from map v1 all stand (glass tokens, stat tiles, approval anatomy, diff viewer, gauges, empty states, tours, toasts/drawer/modal/skeleton carried; Chart.js, hand-drawn icon set, 3D avatar/galaxy, cross-column drag dropped).

## 5. Approved requirement: a new task in a project builds on the project's prior work

Operator requirement (2026-08-05, approved with v2): *"a new task needs to be able to be generated depending on the previous work done in the project… since the specs, planning and work on a new task depend on the work done in the project."*

**The spec already mandates this and the backend already does most of it** — four built mechanisms:

1. **Registry injection (S06.2/S13.7, built):** a matched project's registry entry — conventions, commands, danger zones, task family — is injected at triage, and registry-supplied facts **resolve interview slots** so the questionnaire never asks what the project record already knows (`RegistrySlice.ResolvedSlots`, live in `internal/intake`).
2. **Planning reads the project itself (S06.6, built):** the Stage-1 planning session runs at C1 over a **read-only snapshot of the project** — the accumulated code/work of every accepted task is literally what the planner plans against.
3. **Project memory influences the work (S09, built):** project-scoped memory entries — including the Wins & Lessons shelves — are injected into matching runs; plans may cite project-truth entries, and a citation that can't resolve blocks approval.
4. **Lineage and freshness (S1.2/S06.9, built):** follow-ups carry their source deliverable into intake; a sibling task accepted in the same project auto-flags pending plan approvals "assumptions may be stale".

**The one gap — and the one backend packet this map queues (P3-RW-1):** the registry match is a deterministic *name-token* scan of the request text (`MatchForIntake`). Typing "…in the shop backend" pins the project; **the Projects-tab door must not depend on the user typing the project's name.** Fix: an **optional `project` field on the intake Submit body** (`internal/stage/surface.go` `submitBody` + `intake.Request`) — when present and the requester owns/belongs to the ACTIVE entry, it pins the registry slice directly; the text match stays for unpinned submissions. This is additive-first API evolution inside the S15.2 tasks-family contract (exact precedent: the `Inputs` field, added additively at B6-7) — **a small four-stage backend packet, no S00.9 amendment**. Go-only paths; the frontend consumes it in the give-work journey. **LANDED 2026-08-05** (`1ffa68a`+`a141e1d`+`04d56b0`, eval PASS after 1 round).

▲ **v3 — the second gap, packet P3-RW-2 (queued 2026-08-06): the projects HTTP family.** Checkpoint-2 grounding: **no `/api/projects` route exists at all** — the SPA can neither list registry entries with their detail nor start onboarding; `OnboardStart` (S13.7 register→clone→scan→draft, owner-approval ask via the existing S15.6 inbox) is a built stage seam wired in shell but served by no HTTP door. P3-RW-2 = the read door (projects visible to the caller: status, capture summary, conventions/commands/danger-zone detail per S13.7 visibility) + the create/onboard door (start onboarding → the drafted entry + its approval ask). Additive, S15.2 posture, owner/member visibility server-side; four-stage pipeline, Go-only paths. The Projects tab consumes it; task-derived aggregates (counts, spend) stay client-side over the already-served task/deliverable rows.

## 6. Operator decisions — the record (2026-08-05)

| Decision | Answer |
|---|---|
| Map v2 | **APPROVED** |
| GPU/VRAM + local-seat fleet-seam wiring | **DEFERRED** by operator ("don't do the backend work yet") — surfaces render honest placeholders |
| CPU/RAM host monitoring (S00.9 amendment) | **DEFERRED** by operator — not proposed further until asked |
| Move-task-between-projects verb | **DEFERRED** by operator — v0: project set at intake, follow-ups inherit |
| Project-context intake (§5) | **REQUIRED** — mechanisms 1–4 surface in the UI; packet P3-RW-1 queued as the enabler of the in-project door |
| Naming | Defaults stand (Specialists · Describe a goal · Health & evals) — no overrides given |

▲ **v3 operator decisions (2026-08-06, checkpoint 2 — free text, authoritative):**

| Decision | Answer |
|---|---|
| Checkpoint-2 verdict on the full-app walk | **FAILED** — 13 findings, all coordinator-verified accurate (`rework-checkpoint2-findings-2026-08-06.md`); operator later confirmed the misread of scope (only shell+Home were rebuilt), verdict on the REBUILT surfaces separately below |
| D-A: "subtasks on the Kanban" | **Option A** — board cards expand to show plan steps / stage progress as sub-items with live status; no backend packet; plan steps stay non-task objects at v0 |
| D-B: board columns | **Done stays VISIBLE** ("I want to see what is already done"); **no Cancelled column** — a cancelled task renders in Backlog with a cancelled sign + why; first column named **Backlog** |
| D-C: shell + Home direction | **CONFIRMED** ("Yes that is fine, very good, well done") — the design language continues |
| Describe-a-goal | Button/function, never a tab (adopted from the verdict verbatim) |
| Task click | Structured overlay card, never a page swap (adopted) |
| Inbox language | Plain words; "opt in/opt out" jargon gone (adopted) |

## 7. Build order (▲ v3 resequenced 2026-08-06 — journey-first milestones, process rule P3)

1. ~~Shell + Home~~ **DONE 2026-08-05, direction operator-confirmed (D-C).**
2. ▲ **The working journey (checkpoint 3):** fence-rule banners on every not-yet-reworked surface FIRST (process rule P1), then: Projects (cards + create/onboard door when P3-RW-2 lands) → Describe-a-goal button + door (interview form, working submit — kills finding 5's dead "Answer here" — plan card, approval) consuming P3-RW-1's pin → the real Kanban board per §3 (Backlog naming, bounded columns, sub-items, cancelled-in-backlog) → the task overlay card. **Checkpoint 3 = the operator completes this journey end-to-end on the seeded world**: create/see a project, describe a goal into it, answer the interview, approve the plan, watch the task on the board, open its card.
3. Decide: Inbox (plain words everywhere — the full row anatomy)
4. Judge: Reviews + Run-it previews
5. Steer/insight: Fleet, Health & evals, Wins & Lessons, Memory, History
6. Specialists, Settings (+ Household onboarding), Manual, Assistant (form/flow/styling; the conversational brain remains a bring-up item)

Backend, `web/src/api.ts`, router and data hooks stay where sound; every view rewritten as one design by the single builder; behavior contracts (review loop, honesty invariants, escape-by-default, owner-scoping) bind throughout.

## 8. v3 amendment record (2026-08-06)

Provenance: checkpoint-2 FAILED verdict + operator answers; analysis of record `P3/design/rework-checkpoint2-findings-2026-08-06.md`. Changes: Describe-a-goal tab → button (§2/§3, nav 15→14); Projects gains the create/onboard door + **packet P3-RW-2 queued** (§3/§5 — no projects HTTP family existed); Board fully specified (§3: Backlog, bounded scrolling columns, Done visible, no Cancelled column, D-A sub-items, honest filters); task detail → structured overlay card + never-raw-errors rule (§3); build order resequenced journey-first with checkpoint 3 defined (§7); process rules P1–P4 bound (fence banners, structure-as-pixels, journey-first checkpoints, seed hygiene — the seeded "moonshot" unknown-status task leaves the operator-facing demo world, golden fixtures untouched). Deferred set (§6 v2 table) unchanged.
