package portpool

import (
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func newAlloc(t *testing.T, lo, hi int) *Allocator {
	t.Helper()
	a, err := New(Config{Dir: filepath.Join(t.TempDir(), "res"), Lo: lo, Hi: hi})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

// TestAllocateReleaseRoundTrip: allocate hands out distinct ports, release
// frees them, and release is idempotent (Spec S13.8 R9/R16).
func TestAllocateReleaseRoundTrip(t *testing.T) {
	a := newAlloc(t, 47600, 47602)
	p1, err := a.Allocate("preview-a")
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	p2, err := a.Allocate("preview-b")
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if p1 == p2 {
		t.Fatalf("allocated the same port twice: %d", p1)
	}
	if p1 < 47600 || p1 > 47602 || p2 < 47600 || p2 > 47602 {
		t.Fatalf("ports %d,%d out of configured range [47600,47602]", p1, p2)
	}
	if err := a.Release(p1); err != nil {
		t.Fatalf("Release: %v", err)
	}
	// Idempotent: releasing again is not an error.
	if err := a.Release(p1); err != nil {
		t.Fatalf("Release (idempotent): %v", err)
	}
	// The freed port is handed out again.
	p3, err := a.Allocate("preview-c")
	if err != nil {
		t.Fatalf("Allocate after release: %v", err)
	}
	if p3 != p1 {
		t.Fatalf("expected freed port %d re-allocated, got %d", p1, p3)
	}
}

// TestPoolExhausted: a full range with no reclaimable reservation refuses the
// next allocation with ErrPoolExhausted (the honest at-capacity signal).
func TestPoolExhausted(t *testing.T) {
	a := newAlloc(t, 47600, 47601)
	if _, err := a.Allocate("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Allocate("b"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Allocate("c"); err != ErrPoolExhausted {
		t.Fatalf("expected ErrPoolExhausted, got %v", err)
	}
}

// TestStaleReclaim: a crash-orphaned reservation older than StaleAfter is
// reclaimed when the range is otherwise full (stale-owner detection, R9).
func TestStaleReclaim(t *testing.T) {
	base := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	clk := base
	a, err := New(Config{
		Dir: filepath.Join(t.TempDir(), "res"), Lo: 47600, Hi: 47600,
		StaleAfter: time.Hour, Now: func() time.Time { return clk },
	})
	if err != nil {
		t.Fatal(err)
	}
	p, err := a.Allocate("crashed-owner")
	if err != nil {
		t.Fatal(err)
	}
	// The single-port range is now full: a same-instant retry is refused.
	if _, err := a.Allocate("fresh"); err != ErrPoolExhausted {
		t.Fatalf("expected exhausted while reservation is fresh, got %v", err)
	}
	// Advance past StaleAfter — the orphan is now reclaimable.
	clk = base.Add(2 * time.Hour)
	p2, err := a.Allocate("fresh")
	if err != nil {
		t.Fatalf("expected reclaim of stale reservation, got %v", err)
	}
	if p2 != p {
		t.Fatalf("reclaimed the wrong port: got %d want %d", p2, p)
	}
	list, err := a.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Owner != "fresh" {
		t.Fatalf("expected one reservation owned by fresh, got %+v", list)
	}
}

// TestListReservations reports every held port with its owner.
func TestListReservations(t *testing.T) {
	a := newAlloc(t, 47600, 47609)
	for _, owner := range []string{"x", "y", "z"} {
		if _, err := a.Allocate(owner); err != nil {
			t.Fatal(err)
		}
	}
	list, err := a.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 reservations, got %d", len(list))
	}
	ports := make([]int, 0, 3)
	for _, r := range list {
		ports = append(ports, r.Port)
	}
	sort.Ints(ports)
	if ports[0] != 47600 || ports[2] != 47602 {
		t.Fatalf("unexpected ports %v", ports)
	}
}

// TestStructuralRangeDefault: no ⚙ key exists — the range defaults structurally
// and is small (the pre-installed static-units shape, §8 reading 1).
func TestStructuralRangeDefault(t *testing.T) {
	a, err := New(Config{Dir: filepath.Join(t.TempDir(), "res")})
	if err != nil {
		t.Fatal(err)
	}
	lo, hi := a.Range()
	if lo != DefaultRangeLo || hi != DefaultRangeHi {
		t.Fatalf("default range = [%d,%d], want [%d,%d]", lo, hi, DefaultRangeLo, DefaultRangeHi)
	}
	if hi-lo+1 > 64 {
		t.Fatalf("default range %d ports is not small (load-bearing for static units)", hi-lo+1)
	}
}

// TestBadOwnerRejected: an empty owner is refused (audit + stale-reclaim signal).
func TestBadOwnerRejected(t *testing.T) {
	a := newAlloc(t, 47600, 47600)
	if _, err := a.Allocate(""); err != ErrBadOwner {
		t.Fatalf("expected ErrBadOwner, got %v", err)
	}
}

// TestCrossInstanceClaim: two allocators over the SAME reservation dir never
// hand out the same port — the O_EXCL file claim is the cross-process guard
// (the daemon + the in-process manager coordinate over one dir).
func TestCrossInstanceClaim(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "shared")
	a1, err := New(Config{Dir: dir, Lo: 47600, Hi: 47601})
	if err != nil {
		t.Fatal(err)
	}
	a2, err := New(Config{Dir: dir, Lo: 47600, Hi: 47601})
	if err != nil {
		t.Fatal(err)
	}
	p1, err := a1.Allocate("a")
	if err != nil {
		t.Fatal(err)
	}
	p2, err := a2.Allocate("b")
	if err != nil {
		t.Fatal(err)
	}
	if p1 == p2 {
		t.Fatalf("two instances over one dir claimed the same port %d", p1)
	}
}
