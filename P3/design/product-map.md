# Sinet product map — the rework's checkpoint 1 (2026-08-05)

**Status: PENDING OPERATOR APPROVAL — nothing gets built before this page is approved** (FRONTEND.md rule 3). Seeded from `rework-operator-findings.md`; every verdict reconciled against the live backend contract (`web/src/api.ts` + `internal/api` routes) and the spec's behavior sections (cited only where load-bearing). Visual direction is already ratified and is NOT re-described here: recreate the Nexus violet-glass look from the real source files, reference-over-prose.

## 1. The jobs this app does for you

1. **Give it work** — describe what you want; it asks its questions like a form; you approve its plan before it spends anything.
2. **See what's happening** — one glance: what's running, what's stuck waiting on you, what it costs, who's burning what.
3. **Decide things** — everything that needs a human sits in one inbox; answering it resumes the work in place.
4. **Judge the work** — see what came back, what the judges said about it, send it back with comments, accept it when it's right.
5. **Steer the machine** — priorities, budgets, pause-my-automation, memory, every setting.
6. **Ask about anything** — the assistant answers questions about the platform and its history, and can take work in too.

## 2. Navigation (plain names, Nexus-style grouped sidebar)

| Group | Entry | Route |
|---|---|---|
| **Work** | Home | `/` (mission control) |
| | **New task** | `/new` — **NEW route** (addition, sanctioned; renames stay banned) |
| | Board | `/board` |
| | Inbox (glowing pending badge) | `/inbox` |
| **Results** | Reviews | `/reviews` — **NEW list route**; deliverable detail stays `/deliverables/:id` |
| **Assistant** | Assistant (pinned entry, Nexus-style) | `/chat` |
| **System** | Fleet | `/fleet` |
| | Workforce | `/workforce` |
| | Memory | `/memory` |
| | Settings | `/settings` |

Published deep-link routes (`/tasks/:id`, `/inbox/:id`, `/deliverables/:id`, `/memory/:id`, `/login`) all stay — push notifications point at them. Sidebar footer keeps the Nexus micro-meters (live burn per person) and version.

## 3. Surfaces — what you see / what you can do

**Home (mission control).** *See:* hero line, four stat tiles (running · waiting-on-you · parked-until · spend today), the live roster of everything running/queued/parked/blocked/recently-finished, per-person burn meters, the activity feed, and the ask-history panel (a query instrument — marked as not-live, honestly). *Do:* drill into any task or run; one-tap personal filters (what-needs-me · mine · running · finished-today); ask questions over history (open-SQL answers visibly carry their lower-confidence flag).

**New task — the questionnaire (finding 6; full reconciliation in §4).** *See:* one ask box; then the interview as a **form, not a chat**: cards of up to 4 questions, each with 2–4 labeled options plus free text; a live **Clearance meter** that rises as you answer; the platform's stakes/size guess. *Do:* answer, or **force-proceed** at any point (open questions become listed assumptions); then the plan card — what I understood · what you'll get · what I'll do · **what I will NOT do** · the assumptions front and center · expected cost/time — with **Approve / Re-plan / Re-interview / Cancel**. Trivial read-only tasks skip the ceremony and just deliver with a completion note.

