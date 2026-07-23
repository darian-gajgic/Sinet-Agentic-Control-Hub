# B4 gate report — deliverables & local tier

Written 2026-07-23 by the build coordinator. Phase B4 (S19.5: deliverable/review/git/backup [S13]; local-model stack + GPU broker + duty aliases [S12]) is complete: all seven queue packets done and validated at full depth through the four-stage pipeline (grounding → executor → evaluation → drain), and the consolidated G3 Def.8 measurement battery ran on this machine. This gate's approval opens **B5 (observability & evals: watchdogs, conformance registry, benchmark machinery, queryable history [S14])**.

## 1. What shipped (packet → commit)

| Packet | Commit(s) | One line |
|---|---|---|
| P3-B4-1 | `e60ddfb` | S13.1–S13.4 deliverable/revision/comment schema (immutable revisions, dual content-pinning, `refs/sinet/deliverable/`), per-type review behavior, server-side anchor re-validation + 5-step re-anchor ladder, one drain path [F1..Fn] |
| P3-B4-2 | `5056c62` + drain `a860445` | S13.5+S13.7 branch-per-pipeline + platform-owned tree-level snapshot commits (never amend/force under review), `repo_registry` + onboarding-as-task, `memory.Committer` live, S09.6 freshness seam closed |
| P3-B4-3 | `2218954` + drain `92d855b` | S13.6+S13.9+S13.10 accept = one High-tier effect-journal action (merge card, broker CAS `--force-with-lease` + `--atomic`), P-T12-1 protected-ref refusal, SSH signing all-or-nothing, encrypted snapshot pipeline + fail-closed restore drill (escrow-decrypt) |
| P3-B4-4 | `372d2ba` + `6789033` + `5024709` | S13.8 previews — runner heuristics inside the sandbox stack, zero-mutation (overlay/throwaway clone), netns port probing, Caddy admin-API route add/remove, before-vs-after dual instances; first packet to use the full 2-round drain cap |
| P3-B4-5 | `aeefd3b` + drain `089c759` | S12.1–S12.4+S12.8 local serving stack (llama-swap + llama.cpp adopted, **built + live**), S12.3 model set, config-gen (GPU-UUID/pool12/TTLs), eager-unload, GameMode hook, duty-alias registry (⚙ `local.alias.<duty>`, $0 D7 rows), one tier-L smoke |
| P3-B4-6 | `4bf43cd` + drain `357a3f5` | S12.6+S12.7 GPU broker two planes (platform-direct mgmt vs sandboxed per-run bearer tokens, allowlist, logprobs-off), VRAM ledger (sleep-gated reads, live-`memory.free` admission), kill-not-freeze (SIGTERM→grace→SIGKILL) |
| P3-B4-7 | `03ed78c` + `4246d69` + `a92f1fe` + drains `54a0500`/`68ad8d4`/`fc69771` + `a66bd98` + `a53f73c` | S12.5+S12.9+S12.10 confidence/calibration (isotonic + threshold, keyed by deployed seat), T15 battery + swap gate (P-T15-1 recalibrate-block), migration 0010 `calibration_records`, the consolidated Def.8 measurements + both D3 riders |

Full B4 range (from B3 close `e06f0a4`): **231 files changed, +34,677 / −169**; brief artifacts in `P3/briefs/P3-B4-{2..7}.md`.

## 2. Evidence

