// Package units renders the systemd unit set of Spec S01.2 — the process
// seam of Spec S01.3 made concrete: every organ its own unit, its own
// journal identity, its own restart policy.
//
// Units are GENERATED, NEVER INSTALLED by this code: output goes to stdout
// or an operator-chosen directory, and host changes (installing under
// /etc/systemd, systemctl calls) are a B0-gate operator decision
// (P3/STATE.md). Adopted organs that ship their OWN unit file (caddy,
// tailscaled) are deliberately not generated here — replacing one is a unit +
// components.lock edit at the process/adoption seams (Spec S01.3).
//
// The carve-out (established at P3-B4-5 §8 reading 1, extended at P3-B5-6A): an
// adopted organ that ships NO unit file of its own gets a Sinet-generated one,
// because that unit is SINET configuration for an unmodified adopted binary
// (S16.1 config-only integration — not a fork). Two units are generated under
// it, both `Draft: true`, and each ExecStart runs the OPERATOR-INSTALLED
// third-party binary, never the sinet multi-call binary:
//
//   - `sinet-llamaswap.service` — llama-swap ships no unit file (S12.2).
//   - `sinet-watchlist.service` — changedetection.io ships no unit file: a
//     recursive tree read of tag 0.55.8 matched no *.service and no systemd/
//     path, only Dockerfile + docker-compose.yml (verified 2026-07-26). The
//     B0-era package doc listed "the watchlist executor" among organs shipping
//     their own units; that was an unchecked assumption and is corrected here
//     (Spec S14.6 "own unit [S01.2]").
//
// Generation NEVER installs: the host install of either organ is a phase-gate
// operator act.
//
// ⚙ values are rendered from the settings registry (shell.watchdog_sec,
// shell.journal_max_use). At B0 generation reads the declared defaults —
// override-aware regeneration needs the operator surfaces of later packets;
// regenerate + reinstall is the update path either way (restart-required
// posture, Spec S01.10).
package units

import (
	"fmt"
	"net"
	"strings"
	"time"
)

// A1 hostname commitment (Spec S01.8, amendment A1, operator 2026-07-19):
// the ts.net machine name is `sinet`. It is recorded here so every
// generated artifact that ever needs the hostname draws it from one place.
const (
	Hostname = "sinet"
	FQDN     = "sinet.tailfd0b1e.ts.net"
)

// DefaultBinaryPath is where the deploy step installs the one release
// artifact (Spec S01.5, S01.11) and therefore the ExecStart= prefix of
// every owned unit. Mode names are stable from B0-1, so these lines never
// change shape.
const DefaultBinaryPath = "/usr/local/bin/sinet"

// Settings is the units-facing view of the settings registry (Spec S01.10).
type Settings interface {
	Int(key string) (int64, error)
	Duration(key string) (time.Duration, error)
}

// Dotted ⚙ keys rendered into the unit set (owned by Spec S01, plus the S13.10
// backup cadences the persistent calendar timers derive from).
const (
	keyWatchdogSec     = "shell.watchdog_sec"
	keyJournalMaxUse   = "shell.journal_max_use"
	keyBackupInterval  = "backup.interval"       // seconds (snapshot cadence)
	keyBackupDrillEach = "backup.drill_interval" // months (restore-drill cadence)
)

// File is one generated configuration file.
type File struct {
	// Name is the file name (unit name, or a drop-in name whose intended
	// target path is stated in its header comment).
	Name string
	// Content is the complete file text.
	Content string
	// Draft marks a unit whose ExecStart (and therefore installability)
	// belongs to a later phase: generated for shape stability, not
	// installable yet.
	Draft bool
}

// Params configures generation.
type Params struct {
	// BinaryPath overrides DefaultBinaryPath.
	BinaryPath string
	// LlamaSwapBinary / LlamaSwapConfig / LlamaSwapListen are the STRUCTURAL
	// config for the generated sinet-llamaswap.service (Spec S12.2; NOT ⚙,
	// §8 reading 7). The binary is the operator/user-installed llama-swap
	// (adopted organ, NOT the sinet binary); the config is the generated
	// llama-swap YAML; the listen address is loopback (S01.1). Defaults below.
	LlamaSwapBinary string
	LlamaSwapConfig string
	LlamaSwapListen string
	// WatchlistBinary / WatchlistDatastore / WatchlistListen are the STRUCTURAL
	// config for the generated sinet-watchlist.service (Spec S14.6; NOT ⚙ —
	// S18 ratifies no key, the sseBatchSize precedent). The binary is the
	// operator-installed changedetection.io (adopted organ, NOT the sinet
	// binary); the datastore is its state directory; the listen address is
	// loopback (S01.1). Defaults below.
	WatchlistBinary    string
	WatchlistDatastore string
	WatchlistListen    string
}

