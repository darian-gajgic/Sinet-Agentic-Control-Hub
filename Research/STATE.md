# Campaign state — live pointer (terse layer)

> Any session resuming the campaign: read this file first, then `CAMPAIGN.md`, then follow `.claude/skills/research-campaign/SKILL.md`. Update this file before and after every action. This is the single source of truth for **live coordination state**.
>
> **Full operational detail + the chronological session log live in [`STATE-ARCHIVE.md`](STATE-ARCHIVE.md)** — read it only when auditing a specific past decision. Keeping this layer terse is deliberate: it lets a Fable 5 session resume without loading security-research operational prose into context (see the Safeguard note).

**Campaign status:** **P1 (research waves) COMPLETE — 17/17 topics; gates G0–G3 CLOSED. P2 (core-architecture spec) APPROVED. P2-entry spike battery (S1–S5) COMPLETE. Benchmark pre-registration v1 REGISTERED — signed commit `5fb7082` (2026-07-18), `Spec/benchmark-preregistration-v1.md`; discharges R12-OQ1/OQ2; `Spec/` now exists.** The autonomous-research portion is finished; the remaining P2 work is interactive and non-security.

**Safeguard note:** the security-flavored spike work tripped Fable 5's intentionally-broad dual-use safeguard (false positive; auto-falls-back to Opus 4.8, lossless). Mitigation in force: this live layer is kept terse + neutral, and any task that must deep-read a security report (09 metering, 10 sandboxing, the spike reports) is delegated to an **Opus-pinned subagent** returning a neutral deliverable. See the coordinator skill (§Security-content isolation) and memory `fable5-safeguard-false-positive`.

**Next action (P2 spec drafting — IN PROGRESS):**
- Sections draft as `Spec/drafts/S00…S19` under `Spec/drafts/CONTRACT.md`; `core-architecture-v1.md` is assembled (concatenation, drafts canonical) once all sections pass coordinator review. Tracker below. As drafts return: coordinator review → bounded revision if needed → commit → next launches.
- **S00–S16 ALL COMMITTED (2026-07-19).** Now running: S17 + S18 sweep sections (concurrent). Then S19 (coordinator, last) → assembly (incl. the parked S11.8↔S01.2 run-unit wording alignment) → G4 review memo → present G4.
- **Operator asks (both CLOSED 2026-07-18):** (a) R06-OQ2 native micro-fanout — **keep disabled** (recorded in S03 Open-items box; G1-rider-2 standing reminder discharged). (b) workspace storage — **git-worktree + overlayfs at v0**; loopback-XFS pre-registered upgrade; repartition reserved for host rebuild (recorded in S02.10).
- Done 2026-07-18 (earlier): benchmark pre-registration signed `5fb7082`; frontend workshop picks in `Spec/frontend-components-v1.md`.

## Spec section tracker

Statuses: `pending` → `running` → `saved` → `reviewed` → `committed`. Model: sections consuming security-flagged inputs run Opus-pinned per the isolation rule; others inherit.

| § | Slug | Model | Status |
|---|---|---|---|
| S00 | front-matter | coordinator | committed ✅ (rode in commit 338b79b) |
| S01 | process-architecture-shell | inherit | committed ✅ |
| S02 | durable-state-recovery | opus | committed ✅ (storage DECIDED: overlayfs+worktree) |
| S03 | engines-adapters | opus | committed ✅ (R06-OQ2 CLOSED: keep disabled) |
| S04 | orchestration | inherit | committed ✅ |
| S05 | context-ledger | inherit | committed ✅ |
| S06 | intake-pipeline | inherit | committed ✅ (R03-OQ5/P47 list discharged) |
| S07 | verification-quality | inherit | committed ✅ (rework-cap tension coordinator-resolved: 3 per R04 §4) |
| S08 | workers-composition-routing | inherit | committed ✅ (R15-OQ4 discharged) |
| S09 | memory-knowledge | inherit | committed ✅ |
| S10 | metering-scheduling | opus | committed ✅ |
| S11 | sandboxing-confinement | opus | committed ✅ (G3 Def.7 discharged: Shape B) |
| S12 | local-models-tier | inherit | committed ✅ (R10-OQ4 broker interface bound) |
| S13 | deliverables-review-git-backup | inherit | committed ✅ |
| S14 | observability-evals-watch | inherit | committed ✅ |
| S15 | frontend-api | inherit | committed ✅ (volley-2 draft recovered on disk; coordinator review PASS 2026-07-19 — no relaunch) |
| S16 | adoption-manifest | inherit | committed ✅ |

