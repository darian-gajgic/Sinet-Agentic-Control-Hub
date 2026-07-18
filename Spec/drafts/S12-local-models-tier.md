## S12 — Local-models tier

**Scope:** The permanent free tier: the local serving architecture, the v0 model set and duty-alias registry, the GPU broker data plane, VRAM-ledger and preemption mechanics, power/residency policy, the bring-up battery, and the lifecycle rules (re-evaluation, swaps, recalibration) for everything local.

**Binding inputs:** Operating reality (local free tier) · 3.9 · 3.11 · S2.7 · 1.10 · 12.1 · 7.3 · [R16 §2, §4, §5, §7] (T15 recommendation as ratified, G3 digest) · [G1 P9; Def.2; Def.9] · [G2 D2.1 (VRAM-ledger admission inside the R09 §4 package); Def.6; Def.8; Def.11; Def.12] · [G3 D3.5; Def.3; Def.4; Def.8; §Follow-ups] · [SPIKE P2-S1 headline] (logprobs exist only on the local lane) · siblings: [S03.2] lane exposure · [S06.10] ceremony duty map · [S01.2] unit-map slot.

Boundary lines: S03 owns adapter mechanics [XREF:S03]; S10 owns pressure/budgets/admission *policy* and all CPU/RAM arbitration [XREF:S10]; S14 owns canary/eval machinery [XREF:S14]; S06/S07 own which duties exist [XREF:S06; XREF:S07]. This section owns the serving stack, which models serve which duties, and the GPU-specific mechanics.

**Term note:** the **GPU broker** ([R16 §4.7], discharging R10-OQ4) is the control-plane-mediated local-inference data plane. It is distinct from the credential **broker** of the S00 glossary; this section always writes "GPU broker".

### S12.1 Position: the permanent free tier

