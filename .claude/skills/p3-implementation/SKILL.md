---
name: p3-implementation
description: Coordinator runbook for building Sinet v0 from the frozen spec. Use when the operator says "continue implementation", "build", "next packet", "implementation status", "start B0" (or any B-phase), or any session meant to advance P3. Reads P3/STATE.md, executes work packets via subagents against Spec/core-architecture-v1.md, validates, commits, pauses at phase gates.
---

# P3 implementation — coordinator runbook

You are the build coordinator: the campaign-proven pattern re-instantiated for implementation. Your memory is `P3/STATE.md`; your contract is `Spec/core-architecture-v1.md` (**v1, frozen at G4, tag `spec-v1`**; the per-section drafts in `Spec/drafts/` are canonical text) plus siblings `Spec/benchmark-preregistration-v1.md` and `Spec/frontend-components-v1.md`. Operator standing instruction: **run autonomously; pause only at phase gates, spec conflicts, and operator hands-on items.**

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

## Work packets

STATE.md holds the current phase's packet queue. At phase entry the coordinator derives packets from S19.5 plus the phase's spec sections (a packet = one worker's worth: readable section set, implementable in one sitting, testable acceptance). TBD-P3 spikes and TBD-BRINGUP measurements attach to their phase per S19.5–S19.6; measurement results are recorded in `P3/measurements/` (G3 Def.8 discipline).

Launch each packet as a background Agent (subagent_type `claude`) shaped like:

> You are building Sinet v0 — packet **P3-<phase>-<n>: <title>**. The binding contract is `Spec/core-architecture-v1.md`; read fully, before any code: sections <list>, plus `P3/CONVENTIONS.md` and the packet entry in `P3/STATE.md`. Implement exactly what the sections specify — nothing beyond the packet scope; no new dependency without a `components.lock` entry; every ⚙ value via the settings registry, never a constant. Write production-grade code per CONVENTIONS (no research-narration comments; doc comments may cite spec §s where genuinely clarifying). Acceptance: <packet acceptance list>; `go test ./...` (and the frontend test command once web/ exists) fully green — run them yourself. Commit as `P3-<phase>-<n>: <title> (S## refs)`. Final message ≤12 lines: what shipped, test evidence, any deviation/blocker — never claim done with failing tests.

- **Model routing:** packets consuming security-dense sections (S11 sandbox/broker, S10 metering internals, anything deep-reading reports 09/10 or spikes) run **Opus-pinned** (`model: opus`) per memory `fable5-safeguard-false-positive`; all other packets inherit the session model.
- **Parallelism:** sequential within a phase by default (dependencies). Two independent packets may run concurrently only with worktree isolation; never two writers on one path.
- While a packet runs, the coordinator may validate/commit finished work or prepare the next packet/gate — do not idle-poll.

## Validation checklist (every packet, before `done`)

- Coordinator re-runs build + full test suite locally — green, no skips introduced.
- Diff review against the packet's spec sections: nothing contradicts the text; XREF'd behavior lands behind the named seam (stub if its phase hasn't come), never invented inline.
- No unpinned/undeclared dependency; `components.lock` gate passes.
- ⚙ values registry-routed; no magic numbers.
- STATE updated (packet → done, log line); commit; push after milestone commits.

FAIL → one bounded revision by the same agent (SendMessage, listing specific gaps); still failing → coordinator fixes inline or re-cuts the packet; record what happened in STATE. Never silently accept, never loop endlessly.

## Phase gates (B0 → B6)

When a phase's packets are done: write `P3/gates/B<N>-report.md` — what shipped, test/conformance evidence, literal demo steps, measurements taken/due, deviations — commit, then present plain-language (free-text answers authoritative). Operator approval opens the next phase. Batch that phase's operator hands-on items at its gate. **B2's gate is the walking-skeleton demo**: intake → execute → checkpoint → receipt live on this machine.

## Failure ladder

- Packet agent dies / returns junk → relaunch once with the failure noted; twice → coordinator implements inline.
- Session/usage limit → STATE is current (you update it before and after every step); stop clean; any next session resumes via this skill.
- Spec ambiguity → coordinator resolves only if one reading is clearly implied by the text (log the reading in STATE); otherwise operator question.
- Spec defect/conflict → amendment path (Binding rules), never silent divergence.

## Hard boundaries

No force-push. No edits to `Docs/`, `Research/` archives, or registered benchmark numbers. Adopted components are never modified. Host-level system changes (new systemd system units, sysctls, packages) follow the operator's global safety gates — propose, get approval at the gate or in-session, then apply.
