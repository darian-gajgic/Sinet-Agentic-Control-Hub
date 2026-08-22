# P3-GF3 — the planning-phase rework design (2026-08-23)

The build contract for the round-3 gate rework. Inputs: the operator's findings
(`b6-gate-operator-findings-r3-2026-08-23.md`, F1–F8), the Nexus Deep Plan study
(`gf3-nexus-deepplan-study-2026-08-23.md`), the live SOTA research
(`gf3-sota-planning-ux-research-2026-08-23.md`), and the binding spec (S06.5/S06.9/S06.10,
drafts canonical). Constraint (operator, authoritative): **backend core logic untouched** —
clearance computation, floor, slot selection/weights, spine, critique, approval freeze, delta
mechanics, verification, routing, band all stay exactly as landed. Everything below is card
CONTENT, wire ADDITIONS, taxonomy DATA, and FE presentation.

## 0. The north star

A person with no IT knowledge describes what they want, is asked a handful of questions they can
actually answer — each carrying a concrete recommendation they can accept in one click — sees
every decision the platform made laid out plainly, can change any of them (several at once, in
their own words), and approves a short plan they understood. Research consensus and the Nexus
mechanism agree on the shape; the operator's findings name every place today's build misses it.

## 1. What is spec-fixed vs open (the readings, logged)

- S06.5 delivery: "batched option cards — up to 4 questions per card, **each with 2–4 labeled
  options plus free text** — highest-weight unresolved slots first." The batch and the option
  shape are spec; today's catalog under-delivers (three weight-12 slots ship zero options). The
  FE may present the ≤4 questions of one card as a focused sequence (one decision in view at a
  time, one send) — presentation, not selection.
- S06.5 resolution arms: "a slot is *resolved* when registry-supplied, requester-answered, **or
  converted to an explicit assumption**." A per-slot, requester-directed conversion ("skip this —
  take the recommendation as an assumption") is squarely inside this model and strictly more
  conservative than the existing all-slots force-proceed. **Coordinator reading: clearly implied;
  logged.**
- S06.5 duty split: "the utility model phrases and summarizes but does not decide what must be
  asked — the taxonomy carries the coverage." Suggestion decoration (a proposed answer per
  question) does not alter coverage; it is phrasing-adjacent utility-seat output, folded by slot
  id under the same containment as `Phrased`. **Reading: legal; logged.**
- S06.9 Re-plan: "structured entry: tap the AC, assumption, or step being contested; produces a
  bounded delta re-plan." Structure = named targets, not a cardinality of one. A SET of targets
  plus one free-text note preserves the structure. Backend `ReviseReq.Findings` is already
  `[]string`. **Reading: multi-contest compatible; logged.**
- S06.9 Re-interview: "returns to Stage 1 with artifacts intact" — the same-four-questions card
  is implementation choice, replaceable.
- S06.9 gives NO verb for the requester editing SPEC/PLAN text directly, and the artifact
  discipline (spine coverage check, critique, frozen ACs) depends on the planner writing the
  artifacts. **Raw manual editing = amendment territory; NOT built this round.** The operator's
  intent ("say everything wrong at once, in my words") is served by multi-contest + the free-text
  channel; if they still want raw editing at the resumed gate, it becomes an S00.9 decision.
- S06.10 duty table: plan drafting stays on the paid planning model (D3: opus-4-8). Local seats =
  phrasing/help/classification. Unchanged.

## 2. The rework, piece by piece

### A. Taxonomy v3 (data, 8.3-gated — backend packet)

