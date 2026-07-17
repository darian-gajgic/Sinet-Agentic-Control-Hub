# GATE-3 — Spec readiness

**Opened:** 2026-07-17 · **Wave covered:** C (final research wave) · **Status:** OPEN
**Reports in scope:** Research/15 (T14 worker ontology), 16 (T15 local models), 17 (T13 platform stack). Campaign totality: **all 17 topics researched and committed (reports 01–17)**; gates G0–G2 closed.

## Findings digest

### T14 — Worker ontology & domain agents (`Research/15-worker-ontology-and-domain-agents.md`)
- **Recommendation:** workers = rows + git-versioned files in a Sinet-owned superset schema, compiled per engine per invocation (hash-pinned, engine formats are compile targets never the store); **guardrail split** — behavioral content in template files, ALL enforcement state (grants, confinement class, budgets, gates) exclusively in control-plane tables recompiled every run; specialization via curated tools + 2–3 concise skills + injected knowledge (persona prose = replicated null with measured harm); composer = one-shot generation guided by a versioned best-practice playbook, through a 4-station battery (lint → deterministic permission audit vs task-class ceilings → sandboxed dry run → approval-as-diff) + supervised first-N; routing = visible, overridable classifier — no trained router at household n.
- **Key evidence:** no cross-vendor agent format exists (convergence only at Agent Skills + AGENTS.md layers); SkillsBench +16.2pp avg from curated skills vs −1.3pp self-generated; Claude Code's own plugin field-stripping is shipped precedent for the guardrail split; DGM's validator-sabotage record is the case for structural (not advisory) 14.2.
- **New problems:** P-T14-1 (engine-pin bumps are mass revalidation events — no provider clock announces them); P-T14-2 (worker definitions are a prompt-injection carrier class — 91% of ClawHub's malicious skills carried injection).

### T15 — Local-models layer (`Research/16-local-models-layer.md`)
- **Recommendation:** llama-swap (MIT, pinned) fronting pinned llama.cpp llama-server per GPU-UUID — full VRAM release on idle, manual unload verb = the ratified operator-wins switch; Ollama = capability-complete fallback; v0 set = Qwen3.5-9B workhorse + 4B fast tier (Apache 2.0; Gemma 4 12B QAT bakeoff alternate) + specialist seats (Granite Guardian entailment, DeBERTa CPU pre-screen, 0.6B embedder post-gate); GPU broker = OpenAI data plane only, per-run virtual keys, duty *aliases* (model swaps invisible to workers); kill-not-freeze for GPU preemption; 6-monthly re-evaluation with **mandatory threshold recalibration on every swap**.
- **Key evidence:** verification refuted its own draft claim — Ollama /v1 logprobs work since v0.12.11, the vendor's own docs were 8 months stale (→ "conformance suites assert behavior, never docs"); NVIDIA RTD3's 200 MB threshold makes an always-resident model and deep GPU sleep mutually exclusive → **report 09's resident-slot sketch REVISED** (TTL warm residency on AC, CPU tier on battery — watchdog floor never dark, never cloud).
- **Discharged:** R09-OQ8 (7-step VRAM-ledger measurement protocol), R10-OQ4 (broker interface), R11-OQ7 (narrowed to a bring-up measurement). 8B generalists judge below random on hard pairs (JudgeBench) → ceremony cut line keeps interviewing/plan-critique/verification-review on paid models.

### T13 — Platform stack & process architecture (`Research/17-platform-stack-architecture.md`)
- **Recommendation:** small owned core as systemd **system** units — `sinet-control` (sole DB writer, API + the one SSE endpoint, watchdog-supervised), separate credential broker, per-user engine units, per-run transient lanes that stream events and never touch the DB, port-pool daemon, adopted organs each their own unit; **five named seams** (storage / process / API / adapter / adoption) encode the anti-monolith lesson; backend **Go** (conditional on operator ratification — D3.2); frontend = disciplined React 19 + Vite SPA, PWA + Declarative Web Push; auth = tailnet wall → serve identity headers as device *hint* → app sessions + per-user PIN authoritative; settings = one schema-driven registry (validation + generated UI + docs) with `(value, floor, ceiling)` clamps + audit trail — the G1 rider made mechanical; native deploy, checksum + signed-tag CI (attestations are Enterprise-gated on private repos); sleep/wake = first-class duty.
- **Key evidence:** the survivor cohort (PocketBase, Miniflux, ntfy…) converges exactly on small-Go-core + SQLite + SSE while the funded control-plane cohort died at ~14 months; the Dec-2025 React RCE lived in RSC (a plain SPA doesn't ship it); no self-hostable Web Push service can exist (RFC 8030) — vendor relays see timing, never content; SQLite has no forcing shape at household scale on sqlite.org's own checklist.
- **New problems:** P-T13-1 (post-resume network-identity reconcile — tailscaled documented to sometimes not reconnect); P-T13-2 (listener-binding audit as a trust-chain invariant); P-T13-3 (accepted-external-observables register — CT logs, push metadata).

## Decisions required

### DECISION 3.1 — Approve P2 start (end of research waves → spec synthesis)
- **Context:** every topic the campaign planned is researched, validated, and committed; gates G0–G2 ratified the architecture direction, substrate, and adoption list. P2 = synthesize `Spec/core-architecture-v1.md` (feature list §15.1) from reports 01–17 + gate decisions. Entry sequence (already-owed items given their slot): (a) **operator prerequisites**, whenever convenient: Z.AI key into opencode (S3 steps) + the report-10 §7.3 host-probe afternoon (D2.3 contingency) + ts.net hostname choice (Default 5 below); (b) **P2-entry spike battery** (D2.9 scope + R15-OQ6 engine-lowering probes); (c) **benchmark pre-registration session** (D2.8; report 12 §4.6 draft); (d) **frontend workshop** (binding component picks incl. settings-form renderer); then spec drafting with per-section workshops consuming the reports. G4 reviews the finished spec and only the operator ends the research phase (CLAUDE.md flag).
- **Options:** A) Approve — P2 begins; spikes/prerequisites run at entry, spec drafting starts on what they don't block. B) Hold — campaign pauses complete-but-unstarted.
- **Recommendation:** A — nothing further is researchable; every remaining unknown is a measurement, a spike, or a spec-drafting decision.
- **Forecloses:** nothing; the research phase does not end until G4 (operator's explicit act).

### DECISION 3.2 — Backend language (R17-OQ1)
- **Context:** report 17 recommends **Go** on external evidence: the 14-year-honored go1compat promise (the only written decade-stability instrument in the field), compiler-enforced typing (the refactoring-safety bus factor 1 needs), single static binary + embedded assets (no interpreter EOLs, no 0.x package manager in the decade budget), pure-Go SQLite, GA anthropic-sdk-go v1, and near-total survivor-cohort convergence. The one input research cannot measure: **your personal velocity** — Nexus was Python/FastAPI, and you know how you work. Sinet's D3 architecture (engines as subprocess/HTTP, tools as runners) structurally quarantines Python's agent-library advantage. If Python: the posture is **Litestar/Starlette + Pydantic** (org-maintained), NOT FastAPI-as-decade-bet (0.x after 8 years, ~20× single-maintainer concentration), deployed via pinned uv. Everything else in report 17 is language-independent — a flip costs one subsection.
- **Options:** A) Go. B) Python (Litestar posture). C) Defer to the first spec workshop (delays only the shell section).
- **Recommendation:** A on the evidence — but this is genuinely your call; B is fully respectable if Go would slow you down enough to matter.
- **Forecloses:** nothing hard either way (the API/schema/seams are identical); language rewrites later are real cost, so deciding now beats drifting.

