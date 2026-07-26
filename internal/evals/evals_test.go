package evals_test

import (
	"context"
	"encoding/json"
	"errors"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/conformance"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/evals"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/verify"
)

// ── migration + floor registry (rubric 9, 19) ──

func TestMigrationContiguousUserVersion12(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	v, err := f.db.UserVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v != 12 {
		t.Fatalf("user_version = %d, want 12 (migration 0012 applied contiguously)", v)
	}
	for _, table := range []string{"eval_floors", "revalidation_stamps"} {
		var n int
		if err := f.db.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil {
			t.Fatalf("%s not queryable (migration did not create it): %v", table, err)
		}
	}
}

func TestSeedFloorsAreMeasurementDerivedAndGatePending(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	n, err := f.store.EnsureFloorsRegistered(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("registered %d floors, want 1 (the rubric-software floor)", n)
	}
	// Re-running is a no-op: registration is insert-once, so a ratification or a
	// later measurement is never overwritten by a boot.
	again, err := f.store.EnsureFloorsRegistered(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if again != 0 {
		t.Fatalf("re-registration created %d floors, want 0 (insert-once)", again)
	}

	r := verify.SeedSoftwareRubric()
	fl, err := f.store.Floor(ctx, r.ID, r.Version)
	if err != nil {
		t.Fatal(err)
	}
	// The floor IS the measured Wilson 95% lower bound, strictly below the
	// measured point estimate — a point-estimate floor would red the rubric on
	// any single miss at n=20 (OQ5(a)).
	if fl.CatchFloor != 0.84 {
		t.Errorf("catch floor = %v, want 0.84 (Wilson 95%% lower bound of 20/20)", fl.CatchFloor)
	}
	if r.GoldenSet.TPR == nil || fl.CatchFloor >= *r.GoldenSet.TPR {
		t.Errorf("the floor must sit strictly below the measured rate (%v)", r.GoldenSet.TPR)
	}
	if fl.TNRReportOnly == nil || *fl.TNRReportOnly != 0.19 {
		t.Errorf("TNR companion = %v, want 0.19 report-only", fl.TNRReportOnly)
	}
	if fl.Ratified {
		t.Error("seed floors must enter gate-PENDING (operator ratification is a B5-gate item)")
	}
	if !strings.Contains(fl.Basis, "2026-07-22") || !strings.Contains(fl.Basis, "Wilson") {
		t.Errorf("floor basis does not name its measurement: %q", fl.Basis)
	}
	if err := f.store.Ratify(ctx, r.ID, r.Version); err != nil {
		t.Fatalf("Ratify: %v", err)
	}
	if fl, err = f.store.Floor(ctx, r.ID, r.Version); err != nil || !fl.Ratified {
		t.Fatalf("ratification did not stick: %+v %v", fl, err)
	}
}

func TestFloorRowIsImmutableExceptRatification(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.registerFloors(t)
	r := verify.SeedSoftwareRubric()

	// A raw-SQL holder cannot dial a registered floor down or delete it: a floor
	// that could move after the fact would falsify nothing.
	for _, stmt := range []string{
		`UPDATE eval_floors SET catch_floor = 0.1`,
		`UPDATE eval_floors SET basis = 'because I say so'`,
		`DELETE FROM eval_floors`,
	} {
		if err := f.rawSQL(ctx, stmt); err == nil {
			t.Errorf("%q succeeded — the floor row must be immutable (Spec S14.8 ¶2)", stmt)
		}
	}
	fl, err := f.store.Floor(ctx, r.ID, r.Version)
	if err != nil || fl.CatchFloor != 0.84 {
		t.Fatalf("floor moved after the attempted rewrites: %+v %v", fl, err)
	}
	// Registering a second floor for the SAME version is refused: re-flooring is
	// a new asset version.
	err = f.store.RegisterFloor(ctx, evals.Floor{
		AssetKind: evals.AssetRubric, AssetID: r.ID, AssetVersion: r.Version,
		CatchFloor: 0.1, Basis: "a second floor",
	})
	if !errors.Is(err, evals.ErrFloorRegistered) {
		t.Fatalf("second floor for the same version: %v, want ErrFloorRegistered", err)
	}
}

// ── rubric falsifiability (rubric 10; Spec S14.8 ¶5) ──

func TestFloorCheckRedsBelowFloorRubric(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.registerFloors(t)
	r := verify.SeedSoftwareRubric()

	// Non-tautological: the SAME catch rate is green above the floor and red
	// below it, so the verdict comes from the registered floor and nothing else.
	green, err := f.store.CheckFloor(ctx, r.ID, r.Version, 0.90)
	if err != nil {
		t.Fatal(err)
	}
	if !green.Green || !green.Registered {
		t.Fatalf("0.90 against floor 0.84 must be green: %+v", green)
	}
	red, err := f.store.CheckFloor(ctx, r.ID, r.Version, 0.70)
	if err != nil {
		t.Fatal(err)
	}
	if red.Green {
		t.Fatalf("0.70 against floor 0.84 must be RED: %+v", red)
	}
	if !strings.Contains(red.Reason, "BELOW") {
		t.Errorf("the red reason must say so plainly: %q", red.Reason)
	}

	// An unregistered version is not evaluable, so it is not green either
	// (BENCH-REG §10.1(d): the gate cannot open while a suite lacks its floor).
	none, err := f.store.CheckFloor(ctx, r.ID, r.Version+1, 1.0)
	if err != nil {
		t.Fatal(err)
	}
	if none.Registered || none.Green {
		t.Fatalf("an unfloored version must be neither registered nor green: %+v", none)
	}
}

// ── recording (rubric 6; Spec S14.2 family 14) ──

func TestRecordedResultCarriesContractMinimum(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	ran := time.Date(2026, 7, 26, 9, 0, 0, 0, time.UTC)
	err := f.store.RecordAsset(ctx, evals.AssetResult{
		AssetKind: evals.AssetRubric, AssetID: "rubric-software", AssetVersion: "v2",
		Suite: "golden-software@v1", Runner: evals.RunnerRider1, RunnerVersion: "golden-software@v1",
		Metrics: map[string]any{"planted_defect_catch": 1.0}, Passed: true, RanAt: ran,
	})
	if err != nil {
		t.Fatalf("RecordAsset: %v", err)
	}

	var payload string
	if err := f.db.QueryRowContext(ctx,
		`SELECT payload FROM run_events WHERE type = ? ORDER BY event_seq DESC LIMIT 1`,
		conformance.EventScoreRecorded).Scan(&payload); err != nil {
		t.Fatalf("no eval.score_recorded event was appended: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(payload), &got); err != nil {
		t.Fatal(err)
	}
	// The S14.2 contract minimum, verbatim: suite id + version, asset id +
	// version, metrics, runner + runner version.
	for _, key := range []string{"suite_id", "suite_version", "asset_id", "asset_version",
		"metrics", "runner", "runner_version", "result"} {
		if _, ok := got[key]; !ok {
			t.Errorf("payload misses the contract-minimum field %q: %s", key, payload)
		}
	}
	if got["suite_id"] != conformance.RowRegressionSweep {
		t.Errorf("suite_id = %v, want the S14.8 recording home", got["suite_id"])
	}
	if got["asset_id"] != "rubric-software" || got["asset_version"] != "v2" {
		t.Errorf("the per-asset result must carry its asset: %s", payload)
	}
	if got["runner"] != evals.RunnerRider1 {
		t.Errorf("runner = %v, want the honest instrument identity", got["runner"])
	}

	// The row mirror moved in the SAME transaction (Spec S14.5).
	row := f.sweepRow(t)
	if row.LastResult != conformance.ResultGreen || !row.HasRun {
		t.Fatalf("the registry row did not mirror the result: %+v", row)
	}
}

func TestSweepAggregateRecordsTheVerdictLast(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	// A green asset, then a red one, then the aggregate: the row's RESTING
	// state must be the sweep verdict, not the last per-asset result.
	for _, r := range []evals.AssetResult{
		{AssetID: "a", AssetVersion: "v1", Suite: "s@v1", Runner: "r", RunnerVersion: "1", Passed: true},
		{AssetID: "b", AssetVersion: "v1", Suite: "s@v1", Runner: "r", RunnerVersion: "1", Passed: false},
		{AssetID: "c", AssetVersion: "v1", Suite: "s@v1", Runner: "r", RunnerVersion: "1", Passed: true},
	} {
		if err := f.store.RecordAsset(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.sweepRow(t).LastResult; got != conformance.ResultGreen {
		t.Fatalf("before the aggregate the row shows the last asset (%q) — the test premise", got)
	}
	if err := f.store.RecordSweep(ctx, evals.SweepResult{
		Assets: 3, RedAssets: []string{"b"}, Runner: "coordinator", RunnerVersion: evals.SweepSuiteVersion,
	}); err != nil {
		t.Fatalf("RecordSweep: %v", err)
	}
	row := f.sweepRow(t)
	if row.LastResult != conformance.ResultRed {
		t.Fatalf("last_result = %q, want red — one red asset reds the sweep (OQ2(a))", row.LastResult)
	}
	// The red row IS the inbox card's derivation input (the B5-4 mechanism); no
	// bare asks row is written for a run-less platform act.
	var asks int
	if err := f.db.QueryRowContext(ctx, `SELECT count(*) FROM asks`).Scan(&asks); err != nil {
		t.Fatal(err)
	}
	if asks != 0 {
		t.Errorf("a conformance red must never write an asks row (asks.run_id is NOT NULL)")
	}
}

func TestLocalBatteryRecordsThrough(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	// OQ6(a): the Go-native T15 battery stays the local-duty executor and its
	// SuiteResults RECORD THROUGH this surface — that record-through IS the
	// connective obligation S12.9 carries.
	err := f.store.RecordLocalSuite(ctx, evals.LocalSuiteResult{
		Duty: "intake-triage", ModelHash: "sha256:abc", EngineBuild: "b10085",
		BatteryVersion: "t15-v1", Total: 30, Passed: 28, TokensPerSec: 44.2,
		WilsonLo: 0.78, WilsonHi: 0.98, Green: true,
	})
	if err != nil {
		t.Fatalf("RecordLocalSuite: %v", err)
	}
	var payload string
	if err := f.db.QueryRowContext(ctx,
		`SELECT payload FROM run_events WHERE type = ? ORDER BY event_seq DESC LIMIT 1`,
		conformance.EventScoreRecorded).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{evals.RunnerBattery, "t15-v1", "b10085", "intake-triage"} {
		if !strings.Contains(payload, want) {
			t.Errorf("the battery record-through payload misses %q: %s", want, payload)
		}
	}
}

// ── sweep dueness (rubric 7, 17) ──

func TestSweepDuenessSurvivesRestart(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	start := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	f.at(start)

	st, err := f.store.SweepState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Due || st.HasRun {
		t.Fatalf("a never-run sweep is due: %+v", st)
	}
	// ⚙ eval.sweep_interval defaults to 3 months and is read by dotted key at
	// evaluation time — never a compiled-in interval.
	if st.Interval != 3*30*24*time.Hour {
		t.Fatalf("interval = %v, want the ⚙ eval.sweep_interval default (3 months)", st.Interval)
	}

	if err := f.store.RecordSweep(ctx, evals.SweepResult{
		Assets: 1, Runner: "coordinator", RunnerVersion: evals.SweepSuiteVersion, RanAt: start,
	}); err != nil {
		t.Fatal(err)
	}
	if st, err = f.store.SweepState(ctx); err != nil || st.Due {
		t.Fatalf("a just-run sweep is not due: %+v %v", st, err)
	}

	// A fresh Store over the SAME DB is a restart: dueness comes from stored
	// state, so the answer is identical — no in-memory ticker exists to lose.
	restarted := evals.NewStore(f.db, f.conf, f.reg)
	restarted.Now = func() time.Time { return start.Add(30 * 24 * time.Hour) }
	if st, err = restarted.SweepState(ctx); err != nil || st.Due {
		t.Fatalf("30 days after a run the sweep is still not due: %+v %v", st, err)
	}
	restarted.Now = func() time.Time { return start.Add(91 * 24 * time.Hour) }
	if st, err = restarted.SweepState(ctx); err != nil || !st.Due {
		t.Fatalf("91 days after a run the sweep is due again: %+v %v", st, err)
	}

	// The registry's own passive dueness surface reads the same ⚙ key through
	// its settings-backed cadence token, so the two can never disagree.
	f.conf.Now = restarted.Now
	if !f.sweepRow(t).Due {
		t.Error("the registry row disagrees with the S14.8 sweep state")
	}

	// Moving the ⚙ value moves dueness live.
	if err := f.reg.Set(ctx, settings.SetRequest{
		Key: "eval.sweep_interval", Value: json.RawMessage(`6`),
		Actor: settings.Actor{Kind: settings.ActorOperator, ID: "op"}, Reason: "widen the sweep",
	}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if st, err = restarted.SweepState(ctx); err != nil || st.Due {
		t.Fatalf("at ⚙ 6 months, 91 days is not yet due: %+v %v", st, err)
	}
}

// ── the eval-object binding (rubric 8; Spec S14.8 ¶2) ──

func TestEvalObjectBindingResolvesExistingArtifacts(t *testing.T) {
	cat := &evals.Catalog{}
	r := verify.SeedSoftwareRubric()
	obj, err := cat.Rubric(r)
	if err != nil {
		t.Fatal(err)
	}
	set := verify.SeedGoldenSet()
	if obj.AssetID != r.ID || obj.AssetVersion != r.Version {
		t.Fatalf("rubric binding = %s v%d, want %s v%d", obj.AssetID, obj.AssetVersion, r.ID, r.Version)
	}
	if obj.GoldenSet.ID != set.ID || obj.GoldenSet.Version != set.Version {
		t.Fatalf("golden set = %v, want the in-tree %s v%d", obj.GoldenSet, set.ID, set.Version)
	}
	// The 20 planted cases ARE the planted-defect suite; the 6 clean controls
	// are the TNR controls. No new case content exists.
	if obj.PlantedCases != 20 || obj.CleanCases != 6 || obj.GoldenCases != 26 {
		t.Fatalf("case counts = %d planted / %d clean / %d total, want 20/6/26",
			obj.PlantedCases, obj.CleanCases, obj.GoldenCases)
	}
	if obj.PlantedSuite != obj.GoldenSet {
		t.Error("at v0 the planted-defect suite is the golden set's planted half")
	}

	// A worker version's S08.1 hooks resolve to concrete (set, version) pairs.
	cat.WorkerHooks = func(context.Context, string) (evals.WorkerAsset, error) {
		return evals.WorkerAsset{
			TemplateID: "wt-1", VersionID: "wv-1", Version: 3,
			Hooks: evals.EvalHooks{GoldenSetRef: "golden-software@v1", PlantedDefectRef: "golden-software"},
		}, nil
	}
	wobj, err := cat.WorkerVersion(context.Background(), "wv-1", verify.DomainSoftware)
	if err != nil {
		t.Fatal(err)
	}
	if wobj.AssetID != "wt-1" || wobj.AssetVersion != 3 || wobj.GoldenSet.ID != set.ID {
		t.Fatalf("worker binding = %+v", wobj)
	}

	// A dangling ref never resolves to something invented.
	cat.WorkerHooks = func(context.Context, string) (evals.WorkerAsset, error) {
		return evals.WorkerAsset{TemplateID: "wt-2", VersionID: "wv-2", Version: 1,
			Hooks: evals.EvalHooks{GoldenSetRef: "golden-nonexistent"}}, nil
	}
	if _, err := cat.WorkerVersion(context.Background(), "wv-2", verify.DomainSoftware); !errors.Is(err, evals.ErrUnknownEvalSet) {
		t.Fatalf("dangling ref: %v, want ErrUnknownEvalSet", err)
	}
}

// ── per-slice comparison (rubric 12; S08.10(a)) ──

func TestAggregateGreenIsInsufficient(t *testing.T) {
	// The planted-defect guard regresses while every other slice improves: the
	// asset must still be RED (a mean would have called this an improvement).
	cmp := evals.Compare([]evals.Slice{
		{Name: "planted_defect_catch", Baseline: 1.0, Measured: 0.8, Guard: true},
		{Name: "task_completion", Baseline: 0.5, Measured: 0.99},
		{Name: "task_latency_score", Baseline: 0.5, Measured: 0.99},
	})
	if cmp.Green {
		t.Fatal("a regressed slice guard must red the asset even when the averages improve")
	}
	if !strings.Contains(cmp.Reason, "slice guard") {
		t.Errorf("the reason must name the slice guard: %q", cmp.Reason)
	}
	// Report-only slices are recorded and never red the asset (OQ5(a): an
	// over-strict judge is the safe direction for a quality gate).
	cmp = evals.Compare([]evals.Slice{
		{Name: "planted_defect_catch", Baseline: 1.0, Measured: 1.0, Guard: true},
		{Name: "clean_control_pass", Baseline: 0.5, Measured: 0.1, ReportOnly: true},
	})
	if !cmp.Green {
		t.Fatalf("a report-only slice must never red the asset: %s", cmp.Reason)
	}
	if cmp.Metrics()["clean_control_pass"] != 0.1 {
		t.Error("a report-only slice is still recorded")
	}
	// A registered floor reds a slice that did not regress but is below floor.
	cmp = evals.Compare([]evals.Slice{
		{Name: "planted_defect_catch", Baseline: 0.5, Measured: 0.6, Floor: 0.84, HasFloor: true, Guard: true},
	})
	if cmp.Green {
		t.Fatal("above baseline but below the registered floor must be red")
	}
}

// ── import wall (rubric 16) ──

const modulePath = "github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/"

// bannedImports are the packages internal/evals must never import. It is the
// S14.8 executor: it records through the S14.5 registry and reads the in-tree
// eval artifacts, but the worker/local/stage surfaces it drives ride func seams
// injected at the shell root.
var bannedImports = map[string]string{
	"internal/worker": "flag + green-stamp verbs ride func seams (the FlagByModelFunc precedent)",
	"internal/local":  "the T15 battery records THROUGH this package, never imports either way",
	"internal/stage":  "the P-T06-5 block is consulted by stage, not the other way round",
	"internal/api":    "the red→card derivation is the api's projection, never an import",
	"internal/gates":  "the quality gate is structurally separate from the effects gate",
}

// forbiddenImport reports whether an import path is a banned package or any
// SUBPACKAGE of one — internal/local/battery is internal/local for wall
// purposes — and why. The match is exact-or-prefix on the module-relative path,
// never a suffix: a suffix match would let a subpackage through and would trip
// on an unrelated package that merely ends in the same letters.
func forbiddenImport(path string) (string, bool) {
	rel, ok := strings.CutPrefix(path, modulePath)
	if !ok {
		return "", false
	}
	for bad, why := range bannedImports {
		if rel == bad || strings.HasPrefix(rel, bad+"/") {
			return why, true
		}
	}
	return "", false
}

func TestImportWall(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, filepath.Join(repoRoot, "internal/evals"), func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	files := 0
	for _, pkg := range pkgs {
		for name, file := range pkg.Files {
			files++
			for _, imp := range file.Imports {
				path := strings.Trim(imp.Path.Value, `"`)
				if why, bad := forbiddenImport(path); bad {
					t.Errorf("%s imports %s — forbidden: %s", filepath.Base(name), path, why)
				}
			}
		}
	}
	if files == 0 {
		t.Fatal("the import wall scanned no files — it would pass vacuously")
	}
}

// TestImportWallCatchesSubpackages proves the wall FAILS when fed a forbidden
// import, including a subpackage of a banned package (the non-tautology
// precedent): a wall that cannot fail guarantees nothing.
func TestImportWallCatchesSubpackages(t *testing.T) {
	for _, path := range []string{
		modulePath + "internal/local",
		modulePath + "internal/local/battery",
		modulePath + "internal/worker",
		modulePath + "internal/worker/automation",
		modulePath + "internal/stage",
		modulePath + "internal/api",
		modulePath + "internal/gates",
	} {
		if _, bad := forbiddenImport(path); !bad {
			t.Errorf("the wall let %s through", path)
		}
	}
	// Negative controls: the allowed edges and a package whose name merely
	// starts with a banned one must NOT trip it.
	for _, path := range []string{
		modulePath + "internal/verify",
		modulePath + "internal/conformance",
		modulePath + "internal/storage",
		modulePath + "internal/lockfile",
		modulePath + "internal/localization",
		"strings",
	} {
		if why, bad := forbiddenImport(path); bad {
			t.Errorf("the wall wrongly rejected %s: %s", path, why)
		}
	}
}

func TestVerifyGainsNoNewImports(t *testing.T) {
	// The P-T06-5 gate function is PURE over existing verify types (OQ3(a)):
	// internal/verify must gain no internal import from this packet.
	src, err := os.ReadFile(filepath.Join(repoRoot, "internal/verify/judgegate.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), "Sinet-Agentic-Control-Hub/internal/") {
		t.Error("the judge gate must import no internal package (CONVENTIONS §15)")
	}
}
