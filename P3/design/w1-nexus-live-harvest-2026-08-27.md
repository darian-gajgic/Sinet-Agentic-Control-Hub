# W1 — Nexus planning-phase LIVE click-through (2026-08-27)

Evidence walk ordered by `b6-gate-operator-findings-r5-2026-08-23.md` §D (W1), under the §D.1
HARVEST CONSTRAINT: **pattern harvest only** — every entry below carries a FIT-AS-IS /
FIT-WITH-MODIFICATION / REJECT verdict grounded in Sinet's own machinery (product-map v3,
S06 interview/planning, S15 API posture, FC-v1). No Nexus code is ported; what crosses over is
the UX pattern re-expressed on Sinet's wire/ledger/spec. Internal engineering study only —
Nexus is never cited in any user-facing deliverable.

Walk parameters: Nexus live at `https://127.0.0.1:8777` (UI footer "NEXUS v3.0"), real Chrome,
real engine calls on the operator's subscription (sanctioned). Goal = the r5 benchmark goal,
verbatim substance: *"Create a webshop for car replacement parts and tuning parts. BMW/Audi
focus. At least 30 seeded items, each with picture, price, info and compatibility. An item
detail view. A cart without payment. Modern design with basic animations. A brand+model search
filter. v1 fully working."* Target repo: fresh empty `/home/sinep/Projects/car-webshop-w1`
(git-initialized, 0 files) — NOT the operator's `car-webshop-test`. Focus context was switched
to car-webshop-w1 for the walk and restored to car-webshop-test afterwards.

Screenshots: `P3/design/rework-screens/w1-nexus/w1-*.jpg` (referenced inline).

---

## A. Walk transcript — surface by surface

### A.1 Entry: Projects list and focus context

- Nexus had ALREADY auto-discovered the fresh repo: `car-webshop-w1` appears in the Projects
  table (personal · master · clean · "⚠ local only" · 0 KB) without any registration step —
  the projects surface scans the filesystem.
- Each project row has a **focus "select" button** with hover text, verbatim: *"Work in this
  project: Workflows & Tasks scope to it; new work is assigned to it"*. Clicking it sets the
  topbar chip **"🎯 WORKING IN 📂 car-webshop-w1 ✕"** plus the caption *"new workflows & tasks
  are assigned here automatically"*, and fires a toast *"Working in project: car-webshop-w1 —
  Tasks are now scoped to it"*. Board/task surfaces re-scope instantly.
- A project row click opens a project detail modal (path, description, git branch + remote,
  size, venv, languages by file count, work history, git workflow verbs). Its work-history
  empty state, verbatim: *"No pipelines have visited this project yet — target it via
  ✨ Describe a goal."*

Screenshot: `w1-01-tasks-board-scoped-w1.jpg` (Tasks board scoped to the empty project;
columns Backlog / To Do / In Progress / Review / Done, "0/59 tasks", the scope hiding all 59
other tasks).

### A.2 The goal-entry wizard ("✨ Describe what you want done")

Opened from the Tasks board toolbar button "✨ Describe a task" (button hover text: *"Describe
it in plain words — the AI fills in every parameter"*). Screenshot: `w1-02-describe-wizard.jpg`.

Verbatim surface text:

> **✨ Describe what you want done**
> Plain words, German or English. The AI may ask up to 5 clarifying questions first (every one
> skippable), then plans: specialist, domain, model, priority, high-stakes flag and the full
> brief — you review before anything is created. Coding goals become the full pipeline (spec →
> implement → review → fix → verify) automatically.
>
> [textarea, placeholder: "e.g. We have to develop our application. It has to be a webshop for
> clothing with a basket and payment…"]
>
> 📂 About an existing project? (optional)
> [dropdown — **pre-selected to the focused project car-webshop-w1**; 35 repo entries]
> Pick the project and the AI plans an **improvement round on the real thing**: it reads the
> project's contents, skips questions the project already answers, and writes briefs that
> reference the existing work. Bug fixes and new features on client projects always go through
> here.
>
> ☐ ✨ Super Result
> A grounded frontier critic independently re-verifies every deliverable (full tool access in a
> disposable sandbox), files line comments, and loops the work until it verifies. ~5–10×
> tokens: independent verification rounds + fan-out.
>
> 💡 This wizard only **plans** — the actual work happens later, inside the task it creates.
> Describe the goal and desired outcome (e.g. "a summary of what is on a picture"); if the work
> needs files, 🖇 attach them to the created task afterwards.
>
> [Cancel] [✦ Deep Plan] [✨ Plan it]

Deep Plan button hover text, verbatim: *"Plan a complex goal conversationally — a short
interview builds a spec, then drafts + premortem-checks the plan"*.

Wire observed on Deep Plan click: `GET /api/plan/sessions` (resume check by identical goal) →
`POST /api/plan/telemetry` → `POST /api/plan/sessions` (turn 0 runs inside this POST;
~60–90 s). A later identical click resumed the existing session (`GET
/api/plan/sessions/{id}`) instead of duplicating — the resume behavior is real.

### A.3 The Deep Plan interview modal — turn 1

Screenshot: `w1-03-deepplan-turn1-content-family.jpg`. Two-pane modal: left = conversation
(planner message + question cards + one free-text box), right = the live SPEC pane. Header:
"✦ Deep Plan" + family chip + repo chip "🧬 car-webshop-w1". Intro, verbatim: *"A few targeted
questions build a structured spec, then it drafts and premortem-checks the plan. You can edit
any slot directly, type freely, switch the plan type, or **Draft plan** at any time."* Footer:
[Abandon] left, [Draft plan →] right. Input placeholder: *"Answer here (click an option to
fill it in), or just describe more…"*. Turn line: *"Turn 1. Ctrl+Enter to send."* + [Send].

**Family misclassification, observed live:** this session came back family **"Content /
marketing"** — the operator's identical-substance goal on 2026-08-23 ran as family
**"software"** (verified via the stored session `plan-28a8fb42`, family=software, repo
car-webshop-test, status drafted). The compressed one-paragraph phrasing was enough to flip
triage. The spec pane therefore templated Audience/Channel/Core-message/Voice/…, not
Goal/Stack/Acceptance-criteria.

Planner message, verbatim: *"Interview round 1 of 3: your goal already fixes scope (30+ items,
detail view, cart, filter) — I've folded that in; three decisions remain: who it's for, where
it runs, and the one promise it must land."*

