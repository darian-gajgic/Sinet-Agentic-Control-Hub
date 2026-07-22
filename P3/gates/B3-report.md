# B3 gate report — workforce & memory

Written 2026-07-22 by the build coordinator. Phase B3 (S19.5: worker registry + composer battery + routing [S08]; memory/knowledge + write gate [S09]; S04 spawn/helper slice; codor C1/C3 per the 2026-07-20 standing directive) is complete: all six queue packets done and validated at full depth. This gate's approval opens **B4 (deliverables & local tier: S13 + S12)**.

## 1. What shipped (packet → commit)

| Packet | Commit | One line |
|---|---|---|
| P3-B2-5 | `2caa810` | S07.7 resume-in-place: verify-class asks answerable (accept/revise/cancel), pipeline resumes generation-fenced |
| P3-B3-1 | `9e791cd` | S09 memory/knowledge substrate: schema, layer/writer enforcement, scoped FTS5 search, containment, seed governance |
| P3-B3-2 | `e625643` | S08 worker store: guardrail split, per-invocation hash-pinned compile, four-station battery, automation dialect |
| P3-B3-3 | `cb3d53d` | S08.8 deterministic routing + S04 spawn/helper slice + no-fit/gap cards + serialize-by-deny reconfirm |
| P3-B3-4 | `6802f7a` | Codor C1: S05.3 recitation live over the probe-proven PostToolUse valve on the S03.4 airlock + C3 conformance rows |
| P3-B3-5 | `7dba4ef` | S08.6 composer one-shot generation + governed playbook + compose verb e2e + S05.3 stage-split execution |

Plus coordinator fix `019f723` (sandbox tests made hermetic vs host srt state — the established CI-fix pattern).

## 2. Evidence

- **Battery:** `go test -count=1 ./...` — 26 packages green at every packet close and re-run independently by the coordinator each time; `-race` green on every touched package set; gofmt/vet clean throughout.
- **Lock gate:** 9 entries, **zero new dependencies across the entire phase** (six packets, ~7,600 insertions).
- **Conformance:** S09 cross-user-read-must-fail standing test; S04.5 five-MUST helper battery (depth-3, lateral, native-spawn, over-budget refusals + mid-run kill→salvage); S08.2 guardrail-in-file structural reject battery; PreToolUse byte-identity under recitation; error-output-never-becomes-stage-output negative row; interaction-boundary crash rows (restart-with-pending, crash-after-answer — DB-state-only reconstruction).
- **Paid spend this phase: ≈ $0.16 total** — serialize-by-deny reconfirm $0.1304 (stop line $0.50) + PostToolUse probe $0.0271 (stop line $0.10). Everything else fake-engine, $0.
- **Measurements (G3 Def.8 files in `P3/measurements/`):**
  - `2026-07-21-serialize-by-deny-reconfirm-s08.md` — **PASS** on the S08-designated default worker (E1 clean park; E3 6/6 faithful single-call parks; round-trip live) → **TBD-BRINGUP(parallel-gate fallback) CLOSED** (decision 2 below records it in spec text).
  - `2026-07-21-posttooluse-additionalcontext-probe.md` — **PASS** at installed 2.1.216, all three pre-registered expectations; four per-pin canary rows now executable as the `SINET_B3_4=1`-gated live test.
  - Auto-memory containment closed at B3-1 per the B1-4 spike file (decision 2 records it).

## 3. Literal demo steps (dev-mode, $0 — run any or none)

```
cd ~/Sinet-Agentic-Control-Hub
go test -count=1 ./...                                                        # full battery, 26 packages
go test -count=1 -v -run TestNoFitComposeVerbE2E ./internal/stage             # earned no-fit → compose → battery → approval-as-diff → re-route → execute
go test -count=1 -v -run TestStageSplitOverflowE2E ./internal/stage           # overflow → auto-accepted split at checkpoint boundary → successor brief from updated ledger
go test -count=1 -v -run TestRecitationE2EDeliversManifestsAndJournals ./internal/stage   # due recitation → real valve → hash-verified manifest event
go test -count=1 -v -run "TestRestartWithPendingAskResumesFromRowAlone|TestCrashAfterAnswerNeverReasksOrDowngrades" ./internal/adapters   # C3 crash rows
```

Your personal acceptance test remains the B6 UI click-through per your 2026-07-20 directive; these are coordinator-driven evidence, not homework.

## 4. Deviations & carried notes (none unresolved)

- **C5** (advisory copy-aside context peek) deliberately not built — Research/18 §7-C5 discretionary; drops without loss.
- `claudecli.Pin` promoted from the conformance test into the package (same value, same lockstep test) — the composition root consumes it as the S08.1 validation-record key.
- **Seam notes carried forward:** (a) if a future phase ever composes recitation with parkable gated runs, the fires-log full re-read needs consume-on-manifest or S02.5-style dedup (unreachable today — DriveStage bars stage-session parks); (b) S09.6 cited-entry freshness joining the S02.6 fingerprint has no consumer until plans can cite entries → B4 cut; (c) `orchestration.stagger_identical_prefix` deliberately unconsumed (no simultaneous-launch surface exists at v0).
- Cosmetic: stage-side recite-fires reader is lenient on corrupt lines (delivered-* evidence remains on disk; valve-side reader is strict).

