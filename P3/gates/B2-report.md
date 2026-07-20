# B2 phase gate — walking skeleton

**Status: OPEN — awaiting the live demo + the operator decisions below.** Written 2026-07-20 by the coordinator at the end of the overnight-autonomous run (operator directive in STATE log). B2's gate criterion per STATE/S19.5: **the walking-skeleton live demo on this machine** plus the host-install batch deferred here from the B0/B1 gates.

## 1. What shipped (4 packets, all validated at full line-read depth, all pushed)

| Packet | Commit | Delivered |
|---|---|---|
| P3-B2-1 | `33e562c` | S05 Task Context Ledger: event-sourced document (six ratified sections, write rules structurally enforced), fresh-context-per-stage assembly with trace manifest + clean-context exception, Claude-lane injection channel per the B1-4 spike (prompt assembly + placed pinned file + SessionStart re-injection hook), S02.4(c) checkpoint seam fill |
| P3-B2-2 | `45d4328` | S06 intake pipeline: Stage-0 triage (P47 rules first, fail-closed high), clarity-driven interview with Clearance, SPEC/PLAN artifact pair (stable keys, dual phrasing, immutable drafts + approved freeze), deterministic spine, critique verdicts (PASS/REVISE/SPEC-DOUBT/TIER-UP), S06.9 approval with ledger pinning + S02.8 write-set claims + delta re-approval with the P-T05-2 measurement hook |
| P3-B2-3 | `616f497` | S07 verification: V0 pre-gates, V1 software check-pack runner (sandboxed, platform-side verdicts), V2 dual-axis judge on the clean-context slice, bounded rework (exact retry package, finding keys, convergence stop rules), 8-category escalation route table **with a planted-defect e2e test per route**, entailment idle + three seed artifacts, **quality-gate ≠ effects-gate proven three ways** (import-graph test + SHIP-over-live-journal + escalation-writes-nothing) |
| P3-B2-4 | `e578f83` | The skeleton: role-run dispatcher (`<task>.intake/.execute/.verify`), stage runner (fresh session per stage, brief-assembled, budget-watched with `context.overflow` proposals), engine-session seams for Planner/Critic/Judge/Rework, claude-cli adapter registered behind the scheduler, receipts purpose-itemized (ceremony/execution/verification), walking-skeleton API surface with S01.9 PIN step-up, fake-engine E2E test of the whole D-line |

Evidence: full battery green at every validation (`go build`, `go test ./...` `-count=1`, `-race` on touched packages, `gofmt`, `go vet`, lockgate 8 entries / **zero new dependencies across all of B2**, no schema migration needed). ~170 tests added across the four packets, including the forced P46 escalation routes and `TestWalkingSkeletonE2E` (request → interview → approval → execute → verify → receipts, fake engine, zero paid calls).

## 2. The gate demo

**`P3/gates/B2-demo-script.md`** — literal numbered steps, expected result per step, ~$0.05–0.30 total paid spend. It exercises intake → plan (real planner+critic sessions) → PIN-stepped approval → execute (real engine, checkpoints with live ledger-revision blocks) → degraded-mode verify (real judge sessions) → purpose-itemized receipts → restart durability.

Note: the optional pre-gate tier-L live smoke was **not run** (the permission prompt was declined during a session crash; not retried on principle). The wiring is proven by the fake-engine E2E; the demo's paid legs are the live proof, with you watching.

## 3. Operator decisions

**D1 — Run the demo.** The gate criterion itself. Pass ⇒ B2 closes, B3 (workforce & memory: S08 + S09) opens.

