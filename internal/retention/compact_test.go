package retention_test

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/retention"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// TestCompactionStripsOnlyAtOrBelowHorizonNonKeepForever (rubric 6, 11) is the
// core S14.9 ¶2 property in one table: past the horizon and not keep-forever is
// stripped; anything else is untouched. Two owners with DIFFERENT per-user
// horizons prove ⚙ retention.compaction_horizon is read with IntFor.
func TestCompactionStripsOnlyAtOrBelowHorizonNonKeepForever(t *testing.T) {
	f := newFixture(t)
	f.user("alice", "member")
	f.user("bob", "member")

	// alice keeps 3 months, bob keeps 12 (default is 6, so neither is the
	// default and a single global read would be visibly wrong for both).
	f.setHorizon("alice", 3)
	f.setHorizon("bob", 12)

	type planted struct {
		name   string
		seq    int64
		strip  bool
		reason string
	}
	var rows []planted
	plant := func(name, owner, typ string, ageMonths int, strip bool, reason string) {
		seq := f.appendAt("", owner, typ,
			`{"tool":"grep","secret":"`+name+`-BODY"}`, f.now.AddDate(0, -ageMonths, 0))
		rows = append(rows, planted{name, seq, strip, reason})
	}
	// alice, horizon 3 months
	plant("alice-old-trace", "alice", "tool.completed", 6, true, "trace past alice's 3-month horizon")
	plant("alice-old-keep", "alice", "verdict.recorded", 6, false, "keep-forever at any age")
	plant("alice-new-trace", "alice", "tool.completed", 1, false, "inside alice's horizon")
	// bob, horizon 12 months
	plant("bob-6mo-trace", "bob", "tool.completed", 6, false, "inside bob's 12-month horizon (it would be OUTSIDE alice's)")
	plant("bob-old-trace", "bob", "tool.completed", 18, true, "trace past bob's 12-month horizon")
	plant("bob-old-routing", "bob", "routing.decided", 18, false, "keep-forever at any age")

	res, err := f.store.Compact(f.ctx, f.now)
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if res.EventsStripped != 2 {
		t.Errorf("events stripped = %d, want 2", res.EventsStripped)
	}
	for _, p := range rows {
		got := f.payloadAt(p.seq)
		stripped := got == retention.CompactedPayload
		if stripped != p.strip {
			t.Errorf("%s: stripped=%v, want %v (%s)", p.name, stripped, p.strip, p.reason)
		}
		if !p.strip && !strings.Contains(got, p.name+"-BODY") {
			t.Errorf("%s: body was lost though it must be preserved (%s)", p.name, p.reason)
		}
	}

	// The per-owner legs carry the horizon, boundary and per-family counts (R7).
	// The pass covers every owner with rows in the log, which here also includes
	// the platform (the ⚙ writes above logged settings.changed rows).
	byOwner := map[string]retention.OwnerPass{}
	for _, o := range res.Owners {
		byOwner[o.UserID] = o
	}
	for _, want := range []string{"alice", "bob"} {
		if _, ok := byOwner[want]; !ok {
			t.Fatalf("owner %q has no leg in the pass (%d legs)", want, len(res.Owners))
		}
	}
	if byOwner["alice"].HorizonMonths != 3 || byOwner["bob"].HorizonMonths != 12 {
		t.Errorf("horizons = alice %d / bob %d, want 3 / 12 — the ⚙ is PER-USER (IntFor, not Int)",
			byOwner["alice"].HorizonMonths, byOwner["bob"].HorizonMonths)
	}
	for _, name := range []string{"alice", "bob"} {
		o := byOwner[name]
		if o.BoundaryEventSeq == 0 {
			t.Errorf("%s: boundary event_seq is 0; the pass must record where its boundary fell", o.UserID)
		}
		if o.EventsStripped != 1 || o.ByFamily["tools_artifacts"] != 1 {
			t.Errorf("%s: leg = %+v, want exactly one tools_artifacts strip", o.UserID, o)
		}
	}

	// No row was deleted and event_seq stays gap-free (rubric 11).
	var count, span int64
	if err := f.db.QueryRowContext(f.ctx,
		`SELECT count(*), max(event_seq)-min(event_seq)+1 FROM run_events`).Scan(&count, &span); err != nil {
		t.Fatal(err)
	}
	if count < int64(len(rows)) || count != span {
		t.Errorf("run_events count=%d span=%d, want >= %d and gap-free — compaction elides bodies, never rows",
			count, span, len(rows))
	}
}

