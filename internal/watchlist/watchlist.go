// Package watchlist is the Spec S14.6 watchlist executor: the machinery that
// detects outside-world change — formats, limits, billing status, adapter
// behavior — before failures or costs spread.
//
// It owns the watch-row config store (migration 0013, the S14.6 ¶1 rows
// `{url|feed, type, parser-hint, lane|candidate}`), the config-as-code driver
// for the adopted changedetection.io page-diff organ (T1), the Sinet-native
// conditional-GET feed poller (T2/T4), the weekly aggregator jobs including the
// genai-prices refresh-as-proposal (T3), the local-tier second-pass
// classification of every hit, the drift.finding emitter with
// fingerprint-per-incident-window dedup, and the P-T11-3 decay posture.
//
// The package owns the LOGIC; the shell owns WHEN (one goroutine, the
// walTruncate/watchdog-loop precedent). Dueness is derived from stored state
// (`watch_rows.last_fetch_at` + the row's cadence), never an in-memory ticker,
// so the loop survives restart and suspend.
//
// The API canary layer (`canary.result`: auth / behavioral / logprob /
// model-list drift) is the sibling half of S14.6 and is NOT built here — this
// package produces `drift.finding` only. The canary seams it will consume
// (periodic execution, card derivation) are the ones below.
//
// Imports are storage/eventlog/local/metering + stdlib only (AST-pinned,
// importwall_test.go): ⚙ values ride the package's own narrow Settings
// interface rather than an internal/settings import, and the S14.8 revalidation
// edge rides a func seam composed at the shell root (the FlagByModelFunc /
// eval-seams precedent, CONVENTIONS §28/§33) rather than an internal/evals
// import. It NEVER imports api/adapters/worker/stage/verify.
package watchlist

import (
	"errors"
	"time"
)

// EventDriftFinding is the S14.2 Drift & canary family type this package mints
// (Spec S14.6 ¶2). Its payload carries the family's contract minimum verbatim:
// source, lane(s), change class, severity, one-line summary, incident
// fingerprint.
//
// It is OUTSIDE-WORLD drift. `registry.drift` (Spec S13.7, internal/project) is
// project-topology drift under the Platform family — a name collision that must
// never mis-route (CONVENTIONS §29).
const EventDriftFinding = "drift.finding"

// platformUser is the owner attribution of every row and event this package
// writes: a watch hit has no run and no person (15.6; S01.9 platform scope ⇒
// the operator sees them, a member never does).
const platformUser = "platform"

// eventSchemaVersion is the S14.2 schema_version of drift.finding at v0.
const eventSchemaVersion = 1

// Watch-row kinds (the S14.6 ¶1 "type" field).
const (
	// KindPage is a page-diff watch driven onto changedetection.io (T1).
	KindPage = "page"
	// KindFeed is an Atom/RSS feed polled by the Sinet-native poller (T2/T4).
	KindFeed = "feed"
	// KindAPI is a structured aggregator endpoint polled and structurally
	// diffed (T3).
	KindAPI = "api"
	// KindStudy is a standing registration/reading obligation with no polled
	// source: the S16.8 gate rows, the components.lock review rows, and the G4
	// [schema] STUDY row. A study row is never polled; it surfaces through
	// PendingReview for the S16.7 quarterly pass.
	KindStudy = "study"
)

// Cadence tokens. These are STRUCTURAL, not ⚙: S18 ratifies no
// watchlist-cadence key and none may be invented (the CONVENTIONS §7
// sseBatchSize precedent, interim under the standing settings-tab directive).
// The values are report-02 §6's cadence set verbatim.
const (
	CadenceContinuous = "continuous"
	CadenceWeekly     = "weekly"
	CadenceMonthly    = "monthly"
	CadenceQuarterly  = "quarterly"
)

// Structural cadence intervals. `continuous` is report-02 §6's "event-driven"
// class realized as a short poll: with conditional GET a no-change poll is a
// 304, so ~15 feeds at this cadence cost essentially nothing. The month is the
// same coarse 30 days internal/conformance uses, so quarterly = 3 months.
const (
	continuousInterval = 15 * time.Minute
	weeklyInterval     = 7 * 24 * time.Hour
	monthlyInterval    = 30 * 24 * time.Hour
	quarterlyInterval  = 3 * monthlyInterval
)

// Change classes — the S14.2 Drift & canary contract-minimum enum verbatim,
// plus the two honest non-answers a local classifier may return.
const (
	ClassPrice         = "price"
	ClassTerms         = "terms"
	ClassLimits        = "limits"
	ClassModels        = "models"
	ClassEndpoints     = "endpoints"
	ClassBillingRegime = "billing-regime"
	// ClassNone: the classifier judged the change irrelevant.
	ClassNone = "none"
	// ClassUnclear is the mandatory abstain member (S12.4): the second pass
	// could not tell, or no second pass ran. It is never a fabricated label.
	ClassUnclear = "unclear"
)

