## S17 — Known-problems register

**Scope:** The single consolidated register of every known platform problem (`P-*`) filed anywhere in the campaign — id, problem, origin, owning section, disposition, and the checks it feeds — plus the named problems that carry no `P-*` id. This section consolidates and reconciles; it introduces no new problems and re-litigates no dispositions.
**Binding inputs:** the **Known problems owned here** blocks and inline `P-*` dispositions of S01–S16 (committed drafts) · gate follow-up lists: [G1 §Follow-ups] (P-T01-1..4, P-T17-1..3, P-T05-1..4, P-T06-1..5, P-T02-1..5, R06×4, R07×4), [G2 §Follow-ups] (P-T07-1..5, P-T08-1..5, T09×2 + T10×3 descriptive, P-T11-1..5, P-T12-1..4, P-T16-1..4), [G3 §Follow-ups] (P-T13-1..3, P-T14-1..2, R16×2 descriptive) · `Research/STATE.md` (drafting-coined id list; spike conclusion headlines) · [R02 §9] (T17 problem statements) · feature list §"Known problems this program must solve" · id-coinage notes in S04, S05, S09, S11, S12.

### S17.1 Register conventions

- **Id convention:** `P-T<topic>-<n>` — topic numbers per the campaign topic board (`Research/STATE.md` §Topic board); topic↔report numbering differs because reports are numbered in completion order. The **Origin** column names the filing gate and report; `→S##` marks a family whose descriptive gate filing received its ids in that drafting section (each such section carries its own coinage note).
- **Owner** = the section whose *Known problems owned here* block (or coinage note) holds the disposition; a `·`-joined pair is a deliberate split home, both sides cross-referencing each other. Three ownership adoptions made by this register are tagged `[coordinator-draft]` and justified in S17.6.
- **Disposition** is a one-line summary; the owner's text is normative and is never restated beyond that line. **Registrations** lists the standing machinery the problem feeds — conformance-registry entries, watchdog checks, canaries, eval-practice hooks, watch rows — by [XREF].
- Rows are grouped by filing gate. Family counts are counts of what the corpus actually contains, verified by sweep (S17.6-R7); the only id filed by a gate and absent from the drafts was P-T17-2, restored in S17.2/S17.6. The register totals **64 ids**:

| Gate | Families (count) | Subtotal |
|---|---|---|
| G1 (S17.2) | T01(4) T02(5) T03(4) T04(4) T05(4) T06(5) T17(3) | 29 |
| G2 (S17.3) | T07(5) T08(5) T09(2) T10(3) T11(5) T12(4) T16(4) | 28 |
| G3 (S17.4) | T13(3) T14(2) T15(2) | 7 |

### S17.2 G1-filed problems (Wave A topics: T01, T02, T03, T04, T05, T06, T17 — 29 ids)