// Default structural paths for the generated llama-swap unit (operator-set at
// install; the composition-root passthrough — the SINET_SRT_PATH precedent).
const (
	defaultLlamaSwapBinary = "/usr/local/bin/llama-swap"
	defaultLlamaSwapConfig = "/etc/sinet/llamaswap.yaml"
	defaultLlamaSwapListen = "127.0.0.1:8791"
	// LocalSlice is the systemd slice the local-inference tier sits in (S12.2:
	// the designated systemd-oomd victim; slice NAME [coordinator-draft]).
	LocalSlice = "sinet-local.slice"
)

// Default structural paths for the generated watchlist-executor unit
// (operator-set at install; composition-root passthrough). The listen address
// is loopback because changedetection.io's OWN default is 0.0.0.0 (its `-h`
// flag / LISTEN_HOST env, verified at tag 0.55.8) — leaving that default would
// bind the organ beyond loopback, which S01.1 forbids, so `-h` is load-bearing
// here and not cosmetic.
const (
	defaultWatchlistBinary    = "/usr/local/bin/changedetection.io"
	defaultWatchlistDatastore = "/var/lib/sinet/watchlist"
	defaultWatchlistListen    = "127.0.0.1:5000"
)

// header is the shared provenance banner.
func header() string {
	return fmt.Sprintf(`# Generated by 'sinet units' — Spec S01.2 unit set. Do not hand-edit;
# regenerate instead. Nothing installs this file: host changes are a
# B0-gate operator decision. Host identity (amendment A1): machine %q,
# tailnet FQDN %q.
`, Hostname, FQDN)
}

// hardening is the standard hardening set applied to every owned unit as
// far as compatible with its duty (Spec S01.2); a per-unit exception is
// recorded in the unit file with a reason.
const hardening = `# Standard hardening set (Spec S01.2).
ProtectSystem=strict
NoNewPrivileges=yes
PrivateTmp=yes
SystemCallFilter=@system-service
`

// staticUser pins every platform unit to the one static sinet user —
// DynamicUser= is NEVER used for state-owning units (UID recycling vs
// decade-lived DB files, Spec S01.1).
const staticUser = `User=sinet
Group=sinet
`

// Files renders the S01.2 unit set in deterministic order.
func Files(settings Settings, p Params) ([]File, error) {
	bin := p.BinaryPath
	if bin == "" {
		bin = DefaultBinaryPath
	}
	watchdogSec, err := settings.Int(keyWatchdogSec)
	if err != nil {
		return nil, fmt.Errorf("units: read ⚙ %s: %w", keyWatchdogSec, err)
	}
	journalMaxUse, err := settings.Int(keyJournalMaxUse)
	if err != nil {
		return nil, fmt.Errorf("units: read ⚙ %s: %w", keyJournalMaxUse, err)
	}
	backupInterval, err := settings.Duration(keyBackupInterval)
	if err != nil {
		return nil, fmt.Errorf("units: read ⚙ %s: %w", keyBackupInterval, err)
	}
	snapCal, err := onCalendarFromSeconds(int64(backupInterval.Seconds()))
	if err != nil {
		return nil, fmt.Errorf("units: ⚙ %s: %w", keyBackupInterval, err)
	}
	drillMonths, err := settings.Int(keyBackupDrillEach)
	if err != nil {
		return nil, fmt.Errorf("units: read ⚙ %s: %w", keyBackupDrillEach, err)
	}
	drillCal := onCalendarFromMonths(drillMonths)
	swapBin := p.LlamaSwapBinary
	if swapBin == "" {
		swapBin = defaultLlamaSwapBinary
	}
	swapCfg := p.LlamaSwapConfig
	if swapCfg == "" {
		swapCfg = defaultLlamaSwapConfig
	}
	swapListen := p.LlamaSwapListen
	if swapListen == "" {
		swapListen = defaultLlamaSwapListen
	}
	watchBin := p.WatchlistBinary
	if watchBin == "" {
		watchBin = defaultWatchlistBinary
	}
	watchStore := p.WatchlistDatastore
	if watchStore == "" {
		watchStore = defaultWatchlistDatastore
	}
	watchListen := p.WatchlistListen
	if watchListen == "" {
		watchListen = defaultWatchlistListen
	}
	watchHost, watchPort, err := net.SplitHostPort(watchListen)
	if err != nil {
		return nil, fmt.Errorf("units: watchlist listen address %q: %w", watchListen, err)
	}
	return []File{
		controlService(bin, watchdogSec),
		brokerService(bin),
		engineTemplate(),
		runTemplate(bin),
		portpoolService(bin),
		snapshotService(bin),
		snapshotTimer(snapCal, keyBackupInterval),
		restoreDrillService(bin),
		restoreDrillTimer(drillCal, keyBackupDrillEach),
		llamaSwapService(swapBin, swapCfg, swapListen),
		watchlistService(watchBin, watchStore, watchHost, watchPort),
		localSlice(),
		journaldDropIn(journalMaxUse),
	}, nil
}

