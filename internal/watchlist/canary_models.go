package watchlist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// canary_models.go — model-list drift (Spec S14.6 ¶3 bullet 5; S03.6; P-T17-3).
//
// S03.6 binds the shape: the adapter "periodically diffs each account's
// observed model list against config", and routing and 2.7 gap advice consume
// the observed list WITH `verified-on` dates. The list is region- and
// account-dependent, so a model that exists for one account simply is not there
// for another — and a routing decision made against a config the account cannot
// serve fails at call time, not at plan time.
//
// This is the canary whose findings can carry a MODEL ID. Change class `models`
// plus a model-id subject is exactly the pair the S14.8 revalidation runbook's
// only wired hook (worker.Store.FlagByModel) can act on (OQ4(a)), so a model
// leaving or entering an account's list flags the worker versions pinned to it.
// The executor half's own sources — feeds, page diffs, structural key diffs —
// name no model id, which is why this layer is the first real producer of one.
//
// $0 POSTURE: reading an account's model list is a real request, so the leg
// ships DISARMED behind the operator arm exactly as the auth canary does.

// ModelListProbe reads one lane account's OBSERVED model list.
type ModelListProbe func(ctx context.Context, lane string) ([]string, error)

// ModelListCanary is the scheduled per-account observed-vs-config diff.
type ModelListCanary struct {
	// Probe is the real-request leg; nil ⇒ DISARMED (the v0 posture).
	Probe ModelListProbe
	// Unavailable NAMES why Probe is nil (drain D1).
	Unavailable string
	// Lanes are the accounts to diff.
	Lanes []string
	// Configured is the per-lane CONFIGURED model list — the S03.6 "config"
	// side of the diff.
	//
	// CORRECTED 2026-08-24 (P3-LN-2B drain r1 D8): this said it was empty at
	// v0 because "no per-account model configuration is registered anywhere",
	// which stopped being true when lanes gained commissioning DOCUMENTS. The
	// composition root now fills it from those documents, which carry each
	// model id with its own verified-on date. It is still empty for a lane
	// with no document.
	//
	// With no config the comparison falls back to the previous OBSERVATION,
	// which is still model-list drift and still names real model ids — never
	// to pretending an empty config means every model was removed.
	Configured map[string][]string
}

// NewModelListCanary builds the model-list canary over the paid lanes with a
// live probe.
func NewModelListCanary(probe ModelListProbe, configured map[string][]string) *ModelListCanary {
	return &ModelListCanary{Probe: probe, Lanes: PaidLanes(), Configured: configured}
}

// DisarmedModelListCanary builds the model-list canary with NO probe and a
// named reason, so the sweep accounts for it (drain D1).
func DisarmedModelListCanary(reason string) *ModelListCanary {
	return &ModelListCanary{Lanes: PaidLanes(), Unavailable: reason}
}

func (m *ModelListCanary) runner(c *Canaries, lane string) func(context.Context) (CanaryResult, error) {
	return func(ctx context.Context) (CanaryResult, error) { return m.Run(ctx, c, lane) }
}

// Run diffs one account's observed model list.
func (m *ModelListCanary) Run(ctx context.Context, c *Canaries, lane string) (CanaryResult, error) {
	if m.Probe == nil {
		return CanaryResult{}, disarmedBecause(firstNonEmpty(m.Unavailable, DisarmedReasonNotArmed))
	}
	observed, err := m.Probe(ctx, lane)
	if errors.Is(err, ErrModelListUnavailable) {
		// The endpoint serves no model list at all. That is not drift, not a
		// probe failure, and emphatically not an empty catalogue: it is a
		// capability this lane's endpoint does not have, and the honest report
		// is to say so, dated, and diff nothing.
		//
		// The Z.AI coding endpoint is the live case (checked 2026-08-23: no
		// documentation of /models on the subscription endpoint). Failing the
		// canary would raise a card every day for a condition nobody can fix;
		// inventing an empty list would report every configured model as
		// REMOVED, which is the fabricated-drift the canary exists to catch.
		return CanaryResult{
			Lane:    lane,
			Pass:    true,
			Summary: fmt.Sprintf("model-list canary on lane %s: the observed list is unavailable — this endpoint serves none", lane),
			Detail: fmt.Sprintf("%v\nNo diff was possible, so none is reported. The configured list stands as the only "+
				"record of this lane's models until the endpoint serves one (S03.6; P-T17-3).", err),
		}, nil
	}
	if err != nil {
		return CanaryResult{
			Lane:        lane,
			Summary:     fmt.Sprintf("model-list canary could not read lane %s", lane),
			Detail:      err.Error(),
			ChangeClass: ClassUnclear,
			Err:         err.Error(),
		}, nil
	}
	observed = normalizeModelIDs(observed)

	res := CanaryResult{Lane: lane, Observed: observed}

	baseline, against := m.Configured[lane], "config"
	if len(baseline) == 0 {
		against = "the previous observation"
		prev, had, err := c.LastObserved(ctx, CanaryModelList, lane)
		if err != nil {
			return res, err
		}
		if !had {
			res.Pass, res.Baseline = true, true
			res.Summary = fmt.Sprintf("model-list canary baseline established on lane %s: %d models observed", lane, len(observed))
			res.Detail = "no configured model list is registered at v0, so the first observation is the baseline (S03.6)"
			return res, nil
		}
		if prev.Truncated {
			// The recorded baseline is a bounded PREFIX, so naming ids from it
			// would invent removals for every id the cap dropped. Compare the
			// full-list digests instead: drift stays detectable, the ids just
			// cannot be named from a truncated record (drain D6).
			return m.compareByDigest(res, lane, prev, observed), nil
		}
		baseline = prev.List
	}
	baseline = normalizeModelIDs(baseline)

	added, removed := diffModelIDs(baseline, observed)
	res.Delta = float64(len(added) + len(removed))
	if len(added) == 0 && len(removed) == 0 {
		res.Pass = true
		res.Summary = fmt.Sprintf("model-list canary on lane %s passed: %d models, matching %s", lane, len(observed), against)
		return res, nil
	}

	res.ChangeClass = ClassModels
	res.Subjects = append(append([]string{}, removed...), added...)
	res.Summary = fmt.Sprintf("model-list drift on lane %s: %d removed, %d added vs %s",
		lane, len(removed), len(added), against)
	res.Detail = fmt.Sprintf("removed: %s\nadded: %s\nobserved (%d): %s",
		join(removed), join(added), len(observed), join(observed))
	return res, nil
}

