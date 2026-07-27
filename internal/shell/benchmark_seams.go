package shell

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/benchmark"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/evals"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/verify"
)

// The Spec S14.7 benchmark practice, composed at the root (B5-7).
//
// internal/benchmark never imports evals, verify, intake, review or accept: the
// BENCH-REG §10.1(d) floor reading, the §2 frozen task statement and the two
// arms' texts all ride func seams satisfied HERE, exactly as the S14.8 eval
// surfaces and the S12.10 swap gate do (CONVENTIONS §28/§33/§34). Composition is
// wiring only — no package gains a forbidden edge, and internal/review and
// internal/accept are untouched: the sampling hook DERIVES its trigger from the
// deliverable.accepted rows they already write.
//
// The shell owns WHEN (the sampling loop below); the package owns the LOGIC.

// benchmarkSurface holds the composed S14.7 machinery.
type benchmarkSurface struct {
	Store    *benchmark.Store
	Sampler  *benchmark.Sampler
	Practice *benchmark.Practice
	// Domains counts the registered launch domains whose bookkeeping row exists
	// after this boot.
	Domains int
	// FloorsWired records whether gate limb (d) is evaluable at all. False is an
	// honest state, not a failure: without the S14.8 surface the limb reads
	// "not evaluable — the gate cannot open", never a fake green.
	FloorsWired bool
}

// buildBenchmarkSurface composes the S14.7 practice.
//
// Domain bookkeeping is seeded for the v0 launch roster idempotently at every
// boot (the conformance-registry / watch-row posture: platform obligations need
// no D10 holder). Nothing else is seeded: BENCH-REG §4.5's bring-up batch is an
// operator intent, not a boot step, and no pair is ever manufactured.
func buildBenchmarkSurface(ctx context.Context, db *storage.DB, log *eventlog.Log,
	reg *settings.Registry, runs *run.Store, sched *scheduler.Scheduler,
	ledger *metering.Ledger, evalStore *evals.Store, pipeline *intake.Pipeline,
	reviews *review.Store, logger *slog.Logger) (*benchmarkSurface, error) {

	if err := checkBenchmarkDomainMapping(); err != nil {
		return nil, err
	}
	store := benchmark.NewStore(db, log)
	domains := 0
	for _, d := range benchmark.V0LaunchDomains() {
		if _, err := store.EnsureDomain(ctx, d); err != nil {
			return nil, fmt.Errorf("shell: register S14.7 benchmark domain %q: %w", d, err)
		}
		domains++
	}
	floors := benchmarkFloorSeam(evalStore)
	return &benchmarkSurface{
		Store: store,
		Sampler: &benchmark.Sampler{
			Store: store, Log: log, Settings: reg, Logger: logger,
		},
		Practice: &benchmark.Practice{
			Store: store, Log: log, Runs: runs, Queue: sched, Ledger: ledger,
			Briefs:   benchmarkBriefSeam(pipeline),
			Platform: benchmarkPlatformTextSeam(reviews),
			Direct:   benchmarkDirectTextSeam(),
			Floors:   floors,
			Logger:   logger,
		},
		Domains:     domains,
		FloorsWired: floors != nil,
	}, nil
}

// benchmarkFloorSeam satisfies BENCH-REG §11(d) over the S14.8 surfaces: the
// domain's regression suites green at their REGISTERED per-version floors.
//
// The v0 software content is `rubric-software` v2 (CONVENTIONS §33: no case
// content is invented), so limb (d) for software-development is that rubric's
// floor check plus the sweep's resting verdict. Every honest failure mode
// reports NOT EVALUABLE with its reason — an unregistered domain, an unratified
// or unfloored version, a sweep that has never run — because §10.1(d) says the
// gate "cannot be evaluated — and therefore cannot open" in exactly those cases.
func benchmarkFloorSeam(store *evals.Store) benchmark.SuiteFloors {
	if store == nil {
		return nil
	}
	return func(ctx context.Context, domain string) (benchmark.FloorReport, error) {
		if domain != benchmark.DomainSoftwareDevelopment {
			return benchmark.FloorReport{
				Reason: "domain " + domain + " has no registered regression suites at v0 (BENCH-REG §10.2: web-research suites register at their 8.3 entry)",
			}, nil
		}
		// The (asset, version) pairs the domain's floors are registered against
		// come from the S14.8 seed set itself, so the seam always tracks the
		// asset a floor was measured against rather than a name pinned here.
		seeds := evals.SeedFloors()
		if len(seeds) == 0 {
			return benchmark.FloorReport{
				Reason: "no regression suite is registered for this domain, so falsifiability is not evaluable",
			}, nil
		}
		for _, seed := range seeds {
			f, err := store.Floor(ctx, seed.AssetID, seed.AssetVersion)
			if errors.Is(err, evals.ErrNoFloor) {
				return benchmark.FloorReport{
					Reason: fmt.Sprintf("%s v%d has no registered floor — an unfloored version is not evaluable",
						seed.AssetID, seed.AssetVersion),
				}, nil
			}
			if err != nil {
				return benchmark.FloorReport{}, err
			}
			if !f.Ratified {
				return benchmark.FloorReport{
					Reason: fmt.Sprintf("%s v%d's floor is registered but NOT ratified — an unratified floor is not a standard the gate may lean on",
						seed.AssetID, seed.AssetVersion),
				}, nil
			}
		}
		sweep, err := store.SweepState(ctx)
		if err != nil {
			return benchmark.FloorReport{}, err
		}
		if !sweep.HasRun {
			return benchmark.FloorReport{
				Reason: "the regression-eval sweep has never run, so no suite has a recorded result to be green",
			}, nil
		}
		return benchmark.FloorReport{
			Evaluable: true,
			Green:     sweep.LastResult == "green",
			Reason: fmt.Sprintf("the %s regression sweep last recorded %q on %s, over %d registered floor(s)",
				domain, sweep.LastResult, sweep.LastRun.Format(time.RFC3339), len(seeds)),
		}, nil
	}
}

