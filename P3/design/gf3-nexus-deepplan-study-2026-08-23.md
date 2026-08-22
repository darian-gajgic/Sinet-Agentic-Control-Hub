# GF3 evidence — the Nexus Deep Plan mechanism study (2026-08-23)

Operator-ordered study of the predecessor's planning phase (`~/nexus-agent-os`), backend and
frontend, as rework input for P3-GF3. Produced by a fresh-context read of plan_engine.py,
server.py, evals.py, static/app.js, settings_registry.py, and the locked design doc
`DEEP-PLAN-MODE-PLAN-2026-07-10.md` (+ its implementation report). This file is the verbatim
agent report; the GF3 design note cites it.

## 1. The end-to-end flow as the operator experiences it

**Step 1 — Goal entry ("✨ Describe a task" wizard modal).** The operator types the goal in plain
words, optionally picks an existing git repository ("🧬 Existing code repository") and optionally
ticks ✨ Super Result. Two buttons: **"✨ Plan it"** (quick one-shot) and **"✦ Deep Plan"**
(app.js:10765) which starts the interview directly — Deep Plan is never only reachable through a
recommendation.

**Step 2 — Triage and the soft-gate recommendation.** On "Plan it", server-side triage may return
`recommend_deep_plan`; the UI then shows a one-time modal ("✦ This looks complex — plan it
properly?") with up to five plain-language REASONS ("multiple deliverables (api, dashboard,
report)", "real-world blast radius (deploy / send / money)", "draft plans disagreed on size
(3–7 tasks)") and two buttons: [Quick plan anyway] / [✦ Start Deep Plan]. Declining costs zero
friction; both choices are logged as telemetry.

**Step 3 — The interview (two-pane "✦ Deep Plan" modal).** Resumes an existing active session for
the identical goal instead of duplicating. Layout: **left = conversation, right = the live SPEC**.
Header: family chip + repo chip when grounded. Footer: [Abandon] left, [Draft plan →] right; the
intro says "You can edit any slot directly, type freely, switch the plan type, or Draft plan at
any time." Turn 0 runs server-side on the goal itself, so questions are on screen the moment the
modal opens.

**Step 4 — Answering turns.** Each question card: bold question, faint *why* line, a grey tag
naming the spec slot it fills, and 3–5 option buttons with the recommended one prefixed "★" and
accent-outlined. Clicking an option APPENDS its label to the free-text box (semicolon-joined) —
it does not submit; the card dims and shows "✓ answered". One Send (Ctrl+Enter) per turn. On
readiness: "✓ Required slots filled — ready to draft."

**Step 5 — SPEC review and editing (right pane).** One labelled resizable textarea per template
slot, each with a status badge: green ✓ filled / amber "● required" empty-required (amber border) /
muted "optional". List slots edit one item per line. Every keystroke debounces 700 ms into a
PATCH — the operator overwrites anything the model inferred, any time. A family dropdown
re-templates the whole spec.

**Step 6 — Draft.** [Draft plan →] is ALWAYS enabled (readiness is advisory, never a gate). Opens
the same proposal modal the quick wizard uses, bannered "Built from your Deep Plan spec — it will
travel with the project (every task, the critic, the judge)."

**Step 7 — Premortem.** Auto-fires on render. "⏳ Premortem critique running…" → "✓ Premortem done —
N finding(s), M structural note(s). Advisory only." Findings attach as amber "Advisory — problem →
fix" lines UNDER the matching task card, each with its own ✕ dismiss; plan-level findings collect
above with "these checks never reference your board and never block creation."

**Step 8 — Automatic revision (exactly one round).** With findings present, one revision fires
automatically: full corrected plan comes back, changed/added tasks get "✎ changed"/"＋ added"
chips, removals are counted. If a finding needs a human decision, a purple block appears: "🤔 The
revised plan needs N decision(s) from you" — each with option buttons + free text, and
[✅ Answer & revise again].

**Step 9 — Manual review, revision, approval.** Inline task editing (one open at a time), add/drop
tasks (quality gates locked 🔒), [🔍 Re-run premortem] and [🔧 Revise plan from findings] unlimited,
project knobs, then [Create project (N tasks)]. Hand-edited plans bounce through the deterministic
plan checker and pause for a second confirmation if the checker changed anything.

**Step 10 — Create and hand-off.** SPEC.md + spec.json land as MUST-READ attachments in the
workflow; criteria embed per task; the judge and critic read the same spec; replan seeds from it.

## 2. Question mechanics, exactly

- **The MODEL generates the questions; a CATALOG defines the structure.** `interview_framing`
  (plan_engine.py:482) enumerates the family's spec slots (key, text/list, REQUIRED/optional,
  hint) and demands JSON-only replies shaped
  `{message, spec_updates, questions:[{id,question,why,slot,options:[{label,recommended}]}], ready}`.
  No hard-coded question texts exist; slots are the catalog, the model phrases to fit the goal.
