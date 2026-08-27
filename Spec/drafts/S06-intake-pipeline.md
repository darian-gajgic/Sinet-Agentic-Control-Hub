## S06 — Intake: interview → specification → plan

**Scope:** Everything between "a request arrives" and "an approved plan exists": triage, the clarity-driven interview, the specification-with-numbered-AC contract, the plan artifact, the deterministic spine, plan self-attack, and the approval/freshness contract — with ceremony scaled by stakes.
**Binding inputs:** R03 (T05, whole report; §4 is the ratified pipeline shape per G1 findings digest + D1.1); G1 P8/P10/P11, D1.1/D1.3/D1.7, Def.5/Def.11/Def.12, riders 1–3; G2 Def.2/Def.3, D2.7; G3 T15 digest (ceremony cut line), D3.4 (compose-when-earned boundary); feature list 1.1–1.10, 2.3, 2.5, 4.3, S1.6, S3.2, 13.5; Nexus traps P45/P46/P47 as carried in R03 §1.

### S06.1 Pipeline shape and stage boundaries

Every task passes through five stages. Only two touch a paid model; the rest are platform code and local duty aliases [R03 §4; G1 D1.1].

| # | Stage | Runs on | Confinement | Emits → Task Context Ledger [XREF:S05] | Boundary |
|---|---|---|---|---|---|
| 0 | Triage | local duty aliases + deterministic rules | control plane (no task sandbox) | intake record (family, stakes tier + floor reasons, size/cost guess, data-bearing flags, registry-slice ref) → artifacts + decisions | automatic |
| 1 | Interview → SPEC → PLAN | planning model (S06.10); utility model for phrasing duties | C1 read-only sandbox, helper-inherited (S06.6) | SPEC + PLAN artifact pair, status `draft` → artifact refs | D7 checkpoint |
| 2 | Deterministic spine | platform code — no model | — | spine findings (coverage table, floor re-check, size delta, research-node presence) → decisions | automatic; may bounce once to Stage 1 |
| 3 | Critique (self-attack) | planning-model class, fresh context | artifact-only input | verdict (PASS / REVISE / SPEC-DOUBT / TIER-UP) + numbered findings → decisions + artifacts | verdict-routed |
| 4 | Approval | the requester [D10] | — | approval record (who/when/card version/choices) → decisions; objective + frozen ACs + constraints/danger zones pinned [G1 Def.12] | human gate — waits |

- Every stage boundary is a D7 checkpoint; an interrupted intake resumes from its last artifact, never by replaying conversation [D7; XREF:S02].
- **No stage ever receives another stage's transcript.** Inter-stage context is the artifact pair plus ledger slices; this kills the measured multi-stage token-multiplication trap structurally, not by tuning [R03 §3-C, §4 "Ceremony economics"; P45-adjacent Nexus 3.5×-spec-stage evidence per R03 §1]. Stage-fit context budgets apply [G1 Def.11; XREF:S05].
- **Gates wait.** No intake gate auto-proceeds on a timer, at any tier [R03 §5 (timed soft gates rejected); D7; D10]. Answering a card resumes the pipeline in place (4.3).
- The restate-and-confirm loop of 1.3 is realized as: SPEC restatement on the approval card, iterated through **Re-plan** / **Re-interview** actions until the requester approves; approval is the confirmation (S06.9).
- After approval, worker/model/lane selection for execution is S08's [XREF:S08]; verification against the frozen ACs is S07's [XREF:S07]; card rendering is S15's [XREF:S15]. This section owns pipeline logic and artifact contracts.

### S06.2 Stage 0 — Triage

On every request, before any paid model runs [R03 §4 Stage 0]:

1. **Task-family detection** (2.3): software / research / content / data / chore, by local duty alias [XREF:S12]. v0 ships the software family plus a generic fallback set; further family question sets arrive with their domains (S00.2 scope) [R03 §4; XREF:S19].
2. **Registry match** (S1.6): the matched project/repository registry entry — conventions, commands, danger zones — is injected so the interview never asks what the platform already knows; danger zones feed the stakes floor [R03 §4 Stage 0; registry home XREF:S13].
3. **Stakes + size guess** (1.8, 2.5): a local classifier proposes a stakes tier (S06.4) and a size/cost estimate in API-equivalent USD (D5 reporting currency; price table [XREF:S10]).
4. **Data-bearing detection** (1.9/P47): the deterministic trigger list of S06.3 first; a local classifier second, which may only *add* the flag, never remove it [R03 §2.7, §4].

