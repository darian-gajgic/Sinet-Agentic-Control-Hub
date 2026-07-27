package watchlist

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/scheduler"
)

// canary_auth.go — the P-T17-1 auth canary (Spec S14.6 ¶3 bullet 1; S03.6;
// S10.5).
//
// Provider sanction is allowlist-shaped and revocable server-side without
// notice, so a lane can be alive, credentialed and still refused. That failure
// arrives on the wire looking exactly like a budget or depletion event —
// budget-as-HTTP-401, a policy ban on VALID credentials — and misclassifying it
// as depletion (retry-parking a revoked lane) is P-T08-2's named worst case.
//
// The canary therefore does exactly two things: it makes one cheap real request
// per lane on the ⚙ canary.auth_interval cadence, and it normalizes the wire
// answer into a scheduler.LimitSignal — including OnValidCredentials, the field
// the scheduler already documents as "the auth-canary fact (P-T17-1)" — and
// hands it to scheduler.Classify.
//
// This is Classify's FIRST production caller. The classifier is NOT
// re-implemented, forked or bypassed here: misclassification stays in one
// tested component with per-lane fixtures (S10.5), and its Class-4-first
// ordering is what guarantees an auth event can never fall through to a
// retry-park. The canary records the returned Action as DATA and raises a
// flag-now card; it never freezes, parks or kills anything itself (S14.4 / G1
// D1.3) — honouring a frozen lane belongs to the dispatch path.
//
// $0 POSTURE: the real-request leg ships DISARMED. Probe is nil unless the
// operator arms it (CanaryArmEnv), and with no probe the canary is skipped
// rather than faked.

// AuthProbe makes one cheap real request on a lane and normalizes the wire
// answer into the scheduler's raw signal. It classifies nothing — the taxonomy
// is scheduler.Classify's (S10.5).
type AuthProbe func(ctx context.Context, lane string) (scheduler.LimitSignal, error)

// AuthCanary is the scheduled per-lane auth probe.
type AuthCanary struct {
	// Probe is the real-request leg; nil ⇒ DISARMED (the v0 posture).
	Probe AuthProbe
	// Unavailable NAMES why Probe is nil. A leg that cannot run must say why:
	// the sweep counts it as disarmed and carries this reason, so a leg is
	// never silently skipped (drain D1).
	Unavailable string
	// Lanes are the lanes to probe — the paid lanes, whose sanction can be
	// revoked. The local lane holds no provider credential and has no sanction
	// to lose, so it is not probed.
	Lanes []string
}

// NewAuthCanary builds the auth canary over the paid lanes with a live probe.
func NewAuthCanary(probe AuthProbe) *AuthCanary {
	return &AuthCanary{Probe: probe, Lanes: PaidLanes()}
}

// DisarmedAuthCanary builds the auth canary with NO probe and a named reason.
// The legs still exist and are still scheduled, so every sweep accounts for
// them out loud instead of skipping a nil (drain D1).
func DisarmedAuthCanary(reason string) *AuthCanary {
	return &AuthCanary{Lanes: PaidLanes(), Unavailable: reason}
}

func (a *AuthCanary) runner(c *Canaries, lane string) func(context.Context) (CanaryResult, error) {
	return func(ctx context.Context) (CanaryResult, error) { return a.Run(ctx, c, lane) }
}

