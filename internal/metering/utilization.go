package metering

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
)

// The Anthropic unified-header utilization overlay (Spec S10.4). On the
// subscription wire the model-egress request returns the
// `anthropic-ratelimit-unified-*` family (16 headers: 5h / 7d / 7d_oi ×
// reset/utilization/status, plus overage/fallback fields) — the classic
// per-request/token buckets are ABSENT on this surface [SPIKE P2-S3 §4;
// R09-OQ6 confirmed]. These are provider-SIGNALED facts (the provider telling
// us, not us modeling), harvested at the credential-injection proxy
// [XREF:S11], which lands at B1-3. This package owns the reading/recording and
// the trust gate; the header SOURCE (the proxy) is the B1-3 seam.
//
// TRUST GATE (TBD-BRINGUP): P2-S3 confirmed the header NAMES; the value scale
// and the 7d_oi (weekly-Opus-input) sub-limit behavior need one bring-up
// observation (S19.6) before the overlay is trusted for park timing. So the
// overlay is RECORDED honestly (enriching the fleet/consumption meters, an
// observed overlay) but NEVER drives park timing until the observation window
// closes. Class-2 resume times come from the STREAM signal
// (rate_limit_event.resetsAt), not from these headers (S10.5). The pressure
// denominator stays the operator budget regardless — utilization is overlay
// and cross-check, never the denominator (D4).

// unifiedPrefix is the header-family prefix (canonical MIME header casing).
const unifiedPrefix = "Anthropic-Ratelimit-Unified-"

// EventUtilization is the provisional observed-overlay event type for a
// recorded utilization reading (platform-scope; naming pending the S14 event
// contract at B5, matching the B0-3/B0-4 precedent).
const EventUtilization = "meter.utilization"

const utilizationEventSchemaVersion = 1

// Window is one rate-limit window's observed state (Spec S10.4). Reset is the
// PARSED reset instant but is UNTRUSTED for park timing until the bring-up
// observation (TrustParkTiming); ResetRaw + Utilization keep the verbatim
// values because their scale is TBD-BRINGUP.
type Window struct {
	Name        string    `json:"name"` // "5h", "7d", "7d_oi"
	Reset       time.Time `json:"reset,omitempty"`
	ResetRaw    string    `json:"reset_raw,omitempty"`
	Utilization string    `json:"utilization,omitempty"`
	Status      string    `json:"status,omitempty"`
}

// Utilization is the parsed unified-header overlay for one model-egress
// response (Spec S10.4). Raw keeps every unified-* header verbatim so nothing
// is lost while the value scale is still TBD-BRINGUP.
type Utilization struct {
	Windows        map[string]Window `json:"windows,omitempty"`
	OverageStatus  string            `json:"overage_status,omitempty"`
	FallbackStatus string            `json:"fallback_status,omitempty"`
	Raw            map[string]string `json:"raw,omitempty"`
}

// knownWindows is the ratified 3-window set (S10.4). 7d_oi is the weekly-Opus-
// input sub-limit whose semantics are TBD-BRINGUP.
var knownWindows = []string{"5h", "7d", "7d_oi"}

// ParseUnifiedHeaders reads the `anthropic-ratelimit-unified-*` family from an
// HTTP response (Spec S10.4). It is forward-tolerant: every unified-* header is
// captured into Raw, and the known {window × field} shapes are extracted where
// present. The bool reports whether ANY unified header was seen (absent on the
// non-subscription surface, and absent entirely until the proxy lands, B1-3).
func ParseUnifiedHeaders(h http.Header) (Utilization, bool) {
	u := Utilization{Windows: map[string]Window{}, Raw: map[string]string{}}
	seen := false
	for name, vals := range h {
		canon := http.CanonicalHeaderKey(name)
		if !strings.HasPrefix(canon, unifiedPrefix) {
			continue
		}
		seen = true
		if len(vals) == 0 {
			continue
		}
		val := vals[0]
		u.Raw[canon] = val
		suffix := strings.ToLower(strings.TrimPrefix(canon, unifiedPrefix))
		switch suffix {
		case "overage-status":
			u.OverageStatus = val
			continue
		case "fallback-status", "fallback":
			u.FallbackStatus = val
			continue
		}
		// window fields: <window>-<field>, longest-window-name match first so
		// "7d-oi" / "7d_oi" is not shadowed by "7d".
		for _, win := range windowsByLenDesc() {
			for _, alias := range windowAliases(win) {
				if field, ok := trimWindow(suffix, alias); ok {
					w := u.Windows[win]
					w.Name = win
					switch field {
					case "reset":
						w.ResetRaw = val
						if t, ok := parseResetUntrusted(val); ok {
							w.Reset = t
						}
					case "utilization":
						w.Utilization = val
					case "status":
						w.Status = val
					}
					u.Windows[win] = w
					goto next
				}
			}
		}
	next:
	}
	if !seen {
		return Utilization{}, false
	}
	if len(u.Windows) == 0 {
		u.Windows = nil
	}
	return u, true
}

