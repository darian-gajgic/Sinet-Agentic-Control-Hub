package api

import (
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/history"
)

// topics_retention_test.go — the B5-8A topic-routing READING (brief R17 / OQ7),
// stated and tested rather than changed. The four-topic set {run, board, fleet,
// inbox} and the map itself are untouched by this packet.

// TestRunSummaryCarriesTheRunTagOnly is the OQ7 disposition, proven.
//
// run.summary_written is run-scoped, so it picks up the `run` tag and reaches
// that run's detail subscriber live. It deliberately does NOT reach `board`:
// FamilyRunSummary is in no family branch of the map, and a summary landing on
// board would double-notify the run's own completion, which the accompanying
// run.state_changed already announces there. A run summary is READ from the run
// surface, not pushed to the board.
//
// If B6 later wants it on board, that is a DATA change to the map proven by a
// test — the B5-7 FamilyBenchmarkEval→inbox precedent — and cheap to make then.
func TestRunSummaryCarriesTheRunTagOnly(t *testing.T) {
	tags := topicsForEvent(eventlog.Event{Type: "run.summary_written", RunID: "r1", SchemaVersion: 1})
	if !hasTopic(tags, topicRun) {
		t.Errorf("run.summary_written tags = %v, want the run tag (it is run-scoped)", tags)
	}
	for _, unwanted := range []string{topicBoard, topicFleet, topicInbox} {
		if hasTopic(tags, unwanted) {
			t.Errorf("run.summary_written reached the %q topic; the reading is run-tag only (OQ7)", unwanted)
		}
	}
	// The family is registered and classifies — it simply maps to no household
	// topic, which is a deliberate absence, not a gap.
	if fam, known := eventlog.Classify("run.summary_written"); !known || fam != eventlog.FamilyRunSummary {
		t.Errorf("run.summary_written classifies as %q,%v; want FamilyRunSummary", fam, known)
	}
}

// TestRetentionCompactedReachesNoTopic is the §30 rule applied: platform-scope
// audit types are deliberately in no household topic and flow only on the
// unfiltered relay. The compaction pass is platform machinery, not a card.
func TestRetentionCompactedReachesNoTopic(t *testing.T) {
	tags := topicsForEvent(eventlog.Event{Type: "retention.compacted", SchemaVersion: 1})
	if len(tags) != 0 {
		t.Errorf("retention.compacted tags = %v, want none (platform-scope audit, §30)", tags)
	}
	if fam, known := eventlog.Classify("retention.compacted"); !known || fam != eventlog.FamilyPlatform {
		t.Errorf("retention.compacted classifies as %q,%v; want FamilyPlatform", fam, known)
	}
}

// TestHistoryQueryAuditedReachesNoTopic — B5-8B drain D9, the §30 rule applied
// to the Layer-2 audit type, on the retention.compacted precedent above. A
// query audit is platform machinery: it belongs in the audit trail and on the
// unfiltered relay, never as a household card. Pinned by test rather than
// asserted in prose.
func TestHistoryQueryAuditedReachesNoTopic(t *testing.T) {
	tags := topicsForEvent(eventlog.Event{Type: history.EventQueryAudited, SchemaVersion: 1})
	if len(tags) != 0 {
		t.Errorf("%s tags = %v, want none (platform-scope audit, §30)", history.EventQueryAudited, tags)
	}
	if fam, known := eventlog.Classify(history.EventQueryAudited); !known || fam != eventlog.FamilyPlatform {
		t.Errorf("%s classifies as %q,%v; want FamilyPlatform", history.EventQueryAudited, fam, known)
	}
}
