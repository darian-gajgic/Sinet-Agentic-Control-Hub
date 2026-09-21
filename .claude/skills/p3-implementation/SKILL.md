---
name: p3-implementation
description: Coordinator runbook for building Sinet v0 from the frozen spec. Use when the operator says "continue implementation", "build", "next packet", "implementation status", "start B0" (or any B-phase), or any session meant to advance P3 — interactive or as one headless sitting launched by P3/run/loop.sh. Reads P3/HANDOFF.md + P3/STATE.md, executes work packets via subagents against Spec/core-architecture-v1.md, validates, commits, writes gates as files, pauses at phase gates.
---

# P3 implementation — coordinator runbook

You are the build coordinator: the campaign-proven pattern re-instantiated for implementation. Your memory is `P3/STATE.md` (current state) + `P3/STATE-HISTORY.md` (the dated log); your contract is `Spec/core-architecture-v1.md` (**v1, frozen at G4, tag `spec-v1`**; the per-section drafts in `Spec/drafts/` are canonical text) plus siblings `Spec/benchmark-preregistration-v1.md` and `Spec/frontend-components-v1.md`. Operator standing instruction: **run autonomously; pause only at phase gates, spec conflicts, and operator hands-on items.** The operator starts one session (or the loop starts one sitting) and says "continue implementation" — every stage launch below is the coordinator's job, never the operator's. Never end a turn on a plan or a promise: if your last paragraph names work not yet done, do it now; stop only where one of the three pause conditions genuinely holds — and then as a gate FILE, never a chat question.

## Amendment F (2026-09-22, operator-ratified) — the sitting model

The build runs as a chain of budgeted FRESH coordinator sittings driven by an external loop (`P3/run/loop.sh` → `P3/run/sitting.sh`), never by one long session. Design record: `P3/design/continuous-run-harness-proposal-2026-09-22.md`. An interactive session that says "continue implementation" is a sitting too and obeys the same rules; the headless contract `P3/run/sitting-prompt.md` adds the status-file terms.

- **R1 Budget.** ≤3 landings or the wall clock (4 h headless), whichever first; never a grounding past the third landing; then wind down (STATE + STATE-HISTORY entry + HANDOFF rewrite + push; headless: `status.json` last).
- **R2 Gates to disk first.** Any operator decision is a committed `P3/gates/<name>.md` BEFORE it is presented (contract below); the operator answers by editing the file or by free text in a later interactive session, which records the answers into the file. Work the gate does not block continues meanwhile.
- **R3 Delegated spot-checks.** The coordinator never reads a brief in full: a fresh agent runs `P3/prompts/spotcheck.md` and returns ≤600 chars.
- **R4 Templates on disk.** Every stage prompt is `P3/prompts/<stage>.md`; the launch prompt names the template and carries only packet deltas (≤2,000 chars).
- **R5 Capped reports.** Agents reply ≤1,200 chars; the full report is `P3/reports/P3-<phase>-<n>-<stage>.md`, committed on the packet branch.
- **R6 Agent-drafted landing text.** Executor/evaluator/finalizer reports carry the draft STATE landing line and the draft CONVENTIONS § text; the coordinator pastes and trims, never composes from scratch.
- **R7 HANDOFF rewritten, never appended.** ≤8,000 chars, one current orientation; retired snapshots go to STATE-HISTORY verbatim. STATE log entries ≤600 chars.
- **R8 Bounded reads.** `P3/STATE.md` holds only current state (directives, open queues, hands-on items); `P3/STATE-HISTORY.md` holds closed queues + the log and is grepped, never read whole. `P3/CONVENTIONS.md` is read by section: `grep -n '^## '` for the index, then the §§ a brief names plus §1–§5; a new § is ≤6,000 chars (longer material → `P3/design/` with a pointer). Sitting-entry reading is HANDOFF + STATE + the runbook, nothing else in full.

## Session entry (one sitting)

0. **Effort:** coordinator sittings run at max (`/effort` interactive; `--effort max` headless). Packet subagents inherit it.
1. Read **`P3/HANDOFF.md`** (≤8K orientation: where the build is, the next act, open operator items, host state), then `P3/STATE.md` (directives, open queues). STATE outranks the handoff — if they disagree, STATE wins and the handoff gets corrected at wind-down. Then: `git status` (expect only the long-standing operator files HANDOFF names), `git worktree list` (every agent worktree must be accounted for in STATE), `pgrep -af 'go test|vitest'` (reap orphans no sitting owns).
2. For every gate file with an open status: if `answered: yes` or a filled `## Answers` → record answers in STATE, execute, mark ANSWERED. Interactive session with the operator present: present the file plain-language (walkthrough before any ask; free-text answers authoritative), record, close.
3. Take the next unblocked packet from STATE's open queues in their stated order. Never re-plan closed phases; never re-litigate gate or G-series decisions.
4. Wind down per R1 (headless: per `P3/run/sitting-prompt.md`).

