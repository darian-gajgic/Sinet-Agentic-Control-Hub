package worker_test

import (
	"context"
	"database/sql"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// STANDING conformance battery (Spec S08.2, S08.9): the structural walls
// are proven by attempted violation on every suite run.

// TestConformanceNoModelInAutomation pins the automation package's import
// graph: it must never import internal/adapters (or the stage runner) — a
// deterministic automation structurally CANNOT reach an engine (Spec
// S08.9 "no model in the loop"), extending the §15/§17 import-graph
// precedent.
func TestConformanceNoModelInAutomation(t *testing.T) {
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller info")
	}
	dir := filepath.Join(filepath.Dir(self), "automation")
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("no sources under %s — package moved? update the conformance scan", dir)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imp := range parsed.Imports {
			for _, banned := range []string{"internal/adapters", "internal/stage"} {
				if strings.Contains(imp.Path.Value, banned) {
					t.Errorf("%s imports %s — an automation executes with no model in the loop (Spec S08.9)", name, banned)
				}
			}
		}
	}
}

// TestConformanceWorkerNeverImportsMemory pins the compile-side posture:
// internal/worker reads the overlay slice ONLY through the OverlaySource
// seam (wired at the composition root), never by importing memory — so
// the S09.1 capability walls stay transitively clean for the engine-facing
// wiring B3-3 adds (CONVENTIONS §17 import discipline, extended).
func TestConformanceWorkerNeverImportsMemory(t *testing.T) {
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller info")
	}
	dir := filepath.Dir(self)
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imp := range parsed.Imports {
			if strings.Contains(imp.Path.Value, "internal/memory") {
				t.Errorf("%s imports internal/memory — overlay reads ride the OverlaySource seam (S08.4/S09.1)", name)
			}
		}
	}
}

// TestConformanceGuardrailRowsResistRawSQL attempts the forbidden
// mutations directly against the tables — the 0005 triggers must hold for
// raw-SQL holders too (Spec S08.2: enforcement state changes only through
// the control-plane approval path; S08.4: history never rewritten).
func TestConformanceGuardrailRowsResistRawSQL(t *testing.T) {
	f := newFix(t)
	f.user("alice", "member")
	ctx := context.Background()
	_, v, _ := draftValidated(t, f, "alice")
	if _, err := f.store.Approve(ctx, "alice", v.ID, worker.ApproveOpts{}); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	attempts := map[string]string{
		"widen granted tools": `UPDATE worker_guardrails SET granted_tools = '["Read","Bash"]' WHERE version_id = '` + v.ID + `'`,
		"loosen class":        `UPDATE worker_guardrails SET confinement_class = 'C2' WHERE version_id = '` + v.ID + `'`,
		"raise first-N":       `UPDATE worker_guardrails SET first_n_remaining = 99 WHERE version_id = '` + v.ID + `'`,
		"delete guardrails":   `DELETE FROM worker_guardrails WHERE version_id = '` + v.ID + `'`,
		"rewrite version":     `UPDATE worker_template_versions SET file_sha256 = '` + strings.Repeat("b", 64) + `' WHERE version_id = '` + v.ID + `'`,
		"delete version":      `DELETE FROM worker_template_versions WHERE version_id = '` + v.ID + `'`,
		"forge validation":    `UPDATE validation_records SET green = 0 WHERE version_id = '` + v.ID + `'`,
		"delete template":     `DELETE FROM worker_templates WHERE template_id = (SELECT template_id FROM worker_template_versions WHERE version_id = '` + v.ID + `')`,
		"change owner":        `UPDATE worker_templates SET user_id = 'mallory' WHERE template_id = (SELECT template_id FROM worker_template_versions WHERE version_id = '` + v.ID + `')`,
		"delete domain":       `DELETE FROM domains WHERE domain = 'software'`,
	}
	for name, stmt := range attempts {
		err := f.db.WriteTx(ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, stmt)
			return err
		})
		if err == nil {
			t.Errorf("%s: raw SQL succeeded — the structural wall is advisory (S08.2/S08.4)", name)
		}
	}

	// The one sanctioned mutation path still works: first-N counts DOWN.
	if _, err := f.store.RecordSupervisedReview(ctx, "alice", v.ID); err != nil {
		t.Fatalf("sanctioned decrement failed: %v", err)
	}
}