| Id | Problem (one line) | Origin | Owner | Disposition (one line) | Registrations |
|---|---|---|---|---|---|
| P-T01-1 | Engine transcripts are not durable checkpoints (cwd-keyed, sweepable, corruptible) | G1·R01 | S03 §S03.3; enforced S02 §S02.4 | Platform checkpoint rows authoritative; engine sessions + transcript copy-aside are resume optimization and harvest material only | copy-aside indexed in `engine_sessions` [XREF:S02]; ask-records durable on observation [XREF:S03] |
| P-T01-2 | Engine schema/behavior drift is permanent, even first-party | G1·R01 | S03 §S03.3; extended S11 §S11.5 | Pin exact + operator-gated deliberate bump + forward-tolerant parsing | per-lane conformance suites, kill-9 harness standing entry [XREF:S14]; model-egress MITM-tolerance canary (cert-pinning regression) [XREF:S11][XREF:S14] |
| P-T01-3 | Headless-subscription billing shifts are ~30-day ops events | G1·R01 | S03 §S03.3 | Flat/metered as data; pre-registered un-pause response = interactive-only demotion [G1 P1]; rehearsed flip, never an architecture change | receipt currency visibly flips [XREF:S10]; S2.8 billing/legal watch [XREF:S14] |
| P-T01-4 | Cancel corrupts engine JSONL mid-tool-call | G1·R01 | S03 §S03.1/§S03.3 | Boundary-first cancel ladder + session-health probe before reuse; fork-from-checkpoint is the standard recovery | adapter-suite park/cancel/resume assertions [XREF:S14] |
| P-T02-1 | Parked engine state silently expires (cleanup-sweep evaporation) | G1·R05 | S03 §S03.4 | Platform ask-record authoritative from the moment observed; engine cleanup horizon raised above the park horizon | ⚙ `adapter.claude.cleanup_period_days` [XREF:S18]; park/resume conformance [XREF:S14] |
| P-T02-2 | Resume-cache fidelity drifts silently | G1·R05 | S03 §S03.3 `[coordinator-draft]` (S17.6-R2) | Standing cache-fidelity alarm: `cache_read` stuck flat across an in-TTL resume vs the calibrated healthy baseline [SPIKE G1-S1 F3] | alarm machinery [XREF:S10][XREF:S14]; P-T08-5's quarterly calibration joins the same suite [XREF:S10] |
| P-T02-3 | Suspend/wake is a crash-equivalent double-resume factory | G1·R05 | S02 §S02.5 | Generation fencing (stale-generation appends rejected) + suspend-aware lease grace render double-resume inert | suspend-cycle test (fake `PrepareForSleep` + clock deltas) [XREF:S14] |
| P-T02-4 | `defer` is single-tool-call-only; parallel gated turns fall back silently (~20%) | G1·R05; SPIKE G1-S2 | S03 §S03.4 | Adapter-side fallback detection + serialize-by-deny as the chosen fallback [SPIKE P2-S4] | ⚙ `adapter.parallel_gate_fallback`; TBD-BRINGUP(fallback rate on default worker model); TBD-P3(serialize-by-deny reconfirm) [XREF:S02] |
| P-T02-5 | The engine harness is itself a quality-drift source invisible to schema tests | G1·R05 | S03 §S03.3 | Engine bump = release event gated on before/after quality probes, not only conformance | bump-gating quality probes + engine-behavior canaries [XREF:S14] |
| P-T03-1 | Whether cache reads weigh less against subscription quotas is publicly unspecified | G1·R06 §7 →S04 | S04 §S04.7 | 0.1× "assumed" weighting + raw counts on receipts [G1 Def.10]; resolution re-opens stagger + weighting as settings changes | provider-watchlist resolution watch [XREF:S14]; weighting consumers [XREF:S05][XREF:S10] |
| P-T03-2 | Sibling-failure containment is Sinet-owned (field default = fail-fast cancellation) | G1·R06 §7 →S04 | S04 §S04.5 | One helper's death/rejection never cancels siblings and never fails the task; salvage carries explicit unfinished markers | mid-run helper-kill conformance test (containment-with-salvage) [XREF:S14] |
| P-T03-3 | No engine enforces D6 (depth caps, spawn logging, no-lateral) | G1·R06 §7 →S04 | S04 §S04.1/§S04.4 | Control-plane-only structural enforcement; engine-native spawning disabled per lane [G1 rider 2] | D6 violation-attempt suite: depth-3, lateral send, native spawn, over-budget spawn [XREF:S14][XREF:S03] |
| P-T03-4 | Helper reports are an injection surface | G1·R06 §7 →S04 | S04 §S04.3 | Reports are data: advisory instruction-pattern screen; structural defense = minimal tools/context per helper | untrusted-content helper confinement [XREF:S11]; steered-output verification backstop [XREF:S07] |
| P-T04-1 | Compaction survival of pinned content is an assumption until tested | G1·R07 §7 →S05 | S05 §S05.7 | Pinned ledger sections re-injected verbatim after any compaction; post-compaction fidelity check on the local tier | canary-constraint→compact→adherence probe, standing conformance entry, re-measured per engine pin [XREF:S14] |
| P-T04-2 | Injected knowledge drifts (stale conventions, shim divergence) | G1·R07 §7 →S05 | S05 §S05.5 | Shim-drift hash check at every assembly + repo-diff-triggered staleness proposals; detection scheduled, never assumed | convention-update proposals via the knowledge gate [XREF:S09] |
| P-T04-3 | Consumption units are lane-heterogeneous (tokens vs prompts vs requests) | G1·R07 §7 →S05 | S10 §S10.1/§S10.4 `[coordinator-draft]` (S17.6-R3) | Approximation-tier hierarchy: Z.AI prompt units stay tier-3 derived and dashboard-reconciled; S05 contributes lane-aware stage granularity | approximation tier stamped on every row and receipt [XREF:S10]; TBD-OPERATOR(Z.AI dashboard prompt-unit calibration) |
| P-T04-4 | Engine-compaction behavior drifts per version (undisableable, version-unstable) | G1·R07 §7 →S05 | S05 §S05.7 | Containment stance: stage-fit budgets keep working sets clear of the trigger; the compactor is never fought or rebuilt | compaction behavior in the canary suite per engine bump [XREF:S14][XREF:S03] |
| P-T05-1 | Plan-stage confinement must bind helpers (prompt-level lockouts fail) | G1·R03 | S06 §S06.6 (policy) · S11 §S11.6 (mechanics) | Declared class flows to every helper tighter-only, enforced at sandbox level; widening is a delta re-plan + re-approval | spawn admission check refuses looser-than-coordinator classes [XREF:S11][XREF:S04] |
| P-T05-2 | Delta re-approval cards carry rubber-stamp risk (no field evidence either way) | G1·R03 | S06 §S06.9 | Delta-only cards + mandatory measurement hook (delta size, time-to-decision, decision, outcome linkage) | analysis owned by the eval practice; measured rubber-stamping proposes a card-format retune [XREF:S14] |
| P-T05-3 | Clarification/intake is an injection surface | G1·R03 | S06 (v0 posture) · joint S11 | v0 intake surface = authenticated workspace only; quoted/pasted material is data, never instructions (4.7) | full C3-class treatment re-enters with 15.5 multi-channel ingress [XREF:S11] |
| P-T05-4 | No published stakes-classification accuracy exists | G1·R03 | S06 §S06.2 | Tolerated by design: deterministic floors override upward, misclassification is cheap by construction, failure fails closed | run-end estimate-vs-actual recording; pre-registered classifier eval before v1 household use [XREF:S14] |
| P-T06-1 | Verifier rot: check suites and rubrics decay silently | G1·R04 | S07 §S07.9 | `verified-on` stamps + scheduled planted-defect audits; stale suites flag verdict cards; audit failure quarantines | ⚙ `verification.check_audit_interval_days`; planted-defect suites in the eval machinery [XREF:S14] |
| P-T06-2 | Retrieval-contaminated verification (pass by lookup, not work) | G1·R04 | S07 §S07.3/§S07.9 | Verification confinement defaults: network off + answer-bearing VCS history stripped; research nodes are the sanctioned lookup path | enforcement mechanics [XREF:S11] |
| P-T06-3 | Style-inflated judging (well-formatted mediocrity over-scores) | G1·R04 | S07 §S07.9/§S07.10 | Judges receive claims/diffs, not presentation; behaviorally anchored rubrics with extractive evidence quotes | per-judge length-bias noted in the rubric bundle, re-measured on judge change [XREF:S14] |
| P-T06-4 | Escalation-path death (the route exists in config but not in fact) | G1·R04 | S07 §S07.7/§S07.9 | Every finding terminates in a human-visible sink; no other sink type exists | three liveness proofs: planted-defect e2e class, daily dead-man canary, quarterly drill [XREF:S14] |
| P-T06-5 | Judge/rubric calibration is invalidated by model change | G1·R04 | S07 §S07.9/§S07.10 | Judge model pinned per rubric version; any change gates on a golden-set re-run before unsupervised judging resumes | revalidation-runbook block [XREF:S14]; local duty-alias swaps included [XREF:S12] |
| P-T17-1 | Provider sanction is allowlist-shaped and revocable server-side without notice | G1·R02 §9 | S03 §S03.6 | Auth canary distinct from limit handling; auth-shaped failure = policy-revocation-suspected → lane freeze + operator alert, never retry-park | ⚙ `canary.auth_interval` daily schedule [XREF:S14]; Class-4 action in the limit taxonomy [XREF:S10] |
| P-T17-2 | Auto-overflow-to-metered plan designs silently violate 3.10 | G1·R02 §9; id restored here `[coordinator-draft]` (S17.6-R1) | S03 §S03.6 | `overflow_mode` classified per model at onboarding; `auto-metered` without a proven disable/zero-balance is rejected | rejection rule [XREF:S10 §S10.2]; receipts visibly change currency on any metered overflow [XREF:S10 §S10.10] |
| P-T17-3 | A lane's model list is region- and account-dependent, not what provider docs say | G1·R02 §9 | S03 §S03.6 | Adapter diffs each account's observed model list against config; routing and 2.7 gap advice consume the observed list with `verified-on` dates | model-list drift on the canary schedule [XREF:S14] |

