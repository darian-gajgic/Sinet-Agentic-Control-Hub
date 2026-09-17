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

---

# Sinet product map v4 — the code-review journey (DRAFT for operator checkpoint 1, 2026-09-17)

**Status: DRAFT, presented for checkpoint 1 under `FRONTEND.md` rule 3 (the map before any component). v3 above is untouched and stays binding for everything it covers; v4 adds one journey.** Packet: P3-SIT-2 (frontend half of "the code has to be in the deliverables"). Grounding: the landed backend contract `web/src/api.ts` + CONVENTIONS §78 (P3-SIT-1), §73 (the receipt's judge line), §76 (`parent_run_id` + `ending`), the six regenerated goldens, and the LIVE served answers of the real webshop deliverable `dlv-t-3120e8e3d14591d3` in a copy of the sitting world (`~/.sinet-sit2-builder`, :8489). Spec wins on behavior: S13.1–S13.3, S13.8, S15.5, S15.8, FC-v1 §2, S02.5, S07.11/S10.10. Reference for the look: the Nexus review v2 source (`app/static/style.css` "Result review" block + `app.js` `renderReviewModal`/`renderReviewFilePane`) on the ratified violet-glass language; it is not running on :8790 and has no venv installed, so the source is the reference.

## 9. The jobs, from the reviewer's chair

You asked for a webshop. A day later the platform says the work is finished and waiting for you. You want to:

1. **See what was built.** The code, all of it: which files exist, what kind of change each one is, how big it is, and the full text of any file you open. Not the worker's essay about the code: the code.
2. **See what changed since last time.** Version 2 against version 1 by default; the first version against the project as it stood before the task; any two versions when you ask.
3. **Try it.** Run the thing. When the platform cannot do that yet, it must say so in words that make clear the work is fine, the gap is the platform's, and what you can do instead today.
4. **Decide.** Accept it, or send it back with a reason, without hunting for the button. And know what the checker found before you decide.

Two riders from this week's backend packets ride the same journey because the same person reads them: **why a run stopped and who carried on** (a crash followed by a successor must read as one story on the task page), and **who checked the work** (the receipt's judge line).

## 10. Navigation, in plain names

| From | Click | You land on |
|---|---|---|
| Sidebar → **Reviews** (`/reviews`) | a row | the work's **review page** (`/deliverables/:id`) |
| Home → "What needs me" | a review-ready row | the review page |
| Board → a task card → the task's card | **Review this work** (its Deliverables section) | the review page |
| the review page | **Back to the task** | the task's card, where the run story and receipts live |
| a push notification about work waiting | its link | the review page |

`/reviews` is a placeholder room today ("build step 4"); this packet fills it with the **index**: every piece of work you may open, grouped *Waiting for you* · *Waiting for someone else* · *Accepted* · *Superseded*, each row = the task's title · what kind of work · version N · whose · when it last moved · **Open review**. It is one small surface over two reads the SPA already makes (`GET /api/deliverables` + `GET /api/tasks` for the titles) and it is the answer to "no idea where I have to click" at the root. **Operator call (Q1 below): include it in this packet, or leave the placeholder.**

## 11. Surfaces — what you see / what you can do

### 11.1 The review page for code (the page that showed only the report)

The page is **one screen with a fixed reading order**, the Nexus review anatomy carried onto a full page (not a modal): a decision strip on top, the file list as the table of contents on the left, the reading pane on the right; on a phone the file list folds into a picker above the pane.

**Top: the decision strip.** *See:* the task's title ("Create a webshop for car replacement parts and tuning parts"), whose work it is, **version N of M**, the state in one sentence ("This finished work is waiting for you. Nobody else reviews it."), and the checker's one-line verdict for this round taken from the served facts (the verification posture banner when the round ran in bootstrap mode, and how the checking round ended, verbatim from the verify run). *Do:* **Accept this work…** (opens the accept card, unchanged), **Ask for changes** (jumps to the comment composer; see 11.1 "What the checker found / your comments" for what a comment does at v0), **Start a follow-up task**, **Try it** (jumps to the try-it section). Every button is a served door; a closed door renders as its reason, never as a dead control.

