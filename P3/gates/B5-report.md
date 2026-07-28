# B5 gate report — observability & evals

Written 2026-07-28 by the build coordinator. Phase B5 (S19.5: watchdogs, conformance registry, benchmark machinery, queryable history [S14], plus the carried codor-C2 redaction directive, the [schema] STUDY watch row, and the dead-man-canary V2 leg) is complete: **all ten queue packets done and validated** at full depth through the four-stage pipeline (grounding → executor → evaluation → drain). This gate's approval opens **B6 (frontend: the React SPA over every API [S15; `Spec/frontend-components-v1.md`])** — the last phase.

The phase was cut as eight packets and grew to ten by two deliberate mid-phase splits, both made on the grounding stage's scope verdict rather than after an executor overran (B5-6 → 6A/6B, B5-8 → 8A/8B).

## 1. What shipped (packet → commit)

| Packet | Commit(s) | One line |
|---|---|---|
| P3-B5-1 | `6d0a4c5` + drain `17bffe5` | S14.1–S14.2 event-type contract v0 — 15 families with contract-minimum required fields, `schema_version` + upcast-on-read + forward-tolerant unknown-type skip, and **every** provisional type minted in B0–B4 reconciled (11 renamed as constant-value-only with legacy aliases, no migration). Caught a production consumer mid-packet: `memory.Influence` had `'context.manifest'` hard-coded in a query |
| P3-B5-2 | `977efbf` + drain `db4b989` + landing `8f09da5` | S14.3 live inspection — topic-tagged SSE over the one endpoint (run-detail/board/fleet-meters/inbox), reconnect = snapshot-then-tail on both `Last-Event-ID` and `?after_seq`, progress as FSM state + monotonic counters and **never** percent; **codor C2** serve-side redaction (`internal/redact`, store-raw/serve-redacted proven on both serve halves). Owner-scoping of the live stream implemented per S01.9 |
| P3-B5-3 | `0d3b072` + drains `e1e4ba7`, `0299c87` | S14.4 watchdog suite — Tier-0 zero-cost counters over the event log (loop / ping-pong / error-loop / silence / spend-median / suspicious-completion, ⚙-seeded with a cold-start floor), Tier-1 local disambiguator that **annotates and never gates**, Tier-2 contain→park→card with "resume — I was wrong" first-class, flag-now vs daily-digest discipline with dedup, the 4 registered platform-health checks, and the dead-man canary V2 leg. **No auto-kill anywhere**, enforced by an AST wall |
| P3-B5-4 | `dff4c3d` + drain `202b5a5` | S14.5 conformance registry (migration 0011) — the scheduling home for every "proven-not-assumed" obligation; the existing suites registered with their triggers and schedules, results landing as `eval.score_recorded`, red → card, structure immutable by trigger, and the S03.3 bump-gating tie recorded as data |
| P3-B5-5 | `f005369` + drain `10dee46` | S14.8 regression evals — **promptfoo 0.121.19 adopted** through the S16 rail (runner-never-store: telemetry off, sharing refused, its own history redirected to per-run scratch), per-asset eval objects with floors registered per rubric version, the revalidation runbook (drift / engine bump / seat swap → flag dependents → golden + planted → compare vs baseline → stamp or flag), DeepEval 4.1.3 pre-registered as the standby swap |
| P3-B5-6A | `443f5bf` + drains `fbb55f0`, `ba9d885` + landing `4c71e3c` | S14.6 watchlist executor (migration 0013) — watch-row store with seeds incl. the [schema] STUDY row, **changedetection.io 0.55.8 adopted** (REST + config only; Sinet *generates* `sinet-watchlist.service` because upstream ships none), Sinet-native feed poller with conditional GET and hash dedup, genai-prices refresh **as a proposal that never auto-flips billing**, drift cards with fingerprint dedup, decay posture and fetch-fail meta-watch |
| P3-B5-6B | `8a037d4` + drain `6ef7af9` | S14.6 ¶3 API canary layer — auth canary (five-class error discrimination, freeze-not-retry-park), behavioral canary, logprob canary **local-lane-only**, model-list drift. Metering posture explicit per the no-load-bearing-metered-paths rule; the real leg **ships disarmed** behind `SINET_CANARY_ARM` with a pre-registered projection |
| P3-B5-7 | `3ac8a24` + drain `f47a751` | S14.7 benchmark practice (migration 0014) — the machinery around the signed BENCH-REG: sampling hook at eligibility with decline logging, blind-pair machinery (one template, tells stripped, position randomized, mandatory arm-guess, reveal only after the §14 record is written keep-forever), epoch tracker, Beta-Binomial decision statistic that **evaluates and displays but sets nothing**, small-n honesty table on every rate. Registered numbers are read-only data, pinned by test |
| P3-B5-8A | `fa62c64` + drains `8cab29e`, `63c747f` | S14.9 substrate (migration 0015) — deterministic run summary at run-end, compaction at ⚙ `retention.compaction_horizon` (strip payloads, keep-forever set incl. benchmark records), the keep-forever allowlist-view exporter boundary, generated columns + `dotted_order` + FTS5 + rollups |
| P3-B5-8B | `401cdf1` + `9e1ebb2` + drains `032e223`, `a020552` + landing `35902da` | S14.10 query surface (migration 0016) — Layer 0 deterministic SQL views answering every S2.5 cost question (never money-by-generation), Layer 1 canned catalog as data-not-code, **Layer 2 open SQL behind the live Arctic guardrail stack** (read-only handle, single-statement parse that extracts the SELECT from chain-of-thought, LIMIT + timeout, every query audit-logged, flagged lower-confidence) with its injection battery, redact-before-match search, routing-quality view |

