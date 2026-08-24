package shell

// zai_ln2b_test.go — P3-LN-2B drain r1 D7 + D3 (S03.6, S08.8, S10.4).
//
// The composition root is where a lane document becomes routing, coverage and
// dispatch inputs. Every derivation here carried a load-bearing claim and none
// of them was tested, so this file tests them through the real functions.

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/opencode"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

func seedLanes(t *testing.T) []opencode.LaneConfig {
	t.Helper()
	lanes, err := opencode.SeedLaneConfigs()
	if err != nil {
		t.Fatalf("SeedLaneConfigs: %v", err)
	}
	if len(lanes) == 0 {
		t.Fatal("the platform ships no lane documents")
	}
	return lanes
}

// commissionedZAI is what the key ceremony leaves behind: one person holding
// the zai provider entry.
// It selects by LANE NAME, never by position: the seed set is sorted by lane
// name and a second document landed at P3-LN-3, so lanes[0] would silently
// commission whichever lane the alphabet put first while claiming to test zai.
func commissionedZAI(t *testing.T, lanes []opencode.LaneConfig) map[string]opencode.ProviderConfig {
	t.Helper()
	return map[string]opencode.ProviderConfig{"alice": laneByName(t, lanes, adapters.LaneZAI).Providers()}
}

// D7 · The four composition-root derivations, in both states that matter.
func TestLaneDerivationsAtTheCompositionRoot(t *testing.T) {
	lanes := seedLanes(t)
	live := commissionedZAI(t, lanes)

	// NOTHING commissioned — every derivation is empty, which is what keeps
	// the whole second-lane path inert at v0.
	if got := commissionedLanes(lanes, nil); len(got) != 0 {
		t.Errorf("commissionedLanes(nothing) = %v, want none", got)
	}
	if got := laneSubstrates(lanes, nil); got != nil {
		t.Errorf("laneSubstrates(nothing) = %v, want nil — an uncommissioned lane cannot be dispatched to", got)
	}
	if got := laneAlternateSeats(lanes, nil); got != nil {
		t.Errorf("laneAlternateSeats(nothing) = %v, want nil — an uncommissioned lane is seated by nothing", got)
	}

	// The configured model list is supplied REGARDLESS of commissioning: it is
	// the config side of the P-T17-3 diff, and a canary that is later armed
	// must already know what it diffs against.
	zai := laneByName(t, lanes, adapters.LaneZAI)
	models := laneConfiguredModels(lanes)
	got := models[adapters.LaneZAI]
	if len(got) != len(zai.Models) {
		t.Fatalf("configured models = %v, want all %d of the document's", got, len(zai.Models))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Errorf("configured models are not sorted (%v) — an unstable list diffs against itself", got)
		}
	}
	declared := map[string]bool{}
	for _, m := range zai.Models {
		declared[m.ID] = true
	}
	for _, id := range got {
		if !declared[id] {
			t.Errorf("configured model %q is not in the lane document — the config side must come from the document", id)
		}
	}

	// COMMISSIONED — the lane becomes coverage, a substrate and a seat.
	if got := commissionedLanes(lanes, live); len(got) != 1 || got[0] != adapters.LaneZAI {
		t.Errorf("commissionedLanes = %v, want [zai]", got)
	}
	subs := laneSubstrates(lanes, live)
	if subs[adapters.LaneZAI] != adapters.SubstrateOpencode {
		t.Errorf("laneSubstrates = %v, want zai→opencode (the document's own substrate)", subs)
	}
	if _, ok := subs[adapters.LaneAnthropic]; ok {
		t.Error("the anthropic lane appears in the lane→substrate map — its substrate is the ceremony default and stays there")
	}
	seats := laneAlternateSeats(lanes, live)
	exec := seats[worker.DutyExecution]
	if len(exec) != 1 {
		t.Fatalf("execution alternates = %v, want exactly one zai seat", exec)
	}
	if exec[0].Lane != adapters.LaneZAI || exec[0].Model != zai.DefaultModel {
		t.Errorf("seat = %+v, want the document's own lane and default model %q", exec[0], zai.DefaultModel)
	}
	if exec[0].WindowTokens <= 0 {
		t.Error("the composed seat carries no context window")
	}
	// Planning and judge are deliberately not seated on a second lane.
	for _, duty := range []string{worker.DutyPlanning, worker.DutyJudge} {
		if len(seats[duty]) != 0 {
			t.Errorf("duty %q gained a second-lane seat — no zai model has been measured against the B3/S07.5 bars", duty)
		}
	}
}

// pressureEnv is a real database with the real gauge behind routePressure.
type pressureEnv struct {
	db  *storage.DB
	reg *settings.Registry
	rp  routePressure
}

