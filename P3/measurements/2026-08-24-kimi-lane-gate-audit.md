# Kimi (Moonshot) lane — Gate A–C onboarding audit

**Packet:** P3-LN-3 · **Audit date / access date for every fetch below:** 2026-08-24
**Checklist executed:** `Research/02-provider-watchlist-and-onboarding-criteria.md` §5 (Gates A1–A5, B1–B5, C1–C5)
**Spec frame:** `Spec/drafts/S03-engines-adapters.md` §S03.6 (lane roadmap & onboarding); manifest home `Spec/drafts/S16-adoption-manifest.md`
**Prior (NOT a source):** R02 §2.1 Moonshot row, verified-on 2026-07-16. Every claim below was re-fetched live; where this audit contradicts or narrows the prior, the audit wins.
**Occasion:** operator order 2026-08-23 to add the Kimi lane early (operator holds a Kimi membership serving K3). Kimi is not in the v0 lane set; this is the S03.6 / G1-rider-3 post-v0 roadmap executed early → carries an S00.9 amendment.

---

## 0. Verdict

| Gate | Verdict | Load-bearing basis |
|---|---|---|
| **A — sanction** | **PASS (class 2, gray-zone note)** — A1 **PARTIAL** | Two full ToS documents fetched and readable; **neither contains any automation / unattended-use / interactive-only clause**. OpenCode is a named, officially-documented sanctioned tool → A3 holds, so class 2 passes per §5 A2. A1 partial: the ToS side is complete, the **international USD plan-price page is a JS shell** (see A1). |
| **B — technical** | **PASS** | Documented plan endpoint `https://api.kimi.com/coding/v1`, Anthropic- *and* OpenAI-compatible; opencode ships a first-party `kimi-for-coding` provider; full documented 401/403/429 error taxonomy; endpoint discipline documented by the vendor. |
| **C — economics/risk** | **PASS, with one mandatory rider** | Overflow = **`opt-in-credits`** with an explicitly **proven disable** (off by default, turn-off-any-time) → satisfies 3.10 / P-T17-2. C5: **mainland-China data residency, training-on-inputs by default** → the DeepSeek-style no-household-personal-data routing rider is **required**, not optional. |

**Lane is BUILDABLE.** Not a REJECT (no class-3 clause exists), not a BytePlus-style PENDING (the sanction-determining pages fetched in full). The one A1 gap is a *price* gap, not a *sanction* gap, and it closes at bring-up from the operator's own console rather than from more fetching (P-T17-3: the account's observed state is the runtime authority).

---

## Gate A — Sanction

### A1. Capture — PARTIAL

**Fetched and readable in full (the sanction-determining set):**

| URL | What it is | Status |
|---|---|---|
| `https://platform.kimi.ai/docs/agreement/modeluse` | **Terms of Service for Kimi OpenPlatform** | Rendered, full text |
| `https://www.kimi.com/user/agreement/modelUse` | Kimi assistant Terms of Service (consumer) | Rendered, full text (simplified Chinese) |
| `https://www.kimi.com/user/agreement/userPrivacy?version=v2` | Kimi Privacy Policy | Rendered, full text (simplified Chinese; the page states the English version has equal legal standing) |
| `https://www.kimi.com/code/docs/en/` | Kimi Code Overview (product docs) | Rendered |
| `https://www.kimi.com/code/docs/en/kimi-code/error-reference.html` | Kimi Code Error Reference | Rendered |
| `https://www.kimi.com/code/docs/en/kimi-code/membership.html` | Kimi Code Membership Benefits | Rendered |
| `https://www.kimi.com/code/docs/en/kimi-code/whats-new.html` | Kimi Code changelog | Rendered |
| `https://www.kimi.com/code/docs/en/kimi-code/faq.html` | Kimi Code FAQ | Rendered |
| `https://www.kimi.com/code/docs/en/third-party-tools/opencode.html` | **Official OpenCode integration page** | Rendered |
| `https://www.kimi.com/en/help/membership/membership-extra-usage` | Extra Usage Pack help article | Rendered |
| `https://www.kimi.com/en/help/kimi-code/benefits` | Kimi Code Benefits and Billing | Rendered |
| `https://platform.kimi.ai/docs/pricing/chat-k3` | K3 metered price table | Rendered |
| `https://platform.kimi.ai/docs/pricing/limits` | Recharge & rate limiting (metered platform) | Rendered |
| `https://platform.kimi.ai/docs/api/overview` | Metered platform API overview | Rendered |

**JS-walled / did not yield the requested content (the recorded gap):**

| URL | What was wanted | What came back |
|---|---|---|
| `https://www.kimi.com/coding` | membership tiers + prices | JavaScript shell. Only fragment rendered: **"Moderato 及更高档位套餐可使用 K3 模型"** ("Moderato and higher-tier packages can use the K3 model"). No price table. |
| `https://www.kimi.com/membership/pricing` | international USD tier→price table | JavaScript shell. Only the page title rendered: **"Kimi AI 官网 - K3 上线，专为智能体编程与知识工作打造"**. No tiers, no prices, no quotas. |
| `https://www.kimi.com/en/help/membership/membership-pricing` | international USD tier→price table | Rendered, but serves the **¥ (RMB)** plan set (Andante ¥49 / Moderato ¥99 / Allegretto ¥199 / Allegro ¥699 per month), **not** the USD set. |

