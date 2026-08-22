# Operator findings — B6 gate click-through, round 3 (2026-08-23, post-GF2)

The operator resumed the sitting, created a project ("Carpart Webshop") and a task, and hit the
planning/interview phase. Verdict (authoritative, free text): the interview is generic, jargon-loaded,
unintuitive, gives no recommendations, wastes time, and is a large downgrade from the predecessor's
Deep Plan flow; the plan verbs are too poor to correct a bad plan; the local model appears unused
(no GPU load); and the first run parked on a verification wall no click can answer. Ordered: a proper
rework of the planning phase — researched against state-of-the-art practice AND the predecessor's
implementation (`~/nexus-agent-os`, operator-ordered study) — smart, guided, operable by a non-IT
user, coherent with the backend, **without touching backend core logic**.

## Findings (with coordinator investigation)

**F1 — The interview questions are raw benchmark catalog text, incomprehensible to the requester.**
Reproduced in code: the first software-family card = the top-4 slots by weight — `collection_semantics`
(12), `comparison_rules` (12), `ordering_atomicity` (12), `technology_stack` (11) — served with the
ClarifyCodeBench Table-2 wording verbatim (`internal/intake/taxonomy.go:231,236,241`;
selection `internal/intake/pipeline.go:715-731`, `maxQuestionsPerCard=4` at `cards.go:108`).
The three weight-12 texts are the benchmark's own phrasing ("collections/state involved",
"deduplicated… ties broken", "atomicity… interleave") — measured for *model* failure, never worded
for a human non-programmer. **Spec check: S06.5 question delivery mandates "up to 4 questions per
card, each with 2–4 labeled options plus free text" — the three weight-12 slots carry NO options at
all** (only edge_cases/indices/output_format and the three RW-12 Deep-Plan slots have options).
The catalog under-delivers the spec's own stated shape. The operator's positive note on the tech
question ("has at least some suggestions") is exactly the one slot built the way S06.5 says all
should be. Taxonomies are S06.5 knowledge objects — versioned, drafted at implementation time,
8.3-gated — so a v3 revision (plain-language texts + 2–4 options per slot + recommended defaults)
is the spec's own amendment-free path, ratified at the packet/gate like RW-12's v2.

**F2 — No task-specific phrasing and no GPU use: the gate world runs with the local tier NEVER
WIRED.** The operator's world log (`~/.sinet-b6-clickthrough/control.log`) says it at startup:
`local: S12 duty surface wired — configured=false`. `P3/gates/B6-clickthrough.sh` never exports
`SINET_LOCAL_*` before starting the control plane, and the walkthrough never says to run
`./P3/gates/local-stack.sh` first. Consequence: the PH-1 phrase seat (proven live at 6.6 s/card)
had no engine — every question fell back to canonical catalog text (the honest
"standard wording throughout" line rode the card, `web/src/Intake.tsx:1319-1323`), classification
and help seats likewise degraded, and the GPU correctly showed zero load. **The operator has been
gate-testing a degraded build through all three rounds.** Not a product bug — a gate-harness gap;
the product's degrade behavior worked as designed (loud in the log, honest on the card).

**F3 — No recommendations/suggestions at the questions.** Nothing proposes an answer: options are
static catalog labels (where they exist at all), no per-question suggested default, no task-grounded
proposal. The planner's proposals DO exist — but only after force-proceed, as assumptions on the
plan card ("what you skip, it will assume out loud"), one screen too late for the person deciding.
S06.9's 13.5 help block already mandates "what the platform recommends and why … at non-IT reading
level" on decision cards — the interview card simply never got that treatment. S06.5's duty split
("the utility model phrases and summarizes but does not decide what must be asked — the taxonomy
carries the coverage") leaves suggestion decoration legal without touching question selection.

**F4 — Round mechanics read as a loop/bug.** Answer one of four, send → the next card re-asks the
three skipped (by design: highest-weight unresolved first, clearance below floor). The send button
disables at 0 answers with the force-proceed arm below (`Intake.tsx:1380-1424`) — the operator
read the disabled state as "greyed out, can't continue", then found force-proceed and assumed the
rest. Nothing explains the loop's shape (why these questions return, what the floor is, how far to
go) in requester terms; the Clearance number rises but the mechanics are opaque.

