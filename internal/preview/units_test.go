package preview

import (
	"strings"
	"testing"
	"time"
)

// TestRenderProxydUnits: the per-preview .socket + socket-proxyd .service TEXT
// carries the ⚙-derived --exit-idle-time, a loopback-only ListenStream, and
// StopWhenUnneeded=yes — GENERATED, never installed (Spec S13.8; R14).
func TestRenderProxydUnits(t *testing.T) {
	idle := 900 * time.Second // ⚙ preview.idle_stop default, passed by the caller
	socket, service, err := RenderProxydUnits(47600, "127.0.0.1:7681", idle)
	if err != nil {
		t.Fatalf("RenderProxydUnits: %v", err)
	}
	if socket.Name != "sinet-preview-47600.socket" || service.Name != "sinet-preview-47600.service" {
		t.Fatalf("unit names = %q,%q", socket.Name, service.Name)
	}
	// Socket: loopback-only ListenStream on the pool port.
	if !strings.Contains(socket.Content, "ListenStream=127.0.0.1:47600") {
		t.Errorf("socket lacks loopback ListenStream:\n%s", socket.Content)
	}
	if strings.Contains(socket.Content, "0.0.0.0") {
		t.Errorf("socket binds non-loopback (host hazard):\n%s", socket.Content)
	}
	// Service: --exit-idle-time from ⚙, StopWhenUnneeded, host systemd-socket-proxyd.
	if !strings.Contains(service.Content, "--exit-idle-time=900s") {
		t.Errorf("service lacks ⚙-derived --exit-idle-time:\n%s", service.Content)
	}
	if !strings.Contains(service.Content, "StopWhenUnneeded=yes") {
		t.Errorf("service lacks StopWhenUnneeded=yes:\n%s", service.Content)
	}
	if !strings.Contains(service.Content, "systemd-socket-proxyd") {
		t.Errorf("service lacks systemd-socket-proxyd ExecStart:\n%s", service.Content)
	}
	if !strings.Contains(service.Content, "127.0.0.1:7681") {
		t.Errorf("service lacks the backend address:\n%s", service.Content)
	}
	if !strings.Contains(service.Content, "User=sinet") {
		t.Errorf("service lacks User=sinet:\n%s", service.Content)
	}
	// GENERATED-never-installed: the render is pure text — a marker comment
	// states it, and nothing here touches the host.
	if !strings.Contains(service.Content, "never installed") {
		t.Errorf("service lacks the generated-never-installed marker:\n%s", service.Content)
	}
}

// TestRenderProxydUnitsIdleFromSetting: a different ⚙ idle value renders through
// (proof the value is not a constant, R14/R15).
func TestRenderProxydUnitsIdleFromSetting(t *testing.T) {
	_, service, err := RenderProxydUnits(47605, "127.0.0.1:5173", 60*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(service.Content, "--exit-idle-time=60s") {
		t.Errorf("idle value not rendered from the setting:\n%s", service.Content)
	}
}

// TestRenderProxydUnitsRejectsBadInput: a non-positive port or empty backend is
// refused loudly (never a malformed unit).
func TestRenderProxydUnitsRejectsBadInput(t *testing.T) {
	if _, _, err := RenderProxydUnits(0, "127.0.0.1:1", time.Second); err == nil {
		t.Errorf("expected error for a non-positive port")
	}
	if _, _, err := RenderProxydUnits(47600, "", time.Second); err == nil {
		t.Errorf("expected error for an empty backend")
	}
}