**Honest consequence (§5 A1, BytePlus precedent applied narrowly):** the *plan-price page* is JS-walled and the *international USD tier→price mapping is UNVERIFIED from any primary page*. The July prior's `$19–$199` with names Moderato/Allegretto/Allegro/Vivace remains **MED-confidence and unconfirmed** — secondary aggregators repeat it, and this audit does not promote secondary repetition to fact. The ToS/usage-policy set, which is what determines *sanction*, fetched completely — so the lane does **not** inherit BytePlus's pending status, whose defect was precisely that the automation stance could not be established. **The price gap closes at bring-up from the operator's own account console, not from further fetching.**

### A2. Automation-stance CLASS — **CLASS 2** (re-verified, unchanged from the July prior)

**Terms of Service for Kimi OpenPlatform** (`https://platform.kimi.ai/docs/agreement/modeluse`, accessed 2026-08-24):
- Clauses about automated / programmatic / scripted use, bots, batch calls, non-interactive scenarios: **ABSENT**.
- Clauses restricting use to interactive or human-only operation: **ABSENT**.
- §1, verbatim: *"You must provide accurate and up-to-date account information. You are fully responsible for all activities under your account, including the activities of any End User or anyone else. You must not share your account credentials or make your account accessible to others, including not transferring, lending, leasing, or providing it to anyone for use in any form without authorization."*
- §3.2(6), verbatim: *"Copying, transferring, renting, lending, selling, or providing sub-licensing or re-licensing of the Services in whole or in part without authorization."*

**Kimi assistant Terms of Service** (`https://www.kimi.com/user/agreement/modelUse`, accessed 2026-08-24):
- Automated / programmatic / scripted / bot / batch-call clauses: **ABSENT**.
- Interactive-or-human-only restriction: **ABSENT**.
- Account transfer, verbatim: *"您的账户不得以任何方式转让，否则 Kimi 智能助手平台有权追究您的违约责任"* (accounts may not be transferred in any manner).

**Positive sanction evidence (why class 2 rather than merely "silent"):** `https://www.kimi.com/code/docs/en/` publishes an explicit sanctioned third-party tool set — **Claude Code, OpenCode, Codex, Hermes Agent** — each with its own official integration page, and states verbatim: *"When integrating Kimi Code into third-party development tools, you need to manually configure an API Key"*.

**Corroboration that unattended operation is not merely unbanned but vendor-endorsed** (`https://www.kimi.com/code/docs/en/kimi-code/whats-new.html`): the vendor's own CLI shipped *"`/auto` permission mode: auto tool approvals, no user questions (unattended-friendly)"* (v0.5.0, 2026-05-28) and *"Auto mode suggested for unattended work"* (v0.11.0, 2026-06-05).

**Classification:** **CLASS 2** — tool-scoped (a named-tool sanction list rather than an open programmatic grant) **without an unattended-use ban**. Per §5 A2 this **passes with a gray-zone note, conditional on A3**, which holds. It does **not** promote to class 1 because the sanctioned surface is expressed as an enumerated tool list.

**No class-3 clause exists** in either agreement as of 2026-08-24 → **no REJECT**.
**Would-change trigger (for the §6 watchlist):** appearance in either ToS of an interactive-only / no-automation / no-non-interactive-batch clause of the kind Alibaba, Tencent and Xiaomi carry verbatim → immediate re-audit and lane freeze before the next run consumes allowance.

### A3. Whitelist fit — **PASS**

**OpenCode is in the sanctioned tool set and has a first-party integration page** (`https://www.kimi.com/code/docs/en/third-party-tools/opencode.html`, accessed 2026-08-24). Verbatim setup:
1. Run `opencode auth login` and select **Kimi For Coding** from the provider list
2. Enter the Kimi For Coding API Key (from the Kimi Code Console, `https://www.kimi.com/code/console`)
3. Launch with `opencode`
4. `/models` to select

npm package named on the page: **`opencode-ai`** — the substrate Sinet already runs and already pins. **No new engine is forced** (§5 A3 satisfied). Claude Code is *also* sanctioned, so the lane would have a second Sinet-native option if ever needed.

### A4. Per-person viability (D2) — **PASS with a recorded gap**

- **Own account per member:** yes. Sharing/transfer is banned in both agreements (quoted at A2) — which §5 A4 expects and treats as fine; no pooling is needed or attempted.
- **No mainland-account / real-name barrier surfaced** for the international `kimi.com` property. (Contrast the Tencent row in R02 §2.1, which is explicitly mainland-only.)
- **Payment rails — UNVERIFIED at primary-source grade.** No primary page reachable in this session states the international payment methods or the USD tier prices. The primary help article served **¥ (RMB)** prices. Secondary aggregators state USD billing with Visa/Mastercard; **this audit does not certify that**. Attempted URLs: `https://www.kimi.com/membership/pricing` (JS shell), `https://www.kimi.com/coding` (JS shell), `https://www.kimi.com/en/help/membership/membership-pricing` (¥ prices only).
- **Concrete EU-friction fact, primary-sourced:** Extra Usage top-ups are denominated in **RMB** — minimum **¥25** per top-up, max **¥3,000/day**, balance cap **¥10,000** (`https://www.kimi.com/en/help/membership/membership-extra-usage`). Since the recommendation below leaves Extra Usage **off**, this does not block the lane, but it is the field to re-check if the operator ever considers enabling overflow.
- **Moot for go-live:** the operator already holds a paying Kimi membership that serves K3, so per-person viability from Germany is demonstrated by existence for at least one member. Viability for *additional* household members stays UNVERIFIED until a second person subscribes.

