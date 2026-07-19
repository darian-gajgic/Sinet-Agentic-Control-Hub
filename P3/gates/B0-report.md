# B0 phase gate — report

**Status: OPEN — awaiting operator.** Written 2026-07-19 at B0 queue completion (5/5 packets done, coordinator-validated, CI green on every push). Contract: `Spec/core-architecture-v1.md` v1 (tag `spec-v1`) + amendment A1. Build record: `P3/STATE.md` log; per-packet readings in the packet commit bodies.

## 1. What shipped (the spine, S19.5 B0)

| Packet | Commit | Delivered |
|---|---|---|
| P3-B0-1 | `3577f77` | Single multi-call `sinet` binary (S01.5); adoption rail: `components.lock` + `internal/lockfile` + `tools/lockgate`; SHA-pinned CI (gofmt/vet/build/test/lock-gate); `P3/CONVENTIONS.md` |
| P3-B0-2 | `89b20ac` | `platform.db` (WAL, `synchronous=FULL`, `BEGIN IMMEDIATE`, integrity-check-at-open, `user_version` migrations; full S02.2 12-family schema); append-only event log (`event_seq`, generation fencing); settings registry — all 118 S18 keys, clamps, audit events, JSON-Schema emission |
| P3-B0-3 | `4bf3415` | `sinet control` real in dev mode: S01.6 startup ladder, fail-closed loopback lint (P-T13-2), SIGTERM clean shutdown, maintenance switch, watchdog/sd_notify; `/api/health` + the one `/events` SSE stream; five seams as packages; `sinet units` — S01.2 unit set **generated, never installed** |
| P3-B0-4 | `8a25ca6` | S02 core: run FSM (every edge spec-anchored), checkpoint-per-paid-call, two-phase effect journal (classes A–D, executing-before-provider-call, saved-plan re-approval), S02.5 recovery ladder live in startup, suspend-aware leases, fork-with-generation-bump + dispatch-id CAS; **kill-9 crash harness** in the test suite |
| P3-B0-5 | `7fc639e` | S01.9 auth stack: argon2id PINs, server-side sessions, device grants (shared-device default, G3 Def.1), verify-at-act step-up, auth events on the log; `/api/auth/*`; cert runbook `P3/gates/B0-cert-steps.md` |

## 2. Evidence

- **Tests:** `go test -count=1 ./...` — 13 packages green; `-race` green on every non-trivial package; coordinator re-ran the full battery independently after each packet. Kill-9 harness SIGKILLs re-exec'd children mid-write: integrity ok, interrupted tx invisible, double-resume fenced inert, effect crash-windows resolved per class.
- **CI:** green on GitHub for every push (runs 29695989125 → latest); lock-gate enforces manifest coverage of all 11 Go modules + both CI actions.
- **Live demos, coordinator-re-run:** health JSON; SSE replay from cursor (`platform.started`, `id: 1`); SIGTERM → ordered shutdown, exit 0; `0.0.0.0` bind refused fail-closed; recovery-ladder pass on a pre-existing state dir (migrations 0001→0003 apply cleanly in sequence); auth flows live-exercised by the packet agent (bootstrap → login → step-up → SSE auth events).
- **Adoption rail:** 5 lock entries — Go toolchain 1.26.5; actions/checkout + actions/setup-go (SHA = v7.0.0, coordinator-verified vs GitHub API); modernc.org/sqlite v1.54.0; golang.org/x/crypto v0.54.0 (both coordinator-confirmed `@latest` on proxy.golang.org at adoption).

## 3. Demo — run it yourself (literal)

```
cd ~/Sinet-Agentic-Control-Hub
go build -o /tmp/sinet ./cmd/sinet
/tmp/sinet control --state-dir /tmp/sinet-demo --http-addr 127.0.0.1:8482   # terminal 1
curl -s http://127.0.0.1:8482/api/health                                    # terminal 2 → ready:true
curl -N 'http://127.0.0.1:8482/events?after_seq=0'                          # streams platform.started
# Ctrl-C terminal 1 → ordered shutdown log, exit 0
/tmp/sinet units --out /tmp/sinet-units && ls /tmp/sinet-units              # 6 unit files, host untouched
```

## 4. Decisions the operator owns at this gate

1. **Ratify the rail adoptions (S16.4 #10):** modernc.org/sqlite v1.54.0 (the S16.3 row's pin) and golang.org/x/crypto v0.54.0 (argon2id — named in S01.9), plus the B0-1 posture readings (lock = strict JSON per S16.2's "P3's choice"; CI actions as SHA-pinned lock entries; `ubuntu-24.04` runner pin).
2. **Auth numbers:** S01.9 ⚙-flags nothing and S18 ratifies no auth keys — session TTL 30 d, 5 PIN attempts, 15 min lockout ship as documented constants. Keep as constants (spec-conformant), or direct an S00.9 amendment adding S18 rows to make them operator-tunable (re-runs the S18 sweep per the standing rule).
3. **Host-side installs (proposal, not yet executed):** install the generated systemd units + journald drop-in; run `P3/gates/B0-cert-steps.md` (13 steps: first cert on `sinet.tailfd0b1e.ts.net` — **publishes the name to CT logs permanently** — plus `tailscale serve` → Caddy chain; Caddy lock entry materializes there per S16.3). Approve now or defer to B1/B2 (dev mode continues working either way; the B2 walking-skeleton gate is the natural latest point).
4. **logind sleep-inhibitor wiring (P-T07-1):** the pre-sleep flush path exists (shutdown-parity) but the PrepareForSleep D-Bus inhibitor is not wired. Options: adopt a D-Bus module through the rail, or implement the wire protocol manually — pick at this gate or defer to B1 (the recovery ladder already treats suspend as crash-equivalent, so the gap costs re-spend bounds, not correctness).
5. **Packet readings en bloc:** 40+ clearly-implied readings logged across the five commit bodies (each section-cited; headline ones in the STATE log). Ratify en bloc or name any to re-open.

## 5. Operator hands-on items due at this gate

- **Item 3 (probe batch):** the S19.6 B0/B1 measurements — reboot-survival of unit corpses, `Persistent=` timer catch-up, user.slice freeze/thaw, and the suspend-session probe. Results go to `P3/measurements/` (G3 Def.8 discipline). Needed before B1 relies on the answers; the suspend probe pairs naturally with your existing battery-drain thread.
- **Item 7 (observables register):** first-cert CT publication is register rows 1+3 — sign-off due no later than first deploy; the cert runbook flags it at the exact step.

## 6. Known-open, tracked (no action needed to close the gate)

- Maintenance-mode HTTP mutation surface: unblocked by auth, lands with B1's surfaces.
- Serialize-by-deny reconfirm on the default worker model — `TBD-P3` rides S02.8, due with B1/B2 lane work.
- Provisional lifecycle event names (`platform.*`, `auth.*`) pending the S14 event contract at B5.
- `P3/measurements/` empty until item-3 probes run.

**To close:** answer §4 (free text is fine); the coordinator records answers here + STATE, then opens B1.
