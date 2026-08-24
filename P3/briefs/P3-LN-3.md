# P3-LN-3 — the Kimi (Moonshot) lane: onboarding via the report-02 §5 checklist

Grounded 2026-08-24 against `Spec/drafts/` (canonical, spec-v1), `P3/CONVENTIONS.md` §61/§62/§63, and the tree at `cdeceaf`. Single-use: this brief expires when the packet lands.

**The Gate A–C audit is done and committed** as `P3/measurements/2026-08-24-kimi-lane-gate-audit.md` — 16 primary URLs fetched live on 2026-08-24, every load-bearing clause quoted verbatim, every unverifiable field in a numbered UNVERIFIED register. **Read it before writing a line of this packet.** It is not background; it is the data this packet turns into code, and it goes to the operator with the gate batch.

## 0. Verdict, and the one thing that is not routine

**The lane is BUILDABLE.** Gate A PASS (class 2, gray-zone note; A1 partial on price pages only), Gate B PASS, Gate C PASS with a mandatory data-routing rider. No class-3 clause exists; the sanction-determining ToS pages fetched in full, so this is **not** a BytePlus-style PENDING.

But this packet is **not** "zai with different strings", and a grounding that says so would be lying. Two findings break that assumption:

1. **Kimi's wire signals are message strings on ambiguous HTTP statuses, not numeric codes.** Quota exhaustion arrives on **403** *and* 429; model-entitlement failures arrive on **401**. `Classify`'s first rule (`internal/scheduler/limits.go:246-249`) freezes any lane on a bare 401/402/403, unconditionally, before any lane branch runs. **Wired naively, this lane hard-freezes itself and alarms the operator every single time its weekly window empties.** §4 is the whole treatment; **OQ1** is the decision it needs.
2. **The opencode substrate for this lane is `@ai-sdk/anthropic`, not `@ai-sdk/openai-compatible`.** The lane document already carries `npm` as data, so this may cost nothing — but "may" is not "does", and R9 makes it a test rather than an assumption.

Everything else really is data-driven, and §3 says exactly where the seams are honest and where two loaders quietly are not.

## 1. Scope wall

Backend only. `web/src` untouched, no running world touched, no route added, no UI surface. **$0 throughout** — every test is a fixture, a fake engine, or an `httptest` server; **no live provider call in this packet**. The operator's key reaches the broker through the guided ceremony only, never through chat, never through a commit, never as a literal in a test.

The audit is done; **this packet does not re-fetch the provider**. If an executor finds itself opening `kimi.com`, it has left the packet.

---

## 2. Requirements

Every requirement carries its S-ref (`Spec/drafts/`) or CONVENTIONS §-ref.

### R1 — the audit document is the packet's evidence base, committed and cited
[R02 §5 preamble ("The audit is a dated document (stored with the platform state); every entry cites primary URLs + access dates"); S03.6 (onboarding runs the report-02 §5 checklist)]

`P3/measurements/2026-08-24-kimi-lane-gate-audit.md` is committed with this brief. Every seed value in R2/R3 traces to a quoted line in it. **No number, id, URL or price may enter the code that is not in that document**, and any value the executor believes should differ is an OQ for the coordinator, never a quiet edit. The document is presented to the operator with the gate batch as the D10 approval input.

### R2 — `lanedata/kimi.json`: the lane document, seeded from captured values
[S03.6 (a lane is a provider entry plus billing flags, never a new substrate); S16.6 (billing flags `overflow_mode` + `region_model_gate` are per-model attributes); P-T17-2, P-T17-3; CONVENTIONS §61 (lane values are data with a date, never constants)]

New file `internal/adapters/opencode/lanedata/kimi.json`, in the exact shape `LaneConfig` already validates (`lane.go:257-352`). Every row carries `verified_on: "2026-08-24"` and a `source` URL. Seed values, all from the audit:

