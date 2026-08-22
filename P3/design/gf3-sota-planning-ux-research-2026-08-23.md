# GF3 evidence — SOTA planning/interview/plan-review UX (live web research, 2026-08-23)

Operator-ordered live research (papers + shipping products) for the P3-GF3 planning-phase rework.
Verbatim agent report; sources fetched 2026-08-23; UNVERIFIED items labeled by the researcher.

## Best-practice principles (numbered, each with source + date)

1. **Generate task-specific questions from the actual ambiguities; never run a static
   questionnaire.** Active Task Disambiguation with LLMs (ICLR 2025, arXiv 2502.04485); LLMREI
   (RE 2025, arXiv 2507.02564: "highly context-dependent questions" were the LLM interviewer's
   standout strength). Cursor, Lovable, Claude Code, OpenAI Deep Research all generate per-task
   questions; none ships a fixed intake form.
2. **Select questions by expected information gain; stop when value < cost.** SAGE-Agent
   (EVPI-based, arXiv 2511.08798, Nov 2025/rev Apr 2026): 1.5–2.7× fewer questions, +7–39%
   coverage; information-gain reward (arXiv 2606.03135, Jun 2026): +3.7% success at ~0.3 extra
   steps. Rank candidates by how much the answer changes the plan; explicit stopping rule.
3. **A good clarifying question is one whose different answers produce materially different
   plans** (2502.04485). If the system cannot name the divergent outcomes, it should not ask.
4. **Ask few questions; efficiency is scored, not just coverage.** ClarifyCodeBench
   (arXiv 2607.00711, Jul 2026) penalizes inefficient questioning (turn-discounted key-question
   rate). Products converge on ~2–5 questions in one round.
5. **Default-with-override beats open questions for non-technical users: every question ships a
   recommended answer acceptable in one action.** NN/g "The Power of Defaults" (2005); Claude
   Code's AskUserQuestion (options + marked recommendation + free-text escape; official docs,
   live 2026). Recognition-over-recall defeats the articulation barrier (NN/g: half the
   population cannot comfortably articulate intent in prose).
6. **When you don't ask, assume-with-disclosure.** LLMs recognize ambiguity but default to
   answering (arXiv 2605.25284, May 2026); BrainGrid (Jul 2026): vague-prompt speed "invents
   unstated decisions" — the fix is surfacing "what did you assume, what breaks if I guess
   wrong" as correctable items. Every silent default appears in the plan as a labeled,
   individually correctable assumption.
7. **Deliver value before and between questions — anchor questions in what the system already
   found.** Devin's Initial Assessment; Cursor investigates first and asks only what
   investigation couldn't resolve (Cursor Plan Mode blog, Oct 2025). Asking what the system
   already knows is the canonical bad question.
8. **The plan the user reviews must be an editable document, not a chat message.** Cursor
   (Markdown plan artifact, editable to-dos); Claude Code (Ctrl+G opens the plan in the editor);
   Kiro (user-editable requirements.md/design.md/tasks.md with regeneration sync); Lovable
   ("inspect, edit, refine"). Direct editing AND free-text amendment must both exist.
9. **Support multi-item feedback in one round, anchored to plan sections.** Plannotator
   (plannotator.ai, 2026): select any plan text, attach comment/deletion/replacement, return ALL
   annotations as one structured batch, with plan diffs between iterations. Chat-only amendment
   forces re-describing locations in prose.
