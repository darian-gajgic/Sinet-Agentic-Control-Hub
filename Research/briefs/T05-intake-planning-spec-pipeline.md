# T05 — Intake, planning & specification pipeline

**Wave:** A2 · **Depth:** FULL · **Report slug:** `intake-planning-spec-pipeline`

## Scope
1.1–1.10 (the whole intake contract: interview, restate-and-confirm, numbered acceptance criteria, criterion traceability, plan self-critique, cheap preview, ask-don't-assume, stakes-gated effort, mandatory live research for data-bearing tasks, ceremony on the utility model), 2.3 (task-type detection), 2.5 (size-guess self-correction), S1.6 (registry-fed intake).

## Why this gates the spec
Intake quality decides everything downstream: bench-02's worst failure was a bad early design frozen into a spec that verification then faithfully defended (P45). The intake pipeline is also the platform's face for non-technical household users — "no prompting skill required" is a hard requirement, not a nicety.

## Core question
How do the best mid-2026 systems turn a vague natural-language request from a possibly non-technical person into a confirmed specification with numbered acceptance criteria and a stress-tested plan — cheaply, with effort scaled to stakes, without interrogating users to death, and without freezing bad ideas into unchallengeable specs?

## Sub-questions
1. Structured intake in the wild: per-task-family interview scaffolds (software vs research vs content need different must-knows — spec 1.2); who does adaptive requirement-elicitation well now (agent products, plan modes of coding harnesses, PM-bot patterns), and what does the evidence say scaffolded interviews add over freeform chat with a strong model?
2. Specification formats agents execute well against: numbered acceptance criteria, testable phrasing, anti-ambiguity idioms; criterion→plan traceability mechanics (1.4: nothing agreed silently disappears).
3. Restate-and-confirm loops (1.3): patterns that converge fast for non-experts; failure modes (users rubber-stamping restatements they didn't read) and mitigations.
4. Plan self-critique / premortem (1.5): evidence that "assume this failed — why?" passes improve plans with 2026 models; and the escape hatch — critique concluding the *spec itself* is bad must surface as a human decision (P45 antidote at intake time).
5. Stakes/size classification (1.8, 2.5): cheap classifiers whose wrongness is cheap (upgrade-before-expensive-work, preview catches overestimates); trivial-task zero-interaction paths that don't leak high-stakes work through.
6. Cheap preview economics (1.6): plan-approval UX that costs the requester seconds; what a "plan" artifact must contain to be approvable by a non-expert.
7. Policy-injected live research (1.9, P47): detecting data-bearing tasks (products, prices, laws, APIs) and forcing a research step by routing policy — never model initiative; who does this today?
8. Ceremony-model routing (1.10): running interview/restate/critique on a designated utility model — quality floor per duty; which duties tolerate small/local models (bridge to T15).
9. Tools-off-during-planning (harvest N6's stance) — does current evidence support planning with tools disabled (except research steps), or has that flipped?

## Constraints that bind this topic
D7 (spec/plan approvals are gates; plans are checkpointed artifacts), D10 (requester approves own spec/plan), 1.8 (stakes-gating is mandatory design, not optional), 13.5 (approvals must explain themselves to non-IT users).

## Harvest-map items to verdict
N6 (Deep Plan engine: triage → family detection → scaffolded interview → DAG with criteria coverage → premortem → bounded revise; tools-off planning), N7 (decision-card pattern at intake), S3/SAW gate-discipline templates (definition-of-done per stage).

## Sources to prioritize
Coding-harness plan modes (docs + engineering posts); framework blogs on planning agents; requirement-elicitation research applied to LLMs (2025–2026); product writeups of intake UX for non-technical users; evidence on self-critique/premortem effectiveness.

## Decisions this feeds
G1: intake pipeline architecture, spec artifact format. Spec: intake/planning stages, acceptance-criteria contract (which T06's verification consumes).
