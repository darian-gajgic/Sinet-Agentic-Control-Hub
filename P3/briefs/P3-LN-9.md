# P3-LN-9 — the per-task lane pin, backend half

**Packet:** P3-LN-9 · **Phase:** LN (lane campaign) · **Grounding:** 2026-08-26
**Carries:** S00.9 amendment **A13** (full draft text at §10)
**Spec sections that bind:** S08.8 (routing), S10.4 (pressure), S15.2 (API surface), S00.9 (amendment mechanics), S01.9 (identity/roles), S12.1 (local tier class (a))
**Sole scope:** backend + API. No `web/src` application source. No door/ceremony/walk changes (P3-LN-10 owns them).

---

## 0. Why this packet exists, in one paragraph

The operator holds ONE Kimi Code membership reachable by two paths and ordered a head-to-head: which path performs better. P3-LN-8 established the comparison is **not runnable** by any existing mechanism. The two lanes draw one quota pool (`kimi-code-membership`, declared once on `internal/metering/plandata/kimi.json:44`), so their consumption-pressure ratios are identical **by construction**; `chooseFlatLane` (`internal/worker/routing.go:807`) takes the strictly less-consumed lane and keeps the earlier candidate on a tie; the candidate order puts `kimi` before `kimi-cli`; one placed credential commissions both (§67), so neither can be held alone; and **no surface anywhere takes a lane from a person**. The operator ruled: build a per-task lane pin. This packet is the backend half; P3-LN-10 builds the picker.

---

## 1. Ground truth established this session

Everything in this section was read from the tree or the spec **this session**. Where the packet's own commissioning assumptions turned out to be wrong, that is stated rather than smoothed over.

### 1.1 The spec already ratifies the override — this amendment is narrower than it looks

S08.8, verbatim: *"**Visible and overridable:** the selected worker and its plain-language reason appear on the plan/approval card; the requester can re-route or **pin** before execution; overrides are recorded with their actor."*