- `lane: "kimi"`, `substrate: "opencode"`, `display_name: "Kimi Code (Moonshot)"`
- `provider_id: "kimi-for-coding"` — **seed, and explicitly marked as such.** The models.dev provider URL resolves under this id; one read also offered `moonshotai` as an inference (audit U8). The document's note must say the installed opencode's own provider list is the authority and this is confirmed at bring-up (P-T17-3).
- `npm: "@ai-sdk/anthropic"` — **the structural difference from zai.** Note field must say why: this lane rides opencode on the Anthropic protocol, `zai` on OpenAI-compatible, and the field exists precisely so that is data.
- `base_url: "https://api.kimi.com/coding/v1"`, `endpoint_marker: "/coding/v1"`
- `endpoint_note`: the vendor's own warning, quoted — *"Kimi Code membership benefits and the Kimi Platform have different Base URLs. Please ensure your Base URL and API Key are matched correctly."* — **plus the honest limit**: unlike Z.AI, Moonshot does **not** publish what a membership key does against the metered endpoint (audit U5). The note says that, rather than implying a guarantee that was never made.
- `recorded_endpoints` (both `wired: false`, the zai precedent — recorded so the self-check owns the wrong-endpoint case by name):
  - `https://api.moonshot.ai/v1` (protocol `openai-general`, the international metered platform)
  - `https://api.moonshot.cn/v1` (as printed in the Kimi Code FAQ's own comparison table)
  - `https://api.kimi.com/coding/` (protocol `anthropic-docs-variant`) — the vendor docs' Anthropic base differs from models.dev's by the `/v1` suffix; recorded as a dated discrepancy, not silently resolved.
- `credential`: `{ profile: "kimi-code", env_var: <the name opencode's kimi-for-coding provider reads> }`. **The env var name is UNVERIFIED** (audit: models.dev returned no env field for this provider) → the executor reads it off the installed opencode provider record; if it cannot be determined without a live call, the document carries it as `""` with a note and R11's test asserts the named-missing-credential state rather than a guessed name. **Never guess a credential env var.**
- `models` — four, all `billing: "flat"`, all `region_model_gate: "none"`, all `overflow_mode: "opt-in-credits"`:
  | id | note |
  |---|---|
  | `k3` | flagship, 1M context; *"available to Moderato members and above"* |
  | `k3-256k` | 256K variant; *"available to Moderato members and above"* |
  | `kimi-for-coding` | *"available to all members"* |
  | `kimi-for-coding-highspeed` | *"available to Allegretto members and above"* |
- `default_model: "k3"` — the operator's stated reason for the lane is K3.
- **Tier gate ≠ region gate.** `region_model_gate` stays `"none"`: Kimi's gate is by *membership tier*, not region. The tier gate is real and enforced on the wire (401 *"Your current subscription does not have access to k3"*), and the thing that handles it is the **model-list-diff canary** (P-T17-3), not a new attribute. The document says this in a note; do not invent a `tier_model_gate` field.
- `overflow_mode: "opt-in-credits"` with a note quoting the proven disable — *"You can turn it off at any time: your balance stays in your account and the system pauses spending from it; turn it back on to resume."* — and recording the recommended operator posture: **Extra Usage OFF**, under which the lane behaves as a hard stop.
- `reset_marker`: **empty / absent.** The audit found **no documented reset-time signal** (U4): the error reference publishes no reset timestamp and states reset times live in the Console. Leaving this empty is the honest seed and routes depletion to Class 3 probe-park rather than a fabricated Class 2. A note says a live header probe at bring-up may populate it.
- `signals`: see R4 — the row shape itself is the open design question.
- `eco_options` / `thinking_disabled_efforts`: **omit.** K3 documents thinking efforts `low`/`high`/`max` (audit, changelog) but **no disable**, so the Z.AI `thinking:{type:disabled}` lever has no Kimi equivalent. Record the efforts as an unwired dated fact (the zai `reasoning_effort` precedent, `wired: false`) rather than wiring an S10.6 rung nothing sanctions.

### R3 — `plandata/kimi.json`: the plan document
[S10.1 (unit tiers), S10.4 (requests-as-proxy), S16.6; CONVENTIONS §63 (a flat lane is never priced in dollars from the engine)]

New file `internal/metering/plandata/kimi.json` in the shape `PlanDoc` validates (`planunits.go:233-295`). Seeded from the audit's C3, and **honest about a shape the current document may not express**:

- Rolling **5-hour** window, unit **requests**: ~300–1,200 per 5h depending on tier, 30 concurrent.
- **7-day** window, unit **credits/tokens**, **subscription-anchored** (*"Kimi Code's quota refreshes automatically every 7 days"*, from the subscription date; *"unused credits do not carry over"*) — structurally the same trap as zai's order-anchored weekly, and modeled the same way.
- **Monthly** membership credit pool, shared with the operator's non-Sinet Kimi use.
- `assumed_note` (mandatory, the zai precedent): every reading this lane produces is **ASSUMED** and labeled so on every surface, for a reason sharper than zai's — *"All signed-in devices and API Key share the same quota"*. Sinet is one consumer of a pool the operator also draws on interactively, so **Sinet's own count is a lower bound on consumption and never the pool state**. Consumption-pressure routing must not present it as anything else.
- **Per-tier allowance multipliers are UNVERIFIED** (audit U2 — secondary sources only). Seed the ratios the operator's own console shows, or seed nothing and let the tier-1 token gauge carry it; **do not seed the 1×/5×/15×/30× figures from aggregators.**

**Design question the executor must surface, not paper over:** the 5-hour window counts **requests** and the 7-day window counts **credits**. If `PlanDoc` carries a single lane-wide `unit` (as `plandata/zai.json` does), **it cannot describe this lane truthfully**. Either quotas carry per-quota units, or the document records one unit and states in `unit_note` exactly which window it describes and that the other is differently denominated. **Coordinator decision: OQ2.** Silently picking one unit and letting both gauges render in it is the failure this requirement exists to prevent.

### R4 — the signal table must express message-keyed rows, or say it cannot
[S03.1 (adapters forward raw signals as DATA; the taxonomy is the scheduler's); S10.5 (the classifier is pure and total)]

`LaneSignalRow` (`lane.go:169-180`) keys on `Code` / `CodeFrom`–`CodeTo` + `HTTPStatus`. **Kimi publishes no numeric error codes** — its taxonomy is (HTTP status × message string). Worse, `ExtractSignal` (`lane.go:~470-500`) requires a decodable `{"error":{"code":…}}` body: with no `code`, `decodeProviderError` fails, **no `LaneSignal` is produced at all**, and only a bare HTTP status reaches `Classify` — landing directly on the broken path in §4.

Requirement: extend `LaneSignalRow` with an **additive, optional** `message_contains` field, and extend `ExtractSignal` so that when no code decodes, it still yields a `LaneSignal` whose `HTTPStatus`/`BodyText` are populated and whose `Known` is true iff a document row's `(http_status, message_contains)` pair matches. Existing zai behavior must be **byte-identical** — code-keyed rows keep winning, and a document with no `message_contains` row behaves exactly as today (assert this with the shipped zai seed as a control, the `lane_ln2b_test.go:21-33` `laneDocWith` idiom).

Seed rows for kimi, verbatim from the error reference (all `verified_on: "2026-08-24"`):

| http_status | message_contains | meaning |
|---|---|---|
| 429 | `engine is currently overloaded` | transient shed |
| 429 | `receiving too many requests` | concurrency, transient |
| 429 | `reached your usage limit for this period` | 5-hour window depletion |
| 429 | `monthly usage limit for this billing cycle` | monthly pool depletion |
| **403** | `reached your usage limit for this billing cycle` | **weekly depletion — NOT auth** |
| **403** | `Access terminated` | **policy revocation — freeze + alert** |
| **401** | `does not have access to` | entitlement/tier gate — model drift, not revocation |
| **401** | `supports only kimi-k3 up to 256K context` | context entitlement — model drift |
| 401 | `API Key appears to be invalid or may have expired` | genuine auth |
| 401 | `Invalid Authentication` | genuine auth |
| 404 | `method not found` | **endpoint defect** — the wrong-endpoint case by name |
| 404 | `Not found the model` | model drift or permission |

### R5 — the limit classifier learns this lane's shape without breaking the ratified order
[S10.5 (the five-class taxonomy; Class 4 first so auth never falls into a retry-park, P-T08-2); CONVENTIONS §62 (the zai branch exists to stop the mirror-image false freeze)]

**See §4 for the full argument.** The requirement: after this packet, a kimi **403 weekly-depletion** signal classifies as depletion (park), a kimi **401 tier-gate** signal surfaces model drift, and only `403 "Access terminated"` and the genuine 401 auth strings freeze the lane. The mechanism is OQ1's answer; whichever is chosen:

- `Classify` stays **pure and total** — no lookups inside it; everything it needs arrives on the payload, as `EndpointVerified` already does (`limits.go:96-106`).
- The **existing zai path is unchanged**, proven by its full existing fixture suite passing untouched.
- No signal ever reaches a Class-1 retry on an auth shape, and no healthy-but-empty lane is ever frozen. Both directions are asserted.
- `laneKimi = "kimi"` joins the constants at `limits.go:22-26`; `isTransient`/`isDepletion`/`isDepletionNoSignal` (`limits.go:397-432`) gain `case laneKimi:` arms for the **codeless** case, mirroring the zai arms.

### R6 — the two seed loaders stop being single-file
[CONVENTIONS §61 (lane values are data); S16 (manifest home)]

**This is the one place the "entirely data-driven" claim is currently false**, and it is the packet's only mandatory plumbing:

- `internal/adapters/opencode/lane.go:25-26, 235-241` — `//go:embed lanedata/zai.json` into one `[]byte`, returning a one-element slice.
- `internal/metering/planunits.go:33-34, 197-203` — identical shape.

Both become **directory embeds** (`//go:embed lanedata/*.json` into an `embed.FS`, walked, returned **sorted by lane name**) — the in-tree precedent is `internal/storage/migrate.go:17`. Dropping a lane document in then genuinely adds a lane, which is what S03.6's "config-only" has to mean to be true.

**Two gates ride this edit** (neither exists today): a document declaring a duplicate `lane` name, or a duplicate `provider_id`, is **refused by name** at seed time. Without them `laneFor` (`lower.go:229`) silently takes the first provider-id match and `laneConfiguredModels` (`engineadapters.go:228`) silently overwrites `out[l.Lane]` — a config-only lane system whose failure mode is silent shadowing is not one.

**Ordering hazard, must be fixed in the same packet:** `internal/shell/zai_ln2b_test.go:41-44` commissions `lanes[0]` and asserts against it (`:66-70`, `:87-89`, `:97-104`). Sorted order puts **kimi first** and breaks all four. Do **not** "fix" this by appending kimi second — that leaves a test asserting things about whichever lane happens to be first while claiming to test zai. Rewrite it to select by lane name, the idiom `lanecred_ln2_test.go:85-89` and `lane_ln2_test.go:36-40` already use.

### R7 — `PaidLanes()` gains kimi, and every pinned count moves with it
[R02 §5 onboarding procedure (auth canary + conformance canary registered per lane); P-T17-1 (auth canary distinct from limit handling); P-T17-3 (observed-vs-config model diff)]

`internal/watchlist/canary.go:687-696`: add `LaneKimi = "kimi"`; `PaidLanes()` returns `{LaneAnthropic, LaneZAI, LaneKimi}`; the doc comment's "the two v0 lanes" is reworded (it is now three, and one is post-v0-by-amendment).

The canary sweep goes from **3 legs × 2 lanes = 6** to **3 × 3 = 9** (`canary.go:407-467`). The logprob leg is untouched — hardcoded to `LaneLocal` at `:462`, and **must stay refused for kimi**: the audit found no logprob support on the coding endpoint, and the S03.7 posture (paid lanes are behavioral-eval-only) applies unchanged. `sweep.Reasons` stays 3 (disarm reasons dedupe by leg kind).

**Every pinned count that moves — all of them, exhaustively:**

| File:line | Current | Becomes |
|---|---|---|
| `internal/watchlist/canary_test.go:223-224` | `Disarmed != 6` ("3 legs × 2 paid lanes") | **9**, comment "3 legs × 3 paid lanes" |
| `internal/watchlist/canary_test.go:388-389` | `Disarmed != 6` | **9** (sibling `len(sweep.Reasons) != 3` at `:392` **stays 3**) |
| `internal/watchlist/canary_test.go:597-598` | `sweep.Cards != 4` | **5** |
| `internal/watchlist/canary_test.go:612` | 4-name row list | add `"adapter-kimi"` |
| `internal/watchlist/canary_test.go:634-635` | `before+4` | **`before+5`** |
| `internal/conformance/conformance_test.go:169-170` | `len(states) != 14` | **15** |
| `internal/conformance/conformance_test.go:538-539` | `len(due) != 14` | **15** |
| `internal/conformance/conformance_test.go:481-497` | `want` map, 8 entries | add `"adapter-kimi": true` → **9** |

Tests that set `Lanes:` explicitly (`canary_test.go:429, 240, 301, 704, 757, 766`; `zai_ln2b_test.go:49, 130, 178`) are unaffected. `zai_ln2b_test.go:201-206` (logprob refused over every paid lane) passes unchanged and gets stronger.

### R8 — the watchlist actually watches this lane
[R02 §6 tier 1 + the §5 re-audit trigger ("wire the provider into §6's watchlist *before* go-live"); S2.8 via S14]

Four edits, three of which are silent-degradation bugs rather than compile errors:

- **`internal/watchlist/watchlist.go:193-197`** — `activeLanes` gains `"kimi": true`. Without it `severityFor` (`:200-219`) routes every price/terms/models/endpoint drift hit on kimi to the **daily digest instead of flag-now**. A lane whose terms can change without an alert is not onboarded.
- **`internal/watchlist/classify.go:70`** — the classifier's JSON-schema enum `["anthropic","zai","local"]` gains `"kimi"`. This is a **constrained-decoding grammar**: until it is added the local classifier *cannot emit* `"kimi"`, so no hit is ever attributed to the lane.
- **`internal/watchlist/seed.go:219-220`** — the row already exists, stamped as a candidate: `page("t1-kimi-coding", "https://kimi.com/coding", "moonshot", …)`. Re-stamp lane `"moonshot"` → `"kimi"` (this is what makes the `activeLanes` edit take effect for it) and drop "(candidate)". Add siblings for what the audit proved actually renders, mirroring `t1-zai-release-notes` (`seed.go:244`) and `t4-hn-zai` (`seed.go:357`):
  - `kimi.com/code/docs/en/kimi-code/error-reference.html` — **the classifier's source of truth**; a diff here can silently invalidate R4/R5. Highest-value row in the set.
  - `kimi.com/code/docs/en/kimi-code/whats-new.html` — vendor changelog (models, plan, Extra Usage)
  - `kimi.com/en/help/kimi-code/benefits` — carries the **"A new membership system is coming soon"** notice
  - `kimi.com/en/help/membership/membership-extra-usage` — the C1 overflow mechanism
  - `platform.kimi.ai/docs/pricing/chat-k3` — the D5 price row
  - `platform.kimi.ai/docs/agreement/modeluse` + `kimi.com/user/agreement/modelUse` — **the Gate-A re-audit trigger**; a class-3 clause here freezes the lane
  - an HN keyword feed for Kimi/Moonshot, points-thresholded, per the `zaiHN` date-stamper pattern (`seed.go:349-352`)
  URLs are asserted **as host+path parts**, never as routable literals (the package tripwire), with a re-verification date in `Notes` — the `zai_ln2b_test.go:215` idiom.
- **`internal/watchlist/cdio.go:408-424`** — `pageIncludeFilters`: **change nothing.** `t1-kimi-coding` is deliberately absent because the page served no main-content landmark, and the audit **re-confirms both `kimi.com/coding` and `kimi.com/membership/pricing` are still JS shells on 2026-08-24**. The pinned `held != 14` (`cdio_test.go:543`) and the `noLandmark` set (`cdio_test.go:518`) both stay as they are. Record the re-confirmation in the row's `Notes` — and note the operational consequence in the seed: **a text diff on a JS shell is near-empty and must never be read as "no change"**; the help-center and docs rows above are the real signal.

### R9 — composition, seats and substrate compose from the document, and this is proven not assumed
[S03.2 (one adapter contract, dual substrate); S08.8 step 3 (the execution seat); CONVENTIONS §63 (drain r2 R1: drive the real call sites, never the resolver alone)]

No new code is *expected* here — `engineadapters.go:110-131, 146-151, 185-204, 214-234, 245-263, 272-285`, `worker/routing.go:172-190, 738-796` and `stage/runner.go:388-401` all iterate the document slice. **The requirement is the proof**, because the `@ai-sdk/anthropic` difference (R2) is exactly the kind of thing that composes fine in one place and not another:

- kimi's substrate maps to `opencode` and dispatches to the registered opencode adapter
- kimi's execution seat takes `default_model` from the document
- planning/judge gain **no** second-lane seat (the `zai_ln2b_test.go:109-113` negative, extended)
- `chooseFlatLane` (`routing.go:738-796`) orders **two** flat lanes by pressure with **no dollar term anywhere** — the reflective money-member scan (`worker/zai_ln2b_test.go:154-163`) now covers a genuine two-lane choice for the first time
- coverage reports kimi only when commissioned

If any of these does need code, that is a finding for the evaluator and the coordinator, not a silent patch.

### R10 — conformance row
[R02 §5 procedure (conformance canary per lane); S03.3]

- `internal/conformance/registry.go` — a new `adapter-kimi` seed row modeled on `adapter-zai` (`registry.go:229-253`): weekly cadence, `AffectLane`, `TriggerEngineBump`+`TriggerWeekly`, `bumpGateNote` appended, and Notes stating plainly **what it does not prove** (no live provider call; fixtures only; `LN-CEREMONY` and `$0` named, per the `conformance_test.go:183-199` assertion).
- `internal/watchlist/canary_conformance.go:110-124` — `conformanceRowLane` gains `case "adapter-kimi": return LaneKimi`. Without it `RecordAdapterSuite` (`:135-143`) **refuses the row outright**, since an unmapped row resolves to `""`.
- Every `Fixture{Pkg, Run}` named must resolve to a test that actually exists — `TestSeedFixturesResolveToRealTests` (`conformance_test.go:205`) source-scans them.

### R11 — credential discipline, unchanged and re-proven for the new lane
[D2 (per-person credentials outside task sandboxes); S03.6 A5; CONVENTIONS §61]

The document holds an auth-profile **name**; material resolves fresh at spawn and is never stored, logged or persisted. Mirror `TestZAICredentialNeverLeaves` (`lane_ln2_test.go:302`) for kimi: a sentinel scanned through every event payload, `run_events` row, park record, identity key, ops log **and the whole SQLite file**. Mirror `TestZAIMissingCredentialIsNamedState` (`:423`) — and if the env var name could not be determined (R2), this test is what makes that absence a **named state** rather than a crash.

### R12 — the S00.9 amendment
[S00.9 amendment mechanics; G1 rider 3; S03.6]

Append to the Post-G4 changelog table in `Spec/drafts/S00-front-matter.md` (§S00.9), as **A11**, and mirror into `Spec/core-architecture-v1.md`'s assembled copy. Exact text — this paragraph is the deliverable, not a paraphrase of it:

> | A11 | 2026-08-24 | **The Kimi (Moonshot) lane is added to the lane set ahead of its post-v0 slot, on explicit operator order (2026-08-23).** S03.6 registers Kimi among the parked post-v0 providers that "join post-v0 config-only via the report-02 §5 onboarding checklist" [G1 rider 3]; this entry records that the operator, holding a Kimi membership that serves K3, ordered the addition early because the Anthropic lane alone is insufficient for reliable testing (Sinet's task execution shares the operator's Anthropic subscription with the build pipeline itself). **The rider-3 mechanism is unchanged and was followed, not bypassed:** the R02 §5 Gate A–C audit ran live on 2026-08-24 against primary sources and is committed at `P3/measurements/2026-08-24-kimi-lane-gate-audit.md` — Gate A **PASS** (class 2, tool-scoped with no unattended-use ban; OpenCode is a named sanctioned tool, so no new engine is forced), Gate B **PASS** (`https://api.kimi.com/coding/v1`, OpenAI- and Anthropic-compatible; opencode ships a first-party `kimi-for-coding` provider), Gate C **PASS** with `overflow_mode: opt-in-credits` on a **proven disable** (satisfying 3.10 / P-T17-2) and a **mandatory no-household-personal-data routing rider** (C5: Beijing operating entity, mainland-China data residency, training-on-inputs by default, no GDPR/EEA language). The audit's A1 is **partial** — the international USD plan-price pages are JavaScript shells — so the per-tier prices and allowance multipliers are carried as UNVERIFIED and close from the operator's own account console at bring-up (P-T17-3), not from documentation. The metered-exception list stays **EMPTY** [G1 P7]: this is a flat subscription lane, and DeepSeek remains the sole pre-registered designated exception. No ⚙ setting's default or clamp is touched — the lane's numbers live on the S18.3 data surfaces (the lane and plan documents) with no dotted key → **no S18 re-sweep**. The S18 tally stays **118 keys / 33 domains**. Marker sites annotated: S03.6 lane roadmap, S03.6 deferred "Post-v0 lanes" row, S16 onboarding manifest. | operator, 2026-08-23 order; presented for veto at the LN gate batch |

**Approval semantics, stated plainly for the gate:** the operator's 2026-08-23 order **is** the D10 approval for adding the lane. It is not retroactive consent for the audit's conclusions — those are presented at the gate batch for veto, and the **C5 routing rider (R13) is the one item that needs an affirmative operator answer**, not merely an absence of objection.

### R13 — the C5 data-routing rider
[R02 §5 C5 ("feeds the per-lane routing policy for personal/household data"); S03.6; the DeepSeek precedent, R02 §4]

The audit establishes, from the published privacy policy: operating entity **北京月之暗面科技有限公司** (Beijing Moonshot AI); personal information stored **within mainland China**; **training on inputs is the default** with only a contact-based opt-out; **no GDPR/EEA language**; and governing law that differs between the two applicable agreements (Singapore for OpenPlatform, PRC for the assistant property) with **no page stating which binds a membership** (U6).

The rider is written to hold under **either** reading:

> **The kimi lane carries a no-household-personal-data routing constraint.** It is approved for code and general technical work only; household personal data, personal correspondence, and identity-bearing content must never route to it.

**This is a routing-policy row on the lane, enforced where the other per-lane data-policy rows are enforced — not a sentence in a document.** If no such enforcement point exists yet, that is **OQ3**, and the honest interim is that the rider is recorded on the lane document and surfaced to the operator at the gate, with the gap named rather than implied away. Do not claim enforcement the code does not have.

Two points for the operator's decision, both from the audit: the operator's *interactive* Kimi use already sits under these same terms — the rider constrains **Sinet's automated routing**, which is the part Sinet controls; and **training-on-inputs is the sharper edge of the two**, since residency is a compliance posture while training means content is retained into a model.

### R14 — S16 onboarding manifest record
[S03.6 ("The onboarding checklist's manifest home is [XREF:S16]"); S16.6]

Record the lane in the S16 manifest: provider, plan, substrate (`opencode`), protocol (`@ai-sdk/anthropic`), billing regime **flat**, `overflow_mode: opt-in-credits` **with the operator posture (Extra Usage OFF)** and the proven-disable citation, the D5 API-equivalent price row (**$3.00 in / $15.00 out per M, cache-hit $0.30/M, USD, 1,048,576 ctx**, verified 2026-08-24), the audit document path, the C5 rider, and `verified_on: 2026-08-24`. The metered-exception list stays empty.

### R15 — receipts must change currency if overflow is ever enabled
[R02 §5 C1 ("Receipts must visibly change currency when overflow triggers", report-01 P-T01-3); 3.10; S10 flip mechanics]

**Kimi is Sinet's first non-`hard-stop` lane** (`zai` is `hard-stop`), so this stops being theoretical the moment the lane exists.

Minimum for this packet: the `opt-in-credits` value is carried end-to-end from the document to whatever surface reads billing regime, and a test asserts that **a lane whose `overflow_mode` is `opt-in-credits` is distinguishable at that surface from a `hard-stop` lane**. Flipping the regime is an operator act through the rehearsed kill-switch operation, **never automatic and never silent** — R02 §6 is explicit that billing-regime changes never auto-flip flags. Building the full currency-changing receipt path is **out of scope** if its surface has not landed; if so, say so in the packet report and leave a `TBD` naming the consuming section, the `ProposePlanBudget`/13.4 precedent recorded at LN-2B.

---

## 3. What the generic machinery genuinely lacks

Summarizing §2 against the survey, so the executor knows the true size of this packet.

**Genuinely zero-code (the machinery is honest here):** adapter registration, provider-entry compilation, lane resolution by provider id, credential injection, coverage, model-list config, lane→substrate dispatch, execution seat, stage dispatch, flat-lane pressure ordering, tier-3 plan reading, the meters API (reads lanes from the `runs` table, not a constant), receipt/ledger/pressure lane resolution. **Twelve seams, all already data-driven.** LN-1/LN-2 did their job.

**MUST change:**

| # | Site | Why |
|---|---|---|
| M1 | `opencode/lane.go:25-26, 235-241` | single-file embed → directory embed + duplicate-lane/provider gates (R6) |
| M2 | `metering/planunits.go:33-34, 197-203` | same shape (R6) |
| M3 | `watchlist/canary.go:687-696` | `LaneKimi` + `PaidLanes()` (R7) |
| M4 | `scheduler/limits.go` (§4) | the 403/401 collision — **the packet's real work** (R5) |
| M5 | `watchlist/watchlist.go:193-197` | `activeLanes` — silent digest-instead-of-flag (R8) |
| M6 | `watchlist/canary_conformance.go:110-124` | `conformanceRowLane` — the row is unrecordable without it (R10) |
| M7 | `conformance/registry.go:155-425` | the `adapter-kimi` seed row (R10) |
| M8 | `opencode/lane.go:169-180` + `ExtractSignal` | `message_contains` rows, additive (R4) |

**Also change:** `adapters/adapters.go:49-53` (`LaneKimi` constant, conventional); `watchlist/classify.go:70` (decoding grammar — functionally required); `watchlist/seed.go:219-220` (+ new rows); `opencode/lane.go:395` `CredentialInjectionFacts` (its doc comment is Z.AI-specific).

**Deliberately unchanged:** `watchlist/cdio.go:408-424` and its pinned counts (R8 — the JS-shell finding is re-confirmed, so the current treatment is correct); `watchlist/prices.go:155-158` `laneForProvider` (a flat lane is never priced; the S10.3 guard refuses an all-zero row).

---

## 4. The classifier collision — the packet's headline risk

`Classify` (`internal/scheduler/limits.go:242-249`) opens with:

```go
	if sig.HTTPStatus == 401 || sig.HTTPStatus == 402 || sig.HTTPStatus == 403 {
		return Action{Class: ClassAuthPolicy, Kind: ActionLaneFreeze, …}
	}
```

Unconditional, and **first** — deliberately, because Class 4 must never fall through to a retry-park (P-T08-2). The zai branch sits third, at `:263`, *after* this.

That order is correct for Z.AI, whose auth codes (1000/1001/1003/1220) genuinely arrive on 401/403 while every depletion code (1113/1302/1305/1308/1310) arrives on 429. **Kimi does not have that property.** From the audit:

| Kimi signal | Truth | Current ladder |
|---|---|---|
| `403 "You've reached your usage limit for this billing cycle"` | weekly window empty | **Class 4 lane freeze** ❌ |
| `401 "Your current subscription does not have access to k3"` | tier gate / model drift | **Class 4 lane freeze** ❌ |
| `401 "Your current plan supports only kimi-k3 up to 256K context"` | context entitlement | **Class 4 lane freeze** ❌ |
| `403 "Access terminated"` | genuine revocation | Class 4 lane freeze ✅ |
| `401 "The API Key appears to be invalid…"` | genuine auth | Class 4 lane freeze ✅ |

**A lane wired naively freezes itself and pages the operator with a suspected policy revocation every time its weekly window empties** — routine, recurring, and indistinguishable at the alert from the real thing. It is the exact mirror of the failure CONVENTIONS §62 records the zai branch was written to prevent, arriving through a different door.

And the fall-through is worse than it looks: with no numeric code, `ExtractSignal` produces **no signal at all** (R4), so nothing but the bare status reaches `Classify`.

**Three options; the coordinator picks (OQ1):**

- **(a) Narrow, kimi-scoped, spec-preserving.** Before the Class-4 status rule, insert a guard: *if the lane's document names this exact (status, message) pair as a depletion or model-drift row, the status rule does not fire.* Smallest change; touches `Classify`'s ratified opening; the guard is data-driven so it cannot fire for a lane that has not published the pair. **Recommended.**
- **(b) Data-driven generalisation.** Add a ratified `semantics` member (`auth` / `transient` / `depletion` / `model-drift` / `balance`) to `LaneSignalRow`, forward it on the payload, and replace `classifyZAICode` with one table-driven `classifyCodedSignal` that also suppresses `looksLikePolicyBan` for any lane whose document names the signal. Structurally the right answer and where this ends up eventually; it re-opens the zai path, so it is a larger blast radius than one lane addition should carry.
- **(c) Defer.** Not viable: the failure is routine and weekly, not an edge case.

**Whichever is chosen, one thing is non-negotiable:** these classifications rest on **documented message strings, not observed wire bodies**. The audit could not verify the actual JSON shape of a Kimi error body, whether it carries a `code` field, or whether any reset header exists (U3, U4, U5). **A live single-request probe at the key ceremony must capture one real 401, one real 429 and — if reachable — one real 403 body before the classifier is treated as final.** Ship the fixtures; do not ship the belief that they are complete.

---

## 5. ⚙ settings discipline

**No new settings key. The S18 tally stays 118 / 33.**

Pinned at `internal/settings/settings_test.go:44-45`, `:79-80`, `:450-451`, `read_test.go:32-33`; prose pins at `internal/settings/index.go:11` and `internal/conformance/registry.go:80`. **None of them moves in this packet** — and a packet that touches them has gone wrong.

Per CONVENTIONS §62/§63 the reason is structural, not incidental: the lane's endpoint, models, per-model billing/overflow/region attributes, credential reference, signal table, plan unit, multipliers and quotas all live in the two JSON documents, which are **S18.3 data-valued settings surfaces with no dotted key**. The lane *reads* only pre-existing keys: `limit.retry_cap`, `limit.retry_budget_ratio`, `limit.probe_interval_max`, `budget.background_window_fraction`, `canary.auth_interval`, `canary.behavioral_interval`, `pressure.cache_read_weight`, `watchlist.fetch_fail_streak`.

The structural constants deliberately left as code (`dateLayout`, `maxErrorScanStarts`, `resetLayouts`, `cancelGrace`, `streamReconnects`, `requestTimeout`, `authProbeTimeout`, `modelListBodyCap`, `conformanceDueWindow`) stay as they are. Making any tunable is a **separate S00.9 amendment that adds S18 rows and re-runs the sweep** — out of scope here, and not to be smuggled in.

---

## 6. Acceptance checklist (the evaluator's rubric)

All tests **$0**: fixtures, fake engines, `httptest`. Armed provider legs stay **LN-CEREMONY**.

**Document gates** (mirror `lane_ln2b_test.go:35` `laneDocWith` — edit the *shipped seed* by one field, so each negative differs from a known-good document by exactly one thing):
1. `TestKimiLaneDocumentLoads` — the shipped `lanedata/kimi.json` loads clean; base URL, endpoint marker, npm, four model ids, all `overflow_mode: opt-in-credits`, `verified_on` present on every dated row.
2. `TestLaneSeedRefusesDuplicateLaneOrProvider` — two documents with the same `lane`, and two with the same `provider_id`, are each **refused by name**; a control pair with distinct names loads. (R6 — a *new* gate.)
3. `TestPlanSeedLoadsEveryDocument` — both plan documents load; a duplicate `lane` is refused.
4. `TestMessageKeyedSignalRowsAreAdditive` — the **unmodified zai seed** classifies byte-identically before and after the `message_contains` extension (the control that proves R4 changed nothing); a kimi row matches on (status, message) and a non-matching message yields `Known == false`.
5. Negative table on the kimi document: invented `overflow_mode`, empty `region_model_gate`, missing credential block, undated signal row, malformed date — each `errors.Is(err, ErrLaneConfig)` with the offending field **named in the message**.

**Classifier fixtures — from the CAPTURED semantics** (mirror `limits_zai_ln2_test.go`, especially `:265` `TestZAINamedCodesAgainstBanAndLimitText`):
6. `TestKimi403WeeklyDepletionParksAndNeverFreezes` — **the headline test.** The 403 weekly-quota message parks; it does **not** freeze; asserted in both directions.
7. `TestKimi403AccessTerminatedFreezesLane` — the genuine revocation still freezes, and never retry-parks.
8. `TestKimi401EntitlementIsModelDriftNotRevocation` — both tier-gate 401s surface model drift; the genuine-auth 401s freeze. A property over the entitlement message set, not two examples.
9. `TestKimi429TransientRetriesAndQuotaParks` — overloaded/too-many-requests retry in place; period/monthly-limit messages park.
10. `TestKimi404MethodNotFoundSurfacesEndpointDefect` — the wrong-endpoint case surfaces `SurfaceEndpointDefect`, never an indefinite park (the R11/P-T08-2 failure class).
11. `TestKimiUnknownSignalNeverFreezesAndNeverRetries` — a property over unrecognised messages on each status: the safe direction (probe-park), never Class 1, never Class 4.
12. `TestClassifyStaysPureAndTotal` — **extended**, not replaced; and the full existing zai fixture suite passes **untouched** (the regression proof for OQ1's answer).

**Canary legs** (mirror `watchlist/zai_ln2b_test.go` exactly):
13. `TestKimiAuthCanaryRegisteredAndFreezesOnAuthShape` — `laneIn(NewAuthCanary(nil).Lanes, LaneKimi)`; an `httptest` 403 drives `ClassAuthPolicy` + `ActionLaneFreeze`, never park/retry; `PurposeTag == metering.PurposeProbe`; non-empty workload class; `VerifiedOn` stamp; finding raised; ⚙ cadence honored in both directions.
14. `TestKimiModelListCanaryToleratesAbsenceAndCatchesTierGate` — 404 ⇒ `ErrModelListUnavailable`, result **pass**, zero observed, zero delta, summary naming "unavailable", zero cards. **Plus the tier-gate case (P-T17-3):** an observed list lacking `k3` while the document lists it produces a **model-drift finding, not a lane freeze** — the K3 entitlement is exactly what this canary exists to catch.
15. `TestKimiBehavioralCanaryAndNoLogprobCanary` — behavioral present; logprob **refused** for kimi over `PaidLanes()`, with the local lane as the positive control.
16. `TestKimiWatchRowsVerified` — host+path compared **as parts**, lane `"kimi"` (not `"moonshot"`), re-verification date in `Notes`, and the JS-shell rows carry the "empty diff is not no-change" note.
17. Sweep counts: 9 disarmed, `Reasons` still 3, cards 5.

**Seat / substrate / composition** (mirror `shell/zai_ln2b_test.go:47`, `worker/zai_ln2b_test.go:41`, `stage/lanesubstrate_ln2b_test.go:60` — drive the **real call sites**, per §63 drain-r2 R1):
18. `TestKimiLaneDerivationsAtTheCompositionRoot` — nothing commissioned ⇒ nil; commissioned ⇒ coverage + substrate + seat; planning/judge gain no second-lane seat.
19. `TestKimiSeatResolvesUnderCoverage` — seat takes `default_model` (`k3`); the anthropic-only seat is unchanged; 2.7 gap advice unchanged without coverage.
20. `TestKimiSubstrateDispatchesToOpencode` — through `substrateFor` and the real dispatch, incl. the helper-spawn and revise call sites.
21. `TestFlatLaneSelectionAcrossTwoFlatLanes` — kimi and zai ordered by **pressure**, no dollar term; the reflective money-member scan extended; deterministic no-budget fallback.
22. **Source scans extended** (`lane_ln2_test.go:118`, `worker/zai_ln2b_test.go:177`, `metering/zai_ln2b_test.go:434`): `"api.kimi.com"`, `"kimi-for-coding"`, `"k3"`, `"api.moonshot"` must appear in **no** non-test source outside the JSON documents, with a vacuity guard.

**Credential:**
23. `TestKimiCredentialNeverLeaves` — sentinel scanned through every event payload, `run_events`, park record, identity key, ops log and the whole SQLite file.
24. `TestKimiMissingCredentialIsNamedState`.

**Conformance:**
25. `TestKimiLaneRowRegistered` — affect class, cadence, non-empty fixtures, tier-prefixed handles, exactly one row for the lane, Notes naming `LN-CEREMONY` and `$0`; row counts 15/15/9; `conformanceRowLane` resolves it.

**Spec + manifest:**
26. The A11 changelog entry is present in **both** `Spec/drafts/S00-front-matter.md` and the assembled `Spec/core-architecture-v1.md`, byte-identical, with the marker sites annotated. The settings tally is **unchanged at 118/33** — asserted, not assumed.

---

## 7. Open questions for coordinator disposition

- **OQ1 — the classifier mechanism (§4).** Narrow kimi-scoped guard (a), data-driven generalisation of signal semantics (b), or something else? Either touches `Classify`'s ratified opening or the ratified taxonomy's dispatch. **Blocks R5. Recommendation: (a) now, (b) as a later amendment when a third coded lane arrives.**
- **OQ2 — per-quota units (R3).** Kimi's 5-hour window counts **requests** and its 7-day window counts **credits**. Extend `PlanDoc` so quotas carry their own unit, or record one unit with an honest `unit_note`? Affects the meters API and the receipt gauge.
- **OQ3 — where the C5 routing rider is enforced (R13).** Is there a per-lane data-policy enforcement point today? If not, the rider is recorded and surfaced but **not enforced**, and the operator must be told that plainly at the gate rather than reading "rider applied" and assuming the code stops it.
- **OQ4 — the credential env var (R2).** If it cannot be read off the installed opencode provider record without a live call, the packet lands with the field empty and a named-missing-credential state, closing at the key ceremony. Confirm that is acceptable rather than blocking.
- **OQ5 — R15 scope.** If the currency-changing receipt surface has not landed, is carrying `opt-in-credits` end-to-end plus a distinguishability test sufficient for this packet, with a `TBD` naming the consuming section?
- **OQ6 — for the operator, at the gate.** (i) Confirm **Extra Usage stays OFF** (the recommendation; it makes the lane a true hard stop and moots the ¥-denominated top-up rails). (ii) Affirmatively accept the **C5 no-household-personal-data rider** — this one needs a yes, not merely no objection. (iii) Supply the **observed tier and model list** from their own console, which is what closes audit U1/U2 and is the runtime authority over anything the docs say (P-T17-3).

---

## 8. Coordinator dispositions (2026-08-24, pre-executor — binding for executor and evaluation)

- **OQ1 → (a), the narrow document-driven guard, with four hard constraints.** (i) The guard fires ONLY on an explicit documented (status, message-match) row naming depletion or model-drift — an unmatched bare 401/402/403 still freezes (the safe default is preserved; the guard is an exemption list, never a re-ordering). (ii) Genuine-revocation rows are never suppressible: 403 "Access terminated" and 401 invalid-key classify Class 4, and BOTH directions are pinned by tests. (iii) `Classify` stays pure and total — document rows reach it as data on the signal/config, never via I/O. (iv) §4's non-negotiable stands verbatim: every message-keyed fixture is marked DOCUMENTED-NOT-OBSERVED, and the ceremony's live probe (one real 401, one real 429, one real 403 body if reachable) precedes treating the classifier as final. The P-T17-1 auth canary is recorded as the authoritative revocation detector for whatever the message grammar misses. (b) is the pre-registered later path — an S00.9 amendment when a third coded lane arrives, noted in the deferred ledger, never smuggled in now.
- **OQ2 → extend `PlanDoc`: each quota window carries its OWN unit** (5h→requests, 7d→credits). A single unit with a prose note would make the numbers lie. The meters payload's unit becomes per-window — additive members, `omitempty`, contract fixtures regenerated through the sanctioned env (the LN-2B R3 precedent; fixtures are backend-owned snapshots) and the delta declared. zai's document (both windows credits) must be expressible unchanged.
- **OQ3 → verify first, then record HONESTLY.** The executor greps for any per-lane data-policy enforcement seam; if none exists (expected), the rider lands as lane-document data (`data_policy` row with the audit citation) + the gate disclosure sentence stating plainly "recorded and surfaced, NOT machine-enforced; enforcement lands with the routing-policy seam" + a deferred-ledger entry. The word "applied" never appears. No enforcement seam is invented in this packet (scope guardrail).
- **OQ4 → acceptable as designed.** The executor attempts to read the env var off the installed opencode provider record ($0, local — the models.dev-vendored provider definitions ship with the engine); if undeterminable without a live call, the field lands `""` with the note and R11 asserts the named-missing-credential state. Never guessed. Closes at the ceremony.
- **OQ5 → sufficient.** `opt-in-credits` carried end-to-end + the distinguishability test + a `TBD` naming the S10.2 currency-flip consumer. The ceremony script gains the "keep Extra Usage OFF" instruction as a literal step.
- **OQ6 → batched to the gate + ceremony**, and sequencing is safe by construction: the lane cannot be commissioned until the ceremony places a key, so the operator's affirmative C5 yes and Extra-Usage-OFF confirmation structurally precede any live use. The observed tier/model list additionally arrives via the model-list canary at first live smoke; docs seed until then.
- **R12 amendment:** the A11 draft is applied to the S00.9 changelog + assembled spec IN THIS PACKET citing the operator's 2026-08-23 order as approval, presented for veto at the gate batch (the C5 rider inside it still needs the affirmative yes — the gate script says so). No ⚙ default/clamp change → no S18 sweep. Draft==assembled byte-discipline per the A4–A8 precedent (awk concat verify).
