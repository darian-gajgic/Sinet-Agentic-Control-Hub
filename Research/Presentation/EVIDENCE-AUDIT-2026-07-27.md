# Evidence audit — Research/Presentation/index.html vs Research/NN-*.md
Date: 2026-07-27. Every row verified against the named research file line.

## A. Old evidence presented as CURRENT state of the art (the operator's main complaint)

| # | Deck location | Deck says | Research file actually says | Fix |
|---|---|---|---|---|
| A1 | p04 impact + "KNOB 1" gauge | "≤4.9% of ambiguous requests get a clarifying question" as today's headline | R03:58 — S38 is **2024-era models**; "GPT-5-era products ask more by default… the raw rate is a moving target". CURRENT = **ClarifyCodeBench 2026-07**: models DO ask, but find the right question ≤30% (GPT-5 0.21, Sonnet 4.5 0.12); with 3 ambiguities ≈0 for every model | Lead 2026 benchmark; 4.9% → history band |
| A2 | p10 impl4, p11 impl4, p13 downside+impl4 | "58.8% of prompt+model combos drop accuracy on provider updates" as current | R15:57,282 — **CAIN 2024, GPT-3.5-family**, "flagged older — direction institutionalized since" by Anthropic lifecycle + OpenAI deprecation policy | Keep direction (cite vendor lifecycle as current); label numbers GPT-3.5-era |
| A3 | p12 before/after, p13 fix diagram | "MiniCheck-7B 77.4 ≈ Claude-3.5-Sonnet 77.2 — matches **frontier** at €0" | R16:67,273 — board has **"no 2026-gen entries (semi-stale)"**; Claude-3.5-Sonnet = 2024 model; "treat every number as an **optimistic upper bound** for agent-trace text"; the **chosen default seat is Granite Guardian 8B (76.5, Apache-2.0)** — MiniCheck is alternate only (CC-BY-NC) | Say "2024-generation frontier model, board has no 2026 entries"; name Granite Guardian as the seat; carry upper-bound caveat |
| A4 | p12 impact gauge, p13 fix | "an 8B judge scored 40.86% — below coin flip" as the current fact | R16:68 — JudgeBench **ICLR 2025**, model **Llama-3.1-8B (2024)**. CURRENT = **2026 Berkeley reliability study**: Qwen3-8B worst position bias (0.192) + near-perfect consistency (0.992) → "a biased small judge repeats its bias reliably" | Lead with Berkeley 2026; 40.86% becomes the origin story |
| A5 | p07 pull-quote + close | Cognition "Don't Build Multi-Agents" (Jun 2025) as closing wisdom | R06:20 — **Cognition revised 2026-04-22**: ships multi-agent in production for a narrower class; its 2026 reviewer finding **contradicts its own 2025 share-everything principle** | Pull-quote → 2026 revision; 2025 line to history band |
| A6 | squeeze, team, p07, p13 | "~15× tokens" as the current multiplier | R06:73 §2.4 — 15× is **vs chat, 2025 framing**. Anthropic **2026-01 restatement: 3–10× vs single-agent**; Google controlled: centralized +285%. Multi-agent ≈ 3–4× one agent | Lead 3–10× vs single agent (2026); keep 15×-vs-chat labelled 2025 |
| A7 | p10 "step repetition 15.7%" | current failure distribution | R06:20 — MAST "measured **2024–25-era frameworks, several of which no longer exist in that form**" | Add vintage; keep directional |
| A8 | p11 impact chart, team HR vignette | "March 2026 study: no persona 71.6% → expert persona 66.3%" applied to employees' frontier agents | R15:33,238 — S20 is **7–8B open models only**, preprint. On frontier models the replicated result (Wharton Dec 2025, 6 models) is a **null**, not a harm | Split: "no gain on frontier (replicated); measured harm on small open models" |
| A9 | p11 "3.8% → 12.47%" under WHAT IT COSTS TODAY | current | R15:248 — SWE-agent **NeurIPS 2024, "Older"**, direction re-confirmed by 2026 variance study | Move to history band |
| A10 | p05 + team "~45% of research responses carry ≥1 unsupported claim" | current | R04:89 — "(GPT-4o-era, medical — **historical severity, not current state**)" | Relabel or replace with 2026 UPenn fabricated-URL figures |
| A11 | p03 lead graphic | NoLiMa GPT-4o 99.3→69.7 as the headline "measured across model families" | R07:44 — "(Independent benchmark; **pre-2026 models**.)" CURRENT = **OOLONG 2025-11** (GPT-5 / Sonnet 4 / Gemini 2.5 Pro <50% at 128K) + **ConstraintRot 2026-06** | Promote OOLONG/ConstraintRot to lead; NoLiMa → history |