func newPressureEnv(t *testing.T) *pressureEnv {
	t.Helper()
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	log := eventlog.New(db, reg)
	runs := run.NewStore(db, log)
	cps := gates.NewCheckpoints(db, log)

	if err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO users (user_id, display_name, role, created_ts) VALUES (?, ?, 'member', ?)`,
			"alice", "alice", time.Now().UTC().Format(time.RFC3339Nano))
		return err
	}); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	for _, lane := range []struct{ id, lane, substrate, model string }{
		{"r-a", adapters.LaneAnthropic, adapters.SubstrateClaudeCLI, "claude-sonnet-5"},
		{"r-z", adapters.LaneZAI, adapters.SubstrateOpencode, "glm-5.3"},
	} {
		if _, err := runs.Create(ctx, run.NewRun{ID: lane.id, UserID: "alice", Lane: lane.lane, Substrate: lane.substrate}); err != nil {
			t.Fatalf("create %s: %v", lane.id, err)
		}
		for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
			if _, err := runs.Transition(ctx, lane.id, st, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
				t.Fatalf("transition %s: %v", lane.id, err)
			}
		}
		if _, err := cps.Write(ctx, gates.NewCheckpoint{
			RunID: lane.id, ModelID: lane.model,
			Usage:            []byte(`{"input_tokens":1000,"output_tokens":500}`),
			SessionSubstrate: lane.substrate, SessionID: "s-" + lane.id,
		}); err != nil {
			t.Fatalf("checkpoint %s: %v", lane.id, err)
		}
	}
	return &pressureEnv{db: db, reg: reg,
		rp: routePressure{g: metering.NewPressureGauge(db, reg), b: metering.NewBudgets(db)}}
}

// D3 · The PRODUCTION pressure reader returns a normalized ratio, per lane, in
// that lane's own unit — never a raw lifetime count.
func TestRoutePressureIsNormalizedPerLane(t *testing.T) {
	e := newPressureEnv(t)
	ctx := context.Background()

	// With no declared budget, neither lane offers a comparable figure — and
	// says so, rather than returning an unbounded total that would hand every
	// dispatch to whichever lane was added most recently.
	for _, lane := range []string{adapters.LaneAnthropic, adapters.LaneZAI} {
		p, err := e.rp.Pressure(ctx, "alice", lane)
		if err != nil {
			t.Fatalf("Pressure(%s): %v", lane, err)
		}
		if p.Applicable {
			t.Errorf("lane %s claims a pressure ratio with no declared budget (S10.4/D4)", lane)
		}
		if p.Ratio != 0 {
			t.Errorf("lane %s ratio = %v with no budget, want 0 — a raw consumption total is not a ratio", lane, p.Ratio)
		}
	}

	// The lanes answer from DIFFERENT gauges, in their own units.
	zai, err := e.rp.Pressure(ctx, "alice", adapters.LaneZAI)
	if err != nil {
		t.Fatalf("Pressure(zai): %v", err)
	}
	if zai.Unit != "credits" {
		t.Errorf("zai unit = %q, want credits — the plan meters in its own unit", zai.Unit)
	}
	anth, err := e.rp.Pressure(ctx, "alice", adapters.LaneAnthropic)
	if err != nil {
		t.Fatalf("Pressure(anthropic): %v", err)
	}
	if anth.Unit == zai.Unit {
		t.Errorf("both lanes report unit %q — comparing a token against a credit is what the ratio exists to avoid", anth.Unit)
	}

	// Declare a budget on the anthropic lane: NOW there is a ratio, and it is
	// consumption over that budget — bounded, and comparable across lanes.
	if _, _, err := metering.NewBudgets(e.db).Declare(ctx, metering.BudgetRow{
		UserID: "alice", Lane: adapters.LaneAnthropic, PeriodTokens: 6000,
		PeriodStart: time.Now().UTC().Add(-time.Hour), PeriodDays: 7,
		DeclaredTS: time.Now().UTC(), DeclaredBy: "alice",
	}); err != nil {
		t.Fatalf("Declare: %v", err)
	}
	got, err := e.rp.Pressure(ctx, "alice", adapters.LaneAnthropic)
	if err != nil {
		t.Fatalf("Pressure(anthropic, declared): %v", err)
	}
	if !got.Applicable {
		t.Fatal("a declared budget did not make the anthropic ratio applicable")
	}
	if got.Ratio <= 0 || got.Ratio > 1 {
		t.Errorf("ratio = %v, want a bounded fraction of the declared budget (1500/6000)", got.Ratio)
	}
	if want := 1500.0 / 6000.0; got.Ratio != want {
		t.Errorf("ratio = %v, want %v (consumption ÷ the declared budget)", got.Ratio, want)
	}
}
