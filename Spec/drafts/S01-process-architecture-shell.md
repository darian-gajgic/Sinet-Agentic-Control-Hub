## S01 — Process architecture & platform shell

**Scope:** The process/unit topology of the owned core and its adopted organs, the five replacement seams, backend language and release artifact, platform lifecycle (startup, shutdown, maintenance/drain, sleep/wake), deploy/CI/logs, the authentication stack, and the settings-registry architecture.
**Binding inputs:** R17 (primary) · G1 Def.6, Def.7, riders 1–2 · G2 D2.1, D2.2, D2.5, Def.12 · G3 D3.2, D3.3, Def.1, Def.2, Def.5, Def.7 · [SPIKE P2-S3] · feature list D1, D2, 4.5, 13.4, 13.5, 15.6, Operating reality · `Spec/frontend-components-v1.md` (picks referenced, never restated).

### S01.1 Shape: small owned core + adopted organs

Sinet is one small owned daemon (`sinet-control`) surrounded by single-purpose organs, every one a systemd **system** unit with its own journal identity [R17 §4.1; G2 D2.1]. Internally `sinet-control` is a modular monolith with enforced module seams (storage / scheduler / gates / ledger / event-log / adapters); decomposition means *seams*, not network services [R17 §4.1]. The web surface is not a process: the SPA, the chat surface, and any future CLI are all clients of the same HTTP API [R17 §4.1; XREF:S15]. This shape is the direct encoding of the Nexus anti-monolith lesson (`Docs/nexus-post-mortem.md`) and the survivor-cohort pattern [R17 §2.1, §6].

Shell-level invariants (violations are platform defects):

- `sinet-control` is the **sole writer** of `platform.db`; run units NEVER touch the DB [G2 D2.1; R17 §4.1].
- Every sinet unit binds **127.0.0.1 or a unix socket only**; the only sanctioned non-loopback listener on the host is `tailscaled` (front door; see S01.8, P-T13-2) [R17 §4.1].
- Credentials never enter run units (D2); only the broker holds decrypted secrets [R17 §4.1; SPIKE P2-S3].
- All platform units are system units running as one **static `sinet` user**; `DynamicUser=` is NEVER used for state-owning units (UID recycling vs decade-lived DB files) [R17 §4.1, §2.2].
- journald is the ops log only; the `platform.db` event log is the only audit truth [R17 §4.8].

### S01.2 Unit map

| Unit | Kind | Role | Key directives |
|---|---|---|---|
| `sinet-control.service` | owned core | Sole `platform.db` writer; scheduler + queue claiming [XREF:S10]; D7 checkpoint/gate machinery + effect journal [XREF:S02]; event log; HTTP API + the one SSE endpoint [XREF:S15]; adapter supervision [XREF:S03] | `Type=notify`, `WatchdogSec=` ⚙, `sd_notify` heartbeat, `Restart=on-failure`, `StateDirectory=sinet`, `ConfigurationDirectory=sinet`, binds 127.0.0.1 only [R17 §4.1] |
| `sinet-broker.service` | owned | Credential broker: ssh-agent-shaped, operation-in/result-out; secrets at rest via systemd-creds + sops/age [G2 D2.2]; performs git signing/pushes [XREF:S13]. Separate process so the large-attack-surface control plane never holds decrypted member credentials [R17 §4.1]. Internals: [XREF:S11] | UDS with peer-cred auth; no TCP listener |
| `sinet-engine@<user>.service` | owned template | Per-user pinned `opencode serve` instance (Z.AI lane); `claude` CLI runs are per-run subprocesses inside run units, never standing services [R17 §4.1; XREF:S03] | Per-user localhost port; `After=sinet-broker.service` (credentials injected at start, outside any sandbox [SPIKE P2-S3]); `Restart=on-failure` |
| **run units** (template instances) | owned, per run | See below | `sinet-run@<run_id>.service` instances of a root-installed fixed-`ExecStart` template [XREF:S11]; `Restart=no` |
| `sinet-portpool.service` | owned | Port-pool daemon for preview allocation [G2 D2.1; XREF:S13] | Loopback only |
| `caddy.service` | adopted organ | Front router: `/api` + `/events` proxying (`/events` unbuffered), preview subdomain routes via admin API [R17 §4.1, §4.6; XREF:S13] | Localhost bind behind `tailscale serve` |
| `changedetection.io` unit | adopted organ | Watchlist executor [G2 Def.12; XREF:S14] | Own unit, own journal identity |
| local-model unit(s) | adopted organ | llama-swap front + llama-server backends [XREF:S12] | Own unit(s) [XREF:S12] |
| `tailscaled.service` | host-managed | Tailnet + TLS front door; not sinet-managed, but sinet health-checks it (P-T13-1, S01.7) | — |

