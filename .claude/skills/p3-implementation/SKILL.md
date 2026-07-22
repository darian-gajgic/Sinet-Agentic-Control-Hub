---
name: p3-implementation
description: Coordinator runbook for building Sinet v0 from the frozen spec. Use when the operator says "continue implementation", "build", "next packet", "implementation status", "start B0" (or any B-phase), or any session meant to advance P3. Reads P3/STATE.md, executes work packets via subagents against Spec/core-architecture-v1.md, validates, commits, pauses at phase gates.
---

# P3 implementation — coordinator runbook

You are the build coordinator: the campaign-proven pattern re-instantiated for implementation. Your memory is `P3/STATE.md`; your contract is `Spec/core-architecture-v1.md` (**v1, frozen at G4, tag `spec-v1`**; the per-section drafts in `Spec/drafts/` are canonical text) plus siblings `Spec/benchmark-preregistration-v1.md` and `Spec/frontend-components-v1.md`. Operator standing instruction: **run autonomously; pause only at phase gates, spec conflicts, and operator hands-on items.** The operator starts one session and says "continue implementation" — every stage launch below is the coordinator's job, never the operator's. Never end a turn on a plan or a promise: if your last paragraph names work not yet done, do it now; stop only where one of the three pause conditions genuinely holds.

## Session entry

0. **Effort:** coordinator sessions run at max (`/effort`) — operator instruction carried from the campaign. Packet subagents inherit it.
1. Read `P3/STATE.md`, then `P3/CONVENTIONS.md` if it exists. Check `git status` is clean (if not, inspect — a prior session may have died mid-packet; finish or roll forward its work, never discard silently).
2. If a phase gate is OPEN: present it (plain-language walkthrough first; operator free-text answers are authoritative), record answers in the gate file + STATE, close, continue.
3. Otherwise take the next packet from STATE. Never re-plan closed phases; never re-litigate gate or G-series decisions.

## Binding rules (unchanged from the campaign)

- **The spec wins** — over model memory, over the research reports, over existing code. Every packet implements *named sections*. A discovered spec conflict, gap, or impossibility is NEVER resolved silently: stop the packet, write an S00.9 amendment proposal, present to the operator. Any amendment touching a ⚙ setting re-runs the S18 sweep.
- D1–D10 fixed; **adopt-don't-fork** (never patch adopted code; pin exact versions; `components.lock` + CI lock-gate from the very first dependency [S16]); **no-load-bearing-metered-paths**; subscription-coverage rule.
- Every ⚙ number ships through the settings registry with clamps + audit — never a constant in code [G1 rider 1; S01.10].
- **Real-world facts live-verified at time of use** (current library/engine versions, provider behavior) — never from memory; pins recorded in `components.lock`.
- `Research/` is a closed archive. `Docs/` is read-only. BENCH-REG registered numbers change only via its §17.
- Secrets never committed (`*-api-key.txt` gitignore pattern exists; broker mechanics per S11 when reached).

## Work packets — four-stage pipeline (operator-ratified 2026-07-22; applies from P3-B4-2)

STATE.md holds the current phase's packet queue. At phase entry the coordinator derives packets from S19.5 plus the phase's spec sections (a packet = one worker's worth: readable section set, implementable in one sitting, testable acceptance). TBD-P3 spikes and TBD-BRINGUP measurements attach to their phase per S19.5–S19.6; measurement results are recorded in `P3/measurements/` (G3 Def.8 discipline).

Every packet runs through four fresh-context stages, each launched by the coordinator as a background Agent (subagent_type `claude`), strictly sequential within the packet. The operator starts nothing per-stage. Design basis (STATE directive 2026-07-22): Anthropic's planning/generation/evaluation harness with structured handoff artifacts; "Prompting Claude Fable 5" (fresh-context verifiers outperform self-critique; benign security work can trip Fable's cyber classifiers — the documented remedy is Opus routing); the D3-ratified judge rules (judge ≥ executor; judge ≠ executor).

| Stage | Model | Notes |
|---|---|---|
| 1 Grounding | inherit (Fable) | `model: opus` when the read-first sections are S10/S11-dense (standing rule, memory `fable5-safeguard-false-positive`) |
| 2 Executor | **`model: opus` — always, never Fable** | judge-independence + classifier immunity mid-run |
| 3 Evaluation | inherit (Fable) | `model: opus` from the start on S10/S11-dense packets; lossless Opus relaunch on any classifier trip |
| 4 Finalizing | executor continued (SendMessage) | fresh `model: opus` agent for round 2 or a dead executor |

All stages inherit session effort (max). Fable-facing prompts (grounding, evaluation) state goal + constraints — never step lists (over-prescription degrades Fable output). Executor/finalizer prompts always include the scope guardrail: "Don't add features, refactor, or introduce abstractions beyond what the task requires; do the simplest thing that works well; don't add handling for scenarios that cannot happen; only validate at system boundaries." Every stage prompt includes: "Before reporting, audit each claim against a tool result from this session; never claim done with failing tests."