- **Grounding:** (a) the goal verbatim; (b) an optional repo-context block ("the goal is a CHANGE
  to this existing work… never re-ask what the context already answers, ask how the change
  integrates"); (c) the CURRENT SPEC STATE re-sent every turn plus "EMPTY REQUIRED SLOTS: …" —
  which makes re-asking directly-edited slots impossible.
- **Caps:** `plan.max_questions_per_turn` default 3 (1–6); `plan.max_turns` default 3 (1–8);
  `parse_turn` hard-clamps to 6 questions / 5 options regardless of model output.
- **Options:** "Each question offers 3–5 concrete options; mark EXACTLY ONE recommended:true (the
  best-practice default). The operator may also answer freely." parse_turn:559 promotes the first
  option if none is starred.
- **Optional slots are inferred, never asked** — "infer a sensible default into spec_updates,
  don't ask"; they appear filled (✓) in the pane where the operator can override. Plus: "Never
  invent facts the operator must decide (names, prices, dates) — ask or leave the slot for them."
- **Readiness** is deterministic server-side (`required_filled`: every required slot non-empty);
  the model's ready flag OR the server's computation flips the green line; it gates NO button.
- **Templates** (SPEC_TEMPLATES :346): software — required goal, stack/platform,
  acceptance_criteria(list); optional users, constraints, data/integrations, out_of_scope, risks,
  notes. Equivalent shapes for analysis-audit / content / research. A universal "Notes / decisions"
  slot persists revision answers.
- **Triage** (:120,:228,:258): deterministic 0–10 complexity from goal features, each contributing
  a human-readable reason string; ambiguity raised only by DIVERGENCE (N=2–3 cheap draft plans on
  the easy model, compared arithmetically: token Jaccard 0.45 + task-count spread 0.35 + DAG-shape
  0.20), sampled only in the uncertain band 3–7, in a daemon thread, cached per goal hash. NO LLM
  self-rating anywhere in triage.

## 3. Spec/decision review mechanics

The right pane renders EVERY slot — filled and empty, required and optional, answered, inferred or
hand-typed — as the single always-visible review surface; there is no separate "decisions I made"
screen because the spec pane IS that screen, updated after every turn. Multiple choice exists only
at the moment of asking; once an answer lands it is free text forever. Changing any decision: (a)
direct edit in the pane (700 ms debounced PATCH through `merge_spec`, no model call; deliberately
no re-render — "would eat the cursor"); (b) tell the interviewer in the conversation box ("Fold
everything the operator tells you into spec_updates" + current-spec-state-per-turn makes manual
edits established fact). Textareas everywhere (a recorded operator finding: one-line inputs hid
long auto-filled values). Family switchable. NOT reviewable: the rendered SPEC.md is returned but
never displayed; the conversation shows only the LATEST assistant message (no scrollback).

## 4. Plan revision mechanics

Five affordances, all in the proposal modal: (1) hand-edit any task inline, unlimited (title,
description, specialist, model, budget, stakes, deps; add/drop; gates locked); (2) dismiss
advisories individually; (3) re-run the premortem on the current edited plan, unlimited; (4)
**Revise from findings — the bulk channel: EVERY current finding is sent at once** (numbered list +
full plan JSON + the SPEC contract → "FULL corrected plan, every task, not a diff"); premortem
self-caps at 8 findings, validator at 20 warnings; manual rounds unlimited, the AUTOMATIC round
capped at exactly one; (5) answer the revision's ≤3 questions (2–4 options, exactly one
recommended) via [Answer & revise again] — answers persist into the spec's notes slot AND ride the
revise body labelled "OPERATOR DECISIONS (treat as binding)".

