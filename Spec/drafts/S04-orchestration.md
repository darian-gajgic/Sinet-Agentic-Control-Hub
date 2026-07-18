## S04 — Orchestration within D6

**Scope:** How task work is and is not split across agents: the coordinator/helper topology made operational, the earned-helper triggers, the brief/report context firewall, spawn budgets and the spawn log, the helper lifecycle with acceptance screening and sibling-failure containment, artifact-collision serialization, and cost shape/attribution.
**Binding inputs:** D6 · feature list 2.4, 7.7, 14.1, 14.4 (+ 3.4, 4.6, 5.6 slices) · R06 §4 as ratified [G1 D1.1] · G1 Def.8, Def.9, Def.10, riders 1–2 · G3 Def.3 · [SPIKE P2-S4], [SPIKE P2-S5] · S00.7 glossary (coordinator, helper).

### S04.1 Topology: D6 made operational

The unit of delegation is the **(coordinator, helpers)** tree of one task:

- **One coordinator per task.** The coordinator is the single engine session driving the task's current stage; at no moment does a task have two coordinators (D6). Stage transitions replace the coordinator's context, not its singular role — fresh-context-per-stage runs on the Task Context Ledger [G1 D1.1; XREF:S05].
- **Helpers are isolated.** A helper is a separate engine sibling session with its own confinement-class-appropriate sandbox [XREF:S11], its own context built only from its brief (S04.3), and no shared conversational state with anything. Helpers are ephemeral: they exist from spawn to terminal outcome and are never standing formations (14.4).
- **Sub-helpers within the depth cap.** Depth counts levels below the coordinator: helpers run at depth 1, sub-helpers at depth 2. `⚙ orchestration.depth_cap = 2` (D6 default); **depth 1 is the operating norm** — nothing in the evidence base demonstrates value at depth ≥3 [R06 §4.3; G1 Def.8]. A helper may propose a sub-helper through the same control-plane spawn API, subject to the same triggers, budgets, and logging; a spawn request at depth > cap is refused.
- **No lateral messaging at any depth** (D6; 14.1). The only communication edges are the spawn edges: brief down, report up. A sub-helper reports to its spawning helper, which integrates the result into its own report — the coordinator never receives a sub-helper report directly, and no helper ever sees a sibling's report, brief, or existence. The control plane provides **no** helper-to-helper channel; a conformance test attempts a lateral send and asserts refusal (S04.5).
- **The control plane is the only enforcement point.** No engine enforces D6: surveyed engines ship fixed or silently-bypassable depth guards, no spawn-reason logging, and no lateral-messaging prohibition — the cap, budgets, logging, and no-lateral rule are therefore control-plane-owned and engine-independent [R06 §2.5, §7; G1 D1.1]. Enforcement is structural (the spawn API is the only way a helper can come to exist), not advisory.
- **Engine-native spawning is disabled on every substrate** [G1 rider 2]. Sinet holds the sole-controller posture: all agent spawning is control-plane-owned; engine-native subagent features stay off on every lane. A helper exists only as a control-plane spawn lowered through the adapter's D3 verbs — full stop. [SPIKE P2-S5] confirmed native spawning is fully disablable via compiled config but that the guarantee is **compound** (one knob per channel); the per-lane disablement knobs and their conformance probes are S03's [XREF:S03]. An engine-native spawn observed at runtime is a platform defect, not a policy option. R06-OQ2 (a narrow native micro-fanout exception) is deferred, not open; it resurfaces only at S03 review with an explicit operator reminder [G1 rider 2].

