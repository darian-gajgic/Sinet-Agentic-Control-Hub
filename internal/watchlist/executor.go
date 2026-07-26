package watchlist

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// executor.go — the S14.6 executor proper: one pass over the DUE watch rows.
//
// The package owns the LOGIC; the SHELL owns WHEN (one goroutine, the
// walTruncate / watchdog-loop precedent). Dueness derives from stored state, so
// a pass resumes correctly across a restart and a wall-clock jump — no ticker
// drives cadence.
//
// Tier independence is structural: the page tier reaches changedetection.io,
// and the feed and API tiers reach the open web. An absent organ degrades the
// page tier alone; the other tiers keep working (R8).

// Executor runs the watchlist.
type Executor struct {
	Store    *Store
	Fetcher  *Fetcher
	Emitter  *Emitter
	Settings Settings
	// CDIO is the page-tier organ driver; nil ⇒ the page tier is absent.
	CDIO   *CDIO
	Logger *slog.Logger
	// Now is the test clock seam; nil ⇒ time.Now.
	Now func() time.Time
}

// Deps are the wired dependencies of a live executor.
type Deps struct {
	Store      *Store
	Emitter    *Emitter
	Settings   Settings
	CDIO       *CDIO
	HTTPClient *http.Client
	Logger     *slog.Logger
}

// New wires an executor.
func New(d Deps) *Executor {
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Executor{
		Store: d.Store, Fetcher: NewFetcher(d.HTTPClient), Emitter: d.Emitter,
		Settings: d.Settings, CDIO: d.CDIO, Logger: logger,
	}
}

func (x *Executor) now() time.Time {
	if x.Now != nil {
		return x.Now()
	}
	return time.Now()
}

// Pass is the outcome of one executor pass, for the shell's log line.
type Pass struct {
	Polled     int
	Hits       int
	Failures   int
	Reconciled ReconcileReport
	// PageTierAbsent records that the organ was unreachable this pass.
	PageTierAbsent bool
}

// RunDue performs one pass: reconcile the page tier onto the organ, then poll
// every due row. A failure on one row never aborts the pass — a dead source is
// recorded and announced, not allowed to silence the rest of the watchlist.
func (x *Executor) RunDue(ctx context.Context) (Pass, error) {
	var p Pass

	if x.CDIO != nil {
		rows, err := x.Store.PageRows(ctx)
		if err != nil {
			return p, err
		}
		rep, err := x.CDIO.Reconcile(ctx, x.Store, rows)
		if err != nil {
			// The organ being down is the honest "page tier absent" state; the
			// watchdog organ-absence check is what surfaces it as a flag.
			p.PageTierAbsent = true
			x.Logger.Warn("watchlist: page tier absent (changedetection.io unreachable); feed and API tiers continue", "err", err)
		} else {
			p.Reconciled = rep
		}
	}

	due, err := x.Store.Due(ctx)
	if err != nil {
		return p, err
	}
	for _, r := range due {
		p.Polled++
		hits, err := x.poll(ctx, r)
		if err != nil {
			p.Failures++
			if derr := x.noteFailure(ctx, r, err); derr != nil {
				x.Logger.Error("watchlist: recording a fetch failure failed", "row", r.ID, "err", derr)
			}
			continue
		}
		p.Hits += hits
	}
	return p, nil
}

// poll runs one row's fetch and emits its findings, returning the hit count.
func (x *Executor) poll(ctx context.Context, r Row) (int, error) {
	switch r.Kind {
	case KindFeed:
		return x.pollFeed(ctx, r)
	case KindAPI:
		if isPriceRow(r) {
			return x.pollPrices(ctx, r)
		}
		return x.pollAPI(ctx, r)
	case KindPage:
		return x.pollPage(ctx, r)
	}
	return 0, fmt.Errorf("watchlist: row %q kind %q is not pollable", r.ID, r.Kind)
}