10. **Approval = a small set of named consequence-bearing choices; "no" loops back to planning,
    never cancels.** Claude Code's three-way approve prompt (rejection keeps planning with
    free-text direction; official docs, live). Auto-proceed timers soft-bypass review (Devin's
    30 s default — offer, don't default for high stakes).
11. **Phase the review with human gates — but offer a skip lane for well-understood work.**
    Kiro's spec flow + gate-free Quick Spec; Cursor: skip plan mode for routine tasks.
    Proportionality is itself a best practice.
12. **One decision per screen as the starting point** (GOV.UK "one thing per page", 2015;
    Smashing 2017 case study); grouping is an optimization you earn through testing.
13. **Progressive disclosure: primary options only, defer the advanced 20%; at most two levels**
    (NN/g; UXPin 2026).
14. **Wizard mechanics: progress shown, back allowed, prior answers become next-time defaults,
    steps self-sufficient** (NN/g Wizards, 2017). Reconciliation of enforce-sequence vs
    skippability: every question skippable BECAUSE every question has a stated default, with the
    consequence visible in the plan.
15. **Plain language throughout; gloss any necessary term inline** (NN/g plain-language +
    technical-jargon research; digital.gov).
16. **Cap review volume; long plans get rubber-stamped** (2026 approval-fatigue literature).
    Short must-read summary, drill-down detail, pre-triaged decisions only.

## Product comparison (verified against current sources)

| Product | Pre-plan questions | Plan artifact | Amendment | Approval |
|---|---|---|---|---|
| Claude Code | generated MC w/ recommendation + free-text escape | plan msg; Ctrl+G editor | free text or edit; reject=keep planning | 3 named options |
| Cursor | after codebase investigation | Markdown file, editable to-dos | chat or direct edit | build from plan |
| Devin | Initial Assessment w/ questions | plan w/ clickable code citations | edit before confirm | confirm; 30s auto-proceed default |
| Replit Agent | some; plan-first optional | ordered task list | refine before approval | approve; per-user prefs |
| Kiro | guided requirements phase | 3 editable artifacts (EARS ACs) | direct edit or spec chat | gate per phase; Quick Spec lane |
| Lovable | conversational clarifying | plan in Plan mode | chat follow-ups | move to Build |
| OpenAI Deep Research | one ignorable batch via intermediate model | adjustable research plan | free text before launch | confirm-or-ignore |
| Bolt.new / v0 | none — builds immediately | none | iterate on output | none |

Composite best-in-class flow: investigate first → one small round of generated,
recommendation-carrying, ignorable questions → short plan artifact with labeled assumptions and
drill-down → direct editing + section-anchored batched comments → few named approval choices
where rejection re-enters planning → proportional bypass for trivial tasks.

## Anti-patterns (each with evidence)

1. Silent assumption-making (2605.25284; BrainGrid Jul 2026).
2. Interrogation before value (voice-AI/support-bot abandonment literature, 2026).
3. Generic questions answerable from context.
4. Questions whose answers don't change the plan (2607.00711; 2511.08798).
5. Open prose questions to non-technical users with no options (NN/g articulation barrier).
6. Forcing an answer with no default and no skip (NN/g defaults).
7. Jargon in questions or plans (NN/g technical jargon).
8. Wall-of-text plans → rubber-stamping (approval-fatigue literature 2026).
9. Chat-only amendment of a structured plan (the reason Plannotator exists).
10. All-or-nothing approval with no per-item channel.
11. Auto-proceed timers as the default gate.
12. One-size process — full ceremony on trivial tasks breeds gate-skipping.
13. Re-asking across sessions what the user already answered (arXiv 2607.26611, Jul 2026,
    search-verified; NN/g wizards on reusing prior selections).

## Research-question detail (condensed)

RQ1: The ask-deficit is behavioral, not perceptual — elicitation must be an engineered step, not
emergent model behavior (2605.25284). Question selection should be information-theoretic
(2502.04485, 2511.08798, 2606.03135) — all reduce question count while improving outcomes.
Clarification competence is DECOUPLED from task competence; performance collapses as ambiguity
density rises (2607.00711) — decompose ambiguity-dense briefs before eliciting. Ontology-guided
interviewing beat LLM baselines on coverage and efficiency (OntoAgent, 2605.05828,
search-verified). Taxonomy-guided follow-ups match or beat human interviewers (2507.02858,
RE 2025). Nothing credible defends static questionnaires; nothing credible defends
open-ended-only questioning for non-technical users — structured options with recommendations
plus a free-text escape is the shipped consensus.

RQ4 caveats: BrainGrid's thorough-interrogation stance vs the information-gain minimal stance
reconciles via stakes-proportionality. UNVERIFIED: OntoAgent and 2607.26611 full texts
(search-verified only); the HBR Mar 2026 piece (paywalled); ChatGPT Work launch date
(third-party).