The three question cards (all in ONE turn; each: bold question + grey slot tag + faint
why-line + option buttons, exactly one ★-recommended):

**Q1** — *"Who is the primary visitor this webshop is built for?"* — slot `audience`
why: *"The audience decides copy tone, how technical compatibility info is phrased, and
whether tuning or replacement parts lead the catalog."*
options: ★ DIY BMW/Audi owners · Casual car owners (non-technical) · Tuning enthusiasts /
car-scene crowd · Workshops / professional mechanics (B2B)

**Q2** — *"Where does v1 run and in what technical form?"* — slot `channel_format`
why: *"The repo is empty, so nothing answers this yet: static-frontend vs frontend+backend
changes how the 30 items are seeded and how the cart persists."*
options: ★ Local static web app (HTML/JS/CSS + seeded JSON/JS data, served locally, cart in
localStorage) · Deployed static site (same stack, built for a host like Netlify/GH Pages from
day one) · Frontend + API backend (FastAPI/Node serving product data, server-side cart) ·
Page/module inside an existing running app of yours

**Q3** — *"What is the ONE thing the shop must land for every visitor?"* — slot `core_message`
why: *"This becomes the core message the design, copy, and filter UX all reinforce — it also
decides whether replacement or tuning parts headline the catalog."*
options: ★ Right part, guaranteed fit — compatibility-first ('this fits your BMW 3er F30') ·
Performance you can feel — tuning-first, motorsport vibe · OEM quality at fair prices —
trust-first replacement parts · Everything for your car in one place — breadth-first one-stop
catalog

**Optional slots arrived already filled** (inferred, never asked), verbatim excerpts:
- Voice: *"Modern, technical, performance-parts aesthetic (BMW M / Audi Sport catalog feel);
  clean and confident, fitment facts front and center"*
- Success metric: *"v1 fully working end-to-end: catalog browsable, brand+model filter narrows
  results, detail view shows compatibility, cart holds items across browsing — all verifiable
  in a browser"*
- Length: *"Scope budget v1: catalog grid + item detail view + cart + brand/model filter +
  seeded data; no payment, auth, or admin backend"*
- Mandatories / taboos: rendered as a RAW list literal —
  `['MUST: ≥30 seeded products, …', …, 'NEVER: payment integration, real checkout, order
  processing, user accounts in v1']` (a list slot leaking Python-repr formatting into the UI).
- Notes / decisions: *"…Project dir /home/sinep/Projects/car-webshop-w1 is currently EMPTY
  (0 tracked files, no languages detected) — so despite 'improvement round' framing, this is
  effectively the first build in that repo, not a modification of existing code; task
  descriptions should still reference the repo path."* — the repo grounding actively corrected
  the wizard's own "improvement round" framing.

### A.4 Family switch (Content / marketing → Software / coding)

The SPEC pane's family dropdown lists: Software / coding · Analysis / audit · Content /
marketing · Research. Switching to Software / coding fired a single
`PATCH /api/plan/sessions/{id}/spec` and re-templated the pane to: Goal ●required ·
Stack / platform ●required · Acceptance criteria ●required · Users · Constraints ·
Data / integrations · Out of scope · Risks (optional) · Notes / decisions.
**All content-family values (Voice, Success metric, Length, Mandatories) were silently
discarded; only the universal Notes / decisions slot survived. The three already-asked
question cards stayed as-is (content-family slot tags now orphaned), and no re-interview
fired.** Screenshot: `w1-04-deepplan-family-switched-software.jpg`.

### A.5 Answering turn 1

