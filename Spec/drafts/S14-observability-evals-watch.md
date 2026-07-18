## S14 — Observability, evals, benchmark & provider watch

**Scope:** How Sinet sees itself and the outside world: the event-type contract that makes the D7 event log the complete observability stack, live-inspection semantics over the one SSE endpoint, the watchdog suite, the conformance-suite registry, the watchlist executor with the lane-canary layer, the machinery around the signed benchmark pre-registration, regression evals and the revalidation runbook, trace retention, and queryable history. Boundary lines: storage is [XREF:S02]; transport and units are [XREF:S01]; surfaces are [XREF:S15]; meters/receipts are [XREF:S10]; local model seats are [XREF:S12]; verification *logic* is [XREF:S07] — this section records and detects, it never judges.

**Binding inputs:** R12 §4 (observability package) [G2 D2.1]; G2 Def.10 (watchdog/inbox numbers), Def.11 (retention), Def.12 (watchlist executor), Def.15 (awesome-harness watch), Def.16 (genai-prices refresh); G1 D1.3 (pause-and-flag), G1 Def.3-as-superseded (SLA set: canary daily, drill quarterly); G3 D3.5 (Layer-2 SQL), G3 Def.2 (`settings.changed`); `Spec/benchmark-preregistration-v1.md` (signed commit `5fb7082`, verified via `Spec/allowed-signers`) — cited below as **BENCH-REG**, amendable only via its §17; [SPIKE P2-S1] (Z.AI no logprobs — headline via S03.7); registrations from siblings: S01.6–S01.8 (listener-binding audit, resume-reconcile), S02.9 (kill-9 + suspend-cycle), S03.3/S03.5 (bump gate, leak tests), S04.5 (D6 violation attempts), S05.7 (compaction canary), S06.7–S06.9 (forced escalations, delta-card measurement hook); P-T11-1..5.

**Term coined here:** **conformance registry** — the control-plane table of standing conformance suites and drills (id, owning section, fixtures, trigger set, schedule, last run, last result), the single scheduling home for every "proven, not assumed" obligation the spec declares (S14.5).

### S14.1 The event log IS the observability stack

`run_events` in `platform.db` [S02.2] is the **only** observability substrate: everything in S2.1–S2.11 is an event type, a view, or a subscriber on that log [R12 §4; G2 D2.1]. No external trace platform ever becomes a system of record: the self-hosted trace market is sized 3–5 orders of magnitude above household volume and is churning through M&A (Helicone → maintenance mode, Langfuse → ClickHouse, promptfoo → OpenAI within six months) — a system of record MUST NOT carry another company's M&A risk (P-T11-4) [R12 §2.1, §5].

- **Full trace per run (S2.1, 11.1).** The complete story — every step, tool call, artifact, gate, decision, verdict, and cost, in `event_seq` order — is reconstructable from `run_events` + `checkpoints` for the whole retention period (S14.9). journald is the ops log, never the audit record [S01.11].
- **OTel posture:** event field naming mirrors `gen_ai.*` vocabulary (operation kind, model, token counts, tool name, parent linkage) so a mechanical OTLP projector remains an afternoon's work; **no OTel dependency ships at v0** — the GenAI semconv is Development-status with renaming rights reserved [R12 §2.1, §4.1]. The projector is parked (see Deferred).
- **Adoption criteria for this whole layer** (P-T11-4, binding on S16 rows): permissive/forkable license, pinned version, pre-registered swap, and **the record always lives in Sinet's DB — tools are runners or projectors, never stores** [R12 §7].

### S14.2 The event-type contract (v0, complete)

Report 08 gave the envelope (`event_seq`, run_id, generation, type, `schema_version`, ts, payload — caps, refs-not-blobs, validate-before-persist all [S02.1/S02.2]); this section completes the **v0 event-type families and their contract-level required fields** [R12 §4.1; G2 D2.1; G3 Def.2]. Exact DDL is the P3 schema workshop's [S02.2].

