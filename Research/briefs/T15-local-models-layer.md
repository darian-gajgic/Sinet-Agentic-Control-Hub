# T15 — The local-models layer (the permanent free tier)

**Wave:** C · **Depth:** FULL (medium breadth — model lists churn; focus on the architecture and the selection *method*, with a current snapshot) · **Report slug:** `local-models-layer`

## Scope
Operating reality (local GPU = permanent free tier: health watching, change detection, inbox risk-ranking, routine classification — costs nobody allowance, works when every paid window is empty), S2.7 (health watchdogs run local), 3.9 (never fully offline), 3.11 (GPU/VRAM/RAM/CPU arbitration; operator's interactive use always wins), 1.10 (utility/ceremony model candidates), 12.1 (future: local image gen — awareness only).

## Why this gates the spec
The free tier is what keeps the platform *alive* when paid windows are empty and what makes always-on watching affordable (S2.7). Hardware is fixed and modest (12 GB VRAM primary, optional separate 24 GB pool): model and serving choices are real engineering constraints, and the arbitration story (3.11) touches sandboxing (T09) and scheduling (T08).

## Core question
Which local models and serving stack (mid-2026) fit an RTX 5070 Ti Mobile 12 GB (+ optional RTX 3090 24 GB eGPU as a second, separate pool) for Sinet's permanent free tier — reliable background classification, health watching, risk-ranking, and summarization with structured outputs — and how are they served, hot-swapped, and scheduled beside sandboxes and the operator's own interactive use of the machine?

## Sub-questions
1. Current small/mid model landscape (snapshot + selection method): instruction models that fit 12 GB (quantized) and 24 GB respectively — current families and their honest capability tiers for classification, structured extraction, summarization, simple judging; benchmark sources that aren't vendor-run; the *method* for re-evaluating as models churn (this list will age fastest of all reports).
2. Reliability engineering for small-model duties: structured-output enforcement (constrained decoding/JSON schema adherence), confidence thresholds, escalate-to-paid patterns when local confidence is low (3.9's "whatever remains feasible" boundary).
3. Serving stack: current state of ollama-class, llama.cpp server, vLLM on a 12 GB laptop GPU (VRAM headroom, multi-model hot-swap latency, OpenAI-compatible APIs for the D3 adapter), embedding models if T10 approved any retrieval use; idle VRAM release (the laptop is also the operator's daily machine).
4. Dual-pool placement: iGPU-drives-display reality (dGPU free for compute), eGPU as a separate pool (no model spanning — bandwidth-limited per hardware context); placement policy per duty (watchdog vs bigger utility jobs); hotplug/absence tolerance (the eGPU is optional and sometimes disconnected).
5. Watchdog architecture (S2.7): deterministic monitors + tiny-model classifiers over traces — division of labor (what needs a model at all?); sampling cadence vs GPU wake cost; alert quality discipline.
6. Ceremony on local (1.10): which ceremony duties (interview drafting, restatement, receipt summaries, risk-ranking) can local models do at acceptable quality vs must ride the paid utility model — evidence-based cut line, revisited per model generation.
7. Arbitration (3.11): detecting operator interactive use (GPU contention signals), preempting/pausing local inference cleanly, resource limits on inference processes; scheduling batch local work into idle windows.
8. Power/thermals on a mobile laptop: sustained-inference realities (throttling, battery mode, fan noise as a household nuisance) — duty-cycling policies.

## Constraints that bind this topic
Operating reality (free tier must cost zero allowance and run unattended), 3.11 (operator always wins), D3 (local models ride the same adapter contract), D5 (local = the floor of consumption-pressure routing), no cloud fallback for watchdog duties (they must work when paid windows are empty — that's their reason to exist).

## Harvest-map items to verdict
None directly (Nexus used qdrant+ollama for memory — that stance is T10's). Note any serving-stack findings that affect the anti-harvest "mem0/qdrant-first" row.

## Sources to prioritize
Serving-stack docs + release notes (primary), independent local-model evaluations (2026 — the churn makes recency critical), GPU inference engineering posts for laptop/consumer hardware, structured-output tooling docs.

## Decisions this feeds
G3: free-tier model set + serving stack, watchdog architecture, arbitration policy. Spec: local-tier section, utility-model recommendation defaults (1.10).

## G2 addendum (2026-07-17) — settled inputs + the concrete duty list, cite don't re-research

G2 is CLOSED (`Research/decisions/GATE-2-substrate-and-adoption.md`); reports 01–14 plus both gate files are committed inputs. The free tier now has a **concrete duty list with acceptance bars** to benchmark within the ratified envelope (G1 P9: v0 ≤8B on the 12 GB pool; the 24 GB eGPU is a second envelope): (1) watchdog disambiguation — report 12 §4.4 tier-1: last-N-turns diagnostic template, grammar-constrained {loop|productive|unclear} verdict + confidence; annotates alerts, never gates; (2) watchlist triage — report 12 §4.5: changedetection.io's native LLM rules point at Sinet's local OpenAI-compatible endpoint, plus the second-pass classifier {lane, change class, severity, summary}; (3) canned-query intent-filling — report 12 §4.8 Layer 1 (grammar-constrained slot filling); open text-to-SQL at 14–32B on the eGPU is the stretch goal; (4) memory-pipeline duties — report 11: L1 observation distiller, lesson drafter (LESSON-requires-correction rule), and the §4.6 contradiction screen whose precision/recall on realistic lesson pairs is an explicit open question (R11-OQ7); (5) claim–citation entailment screen for the research domain (G1 Def.2 carry-over); (6) run-summary generation at run end (report 12 §4.2). Arbitration (SQ7) composes with the ratified report 09 §4.9 design: systemd slices, PSI-triggered batch pause, VRAM-ledger admission (R09-OQ8 — per-model footprints and compositor headroom are per-machine *measurements*; design the measurement method), and v0 operator-wins = manual eager-unload switch + GameMode hook (G2 Default 6 — idle-detection auto-pause is post-v0). This topic also owns the GPU-broker interface question (R10-OQ4): what API the broker exposes to sandboxes for inference/embeddings — sandboxes never get `/dev/nvidia*` (report 10); gVisor nvproxy on Blackwell mobile is unverified, test only if a sandbox-GPU path is ever wanted. The local lane rides the same D3 adapter contract and is the permanent floor of D5 pressure routing — settled, don't reopen.
