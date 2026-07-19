# P3 implementation — live state

> Any session advancing the build: read this file, then follow `.claude/skills/p3-implementation/SKILL.md`. Update this file before and after every step — it is the single source of truth for build coordination. The contract is `Spec/core-architecture-v1.md` (v1, frozen, tag `spec-v1`); the campaign record in `Research/` is a closed archive.

**Phase status:** **B0 (spine) OPEN — P3-B0-4 running** (B0-1..B0-3 done). `sinet control` lives in dev mode: health + SSE demoed; units generated-not-installed.

**Model routing:** packets consuming S11 (sandbox/broker) or S10-internals content run Opus-pinned per memory `fable5-safeguard-false-positive`; everything else inherits. First Opus-pinned work arrives at B1 (sandbox stack).

## Phase board (S19.5)

| Phase | Scope (owning sections) | Status |
|---|---|---|
| B0 | Spine: sinet-control process, platform.db, event log, settings registry, auth stack, five seams [S01]; run FSM, checkpoint/effect journal, recovery ladder [S02]; adoption manifest + CI lock-gate [S16] | **OPEN** |
| B1 | Execution substrate: one adapter + one lane [S03]; metering ledger + limit taxonomy + scheduler skeleton [S10]; per-run sandbox C0–C2 + credential broker [S11] | pending |
| B2 | Pipeline: intake→spec→plan [S06]; context ledger [S05]; verification [S07] — closes the walking skeleton (gate = live demo) | pending |
| B3 | Workforce & memory: worker registry + composer battery + routing [S08]; memory/knowledge + write gate [S09] | pending |
| B4 | Deliverables & local tier: deliverable/review/git/backup [S13]; local-model stack + GPU broker + duty aliases [S12] | pending |
| B5 | Observability & evals: watchdogs, conformance registry, benchmark machinery, queryable history [S14] — incl. the [schema] STUDY watch row (G4 follow-up) | pending |
| B6 | Frontend: React SPA consuming every API [S15; FC-v1] | pending |

## B0 packet queue

Statuses: `pending` → `running` → `review` → `done`.

| Packet | Title | Read-first sections | Acceptance (headline) | Status |
|---|---|---|---|---|
| P3-B0-1 | Repo scaffold + adoption rail + CONVENTIONS | S01 (esp. S01.5 release artifact), S16, S19.5 | Go module builds a stub `sinet` binary; CI (build + test + lock-gate) green; initial `components.lock`; `P3/CONVENTIONS.md` authored from the sections | done |
| P3-B0-2 | platform.db + settings registry + event log | S01.9–S01.10, S02 (schema/tables), S18 (key index) | DB bootstrap with ratified pragmas + migrations; declare-once registry (clamps, audit events, schema emission); append-only event log with `event_seq`; unit tests | done |
| P3-B0-3 | sinet-control shell + API/SSE skeleton + five seams | S01 full | Dev-mode process serves health + the one SSE endpoint; watchdog wiring; seams as package boundaries; systemd unit files *generated, not installed* (host changes wait for the gate) | done |
| P3-B0-4 | Run FSM + checkpoint/effect journal + recovery ladder | S02 full | FSM with derived-not-stored stalled state; checkpoint rows; two-phase effect journal; recovery ladder + leases/generation fencing; **kill-9 harness v1** in the test suite | running |
| P3-B0-5 | Auth stack (tailnet wall → header hint → sessions + PIN) | S01 (auth), S15.2 (API posture) | Session + PIN machinery with tests; identity-header parsing in dev mode; cert/hostname steps documented for the B0 gate (needs operator item 2) | pending |

**B0 gate (when queue done):** phase report + demo; propose host-side installs (systemd units) for approval; operator item due: the **root/reboot/suspend probe batch** (S19.6 B0/B1 measurements — suspend-session probe records to `P3/measurements/`). Hostname is settled and live: machine `sinet`, FQDN `sinet.tailfd0b1e.ts.net` (amendment A1) — first cert can be issued at this gate.

## Operator hands-on items (carried from G4)

| # | Item | Due at |
|---|---|---|
| 1 | Z.AI dashboard prompt-unit calibration (5-step recipe in P2-S1 report §Blocked) | B1 (metering trust) |
| 2 | ts.net hostname pick — **FULLY DONE 2026-07-19: `sinet`** (spec amendment A1) **and Tailscale machine renamed** (was `sinep-predator`); FQDN `sinet.tailfd0b1e.ts.net`. First cert issuance remains a B0-gate step | done |
| 3 | Root/reboot/suspend probe batch (reboot-survival; `Persistent=` catch-up; user.slice freeze/thaw) | B0/B1 gate |
| 4 | Week-one push drill on household phones | first deploy |
| 5 | (Optional) GitHub Verified badge — signing-key upload | anytime |
| 6 | age identity escrow — paper copy + passphrase-encrypted copy off-host (S13.10; found missing from this table 2026-07-19) | **B4, before first snapshot push** |
| 7 | Observables-register sign-off — one signature on the S01.8 register (deferred at G4) | first deploy |

## Log

