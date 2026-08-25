> **EXPIRED at landing (2026-08-25). Single-use artifact: after drains r1/r2 this brief no longer matches the code (T3 replaced by the coherence rule; D6 expiry semantics; the coherent-triple wire members). Later grounding must never read it as truth — code + spec only.**

# P3-LN-6 — the plan-budget declaration path

**The ratified LN-2B lever made real, so a commissioned lane can genuinely win routing selection.**

Refs: S10.1–S10.5, S10.7, S08.8, S18.3, S15.9 · CONVENTIONS §11, §30, §63 (D1/D2/D3, r2 R1/R3), §65 · feature 13.4.

This brief is self-contained. The executor and the evaluator work from it plus the sections it cites. Every prior brief is EXPIRED; the code and the spec are the only truth.

---

## 0. The gap, stated exactly

`internal/shell/shell.go:1583–1593` — `routePressure.Pressure`, the production pressure reader that `worker.Router` consumes:

```go
if doc, ok := metering.PlanDocFor(lane); ok {
    // No plan-budget surface exists yet, so this is honestly undeclared;
    // the 13.4 surface is where an operator will declare one from the
    // document's own proposal (metering.ProposePlanBudget).
    r, err := p.g.ReadPlanUnits(ctx, owner, lane, doc, metering.UndeclaredPlanBudget(), time.Now())
```

`metering.UndeclaredPlanBudget()` (`internal/metering/planunits.go:240`) returns the zero `PlanBudget`, so `ReadPlanUnits` never reaches its `budget.Declared && budget.PeriodUnits > 0` leg (`planunits.go:566`) and reports `Applicable=false` with `Pressure=0`. `chooseFlatLane` (`internal/worker/routing.go:738`) then hits its `!p.Applicable` branch and returns the deterministic duty-map order with the stated reason. **A commissioned GLM/Kimi lane can never win the execution seat through normal intake, whatever an operator declares.** The same hardcode sits on the read path at `shell.go:1725–1729` (`projMeter.LaneMeter`), so `GET /api/meters` reports the same absence.

`metering.PressureGauge.ProposePlanBudget` (`planunits.go:477–513`) already implements the ratified proposal — the provider allowance at a conservative ⚙ fraction — and **nothing in the tree calls it**. That is the wired-and-read-by-nothing class §63 D2 exists to close.

This packet builds the BACKEND half: a per-person per-lane-per-window PLAN-budget row store, a declaration verb, and `routePressure` + `projMeter` consuming declared rows. Backend ONLY.

### 0.1 The 13.4 precedent, quoted

`internal/adapters/opencode/lane.go:249–257`, in the `TBD-S10.2(currency-flip receipts)` note:

> "…the currency-changing receipt path belongs to whichever packet lands S10.2's flip mechanics (**the `ProposePlanBudget`/13.4 precedent: build the honest half, name the consumer, do not invent the surface**). The flip itself is an operator act through the rehearsed kill-switch, never automatic and never silent (R02 §6)."

### 0.2 What "13.4" is, and why this does not contradict it

Feature 13.4 (`Docs/agent-platform-feature-list-v1.md:277`): *"A settings tab includes every single setting, well organized, for the administrator to change — including the flat-rate/metered flags, the API-equivalent price table, **automation budgets**, freshness thresholds, depth cap, retention period, and missed-slot defaults."*

Its architecture is **S15.9** (Settings UI: JSON Forms over the registry-emitted schema, "**Every setting** is present and editable here (13.4) … automation budgets …") plus **S01.10** (registry architecture) plus **S18.3** (data-valued surfaces that carry no dotted key). 13.4 is therefore a **frontend surface over a backend the backend must first have**. The precedent is already landed and in the tree: the *token* budget's store (migration `0017_budgets_pause_hint.sql`), its verb (`POST /api/meters/budget`, `internal/api/meters_verbs.go:118`) and its route registration (`internal/api/api.go:586`) all shipped in B6-2B with **no 13.4 UI**. This packet is the identical move for the plan-unit sibling. **No contradiction; the 13.4 settings UI stays future and is a non-goal here.**

---

## 1. Requirements

Each requirement carries its ref. "MUST" is binding; a deviation is a finding, not a choice.

### R1 — The row grain is (person, lane, **window**), because the spec ratified three components and the data needs all three

**S18.3** (`Spec/drafts/S18-settings-registry.md:217–226`, mirrored in `Spec/core-architecture-v1.md`), the *Data-valued settings surfaces (13.4-editable, no dotted key)* table:

| Surface | Shape | Owner | Ratified by |
|---|---|---|---|
| **Per-person automation budgets** | **per (person, lane, period)**; seeded from plan marketing shape, labeled "assumed" | S10.4 | G2 Def.5 |

The ratified grain is **(person, lane, period)**. The landed token table narrowed it to `PRIMARY KEY (user_id, lane)` (0017), which was adequate because a token gauge has one period. **The plan data makes the third component load-bearing** and the document proves it, not this brief:

- `internal/metering/plandata/kimi.json` declares **two** quota windows with **different units**: `rolling-5h` in `requests` (300 units, 5 h) and `weekly` in `credits` (168 h, `allowance_unverified: true`).
- `internal/metering/plandata/zai.json` declares two windows, both in `credits`: `rolling-5h` (28 000, 5 h) and `weekly` (140 000, 168 h).
- `PlanQuota.Unit`'s own doc comment (`planunits.go:112–120`): *"A single lane-wide scalar cannot describe that, and rendering both gauges in one unit would be a quiet lie about what was counted."*