| Family | v0 types | Required payload fields (contract minimum) | Anchor |
|---|---|---|---|
| Run lifecycle | `run.state_changed` · `stage.started/finished` | from→to + cause (every FSM transition appends [S02.3]); stage id/kind, stage-brief hash, outcome | S2.1; [XREF:S05] |
| Checkpoint | `checkpoint.written` | checkpoint id + the S02.4 (a)–(e) refs (usage block, session cursor, ledger revision, artifact snapshot, version fields) | D7; [S02.4] |
| Usage & limits | `usage.recorded` · `limit.event` · `run.parked/resumed` | per-paid-call token/cache fields + price-table version; limit class (five-class taxonomy [XREF:S10]) + provider signal + resets-at; park cause + parked-until | D4/D5; S2.5 |
| Gate / ask | `ask.observed` · `ask.answered` | ask id, kind (gate\|question), invocation-snapshot ref, answer ref — projected from `asks` [S02.2] | 4.2/4.3 |
| Human decision | `decision.recorded` | actor, card id + type, decision, presented-at → decided-at (latency); effect approvals additionally journaled [S02.7] | S2.4 |
| Verification verdict | `verdict.recorded` | rubric id + version, axis (spec-compliance\|outcome-sanity), per-criterion results, findings `[F1..Fn]` with anchors, blocker/note split, **round number**, judge model id + its golden-set error rates at judging time | S2.3; logic [XREF:S07] |
| Routing | `routing.decided` | `{cause enum, score, signals, routed worker/model/lane, effort mode, plain_reason}` — one row per routed call; plain-language reason is mandatory, not prose-optional | S2.6/7.7 [R12 §4.1] |
| Knowledge injection | `knowledge.injected` | the trace-manifest entries `{item_id, source_path, content_hash, version, selector_rule, precedence_label}`, incl. post-compaction re-injection | S4.6/S2.9; [S05.4]; [XREF:S09] |
| Orchestration | `helper.spawned` · `spawn.refused` | spawn-record ref (trigger, reason, depth, budgets, brief hash [S04.4]); refusals name the failed check | D6; [S04.4/S04.5] |
| Watchdog | `watchdog.flagged/annotated/suppressed` | rule id, trigger evidence (signature, counts, spend vs baseline), Tier-1 annotation ref; suppressions are tuning signals (S14.4) | S2.7 |
| Drift & canary | `drift.finding` · `canary.result` | source, lane(s), change class (price/terms/limits/models/endpoints/billing-regime), severity, one-line summary, incident fingerprint; canary kind + lane + pass/fail/delta | S2.8 |
| Platform | `settings.changed` [G3 Def.2] · `auth.event` · `compaction.anomaly` · `retention.compacted` | settings: `{actor, key, old, new, reason}` (mirrors `settings_events` [S01.10]); auth: kind/user/device [S01.9]; compaction anomaly: `{stage, lane, engine version, window fill at trigger, pinned sections at risk, summary artifact ref}` [S05.7]; compaction pass logs itself | 13.4; S2.1 |
| Tools & artifacts | `tool.called/completed` · `artifact.produced` | tool name, args digest, duration, artifact refs; artifact ref + hash | S2.1 |
| Benchmark & eval | `benchmark.pair_recorded` · `eval.score_recorded` | the BENCH-REG §14 record schema verbatim; suite id + version, asset id + version, metrics, runner + runner version | S2.11; S14.7/S14.8 |
| Run summary | `run.summary_written` | summary artifact ref + generation inputs digest | 11.1; S14.9 |

**Contract rules.** (1) *Completeness:* an S2 capability that cannot be answered from these families is a contract defect — fix the contract, never bolt on a side store. (2) *Evolution:* new types/fields enter only by dated spec amendment (S00.9); every type carries `schema_version`, upcast-on-read [S02.1]. (3) *Consumers parse forward-tolerantly* — unknown types are logged and skipped, never fatal (the same discipline S03.1 imposes on engine streams).

### S14.3 Live inspection (S2.2, S3.9)

Semantics live here; the front chain, HTTP/2 termination, and `/events` unbuffering are [S01.4]; what each surface renders is [XREF:S15].

