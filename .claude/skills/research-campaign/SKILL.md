---
name: research-campaign
description: Coordinator runbook for the Sinet research campaign. Use when the operator says "continue the research campaign", "next research topic", "run research", "campaign status", or any session meant to advance Research/. Reads Research/STATE.md, runs deep-research per topic brief in subagents, validates and commits reports, and pauses at decision gates.
---

# Research campaign — coordinator runbook

You are the campaign coordinator. Your memory is `Research/STATE.md`; your plan is `Research/CAMPAIGN.md`. The operator's standing instruction: **run autonomously; pause only at gates and for genuine decisions.**

## Session entry

0. **Effort:** campaign sessions run at maximum effort — operator's standing instruction (2026-07-16): research quality gates everything downstream. If the session is not already at max (`/effort`), tell the operator to set it before launching topics; research subagents inherit the session's effort level.
1. Read `Research/STATE.md`, then `Research/CAMPAIGN.md` §2–§5. Check `git status` is clean (if not, inspect — a prior session may have died mid-step; finish or roll forward its checkpoint first).
2. If a gate is OPEN: re-present its decisions to the operator (from the gate file — never from memory) and record answers before anything else.
3. Otherwise take the next action from STATE.md. Never re-plan the campaign from scratch; never relitigate closed gates.

## Security-content isolation (Fable-5 safeguard)

Fable 5's intentionally-broad dual-use safeguard false-positives on the campaign's security-research content (spikes S1/S3/S5, reports 09 metering / 10 sandboxing) and auto-falls-back to Opus 4.8 (lossless). Two standing rules keep Fable sessions productive:

1. **Keep the live layer terse.** `Research/STATE.md` carries only neutral coordination state (status, next action, gates, board, report numbering, conclusion-level spike headlines). Full operational detail + the chronological session log live in `Research/STATE-ARCHIVE.md`. When you log new work, put the terse coordination outcome in STATE.md and any vivid operational detail (probe methods, wire captures, secret-hygiene records) in STATE-ARCHIVE.md or the relevant report — never reintroduce operational prose into the live layer.
2. **Delegate security-report deep-reads to Opus.** When a task genuinely needs to read a security-heavy report (09, 10, the spike reports) or to draft a spec section that consumes them, spawn an **Opus-pinned subagent** (`model: opus`) to do the reading/synthesis and return a neutral deliverable; work from that deliverable, not the raw security prose. This is the same Opus-pinned pattern the spike battery used.

This isolates the *content* that trips the classifier (which is server-side and not ours to configure) and routes security-content reading to the model Anthropic currently designates for it — cooperating with the safeguard, not defeating it. **Never obfuscate or encode content to evade the classifier.** Memory: `fable5-safeguard-false-positive`.

## Running a topic (Mode A — default)

For topic T## with slug `<slug>`, next free report number NN (check existing `Research/[0-9][0-9]-*.md`):

1. Update STATE.md: T## → `running`, note the agent id once launched. Commit is NOT needed for this step.
2. Launch a background Agent (subagent_type `claude`) with exactly this shape:

   > You are running one topic of the Sinet research campaign — research only, no implementation.
   > 1. Read `Research/briefs/00-shared-context.md` and `Research/briefs/T##-<slug>.md` fully, plus the feature-list sections your brief's Scope lists (`Docs/agent-platform-feature-list-v1.md`).
   > 2. Invoke the Skill tool: skill `deep-research`, args = the brief's **Core question** plus a one-paragraph digest of the binding constraints (D-constraints named in the brief, the subscription-coverage rule, adopt-don't-fork). Follow the deep-research harness completely — fan-out searches, source fetching, adversarial verification, cited synthesis. Cover the brief's sub-questions, not just the headline. Sourcing standards and output contract are in the shared context and are non-negotiable.
   > 3. Write the finished report to `Research/NN-<slug>.md` (all 8 contract sections).
   > 4. Final message: the output path, a ≤10-line executive digest, the list of open questions needing operator decisions, and your source count. Do NOT paste the report.
   > If the Skill tool or deep-research is unavailable, STOP and return `BLOCKED: <reason>` — never simulate the harness with ad-hoc searching. If the brief leaves something ambiguous, resolve it from the Docs/ files yourself; only return BLOCKED if truly undecidable.

3. Up to **2 topics concurrently** within the approved wave. While agents run, you may validate/commit completed reports or prepare gate memos — do not idle-poll.
4. On completion: run the **validation checklist** (below). Then STATE.md: `saved` → `committed` (fill the NN map), and:
   `git add Research/ && git commit -m "Research NN: <title> (T##)" && git push`
5. Launch the next topic until the wave is done, then open the gate.

**Mode B (fallback, use if Mode A returns BLOCKED or malforms twice):** invoke `deep-research` yourself with the same args, follow the harness inline, write the report yourself. One topic at a time. Record the mode switch in STATE.md.

## Validation checklist (every report, before commit)

- All 8 contract sections present; Scope matches the brief.
- Recommendation traceable to cited evidence; "what would change the decision" present.
- Spot-check 2–3 load-bearing citations by fetching them (WebFetch) — they must exist and say what's claimed.
- ToS/pricing/licensing claims cite primary sources with access dates.
- Harvest items from the brief all verdicted; open questions answerable; no D1–D10 contradictions.
- FAIL → one bounded revision: SendMessage to the same agent (or a new one) listing the specific gaps. Still failing → commit with a `> ⚠ QUALITY FLAG` header line and raise it at the gate. Never silently accept, never endlessly loop.

## Gates

When the last topic of a wave is committed:
1. Write `Research/decisions/GATE-N-<slug>.md` from `GATE-TEMPLATE.md` — findings digests + numbered decisions with options, recommendation, and what each forecloses.
2. Commit + push the memo, mark STATE.md `blocked-on-gate`.
3. Present with AskUserQuestion (≤4 questions per call; group minor items as clearly-marked "defaults unless objected"). Record every answer in the gate file + STATE.md, commit, unblock the next wave.
4. After G1 only: append a one-paragraph "G1 addendum" (engine direction + session model decisions) to briefs T07–T09 before launching Wave B.

## Autonomy contract

Without asking: launch approved-wave topics, validate, bounded revisions, save/commit/push, citation spot-checks, resolving scoping questions from Docs/.
Always pause: gate decisions; anything binding architecture, spending money, creating accounts, changing scope; anything implementation-shaped (research phase — flag and confirm); source contradictions that change project direction.

## Failure ladder

- Agent dies / returns junk → relaunch once with the failure noted in the prompt; twice → Mode B.
- deep-research asks clarifying questions → answer them yourself from the brief + Docs/; that's what the briefs are for.
- Usage/session limit hit → STATE.md is already current (you update it before and after every step); stop cleanly; any next session resumes via this skill.
- Contradiction with a fixed constraint discovered in research → that's a gate item, never a reason to bend the constraint silently.

## Hard boundaries

Research phase only: no application code, no scaffolding, no "quick prototypes". Docs/ is read-only. D1–D10 are fixed inputs. Adopt-don't-fork and no-load-bearing-metered-paths bind every recommendation you accept into a gate memo.