**Deterministic floors override everything upward.** Any of: a planned outward effect, new spend, credential touch, shared-asset write, or a named regulated domain forces the high tier regardless of classifier output and regardless of the requester's convenience settings (1.8) [R03 §2.6 (OWASP rule-floor pattern), §4 Stage 0]. Classifier failure fails closed: the task is treated as high-stakes [R03 §2.6]. The requester's own accept-time commit of a reviewed deliverable (6.3, D9) is the requester's action, not a task-issued outward effect, and does not by itself trip the floor. Floors size *ceremony only* — safety never depends on them, because D7 gates every outward effect as a proposal no matter what intake decided [D7; R03 §2.6].

Misclassification is cheap by construction: underestimates are caught by the Stage-2 size-delta rule before expensive work; overestimates are caught by the preview and the run ceilings (2.5, 3.7) [R03 §4 Stage 0]. At run end, actual consumption is recorded against the Stage-0 estimate; classifier calibration is owned by the eval practice [P-T05-4; XREF:S14].

### S06.3 The P47 research-trigger list `[coordinator-draft]` (discharges R03-OQ5)

Tasks whose output depends on facts in the world always include live research as a required step (1.9); the trigger layer is deterministic rules first, classifier second [R03 §2.7, §4; G1 follow-ups: R03-OQ5 assigned to coordinator at P2]. The v1 rule file ships as an operator-editable, versioned platform file (`intake/p47-triggers`, git-tracked like knowledge objects [XREF:S09]); seed content:

| Id | Trigger class | Cue examples (non-exhaustive) |
|---|---|---|
| P47-1 | Prices & costs | currency amounts; price/fee/tariff/subscription/tier/"cheapest" |
| P47-2 | Products & vendors | named commercial products, models, SKUs, services; availability; "best X"/comparisons |
| P47-3 | Laws, regulation & terms | legal/tax/licensing/compliance references; provider ToS |
| P47-4 | Technical interfaces & versions | API/SDK/library/package names whose version, capability, or deprecation status the work depends on; model capabilities |
| P47-5 | Schedules & external events | dates, timetables, opening hours, deadlines not set by the requester |
| P47-6 | Churn-prone named entities | organizations, people-in-role, live services (fast-changing class per R03 §2.7 freshness typology) |
| P47-7 | Temporal cues | "current", "latest", "today", "now", "recent", "this year" |
| P47-8 | Locality cues | "near me"; regional availability, shipping, or pricing |
| P47-9 | Explicit lookup requests | "look up", "check online", "verify", "find out" |
| P47-10 | Prior art & external corpora | "existing solutions", "competitors", "state of the art" |
| P47-11 | Requester-asserted external facts | the request states a checkable claim about the world that the plan depends on → verify node at standard tier and above (false-premise guard) |

Semantics of a hit:
- The **data-bearing flag** is set on the intake record; it excludes the task from the zero-interaction band (S06.4) and obliges research nodes in the PLAN (S06.6).
- A hit is dismissible **only by the requester** explicitly supplying or owning the fact, recorded on the SPEC as a requester-supplied input — never by a model [1.9; R03 §5 "model-initiative research" rejected].
- Maintenance: v0 additions are manual operator edits to the rule file; miss-driven auto-proposals enter through the 8.1 lesson gate only when the proposal pipeline activates at v1 [G2 D2.7]. Classifier evaluation belongs to the eval practice [XREF:S14].

### S06.4 Stakes tiers and the zero-interaction band

Planning effort scales with stakes automatically, regardless of the requester's convenience settings (1.8). Four tiers; automatic adjustments are monotone upward within a task (TIER-UP, floors, size-delta); lowering happens only by explicit requester action and never below a floor [R03 §4].