**Run unit** (term coined here): the per-run, ephemeral systemd unit that hosts one run's engine process inside its sandbox — the process form of a run executing on a lane — realized as an instance `sinet-run@<run_id>.service` of a root-installed template [G1 Def.6; R17 §4.1; XREF:S11 — S11.8 Shape B]. Run units compose the sandbox stack [XREF:S11] around the run's workspace [XREF:S02], carry PID-1-enforced time ceilings and cgroup accounting (the cost-ceiling backstop [G1 Def.6; XREF:S10]), and stream events and checkpoints to `sinet-control` over the local API — the control plane persists, so sole-writer holds, each run is crash-isolated, and every run is visible in `systemctl`/journald under its own identity [R17 §4.1]. Run units are never auto-restarted by PID 1: a dead run is recovered by the S02 recovery ladder (fork-from-checkpoint), a decision the control plane owns [XREF:S02]. Unit names are the template-instance names `sinet-run@<run_id>.service`, so journal attribution is mechanical (`%i` identity). The mechanism by which the unprivileged `sinet` user starts these system-scope instances — a fixed-`ExecStart` template plus a single name-scoped polkit grant (Shape B) — is S11's privileged-surface design, alongside the resume-remediation path [G3 Def.7; XREF:S11]. *(Wording aligned to S11.8 at assembly, 2026-07-19 [coordinator-draft].)*

The standard hardening set (`ProtectSystem=strict` + `ReadWritePaths`/`StateDirectory`, `NoNewPrivileges=`, `PrivateTmp=`, syscall filter) applies to every owned unit as far as compatible with its duty; per-unit exceptions are recorded in the unit file with a reason [R17 §4.1]. System units (not user units + linger) are load-bearing: user units lack parts of the namespacing hardening and `WakeSystem=` timers, and boot-start needs no login plumbing [R17 §2.2, §5].

### S01.3 The five seams

The five named replacement boundaries [R17 §4.1; G3 digest]. Every future "replace X" conversation happens *at a seam*; anything that cannot be swapped at a seam is a design defect to raise.

| Seam | What it isolates | What swapping at it costs |
|---|---|---|
| **Storage** | The persistence engine behind the control plane's storage module: one writer, read-only opens for everything else | SQLite → client/server DB = rewrite one module + the backup lane; schema and API untouched. Reopened only by named triggers: second host, sustained >100 writes/s, multi-process writers [R17 §4.10] |
| **Process** | Each organ's lifecycle, failure, and logs (own unit, own unforgeable `_SYSTEMD_UNIT` journal identity, own restart policy) | Replace the unit + its `components.lock` entry; core untouched (e.g. watchlist executor → Miniflux fallback [G2 Def.12]) |
| **API** | Every surface from the core: SPA, chat, future CLI are equal clients of the HTTP API + SSE endpoint | Rebuild a surface (e.g. the HTMX re-entry conditions [G3 D3.3]) with zero core change; the cost is the surface itself [XREF:S15] |
| **Adapter** | Engine/provider specifics behind the D3 contract verbs | New engine or provider = one new adapter; orchestration, metering, state untouched [XREF:S03] |
| **Adoption** | Every third-party organ behind a pin + replacement path + abandonment criteria in `components.lock` | Pre-planned per entry; exit plans exist before they are needed [G2 D2.2; XREF:S16] |

### S01.4 Front chain & IPC

**Front chain (the trust chain):** `tailscale serve` (tailnet-only TLS on the ts.net name, injects `Tailscale-User-*` identity headers, strips them from inbound requests) → Caddy (routing; `/events` unbuffered) → `sinet-control` on 127.0.0.1 [R17 §4.1, §2.8]. **HTTP/2 terminates at `tailscale serve`** so the browser leg multiplexes and multi-tab SSE never hits the 6-connection limit [R17 §4.6]. The SPA's static assets are embedded in the control-plane binary and served through this same chain (S01.5); Caddy's role is routing, not asset hosting. Exposure beyond the tailnet does not exist (D1); LAN access rides the tailnet's direct LAN paths.