Clicking each ★ option appended its label to the single free-text box, semicolon-joined —
click = quick-fill, not commit; the box read:
`DIY BMW/Audi owners; Local static web app (HTML/JS/CSS + seeded JSON/JS data, served locally,
cart in localStorage); Right part, guaranteed fit — compatibility-first ('this fits your BMW
3er F30')` — then ONE Send (`POST /api/plan/sessions/{id}/turn`, ~60–90 s, question cards dim
to "✓ answered" while pending). All three answers ride one concatenated string; slot
attribution is left to the model.

### A.6 Turn 2 — the spec completed, zero further questions

Screenshot: `w1-05-deepplan-turn2-spec-filled.jpg`. The conversation pane REPLACED the old
message (no scrollback — only the latest planner message is ever visible). Planner, verbatim:
*"Audience, format, and core promise locked in — that fills every required slot, so the SPEC
is ready to draft."* Green readiness line: *"✓ Required slots filled — ready to draft."*
[Draft plan →] was never disabled at any point (readiness advisory only).

Every software slot now filled by the model (operator-editable textareas), verbatim highlights:
- **Goal:** *"A local static webshop (v1 fully working) for BMW/Audi replacement and tuning
  parts, aimed at DIY owners, where compatibility is the core promise: users filter by
  brand+model, verify 'this fits your car' fitment on every item and its detail view, and
  collect parts in a localStorage cart with no payment step."*
- **Stack / platform:** *"Local static web app: vanilla HTML/CSS/JS in
  /home/sinep/Projects/car-webshop-w1 (repo currently empty — this is the first build there),
  product data seeded as a bundled JSON/JS file (≥30 items), served locally over a simple
  static server, cart persisted in localStorage. No backend, no external APIs, no payment."*
- **Acceptance criteria** (6, each independently testable): ≥30 seeded items with
  picture/price/info/compatibility on every card; detail view with full-size picture, price,
  detailed info, explicit fitment list ("fits BMW 3er F30 2012–2015"); brand+model filter
  narrows results with graceful empty state; cart add from card AND detail view, quantity
  change, removal, localStorage persistence across reloads; *"No payment, checkout, or
  order-transmission flow exists anywhere in v1 — the cart is the final step"*; modern design
  with basic animations rendering *"smoothly without layout shift"*, plus *"the app runs fully
  from the repo when served locally — core browsing, filtering, and cart work with no external
  network calls"*.
- **Users:** *"DIY BMW/Audi owners — hobby mechanics who know their model/generation (e.g. 3er
  F30, A4 B9) and want certainty a part fits before buying; desktop-first browsing."*
- **Constraints:** static-only, no build step, all images bundled locally, *"cart limited to a
  versioned localStorage key so schema changes don't corrupt old carts"*.
- **Data / integrations:** *"None external in v1 …"*
- **Out of scope:** *"Payment/checkout, order processing, user accounts/auth, any backend or
  database, admin UI, real inventory/pricing feeds, deployment/hosting, non-BMW/Audi brands."*
- **Risks:** *"Fitment data is hand-seeded and fictional — a wrong 'fits your car' claim is
  the worst failure mode for a compatibility-first shop … sourcing 30+ distinct part images is
  the biggest content risk … localStorage schema drift can break persisted carts; scope creep
  toward checkout is the main v1 risk."*
- **Notes / decisions:** running ledger of every operator decision so far (goal facts + the
  three answers + the empty-repo correction), maintained by the model across turns.

The interview asked **3 questions total, in one round**, then declared itself done — the
operator's r5 complaint ("too few questions to make an actual plan") reproduced exactly.
Never asked: currency/price format, seed-data sourcing (real vs fictional part names),
image sourcing (it self-decided "locally generated placeholder acceptable" inside Risks),
design direction beyond the goal's own words, browser/responsive targets (it self-decided
"desktop-first"), how "30 items" split between replacement and tuning parts.

### A.7 The Deep Plan draft stage — DEAD in this environment (4/4 failures, all silent)

[Draft plan →] was clicked four times (real engine calls, sanctioned). Each time the button
went disabled "Drafting…" for 4.5–7 minutes, then simply reverted to "Draft plan →" with
**no toast, no error line, no state change anywhere in the UI**. The server responses
(captured on the wire) were:

