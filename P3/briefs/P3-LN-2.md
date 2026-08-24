# ⚠️ EXPIRED 2026-08-24 — single-use artifact; served BOTH split halves (2A landed `80a17f3`, 2B landed with drains `ac6b91b`/`5411c74`); the code no longer matches this text (notably: R28's fixture line was set aside by coordinator order for the contract-snapshot regeneration, §63). Later grounding must never read this as truth (amendment D).

# P3-LN-2 — Z.AI/GLM lane commissioning (grounded brief)

**Packet:** P3-LN-2 · **Grounded:** 2026-08-23 · **Substrate:** `internal/adapters/opencode` (landed P3-LN-1, commit `25d41cc`)
**Binding contract:** `Spec/core-architecture-v1.md`; drafts in `Spec/drafts/` are canonical.
**Read-first sections:** S03.2 · S03.6 · S03.7 · S10.1–S10.5 · S10.7 · S11 (full, esp. S11.1/S11.3/S11.5) · S08.8 · S14.6 · S18.1/S18.3/S18.4 · `P3/CONVENTIONS.md` §10, §11, §12, §26, §61.

This brief is self-contained. Read it plus the cited sections plus `P3/CONVENTIONS.md`. **Do not read other briefs** — expired briefs lie; code and spec are the only truth.

---

## 0. What this packet is, and what it is NOT

S03.6 is the governing sentence: *"Adding a lane is a provider entry per user plus billing flags — never a new substrate."* LN-1 landed the substrate. LN-2 commissions the **first lane on it**: `zai`.

**NOT in this packet:** a new substrate; a new engine adoption; any live provider call; any credential material anywhere in the repo; the local engine lane (class (a), §26 — a separate lane commissioning); the credential-injection proxy (see OQ-1); the per-user jail (see OQ-2); anything under `web/src`.

**$0 is absolute.** No live provider call. The operator holds GLM Coding Max, but **no real key exists in the system at this packet** — key placement is `LN-CEREMONY`, a later queue item. Everything here runs against fixtures and the loopback fake OpenAI-compatible provider LN-1 already built (`fakeprovider_test.go`). Tier L stays structurally unreachable and prints its named skip.

---

## 1. Live verification — the 2026-07-16 audit row re-stamped 2026-08-23

Binding rule: provider facts are verified at time of use, never from memory or the archive. All fetches below are PRIMARY (`docs.z.ai`). **These are DATA with dates, never constants.**

| # | Field | July-2026 audit prior | **Verified 2026-08-23** | Source URL |
|---|---|---|---|---|
| V1 | Coding-plan endpoint (OpenAI-compatible) | `https://api.z.ai/api/coding/paas/v4` | **UNCHANGED — confirmed.** Docs warn it is not interchangeable with the general `/api/paas/v4`; wrong endpoint ⇒ the subscription quota is not used | `https://docs.z.ai/devpack/tool/others`, `https://docs.z.ai/devpack/faq` |
| V2 | Anthropic-protocol variant | not recorded | **SANCTIONED: `https://api.z.ai/api/anthropic`** — docs assign it to Claude Code and Goose; other tools use V1 | `https://docs.z.ai/devpack/tool/claude`, `https://docs.z.ai/devpack/faq` |
| V3 | Models the coding plan serves | GLM-5.2 + GLM-5-Turbo + GLM-4.7 | **CHANGED → GLM-5.3 + GLM-5-Turbo + GLM-4.7** (+ GLM-4.6V via Vision MCP). Verbatim: *"Requests for previous models (GLM-5.2/GLM-5.1) will be automatically routed to GLM-5.3."* | `https://docs.z.ai/devpack/overview` |
| V4 | Literal API model id strings | — | **`"glm-5.3"`, `"glm-5-turbo"`, `"glm-4.7"`** — lowercase, from the `model` parameter enum. (`"glm-5.2"`, `"glm-5.1"` remain valid platform-wide enum values; on the coding plan they route to 5.3 per V3.) | `https://docs.z.ai/api-reference/llm/chat-completion` |
| V5 | Quota mechanics | "~1,600 prompts/5 h" marketing shape (S10.4, G1 D1.5) | **CHANGED SHAPE — credits, not prompts.** Lite 2,000/5 h + 10,000/wk · Pro 12,000/5 h + 60,000/wk · **Max 28,000/5 h + 140,000/wk** | `https://docs.z.ai/devpack/overview` |
| V6 | Peak/off-peak multipliers | "documented peak 3× / off-peak 2×" (S10.1) | **CHANGED — verbatim:** *"During off-peak hours, model usage is charged at 50% of the standard credit rate."* Peak = *"Monday to Friday, 14:00–18:00 Singapore Standard Time (UTC+8)."* ⇒ peak 1.0×, off-peak 0.5× | `https://docs.z.ai/devpack/overview` |
| V7 | Weekly window reset | — | *"starts counting from the time you place your order, and the quota is refreshed and reset on a 7-day cycle"* — per-account, order-anchored | `https://docs.z.ai/devpack/faq` |
| V8 | `thinking` effort knob (S03.7 / S10.6 Eco lever) | `thinking:{type:disabled}` | **STILL SUPPORTED.** Shape `{"type": "enabled"\|"disabled", "clear_thinking": bool}`; default `enabled`; *"GLM-4.5 series and higher models"* | `https://docs.z.ai/api-reference/llm/chat-completion` |
| V9 | `reasoning_effort` (NEW, not in spec) | — | `max`/`xhigh`/`high`/`medium`/`low`/`minimal`/`none`; default `max`; GLM-5.2 and above; **GLM-5.3 supports only `low`/`high`/`max`**; *"takes effect when `thinking` is enabled"* | same as V8 |
| V10 | Response `usage` fields | per-request token usage incl. cached detail | `prompt_tokens`, `completion_tokens`, `prompt_tokens_details.cached_tokens`, `total_tokens` | same as V8 |
| V11 | logprobs | absent endpoint-wide | **Not documented on chat-completion** — S03.7's behavioral-eval-only posture stands unchanged | same as V8 |
| V12 | Limit/auth error codes | 1302/1305/1308/1310/1113 from R02 + SPIKE P2-S1 | **ALL CONFIRMED VERBATIM** (table below) | `https://docs.z.ai/api-reference/api-code` |

### V12 detail — the wire signal set, verbatim (this is the classifier's fixture source)

| Code | HTTP | Verbatim message | S10.5 class |
|---|---|---|---|
| `1000` | 401 | Authentication Failed | 4 |
| `1001` | 401 | "Authentication parameter not received in Header, unable to authenticate" | 4 |
| `1003` | 401 | "Authentication Token expired, please regenerate/obtain" | 4 |
| `1220` | 403 | "You do not have permission to access ${API_name}" | 4 |
| `1113` | **429** | "Insufficient balance or no resource package. Please recharge." | 3 (after endpoint self-check — R11) |
| `1211` | 400 | "Unknown Model, please check the model code." | not a limit event (see R13) |
| `1302` | 429 | "Rate limit reached for requests" | 1 |
| `1305` | 429 | "The service may be temporarily overloaded, please try again later" | 1 |
| `1308` | 429 | "Usage limit reached for \`{number}\` \`{unit}\`. Your limit will reset at \`{next_flush_time}\`" | 2 |
| `1310` | 429 | "Weekly/Monthly Limit Exhausted. Your limit will reset at \`{next_flush_time}\`" | 2 |
| `1311`–`1321` | 429 | "Various subscription/usage limit violations" | **NOT IN SPEC → OQ-3** |

Error body shape, verbatim: `{"error":{"code":"1214","message":"Parameter `${field}` is invalid. Please check the documentation."}}` — **note `code` is a JSON *string*, not a number.**

Two notes that matter for classification: **`1113` is HTTP 429, not 402** (so a status-only rule cannot reach Class 4 through it), and **`next_flush_time` is confirmed present in the 1308/1310 message text**, exactly as S10.5 assumes.

### UNVERIFIED fields (recorded honestly, never filled from memory)

| Field | Attempted URL | Date | Result |
|---|---|---|---|
| Dated release-notes stream for the coding plan | `https://docs.z.ai/devpack/release-notes` | 2026-08-23 | **HTTP 404.** No dated changelog was reachable at that path. The watchlist seed row `t1-zai-release-notes` points at `https://docs.z.ai/release-notes/new-released` (see R20 — verify or repoint) |
| Consolidated error-code page at the spec-era path | `https://docs.z.ai/api-reference/error-code` | 2026-08-23 | HTTP 404 — **superseded, not lost**: the live path is `https://docs.z.ai/api-reference/api-code` (V12 above fetched successfully there) |
| `1311`–`1321` individual meanings | `https://docs.z.ai/api-reference/api-code` | 2026-08-23 | Page collapses the band to one row, "Various subscription/usage limit violations". Individual semantics **UNVERIFIED** → OQ-3 |
| Whether the *coding* endpoint honors `reasoning_effort` (V9) | — | 2026-08-23 | Documented on the general chat-completion reference; **no coding-endpoint-specific statement found.** Do not assume → OQ-4 |
| Whether `/models` is served on the coding endpoint | — | 2026-08-23 | Not documented. The model-list canary (R19) must therefore **tolerate absence** and report it, never crash |

**How the drift lands (this is the design point, not a footnote):** V3/V5/V6 all moved in five weeks. That is precisely why S03.6 makes model ids per-user config DATA with `verified-on` dates and why P-T17-3 makes the **account's observed list** — never the docs — the truth. The verified values above are *seed data with a date stamp*, not constants, and not the authority.

---

## 2. What already exists (code truth, verified this session — do not rebuild it)

