# Reboot-survival of unit corpses (PROBES.md Probe 1) — 2026-07-20

- systemd: systemd 259 (259.5-0ubuntu3)
- Steps run: as PROBES.md (deviation: the "reboot" was performed as a full shutdown → cold boot rather than a warm `reboot`, so the same power cycle also crosses a quarter-hour boundary for the Probe-2 reboot leg; equivalent for corpse survival — systemd runtime state is lost either way)

## Pre-reboot (2026-07-20, ~03:0x CEST)

- Corpse creation at 03:06:49 CEST (`sudo systemd-run --unit=sinet-probe-corpse --service-type=exec -p RemainAfterExit=yes -p ExitType=cgroup /bin/true`):

  ```
  Running as unit: sinet-probe-corpse.service; invocation ID: d5fddf727d33419da075a9f97837bed1
  ```

- `systemctl status sinet-probe-corpse --no-pager | head -5` (the corpse, `active (exited)` as expected; note the transient fragment lives on tmpfs `/run`):

  ```
  ● sinet-probe-corpse.service - [systemd-run] /bin/true
       Loaded: loaded (/run/systemd/transient/sinet-probe-corpse.service; transient)
    Transient: yes
       Active: active (exited) since Mon 2026-07-20 03:06:49 CEST; 4ms ago
   Invocation: d5fddf727d33419da075a9f97837bed1
  ```

## Post-reboot (checked 2026-07-20 03:17:35 CEST; boot at 03:16:33)

- `systemctl status sinet-probe-corpse --no-pager | head -5`:

  ```
  Unit sinet-probe-corpse.service could not be found.
  ```

- Nothing lingered — PROBES.md step 5 cleanup not needed.

- Observed: corpse was `active (exited)` at 03:06:49 pre-shutdown; after the power cycle (shutdown ~03:14, boot 03:16:33) the unit is gone entirely — `Unit sinet-probe-corpse.service could not be found.` Consistent with the transient fragment living on tmpfs (`/run/systemd/transient/`).
- Verdict: corpses do NOT survive reboot, as spec expects (P-T07-2 [SPIKE P2-S2]) — the run wrapper's own exit-record append remains the mandatory durable evidence; corpse harvest is enrichment only and is unavailable across a reboot.
