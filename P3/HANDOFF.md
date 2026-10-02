# P3 handoff — rewritten 2026-10-02 (interactive sitting after the 09-22 crash: three landings H-3 · TQ-8 · TQ-4c; everything pushed; no agent worktree left)

Read this, then `P3/STATE.md` (current state, outranks this file), then follow `.claude/skills/p3-implementation/SKILL.md` (amendment F). History = `P3/STATE-HISTORY.md` (closed queues, dated log, retired snapshots): grep it, never read it whole. This file is rewritten at every wind-down, never appended (rule R7, ≤8K).

## 1. Where the build is

- **Product:** B0–B6 closed; UI rework, planning rework (GF) and LN lane campaign landed. The task-quality/deliverable-surface queue (TQ/SIT) is executing: eleven packets landed through **TQ-8** (§81: the judge reads the tree, not the report) and **TQ-4c** (§82: detected rungs are evidence on their own lane; a failed one is a visible note). §79/§80 amended at the TQ-4c landing. Nothing is in flight; both agent worktrees were merged and removed.
- **Harness:** amendment F in force (R1–R8). The first headless sitting (2026-09-22) ran 44 min, landed bookkeeping + the TQ-8 evaluation, then died on a transient API 500; the next sitting recovered everything from git (the dead finalizer's WIP survived on disk). **H-3 landed 2026-10-02** (limits read from the CLI's retry events, update guard with CLI pin 2.1.287, measured packet cap `P3/run/cap`). Research record: `P3/design/harness-sota-research-2026-09-22.md`. Nothing has run unattended overnight yet, by decision: the `main`-boundary question is gate item C2.
- **Counters:** migrations 0001–0025 · lock 40 · CONVENTIONS §1–§82 · go 48 pkgs (full serial battery green at both landings) · vitest 860 (no `web/src` change this sitting) · ⚙ 118/33 · amendments A1–A16 · engine pin 2.1.218 (gate B12; installed CLI 2.1.287).

## 2. The next act, in order

1. **Operator answers three gate files** (free text to any session, or edit the file and set `answered: yes`): `P3/gates/sit2-checkpoint-1.md` (approve the code-review page map; Q1–Q4), `P3/gates/rework-sitting-gate.md` (A3/A4 veto windows, A6 readings, B9 preview substrate, B12 engine pin bump, B13 rubric-v4 rider, B14 sandbox browser), `P3/gates/harness-hardening-gate.md` (C1 evidence-gated PASS, C2 how landings reach `main`, C3 frontend across sittings). Recommendations are in each file; "as recommended" accepts all.
2. **Operator, one command:** `P3/run/install-hooks.sh` — H-3 widened the StopFailure matcher to `rate_limit|usage_limit|.*limit.*`; as of this rewrite the installed `.claude/settings.json` still carries the old `rate_limit|usage_limit` (verify: `jq -r '.hooks.StopFailure[0].matcher' .claude/settings.json`).
3. **Next sitting (interactive "continue implementation", or `tmux new -s p3loop 'P3/run/loop.sh --once'` for the second supervised headless run):** execute gate answers first (H-4 per C1–C3; B12 pin bump; B13 rider; TQ-4b after B14; SIT-3 after B9; SIT-2 fresh author after checkpoint 1), then **P3-TQ-4d** (posture-note copy, light path), then **RUBRIC-V4** (after B13). Unattended nights start only after C2 is decided and H-4's `main` guard exists.
4. Separate operator streams, unchanged: the LN lane sitting (`./P3/gates/lane-test-door.sh`, two key pastes) → its 15-item gate batch; the B6 gate stream (F1a/F1b, then W1–W4).

## 3. Open operator items

- The three gate files above. SIT-2 waits on checkpoint 1; SIT-3 on B9; TQ-4b on B14; RUBRIC-V4 on B13; H-4 on C1–C3.
- **Was the deletion of `~/.sinet-b45` (local tier + weights) deliberate?** Disk went 91 % → 51 %; the two GPU-live tests skip while it is absent.
- Carried G4 hands-on items 1, 3, 4, 5 (STATE table).

