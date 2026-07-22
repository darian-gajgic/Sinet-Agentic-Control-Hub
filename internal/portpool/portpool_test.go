package portpool

import (
	"fmt"
	"path/filepath"
	"sort"
	"sync"
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

// TestStaleReclaimContention pins F11: separate allocators over ONE shared dir
// (distinct processes), racing to reclaim a range full of STALE reservations,
// never double-claim a port. The shipped mechanism is a per-port O_EXCL lock
// (<port>.json.lock) whose critical section removes the stale file AND hard-links
// the fresh one atomically, so no second reclaimer can remove the fresh
// reservation a winner just linked, and the temp+link claim never leaves an
// empty file to misread. One run drives `contentionRounds` fresh rounds so a
// single `go test -race` shakes the interleaving repeatedly (R2-3). Run -race.
func TestStaleReclaimContention(t *testing.T) {
	const contentionRounds = 40
	for round := 0; round < contentionRounds; round++ {
		runContentionRound(t)
	}
}

// runContentionRound fills a 4-port range with stale reservations, then races 8
// separate allocators to reclaim them, asserting no port is double-owned.
func runContentionRound(t *testing.T) {
	t.Helper()
	base := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	clk := base
	dir := filepath.Join(t.TempDir(), "res")
	mk := func() *Allocator {
		a, err := New(Config{Dir: dir, Lo: 47600, Hi: 47603, StaleAfter: time.Hour, Now: func() time.Time { return clk }})
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	// Fill the 4-port range with reservations, then age them past StaleAfter.
	seed := mk()
	for i := 0; i < 4; i++ {
		if _, err := seed.Allocate("crashed"); err != nil {
			t.Fatal(err)
		}
	}
	clk = base.Add(2 * time.Hour) // set BEFORE the goroutines: no concurrent clk write

	const workers = 8 // more claimants than ports (4)
	type res struct {
		port int
		err  error
	}
	out := make(chan res, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			p, err := mk().Allocate(fmt.Sprintf("claim-%d", id))
			out <- res{p, err}
		}(i)
	}
	wg.Wait()
	close(out)

	seen := map[int]int{}
	success := 0
	for r := range out {
		if r.err == nil {
			seen[r.port]++
			success++
		}
	}
	for port, c := range seen {
		if c > 1 {
			t.Errorf("port %d double-claimed %d times — the reclaim race (F11)", port, c)
		}
	}
	if success < 1 || success > 4 {
		t.Errorf("reclaim successes = %d, want 1..4 (exactly 4 ports)", success)
	}
	// The strongest invariant: the count of successful allocations equals the
	// number of live reservation files — no port is owned by two callers at once.
	final, err := seed.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(final) != success {
		t.Errorf("live reservations (%d) != successful allocations (%d) — a port is double-owned (F11)", len(final), success)
	}
}