// watchlistService renders the S14.6 adopted-organ unit for changedetection.io
// (Spec S14.6 T1 "own unit [S01.2]"). GENERATED, never installed — the host
// install is a B5-gate operator act. ExecStart runs the OPERATOR-INSTALLED
// changedetection.io entry point, NOT the sinet binary: the organ is adopted
// unmodified and integrated by REST + config only (S16.1, adopt-don't-fork).
//
// The flags are the pinned version's own (`-d` datastore, `-h` host, `-p`
// port — getopt string "6Csd:h:p:l:P:" at tag 0.55.8). Binding to loopback is
// deliberate: the organ's default host is 0.0.0.0.
func watchlistService(binary, datastore, host, port string) File {
	var b strings.Builder
	b.WriteString(header())
	fmt.Fprintf(&b, `# GENERATED, not installed (P3-B5-6A; install = the B5 gate). This is SINET
# configuration for the UNMODIFIED adopted changedetection.io organ (S16.1
# config-only integration, not a fork); changedetection.io ships no unit file
# of its own (verified by a recursive tree read at tag 0.55.8), so this unit is
# Sinet's. Its ExecStart runs the operator-installed changedetection.io binary,
# NEVER the sinet binary.
[Unit]
Description=Sinet watchlist executor (changedetection.io page-diff tier; Spec S14.6)
After=network.target

[Service]
Type=exec
# Loopback listen only (S01.1 invariant): the organ's OWN default is 0.0.0.0,
# so -h is load-bearing. Sinet drives its watch set over the REST API from the
# control plane; hits are POLLED, so no inbound route exists.
ExecStart=%s -d %s -h %s -p %s
Restart=on-failure
StateDirectory=sinet/watchlist
`, binary, datastore, host, port)
	b.WriteString(staticUser)
	b.WriteString(`# Standard hardening set (Spec S01.2), with one recorded exception:
# ProtectSystem= is 'full' rather than 'strict' because the organ is a Python
# application whose interpreter and site-packages live under /usr; its own
# writable state is confined to StateDirectory=. /home and /root stay hidden.
ProtectSystem=full
ProtectHome=yes
NoNewPrivileges=yes
PrivateTmp=yes
SystemCallFilter=@system-service

[Install]
WantedBy=multi-user.target
`)
	return File{Name: "sinet-watchlist.service", Content: b.String(), Draft: true}
}

