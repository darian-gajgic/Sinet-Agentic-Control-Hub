# Operator probe batch (item 3) — S19.6 B0/B1: suspend-session probe set

Three host probes, operator-run (root + one reboot + one suspend needed). Each records a result file in this directory — G3 Def.8 discipline: date, exact observation, `systemctl --version` first line. Templates at the bottom. These answers gate what B1+ may rely on (unit-corpse harvest after reboot; timer catch-up; freeze/thaw as a mechanism).

## Probe 1 — reboot-survival of unit corpses (expected per spec: corpses do NOT survive)

1. `sudo systemd-run --unit=sinet-probe-corpse --service-type=exec -p RemainAfterExit=yes -p ExitType=cgroup /bin/true`
   → expect `Running as unit: sinet-probe-corpse.service`.
2. `systemctl status sinet-probe-corpse --no-pager | head -5`
   → record: should show `active (exited)` — this is the "corpse".
3. Reboot the machine normally.
4. `systemctl status sinet-probe-corpse --no-pager | head -5`
   → record verbatim (expected: `Unit sinet-probe-corpse.service could not be found.`).
5. If anything lingers: `sudo systemctl reset-failed sinet-probe-corpse 2>/dev/null; sudo systemctl stop sinet-probe-corpse 2>/dev/null`.
6. Write `P3/measurements/<today>-reboot-survival.md` (template below).

## Probe 2 — `Persistent=` timer catch-up (across suspend AND across reboot)

1. `mkdir -p ~/.config/systemd/user`
2. Create `~/.config/systemd/user/sinet-probe-timer.service`:
   ```
   [Service]
   Type=oneshot
   ExecStart=/bin/sh -c 'date "+%%F %%T fired" >> %h/sinet-probe-timer.log'
   ```
3. Create `~/.config/systemd/user/sinet-probe-timer.timer`:
   ```
   [Timer]
   OnCalendar=*:0/15
   Persistent=yes
   [Install]
   WantedBy=timers.target
   ```
4. `systemctl --user daemon-reload && systemctl --user enable --now sinet-probe-timer.timer`
5. `systemctl --user list-timers sinet-probe-timer.timer --no-pager` → note NEXT elapse.
6. **Suspend leg:** suspend the laptop across at least one 15-min boundary, wake, then:
   `tail -3 ~/sinet-probe-timer.log && systemctl --user list-timers sinet-probe-timer.timer --no-pager`
   → record: did a missed firing run promptly on wake?
7. **Reboot leg:** power off across a boundary (or leave off >15 min), boot, log in, wait ~2 min, same check → record catch-up-after-boot.
8. Cleanup: `systemctl --user disable --now sinet-probe-timer.timer && rm ~/.config/systemd/user/sinet-probe-timer.{service,timer} && systemctl --user daemon-reload && rm ~/sinet-probe-timer.log`
9. Write `P3/measurements/<today>-persistent-catchup.md`.

## Probe 3 — user.slice freeze/thaw (⚠ your desktop will freeze for ~30 s — save work first)

1. Marker process: `systemd-run --user --unit=sinet-probe-freeze /bin/sh -c 'while true; do date "+%%F %%T tick" >> $HOME/sinet-probe-freeze.log; sleep 5; done'`
2. **Schedule the thaw BEFORE freezing** (it must run outside user.slice):
   `sudo systemd-run --on-active=30s --slice=system.slice --unit=sinet-probe-thaw systemctl thaw user.slice`
3. `sudo systemctl freeze user.slice`
   → observe: what freezes (GUI? cursor? audio?). Expect total UI freeze until the scheduled thaw fires.
4. After thaw: `tail -12 ~/sinet-probe-freeze.log` → record the tick gap (should show a ~30 s hole, then resume).
5. Record whether the session recovered fully (no crashed apps/compositor).
6. Cleanup: `systemctl --user stop sinet-probe-freeze; rm ~/sinet-probe-freeze.log; sudo systemctl reset-failed sinet-probe-thaw 2>/dev/null`
7. Write `P3/measurements/<today>-freeze-thaw.md`.

## Result-file template (one per probe)

```markdown
# <probe name> — <YYYY-MM-DD>
- systemd: <first line of `systemctl --version`>
- Steps run: as PROBES.md (deviations: none / <list>)
- Observed: <verbatim key output>
- Verdict: <one line — e.g. "corpses do not survive reboot, as spec expects">
```

Commit the result files when done (or tell the coordinator session and it will).