// TestCompactionIsLiveApplyPerUser: the horizon is read at evaluation time, so
// an operator change lands on the next pass with no restart.
func TestCompactionIsLiveApplyPerUser(t *testing.T) {
	f := newFixture(t)
	f.user("alice", "member")
	seq := f.appendAt("", "alice", "tool.completed", `{"secret":"MID-BODY"}`, f.now.AddDate(0, -8, 0))

	f.setHorizon("alice", 12)
	if _, err := f.store.Compact(f.ctx, f.now); err != nil {
		t.Fatal(err)
	}
	if f.payloadAt(seq) == retention.CompactedPayload {
		t.Fatal("an 8-month-old row was stripped under a 12-month horizon")
	}
	// Move the dial mid-life; the very next pass honors it.
	f.setHorizon("alice", 6)
	if _, err := f.store.Compact(f.ctx, f.now); err != nil {
		t.Fatal(err)
	}
	if f.payloadAt(seq) != retention.CompactedPayload {
		t.Error("the horizon change was not applied live — ⚙ must be read at evaluation time")
	}
}

// TestCompactionIsIdempotent (rubric 12): a second pass over the same window
// strips nothing further and logs nothing further.
func TestCompactionIsIdempotent(t *testing.T) {
	f := newFixture(t)
	f.user("alice", "member")
	f.appendAt("", "alice", "tool.completed", `{"secret":"OLD"}`, f.now.AddDate(0, -9, 0))

	first, err := f.store.Compact(f.ctx, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if first.EventsStripped != 1 || first.EventSeq == 0 {
		t.Fatalf("first pass = %+v, want 1 stripped and a logged retention.compacted row", first)
	}
	second, err := f.store.Compact(f.ctx, f.now)
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if second.EventsStripped != 0 {
		t.Errorf("second pass stripped %d, want 0 (idempotent)", second.EventsStripped)
	}
	if second.EventSeq != 0 {
		t.Error("a no-op pass logged retention.compacted; the audit trail records COMPACTION, and none happened")
	}
	// Exactly one retention.compacted row exists.
	var n int
	if err := f.db.QueryRowContext(f.ctx,
		`SELECT count(*) FROM run_events WHERE type = ?`, retention.EventCompacted).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("retention.compacted rows = %d, want 1", n)
	}
}

// TestCompactionPassLogsItself (rubric 10): platform-scope, per-owner counts.
func TestCompactionPassLogsItself(t *testing.T) {
	f := newFixture(t)
	f.user("alice", "member")
	f.user("bob", "member")
	f.appendAt("", "alice", "tool.completed", `{"secret":"A"}`, f.now.AddDate(0, -9, 0))
	f.appendAt("", "bob", "helper.spawned", `{"depth":1}`, f.now.AddDate(0, -9, 0))

	res, err := f.store.Compact(f.ctx, f.now)
	if err != nil {
		t.Fatal(err)
	}
	var owner, payload string
	var runID sql.NullString
	if err := f.db.QueryRowContext(f.ctx,
		`SELECT user_id, run_id, payload FROM run_events WHERE event_seq = ?`, res.EventSeq).
		Scan(&owner, &runID, &payload); err != nil {
		t.Fatal(err)
	}
	if owner != "platform" {
		t.Errorf("retention.compacted owner = %q, want platform (one row per pass, per-owner counts inside)", owner)
	}
	if runID.Valid {
		t.Error("retention.compacted must be platform-scope (run_id NULL)")
	}
	var decoded retention.PassResult
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("payload is not the pass result: %v", err)
	}
	if decoded.AsOf == "" {
		t.Error("the payload must carry the pass's as-of stamp")
	}
	stripped := map[string]retention.OwnerPass{}
	for _, o := range decoded.Owners {
		if o.BoundaryTS == "" || o.HorizonMonths == 0 {
			t.Errorf("%s leg is missing its horizon/boundary: %+v", o.UserID, o)
		}
		if o.EventsStripped > 0 {
			stripped[o.UserID] = o
		}
	}
	if len(stripped) != 2 || stripped["alice"].ByFamily == nil || stripped["bob"].ByFamily == nil {
		t.Errorf("both owners' per-family counts must ride the one platform-scope row; got %+v", stripped)
	}
}

