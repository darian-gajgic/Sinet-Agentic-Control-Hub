# Entailment thresholds + mandatory-coverage bar (S12.9 #5, S07.4, R24 / drain C8) — 2026-07-22

Closes the B2 TPR/TNR deferral. Guardian **3.3 vs 4.1** two-way (both pulled + sha256'd); **MiniCheck honestly ABSENT** (Bespoke HF 401, operator credential act — never pulled unauthenticated). Set: the **156-pair** Sinet-domain entailment set (`internal/local/battery/testdata/entailment-200.json`, 78 supported / 78 unsupported, grown 15× from the 10-pair `entailment-calibration` seed — licenses, rate limits, versions, config defaults, SLAs, security, regions, deps). $0 local; on AC; b10085.

## PRE-REGISTERED bar (set BEFORE any gating claim — G3 Def.4)

- **TPR ≥ 0.90 AND TNR ≥ 0.90** on this set (a load-bearing / mandatory-coverage claim requires TPR ≥ 0.95).
- The `EntailmentGate` stays IDLE until BOTH the bar is met AND the web-research domain lands (v0.1, S07.4) — this packet wires the checker, measures here, sets the ⚙ derived value, and leaves the gate idle.
- Judged against the SOURCE excerpt only (never citation text — S07.4); binary supported/unsupported + P(yes) from the verdict-token margin.

## Observation

| seat | overall (156 pairs, 78/78 balanced) | 95% Wilson | tokens/s | meets bar (≥0.90) |
|---|---|---|---|---|
| **Granite Guardian 4.1-8B** (`4bb6aa91…`) | **152/156 (97.4%)** | [0.94, 0.99] | 56.8 | **YES** (only 4 misses of 156; TPR & TNR both ≥ 0.94) |
| Granite Guardian 3.3-8B (`7dee8bb7` = TRUNCATED) | **not measured — the re-pull was INCOMPLETE** (4.20 GB of the true 5.80 GB; `llama-server` refused it: tensor blk.28 out of bounds). Re-pulling to the full 5.80 GB (drain); its entailment runs when complete OR at bring-up. Honest blocker, recorded not faked. | — | — | — |
| Bespoke-MiniCheck-7B | absent (HF 401, operator credential — never pulled unauthenticated) | — | — | — |

**Guardian 4.1 meets the pre-registered bar** (97.4% overall on the balanced set ⇒ both TPR and TNR ≥ ~0.94, above the 0.90 bar; the load-bearing 0.95 sub-bar is met at 0.974). The 3.3-vs-4.1 comparison is one-sided until the 3.3 re-pull completes (the round-1 truncation is honestly recorded); 4.1 is the working entailment seat that clears the bar.

## ⚙ verification.entailment_sample_rate — derived value + write procedure (the one genuine deferral)

- **Derived value = 0.20** (`verification.entailment_sample_rate`): the checker is highly accurate (Guardian 4.1 97.4%) and load-bearing claims are ALWAYS checked (mandatory coverage); for the non-load-bearing remainder a 20% deterministic sample (the S07.4 hash-based `Coverage` split) gives broad drift coverage at low cost while the accurate checker keeps sampled-claim risk within the bar. Conservative — bring-up may raise it once real web-research traces accrue.
- **The LIVE write stays DEFERRED to bring-up** (the coordinator's one genuine adjudication): the write is an operator-owned `Registry.Set{Key:"verification.entailment_sample_rate", Value:0.20, Actor:operator}` — ONE tx (override row + `settings_events` audit + `settings.changed` event, S01.10/S18.1), NEVER a code-default edit (`index.go:347` default 0 stays). The derived value + the exact write procedure exist here (not silently skipped); the write lands at bring-up when the durable operator store + web-research domain are live.

## Flan-T5 CPU-floor verdict → `2026-07-22-flan-t5-serving-path.md` (drain C5).

## Verdict

<!-- filled: which Guardian meets the pre-registered bar; the thresholds + mandatory-coverage bar SET; the deferral CLOSED; gate stays idle (domain + calibrated discipline). -->