**Volley-2 pause/resume note (2026-07-18):** the ten S07–S16 agents were paused on operator hold during their reading phase (no files written), then **resumed in place with context intact** on operator "continue" — same agents, same assignments, no work lost.

**Volley-2 outcome (2026-07-18, CLOSED 2026-07-19):** 9 of 10 committed same-day (S07–S14 minus S15, plus S16). The S15 agent died on the session limit, but its **complete 202-line draft had flushed to disk** (21:46, after the failure was recorded) — found as an untracked file at next session start, coordinator-reviewed PASS (full input verification; detail in STATE-ARCHIVE), committed. **Volley 2 fully closed: all 16 numbered sections S00–S16 committed.**

**Pending assembly reconciliation (not blocking):** S11.8 refines S01.2's run-unit realization from `systemd-run` transient `sinet-run-<id>` to a root-installed **template instance** `sinet-run@<id>.service` (so the polkit grant is property-safe). Functionally identical to S01's intent; the coordinator aligns S01.2's wording at assembly. Flagged `[coordinator-draft]` in both S11.8 and its Open-items.

**Remaining after S15:** S17 (known-problems register — consolidates all P-* incl. the ids coined in drafting: P-T03-1..4, P-T04-1..4, P-T10-1..3, P-T15-1..2, P-T09-1..2), S18 (settings-registry index — sweep every ⚙ table, dedup cross-section settings), S19 (v0 boundary + coverage + build-order); then assembly of `core-architecture-v1.md` and the G4 review memo.
| S17 | known-problems-register | inherit | committed ✅ (64 P-* zero-orphan; P-T17-2 restored; R1–R4 tag backfills queued for assembly) |
| S18 | settings-registry | inherit | running (launched 2026-07-19) |
| S19 | v0-boundary-buildorder | coordinator | pending (last) |

**Operator action items still open (whenever convenient):**
1. **Z.AI dashboard prompt-unit calibration** (R09-OQ4 residual) — 5-step recipe in the P2-S1 report §Blocked items.
2. **ts.net hostname pick** (G3 Def.5 — bland + permanent, before the first cert).
3. **Deferred host probes** needing root/reboot/suspend — batched in the P2-S2 report §Deferred (reboot-survival + one suspend session: user.slice freeze/thaw + R09-OQ7 `Persistent=` catch-up).
4. **Week-one push drill** (G3 Def.6, at first deploy).
5. **(Optional, cosmetic)** GitHub "Verified" badge for registration commits: `gh auth refresh -h github.com -s admin:ssh_signing_key` then `gh ssh-key add ~/.ssh/git-signing-ed25519.pub --type signing --title "sinep-git-signing-sinet"`. Local `git verify-commit` already works without it.

**Provisioned:** Z.AI GLM Coding Max key at `zai-api-key.txt` (repo root; gitignored via `*-api-key.txt`, chmod 600, never committed or printed).

## P2-entry spike battery — COMPLETE ✅

All five spikes done, validated (spike checklist), committed, pushed. Operational detail (probe method, measurements, teardown / secret-hygiene records, reconciliation + pause/resume history, task-id tables) is in **STATE-ARCHIVE.md** and in each spike report.

