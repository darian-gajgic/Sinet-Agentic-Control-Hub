> **EXPIRED at landing (2026-08-25). Single-use artifact: after the r1/r2 drains this brief no longer matches the code. Later grounding must never read it as truth — code + spec only.**

# P3-LN-4 — commissioned-lane activation: a placed key becomes a routable lane

Grounded 2026-08-24 against `Spec/drafts/` (canonical, spec-v1), `P3/CONVENTIONS.md` §10/§61–§64, and the tree at `b842267`. Single-use: this brief expires when the packet lands.

**Every earlier brief in `P3/briefs/` is stamped EXPIRED and lies.** Code and spec are the only truth; every line below was read off the tree in this session and carries its file:line.

---

## 0. The verdict, in one line of Go

`internal/shell/shell.go:561`:

```go
engineCommissioned := map[string]opencode.ProviderConfig{}
```

That is the whole gap. The comment three lines above it (`shell.go:556-560`) already names its own consequence: *"The map is the seam that ceremony fills, and it feeds BOTH the adapter registration and the credential injector, so a lane can never be dispatchable without its credential path."* Nothing fills it. So the ceremony can place a real credential in a real broker store, `lanekey verify` can print **COMMISSIONED**, the tier-L smoke can make a real paid call — and the control plane will still route no work to that lane, because coverage, the lane→substrate map and the alternate seats are all derived from this map and this map is empty.

The key ceremony is honest about it in operator words today (`P3/gates/lane-key-ceremony.sh:419-427`, step 8: *"still constructed EMPTY at v0 … the lanes are credentialled and the control plane will still not route work to them"*). **When this packet lands, that same honest paragraph becomes a lie in the other direction**, so updating it is a requirement here (R12), not a courtesy.

The operator has ordered this closed **before** the key ceremony runs. Nothing in this packet places, reads, or holds a credential.

## 1. Scope wall

Backend only. `web/src` untouched, no route added, no UI surface, no running world touched.

**$0 absolutely.** No live provider call in any test or code path. No real key anywhere: test worlds seed a **fake** credential into a **test-scoped** store (`t.TempDir()`), never the operator's. Tier L stays behind `SINET_LIVE_SMOKE=1`; the canary legs stay behind `SINET_CANARY_ARM` and this packet does not touch that decision.

**Not ours to edit:** `internal/adapters/opencode/lanedata/*.json` (the lane documents), the classifier fixtures, `Spec/`, any adopted engine. **Ours to edit:** `P3/gates/lane-key-ceremony.sh` (step 8 only).

Operator directives that bind execution: tests run **serial** (`go test -p 1`), never two batteries at once, reap orphans; the disk is ~90% full so **pull nothing** — no new module, no `go get`, `components.lock` untouched.

---

## 2. Requirements

Every requirement carries its `Spec/drafts/` S-ref or its `P3/CONVENTIONS.md` §-ref.

### R1 — the commissioned map is FILLED at control-plane startup, from what is placed
[S03.6 ("Adding a lane is a provider entry per user plus billing flags"); S11.5 (the credential lives in the broker, outside everything else); CONVENTIONS §62 ("A lane is CONFIGURED by a document and COMMISSIONED by a credential, and the two are different states")]

At startup the control plane composes `engineCommissioned` as: **for every person who has a broker store, and every shipped lane document whose credential is actually placed in that person's store under the document's own auth profile — that lane's provider entry, under that person's key.**

The map is `map[string]opencode.ProviderConfig` = person → (providerID → entry). It replaces the empty literal at `shell.go:561` and must be composed **before** `engineAdapters(...)` at `shell.go:562`, because four of its five consumers snapshot it rather than read it live (R2).

### R2 — one map, one lane load, five consumers — and four of them snapshot
[S03.2 (registration); S08.8 step 3 (coverage); S11.5 (spawn-time injection); CONVENTIONS §63 D7]

The map feeds exactly five sites, all in `shell.go`:

| line | consumer | reads the map… |
|---|---|---|
| 566 | `engineAdapters(...)` → `Adapter.ProvidersFor` closure (`engineadapters.go:126-128`) | **live**, per call |
| 598 | `commissionedLanes(...)` → `stage.Config.CommissionedLanes` | **snapshot** at composition |
| 601 | `laneSubstrates(...)` → `stage.Config.LaneSubstrates` | **snapshot** |
| 604 | `laneAlternateSeats(...)` → `stage.Config.AlternateSeats` | **snapshot** |
| 621 | `laneCredInjector(...)` → `stage.Config.CredInject` | **snapshot** of the map, live per spawn for the secret |

