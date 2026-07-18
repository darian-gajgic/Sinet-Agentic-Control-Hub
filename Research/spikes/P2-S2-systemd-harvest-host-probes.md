# Spike P2-S2 — systemd harvest matrix + host sandbox prerequisites

**Run:** 2026-07-18. **Run twice independently** (a delegated background agent + a coordinator-inline pass after a task-tracking glitch hid the delegated launch); both agreed on every core fact, the delegated run went deeper. Where they differed the delegated run's live measurement wins — noted inline. All probes read-only or user-scope transient units with verified cleanup.
**Scope:** R08-OQ8 (systemd harvest matrix) + read-only parts of R10-OQ3 (host sandbox prerequisites). **Discharges the operator's "report-10 §7.3 host-probe afternoon" for everything checkable without root/reboot/suspend** (the D2.3 sandbox-ratification contingency).
**Cost:** $0.00 (local only; no inference, no accounts, no network beyond none). **Hard rules honored:** read-only inspection + one userspace C compile (read-only Landlock ABI query, enforces nothing) + user-scope `systemd-run --user` transients (prefix `p2s2-`/`sinet-probe`, all removed, verified no residue). **No kernel modules loaded, no `sudo`, no `apt`, no reboot, no suspend, no boot/driver/firmware change; every `journalctl` bounded with `-n`.**
**Status:** PASS.

## Host baseline (verbatim)

| Fact | Value |
|---|---|
| Kernel | `7.0.0-27-generic` x86_64 |
| OS | `Ubuntu 26.04 LTS (Resolute Raccoon)` (`VERSION_ID="26.04"`) |
| systemd | `systemd 259 (259.5-0ubuntu3)` — same major the R08 verifier live-tested |
| Active LSMs | `lockdown,capability,landlock,yama,apparmor,ima,evm` |
| User | single user `sinep` (uid 1000), **`Linger=yes`**, session `active`; user manager `degraded` from two pre-existing failed units (`orca.service`, `update-notifier-crash.service` — unrelated, left as found) |

## R10-OQ3 — sandbox prerequisites (the D2.3-blocking trio, all discharged)