- 2026-07-19 — P3 machinery created (skill + this STATE + CLAUDE.md pointer) in the G4-closing session; B0 opened, queue cut from S19.5 + S01/S02/S16 scopes. First build packet deliberately left for a fresh session.
- 2026-07-19 — **Host toolchain:** operator reported Go 1.26 installed, but no Go existed anywhere on the host (PATH, /usr/local, snap, dpkg, version managers all checked). Coordinator installed **Go 1.26.5** user-level: sha256-verified official tarball → `~/.local/go`, wrapper scripts `~/.local/bin/{go,gofmt}` (already on PATH). No root, no host-level change; reversible by deleting those paths. P3-B0-1 launched.
- 2026-07-19 — **Operator item 2 closed: ts.net hostname = `sinet`** (operator pick, chat session). Recorded as spec amendment **A1** (S00.9 post-G4 changelog started; S01.8 TBD-OPERATOR closed in draft + assembled spec). All build code/config referencing the ts.net name uses `sinet` from now on. **Same day, operator request: Tailscale machine renamed** `sinep-predator` → `sinet` (`tailscale set --hostname=sinet`); live FQDN `sinet.tailfd0b1e.ts.net`. Any household device that referenced `sinep-predator` by name must use `sinet` now. First cert still unissued — deliberately left for the B0 gate; nothing is in CT logs yet.
- 2026-07-19 — **P3-B0-3 done** (commit `4bf3415`, validated). `internal/shell` (S01.6 ladder: bootstrap → registry attach/reapply → fail-closed loopback lint P-T13-2 → recovery-ladder stub (B0-4) → READY/watchdog → admission seam (B1); SIGTERM clean shutdown; maintenance switch ⚙ drain_grace; WAL-truncate loop), `internal/api` (`/api/health`, `/events` SSE with id=event_seq + Last-Event-ID/after_seq resume + ⚙ obs.sse_keepalive; identity seam dev-impl pending B0-5), `internal/sdnotify` (stdlib unixgram), seam stubs scheduler/gates/ledger/adapters, `sinet units` (6 S01.2 files incl. journald drop-in; engine@/run@ DRAFTs; A1 hostname; generated-never-installed). Zero new deps. Coordinator: battery + race green; **live demo re-run independently** — health JSON ok, SSE replayed `platform.started` (id:1) from cursor, SIGTERM → exit 0 with ordered shutdown log, `0.0.0.0` bind refused fail-closed exit 1, units rendered to scratch only. 10 readings in the commit body (headline: provisional lifecycle event names pending S14/B5; dev posture = absent systemd env; cursorless SSE tails from head). `sseBatchSize=256` correctly a plain constant (S18 ratifies no such key — documented in-code).
- 2026-07-19 — **P3-B0-2 done** (commit `89b20ac`, validated). `internal/storage` (WAL/FULL/BEGIN-IMMEDIATE/integrity-check per S02.1; `user_version` one-tx migrations; full S02.2 12-family schema + settings tables with append-only triggers), `internal/eventlog` (global `event_seq`, generation fencing vs `runs.generation`, validate-before-persist, ⚙ payload cap, same-tx `AppendTx`), `internal/settings` (all 118 S18 keys — tests pin the S18.5 tallies 118/33/per-owner; clamps; operator-owned bounds; one-tx audit row + `settings.changed`; JSON-Schema emission). First Go dependency through the rail: **modernc.org/sqlite v1.54.0** — coordinator independently confirmed `@latest` on proxy.golang.org; lock entry covers all 10 transitive modules with per-module licenses; libc pin-coupling documented. Coordinator re-ran battery: build/test(-count=1)/gofmt/vet/lockgate green; migration SQL, storage.go, eventlog.go read in full against S02.1–S02.3 (runs.state CHECK = exactly S02.3's stored states; stalled/wedged correctly derived-only); S18 tallies + `entailment_sample_rate` TBD-BRINGUP-default-0 verified against spec text. 8 clearly-implied readings logged by the packet (agent final message, repeated in CONVENTIONS §6); **driver adoption S16.4 #10 ratification queued for B0 gate** alongside B0-1's readings.
- 2026-07-19 — **P3-B0-1 done** (commit `3577f77`, validated). Scaffold per S01.5 (single multi-call binary, `cmd/sinet` + `internal/`), `components.lock` + `internal/lockfile` + `tools/lockgate`, SHA-pinned CI, `P3/CONVENTIONS.md`. Coordinator re-ran the battery independently: build/test/gofmt/vet/lockgate all green; action SHA pins independently verified against the GitHub API (both = tag v7.0.0). **Readings logged** (runbook ambiguity rule — each clearly implied by spec text): (1) lock serialization = strict pretty-printed JSON — S16.2 explicitly makes serialization "P3's choice", exercised now because B0 requires the manifest from the first dependency; dated in CONVENTIONS §4. (2) CI actions (`actions/checkout`, `actions/setup-go`) recorded as SHA-pinned `toolchain` lock entries — constituents of the ratified S01.11 CI mechanism, not running units/bundled deps; extra rigor beyond the S16.2 CI rule's scope. Formal S16.4 #10 operator ratification of both readings queued for the **B0 gate report**. (3) CI runner pinned `ubuntu-24.04` (26.04 runner image still preview — floating/preview labels never used).