**F5 — Re-interview re-issues the same card.** `answer.go:338` sets `ReinterviewRequested`;
`pipeline.go:701-710` then issues ONE full card of the top-weight slots — the same four questions,
unphrased in the degraded world. Spec check: S06.9 says only "Re-interview — returns to Stage 1
with artifacts intact"; the same-card shape is implementation choice, not spec. The operator wanted
a guided per-decision pass over what was settled (change what I want, keep the rest), not the same
form again.

**F6 — Re-plan contests exactly ONE item.** The FE pane holds a single `target` + one note
(`Intake.tsx` PlanCard contest pane), the wire accepts a single `Contest{Target,Note}`
(`answer.go:413-424`) — while the backend's own revise carrier is already plural
(`ReviseReq.Findings []string`, `mergeRevise` merges). Multiple dissatisfactions today mean
serial replan rounds, each a full redraft. Spec check: S06.9's "structured entry: tap the AC,
assumption, or step being contested" names the STRUCTURE (named targets), not a cardinality of one —
a multi-target contest with one free-text note is a compatible reading (coordinator ruling, logged).

**F7 — No manual plan editing.** The verbs are Approve / Re-plan / Re-interview / Cancel — S06.9's
exact set. A raw operator textbox editing SPEC/PLAN would bypass the spine's coverage check and the
critique pass (the artifacts are planner-drafted, delta-re-approved by design) — that would need an
S00.9 amendment and would gut verification's contract. The spec-honest equivalent of "let me say
everything that's wrong, in my words": multi-target Re-plan (F6) + an always-present free-text
"what I want different" channel riding the same contest, producing the bounded delta re-plan whose
delta card shows exactly what changed. Presented to the operator as the recommendation; a true
edit-the-artifact affordance stays available as an amendment decision if they insist.

**F8 — The verification wall: a parked task whose card's instruction has no door.** The run parked
on `verify: launch-domain deliverable without a domain check pack (Spec S07.8)` (`internal/verify/verify.go:229`,
suite-refusal ladder per RW-14 OQ1) because the fresh "Carpart Webshop" project has no captured
commands. The card says "capture them for the project (its Commands), then retry" — **but no
surface can do that**: `Projects.tsx` only displays the capture ("no commands captured", :583),
no write route exists for commands on `/api/projects`, and the rescan HTTP door is on the deferred
ledger. Retry/cancel are the only verbs — retry cannot succeed. This is precisely the scenario the
QUEUED S00.9 amendment ("bootstrap verification posture for command-less first tasks") exists for;
the operator hit it live before the gate could present it. Fix = the amendment (operator decision,
presented at the resumption) + its implementation (backend, four-stage) + the card's real door (FE).
GF2's rule applies: every answerable card carries its real door.

## The ordered rework (round 3 — P3-GF3)

Research first (operator-ordered), then build:

1. **Research inputs** (this session): (a) live web research — SOTA clarification/plan-review UX for
   AI agent platforms, papers + shipping products; (b) full study of the Nexus Deep Plan
   implementation, backend (`plan_engine.py`, server orchestration) and frontend (spec review,
   option rendering, revision flow) — both filed under `P3/design/` as the GF3 evidence base.
2. **Design note** `P3/design/gf3-planning-rework-design-2026-08-23.md`: the target flow, the
   FE/backend split, and the spec readings — coordinator-drafted from the findings + research,
   the build contract for the builder round and any backend micros.
3. **Expected split** (per the 2026-08-05 routing rule): FE builder round under FRONTEND.md
   (interview surface, plan-card verbs, doors, jargon); taxonomy v3 revision (knowledge-object
   drafting, 8.3-gated, operator-ratified); small additive backend micros ONLY where a needed fact
   is unserved (suggestion decoration on question cards; plural contest targets on the replan wire) —
   four-stage ceremony, no core-logic rework (selection/clearance/spine/critique untouched);
   the gate harness fix (local-tier wiring in `B6-clickthrough.sh`, stays uncommitted).
4. **F8** routes through the queued amendment at the gate resumption — not silently implemented.

## What stays binding

- D1–D10; the S06 pipeline structure (clearance floor, spine, critique, approval verbs, delta
  re-approval) is NOT up for rework — this round changes what the cards carry and how the FE
  guides, never how coverage/verification decide.
- Backend core logic untouched per the operator's explicit instruction; additive card-content and
  wire fields only, each through the four-stage ceremony.
- The operator's world `~/.sinet-b6-clickthrough` stays live and untouched; the sitting resumes on
  it after rework + live review + cold walk + battery, now WITH the local tier wired.
