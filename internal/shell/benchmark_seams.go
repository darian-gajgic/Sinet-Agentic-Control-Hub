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
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/stage"
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
	// Runs is the S02.3 run store, read by the dispatch→render DRIVER below to
	// ask one question and nothing else: has this pair's direct-arm run ENDED?
	// The driver never transitions a run — internal/benchmark's no-kill wall and
	// this loop's own posture both hold — it only waits for one.
	Runs *run.Store
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
	if err := checkBenchmarkDirectSuffix(); err != nil {
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
			Direct:   benchmarkDirectTextSeam(store),
			Floors:   floors,
			Logger:   logger,
		},
		Runs:        runs,
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
// B5-7 shipped this as an HONEST ABSENCE — the direct arm's run was created and
// admitted, but nothing executed it, so the seam named the clause it could not
// satisfy. B6-2C makes it REAL: the dispatcher's `.direct` leg (internal/stage)
// runs the single shot and writes its final text against the run (migration
// 0018), and this is the read of that capture.
//
// The absence did not go away, it got NARROWER and it is still honest: a pair
// whose arm has not run — or whose leg never reached its capture point — reads
// back ErrNoDirectCapture and is simply not rendered. A placeholder body would
// render a blind pair whose "direct arm" was the platform talking to itself and
// every statistic downstream would be fiction, so absence stays an error rather
// than becoming an empty string. A CAPTURED empty answer is a different thing
// and reads back as one: the arm answered with nothing, which is a real
// single-shot outcome.
func benchmarkDirectTextSeam(store *benchmark.Store) benchmark.DirectText {
	if store == nil {
		return nil
	}
	return store.CapturedDirectText
}

// directArmSeams composes the two things the S05.3 dispatcher's `.direct` leg
// needs across the §34/§35 seam boundary (B6-2C, OQ9). internal/stage never
// imports internal/benchmark and internal/benchmark never imports internal/stage,
// so the frozen §2 statement and the single-shot capture cross as func seams
// satisfied HERE — the composition root is the one place that may see both.
//
// Both fields are LATE-BOUND, and that is composition order rather than
// looseness: the intake pipeline is built by the skeleton (whose Config carries
// these seams) and the benchmark store is built after the scheduler the practice
// enqueues onto, so neither exists when stage.New is called. The pseams.pipe
// precedent, applied to one more seam pair. An unbound seam refuses LOUDLY — the
// leg never invents a prompt and never silently drops a capture.
type directArmSeams struct {
	pipe  *intake.Pipeline
	store *benchmark.Store
}

// Statement is stage.Config.BenchmarkStatement: the CONFIRMED §2 task statement
// from the S06 artifact of record, hash-verified, never the arriving request
// (the §3.1 obligation briefFromState implements — one resolution, shared with
// the benchmark package's own brief seam so both arms provably answer one text).
func (d *directArmSeams) Statement(ctx context.Context, taskID string) (string, error) {
	if d.pipe == nil {
		return "", errors.New("shell: the intake pipeline is not composed, so the BENCH-REG §2 frozen statement cannot be read")
	}
	st, err := d.pipe.LoadState(ctx, taskID)
	if err != nil {
		return "", err
	}
	brief, err := briefFromState(ctx, taskID, st)
	if err != nil {
		return "", err
	}
	return brief.Statement, nil
}

// Capture is stage.Config.BenchmarkCapture: the single shot's final text,
// persisted against its run. Refs-not-blobs holds by construction — the text
// goes to the pair row migration 0018 opened for it and never into an event
// payload, so ⚙ state.event_payload_cap is respected by never being approached.
func (d *directArmSeams) Capture(ctx context.Context, runID, text string) (bool, error) {
	if d.store == nil {
		return false, errors.New("shell: the S14.7 benchmark store is not composed, so a direct-arm capture has nowhere durable to land")
	}
	return d.store.CaptureDirectText(ctx, runID, text)
}

// checkBenchmarkDirectSuffix is the composition-root end of the second pinned
// reading this area needs (the checkBenchmarkDomainMapping precedent, B6-2C):
// internal/benchmark mints the direct arm's run id and internal/stage routes it
// by suffix, and neither may import the other. If either side renames its half,
// a `.direct` run becomes unroutable and every dispatched pair silently rests
// forever — so the two halves are asserted against each other at boot.
func checkBenchmarkDirectSuffix() error {
	id := benchmark.DirectRunID("bp-1")
	if !strings.HasSuffix(id, stage.RunSuffixDirect) {
		return fmt.Errorf("shell: benchmark.DirectRunID mints %q but the dispatcher routes on %q — "+
			"a direct-arm run that no role matches never runs, and its pair rests forever (BENCH-REG §2)",
			id, stage.RunSuffixDirect)
	}
	return nil
}

// benchmarkLoopInterval is the sampling loop's TICK — never a cadence. The
// sampler's cursor is durable, so what a tick does is "consume whatever accepted
// deliverables have landed"; the interval only decides how promptly a sampled
// pair appears after an accept. A minute matches the watchlist executor's tick
// (CONVENTIONS §34): brisk enough that a requester sees the card while the task
// is fresh, slow enough to cost nothing. Structural, not ⚙ — S18 ratifies no key.
const benchmarkLoopInterval = time.Minute