// llamaSwapService renders the S12.2 adopted-organ unit for llama-swap (brief
// R11; §8 reading 1). GENERATED, never installed (install = the B4 gate /
// hardening session). ExecStart runs the operator-installed llama-swap binary
// + the generated config (structural config), NOT the sinet binary — llama-swap
// is an adopted organ with no new cmd/ or mode. Loopback bind (S01.1); own
// journal identity; User=sinet; the S01.2 hardening set as far as compatible
// (the GPU device exception is recorded in-file, S01.2). Placed in the
// local-inference slice (the designated systemd-oomd victim; slice POLICY is
// S10's, deferred — no policy values invented here).
func llamaSwapService(binary, config, listen string) File {
	var b strings.Builder
	b.WriteString(header())
	fmt.Fprintf(&b, `# GENERATED, not installed (P3-B4-5; install = B4 gate / hardening). This is
# SINET configuration for the UNMODIFIED adopted llama-swap organ (S16.1
# config-only integration, not a fork); llama-swap ships no unit file, so
# this unit is Sinet's. Its ExecStart runs the operator-installed llama-swap
# binary + the generated config (structural config), NEVER the sinet binary.
[Unit]
Description=Sinet local-inference front (llama-swap → llama-server; Spec S12.2)
After=network.target
# The control plane reaches this on loopback (platform plane, S12.6); it is
# not a hard dependency (the local tier degrades when absent, S12.4).

[Service]
Type=exec
# ExecStart: the operator-installed llama-swap binary + the generated config.
# Loopback listen only (S01.1 invariant); llama-swap spawns llama-server
# backends as child processes (one per loaded model; process death returns
# VRAM fully to zero, S12.2), so the whole tier sits in this unit's slice.
ExecStart=%s --listen %s --config %s
Restart=on-failure
# local-inference slice: the designated systemd-oomd victim (S12.2). oomd/
# weight POLICY is Spec S10's and is DEFERRED to B4-6/hardening — no policy
# values are invented here (brief R11).
Slice=%s
`, binary, listen, config, LocalSlice)
	b.WriteString(staticUser)
	// Hardening set, with the recorded GPU exception (S01.2: per-unit
	// exceptions recorded in-file with a reason).
	b.WriteString(`# Standard hardening set (Spec S01.2), with one recorded exception:
# PrivateDevices= is deliberately NOT set — the backends need /dev/nvidia*
# (CUDA); hiding devices would break GPU inference. ProtectSystem=strict keeps
# /usr read-only (llama-swap + the model cache live elsewhere). The model
# cache + config are read-only inputs; llama-swap writes no durable state.
# Power/residency (S12.8): this unit NEVER runs nvidia-persistenced (it would
# disable RTD3 deep GPU sleep) — nothing here starts it; -lgc clock caps are a
# root operator/hardening flag, never applied by the platform.
ProtectSystem=strict
NoNewPrivileges=yes
PrivateTmp=yes
SystemCallFilter=@system-service
`)
	b.WriteString(`
[Install]
WantedBy=multi-user.target
`)
	return File{Name: "sinet-llamaswap.service", Content: b.String(), Draft: true}
}

// localSlice renders the minimal local-inference slice (brief R11). It carries
// NO oomd/weight policy values — that POLICY is Spec S10's (arbitration), a
// B4-6/hardening concern; a comment states the deferral. GENERATED, not
// installed.
func localSlice() File {
	var b strings.Builder
	b.WriteString(header())
	fmt.Fprintf(&b, `# GENERATED, not installed (P3-B4-5). The local-inference slice: llama-swap
# and its llama-server backend children run here so the whole GPU tier is one
# systemd-oomd target (S12.2 — the designated victim under memory pressure).
[Unit]
Description=Sinet local-inference slice (GPU tier; Spec S12.2)

[Slice]
# oomd/weight POLICY (MemoryHigh=, ManagedOOMMemoryPressure=, CPUWeight=,
# io/memory accounting) is Spec S10's arbitration and is DEFERRED to
# B4-6/hardening — no policy values are invented here (brief R11). This slice
# file exists so the unit's Slice=%s target is present.
`, LocalSlice)
	return File{Name: LocalSlice, Content: b.String(), Draft: true}
}