| Surface | State today | LN-2's job |
|---|---|---|
| `internal/adapters/opencode/` | Full D3 substrate: `opencode.go`, `serve.go`, `client.go`, `lower.go`, `parse.go` + suites | consume it; **no substrate changes beyond the named seams** |
| `Adapter.Providers ProviderConfig` (`opencode.go:120-146`) | The lane-config seam **exists**, documented *"Lane values are DATA, never compiled-in identifiers"*. `ProviderConfig = map[string]ProviderEntry`; `ProviderEntry{NPM, Name, Options map[string]any, Models map[string]ModelEntry}`; compiled into `engineConfig.Provider` at `lower.go:102-109`/`161-173`. **`baseURL` is not a typed field — it rides `Options map[string]any`.** The only in-repo shape is the test fixture `conformance_test.go:41-50` (`NPM:"@ai-sdk/openai-compatible"`, `Options:{"baseURL": …}`, `Models:{…}`) | populate it — **per user** (R2), never process-wide |
| Config delivery (`serve.go:330-334`) | the compiled config reaches the engine as **`OPENCODE_CONFIG_CONTENT` in the env** — never a file, never argv | so the provider block (and any credential in it) is an **env** fact: it flows through CredInject and into the identity digest (R8/R10) |
| `identityOf` (`serve.go:169-180`) | sha256 over `ConfigJSON`, `Root`, `Cwd`, **and every env entry** | a provider-entry or credential change restarts that user's serve — correct, assert it (R2/R10) |
| `parseModel` (`lower.go:225-231`) | platform model string is `<providerID>/<modelID>`, marshalled per-turn as `{"providerID":…,"modelID":…}` in `promptBody` (`client.go:164-169`) | zai models are `zai-coding-plan/glm-5.3` etc. |
| `adapters.LaneZAI = "zai"` (`adapters.go:51`) | the **exported** lane-name constant already exists | use it; do not mint a variant |
| `internal/shell/shell.go:583-588` | registers **exactly one** adapter: `adapters.SubstrateClaudeCLI → claudecli.Adapter`. The opencode `Adapter` has **zero production call sites** anywhere | **R30 — this is where the lane actually goes live** |
| `internal/conformance/registry.go:192, 204, 227` | Notes say verbatim *"The zai lane is OMITTED"* and that its row / lane canary rows **complete at LN-2** | R31 |
| `internal/watchlist/prices.go:157` | provider-id→lane alias map already carries `"zhipuai": "zai"` | the hook R16's price rows land on |
| `internal/watchlist/watchlist.go:195`, `classify.go:70` | `"zai"` already in the lane allowlist and the drift-classifier enum | nothing owed |
| `internal/scheduler/limits.go` | Classifier landed at B1-2. `laneZAI = "zai"` (l.23); **1302/1305 → Class 1** (l.201-202); **1308/1310 + ResetAt → Class 2** (l.212-213); **1113 → Class 3** (l.223-224); Class 4 checked FIRST (l.145-155) | R11 (endpoint self-check), R12 (signal extraction), R13/OQ-3 |
| `internal/scheduler/limits_test.go:28-38` | Z.AI fixtures for 1302/1305/1308/1310/1113 **already present** | extend, don't duplicate (sanctioned edit, §8) |
| `LimitSignal` (`limits.go:54-78`) | carries `Lane`, `HTTPStatus`, `ErrorCode`, `ResetAt`, `BodyText`, `OnValidCredentials`, `EngineBudgetExhausted` | needs the endpoint-verification input (R11) |
| `internal/watchlist/canary.go:687-696` | `LaneAnthropic`/`LaneZAI` + `PaidLanes() []string{LaneAnthropic, LaneZAI}` **already return zai**. Canary types exist as struct fields on `Canaries` (`canary.go:260-283`), nil-safe: `Auth`, `Behavioral`, `Logprob`, `ModelList`, `Conformance`. **`canary_auth.go:62,69`, `canary_behavioral.go:65,71`, `canary_models.go:55,61` already iterate `PaidLanes()` — so zai is structurally in scope today.** Real-request legs are **DISARMED** at v0 behind `CanaryArmEnv = "SINET_CANARY_ARM"` (`canary.go:88-115`, `ErrCanaryDisarmed`) | R19/R20 are **probe wiring + assertions**, not row registration — see the corrected requirements |
| `internal/watchlist/seed.go` | zai **watch** rows (not canaries) exist: `t1-zai-devpack` → `https://docs.z.ai/devpack/overview`, `t1-zai-release-notes` → `https://docs.z.ai/release-notes/new-released`, `t4-hn-zai`. 101 seed rows total | R20 — re-verify URLs against §1 |
| `internal/metering/` | `ApproximationTier` 1–5 (`metering.go:69-81`), derived at `metering.go:162-167` as **measured→1 else 5** (tier 3 is documented-but-unreached, `metering.go:55-68`, reserved verbatim for *"derived plan units (Z.AI prompts)"*). `LineItem.Tier` takes the **worst** tier (`ledger.go:49-72`). **Two independent $0 guards already exist**: `price.go:151-156` returns `Unpriced` with reason *"flat-rate lane would price from a $0 row (S10.3 coverage guard)"*, and `pricestore.go:287-318` refuses storing a zero/all-zero row. Local's TRUE $0 is a separate, deliberate path (`local.go:68-81`, `ZeroAllowance()`). `MeteredExceptions.lanes` is **unexported with only `NoMeteredExceptions()`** — there is no way to add a metered lane without a code change, by design | R14–R17 |
| `internal/worker/routing.go:542-620` `resolveSeat` | `DefaultDutyMap()` (`routing.go:126-135`) is **all-anthropic**; the utility seat is deliberately absent. `Coverage{FlatRateLanes, LocalAvailable, MeteredAllowed}`; `modelCovered` ignores the model (`_ = model`, l.625-628); comment at l.552-558 asserts *"no second adapter exists in this cut"* | R21–R23 — the comment is now FALSE |
| "one agentic lane" | **NOT enforced in code.** The only occurrence in the whole Go tree is a comment at `internal/worker/ceiling.go:23`. The de-facto enforcement is the single-adapter wiring at `shell.go:583-588` | R23 — say this precisely |
| claudecli `UpdatedInput` (`claudecli.go:157-175`, `hook.go:116-125`, `hook.go:276-289`) | a **genuine tool-input substitution**: `writeAnswer` stages `{"updatedInput":…}` to `<ctlDir>/answers/`, the PreToolUse hook reads it and emits `permissionDecision:"allow"` + `updatedInput`. **The answered path only ever writes `"allow"`** | R24 — a first-class reject needs a `deny` decision path in the hook |
| `internal/adapters/adapters.go:402-413` `Answer` | `{AskID string; UpdatedInput json.RawMessage; Continuation string}` — `UpdatedInput` set ⇒ replaces the gated call's input wholesale; **nil ⇒ approve unchanged**. No reject member | R24–R26 |
| `internal/broker/` | Per-user UDS + **SO_PEERCRED** (`broker.go:88-94`); `KindEngineCred = "engine-cred"` **delivered at spawn**, `KindSigningKey` result-only, enforced as a destination constraint (`broker.go:132-142`: `kind != KindEngineCred → ErrWrongKind`). Store is one AES-256-GCM file per profile at `<root>/<user>/<profile>.cred`, dir `0700`, records `0600`, **AAD = `"sinet-broker\0"+user+"\0"+profile+"\0"+kind`** (`store.go:267-269`). **The ready-made spawn path is `broker.EnvInjector(socket, profile, varName)`** (`client.go:162-178`) → wired as `StartRequest.CredInject` at `stage/runner.go:278` and `stage/direct.go:145` | R7–R10 — add a zai auth-profile + its env var name |
| `internal/adapters/opencode/opencode.go:341-351` | `CredInject` fires inside `prepare`, **transforming the SERVE environment**, resolved fresh and never stored | R7 |
| `internal/settings/` | **118 keys / 33 domains, pinned in FIVE assertions across three files**: `read_test.go:32`, `settings_test.go:44` (keys), `:79` (domains), `:450` (schema `x-sinet` leaves), `uischema_test.go:262` (domains). Per-owner and per-domain tallies are pinned too (`settings_test.go:56-78`) — S10 owns 9, S03 owns 3, domains `limit 3 · pressure 2 · budget 1 · canary 2 · adapter 3` | **must stay 118/33** (R29) |
| credential-injection proxy | **DOES NOT EXIST — verified by grep.** `internal/broker` contains no `net/http`, no `crypto/tls`, no proxy: its only mention is a *comment* at `client.go:160`. It is referenced as not-yet-built at `sandbox/confiner.go:49`, `sandbox/srt.go:94`, `shell/shell.go:346,356` (*"a B2-gate host component"*), `stage/stage.go:200`, `metering/utilization.go:22` | OQ-1 |

---

## 3. Numbered requirements

### A. The provider entry — lane config as per-user DATA (S03.2, S03.6)

**R1 · The `zai-coding-plan` provider entry is DATA, not code** (S03.6; S03.2). Build a `ProviderConfig` for the zai lane whose `Options` carry `baseURL` = the V1 endpoint and whose `Models` map carries the V4 ids. Every one of those values arrives from configuration and carries a `verified-on` date; **no endpoint string and no model id is a Go constant in a non-test file.** A seed/default set may ship (it is config seed data, like the price table), but the consuming path must read config, and a test must prove a different endpoint/model set flows end-to-end unchanged.

