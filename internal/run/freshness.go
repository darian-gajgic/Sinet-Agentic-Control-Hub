package run

import (
	"fmt"
	"maps"
	"slices"
	"time"
)

// Freshness re-validation on stale resume (Spec S02.6): "blocked is not
// failed" — an answered run resumes from its checkpoint, but if it slept
// past a threshold or its target moved, the remaining plan is re-validated
// at low cost BEFORE spending anything significant. S02 owns the durable
// inputs to that decision — the checkpoint version fields (S02.4e) and the
// G1 Def.5 fingerprint set — and this comparison; the low-cost re-plan
// action itself is Spec S06's (B2 seam), and hold-vs-park enforcement
// (⚙ freshness.hold_vs_park) is the intake/adapter's (Spec S03/S06).

// keyFreshnessMaxAge is the ⚙ resume-age trigger (G1 Def.5, listed in S02's
// settings table).
const keyFreshnessMaxAge = "freshness.max_age"

// FreshnessSettings is this file's view of the settings registry.
type FreshnessSettings interface {
	Duration(key string) (time.Duration, error)
}

// Fingerprint is the durable freshness-fingerprint set (G1 Def.5, Spec
// S02.6): {repo HEAD, source content hashes/ETags, spec+plan version,
// price-table version}, plus cited project-truth knowledge-entry versions
// (the ratified R11 §4.7 extension).
type Fingerprint struct {
	RepoHead           string
	SourceHashes       map[string]string // content hash / ETag by source key
	SpecPlanVersion    string
	PriceTableVersion  string
	CitedEntryVersions map[string]string // knowledge entry version by entry key
}

// VersionFields are the checkpoint version fields of Spec S02.4e: model id,
// invocation-config fingerprint (settings hash + permission mode + model),
// and tool/prompt schema versions.
type VersionFields struct {
	ModelID               string
	InvocationFingerprint string
	ToolSchemaVersion     string
	PromptSchemaVersion   string
}

// FreshnessInput is what a resume compares (Spec S02.6): the stored side
// comes from the last checkpoint; the current side from live observation.
type FreshnessInput struct {
	CheckpointTime  time.Time
	Stored, Current Fingerprint
	StoredVersions  VersionFields
	CurrentVersions VersionFields
	// SiblingAccept reports a sibling-accept event in the project since the
	// checkpoint (Spec S02.8) — a ratified trigger.
	SiblingAccept bool
}

// Freshness is the route-to-freshness classification: Fresh means resume
// directly from the checkpoint; otherwise the run routes to the S02.6
// re-validation pass (continue / adjust-with-a-note / escalate — the pass
// itself is Spec S06, B2) before anything significant is spent.
type Freshness struct {
	Fresh   bool
	Reasons []string
}

// EvaluateFreshness applies the ratified triggers (Spec S02.6): age >
// ⚙ freshness.max_age, OR any fingerprint drift, OR a sibling-accept event.
// Price-table drift ALONE triggers — it re-prices the remaining plan, a
// first-class staleness cause (G1 Def.5).
func EvaluateFreshness(settings FreshnessSettings, in FreshnessInput, now time.Time) (Freshness, error) {
	maxAge, err := settings.Duration(keyFreshnessMaxAge)
	if err != nil {
		return Freshness{}, fmt.Errorf("run: read ⚙ %s: %w", keyFreshnessMaxAge, err)
	}
	var reasons []string
	if !in.CheckpointTime.IsZero() && now.Sub(in.CheckpointTime) > maxAge {
		reasons = append(reasons, fmt.Sprintf("age %s > ⚙ %s %s", now.Sub(in.CheckpointTime).Round(time.Second), keyFreshnessMaxAge, maxAge))
	}
	reasons = append(reasons, fingerprintDrift(in.Stored, in.Current)...)
	reasons = append(reasons, versionDrift(in.StoredVersions, in.CurrentVersions)...)
	if in.SiblingAccept {
		reasons = append(reasons, "sibling-accept event in the project (S02.8)")
	}
	return Freshness{Fresh: len(reasons) == 0, Reasons: reasons}, nil
}

func fingerprintDrift(stored, current Fingerprint) []string {
	var reasons []string
	if stored.RepoHead != current.RepoHead {
		reasons = append(reasons, "repo HEAD moved")
	}
	if stored.SpecPlanVersion != current.SpecPlanVersion {
		reasons = append(reasons, "spec+plan version changed")
	}
	if stored.PriceTableVersion != current.PriceTableVersion {
		reasons = append(reasons, "price-table version changed (re-prices the remaining plan)")
	}
	reasons = append(reasons, mapDrift("source", stored.SourceHashes, current.SourceHashes)...)
	reasons = append(reasons, mapDrift("cited knowledge entry", stored.CitedEntryVersions, current.CitedEntryVersions)...)
	return reasons
}

func versionDrift(stored, current VersionFields) []string {
	var reasons []string
	if stored.ModelID != current.ModelID {
		reasons = append(reasons, "model id changed")
	}
	if stored.InvocationFingerprint != current.InvocationFingerprint {
		reasons = append(reasons, "invocation-config fingerprint changed")
	}
	if stored.ToolSchemaVersion != current.ToolSchemaVersion {
		reasons = append(reasons, "tool schema version changed")
	}
	if stored.PromptSchemaVersion != current.PromptSchemaVersion {
		reasons = append(reasons, "prompt schema version changed")
	}
	return reasons
}

// mapDrift reports keys whose stored value differs from the current one, in
// sorted key order. A key present only on the stored side drifted (its
// source vanished); keys only on the current side are new observations, not
// resume drift.
func mapDrift(kind string, stored, current map[string]string) []string {
	var reasons []string
	for _, k := range slices.Sorted(maps.Keys(stored)) {
		cur, ok := current[k]
		if !ok {
			reasons = append(reasons, fmt.Sprintf("%s %q no longer observable", kind, k))
			continue
		}
		if cur != stored[k] {
			reasons = append(reasons, fmt.Sprintf("%s %q changed", kind, k))
		}
	}
	return reasons
}