**D2 — Host-install batch** (everything deferred from B0/B1; each is yes / no / defer-again; none block dev-mode work, but several unlock deferred capabilities):
1. systemd unit set install + enable (`sinet units` output; includes journald cap drop-in) — makes the platform survive reboots/logout for real.
2. First ts.net cert issuance (`P3/gates/B0-cert-steps.md`, 13 steps, hostname `sinet` per A1) — puts the name in CT logs; needed before any household device connects over HTTPS.
3. srt host install + activation smoke (v0.0.66 pinned; `SINET_SRT_PATH`/PATH discovery already wired) — flips confinement composition from native fallback to the ratified adopted path.
4. Egress substrate (nftables default-DROP + host proxy + credential-injection proxy, S11.4/S11.5) — unlocks **confined+authenticated** engine spawns, the utilization-header overlay source, and research tooling. Biggest single unlock; also the B4 obligation trigger (verification profile must stay network-off once this lands).
5. age/sops at-rest broker crypto (replaces dev stdlib AES-GCM store) — pairs naturally with escrow item 6 (paper + off-host copy), which is due before B4's first snapshot push anyway.
6. logind PrepareForSleep inhibitor wiring (D-Bus adoption decision) — pairs with the open suspend-leg probe.
7. `sinet-run@` template install + name-scoped polkit grant (S11.8 Shape B).
Coordinator recommendation: 1+2 now (cheap, reversible, immediately useful), 3+4 as the next dedicated session (they interact), 5 with item 6's escrow, 6+7 whenever the suspend probe happens. Free-text answers per item are fine.

**D3 — Seed-content ratification** (all versioned, operator-editable, strict-JSON loadable): interview taxonomies (software = ClarifyCodeBench 10-type, live-verified from arXiv:2607.00711; generic fallback), the P47 trigger table (11 rules), the software rubric bundle, the golden set (26 cases), the entailment calibration set (10 pairs). Formal 8.3-gate entry re-runs at B3 when S09 lands; this ratification covers their use until then.

**D4 — Readings en bloc:** 49 section-cited clearly-implied readings across the four packets (12+10+14+13), full text in the commit bodies + CONVENTIONS §13–§15 and `e578f83`. Standing offer: walk through any subset in plain language before you ratify.

**D5 — Constants note** (standing settings-tab directive; no action now, recorded for the clamped-S18 amendment): `maxQuestionsPerCard=4`, `v0RegenerationLimit=1`, `convergenceSimilarity=0.9`, launch-domain roster, placeholder lexicon, check class C2, `anthropicContextWindowTokens=200k`, `DefaultModel`, exec toolset, kanban strings, plus B2-1's name/enum constants.

**D6 — Probe homework** (due at this gate per STATE): the `Persistent=` catch-up **suspend leg** (pairs with the battery-drain 1 h unplugged suspend — probe timer is still installed) and the **user.slice freeze/thaw** probe. Both operator-hands-on; can be done alongside the demo or explicitly re-deferred.

## 4. Measurements taken / due

- **Taken at B2:** B1-4 spike results consumed (injection channel wired exactly as spiked); reboot-survival + catch-up reboot leg (pre-B2, both spec-conform).
- **Recorded deferral:** `P3/measurements/2026-07-20-verification-seeds-deferral.md` — entailment TPR/TNR + judge-as-classifier calibration + length-bias all need the **B4 local tier** (pre-registered bars before anything gates unsupervised).
- **Opens after D2.4:** the anthropic unified-header utilization observation window (S19.6) — its source is the injection proxy.

## 5. Deviations & known-degraded (all deliberate, all recorded)

- B2-4's packet agent was killed twice by host session crashes; the coordinator rolled its surviving work forward and completed the packet inline per the failure ladder (full line-read validation; commit `e578f83` notes it).
- PreCompact *blocking* available but unwired (containment stays primary, S05.7); recitation ⚙ unconsumed (no per-turn channel exists — B1-4 spike; arrives with S04/B3); CheckPacks empty (software verify fails loud until the S13 registry pack, B4); research counters record UNVERIFIABLE-HERE (until D2.4); Utility duty nil (pinned local, S12/B4); judge honestly self-family-flagged (S08 selection, B3).
- Z.AI calibration (operator item 1) still parked — no Z.AI lane exists yet.
