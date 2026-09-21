# P3 handoff — rewritten 2026-09-22 (harness sitting: H-0 built, H-1 built; H-2 bring-up is the operator's next act)

Read this, then `P3/STATE.md` (current state, outranks this file), then follow `.claude/skills/p3-implementation/SKILL.md` (amendment F). History = `P3/STATE-HISTORY.md` (closed queues, dated log, retired snapshots): grep it, never read it whole. This file is rewritten at every wind-down, never appended (rule R7, ≤8K).

## 1. Where the build is

- **Product:** B0–B6 closed; the UI rework, the planning rework (GF), and the LN lane campaign all landed. The task-quality/deliverable-surface queue (TQ/SIT) is executing: nine packets landed 2026-09-17/18 through TQ-7 (CONVENTIONS §80, amendment A16 applied).
- **In flight (worktrees under `.claude/worktrees/`, both clean):** P3-TQ-8 executor COMPLETE at `86946c7` in `agent-a0253ce364edb86af` — its evaluation died at the Fable limit, relaunch it fresh; P3-TQ-4c GROUNDED at `48cf2d6` in `agent-a08ed49b57e61379b` — spot-check owed, executor after TQ-8 merges (rebase onto main; sanctioned edits in brief §R12). P3-SIT-2 (frontend lane) parked at operator checkpoint 1: map v4 `P3/design/product-map.md` §9–§17 + Q1–Q4, to be written as a gate file.
- **Process (this sitting):** the continuous-run harness exists — runbook rules R1–R8, stage templates `P3/prompts/`, reports `P3/reports/`, sitting contract `P3/run/sitting-prompt.md`, gate-file contract, `P3/run/sitting.sh` + `loop.sh` + classifier tests (28/28, stubbed loop runs verified, smoke sitting OK); STATE split into current + history; SOTA research record `P3/design/harness-sota-research-2026-09-22.md` (design confirmed, refinements applied). Nothing has run headless for real yet.

## 2. The next act, in order

1. **H-2, operator, two commands from the repo root:** first `P3/run/install-hooks.sh` (installs the StopFailure/PreCompact/PermissionDenied hooks the coordinator may not write itself), then `tmux new -s p3loop 'P3/run/loop.sh --once'` — watch ONE sitting (attach later with `tmux attach -t p3loop`; stop with `touch P3/run/STOP`). Its first acts: TQ-8 evaluation relaunch (`P3/prompts/evaluate.md`, range `main..86946c7`, `model: opus` if Fable is limited) → on PASS merge `--no-ff`, CONVENTIONS §81 from the executor's draft, brief EXPIRED, serial landing battery (`go test -p 1 -count=1 ./...`; the two GPU-live tests skip while `~/.sinet-b45` is absent), worktree removed; then TQ-4c spot-check → executor → evaluation → landing (§82; apply its ledger note: amend §79/§80's "mints no finding" sentences and the stale `BootstrapPostureNote` wording); then SIT-2 checkpoint 1 as `P3/gates/sit2-checkpoint-1.md`.
2. **H-2(b):** one unattended night; morning review = landings per sitting, transcript size per sitting (`P3/run/log/`), limit handling. **H-2(c):** propose the `systemd --user` unit (linger is already enabled for `sinep`) as a gate file.
3. Then the queue continues per STATE: RUBRIC-V4 (after gate B13), TQ-4b (B14), SIT-3 (B9); the operator's two waiting streams stay separate (LN lane sitting → its 15-item gate batch; B6 gate stream F1a/F1b then W1–W4).

## 3. Open operator items (answer by editing the gate file or in free text to any session)

- `P3/gates/rework-sitting-gate.md`: A3/A4 (A14/A15 veto windows — explanations owed, the next sitting writes them into the file), A6 (readings), B9 (preview substrate promotion), B12 (engine pin 2.1.218 → installed 2.1.278), B13 (rubric v4 rider), B14 (browser inside the sandbox); the v4.1 language-question displacement.
- SIT-2 checkpoint 1 + Q1–Q4 (Q1 a `/reviews` index; Q2 try-deliverable line for everyone or operator-only; Q3 default views by kind; Q4 the label "Ask for changes").
- **Was the deletion of `~/.sinet-b45` (local tier + weights, ~170 GB) deliberate?** The local-tier live tests skip while it is absent.
- LN key ceremony `./P3/gates/lane-test-door.sh` (two key pastes) — the only thing that ends the self-family-judging interim (TQ-F9); then the LN gate batch (15 items, script in STATE-HISTORY §Retired HANDOFF snapshot).
- Carried G4 hands-on items 1, 3, 4, 5 (STATE table).

