# `Persistent=` timer catch-up (PROBES.md Probe 2) — 2026-07-20 — reboot leg (suspend leg deferred)

> **STATUS: reboot leg DONE (2026-07-20); suspend leg PENDING** — timer deliberately left installed. Run the suspend leg together with the battery-drain 1 h suspend measurement, then do PROBES.md step 8 cleanup.

- systemd: systemd 259 (259.5-0ubuntu3)
- Steps run: PROBES.md steps 1–5 + reboot leg (step 7, via the Probe-1 power cycle). Suspend leg (step 6) deferred — timer deliberately left installed for it. Cleanup (step 8) deferred until after the suspend leg.
- Deviation (recorded per Def.8): instead of waiting for the first natural :0/15 elapse, the first firing was seeded at 03:10:42 via a temporary drop-in (`OnActiveSec=1`, fired ~34 s after restart — default `AccuracySec=1min` coalescing), then the drop-in was removed + `daemon-reload` + timer restarted before the reboot (verified: `DropInPaths=` empty, `LastTriggerUSec=03:10:42`, stamp file mtime 03:10). Catch-up semantics are unaffected: the mechanism only compares the next calendar elapse after the recorded last trigger against the clock at (re)activation.
- Interpretation context: `loginctl show-user sinep` → **`Linger=yes`** — the user manager starts at boot without a login, so the catch-up firing is expected at ~boot time, not at first login.

## Setup (2026-07-20 03:02:58 CEST)

Unit files created verbatim per PROBES.md (`~/.config/systemd/user/sinet-probe-timer.{service,timer}`; OnCalendar=\*:0/15, Persistent=yes).

```
$ systemctl --user daemon-reload && systemctl --user enable --now sinet-probe-timer.timer
Created symlink '/home/sinep/.config/systemd/user/timers.target.wants/sinet-probe-timer.timer' → '/home/sinep/.config/systemd/user/sinet-probe-timer.timer'.

$ systemctl --user list-timers sinet-probe-timer.timer --no-pager
NEXT                          LEFT LAST PASSED UNIT                    ACTIVATES
Mon 2026-07-20 03:15:00 CEST 12min -         - sinet-probe-timer.timer sinet-probe-timer.service
```

Bonus observation (systemd 259): the Persistent stamp file was created already at `enable --now` time (03:02:58 mtime, before any firing) — systemd 259 touches the stamp on timer **activation**, not only on trigger. Implication for Sinet: even a freshly installed, never-fired persistent timer has a catch-up baseline from its first activation on this host.

Plan: last recorded trigger = 03:10:42 → next scheduled slot 03:15:00 (then :30, :45, …). Machine is shut down and stays OFF across at least one slot; on boot (user manager starts at boot — Linger=yes), exactly ONE catch-up firing is expected (no per-missed-slot pileup), log-stamped ≈ boot time. Fallback: if the off-window happened to cross NO slot (instant reboot), the leg records nothing this cycle and self-records at the next natural boot whose off-window crosses a slot — the resumed session records whichever occurred.

## Reboot leg — result (checked 2026-07-20 03:17:35 CEST)

```
$ uptime -s
2026-07-20 03:16:33

$ tail -5 ~/sinet-probe-timer.log
2026-07-20 03:10:42 fired
2026-07-20 03:16:39 fired

$ systemctl --user list-timers sinet-probe-timer.timer --no-pager
NEXT                          LEFT LAST                          PASSED UNIT                    ACTIVATES
Mon 2026-07-20 03:30:00 CEST 12min Mon 2026-07-20 03:16:39 CEST 59s ago sinet-probe-timer.timer sinet-probe-timer.service
```

- Observed (reboot leg): shutdown ~03:14 with last trigger 03:10:42 → machine off across exactly one slot (03:15:00); boot 03:16:33; **exactly one catch-up firing at 03:16:39, ~6 s after boot and before any login** (Linger=yes user manager) — then the timer resumed the normal grid (NEXT 03:30:00). Stamp file mtime advanced to 03:16.
- Rider: only one slot was missed, so single-fire-vs-pileup across MANY missed slots is not yet exercised by this leg; it self-records at any future boot after a longer off period (timer stays installed), and the ≥1 h suspend leg exercises it too.
- Observed (suspend leg): DEFERRED — run with the battery-drain suspend session (unplug + ≥1 h suspend crosses several slots).
- Verdict (reboot leg): `Persistent=` catch-up across reboot behaves exactly as S01 (R17 §4.9) assumes — a slot missed while powered off fired once, promptly (~6 s) on user-manager activation at boot, no login required, no duplicate firings.
