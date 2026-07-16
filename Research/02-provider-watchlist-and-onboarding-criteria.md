# T17 — Provider watchlist & onboarding criteria (extends report 01)

**Report date:** 2026-07-16 · **Depth:** FULL rigor, narrow scope (Wave A1b addendum) · **Method:** deep-research harness — 5 fan-out search angles (xAI audit / DeepSeek audit / Chinese-provider sweep / hosted open-weights flat lane / open-weights evals + watchlist evidence), primary-source fetching, adversarial verification pass over the 13 most load-bearing claims (verdicts: 6 SUPPORT, 3 PARTIAL with corrections folded in, 4 cross-source conflicts resolved). All URLs accessed 2026-07-16 unless noted. **Extends `Research/01-execution-engines-and-adapters.md`** — providers audited there are re-stated only where status changed; §2.1 below is the superset table the brief requires. Environment caveats: `x.ai/*` and `help.x.com` are Cloudflare-403 to this host's fetcher (fallbacks: `docs.x.ai` direct, search-indexed snippets, vendor-side docs, multi-secondary — marked per claim); `byteplus.com`/`docs.byteplus.com`, `z.ai/subscribe`, `volcengine.com` plan pages, `trae.ai/pricing` are JS-walled (indexed-text + corroboration used, marked).

---

## 1. Scope

Feature-list items covered (per brief T17):

- **2.7** — gap advice needs a *complete and refreshable* subscription-landscape table → §2.1 (updated consolidated table) + §6 (refresh process).
- **Operating reality** — the subscription-coverage rule applied to xAI, DeepSeek, the Chinese-provider field, and hosted open-weights services.
- **D3** — every new provider = a lane behind the same adapter contract; verified per provider (wrap vs API vs disqualified).
- **3.1 / 13.4** — per-model flat/metered flags as data → onboarding checklist Gate C encodes how a new plan populates them.
- **S2.8** — outside-world drift watch → §6 resolves report 01 open question #8 (refresh cadence + source list).
- Rider: harvest-map **O2** xAI/Grok OAuth claim → §8.

Constraints researched within: D2 (per-person, never pooled), D3, D4 (never model quota windows), D5 (two currencies), 3.10 (metered = explicit opt-in only), adopt-don't-fork. A provider with strong models but no sanctioned subscription-programmatic path lands in §7 with its would-change-this trigger.

---

## 2. Current state of the art (mid-2026)

### 2.1 Updated consolidated provider table (superset of report 01 §2.1)

Legend: **● NEW** row (absent from report 01) · **◆ CHANGED** since report 01 (change described in Notes) · unmarked = unchanged (carried from report 01; change-swept this session, no drift found, not re-audited in depth). Same columns as report 01.