### S17.3 G2-filed problems (Wave B topics: T07, T08, T09, T10, T11, T12, T16 — 28 ids)

| Id | Problem (one line) | Origin | Owner | Disposition (one line) | Registrations |
|---|---|---|---|---|---|
| P-T07-1 | The pre-sleep inhibitor budget is short and host-configurable (5 s stock / 30 s v0 host) | G2·R08; SPIKE P2-S2 | S02 §S02.5 | Pre-sleep is an O(1) durable flush; all reconcile is wake-side; nothing heavy is ever sized to the window | suspend-cycle test [XREF:S14]; TBD-BRINGUP(operator suspend-session probe) [XREF:S01] |
| P-T07-2 | Reboot destroys the unit-corpse evidence that restart preserves | G2·R08; SPIKE P2-S2 | S02 §S02.5 | The run wrapper's own exit-record append is the mandatory durable evidence; corpses + engine stores are enrichment only | kill-9 harness classification assertions [XREF:S14]; TBD-OPERATOR(reboot-survival host probe) [XREF:S11] |
| P-T07-3 | Idempotency-less channels leave an irreducible unknown-outcome window | G2·R08; G2 D2.4 | S02 §S02.7 | `unknown` is a first-class effect state with a human card; idempotent-capable providers required, plain SMTP an explicit exception | zero-double-executed-effects asserted by the kill-9 harness [XREF:S14] |
| P-T07-4 | No single clock is trustworthy on a sleeping laptop | G2·R08 | S02 §S02.5 | Ordering only from `event_seq`; deadlines are wall-clock DB columns evaluated suspend-aware (wake grace) | suspend-cycle test asserts wake-grace + fencing behavior [XREF:S14] |
| P-T07-5 | The event log is itself a growth/bloat failure mode | G2·R08 | S02 §S02.1 | Payload caps, refs-not-blobs, validate-before-persist | event-log size watch feeding the Tier-0 watchdog [XREF:S14]; ⚙ `state.event_payload_cap` [XREF:S18] |
| P-T08-1 | Engine usage/cost *values* drift and break, not just schemas (3–8× dup, 10× inflation) | G2·R09 | S10 §S10.1 | Ledger sanity bounds + tier-1-vs-tier-2 divergence alarm; no row silently prices to $0 (UNPRICED is visible) | ⚙ `meter.value_divergence_alarm`; conformance suites assert values on known fixtures [XREF:S14] |
| P-T08-2 | Budget, policy, and auth events masquerade as one another on the wire | G2·R09 | S10 §S10.5 | The five-class limit classifier is a tested component; the named worst case is retry-parking a revoked lane | per-lane classifier fixtures [XREF:S14]; Class-4 routes to the P-T17-1 lane freeze [XREF:S03] |
| P-T08-3 | Prices carry effective dates, not just values | G2·R09 | S10 §S10.3 | Effective-dated price table with future-dated rows first-class; drift fires the freshness trigger [G1 Def.5] | watcher diffs announced *future* prices → proposal cards [XREF:S14] |
| P-T08-4 | Pause semantics can destroy deferred work (the drop-on-expiry shape) | G2·R09 | S10 §S10.4 | "Pause preserves everything queued and parked" is a named invariant of one-switch pause and maintenance mode | mandatory test on the pause paths [XREF:S14] |
| P-T08-5 | The pressure gauge inherits the ⚙0.1× cache-weight assumption | G2·R09 | S10 §S10.4 | "Assumed" label stays visible; quarterly calibration against observed depletion events, never window modeling | joins P-T02-2's cache-fidelity alarm suite [XREF:S14]; unified-header observed overlay [XREF:S11] |
| P-T09-1 | Agent-writable configuration is an escape surface (CVE-backed) | G2·R10 (descriptive) →S11 | S11 §S11.7 | Sandbox-side deny on all config paths (symlink-resolved) + empty-env deny-by-default; confinement never depends on engine memory | engine-lowering leak tests per config channel [XREF:S03][XREF:S14]; listener lint [XREF:S01]; guardrail-split schema [XREF:S08] |
| P-T09-2 | Allowlisted egress is an exfil channel (residual) | G2·R10 (descriptive) →S11 | S11 §S11.7/§S11.10 | Bounded, not closed: minimal reachable surface + method restriction + injection proxy; honest residual in the blast-radius invariant | egress volume/pattern anomaly hook in the watchdog suite [XREF:S14] |
| P-T10-1 | Engine-native memory drift is an 8.1-bypass class | G2·R11 (descriptive) →S09 | S09 §S09.9 | Disable-or-contain via compiled config; contained memory dirs are L0, wiped with the task | engine-memory canary entry re-checked per engine pin bump [XREF:S14]; TBD-P3(Claude-lane auto-memory containment) [XREF:S03] |
| P-T10-2 | Proposal-noise economics are unmeasured field-wide | G2·R11 (descriptive) →S09 | S09 §S09.4 (v1 activation) | Accept/dismiss rates instrumented from the pipeline's first day; low acceptance is a drafter/cap defect, never a gate weakening | metrics owned by the eval practice, reviewed at 11.2 checkpoints [XREF:S14] |
| P-T10-3 | Knowledge-injection budget pressure (landfill risk) | G2·R11 (descriptive) →S09 | S09 §S09.8 | Per-scope budgets, deterministic drop order, over-budget drops manifested + carded; silent truncation banned | budget-enforcement conformance test [XREF:S14]; trace-manifest `over_budget_dropped` entries [XREF:S05] |
| P-T11-1 | Benchmark blinding partially fails at household scale | G2·R12 | S14 §S14.7 | Uniform render template; blindness measured, never assumed (BENCH-REG §5) | mandatory arm-guess per verdict; guess accuracy beside every published rate |
| P-T11-2 | The benchmark's direct arm is un-pinnable (consumer surfaces auto-upgrade) | G2·R12 | S14 §S14.7 | Epoch tracker: an observed identity change ends the epoch; no decision statistic pools across epochs (BENCH-REG §9) | per-pair model identity recorded |
| P-T11-3 | Watcher sources decay structurally | G2·R12 | S14 §S14.6 | Per-source fetcher escalation ladder; a dead watch must announce itself | ⚙ `watchlist.fetch_fail_streak` meta-watch; standing migrate-to-feed bias |
| P-T11-4 | Observability/eval tooling ownership churn (M&A risk) | G2·R12 | S14 §S14.1 | Tools are runners or projectors, never stores; the record always lives in `platform.db` | binding adoption criteria on manifest rows + onboarding checklist #7 [XREF:S16] |
| P-T11-5 | Small-n statistical honesty is a product surface | G2·R12 | S14 §S14.7 | The honest-claims table renders wherever a win rate renders; every rate carries n, `G`, tie/decline/guess rates (BENCH-REG §15) | — |
| P-T12-1 | The broker is the ONLY ref protection on Free private repos | G2·R13 | S13 §S13.6 | Stated security property: broker-only keys, protected-ref policy as registry data, CAS-only pushes | broker-side refusal conformance test [XREF:S14]; PAT-gap + Free-ruleset watch rows [XREF:S16] |
| P-T12-2 | Comment orphaning is guaranteed at some rate (~22% open-web baseline) | G2·R13 | S13 §S13.3/§S13.4 | Explicit ORPHAN state; no comment without a render location; orphaned findings still delivered | anchoring-quality 11.2 spot-check [XREF:S14] |
| P-T12-3 | Encrypted remnants make key compromise retroactive | G2·R13 | S13 §S13.10 | Escrow-identity hygiene + annual snapshot-repo rotation; Support-ticket purge as last resort | ⚙ `backup.repo_rotation`; TBD-OPERATOR(age identity escrow before the first snapshot push) |
| P-T12-4 | Snapshot substitution is undetectable by encryption alone (age has no sender auth) | G2·R13 | S13 §S13.10 | The SHA-256 snapshot ledger (DB + in-archive) is load-bearing integrity | fail-closed ledger verify in the scheduled restore drill [XREF:S14] |
| P-T16-1 | Ecosystem mortality: ~14-month half-life for agent components | G2·R14 §7 | S16 §S16.2/§S16.4 | Component-onboarding checklist + mandatory funeral-plan fields before first use | CI lock-presence rule [XREF:S01]; quarterly dependency pass (S16.7); frontend-tree enforcement [XREF:S15] |
| P-T16-2 | Repo-level licenses lie; per-directory splits are standard | G2·R14 | S16 §S16.2 | Path-scoped license field verified per-directory at the exact pinned ref | checklist #4; re-check on every pin bump (S16.7) |
| P-T16-3 | Protocol-surface churn is a scheduled event, not a surprise | G2·R14 | S16 §S16.8 | Protocol touchpoints pin their protocol version with dated migration triggers | MCP 2026-07-28 + ACP v1 watch rows [XREF:S14][XREF:S03] |
| P-T16-4 | Registry supply chain: public skill/template registries are poisoned (~12% observed) | G2·R14 §7 | S16 §S16.6 (rule) · S08 §S08.10 (enforcement) (S17.6-R4) | No auto-import from any public registry, ever; human imports pass the full battery through the D10 gate | import battery: lint + instruction-pattern screen + permission audit + dry run [XREF:S08] |