| Tier | Interview | Critique pass | Approval |
|---|---|---|---|
| **Trivial** (= the zero-interaction band) | none — unresolved slots become auto-listed assumptions | none | no blocking gate; completion card only |
| **Low** | only while Clearance < ⚙ `intake.clearance_floor.low` | none — spine only `[coordinator-draft]` (R03 §4 Stage 3 mandates critique from standard up) | one-tap card; inbox risk tier Low, batchable [S3.2] |
| **Standard** | while Clearance < ⚙ `intake.clearance_floor.standard` | mandatory, one pass | full two-layer card; risk tier Medium [S3.2] |
| **High** | while Clearance < ⚙ `intake.clearance_floor.high`; uncapped, batched | mandatory, one pass | full card; risk tier High — never batchable, individually approved [S3.2] |

**The zero-interaction band** [G1 P11]: a task is Trivial if and only if all four conditions hold —

1. **read-only** — the plan's declared write-set outside the run workspace is empty and no outward-effect proposals are expected;
2. **no data-bearing flag** (S06.3);
3. **estimated cost** < ⚙ `intake.zero_interaction_cost_usd` (API-equivalent, D5 currency) [G1 D1.7];
4. **no new tools, workers, grants, or automations** — existing validated workers with existing grants only [XREF:S08; G3 D3.4 boundary].

Conditions are evaluated conservatively at Stage 0 and re-verified deterministically at Stage 2 from the PLAN's actual contents; failing any re-check ejects the task from the band, and band exit is one-way for that task [R03 §4 Stage 2(b)]. Inside the band the pipeline still runs — auto-generated SPEC (restatement + ACs + listed assumptions) and a degenerate PLAN exist in the ledger, ungated — and completion delivers one non-blocking card carrying restatement, assumptions, deliverable, and receipt. Trivial tasks never require a confirmation click [1.8, 2.5]. The band cannot leak: membership is rule-decided, floors override upward, and D7 gates any outward effect even on a floor miss [R03 §2.6, §4 Stage 0].

### S06.5 Stage 1 — the clarity-driven interview and the Clearance indicator

The platform refuses to run on vagueness (1.2): below its tier's Clearance floor a task is either interviewed or explicitly force-proceeded by the requester with the vagueness converted into visible, approval-gated assumptions — never silently executed.

- **Question sets are per kind-of-task** (1.2): each family carries a versioned must-know taxonomy (software seed: the ClarifyCodeBench 10-type taxonomy; the generic fallback set covers unmatched requests) [R03 §2.3, §4 Stage 1]. Taxonomies are knowledge objects: drafted at implementation time with the strongest available frontier model, entering through the 8.3 knowledge gate — versioned, attributable, removable [5.7 pattern; XREF:S09]. TBD-P3(seed interview-taxonomy drafting session, 8.3-gated).
- **Clearance (0–100%)** is computed deterministically from the active question set: each must-know slot carries a weight (shipped in the taxonomy file, operator-editable); a slot is *resolved* when registry-supplied, requester-answered, or converted to an explicit assumption; Clearance = 100 × resolved weight / total weight [G1 P8]. It is displayed live during intake, rises as the requester answers, and appears on the approval card.
- **There are no fixed question caps, per tier or otherwise** [G1 P8 — fixed per-tier caps rejected]. The interview continues while Clearance is below the tier floor and unresolved slots remain; it stops when the floor is reached, slots are exhausted, or the requester force-proceeds. Force-proceeding converts every unresolved must-know slot into a listed assumption that lands on the approval card's centerpiece (S06.9).
- **Question delivery**: batched option cards — up to 4 questions per card, each with 2–4 labeled options plus free text — highest-weight unresolved slots first [R03 §2.7, §4 Stage 1; card component and transport XREF:S15, XREF:S03]. Unanswered interview cards follow the platform ask SLAs and expiry [G1 Def.3 superseded set; G2 Def.2; XREF:S02].
- **Ask-don't-assume (1.7) as one-question escalations**: outside the interview — during planning, critique, or execution — a *consequential* ambiguity raises exactly one clarifying question as a single-question card; the run blocks-not-fails and resumes on answer (4.3). Consequential = would change an AC, the declared write-set, an outward effect, the stakes tier, or the cost estimate beyond the S06.7 size-delta factor. Non-consequential ambiguities become logged assumptions, visible on the card and in the ledger [R03 §4 Stage 1].
- The interview runs inside the Stage-1 planning session on the planning model; the utility model phrases and summarizes but does not decide what must be asked — the taxonomy carries the coverage [R03 §2.3; G3 T15 digest].