## 5. Decisions for the operator

**Decision 1 — engine pin bump 2.1.215 → 2.1.216 (S03.3 deliberate-bump).** Installed `claude` is 2.1.216; the lock pin is 2.1.215. Evidence at 2.1.216 is already strong: the B3-3 reconfirm PASSED on it (serialize-by-deny fully faithful — the E2 favorable drift persists), and the B3-4 probe PASSED on it (PostToolUse channel confirmed). The S03.3 procedure = move `claudecli.Pin` + the lock entry in lockstep, re-run conformance at the new pin, and run the new per-pin canary (the `SINET_B3_4=1` live test, ~$0.03). P-T14-1 mass-revalidation triggers on production workers — none exist yet (composed workers live only in tests), so the bump is cheap now and costs more later. **Recommendation: bump now.**

**Decision 2 — spec-text bookkeeping of the two measurement-closed TBD markers (S00.9 dated changelog entries; drafts below, applied on your approval).**
- Draft entry A: *"2026-07-22 — TBD-P3(auto-memory containment) closed: S09.9 containment implemented and conformance-tested at P3-B3-1 (platform-supplied config-root `memory/` wiped at session start, resume exempt; engine-native memory default-off posture confirmed by the B1-4 spike, result file 2026-07-20). Marker removed from S09.9."*
- Draft entry B: *"2026-07-22 — TBD-BRINGUP(parallel-gate fallback) closed: serialize-by-deny reconfirmed PASS on the S08-designated default worker at engine 2.1.216 (P3-B3-3, result file 2026-07-21; B1-4 spike-1 PASS-PROVISIONAL made final). S03.4 fallback stands ratified; GateFallback detector retained as a dormant per-pin canary."*
- Mechanics: dated entries land in the S00.9 changelog (drafts + assembled spec), the two TBD markers come off S09.9/S03.4. No ⚙ touched → no S18 sweep.

**Decision 3 — v0 duty map ratification.** All paid seats currently `claude-haiku-4-5` (coordinator/planning/execution/verification classes alike), subscription-coverage-bound with the metered list EMPTY (D5: pressure never spends dollars). The utility seat stays absent until the S12 local tier lands at B4. This is the routing default every stage now dispatches through (the B2-4 DefaultModel stub is retired). **Recommendation: ratify as-is for v0; revisit seats when S12 duty aliases arrive.**

**Decision 4 — composer playbook seed ratification (S09.10: house-scope objects need operator approval).** Seeded at B3-5 as governed entry `seed-composer-playbook` (provenance says "pending — B3 gate"; content in `internal/worker/playbook.go`, ~55 lines of markdown). It steers the composer with the S08.5/S08.6 ratified practice: one-shot contract; specialization levers in order (curated tools → 2–3 skills → injected knowledge → output contract → persona-as-tone-lever-only); thin-templates honesty rules; the four-part draft package. Post-ratification edits are ordinary D10-gated versions. **Recommendation: ratify the seed; edit anytime through the gate.**

**Decision 5 — phase readings, en bloc.** The six packets logged their clearly-implied readings in the commit bodies and CONVENTIONS §16–§21 (≈50 across the phase; headline ones: S04-lands-at-B3 placement, duty-map-as-code-data, turns = per-call usage events, overflow scope per-planned-stage, compose-requires-standing-gap). Precedent: ratified en bloc at B0–B2. **Recommendation: ratify en bloc.**

## 6. Standing operator items (unchanged, listed per the batching rule — none block B4 opening)

1. Hardening session scheduling (needs sudo presence): AppArmor userns grant for service-context confined runs, `socat` (srt runtime dep), egress substrate, run@ install, Landlock live enforcement; freeze/thaw probe rides along.
2. Sanctioned deletions at your leisure: `rm -rf ~/sinet-demo ~/sinet-demo-run1-failed ~/sinet-demo-run2-failed` and `rm -rf tools/dbpeek/`.
3. Suspend probe leg ↔ the battery-drain measurement hour (timer still installed).
4. age identity escrow — due at B4 **before the first snapshot push** (item 6 of the G4 table; B4 is next, so this becomes live homework this phase).
5. Z.AI prompt-unit calibration — still parked (no Z.AI lane exists).

## 7. Gate answers (recorded at close — operator free-text, 2026-07-22)

**Verbatim:** "1. ok, 2. ok, 3. why we only us claude haiku and not glm-5.2 or opus 4.8?, 4. ok, 5. ok." — then, after the coordinator's D3 walkthrough (GLM structurally unavailable: no Z.AI lane exists at v0 and D5 bans metered seats; Opus possible today; option (b) = frontier ceremony seats): **"3b but take opus 4.8 as executor and Sonnet 5 as judge"**.

