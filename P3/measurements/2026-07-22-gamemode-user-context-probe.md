# Probe — GameMode per-user D-Bus signal reach to a system-scope subscriber (TBD-P3, S12.2 / R16 §7-OQ7)

- **Packet:** P3-B4-5 (S12 local serving stack), 2026-07-22. The spec's `TBD-P3(GameMode user-context probe)` executed live before wiring R13(b); the verdict BINDS the implementation (brief §7 item 9; G3 Def.8 discipline: question, method, raw observation, verdict, consequence).
- **Question:** GameMode's `GameRegistered`/`GameUnregistered` D-Bus signals are the auto-pause/resume input the control plane wants to subscribe to (S12.2). The control plane runs as the **`sinet` system user** (S01.2 `sinet-control.service`, system scope). GameMode (`gamemoded`) is a **per-user** daemon. Do its signals reach a **system-scope** subscriber without host/session changes? If not, the busctl-subprocess subscription (R13b) must run where it can, and the `gamemode.ini` scripts leg (R13a) is the always-works layer.
- **Method (user-level only, no host changes):** on this host `gamemoded` v1.8.1 and `busctl` are present; the operator session is uid 1000 (`sinep`), session bus at `unix:path=/run/user/1000/bus`. (1) enumerate the bus a name is owned on: `busctl --user list` vs `busctl --system list`, grep `gamemode`. (2) capture signals live: `busctl --user monitor com.feralinteractive.GameMode` in the background while triggering a real register/unregister cycle with `gamemoderun sleep 1` (user-level; sets `LD_PRELOAD` + calls `RegisterGameByPIDFd` on start, unregisters on exit). (3) attempt the same from **system scope**: `busctl --system monitor com.feralinteractive.GameMode`. No sysctl written, no unit touched, no host package changed.

## Raw observations (live, 2026-07-22)

- **Name ownership — SESSION only.** `busctl --user list` → `com.feralinteractive.GameMode … (activatable)`. `busctl --system list` → **not present** (grep empty). GameMode owns its name on the per-user session bus, not the system bus.
- **Signals fire on the session bus (captured).** With `busctl --user monitor com.feralinteractive.GameMode` running, `gamemoderun sleep 1` (exit 0) produced, in order:
  - `Member=RegisterGameByPIDFd` (method call → `com.feralinteractive.GameMode`)
  - `Member=GameRegistered` (**signal** from `:1.318`, path `/com/feralinteractive/GameMode`, interface `com.feralinteractive.GameMode`)
  - `Member=PropertiesChanged`
  - `Member=UnregisterGameByPIDFd` (method call) → `Member=GameUnregistered` (**signal**) → `PropertiesChanged`
  - 108 lines total captured; both `GameRegistered` and `GameUnregistered` observed on the session bus.
- **System-scope observation FAILS.** `busctl --system monitor com.feralinteractive.GameMode` → `Call to org.freedesktop.DBus.Monitoring.BecomeMonitor failed: Access denied`. gamemoded is absent from the system bus regardless, so even with monitor rights there is nothing there to observe.

## Verdict

**NOT reachable from system scope without host/session config.** GameMode's `GameRegistered`/`GameUnregistered` are emitted on the **per-user session bus** (`/run/user/<uid>/bus`, owned by the operator uid). A subscriber running as the `sinet` **system** user has no session bus of its own and cannot reach the operator's session bus without that session's `DBUS_SESSION_BUS_ADDRESS` — a per-login, operator-owned value that is not a system-scope fact and cannot be obtained without a host/session change (e.g. a user-bus proxy, an `--user` unit in the operator's session, or exporting the address into the control plane). This is the well-known gamemoded design (session-scoped), now confirmed live on this host at v1.8.1.

## Consequence (BINDS R13b)

1. **The `gamemode.ini` `[custom]` scripts leg (R13a) is the always-works layer and carries the duty.** Those scripts run **inside the operator's game session** and call the two eager-unload/resume verbs directly (loopback platform API + the llama-swap unload leg) — bus-scope-independent, so pause/resume works regardless of this finding. This is the primary mechanism.
2. **The busctl-subprocess subscription (R13b) is built but runs where it can.** It is a supervised `busctl … monitor com.feralinteractive.GameMode` subprocess (zero new Go deps, §8 reading 8) that MUST be pointed at the operator's session bus. The control plane cannot supply that from system scope, so at v0 the subscription is **deferred-with-finding**: it activates only when the operator supplies `DBUS_SESSION_BUS_ADDRESS` (bring-up config — the `SINET_LOCAL_*` structural-config precedent) or runs the monitor leg in their own user session. Absent that, the subscription is inert and the scripts leg alone carries pause/resume. Either outcome is spec-satisfying (brief R14); an unexecuted probe would not be.
3. **Flagged to the B4 gate / hardening (brief §6 item g):** the operator decides whether to wire the session-bus address into the control plane (or run a user-scope monitor) to enable the D-Bus subscription's daemon-restart coverage; the scripts leg needs no such decision.

## Spend

$0 — user-level `gamemoderun`/`busctl` only, no engine calls, no host changes. Raw monitor captures retained in the session scratchpad; this file is the durable record.
