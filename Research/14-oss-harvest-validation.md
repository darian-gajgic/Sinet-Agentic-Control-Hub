# Research 14 — OSS harvest validation & adoptable-component sweep (T16)

**Date:** 2026-07-17 · **Wave:** B2 · **Depth:** FULL
**Brief:** `Research/briefs/T16-oss-harvest-validation.md` · **Shared context:** `Research/briefs/00-shared-context.md`
**Consumes (settled, cited, not re-researched):** `Research/decisions/GATE-1-architecture-direction.md` (CLOSED 2026-07-17, incl. sole-controller rider), committed reports 01–12, `Docs/nexus-post-mortem.md`, `Docs/component-harvest-map-proposal-v1.md` (the proposal under validation).
**Method:** deep-research harness — 5 fan-out search angles (engines / map sources / HITL & control planes / infra components / licenses & anti-harvest), primary-source fetching (LICENSE files, GitHub API, official docs), adversarial verification of load-bearing claims (independent re-fetches; one secondary claim killed by primary evidence, §7 OQ-8), cited synthesis. All web sources accessed **2026-07-17**.

**Effect of this report:** per the brief, this report supersedes `Docs/component-harvest-map-proposal-v1.md` as the citable harvest reference. The proposal doc stays untouched in `Docs/`; the spec cites this report.

---

## 1. Scope