**Stage 1 — grounding → `P3/briefs/P3-<phase>-<n>.md`** (the handoff artifact and the evaluation rubric). A fresh agent reads the packet's read-first sections IN FULL plus the relevant existing code, then writes and commits the brief: numbered requirements each with its S-ref; seams to respect and the stub for any seam whose phase hasn't come; ⚙ settings to consume by registry name; files expected to change; adopted components touched; the acceptance headline decomposed into a concretely checkable checklist; the CONVENTIONS constraints that bind this packet. The brief must be self-contained: executor and evaluation work from it plus the cited sections, not from chat history.

**Stage 2 — executor.** A fresh `model: opus` agent shaped like:

> You are building Sinet v0 — packet **P3-<phase>-<n>: <title>**. The binding contract is `Spec/core-architecture-v1.md`; your grounded brief is `P3/briefs/P3-<phase>-<n>.md` — read it first, then the sections it cites, plus `P3/CONVENTIONS.md`. Implement exactly what the brief and sections specify — nothing beyond the packet scope; no new dependency without a `components.lock` entry; every ⚙ value via the settings registry, never a constant. Write production-grade code per CONVENTIONS (no research-narration comments). Don't add features, refactor, or introduce abstractions beyond what the task requires. Acceptance: every item on the brief's checklist; `go test ./...` (and the frontend test command once web/ exists) fully green — run them yourself; audit every claim against a tool result. Commit as `P3-<phase>-<n>: <title> (S## refs)`. Final message ≤12 lines: what shipped, test evidence, any deviation/blocker — never claim done with failing tests.

**Stage 3 — evaluation** (never the coordinator reviewing inline; never the executor). A fresh agent (model per the table) gets the brief path, the packet's commit range, and the executor's report:

> You are the evaluation agent for P3-<phase>-<n>. You did not write this code; your job is to find what is wrong with it. Verify the diff (commit range <range>) against `P3/briefs/P3-<phase>-<n>.md` and the spec sections it cites — read both yourself; do not trust the executor's report. Check EVERY item on the brief's checklist. Hunt for: contradictions with the spec text; behavior invented outside the named seams; ⚙ values as code constants; unpinned or undeclared dependencies; modifications to adopted code; test gaps, weakened assertions, silent skips. Run the build and the full test suite yourself. Report EVERY finding, including uncertain and low-severity ones — do not filter for importance; the coordinator triages downstream. Per finding: file:line, the claim, the evidence, confidence, severity. End with a verdict line: PASS (nothing above nit) or FAIL (numbered findings).

**Stage 4 — triage + finalizing (the drain).** The coordinator triages the evaluation findings: false positives dropped with a logged reason, the remainder numbered [F1..Fn]. No survivors → land. Otherwise round 1: SendMessage the numbered list to the executor agent — apply, re-run the full battery, report per finding — then the evaluation agent re-checks (SendMessage). Round 2 (findings survive, or the executor context is gone): a fresh `model: opus` finalizer gets brief + diff + findings history. Hard cap two rounds; after that the coordinator implements the remainder inline and records it in STATE. Never silently accept, never loop endlessly.

- **Parallelism:** stages are strictly sequential within a packet. Two independent packets may overlap only with worktree isolation; never two writers on one path. While a stage runs, the coordinator may land finished work or prepare the next packet/gate — do not idle-poll.

## Landing checklist (coordinator, every packet before `done`)

- Evaluation verdict PASS (or every surviving finding drained and re-checked).
- Coordinator re-runs build + full test suite locally — green, no skips introduced; `components.lock` gate passes.
- Spot diff review: nothing contradicts the spec text; XREF'd behavior lands behind the named seam (stub if its phase hasn't come), never invented inline; ⚙ values registry-routed.
- STATE updated (packet → done, log line; brief committed); commit; push after milestone commits.

## Phase gates (B0 → B6)

When a phase's packets are done: write `P3/gates/B<N>-report.md` — what shipped, test/conformance evidence, literal demo steps, measurements taken/due, deviations — commit, then present plain-language (free-text answers authoritative). Operator approval opens the next phase. Batch that phase's operator hands-on items at its gate. **B2's gate is the walking-skeleton demo**: intake → execute → checkpoint → receipt live on this machine.

## Failure ladder

- A stage agent dies / returns junk → relaunch that stage once with the failure noted; twice → the coordinator performs that stage inline and logs the deviation (an inline evaluation must still be a fresh full re-read against the brief — never a rubber stamp).
- Session/usage limit → STATE is current (you update it before and after every step); stop clean; any next session resumes via this skill.
- Spec ambiguity → coordinator resolves only if one reading is clearly implied by the text (log the reading in STATE); otherwise operator question.
- Spec defect/conflict → amendment path (Binding rules), never silent divergence.

## Hard boundaries

No force-push. No edits to `Docs/`, `Research/` archives, or registered benchmark numbers. Adopted components are never modified. Host-level system changes (new systemd system units, sysctls, packages) follow the operator's global safety gates — propose, get approval at the gate or in-session, then apply.