### S06.6 The artifact pair: SPEC and PLAN

Stage 1 emits one artifact pair — Sinet-owned markdown + YAML frontmatter (ids, owner, stakes tier, model/version provenance, status), stable keys, git-committable [D9], checkpointed at every stage boundary [D7], referenced from the ledger [XREF:S05; R03 §4 "Artifact format"].

**SPEC — the contract all later work is measured against (1.3):**
- Plain-language goal restatement in the requester's terms.
- **Numbered acceptance criteria `AC-1..n`** on stable keys: atomic (one behavior each), binary-decidable, measurable, free of vague adjectives [R03 §2.2 anti-ambiguity idioms, §4].
- **Dual phrasing** [G1 P10]: every AC has a plain-language line, always; a structured machine-checkable sub-line (EARS phrasing for behavioral criteria, GWT example scenarios where examples communicate better) exists **only where the criterion is machine-checkable**. Where a structured sub-line exists, verification binds to it; otherwise verification judges the plain line [XREF:S07]. The requester is never required to read notation.
- Technology-agnostic outcome criteria (SC-style) so a non-expert can recognize their goal [R03 §4].
- An explicit **assumptions list**; an explicit **out-of-scope / "will NOT do" list**; requester-supplied inputs (S06.3).
- Unresolved points carry `NEEDS-CLARIFICATION` markers; **an artifact with open markers cannot reach approval** — each marker is either asked (S06.5) or converted to a listed assumption [R03 §4 Stage 1].

**PLAN:**
- Numbered steps, each with a per-step **"Done when"** stage contract, consumed by verification [R03 §4; harvest S3 pattern; XREF:S07].
- **Per-step approach [A15, 2026-08-27]:** every step also states, in plain words at non-IT reading level, HOW it will be built — the chosen method, the material decisions the planner made inside the step with the alternatives it considered and why the chosen one won, and, where the ordering is load-bearing, why this step sits where it does. A step that names only its outcome cannot be judged for approach, which defeats the 1.6 cheap preview — catching a wrong approach BEFORE hours burn is that preview's entire point [1.6; operator record `P3/design/b6-gate-operator-findings-r4-2026-08-23.md` §F3]. The approach text is requester-facing plan content rendered under the step with the assumptions treatment (S06.9 Layer 2); verification continues to bind to the ACs and the "Done when" contracts — the approach informs the human judgment at Stage 4, it is not a new machine-check target.
- The **AC coverage map**: every `AC-k` → owning step id(s). This is the 1.4 traceability substrate (S06.7).
- **Research nodes** for every data-bearing flag — present by policy before any model drafted the plan, never left to model initiative [1.9/P47; R03 §4]. At v0 research nodes execute via provider-side search tools on lanes that offer them — task sandboxes get no egress (C3 confined-fetch arrives v0.1 [XREF:S11; XREF:S19]); routing to a search-capable lane is S08's [XREF:S08]; the runtime did-research-actually-run check via usage counters, with retry-then-escalate, is verification's [R03 §4 Stage 2(d); XREF:S07].
- **Confinement declaration**: each step/worker declares its confinement class (C0–C2 at v0) [XREF:S11]. **The class declared at plan time flows to every helper**: the planning stage itself runs at C1 with a read-only project snapshot, enforced at sandbox level — never prompt level — and inherited by any helper the planner spawns; at execution every helper inherits its declaring step's class or tighter, never wider. Widening confinement is a plan change: delta re-plan + re-approval [P-T05-1; R03 §2.1 (prompt-level lockouts demonstrably fail); XREF:S11 mechanics, XREF:S04 spawn mechanics].
- **Write-set declaration** per step; a plan that cannot bound its write-set takes the whole-project claim [G2 Def.3; claim/parallelism mechanics XREF:S02].
- Risk notes, and a size/cost estimate explicitly compared to the Stage-0 guess (2.5) — surfaced, never silent [R03 §4].
- The plan declares *requirements* (class, tools, family); selecting which worker, model, and lane executes it is S08's [XREF:S08].

### S06.7 Stage 2 — the deterministic spine (platform code, no model)