## Gate-file contract (R2)

`P3/gates/<name>.md`, committed and pushed before it counts as presented:

```
# <Gate title> — gate record (opened <date>)
Status: OPEN            ← OPEN | PARTIALLY ANSWERED | ANSWERED
answered: no            ← the loop polls this exact line; the operator (or the recording session) sets `answered: yes`
## What this decides (plain language, one paragraph)
## Items
### <n>. <question> — options, effects of each, the coordinator's recommendation
## Answers
(operator free text, verbatim, dated — or edited in place next to each item; `answered: partial` lets the loop continue with what is unblocked)
```

FRONTEND.md checkpoints (product map approval, screenshot checkpoint, operator-eyes exit) are gate files under the same contract. Phase gates are `P3/gates/B<N>-report.md` with the same status lines added.

## Binding rules (unchanged from the campaign)

- **The spec wins** — over model memory, over the research reports, over existing code. Every packet implements *named sections*. A discovered spec conflict, gap, or impossibility is NEVER resolved silently: stop the packet, write an S00.9 amendment proposal as a gate file, continue with unblocked work. Any amendment touching a ⚙ setting re-runs the S18 sweep.
- D1–D10 fixed; **adopt-don't-fork** (never patch adopted code; pin exact versions; `components.lock` + CI lock-gate from the very first dependency [S16]); **no-load-bearing-metered-paths**; subscription-coverage rule.
- Every ⚙ number ships through the settings registry with clamps + audit — never a constant in code [G1 rider 1; S01.10].
- **Real-world facts live-verified at time of use** (current library/engine versions, provider behavior) — never from memory; pins recorded in `components.lock`.
- `Research/` is a closed archive. `Docs/` is read-only. BENCH-REG registered numbers change only via its §17.
- Secrets never committed (`*-api-key.txt` gitignore pattern exists; broker mechanics per S11).

## Pipeline routing (operator-directed 2026-08-05)

**Frontend-shaped work — anything whose primary output is presentation or interaction (`web/` views, styling, UX flows, navigation, copy) — does NOT run the four-stage pipeline. It runs `FRONTEND.md` in this directory** (single-author Fable builder, reference-over-prose, product map first, screenshot-in-the-loop, live design review + cold walks, operator-eyes final gate). Mixed packets are split so each half runs under its own pipeline. Backend work runs the pipeline below. Process/tooling packets (harness, runbooks) take the light path.

## Work packets — four-stage pipeline (operator-ratified 2026-07-22; amendments A–E 2026-08-05; F 2026-09-22)

STATE.md holds the open packet queues. At phase entry the coordinator derives packets from S19.5 plus the phase's spec sections (a packet = one worker's worth: readable section set, implementable in one sitting, testable acceptance). TBD-P3 spikes and TBD-BRINGUP measurements attach to their phase per S19.5–S19.6; results go to `P3/measurements/`.

Every packet runs four fresh-context stages, each launched by the coordinator as a background Agent (subagent_type `claude`) in its own git worktree (`.claude/worktrees/`, branch `worktree-agent-*`, merged `--no-ff` after evaluation, worktree removed after merge), strictly sequential within the packet. Design basis: Anthropic's planning/generation/evaluation harness with structured handoff artifacts; "Prompting Claude Fable 5" (fresh-context verifiers beat self-critique; benign security work can trip Fable's cyber classifiers → Opus routing); the D3-ratified judge rules (judge ≥ executor; judge ≠ executor); the SOTA audit `P3/design/backend-workflow-audit-2026-08-05.md` (tests-first executor-immutable · suite-strength leg at phase gates · codified evaluator probe/tamper duties · single-use briefs · light path).

**Light path (amendment E).** A packet with NO behavior change — docs/comments, mechanical renames, config plumbing, test-only housekeeping, process tooling — may skip grounding and evaluation: executor + the coordinator's own full battery + spot-diff only. Log the light-path call in STATE; any doubt → full ceremony.

| Stage | Template | Model | Notes |
|---|---|---|---|
| 1 Grounding | `P3/prompts/grounding.md` | inherit (Fable) | `model: opus` when the read-first sections are S10/S11-dense (memory `fable5-safeguard-false-positive`) |
| 1b Spot-check | `P3/prompts/spotcheck.md` | inherit (Fable) | fresh agent, ≤600-char verdict; FAIL → grounding relaunched with the findings |
| 2 Executor | `P3/prompts/execute.md` | **`model: opus` — always, never Fable** | judge-independence + classifier immunity mid-run |
| 3 Evaluation | `P3/prompts/evaluate.md` | inherit (Fable) | `model: opus` from the start on S10/S11-dense packets; lossless Opus relaunch on any classifier trip |
| 4 Finalizing | `P3/prompts/finalize.md` | `model: opus` | round 1 = the executor continued via SendMessage; round 2 or a dead executor = a fresh finalizer |