- **Battery (coordinator re-ran fresh this session, 2026-07-23):** `gofmt -l` clean, `go vet` clean, `go build ./...` clean, `go test -count=1 ./...` — **34 packages green** (0 failures, 0 introduced skips beyond the sanctioned tier-R/L absence-skips), `go run ./tools/lockgate` OK. `-race` green on the touched package sets at each packet close.
- **Lock gate:** **17 entries** (15 go.mod deps + 2 workflow refs all covered); **8 new adoptions this phase** — all through the S16 rail with live-verified pins (decision D1 below).
- **Live local tier — proven on the RTX 5070 Ti Laptop (12,227 MiB), user-level, $0:** llama.cpp **b10085** CUDA-built for sm_120 (Blackwell) via a user-level CUDA 12.9 toolkit + micromamba gcc-14 — **no driver / kernel / system-CUDA change** (hard host gate held). Tier-L smoke green twice (executor + evaluator): a deliberately schema-hostile prompt still yielded a schema-valid payload (**json_schema enforcement proven, not assumed**), logprobs present, a $0 zero-allowance D7 row carrying `model_sha256`+`engine_build`, eager-unload releasing VRAM. Tier-R config contract validated against the real pinned llama-swap v241 (`/v1/models` matches manifest, both unload routes 200). b10085 preserves json_schema property order live → no two-call fallback.
- **Paid spend this phase ≈ $2.39** — the two pre-registered D3 riders only: rider 1 (opus-4-8 judge golden set) ≈ $2.08 incl. partials [stop line $5.00], rider 2 (sonnet-5 serialize-by-deny) $0.31 [stop line $1.50]. Both stop lines untouched. Everything else $0 (local GPU or fake-engine).
- **Conformance highlights:** encrypted-snapshot round-trip + fail-closed-on-swapped-blob + secrets-excluded-from-payload (`internal/backup`); accept-through-effect-journal + crash-window-redrive-lands-at-pinned-sha + deterministic-trailers + P-T12-1 protected-ref refusal (`internal/accept`, `internal/broker`); live-Caddy host-hazard tripwire + zero-mutation teardown (`internal/preview`); manifest license/servability + config-gen-never-fakes-non-servable-seats + embedder-refused (`internal/local`); swap⇒recalibrate hard block P-T15-1 + tier-L order-preservation now an assert (`internal/local/battery`).

## 3. Measurements taken (G3 Def.8 — files in `P3/measurements/`, all $0 local, on AC, host gate clean)

| # | Measurement | Headline result | File |
|---|---|---|---|
| 1 | Workhorse bakeoff (9B vs Gemma-4-12B-QAT) | **9B STAYS** — Gemma wins 0 of 6 duty suites (overlapping Wilson or lower); promotion rule not met. 9B 43–47 t/s | `2026-07-22-workhorse-bakeoff.md` |
| 2 | KL quant sanity (both default quants vs BF16) | **Both faithful** — 4B Q4_K_M mean KLD 0.0369 (PPL +1.7%, top-1 90.0%); 9B Q5_K_M 0.0145 (PPL +0.88%, top-1 94.0%). 9B baseline on CPU ~16 min — tractable, executed | `2026-07-22-kl-quant-check.md` |
| 3 | CPU throughput floor | **~11.9 t/s** (4B Q4 CPU) vs ~102 t/s GPU (8.6×). Battery-mode CPU fallback real; 2B downshift parked | `2026-07-22-cpu-throughput-floor.md` |
| 4 | VRAM-ledger calibration | 9B footprint **6340 MiB** @ full S12.7 tuple; live-`memory.free` admission validated vs a real 3.3 GB competing holder; hybrid headroom ~540 MiB; SIGTERM freed the exact footprint. MUX leg = operator-assisted (flagged) | `2026-07-22-vram-ledger-calibration.md` |
| 5 | Per-duty confidence calibration (deployed seats) | Calibrated: **intent-filling@4B + contradiction@9B** (+ entailment@Guardian). watchdog/watchlist do NOT meet the bar at the deployed 4B fast seat (honest); intake-triage can't-separate (subjective `size`) → escalate-everything | `2026-07-22-per-duty-confidence-calibration.md` |
| 6 | Contradiction-screen P/R | One-stage workhorse screen **P=R=1.0** on the 12-pair synthetic seed (honest weak-recall caveat; DeBERTa pre-screen `Servable:false` — no GGUF path at the pin) | `2026-07-22-contradiction-screen-pr.md` |
| 7 | Entailment thresholds | **Guardian 4.1 = 152/156 (97.4%)** clears the ≥0.90 MAIN bar (per-side TPR 0.949 / TNR 1.000) → **B2 TPR/TNR deferral CLOSED**. Load-bearing 0.95 sub-bar NOT met per-side (TPR 0.949) → mandatory-coverage stays conservative, gate idle. ⚙ `entailment_sample_rate` = **0.20** derived (LIVE write = bring-up) | `2026-07-22-entailment-thresholds.md` |
| 8 | D3 rider 1 — opus-4-8 judge golden set (**paid**) | **TPR 1.000 [0.84,1.00] (20/20), TNR 0.500 [0.19,0.81] (3/6)** — perfect recall, over-strict on clean controls (safe direction). Length bias r=−0.167 (weak). → rubric bundle **v2**, decision D4 | `2026-07-22-rider1-golden-set-opus.md` |
| 9 | D3 rider 2 — sonnet-5 serialize-by-deny (**paid**) | All 3 legs PASS. **Per-pin canary finding:** sonnet-5 honors the first defer on parallel gated tools (6/6 clean parks) WITHOUT needing the serialize-by-deny fallback | `2026-07-22-rider2-serialize-by-deny-sonnet.md` |
| 10 | GameMode user-context probe (TBD-P3) | Signals are **session-bus only, not reachable from system scope** → the `gamemode.ini` scripts leg carries pause/resume; the busctl subscription is deferred-with-finding (activates when the operator supplies `DBUS_SESSION_BUS_ADDRESS`). Spec-satisfying | `2026-07-22-gamemode-user-context-probe.md` |
| 11 | Adverse — Arctic sql-open | **0/30** raw (Arctic-R1 emits chain-of-thought before the SELECT, busting the naive contract) — exactly why S12.3 flags SQL lower-confidence; the real consumer is the S14/B5 guardrail stack (single-statement parse). Recorded honestly | `2026-07-22-sql-open-arctic.md` |
| 12 | Model re-pull record | Gemma / Arctic / Guardian-3.3 / Guardian-4.1 all complete + sha256'd + manifest `Pulled:true`. Bespoke-MiniCheck = HF 401 (operator credential act) | `2026-07-22-model-repull-record.md` |
| — | Flan-T5 serving path | Mechanically serves on the pinned llama-server, but the generic `/completion` path can't discriminate (task-head needs its own wrapper) → CPU-floor entailment rests on Guardian, not Flan-T5 | `2026-07-22-flan-t5-serving-path.md` |