Runs after every Stage-1 emission and after every revision [R03 §4 Stage 2]:

(a) **Coverage check (1.4).** Keyed AC↔step cross-reference on stable keys. Any uncovered AC triggers at most ⚙ `intake.coverage_autofix_rounds` bounded auto-fix round(s) with the planner, then a decision card to the requester — an agreed criterion never disappears silently, at intake or ever after: any later AC removal is a REMOVED delta requiring re-approval (S06.9). Drop-detection is deterministic on keys; a local-model semantic spot-check ("does step S plausibly deliver AC-3?") runs advisory-only [R03 §4, §5 (LLM-only coverage rejected)]. This escalation route is covered by an end-to-end test that forces it [P46; 5.6].
(b) **Floor re-check against the plan's contents.** If the PLAN introduces outward effects, spend, credential touches, shared-asset writes, or new tools/workers the request didn't mention, the tier rises now, and zero-interaction band membership is re-decided (S06.4) [R03 §4 Stage 2(b)].
(c) **Size-delta rule (2.5).** Plan-derived estimate > Stage-0 guess × ⚙ `intake.size_recheck_factor` → reclassify size, re-run tier assignment, and upgrade ceremony *before* expensive work; plan ≪ guess → noted on the approval card. Display is stakes-gated: non-trivial tasks show the classification on the card for verify-or-change; trivial tasks never require a confirmation click [2.5; 1.8].
(d) **Research-node presence check** for every data-bearing flag; a missing node is a defect that bounces the plan once, then cards [R03 §4 Stage 2(d)].

### S06.8 Stage 3 — plan self-attack (1.5)

A separate fresh-context session that reads **only the artifact pair** — never the interview transcript; context separation is the measured active ingredient and keeps the pass at artifact-sized cost [R03 §2.5, §4 Stage 3]. Explicit devil's-advocate role, per-family rubric, premortem frame: "assume this failed — why?" [1.5; R03 §2.5]. Runs at planning-model class; **local models never hold this duty** — small-model critique capitulates [R03 §2.5, §5; G3 T15 digest]. Mandatory from the standard tier up (S06.4).

Four terminal verdicts, each written to the ledger:
- **PASS** → Stage 4.
- **REVISE** — numbered blocker findings; the planner fixes within ⚙ `intake.critique_revise_rounds` round(s); re-critique checks only the named findings; criteria do not drift. One pass, never loops — critique is a filter, not a guarantee [R03 §4 Stage 3 calibration honesty].
- **SPEC-DOUBT** — the P45 antidote: the critique concludes the *specification itself* is a bad idea and says why in plain language → a mandatory decision card to the requester (**proceed anyway / adjust spec / rethink**). Never absorbed, never softened into a note; the route is covered by an end-to-end test that forces it [1.5; P45/P46; 5.6; R03 §2.5 (no product wires this — Sinet-built), §4 Stage 3].
- **TIER-UP** — the plan reveals stakes the classifier missed; the tier rises and the pipeline re-enters at the new tier's requirements [R03 §4 Stage 3].

### S06.9 Stage 4 — approval, freshness, and deltas (1.6)

The cheap preview before expensive execution: reviewing a plan costs seconds; a wrong execution burns hours of quota (1.6). The requester approves their own spec and plan [D10]; nothing self-approves (5.8).

**Card content contract** (rendering XREF:S15; inbox mechanics S3.2):
- **Layer 1 — one phone screen**: what I understood (restatement); what you'll get; what I'll do (numbered plain steps); **what I will NOT do**; **the assumptions list as the centerpiece** — the one RCT-evidenced overreliance mitigation; no additional forcing functions are stacked (stacking measured detrimental) [R03 §2.4]; what could go wrong + expected cost/time + Clearance + size class; and the 13.5 help block: what this decision does, what could go wrong, what the platform recommends and why — drafted by the utility model from the artifacts at non-IT reading level [13.5; R03 §2.7 (white space — Sinet-built)].
- **Layer 2 — expandable**: full numbered ACs (both phrasings), plan with coverage map and each step's approach rendered under it with the assumptions treatment [A15], critique verdict and findings, research findings, estimate detail.
- **Actions**: **Approve** · **Re-plan** — structured entry: tap the AC, assumption, or step being contested; produces a bounded delta re-plan [S3.2: plans always carry both Approve and Re-plan] · **Re-interview** — returns to Stage 1 with artifacts intact. Cancel is always available (4.5).
- Approval UX is tuned by measured outcomes through the benchmark/eval practice, never by user preference [R03 §5; XREF:S14].

