# P3 handoff — last rewritten 2026-08-12 (builder's final verification in flight): **THE CHECKPOINT-3 CAMPAIGN IS CODE-COMPLETE — P3-RW-2..RW-9 ALL LANDED + PUSHED (tree `274bf1c`); the builder is verifying the last two fixed legs on a fresh world; then CHECKPOINT 3 = the operator completes the give-work journey.**

**Read this first, then `P3/STATE.md`.** This file is a *snapshot*; **`P3/STATE.md` is the single source of truth and outranks anything here.** If they disagree, STATE wins and this file gets corrected.

Authority order: **`Spec/core-architecture-v1.md` (frozen v1 + A1–A10; drafts canonical)** → BENCH-REG (signed) and FC-v1 → `P3/CONVENTIONS.md` (§1–§55) → **frontend work: `.claude/skills/p3-implementation/FRONTEND.md`** → **`P3/design/product-map.md` v3 (APPROVED)** → `P3/design/rework-checkpoint2-findings-2026-08-06.md` (P1–P4) → `P3/STATE.md` → this file. `Research/` closed archive; `Docs/` read-only.

---

## 1. Where the build is (2026-08-12, all pushed through `274bf1c`)

**Session 2 landed EIGHT backend packets, all four-stage validated (every brief EXPIRED):**
- **RW-2** projects HTTP family (drain: verified in-flight pre-read + torn-heal; path-free 409 store conflict; real onboardDoor tests).
- **RW-3** pre-approval project attribution — migration **0022** pin-edge view; money follows attribution; a pinned task serves its project from birth.
- **RW-4** (light path) demo seed ladder-inert (9 parked-with-asks + 1 completed; no queue rows) + self-updating guard.
- **RW-5** interview survival — fenced `RenewLeaseTx` + `run.LeaseKeeper` (⚙`recovery.heartbeat`'s first consumer) + cursor-gated DEAD; parked never scanned; CONVENTIONS §54.
- **RW-6** (A/B split) fork supersession — `AdvanceDispatched` rebind (lineage-guarded, ask re-point, inherited-gate park); onboard ask rebind; ONE fork-suffix matcher (`metering.StripForkSuffix`, four copies replaced); unroutable dispatch leaves a corpse; roleless runs (incl. `platform.deadman.*`/`platform.advisory.*`) FINALIZE never fork; ladder-terminal queue rows settle; verify cwd rides the producing run (`executeRunID` forward walk).
- **RW-7** onboarding board honesty — born `intake` (Backlog), `done` at approval; migration **0023** registry-join arm in `task_project`.
- **RW-8** the approve verb reaches the operator — the onboard card declares `decision` + one `approve` choice (constant-validated; `Approve *bool`, contradictions refused; choice+draft applies, coordinator-pinned); Inbox wording (no `/api/` in operator prose).
- **RW-9** adapter result-text robustness + intake corpse posture — text-bearing retention (result field wins; never repaired); state-first corpse cut incl. continuation legs; `advance_crashed_recovering` surface code; heal = crash → ladder fork → rebind → re-drive, ≤ one sweep, machine-only.

**Frontend (builder, committed-canon relaunches ×5):** step-2 code leg (`2ae64d7`), door fixes (`efe1bdc`+`6c6c41e`), the checkpoint-3 world walk (`656f959`: 22 PNGs; slow-interview survival PROVEN — 11 min across 3 sweeps; walkthrough copy tells the inert-seed truth; `B6-walkthrough.html` joined the tree builder-maintained). **In flight right now: the final two-leg verification** (Inbox approve button as pixels; the compressed journey on the healed world) + PNG refresh → the WORLD READY line.

**Backend health at `274bf1c`:** 45 pkgs 0 FAIL (skips = the sanctioned tier-R/L + §41 classes); lockgate 38; ⚙ 118/33 (heartbeat now consumed); migrations 0001–**0023**; CONVENTIONS §1–§55; web vitest 710/710 at `2ae64d7`-era tree (builder re-verifies at its commits); escape allowlist EMPTY.

## 2. The next acts, in order

1. **Builder verification completes** → coordinator validates + lands its commit (push) → on WORLD READY:
2. **CHECKPOINT 3 (operator, free-text authoritative):** on the fresh :8483 world — create a project → **approve its onboarding from the Inbox (the new Approve button)** → describe a goal into it (button) → interview at any pace → approve the plan (PIN step-up) → watch the board (project-named from birth) → open the task overlay card. Then autonomous through map §7 steps 3–6 (Inbox → Reviews → insight surfaces → system surfaces).
3. **Exit per FRONTEND.md rule 5** (live design review → cold walks → machine battery → operator click-through), **then the deferred queue:** D6 upgrade ceremony (`sudo -v`, prod-DB backup first; `/usr/local/bin/sinet` is still the 20-July B2-gate binary) → bring-up per S19.6.

## 3. The deferred ledger (do NOT do unless asked / future packets)

Micro-packet candidates recorded in STATE: status-less follow-up tasks + spawn double-insert (`intake/followup.go`); verify-escalation cards' envelope gap (SF1); cancelled-onboarding registry residue (SF2); onboard refusal 500-mapping + double-answer race; RW-9's execute-launch dangle (Enqueue refusal at approval → run stuck `new`, nudge-healable); DEF-4 claim release/promotion (→ the S02.8 serialize-by-deny item); the orphan-store heal gap; N3's crafted-pin cost decoration. Operator-deferred standing items unchanged (GPU/VRAM fleet seam, CPU/RAM host monitoring, move-task-between-projects, the settings-tab clamped-⚙ amendment, the assistant's conversational brain = bring-up).