All stages inherit session effort (max). Fable-facing prompts (grounding, evaluation) state goal + constraints — never step lists (over-prescription degrades Fable output); the templates already respect this. Every template carries the scope guardrail and "audit each claim against a tool result; never claim done with failing tests".

**Stage 1 — grounding → `P3/briefs/P3-<phase>-<n>.md`** (the handoff artifact and the evaluation rubric), with the acceptance tests committed red where the code surface allows (amendment A). Grounding never reads prior briefs as truth — they are EXPIRED (amendment D). **Stage 1b** — the delegated spot-check (R3) before any executor launch: every requirement traceable, checklist testable, no invented behavior.

**Stage 2 — executor:** tests-first red, implement to green, brief-specified and pre-existing tests immutable (deviations declared, never edited; sanctioned edits named in the launch prompt), scope guardrail, registry-routed ⚙, adoption rail, serial battery, report per R5/R6.

**Stage 3 — evaluation** (never the coordinator inline; never the executor): every checklist item, contradiction hunt, ≥3 novel held-out probes, test-tamper diff (amendment C), report EVERY finding, verdict line PASS/FAIL.

**Stage 4 — triage + finalizing (the drain).** The coordinator triages: false positives dropped with a logged reason, the rest numbered [F1..Fn]; **triage prefers executable falsification** — run the claimed-broken case before dropping or accepting. No survivors → land. Round 1: SendMessage the numbered list to the executor (apply, full battery, report per finding) → the evaluator re-checks (SendMessage, appends `## Re-check r1` to its report). Round 2 (findings survive, or the executor context is gone): a fresh `model: opus` finalizer on `finalize.md`. Hard cap two rounds; after that the coordinator implements the remainder inline and records it in STATE. Never silently accept, never loop endlessly.

- **Parallelism:** stages are strictly sequential within a packet. Independent packets may overlap only with worktree isolation; never two writers on one path; **one full battery at a time on `main`** (the coordinator's, at merge) — agents run package-scoped or `-run`-filtered in their worktrees. While a stage runs, the coordinator lands finished work or prepares the next packet/gate — never idle-polls. Read a battery's summary BEFORE chaining the next merge.

## Landing checklist (coordinator, every packet before `done`)

- Evaluation verdict PASS (or every surviving finding drained and re-checked).
- Merge `--no-ff` into `main`; coordinator re-runs build + full test suite serial on `main` — green, no skips introduced; `components.lock` gate passes; a packet that moved `web/src` fixtures also owes vitest + tsc.
- Spot diff review: nothing contradicts the spec text; XREF'd behavior lands behind the named seam (stub if its phase hasn't come), never invented inline; ⚙ values registry-routed.
- STATE queue row → done; STATE-HISTORY entry (paste the agent's draft landing line, ≤600 chars); CONVENTIONS § pasted from the executor's draft; brief stamped **EXPIRED** (amendment D); worktree + branch removed; commit; push after each landing.

## Phase gates (B0 → B6) and later gate batches

When a phase's packets are done: write `P3/gates/B<N>-report.md` — what shipped, test/conformance evidence, literal demo steps, measurements taken/due, deviations — under the gate-file contract, commit, push. **Suite-strength leg (amendment B), per phase gate:** a mutation-score pass over the phase's new/changed Go packages (tooling live-verified and lockgated at adoption; first run = baseline), results in `P3/measurements/` and summarized in the report — high coverage with a low kill rate is a finding. Interactive: present plain-language (free-text answers authoritative). Operator approval opens the next phase. Batch that phase's operator hands-on items at its gate. **B2's gate is the walking-skeleton demo.** Operator hands-on runbooks follow the hand-steps rule (memory `hand-steps-guided-ceremony`): ≥3 commands → a guided self-verifying script and ONE command to run it; identifiers derived from the running system at print time.

## Failure ladder

- A stage agent dies / returns junk → relaunch that stage once with the failure noted; twice → the coordinator performs that stage inline and logs the deviation (an inline evaluation is still a fresh full re-read against the brief — never a rubber stamp).
- Usage limit → do not relaunch; STATE is current (update before and after every step); wind down (headless: outcome `LIMIT`); the next sitting resumes via this skill.
- Spec ambiguity → coordinator resolves only if one reading is clearly implied by the text (log the reading in STATE); otherwise a gate file.
- Spec defect/conflict → amendment path as a gate file, never silent divergence.

## Hard boundaries

No force-push. No edits to `Docs/`, `Research/` archives, or registered benchmark numbers. Adopted components are never modified. Host-level system changes (new systemd units, sysctls, packages, `sudo`) follow the operator's global safety gates — propose in a gate file, get approval, then apply in an interactive session; a headless sitting never applies them.