### A5. Sanction mechanism — **open API key, tool-scoped**

An **API key created by the member in the Kimi Code Console** (`https://www.kimi.com/code/console`), manually configured into a third-party tool. **Not** a partner-allowlisted OAuth flow (contrast xAI) and **not** first-party-tool-only (contrast Google/Cursor).

**Consequence for lane design:** the key is member-issued, so this is not the server-side-revocable OAuth shape that P-T17-1 was written for. The auth canary is still required and still distinct from limit handling, but the realistic revocation shapes here are membership expiry, tier downgrade, and account suspension — all of which surface as **documented 401/403 codes** (see B4), not as silent OAuth revocation. The documented `403 "Access terminated"` (*"Account suspended for policy violation"*) is the policy-revocation signal that must alert the operator and freeze the lane, **never** enter a 3.2 retry-park.

---

## Gate B — Technical fit (D3)

### B1. Protocol and exact base URL — **PASS**

**The subscription/member endpoint** (`https://www.kimi.com/code/docs/en/`, accessed 2026-08-24), verbatim:
- OpenAI-compatible — Base URL: **`https://api.kimi.com/coding/v1`**; example endpoint `https://api.kimi.com/coding/v1/chat/completions`
- Anthropic-compatible — Base URL: **`https://api.kimi.com/coding/`**; messages path `https://api.kimi.com/coding/v1/messages`

The error-reference page restates both verbatim: *"Correct Base URLs: `https://api.kimi.com/coding/v1` (OpenAI protocol) or `https://api.kimi.com/coding/` (Anthropic protocol)."*

**Which protocol the SUBSCRIPTION key reaches through opencode:** `https://models.dev/providers/kimi-for-coding` (accessed 2026-08-24) records the first-party opencode provider as:

| Field | Value |
|---|---|
| provider page / id | `kimi-for-coding` |
| display name | Kimi For Coding |
| npm | **`@ai-sdk/anthropic`** |
| base URL | **`https://api.kimi.com/coding/v1`** |
| models | `k3`, `k3-256k`, `kimi-for-coding`, `kimi-for-coding-highspeed` |
| listed price | `$0.00 / $0.00` on every model |

**This is the structural difference from the Z.AI lane and the single most important B-gate fact for implementation:** the Kimi lane rides opencode on the **Anthropic** protocol via `@ai-sdk/anthropic`, where `zai` rides it on **`@ai-sdk/openai-compatible`**. The existing lane document already carries `npm` as a data field, so this is a data difference, not necessarily a code difference — but it must be asserted by test, not assumed.

**Recorded discrepancy (minor, deliberately flagged):** the vendor docs give the Anthropic base as `https://api.kimi.com/coding/` while models.dev gives `https://api.kimi.com/coding/v1` for the `@ai-sdk/anthropic` provider. Both agree on the substring **`/coding/v1`**, which is what the endpoint marker should key on. The provider **id** `kimi-for-coding` is taken from the resolving models.dev provider URL; one summarizer pass also offered `moonshotai` as an inference — treat the id as **seed data to be confirmed against the installed opencode's own provider list at bring-up**, exactly as P-T17-3 prescribes.

**`$0.00 / $0.00` — the S03.7 corollary carries over verbatim:** opencode prices this provider at zero, so the platform must **never meter dollars from opencode** for this lane; dollars come from Sinet's own metered price rows (C2 below).

### B2. Headless auth — **PASS**

Member-created API key, entered once via `opencode auth login`; no browser is required at run time and no OAuth device flow is involved. The key is a per-person credential the member creates and can regenerate in their own console → storable in Sinet's per-user credential store outside task sandboxes and revocable per person (D2 satisfied). Consistent with the existing broker auth-profile pattern: Sinet holds a profile *name*, resolves material fresh at spawn, never stores or logs it.

### B3. Usage fidelity (D4) — **PASS on wire-side tokens; DEGRADED on plan-unit consumption**

- The endpoint is OpenAI-/Anthropic-shaped, so per-response `usage` token fields are present in the standard position for both protocols.
- **Better than the Z.AI lane on one axis:** a **`/usage` surface exists**. The Kimi Code FAQ (accessed 2026-08-24) states the `/usage` command *"check your current quota and membership status"*. **The response field names are UNVERIFIED** — no primary page documents them. Attempted: `https://www.kimi.com/code/docs/en/kimi-code/faq.html` (mentions it, does not specify fields), `https://www.kimi.com/code/docs/en/kimi-code/quota.html` (**404**), targeted search for `/coding/v1/usage` (no primary doc found).
- **Plan-unit consumption is not derivable from the wire.** `https://www.kimi.com/en/help/kimi-code/benefits` states verbatim that **"All signed-in devices and API Key share the same quota"** — so the operator's interactive Kimi use and Sinet's runs draw on **one shared pool**. Sinet can never compute remaining allowance from its own request history alone.
- **Verdict:** `usage_fidelity: degraded` for plan units (same tier-3 / requests-as-proxy treatment S10.1/S10.4 already gives Z.AI), **not** degraded for per-response tokens. The `/usage` surface is a genuine improvement to reach for at bring-up, but its shape must be *observed*, not assumed, before anything is built on it.

