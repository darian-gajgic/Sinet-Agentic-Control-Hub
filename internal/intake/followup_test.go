package intake_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

type fuFix struct {
	t   *testing.T
	ctx context.Context
	db  *storage.DB
	fu  *intake.FollowUp
}

func newFUFix(t *testing.T) *fuFix {
	t.Helper()
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	log := eventlog.New(db, reg)
	// Seed a user, source task, deliverable + revision (the follow-up source).
	if err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		for _, q := range []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO users (user_id, role, created_ts) VALUES ('u1','operator','t')`, nil},
			{`INSERT INTO tasks (task_id, user_id, created_ts) VALUES ('src','u1','t')`, nil},
			{`INSERT INTO deliverables (deliverable_id, user_id, task_id, dtype, current_revision, state, created_ts, updated_ts)
			  VALUES ('dlv','u1','src','markdown',1,'accepted','t','t')`, nil},
			{`INSERT INTO deliverable_revisions (deliverable_id, n, user_id, pin_kind, content_sha256, platform_ref, created_ts)
			  VALUES ('dlv',1,'u1','content','abc','refs/sinet/deliverable/dlv/rev-1','t')`, nil},
		} {
			if _, err := tx.ExecContext(ctx, q.sql, q.args...); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return &fuFix{t: t, ctx: ctx, db: db, fu: &intake.FollowUp{DB: db, Log: log}}
}

// TestSpawnFollowUpAndBidirectionalLineage (rubric 11): a finished deliverable
// spawns a successor task in one action carrying a successor_of link, entering
// normal intake with inherited context + the predecessor ref as brief material;
// lineage is queryable in BOTH directions.
func TestSpawnFollowUpAndBidirectionalLineage(t *testing.T) {
	f := newFUFix(t)
	var started intake.Request
	f.fu.Start = func(_ context.Context, req intake.Request) error { started = req; return nil }

	req, err := f.fu.Spawn(f.ctx, intake.FollowUpInput{
		NewTaskID: "succ", Owner: "u1", DeliverableID: "dlv", Revision: 1, ProjectID: "proj",
		Preset: intake.PresetCounterpart, PresetDetail: "now the English version", Objective: "Translate it",
		Title: "English version",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	// The successor entered normal intake with the built request.
	if started.TaskID != "succ" || started.UserID != "u1" {
		t.Errorf("successor did not enter intake: %+v", started)
	}
	// The framing carries the preset, the predecessor ref (brief material), and
	// the inherited project context.
	for _, want := range []string{"Counterpart", "now the English version",
		"refs/sinet/deliverable/dlv/rev-1", "brief material", "Project: proj", "inherited context", "Translate it"} {
		if !strings.Contains(req.Text, want) {
			t.Errorf("follow-up framing missing %q:\n%s", want, req.Text)
		}
	}
	// Bidirectional lineage.
	dlv, rev, ok, err := f.fu.PredecessorOf(f.ctx, "succ")
	if err != nil || !ok || dlv != "dlv" || rev != 1 {
		t.Errorf("PredecessorOf(succ) = %s,%d,%v,%v", dlv, rev, ok, err)
	}
	succ, err := f.fu.SuccessorsOf(f.ctx, "dlv", 1)
	if err != nil || len(succ) != 1 || succ[0] != "succ" {
		t.Errorf("SuccessorsOf(dlv,1) = %v, %v", succ, err)
	}
}

// TestPresetsAreFramingsNotSchemaVariants (rubric 12): revision/extension/
// counterpart change only the intake framing, over the SAME link shape.
func TestPresetsAreFramingsNotSchemaVariants(t *testing.T) {
	f := newFUFix(t)
	presets := []intake.Preset{intake.PresetRevision, intake.PresetExtension, intake.PresetCounterpart}
	for i, p := range presets {
		req, err := f.fu.Spawn(f.ctx, intake.FollowUpInput{
			Owner: "u1", DeliverableID: "dlv", Revision: 1, Preset: p, Objective: "x",
		})
		if err != nil {
			t.Fatalf("spawn %s: %v", p, err)
		}
		// Every preset produces the SAME link to (dlv,1) — proven by the source
		// side collecting all successors.
		_ = req
		_ = i
	}
	succ, err := f.fu.SuccessorsOf(f.ctx, "dlv", 1)
	if err != nil || len(succ) != len(presets) {
		t.Fatalf("presets did not all link to (dlv,1): %v, %v", succ, err)
	}
	// A revision preset frames differently than an extension — same schema, same
	// link, different framing.
	rev, _ := f.fu.Spawn(f.ctx, intake.FollowUpInput{Owner: "u1", DeliverableID: "dlv", Revision: 1, Preset: intake.PresetRevision})
	ext, _ := f.fu.Spawn(f.ctx, intake.FollowUpInput{Owner: "u1", DeliverableID: "dlv", Revision: 1, Preset: intake.PresetExtension})
	if rev.Text == ext.Text {
		t.Error("revision and extension presets produced identical framing (should differ)")
	}
}

// TestFollowUpRejectsUnknownRevision: the composite FK rejects a link to a
// non-existent (deliverable, revision).
func TestFollowUpRejectsUnknownRevision(t *testing.T) {
	f := newFUFix(t)
	if _, err := f.fu.Spawn(f.ctx, intake.FollowUpInput{Owner: "u1", DeliverableID: "dlv", Revision: 99}); err == nil {
		t.Fatal("a follow-up linked to a non-existent revision was not rejected")
	}
}