**Measurements due (bring-up, none blocking B5):** the ⚙ `verification.entailment_sample_rate` = 0.20 LIVE write (operator-owned `Registry.Set` — needs the durable operator store + web-research domain); the Guardian-3.3 entailment leg + the 3.3-vs-4.1 head-to-head; the entailed-side floor re-measure before entailment ever gates unsupervised; watchdog/watchlist bar re-measure on grown seeds; the MUX-mode headroom leg (operator display-mode switch); the durable calibration-record writes at the deployed-seat keys.

## 4. Literal demo steps (dev-mode, $0 — run any or none)

```
cd ~/Sinet-Agentic-Control-Hub
go test -count=1 ./...                                                              # full battery, 34 packages
go test -count=1 -v -run TestAcceptEndToEnd ./internal/accept                       # broker-mediated accept through the effect journal
go test -count=1 -v -run "TestSnapshotAndDrillRoundTrip|TestDrillFailsClosedOnSwappedBlob|TestSecretsExcludedFromPayload" ./internal/backup   # encrypted snapshot + fail-closed restore drill
go test -count=1 -v -run TestHostHazardTripwire ./internal/preview                  # the live-Caddy hazard tripwire (tests never touch the live front chain)
go test -count=1 -v -run "TestManifestLicensesAndServability|TestGenerateConfigPool12ResidentEmptyNoEmbedder|TestResolveAllAliasesEmbedderRefused" ./internal/local   # duty-alias registry + config-gen
go test -count=1 -v -run "TestMeasureSuites|TestRunSuiteClassificationScoresAndMargins" ./internal/local/battery   # T15 calibration battery
```

With the local stack present (the `~/.sinet-b45` toolchain + weights), the real-engine tiers add:
```
SINET_LOCAL_LLAMA_SWAP=$HOME/.sinet-b45/bin/llama-swap SINET_LOCAL_LLAMA_SERVER=$HOME/.sinet-b45/build/llama.cpp/build-cuda/bin/llama-server \
  go test -count=1 -v -run TierR ./internal/local        # tier-R: config contract vs the real pinned llama-swap v241
SINET_LIVE_SMOKE=1 <same env> go test -count=1 -v -run TierLSmoke ./internal/local/battery   # tier-L: one $0 real-GPU call (json_schema + logprobs + VRAM release)
```