### B4. Limit observability — **PASS (the strongest documented taxonomy of any lane so far)**

From `https://www.kimi.com/code/docs/en/kimi-code/error-reference.html` (accessed 2026-08-24), verbatim messages:

**401 — authentication / entitlement:**
- *"The API Key appears to be invalid or may have expired"*
- *"Invalid Authentication"* (unsupported authentication format)
- *"Your current subscription does not have access to k3"*
- *"Your current plan supports only kimi-k3 up to 256K context"*
- *"Your current subscription does not have access to kimi-for-coding-highspeed"*
- *"Your model id does not exist, recognized as other:<model-id>"*

**429 — rate / quota:**
- *"The engine is currently overloaded, please try again later"* (transient capacity)
- *"We're receiving too many requests at the moment"* (account concurrency limit)
- *"You've reached your usage limit for this period"* (5-hour rolling window exhausted)
- *"You've reached kimi monthly usage limit for this billing cycle"* (monthly quota consumed; frozen until reset)

**403 — billing / access:**
- *"You've reached your usage limit for this billing cycle"* (weekly quota exhausted, awaiting refresh)
- *"Access terminated"* (**account suspended for policy violation**)

**400 / 404 — request shape:**
- *"total message size N exceeds limit 2097152"*, *"Your request exceeded model token limit: 262144"*, *"thinking is enabled but reasoning_content is missing"*, *"The request was rejected because it was considered high risk"*
- *"Not found the model kimi-for-coding or Permission denied"*, *"method not found"* (**endpoint path error** — directly useful to the endpoint self-check)

**Three semantics that bind the adapter, and one that constrains it:**
1. **Quota exhaustion splits across 429 *and* 403.** A 403 here is a *limit* event (weekly window), not an auth event — while a different 403 (`"Access terminated"`) is the *policy-revocation* event. **HTTP status alone is not a classifier for this lane**; the message string is load-bearing. This is a materially different shape from Z.AI's numeric `code` field.
2. **Entitlement failures arrive as 401**, not 403 — tier-gate misses (`does not have access to k3`) look like auth failures on status alone and must **not** trip the policy-revocation alert.
3. **`"The engine is currently overloaded"` is a transient 429**, distinct from a quota 429 — retry, not park.
4. **Constraint: no reset-time header is documented.** The error reference documents no `x-ratelimit-*` or reset timestamp; reset times are stated to be viewable in the Console dashboard. So the adapter **parks on signal and re-probes**; it cannot read a reset instant off the wire. Same D4 posture as every other lane (react, never predict) — but note this is *weaker* than Z.AI, whose codes 1308/1310 embed `{next_flush_time}` in the message. **UNVERIFIED whether any reset header exists in practice**; a live probe at bring-up should check response headers before the design is frozen.

No evidence of silent throttling or quality degradation without a signal was found → no §5 B4 flag.

### B5. Endpoint discipline — **PASS (documented, and the BytePlus lesson applies)**

There are **two** metered platform base URLs and **one** plan base URL:

| Surface | Base URL | Billing | Key source |
|---|---|---|---|
| **Kimi Code (the plan)** | `https://api.kimi.com/coding/v1` | **Membership subscription (quota included)** | Kimi Code Console |
| Kimi Open Platform (intl) | `https://api.moonshot.ai/v1` | Pay-as-you-go | Kimi Open Platform |
| Kimi Open Platform (as shown in the Kimi Code FAQ table) | `https://api.moonshot.cn/v1` | Pay-as-you-go | Kimi Open Platform |

The vendor's own warning, verbatim from the Kimi Code FAQ (accessed 2026-08-24):

> **"Kimi Code membership benefits and the Kimi Platform have different Base URLs. Please ensure your Base URL and API Key are matched correctly."**

**Reading, and the honest limit of it:** keys are **platform-scoped** — issued from two different consoles, billing against two different regimes. This is a *weaker* published guarantee than Z.AI's, which states outright that a request to the wrong endpoint does not draw on the subscription and spends pay-as-you-go balance instead. Moonshot documents the separation and warns about mismatching, but **does not publish what a membership key does when sent to `api.moonshot.ai/v1`** — that specific failure mode is **UNVERIFIED** (attempted: the FAQ table above, `https://platform.kimi.ai/docs/api/overview`, `https://platform.kimi.ai/docs/pricing/limits`; none state it).

**Corroborating structural fact:** the metered platform requires a **funded balance** — `https://platform.kimi.ai/docs/pricing/limits` gives Tier0 at **$1 cumulative recharge**, and a minimum $1 recharge is required to serve requests at all. On a membership-only account with no platform balance, the plausible outcome of a wrong-endpoint call is an auth or insufficient-balance error rather than a silent charge. **Plausible is not proven.**

**Design consequence (identical treatment to the Z.AI lane, which is the point):** pin `https://api.kimi.com/coding/v1`; carry an **endpoint marker on `/coding/v1`**; record **both** `api.moonshot.ai/v1` and `api.moonshot.cn/v1` as `wired: false` recorded endpoints so the self-check owns the wrong-endpoint case *by name*; never wire either. The BytePlus lesson is honored by construction rather than by trusting an unpublished behavior.

---

## Gate C — Economics & risk (D5, 3.10)

### C1. Overflow behavior — **`opt-in-credits`, with a PROVEN disable → PASS (§5 C1, P-T17-2)**

