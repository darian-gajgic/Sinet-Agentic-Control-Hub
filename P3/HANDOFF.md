# P3 handoff — last rewritten 2026-08-11 (mid-session pause at the Chrome gate): **P3-RW-2 LANDED, P3-RW-3 (pre-approval pin attribution, migration 0022) LANDED, builder step-2 CODE leg LANDED — the ONLY blocker is Chrome (operator `/chrome`); then the builder's browser legs, then CHECKPOINT 3 = the operator completes the give-work journey.**

**Read this first, then `P3/STATE.md`.** This file is a *snapshot* to orient a fresh session in two minutes. It goes stale; **`P3/STATE.md` is the single source of truth and outranks anything here.** If they disagree, STATE wins and this file gets corrected.

Authority order: **`Spec/core-architecture-v1.md` (frozen v1 + A1–A10; drafts canonical)** → BENCH-REG (signed) and FC-v1 → `P3/CONVENTIONS.md` (§1–§53) → **frontend work: `.claude/skills/p3-implementation/FRONTEND.md`** → **`P3/design/product-map.md` v3 (the APPROVED product contract)** → `P3/design/rework-checkpoint2-findings-2026-08-06.md` (the 13-finding ledger + process rules P1–P4) → `P3/STATE.md` → this file. `Research/` closed archive; `Docs/` read-only.

---

## 1. Where the build is (2026-08-11, all pushed through `817e7a0`)