### DECISION 3.3 — Frontend architecture ratification (R17-OQ2)
- **Context:** report 17 recommends a **plain React 19 + Vite SPA in TypeScript — no meta-framework, no RSC/SSR** (the Dec-2025 RCE class lived in RSC; a tailnet app has no SEO case), installed as a PWA, notifications via Declarative Web Push (iOS 18.4+), with discipline riders (lockfile-pinned minimal tree, components in the P-T16-1 manifest, quarterly dependency pass, Vite majors on a lag). HTMX 2.x is the documented runner-up with named re-entry conditions. Binding component picks (dnd-kit vs alternatives, diff viewers, chat UI, settings renderer RJSF-vs-JSON-Forms) stay with the P2 frontend workshop.
- **Options:** A) Ratify SPA-with-discipline now; workshop makes component picks only. B) Leave architecture open; workshop decides SPA-vs-hypermedia + components together.
- **Recommendation:** A — the widget profile (drag-drop board, diff/monaco review surfaces, chat) sits in hypermedia's own declared weak quadrant; deciding architecture now lets the workshop be concrete.
- **Forecloses:** A → HTMX re-entry only via its named conditions · B → workshop scope doubles.

### DECISION 3.4 — Worker-composition triggers + supervised first-N (R15-OQ1 + OQ2)
- **Context:** when no existing worker fits a task, the platform can compose one (7.1). Report 15 proposes: **compose only when the work is recurring, schedule/trigger-registered, or explicitly requested** — one-off tasks run as generalist-with-injected-knowledge instead (gap records accumulate so a second occurrence surfaces a composition proposal); after approval, the first **⚙3** outputs require requester review in full-pipeline domains (degraded domains review everything anyway), reset when a new version changes body/equipment. Count-based graduation has no public prior art (industry uses time/traffic %) — it's a Sinet-specific formalization.
- **Options:** A) As proposed (compose-when-earned; first-N = 3, count-based). B) Compose more freely (also for one-offs). C) Add a time floor to graduation (e.g., N outputs AND 2 weeks).
- **Recommendation:** A — 14.4's machinery-when-earned made operational; N=3 is an operator-editable setting either way.
- **Forecloses:** nothing — all ⚙ settings.

