package preview

import (
	"fmt"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/units"
)

// units.go renders the per-preview systemd-socket-proxyd idle-stop unit TEXT
// (Spec S13.8: "systemd-socket-proxyd --exit-idle-time + StopWhenUnneeded= give
// on-demand start and idle-stop for free from the substrate"; R14). The pair is
// GENERATED, never installed — the B0/B4-3 generated-never-installed precedent
// (CONVENTIONS §7/§24): this returns text, nothing writes /etc, calls systemctl,
// or starts anything. In the grant-compatible production shape (§8 reading 1)
// these are the fixed static pairs pre-installed once per pool port at the gate;
// the S11.8 grant cannot install per-preview dynamic units, so systemd itself
// does the on-demand activation and --exit-idle-time/StopWhenUnneeded the
// idle-stop.
//
// systemd-socket-proxyd is part of host systemd (host-managed, the B4-3 timers
// precedent) — NO components.lock entry. The static owned unit set in
// internal/units is unchanged; these are dynamic parameterized data reusing the
// units.File shape (R14).

// proxydBinary is host systemd's socket-proxyd (text render only; host-managed).
const proxydBinary = "/usr/lib/systemd/systemd-socket-proxyd"

// RenderProxydUnits renders the .socket + proxyd .service pair for one pool port
// (Spec S13.8; R14): the socket listens loopback-only on the pool port, the
// proxyd forwards to the in-netns dev-server backend, and idle-stop is
// --exit-idle-time from ⚙ preview.idle_stop (the proxy exits when idle, the
// socket re-arms, the next connection re-activates it — the correct mechanism
// for this no-backend-unit topology; StopWhenUnneeded is not set, F9). Text
// only; installing/enabling/starting is a gate operator step, never this code's.
func RenderProxydUnits(poolPort int, backend string, idle time.Duration) (socket, service units.File, err error) {
	if poolPort <= 0 {
		return units.File{}, units.File{}, fmt.Errorf("preview: proxyd units need a positive pool port, got %d", poolPort)
	}
	if backend == "" {
		return units.File{}, units.File{}, fmt.Errorf("preview: proxyd units need a backend address")
	}
	idleSec := int64(idle.Seconds())
	if idleSec < 1 {
		idleSec = 1
	}
	base := fmt.Sprintf("sinet-preview-%d", poolPort)

	socket = units.File{
		Name: base + ".socket",
		Content: fmt.Sprintf(`# GENERATED per-preview socket (Spec S13.8) — never installed by this code;
# host install is a gate operator step (§8 reading 1: fixed static pairs, one
# per pool port). Loopback only (Spec S01.1); the tailnet front chain is the
# only exposure (S13.8/S01.4).
[Unit]
Description=Sinet preview socket for pool port %d (Spec S13.8)

[Socket]
ListenStream=127.0.0.1:%d

[Install]
WantedBy=sockets.target
`, poolPort, poolPort),
	}

	service = units.File{
		Name: base + ".service",
		Content: fmt.Sprintf(`# GENERATED per-preview socket-proxyd (Spec S13.8) — never installed by this
# code. systemd-socket-proxyd is host systemd (host-managed, no components.lock
# entry). Idle-stop in THIS topology (no separate backend unit) is
# --exit-idle-time=<⚙ preview.idle_stop>: the proxy EXITS when idle and the
# paired .socket re-arms, so the next connection re-activates it (Spec S13.8 R14
# names the mechanism; StopWhenUnneeded is deliberately NOT set — nothing
# Requires= this proxy, so it would let systemd stop a just-activated proxy; F9).
[Unit]
Description=Sinet preview socket-proxyd for pool port %d (Spec S13.8)
Requires=%s.socket
After=%s.socket

[Service]
ExecStart=%s --exit-idle-time=%ds %s
User=sinet
Group=sinet
NoNewPrivileges=yes
ProtectSystem=strict
PrivateTmp=yes
`, poolPort, base, base, proxydBinary, idleSec, backend),
	}
	return socket, service, nil
}