func controlService(bin string, watchdogSec int64) File {
	var b strings.Builder
	b.WriteString(header())
	fmt.Fprintf(&b, `# Front chain (Spec S01.4): tailscale serve (tailnet-only TLS on %s,
# identity headers) -> caddy (routing; /events unbuffered) -> 127.0.0.1.
# This unit binds loopback only; the in-process listener lint refuses to
# start otherwise (P-T13-2, Spec S01.6 step 2).
[Unit]
Description=Sinet control plane (sole platform.db writer; Spec S01.2)
# The broker starts first: engines receive credentials at start and the
# control plane consumes broker operations (Spec S01.6).
After=network.target sinet-broker.service
Wants=sinet-broker.service

[Service]
Type=notify
ExecStart=%s control
`, FQDN, bin)
	fmt.Fprintf(&b, "# ⚙ shell.watchdog_sec (Spec S01.2): sd_notify heartbeat budget.\nWatchdogSec=%d\n", watchdogSec)
	b.WriteString(`Restart=on-failure
# /var/lib/sinet (platform.db home) and /etc/sinet (bootstrap config) —
# zero custom path code (Spec S01.11).
StateDirectory=sinet
ConfigurationDirectory=sinet
# MUST exceed the flush budget (Spec S01.6); the in-process shutdown budget
# is 20 s.
TimeoutStopSec=90
`)
	b.WriteString(staticUser)
	b.WriteString(hardening)
	b.WriteString(`
[Install]
WantedBy=multi-user.target
`)
	return File{Name: "sinet-control.service", Content: b.String()}
}

func brokerService(bin string) File {
	var b strings.Builder
	b.WriteString(header())
	fmt.Fprintf(&b, `# Separate process so the large-attack-surface control plane never holds
# decrypted member credentials (Spec S01.2, D2). UDS with peer-cred auth;
# no TCP listener. Broker internals — secrets at rest via systemd-creds +
# sops/age, LoadCredential= wiring — are Spec S11's and land at B1.
[Unit]
Description=Sinet credential broker (secrets never in run units; Spec S01.2)
Before=sinet-control.service
After=network.target

[Service]
Type=exec
ExecStart=%s broker
Restart=on-failure
# Broker state root (socket + per-person stores): systemd provisions
# /var/lib/sinet and exports $STATE_DIRECTORY, which the broker's
# defaultStateDir honors — without it, ProtectSystem=strict leaves the
# broker no writable path and it dies at first mkdir (found at the B2-gate
# host install; Spec S01.2/S01.11 zero-custom-path-code posture).
StateDirectory=sinet
`, bin)
	b.WriteString(staticUser)
	b.WriteString(hardening)
	b.WriteString(`
[Install]
WantedBy=multi-user.target
`)
	return File{Name: "sinet-broker.service", Content: b.String()}
}

func engineTemplate() File {
	var b strings.Builder
	b.WriteString(header())
	b.WriteString(`# DRAFT — not installable until B1: the ExecStart= invocation (per-user
# pinned 'opencode serve' on a per-user localhost port, credentials
# injected at start outside any sandbox, SPIKE P2-S3) is Spec S03's design.
# Directives below are the ones Spec S01.2 fixes for this template.
[Unit]
Description=Sinet engine instance for user %i (Spec S01.2)
After=sinet-broker.service
Wants=sinet-broker.service

[Service]
# ExecStart= lands at B1 (Spec S03).
Restart=on-failure
`)
	b.WriteString(staticUser)
	b.WriteString(hardening)
	return File{Name: "sinet-engine@.service", Content: b.String(), Draft: true}
}

func runTemplate(bin string) File {
	var b strings.Builder
	b.WriteString(header())
	b.WriteString(`# The fixed-ExecStart sandbox composition of Spec S11.8 Shape B: a
# root-installed template + a single name-scoped polkit grant. ExecStart is
# the run launcher (Spec S11.8) — fixed and non-agent-reachable; the control
# plane, as unprivileged sinet, writes only DATA (the per-run confinement/
# spawn record) to the spool file and starts the instance. Installing this
# template (root) and the ^sinet-run@.*\.service$ polkit rule is the operator
# gate step (no host changes at B1); per-run properties (RuntimeMaxSec= time
# ceiling, cgroup accounting) are set per instance at dispatch (Spec S01.2,
# S10). Wording reconciled to the multi-call binary (Spec S01.5): S11.8's
# /usr/lib/sinet/run-launch is this binary in the run-launch mode.
[Unit]
Description=Sinet run unit %i (one run's engine process in its sandbox; Spec S01.2/S11.8)

[Service]
# Harvest lane recipe (Spec S02.5): unit corpses must outlive the process
# tree so finished-during-outage results stay classifiable — never
# systemd-run --collect.
Type=exec
RemainAfterExit=yes
ExitType=cgroup
`)
	fmt.Fprintf(&b, "ExecStart=%s run-launch --job /run/sinet/jobs/%%i.json\n", bin)
	b.WriteString(`# Run units are never auto-restarted by PID 1: a dead run is recovered by
# the S02 recovery ladder (fork-from-checkpoint), a control-plane decision
# (Spec S01.2).
Restart=no
# Per-unit journald rate-limit overrides for chatty workers
# (LogRateLimitIntervalSec=/LogRateLimitBurst=) are set per instance
# (Spec S01.11).
`)
	b.WriteString(staticUser)
	return File{Name: "sinet-run@.service", Content: b.String()}
}

