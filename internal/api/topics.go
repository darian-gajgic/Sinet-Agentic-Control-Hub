package api

import "github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"

// topics.go is the S14.3 topic map (brief §2): the coarse routing grouping
// ABOVE the S14.2 event-type families, computed as data. The one SSE endpoint
// is topic-tagged so a single client stream multiplexes run detail, board,
// fleet/meters and inbox; every delivered frame carries its (multi-valued)
// tags so the client routes it, and the server filters the tail to the
// subscribed set. The map keys on the canonical families via eventlog.Classify
// — which already resolves the 11 legacy aliases to their family (R24) — so a
// pre-existing dev/test row still routes; a type in no topic simply never
// matches a topic filter (forward-tolerant, S14.2 rule 3).

// The four household topics (S14.3 ¶1: "run detail, board, fleet/meters,
// inbox"). fleet and meters are one topic (the spec's "fleet/meters").
const (
	topicRun   = "run"   // per-run detail: the full run-scoped trace
	topicBoard = "board" // tasks + run-lifecycle across the owner's runs
	topicFleet = "fleet" // fleet/meters: usage & limits + local seats + GPU
	topicInbox = "inbox" // asks, approvals, watchdog flags, drift cards
)

// knownTopics is the closed subscribe set; an unknown ?topics name is a 400
// (the startCursor bad-param precedent).
var knownTopics = map[string]bool{
	topicRun:   true,
	topicBoard: true,
	topicFleet: true,
	topicInbox: true,
}

// fleetPlatformTypes are the canonical Platform-family types that route to the
// fleet/meters topic even though their family is Platform (not Usage & limits):
// the S12 local-seat operational events. GPU/VRAM types (B4-6) join here when
// their producers land — declared by absence today (they classify Platform and
// simply gain a fleet tag once listed).
var fleetPlatformTypes = map[string]bool{
	"local.admissions_stopped": true,
	"local.admissions_resumed": true,
	"local.alias_retargeted":   true,
	"local.unmetered_defect":   true,
}

// topicsForEvent returns the topic tags an event carries (brief §2). Tags are
// multi-valued: a run.state_changed carries both `board` (its family) and the
// `run` tag (it is run-scoped), so a board subscriber and that run's detail
// subscriber both receive it. The `run` tag rides run-scope (a non-empty
// run_id) — the per-run detail is "ALL run-scoped events for ?run", the full
// trace across families — while board/fleet/inbox ride the family map.
func topicsForEvent(e eventlog.Event) []string {
	var tags []string
	if fam, known := eventlog.Classify(e.Type); known {
		switch fam {
		case eventlog.FamilyRunLifecycle:
			tags = append(tags, topicBoard)
		case eventlog.FamilyUsageLimits:
			tags = append(tags, topicFleet)
		case eventlog.FamilyGateAsk,
			eventlog.FamilyHumanDecision,
			eventlog.FamilyWatchdog,
			eventlog.FamilyDriftCanary:
			tags = append(tags, topicInbox)
		}
	}
	if fleetPlatformTypes[e.Type] {
		tags = append(tags, topicFleet)
	}
	if e.RunID != "" {
		tags = append(tags, topicRun)
	}
	return tags
}