// Alert severities — the S14.4 alert-routing classes (distinct from the inbox
// Low/Medium/High risk tiers).
const (
	// SeverityFlagNow: a hit touching an ACTIVE lane's price/terms/limits/
	// models/endpoints, or any billing-regime change.
	SeverityFlagNow = "flag-now"
	// SeverityDigest: everything else. The two strings are deliberately the
	// same vocabulary internal/watchdog emits (flag-now / daily-digest), so the
	// two S14.4 alert-routing producers never disagree on a wire value.
	SeverityDigest = "daily-digest"
)

// Source groups — the seeding provenance recorded on every row (R3) and the
// filter the S16.7 quarterly pass reads (R21).
const (
	GroupReport02   = "report-02-§6"
	GroupS168       = "S16.8"
	GroupLockReview = "components.lock"
	GroupStudy      = "G4-study"
	// GroupCanarySet holds the S14.6 ¶3 canary-set members that the gates
	// registered as WATCH ROWS rather than as probes (B5-6B).
	GroupCanarySet = "S14.6-canary-set"
	// GroupBenchmarkQuestions holds the four standing benchmark/eval questions
	// S14.7's last bullet registers "so they cannot evaporate" (B5-7). They are
	// registrations, never probes: each is a question the practice must
	// eventually answer, surfaced through PendingReview for the S16.7 quarterly
	// pass. No analysis machinery is built for them at v0.
	GroupBenchmarkQuestions = "S14.7-questions"
)

// Package errors. Callers branch on these.
var (
	// ErrUnknownRow: no watch row has the given id.
	ErrUnknownRow = errors.New("watchlist: unknown watch row")
	// ErrInvalidRow: a seed or operator-supplied row fails validation.
	ErrInvalidRow = errors.New("watchlist: invalid watch row")
	// ErrOrganAbsent: the changedetection.io organ is not configured or not
	// reachable. The page tier degrades to absent; the feed and API tiers keep
	// working (R8).
	ErrOrganAbsent = errors.New("watchlist: changedetection.io organ absent")
)

// Settings is this package's narrow view of the settings registry (CONVENTIONS
// §2: consumers read ⚙ values by dotted key through per-package interfaces,
// live at evaluation time). *settings.Registry satisfies it.
//
// Consumed: watchlist.fetch_fail_streak (the R18 meta-watch),
// adoption.price_data_stale_days (the R14 staleness horizon).
//
// Referenced but NOT consumed here, named so nothing hardcodes them:
// scheduler.missed_slot_default (the v1 user-facing schedule surface, S10.7 —
// at v0 the only schedules are platform-internal timers),
// adoption.dependency_pass_months / adoption.abandonment_months (the S16.7
// quarterly pass, which READS this store through PendingReview but owns its own
// cadence), and canary.auth_interval / canary.behavioral_interval (the API
// canary layer's, not this half's).
type Settings interface {
	Int(key string) (int64, error)
}

// ⚙ dotted keys consumed by this package. Read live at evaluation time so an
// operator change applies without a restart.
const (
	keyFetchFailStreak    = "watchlist.fetch_fail_streak"
	keyPriceDataStaleDays = "adoption.price_data_stale_days"
)

// cadenceInterval resolves a cadence token to its structural interval.
func cadenceInterval(cadence string) (time.Duration, bool) {
	switch cadence {
	case CadenceContinuous:
		return continuousInterval, true
	case CadenceWeekly:
		return weeklyInterval, true
	case CadenceMonthly:
		return monthlyInterval, true
	case CadenceQuarterly:
		return quarterlyInterval, true
	}
	return 0, false
}

// activeLanes are the lanes a hit can touch for the S14.4 flag-now split. A
// candidate name (a watch-only provider) is not an active lane, so its hits
// route to the daily digest (Spec S03.2 lane set).
var activeLanes = map[string]bool{
	"anthropic": true,
	"zai":       true,
	"local":     true,
}

// severityFor computes the S14.4 alert-routing class for a classified hit
// (S14.6 ¶2): flag-now for a hit touching an ACTIVE lane's price / terms /
// limits / models / endpoints, and for ANY billing-regime change (a
// billing-regime change is load-bearing even on a candidate — the operator
// confirms before the rehearsed S10.2 flip); daily digest otherwise.
func severityFor(class string, lanes []string) string {
	if class == ClassBillingRegime {
		return SeverityFlagNow
	}
	switch class {
	case ClassPrice, ClassTerms, ClassLimits, ClassModels, ClassEndpoints:
		for _, lane := range lanes {
			if activeLanes[lane] {
				return SeverityFlagNow
			}
		}
	}
	return SeverityDigest
}