- **One SSE endpoint** on the control plane [S01.2]; one multiplexed stream per client, topic-tagged (run detail, board, fleet/meters, inbox). SSE `id:` = `event_seq`. Resume honors **both** `Last-Event-ID` and explicit `?after_seq=` — resume is one indexed SELECT over `run_events`, the entire cloud "resumable streaming" cost class deleted by the durable log [R12 §2.3, §4.3].
- **Reconnect = snapshot-then-tail:** current state projected from the DB, then tail events after the cursor (the AG-UI StateSnapshot→Delta pattern) [R12 §4.3]. Keepalive comments every `⚙ obs.sse_keepalive` (within the researched 15–30 s band).
- **Polling fallback is sanctioned:** clients MAY poll the same cursor (`GET …?after_seq=`) — transport is a client detail, never an architecture fork [R12 §3-B]. No engine SSE replay is ever relied on (opencode ignores `Last-Event-ID`, fix declined upstream) — a standing conformance assertion (S14.5) [R12 §7.9].
- **Progress semantics — never percent-complete** (no shipped product does it, for good reason): cards show FSM state incl. first-class *waiting-on-human* and *parked-until*, current stage, current tool + args digest, monotonic counters (tokens, API-equivalent cost so far, elapsed, steps), and the last-activity line; the derived `wedged` state [S02.3] surfaces here too [R12 §2.3, §4.3].

### S14.4 The watchdog suite (S2.7, 4.6, 3.7)

Three tiers; runs entirely on platform code + the local tier — **costs no allowance and keeps working when every paid window is empty** (Operating reality). Local seats ride duty aliases with a CPU floor, so the watchdog is never dark [XREF:S12].

**Tier 0 — deterministic counters over the event log, always-on, zero-cost.** Thresholds ship as ⚙ per G2 Def.10, seeded from shipped prior art [R12 §2.4, §4.4]:
- **Loop:** identical tool-call signature (hash of tool name + serialized args) `⚙ watchdog.loop_repeat = 5`× consecutive.
- **Ping-pong:** two-pair alternation `⚙ watchdog.pingpong_cycles = 6` cycles.
- **Error-loop:** same action → error `⚙ watchdog.error_loop = 3`×.
- **Silence:** no event past the run-type's budget `⚙ watchdog.silence_budget.<run_type>` — seeded from `recovery.dead_after` (5 min [G2 Def.2]) until per-type calibration; TBD-BRINGUP(per-run-type silence budgets from observed event cadence).
- **Spend:** per-run ceilings are enforced at [S02.2]/[XREF:S10]; the watchdog adds daily per-person total > `⚙ watchdog.spend_median_mult = 3`× the trailing-14-day median, **armed only after `⚙ watchdog.spend_arm_days = 14` days of history** — below that, ceilings only (every learned-baseline product documents a cold-start floor; fixed thresholds are methodologically correct at household cadence, not a compromise) [R12 §2.4].
- **Suspicious completion:** run ended with anomalously low tool/verification activity for its class — the silent-failure class [R12 §4.4].

**Tier 1 — local-model disambiguator [XREF:S12].** Invoked **only** on Tier-0 triggers, never per-event: last-N-turns + original request → grammar-constrained verdict `{loop | productive | unclear}` + confidence + one-line evidence, with retrieval over the run's own recent events. **It ANNOTATES the alert — it never gates, never kills** [R12 §4.4; G1 Def.9 discipline].

**Tier 2 — pause-and-flag, always [G1 D1.3].** Contain (stop admitting the next paid call) → park at checkpoint → card. Every card carries the trigger evidence, the Tier-1 annotation, **"resume — I was wrong"** as a first-class action, and per-rule **suppress** whose usage is logged: a rule suppressed `⚙ watchdog.suppress_retune_count = 2`× proposes its own threshold raise as an admin card [G2 Def.10]. Auto-kill does not exist anywhere in this suite — the Gemini-CLI/OpenHands false-positive record is the ratified evidence [R12 §2.4; G1 D1.3].

**Alert discipline (one reader, bus factor 1).** Two severities: **flag-now** (stall/loop/spend/auth-canary on active runs; foreign listener violation [S01.6]) and **daily digest** (everything else); dedup one alert per (run, anomaly class) with updates folded in; target `⚙ watchdog.flag_now_target = 2`/day — sustained breach is itself a digest-tier meta-alert ("your watchdog is too chatty") [G2 Def.10; R12 §4.4]. These severities are alert-routing classes, distinct from the approval inbox's Low/Medium/High risk tiers [XREF:S15]. The watchdog's own liveness rides the dead-man canary, daily [G1 Def.3 superseded set].

**Registered platform-health checks** (recurring watchdog entries declared by siblings, scheduled here):
1. **Resume-reconcile check** — confirms every S01.7 wake step completed (flush, network-identity reconcile, listener re-audit) [S01.7; P-T13-1 detection side].
2. **Listener-binding recurring audit** — no sinet process listens beyond loopback; explicit operator-visible allowlist; foreign violation = flag-now [S01.6/S01.8; P-T13-2].
3. **Organ-absence degraded state** — adopted organs down (watchlist executor, local-model units, port-pool) surface as degraded-mode flags [S01.6].
4. **Event-log size watch** — WAL size + table growth meters feed Tier-0 (P-T07-5 [S02.1]).