### S17.4 G3-filed problems (Wave C topics: T13, T14, T15 — 7 ids)

| Id | Problem (one line) | Origin | Owner | Disposition (one line) | Registrations |
|---|---|---|---|---|---|
| P-T13-1 | Post-resume network-identity reconcile (`tailscaled` documented to wedge after wake) | G3·R17 §7 | S01 §S01.7 (detection/duty) · S11 §S11.8 (privileged path) | Wake-side health-check + fixed-verb `sinet-netremediate.service` under the scoped polkit grant [G3 Def.7] | resume-reconcile watchdog check [XREF:S14]; every remediation trigger event-logged [XREF:S11] |
| P-T13-2 | Listener-binding drift silently collapses the identity story | G3·R17 §7 | S01 §S01.6/§S01.8 | Fail-closed startup lint + explicit operator-visible allowlist; re-run at every resume | recurring watchdog audit; foreign violation = flag-now [XREF:S14] |
| P-T13-3 | Three metadata observables leave the LAN by design (CT logs, push metadata, TLS issuance) | G3·R17 §7 | S01 §S01.8 | Complete signed register; a fourth observable only via S00.9 amendment | TBD-OPERATOR(observables-register sign-off); frontend no-new-observables duty [XREF:S15] |
| P-T14-1 | Engine-pin bumps are mass revalidation events with no provider clock | G3·R15 §7 | S08 §S08.10(b) | Sinet's deliberate-bump procedure IS the announcement clock; every affected template flagged before further unsupervised use | bump gate [XREF:S03]; revalidation runbook executes the re-runs [XREF:S14]; engine-behavior canaries between bumps [XREF:S14] |
| P-T14-2 | Worker definitions are a prompt-injection carrier class (91% of malicious registry skills) | G3·R15 §7 | S08 §S08.10 | Station-1 instruction-pattern screen on every draft/import; static-only skills; review-as-diff; the guardrail split keeps enforcement unreachable | battery re-runs on every version change [XREF:S08] |
| P-T15-1 | Model churn invalidates calibration (thresholds, confidence maps) | G3·R16 ("model-churn invalidates calibration") →S12 | S12 §S12.10 | Swap ⇒ recalibrate + revalidate is a hard gate; no alias retarget goes live uncalibrated | recalibration mandate on every local-seat swap [XREF:S14]; 7.3 worker revalidation flags [XREF:S08] |
| P-T15-2 | Stack-capability drift — docs lie, probe behavior | G3·R16 ("stack-capability drift") →S12 | S12 §S12.2/§S12.10 | Behavioral conformance probes (`/v1` logprobs, `json_schema`, llama-swap contract) at every engine bump; documentation is never the check | conformance-registry entries [XREF:S14]; pins [XREF:S16] |