// pollFeed polls an Atom/RSS row: conditional GET, whole-body hash dedup, then
// per-ENTRY hash dedup so a re-published item is not raised twice.
func (x *Executor) pollFeed(ctx context.Context, r Row) (int, error) {
	resp, err := x.Fetcher.Get(ctx, r.URL, r.FetchState)
	if err != nil {
		return 0, err
	}
	st := r.FetchState
	st.LastFetchAt = x.now()
	st.ETag, st.LastModified, st.ContentHash = resp.ETag, resp.LastModified, resp.ContentHash

	if resp.NotModified || resp.Unchanged {
		return 0, x.Store.RecordSuccess(ctx, r.ID, st)
	}

	_, entries, err := ParseFeed(resp.Body, r.ParserHint)
	if err != nil {
		// A malformed feed is a FETCH FAILURE — never a panic, never fatal.
		return 0, err
	}

	hits := 0
	var fresh []string
	for _, e := range entries {
		h := e.Hash()
		if r.seen(h) {
			continue
		}
		fresh = append(fresh, h)
		if _, err := x.Emitter.Emit(ctx, Hit{
			RowID: r.ID, Kind: r.Kind, Source: r.URL, Lane: r.Lane,
			Title:  e.Title,
			Detail: feedDetail(e),
			// A feed entry names no model id, so it can carry no model
			// subject into the S14.8 runbook — the finding records that.
			MigrateBias: r.MigrateBias,
		}); err != nil {
			return hits, err
		}
		hits++
	}
	st.SeenEntries = r.withSeen(fresh...)
	return hits, x.Store.RecordSuccess(ctx, r.ID, st)
}

func feedDetail(e Entry) string {
	return fmt.Sprintf("title: %s\nlink: %s\npublished: %s\n\n%s", e.Title, e.Link, e.Published, e.Summary)
}

// pollAPI polls a T3 aggregator row and raises a STRUCTURAL diff — added,
// removed and changed leaf keys — never free text.
func (x *Executor) pollAPI(ctx context.Context, r Row) (int, error) {
	resp, err := x.Fetcher.Get(ctx, r.URL, r.FetchState)
	if err != nil {
		return 0, err
	}
	st := r.FetchState
	st.LastFetchAt = x.now()
	st.ETag, st.LastModified, st.ContentHash = resp.ETag, resp.LastModified, resp.ContentHash

	if resp.NotModified || resp.Unchanged {
		return 0, x.Store.RecordSuccess(ctx, r.ID, st)
	}

	baseline, ok := StructuralBaseline(resp.Body)
	if ok {
		st.Baseline = baseline
	} else {
		st.Baseline = ""
	}
	// First observation establishes the baseline without raising a card: there
	// was no prior state to have drifted from.
	if r.Baseline == "" && !r.HasFetched {
		return 0, x.Store.RecordSuccess(ctx, r.ID, st)
	}

	diff := DiffStructural(r.Baseline, resp.Body)
	if diff.Empty() {
		return 0, x.Store.RecordSuccess(ctx, r.ID, st)
	}
	if _, err := x.Emitter.Emit(ctx, Hit{
		RowID: r.ID, Kind: r.Kind, Source: r.URL, Lane: r.Lane,
		Title:  diff.Summary(),
		Detail: diff.Detail(),
		// No Subject: a structural key diff names paths, not model ids, and a
		// guessed subject would flag the wrong assets. The producer that CAN
		// carry a model-id subject into the S14.8 edge is the model-list canary
		// of the sibling API-canary layer.
		MigrateBias: r.MigrateBias,
	}); err != nil {
		return 0, err
	}
	return 1, x.Store.RecordSuccess(ctx, r.ID, st)
}

// pollPrices runs the genai-prices refresh. It ALWAYS lands as a proposal and
// never writes a price table (G2 Def.16; S10.3).
func (x *Executor) pollPrices(ctx context.Context, r Row) (int, error) {
	resp, err := x.Fetcher.Get(ctx, r.URL, r.FetchState)
	if err != nil {
		return 0, err
	}
	now := x.now()
	st := r.FetchState
	st.LastFetchAt = now
	st.ETag, st.LastModified, st.ContentHash = resp.ETag, resp.LastModified, resp.ContentHash

	if resp.NotModified || resp.Unchanged {
		if err := x.Store.RecordSuccess(ctx, r.ID, st); err != nil {
			return 0, err
		}
		return x.checkPriceStaleness(ctx, r, now)
	}

	prop, err := BuildPriceProposal(resp.Body, now)
	if err != nil {
		return 0, err
	}
	if _, err := x.Emitter.Emit(ctx, Hit{
		RowID: r.ID, Kind: r.Kind, Source: r.URL, Lane: r.Lane,
		Class:    ClassPrice,
		Summary:  priceProposalSummary(prop),
		Detail:   priceProposalDetail(prop),
		Proposal: prop,
	}); err != nil {
		return 0, err
	}
	return 1, x.Store.RecordSuccess(ctx, r.ID, st)
}