**R2 · Provider config resolves PER USER** (S03.6 *"a provider entry per user"*; S03.2 *"one instance per user"*). Today `Adapter.Providers` is a single value serving all runs (`opencode.go:144-146`). Extend it to a per-user resolution seam (e.g. `Adapter.ProvidersFor(userID)` or an injected resolver interface) so two users can hold different entries, different models, and different credentials. **This is load-bearing for D2**: the credential and the model set are per-person facts. Preserve the existing field's behavior as the single-user/default case so no LN-1 test regresses.
*Trap:* `Manager.Acquire`'s instance identity digest already folds the compiled config JSON and the injected env (§61(a)). Per-user providers therefore key correctly for free — but changing a user's provider entry **restarts that user's serve**. That is correct and intended; assert it, don't defeat it.

**R3 · Lane name and substrate name are the ratified constants** (CONVENTIONS §10; S03.2). Lane = `zai`. Substrate = `opencode`. Provider id inside opencode's config = `zai-coding-plan`. The platform model string is `zai-coding-plan/<modelID>` (`parseModel`, `lower.go:222`). Do not mint variants.

**R4 · The endpoint is the CODING endpoint, and that is a checkable fact** (V1; S11.5). `https://api.z.ai/api/coding/paas/v4`. Record in code documentation, from the live docs, that the general `/api/paas/v4` is **not interchangeable** and that pointing at it silently spends pay-as-you-go balance instead of the subscription. R11 turns this into a runtime check.

**R5 · The Anthropic-protocol variant is recorded, not wired** (V2). `https://api.z.ai/api/anthropic` is documented for Claude Code and Goose. Sinet's zai lane rides **opencode as OpenAI-compatible** (S03.6). Record V2 as a dated fact with its URL; do not build a second path.

**R6 · Per-model attributes ride the S18.3 data surface, not new ⚙ keys** (S03.6; S18.3). The lane declares `overflow_mode` and `region_model_gate` per model, plus the flat/metered flag. S18.3 makes these one of the **three data-valued settings surfaces with no dotted key**. Set `overflow_mode` from what §1 actually establishes; a model whose `overflow_mode` is `auto-metered` without a proven disable/zero-balance is **rejected at onboarding** (S03.6; 3.10). The coding plan is flat-rate: its rows are flat, and the metered-exception list stays EMPTY (G1 P7).

### B. Credential wiring (S11.5, D2)

**R7 · The key reaches the engine only through the broker** (S11.5; §12). The zai credential is an `engine-cred` auth-profile reference resolved by `internal/broker` and delivered at spawn through the existing `CredInject` seam (`adapters.StartRequest.CredInject`, `json:"-"`). The worker record holds a **named reference**, never material (S11.5: *"Every worker holds only auth-profile references… never raw secrets"*).

**R8 · The secret appears nowhere else, and that is tested as a property** (D2; §61; §10 credential hygiene). Never in: the sandbox, logs, `run_events`, the DB, park records, fixtures, test output, the compiled config on disk, or the instance-identity key. §61 records that `Manager.Acquire` folds a **digest** of the injected env — *"no raw env or secret ever appears in the key"*. Extend that guarantee to the provider entry's credential field: the config the engine receives may carry the resolved value, but nothing Sinet persists or prints may. Prove it by scanning every persisted artifact and every emitted event for the fixture secret.

**R9 · No key exists yet, and the design must not require one** (STATE LN-2 row; LN-CEREMONY). Every path is exercised with a **fixture/sentinel** credential against the loopback fake provider. A missing zai credential is an honest, named, surfaced state — the lane reports "not commissioned" — never a panic, never a silent unauthenticated call, never a fake success.

**R10 · Rotation restarts, staleness never survives** (§61(a), D3 drain finding). A rotated credential must reach a **fresh** serve instance, never a stale one. That guarantee already exists via the env digest; assert it holds for the zai path specifically.

### C. Limit events (S10.5, P-T08-2)

**R11 · `1113` gains the endpoint self-check S10.5 names** (S10.5 Class 3 row: *"Z.AI 1113 (after endpoint self-check)"*; V1; V12). Today the classifier maps `1113 → Class 3` **unconditionally** (`limits.go:223-224`). But `1113` is *"Insufficient balance or no resource package"* — on a coding-plan account that is the documented symptom of being pointed at the **wrong endpoint** (the general `/api/paas/v4`, where the subscription does not apply and the pay-as-you-go balance is $0). Parking a run on a probe schedule forever because the endpoint is misconfigured is exactly the P-T08-2 failure class.
Required: `LimitSignal` gains an input carrying whether the lane's configured endpoint was verified to be the coding endpoint. `Classify` stays **pure and total** (`limits.go` doc comment) — the check is an input, not an I/O call inside the classifier.
- endpoint verified as the coding endpoint + `1113` ⇒ **Class 3**, park with the probe schedule (unchanged behavior).
- endpoint NOT the coding endpoint + `1113` ⇒ **NOT a limit event**: a surfaced configuration defect with a plain-language reason naming the wrong endpoint. It must not retry, must not park on a probe schedule, and must not freeze the lane.
- Ordering is preserved: Class 4 is still checked first (l.145-155).

**R12 · The adapter extracts the Z.AI signal from the opencode wire** (S03.1 *"Adapters forward the raw signals as data"*; S10.5). Nothing today builds a `LimitSignal` from an opencode/Z.AI error body. `parse.go` has the error surface (`engineErrorInfo`, `describeEngineError` at l.589-606; `KindRateLimit` emission at l.312). Required: parse the verbatim V12 body shape — `{"error":{"code":"<string>","message":"..."}}`, **`code` is a string** — into `LimitSignal{Lane:"zai", ErrorCode, HTTPStatus, ResetAt, BodyText}`, and extract `next_flush_time` from the 1308/1310 message into `ResetAt`. Forward-tolerant parsing is a MUST (S03.1): an unknown code is logged and forwarded as data, **never fatal, never dropped, never minted as a new platform kind** (§10).

**R13 · Unrecognised Z.AI codes fail in the SAFE direction** (S10.5; P-T08-2). A zai error the taxonomy does not name must never land Class 1 (a retry storm against a depleted plan) and never Class 4 (freezing a healthy lane). Pin this as a **property over the whole observed code space**, not one example: for every zai `429` code with no explicit rule, the verdict is a park, and for every zai `401`/`403`, the verdict is Class 4. `1211` ("Unknown Model") is a 400 and is **not a limit event** — it is the P-T17-3 model-drift symptom (R19), and must be surfaced as that, not parked.

### D. Metering and pricing (S10.1, S10.3, S10.4, S03.7)

**R14 · The token row is tier-1; the plan-unit gauge is tier-3. Do not conflate them.** This is the packet's easiest mistake.
- S10.1 tier **1** = *"Measured per-call usage from the substrate stream/DB — both v0 subscription lanes and local, the normal case; exact on every surface, confirmed live incl. per-message cache detail."* Z.AI's per-request `usage` (V10: `prompt_tokens`, `completion_tokens`, `prompt_tokens_details.cached_tokens`, `total_tokens`) is measured ⇒ **the ledger row's token usage is tier 1.**
- S10.1 tier **3** = *"Derived plan units"* — the prompt/credit consumption figure, which has no per-request counter. S10.4: *"Z.AI lane: requests-as-prompt-proxy with the documented multipliers applied as data (tier-3). Every gauge reading carries its approximation tier."* ⇒ **the consumption-pressure gauge reading is tier 3**, stamped, and labeled "assumed".

**R15 · The multipliers and the budget denominator are DATA with dates** (S10.4; S18.3; V5/V6). Seed from the §1 live values — peak 1.0× / off-peak 0.5×, peak window Mon–Fri 14:00–18:00 UTC+8, Max 28,000 credits/5 h + 140,000/week, weekly window order-anchored (V7) — as **per-person automation budget** rows on the S18.3 data surface, at a conservative fraction (`budget.background_window_fraction`), **labeled "assumed"**. No multiplier and no quota number is a Go constant. Every gauge reading carries the tier and the label.
**Spec-drift flag (do not resolve silently):** S10.1's parenthetical *"documented peak 3× / off-peak 2×"* and S10.4's *"GLM Coding Max ~1,600 prompts/5 h shape"* are **stale as of 2026-08-23**. Both sections state these are applied *as data*, so the live values bind and no amendment is mechanically required — but the coordinator must see this (see §11, C-1/C-2).

**R16 · Price from metered `zai` rows; never from a $0 coding-plan row** (S10.3, verbatim: *"Never price a flat-rate lane's usage from a $0 flat-rate row… Z.AI usage prices API-equivalent from the metered per-token `zai` rows, never the zero-rate coding-plan rows, which would render every receipt $0"*). The price store already defends this from the inside: `pricestore.go:287-318` refuses a zero or all-zero row with an explicit UNPRICED rationale. LN-2 must not weaken that guard and must not add a $0 zai row to make receipts look complete. With no metered `zai` row loaded, zai rows price **UNPRICED (tier 5)** — the honest state (S10.1: *"No row silently prices to $0"*).

**R17 · Never meter dollars from opencode** (S03.7; S10.3, both verbatim). opencode prices `zai-coding-plan` at `$0` (flat-rate). Its `.cost` is a cross-check at most, never billing truth, and must never reach `priced_cost`. Assert this with a fixture whose opencode `.cost` is non-zero and non-$0-ish: the ledger must ignore it entirely.
*Reconciliation key:* S03.7 names `request_id` as the key to the operator dashboard. Carry it on the row so the `TBD-OPERATOR(Z.AI dashboard prompt-unit calibration)` recipe has something to reconcile against when it runs.

### E. Effort ladder (S10.6, S03.7)

