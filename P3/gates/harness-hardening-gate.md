# Harness hardening — gate record (opened 2026-10-02)
Status: ANSWERED 2026-10-07 — every recommendation accepted
answered: yes

## What this decides (plain language)

The continuous-run harness ran its first real sitting on 22 September. It worked (four landings of bookkeeping, one evaluation, one drain started) and then a transient server error killed it 44 minutes in; the next sitting recovered from git with nothing lost. Three mechanical weaknesses it exposed need no decision and are queued as H-3 (deterministic limit detection, a guard against the CLI auto-updating under the loop, a measured instead of guessed packet cap). Three further changes alter how work is judged, how it reaches `main`, and how frontend work survives fresh sittings. Those are yours to decide. Each item below says what changes, what you gain, what it costs, and what I recommend.

## Items

### 1. Evidence-gated evaluation (Anthropic's "default-FAIL" pattern) — recommended: yes

What changes: an evaluator may only write a PASS verdict if a battery wrapper has produced a machine-readable green result for that exact worktree commit; a hook refuses the write otherwise. The spot-check signs the acceptance checklist into the brief before the executor starts, and any later checklist change is a finding. Every triage DROP must record the exact falsification command and its output.
Gain: a PASS can no longer be typed, only earned; the judge's criteria are fixed before the builder sees them; rubber-stamped drops become visible at phase gates.
Cost: one more hook in the project settings (you install it, as before), a small battery wrapper script, slightly longer evaluations.
Options: (a) all three parts; (b) only the hook-gated PASS; (c) leave as is.

### 2. How landings reach `main` — recommended: (a)

Today a headless sitting merges a packet into `main` and pushes it directly. CI runs the full battery on every push.
(a) Keep direct pushes, add two guards: the loop refuses to start a sitting while the last push's CI is red or still running, and every landing is tagged `sitting/<timestamp>` so a rollback is one command. You stay out of the daily path.
(b) Sittings push to a `p3/auto` branch; `main` moves only when you (or a morning session) fast-forward it after a look. Safer boundary, but it puts a human step back into every day, which the harness exists to remove.
(c) Leave as is.

### 3. Frontend work across fresh sittings — recommended: (a)

FRONTEND.md requires one long-lived author for visual coherence; a sitting kills that author every few hours.
(a) The builder keeps a design ledger as it works (decisions, token values, component inventory, screenshot index, open threads); a frontend sitting is one journey slice that must end at a rendered, screenshotted checkpoint, never mid-surface; a continuation sitting starts by diffing its render against the previous screenshots. Cold walks and your eyes stay the final guard.
(b) Frontend journeys run only in interactive sessions with a single long-lived author (the original rule); the loop does backend work only. Coherence preserved, but frontend progress waits for you.
(c) Try (a); if a cold walk fails on a journey that spanned sittings, that journey falls back to (b).

## Answers

(operator, free text, dated — "ok" or "as recommended" accepts every recommendation; then set `answered: yes` above)

## Answers (operator, chat, 2026-10-07 — verbatim, authoritative)

> "I aprove everything. What else do you need from me in order to continue?"

Coordinator reading: a blanket acceptance of every recommendation in this file (the gate-presentation convention: free text is authoritative, nothing is re-asked).

| Item | Answer | Reading + execution |
|---|---|---|
| C1 evidence-gated evaluation | yes, all three parts | **H-4b**: battery wrapper → machine result per worktree commit; a PreToolUse hook refuses a PASS verdict without a green result for that HEAD (operator installs via `install-hooks.sh`); the spot-check signs the acceptance checklist into the brief and later changes are findings; every triage DROP records its falsification command + output. |
| C2 landings → main | (a) | **H-4a (FIRST)**: the loop refuses to start a sitting while the last push's CI run on `main` is red or in progress; every landing is tagged `sitting/<ts>`; direct pushes stay. Unattended nights may start once H-4a is in. |
| C3 frontend across sittings | (a) | **H-4c**: FRONTEND.md amendment — the builder keeps `P3/design/<journey>-ledger.md`; a frontend sitting is one journey slice ending at a screenshotted checkpoint; a continuation sitting starts by diffing its render against the previous screenshots. Cold walks + operator eyes stay the final guard. |
