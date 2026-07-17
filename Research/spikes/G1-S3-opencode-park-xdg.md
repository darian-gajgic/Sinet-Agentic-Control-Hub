# Spike G1-S3 — opencode park conformance & per-user XDG isolation

**Date:** 2026-07-17 · **Answers:** R05-OQ7 (park conformance) + R01-OQ6 (per-user isolation) · **Gate:** D1.6-C spike battery
**Substrate under test:** `opencode serve` v1.18.3 (npm `latest`), the exact version Sinet would pin.
**Spend:** $0.00 inference. All probes ran with zero provider credentials.

---

## Purpose

Sinet's API lane runs one pinned `opencode serve` per user (report 01 §recommendation #1). Two open questions block the T07/T09 briefs:

1. **R01-OQ6:** does XDG-env separation *fully* contain each instance's auth/config/state on disk (issue #18633 claims some state files stray)?
2. **R05-OQ7:** park conformance — pending-ask persistence across restart, the abort → health-probe → fork drill, and the exact `permission.list`/`question.list` shapes in the pinned v1.x OpenAPI.

## Method

- **Install (user-space, no sudo):** `npm install -g opencode-ai@1.18.3` into the user-writable prefix `/home/sinep/.npm-global` (verified via `npm config get prefix` first). Binary: `/home/sinep/.npm-global/bin/opencode`; `opencode --version` → `1.18.3`.
- **Version/pin situation (checked live 2026-07-17):** npm dist-tag `latest` **is still v1.x** (`1.18.3`); OpenCode 2.0 exists only as `beta`/`dev`/`next` `0.0.0-*` snapshot tags. So today "install latest v1.x" ≡ `npm install -g opencode-ai@1.18.3`. **Pin exact** (`@1.18.3`), not `@1` (which floats within the 1.x line) and never `latest` (which will silently become 2.0 at GA). Note: the npm package ships a **Bun-compiled platform binary** (`opencode-linux-x64`, 178 MB ELF) — there is no readable TS source in `node_modules`.
- **Isolation probe:** two `opencode serve` instances (ports 14096/14097, both `--hostname 127.0.0.1`) launched with fully disjoint `XDG_DATA_HOME`/`XDG_CONFIG_HOME`/`XDG_STATE_HOME`/`XDG_CACHE_HOME` trees under the session scratchpad (`.../spike-s3/userA/{data,config,state,cache}`, same for `userB`). A `stray-marker` file was touched before launch; after the full exercise (start → session create → fork → abort → restart → shutdown), `find <default XDG opencode dirs> -newer stray-marker` detected strays. Both servers were shut down at the end; ports verified free.
- **API probe:** OpenAPI spec fetched from `GET /doc` on the running instance; sessions created/listed on both instances; abort/fork/health/restart exercised (all auth-free).
- **Source inspection (read-only; adopt-don't-fork honored):** the compiled binary carries no source, so the **v1.18.3 git tag tarball** was downloaded from the MIT-licensed `anomalyco/opencode` repo and read — same version as installed, nothing modified, nothing executed from it.
- **Credential check:** `opencode auth list` → `0 credentials` (`~/.local/share/opencode/auth.json`); env scan found no Z.AI/GLM/Anthropic/OpenAI keys. Live-session park tests are therefore **BLOCKED** (operator steps below). Per report 01, Anthropic OAuth in opencode is prohibited — not attempted.

## Findings

### 1. Per-user XDG isolation (R01-OQ6): **PASS — zero cross-leak, zero strays**

- Each instance wrote **only** inside its own XDG tree. Post-marker stray count in `~/.local/share/opencode`, `~/.config/opencode`, `~/.local/state/opencode`, `~/.cache/opencode`, `~/.bun`, `~/.cache/bun`: **0 files** across the entire lifecycle (boot, config load, session create, fork, abort, restart, shutdown).
- Per-instance layout (excluding `node_modules`):
  - `$XDG_DATA_HOME/opencode/` → `opencode.db` (+ `-wal`, `-shm`), `log/opencode.log` — **and `auth.json` when credentials exist** (verified: `opencode auth list` under the userA env reports `.../userA/data/opencode/auth.json`).
  - `$XDG_CONFIG_HOME/opencode/` → `opencode.jsonc`, `package.json`, `package-lock.json`, `.gitignore`, `node_modules/` (see cost note).
  - `$XDG_CACHE_HOME/opencode/` → `models.json` (models.dev catalog), `bin/`.
- API-level separation: instance A lists only A's sessions, B only B's; each is healthy on its own port. `projectID` is a hash of the launch cwd (identical across instances started from the same directory) but all backing state stays in the per-instance data dir — no shared files.
- **Issue #18633 (XDG stragglers) did not reproduce** on this serve-only path in 1.18.3. Caveat: no live turns, snapshots, share, or TUI were exercised — stragglers in those code paths remain possible; re-sweep during the live-auth run.
- **Per-user provisioning costs found:** (a) first boot downloads a ~62 MB provider dependency tree (`node_modules` of `@ai-sdk/*` packages) into each user's config dir — first start per user needs network and a few seconds; (b) even `opencode --version` creates the default XDG dirs if the env vars are unset — the control plane must set the XDG env for *every* opencode invocation for that user (including `auth login`), not just `serve`.
- Security notes for T09: v1.18.3 warns `OPENCODE_SERVER_PASSWORD is not set; server is unsecured` — a built-in per-instance auth knob Sinet should set per user. Default bind is already `127.0.0.1`.

### 2. OpenAPI surface (v1.18.3, fetched from `GET /doc`): all park verbs present, two generations coexist

Spec: OpenAPI **3.1.0**, 162 paths. The v1 "classic" surface and a newer `/api/*` (v2-flavored) surface coexist in the same server. Verbatim shapes the adapter parks on:

**`GET /permission` — opId `permission.list`** (the pull-based pending enumeration report 05 requires):

```json
resp 200: {"type": "array", "items": {"$ref": "#/components/schemas/PermissionRequest"}}
```

**`PermissionRequest`:**

```json
{"type": "object", "properties": {
  "id": {"type": "string", "pattern": "^per"},
  "sessionID": {"type": "string", "pattern": "^ses"},
  "permission": {"type": "string"},
  "patterns": {"type": "array", "items": {"type": "string"}},
  "metadata": {"type": "object"},
  "always": {"type": "array", "items": {"type": "string"}},
  "tool": {"type": "object", "properties": {"messageID": {"type": "string"}, "callID": {"type": "string"}},
           "required": ["messageID", "callID"], "additionalProperties": false}},
 "required": ["id", "sessionID", "permission", "patterns", "metadata", "always"],
 "additionalProperties": false}
```

**`POST /permission/{requestID}/reply` — opId `permission.reply`** (newer; supports reject-with-feedback):

```json
requestBody: {"type": "object",
  "properties": {"reply": {"type": "string", "enum": ["once", "always", "reject"]},
                 "message": {"type": "string"}},
  "required": ["reply"], "additionalProperties": false}
resp 200: {"type": "boolean"}   resp 404: PermissionNotFoundError
```

**`POST /session/{sessionID}/permissions/{permissionID}` — opId `permission.respond`** (the endpoint report 01 cited — still present, no `message` field):

```json
requestBody: {"type": "object", "properties": {"response": {"type": "string", "enum": ["once", "always", "reject"]}},
              "required": ["response"], "additionalProperties": false}
```

**`GET /question` — opId `question.list`** → `QuestionRequest[]`:

```json
{"type": "object", "properties": {
  "id": {"type": "string", "pattern": "^que"},
  "sessionID": {"type": "string", "pattern": "^ses"},
  "questions": {"type": "array", "items": {"$ref": "#/components/schemas/QuestionInfo"}},
  "tool": {"$ref": "#/components/schemas/QuestionTool"}},
 "required": ["id", "sessionID", "questions"], "additionalProperties": false}
```

**`POST /question/{requestID}/reply`** body: `{"answers": QuestionAnswer[]}` where `QuestionAnswer = string[]` ("each answer is an array of selected labels"); **`POST /question/{requestID}/reject`** takes no body. Both return boolean / 404 `QuestionNotFoundError`.

**New `/api/*` equivalents** (same server): `GET /api/permission/request` (opId `v2.permission.request.list`) and `GET /api/question/request` return `{"location": LocationInfo, "data": PermissionV2Request[] | QuestionV2Request[]}`; reply via `POST /api/session/{sessionID}/permission/{requestID}/reply`. `PermissionV2Request` renames fields: `action` + `resources[]` + `save[]` + `source` instead of `permission`/`patterns`/`always`. Event schema has both generations too (`EventPermissionAsked` / `EventPermissionV2Asked`, same for question/replied variants).

**Session/park/drill verbs, all confirmed present:** `POST /session` · `GET /session` · `POST /session/{id}/prompt_async` (body includes per-message `model: {providerID, modelID}`, `agent`, `tools`, `parts[]`) · `POST /session/{id}/abort` → boolean · **`POST /session/{id}/fork`** (body `{messageID?}` → full `Session`) · `GET /session/status` → map of `{type: "idle" | "busy" | "retry"}` (the `retry` variant carries `attempt`, `message`, `next`, and an optional provider-`action` object — useful 429/limit signal for D4) · health: **`GET /api/health`** → `{"healthy": true}` and **`GET /global/health`** → `{"healthy": true, "version": "1.18.3"}` · events: `GET /event`, `GET /global/event`, `GET /api/event`, per-session `GET /api/session/{id}/event`; plus `GET /session/{sessionID}/message` for transcript pulls.
Also present: `PUT /auth/{providerID}` (body = `Auth` = `OAuth | ApiAuth | WellKnownAuth`; `ApiAuth = {"type": "api", "key": string, "metadata"?}`) — a REST path for credential provisioning; and `POST /global/upgrade` — a **self-upgrade endpoint the control plane must never expose or call** (pin discipline).

Live behavior checks (auth-free): create session → full Session JSON with typed token/cost fields; `abort` on an idle session → `true`; `fork` → new session titled "… (fork #1)"; `permission.list`/`question.list` → `[]`.

### 3. Persistence layout: v1.18.3 stores state in **SQLite (WAL)** — sessions durable, pending asks NOT

The old JSON-file-per-message layout is gone; each instance owns `$XDG_DATA_HOME/opencode/opencode.db` (WAL mode). Tables (inspected read-only): `project`, `project_directory`, `session`, `message`, `part`, `session_message` (typed, seq-ordered), `session_input` (admitted-prompt queue with `admitted_seq`/`promoted_seq`), `session_context_epoch` (baseline/snapshot refs), `event` + `event_sequence` (**a per-session durable event log** — matches the `SessionDurableEvent` schema in the API), `todo`, `session_share`, `permission`, `account`, `account_state`, `control_account`, `credential`, `migration` (38 applied).

- The `session` row carries `cost`, `tokens_*` (input/output/reasoning/cache read+write), `parent_id`, `revert`, `permission`, `agent`, `model` — D5 accounting fields are first-class columns.
- **The `permission` table is `(project_id, action, resource)` — saved "always" *rules* (the `/api/permission/saved` surface), NOT pending asks. No table anywhere holds a pending permission or question request.**
- **Restart drill (empirical): PASS for sessions.** SIGTERM on instance A → restart with the same XDG env → both the original and the forked session listed intact; health OK. Sessions, messages, and the durable event log survive restarts; this confirms report 05's "session persists on disk; supported park-across-restart is re-prompting the same session id".
- **Caution for T09:** `account`/`control_account`/`credential` tables store access/refresh tokens **in plaintext inside the per-user DB** (opencode cloud/console + integration credentials), alongside `auth.json`. Per-user data dirs need `0700` and backup-encryption treatment.

### 4. Source inspection (v1.18.3 tag, `packages/opencode/src/permission/index.ts` + `question/index.ts`): pending-ask volatility CONFIRMED, with new precision

The report-05 finding reproduces exactly in the pinned version, and extends to questions:

```ts
interface PendingEntry {
  info: PermissionV1.Request
  deferred: Deferred.Deferred<void, PermissionV1.RejectedError | PermissionV1.CorrectedError>
}
interface State {
  pending: Map<PermissionV1.ID, PendingEntry>
  approved: PermissionV1.Rule[]
}
```

- A parked turn literally awaits an in-process Effect `Deferred` (`ask` → `pending.set(id, …)` → `Deferred.await`). Zero cost while parked; nothing written to the DB.
- **Graceful shutdown rejects, crash loses:** an `Effect.addFinalizer` iterates `state.pending` and fails every deferred with `RejectedError`, then clears the Map. So on orderly disposal the in-flight turn sees a rejection; on SIGKILL/crash the ask simply evaporates (issue #15172 behavior). Either way **the ask does not survive into the next server life** — platform-side ask persistence (report 05 §4.3, P-T02-1) is mandatory, confirmed at source level.
- **Reject-with-feedback is first-class:** `reply: "reject"` with a `message` fails the deferred with `CorrectedError({feedback})` — the model receives the correction text. This is the newer `/permission/{requestID}/reply` body's `message` field; the older `permission.respond` endpoint cannot carry it. **Adapter should target `permission.reply`.**
- A `reject` also auto-rejects **all other pending asks in the same session**; an `always` reply pushes rules onto the in-memory `approved` array and auto-approves matching same-session pending asks. **V1 `always` approvals are in-memory only — they do not survive restart either** (the SQLite `permission` table belongs to the V2 saved-permission surface).
- `question/index.ts` mirrors the same structure (`pending: Map<QuestionID, PendingEntry>`, same finalizer pattern) — **question asks are exactly as volatile as permission asks.**

## Verdict

**Park conformance status: architecture-conformant with the known volatility, pending live confirmation.**

- **R01-OQ6 — ANSWERED: PASS.** XDG-env separation fully contains serve-path state per instance in v1.18.3 (auth.json, SQLite DB, config, cache, logs; zero strays, zero cross-leak). The per-user `opencode serve` design is validated at the isolation level. #18633 did not reproduce (serve path; live-turn/TUI paths unexercised).
- **R05-OQ7 — mechanics answered, live behavior blocked.** Confirmed without auth: all park verbs and both list endpoints exist with the exact shapes quoted above; sessions/forks survive restart (SQLite); pending permission *and* question asks are in-memory `Map`s that are rejected-on-graceful-shutdown / lost-on-crash (source-verified in the pinned version). Sinet's design consequence stands firm: **persist every ask as a platform record the moment `permission.asked` is observed; treat a serve restart as "ask gone, session intact, re-prompt same session id (a paid turn)".**
- **Only a live-auth run can confirm:** (1) an actual `permission.asked` park (ask → park at zero cost → answer via REST → turn resumes); (2) ask disappearance from `permission.list` after restart *with a real pending ask* and the re-prompt recovery path; (3) the #16367 serve+attach hang regression under `ask` permissions; (4) the abort → health-probe → fork drill **during a running turn** (mechanical verbs all work idle); (5) whether SIGTERM's `RejectedError` finalizer surfaces as a visible `permission.replied`/session error event that the adapter can log; (6) parallel-ask fan-out behavior under real tool calls.

## Implications for T07/T09 briefs

1. **T07 (checkpoint store):** opencode 1.18.3 now keeps a per-session durable event log (`event`/`event_sequence` + `SessionDurableEvent` API schema) and typed cost/token columns — good cross-check material, but pending asks are excluded from it, so the platform ask record remains authoritative (P-T02-1 unchanged, now source-pinned to the exact version).
2. **T07 (adapter):** target `permission.list`/`question.list` for attach-time enumeration and `POST /permission/{requestID}/reply` (not the legacy `permission.respond`) to keep reject-with-feedback (`CorrectedError`). Note both event generations (`EventPermissionAsked`/`EventPermissionV2Asked`) — parse tolerantly.
3. **T07 (drill wiring):** health = `GET /api/health` (or `/global/health` for version); status = `GET /session/status` (its `retry` variant carries provider-limit context for D4); fork = `POST /session/{id}/fork {messageID?}`.
4. **T09 (per-user provisioning):** set all four XDG vars for *every* opencode invocation per user; budget ~62 MB + network for each user's first boot (provider node_modules); set `OPENCODE_SERVER_PASSWORD` per instance; `chmod 0700` data dirs (plaintext tokens in both `auth.json` and the SQLite DB); never expose `POST /global/upgrade`.
5. **Pinning:** `npm install -g opencode-ai@1.18.3` (exact). npm `latest` is still 1.x today but will flip at 2.0 GA; `@1` floats. Re-verify dist-tags at adoption time.

## Blocked items — literal operator steps (Z.AI GLM Coding Max key)

Live park tests need one provider credential. Sanctioned path: the operator's Z.AI GLM Coding Max plan officially whitelists opencode (report 01, S42). Steps (either option; both write `auth.json` under the active `XDG_DATA_HOME`):

**Option A — CLI login (interactive paste, no browser):**

1. Log in at `https://z.ai` → open the GLM Coding Plan dashboard → create/copy the coding-plan API key. Expected: a key string.
2. In a terminal on this machine, run: `opencode auth login`
   Select provider **"Z.AI Coding Plan"** (`zai-coding-plan`), paste the key. Expected: prompt confirms; exit code 0.
3. Verify: `opencode auth list` — expected output: `1 credential` listing `zai-coding-plan`, stored at `~/.local/share/opencode/auth.json`.
4. Tell the spike runner which XDG env (if any) you used; the spike re-run will copy nothing — it will either point its instances at that data dir or repeat step 2 with the spike's `XDG_DATA_HOME` prefix.

**Option B — non-interactive, per-instance REST (no TTY needed):**

1. Start the target instance (spike env shown): `XDG_DATA_HOME=<user-data-dir> opencode serve --port 14096`
2. `curl -s -X PUT http://127.0.0.1:14096/auth/zai-coding-plan -H 'Content-Type: application/json' -d '{"type":"api","key":"<PASTE-KEY>"}'` — expected: HTTP 200.
3. Verify: `XDG_DATA_HOME=<user-data-dir> opencode auth list` — expected: `1 credential` at `<user-data-dir>/opencode/auth.json`.

(Alternative accepted by the catalog: export `ZHIPU_API_KEY=<key>` in the serve process env — works without auth.json; provider id `zai-coding-plan`, endpoint `https://api.z.ai/api/coding/paas/v4`, models `glm-4.7`, `glm-5-turbo`, `glm-5.1`, `glm-5.2`, … per the models.dev catalog fetched live 2026-07-17.)

**Blocked test list (runnable immediately after provisioning, ≤$0.50 budget):** live `permission.asked` park + REST answer round-trip; pending-ask loss across restart with a real ask (reproduce #15172); #16367 serve+attach hang regression; abort → health-probe → fork during a running turn; SIGTERM finalizer visibility; parallel-ask behavior.

## Cost disclosure

- **Inference: $0.00.** No provider credential existed; every probe was auth-free.
- Network: npm package download (~178 MB binary), v1.18.3 source tarball from GitHub (80 MB, read-only inspection), models.dev catalog (auto-fetched by the binary at boot).
- Both serve instances were localhost-only and are shut down; ports 14096/14097 verified free. No git commits made. Nothing in `Docs/` or engine source modified.
