# Evidence audit — pass 2 (v3), 2026-07-27
Follows `EVIDENCE-AUDIT-2026-07-27.md` (pass 1 / v2). Operator findings that triggered this pass:

1. **The predecessor platform must never be cited.** It was an unrepresentative self-test with no proper setup. Every
   reference, and every claim resting only on it, is removed — not just the name.
2. **Chapter 13's recommendation was one-directional and therefore wrong.** "Frontier does the work, mid-tier judges" —
   the operator's correction: *sometimes a frontier model is needed as the judge and a weaker model does the
   implementation; it depends on the task.*
3. **Every claim and recommendation must trace to current research.** Obsolete evidence may be shown as history but must
   not carry a recommendation.
4. **Recommendations must be detailed** — no dropped conditions.

## A. Predecessor references removed — and what replaced each one

Nothing was simply deleted: every load-bearing point was re-founded on independent published evidence, or dropped.

| # | Where | Was | Now rests on |
|---|---|---|---|
| A1 | p13 (whole chapter) | The bench-02 pair (44/50 $13.71 · 28/50 $241.23 · ~17×/~64%) as the chapter's spine, plus "9 judge rounds defended a bad spec" | Chapter rebuilt from scratch — see §B |
| A2 | team · marketing vignette | "its cheap executor invented product names and prices" | KWBench Apr 2026: best single model applies knowledge it has, unprompted, **27.9%** of the time (arXiv 2604.15760) |
| A3 | p03 before/after | "~15× measured in the predecessor's own ledger" | **Cascading redundant prefill** — cumulative prefill O(M²) in stage count, with named mitigations (report 04 §2.9) |
| A4 | p04 impact | same fabrication claim | same KWBench result |
| A5 | p04 honest downsides | "its spec stage cost 3.5× a direct frontier run" | Dropped. The independent Scott Logic measurement already carried the point: 2,577 lines of markdown, 33.5 min vs 8 min agent time, 3.5 h vs 24 min review |
| A6 | p04 impl + spec list | "its judges defended a frozen bad spec for 9 rounds" | **MAC-Bench** Jun 2026 (arXiv 2606.07805): GPT-5 98.2% success rate vs **35.2% compliance** under adversarial pressure — the "Machiavellian gap"; success-only checking incentivizes spec gaming |
| A7 | p05 impl | "it had a review stage that saw a cost absurdity and no route to a person" | Report 04 §2.7: nobody ships a planted-defect-reaches-a-human harness; the named industry failure is escalation that "becomes a ritual" |
| A8 | p11 tools table | "it shipped an 18-specialist standing roster" | Clause dropped; the persona-army row stands on the 2023 study + Wharton replication |
| A9 | blueprint | "one 11,476-line file · 248 routes; put everything in one file and died with it" | The third-party wrapper cohort already in that chapter (Omnara, agentapi, GitButler, CCManager) + the ~14-month funded-platform half-life |

Verification: `grep -ci "nexus\|predecessor" index.html` → **0**.

## B. Chapter 13 rebuilt — the task-dependence defect

The old chapter argued one rule ("never cheap out on the work; the mid tier takes the judge seat"). That contradicts the
research in both directions. The rebuild encodes a **duty table** instead, and states the finding plainly: *there is no
single right tier.*

**New evidence spine (all current, all published):**

- **The task decides, measurably.** Google Research, arXiv 2512.08296 (v3 2026-04-08), 260 configurations / 5
  architectures / 3 model families: the same architectural choice ranges **+80.8% to −70.0%** by task. This replaces the
  bench-02 run as the chapter's headline measurement — and is far stronger evidence.
- **Where cheap wins as the *worker*:** grounding board flat at the top (MiniCheck-7B 77.4, Granite Guardian 8B 76.5 vs
  Claude-3.5-Sonnet 77.2, GPT-4o 75.9); open-weight function calling within 3–4 points of frontier; local 9B
  summarization 4.4–4.8% hallucination.
- **Where cheap loses as the *worker*:** 8B open-ended adjudication 40.86% (below the 50% floor) + the 2026 Berkeley
  consistency trap (worst position bias 0.192, consistency 0.992).
- **Where cheap wins as the *checker*:** mid-tier judge + position-swap + combined rubric beat Claude Sonnet 4 as judge at
  ≈1/15 cost (71.0% agreement, kappa 0.549).
- **Where a frontier model IS required as the checker — the operator's correction, now explicit:**
  - **plan critique**: fresh-context, frontier-class, devil's-advocate — ~29% F1 on injected plan errors (report 03 §4);
  - **objectively-hard pairs**: strong *non-reasoning* judges near chance (JudgeBench);
  - **report-length deliverables**: mean judge accuracy 0.563 across 32 configurations, best 0.672 (LongJudgeBench).
- **Where no model belongs at all:** aggregated programmatic weak verifiers reach up to **7× the F1** of model judges, and
  in the same study model judges accepted outputs violating explicitly stated constraints.