**IPC map** [R17 §4.1]: browser → serve → Caddy → control (HTTPS + SSE); run units → control (local API: UDS or localhost HTTP — the invariant is *never the DB*, the exact transport is P3's choice within that bound); control → engines (localhost HTTP, per-user ports); control + run units → broker (UDS, operation-in/result-out); queue = SQLite tables claimed only inside the control plane [G2 D2.1]. No gRPC, no message broker, no Redis — every added standing service is bus-factor-1 ops surface with no single-host advocate in the field [R17 §3, §5].

### S01.5 Backend language & release artifact

**The backend language is Go** — for `sinet-control`, `sinet-broker`, and `sinet-portpool` [G3 D3.2]. Grounds as ratified: the go1compat decade-stability promise, compiler-enforced typing at bus factor 1, single-static-binary deploy with embedded assets, pure-Go SQLite (no cgo), GA `anthropic-sdk-go` v1, survivor-cohort convergence [R17 §4.2]. No language couples to adopted organs: engines are subprocesses/HTTP servers behind adapters (D3), evals run via adopted runners, and Python/JS organs are consumed as processes or data files, never linked libraries [R17 §4.2].

**Release artifact:** one static Go binary with the built SPA assets embedded (`go:embed` posture) [R17 §4.2–4.3; G3 D3.3]. The broker and port-pool daemons are the same binary invoked in dedicated modes (multi-call), so deploy verifies exactly one checksummed artifact while the *process* separation of S01.2 is fully preserved [coordinator-draft]. The exact SQLite driver pin lands in `components.lock` [XREF:S16].

**The Python/Litestar fallback** documented in R17 §4.2 remains exactly that: documentation of the considered alternative and its posture. It is not a live option, not a parallel track, and imposes no compatibility duty on this spec [G3 D3.2 — operator decided outright, spike declined].

### S01.6 Startup, shutdown & maintenance mode

**Startup ordering.** `tailscaled` and `caddy` start independently (front door tolerates a not-yet-ready backend). `sinet-broker` starts before `sinet-control` and before any `sinet-engine@` unit (`After=`/`Wants=` — engines receive credentials at start [SPIKE P2-S3]). Organs (`portpool`, watchlist executor, local-model units) start independently; `sinet-control` tolerates their absence as a degraded state surfaced by the watchdog suite [XREF:S14]. On start, `sinet-control` executes, in order:

1. Load bootstrap config (`/etc/sinet`) + settings registry (S01.10).
2. **Listener-binding lint** (P-T13-2): assert its own listeners are loopback-only — failure here is fail-closed (the unit refuses to start with a named error); audit the sinet unit set for non-loopback listeners — a foreign violation surfaces immediately as a High-severity flag [R17 §7; XREF:S14].
3. Run the S02 recovery ladder over runs, effects, and leases [XREF:S02].
4. `sd_notify(READY)`; begin the `WatchdogSec` heartbeat.
5. Resume admission (scheduler claiming) [XREF:S10].

**Shutdown.** `systemctl stop sinet-control` → SIGTERM → stop admission, O(1) flush (identical to the pre-sleep path, S01.7), exit. `TimeoutStopSec` MUST exceed the flush budget. Host shutdown additionally stops run units; any loss is bounded by the last checkpoint (D7) and harvested by the recovery ladder at next start [XREF:S02]. A hard restart is therefore always *safe*, merely impolite — bounded re-spend, never a repeated outward effect (D7).

**Maintenance mode (4.5).** One operator switch:

- **Enter:** admission stops — the scheduler claims nothing new [XREF:S10]; surfaces stay readable; approvals remain answerable but answered items queue rather than launching resumes.
- **Drain:** in-flight runs continue for ⚙ `shell.drain_grace` = **15 min** [G1 Def.7]. Runs that finish, finish normally.
- **Grace expiry:** still-running runs are **parked**: the run unit is stopped and the run's next resume starts from its last checkpoint; loss is bounded by D7. Never a kill of record — parked, flagged, resumable [G1 D1.3 posture].
- **Exit:** admission resumes; parked runs resume per scheduler priority [XREF:S10].

Planned restarts and deploys SHOULD pass through maintenance mode; the deploy script treats it as the polite path, not a precondition (S01.11).

### S01.7 Sleep/wake as a first-class duty

The host is a laptop that sleeps and travels; the shell owns suspend/resume as a lifecycle event, not an anomaly [feature list Operating reality; R17 §2.11].

**Pre-sleep.** `sinet-control` holds a **delay-mode inhibitor lock** on `sleep` (the sanctioned mechanism; hook scripts are "hacks" per systemd) [R17 §4.9]. On `PrepareForSleep(true)`: stop queue claiming, checkpoint, `wal_checkpoint(TRUNCATE)`, mark in-flight run units parked. This is an **O(1) flush designed to fit even the stock 5 s inhibitor window** — recovery work is wake-side, never pre-sleep [R17 §4.9; G2 D2.1]. The v0 host's measured logind state is already 30 s via an existing drop-in [SPIKE P2-S2]; ⚙ `shell.inhibit_delay_max` = 30 s records that reality (applied as host logind configuration; restart-required; the flush never depends on more than 5 s).

**Post-resume.** Resume is detected by `PrepareForSleep(false)` plus wall-vs-monotonic clock-jump detection as backup (the Tailscale netmon pattern) [R17 §4.9]. Then, in order:

1. The S02 recovery ladder (suspend-aware grace applies) [XREF:S02].
2. **Network-identity reconcile** (P-T13-1): health-check `tailscaled` and `tailscale serve` state, verify tailnet connectivity; on failure, remediate through the deliberately designed privileged path — a scoped polkit rule vs minimal root helper decision that S11 owns with T09 review [G3 Def.7; XREF:S11]. `tailscaled` failing to reconnect after wake is documented reality, not a hypothetical [R17 §7 P-T13-1].
3. Re-run the listener-binding audit (P-T13-2) — resume is a config-drift opportunity.
4. The watchdog suite's resume-reconcile check confirms all of the above completed [R17 §4.9; XREF:S14].

**Timers.** Platform-internal schedules (snapshots, quarterly passes, canaries) run as system-manager calendar timers with `Persistent=true`, so a slot missed in suspend fires once on next activation [R17 §4.9]. v0 schedules never wake a sleeping host — availability is best-effort while the host is up [feature list Operating reality]; `WakeSystem=` remains available and unused. User-facing schedules and missed-slot policies are v1 [S00.2; XREF:S10]. `Persistent=` catch-up and cgroup freeze/thaw behavior across a real suspend: TBD-BRINGUP(operator suspend-session probe — reboot-survival + `Persistent=` catch-up + user.slice freeze/thaw).

### S01.8 Trust-chain invariants & accepted external observables

**Listener-binding audit (P-T13-2).** The identity story (serve headers, S01.9) and the tailnet wall (D1) both collapse silently if any backend unit ever binds beyond localhost. Therefore: deterministic startup lint (S01.6 step 2) + recurring watchdog check — "no sinet process listens on non-loopback; the only sanctioned exceptions are the front door (`tailscaled`, and `caddy` only if an explicit front-door binding is ever configured)" — with an explicit, operator-visible allowlist [R17 §7; XREF:S14]. Config drift here is a cousin of the config-poisoning escape class [XREF:S11].

**Accepted-external-observables register (P-T13-3).** Three metadata channels leave the LAN by design; none carries content. The register below is the complete v0 list; the operator signs it once, and 13.5 approval-help text references it [R17 §7]:

| # | Observable | Where it goes | What leaves | What never leaves |
|---|---|---|---|---|
| 1 | ts.net machine hostname | Public Certificate Transparency logs | The hostname string | Anything else about the tailnet |
| 2 | Push timing/volume | Browser-vendor push services (Apple/Google relays) | Timing, volume, endpoint metadata | Payload content (encrypted to subscription keys) |
| 3 | TLS issuance events | Let's Encrypt / CT | Issuance cadence for the hostname | Content, traffic, client identity |

Adding any fourth observable requires amending this register through the S00.9 amendment mechanics — silence would violate the platform's own honesty standard [R17 §7]. Operator sign-off: TBD-OPERATOR(observables-register sign-off — one signature, at G4 or first deploy).

**Hostname prerequisite.** TBD-OPERATOR(ts.net hostname pick — bland + permanent, chosen before the first cert) [G3 Def.5]. Rationale: the name lands in the public CT ledger the moment the first cert is issued (register row 1) and anchors the WebAuthn RP-ID for any future passkeys; renaming later strands credentials and re-publishes the name [R17 §2.8, §7].

### S01.9 Identity: the authentication stack (15.6)

Three layers; the outer two are walls and hints, only the third is authoritative [R17 §4.4].

1. **Network wall.** The app exists only behind `tailscale serve` (D1); nothing listens beyond localhost (S01.1 invariant). Reaching a login screen already proves tailnet membership.
2. **Device hint.** `Tailscale-User-Login` — spoof-resistant given the S01.4 chain, because serve strips inbound copies — *suggests* the account: it prefills the login picker, and on personal devices with an operator grant it may complete login. It is device identity, not person identity: one active Tailscale account per device means a shared tablet surfaces as its registered user [R17 §2.8, §4.4].
3. **Person identity (authoritative).** Server-side sessions as owner-attributed rows in `platform.db` [15.6; schema XREF:S02]. Login = user picker + per-user PIN/password (argon2id). **Shared-device policy [G3 Def.1]:** PIN always required on shared devices; per-device trusted auto-login exists only on personal devices and only by explicit operator grant — the default for every device is shared. High-tier approvals (S3.2 High) re-prompt the PIN: approval identity is NEVER inherited from an idle session [R17 §4.4]. Every auth event (login, failure, grant, re-prompt) lands on the event log [R17 §4.4; XREF:S14].

**AuthZ.** Enforced in the control plane's data layer: every query passes through owner-scoped accessors keyed on the 15.6 owner columns [XREF:S02]; one role bit (operator vs member) implements D10 co-approval. No policy engine at household n [R17 §4.4]. SQLite has no row-level security and needs none: the sole-writer control plane is architecturally the only gatekeeper.

**Parked:** passkeys as an optional v1 enhancement (platform-sync gaps; RP-ID pins to the ts.net hostname — another reason S01.8's hostname pick precedes the first cert); tsidp watch-only [R17 §4.4; G3 follow-ups].

### S01.10 Settings-registry architecture (13.4 + G1 rider 1, made mechanical)

One **settings registry in code**. Every setting is declared exactly once with: key (dotted, per section domain), type, default, group/section, title, plain-language help (13.5), ⚙ flag, optional `(floor, ceiling)` bounds, restart-required flag [R17 §4.5]. The registry emits a JSON Schema that drives all three consumers, so UI, validation, and docs cannot drift apart:

- (a) validation-on-write in the control plane;
- (b) the generated settings UI — grouping, categorization, conditional visibility; renderer per the frontend workshop's binding pick [XREF:S15];
- (c) the generated settings reference docs.

**Storage** is the convergent hybrid [R17 §4.5, §2.9]: bootstrap-only config (bind address, DB path, broker key via systemd-creds) lives in `/etc/sinet/`; everything else is rows in `platform.db` — the default lives in code, the row stores only the override, reset-to-default = delete the row [schema XREF:S02].

**G1 rider 1 as schema:** every ⚙ number ratified anywhere in this spec seeds a registry entry with the ratified value as default. Auto-adjustable settings store `(value, floor, ceiling)`: automation (complexity-based adjustment) may move *value* strictly within bounds; only the operator edits bounds; auto-raises are visible on receipts [G1 rider 1; R17 §4.5; XREF:S10]. Every write — human or automated — appends a `settings_events` row `{actor, key, old, new, timestamp, reason}` AND emits a `settings.changed` event on the main event log [G3 Def.2; XREF:S14]. The D5 price table is its own owner-attributed table under the same audit pattern [XREF:S10].

This section defines the architecture only; the full settings index is S18's sweep of every section's ⚙ table [XREF:S18].

### S01.11 Deploy, CI & logs

**Native deploy under systemd — the platform itself is never containerized.** The control plane's job is host management (transient run units, sandbox composition, `tailscale` CLI, journald, GPU arbitration [XREF:S12]); containerizing it inverts the design. Sandboxes are S11's separate, ratified machinery [R17 §4.8, §3; XREF:S11]. Layout: `/etc/sinet` via `ConfigurationDirectory=`, `/var/lib/sinet` via `StateDirectory=` (home of `platform.db` [XREF:S02]) — zero custom path code [R17 §4.8].

**CI (D9).** GitHub Actions on tag: build → test → release artifact + `SHA256SUMS` + **signed tag**. The host deploy script: verify checksum + verify tag signature → install → (SHOULD: maintenance-mode drain, S01.6) → `systemctl restart` → `is-active` gate [R17 §4.8]. **Artifact attestations are not available to this repo**: on private/internal repos they require GitHub Enterprise Cloud (Free/Pro/Team plans: public repos only) — the documented reason the checksum + signed-tag fallback is the mechanism; it preserves most of the value and is revisited only on a plan change [R17 §4.8, §2.10]. CI additionally fails if any running unit or bundled dependency lacks a `components.lock` entry — the adoption seam enforced mechanically [G2 D2.2; XREF:S16].

**Logs.** Every unit logs stdout → journald; journald is the ops log and never the audit record — the `platform.db` event log is the only audit truth [R17 §4.8]. Underscore journal fields give unforgeable per-unit attribution; run units carry per-unit `LogRateLimitIntervalSec/Burst` overrides (chatty workers); total journal use is capped ⚙ [R17 §2.10, §4.8].

---

**Settings introduced (⚙):**

| Name | Default | Clamp/range | Ratified by |
|---|---|---|---|
| `shell.drain_grace` | 15 min | (1 min, 24 h) [coordinator-draft] | G1 Def.7 |
| `shell.watchdog_sec` | 30 s [coordinator-draft] | (10 s, 300 s) [coordinator-draft] | R17 §4.1 (⚙ flagged unnumbered) |
| `shell.inhibit_delay_max` | 30 s (v0 host measured [SPIKE P2-S2]; logind stock default 5 s) | (5 s, 60 s) [coordinator-draft]; restart-required | R17 §4.9; SPIKE P2-S2 |
| `shell.journal_max_use` | 4 GB [coordinator-draft] | (512 MB, 32 GB) [coordinator-draft]; restart-required | R17 §4.8 (⚙ flagged unnumbered) |

**Known problems owned here:**

- **P-T13-1** — post-resume network-identity reconcile: owned at S01.7 (detection + reconcile duty); the privileged remediation path is designed in S11 with T09 review [G3 Def.7; XREF:S11].
- **P-T13-2** — listener-binding audit: owned at S01.6/S01.8 (fail-closed startup lint + explicit allowlist); the recurring check registers into S14's watchdog suite [XREF:S14].
- **P-T13-3** — accepted-external-observables register: owned at S01.8 (complete v0 register + amendment rule); operator sign-off pending (TBD-OPERATOR).

**Deferred / parked:**

- `litestream.service` unit slot → re-decided at implementation once its silent-replication-failure issue is triaged [G2 D2.5]; the committed backup lane is S13's snapshot pipeline [XREF:S13].
- GitHub artifact attestations → re-entry on a GitHub plan change [R17 §4.8].
- Passkeys → v1 enhancement; re-entry after the hostname commitment (S01.8) and platform-sync maturity [R17 §4.4].
- tsidp as SSO → watchlist only [R17 §4.4].
- `WakeSystem=` wake-the-host scheduling → unused at v0; re-entry only with a user-facing-schedule decision at v1 [S00.2].
- Suspend-cycle probes (`Persistent=` catch-up, freeze/thaw) → TBD-BRINGUP(operator suspend-session probe), tracked in STATE follow-ups.

**Coverage:**

| Feature-list item | Where |
|---|---|
| 4.5 maintenance mode + clean stop | S01.6 |
| D1 exposure (tailnet-only, no other listeners) | S01.1, S01.4, S01.8 |
| D2 credential isolation (broker as separate unit; secrets never in run units) | S01.1, S01.2 |
| 13.4 settings surface (architecture; index → S18) | S01.10 |
| 13.5 plain-language help (as registry field; surface → S15) | S01.10 |
| 15.6 owner on every record (enforcement point: owner-scoped accessors, owner-attributed sessions) | S01.9 |
| 3.8 crash/restart/sleep survival (shell duties; ladder → S02) | S01.6, S01.7 |
| 4.6 nothing fails silently (control-plane self-supervision via watchdog; run watching → S14) | S01.2, S01.6 |
| Operating reality: unattended-while-up on a sleeping, traveling host | S01.7 |
| 11.1 audit substrate posture (journald ops-only; event log = audit truth) | S01.11 |

**Open items for G4:** none. The three drafting-time sub-choices are flagged inline as [coordinator-draft] for G4 attention: the multi-call single-artifact packaging (S01.5), the `shell.watchdog_sec` = 30 s number, and the `shell.journal_max_use` = 4 GB number (plus proposed clamp ranges in the ⚙ table).