- **Whole harvest map validated:** Nexus ports N1–N22; opencode O1–O4; OpenClaw C1–C5; Archon A1–A7; Crush K1–K2; SAW S1–S3; gh-aw G1; reference rows (deepclaude, deepagents, OpenHands, Goose, n8n starter-kit, awesome-harness-engineering); the full ANTI-HARVEST list; the coverage table's dangling "Slice/Phase 5/RQ" coordinates.
- **Consolidation:** every verdict already issued in reports 01–12 is collected into one table (§6.1); T16 issues fresh verdicts only for the remainder (§6.2) — Wave A/B findings deliberately outrank and pre-empt this report where they already ruled.
- **Live status checks** on everything marked ADOPT or STUDY: license, maintenance, capability claims (§2, §6).
- **NEW-candidates sweep** (operator's explicit ask): approval-inbox/HITL libraries, self-hosted agent control planes, durable-execution changes, UI kits, credential/secrets tooling, price-table/metering components (§3).
- **License audit** for the ADOPT set + NOTICE mechanics for a private never-distributed platform (§6.4). **Anti-harvest re-validation** against 2026 evidence (§6.5).
- Feature-list touchpoints: everywhere the map points; re-targeting table in §6.1/§6.3.

## 2. Current state of the art (mid-2026)

### 2.1 The engine layer held; the first-party lock hardened
opencode is alive and unweakened: now homed at **anomalyco/opencode** (sst/ redirects; the former SST team as Anomaly Innovations), **MIT** (LICENSE verified), latest **v1.18.3 (2026-07-16)**, ~daily releases (10 in 9 days), 963 contributors, 186.6k stars [S1–S3]. `opencode serve` still exposes the exact contract the ratified adapter builds on: OpenAPI 3.1 at `/doc`, SSE `/event`, generated `@opencode-ai/sdk`, and **per-message `{providerID, modelID}` selection** [S4–S5]. Engine-native subagents hard-disable via `{"permission": {"task": "deny"}}` — the GATE-1 sole-controller rider is satisfiable in config, no fork needed [S7]. Pinning is deterministic (install script `VERSION`/`--version` → versioned release binaries; `opencode-ai@x.y.z` npm) [S10]. Churn is real (a "stop breaking things" issue closed as not-planned; two open `serve` bugs [S11–S13]) — report 01's pin-plus-conformance-suite riders remain exactly right.

The Anthropic side hardened in H1 2026: server-side blocks (reported from 2026-01-09), a docs/ToS clarification (2026-02), and an enforcement wave (era of opencode removing its bundled Pro/Max plugins at 1.3.0 — its docs now say "Anthropic explicitly prohibits this") [S9, S17–S18]. The live legal doc's operative lines: OAuth is "intended exclusively for purchasers of Claude … subscription plans … to support ordinary use of Claude Code and other native Anthropic applications"; third-party developers may not "offer Claude.ai login or … route requests through Free, Pro, or Max plan credentials on behalf of their users"; and — decisive for report 01's posture — "Advertised usage limits for Pro and Max plans assume ordinary, individual usage of Claude Code **and the Agent SDK**" [S17]. Google went further: consumer tiers cut off from Code Assist/CLI serving entirely on 2026-06-18 [S119–S120]. Net: **the dual substrate (pinned opencode per user + wrapped `claude` CLI per user) is corroborated on every axis this sweep touched** — the wrap is the only ToS-compliant Claude-subscription path in the surveyed field (OpenHands OAuth = OpenAI-subscription only [S22]; Goose = no Anthropic OAuth [S29]).

### 2.2 The control-plane cohort died (~14-month half-life)
Every VC-backed free-and-open "agent control plane" of the 2025 cohort is dead or pivoted: HumanLayer SDK deprecated by its own README → closed commercial IDE [S51–S52]; **Bloop shut down 2026-04-10**, leaving Vibe Kanban (Apache-2.0, 27.4k stars) unmaintained-in-practice (last release 2026-04-24) [S58–S59]; **Omnara archived 2026-02-02** — its stated reason: the Claude Code CLI wrapper "became unfeasible to maintain" (a live datapoint for report 01's drift-canary discipline) [S60]; Terragon dead (2026-01-16) [S61]; Crystal deprecated [S68]. Survivors are company-subsidized (Emdash, Composio agent-orchestrator, Sculptor) or solo passion projects (Paseo, Claude Squad) [S62–S67]. **Implication: nothing in this space is a safe spine; Sinet owning its control plane (GATE-1 D1.1) is validated by revealed mortality, not just by design argument.** Adoption of anything from this ecosystem needs a funeral plan (P-T16-1, §7).

### 2.3 The approval-inbox niche is empty
There is **no mature, self-hostable, framework-agnostic approval-inbox library** in mid-2026. LangChain's agent-inbox (MIT) hard-requires a LangGraph deployment + LangSmith key [S53]; HumanLayer is dead [S51]; gotoHuman is SaaS; the one standalone match (impri) is v0.1/1-star [S55]. Sinet building S3.2 in-house is not wheel-reinvention. The best pattern to copy verbatim: the LangGraph interrupt payload — action request + `allow_ignore/allow_respond/allow_edit/allow_accept` + four response types (`accept/edit/response/ignore`) [S54]. MCP elicitation is mid-restructure (the 2026-07-28 spec replaces server-initiated elicitation with SEP-2322 `InputRequiredResult`; stateless core; sampling deprecated) — do not build the approval spine on it; keep an adapter seam [S56–S57].

### 2.4 Categories that newly exist since the map was written
- **Agent credential brokers:** Infisical **Agent Vault** (MIT, v0.39.0 June 2026) — egress placeholder-substitution so sandboxes never hold real secrets; the exact category D2/report 10 describe [S77–S78]. systemd v256+ has **per-user encrypted credentials** (`systemd-creds --user`, key bound to UID+machine-id; ramfs; not inherited down the process tree) [S71–S72].
- **Maintained open price tables:** pydantic/**genai-prices** (MIT, v0.0.71 July 2026; 30+ providers, 800+ models, historic/effective-dated prices, schema'd JSON exports) [S82]; LiteLLM's `model_prices_and_context_window.json` (MIT; 2,600+ models) as breadth cross-check [S83–S84].
- **Protocol maturation:** ACP hit stable v1 with foundation-ish governance (Apache-2.0, v1.19.0 schema 2026-07-06, 5 official SDKs, Zed registry) — report 01's O4 "align verbs now, adopt later" is aging well [S15–S16]. AG-UI (MIT) emerged as an agent↔UI event protocol worth mirroring [S94].
- **Sandbox/runtime donors** (already ratified by report 10, re-verified healthy): composed kernel primitives + Anthropic `sandbox-runtime`.

### 2.5 Licensing climate: fair-source is normal; licenses split per-directory
The field normalized non-OSI licenses (FSL: Crush; BUSL: Vault, Restate; SUL: n8n; ELv2: Phoenix; SSPL: Inngest) — all of which permit private self-hosted household use (§6.4). The subtler 2026 hazard: **repo-level license labels lie**. OpenHands is "MIT except `enterprise/`" [S20]; LiteLLM and Infisical likewise carve out `enterprise/`/`ee/` [S76, S83]. License checks must be path-scoped (P-T16-2).

### 2.6 A supply-chain warning shot
OpenClaw — MIT under the OpenClaw Foundation, hyper-active (v2026.7.1, 383k stars) — went through a severe 2026 security sequence: CVE-2026-25253 (8.8, one-click RCE-class, patched), and the **ClawHavoc** campaign: 341 malicious skills (~12% of its registry) growing to 824+, mostly stealer delivery; tens of thousands of exposed instances [S45, S47–S48]. STUDY-only posture emphatically validated — and the transferable lesson is about Sinet's own self-extending workforce: **no public skill/template registry imports, ever; imported definitions are untrusted content** (P-T16-4).

## 3. Candidate approaches (new-candidates sweep)

Verdict-per-candidate; "saves" = what Sinet doesn't build from scratch. Constraints applied: self-hostable only, adopt-unmodified-or-pattern, D-constraints, sole-controller rider.

### 3.1 Credentials & secrets (D2, feeds report 10's broker design)
| Candidate | License / status | Saves | Mode |
|---|---|---|---|
| **systemd-creds / LoadCredential** | OS-native; v256+ per-user scoping [S71–S72] | At-rest encryption + TPM sealing + per-service injection; sandbox units simply get no credential lines | **ADOPT** (mechanism inside report 10's broker) |
| **sops + age** | MPL-2.0 / BSD-3, both active (verified) [S79–S80] | Per-person encrypted file stores; multi-recipient; composes with `LoadCredentialEncrypted` | **ADOPT** (preferred store format) |
| gopass | MIT, active [S81] | Same role, GPG+git flavor | Alternative to sops+age |
| **Infisical Agent Vault** | MIT (ee/ carve-out), v0.39.0, young [S77–S78] | Broker-with-egress-substitution reference; "agent never sees the secret" | **STUDY → PATTERN** (single-host isolation assumption differs; report 10 owns the broker shape) |
| OpenBao | MPL-2.0, healthy (v2.5.5) [S73] | A full secrets server | REJECT for one host / STUDY if a vault is ever needed (license-safe Vault successor) |
| Vault (IBM, BUSL) [S74–S75], Infisical core [S76], 1Password broker (SaaS), secr.dev (SaaS) | — | — | REJECT (weight / open-core gates / not self-hostable) |

### 3.2 Price table & metering (D4/D5, feeds report 09's ledger)
| Candidate | License / status | Saves | Mode |
|---|---|---|---|
| **pydantic/genai-prices** | MIT, v0.0.71 2026-07, Pydantic-maintained, historic prices [S82] | The shipped-defaults half of the 3.1 price table + its update pipeline; user table shrinks to an override layer; historic prices let old receipts re-price correctly | **ADOPT** (vendored/pinned data + scheduled refresh; §7 OQ-5) |
| LiteLLM `model_prices...json` | MIT (root) [S83–S84] | Breadth cross-check (2,600+ models) | **ADOPT as cross-check data** |
| OpenRouter `/api/v1/models` | public endpoint, no auth [S85] | Live refresh source for router-visible prices | PATTERN (refresh source; never load-bearing at runtime) |
| tokencost | MIT, stale (last release 2025-08) [S86] | — | REJECT |
| OpenMeter (Apache-2.0) / Lago (AGPL) [S87–S88] | — | — | REJECT — millions-of-events billing engines; one-host overkill; Sinet's meter is its own event log (report 09) |
| LiteLLM proxy as meter | MIT core [S105] | — | REJECT (report 09 §5 stands; structurally blind to CLI-wrapped subscription lanes — the primary spend path never traverses a proxy) |

**No purpose-built self-hosted "per-user agent receipts" project exists** — receipts on Sinet's own event log + genai-prices remains the answer; nothing was missed.

### 3.3 Frontend / UI components (S1/S3; frontend stack not yet chosen — these are shortlist entries, not bindings)
| Candidate | License / status | Saves | Mode |
|---|---|---|---|
| **dnd-kit** | MIT, very active [S104] | Board drag-drop core (S1.3) | ADOPT-candidate (preferred) |
| @hello-pangea/dnd | Apache-2.0, maintained-slow [S103] | Kanban-specific drag-drop, faster path | ADOPT-candidate (fallback) |
| **react-diff-view** (+ gitdiff-parser) | MIT [S102] | Code/text diff review surface (S3.5) | ADOPT-candidate |
| monaco-editor diff | MIT [S101] | Full editor-grade side-by-side diffs | ADOPT-candidate (if editor wanted) |
| assistant-ui | MIT; runtime-swappable primitives [S93] | Chat surface (S3.6) without a framework backend | ADOPT-candidate (verify backend-agnostic claim at build; single-source download stats) |
| **AG-UI protocol** | MIT [S94] | Agent↔UI event vocabulary for S3.9 live surfaces | PATTERN (mirror the event shapes; don't adopt CopilotKit's runtime) |
| CopilotKit | MIT [S95] | — | STUDY only (assumes its runtime in the loop) |
| OpenLLMetry | Apache-2.0 [S100] | OTel span instrumentation | PATTERN/optional — report 12 ruled the event log is the substrate; OTel export is a projection, not a second truth |
| Langfuse (MIT core, ClickHouse-owned, heavy self-host) [S96–S97]; Phoenix (ELv2, light) [S98]; Laminar (Apache-2.0) [S99] | — | Trace-viewer UI | Projection targets only, per report 12 §5 — Phoenix is the lightest if ever wanted |

### 3.4 Control planes & HITL (pattern sources only — see §2.2/§2.3 mortality)
| Candidate | License / status | Worth taking | Mode |
|---|---|---|---|
| LangGraph interrupt schema / agent-inbox | MIT (LangGraph-locked) [S53–S54] | The approval-item contract: accept/edit/respond/ignore + per-action config flags | **PATTERN (copy verbatim into S3.2 card contract)** |
| Vibe Kanban | Apache-2.0, unmaintained since 2026-04 [S58–S59] | Engine-adapter layer, worktree lifecycle + orphan-GC war stories (already cited by report 05) | STUDY |
| Composio agent-orchestrator | Apache-2.0, active, 23 harnesses [S66] | The largest adapter matrix = drift-canary fleet member (extends C2's canary list) | STUDY |
| Paseo | AGPL-3.0, solo maintainer; daemon + native mobile clients [S67] | Closest architectural cousin (daemon + LAN/mobile approval); AGPL is fine for household use (§6.4) but solo-maintainer + spine-shaped = no adopt | STUDY |
| Emdash (Apache-2.0) [S65], Claude Squad (AGPL) [S62], Sculptor (MIT) [S64], container-use (Apache-2.0, Dagger) [S63] | active | Worktree/session isolation details; container-use is adopt-shaped but report 10's ratified kernel-primitive stack + sole-controller posture supersede a Docker-per-agent MCP server | STUDY |
| Omnara / Terragon / CodeLayer / Crystal (dead) [S60–S61, S68, S51] | — | Post-mortem reading; Omnara's death-reason feeds S2.8 canary discipline | STUDY (archival) |
| MCP elicitation | spec mid-restructure [S56–S57] | Post-2026-07-28 `InputRequiredResult` as a future inbox *adapter* | WATCH |

### 3.5 Durable execution — premise refresh only
DBOS now defaults to **SQLite** as its system database (dbos-transact-py MIT) [S89–S90] — the one material premise change since Wave B1. Report 08 already corrected the stale Postgres rationale and kept the rejection on shape grounds (re-invocable functions own control flow; wrong for parked interactive subprocesses). Restate is BSL 1.1 [S91]; Inngest SSPL [S92]; Temporal unchanged-heavy. **Nothing overturns report 05/08: rejections stand; DBOS remains the pre-registered fallback.**

### 3.6 Coverage gaps with no donor — confirmed empty
Intake pipeline (report 03 designed it; no OSS equivalent found), the consumption meter itself (report 09; §3.2), the approval inbox (§2.3), the benchmark practice (report 12 adopted promptfoo as runner; methodology is N21's). These stay build-fresh — the sweep confirms the map's §10 claim that the genuinely new pieces are the right ones to be new, and adds the inbox to that list.

## 4. Recommendation for Sinet

### 4.1 The G2 adoption package (consolidated)
**Layer A — ratified running dependencies, re-verified healthy today (cite: GATE-1 + this report §2.1):**
pinned **opencode v1.x serve** per user (MIT) [S1–S5]; wrapped **`claude` CLI** per user (proprietary; Consumer ToS); **SQLite-WAL** as sole state store (report 08); **systemd transient units + bubblewrap + seccomp + Landlock + netns/nftables + Anthropic `sandbox-runtime`** (report 10); **AGENTS.md + CLAUDE.md shim** (report 07); **promptfoo** pinned + **changedetection.io** (report 12); **xAI-via-opencode** when a member holds the plan (report 02, deferred per P6).

**Layer B — new ADOPTs from this sweep (small, data/mechanism-grade, no new spine):**
1. **genai-prices** (MIT) as the shipped price-table default + LiteLLM JSON as cross-check [S82–S84] — closes the one feature area (3.1/D5 defaults) that had no donor.
2. **systemd-creds/LoadCredential** + **sops+age** as the per-person store + injection mechanics inside report 10's broker [S71–S72, S79–S80].
3. Frontend component shortlist (dnd-kit, react-diff-view, monaco-diff, assistant-ui candidate) — pre-selections for the spec's frontend workshop, not bindings [S93, S101–S104].

**Layer C — patterns to copy (no dependency):** LangGraph interrupt/approval-item schema [S54]; gh-aw safe-outputs shape incl. default-on threat-detection pass [S39–S40]; Agent Vault egress-substitution broker idea [S77]; AG-UI event vocabulary [S94]; A2 workflow-dialect vocabulary (§6.2).

**Reasoning.** The sweep's dominant finding is negative space: the ecosystem produced no adoptable control plane, no approval inbox, no per-user metering — and everything spine-shaped died within ~14 months (§2.2). Sinet's build-the-control-plane/adopt-the-organs strategy is therefore not just ratified by GATE-1, it is what the 2026 graveyard teaches. Every new ADOPT above is deliberately *organ-grade*: data files, OS mechanisms, single-purpose UI components — replaceable, pinnable, no orchestration opinions.

### 4.2 What would change the decision
- **opencode v2 ships** (declared server-API break, report 01): conformance suite gates the migration; pin holds until it passes.
- **Anthropic un-pauses the credits change or narrows "ordinary, individual usage":** pre-registered P1 response (interactive-only demotion; headless → Z.AI/local).
- **A control-plane survivor consolidates** into a foundation-governed, self-hosted, engine-agnostic spine with >1-year health: re-run this sweep post-v0 (unlikely on current evidence).
- **FSL conversions mature** (Crush's earliest FSL releases → MIT ~mid-2027): the K-row "no code" policy can be revisited per version [S35–S37].
- **genai-prices goes stale** (>60 days without updates while providers reprice): fall back to LiteLLM JSON as primary + manual table (the spec's original posture).

## 5. What NOT to use and why

**Newly rejected by this sweep:**
- **HumanLayer SDK** — self-declared deprecated; company pivoted closed [S51–S52]. (Extends reports 03/04/05.)
- **agent-inbox as dependency** — LangGraph+LangSmith-locked [S53]; schema is PATTERN only.
- **MCP elicitation as approval spine now** — primitive replaced in the 2026-07-28 spec; client support inconsistent [S56–S57]. WATCH.
- **Vibe Kanban / Omnara / Terragon / CodeLayer / Crystal as dependencies** — dead or unmaintained (§2.2). STUDY only.
- **Paseo / Claude Squad / Emdash / Sculptor / Composio orchestrator as dependencies** — alive but spine-shaped (they want to own sessions/boards Sinet owns), solo-maintainer or vendor-subsidized; sole-controller rider excludes their orchestration layer by construction. STUDY.
- **container-use as the isolation layer** — Docker-per-agent MCP server; report 10's ratified kernel-primitive stack is lighter, credential-broker-compatible, and already decided. STUDY.
- **OpenMeter / Lago / LiteLLM-proxy as metering** — §3.2; report 09 §5 stands, now with the structural argument that proxies cannot see CLI-wrapped subscription traffic.
- **Vault (BUSL/IBM) / OpenBao / Infisical core as secret store** — one-host overkill; OpenBao is the noted license-safe successor if that ever changes [S73–S76].
- **tokencost** — 11 months stale [S86].
- **Trace platforms as system of record** — report 12 §5 stands; Langfuse self-host is 5-service/16-GiB-class heavy [S97]; Phoenix (ELv2) only as optional projection [S98].
- **n8n lane** — conditional never fired; see A-row/§6.2 n8n verdict.

**Standing rejections re-confirmed with fresh evidence:** LangGraph/Temporal/DBOS/Restate/Inngest as spine (§3.5); Gemini consumer lane (now fully cut off [S119–S120]); Goose *for now* (report 01 REVISE holds; AAIF donation improves the long-term fallback story [S23–S26]); peer-mesh, vector-first memory, standing rosters, metered load-bearing paths (§6.5).

## 6. Harvest-map verdicts

Legend: verdict — where issued — re-targeted feature-list home (repairs the map's dead "Slice/Phase 5/RQ" coordinates).

### 6.1 Consolidated verdict table (full map)

| ID | Item (short) | Verdict | Issued by | Feature-list home |
|---|---|---|---|---|
| N1 | Orphan-harvest ladder | **CONFIRM + WIDEN** (reconcile pass; OpenClaw hardenings; `wedged`; fencing) | 05 §6, 08 §6 | 3.8, 4.8 |
| N2 | CAS claiming + slot gates | **CONFIRM** (leases + fencing added; slots = `lanes` rows) | 08 §6 | 3.2–3.3, 15.6 |
| N3 | Quota-storm handling | **CONFIRM lesson / REVISE mechanics** (five-class taxonomy; downgrade ladder; jitter+budgets+breaker) | 09 §6 | 3.2, D4 |
| N4 | Dispatch state machine | **CONFIRM — ratified, extended** (tasks-vs-runs split; effect states) | 08 §6 (G1) | 4.6, 9.1 |
| N5 | Judge-loop v0 subset | **CONFIRM + 2 mandatory extensions** (P45 outcome axis w/ REOPEN-SPEC; ESCALATE verdict, P46) | 04 §6 | 5.2–5.5 |
| N6 | Deep Plan engine | **REVISE (heavily)** — ideas yes, five-stage shape no (15×-class trap; tools-off superseded by read-only exploration; SPEC-DOUBT added) | 03 §6 | 1.1–1.9 |
| N7 | Decision-card pattern | **CONFIRM — strengthened** (+13.5 explain layer; assumptions centerpiece) | 03 §6 | S3.2, 13.5, 5.6 |
| N8 | Frontier ledger | **CONFIRM posture / REVISE mechanism** (per-call usage × effective-dated prices; envelope $ demoted to cross-check) | 09 §6 | 3.1, 3.6, S2.5 |
| N9 | worktree.py | **CONFIRM** | **T16** (§6.2) | 4.1, S1.11, D9 |
| N10 | verify.sh culture | **CONFIRM + P46 extension** | **T16** (§6.2) | 15.1 practice, S2.7, 5.6 |
| N11 | Full judge cascade | **REVISE** (dissimilarity-chosen screen; cost-relief-only expectation; consumption caps) | 04 §6 | 5.3 (post-gate) |
| N12 | Quality Autopilot two-knob | **REVISE** | **T16** (§6.2) | 1.8, 3.5, 13.4 |
| N13 | Gated lessons + KB distillation | **CONFIRM — field-validated** (+batch review, Guru lifecycle, influence tracing) | 11 §6 | 8.1–8.3, S4.7 |
| N14 | WINS/LESSONS + exemplars | **CONFIRM + sharpening** (exemplars double as verification anchors) | 11 §6 | S4.7, 8.3 |
| N15 | Review v2 | **CONFIRM (rider: report 13/T12 owns final shape)** | **T16** (§6.2) | 6.1–6.3, S3.5 |
| N16 | Routing telemetry + collapse watch | **CONFIRM habit / REVISE router** (keyword router superseded; LiteLLM record shape + plain_reason) | 09 §6, 12 §6 | 7.7, S2.6 |
| N17 | Agent-lane memory scopes | **REVISE** (taxonomy → L0/L1/L2; **drop `lts`**) | 11 §6 | 8.6, S4.2 |
| N18 | Repo onboarding template | **REVISE** (AGENTS.md+shim; drop architecture map; human-prune gate) | 07 §6 | S1.6 |
| N19 | app_runner + preview | **REVISE** | **T16** (§6.2) | 6.4, S1.9 |
| N20 | Knowledge base day-one | **CONFIRM** (+registry treatment on import) | 11 §6 | S4.5, 8.3 |
| N21 | Benchmark methodology | **CONFIRM practice / REVISE 2 mechanics** (epoch stamping; anytime-valid rule) | 12 §6 | 11.2, S2.11 |
| N22 | Specialist prompt bodies | **CONFIRM at sanity level** (tool+context bundles first) | 06 §6 | 7.1, 7.5 (→T14) |
| O1 | opencode engine | **CONFIRM — strengthened, 3 riders** (pin v1.x; API lane only; conformance suite) | 01 §6 | D3, 2.3 |
| O2 | opencode provider layer | **REVISE** (Anthropic OAuth dead; xAI clause upgraded CONFIRM by 02 §6) | 01 §6, 02 §6 | D2/D3, 3.1 |
| O3 | Config formats as compile targets | **CONFIRM** (V2 migration rider) | 01 §6 | 7.2, 7.8, D8 |
| O4 | ACP support | **CONFIRM as STUDY — upgraded urgency** (v1 stable; gaps: checkpoint/fork, per-turn usage) | 01 §6 (+T16 status: v1.19.0 healthy [S15]) | D3 verbs, S2.8 |
| C1 | Runtime-policy pattern | **CONFIRM** (mechanism verified in current OpenClaw docs [S46]) | 01 §6 | D3 |
| C2 | claude-cli drift canary | **CONFIRM + WIDEN** (canary fleet; add Composio orchestrator §3.4) | 01 §6 | S2.8 |
| C3 | Per-agent auth-profile stores | **CONFIRM goal / REVISE mechanism** (broker outside sandbox; 0700+backup-crypto) | 01 §6, 10 §6 | D2, 10.1 |
| C4 | Quota-window surfacing | **CONFIRM as observed-state PATTERN** (prediction stays banned) | 01, 09, 12 §6 | 9.3, S2.5, 3.2 |
| C5 | Token-sink / OAuth refresh | **CONFIRM as STUDY** | 01 §6 | D2 credential manager |
| A1 | Archon dashboard + DAG builder | **REVISE** | **T16** (§6.2) | 9.4/S3.3 view; S1.3; editable → 15.5 |
| A2 | Workflow YAML schema | **REVISE** | **T16** (§6.2) | 7.1, 2.6 |
| A3 | `interactive: true` park/resume | **CONFIRM + 2 precisions** (approval node is the gate; resume is opt-in) | 05 §6 | 4.3, S3.2 |
| A4 | `fresh_context` per iteration | **CONFIRM + PROMOTE** (default session model at stage boundaries; ledger is the load-bearing half) | 06 §6, 07 §6 | 4.3, 8.7, D7 |
| A5 | 7-table schema reference | **CONFIRM as reference — job done** (now 14–15 tables upstream [S33]; Sinet ~12 with the machinery Archon lacks) | 08 §6 | 15.6, 11.1 |
| A6 | Platform adapters | **REVISE** | **T16** (§6.2) | 10.3, S3.8 (15.5) |
| A7 | Archon worktree manager | **CONFIRM (STUDY)** | **T16** (§6.2) | 4.1, S1.11 |
| K1 | Shared-workspace permission queue | **CONFIRM as PATTERN + rider** (durable ask record, not in-memory park; pull-based enumeration) | 01 §6, 05 §6 | S3.2, 4.3 |
| K2 | Default-deny permission service | **CONFIRM + boundary caveat** (ToolPolicy atop OS sandbox, never instead) | 01 §6, 10 §6 | 4.4, S5 |
| S1 | SAW role palette | **CONFIRM (PATTERN)** | **T16** (§6.2) | 7.5 (→T14) |
| S2 | Plan-strong / execute-cheap | **CONFIRM (PATTERN + rider)** | **T16** (§6.2; 06 §6 noted alignment, no verdict owed) | 2.3, 3.5 |
| S3 | Gate-discipline / DoD per stage | **CONFIRM + escalation rider** (stage contract declares finding categories + routes) | 03 §6, 04 §6 | 1.3, 5.4, 5.6 |
| G1 | gh-aw safe-outputs | **CONFIRM — promoted to primary pattern** (effects gate ≠ quality gate; threat-detection pass) | 04 §6, 10 §6 (+T16 status: graduated to github org, MIT, active [S38–S40]) | 4.2, 4.7 |
| ref | deepclaude / Design Space | **CONFIRM boundary + corollary** (design around Claude-lane compaction) | 07 §6 | 4.3, S2.8 |
| ref | deepagents | **CONFIRM reference / never depend** (mirror its published numbers) | 06 §6 (+T16: now a full harness product, 0.6.12 + deepagents-code — churn continues [S41]) | contract numbers |
| ref | OpenHands | **CONFIRM — upgraded** (best-aligned D7 fallback; 1-day spike queued) | 01 §6 (+T16 riders §6.2) | fallback slot |
| ref | Goose | **REVISE — wait for 2.0 consolidation** | 01 §6 (+T16 riders §6.2) | fallback slot |
| ref | n8n starter-kit | **REJECT (condition lapsed)** | **T16** (§6.2) | — (2.6 via C0 connectors) |
| ref | awesome-harness-engineering | **REVISE (pin referent)** | **T16** (§6.2) | post-gate Tier-B index |

### 6.2 T16 verdicts on the remainder — reasoning

- **N9 worktree.py — CONFIRM.** Report 08 already uses `snapshot_commit` in the checkpoint design and composes N9 with S5 sandboxes (08 §2.2, §2.9) — this formalizes that de-facto adoption. Fresh evidence: worktree-per-task is now the universal isolation idiom (Vibe Kanban, Claude Squad, Sculptor, Emdash, Archon all built on it [S59, S62, S64, S65, S33]); Nexus's branch-per-pipeline reasoning (stages share a branch; parallel pipelines isolated) remains better-argued for pipeline work than any surveyed per-run scheme. Port with its debugged edge cases; A7 stays the comparison read at design time. Target: 4.1, S1.11, D9. Effort S (unchanged).
- **N10 verify.sh culture — CONFIRM, extended by P46.** Pure practice, no license/maintenance axis; the post-mortem credits it for keeping a 10-day monolith operable, and spec 5.6 demands escalation paths "proven by tests." Extension: the check culture explicitly includes **escalation-route e2e tests** (a finding must provably reach a card — P46) and the dead-man canary Def.3 SLAs. Nothing in 2026 supersedes self-counting checks + pre-commit gates. Target: 15.1 build practice, S2.7, 5.6.
- **N12 Quality Autopilot two-knob — REVISE.** The derivation idea (two human-facing knobs → many internal settings, with a hard risk floor) survives; the ground under it changed at GATE-1: effort modes are defined against depletion (3.5), stakes-gating is ratified (1.8, reports 03/04), and the operator's settings-not-constants rider makes every derived number an editable, ceilinged setting. N12 ports as the **preset/derivation layer over that ratified settings surface** — Involvement×Spend presets emitting 13.4 setting values, risk floor = the 1.8 stakes gate. v0 ships fixed defaults (as the map already said). Target: 1.8, 3.5, 13.4.
- **N15 Review v2 — CONFIRM, with an ownership rider.** The sweep found **no OSS equivalent**: code-diff components exist (react-diff-view, monaco [S101–S102]) but nothing covers PR-style review of *every* deliverable type with findings→anchored-comments→retry drainage — the loop stays genuinely novel and spec-required (6.1–6.3, S3.5, 5.5). Rider: report 13 (T12, deliverables/review — in flight in parallel) owns the fresh design research; if its verdict diverges, report 13 wins and the coordinator reconciles (§7 OQ-3). Target: 6.1–6.3, S3.5. Effort L (unchanged).
- **N19 app_runner + time-travel preview — REVISE.** The capability is spec-required (6.4, S1.9) and no donor emerged. Revision: the substrate changed — disposable preview environments run inside **report 10's confinement classes** (C2-class, netns port remap under the sandbox stack), not bespoke bare venvs; previews are read-only effects and stay inside the gate model. Keep post-Review-v2 priority. Target: 6.4, S1.9.
- **A1 Archon dashboard/DAG builder — REVISE (PORT/ADAPT → STUDY now; PATTERN at 15.5).** Everything the map claimed exists and is MIT [S30, S33] — but three facts demote porting: (1) Archon was fully rewritten (Python→TS/Bun) in the April-2026 relaunch and moves fast (v0.5.0 June 2026) with an effectively **2-person bus factor** (1,010+418 commits, next contributor 45) [S30, S33–S34]; transplanted components inherit that churn — the fork antipattern in frontend clothes. (2) Sinet's spec makes the workforce map **view-only at launch** (S3.3; editing via the map is explicitly 15.5) — the *editable* DAG builder solves a deferred problem. (3) Generic MIT components (§3.3) + Sinet's own schema cover S1.3/9.4 more cheaply than adapting Archon's schema-coupled React tree. Take: Mission Control layout, DAG-viewer interaction patterns, `dag-layout.ts` ideas — as reading. Target: 9.4/S3.3 (view), S1.3 patterns; editable builder → 15.5.
- **A2 workflow YAML schema — REVISE (ADOPT-as-format → PATTERN: own dialect, mirrored vocabulary).** The schema is verified current (`prompt`/`bash` nodes, `depends_on`, `loop/until`, `loop.fresh_context`, `interactive: true` [S31–S32]) and remains a far better meta-agent emit-target than n8n JSON — that half of the map's claim stands. But adopting the format *verbatim* couples Sinet's generated workflows to a 2-person repo's evolving parser semantics while gaining no runtime (sole-controller rider: Sinet's control plane executes workflows; Archon's engine is never in the loop). Sinet defines its **own versioned dialect** mirroring the proven vocabulary, with two report-corrections baked in: `fresh_context` defaults ON at stage boundaries (report 07 promotion — Archon defaults it off), and the human gate is an explicit **approval node** (report 05's A3 precision), not an `interactive` dispatch flag. Deterministic `bash` nodes align with 2.6's born-as-machinery automations. Target: 7.1, 2.6.
- **A6 platform adapters — REVISE (ADOPT-later → PATTERN at 15.5).** The adapters exist but are in-monorepo TS modules coupled to Archon's runtime (Discord is community-tier, not core) [S33] — not standalone dependencies. At 15.5, ingress builds on the channels' own first-party SDKs with Archon's adapter *shapes* as reference; the P44 caveat carries unchanged (every ingress spawns under Sinet's scheduler with owner attribution, 3.4/10.3). `ARCHON_TELEMETRY_DISABLED=1` remains correct if Archon is ever run for study [S33]. Target: 10.3, S3.8.
- **A7 Archon worktree manager — CONFIRM (STUDY, as mapped).** Present in the current tree (`packages/git/worktree.ts`, isolation providers, worktree guide [S33]). Report 08 already leaned N9+sandbox composition; A7 stays the cleanliness-comparison read when slice-10-class design lands. Target: 4.1, S1.11.
- **S1 SAW role palette — CONFIRM (PATTERN).** SAW is MIT (README adds an attribution request stricter-sounding than MIT — honor it in NOTICE hygiene, §6.4), idle since 2026-03-19 [S50] — irrelevant for content-grade harvest. Consumed by T14 with report 06's steer: palette entries as tool+context bundles first, role prose second; never a standing formation (anti-harvest row holds). Target: 7.5, 7.1.
- **S2 plan-strong / execute-cheap — CONFIRM (PATTERN with the post-mortem rider).** Report 06 noted alignment ("no verdict owed"); T16 closes it: per-stage model floors survive as **effort-mode routing inputs** (2.3/3.5) — planning/judging never below a floor — but the *executor* side is bounded by inversion #2: cheap-executor-by-default is the bench-02 failure and stays dead; Smart mode routes execution to frontier flat-rate lanes. Target: 2.3, 3.5.
- **n8n self-hosted-ai-starter-kit — REJECT (condition lapsed).** The map's row was conditional: "ADOPT if DECISIONS picks n8n for the non-dev workflow lane." No decision picked n8n; the workflow lane that actually emerged from reports 03/06/07 + A2 is a Sinet-owned dialect executed by Sinet's control plane, with C0 connectors (2.6) for deterministic chores. Status seals it: the kit is near-dormant (Apache-2.0; last push 2026-01-06, no releases) [S42]; n8n core's Sustainable Use License would permit household use [S43] — this is a fit rejection, not a license one. SUPERSEDED-BY: own workflow dialect + C0 connector class.
- **awesome-harness-engineering — REVISE (ambiguous referent; pin it).** At least four same-named repos exist, all created ~March 2026; the two viable referents are `walkinglabs/awesome-harness-engineering` (3.6k stars, idle since 2026-05-22) and `ai-boost/awesome-harness-engineering` (3.1k stars, pushed 2026-07-17) [S44]. Keep as a STUDY index for post-gate Tier-B work; watch both until one wins (§7 OQ-2).
- **OpenHands (reference row) — report 01's CONFIRM stands; T16 adds status riders:** org moved to `OpenHands/OpenHands`; repo splitting (agent code → `software-agent-sdk`, MIT, v1.36.1 — the actual future integration target); root LICENSE is now **MIT with an `enterprise/` carve-out**; subscription OAuth is OpenAI-only (Anthropic = metered API keys; Claude-OAuth is an open feature request) [S19–S22]. The fallback spike, when it runs, targets the SDK repo and re-checks split churn.
- **Goose (reference row) — report 01's REVISE stands; T16 adds:** donated to the **Linux Foundation's Agentic AI Foundation** (strongest governance in the field), Apache-2.0 verified, v1.43.0 active, headless `goose run` intact, still no Anthropic subscription path [S23–S29]. Re-evaluate after its 2.0/ACP consolidation, as report 01 said — the foundation move raises its long-horizon fallback credibility.

### 6.3 Dangling-coordinate repair — result
Every kept item re-targeted to feature-list coordinates in §6.1 (the map's "Slice NN / Phase 5 / RQ#" system is fully replaced). **No kept item lacks a feature-list home** — the map's coverage claim survives translation. The only rows that died did so on merit (n8n conditional lapsed) or identity (ambiguous awesome-list), not homelessness. The two "should-be-new" builds the map named (window-aware scheduling half, outcome-sanity axis) were both confirmed and absorbed by reports 09 and 04 respectively; this sweep adds a third confirmed-new build: the approval inbox (§2.3).

### 6.4 License audit — the ADOPT set

Context: private, self-hosted, never-distributed, non-commercial platform; household users over LAN/Tailscale. Governing mechanics (verified against license texts [S35, S37, S43, S74, S107–S109]): **every attribution/NOTICE obligation in MIT/Apache-2.0/MPL-2.0/BSD is conditioned on copying-plus-distribution, never on execution or network serving — a never-distributed platform triggers none of them.** AGPL §13 binds only *modified* versions, and only toward the users who interact with it (the household). SUL/FSL/BUSL/ELv2/SSPL all permit private internal use.

| Component | License (verified) | Obligations for Sinet | Hygiene |
|---|---|---|---|
| opencode | MIT [S2] | none at runtime | keep LICENSE in-tree if vendored |
| `claude` CLI | proprietary (Consumer ToS) [S17] | ToS compliance, not copyright — P2 posture governs | S2.8 watch |
| sandbox-runtime (srt) | Apache-2.0 (report 10) | none (dependency) | keep LICENSE+NOTICE in-tree |
| SQLite | public domain | none | — |
| systemd/bubblewrap/nftables etc. | OS packages (various) | used as OS tools, unmodified — none | — |
| genai-prices | MIT [S82] | none | pin + refresh task |
| LiteLLM price JSON | MIT root; `enterprise/` excluded [S83] | none — data file is in root | **path-scoped license check (P-T16-2)** |
| sops / age | MPL-2.0 / BSD-3 [S79–S80] | none unmodified | — |
| promptfoo | MIT (report 12) | none | pinned |
| changedetection.io | Apache-2.0 (report 12) | none | local-ruleset config only |
| dnd-kit / react-diff-view / monaco / assistant-ui | MIT [S101–S104] | none until the frontend is ever distributed | keep headers |
| @hello-pangea/dnd | Apache-2.0 [S103] | none | keep NOTICE |
| SAW content (S1 palette) | MIT + README attribution request [S50] | attribution not legally required for content-derived patterns; honor the request anyway | one NOTICE line |
| AGPL STUDY items (Paseo, Claude Squad, Lago) | AGPL-3.0 | none (not adopted; even adopted-unmodified: no §13 duty; modified: source offer to household only) [S107] | — |

**Rule adopted:** keep upstream LICENSE/NOTICE files and file headers in-tree for everything vendored or copied — zero cost now, automatic compliance if anything is ever distributed. License checks are per-directory, never per-repo (P-T16-2).

### 6.5 Anti-harvest review (all rows)

| Exclusion | Verdict | 2026 evidence |
|---|---|---|
| Nexus 8 core-mods + guardian | **CONFIRM** (01 §6) | keystone (per-session models) native as per-message API param [S5]; release velocity makes forks deader |
| 18-specialist standing roster | **CONFIRM** (06 §6 + T16) | single-agent-first is Anthropic/Cognition/Google documented posture; counter-current (persistent agent pools, philschmid [S118]) is exploratory, not consensus; niche fixed-pipeline verticals don't transfer |
| server.py monolith topology | **CONFIRM** (T16) | unchanged bus-factor logic; §2.2 mortality shows even funded teams drown in big agent-platform surfaces — small owned core + adopted organs is the survivable shape |
| Judge-only frontier economics | **CONFIRM** (T16) | subscription execution economics verified current: Pro/Max limits sentence explicitly covers Claude Code + Agent SDK ordinary use [S17]; GATE-1 dual substrate + report 09 routing implement inversion #2 |
| JARVIS breadth (voice/vision/galaxy) | **CONFIRM** (T16) | 14.5/15.3 benchmark gate ratified; nothing in 2026 changes validate-before-breadth; satellite-tools framing stands (12.4) |
| mem0/qdrant-first memory | **CONFIRM + boundary conditions** (11 §6 + T16) | grep-class beats vector on memory-shaped data; Letta's own filesystem benchmark (74.0% > Mem0's 68.5%) [S116]; Anthropic memory tool is file-based [S115]; counter-evidence is vendor-benchmark-grade on disputed benchmarks [S117]; report 11's pre-registered exit criteria stand |
| Live-tree symlinks / machine-coupled deploys | **CONFIRM** (T16) | Def.6 systemd transient units ratified; opencode pins deterministically [S10]; CI-verified-package posture unchallenged |
| Archon fire-and-forget triggers | **CONFIRM** (T16) | P44 + sole-controller rider; A6's demotion to PATTERN makes it moot by construction — no Archon runtime ever runs |
| Crush: any code | **REVISE (rationale corrected; policy retained)** (T16) | the map's "legally required" is **wrong**: FSL-1.1-MIT permits private non-competing use *including code* today; per-version MIT conversion from ~mid-2027; pre-2025-05-30 portions MIT now [S35–S37]. Patterns-only **stays as policy** (adopt-don't-fork; Go/TUI stack mismatch; K1/K2 patterns suffice) — operator ratifies at G2 (§7 OQ-1) |
| Peer-mesh / group-chat topologies | **CONFIRM — strengthened** (06 §6 + T16) | 17.2× vs 4.4× error amplification (Google/MIT, 180 configs [S111]); 72% fact erasure; error-seed consensus solidification [S112]; Cognition single-thread doctrine [S113]; Anthropic's own 15× token figure [S114]; adversarial search found **no** production-scale validation of free mesh |
| Load-bearing metered paths | **CONFIRM — strengthened** (T16) | two of three majors tightened H1 2026 (Anthropic enforcement wave; Google consumer-CLI cutoff 2026-06-18 [S119–S120]); OpenAI's forbearance is one vendor's revocable posture, exactly the fragility the rule targets; 3.10/P7 stand |

## 7. Open questions

1. **Crush patterns-only ratification (R14-OQ1).** Legal necessity is refuted (§6.5); keep patterns-only as *policy*? **Proposed: yes** (adopt-don't-fork; no need for Go TUI code; revisit per-version as FSL→MIT conversions land from ~mid-2027). Owner: operator at G2.
2. **awesome-harness-engineering referent (R14-OQ2).** ≥4 same-named repos; proposed: watch `walkinglabs/…` (most starred) + `ai-boost/…` (most active) until one wins; drop the row if both stall. Owner: operator nod at G2; then T11 watchlist.
3. **N15/N19 reconciliation with report 13 (R14-OQ3).** T12 (deliverables/review) runs in parallel and owns the fresh design for exactly N15/N19 territory. If report 13's verdicts diverge from §6.2, **report 13 wins**; coordinator reconciles the consolidated table when it lands. Owner: coordinator.
4. **Frontend component picks (R14-OQ4).** §3.3 is a shortlist, not a binding (frontend stack undecided; assistant-ui's backend-agnostic claim and dnd-kit-vs-pangea need a build-time spike). Owner: spec-phase frontend workshop.
5. **genai-prices integration shape (R14-OQ5).** Proposed: vendor the pinned `data.json` + a scheduled watchlist task refreshing it as a *proposal* (price-table drift already triggers freshness re-validation per GATE-1 Def.5); user edits overlay, never overwritten. Owner: T08/spec.
6. **Agent Vault on one host (R14-OQ6).** Its egress-substitution broker assumes a separate machine; on Sinet's single host the sandbox↔broker boundary is netns/sandbox-enforced. Whether substitution-at-proxy beats report 10's ssh-agent-shaped operation-brokering for any lane is a STUDY input to the broker spec, not a decision now. Owner: report-10 spec workshop.
7. **OpenHands fallback target moved (R14-OQ7).** The 1-day fallback spike (report 01) should target `OpenHands/software-agent-sdk` (not the app repo) and re-check split churn + the MIT/`enterprise/` boundary. Owner: whoever runs the fallback spike (post-G2, only if triggered).
8. **Contradiction check — resolved, for the record (R14-OQ8).** A secondary source (The Register) paraphrased Anthropic's Feb-2026 clarification as banning subscription OAuth "including the Agent SDK." The **primary** legal doc instead (a) counts "ordinary, individual usage of Claude Code and the Agent SDK" inside Pro/Max limits and (b) prohibits *third-party developers* offering Claude.ai login / routing plan credentials *on behalf of their users* [S17]. This **supports** report 01 §2.1 and the ratified P2 gray-zone posture (each household member authenticates as themselves through Anthropic's first-party tool on the operator's host; Sinet offers no Claude.ai login and routes nothing on behalf of third parties). No prior report is contradicted; the S1 spike and T08's watch inherit the exact quoted language. Owner: closed; text lands in the spec's compliance note.

**New platform problems (for the spec's Known-problems list):**
- **P-T16-1 — Ecosystem mortality.** Agent-platform OSS half-life ≈14 months (§2.2). Every ADOPT needs: pinned vendored copy, a named replacement-or-rebuild path, and abandonment criteria (no release/security response in N months → exit). Extend report 02 §5's provider-onboarding checklist with a component-onboarding twin. Owner: spec (G2).
- **P-T16-2 — Per-directory license splits.** MIT-except-`enterprise/` is now a standard repo shape (OpenHands, LiteLLM, Infisical). Repo-level SPDX labels are insufficient; license verification is path-scoped, at adoption time and on every pinned-version bump. Owner: spec/NOTICE hygiene.
- **P-T16-3 — Protocol-surface churn as a scheduled event.** MCP restructures on 2026-07-28 (elicitation replaced, stateless core, sampling deprecated) [S57]; ACP is versioned-stable but pre-checkpoint/fork. Any Sinet protocol touchpoint pins the protocol version and joins the S2.8 canary watch with a dated migration trigger. Owner: adapter spec, S2.8.
- **P-T16-4 — Registry supply chain for a self-extending workforce.** ClawHavoc poisoned ~12% of OpenClaw's skill registry [S47]. Sinet's 7.x pipeline must never auto-import public skills/templates/workers; anything imported by a human is untrusted content under 4.7 and enters only through D10 gates with provenance recorded. Owner: T14/spec 7.x.

## 8. Sources

All accessed 2026-07-17. Tier noted where not primary. Repo-tree citations name the branch path; GitHub API citations verified live (several independently re-fetched by the coordinator as adversarial verification votes).

**opencode / engines / Anthropic policy**
- S1 https://api.github.com/repos/sst/opencode — identity: anomalyco/opencode, MIT, pushed 2026-07-17, 186.6k stars (re-verified).
- S2 https://raw.githubusercontent.com/anomalyco/opencode/dev/LICENSE — MIT text.
- S3 https://github.com/anomalyco/opencode/releases — v1.18.3 (2026-07-16); 10 releases in 9 days.
- S4 https://opencode.ai/docs/server/ — serve: OpenAPI 3.1 `/doc`, SSE `/event`, session endpoints.
- S5 https://opencode.ai/docs/sdk/ — `@opencode-ai/sdk`; per-message `{providerID, modelID}`.
- S6 https://opencode.ai/docs/agents/ — agent markdown/JSON config format.
- S7 https://opencode.ai/docs/permissions/ — allow/ask/deny grammar; `task` permission disables subagents.
- S8 https://opencode.ai/docs/acp/ — `opencode acp` client support.
- S9 https://opencode.ai/docs/providers/ — Anthropic Pro/Max plugin removal at 1.3.0; "Anthropic explicitly prohibits this."
- S10 https://raw.githubusercontent.com/anomalyco/opencode/refs/heads/dev/install — `VERSION`/`--version` deterministic pinning.
- S11 https://github.com/anomalyco/opencode/issues/22221 — churn complaint, closed not-planned.
- S12 https://github.com/anomalyco/opencode/issues/12065 — serve ignores configured model (open).
- S13 https://github.com/anomalyco/opencode/issues/26365 — serve web-UI tasks never terminate (open).
- S14 https://techfundingnews.com/opencode-the-background-story-on-the-most-popular-open-source-coding-agent-in-the-world/ — Anomaly Innovations backing (secondary, single-source).
- S15 https://github.com/zed-industries/agent-client-protocol — ACP Apache-2.0, schema v1.19.0 (2026-07-06), 5 SDKs, governance.
- S16 https://zed.dev/blog/acp-registry — live ACP agent registry.
- S17 https://code.claude.com/docs/en/legal-and-compliance — OAuth intended use; third-party routing prohibition; "Pro and Max plans assume ordinary, individual usage of Claude Code and the Agent SDK" (re-verified, quoted).
- S18 https://www.theregister.com/2026/02/20/anthropic_clarifies_ban_third_party_claude_access/ — enforcement timeline (secondary; its SDK reading killed by S17, see OQ-8).
- S19 https://api.github.com/repos/All-Hands-AI/OpenHands — org move to OpenHands/OpenHands.
- S20 https://raw.githubusercontent.com/All-Hands-AI/OpenHands/main/LICENSE — MIT with `enterprise/` carve-out.
- S21 https://github.com/OpenHands/software-agent-sdk — split-out SDK, MIT, v1.36.1 (2026-07-15).
- S22 https://docs.openhands.dev/sdk/guides/llm-subscriptions — subscription OAuth = OpenAI only; Claude OAuth open request (#2637).
- S23 https://api.github.com/repos/block/goose — redirect to aaif-goose/goose.
- S24 https://www.linuxfoundation.org/press/linux-foundation-announces-the-formation-of-the-agentic-ai-foundation — AAIF formation (2025-12-09).
- S25 https://goose-docs.ai/blog/2026/04/07/goose-moves-to-aaif/ — migration to foundation.
- S26 https://raw.githubusercontent.com/block/goose/main/LICENSE — Apache-2.0.
- S27 https://github.com/block/goose/releases — v1.43.0 (2026-07-14), 5–10-day cadence.
- S28 https://goose-docs.ai/docs/tutorials/headless-goose/ — `goose run` headless surface.
- S29 https://goose-docs.ai/docs/getting-started/providers — Anthropic API-key-only; OAuth only Copilot/Codex/Gemini.

**Map sources (Archon / Crush / gh-aw / OpenClaw / SAW / n8n / lists)**
- S30 https://github.com/coleam00/Archon (+ API) — MIT, pushed 2026-07-16, 22.9k stars, v0.5.0 (2026-06-26); ~2-person contributor concentration (re-verified).
- S31 Archon dev tree: `.claude/docs/workflow-yaml-reference.md` — YAML schema: `loop.until`, `loop.fresh_context` (default false), `depends_on`, prompt/bash nodes.
- S32 Archon dev tree: `.archon/workflows/defaults/archon-interactive-prd.yaml`; `.claude/skills/archon/references/interactive-workflows.md` — `interactive: true` exists.
- S33 Archon dev tree: `packages/web/src/components/workflows/*` (DAG builder), `packages/adapters/src/*` (telegram/slack/github core; discord community), `packages/cli/src/commands/telemetry.ts` (`ARCHON_TELEMETRY_DISABLED`), `migrations/000_combined.sql` (15 tables), `packages/git/src/worktree.ts` — feature-existence checks; no Supabase anywhere (SQLite/Postgres).
- S34 https://betterstack.com/community/guides/ai/archon-ai/ — April-2026 pivot/rewrite narrative (secondary; corroborated by release timeline + Bun prerequisite).
- S35 https://github.com/charmbracelet/crush/blob/main/LICENSE.md — FSL-1.1-MIT verbatim + Hoxha MIT trailer (2025-03-21→2025-05-30 portions); independently fetched twice.
- S36 Crush LICENSE commit history via GitHub API — MIT→FSL switch in #318, 2025-07-28; never reverted.
- S37 https://fsl.software/ — FSL-1.1 text/FAQ: Permitted Purpose incl. internal/non-commercial use; per-version 2-year MIT conversion.
- S38 https://github.com/github/gh-aw — graduated githubnext→github org; MIT; v0.81.6 (2026-06-27); active.
- S39 https://github.github.com/gh-aw/reference/safe-outputs/ — read-only default, structured action requests, per-handler caps, sanitization.
- S40 https://github.github.com/gh-aw/reference/threat-detection/ — default-on threat-detection pass; blocks safe outputs on detection.
- S41 https://github.com/langchain-ai/deepagents — MIT; 0.6.12 (2026-06-25); now a full harness product + deepagents-code.
- S42 GitHub API: n8n-io/self-hosted-ai-starter-kit — Apache-2.0; last push 2026-01-06; no releases.
- S43 https://github.com/n8n-io/n8n/blob/master/LICENSE.md — Sustainable Use License: internal/personal use permitted.
- S44 GitHub API: walkinglabs/awesome-harness-engineering (3.6k★, idle since 2026-05-22); ai-boost/awesome-harness-engineering (3.1k★, pushed 2026-07-17); +2 smaller same-named repos — ambiguous referent.
- S45 https://github.com/openclaw/openclaw — MIT (OpenClaw Foundation); v2026.7.1 (2026-07-13); 383k stars.
- S46 OpenClaw docs: `docs/concepts/agent-runtimes.md` (per-model runtime policy), `docs/gateway/cli-backends.md` (claude-cli backend), `docs/cli/secrets.md` (per-agent auth-profile stores), `docs/reference/api-usage-costs.md` (usage surfacing) — C1–C5 mechanisms exist today.
- S47 https://unit42.paloaltonetworks.com/openclaw-ai-supply-chain-risk/ — ClawHavoc: 341→824+ malicious skills (~12% of registry) (secondary, reputable).
- S48 CVE-2026-25253 coverage (bitdoze.com, conscia.com) — CVSS 8.8 one-click flaw, patched v2026.1.29 (secondary, multi-outlet; CVE-2026-32922 NOT independently verified — excluded from findings).
- S49 https://n9o.xyz/posts/202602-steipete-openclaw-openai/ — steipete→OpenAI; Foundation stewardship (secondary, corroborated).
- S50 https://github.com/bybren-llc/safe-agentic-workflow — MIT; README attribution request; v2.10.0, idle since 2026-03-19.

**HITL / control planes**
- S51 https://github.com/humanlayer/humanlayer — Apache-2.0; README: "pretty much all deprecated."
- S52 https://www.humanlayer.dev/ — commercial closed IDE; $100/user/mo Pro tier.
- S53 https://github.com/langchain-ai/agent-inbox — MIT; requires LangGraph deployment + LangSmith key.
- S54 https://docs.langchain.com/oss/python/langchain/human-in-the-loop — interrupt payload: allow_accept/edit/respond/ignore; four response types.
- S55 https://github.com/sekera-radim/impri — standalone HITL inbox, v0.1, 1 star (single-source; immature).
- S56 https://modelcontextprotocol.io/specification/2025-06-18/client/elicitation — elicitation as shipped.
- S57 https://blog.modelcontextprotocol.io/posts/2026-07-28-release-candidate/ — SEP-2322 InputRequiredResult; stateless core; sampling deprecated; 12-month deprecation windows.
- S58 https://www.vibekanban.com/blog/shutdown — Bloop shutdown 2026-04-10.
- S59 https://github.com/BloopAI/vibe-kanban (+ API) — Apache-2.0, 27.4k stars, not archived, last release 2026-04-24 (re-verified).
- S60 https://github.com/omnara-ai/omnara — archived 2026-02-02; CLI wrapper "unfeasible to maintain."
- S61 https://github.com/terragon-labs/terragon-oss — shutdown snapshot (2026-01-16), no maintenance.
- S62 https://github.com/smtg-ai/claude-squad — AGPL-3.0; v1.0.19 (2026-06-17).
- S63 https://github.com/dagger/container-use — Apache-2.0; early development.
- S64 https://github.com/imbue-ai/sculptor — MIT; v0.42.0 (2026-07-10); not accepting contributions.
- S65 https://github.com/generalaction/emdash — Apache-2.0; v1.1.39 (2026-07-14); desktop-only.
- S66 https://github.com/ComposioHQ/agent-orchestrator — Apache-2.0 (repo sidebar; a directory's "MIT" label wrong); v0.10.3; 23 worker harnesses.
- S67 https://github.com/getpaseo/paseo — AGPL-3.0; v0.1.110 (2026-07-16); self-acknowledged solo maintainer; daemon + native mobile clients.
- S68 https://github.com/stravu/crystal — MIT; deprecated 2026-02-26 (successor: Nimbalyst).
- S69 https://www.augmentcode.com/tools/open-source-agent-orchestrators — landscape directory (secondary; license claims unreliable — repo wins).
- S70 https://yetanotherorchestrator.app/ — 48-app orchestrator directory, ~2/3 closed-source (secondary; license column demonstrably wrong on Claude Squad).

**Infra: credentials / metering / durable execution / UI**
- S71 https://man7.org/linux/man-pages/man1/systemd-creds.1.html — `--user`/`--uid` (v256+), TPM2 sealing, `--name` binding.
- S72 https://systemd.io/CREDENTIALS/ — ramfs storage; service-user-restricted; not propagated down process tree; 1M/service.
- S73 https://github.com/openbao/openbao — MPL-2.0; LF/OpenSSF; v2.5.x 2026 releases.
- S74 https://raw.githubusercontent.com/hashicorp/vault/main/LICENSE — BUSL-1.1, IBM licensor; internal use non-competitive; 4-year per-version MPL conversion.
- S75 https://newsroom.ibm.com/2025-02-27-ibm-completes-acquisition-of-hashicorp,-creates-comprehensive,-end-to-end-hybrid-cloud-platform — acquisition closed.
- S76 https://github.com/Infisical/infisical — MIT core, `ee/` proprietary; RBAC/versioning paywalled self-hosted.
- S77 https://github.com/Infisical/agent-vault — MIT (ee/ reserved); v0.39.0 (2026-06); egress placeholder-substitution broker; separate-machine deployment assumption.
- S78 https://infisical.com/blog/agent-vault-the-open-source-credential-proxy-and-vault-for-agents — announcement (official).
- S79 https://api.github.com/repos/getsops/sops — MPL-2.0; pushed 2026-07-13 (re-verified).
- S80 https://api.github.com/repos/FiloSottile/age — BSD-3-Clause; pushed 2026-03-20 (re-verified).
- S81 https://github.com/gopasspw/gopass — MIT; v1.16.1.
- S82 https://github.com/pydantic/genai-prices (+ API) — MIT; v0.0.71 (2026-07); 800+ models; historic prices; schema'd exports; README accuracy caveat (re-verified).
- S83 https://raw.githubusercontent.com/BerriAI/litellm/main/LICENSE — MIT excluding `enterprise/`.
- S84 https://github.com/BerriAI/litellm/blob/main/model_prices_and_context_window.json — 2,600+ model price/context JSON (root = MIT).
- S85 https://openrouter.ai/api/v1/models — unauthenticated per-model pricing endpoint (routed prices, not list prices).
- S86 https://github.com/AgentOps-AI/tokencost — MIT; last release 2025-08 (stale).
- S87 https://github.com/openmeterio/openmeter — Apache-2.0 event-metering engine (scale-grade).
- S88 https://github.com/getlago/lago — AGPL-3.0 metering/billing platform (multi-service).
- S89 https://www.dbos.dev/blog/new-in-dbos-june-2026 — SQLite system-database support (official).
- S90 https://docs.dbos.dev/python/reference/configuration — SQLite default; Postgres recommended for production.
- S91 https://raw.githubusercontent.com/restatedev/restate/main/LICENSE — BSL 1.1; 4-year Apache conversion.
- S92 https://www.inngest.com/docs/self-hosting — SSPL server; 3-year per-release Apache conversion; single-node SQLite mode.
- S93 https://github.com/assistant-ui/assistant-ui — MIT; runtime-swappable React chat primitives (download stats single-source).
- S94 https://raw.githubusercontent.com/ag-ui-protocol/ag-ui/main/LICENSE — AG-UI protocol MIT.
- S95 https://raw.githubusercontent.com/CopilotKit/CopilotKit/main/LICENSE — MIT.
- S96 https://clickhouse.com/blog/clickhouse-acquires-langfuse-open-source-llm-observability — acquisition 2026-01-16; MIT core unchanged.
- S97 https://langfuse.com/self-hosting/deployment/docker-compose — web+worker+Postgres+ClickHouse+Redis+S3; ~4 cores/16 GiB.
- S98 https://arize.com/docs/phoenix/self-hosting/license — Phoenix ELv2; single-container.
- S99 https://github.com/lmnr-ai/lmnr — Laminar Apache-2.0; v0.2.1 (2026-07).
- S100 https://github.com/traceloop/openllmetry — Apache-2.0 OTel instrumentation (emits to any backend).
- S101 https://github.com/microsoft/monaco-editor/blob/main/LICENSE.txt — MIT; built-in diff editor.
- S102 https://github.com/otakustay/react-diff-view — MIT git-diff React component (+ gitdiff-parser, MIT, stable-stale).
- S103 https://raw.githubusercontent.com/hello-pangea/dnd/main/LICENSE — Apache-2.0 (not MIT); maintained react-beautiful-dnd fork.
- S104 https://github.com/clauderic/dnd-kit — MIT; active 2026.
- S105 https://docs.litellm.ai/docs/proxy/users — proxy per-user budgets/spend (OpenAI-compatible traffic only).
- S106 https://www.trmlabs.com/trm-tech-blog/never-give-an-ai-agent-a-credential-a-broker-and-the-process-we-trusted-to-build-one — broker pattern war story (secondary).

**License texts / anti-harvest evidence**
- S107 https://opensource.org/license/agpl-v3 — AGPL-3.0 §13 ("If you modify…") + §2 unconditional running of unconveyed works.
- S108 https://www.elastic.co/licensing/elastic-license — ELv2 three limitations.
- S109 https://www.mongodb.com/legal/licensing/server-side-public-license — SSPL §13 service-offering trigger.
- S110 https://arxiv.org/abs/2503.13657 — MAST: 14 MAS failure modes, 1,600+ traces, κ=0.88.
- S111 https://research.google/blog/towards-a-science-of-scaling-agent-systems-when-and-why-agent-systems-work/ (arXiv:2512.08296) — 17.2× independent vs 4.4× centralized error amplification; −70% sequential-task degradation; 180 configs.
- S112 https://arxiv.org/abs/2603.04474 — "From Spark to Fire": error-seed → system-level false consensus; ≥89% defense rates.
- S113 https://cognition.com/blog/dont-build-multi-agents — single-threaded doctrine; share full traces.
- S114 https://www.anthropic.com/engineering/multi-agent-research-system — multi-agent ≈15× chat tokens; hierarchical (not mesh) wins.
- S115 https://platform.claude.com/docs/en/agents-and-tools/tool-use/memory-tool — file/directory memory tool GA.
- S116 https://www.letta.com/blog/benchmarking-ai-agent-memory/ — filesystem agent 74.0% LoCoMo > Mem0 graph 68.5%.
- S117 https://mem0.ai/blog/state-of-ai-agent-memory-2026 — vendor counter-evidence (disputed benchmarks; different question).
- S118 https://www.philschmid.de/subagent-patterns-2026 — persistent agent pools/teams as exploratory counter-current (secondary).
- S119 https://geminicli.com/docs/resources/tos-privacy/ — Google sanctioned path = API keys; third-party OAuth violation.
- S120 https://github.com/google-gemini/gemini-cli/discussions/22970 — consumer tiers cut from CLI serving 2026-06-18.

*(120 numbered sources; S1, S30, S59, S79, S80, S82, S17 independently re-fetched by the coordinator as verification votes; S18's strong reading killed by S17 primary.)*