### DECISION 3.5 — Layer-2 open SQL at v0 (R16-OQ10)
- **Context:** queryable history has 3 layers (report 12): deterministic views → canned queries → open text-to-SQL as escalation. Report 12 assumed Layer 2's open-SQL needed the 24 GB eGPU; report 16 found **Arctic-Text2SQL-R1-7B fits the 12 GB pool** at ~70% BIRD-class accuracy (open-model SOTA is ~74%; human 93%). Guardrails are settled regardless: read-only connection, allowlisted views, single-statement parse, LIMIT+timeout, audit-logged, answers flagged lower-confidence.
- **Options:** A) Enable at v0 on the 7B, flagged lower-confidence, measured by the bring-up battery. B) Hold Layer 2 eGPU-gated as report 12 assumed.
- **Recommendation:** A — the guardrail stack makes wrong SQL an annoyance, not a hazard; canned queries stay the reliability floor.
- **Forecloses:** nothing — a config flip either way.

### Defaults unless objected (adopted silently at gate close if unflagged)

1. **Shared-device policy (R17-OQ5):** PIN always required on shared devices; per-device trusted auto-login only on personal devices, operator-granted.
2. **`settings.changed` event type (R17-OQ7):** joins the report-12 §4.1 event-type contract at spec time.
3. **Helper-report screen (G1 rider resolution, R16 §4.9):** stays conformance-only at v0; a local plausibility screen becomes admissible post-v0 only when the T15 battery shows ≥ the pre-registered bar on real helper outputs.
4. **Entailment thresholds (G1 Def.2 / R16-OQ3):** Granite-Guardian-8B default seat; thresholds + mandatory-coverage bar set from the bring-up calibration set at the spec workshop; CPU floor (Flan-T5-0.8B) for sampled checks pending the same measurement.
5. **ts.net hostname (R17-OQ3, action note):** operator picks a bland, stable machine name before the first cert — it lands in public CT logs and anchors passkey RP-IDs; renaming later strands credentials.
6. **Push drill (R17-OQ4, action note):** week-one real-device drill on household phones at first deploy; acceptance = notified → glance → decide < 10 s; failure flips notifications to ntfy-with-relay per report 17 §4.10.
7. **Privileged resume-remediation path (R17-OQ6):** designed at spec time with T09 review (scoped polkit rule vs minimal root helper — new privileged surface, deliberate design).
8. **Workhorse bakeoff + VRAM ledger + CPU-tier throughput + contradiction-screen P/R (R16-OQ1/2/4/5):** scripted bring-up measurements, results recorded in the platform repo; Qwen3.5-9B ships as default alias target unless the bakeoff flips it.

## Decisions taken (filled at close)

| # | Decision | Chosen | By | Date | Notes |
|---|---|---|---|---|---|

## Follow-ups spawned

- **Known-problems list additions (spec):** P-T13-1..3; P-T14-1..2; R16's two (model-churn invalidates calibration → swap ⇒ recalibrate gate; stack-capability drift → behavioral conformance probes).
- **P2 entry sequence** (on D3.1 approval): spike battery (D2.9 scope + R15-OQ6 engine-lowering probes + R16-OQ7 GameMode user-context check) → benchmark pre-registration session (D2.8) → frontend workshop (components + settings renderer; R14-OQ4 + R17-OQ2). Operator hands-on prerequisites tracked in STATE.md standing follow-ups.
- **Standing reminder (operator-requested, G1 rider 2):** R06-OQ2 native micro-fanout resurfaces when the **adapter spawning section** is drafted at P2 — the coordinator must actively raise it there.
- **Spec section ↔ report map** (P2 working index): shell/deploy/auth/settings ← 17; state/recovery ← 08; metering/scheduling ← 09; sandbox ← 10; memory ← 11; observability/evals/watchlist ← 12; deliverables/review/git/backup ← 13; adoption manifest ← 14 §6.1 + `components.lock` (17 §4.8); workers/routing/automations ← 15; local tier ← 16; engines/adapters ← 01/02/05; orchestration ← 06; context ← 07; intake ← 03; verification ← 04.
- **v1+ items re-confirmed parked:** template signing (R15-OQ7); passkeys as auth enhancement; C3 web-reading tier (v0.1); memory proposal pipeline (v1 per D2.7); GitHub Pro (second member); speculative decoding; local image gen (12.1).
