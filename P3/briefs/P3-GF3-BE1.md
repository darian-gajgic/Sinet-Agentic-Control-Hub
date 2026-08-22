# P3-GF3-BE1 — the guided interview's served substrate (grounding brief)

Grounded 2026-08-23 on main @ c80c2d0. Binding contract: `Spec/core-architecture-v1.md` with
`Spec/drafts/` canonical — S06.5, S06.6, S06.9, S06.10 read in full this session — plus the
operator-ordered design contract `P3/design/gf3-planning-rework-design-2026-08-23.md` (its §1
readings and §2 pieces A–E are ratified packet scope) and `P3/CONVENTIONS.md` (§2, §3, §5, §6,
§14, §16, §26, §57, §59 bind here). This brief is self-contained: executor and evaluation work
from it plus the cited sections, never from chat history or prior briefs.

**Hard boundary (operator, authoritative):** backend core logic untouched. Clearance
computation, floor VALUES, slot selection order/weights, the spine, critique, approval freeze,
delta mechanics, verification, routing, and the zero-interaction band all stay exactly as
landed. Everything below is card CONTENT, wire ADDITIONS, taxonomy DATA, and one seam
extension — all additive.

---

## 1. Requirements

### A — Taxonomy v3 (software + generic fallback)

- **R1** (design §2.A; S06.5 "each with 2–4 labeled options plus free text"): revise
  `softwareSeed()` and `genericSeed()` in `internal/intake/taxonomy.go` to v3:
  - ids and weights **verbatim** — the exact id→weight maps are pinned by the committed
    green-guard test T7 (software: behavior 10, terminology 10, edge_cases 10,
    collection_semantics 12, comparison_rules 12, ordering_atomicity 12, indices_ranges 8,
    output_format 8, units 6, numerical_precision 6, technology_stack 11, assets_media 10,
    look_feel 10; generic: goal 12, deliverable 10, scope 10, inputs 8, constraints 8,
    audience 6, quality_bar 6, deadline 4). Slot COUNT and ORDER also verbatim.
  - every slot gets a plain-language `Question` a non-programmer can answer; the current
    benchmark-derived texts move into the internal `MustKnow` rationale, **never** the asked
    question (design §2.A(i)).
  - **2–4 concrete labeled options on EVERY slot** of both sets (design §2.A(ii); today seven
    software slots and six generic slots ship zero options — including all three weight-12
    software slots). Where a slot asks for a technical/aesthetic decision, the
    `plannerChoosesValue` option leads, first position, label prefixed "You choose" (the
    RW-12 rule, pinned by the existing `TestStackSlotOffersPlannerChoosesDefault`).
  - every slot gets a one-line plain-words why: new field `Slot.Why` (json `why,omitempty`),
    requester-facing, distinct from `MustKnow` (design §2.A(iii); the FE renders it as the
    per-question "why line").
  - `Version: "v3"` on BOTH sets (generic jumps v1→v3 per the design note's letter — OQ-5).
  - `Source` provenance per the `rw12Provenance` pattern: drafting model + date + packet id
    (P3-GF3-BE1), **appended** to the existing record — the CCB citation "2607.00711" and the
    RW-12 strings ("P3-RW-12", "claude-opus-5", "2026-08-13") must remain (existing test pins
    them), and the record states operator ratification **pending the resumed B6 gate**.
  - drafting constraints from existing pins (may not change without sanction):
    `technology_stack` question stays exactly "What should this be built with?" and its first
    option label exactly "You choose for me and show me what you picked"
    (`TestStackSlotOffersPlannerChoosesDefault` pins the strings); every planner-chooses
    option stays first with a "You choose" label prefix; `MustKnow` and `Name` non-empty on
    every slot; weight-shape invariant `TestSeedWeightsMeetTierFloorShape` must stay green
    (automatic with verbatim weights).
  - `Taxonomy.Validate()` stays as-is (0 or 2–4 options): the every-slot-options rule is a
    v3 SEED contract pinned by test, not a schema rule — operator override files and the four
    other family seeds legitimately carry option-less slots.
- **R2** (CONVENTIONS §57, binding: "for the next packet that edits a question set: do not
  update the digests, and do not extend this Ensure — mint your own"): the governance leg.
  - Freeze the RW-12-ratified content as in-code snapshots (the `b2taxonomy_v1.go`
    precedent): re-point `taxonomyContent` in `internal/memory/rw12seeds.go` from live
    `intake.SeedTaxonomies()` to the frozen snapshot for every family RW-12 attests
    (software v2, research/content/data/chore v1, generic v1), so `rw12ContentDigest` stays
    byte-true and `EnsureRW12TaxonomyGovernance` keeps writing exactly what its gate
    ratified. The `rw12ContentDigest` values are **never edited**.
  - Mint `EnsureGF3TaxonomyGovernance` (new file in `internal/memory`), with its own
    originRef (packet P3-GF3-BE1, decision "taxonomy v3", ratification recorded as PENDING
    the resumed B6 gate — OQ-6) and its own content-digest map over the v3 software+generic
    content, superseding the two governed entries by content-inequality exactly as RW-12
    supersedes B2's (row-not-file comparison via `committedContentHash`; unverifiable ⇒
    skip+count; never resurrect a removed entry; `ErrNoOperator` defers).
  - Boot wiring: the GF3 Ensure runs AFTER `EnsureRW12TaxonomyGovernance` (same call site,
    `internal/shell/shell.go`).
  - This leg requires the OQ-1 by-name test sanctions before implementation starts.

### B — Suggestion decoration

- **R3** (design §2.B; S06.5 duty split — "the utility model phrases and summarizes but does
  not decide what must be asked"; S06.10 utility row): seam extension in
  `internal/intake/intake.go`, all additive:
  - `PhraseQuestion` gains `Options []Option` (the seat must see a slot's options to name
    one; RW-12 OQ4 withheld options from PHRASING — suggestion decoration is the new,
    design-ratified consumer).
  - `PhraseResult` gains `Suggestions map[string]string` (per asked slot id: a one-line,
    task-grounded proposed answer) and `SuggestedOptions map[string]string` (per asked slot
    id: an EXISTING option value the suggestion corresponds to).
  - The seam stays OPTIONAL: nil/erroring Phraser ⇒ no suggestions, no new failure mode —
    the landed honest-absence posture (verbatim questions, zero added clicks,
    `logSeatDegrade`) covers the absence unchanged.
- **R4** (design §2.B; CONVENTIONS §26/§57): the local seat half.
  - `local.PhraseSchema` (internal/local/schema.go) extends so the engine-enforced schema
    admits, per asked id ONLY, the phrasing plus the suggestion (and the suggested option,
    enum-constrained to that question's option values where the shape allows);
    `additionalProperties:false` and one-entry-per-asked-id stay — the schema remains the
    BELT, the platform fold the authority (R03 §2.1 via §57).
  - `internal/stage/local.go` `PhraseAndSummarize`: prompt gains the options + the
    suggestion instruction (request text stays quoted as material, never instructions —
    P-T05-3); still ONE duty call per card, one $0 D7 row; `NoThink` stays true;
    `Classification` stays false.
  - **Token headroom (PH-1 discipline):** the output grows (suggestion per question) and the
    review card grows the question count to the full slot set (R8) — the budget must be
    derived from the question count (per-question budget × len(ids) + summary/reason
    headroom) or re-measured; it stays a **structural constant** (S18 declares no key; no
    new ⚙; settings-tab flagged). A hermetic test asserts the budget math admits the full
    review-card call (T6). The landed auto-run live tripwire
    `TestLivePhraseAndSummarize` (internal/stage/phrase_test.go) is NOT modified and keeps
    asserting non-empty phrasings + cap headroom on the D7 row — it will exercise the
    extended schema wherever the stack is installed. The NEW extended-output live
    measurement (full review-card size, suggestion fields, headroom recorded) is a separate
    leg behind a dedicated opt-in env var, GPU stack, serial, per the design note — never
    run in CI and not run by the executor's ordinary battery.
- **R5** (design §2.B): the fold, in `buildInterviewCard` (internal/intake/pipeline.go),
  under the exact containment discipline the function documents:
  - `Question` gains `Suggested string` and `SuggestedOption string` (json
    `suggested`/`suggested_option`, omitempty), folded BY SLOT ID from
    `PhraseResult.Suggestions`/`SuggestedOptions`.
  - an id that was not asked is dropped; question count/order/ids/`Options`/`Text` are
    untouchable by construction; a `SuggestedOption` value that names no existing option
    value of THAT question is dropped (the `Suggested` text is kept); empty-string
    suggestions are not folded.
  - served additively on the wire (ask snapshot → taskView open_card): absent when the seat
    degrades.

### C — Per-slot skip

- **R6** (design §2.C; S06.5's third resolution arm — "converted to an explicit
  assumption"): `SlotAnswer` gains `Skip bool` (json `skip,omitempty`).
  `applyInterviewAnswer` (internal/intake/answer.go):
  - `{id, skip:true}` is valid only for a slot ASKED on this card (same containment as
    `answers`); skip with a non-empty `value` refuses `ErrBadAnswer`; skip of an unasked or
    unknown slot refuses `ErrBadAnswer`.
  - converts THAT slot via the existing `resolveSlot(SlotResolution{How:
    ResolvedAssumption, ...})` — no new state kind, no new resolution arm.
  - the assumption text carries the card's served `Suggested` for that question when one
    exists (read from the CARD SNAPSHOT — the durable ask row — never recomputed); when
    none was served, a plain-language line naming the slot and saying the platform will
    assume a sensible default and show it on the plan (requester-facing prose per
    CONVENTIONS §59: say why in words, no internal tokens; this arm's reason differs from
    the force-proceed and band arms and must say its own thing).
  - Clearance recomputes through the landed `tax.Clearance(st.resolvedSet())` — no new
    computation. A skipped slot is resolved, so the landed `Unresolved` selection never
    re-picks it: the "asks again about the N you skipped" loop dies with zero selection
    changes.
  - on the `!NeedsDraft` merge path, skip resolutions ride `merge.Resolutions`
    (`ReviseInterview`) exactly as answers and assumes do.
  - the whole-interview `force_proceed` arm stays **byte-identical in behavior** (existing
    tests pin it; `ForceProceeded` never set by skip).
- **R7**: the skipped slot's assumption reaches the S06.9 centerpiece through the landed
  path (planner receives `Resolutions`; SPEC assumptions carry `Origin`), and the
  understood block/recap shows it as `how: "assumption"` — nothing new to build, asserted
  by test.

### D — Reinterview review card

- **R8** (design §2.D; S06.9 Re-interview "returns to Stage 1 with artifacts intact" — the
  same-four-questions card is logged implementation choice, replaceable): the
  `ReinterviewRequested` branch (internal/intake/pipeline.go ~:701) issues ALL of the
  active taxonomy's slots — taxonomy declaration order (the understood block's stable
  order) — instead of the first four:
  - each `Question` carries the normal Text/Phrased/Options/Weight plus new fields
    `Why string` (json `why,omitempty`, from `Slot.Why` — also on NORMAL interview cards)
    and `Resolution *UnderstoodItem` (json `resolution,omitempty`): the slot's CURRENT
    resolution (How = registry | answered | assumption, Value/Assumption, plain Name) when
    resolved, nil when unresolved. Composed from `State.Resolutions` by platform code —
    the `understoodBlock` discipline.
  - suggestion + phrasing decoration per R5 applies (the seat call carries the full slot
    list — budget per R4).
  - the NORMAL interview path's selection loop is untouched: up-to-4, highest-weight-first,
    below-floor-only (the ≤4 pin in pipeline_test.go stays green). `maxQuestionsPerCard`
    stays as landed and its doc comment gains one scoping sentence (it bounds interview
    DELIVERY on the normal path; the S06.9 review surface re-presents the whole set —
    OQ-3).
  - `SpecDoubt → adjust_spec` reuses the same branch (it sets `ReinterviewRequested`) and
    therefore gets the same review card — intended, no special-casing.
- **R9**: answering the review card needs no new verbs: any subset of slots may be
  answered/assumed/skipped; answered values overwrite via the landed last-write-wins
  `resolveSlot`; the landed `!NeedsDraft` → `ReviseInterview` merge carries the changes
  into a bounded revision.

### E — Multi-contest replan

- **R10** (design §2.E; S06.9 Re-plan structured entry — §1 logged reading: "structure =
  named targets, not a cardinality of one"; backend `ReviseReq.Findings` already
  `[]string`): `Answer` gains `Contests []ContestRef` (json `contests,omitempty`).
  `applyApprovalAnswer` ActionRePlan:
  - accepts any combination of the legacy single `contest` (back-compat, byte-identical
    semantics), `contests`, and an optional top-level free-text `note` ("what I want
    different, in my words") — validation: at least one contest target or a non-empty note,
    else `ErrBadAnswer` (note-only is valid — OQ-4).
  - each contest folds to a finding "target" or "target: note"; the top-level note becomes
    its own target-less finding; ALL merge into ONE `ReviseReq{Reason: ReviseContest}` via
    the landed `mergeRevise` → exactly one bounded delta re-plan (one `Revise` call), spine
    re-run, NO second paid critique (§14 clearly-implied reading 5) — all landed mechanics.
  - the ledger decision entry names every contested target (human-authored, "approval"
    stage, as landed).
  - the `note` channel conflicts with the RW-19 ratified OQ2 reading of `Answer.Note` —
    see OQ-2; the committed red test follows the design note's wire name (`note`) pending
    the ruling. The `Answer.Note` doc comment (cards.go) must be updated honestly if OQ-2
    resolves to reuse it.

### Floor serve

- **R11** (design §2.F wire line; verified UNSERVED this session — `Card` carries
  `clearance`+`tier` only, `taskView` carries `clearance` only, `web/src/api.ts` has no
  intake floor field): `Card` gains `ClearanceFloor float64` (json
  `clearance_floor,omitempty`), set in `issueCard` from the landed
  `clearanceFloor(st.Tier)` (⚙ values read at issue time; trivial tier yields 0 and the
  field is omitted — the band never asks anyway). One site, every card kind, next to the
  served clearance. The floor VALUE and its consumption stay exactly as landed.

### Cross-cutting

- **R12**: every wire change is additive with `omitempty`; pre-GF3 ask snapshots and state
  events decode unchanged (the `TestPreRW12SnapshotsDecodeAndAnswer` posture). No
  migration — asks/state are JSON payloads.
- **R13**: `web/` is NOT touched (api.ts mirrors + rendering are GF3-FE, design §3 packet
  2). The golden-fixture compare (`go test -p 1 ./internal/api -run TestWebAPIFixtures`)
  is expected UNCHANGED — the fixture worlds' interview cards are hand-seeded literals,
  verified this session; if it trips, regeneration is the deliberate
  `SINET_WRITE_API_FIXTURES=1` act with the diff reviewed, never a reflex.
- **R14**: scope guardrail — no refactors, no new abstractions, validation only at system
  boundaries (answer decode, seat-result fold, taxonomy load), the simplest thing that
  works well.

---

## 2. Seams to respect

- `intake.Phraser` stays a SEPARATE optional seam; nil or erroring seat ⇒ verbatim
  questions, no suggestions, zero added asks, `logSeatDegrade` — never faked content
  (CONVENTIONS §14/§26/§57). The fold in `buildInterviewCard` is the containment
  authority; the engine schema is a belt; never a prompt-level lockout.
- `Planner`/`Critic`/`Classifier`/`Utility`/`Registry` seams: signatures untouched.
- Import walls: `internal/local` never imports intake/stage (AST-pinned
  `importwall_test.go`); the seat adapter lives in `internal/stage/local.go`; `intake`
  never imports `local`. `internal/memory` ↔ `intake` only via the existing direction
  (memory imports intake).
- Run identity: `PhraseInput.RunID` explicit on the seam (consuming run, fork after
  rebind) — unchanged.
- Governance decides from the ROW, never the file (`committedContentHash`); supersession,
  never in-place edit; frozen snapshots, never live pointers (§57).
- Ledger never nests inside an intake WriteTx (§14).

## 3. ⚙ settings consumed (registry name, never hardcoded)

Unchanged set, no new keys (the 118-key S18.5 tally stays green):
`intake.clearance_floor.low` / `intake.clearance_floor.standard` /
`intake.clearance_floor.high` (now additionally SERVED per R11, read at card issue),
`intake.zero_interaction_cost_usd` (per-user `FloatFor`), `intake.size_recheck_factor`,
`intake.coverage_autofix_rounds`, `intake.critique_revise_rounds`, `freshness.max_age`,
`claims.default_write`. Any NEW tunable number this packet wants (phrase budget scaling)
is a structural constant under the standing settings-tab directive — if the executor
concludes a number genuinely needs operator tuning, that is an S18/S00.9 question for the
coordinator, never a code constant AND never an invented key.

## 4. Files expected to change

- `internal/intake/taxonomy.go` — v3 seeds, `Slot.Why` (R1)
- `internal/intake/cards.go` — `Question.{Why,Suggested,SuggestedOption,Resolution}`,
  `Card.ClearanceFloor`, `SlotAnswer.Skip`, `Answer.Contests`, doc updates (R5/R6/R10/R11)
- `internal/intake/intake.go` — `PhraseQuestion.Options`,
  `PhraseResult.{Suggestions,SuggestedOptions}` (R3)
- `internal/intake/pipeline.go` — fold (R5), reinterview branch (R8), `issueCard` floor
  (R11), Question composition carries Why (R8)
- `internal/intake/answer.go` — skip arm (R6), multi-contest (R10)
- `internal/local/schema.go` — `PhraseSchema` extension (R4)
- `internal/stage/local.go` — prompt + suggestion fold-back + budget (R4)
- `internal/memory/` — new GF3 governance file + frozen RW-12 snapshot + `rw12seeds.go`
  re-point (R2, pending OQ-1); `internal/shell/shell.go` — one boot call (R2)
- tests: `internal/intake/gf3be1_test.go` (committed RED by this grounding), plus the
  specs in §8 the current surface could not express
- NOT: `web/`, migrations (0001–…: byte-untouched), `Docs/`, `Spec/`, `Research/`,
  `P3/STATE.md`, `components.lock`

## 5. Adopted components touched

None. No new Go modules, no lock edits, no engine changes. `go.mod` untouched.

## 6. Acceptance headline, decomposed

The headline: *a non-programmer gets plain questions with concrete options and a
recommendation they can take in one click or skip; changing answers re-presents everything
already settled; the plan card takes many objections at once in their own words; and none
of it moves a single core-pipeline number.* Checkable as:

1. Every software+generic v3 slot: plain question ≠ MustKnow text, 2–4 labeled options,
   non-empty why; ids/weights/count/order byte-verbatim vs v2 (T1, T7).
2. Suggestions fold by asked slot id only; ids/count/order/options/text untouchable;
   invalid suggested_option dropped; seat degrade ⇒ absent, zero new failure modes (T2).
3. `{id, skip:true}` resolves THAT slot as an explicit assumption carrying the served
   suggestion; clearance rises per the landed computation; the slot is never re-asked;
   force_proceed behavior byte-identical; the assumption reaches the approval centerpiece
   origin-labeled (T3).
4. Re-interview issues the full slot set with per-slot current resolutions attached;
   normal-path selection untouched (existing ≤4 pin green) (T4).
5. Replan accepts `contests[]` + top-level note; all merge into ONE ReviseContest revision;
   single-contest back-compat; one revise call, no second critique (T5).
6. `clearance_floor` served on every intake card next to `clearance`, from the tier's ⚙
   key (T6a).
7. Phrase budget provably admits the full review-card call (T6b); live tripwire unmodified.
8. Governance: RW-12 digests untouched and still true (frozen re-point); GF3 Ensure
   supersedes software+generic to v3 under its own PENDING-ratification record; boots
   idempotent (T8, pending OQ-1).
9. Batteries: `go build ./...` green; `go test -p 1` green package-by-package (intake,
   local, stage, memory, api); fixture compare unchanged (R13); no new ⚙ (settings tally
   test green); existing named pins green except the OQ-1 sanctioned three.

## 7. CONVENTIONS constraints that bind

- §3 tests: stdlib testing only, colocated, `t.TempDir()` only; SERIAL (`go test -p 1`,
  operator directive); tests-first — the red window is declared in the commit message and
  closed by the implementation commit; `go build ./...` stays green throughout.
- §5 commits: subject `P3-GF3-BE1: … (S06.5, S06.9, S06.10)`; Fable trailer; stage by
  explicit pathspec; never stage `P3/STATE.md`, `Presentation/`, `Research/Presentation/`,
  `Sinet-Logo.jpeg`, `tools/db*`, `tools/rescanseed/`, `tools/driftseed/`,
  `P3/gates/rw10-heal.sh`, `P3/gates/B6-clickthrough.sh`; packet sessions never push.
- §2 settings discipline: no ⚙ value hardcoded; no invented keys.
- §14: intake state is event-sourced full-state payloads; artifact bodies never ride
  payloads; ledger composes before/after intake WriteTx, never inside.
- §26: local-call metering unchanged (ONE $0 D7 row per card call).
- §57: fold-by-id containment; honest absence; snapshot-not-pointer governance; mint-your-
  own-Ensure; `Question.Text` canonical, decorations additive.
- §59: requester-facing prose is for the person — each assumption arm says its own reason
  in words, no internal tokens.
- Paid/live discipline: no paid calls anywhere in this packet; the new live measurement
  leg is opt-in-env, GPU stack, serial; hermetic tests must not start engines or servers.

## 8. Acceptance-test specifications

Committed RED this grounding (file `internal/intake/gf3be1_test.go`, package
`intake_test`, reusing `newFix`/`fakePhraser`/`stdRequest` from the landed suite — same
package, no existing file modified). Verified failure reasons are recorded in the red
commit message.

- **T1 `TestGF3TaxonomyV3EverySlotCarriesOptionsAndWhy`** (RED): for software+generic
  seeds, every slot has 2–4 options and non-empty `Why`. Fails today: seven software and
  six generic slots carry zero options; no slot carries a why. (The `Version=="v3"`
  assertion is deliberately NOT here — it contradicts the existing v2 pin until OQ-1 is
  ruled; the executor adds it to this test in the implementation commit together with the
  sanctioned one-line edit.)
- **T2 `TestGF3SuggestionsFoldBySlotIDUnderContainment`** (RED): fixture with
  `fakePhraser` returning, per asked id, `Suggestions[id]="SUGGEST-"+id`, plus a
  suggestion for `a_slot_nobody_asked`, plus `SuggestedOptions` naming a REAL option value
  on `technology_stack` and a bogus value on `collection_semantics`. Asserts: every asked
  question carries its `Suggested`; the unasked id reaches nothing; `SuggestedOption`
  present only where it names an existing option value of that question; question
  count/order/ids/options/text equal the taxonomy's own selection. Fails today:
  `Suggested` never folded (empty).
- **T3 `TestGF3PerSlotSkipConvertsToAssumptionCarryingTheServedSuggestion`** (RED): seat
  serves suggestions; answer card 1 with `{id:q0, skip:true}` + three real answers.
  Asserts: no error; `Resolutions[q0].How==ResolvedAssumption` with the served suggestion
  text inside; `ForceProceeded` false; clearance equals the landed
  `tax.Clearance(resolvedSet)` recomputation; the next interview card never re-asks q0;
  after the floor, the approval card's Layer1 assumptions and understood recap carry the
  skipped slot's suggestion, origin-labeled. Sub-cases: `skip` with non-empty value ⇒
  `ErrBadAnswer` (RED — resolves as answered today); `skip` of an unasked slot ⇒
  `ErrBadAnswer` (holds today via the empty-value check — documented green, kept as the
  contract). Main path fails today: `ErrBadAnswer: empty answer for q0`.
- **T4 `TestGF3ReinterviewIssuesTheFullReviewCard`** (RED): walk to the approval card,
  answer `ActionReInterview`. Asserts: next card is `CardInterview` carrying ALL the
  family's slots in taxonomy order; every slot resolved in state carries `Resolution`
  matching `State.Resolutions` (how/value/assumption + plain name); unresolved slots carry
  nil; `Why` equals the slot's `Why`; answering a changed subset merges via
  `ReviseInterview` into one revision (planner capture). Fails today: 4 questions, not the
  full set.
- **T5 `TestGF3MultiContestReplanMergesIntoOneRevision`** (RED): walk to approval; capture
  `ReviseInput` via the planner fake; answer
  `{action:"replan", contests:[{target:"AC-1",note:"wrong"},{target:"S-2"}], note:"free text"}`.
  Asserts: no error; exactly ONE revise call, reason `ReviseContest`; findings contain
  "AC-1: wrong", "S-2", and the top-level note as its own finding; critique still ran
  exactly once (no second paid pass); the ledger decision names the targets. Companion
  `TestGF3ReplanBackCompatAndRefusals`: single `contest` still works (green today,
  guard); `{action:"replan"}` with no contest/contests/note refuses `ErrBadAnswer` (green
  today, guard); note-only replan accepted (RED today — OQ-4). Main path fails today:
  `re-plan needs the contested target`.
- **T6a `TestGF3CardsServeTheClearanceFloor`** (RED): standard-tier fixture: interview
  card and approval card both carry `clearance_floor == 75` (the ⚙ standard default) next
  to `clearance`. Fails today: field absent (0).
- **T6b** (executor adds; hermetic, internal/stage or internal/local): the phrase budget
  math admits a full review-card call — for len(ids) == the largest seeded slot count, the
  configured budget ≥ the per-question cost model the executor derives; asserts the
  constant cannot silently under-provision the R8 card. Plus: the extended live
  measurement leg behind its dedicated opt-in env (never CI; GPU; serial), recording
  cold/warm wall time and output-token headroom to `P3/measurements/`.
- **T7 `TestGF3WeightsAndIDsStayVerbatim`** (committed GREEN guard): pins the exact
  id→weight map, slot count and order for software+generic — the "measured thing,
  untouched" wall for the v3 redraft.
- **T8** (executor adds, internal/memory, pending OQ-1): GF3 governance — fresh world:
  B2 Ensure → RW-12 Ensure (writes the FROZEN v2/v1 content; digests true) → GF3 Ensure
  supersedes software+generic to v3 under the GF3 originRef with PENDING-ratification
  wording, `supersedes_id` chained, old rows retired; second boot no-op; removed entries
  stay dead; governed files LoadTaxonomy-clean and DeepEqual the v3 seeds; RW-12's
  `rw12ContentDigest` map byte-untouched (git diff empty on those lines);
  `TestRW12DigestsMatchTheShippedSeeds` passes UNMODIFIED (it verifies the frozen
  snapshot after the re-point).
- **T9** (executor adds): honest-absence regression for suggestions — erroring seat ⇒ no
  `suggested`/`suggested_option`/`phrased` anywhere on the card, zero added asks, degrade
  logged once (extends the landed absence posture; the landed
  `TestPhrasingAbsenceIsVerbatimZeroClicks` must stay green untouched).
- **T10** (executor adds): pre-GF3 snapshot compatibility — a pre-GF3 ask snapshot and
  state event (no new fields) decode and answer exactly as landed (the
  `TestPreRW12SnapshotsDecodeAndAnswer` pattern; do not modify that test — add a GF3
  sibling).

Red-window mechanics (CONVENTIONS §3 Amendment-A): the red commit carries the inert type
surface (new struct fields nothing reads: `Slot.Why`, `Question.{Why,Suggested,
SuggestedOption,Resolution}`, `Card.ClearanceFloor`, `SlotAnswer.Skip`, `Answer.Contests`,
`PhraseQuestion.Options`, `PhraseResult.{Suggestions,SuggestedOptions}`) so
`go build ./...` stays green while T1–T6a fail; behavior lands only in the implementation
commit, which closes the window.

## 9. Existing test files near this code — the executor may NOT modify them

(Exception: the three by-name sanctions requested in OQ-1, and only after the coordinator
grants them.)

- `internal/intake/pipeline_test.go` — the S06 battery + the shared `fix`/fakes; pins the
  NORMAL interview card at ≤4 questions and weight order (must stay green under R8).
- `internal/intake/taxonomy_test.go` — pins software `Version=="v2"` (OQ-1 item 1), the
  `technology_stack` strings, planner-chooses rules, weight-shape floors, CCB citation +
  RW-12 provenance strings in Source.
- `internal/intake/deepplan_test.go` — phrasing fold containment, honest absence,
  understood blocks, consuming-run metering, pre-RW-12 snapshot decode. Compares cards
  RELATIONALLY against live seeds — stays green under v3.
- `internal/intake/familygate_test.go`, `familydrain_test.go`, `triageadvance_test.go`,
  `triageproperty_test.go`, `triggers_test.go`, `artifact_test.go`, `pin_test.go`,
  `rebind_test.go`, `registry_ledger_test.go`, `route_test.go`,
  `routewrite_rw14_test.go`, `citations_test.go`, `citations_internal_test.go`,
  `degradelog_test.go`, `cancelmint_rw18_test.go`, `cancelwhy_rw19_test.go` (pins the
  Note-ignored-outside-cancel property WITHOUT covering replan — stays green under R10
  either way of OQ-2).
- `internal/stage/phrase_test.go` (`TestPhraseAdapterMetersOnConsumingRun` +
  `TestLivePhraseAndSummarize` — auto-runs where the stack is installed; question-text
  literals there are fake INPUTS, not seed pins), `internal/stage/nothink_test.go`,
  `internal/stage/local_test.go`, `internal/stage/livetriage_test.go`.
- `internal/local/local_test.go` (schema shapes), `duty_test.go`, `truncation_test.go`,
  `importwall_test.go`.
- `internal/memory/rw12seeds_test.go` (OQ-1 items 2+3),
  `internal/memory/rw12snapshot_internal_test.go` (`TestRW12DigestsMatchTheShippedSeeds` —
  survives UNMODIFIED given the R2 re-point), `internal/memory/seeds.go`-side B2 tests
  (`TestB2GovernedTaxonomiesAreTheRatifiedSnapshot` — frozen fixtures, stays green).
- `internal/api/apifixtures_test.go` + `web/src/fixtures/api/*` — golden compare; expected
  unchanged (hand-seeded cards), see R13.

## 10. Open questions / conflicts — REPORTED, not resolved

- **OQ-1 (blocking for piece A only): taxonomy v3 cannot land green inside the design
  note's named file scope.** The design note (§3 packet 1) names "internal/intake +
  internal/local + internal/api — additive; existing tests untouched per amendment A".
  But CONVENTIONS §57 binds any packet that edits a question set to freeze the prior
  packet's snapshot and mint its own governance in `internal/memory` (+one boot line in
  `internal/shell`), and three landed tests hard-pin the current content this session
  verified:
  1. `internal/intake/taxonomy_test.go` `TestSeedTaxonomiesCoverSixFamilies` — the literal
     `soft.Version != "v2"` check (one-line edit to "v3").
  2. `internal/memory/rw12seeds_test.go` `TestTaxonomyGovernanceCreatesAndSupersedes` —
     DeepEqual of governed files against LIVE `intake.SeedTaxonomies()` (must move onto
     the frozen RW-12 content, or additionally run the GF3 Ensure — the §57-recorded
     precedent: "the test that guarded it moved onto…", and rw12seeds_test.go's own
     comment anticipates exactly this trip).
  3. `internal/memory/rw12seeds_test.go` `TestGovernanceDecidesFromTheRowNotTheFile` —
     tears the write with LIVE-seed bytes and asserts the superseding version equals them
     (must tear with the frozen v2 bytes instead).
  Everything else stays green (verified: digests test survives the re-point; B2 snapshot
  tests, deepplan relational tests, weight-shape test, apifixtures all unaffected).
  **The coordinator must either grant these three by-name edit sanctions plus the
  internal/memory + internal/shell scope (recommended — the §58-R1 precedent), or
  re-scope piece A out of this packet.** There is no third option: landing v3 seeds
  without the governance leg reds the suite.
- **OQ-2: the replan free-text `note` collides with the RW-19 ratified OQ2 reading.**
  `Answer.Note` is ratified as honored ONLY on the two cancel-shaped answers ("everywhere
  else it is ignored" — cards.go doc, api.ts doc, pinned by
  `TestIntakeNoteIsIgnoredOutsideTheCancelShapedAnswers`, which does not cover replan and
  stays green either way). The design note orders "an optional top-level `note`" on the
  replan answer. Brief + red test follow the design note's wire name; the coordinator
  confirms the OQ2 reading is extended to "the two cancel-shaped answers plus the replan
  free-text channel" (doc comments updated honestly), or names a distinct field (one
  string in the red test changes).
- **OQ-3: the review card vs S06.5's ratified "up to 4 questions per card".** The design
  note's logged reading treats the S06.9 Re-interview surface as replaceable
  implementation, but S06.5's delivery sentence is ratified text and the landed
  `maxQuestionsPerCard` comment calls it spec-structural. This brief scopes the 4-question
  bound to the NORMAL interview delivery (fresh asking) and treats the review card as the
  S06.9 verb's own surface (the CardFamily precedent: a card that is not interview
  DELIVERY is not bound by the interview delivery shape) — same CardKind per the design
  note ("same card kind, richer content"). Coordinator confirms this reading, or the
  review card becomes its own kind.
- **OQ-4: note-only replan (zero contest targets).** S06.9 says "structured entry: tap
  the AC, assumption, or step being contested"; the design's always-present textarea
  implies a send with no chips selected. Brief specs it VALID (one target-less finding).
  Confirm or require ≥1 target.
- **OQ-5: generic version label jumps v1 → v3** (design: "software + generic revised to
  v3"). Taken literally so the GF3 revision carries one label across both sets. Confirm
  (a "v2" for generic would instead need a design-note correction).
- **OQ-6: governing v3 under a PENDING ratification record.** RW-12 governed content its
  gate had already ratified; the design note orders v3 ratification AT the resumed B6
  gate, after the packet lands. The brief specs: the GF3 Ensure runs at boot with an
  originRef that says ratification is PENDING the resumed gate (honest record; the gate
  ruling then stands in STATE). The alternative — defer the Ensure until after the gate —
  leaves the governed file at v2 while the runtime serves v3, the exact live-pointer
  drift §57 closed. Confirm the ship-with-pending-record posture.
- **Noted, no ruling needed:** the design note's "opt-in env" live leg coexists with the
  landed §57 auto-run tripwire by keeping the tripwire unmodified (it will exercise the
  extended schema wherever the stack is installed) and putting only the NEW extended
  measurement behind the opt-in env; the landed reinterview branch's "top-weight" comment
  does not match its declaration-order behavior — R8 replaces the branch, mooting it.

---

## 11. Coordinator rulings on §10 (2026-08-23, before executor launch — the RW-19 precedent)

- **OQ-1 — GRANTED as recommended.** Packet scope WIDENS to `internal/memory` (the
  `rw12seeds.go` re-point onto frozen RW-12 content + the new `EnsureGF3TaxonomyGovernance`
  file) and ONE boot line in `internal/shell/shell.go` (GF3 Ensure after the RW-12 Ensure).
  **Three by-name edit sanctions, exactly and only:**
  1. `internal/intake/taxonomy_test.go` `TestSeedTaxonomiesCoverSixFamilies` — the one-line
     software version pin `"v2"` → `"v3"` (plus the same pin for generic if that test carries
     one).
  2. `internal/memory/rw12seeds_test.go` `TestTaxonomyGovernanceCreatesAndSupersedes` —
     re-point its expectation onto the frozen RW-12 snapshot content (and/or additionally run
     the GF3 Ensure), per the §57-recorded precedent.
  3. `internal/memory/rw12seeds_test.go` `TestGovernanceDecidesFromTheRowNotTheFile` — tear
     with the frozen v2 bytes instead of live-seed bytes.
  No other landed test may change. `rw12ContentDigest` values are never edited; the digest
  tripwire survives unmodified. The design note's §3 file scope is corrected by this ruling
  (recorded in STATE).
- **OQ-2 — EXTEND the reading.** `Answer.Note` is honored on the two cancel-shaped answers
  AND on `ActionRePlan`, where it is the free-text contest channel ("what I want different,
  in my words"); it stays ignored everywhere else. One requester-words field with per-action
  meaning beats a parallel second string. Update the `cards.go` and `api.ts` doc comments to
  say exactly this, honestly. The RW-19 pinned test stays green (it does not cover replan);
  the RW-19 ratified record is EXTENDED, not contradicted — STATE logs the extension.
- **OQ-3 — CONFIRMED as briefed.** S06.5's "up to 4 questions per card" governs interview
  DELIVERY (fresh asking, highest-weight-first, below-floor). The S06.9 Re-interview verb's
  review surface re-presents the whole set with current resolutions — a review-and-adjust
  act the requester explicitly requested, not delivery of unresolved questions (the
  CardFamily precedent). Same CardKind, richer content; `maxQuestionsPerCard` doc gains the
  one scoping sentence. Reading logged in STATE.
- **OQ-4 — CONFIRMED: note-only replan is VALID** (one target-less finding). The structured
  entry remains the primary affordance; the requester's own words entering the same bounded
  delta re-plan serves S06.9's purpose (bounded re-plan, delta re-approved). Reading logged.
- **OQ-5 — CONFIRMED as the design note's letter:** generic jumps v1 → v3; "v3" = the GF3
  revision label across both revised sets. Version labels are provenance, not per-set
  sequence.
- **OQ-6 — CONFIRMED: ship-with-pending-record.** The GF3 Ensure runs at boot with an
  originRef stating operator ratification is PENDING the resumed B6 gate; the gate ruling
  then stands in STATE. Deferring the Ensure would recreate the exact live-pointer drift
  §57 closed. If the gate REFUSES v3, the refusal is executed as its own supersession back
  to the frozen v2 content (governance machinery already supports it) — the pending record
  makes that path honest.
