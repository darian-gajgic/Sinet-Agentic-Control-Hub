# Research 16 — The local-models layer (the permanent free tier)

**Topic:** T15 · Wave C · Depth FULL · **Date:** 2026-07-17 · **Method:** deep-research harness (5 fan-out search agents, ~290 tool calls; 3-verifier adversarial wave over 30 decision-driving claims, ~140 verification calls; 2/3-refute kill rule). One decisive claim (V1, "Ollama /v1 lacks logprobs") was refuted by the deepest verifier against release notes + source code and confirmed refuted by coordinator tie-break fetch — corrected throughout; five further wording-level corrections applied. The refuted claim came from Ollama's own stale documentation — a live specimen of why Sinet's conformance suite must assert behavior, never docs (§7-OQ9).
**Inputs held fixed:** G1 P9 envelope (v0 ≤8B-class on the 12 GB pool; RTX 3090 24 GB eGPU = second envelope), G2 Default 6 (v0 operator-wins = manual eager-unload + GameMode hook), report 09 §4.9 arbitration design, report 10 GPU-broker stance (no `/dev/nvidia*` in sandboxes), the six-duty list with acceptance bars (reports 11/12 + G2 addendum), D3/D5/3.9/3.11/S2.7.

---

## 1. Scope

Feature-list items covered: **Operating reality** (local GPU = permanent free tier: health watching, change detection, inbox risk-ranking, routine classification — zero allowance, works when every paid window is empty), **S2.7** (health watchdogs run local), **3.9** (never fully offline), **3.11** (GPU/VRAM/RAM/CPU arbitration; operator's interactive use always wins), **1.10** (utility/ceremony model candidates — the local-vs-paid cut line), **12.1** (local image gen — awareness only).

G2-addendum obligations discharged here: the **six-duty benchmark set** within the P9 envelope (watchdog disambiguation, watchlist triage, canned-query intent-filling, memory-pipeline duties, claim–citation entailment, run-summary generation); the **GPU-broker interface** (R10-OQ4); the **VRAM-ledger measurement method** (R09-OQ8); **contradiction-screen quality** (R11-OQ7); **local entailment** (G1 Def.2 carry-over). Settled and not reopened: local rides the D3 adapter contract; local is the permanent floor of D5 pressure routing; watchdog duties have no cloud fallback.

---

## 2. Current state of the art (mid-2026)

### 2.1 The model landscape moved a full generation in the last twelve months (snapshot dated 2026-07-17)

The small-model field of mid-2025 (Qwen3, Gemma 3, Phi-4, Llama 3.1 8B) has been superseded almost entirely, and — the single most consequential shift for a permissive-license platform — **the strong small models are now overwhelmingly Apache 2.0**:

- **Qwen3.5 small series** (0.8B / 2B / 4B / 9B; HF repos created 2026-02-24..28): Apache 2.0, 262K native context, natively multimodal, hybrid Gated-DeltaNet architecture [S1, S2 — verified via HF API license tags]. **Qwen3.6** shipped only 27B (repo 2026-04-21) and 35B-A3B MoE (2026-04-15); **no small Qwen3.6 exists yet** [S2, S3].
- **Gemma 4** (E2B, E4B, 26B-A4B MoE, 31B — staggered 2026-03; 12B added ~2026-06): **switched to Apache 2.0** — Google's own terms page now carves Gemma 4 out of the restricted Gemma Terms of Use — with **official QAT (int4) checkpoints for every size** and bundled draft models for speculative decoding [S4, S5, S6 — double-verified: Google terms page + HF license tags; Gemma 3 repos remain `license:gemma`].
- **Ministral 3** (3B / 8B / 14B; base/instruct/reasoning; image input; 2025-12-02): all Apache 2.0 — ends the research-only-license problem of the 2024 Ministral 8B [S7].
- **Granite 4.1** (3B / 8B / 30B; HF repos 2026-04, artifacts through late April): Apache 2.0, tuned specifically for instruction-following/tool-calls/RAG; a **granite-guardian-4.1-8b** sibling exists (updated 2026-04-29) [S8].
- **Nemotron 3 Nano 30B-A3B** (Mamba-MoE, 2025-12-15): capable but **license unverified** (gated HF card; the Nano 2 line used the conditions-bearing NVIDIA Open Model License) [S9 — flagged].
- **What does NOT exist** (verified against the HF orgs — 2026 roundup blogs hallucinate models): no dense text Llama smaller than 70B since **Llama 3.2 1B/3B (2024-09)** — nothing 2025–2026; no Phi-5 (newest Phi: Phi-4-reasoning-vision-15B, 2026-01, MIT); DeepSeek's newest small *reasoning distill* is still R1-0528-Qwen3-8B (2025-05; the June-2026 small releases are speculative-decoding draft adapters only) [S10, S11, S12].

**Envelope 1 (12 GB pool, v0 ≤8B-class per P9)** — the honest tier list for Sinet's duty profile (classification, JSON extraction, log/trace summarization, screening):

| Model | Q4/Q5 class | License | Standing (independent evals) |
|---|---|---|---|
| **Qwen3.5-9B** | ~6–7 GB | Apache 2.0 | Artificial Analysis small-open index **21**; strongest sub-10B generalist [S13] |
| **Gemma 4 12B** (QAT int4) | ~7 GB | Apache 2.0 | AA **22** (reasoning mode) — the top ≤12B entry, just above the 9B [S13 — verifier-corrected ranking]; Gemma-12B lineage = 4.4% HHEM grounded-summarization hallucination [S14] |
| Qwen3.5-4B | ~3 GB | Apache 2.0 | Fast tier for bursts; CPU-viable |
| Granite 4.1 8B | ~5 GB | Apache 2.0 | IF/tool/RAG specialist; strongest governance story [S8] |
| Phi-4 (14B) | ~9 GB (tight) | MIT | **Best grounding on the HHEM board (3.7%)** but 14B squeezes KV on 12 GB [S14] |
| Phi-4-mini (3.8B) | ~2.5 GB | MIT | **23.5% HHEM — worst on the board; never for summarization** [S14] |
| gpt-oss-20b | ~12 GB MXFP4 | Apache 2.0 | Borderline on 12 GB; open sm_120 MXFP4 compile issue [S15 — single-source]; poor grounding lineage |

**Envelope 2 (24 GB eGPU):** **Qwen3.6-27B** (~16–17 GB Q4) is the **#1 open 4–40B model on Artificial Analysis** (index 37 reasoning / 30 non-reasoning — verified exactly); **Qwen3.6-35B-A3B** (~19–20 GB Q4, 3B active) gives near-27B quality at MoE throughput for high-volume triage; **Gemma 4 31B** QAT (~18 GB, AA 29) is the summarization-leaning alternative [S13]. Text-to-SQL specialists (BIRD single-model test EX, human = 92.96%): **Arctic-Text2SQL-R1** 32B **73.84** / 14B **72.22** / **7B 70.43** (Apache 2.0, open on HF), OmniSQL-32B 72.05 — note the 7B fits the *12 GB* pool, which changes the Layer-2-SQL calculus (§4.2, OQ) [S16, S17].

**Quantization:** FP8 W8A8 is effectively lossless and W4A16 rivals 8-bit across scales (Neural Magic, ~500K evals, ACL 2025); the same holds for reasoning distills; **KV-cache quantization is the risky axis** — keep KV at fp16/q8_0 [S18, S19]. Practical rule: Q5/Q6 where VRAM allows (≤9B on 12 GB), Q4_K_M for 27–36B on 24 GB, first-party QAT where offered (Gemma 4).

### 2.2 The evaluation-source landscape (what survives to re-evaluate against)

Alive and useful: **Artificial Analysis** (dedicated 4–40B open filter — the single best shortlist view) [S13]; **Vectara HHEM** grounded-summarization board (HHEM-2.3, updated 2026-05-11) [S14]; **BIRD** (SQL) [S16]; **BFCL V4** (tool calling, updated 2026-04) [S20]; **LLM-AggreFact** (entailment; semi-stale — no 2026-generation entries) [S21]; **EQ-Bench/Judgemark v4** (judge ability; solo-maintained) [S22]; LiveBench; LMArena (popularity signal only). Dead or dormant — do not cite them for 2026 models: **HF Open LLM Leaderboard retired** (~2025-03; v1 archived 2024-06), **dubesor benchtable retired 2026-04-19**, **oobabooga bench moved to "LocalBench"** (last entry 2026-01-08) [S23, S24, S25]. Consequence: leaderboards churn as fast as models — the re-evaluation *method* (§4.10) matters more than any snapshot.

### 2.3 Serving stacks on a shared consumer laptop

- **llama.cpp `llama-server`** (MIT; sequential `bNNNNN` releases, 3–4/day — pin a tag): OpenAI-compatible `/v1/chat/completions`, `/v1/completions`, `/v1/embeddings`, `/v1/models`, plus `/reranking`; **`json_schema` response_format and GBNF grammars natively**; **OpenAI-compatible `logprobs`/`top_logprobs` since Dec 2024** (PR #10783); llguidance available as an opt-in Rust build flag [S26, S27, S28]. **Router mode** (announced 2025-12-11): multi-model, one subprocess per model, LRU eviction (`--models-max`, default 4), `--sleep-idle-seconds` idle unload — **but a sleeping subprocess retains ~600 MiB of VRAM** (CUDA context), and the request for full process termination was closed unresolved (2026-06-25; stale/not-planned — no maintainer commitment either way) [S29, S30 — 600 MiB figure single-source]. Release assets ship **no prebuilt Linux CUDA binaries** (Windows CUDA only) — Linux CUDA means source build, the project's Docker images, or llama-swap's `unified-cuda` container [S31].
- **Ollama** (v0.32.1, 2026-07-16; MIT core): structured outputs (JSON-schema → GBNF) since v0.5; keep_alive residency semantics (5 min default; `0` = unload now; `-1` = pin); official Blackwell CC-12.0 support including RTX 5070 Ti in stock builds; **`logprobs`/`top_logprobs` on BOTH the native and OpenAI-compatible APIs since v0.12.11 (2025-11-12 release notes, verbatim)** [S91]. Verification note: Ollama's own compat-docs checklist still lists logprobs as unsupported and issue #16117 sits closed — two of three verifiers initially confirmed the gap from those pages before the third refuted it against the release notes and `openai/openai.go` conversion code; coordinator tie-break fetch confirmed the refutation [S32, S34, S91 — a live specimen of docs-vs-behavior drift on a vendor's own API]. Real remaining gaps: no per-model GPU pinning (separate pools require two daemons), no rerank endpoint, closed-source GUI app, cloud-drift and attribution controversies (attribution since remedied) [S35, S36].
- **llama-swap** (MIT, ~5k stars, v240 2026-07-15, releases every 1–3 days; **bus factor 1** — mostlygeek, ~95% of commits): a zero-dependency Go proxy that **spawns/kills backend server processes per model** — process death returns VRAM fully to zero, CUDA context included. Per-model YAML: `cmd`, `ttl` (idle auto-unload), `env` (documented `CUDA_VISIBLE_DEVICES` placement pattern), groups (`swap`/`exclusive`/**`persistent`** — the pin primitive), startup preload hooks; `POST /api/models/unload` (manual full release); proxies the full OpenAI surface **plus Anthropic `/v1/messages`** and `/v1/rerank`; Prometheus `/metrics`. Maintainer-measured warm model load: **~9 GB/s from the Linux page cache** [S37, S38, S39].
- **vLLM** (v0.25.x): `gpu_memory_utilization` **default 0.92, preallocated at engine init** (verified in source); sleep mode is real but dev-gated (`VLLM_SERVER_DEV_MODE=1`) and documented as releasing "up to 90%+" — not all — with recent (2026) leak reports; one engine per model, no hot-swap [S40, S41]. Community guidance for small consumer GPUs is a list of mitigations against its server-first defaults.
- Excluded from candidacy on license alone: **LM Studio** (closed source; personal/internal-use terms) [S42]. Watched, not adopted: Docker Model Runner (llama.cpp default engine, load-on-request/unload, idle timeout not configurable), Lemonade Server [S43].

**Load/swap latency reality:** a ~4.5 GB 7B Q4 GGUF cold-loads from NVMe in ~2 s with mmap (default); warm loads from page cache run at ~9 GB/s (≈0.6 s for 5 GB, ≈2.2 s for 20 GB) plus process spawn + CUDA init + health check — practical swaps land at **~2–10 s warm** [S44, S37]. System RAM is the true residency layer: with the 96 GB expansion, the whole model rotation stays page-cache-warm.

### 2.4 Structured outputs and confidence: the consensus, post-debate

The 2024 "Let Me Speak Freely" reasoning-degradation result was substantially a prompt/parser artifact (dottxt re-ran with matched prompts: structured ≥ unstructured on all three tasks; LMSF itself found **classification improves under JSON-mode**), and JSONSchemaBench's independent follow-up found constrained decoding neutral-to-positive with schema-aware prompting [S45, S46, S47]. The residual real risks (BAML): schema-forced fabrication and CoT suppression inside JSON [S48]. **Mid-2026 best practice for fixed-label duties:** (a) schema in the prompt, not only the decoder mask; (b) reasoning *before* the constrained region; (c) an explicit `unclear`/abstain enum member so the model is never forced to fabricate a label; (d) engine-level enforcement (llama.cpp `json_schema`/GBNF; llguidance/XGrammar-class backends compute masks in µs and JSON *validity* is an engine property, not a model property) [S47, S28, S49].

**Confidence:** verbalized confidence from 4–8B models is unreliable as a routing signal (prompt-sensitive at best) [S50]; the supported cheap signal is the **label-token logprob margin (top1−top2) under greedy decoding**, post-hoc calibrated (isotonic/temperature scaling) on a local labeled set — the UCCI preprint demonstrates the full recipe (margin → isotonic → cost-constrained threshold; ECE 0.12→0.03 on a 4B; −31% cost at the large model's quality in a 4B→12B cascade) but is **single-author and unreviewed: adopt the recipe shape, re-derive the numbers locally** [S51 — flagged]. Conformal abstention is the guarantee-bearing alternative [S52]. Greedy decoding is chosen here for determinism and a well-defined margin — a design choice, not a benchmark-proven claim (verifier-softened). **This entire signal path requires logprobs on the OpenAI-compatible surface — served by llama.cpp since Dec 2024 and by Ollama since v0.12.11 (Nov 2025), so both the primary stack and its fallback carry it (§2.3); the conformance suite asserts it on every version bump because vendor docs demonstrably lag behavior.**

### 2.5 What small models can and cannot do (duty-relevant evidence)

- **Entailment / grounding checks — specialists match frontier generalists:** LLM-AggreFact: **Bespoke-MiniCheck-7B 77.4** (top; **CC-BY-NC 4.0** — usable for Sinet's personal infra, flagged for any commercial reuse), **Granite Guardian 3.3 8B 76.5** (IBM claims #1 on REVEAL reasoning-chain verification), **FactCG-DeBERTa-L 0.4B 75.6**, vs Claude-3.5-Sonnet 77.2 / GPT-4o 75.9 — a 0.2-point spread at the top: "specialized ≤8B matches frontier on grounding screens" is verified; expect ~77–78% balanced accuracy ceiling on *hard* pairs, much higher on clean ones. All benchmark domains are news/wiki English — treat every number as an optimistic upper bound for agent-trace text [S21, S53, S54].
- **Open-ended judging — unsafe at 8B:** JudgeBench (ICLR 2025, verified from the PDF): Llama-3.1-8B-Instruct **40.86%** on hard pairwise judging — *below the 50% random floor*; fine-tuned Skywork-8B judge 53.43%; the Skywork-Reward-8B reward model 62.29% ≈ Claude-3.5-Sonnet 64.29% [S55]. The 2026 Berkeley reliability study adds the "consistent-but-invalid" trap: Qwen3-8B pairs the *worst* position bias (0.192) with near-perfect test-retest consistency (0.992) — a biased small judge repeats its bias reliably [S56]. Verdict: fixed-schema screens yes; open preference/correctness adjudication no; where pairwise is unavoidable, position-swap and average.
- **Summarization at 4–9B:** HHEM-2.3 board: Qwen3-8B 4.8%, Gemma-3-12B 4.4%, Gemma-3-4B 6.4% hallucination vs frontier 0.7–1.5% — 2–5× worse relatively, ~95% clean absolutely on short docs; rates worsen with input length. Mitigations with support: grounding instructions, extract-then-abstract, citation-forcing, length caps, and generate-then-verify with a MiniCheck-class screen [S14, S57]. No published log/trace-domain faithfulness benchmark exists — Sinet's battery has to create one.
- **Contradiction/NLI:** high precision, weak recall at small scale (ECon); known failure modes: temporal (~55% on temporal probes with an 83% entailment bias), numeric, negation. No published P/R for lesson-pair screening — R11-OQ7 stays open until measured locally; the duty must be designed as a **high-precision advisory screen, not an exhaustive detector** [S58, S59].
- **SQL:** §2.1 numbers; all open specialists are Qwen2.5-Coder-era fine-tunes; the single-model track is now topped by closed Gemini-SQL2 (80.04) — open ≈ GPT-4o-class, well below human (92.96).

### 2.6 Laptop-inference operational facts

- **Power control:** `nvidia-smi -pl` is **blocked on mobile GPUs** under Linux (breakage began with driver 530.41; issue open for 3+ years with no NVIDIA response); the working alternative is **clock capping** (`-lgc`/`-lmc`, root, RTX-era laptops) [S60]. Dynamic Boost requires the optional `nvidia-powerd` daemon and is **AC-only** [S61]. RTX 5070 Ti Laptop: GB205, 12 GB GDDR7 (192-bit, 672 GB/s), TGP 60–115 W +25 W boost [S62].
- **RTD3 runtime power-off** (NVIDIA README, verified verbatim): fine-grained runtime PM is the *default* on Ampere+ notebooks; the **deepest state (VRAM off) requires used VRAM below `NVreg_DynamicPowerManagementVideoMemoryThreshold` — default 200 MB, max 1024 MB**; above it, at best self-refresh. `nvidia-persistenced` in UVM persistence mode disables RTD3 entirely. Community-tier but load-bearing: **polling nvidia-smi/NVML wakes a suspended GPU** — monitoring must gate on `/sys/.../power/runtime_status` first [S63, S64]. Consequence: **"a multi-GB model always resident" and "dGPU powers off" are mutually exclusive** — this revises report 09's resident-slot sketch (§4.6).
- **Burst economics:** a 1–2K-token small-model call is ~1–3 s of GPU activity, ~0.03–0.05 Wh — thermally and energetically trivial; generation is bandwidth-bound and runs far below TGP (a 3060 generating at ~33 W in P3); sustained *batch* is what heats the chassis (this chassis holds 140 W-class GPU load for an hour without throttling — at 47–50 dB(A) fan noise) [S65, S66, S67, S68]. Token/s-vs-watts sweeps show throughput flat above ~⅔ power while J/token worsens — capping clocks for background work is the accepted practice [S69].
- **CPU as a third pool:** the 275HX (AVX2+VNNI, no AVX-512/AMX, DDR5-6400 ≈ 102 GB/s theoretical) plausibly runs 3–4B Q4 at ~20–35 t/s and ≤1.5B at 50+ t/s (class estimate from desktop-CPU measurements — **no citable laptop source; measure at bring-up**); the Intel iGPU shares the same DRAM bandwidth and does not beat the CPU for generation — not worth a third backend [S70, S71].
- **eGPU over TB4:** effective ~2.8–2.9 GB/s; once weights are resident the token-rate penalty is small (measurements range 5–17% — re-measure on-rig), but **model load is ~7× slower** — the eGPU is a resident-model pool, not a swap pool. `boltctl enroll` gives persistent authorization; **detach must be choreographed** (unload → verify no `/dev/nvidia*` holders → unplug); treat cable-yank as reboot-likely. A Dec-2025 open issue shows Blackwell-over-Thunderbolt hard-locking on CUDA — relevant only if the 3090 is ever replaced [S72, S73, S74].

---

## 3. Candidate approaches

### 3.1 Serving stack (the D3 endpoint for the local lane)

| Option | For | Against | Verdict |
|---|---|---|---|
| **A. llama-swap fronting pinned `llama-server` processes** | OpenAI surface incl. **logprobs** + `json_schema` + embeddings + rerank; TTL idle unload with **full VRAM release** (process kill); `persistent` group = pin primitive; per-model `env` GPU placement (UUID) = clean dual-pool; manual `/api/models/unload` = the G2 Default-6 eager-unload verb; Prometheus metrics; MIT+MIT | Two pinned dependencies; llama-swap bus factor 1; Linux CUDA llama.cpp must be built/containerized | **Primary** |
| B. Ollama daemon(s) | One binary, official Blackwell support, keep_alive semantics, structured outputs, **`/v1` logprobs since v0.12.11** (docs lag behavior — verified against release notes) | No per-model GPU pinning — two daemons for two pools; no rerank; weaker per-model TTL/group/alias primitives than the broker design leans on; governance drift (closed GUI, cloud push) and demonstrated docs-vs-behavior drift on its own API | **Strong fallback** (capability-complete for the logprob-dependent designs) |
| C. llama-server router mode alone | One dependency; LRU multi-model; management verbs built in | Idle "sleep" retains ~600 MiB per model subprocess — GPU never returns to zero; full termination refused upstream | Rejected as the idle story; the underlying `llama-server` is still option A's engine |
| D. vLLM | Throughput, FP8, batch | 0.92 preallocation at init; dev-gated sleep with known leaks; no hot-swap; fights the shared-laptop reality | Not primary; optional llama-swap-managed **eGPU batch backend** if ever needed |

### 3.2 Model set per envelope — candidates

12 GB pool: workhorse = Qwen3.5-9B **or** Gemma 4 12B QAT (AA 21 vs 22; the 9B leaves ~1 GB more for KV — relevant for long-trace run summaries; the Gemma lineage grounds better on HHEM; both Apache 2.0 — this is a bakeoff, not an architecture decision); fast tier = Qwen3.5-4B; entailment specialist = Granite Guardian (3.3 benchmarked / 4.1 current — evaluate at adoption) vs Bespoke-MiniCheck-7B (CC-BY-NC) vs MiniCheck-Flan-T5-0.8B (CPU); NLI pre-screen = DeBERTa-class cross-encoder (CPU, ~100 MB); embedder (post-gate per report 11) = Qwen3-Embedding-0.6B GGUF. 24 GB pool: Qwen3.6-27B (quality) / Qwen3.6-35B-A3B (throughput) / Gemma 4 31B QAT (summarization); SQL = Arctic-Text2SQL-R1 14B/32B (7B also fits the 12 GB pool).

### 3.3 Residency / placement policies

(i) Always-resident GPU model (report 09's sketch) — burns RTD3 (§2.6), ~7–18 W idle-awake class, 170–430 Wh/day; (ii) **on-demand with page-cache-warm loads** (~2 s for the 4B, ~3–7 s for the 9B incl. spawn) — free tier stays free in watts too; (iii) **CPU-first for the tiniest duties** (dGPU stays suspended entirely). The duties are event-driven annotations and batch passes — none is latency-critical below ~10 s.

### 3.4 Broker interface options

(i) Expose the llama-swap endpoint directly into sandboxes — rejected: no per-run identity, model-management surface reachable; (ii) **Sinet control-plane-mediated data plane** (LiteLLM virtual-key pattern: per-client key, model allowlist, budgets/rate limits; management verbs on a separate master-key plane) — matches report 10's control-plane-as-sole-egress shape and the industry sandbox posture (E2B/Daytona/Vercel: no GPU in sandbox, call a service; Daytona's gVisor blocks passthrough outright) [S75, S76].

---

## 4. Recommendation for Sinet

### 4.1 The serving stack (primary)

**Adopt `llama-swap` (pinned release, MIT) fronting pinned `llama.cpp llama-server` backends (pinned `b`-tag via the `unified-cuda` container or a one-time source build), as the single OpenAI-compatible local endpoint for the D3 local-lane adapter.** One llama-swap instance; per-model backend processes pinned by GPU **UUID** (`env: CUDA_VISIBLE_DEVICES=<uuid>`); three groups: `pool12` (swap group, 12 GB), `pool24` (swap group, eGPU — configured only when enrolled), `resident` (persistent group — empty by default; used per §4.6 policy). TTL defaults: fast tier ~120 s, workhorse ~300 s, eGPU members ~1800 s (all ⚙ operator-editable per G1 rider 1). Embeddings ride the same stack (`llama-server --embedding` member with the 0.6B embedder) — **no new framework when report 11's vector post-gate opens.** vLLM is *not* installed at v0; it is the pre-registered eGPU batch backend if a throughput need materializes (llama-swap manages it unchanged).

This satisfies every hard requirement simultaneously: OpenAI-compatible surface for D3 and for changedetection.io's native LLM rules (report 12 §4.5); **logprobs for the confidence-margin signal and the report 12 logprob drift canary**; engine-level `json_schema`/GBNF for every grammar-constrained duty; full-VRAM-release idle behavior for the operator's machine; a manual unload verb for G2 Default 6; per-pool placement and eGPU absence tolerance (an absent pool's members simply fail health checks; Sinet routes around). Adopt-don't-fork holds: both components run unmodified, configured only.

**Risk accepted and mitigated:** llama-swap's bus factor 1. Mitigation: it is a small MIT Go proxy whose YAML/endpoint contract Sinet's conformance suite (P-T01-2/P-T08-1 home, report 12 §4.5c) asserts on every version bump; the pre-registered fallback is llama-server router mode + a Sinet-side kill-idle timer (accepting its 600 MiB-per-sleeping-model cost until upstream relents), or Ollama — capability-complete for the logprob-dependent designs since v0.12.11, at the cost of the per-model placement/group/alias primitives. The conformance suite **must assert logprobs and schema enforcement behaviorally on `/v1`** — Ollama's own docs understated its logprobs support for ~8 months, so documentation is never the check (new check, §7-OQ9).

### 4.2 v0 model set and the per-duty table

Ship **Qwen3.5-9B (Q5_K_M)** as the default workhorse and **Qwen3.5-4B (Q4_K_M)** as the fast tier (one family, one prompting dialect, both Apache 2.0); **Gemma 4 12B QAT** is the pre-registered workhorse alternate, decided by the bring-up bakeoff (§4.9) — AA scores them 21 vs 22, within noise of each other, and the 9B's extra KV headroom matters for long-trace summaries. Specialist seats: **Granite Guardian 8B** (entailment; benchmark row is 3.3 at 76.5 — evaluate 4.1 at adoption; Apache), **Bespoke-MiniCheck-7B** as the accuracy alternate (CC-BY-NC — acceptable for personal infrastructure, blocked if Sinet ever commercializes), **DeBERTa-class NLI cross-encoder** (CPU) as the contradiction pre-screen, **MiniCheck-Flan-T5-0.8B** (CPU) as the entailment floor when the GPU is contended, **Qwen3-Embedding-0.6B** (embedder, post-gate). eGPU seats (when present): **Qwen3.6-27B**, **Qwen3.6-35B-A3B**, **Arctic-Text2SQL-R1-14B/32B**.

| # | Duty (settled source) | v0 model | Enforcement | Confidence/escalation |
|---|---|---|---|---|
| 1 | Watchdog disambiguation — {loop\|productive\|unclear}+confidence+evidence (R12 §4.4 T1) | 4B (GPU) or 4B/2B (CPU when dGPU busy/asleep) | `json_schema` enum + evidence string; greedy | Label margin; low margin ⇒ verdict literally `unclear` — **never cloud** (S2.7); annotates, never gates |
| 2 | Watchlist triage — 1st-pass "matters?" + 2nd-pass {lane, class, severity, summary} (R12 §4.5) | 1st: 4B via changedetection.io LLM rules → local `/v1`; 2nd: 4B; severity ≥ high re-checked by 9B | `json_schema` enums | Margin-gated 9B re-check; price/billing hits always carded to human (settled) |
| 3 | Canned-query intent-filling, Layer 1 (R12 §4.8) | 4B; 9B on low margin | Grammar-constrained slot types | Below threshold ⇒ "which of these did you mean?" card, not a guess. **Layer 2 open SQL:** Arctic-R1-7B *fits the 12 GB pool* at ~70% BIRD-class — G3 decides whether Layer 2 ships pre-eGPU under the settled guardrail stack (§7-OQ10); 14B/32B on eGPU = quality path |
| 4 | Memory pipeline (R11): L1 distiller · lesson drafter · contradiction screen | 9B · 9B (LESSON-requires-correction rule in prompt; human gate downstream) · DeBERTa-NLI (CPU) pre-screen → 9B confirm on hits | Distiller: length caps + event-id citation-forcing; screen: `json_schema` | Contradiction screen = high-precision advisory (question cards, never silent resolution — settled); P/R measured at bring-up (R11-OQ7) |
| 5 | Claim–citation entailment (G1 Def.2) | Guardian 8B (default) / MiniCheck-7B (alternate) / Flan-T5-0.8B (CPU floor) | Binary + P(yes) from logprobs | Threshold from local calibration set; load-bearing claims mandatory + sampled rest (settled); low-confidence ⇒ flag for the paid verification pass, never silent pass |
| 6 | Run-summary generation at run end (R12 §4.2) | 9B + deterministic aggregation (settled shape) | Grounding instruction, extract-then-abstract, section schema | Sampled summaries screened by duty-5 checker (generate-then-verify); persistent failures ⇒ escalate model choice, not the run |

Cross-cutting rules: temperature 0 for all classification-shaped duties (determinism + defined margin); every schema carries an abstain member; every call logged to the D7 ledger with model hash + engine build + tokens (local lane rows price to $0 and count into pressure as the D5 floor — consumption is still *measured*, per D4).

### 4.3 Confidence and escalation discipline (SQ2)

Per duty: (1) collect a small labeled set (bootstrap ~30–50 items/duty at bring-up; grow from production per the report 04/12 golden-set governance); (2) compute the label-margin signal under greedy decoding via `/v1` logprobs; (3) fit an isotonic map margin→P(error) on a calibration split; (4) pick the escalation threshold on a validation split — minimize local-only cost subject to the duty's acceptance bar (UCCI recipe shape [S51 — single-source, shape adopted, numbers re-derived locally]); (5) **recalibrate whenever the model, quant, or engine build changes** (§4.10). Escalation targets: duties 2–6 escalate to the requester's paid utility model (1.10) *when a window is open* — billed to the requester, itemized as ceremony; duty 1 never escalates (its degraded state is the honest `unclear` annotation). 3.9's "whatever remains feasible" boundary is exactly this table: with all windows empty, everything in §4.2 keeps running at local quality, escalations queue as parked work.

### 4.4 The VRAM ledger: measurement method (R09-OQ8 discharged)

The ledger's numbers are per-machine measurements, never documented constants. **Protocol** (scripted at bring-up; re-run on driver upgrade, engine bump, model/quant change, display-mode change):

1. **Enumerate pools:** `nvidia-smi -L` → UUIDs (all placement by UUID — indices renumber on eGPU attach).
2. **Sleep-gate every read:** check `/sys/bus/pci/devices/<addr>/power/runtime_status` first; `suspended` ⇒ record "asleep, 0 client bytes" and **do not run nvidia-smi** (polling wakes the GPU) [S64].
3. **Device truth:** `nvidia-smi --query-gpu=index,uuid,memory.total,memory.used,memory.free,memory.reserved --format=csv,nounits`. Admission math uses device `memory.free`, never the sum of per-process numbers (driver reserve + fragmentation) [S77].
4. **Per-process attribution:** `--query-compute-apps=pid,process_name,used_gpu_memory` for inference; graphics clients (compositor, games) via `nvidia-smi -q -d PIDS` / NVML `GetGraphicsRunningProcesses_v3` — **compute queries alone miss every graphics process** [S78].
5. **Model footprint:** per (model, quant, context, parallel-slots, engine build): record device `memory.used` (a) before backend start, (b) after load, (c) after one warm-up generation at the configured context — KV and CUDA graphs only fully materialize then. Ledger entry = max(c)−(a) + guard band (⚙, start 512 MB). CUDA context overhead (~300–800 MB/process class) is inside the measurement — another reason for one backend process per loaded model, not N servers per pool [S79].
6. **Compositor headroom, per display mode:** with no inference loaded, log step 3+4 over a representative desktop day *in hybrid mode* and again *in MUX-dGPU mode* — hybrid parks desktop allocations on the iGPU (dGPU baseline ≈ MBs), MUX moves hundreds of MB of desktop onto the pool. Headroom = observed max + margin, stored per mode; **hybrid is the VRAM-maximizing and recommended mode for the platform** [S80].
7. **Admission check (live):** load admitted iff current `memory.free` ≥ ledger(model) + guard — evaluated against the live reading, never the ledger's belief (zombie-VRAM exists; after any forced kill, verify `memory.free` recovered before the next admission) [S81].

### 4.5 Arbitration composition (3.11 — composes with report 09 §4.9, G2 Default 6)

- **CPU/RAM (unchanged from report 09):** systemd slices (operator session > control plane > local inference > sandbox batch `CPUWeight=idle`), `MemoryHigh` fences (throttles/reclaims, never OOM-kills — verified verbatim), PSI triggers pause batch admission; **pause primitive = `systemctl freeze/thaw`** (cgroup v2 freezer; frozen processes remain killable). systemd-oomd (Ubuntu default-on): mark the inference slice the designated victim (`ManagedOOMMemoryPressure=kill`), broker unit `ManagedOOMPreference=omit` — kernel pressure response and Sinet's own point at the same target [S82, S83].
- **GPU (the part this report owns):** **freezing is categorically wrong for VRAM** — a stopped process keeps its CUDA context and allocations; the driver frees memory only at process teardown. Preemption = **unload, not pause**: llama-swap `POST /api/models/unload` (all) or per-model; backend stop discipline = SIGTERM → 5 s → SIGKILL (llama.cpp servers can wedge and ignore SIGTERM — the kill path is mandatory, not a fallback), then verify `memory.free` recovery (§4.4-7). In-flight granularity: generation cancels between decode steps (~ms); a running prompt-processing batch cannot be interrupted — **worst-case voluntary-preemption latency ≈ one prefill batch; when the operator needs VRAM *now*, kill the process** [S84, S85].
- **Operator-wins, v0 (G2 Default 6, mechanized):** (a) the **manual eager-unload switch** = a control-plane action that stops local-lane admissions and calls `/api/models/unload` (surfaced as a one-tap card/CLI); (b) the **GameMode hook** = `gamemode.ini [custom] start=/end=` scripts calling the same two verbs (the shipped example literally stops Folding@home on game start — this exact pattern is upstream prior art), **plus** a control-plane D-Bus subscription to `com.feralinteractive.GameMode` `GameRegistered`/`GameUnregistered` (verified in daemon source) — scripts fire even if Sinet is restarting; signals cover the daemon's own restarts [S86]. (c) Contention *detection* beyond GameMode (browser games, video editing): NVML graphics-process delta against the known-desktop set — robust in hybrid mode; degraded in MUX mode (compositor is always a G-process) — plus device-utilization corroboration. Automatic idle-detection resume stays post-v0 (settled). Wayland fullscreen heuristics are rejected as a signal (fragmented per compositor).
- **Watchdog floor under arbitration:** even at full preemption, Tier-0 stays deterministic (no GPU) and Tier-1 falls back to the CPU tier (§4.6) — S2.7 never goes dark; 3.9 holds by construction rather than by a reserved GPU slot.

### 4.6 Residency and power policy (SQ7/SQ8 — revises one report 09 sketch)

**Revision, with reason:** report 09 §4.9 sketched "background intelligence reserves a small always-resident model slot." The RTD3 evidence (§2.6) makes a resident multi-GB model mutually exclusive with dGPU power-off (200 MB/1024 MB threshold, verified from NVIDIA's README) — a cost of roughly 7–18 W-class idle-awake, i.e. hundreds of Wh/day, for latency the duties don't need. **Policy instead (all ⚙):**

- **On AC, active platform hours:** TTL-based warm residency (llama-swap `ttl`) — models stay loaded while duties flow, unload after idle; page cache (RAM) is the residency layer, giving ~2 s (4B) / ~3–7 s (9B) reloads. Optionally pin the 4B into the `resident` group during heavy runs.
- **On battery:** nothing resident on the dGPU; duty-1 verdicts and the tiniest screens run on the **CPU tier** (4B/2B/Flan-T5 — burst CPU cost is fan-invisible); larger duties either load-on-demand (accepting a wake) or park until AC per each duty's urgency class. Batch passes (compaction summaries, contradiction sweeps, benchmark evals) are **AC-only** (`ConditionACPower=true` + udev power_supply trigger for immediate reaction) and preferably in the operator-configured off-hours *daytime* window — not at night: sustained batch is what raises fans (47–50 dB(A) at full tilt in this chassis).
- **Quiet/battery clock policy:** `-lgc` clock caps for background work (`-pl` is unavailable on mobile — verified); do not run `nvidia-persistenced` in UVM mode; `nvidia-powerd` optional (AC-only anyway). Fan-curve control on the PHN16-73 is an unsolved Linux gap (Linuwu-Sense supports the -71 sibling only; nbfc-linux has no PHN16 config) — duty-cycling and clock caps are the available levers, not fan curves [S87]. Battery hygiene: 80% charge cap via `acer-wmi-battery` (mainline submission Jan 2026) for the mostly-plugged host [S88].
- **eGPU pool:** resident-model pool by design (TB4 load penalty ~7×; runtime penalty small). Attach = `boltctl enroll` once, then plug → verify enumeration → start `pool24` members. Detach = choreographed runbook (unload pool → `fuser -v /dev/nvidia*` clean → unplug); yank = assume reboot. Absence = health-check failure ⇒ pool marked absent ⇒ duties fall back to 12 GB/CPU or park — no schedule may *require* the eGPU (it is optional hardware).

### 4.7 GPU-broker interface (R10-OQ4 discharged)

**Shape: OpenAI-compatible data plane, Sinet-mediated, capability-scoped per run.** Sandboxes never see `/dev/nvidia*` (report 10, settled; the ioctl LPE surface and container-toolkit escape class stay outside the boundary). A sandboxed worker that is *granted* local inference gets exactly:

- **Endpoint:** `POST /v1/chat/completions` and `POST /v1/embeddings` (that is the whole data plane; `/v1/models` returns only the run's allowlist), reached at a per-sandbox loopback/unix-socket bridge the sandbox runtime injects — never a routable host address.
- **Identity:** a per-run bearer token minted by the control plane at spawn (LiteLLM virtual-key pattern: model allowlist + rate/size limits + expiry = run lifetime) [S75]. The broker resolves the token → run → owner and writes every call to the D7 ledger (local rows: $0, pressure-floor, still itemized on receipts — the free tier is metered, just free).
- **Model field = duty alias**, not a raw model id (llama-swap aliases; the control plane maps aliases → current model per §4.10 churn policy — workers never learn or choose concrete models, which also keeps model swaps invisible to templates, protecting D8 configs from deprecation churn).
- **Excluded by construction:** all management verbs (load/unload/config — Sinet-internal, master plane only), logprobs *for sandboxed callers* by default (no need; reduces side-channel surface — ⚙ per-template), rerank/completions until a template needs them.
- **Confinement interaction:** C1 workers may get the bridge; C3 (web-reading) workers get it with tighter rate/size budgets (fetched hostile content will sit in prompts — the broker logs make steered-inference auditable); C0 connectors never get it. gVisor **nvproxy stays a non-path** (datacenter GPUs only; Blackwell mobile unverified) unless a future capability genuinely needs in-sandbox CUDA — then the pre-registered spike is `runsc nvproxy list-supported-drivers` on this exact driver (report 10, unchanged).

### 4.8 Watchdog division of labor and cadence (SQ5)

Confirmed as designed in report 12 §4.4 — with the economics now quantified: Tier 0 is deterministic counters over the event log (zero GPU, always on); **Tier 1 is event-driven, not periodic** — invoked only on Tier-0 triggers, so its GPU cost is bounded by the alert rate (target ≤2 flag-now/day ⇒ Tier 1's *worst* case is a handful of 1–3 s calls costing ~0.1 Wh/day; a cold call adds a 2–6 s load). No sampling cadence exists to tune, and **no periodic GPU wake is ever needed** — the ledger daemon's sleep-gate (§4.4-2) keeps monitoring itself from defeating RTD3. Alert-quality discipline (dedup, suppress-as-tuning-signal, chatty-watchdog meta-alert) stays exactly as settled. The one addition from this report: when the dGPU is contended or asleep-on-battery, Tier 1 runs its verdict on the CPU tier rather than waking/queueing — annotation latency is irrelevant to a parked card.

### 4.9 Ceremony cut line (1.10 vs the free tier — SQ6) and the bring-up battery

Evidence-based cut (revisited per model generation via §4.10): **local takes** every fixed-schema screen, annotation, triage, and gated draft — duties 1–6, inbox risk-ranking, receipt-line drafting over deterministic numbers, lesson/observation drafting (human-gated downstream). **The paid utility model keeps** everything user-facing-conversational or open-judgment: interviewing (1.2), restatement-until-confirmed (1.3 — contract-grade fidelity), plan critique (1.5), verification review (5.x), gap advice (2.7). The JudgeBench/Berkeley evidence (§2.5) is the reason: sub-random open judging and high self-consistent position bias at 8B make "local plan critic" a false economy; the G1 rider's "helper screen = conformance-only at v0, local-model plausibility T15-gated" resolves to: **stay conformance-only at v0**; a local plausibility screen becomes admissible once the bring-up battery shows ≥ the pre-registered bar on real helper outputs (post-v0 gate, measured not assumed).

**The T15 battery** (the acceptance instrument for all of the above, built once at bring-up, versioned in the platform repo): per duty ~30–50 labeled cases from synthetic seeds + (once live) real traces per the settled golden-set governance; plus the quant sanity check (`llama-perplexity --kl-divergence` on ~250K tokens of Sinet-domain text against the FP16 baseline); run via the settled eval runner (promptfoo, report 12 §4.7) against the *exact deployed quant and engine build*; results land in Sinet's DB. It doubles as (a) the workhorse bakeoff (9B vs Gemma 4 12B), (b) the calibration set source (§4.3), (c) the acceptance bars' measurement (R12-OQ6, R11-OQ7, Def.2 entailment spike).

### 4.10 The re-evaluation method (the churn answer — this list ages fastest)

Cadence: every 6 months, **or** on watchlist events (the report 12 §4.5 watcher already tracks models.dev/HF/release feeds — add "new open ≤40B family" as a drift class). Steps: (1) shortlist from Artificial Analysis' 4–40B open filter + adoption signals; **verify license on the HF card itself** (this cycle's roundups hallucinated "Llama 3.3 8B" and "Phi-5"); (2) duty-leaderboard pass: HHEM (summarization), AggreFact-or-successor (entailment), BIRD (SQL), BFCL (structured/tool), Judgemark (judging); (3) local KL-divergence quant check; (4) run the T15 battery at the deploy quant; (5) **promotion rule:** challenger replaces incumbent only if it wins ≥2 duty suites with non-overlapping 95% Wilson intervals at equal-or-better tokens/s; (6) **every swap triggers threshold recalibration (§4.3) and the report 12 §4.7 revalidation runbook** — a model swap without recalibration is a silent-quality regression (new platform problem, §7-OQ8). Aliases (§4.7) make swaps invisible to workers and templates.

### What would change the decision

- **llama-swap goes unmaintained or its contract breaks** → fallback ladder in §4.1 (router mode + kill-timer, or Ollama — capability-complete incl. `/v1` logprobs); the conformance suite detects, the alias layer makes the swap mechanical.
- **Ollama adds per-model GPU pinning and group/pin primitives** → re-run the stack comparison; with `/v1` logprobs already present (v0.12.11), single-binary simplicity would then compete on merits.
- **llama.cpp router mode gains full idle process termination** → llama-swap's decisive advantage narrows to groups/aliases/Anthropic-surface; consider consolidating to one dependency.
- **A small Qwen3.6 / Gemma 4.x refresh or a specialized ≤8B entailment model with 2026 training lands** → §4.10 handles it; the battery is the gate, not this report's tables.
- **The eGPU becomes permanent and RAM reaches 96 GB** → revisit placing the workhorse permanently on the 3090 (resident pool) and widening the 12 GB pool's headroom for sandboxed bursts.
- **A duty's acceptance bar proves unreachable at ≤8B** (candidates: contradiction recall, entailment on hard technical claims) → per 3.9, the duty runs degraded-with-disclosure locally and the *mandatory* path shifts to the paid utility model where the duty allows it; watchdog duties instead widen their `unclear` band — they never gain a cloud dependency.

---

## 5. What NOT to use and why

- **vLLM as the primary local server** — 0.92-of-VRAM preallocation at init, dev-gated sleep with documented partial release and recent leak reports, no multi-model hot-swap: structurally wrong for a shared daily-driver 12 GB laptop. Keep only as the optional eGPU batch backend under llama-swap.
- **Ollama as the *primary* D3 local endpoint** — not for the once-reported logprobs gap (refuted in verification: `/v1` logprobs work since v0.12.11; the compat docs are stale) but for structure: no per-model GPU pinning (dual pools require two daemons), no rerank endpoint, and none of the group/persistent/alias primitives §4.1/§4.7 lean on — plus governance drift and the demonstrated docs-vs-behavior drift on its own API. Remains the named, capability-complete fallback.
- **llama-server router-mode sleep as the idle story** — ~600 MiB retained per sleeping model defeats "GPU returns to the operator"; upstream declined full termination.
- **LM Studio** — closed source, personal/internal-use license terms; fails the permissive-pinned-dependency rule regardless of features.
- **SIGSTOP / cgroup-freeze as GPU preemption** — frozen processes keep their VRAM; the driver frees memory only at teardown. Freeze is the CPU/RAM tool; unload/kill is the GPU tool.
- **`nvidia-persistenced` (UVM persistence mode)** — disables RTD3 entirely on this laptop.
- **Periodic GPU polling for monitoring** — every nvidia-smi/NVML query wakes a suspended GPU; all monitoring gates on `runtime_status` first.
- **A permanently resident multi-GB watchdog model** (report 09's sketch, revised §4.6) — forecloses deep GPU sleep for latency no duty needs; CPU tier + warm loads deliver the same guarantee at ~zero idle watts.
- **Prompted ≤8B generalists as open-ended judges** (plan critique, pairwise preference, correctness adjudication) — sub-random on hard pairs (JudgeBench, verified); allowed only as fixed-schema screens with abstain, position-swapped where pairwise.
- **Phi-4-mini for any summarization duty** — 23.5% HHEM hallucination rate, worst on the board (fine as a CPU classifier if ever needed).
- **gpt-oss-20b on the 12 GB pool** — borderline fit, open sm_120 MXFP4 compile issue (single-source), weak grounding lineage.
- **Nemotron 3 Nano and EXAONE 4.5** until licenses are verified on their cards — the Nano card is gated (NVIDIA OML lineage has conditions); EXAONE was historically non-commercial.
- **Bespoke-MiniCheck-7B as the *default* entailment screen** — CC-BY-NC 4.0; fine for this household deployment, but defaults should not embed a license landmine when an Apache-2.0 peer sits 0.9 points behind. Alternate seat only.
- **The Intel iGPU as a third inference pool** — shares DRAM bandwidth with the CPU, no generation-speed win, one more backend to maintain. CPU-first for tiny duties instead.
- **Wayland fullscreen-detection heuristics** for operator-detection — fragmented per compositor (wlrctl fails on GNOME/KDE); GameMode + NVML G-process delta are the robust pair.
- **Model spanning across the TB pools** — reaffirmed (bandwidth-limited; already excluded by the hardware context).
- **Local fine-tuning at v0** — every fine-tune re-ages with each 6-month generation swap and adds a training pipeline to a bus-factor-1 platform; the specialist seats (Guardian/MiniCheck/Arctic) buy the same capability off the shelf. Post-v0 lever only if the battery proves a persistent, material gap.

---

## 6. Harvest-map verdicts

The brief names no direct harvest items. Two adjacent rows are affected by this report's findings:

- **Nexus `qdrant + ollama` memory stack (anti-harvest row; report 11 already demoted storage/assembly):** **SUPERSEDED-BY** the ratified FTS5-now/vector-post-gate design *plus this report's stack* — when the vector gate opens, embeddings are served by the same pinned llama-server (`--embedding`, Qwen3-Embedding-0.6B) behind the same broker; no qdrant, no Ollama, no second serving framework. No serving-stack reason to revive the row exists.
- **Ollama as a general local-serving candidate (implicit in prior systems):** **REVISE** — demoted from default local server to *named fallback* on placement/primitive/governance evidence (§2.3, §5; the initially-reported `/v1` logprobs gap was refuted in verification and is **not** among the reasons); its keep_alive semantics, official Blackwell builds, and `/v1` logprobs keep the fallback fully capable.

---

## 7. Open questions

1. **Workhorse bakeoff (operator, v0 bring-up):** Qwen3.5-9B vs Gemma 4 12B QAT on the T15 battery over real Sinet-domain traces; ship the winner as the default alias target. (AA has them 21 vs 22 — indistinguishable without the battery.)
2. **R11-OQ7, narrowed but open (bring-up battery):** contradiction-screen precision/recall on realistic lesson pairs — literature predicts high-precision/low-recall with temporal/numeric blind spots; the two-stage (DeBERTa pre-screen → 9B confirm) design needs its measured operating point before the question-card volume is known.
3. **Entailment spike (G1 Def.2 carry-over; spec workshop + bring-up):** Guardian (3.3 vs 4.1) vs MiniCheck on ~200 Sinet-domain claim–citation pairs; set the mandatory-coverage threshold; decide whether the CPU floor (Flan-T5-0.8B) is adequate for *sampled* checks. Also verify Guardian 4.1's license on its card at adoption (series is Apache; card not individually fetched).
4. **VRAM ledger numbers (operator, scripted at bring-up):** the §4.4 protocol run on the real machine — per-model footprints, both compositor-headroom figures (hybrid + MUX), unload/kill→free latencies, idle-with-context wattage vs D3cold. Method is delivered; numbers are hardware-truth.
5. **CPU-tier throughput (bring-up battery):** the ~20–35 t/s (3–4B Q4) estimate has no citable laptop source — measure on the 275HX; it decides how much of the battery-mode policy (§4.6) is real.
6. **eGPU on-rig validation (operator, when hardware present):** TB4 token-rate penalty (published spread 5–17%), load times, detach runbook rehearsal; and the standing caution that *Blackwell*-over-Thunderbolt currently hard-locks (open issue) if the 3090 is ever replaced.
7. **GameMode execution context (implementation spike):** gamemoded runs per-user — verify the hook scripts and D-Bus signals cross the user boundary to Sinet's control plane cleanly on this desktop session setup.
8. **NEW PLATFORM PROBLEM — model-churn invalidates calibration:** every local model/quant/engine swap silently invalidates per-duty confidence thresholds and calibration maps. Owner: spec (wire "swap ⇒ recalibrate + revalidate" into the 7.3/§4.7 runbook as a hard gate; §4.10-6).
9. **NEW PLATFORM PROBLEM — stack-capability drift (docs lie; probe behavior):** the local lane's logprobs/json_schema capabilities are load-bearing for canaries and confidence; a stack swap or upstream regression would fail them *silently*. This report caught the live specimen: Ollama's compat docs listed `/v1` logprobs as unsupported ~8 months after v0.12.11 shipped them — two of three verifiers were misled by the vendor's own documentation. Owner: spec (extend the P-T01-2 conformance suite to assert `/v1` logprobs + schema enforcement *behaviorally* on every engine bump; documentation is never the check).
10. **Layer-2 SQL before the eGPU (G3 decision):** Arctic-Text2SQL-R1-7B fits the 12 GB pool at ~70% BIRD-class — enable Layer 2 at v0 under the settled read-only/allowlisted-view guardrails, or hold it eGPU-gated as R12 assumed? Proposed: enable, flagged lower-confidence, measured by the battery.
11. **12.1 awareness (future):** local image gen is a *separate* serving stack (SD-class, not llama-server) — nothing in this report's design blocks it; the broker's alias pattern extends to it. No v0 work.
12. **Speculative decoding as a later latency lever (post-v0):** Gemma 4 ships first-party draft models; llama.cpp supports draft speculation — worth a battery entry only if duty latency ever matters.

---

## 8. Sources

All accessed 2026-07-17. Tiers: P = primary (vendor/project doc, source code, model card), I = independent evaluation, A = academic, C = community/practitioner. Verification: 30 decision-driving claims checked by 3 independent adversarial verifiers (2/3-refute kill rule); corrections folded in above.

**Models & licenses**
- S1. https://huggingface.co/Qwen/Qwen3.5-9B — P — Qwen3.5-9B card: Apache 2.0, 262K ctx, multimodal, Feb 2026.
- S2. https://huggingface.co/models?author=Qwen (org API listing) — P — Qwen3.5 0.8/2/4/9B (repos 2026-02-24..28, Apache); Qwen3.6 = 27B + 35B-A3B only.
- S3. https://github.com/QwenLM/Qwen3.6 — P — Qwen3.6 release scope/dates.
- S4. https://ai.google.dev/gemma/terms — P — Gemma Terms carve Gemma 4 out → Apache 2.0 pointer (the license-change primary source).
- S5. https://huggingface.co/google/gemma-4-12B-it (+ org listing) — P — `license:apache-2.0` tags on all gemma-4 repos; QAT q4_0 repos per size; gemma-3 remains `license:gemma`.
- S6. https://ai.google.dev/gemma/docs/releases — P — Gemma 4 sizes/dates incl. 12B (2026-06).
- S7. https://mistral.ai/news/mistral-3/ — P — Ministral 3 3B/8B/14B, base/instruct/reasoning, vision, Apache 2.0, 2025-12-02.
- S8. https://huggingface.co/models?author=ibm-granite — P — granite-4.1 3/8/30B Apache (repos 2026-04-06); granite-guardian-4.1-8b exists.
- S9. https://research.nvidia.com/labs/nemotron/Nemotron-3/ — P — Nemotron 3 Nano 30B-A3B; HF card gated ⇒ license unverified (flagged).
- S10. https://huggingface.co/models?author=meta-llama — P — no dense <70B text Llama after Llama 3.2 (2024-09); Llama 4 starts at 109B-total MoE.
- S11. https://huggingface.co/models?author=microsoft — P — no Phi-5; newest Phi-4-line entries 2026.
- S12. https://huggingface.co/models?author=deepseek-ai — P — newest small reasoning distill = R1-0528-Qwen3-8B; 2026-06 small repos are speculative-decoding drafts.
- S13. https://artificialanalysis.ai/models/open-source/small — I — 4–40B open filter: Qwen3.6-27B 37/30 (top); Gemma 4 12B 22; Qwen3.5-9B 21 (verifier-corrected ranking).
- S14. https://github.com/vectara/hallucination-leaderboard — I — HHEM-2.3 (2026-05-11): Phi-4 3.7 / Gemma-3-12B 4.4 / Qwen3-8B 4.8 / Gemma-3-4B 6.4 / Phi-4-mini 23.5.
- S15. https://github.com/ggml-org/llama.cpp/issues/19662 — P — MXFP4 CUDA kernels fail to compile for sm_120 (open; single-source).
- S16. https://bird-bench.github.io/ — I — single-model track: Arctic-R1 32B 73.84 / 14B 72.22 / 7B 70.43; OmniSQL-32B 72.05; human 92.96; closed Gemini-SQL2 80.04 tops (2026-06).
- S17. https://huggingface.co/Snowflake/Arctic-Text2SQL-R1-7B — P — Apache 2.0, ungated; card self-reports 68.5 test.
- S18. https://arxiv.org/abs/2411.02355 — A — "Give Me BF16…": FP8 W8A8 effectively lossless; W4A16 rivals 8-bit (~500K evals; ACL 2025).
- S19. https://arxiv.org/abs/2504.04823 — A — W8A8/W4A16 lossless for reasoning distills; sub-4-bit and KV axes risky.
- S20. https://gorilla.cs.berkeley.edu/leaderboard.html — I — BFCL V4 (updated 2026-04): tool-calling incl. small open models.
- S21. https://llm-aggrefact.github.io/ — I — MiniCheck-7B 77.4, Granite Guardian 3.3 76.5, FactCG-DeBERTa-L 75.6, Claude-3.5-Sonnet 77.2; no 2026-gen entries (semi-stale).
- S22. https://eqbench.com — I — Judgemark v4 (judge-ability); solo-maintained.
- S23. https://huggingface.co/docs/leaderboards/open_llm_leaderboard/archive — P — Open LLM Leaderboard archived (v1 2024-06; v2 retired ~2025-03).
- S24. https://dubesor.de/benchtable — C — "retired on 2026-04-19".
- S25. https://oobabooga.github.io/benchmark.html — C — last entry 2026-01-08; moved to LocalBench (KL-divergence methodology); page intermittently unreachable at access time — treat as dormant.

**Serving stack**
- S26. https://github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md — P — endpoints, `json_schema`/GBNF, pooling-gated /v1/embeddings, /reranking, router verbs /models/load|unload, /slots.
- S27. https://github.com/ggml-org/llama.cpp/pull/10783 — P — OpenAI-compatible logprobs on chat completions (merged 2024-12-19).
- S28. https://github.com/ggml-org/llama.cpp/blob/master/docs/llguidance.md — P — llguidance opt-in build flag (`-DLLAMA_LLGUIDANCE=ON`, Rust).
- S29. https://huggingface.co/blog/ggml-org/model-management-in-llamacpp — P — router mode (2025-12-11): subprocess-per-model, `--models-max` LRU, `--sleep-idle-seconds`.
- S30. https://github.com/ggml-org/llama.cpp/issues/19379 — P — sleeping router subprocess retains ~600 MiB (single-source figure); full-termination request closed not-planned 2026-06-25.
- S31. https://github.com/ggml-org/llama.cpp/releases — P — b10058/59 (2026-07-17): Windows CUDA assets only; no Linux CUDA binaries.
- S32. https://docs.ollama.com/api/openai-compatibility — P — /v1 unsupported-features checklist still lists logprobs: **stale vs shipped behavior** (see S91) — cited as the docs-drift specimen, not as capability truth.
- S33. https://docs.ollama.com/api/generate — P — native-API `logprobs`/`top_logprobs`.
- S34. https://github.com/ollama/ollama/issues/16117 — P — "/v1 logprobs" issue; closed after in-thread repro showed the field already returned (state not_planned; effectively not-an-issue).
- S35. https://docs.ollama.com/faq (+ /gpu) — P — keep_alive semantics; CC 12.0 incl. RTX 5070 Ti; UUID recommendation; no per-model pinning.
- S36. https://github.com/ollama/ollama/releases — P — v0.32.1 (2026-07-16); MIT core.
- S37. https://github.com/mostlygeek/llama-swap (+ /wiki/Configuration, /releases) — P — mechanism, ttl/env/groups/persistent, /api/models/unload, OpenAI+Anthropic surface, /metrics; v240 2026-07-15; MIT; ~95% single-maintainer commits.
- S38. https://github.com/mostlygeek/llama-swap/discussions/524 — P — ~9 GB/s page-cache→VRAM; process-level-management rationale.
- S39. https://www.kdnuggets.com/how-to-run-multiple-llms-locally-using-llama-swap-on-a-single-server — C — production-pattern corroboration.
- S40. https://github.com/vllm-project/vllm (vllm/config/cache.py) — P — `gpu_memory_utilization` default 0.92 (verified in source).
- S41. https://docs.vllm.ai/en/latest/features/sleep_mode/ — P — sleep levels; VLLM_SERVER_DEV_MODE gating; partial release wording.
- S42. https://lmstudio.ai/terms — P — closed-source; personal/internal-business use terms.
- S43. https://docs.docker.com/ai/model-runner/ — P — DMR: llama.cpp default engine, load-on-request/unload, driver ≥575.57.08.
- S44. https://github.com/ggml-org/llama.cpp/discussions/18758 — P — mmap/page-cache load behavior (fast warm reloads; slow no-mmap).
- S91. https://github.com/ollama/ollama/releases/tag/v0.12.11 — P — release notes (2025-11-12), verbatim: "Ollama's API and OpenAI-compatible API now support log probabilities" — the capability truth that supersedes S32's stale checklist (coordinator tie-break fetch).

**Structured outputs, confidence, duty evidence**
- S45. https://arxiv.org/abs/2408.02442 — A — "Let Me Speak Freely": format-restriction degradation claims; classification *improved* under JSON-mode.
- S46. https://blog.dottxt.ai/say-what-you-mean.html — C (COI: vendor) — matched-prompt re-runs: structured ≥ unstructured (0.78/0.77, 0.77/0.73, 0.44/0.41).
- S47. https://arxiv.org/abs/2501.10868 — A — JSONSchemaBench: engine-dominated validity; efficiency/coverage/quality across constrained-decoding engines.
- S48. https://boundaryml.com/blog/structured-outputs-create-false-confidence — C (COI) — schema-forced-value + CoT-suppression failure modes.
- S49. https://github.com/guidance-ai/llguidance — P — ~50µs/token mask computation; integration list.
- S50. https://arxiv.org/abs/2412.14737 — A — verbalized-confidence reliability is prompt-dependent; not a robust routing signal at small scale.
- S51. https://arxiv.org/abs/2605.18796 — A (preprint, single-author — flagged) — UCCI: margin→isotonic→cost-constrained threshold; ECE 0.12→0.03; −31% cost on 4B→12B cascade.
- S52. https://arxiv.org/abs/2405.01563 — A — conformal abstention (distribution-free guarantee alternative).
- S53. https://huggingface.co/bespokelabs/Bespoke-MiniCheck-7B — P — CC-BY-NC 4.0 (verbatim); commercial licensing via Bespoke.
- S54. https://github.com/ibm-granite/granite-guardian — P — Guardian claims (REVEAL #1; AggreFact top-3), NAACL-Industry 2025.
- S55. https://arxiv.org/abs/2410.12784 — A — JudgeBench (ICLR 2025), figures verified from PDF: Llama-3.1-8B 40.86; Skywork-8B judge 53.43; Skywork-Reward-8B 62.29; Claude-3.5-Sonnet 64.29.
- S56. https://arxiv.org/abs/2606.19544 — A — 21-judge reliability study: κ 33.8–41.3 pts below raw agreement; Qwen3-8B position bias 0.192 with 0.992 self-consistency.
- S57. https://ollama.com/blog/reduce-hallucinations-with-bespoke-minicheck — P — generate-then-verify deployment pattern.
- S58. https://arxiv.org/abs/2410.04068 — A — ECon: evidence-conflict detection = high precision / low recall for weaker models.
- S59. https://arxiv.org/abs/2110.01113 — A (2021 — age-flagged) — MNLI-tuned models ~55% on temporal probes; 83% entailment bias.

**Arbitration, power, hardware**
- S60. https://github.com/NVIDIA/open-gpu-kernel-modules/issues/483 — P — mobile `-pl` blocked (breakage from 530.41; 525.x last working; open, no NVIDIA response); `-lgc` workaround recipes.
- S61. https://download.nvidia.com/XFree86/Linux-x86_64/570.86.16/README/dynamicboost.html — P — nvidia-powerd: manual enable, AC-only.
- S62. https://www.notebookcheck.net/Nvidia-GeForce-RTX-5070-Ti-Laptop-Benchmarks-and-Specs.934945.0.html — I — GB205, 12 GB GDDR7/192-bit, TGP 60–115+25 W.
- S63. https://download.nvidia.com/XFree86/Linux-x86_64/570.144/README/dynamicpowermanagement.html — P — RTD3: 0x03 default fine-grained on Ampere+ notebooks; 200 MB default / 1024 MB max VRAM threshold; UVM persistence disables RTD3 (verified verbatim).
- S64. https://forums.developer.nvidia.com/t/xorg-still-in-gpu-with-prime-offload-and-dynamic-power-management/170485 — C (2021 — age-flagged) — polling wakes suspended GPU; Xorg ~4–5 MiB on dGPU under PRIME offload.
- S65. https://www.localscore.ai/accelerator/160 — C — 5070 Ti (desktop-class) throughput/TTFT anchors (laptop lands lower — flagged).
- S66. https://github.com/ggml-org/llama.cpp/issues/17880 — P — generation at ~33 W in reduced P-state (bandwidth-bound evidence).
- S67. https://laptopmedia.com/review/acer-predator-helios-neo-16-ai-phn16-73-review-the-most-beautiful-screen-you-cant-see/ — I — PHN16-73: 130 W CPU sustain @91 °C 30 min; GPU 82–83 °C 1 h no throttle; 90 Wh; 330 W PSU.
- S68. https://mkgaminglaptop.com/acer-predator-helios-neo-16-ai-review/ — C (low-tier) — 47–50 dB(A) Turbo fan noise; vent-blocked throttling.
- S69. https://jacquesmattheij.com/llama-energy-efficiency/ — C — tokens/s flat above ~⅔ power; J/token rises to max power (2×3090 sweep).
- S70. https://github.com/ggml-org/llama.cpp/discussions/15013 — C — llama.cpp GPU perf collection (desktop 5070 Ti rows; RTX 4050 mobile floor).
- S71. https://github.com/ggml-org/llama.cpp/discussions/12570 — C — Intel iGPU/SYCL/Vulkan status: ~⅓ bandwidth efficiency; tg ≈ CPU.
- S72. https://manpages.debian.org/testing/bolt/boltctl.1.en.html — P — boltctl enroll/authorize/forget semantics.
- S73. https://localaimaster.com/blog/thunderbolt-vs-oculink-ai — C (single-source; corroborated in range by egpu.io threads) — TB4 ≈2.84–2.91 GB/s; load ~7× slower; runtime penalty small (5–17% spread — re-measure).
- S74. https://github.com/NVIDIA/open-gpu-kernel-modules/issues/979 — P — Blackwell-over-TB5 CUDA hard-lock (open, 2025-12).
- S75. https://docs.litellm.ai/docs/proxy/virtual_keys — P — per-client keys: model allowlist, budgets, tpm/rpm; master-key management plane.
- S76. https://www.particula.tech (2026 sandbox comparison) — C — only Modal offers GPU-in-sandbox; E2B/Daytona/Vercel call external services; Daytona gVisor blocks passthrough.
- S77. https://www.mankier.com/1/nvidia-smi — P — query fields incl. memory.reserved; compute-apps semantics; WDDM caveat (Windows-only).
- S78. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html — P — Compute vs Graphics running-process lists (verbatim); MemoryInfo_v2.
- S79. https://localllm.in/blog/llamacpp-vram-requirements-for-local-llms — C — CUDA context overhead ~300–800 MB/process class (per-machine; flagged).
- S80. https://download.nvidia.com/XFree86/Linux-x86_64/555.42.02/README/primerenderoffload.html — P — PRIME render offload: offloaded apps appear as G-processes on the dGPU.
- S81. https://forums.developer.nvidia.com/t/11-gb-of-gpu-ram-used-and-no-process-listed-by-nvidia-smi/44459 — C (age-flagged; failure mode still reported in 2025-era tools) — zombie/leaked VRAM recovery ladder.
- S82. https://docs.kernel.org/admin-guide/cgroup-v2.html — P — freezer (“can be killed by a fatal signal”); memory.high never-OOM (verbatim).
- S83. https://man7.org/linux/man-pages/man5/systemd.resource-control.5.html (+ systemd-oomd(8), systemd.unit(5) ConditionACPower) — P — slices/weights/MemoryHigh; oomd preferences; AC-condition semantics.
- S84. https://github.com/ggml-org/llama.cpp/issues/20921 — P — llama-server can wedge and ignore SIGTERM (closed unconfirmed/stale).
- S85. https://github.com/ggml-org/llama.cpp/issues/10509 — P — no cancellation inside llama_decode; between-step only (closed unimplemented).
- S86. https://github.com/FeralInteractive/gamemode — P — 1.8.2 (2024-08-19); example gamemode.ini [custom] start/end + script_timeout; D-Bus signals GameRegistered/GameUnregistered + ClientCount (verified in daemon/gamemode-dbus.c).
- S87. https://github.com/0x7375646F/Linuwu-Sense (+ https://github.com/nbfc-linux/nbfc-linux) — C — Predator fan control: PHN16-71 only / no PHN16 config; PHN16-73 unverified.
- S88. https://github.com/frederik-h/acer-wmi-battery (+ https://www.phoronix.com/news/Acer-Laptop-Battery-Linux) — C/I — 80% health-mode charge cap; mainline submission 2026-01.
- S89. https://docs.kernel.org/accounting/psi.html — P — PSI file formats, trigger windows, per-cgroup pressure.
- S90. https://huggingface.co/Qwen/Qwen3-Embedding-0.6B-GGUF — P — official llama-server embedding serving command; Apache 2.0; Q8_0 639 MB.