**MUST:** the plan-budget row is keyed `(user_id, lane, window)` where `window` is the plan document's own quota **name** — the exact string `PlanBudget.SeededFrom` already carries (`planunits.go:233`) and that `ReadPlanUnits` already uses to pick the reading's unit (`planunits.go:544–547`). No new vocabulary is minted: the names come from the documents (`rolling-5h`, `weekly`).

**MUST:** the row's stored unit is `doc.QuotaUnit(window)` — the window's own unit, the document's unit only as the declared fallback (`planunits.go:293–298`). Never a lane-wide scalar, never tokens, never dollars.

**MUST NOT** widen, re-key or migrate the existing `budgets` table. It is a committed migration and immutable (CONVENTIONS §6, restated in the 0017 header).

### R2 — A SIBLING store, not a reuse of `BudgetRow` (units and semantics differ)

The brief's instruction is to decide reuse-vs-sibling per the spec's data-shape rules and **not to force reuse**. The decision is **SIBLING**, on four grounded differences:

1. **Unit.** `BudgetRow.PeriodTokens int64` is weighted-consumption units (`budgets.go:32`; 0017 DDL: *"The unit is the GAUGE'S OWN unit — weighted-consumption units… It is NEVER dollars"*). A plan budget is in the **plan's own unit per window** and is a `float64` (`PlanBudget.PeriodUnits`, `planunits.go:222`) because a proposal is a fraction of an allowance.
2. **Period shape.** `budgets.period_days INTEGER CHECK (period_days > 0)` cannot express the seedable window: `rolling-5h` is **5 hours**. `PlanBudget.PeriodHours float64` (`planunits.go:230`) already exists for exactly that.
3. **Key.** (user_id, lane) vs (user_id, lane, window) — R1.
4. **The package's own ratified rule.** `planunits.go:1–21` head comment: the two readings *"are therefore two readings, each stamped with its own tier"*; §63 bullet 1: *"They are therefore two types, not two fields … and a test pins both on the SAME run and fails if the two numbers are ever equal."* One table with a nullable unit column is how somebody later adds a credit to a token.

**MUST:** new migration (next free number in `internal/storage/migrations/`, currently `0025_*`) creating table `plan_budgets`, following the 0017 DDL conventions verbatim in kind: `NOT NULL` + `CHECK` on every column, RFC3339Nano UTC timestamps stored as TEXT, a commented reason per column, `PRIMARY KEY (user_id, lane, window)`.

**MUST:** new store type in `internal/metering/planbudgets.go` mirroring `Budgets` (`budgets.go:54–171`) method-for-method:

- `NewPlanBudgets(db *storage.DB) *PlanBudgets`
- `Declare(ctx, PlanBudgetRow) (prior PlanBudgetRow, existed bool, err error)` — upsert, returns the row it REPLACED so the audit is old→new (S14.2 family 5), refuses an empty key / non-positive units / empty declaring actor / non-positive period hours as **defects**, and refuses a person who does not exist with the shared `ErrNoSuchPerson` (`budgets.go:52`, same reason: *"a budget declared for nobody is a decision recorded about nobody"*).
- `Row(ctx, userID, lane, window) (PlanBudgetRow, bool, error)` — the bool is the honest absence, never an error.
- `Rows(ctx, userID, lane) ([]PlanBudgetRow, error)` — every declared window for the lane (R6 needs it).
- `PlanBudget(ctx, userID, lane, window) (PlanBudget, error)` — the value object, falling back to `UndeclaredPlanBudget()` when absent. **This MUST be the ONE place the undeclared plan posture is produced from storage**, exactly as `Budgets.Budget` documents for tokens (`budgets.go:135–148`): *"so a caller can never accidentally invent a denominator (D4)."*
- `PlanBudgetRow.Budget() PlanBudget` — the conversion, carrying `SeededFrom`/`Fraction` through as provenance.

### R3 — Provenance is stored, per the existing budget-row precedent

**MUST:** every row carries `declared_ts` + `declared_by` (the 0017 audit-face precedent: *"these two make the row itself say who last set it and when, without a join"*), **plus** the plan-specific provenance the type already models:

- `seeded_from` — the quota row a proposal came from (`PlanBudget.SeededFrom`, `planunits.go:233`), empty for a hand-set row.
- `fraction` — the share of the allowance taken (`PlanBudget.Fraction`), 0 for a hand-set row.
- `source` — a closed two-member vocabulary `{proposal-seeded, operator-set}`, `CHECK`-constrained in the DDL. A value outside the set is refused at write, not stored and rendered.

`SeededFrom`/`Fraction` are **provenance, never inputs to the arithmetic** — `planunits.go:234–236` says so verbatim: *"never an input to the arithmetic."* No requirement may make the reading depend on them beyond the unit lookup at `planunits.go:544–547`.

The **"assumed"** label is NOT re-derived here: it rides the reading from the document's own `AssumedNote` (`planunits.go:553`, validated non-empty at `planunits.go:350`) and stays exactly as it is. A declared budget does not make a tier-3 reading measured. **MUST:** `PlanReading.Tier` stays `TierDerived` and `Assumed` stays `true` on every path this packet touches.

### R4 — The ⚙ is consumed by REGISTRY NAME; no key is minted

**MUST:** the proposal fraction is read as ⚙ **`budget.background_window_fraction`** — the existing registry key. Evidence it exists and is ratified:

