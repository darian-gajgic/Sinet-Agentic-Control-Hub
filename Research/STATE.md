# Campaign state — live pointer (terse layer)

> Any session resuming the campaign: read this file first, then `CAMPAIGN.md`, then follow `.claude/skills/research-campaign/SKILL.md`. Update this file before and after every action. This is the single source of truth for **live coordination state**.
>
> **Full operational detail + the chronological session log live in [`STATE-ARCHIVE.md`](STATE-ARCHIVE.md)** — read it only when auditing a specific past decision. Keeping this layer terse is deliberate: it lets a Fable 5 session resume without loading security-research operational prose into context (see the Safeguard note).

**Campaign status:** **P1 (research waves) COMPLETE — 17/17 topics; gates G0–G3 CLOSED. P2 (core-architecture spec) APPROVED. P2-entry spike battery (S1–S5) COMPLETE, validated, committed, pushed (2026-07-18).** The autonomous-research portion is finished; the remaining P2 work is interactive and non-security.

**Safeguard note:** the security-flavored spike work tripped Fable 5's intentionally-broad dual-use safeguard (false positive; auto-falls-back to Opus 4.8, lossless). Mitigation in force: this live layer is kept terse + neutral, and any task that must deep-read a security report (09 metering, 10 sandboxing, the spike reports) is delegated to an **Opus-pinned subagent** returning a neutral deliverable. See the coordinator skill (§Security-content isolation) and memory `fable5-safeguard-false-positive`.

**Next action (all INTERACTIVE — operator-driven):**
- (a) **Benchmark pre-registration session** (D2.8) — report 12 §4.6 is the draft package: freeze per-domain metrics, gate mins (⚙20 pairs / P≥0.90), done-directly formula text; signed commit.
- (b) **Frontend workshop** — binding component picks incl. settings-form renderer (RJSF vs JSON-Forms); report 14 §3.3 shortlist + R14-OQ4.
- (c) Create `Spec/`, draft `core-architecture-v1.md` per the G3 memo's section↔report map; per-section workshops consuming reports 01–17 + spikes. **Raise R06-OQ2 native micro-fanout when the adapter-spawning section is drafted** (operator standing request). G4 reviews the finished spec; only the operator ends the research phase.

**Operator action items still open (whenever convenient):**
1. **Z.AI dashboard prompt-unit calibration** (R09-OQ4 residual) — 5-step recipe in the P2-S1 report §Blocked items.
2. **ts.net hostname pick** (G3 Def.5 — bland + permanent, before the first cert).
3. **Deferred host probes** needing root/reboot/suspend — batched in the P2-S2 report §Deferred (reboot-survival + one suspend session: user.slice freeze/thaw + R09-OQ7 `Persistent=` catch-up).
4. **Week-one push drill** (G3 Def.6, at first deploy).

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
**Last updated:** 2026-07-18 — STATE.md slimmed to a terse live layer; full history archived verbatim to STATE-ARCHIVE.md; Opus-delegation rule for security reports added to the coordinator skill.