So the pin is **not** a new power. Two things are genuinely new and A13 records exactly those two: the pinnable **axis** is named as the *lane* (S08.8's sentence does not name an axis, and the shipped machinery implements it for the *worker* axis only), and the pin may be declared at **task creation**, ahead of the first selection, not only as a card override. Note also S08.8's actor word is **"the requester"**, not "the operator" — this settles R6 below.

### 1.2 A pin mechanism already exists, and reusing its flag would be a silent semantic change ⚠

`intake.RouteBlock` already carries `Pinned bool` and `OverriddenBy string` (`internal/intake/route.go:81-82`), `intake.RouteOverride{Target, Pin}` already exists (`route.go:93-103`), `Decision.Cause` already documents `override | pinned` (`internal/worker/routing.go:331`), and `routing.decided` already serializes an `override {pinned, actor}` block (`internal/stage/router.go:151-168`).

**But that pin is the WORKER axis.** `RouteOverride.Target` is a template id or `"generalist"` — never a lane. And `Pinned` means one specific thing: *freeze the worker choice against re-plan recompute* (`route.go:156-185`). Crucially, even on a pinned card the **lane is taken from the fresh recompute** (`route.go:168` `kept := fresh`, and only worker identity is overwritten).

> **Trap, stated so the executor does not walk into it:** setting `RouteBlock.Pinned = true` for a lane pin would ALSO freeze the worker choice, which nobody asked for and which breaks R2. The lane pin gets its **own** additive member; `Pinned`/`OverriddenBy` keep exactly today's meaning.

### 1.3 The task-creation verb is not what the packet assumed

**There is no `POST /api/tasks`.** The only task-creating ingress is `POST /api/intake/requests` (`internal/api/api.go:605` → `internal/api/intake_handlers.go:120`), a thin pass-through handing `json.RawMessage` to `Surface.Submit`. The real request struct is `submitBody` (`internal/stage/surface.go:157-174`): `title`, `text`, `inputs,omitempty`, `project,omitempty`.

`submitBody.Project` is the **exact precedent** for what R5 needs, doc comment verbatim: *"Project OPTIONALLY pins the request to a registered project by registry id, ADDITIVE (S15.2 …). The id is validated server-side at the registry seam … and an invalid pin refuses the submission rather than quietly dropping it."*

### 1.4 No migration is needed

`intake.Request` (`internal/intake/intake.go:123-139`) carries `Inputs` and `Project`, and **neither has a `tasks` column**. The `tasks` table (`0001_core_schema.sql:23-29`) has never been altered. Durability comes from the doc comment at `intake.go:128-131`: *"The State carrying this Request is marshaled whole onto every intake.state event, so recording them here IS recording them on the intake record."* A lane pin on `Request` is **durable for free**. Migrations stay at `0001–0025`.

### 1.5 Exactly two production call sites of `Route`

| # | Site | Context |
|---|---|---|
| 1 | `internal/stage/router.go:36` (`skeletonRouter.RouteTask`) | intake-time task routing, fed by `intake.routeQueryFor` (`internal/intake/route.go:111`) |
| 2 | `internal/stage/helpers.go:392` (`routeHelper`) | helper spawn — the SAME pipeline, per S08.8 step 5 |

`worker.RouteQuery` (`routing.go:273-315`) has **no lane/pin field of any kind**. A repo-wide grep for `PinnedLane|lane_pin|forceLane|laneOverride|preferredLane` returns one false positive (`internal/metering/local_test.go:65`, about metering attribution).

`routeHelper` builds a **fresh** query from `req.Brief` and never reads `intake.State` — so a task pin reaches helpers only if it is threaded deliberately (R7).

### 1.6 ⚠ THE FINDING THAT RESHAPES THIS PACKET: the receipt's Lane column does NOT show the lane that ran

The packet's commissioning note said the receipts Lane path *"should need NO change — verify"*. **Verified, and it is false.**

- Rendered `web/src/TaskDetail.tsx:1291,1307`; built `internal/metering/receipt.go:130-180`; value origin `internal/metering/ledger.go:321-322`:
  `SELECT c.model_id, r.lane, … FROM checkpoints c JOIN runs r ON r.run_id = c.run_id`
- `checkpoints` has **no lane column** (`0001_core_schema.sql:95-113`).
- `runs.lane` is written **once**, at run birth, from process config: `internal/stage/skeleton.go:522-528` `launchRole(…)` sets `Lane: s.cfg.Lane`. There is **no `UPDATE runs SET lane` anywhere in the tree**.
- The decided lane is consumed only to pick the engine: `substrateFor` (`internal/stage/runner.go:377-400`), whose own comment says the run row *"is stamped at CREATION … long before routing has chosen anything."*

**Consequence:** today a run that routes to `zai`/`kimi`/`kimi-cli` still meters and receipts as `anthropic`. This is a **pre-existing honesty defect** that the pin makes load-bearing, because the operator was told (HANDOFF, LN-8 walk) that *"the receipt's Lane column is the proof."* It is not. `routing_quality` (`0016_queryable_history.sql:288` vs `:298`) is the one place the divergence is already visible, carrying both `lane` and `run_lane`.

The STATE row's acceptance headline already sanctions the fix in its own words — *"receipts unchanged in shape — the Lane column simply shows the truth"* — so R9 below closes it, narrowly.

### 1.7 Coverage is process-scoped and includes the configured lane by construction

`internal/stage/skeleton.go:128-131`:
```go
Coverage: worker.Coverage{
    FlatRateLanes:  append([]string{cfg.Lane}, cfg.CommissionedLanes...),
    LocalAvailable: cfg.LocalAvailable,
},
```
`cfg.CommissionedLanes` comes from `opencode.CommissionedLanes` (`internal/adapters/opencode/lane.go:808`), explicitly the **union across every person** (§65: *"Coverage stays the union across people, with its recorded over-approximation; per-person coverage is B6/v1"*). The per-person commissioned map exists one level up (`internal/shell/engineadapters.go:225` `broker.PlacedEngineCreds`) but is keyed by the broker `who`, whose relationship to `auth.User.ID` is **unsettled** and is already LN gate-batch item 8. See **OQ-2**.

### 1.8 `local` is not a lane in the registry at all

`internal/adapters/opencode/lanedata/` holds exactly `kimi.json`, `kimi-cli.json`, `zai.json`. `CommissionedLanes` can never return `"local"`, so `Coverage.laneCovered("local")` is **always false** — for a reason that has nothing to do with flat-rate coverage. The local tier is addressed by **duty alias → seat** (`internal/local/alias.go:13-25`), not by lane, and `resolveSeat` (`routing.go:657-667`) always rewrites the duty to execution and rides the paid seat because the local ENGINE lane (S12.1 class (a)) has no commissioned provider entry.

### 1.9 The reason string is a single choke point

`resolveSeat` (`internal/worker/routing.go:715-728`) is where the lane sentence is minted. Every downstream surface copies `plain_reason` **verbatim**: the approval card (`web/src/Intake.tsx:2506-2507`), Workforce rows (`web/src/Workforce.tsx:684-688`), the override ledger decision (`internal/intake/answer.go:428-430`), the `routing.decided` event, the `routing_quality` view, and three history catalog queries (`internal/history/catalog.go:452,485,507`).

**This is why R1 is satisfiable with zero `web/src` changes**: putting the pin in the reason sentence puts it on every surface that renders a reason.

### 1.10 Fixtures

`web/src/fixtures/api/` is a **backend-owned contract snapshot** (`internal/api/apifixtures_test.go:71`), byte-compared by `TestWebAPIFixtures` (`:2289`), regenerated only through `SINET_WRITE_API_FIXTURES=1 go test ./internal/api -run TestWebAPIFixtures`. `web/src/doubles.ts:54-65` states the binding: *"THE BODIES ARE THE GOLDEN FIXTURES."*

Per §63-R3: an `omitempty` member no fixture exercises **is a contract nobody agreed to**. So the fixture world must gain a lane-pinned task — which means **the fixtures WILL move, and vitest is owed at landing** (standing rule / gate item 12). This is an expectation, not a possibility.

The route-inventory sweep (`web/src/sweep.test.tsx`) only trips on a new **route**. This packet adds a **field**, so no reported-gap entry is owed.

### 1.11 Spec assembly, verified this session

`cd Spec && awk 'FNR==1&&NR>1{print""}{print}' drafts/S[0-9]*.md > core-architecture-v1.md` reproduces the committed assembled file **byte-identically right now**: sha256 `31f73c56a326aee76ed7089bd4db25d730d4645e9379746a80d3ed71ade5b75c` on both sides. (Earlier briefs cite `f3332d47…`/`8b4b0ef4…`; those are older trees. Use the number you measure, not this one.)

---

## 2. Numbered requirements

### R1 — A pinned lane is honored by selection, at the real call sites [S08.8]

`worker.RouteQuery` gains `PinnedLane string`. In `resolveSeat`, after duty resolution and **before** `chooseFlatLane`:

- Resolve the pin against the seats available to that duty: `r.DutyMap[duty]` ∪ `r.Alternates[duty]`. A seat whose `Lane` equals the pin wins.
- When the pin binds, **`chooseFlatLane` is skipped entirely** — the exact precedent the template model pin already sets (`routing.go:690-697`: *"A pinned model skips this entirely: the template asked for one model, and offering it a different lane's model would not be honoring the pin."*). The same sentence is true of a lane.
- The existing coverage gate at `routing.go:699-708` stays **untouched** and applies to the pinned seat too (R3's third layer).

**Proof discipline (CONVENTIONS §63 R1, non-negotiable):** *"a correct function reached through a dropped argument is a broken feature."* Every guard drives the **real call sites** (`internal/stage/router.go:36` and `internal/stage/helpers.go:392`) through a composed stage, never `resolveSeat` in isolation, and every one is mutation-verified.

### R2 — The routing reason states the pin honestly, everywhere reasons surface [S08.8 7.7; 14.3]

The sentence is minted at the one choke point (§1.9). Derive the wording from the existing conventions in that function; a serviceable form:

> `Lane %q is pinned on this task, so selection honored the pin instead of comparing consumption pressure across the %d covered flat-rate lanes (S08.8 visible-and-overridable; never dollars — D5).`

Constraints on the wording: it must name the lane; it must say the pressure comparison was **replaced**, not merely skipped; and it must not imply a price was considered. Where both kimi lanes are covered, the reason SHOULD note that a pin between them does not change the allowance because they share one pool (§1, `pool_note`) — otherwise an operator reading a pin's receipt will misread the pool.

Additionally: `worker.Decision` gains `LanePin string \`json:"lane_pin,omitempty"\`` and `intake.RouteBlock` the mirror, copied in `blockFromDecision`/`decisionFromBlock` (`internal/stage/router.go:55-102`). This is the structured member LN-10's picker binds to; the *reason string* is what makes R1 visible today with no `web/src` change.

### R3 — A pin the platform cannot honor REFUSES loudly, and never falls back [S15.2; the `Request.Project` precedent]

**One predicate, three layers** — the ratified `metering.PlanPoolRefusal` shape (§67), applied verbatim in structure:

1. **Boundary.** `intake.Pipeline.Start` refuses **before** the task/run/event birth transaction (`internal/intake/pipeline.go:216-217,252`), so no task is born believing it got a lane it did not get. New sentinel family beside `ErrPinRefused` (`intake.go:532-547`), mapped in `mapIntakeErr` (`internal/stage/surface.go:39-53`) to a **4xx with a code and a message naming the lanes that ARE pinnable** — the shape `meters_verbs.go:407-410` already uses (*"lane %q declares no window %q. Its windows are: %s"*).
2. **Carried, never re-derived.** `internal/intake` may not import `internal/worker` (`route.go:12`). The pinnable set reaches it as **data through a seam**, exactly as LN-6's `PlanWindows` seam hands `internal/api` the declared windows (§66: *"a defect check standing in for an input check"* is the thing to avoid). The seam is composed where `Coverage` is composed (`skeleton.go:120-133`) so the pinnable set has **one spelling** (§65 D4: *"a rule spelled twice drifts"*).
3. **Selection re-checks.** `resolveSeat` refuses a pin it cannot honor even if it arrives by some other route, so a planted pin cannot steer dispatch.

Refusal is **not** the existing degrade-and-explain posture, and the inversion is the point: routing degrades when the **platform** chose a lane it cannot use; it refuses when a **person** did.

### R4 — D5 untouched; pressure accounting stays honest [D5; S10.2; S10.4]

- No money-shaped member enters selection. The D5 reflective scan and the source scan asserting `routing.go` names no price/cost/metering symbol (§63) must stay green with the new field.
- The pin **replaces** the pressure comparison; it does not add a currency to it.
- Consumption lands on the lane that **ran**. Combined with R9, a pinned `kimi-cli` dispatch meters as `kimi-cli` and still aggregates into the shared `kimi-code-membership` pool reading (`internal/metering/planunits.go:435-461`) — the pool predicate is untouched, and a test must assert the pooled reading still sums both lanes under a pin.
- The canonical-lane plan-budget refusal (`PlanPoolRefusal`) is untouched: a pin does not make the sibling lane budgetable.

### R5 — Additive on the wire, on the existing verb [S15.2 "evolution is additive-first"]

- `submitBody.PinnedLane string \`json:"pinned_lane,omitempty"\`` (`surface.go:157`) and `intake.Request.PinnedLane` (`intake.go:123`). **Value type + `omitempty`, never a pointer** — `""` is an unambiguous "no pin", and pointers appear only on response structs in this codebase.
- Goldens **net-additive**; regenerate only via `SINET_WRITE_API_FIXTURES=1`. The fixture world gains a lane-pinned task so the members are exercised (§1.10). Any pre-existing member whose value moves is **named explicitly** in the CONVENTIONS entry — §64 C3 / §66 are standing corrections against a fixture claim that overstates its own innocence.
- **`web/src` application source is untouched**; when the fixtures move, the **frontend battery is owed at landing** (gate item 12).

### R6 — Role posture: the REQUESTER may pin, scoped by refusal [S08.8; S01.9; D10]

**Determined, not open.** S08.8's own word is *"the requester can re-route or pin"*, D10 is *"everyone approves their own objects"*, and S01.9 knows exactly one role bit (*"one role bit (operator vs member) implements D10 co-approval"*) whose documented job is D10 co-approval, not task authorship. So: **any task creator may pin their own task**, and no operator-only gate is invented.

What makes that safe is R3, not a role check: a person can only pin a lane the platform will refuse to withhold. The residual — that the pinnable set is a union rather than per-person — is **OQ-2**, not a reason to invent a role rule the spec does not have.

### R7 — The pin binds helpers; ceremony seats are untouched [S08.8 step 5; S07.5]

**Derived, not open.** S08.8 step 5: *"helper worker/model/lane selection uses this same pipeline."* The pin is a fact about the **task**, and a comparison split between a coordinator on one lane and its helpers on another is not a comparison. So `routeHelper` (`helpers.go:390`) reads the task's pin — the same `s.pipe.LoadState(ctx, r.TaskID)` read `executeRouting` already performs (`router.go:120`) — and sets it on its `RouteQuery`.

**Ceremony duties are unaffected and this must be stated in the code**: planning and judge never pass through `Route` at all (they resolve via `s.seat(duty)`), they are anthropic-only by B3-gate ratification, and the judge additionally carries S07.5's capability-≥-executor and different-model-than-the-executor bars. Seating a pinned lane there would be **inventing a ratification** (§63). A pin therefore binds the **execution** dispatch only.

`routeHelper` also currently emits `executeRouting{Decision: decision}` with a zero `Override` (`helpers.go:380`), so a helper's `routing.decided` carries no pin bookkeeping — with `Decision.LanePin` (R2) it does, for free.

### R8 — Named interactions, each decided or explicitly OQ'd

**(a) Gate/ask cards mid-run; parked and resumed runs keep the pin.** A resumed run re-reads the recorded block (`executeRouting`, `router.go:118-131`), and a **re-plan** recompute re-reads `st.Req.PinnedLane` fresh (`routeQueryFor`, `route.go:111`). So the lane pin survives re-planning **by construction** and needs none of the worker pin's freeze mechanic (`route.go:156-185`) — which is precisely why R2 must not reuse `RouteBlock.Pinned`. Assert both directions.

**(b) The local tier is NOT pinnable at v0.** §1.8: `local` is in no lane document, `laneCovered("local")` is always false, and honoring the pin could only mean riding the paid seat — the silent fallback R3 forbids. It refuses **in its own words**: borrowing the 2.7 subscription-gap sentence would tell an operator to buy a subscription they already hold. Cite S12.1 class (a). (Note the render side is already ready: `web/src/Intake.tsx:2453` special-cases `lane === 'local'`.)

**(c) The benchmark direct arm is EXEMPT.** `internal/benchmark/pair.go:36-37` (`directSubstrate = "claude-cli"`, `directLane = "anthropic"`) are structural constants naming which substrate IS the requester's frontier surface, and `pair.go:160-166` births the direct-arm run **without consulting `Route` at all**. The arm's identity *is* its lane; a task pin moving it would corrupt the blind-pair protocol. Assert the direct arm's lane is unchanged under a pinned task.

**(d) The chat handoff does not carry the pin, on precedent.** `chatHandoff` (`internal/api/chatapi.go:453-457`) builds its submit payload from an **inline anonymous struct** carrying only `title`/`text`/`inputs` — it does not reuse `submitBody`, and it already carries no `project` pin either. Leaving it alone matches the `Project` precedent exactly. **Name it in the CONVENTIONS entry as not-landing with its reason** rather than letting the next reader discover it. (This literal is the single easiest thing in the packet to miss.)

### R9 — The run row tells the truth about the lane that ran [S10.1; the STATE acceptance headline]

Close §1.6: when the execute run's selection settles, stamp the **decided** lane onto the run row so `runs.lane` names the lane that actually ran. Then the receipt's Lane column, `MissionControl`, `Filters` and `RunListItem.Lane` all stop lying, and `routing_quality`'s `lane`/`run_lane` pair agrees.

Bounds on this requirement, so it stays a correction and not a redesign:

- The row is stamped once, from the settled decision, on the path that already resolves it (`executeRouting` → `substrateFor`). `internal/run` has no lane-update verb today (it updates state/generation/lease/workspace_ref only) — one is added, respecting that package's fencing/generation discipline.
- **Interaction with R10, stated:** in the default world nothing is commissioned, so the decided lane **equals** `cfg.Lane` and the stamp is a no-op — byte-identity holds. The value only moves where the decision already diverges from the row, which is exactly the pre-existing defect. That divergence correction is a **named** behavior change with its own test and its own CONVENTIONS bullet, never folded into the pin's own claims.
- Helpers share the coordinator's run row, so an unpinned multi-lane world can still meter a helper under the coordinator's lane. Out of scope; **name it** as a known limitation.
- Whether `runs.substrate` must move with `runs.lane` is **OQ-3**. Leaving them incoherent is not an option the executor may take silently.

### R10 — An unpinned task is byte-identical to today, proven both directions

The §66 **honest-absence property** is the model, and it is a property, not an example: over fixed-seed draws across families, coverage shapes and candidate sets, an unpinned `Decision` must equal the pre-packet decision **field for field** — including `PlainReason` byte-for-byte, `Cause`, `Lane`, `Model`, `Signals` and the new `LanePin` being empty. §66 D8's lesson binds: *"the honest-absence property compared too few fields"* — compare all of them, and hold out a mutation that fabricates a pin on an unpinned task to prove the comparison can fail.

The other direction: with a pin set, the decision **must** differ, at the lane and in the reason. A property that can only pass is not a property.

### R11 — S00.9 amendment A13, applied byte-exact and both copies proven

Apply the §10 text as **one table row on one line** in `Spec/drafts/S00-front-matter.md` immediately after A12. Annotate the marker sites (§10). Regenerate `Spec/core-architecture-v1.md` with the §1.11 concat and prove byte-identity to a fresh concat, recording the sha256 pair. Do not hand-edit the assembled file. The standing test that reproduces this check (§64) must stay green. Expected: **no ⚙ default or clamp moves → no S18 re-sweep; tally stays 118 keys / 33 domains; data-surface count stays 3** — asserted against the spec text, never assumed (§64/§65 precedent).

### R12 — Process constraints

`$0` — no live provider call on any code or test path; no credential material committed. Batteries serial (`go test -p 1`), one package at a time, one world at a time. `Docs/` and `Research/` are read-only. Adopt-don't-fork; `components.lock` untouched; no new dependency. Every ⚙ through the registry — and this packet expects **none**.

---

## 3. Seams to respect, and stubs for phases not yet come

| Seam | Posture in this packet |
|---|---|
| **`intake` ⇸ `worker`** (`route.go:12`) | Absolute. The pinnable set crosses as **data** through a seam value, never an import. The LN-6 `PlanWindows` seam is the shape to copy. |
| **`internal/api` holds no domain rule** (§11) | The refusal verdict is **computed once and carried**, never re-derived at the boundary (§66 D1, §67 OQ-2). |
| **S12 local tier (class (a))** | Stays absent. The pin does not commission a local provider entry and must not pretend one exists (`routing.go:640-665`). |
| **S11.5 credential-injection proxy** | Untouched and still deferred. A pin changes *which* lane is selected, never how a secret travels (§65). |
| **LN-10 (frontend)** | Owns the picker, the `web/src` TS types, and the door walk's head-to-head recipe. This packet ships the wire member and names LN-10 as its consumer — the `ProposePlanBudget` / §66 *"build the honest half, name the consumer, do not invent the surface"* precedent. |
| **Per-person coverage (B6/v1)** | Not built here. See OQ-2. |
| **Card-side lane override** | `intake.RouteOverride` gains **no** lane axis in this packet (see OQ-1). |

---

## 4. ⚙ settings consumed, by registry name

**NONE new, and none consumed by the pin itself.** The pin is a per-task operator choice, not a tunable number; the pinnable set is derived from what an operator has actually placed. If the executor believes a ⚙ is needed, that is an **OQ to the coordinator, never a minted key** — a new ⚙ requires an S18 sweep and an amendment of its own (§65/§67 posture).

Structural constants introduced (if any) get a reason in a doc comment beside them and go on the settings-tab flag list, per the §61/§62/§67 pattern.

---

## 5. Files expected to change

**Backend**
- `internal/stage/surface.go` — `submitBody.PinnedLane`; pass into `intake.Request`; `mapIntakeErr` rows for the new refusal class
- `internal/intake/intake.go` — `Request.PinnedLane`; the `ErrLanePinRefused` sentinel family beside `ErrPinRefused`
- `internal/intake/pipeline.go` — refuse before the birth transaction
- `internal/intake/route.go` — `RouteQuery.PinnedLane`, `RouteBlock.LanePin`, `routeQueryFor` reads `st.Req`
- `internal/stage/router.go` — thread the pin into `worker.RouteQuery`; carry `LanePin` in `blockFromDecision`/`decisionFromBlock`
- `internal/stage/helpers.go` — `routeHelper` reads the task's pin (R7)
- `internal/stage/skeleton.go` — compose the pinnable-lane seam beside `Coverage`; the R9 stamp
- `internal/stage/stage.go` — the seam's config field, documented
- `internal/worker/routing.go` — `RouteQuery.PinnedLane`, `Decision.LanePin`, the `resolveSeat` arm, the reason sentence, the refusal predicate
- `internal/run/run.go` — the lane-stamp verb (R9)

**Spec** (the only `Spec/` edits)
- `Spec/drafts/S00-front-matter.md` — the A13 row
- `Spec/drafts/S08-workers-composition-routing.md` — marker annotation
- `Spec/drafts/S15-frontend-api.md` — marker annotation
- `Spec/core-architecture-v1.md` — **regenerated**, never hand-edited

**Tests / fixtures / record**
- `internal/stage/lanepin_ln9_test.go` — **already committed RED by grounding** (§9)
- new `*_ln9_test.go` in `internal/worker`, `internal/intake`, `internal/api`, and `internal/shell` (the composed-world call-site proof)
- `web/src/fixtures/api/*.json` — regenerated via the sanctioned path only
- `P3/CONVENTIONS.md` — **§68** (next free; §67 is the highest today)

**Not touched:** `web/src` application source · `P3/gates/*` · `Docs/` · `Research/` · `internal/benchmark` · `components.lock` · `lanedata/*.json` · `plandata/*.json`

---

## 6. Adopted components touched

**None.** No engine, no dependency, no adopted code. `components.lock` is untouched and the lockgate count stays **39**.

---

## 7. Acceptance checklist — the evaluation agent's rubric

Each line is independently checkable. "Proven" means a test that fails when the behavior is reverted.

**The pin works**
- [ ] `POST /api/intake/requests` accepts `pinned_lane` as an optional member; a body without it is unchanged
- [ ] A pinned task's selection resolves to the pinned lane, **proven at the real call site** (`internal/stage/router.go:36`), never at `resolveSeat` in isolation
- [ ] A pinned task's **helper** spawn resolves to the same lane, proven at `internal/stage/helpers.go:392`
- [ ] `chooseFlatLane` is not consulted when a pin binds
- [ ] Every guard above is **mutation-verified**: revert the fix, the guard fails

**The reason is honest**
- [ ] `plain_reason` names the pinned lane and says the pressure comparison was replaced
- [ ] The pin reaches the approval card, `routing.decided`, `routing_quality` and the Workforce row (all via `plain_reason` + `lane_pin`)
- [ ] `Decision.LanePin` / `RouteBlock.LanePin` are `omitempty` and empty for unpinned tasks
- [ ] `RouteBlock.Pinned` and `OverriddenBy` keep **exactly** today's worker-axis meaning; a lane pin does not set them

**The refusal is loud**
- [ ] The two committed red tests (§9) are **green** and still fail under mutation
- [ ] A pin to an uncommissioned/uncovered lane refuses 4xx, names the pinnable lanes, and **no task row is born**
- [ ] A pin to `local` refuses in its own words, not the 2.7 subscription-gap wording
- [ ] The refusal verdict is computed **once** and carried across the seam; a second spelling is a failure
- [ ] `resolveSeat` refuses a planted pin arriving by any other route

**Nothing else moved**
- [ ] Unpinned decisions are byte-identical field-for-field, proven as a **property** over fixed-seed draws, with a held-out mutation that fabricates a pin
- [ ] A pin is proven to CHANGE the decision (the property can fail in both directions)
- [ ] The benchmark direct arm's lane is unchanged under a pinned task
- [ ] D5 scans green: no money-shaped member in selection inputs; `routing.go` names no price/cost/metering symbol
- [ ] The `kimi`/`kimi-cli` pooled reading still aggregates both lanes under a pin; `PlanPoolRefusal` untouched

**Receipts tell the truth (R9)**
- [ ] `runs.lane` carries the decided lane for an execute run; the receipt Lane column shows the lane that ran
- [ ] In a world with nothing commissioned the stamp is a **no-op** and goldens for that world are unchanged
- [ ] The divergence correction is named in §68 with its own test, not folded into the pin's claims

**Amendment**
- [ ] A13 applied byte-exact as one row on one line after A12, with the operator ruling as recorded
- [ ] Marker sites annotated (S08.8, S15.2, `routing.go` head comment)
- [ ] `core-architecture-v1.md` regenerated by the sanctioned concat; byte-identity proven with the sha256 pair recorded
- [ ] ⚙ tally asserted **118 keys / 33 domains**, data surfaces **3**, no S18 re-sweep

**Process**
- [ ] `$0` — no live provider call anywhere; no credential material committed
- [ ] Go battery serial (`-p 1`), gofmt/vet clean, all packages green
- [ ] Fixtures regenerated **only** via `SINET_WRITE_API_FIXTURES=1`; every moved pre-existing member named explicitly
- [ ] **vitest run and green at landing** (fixtures moved ⇒ the frontend battery is owed — gate item 12)
- [ ] `P3/CONVENTIONS.md` §68 written; migrations still `0001–0025`; lockgate still 39

---

## 8. CONVENTIONS constraints that bind

| § | Constraint as it applies here |
|---|---|
| **§63 R1** | *"A correct function reached through a dropped argument is a broken feature."* Call-site tests only. The two `Route` sites and the two-of-eight `SessionInput` sites are the recorded precedent for how this fails. |
| **§63 D2** | The wired-and-read-by-nothing class. A pin member nothing consumes, or a seam nothing calls, is the defect this section exists to close. |
| **§63 R3** | `web/src/fixtures/api/` is a backend-owned contract snapshot; a member no fixture exercises is a contract nobody agreed to. |
| **§65 D4** | One predicate, one spelling. The pinnable/refusal rule may not exist twice. |
| **§65 D5** | A nil-fallback that re-creates the hazard the fix removed must be guarded, and the guard mutation-verified. |
| **§66 D1 / §67 OQ-2** | One predicate carried across layers; the boundary never re-derives a domain verdict. |
| **§66 (honest absence)** | The unpinned world is pinned as a **property**, and §66 D8 says compare *all* the fields. |
| **§64 C3 / §66** | A fixture claim that overstates its own innocence is the kind a reviewer stops checking — name every moved member. |
| **§30** | An unhonorable pin is a bad **request** (4xx), never a 500; never-percent scans cover every new shape, walked populated. |
| **§12** | Fail-closed: the only default is consent. An unhonorable pin refuses; it never auto-approves itself onto another lane. |
| **§19** | Absent duties degrade with a recorded reason, never faked — and a *pin* is the case where degrading is forbidden and refusing is required. |
| **§10** | Any sanctioned skip uses the exact `SANCTIONED SKIP (CONVENTIONS §10)` form, counted rather than spot-read. |
| **§6** | Landed migrations are immutable. This packet expects **no** migration at all. |

---

## 9. Acceptance-test specifications

### 9.0 Committed RED by grounding — `internal/stage/lanepin_ln9_test.go`

Two tests are **already committed and already failing**, and they fail for exactly one reason: the task-creation verb accepts an unknown `pinned_lane` member and silently drops it, so a pin the platform cannot honor is answered 200 with a task born.

They compile against today's surface because `Submit` takes a `json.RawMessage` — no identifier from the unbuilt feature appears in them. (§65 D10 records what the other choice costs: a tests-first commit that does not build makes a bisect land on a tree that cannot answer.)

Verified this session:

```
--- FAIL: TestLN9LanePinToAnUnpinnableLaneRefusesAtTheBoundary/kimi-cli
    Submit pinned to "kimi-cli" succeeded — a pin the platform cannot honor must
    refuse loudly at the boundary (brief R3), never fall back to routing's own choice
--- FAIL: TestLN9LanePinToLocalRefusesWithItsOwnReason
    Submit pinned to the local tier succeeded — the local engine lane carries no v0
    consumer, so honoring the pin could only mean riding the paid seat (brief R8(b))
```

`gofmt` and `go vet ./internal/stage/` are clean. These two must be **green at landing**, and the executor may tighten their status/code assertions once the refusal class exists, but may not weaken them (a weakened assertion nobody announced is indistinguishable from a passing one — §67 F14/F15).

### 9.1 `TestLN9PinnedLaneWinsAtTheRealCallSite` (`internal/shell` or `internal/stage`)

*Setup:* a composed world (the `internal/shell/planbudget_ln6_surface_test.go` shape — production fill, real router, real coverage, real seats) with a second lane commissioned so at least two lanes are covered. Submit two identical tasks: one unpinned, one pinned to the commissioned lane.
*Assert:* the pinned task's recorded `RouteBlock.Lane` is the pinned lane; the unpinned task's is whatever pressure/order chose; `plain_reason` on the pinned one names the lane and says the pressure comparison was replaced; `LanePin` is set on one and empty on the other.
*Mutation:* delete the pin arm in `resolveSeat` → the test must fail. Delete the pin from the `RouteQuery` construction at `router.go:36` → the test must **also** fail (this is the dropped-argument guard; a resolver-only test cannot see it).

### 9.2 `TestLN9PinBindsTheHelperSpawn` (`internal/stage`)

*Setup:* a pinned task driven to a helper spawn, against recording adapters.
*Assert:* the helper's decision carries the same lane, and the engine actually reached is the pinned lane's substrate (`laneSubstrates`). The §63-R2/§65 recording-adapter shape — assert **which engine was reached**, not which lane was computed.
*Control:* an unpinned task takes the unchanged path.
*Mutation:* drop the pin read in `routeHelper` → fail.

### 9.3 `TestLN9UnpinnedIsByteIdentical` (`internal/worker`) — a PROPERTY

*Setup:* ≥200 fixed-seed draws over (family × domain × candidate set × coverage shape × research/writes/mechanical flags), no pin.
*Assert:* every `Decision` field equals the pre-packet value — `Cause`, `Score`, `Signals`, worker identity, `Model`, `Lane`, `Effort`, `WindowTokens`, `PlainReason` **byte-for-byte**, `Degraded`, `Candidates`, gap fields, and `LanePin == ""`.
*Held-out mutation (required):* fabricate a pin on an unpinned query → the property must go red. §66 D8 is the record of this property passing while comparing too few fields.

### 9.4 `TestLN9PinRefusesAndNothingIsBorn` (`internal/stage`)

The green successor of §9.0, extended: exact status + code per refusal class; the message enumerates the pinnable lanes; the world with the lane commissioned **accepts** the same body (the inverse control — a refusal that refuses everything is not a fix).

### 9.5 `TestLN9PlantedPinCannotSteerDispatch` (`internal/worker`)

*Setup:* a `RouteQuery` carrying a pin to an uncovered lane, handed straight to `Route` (bypassing the boundary).
*Assert:* selection refuses/routes nowhere with a reason naming the pin; it does **not** silently pick another lane. This is layer 3 of R3 and mirrors `ReadPlanUnits` refusing a planted row (§67 F3).

### 9.6 `TestLN9PinSurvivesReplanAndResume` (`internal/stage`)

*Assert:* after a re-plan recompute the pin is still honored (it is re-read from `st.Req`, not frozen); after a park/resume the recorded lane is unchanged; `RouteBlock.Pinned` is **still false** on a lane-pinned task that carries no worker pin (the §1.2 trap, pinned as a test).

### 9.7 `TestLN9ReceiptLaneIsTheLaneThatRan` (`internal/metering` or `internal/api`)

*Assert:* a run whose decision named lane L has `runs.lane == L` and a receipt whose Lane column shows L.
*Control (byte-identity):* in a world with nothing commissioned the decided lane equals `cfg.Lane` and nothing moves.
*Regression pin:* the pre-packet behavior (decision `zai`, receipt `anthropic`) must be what fails.

### 9.8 `TestLN9PooledPressureUnchangedUnderAPin` (`internal/metering`)

*Assert:* pinning between `kimi` and `kimi-cli` does not change the pooled reading; the pool still aggregates both lanes; `PlanPoolRefusal` still refuses a sibling-lane budget. The two lanes' signal tables stay pinned equal (§67).

### 9.9 `TestLN9BenchmarkDirectArmIgnoresThePin` (`internal/benchmark`)

*Assert:* a pinned task's direct arm is still born on `claude-cli`/`anthropic` (`pair.go:36-37`). The arm's identity is its lane.

### 9.10 Scans and tallies

D5 reflective scan (no money-shaped member in selection inputs, following maps/slices/pointers — §63 D10); the `routing.go` source scan; the never-percent scan over every new shape walked **populated** (§66 D3); the ⚙ tally asserted at 118/33 against the spec text; the spec-assembly byte-identity test.

---

## 10. S00.9 amendment A13 — full draft text

**Placement:** `Spec/drafts/S00-front-matter.md`, the Post-G4 changelog table, **immediately after A12**, as **one table row on one line** (A11/A12's exact form).

**The `<OPERATOR RULING>` placeholder is filled from the STATE log's recorded ruling** (`P3/STATE.md`, 2026-08-26): *"OPERATOR RULED (in-session form): BUILD THE LANE PIN NOW (LN-8 head-to-head decision, option A recommended-and-taken)."* Apply the attribution in the A12 phrasing — **coordinator-drafted option text, operator-selected, in-session** — and do not paraphrase it.

> `| A13 | 2026-08-26 | **S08.8's "Visible and overridable" gains a second expression point and names its axis: a task may carry an operator-declared PINNED LANE, declared at task CREATION ahead of the first selection rather than only as a re-route on the plan/approval card, which selection honors in place of the step-3 consumption-pressure comparison among covered flat-rate lanes — on explicit operator order (2026-08-26).** S08.8 already ratifies the override itself ("the requester can re-route or pin before execution; overrides are recorded with their actor"), and the shipped machinery already implements that sentence for the WORKER axis alone: `intake.RouteOverride{Target, Pin}` selects a template id or the generalist, and the lane is re-derived from the chosen worker's seat on every recompute. What this entry amends is therefore narrow and is stated rather than blurred: **(a)** the pinnable AXIS is named as the LANE, and **(b)** the pin may be declared at task creation — the S15.2 `tasks` family's create verb, which at v0 is `POST /api/intake/requests` — because a pin expressible only on the card cannot precede the first selection, and per-task-at-creation is the ordered use. **Why now, and it is a capability the platform provably lacked:** P3-LN-8 established that the operator's ordered comparison of the two Kimi paths (lane `kimi` on the opencode substrate [A11]; lane `kimi-cli` on Moonshot's own CLI [A12]) is NOT RUNNABLE by any existing mechanism — the two lanes draw ONE membership quota pool (`kimi-code-membership`, declared once, never as two allowances), so their consumption-pressure ratios are identical BY CONSTRUCTION; `chooseFlatLane` takes the strictly less-consumed lane and keeps the earlier candidate on a tie, and the candidate order puts `kimi` first; one placed credential commissions both, so neither can be held alone; and no surface anywhere takes a lane from a person. **Operator ruling, 2026-08-26: BUILD THE LANE PIN NOW** (option A, recommended-and-taken) — coordinator-drafted option text, operator-selected, in-session. **What does NOT change, each load-bearing:** S08.8 step 3's "subscription coverage binds every choice" is UNCHANGED and OUTRANKS the pin — a pin can never select a lane the owner does not hold flat-rate; it is refused at the system boundary before a task is born, the refusal verdict is computed once and carried rather than re-derived, and selection re-checks it so a pin arriving by any other route cannot steer dispatch. **D5 is untouched**: a pin is a person's named choice and carries no money — it REPLACES the pressure comparison rather than adding a currency to it, and no price, cost or dollar figure enters selection's inputs, which remains a structural property rather than a discipline. The local tier is NOT pinnable at v0 — its engine lane has no commissioned provider entry (S12.1 class (a)) — and refuses in its own words rather than borrowing the 2.7 subscription-gap wording. The benchmark direct arm is deliberately exempt: its lane is a structural constant naming the requester's frontier surface and the arm's identity IS its lane. The metered-exception list stays **EMPTY** [G1 P7]. **No ⚙ setting's default or clamp is touched** — the pinnable set is derived from what an operator has actually placed and has no dotted key → **no S18 re-sweep**; the S18 tally stays **118 keys / 33 domains** and the data-surface count stays **3**. Marker sites annotated: S08.8's "Visible and overridable" paragraph, the S15.2 `tasks` family row, and the `internal/worker/routing.go` head comment where the selection pipeline's inputs are enumerated. | operator, 2026-08-26 ruling; presented for veto at the LN gate batch (item 15) |`

**Marker annotations** (bold inline `[S00.9 A13]` references, the A11/A12 form — see `S03-engines-adapters.md:41,106,139` for how those read):

1. **S08.8, the "Visible and overridable" paragraph** — after *"the requester can re-route or pin before execution; overrides are recorded with their actor"*, add a sentence naming the lane axis and the creation-time expression point, tagged `[S00.9 A13]`, and restating that coverage still binds and an unhonorable pin refuses.
2. **S15.2, the `tasks` family row** — the mutating-verbs cell notes that create carries the optional lane pin, tagged `[S00.9 A13]`.
3. **`internal/worker/routing.go` head comment** — the pipeline enumeration gains the pin as a sanctioned step-3 override input, citing A13 (the `adapters.go` substrate-const comment is A12's precedent for a code-side marker).

Then regenerate and prove byte-identity (§1.11 / R11).

---

## 11. Open questions

Each carries a recommendation. The executor implements the recommendation unless the coordinator rules otherwise; a genuine reversal touches one function and one test.

**OQ-1 — Does the approval card's `RouteOverride` gain a lane axis in this packet?**
S08.8's literal sentence puts the override on the card, and `RouteOverride` (`route.go:93-103`) has no lane field. But the operator's ordered use is per-task-at-creation, and LN-10 builds the picker on the *task-creation* surface.
**Recommendation: NO — creation-time only in LN-9.** Ship the honest half and name the consumer (§66 precedent). Note in §68 that the card's lane axis does not exist, so nobody re-derives it as missing. Also worth knowing: `web/src` has **no re-route/pin control at all** today — `intake.Answer.Route` exists in Go with no web caller — so the card axis would ship unreachable in both directions.

**OQ-2 — Per-person scoping of the pinnable set.**
`Coverage.FlatRateLanes` is the union across people (§1.7); the per-person commissioned map exists but is keyed by the broker `who`, whose relationship to `auth.User.ID` is unsettled and is already **LN gate-batch item 8**.
**Recommendation: validate against the process pinnable set (the union), state the over-approximation in the seam's own doc comment, and name it as riding gate item 8.** At v0 the platform is operated single-user, so union == the operator's set. Inventing a namespace mapping here would settle a household question the spec has not settled — the §65 posture exactly.

**OQ-3 — Does `runs.substrate` move with `runs.lane` (R9)?**
`substrate` is stamped at creation from the same process config and is equally stale, and `substrateFor` already resolves the real one at session build.
**Recommendation: stamp both, so the row is coherent** — but the executor must first enumerate `runs.substrate`'s readers (recovery, dispatch, conformance) and report if any depends on the creation-time value. Leaving the two incoherent is not an option; if stamping `substrate` breaks a reader, **stop and report** rather than working around it.

**OQ-4 — Precedence between a task lane pin and a template model pin.**
`ExecutionProfile.ModelPin` (`routing.go:679-688`) pins a model and skips `chooseFlatLane`. If a template pins model M and the task pins lane L, both cannot be honored when M does not live on L. The spec does not settle this by name.
**Recommendation: the task pin WINS**, the seat becomes the pinned lane's seat, and the reason states the supersession quoting the template's own `ModelPinReason`. Grounds: S08.8 records overrides *with their actor* — a person's act on this task outranks a standing template default whose recorded reason is preserved in the reason string, and S08.4's 8.9 precedence puts task spec above template baseline. The alternative (refuse the pin) is defensible and cheap to swap: one branch, one test.

**OQ-5 — Refusal code granularity.**
One code for every unhonorable pin (detail distinguishes the cases), or distinct codes for *not-commissioned* vs *local-not-dispatchable*?
**Recommendation: one code, distinct detail sentences** — matching `bad_request` + a naming message (`meters_verbs.go:407-418`) rather than `project_not_active`'s per-case code. Lane names are not secret (they ship in lane documents and LN-10's picker enumerates them), so unlike the `Project` refusal there is **no existence-oracle concern** and the message may name the pinnable lanes freely.

---

*Grounding agent, 2026-08-26. Truth in this brief is code + spec as read this session; where a commissioning assumption was falsified (§1.3, §1.6) the falsification is the finding, not a detail.*