This is the gate that would have rejected the lane, and it clears on primary evidence.

From `https://www.kimi.com/en/help/membership/membership-extra-usage` (accessed 2026-08-24):
- **Default state: OFF.** Extra Usage requires explicit activation.
- Enabling, verbatim: *"After you turn on 'Extra Usage Credit Pack,' the system will automatically deduct from your Extra Usage Pack balance when your subscription credits run out."*
- **Disable, verbatim — the clause that clears 3.10:** *"You can turn it off at any time: your balance stays in your account and the system pauses spending from it; turn it back on to resume."*
- Optional monthly spending cap, itself **off by default**; if set it must exceed the ¥25 minimum single top-up, and *"Once your monthly spending reaches the cap, the system pauses Extra Usage Pack spending for the rest of the month."*
- Top-up bounds: min ¥25, max 10 top-ups and ¥3,000 per day, balance cap ¥10,000.

From `https://www.kimi.com/code/docs/en/kimi-code/membership.html` (accessed 2026-08-24): with Extra Usage enabled, users *"keep making requests using your Extra Usage balance without waiting for a refresh"* and operations *"don't stop or error"*.

From the changelog (`whats-new.html`), Extra Usage shipped **2026-07-09** — i.e. *after* the July prior's access date of 2026-07-16 in the same month; it is a recent mechanism, which is itself a churn signal (C4).

**Classification: `opt-in-credits`.** It is emphatically **not** `auto-metered`: it is off by default, requires funding, and carries an explicit any-time disable. It is not `hard-stop` either — with the feature on, exhaustion becomes a silent currency change, which is exactly the 3.10 hazard the flag exists to name.

**Two consequences that must be carried into the build, not left as prose:**
1. **The Kimi lane is Sinet's first non-`hard-stop` lane** (`zai` is `hard-stop`). The 3.10 requirement that **receipts visibly change currency when overflow triggers** stops being theoretical the moment this lane exists — §5 C1 and report-01 P-T01-3 both require it.
2. **Recommended operator posture: leave Extra Usage OFF.** With it off, the lane behaves as a true hard-stop: the documented 429/403 quota messages fire and Sinet parks. That keeps the lane inside the same discipline `zai` already runs under, and makes the ¥-denominated top-up rails (A4) irrelevant. Turning it on is a deliberate, reversible operator act — and must flip the lane's billing flag through the rehearsed kill-switch operation, never silently.

### C2. Price row (D5) — **RE-VERIFIED, matches the July prior**

From `https://platform.kimi.ai/docs/pricing/chat-k3` (accessed 2026-08-24):

| Metric | Price |
|---|---|
| Input (cache **miss**) | **$3.00** / 1M tokens |
| Input (cache **hit**) | **$0.30** / 1M tokens |
| Output | **$15.00** / 1M tokens |
| Context window | 1,048,576 tokens |
| Currency | USD |

Verbatim note: *"Prices exclude applicable taxes. Specific tax obligations are subject to local tax regulations and will be calculated at checkout based on your jurisdiction."*

The July prior's `$3/$15 per M` is **confirmed**, and this audit adds the cache-hit rate ($0.30/M) the prior lacked. These are the **API-equivalent** numbers that populate the D5 table — the plan itself is flat, and opencode reports `$0.00`, so the dollar figures on receipts must come from these rows.

### C3. Window shape — recorded as data: **three overlapping windows, two different units**

| Window | Unit | Numbers | Reset behavior |
|---|---|---|---|
| Rolling **5-hour** | **requests** | *"approximately 300–1,200 requests per 5-hour window, with up to 30 concurrent requests"* (`kimi.com/code/docs/en/`) | rolling |
| **7-day** Kimi Code allowance | **credits / tokens** | per-tier multiplier (1× / 5× / 15× / 30× reported by secondary sources — **UNVERIFIED**) | *"Kimi Code's quota refreshes automatically every 7 days"* (`membership.html`); *"Credits refresh on a 7-day cycle"* from the subscription date, and *"unused credits do not carry over to the next cycle"* (`help/kimi-code/benefits`) |
| **Monthly** membership credit pool | credits | shared with the rest of the Kimi membership | monthly billing cycle; 429 *"You've reached kimi monthly usage limit for this billing cycle"* |

**Three facts worth carrying explicitly:**
- **The 7-day window is subscription-anchored, not a calendar week** — structurally the same trap as the Z.AI lane's order-anchored weekly window, and it must be modeled the same way.
- **The 5-hour window counts requests; the weekly counts credits/tokens.** A single "units" scalar cannot describe this lane. Whatever the plan-data document does, it must let a lane carry per-quota units rather than one lane-wide unit.
- **The quota is shared across all signed-in devices and the API key** — Sinet is one consumer of a pool the operator also draws on interactively. Consumption-pressure routing must treat its own count as a **lower bound**, never as the pool state.

Per D4 this shape is *consumed by consumption-pressure routing and receipts, never by prediction*.

### C4. Churn & exit

**Churn history — Kimi is a fast-moving surface, and this is a real risk row.** From `whats-new.html` (accessed 2026-08-24): **~40 CLI releases between 2026-05 and 2026-08-20**; **K3 launched 2026-07-16** (2.8T params, 1M context, open-sourced); **K2.7 Code 2026-06-12**; **HighSpeed tier and Extra Usage both added 2026-07-09**. The Kimi Code Benefits help page states verbatim: **"A new membership system is coming soon."**

