# P3-GF7 — Interview taxonomy v4 (the W2 merge)

**Grounding brief, 2026-08-27.** Backend four-stage packet. The operator benchmarked Sinet's
interview live against the predecessor and ordered a taxonomy rebuild merging the best of both.
This brief is self-contained; prior briefs are EXPIRED and are not truth. Truth is: the code
(read at commit `19f80c4` + unpushed landings), the frozen spec drafts, and the operator records:

- `P3/design/b6-gate-operator-findings-r5-2026-08-23.md` — §A per-slot verdicts (slot ids are
  `internal/intake/taxonomy.go` ids), §C the SEVEN HARD RULES (the acceptance bar), §D the W2
  order, §D.1 the harvest constraint (pattern harvest only), §E coordinator notes.
- `P3/design/b6-gate-operator-findings-r4-2026-08-23.md` §F2 — taxonomy pass driven by real
  transcripts; folds into the pending v3 ratification ("a v4 revision is the same machinery").
- `P3/design/w1-nexus-live-harvest-2026-08-27.md` — H1/H2/H3/H10 ratified patterns, H18
  counter-example, H23 reference question set, §D.1 what Nexus under-asked.
- Spec: S06.5, S06.6 (incl. A15), S06.7(a), S06.9, S06.10 (`Spec/drafts/S06-intake-pipeline.md`),
  S09.10/S09.4/S09.8 (`Spec/drafts/S09-memory-knowledge.md`), S00.9 row A15.

The escalation stake (r5 header) binds prioritization: this is the planning surface's second
consecutive failed round; the operator is considering replacing the frontend wholesale.

---

## 1. Where the build stands (code facts, verified this session)

- **The software set is v3, 13 slots, total weight 125** (`internal/intake/taxonomy.go:231-409`):
  behavior 10 · terminology 10 · edge_cases 10 · collection_semantics 12 · comparison_rules 12 ·
  ordering_atomicity 12 · indices_ranges 8 · output_format 8 · units 6 · numerical_precision 6 ·
  technology_stack 11 · assets_media 10 · look_feel 10. Delivery order (weight desc, stable):
  card 1 = collection/comparison/ordering/stack (47), card 2 = behavior/terminology/edge/assets
  (40), card 3 = look/indices/output/units (32), card 4 = numerical_precision (6).
- **Slot vocabulary** (`taxonomy.go:36-47`): ID, Name, MustKnow (planner-read, never shown),
  Weight, Question, Why (requester-facing reason line), Options (Label+Value only — no effect
  line, no recommended marker exists today). `Validate` (`:59-85`): unique ids, weight > 0,
  Question required, 0 or 2–4 options. `LoadTaxonomy` (`:133-148`) is strict JSON
  (DisallowUnknownFields) — new fields must land in the struct or governed files stop parsing.
- **Delivery** (`pipeline.go:711-814` `phaseInterview`): familyGate first (`:716`), then
  Clearance over `resolvedSet()`, then while `Clearance < floor && len(unresolved) > 0` the top
  ≤4 unresolved-by-weight ship as a card (`:741-749`). Band/force-proceed convert ALL unresolved
  slots to `ResolvedAssumption` (`:764-783`); a floor reached leaves the unresolved tail to the
  planner's own listed assumptions. `reviewQuestions` (`:904-921`) re-presents EVERY slot of the
  set on the S06.9 Re-interview card, resolved ones carrying `Question.Resolution`.
- **Resolution vocabulary**: `SlotResolution{How: ResolvedRegistry|ResolvedAnswered|
  ResolvedAssumption}`; the per-slot skip arm (`answer.go:176-197`) converts one slot to
  `ResolvedAssumption` with `skipAssumption` prose; `Assume` entries (`answer.go:204-208`)
  accept any slot of the active set. `acceptEmission` (`pipeline.go:1360-1400`) GUARANTEES every
  assumption-kind resolution lands on the SPEC as `Assumption{Origin: "slot:<id>"}` — and keeps
  the planner's own text when the planner already listed that origin.