- `Spec/drafts/S18-settings-registry.md:138`: `| budget.background_window_fraction | 0.5 | 0 – 1; labeled "assumed" | | | G2 Def.4/5 |`
- `internal/settings/index.go:538`: `Key: "budget.background_window_fraction", Section: "S10", Type: TypeFloat, Unit: "ratio"`
- `internal/metering/pressure.go:16`: `keyBgWindowFraction = "budget.background_window_fraction"`
- S10.4 bullet 2: *"Background budgets default to `⚙ budget.background_window_fraction = 0.5` (≤50% of the advertised window, "assumed") [G2 Def.4/5]."*

LN-2B minted **no** new key: `ProposePlanBudget` already reads this one (`planunits.go:501`). **MUST NOT** mint a new key, and **MUST NOT** introduce a Go constant for the fraction — S18 amendment territory, never a constant.

**MUST:** the S18 tallies stay **118 keys / 33 domains** (asserted at `internal/settings/settings_test.go:44`, `:450`, `internal/settings/read_test.go:32`) and the **S18.3 data-surface count stays 3** (§65 precedent). A plan-budget table is **not a fourth surface**: it IS the ratified *"Per-person automation budgets — per (person, lane, period)"* surface, at the grain the spec already ratified. **MUST** record that argument in the CONVENTIONS entry rather than leave the count to be re-derived.

> **⚙ OQ-1 — FLAGGED, NOT RESOLVED. Coordinator decision required before this packet lands.**
> `budget.background_window_fraction` is doing **double duty** in the tree today:
> **(a)** the fraction of the **provider's advertised allowance** taken to propose a budget (`planunits.go:501`) — which is what S10.4's sentence describes ("≤50% of the advertised window"); and
> **(b)** the fraction of the **already-declared budget** that is background work's own ceiling (`planunits.go:569` `BackgroundCeiling`; `pressure.go:96` `BackgroundBudget`; asserted at `budgets_test.go:154`).
> Composed, a proposal-seeded budget gives background work **0.25 of the advertised window** (0.5 × 0.5). The spec sentence names only (a). This packet **consumes the key as it stands** and changes neither call site; the executor MUST NOT "fix" this. Whether (b) needs its own S18 row is an S00.9/S18-sweep question for the coordinator.

### R5 — The declaration verb, sibling to `POST /api/meters/budget`, behind the §11 wall

**MUST:** `POST /api/meters/plan-budget`, registered beside its sibling in `internal/api/api.go` (`protected("POST /api/meters/budget", …)` at `:586`) as `protected(...)`.

It **MUST** follow `handleBudgetDeclare` (`meters_verbs.go:118–206`) point for point:

1. `s.projReady(w)` guard, then a nil-seam guard answering **503 `not_wired`** with a message naming the store (`meters_verbs.go:122–126`).
2. `s.readBody` + `json.Unmarshal` → **400 `bad_body`**.
3. Authority: **own + operator-any** (the recorded OQ4 posture, `meters_verbs.go:137–148`). `person` defaults to the caller; another person's row is the operator's, with a 403 whose message names S15.2/D10. Then `s.requirePerson`.
4. Field validation with messages that say what is wrong and why, in the house voice: missing `lane`; missing `window`; non-positive `period_units`; non-positive `period_hours`.
5. Audit: `s.recordDecision` with a **new** card type `cardTypePlanBudget = "plan_budget"` beside `cardTypeBudget` (`meters_verbs.go:81–85`), carrying old→new via a `planBudgetSnapshot` mirroring `budgetSnapshot` (`meters_verbs.go:211–228`) — an absent prior renders as the explicit `{"declared":false}` object, never a missing member. The audit row lands **AFTER** the store act (the part-A drain-D6 principle stated at `meters_verbs.go:186–188`).

**MUST:** the §11 wall holds — `internal/api` **never** imports `internal/metering` (`shell.go:1609–1616`: *"the same wall projMeter keeps below, for the same reason (§11)"*). The transport speaks its own record types (`PlanBudgetRecord`, `PlanWindowRecord`) and the shell adapts.

**MUST:** the seam is a new interface in `meters_verbs.go` beside `BudgetStore` (`:51–60`):

```
PlanBudgetStore interface {
    DeclarePlanBudget(ctx, PlanBudgetRecord) (prior PlanBudgetRecord, existed bool, err error)
    PlanBudget(ctx, userID, lane, window string) (PlanBudgetRecord, bool, error)
    PlanWindows(ctx, lane string) ([]PlanWindowRecord, error)
    ProposePlanBudget(ctx, userID, lane, window string, start time.Time) (PlanBudgetRecord, error)
}
```

`PlanWindows` exists for a structural reason, not convenience: **the window name must be validated against the lane's own document, and `internal/api` cannot see the document.** CONVENTIONS §30 puts input validation at the boundary that admits a person's input (restated at `budgets.go:66–70`). So the verb reads the lane's declared windows through the seam and refuses an unknown window with a **400** naming the windows that exist. Without this the check would either move into the store (a defect check standing in for an input check) or surface a 500 for a typo.

**MUST:** `ProposePlanBudget` on the seam is the **production consumer** the gauge method has never had. The verb accepts `"from_proposal": true` with no `period_units`, asks the seam to propose, and stores the result with `source = proposal-seeded`. It **MUST** surface the existing refusal for an unverified allowance as a **400**, not a 500 — the kimi `weekly` row is `allowance_unverified` and `planunits.go:493–500` already refuses it with the right words (*"publishes no allowance at primary-source grade… the operator's own account console closes it (P-T17-3)"*). Proposing from a window nobody published an allowance for is the inferred provider window D4 bars.

