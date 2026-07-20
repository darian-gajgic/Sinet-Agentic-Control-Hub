# B2 phase gate — walking skeleton

**Status: OPEN — demo DONE (§6), D2 answered+executed, D1 readings ratified + close-mode chosen + D6 re-deferred (§7); awaiting D3+D4 verdicts (detail walkthrough delivered).** Written 2026-07-20 by the coordinator at the end of the overnight-autonomous run (operator directive in STATE log). B2's gate criterion per STATE/S19.5: **the walking-skeleton live demo on this machine** plus the host-install batch deferred here from the B0/B1 gates.

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

### D2 record (answered 2026-07-20, operator free-text; items 1+3 executed same day)

Operator preamble: the host-untouched posture is lifted — "the host does not need to be untouched." Coordinator explained the rule's origin (operator's own machine-safety gates + the spec's batch-at-gates discipline + early-build reversibility); agreed it was always a deferral, not a prohibition, and this gate is where it ends.

1. **Units — YES, executed.** Binary → `/usr/local/bin/sinet`; system user `sinet`; `sinet-control` + `sinet-broker` installed+enabled+**active** (portpool unit installed, NOT enabled — its mode is still a reserved stub); journald cap drop-in applied; `/etc/sinet/bootstrap.conf` → loopback `127.0.0.1:8482`; production state at `/var/lib/sinet`, watchdog armed, health green, recovery-ladder pass clean. **Found+fixed by the install: P3-B2-fix4** — the broker unit lacked `StateDirectory=` and died read-only under `ProtectSystem=strict`. A stray fresh dev instance holding 8482 (started by the parallel cert session for testing) was SIGTERM'd cleanly; the unit owns 8482 now. **Operator to-do: the production instance starts with an empty `users` table — bootstrap your operator user against 8482 (same two curls as demo steps 5–6).**
2. **Cert — YES; EXECUTED same evening in the parallel session** (full record: `B0-cert-steps.md` §Execution record). Chain live: serve (tailnet-only) → Caddy 2.6.2-14 loopback 8481 (lock entry added; runbook Caddyfile wildcard-bind defect found+fixed) → unit 8482; production-posture checks exact per runbook (health / `authenticated:false`+hint / cookie-less SSE 401). Cert found **pre-issued 2026-07-19 21:27 UTC** (leftover serve config × rename) — item 7 signed 2026-07-20, retroactive cover recorded. Steps 11–12 DONE later same evening — operator `darian` bootstrapped (placeholder-PIN slip caught + rotated via the step-up PIN-set API; full audit trail on the event log), SSE-with-session verified through the chain. Runbook CLOSED; crt.sh observation due within a day.
3. **srt — YES, executed.** `@anthropic-ai/sandbox-runtime@0.0.66` npm-global (root prefix `/usr/local`); package.json verified 0.0.66 post-install; probe now logs "ADOPTED primary composition path". CLI `--version` prints a commander fallback `1.0.0` outside npm context — documented in the lock entry, not pin drift. Engine confinement itself still activates with item 4's proxy.
4. **+ 7. Egress substrate + run@ — coordinator's recommendation adopted**: dedicated hardening session, soon. **NEW MANDATORY ITEM for that session, found at install:** unprivileged user namespaces are **blocked by AppArmor for the systemd service context** on this Ubuntu (probe: available=false as the `sinet` service; functional as the desktop user) — the hardening session must grant the userns profile for the service before confined runs can compose from it.
5. **age crypto — recommendation adopted:** bundled with the escrow act, before B4's first snapshot push.
6. **Inhibitor — recommendation adopted:** wired right after the suspend-leg probe (D6), measured-then-fixed.

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

---

## 6. D1 record — the live demo (2026-07-20, operator present, coordinator-driven)

**Outcome: the walking skeleton stands — proven live on the third run, after the demo caught five real platform defects the fake-engine battery could not.** Operator directive at the gate: coordinator drives everything; the operator observes and answers.

| Run | Result | What it caught | Fix |
|---|---|---|---|
| 1 | FAILED at step 13 | (a) execute sessions had no engine permission grant — headless Write auto-denied, the executor's plea got filed as the deliverable; (b) an all-Unknown judge round computed **SHIP** — undecided criteria died silently (B2-3's pinned "Unknown is no blocker" reading overturned vs S07.7 sinks-only + S06 criterion-never-disappears) | `8f89677` — `execPermissionMode="acceptEdits"` plumbed (consent ≠ boundary, S11.1); `verify.UnknownEscapes` → blocker-per-Unknown into the ratified drain |
| 2 | FAILED in verify | (c) judge replied prose → JSON parse error → whole run **crashed**; (d) recovery forks (`verify.g1`…) unroutable — role parse broke on fork suffixes, ladder burned to tombstone with zero real retries; (e) tombstoned lineage sat under kanban "verifying" indefinitely | `0732d33` — fork-suffix-aware `runRole`; bounded JSON re-ask (`jsonRetryLimit=1`) on planner/critic/judge; derived-not-stored attention kanban |
| 3 | **PASSED** | (f, read-side) the verify CAP-HIT card was invisible in the task view (intake-only ask lookup) | `f7be92c` — view falls back to the task's oldest open ask across all runs; proven live post-restart |

