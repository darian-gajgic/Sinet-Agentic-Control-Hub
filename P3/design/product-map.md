# Sinet product map v2 — the rework's checkpoint 1 (2026-08-05)

**Status: PENDING OPERATOR APPROVAL — nothing gets built before this page is approved** (FRONTEND.md rule 3). v2 after operator feedback on v1: the map now reconciles **the full Nexus tab set** (23 views read from the real `index.html` nav), not just visual patterns. Seeded from `rework-operator-findings.md` + the operator's v1 feedback; every verdict grounded against the live backend (`web/src/api.ts`, `internal/api` routes and projections) and the spec's behavior sections. Visual direction stays as ratified: recreate the Nexus violet-glass look from the real source, reference-over-prose.

## 1. The jobs this app does for you

1. **Give it work** — "Describe a goal" in plain words; it asks its questions like a form; you trim/approve its plan before it spends anything.
2. **See what's happening** — one glance: what's running, what's stuck waiting on you, what it costs, who's burning what, how the machine itself is doing.
3. **Work by project** — tasks live in projects; pick a project and the app shows you that project's world.
4. **Decide things** — everything that needs a human sits in one inbox; answering resumes the work in place.
5. **Judge the work** — see what came back and what the judges said; **run the program it built**; send it back with comments; accept when right.
6. **Make it smarter** — wins and lessons it reads on future work; memory; specialists and their measured quality.
7. **Ask about anything** — the assistant + a real history/observability instrument.

## 2. Navigation (plain names, Nexus-style grouped sidebar)

| Group | Entry | Route | Status |
|---|---|---|---|
| **Work** | Home | `/` | exists |
| | Projects | `/projects` | **NEW route** |
| | Describe a goal | `/new` | **NEW route** |
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

**Topbar: the global project selector** — the Nexus focus-context chips, carried properly. Pick a project (or "(no project)" — the backend serves it as a real bucket) and every surface whose data carries a project dimension scopes to it: Home, Board, Reviews, History, task lists. Surfaces without that dimension (Fleet is per-person/lane, Settings, Health) say so instead of pretending. **Sidebar footer: the CPU · MEM · GPU · GPU-MEM micro-meters** — see §5, two need backend work.

All published deep-link routes stay (`/tasks/:id`, `/inbox/:id`, `/deliverables/:id`, `/memory/:id`, `/login`).

## 3. Surfaces — what you see / what you can do

**Home.** *See:* hero, four stat tiles (running · waiting-on-you · parked-until · spend today), the live roster, per-person burn, a slim recent-activity feed. *Do:* drill anywhere; personal filters (what-needs-me · mine · running · finished-today). The deep history instrument moves to History; Home stays a glance.

**Projects.** *See:* every project as a card — active tasks by stage, waiting-on-you count, spend, recent deliverables, follow-up lineage; "(no project)" as its own bucket. *Do:* open one → the global selector scopes the app to it; "Describe a goal" straight into this project; jump to its board/reviews/history pre-filtered. *Honest v0 limits:* a task's project is set at intake (the questionnaire asks; the project registry prefills) and follow-ups inherit it — **there is no move-task-between-projects verb in the backend today** (§6, decision 3).

**Describe a goal.** *See:* one plain ask box (literally the Nexus phrase), then the interview as a **form, not a chat**: cards of up to 4 questions, 2–4 labeled options + free text each; a live Clearance meter; the stakes/size guess. *Do:* answer or force-proceed (open questions become listed assumptions); then the plan card — what I understood · what you'll get · numbered steps · **what I will NOT do** · assumptions front-and-center · cost/time — **Approve / Re-plan / Re-interview / Cancel**. Nexus's "untick optional stages" carries as Re-plan's structured entry: tap the step you're contesting, it comes back re-planned without it. Trivial read-only tasks skip ceremony. *(Reconciliation note: no spec amendment needed — the interview is specified as a questionnaire (S06.5), and `POST /api/intake/requests` already exists server-side; the rejected UI just never called it. Chat remains the secondary door, same interview inline.)*