// checkPriceStaleness raises the S16.8 genai-prices staleness proposal when
// upstream has not moved past ⚙ adoption.price_data_stale_days. The clock runs
// from the row's last OBSERVED change (derived from the log — no side store),
// falling back to the vendored file's pin date.
func (x *Executor) checkPriceStaleness(ctx context.Context, r Row, now time.Time) (int, error) {
	days, err := x.Settings.Int(keyPriceDataStaleDays)
	if err != nil {
		return 0, fmt.Errorf("watchlist: read ⚙ %s: %w", keyPriceDataStaleDays, err)
	}
	last, ok, err := x.Emitter.LastFindingAt(ctx, r.ID)
	if err != nil {
		return 0, err
	}
	if !ok {
		last = vendoredBaselineDate()
	}
	if last.IsZero() {
		return 0, nil
	}
	since := now.Sub(last)
	if since < time.Duration(days)*24*time.Hour {
		return 0, nil
	}
	prop := &PriceProposal{
		BaselinePin: VendoredPricePin, BaselineSHA256: VendoredPriceSHA256,
		UpstreamSHA256: r.ContentHash, Diff: "no upstream change observed",
		Stale: stalenessNote(days, since),
	}
	if _, err := x.Emitter.Emit(ctx, Hit{
		RowID: r.ID, Kind: r.Kind, Source: r.URL, Lane: r.Lane,
		Class:    ClassPrice,
		Summary:  priceProposalSummary(prop),
		Detail:   priceProposalDetail(prop),
		Proposal: prop,
	}); err != nil {
		return 0, err
	}
	return 1, nil
}

// pollPage reads the page tier through the organ (poll-first, OQ2(b)): the
// watch list carries each watch's last_changed, and a watch that changed since
// this row's last poll has its rendered diff read from
// /difference/previous/latest.
func (x *Executor) pollPage(ctx context.Context, r Row) (int, error) {
	if x.CDIO == nil {
		return 0, fmt.Errorf("%w: page row %q cannot be polled", ErrOrganAbsent, r.ID)
	}
	if r.ExternalRef == "" {
		return 0, fmt.Errorf("watchlist: page row %q has no reconciled organ watch yet", r.ID)
	}
	remote, err := x.CDIO.ListWatches(ctx)
	if err != nil {
		return 0, err
	}
	w, ok := remote[r.ExternalRef]
	if !ok {
		return 0, fmt.Errorf("watchlist: organ watch %q for row %q is gone", r.ExternalRef, r.ID)
	}
	st := r.FetchState
	st.LastFetchAt = x.now()

	changed := time.Unix(w.LastChanged, 0).UTC()
	if w.LastChanged == 0 || (r.HasFetched && !changed.After(r.LastFetchAt)) {
		return 0, x.Store.RecordSuccess(ctx, r.ID, st)
	}

	diff, err := x.CDIO.Difference(ctx, r.ExternalRef, "previous", "latest")
	if err != nil {
		return 0, err
	}
	hash := contentHash([]byte(diff))
	if hash == r.ContentHash {
		return 0, x.Store.RecordSuccess(ctx, r.ID, st)
	}
	st.ContentHash = hash
	if _, err := x.Emitter.Emit(ctx, Hit{
		RowID: r.ID, Kind: r.Kind, Source: r.URL, Lane: r.Lane,
		Title:       firstNonEmpty(w.Title, w.PageTitle, r.ID),
		Detail:      diff,
		DecayStage:  r.DecayStage,
		MigrateBias: r.MigrateBias,
	}); err != nil {
		return 0, err
	}
	return 1, x.Store.RecordSuccess(ctx, r.ID, st)
}

// OrganLiveness probes the adopted page-diff organ for the watchdog's
// organ-absence check (S14.4 registered check 3). An unconfigured or
// unreachable organ is DOWN, which surfaces as
// `watchdog.organ_absence:watchlist`; an installed-vs-pin delta is reported
// LOUDLY in the note and never silently retargeted (CONVENTIONS §10).
func (x *Executor) OrganLiveness(ctx context.Context) (up bool, note string) {
	if x.CDIO == nil {
		return false, "changedetection.io is not configured (" + CDIOURLEnv + " unset); the page tier is absent and the feed/API tiers continue"
	}
	info, err := x.CDIO.SystemInfo(ctx)
	if err != nil {
		return false, "changedetection.io systeminfo probe failed: " + err.Error()
	}
	if info.Version != Pin {
		return true, fmt.Sprintf("PIN DELTA: changedetection.io reports %q, components.lock pins %q — reconciling is the operator's deliberate-bump decision, never a silent retarget", info.Version, Pin)
	}
	return true, fmt.Sprintf("changedetection.io %s, %d watches", info.Version, info.WatchCount)
}