**Run 3, step by step:** boot ✓ (probes, UNCONFINED dev warning, loopback bind) · health ✓ · bootstrap+login ✓ (201/200, 30-day session) · SSE spine ✓ · triage fail-closed high/generic ✓ · interview with 4 real answers → Clearance 62.5 ✓ · force-proceed ✓ · planner+critic (2 bounded REVISE rounds, residuals surfaced as `open_findings`) → approval card with 4 ACs incl. the requester scope constraint as AC-4, full coverage map ✓ · 401 `pin_required` then PIN-approve ✓ · claims row (`haiku.txt`, plan-v2) + `.approved` freeze ✓ · execute: 3 sessions, **haiku.txt genuinely written** (valid 5-7-5, three lines only) ✓ · verify: V0 pass → round-1 REVISE with `unknown=[AC-1,AC-3]` (**the corrected Unknown escape firing live**) → rework round → V0 `diff-empty` caught the no-op revision → one-regeneration bound (S07.2) → **durable CAP-HIT decision card with SLA** (remind 4 h / push 24 h), best-effort revision pinned by sha256 ✓ · kanban `attention` with the card visible ✓ · receipts: purposes ceremony/execution/verification, calls 4/3/3, **all unpriced** (table `empty-v0`), token counts at approximation tier 1 (measured) ✓ · durability: clean stop, restart **across a binary upgrade**, `PRAGMA integrity_check ok`, ladder classifies nothing, read-back identical ✓. 119 events, 10 checkpoints (one per paid call, each with the live ledger-revision block).

The attention ending is the script's sanctioned branch — and the *correct* one: AC-1 (file-exists) is structurally judge-undecidable in the generic family until B4's check packs make it mechanical; the honest terminal is a requester card, which is what happened. Runs 1 and 2 also exercised routes run 3 never hit: the marker-clarification card, a genuine critic **SPEC-DOUBT** catch (a real spec self-contradiction) with `adjust_spec` → re-interview → SPEC-v4, and the S02.5 fork/tombstone ladder end to end.

**Demo-driven readings presented for ratification under D1** (runbook ambiguity rule, logged in STATE):
1. **Unknown escape** (S07.5 + S07.7 + S06): an Unknown AC verdict synthesizes a blocker-class AC-BLOCKER finding (stable key `unknown:AC-n`) into the ratified drain — rework chance, recurrence → convergence → CAP-HIT card; never SHIP, never silent. Supersedes B2-3's narrower no-verify-stamp-only reading.
2. **Derived attention kanban**: a tombstoned run in the lineage derives `attention` at view time (the S02.3 stalled pattern); the rich tombstone-review card remains B0-4's recorded deferral (B5/B6).
3. **Bounded JSON re-ask** (`jsonRetryLimit=1`, house bounce-once) as the S03.4-side handling of prose replies on JSON-contract sessions.

**Follow-up packet proposed — P3-B2-5 (S07.7 resume-in-place):** answering non-intake asks. The CAP-HIT card is durable and visible but not yet answerable via the API ("answering resumes the pipeline in place" — accept_best_effort / revise_with_guidance / cancel application, resume wiring, planted-route e2e). A B2-3 delivery shortfall found at the gate; operator picks: close B2 with P3-B2-5 as the first B3-session item, or hold B2 open until it lands.

**Cosmetic/deferred notes** (recorded, none blocking): approval-card `cost_time` renders "~0.00 USD (API-equivalent)" instead of an explicit UNPRICED word (the unpriced side IS surfaced in `size_note`); planner sometimes emits `structured_kind` with no structured text (dangling marker, zero binding effect); run-3's planner folded force-proceed slot conversions into its own assumption phrasing (deterministic conversion record stays in the ledger; run 1 proved the labeled path); receipt JSON leaks Go field casing (B6 API-shape pass); unknown-escape rework detour on judge-undecidable ACs (disappears for software at B4; acceptable for generic); demo-script steps 5/14 corrected in place (role field mandatory; tier wording); B2-4's "gofmt green" was stale on 3 files (formatted in `8f89677`).

**Spend:** ~29 haiku-class calls across the three runs (incl. 1 wasted on run-2's crashed judge), all subscription-lane, utilization steady at ~36% of the 7-day window, no overage — within the script's posture. Run-1/run-2 state dirs preserved as traces (`~/sinet-demo-run1-failed`, `-run2-failed`); run-3 dir live with the open card.

---

## 7. Gate answers — running record (2026-07-20, gate-close session; operator free-text "D1 ok. D2 ok. D3, D4 more details. D5 do it later.", mapping = the session's presented Decisions 1–5)

- **D1 — RATIFIED.** Demo criterion done (§6); all three demo-driven readings (Unknown escape / derived attention kanban / bounded JSON re-ask) ratified as presented.
- **Close mode — CLOSE B2 NOW, P3-B2-5 first in B3** (presented Decision 2, coordinator recommendation adopted). Executes the moment D3+D4 land. Demo-dir plan: failed-run trace dirs (`~/sinet-demo-run1-failed`, `-run2-failed`) delete at gate close; `~/sinet-demo` (run 3, open CAP-HIT card) kept until P3-B2-5 validates as a live resume-in-place fixture.
- **D6 — RE-DEFERRED with intent** (presented Decision 5, "do it later"): suspend leg pairs with the battery-drain unplugged hour whenever that happens (probe timer stays installed); user.slice freeze/thaw alongside the hardening session; explicitly NOT a B3 blocker; logind inhibitor wiring stays sequenced after the suspend leg (D2.6). The D5 constants note likewise stays parked for the clamped-S18 amendment — both "later" items, neither blocking.
- **D3 + D4 — details requested before ratification; walkthrough delivered in-session** (all five seed files item-by-item; all 49 readings one line each). Verdicts pending — **the gate stays OPEN until these two land.** Note for the record: B2-3's superseded Unknown-handling reading is NOT part of the D4 set — it was replaced live at the demo by D1 reading 1, already ratified above.