| # | Decision | Answer | Execution |
|---|---|---|---|
| 1 | Engine pin bump 2.1.215→2.1.216 | **ok** | Executed: pin+lock lockstep, battery green, per-pin canary run+PASS live ($<0.01, 14s). Commit `4132b09`-family (S03.3 bump commit) |
| 2 | Spec bookkeeping ×2 | **ok** | Executed: A2+A3 changelog rows + all 7 marker sites annotated (drafts + assembled). Amendment commit |
| 3 | v0 duty map | **RE-OPENED** | Sequence: operator first answered "3b but take opus 4.8 as executor and Sonnet 5 as judge" → coordinator applied it (exec=opus-4-8, planning=opus-4-8, judge=sonnet-5; the commit stands as INTERIM code, tests green) → operator superseded it same session: *"I dont think the current model split makes sense and is following best practices! We have to split the models how the current research say its correct"* + *"we also should not implement it in this session since its already 48% context"*. **Disposition: D3 is decided by current research, presented for ratification and applied as the NEXT session's first act.** Coordinator live-research findings this session (2026-07-22, sources in STATE log): Anthropic's own tested advisor pattern = Opus plans/advises + Sonnet as DEFAULT executor (≈11% cheaper than all-Opus with ≈2% benchmark GAIN; Haiku only for simple high-volume execution) — contradicting both haiku-everywhere and opus-as-executor; LLM-as-judge best practice = judge must differ from the EXECUTOR (self-preference bias −38%..+90% on ArenaHard, grows with capability; calibrate against humans); the API's own advisor-pairing rule requires advisor/judge capability ≥ executor; Max-plan economics: Opus ≈ 5× Haiku weight, Sonnet ≈ 3×, PLUS a separate Sonnet-specific weekly cap = extra dedicated Sonnet pool. **Preliminary research-derived mix to present: planning=opus-4-8, execution=sonnet-5, judge=opus-4-8** (cross-model vs the executor whose output it judges; ≥-executor rule; S06.10 frontier bar). **RATIFIED 2026-07-22, same session:** operator asked three verification questions (is Sonnet-executes/Opus-judges the current Anthropic recommendation; how does seat selection automate; how do new roles arise), received the grounded answers (recommendation confirmed in substance: Sonnet-5-default-executor positioning + advisor pattern + advisor≥executor API rule + judge≠executor bias literature), and answered **"ok"** — authoritative free-text per the standing gate-answer convention. Implementation deferred to the next session per the operator's same-session context directive; the next session applies WITHOUT re-presentation. Conditions that attach to whatever mix ratifies: P-T06-5 golden-set re-run on the ratified judge (B4 judge-calibration row); serialize-by-deny E3-leg re-run on the ratified executor seat (cheap, pre-registered, B4 battery); GLM-5.2 not seatable at v0 (no Z.AI lane; D5 metered ban) — stays post-v0. **APPLIED 2026-07-22 (`e06f0a4`), closing the decision:** advisor split live in `DefaultDutyMap` — planning=opus-4-8, execution=sonnet-5, judge=opus-4-8; 7 seat-coupled assertion sites moved (routing tests, route e2e; composer provenance stays on the unchanged planning seat); battery `-count=1` 26 pkgs + race worker/stage + gofmt/vet/lockgate green. Both ratification riders are pre-registered in the B4-7 queue row: P-T06-5 golden-set run on the opus-4-8 judge (+ the B2-3 length-bias deferral) and the serialize-by-deny E3 leg on the sonnet-5 executor |
| 4 | Composer playbook seed | **ok** | Ratified; in-code provenance updated to the ratification record (production first-boot seeds carry it) |
| 5 | Phase readings (~50) | **ok** | Ratified en bloc (B0–B2 precedent) |

**Coordinator note found during D2 execution:** a THIRD measurement-closed marker exists that predates this gate's drafts — TBD-P3(PreCompact-blocking + Claude-lane injection-mechanics spike), S03 deferred list, closed by the B1-4 spike (PASS, result file 2026-07-20). Not covered by the operator's D2 "ok" (which ratified two drafted entries), so NOT applied. Drafted as **A4** for a future free-text ok: *"A4 | TBD-P3(PreCompact/injection mechanics) closed: spike PASS at P3-B1-4 (PreCompact can block; SessionStart source:'compact' re-injection confirmed; containment stays primary per S05.7) — result file 2026-07-20."*

**GATE STATUS 2026-07-22 — FINAL: ALL FIVE DECISIONS CLOSED AND EXECUTED. B3 CLOSED.** D1 engine 2.1.216 (canary PASS live); D2 amendments A2+A3 applied (A4 drafted, awaiting a future ok); D3 advisor split ratified + applied (`e06f0a4`; riders → B4-7); D4 playbook seed ratified (provenance in production seeds); D5 readings en bloc. B4 opened the same session — queue in `P3/STATE.md`; escrow (G4 item 6) is live homework due before B4's first live snapshot push.