// compareByDigest is the honest degradation for a catalogue too large to
// record: "something moved and I cannot name it" — never "nothing moved".
func (m *ModelListCanary) compareByDigest(res CanaryResult, lane string, prev Observation, observed []string) CanaryResult {
	_, _, digest := boundObserved(observed)
	if prev.Digest != "" && prev.Digest == digest {
		res.Pass = true
		res.Summary = fmt.Sprintf("model-list canary on lane %s passed: %d models, unchanged (compared by digest — the catalogue exceeds the recorded-list cap)",
			lane, len(observed))
		return res
	}
	res.Delta = 1
	res.ChangeClass = ClassModels
	res.Summary = fmt.Sprintf("model-list drift on lane %s: the catalogue changed (%d models)", lane, len(observed))
	res.Detail = fmt.Sprintf("the previous observation exceeded the %d-id recording cap, so this diff ran on the full-list DIGEST and no model id can be named from it; "+
		"no revalidation subject is claimed rather than a guessed one", observedListCap)
	return res
}

func join(ids []string) string {
	if len(ids) == 0 {
		return "(none)"
	}
	return strings.Join(ids, ", ")
}

// normalizeModelIDs trims, drops empties and sorts, so a provider reordering
// its list is not mistaken for drift.
func normalizeModelIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// diffModelIDs returns what the observed list added and removed relative to the
// baseline. Both inputs are normalized.
func diffModelIDs(baseline, observed []string) (added, removed []string) {
	have := make(map[string]bool, len(baseline))
	for _, id := range baseline {
		have[id] = true
	}
	for _, id := range observed {
		if !have[id] {
			added = append(added, id)
		}
	}
	now := make(map[string]bool, len(observed))
	for _, id := range observed {
		now[id] = true
	}
	for _, id := range baseline {
		if !now[id] {
			removed = append(removed, id)
		}
	}
	return added, removed
}

// ErrModelListUnavailable reports an endpoint that serves no model list at
// all — not a list that failed to arrive.
//
// The distinction is load-bearing. A subscription endpoint may simply not
// expose `/models` (the Z.AI coding endpoint is undocumented for it as of
// 2026-08-23), and a canary that treated that as a failure would raise a card
// every cycle for a condition nobody can act on, while one that treated it as
// an empty catalogue would report every configured model as removed. Neither
// is true, so it gets its own answer.
var ErrModelListUnavailable = errors.New("watchlist: the lane endpoint serves no model list")

// ---- the real-request leg (disarmed at v0) ----

// modelListBodyCap bounds the model-list response read. STRUCTURAL, not ⚙: a
// provider catalogue is a small document and an unbounded read of a remote body
// is the thing to avoid.
const modelListBodyCap = 1 << 20

// HTTPModelListProbe is the real-request leg: one bounded GET against an
// OpenAI-compatible `/models` surface, returning the `data[].id` list.
//
// credential returns the header value at call time and is never stored, logged
// or marshaled (D2/S11.5). A non-2xx answer is an error, not an empty list —
// reporting "no models" for a 403 would raise a removal finding for every model
// the account has.
func HTTPModelListProbe(client *http.Client, endpoint, header string, credential func() string) ModelListProbe {
	if client == nil {
		client = &http.Client{Timeout: authProbeTimeout}
	}
	return func(ctx context.Context, lane string) ([]string, error) {
		ctx, cancel := context.WithTimeout(ctx, authProbeTimeout)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("watchlist: model-list request for lane %s: %w", lane, err)
		}
		req.Header.Set("User-Agent", UserAgent)
		if header != "" && credential != nil {
			req.Header.Set(header, credential())
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("watchlist: model-list probe for lane %s: %w", lane, err)
		}
		defer resp.Body.Close()
		// 404 means the endpoint does not SERVE a model list, which is a
		// different fact from a list that could not be read, and the two must
		// not share a verdict: one is a permanent property of the endpoint,
		// the other is an incident. Every other non-2xx stays an error.
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("%w: lane %s answered HTTP 404 at %s — this endpoint serves no model list",
				ErrModelListUnavailable, lane, endpoint)
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return nil, fmt.Errorf("watchlist: model-list probe for lane %s: HTTP %d", lane, resp.StatusCode)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, modelListBodyCap))
		if err != nil {
			return nil, fmt.Errorf("watchlist: model-list probe for lane %s: %w", lane, err)
		}
		var doc struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &doc); err != nil {
			return nil, fmt.Errorf("watchlist: model-list probe for lane %s: parse: %w", lane, err)
		}
		ids := make([]string, 0, len(doc.Data))
		for _, d := range doc.Data {
			ids = append(ids, d.ID)
		}
		return ids, nil
	}
}