Post-v0 evolution (parked): CodeAD-shaped offline rule synthesis — repeated Tier-1 annotations or operator suppressions draft new deterministic rules as proposal cards; detection improves while the free-tier property is preserved by construction [R12 §4.4].

### S14.5 The conformance registry

Every "proven, not assumed" obligation in this spec is a **conformance registry** row; this section owns the registry and its scheduling — the suites' *content* stays with the declaring section. Results land as `eval.score_recorded` events in Sinet's DB (P-T11-4); any red raises a card (flag-now when lane- or storage-affecting). Suites **assert behavior, never docs** [S03.3].

| Registry entry | Declared by | Triggers / schedule |
|---|---|---|
| kill-9 crash harness (application invariants post-crash) + suspend-cycle test (fake `PrepareForSleep` + clock deltas) | [S02.9] | before any storage-touching component bump; quarterly sweep |
| Adapter per-lane conformance suites — D3 verb behavior, stream schema *and values* on fixtures, park/resume mechanics, **engine-lowering leak tests** (attempt-the-leak per config channel; assert the decoy did NOT take effect), native-spawn-disabled probes, no-reliance-on-engine-SSE-replay | [S03.1/S03.5]; [R12 §4.5(b), §7.9] | on any engine/CLI version change + weekly |
| D6 violation attempts: depth-3 recursion, lateral send, engine-native spawn per lane, over-budget spawn, mid-run helper kill — refusal asserted for the first four, containment-with-salvage for the fifth | [S04.5] | quarterly + on engine bump |
| Compaction canary: canary-constraint → compact → adherence check (pinned-survival is tested behavior, P-T04-1); compaction behavior re-measured per engine pin | [S05.7] | on engine bump + quarterly |
| Forced-escalation end-to-end tests (5.6: a finding that dies in a log is a platform defect): intake coverage-check card [S06.7], plan self-attack spec-is-bad [S06.8], helper ESCALATED path [S04.5], verification escalation [XREF:S07] | [S06]/[S04]/[XREF:S07] | quarterly, with the full escalation drill [G1 Def.3 superseded set] |
| Verified-restore drill (11.3) — registered here for visibility; pipeline and drill content owned by [XREF:S13] | [S02.9]/[XREF:S13] | per S13's schedule |

**Bump gating tie [S03.3]:** an engine bump lands only after (a) the candidate passes its per-lane conformance suite **and** (b) the before/after quality probe (S14.8) shows no regression — the Apr-2026 silent-drift class is invisible to schema tests and uptime checks (P-T02-5). A landed bump is a mass worker-revalidation trigger (P-T14-1) executed by the S14.8 runbook [XREF:S08].

### S14.6 The watchlist executor & the canary layer (S2.8)

Detect outside-world change **before failures or costs spread** — formats, limits, billing status, adapter behavior (D3). Executor posture per [G2 Def.12]; sources and cadence are the settled report-02 §6 list, held as config rows `{url|feed, type, parser-hint, lane|candidate}`.