**MUST NOT:** any dollar field on `PlanBudgetRecord`, on the body, or in the response. The `meters_verbs.go:14–27` head rule is binding here verbatim: *"NO MONEY IS COMPUTED IN `internal/api`."* No plan unit may be multiplied by a price anywhere in this packet (S10.3: *"Never price a flat-rate lane's usage from a $0 flat-rate row"*; money is READ, never computed — §11).

### R6 — `routePressure` consumes declared rows; the honest absence is unchanged

**MUST:** `routePressure` gains the plan-budget store (`shell.go:1569–1572` currently holds `g` + `b`) and its plan branch (`shell.go:1584–1593`) reads the declared rows instead of hardcoding `UndeclaredPlanBudget()`. The stale comment at `:1585–1587` is replaced, not left (the §63 D8 stale-comment finding).

**MUST:** with **no row for any window** on the lane, the branch passes `UndeclaredPlanBudget()` and the behaviour is **byte-identical to today**: `Applicable=false`, `Ratio=0`, `Unit` unchanged, and `chooseFlatLane` returns the deterministic duty-map order with its existing reason string (`routing.go`, the `!p.Applicable` branch: *"…has no declared automation budget, so there is no comparable consumption pressure and the deterministic duty-map order stands (S10.4; never dollars — D5)"*). **This is pinned, not broken** (R11's property test).

**MUST:** with rows declared, the reading is `ReadPlanUnits(…, row.Budget(), now)` and the returned `worker.LanePressure` carries `Ratio: r.Pressure`, `Applicable: r.Applicable`, `Unit: r.Unit` exactly as today — the unit being the **binding window's** unit, which `ReadPlanUnits` already derives from `budget.SeededFrom` (`planunits.go:544–547`). No new arithmetic is added to `metering`: the packet supplies the denominator, it does not re-implement the reading.

**Multi-window rule (see ⚙/OQ-2 below):** when more than one window carries a declared row, `routePressure` reads each and returns the **maximum applicable ratio** — the most-consumed window binds — and the note/unit name that window. Ground: S10.4's headroom rule stops background admission at `⚙ pressure.bg_admit_stop = 0.7`; a lane whose 5-hour window sits at 0.9 has no headroom regardless of its weekly, so a minimum or a mean would admit work into an exhausted window, and interactive use is CRITICAL_PLUS.

> **OQ-2 — FLAGGED, NOT RESOLVED. Coordinator ruling required.**
> **No spec section states an aggregation rule across a lane's windows by name.** S10.4 speaks of "the operator-declared automation budget" in the singular; the two-window shape is a fact of the plan documents, not of the spec text. The max-binds rule above is *derived* from S10.4's headroom sentence, which is a derivation and not a citation. Named alternatives the coordinator may prefer: **(i)** the narrowest window only (`rolling-5h`), the one the plans actually meter against in the short run; **(ii)** an operator-designated "routing window" per lane, one more data column. The single-window case — the load-bearing one for R10's selection proof, and the only one kimi can reach, since its `weekly` allowance is unverifiable — is unambiguous under all three. The executor implements max-binds **and** pins the single-window case separately, so a coordinator reversal touches one function and one test, never the store or the verb.

**MUST:** `projMeter.LaneMeter` (`shell.go:1701–1763`) makes the SAME change at `:1725–1729`. Leaving it undeclared would make `GET /api/meters` contradict the router — the exact drain-D2 self-contradiction the token path's own comment records (`shell.go:1666–1674`): *"a hardcoded undeclared read makes `GET /api/meters` CONTRADICT ITSELF."*

### R7 — Composition root wiring

**MUST:** `shell.go:605` composes `routePressure` with the new store; `shell.go:977` composes the API server with the new seam (the `budgetAdapter` precedent at `:1616–1644`). A `nil` store keeps the pre-migration posture exactly: verb 503, reading undeclared, routing deterministic (`meters_verbs.go:52`: *"nil leaves the budget verb answering 503"*).

**MUST:** a new `planBudgetAdapter` in `shell.go` beside `budgetAdapter`, converting and nothing else — *"the unit stays the gauge's … unit on both sides, and nothing here multiplies, prices or rescales anything"* (`shell.go:1614–1615`).

### R8 — The LN-4 tripwire is UPDATED as part of this packet

`internal/shell/lanepressure_r2_test.go` is the P3-LN-4 drain-r2/R2 evidence pin. Its head comment, verbatim:

> "The drain asked for a case where a commissioned lane WINS selection through the real pressure reading, with budget-seeded rows making the ratio favour it. That case is not reachable at v0, and this is the proof rather than the assertion: the production adapter routes a lane carrying a PLAN DOCUMENT to the tier-3 plan reading against `UndeclaredPlanBudget()`, and no surface for declaring a plan budget exists yet (13.4). So the reading answers "not applicable" for zai and kimi no matter what an operator declares, and `chooseFlatLane` stops at the deterministic duty-map order before any ratio is compared.
> **Pinning it here means the day the 13.4 plan-budget surface lands, this test fails and says exactly which claim has to be revisited.**"

and its failing assertion:

```go
if zai.Applicable {
    t.Errorf("the zai lane reports a comparable pressure ratio (%+v).\n"+
        "If a plan budget can now be declared, `chooseFlatLane` can separate two flat lanes on the real "+
        "reading and P3-LN-4 drain r2's recorded limitation is stale: the both-directions selection "+
        "case can and should be driven through the production adapter.", zai)
}
```

**This packet is that day.** The test's own message names the obligation: drive the both-directions selection case through the production adapter.

**MUST:** rewrite `TestLN4PlanDocumentedLaneHasNoComparablePressureAtV0` into a test that keeps every claim of it that is still true and inverts the one that is not:

- **KEEP** the control: a lane with no plan document answers from the tier-1 token gauge and a declared budget makes it comparable (`lanepressure_r2_test.go:45–55`).
- **KEEP** the premise guard: `metering.PlanDocFor(adapters.LaneZAI)` must still return a document, else the test's premise moved (`:64–66`).
- **KEEP** the unit assertion: `zai.Unit == "credits"` (`:73–75`).
- **KEEP, RENAMED** the honest-absence half: a **token** budget declared on a plan-documented lane still changes nothing, because that lane is not read from the token gauge at all. That finding is untouched by this packet and is the trap most likely to be re-introduced.
- **INVERT** the `zai.Applicable` assertion: with a **plan** budget declared, it MUST now be applicable.
- **MUST** rename the test so the name no longer asserts the retired claim, and **MUST** rewrite the head comment to record what changed and when, dated (§63 D8: stale comments are corrected, not left).

**MUST NOT** delete the file or weaken it to a skip.

### R9 — Wire deltas ADDITIVE; goldens regenerated only through the sanctioned path

§63 R3, restated at §64: *"A wire member no fixture exercises is a contract nobody agreed to."*

**MUST:** the meters payload's plan block (`api.LanePlanMeter` → `MeterLanePlan`, `internal/api/meters.go`) gains a **`budget` object**, `omitempty`: `{period_units, unit, window, period_start, period_hours, source, seeded_from, fraction, declared_by, declared_ts}`. **No dollar member.** `pressure` stays exactly as it is — a `*float64` that is non-null only when applicable.

**MUST:** the fixture world exercises the new member: at least one fixture lane's plan block carries a declared budget and therefore a non-null `pressure`, alongside at least one that keeps the undeclared shape (`internal/api/apifixtures_test.go:128–170` is where the plan blocks are built; the committed bodies are `web/src/fixtures/api/meters.json` and `meters-member.json`).

**MUST:** goldens regenerated **only** through `SINET_WRITE_API_FIXTURES=1 go test ./internal/api -run TestWebAPIFixtures` (`apifixtures_test.go:20`, `:75`).

**MUST:** state honestly in the commit body whether **any existing member changed value** — checked, not assumed. §64 C3 is the standing correction: *"'no existing member changed value' was inaccurate… A fixture claim that overstates its own innocence is the kind a reviewer stops checking."*

**MUST NOT:** touch `web/src` application source. The SPA adopts on its own pipeline.

### R10 — The selection proof at the REAL call sites, both directions

§63 drain r2 R1, verbatim: *"A correct function reached through a dropped argument is a broken feature… A test of the resolver in isolation cannot see a dropped argument — the same blind spot as the `fixedPressure` fake one round earlier."* §63 D3 records the same lesson from the other end: *"The gap that let this through was that the D5 test used a fake."*

**MUST:** the selection proof runs through `worker.Router` / `chooseFlatLane` with the **production** `routePressure` (real gauge, real DB, real checkpoints, real declared plan-budget rows) — never a `fixedPressure` fake and never `ReadPlanUnits` in isolation. Both directions: a commissioned lane **wins** when it is the less consumed, and **loses** when it is the more consumed, with only the consumption differing between the two runs.

**MUST:** mutation-verify — reverting the `routePressure` change collapses both directions into the deterministic order and the guard fails. State the mutation and its observed failure in the commit body.

### R11 — Honest absence as a property, not an example

**MUST:** a property-based test (fixed-seed, the §65 200-draw precedent) asserting: **for arbitrary consumption histories, with no plan-budget row declared, the `PlanReading` and the `worker.LanePressure` are identical to what the pre-packet code produces** — `Applicable=false`, `Pressure=0`, `BackgroundCeiling=0`, same `Unit`, same `Calls`, same `Consumed`, same `Windows`, same `Tier`, same `Assumed`/`AssumedNote`. This is spec-stated (S10.4: the denominator is the operator's budget, *"never an inferred provider window"*), so it is pinned as an invariant rather than as a case.

### R12 — Standing constraints

- **No new dependency.** `components.lock` untouched; no adopted code modified (CLAUDE.md: *"Adopt, don't fork"*).
- **$0 absolute.** Fixtures and fakes only; no live provider call on any path; no test may reach a network. Tier-L smoke stays behind its existing gate and is not armed.
- **No changes to registered BENCH-REG numbers** (`Spec/benchmark-preregistration-v1.md` §17 is the only door).
- **Serial tests:** `go test -p 1 ./...`. Disk is ~90% full — pull nothing, download nothing.
- **`plandata/*.json` and `lanedata/*.json` are untouched.** This packet declares budgets against the documents; it does not edit them.
- **No new Go constant** for any quota number, window hour, multiplier or fraction — the no-lane-constants source scan (§62/§63 D5, extended over `internal/worker`, `internal/shell`, `internal/stage`) stays green.

---

## 2. Seams and stubs

| Seam | Where | Shape | Note |
|---|---|---|---|
| `PlanBudgetStore` | `internal/api/meters_verbs.go` | 4 methods (R5) | api never imports metering (§11); nil ⇒ 503 |
| `PlanBudgetRecord` | `internal/api/meters_verbs.go` | transport record | no dollar member, ever (D5) |
| `PlanWindowRecord` | `internal/api/meters_verbs.go` | `{name, unit, window_hours, allowance, allowance_unverified}` | lets the boundary validate the window (§30) |
| `planBudgetAdapter` | `internal/shell/shell.go` | converts only | `budgetAdapter` precedent (`:1616`) |
| `*metering.PlanBudgets` | `internal/metering/planbudgets.go` | store (R2) | the ONE producer of the undeclared posture |
| `routePressure.pb` | `internal/shell/shell.go:1569` | new field | nil ⇒ today's posture exactly |
| `projMeter.planBudgets` | `internal/shell/shell.go:1675` | new field | nil ⇒ today's posture exactly |

**Stubs / deliberate non-implementations, to be named in code rather than discovered:**

- No **13.4 UI**. The verb + store are the honest half; the consumer is named (S15.9) and not invented — the `lane.go:255` precedent.
- No **enforcement**. A declared plan budget makes pressure *comparable*; it does not gate spawn or park a run. S10.4's two-gate enforcement (spawn gate + per-checkpoint gate) is not this packet, and any code suggesting otherwise is a defect.
- No **period rollover**. Re-declaring starts the next period, exactly as the token budget records (0017: *"a rollover would be a timer, and dueness comes from stored state, never from a ticker (CONVENTIONS §34)"*).
- No **observed-state overlay**. S10.4's `anthropic-ratelimit-unified-*` overlay rides the S11 proxy and is untouched; plan lanes expose no such headers anyway (S10.4, S10.5).

---

## 3. ⚙ settings

| Key | Role in this packet | Status |
|---|---|---|
| `budget.background_window_fraction` | the proposal fraction, read at `planunits.go:501` | **consumed by name**, unchanged, no new call site |
| `pressure.bg_admit_stop` | cited as the ground for OQ-2's max-binds rule | **not consumed**, not touched |
| `pressure.cache_read_weight` | token gauge only | untouched |

**Minted: none.** S18 stays 118 keys / 33 domains; S18.3 stays 3 data surfaces (R4).

**Open questions carried to the coordinator, unresolved:** **⚙ OQ-1** (the double duty of `budget.background_window_fraction` — §R4) and **OQ-2** (no spec rule for aggregating a lane's multiple windows — §R6). Neither may be silently decided by the executor.

---

## 4. Files expected to change

**New**
- `internal/storage/migrations/00NN_plan_budgets.sql`
- `internal/metering/planbudgets.go` (+ `planbudgets_test.go`)
- `internal/shell/planbudget_ln6_test.go` — **exists already, committed RED by grounding** (§6)
- `internal/api/plan_budget_verb_test.go` (or extend `meters_verbs_test.go`)

**Modified**
- `internal/api/meters_verbs.go` — record, seam, verb, card type, snapshot
- `internal/api/api.go` — `planBudgets` field + `protected("POST /api/meters/plan-budget", …)`
- `internal/api/meters.go` — the additive `budget` member on the plan block
- `internal/shell/shell.go` — `routePressure` (`:1569`, `:1583–1593`), `projMeter` (`:1675`, `:1725–1729`), composition (`:605`, `:977`), `planBudgetAdapter`
- `internal/shell/lanepressure_r2_test.go` — **R8**, rewritten and renamed
- `internal/api/apifixtures_test.go` + `web/src/fixtures/api/meters.json`, `meters-member.json` — **R9**, regenerated
- `P3/CONVENTIONS.md` — the §66 entry, incl. the R4 tally argument and both OQs
- `P3/STATE.md` — coordinator's, at landing

**Forbidden**
- `web/src/**` except the two regenerated `fixtures/api/*.json`
- `internal/metering/plandata/*.json`, `internal/adapters/opencode/lanedata/*.json`
- `internal/storage/migrations/0001–0024` (immutable, §6)
- `Spec/**`, `Docs/**`, `Research/**`
- `Presentation/`, `Research/Presentation/`, `Sinet-Logo.jpeg`, `tools/dbpeek|dbq|rescanseed|driftseed/`, `P3/gates/rw10-heal.sh`, `P3/gates/B6-clickthrough.sh` — **operator files, never staged**

---

## 5. Acceptance checklist

Concretely checkable. Every line is a command or a grep.

1. `go build ./...` clean.
2. `go test -p 1 ./...` green.
3. `grep -n "UndeclaredPlanBudget()" internal/shell/shell.go` → returns only guarded/absence paths; **no unconditional call inside `routePressure.Pressure` or `projMeter.LaneMeter`**.
4. `grep -rn "budget.background_window_fraction" internal/` → the key appears by name; no new float literal `0.5` stands in for it anywhere in `internal/metering` or `internal/shell`.
5. `go test -p 1 ./internal/settings -count=1` green → 118 keys / 33 domains unchanged.
6. `git diff --stat -- web/src` → only `web/src/fixtures/api/meters.json` and `meters-member.json`.
7. `git diff -- web/src/fixtures/api/` → the diff is **additive**; any existing member whose value moved is named in the commit body with its reason (§64 C3).
8. `grep -rn "usd\|USD\|price\|dollar" internal/metering/planbudgets.go internal/api/meters_verbs.go` → no dollar arithmetic on any plan-unit path.
9. `grep -c "CHECK" internal/storage/migrations/00NN_plan_budgets.sql` → every column constrained; `PRIMARY KEY (user_id, lane, window)` present.
10. `git diff --stat internal/storage/migrations/` → only the new file.
11. `internal/shell/lanepressure_r2_test.go` renamed + head comment rewritten and dated; its token-budget-on-a-plan-lane control still present.
12. Both RED tests from §6 are now **green for the right reason** (the surface landed, not the assertion weakened).
13. `git diff --stat components.lock go.mod go.sum` → empty.
14. No test reaches a network: `grep -rn "http.Get\|net.Dial\|SINET_LIVE" <new test files>` → nothing new armed.
15. The mutation for R10 is stated in the commit body with its observed failure output.
16. `P3/CONVENTIONS.md` §66 records: the sibling-not-reuse decision and its four grounds, the S18.3 grain citation, the tally argument, ⚙ OQ-1, OQ-2, and what did NOT land (R-§2 stubs).
17. `git status --porcelain` shows no operator file staged (see §4 Forbidden).

---

## 6. Acceptance-test specifications

Names, setup, exact assertions. Property-based where the invariant is spec-stated.

### T1 · `TestLN6PlanBudgetRowRoundTrips` — `internal/metering/planbudgets_test.go`
**Setup:** `t.TempDir()` DB, migrate, seed user `alice`.
**Assert:** `Declare` a `rolling-5h` row for `(alice, zai)` → `existed=false`, `prior` zero. `Row` returns it with every field byte-equal, timestamps round-tripping through RFC3339Nano UTC. Re-`Declare` with different units → `existed=true` and `prior` is the FIRST row exactly. `Row(alice, zai, "weekly")` → `ok=false`, nil error (the honest absence). `Rows(alice, zai)` → exactly the declared windows.

### T2 · `TestLN6PlanBudgetRefusesDefects` — `internal/metering/planbudgets_test.go`
**Assert, each with its own error naming what is wrong:** empty user / empty lane / empty window / `period_units <= 0` / `period_hours <= 0` / empty `declared_by` / a `source` outside `{proposal-seeded, operator-set}`. A row for a person who does not exist → `errors.Is(err, metering.ErrNoSuchPerson)`.

### T3 · `TestLN6TwoWindowsOnOneLaneKeepTheirOwnUnits` — `internal/metering/planbudgets_test.go`
**Setup:** declare `(bob, kimi, rolling-5h)` in `requests` and `(bob, kimi, weekly)` in `credits`.
**Assert:** both rows persist independently; neither overwrites the other; each `Row(...).Budget()` produces a `PlanBudget` whose `ReadPlanUnits` reading carries **that window's** unit (`requests` vs `credits`) via `doc.QuotaUnit`. **Assert the units are not equal** — the single-scalar collapse this grain exists to prevent (R1).

### T4 · `TestLN6ProposalSeedsFromTheAllowanceAtTheRegistryFraction` — `internal/metering/planbudgets_test.go`
**Assert:** proposing from `zai`/`rolling-5h` yields `PeriodUnits == 28000 × settings.Float("budget.background_window_fraction")`, `SeededFrom == "rolling-5h"`, `Fraction ==` that value, `PeriodHours == 5`. **Then move the registry value** and assert the proposal moves with it — proving the fraction is read by name and is not a constant (R4). **Then** assert proposing from `kimi`/`weekly` **fails** with `ErrPlanDoc` and a message naming the unverified allowance (R5; `planunits.go:493–500`).

### T5 · `TestLN6DeclaredPlanBudgetMakesPressureApplicable` — `internal/shell/`
**Setup:** `newPressureEnv(t)` (`internal/shell/zai_ln2b_test.go:127`) — real DB, real gauge, real checkpoints on `r-z` (zai) and `r-a` (anthropic).
**Assert:** before declaring, `e.rp.Pressure(ctx, "alice", LaneZAI)` → `Applicable=false` (today's behaviour). After declaring a `rolling-5h` plan budget → `Applicable=true`, `0 < Ratio`, `Unit == "credits"`, and `Ratio == consumed / PeriodUnits` where `consumed` comes from the reading. **Assert `Ratio != consumed / 28000`** — the exact arithmetic §63 D1 found wrong; the provider allowance is a seed, never a denominator.

### T6 · `TestLN6CommissionedLaneWinsAndLosesOnConsumption` — `internal/shell/` — **the selection proof (R10)**
**Setup:** a `worker.Router` composed with the **production** `routePressure` (no fake), coverage for both `anthropic` and `zai`, an execution duty with a zai alternate seat, declared budgets on **both** lanes (token budget on anthropic, plan budget on zai).
**Assert, direction A:** with zai's consumption low relative to its budget and anthropic's high, `Route`/`chooseFlatLane` seats the **zai** lane and the reason string names the pressure comparison.
**Assert, direction B:** identical setup with only the consumption reversed → seats **anthropic**.
**Assert:** the two runs differ in nothing but consumption. **Mutation-verify:** restore `UndeclaredPlanBudget()` in `routePressure` → both directions collapse to the deterministic order and this test fails.

### T7 · `TestLN6NoRowIsIdenticalToTodayAcrossArbitraryHistories` — `internal/metering/` — **property (R11)**
**Setup:** fixed seed, ≥200 draws over `(call count 0..50, timestamps spanning in/out of any period, multiplier windows, both plan lanes)`.
**Assert per draw, with no plan-budget row:** `Applicable == false`, `Pressure == 0`, `BackgroundCeiling == 0`, and `Calls`/`Consumed`/`Unit`/`Windows`/`Tier`/`Assumed`/`AssumedNote` equal to the reading taken with `UndeclaredPlanBudget()` — i.e. the packet is invisible until a row exists.

### T8 · `TestLN6DeterministicOrderStandsWithNoPlanBudget` — `internal/shell/`
**Assert:** through the real `chooseFlatLane`, with two covered flat lanes and **no** plan budget, the seat is the duty-map order's first and the reason contains the unchanged wording *"no declared automation budget"* and *"never dollars — D5"*. The honest absence is pinned as behaviour, not as a comment.

### T9 · `TestLN6PlanBudgetVerbAuthorityAndValidation` — `internal/api/`
**Assert** (fake `PlanBudgetStore`, the `fakeBudgetStore` precedent at `meters_verbs_test.go:33`): nil store → **503 `not_wired`**. A member declaring their own → 200. A member declaring another's → **403** whose message names S15.2/D10. The operator declaring another's → 200 with `declared_by` = the operator. A ghost person → the `requirePerson` refusal. Missing `lane` / missing `window` / `period_units <= 0` / `period_hours <= 0` / an **unknown window name** → **400** each, with the unknown-window message naming the windows that do exist. A body carrying any dollar-shaped member → not accepted as a budget (no dollar field exists to bind to).

### T10 · `TestLN6PlanBudgetVerbRecordsOldToNew` — `internal/api/`
**Assert:** the first declaration emits `decision.recorded` with `card_type == "plan_budget"`, `old == {"declared":false}` and a populated `new`; the second emits `old` = the first row exactly (the `oversight_test.go:593` precedent). The audit row lands **after** the store act — a store error produces **no** decision row.

### T11 · `TestLN6ProposalPathIsTheProductionConsumer` — `internal/api/` + `internal/shell/`
**Assert:** `{"from_proposal": true}` with no `period_units` stores a row with `source == "proposal-seeded"`, `seeded_from == "rolling-5h"` and a non-zero `fraction`; the same call against `kimi`/`weekly` returns **400** naming the unverified allowance. **Assert by source scan** that `ProposePlanBudget` now has a non-test caller in `internal/shell` — the "wired and read by nothing" class (§63 D2) closed by observation, not by claim.

### T12 · `TestLN6MetersReadAgreesWithTheRouter` — `internal/shell/`
**Assert:** for the same `(person, lane)` and the same instant, the `plan.pressure` served by `projMeter.LaneMeter` equals the `Ratio` returned by `routePressure.Pressure`, in both the declared and undeclared states. The drain-D2 self-contradiction (`shell.go:1666–1674`) cannot recur.

### T13 · `TestLN6WireDeltaIsAdditive` — `internal/api/`
**Assert:** the golden bodies contain a populated plan `budget` object on at least one lane and retain an undeclared plan block on another; every pre-existing member name is still present. Regeneration only via `SINET_WRITE_API_FIXTURES=1`.

### T14 · (R8) the rewritten `lanepressure_r2_test.go`
**Assert:** the anthropic control still holds; a **token** budget on a plan-documented lane still changes nothing; a **plan** budget makes it applicable; `zai.Unit == "credits"`; `PlanDocFor(LaneZAI)` still returns a document.

---

## 7. Tests committed RED by grounding

Committed with this brief, in `internal/shell/planbudget_ln6_test.go`, package `shell`. Both **compile** and fail for the right reason — the §65 lesson: *a red commit that does not COMPILE is a finding.* Structural red only; the behavioural reds (T5–T7, T9–T13) are specified above for the executor because they cannot compile before the surface exists.

- **`TestLN6PlanBudgetRowsArePersisted`** — migrates a real DB and queries `sqlite_master` for table `plan_budgets`. **RED today**: no such table.
- **`TestLN6RoutePressureConsumesDeclaredPlanBudgets`** — source scan of `internal/shell/shell.go` asserting `metering.UndeclaredPlanBudget()` no longer appears inside `routePressure.Pressure` or `projMeter.LaneMeter`. **RED today**: it appears in both (`:1588`, `:1729`). The no-lane-constants source-scan precedent (§62/§63 D5).

**The executor MUST NOT delete or weaken either.** They go green when the surface lands, and only then.

---

## 8. Non-goals

No `web/src` application source. No 13.4 settings UI. No per-person duty-map surface. No C5 enforcement. No canary changes. No changes to registered BENCH-REG numbers. No spawn-gate or per-checkpoint budget enforcement. No period rollover timer. No observed-state overlay. No edits to `plandata`/`lanedata`. No new dependency. No live provider call.

---

## Coordinator dispositions (appended 2026-08-25, before executor launch)

**OQ-1 — PRE-EXISTING double duty; consume as ratified, split at the gate.** The `budget.background_window_fraction` double duty (allowance-proposal fraction at planunits.go:501 vs background-ceiling fraction at :569/pressure.go:96) predates this packet — LN-2B ratified `ProposePlanBudget` consuming it, and this packet merely makes that function reachable. The compounding direction is conservative (a proposal-seeded budget gives background f² of the advertised allowance — it spends LESS, never more) and every seeded row is "assumed"/operator-editable by design. The executor consumes the fraction exactly as the code already does: NO new ⚙ key minted, NEITHER use changed. **Gate item recorded:** an S00.9 amendment proposal to split the knob into two registry keys (seeding fraction / background ceiling) with its S18 sweep — presented at the LN gate batch, decided there.

**OQ-2 — MAX-BINDS RATIFIED as the packet reading.** A lane's selection ratio is the maximum across its windows: the most-constrained window binds. This is the reading S10.4's headroom rule implies — a lane near exhaustion in ANY window must report high pressure so routing steers away; averaging or min would let an exhausted 5h window hide behind a fresh weekly one, which is the overrun class S10 exists to prevent. Reversal stays confined to the one aggregation function + its test, as the brief specifies. The gate sees the choice and the two alternatives by name.