## 4. Machinery + constraints (two pipelines — do not mix)

- **Frontend:** FRONTEND.md; builder relaunch-with-committed-canon is proven ×5 (transcript eviction is routine — never re-derive, always relaunch on commits+PNGs).
- **Backend:** SKILL.md four-stage + A–E. Session-2 precedent additions: parallel grounding under a red-window HOLD (specs in the brief, no red commits); evaluation re-check by resumed evaluator (SendMessage); coordinator inline landing fixes (comment nits, one-test pins) with logged sanction; deviations adjudicated per-hunk with revert-verification (the RW-9 gold standard: corrected red tests re-run against the pre-implementation tree).
- **Cross-stream:** `web/src/kanban.ts` never moves (Go seed guard parses it); `web/src/fixtures/` writes are backend-only (env-gated); explicit-pathspec staging + index.lock retry; the operator's `B6-clickthrough.sh` edit stays uncommitted.
- STATE discipline unchanged: update before/after every step; coordinator pushes after milestones.

## 5. Invariants (unchanged, load-bearing)

The spec wins; D1–D10 fixed; adopt-don't-fork + exact pins + lockgate (engine pin 2.1.218 — the empty-result flake is handled OUR side; next S03.3 bump re-runs the RW-9 T1–T4 tripwire); every ⚙ through the registry; no load-bearing metered paths; money read never computed; honest absence fails loud; live-verify real-world facts; NO auto-kill; content-vs-telemetry; escape-by-default; capability URLs are secrets; secrets never committed; host changes proposed-then-approved; bundled assets only; AGPL ideas-only; seeded world dev-only by compile boundary; TAILWIND SCANS PROSE; cascade-collision class → verify in the BUILT artifact in real Chrome.

## 6. Host and environment state you inherit

- **Production untouched** — `tailscale serve` → Caddy 127.0.0.1:8481 → unit on 8482; `sinet-control`+`sinet-broker` active; `/usr/local/bin/sinet` = 20-July B2-gate binary until D6.
- The demo world on **:8483** is being `--clean`-reseeded by the builder's verification run on the `274bf1c` binary (migrations 0001–0023).
- Local inference tier installed user-level, NOT running (`~/.sinet-b45`). B5 organs installed, canaries disarmed. AppArmor userns finding still parks confined service-context runs (hardening session).
- Long-standing operator files, never staged: `Presentation/`, `Research/Presentation/`, `Sinet-Logo.jpeg`, `tools/dbpeek/`, the operator's `B6-clickthrough.sh` hunks. Chrome connected this session via the operator's `/chrome`.
- GitHub identity: active `gh` = `darian-gajgic`. Everything through `274bf1c` is pushed.

## 7. Model routing

Coordinator at max effort. Backend: executors/finalizers `opus`; grounding/evaluation inherit Fable (zero classifier trips across the entire session — incl. adapter, recovery and scheduler surfaces). Frontend: builder/reviewers/walkers `fable`. Judge ≥ executor everywhere; independence = fresh context + hazards named in advance; evaluation re-checks by resumed evaluator.
