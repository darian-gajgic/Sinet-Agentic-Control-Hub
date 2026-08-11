package run

import (
	"context"
	"log/slog"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// INERT SURFACE (P3-RW-5 red-tests commit): the types the acceptance battery
// needs to compile. Behaviour lands in the implementation commit.

// LeaseSettings is the keeper's view of the settings registry (Spec S01.10).
type LeaseSettings interface {
	Duration(key string) (time.Duration, error)
}

// LeaseKeeperConfig assembles a LeaseKeeper.
type LeaseKeeperConfig struct {
	DB       *storage.DB
	Runs     *Store
	Settings LeaseSettings
	Logger   *slog.Logger
	Now      func() time.Time
	Tick     func(d time.Duration) (<-chan time.Time, func())
}

// LeaseKeeper renews the lease of a run the platform is actively driving.
type LeaseKeeper struct{ cfg LeaseKeeperConfig }

// NewLeaseKeeper returns the keeper.
func NewLeaseKeeper(cfg LeaseKeeperConfig) (*LeaseKeeper, error) { return &LeaseKeeper{cfg: cfg}, nil }

// Beat performs one immediate fenced renewal.
func (k *LeaseKeeper) Beat(ctx context.Context, runID, holder string) {}

// Hold renews runID's lease until the returned release func is called.
func (k *LeaseKeeper) Hold(ctx context.Context, runID, holder string) func() { return func() {} }