// Run probes one lane and classifies the answer.
func (a *AuthCanary) Run(ctx context.Context, c *Canaries, lane string) (CanaryResult, error) {
	if a.Probe == nil {
		return CanaryResult{}, disarmedBecause(firstNonEmpty(a.Unavailable, DisarmedReasonNotArmed))
	}
	cfg, err := scheduler.LoadLimitConfig(c.Settings)
	if err != nil {
		return CanaryResult{}, err
	}

	sig, perr := a.Probe(ctx, lane)
	if perr != nil {
		// A probe that did not complete proves nothing about sanction. It is
		// an honest failure routed to the digest (ClassUnclear is the S14.2
		// abstain member), never a fabricated policy revocation.
		return CanaryResult{
			Lane:        lane,
			Summary:     fmt.Sprintf("auth canary could not reach lane %s", lane),
			Detail:      perr.Error(),
			ChangeClass: ClassUnclear,
			Err:         perr.Error(),
		}, nil
	}
	sig.Lane = lane

	act := scheduler.Classify(sig, cfg)
	res := CanaryResult{
		Lane:       lane,
		Action:     string(act.Kind),
		LimitClass: int(act.Class),
		Detail:     act.Reason,
	}
	if act.Class == scheduler.ClassAuthPolicy {
		// policy-revocation-suspected (S03.6): lane freeze + flag-now, NEVER an
		// infinite retry-park. The change class is `terms` — a sanction
		// withdrawn is a terms change — which on an ACTIVE lane routes to
		// flag-now through the same server-side S14.4 rule every drift card
		// uses.
		res.Summary = fmt.Sprintf("auth canary on lane %s classified policy-revocation-suspected — %s", lane, act.Kind)
		res.ChangeClass = ClassTerms
		return res, nil
	}
	// Every other class means the lane answered on good credentials: sanction
	// stands. A depletion or a transient shed is a LIMIT event, handled by the
	// dispatch path on a real call — it is not an auth finding and raises no
	// card.
	res.Pass = true
	res.Summary = fmt.Sprintf("auth canary on lane %s passed (%s)", lane, act.Kind)
	return res, nil
}

// ---- the real-request leg (disarmed at v0) ----

// authProbeTimeout bounds one canary request. STRUCTURAL, not ⚙: a canary that
// hangs is a canary that stops reporting, and S18 ratifies no probe-timeout
// key. Ten seconds is generous for a credential check and short enough that a
// hung lane is reported on its own cadence.
const authProbeTimeout = 10 * time.Second

// authProbeBodyCap bounds how much of an error body is read for the policy-ban
// text match. STRUCTURAL, not ⚙: policy-ban text arrives in the first bytes of
// a response, and an unbounded read of a hostile body is the thing to avoid.
const authProbeBodyCap = 8 << 10

// HTTPAuthProbe is the real-request leg: one bounded GET against a lane's
// credentialed endpoint, normalized into the scheduler's raw signal.
//
// credential returns the header VALUE at call time — it is never stored on this
// struct, never logged, never marshaled, and never placed on anything that
// reaches a park record or the DB (D2/S11.5). The response body is read only up
// to authProbeBodyCap and only to carry policy-ban TEXT into BodyText, which is
// the one thing S10.5 needs from it.
//
// OnValidCredentials is set true because the canary probes with the lane's
// KNOWN-GOOD stored credential: that is precisely what makes a 403 here mean
// policy revocation rather than a stale key (P-T17-1).
func HTTPAuthProbe(client *http.Client, endpoint, header string, credential func() string) AuthProbe {
	if client == nil {
		client = &http.Client{Timeout: authProbeTimeout}
	}
	return func(ctx context.Context, lane string) (scheduler.LimitSignal, error) {
		ctx, cancel := context.WithTimeout(ctx, authProbeTimeout)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return scheduler.LimitSignal{}, fmt.Errorf("watchlist: auth canary request for lane %s: %w", lane, err)
		}
		req.Header.Set("User-Agent", UserAgent)
		if header != "" && credential != nil {
			req.Header.Set(header, credential())
		}
		resp, err := client.Do(req)
		if err != nil {
			return scheduler.LimitSignal{}, fmt.Errorf("watchlist: auth canary probe for lane %s: %w", lane, err)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(io.LimitReader(resp.Body, authProbeBodyCap))
		return scheduler.LimitSignal{
			Lane:               lane,
			HTTPStatus:         resp.StatusCode,
			BodyText:           strings.TrimSpace(string(body)),
			OnValidCredentials: true,
		}, nil
	}
}