**Board.** *See:* a **recognizable Kanban** — stage columns, glowing-dot headers, count pills; cards = what/whose/stage/effort/cost-so-far/waiting-on-human + priority rail; grouped by project when unscoped. *Do:* drag to reorder **your own queued** tasks (priority hint — columns are the machine's state, never writable by drag); per-card cancel; click through.

**Task detail.** *See:* the confirmed spec with numbered acceptance criteria, the plan as numbered steps each with "done when", live per-stage progress, every human decision, every revision, the receipt (ceremony vs execution, honest labels), lineage (project + follow-ups). *Do:* state-computed action bar — only what this state allows (cancel · follow-up · answer the open card · jump to review).

**Inbox.** *See:* risk-ranked cards — what's being approved · mono provenance line · plain "what to check first" · jump to the thing · expiry countdown; stale plans auto-flag. *Do:* Approve / Deny / Answer / Re-plan; Low tier batchable; High tier re-prompts PIN; answering resumes work in place. Blind-pair benchmark verdict forms arrive here too.

**Reviews.** *See:* deliverables as numbered immutable revisions; diffs side-by-side/inline with line-anchored comments; **the judge's verdict + numbered findings per round**; round-over-round "what changed since I sent it back" as the default rework view; images as a proper visual compare (Side by side · Swipe · Overlay — overlay aligns bitmaps only, never text); binaries as hash + download. *Do:* **Run it** — for a program/site deliverable one click starts it live in the before/after preview (the dual-iframe try-it stack, already served: launch/stop/compare); **every previous revision navigable, previewable, downloadable**; comment at the line; request a bounded revision (that IS the loop); accept in one action; spawn a follow-up task.

**Wins & Lessons.** *See:* the Nexus feedback ledger, rebuilt on Sinet memory: WINS (measured successes) and LESSONS (flop + its correction) as browsable shelves — memory entries carry `kind`, scope (person/project/house) and provenance, so the shelves are real reads, not decoration. *Do:* log a win/lesson from a task or deliverable (pre-linked to it); browse by project/person; retire. **These feed future work today** — the S09 influence machinery injects matching entries into runs at v0. *Honest v0 limit:* auto-distilled lessons from your rejections arrive with the v1 proposal pipeline (spec-scheduled); at v0 logging is manual, reading is automatic.

**Assistant.** *See:* persistent sessions, streamed turns, file sidebar (drag-drop up, produced-files chips down), the same intake questionnaire inline when you hand it work. *Do:* ask about the platform and its history (answers through the query layers; open-SQL answers visibly carry their lower-confidence flag); hand over work; exchange files; stop a turn; navigation never kills a running turn. Finding 5's broken plain-ask is reproduced on the seeded world and fixed inside this journey.

**History.** *See:* the observability instrument, findable at last: the filterable event stream (project/person/task/date), the served query registry (views → canned queries → open SQL), answers with their audit attached; refusals and disambiguations rendered as the honest non-answers they are; marked not-live (a query instrument, not a feed). *Do:* ask, search, drill from any answer to the task/run it names.

**Memory.** *See:* every entry scoped person/project/house with provenance, gate status, verification state; conflicts surfaced. *Do:* browse/read/create/new-version/retire/delete-own; resolve conflicts; notes live here as plain entries; promotion to house via operator approval.

**Specialists** (the workforce map, plain-named). *See:* every worker: what it's equipped with (tools, knowledge, permissions, helpers), how multi-stage procedures connect, and **its measured quality/cost per version** — the evals-per-specialist read. *Do:* view-only at v0 (editing parked by spec — said on the surface, no dead controls). *Honest v0 limit:* per-specialist learned-lesson overlays exist in the schema but are dormant until v1 writers arrive.

**Fleet.** *See:* who burns what — per-person/per-lane meters, donut gauges, budget bars with hot state, burn rates, limit events with "parked until…"; accounts always distinguished; local-tier seat states and GPU/VRAM once wired (§5). *Do:* edit own budgets; pause-my-automation; filter.

**Health & evals.** *See:* alerts-first, Nexus-monitor style: **Known issues** at the top (open watchdog flags, drift records, conformance failures, alarms, parked runs — each with what/why/what-to-do), then benchmark state (your opt-in, blind-pair verdict history, what the numbers mean), canary status (disarmed at v0 — displayed honestly, not hidden), then — after §5 lands — host meters (CPU/RAM/GPU/VRAM). Badge on the nav entry = open issue count. *Do:* suppress a flag (with reason), dismiss drift, acknowledge conformance, dispose an alarm, resume a parked run, set benchmark opt-in — every verb already served.

**Settings.** *See:* every setting with bounds, plain help, per-setting audit history; the price table; push devices; **Household** — the member list with add-member (served: `GET/POST /api/auth/users`), PIN posture, push enrolment per device. *Do:* edit values within clamps (operator edits bounds); onboard a household member end-to-end. The standing "see and change everything incl. today's constants" directive remains queued as its own amendment; the surface is built so those rows slot in.

**Manual.** *See:* the Nexus manual carried: a plain-words page per surface — what it's for, how to use it, what the words mean — plus the first-run tour launcher. Frontend-only, no backend. *Do:* start any surface's guided tour (rebuilt so the spotlight never swallows clicks).

**Login.** Tailnet + PIN, unchanged behavior, styled with the rest.

**Cross-cutting rules (bind everywhere):** every control labeled in plain words and either works or isn't rendered; disabled controls say why; connection pill always visible, stale never poses as live; empty states say what will appear; phone-complete = inbox + decisions + filters + board/fleet glance + task status + chat + push.

## 4. The full Nexus tab reconciliation (all 23 views)

| Nexus tab | Verdict | Where it lands in Sinet |
|---|---|---|
| dashboard | **CARRY** | Home (hero → tiles → gauges/roster → feed hierarchy) |
| projects (+ Describe a goal) | **CARRY/ADAPT** | Projects tab + `/new` door; assignment at intake, follow-ups inherit (§6-3) |
| workflows (pipeline stages) | **ADAPT** | The plan inside each task (numbered steps + done-when); big goals = follow-up lineage under one project — no separate pipelines object at v0 |
| kanban | **CARRY/ADAPT** | Board; drag = own-queue priority hint only, columns are FSM truth |
| deliverables | **CARRY** | Reviews (+ Run-it previews, revision history) |
| meetings | **DROP** | No backend concept; its job (watch agents work) is served by live per-stage progress on task detail |
| agents (live roster) | **CARRY** | Home roster + Fleet |
| decisions | **CARRY** | Inbox (one queue for everything needing a human) |
| agentic (approvals) | **CARRY** | Inbox — row anatomy carried ~1:1 (what · provenance · check-first · jump · verbs) |
| specialists | **CARRY/ADAPT** | Specialists (`/workforce`); view-only v0, learning overlays dormant until v1 |
| memory | **CARRY** | Memory (scoped entries, provenance, conflicts) |
| skills | **ADAPT** | Inside a specialist's detail: its equipment (tools/knowledge/permissions/helpers) |
| monitor (CPU/MEM/GPU/VRAM) | **ADAPT — needs backend** | Health & evals + sidebar micro-meters; §5: VRAM/seats = named seam, CPU/RAM = amendment |
| tools | **ADAPT** | Specialist equipment view (same data home as skills) |
| programs | **ADAPT** | Reviews' Run-it previews — a "program" is a deliverable you can run |
| guardian | **ADAPT** | Health & evals (watchdogs, canaries, drift, conformance — the Sinet guardians) |
| usage | **CARRY** | Fleet + per-task receipts |
| observability | **CARRY** | History (event stream + query layers with audit) |
| issues (known issues) | **ADAPT** | Health & evals' Known-issues section + nav badge; platform-detected issues (flags/drift/conformance/alarms) — no user-filed tracker at v0 |
| notes | **ADAPT** | Memory entries (person scope) |
| settings | **CARRY** | Settings (registry-generated, every setting + bounds + audit) |
| manual | **CARRY** | Manual tab + per-surface tours |
| jarvis (assistant) | **CARRY/ADAPT** | Assistant (persistent sessions, files, intake handoff); voice/avatar stays parked v1+ by spec |
| — global focus context (topbar) | **CARRY** | The global project selector (§2) |
| — sidebar CPU/MEM/GPU/GPU-MEM footer | **CARRY — needs §5** | Sidebar micro-meters |
| — wins & lessons ledger (inside memory view) | **CARRY** | Its own nav entry, Wins & Lessons, on memory kinds |

Visual-pattern verdicts from map v1 all stand (glass tokens, stat tiles, approval anatomy, diff viewer, gauges, empty states, tours, toasts/drawer/modal/skeleton carried; Chart.js, hand-drawn icon set, 3D avatar/galaxy, cross-column drag dropped).

## 5. Backend work this map needs (runs under the FOUR-STAGE pipeline, not the frontend rework)

1. **Wire the declared fleet seams (no amendment needed):** `FleetSnapshot` already declares `gpu {vram_used, vram_total}` and `local_seats[]` as honest-empty v0 seams (`internal/api/projection.go`), and the B4-6 VRAM admitter already does live sysfs/nvidia-smi reads. One small packet connects them so Fleet, Health and the sidebar GPU/GPU-MEM meters read truth.
2. **CPU/RAM host monitoring — needs an S00.9 amendment:** no host-resource read exists anywhere in the spec or backend. Proposal: a minimal read-only host block (CPU %, RAM used/total, load) on the fleet snapshot, same honest-seam pattern. Small, read-only, no new dependency expected. **Your call (§6-2).**

## 6. Decisions I need from you (free text, as always)

1. **Approve map v2** as the rework's contract (or mark up anything — names, groupings, verdicts).
2. **CPU/RAM amendment (§5-2):** recommend YES — without it the machine-health story stays GPU-only.
3. **Move-task-between-projects:** not served by the backend today; v0 = project set at intake + follow-ups inherit + "(no project)" bucket. Recommend accepting for v0 and amending later only if it actually hurts. Say the word if you want the reassign verb now (that's an amendment + backend packet too).
4. **Naming taste:** "Specialists" vs "Workforce", "Describe a goal" vs "New task", "Health & evals" vs "Monitor" — defaults are as written; override freely.

## 7. Build order (journey-sliced; checkpoint 2 = rendered screenshots after step 1)

1. Shell (nav, project selector, footer meters with honest placeholders) + Home
2. Give-work: Describe a goal → interview → plan approval (+ its inbox card)
3. Projects + Board + task detail
4. Decide: Inbox
5. Judge: Reviews + Run-it previews
6. Steer/insight: Fleet, Health & evals, Wins & Lessons, Memory, History
7. Specialists, Settings (+ Household onboarding), Manual, Assistant (finding-5 reproduction first)

Backend, `web/src/api.ts`, router and data hooks stay where sound; every view rewritten as one design by the single builder; behavior contracts (review loop, honesty invariants, escape-by-default, owner-scoping) bind throughout.
