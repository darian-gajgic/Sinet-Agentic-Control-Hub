# P3 implementation — live state

> Any session advancing the build: read this file, then follow `.claude/skills/p3-implementation/SKILL.md`. Update this file before and after every step — it is the single source of truth for build coordination. The contract is `Spec/core-architecture-v1.md` (v1, frozen, tag `spec-v1`); the campaign record in `Research/` is a closed archive.

**Phase status:** **B0 (spine) OPEN — P3-B0-1 done (validated); P3-B0-2 is next.** Scaffold, adoption rail, and `P3/CONVENTIONS.md` exist; conventions in that file bind all later packets.

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
| P3-B0-2 | platform.db + settings registry + event log | S01.9–S01.10, S02 (schema/tables), S18 (key index) | DB bootstrap with ratified pragmas + migrations; declare-once registry (clamps, audit events, schema emission); append-only event log with `event_seq`; unit tests | pending |
| P3-B0-3 | sinet-control shell + API/SSE skeleton + five seams | S01 full | Dev-mode process serves health + the one SSE endpoint; watchdog wiring; seams as package boundaries; systemd unit files *generated, not installed* (host changes wait for the gate) | pending |
| P3-B0-4 | Run FSM + checkpoint/effect journal + recovery ladder | S02 full | FSM with derived-not-stored stalled state; checkpoint rows; two-phase effect journal; recovery ladder + leases/generation fencing; **kill-9 harness v1** in the test suite | pending |
| P3-B0-5 | Auth stack (tailnet wall → header hint → sessions + PIN) | S01 (auth), S15.2 (API posture) | Session + PIN machinery with tests; identity-header parsing in dev mode; cert/hostname steps documented for the B0 gate (needs operator item 2) | pending |

**B0 gate (when queue done):** phase report + demo; propose host-side installs (systemd units) for approval; operator items due: **ts.net hostname pick** (before first cert) and the **root/reboot/suspend probe batch** (S19.6 B0/B1 measurements — suspend-session probe records to `P3/measurements/`).

## Operator hands-on items (carried from G4)

| # | Item | Due at |
|---|---|---|
| 1 | Z.AI dashboard prompt-unit calibration (5-step recipe in P2-S1 report §Blocked) | B1 (metering trust) |
| 2 | ts.net hostname pick — bland + permanent | **B0 gate, before first cert** |
| 3 | Root/reboot/suspend probe batch (reboot-survival; `Persistent=` catch-up; user.slice freeze/thaw) | B0/B1 gate |
| 4 | Week-one push drill on household phones | first deploy |
| 5 | (Optional) GitHub Verified badge — signing-key upload | anytime |

## Log

- 2026-07-19 — P3 machinery created (skill + this STATE + CLAUDE.md pointer) in the G4-closing session; B0 opened, queue cut from S19.5 + S01/S02/S16 scopes. First build packet deliberately left for a fresh session.
- 2026-07-19 — **Host toolchain:** operator reported Go 1.26 installed, but no Go existed anywhere on the host (PATH, /usr/local, snap, dpkg, version managers all checked). Coordinator installed **Go 1.26.5** user-level: sha256-verified official tarball → `~/.local/go`, wrapper scripts `~/.local/bin/{go,gofmt}` (already on PATH). No root, no host-level change; reversible by deleting those paths. P3-B0-1 launched.
- 2026-07-19 — **P3-B0-1 done** (commit `3577f77`, validated). Scaffold per S01.5 (single multi-call binary, `cmd/sinet` + `internal/`), `components.lock` + `internal/lockfile` + `tools/lockgate`, SHA-pinned CI, `P3/CONVENTIONS.md`. Coordinator re-ran the battery independently: build/test/gofmt/vet/lockgate all green; action SHA pins independently verified against the GitHub API (both = tag v7.0.0). **Readings logged** (runbook ambiguity rule — each clearly implied by spec text): (1) lock serialization = strict pretty-printed JSON — S16.2 explicitly makes serialization "P3's choice", exercised now because B0 requires the manifest from the first dependency; dated in CONVENTIONS §4. (2) CI actions (`actions/checkout`, `actions/setup-go`) recorded as SHA-pinned `toolchain` lock entries — constituents of the ratified S01.11 CI mechanism, not running units/bundled deps; extra rigor beyond the S16.2 CI rule's scope. Formal S16.4 #10 operator ratification of both readings queued for the **B0 gate report**. (3) CI runner pinned `ubuntu-24.04` (26.04 runner image still preview — floating/preview labels never used).
