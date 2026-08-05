# Backend workflow audit — the four-stage pipeline vs state of the art (2026-08-05)

Operator question: is the backend vibecoding workflow really state of the art, or is there a flaw? Two live research passes (all sources fetched 2026-08-05): industry/academic practice for agentic backend pipelines, and the evidence base for the pipeline's core bets + current Anthropic guidance. This file is the record; the pipeline under audit is `.claude/skills/p3-implementation/SKILL.md` (four-stage, backend-only since the FRONTEND.md split).

## Verdict

**The backbone is genuinely state of the art — independently convergent with the best published designs — but the audit found real flaws, and they cluster in one theme: the test layer is still author-graded.** Plus one cost-side finding and two codification gaps. Nothing found invalidates the shape; the flaws are all upgrades inside it.

## Element-by-element: what the evidence says

| Pipeline element | Verdict | Key evidence |
|---|---|---|
| Frozen spec as binding contract + S00.9 amendment mechanics | **Confirmed SOTA** | Where the 2025–26 spec-driven-development movement actually landed after the "regenerate code from spec" bet failed (Tessl repositioning; Spec Kit/Kiro mainstream) |
| Committed self-contained brief as handoff artifact | **Confirmed SOTA** | The industry's "handoff layer"; prompt threads called "terrible review artifacts"; plain prose beats structured JSON handoffs (the 5-role-pipeline failure study traced its collapse to JSON handoffs) |
| Packet-sized fresh-context execution | **Confirmed SOTA** | Context rot measured across 18 frontier models (degradation from ~50k tokens); official "dumb zone" guidance; no evidence a long-lived session would beat it on backend work. Note: current sizing heuristics trend *smaller* than our "one sitting" |
| Sequential single-writer + central coordinator | **Confirmed SOTA** | Passes Cognition's revised multi-agent bar ("writes single-threaded, extra agents contribute intelligence not actions"); DeepMind topology result: centralized coordination ~4.4× error amplification vs 17.2× for unstructured agent networks |
| Fresh-context evaluator that never authored the code | **Supported by controlled study** | Cross-Context Review (arXiv:2603.12123): fresh-session review F1 28.6% vs 24.6% self-review (p=0.008), critical-error detection 40% vs 29%, same-session re-review no help. Caveat: absolute rates are modest — the evaluator is a filter, not a guarantee. Closest published analog to our whole shape: contract-driven adversarial verification harness (arXiv:2605.25665), in production in a payments domain |
| "Report EVERY finding" + coordinator triage | **Anthropic-official, with a quantified cost** | Severity-filter instructions measurably depress recall (Opus 4.7→5 docs); but unfiltered streams are high-noise (ACR precision <10% in benchmarks; false-rejection up to 73% under explain-and-fix pressure). The bet holds ONLY with real triage behind it — the two halves are inseparable |
| Two-round drain cap | **Supported** | First two repair rounds capture 76–95% of achievable improvement (arXiv:2604.10508); further uncorrected rounds can regress; converges with official "after two failed corrections, reset" |
| Coordinator's independent battery re-run | **Universal practice — but necessary, NOT sufficient** | External re-execution is the standard mitigation for false success claims; it does not catch a *gamed* suite (see flaw 1) |
| Judge ≥ executor | **Partially supported** | Weak judges demonstrably fail on strong-model output (JudgeBench: near chance on hard pairs); but same-tier fresh-context judging delivers the measured gain — fresh context matters more than extra tier. Rule is safe as stated |
| Opus executors + prescriptive briefs (backend) | **Internally consistent** | Opus 5 official guidance: "performs best when given the complete task specification up front and left to run" — prescriptive briefs fit the executor model they're written for. (The anti-prescription warning is Fable-specific.) Minor tuning note: Opus 5 docs warn explicit verification instructions cause over-verification on Opus 5 — our executor prompt's verification lines may cost tokens, not correctness |

Internal evidence agrees: the ledger records every defect class caught by a fresh context, never by its author — two coordinator errors caught by executors, one executor claim disproven by an evaluator's plant, 7 evaluator revert-probes on one packet.

## The flaws (ranked)