// TrustParkTiming reports whether the overlay may drive park timing. It is
// ALWAYS false at B1-2: the value scale + 7d_oi semantics are TBD-BRINGUP and
// must be observed once before the overlay is trusted (Spec S10.4). Park times
// come from the stream signal until this returns true (S10.5).
func (Utilization) TrustParkTiming() bool { return false }

// ResetFor returns a window's parsed reset instant, but ONLY the caller that
// has already cleared TrustParkTiming should act on it. It exists so the
// bring-up observation can compare the header reset against the stream signal.
func (u Utilization) ResetFor(window string) (time.Time, bool) {
	w, ok := u.Windows[window]
	if !ok || w.Reset.IsZero() {
		return time.Time{}, false
	}
	return w.Reset, true
}

// Record appends the overlay as a platform-scope observed-overlay event (Spec
// S10.4: enrich the fleet/consumption meters). It is attributed to the
// credential owner. run_id is NULL — the utilization is a lane/subscription-
// level fact, not tied to one run's generation. This is the recording path the
// proxy (B1-3) drives; it is trust-gated for park timing regardless.
func (u Utilization) Record(ctx context.Context, log *eventlog.Log, ownerUserID string) error {
	if ownerUserID == "" {
		return fmt.Errorf("metering: utilization record without an owner (15.6)")
	}
	payload, err := json.Marshal(struct {
		Utilization
		Trusted bool   `json:"trusted_for_park_timing"`
		Note    string `json:"note"`
	}{u, u.TrustParkTiming(), "observed overlay; park timing gated on the S19.6 bring-up observation (TBD-BRINGUP)"})
	if err != nil {
		return fmt.Errorf("metering: marshal utilization overlay: %w", err)
	}
	_, err = log.Append(ctx, eventlog.Append{
		UserID:        ownerUserID,
		Type:          EventUtilization,
		SchemaVersion: utilizationEventSchemaVersion,
		Payload:       payload,
	})
	return err
}

func windowsByLenDesc() []string {
	ws := append([]string(nil), knownWindows...)
	sort.SliceStable(ws, func(i, j int) bool { return len(ws[i]) > len(ws[j]) })
	return ws
}

// windowAliases maps a canonical window name to the header spellings it may
// appear as (hyphen or underscore; the exact spelling is TBD-BRINGUP, so both
// are accepted tolerantly).
func windowAliases(win string) []string {
	switch win {
	case "7d_oi":
		return []string{"7d-oi", "7d_oi", "7doi"}
	default:
		return []string{win}
	}
}

// trimWindow reports whether suffix is "<alias>-<field>" and returns the field.
func trimWindow(suffix, alias string) (string, bool) {
	p := alias + "-"
	if strings.HasPrefix(suffix, p) {
		return strings.TrimPrefix(suffix, p), true
	}
	return "", false
}

// parseResetUntrusted attempts both unix-seconds and RFC3339 forms. The value
// scale is TBD-BRINGUP, so this is best-effort and never load-bearing — the
// result is trust-gated by TrustParkTiming.
func parseResetUntrusted(v string) (time.Time, bool) {
	if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
		// Heuristic unix-seconds vs milliseconds; both recorded raw regardless.
		if n > 1e12 {
			return time.UnixMilli(n).UTC(), true
		}
		if n > 1e9 {
			return time.Unix(n, 0).UTC(), true
		}
	}
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(v)); err == nil {
		return t.UTC(), true
	}
	return time.Time{}, false
}