That last sentence is the highest-value early-warning signal in this audit: **the membership structure this lane is being seeded from is announced as changing.** Every tier name, price, allowance and multiplier recorded here must be treated as a dated seed with a short half-life, and the lane's watch rows must be live *before* go-live rather than after.

**Exit is config-only:** the lane is an opencode provider entry plus a member API key. Exit = delete the lane document, revoke the key in the Kimi Code Console, remove the plan document. **No code change**, provided the machinery stays data-driven — which is precisely what the implementation must preserve and prove by test.

**Watch rows required before go-live** (R02 §6 tier 1, plus what this audit learned):
- `https://www.kimi.com/coding` and `https://www.kimi.com/membership/pricing` — tier-1 page diffs (**both JS shells**: a text diff will be near-empty and must not be mistaken for "no change"; flag them as low-signal and lean on the alternatives below)
- `https://www.kimi.com/en/help/membership/membership-pricing` and `https://www.kimi.com/en/help/kimi-code/benefits` — the help center actually renders, and carries the "new membership system is coming soon" notice
- `https://www.kimi.com/code/docs/en/kimi-code/whats-new.html` — the vendor changelog; carries model, plan and Extra-Usage changes
- `https://www.kimi.com/code/docs/en/kimi-code/error-reference.html` — **the classifier's source of truth**; a diff here can silently invalidate the limit classifier
- `https://platform.kimi.ai/docs/pricing/chat-k3` — the D5 price row
- `https://platform.kimi.ai/docs/agreement/modeluse` and `https://www.kimi.com/user/agreement/modelUse` — the Gate-A re-audit trigger; a class-3 clause appearing here freezes the lane
- `https://www.kimi.com/en/help/membership/membership-extra-usage` — the C1 overflow mechanism
- `https://models.dev/providers/kimi-for-coding` — provider id / npm / base URL drift (Tier 2 canary)

### C5. Data & compliance — **PRC data residency: routing rider REQUIRED**

From `https://www.kimi.com/user/agreement/userPrivacy?version=v2` (accessed 2026-08-24), verbatim:
- **Operating entity:** *"北京月之暗面科技有限公司"* — Beijing Moonshot AI, registered at *"北京市海淀区知春路76号京东科技大厦1栋13层"*.
- **Residency:** *"我们将您的个人信息存储于中华人民共和国境内"* — personal information is stored **within mainland China**. The policy states no cross-border transfer without separate consent and legal compliance.
- **Training on inputs — ON by default:** *"我们会将您输入输出的内容用来优化模型"* (input and output content is used to optimize models). **Opt-out is contact-based:** *"您可以通过本协议第11节所载的联系方式联系我们的客服并提出要求"* — the user must contact customer service to request it.
- **Retention:** *"我们仅在为实现服务目的所必需的时间内保留您的个人信息"*; on discontinuation, data is anonymized or deleted within a reasonable period unless law requires otherwise.
- **GDPR / EEA:** **no GDPR or EEA-specific language present**; no international-transfer provisions.

**Jurisdictional ambiguity, recorded honestly and NOT resolved:** the two agreements name **different governing law**.
- Kimi **OpenPlatform** ToS §12: *"The establishment, effectiveness, interpretation, revision, supplementation, termination, enforcement, and dispute resolution of these terms shall all be governed by the laws of Singapore"*, with SIAC arbitration seated in Singapore, in English. Its training clause is softer and enterprise-negotiable: *"We may use Content to provide, maintain, develop, support, and improve the Services... Customer who requires restrictions on the use of Customer Content for training or improving Moonshot AI models may contact Moonshot AI to discuss available enterprise arrangements or separate written agreements."*
- Kimi **assistant** ToS: *"本协议之订立、生效、解释、修订、补充、终止、执行与争议解决均适用中华人民共和国大陆地区法律"* — **PRC mainland law**.

**Which one governs a Kimi Code *membership* is UNVERIFIED.** No page reachable in this session states it. The membership is bought on `kimi.com` (assistant property → PRC-law ToS, PRC-residency privacy policy) but is consumed through a developer API surface (OpenPlatform → Singapore law). Attempted: both ToS URLs above, `https://platform.kimi.ai/docs/agreement/privacy` (returned the Quickstart page, not a privacy policy), `https://www.kimi.com/en/help/kimi-code/faq`.

**Rider proposed to the operator (mirrors the DeepSeek treatment in R02 §4, and is required here rather than advisory):**

> **The Kimi lane carries a no-household-personal-data routing constraint.** Under either governing-law reading the lane is PRC-adjacent: the operating entity is a Beijing company, the published privacy policy places personal information inside mainland China, there is no GDPR/EEA language, and training on inputs is the default with only a contact-based opt-out. The lane is therefore approved for **code and general technical work only**; household personal data, personal correspondence, and identity-bearing content must never route to it. This is a routing-policy row on the lane, enforced where the other per-lane data-policy rows are enforced, not a note in a document.

Two supporting points for the operator's decision: the operator's *interactive* Kimi use already sits under these same terms — the rider constrains **Sinet's automated routing**, which is the part Sinet controls. And the training default is the sharper edge of the two: residency is a compliance posture, but training-on-inputs means content is retained into a model.

---

## What this audit changes versus the July prior (R02 §2.1 Moonshot row, verified-on 2026-07-16)

