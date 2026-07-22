package portpool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

// serve.go is the `sinet portpool` daemon's serve loop and a thin client — the
// same file-backed Allocator (portpool.go) exposed over a loopback/UDS listener
// so the generated sinet-portpool.service has a working mode behind it (Spec
// S01.2; S13.8 R10). The transport is a minimal JSON HTTP API (the S01.4
// IPC-transport choice is P3's within the loopback/UDS bound); no external
// dependency. The v0 control-plane manager consumes the Allocator IN-PROCESS —
// this daemon is the standing S01.2 organ, not the dev-mode path.

// Server serves an Allocator over a listener.
type Server struct {
	alloc *Allocator
	log   *slog.Logger
}

// NewServer wraps an allocator.
func NewServer(alloc *Allocator, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{alloc: alloc, log: log}
}

// Handler is the loopback/UDS-only JSON API: POST /allocate {"owner":…} →
// {"port":…}; POST /release {"port":…}; GET /list → [reservation…].
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /allocate", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Owner string `json:"owner"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		port, err := s.alloc.Allocate(req.Owner)
		if errors.Is(err, ErrBadOwner) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if errors.Is(err, ErrPoolExhausted) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]int{"port": port})
	})
	mux.HandleFunc("POST /release", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Port int `json:"port"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.alloc.Release(req.Port); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /list", func(w http.ResponseWriter, _ *http.Request) {
		list, err := s.alloc.List()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if list == nil {
			list = []Reservation{}
		}
		writeJSON(w, list)
	})
	return mux
}

// Serve runs the HTTP API on ln until ctx is cancelled, then shuts down
// gracefully. The caller owns ln (and the loopback/UDS assertion on it).
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// Client dials a portpool daemon over its UDS (or loopback TCP) and calls the
// allocate/release/list API.
type Client struct {
	http *http.Client
	base string
}

// DialUDS returns a client speaking to the daemon's Unix-domain socket.
func DialUDS(socket string) *Client {
	return &Client{
		base: "http://unix",
		http: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, "unix", socket)
				},
			},
		},
	}
}

// DialLoopback returns a client speaking to a loopback TCP daemon.
func DialLoopback(addr string) *Client {
	return &Client{base: "http://" + addr, http: &http.Client{Timeout: 5 * time.Second}}
}

// Allocate requests a port for owner.
func (c *Client) Allocate(owner string) (int, error) {
	body, _ := json.Marshal(map[string]string{"owner": owner})
	resp, err := c.http.Post(c.base+"/allocate", "application/json", strings.NewReader(string(body)))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return 0, ErrPoolExhausted
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("portpool client: allocate: status %d", resp.StatusCode)
	}
	var out struct {
		Port int `json:"port"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	return out.Port, nil
}

// Release frees a port.
func (c *Client) Release(port int) error {
	body, _ := json.Marshal(map[string]int{"port": port})
	resp, err := c.http.Post(c.base+"/release", "application/json", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("portpool client: release: status %d", resp.StatusCode)
	}
	return nil
}

// List returns the daemon's current reservations.
func (c *Client) List() ([]Reservation, error) {
	resp, err := c.http.Get(c.base + "/list")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("portpool client: list: status %d", resp.StatusCode)
	}
	var out []Reservation
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// assertLoopback fails closed for any TCP address that is not a literal
// loopback IP (Spec S01.1 invariant; the P-T13-2 listener-binding posture —
// literal loopback IPs only, hostnames including "localhost" rejected). A UDS
// path is host-local by construction and needs no check.
func assertLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("portpool: address %q is not host:port: %w", addr, err)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("portpool: bind host %q is not a literal IP (hostnames incl. localhost are rejected, P-T13-2)", host)
	}
	if !ip.IsLoopback() {
		return fmt.Errorf("portpool: bind host %q is not loopback (Spec S01.1: 127.0.0.1 / UDS only)", host)
	}
	return nil
}