**Left: the files — the table of contents.** *See:* every file of the served inventory (`change.files`, never truncated): path, the kind as a coloured mark and a word (**new** · **changed** · **deleted** · **renamed from …**), size, `+added −removed` line counts, a **binary** mark where git says so, and a comment badge where a comment or finding anchors in that file. A totals line ("23 files, all new · 42 KB · +1,301 lines"). Base-side comparisons say so in the list's head: "everything below is new: version 1 is compared with the project before the task". *Do:* click a file to read it in the pane; the list scrolls independently on desktop; on a phone it is a select.

**Right: the reading pane.** *See:* the selected file's name, kind and sizes, then one of two views with a toggle: **Changes** (the widget's diff of this file only, side-by-side on desktop, inline on a phone, with word-level edit marks) or **Whole file** (the file at this version, read through the new `files` route, as escaped text with line numbers). Defaults by kind: a **new** file opens as **Whole file** (a wall of green is not how a person reads a new file), a **changed** file opens as **Changes**, a **deleted** file shows the deletion diff and its **Whole file** reads the older version's text where that version is a numbered revision (the pre-task base is not a revision the files route serves; the diff already shows every line), a **binary** file shows its sizes and "no text to show; the bytes are in the record below". Every cut is a sentence: a per-file diff cut at a hunk boundary says so with the served reason and points to Whole file; a whole-file cut at a line boundary says how much of the file is shown and why, verbatim from the wire. *Do:* click a gutter line to comment on it (the anchored-comment loop, unchanged behaviour); switch views; open the next/previous file.

**Below the pane: what changed since last time.** *See:* the version strip (v1 · v2 · … newest selected) and "compared with: **the previous version**" (the platform's own default, the client sends no bounds) with a picker: the previous version · the project before the task · any specific version. The label of the pair always comes from the served answer. *Do:* pick a pair; the file list and the pane follow the new pair's inventory.

**What the worker says it did.** *See:* the step report (`deliverable.md`), rendered as it is today (escape-first markdown in the sandboxed document frame), **demoted**: a collapsed panel under a plain heading with the lead sentence "This is the worker's own account of the work. It is a claim, not a check: the files above are the fact and the checker's findings are below." Open by default only when the deliverable is not repo-backed (then the document IS the work, exactly as today). *Do:* expand/collapse, download the exact bytes.

**What the checker found, and your comments.** *See:* the verification posture banner (bootstrap: "your review is what decides here", with the Commands door), then every finding and comment of this version in one list under the same schema as today, each with its category as served (sanity-blocker · CHECK-INTEGRITY · RESEARCH-NOT-RUN …), its severity, its placement status, and a **jump** into the file where it anchors; findings without a live anchor stay on the always-visible strip. *Do:* comment on the whole work or on a line. Honest at v0: a comment is what the next round works from; the "request a revision" door opens only when the platform asks (a rework card) and is otherwise rendered closed with its served reason; a finished (accepted) work's "ask for changes" starts a follow-up, as the served door says.

**Try it.** *See:* the honest state, in the operator's words (11.4). *Do:* Launch (answers with the platform's served disposition, rendered verbatim), or follow the "what to do instead" line.

**Accept.** Unchanged card and flow, reached from the strip; a closed accept door leads with its reason.

**The record (folded).** Every revision with its pin, minting run, verdict reference and time; lineage; every door with its technical detail; downloads. This is the today's "Revisions" + "What you can do" content, moved out of the reading path.

### 11.2 The task's card — the run story (TQ-F8)

*See:* the stage rail grouped **per attempt**: a header row per run ("Attempt 1 · `t-….execute` · stopped at step S-4: «the served ending, verbatim»"), the steps that attempt drove beneath it, and, where a run carries `parent_run_id`, a joining line into the next header ("→ Attempt 2 · `t-….execute.g1` picked the work up here; its receipt carries what it used, nothing from attempt 1 counts twice") and the successor's own ending ("finished: execution complete: deliverable produced"). On a fresh crash the served ending already says whose fault it was ("Step S-4: the platform could not run the work session for this step. Steps finished earlier stay finished; this attempt stops here." or its "the work session stopped before the step was finished" twin); on the sitting's older world it reads "stage dispatch failed" and renders exactly that. The run-standing nodes at the rail's end fold into these headers so the two sequences stop reading as unconnected. *Do:* nothing new; the cancel and inbox doors stay where they are.

### 11.3 The receipt's judge line (§73)