1. ~4.7 min → (response not captured; behavior identical to #2)
2. ~5.0 min → HTTP 502 `{"error":"turn exceeded max_seconds cap; run may finish orphaned
   (recover via transcript)"}`
3. ~7 min → HTTP 502 `{"error":"draft did not converge on a plan"}`
4. ~5.5 min → HTTP 502 `{"error":"turn exceeded max_seconds cap; …"}`

Four paid model runs lost, invisible to the user. The session stayed `active`. Because of
this, the DEEP-PLAN side of steps 6–10 (spec-bannered proposal, premortem auto-fire,
automatic revision round, revision questions, SPEC.md/spec.json hand-off) was **unreachable
live** — the walk covers those surfaces only as far as the docs study describes them.
(Also observed twice earlier: the wizard→Deep Plan click created/attached the session
server-side but failed to present the interview modal in this Chrome; attaching later
worked. Render-path flakiness, root cause not established.)

### A.8 Fallback route: the QUICK path — soft gate + "Quick questions first"

With the deep draft dead, the walk continued through the wizard's "✨ Plan it" quick path
(the docs say it ends in the SAME proposal modal). `POST /api/tasks/wizard`, button honestly
reading *"✨ Planning… (up to ~3 min under load)"*. After ~4 min the **soft-gate triage
modal** appeared (screenshot `w1-06-softgate-recommendation.jpg`), verbatim:

> **✦ This looks complex — plan it properly?**
> A short guided interview builds a structured spec, then drafts and premortem-checks the
> plan. It steers the whole run with a better contract — worth it for complex or ambiguous
> goals. Planning tokens are tiny next to the work.
> **Why:**
> · long, detailed goal (many moving parts)
> · spans ≥2 domains (marketing, design)
> · real-world blast radius (deploy / send / money)
> ✨ Super Result is also suggested for this goal (independent grounded verification).
> [Quick plan anyway] [✦ Start Deep Plan]

Two of three reasons are **false for this goal** (nothing deploys, sends, or moves money; no
marketing domain) — keyword-derived triage theater. The decline ("Quick plan anyway") was
telemetry-logged (`POST /api/plan/telemetry`), as the docs promised.

Then a surface the docs study does not contain at all: **"✨ Quick questions first"**
(screenshot `w1-07-quick-questions-form.jpg`). Header understanding statement, verbatim (the
text is served truncated mid-word): *"Understood: /home/sinep/Projects/car-webshop-w1 is
empty (0 files), so this is a fresh v1 build of a BMW/Audi parts webshop (30+ item catalog
with picture/price/info/compatibility, detail view, paymentless cart, modern animated
design, brand+model filter) to be run as the 5-stage dev pipeline — a fe"*. Footer rule:
*"One round only — skipped questions use their default and show up as stated assumptions in
the plan."* Buttons: [Cancel] [Skip all — use defaults] [✨ Answer & plan].

Five questions, each: bold question + one-line purpose + 3 radio options with **explicit +
and − tradeoff lines** + exactly one "★ recommended" pre-selected + "Other…" with free-text:

1. *"Which stack should the webshop be built on?"* — "The project directory is empty so
   nothing pins a stack; this decision defines the whole architecture and how acceptance is
   tested." — ★ FastAPI + SQLite + vanilla JS SPA (*"+ Real REST API + seed script,
   pytest-testable, **matches your Python/FastAPI ecosystem**, clean path to payments later /
   − More moving parts than a static site; needs a server process running"*) · Static SPA ·
   Next.js/React full-stack · Other…
2. *"Where should the 30+ product pictures come from?"* — "Image sourcing changes scope and
   reliability — network/licensing risk vs consistency of the 'modern design'." —
   ★ Generated SVG product graphics (*"+ Deterministic, no licensing or URL-rot risk,
   consistent premium catalog look, verifiable as files / − Not photorealistic"*) ·
   Hotlinked stock photos (Unsplash/Pexels) (*"− License ambiguity, links rot…"*) · Locally
   downloaded CC0 photos · Other…
3. *"Which UI language(s) for v1?"* — "Content language changes every seeded string, filter
   label and detail page; cheaper to fix before the SPEC than after." — ★ English only
   (i18n-ready strings) · German only (*"+ Fits the local Munich / BMW-Audi buyer base"*) ·
   Bilingual DE/EN toggle (*"+ …mirrors the bilingual pattern from **your restourante
   project**"*) · Other…
4. *"How deep should the BMW/Audi model compatibility data go?"* — "…the filter is only as
   good as the compatibility matrix behind it — this sets seed-data scope." — ★ ~4 models
   per brand (BMW 3er/5er/X3/X5, Audi A3/A4/A6/Q5) · 8+ models incl. generations
   (E90/F30/G20, 8V/8Y) · Free-text compatibility strings only (*"− Weakens the required
   brand+model filter to plain text search"*) · Other…
5. *"What quality bar must 'modern design' pass in acceptance?"* — "This decides whether the
   acceptance stage runs hard audit gates or only functional checks — **it changed a past
   webshop verdict**." — ★ Strict: Lighthouse ≥90 AND no failed audit, WCAG AA contrast ≥4.5
   (*"+ Matches your evaluation discipline; a composite 95 can hide a failed color-contrast
   audit"*) · Lighthouse ≥90 only (*"− Hides individual audit failures — **the exact
   false-pass you hit before**"*) · Feature-correct only, no audits · Other…

**The quick path out-asked Deep Plan.** These five are precisely the concrete,
assumption-preventing, recommendation-carrying questions the operator's r5 bar demands
(stack, image sourcing = the praised `assets_media` class, language, data depth, quality
bar), each grounded in the repo, the operator's ecosystem, and past project history.

Answered with all five ★ defaults → second `POST /api/tasks/wizard` (~7 min).

### A.9 The proposal modal and the produced tasks (STOP point)

Screenshots `w1-08-proposal-summary-assumed.jpg`, `w1-09-proposal-stages-3-5.jpg`. Title:
**"✨ — proposed project: car-webshop-w1 v1 working round"**. Content, top to bottom:

- **Summary brief** (one paragraph): *"Deliver fully-working v1 of the BMW/Audi car
  replacement + tuning parts webshop in /home/sinep/Projects/car-webshop-w1: FastAPI +
  SQLite + vanilla-JS SPA, >=30 seeded products each with generated SVG graphic / price /
  info / compatibility (~4 models per brand), item detail view, payment-free cart,
  brand+model search filter, modern design with basic animations, English-only i18n-ready
  UI, passing the strict bar of Lighthouse >=90 in every category with zero failed audits
  and WCAG AA contrast >=4."* (goal + all five answers, restated as one contract).
- **"Assumed:" block** (amber, front and center) — 8 concrete assumptions, each with its
  consequence, incl.: repo shows 0 tracked files → *"agents verify the on-disk state
  first"*; cart is client-side, no PII → *"therefore no extra high_stakes flag on the
  implementation stage"*; no web-researcher prep task (established stacks); exact model
  lists fixed by the tech-lead in the SPEC, brief examples are suggestions only; *"'Basic
  animations' is operationalized in the SPEC as >=3 named concrete effects (suggested:
  hover feedback, add-to-cart feedback, list-to-detail transition)"*; quality gates run
  headless against the locally served SPA, *"contrast is measured on computed colors of the
  actual rendered UI, not on palette constants"*; SVG graphics are static per-product assets
  from a deterministic generator step; *"Priority 1 (high) for the whole project, glm-5.2 on
  all five stages, stage budgets at the template defaults."*
  Then: *"Wrong assumption? Cancel and rephrase — or ✏️ edit the affected task right here."*
- **Auto-fix disclosure**: *"🔧 wizard auto-fixed: implementation now waits for the spec
  stage (missing dependency restored) · code review now waits for every implementation task
  · fix task now waits for every review · acceptance verification now gates on review +
  every open task · spec stage now owns the acceptance/test suite (tests-first) · verifier
  now checks acceptance-file hashes before running them"* — every silent repair the
  deterministic checker made, disclosed in one line.
- **Five stage cards**, each: checkbox (optional stages tickable; gate stages 🔒 locked) ·
  number · title · specialist chip · domain tags · ✏️ edit pencil · dependency line
  ("⛓ waits for: 1, 2") · brief excerpt. Stage 1 Spec & plan (tech-lead-orchestrator) →
  2 Implement + tests (code-implementer, waits 1) → 3 Code review (code-reviewer, 🔒 gate,
  waits 1,2) → 4 Fix review findings (waits 2,3) → 5 Acceptance verification
  (acceptance-verifier ⚖, 🔒 gate, waits 1,3,4 — *"re-verify the SHA-256 of every
  acceptance/ file against…"*). Plus "➕ Add a task" and *"✏️ edit any task — the plan
  checker re-verifies edited plans (quality gates, wiring) before anything is created."*
- **Knobs**: repo picker (correctly pre-bound to `/home/sinep/Projects/car-webshop-w1`;
  *"Pick a repo and every coding stage works INSIDE it: isolated branch, the repo's own
  conventions and tests, and a reviewable diff as the deliverable. Your checkout is never
  touched."*) · Client scope (memory isolation) · ⚖ High-stakes toggle (frontier judge) ·
  ✨ Super Result · 🎚 Autopilot (Full Auto / Assisted / Manual, plain-words each) · spend
  (🌱 Eco *"(Heuristic — not yet benchmark-validated.)"* / ⚖ Balanced *"(Heuristic — Phase-8
  benchmark pending.)"* / 🧠 Smart "~2× the fuel") with *"🧭 Suggested for this goal: 🧠 Smart
  · real-world blast radius (deploy / send / money) — maximum verification pays for itself
  here"* — the same FALSE keyword reason now driving a max-spend recommendation ·
  🔁 Looping (plain-words explanation, "recommended — this project has a final inspection
  stage") · 📎 Attachments (drag & drop, per-file target auto-suggested).
- Footer, verbatim: *"Tasks are created in Backlog so you can fill any [brackets] first.
  Move task 1 to Todo to start the chain."* — [Cancel] [Create project (5 tasks)].
- **No premortem appears anywhere on this quick-path proposal** (no "Premortem…" /
  "Advisory" text in the DOM) — the premortem belongs to the deep path only.

**Create** fired `POST /api/loop/design` + `POST /api/workflows` + 5× `POST /api/tasks`.
Result (screenshots `w1-10-created-workflow-card.jpg`, `w1-11-board-5-tasks-backlog.jpg`):
workflow `wf-162c99dc` "CAR-WEBSHOP-W1 V1 WORKING ROUND" (active, 0/5 done, backlog: 5) and
five tasks ALL in Backlog, unassigned, nothing executing. The produced Spec&plan task's
stored description opens: *"Read-only spec round for the car-parts webshop at
/home/sinep/Projects/car-webshop-w1. Git reports 0 tracked files — FIRST verify the actual
on-disk state read-only … **Operator-fixed decisions, do not re-litigate: FastAPI + SQLite
backend, vanilla-JS SPA frontend, English-only UI with i18n-ready strings (all copy through
one central strings module), generated static SVG product graphics.**"* — the interview
answers land in the task contract as binding decisions. **HARD STOP honored here**: no task
was moved to Todo, no Start control clicked, the client repo still has zero files.

Walk residue left in Nexus (for the coordinator): deep-plan session `plan-1fdc8115`
(status `active`, family software, repo car-webshop-w1); workflow `wf-162c99dc` + tasks
`task-2cf7fd4c`, `task-13b4c983`, `task-65535caf`, `task-5513eb02`, `task-acc2a944` (all
backlog). Focus chip restored to `car-webshop-test`. Engine calls made on the operator's
subscription: 1 session-create turn, 1 answer turn, 4 failed drafts, 2 wizard calls,
1 loop-design.

---

## B. The harvest list — one entry per pattern, with architecture verdict

Verdicts per §D.1: the coordinator triages against S06/S15/FC-v1; contested items go to the
operator. "Sinet home" names where the pattern would land in product-map v3.

| # | Pattern (as observed live) | Verdict | Rationale / modification |
|---|---|---|---|
| H1 | **Understanding statement before any question** — the question form opens with "Understood: <concrete repo facts> so this is <reading of the goal>" | **FIT-AS-IS** | Rule C.3 exactly (state understanding, let the user correct); slots into the give-work door's "what it understood so far" header (§3, S06.5 questionnaire — content, no wire change) |
| H2 | **Why-lines that bind a question to its consequence** ("The repo is empty, so nothing answers this yet: static vs backend changes how the 30 items are seeded and how the cart persists") | **FIT-AS-IS** | The rule-C.4 "what breaks if unanswered" bar, live-proven; W2 taxonomy phrasing standard |
| H3 | **Per-option + / − tradeoff lines** under every option | **FIT-AS-IS** | Extends Sinet's labeled options with the effect disclosure C.4 demands; pure form content |
| H4 | **Exactly one ★ recommended, pre-selected, with a personal-history reason** ("matches your Python/FastAPI ecosystem", "mirrors the bilingual pattern from your restourante project", "the exact false-pass you hit before") | **FIT-WITH-MODIFICATION** | Recommendation+why is C.4; but every "your X" claim must be grounded in a citable Sinet memory/registry entry (S09 influence + S13.7 registry; Sinet already blocks uncitable plan citations) — never a model free-association |
| H5 | **Option click = quick-fill, not commit** — fills the answer text, stays editable; free text available on every question ("Other…") | **FIT-AS-IS** | Rule C.5's "own words stays available everywhere"; matches the form-not-chat ruling |
| H6 | **"Skip all — use defaults" + "skipped questions use their default and show up as stated assumptions in the plan" + one-round cap** | **FIT-AS-IS** | Product-map §3 force-proceed made concrete; the Clearance meter and slot machinery stay (D.1) |
| H7 | **The ASSUMED block on the plan card** — every inference listed with its consequence + "Wrong assumption? Cancel and rephrase — or edit the affected task right here" | **FIT-AS-IS** | §3 already promises assumptions front-and-center; this is the content bar (concrete, per-assumption, actionable) |
| H8 | **Auto-fix disclosure line** — every repair the deterministic plan checker made, disclosed on the card ("implementation now waits for the spec stage (missing dependency restored) · …") | **FIT-WITH-MODIFICATION** | Sinet's planner validates server-side (S06.6); render the disclosure from Sinet's own validation record/ledger, never as model-authored prose |
| H9 | **Live editable spec pane beside the conversation** — every slot (required + inferred-optional) as an editable field with ✓/●required badges, visible at all times | **FIT-WITH-MODIFICATION** | The W3 centerpiece, operator-ruled wanted; must be re-expressed through Sinet's plan/spec artifact + replan/amend wire under ledger immutability (D.1 explicitly bars free-text mutation of a served plan). The ✓/●required badge grammar carries as-is |
| H10 | **Optional slots inferred, never asked** — only genuine gaps get questions | **FIT-WITH-MODIFICATION** | The mechanism that keeps question count low at zero transparency cost — but it is also WHY Nexus under-asks (B.5). Sinet keeps gap-finding as the interview's point (C.1): inferred slots surface as confirmable assumptions counted by the Clearance meter, not as silently "filled" state |
| H11 | **Readiness advisory, never a gate** — "✓ Required slots filled — ready to draft"; Draft always enabled | **FIT-AS-IS** | Matches escape-by-default + force-proceed-with-assumptions posture |
| H12 | **Decisions ledger → task contract** — answers accumulate in a Notes/decisions slot; produced briefs open with "Operator-fixed decisions, do not re-litigate: …" | **FIT-AS-IS** | Decision provenance traveling into execution; Sinet's spec already travels (S06.6) — the explicit do-not-re-litigate phrasing is the carry |
| H13 | **Soft-gate recommendation modal** — "This looks complex — plan it properly?" + plain reasons + zero-friction decline (logged) | **FIT-WITH-MODIFICATION** | The recommend-the-heavier-path pattern fits the door's stakes/size guess; but reasons must come from real, grounded signals — live run showed keyword-false reasons ("deploy / send / money" on a paymentless goal). Honesty invariants bind: state only reasons Sinet can defend |
| H14 | **Duration-honest progress labels** — "✨ Planning… (up to ~3 min under load)" | **FIT-AS-IS** | Every long-running Sinet control should state expected duration; contrast the deep path's bare "Drafting…" |
| H15 | **Failure swallowing** — four 502 draft failures with zero UI signal (button silently re-enables) | **REJECT** (counter-example) | Direct violation of Sinet's honesty invariants (§3 cross-cutting: stale never poses as live; absent data gets honest absence). Carries into W4 as the error-surface requirement: every failed engine call = plain-words failure card + retry |
| H16 | **Repo grounding quoted inside planning surfaces** — "Git reports 0 tracked files — FIRST verify the actual on-disk state read-only"; empty-repo fact corrects the wizard's own "improvement round" framing | **FIT-AS-IS** | Sinet already plans over the project snapshot + registry (S06.6/S13.7); the pattern is SHOWING the concrete grounding facts to the user in door + plan card |
| H17 | **Family switch re-templates the spec, silently discarding prior answers** (only the universal notes slot survived; stale question cards remain) | **REJECT** | Contradicts Sinet's no-silent-data-loss/ledger posture; confused even this walk. Any family-override surface must preview keep/lose and re-run gap analysis |
| H18 | **Silent family/domain guess** — the misclassification (Content/marketing for a webshop build) was visible only as a header chip; wrong-template questions followed | **REJECT** (as silent behavior) | Sinet's door already shows the stakes/size guess; the family/task-shape guess must be shown and cheaply correctable BEFORE questions render, not discovered through wrong questions |
| H19 | **Gate stages locked, optional stages tickable** on the plan card | **FIT-WITH-MODIFICATION** | Already ruled in product-map §3: "untick optional stages" carries as Re-plan's structured entry (tap the contested step); the live walk confirms the other half — verification gates are never untickable |
| H20 | **Dependency chains on stage cards ("⛓ waits for: 1, 2") + hand-edits bounce through the plan checker before creation** | **FIT-WITH-MODIFICATION** | Sinet's plan card gains the waits-for line (content); hand-edits must round-trip through Sinet's planner validation via the Re-plan wire — never direct artifact mutation |
| H21 | **Create is inert** — "Tasks are created in Backlog … Move task 1 to Todo to start the chain"; explicit Start controls | **FIT-AS-IS** | Matches approve-before-spend; the plain sentence saying what will NOT happen yet is the carry |
| H22 | **Autopilot/spend dials with honest unvalidated-heuristic labels** ("Heuristic — not yet benchmark-validated", "Phase-8 benchmark pending") + a suggested tier with a reason | **FIT-WITH-MODIFICATION** | If/when Sinet surfaces per-task autonomy/spend dials (S07 budgets exist), the honest not-yet-measured labeling carries; the suggestion reason must be grounded (see H13 — here the same false "money" keyword drove a max-spend suggestion). Scope = coordinator call, possibly v1+ |
| H23 | **The five webshop questions themselves** (stack / image sourcing / UI language / compatibility-data depth / acceptance quality bar), each with purpose+options+tradeoffs+recommendation | **FIT-AS-IS** (as content) | Direct W2 taxonomy input: reference-grade question set for the software/webshop family, at the operator's `assets_media`/`look_feel` bar. Not code — question content and phrasing standard |

**Counts: 12 FIT-AS-IS · 8 FIT-WITH-MODIFICATION · 3 REJECT (23 entries).**

## C. Deltas vs the docs study (`gf3-nexus-deepplan-study-2026-08-23.md`)

1. **The "Quick questions first" surface does not exist in the docs study at all** — and it
   out-performed the documented Deep Plan interview on the operator's own quality bar (5
   concrete questions vs 3 template-shaped ones). The study's framing "Deep Plan = the
   scaffolded interview" is inverted live: the quick path asked the better questions.
2. **Draft failure has no UI surface.** The study says "NOTHING blocks: … Create is enabled
   throughout" but never covers the failure path: live, four 502s (max_seconds cap ~5–7
   min; "draft did not converge") were swallowed with zero feedback, losing four paid runs.
3. **Family assignment can misfire and is nondeterministic across phrasings** — the same
   goal substance produced family=software (operator, 2026-08-23) and family=content (this
   walk). The study documents keyword triage for complexity but not family misassignment or
   its blast radius (wrong template → wrong questions → wrong spec shape).
4. **Family switch nuance:** the study lists "family switch silently discards prior answers";
   live adds: the universal Notes/decisions slot SURVIVES, the already-rendered question
   cards stay (stale slot tags), and the switch is one `PATCH /spec` with no model turn.
5. **The premortem was unobservable** — the deep path died before its proposal, and the
   quick-path proposal contains NO premortem (confirmed by DOM scan). The study's premortem
   description (steps 7–8) therefore remains docs-only, unverified by live evidence.
6. **`plan.*` settings knobs are not in the served `/api/settings`** of the running build
   (the study cites settings_registry.py:290-344); the draft time cap behaves as a code
   constant. The running instance ("NEXUS v3.0" at :8777) differs from the studied source
   tree — the studied `~/nexus-agent-os` paths are not what serves this UI.
7. **Latency reality:** turn calls ~60–90 s, drafts 4.5–6.5 min, quick wizard ~4–7 min,
   nothing streams — each step is one long blocking POST (the study's "questions are on
   screen the moment the modal opens" describes the resume case, not the wait).
8. **Deep Plan hand-off flakiness in real Chrome:** twice, the wizard→Deep Plan click
   created the session server-side but never presented the interview modal; attaching later
   (resume path) worked. Not in the study.
9. **Confirmed exactly as documented:** resume-instead-of-duplicate on identical goal;
   option-click = append-to-textbox (semicolon-joined), one Send per turn; "✓ answered"
   dimming; no conversation scrollback (latest message only); readiness line advisory;
   optional-slots-inferred-never-asked; assumptions as stated plan content; decline
   telemetry on the soft gate; tasks created into Backlog.
10. **Small served-content defects not in the study:** the understanding statement arrives
    truncated mid-word ("… — a fe"); the Mandatories/taboos list slot renders as a raw
    Python-list literal (`['MUST: …']`).

## D. Nexus weaknesses observed live (incl. where it under-asks)

1. **Under-asking, quantified:** the deep interview asked 3 questions — all shaped by the
   (misassigned) content template (audience / runtime-form / core promise) — then declared
   ready. Never asked on either path: **currency/price format** (the operator's r5 round-4
   case — never surfaced at all here), seed-data realism (real vs fictional part names),
   tuning-vs-replacement catalog split, responsive/mobile targets (self-decided
   "desktop-first" in the deep spec vs mobile+desktop Lighthouse in the quick brief —
   the two paths silently disagree), animation specifics (self-decided "≥3 named effects"),
   accessibility beyond contrast. The quick path asked 5 good questions but still one round
   only, no follow-ups on answers.
2. **Silent failure swallowing** (H15): the worst defect seen — four lost paid drafts with
   the UI pretending nothing happened. The error text itself ("run may finish orphaned
   (recover via transcript)") is jargon aimed at nobody.
3. **Triage/keyword theater:** false reasons ("real-world blast radius (deploy / send /
   money)" on a paymentless local build; "spans ≥2 domains (marketing, design)") drove BOTH
   the soft-gate recommendation AND a max-spend ("🧠 Smart … maximum verification pays for
   itself here") suggestion — a false positive that would roughly double spend if followed.
4. **Family misfire cascade** (H18/H17): wrong family → wrong template → wrong questions;
   the only cue is a small header chip; correcting it silently destroys inferred content.
5. **Blocking multi-minute calls with thin progress:** deep path shows only a disabled
   button ("Drafting…"), stale planner text, no elapsed time, no cancel; the quick path at
   least states "(up to ~3 min under load)" — then took 4–7.
6. **No conversation scrollback** (confirmed): after turn 2 the turn-1 planner reasoning is
   gone; the spec pane is the only memory.
7. **Cosmetic/content leaks:** raw list-literal rendering in a spec slot; truncated
   understanding statement; sessions-quota header ("0/8 …") meaningless to a requester.
8. **Two-path inconsistency:** deep and quick paths derive conflicting specs from the same
   goal (desktop-first vs mobile+desktop gates; static-app recommendation vs FastAPI
   recommendation ~40 min apart) — whichever door the user picks silently changes the
   product's shape. (For Sinet: one interview machinery, one spec derivation — already the
   S06 posture — is the fix; worth stating as evidence for why the merge keeps Sinet's
   single pipeline.)

## E. Walk hygiene record

- Hard stops honored: no execution started (all 5 tasks left in Backlog, unassigned; Start
  controls untouched); `~/Nexus-Agentic-Coding-Setup` and `~/nexus-agent-os` untouched;
  no credentials encountered; `~/Projects/car-webshop-test` untouched (verified focused
  session list only read-only).
- `/home/sinep/Projects/car-webshop-w1` verified empty after the walk (0 working files; only
  `.git` index mtime moved — consistent with Nexus's read-only repo scanner).
- Focus context restored to `car-webshop-test`.
- Screenshots: 11 files under `P3/design/rework-screens/w1-nexus/` (w1-01 … w1-11).
- Unreachable live (documented via docs study only): the deep-spec-bannered proposal, the
  premortem, the automatic revision round + revision questions, SPEC.md/spec.json
  attachment hand-off.