- **T1 — page diffs: pinned changedetection.io** (Apache-2.0; own unit [S01.2]). Watch list managed via its REST API from Sinet's config rows (config-as-code); region-filtered verbatim diffs; Playwright fetcher for JS-walled targets; its **native LLM rules pointed at Sinet's local OpenAI-compatible endpoint** as first-pass "does this diff matter?" triage; hits POST via `json://` webhook to the control plane [R12 §4.5].
- **T2/T4 — feeds: Sinet-native feed poller** on the ratified scheduler [XREF:S10] — conditional GET, content-hash dedup, ~15 feeds (GitHub releases/issues Atom, models.dev commits, hnrss.org — never self-hosted hnrss (no license file) — authenticated Reddit feed URLs). **Miniflux is the pre-registered fallback** if feed handling proves gnarly; its HMAC-signed webhook shape is the drop-in contract [G2 Def.12; R12 §4.5]. **T3 — aggregator APIs:** weekly scheduler jobs (models.dev `api.json` diff, price/model-list sources); the genai-prices `data.json` refresh runs here and always lands as a *proposal* — price-table drift then fires the G1 Def.5 freshness trigger [G2 Def.16; XREF:S10].
- **Second-pass classification (local tier):** every hit — page diff, feed entry, API diff — is classified `{relevant lane(s), change class, severity, one-line summary}`, grammar-constrained; relevant hits become **drift cards** in the approval inbox with fingerprint-per-incident-window dedup (one card per storm); price hits carry proposed price-table rows with effective dates (P-T08-3 [XREF:S10]); **billing-regime changes never auto-flip flags** — operator confirms, then the rehearsed 3.10 flip runs [XREF:S10; R12 §4.5].
- **The API canary layer (Sinet-built — the one genuinely novel piece):** per lane —
  - **Auth canary, distinct from limit handling (P-T17-1):** scheduled cheap real request at `⚙ canary.auth_interval` (default daily [G1 Def.3 set]), error-class discrimination via the five-class classifier [XREF:S10]; an auth-shaped failure classifies *policy-revocation-suspected* → lane freeze + flag-now, **never** an infinite retry-park [S03.6]. Canary consumption is metered like everything (D4) and itemized on the lane owner's meters — negligible by construction [XREF:S10].
  - **Conformance canaries:** the S14.5 adapter suites on their weekly schedule.
  - **Behavioral canaries:** scheduled fixed-prompt mini-eval per lane (promptfoo-runnable), alert on score drop, at `⚙ canary.behavioral_interval` (default weekly, aligned with the conformance cadence `[coordinator-draft]`) — the **only** drift detection available to the subscription lanes.
  - **Logprob canary — LOCAL LANE ONLY:** one-token logprob drift tracking at ~zero cost. Z.AI exposes no logprobs endpoint-wide and the wrapped Anthropic CLI exposes none either → both subscription lanes are behavioral-eval-only [S03.7; SPIKE P2-S1 Probe 8; R12 §4.5].
  - **Model-list drift:** observed-vs-config diff per account (P-T17-3) rides the same schedule [S03.6]. **Engine memory features and compaction behavior join the canary set** [G2 §Follow-ups; S05.7].
- **Decay posture (P-T11-3):** per-source fetcher escalation ladder (plain → Playwright → flagged-decayed card proposing an alternative source); a meta-watch flags fetch-failure streaks ≥ `⚙ watchlist.fetch_fail_streak = 3` cycles — a dead watch MUST announce itself; standing bias to migrate any scrape target to a feed/API the moment one exists [R12 §4.5].

**Standing watch items registered by the gates** (rows in the same config store): fine-grained-PAT collaborator gap · Free-plan ruleset availability · diffity maturity · jj compat matrix · X-Wing age-CLI plugin status · DBOS TS/SQLite parity · MCP 2026-07-28 final (Tasks/Extensions — the S02 durable-task-handle watch) · Litestream #1083 · Python-Redlines + pandiff license/maintenance verify [G2 §Follow-ups; S02 Deferred] · awesome-harness-engineering both referents, drop if both stall [G2 Def.15].

### S14.7 The benchmark practice (11.2 / S2.11 / 15.3)

**The protocol is BENCH-REG** — the signed registration (commit `5fb7082`), not this spec. This section builds the machinery around it and cites it; **none of its registered numbers is this spec's to change** — amendments happen only via BENCH-REG §17, and silent drift between that text and the running platform is a platform defect (its own clause). What S14 owns:

- **Sampling hook:** fires at eligibility (BENCH-REG §4.2 rules; uniform-random among eligible tasks — the uniformity is frozen, the rate is ⚙ per its §1); declines are logged and the decline rate reported. Duplicate (direct-arm) runs are admitted as background work drawn from the requester's automation budget under their standing opt-in — they throttle under pressure like any background work [G2 Def.4; XREF:S10].
- **Blind-pair machinery:** both arms rendered through the one uniform presentation template (tells stripped, length never truncated); position randomized; verdict A/B/tie/both-bad **plus the mandatory arm-guess in the same form** — blindness is measured, never assumed (P-T11-1; BENCH-REG §3/§5). Reveal only after the §14 record is written to Sinet's store (keep-forever class). Card/render surfaces are [XREF:S15].
- **Epoch tracker (P-T11-2):** the direct arm's observed model identity is recorded per pair; an identity change ends the epoch; **no decision statistic ever pools across epochs** (BENCH-REG §9). Nexus bench-02 is a closed historical epoch, never pooled.
- **Decision statistic:** the Beta-Binomial posterior `G` per (domain, epoch), updated per pair, anytime-valid with optional stopping — the only decision input; fixed-n views are readouts (BENCH-REG §7/§16). **Alarm** (`1 − G` past the registered threshold): flag-now card + **expansion freeze** (14.5 hold), operator disposition logged as a decision event (BENCH-REG §12). **Gate** (15.3): the four registered limbs (minimum non-tied pairs, `G` floor, no active alarm, regression suites green at registered floors) evaluated per launch domain over the current epoch (BENCH-REG §10/§11) — this section evaluates and displays; it sets nothing.
- **Small-n honesty as a product surface (P-T11-5):** wherever a win rate renders, the honest-claims table renders with it (BENCH-REG §15), and every published rate carries n, `G`, tie/both-bad rates, decline rate, and guess accuracy. There is no benchmark number without its epistemics.
- **Done-directly feed [XREF:S10]:** receipts carry the per-run heuristic line from day one; once a domain accrues the registered minimum of measured pairs, S14 computes and publishes the **measured median direct-arm consumption per domain** — the aggregate honesty figure S10's dashboards consume, labels verbatim per BENCH-REG §13.
- **Standing benchmark/eval questions register** (questions the practice must eventually answer, recorded so they cannot evaporate): (a) **R13-OQ6** — do anchored `[F#]` findings beat file-level notes on retry quality? [G2 §Follow-ups]; (b) **P-T05-2** — delta-card rubber-stamp measurement: S06.9's hook records presented-delta size, time-to-decision, decision, outcome linkage; the analysis runs here, and measured rubber-stamping proposes a card-format retune [S06.9]; (c) **P-T05-4** — pre-registered stakes-classifier eval before v1 household use [S06]; (d) intake approval-UX changes are justified only by measured outcomes [S06.9].

### S14.8 Regression evals & the revalidation runbook (7.3, 5.7)

- **Runner: pinned promptfoo** (MIT; pin in `components.lock` [XREF:S16]); **DeepEval is the pre-registered swap** if post-acquisition governance drifts. Tools are runners, never stores: every score/verdict lands in Sinet's DB as `eval.score_recorded` (P-T11-4) [R12 §4.7]. Local models run via the local lane [XREF:S12].
- **Per-asset eval objects:** every worker template and every rubric carries an eval set — golden cases (25–50, grown from real outcomes) + a **planted-defect suite** the judge must fail (code: test-adequacy defects + known-bad artifacts; research at v0.1: planted unsupported citations) [R12 §4.7]. Floors register per rubric/eval-set version at 8.3 knowledge-gate entry [BENCH-REG §10.1(d); XREF:S09].
- **Revalidation runbook** (trigger: a watchlist drift finding, an engine bump, or a model/duty-alias swap): flag dependent workers/judges/rubrics (7.3) → run affected golden sets + planted-defect suites → compare TPR/TNR and task metrics against the pinned baseline → green: release with a dated revalidation stamp; red: asset stays flagged, card to its owner. **Judge-model changes block unsupervised judging until golden-set re-measurement passes** (P-T06-5; judge logic [XREF:S07]). **Every local-tier seat swap mandates threshold recalibration** — model churn invalidates calibration [G3 §Follow-ups; XREF:S12]. Cadence: on-trigger + `⚙ eval.sweep_interval = quarterly` full sweep [R12 §4.7].
- **Engine-bump quality probes [S03.3] run here:** the before/after small-task battery that gates a bump (S14.5) — schema tests catch schema drift; only quality probes catch effort/context-handling drift (P-T02-5).
- **Rubric falsifiability (5.7):** a rubric "works" iff its planted-defect suite catches at least its registered floor, measured per rubric version; the benchmark practice is the outer falsification loop — does rubric-passing work actually win blind pairs? [R12 §4.7].

### S14.9 Retention & compaction (11.1) [G2 Def.11]