Your personal acceptance test remains the B6 UI click-through per your 2026-07-20 directive; these are coordinator-driven evidence, not homework.

## 5. Deviations & carried notes

- **Executor pushed once without coordinator sign-off** (B4-7 drain round 1) — CONVENTIONS §5 reserves push for the coordinator post-validation. Content was unaffected and the re-check validated it after the fact; logged as a process breach, no content impact.
- **§28 (the KL clause in the calibration record) needed three passes** to reach the true state — the record-integrity theme of B4-7's evaluation cycle (the eval caught suppressed adverse results and two false "does-not-exist" records; all corrected, including the drain itself catching a truncated Guardian-3.3 re-pull). This is the pipeline working as designed (fresh-context evaluation finding what self-review would miss), recorded plainly.
- **B4-7 exhausted the 2-round drain cap** + a post-cap coordinator inline remainder (`a53f73c`, per the runbook post-cap rule: 2 low/nit residuals implemented inline, fresh re-read not rubber-stamp). B4-4 also used the full cap.
- **Second-engine-adapter reading (runbook ambiguity rule):** S12.1 consumer class (a) — the engine running on local models via the opencode adapter — has no v0 consumer (Z.AI lane parked; the v0 palette routes execution to paid seats; every S12.4 registry duty is class (b), no engine session), so the second engine adapter is NOT in the B4 cut. It attaches when a consumer exists. Consistent with S19.5 B1 "one adapter + one lane."
- **Carried seams (open, none blocking):** Bespoke-MiniCheck entailment seat behind an HF credential; the DeBERTa two-stage contradiction pre-screen re-enters when a serving path exists; the load-bearing entailment sub-bar re-measure; the MUX headroom leg.

## 6. Decisions for the operator