*See:* on every receipt that carries `judge`: **Checked by** «model» followed by the served note verbatim ("both done by the same family of models (X did the checking), so this check is less independent than usual" / "a different model family from the one that did the work"), with a yellow **same model family** chip when `self_family` is true. It sits directly under the totals line and above Parks, because it answers the reader's next question after "what did this cost": "who checked it". A verification receipt with no judge line says "no judge line is recorded on this receipt" (older receipts); other receipts say nothing, since most runs never reach a verdict.

### 11.4 The preview absence, in the operator's words (SIT-F3)

Today the section offers "Launch a preview" and only after the click answers, in spec dialect, that nothing can be served. Proposed copy, rendered before any click:

> **Try it live: not available on this platform yet.** The work is complete and can be run; what is missing is the platform's own live-preview feature (the host part that would let a preview be reached, and the code that starts and routes one, are not built yet). That is a gap in the platform, not a fault in this work, and it does not block accepting.
> **To try the app now:** on the host, run `P3/gates/try-deliverable.sh dlv-t-3120e8e3d14591d3`. It checks this exact version out beside the platform (read-only, the platform is not touched), installs its packages, runs its tests, starts its dev server and prints the address to open.

The first paragraph is a v0 build fact written into the SPA and dated, removed by P3-SIT-3 when the substrate lands; **Launch** stays and renders the served disposition verbatim beneath ("a dev-server preview can be prepared, but it cannot be served live yet: …"). The script line names the deliverable id the page is showing. **Operator call (Q2): keep the script line on the page for every household member, or show it to the operator only.**

## 12. The revision-navigation rule

- The **default pair is the platform's**: the client sends no bounds; the answer says which two it compared (existing behaviour test, kept). That is N vs N−1, and for version 1 the pre-task base (`old_n = 0`, `old_is_base = true`).
- The **first version's only comparison is the base**, said in plain words ("version 1 is compared with the project before the task, so everything in it is new").
- **Any pair on demand**: the picker offers the previous version, the base, and every numbered version; the inventory and the pane always follow the served `change` of the pair shown.
- **Whole-file reads** are keyed on the pair's newer side (`files?revision=<new_n>`); a deleted file's whole-file read uses the older side when it is a numbered revision.
- The **inventory is the authority for the kind** of every file (gitdiff-parser types everything "modify"); the widget renders with the inventory's kind.
- A leading `./` is stripped before any `compare?path=` (and `files?path=`) call: the inventory keys by tree path, and the two ingresses normalise differently.

## 13. Carry / adapt / drop, reconciled against the CURRENT contract