### 1. Unprivileged userns under Ubuntu's AppArmor restriction — **AVAILABLE, profile proven working**
- `kernel.apparmor_restrict_unprivileged_userns = 1` (enforced) and `kernel.apparmor_restrict_unprivileged_unconfined = 1`. Plain `unshare -U -r` is **blocked** (EPERM on uid_map) — the restriction is live, not nominal.
- **bubblewrap 0.11.1** at `/usr/bin/bwrap`; **`/etc/apparmor.d/bwrap-userns-restrict` ships and functions**: `bwrap --unshare-user --uid 0` inside a transient unit printed uid `0`, and children ran under the stacked label **`bwrap//&unpriv_bwrap (enforce)`** — userns granted to bwrap, capabilities stripped from children (the profile's stated design). Also present: `unprivileged_userns`, `lxc-usernsexec`.
- **Verdict:** the report-10 stack (bwrap + seccomp + Landlock; Anthropic's `srt` wraps system bwrap) runs on this userns-restricted host with **no fallback needed** — the "per-binary AppArmor profile" fix report 10 named is shipped and functionally proven. **Rider for the spec:** the profile attaches to `/usr/bin/bwrap`; Sinet must invoke the *system* bwrap (not a vendored/renamed copy) or ship its own `userns`-granting profile.

### 2. Landlock ABI — **version 8**
- Compiled syscall probe returned **`Landlock ABI version: 8`**. TSYNC-level multithreaded enforcement present (meets report 10 OQ3's ABI-8 bar). **ABI 10 UDP scoping absent** (kernel is at 8) → report 10's **nftables egress fallback stands** (nftables is the real network boundary anyway; Landlock network control is TCP-bind/connect only at this level).

### 3. Filesystem / reflink CoW — **single ext4 root, no reflink; XFS/btrfs need repartition or loopback**
- Everything (`/`, `/home`, the repo) is one **~420 GB ext4** root on `/dev/nvme0n1p5`. No separate `/home`, no LVM, no unallocated space. The rest of the NVMe is EFI + **two Windows NTFS partitions**; swap is a swapfile.
- **ext4 has no reflink** → report 10's reflink-CoW per-run workspaces are unavailable as-is. The `xfs.ko`/`btrfs.ko` modules ship with this kernel (so the capability exists), but **there is no free space/partition to host an XFS/btrfs volume without repartitioning** (the Windows partition is the only large reclaimable space) **or a loopback-image mount**. bcachefs absent.
- **Operator/G-decision (implementation phase):** provision a reflink-capable project-store volume (repartition or loopback) **or** design the workspace clone around copy / hardlink-tree / overlayfs. *This spike neither repartitions nor loads modules.*
- **Live constraint:** root is **91% full (~39 GB free)** — a workspace/snapshot-heavy design needs headroom regardless.

## R08-OQ8 — systemd harvest matrix (measured on-host, user scope)

| Cell | Measured result |
|---|---|
| **Success corpse** (`RemainAfterExit=yes`, exit 0) | Persists as `active (exited)`: `Result=success`, `ExecMainCode=1`(CLD_EXITED), `ExecMainStatus=0`, `MainPID=0`, InvocationID retained. **`CPUUsageNSec` + `MemoryPeak` survive into the corpse** (free post-exit accounting); `MemoryCurrent=[not set]` |
| **Failure corpse** (`RemainAfterExit=yes`, exit 42) | Lands `failed` (RemainAfterExit does not keep failures "active"); `Result=exit-code`, `ExecMainStatus=42`; CPU/mem-peak retained |
| **Failure, no `RemainAfterExit`** (exit 7) | **Still lingers** (`CollectMode=inactive` default): `failed`, `ExecMainStatus=7` readable until `reset-failed` |
| **Success, no `RemainAfterExit`** | **Gone within seconds** (`not-found`). The asymmetry: failures linger by default; successes must opt in with `RemainAfterExit=yes` |
| **`--collect`** (CollectMode=inactive-or-failed), exit 9 | GC'd even on failure → **never use `--collect` on a lane whose corpse you want to mine** |
| **`daemon-reload`** | Everything survives — running units, active-exited corpses, failed corpses; properties byte-identical (InvocationID, ExecMainStatus, CPUUsageNSec, MemoryPeak); transient files still on disk. Confirms R08 [S35] on-host |
| **`daemon-reexec`** (crash-adjacent) | **Everything survives manager re-execution too** — corpses intact, a running `sleep` kept its MainPID. Package-upgrade-grade restarts don't lose harvest state |
| **Transient unit file** | `/run/user/1000/systemd/transient/<unit>.service`, world-readable, serialized properties, header "created programmatically via the systemd API. Do not edit." GC deletes it with the unit |
| **`ExitType=cgroup`** | Works at user scope: after the main process exits, the unit stays `active (running)` on a **detached child alone**; when the cgroup empties it moves to the normal corpse. `ExecMainStatus` is harvestable mid-flight |
| **`ExitType=cgroup` + bwrap `--unshare-pid` tree** | The tree **outlived bwrap's own main-process exit**; the unit correctly stayed `running` for the full child lifetime, ending in a success corpse → **lanes need `ExitType=cgroup`**: the sandbox binary exiting ≠ the tree being dead |
| **`ExitType=cgroup` + `Type=oneshot`** | **Rejected**: `"bad unit file setting"`. Run-wrapper units needing cgroup exit-tracking must be `Type=exec`/`simple`/`notify`, never `oneshot` |
| **`Type=oneshot` + `Restart=on-failure`** | Accepted and runs (R08 [S36] confirmed on 259) |
| **`Type=oneshot` + `Restart=always`** | Refused (`"bad unit file setting"`), **no residue** (immediately `not-found`) |
| **Journal mining by invocation** | Per-unit `-u` works. By invocation id at **user scope you must OR two fields**: `_SYSTEMD_INVOCATION_ID=X` (process lines, trusted) **and** `USER_INVOCATION_ID=X` (manager "Started/Stopped" lines); plain `INVOCATION_ID=` matches nothing. A silent unit is findable only via `USER_INVOCATION_ID` |
| **Journald persistence** | `/var/log/journal` exists → persistent journal. **Per-run journal records survive reboot; unit corpses do not** → enrichment for P-T07-2; the wrapper-written exit record stays mandatory |
| **Resource-control delegation** | User-scope transients land in `app.slice` under `user@1000.service`; delegated controllers = **cpu, memory, pids — no `io`** → if IO limits are wanted, that's a point for system-scope lanes |
| **Cleanup** | `stop 'p2s2-*'` + `reset-failed 'p2s2-*'` (globs work) release everything; transient files deleted. Verified: zero `p2s2` residue, failed-set back to the two pre-existing units |
| **Reboot behavior** | **Not probed (forbidden).** Documented answer stands: transients released on reboot (P-T07-2 unchanged) |

**Lane recipe (feeds R17 per-run transient lanes):** `RemainAfterExit=yes` + `ExitType=cgroup` + `Type=exec` + **never `--collect`**; the run wrapper mines `ExecMainStatus`/`Result`/`CPUUsageNSec`/`MemoryPeak` from the corpse and the journal via the two-field invocation query.

## Sleep / inhibitor facts

- **`InhibitDelayMaxSec` = 30 s on this host, not the 5 s upstream default** — set by **unattended-upgrades' drop-in** `/usr/lib/systemd/logind.conf.d/unattended-upgrades-logind-maxdelay.conf`; `/etc/systemd/logind.conf` still shows the commented `#InhibitDelayMaxSec=5`. Live: `InhibitDelayMaxUSec=30000000` (property is `const` — changes need a logind restart). **Design posture:** size the O(1) pre-sleep flush to the **5 s portable floor**; the 30 s is another package's drop-in, not a Sinet-owned guarantee.
- **`PrepareForSleep` signal + `PreparingForSleep` property confirmed present** on `org.freedesktop.login1.Manager`; `DelayInhibited="shutdown:sleep"`; 11 live inhibitors (GNOME shell etc.), `InhibitorsMax=8192`; `SleepOperation = suspend-then-hibernate, suspend, hibernate`. → the R08 §4.4 logind delay-inhibitor + `PrepareForSleep` mechanism is wired exactly as designed.
- **user.slice is NOT frozen during sleep on this host** — all four sleep units carry an **NVIDIA-shipped drop-in** (`/usr/lib/systemd/system/systemd-suspend.service.d/nvidia-suspend-nofreeze.conf` etc.) setting `SYSTEMD_SLEEP_FREEZE_USER_SESSIONS=false` to avoid an Xorg VT-switch deadlock ("For now, disable freeze on suspend"). Upstream default *is* to freeze. **This corrects the inline pass's initial reading** (which assumed the freeze applies): on this exact host it's vendor-disabled — but it's a **fragile vendor workaround a driver update can remove**, so it must not become a design foundation. Net: R17's system-unit choice is sound but is **not load-bearing on freeze avoidance today**.

## User-vs-system service — evidence for R17's unit set
R17's system-unit recommendation for the always-on control plane **stands**, now with host ground: linger is already on (user manager persists across logout, `KillUserProcesses=no`), user scope lacks `io`-controller delegation, user transients start AppArmor-`unconfined`, and the freeze argument is currently moot-but-fragile. Everything measured here (corpse semantics, reload/reexec survival, ExitType) is manager-generic → a five-minute re-verify at system scope on implementation day. Per-user engine units may reasonably be **user** units (linger on); the control plane stays **system** units. Final split → spec process-seam section.

## Deferred (need root / reboot / a real suspend — operator-assisted, implementation phase)
1. **System-unit reboot survival + `RemainAfterExit` + `daemon-reload`** (root + reboot).
2. **Real user.slice freeze/thaw timing across suspend→resume** — **batch with R09-OQ7** (`Persistent=` catch-up test) into one operator-assisted suspend session: unplug → `systemctl suspend` → resume after > one timer interval → observe timer catch-up + (given the NVIDIA drop-in) confirm sessions actually stay thawed.
3. **`ExitType=cgroup` against the real bwrap+engine+children run-wrapper** (vs synthetic `sleep`).
4. **System-scope `systemd-run` transient variants** (root). The user-scope result establishes the mechanism.

## Contradiction / correction records (explicit)
- **Report 08 P-T07-1 ("pre-sleep budget ~5 s"):** measured **30 s** effective (unattended-upgrades drop-in). A measured refinement, not a D-constraint conflict. → Reword P-T07-1 in the spec Known-problems list: *"the pre-sleep inhibitor budget is short and host-configurable (5 s systemd default; 30 s on the v0 host via unattended-upgrades) — never size a checkpoint to it; pre-sleep is an O(1) durable flush, all reconcile is wake-side."*
- **Report 08 freeze note:** on this host user-session freeze-on-suspend is **vendor-disabled** (NVIDIA drop-in). Record so the wake-side reconcile design treats freeze as *possible-but-currently-off-and-fragile*, not guaranteed.

## Implications for the spec
- **Sandbox section (← report 10):** all three prerequisites clear; add the system-bwrap-only / BYO-AppArmor-profile rider; nftables remains the egress boundary (Landlock ABI 8 < UDP-scoping).
- **Workspace strategy (← reports 10/13):** reflink not free on ext4 and no free space for an XFS/btrfs partition → decide repartition-vs-loopback-vs-non-reflink in the storage-seam section; mind the 91%-full disk.
- **Process seam (← report 17):** control plane = system units (confirmed sound, freeze-independent); per-user engine units may be user units; per-run lane recipe above.
- **Run-lifecycle / sleep-wake (← report 08):** P-T07-1 reworded; freeze treated as fragile-vendor-disabled; 5 s portable floor for the pre-sleep flush.