| Spike | Closed | Commit | Report (`Research/spikes/`) |
|---|---|---|---|
| P2-S1 | opencode live-auth conformance + Z.AI calibration (R09-OQ4) + logprob probe (R12-OQ7) | 5702a96 | P2-S1-opencode-live-auth-zai-calibration.md |
| P2-S2 | systemd harvest matrix (R08-OQ8) + host sandbox prereqs (R10-OQ3 read-only) | 22fcc46 | P2-S2-systemd-harvest-host-probes.md |
| P2-S3 | credential-injection per lane (R10-OQ1) + rate-limit header capture (R09-OQ6) | be507c8 | P2-S3-credential-injection-wire-probes.md |
| P2-S4 | serialize-by-deny fallback (R08-OQ6) | 02acd4e | P2-S4-serialize-by-deny.md |
| P2-S5 | engine-lowering (R15-OQ6) | 0ffb493 / f18e82b | P2-S5-engine-lowering.md |

**Conclusion-level headlines** (read the report for detail):
- **S1** — Z.AI exposes no wire-side prompt counter / usage endpoint (prompt-unit stays dashboard-calibrated → operator item 1) and no logprobs (behavioral-eval-only; only the local lane gets the logprob canary); restart-ask-loss reproduced (#36347).
- **S2** — host sandbox prereqs clear (bwrap 0.11.1 + shipped AppArmor profile, Landlock ABI 8). **Storage seam:** project volume is a single ext4, ~91% full, no reflink → CoW workspaces need repartition-or-loopback (a spec decision).
- **S3** — the credential-handling design keeps engine secrets fully outside the sandbox on both lanes; the same wire observation point serves D4 rate-limit capture (R09-OQ6 confirmed).
- **S4** — serialize-by-deny VIABLE on haiku (~+1 turn); reframes the S2 held-process-parking question (parking looks unnecessary — carry-forward caveat: reconfirm on the default worker model).
- **S5** — compiled config can be the only config an engine sees, and native spawning is fully disablable, but it's a **compound** guarantee (one knob per channel).

**Skipped (with reasons):** optional ~$2 live parallel-rate battery — S2's N=7,325 corpus measurement is the stronger estimator; run later only if the operator asks. R09-OQ7 `Persistent=` suspend test — needs a real suspend cycle (operator-assisted, implementation phase).

**Standing follow-ups:** (a) raise **R06-OQ2 native micro-fanout** at the adapter-spawning spec section (operator request). (b) G3 Def.6 push drill = week-one real-device drill at first deploy (P3). (c) v1+ parked list: G3 memo §Follow-ups.

## Gate log

| Gate | Subject | Status | Decision | Recorded in |
|---|---|---|---|---|
| G0 | Launch: pilot vs review vs full Wave A | CLOSED | **Pilot first** — T01 alone, operator reviews before Wave A2 | (this row) |
| G1 | Architecture direction (after Wave A) | CLOSED 2026-07-17 | Package ratified (D1.1-A); $0.50 trivial threshold; Z.AI tier moot (operator holds Max); spikes S1–S3; defaults adopted + 2 operator riders | decisions/GATE-1-architecture-direction.md |
| G2 | Substrate + adoption list (after Wave B) | CLOSED 2026-07-17 | All recommendations adopted; sandbox ratified contingent on host probes; done-directly two-stage + P2 pre-registration; spike battery at P2 entry; Def.1–16 adopted | decisions/GATE-2-substrate-and-adoption.md |
| G3 | Spec-readiness (after Wave C) | CLOSED 2026-07-18 | P2 approved; backend = Go; React 19 + Vite SPA; compose-when-earned + first-N=3; Layer-2 SQL at v0 on the 7B; Def.1–8 adopted | decisions/GATE-3-spec-readiness.md |
| G4 | Spec review → end research phase | pending | — | decisions/GATE-4-*.md |

## Topic board

Statuses: `pending` → `ready` → `running` → `saved` → `committed` → `gated`. **All 17 topics: committed + validated.** Per-report validation detail (line/source counts, spot-checks, verification tallies) is in the STATE-ARCHIVE.md session log and each report.

| T | Slug | Wave | Depth | Status | Report |
|---|---|---|---|---|---|
| T01 | execution-engines-and-adapters | A1 | FULL | committed ✅ | 01-execution-engines-and-adapters.md |
| T02 | agent-loop-and-harness-engineering | A2 | FULL | committed ✅ | 05-agent-loop-and-harness-engineering.md |
| T03 | orchestration-and-multiagent | A2 | FULL | committed ✅ | 06-orchestration-and-multiagent.md |
| T04 | context-engineering | A2 | FULL | committed ✅ | 07-context-engineering.md |
| T05 | intake-planning-spec-pipeline | A2 | FULL | committed ✅ | 03-intake-planning-spec-pipeline.md |
| T06 | verification-and-quality-loops | A2 | FULL | committed ✅ | 04-verification-and-quality-loops.md |
| T07 | durable-state-checkpointing-recovery | B1 | FULL | committed ✅ | 08-durable-state-checkpointing-recovery.md |
| T08 | metering-quota-scheduling | B1 | FULL | committed ✅ | 09-metering-quota-scheduling.md |
| T09 | sandboxing-confinement | B1 | FULL | committed ✅ | 10-sandboxing-confinement.md |
| T10 | memory-and-knowledge-architecture | B2 | FULL | committed ✅ | 11-memory-and-knowledge-architecture.md |
| T11 | evals-observability-benchmark | B2 | FULL | committed ✅ | 12-evals-observability-benchmark.md |
| T12 | deliverables-review-git | B2 | FULL | committed ✅ | 13-deliverables-review-git.md |
| T16 | oss-harvest-validation | B2 | FULL | committed ✅ | 14-oss-harvest-validation.md |
| T13 | platform-stack-architecture | C | LIGHT | committed ✅ | 17-platform-stack-architecture.md |
| T17 | provider-watchlist-and-onboarding-criteria | A1b | FULL (narrow) | committed ✅ | 02-provider-watchlist-and-onboarding-criteria.md |
| T14 | worker-ontology-and-domain-agents | C | FULL | committed ✅ | 15-worker-ontology-and-domain-agents.md |
| T15 | local-models-layer | C | FULL | committed ✅ | 16-local-models-layer.md |

## Report numbering

Reports take the next free `NN` in `Research/` in completion order:

| NN | T | File |
|---|---|---|
| 01 | T01 | 01-execution-engines-and-adapters.md |
| 02 | T17 | 02-provider-watchlist-and-onboarding-criteria.md |
| 03 | T05 | 03-intake-planning-spec-pipeline.md |
| 04 | T06 | 04-verification-and-quality-loops.md |
| 05 | T02 | 05-agent-loop-and-harness-engineering.md |
| 06 | T03 | 06-orchestration-and-multiagent.md |
| 07 | T04 | 07-context-engineering.md |
| 08 | T07 | 08-durable-state-checkpointing-recovery.md |
| 09 | T08 | 09-metering-quota-scheduling.md |
| 10 | T09 | 10-sandboxing-confinement.md |
| 11 | T10 | 11-memory-and-knowledge-architecture.md |
| 12 | T11 | 12-evals-observability-benchmark.md |
| 13 | T12 | 13-deliverables-review-git.md |
| 14 | T16 | 14-oss-harvest-validation.md |
| 15 | T14 | 15-worker-ontology-and-domain-agents.md |
| 16 | T15 | 16-local-models-layer.md |
| 17 | T13 | 17-platform-stack-architecture.md |

## History

Full chronological session log + spike-battery operational detail (probe measurements, teardown / secret-hygiene records, reconciliation notes, pause/resume history, task-id tables) → **[`STATE-ARCHIVE.md`](STATE-ARCHIVE.md)**.

**Operating note:** campaign sessions run at **max effort** (operator instruction 2026-07-16); research subagents inherit it.
**Last updated:** 2026-07-19 — S15 recovered from disk + reviewed + committed → **all of S00–S16 in**. S17 + S18 launched concurrently; then S19 (coordinator), assembly, G4 memo.
