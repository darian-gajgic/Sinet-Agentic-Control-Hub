## S07 — Verification & quality

**Scope:** Everything between "execution produced a deliverable revision" and "the requester sees it": the two-axis, four-layer verification pipeline on every deliverable, judge routing and independence, bounded rework with carried findings, typed and test-proven escalation with the ratified SLA set, the verification-practice failure modes as first-class problems, and rubric/judge governance.
**Binding inputs:** R04 (whole report; §4 is the ratified pipeline per G1 D1.1(d)) · G1 D1.1/D1.2, P10, Def.1, Def.2, Def.3 as superseded at close, rider 1; D1.3 + Def.9 as boundaries · G2 D2.1(d) (event-type contract), Def.2, Def.11 · G3 Def.3 (boundary), Def.4, Def.8, T15 digest (ceremony cut line) · feature list 5.1–5.8; slices of 1.9, 2.1/7.6, 3.4/3.6, 4.7/S5.4/S5.6, S2.3, 11.2 · siblings: S04 (acceptance-screen boundary), S05 (clean-context exception), S06 (frozen ACs, dual phrasing, research nodes, SPEC-DOUBT) · `Spec/benchmark-preregistration-v1.md` (cited, never restated).

### S07.1 Shape: two axes, four V-layers, every deliverable

**Nothing is delivered unverified (5.1) — 100% of deliverables, never sampled.** Industry samples its quality gates for volume economics; Sinet's delivery gate covers every deliverable, and sampling exists only inside the meta-eval and benchmark practice (11.2) [R04 §2.9; XREF:S14].

Verification asks **two independent questions** of every deliverable [5.2; G1 D1.1(d)]:

- **Axis 1 — spec compliance:** does it meet the frozen numbered ACs (S06.6)? Judged criterion by criterion, binding to the structured sub-line where one exists [G1 P10].
- **Axis 2 — outcome sanity:** is it actually good? A separately prompted judgment (S07.5) — formal compliance and a bad product can coexist, and an axis-2 finding can reopen the specification itself as an explicit human decision (5.2), never as a model's edit.

New term — **V-layer**: one of the four verification layers below ("V" prefix avoids collision with the memory layers L0–L2 [S00.7]). The cascade is cheap-first (5.3): each layer is strong exactly where the next is weak, and nothing reaches paid judging that a free check could kill [R04 §2.3, §4].

| V-layer | What runs | Runs on | Allowance cost |
|---|---|---|---|
| **V0** pre-gates | deterministic structural checks | platform code | zero |
| **V1** domain checks | executable/mechanical checks per domain + plan-conformance checks | local free tier, task sandbox | zero |
| **V2** judge pass | one dual-axis model review | paid flat-rate lane, planning-model class (S07.5) | the only scarce verification spend |
| **V3** human gate | the requester accepts or rejects (5.8) | human, via the approval inbox | — |

Effort scales with survivors and stakes (5.3): V0/V1 kill broken output before any paid call; axis 2 is stakes-gated outside launch domains (S07.8); entailment coverage splits mandatory/sampled (S07.4); rework is round-capped (S07.6). **The quality gate is never the effects gate:** sequence is quality verdict (V0–V2) → human accept (V3) → gated effect execution under D7 — verdicts release nothing outward, and the judge is never an implicit release authority [R04 §6-G1; D7; D10].

**Boundary lines.** S04 owns the helper acceptance screen — form-conformance on helper reports, never content truth [S04.5; G1 Def.9; G3 Def.3]; helper-derived content is verified here at deliverable level. S06 owns intake-side critique — SPEC-DOUBT is the pre-execution spec challenge; REOPEN-SPEC (S07.5) is its deliverable-time counterpart. S13 owns deliverable/revision/comment mechanics including anchor storage and re-anchoring. S14 owns eval/benchmark machinery, watchdogs, and canary scheduling infrastructure. S08 owns worker/judge model selection mechanics. S11 owns the sandbox mechanics verification relies on.

### S07.2 V0 — deterministic pre-gates (free, every domain, always)