| Field | July prior | This audit (2026-08-24) |
|---|---|---|
| Automation stance | class 2, member keys "explicitly for third-party tools", no unattended ban | **Confirmed class 2**, on two full ToS reads; strengthened by an explicit named-tool list and vendor-endorsed unattended CLI modes |
| Protocols | "OpenAI + Anthropic protocols" | **Confirmed both**, with exact base URLs; **new:** opencode's first-party provider rides **`@ai-sdk/anthropic`**, not openai-compatible |
| K3 tier gate | "gated to ~Moderato+ tiers, 256K ctx; 1M ctx on ~Allegretto+" | **Confirmed from primary docs and error strings**: `k3` and `k3-256k` "available to Moderato members and above"; the 401 *"Your current plan supports only kimi-k3 up to 256K context"* is the live enforcement |
| API price | "$3/$15 per M" | **Re-verified $3.00/$15.00**; **new:** cache-hit $0.30/M, 1,048,576 ctx |
| Window shape | "5h + weekly, Anthropic-mirroring" | **Refined:** *three* windows — rolling 5h in **requests**, 7-day subscription-anchored in **credits**, plus a **monthly** shared membership pool; quota shared across all devices and the API key |
| Overflow | not assessed | **NEW and load-bearing:** Extra Usage = **`opt-in-credits`**, off by default, **proven disable** (shipped 2026-07-09, after the prior's access date) |
| Tier prices | "$19–$199, name→price mapping MED-confidence" | **Still MED-confidence and UNVERIFIED** — the USD plan pages are JS shells; the help center serves ¥ prices |
| Data/compliance | not assessed | **NEW:** Beijing entity, mainland-China residency, training-on-inputs by default, no GDPR language; governing law **ambiguous** (Singapore for OpenPlatform vs PRC for the assistant) |

---

## UNVERIFIED register (every field, with the attempted URL)

| # | Field | Why unverified | Attempted |
|---|---|---|---|
| U1 | International **USD tier→price** mapping (Moderato/Allegretto/Allegro/Vivace at $19–$199) | plan pages are JS shells; help center serves ¥ prices | `kimi.com/membership/pricing`, `kimi.com/coding`, `kimi.com/en/help/membership/membership-pricing` |
| U2 | Per-tier **7-day allowance multipliers** (1×/5×/15×/30×) | secondary aggregators only; no primary page | same as U1 |
| U3 | `/usage` **response field names and shape** | mentioned but never specified | `code/docs/en/kimi-code/faq.html`, `…/quota.html` (**404**), targeted search |
| U4 | Whether any **reset-time header** exists on 429 responses | error reference documents none; Console-only is stated | `code/docs/en/kimi-code/error-reference.html` |
| U5 | What a **membership key does against `api.moonshot.ai/v1`** (the exact B5 failure mode) | separation documented, behavior not published | FAQ table, `platform.kimi.ai/docs/api/overview`, `…/pricing/limits` |
| U6 | **Which agreement governs a Kimi Code membership** (Singapore vs PRC law) | the two agreements differ; none states which binds the membership | both ToS URLs, `platform.kimi.ai/docs/agreement/privacy` (served Quickstart) |
| U7 | International **payment rails** from Germany/EU at primary grade | no primary page reachable | as U1 |
| U8 | opencode **provider id** exactly (`kimi-for-coding` vs `moonshotai`) | provider URL resolves as `kimi-for-coding`; one pass inferred `moonshotai` | `models.dev/providers/kimi-for-coding`, `models.dev/api.json` |
| U9 | Viability for **additional household members** | only the operator's own subscription is demonstrated | n/a — closes when a second member subscribes |

**U1, U2, U3, U4, U5 and U8 all close the same way: from the operator's own account and a live probe at bring-up**, which is the P-T17-3 posture anyway (the account's observed state is the runtime authority; docs are the seed). None of them blocks the build. **U6 does not block either** — the C5 rider is written to hold under *both* readings. **U7 and U9 are operator-facts, not platform-facts.**

---

## Sources

Every URL below was fetched live on **2026-08-24**; quotes above are verbatim from these pages.

1. https://platform.kimi.ai/docs/agreement/modeluse — Terms of Service for Kimi OpenPlatform
2. https://www.kimi.com/user/agreement/modelUse — Kimi assistant Terms of Service
3. https://www.kimi.com/user/agreement/userPrivacy?version=v2 — Kimi Privacy Policy
4. https://www.kimi.com/code/docs/en/ — Kimi Code Overview (base URLs, tools, models, tier gates, 5h window)
5. https://www.kimi.com/code/docs/en/kimi-code/error-reference.html — Error Reference (the limit-classifier source)
6. https://www.kimi.com/code/docs/en/kimi-code/membership.html — Membership Benefits (7-day refresh, Extra Usage behavior)
7. https://www.kimi.com/code/docs/en/kimi-code/faq.html — FAQ (endpoint-discipline table, `/usage`)
8. https://www.kimi.com/code/docs/en/kimi-code/whats-new.html — Changelog (K3, HighSpeed + Extra Usage 2026-07-09, ~40 releases)
9. https://www.kimi.com/code/docs/en/third-party-tools/opencode.html — official OpenCode integration
10. https://www.kimi.com/en/help/membership/membership-extra-usage — Extra Usage Pack (the C1 disable clause)
11. https://www.kimi.com/en/help/kimi-code/benefits — Benefits and Billing (shared quota, "new membership system is coming soon")
12. https://www.kimi.com/en/help/membership/membership-pricing — membership tiers (¥ prices)
13. https://platform.kimi.ai/docs/pricing/chat-k3 — K3 metered prices (the D5 row)
14. https://platform.kimi.ai/docs/pricing/limits — recharge tiers / rate limits (metered platform)
15. https://platform.kimi.ai/docs/api/overview — metered platform API overview (`api.moonshot.ai/v1`)
16. https://models.dev/providers/kimi-for-coding — opencode provider record (npm, base URL, model ids)
17. https://www.kimi.com/coding — **JS shell** (recorded as a gap)
18. https://www.kimi.com/membership/pricing — **JS shell** (recorded as a gap)

