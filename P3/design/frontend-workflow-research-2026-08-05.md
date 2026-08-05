# Frontend workflow research — evidence base for FRONTEND.md (2026-08-05)

Two live research passes (background agents, ~30 web fetches each, all sources fetched 2026-08-05) commissioned after the operator rejected the UI batch. Stream A: Anthropic official guidance. Stream B: industry practice 2025–2026. This file is the provenance archive; the binding process artifact is `.claude/skills/p3-implementation/FRONTEND.md`.

## The verdict the research delivers on the UI batch

Every root cause identified in the session's error analysis is a *named, documented* failure mode in current sources — the batch's workflow was state-of-the-art for backend correctness and anti-pattern-by-the-book for frontend:

1. **Fresh-context-per-packet authorship → incoherent UI** is Cognition's canonical example ("Don't build multi-agents", cognition.com/blog/dont-build-multi-agents): subagents' "actions carry implicit decisions, and conflicting decisions carry bad results" — their illustration is literally a UI built by two agents with clashing visual assumptions. Anthropic's multi-agent research post agrees: "most coding tasks involve fewer truly parallelizable tasks than research."
2. **Rubric/test-only acceptance → proxy divergence** is published: rubric verifiers for frontend saturate and get gamed ("The Verification Horizon", arxiv.org/abs/2606.26300); LLM judges are weakest exactly on visual coherence and interaction usability (WebDevJudge, arxiv.org/pdf/2510.18560); "error-free code may exhibit poor visual quality, broken animations, or incorrect interactions" (ArtifactsBench).
3. **Prescriptive briefs degrade output** — Anthropic's own Fable 5 guide: "Skills developed for prior models are often too prescriptive for Claude Fable 5 and can degrade output quality."
4. **No screenshot in the loop** violates the #1 rule of the current Claude Code best-practices doc: "Give Claude a check it can run: tests, a build, a screenshot to compare." The docs' UI recipe is paste-mock → implement → screenshot → list differences → fix.
5. **Prose specs of visual designs** — no source recommends specifying UI in requirements prose; the whole prototype-first/reference-image literature exists because text descriptions of visuals lose the information that matters.

## Load-bearing findings (with sources)

### Authorship and slicing
- Single-author coherence: one strong-model agent owns the entire frontend in one continuous context; subagents research or review, never co-author screens (cognition.com/blog/dont-build-multi-agents; addyosmani.com/blog/code-agent-orchestra — "single agents consistently outperform multi-agent setups when tasks lack clear decomposition").
- Anthropic sub-agents doc: iterative refinement with shared context belongs in ONE conversation (code.claude.com/docs/en/sub-agents, "Choose between subagents and main conversation"). Harness-design post: long continuous sessions beat fresh-context slicing for app building on current models (anthropic.com/engineering/harness-design-long-running-apps).
- If splitting is unavoidable: vertical slices by user journey/page, never by spec section or layer (ertyurk.com/posts/full-stack-vertical-slices-the-only-way-to-ship-with-ai; jeremydmiller.com 2026-06-04). Even multi-agent advocates (Builder.io) anchor every agent on one shared design-system context and a live-rendered canvas.
- Commercial state of the art (Vercel v0 Feb-2026 relaunch): single pipeline holding the whole app + strong stack defaults + human watching a live preview — not decomposed generation.

### The visual loop
- Screenshot-in-the-loop self-verification is the consensus #1 quality lever (code.claude.com/docs/en/best-practices; 1D-Bench arxiv.org/pdf/2602.18548 quantifies it: screenshot feedback is the most effective feedback type for layout/styling).
- Full-viewport, multi-breakpoint, full-flow rendering — per-region crops miss "a button that's technically rendered but visually buried" (medium.com/@rotbart round-trip screenshot testing). The UI batch's per-region verification was the wrong granularity.
- Claude in Chrome is the first-party loop; "Design verification" is a named capability (code.claude.com/docs/en/chrome). Anthropic's own harness evaluator drives Playwright, "screenshotting and carefully studying the implementation before producing its assessment."
- Evidence over assertion: the agent shows screenshots, never claims "looks good" (best-practices doc, "trust-then-verify gap").

