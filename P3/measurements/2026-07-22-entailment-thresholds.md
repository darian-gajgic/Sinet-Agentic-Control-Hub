# Entailment thresholds + mandatory-coverage bar (S12.9 #5, S07.4, R24 / drain C8) — 2026-07-22

Closes the B2 TPR/TNR deferral. The threshold measurement ran on **Guardian 4.1** (the measured seat). **Guardian 3.3 is now pulled + sha256'd** (`d7a29778…`, completed in drain round 2, size == content-length 5,797,465,184 B) but was CDN-TRUNCATED at measurement time, so **its own entailment leg was NOT run this session** — it completes at bring-up (see the 3.3 row). This is therefore a **4.1 result, not a 3.3-vs-4.1 head-to-head**. **MiniCheck honestly ABSENT** (Bespoke HF 401, operator credential act — never pulled unauthenticated). Set: the **156-pair** Sinet-domain entailment set (`internal/local/battery/testdata/entailment-200.json`, 78 supported / 78 unsupported, grown 15× from the 10-pair `entailment-calibration` seed — licenses, rate limits, versions, config defaults, SLAs, security, regions, deps). $0 local; on AC; b10085.

## PRE-REGISTERED bar (set BEFORE any gating claim — G3 Def.4)

- **TPR ≥ 0.90 AND TNR ≥ 0.90** on this set (a load-bearing / mandatory-coverage claim requires TPR ≥ 0.95).
- The `EntailmentGate` stays IDLE until BOTH the bar is met AND the web-research domain lands (v0.1, S07.4) — this packet wires the checker, measures here, sets the ⚙ derived value, and leaves the gate idle.
- Judged against the SOURCE excerpt only (never citation text — S07.4); binary supported/unsupported + P(yes) from the verdict-token margin.

## Observation

| seat | overall (156 pairs, 78/78 balanced) | 95% Wilson | tokens/s | meets bar (≥0.90) |
|---|---|---|---|---|
| **Granite Guardian 4.1-8B** (`4bb6aa91…`) | **152/156 (97.4%)** | [0.94, 0.99] | 56.8 | **MAIN bar YES** (per-side TPR 0.949 / TNR 1.000 — both ≥ 0.90; per-side split below) |
| Granite Guardian 3.3-8B (now `d7a29778…`) | **not measured this session** — the seat was CDN-TRUNCATED at measurement time (4.20 GB of the true 5.80 GB; `llama-server` refused it: tensor blk.28 out of bounds). Re-pull COMPLETED in drain round 2 (full 5,797,465,184 B == content-length; sha256 `d7a29778…`; manifest `Pulled:true`); its own entailment leg runs at bring-up on this now-complete seat. Honest, recorded not faked. | — | — | — |
| Bespoke-MiniCheck-7B | absent (HF 401, operator credential — never pulled unauthenticated) | — | — | — |

**Guardian 4.1 clears the pre-registered MAIN bar; the load-bearing sub-bar is per-side and must be computed per-side.** The bar (TPR ≥ 0.90 AND TNR ≥ 0.90; load-bearing needs TPR ≥ 0.95) is a per-side bar, so the overall 152/156 = 97.4% is not the governing number. The **PER-SIDE split** (the 4 failures are all `want=supported, got=unsupported` — false-negatives on the entailed side; `e2-029/133/149/155`, confirmed against the labeled set):

| side | rate | 95% Wilson | vs bar |
|---|---|---|---|
| **TPR** — entailed / supported (78 pairs; 4 false-negatives) | **74/78 = 0.949** | [0.875, 0.980] | ≥ 0.90 main ✓; **< 0.95 load-bearing sub-bar ✗** |
| **TNR** — unsupported (78 pairs; 0 false-positives) | **78/78 = 1.000** | [0.953, 1.000] | ≥ 0.90 ✓ |

**The load-bearing 0.95 sub-bar is NOT met on the entailed side** (TPR 0.949 < 0.95) — the earlier "met at 0.974" reading used the OVERALL rate, which masks the entailed-side false-negatives. The ≥0.90 MAIN bar holds on both sides (0.949 / 1.000), so 4.1 is a working entailment seat. **Consequence (R3):** the mandatory-coverage (load-bearing) bar stays CONSERVATIVE — load-bearing claims are ALWAYS checked (the sample rate below governs only the non-load-bearing remainder), the `EntailmentGate` stays idle, and the entailed-side floor is re-measured at bring-up (a larger set / the 3.3 head-to-head / a threshold sweep) before any claim rests unsupervised on a single Guardian pass.

## ⚙ verification.entailment_sample_rate — derived value + write procedure (the one genuine deferral)

- **Derived value = 0.20** (`verification.entailment_sample_rate`): the checker is highly accurate (Guardian 4.1 97.4%) and load-bearing claims are ALWAYS checked (mandatory coverage); for the non-load-bearing remainder a 20% deterministic sample (the S07.4 hash-based `Coverage` split) gives broad drift coverage at low cost while the accurate checker keeps sampled-claim risk within the bar. Conservative — bring-up may raise it once real web-research traces accrue.
- **The LIVE write stays DEFERRED to bring-up** (the coordinator's one genuine adjudication): the write is an operator-owned `Registry.Set{Key:"verification.entailment_sample_rate", Value:0.20, Actor:operator}` — ONE tx (override row + `settings_events` audit + `settings.changed` event, S01.10/S18.1), NEVER a code-default edit (`index.go:347` default 0 stays). The derived value + the exact write procedure exist here (not silently skipped); the write lands at bring-up when the durable operator store + web-research domain are live.

## Flan-T5 CPU-floor verdict → `2026-07-22-flan-t5-serving-path.md` (drain C5).

## Verdict — Guardian 4.1 clears the MAIN bar; the load-bearing sub-bar is NOT met per-side (honest, conservative)

Guardian 4.1 is the working entailment seat: it clears the pre-registered **MAIN bar** — per-side TPR **0.949** (74/78) and TNR **1.000** (78/78), both ≥ 0.90 — on the 156-pair balanced Sinet-domain set. The **load-bearing 0.95 sub-bar is NOT met on the entailed side** (TPR 74/78 = 0.949 < 0.95; the 4 misses are all false-negatives on `supported`), so the **mandatory-coverage bar stays CONSERVATIVE**: load-bearing claims are ALWAYS checked, the `EntailmentGate` stays IDLE (it additionally requires the web-research domain at v0.1 AND `Calibrated`), and the entailed-side floor is re-measured at bring-up before entailment ever gates unsupervised. The B2 TPR/TNR deferral is **CLOSED** with these real per-side numbers — the bar was pre-registered before any gating claim (G3 Def.4), and the recorded verdict is what the data shows, pass or fail. ⚙ `verification.entailment_sample_rate` derived value **0.20** is recorded (governs the non-load-bearing remainder); the LIVE write stays the one genuine bring-up deferral (operator-owned `Registry.Set` — override row + `settings_events` + `settings.changed`, never a code-default edit). Guardian 3.3 is now complete + hashed (`d7a29778…`, manifest `Pulled:true`); its own entailment leg + the 3.3-vs-4.1 head-to-head run at bring-up. $0 local; on AC; host gate clean.