- **P3-RW-2 (projects HTTP family): DONE + VALIDATED** (`8f2a8f4`+`0fc74d2`+drain `ef47563`; eval FAIL → six-item drain → independent re-check PASS). All six findings were coordinator-confirmed by execution before draining. Landed behavior: the OQ7 in-flight 200 VERIFIES the task row and falls through to the seam on a torn state (heals); the store-exists refusal is conflict-shaped and path-free (409, no host path on any wire); the real `onboardDoor` has real seam tests (368-line shell test over real store+skeleton+scheduler); member/operator POST directions pinned; never-percent walk shipped. Brief EXPIRED with the eval-report pointer (`P3/briefs/P3-RW-2-eval-report.md` is the record).
- **P3-RW-3 (pre-approval project attribution): DONE + VALIDATED** (red `0b26253` + impl `78e3b64`; eval PASS FIRST pass, 3 nits — 2 comment-nits coordinator-fixed inline sanctioned, N3 accepted with note). Landed: migration **0022** re-creates `task_project` as COALESCE(claims-collapse verbatim, latest-`intake.state` `$.registry.project` pin edge) + `cost_per_project` over it (**money follows attribution** — OQ-(b)=B1) + `runs_task_idx` on EXPLAIN evidence; `redactTaskLineage` at the detail edge; two honest history Descriptions; four `user_version` pins → 22; preview guard → 0023. **A pinned task now serves its project on list/detail/cost from birth — the checkpoint-3 journey reads correctly pre-approve.** OQ dispositions (STATE 2026-08-10): a=0022-view, b=money-follows-attribution, c=indistinguishable wire field, d=board SSE as-is. Brief EXPIRED.
- **Builder step-2 CODE leg: LANDED (`2ae64d7`)** by the fresh-relaunch builder (committed-code-as-canon, third use): `api.projects()`/`api.project(id)` typed against the FINAL RW-2 shapes + `Projects.tsx` registry consumption (cards, detail modal, create door; Describe-a-goal only on ACTIVE entries); sweep exception list CLOSED with dated notes — the whole `/api/projects` family consumed, `declaredButUncalled` empty. Coordinator re-verified tsc + vitest 710/710.
- **Backend health at `817e7a0`:** `go test ./...` 45 pkgs 0 FAIL 0 SKIP; `-race` green (api/shell/project + RW-3's six); lockgate 38; ⚙ 118/33 (no new keys); migrations 0001–**0022**; web vitest 710/710 (31 files) on the landed tree; escape allowlist EMPTY.
- Session-2 process records in STATE: RW-2 triage (all six findings execution-confirmed; OQ7(a) mechanics amendment; the sanctioned internal/project retype), RW-3 cut + grounding (brief `77f8091`, spot-check passed) + the four OQ dispositions, the orphan-store pre-existing observation (NOT drained, logged), N3's crafted-pin cost-row decoration observation (accepted, future hardening).

## 2. The next acts, in order (everything below the first is operator-gated)

1. **Chrome connect (OPERATOR): run `/chrome` in the Claude Code session (or relaunch `claude --chrome`).** Verified NOT connected twice this session — it is the single blocker for act 2.
2. **SendMessage the builder** (it is stopped clean after the code leg; its continuity = committed canon): RW-3 is landed and Chrome is live → run the browser legs: `./P3/gates/B6-clickthrough.sh --clean` reseed of :8483 on a post-0022 binary (the stale world predates the seed fixes AND 0022; the operator's rows there die with the wipe — sanctioned), re-verify the approve leg (the `release-notes` seeded project holds an active `["**"]` W claim — collision is DESIGNED; a fresh project approves clean), full-journey screenshot verification at two widths, checkpoint-3 PNGs to `P3/design/rework-screens/step2/`. On the walk the board must name the pinned project from submit onward (0022's effect — a "(no project)" flash pre-approve is now a DEFECT, not known-expected).
3. **Checkpoint 3 (operator):** they COMPLETE the journey on the reseeded world — create/see a project → describe a goal into it (button) → interview → approve the plan → watch the board → open the task card. Free-text answer authoritative. Then autonomous through map §7 steps 3–6 (Inbox → Reviews → insight surfaces → system surfaces).
4. **Exit per FRONTEND.md rule 5** (live design review → cold walks → machine battery → operator click-through), **then the deferred queue:** D6 upgrade ceremony (`sudo -v`, prod-DB backup first, binary still the 20-July B2-gate build) → bring-up per S19.6 (D2 paid sweep ~$2.39 spent of the ratified lines, local-tier wiring in `~/.sinet-b45`, calibration writes, B6 report §8 standing items).

## 3. Operator-deferred (do NOT do unless asked)

- GPU/VRAM + local-seat fleet-seam wiring; CPU/RAM host monitoring (S00.9; never re-propose unprompted); move-task-between-projects; the settings-tab constants-as-clamped-⚙ amendment (`settings-tab-see-change-everything`).
- The assistant's conversational brain (Jarvis behavior) is a BRING-UP item — the rework fixes its form/flow/styling only.
- The orphan-store heal gap (crash between store-create and Register; pre-existing, pre-RW-2) and N3's crafted-pin cost-row name decoration — both logged observations, future packets only if they bite.

## 4. Machinery + constraints (two pipelines — do not mix)

- **Frontend:** FRONTEND.md (one long-lived Fable author — relaunch-with-committed-canon proven three times; reference-over-prose; screenshot loop; Banned list load-bearing) + P1–P4. The builder is CURRENT and stopped clean; resume via SendMessage, never a fresh launch while its canon holds.
- **Backend:** SKILL.md four-stage with A–E. Session-2 additions to the precedent book: parallel grounding under a red-window HOLD (grounding writes specs into the brief, commits no red tests, executor materializes — used for RW-3, logged); evaluation re-check by fresh agent when the original evaluator is dead; coordinator inline comment-nit fixes at landing (B5-2 NF1 proportionality).
- **Cross-stream constraints:** `web/src/kanban.ts` must not MOVE or rename (Go seed guard reads it); `web/src/fixtures/` writes belong to the BACKEND pipeline (env-gated regeneration only); concurrent agents stage by explicit pathspec only, retry on index.lock.
- STATE discipline unchanged: update before/after every step; commit; coordinator pushes after milestones.

## 5. Invariants (unchanged, load-bearing)

The spec wins; D1–D10 fixed; adopt-don't-fork + exact pins + lockgate; every ⚙ through the registry; no load-bearing metered paths; money read never computed; honest absence fails loud; live-verify real-world facts; no auto-kill; content-vs-telemetry; escape-by-default (allowlist EMPTY; preview iframe = the one raw-HTML channel); capability URLs are secrets; secrets never committed; host changes proposed-then-approved; bundled assets only; AGPL ideas-only; seeded world dev-only by compile boundary; TAILWIND SCANS PROSE; cascade-collision class → verify in the BUILT artifact in real Chrome.

## 6. Host and environment state you inherit

- **Production untouched** — `tailscale serve` → Caddy 127.0.0.1:8481 → unit on 8482; `sinet-control`+`sinet-broker` active; `/usr/local/bin/sinet` = 20-July B2-gate binary until D6.
- The stale seeded world may still serve on **:8483** (predates the seed fixes AND migration 0022) — the builder's `--clean` relaunch replaces it before any walk.
- Local inference tier installed user-level, NOT running (`~/.sinet-b45`). B5 organs installed, canaries disarmed. AppArmor userns finding still parks confined service-context runs (hardening session).
- Long-standing operator files, never staged: `Presentation/`, `Research/Presentation/`, `Sinet-Logo.jpeg`, `tools/dbpeek/`, the operator's `B6-clickthrough.sh` edit + `B6-walkthrough.html`.
- GitHub identity: active `gh` = `darian-gajgic`. Everything through `817e7a0` is pushed.

## 7. Model routing

Coordinator at max effort. Backend: executors/finalizers `opus`; grounding/evaluation inherit Fable (lossless opus relaunch on any classifier trip — zero trips to date, incl. all of session 2). Frontend: builder/reviewers/walkers `fable`. Judge ≥ executor everywhere; independence = fresh context + hazards named in advance.