**Boundary lines.** S03 owns lowering spawns onto engines and the native-spawning disablement mechanics. S05 owns context/ledger internals (what a brief's context slice is built from). S07 owns verification of helper-derived deliverable content beyond the S04.5 acceptance screen. S08 owns which worker/model/lane a helper runs on. S10 owns scheduling, pressure accounting, and receipt mechanics. S11 owns helper confinement classes.

### S04.2 Single-agent-first: the earned-helper test

The default formation for every task is **one agent** — the coordinator working alone (14.4; single-agent-first [G1 D1.1]). Helpers are never a default and never decorative: a spawn is legal only when at least one of **exactly three** triggers holds, and the trigger is recorded in the spawn log as the machine-readable spawn reason (D6; 7.7) [R06 §4.1]:

- **T-CTX — context protection.** The subtask would flood the coordinator with material it will not reference again (grep/log/doc trawls, large read-ins). Even a single helper pays here.
- **T-PAR — read fan-out.** ≥2 genuinely independent, read-only facets whose results merge only at synthesis. NEVER for write work.
- **T-SPEC — specialization quarantine.** The subtask needs a tool/permission set the coordinator must not hold (including parsing untrusted content — the injection blast-radius case [XREF:S11]) or a materially different model/effort tier.

Anything that matches no trigger runs single-agent. The coordinator's prompt carries the effort rubric (models misjudge effort unaided [R06 §2.5]): simple lookup → 0 helpers, inline; one noisy read → 1 helper; independent facets → 2–4 helpers; more requires spawn-budget headroom (S04.4).

Formation rubric — 2.4's "best practice chooses structure and depth within the cap" made concrete [R06 §4.1; G1 D1.1]:

| Task shape | Default formation |
|---|---|
| Simple lookup / anything the coordinator does in-window | coordinator only |
| One noisy read subtask (log/doc/code trawl) | 1 helper (T-CTX) |
| 2–4 independent read-only facets | parallel helpers (T-PAR), reports merged at synthesis |
| Parsing untrusted external content | 1 quarantined helper (T-SPEC), minimal tools, report-as-data [XREF:S11] |
| Judgment/review subtask on finished work | 1 clean-context helper (T-SPEC), no shared history (platform verification stages proper are S07's [XREF:S07]) |
| Sequential / tightly-coupled reasoning | coordinator only — never decompose |
| Any write work | coordinator writes, single-threaded |
| Mass mechanical fan-out (≥50 items) | out of v0 scope — parked (see Deferred) |

**Writes are single-threaded, always** [R06 §4.1]: one writer per workspace; helpers take write work NEVER. Parallel write work exists only as separate sibling *tasks* in isolated workspaces, coordinated by artifact claims [XREF:S02] — never as helpers of one task.

Spawning within budgets is not an outward effect and needs no human approval; it is logged, not gated. The only human gates in this section are the spawn-budget overrun gate (S04.4) and operator ceiling changes [G1 rider 1].

### S04.3 The context firewall: brief in, report out

The helper context contract is the anti-15× mechanism: Nexus's ~15×-class token blowup came from re-sending context per stage, and an undesigned contract (full re-sends, paraphrase chains, cold caches) reproduces it; the designed shape below yields ≈2–4× single-agent cost (S04.7) [R06 §2.4].

**Down: the brief.** A **brief** is the complete context a helper receives — structured prose with mandatory sections; helpers start with **no inherited coordinator history**, ever [R06 §4.2]. Vague briefs are the measured failure mode (duplication, misinterpretation), so every section is mandatory:

```
HELPER BRIEF (v0 contract)
objective:        one paragraph — what "done" looks like
output_contract:  report ≤ ⚙ orchestration.report_tokens; sections FINDINGS / EVIDENCE (paths, URLs) / GAPS;
                  bulk artifacts → task workspace, referenced by path
tools_sources:    allowed tools; where to look first
boundaries:       read-only unless stated; explicit "do NOT …" lines; at budget, stop and report partial
budget:           ≤ N turns, ≤ M tokens (this spawn's effective values)
context:          only the slice this helper needs — drawn from the Task Context Ledger [XREF:S05], never raw history
```

The brief is hashed at spawn (`brief_hash` in the spawn record) and checkpointed [XREF:S02].

**Up: the report.** A **helper report** is ≤ `⚙ orchestration.report_tokens = 2000` [G1 Def.8] plus filesystem artifact references: restorable compression — keep the path/URL, drop the bulk. Raw material above `⚙ orchestration.bulk_offload_tokens = 20000` goes to the task workspace as files (under a per-spawn subpath [coordinator-draft]), never into the return message [R06 §4.2]. Helpers never see the task's full history and the task never ingests a helper's full transcript — the report and its artifacts are the entire upward surface.

**Relay verbatim.** Where a helper conclusion feeds downstream consumption, the coordinator relays it verbatim — paraphrase relay is a measured degradation ("telephone game") and a regeneration cost [R06 §4.2].

**Reports are data, not instructions.** On ingestion the control plane runs an instruction-pattern screen over the report; hits annotate (mark the span as data), never reword. The screen is advisory by design — the structural defense is minimal tools and minimal context per helper, and confinement classes decide which helper classes may read untrusted content at all [R06 §4.2, §7; XREF:S11]. Verification of anything a steered helper contributed to a deliverable is S07's [XREF:S07].

**Cache-aware mechanics** [R06 §4.2]: helper templates keep stable per-template prefixes; briefs are append-only; identical-prefix fan-outs are staggered by first response when `⚙ orchestration.stagger_identical_prefix = on`, because simultaneously launched identical-prefix helpers each pay a full cache-cold cost on API-priced semantics — whether that also holds against subscription windows is exactly the P-T03-1 unknown (S04.7).

### S04.4 Budgets, caps, and the spawn log

All numbers are ⚙ operator-editable settings with the ratified value as default [G1 Def.8; G1 rider 1]; the settings registry carries the `(value, floor, ceiling)` clamps and audit trail [XREF:S18]. The scheduler MAY auto-scale a task's effective values by task complexity **only within the operator ceilings**; any per-task effective value above the ratified default (possible only where the operator has raised the ceiling) is an auto-raise and is itemized on the task receipt [G1 rider 1; XREF:S10].

- `⚙ orchestration.max_concurrent_helpers = 4` per task — counting all live helpers of the task across depths [coordinator-draft on the all-depths reading]. Field defaults of 10–25 are cloud-scale; 4 fits one laptop plus consumption pressure [R06 §4.3].
- `⚙ orchestration.helper_turns = 20` and `⚙ orchestration.helper_tokens = 80000` per helper, any depth — envelope over observed real fan-outs (45–70k tokens) [R06 §4.3; G1 Def.8].
- `⚙ orchestration.spawn_budget = 8` per task **including sub-helpers and retries** (a retry is a fresh spawn; see S04.5). Exhausting it stops further spawning; overrunning it happens only through an operator-visible gate — an approval card, never silently [R06 §4.3].

**The spawn log.** Every spawn writes a **spawn record** before the helper session starts; no code path creates a helper without one (D6: "every spawn logged with its reason"). No industry schema exists for spawn reasons — Sinet defines its own, shaped to map onto OTel `invoke_agent` spans for future export [R06 §2.5, §4.3]:

| Field | Content |
|---|---|
| `spawn_id`, `ts` | record identity, spawn time |
| `task_id`, `owner` | the task and the person billed (15.6; 3.4) — *who* |
| `parent_session` → `child_session` | spawner and spawned sessions; depth-lineage — *who* |
| `depth` | 1 or 2 |
| `trigger` | `T-CTX \| T-PAR \| T-SPEC` — *why*, machine-readable |
| `reason` | one human-readable line (7.7) — *why*, auditable |
| `brief_hash` | hash of the brief as issued |
| `model`, `lane` | what the helper runs on (selection itself is S08's [XREF:S08]) |
| `budget` | `{turns, tokens}` — this spawn's effective (post-auto-scale) values |
| `outcome` | terminal lifecycle state (S04.5): `ACCEPTED \| REJECTED \| SALVAGED \| FAILED \| ESCALATED`, written at the terminal transition |

Spawn-request refusals (depth over cap, budget exhausted, missing trigger/brief, lateral attempt) are logged as refusal events naming the failed check — refusals are evidence, and the conformance tests assert them (S04.5). Spawn records and lifecycle transitions are event-log rows in `platform.db` [XREF:S02]; receipts join helper usage through the metering ledger [XREF:S10].

### S04.5 Helper lifecycle, acceptance screen, and containment

Lifecycle — every transition a logged event [R06 §4.6]:

| State | Meaning | Exit |
|---|---|---|
| PROPOSED | coordinator (or helper, for a sub-helper) emits a spawn request: trigger + reason + brief + budget | validation → SPAWNED, or logged refusal |
| SPAWNED | control plane validated depth/budget/trigger, wrote the spawn record, started the sibling session via the adapter [XREF:S03] | → RUNNING |
| RUNNING | helper streams; checkpoints per paid call (D7) [XREF:S02] | clean report → RETURNED; budget/timeout/engine death → SALVAGED |
| RETURNED | report received; acceptance screen runs | → ACCEPTED or REJECTED |
| ACCEPTED | report integrated; verbatim relay downstream | terminal |
| REJECTED | screen failed; up to `⚙ orchestration.helper_retry_limit = 1` retry as a **fresh helper with a revised brief** (fork-don't-poison) [R06 §4.4] | retry → PROPOSED (new spawn record); second failure → FAILED or ESCALATED |
| SALVAGED | partial output carried with an **explicit unfinished marker** — never silent loss [R06 §4.4] | terminal (partial screened and consumed as partial) |
| FAILED | facet abandoned; the gap is recorded in the coordinator's own output — a dropped facet is never silent (1.4 spirit) | terminal |
| ESCALATED | unresolvable → human gate; the escalation route is test-proven, because a finding that dies in a log is a platform defect (5.6) | terminal |

Nothing exits RETURNED without an acceptance verdict — a helper report that dies unread is a platform defect [R06 §4.6].

**Acceptance screen — conformance-only at v0** [G1 Def.9 as amended at close; G3 Def.3]. Before any report is consumed, the control plane checks contract conformance: mandatory sections present, size within `⚙ orchestration.report_tokens`, declared boundaries respected, unfinished marker consistent with the exit path. It judges form, not truth. A **local-model plausibility screen** is NOT part of v0: it becomes admissible only when the local-tier battery clears the pre-registered bar on real helper outputs [G3 Def.3; XREF:S12] — and even then it annotates, never auto-kills [G1 Def.9]. Everything beyond the screen — whether helper-derived content is *correct* — is S07's two-axis verification at the deliverable level [XREF:S07].

**Sibling-failure containment (Sinet-owned).** The field's default is the opposite — fail-fast sibling cancellation or undefined behavior — so containment is explicit control-plane logic, not an engine property [R06 §4.4, §7]:

- One helper's death, derailment, budget exhaustion, or rejection NEVER cancels siblings and NEVER fails the task. Siblings run to their own outcomes; the coordinator proceeds with what it has.
- Helper death is a scheduling event, not a task failure (the D4 posture applied to helpers). A dead helper costs a resume, never spent tokens: brief-at-spawn, report-at-return, and per-paid-call usage are all checkpointed (D7) [R06 §4.5; XREF:S02].
- Derailment (loop/silence/runaway flagged by the watchdogs [XREF:S14]) follows the platform rule: pause-and-flag, never auto-kill [G1 D1.3].
- Containment and enforcement are proven, not assumed — standing conformance-suite entries [XREF:S14] MUST attempt: depth-3 recursion; a lateral send; an engine-native spawn (per lane [XREF:S03]); an over-budget spawn; and a mid-run helper kill — asserting refusal for the first four and clean containment-with-salvage for the fifth [R06 §7].

### S04.6 Artifact collisions: serialize-by-deny

Helpers hold no write claims on project artifacts (writes are coordinator-only, S04.2), but claimed-artifact collisions can still arise around a task's tree — e.g., a proposed helper whose declared reads require a claim a sibling task holds, or workspace-artifact overlap. The rule: **when parallel work would collide on claimed artifacts, the scheduler serializes it** — the claim is denied, the spawn/work queues until release, and nothing is ever overwritten. Deny-and-retry is live-probed viable at ≈ +1 turn of overhead [SPIKE P2-S4]; TBD-P3(re-confirm P2-S4 serialize-by-deny behavior on the default worker model — spike caveat). Claim semantics are S02's [XREF:S02]; queueing/priorities are S10's [XREF:S10].

### S04.7 Cost shape, attribution, and the caching unknown

- **Designed cost shape:** a coordinator + helpers run under the S04.3 contract costs ≈2–4× the same task single-agent; 15×-class multipliers are the signature of an undesigned context contract (full re-sends, paraphrase chains, cold caches) and their absence is what S04.3 exists to guarantee [R06 §2.4]. The formation rubric keeps the multiplier paid only where delegation demonstrably wins.
- **Attribution:** every helper is paid work billed to the task's requester (3.4); per-spawn model + lane + usage lands itemized on the receipt (7.7) via the metering ledger [XREF:S10]. Helper lane choice among flat-rate lanes routes by consumption pressure, never dollars (D5); mechanical helper duties default to the local lane — the permanent free tier — with selection machinery in S08 [R06 §4.5; XREF:S08].
- **The caching unknown (P-T03-1):** cache discounts are documented for API billing only; whether cached reads weigh less against *subscription* quota windows is publicly unspecified for every lane [R06 §7]. Until resolved, consumption pressure weights cache reads at 0.1×, labeled **"assumed"**, with receipts keeping raw counts [G1 Def.10; XREF:S05]. Resolution (provider watchlist [XREF:S14]) re-opens the stagger policy (S04.3) and the pressure weighting — as a settings change with audit trail, not a redesign.

**Settings introduced (⚙):**

| Name | Default | Clamp/range | Ratified by |
|---|---|---|---|
| `orchestration.depth_cap` | 2 | 0–2 at v0; >2 unevidenced, requires deliberate operator ceiling change | D6; G1 Def.8 |
| `orchestration.max_concurrent_helpers` | 4 per task (all depths) | 0–ceiling; auto-scale per complexity within ceilings, auto-raises on receipts | G1 Def.8 + rider 1 |
| `orchestration.helper_turns` | 20 per helper | 1–ceiling; same auto-scale rule | G1 Def.8 + rider 1 |
| `orchestration.helper_tokens` | 80,000 per helper | auto-scale within ceilings | G1 Def.8 + rider 1 |
| `orchestration.spawn_budget` | 8 per task incl. sub-helpers + retries | overrun only via operator-visible gate | G1 Def.8; R06 §4.3 |
| `orchestration.report_tokens` | 2,000 per report | screen-enforced (S04.5) | G1 Def.8 |
| `orchestration.bulk_offload_tokens` | 20,000 | raw material above this → workspace files, path-referenced | R06 §4.2; G1 D1.1 |
| `orchestration.helper_retry_limit` | 1 | 0–ceiling | R06 §4.4; G1 D1.1 |
| `orchestration.stagger_identical_prefix` | on | on/off | R06 §4.2; G1 D1.1 |

**Known problems owned here** (ids assigned here as P-T03-1..4 for the R06 §7 problem set, per the P-T##-# convention; S17 register reconciles):

- **P-T03-1 — caching-vs-subscription-quota unknown:** mitigated by the 0.1× "assumed" weighting + raw counts on receipts [G1 Def.10]; resolution owned by the provider watchlist [XREF:S14]; re-opens stagger + weighting as settings changes (S04.7).
- **P-T03-2 — sibling-failure containment is Sinet-owned:** discharged by the S04.5 containment rules + the mandatory killed-helper conformance test.
- **P-T03-3 — no engine enforces D6:** discharged by control-plane-only enforcement (S04.1/S04.4) + violation-attempt conformance tests (S04.5); native spawning disabled per lane [G1 rider 2; XREF:S03].
- **P-T03-4 — helper-report injection surface:** report-as-data ingestion screen owned here (S04.3, advisory); the structural defense (per-helper tool restriction, confinement classes for untrusted-content helpers) is S11's [XREF:S11]; verification of steered output is S07's [XREF:S07].

**Deferred / parked:**

- R06-OQ2 engine-native micro-fanout exception → re-entry ONLY at S03 adapter-spawning review, with the explicit operator reminder [G1 rider 2] (queued in campaign state).
- Local-model plausibility screen on helper reports → re-entry when the T15 battery meets the pre-registered bar on real helper outputs [G3 Def.3; XREF:S12].
- Script-driven mass fan-out lane (≥50-item mechanical) → re-entry post-benchmark-gate (15.3) when a real workload exists [R06 §3].
- Managed-Agents-shaped first-party orchestration on a subscription lane → watchlist item; if it GAs subscription-covered, evaluate as the Anthropic-lane *mechanism* behind this same contract — the policy layer is mechanism-agnostic [R06 §4].
- Parallel-writer unfreeze → only on credible measured evidence of reliable parallel writers (currently zero) [R06 §4].

**Coverage:**

| Feature-list item | Where |
|---|---|
| D6 (topology, depth cap, no lateral, logged spawns) | S04.1, S04.4 |
| 2.4 automatic orchestration within the topology | S04.2 (triggers, formation rubric) |
| 7.7 routing accountability — spawn-reason slice | S04.4 spawn record; receipts S04.7 (worker/model choice [XREF:S08]) |
| 14.1 no chatting swarms | S04.1 (no lateral, structural) |
| 14.4 no standing army / machinery-when-earned | S04.2 (earned-helper test) |
| 3.4 / 3.6 slices — helper billing + receipt itemization | S04.7 [XREF:S10] |
| 4.6 slice — helper failure noticed, contained, surfaced | S04.5 |
| 5.6 slice — tested escalation from delegation | S04.5 (ESCALATED; conformance tests) |

**Open items for G4:** none — every load-bearing choice in this section is ratified (D6, G1 D1.1/Def.8–10/riders 1–2, G3 Def.3); the two [coordinator-draft] tags (all-depths concurrency counting; per-spawn workspace subpaths) are clarifying readings, flagged for G4 attention, not open choices.
