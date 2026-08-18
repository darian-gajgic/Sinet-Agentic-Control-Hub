package review_test

// P3-RW-18 D2 acceptance tests — committed RED with the brief (Amendment-A;
// red window closed by the packet's implementation commit).
//
// Walk-proven gap: the deliverable mint (stage/review_sink.go → EnsureDeliverable)
// never fills project_id, though the project is known at mint time — the
// task→project edge is durable platform data (the task_project view, migration
// 0016/0022/0023: approved claims, else the intake registry pin, else the
// onboarding id). S13.1's row contract names "project ref" as a deliverable
// column; with it empty, the accept door refuses a repo-backed accept with
// "belongs to no project" (internal/api/accept.go acceptable()) — walk row
// dlv-t-1e211253dfa21c28. Fix shape: mint resolves the edge (write side), and
// reads COALESCE through the same task_project expression so pre-fix rows
// serve without any data migration (migrations 0001–0024 stay byte-untouched).

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
)

// claim records an active artifact claim, the strongest leg of the
// task_project edge (the api-test twoOwners shape).
func claim(f *fix, taskID, project, owner string) {
	f.t.Helper()
	err := f.db.WriteTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(),
			`INSERT INTO artifact_claims (task_id, project, user_id, path_globs, mode, status, created_ts)
			 VALUES (?,?,?,?,?,?,?)`,
			taskID, project, owner, "**", "W", "active", time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
	if err != nil {
		f.t.Fatalf("insert artifact claim: %v", err)
	}
}

// TestEnsureDeliverableResolvesTheProjectAtMint — the mint site passes no
// ProjectID (both stage/review_sink.go callers); the store must resolve the
// task's registered project through the SAME expression the views use
// (task_project), so the row is born whole (S13.1: task ref, project ref, …).
func TestEnsureDeliverableResolvesTheProjectAtMint(t *testing.T) {
	f := newFix(t)
	f.task("t1", "u1")
	f.run("r1", "t1")
	claim(f, "t1", "proj-x", "u1")

	d, err := f.store.EnsureDeliverable(context.Background(), review.EnsureInput{
		ID: "dlv-t1", Owner: "u1", TaskID: "t1", Type: "markdown",
	})
	if err != nil {
		t.Fatalf("EnsureDeliverable: %v", err)
	}
	if d.ProjectID != "proj-x" {
		t.Fatalf("minted deliverable project = %q, want proj-x — the walk-proven empty-project mint (RW-18 D2; S13.1)", d.ProjectID)
	}

	// An explicitly passed project still wins: the resolver is a fallback for
	// callers that do not know it, never an override of one that does.
	d2, err := f.store.EnsureDeliverable(context.Background(), review.EnsureInput{
		ID: "dlv-t1-def", Owner: "u1", TaskID: "t1", Type: "worker-definition", ProjectID: "explicit",
	})
	if err != nil {
		t.Fatalf("EnsureDeliverable explicit: %v", err)
	}
	if d2.ProjectID != "explicit" {
		t.Fatalf("explicit project overwritten: %q", d2.ProjectID)
	}
}

// TestDeliverableReadsServeTheProjectForLegacyEmptyRows — rows minted before
// the fix (the live walk world's dlv-t-1e211253dfa21c28) carry the empty
// string. The read
// side serves the task-join fallback so the accept door and every view see the
// truth WITHOUT a data migration: serve-time COALESCE over task_project.
func TestDeliverableReadsServeTheProjectForLegacyEmptyRows(t *testing.T) {
	f := newFix(t)
	f.task("t1", "u1")
	f.run("r1", "t1")

	// The legacy row: project_id '', exactly as today's mint writes it.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err := f.db.WriteTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(),
			`INSERT INTO deliverables (deliverable_id, user_id, task_id, project_id, subject_ref, dtype,
			                           current_revision, state, created_ts, updated_ts)
			 VALUES ('dlv-t1', 'u1', 't1', '', '', 'markdown', 0, 'in-review', ?, ?)`, now, now)
		return err
	})
	if err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	claim(f, "t1", "proj-x", "u1")

	d, err := f.store.Deliverable(context.Background(), "dlv-t1")
	if err != nil {
		t.Fatalf("Deliverable: %v", err)
	}
	if d.ProjectID != "proj-x" {
		t.Fatalf("legacy empty-project row serves project %q, want proj-x via the task_project fallback (RW-18 D2)", d.ProjectID)
	}

	// A task with NO registered project stays honestly projectless: the
	// fallback resolves recorded facts, it never invents a linkage.
	f.task("t2", "u1")
	err = f.db.WriteTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(),
			`INSERT INTO deliverables (deliverable_id, user_id, task_id, project_id, subject_ref, dtype,
			                           current_revision, state, created_ts, updated_ts)
			 VALUES ('dlv-t2', 'u1', 't2', '', '', 'markdown', 0, 'in-review', ?, ?)`, now, now)
		return err
	})
	if err != nil {
		t.Fatalf("insert projectless row: %v", err)
	}
	d2, err := f.store.Deliverable(context.Background(), "dlv-t2")
	if err != nil {
		t.Fatalf("Deliverable t2: %v", err)
	}
	if d2.ProjectID != "" {
		t.Fatalf("projectless task gained project %q — the fallback invented a linkage", d2.ProjectID)
	}
}