## B. Reasoning errors / dropped scope

| # | Location | Defect |
|---|---|---|
| B1 | p05 honest-downsides | "'Try again' makes **strong models** worse: 91.2→85.0" — R04:100/331 measured **GPT-4o-mini**. Claim contradicts its own source. |
| B2 | p05 why-measured | ">94% valid links coexisted with **39%** claim accuracy" — R04:95 says **39–77%**; deck quotes the floor as the value |
| B3 | p04 fix step 4 | "devil's-advocate … is what **measurably works**: 99.2% vs 48.3%" — R03:76 measures **disagreement rate**; "the **disagreement→quality link is unproven**" |
| B4 | p05 | "frontier models edit tests >79%" — R04:69 says **>79% is Claude-family**; GPT-5 76% oneoff / 54% conflicting |
| B5 | p05 | "STOP escape 93%→1%" — R04:69 "**on one benchmark variant**"; "instruction efficacy is variant-dependent" |
| B6 | p09 impact gauge (the chapter's biggest graphic) | "248 rec 39% → 2,400+ rec 13%" — R11:43,280 "secondary, **unmapped to primary — flagged**"; "magnitudes single-source" |
| B7 | p09 why-measured | "15–25 pp" — R11:42,273 the headline number is **unstable**; v1's "~10% avg" was **dropped from the camera-ready**; direction solid only |
| B8 | p05 before/after | "9 judges ≈ 2 effective votes" — R04:35 **May 2026 single-author preprint, "weight moderated"** |
| B9 | p10 why-measured | "95.64% on real system logs" — R12:371 "**single paper, flagged**" |
| B10 | p11 + team | "+16.2 points" — R15:35 "one benchmark lineage, **not yet independently replicated** — flagged" |
| B11 | p09 Zep row | stops at "corrected to 75.14%"; R11:22 adds "**Zep has since published a new 80% figure (2025-12)**" |
| B12 | p13 fix diagram | prescribes "GPT-4o Mini beat GPT-5-high … 78× lower cost" as the mid-tier lane's justification — Dec 2025 study, 2024 model. R04:35's own framing: **the rubric is the lever**, not that model |

## C. Verified sound — do not "fix"
- **p01 metering** — issues 6805 (3–8×), 53371 (10×), 3434, 35744; LiteLLM 2026-01-27 incident; 19× overshoot. All current, correctly represented (R09:26,38,66).
- **p02 providers** — every pin is a dated 2026 event (R01, R02).
- **p06 machinery** — Apr 2026 Claude Code postmortem, LangGraph/Temporal, RFC 1047 correctly framed as history (R05, R08).
- **p08 security** — Attacker Moves Second (2025-10), CamoLeak, "How we contain Claude" (May 2026), Rule of Two (Oct 2025). Current; caveats already carried (R10:55,66,67,224).
- **p13 Nexus bench-02** — 44/50 $13.71 51min · 43/50 $16.48 34min · 28/50 $241.23 2.5–3.3h, ~17×/~64%, n=1 caveat stated. Matches Docs/nexus-post-mortem.md:30-34 exactly.
