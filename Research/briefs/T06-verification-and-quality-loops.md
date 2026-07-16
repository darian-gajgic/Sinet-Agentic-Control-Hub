# T06 — Verification & quality loops

**Wave:** A2 · **Depth:** FULL · **Report slug:** `verification-and-quality-loops`

## Scope
5.1–5.8 (nothing delivered unverified; spec-compliance AND actually-good; cheap-first cascades; bounded rework; concrete carried-forward feedback; tested escalation; per-domain judges; human final gate), P45 (spec lock-in), P46 (guaranteed escalation routes), 2.1 (launch domains: code = executable checks, web research = citation rubrics), 7.6 (degraded-mode honesty).

## Why this gates the spec
Verification is the platform's quality engine and its second-worst historical failure mode: Nexus's judge machinery flawlessly defended a bad spec (P45) and a stage that saw the problem had no wired route to a human (P46). The spec needs a verification architecture with *evidence* it catches real defects without rubber-stamping or infinite loops.

## Core question
What verification architecture actually catches bad work in mid-2026 — combining deterministic/executable checks, LLM-as-judge, and human gates — while defeating the known failure modes: rubber-stamping, judge bias, spec lock-in (compliant-but-bad), unbounded revision spirals, and escalations that die in logs?

## Sub-questions
1. LLM-as-judge, current truth: reliability and bias findings (position, verbosity, self-preference…), calibration techniques, rubric-anchored vs freeform judging, pairwise vs absolute — the mid-2026 consensus from eval practitioners, not vendor marketing.
2. Two-axis verification (P45): spec-compliance AND spec-independent outcome sanity ("is this actually good?") — does anyone formalize the second axis today? Concrete rubric patterns for it; when the second axis may reopen the spec (5.2) as a human decision.
3. Cheap-first cascades (5.3): deterministic zero-cost pre-gates (empty/truncated/placeholder/diff-empty), small-model screens — evidence screens add signal rather than rubber-stamps; cascade designs that never let a screen block on its own failure.
4. Executable verification for code (launch domain): current best practice for agent-built code (tests, typecheck, runtime smoke, e2e drive-the-feature), sandboxed execution of checks, flaky-check handling.
5. Rubric-driven verification for web research (v0.1 domain): citation/source rubrics that catch fabrication and stale sources; how research tools score output quality today.
6. Bounded rework (5.4): revision caps, blockers-vs-notes separation (only blockers loop; polish travels as notes), frozen criteria across rounds (no goalpost drift), convergence detection — validate the harvested judge-loop semantics (N5) against current practice; what's state of the art for feedback that actually lands (5.5: numbered findings anchored to exact places, carried into the retry)?
7. Escalation that provably works (P46, 5.6): finding-category → decision-route wiring; testing strategies that force an escalation end-to-end (a platform e2e test class); who treats "escalation dies in a log" as a defect today?
8. Who judges the judges: meta-evals, benchmark tie-in (11.2 practice tests whether rubric review catches planted defects — 5.7), drift detection when judge models change (bridge to T11).
9. Verification cost architecture: where verification spend sits relative to execution spend in well-run systems (post-mortem: Nexus's verification overhead was structural, not incidental).

## Constraints that bind this topic
D7 (verification verdicts are checkpointed; outward effects stay proposals through verification), D10/5.8 (human is the final gate — nothing self-approves), 2.1 (degraded-mode domains: visibly marked, mandatory requester review — never unsupervised), D5 (judge-model routing between flat-rate options by consumption pressure).

## Harvest-map items to verdict
N5 (judge-loop v0 subset semantics), N11 (full cascade — later), G1/gh-aw safe-outputs (verification gate before anything externalizes), S3 (stage contracts / definition-of-done per stage).

## Sources to prioritize
Eval practitioners (the current canon on LLM-judging), framework eval docs (LangSmith/Langfuse-class), Anthropic/OpenAI eval guidance, academic judge-bias studies 2025–2026, production reports on agent verification loops.

## Decisions this feeds
G1: verification architecture axes (two-axis design, cascade shape, rework bounds). Spec: verification pipeline, launch-domain check definitions, escalation wiring + its e2e test class (with T11's benchmark hooks).