## 4. Machinery (pointers)

Backend = SKILL.md four-stage (grounding → spot-check → opus executor → Fable eval → drain cap 2) in worktrees; frontend = FRONTEND.md (single Fable author on committed canon, live review + cold walks, checkpoints as gate files). **Hard directives:** tests serial (`-p 1`; `-race` one package at a time; one full battery at a time on `main`, agents package-scoped in worktrees); local inference GPU-only via `./P3/gates/local-stack.sh`; paid tests only behind explicit opt-in env; reap orphans after every agent death (`pgrep -af 'go test|vitest'`); ≥8 GiB free before live legs; worktrees removed after merge; briefs single-use (EXPIRED); a backend packet that moves `web/src` fixtures owes vitest + tsc; `web/src/kanban.ts` never moves; the operator's `B6-clickthrough.sh` hunks stay uncommitted; the engine-hook fork-bomb pins (HookCmd) in the harnesses are never removed.

## 5. Host, worlds, counters

- Production untouched (`tailscale serve` → Caddy :8481 → unit :8482; `/usr/local/bin/sinet` = the 20-July binary until D6). Local tier STOPPED; `~/.sinet-b45` absent (question above). **Disk 91–96 % full** — check before anything disk-hungry; the only Timeshift snapshot is `2026-08-21_23-47-22` and it includes `/home/sinep`.
- Chrome extension unpaired after every reboot (`/chrome`); headless Chromium hangs on http on this host — screenshots via Firefox headless `tools/ffshot/` (untracked). The claude CLI auto-updates (2.1.278 now). `notify-send` at `/usr/bin/notify-send`; `Linger=yes` for `sinep`.
- Long-standing operator files, never staged: `Presentation/`, `Research/Presentation/`, `Sinet-Logo.jpeg`, `tools/dbpeek|dbq|driftseed|ffshot|rescanseed/`, `P3/gates/rw10-heal.sh`, the `B6-clickthrough.sh` hunks.
- Worlds under `$HOME` (all stopped; never reseed evidence worlds; fresh walks use fresh dirs): `.sinet-rework-sitting` (:8483, the interview-rework sitting, keep; webshop task `t-3120e8e3d14591d3` in-review), `.sinet-sit2-builder` (:8489, SIT-2 reference copy; its control plane may still run — harmless, `pkill -f sinet-sit2-builder` if unwanted), `.sinet-code-tryout` (disposable after the TQ packets land), `.sinet-lane-test` (door-owned, :8485), `.sinet-rw19-walk` and `.sinet-b6-final` (KEEP, evidence), `.sinet-rewalk-a`, `.sinet-b6-clickthrough`, `.sinet-fefollow2`, `.sinet-exitwalk` (kept), `.sinet-fefollow`, `.sinet-rw19-builder` (deletable).
- Counters: migrations 0001–0025 · lock 40 · CONVENTIONS §1–§80 · go 48 pkgs · vitest 860 · ⚙ 118/33 · amendments A1–A16 · engine pin 2.1.218 (gate B12).

## 6. Authority order and routing

`Spec/core-architecture-v1.md` (v1 + A1–A16; drafts canonical) → BENCH-REG (signed), FC-v1 → `P3/CONVENTIONS.md` (by section) → `FRONTEND.md` → `P3/design/product-map.md` v4 → the walk ledgers under `P3/design/` → `P3/STATE.md` → this file. `Research/` closed; `Docs/` read-only. Routing: coordinator max effort; executors/finalizers `opus`; grounding/evaluation Fable (`opus` when S10/S11-dense; lossless `opus` relaunch on any classifier trip); frontend builder/reviewers/walkers `fable`; headless sittings run on Opus 5 while the Fable limit is active. Judge ≥ executor; fresh contexts; agents finish batteries in the foreground.