---

## Addendum — 2026-08-26 · Gate A RE-AUDITED, and this audit's own verdict corrected (P3-LN-7 / S00.9 A12 / R30)

**This addendum outranks §0's Gate-A row above.** The 2026-08-24 verdict was reached without reading
a first-party page that materially changes it, and this record's whole value is that its claims are
dated — so the omission is recorded here rather than repaired invisibly.

### The omission

Gate A passed **class 2** on the strength of two full ToS reads and the sentence *"neither contains
any automation / unattended-use / interactive-only clause"*. That sentence remains **true of the two
documents it names**. It is not true of the membership's usage policy as a whole: a **third,
previously unread** first-party surface exists —
`https://www.kimi.com/code/docs/en/kimi-code/community-guidelines.html` — and it is not among the 18
sources listed above. Read on **2026-08-26**, it carries verbatim:

```
Scope of Use

Kimi Code subscriptions are for interactive use only.
```
```
Don't use Kimi Code for non-interactive automation

Kimi Code subscriptions are for personal interactive use only. Using it for non-interactive
purposes — such as scripted batch execution or data annotation pipelines — goes beyond normal use.
```

Under S03.6's ratified taxonomy that is **class 3** language — *"explicit interactive-only /
automation-banned → auto-disqualifying"* — stated on a first-party page.

### The pre-registered trigger fired

This audit's own §6 watchlist entry pre-registered exactly this text, verbatim:

> **Would-change trigger (for the §6 watchlist):** appearance in either ToS of an interactive-only /
> no-automation / no-non-interactive-batch clause of the kind Alibaba, Tencent and Xiaomi carry
> verbatim → **immediate re-audit and lane freeze before the next run consumes allowance.**

The trigger has fired. It binds the **already-commissioned `kimi` lane** exactly as it binds the new
`kimi-cli` lane: it is one subscription, one quota pool, and the clause is a property of the
subscription rather than of the client path. This is a **gap in this audit's source coverage**, not a
change in Moonshot's position — the page predates the audit.

### The countervailing text, quoted so the ruling rests on both halves

The same page sanctions agent frameworks by name and describes graduated enforcement:

```
We're compatible with mainstream coding tools and agent frameworks (Kimi CLI, VS Code, Claude Code,
OpenCode, OpenClaw, etc.), so you can call Kimi Code's AI capabilities from the tools you already use.
```
```
Q3: I use Kimi Code across multiple devices and tools at the same time. Will that get me suspended?
No. Switching between devices (e.g., work laptop, personal machine) or different coding tools (Kimi
CLI, VS Code, Claude Code) is a completely normal usage pattern.
```
```
If your usage doesn't align with the guidelines above, we'll review the situation first and take
appropriate action—such as limiting concurrent access—based on the severity.
```

### The ruling

**Operator ruling, 2026-08-26 (in-session; coordinator-presented option form, operator-selected —
their authoritative act): PROCEED, acceptance recorded.** Recorded reasoning, as selected:

> personal interactive use through agent frameworks the guidelines page itself sanctions (Kimi CLI,
> VS Code, Claude Code, OpenCode); the banned examples (scripted batch execution, data-annotation
> pipelines) do not describe this use; stated enforcement is graduated (concurrency limiting);
> accepted as a recorded gray zone, the same posture as the Anthropic lane's G1 P2 note.

**Corrected Gate-A verdict: PASS as a RECORDED GRAY ZONE on explicit operator acceptance**, replacing
the unqualified class-2 PASS above. Both kimi lanes proceed. Neither lane is frozen.

### What this addendum changes downstream

1. `403 "You've reached your concurrent request limit"` is **both** an ordinary concurrency shed
   **and** the vendor's stated enforcement signal for a terms concern. It therefore carries **no
   `documented_class`** on either lane document and falls through to the Class-4 status rule (freeze
   + operator alert). Classing it `transient` would retry silently through an enforcement action
   against the operator's own account.
2. `community-guidelines.html` joins the watchlist as a **tier-1 row** on both kimi lanes. This audit
   not watching it is precisely how the clause stayed invisible for two days.
3. The escape route the capture suggested — a Kimi **Open Platform** pay-as-you-go key — remains
   **closed at v0**: it is a metered lane, G1 P7 keeps the metered-exception list EMPTY with DeepSeek
   the sole pre-registered exception, and it would need its own amendment and its own Gate A–C audit.
   No Open Platform ToS document was locatable in the 2026-08-26 capture.

**Sources added by this addendum:**

19. https://www.kimi.com/code/docs/en/kimi-code/community-guidelines.html — Community Guidelines / Scope of Use (read 2026-08-26; the class-3 language and its countervailing FAQ)