- **Per-goal phrasing — the proven mechanism** (r5 §E cites look_feel round 3): the utility seat
  (`intake.go:404-464`) returns `Phrasings` (question rewording), `Suggestions` +
  `SuggestedOptions` (one-line task-grounded proposed answer naming an existing option value),
  folded by ASKED slot id in platform code (`cards.go:54-86`, `pipeline.go:949`); option labels
  are context only, never rewritten (P3-RW-12 OQ4). Absent seat = byte-identical taxonomy words.
- **Family**: resolved zero-touch by the classifier on the common path (P3-RW-13; the requester
  never sees it); `familyGate` asks ONLY when `FamilySourceDefault` (`pipeline.go:1025-1046`).
  `State` records `FamilySource` (`state.go:99-102`; vocabulary `intake.go:91-100`) and
  `TaxonomyID/TaxonomyVersion` (`pipeline.go:457`) — but the interview `Card` (`cards.go:134-160`)
  serves NO family member: the guess is invisible on the cards, and no wire exists to correct a
  classifier-guessed family mid-interview (`applyFamilyAnswer` serves only the unresolved case).
- **Governance**: production runs the in-code seed (`pipeline.go:149-154`; no shell wiring of
  `Pipeline.Taxonomies` exists). The governed S09.10 entries are written at boot:
  `EnsureRW12TaxonomyGovernance` then `EnsureGF3TaxonomyGovernance` (`internal/shell/shell.go:431,466`).
  `gf3seeds.go` holds the v3 PENDING-ratification record and pins the LIVE seed by digest
  (`gf3ContentDigest`); its own instruction to the next editor (`gf3seeds.go:48-59`): freeze the
  v3 content as a snapshot the way `rw12taxonomy_v2.go` froze v2, do NOT touch GF3's digests or
  Ensure, mint your own Ensure with your own provenance, call it after GF3's. Precedent for the
  one sanctioned GF3-file edit: `taxonomyContent` was repointed from the live seed to the frozen
  v2 snapshot when GF3 edited the sets (`rw12seeds.go:154-162`) — GF7 repeats that move for
  `gf3TaxonomyContent`.
- **Pins that break under v4** (sanctioned-edit candidates, §8): `TestGF3WeightsAndIDsStayVerbatim`
  (`gf3be1_test.go:92-121` — ids/weights/count/order wall), `TestGF3TaxonomyV3EverySlotCarriesOptionsAndWhy`
  (`:65-85` — version "v3", 2–4 options + why on EVERY slot), `TestSeedWeightsMeetTierFloorShape`
  (`taxonomy_test.go:104-129` — cumulative clearance after 2/3 cards vs floors 60/75).
- **⚙ consumed** (registry names, existing — expect NO new keys): `intake.clearance_floor.low`,
  `intake.clearance_floor.standard`, `intake.clearance_floor.high` (`pipeline.go:1585-1602`).
  Slot weights are governed-file content, not ⚙. Floors' values and clamps are untouched.

## 2. Ordered decisions (r5 §D W2 — these are decisions, not questions)

KILL as-asked: `behavior`, `terminology` (INVERT: the system states its understanding, the user
corrects — never ask the user to guess the system's misreads), `indices_ranges` (resolve
internally, never ask). FIX: `comparison_rules` (options bind to a named surface — search
results vs default view; per-goal phrasing per §E), `output_format` (intelligible or dead),
`units` (why-line in plain project terms or nothing). KEEP: `edge_cases`, `assets_media` (the
quality bar), `look_feel`, `technology_stack`, `collection_semantics`, `ordering_atomicity`
(system-decides as the recommended default). EVERY surviving slot meets the
`assets_media`/`look_feel` bar — concrete, project-specific, assumption-preventing, a real
recommendation with why and effect — or dies (r5 §E last note). Target: gap-finding, never
slot-filling (§C.1); no fixed caps, Clearance floors unchanged (S06.5).

## 3. Requirements