**R18 · `thinking:{type:disabled}` is the lane's Eco/Balanced lever** (S03.7 bullet 3; S10.6 verbatim: *"The Z.AI `thinking:{type:disabled}` lever (~50× token reduction on trivial work) is an Eco/Balanced rung on that lane"*). Wire it as an effort-mode-driven, per-turn request shape on the zai lane. V8 confirms the parameter is still supported with the exact shape `{"type":"enabled"|"disabled","clear_thinking":bool}` and defaults to `enabled`. Eco ⇒ disabled. The mode must be a **disclosed state**, not a log line (S10.6: *"A mode change is a visible state, not a log line"*).
**Do NOT wire `reasoning_effort` (V9).** It is real and newly verified, but no named section sanctions it → OQ-4. Record it as a dated fact; do not invent behavior around it.

### F. Watch and drift (S14.6, S03.6, S03.7)

**R19 · Complete the two canaries for the zai lane** (S14.6; P-T17-1; P-T17-3). **Corrected against code:** `PaidLanes()` already returns `zai`, and `canary_auth.go`, `canary_behavioral.go`, `canary_models.go` already **iterate `PaidLanes()`** — so zai is structurally in scope today and the job is not "register a row" but "make the zai leg real and assert it". What is owed: the zai probe's endpoint/credential wiring, its classifier hand-off, and the assertions. The real-request legs stay **DISARMED** behind `SINET_CANARY_ARM` (`canary.go:88-115`) — **do not arm them**; the armed leg belongs to LN-CEREMONY. Results ride `CanaryResult`/`CanaryPayload` (`canary.go:171-255`), which already carry `Action`, `LimitClass`, `Observed`, and `verified_on` — the exact fields these two canaries need.
- **Auth canary** (P-T17-1): scheduled cheap real request at ⚙ `canary.auth_interval` (default daily). Error-class discrimination runs **through the five-class classifier** (S14.6). An auth-shaped failure classifies *policy-revocation-suspected* ⇒ **lane freeze + flag-now**, **never** an infinite retry-park (S03.6; S10.5 Class 4). Canary consumption is metered like everything (D4) and itemized on the lane owner's meters. **At this packet it runs against the fake provider at $0** — the live leg belongs to LN-CEREMONY.
- **Model-list-diff canary** (P-T17-3): the account's **observed** model list diffed against config, on the same schedule, `verified-on` dated. This is the canary that would have caught GLM-5.2 → GLM-5.3 (§1 V3). Per §1, `/models` on the coding endpoint is **undocumented**: the canary must tolerate its absence and report that honestly rather than fail or fabricate.
- Results ride the S14.1 event rows `canary.result` (canary kind + lane + pass/fail/delta) and `drift.finding`.

**R20 · Behavioral-eval-only, and say why** (S03.7 bullet 2; S14.6). Z.AI exposes no logprobs (V11 re-confirms: undocumented). The zai lane registers a **behavioral canary** at ⚙ `canary.behavioral_interval` and **no logprob canary** — that one is LOCAL LANE ONLY. The absence must be explicit and reasoned in code, not an accidental omission.
Also re-verify the seed watch rows against §1: `t1-zai-devpack` → `https://docs.z.ai/devpack/overview` is **live and correct**; `t1-zai-release-notes` → `https://docs.z.ai/release-notes/new-released` was **not verifiable this session** (the `devpack/release-notes` path 404s). Verify or repoint it, dated; a dead watch MUST announce itself (S14.6 decay posture, ⚙ `watchlist.fetch_fail_streak`).

### G. Routing (S08.8)

**R21 · The zai seat joins the duty map, under coverage** (S08.8 step 3, verbatim: *"routing selects only models the owner declared flat-rate, or the local tier"*). Add zai seats (model + lane + window) to the duty-map defaults as ⚙/config data, gated by `Coverage.laneCovered("zai")`. An owner without zai coverage sees the existing 2.7 subscription-gap advice, unchanged.

**R22 · Among flat lanes, pressure — never dollars** (S08.8; S10.2, both verbatim: *"Dollar-based routing between flat-rate lanes is a D5 violation and NEVER done"*). With `anthropic` and `zai` both flat-rate and live, this rule is **reachable for the first time**. Selection between them uses the consumption-pressure gauge, effort mode, and task classification. Pin it as a test that a price difference between the lanes changes **nothing** about selection. Every decision emits `routing.decided {cause, score, signals, worker+version, model, lane, effort, plain_reason}` (S08.8).

**R23 · Revisit the consumer-class split and the "one agentic lane" posture HONESTLY** (CONVENTIONS §26; §288; `routing.go:552-558`). The comment reads *"the local ENGINE lane (class (a)…) has NO v0 consumer in this cut — no second adapter exists"*. Since LN-1, **the second adapter exists** — that clause is now false and must be corrected, dated, in **three** places that repeat it: `routing.go:553`, `internal/local/local.go:13`, `internal/local/gpubroker.go:45`, plus CONVENTIONS §26.
**The "one agentic lane" posture is not code.** Verified: its only occurrence in the Go tree is a comment at `internal/worker/ceiling.go:23`. It has always been enforced by **wiring** — `shell.go:583-588` registers exactly one adapter. R30 is therefore the literal act of changing that posture, and it must be stated as such rather than left implicit.
State the post-LN-2 truth precisely, and change nothing else:
- **What changes:** `zai` becomes a real, dispatchable **paid agentic engine lane on the opencode substrate**. There are now **two** paid agentic lanes, because two adapters are registered. R22's flat-lane pressure rule goes live. `ceiling.go:23`'s comment is corrected.
- **What stays:** the **local** engine lane (class (a)) still has **no v0 consumer** — not because no adapter exists, but because **no local provider entry is commissioned** (that is a separate lane commissioning, not this packet). So `DutyUtility`/mechanical still degrades to the paid execution seat — with the corrected reason. Class (b) — the control plane calling S12.4 duty aliases directly — is untouched.
- `modelCovered` (`routing.go:625-628`) currently ignores the model. With zai serving three distinct ids, note the seam; **tightening it to per-model coverage consumes the R19 observed list and is not this packet's** unless the coordinator says otherwise (S08.8 defers observed-list diffing to the S03.6 watch).

### H. The shared `adapters.Answer` member (LN-1 §61 carry)

**R24 · Add a first-class approve/reject member to `adapters.Answer`** (S03.4; §61). LN-1 was forced into a documented `sinet.decision` envelope smuggled through `UpdatedInput` because the shared type had no reject. §61 names the first-class member the honest fix and flags it here. This is a **SHARED-package change** — design it against **both** substrates.
- **claudecli semantics are preserved exactly.** Today `UpdatedInput` is a **genuine tool-input substitution**: `Resume` → `writeAnswer` stages `{"updatedInput":…}` under `<ctlDir>/answers/` (`hook.go:276-289`), and the PreToolUse hook reads it back and emits `permissionDecision:"allow"` + `updatedInput` (`hook.go:116-125`). The new member's **zero value must mean approve**, so every existing construction site keeps its meaning with no edit and no behavior change.
- **The Claude lane needs a deny path.** `hook.go:116-125`'s answered branch only ever writes `"allow"`. A first-class reject must produce `permissionDecision:"deny"` with the reason — the same mechanism S03.5's serialize-by-deny already relies on (*"deny all calls of a parallel gated turn with reason"*), so this is wiring an existing engine capability, not inventing one. `writeAnswer`/`readAnswer` and the `hookDecision` wire struct (`hook.go:59-68`) carry the decision alongside `updatedInput`.
- Reject carries a reason/message — opencode's `permission.reply` takes `{reply, message}` and preserves reject-with-feedback `CorrectedError` (S03.4).
- **The opencode `sinet.decision` envelope is RETIRED** in favor of the member. Concretely: `decisionKey`/`decisionApprove`/`decisionReject`/`decisionUnencodable` (`opencode.go:75-84`), the envelope built in `RejectAnswer` (`opencode.go:372-384`), and `decodeDecision`'s anonymous `{Decision \`json:"sinet.decision"\`; Message}` struct (`opencode.go:387-422`) all go. `ApproveAnswer`/`RejectAnswer` build the member directly; `decodeDecision` reads it.
- Two LN-1 guarantees must survive verbatim: the client's `reply` accepts only `once`/`reject` — **`always` stays unsendable by construction** (S03.4: *"Never send `always` through opencode"*); and a RAW replacement input on the opencode lane still refuses with `ErrUpdatedInputUnsupported`, because that substrate has **no input-substitution channel** and silently dropping a human's edit would execute the parked call unreviewed (S03.4).

**R25 · The envelope-refusal guarantee is not lost in the migration** (§61; LN-1 drain D4). LN-1's decision envelope *"refuses extra members (silent edit-drop)"*. Whatever that protected must remain protected by the typed member: an answer that carries a decision the substrate cannot express fails **loudly and fail-closed** (the §12 precedent, and LN-1 drain D7's `RejectAnswer` marshal fallback).

**R26 · Both substrates' existing suites stay green.** The claudecli adapter's gate/answer tests and the opencode adapter's answer tests must pass unmodified except where §8 sanctions an edit by name.

### I. Surfacing (S10.10, S15 read-only)

**R27 · Receipts and meters show the lane honestly** (S10.10; S10.1). Zai lines carry the approximation tier, the "assumed" label on gauge readings (R14/R15), UNPRICED where no metered row exists (R16), and park history where a limit event parked the run. Currency is **api-equivalent** for this flat lane (S10.2/S10.10); it flips visibly only for a metered-flagged run, which cannot happen at v0.

**R28 · `web/src` is UNTOUCHED — but flag any wire-shape change.** Frontend work runs a different pipeline. If any JSON the SPA consumes gains or changes a member — `metering.Receipt`, `LineItem`, the routing decision payload, a canary/event payload — **report it as a flagged fixture-contract change in the executor's final message**. Do not edit `web/src`, do not edit its fixtures.

**R29 · No new ⚙ key.** See §4.

### J. The two steps that actually make the lane exist (found in code, not in the packet note)