Revisions re-run the same deterministic pipeline as a fresh draft; `_preserve_task_fields`
restores operator-owned dials (model, budget, deliverable_type, autopilot, spend) outright even if
the model rewrote them — "a revision turn's mandate is the plan's content, never the operator's
dials"; risk flags can be raised, never dropped. `_distribute_criteria` appends any uncovered
acceptance criterion verbatim onto the best-matching task (repairs, doesn't nag). NOTHING blocks:
findings are advisory by construction, Create is enabled throughout; the only forced pause is the
plan checker's second confirmation after hand-edits.

**Honest gap:** there is NO general free-text "here is what I dislike, redo it" box in the plan
editor — the operator's unprompted objection has home only in hand-edits or abandoning. And there
is NO path back from the plan editor to the interview/spec pane after drafting (a UI gap — the
session's spec is still PATCHable via the API).

## 5. The design doc's rationales (DEEP-PLAN-MODE-PLAN-2026-07-10.md, locked §3)

- **Ask-gated + capped** — ClarifyGPT (FSE'24): clarification helps only when triggered by
  detected ambiguity; Ask-before-Plan (EMNLP'24): most tasks need 0–3 questions, over-asking adds
  friction without quality; a clarification-options study found 3–5 options beats 2.
- **Scaffolded interview** — interview-style elicitation is preferred over forms, but free-form
  LLM interviews miss more than half of implicit requirements; the per-family template drives
  coverage, "conversation fills slots, not vibes".
- **External premortem** — Valmeekam & Kambhampati: LLM self-critique of plans actively DEGRADES
  them (false-positive approvals); external/structural verifiers work (LLM-Modulo, VerifyLLM);
  hence deterministic DAG checks + one critique by a different, stronger model, never the planner
  grading itself.
- **No LLM self-rating in triage** — sample-disagreement (AutoMix/BEST-Route) is the reliable
  complexity signal; "rate this 1–10" self-assessment is not.
- Soft gate because plan-first only beats ReAct on dependency-heavy tasks (quick path stays
  default); persistent on-disk spec because every comparable product converged on it (Spec Kit,
  Cursor, Claude Code plan mode).
- Honest economics: $0.10–0.50/goal; explicitly won't fix "mid-execution drift" or "goals the user
  themselves can't specify — the interview surfaces requirements, it doesn't invent intent";
  the deep-vs-quick benchmark (step 11) was NEVER RUN — the quality claim rests on literature.

## 6. What is genuinely good / weaknesses

**Good:** deterministic scaffold + generative phrasing (the interview cannot wander off-catalog and
cannot declare readiness the server disagrees with); optional-slots-inferred-never-asked (THE rule
that keeps three questions instead of nine, at zero transparency cost); the live spec pane as a
permanent complete editable state display beside the conversation (interview and artifact are the
SAME screen); options as quick-fill-not-commitment (click ★, edit the text, send once; "✓
answered" dimming tracks coverage); external structural advisory-only verification mapped onto the
exact task card; the revision loop that converts criticism into a corrected artifact with a diff
(the 2026-07-12 postmortem: annotation-only produced "a wall of ⚠ warnings instead of a usable
plan" — the fix is the difference between a critic and a collaborator); auto-round capped at one,
manual unlimited; operator dials survive revisions; coverage repaired not warned; the contract
travels to execution/judge; honest unmeasured-win disclosure.

**Weak:** no way back from plan editor to interview (UI gap); no free-text objection channel into
revision (looks like oversight — notes already rides the body); the SPEC invisible at approval
time; no conversation scrollback; answers ride ONE concatenated string so slot attribution is
model-trusted (per-question {slot: answer} binding is the clear successor improvement); a 700 ms
debounce race can draft against a stale spec; family switch silently discards prior answers;
parse_turn permits multiple ★; single-task deep plans get NO premortem; recommendation offered
once per goal per page load; divergence usually arrives too late to inform its own
recommendation; telemetry written, never consumed; English-keyword heuristics (recorded real
mis-recommendation on a German goal); the premortem is an opaque blocking subprocess (600 s, no
progress, no cancel).

## 7. Key references

plan_engine.py: SPEC_TEMPLATES :346 · interview_framing :482 · parse_turn :537 · required_filled
:442 · triage :120/:228/:258. server.py: _plan_turn_message :8141 · _plan_repo_block :8156 ·
_validate_plan :8411 · critique :8555 · draft :8649 · _preserve_task_fields :8754 ·
_clamp_plan_questions :8787 · _plan_revise_raw :8810 · attach :9046. evals.py run_plan_critique
:1179. app.js: deepPlanModal :11529 · ConvoHTML :11574 · SpecHTML :11599 · slot PATCH :11641 ·
proposeWorkflowModal :11237 · deepPlanRevise :11765 · RenderQuestions :11812.
settings_registry.py:290-344 (plan.* knobs). Design doc:
~/Nexus-Agentic-Coding-Setup/docs/archive/DEEP-PLAN-MODE-PLAN-2026-07-10.md + IMPLEMENTATION-REPORT-DEEP-PLAN.md.