## 4. Machinery (pointers)

Backend = SKILL.md four-stage (grounding → spot-check → opus executor → Fable eval → drain, cap 2, then coordinator post-cap) in worktrees under `.claude/worktrees/`, templates `P3/prompts/`, reports `P3/reports/`; frontend = FRONTEND.md (single Fable author on committed canon, checkpoints as gate files). Harness = `P3/run/` (README there; `loop.sh`, `sitting.sh`, `lib.sh`, tests `test-classify.sh` 53 + `test-loop.sh` 43). **Hard directives:** tests serial (`-p 1`; one full battery at a time on `main`; agents package-scoped in worktrees); local inference GPU-only via `./P3/gates/local-stack.sh`; paid tests only behind explicit opt-in env; reap orphans with bracketed patterns (`pgrep -af '[g]o test |[v]itest'` — a plain pattern killed its own caller once); worktrees removed after merge; briefs single-use (EXPIRED); a backend packet that moves `web/src` fixtures owes vitest + tsc; `web/src/kanban.ts` never moves; the operator's `B6-clickthrough.sh` hunks stay uncommitted; HookCmd fork-bomb pins never removed. **Never run an interactive coordinator while the loop is up** (`touch P3/run/STOP`, wait for the sitting to end).

## 5. Host, worlds

- Production untouched (`tailscale serve` → Caddy :8481 → unit :8482; `/usr/local/bin/sinet` = the 20-July binary until D6). Local tier STOPPED; `~/.sinet-b45` absent. Disk 51 %. Timeshift snapshot `2026-08-21_23-47-22` (includes `/home/sinep`).
- CLI 2.1.287 (auto-updates; the harness pins it in `P3/run/cli-version.pinned` and probes on drift). Hooks installed in `.claude/settings.json` (committed). Chrome extension unpaired after reboots (`/chrome`); headless Chromium hangs on http — Firefox headless via `tools/ffshot/` (untracked). `notify-send` works; `Linger=yes`.
- Long-standing operator files, never staged: `Presentation/`, `Research/Presentation/`, `Sinet-Logo.jpeg`, `tools/dbpeek|dbq|driftseed|ffshot|rescanseed/`, `P3/gates/rw10-heal.sh`, the `B6-clickthrough.sh` hunks, `.claude/settings.json.bak.*`.
- Worlds under `$HOME` (stopped; never reseed evidence worlds): `.sinet-rework-sitting` (:8483, keep; webshop task `t-3120e8e3d14591d3` in-review), `.sinet-sit2-builder` (:8489, SIT-2 reference copy), `.sinet-code-tryout` (disposable after the TQ packets), `.sinet-lane-test` (door-owned), `.sinet-rw19-walk`, `.sinet-b6-final`, `.sinet-fefollow2`, `.sinet-exitwalk` (KEEP), `.sinet-rewalk-a`, `.sinet-b6-clickthrough`, `.sinet-fefollow`, `.sinet-rw19-builder` (deletable).

## 6. Authority order and routing

`Spec/core-architecture-v1.md` (v1 + A1–A16; drafts canonical) → BENCH-REG (signed), FC-v1 → `P3/CONVENTIONS.md` (by section) → `FRONTEND.md` → `P3/design/product-map.md` v4 → walk ledgers under `P3/design/` → `P3/STATE.md` → this file. `Research/` closed; `Docs/` read-only. Routing: coordinator max effort; executors/finalizers `opus`; grounding/evaluation Fable (`opus` when S10/S11-dense; lossless `opus` relaunch on any classifier trip); frontend builder/reviewers/walkers `fable`; headless sittings on Opus 5 while the Fable limit is active. Judge ≥ executor; fresh contexts; batteries in the foreground.