func portpoolService(bin string) File {
	var b strings.Builder
	b.WriteString(header())
	fmt.Fprintf(&b, `# Port-pool daemon for preview allocation (Spec S01.2; consumer Spec S13).
# Loopback only, like every sinet unit (Spec S01.1).
[Unit]
Description=Sinet preview port-pool daemon (Spec S01.2)
After=network.target

[Service]
Type=exec
ExecStart=%s portpool
Restart=on-failure
`, bin)
	b.WriteString(staticUser)
	b.WriteString(hardening)
	b.WriteString(`
[Install]
WantedBy=multi-user.target
`)
	return File{Name: "sinet-portpool.service", Content: b.String()}
}

// snapshotService is the one-shot `sinet snapshot` service (Spec S13.10). Its
// live configuration — the private snapshot-repo URL, the broker git-ssh-key
// profile, the escrow identity path, the file stores — rides an EnvironmentFile
// the operator supplies at the B4 gate/bring-up, so the LIVE leg is pure config
// with no code change (R24). Generated, never installed.
func snapshotService(bin string) File {
	var b strings.Builder
	b.WriteString(header())
	fmt.Fprintf(&b, `# One-shot platform-state snapshot (Spec S13.10). Bring-up config
# (SINET_SNAPSHOT_REMOTE, SINET_SNAPSHOT_GIT_PROFILE, SINET_SNAPSHOT_IDENTITY,
# SINET_SNAPSHOT_STORES) rides /etc/sinet/snapshot.env — the operator supplies
# it at the B4 gate (the private repo designation + broker key enrollment are
# operator prerequisites; nothing is created automatically).
[Unit]
Description=Sinet platform-state snapshot (Spec S13.10)
After=sinet-broker.service
Wants=sinet-broker.service

[Service]
Type=oneshot
EnvironmentFile=-/etc/sinet/snapshot.env
ExecStart=%s snapshot --snapshot-remote ${SINET_SNAPSHOT_REMOTE} --git-profile ${SINET_SNAPSHOT_GIT_PROFILE} --identity ${SINET_SNAPSHOT_IDENTITY} ${SINET_SNAPSHOT_STORES}
StateDirectory=sinet
`, bin)
	b.WriteString(staticUser)
	b.WriteString(hardening)
	return File{Name: "sinet-snapshot.service", Content: b.String(), Draft: true}
}

// snapshotTimer is the S01.7 PERSISTENT calendar timer for snapshots: a slot
// missed in suspend fires once on next activation (distinct from the in-process
// WAL/recovery tickers). OnCalendar derives from ⚙ backup.interval. Generated,
// never installed.
func snapshotTimer(onCalendar, key string) File {
	return calendarTimer("sinet-snapshot", "platform-state snapshot", onCalendar, key)
}

// restoreDrillService is the one-shot `sinet restore-drill` service (Spec
// S13.10): fetch the newest blob → verify against the ledger fail-closed →
// decrypt with the escrow identity → rebuild → integrity + S02.9. Generated,
// never installed.
func restoreDrillService(bin string) File {
	var b strings.Builder
	b.WriteString(header())
	fmt.Fprintf(&b, `# One-shot verified-restore drill (Spec S13.10: a backup that is not
# restore-tested does not exist). Shares /etc/sinet/snapshot.env with the
# snapshot service. A fail-closed drill raises a High flag (S14).
[Unit]
Description=Sinet restore drill (Spec S13.10)
After=sinet-broker.service
Wants=sinet-broker.service

[Service]
Type=oneshot
EnvironmentFile=-/etc/sinet/snapshot.env
ExecStart=%s restore-drill --snapshot-remote ${SINET_SNAPSHOT_REMOTE} --git-profile ${SINET_SNAPSHOT_GIT_PROFILE} --identity ${SINET_SNAPSHOT_IDENTITY}
StateDirectory=sinet
`, bin)
	b.WriteString(staticUser)
	b.WriteString(hardening)
	return File{Name: "sinet-restore-drill.service", Content: b.String(), Draft: true}
}