**R30 · Register the opencode adapter in the control plane** (S03.2; CONVENTIONS §10). Verified: `internal/shell/shell.go:583-588` registers **exactly one** adapter (`adapters.SubstrateClaudeCLI → &claudecli.Adapter{…}`), and the opencode `Adapter` type has **zero production call sites** in the whole tree — it lives only in its own tests. **Without this, nothing in §3 can dispatch.** Add `adapters.SubstrateOpencode → &opencode.Adapter{…}` to that map, supplying `Instances` (the dev-posture `Manager`), the per-user `Providers` resolver (R2), and `Root`.
This is the literal act that ends the "one agentic lane" posture (R23), so it must be deliberate and stated, not a side effect. Two guardrails: registering the adapter must not change **any** claudecli dispatch (assert the existing stage tests are untouched in behavior), and with no zai provider entry configured the second adapter must be inert — present but never selected (R9's "not commissioned" state).

**R31 · The `zai` conformance-registry row lands here** (S14.5; CONVENTIONS §32). `internal/conformance/registry.go` says so in its own Notes, verbatim: *"The zai lane is OMITTED"* (l.192) and the lane's row plus its canary rows **complete at LN-2** (l.204, l.227). Add the zai lane row with honest Notes distinguishing what it proves (the lane, config-only, atop the LN-1 substrate) from what it does not (no live provider call until the ceremony). Handles are prose-prefixed by tier (`tier F:` / `tier R:` / `tier L:`) per the existing nine opencode handles at `registry.go:213-221`. **The row-set-pinning tests move** — that edit is sanctioned by name in §8.

---

## 4. ⚙ settings — consumed by registry name, and the 118-key wall

**The S18 index is test-pinned at 118 keys / 33 domains in FIVE assertions across three files** — `internal/settings/read_test.go:32`, `internal/settings/settings_test.go:44` (keys), `:79` (domains), `:450` (schema `x-sinet` leaves), `internal/settings/uischema_test.go:262` (domains) — plus per-owner and per-domain tallies at `settings_test.go:56-78` (S03 owns 3, S10 owns 9; domains `adapter 3 · pressure 2 · budget 1 · limit 3 · canary 2`). **A new key needs an S00.9 amendment plus an S18 sweep and is a COORDINATOR FLAG, not a silent addition.** Do not add one; do not edit those five assertions or the tallies.

**Consumed (existing keys, by dotted key, never a constant):**

| Key | Owner | Used here for |
|---|---|---|
| `limit.retry_cap` | S10.5 | Class-1 retry on 1302/1305 (already loaded by `LoadLimitConfig`) |
| `limit.retry_budget_ratio` | S10.5 | per-lane retry budget |
| `limit.probe_interval_max` | S10.5 | Class-3 probe schedule on 1113 |
| `pressure.cache_read_weight` | S10.4 | gauge weighting; V10 confirms `prompt_tokens_details.cached_tokens` is present, so cache reads are real on this lane |
| `pressure.bg_admit_stop` | S10.4 | background admission stop (0.7) |
| `budget.background_window_fraction` | S10.4 | the conservative fraction of the V5 credit window (R15) |
| `meter.value_divergence_alarm` | S10.1 | tier-1 vs tier-2 divergence — note it stays non-comparable while UNPRICED |
| `canary.auth_interval` | S14.6 | the zai auth canary cadence (R19) |
| `canary.behavioral_interval` | S14.6 | the zai behavioral canary — the lane's ONLY drift detection (R20) |
| `watchlist.fetch_fail_streak` | S14.6 | dead-watch announcement for the zai watch rows (R20) |

**Referenced as named seams, honestly NOT consumed** (state the reason in a doc comment; do not fake a consumer — §61's precedent):
`sandbox.model_egress_tls_terminate` (S11.5 — the injection proxy does not exist; OQ-1) · `adapter.engine_ceiling_backstop_mult` (no engine budget belt at opencode 1.18.3, §61) · `adapter.parallel_gate_fallback` (a defer trap this substrate does not have) · `adapter.claude.cleanup_period_days` (Claude-lane retention) · `freshness.hold_vs_park` (S10.7 cache-window resume preference, owned by the scheduler).

**Where the lane's own numbers live instead — the S18.3 data surfaces (no dotted key by design):**
1. **Price table** (S10.3) — the metered `zai` per-token rows (R16).
2. **Per-person automation budgets** (S10.4) — the V5/V6/V7 credit window and multipliers, labeled "assumed" (R15).
3. **Per-model attributes** (S10.2 flag; S03.6 pair) — flat/metered flag, `overflow_mode`, `region_model_gate` (R6).
Plus the provider entry itself (endpoint + model ids), which is engine lowering config, not a registry key.

---

## 5. Seams to respect, and the stubs for later phases

- **`Instances`** (`opencode.go:139-142`) — the per-user serve endpoint seam. LN-2 consumes it. The D6 `sinet-engine@<user>` unit replaces the *provider*, not the adapter; that unit's `ExecStart` stays a DRAFT.
- **`Adapter.Providers`** — the lane-config seam (R1/R2). This is the seam S03.6 promises; use it, don't route around it.
- **`CredInject` / `Confiner`** (`adapters.StartRequest`, both `json:"-"`) — recompiled every run. A non-nil per-run `Confiner` on this substrate is permanently `ErrConfinementUnsupported` (§61(b)) — correct semantics, not an interim guard. Do not "fix" it.
- **Credential-injection proxy** (S11.5) — **deferred stub**. Record the binding wire facts so the later packet has them without re-deriving: inject **only** on `/api/coding/paas/v4/chat/completions`, header `Authorization: Bearer`, pass every other request untouched; per-process CA trust only (`NODE_EXTRA_CA_CERTS`), the system trust store never touched; the pin-regression canary **P-T01-2** asserts per engine version that a trusted-CA terminating proxy on the model path still yields 200, and on regression ⚙ `sandbox.model_egress_tls_terminate = false` for that lane falls back to pattern-2 scoped-egress. P2-S3 already demonstrated a **Z.AI 401→200 purely from proxy-side substitution**. Z.AI has **no** `x-ratelimit-*` headers (S03.7, S10.4), so the proxy's second purpose — the `anthropic-ratelimit-unified-*` harvest — has **no zai analogue**: this lane's limit state rides error-code bodies (R12).
- **Per-user jail** (S11.1) — **deferred stub** (OQ-2). Record what is owed: bwrap + empty netns around the whole per-user serve, scoped to that user's workspaces and model-egress; opencode's own `permission`/`allowRead`/`denyRead` is an **inner soft layer only, never the boundary**; the cross-user boundary is preserved by per-user server + per-user XDG + per-user jail; per-session workspaces are S11.3.
- **Dashboard calibration** — `TBD-OPERATOR(Z.AI dashboard prompt-unit calibration)` (S03.7, S10.1). Leave `request_id` on the row as its reconciliation key (R17). The recipe runs at LN-CEREMONY.
- **Tier L** — `SINET_LIVE_SMOKE=1` is THE tier-L env; **no second env name is minted**. It stays structurally unreachable until the key ceremony and prints its named skip.

---

## 6. Files expected to change

**Shared adapter package (SHARED — design against both substrates):**
- `internal/adapters/adapters.go` — the `Answer` decision member (R24/R25)
- `internal/adapters/claudecli/*.go` — consume the member; `UpdatedInput` semantics preserved verbatim (R24)

**The lane:**
- `internal/adapters/opencode/opencode.go` — per-user provider resolution (R2); retire `sinet.decision` (R24)
- `internal/adapters/opencode/lower.go` — provider entry compilation, the `thinking` lever (R18)
- `internal/adapters/opencode/parse.go` — Z.AI error/limit-signal extraction (R12)
- `internal/adapters/opencode/` — a new lane-config file (e.g. `lane_zai.go`) for the seed DATA + its `verified-on` dates
- `internal/broker/` — the `engine-cred` zai auth-profile path (R7–R10)

**Accounting / scheduling / watch / routing:**
- `internal/scheduler/limits.go` — endpoint self-check input + classification (R11/R13)
- `internal/metering/` — tier split, gauge multipliers as data, zai price-row semantics (R14–R17, R27)
- `internal/watchlist/canary.go`, `internal/watchlist/seed.go` — auth + model-list-diff canary rows; watch-row URL re-verification (R19/R20)
- `internal/worker/routing.go` — zai seat in `DefaultDutyMap()` (currently all-anthropic, `routing.go:126-135`), flat-lane pressure selection, the corrected consumer-class comment (R21–R23)
- `internal/worker/ceiling.go` — zai ceiling defaults if the duty map demands them; the `ceiling.go:23` "one agentic lane" comment corrected (R21/R23)
- `internal/local/local.go:13`, `internal/local/gpubroker.go:45` — **comment corrections only**, same stale "no second adapter exists" clause (R23)
- `internal/shell/shell.go` — register the opencode adapter (R30) — **the step that makes the lane exist**
- `internal/conformance/registry.go` — the `zai` lane row (R31)

**Docs:** `P3/CONVENTIONS.md` — a new dated section for this packet, **and** a dated correction to §26's "no second adapter exists" clause (R23). Append-mostly: never silently rewrite §61.

**Never:** `web/src` (R28) · `Docs/` · `Spec/` · `Research/` · `P3/STATE.md` · the four settings assertions (R29) · any adopted component's source (adopt-don't-fork) · `components.lock` (no new dependency is expected; if one becomes necessary, **stop and flag** — a lock entry is an S16.4 event).

---

## 7. Acceptance checklist (the headline, decomposed)

Each line is mechanically checkable by someone who reads only this brief and the cited sections.

1. **Provider entry** `zai-coding-plan` exists as configuration with endpoint `https://api.z.ai/api/coding/paas/v4` and model ids `glm-5.3` / `glm-5-turbo` / `glm-4.7`, each carrying a `verified-on` date; a test flows a *different* endpoint + model set end-to-end unchanged, proving they are DATA. **No endpoint or model-id string is a constant in a non-test file.**
2. **Per-user resolution**: two users resolve different provider entries; changing one user's entry restarts only that user's serve; the single-value default case still works (no LN-1 test regresses).
3. **Broker credential**: the zai `engine-cred` resolves through `internal/broker` and reaches the engine via `CredInject`; a run with **no** zai credential reports a named "not commissioned" state — no panic, no unauthenticated call, no fake success.
4. **Secret containment (property test)**: the fixture secret appears in **zero** of — logs, `run_events`, DB rows, park records, the instance-identity key, test output, committed fixtures. Rotation reaches a fresh instance, never a stale one.
5. **Classifier fixtures**: 1302 → Class 1 · 1305 → Class 1 · 1308 + `next_flush_time` → Class 2 with the parsed resume time · 1310 + `next_flush_time` → Class 2 · 1113 **with** verified coding endpoint → Class 3 probe-park · **1113 with an unverified/wrong endpoint → surfaced configuration defect, NOT a limit event, no retry, no probe-park, no lane freeze** · 1000/1001/1003 (401) and 1220 (403) → Class 4 lane freeze.
6. **Class-4-first ordering survives**: an auth signal can never reach a retry-park, asserted as a property over the code space, not one example (P-T08-2).
7. **Safe-direction property**: every zai `429` code with no explicit rule parks (never Class 1, never Class 4); `1211` (400, "Unknown Model") is not a limit event and surfaces as model-drift.
8. **Wire extraction**: the verbatim body `{"error":{"code":"1308","message":"Usage limit reached for `N` `unit`. Your limit will reset at `<t>`"}}` — with `code` as a **string** — produces `LimitSignal{Lane:"zai", ErrorCode:"1308", ResetAt:<t>}`. An unknown code is logged, forwarded as data, and is **not fatal**.
9. **Tier discipline**: a zai ledger row with measured tokens stamps **tier 1**; the zai consumption-pressure gauge reading stamps **tier 3** and is labeled "assumed". A test asserts both on the same run.
10. **Never $0 from a flat row**: with no metered `zai` price row, zai usage renders **UNPRICED (tier 5)** on the receipt; the `pricestore` zero-row refusal is intact and untouched; no $0 zai row is added anywhere.
11. **Never dollars from opencode**: a fixture with a non-zero opencode `.cost` produces a receipt whose `priced_cost` ignores it entirely.
12. **Multipliers/quota as data**: peak 1.0× / off-peak 0.5×, the Mon–Fri 14:00–18:00 UTC+8 window, and the 28,000/5 h + 140,000/week Max figures are configuration rows labeled "assumed", not constants; changing them changes gauge output with no code edit.
13. **Eco lever**: effort mode Eco produces a zai request carrying `thinking:{"type":"disabled"}`; Smart does not; the active mode is a **disclosed state** on the task surface, not only a log line. `reasoning_effort` is **not** wired.
14. **Auth canary** registered for lane `zai` at ⚙ `canary.auth_interval`, discriminating through the five-class classifier; an auth-shaped failure yields lane freeze + flag-now and **never** a retry-park; the canary's own consumption is metered and itemized.
15. **Model-list-diff canary** registered for lane `zai` on the same schedule, `verified-on` dated, tolerating an absent `/models` endpoint by reporting it honestly.
16. **Behavioral canary** registered for lane `zai`; **no logprob canary exists for zai**, with the reason explicit in code (S03.7 / V11).
17. **Routing**: a zai seat resolves under coverage; an owner without zai coverage still gets the 2.7 gap advice unchanged; `routing.decided` carries lane `zai` with a plain-language reason.
18. **Flat-lane pressure rule**: a price difference between `anthropic` and `zai` changes selection **not at all**; selection responds to the pressure gauge. (D5 violation test.)
19. **Consumer-class split corrected**: `routing.go`'s "no second adapter exists" clause is corrected and dated; `DutyUtility` still degrades to the paid seat with the **corrected** reason (no local provider entry commissioned); class (b) untouched; CONVENTIONS §26 carries the dated correction.
20. **`adapters.Answer` member**: approve/reject is a first-class typed member whose **zero value means approve**; every pre-existing claudecli construction site compiles and behaves identically; the opencode `sinet.decision` envelope is gone; `always` remains unsendable; `ErrUpdatedInputUnsupported` still refuses a raw replacement input on opencode; an inexpressible decision fails loudly and fail-closed.
21. **Receipts**: zai lines carry tier, "assumed" labels, UNPRICED where applicable, and park history; currency is api-equivalent.
22. **⚙ discipline**: `go test ./internal/settings/...` green with **118 keys / 33 domains unchanged**; the five pin assertions and the owner/domain tallies unedited; every ⚙ read by dotted key; non-consumed keys documented with reasons, never faked.
22a. **Adapter registered**: `internal/shell` registers `adapters.SubstrateOpencode` alongside `SubstrateClaudeCLI`; with no zai provider entry configured the second adapter is present but never selected; no claudecli dispatch behavior changes.
22b. **Conformance row**: a `zai` lane row exists in `internal/conformance/registry.go` with tier-prefixed handles and Notes that distinguish the lane from the substrate; the row-set pins move to the exact new counts.
23. **$0 proof**: no test, fixture, or code path performs a live provider call; tier L prints `SANCTIONED SKIP` (§10); no credential material is committed.
24. **Battery**: `gofmt`-clean, `go vet`-clean, `go build ./...` green, `go test -p 1 ./...` green, `-race` clean on the touched packages, `go run ./tools/lockgate` green with **no new lock entry**.
25. **`web/src` untouched**; any SPA-consumed wire-shape change is reported as a flagged fixture-contract note.

---

## 8. CONVENTIONS constraints binding this packet

- **§5** — commit subject `P3-LN-2: <summary> (S## refs)`; trailer `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`; **stage files explicitly, never `git add -A`**; packets never push, never force-push, never edit `Docs/`/`Spec/`/`Research/`/`P3/STATE.md`, never read or stage `*-api-key.txt`.
- **§2** — stdlib-first; `gofmt`/`vet` clean; **no ⚙ value as a hardcoded constant**; wrap errors with `%w`; no research narration in comments.
- **§3 + Amendment A** — tests-first: materialize §9's specs as tests, run them, confirm they FAIL, **commit them red** (declare the red window in the commit message), then implement until green. `go build ./...` stays green throughout. Batteries run **`-p 1`** (serial — standing operator directive after the RW-14B runaway).
- **§4** — no new dependency without a `components.lock` entry; exact pins only. **None is expected here** — if one becomes necessary, stop and flag.
- **§10** — substrate names are package constants (`opencode`); lane names `anthropic`/`zai`/`local`; unknown engine frames logged and skipped, **never fatal, never minted as new platform kinds**; conformance tiers F/R/L with `SANCTIONED SKIP (CONVENTIONS §10)` as the one allowed skip class; suites assert **behavior, never docs**; pin↔installed deltas reported loudly, never silently retargeted; credential hygiene — refs are paths, never material.
- **§11** — Class 4 checked FIRST; per-lane fixtures live in `limits_test.go`; `MeteredExceptions` EMPTY; the price table ships EMPTY ⇒ UNPRICED; dollar-ranked choice between flat lanes is never coded.
- **§12** — `sandbox → adapters` is the **only** import direction; no adapter imports `sandbox`; broker decision path holds zero secrets; credentials are audience-bound.
- **§61** — the opencode substrate's recorded truths bind and must not be silently reversed: startup-bound config restarts the instance; the identity digest excludes the S02.4e fingerprint and the per-message model, and folds the injected env; Basic-auth username **must** be `opencode`; `task` deny is structural and `ErrNativeSpawnTool` refuses re-admission; `always` unsendable; `POST /global/upgrade` unreachable; resume **diffs**, never assumes; nothing claims SSE replay; `session.idle` honored only after `busy`; `OutcomeDiedAtGate` and `GateFallback` stay structurally unset.
- **Import discipline** — `internal/adapters/opencode` imports `internal/adapters` + stdlib only.

**Test edits sanctioned BY NAME** (the executor may otherwise not modify any pre-existing test file):
- `internal/scheduler/limits_test.go` — **add** Z.AI fixtures (endpoint self-check, the safe-direction property, auth codes). The five existing zai cases at lines 28-38 may be extended but **not weakened or deleted**.
- `internal/watchlist/*_test.go` and `internal/conformance/*_test.go` — **update the row-count pins only**, to the exact new counts, for the rows this packet adds (§61 moved the conformance row set 12→13 and the engine_bump set 6→7; R31 moves them again). Pins may move to a new exact number; they may **not** be relaxed into an inequality or deleted.
- `internal/stage/*_test.go` — **only** if registering the second adapter (R30) mechanically forces a fixture update. Any behavior change to a claudecli dispatch assertion is a finding, not a fixture update.
- `internal/adapters/claudecli/*_test.go` and `internal/adapters/opencode/*_test.go` — **only** the minimum edits the `Answer` member migration mechanically forces (R24). Any assertion weakened rather than mechanically migrated is a finding.
- **NOT sanctioned:** `internal/settings/read_test.go`, `internal/settings/settings_test.go`, `internal/settings/uischema_test.go` — the 118/33 pins stand.

---

## 9. Acceptance-test specifications

**Status: SPEC-ONLY (not committed red) — and the reason, honestly, per test class.** Amendment A asks for red tests "where the current code surface allows."

- **Most specs need a symbol that does not exist yet** and cannot compile: the per-user provider resolver (R2), the endpoint-verification input on `LimitSignal` (R11), the Z.AI signal extractor (R12), the tier-3 gauge stamp (R14), and the `Answer` decision member (R24). Committing tests against invented names would pre-commit this packet's type surface beyond Amendment A's sanction and constrain the executor's design — the same rationale the coordinator accepted at LN-1.
- **Two specs would compile today and genuinely fail** — spec 33 (`TestOpenCodeAdapterRegistered`: `shell.go:583-588` registers one adapter) and spec 35 (`TestZAILaneRowRegistered`: `registry.go:192` says the zai row is omitted). They are named here as the closest red candidates. They are **not** committed because both assert the *shape* of a registration whose constructor arguments R2/R30 have not yet fixed, so a red test written now would encode a guess about the seam and then have to be edited by the executor — the one thing Amendment A forbids. **If the coordinator wants a red commit, these two are the ones to ask for**, after R2's resolver signature is settled.
- **One spec that would compile is deliberately excluded**: a bare `Classify` call on an unknown zai code asserts behavior **OQ-3 has not settled**; pre-committing it would bake in an unratified answer.

Specs below are written to be mechanically translatable: exact names, setup, and assertions.

**Package `internal/adapters/opencode`**
1. `TestZAIProviderEntryIsData` — build the lane config from a fixture carrying a **non-default** endpoint and model set; assert the compiled `engineConfig.Provider` JSON contains exactly those values. Assert by source scan that no non-test `.go` file in the package contains the literal `api.z.ai` or `glm-5.3`.
2. `TestZAIProviderResolvesPerUser` — users `alice` and `bob` with different entries; assert each lowering carries its own; assert the two `Manager.Acquire` identity digests differ.
3. `TestZAIProviderChangeRestartsOnlyThatUsersServe` — change alice's entry; assert alice's instance restarts and bob's does not.
4. `TestZAICredentialNeverLeaves` — run a full fixture turn with sentinel secret `SINET-TEST-SECRET-<uuid>`; scan every emitted event payload, every `run_events` row, the park record, the instance identity key, the captured log buffer, and the test's own output: **zero occurrences**.
5. `TestZAIMissingCredentialIsNamedState` — no zai credential configured; assert a named error/state ("not commissioned"), no panic, and that the fake provider received **zero** requests.
6. `TestThinkingDisabledOnEcoOnly` — effort Eco ⇒ the request body carries `"thinking":{"type":"disabled"}`; effort Smart ⇒ it does not. Assert `reasoning_effort` is absent in both.
7. `TestZAIErrorBodyExtractsLimitSignal` — table over the verbatim V12 bodies (string `code`); assert `{Lane, ErrorCode, HTTPStatus, ResetAt}` for 1302/1305/1308/1310/1113/1220; assert 1308/1310 parse `next_flush_time` into `ResetAt`.
8. `TestUnknownZAIErrorCodeIsForwardedNotFatal` — code `"9999"`; assert the session does not crash, the signal is forwarded with `ErrorCode:"9999"`, and one log line records the skip.
9. `TestOpenCodeCostNeverPricesZAI` — fixture whose opencode `.cost` is `1.23`; assert the ledger row's `priced_cost` is unaffected and the row reports UNPRICED.

**Package `internal/scheduler`**
10. `TestClassify1113RequiresEndpointSelfCheck` — (a) `{Lane:"zai", ErrorCode:"1113", HTTPStatus:429, EndpointVerified:true}` ⇒ `ClassDepletionNoSignal` / `ActionParkProbe` with `ProbeIntervalMax` from ⚙. (b) same with `EndpointVerified:false` ⇒ **not** a limit event: no retry action, no park, no freeze, and a `Reason` naming the wrong endpoint.
11. `TestClassifyZAIAuthCodesFreezeLane` — 1000/1001/1003 at HTTP 401 and 1220 at 403 ⇒ `ClassAuthPolicy` / `ActionLaneFreeze`, for both `OnValidCredentials` values.
12. `TestClassifyZAIUnknown429AlwaysParks` — **property** over codes 1300-1399 excluding the named ones, at HTTP 429: assert the class is 2 or 3 and the action is a park; assert it is never `ActionRetryInPlace` and never `ActionLaneFreeze`.
13. `TestClassify1211IsNotALimitEvent` — `{Lane:"zai", ErrorCode:"1211", HTTPStatus:400}` ⇒ `ClassNone`; assert it is surfaced as model-drift input rather than parked.
14. `TestClassifyStaysPureAndTotal` — property: `Classify` performs no I/O and returns a non-zero `Kind` for every input in the fixture space (extends the existing purity posture).

**Package `internal/metering`**
15. `TestZAITokenRowIsTier1AndGaugeIsTier3` — one zai run with measured tokens; assert the ledger line's tier is `TierMeasured` **and** the consumption-pressure gauge reading for `(user, zai)` stamps `TierDerived` with an "assumed" label. Both on the same run.
16. `TestZAIUnpricedWithoutMeteredRow` — empty/coding-plan-only table ⇒ zai line `Unpriced()` true, `TotalPricedUSD == 0`, receipt says UNPRICED.
17. `TestZAIZeroPriceRowRefused` — attempt to store a `{model:"glm-5.3", lane:"zai"}` row with all-zero unit prices ⇒ `ErrBadPriceRow` (pins `pricestore.go:313-318` against this packet).
18. `TestZAIMultipliersAreConfigNotConstants` — change the peak/off-peak multiplier rows; assert the gauge output changes with no code edit; assert both readings carry the "assumed" label.
19. `TestZAIReceiptCarriesRequestIDForReconciliation` — assert `request_id` survives onto the row (S03.7 dashboard key).

**Package `internal/watchlist`**
20. `TestZAIAuthCanaryRegisteredAndFreezesOnAuthShape` — assert a zai auth canary row exists at ⚙ `canary.auth_interval`; drive an auth-shaped failure through it; assert lane freeze + flag-now and **no** retry-park; assert its consumption is metered.
21. `TestZAIModelListCanaryRegisteredAndToleratesAbsence` — assert the row exists; with the fake provider returning 404 for `/models`, assert an honest "unavailable" result with a `verified-on` date, not a failure and not a fabricated list.
22. `TestZAIHasBehavioralCanaryAndNoLogprobCanary` — assert a behavioral row at ⚙ `canary.behavioral_interval` and assert **no** logprob canary row exists for lane `zai`.
23. `TestZAIWatchRowURLsVerified` — assert the seed rows' URLs match the §1 verified set with their dates.

**Package `internal/worker`**
24. `TestZAISeatResolvesUnderCoverage` — coverage includes `zai` ⇒ a zai seat resolves with a plain-language reason; coverage excludes it ⇒ the existing 2.7 gap advice, unchanged.
25. `TestFlatLaneSelectionIgnoresDollars` — two covered flat lanes with a large price difference; assert selection is identical with the prices swapped, and that it tracks the pressure gauge instead (D5).
26. `TestUtilityDutyStillDegradesWithCorrectedReason` — `DutyUtility` still rides the paid execution seat; assert the reason string no longer claims "no second adapter exists" and instead names the absent local provider entry.

**Package `internal/adapters` (shared)**
27. `TestAnswerZeroValueIsApprove` — a zero-valued `Answer` with `UpdatedInput` set means approve-with-edit on claudecli, byte-identical to today's behavior.
28. `TestRejectIsFirstClassOnBothSubstrates` — a reject `Answer` with a reason produces opencode's `{reply:"reject", message:<reason>}`; assert **no** `sinet.decision` envelope appears anywhere.
29. `TestAlwaysStillUnsendable` — **property** over random answers: opencode's `reply` only ever emits `once` or `reject`.
30. `TestRawUpdatedInputStillRefusedOnOpenCode` — `ErrUpdatedInputUnsupported` intact.
31. `TestInexpressibleDecisionFailsClosed` — a decision the substrate cannot express errors loudly; assert it never degrades to an approve. Pin the `RejectAnswer` fail-closed posture that `opencode.go:378-382` currently gets via the `unencodable` sentinel: a marshal failure must never become consent.
32. `TestClaudeHookEmitsDenyOnReject` — a reject `Answer` staged via `writeAnswer` makes the PreToolUse hook emit `permissionDecision:"deny"` with the reason (today `hook.go:116-125` only ever emits `"allow"`); an approve still emits `"allow"` + `updatedInput` byte-identically.

**Package `internal/shell` (R30)**
33. `TestOpenCodeAdapterRegistered` — the control plane's `Adapters` map contains both `adapters.SubstrateClaudeCLI` and `adapters.SubstrateOpencode`.
34. `TestSecondAdapterInertWithoutLaneConfig` — with no zai provider entry configured, a dispatch never selects the opencode adapter and the "not commissioned" state surfaces; assert **zero** claudecli behavior change against the pre-existing stage fixtures.

**Package `internal/conformance` (R31)**
35. `TestZAILaneRowRegistered` — a `zai` lane row exists, its handles are tier-prefixed, and its Notes distinguish the lane from the substrate; the row-set pin equals the exact new count (an inequality is a finding).

---

## 10. Split recommendation (the coordinator decides)

The grounded scope is **larger than one sitting**: 31 requirements across ten packages, including a shared-package type migration that touches both substrates *and* the control-plane adapter registration that ends the one-agentic-lane posture. Recommended split at a clean seam — **2A makes the lane run; 2B makes it accounted for and watched.** 2B consumes only 2A's lane constant and config surface.

**P3-LN-2A — the lane runs** (R1–R13, R18, R24–R26, R28, R29, **R30**)
Provider entry as per-user DATA · broker credential wiring · `thinking` Eco lever · the shared `adapters.Answer` member (incl. the claudecli deny path) and the `sinet.decision` retirement · Z.AI wire signal extraction + the 1113 endpoint self-check + classifier fixtures · **registering the opencode adapter in `internal/shell`**.
*Acceptance headline:* a run dispatches on lane `zai` against the loopback fake provider at $0, its gate round-trip uses the first-class decision member, and its limit signals classify correctly including the endpoint self-check.

**P3-LN-2B — the lane is accounted for and watched** (R14–R17, R19–R23, R27, **R31**)
Tier-1/tier-3 split · multipliers and budget denominators as dated data · metered `zai` price-row semantics · the auth / model-list / behavioral canary legs · routing seat, flat-lane pressure rule, and the honest consumer-class + one-agentic-lane correction · receipts/meters labels · the zai conformance row.
*Acceptance headline:* a zai run's consumption, receipt, routing decision, and drift watch are all honest, tiered, and labeled.

**Why this seam and not another:** R30 is the single change that makes every other requirement observable, so it must sit with the run path; R24's shared-type migration is riskiest and belongs where both substrates' suites are already in scope. 2B touches no adapter code at all.

If the coordinator prefers one packet, the checklist in §7 is already ordered so 2A's items (1–8, 13, 20, 22a, 23–25) can land first.

---

## 11. Spec conflicts, gaps, and open questions — NOT resolved here

**Conflicts / drift (coordinator disposition required; do not resolve silently)**

- **C-1 · S10.1's multiplier numbers are stale.** S10.1 says *"documented peak 3× / off-peak 2× multipliers applied as data"*. Live (V6): off-peak is **50% of the standard rate** — i.e. peak 1.0×, off-peak 0.5×. S10.1 itself mandates these be applied *as data*, so the live values bind and **no amendment is mechanically required**; the parenthetical is a stale provenance note. Recommend: implement the live values as dated data (R15), record the drift in CONVENTIONS, and let the coordinator decide whether S10.1's prose warrants an S00.9 cosmetic entry.
- **C-2 · S10.4's budget-denominator seed is stale.** *"the GLM Coding Max ~1,600 prompts/5 h shape [G1 D1.5]"* — the plan is now denominated in **credits** (V5: Max 28,000/5 h + 140,000/week). Same class as C-1: the seed is data labeled "assumed". Same recommendation. **Note this also changes the *unit*, not just the number** — "prompts" vs "credits" — which the receipts and gauge labels must say honestly.
- **C-3 · The operator's order named "GLM 5.2"; the live plan serves GLM-5.3.** The 2026-08-23 lane-expansion order and the STATE LN-2 row both say GLM 5.2. Live (V3), GLM-5.2 requests are **auto-routed to GLM-5.3**. This is not a conflict to resolve silently: the seed config uses the live ids, and P-T17-3 (R19) makes the account's observed list the authority. Worth one line to the operator at the gate — the lane they asked for is the lane they get, under its current name.
- **C-4 · `routing.go:552-558` asserts a fact that is no longer true.** *"no second adapter exists in this cut"* — LN-1 landed it. R23 corrects it, dated. Flagged because it is a code comment that a future reader would take as truth.
- **C-5 · Coordinator packet-note slip (harmless).** The packet instruction cites "CONVENTIONS §99 package map". **There is no §99** — CONVENTIONS package maps are per-section bullets (§10 for adapters, §11 metering/scheduler, §12 sandbox/broker, §26 local, §61 opencode). No action needed; recorded so the executor does not hunt for it.

**Open questions**

- **OQ-1 · Does the S11.5 credential-injection proxy land here, or split out? (recommendation: SPLIT OUT.)** Determined from S11 + the code, honestly: **the proxy does not exist at all.** CONVENTIONS §12 lists it as a deferred seam and records that srt's `credentials` injection block was deliberately omitted at B1. Building a TLS-terminating proxy with a per-process CA, the path-scoped injection rule, and the P-T01-2 pin-regression canary is a **full packet on its own** and it is host-coupled (it batches with the deferred egress substrate). Recommendation: LN-2 wires the credential through the broker's **existing** `engine-cred` delivery (R7) — which is correct at v0 dev posture, because S11.5's D2 invariant is about the **task sandbox**, and the serve process is not one — and records the exact wire facts (§5) for the later packet. If the coordinator wants the proxy inside LN-2, LN-2 needs a third cut.
- **OQ-2 · Where does the per-user jail composition land? (recommendation: D6 / the host batch, NOT here.)** §61 flagged it as "LN-2/D6 scope". Grounded in S11.1/S11.3: it requires bwrap + an empty netns around a **long-lived** per-user server, per-session workspaces, and the host egress substrate — all of which §12 already defers to the B2/host batch, and all of which are host changes a packet session does not perform. Recommendation: LN-2 records what is owed (§5) and does not compose it. Note the standing watch: opencode issue **#5529** (native OS sandboxing) would reduce this to configuration if it lands (S11 Deferred).
- **OQ-3 · How should Z.AI codes `1311`–`1321` classify?** Newly verified live (V12) and **not in S10.5's signal set**; the docs collapse them to "Various subscription/usage limit violations" at HTTP 429, so their individual semantics are UNVERIFIED. Today they would fall through to Class 3 (park-probe) — a safe direction. Recommendation: pin that safe fallback as the **property** in R13/spec-12 rather than inventing per-code rules, and add the band to the model/limit watch so a future doc expansion is caught. Coordinator confirms whether a Class-2 mapping (when a `next_flush_time` is present) is sanctioned or needs an amendment.
- **OQ-4 · Is `reasoning_effort` in scope?** Newly verified (V9): `max`/`xhigh`/`high`/`medium`/`low`/`minimal`/`none`, GLM-5.2+, **GLM-5.3 restricted to low/high/max**, effective only when `thinking` is enabled. S10.6's effort ladder names *"vendor effort param"* generically but S03.7 names only `thinking:{type:disabled}` for this lane, and no page confirms the **coding** endpoint honors it. Recommendation: **not this packet** — record it as a dated fact; wiring it is a graded rung the ladder does not currently specify, and inventing it would exceed the named sections. Natural home: an S00.9 note when the coding-endpoint behavior is confirmed live at LN-CEREMONY.
- **OQ-5 · Should `modelCovered` tighten to per-model coverage?** It currently ignores the model (`routing.go:625-628`) — defensible while a lane served one model. With zai serving three ids, per-model coverage becomes meaningful, and R19's observed list is exactly its input. S08.8 defers observed-list diffing to the S03.6 watch (B5). Recommendation: leave the seam noted and untightened at LN-2; revisit once the model-list canary has produced observed data.

---

## 12. Coordinator dispositions (2026-08-23, pre-executor — binding for executors and evaluation)

- **SPLIT ACCEPTED: 2A/2B as §10 recommends.** One brief serves both (the B6-2 precedent). 2A = R1–R13, R18, R24–R26, R28, R29, R30 (checklist items 1–8, 13, 20, 22a, 23–25); 2B = R14–R17, R19–R23, R27, R31 (the rest). 2B follows 2A sequentially.
- **C-1/C-2 RESOLVED — the live values bind, per the spec's own text.** S10.1/S10.4 mandate the multipliers and denominators be applied *as data* with "assumed" labels; the stale parentheticals are provenance notes, not bindings. Implement the V5/V6/V7 live values as dated data (R15, 2B). Receipts and gauge labels say **credits**, honestly (the unit changed). ONE cosmetic S00.9 changelog entry (Z.AI plan re-denomination prompts→credits, off-peak 0.5×, prose examples generalized — no ⚙ default/clamp change, no S18 sweep) is QUEUED TO THE GATE BATCH for operator approval; the packet does not block on it.
- **C-3 NOTED FOR THE OPERATOR** (one line at the gate + in the coordinator's report): the ordered "GLM 5.2 lane" is served as GLM-5.3 by the plan itself (5.2 auto-routes); seed config uses the live ids; the observed-list canary (P-T17-3) is the standing authority.
- **C-4 → R23 as written. C-5 acknowledged — coordinator slip, the brief's §-map is correct.**
- **OQ-1 RESOLVED: proxy SPLIT OUT, as recommended.** The broker's existing `engine-cred` delivery is the v0 dev-posture-correct path (S11.5's D2 invariant guards the TASK sandbox; the serve process is not one). The TLS-terminating injection proxy + P-T01-2 pin canary batches with the D6/host ceremony alongside the jail — recorded in the STATE deferred ledger; §5's wire-facts stub is its handoff.
- **OQ-2 RESOLVED: per-user jail → D6/host batch** (consistent with LN-1's Q1 disposition; #5529 watch stands).
- **OQ-3 RESOLVED — the safe fallback is sanctioned, signal-based per S10.5's own class definitions:** an unknown zai 429 code WITH a parseable `next_flush_time` ⇒ Class 2 (depletion + signal — the class is defined by the signal, not the code); without ⇒ Class 3 probe-park. Never Class 1, never Class 4 (the R13 property). The 1311–1321 band joins the watch so a future doc expansion is caught. No amendment — a clearly-implied reading, logged here.
- **OQ-4 RESOLVED: `reasoning_effort` NOT wired.** Recorded as a dated fact (V9); revisit with live coding-endpoint confirmation at/after LN-CEREMONY via an S00.9 note if wanted.
- **OQ-5 RESOLVED: leave `modelCovered` untightened**; revisit when the model-list canary produces observed data.
- **Red tests:** specs 33/35 stay spec-only as §9 argues; the 2A executor materializes ALL its specs red-first per Amendment A, settling R2's resolver signature itself.

---

*Grounded 2026-08-23. Provider facts in §1 are DATA verified on that date against primary `docs.z.ai` sources, never constants and never the authority — the account's observed model list is (P-T17-3).*