// TestCompactionStripsTranscriptCopyAsides (R5): the S02.2 engine_sessions
// copy-aside — the FILE and the row's reference to it — past the horizon. The
// row itself survives (it is the reboot-case mining index).
func TestCompactionStripsTranscriptCopyAsides(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.jsonl")
	newPath := filepath.Join(dir, "new.jsonl")
	for _, p := range []string{oldPath, newPath} {
		if err := os.WriteFile(p, []byte(`{"transcript":"body"}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	f := newFixture(t)
	f.user("alice", "member")
	f.startRun("r-old", "alice", "")
	f.startRun("r-new", "alice", "")
	seed := func(key int64, runID, path string, ageMonths int) {
		ts := f.now.AddDate(0, -ageMonths, 0).Format(time.RFC3339Nano)
		if err := f.db.WriteTx(f.ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(f.ctx,
				`INSERT INTO engine_sessions (session_key, run_id, user_id, transcript_copy_path, created_ts, updated_ts)
				 VALUES (?, ?, 'alice', ?, ?, ?)`, key, runID, path, ts, ts)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	seed(1, "r-old", oldPath, 9)
	seed(2, "r-new", newPath, 1)

	res, err := f.store.Compact(f.ctx, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if got := ownerLeg(t, res, "alice").TranscriptsStripped; got != 1 {
		t.Errorf("transcripts stripped = %d, want 1", got)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("the old transcript copy-aside file survived the horizon")
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("a transcript inside the horizon was deleted: %v", err)
	}
	var oldRef, newRef string
	var rowCount int
	if err := f.db.QueryRowContext(f.ctx, `SELECT count(*) FROM engine_sessions`).Scan(&rowCount); err != nil {
		t.Fatal(err)
	}
	if rowCount != 2 {
		t.Errorf("engine_sessions rows = %d, want 2 (the session record survives; only the copy-aside goes)", rowCount)
	}
	if err := f.db.QueryRowContext(f.ctx,
		`SELECT transcript_copy_path FROM engine_sessions WHERE session_key = 1`).Scan(&oldRef); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRowContext(f.ctx,
		`SELECT transcript_copy_path FROM engine_sessions WHERE session_key = 2`).Scan(&newRef); err != nil {
		t.Fatal(err)
	}
	if oldRef != "" {
		t.Errorf("the stripped row still references %q — the reference goes with the file", oldRef)
	}
	if newRef != newPath {
		t.Errorf("an in-horizon reference was cleared: %q", newRef)
	}
}

// TestTranscriptStripErrorLeavesTheReference: a filesystem error is counted and
// the reference stays, so the next pass retries rather than orphaning a file.
func TestTranscriptStripErrorLeavesTheReference(t *testing.T) {
	f := newFixture(t, withRemoveFile(func(string) error { return errPermission{} }))
	f.user("alice", "member")
	f.startRun("r1", "alice", "")
	ts := f.now.AddDate(0, -9, 0).Format(time.RFC3339Nano)
	if err := f.db.WriteTx(f.ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(f.ctx,
			`INSERT INTO engine_sessions (session_key, run_id, user_id, transcript_copy_path, created_ts, updated_ts)
			 VALUES (1, 'r1', 'alice', '/nope/x.jsonl', ?, ?)`, ts, ts)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	res, err := f.store.Compact(f.ctx, f.now)
	if err != nil {
		t.Fatalf("a filesystem error must not fail the pass: %v", err)
	}
	leg := ownerLeg(t, res, "alice")
	if leg.TranscriptErrors != 1 || leg.TranscriptsStripped != 0 {
		t.Errorf("owner leg = %+v, want the error counted and nothing claimed stripped", leg)
	}
	var ref string
	if err := f.db.QueryRowContext(f.ctx,
		`SELECT transcript_copy_path FROM engine_sessions WHERE session_key = 1`).Scan(&ref); err != nil {
		t.Fatal(err)
	}
	if ref == "" {
		t.Error("the reference was cleared though the file could not be removed — the file would be orphaned")
	}
}

// ownerLeg picks one owner's leg out of a pass result.
func ownerLeg(t *testing.T, res retention.PassResult, userID string) retention.OwnerPass {
	t.Helper()
	for _, o := range res.Owners {
		if o.UserID == userID {
			return o
		}
	}
	t.Fatalf("no pass leg for owner %q", userID)
	return retention.OwnerPass{}
}

type errPermission struct{}

func (errPermission) Error() string { return "permission denied" }

// TestCompactedRowStillClassifiesAndTheGeneratedColumnsGoHonestlyNull: after a
// strip the row keeps its identity (type/ts/owner/seq) and the payload-derived
// generated columns read NULL rather than a stale value.
func TestCompactedRowKeepsItsIdentity(t *testing.T) {
	f := newFixture(t)
	f.user("alice", "member")
	seq := f.appendAt("", "alice", "drift.finding", `{"change_class":"price","summary":"x"}`, f.now.AddDate(0, -9, 0))
	// drift.finding is keep-forever, so plant a trace row to actually strip.
	traceSeq := f.appendAt("", "alice", "helper.spawned", `{"depth":2,"brief_hash":"h"}`, f.now.AddDate(0, -9, 0))
	if _, err := f.store.Compact(f.ctx, f.now); err != nil {
		t.Fatal(err)
	}
	var typ, owner, ts string
	var changeClass sql.NullString
	var dotted string
	if err := f.db.QueryRowContext(f.ctx,
		`SELECT type, user_id, ts, change_class, dotted_order FROM run_events WHERE event_seq = ?`, traceSeq).
		Scan(&typ, &owner, &ts, &changeClass, &dotted); err != nil {
		t.Fatal(err)
	}
	if typ != "helper.spawned" || owner != "alice" || ts == "" {
		t.Errorf("identity moved on a compacted row: type=%q owner=%q ts=%q", typ, owner, ts)
	}
	if changeClass.Valid {
		t.Errorf("change_class = %q on a compacted row; a payload-derived column must go NULL, not stale", changeClass.String)
	}
	if !strings.HasPrefix(dotted, "platform.00.") {
		t.Errorf("dotted_order = %q; a compacted row degrades to depth 0 honestly", dotted)
	}
	// The keep-forever row still carries its class.
	if err := f.db.QueryRowContext(f.ctx,
		`SELECT change_class FROM run_events WHERE event_seq = ?`, seq).Scan(&changeClass); err != nil {
		t.Fatal(err)
	}
	if changeClass.String != "price" {
		t.Errorf("keep-forever change_class = %q, want price", changeClass.String)
	}
}

// ── R9 / OQ2: the 11.3 exporter boundary ────────────────────────────────────

// TestPlantedSecretNeverLeavesTheHost (rubric 13): a secret in a NON-keep-forever
// payload is absent from the snapshot dump at any age; a keep-forever payload
// survives it. This is the S14.9 ¶3 "structurally unreachable" property.
func TestPlantedSecretNeverLeavesTheHost(t *testing.T) {
	f := newFixture(t)
	f.user("alice", "member")
	// Three ages, all trace: brand new, inside the horizon, past it. None may
	// reach the dump — the boundary is a TYPE boundary at every age.
	f.appendAt("", "alice", "tool.completed", `{"out":"SECRET-FRESH"}`, f.now)
	f.appendAt("", "alice", "engine.message", `{"text":"SECRET-RECENT"}`, f.now.AddDate(0, -2, 0))
	f.appendAt("", "alice", "helper.spawned", `{"brief":"SECRET-ANCIENT"}`, f.now.AddDate(0, -24, 0))
	// Keep-forever, and it must survive.
	f.appendAt("", "alice", "verdict.recorded", `{"verdict":"pass","note":"KEEP-VERDICT"}`, f.now.AddDate(0, -24, 0))
	f.appendAt("", "alice", "routing.decided", `{"plain_reason":"KEEP-ROUTING"}`, f.now)

	vac := filepath.Join(t.TempDir(), "vac.db")
	if err := f.db.VacuumInto(f.ctx, vac); err != nil {
		t.Fatal(err)
	}
	dump, uv, err := storage.DumpFrom(f.ctx, vac)
	if err != nil {
		t.Fatalf("DumpFrom: %v", err)
	}
	for _, secret := range []string{"SECRET-FRESH", "SECRET-RECENT", "SECRET-ANCIENT"} {
		if strings.Contains(dump, secret) {
			t.Errorf("%s reached the snapshot dump — raw trace payload bodies must be structurally unreachable (S14.9 ¶3)", secret)
		}
	}
	for _, kept := range []string{"KEEP-VERDICT", "KEEP-ROUTING"} {
		if !strings.Contains(dump, kept) {
			t.Errorf("%s was stripped from the dump — the keep-forever set must survive the export", kept)
		}
	}
	if !strings.Contains(dump, retention.ExportStrippedPayload) {
		t.Error("the export marker is absent; a stripped row must say why its body is missing")
	}

	// The rebuilt database still satisfies the S02.9 invariants: every row is
	// present and event_seq is gap-free.
	rebuilt := filepath.Join(t.TempDir(), "rebuilt.db")
	if err := storage.RebuildFromDump(f.ctx, rebuilt, dump, uv); err != nil {
		t.Fatalf("RebuildFromDump: %v", err)
	}
	check, err := storage.CheckRestored(f.ctx, rebuilt)
	if err != nil {
		t.Fatal(err)
	}
	if !check.AllOK() {
		t.Fatalf("S02.9 invariants failed after the boundary strip: %+v", check)
	}
	if check.RunEventCount != 5 {
		t.Errorf("rebuilt run_events = %d, want 5 (rows preserved, only bodies elided)", check.RunEventCount)
	}
}

// TestExportViewIsTheOnlyRunEventsSourceInTheDump: the boundary is by
// CONSTRUCTION, so a dump taken against a database without the view must FAIL
// loudly rather than fall back to the raw table.
func TestDumpRefusesWithoutTheExportView(t *testing.T) {
	f := newFixture(t)
	f.user("alice", "member")
	f.appendAt("", "alice", "tool.completed", `{"out":"SECRET"}`, f.now)
	vac := filepath.Join(t.TempDir(), "vac.db")
	if err := f.db.VacuumInto(f.ctx, vac); err != nil {
		t.Fatal(err)
	}
	// Drop the view from the SNAPSHOT copy only (the live DB is untouched).
	raw, err := sql.Open("sqlite", "file:"+vac)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(f.ctx, `DROP VIEW run_events_export`); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	if _, _, err := storage.DumpFrom(f.ctx, vac); err == nil {
		t.Fatal("the dump proceeded without the export view — it would have shipped raw trace bodies")
	} else if !strings.Contains(err.Error(), "run_events_export") {
		t.Errorf("the refusal must name the missing boundary; got %v", err)
	}
}