### S17.5 Named problems without P-* ids

| Item | Named by | Owner(s) | Disposition |
|---|---|---|---|
| **Context rot** | feature list §Known-problems; carried in S05's owned block | S05 §S05.3 (within-run) · S05 §S05.6 + S02 §S02.6 (across-pause) · S09 §S09.8 (memory hygiene) | No single mechanism claims it; each dimension has its named owner [R07 §7.8 as cited in S05]. Deliberately carries no P-* id — it is a feature-list-owned problem whose split disposition this row makes auditable. |
| **llama-swap bus-factor-1** | S12 §S12.2 (owned block, "referenced") | S12 §S12.2 | Accepted risk, not an open problem: pre-registered fallback ladder (router mode + kill-idle timer, or Ollama) + `components.lock` exit plan [XREF:S16]; its YAML/endpoint contract conformance-asserted on every bump [XREF:S14]. |

The feature list's remaining known-problem rows (deferred error correction, session amnesia, silent failures, quota-state opacity, provider drift, model deprecation, injection/exfiltration, auto-composed-worker validation, host availability, resource contention, state loss, write collisions, permission-config drift) each carry their owner in the feature list itself; verifying their spec coverage is S19's sweep [XREF:S19]. This register consolidates the research-phase `P-*` set that extends that list, per the feature list's own instruction that research keep hunting and assign owners.

