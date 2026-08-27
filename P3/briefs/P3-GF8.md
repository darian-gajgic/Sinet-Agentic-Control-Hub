> **EXPIRED at P3-GF8 landing (2026-08-27).** Single-use artifact — drain fixes moved the code past this text; never read as truth.

# P3-GF8 — The editable understood-spec WIRE + the A15 per-step approach (W3, backend half)

**Status: ACTIVE (grounding committed 2026-08-27). EXPIRES when P3/STATE.md marks the packet landed.**

Two operator-ordered capabilities, both backend/wire only — every pixel is GF9's:

- **(A) The editable understood-spec surface's wire** (operator record r5 §B.1 + §C rule 7; harvest
  H9 **FIT-WITH-MODIFICATION**): the platform's derived understanding, served as STRUCTURED,
  per-field addressable data at the pre-draft, post-answers, and drafted-plan stages, with a
  correction verb per field that routes through the machinery that already owns that kind of
  change. NEVER free-text mutation of a served artifact (r5 §D.1; H9's ratified modification).
- **(B) The A15 per-step approach** (Spec S06.6 as amended 2026-08-27; S00.9 row A15; r4 §F3):
  the PLAN carries, per step, HOW it will be built — chosen method in plain words, material
  decisions with alternatives and why, load-bearing ordering rationale. Schema, planner-prompt
  contract, boundary validation, and the wire serving it are THIS packet's; the S06.9 Layer-2
  rendering is GF9's. Verification binding is UNCHANGED.

Binding sources: `Spec/drafts/S06-intake-pipeline.md` §§S06.5–S06.10 (S06.6 [A15] bullet, S06.9);
`Spec/drafts/S00-front-matter.md` §S00.9 row A15; `P3/design/b6-gate-operator-findings-r5-2026-08-23.md`
(§B, §C, §D.1); `...-r4-2026-08-23.md` (§F3, §F4); `P3/design/w1-nexus-live-harvest-2026-08-27.md`
(H9, H10, H17). Prior briefs are EXPIRED and were not used as truth.

## 1. Where the build stands (code facts, verified this session)

1. **The artifact pair** (`internal/intake/artifact.go`): `Spec{Restatement, Outcome, ACs,
   Constraints, Assumptions(+Origin "slot:<id>"|"force_proceed"|"planner"|"band"), OutOfScope,
   Supplied, Clarifications}`; `Plan{Steps, Coverage, ResearchNodes, Risks, CitedEntries, Est}`;
   `Step{ID "S-n", Title, DoneWhen, Class, WriteSet/ReadSet/Unbounded, effect flags, Research}`.
   Markdown of record + JSON sidecar, byte-identical re-render verified on load (`loadVerified`);
   guarded sections keep old artifacts byte-stable (the `CitedEntries` precedent, artifact.go).
   The grounding commit added the INERT [A15] members: `Step.Approach`, `Step.Decisions
   []StepDecision{Decision, Alternatives, Why}`, `Step.OrderingRationale` — no validation, no
   rendering, no prompt yet (that is the implementation).
2. **Cards** (`cards.go`): `Card{Kind, …, Family/FamilySource (GF7 R9), Questions, Decision,
   Approval, Delta, Understood}`. `UnderstoodBlock`/`UnderstoodItem{SlotID, Name, How, Value,
   Assumption}` is composed by platform code from `State.Resolutions` — never a model
   (`understoodBlock`/`understoodRecap`, pipeline.go). The approval card serves Layer 1
   (restatement, deliverable=Outcome, steps-as-titles, WillNotDo=OutOfScope, Assumptions
   centerpiece, Risks, cost, Understood recap) and Layer 2 (ACs, full Steps, Coverage, Verdicts,
   ResearchNodes, Estimate, refs). **Layer 2 did NOT serve `Spec.Constraints` or `Spec.Supplied`**
   — the two r5 §B.1 understanding fields missing from the drafted-plan surface. The grounding
   commit added the inert `ApprovalLayer2.Constraints/Supplied` fields (unpopulated).
3. **Correction verbs already on the wire** (`answer.go`):
   - Interview: `Answers` (card-bound — `slot %q was not asked` for anything else), `Assume` (ANY
     taxonomy slot), per-slot `Skip`, `ForceProceed`, `Family` correction (H17-safe: resolutions
     retained across the switch).
   - Approval: `ActionRePlan` with `Contest`/`Contests []ContestRef{Target, Note}` —
     **`ContestRef.Note` is per-target on the wire** (r4 §F4 confirmed: the gap is FE-only, GF9's);
     plus the top-level `Note` as target-less finding. `replanContest` folds targets+notes into ONE
     `ReviseReq{Reason: ReviseContest, Findings}` → one bounded `Planner.Revise` round. Targets are
     currently UNVALIDATED free strings; the FE sends `S-n` / `AC-n` / `assumption:<text>`
     (Intake.tsx `contestGroups`).
   - `ActionReInterview` → `reviewQuestions`: the full active set, each resolved slot carrying its
     current `Question.Resolution` — the per-field review surface, landed (GF3-BE1 R8).
4. **Post-approval deltas** (`delta.go`): `ProposeDelta` diffs frozen vs next via `diffPairs` —
   **ACs, Steps (Title/DoneWhen/Class only), Assumptions-by-text. It does NOT diff Restatement,
   Outcome, Constraints, OutOfScope, Risks, or the [A15] members**: a pure understanding
   correction post-approval errors `proposed revision changes nothing`, and an approach change
   would be INVISIBLE on a delta card. Verified red: `TestGF8UnderstandingDeltaProperty`.
5. **The planner seam** (`internal/stage/engines.go`): `EnginePlanner.Draft/Revise` build one
   instruction string around the shared `pairSchema` const (the JSON emission contract). The
   schema knows nothing of approach. The pipeline stamps identity/versions and validates
   (`pairSession` → `Pair.Validate`); trust is in the spine, not the seam.
6. **Serving** (`internal/api`): cards pass through raw as ask snapshots (`IntakeSurface.Task` →
   `open_card`); `TaskDetail{Spec *intake.Spec, Plan *intake.Plan}` serves the full artifacts;
   `Artifacts` serves the pair. `web/src/api.ts` mirrors: `IntakeCard`, `IntakeQuestion`
   (+`resolution`), `IntakeUnderstood(Item)`, `IntakeApproval` (layer1/layer2), `IntakePlanStep`
   (no approach), `IntakeAnswerBody` (answers/skip/contests/note). Steps-bearing fixtures:
   `web/src/fixtures/api/task-detail.json`, `task-detail-draft.json`, `approvals.json`,
   `approvals-mine.json`.
7. **GF7 is truth and stays**: taxonomy v4 (11 asked / 4 ask-never in software), ask-never entry
   settlement (`applyFamilyTaxonomy` → `ViaSystem` resolutions, back-filled from the accepted
   artifact in `acceptEmission`), `Card.Family/FamilySource`, family correction on the interview
   answer. Nothing here may undo or duplicate it.

## 2. The three-stage mapping (the W3 architecture decision)

Nexus shows one mutable spec document at three moments (r5 §B.1). Sinet's ratified modification
(r5 §D.1; H9): the same capability re-expressed through Sinet's artifact + interview/contest/
replan wire under ledger immutability. The spec already names the frame: *"the restate-and-confirm
loop of 1.3 is realized as: SPEC restatement on the approval card, iterated through Re-plan /
Re-interview until the requester approves; approval is the confirmation"* (S06.1). Therefore —
**no new gate, no new card kind, no mutation door**:

| Operator stage (r5 §C.7) | Sinet surface (wire) | Correction verb (owning machinery) |
|---|---|---|
| Pre-draft (during the interview) | interview card's `Understood` block + questions | `Answers` (widened, R2) · `Assume` · `Skip` · `Family` |
| Post-answers | the same block grown per round; clarification card (`Understood` + markers); `TaskDetail.Spec/Plan` once drafted | marker answers · R2 corrections while interviewing continues |
| Drafted plan | approval card L1+L2 (completed, R4) + `ActionReInterview` review card | `Contests` with the field grammar (R5–R7) · Re-interview · post-approval delta (R8) |

The requester's edit is always a REQUEST routed through the owner of that change — interview
record, assumption conversion, or a bounded re-plan — and lands as a new artifact version. That
is "editable" under D9/D7/S05 immutability, exactly what H9's FIT-WITH-MODIFICATION ordered.

## 3. Requirements

**A — the understanding wire**

- **R1 (mapping ratified).** Implement §2's mapping exactly: no new card kind, no new gate, no
  endpoint that writes requester text into an artifact [r5 §D.1; H9; S06.1; S05]. Negative
  requirement — the evaluator checks nothing of the sort appeared.
- **R2 (pre-draft per-field correction).** `applyInterviewAnswer` accepts a `SlotAnswer` for a
  slot NOT among this card's questions IFF the active taxonomy holds the slot AND it is currently
  resolved (i.e. the card's `Understood` block shows it). Applied as `ResolvedAnswered` via
  `resolveSlot` (last write wins — the requester corrects their OWN record, H17's silent-discard
  is not in play); Clearance recomputed; when a pair already exists the correction joins the
  `ReviseInterview` merge exactly as card answers do. Ask-never slots ARE correctable this way:
  GF7 R2 bans the PLATFORM's asking, not the requester's correcting (the `reviewQuestions` comment
  states correction stays available). Refusals unchanged for: unresolved+unasked slots (delivery
  order is the taxonomy's), unknown slots, `Skip` off-card (a skip takes the CARD's served
  suggestion; no card question, no suggestion). [r5 §C.3; H9 ✓-badge grammar; S06.5]
- **R3 (per-field identity, stated).** Slot-backed understanding is addressed by `slot_id`
  (served today in `UnderstoodItem`/`Question.Resolution`) — no wire change owed pre-draft beyond
  R2. State in code comments; GF9 renders the edit affordance from these ids.
- **R4 (the drafted-plan surface serves the WHOLE understanding).** `buildApprovalCard` populates
  the (inert-landed) `ApprovalLayer2.Constraints` and `ApprovalLayer2.Supplied` from the pair.
  With that, the r5 §B.1 field list maps onto served, structured data: goal restatement →
  `Layer1.Restatement`; acceptance criteria → `Layer2.ACs`; users → family slots (`audience` etc.)
  in the Understood recap; constraints → `Layer2.Constraints` (NEW); data/integrations → family
  slots + `Layer2.Supplied` (NEW) + `Layer2.ResearchNodes`; out-of-scope → `Layer1.WillNotDo`;
  risks → `Layer1.Risks`; assumptions/decisions → `Layer1.Assumptions` + Understood recap. No new
  SPEC members are invented — S06.6's SPEC enumeration is untouched. Layer 1 stays one phone
  screen [A15 "What does NOT change"].
- **R5 (the field-target grammar — one ratified list).** The contest/delta target vocabulary is:
  `AC-<n>` · `S-<n>` · `assumption:<text>` · `confinement:S-<n>` (all landed) — plus NEW
  `restatement` · `outcome:<n>` · `constraint:<n>` · `out_of_scope:<n>` · `risk:<n>` ·
  `approach:S-<n>` (1-based indices into the served artifact version). One list, two readers
  (CONVENTIONS §43): `replanContest` and `diffPairs` consume the SAME vocabulary, declared once.
  Index-as-key is sound because a contest always addresses the OPEN card's pair version and a
  delta item carries Old/New text.
- **R6 (contest validation + expansion).** In `replanContest` (which gains access to the current
  pair): a grammar-shaped target is validated against the pair — a target naming nothing (e.g.
  `out_of_scope:99` against a 1-item list, `approach:S-9`) is refused `ErrBadAnswer`, naming what
  the card offers. A validated field target's finding is EXPANDED with the field's current text —
  `out_of_scope:1 ("no deploys"): <note>` — so field identity and content reach the reviser,
  never a blob [r5 §C.7; S06.9 structured entry]. Non-grammar free-text targets keep today's
  permissive fold (OQ1). Extending expansion to the landed `S-n`/`AC-n` targets is sanctioned but
  optional (a landed test pinning old finding wording outranks it).
- **R7 (the sanctioned road, made cheap — no mutation door).** A pre-approval understanding
  correction rides the EXISTING one-bounded-revise contest path (`ReviseContest` → spine →
  re-critique only per landed rules); S06.9's Re-plan is the only sanctioned vehicle and this
  packet does NOT invent a cheaper artifact-mutation door. What keeps it cheap: R6's expansion
  gives the reviser a minimal, named delta; `specContentEqual` already keeps the spec version
  still when content is unchanged; no second paid critique is triggered by the landed flow.
  State this in the brief-facing comment at `replanContest`. [S06.9; r5 §D.1; H20]
- **R8 (post-approval: the delta vocabulary covers the whole requester-facing content).**
  `diffPairs` widens to R5's grammar: `restatement` (MODIFIED), `outcome:<n>` / `constraint:<n>` /
  `out_of_scope:<n>` / `risk:<n>` (index-paired ADDED/MODIFIED/REMOVED with Old/New, the AC
  pattern), and `approach:S-<n>` (MODIFIED when any [A15] member of that step changed; Old/New
  must make the change legible on the delta card). A pure understanding correction is then
  expressible; `changes nothing` remains only for a truly unchanged pair. This is also what A15's
  "approach text freezes with the plan and changes through the same delta re-approval" REQUIRES —
  without it an approach change would be structurally invisible. [S06.9 delta contract; A15]
- **R9 (re-interview: no work).** `reviewQuestions` already delivers the per-field review card.
  Verified fact; do not duplicate.

**B — the A15 per-step approach**

- **R10 (schema).** The [A15] members landed inert in the grounding commit are the schema:
  `Step.Approach string` (required), `Step.Decisions []StepDecision{Decision, Alternatives, Why}`
  (optional list; each listed decision complete), `Step.OrderingRationale string` (optional —
  only where ordering is load-bearing). Marker comments [A15] stand at the struct (landed) and
  MUST be added at `pairSchema` (S00.9 A15 row: the implementing packet annotates its
  planner-prompt and artifact-schema sites).
- **R11 (boundary validation).** `Plan.Validate` requires per step: non-empty `Approach` (≤
  `approachMaxRunes`); each `StepDecision` carries non-empty `Decision` + ≥1 `Alternative` +
  non-empty `Why` (a "material decision" with no alternative is not one — S06.6 [A15] wording);
  `OrderingRationale` optional; structural caps per §6. Enforcement bites at `acceptEmission` /
  `ProposeDelta` (both call `Validate`); stored pre-A15 artifacts are unaffected — `loadPair`
  never re-validates (verified fact). No bounce round is added: an invalid emission errors exactly
  as any invalid emission does today.
- **R12 (rendering on the artifact of record).** `renderPlanMD` renders per step, GUARDED (the
  `CitedEntries` precedent — pre-A15 sidecars re-render byte-identically): the approach line, the
  decisions (decision, alternatives, why), the ordering rationale. The markdown of record — not
  just the sidecar — says HOW [S06.6; D9].
- **R13 (planner-prompt contract).** `pairSchema` gains, per step:
  `"approach":string, "decisions":[{"decision":string,"alternatives":[string...],"why":string}...],
  "ordering_rationale":string?` and the rules paragraph gains: every step states, in plain words a
  non-programmer can read, HOW it will be built; the material decisions made inside the step with
  the alternatives considered and why the winner won; the ordering rationale only where the
  ordering is load-bearing; a step naming only its outcome is invalid. This binds Draft AND Revise
  (both wrap the one const). It is the PLANNING model's duty — S06.10 duty table: the planning
  model conducts substance; no utility-seat involvement. [A15; r4 §F3 "I don't see HOW"]
- **R14 (wire serving — mostly free).** `ApprovalLayer2.Steps` is `[]Step`, `TaskDetail.Plan` is
  `*intake.Plan`: the [A15] members flow to every surface once populated (verified fact — state,
  don't rebuild). `web/src/api.ts` `IntakePlanStep` gains
  `approach?: string; decisions?: { decision: string; alternatives?: string[]; why: string }[];
  ordering_rationale?: string`; `IntakeApproval.layer2` gains `constraints?: string[]` and
  `supplied?: { rule_id: string; fact: string }[]`; the `IntakeAnswerBody` contest comment
  documents R5's grammar. Comments carry the [A15] tag.
- **R15 (execution reads the approved method).** `stepContract` (plansource.go) gains the
  approach (and, when present, decisions/ordering) as guarded lines: the executing stage receives
  the approved HOW instead of re-deriving it. Verification binding untouched — see R16. (OQ3
  offers the out.)
- **R16 (verification binding UNCHANGED — negative requirement).** No change to
  `internal/verify`, S07 consumers, or the "Done when"/AC binding; the approach is never a
  machine-check target, never a rubric input, never validated for reading level. [A15 "What does
  NOT change"; S13.4/S13.6 review wire untouched.]
- **R17 (fixtures + web battery).** Steps-bearing fixtures (`task-detail.json`,
  `task-detail-draft.json`, `approvals.json`, `approvals-mine.json`) gain honest approach content
  (and layer-2 constraints where an approval card is embedded) so GF9 builds against real shapes.
  Moving web/src fixtures owes the vitest run for the consuming suites — at LANDING, serial,
  never during the live-walk sitting (host constraint, serial-tests directive).
- **R18 (ledger/freeze mechanics untouched).** Approach freezes with the plan via the existing
  artifact freeze (approve() writes `.approved.md`); ledger pins (§1 objective+ACs, §2
  constraints+danger zones, plan version) are NOT extended. [S05; S06.9 "On Approve"; A15.]

## 4. Seams and stubs (state, don't build)

- **GF9 (rendering seam — the whole visible half):** S06.9 Layer-2 approach under each step with
  the assumptions treatment; per-field edit affordances on the understood block and approval card
  driving R2/R5 verbs; per-target contest note boxes (r4 §F4 — wire already carries them); the
  family chip + correction affordance (GF7 R9). GF8 serves; GF9 draws.
- **Delta-card provenance:** post-approval understanding deltas render on the EXISTING delta card
  (R8 targets + Old/New); no new card kind.
- **Knowledge-gate/taxonomy:** untouched — R2 widens answer ACCEPTANCE, not the question sets.

## 5. Adopted components

None. No `components.lock` change. Nexus is a STUDY source only (r5 §D.1) — zero code crosses.

## 6. ⚙ and structural constants

**No new ⚙ keys; no default or clamp touched; no S18 re-sweep (A15 row: tally stays 118/33).**
Structural constants (named, commented with reasons, flagged to the gate per the settings-tab
directive — the `maxQuestionsPerCard`/`sseBatchSize` precedent), coordinator-draft values the
executor may tune with a stated reason:

| Constant | Draft value | Reason |
|---|---|---|
| `approachMaxRunes` | 1200 | abuse-guard on card/artifact size; approach is a few plain sentences, not an essay |
| `decisionFieldMaxRunes` | 400 | per Decision/Alternative/Why string |
| `maxStepDecisions` | 8 | more is a step that should be split |
| `orderingMaxRunes` | 400 | one load-bearing sentence or two |

## 7. Open questions (recommendation first; contested → operator)

- **OQ1 — non-grammar contest targets.** Keep the permissive free-text fold (recommended: they
  are the requester's own words, the landed §11 OQ-4 posture) vs refuse everything non-grammar.
  Strict refusal would be a behavior break for no operator-ordered gain.
- **OQ2 — amendment owed beyond A15?** Recommendation: **NO**, presented for coordinator
  confirmation at the gate batch — never resolved silently. Reasoning: (i) S06.9's Re-plan line
  ("tap the AC, assumption, or step") is a structure the landed multi-contest already read as
  not-a-cardinality (GF3-BE1 precedent); reading it as not-a-closed-enumeration over the same
  card's served members is the same discipline, and the operator record (r5 §C.7 + §D.1) is the
  ordering authority for exactly this capability on exactly this wire. (ii) S06.9's delta
  contract already binds "every post-approval change to SPEC or PLAN" — R8 implements, not
  extends, it. (iii) R4 adds content to Layer 2's expandable enumeration additively; Layer 1 is
  untouched. If the coordinator reads any of these as spec-text drift, the fix is an editorial
  S06.9 marker note riding A15's annotation duty — not a mechanics amendment.
- **OQ3 — approach into `stepContract`?** Recommended YES (R15): execution should follow the
  approved method. The out: drop R15 with a stated reason; nothing else depends on it.
- **OQ4 — `IntakeCard.family`/`family_source` in api.ts.** GF7 serves them; GF9 needs them.
  Recommended: ride this packet's api.ts edit (two optional lines).
- **OQ5 — the §6 constant values.** Coordinator-draft; executor tunes with reasons in the impl
  commit.

## 8. Files expected to change (implementation commit)

- `internal/intake/artifact.go` — R11 validation, R12 rendering ([A15] fields landed inert).
- `internal/intake/cards.go` — nothing further (Layer2 fields landed inert).
- `internal/intake/answer.go` — R2 (`applyInterviewAnswer`), R6/R7 (`replanContest` + pair access).
- `internal/intake/delta.go` — R8 (`diffPairs`).
- `internal/intake/pipeline.go` — R4 (`buildApprovalCard`).
- `internal/intake/plansource.go` — R15 (`stepContract`).
- `internal/stage/engines.go` — R13 (`pairSchema` + rules, [A15] marker).
- `web/src/api.ts` + `web/src/fixtures/api/{task-detail,task-detail-draft,approvals,approvals-mine}.json` — R14/R17.
- Test-seam edits (declare in the impl commit; evaluator adjudicates — the GF7 §8 discipline):
  `pipeline_test.go` `basePair`/`baseRevise` and any inline-constructed Plan gain approach so the
  R11 validation keeps the battery green; no assertion may be weakened.

## 9. Acceptance tests

**Committed RED this grounding commit** (`internal/intake/gf8red_test.go`; Amendment-A window
OPEN at this commit, closes at the P3-GF8 implementation commit; `go build ./...` green; every
pre-existing test green — verified this session):

| Test | Pins |
|---|---|
| `TestGF8ApproachRequiredAtValidation` | R10/R11: approach required; incomplete decision refused; complete step passes |
| `TestGF8ApproachRendersOnThePlanOfRecord` | R12: markdown of record carries approach/decisions/ordering |
| `TestGF8ApprovalCardServesConstraints` | R4: Layer 2 serves the SPEC constraints |
| `TestGF8ContestCarriesTheFieldNotABlob` | R5/R6/R7: field target expands with current text into the revise findings |
| `TestGF8ContestRefusesAFieldThatIsNotThere` | R6: grammar-shaped target naming nothing → `ErrBadAnswer` |
| `TestGF8PreDraftCorrectionOfAResolvedSlot` | R2: understood-block slot correctable on the next card; last write wins |
| `TestGF8UnderstandingDeltaProperty` | **PROPERTY** for R8: for EVERY requester-facing field, a single-field post-approval change yields a delta item naming that field — a content change can never be invisible |

**Executor-owed (add green with the implementation):** (a) `internal/stage` prompt-contract test —
`pairSchema` names approach/decisions/ordering_rationale and the rules sentence (not committable
red from this sitting: internal/stage batteries are barred while the live walk shares the host);
(b) R4 `Supplied` serving via the research supply-fact path; (c) R15 `stepContract` carries the
approach; (d) R11 cap refusals at the boundary; (e) vitest for the fixture-consuming suites
(landing, serial). Property mark (b): re-render integrity — a pre-A15 sidecar (no approach)
re-renders byte-identically post-R12 (extend `artifact_test.go`).

**Batteries this packet may run:** single-package `go test -p 1 ./internal/intake/` only during
the sitting; `internal/stage`, full battery, and web batteries at landing per coordinator.

## 10. Packet checklist (the evaluator's gate)

1. All seven RED tests green; zero pre-existing assertions weakened (test-seam edits declared).
2. No new card kind, gate, endpoint, or artifact-mutation door (R1) — grep-checkable.
3. `diffPairs` and `replanContest` consume ONE declared target vocabulary (R5, §43).
4. [A15] markers present at `Step`, `pairSchema` (S00.9 annotation duty).
5. Pre-A15 artifact re-render integrity proven (R12 property).
6. No new ⚙ key; §6 constants named with reasons; no S18 delta.
7. `internal/verify` and the review wire (S13.4/S13.6) untouched (R16).
8. api.ts + fixtures updated; vitest for consuming suites green at landing (R14/R17).
9. OQ2's no-amendment reading presented at the gate batch, not silently assumed.