Because four snapshot, a map mutated after `stage.New` produces a control plane where the adapter believes a lane is held and the router has never heard of it. **Compose once, before line 562, and never mutate afterwards.**

While here: `shell.go` calls `engineLanes(logger)` **five separate times** (inside `engineAdapters`, then at 598, 601, 604, 621), each re-reading and re-validating the embedded documents. Hoist it to one `engineLanes` value used by all five and by the commissioning read. This is not cosmetic — the commissioned map and the lanes it was derived from must be the same lane set, or a document that stops loading between two calls desynchronises them silently.

### R3 — presence detection is a SECRET-FREE read
[S11.5 ("a policy-decision/credential-delivery split kept as an internal code boundary (the decision path holds zero secrets)"); the broker's own `kindOf` (`internal/broker/store.go:231-251`)]

The question is *"is a credential placed under this profile, and is it an engine-cred?"* — a question about a **kind**, never about a secret. The read MUST:

- read the record's **plaintext `kind`** field only (`profileRecord.Kind`, `store.go:89-93` — plaintext by design, "not a secret");
- **never decrypt** (`Store.resolve` / `Client.Resolve` are out of bounds);
- **never create the master key** — which rules out `broker.OpenStore` outright: it `MkdirAll`s the dir, `Chmod`s it, and **generates `master.key` if absent** (`store.go:100-117`, `loadOrCreateMaster` at `119-151`). A control plane that called it would mutate the host on every start with nothing commissioned, and would hold a decrypting `*Store` it has no business holding;
- **never write** anything.

This is the same posture the tier-L predicate already takes (`internal/adapters/opencode/live_smoke_test.go:68-100`, `placedEngineCreds`) — and that predicate lives in a **test file**. Production needs the same question answered in a production home; **do not import from a `_test.go` file and do not copy it a third time.**

**Recommended home: `internal/broker`.** The package owns the store layout (`<root>/<user>/<profile>.cred`) and the record shape, so a reader that mirrors both from another package is a second definition waiting to drift. Export a read-only pair — names are the executor's, e.g.:

```go
// StoreRoot reports the per-person store root under a state dir. Mirrors the
// `sinet broker` default (mode.go:53) — the ONE place that default lives.
func StoreRoot(stateDir string) string

// PlacedKinds reports profile → plaintext kind for one person's store, without
// decrypting anything, creating anything or writing anything. A missing store
// is not an error: it is a person with nothing placed.
func PlacedKinds(root, user string) (map[string]string, error)

// StorePeople lists the people who have a store under root (sorted).
func StorePeople(root string) ([]string, error)
```

Then `internal/shell` composes lanes × people from those, and `live_smoke_test.go`'s local mirror can be reduced to a call (optional, but it is the whole reason to put the reader here).

### R4 — the identity the map is keyed by must be the identity injection dials
[D2 (credentials are per-PERSON facts); S11.5 (per-user UDS, SO_PEERCRED)]

`laneCredInjector` dials `brokerSocketFor(stateDir, userID)` = `<stateDir>/broker/<userID>.sock` (`engineadapters.go:62-64`), and `Adapter.ProvidersFor(userID)` indexes the same map. The broker daemon derives **both** its socket name and its store directory from one `who` (`internal/broker/mode.go:43-54`). **Therefore the map key must be the broker's `who`** — the same string that names the store directory — or the injector dials a socket for a person the store knows nothing about.

**⚠ This is the packet's one genuine spec ambiguity — flag it to the gate, do not resolve it silently.** See §5.

### R5 — provider entries are composed from the lane DOCUMENTS, never hand-built
[S03.6 (lane additions are data); CONVENTIONS §62 ("No endpoint and no model id is a Go constant in a non-test file")]

The entry is `LaneConfig.Providers()` / `LaneConfig.ProviderEntry()` (`internal/adapters/opencode/lane.go:629-651`), which already renders the endpoint, the model map and `options.apiKey = "{env:<VAR>}"` — the **variable name, never a value**. Nothing in this packet may construct a `ProviderEntry` literal, name an endpoint, or name a model id in a non-test file. The existing no-lane-constants source scan covers `internal/shell` (CONVENTIONS §63 D5, §64) and must stay green.

### R6 — a lane that cannot be commissioned is skipped, by the same test the injector uses
[S11.5; CONVENTIONS §62 ("both gates fail OPEN if left optional")]

`laneCredInject` refuses to build an injector when `lane.Credential.Profile == ""` or `lane.Credential.EnvVar == ""` (`engineadapters.go:146-151`). Commissioning must use **the same conjunction**: a document declaring no profile or no variable is not commissionable, so it never enters the map. Otherwise a lane is registered as held, routing seats it, and the spawn produces an engine that authenticates as nobody — the exact failure `ErrLaneNotCommissioned` (`lower.go:179`) exists to make visible.

Only `KindEngineCred` counts (`broker.KindEngineCred`, `store.go:50`). A `signing-key` or `git-ssh-key` sitting under a lane's profile name commissions **nothing** — the destination constraint is a S11.5 invariant, not a convention.

### R7 — no placed key ⇒ byte-identical behaviour
[CONVENTIONS §63 ("empty at v0, which means Coverage is byte-identical to the pre-LN-2 single-lane world"); S08.8]

With nothing placed the map stays empty and every derivation is exactly what it is today: `commissionedLanes` → none, `laneSubstrates` → `nil`, `laneAlternateSeats` → no execution alternates, `laneCredInjector(who)` → `nil` for every person, `ProvidersFor(who)` → empty. **An acceptance test proves this path explicitly** (T4, already committed and green — see §7). An absent store root is *not an error*: it is a person, or a host, with nothing placed.

### R8 — a commissioned lane is SELECTABLE, proven through the REAL dispatch path
[S08.8 steps 1–3; S03.2; CONVENTIONS §63 drain r2 R1]

The §63 lesson is quoted here because it is the exact trap this packet can fall into:

> **"A correct function reached through a dropped argument is a broken feature."** … **"A test of the resolver in isolation cannot see a dropped argument"** — so both guards drive the real call sites through recording adapters and assert which engine was reached, and both were mutation-verified: revert either fix and the guard fails.

So: coverage growth, seat activation and lane→substrate dispatch must be asserted **through the production composition and the real call sites** — `internal/stage`'s `substrateFor` consumers (`runner.go:159`, plus the helper-spawn and revise sites §63 r2 found dropping the lane), against `recordingAdapter` (`internal/stage/lanesubstrate_ln2b_test.go:177-193`). A test that only calls `commissionedLanes(lanes, map)` with a hand-built map proves nothing about whether the *fill* reaches routing.

### R9 — the credential still travels the S11.5 spawn-time path, and only that path
[S11.5; S01.6 ("engines receive credentials at start"); CONVENTIONS §62]

Commissioning changes **who gets an injector**, never **how the secret travels**. Resolution stays per-spawn inside the closure (`engineadapters.go:95-105`); the compiled config still names `{env:VAR}` and never a value; the containment property (scan every event payload, `run_events` row, park record, instance identity key, ops log and the whole SQLite file for a sentinel) must be re-run for a lane commissioned by the **new** path, not merely inherited from LN-2's hand-built map.

### R10 — commissioning is observable at startup, and says nothing secret
[S14.1–S14.2 (ops logging); S11.5]

One INFO line at the composition root naming what got commissioned: person count, lane names, and per person which lanes. Auth-profile names and env-var names are **not** secrets (they ship in the lane documents) and may appear; nothing else may. A control plane that silently commissions is one where an operator cannot tell a placed key from a typo'd profile name.

### R11 — commissioning is STARTUP-BOUND, and no reload seam is invented
[CONVENTIONS §61(a) ("Config is startup-bound … a change to any STARTUP-BOUND part of the invocation RESTARTS the user's instance"); R2 above]

**There is no existing rescan/reload seam that applies, and this brief names none.** The two `Reload`/`Rescan` symbols in the tree are unrelated: `project.Store.Rescan` (`internal/project/onboard.go:231`) re-scans an onboarded project's repo, and `metering.LivePriceTable.Reload` (`internal/metering/pricestore.go:380`) re-composes the price table. Neither has anything to do with adapters, provider entries or the broker.

So the truth this packet ships is: **a key placed while the control plane is running is picked up on its next start.** Do not build a watcher, a SIGHUP path, or a periodic re-scan — R2's four snapshot consumers mean a live refill would produce a half-commissioned control plane, which is worse than a restart.

### R12 — the ceremony's step-8 summary is updated to the new truth
[This packet's own consequence; the script is ours to edit per §1]

`P3/gates/lane-key-ceremony.sh` step 8 (`do_summary`, lines 400-448) currently tells the operator, correctly-for-today:

> *"A lane is COMMISSIONED by a credential and made SELECTABLE by a provider entry. This ceremony does the first. The second lives in the control plane's commissioned map (internal/shell/shell.go), which is still constructed EMPTY at v0. Until a packet fills it from what is placed, the lanes are credentialled and the control plane will still not route work to them."*

Replace it with the truth after this packet, and the replacement **must** carry the restart: placing a key commissions the lane **at the control plane's next start**; a control plane already running when the key was placed will not route to it until restarted. Also retire LN gate-batch item 3 (lines 443-444, *"Fill the control plane's commissioned map from what is placed"*) — this packet is that item. Items 1, 2, 4 and 5 stay.

The tier-L note at lines 425-427 (*"the tier-L smoke in step 4 goes around that map on purpose"*) stays TRUE and should stay: the smoke drives the adapter directly and still proves the key rather than the routing.

### R13 — no ⚙ setting is added, and nothing tunable becomes a constant
[S18; CONVENTIONS §61/§62/§63/§64 ("⚙: none added", standing tally **118 keys / 33 domains**)]

Commissioning consumes **no** ⚙ key and adds none. There is no number in it: the inputs are the state dir (already a flag/`$STATE_DIRECTORY`), the shipped lane documents, and the contents of the broker store. The S18 tally stays 118/33 and the S18.3 data-surface count stays 3 — **assert both against the spec text rather than assuming**, per the §64 precedent.

If an executor finds itself wanting a knob (a lane allow-list, a "commission nothing" switch, a store-root override), that is an **S00.9 amendment adding an S18 row**, and it is an OQ for the coordinator — never a constant, never an env var minted here. The one env name this tree sanctions for opt-in test tiers is `SINET_LIVE_SMOKE` (CONVENTIONS §10) and no second one is minted.

### R14 — explicit NON-GOALS
Each of these is a real thing somebody could reasonably build here. None of them is this packet.

- **The C5 no-household-personal-data rider stays RECORDED-NOT-ENFORCED.** It rides `lanedata/kimi.json` as `data_policy` with `enforced: false` (CONVENTIONS §64). There is still **no per-lane data-policy enforcement point anywhere in the tree**, and inventing one is out of scope — the routing-policy seam is a **gate item**. Commissioning the kimi lane does not change that, and must not be described as if it did.
- **No per-lane data-policy enforcement invented**, and no `routing.decided` field added for it.
- **No per-person coverage.** `commissionedLanes` is deliberately the **union** across people, with the honest over-approximation recorded at `engineadapters.go:181-184`; per-person coverage arrives with the per-person duty-map surface (1.10, B6/v1). Do not narrow it here.
- **No seat for planning, judge or critic.** `laneAlternateSeats` seats **execution only** and that is ratified (CONVENTIONS §63: nobody has measured a zai or kimi model against the S07.5 capability bar; seating one would be inventing a ratification). Commissioning must not widen the duty set.
- **No S11.5 injection proxy.** It stays deferred to the D6/host batch (OQ-1), recorded on `opencode.CredentialInjectionFacts` (`lane.go:597-621`).
- **No new dependency, no adopted-code modification, `components.lock` untouched.**
- **`tools/lanekey` is not a dependency of this packet.** It is the ceremony's placement helper, committed as **UNFINISHED and unreviewed** (`6c170c9`), and it uses `broker.OpenStore` because placing a key must encrypt. Production commissioning must not import from `tools/`, and must not copy that posture (R3).

---

## 3. Seams to respect, and the stubs for phases that have not come

| seam | status | what this packet does |
|---|---|---|
| `opencode.Adapter.ProvidersFor(userID)` (`lower.go:222-226`) | live, per-user | fill it via the map; do not change the signature |
| `broker.EnvInjector` (`client.go:162-178`) | live | unchanged; still resolves per spawn |
| `broker.Client.HasKey` (`client.go:129`) | live, socket-borne | **considered and NOT the recommended read** — see §5 |
| `stage.Config.LaneSubstrates` validation (`skeleton.go:99-103`) | live | a commissioned lane naming an unregistered substrate is now a **startup refusal**, where the empty map made it unreachable. Every shipped document names `opencode`, which IS registered — but the refusal is now on a live path and the executor must know it |
| `worker.Router.Pressure` / `chooseFlatLane` (`routing.go:738`) | live since §63 | becomes reachable for the first time with two lanes actually covered; do not modify it |
| S11.5 injection proxy | **deferred** (OQ-1, D6/host batch) | stub: the doc comment on `opencode.CredentialInjectionFacts` stays the record |
| per-person duty map / per-person coverage | **B6/v1** | stub: the union, with its recorded reason (`engineadapters.go:181-184`) |
| routing-policy seam for the C5 rider | **gate item** | stub: `data_policy.enforced: false` on the lane document, unchanged |
| `SINET_CANARY_ARM` real-request legs | **operator gate decision** | untouched |

---

## 4. Files expected to change

| file | change |
|---|---|
| `internal/broker/store.go` (or a new `internal/broker/placed.go`) | the read-only placement reader + store-root helper (R3) |
| `internal/shell/shell.go` | replace the empty literal at 561 with the composed map; hoist the single `engineLanes` value (R1, R2); the startup INFO line (R10) |
| `internal/shell/engineadapters.go` | the commissioning composer + its doc comment; correct the now-stale `Commissioned` field comment at 37-40 (*"EMPTY at v0, and empty is the honest state"*) and the file-head comment at 12-16 |
| `internal/shell/lanecommission_ln4_test.go` | **already committed with this brief** — two RED, two GREEN (§7) |
| `internal/broker/*_test.go`, `internal/shell/*_test.go`, `internal/stage/*_test.go` | the acceptance tests of §7 |
| `P3/gates/lane-key-ceremony.sh` | step 8 only (R12) |
| `P3/CONVENTIONS.md` | a new §65 recording what landed, in the §61–§64 house style |
| `internal/adapters/opencode/live_smoke_test.go` | *optional* — reduce the mirrored predicate to a call once the broker exports one (R3) |

**Not expected to change:** `lanedata/*.json`, classifier fixtures, `web/src`, `components.lock`, `Spec/`, `internal/worker/routing.go`, `internal/stage/skeleton.go`.

---

## 5. The one genuine ambiguity — flagged, not resolved

**Two person-namespaces exist in this tree and nothing reconciles them.**

- The broker's `who` — the store directory name and the socket name — is an **OS-level** person name: `mode.go:83-88` `currentUser()` returns `user.Current().Username`, and the ceremony places under `STORE_USER="$(id -un)"` (`lane-key-ceremony.sh:66`). `buildAcceptSurface` independently uses `os.Getenv("USER")` with a `"sinet"` fallback (`shell.go:735-742`).
- The **platform** person id is `auth.User.ID` (`internal/auth/auth.go:123-130`), which is what `runs.user_id`, `Adapter.ProvidersFor(userID)` and `laneCredInjector(userID)` carry.

At v0 — one operator, dev posture — these coincide, which is why nothing has broken. With the "small trusted household" the project exists for, they need not. **Nothing in the spec settles which namespace the broker store is keyed by**, and this brief does not invent an answer.

**Executor instruction:** key the map by the **broker `who`** (R4) — it is the only choice that makes injection dial the right socket — and make the mismatch *visible* rather than silent: the R10 startup line names the commissioned person strings, so an operator can see `sinep` where the platform expects `alice`. Note that `auth.New` is constructed at `shell.go:944`, **383 lines after** the commissioning point, so a cross-check against the person rows is not available at this point in the composition order without hoisting the auth store — which is itself a decision, not a detail.

**→ OQ1 for the coordinator (§8).**

### The second decision: file read vs. socket dial

Both are secret-free and both have a production precedent.

- **File read** (recommended): read `<stateDir>/broker-store/<who>/<profile>.cred` and take its plaintext `kind`. Precedent: the broker's own `kindOf` and the tier-L predicate. **Robust** — it answers the same way whether or not the daemon is up.
- **Socket dial**: `broker.Dial(<stateDir>/broker/<who>.sock)` then `HasKey(profile)`. Precedent: the signing-posture seam (`accept_seams.go:78-86`). **Respects a `--store-dir` override** the control plane cannot otherwise see (`mode.go:32`), and never mirrors the record shape.

The recommendation is the **file read**, for one reason: commissioning is startup-bound (R11), and a startup-time dial makes a transient broker outage **silently uncommission every lane** — a control plane that routes differently depending on whether a daemon answered a probe once is worse than one that reads the truth off disk. The socket seam's own precedent dials **per call**, lazily, which is a different thing entirely.

The cost is stated rather than hidden: the file read mirrors `mode.go`'s **default** store root, so an operator running the broker with a non-default `--store-dir` gets a control plane that sees nothing. The ceremony computes the same default the same way (`lane-key-ceremony.sh:65`), so the dev posture agrees — and R3's `StoreRoot(stateDir)` helper makes that default exist in exactly one place instead of three.

---

## 6. ⚙ settings discipline

Read by dotted registry name, as before, by code this packet does not touch: `limit.probe_interval_max`, `limit.retry_cap`, `limit.retry_budget_ratio`, `budget.background_window_fraction`, `pressure.cache_read_weight`, `canary.auth_interval`, `canary.behavioral_interval`, `watchlist.fetch_fail_streak`.

**Consumed by this packet: none. Added by this packet: none.** S18 tally stays **118 keys / 33 domains**; S18.3 data surfaces stay **3**. Both asserted, not assumed (§64 precedent). Any new structural constant must be flagged to the gate under the `sseBatchSize`/`cancelGrace` precedent (§7/§9/§11/§61) — and this packet is expected to introduce **none**.

---

## 7. Acceptance tests — written before the implementation

### Already committed with this brief

`internal/shell/lanecommission_ln4_test.go`. Run: `go test -p 1 -run 'TestLN4' ./internal/shell/`

| test | state at commit | why |
|---|---|---|
| `TestLN4CommissionedMapIsFilledFromPlacedCredentials` | **RED** | `shell.go` still contains the empty literal, and no non-test source in `internal/shell` names the broker store. Fails for the feature's absence, not a compile error. |
| `TestLN4CeremonyStep8TellsTheNewTruth` | **RED** | the ceremony still carries all three stale sentences and never mentions a restart (R12). |
| `TestLN4PresenceReadNeverOpensTheBrokerStore` | **GREEN, must stay green** | bans `broker.OpenStore`, `broker.NewServer` and `.Resolve(` from `internal/shell` non-test sources (R3). Vacuous today, load-bearing the moment the reader lands. |
| `TestLN4NothingPlacedIsInert` | **GREEN, must stay green** | the R7 empty-path pin over the seed documents. |

These are structural because the behavioural ones below cannot compile against a producer that does not exist, and a compile error proves nothing about behaviour. **They are the floor, not the rubric** — T5–T12 are the packet.

### To be written by the executor

Names are prescriptive; the assertions are the contract.

**T5 · `TestLN4PlacedCredentialCommissionsTheLane`** — `internal/shell`.
*Setup:* `stateDir := t.TempDir()`; build `<stateDir>/broker-store/me/` **by hand** (0700) and write `<profile>.cred` containing `{"kind":"engine-cred","nonce":"","ct":""}` for the zai document's profile — **a fake record with no secret in it at all**; the presence read never decrypts, so it never needs one.
*Assert:* the composed map has exactly one person `me`; `map["me"]` has exactly one entry keyed by the zai `provider_id`; the entry's `options.baseURL` equals the **document's** `base_url` and its `options.apiKey` is `"{env:<the document's env_var>}"` — never a value; the kimi lane is **absent** because nothing was placed for it.

**T6 · `TestLN4NonEngineCredNeverCommissions`** — `internal/shell`.
*Setup:* same, but the record's `kind` is `"signing-key"`, then `"git-ssh-key"`, then a garbage string, then a malformed JSON body, then a `.cred`-less file name.
*Assert:* the map is **empty** in every case, and no error escapes to the caller for the malformed/garbage cases — a corrupt record is a lane that is not commissioned, never a control plane that will not start. [S11.5 destination constraint]

**T7 · `TestLN4PresenceReadIsSecretFreeAndSideEffectFree`** — `internal/broker`.
*Setup:* a store dir seeded by hand with one engine-cred record and **no `master.key`**. Snapshot the directory listing and every file's mtime + mode before the read.
*Assert:* the read succeeds; `master.key` **still does not exist**; the listing, mtimes and modes are **byte-identical** after; no plaintext secret is returned by any exported signature (the reader returns kinds, not secrets — assert by type). Then repeat with a **non-existent** root and assert a clean empty result, not an error.

**T8 · `TestLN4CommissioningIsAProperty`** — `internal/shell`, **property-based**, over the spec invariant *"a placed engine-cred is a commissioned lane, and nothing else is"* (S03.6 + S11.5).
*Setup:* for N pseudo-random draws (fixed seed, so failures are reproducible): pick a random subset of (person, lane) pairs from `{p1,p2,p3} × seedLanes`, and a random kind per pair from `{engine-cred, signing-key, git-ssh-key, ""}`; materialise exactly those records in a test-scoped store.
*Assert:* the composed map's set of `(person, providerID)` pairs equals **exactly** the subset whose kind was `engine-cred` **and** whose lane document declares both a profile and an env var (R6) — no more, no fewer, for every draw. This is the invariant an example-based test can only sample.

**T9 · `TestLN4CommissionedLaneIsDispatchedThroughTheRealCallSites`** — `internal/stage`, extending `lanesubstrate_ln2b_test.go`'s `recordingAdapter` (`:177-193`).
*Assert:* a stage composed from the **filled** map dispatches an execution-seated zai/kimi decision to the `opencode` recording adapter and an anthropic decision to `claude-cli`; run the **same** assertion through the helper-spawn and revise sites, which are the two §63 drain r2 R1 found dropping the lane. **Mutation-verify:** revert the fill and this test must fail. A test that only asserts `commissionedLanes(lanes, handBuiltMap)` does **not** discharge this. [CONVENTIONS §63 drain r2 R1]

**T10 · `TestLN4CoverageAndSeatsGrowOnlyFromPlacement`** — `internal/shell`.
*Assert:* with a placed zai credential, `commissionedLanes` = `[zai]`, `laneSubstrates["zai"]` = `"opencode"`, `laneAlternateSeats[DutyExecution]` carries exactly the zai document's `default_model`, and `laneCredInjector(stateDir, lanes, m)("me")` is **non-nil** while `("someone-else")` is **nil**. With zai **and** kimi placed: two lanes, sorted, two seats. Planning/judge/critic duties gain **nothing** (R14).

**T11 · `TestLN4CredentialContainmentOnACommissionedLane`** — `internal/shell`, extending `lanecred_ln2_test.go`'s sentinel machinery.
*Setup:* the LN-2 in-process broker holding `laneSentinel` under the lane's profile, reached through the **newly composed** map rather than a hand-built one; `recordingInstances` stops one step before a process exists.
*Assert:* the sentinel reaches the lowered serve env **exactly once**, and appears in **no** event payload, **no** `run_events` row, **no** park record, **not** in the instance identity key, **not** in the ops log and **nowhere** in the SQLite file. [S11.5; §62]

**T12 · `TestLN4StartupIsUnchangedWithNothingPlaced`** — `internal/shell`.
*Assert:* with an empty state dir, the composed map is empty **and** `engineAdapters(...)`'s registered opencode adapter answers `ProvidersFor(anyone)` with an empty `ProviderConfig` and a nil error — the R7 pin at the adapter surface rather than the derivation surface.

**Plus, non-negotiable:** `TestLN4CeremonyStep8TellsTheNewTruth` must be **green** at landing, and the whole battery runs `go test -p 1 ./...` serially with no orphan processes left behind.

---

## 8. Acceptance checklist — the evaluator's rubric

The headline is *"a placed key = a routable lane."* Decomposed into what is concretely checkable:

**Fill**
1. `internal/shell/shell.go` no longer contains `engineCommissioned := map[string]opencode.ProviderConfig{}`.
2. The map is composed **before** `engineAdapters(...)` and is not mutated afterwards. (R2)
3. `engineLanes(logger)` is evaluated **once** and shared by all consumers plus the commissioning read. (R2)
4. The map's provider entries come from `LaneConfig.Providers()`/`ProviderEntry()`; the no-lane-constants source scan over `internal/adapters/opencode`, `internal/worker`, `internal/shell`, `internal/stage` is green. (R5)
5. A lane document with no profile or no env var never enters the map. (R6)
6. Only `kind == "engine-cred"` commissions. (R6)

**Posture**
7. No production code path calls `broker.OpenStore`, `Store.resolve`, `Client.Resolve` or writes to the store; `TestLN4PresenceReadNeverOpensTheBrokerStore` green. (R3)
8. Reading a store with no `master.key` leaves the directory byte-identical, proven by T7. (R3)
9. The map is keyed by the broker `who`, and the R10 startup line makes that string visible. (R4, R10)
10. No secret, and no credential material, in any log line, test fixture, or commit. (R9)

**Behaviour**
11. Empty path byte-identical: `TestLN4NothingPlacedIsInert` and T12 green, and no existing test in `internal/shell`, `internal/stage`, `internal/worker` changed its expectations to accommodate the fill. (R7)
12. A placed credential produces a lane that routing **selects** and dispatch **reaches**, proven at the real call sites and **mutation-verified**. (R8, T9)
13. Coverage, lane→substrate and alternate seats grow from placement only; execution duty only. (R8, R14, T10)
14. Containment re-proven for a lane commissioned by the new path. (R9, T11)
15. The commissioning invariant is pinned as a **property**, not only examples. (T8)

**Truth-telling**
16. `P3/gates/lane-key-ceremony.sh` step 8 states the new truth **and** the restart; `TestLN4CeremonyStep8TellsTheNewTruth` green. (R12)
17. LN gate-batch item 3 retired from the script; items 1, 2, 4, 5 intact. (R12)
18. The stale comments at `engineadapters.go:12-16` and `:37-40` corrected, dated. (R14 / §4)
19. `P3/CONVENTIONS.md` gains §65 in the §61–§64 house style, naming what landed **and what did not**.

**Discipline**
20. ⚙: none consumed, none added; S18 tally asserted at 118/33 and S18.3 surfaces at 3. (R13)
21. No new dependency; `components.lock` untouched; no adopted code modified; `lanedata/*.json` and the classifier fixtures untouched; `web/src` untouched. (R14, §1)
22. **$0**: no live provider call; every test credential is a fake in a test-scoped store; tier L still behind `SINET_LIVE_SMOKE=1`; `SINET_CANARY_ARM` untouched. (§1)
23. `go test -p 1 ./...` green, serial, no orphans. Any `SANCTIONED SKIP (CONVENTIONS §10)` lines are the only skips.
24. §5's namespace ambiguity is carried to the gate as an open item, not silently resolved.

---

## 9. Open questions for coordinator disposition

**OQ1 — the person-namespace reconciliation.** The broker store/socket `who` is an OS-level name; `runs.user_id` / `ProvidersFor(userID)` / `auth.User.ID` are platform ids; `buildAcceptSurface` uses a third source (`os.Getenv("USER")` with a `"sinet"` fallback). Nothing in the spec settles which one keys the broker store. This brief's instruction is to key by the broker `who` and make it visible (§5). **Is that the ratified reading, or does the household case need a mapping — and if so, whose?** This is a gate question; it should not be answered by an executor.

**OQ2 — the store-root override.** `sinet broker --store-dir` can point the store somewhere the control plane's `<stateDir>/broker-store` derivation will never look. Accept the recommended file read with that stated limitation (the ceremony computes the same default), or spend the robustness budget on the socket dial and accept that a broker that is down at control-plane start silently uncommissions every lane? §5 recommends the former.

**OQ3 — `live_smoke_test.go`'s mirrored predicate.** Once `internal/broker` exports the reader, the tier-L file's `placedEngineCreds`/`brokerStoreRoot` mirror becomes a second definition of the same thing. Reduce it to a call in this packet, or leave it and carry the duplication to the LN gate batch? Reducing it is cheap and is the whole argument for R3's recommended home.

---

## 10. Coordinator dispositions (appended 2026-08-24, before executor launch)

**OQ1 — RATIFIED as the packet's reading, gate item preserved.** Key the map by the broker `who`. It is the only injection-consistent choice and the code clearly implies it: the broker derives socket name AND store directory from one `who` (`mode.go:43-54`), and `laneCredInjector` dials that socket — any other key breaks injection by construction. The R10 startup line makes the person strings visible. The household-era reconciliation (OS name vs `auth.User.ID` vs the `buildAcceptSurface` env read) is NOT settled by the spec and goes to the LN gate batch as an open item (checklist 24) — the executor must not invent a mapping.

**OQ2 — FILE READ, as §5 recommends.** Startup commissioning must be deterministic and disk-truth-based: a startup-time socket probe that silently uncommissions every lane on a transient broker outage is the dishonest-absence failure class this platform refuses everywhere else. The `--store-dir` override limitation is stated in §65 as a recorded cost (the ceremony computes the same default; `StoreRoot(stateDir)` collapses the three definitions to one). If an override knob is ever wanted, that is an S00.9 amendment adding an S18 row — never minted here (R13).

**OQ3 — REDUCE the tier-L mirror in this packet.** `live_smoke_test.go`'s `placedEngineCreds`/`brokerStoreRoot` mirror becomes a call into the exported reader. Sanctioned: same file this campaign already edits, and the duplication is exactly the drift R3's home choice exists to prevent. Constraint: tier-L behavior stays outcome-identical — same gates, same sanctioned-skip texts, proven by the before/after suite.