### S17.6 Reconciliations — register findings and adopted readings

Sweep findings, each resolved by the reading the ratified record implies; none re-litigates a disposition.

- **R1 — P-T17-2 id restored.** G1 filed P-T17-1..3 [G1 §Follow-ups]; the drafts carry -1 and -3 but nowhere tag -2. Its disposition nonetheless survived intact and in substance: the `overflow_mode` onboarding attribute with reject-on-unprovable-disable is S03 §S03.6 text; the rejection rule and the visible currency flip are S10 §S10.2/§S10.10 text — exactly the R02 §9 remedy. The register therefore records P-T17-2 with owner S03 §S03.6 `[coordinator-draft]`; the coordinator adds the id tag to S03.6 at assembly (a tag, not a text change).
- **R2 — P-T02-2 owner adopted.** Dispositioned in S03 §S03.3 (the standing cache-fidelity alarm) but claimed by no owned block — S03 and S10 both list it "shared/referenced". Adopted owner: **S03 §S03.3** `[coordinator-draft]` — S03 hosts the countermeasure text and owns every other engine-behavior member of the T02 family except P-T02-3 (explicitly S02's); the alarm machinery and the P-T08-5 calibration remain S10/S14 registrations.
- **R3 — P-T04-3 ownership completed.** S05's owned block assigns it to S10 ("owned by S10"), but S10's block does not list it. The disposition substance is S10 text (tier-3 derived prompt units, per-lane normalization, approximation labels), so the assignment is correct and merely unreciprocated. Adopted owner: **S10 §S10.1/§S10.4** `[coordinator-draft]`.
- **R4 — P-T16-4 dual listing split.** Both S08 and S16 list it under *owned here*. The contents agree; the homes differ by role. Register reading: **S16 §S16.6 owns the binding rule** (no-public-registry-imports, an anti-harvest row), **S08 §S08.10 owns the enforcement** (the import battery in the composer/import path). Recorded as a deliberate split, mirroring P-T05-1 and P-T13-1.
- **R5 — descriptive filings ↔ ids.** Gate follow-up lists that filed problems descriptively map one-to-one onto drafting-coined ids, per each section's own coinage note: G1's "R06×4" = P-T03-1..4 (S04); G1's "R07×4" = P-T04-1..4 (S05, R07 §7.7 order); G2's "config-poisoning-as-escape-surface / allowlisted-egress-exfil" = P-T09-1/-2 (S11); G2's "engine-memory drift = 8.1 bypass / proposal-noise economics / knowledge-injection budget pressure" = P-T10-1/-2/-3 (S09, R11 §7.8–7.10 order); G3's "model-churn invalidates calibration / stack-capability drift" = P-T15-1/-2 (S12). S08's "config-poisoning… unnumbered in the G2 memo" note resolves to P-T09-1. No ambiguity remains; the drafting-coined id set matches `Research/STATE.md` exactly (P-T03-1..4, P-T04-1..4, P-T10-1..3, P-T15-1..2, P-T09-1..2).
- **R6 — deliberate dual homes (not defects).** P-T05-1 (S06 policy · S11 mechanics) and P-T13-1 (S01 detection/duty · S11 privileged path) are split by design; each side cross-references the other. P-T16-4 joins this class via R4.
- **R7 — family counts verified.** Register totals are counts of the corpus, not of assumed ranges: T01=4, T02=5, T03=4, T04=4, T05=4, T06=5, T07=5, T08=5, T09=2, T10=3, T11=5, T12=4, T13=3, T14=2, T15=2, T16=4, T17=3 — **64 ids**. The gate follow-up ranges match these counts everywhere; the only gap found anywhere was P-T17-2 (R1).

### S17.7 Register invariants

- **Zero orphans.** Every one of the 64 registered ids has an owning section and a disposition; three carry `[coordinator-draft]` ownership adoptions (R1–R3), none is undispositioned, and no *Open item* was needed for any id.
- **No coinage, no re-litigation.** This register introduces no new problem and modifies no disposition; where a disposition spans sections, the owner's text is normative and the other homes are registrations.
- **Registration completeness.** Every check named in a Registrations cell exists as declared machinery in its owning section — conformance-registry entries and watchdog checks in [XREF:S14], watch rows in [XREF:S16], settings in [XREF:S18]; this register adds pointers, never machinery.
- **Maintenance rule (post-G4).** A new `P-*` enters only through S00.9 amendment mechanics and must land as a row here in the same amendment — id, one-line problem, origin, owner, disposition, registrations. A `P-*` named anywhere in this spec without a row in this register is a spec defect. Id convention stays `P-T<topic>-<n>`; new problems in an existing topic take the next free `<n>`.

---

**Settings introduced (⚙):**

| Name | Default | Clamp/range | Ratified by |
|---|---|---|---|
| — none — | | | S17 introduces no settings; every ⚙ cited in a row is owned by its disposition's section and indexed by [XREF:S18] |

**Known problems owned here:** none — S17 owns no problems and no dispositions; it is the register. The three ownership adoptions (S17.6 R1–R3) assign owners in S03/S10, tagged `[coordinator-draft]` for G4.

**Deferred / parked:**

- Id-tag backfill into the owning drafts — P-T17-2 tag in S03.6; owned-block acknowledgments for P-T02-2 (S03) and P-T04-3 (S10) → **APPLIED 2026-07-19** by the coordinator, together with the S11.8↔S01.2 run-unit wording alignment; R4 needed no edit (the deliberate split was already recorded on both sides).
- Register upkeep → post-G4 S00.9 amendment mechanics (S17.7 maintenance rule); no standing schedule of its own.

**Coverage:**

| Scope input | Where consolidated |
|---|---|
| G1 §Follow-ups known-problems list (incl. "R06×4", "R07×4") | S17.2; S17.6-R5 |
| G2 §Follow-ups known-problems list (incl. T09/T10 descriptive filings) | S17.3; S17.6-R5 |
| G3 §Follow-ups known-problems additions (incl. R16's two descriptive) | S17.4; S17.6-R5 |
| Every S01–S16 "Known problems owned here" block + inline P-* dispositions | Owner + Disposition + Registrations columns, S17.2–S17.4 |
| Drafting-coined id list (`Research/STATE.md`) | S17.2–S17.4 rows marked `→S##`; S17.6-R5 |
| R02 §9 T17 problem statements | S17.2 (P-T17-1..3); S17.6-R1 |
| Feature list §"Known problems this program must solve" | S17.5 (context rot row + boundary note; coverage sweep [XREF:S19]) |
| Named non-id problems in owned blocks (context rot; llama-swap bus factor) | S17.5 |

**Open items for G4:** none. The four register findings that required a reading (R1–R4) are resolved with `[coordinator-draft]` tags and queued as assembly touch-ups — each is the only reading consistent with the ratified record, flagged for G4 attention rather than left open.