| Provider | Plan | Programmatic surface on flat rate | First-party-only? | Headless sanctioned? | Notes |
|---|---|---|---|---|---|
| Anthropic | Pro $20 / Max $100 / Max $200 | Claude Code CLI incl. `-p`, IDE, web; Agent SDK (gray zone) | **Yes** (native apps; third-party discretionary) | Yes (`setup-token` documented) | Unchanged; credits change still paused (report 01 §2.1) |
| OpenAI | ChatGPT Free–Pro | Codex CLI `codex exec --json`, app-server, SDK | De jure unstated; de facto tolerant | Yes, documented; API key "the right way" | Unchanged; auto-ban enforcement-risk footnote stands |
| Google | AI Pro / Ultra | **None** (consumer Gemini CLI dead 2026-06-18; `agy` limited) | **Yes — hard** | Partially (agy) | Unchanged |
| ◆ Z.AI | GLM Coding Lite $18 / Pro $72 / Max $160 (−30 % promo → $12.60/$50.40/$112, **dollar digits secondary-only**) | Anthropic- + OpenAI-compatible endpoints inside official tool whitelist (now **20+ tools**: Claude Code, OpenCode, Cline, Crush, Goose…) | No — third-party is the product | Tool-scoped policy; no unattended-use ban | **CHANGED:** all tiers serve **GLM-5.2 + GLM-5-Turbo + GLM-4.7** since 2026-06-13; quota multipliers on 5.2/Turbo: 3× peak (14:00–18:00 UTC+8) / 2× off-peak, promo 1× off-peak through Sep 2026; ~80/400/1,600 prompts/5 h + weekly caps intact |
| ◆ Moonshot | Kimi memberships $19–$199 (names Moderato/Allegretto/Allegro/Vivace; exact name→price mapping MED-confidence) | Member API keys **explicitly for third-party tools** (OpenAI + Anthropic protocols); Kimi Code CLI | No | Not addressed | **CHANGED:** K2.7-Code shipped 2026-06-12; **K3 launched 2026-07-16** (hosted; Kimi Code `k3` gated to ~Moderato+ tiers, 256 K ctx; 1 M ctx on ~Allegretto+; API $3/$15 per M; open weights promised, not yet on HF); Agent Swarm / Kimi Claw features added |
| ◆ MiniMax | Token Plan $20/$50/$120 | `sk-cp` subscription key, OpenAI + Anthropic endpoints, 18+ listed tools | No | "Individual, interactive developer use"; batch may be throttled | **CHANGED (minor):** plan now covers "all models on the API platform" incl. **M3**; **overflow auto-spends prepaid credits** (1,000 credits = $1, 365-day validity) — 3.10 hazard, see §5 C1; promo ends 2026-07-31 |
| ◆ Alibaba | Model Studio Coding Plan **Pro $50 only** | 6,000 req/5 h, 45 k/wk, 90 k/mo behind expanded whitelist (now incl. OpenClaw, Hermes, Qoder, Cursor) | No (whitelist) | **NO — contractually banned** (verbatim clause intact: no "automated scripts, application backends, or other non-interactive scenarios") | **CHANGED:** Lite tier discontinued for new subs 2026-03-20; plan went multi-model (qwen3.7-plus, qwen3.6-plus, kimi-k2.5, glm-5, minimax-m2.5); ban + no-sharing unchanged → still NOT usable |
| GitHub | Copilot Free–Max | Copilot CLI programmatic mode | Officially yes | Yes | Unchanged; usage-based "AI Credits" since 2026-06-01 |
| ◆ Cerebras | Code Pro $50 / Max $200 | Plain OpenAI-compatible API key, any tool ("works with any AI-friendly editor or agent") | No | Neutral | **CHANGED (detail):** still **sold out** (all tiers, re-verified twice today); single model now **GLM-4.7** (~1,000 tok/s); quotas Pro 24 M tok/day · 50 RPM · 1 M TPM, Max 120 M/day · 120 RPM · 1.5 M TPM; hard daily reset, 429 on bursts, no refunds; restock announced only via pricing page/X/newsletter → §6 watch item |
| Cursor / Cognition | Pro $20+ / Devin | Cursor CLI print mode (own tool only) / none | Yes | Cursor yes | Unchanged |
| **● xAI (SpaceXAI)** | SuperGrok Lite $10 (no Build) / **SuperGrok $30** / **X Premium+ $40** / Heavy $300 | **Grok Build** — first-party coding-agent CLI (open-sourced Apache-2.0 2026-07-15) **plus xAI-sanctioned subscription OAuth in allowlisted third-party tools (OpenCode, Warp, Kilo Code)**; browser PKCE + headless RFC-8628 device-code | **No — per-partner allowlist** (third-party sanctioned, but per-tool, server-side revocable) | Device-code flow documented (opencode docs); AUP still bans bots *outside* the sanctioned lane | **NEW:** the May-2026 story — Build beta 05-14 (Heavy), expanded 05-25 to SuperGrok/Premium+; quota shape **unpublished** (third-party reports conflict: daily / rolling-2 h / monthly; "weekly pool" claim refuted in verification) — D4-compatible (observe, never model); **Grok 4.5 EU-blocked** at access date (ETA "later this month"), EU lane runs grok-build-0.1/grok-4.3-class; coding-data retention incident (fixed, retention now off by default); account sharing banned (per-person ✓); API separate metered (grok-4.5 $2/$6; grok-build-0.1 $1–2/$2–4, 200 k threshold); merged into SpaceX 02-02, rebranded SpaceXAI 07-06 |
| **● DeepSeek** | **No subscription exists** (chat/app free; API prepaid metered) | None on flat rate | n/a | n/a (consumer ToS bans automating the free chat) | **NEW:** metered-exception profile only — V4-Flash $0.14/$0.28, V4-Pro $0.435/$0.87 per M (cache-hit ~$0.0028/$0.0036); OpenAI + **official Anthropic endpoint** (`api.deepseek.com/anthropic`) with first-party Claude Code/OpenCode/OpenClaw guides; per-person API keys mandated; concurrency 500/2,500, 429 semantics; PRC governing law + China data residency (Berlin DPA DSA referral targets the *app*, not the API); legacy `deepseek-chat`/`deepseek-reasoner` IDs die **2026-07-24** |
| **● ByteDance (BytePlus)** | ModelArk **Coding Plan** intl: Lite **$10** / Pro $50 (the $5/$25 figures were a first-purchase promo, **suspended**); Team tier new | Lite ~1,200 req/5 h + 18 k/mo, Pro 5×; endpoint `ark.ap-southeast.bytepluses.com/api/coding/v3` (OpenAI-compat); official Claude Code doc page; tools: Claude Code, Cursor, Cline, Kilo, Roo, OpenCode | No (tool list) | **UNVERIFIED** — plan ToS JS-walled; anti-abuse clauses found (multi-account collapse, no refunds), no automation-ban text surfaced but not proven absent | **NEW:** models Dola-Seed-2.0 pro/lite, DeepSeek-V3.2, GLM-5.1, GLM-4.7, Kimi-K2.5, GPT-OSS; **hazard: same key on plain `/api/v3` bills metered outside the plan** (§5 B5); Germany/EU purchasability unresolved ("availability may vary by region") → candidate pending spike, not a lane yet |
| **● Tencent** | Cloud TokenHub Coding Plan — **mainland-only** (¥40 / ¥200); intl "Large Model Token Plan" = **coming soon** | 1,200–6,000 req/5 h + weekly + monthly windows; `sk-sp` keys; **dual OpenAI + Anthropic endpoints**; whitelist incl. Claude Code, OpenCode, Cline, OpenClaw | No (whitelist) | **NO — contractually banned** ("forbidden for automated scripts, custom app backends, or any non-interactive batch calls"; sharing strictly banned) | **NEW:** resells partner models (GLM-5, Kimi-K2.5, MiniMax-M2.5, Hunyuan retiring); mainland account + CNY required → NOT usable today; watch the intl launch *terms*. Sidebar: CodeBuddy intl ($9.95 promo) is own-agent credits — but ships an **HTTP API Beta** (REST + ACP) explicitly for webhooks/daemons — own-agent headless pattern worth watching |
| **● StepFun** | **Step Plan** intl: Flash Mini $6.99 / Plus $9.99 / Pro $29 / Max $99 | 100/400/1,500/5,000 prompts per 5 h + weekly ≈4× (1 prompt ≈ 15–20 model calls); dedicated endpoint `api.stepfun.ai/step_plan/v1` (OpenAI-compat); tools: OpenClaw, Claude Code, Cline, Goose, Kilo, Roo (+ OpenCode/Zed integration pages); **Stripe for overseas** | No (tool list) | Tool-scoped: paid-service agreement has **supported-tools-only clause** + anti-circumvention (scripts/proxies/token-rotation/abnormal concurrency) + sharing/resale ban — but **no interactive-only/automation ban** | **NEW:** models are **flash-class only** (step-3.7-flash: AA v4.1 index 30, #1 speed 378 tok/s, verbose) — a cheap-fast lane, not a frontier lane; non-refundable; MCP unsupported at launch |
| **● Xiaomi** | MiMo Token Plan global: $6/$16/$50/$100 per mo (monthly token-credit quotas 4.1B–82B; off-peak 00:00–08:00 CST ×0.8) | API key usable in "programming tools (such as OpenClaw, OpenCode, etc.)"; Claude Code/Codex listed; endpoint `api.xiaomimimo.com` | No (tool list) | **NO — contractually banned** ("prohibited… automated scripts and custom application backends") | **NEW:** mimo-v2.5-pro/-v2.5 + speech models (ASR/TTS) — unusual multimodal inclusion; USD pricing, overseas invoices; same restricted class as Alibaba/Tencent → NOT usable |
| **● Synthetic** | $30/mo or $1/day **per pack**; stackable packs | **500 requests / 5 h per pack**, 1 concurrent per model per pack; UI + API; OpenAI-compat + **Anthropic-compat `api.synthetic.new/anthropic/v1`**; 7 agents documented (Claude Code, OpenClaw, Opencode, Crush, Copilot, Octofriend, Xcode) | No — any-agent is the product | Agentic API use affirmatively documented; no automation ban; sharing banned | **NEW:** current standard lineup 7 models — **GLM-5.2, Kimi-K2.7-Code, MiniMax-M3, Qwen3.6-27B, gpt-oss-120b, Nemotron-3-Super-120B, GLM-4.7-Flash** (lineup churns); no-train, 14-day API-data deletion, GDPR page; US entity (Synthetic Lab, Co.; glhf.chat rebrand, ~2 yr history) — **tiny-team risk**; best model-currency of any neutral host |
| **● NanoGPT** | $8/mo open-model subscription | API (OpenAI-compat) + web; **60 M input tok/wk, 5,000 req/day, 60 k req/mo, 10 concurrent** | No | Personal agent use not prohibited; clause bans running it as a backend for another service | **NEW:** 200+ open models incl. GLM-5/K2.5-class; small crypto-adjacent operator; changed limits on 2 days' notice (Feb 2026) → budget/fallback lane with volatility flag |
| **● Featherless** | Premium $25 / Agent Std $100 / Agent Pro $200 | OpenAI-compat API; **unlimited tokens, concurrency-capped** (4/8 concurrent) | No | Neutral | **NEW:** $25 tier capped at **32 K context** — disqualifying for agent work; model catalog trails ~1 gen (K2.6/GLM-5.1); plan lineup itself shifted recently (MED confidence) |
| **● Chutes** | Plus $10 / Pro $20 (Base $3 apparently retired) | API on Bittensor subnet infra | No | Thin ToS, largely silent | **NEW — REJECTED as flat lane:** since 2026-02-27 quota = **5× PAYG-value caps over rolling windows and overage AUTO-CONVERTS to pay-as-you-go** — a built-in silent metered path (3.10 violation unless zero-balance discipline); TAO/crypto payment emphasis; single-validator infra; stuck-session anecdotes |
| **● Poe (Quora)** | $20+/mo (points pool) | **Subscriber API**: OpenAI-compatible `api.poe.com/v1` (+ Anthropic-compatible variant documented), explicitly sanctioned for third-party tools (Cursor, Cline, Roo, `llm` CLI) | No | Sanctioned | **NEW:** flat price but **points-metered interior** (~provider cost per call; web + API share the pool) → bounded hybrid lane at best; open-weights + frontier models both present |
| **● Hugging Face** | PRO $9 / Team $2-per-seat credits | `router.huggingface.co/v1` (OpenAI-compat) across Inference Providers | No | Neutral | **NEW:** included credits are **$2/mo** (PRO) then PAYG continues — a micro top-up, not a lane |
| **● Mistral** | Le Chat Pro $14.99 / Team $24.99 | **None** — subscriptions include "all-day coding" only via their own Vibe CLI/IDE surfaces (fair-use); **no API allowance in any Le Chat plan**; La Plateforme separately metered | Yes (own tools) | Undocumented for third-party/headless | **NEW:** EU-domiciled provider (strategically interesting for this household) but currently not a programmatic lane; watch for a coding-plan product |
| **● Perplexity** | Pro / Max | API-credit perk ($5/mo Sonar) **conflictingly reported, likely ended Feb 2026**; API pricing docs are silent on it | n/a | n/a | **NEW:** COULD NOT VERIFY as current; Sonar models closed-weights anyway → not a lane |
| **● Meituan (LongCat)** | No subscription — PAYG + 30-day prepaid token packs (daily flash sales) | Metered only; OpenAI + Anthropic endpoints on longcat.chat platform | No | n/a | **NEW:** LongCat-2.0 (2026-06-30) is near-frontier open-weights; heavy community Claude Code/OpenClaw use; watch for a plan launch; intl signup unverified |
| **● Metered-only sweep** | Groq, Together, Fireworks, DeepInfra, Novita, Nebius, Hyperbolic, Baseten, Parasail, OpenRouter | No flat programmatic subscription at any of them (2026-07-16) | n/a | n/a | **NEW (aggregate row):** OpenRouter = prepaid credits, free-model tier 50–1,000 req/day is a free tier, not a sub; Groq free tier exists; Nebius/Hyperbolic/Baseten/Parasail are sweep-level negatives (not page-audited — residual uncertainty flagged) |

### 2.2 xAI: the audit in brief

The single biggest landscape change since report 01. Timeline (multi-source, §10 nos. 1–16): Grok Build CLI beta for Heavy 2026-05-14 → expanded to SuperGrok $30 and X Premium+ $40 on 2026-05-25 → xAI publishes "Use Grok in OpenCode" (05-21) and "Use Grok in Warp" → CLI open-sourced Apache-2.0 2026-07-15. Auth is subscription OAuth (browser PKCE; RFC-8628 device-code for headless), consumption draws from the subscription's quota (shape unpublished; conflicting third-party reports; the "weekly pool" phrasing failed adversarial verification — treat as *undocumented*, which D4 handles by design). The consumer ToS bans account sharing (per-person ✓ D2) and the AUP still bans bots/automation *in general* — the sanctioned lane is a carve-out enforced as a **server-side per-partner allowlist** (OpenCode, Warp, Kilo Code named; pre-expansion Heavy-only 403s in hermes-agent #26847 / openclaw #84504 prove the allowlist is enforced and can change without notice). Model standing: Grok 4.5 (2026-07-08) is near-frontier per independent evals (AA v4.1 ≈54 at launch — 4th among flagships; SWE-bench Verified 86.6 % per vals.ai; Terminal-Bench 2.1 83.3 % single-source; a Cursor-data contamination flag exists for CursorBench-derived scores) — **but is EU-blocked on all xAI surfaces incl. Grok Build and the API console at access date** (promise slipped from "mid-July" to "later this month"). A Grok Build coding-data retention incident (uploaded SSH keys/password DBs retained; xAI turned retention off by default and deleted the data) is directly relevant to a household platform's risk posture.

**Sinet-relevant readout:** xAI is now the **third major with a sanctioned subscription-programmatic path, and the first whose sanction covers third-party tools by name** — it lands in the Z.AI-class "third-party-open (allowlisted)" camp, not the Anthropic/Google "first-party-only" camp. Report 01's structural premise (two first-party-only majors) survives unchanged.

### 2.3 DeepSeek: the audit in brief

No subscription product exists anywhere — consumer chat/app entirely free (and its ToS bans automating it), API entirely prepaid metered (§10 nos. 17–25). Under Sinet's rules DeepSeek is **metered-exception-only** — but it is the *best-shaped* metered exception in the field: official Anthropic-compatible endpoint with a first-party Claude Code setup guide, official OpenCode/OpenClaw integration docs, per-person API keys mandated by ToS (D2-aligned), effectively unlimited concurrency for a household (500/2,500), V4 prices post-cut ($0.14/$0.28 Flash, $0.435/$0.87 Pro, ~99 % cache-hit discounts; off-peak windows abolished ~2025-09). Model relevance is real (V4-Pro in the open-weights top cluster, MIT weights). Caveats: PRC governing law + China data residency (route no household personal data); Berlin DPA's DSA referral targets the consumer app, not the API; legacy model IDs die 2026-07-24.

### 2.4 The Chinese coding-plan wave

Z.AI's GLM Coding Plan template became a product category in Q1–Q2 2026: ByteDance (Volcengine domestic + BytePlus intl), Tencent (TokenHub, mainland), StepFun (Step Plan, intl), Xiaomi (MiMo Token Plan, global) all launched coding-plan subscriptions with tool whitelists that now routinely include OpenClaw, Claude Code, and OpenCode (§10 nos. 26–41). The decisive differentiator is a single ToS clause, which splits the field into three classes:

- **Class 1 — open programmatic:** plain API key or sanctioned OAuth, any tool or broad allowlist, no automation restriction. Members: xAI (allowlisted OAuth), Cerebras, Synthetic, NanoGPT, Featherless, DeepSeek (metered).
- **Class 2 — tool-scoped, no unattended-use ban:** key usable only in whitelisted coding tools; nothing bans non-interactive/agentic operation inside them. Members: Z.AI, Moonshot/Kimi, StepFun, (MiniMax, with its "individual, interactive developer use" framing sitting at the class boundary — flagged since report 01).
- **Class 3 — interactive-only, automation contractually banned:** verbatim bans on "automated scripts, application backends, non-interactive scenarios". Members: Alibaba (unchanged), Tencent TokenHub, Xiaomi MiMo. **Structurally incompatible with Sinet** — the platform is a supervised *non-interactive* orchestrator; these plans are disqualified regardless of technical fit, each with a watch trigger (clause removal / intl relaunch under different terms).

BytePlus is the one unclassified candidate (ToS unreadable this session, no ban text surfaced, EU purchasability unknown) — pending an operator spike, not a lane.

### 2.5 The hosted open-weights flat lane

Real but thin and volatile (§10 nos. 42–51). Cerebras Code — the quota-per-dollar leader (24 M/120 M tok/day) — remains **sold out** with restocks announced only via its own pages. The buyable field today: **Synthetic** (cleanest: current-gen models, dual protocols incl. Anthropic-compatible, agentic use affirmatively sanctioned, EU-friendly policies; tiny team), **NanoGPT** ($8 budget lane, transparent hard caps, volatile), **Featherless** (unlimited tokens but 32 K ctx at the affordable tier — unusable for agent work), **Chutes** (rejected: overage auto-converts to PAYG — a structural 3.10 violation). Poe and HF PRO are hybrid/micro shapes, not load-bearing lanes. Every audited service changed price or quota shape at least once in the last 9 months; two (Chutes, Alibaba) restructured in ways that killed or broke their flat guarantee; one (BytePlus) exposes a same-key metered endpoint one path-segment away from the plan endpoint. **A flat plan is a data point with a shelf life, never a stable fact** — this is what §5/§6 encode.

### 2.6 Open-weights frontier snapshot (thin — routing awareness; depth belongs to T15)

Per Artificial Analysis Intelligence Index **v4.1** (re-baselined 2026-06-15 toward agentic workloads — cite only v4.1 numbers; the April v4.0 figures circulating in release articles are incomparable) and corroborating boards, as of 2026-07-16:

| Family | Newest (date) | License | AA v4.1 (open rank) | Where it's flat-serveable today | Own-GPU? |
|---|---|---|---|---|---|
| Zhipu GLM-5.2 | 2026-06-13 | MIT (753B MoE) | 51 (#1 open) | Z.AI plans, Synthetic; GLM-4.7 on Cerebras (sold out); GLM-5.1 on BytePlus | No (hosted class) |
| MiniMax M3 | 2026-06-01 | Custom restricted (commercial needs agreement) | 44 | MiniMax plan, Synthetic | No |
| DeepSeek V4 Pro / Flash | 2026-04-24 | MIT (1.6T/49B act.; 284B/13B) | 44 / 40 | No flat lane (metered only); V3.2 on BytePlus/NanoGPT-class | No (even Flash) |
| Moonshot Kimi K2.6 / K2.7-Code; **K3 (launched today, weights pending)** | 04-20 / 06-12 / 07-16 | Modified MIT; K3 TBD | K2.6 44; K3 ≈57 (launch-day) | Kimi plans (K3 tier-gated), Synthetic (K2.7-Code), BytePlus/Tencent (K2.5) | No |
| Xiaomi MiMo-V2.5-Pro | Dec 25–Mar 26 | Unverified | 42 | Xiaomi plan only (class 3 — unusable) | No |
| Meituan LongCat-2.0 | 2026-06-30 | "Open" (exact license unverified) | Near-frontier (VentureBeat; pre-reveal OpenRouter leader) | None (PAYG only) | No |
| Qwen3.5 / 3.6 | Feb–Apr 2026 | Apache-2.0 (3.7 Max is closed) | 34 @ 397B | Synthetic (3.6-27B); Alibaba plan (class 3) | **Yes: Qwen3.6-27B ≈ consensus best 24 GB local coder (Q4 ~18 GB); Qwen3-Coder-30B-A3B** |
| Mistral Devstral 2 / Small 2 24B | Jun 2026 | Mod-MIT / Apache-2.0 | Vendor-only SWE claims (flagged) | None flat | **Yes: Small 2 fits 24 GB (Q4 ~14 GB), marginal 12 GB** |
| OpenAI gpt-oss-120b / 20b | 2025-08 (no successor) | Apache-2.0 | Aging mid-pack | Synthetic, BytePlus (120b/OSS) | **Yes: 20b = default 12–16 GB-class pick** |
| Meta Llama 4 | Apr 2025 (last open; "Llama 5 open" UNVERIFIED, low-tier sources only) | Llama license | Not competitive on 2026 coding boards | — | Scout heavily quantized on 24 GB |
| Baidu ERNIE 4.5 (open) / 5.1 (closed) | Jul 2025 / 2026 | Apache-2.0 (4.5) | Off-board | — | 21B-A3B variants on 24 GB |
| Ant/inclusionAI Ling-2.6-1T | ~Apr 2026 | MIT | Not board-placed | None (free/metered APIs) | No |
| xAI | Grok 2.5 weights only (Aug 2025); Grok 3 open-sourcing promise unfulfilled | Custom | Irrelevant for routing | n/a | No |

**T15 flag-forward:** own-GPU candidates worth T15 depth: Qwen3.6-27B + Qwen3-Coder-30B-A3B (RTX 3090 24 GB), Devstral Small 2 24B (24 GB), gpt-oss-20b (12 GB class on the 5070 Ti). Everything frontier-competitive is hosted-only. Benchmark caveats: SWE-bench numbers in circulation are mostly the vendor-aggregate tracker (Scale's standardized SWE-bench Pro board runs 10–30 pts lower and reorders); Aider polyglot is stale (last update 2025-11-20); every AA number must carry its access date — the index re-baselines continuously.

### 2.7 Structural readouts

1. **Report 01's premise holds:** exactly two majors (Anthropic, Google) are first-party-only. xAI joins as a third-party-open (allowlisted) major; nothing new requires a wrap lane.
2. **Sanction is increasingly allowlist-shaped** — xAI per-partner OAuth allowlist, Z.AI/StepFun/BytePlus/Tencent/Xiaomi/Alibaba tool whitelists. Allowlists are server-side enforceable and revocable without notice (xAI's May 403s; Anthropic's Jan–Apr arc from report 01). This is a new named platform problem (P-T17-1, §9).
3. **Window-shape convergence:** 5 h rolling + weekly caps is the dominant quota grammar (Anthropic-mirroring: Z.AI, MiniMax, Kimi, StepFun, Synthetic, Tencent). Outliers: tokens/day (Cerebras), monthly token-credits (Xiaomi), weekly tokens + daily requests (NanoGPT), concurrency-only (Featherless), undocumented (xAI). All are data for consumption-pressure routing (D5), never prediction targets (D4).
4. **Churn calibration for §6:** in the ~6 months covered — 1 provider category was born (Chinese coding plans), 2 flat guarantees broke (Chutes restructuring, MiniMax credit-overflow), 1 tier died (Alibaba Lite), 1 promo was suspended (BytePlus), 1 leader stayed sold out (Cerebras), 1 sanction expanded (xAI third-party OAuth), 1 model line launched same-day-as-research (K3). Monthly-or-faster drift is the confirmed baseline.

---

## 3. Candidate approaches

How the new landscape maps onto Sinet's lanes (D3: every provider = configuration + an existing lane, never a new architecture):

**A. xAI as an opencode-lane provider (config-only).** opencode ships the sanctioned path natively: SuperGrok browser OAuth + headless device-code + API key (docs verbatim, PR #28557 merged 2026-05-21, endorsed by xAI's own announcement). If a household member holds SuperGrok/X Premium+, the lane costs: one provider entry per user, an auth canary (allowlist revocation risk), and the EU model-list caveat. No new substrate. *Trade-off:* quota shape undocumented (D4 tolerates); policy risk concentrated in a revocable allowlist; EU currently gets grok-build-0.1/4.3-class models only.

**B. Grok Build as a third substrate (wrap lane).** Rejected for now: it would add a third conformance suite for a lane opencode already covers with xAI's blessing. Becomes relevant *only* if xAI de-allowlists opencode while keeping Grok Build open — pre-register that trigger. (Grok Build being Apache-2.0 and no-upstream-PRs is adopt-don't-fork-compatible if ever needed.)

**C. Synthetic as the hosted open-weights lane (config-only).** Dual-protocol endpoints (incl. Anthropic-compatible), agentic use sanctioned, current-gen open models, EU-workable policies. Fits the 2.7 gap-advice answer for "open-frontier capacity without a mainland plan". *Trade-off:* tiny operator (pair with a named fallback: NanoGPT budget lane, or Cerebras on restock); per-pack concurrency of 1/model shapes scheduling.

**D. StepFun as a cheap-fast lane (config-only).** $6.99–9.99 buys a class-2-sanctioned, Stripe-payable, OpenAI-compatible lane — but flash-class models only (AA 30). Useful as a bulk/background tier for low-stakes work classes, not as frontier capacity. *Trade-off:* supported-tools-only clause (opencode is on the list — wait, verify: its list names OpenClaw/Claude Code/Cline/Goose/Kilo/Roo + integration pages incl. OpenCode/Zed — opencode is covered via integration docs); anti-circumvention clause forbids concurrency games.

**E. DeepSeek as the designated metered exception.** Pre-register it in the 3.10 config as the default explicit-metered fallback: cheapest agentic-competent tokens in the field, official dual-protocol support, per-person keys. Policy rider: no household personal data (PRC residency).

**F. Watch-only candidates (no action):** Cerebras (restock), BytePlus (ToS + EU spike first), Tencent intl (launch terms), Mistral (EU provider, no lane yet), LongCat (plan launch), Kimi K3 open-weights drop (T15 relevance).

---

## 4. Recommendation for Sinet

**Cross-check verdict (brief SQ8): NO — nothing found changes report 01's recommendation.** The dual substrate (opencode API lane + wrapped `claude` CLI Anthropic lane) stands, *strengthened*: every new sanctioned provider found this session — xAI, Synthetic, StepFun, NanoGPT, BytePlus-pending, and the changed Z.AI/Kimi/MiniMax plans — enters as an opencode provider config (OpenAI- or Anthropic-compatible endpoint, or opencode-native OAuth in xAI's case). Zero new substrates. Zero architecture deltas. The wrap-or-API split's boundary cases resolve the same way they did in report 01.

Concretely:

1. **Adopt the three-class ToS taxonomy (§2.4) as spec vocabulary.** Class 3 (automation-banned) is auto-disqualifying; class 2 requires the lane to run inside a whitelisted tool (opencode/Claude Code) and carries a documented gray-zone note; class 1 is unrestricted. This turns the vaguest part of provider vetting into a mechanical test.
2. **Encode §5 (onboarding checklist) and §6 (watchlist) in the spec.** They are the durable artifacts of this report; the provider table itself is data with per-row `verified-on` dates, refreshed per §6 — never a spec constant.
3. **Lane actions now (operator purchase decisions, not architecture):** (a) if any member holds SuperGrok/X Premium+, wire the xAI lane through opencode's sanctioned OAuth with an auth canary — for an EU household, expect grok-build-0.1/4.3-class until the EU release lands; (b) name Synthetic the primary hosted-open-weights candidate for 2.7 gap advice (with NanoGPT as budget fallback and Cerebras as restock-watch); (c) pre-register DeepSeek as the explicit metered exception with its price rows in the D5 table.
4. **Per-model flat/metered flags (3.1/13.4) get two new required attributes** learned this session: `overflow_mode` (hard-stop / opt-in-credits / auto-metered — the last requiring zero-balance proof or lane rejection) and `region_model_gate` (account-observed model list vs provider-global docs — Grok 4.5 EU lesson).

**What would change the decision:**

- **xAI de-allowlists opencode** → the lane dies config-only; decision point: wrap Grok Build (Apache-2.0, viable) or drop the lane. Pre-registered in §3-B.
- **Cerebras restocks** → it becomes the quota-per-dollar leader for GLM-4.7-class work; slot it above Synthetic for bulk classes if terms hold.
- **Tencent/BytePlus international terms arrive clean (no class-3 clause, EU-payable)** → cheap multi-model lanes worth onboarding via §5.
- **A class-3 provider drops its automation ban** → re-run §5 Gate A on it.
- **Anthropic credits un-pause** (report 01's standing trigger) → unchanged response plan; this report adds nothing to that decision but the watchlist (§6) is its detection mechanism.

---

## 5. Provider onboarding criteria — the checklist (spec-ready)

*A candidate provider/plan becomes a Sinet lane only by passing every gate below. The audit is a dated document (stored with the platform state); every entry cites primary URLs + access dates. Marketing pages never count as sanction; only ToS/usage-policy/plan-terms pages do.*

**Gate A — Sanction (ToS, primary-source):**
- **A1. Capture:** plan page + ToS/usage-policy/paid-service-agreement URLs fetched, dated, key clauses quoted verbatim in the audit doc. If pages are JS-walled/unfetchable, the audit is *incomplete* — the lane stays pending (BytePlus precedent).
- **A2. Classify the automation stance** (three classes, §2.4): CLASS 1 open-programmatic → pass. CLASS 2 tool-scoped without unattended-use ban → pass with gray-zone note *only if* A3 holds. CLASS 3 explicit interactive-only/no-automation clause → **REJECT** (record the clause + a would-change trigger).
- **A3. Whitelist fit:** if tool-scoped, the whitelist (or sanctioned-OAuth allowlist) must include a substrate Sinet already runs (opencode or Claude Code). A lane must never force adopting a new engine (default reject; operator may grant an exception knowingly).
- **A4. Per-person viability (D2):** each household member can hold their own account; sharing bans are expected and fine; pooling is never needed; payment rails work from Germany/EU; no mainland-account/real-name barrier.
- **A5. Sanction mechanism recorded:** open API key / partner-allowlisted OAuth (**revocable — requires auth canary, see procedure**) / first-party-tool-only. This field drives the lane type and its canary design.

**Gate B — Technical fit (D3):**
- **B1. Protocol:** documented OpenAI- or Anthropic-compatible endpoint (exact base URL) → rides the opencode lane as config. Else: a first-party tool with a real machine surface (headless JSON, resume, usage reporting) → candidate wrap lane, which demands explicit justification (new conformance suite = real maintainer cost). Neither → **REJECT**.
- **B2. Headless auth:** API key or device-code OAuth that works without a browser; storable in the per-user credential store outside task sandboxes (D2); revocable per person.
- **B3. Usage fidelity (D4):** per-response token usage fields (or a usage API) for cross-checking Sinet's own metering. Absent → lane runs on Sinet-side counting only; mark `usage_fidelity: degraded`.
- **B4. Limit observability (3.2):** documented 429/limit-error semantics and reset signals the adapter can park on. Unpublished quota *windows* are acceptable (D4 reacts, never predicts). Silent throttling/quality degradation with no signal → flag; two independent reports of it → reject.
- **B5. Endpoint discipline:** if the provider exposes both a plan endpoint and a metered endpoint, test what the plan key does against the metered endpoint (BytePlus lesson: same key on `/api/v3` bills outside the plan). The adapter pins the plan URL; the audit records the failure mode.

**Gate C — Economics & risk (D5, 3.10):**
- **C1. Overflow behavior classified:** `hard-stop` (best) / `opt-in credits` (acceptable — maps to the 3.10 explicit flag) / **`auto-metered overflow`** (Chutes, MiniMax-credits pattern) → acceptable *only* with a proven disable or zero-balance state; otherwise **REJECT**. Receipts must visibly change currency when overflow triggers (report 01 P-T01-3).
- **C2. Price-table row:** the provider's own per-token API prices populate the D5 API-equivalent table; if the provider has no metered API (pure-sub), record the documented proxy used and its rationale.
- **C3. Window shape recorded as data:** 5h/weekly/daily-tokens/monthly-credits/concurrency-only/undocumented — consumed by consumption-pressure routing and the receipts, never by prediction.
- **C4. Churn & exit:** note the provider's change history (this report's §2.7-4 is the baseline); confirm exit = config-only (delete provider entry, revoke key, no code changes); wire the provider into §6's watchlist *before* go-live.
- **C5. Data/compliance note:** jurisdiction + governing law, training-on-inputs default, retention behavior (xAI incident precedent), data residency (PRC lanes) → feeds the per-lane routing policy for personal/household data.

**Onboarding procedure (the checklist's runtime):** watchlist candidate → Gate A–C audit doc → operator approval (D10) → config-only integration (provider entry per user + 3.1/13.4 billing flags incl. `overflow_mode` + D5 price row) → **auth canary + conformance canary registered** (a cheap scheduled probe per lane that distinguishes limit-park (3.2) from policy-revocation (alert operator — P-T17-1) and diffs the account's *actual* model list against config (P-T17-3)) → live, with the provider's §6 watch items active and a `verified-on` stamp in the 2.7 table.

**Re-audit trigger:** any §6 hit on an active lane (price/terms/model/endpoint change) → re-run Gate A + C1 before the next scheduled run on that lane consumes allowance.

---

## 6. Watchlist: sources + cadence (resolves report 01 OQ#8)

**What the 2026 evidence says moves first** (five drift events analyzed, §10 nos. 68–71): provider **support/help-center articles and pricing pages** are ground zero and sometimes the *only* record (Anthropic credits announce+pause: support article + subscriber email, no blog post; Cerebras sell-out: pricing page flip only). **GitHub repos front-run or are the announcement** for CLI-ecosystem changes (Qwen's free-tier kill notice was a GitHub issue; Gemini CLI deprecation mechanics lived in repo issues; opencode commits surfaced Anthropic's OAuth block weeks before press). Official blogs lead only for scheduled corporate changes (Google I/O, GitHub billing). Tech press is reliably last (The New Stack/GIGAZINE fastest); HN is same-day for anything Anthropic/OpenAI-shaped.

**Source list (tiered; mechanism per item):**

- **Tier 1 — authoritative, poll/diff (weekly):** pricing/plan pages diffed verbatim — claude.com/pricing, learn.chatgpt.com/docs/pricing, docs.z.ai/devpack/overview, platform.minimax.io token-plan pages, kimi.com/coding + membership pages, cerebras.ai/code (sell-out/restock lives here), synthetic.new/pricing, nano-gpt.com pricing/blog, platform.stepfun.ai step-plan docs, byteplus.com codingplan activity page, x.ai/pricing-class pages (via index snippets where 403), alibabacloud.com coding-plan help. Provider release-notes/changelogs scraped: support.claude.com release-notes article 12138966 + Help Center (no first-party RSS — use community mirrors), help.openai.com articles 6825453/9624314 + developers.openai.com/codex/changelog, docs.z.ai/release-notes/new-released (GLM plan-quota mechanics appear here), platform.minimax.io/docs/release-notes/models, inference-docs.cerebras.ai/support/change-log, api-docs.deepseek.com/news.
- **Tier 2 — canaries, event-driven (RSS/notifications):** github.com/anomalyco/opencode issues+releases (strongest cross-provider canary), anthropics/claude-code issues+CHANGELOG, openai/codex releases, QwenLM/qwen-code issues, google-gemini/gemini-cli issues, xai-org/grok-build, anomalyco/models.dev commits (new provider/model rows appear as TOML commits).
- **Tier 3 — structured aggregators (weekly API poll):** models.dev public API (API pricing/capabilities — does NOT track subscription plans; treat as model/pricing feed), Artificial Analysis Data API (free tier 1,000 req/day — evals + per-provider pricing snapshot), OpenRouter blog RSS (`/blog/feed.xml`) + its model list (stealth aliases like Owl-Alpha→LongCat make it an early-warning channel), usagepricing.com activity log (maintained; caught the Cerebras cycle).
- **Tier 4 — community (low-noise keyword feeds):** hnrss.org keyword feeds per provider name (points-threshold filtered), r/LocalLLaMA via `/.rss`. Everything else in the "AI pricing tracker" genre tested as blogspam or derivative — not worth subscribing.

**Cadence (the OQ#8 answer):**

- **Continuous (event-driven):** Tier 2 + Tier 4 feeds into the platform's inbox; classification (relevant/not, which lane) runs on local models (Operating reality: costs no allowance).
- **Weekly:** automated Tier 1 page-diff + Tier 3 API poll; diffs classified locally; anything touching an *active* lane's price, terms, models, or endpoints → drift alert to the operator + the §5 re-audit trigger. Weekly beats the observed ~30-day-notice regime (P-T01-3) with margin, and page-diffing catches the zero-notice events (Cerebras class).
- **Monthly:** re-stamp the §2.1 table — every row's canonical URL(s) re-checked, `verified-on` date refreshed; rows older than 60 days render as stale in the 2.7 gap-advice UI (gap advice must never quote a stale plan as current).
- **Quarterly (or on any Tier-1 hit):** full Gate-A re-audit of all *active* lanes; watch-only candidates (Cerebras restock, Tencent intl, BytePlus, Mistral, LongCat, K3 weights) checked for status change.
- **How S2.8 consumes it:** watch items are config rows (`{url|feed, type, parser-hint, lane|candidate}`); hits create drift alerts routed through the approval inbox; **billing-regime changes never auto-flip flags** — the operator confirms, then the 3.1/13.4 flat/metered flag flip runs as the rehearsed 3.10 kill-switch operation with receipts changing currency (report 01 P-T01-3).

---

## 7. What NOT to use and why

- **Chutes:** overage auto-converts to PAYG (since 2026-02-27) — a built-in silent metered path violating 3.10 by design; plus crypto-first payments, thin ToS, single-validator decentralized infra, stuck-session anecdotes. *Trigger:* a true hard-cap mode + fiat clarity.
- **Class-3 automation-banned plans — Alibaba Model Studio (still), Tencent TokenHub, Xiaomi MiMo:** contractual bans on non-interactive use are structurally incompatible with a supervised orchestrator, regardless of price or model quality. *Triggers:* clause removal; Tencent's international launch shipping different terms.
- **Grok Build as a third substrate:** unnecessary while opencode's xAI OAuth is allowlisted; a third conformance suite is real solo-maintainer cost for zero capability gain. *Trigger:* xAI de-allowlisting opencode.
- **xAI API / DeepSeek API / LongCat platform as default routes:** all metered — usable only as explicit 3.10 exceptions (DeepSeek is the *designated* one; the others need a reason it doesn't cover).
- **Featherless Premium $25 for agent work:** 32 K context is below agent-workload floor; higher tiers are dominated by Synthetic on model currency at comparable spend.
- **Poe as a load-bearing lane:** flat price, metered interior (points) — the flat-rate guarantee Sinet routes on doesn't actually exist inside it. Acceptable someday as a bounded utility lane; nothing more.
- **Hugging Face PRO as a lane:** $2/mo of credits is a rounding error; after credits it silently continues as PAYG — the exact pattern 3.10 exists to force into the open.
- **Mistral Le Chat as a programmatic lane:** no API allowance; coding runs only inside their own Vibe surfaces with headless/third-party sanction undocumented. Watch (EU provider) — don't build.
- **Perplexity subscriptions for API use:** the credit perk is conflictingly reported and absent from the current API pricing docs; Sonar is closed-weights. Nothing to build on.
- **Free tiers as lanes:** Qwen's free OAuth is dead (2026-04-15, confirmed from official docs; the "still alive" claim circulating is refuted); OpenRouter's free models cap at 50–1,000 req/day and can vanish. Free tiers may serve as canary probes, never as load-bearing capacity.
- **Account pooling in any form:** every audited ToS bans sharing (xAI, DeepSeek, Z.AI, StepFun, Synthetic, Tencent, Alibaba…) — and D2 already forbids it.
- **Building routing logic on launch-day benchmark numbers:** AA re-baselined mid-cycle (v4.0→v4.1 compressed scores ~8 points); SWE-bench aggregate vs standardized boards disagree by 10–30 points; Aider polyglot is 8 months stale. The 2.7 table stores *dated* eval snapshots; routing weights update on the §6 cadence, not on press releases.

---

## 8. Harvest-map verdicts

| Item | Verdict | Detail |
|---|---|---|
| **O2 rider — opencode xAI/Grok OAuth path** (map: "Provider layer… OpenAI/xAI/Copilot OAuth") | **CONFIRM — now true, sanctioned, with riders** | The path exists today exactly as the map hoped: opencode docs list "xAI Grok OAuth (SuperGrok Subscription)" browser PKCE (callback `127.0.0.1:56121`) + a headless device-code variant + API key; implemented in PR #28557 (merged 2026-05-21, days after xAI opened the surface), and — decisively — **sanctioned by xAI's own "Use Grok in OpenCode" announcement (x.ai/news/grok-opencode)**, not reverse-engineered. It rides on: SuperGrok $30 / X Premium+ $40 / Heavy $300 subscription quota via xAI's official OAuth (accounts.x.ai), enforced as a **per-partner server-side allowlist**. Riders: (1) allowlist revocable without notice (May 2026 tier-gating 403s prove enforcement) → auth canary mandatory (P-T17-1); (2) quota shape unpublished — D4 handles; (3) EU model gap (Grok 4.5 blocked at access date) → region canary (P-T17-3); (4) report 01's overall O2 verdict (**REVISE**) stands — Anthropic OAuth remains dead there, Copilot OAuth remains credit-metered; this rider upgrades only the xAI clause from unverified to confirmed-sanctioned. |

---

## 9. Open questions

**Operator decisions needed:**

1. **xAI lane: adopt, and when?** Requires a member actually holding SuperGrok ($30) or X Premium+ ($40). Sub-decisions: wait for Grok 4.5 EU availability (promised "later this month") or run the lane on grok-build-0.1/4.3-class now; accept the retention-incident history with retention-off defaults verified in the §5 C5 audit. *Owner: operator (purchase + policy).*
2. **Hosted open-weights lane: buy Synthetic now, wait for Cerebras restock, or neither?** Synthetic is the cleanest buyable lane today but is a tiny operator; Cerebras dominates on quota-per-dollar when purchasable. Also: is this lane per-member or operator-only (D2 says per-person; a single-member lane is fine — it's that member's subscription)? *Owner: operator (purchase); T08 consumes the window shapes.*
3. **StepFun cheap-lane experiment:** at $6.99–9.99/mo, a one-month Gate-B/C validation run (usage fidelity, 429 semantics, real prompt-accounting of the 1-prompt≈15–20-calls rule) is nearly free. Worth doing before the spec freezes the low-cost work-class routing? *Owner: operator (trivial purchase); G1-adjacent spike.*
4. **BytePlus spike:** the only way to close its audit is a real signup attempt from Germany + reading the plan ToS from inside the console (docs are JS-walled to research tooling). Cheap, bounded, resolves a genuine multi-model $10 lane. *Owner: operator spike.*
5. **Class-2 gray-zone posture (generalizes report 01 OQ#2):** pre-register the household's stance on tool-scoped plans with no unattended-use ban (Z.AI, Kimi, MiniMax, StepFun): run headless inside whitelisted tools as the vendors' own integration docs demonstrate, and accept the residual ambiguity — or restrict class-2 lanes to attended sessions? One policy, applied uniformly, written into the spec. *Owner: operator.*
6. **Metered-exception designation:** ratify DeepSeek as the single pre-registered 3.10 metered fallback (with the no-personal-data rider), or keep the exception list empty until a concrete need appears? *Owner: operator.*
7. **Watchlist implementation owner:** §6 is a source list + cadence; the S2.8 watcher that executes it (page-diff runner, feed ingestion, local-model classification) needs a home in the T-series (change-detection topic) and a spec section. *Owner: coordinator (assign); this report feeds it.*

**New platform problems discovered (for the spec's Known-problems list):**

- **P-T17-1 — Sanction is allowlist-shaped and revocable server-side without notice.** xAI enforces per-partner OAuth allowlists (May 2026: 403s despite docs); Anthropic's 2026 arc was the same mechanism. A lane can die by policy while credentials and configs remain valid-looking. → Every lane gets an **auth canary** distinct from limit handling: an auth-shaped failure classifies as *policy-revocation-suspected* → operator alert + lane freeze, never an infinite 3.2 retry-park. *(Feeds T08 limit-event taxonomy + S2.8.)*
- **P-T17-2 — Auto-overflow-to-metered plan designs silently violate 3.10.** Chutes converts overage to PAYG; MiniMax auto-spends prepaid credits; HF continues as PAYG post-credits. → Onboarding C1 classifies `overflow_mode`; the adapter enforces it (zero-balance proof or rejection); receipts visibly change currency on any overflow. *(Feeds T08 metering + receipts.)*
- **P-T17-3 — A lane's model list is region- and account-dependent, not what provider docs say.** Grok 4.5 exists globally but not for EU accounts (all xAI surfaces); Kimi K3 is tier-gated inside the same plan family; BytePlus/Tencent model mixes shift by contract. → The adapter periodically diffs each account's *observed* model list against config; routing and 2.7 gap advice consume the observed list, with per-row `verified-on` dates. *(Feeds the adapter spec + 2.7 data model.)*

---

## 10. Sources

All accessed 2026-07-16. Tier P = primary (provider/project's own page, repo, or official announcement); S = secondary. (v) = claim additionally attacked and upheld/corrected in this session's adversarial verification pass. x.ai pages marked *(403)* were unfetchable directly (Cloudflare) — content established via docs.x.ai, search-indexed snippets, vendor-side docs, and multi-secondary corroboration; JS-walled pages marked *(js)*.

**xAI / Grok**
1. P — https://docs.x.ai/grok/faq — subscription vs API separation; no API credits in subs; quota language tier-descriptive, no table. (v)
2. P — https://docs.x.ai/build/overview — Grok Build auth: browser OAuth; API key for non-browser environments.
3. P — https://docs.x.ai/developers/models and https://docs.x.ai/developers/pricing — lineup (grok-4.5 2026-07-08, 500k ctx, $2/$6 <200k, $4/$12 ≥200k, $0.50 cached; grok-build-0.1 $1–2/$2–4; grok-4.3/4.20) . (v)
4. P — https://docs.x.ai/developers/migration/may-15-retirement — grok-code-fast-1 deprecated 2026-05-15, auto-rerouted to grok-build-0.1; full retirement 2026-08-15. (v)
5. P *(403; snippets + coverage)* — https://x.ai/news/grok-build-cli, https://x.ai/news/grok-opencode (2026-05-21), https://x.ai/news/grok-warp — Build launch/expansion; third-party OAuth sanction announcements; + xAI status post x.com/xai/status/2066625790704521699. (v)
6. † P — https://opencode.ai/docs/providers/ — verbatim today: xAI Grok OAuth (SuperGrok Subscription) browser PKCE + Headless/Remote/VPS device-code + API key. (v)
7. P — https://github.com/anomalyco/opencode/pull/28557 (merged 2026-05-21), issue #28411, issue #31475 — OAuth implementation (PKCE loopback 127.0.0.1:56121, device-code, refresh); SuperGrok device-code in real use 06-09. (v)
8. P — https://docs.warp.dev/agent-platform/inference/grok-subscription/ — Warp subscription OAuth; grok-build-0.1 + grok-4-3 tiers; tokens local-only; Cloud Agents excluded. (v)
9. P — https://github.com/xai-org/grok-build + S https://simonwillison.net/2026/Jul/15/grok-build/ — Apache-2.0 open-sourcing 2026-07-15; no upstream PRs; coding-data retention incident + fix.
10. S — https://shareallai.github.io/familypro/en/blog/grok-build-guide/ and /grok-plan-guide/ (upd. 2026-07-03) — post-expansion tier availability (SuperGrok + X Premium Plus). (v)
11. S — https://aibusinessweekly.net/p/grok-ai-pricing (upd. 2026-06-27), felloai/jingrey/suprmind roundups — $10/$30/$40/$300 tier prices (x.ai/pricing 403; multi-secondary). (v)
12. S — https://github.com/NousResearch/hermes-agent/issues/26847 (2026-05-16) and https://github.com/openclaw/openclaw/issues/84504 — pre-expansion Heavy-only 403s; hermes docs note per-partner backend allowlist. (v)
13. S — https://www.trendingtopics.eu/spacexai-and-cursor-launch-grok-4-5-not-yet-in-the-eu/, https://www.heise.de/en/news/SpaceXAI-introduces-AI-model-Grok-4-5-EU-users-must-wait-11359422.html, https://moclaw.ai/blog/grok-4-5-not-available-eu (upd. 2026-07-16) + P https://docs.x.ai/developers/models/grok-4.5 ("later this month") — EU block current today, Germany included. (v)
14. S — https://conductatlas.com/platform/xai/xai-terms-of-service/ + P-mirror loyalagents.github.io (consumer ToS text prev. 2025-06-09, stale-flagged) + P-snippet x.ai/legal/acceptable-use-policy *(403)* — no-sharing clause; AUP bot/automation prohibition outside sanctioned surfaces.
15. S — dataconomy.com/2026/07/07/elon-musk-rebrands-merged-xai-and-spacex-as-spacexai/ (+ Yahoo Finance, TechBriefly) — SpaceX merger 2026-02-02; SpaceXAI rebrand 2026-07-06.
16. S (independent evals) — https://artificialanalysis.ai/models/grok-4-5 (AA 54 at launch; #4 flagship), vals.ai/benchmarks/swebench (86.6% SWE-V, 3rd), contracollective.com (SWE-Pro 3rd-of-4, Terminal-Bench 83.3% — single-source flags), arena.ai changelog (Agent Arena adds 2026-07-13; no text-arena Elo yet), techtimes (Cursor-data contamination flag).

**DeepSeek**
17. P — https://www.deepseek.com/ — product surface: free chat/app; only pricing link is API.
18. P — https://api-docs.deepseek.com/quick_start/pricing/ — V4-Flash $0.0028/$0.14/$0.28, V4-Pro $0.003625/$0.435/$0.87; 1M ctx; prepaid deduction model; no off-peak windows.
19. P — https://api-docs.deepseek.com/news/news260424/ — V4 release 2026-04-24 (1.6T/49B; 284B/13B), open-sourced, OpenAI+Anthropic compatible; legacy IDs retired 2026-07-24.
20. P — https://api-docs.deepseek.com/guides/anthropic_api/, /quick_start/agent_integrations/claude_code/, /guides/coding_agents/ — official Anthropic endpoint + Claude Code env config; OpenCode (≥1.14.24) and OpenClaw officially documented.
21. P — https://api-docs.deepseek.com/quick_start/rate_limit — concurrency 500 (v4-pro) / 2,500 (v4-flash); HTTP 429; expansion request form.
22. P — https://cdn.deepseek.com/policies/en-US/deepseek-open-platform-terms-of-service.html (eff. 2026-04-29: per-person keys, no sharing, PRC law) and /deepseek-terms-of-use.html (upd. 2026-03-27: no account transfer; no bots on consumer service).
23. S — Engadget engadget.com/2180062 (+ TNW, InfoWorld) — V4-Pro 75% price cut made permanent 2026-05-22; S datastudios/getaiperks/felloai — no subscription tier exists in any region.
24. S — artificialanalysis.ai April V4 article (v4.0-era figures — superseded by v4.1, cited only as release context); benchlm.ai/llm-stats SWE-V ~80.6% (vendor-aggregate caveat).
25. S — BleepingComputer/TechRadar/the-decoder (Berlin DPA DSA Art. 16 referral 2025-06-27; app not removed) + compound.law/en-DE/tools/deepseek/ (no GDPR DPA offered) — app-vs-API distinction for a German household.

**Chinese coding-plan sweep**
26. P *(js; snippets)* — https://www.volcengine.com/activity/codingplan + S codepick.dev/en/guides/ark-coding-plan-guide, codingplan.org — domestic ByteDance plan (¥40/¥200; 1,200 req/5h; Claude Code/Codex/TRAE/OpenCode/Cline; Anthropic-compat endpoint via S corroboration + claude-code-router issue #1102).
27. P — https://www.byteplus.com/en/activity/codingplan *(js)* + docs.byteplus.com/en/docs/modelark/1928265 (promo suspension: standard Lite $10 / Pro $50), /ModelArk/1925115, /ModelArk/1928262 (Claude Code doc), /ModelArk/2276791 (Team tier), /ModelArk/2165245 (FAQs, anti-abuse), /ModelArk/availability (region-varies) + P X @BytePlusGlobal status/2046811781365121082 (GLM-5.1 added) — intl Coding Plan. (v)
28. S — j4nt4ncrypto.medium.com BytePlus+opencode setup (2026-04-17) + https://pi.dev/packages/pi-byteplus-modelark — endpoint `ark.ap-southeast.bytepluses.com/api/coding/v3`; **warning: `/api/v3` bills outside plan quota**. (v)
29. P — https://cloud.tencent.com/document/product/1823/130092 + /130103 (+ /130060) — TokenHub Coding Plan: ¥40/¥200, windows, `sk-sp` keys, dual OpenAI (`api.lkeap.cloud.tencent.com/coding/v3`) + Anthropic (`/coding/anthropic`) endpoints; verbatim automation ban; no sharing; whitelist.
30. P — https://www.tencentcloud.com/products/tokenhub — international "Large Model Token Plan (Coming Soon)".
31. P — https://codebuddy.ai/docs/ide/Account/pricing + /docs/cli/http-api — CodeBuddy intl pricing; HTTP API Beta (REST + ACP) for webhooks/daemons.
32. P — https://platform.stepfun.ai/docs/en/step-plan/overview + /docs/en/step-plan/paid-service-agreement.md + /docs/en/agreement/userservice.md + P X @StepFun_ai status/2035815197584023835 — tiers $6.99–$99; prompt windows; endpoint; tool list; Stripe; **supported-tools-only + anti-circumvention + sharing/resale clauses (found in verification); no interactive-only ban**. (v)
33. S — efficienist.com StepFun launch coverage (2026-03-22) + P(board) https://artificialanalysis.ai/models/step-3-7-flash — AA v4.1 index 30, 378 tok/s #1 speed, verbose flag. (v)
34. P — https://mimo.mi.com/docs/en-US/price/token-plan + P X @XiaomiMiMo status/2059314052892099070 — Token Plan global tiers $6–$100; credit quotas; off-peak ×0.8; verbatim programming-tools-only / no-automation clause.
35. P — https://www.alibabacloud.com/help/en/model-studio/coding-plan — Lite discontinued 2026-03-20; Pro $50 multi-model (qwen3.7-plus, kimi-k2.5, glm-5, minimax-m2.5); automation ban + no-sharing verbatim intact. P — https://qwenlm.github.io/qwen-code-docs/en/users/configuration/auth/ + github.com/QwenLM/qwen-code issues #3203/#3316 — free OAuth tier dead 2026-04-15 (1,000→100→0). (v)
36. P — https://www.kimi.com/coding ("K3 is now available", 1M ctx) + kimi.com/code/docs/en/kimi-code/models (`k3` Moderato+; 1M on Allegretto+) + platform.kimi.ai/docs/pricing/chat-k3 ($3.00/$0.30-hit/$15.00) + kimi.com/resources/kimi-k2-7-code-pricing + S TechCrunch 2026-07-16 (pre-launch "upcoming… close the gap with Opus 4.8") + openrouter.ai/moonshotai/kimi-k3 — K3 launch-day resolution; weights promised, absent from huggingface.co/moonshotai. (v)
37. P — https://platform.minimax.io/docs/guides/pricing-token-plan + /subscribe/coding-plan — tiers unchanged; "all models on the API Platform" incl. M3; credit-overflow mechanics (1,000 credits = $1); promo ends 2026-07-31.
38. P — https://docs.z.ai/devpack/overview ("All plans support GLM-5.2, GLM-5-Turbo and GLM-4.7"; multipliers 3×/2×, promo 1× off-peak through Sep 2026) + z.ai/subscribe *(js)* + S aipricing.guru (data upd. 2026-07-16: $18/$72/$160, −30% promo) — Z.AI changes; dollar digits secondary-only, flagged. (v)
39. P — https://longcat.chat/platform/docs (Pricing, Token Pack, APIPayAsYouGo) — PAYG + prepaid packs only; OpenAI + Anthropic endpoints. S — decrypt.co, VentureBeat LongCat-2.0 (2026-06-30; "Owl Alpha" stealth alias; Chinese-chip training).
40. P — https://huggingface.co/inclusionAI — Ling-2.6-1T MIT weights. S — linux.do/t/topic/1786495 + github.com/iflow-ai/iflow-cli README — iFlow shutdown 2026-04-17 confirmed permanent; users migrated to Qoder.
41. P — https://www.trae.ai/pricing *(js)* — Trae tiers expose no API/CLI surface. P — alibabacloud.com/help/en/lingma/product-overview/billing-description — Qoder credits own-suite only. S — comate.baidu.com snippets — Baidu Comate IDE-scoped.

**Hosted open-weights flat plans**
42. P — https://www.cerebras.ai/code (fetched 2×: sold out all tiers; GLM-4.7 single model; Pro 24M tok/day, Max 120M) + support.cerebras.net/articles/9996007307-cerebras-code-faq (limit semantics: clear error on daily quota, 429 on bursts, no refunds) + P X @CerebrasSystems status/1965133260343968191 (quota history). (v)
43. P — https://synthetic.new/pricing ($30/mo or $1/day per pack; 500 req/5h; 1 concurrent/model/pack; UI+API) + https://synthetic.new/policies/terms-of-service (eff. 2026-01-11: no credential sharing; anti-scrape only — no automation ban; no-train; 14-day deletion) + https://dev.synthetic.new (Anthropic base `api.synthetic.new/anthropic/v1`; Claude Code guide; 7 agents) — all re-verified in adversarial pass. S — patshead.com review (2026-02-20: fastest of budget trio, variability note); glhf.chat rebrand posts. (v)
44. P — https://chutes.ai/pricing + https://chutes.ai/news/community-announcement-february (2026-02-27: 5× PAYG-value caps over rolling windows; **overage auto-converts to PAYG**; Base-tier model removals; TAO payments) — the disqualifying primary source. S — rpwithai.com, tao.media (single-validator), patshead (stuck sessions).
45. P — https://nano-gpt.com/blog/subscription-update-february-2026 (60M input tok/wk; changes on 2 days' notice; GLM-5/K2.5 cost rationale; anti-backend clause) + docs.nano-gpt.com/api-reference/endpoint/subscription-usage (5,000 req/day, 60k/mo enforced) — $8/mo lane shape.
46. P — https://featherless.ai *(js, partial)* — Premium $25 (4 conc., 32K ctx) / Agent Std $100 / Agent Pro $200; unlimited-token concurrency-capped model; lineup shift flagged MED.
47. P — https://groq.com/pricing (metered + free tier) and https://openrouter.ai/docs/api-reference/limits (prepaid credits; free models 50 vs 1,000 req/day at $10 lifetime; 20 RPM) — metered-only confirmations. S — helicone/pricepertoken/infrabase sweeps for Together/Fireworks/DeepInfra/Novita (PAYG only); Nebius/Hyperbolic/Baseten/Parasail = sweep-level negatives, flagged.
48. P — https://huggingface.co/docs/inference-providers/en/pricing — PRO $2.00/mo included credits; `router.huggingface.co/v1`; PAYG continues after credits.
49. P — https://mistral.ai/pricing — Le Chat Free/Pro $14.99/Team $24.99; coding via own CLI/IDE "all-day" fair-use; no API allowance in any tier; S cloudzero/techjack corroboration.
50. P — https://docs.perplexity.ai/docs/getting-started/pricing (silent on subscriber credits) + S conflict: opslyft ("$5 credit ended Feb 2026") vs felloai/suprmind ("still included") — perk unverifiable, treat as dead.
51. P — https://poe.com/blog/introducing-the-poe-api (2025-07-31) + creator.poe.com docs (OpenAI-compatible api.poe.com/v1; Anthropic-compatible page; sanctioned for Cursor/Cline/Roo/llm) — points-metered interior.

**Open-weights snapshot / evals**
52. P(board) — https://artificialanalysis.ai/models/open-source (Intelligence Index v4.1 board: GLM-5.2 51; MiniMax-M3 44; DeepSeek V4 Pro 44; Kimi K2.6 44; MiMo-V2.5-Pro 42; V4 Flash 40; Qwen3.5-397B 34) + /articles/artificial-analysis-intelligence-index-v4-1 (2026-06-15 re-baselining: agentic 34%/coding 24%) — the v4.0→v4.1 conflict resolution. (v)
53. P — https://huggingface.co/zai-org/GLM-5.2 (license: mit; 753B MoE; no regional gating) + S MarkTechPost/digitalapplied (launch 2026-06-13 to all plan tiers; weights ~06-16) + S VentureBeat GLM-5.2 coverage. (v)
54. S — https://www.morphllm.com/swe-bench-pro (vendor-aggregate: GLM-5.2 62.1% best-open) vs https://labs.scale.com/leaderboard/swe_bench_pro_public (standardized, 10–30 pts lower, reordered) — benchmark-dispersion caveat.
55. P(board) — https://www.tbench.ai/leaderboard/terminal-bench/2.1 — GLM-5.1 only open model in top cluster (58.7%).
56. P(board) — https://aider.chat/docs/leaderboards/ — last updated 2025-11-20; stale-signal warning.
57. S — VentureBeat Meituan LongCat-2.0 (1.6T, near-frontier, OpenRouter pre-reveal leader) + techtimes MiniMax M3 license terms (custom; commercial agreement required) — single-source flags noted.
58. P — https://mistral.ai/news/devstral-2-vibe-cli/ (Devstral 2 123B mod-MIT; Small 2 24B Apache-2.0; vendor 72.2% SWE-V claim flagged) + https://openai.com/index/introducing-gpt-oss/ (Apache-2.0; 120b/20b; no successor per help.openai.com release notes) + S promptquorum.com/local-llms, llmconfigurator.com (Qwen3.6-27B 24GB consensus).
59. S — https://techcrunch.com/2026/07/16/moonshots-upcoming-kimi-3-is-expected-to-close-the-gap-with-anthropics-opus-4-8/ (FT-sourced pre-launch; superseded same day by #36) + techbuzz.ai (Grok 2.5 weights Aug 2025; Grok 3 open-sourcing unfulfilled, API access removed 2026-05-15).
60. S(board) — https://arena.ai/leaderboard + changelog — Grok/Kimi arena additions; Meta "Muse Spark" (closed) as Meta's 2026 frontier; "Llama 5 open, Apr 2026" found only on low-tier content sites → UNVERIFIED, excluded.

**Watchlist / tracker infrastructure**
61. P — https://models.dev/ + https://github.com/anomalyco/models.dev — open provider/model DB (TOML per provider; public API; entries current to Jul 2026); tracks API pricing/capabilities, NOT subscription plans; commit stream = new-model feed.
62. P — https://artificialanalysis.ai/data-api (+ /api-reference) — free Data API tier, 1,000 req/day: intelligence/coding indices + per-provider pricing.
63. P — https://openrouter.ai/blog (RSS /blog/feed.xml verified) + model list — launches; stealth-alias early warnings (Owl Alpha→LongCat-2.0 precedent).
64. P — https://support.claude.com/en/articles/12138966-release-notes (+ Help Center; no first-party RSS; community mirrors github.com/taobojlen/anthropic-rss-feed, Olshansk/rss-feeds) + https://help.openai.com/en/articles/6825453 + /9624314 + https://developers.openai.com/codex/changelog — first-mover provider channels (Agent-SDK episode: support article + email first, no blog).
65. P — https://docs.z.ai/release-notes/new-released (GLM plan-quota mechanics appear here first) + https://platform.minimax.io/docs/release-notes/models + https://inference-docs.cerebras.ai/support/change-log + https://api-docs.deepseek.com/news — provider changelogs verified live.
66. P — https://hnrss.org/ (+ github.com/hnrss/hnrss — maintained) — keyword+points feeds per provider; r/LocalLLaMA via /.rss.
67. S — https://www.usagepricing.com (+ /blueprint/activity/cerebras-2026-05-30-launch) — maintained third-party pricing/activity tracker; caught the Cerebras relaunch/sell-out cycle that no press carried.
68. S/P — Agent-SDK-credits channel timing: thenewstack.io/anthropic-pauses-claude-agent-sdk-subscription-change/ ("started emailing subscribers"), news.ycombinator.com/item?id=48545980 (pause-day thread), gigazine.net/gsc_news/en/20260514 (24h follower) — support-page/email → HN same-day → press last.
69. P — https://github.blog/news-insights/company-news/github-copilot-is-moving-to-usage-based-billing/ (2026-04-27, blog+forum same day) + GitHub community discussion #192948 — the scheduled-corporate-change pattern.
70. P — https://github.com/google-gemini/gemini-cli/issues/27998 + developers.googleblog.com Antigravity transition post — repo issues carried the actionable mechanics; press clustered at effective date.
71. P — https://github.com/anomalyco/opencode issues #18329, #21922 — opencode repo surfaced Anthropic's OAuth block weeks before press (canary proof); + github.com/openai/codex/releases (120 releases; v0.144.5 on 2026-07-16) as high-cadence canary.
72. S — event-corroboration set: tech.yahoo.com "Free Qwen Is Dead" (weeks-late press for #35), visualstudiomagazine.com Copilot-billing backlash (same-day trade press), 9to5google post-cutoff coverage — press-lag calibration for §6.

*Sourcing notes:* every load-bearing claim carries ≥2 independent sources or an explicit single-source flag inline (notable single-source items: Terminal-Bench/SWE-Pro Grok-4.5 placements, LongCat license terms, MiMo OpenRouter share, BytePlus quota digits, Featherless current tier lineup, Z.AI dollar digits — all flagged where used). ToS/pricing claims rest on provider primaries except where pages were 403/JS-walled — those are marked and carry corroboration chains. The 13 most load-bearing claims went through a dedicated adversarial pass; its corrections are folded in (xAI "weekly pool" refuted → quota shape undocumented; StepFun "no restrictive clause" corrected → tools-whitelist + anti-circumvention clauses exist; BytePlus $5 Lite → suspended promo, standard $10; K3 "unreleased" → launched 2026-07-16; AA v4.0 figures → superseded by v4.1). Launch-day facts (K3, AA board) are pinned to 2026-07-16 and expected to drift within days — §6's cadence exists for exactly this.