// benchmarkBriefSeam satisfies BENCH-REG §2's "the same confirmed task
// statement + the same attachments the specification had" from the S06 intake
// ARTIFACT OF RECORD. Both arms answer this identical frozen text (§3.1/§6).
//
// The statement is the CONFIRMED spec markdown of record — never the arriving
// request. The platform arm executed from the confirmed specification, and
// intake's bounded revisions (interview, clarification, critique, coverage) can
// move that text a long way from what the requester first typed. Handing the
// direct arm the raw ask would put the two arms on different questions, and
// every comparison afterwards would be measuring the intake pipeline rather
// than the platform — the confound §3.1 exists to forbid.
//
// The markdown is hash-verified against the artifact ref before it is used: a
// drifted artifact of record fails LOUDLY rather than feeding the direct arm a
// statement the platform arm never saw.
//
// Attachments: at v0 intake carries no attachment channel separate from the
// specification — the requester's supplied inputs (S06.3) are rendered INTO the
// markdown of record — so "identical attachments" holds by construction, and
// what travels here is the artifact REF itself (refs, never bodies, P-T07-5).
// A future separate attachment channel would extend this seam, not the package.
func benchmarkBriefSeam(pipeline *intake.Pipeline) benchmark.BriefSource {
	if pipeline == nil {
		return nil
	}
	return func(ctx context.Context, taskID string) (benchmark.TaskBrief, error) {
		st, err := pipeline.LoadState(ctx, taskID)
		if err != nil {
			return benchmark.TaskBrief{}, err
		}
		return briefFromState(ctx, taskID, st)
	}
}

// briefFromState is the seam's resolution, split from the pipeline lookup so
// the §3.1 obligation it implements is directly testable.
func briefFromState(_ context.Context, taskID string, st *intake.State) (benchmark.TaskBrief, error) {
	if st.SpecRef == nil || st.SpecRef.Path == "" {
		return benchmark.TaskBrief{}, fmt.Errorf(
			"shell: task %q has no confirmed S06 specification of record — the direct arm has no frozen statement to answer (BENCH-REG §2/§3.1)",
			taskID)
	}
	body, err := os.ReadFile(st.SpecRef.Path)
	if err != nil {
		return benchmark.TaskBrief{}, fmt.Errorf(
			"shell: read the S06 artifact of record for %q: %w", taskID, err)
	}
	sum := sha256.Sum256(body)
	got := hex.EncodeToString(sum[:])
	if st.SpecRef.SHA256 != "" && got != st.SpecRef.SHA256 {
		return benchmark.TaskBrief{}, fmt.Errorf(
			"shell: the S06 artifact of record for %q has drifted from its pin (%s vs %s) — refusing to hand the direct arm a statement the platform arm never answered (BENCH-REG §3.1)",
			taskID, got, st.SpecRef.SHA256)
	}
	ref := fmt.Sprintf("s06-artifact-of-record:%s@sha256:%s",
		filepath.Base(st.SpecRef.Path), got)
	return benchmark.TaskBrief{
		TaskID:          taskID,
		Statement:       string(body),
		StatementSource: ref,
		Attachments:     []string{ref},
	}, nil
}