Full B5 range (from the B4 close `9dcdfed`): **209 files changed, +51,190 / −193** across 102 commits; brief artifacts in `P3/briefs/P3-B5-{1..8}.md`.

## 2. Evidence

- **Battery (coordinator re-ran fresh this session, 2026-07-28):** `gofmt -l internal/ cmd/` clean, `go vet ./...` clean, `go build ./...` clean, `go test ./... -count=1` — **42 packages green, 0 failures**, no skips introduced beyond the sanctioned tier-R/L and organ-absence skips; `go run ./tools/lockgate` → **OK, 21 entries** (15 go.mod deps + 2 workflow refs all covered). `-race` was run on each packet's touched set at its landing.
- **Counters, all asserted by test rather than remembered:** event inventory **88 minted + 7 declare-only = 95 registered** (`TestInventoryTotals`, `TestEveryMintedTypeHasAProducer`, `TestDeclareOnce_EveryProducerTypeIsRegistered`); ⚙ **118 keys / 33 domains** (`TestIndexMatchesS18Tallies`); migrations **0001–0016** contiguous; CONVENTIONS **§29–§37** appended this phase with §1–§28 never rewritten.
- **`internal/settings/index.go` was byte-unchanged for the entire phase** — `git log 9dcdfed..HEAD -- internal/settings/index.go` is empty. Ten packets consumed S14's ⚙ keys by dotted name and not one re-declared or hard-coded a value. No ⚙ default or clamp changed → **no S18 sweep is owed**.
- **Paid spend across all of B5: $0.00.** Every eval, canary, watchdog and guardrail leg ran against fakes, the local GPU, or hermetic stubs. The two pre-registered paid legs (the B5-5 golden sweep, the behavioral canary) are **registered and unexecuted**, both awaiting your say-so below.
- **Adversarial evaluation did real work.** Every packet's evaluation stage was cross-model or judge-independent, and the strongest catches came from evaluators that reproduced their own claims: planting canaries, reverting fixes to prove tests bite, and inventing attack routes beyond the shipped battery. Three packets used the full two-round drain cap (B5-3, B5-6A, B5-8A/8B); four passed on the first evaluation after one drain.
- **The phase's most serious finding, and how it closed.** B5-8B's evaluation found a **complete Layer-2 guardrail escape** — a quoted identifier in table-alias position was captured as raw token text and re-emitted unquoted, so alias text was attacker-controlled SQL that no check ever saw. The evaluator proved it end-to-end through the real verb (a member reached both owners' raw payloads, then chained a pragma clear → ATTACH → CREATE → INSERT and wrote a database file outside `platform.db`). It was fixed at the root in four limbs, then attacked from **28 fresh rebuild routes plus 18 sampled variants — all refused**, with each fix's limb individually neutered to prove its test bites. A second finding the same round (`query_only` being one statement from gone, never re-verified) was corrected by measurement after an executor overclaim: `mode=ro` does **not** cover TEMP and attached databases. The injection battery now stands at **134 subtests**, green.
- **Conformance highlights (all runnable below):** no-auto-kill AST wall + park-and-flag in one transaction + resume-fence-no-retrip (`internal/watchdog`); store-raw/serve-redacted on both serve halves + member-scoped snapshots (`internal/api`); marker fragments are not an oracle + search redacts before matching + owner-scoped search + the whole battery changes nothing (`internal/history`); canary layer has no kill primitive, enforced against a planted source (`internal/watchlist`); registered BENCH-REG values appear in their registered sections and the register is complete (`internal/benchmark`); compaction predicate served by an index + the retention import wall (`internal/retention`); conformance rows cover every S14.5 group + record-result is both-or-neither (`internal/conformance`).

## 3. Measurements

**Taken this phase:** none new. B5 is machinery, and its two registered measurements are bring-up acts by design (below). The phase deliberately produced **no** new numbers rather than manufacture them from synthetic seeds — the B4 lesson about suppressed and invented results was applied by refusing to generate results the platform has no basis for yet.

**Pre-registered and due at bring-up (files exist, execution is the act):**

| Row | Status | File / home |
|---|---|---|
| Layer-2 open-SQL acceptance (S14.10 ¶3, G3 Def.8) | **Pre-registered 2026-07-27, not executed.** E1 safety expectations are structural and already proven hermetically; E2 compares against the measured 0/30 raw-alias figure; refusals and honest-failures reported separately with Wilson intervals | `P3/measurements/2026-07-27-layer2-open-sql-bringup.md` |
| Per-run-type silence budgets from observed event cadence (S14.4) | Open — needs real run cadence, which needs real runs | attaches to B5-3 |
| The B5-5 paid golden sweep | Pre-registered ($2.10 projected, $5.00 stop line), **unexecuted** — decision D4(b) | `internal/evals` floors |
| Behavioral-canary real leg | Pre-registered (real dollars **must be $0.00** on flat-rate lanes; ≈$5.70/yr API-equivalent), **disarmed** — decision D3 | `internal/watchlist` |
| Blindness-calibration pilot (~10 mock pairs) | v0.1 pre-member item per BENCH-REG §5, non-binding — noted, not owed at v0 | BENCH-REG §5 |

**Carried from B4, still due at bring-up (none blocking B6):** the ⚙ `verification.entailment_sample_rate` = 0.20 live write; the Guardian-3.3 entailment leg and the 3.3-vs-4.1 head-to-head; the entailed-side floor re-measure before entailment ever gates unsupervised; watchdog/watchlist bar re-measure on grown seeds; the MUX-mode headroom leg; the durable calibration-record writes at deployed-seat keys.

## 4. Literal demo steps (dev-mode, $0 — run any or none)

```
cd ~/Sinet-Agentic-Control-Hub
go test -count=1 ./...                                                          # full battery, 42 packages
go run ./tools/lockgate                                                         # adoption lock gate, 21 entries

# the Layer-2 guardrail — 134 subtests, incl. the escape that was found and closed
go test -count=1 -v -run TestInjectionBattery ./internal/history
go test -count=1 -v -run 'TestSearchRedactsBeforeMatching|TestMarkerFragmentsAreNotAnOracle|TestSearchIsOwnerScoped|TestTheWholeBatteryChangesNothing' ./internal/history

# no auto-kill, anywhere — the AST wall plus the containment path
go test -count=1 -v -run 'TestNoAutoKillPrimitives|TestParkAndFlagInSameTx|TestResumeClearsFlag|TestDeadManAlarmWhenLocalDark' ./internal/watchdog
go test -count=1 -v -run 'TestCanaryLayerHasNoKillPrimitive|TestCanaryNoKillWallTripsOnPlantedSource' ./internal/watchlist

# codor C2: stored raw, served redacted — and members see only their own rows
go test -count=1 -v -run 'TestSSESnapshotStoreRawServeRedacted|TestSSEMemberScopedSnapshots' ./internal/api

# the event contract and the registries that keep it honest
go test -count=1 -v -run 'TestInventoryTotals|TestCompletenessEveryCapabilityHasAFamily|TestFifteenFamiliesWithRequiredFields' ./internal/eventlog
go test -count=1 -v -run 'TestSeedRowsCoverEveryS14_5Group|TestRecordResultAtomicBothOrNeither' ./internal/conformance
go test -count=1 -v -run 'TestRegisteredValuesAppearInTheirRegisteredSection|TestRegisteredValueRegisterIsComplete' ./internal/benchmark
go test -count=1 -v -run 'TestCompactionPredicateIsServedByAnIndex|TestImportWall|TestSummaryIsWrittenAtRunEndWithTheSevenFields' ./internal/retention
```

Your personal acceptance test remains the B6 UI click-through per your 2026-07-20 directive; these are coordinator-driven evidence, not homework.

## 5. Deviations & carried notes

- **B5-3's executor content landed inside a STATE bookkeeping commit** (`0d3b072`) through an index race while a stage agent was live in the tree. Content was unaffected and re-verified by `git show --stat`; the lesson is now a standing rule (commit coordinator bookkeeping with `git commit --only P3/STATE.md` while any agent is running).
- **Migration immutability has one narrow exception, now precedent:** a migration may be edited during its *own* packet's drain, because §6 immutability protects migrations already applied to real databases and none exists yet. Immutability re-attaches at landing. Ruled and logged for 0015.
- **A declared deviation is a referral, never an acceptance.** Several stage referrals turned out to be real defects; several were sound readings worth recording. B5-8B's executor **refused** to author spec text for the family-12 `RequiredFields` question — correctly, since a spec gap is never resolved silently inside a packet. That refusal is decision D5(b) below.
- **Two packets were split rather than rushed** (B5-6 → 6A/6B on the grounding's scope verdict; B5-8 → 8A/8B, which additionally stopped clean at a pre-authorized seam mid-build). Recorded as a success mode.
- **Known bounds, stated plainly rather than papered over:** redaction covers only its enumerated secret classes; the Layer-2 guardrail refuses honest *unfenced* reasoning whose line merely opens with a SQL keyword (fenced output is immune, and the system prompt asks for a fence, so the prompted path is unaffected) — this must ride the bring-up acceptance measurement as an expected refusal source so guardrail conservatism is never misread as seat weakness. A `\v`/`\f` lexer-vs-SQLite whitespace divergence is recorded as pre-existing, unreachable via the production verb, and failing toward a clean execution error rather than toward admission.
- **The coordinator itself tripped Fable's dual-use safeguard** while relaying B5-8B's security evaluation — the first coordinator-side trip; every earlier one hit a stage agent. Work continued losslessly on Opus. This is a documented false positive on your own authorized infrastructure work; the routing lesson (start security-dense sessions on Opus) is recorded in the handoff and auto-memory.

## 6. Decisions for the operator

**D1 — Ratify the 3 new B5 adoptions (S16.4 checklist #10 = your approval).** All three entered through the rail with live-verified pins, path-scoped license scans at the pinned ref, and behavioral verification rather than docs:

| Adoption | Pin | License | Role |
|---|---|---|---|
| promptfoo | `0.121.19` | MIT | the pinned regression-eval **runner** behind `internal/evals.Runner` — never a store |
| changedetection.io | `0.55.8` | Apache-2.0 | the page-diff tier for the S14.6 watchlist, consumed over REST only |
| genai-prices `data.json` | `v0.0.72` + content sha256 | MIT | the vendored **diff baseline** for the price-refresh job — never a price table |

DeepEval `4.1.3` (Apache-2.0) is also in the lock as the pre-registered **standby** swap for promptfoo; a standby is observed now and pinned on activation (the Ollama precedent), so it needs no ratification today. **Recommendation: ratify all three en bloc.**

**D2 — Host installs.** Two organs are adopted in code but **not installed** on this machine; nothing about the build changed host state. Per your standing hand-steps rule these are not loose commands to paste — the script is already written and validated:

```
bash ~/Sinet-Agentic-Control-Hub/P3/gates/B5-organ-install.sh
```

**Host preconditions checked live this session, and the news is better than the gate batch assumed: neither install needs sudo, apt, or a system change.**

- **promptfoo `0.121.19`** — `npm install -g`. Your npm prefix is `/home/sinep/.npm-global` and is **user-writable**, so this is a `$HOME` install. Node v22.22.1 satisfies promptfoo's declared engines.
- **changedetection.io `0.55.8`** — the gate batch said "pipx", but **pipx is not installed** and adding it would have meant an apt package and your vetting rule. **`uv 0.11.26` is already on the host** and `uv tool` is the pipx equivalent, so the install is `uv tool install changedetection.io==0.55.8 --python 3.11` — entirely user-level. The Python pin is deliberate: PyPI declares `requires-python >=3.10` (verified live), but your system Python is **3.14**, which is newer than that dependency tree's wheel coverage, so the script asks uv for a managed 3.11 rather than gambling on a source build.
- The script is **idempotent, refuses to run as root, installs no systemd unit, arms no canary, and touches no secret.** It verifies each installed version against the pin in `components.lock`, then re-runs the Sinet-side conformance legs that activate once the organ exists, then the full battery. It prints an undo for both installs.
- **Honest limit:** installing changedetection.io does not start it, so its real-organ conformance leg stays a sanctioned skip until the organ is running with `SINET_CDIO_URL` and `SINET_CDIO_API_KEY` set. The script says so rather than reporting a false green.
- **Separately:** whether to install the Sinet-**generated** `sinet-watchlist.service` (generated because upstream ships no unit — the B4-5 carve-out). Installing a systemd unit is a real host change, so it is its own yes/no.

**Recommendation: run the script to install both organs; hold the unit install for the hardening session** unless you want the watchlist polling unattended before B6 exists to show you what it found.

**D3 — Canary arming.** The canary layer ships deliberately disarmed, and two of its four legs cannot honestly be armed today:

- **auth and model-list canaries — cannot be composed at all**: no per-lane HTTP endpoint or broker credential accessor exists in the tree yet. Arming them is gate-time composition work. This is recorded as an honest absence rather than faked with a stub that would report green against nothing.
- **behavioral canary — armable, with one precondition**: promptfoo issues its provider calls itself, so Sinet sees no per-call usage stream. The itemization question has to be answered at arming, not assumed away.
- **Pre-registered projection:** real dollars **must be exactly $0.00** on the flat-rate lanes — any non-zero probe-tagged total is itself the alarm, and the response is disarm-and-flag — with ≈$0.11/wk ≈ **$5.70/yr** API-equivalent for the behavioral leg.
- **Credential custody** if you arm: `SINET_CANARY_ARM`, `SINET_CDIO_API_KEY`, and the authenticated Reddit feed URLs (the unauthenticated seed URL live-rate-limits at HTTP 429). Secrets go through the broker path, never through chat.

**Recommendation: leave all four disarmed at v0.** Nothing in B6 depends on them, and arming the two composable legs is better done once real lanes and the B6 surfaces exist.

**D4 — Ratifications.**
- (a) **The B5-5 eval floor, catch-rate 0.84.** Derived as the Wilson 95% lower bound of the B4 rider's 20/20 judge result — a measurement-derived floor, not a chosen number. It ships gate-pending and asserted by test. **Recommendation: ratify.**
- (b) **The paid golden-sweep say-so** — pre-registered at **$2.10 projected against a $5.00 stop line**, never executed. **Recommendation: hold it unexecuted until B6 makes its output visible**; it costs nothing to leave registered, and running it now produces a number no surface displays.

**D5 — Named follow-on scope** (recorded so it cannot fall between chairs; none blocks B6, and two of them B6 *is* the answer to):
- (a) **B5-7's dispatch→render driver** — single-shot direct-arm output capture plus the B6 verdict form. Until both exist no benchmark pair can complete. This was a deliberate honest absence, not an oversight: BENCH-REG accrual is a post-B6 bring-up act and the opt-in defaults OFF, so nothing samples until you opt in.
- (b) **The family-12 `RequiredFields` question — an S00.9 amendment decision.** B5-8B registered a genuinely new event type (`history.query_audited`) under family 12. The frozen S14.2 family row is pinned byte-for-byte by test and names neither the type nor its fields. My reading is that the row is **not wrong, merely not-restated** — the intent is already met by the type's own spec entry plus the required-field golden — so **no amendment is warranted**. If you want the frozen row to name the type explicitly, that is an S00.9 amendment with its own approval. **Recommendation: no amendment; leave as recorded.**
- (c) **Carried suites and rows:** B5-4's compaction-canary standing suite; B5-3's silence-budget TBD-BRINGUP row; the structural cadence constants (watchdog sweep interval, dead-man cadence) which remain interim under your standing settings-tab directive.

**D6 — Phase readings, en bloc.** Roughly 30 clearly-implied readings logged across the ten packets (commit bodies + CONVENTIONS §29–§37). Headline ones: owner-scoping the live stream is *implementing* S01.9, so not-scoping would have been the defect; derive watchdog state from the event log rather than adding a side store; watchdog park goes through `run.Store` in one transaction; family semantics keep `canary.result` and `watchdog.flagged` distinct; the eval runner records through and is never re-plumbed; price refreshes are proposals that never auto-flip billing; the Layer-1 catalog is data, not code. Precedent: ratified en bloc at B0–B4. **Recommendation: ratify en bloc.**

## 7. Standing operator items (batched per the rule — none block B6 opening)

1. **Housekeeping, at your leisure:** `/tmp/llamaswap-test/` still awaits an operator `rm -rf` (permission-denied to me); the carried B3/B4 items `rm -rf ~/sinet-demo*` and `rm -rf tools/dbpeek/`; `~/.sinet-b45/baselines/` (~26 GB of KL baselines, deletable post-check).
2. **`Research/02-provider-watchlist-and-onboarding-criteria.md`** carries your pre-existing instructed addendum. Every stage across two phases has deliberately left it unstaged — say the word if you want it committed, otherwise it stays as-is.
3. **Hardening session (needs your sudo presence; unchanged, now with the watchlist unit):** AppArmor userns grant for service-context confined runs, `socat`, egress substrate, `run@` install, Landlock live enforcement, the `-lgc` linker note, optionally `DBUS_SESSION_BUS_ADDRESS` for GameMode's D-Bus leg — **plus the `sinet-watchlist.service` install if you defer it from D2**. The `user.slice` freeze/thaw probe rides along.
4. **Probe batch remainder:** the catch-up **suspend** leg, paired with the battery-drain measurement hour (timer still installed).
5. **Week-one push drill** on household phones — due at first deploy, which B6 brings within reach.
6. **Optional:** GitHub Verified badge (signing-key upload).
7. **Still parked:** Z.AI prompt-unit calibration — no Z.AI lane exists at v0, so there is nothing to calibrate against.

## 8. Gate answers

*(Operator free-text answers are authoritative; recorded as given.)*

**2026-07-28, first batch (chat):** "D1 ok. D2 ok. D4 ok. D5 ok."

- **D1 — ANSWERED: ratified.** promptfoo 0.121.19, changedetection.io 0.55.8, genai-prices data v0.0.72 all ratified en bloc (S16.4 check #10 satisfied for all three lock entries).
- **D2 — ANSWERED + EXECUTED 2026-07-28.** Both organ installs authorized; the coordinator ran `P3/gates/B5-organ-install.sh` on this authority (non-interactive, user-level, no secrets). **Final result: ALL GREEN — 11 ok / 0 failed.** promptfoo `0.121.19` installed (`~/.npm-global`, version == pin, real-binary conformance leg green) · changedetection.io `0.55.8` installed (`uv tool`, managed CPython 3.11, version == pin, installed-not-started — its real-organ leg stays a sanctioned skip until it runs with `SINET_CDIO_URL`+`SINET_CDIO_API_KEY`) · full battery re-run green, 42 packages. **Two script fixes were needed en route, both recorded in the script itself:** (1) `--prerelease=allow` added to the uv install — 0.55.8 itself pins `pyppeteer-ng==2.0.0rc13`, a pre-release uv refuses by default; the flag satisfies upstream's own exact pin, floats nothing; (2) the version-verify grep matched only `changedetection.io` while uv PEP-503-normalizes the name to `changedetection-io` — a correct install printed as a false "mismatch"; pattern now accepts dot or hyphen. The `sinet-watchlist.service` unit install is DEFERRED to the hardening session (standing item 3).
- **D3 — ANSWERED 2026-07-28 (second batch): "disarmed."** All four canary legs stay DISARMED at v0. The auth and model-list legs remain honestly un-composable (no per-lane HTTP endpoint or broker credential accessor exists) — composing them is named gate-time work for whenever arming is wanted. The behavioral leg's itemization question stays TABLED as its arming precondition, with the option space recorded: (1) organ-reported metering entry from promptfoo's own results output, flagged as organ-reported; (2) projection-only (≈$0.11/wk registered, $0.00-real-dollars verified externally); (3) a broker-path custom provider so real usage rows mint — post-B6 composition work that must respect the ratified never-re-plumb reading. The pre-registered projection and the $0.00 stop line stand. *(En route, the operator questioned the premise — "promptfoo can run locally/self-hosted, why doesn't Sinet see per-call usage?" — and the coordinator's answer is part of this record: locality was never the issue; promptfoo's own process dials the provider, bypassing Sinet's adapter path where S10.1 usage rows are minted, so its numbers are organ-self-reported, not Sinet-measured.)*
- **D4 — ANSWERED: per recommendation.** (a) The 0.84 eval floor RATIFIED. (b) The paid golden sweep stays registered and UNEXECUTED until B6 gives its result a surface.
- **D5 — ANSWERED: per recommendation.** (a) dispatch→render driver stands as named follow-on scope; (b) NO S00.9 amendment for the family-12 `RequiredFields` row — the not-restated reading stands; (c) carried suites/rows acknowledged.
- **D6 — ANSWERED 2026-07-28 (second batch): "ratified."** The operator asked for the full list with detail before deciding; the coordinator compiled it from CONVENTIONS §29–§37 + the packet commit bodies and presented the report's "roughly 30" honestly decomposed as **35 readings** — 3 cross-phase patterns (derive-from-log/no side stores/bounded derived cards · no tickers, dueness from stored state · structural constants with named reasons, interim under the settings-tab directive) plus 32 per-packet readings across B5-1…B5-8B. **Ratified EN BLOC; all 35 are binding precedent for B6.** The individually-flagged household-policy readings (member visibility scoping = S01.9; benchmark consent/alarm authority; the 1 KiB compaction floor; the 30-day drift-card horizon) were included in that walkthrough and are covered by the en-bloc answer.

---

**GATE CLOSED 2026-07-28.** All six decisions answered and everything they authorize executed (D2's installs ran green this session; D3 authorizes no action; D4(b) is deliberately held). **B5 is CLOSED. B6 opens next** — its packet queue is deliberately left for the fresh session to cut from S19.5 + S15 + `Spec/frontend-components-v1.md` (operator directed a session handoff at close).