**R1 — Software taxonomy v4, drafted at the bar.** `softwareSeed()` moves to `Version: "v4"`.
Surviving-slot ids and weights stay VERBATIM (the measured ClarifyCodeBench weights are still
the measured thing; the operator moved CONTENT, not weights). Every ASKED slot's question, why,
and options are redrafted to the seven hard rules (§4 per-slot matrix) — drafted in the packet's
own session with the strongest available frontier model, and the `Source` string + governance
record SAY which model and when (S06.5 "drafted at implementation time with the strongest
available frontier model"; the gf3Provenance/rw12Provenance pattern, `taxonomy.go:188-206`).
[r5 §D W2, §E; S06.5]

**R2 — The never-ask posture.** New slot vocabulary: an ask-posture member (recommended:
`Ask string` json `ask,omitempty`, "" = asked, `"never"` = system-resolves). `behavior`,
`terminology`, `indices_ranges` become ask-never: they KEEP their ids and weights in the file
(coverage stays accounted — S06.5's resolution set already includes "converted to an explicit
assumption"; H10 ratified: inferred slots are confirmable assumptions COUNTED by the Clearance
meter, never silently filled state). Delivery excludes them everywhere a fresh question ships:
`phaseInterview`'s selection AND the loop's continue-condition (askable-unresolved, else an
exhausted interview below floor would issue an empty card), AND `reviewQuestions` (a re-interview
must not present the guess-my-misread question either — their current resolution rides the
understood recap instead; correction stays available via `Assume`, free text, and Re-plan
contests until GF8's editable surface lands). `Validate` accepts ask-never slots with MustKnow
required and Question/Options/Why optional. [r5 §C rules 2+3; W1 H10; S06.5]

**R3 — System-resolution accounting (the H10 mechanism).** At interview entry (recommended site:
`applyFamilyTaxonomy`, which already stamps TaxonomyID/Version and resolves registry slots,
`pipeline.go:455-471`), every ask-never slot resolves as `ResolvedAssumption` with honest prose
("this is a detail the platform settles itself; what it picked is shown on the plan" — final
wording per CONVENTIONS §57/§59 drafting rules), idempotent on re-entry and on family switch.
Consequences that must hold: Clearance counts them from the start; `acceptEmission`'s existing
guarantee mints the `slot:<id>` SPEC assumption for each, and the planner's own concrete text
WINS when it lists that origin (already the code's behavior, `pipeline.go:1381-1398`) — the
served assumption the requester reads must carry the planner's concrete resolution wherever the
planner stated one (recommended: after emission acceptance, back-fill the state resolution's
Assumption text from the accepted SPEC's slot-origin entry — platform code copying from the
artifact, never model prose invented pipeline-side). A system-resolved slot must be
DISTINGUISHABLE from a requester skip on the record (recommended: additive marker on
`SlotResolution`, e.g. `Via: "system"`) — the operator accepted the skip sentence "you skipped
this one, so I will pick something sensible" (r5 §A round 1); a slot the requester was never
asked must not claim they skipped it. [W1 H10; S06.5; r5 §C rule 2]

**R4 — The terminology/behavior INVERSION lands as content + accounting, not as a new surface.**
The inverted mechanism ("tell me what you understand and I correct it" — r5 §A round 2, §C rule
3) is, at W2's depth, ALREADY the platform's shape: the deterministic `UnderstoodBlock` on every
card (`cards.go:112-123`), the SPEC's plain-language restatement + assumptions centerpiece
(S06.6/S06.9), and the slot-origin assumptions from R3. W2's half: the ask-never MustKnow texts
are REWRITTEN as planner instructions — `terminology`: state, in the restatement/assumptions,
the reading of every domain term the request uses in a special sense, as correctable statements;
`behavior`: derive intended behavior from the request and restate it (the goal restatement IS the
behavior understanding); `indices_ranges`: resolve counting/boundary conventions and disclose
the choice ("30 items means 30 items"). MustKnow rides `DraftInput.Taxonomy` to the planner
already (`pipeline.go:786-795`) — no planner-adapter code changes in this packet. The EDITABLE
understanding surface is GF8/GF9's centerpiece and is NOT built here (§5 seam). [r5 §C rule 3;
W1 H1; S06.10 duty split]

**R5 — FIX `comparison_rules`: options bind to a named referent.** Content requirements the
evaluator can check against the r5 §A verdict verbatim: (i) the question/options name WHAT
surface the order governs, or the options let the requester answer per-surface — no option whose
referent is ambiguous survives ("Closest to what the person was looking for" is the condemned
example: unanswerable, referent-free); (ii) each option states its effect (R7 vocabulary);
(iii) the per-goal binding rides the EXISTING seat suggestion (`Suggested`/`SuggestedOption` —
the look_feel round-3 proof), not a new seat duty: option labels stay unphrased at v0 (P3-RW-12
OQ4 stands). [r5 §A round 1, §E note 1; W1 H2/H3]

**R6 — FIX `output_format`: intelligible or dead.** The operator could not tell WHAT was asked
(the website's design? the folders?) nor what problem it solves. Redraft to a question a
non-programmer can answer about the deliverable's arrival shape (the generic set's `deliverable`
slot proves the answerable form, `taxonomy.go:891-900`), with a why naming what breaks; if the
redraft cannot meet the bar for the software family, the slot goes ask-never instead (the
planner states the delivery shape as a confirmable assumption). Executor drafts; evaluator
judges against §A round 3. [r5 §A round 3, §E last note]

**R7 — Recommendation + effect vocabulary (rule C.4 made structural).** Additive taxonomy
members so a surface CAN render the operator's bar: per-option effect line (recommended:
`Option.Effect string`, plain words: what picking this does to the result) and a per-slot
recommended default (recommended: `Slot.Recommended string` naming an option Value; the
recommended option's Effect carries why-it-is-recommended). Every ASKED v4 software slot carries
2–4 concrete options, every option an Effect, exactly one Recommended. `units` keeps its
question, its why rewritten in plain project terms or dropped (Why is already omitempty).
`ordering_atomicity` gains the planner-chooses option (`plannerChoosesValue`, `taxonomy.go:24`)
as its FIRST option and as Recommended — the operator ruled system-decides "the only honest
default" for a non-IT requester. The per-goal recommendation stays the seat's
`SuggestedOption` fold, which OVERRIDES the authored default at render time (GF9's rendering
rule; W2 serves both). JSON stays strict-decodable both ways (struct fields, `LoadTaxonomy`
round-trip). Rendering of the marker/effect lines is GF9's; W2 ships data + tests only.
[r5 §C rules 4+5, §A rounds 1–3; W1 H3; S06.5 delivery text unchanged]

**R8 — H23 composition: two new slots, the rest is planner substance.** The H23 reference set
(stack / image sourcing / UI language / data depth / quality bar) composes with the KEEP set as:
stack = `technology_stack`, images = `assets_media` (both kept, raised to the bar with H23's
phrasing grade as the reference); NEW software slots `language_locale` (which language(s) the
deliverable speaks, locale conventions — every seeded string and label depends on it; W1 §D.1
shows it un-asked nowhere else) and `quality_bar` (what the finished thing must pass to count as
done — feature-correct only, or a stated polish/accessibility bar; it decides what verification
gates on and "changed a past webshop verdict"), weights REASONED and recorded in Source
(pattern: reasoned never outranks the measured 12s; recommend 9 and 8). H23's data-depth
question and the rest of the W1 §D.1 under-asked list (currency, seed-data realism, catalog
split, responsive targets, animation specifics) are GOAL-SPECIFIC: they belong to the planning
model's substance — 1.7 single-question escalations and post-draft NEEDS-CLARIFICATION markers
(S06.5/S06.6) — not to universal slots; the taxonomy carries coverage, the planning model
conducts substance (S06.10). Question-count arithmetic (the no-inflation check): v3 asked 13
slots; v4 asks 10 surviving + 2 new = 12, with 3 killed slots' weight (28 of ~142) resolved at
entry — at unchanged floors the interview asks the same or fewer questions, each at higher
grade. [W1 H23 + §D.1; r5 §C rule 1; S06.5 no-caps]

**R9 — Family-guess visibility, W2's half (H18).** Additive `Card` members `Family` +
`FamilySource` (omitempty), populated from state on every issued card (the family card itself
naturally omits them — family is unresolved there), so a surface can show "I am treating this as
software work (my guess)" BEFORE questions render. Additive interview-answer member to CORRECT
a wrong guess (recommended: `Answer.Family` on interview cards; validated against `ValidFamily`,
sets `FamilySourceRequester`, re-runs `applyFamilyTaxonomy` — which already re-scopes Clearance
to the new set while `State.Resolutions` retains everything (no H17 silent loss; cross-family
resolutions simply stop counting), re-resolves registry + ask-never slots for the new set, and
records a platform decision line (the `recordTaxonomyFallback` precedent, `pipeline.go:475-482`).
RW-13's zero-touch posture MUST NOT regress: the classifier path gains no card, no click, no new
ask — the guess is passive data on cards that already exist; `familyGate` semantics untouched.
Rendering the chip + correction affordance is GF9's. [W1 H18 (REJECT-as-silent) + H17; P3-RW-13
R13; r5 §C rule 3]

**R10 — The v3→v4 governance fold.** Per `gf3seeds.go`'s own instructions and the
`rw12taxonomy_v2.go` precedent: (i) freeze the v3 software content byte-exact in a new snapshot
file (e.g. `internal/memory/gf3taxonomy_v3.go`) and repoint `gf3TaxonomyContent` at it (the ONE
sanctioned GF3-file edit — same move GF3 made to RW-12's `taxonomyContent`); GF3's digests and
Ensure otherwise untouched, so a fresh world still writes the true chain v1→v2→v3→v4; (ii) mint
`internal/memory/gf7seeds.go`: own provenance type, originRef naming packet P3-GF7 + the r5/W1
records + this brief, decision text naming the kills/inversions/fixes/adds, `Ratified: PENDING`
at the planning-rework exit gate with refusal-executed-as-supersession-back-to-v3 semantics,
strongest-model drafting statement (R1), own `gf7ContentDigest` pinning the v4 software bytes,
`EnsureGF7TaxonomyGovernance` superseding the SOFTWARE entry only (generic stays v3 — no
operator finding touches it), with the same committed-hash / decisionRecorded / repair
discipline gf3seeds carries; (iii) third call at the boot site after GF3's
(`internal/shell/shell.go:466`). [S09.10 row "Interview must-know taxonomies"; S09.8; r4 §F2
"same machinery"; CONVENTIONS §57]

**R11 — No spec amendment; batteries.** S06.5 already carries versioned taxonomies through the
8.3 gate; the kills/inversions are a governed content supersession, not spec change; S06.5's
resolution vocabulary already contains assumption-conversion; floors, no-caps, 2–4-option
delivery text all untouched; A15 (per-step approach) is landed spec-side and is GF8's to
implement. If the executor finds any ratified sentence contradicted, STOP and surface it —
expect none. Batteries: single-package only while the GF6 builder runs —
`go test -p 1 ./internal/intake`, `go test -p 1 ./internal/memory`, plus `go build ./...` and
`go vet ./...`; NEVER the full test battery; no web batteries. [S00.9; operator serial-tests
directive]

## 4. The per-slot acceptance matrix (makes the seven rules checkable)

For EVERY asked v4 software slot the evaluator verifies, against r5 §A verbatim and H23's grade:

| Check | Concretely |
|---|---|
| C.4 purpose | Why states what breaks if unanswered, in plain project terms — or Why is absent. The condemned units line ("Mixed-up units stay invisible…") is gone. |
| C.4 options | 2–4 options a non-programmer can pick between; no abstract category labels (the condemned behavior options); no ambiguous referent (the condemned comparison_rules option). |
| C.4 recommendation | Exactly one Recommended default; its Effect says why; system-decides recommended where the requester cannot honestly answer (ordering_atomicity). |
| C.4 effect | Every option carries an Effect line (what picking this does). |
| C.5 finite sets | Choices wherever the answer set is finite; free text + per-slot skip remain everywhere. |
| C.2/C.3 | behavior/terminology/indices_ranges (+ any slot ruled under OQ1) are never delivered as questions on ANY card kind; their understanding shows as confirmable statements. |
| The bar | Each slot reads at `assets_media`/`look_feel` grade: concrete, assumption-preventing, answerable by describing what you want. Reference: replay the r5 car-parts goal against the v4 set and check every would-be card against §A + H23. |

## 5. Seams and stubs (state, don't build)

- **GF8 (W3 wire)**: the editable understood-spec artifact (pre-draft / post-answers /
  drafted-plan, through Sinet's artifact + replan machinery, never free-text mutation) and the
  A15 per-step approach member on the PLAN. W2 hands it: v4 content, R3's system-resolution
  records, R4's MustKnow planner instructions. W2 touches NO plan-artifact schema, NO planner
  adapter, NO new endpoints beyond R9's answer member.
- **GF9 (W3+W4 FE)**: renders Recommended markers, Effect lines, the family chip + correction
  affordance, the understanding statement (H1 grade), and rebuilds the open-points card to
  interview standard (currency = the worked example; RA-4 dead). W2 serves data only.
- **W4 error/marker surface**: the round-4 bare-textbox defect (planner NEEDS-CLARIFICATION
  markers surfacing raw) is GF9's; W2 changes no marker mechanics.

## 6. Open questions (recommendation first; contested → operator)

- **OQ1 — `numerical_precision`.** Unruled by r5 (never asked in that transcript). It sits in
  the same measured natively-handled band as the killed `indices_ranges` and fails C.2 the same
  way. RECOMMEND: ask-never (system resolves rounding, discloses "money shown as €12.34"-style
  assumptions; 1.7 escalation when genuinely consequential). Flag at the gate with the v4
  ratification.
- **OQ2 — new-slot ratification.** `language_locale` + `quality_bar` enter under the same
  PENDING v4 record — no separate ceremony. RECOMMEND: yes (r4 §F2: same machinery; refusal
  granularity stays whole-version, matching the gf3 refusal semantics).
- **OQ3 — Effect/Recommended on the other five families.** RECOMMEND: software-only this
  packet (no operator findings against the others; Validate must tolerate absence), each family
  raised at its own next revision.
- **OQ4 — seat option-label phrasing.** RECOMMEND: stays OFF (P3-RW-12 OQ4); per-goal content
  rides Suggested/SuggestedOption. Revisit only if the gate finds R5's authored binding
  insufficient on the replayed r5 goal.
- **OQ5 — review-card posture for ask-never slots** (R2 recommends exclude-from-ask, correct
  via recap/Assume/contest). Confirm; the alternative (a confirm-phrased entry per ask-never
  slot on the review card) is GF8-adjacent and should not be pre-built here.

## 7. Files expected to change

`internal/intake/taxonomy.go` (v4 seed + vocabulary + Validate) · `internal/intake/pipeline.go`
(entry-resolution of ask-never; askable delivery loop; reviewQuestions; Card family members) ·
`internal/intake/cards.go` (Card additive members) · `internal/intake/answer.go` (family
override; skip/system distinguishability) · `internal/intake/state.go` (SlotResolution marker if
OQ-free shape adopted) · intake tests: `gf7red_test.go` (reds → green), new v4 tests, sanctioned
supersessions of `gf3be1_test.go` verbatim wall + options/why pin and `taxonomy_test.go` floor
shape (§8) · `internal/memory/gf3taxonomy_v3.go` (new frozen snapshot) ·
`internal/memory/gf3seeds.go` (content-source repoint ONLY) · `internal/memory/gf7seeds.go`
(new) + memory tests · `internal/shell/shell.go` (third Ensure call). NO web/src paths (GF6
builder's lane), no Spec edits, no ⚙ registry changes.

## 8. Sanctioned test-seam edits (declare in the impl commit; evaluator adjudicates)

1. `TestGF3WeightsAndIDsStayVerbatim` — pins v3 count/ids/weights/order; supersede with a v4
   wall (surviving ids/weights verbatim, killed ids present-as-ask-never per R2, new slots
   appended). The v3 pin moves to the frozen memory snapshot's own test.
2. `TestGF3TaxonomyV3EverySlotCarriesOptionsAndWhy` — version literal + every-slot options/why;
   re-scope to ASKED slots and "v4".
3. `TestSeedWeightsMeetTierFloorShape` — re-derive with v4 weights + entry-resolved ask-never
   mass. No other committed test files may be touched except by declared deviation.

## 9. Acceptance tests

**Committed RED with this brief (Amendment-A window OPENS at this commit, closes at the P3-GF7
implementation commit; `go build ./...` stays green): `internal/intake/gf7red_test.go`**

- T1 `TestGF7KilledSlotsAreNeverAsked` — drives a standard-tier software interview; asserts no
  card ever carries behavior/terminology/indices_ranges as a question. [R2]
- T2 `TestGF7SoftwareV4ShapeAndSystemDecidesDefault` — version "v4"; ordering_atomicity offers
  planner_chooses. [R1, R7]
- T3 `TestGF7SystemResolvedSlotsLandAsSpecAssumptions` — after answering to the floor, the SPEC
  carries `slot:behavior`/`slot:terminology`/`slot:indices_ranges` origin assumptions. [R3]
- T4 `TestGF7InterviewCardServesTheFamilyGuess` — the served interview-card JSON carries
  `family` + `family_source` from the classifier. [R9]
- T5 `TestGF7UnitsWhyLineRewritten` — the condemned units why-line is gone. [R7]

**Executor-written (spec here, land green with the implementation):**

- T6 governance: `EnsureGF7TaxonomyGovernance` supersedes v3→v4 once, idempotently; refusal
  recorded via decisionRecorded is honored; seed drift vs `gf7ContentDigest` = ErrSeedDiverged;
  GF3's Ensure still writes the FROZEN v3 bytes. [R10]
- T7 family override: interview-answer family correction re-scopes the question set, retains
  resolutions, sets FamilySourceRequester, records the decision line; the classifier zero-touch
  path issues no new card (RW-13 guard: existing livetriage assertions untouched). [R9]
- T8 review card: ask-never slots absent from reviewQuestions; their resolutions ride the
  understood recap; Assume on an ask-never slot still lands. [R2, OQ5]
- T9 skip-vs-system: a requester skip and a system resolution are distinguishable on the record
  and produce their own prose. [R3]
- T10 vocabulary round-trip: v4 marshals/parses through `LoadTaxonomy` strict decode with
  Effect/Recommended/ask fields; Validate rejects a Recommended naming no option, an asked slot
  without options, an ask-never slot without MustKnow. [R7, R2]
- T11 seat fold on v4: suggestions/phrasings fold on v4 ids under containment (existing
  gf3be1 folds stay green on the new set). [R5]
- **Property marks**: P1 — for arbitrary resolution states, delivery never emits an ask-never
  slot and never emits an empty interview card (askable-exhaustion). P2 — Clearance with
  ask-never mass: monotone under resolution, 100 iff all slots resolved, entry value equals the
  ask-never weight share. [R2, R3]

## 10. Packet checklist (the evaluator's gate)

1. All R1–R11 traced to landed code; T1–T5 green; T6–T11 + P1/P2 landed green.
2. §4 matrix passes per slot, including the r5-goal replay against §A verdicts + H23 grade.
3. Killed slots: never asked on any card kind, in any tier, any path (fresh, re-interview,
   force-proceed, band).
4. v4 ids/weights of surviving slots byte-verbatim vs v3; Source records model + date + reasoning
   for new-slot weights.
5. Governance chain in a fresh world reads v1→v2→v3→v4 with the v4 record PENDING; gf3 digests
   and provenance untouched; refusal path proven.
6. No new ⚙ keys; floors byte-unchanged; S18 tally untouched (118/33).
7. RW-13 zero-touch unregressed; no new card kinds; family members additive-only (old snapshots
   decode).
8. Amendment-A window closed; sanctioned edits (§8) enumerated in the impl commit message;
   single-package batteries only.
9. GF8/GF9 seams honored: no plan-schema, planner-adapter, FE, or open-points changes.