- The platform's own background intelligence — health watching, change detection, inbox risk-ranking, routine classification — runs on local models only, costs nobody any allowance, and keeps working when every paid window is empty [Operating reality; 3.9]. The local tier is the permanent floor of D5 pressure routing.
- The local `/v1` surface has **two consumer classes**: (a) the **D3 local lane** — runs executing on local models ride the pinned `opencode serve` adapter as OpenAI-compatible provider entries pointing at the S12.2 endpoint [S03.2]; (b) **platform duty calls** — the control plane invokes the S12.4 duty aliases directly over the same OpenAI surface, with no engine session involved (intake triage, watchdog verdicts, and their peers are not runs) [R16 §4.2; S06.2].
- The local lane is the **only v0 lane exposing logprobs** [SPIKE P2-S1 headline; S03.2] — it therefore hosts the logprob drift canary [XREF:S14] and carries the S12.5 confidence-margin signal. Both subscription lanes are behavioral-eval-only.
- Model weights live in the shared **read-only model cache** (4.1's sanctioned sharing); the OS page cache is the residency layer (S12.8).
- The free tier is metered, just free: every local call — either consumer class — writes a D7 ledger usage row (model hash + engine build + tokens) priced at $0 and counting into consumption pressure as the D5 floor [D4; R16 §4.2]; local duties appear as zero-allowance receipt lines [S06.10; XREF:S10].

### S12.2 Serving architecture

**One pinned `llama-swap` instance (MIT) is the single OpenAI-compatible local endpoint, fronting pinned `llama.cpp llama-server` backend processes** (pinned `b`-tag; Linux CUDA ships no prebuilt binaries — one-time source build or the project's container image) [R16 §4.1, §2.3; G3 digest]. llama-swap spawns and kills **one backend process per loaded model**; process death returns VRAM fully to zero, CUDA context included [R16 §2.3]. Both components run unmodified, configured only (adopt-don't-fork); pins, replacement path, and abandonment criteria live in `components.lock` [XREF:S16; G2 D2.2].

- **Placement is by GPU UUID**, never index (`env: CUDA_VISIBLE_DEVICES=<uuid>` per model — indices renumber on eGPU attach) [R16 §4.1, §4.4].
- **Groups:** `pool12` (swap group, the 12 GB internal GPU) · `pool24` (swap group; configured only when the eGPU is enrolled, S12.11) · `resident` (persistent group — **empty by default**; S12.8) [R16 §4.1]. Members of a swap group hold their pool exclusively — loading one unloads the others; concurrency comes from the CPU tier and queueing, never co-residency [R16 §2.3].
- **Idle:** per-model TTL expiry kills the backend → full VRAM release (⚙ `local.ttl.*`; defaults S12-table) [R16 §4.1].
- **Manual eager-unload verb = the ratified operator-wins switch [G2 Def.6]:** a control-plane action that (1) stops local-lane admissions and (2) calls `POST /api/models/unload` — surfaced as a one-tap card and CLI verb [R16 §4.5].
- **GameMode hook [G2 Def.6]:** `gamemode.ini [custom]` start/end scripts call the same two verbs, **plus** a control-plane D-Bus subscription to the GameMode `GameRegistered`/`GameUnregistered` signals — scripts fire even while the control plane restarts; signals cover the daemon's own restarts [R16 §4.5]. TBD-P3(GameMode user-context probe — per-user `gamemoded` hooks/signals must reach the system-scope control plane; R16 §7-OQ7). Contention detection beyond GameMode: NVML graphics-process delta against the known-desktop set + device-utilization corroboration (robust in hybrid display mode, degraded in MUX); Wayland fullscreen heuristics are rejected as a signal. Automatic idle-detection (auto-pause/resume) is post-v0 novel work [G2 Def.6; R16 §4.5].
- **Embeddings ride the same stack**: a `llama-server --embedding` member serving the 0.6B embedder is configured **only when the G2 Def.8 vector gate opens** — no second serving framework, ever [R16 §4.1; G2 Def.8].
- **Unit placement:** llama-swap runs as its own adopted-organ system unit, `sinet-llamaswap.service` `[coordinator-draft]` name, loopback-bind per the S01.1 invariant, own journal identity; backends are its child processes, not units, so the whole tier sits in the local-inference slice (the designated systemd-oomd victim; slice policy [XREF:S10]) [S01.2; R16 §4.5].
- **Fallback ladder (pre-registered)** [R16 §4.1]: llama-swap's bus factor 1 is an accepted risk — it is a small MIT Go proxy whose YAML/endpoint contract the conformance suite asserts on every bump. If it goes unmaintained or its contract breaks: (1) `llama-server` router mode + a Sinet-side kill-idle timer, accepting ~600 MiB retained per sleeping model; or (2) **Ollama — the named, capability-complete fallback**, including `/v1` logprobs since v0.12.11 (a fact its own compat docs mis-stated for ~8 months — the live specimen behind P-T15-2: **conformance suites assert behavior, never docs**) [R16 §2.3, §7-OQ9]. Ollama is not primary for structural reasons only: no per-model GPU pinning (two pools ⇒ two daemons), no rerank endpoint, none of the group/persistent/alias primitives this design leans on [R16 §5].
- Not used, for cause [R16 §5]: vLLM as primary (0.92 VRAM preallocation at init; dev-gated partial sleep — it is only the pre-registered pool24 batch backend, S12.11); router-mode sleep as the idle story (~600 MiB retained per model); LM Studio (closed source, restrictive terms).

### S12.3 v0 model set

| Seat | v0 model | Quant · pool | License | Basis |
|---|---|---|---|---|
| Workhorse | **Qwen3.5-9B** | Q5_K_M · pool12 | Apache 2.0 | default alias target unless the bakeoff flips it [G3 Def.8; R16 §4.2] |
| Workhorse alternate | Gemma 4 12B QAT | first-party int4 QAT · pool12 | Apache 2.0 | bakeoff alternate — scores within noise of the 9B; better grounding lineage; less KV headroom [R16 §2.1, §4.2] |
| Fast tier | Qwen3.5-4B | Q4_K_M · pool12 + CPU tier | Apache 2.0 | burst/classification seat; CPU-viable; same family/dialect as the workhorse [R16 §4.2] |
| Entailment | **Granite Guardian 8B** | pool12 | Apache 2.0 | default seat [G3 Def.4]; benchmark row is 3.3 (AggreFact 76.5 — frontier-matching for grounding screens); evaluate 4.1 and verify its card license at adoption [R16 §2.5, §7-OQ3] |
| Entailment accuracy-alternate | Bespoke-MiniCheck-7B | pool12 | **CC-BY-NC 4.0** | alternate seat only — acceptable for this household deployment, never the default (license landmine; blocked if Sinet ever commercializes) [R16 §5] |
| Entailment CPU floor | MiniCheck-Flan-T5-0.8B | CPU | per card | sampled checks only, pending the G3 Def.4 measurement (S12.9) |
| Contradiction pre-screen | DeBERTa-class NLI cross-encoder | CPU (~100 MB) | per card | high-precision advisory pre-screen [R16 §2.5, §4.2] |
| Embedder | Qwen3-Embedding-0.6B (GGUF Q8_0) | pool12 | Apache 2.0 | **post-gate only** [G2 Def.8]; rank-only, selection stays trace-manifested [XREF:S09] |
| Layer-2 open SQL | **Arctic-Text2SQL-R1-7B** | pool12 | Apache 2.0 | fits the 12 GB pool at ~70% BIRD-class (open SOTA ~74%; human ~93%); ships at v0 flagged lower-confidence [G3 D3.5; R16 §2.1] |

- **Envelope reading:** G1 P9's "≤8B" binds as the **≤8B-class / 12-GB-pool envelope** — the reading R16 held fixed and the gates ratified by naming the 9B (G3 Def.8) and the 7B (G3 D3.5) seats; post-v0 the per-duty ceiling is the RTX 3090 24 GB envelope (S12.11) [G1 P9; R16 §1].
- **Quant policy** [R16 §2.1]: Q5/Q6 where VRAM allows (≤9B on pool12); first-party QAT where offered; Q4_K_M for 27–36B on pool24; **KV cache stays fp16/q8_0 — KV quantization is the risky axis**. Deployed quants are validated by the S12.9 KL check.
- **License rule:** Apache-2.0/MIT by default; any exception is carried per-seat with its reason (the sole v0 exception is the MiniCheck alternate). Licenses are verified **on the model card itself** at adoption — aggregator roundups hallucinate models and licenses [R16 §4.10].
- **Layer-2 SQL guardrails, restated by reference [G3 D3.5]:** read-only connection, allowlisted views, single-statement parse, LIMIT + timeout, audit-logged, every answer flagged lower-confidence; canned queries remain the reliability floor. The query surface itself is [XREF:S14].

### S12.4 Duty-alias registry

A **duty alias** (S00 glossary) is a named capability slot mapping to a swappable model; **workers, templates, and platform callers only ever address aliases** — model swaps are invisible to them, which also protects D8 configs from deprecation churn (7.3) [R16 §4.7]. Which duties exist is owned by their sections; this registry binds each to its serving model. Alias names `[coordinator-draft]`; the alias→model map is settings-backed (⚙ `local.alias.<duty>`), changed only through the S12.10 swap gate.

| Alias | Serves [owner] | v0 target | Enforcement | On low confidence |
|---|---|---|---|---|
| `utility` (per-person, 1.10) | card/question phrasing, 13.5 help drafting, summaries, receipt-line drafting over deterministic numbers, inbox risk-ranking, lesson/observation drafting [S06.10; XREF:S09] | workhorse (recommended platform-wide default; per-user override per 1.10) | per-duty schema; length caps | outputs are human-gated downstream; this seat never decides |
| `intake-triage` | task-family/stakes/size classification, data-bearing classifier, advisory coverage spot-check [XREF:S06] | fast; workhorse re-check on low margin | `json_schema` enums | S06's deterministic floors override; classifier failure fails closed at S06's layer |
| `watchdog-disambiguator` | {loop\|productive\|unclear} verdicts on Tier-0 triggers [XREF:S14] | fast (GPU); CPU tier when the dGPU is contended or asleep | `json_schema` enum + evidence string; greedy | low margin ⇒ the verdict **is** `unclear`; annotates, never gates; **NEVER a cloud fallback** (S2.7) |
| `watchlist-triage` | pass 1 "does this change matter?", pass 2 {lane, class, severity, summary} [G2 Def.12; XREF:S14] | fast; severity ≥ high re-checked by workhorse; pass 1 runs via changedetection.io's native LLM rules against the local `/v1` surface [R16 §4.2] | `json_schema` enums | margin-gated workhorse re-check; price/billing hits are always carded to a human |
| `intent-filling` | canned-query Layer-1 slot filling [XREF:S14] | fast; workhorse on low margin | grammar-constrained slot types | below threshold ⇒ a "which of these did you mean?" card — never a guess |
| `sql-open` | Layer-2 open SQL escalation [G3 D3.5] | Arctic-Text2SQL-R1-7B | the S12.3 guardrail stack | every answer flagged lower-confidence by construction |
| `entailment` | claim–citation entailment: mandatory on load-bearing claims + sampled rest [G1 Def.2; XREF:S07] | Guardian 8B; CPU floor for sampled checks pending [G3 Def.4] | binary + P(yes) from logprobs | below threshold ⇒ flagged into the paid verification pass — never a silent pass |
| `contradiction-screen` | lesson/memory contradiction screen [XREF:S09] | DeBERTa CPU pre-screen → workhorse confirm on hits | `json_schema` | high-precision advisory: question cards only, never silent resolution |
| `distill-summarize` | L1 distiller; run summary at run end [G2 Def.11; XREF:S09; XREF:S14] | workhorse | grounding instructions, extract-then-abstract, event-id citation-forcing, length caps, section schema | sampled outputs screened by `entailment` (generate-then-verify); persistent failure escalates the model choice, never the run |
| `embedder` | retrieval candidate ranking, post-gate [G2 Def.8; XREF:S09] | Qwen3-Embedding-0.6B | rank-only; selection stays trace-manifested | n/a |

**Cross-cutting duty rules** [R16 §4.2, §2.4]: temperature 0 / greedy decoding for every classification-shaped duty (determinism + a defined margin); every schema carries an explicit abstain/`unclear` member — a model is never schema-forced to fabricate a label; the schema appears in the prompt *and* is enforced at the engine (`json_schema`/GBNF — validity is an engine property); free-text reasoning precedes the constrained region; every call is ledgered per S12.1.

**The ceremony cut line** [R16 §4.9, §2.5; G3 digest; consistent with S06.10]: local takes every fixed-schema screen, annotation, triage, and human-gated draft — the registry above. **Interviewing (1.2), restatement-until-confirmed (1.3), plan critique (1.5), verification review (5.x), and gap advice (2.7) run on paid frontier-class models and are NEVER assigned to local aliases**: 8B-class generalists judge below the random floor on hard pairwise judging (JudgeBench 40.86% vs the 50% floor) and pair worst-in-class position bias with near-perfect self-consistency — a biased small judge repeats its bias reliably. Helper-report screening stays conformance-only at v0 [G1 Def.9; G3 Def.3]; a local plausibility screen becomes admissible post-v0 only when the S12.9 battery shows ≥ the pre-registered bar on real helper outputs.

### S12.5 Confidence & escalation discipline

Verbalized confidence from 4–8B models is not a routing signal. The supported cheap signal is the **label-token logprob margin (top1−top2) under greedy decoding**, post-hoc calibrated per duty [R16 §2.4]: (1) a labeled set (~30–50 items bootstrap, grown from production under golden-set governance [XREF:S14]); (2) margins via `/v1` logprobs; (3) an isotonic map margin→P(error) on a calibration split; (4) the escalation threshold picked on a validation split — minimize local-only cost subject to the duty's acceptance bar (recipe shape adopted; every number re-derived locally) [R16 §4.3]. All thresholds TBD-BRINGUP(per-duty confidence calibration); calibrated values are platform data keyed by (duty, model hash, engine build) — which is exactly what the S12.10 swap gate keeps valid.

Escalation targets: registry duties escalate to the requester's paid ceremony/verification seat *when a window is open* — billed to the requester, itemized (1.10, 3.4) [XREF:S06; XREF:S07; XREF:S10]. With every paid window empty, everything in the registry keeps running at local quality and escalations queue as parked work — **this table is 3.9's "whatever remains feasible" boundary, made concrete** [R16 §4.3]. The `watchdog-disambiguator` never escalates beyond `unclear` (S2.7). If a duty's acceptance bar proves unreachable at this scale, the duty runs degraded-with-disclosure locally and only its *mandatory* path may shift to a paid seat; watchdog duties instead widen their `unclear` band — they never gain a cloud dependency [R16 §4 "What would change"].

### S12.6 The GPU broker (R10-OQ4 discharged)

Two caller planes [R16 §4.7, §3.4]:

- **Platform plane:** `sinet-control` and adopted watch organs reach llama-swap directly on loopback [S01.1]. All management verbs (load/unload/config) exist **only** on this plane.
- **Sandboxed plane:** sandboxes never see `/dev/nvidia*` and never a routable host address [R16 §4.7; XREF:S11]. A worker granted local inference receives exactly: `POST /v1/chat/completions` + `POST /v1/embeddings`; `/v1/models` returns only the run's allowlist; transport is a per-sandbox loopback/unix-socket bridge injected by the sandbox runtime [XREF:S11].

Binding properties:

- **Identity:** a per-run bearer token minted by the control plane at spawn (virtual-key *pattern*; no proxy product adopted) — model-alias allowlist + rate/size limits + expiry = run lifetime. The GPU broker resolves token → run → owner and writes every call to the D7 ledger ($0, pressure floor, itemized) [R16 §4.7; XREF:S10].
- **The model field carries a duty alias, never a raw model id** (S12.4).
- **Excluded by construction:** management verbs; logprobs for sandboxed callers (⚙ `local.broker.sandbox_logprobs`, default off — side-channel reduction); rerank/completions until a template needs them [R16 §4.7].
- **Confinement interaction:** C1 workers may be granted the bridge; C3 workers (v0.1) get tighter rate/size budgets — the broker's logs make steered inference auditable; C0 connectors never get it [R16 §4.7; XREF:S11]. gVisor nvproxy stays a non-path unless a future capability genuinely needs in-sandbox CUDA (pre-registered probe, unchanged) [R16 §4.7].
- **Admission:** before any model load, the GPU broker runs the S12.7 VRAM-ledger check — **these are the mechanics behind the scheduler's GPU-admission policy hook**; whether the *work* is admitted at all (pressure, budgets, background caps, priorities) is S10's decision [G2 D2.1; XREF:S10].

### S12.7 VRAM ledger & GPU preemption

- **Ledger values are per-machine measurements, never documented constants** [R16 §4.4]. They are produced by the R09-OQ8 protocol — 7 steps, carried by reference [R16 §4.4]: (1) pool enumeration by UUID; (2) **sleep-gate every read** — check `power/runtime_status` first; a suspended GPU is recorded as "asleep, 0 client bytes" and never polled (polling wakes it); (3) device truth from device-level `memory.free`, never per-process sums; (4) per-process attribution including graphics clients (compute queries miss every graphics process); (5) per-(model, quant, context, slots, engine build) footprint measured through a warm-up generation (KV + CUDA graphs only fully materialize then) + guard band ⚙; (6) compositor headroom logged per display mode — hybrid and MUX separately; **hybrid is the recommended, VRAM-maximizing mode**; (7) live-admission verification. Re-run on driver upgrade, engine bump, model/quant change, or display-mode change. TBD-BRINGUP(VRAM-ledger calibration run).
- **Admission math:** a load is admitted iff live `memory.free` ≥ ledger(model) + ⚙ `local.vram.guard_band_mb` — always the live reading, never the ledger's belief; after any forced kill, `memory.free` recovery MUST be verified before the next admission (zombie VRAM exists) [R16 §4.4].
- **Preemption is kill-not-freeze.** Freezing is categorically wrong for VRAM: a stopped process keeps its CUDA context and allocations; the driver frees memory only at teardown. Freeze/thaw remains the CPU/RAM tool [XREF:S10]; **unload/kill is the GPU tool** [R16 §4.5, §5]. Backend stop discipline: SIGTERM → ⚙ grace → SIGKILL — the kill path is mandatory, not a fallback (`llama-server` can wedge and ignore SIGTERM) — then verify recovery per the admission rule [R16 §4.5].
- **Granularity:** generation cancels between decode steps (~ms); a running prefill batch cannot be interrupted — worst-case *voluntary* preemption ≈ one prefill batch. When the operator needs VRAM now, the process is killed (the eager-unload verb, S12.2) [R16 §4.5]. One backend process per loaded model keeps CUDA-context overhead (~300–800 MB class) inside the measured footprint [R16 §4.4].

### S12.8 Power & residency policy

**Revision, stated:** report 09 §4.9 sketched a small always-resident GPU slot for background intelligence. **That sketch is REVISED by R16** [G3 digest]: NVIDIA RTD3's deepest state (VRAM off) requires used VRAM below the `NVreg_DynamicPowerManagementVideoMemoryThreshold` — default 200 MB, max 1024 MB — so **an always-resident multi-GB model and deep GPU sleep are mutually exclusive**; a resident slot would buy 7–18 W-class idle-awake (hundreds of Wh/day) for latency no duty needs — every registry duty is an event-driven annotation or batch pass, none latency-critical below ~10 s [R16 §2.6, §3.3, §4.6].

Policy instead (all ⚙) [R16 §4.6]:

- **On AC, active platform hours:** TTL warm residency — models stay loaded while duties flow, unload on TTL expiry; the OS page cache is the true residency layer (~2 s reloads for the 4B, ~3–7 s for the 9B, including spawn). The operator may pin the fast tier into the `resident` group during heavy runs (an operator verb, empty by default).
- **On battery:** nothing resident on the dGPU. Watchdog verdicts and the tiniest screens run on the **CPU tier** (burst CPU cost is fan-invisible); larger duties load-on-demand (accepting a wake) or park until AC per urgency class (⚙ `local.battery.gpu_admission`).
- **Batch passes** (compaction summaries, contradiction sweeps, benchmark evals) are **AC-only** (`ConditionACPower=` + a udev power-supply trigger for immediate reaction) and SHOULD run in the operator's daytime off-hours window — sustained batch is what raises fans; bursts are energetically trivial (~0.03–0.05 Wh) [R16 §2.6, §4.6].
- **Monitoring discipline:** every GPU read sleep-gates on `runtime_status` (S12.7); Tier-1 watching is event-driven with **no periodic GPU wake anywhere** — monitoring never defeats RTD3 [R16 §4.8].
- **Clock/power hygiene:** background inference runs under `-lgc` clock caps (`-pl` is unavailable on mobile GPUs); `nvidia-persistenced` in UVM persistence mode is NEVER run (it disables RTD3 outright) [R16 §2.6, §5].
- **The watchdog floor is never dark and never cloud:** Tier 0 = deterministic counters over the event log — no GPU, always on; Tier 1 = event-driven local verdicts, falling to the CPU tier when the dGPU is contended or asleep. **S2.7 and 3.9 hold by construction, not by a reserved slot** [R16 §4.5, §4.8]. Watchdog thresholds and machinery are [XREF:S14].

### S12.9 Bring-up battery (G3 Def.8)

The **T15 battery** is this section's acceptance instrument: built once at bring-up, versioned in the platform repo, run via the pinned eval runner against the *exact deployed quant and engine build*, results recorded in `platform.db` [R16 §4.9; XREF:S14 for runner + golden-set governance]. Contents: ~30–50 labeled cases per registry duty (synthetic seeds + real traces as they accumulate) plus the quant sanity check (`llama-perplexity --kl-divergence` on ~250K tokens of Sinet-domain text against the FP16 baseline). Named measurements:

1. TBD-BRINGUP(workhorse bakeoff) — Qwen3.5-9B vs Gemma 4 12B QAT on the battery over real Sinet-domain traces; **the 9B ships as default alias target unless the bakeoff flips it** [G3 Def.8].
2. TBD-BRINGUP(VRAM-ledger calibration) — the S12.7 protocol on the real machine: per-model footprints, both compositor-headroom figures (hybrid + MUX), unload/kill→free latencies [R16 §7-OQ4].
3. TBD-BRINGUP(CPU-tier throughput floor) — 3–4B Q4 tokens/s on the host CPU (the ~20–35 t/s figure is a class estimate with no citable laptop source); decides how much of the battery-mode policy is real and whether a 2B CPU downshift is needed [R16 §2.6, §7-OQ5].
4. TBD-BRINGUP(contradiction-screen P/R) — measured operating point of the DeBERTa→workhorse two-stage screen on realistic lesson pairs (R11-OQ7, narrowed); literature predicts high precision / weak recall with temporal/numeric blind spots [R16 §2.5, §7-OQ2].
5. TBD-BRINGUP(entailment thresholds + mandatory-coverage bar) — Guardian (3.3 vs 4.1) vs MiniCheck on ~200 Sinet-domain claim–citation pairs; sets the thresholds and the coverage bar, and decides whether the Flan-T5-0.8B CPU floor is adequate for sampled checks [G3 Def.4; R16 §7-OQ3].
6. TBD-BRINGUP(per-duty confidence calibration) — the S12.5 margin→isotonic→threshold fit for every registry duty [R16 §4.3].

### S12.10 Lifecycle: re-evaluation, swaps, recalibration

- **Cadence:** full re-evaluation every ⚙ `local.reeval.cadence_months` (6), or sooner on watchlist drift events — "new open ≤40B family" is a registered drift class on the provider watch [R16 §4.10; XREF:S14].
- **Method** [R16 §4.10]: shortlist from the current small-open index + adoption signals, **license verified on the HF card itself**; duty-relevant leaderboard pass (summarization grounding, entailment, SQL, structured/tool, judging — leaderboards churn as fast as models, so the method outranks any snapshot [R16 §2.2]); local KL quant check; the T15 battery at the deploy quant; **promotion rule:** a challenger replaces an incumbent only if it wins ≥2 duty suites with non-overlapping 95% Wilson intervals at equal-or-better tokens/s.
- **Swap ⇒ recalibrate is a hard gate (P-T15-1).** Every model, quant, or engine-build swap silently invalidates per-duty confidence thresholds and calibration maps. A swap MUST re-run the S12.5 calibration and the battery's affected duty suites *before* the new target goes live behind its alias; a swap without recalibration is a silent-quality regression [R16 §7-OQ8, §4.10].
- Every swap also fires the **7.3 revalidation trigger**: worker versions tuned against the swapped alias are flagged and revalidated before further unsupervised use [7.3; XREF:S08].
- **Serving-engine bumps** (llama.cpp `b`-tag, llama-swap release, Ollama if ever activated) follow the S03.3 deliberate-bump procedure; the local lane's conformance entries assert `/v1` logprobs, `json_schema` enforcement, and the llama-swap YAML/endpoint contract **behaviorally** on every bump — documentation is never the check (P-T15-2) [XREF:S03; XREF:S14; pins XREF:S16].
- Aliases make every swap invisible to workers, templates, and platform callers (S12.4, S12.6) [R16 §4.7].

### S12.11 Post-v0 envelope

- **eGPU pool:** the RTX 3090 24 GB enrolls as a second, separate VRAM pool (`pool24`); the per-duty ceiling rises to the 24 GB envelope post-v0 [G1 P9]. It is a **resident-model pool by design** — TB4 model load is ~7× slower while the runtime penalty is small; it is not a swap pool [R16 §2.6, §4.6].
- **Pre-registered pool24 seats** (re-run through S12.10 at enrollment, never auto-adopted): Qwen3.6-27B (quality), Qwen3.6-35B-A3B (throughput), Gemma 4 31B QAT (summarization-leaning), Arctic-Text2SQL-R1-14B/32B (SQL quality path) [R16 §2.1, §4.2].
- **Attach/detach:** `boltctl enroll` once; attach = plug → verify enumeration → start `pool24` members. Detach = choreographed runbook: unload pool → verify no `/dev/nvidia*` holders → unplug; cable-yank = assume reboot. Absence = health-check failure ⇒ pool marked absent ⇒ duties fall back to pool12/CPU or park — **no schedule may require the eGPU** (optional hardware) [R16 §4.6]. TBD-OPERATOR(eGPU on-rig validation at enrollment — token-rate penalty, load times, detach rehearsal; R16 §7-OQ6).
- **vLLM** is the pre-registered `pool24` batch backend under llama-swap, installed only if a real throughput need materializes [R16 §4.1].
- **Parked:** speculative decoding (first-party draft models exist; a battery entry only if duty latency ever matters) [G3 §Follow-ups; R16 §7-OQ12] · local image generation (12.1) — a separate SD-class serving stack; nothing here blocks it and the broker's alias pattern extends to it; no v0 work [G3 §Follow-ups; R16 §7-OQ11].

---

**Settings introduced (⚙):**

| Name | Default | Clamp/range | Ratified by |
|---|---|---|---|
| `local.ttl.fast_s` | 120 | 0–3600 `[coordinator-draft]` clamp | [R16 §4.1]; G1 rider 1 |
| `local.ttl.workhorse_s` | 300 | 0–7200 `[coordinator-draft]` clamp | [R16 §4.1]; G1 rider 1 |
| `local.ttl.egpu_s` | 1800 | 0–86400; dormant until pool24 | [R16 §4.1]; G1 rider 1 |
| `local.vram.guard_band_mb` | 512 | ≥128 `[coordinator-draft]` clamp | [R16 §4.4 step 5] |
| `local.unload.term_grace_s` | 5 | 1–30 `[coordinator-draft]` clamp | [R16 §4.5] |
| `local.gamemode_hook` | on | {on, off} | [G2 Def.6] |
| `local.battery.gpu_admission` | `urgent-only` | {never, urgent-only, always} — name `[coordinator-draft]` | [R16 §4.6] |
| `local.batch.ac_only` | true | {true, false} | [R16 §4.6] |
| `local.broker.sandbox_logprobs` | off | {off, on}, per-template | [R16 §4.7] |
| `local.reeval.cadence_months` | 6 | 1–12 | [R16 §4.10] |
| `local.alias.<duty>` (map) | per the S12.4 table; workhorse default = Qwen3.5-9B | changes only via the S12.10 swap gate | [R16 §4.7]; [G3 Def.8] |

All per G1 rider 1: operator-editable, audit-trailed, auto-adjustment only within operator ceilings, visible on receipts.

**Known problems owned here:**
- **P-T15-1 — model churn invalidates calibration** (id coined here for R16's first filed problem [G3 §Follow-ups]): dispositioned as the swap ⇒ recalibrate + revalidate hard gate (S12.10); no alias retarget goes live uncalibrated.
- **P-T15-2 — stack-capability drift; docs lie, probe behavior** (id coined here for R16's second filed problem [G3 §Follow-ups]): logprobs/`json_schema` are load-bearing for canaries and confidence; dispositioned as behavioral conformance probes on `/v1` at every engine bump (S12.2, S12.10); machinery [XREF:S14].
- Referenced, owned elsewhere: P-T01-2 (schema drift; [XREF:S03]) · P-T14-1 (engine-pin bumps = mass revalidation; [XREF:S08]) · llama-swap bus-factor-1 = accepted risk with the pre-registered fallback ladder (S12.2) and a `components.lock` exit plan [XREF:S16].

**Deferred / parked:**
- eGPU `pool24` activation + per-duty 24 GB ceiling → re-entry: hardware enrolled (S12.11) [G1 P9].
- vLLM batch backend → re-entry: measured throughput need on pool24 [R16 §4.1].
- Speculative decoding → re-entry: a duty latency requirement appears [G3 §Follow-ups].
- Local image generation (12.1) → re-entry: post-benchmark-gate content work; separate serving stack [G3 §Follow-ups].
- Local helper-report plausibility screen → re-entry: battery ≥ pre-registered bar on real helper outputs [G3 Def.3].
- Local fine-tuning → re-entry: the battery proves a persistent, material gap no off-the-shelf specialist closes [R16 §5].
- Automatic idle-detection (GPU auto-pause/resume) → re-entry: post-v0 [G2 Def.6].
- Ollama / router-mode fallback activation → re-entry: llama-swap unmaintained or contract break (S12.2 ladder) [R16 §4.1].
- 2B CPU downshift for the fast tier → re-entry: TBD-BRINGUP(CPU-tier throughput floor) outcome (S12.9).
- TBD-P3(GameMode user-context probe) → implementation start (S12.2) [R16 §7-OQ7].

**Coverage:**

| Feature-list item | Subsection |
|---|---|
| Operating reality — local GPU as permanent free tier; background intelligence local-only, zero allowance, works when windows are empty | S12.1, S12.4 |
| 3.9 never fully offline | S12.4, S12.5, S12.8 |
| 3.11 local resources scheduled; operator's interactive use always wins (GPU part; CPU/RAM [XREF:S10]) | S12.2 (eager-unload + GameMode), S12.6–S12.8 |
| S2.7 self-watching runs local, no allowance | S12.4 (`watchdog-disambiguator`), S12.8 (floor) |
| 1.10 utility model + ceremony cut line (map at S06.10) | S12.4 |
| 7.3 model-swap revalidation trigger (local side) | S12.10 |
| 12.1 local image gen (awareness only) | S12.11 (parked) |
| D4/D5 — free tier measured exactly, pressure floor (policy [XREF:S10]) | S12.1, S12.6 |

**Open items for G4:** none. Drafting-time notes flagged for G4 attention: (a) `[coordinator-draft]` items — the alias names (S12.4), the `sinet-llamaswap.service` unit name (S12.2), the clamp ranges and the `local.battery.gpu_admission` knob name (⚙ table); (b) resolved-tension note: G1 P9's "≤8B" is applied as the ≤8B-class / 12-GB-pool envelope — the reading R16 held fixed and G3 ratified by naming the 9B (Def.8) and 7B (D3.5) seats (S12.3); (c) P-T15-1/-2 ids are coined here for R16's two filed problems, for pickup by the S17 register.