// restoreDrillTimer is the S01.7 persistent calendar timer for the restore
// drill; OnCalendar derives from ⚙ backup.drill_interval. Generated, never
// installed.
func restoreDrillTimer(onCalendar, key string) File {
	return calendarTimer("sinet-restore-drill", "restore drill", onCalendar, key)
}

// calendarTimer renders a persistent (suspend-catch-up) calendar timer unit
// (Spec S01.7). Persistent=true is what makes a missed slot fire once on next
// activation.
func calendarTimer(unit, what, onCalendar, key string) File {
	var b strings.Builder
	b.WriteString(header())
	fmt.Fprintf(&b, `# Persistent calendar timer for the %s (Spec S01.7: a slot missed in
# suspend fires once on next activation; NOT an in-process ticker). OnCalendar
# derives from ⚙ %s. A sub-day hour step (0/N) fires at 0,N,2N,... each day then
# resets at the day boundary; a non-clean ⚙ value honors the nearest whole
# hour/day cadence (F7 — never a generation failure inside the clamp).
[Unit]
Description=Sinet %s schedule (Spec S13.10)

[Timer]
OnCalendar=%s
Persistent=true
Unit=%s.service

[Install]
WantedBy=timers.target
`, what, key, what, onCalendar, unit)
	return File{Name: unit + ".timer", Content: b.String(), Draft: true}
}

// onCalendarFromSeconds maps a ⚙ backup.interval (seconds) to a systemd
// OnCalendar expression (Spec S01.7). It is TOTAL over the ⚙ clamp (6 h – 7 d)
// and never fails generation (F7): a day renders daily, a week weekly, other
// sub-day intervals as an hour step (systemd `0/N`, which fires at 0,N,2N,…
// within a day then resets at the day boundary — the documented wrap), and
// multi-day intervals as a day-of-month step (month-boundary caveat). Clean
// hour/day values render exactly; a non-clean value (a fractional hour/day the
// clamp technically permits) honors the NEAREST whole-hour/day cadence — never
// a silent mis-schedule (the timer comment states this), never a failure. Only
// a non-positive value (never clamp-valid) errors.
func onCalendarFromSeconds(sec int64) (string, error) {
	if sec <= 0 {
		return "", fmt.Errorf("interval %ds is not positive", sec)
	}
	d := time.Duration(sec) * time.Second
	day := 24 * time.Hour
	switch {
	case d == day:
		return "*-*-* 00:00:00", nil // daily
	case d == 7*day:
		return "Mon *-*-* 00:00:00", nil // weekly
	case d < day:
		h := int((d + 30*time.Minute) / time.Hour) // nearest whole hour
		if h < 1 {
			h = 1
		}
		if h > 23 {
			h = 23
		}
		return fmt.Sprintf("*-*-* 0/%d:00:00", h), nil
	default:
		days := int((d + 12*time.Hour) / day) // nearest whole day
		if days < 1 {
			days = 1
		}
		return fmt.Sprintf("*-*-1/%d 00:00:00", days), nil
	}
}

// onCalendarFromMonths maps a ⚙ backup.drill_interval (months) to a systemd
// OnCalendar expression: every n-th month on the 1st (a year-boundary caveat for
// n that does not divide 12). n=1 is monthly.
func onCalendarFromMonths(n int64) string {
	if n <= 1 {
		return "*-*-01 00:00:00"
	}
	return fmt.Sprintf("*-1/%d-01 00:00:00", n)
}

func journaldDropIn(journalMaxUse int64) File {
	var b strings.Builder
	b.WriteString(header())
	fmt.Fprintf(&b, `# Intended target: /etc/systemd/journald.conf.d/sinet.conf
# (restart-required ⚙; journald restart applies it).
[Journal]
# ⚙ shell.journal_max_use (Spec S01.11): journald is the ops log only and
# its total use is capped; the platform.db event log is the only audit
# truth.
SystemMaxUse=%d
`, journalMaxUse)
	return File{Name: "journald-sinet.conf", Content: b.String()}
}