### Design direction
- Generic output is distributional convergence, not bad luck — Anthropic's own term for the "AI slop" default (claude.com/blog/improving-frontend-design-through-skills, 2025-11-12). Mitigation order of strength: concrete reference (screenshots/named apps — "models interpolate well from strong references") > per-dimension constraints + prohibition lists > persona prompting (weakest, tone-only).
- Official `frontend-design` skill (github.com/anthropics/skills, ~400 tokens, "right altitude"): demands an aesthetic commitment before any CSS; two-pass process (compact design plan → critique against brief → build). Vendor its principles; note "product mode" restraint for data-dense dashboards — bold-aesthetic skills degrade admin UIs (ruoqijin.com/blog/frontend-design-skills-ai-agents, June 2026).
- Sonnet 5 guide's officially recommended pattern where direction is open: propose N concrete visual directions, human picks one, implement only that. For Sinet this gate is already satisfied: the operator ratified "recreate the Nexus look" 2026-08-05 — the Nexus token block + screenshots ARE the art direction.
- Design tokens + a small component kit, approved once, as the mechanical guarantee of cross-surface coherence (supernova.io 2026 trends: token adoption 56%→84% in a year; builder.io multi-agent explainer).

### Evaluation
- Live-environment design review: a fresh-context agent drives the RUNNING app through interaction flows → responsiveness → visual polish → accessibility → robustness (OneRedOak/claude-code-workflows design-review, 3.7k stars — the most-adopted community pattern). Anthropic's harness grades on four criteria: Design Quality, Originality, Craft, Functionality — a ready-made judge rubric applied to pixels, not code.
- Simulated-user cold walks are validated research technique for exactly the "owner can't find his board" failure class: persona-driven agents attempt real tasks in a browser and narrate friction (UXAgent arxiv.org/abs/2502.12561; Avenir-UX; logrocket.com guidance: simulated users for cheap early iteration, real humans final).
- Automated acceptance is a filter, never the gate — the field's benchmarks all moved to human pairwise preference because nothing else measures UI quality (WebDev Arena 288k human votes; UI-Bench). The operator's eyes are the final gate; pairwise pick between 2–3 rendered variants is the highest-signal cheap human evaluation.
- Visual regression (screenshot baselines) is a ratchet that protects operator-approved states from drift — never a quality judge.
- Reviewer scope discipline: "A reviewer prompted to find gaps will usually report some, even when the work is sound" — scope reviewers to correctness/requirement gaps + the four criteria (best-practices doc).

### Prompting and model routing
- Goal-shaped briefs: intent + audience + constraints + verification; give the reason behind the request; "sprint contracts" negotiated with the builder beat detailed technical specs that cascade errors (Fable 5 guide; harness-design post).
- Quality modifier belongs in the build prompt ("go beyond the basics to create a fully-featured implementation") — a bare functional spec produces a bare UI (prompting best-practices, migration notes).
- Fable 5 vision reads dense screenshots "with substantially higher accuracy" (Fable 5 guide) — the builder in a screenshot loop should be Fable. No official Fable-vs-Opus aesthetics ranking exists; routing is our call. Claude Code subagent `model:` frontmatter accepts `fable`. Fable classifier-trip risk on UI work: zero trips across the entire UI batch; lossless Opus fallback stays as the standing remedy (memory `fable5-safeguard-false-positive`).
- Checkpoint cadence: "course-correct early and often"; after two failed corrections, reset with a better prompt; human sees rendered UI at flow milestones, not phase gates (best-practices; designproject.io/blog/claude-code-designer-workflow).

## Evidence-strength notes

Strongest (multiple independent primary sources): single-author coherence, screenshot-in-the-loop, journey slicing, automated-acceptance-as-filter. Validated-in-research/early-in-industry: agent cold-walk usability. Single-source directional numbers (treat as estimates): token-naming accuracy claims, Figma-pipeline costs. Thinnest: persona prompting alone. The one live contest in the field — multi-agent-with-shared-canvas vs single-threaded — is irrelevant to us at one-maintainer scale, and both camps agree on shared design context, live rendering, and human visual review.