**On Approve**: the ACs freeze under their numbers — later verification and re-review judge against exactly these [5.4; XREF:S07]; objective, frozen ACs, and constraints/danger zones are pinned into the ledger [G1 Def.12; XREF:S05]; the spec+plan version joins the freshness fingerprint [G1 Def.5; XREF:S02].

**Freshness of pending approvals**: a card pending past ⚙ `intake.approval_stale_hours`, or whose freshness fingerprint drifts (repo HEAD, source hashes, spec+plan version, price table), or whose project accepted a sibling task, is auto-flagged **"assumptions may be stale"** with a one-click re-plan [S3.2; 4.3; G1 Def.5; fingerprint mechanics XREF:S02]. The flag never blocks the requester from approving; it makes staleness visible. Answering resumes the pipeline in place (4.3); unanswered asks expire per platform policy [G2 Def.2; XREF:S02].

**Delta re-approval**: every post-approval change to SPEC or PLAN — freshness re-validation findings (4.3), sibling-collision re-plans (S1.11), contested-card fixes, confinement widening — is expressed as an ADDED / MODIFIED / REMOVED delta against the frozen artifacts and approved on a **delta-only card** showing exactly what changed; a silently disappearing criterion is structurally impossible [1.4; R03 §2.2 (OpenSpec pattern), §4 Stage 4]. Delta cards carry rubber-stamp risk with no field evidence either way (P-T05-2), so each one ships the **measurement hook**: the event log records presented-delta size, time-to-decision, decision, and outcome linkage; the eval practice owns the analysis, and measured rubber-stamping proposes a card-format retune [P-T05-2; XREF:S14].

### S06.10 Ceremony engines and billing (1.10)

Ceremony has a designated engine per duty. 1.10's designated-engine requirement is satisfied by a per-person **ceremony duty map** with a uniform recommended default and per-user override — best result per unit of allowance, not merely cheapest [1.10]. The v0 cut line keeps interviewing, plan critique, and verification review on paid frontier-class models; light duties ride the local free tier [G1 P9; G3 T15 digest; R03 §2.6].

New term — **planning model**: the requester's designated frontier-class model for the Stage-1 planning session and the Stage-3 critique pass; recommended platform-wide like the utility model, user-overridable [1.10; R03 §4 Stage 1]. Distinct from execution routing [XREF:S08].

| Duty | Engine | Provenance |
|---|---|---|
| Family / stakes / size classification; data-bearing classifier | local duty aliases (free tier) [XREF:S12] | R03 §2.6; G1 P9 |
| Interview conduct, restatement, SPEC/PLAN drafting | planning model | R03 §4 Stage 1; G3 T15 digest |
| Question/card phrasing, 13.5 help drafting, summaries | utility model (local, ≤8B at v0) | 1.10; G1 P9 |
| Coverage / floor / size / research-presence checks | platform code — no model | R03 §4 Stage 2 |
| Advisory semantic coverage spot-check | local duty alias | R03 §4 Stage 2(a) |
| Critique / premortem / SPEC-DOUBT | planning-model class, fresh context — never local | R03 §2.5; G3 T15 digest |

Every ceremony consumption is billed to the requester and itemized separately from execution on the receipt [1.10; 3.4; 3.6; XREF:S10]. Local-tier duties consume no allowance and appear as zero-allowance receipt lines [Operating reality; XREF:S12]. The structural cost property: at most two heavyweight paid passes per intake (planning session + critique) plus bounded single revisions; everything else is local or deterministic [R03 §4 "Ceremony economics"] — which is what lets the 11.2 benchmark price intake honestly against direct use [XREF:S14].

**Settings introduced (⚙):**