- **At run end** (never later): the **run summary** — a compact structured story (objective, stages, tool-call counts, verdicts, decisions, receipts, final state) from deterministic aggregation + the local tier [XREF:S12]. Incremental-per-run is the documented safe shape; bulk summarization over months of trace is the documented failure shape [R12 §4.2].
- **At `⚙ retention.compaction_horizon = 6 months`** (per-user, 13.4): the scheduled compaction pass strips bulky event payloads and transcript copy-asides. **Keep-forever set:** run summaries, verdicts, decisions, receipts, routing records, drift events, benchmark records [G2 Def.11]. The pass logs itself (`retention.compacted`) — the audit trail records its own compaction.
- **11.3 boundary by construction:** the snapshot exporter [XREF:S13] reads an allowlisted view containing only the keep-forever set; raw trace payloads are structurally unreachable from it. The event-log growth watch (P-T07-5) feeds S14.4.

### S14.10 Queryable history (S2.10)

Everything above is filterable and searchable — by project, person, status, date range, worker, domain, lane, run/task id — through three layers; the conversational assistant consumes these layers and nothing else [XREF:S15]:

- **Layer 0 — deterministic views (no model):** every S2.5 cost question (per run/task/project/person/period, budget remainders, burn rate, limit-event history, done-directly figures) is a named SQL view over the ledger/receipts (meter semantics [XREF:S10]). The assistant *selects* views — **it never computes money by generation**.
- **Layer 1 — canned parameterized catalog:** ~30–50 named queries (status, failures, verdicts, routing quality, drift history) with typed slots; the local model classifies intent and fills slots, grammar-constrained [XREF:S12]; answers render with the query named. This is the reliability floor.
- **Layer 2 — open SQL, enabled at v0 [G3 D3.5]:** Arctic-Text2SQL-R1-7B generates against **allowlisted views only**, under the full guardrail stack — read-only connection (`query_only`), single-statement parse rejecting DDL/DML, LIMIT + timeout injection, every generated query audit-logged — and every answer is **flagged lower-confidence** in the UI. Seat and hardware path are [XREF:S12]; TBD-BRINGUP(Layer-2 open-SQL acceptance measurement, G3 Def.8 battery). Canned queries remain the floor; Layer 2 is escalation, never default [R12 §4.8].
- **Indexing (DDL detail is P3's [S02.2]):** generated columns over event-JSON hot fields; a `dotted_order`-style generated column for ordered-tree queries; FTS5 over run summaries, verdict texts, and drift summaries; rollup tables maintained by scheduler jobs. DuckDB read-only ATTACH is the documented later analytics sidecar — not v0 [R12 §4.8].
- **Routing-quality view (S2.6 analyzable-in-aggregate):** a periodic view joins `routing.decided` causes to outcomes (rework rates, verdicts, receipts) so routing itself is auditable over time [R12 §4.1].

---

**Settings introduced (⚙):** (all operator-editable with audit trail per G1 rider 1; auto-adjust only within operator ceilings)

| ⚙ setting | default | clamp / range | ratified by |
|---|---|---|---|
| `obs.sse_keepalive` | 20 s | 15–30 s | R12 §4.3 |
| `watchdog.loop_repeat` | 5 | 3–10 | G2 Def.10 |
| `watchdog.pingpong_cycles` | 6 | 3–12 | G2 Def.10 |
| `watchdog.error_loop` | 3 | 2–10 | G2 Def.10 |
| `watchdog.silence_budget.<run_type>` | = `recovery.dead_after` (5 min) seed | ≥ 1 min; per-type | G2 Def.10; TBD-BRINGUP |
| `watchdog.spend_median_mult` | 3× | ≥ 1.5× | G2 Def.10 |
| `watchdog.spend_arm_days` | 14 d | ≥ 7 d | G2 Def.10 |
| `watchdog.flag_now_target` | 2/day | 1–10 | G2 Def.10 |
| `watchdog.suppress_retune_count` | 2 | 1–5 | G2 Def.10 |
| `watchlist.fetch_fail_streak` | 3 cycles | 2–10 | R12 §4.5 / P-T11-3 |
| `canary.auth_interval` | daily | 4/day – weekly | G1 Def.3 set; P-T17-1 |
| `canary.behavioral_interval` | weekly | daily – monthly | R12 §4.5 `[coordinator-draft]` |
| `retention.compaction_horizon` | 6 months | ≥ 1 month; per-user | G2 Def.11; 13.4 |
| `eval.sweep_interval` | quarterly | monthly – semi-annual | R12 §4.7 |
| `benchmark.sampling_rate` | BENCH-REG §4.1 schedule (100% pre-gate / 25% maintenance) | 0–100% | BENCH-REG §4.1 (rate ⚙; uniformity frozen) |

**Registered-number rule:** the benchmark's frozen values (gate limbs, alarm threshold, done-directly minimum, labels) surface in the settings registry **marked "registered — changing this value requires a re-registration commit"**, with the audit trail linking to the registration hash [BENCH-REG §1; S01.10].

**Known problems owned here:**
- **P-T11-1** — blinding partially fails at household scale → uniform render template + measured blindness; guess accuracy reported beside every win rate (machinery S14.7; obligation BENCH-REG §5).
- **P-T11-2** — the direct arm is un-pinnable → epoch tracker, per-pair model identities, no cross-epoch pooling (S14.7; BENCH-REG §9).
- **P-T11-3** — watcher sources decay structurally → fetcher escalation ladder + fetch-failure meta-watch + migrate-to-feed bias (S14.6).
- **P-T11-4** — observability/eval tooling ownership churn → runners-never-stores, permissive licenses, pins, pre-registered swaps; all records in Sinet's DB (S14.1, S14.5, S14.8).
- **P-T11-5** — small-n honesty is a product surface → honest-claims table + full epistemics on every published rate (S14.7; BENCH-REG §15).

Executed/consumed here for owners elsewhere: P-T07-5 (size watch, S14.4), P-T02-5 (bump quality probes, S14.8), P-T17-1 (auth canary, S14.6), P-T17-3 (model-list diff schedule, S14.6), P-T04-1 (compaction canary home, S14.5), P-T05-2/P-T05-4 (analysis + pre-v1 eval, S14.7), P-T06-5 (judge-change block, S14.8), P-T14-1 (bump revalidation execution, S14.8), P-T03-1 (cache-weighting resolution watch, S14.6), P-T08-3 (price-drift proposals, S14.6), P-T13-1/P-T13-2 (recurring checks, S14.4).

**Deferred / parked:**
- OTLP projector (approach A3) → re-entry: OTel GenAI semconv reaches Stable, or a one-container SQLite OTLP UI matures [R12 §4].
- CodeAD-shaped watchdog rule synthesis → post-v0, as proposal cards [R12 §4.4].
- Miniflux feed backend → re-entry: native poller proves gnarly; webhook contract is the drop-in [G2 Def.12].
- DeepEval swap → re-entry: promptfoo governance drift manifests [R12 §4.7].
- DuckDB ATTACH analytics sidecar → re-entry: query latency becomes real [R12 §4.8].
- Layer-2 SQL on 14–32B (eGPU) → re-entry: T15 stretch bar + hardware present [XREF:S12].
- Blindness-calibration pilot (~10 mock pairs) → v0.1, before member-facing benchmark use [BENCH-REG §5, non-binding note].
- Learned anomaly baselines → only if history floors are met AND fixed thresholds measurably underperform [R12 §5].
- Low-n catastrophic-only gate variant → only via dated re-registration if accrual < ~5 pairs/month [BENCH-REG §11].

**Coverage:** (Scope → subsection)
| feature-list item | subsection |
|---|---|
| 11.1 full audit trail + retention | S14.1, S14.2, S14.9 |
| 11.2 / S2.11 / 15.3 benchmark practice + gate | S14.7 (protocol = BENCH-REG) |
| S2.1 full traceability | S14.1, S14.2 |
| S2.2 / S3.9 live inspection | S14.3 |
| S2.3 verdict records per round | S14.2 (`verdict.recorded`); logic [XREF:S07] |
| S2.4 human decisions recorded | S14.2 (`decision.recorded`) |
| S2.5 cost observability at every altitude | S14.10 Layer 0; meters [XREF:S10] |
| S2.6 / 7.7 routing explainability | S14.2 (`routing.decided`), S14.10 |
| S2.7 / 4.6 / 3.7 self-health watching | S14.4 |
| S2.8 outside-world drift detection | S14.6 |
| S2.9 auditable learning (trace side) | S14.2 (`knowledge.injected`); lifecycle [XREF:S09] |
| S2.10 queryable history | S14.10 |
| 5.6 escalation paths proven by tests | S14.5 |
| 5.7 rubric falsifiability | S14.8 |
| 7.3 model-change revalidation | S14.8 |

**Open items for G4:** none. (Two flagged drafting sub-choices ride their markers: the behavioral-canary cadence `[coordinator-draft]` and the silence-budget seeds TBD-BRINGUP — both operator-tunable settings, not open decisions.)