- **A new axis the old chapter missed entirely:** the checker must be **family-dissimilar from the worker** — model errors
  grow *more* correlated as capability rises (CAPA), and same-family judges inflate their own style. So "just use the best
  model to check" can be wrong even when budget is not the constraint.
- **Routers do not solve this for you:** RouterArena (Oct 2025, ~8,400 queries, 12 routers) — every router short of
  oracle, failing systematically at recognising when the cheap model suffices.

**Demoted to a `HOW WE GOT HERE` history band (no recommendation rests on them):** FrugalGPT's 2023 "+4% accuracy"
(not reproduced in 2026 production evidence); the 2024 panel-of-small-judges result (superseded — 9 judges ≈ 2 independent
votes); RouteLLM's 2024 ">85% cost reduction" (measured against a 2023-generation reference model).

**Scope note kept and sharpened:** the "78× lower cost" judge result is a Dec 2025 study whose named pairing (GPT-4o Mini
vs GPT-5-high) is now a generation behind, and whose headline is really a **175× cost spread ($0.45–78.96 per 1k evals)**.
The chapter reads it as a class result — *"frontier judge = best judge" fails empirically; the rubric is the lever* — and
explicitly does not recommend the named model.

## C. Other defects found and fixed in this pass

| # | Location | Defect | Fix |
|---|---|---|---|
| C1 | squeeze + p01 stat rows | $40k/engineer, Uber-by-April and 18.6× presented as "documented, not hypothetical" / "measured" | All three are one TechCrunch article (secondary tier) whose own $500M figure report 05 flags as unverified and whose sibling "$47k in 11 days" story the report **excludes** as fabricated. Relabelled "reported", THIN EVIDENCE chip added in both places |
| C2 | team · analyst vignette | "the Friday swarm burns 15× tokens" — the vs-chat baseline, contradicting the corrected 3–10×-vs-single-agent framing two paragraphs above | Changed to 3–10× a single agent's tokens |
| C3 | team · developer vignette | "✓ approved (auto-approve: on — like 93% of prompts)" conflated two distinct telemetry stats | Split: auto-approve is on; separately, when asked, users approve 93% |
| C4 | squeeze | "Flat-rate plans across the market get re-priced on ~30 days' notice" | Report 02 calls it the *observed* ~30-day regime and records zero-notice events; now "sometimes two days, sometimes none" (p02 already said this — the two sections now agree) |
| C5 | p07 evidence table | One row merged two unrelated sources: 72% fact erasure attributed to "LangChain multi-agent benchmarks, §2.4" | Split into two rows — 72% → "Deliberative Illusion" arXiv 2606.03032 (§5); ~50% verbatim-relay → LangChain benchmark (§2.1), tagged VENDOR-MEASURED |
| C6 | p10 before/after | The 58.8% model-drift figure read as current in the visible prose; the GPT-3.5-era caveat was only inside the citation chip | Vintage moved into the visible caption: "the only peer-reviewed measurement of this is a 2024 study of GPT-3.5-family updates… shown for direction only" |
| C7 | p10 impl step 6 | "95.64% measured on real system logs" driving an implementation step with no caveat, while the evidence table flags it THIN | Step now says single flagged paper — adopt the shape, re-measure locally |
| C8 | team · HR vignette | "Curated skills are the measured win" — a strong recommendation on one unreplicated benchmark lineage | Softened to "the one intervention here with a measured gain, where the persona has none". (p11's own `How firm is this?` band already carries the full caveat) |
| C9 | p03 lede | "~15× a plain chat" with no provenance | Citation chip added; framed as report 06 §2.4's *anatomy* of the multiplier (undesigned contract → 15×-class; designed → ~2–4×), not a standalone measurement |

## D. Verified sound in this pass — do not "fix"

p01 (metering), p02 (providers), p04 (intake — the B3 disagreement-rate scope note is correct as written), p05
(verification — already carries the 175× spread and the mid-tier/GPT-4o-mini scope note), p06, p07 (apart from C5), p08,
p09 (memory — the "How firm is this?" band correctly demotes the 248→2,400 figures to "secondary source with no traceable
primary"), p11, p12 (local tier — Granite Guardian correctly named as the seat actually taken; both board caveats stated),
blueprint, close.

## E. QA after this pass

```
data-el          13 × each of impact/cause/solution/beforeafter/impl
beats / evb      13 / 13
status tags      286 (was 251)
S##/R##/D# refs  0
duplicate keyframes  none
div depth        0
nexus|predecessor    0
./qa/run-textfit.sh  1 finding (the intentional p08 '✕') at 1440px and at 860px
```

The p13 solution graphic was rebuilt from a 3-lane layout into **six full-width vertical bands** after the first splice
produced 17 text-fit defects — the v2.1 lesson applies unchanged: vertical space is free, horizontal space is not.