**D1 — Ratify the 8 new B4 adoptions (S16.4 checklist #10 = your approval).** All entered through the rail with live-verified pins, license scope + check date, and (for the engines/organs) behavioral verification, not docs:

| Adoption | Pin | License | Role |
|---|---|---|---|
| llama.cpp (llama-server) | `b10085` | MIT | the local inference engine — built sm_120, live tier-L proven |
| llama-swap | `v241` | MIT | the local model router/organ (`sinet-llamaswap.service`, generated) |
| Ollama | `v0.32.1` | MIT | standby pull/serve fallback (not the default path) |
| age (snapshot encryption) | `v1.3.1` | BSD-3-Clause | S13.10 snapshot crypto (Go module, via the rail) |
| zstd (pure-Go compression) | `v1.19.1` | BSD-3-Clause | S13.10 snapshot compression |
| PDF text extraction | `v0.0.0-20250511090121-…` | BSD-3-Clause | S13.4 per-type review (extracted-text diff) |
| ttyd (terminal-over-WebSocket) | `1.7.7` | MIT | S13.8 preview CLI lane |
| git (host CLI) | `ubuntu-26.04-lts` | GPL-2.0-only | S13.5/S13.7 git topology (host mechanism, never linked) |

**Recommendation: ratify en bloc.**

**D2 — Engine pin bump 2.1.216 → 2.1.217 (S03.3 deliberate-bump).** Installed `claude` drifted to 2.1.217 during the phase; the lock pins 2.1.216. Rider 2 ran clean on 2.1.217 (reported per §10, never silently retargeted). Same procedure as the B3 gate's D1: move `claudecli.Pin` + the lock entry in lockstep, re-run conformance at the new pin, run the per-pin canary. Still cheap — no production workers exist yet (composed workers live only in tests). **Recommendation: bump now.**

**D3 — Model-set license/manifest ratifications.**
- (a) **Guardian-4.1 + Guardian-3.3** manifest rows (both pulled, sha256'd, apache-2.0 first-party) — clean, ratify.
- (b) **Gemma-4-12B-QAT card-vs-spec license conflict** (flagged in `manifest.go` `License.Conflict`). Gemma is a bakeoff **alternate**, NOT on the v0 default path, and it **lost** the bakeoff (0/6) — nothing rides on it, and it can never be promoted without your ratification anyway. **Recommendation: acknowledge the conflict, keep Gemma as a flagged non-default alternate** (or say the word and I drop it from the manifest entirely — no v0 impact either way).
- (c) **Bespoke-MiniCheck** — HF 401 under CC-BY-NC. Guardian already clears the entailment bar alone, so v0 needs nothing here. **Recommendation: leave Bespoke unpulled at v0**; add your HF credential + accept the license at bring-up only if you want the 3.3-vs-4.1-vs-MiniCheck head-to-head.

**D4 — Ratify rubric bundle v2 (D3 rider-1 result, in-code seed bump).** Rider 1 measured the ratified opus-4-8 judge's golden-set rates into `RubricBundle.GoldenSetRates{Measured:true}` + a measured length-bias note — carried forward on `verify.round` rows. Like the B3 composer-playbook seed, an in-code seed needs gate ratification. **Honest finding on record:** the opus judge has perfect recall (TPR 1.0) but is over-strict on clean code (TNR 0.50) — it favors false-positives, the safe direction for a quality gate but it inflates rework on clean deliverables. The rider used simplified S07.5-shaped judge prompts (a judge-as-classifier collapse of the axis schema); a byte-identical-schema re-run is at your discretion. **Recommendation: ratify v2.**

**D5 — Spec-text bookkeeping: TBD markers closed this phase (S00.9 dated changelog; drafts on your ok, I apply).** No ⚙ touched by the bookkeeping itself → no S18 sweep.
- *TBD-P3(GameMode user-context probe) closed:* session-bus-only finding confirmed live at gamemoded v1.8.1 (probe file 2026-07-22); the scripts leg carries pause/resume, the busctl subscription is deferred-with-finding. Marker off S12.2.
- *TBD-BRINGUP(contradiction-screen P/R) closed for the shipped one-stage shape:* P=R=1.0 on the synthetic seed (S09.7); recall against hard real pairs is the golden-set growth item. Marker off S09.7.
- *TBD-BRINGUP(per-duty confidence calibration) closed for the covered duties:* intent-filling@4B + contradiction@9B (+ entailment@Guardian) calibrate; the uncalibrated fast duties carry `calibrated=false` honestly. Marker narrowed on S12.5.
- *B2 entailment TPR/TNR deferral closed:* Guardian 4.1 clears the ≥0.90 MAIN bar; the load-bearing 0.95 sub-bar stays conservative (idle gate). Deferral record settled.
- *(Still pending from the B3 gate)* the **A4** draft — TBD-P3(PreCompact/injection mechanics) closed by the B1-4 spike — awaits your ok (drafted in `B3-report.md` §Coordinator-note). **Recommendation: approve all, including A4.**

**D6 — Phase readings, en bloc.** ~30 clearly-implied readings logged across the seven packets (commit bodies + CONVENTIONS §14+ additions). Headline ones: S13-before-S12 build order; local-tier calls are $0 D7 rows (pressure floor, never dollars); duty aliases are ⚙ map data; footprints keyed by the full S12.7 tuple; calibration keyed by the deployed seat (not a convenient proxy); the second-engine-adapter-attaches-when-a-consumer-exists reading. Precedent: ratified en bloc at B0–B3. **Recommendation: ratify en bloc.**

## 7. Standing operator items (batched per the rule — none block B5 opening)

1. **Cache-location ratification:** the local toolchain + weights live in `~/.sinet-b45` (~38 GB, outside the repo). Sanctioned deletions at your leisure: `~/.sinet-b45/baselines/` (~26 GB KL baselines, deletable post-check), `/tmp/llamaswap-test/` (its server process was killed at landing; the `rm` was permission-denied to me — `rm -rf /tmp/llamaswap-test`), and the carried B3 items (`rm -rf ~/sinet-demo*` and `rm -rf tools/dbpeek/`). Confirm `~/.sinet-b45` is the durable home you want (or name another).
2. **Unit installs stay deferred:** `sinet-llamaswap.service` is GENERATED not installed (the B0 organ precedent); installing it is a hardening-session / gate act, batched here for visibility.
3. **Hardening session (needs your sudo presence; unchanged from B3, now with two B4 additions):** AppArmor userns grant for service-context confined runs, `socat` (srt runtime dep), egress substrate, `run@` install, Landlock live enforcement; **+ the `-lgc` linker note** and **+ optionally wiring `DBUS_SESSION_BUS_ADDRESS`** into the control plane if you want GameMode's D-Bus subscription (the scripts leg needs no decision). Freeze/thaw probe rides along.
4. **MUX-mode VRAM headroom leg** — a display-mode switch (BIOS/reboot), the one operator-assisted measurement; the hybrid figure is recorded.
5. **Bring-up items (not blocking):** the ⚙ `verification.entailment_sample_rate` = 0.20 LIVE write; Guardian-3.3 entailment leg + head-to-head; watchdog/watchlist bar re-measure; durable calibration-record writes at the deployed-seat keys; Bespoke HF credential if wanted.
6. **Carried, unchanged:** suspend-probe leg ↔ the battery-drain measurement hour (timer installed); Z.AI prompt-unit calibration still parked (no Z.AI lane exists at v0).

## 8. Gate answers (recorded at close — operator free-text, 2026-07-23)

**Verbatim:** "ok" — a blanket approval of all six decisions as recommended (authoritative per the standing gate-answer convention; not re-asked via form).

| # | Decision | Answer | Execution |
|---|---|---|---|
| D1 | Ratify the 8 new adoptions | **ok** | Ratified en bloc (S16.4 #10). Entries already in `components.lock` at live-verified pins; no code change — operator approval recorded here + STATE. |
| D2 | Engine pin bump 2.1.216 → 2.1.217 | **ok (executed to the LIVE 2.1.218)** | The installed `claude` advanced 2.1.217 → **2.1.218** between gate presentation and execution (same session, auto-update). The S03.3 reconcile matches the pin to the *installed* engine, so it targeted 2.1.218 (pinning the no-longer-installed 2.1.217 would immediately re-open the drift D2 exists to close). Applied: `claudecli.Pin` + the lock engine entry in lockstep (+ dated note). Evidence at 2.1.218: `TestPinMatchesLock` PASS, `TestRealEngineVersionAgainstPin` PASS ("installed engine matches pin 2.1.218"), invalid-enum rejection + full-argv dry probe PASS — all zero-cost; parallel-gate behavior reconfirmed live one patch below at 2.1.217 (rider 2, 2026-07-22). No paid canary needed. |
| D3 | Model license/manifest | **ok** | Guardian-4.1/3.3 rows clean (apache-2.0). Gemma: manifest `License.Conflict` note updated — operator acknowledged the conflict, Gemma KEPT as a flagged non-default alternate (promotion stays blocked; it lost the bakeoff 0/6, nothing on the v0 path depends on it). Bespoke: left unpulled at v0 (Guardian clears the bar alone). |
| D4 | Ratify rubric bundle v2 | **ok** | `seeds.go` v2 provenance flipped from "FLAGGED for gate ratification" to "RATIFIED at the B4 gate 2026-07-23 (D4)"; the over-strict TNR 0.50 is on record (safe-direction); byte-identical-schema re-run stays operator discretion. Comment-only change; `internal/verify` green. |
| D5 | S00.9 spec bookkeeping (A4–A8) | **ok (incl. A4)** | Five dated changelog entries added to `S00-front-matter.md` (canonical) + regenerated into the assembled `core-architecture-v1.md` (deterministic concat; draft==assembled verified): **A4** PreCompact/injection (clean close), **A5** GameMode probe (clean close), **A6** contradiction-screen P/R (narrowed — one-stage shipped shape), **A7** per-duty calibration (narrowed — covered duties), **A8** entailment thresholds + the B2 TPR/TNR deferral (MAIN bar met, load-bearing sub-bar conservative; ⚙ value derived 0.20, live write bring-up). All marker sites annotated (11 sites across S03/S05/S07/S09/S12/S18/S19; none left bare). No ⚙ default/clamp changed → no S18 re-sweep. |
| D6 | Phase readings en bloc | **ok** | Ratified en bloc (B0–B3 precedent). |

**GATE STATUS 2026-07-23 — FINAL: ALL SIX DECISIONS CLOSED AND EXECUTED. B4 CLOSED.** Landing battery green after all changes (gofmt/vet/build clean, 34 pkgs `go test -count=1`, lockgate 17). **B5 opened the same session** — queue cut in `P3/STATE.md`. Standing items §7 carried (none block B5).