// benchmarkPlatformTextSeam resolves the platform arm: "the accepted final
// artifact set (diff/patch/files) plus its accompanying summary, as accepted at
// review" (BENCH-REG §10.1).
func benchmarkPlatformTextSeam(reviews *review.Store) benchmark.PlatformText {
	if reviews == nil {
		return nil
	}
	return func(ctx context.Context, deliverableID string) (string, error) {
		rev, err := reviews.AcceptedRevision(ctx, deliverableID)
		if err != nil {
			return "", err
		}
		files, err := reviews.RevisionFiles(ctx, deliverableID, rev.N)
		if err != nil {
			return "", err
		}
		if len(files) == 0 {
			return "", fmt.Errorf("shell: accepted revision %d of %q pins objects rather than content, so it has no text artifact set to render blind (BENCH-REG §10.1)",
				rev.N, deliverableID)
		}
		names := make([]string, 0, len(files))
		for name := range files {
			names = append(names, name)
		}
		// Deterministic order: the same accepted revision must render the same
		// way every time, or the template would be a source of variation
		// between the arms rather than a remover of one.
		sort.Strings(names)
		var b strings.Builder
		for i, name := range names {
			if i > 0 {
				b.WriteString("\n\n")
			}
			b.WriteString("## " + name + "\n\n")
			b.WriteString(files[name])
		}
		return b.String(), nil
	}
}

// benchmarkDirectTextSeam resolves the direct arm's produced answer.
//
// It is HONESTLY ABSENT at v0 and says so. The direct arm's run is created and
// admitted by this packet (as ordinary background work under the requester's own
// user_id), but nothing executes it yet: the single-shot engine invocation and
// its output capture belong to the dispatcher, and B6 wires the verdict form a
// pair needs before it can complete anyway. Returning a clear error is the whole
// point — a placeholder body would render a blind pair whose "direct arm" was
// the platform talking to itself, and every statistic downstream would be
// fiction. Real accrual is a bring-up act under BENCH-REG §4.1's own schedule.
func benchmarkDirectTextSeam() benchmark.DirectText {
	return func(_ context.Context, runID string) (string, error) {
		return "", fmt.Errorf("shell: the direct arm of run %q has produced no captured output — "+
			"single-shot execution and output capture land with the dispatcher, and no pair completes "+
			"until they and the B6 verdict form do (BENCH-REG §2; Spec S15.6)", runID)
	}
}

// benchmarkLoopInterval is the sampling loop's TICK — never a cadence. The
// sampler's cursor is durable, so what a tick does is "consume whatever accepted
// deliverables have landed"; the interval only decides how promptly a sampled
// pair appears after an accept. A minute matches the watchlist executor's tick
// (CONVENTIONS §34): brisk enough that a requester sees the card while the task
// is fresh, slow enough to cost nothing. Structural, not ⚙ — S18 ratifies no key.
const benchmarkLoopInterval = time.Minute

// benchmarkLoop drives the BENCH-REG §4 sampling hook. It consumes the accepted
// deliverables past the durable cursor and applies the frozen eligibility
// conjunction plus the uniform draw to each. The cursor lives in the database,
// so a restart neither re-samples an accept nor silently skips one — a skipped
// accept is a LOST DRAW, and §4.1's uniformity is frozen.
func benchmarkLoop(ctx context.Context, bs *benchmarkSurface, logger *slog.Logger) {
	t := time.NewTicker(benchmarkLoopInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pass, err := bs.Sampler.Tail(ctx)
			if err != nil {
				logger.Warn("benchmark: sampling pass failed", "err", err)
				continue
			}
			if pass.Considered > 0 {
				logger.Info("benchmark: sampling pass complete",
					"considered", pass.Considered, "sampled", pass.Sampled,
					"skipped", pass.Skipped, "cursor", pass.Cursor)
			}
		}
	}
}

// checkBenchmarkDomainMapping is the composition-root end of the ONE reading
// this packet takes on the tree's domain vocabulary: BENCH-REG §10.1 registers
// the domain as "software-development" while internal/verify's launch-domain
// constant is "software". internal/benchmark holds and tests the mapping without
// importing verify; this asserts the OTHER side of it at boot, so a rename on
// either side fails loudly rather than silently mislabelling every pair record.
func checkBenchmarkDomainMapping() error {
	registered, ok := benchmark.RegisteredDomain(verify.DomainSoftware)
	if !ok || registered != benchmark.DomainSoftwareDevelopment {
		return fmt.Errorf("shell: verify.DomainSoftware (%q) no longer maps to the BENCH-REG §10.1 registered domain %q — "+
			"every pair record carries the registered name, so this mapping is load-bearing",
			verify.DomainSoftware, benchmark.DomainSoftwareDevelopment)
	}
	return nil
}