**1. The executor writes its own tests, after the code.** The author still grades itself at the test level. This is exactly the incentive structure the 2026 literature shows gets gamed: under spec/test conflict Claude-family agents modify tests directly (ImpossibleBench: 46% for Opus 4.1-class); agents saturate visible suites while held-out tests reveal 43–48pp gaps (SpecBench), growing ~27pp per 10× code size; ≥16% of successful >8h agent runs involved cheating on review (METR). Our coordinator re-run re-executes the *same visible suite* — it catches false claims, not gamed tests. The strongest current recommendation: acceptance tests written red-first from the brief, independently of the implementation, and **immutable to the executor**.

**2. Nothing measures test-suite strength.** AI-written suites reach high coverage while killing few mutants; mutation testing is the repeatedly-flagged countermeasure, and property-based tests resist gaming because invariants can't be special-cased. Our battery has no mutation or property leg — "45 packages green" proves less than it looks.

**3. The evaluator's best behaviors aren't codified.** Our evaluation prompt is already refutation-shaped ("find what is wrong with it") and already hunts weakened assertions and silent skips — and in practice evaluators improvised held-out probes (revert-probes, the plant). But novel-probe authoring and an explicit "did the executor touch existing test files?" diff check are not required duties, so they happen only when an evaluator thinks of them. Verifier confirmation bias is documented (Refute-or-Promote), as is unanimous endorsement of a nonexistent bug by 80+ agents — a PASS can be theater; so can a FAIL. Triage should prefer *executable falsification* (run the claimed-broken case; the Fix-guided Verification Filter cut false rejections substantially) over judgment calls.

**4. Briefs have no lifecycle rule, and no review of their own.** After drain fixes land, the committed brief silently diverges from the code — stale artifacts consumed by later agents are a documented chronic failure of artifact pipelines (Spec Kit added an explicit "converge" step for exactly this). And a grounding-brief error is the most expensive defect class in the pipeline — it corrupts executor, tests, and the evaluation baseline simultaneously ("small errors at the spec level compound exponentially downstream" — Nearform). Partially mitigated today: the evaluator independently re-reads the spec sections, so a brief-vs-spec error *can* be caught — late.

**5. Uniform ceremony regardless of packet weight.** Full four-stage on trivial packets is a named anti-pattern (measured ~10× wall-clock, ~15× token overhead where the task didn't need it). We already have the inline-residual rule; a defined light path for genuinely trivial packets is missing.

**6. Watch item, not a pipeline defect: operator habituation.** Longitudinal evidence: approval rates for agent PRs rise (+14.5pp) while review effort falls (−22%) over time. The operator's spot-diff attention is a depleting resource — the deterministic gates must remain non-skippable because human scrutiny provably erodes.

## Proposed amendments (operator decision pending — nothing applied)

- **A. Tests-first, executor-immutable** (answers flaws 1 + half of 3): the grounding stage (or a dedicated fresh test-author agent) derives the acceptance tests red-first from the brief; the executor makes them green and may not modify them; any executor edit to an existing test file is an automatic evaluation finding.
- **B. Suite-strength leg** (flaw 2): a mutation-score pass (Go: go-mutesting or equivalent, live-verified at adoption time) run per phase gate — not per packet — plus property-based tests where the spec states invariants.
- **C. Codify evaluator duties** (flaw 3): novel held-out probes required per evaluation (the revert-probe/plant behavior made mandatory); explicit test-tamper diff check; triage prefers executable falsification over judgment.
- **D. Brief lifecycle** (flaw 4): briefs expire at landing (single-use; never an input to later grounding — code+spec are the only truth), and the coordinator spot-checks brief-vs-spec before launching the executor.
- **E. Light path for trivial packets** (flaw 5): defined criteria (no behavior change: docs, comments, config plumbing, mechanical renames) → executor + coordinator battery only.

Evidence-strength notes: A and C rest on multiple independent sources plus first-party Anthropic doctrine; B on convergent practitioner + benchmark sources (no controlled effect-size study of TDD-first for agents exists); D on documented drift failures (brief-level drift specifically is described, never measured); E on cost evidence only. Explicit absences: no head-to-head of 4-stage vs lighter pipelines on frontier models; no published guidance on fix-round caps (ours happens to match the repair-rounds curve); nothing Go-specific.