// benchmarkDriverBatch bounds how many pairs ONE driver pass picks up per state
// (B6-2C). Structural, not ⚙ — S18 ratifies no key here (the §35 tail-batch and
// §7 sseBatchSize precedent, interim under the standing settings-tab directive)
// — with its reason: a pass shares the control plane's single writer connection
// (S02.1) and each dispatch is a run creation plus an enqueue, so the bound is
// what keeps one pass from holding that writer against everything else. It never
// decides WHICH pairs are due: dueness is the stored state, the listing is
// oldest-draw-first, and a pair beyond the bound is the next pass's first item.
const benchmarkDriverBatch = 32

// benchmarkLoop drives the BENCH-REG §4 sampling hook and the §3 dispatch→render
// DRIVER. It consumes the accepted deliverables past the durable cursor and
// applies the frozen eligibility conjunction plus the uniform draw to each, then
// walks the pairs whose STORED state says they are due to advance. The cursor
// lives in the database, so a restart neither re-samples an accept nor silently
// skips one — a skipped accept is a LOST DRAW, and §4.1's uniformity is frozen.
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
			} else if pass.Considered > 0 {
				logger.Info("benchmark: sampling pass complete",
					"considered", pass.Considered, "sampled", pass.Sampled,
					"skipped", pass.Skipped, "cursor", pass.Cursor)
			}
			benchmarkDriverPass(ctx, bs, logger)
		}
	}
}

// benchmarkDriverPass is the BENCH-REG §3 dispatch→render driver (B6-2C, R18):
// production wiring that moves pairs sampled → dispatched → rendered and has NO
// PROTOCOL AUTHORITY OF ITS OWN.
//
// Everything about WHETHER a pair may advance is the package's: DispatchDirectArm
// refuses a pair that is not `sampled`, RenderBlind refuses one that is not
// `dispatched`, and the runs table's primary key refuses a second direct arm for
// a pair that already has one. This pass decides only WHEN to ask, which is the
// shell's standing duty (§31/§34/§35 — the package owns the LOGIC, the shell owns
// WHEN). It advances nothing synthetically: there is no state write here at all.
//
// Dueness is the STORED state, never the tick. The tick decides how promptly a
// verdict card appears after an accept; a pass that finds nothing does nothing,
// and a failure is logged and retried on the NEXT pass by re-reading state —
// which is safe re-entry precisely because the verbs check state themselves.
func benchmarkDriverPass(ctx context.Context, bs *benchmarkSurface, logger *slog.Logger) {
	sampled, err := bs.Store.PairsInState(ctx, benchmark.StateSampled, benchmarkDriverBatch)
	if err != nil {
		logger.Warn("benchmark: driver could not list sampled pairs", "err", err)
	}
	for _, p := range sampled {
		if _, err := bs.Practice.DispatchDirectArm(ctx, p.PairID); err != nil {
			// Logged and left alone. The pair keeps its state, so the next pass
			// re-reads it and tries again; nothing is marked failed, because a
			// pair that could not be dispatched today is still a pair awaiting
			// its arm and the practice records no failure state for one.
			logger.Warn("benchmark: direct-arm dispatch failed; retried on the next pass",
				"pair", p.PairID, "err", err)
			continue
		}
		logger.Info("benchmark: direct arm dispatched (BENCH-REG §2, ordinary background admission)",
			"pair", p.PairID, "owner", p.UserID)
	}

	dispatched, err := bs.Store.PairsInState(ctx, benchmark.StateDispatched, benchmarkDriverBatch)
	if err != nil {
		logger.Warn("benchmark: driver could not list dispatched pairs", "err", err)
		return
	}
	for _, p := range dispatched {
		if !benchmarkArmEnded(ctx, bs, p, logger) {
			continue
		}
		if _, err := bs.Practice.RenderBlind(ctx, p.PairID); err != nil {
			// The commonest reason is the honest one: the arm ended without
			// producing a capture, so there is no direct text to render and the
			// pair correctly waits rather than being rendered against nothing.
			logger.Warn("benchmark: blind render failed; retried on the next pass",
				"pair", p.PairID, "direct_run", p.DirectRunID, "err", err)
			continue
		}
		logger.Info("benchmark: pair rendered blind and is awaiting its requester's verdict (BENCH-REG §3.2)",
			"pair", p.PairID, "owner", p.UserID)
	}
}

// benchmarkArmEnded reports whether a dispatched pair's direct-arm run has
// finished, which is the ONE precondition the driver checks before asking for a
// render.
//
// "Finished" is `run.IsTerminal` — a state that admits no further transitions.
// `crashed` is deliberately NOT that (S02.3/S02.5: recovery supersedes a crash by
// harvest, fork, tombstone or finalize), so a crashed arm waits for the recovery
// ladder's disposition instead of being rendered out from under it. Whatever that
// disposition turns out to be, the pair's §6 parity note at record time reports a
// run that did not end on its own terms — the arm is never re-run (§17).
func benchmarkArmEnded(ctx context.Context, bs *benchmarkSurface, p benchmark.Pair, logger *slog.Logger) bool {
	if p.DirectRunID == "" || bs.Runs == nil {
		return false
	}
	r, err := bs.Runs.Get(ctx, p.DirectRunID)
	if err != nil {
		logger.Warn("benchmark: driver could not read the direct-arm run",
			"pair", p.PairID, "run", p.DirectRunID, "err", err)
		return false
	}
	return run.IsTerminal(r.State)
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
