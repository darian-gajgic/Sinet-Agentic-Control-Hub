# 17 — Platform stack & process architecture

**Topic:** T13 · Wave C · Depth LIGHT (currency check + fit reasoning, per brief) · **Date:** 2026-07-17
**Method:** 5 search angles (session WebSearch budget was exhausted mid-run; all angles fell back to direct WebFetch of primary sources + the GitHub API + HN Algolia — a stronger sourcing mix for status/version claims, at the cost of thinner practitioner-thread coverage, flagged inline where it matters). 22 load-bearing claims went through a 3-vote adversarial pass (66 votes): **63 SUPPORT / 3 PARTIAL / 0 REFUTE — no claim killed.** PARTIALs folded: V1 (man page says "some", not "most", sandboxing is unavailable to user units, with a `PrivateUsers=` carve-out), V21 (passkeys.dev's Windows synced-passkeys cell is blank/unshipped — the literal "Planned" label sits on Ubuntu; substance unchanged). One open unknown resolved (V4: GitHub artifact attestations on private repos are Enterprise-Cloud-gated). Corrections are folded into the text below.

---

## 1. Scope

Feature-list items: **S3.1–S3.10** (the stack that carries mission control, approval inbox, workforce map, board, review surfaces, chat, notifications, live-by-default, multi-user-aware — UX details stay with the spec), **§13** incl. **13.4** (settings surface), **15.6** (owner on every record from day one), **D1** (one host, LAN/Tailscale exposure only), and the **anti-monolith lesson** (Nexus: `server.py` ≈ 11,476 lines / 248 routes / 32 tables plus an 11.7k-line vanilla-JS SPA — "no seams, one maintainer" per the post-mortem). Operating-reality bindings: survives sleep/wake; unattended while the host is up.

Settled organs this topic designs around, never reopens: SQLite-WAL `platform.db`, sole-writer control plane, `BEGIN IMMEDIATE`, `synchronous=FULL` (report 08 §4.1); the D7 event log as observability substrate and **one SSE endpoint** with `event_seq` cursor resume (report 12 §4.1/4.3); the sandbox stack (report 10 §4), credential broker (systemd-creds + sops/age), preview plumbing (Caddy admin-API routes, `tailscale serve`, systemd-socket-proxyd idle-stop, port-pool daemon — report 13 §4.8); the report 14 §3.3 frontend shortlist as **candidates only** (binding picks at the P2 frontend workshop); P-T11-4 (tools are runners/projectors, never stores) and P-T16-1/-2 (component-onboarding checklist; path-scoped licenses) as adoption law; the G1 settings-not-constants rider.

---

## 2. Current state of the art (mid-2026)

### 2.1 The survivor pattern: small core, embedded SQLite, boring transport

The self-hosted platforms that live for years under one maintainer converge hard: **single Go binary (or one-runtime monolith) + embedded SQLite + pragmatic realtime + bundled boring frontend**. PocketBase — the closest shape twin to Sinet's shell — is a solo personal project ("no paid team or company behind it"), Go, SQLite-WAL, realtime verbatim "implemented via Server-Sent Events (SSE)", v0.39.7 (2026-07-16), still pre-1.0 [S104–S106]. Miniflux (Go, solo, 9 years, deliberately Postgres-only and framework-free "statically compiled binary without any dependencies") states scope refusal as its survival strategy in writing [S51, S108]. ntfy, Vikunja, Audiobookshelf, linkding, changedetection.io — all effectively one-person, all SQLite-or-flat-file [S115–S120]. Even the big-team outliers default to embedded SQLite (Home Assistant: "the default, and recommended, database engine is SQLite"; Grafana ships sqlite3 in-binary) [S110, S112]. SQLite is the default store in 8 of 11 platforms checked. Contrast case: report 14 §2.2's control-plane cohort — funded teams building big agent-platform surfaces — died at ~14-month half-life. The survivable shape at bus factor 1 is exactly what G2 ratified: **small owned core + adopted organs**, and the cohort shows what the core should look like.

### 2.2 Process supervision practice

Multi-unit systemd decomposition is live practice for self-hosted Python/Go platforms: paperless-ngx ships web server / consumer / scheduler / task-queue as four plain system units plus a socket unit, backend bound to localhost TCP [S12]. The decisive system-vs-user-unit facts: *some* sandboxing functionality (the filesystem-namespacing settings, absent `PrivateUsers=`) is unavailable to user services, and `WakeSystem=` timers "require privileges and [are] thus generally only available in the system service manager" [S1, S3 — verified 3/3, wording corrected]. systemd's own recommendation for long-running services is `Restart=on-failure`; `Type=notify` + `WatchdogSec=` + `sd_notify(WATCHDOG=1)` is the liveness pattern [S2]. `DynamicUser=` recycles UIDs 61184–65519 and is documented as unsuitable for services leaving long-lived owned files — a stateful control plane wants a static user + `StateDirectory=` [S1]. For co-located IPC, the field runs unix sockets or localhost HTTP; DB-as-queue is Rails-8-mainstream ("Now all of it can be done with SQLite thanks to … Solid Queue", default in new Rails apps, SQLite explicitly supported) — no current primary source recommends gRPC or a broker for single-host lanes [S13–S15]. This re-validates report 08 §4.6's queue-on-SQLite verdict from a second direction.

### 2.3 Backend language state

- **Python:** FastAPI 0.139.2 (2026-07-16) — still pre-1.0 after ~8 years, with ~20× commit concentration on one maintainer (2,206 vs next human 111) [S27, S28]. Pydantic v2 mature (2.13.4) [S30]; Litestar is the org-maintained alternative (v2.24.0) [S31]. uv is the packaging answer but itself 0.x (0.11.29, 2026-07-15) [S21]; Astral's `ty` type checker is explicitly beta — typing remains an external optional layer [S32]. Annual releases / 5-year EOL ⇒ ~10 interpreter migrations per decade [S33]. Python remains the first-class agent ecosystem (Anthropic SDK page lists it first; pydantic-ai v2.12.0 ships model support within days) [S35, S36].
- **Go:** the only track with a written, 14-year-honored compatibility promise ("It is intended that programs written to the Go 1 specification will continue to compile and run correctly, unchanged" — verified verbatim, hedge included) [S37]; 6-month cadence, non-breaking [S38]. Stdlib `net/http` routing (1.22+) makes frameworks optional; SSE is a `Flusher` loop [S39]. Pure-Go SQLite (modernc v1.54.0, wraps SQLite 3.53.3) removes cgo from builds; mattn also current [S40, S41]. The historical blocker is gone: **anthropic-sdk-go v1.58.0 GA on a stable v1 line** (openai-go is GA but already at major v3 — churnier) [S42, S43].
- **TypeScript/Node:** structural churn documented from primary sources: Node = ~5 forced LTS migrations/decade [S44]; **Bun was acquired by Anthropic 2025-12-02 and is being rewritten in Rust (post dated 2026-07-08)** — a flagship runtime changing owner and implementation language within eight months [S45]; GitHub's own 2025-09-22 post documents the self-replicating Shai-Hulud npm worm (500+ packages) forcing registry-wide publishing changes [S49]; Express spent ten years shipping v5 [S47]. The wrapped tools being TS confers nothing across a subprocess/HTTP boundary — and their runtimes ship on the host regardless.

### 2.4 Frontend state

- **React/Vite (SPA mainline):** React 19 line calm at the API level (19 → 19.2 additive; React Foundation under Linux Foundation 2026-02-24 is a governance positive), but the ecosystem taxes attention: Create React App sunset 2025-02-14; a **critical unauthenticated RCE in React Server Components 2025-12-03** (fixed same day across 19.0.1/19.1.2/19.2.1 — the vulnerability class lived in RSC, which a plain SPA does not ship); Vite majors near-annual (6 → 7 → 8 in ~16 months, v8 2026-03-12) [S57, S58 — verified 3/3].
- **HTMX (hypermedia mainline):** the only frontend project with a written perpetuity commitment — "htmx 2.0 … will be supported *in perpetuity*" (The fetch()ening, 2025-11-01), demonstrated by parallel maintenance of 2.x (v2.0.9, 2026-04-20) alongside 4.0 betas; upgrades are optional by policy [S52–S54]. Its own fit doctrine flags widget-dense, frequently-updating UIs as hypermedia's weak quadrant and concedes "it is typically easier to embed SPA components within a larger hypermedia architecture than vice versa" [S62 — 2022, flagged old but canonical]. Drag-drop has an official SortableJS island pattern [S63].
- **Datastar:** the SSE-native hypermedia framework reached 1.0.0 on **2026-04-16** (v1.0.2, 2026-06-02) — transport-aligned with Sinet's settled SSE, but a 3-month-old 1.0 with one RC-era breaking rewrite is not a decade bet yet [S55, S56]. Svelte 5 calm; Alpine/Lit demonstrate the low-churn client end [S59–S61].

### 2.5 Phone approvals: the push reality

Standard Web Push always transits the **browser vendor's** push service — RFC 8030: "User agents are expected to be configured with a URL for a push service"; the app server only receives the subscription endpoint. **There is no self-hostable push service for standard Web Push** (an architectural consequence all three verifiers endorse, not a literal RFC sentence) [S65, S66 — verified 3/3]. That is compatible with a tailnet-only app: the host needs only *outbound* HTTPS to vendor endpoints, payloads are encrypted to subscription keys (vendors see timing/volume, not content), nothing inbound is exposed [S66, S67]. iOS: web push exists since 16.4 **only for web apps added to the Home Screen**; **Declarative Web Push shipped in iOS/iPadOS 18.4** — JSON payload, no service worker, browser-rendered, with a **required `"navigate"` field** (the URL opened on tap) and `app_badge` (pending-count badge) [S68–S70 — verified 3/3]. Android is the easy half (Chromium push standard) [S71]. HTTPS/secure context over Tailscale: `tailscale serve` auto-provisions Let's Encrypt certs for the ts.net name and serves tailnet-only; the KB warns machine names land in the **public CT ledger**, and file-based `tailscale cert` certs carry manual ~90-day renewal (serve manages its own) [S90, S92]. Fallbacks: ntfy is Android-perfect fully self-hosted, but its own docs concede instant iOS push from a self-hosted server requires forwarding `poll_request`s to an APNs-connected upstream — effectively ntfy.sh for the official app ("iOS heavily restricts background processing, which sadly makes it impossible … without a central server") [S72]; Gotify is Android-only [S73]; UnifiedPush has no iOS story [S74]. For iOS, *every* instant option transits an Apple-blessed cloud — Web Push (APNs via Safari) is the same compromise class as ntfy-with-relay, with fewer moving parts.

### 2.6 Real-time currency check (settled SSE)

No shift away from SSE for server→client live UI at this scale. EventSource is Baseline-stable; the HTTP/1.1 6-connections-per-origin limit remains Won't-fix and HTTP/2 (default ~100 streams) remains the sanctioned mitigation [S75]. WebTransport only completed cross-browser support with Safari 26.4 and targets multi-stream/unreliable delivery — arriving, not displacing, and the wrong shape for one-way event fan-out [S76]. The newest framework activity of 2026 (Datastar 1.0) is built *around* SSE [S55]. **Report 12 §4.3's design is confirmed as-is; this topic adds only: terminate HTTP/2 at the front door so multi-tab never hits the connection limit.**

### 2.7 SQLite ceiling + backup tooling

No axis stops sufficing at household scale. Size: 281 TB theoretical; a decade of event log at generous rates ≈ single-digit GB — 3–4 orders below where sqlite.org itself starts recommending client/server [S77, S78]. Writes: fsync-bound; even `synchronous=FULL` on NVMe leaves 2–4 orders of headroom over Sinet's <10 writes/s [S79, S80]. sqlite.org's own forcing checklist (network-separated data, many concurrent writers, terabyte range, write-heavy multi-server) — none applies to a sole-writer single-host design [S78 — verified 3/3]. Two ≤12-month production accounts both trace their SQLite incidents to *violating* single-writer (overlapping blue-green containers; an accidental second queue process) — reinforcing, not undermining, the settled discipline [S88, S89]. Operational currency: SQLite line at 3.53.3 (2026-06-26); a **WAL-reset database-corruption bug was fixed in 3.51.3/3.53.0** — a concrete reason to keep the bundled SQLite current [S83]. WAL caveat: transactions >100 MB degrade, >1 GB can fail — chunk bulk imports [S79]. `BEGIN CONCURRENT` remains branch-only (no 2025–26 release notes mention); hctree remains an incomplete prototype — neither is a planning input [S85, S86]. Litestream: v0.5.14 (2026-07-06) still latest; the 0.5.x line had one refactor-revert episode (0.5.13) — pin and read release notes [S87]. **sqlite3_rsync** (official; since 3.50 the WAL-both-sides and same-page-size restrictions were removed, safe on a live origin DB) is a sound periodic point-in-time complement to Litestream streaming [S84].

### 2.8 Identity over Tailscale

`tailscale serve` injects `Tailscale-User-Login/-Name/-Profile-Pic` on proxied tailnet requests, **strips those headers from incoming requests** ("to avoid header spoofing"), and omits them for Funnel traffic and tagged nodes; documented best practice is backend-binds-localhost so nothing bypasses the proxy [S90 — verified 3/3]. The underlying mechanism is LocalAPI `WhoIs` (source ip:port → user profile), the same one tsnet apps use [S91, S94]; Grafana's auth-proxy is the canonical trusted-header consumer pattern [S100]. **Limit: this is device identity, not person identity** — a Tailscale device "lets only one active account log in at a time", so on a shared tablet every family member surfaces as the device's registered user [S93]. tsidp (Tailscale as OIDC IdP) is explicitly experimental (`TAILSCALE_USE_WIP_CODE=1`, v0.0.14) — watch-only [S95]. Passkeys: server libraries healthy (go-webauthn v0.17.4; py_webauthn v3.0.0, 2026-06-29); platform sync is great on phones but **Windows synced passkeys remain unshipped** (matrix cell blank as of 2026-05-20; "Planned" is Ubuntu's label) and Linux is extension-only; WebAuthn RP-ID pins credentials to the ts.net hostname — renaming the machine strands every credential [S96–S99]. Community practice runs the gamut, with the vocal norm being defense-in-depth, not Tailscale-only, for anything that holds keys or takes actions [S103]. AuthZ: SQLite has no row-level security; app-layer owner scoping is the norm; casbin is alive (snapshot-tag cadence), oso OSS is deprecated (support + critical fixes only) [S101, S102].

### 2.9 Settings machinery

The mature pattern is a **hybrid**, converged on from three independent directions: PocketBase stores app settings in the DB as JSON (bootstrap secret via `--encryptionEnv`) [S107]; Gitea — a file-config platform — is migrating runtime tunables into the DB one deprecation at a time ("moved to database. Use admin panel to configure") [S113]; Home Assistant's 2020 decision record moved runtime config from YAML into managed, versioned, migrated storage [S125, S126]. Grafana is the counterexample proving the cost: ini/env only, ~60+ sections, restart-to-apply, no in-product organization [S112]. Schema-generated settings UIs are a solved problem with two live incumbents: RJSF v6.6.2 (2026-06-06, org-maintained, ten theme packages) and JSON Forms v3.8.0 (separate UI-schema with group/categorization layouts + rule-based visibility out of the box — the more direct fit for a 100+-setting surface) [S123, S124]. django-constance codifies default-in-code + override-row + reset=delete-row, and confirms the negative space: **no settings library ships change history — the audit trail must be built** (it fires a `config_updated` signal with old/new values; storage is on you) [S128]. Named prior art for ceiling-bounded auto-adjustment: the Kubernetes HPA min/max clamp — controller adjusts freely *within* operator-set bounds [S129]. Bootstrap layer: pydantic-settings-class typed loaders with layered precedence are standard [S122].

### 2.10 Packaging / deploy / ops

For a Python service, `uv sync --locked` + per-app venv is uv's *own documented* deploy pattern (Docker and GitHub Actions guides; pin the uv version) [S21–S23]. Containerizing the platform itself is sanctioned via Podman Quadlet, but a control plane that must drive host systemd units, bwrap sandboxes, and the tailscale CLI would have to punch all of those through the container boundary — negating the isolation that is the container's point [S19]. Directory layout follows the unit type: system units get `/etc` + `/var/lib/<name>` via `ConfigurationDirectory=`/`StateDirectory=` with zero custom code; XDG (spec v0.8, 2021 — old but current) applies to user services [S1, S20]. Logs: stdout→journald is the default plumbing, not a hack; underscore-prefixed fields (`_SYSTEMD_UNIT`, `_PID`, `_UID`) are journal-added and unforgeable — free per-process attribution; per-unit `LogRateLimitIntervalSec/Burst` overrides matter for chatty workers; `SystemMaxUse` defaults ~10%/4 GB [S9–S11]. CI: GitHub Actions artifact attestation is one step and gives SLSA Build L2 — **but on private/internal repos it requires GitHub Enterprise Cloud** (Free/Pro/Team: public repos only — resolved from the github/docs source reusable, which no longer renders on the live page) [S24–S26 — verified 3/3]. The fallback for a private repo: checksums + signed tags verified by the deploy script.

### 2.11 Sleep/wake

systemd's position: `system-sleep/` hook scripts are "hacks"; the sanctioned mechanism is **inhibitor locks** — a delay-mode lock on `sleep` gives a bounded pre-suspend window (`InhibitDelayMaxSec` defaults to 5 s) [S6–S8]. Timers: monotonic timers pause during suspend; `Persistent=true` (OnCalendar) fires a missed run immediately on next activation; `WakeSystem=` is system-manager-only [S3]. What actually breaks after resume: stale NAT mappings and expired leases — Tailscale's own netmon polls wall-vs-monotonic clock skew every 15 s as its resume detector and rebinds [S16]; and **Linux tailscaled sometimes fails to reconnect after wake** (issue #10688, open since 2023, activity through 2026-04; community workaround `systemctl restart tailscaled`) [S17]. A platform that promises "unattended while up" on a laptop must own resume as a first-class event.

---

## 3. Candidate approaches

| Axis | Candidates | Verdict against operator context |
|---|---|---|
| **Decomposition** | (a) modular monolith, one process; (b) small core + organ units (control plane / front door / per-user engines / transient lanes / single-purpose daemons); (c) microservices | (a) is the Nexus failure restated; (c) multiplies ops surface at bus factor 1 with zero scale need. **(b)** matches both the survivor cohort and the already-settled organ set — the seams are process+storage+API boundaries, not network-service count. |
| **Supervision** | system units vs user units + linger | System units: hardening breadth (namespacing settings without `PrivateUsers=` gymnastics), `WakeSystem=`, `/var/lib` via `StateDirectory=`, boot-start without login plumbing. User units buy nothing Sinet needs [S1, S3–S5]. |
| **IPC** | unix sockets / localhost HTTP / SQLite queue / gRPC / broker | UDS + localhost HTTP for request paths; SQLite tables as the only queue (settled 08 §4.6, re-validated by Rails 8 mainstreaming). gRPC/brokers: overkill with no single-host advocate in the field [S13–S15]. |
| **Backend language** | Go / Python (FastAPI or Litestar) / TypeScript | **Go** wins longevity (go1compat), compiler-enforced typing at bus factor 1, single-binary deploy, pure-Go SQLite, GA Anthropic SDK, and total cohort convergence. **Python** wins operator's demonstrated fluency (Nexus was FastAPI) and agent-library gravity — but Sinet's D3 architecture (subprocess + HTTP adapters) structurally quarantines that advantage. **TS** loses on documented runtime/supply-chain churn [S37–S49]. |
| **Frontend** | React 19 + Vite SPA / HTMX 2.x + islands / Datastar / hybrid shell+islands | The widget profile (S1.3 drag-drop board, S3.5 diff/monaco surfaces, S3.6 chat) is React-shaped per the §3.3 shortlist and sits in hypermedia's own declared weak quadrant. SPA-with-discipline carries a known, bounded churn tax; HTMX is the strongest longevity paper but re-opens every widget; hybrid = two toolchains (worst surface count); Datastar too young (1.0 ~3 months) [S52–S64]. |
| **Push** | Web Push (Declarative on iOS) / ntfy / Gotify / Telegram | Web Push: standards-clean, no extra app, deep-links via required `navigate`, badge counts; vendor-relay metadata accepted. ntfy: Android-perfect self-hosted, iOS needs ntfy.sh relay — same compromise class, more moving parts, second app. Gotify/UnifiedPush: no iOS [S65–S74]. |
| **AuthN** | Tailscale-only / TS-headers + app sessions / passkeys-first | TS-only conflates device with person and makes a lost phone a full-power actor. **Layered**: tailnet wall → serve headers as spoof-resistant device *hint* → server-side sessions + per-user PIN as authoritative person identity; passkeys as optional upgrade [S90–S99, S103]. |
| **Settings storage** | all-files (Grafana) / all-DB / hybrid | **Hybrid** is the convergent norm: bootstrap in env/file, runtime tunables as typed DB rows with defaults-in-code [S107, S113, S125, S128]. |
| **Packaging** | native (binary or uv venv) under systemd / Podman Quadlet containers | **Native.** The platform manages host resources (systemd-run lanes, bwrap, tailscale CLI, journald, GPU broker); containerizing it inverts its job. Sandboxes are T09's separate concern [S19]. |

---

## 4. Recommendation for Sinet

### 4.1 Process map — the anti-monolith, operable by one person

All platform units are **system units** with a static `sinet` user (never `DynamicUser=` — UID recycling vs long-lived DB files [S1]), standard hardening set (`ProtectSystem=strict` + `ReadWritePaths`/`StateDirectory`, `NoNewPrivileges=`, `PrivateTmp=`, syscall filter), `Restart=on-failure`:

- **`sinet-control.service`** — the owned core and the only novel daemon of size. Sole `platform.db` writer (08 §4.1); scheduler + queue claiming (08 §4.6); D7 checkpoint/gate machinery + effect journal; the event log; the **HTTP API + the one SSE endpoint** (12 §4.3); adapter supervision. `Type=notify`, `WatchdogSec=` ⚙, `sd_notify` heartbeat. Internally a modular monolith with enforced module seams (storage / scheduler / gates / ledger / event-log / adapters) — decomposition here means *seams*, not network services.
- **`sinet-broker.service`** — the settled credential broker (10 §4): ssh-agent-shaped, UDS with peer-cred auth, systemd-creds + sops/age at rest. Separate process so the large-attack-surface control plane never holds decrypted member credentials.
- **`sinet-engine@<user>.service`** — templated per-user units for the pinned `opencode serve` instances (report 01's substrate); `claude` CLI runs are per-run subprocesses inside lanes, not standing services.
- **Execution lanes** — per-run **transient units** (`systemd-run`) composing report 10's sandbox stack. Lanes never touch `platform.db`: they stream events/checkpoints to `sinet-control` over a local API, and the control plane persists (sole-writer preserved, crash isolation per run, every lane visible in `systemctl`/journald under its own identity).
- **`sinet-portpool.service`** — the ~200-line port-pool daemon (13 §4.8).
- **Adopted organs, each its own unit:** `caddy.service` (front router: static assets, `/api` + `/events` proxy with buffering off, HTTP/2, preview subdomain routes via admin API), `litestream.service` (pinned), changedetection.io (12 §4.5 watchlist executor), the T15 local-model server(s), `tailscaled` (given).

**Front chain (trust chain):** `tailscale serve` (tailnet-only TLS, identity headers, strips inbound spoofs) → Caddy → `sinet-control` bound to **127.0.0.1 only**. Every backend binding localhost is a *security invariant*, not a style choice — a `0.0.0.0` listener bypasses both the identity headers and the tailnet wall on the LAN (→ P-T13-2, §7). The web surface is **not a custom process**: static SPA files served by Caddy + the control plane's API. The SPA, the chat surface, and any future CLI are all just API clients — the API seam is what makes the monolith impossible to rebuild by accident.

**IPC:** browser→serve→Caddy→control (HTTPS/SSE, HTTP/2 at the front); lanes→control (UDS or localhost HTTP); control→engines (localhost HTTP, per-user ports); control+lanes→broker (UDS, operation-in/result-out); queue = SQLite tables claimed only inside the control plane. No gRPC, no broker, no Redis [S13–S15].

**The five seams that encode the monolith lesson:** (1) storage — one writer, read-only opens for everything else; (2) process — every organ a unit with its own journal identity (`_SYSTEMD_UNIT` is unforgeable [S10]); (3) API — surfaces only through the HTTP API; (4) adapter — D3's contract per engine; (5) adoption — every third-party organ behind the P-T16-1 manifest (§4.8). Nexus had none of these; the seams, not process count, are the requirement.

### 4.2 Backend language: Go (conditional on G3 operator ratification)

**Recommendation: Go** for `sinet-control`, the broker, and the port-pool daemon. Grounds, all primary-verified: the go1compat promise is the only written 10-year API-stability instrument in the field [S37]; typing is compiler-enforced — the refactoring-safety property bus factor 1 actually needs (Python's next-gen checker is literally in beta [S32]); deployment collapses to one static binary + systemd with frontend assets embedded (`go:embed` — the PocketBase pattern), removing interpreter EOLs, venvs, and a 0.x package manager from the decade budget [S21, S33]; pure-Go SQLite is current [S40]; anthropic-sdk-go is GA on a stable v1 [S42]; and the survivor cohort is overwhelmingly Go [§2.1]. Sinet's own architecture neutralizes Python's classic advantage: engines are wrapped CLIs/HTTP servers (D3), evals run via adopted runners (promptfoo — a subprocess), local models are HTTP services, and adopted Python/JS organs (genai-prices data, opencode) are consumed as processes or data files per P-T11-4 — never linked libraries. No language couples to them.

**The honest counter-case, stated for G3:** the operator built Nexus in Python and personal velocity is the one input research cannot measure; Python remains where agent-adjacent libraries land first, which matters *only if* scope ever creeps from orchestrating engines to embedding agent loops — a creep D3 exists to prevent. If the operator rules Go velocity unacceptable, the Python variant is Litestar (org-maintained) or Starlette+Pydantic — not bare FastAPI-as-decade-bet (0.x, ~20× single-maintainer concentration [S27, S28]) — deployed via pinned uv `sync --locked` [S21, S22]. **Everything else in this report is language-independent; a G3 flip costs nothing but this subsection.**

### 4.3 Frontend architecture: disciplined SPA; binding picks stay with the workshop

**Recommendation: a plain React 19 + Vite SPA in TypeScript — explicitly no meta-framework, no RSC, no SSR layer** (the Dec-2025 RCE class lived in RSC [S57]; a tailnet app has no SEO/first-paint case). It consumes the settled SSE endpoint (snapshot-then-tail), ships as static files embedded in the binary, and installs as a PWA. This is the architecture recommendation the P2 frontend workshop starts from; the §3.3 shortlist (dnd-kit, react-diff-view, monaco diff, assistant-ui) remains candidates for the workshop's binding picks — this report only settles SPA-vs-hypermedia. Reasoning: S1.3 + S3.5 + S3.6 are exactly the widget-dense, high-frequency-update quadrant that the hypermedia school itself declares its weak spot [S62], and every shortlisted component is React-native. HTMX 2.x is the runner-up with the field's best longevity paper ("supported in perpetuity", demonstrated by parallel 2.x/4.x maintenance [S52–S54]) — it loses here only because it re-opens every rich widget; the hybrid (server shell + React islands) was rejected for carrying both toolchains at bus factor 1. **Discipline riders that make the SPA churn tax bounded:** lockfile-pinned minimal dependency tree; the P-T16-1 manifest covers frontend components too; a scheduled ⚙ quarterly dependency pass; Vite majors adopted on a lag, never day-one. Nexus's 11.7k-line *untyped, hand-rolled* SPA is the anti-pattern being corrected — typed framework + adopted components, not artisanal vanilla JS at scale.

### 4.4 AuthN/AuthZ: three layers, person identity in the app (15.6)

1. **Network wall:** the app exists only behind `tailscale serve` (D1); nothing listens beyond localhost (§4.1 invariant).
2. **Device signal:** `Tailscale-User-Login` (spoof-resistant given the localhost chain [S90]) *suggests* the account — prefilled login, and optional operator-granted per-device auto-login for personal phones.
3. **Person identity (authoritative):** boring server-side sessions in `platform.db` — session rows owner-attributed like everything else. Login = user picker + per-user PIN/password (argon2id). **Shared devices always require the PIN** (device ≠ person: one active Tailscale account per device [S93]). High-tier approvals (S3.2 High) re-prompt the PIN — approval identity is never inherited from an idle session. Every auth event lands on the event log (12 §4.1 `decision.recorded` sibling).

**AuthZ:** enforced in the control plane's data layer — every query passes through owner-scoped accessors keyed on the settled 15.6 owner columns (08 §4.2); one role bit (operator vs member) implements D10 co-approval. SQLite has no RLS and needs none here: the sole-writer control plane is architecturally the only gatekeeper [S78]. No policy engine — casbin is unnecessary at household n; oso OSS is deprecated [S101, S102]. **Passkeys:** optional v1 enhancement, not the baseline (Windows synced passkeys still "Planned", Linux extension-only, RP-ID pins to the ts.net hostname [S96–S99]). **tsidp:** watchlist only [S95].

### 4.5 Settings machinery (13.4 + the G1 rider, made mechanical)

One **settings registry in code**: every setting declared once — key, type, default, group/section, title, plain-language help (13.5), ⚙ flag, optional `(floor, ceiling)` bounds, restart-required flag. The registry emits a JSON Schema that drives all three consumers: (a) validation-on-write in the control plane; (b) the **generated settings UI** — grouping/categorization/conditional visibility per the JSON Forms pattern (renderer choice RJSF-vs-JSON-Forms is a workshop pick with the rest of the frontend [S123, S124]); (c) the settings reference docs. Storage is the convergent hybrid [§2.9]: bootstrap-only config (bind address, DB path, broker key via systemd-creds) in `/etc/sinet/`; **everything else is rows in `platform.db`** — default lives in code, the row stores only the override, reset-to-default = delete the row (constance model [S128]). **The G1 rider lands as schema:** every ⚙ number from reports 01–16 seeds a registry entry; auto-adjustable settings store `(value, floor, ceiling)` with HPA clamp semantics — automation may move value only within bounds, only the operator edits bounds [S129]; every write appends a `settings_events` row (actor, key, old, new, timestamp, reason) *and* a `settings.changed` event on the main event log — the audit trail no library ships [S128]. 13.4's enumerated settings (flat-rate/metered flags, price table, budgets, freshness thresholds, depth cap, retention, missed-slot defaults) all live in this registry; the D5 price table is its own owner-attributed table under the same audit pattern.

### 4.6 Real-time: confirmed, one addition

Report 12 §4.3 stands unchanged (one SSE endpoint, `event_seq` ids, `Last-Event-ID` + `?after_seq`, snapshot-then-tail, keepalives, polling fallback). This topic adds: **HTTP/2 terminates at `tailscale serve`** so the browser leg multiplexes (6-connection limit retired [S75]); Caddy passes `/events` unbuffered; Declarative Web Push's `app_badge` becomes the pending-approvals badge on phone home screens [S69]. WebTransport: watch-only [S76].

### 4.7 Storage posture: confirmed, with three operational riders

The settled design (08 §4.1) survives every 2026 check [§2.7]. Riders this topic adds: (1) **SQLite currency policy** — bundle/pin the current release line and take patch releases deliberately (the 3.51.3/3.53.0 WAL-reset corruption fix is the standing example of why [S83]); (2) **bulk-write chunking** — nothing writes >100 MB in one WAL transaction [S79]; (3) **backup belt-and-braces** — Litestream pinned (release-notes review before any bump — the 0.5.13 revert episode [S87]) *plus* a periodic `sqlite3_rsync` point-in-time snapshot feeding the report 13 §4.9 encrypted off-host path and its restore drill [S84]. T07/T11 schema loads (event log per paid call; verdict/routing events) re-checked against §2.7 ceilings: 3–4 orders of headroom — nothing in the feature list forces more than SQLite at household scale.

### 4.8 Packaging, CI, logs — and the P-T16-1 checklist's home

**Native deploy under systemd, never containerized** (§3 table; sandboxes are T09's separate machinery). Go: single binary + embedded assets; `/etc/sinet` + `/var/lib/sinet` via `ConfigurationDirectory=`/`StateDirectory=` [S1]. Logs: stdout→journald; per-unit rate-limit overrides on lane units; `SystemMaxUse` capped ⚙; **journald is the ops log — `platform.db`'s event log stays the only audit truth** (P-T11-4 spirit) [S9–S11].

**CI (D9):** GitHub Actions on tag → build, test, release artifact + SHA256SUMS + **signed tag**; the host deploy script verifies checksum + tag signature, installs, `systemctl restart`, `is-active` gate. **Artifact attestation is not available to this repo** — private repos require GitHub Enterprise Cloud (Free/Pro/Team: public only; resolved from the github/docs source, which no longer renders this note on the live page [S26]) — the checksum+signed-tag fallback preserves most of the value; revisit only on a plan change.

**The component-onboarding checklist (P-T16-1) lives here:** a `components.lock` manifest in the platform repo — one entry per adopted organ and per frontend component: exact pin, license + **path-scoped** license-check date (P-T16-2), replacement-or-rebuild path, abandonment criteria (⚙ N months without release/security response → exit plan activates), last-review date. CI fails if a running unit or bundled dependency lacks a manifest entry; the quarterly review is co-scheduled with the report 08 restore drill. This makes the G2 adoption law a *mechanical* property of the stack, not a document.

### 4.9 Sleep/wake: a first-class platform duty

- **Pre-sleep:** `sinet-control` holds a **delay-mode inhibitor lock** on `sleep` (the sanctioned mechanism; hook scripts are "hacks" per systemd [S6, S7]); on `PrepareForSleep(true)`: stop queue claiming, checkpoint, `wal_checkpoint(TRUNCATE)`, mark in-flight lanes parked. Default window is 5 s (`InhibitDelayMaxSec` [S8]) — the flush is designed to fit it; raising the limit is an operator setting ⚙, not an assumption.
- **Post-resume:** `PrepareForSleep(false)` plus wall-vs-monotonic jump detection as backup (the Tailscale netmon pattern [S16]) triggers: report 08 §4.4's recovery ladder, then the **network-identity reconcile** — health-check tailscaled/serve and remediate through a deliberately designed privileged path (issue #10688 is documented reality [S17]; → P-T13-1, §7). Schedules: `Persistent=true` calendar timers + the 13.4 missed-slot policy [S3]. The 12 §4.4 watchdog suite gains a resume-reconcile check.

### 4.10 What would change the decision

- **Language:** operator declares Go velocity unacceptable at G3 → Python/Litestar variant; §4.1's decomposition, schema, and IPC are unaffected.
- **Frontend:** the workshop finds the shortlist replaceable by framework-agnostic widgets at acceptable cost → HTMX 2.x re-opens (it holds the longevity high ground); Datastar earns a re-look at the first major frontend overhaul if its 1.x line proves stable through ~2027.
- **Push:** the week-one real-device drill (§7 OQ4) fails on household iPhones → accept ntfy-with-relay as the notification channel and keep Web Push for badges only.
- **CI:** GitHub plan change → adopt artifact attestation (one workflow step).
- **Scale/topology:** a second host or >LAN exposure (D1 change), sustained >100s writes/s, or multi-process writers — none plausible at household n — would reopen storage and decomposition.

---

## 5. What NOT to use and why

- **A second Nexus-shaped monolith** (one process, hand-rolled SPA, no seams) — the post-mortem's named anti-pattern; §4.1's seams exist to make it structurally impossible.
- **Microservices / gRPC / message brokers / Redis** — no single-host advocate in the field; every added standing service is bus-factor-1 ops surface; SQLite-as-queue is mainstream [S13–S15].
- **Postgres at v0** — no forcing shape applies (sqlite.org's own checklist [S78]); a second DBMS is the largest single ops-surface add available for zero capability.
- **Containerizing the platform itself** (Quadlet or otherwise) — the control plane's job is host management (systemd-run, bwrap, tailscale CLI, journald, GPU broker); containerizing it inverts the design [S19]. Sandboxes remain T09's separate, ratified machinery.
- **User units + linger as the platform home** — `WakeSystem=` is system-manager-only and some namespacing-based hardening is unavailable without `PrivateUsers=` workarounds [S1, S3]; system units cost nothing here.
- **`DynamicUser=`** for any state-owning unit — recycled UIDs vs a decade-lived DB [S1].
- **FastAPI as the decade bet** (if Python wins G3) — 0.x after 8 years, ~20× single-maintainer commit concentration [S27, S28]; Litestar/Starlette posture instead.
- **A TypeScript/Node control plane** — documented structural churn: 5 LTS migrations/decade, Bun acquired mid-rewrite, npm's registry-operator-confirmed supply-chain crisis [S44–S49]. (The *frontend* build using Vite/npm is accepted and bounded: dev-time, lockfile-pinned, no runtime exposure.)
- **HTMX/Datastar for this widget profile** — honest rejection, not dismissal: hypermedia's own doctrine flags the S1.3/S3.5/S3.6 quadrant [S62]; Datastar's 1.0 is three months old [S55]. Both carry named re-entry conditions (§4.10).
- **Meta-framework/SSR/RSC layer** — the Dec-2025 RCE class lived there [S57]; no tailnet app needs it.
- **WebSocket as primary transport / WebTransport** — settled and re-confirmed; WebTransport is the wrong shape and only just cross-browser [S75, S76].
- **Tailscale-only auth** — device identity is not person identity [S93]; an agent platform holding credentials and taking actions cannot let a lost phone act with full power silently.
- **Passkeys as the sole factor; tsidp as SSO; oso** — sync gaps + RP-ID pinning [S96–S99]; experimental [S95]; deprecated [S102].
- **Any "self-hosted Web Push relay"** — the category cannot exist: the UA chooses its push service (RFC 8030 [S65]). ntfy/Gotify are separate app-push systems, not Web Push.

---

## 6. Harvest-map verdicts

| Item | Verdict | Reasoning |
|---|---|---|
| **A1** Archon dashboard components (map: PORT/ADAPT) | **CONFIRM report 14 §6.2's REVISE** (STUDY now; PATTERN at 15.5) — stack axis adds evidence, changes nothing | Stack-compatibility check result: post-relaunch Archon is TS/Bun with React components coupled to its own 15-table schema and 2-person-bus-factor release pace — transplanting inherits that churn (the fork antipattern in frontend clothes). With §4.3's SPA recommendation, Archon's Mission-Control layout and DAG-view interaction ideas stay *reading material*; actual components come from the generic §3.3 shortlist under the P-T16-1 manifest. |
| **A5** 7-table schema sanity reference (map: STUDY) | **CONFIRM — job done** (as report 14 already ruled) | Report 08 §4.2's durable set is the schema of record; A5 served as its sanity mirror and is now historical. Nothing at the stack layer (owner threading, settings tables, session rows — all additive to 08's set) changes that. |
| **Anti-harvest: server.py monolith topology** | **CONFIRM — this report is its encoding** | The lesson now rests on three independent evidence legs: Nexus's own numbers (11,476-line `server.py`, 248 routes, 32 tables, 11.7k-line untyped SPA — no seams); report 14 §2.2's cohort mortality (funded teams drowned in big agent-platform surfaces, ~14-month half-life); and the survivor cohort's inverse shape (§2.1 — small, boring, SQLite, scope refusal). §4.1's five seams are the operational form; the P-T16-1 manifest keeps adopted organs from quietly growing into a second monolith. |

---

## 7. Open questions

1. **OQ1 — Backend language ratification (owner: operator, G3).** Go is recommended on the external evidence; the unmeasurable input is operator velocity/preference at bus factor 1. G3 asks directly. If Python: ratify the Litestar-vs-Starlette posture in the same breath. Everything else in this report survives either answer.
2. **OQ2 — Frontend workshop agenda (owner: operator + spec, P2 — already owed per G2).** Ratify SPA-with-discipline (§4.3); make the binding component picks from the §3.3 shortlist; **add the settings-form renderer (RJSF vs JSON Forms) to the agenda** — it inherits the same React decision.
3. **OQ3 — Hostname/tailnet naming as a commitment (owner: operator, before first cert).** The ts.net machine name lands in public CT logs [S92] and becomes the passkey RP-ID anchor [S99]. Pick a bland, stable name once; renaming later strands credentials and re-publishes.
4. **OQ4 — Week-one real-device push drill (owner: operator, at first deploy).** Verify on actual household phones: iOS Home-Screen install + Declarative push + `navigate` deep-link; Android classic push; the tap-with-tailnet-down edge (notification arrives, link fails until VPN resumes — angle C found no vendor doc for the combined scenario; it must be observed). Acceptance: notified → glance → decide < 10 s (S3.7).
5. **OQ5 — Shared-device policy default (owner: spec, §10/13.4 defaults).** PIN always required on shared devices; per-device trusted auto-login only on personal devices, operator-granted. Confirm as the shipped default.
6. **OQ6 — Privileged resume-remediation path (owner: spec, with T09 review).** Restarting tailscaled after a failed resume needs root; design the sanctioned path (scoped polkit rule vs a minimal root helper unit) deliberately — it is new privileged surface (see P-T13-1).
7. **OQ7 — `settings.changed` on the main event log (owner: spec).** §4.5 emits settings writes both to `settings_events` and as an event-log type; confirm the event-type addition to report 12 §4.1's contract at spec time.

**New platform problems (for the spec's Known-problems list):**

- **P-T13-1 — Post-resume network-identity reconcile.** tailscaled is documented to sometimes not reconnect after sleep (#10688 [S17]); a platform promising "unattended while up" on a laptop must own resume detection (logind signal + clock-jump backup), connectivity verification, and a *sanctioned* privileged remediation — which is itself new attack surface to design, not an afterthought. Owner: spec (platform shell / ops).
- **P-T13-2 — Listener-binding audit as a trust-chain invariant.** The whole identity story (serve headers) and the tailnet wall both collapse silently if any backend unit ever binds beyond localhost [S90]. A deterministic startup lint + recurring watchdog check ("no sinet process listens on non-loopback except tailscaled/caddy front door") joins the 12 §4.4 suite; config drift here is a cousin of T09's config-poisoning class. Owner: spec (platform shell), watchdog suite.
- **P-T13-3 — Accepted-external-observables register.** Three metadata channels leave the LAN by design: the ts.net hostname in public CT logs [S92], push timing/volume through vendor push services [S65, S66], and LE issuance events. None carries content. The spec should carry a one-page register the operator signs once, and 13.5 approval-help text should reference it — silence would violate the platform's own honesty standard. Owner: spec (trust section), operator sign-off.

---

## 8. Sources

All accessed 2026-07-17. [P] primary / [S] secondary. Flagged-old sources are stable references (specs, decision records) unless noted.

**systemd / process / ops**
- S1 https://man7.org/linux/man-pages/man5/systemd.exec.5.html [P, systemd 261~rc1] — user-unit sandboxing limits ("some… not available", PrivateUsers carve-out); DynamicUser recycling; StateDirectory/ConfigurationDirectory mapping; hardening set.
- S2 https://man7.org/linux/man-pages/man5/systemd.service.5.html [P] — Restart=on-failure recommendation; Type=notify; WatchdogSec/SIGABRT semantics.
- S3 https://man7.org/linux/man-pages/man5/systemd.timer.5.html [P] — monotonic timers pause in suspend; WakeSystem privileges; Persistent= missed-run semantics.
- S4 https://man7.org/linux/man-pages/man1/loginctl.1.html [P] — enable-linger semantics.
- S5 https://man7.org/linux/man-pages/man5/user@.service.5.html [P] — per-user manager lifecycle.
- S6 https://man7.org/linux/man-pages/man8/systemd-suspend.service.8.html [P] — sleep hooks are "hacks"; use inhibitor locks.
- S7 https://man7.org/linux/man-pages/man1/systemd-inhibit.1.html [P] — delay-mode inhibitor mechanics.
- S8 https://man7.org/linux/man-pages/man5/logind.conf.5.html [P] — InhibitDelayMaxSec default 5 s.
- S9 https://man7.org/linux/man-pages/man8/systemd-journald.service.8.html [P] — stdout→journald default plumbing; native socket.
- S10 https://man7.org/linux/man-pages/man7/systemd.journal-fields.7.html [P] — unforgeable underscore trusted fields.
- S11 https://man7.org/linux/man-pages/man5/journald.conf.5.html [P] — rate limits (10k/30s per service), SystemMaxUse defaults, vacuum semantics.
- S12 https://github.com/paperless-ngx/paperless-ngx/tree/main/scripts [P] — live 4-unit + socket decomposition of a self-hosted Python platform; localhost binding.
- S13 https://redis.io/docs/latest/operate/oss_and_stack/management/optimization/benchmarks/ [P; figure old, flagged] — UDS ~50% over loopback TCP.
- S14 https://rubyonrails.org/2024/11/7/rails-8-no-paas-required [P, 2024-11-07, >12 mo, flagged landmark] — DB-as-queue default; broker removal.
- S15 https://github.com/rails/solid_queue [P] — v1.4.0 (2026-03-20); SQLite supported; sequential-writes note.
- S16 https://github.com/tailscale/tailscale/blob/main/net/netmon/netmon.go [P] — wall-vs-monotonic resume detection; stale-NAT rationale.
- S17 https://github.com/tailscale/tailscale/issues/10688 [P] — Linux tailscaled reconnect-after-sleep failures, open 2023→2026.
- S18 https://tailscale.com/kb/1023/troubleshooting [P, updated 2026-01-05] — no sleep/wake guidance exists (negative finding).
- S19 https://docs.podman.io/en/latest/markdown/podman-systemd.unit.5.html [P] — Quadlet mechanics (candidate rejected for the platform itself).
- S20 https://specifications.freedesktop.org/basedir/latest/ [P, v0.8 2021, flagged old-but-current] — XDG layout for user-scope services.
- S21 https://github.com/astral-sh/uv/releases [P] — 0.11.29 (2026-07-15), pre-1.0.
- S22 https://docs.astral.sh/uv/guides/integration/docker/ [P] — `uv sync --locked` as the documented deploy pattern.
- S23 https://docs.astral.sh/uv/guides/integration/github/ [P] — pinned-uv CI pattern.
- S24 https://docs.github.com/en/actions/concepts/security/artifact-attestations [P] — SLSA v1.0 Build L2 by default.
- S25 https://docs.github.com/en/actions/security-for-github-actions/using-artifact-attestations/using-artifact-attestations-to-establish-provenance-for-builds [P] — private repos use GitHub's Sigstore instance, no transparency log.
- S26 github/docs source: `data/reusables/gated-features/attestations.md`, rendered via the S25 how-to page [P, verifier-resolved 3/3] — "To use artifact attestations in private or internal repositories, you must be on a GitHub Enterprise Cloud plan" (Free/Pro/Team: public repos only; one verifier found the note rendered on the how-to page, one only in the docs-source file — cite both).

**Backend languages**
- S27 https://api.github.com/repos/fastapi/fastapi/releases/latest [P] — 0.139.2, 2026-07-16.
- S28 https://api.github.com/repos/fastapi/fastapi/contributors?per_page=5 [P] — tiangolo 2,206 vs next human 111 (~20×).
- S29 https://lp.jetbrains.com/python-developers-survey-2024/ [P for own data; ~20 mo, flagged] — FastAPI 38% usage.
- S30 https://api.github.com/repos/pydantic/pydantic/releases/latest [P] — v2.13.4.
- S31 https://api.github.com/repos/litestar-org/litestar/releases/latest [P] — v2.24.0, org-maintained.
- S32 https://raw.githubusercontent.com/astral-sh/ty/main/README.md (+releases) [P] — "currently in beta", 0.0.x.
- S33 https://devguide.python.org/versions/ [P] — annual cadence, 5-year EOL.
- S34 https://docs.python.org/3/whatsnew/3.14.html [P] — asyncio daemon-debugging tooling; free-threaded support.
- S35 https://platform.claude.com/docs/en/api/client-sdks [P] — Python listed first; Pydantic integration.
- S36 https://api.github.com/repos/pydantic/pydantic-ai/releases/latest [P] — v2.12.0 (2026-07-17).
- S37 https://go.dev/doc/go1compat [P] — the Go 1 promise, verified verbatim.
- S38 https://go.dev/doc/devel/release [P] — 1.26 current (2026-02-10); support policy.
- S39 https://go.dev/blog/routing-enhancements [P, 2024-02, flagged permanent record] — stdlib routing.
- S40 https://pkg.go.dev/modernc.org/sqlite [P] — v1.54.0 pure-Go, wraps SQLite 3.53.3.
- S41 https://api.github.com/repos/mattn/go-sqlite3/releases/latest [P] — v1.14.48, active.
- S42 https://api.github.com/repos/anthropics/anthropic-sdk-go/releases/latest [P] — v1.58.0 GA (2026-07-16).
- S43 https://api.github.com/repos/openai/openai-go/releases/latest [P] — v3.43.0 (two breaking majors since GA).
- S44 https://raw.githubusercontent.com/nodejs/Release/main/README.md [P] — LTS cadence; v24 Active LTS.
- S45 https://bun.com/blog [P] — "Bun is joining Anthropic" (2025-12-02); "Rewriting Bun in Rust" (2026-07-08).
- S46 https://api.github.com/repos/denoland/deno/releases/latest [P] — v2.9.3.
- S47 https://expressjs.com/en/blog/2024-10-15-v5-release [P, >12 mo, flagged] — ten-year v5 gap.
- S48 https://api.github.com/repos/honojs/hono/releases/latest [P] — v4.12.30.
- S49 https://github.blog/security/supply-chain-security/our-plan-for-a-more-secure-npm-supply-chain/ [P, 2025-09-22] — Shai-Hulud worm; forced registry auth overhaul.
- S50 https://mcfunley.com/choose-boring-technology [S, 2015, flagged reference text] — innovation-token framing.
- S51 https://miniflux.app/opinionated.html [P] — solo-survival strategy in writing.

**Frontend / push / realtime**
- S52 https://htmx.org/essays/future/ [P, 2025-01] — stability doctrine.
- S53 https://htmx.org/essays/the-fetchening/ [P, 2025-11-01] — htmx 4 breaking major; "htmx 2.0 … supported in perpetuity" (verified verbatim).
- S54 https://api.github.com/repos/bigskysoftware/htmx/releases [P] — 2.0.9 (2026-04-20) ∥ 4.0.0-beta5 (2026-06-26).
- S55 https://api.github.com/repos/starfederation/datastar/releases [P] — 1.0.0 (2026-04-16), 1.0.2 (2026-06-02).
- S56 https://data-star.dev/ [P] — SSE-native hypermedia model.
- S57 https://react.dev/blog [P] — CRA sunset (2025-02-14); RSC critical RCE (2025-12-03); Foundation (2026-02-24).
- S58 https://vite.dev/blog [P] — Vite 8 (2026-03-12); near-annual majors.
- S59 https://svelte.dev/blog [P] — Svelte 5 calm through 2026-07.
- S60 https://api.github.com/repos/alpinejs/alpine [P] — v3 for ~5 years, active.
- S61 https://api.github.com/repos/lit/lit/releases [P] — lit@3.3.3.
- S62 https://htmx.org/essays/when-to-use-hypermedia/ [P, 2022, flagged old-but-canonical] — hypermedia's weak quadrant; embed-SPA-islands guidance.
- S63 https://htmx.org/examples/sortable/ [P] — official SortableJS drag-drop island pattern.
- S64 https://htmx.org/essays/a-real-world-react-to-htmx-port/ [S, 2022, advocacy venue, flagged] — Contexte −67% LOC port.
- S65 https://datatracker.ietf.org/doc/html/rfc8030 [P, 2016, stable standard] — UA-chosen push service; no substitution.
- S66 https://web.dev/articles/push-notifications-overview [S, 2020, flagged; restates RFC] — "you as a website developer have no control over that".
- S67 https://developer.mozilla.org/en-US/docs/Web/API/Push_API [P, living] — SW requirement; encrypted payloads.
- S68 https://webkit.org/blog/13878/web-push-for-web-apps-on-ios-and-ipados/ [P, 2023, policy-defining, flagged] — iOS 16.4 Home-Screen-only baseline.
- S69 https://webkit.org/blog/16535/meet-declarative-web-push/ [P, 2025-03-27] — no-SW push; required `navigate`; `app_badge`.
- S70 https://webkit.org/blog/16574/webkit-features-in-safari-18-4/ [P, 2025-03-31] — Declarative push "for web apps added to the Home Screen".
- S71 https://caniuse.com/push-api [P, live] — iOS "partial" through 26.x; Android standard.
- S72 https://docs.ntfy.sh/config/ [P, living] — iOS instant push requires upstream ntfy.sh relay (verified verbatim); Android fully self-hosted.
- S73 https://gotify.net/ [P] — Android-only client.
- S74 https://unifiedpush.org/ [P] — Android/Linux distributors; no iOS.
- S75 https://developer.mozilla.org/en-US/docs/Web/API/EventSource [P, living] — Baseline; 6-connection HTTP/1.1 limit; HTTP/2 ~100 streams.
- S76 https://caniuse.com/webtransport [P, live] — cross-browser only since Safari 26.4.

**Storage / auth**
- S77 https://sqlite.org/limits.html [P, upd. 2026-03-11] — 281 TB; row/page bounds.
- S78 https://sqlite.org/whentouse.html [P, upd. 2025-05-31] — the four client/server-forcing shapes (verified 3/3); "choose SQLite!" guidance.
- S79 https://sqlite.org/wal.html [P] — FULL-vs-NORMAL sync points; >100 MB/1 GB transaction caveats.
- S80 https://sqlite.org/faq.html [P; era-old figures flagged, 2024 note] — fsync-bound commit rate.
- S81 https://sqlite.org/fts5.html [P] — in-amalgamation, default-on, still gaining features.
- S82 https://sqlite.org/json1.html [P] — JSON default since 3.38; JSONB since 3.45.
- S83 https://sqlite.org/changes.html [P] — 3.53.3 (2026-06-26); WAL-reset corruption fix 3.51.3/3.53.0 (verified 3/3); no BEGIN CONCURRENT in 2025–26 notes.
- S84 https://sqlite.org/rsync.html [P] — live-DB-safe; ≥3.50 restrictions removed.
- S85 https://sqlite.org/src/doc/begin-concurrent/doc/begin_concurrent.md [P] — branch-only.
- S86 https://sqlite.org/hctree/doc/hctree/doc/hctree/index.html [P] — incomplete prototype.
- S87 https://github.com/benbjohnson/litestream/releases [P] — v0.5.14 (2026-07-06) latest; 0.5.13 revert episode.
- S88 https://ultrathink.art/blog/sqlite-in-production-lessons [S, 2026-04-03] — data loss = single-writer violation (blue-green containers).
- S89 https://www.bendangelo.me/2026/07/04/avoiding-sqlite-database-locks-in-production/ [S, 2026-07-04] — locks = accidental second writer; WAL + busy_timeout fix.
- S90 https://tailscale.com/kb/1312/serve [P, upd. 2026-01-20] — identity headers injected/stripped; Funnel/tagged absent; tailnet-only TLS auto-provision; localhost best practice (verified 3/3).
- S91 https://tailscale.com/kb/1244/tsnet [P, validated 2026-07-07] — WhoIs consumer pattern.
- S92 https://tailscale.com/kb/1153/enabling-https [P, validated 2025-12-10] — CT public-ledger warning; 90-day manual renewal for file-based certs.
- S93 https://tailscale.com/kb/1225/fast-user-switching [P, validated 2025-12-04] — one active account per device (verified verbatim).
- S94 https://pkg.go.dev/tailscale.com/client/local [P, v1.100.0] — WhoIs API surface.
- S95 https://github.com/tailscale/tsidp [P] — experimental; WIP flag; v0.0.14.
- S96 https://github.com/go-webauthn/webauthn/releases [P] — v0.17.4 (2026-05-22).
- S97 https://api.github.com/repos/duo-labs/py_webauthn/releases/latest [P] — v3.0.0 (2026-06-29).
- S98 https://passkeys.dev/device-support/ [P/community, upd. 2026-05-20] — Windows synced passkeys unshipped (hand-maintained matrix; verifier reads split between "Planned" and blank cell — 3/3 agree on the substance: not shipped; eyeball before making load-bearing).
- S99 https://developer.mozilla.org/en-US/docs/Web/API/Web_Authentication_API [S, living] — secure context; RP-ID origin binding.
- S100 https://grafana.com/docs/grafana/latest/setup-grafana/configure-security/configure-authentication/auth-proxy/ [P] — trusted-header auth pattern + spoofing warning.
- S101 https://api.github.com/repos/casbin/casbin/releases/latest [P] — v3.11.0-snapshot.7 (2026-06-23; snapshot tag — "active" with that caveat).
- S102 https://github.com/osohq/oso [P] — deprecation banner ("support and critical bug fixes"; not EOL).
- S103 https://hn.algolia.com/api/v1/search?query=%22behind%20tailscale%22 [S] — defense-in-depth norm in dated threads (2025–2026).

**Reference platforms / settings**
- S104 https://github.com/pocketbase/pocketbase [P] — Go, v0.39.7 (2026-07-16), 59.8k stars, pre-1.0 warning (README).
- S105 https://pocketbase.io/faq/ [P] — solo, "no paid team or company behind it" (verified verbatim).
- S106 https://pocketbase.io/docs/api-realtime/ [P] — "Realtime API is implemented via Server-Sent Events (SSE)" (verified verbatim).
- S107 https://pocketbase.io/docs/going-to-production/ [P] — settings in DB as plain JSON; `--encryptionEnv`.
- S108 https://github.com/miniflux/v2 [P] — Go, Postgres-only, v2.3.2 (2026-06-27), primary maintainer.
- S109 https://github.com/home-assistant/core [P] — Python monolith, Open Home Foundation, 2026.7.2.
- S110 https://www.home-assistant.io/integrations/recorder/ [P] — SQLite "default, and recommended".
- S111 https://developers.home-assistant.io/docs/frontend/architecture/ [P] — custom web-components + WS/REST (org-scale affordance).
- S112 https://grafana.com/docs/grafana/latest/setup-grafana/configure-grafana/ [P] — sqlite3 in-binary default; ini/env sprawl, restart-to-apply.
- S113 https://docs.gitea.com/administration/config-cheat-sheet [P] — "DEPRECATED … moved to database. Use admin panel to configure" (verified verbatim).
- S114 https://forgejo.org/faq/ [P] — community hard fork, GPLv3+ since v9.
- S115 https://github.com/advplyr/audiobookshelf [P] — Node+Vue, SQLite (sequelize), solo-led.
- S116 https://actualbudget.org/docs/contributing/project-details/architecture/ (+repo) [P] — local-first SQLite, community-run.
- S117 https://github.com/binwiederhier/ntfy [P] — Go + SQLite cache; SSE/WS/poll delivery; solo.
- S118 https://vikunja.io/docs/config-options/ (+repo) [P] — Go+Vue, SQLite default.
- S119 https://github.com/sissbruecker/linkding [P] — Django + SQLite default, solo.
- S120 https://github.com/dgtlmoon/changedetection.io [P] — Python/Flask, flat-file store, solo+SaaS twin.
- S121 https://selfh.st/survey/2025-results/ [S, 2025-11-21, self-selected sample, fetched via mirror] — 4,081 responses; containers near-universal for *consumers* of self-hosted apps (deployment preference, not producer stack); category winners.
- S122 https://pydantic.dev/docs/validation/latest/concepts/pydantic_settings/ [P] — layered source precedence; validated defaults.
- S123 https://github.com/rjsf-team/react-jsonschema-form [P] — v6.6.2 (2026-06-06), org-maintained, ten themes.
- S124 https://jsonforms.io/ [P] — v3.8.0; group/categorization layouts; rule-based visibility.
- S125 https://www.home-assistant.io/blog/2020/04/14/the-future-of-yaml/ [P, 2020, flagged decision record] — why runtime config moved out of files.
- S126 https://developers.home-assistant.io/docs/config_entries_index/ [P] — versioned config entries + mandatory migrations.
- S127 https://docs.gitlab.com/ee/user/compliance/audit_events.html [P] — who/what/when audit-event triple.
- S128 https://django-constance.readthedocs.io/en/latest/ [P] — default-in-code + override-row + reset=delete; `config_updated` signal; **no built-in history**.
- S129 https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/ [P] — min/max bounds-clamp as named prior art for operator-ceilinged auto-adjustment.

**Internal:** feature list (Docs/agent-platform-feature-list-v1.md), Nexus post-mortem (Docs/nexus-post-mortem.md), harvest map (Docs/component-harvest-map-proposal-v1.md), reports 01, 02, 08, 10, 12, 13, 14, GATE-1 (settings rider), GATE-2 (adoption law, settled organs).