| Name | Default | Clamp/range | Ratified by |
|---|---|---|---|
| `intake.zero_interaction_cost_usd` | 0.50 | ≥ 0; per-user | G1 P11 + D1.7 |
| `intake.clearance_floor.low` | 60 | 0–100 | G1 P8 mechanism; default `[coordinator-draft]` |
| `intake.clearance_floor.standard` | 75 | 0–100 | G1 P8 mechanism; default `[coordinator-draft]` |
| `intake.clearance_floor.high` | 90 | 0–100 | G1 P8 mechanism; default `[coordinator-draft]` |
| `intake.size_recheck_factor` | 2.0 | > 1.0 | 2.5; R03 §4 Stage 2(c); default `[coordinator-draft]` |
| `intake.approval_stale_hours` | 24 | > 0 | 4.3 default; G1 Def.5 |
| `intake.coverage_autofix_rounds` | 1 | 0–2 | R03 §4 Stage 2(a) ("bounded") |
| `intake.critique_revise_rounds` | 1 | 0–2 | R03 §4 Stage 3 ("fixes once") |

All per G1 rider 1: operator-editable, audit-trailed, auto-adjustment only within operator ceilings, visible on receipts.

**Known problems owned here:**
- **P-T05-1** — plan-stage confinement must bind helpers: dispositioned in S06.6 (sandbox-level read-only planning inherited by planning helpers; declared class flows to every execution helper, tighter-only); enforcement mechanics [XREF:S11].
- **P-T05-2** — delta re-approval rubber-stamp risk: dispositioned in S06.9 (delta-only cards + mandatory measurement hook); analysis owned by the eval practice [XREF:S14].
- **P-T05-3** — clarification as an injection surface: v0 intake surface is the authenticated workspace only; quoted/pasted material in requests is treated as data, never instructions (4.7); full C3-class treatment of channel-ingested content parks with 15.5 multi-channel ingress, jointly owned with [XREF:S11].
- **P-T05-4** — no published stakes-classification accuracy: tolerated by design (deterministic floors + cheap wrongness, S06.2); a pre-registered classifier eval is required before v1 household use [XREF:S14].

**Deferred / parked:**
- High-stakes comprehension friction (embedded-real-choice card, R03-OQ6) → operator decision at the v1 household pilot [G1 follow-ups].
- Channel-ingress intake hardening (P-T05-3 full treatment) → 15.5 multi-channel ingress.
- Per-family question sets beyond software + generic → arrive with each domain (research at v0.1; degraded domains at v1).
- Miss-driven auto-proposal of P47 rule additions → v1 memory proposal pipeline [G2 D2.7]; v0 is manual operator edits.
- Structured sub-line format winner (EARS vs GWT) → adopt if a real format A/B appears [R03 §4 change-trigger 2]; both admissible at v0 under G1 P10.
- Local-model critique duty → re-test when a T15-class eval shows non-capitulating critique in the 24 GB class [R03 §4 change-trigger 3].
- Zero-interaction band widening → 11.2 evidence that intake overhead goes unpaid on Eco-tier tasks [R03 §4 change-trigger 5].

**Coverage:**

| Feature-list item | Subsection |
|---|---|
| 1.1 natural-language intake | S06.1, S06.5 |
| 1.2 refuses vagueness / kind-of-task questions | S06.5 |
| 1.3 restate-confirm, numbered-AC contract | S06.6, S06.9 |
| 1.4 criterion traceability, no silent drops | S06.6, S06.7(a), S06.9 (deltas) |
| 1.5 plan self-attack + spec-is-bad escalation | S06.8 |
| 1.6 cheap preview | S06.9 |
| 1.7 ask-don't-assume | S06.5 |
| 1.8 stakes-scaled effort, zero-interaction band | S06.4 |
| 1.9 mandatory live research (P47) | S06.3, S06.6 |
| 1.10 ceremony engine + billing | S06.10 |
| 2.3 task-type detection | S06.2 |
| 2.5 size-guess self-correction | S06.2, S06.7(c), S06.9 |
| 4.3 (approval-freshness slice) | S06.9 |
| S1.6 (registry-fed intake slice) | S06.2 |
| S3.2 (plan-card slice: Approve+Re-plan, stale flag) | S06.9 |
| 13.5 (card content contract; rendering S15) | S06.9 |

**Open items for G4:** none. Coordinator-drafted sub-choices are tagged inline for G4 attention: the P47 trigger table (S06.3), Clearance floor defaults, the low-tier critique skip (S06.4), and the size-recheck factor.
