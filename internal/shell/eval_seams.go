package shell

import (
	"context"
	"fmt"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/conformance"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/evals"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// The Spec S14.8 regression-eval surface, composed at the root (B5-5).
//
// internal/evals never imports internal/worker: the flag surfaces (S08.10) and
// the green-stamp verb ride func seams satisfied here, exactly as the S12.10
// swap gate's FlagByModelFunc does (CONVENTIONS §28). Composition is wiring
// only — no package gains a forbidden edge.

// evalSurface holds the composed S14.8 machinery: the floor registry + sweep
// store and the revalidation runbook over it.
type evalSurface struct {
	Store   *evals.Store
	Runbook *evals.Runbook
	// FloorsRegistered counts floors newly registered at this boot.
	FloorsRegistered int
}

// buildEvalSurface composes the S14.8 surface and registers the seed floors.
//
// Floor registration attaches to the 8.3 knowledge-gate ENTRY POINT (Spec
// S14.8 ¶2; BENCH-REG §10.1(d)) — the same boot step that re-homes the B2 seed
// objects under S09.10 governance, not a second gate. Floors are platform data,
// so unlike the house-scope seeds they need no D10 holder and register
// unconditionally (the conformance-registry seeding posture). Registration is
// insert-once: an operator ratification or a later measurement is never
// overwritten by a boot.
func buildEvalSurface(ctx context.Context, db *storage.DB, conf *conformance.Store,
	reg *settings.Registry, work *worker.Store) (*evalSurface, error) {

	store := evals.NewStore(db, conf, reg)
	registered, err := store.EnsureFloorsRegistered(ctx)
	if err != nil {
		return nil, fmt.Errorf("shell: register S14.8 eval floors: %w", err)
	}
	return &evalSurface{
		Store:            store,
		FloorsRegistered: registered,
		Runbook: &evals.Runbook{
			Store:   store,
			Catalog: &evals.Catalog{WorkerHooks: workerEvalHooks(work)},
			Hooks: evals.Hooks{
				// An engine bump flags every version validated against the old
				// pin (S08.10(b) / P-T14-1); a watchlist drift finding naming a
				// model flags by model (S08.10(a)). A swap needs no hook: the
				// S12.10 swap gate already flagged before the alias went live,
				// and the runbook consumes that set.
				FlagByEnginePin: work.FlagByEnginePin,
				FlagByModel:     work.FlagByModel,
				Stamp:           evalStampHook(work),
			},
		},
	}, nil
}

// workerEvalHooks satisfies the S08.1 eval-hook lookup across the import wall.
func workerEvalHooks(work *worker.Store) evals.WorkerHooksFunc {
	return func(ctx context.Context, versionID string) (evals.WorkerAsset, error) {
		v, err := work.VersionByID(ctx, versionID)
		if err != nil {
			return evals.WorkerAsset{}, err
		}
		src, err := work.VersionSource(ctx, versionID)
		if err != nil {
			return evals.WorkerAsset{}, err
		}
		def, _, err := worker.ParseTemplate(src)
		if err != nil {
			return evals.WorkerAsset{}, err
		}
		return evals.WorkerAsset{
			TemplateID: v.TemplateID, VersionID: v.ID, Version: int(v.Version),
			Hooks: evals.EvalHooks{
				GoldenSetRef:     def.Eval.GoldenSetRef,
				PlantedDefectRef: def.Eval.PlantedDefectRef,
			},
		}, nil
	}
}

// evalStampHook satisfies the green-stamp seam with the worker-lifecycle verb.
func evalStampHook(work *worker.Store) func(context.Context, evals.Stamp) error {
	return func(ctx context.Context, st evals.Stamp) error {
		_, err := work.Revalidate(ctx, st.Actor, worker.RevalidationStamp{
			TemplateID: st.TemplateID, VersionID: st.VersionID,
			TriggerKind: st.TriggerKind, TriggerSubject: st.TriggerSubject,
			Suites: st.Suites, Baseline: st.Baseline, Green: st.Green,
		})
		return err
	}
}