Empty, truncated, placeholder-marker, diff-empty, and schema-invalid output dies instantly at zero cost, plus artifact-shape checks per deliverable type (5.3). This layer is load-bearing, not hygiene: superficial/truncated/placeholder output is precisely the input class model judges most reliably rubber-stamp (master-key false-positive rates to 80%), so the free gate removes exactly what the paid gate cannot be trusted with [R04 §2.1, §2.3, §4].

- A V0 **verdict** of "artifact malformed" is a hard kill: it costs one regeneration, never a judge call.
- A V0 **tool failure** (the gate crashed, not the artifact) fails open to V1 with a logged incident: a screen never blocks on its own failure, a screen outage escalates rather than approves, and no screen's SHIP is ever final [R04 §2.3, §5].

### S07.3 V1 — domain check packs and plan-conformance checks

**Code (v0 launch domain).** The executable ladder, run per revision in the task sandbox with evidence artifacts retained: lint + typecheck + build → unit/integration tests → runtime smoke → drive-the-feature e2e [5.1; R04 §2.4, §4]. Hardened execution rules, each a **MUST** [R04 §4]:

1. **Network off by default; answer-bearing VCS history stripped** from the verification workspace (P-T06-2) — otherwise the pass signal measures lookup ability, not work (63% of one frontier model's benchmark successes were retrieved, not derived [R04 §2.4]). Confinement mechanics [XREF:S11].
2. **Tests are read-only to the executor** — direct test editing is the dominant measured cheat class for Claude-family models [R04 §2.4].
3. **The pass/fail verdict is computed outside every agent-accessible environment** — in-band success signals (exit codes from inside the agent's process) are never trusted [R04 §2.4].
4. **Test provenance:** acceptance tests originate from the frozen ACs — from the structured sub-line where one exists [G1 P10; S06.6] — and are written in a separate context (different session or model) from the implementation; doer-written tests pass by construction [R04 §2.4].
5. **Sanctioned challenge instead of edit access:** the executor gets an explicit "these tests look wrong" escalation (→ CHECK-INTEGRITY card, S07.7) — a measured anti-hacking device, while prompt text is never the load-bearing defense; structure is [R04 §2.4].
6. **Flaky checks are quarantined** with an owner and a fix-by date, never retried-until-green [R04 §2.4].
7. Every check suite carries a **`verified-on` stamp** and a periodic planted-defect audit (P-T06-1, S07.9).

**Web research (specified now; idles until the domain arrives at v0.1 [XREF:S19]).** Mechanical citation pass — extract claim–URL pairs, fetch-and-check with fabricated-vs-rotted disambiguation, staleness stamps against the task's recency requirement — run as a **platform check under its own egress-allowlisted confinement** (the executor's verification sandbox stays network-off; C3 arrives with the domain [XREF:S11]). Link validity is never treated as quality: >94% link validity coexists with 39% claim accuracy — entailment (S07.4) is the load-bearing check [R04 §2.5, §4]. The research rubric names which failure rate each check measures (fabricated-URL vs claim-support vs response-level) [R04 §2.5].

**Stage contracts.** Every PLAN step's "Done when" line (S06.6) is checked at its stage boundary; contract states are PASS / FAIL / **N-A** / **UNVERIFIABLE-HERE**, with first-upstream-failure attribution. A stage contract is incomplete unless it declares its finding categories and their escalation routes — definition-of-done includes definition-of-cannot-be-done-here [R04 §6-S3].

**Did-research-actually-run (1.9).** For every research node in the PLAN (S06.6), a deterministic post-step check reads the metering ledger's per-step usage counters [XREF:S10] and requires ≥1 search/research-tool invocation. Zero invocations → the node re-runs once in a fresh session (`⚙ verification.research_rerun_limit = 1`), then a RESEARCH-NOT-RUN decision card — correctness on world-facts is never left to model initiative or memory [1.9; S06.6 hand-off].

### S07.4 Entailment coverage for research-domain claims

Claim–citation entailment coverage is **mandatory for load-bearing claims and sampled for the rest** (`⚙ verification.entailment_sample_rate`) [G1 Def.2]. Claims are judged against the **fetched source content**, never against the deliverable's own citation text — judges are flippable by fabricated authority [R04 §2.1, §2.5]. Entailment is a narrow task validated at expert grade (judge agreement with physician consensus exceeded inter-physician agreement) and runs on the local tier [R04 §2.5]: default seat Granite-Guardian-8B, CPU floor Flan-T5-0.8B for sampled checks [G3 Def.4; seat mechanics XREF:S12].

Thresholds and the mandatory-coverage bar (which claims count as load-bearing) are TBD-BRINGUP(entailment calibration set — planted supported/unsupported pairs + first real outcomes; pre-registered TPR/TNR bar before entailment gates unsupervised) [G3 Def.4, Def.8]. **A8 (2026-07-23, B4 gate):** the pre-registered bar is now MEASURED — Granite Guardian 4.1 on the 156-pair Sinet set gives per-side TPR 0.949 / TNR 1.000: the ≥0.90 MAIN bar is met, the load-bearing 0.95 sub-bar is NOT (entailed side 0.949), so entailment stays conservative and idle; ⚙ `verification.entailment_sample_rate` DERIVED 0.20, live write pending at bring-up (result file `P3/measurements/2026-07-22-entailment-thresholds.md`). **v0 status:** the launch domain is software, so this machinery is specified and ships idle; it activates with the web-research domain at v0.1 [G1 D1.2; XREF:S19].

### S07.5 V2 — the dual-axis judge pass

**Judge engine and routing.** The V2 judge runs at **planning-model class on a paid flat-rate lane**: G1 Def.1 named the utility model as default judge before the G3 ceremony cut line measured ≤8B-class generalists judging below random on hard pairs; the cut line governs the engine class, and Def.1's pairing and flagging rules apply to it unchanged [G1 Def.1; G3 T15 digest; S06.10]. The pairing rule [G1 Def.1]:

- Default judge = the requester's designated verification-review engine (recommended platform-wide, user-overridable, per 1.10).
- When the judge's model family equals the executor's and another flat-rate lane is connected, D5 consumption-pressure routing MAY swap the judge to the dissimilar lane — same-family judges inflate own-style output through familiarity, and model errors grow more correlated as capability rises [R04 §2.1, §2.3].
- **Self-family judging is always flagged on the receipt** [G1 Def.1]. Selection mechanics are S08's [XREF:S08].

**Independence.** The judge is a fresh, clean-context platform session — never the executor's session, never a continuation of its context [G1 D1.1]. It receives the frozen ACs and the deliverable diff and never the executor's overlay, history, or `learned_this_task` [XREF:S05 clean-context exception]. Judgment-shaped helper work inside a task uses the same clean-context rule [S04.2]; the deliverable gate specified here is always a platform stage, not a helper.

**Judge input slice** (the whole world the judge sees): the artifact + its diff against the previous revision [XREF:S13] + the frozen ACs + the rubric version + V1 check outcomes as evidence + prior-round findings — **never the execution transcript** (transcript re-sends are the measured O(M²) prefill trap, and the transcript is the executor's frame) [R04 §2.9, §4]. Trajectory/transcript audit exists only as a special-purpose side-effect audit, never the default [R04 §2.9].

**Axis 1 — spec compliance.** One binary verdict per numbered AC with a mandatory extractive evidence quote and an Unknown escape [R04 §2.1, §3-B]. Where a structured sub-line exists, the verdict binds to it; otherwise it judges the plain line [G1 P10; S06.6]. For sub-lines already executed at V1, the judge consumes the V1 outcome as evidence and does not re-decide the mechanical fact; a judge–check disagreement is a CHECK-INTEGRITY finding, not an override `[coordinator-draft]`. Findings are emitted as numbered **`[F1..Fn]`**, each anchored to the exact place it concerns (file:line / section / claim id; anchor mechanics [XREF:S13]) and tagged **blocker | note** [5.5; R04 §4]. Every blocker MUST cite the frozen criterion (or axis-2 rubric item) it violates; a finding that cites none can only be a note or a REOPEN-SPEC matter — that is what keeps goalposts structurally fixed (5.4).

**Axis 2 — outcome sanity (the P45 axis).** Separately prompted with its own mini-rubric, never folded into the compliance pass — models spontaneously notice what they weren't asked about ≈28% of the time, so an unprompted second axis does not exist [R04 §2.2, §3-C]. Its four probes: the reasonable-user test ("would a reasonable user consider this done and good?"), the implicit-expectations scan ("what would a well-informed person expect that is absent?"), the side-effect scan (unrequested changes are failures, not bonuses), and the expert-standard comparison [R04 §3-C].

**Verdicts:** **SHIP / SHIP-with-notes / REVISE (blockers only) / ESCALATE**, plus axis 2's unique power **REOPEN-SPEC** [R04 §3-B, §4]. REOPEN-SPEC files a decision card to the requester — proceed as specified / adjust the specification / rethink — and can never change the spec itself (5.2; D10). An accepted adjustment lands as an S06.9 ADDED/MODIFIED/REMOVED delta and re-freezes the ACs; later rounds judge against the amended frozen set. The only sanctioned way criteria ever change is through that human-approved delta — never through the judge.

Both axes fit one prompted call each; both are ceremony-billed to the requester and itemized (S07.11).

### S07.6 Bounded rework (5.4/5.5)

- **Only blockers trigger another round.** Notes ride along to the requester as review comments [XREF:S13] and never spin loops (5.4).
- **The retry package** is exactly: the original SPEC + the frozen ACs + the numbered findings `[F1..Fn]` with their anchors + the deliverable. Findings are carried verbatim as numbered points; the retry fixes named problems and never regenerates blind — intrinsic self-correction degrades strong models, and changes to already-correct work are ~always harmful [R04 §2.6; 5.5]. Requester comments (6.2) enter the retry through the same numbered-anchored-point channel [XREF:S13].
- Each round's executor is a **fresh session** built from the retry package (fork-don't-poison, consistent with the S04.5 helper-retry rule and fresh-context-per-stage [G1 D1.1]).
- **Re-review judges against the ORIGINAL criteria:** same frozen ACs, same rubric version, prior findings in scope. After round 1, new note-class findings are suppressed to a count (goalpost-drift suppression); new blockers are admissible only citing the same frozen criteria [5.4; R04 §2.6].
- New term — **finding key**: the stable identity of a finding (criterion + anchor + failure class). Recurrence of a finding key across rounds is the convergence signal.
- **Stop rules:** hard cap `⚙ verification.rework_rounds = 3`; convergence = the same finding keys recur unresolved OR artifact similarity holds for `⚙ verification.convergence_patience_rounds = 2` consecutive rounds ("model inertia" — cosmetic edits in response to findings) [R04 §2.6, §3-D]. No judge-every-round continuation gating: the smart stopper costs 2.3× tokens for zero quality gain [R04 §2.6, §5].
- **Cap-hit or convergence without SHIP → ESCALATE decision card** carrying best-effort state and the full round history — never a silent stop, never an error [R04 §3-D]. Every round's consumption bills to the requester, itemized per round (3.4; S07.11).

### S07.7 Escalation: typed routes, proven live (5.6, P46)

**Every verification finding terminates in a human-visible sink — a decision card, an operator alert, or a worker-flaw ticket. No other sink type exists**, so "a finding that died in a log" is structurally impossible — and the claim is proven by tests, not assumed (5.6; P46) [R04 §4]. Escalations enter the risk-ranked, batchable approval inbox (S3.2) as structured, resumable asks; answering resumes the pipeline in place (4.3). Queue curation is a verification requirement, not UX polish: past reviewer capacity, additional escalations measurably deplete attention and the human gate rots [R04 §2.7].

**Route table** (categories and routes; the enum is `[coordinator-draft]`, the sink-types-only rule is ratified [R04 §3-E, §4]):

| Category | Raised by | Route | SLA class |
|---|---|---|---|
| AC-BLOCKER | axis 1 | rework round (S07.6); at cap → requester decision card | approval |
| SANITY-BLOCKER | axis 2, short of spec doubt | rework round; at cap → requester decision card | approval |
| REOPEN-SPEC | axis 2 | requester decision card → S06.9 delta on accept (S07.5) | approval |
| CHECK-INTEGRITY | executor test-challenge; judge–check disagreement; flake quarantine; failed suite audit | requester decision card + suite quarantined pending fix | approval |
| RESEARCH-NOT-RUN | S07.3 counter check, after its retry | requester decision card | approval |
| CAP-HIT | S07.6 stop rules | requester decision card + best-effort state | approval |
| WORKER-FLAW | recurring defect pattern attributable to a worker | worker-flaw ticket (7.2/7.3) [XREF:S08] | approval |
| SAFETY | watchdog pause-and-flag [G1 D1.3; XREF:S14]; confinement violation [XREF:S11] | operator alert | safety |

**The ratified SLA set** [G1 Def.3 as superseded at close]: approval-class cards remind at `⚙ verification.card_remind_hours = 4` and push-notify at `⚙ verification.card_push_hours = 24`; safety-class items push **immediately** and re-ping at `⚙ verification.safety_reping_hours = 1`. Unanswered asks expire per platform policy [G2 Def.2; XREF:S02]. These are inbox-wide numbers ratified at G1 close, registered here and consumed by the inbox surface [XREF:S15]; the S18 sweep reconciles naming.

**Liveness is proven three ways, all mandatory** [R04 §2.7, §4; P-T06-4]:

1. **Planted-defect e2e test class.** For every route-table category, a synthetic task plants one defect of that category and the test asserts a human-visible card/alert reaches the inbox within its SLA. Standing conformance-suite entries, run in CI and on schedule [XREF:S14] — the same prove-by-violation pattern as S04.5.
2. **Dead-man canary, daily.** A scheduled trivial always-escalating synthetic task runs the real pipeline end-to-end on the local tier (costs no allowance) at `⚙ verification.canary_interval_hours = 24`; a watchdog **external to the pipeline** alerts the operator when the canary's card stops arriving. Alert-on-silence: the death of the escalation path is itself what fires [G1 Def.3 as superseded; R04 §2.7].
3. **Quarterly full drill.** At `⚙ verification.drill_interval_days = 90`, a Human-Handoff-style drill: a planted defect must traverse to the right person carrying facts, sources, uncertainty, action history, and a recommended next step; pass/fail recorded [G1 Def.3 as superseded; R04 §2.7]. TBD-OPERATOR(answer the quarterly drill card end-to-end, from a phone).

### S07.8 Scope rules: stakes gating, steered output, degraded domains, graduation

- **Launch domains get axis 2 on every deliverable** [G1 D1.2]: software at v0; web research joins at v0.1. Elsewhere axis 2 is stakes-gated: it runs at stakes tier ≥ `⚙ verification.sanity_stakes_floor = standard` `[coordinator-draft default]` [G1 D1.2; tiers S06.4].
- **The zero-interaction band skips ceremony, never verification.** Trivial-band tasks pass V0–V2 like everything else (5.1 has no exceptions); the tier changes only V3's blocking-ness — trivial delivers the non-blocking completion card [S06.4], every other tier requires explicit accept (5.8).
- **Steered-output backstop (4.7).** A deliverable any part of which flowed through an untrusted-content reader (a T-SPEC quarantined helper today [S04.2]; a C3 worker at v0.1) never skips V2 and never skips axis 2, regardless of stakes gating — injected content can steer reasoning inside any sandbox, and the verification gate judging steered output is the guaranteed blast-radius layer [4.7; S5.4; S5.6].
- **Degraded-mode domains** (arrive v1 [XREF:S19]): V0 and V3 are universal from day one; V1 is empty until the domain's purpose-built check pack exists; V2 runs a generic rubric with its verdict **advisory and visibly marked non-authoritative**; mandatory requester review is the real gate; receipts carry the domain's verification-maturity level [2.1; 7.6; R04 §4].
- **Bootstrap posture — the command-less launch-domain task [A14, 2026-08-27].** A launch-domain deliverable whose project has NO captured check pack (the fresh-scaffold case: the S13.7 registry capture holds no build/test/lint commands, so S07.3's executable ladder has nothing it could run) is NEVER a verification refusal and never parks the run. Instead: V0 runs unchanged; **V1 runs the S07.3 stage-contract checks**, with every executable-ladder rung that would need the missing commands recording **`UNVERIFIABLE-HERE`** (the ratified S07.3 contract vocabulary) rather than being skipped silently or reported as PASS; **V2 runs with its verdict advisory and visibly marked non-authoritative** (the degraded-mode treatment above); **requester review is mandatory** — V3 blocks at every stakes tier, including trivial-band. The verdict card and receipt name the bootstrap posture in plain words, including that capturing the project's commands restores the full ladder. The posture is computed per revision from the registry's current capture — once commands exist, the full ladder resumes and the advisory marking drops; no user action is ever a precondition for verification itself to run. A verification refusal terminal remains only for genuine integrity cases (S07.7 CHECK-INTEGRITY), never for pack absence [operator record `P3/design/b6-gate-operator-findings-r4-2026-08-23.md` F1a; drafted at P3-RW-14 OQ3].
- **Graduation rule — 7.6 made falsifiable:** a domain gains full-pipeline status only when its purpose-built check pack exists AND passes its planted-defect suite [R04 §4]. Suite execution is benchmark/eval machinery [XREF:S14].

### S07.9 Verification-practice failure modes (P-T06-1..5, first-class)

Verification is a platform practice with its own failure modes, not a stage [G1 findings digest, T06]. Each is owned here with a normative countermeasure:

| Id | Failure mode | Countermeasure (binding) |
|---|---|---|
| **P-T06-1** verifier rot | check suites and rubrics decay silently (30% of a professional benchmark broken; 345 falsely-passing patches) [R04 §2.4] | `verified-on` stamp on every check suite and rubric bundle; planted-defect audit every `⚙ verification.check_audit_interval_days`; a suite past its audit interval flags the verdict card stale; audit failure → CHECK-INTEGRITY quarantine (S07.7) |
| **P-T06-2** retrieval-contaminated verification | executor passes checks by lookup, not work [R04 §2.4] | verification-run confinement defaults: network-off + answer-bearing VCS history stripped (S07.3 rule 1); research nodes are the sanctioned, PLAN-declared lookup path [S06.6]; mechanics [XREF:S11] |
| **P-T06-3** style-inflated judging | well-formatted mediocrity over-scores (style bias 0.10–0.76, dwarfing position bias) [R04 §2.1] | judges receive extracted claims/diffs, not presentation, wherever the deliverable type allows; behaviorally anchored rubrics with evidence quotes (S07.10); per-judge-model length-bias noted in the rubric bundle and re-measured on judge change |
| **P-T06-4** escalation-path death | the route exists in config but not in fact — P46 in new clothes | the three S07.7 liveness proofs (planted-defect e2e class + daily dead-man canary + quarterly drill), wired into observability [XREF:S14] |
| **P-T06-5** judge/rubric calibration invalidated by model change | provider swaps silently change judge behavior mid-stream [R04 §2.8] | judge model pinned per rubric version; any judge-model change — including local duty-alias swaps [XREF:S12] — gates on a golden-set re-run (TPR/TNR re-measured) before unsupervised judging resumes (the 7.3-analog); every local-tier swap mandates threshold recalibration [G3 T15 digest] |

### S07.10 Rubric and judge governance (5.7)

- **Rubrics are versioned knowledge objects entering through the 8.3 knowledge gate** — drafted frontier-assisted at implementation time, versioned, attributable, removable [5.7; XREF:S09]. TBD-P3(seed rubric drafting session per launch domain, 8.3-gated).
- **Rubric engineering rules** [R04 §2.5, §4]: binary verdict per criterion; one construct per criterion; behavioral anchors per level; immutable versioned bundles; extractive-quote grounding so a high score is impossible without evidence.
- **Each rubric version pins its judge model**; changes follow P-T06-5 (S07.9).
- **Golden sets:** 25–50 human-labeled cases per launch domain, refreshed from real incidents; the judge is validated as a classifier (TPR/TNR) against them, and judge-reported rates are statistically corrected rather than taken raw [R04 §2.1, §2.8, §4]. Seeding: TBD-BRINGUP(golden-set seed per launch domain — planted-defect cases + first real outcomes). Storage and runners are eval machinery [XREF:S14].
- **Falsifiability:** the benchmark practice tests whether rubric-driven review actually catches real and planted defects, so the rubrics themselves stay falsifiable [5.7; 11.2]; protocol per `Spec/benchmark-preregistration-v1.md`, cited never restated [XREF:S14]. Planted defects are the standard falsification method for judges, rubrics, AND executable checks — checks rot too (P-T06-1) [R04 §2.8].
- **No prefab scores:** imported "hallucination/quality scores" whose mechanics Sinet cannot inspect are banned; every check is a named, versioned, falsifiable object [R04 §5; 5.7].

### S07.11 Recording, receipts, cost shape, and the watchdog boundary

- **Every verdict is recorded with its reasons, per round (S2.3):** what was checked, against which criteria (AC ids + rubric version + judge model + its current golden-set error rates), what passed, what failed, what was flagged note-vs-blocker — as event-log rows under the ratified event-type contract [G2 D2.1(d); XREF:S14], so progression across revisions is visible. Verdicts are keep-forever [G2 Def.11]. Verdicts also land in the ledger's verified-status section [G1 Def.12; XREF:S05], and every V2 verdict is checkpointed (D7) [XREF:S02].
- **Cost shape:** per-deliverable paid verification is bounded to ≤ rounds × 2 judge calls (one compliance + one sanity); V0, V1, entailment, and any future screens run on the free tier; the judge sees the S07.5 input slice, never history [R04 §2.9, §4]. Verification consumption is ceremony-billed to the requester and itemized on the receipt (3.4/3.6) [XREF:S10] — the platform's verification tax is a permanently measured number, and the self-family-judge flag rides the same receipt [G1 Def.1].
- **Watchdog boundary:** stalled/looping/silent/runaway detection belongs to the watchdogs [4.6; XREF:S14]; containment is pause-and-flag, never auto-kill [G1 D1.3]. Verification never watches liveness, and a watchdog flag is never a quality verdict. The single crossing point: SAFETY-class routing (S07.7) and the shared run identifiers in the event log, so a paused run's verification history is one query away [XREF:S14].

**Settings introduced (⚙):**

| Name | Default | Clamp/range | Ratified by |
|---|---|---|---|
| `verification.rework_rounds` | 3 | 0–ceiling | R04 §4 via G1 D1.1(d); see Open item 1 |
| `verification.convergence_patience_rounds` | 2 | 1–`rework_rounds` | R04 §2.6/§3-D via G1 D1.1(d) |
| `verification.sanity_stakes_floor` | standard | tier enum (S06.4) | G1 D1.2 mechanism; default `[coordinator-draft]` |
| `verification.entailment_sample_rate` | TBD-BRINGUP → derived 0.20 (A8, 2026-07-23); live write pending bring-up | 0–1 | G1 Def.2; G3 Def.4/Def.8 |
| `verification.research_rerun_limit` | 1 | 0–2 | 1.9; S06.6 hand-off (R03 §4 Stage 2(d) as cited there) |
| `verification.check_audit_interval_days` | 90 | > 0 | P-T06-1; R04 §4 "periodic"; default `[coordinator-draft]`, aligned to the drill cadence |
| `verification.canary_interval_hours` | 24 | > 0 | G1 Def.3 as superseded at close ("canary daily") |
| `verification.drill_interval_days` | 90 | > 0 | G1 Def.3 as superseded at close ("drill quarterly") |
| `verification.card_remind_hours` | 4 | > 0 | G1 Def.3 as superseded at close |
| `verification.card_push_hours` | 24 | > 0 | G1 Def.3 as superseded at close |
| `verification.safety_reping_hours` | 1 | > 0 | G1 Def.3 as superseded at close |

All per G1 rider 1: operator-editable with audit trail; auto-adjustment only within operator ceilings, visible on receipts. The four SLA values are inbox-wide (registered here, consumed by S15; S18 reconciles naming).

**Known problems owned here:**
- **P-T06-1** verifier rot → `verified-on` stamps + scheduled planted-defect audits + stale-flagging + audit-failure quarantine (S07.3, S07.9).
- **P-T06-2** retrieval-contaminated verification → network-off, history-stripped verification confinement defaults (S07.3); enforcement mechanics [XREF:S11].
- **P-T06-3** style-inflated judging → claims/diffs-not-presentation judge inputs + anchored rubrics + per-judge length-bias tracking (S07.5, S07.9, S07.10).
- **P-T06-4** escalation-path death → three liveness proofs (S07.7); observability wiring [XREF:S14].
- **P-T06-5** judge calibration invalidated by model change → judge-pin per rubric version + golden-set re-run gate before unsupervised judging resumes (S07.9, S07.10).

**Deferred / parked:**
- Local calibrated-abstention screen tier (the N11 cascade middle) → re-entry when the local battery clears a pre-registered TPR/TNR bar on Sinet golden sets on real outputs AND consumption pressure demands relief; expectation pinned to window relief only — production cascades are accuracy-neutral [R04 §6-N11; G3 Def.3 pattern; XREF:S12].
- Axis-2 demotion to stakes-gated in launch domains → 11.2 evidence that axis 2 adds no catches over compliance-plus-human across 3 months of real use [R04 §4; G1 D1.2].
- V2 migration to the free tier → a local model reaches frontier-band agreement on the golden sets [R04 §4].
- Aggregated programmatic weak verifiers with learned weights → a labeled-outcome corpus exists (post-launch) [R04 §3-A4].
- Research-domain V1 pack + entailment activation → v0.1 domain arrival [XREF:S19].
- Standing trajectory/side-effect audit machinery → only if a real side-effect incident class emerges; admissible today solely as the special-purpose audit named in S07.5 [R04 §2.9].

**Coverage:**

| Feature-list item | Subsection |
|---|---|
| 5.1 nothing delivered unverified; type-matched checks | S07.1, S07.2, S07.3 |
| 5.2 two independent questions; reopen-the-spec as human decision | S07.1, S07.5 |
| 5.3 cheap-first; effort scales with survivors and stakes | S07.1, S07.2, S07.3 |
| 5.4 bounded rework; blockers-vs-notes; original criteria | S07.5 (blocker rule), S07.6 |
| 5.5 concrete, carried-forward feedback | S07.5 (`[F1..Fn]`), S07.6 |
| 5.6 tested escalation paths | S07.7 |
| 5.7 per-domain judges; falsifiable rubrics | S07.10 (+ S07.8 graduation) |
| 5.8 human final gate | S07.1 (V3), S07.7 (inbox), S07.8 (band rule); accept mechanics [XREF:S13] |
| 1.9 slice — did-research-actually-run | S07.3 |
| 2.1/7.6 slice — verification maturity, degraded mode, graduation | S07.8 |
| 3.4/3.6 slice — verification billing and itemization | S07.11 |
| 4.7/S5.4/S5.6 slice — verification gate on steered output | S07.8 |
| S2.3 verdicts recorded with reasons, per round | S07.11 |
| 11.2 slice — rubric falsifiability via the benchmark practice | S07.10 [XREF:S14] |

**Open items for G4:** none.
1. ~~Rework-round default~~ **Coordinator-resolved (2026-07-18): default 3 stands.** The apparent conflict is a binding-layer question the contract already answers: G1 D1.1(d)'s operative text ratifies **"(report 04 §4)"** — whose stopping rule reads verbatim "hard cap (default 3 rounds, config)" [R04 §4-D, §4 rework-loop] — while "(one retry)" appears only in the same gate file's findings-digest compression, which is not the ratified object. Blockers-only re-entry makes rounds 2–3 rare in practice; a ⚙ setting either way. Flagged for G4 attention as a resolution note, not an open choice.