| Reference pattern | Verdict | Why, against `api.ts` + §78 |
|---|---|---|
| Nexus: file list rail with status mark · path · comment badge · `+a −d` | **CARRY** | `change.files[]` serves path, kind, sizes, additions/deletions, binary; comments carry `file_path`; emoji marks become tone dots |
| Nexus: file pane renders the selected file only | **CARRY** | 23 files at once is the wall the operator hit; the widget renders the selected file's hunks from the one served diff |
| Nexus: Unified / Side-by-side toggle, remembered in localStorage | **ADAPT** | toggle kept (already "Side by side / Inline"); no web storage (§41-B) so it is per visit; phone defaults to inline |
| Nexus: pair selector (`round \| base`, `vN → current`) | **CARRY** | S13.1 + `compare?old=&new=` serve exactly that; base = `old=0` |
| Nexus: "New file (preview it under the task's Files section)" | **ADAPT → the Whole-file view** | the new `files` route serves one file of one version; this closes the declared sweep gap |
| Nexus: hunk header · add/del tints · mono 11.5px · faint line numbers · hover "+" | **CARRY (through the widget's own variables/selectors)** | adopt-don't-fork: react-diff-view's DOM is styled through its CSS variables and our frame, never patched |
| Nexus: Pygments syntax colours | **DROP** | FC-v1 §2: client-side tokenizer, no server highlighter; a grammar package is a new adoption and not this packet's; word-level edit marks stay |
| Nexus: Findings panel (open / addressed batches by `consumed_at`, jump to the line) | **CARRY** | the comments block + the synthetic strip already render this; jump-to-file added; consumed batches already show their attempt |
| Nexus: Critic panel with SHIP/REVISE/REWRITE chip | **ADAPT** | no verdict document is served on the deliverable; the verify run's `ending` on the task read is the served sentence and renders verbatim in the strip |
| Nexus: "Retry with this feedback" | **ADAPT → served doors only** | v0 has no retry verb; comments drain at the S13.4 point when the platform asks; the request-revision door renders its served state |
| Nexus: edit / delete a comment | **DROP** | comments are immutable (S13.3; pinned by a landed test) |
| Nexus: modal review | **ADAPT → full page** | `/deliverables/:id` is a published deep link (push targets it) |
| Nexus: image pane inside the file pane | **CARRY (existing)** | the image-pair surface and its trio stay as landed |
| Nexus: "Binary file — status, KB → KB" | **CARRY** | `binary`, `old_size`, `new_size` are served |
| Current page: "The work" (rendered report first) | **ADAPT → demoted for repo-backed work** | for a repo-backed revision the tree is the work; for content-pinned documents the rendered document stays first exactly as today |
| Current page: doors list with open/closed chips | **ADAPT** | doors feed the decision strip; the list moves into the folded record |
| Current page: verification posture banner | **CARRY (moved)** | sits with the checker's findings |
| Current page: revisions list + lineage | **ADAPT** | the version strip + the folded record |
| Current page: try-it copy | **ADAPT** | 11.4 |
| Current page: accept card, comment loop, follow-up spawn | **CARRY** | behaviour contracts, unchanged |
| Task card: undifferentiated rail + "run stands at crashed" nodes | **ADAPT** | 11.2, from `parent_run_id` + `ending` (§76) |
| Receipt: no judge line | **ADD** | 11.3, from `receipt.judge` (§73) |

## 14. Honesty invariants this journey keeps (and how each absence reads)

- **Escape-first, no raw HTML**: the widget renders text; the whole-file view is escaped text in a `<pre>`; the report keeps its sandboxed frame; nothing else may render markup.
- **Nothing invented**: every count, size, kind and pin on the page is a served field; totals are sums of served rows and say so.
- `change.absent_reason` → "The file list could not be built: «reason»" (the page still shows the report and the record).
- `500 content_drift` on any read → "The platform can no longer find the saved version this work pins («served detail»). That is a platform fault, not the work's; nothing here is invented to cover it." The rest of the page renders what it has.
- A `404` on a file → the served detail ("holds no file … at version …").
- Truncation on the three caps → the served `truncation_reason` verbatim, plus where to read the rest.
- A comment with no live anchor → the strip (no comment without a render location).
- A door closed → its served reason; a launch → the served disposition; a state the build does not know → rendered as itself.

## 15. The contract consumed, and what the SPA's mirror gains (additive, `api.ts`)

`DeliverableDetail.change?`, `Comparison.change?` (`Change` + `ChangedFile`), `api.compare(id, {old, new, path})`, `api.revisionFile(id, revision, path)` → `FileContent` (`truncated`, `truncation_reason`), `TaskRunView.parent_run_id?`/`ending?`, `Receipt.judge?` (`model`, `self_family`, `note`). The fixtures are backend-owned and untouched; renders the goldens do not carry (judge line, a fresh crash's ending, truncation, `content_drift`) are proven with hand-scripted bodies citing their Go producers, the landed precedent. `GET /api/deliverables/{}/files` leaves the sweep's gap list (counts 2 → 1).

## 16. Build order inside the packet, and the checkpoints

1. `api.ts` mirror → the review page's shell on the real webshop: decision strip, file rail, reading pane (Changes / Whole file), version strip. **Checkpoint 2 = rendered screenshots (desktop + 390px) of this page over the builder world.**
2. The report demoted, the checker's findings with jumps, the try-it copy, the folded record; phone layout.
3. The task card's attempt-grouped rail, the receipt's judge line, the Reviews index (if Q1 = yes), the task card's "Review this work" door.
4. Tests (behaviour contracts kept; presentation-coupled ones rewritten after the design settles), the sweep gap closed, the web battery; then the coordinator's live design review + cold walks + machine battery.

## 17. Questions for the operator (checkpoint 1)

- **Q1** The Reviews index at `/reviews` as the front door: build it in this packet (recommended, small) or keep the placeholder?
- **Q2** The "try it now" script line (11.4): on the page for everyone, or operator-only?
- **Q3** New files open as **Whole file** by default, changed files as **Changes**: agree?
- **Q4** The word for the send-back action on the strip: "Ask for changes" (proposed) or another phrase you use?