Software family + generic fallback revised to v3: ids and weights **verbatim** (the measured
thing, untouched); every slot gets (i) a plain-language question a non-programmer can answer —
the ClarifyCodeBench texts stay as internal `MustKnow` rationale, never as the asked question;
(ii) **2–4 concrete labeled options** per S06.5 (with an "I'll say in my own words" escape
implied by the free-text field, and where honest a "you choose for me and show me" option — the
RW-12 Deep-Plan slots' proven pattern); (iii) a one-line plain-words why. Drafted with the
strongest frontier model per S06.5, provenance-stamped, **ratified by the operator at the resumed
gate** (RW-12 v2 precedent). Example direction (drafting session decides final texts):
`collection_semantics` → "What things does this keep track of (products, orders, customers…),
and what should happen when the same one shows up twice?"; `comparison_rules` → "When things get
listed or ranked, what order should they be in?"; `ordering_atomicity` → "Does anything here have
to happen strictly before something else, or all-or-nothing?"

### B. Suggestion decoration on interview cards (additive backend)

`PhraseAndSummarize` is extended: for each question the utility seat may also return a
`suggested` — a one-line, task-grounded proposed answer ("For a car-parts webshop I'd suggest:
start with a product catalog with search by car model; duplicates merged by part number"), plus
an optional `suggested_option` naming the option value it corresponds to. Folded in
`buildInterviewCard` by slot id exactly like `Phrased` (question count/order/ids/options remain
untouchable by construction); absent whenever the seat degrades (the existing honest
"standard wording" posture covers the absence — no new failure mode). Served additively on the
wire. PH-1 discipline: the live measurement leg re-checks token headroom on the extended output
(opt-in env, GPU stack, serial).

### C. Per-slot skip — "take the recommendation and move on" (additive backend)

The interview answer body gains a per-question `skip` arm: `{id, skip: true}` converts THAT slot
to an explicit assumption (reusing `resolveSlot`/`ResolvedAssumption` — the S06.5 resolution arm)
carrying the suggested answer when one was served, else the planner's default at draft time.
Clearance rises exactly as the spec computes it; the assumption lands on the plan card's
centerpiece, contestable, origin-labeled. This kills the "it will ask again about the N you
skipped" loop (operator F4) without touching selection: a skipped slot is resolved, so it is
never re-picked. The all-or-nothing force-proceed arm stays for "assume everything".

### D. Re-interview becomes "Change my answers" (additive backend + FE)

The reinterview branch (pipeline.go:701) issues a **review card**: ALL the family's slots, each
carrying its current resolution (your answer / assumed / from the project — the `Resolutions`
state serves this) prefilled, options + suggestions decorated as in B. The FE renders it as a
review-and-adjust surface — settled items shown settled, editable; nothing re-asked blind. Same
card kind, richer content; selection logic for the NORMAL interview path untouched.

### E. Multi-contest Re-plan + the free-text channel (additive backend + FE)

Wire: `contests: [{target, note?}, …]` accepted alongside the existing single `contest`
(back-compat), plus an optional top-level `note` — "what I want different, in my words" — that
becomes a target-less finding. All merge into the existing plural `ReviseReq.Findings`; one
bounded delta re-plan (existing S06.9 mechanics); the delta card shows exactly what changed.
FE: the contest pane becomes multi-select chips over steps/ACs/assumptions + one always-present
textarea; one send. (Nexus's bulk "revise from findings" + the free-text objection channel its
own postmortem shows it missed; SOTA principles 8–10.)

### F. The FE interview surface rework (FRONTEND.md builder round)

- **The contract up front**: the interview intro says what this is — "A few questions, each with
  a recommendation you can take in one click. Skip anything — skipped items become listed
  assumptions you'll see on the plan before anything runs or costs money." (anti-pattern 2/6
  counters; the understood panel keeps delivering value between rounds.)
- **Decision cards**: per question — plain question (phrased when the seat answers), the why
  line, option chips with the suggested one marked ★ recommended, free text always visible,
  a "skip — take the recommendation" affordance, answered/skipped state chips (the Nexus
  "✓ answered" dimming). Within a card the builder may sequence questions one-in-view-at-a-time
  (GOV.UK one-thing-per-page) or grouped — the cold walk decides what a non-IT persona actually
  gets through faster; both are spec-legal presentations of one card.
- **Progress that explains itself**: the clearance meter says what it measures and where it
  stops ("how settled the must-knows are — questions stop at N% for work like this"); floor
  served on the wire (tiny addition if the check finds it unserved).
- **Plan-card verbs**: Approve · "Change the plan…" (multi-contest pane, E) · "Change my
  answers…" (review card, D) · Cancel — plain names, each stating its consequence; disabled
  states keep printing their reason.
- **Jargon sweep** over every intake string (NN/g plain-language; the standing wire-side
  spec-ref G-family gets its intake slice cleaned where FE-owned).
- Builder world runs WITH the local stack wired (export SINET_LOCAL_* from the running stack's
  port file) — building the suggestion/phrasing UX against a dead seat is how F2 happened.

### G. Explicitly NOT this round

- Raw manual SPEC/PLAN editing (amendment decision, presented at the resumed gate).
- F8 (bootstrap verification posture for command-less projects + the Commands door): the queued
  S00.9 amendment is presented at the resumed gate; its implementation is its own four-stage
  packet after operator approval.
- Question-selection changes, information-gain ranking, cross-session answer memory (SOTA
  frontier — candidates for a future amendment round, noted for the record).
- Core pipeline logic of any kind.

## 3. Packets and sequencing

1. **P3-GF3-BE1 — "the guided interview's served substrate"** (four-stage, opus executor,
   tests-first): pieces A + B + C + D-backend + E-backend (+ floor serve if unserved). All in
   internal/intake + internal/local (prompt) + internal/api (serve) — additive; existing tests
   untouched per amendment A. Live phrase-seat leg opt-in, GPU-only, serial.
2. **P3-GF3-FE — the builder round** (FRONTEND.md, single Fable author): piece F on the landed
   BE1 wire. References in context: this note, the Nexus study, the SOTA principles, the served
   fixtures. Then live design review → cold walk (non-IT persona runs the CARPART WEBSHOP journey
   end to end, local stack live, $0) → coordinator battery.
3. Gate resumption: the operator re-runs `./P3/gates/B6-clickthrough.sh` (now local-tier-wired),
   walks the same journey, and rules; taxonomy v3 ratification + the two queued amendments + the
   raw-plan-editing question presented there.
