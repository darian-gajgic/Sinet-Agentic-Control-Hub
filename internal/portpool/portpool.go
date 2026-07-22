// Package portpool is the Spec S13.8 preview port-pool allocator and its
// Spec S01.2 daemon mode (`sinet-portpool.service`). Each live preview gets a
// host-side public port — the Caddy upstream / socket-unit ListenStream — from
// a SMALL fixed configured host-port range, backed by file-based reservations
// (the "file-stub" of P3/STATE.md B2): an O_EXCL reservation file per port is
// the atomic, cross-process claim, so the in-process control-plane manager and
// the standalone daemon coordinate over one shared directory without a lock
// server.
//
// The port range is STRUCTURAL config (a code default plus a composition-root
// override) — Spec S18 ratifies no port-range ⚙ key and none is invented; a
// small fixed range is load-bearing for the pre-installed static socket+proxyd
// unit shape (the S11.8 grant cannot install per-preview units). Dev mode and
// tests consume the allocator IN-PROCESS — no daemon required (Spec S13.8
// "the port comes from sinet-portpool"; the file-stub headline). The daemon
// (serve.go / mode.go) serves the same allocator loopback/UDS-only so the
// generated unit has a working mode behind it (Spec S01.1 invariant).
//
// Import discipline (CONVENTIONS §25): stdlib only, plus the settings interface
// at most — the allocator core is a leaf so previews and the daemon both rest
// on it.
package portpool

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Default fixed range (STRUCTURAL, Spec S13.8/S01.2; overridable at the
// composition root). Small and load-bearing: the pre-installed static socket+
// proxyd unit pairs are one per port in this range (§8 reading 1). Loopback
// preview ports — deliberately far from the LIVE front chain (127.0.0.1:8481/
// 8482), which this package never touches (host hazard).
const (
	DefaultRangeLo = 47600
	DefaultRangeHi = 47619
)

// DefaultStaleAfter is the age past which a reservation is treated as a crash
// orphan and reclaimable. It sits safely beyond the ⚙ preview.idle_stop max
// clamp (86400 s) so a legitimately long-idle live preview is never reclaimed
// out from under itself — only a reservation left behind by a crashed process
// (previews are in-memory and disposable, Spec S13.8, so nothing refreshes the
// file) is reclaimed. STRUCTURAL, overridable per allocator.
const DefaultStaleAfter = 48 * time.Hour

// Errors callers branch on.
var (
	// ErrPoolExhausted: every port in the configured range is reserved (and
	// none reclaimable) — the honest at-capacity signal.
	ErrPoolExhausted = errors.New("portpool: no free port in the configured range")
	// ErrBadOwner rejects an empty reservation owner (owner attribution is the
	// stale-reclaim and audit signal).
	ErrBadOwner = errors.New("portpool: reservation owner is required")
	// ErrBadRange rejects a lo > hi or non-positive range.
	ErrBadRange = errors.New("portpool: invalid port range")
)

// Reservation is one file-backed port reservation.
type Reservation struct {
	Port       int       `json:"port"`
	Owner      string    `json:"owner"`
	ReservedAt time.Time `json:"reserved_at"`
}

// Config assembles an Allocator. Dir is required; the rest default.
type Config struct {
	// Dir holds one reservation file per allocated port. Created if absent.
	Dir string
	// Lo/Hi bound the fixed range (inclusive). Zero ⇒ the structural default.
	Lo, Hi int
	// StaleAfter overrides DefaultStaleAfter; a negative value disables
	// stale-reclaim entirely (a reservation is then held until explicitly
	// released).
	StaleAfter time.Duration
	// Now is the clock seam (nil ⇒ time.Now) — the stale-reclaim test drives it.
	Now func() time.Time
}

// Allocator is the file-backed port allocator over a fixed range. It is safe
// for concurrent in-process use (the mutex) and safe across processes (the
// O_EXCL reservation-file claim).
type Allocator struct {
	dir        string
	lo, hi     int
	staleAfter time.Duration
	now        func() time.Time
	mu         sync.Mutex
}

// lockStale bounds a reclaim critical section (a stat + a remove — microseconds);
// a per-port reclaim lock older than this is a crashed-holder leak and is itself
// reclaimable, so a crash during reclaim can never permanently strand a port.
const lockStale = 30 * time.Second

// New opens an allocator over Config.Dir, creating the directory if absent.
func New(cfg Config) (*Allocator, error) {
	if cfg.Dir == "" {
		return nil, errors.New("portpool: reservation Dir is required")
	}
	lo, hi := cfg.Lo, cfg.Hi
	if lo == 0 && hi == 0 {
		lo, hi = DefaultRangeLo, DefaultRangeHi
	}
	if lo <= 0 || hi < lo {
		return nil, fmt.Errorf("%w: [%d,%d]", ErrBadRange, lo, hi)
	}
	if err := os.MkdirAll(cfg.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("portpool: reservation dir: %w", err)
	}
	staleAfter := cfg.StaleAfter
	if staleAfter == 0 {
		staleAfter = DefaultStaleAfter
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Allocator{dir: cfg.Dir, lo: lo, hi: hi, staleAfter: staleAfter, now: now}, nil
}

// Range reports the configured inclusive range (evidence + the socket-unit
// pre-install shape).
func (a *Allocator) Range() (lo, hi int) { return a.lo, a.hi }

func (a *Allocator) path(port int) string {
	return filepath.Join(a.dir, fmt.Sprintf("%d.json", port))
}

// Allocate reserves the lowest free port in the range for owner and returns it.
// The claim is a hard-link of a fully-written temp file (tryClaim) — atomic
// across processes AND never leaving an empty/half-written reservation visible,
// so a concurrent reclaimer can never misread a just-claimed slot. A reservation
// older than StaleAfter is reclaimed as a crash orphan under a per-port lock
// (reclaim). When every port is reserved and none is reclaimable,
// ErrPoolExhausted.
func (a *Allocator) Allocate(owner string) (int, error) {
	if owner == "" {
		return 0, ErrBadOwner
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	for port := a.lo; port <= a.hi; port++ {
		path := a.path(port)
		ok, err := a.tryClaim(path, port, owner, now)
		if err != nil {
			return 0, err
		}
		if ok {
			return port, nil
		}
		// Occupied — if the reservation is a stale crash orphan, reclaim AND claim
		// it atomically under the per-port lock (remove + link in one locked
		// critical section, so no other reclaimer can remove the fresh slot we
		// just linked, F11).
		ok, err = a.reclaimAndClaim(path, port, owner, now)
		if err != nil {
			return 0, err
		}
		if ok {
			return port, nil
		}
	}
	return 0, ErrPoolExhausted
}

// tryClaim atomically creates the reservation at path IF it does not exist: it
// writes the full content to a temp file, then HARD-LINKs it into place (link
// fails EEXIST if the port is taken — the atomic single-winner claim — and the
// file appears at path already complete, so no reader ever sees an empty or
// half-written reservation). Returns (true,nil) on claim, (false,nil) when the
// port is taken.
func (a *Allocator) tryClaim(path string, port int, owner string, now time.Time) (bool, error) {
	enc, err := json.Marshal(Reservation{Port: port, Owner: owner, ReservedAt: now})
	if err != nil {
		return false, fmt.Errorf("portpool: marshal reservation: %w", err)
	}
	tmp, err := os.CreateTemp(a.dir, ".tmp-*")
	if err != nil {
		return false, fmt.Errorf("portpool: temp reservation: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(enc); err != nil {
		tmp.Close()
		return false, fmt.Errorf("portpool: write reservation: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return false, fmt.Errorf("portpool: close reservation: %w", err)
	}
	if err := os.Link(tmpName, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return false, nil // port already claimed
		}
		return false, fmt.Errorf("portpool: link reservation: %w", err)
	}
	return true, nil
}

// Release frees a port's reservation. Idempotent: an already-absent reservation
// is not an error (teardown re-runs safely, Spec S13.8 R16).
func (a *Allocator) Release(port int) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := os.Remove(a.path(port)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("portpool: release %d: %w", port, err)
	}
	return nil
}

// List returns the current reservations (unordered; the caller sorts if it
// needs order). A corrupt/unparseable file is skipped — it is reclaimed as
// stale on the next Allocate.
func (a *Allocator) List() ([]Reservation, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	entries, err := os.ReadDir(a.dir)
	if err != nil {
		return nil, fmt.Errorf("portpool: list: %w", err)
	}
	var out []Reservation
	for _, e := range entries {
		// Only reservation files (<port>.json); a reclaim tombstone
		// (<port>.json.reclaim.<pid>.<seq>) has a different final extension and
		// is skipped (F11).
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		res, ok := a.read(filepath.Join(a.dir, e.Name()))
		if ok {
			out = append(out, res)
		}
	}
	return out, nil
}

// reclaimAndClaim reclaims a STALE reservation and claims the port in ONE locked
// critical section (F11): a per-port O_EXCL <path>.lock serializes reclaimers,
// and — crucially — the stale REMOVE and the fresh CLAIM (link) both happen under
// that lock, so no second reclaimer can remove the fresh reservation this one
// just linked (the gap-between-remove-and-link race). Under the lock it
// RE-verifies staleness (a concurrent claimant may have recreated a fresh
// reservation) and never removes a fresh one. Returns (true,nil) when the port
// was reclaimed and claimed; (false,nil) when the slot is fresh, the lock is
// held, or an outer claimant grabbed the freed slot first. Called under a.mu.
func (a *Allocator) reclaimAndClaim(path string, port int, owner string, now time.Time) (bool, error) {
	if a.staleAfter < 0 || !a.stale(path, now) {
		return false, nil // stale-reclaim disabled, or the reservation is fresh
	}
	lock := path + ".lock"
	lf, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, os.ErrExist) {
		// A lock older than lockStale is a crashed-holder leak: remove + retry once
		// so a crash during reclaim never strands the port. Real-wall-clock (lock
		// mtime), distinct from the injectable reservation-staleness clock.
		if fi, statErr := os.Stat(lock); statErr == nil && time.Since(fi.ModTime()) > lockStale {
			_ = os.Remove(lock)
			lf, err = os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		}
	}
	if err != nil {
		return false, nil // another reclaimer holds the lock — skip this port
	}
	defer func() { _ = lf.Close(); _ = os.Remove(lock) }()
	if !a.stale(path, now) {
		return false, nil // recreated fresh since the pre-check — never remove it
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	// Claim WHILE HOLDING THE LOCK: an outer (unlocked) tryClaim may have grabbed
	// the freed slot first → link EEXIST → (false,nil), and we yield to it.
	return a.tryClaim(path, port, owner, now)
}

// stale reports whether the reservation at path is older than StaleAfter. An
// unparseable/unreadable reservation is treated as stale (a crash orphan or
// corruption is reclaimable, never a permanent leak).
func (a *Allocator) stale(path string, now time.Time) bool {
	res, ok := a.read(path)
	if !ok {
		return true
	}
	return now.Sub(res.ReservedAt) > a.staleAfter
}

func (a *Allocator) read(path string) (Reservation, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Reservation{}, false
	}
	var res Reservation
	if err := json.Unmarshal(b, &res); err != nil {
		return Reservation{}, false
	}
	return res, true
}