**Board.** *See:* a **recognizable Kanban** — columns are the task stages with glowing-dot headers and count pills; cards show what it is, whose it is, stage, effort mode, cost so far, waiting-on-human; a slim priority rail; follow-up lineage grouped by project. *Do:* drag to reorder **your own queued** tasks (a priority hint — columns are never writable by drag; the stage is the machine's own state); per-card cancel; click through to detail.

**Task detail (finding 9's home — the "nice organized structure").** *See:* the confirmed spec with its numbered acceptance criteria, the approved plan as numbered steps each with its "done when", live per-stage progress, every human decision along the way, every deliverable revision, and the receipt (ceremony vs execution, honest labels). *Do:* a **state-computed action bar** — only the actions this state allows (cancel · follow-up · answer the open card in place · jump to review).

**Inbox.** *See:* cards ranked by risk; each card = **what's being approved · a mono provenance line · plain-language "what to check first" · jump to the thing itself · expiry countdown**; stale plans auto-flag "assumptions may be stale". *Do:* Approve / Deny / Answer / Re-plan; batch-answer the Low tier only; High tier re-prompts your PIN; answering resumes the paused work in place.

**Reviews (findings 7+8's home — the judge feedback and the loop, findable).** *See:* deliverables as numbered immutable revisions; code/text as side-by-side or inline diffs with comments anchored to lines; **the judge's verdict and numbered findings per round**; **round-over-round "what changed since I sent it back"** as the default rework view; images as a visual compare rebuilt properly (overlay aligned bitmaps only — captions and text never overlap; the three modes labeled in plain words: Side by side · Swipe · Overlay); binaries as hash + download; one-click before/after preview. *Do:* comment at the exact line; request a bounded revision (that IS the loop); accept in one action; spawn a follow-up task.

**Assistant.** *See:* your chat sessions (kept on the platform, on every device), streamed answers, a file sidebar (drag-drop up, produced-files chips down), and — when you hand it work — the same intake questionnaire inline. *Do:* ask about the platform and its history; hand over work (becomes a task through the same interview); exchange files; stop a turn; navigation never kills a running turn. The broken plain-ask flow ("picture of a car") is reproduced on the seeded world and fixed as part of this journey.

**Fleet.** *See:* who burns what — per-person/per-lane meters as donut gauges, budget bars with a "hot" state past threshold, burn rates, limit events with "parked until…" times; accounts always distinguished. *Do:* edit your own budgets; pause-my-automation (two-position, honest about what it stops); filter by person/lane/period.

**Workforce.** *See:* every worker, what it's equipped with, how multi-stage procedures connect, per-version quality/cost readings. *Do:* view-only at v0 (editing is parked by spec — the map says so on the surface instead of showing dead controls).

**Memory.** *See:* entries scoped person/project/house with provenance and gate status; conflicts surfaced. *Do:* browse/read/create/new-version/retire/delete-own; resolve conflicts; promotion to house goes through operator approval.

**Settings.** *See:* every setting with its bounds, plain help text, and per-setting audit history; the price table; push devices; benchmark opt-in. *Do:* edit values within clamps (operator edits bounds); enrol this phone for push. The standing "see and change everything, including today's code constants" directive stays queued as its own later amendment — the surface is built so those rows slot in.

**Login.** Tailnet + PIN, unchanged behavior, styled with the rest.

**Cross-cutting rules (findings 1/3/10):** every control is labeled in plain words and either works or is not rendered; a disabled control says why; the connection pill (LIVE / catching up / disconnected) is always visible and stale data never poses as live; empty states say what will appear there; the per-view guided tour returns, rebuilt so its overlay can never swallow clicks; phone-complete = inbox + every decision + filters + board/fleet glance + task status + chat + push.

## 4. The questionnaire-vs-chat reconciliation (finding 6 — named, not buried)

You expected a Nexus-style questionnaire; the built UI's only door was the assistant chat. **The spec already agrees with you** — no amendment is needed:

- The intake interview is *specified* as a questionnaire: batched option cards, 2–4 labeled options + free text per question, a live Clearance indicator, force-proceed (S06.5).
- A direct create door is contract: the tasks family lists "create (opens intake)" (S15.2), and the backend already serves it — `POST /api/intake/requests` exists today and the rejected UI simply **never called it**; chat handoff was the only door built.

**Verdict: ADAPT.** "New task" becomes the primary door, rendering the interview as the questionnaire above. Chat handoff stays as the secondary door (per spec S15.7) — same interview, inline. The one honest delta from Nexus: the form is not fixed — questions are generated per kind of task and stop when the task is clear enough (or when you force-proceed), so simple asks stay short.

## 5. Nexus pattern ledger — carry / adapt / drop

| Nexus pattern | Verdict | Reconciliation vs the current backend |
|---|---|---|
| Violet-glass design language, tokens, aurora, glow-as-status | **CARRY** | Ratified; poured from the real `style.css`, not prose |
| Shell: blurred grouped sidebar + topbar + connection pill + footer micro-meters | **CARRY** | Meters read `/api/meters`; pill reads the SSE state |
| Dashboard hierarchy: hero → stat tiles → gauges/fleet row → live roster → feed | **CARRY** | = Home; roster/feed ride the live event feed |
| Stat tiles (gradient hairline, mono count-up) | **CARRY** | Data from tasks/runs/meters snapshots |
| Live Kanban board (columns, glowing headers, count pills, card chip-strips, priority rail) | **ADAPT** | Columns = the machine's stages, **never writable by drag**; drag = own-queued reorder only (`priority-hint`); per-card stop = the cancel verbs |
| Task-detail with state-computed action bar | **ADAPT** | Actions = cancel / follow-up / answer-card / accept — only what the state allows |
| Questionnaire intake | **ADAPT** | §4 — primary New-task door on `POST /api/intake/requests`; clarity-driven, not a fixed form |
| Approval rows: what · provenance · "check this first" · jump · verbs; glowing pending badge | **CARRY** | Maps ~1:1 onto the inbox contract (tiers, batch-Low, PIN on High) |
| PR-style diff viewer (file list, split/inline, line comments) | **CARRY** | Via the binding react-diff-view pick; comments/anchors are the server's |
| Guided tours (`?` spotlight walkthrough) | **ADAPT** | Kept; overlay rebuilt so it never blocks input (batch defect) |
| Empty states that explain what will appear | **CARRY** | App-wide rule |
| Toasts / drawer / modal / skeleton | **CARRY** | From the Nexus overlay set |
| Donut gauges, micro-bars, budget bars with hot state | **CARRY** | Fleet + sidebar; owned SVG (the Nexus generator is trivially a component) |
| Global focus context (Project › Workflow scoping chips) | **ADAPT** | Becomes the personal filters + project grouping the backend actually serves |
| Inline hand-drawn SVG icon set | **DROP** | lucide is the adopted, lock-pinned icon organ — same stroke discipline, no hand-copied set |
| Chart.js charts | **DROP** | CDN dep, banned; gauges/bars/sparklines are owned SVG at v0 |
| 3D avatar, memory galaxy, Jarvis voice | **DROP at v0** | Parked by spec behind the benchmark gate (v1 re-entry plan recorded) |
| Fixed 5-column board taxonomy, cross-column drag | **DROP** | Stage set comes from the FSM; drag-to-move-stage contradicts the machine |

## 6. Build order (sliced by journey, per FRONTEND.md — for the record, not for approval line-items)

1. Shell + Home → **checkpoint 2: rendered screenshots to you**
2. Give-work journey: New task → interview → plan approval (+ its inbox card)
3. Decide journey: Inbox + task detail
4. Judge journey: Reviews + deliverable detail + previews
5. Steer journey: Board, Fleet, Settings, Memory, Workforce
6. Assistant (with the seeded-world reproduction of finding 5 first)

Backend, `web/src/api.ts`, the router and data hooks stay where sound; every view is rewritten as one design by the single builder. Behavior contracts (review loop, honesty invariants, escape-by-default, owner-scoping) stay binding throughout.
