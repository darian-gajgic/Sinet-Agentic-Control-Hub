package intake

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// Pipeline is the S06 intake pipeline over the landed substrate. All model
// duties ride seams (S06.10); everything else is platform code. The
// pipeline operates on one intake run per task: stage work requires the
// run to be running (admission is the scheduler's, Spec S10; dev-mode
// harnesses walk the FSM), gates park it, answers resume it in place.
type Pipeline struct {
	DB       *storage.DB
	Log      *eventlog.Log
	Runs     *run.Store
	Ledger   *ledger.Store
	Settings Settings

	// ArtifactRoot is the durable artifact directory (per-task subdirs).
	ArtifactRoot string

	// Taxonomies are the active per-family question sets; families without
	// one fall back to FamilyGeneric (Spec S06.2: v0 ships software +
	// generic). Nil = SeedTaxonomies().
	Taxonomies map[Family]*Taxonomy
	// Triggers is the P47 rule file. Nil = SeedTriggers().
	Triggers *TriggerFile

	// Seams (Spec S06.10). Planner is required; Critic is required from
	// the standard tier up (critique is mandatory there — ErrSeamMissing
	// otherwise); the rest are optional.
	Planner    Planner
	Critic     Critic
	Classifier Classifier
	Registry   Registry
	Utility    Utility
	SpotCheck  SpotCheck
	// Phraser is the S06.5 phrase-and-summarize seat (P3-RW-12 R6/R7).
	// Optional: nil leaves every card carrying the taxonomy's own words.
	Phraser Phraser

	// Router is the S08.8 selection seam (B3-3): the approval card carries
	// the selected worker + plain-language reason PRE-execution, with
	// re-route/pin on the answer (route.go). Nil = test-only posture (no
	// routing block; the composition root always wires it).
	Router Router

	// Fingerprint supplies the current freshness fingerprint for the
	// approval staleness check (G1 Def.5; S02 owns the durable set). The
	// matched project (from the registry slice, "" when none) lets the probe
	// supply that project's real repo HEAD (Spec S13.7 feed; R33); members the
	// platform cannot honestly observe stay empty, never faked. Optional; nil
	// restricts staleness to the age trigger.
	Fingerprint func(ctx context.Context, project string) (run.Fingerprint, error)

	// CitedEntryVersions resolves the CURRENT versions of cited project-truth
	// knowledge entries (Spec S09.6): given entry keys, it returns {key:
	// version} for those currently ACTIVE — a key omitted from the result has
	// been superseded, removed, or retired, so mapDrift fires vanished-key
	// drift. Wired by the shell to the memory store; internal/intake never
	// imports internal/memory (the S09.1 capability wall — stage imports
	// intake, so the walls stay transitively clean, R32). Nil leaves cited
	// drift unmeasurable (the stored versions carry forward, no false drift).
	CitedEntryVersions func(ctx context.Context, keys []string) (map[string]string, error)

	// Leases renews the run's lease while this pipeline drives its advance
	// (Spec S02.2 lease block at ⚙ recovery.heartbeat; P3-RW-5 R3). Nil is
	// inert — an unwired pipeline behaves exactly as before.
	Leases *run.LeaseKeeper

	// Now is the clock seam (tests). Timestamps are recorded, never an
	// ordering authority (P-T07-4).
	Now func() time.Time
}

// leaseHolderIntake names the intake pipeline in the lease block while it is
// driving a run's advance.
const leaseHolderIntake = "intake-advance"

func (p *Pipeline) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

func (p *Pipeline) nowRFC3339() string { return p.now().UTC().Format(time.RFC3339Nano) }

func (p *Pipeline) store() artifactStore { return artifactStore{root: p.ArtifactRoot} }

func (p *Pipeline) taxonomies() map[Family]*Taxonomy {
	if p.Taxonomies != nil {
		return p.Taxonomies
	}
	return SeedTaxonomies()
}

func (p *Pipeline) triggers() *TriggerFile {
	if p.Triggers != nil {
		return p.Triggers
	}
	return SeedTriggers()
}

func (p *Pipeline) taxonomyFor(family Family) *Taxonomy {
	tax := p.taxonomies()
	if t, ok := tax[family]; ok {
		return t
	}
	return tax[FamilyGeneric]
}

// ---- Start: Stage 0 (Spec S06.2) ----

// Start receives a request: it creates the task and its intake run, runs the
// DETERMINISTIC half of triage (registry match, the P47 rule layer, the
// fail-closed baseline), persists the baseline intake record, and emits the
// Stage-0 ledger writes. No model runs here at all.
//
// The CLASSIFIER half is not Start's (classifyStep): its mandatory $0 D7 row
// must ride a checkpointable consuming run, and at Start the intake run neither
// exists nor runs — admission is the scheduler's and intake never self-admits
// (Spec S10, S02.3). Start therefore leaves the state marked TriagePending and
// the first advance classifies on the running run. Classification still
// precedes every paid seam by code order, so S06.2's ordering guarantee holds.
//
// Stage-0 floor observation: the spec pins the five floor classes, not a
// request-text cue lexicon — Stage-0 floors arrive from the registry and
// the classifier (add-only), and Stage 2(b) re-checks deterministically
// against the PLAN's declared contents, with D7 gating every outward
// effect regardless of what intake decided (Spec S06.2).
func (p *Pipeline) Start(ctx context.Context, req Request) (*State, error) {
	if req.UserID == "" {
		return nil, fmt.Errorf("intake: request without user (15.6 owner attribution)")
	}
	if req.Text == "" {
		return nil, fmt.Errorf("intake: empty request")
	}
	if req.TaskID == "" {
		req.TaskID = "t-" + randomHex(8)
	}

	st := &State{
		Phase:      PhaseInterview,
		TaskID:     req.TaskID,
		RunID:      req.TaskID + ".intake",
		Owner:      req.UserID,
		Req:        req,
		NeedsDraft: true,
		Tier:       TierHigh, // fail-closed baseline until classified (S06.2)
		Family:     FamilyGeneric,
		// Nothing has resolved the family yet. This baseline is what the family
		// question exists to replace on a non-band task (P3-RW-11 R5): "generic,
		// because nobody said otherwise" is never allowed to reach the interview
		// disguised as an answer.
		FamilySource: FamilySourceDefault,
		Guess:        Estimate{Known: false, Basis: "unclassified"},
		// The classify step is owed on this task (see classifyStep).
		TriagePending: true,
	}

	// Registry match (S1.6): injected so the interview never asks what the
	// platform already knows; danger zones feed the stakes floor.
	// A PINNED request never degrades (P3-RW-1; S15.2): the requester named a
	// specific project, so ANY failure to resolve it fails Start — a refusal
	// (mapped 4xx) and an infrastructure error (an ordinary internal error)
	// alike — and no task is born believing it got a project it did not.
	// Nothing durable exists yet at this point: the task, run and first state
	// event are written below. The UNPINNED scan keeps its S06.2 degrade
	// posture untouched — a no-match and a seam error both leave no slice.
	if p.Registry != nil {
		slice, ok, err := p.Registry.Match(ctx, req)
		switch {
		case err != nil && req.Project != "":
			return nil, err
		case err == nil && ok:
			st.Registry = &slice
			// VALIDATED AT THE BOUNDARY (P3-RW-11 drain D3). A seam hands over
			// whatever it has, and a family outside the six-value vocabulary is
			// not a family — it is a value that would key `taxonomyFor` to
			// nothing, silently take the generic set, and be ATTRIBUTED to the
			// registry while it did so. Treating it as unresolved keeps the one
			// rule this packet rests on: a family the platform cannot stand
			// behind is asked, never assumed. ValidFamily is false for "" too,
			// so absence and nonsense take the same honest path.
			if ValidFamily(slice.Family) {
				st.Family, st.FamilySource = slice.Family, FamilySourceRegistry
			}
		}
	}

	// Deterministic P47 rule layer FIRST (S06.3).
	st.addHits(p.triggers().Detect(req.Title + "\n" + req.Text))

	// Key the state to its family's question set, apply the registry's
	// pre-answered slots against THAT set, and recompute clearance. A family
	// with no seeded set yields the disclosure line recorded below.
	fallback := p.applyFamilyTaxonomy(st)

	// Intake record v1 — the durable Stage-0 artifact (S06.1 Stage 0), carrying
	// the honest pre-classification baseline and never values the platform does
	// not hold yet. The classify step writes v2 beside it.
	ref, err := p.writeRecord(st, 1)
	if err != nil {
		return nil, err
	}
	st.RecordRef = &ref

	// Task + run + first state event, one transaction: the task exists in
	// the D7 record from its first instant (CONVENTIONS §8 precedent).
	err = p.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO tasks (task_id, user_id, title, kanban_status, created_ts)
			 VALUES (?, ?, ?, 'intake', ?)`,
			st.TaskID, st.Owner, req.Title, p.nowRFC3339()); err != nil {
			return fmt.Errorf("intake: insert task: %w", err)
		}
		if _, err := p.Runs.CreateTx(ctx, tx, run.NewRun{
			ID: st.RunID, UserID: st.Owner, TaskID: st.TaskID,
		}, run.EventCreated, json.RawMessage(`{"reason":"intake run (S06.1)"}`)); err != nil {
			return err
		}
		return p.appendStateTx(ctx, tx, st)
	})
	if err != nil {
		return nil, err
	}

	// Stage-0 ledger emissions (S06.1: intake record → artifacts +
	// decisions). Sequential after the birth transaction; the state event
	// and record file are the durable Stage-0 record either way.
	gen, err := p.currentGen(ctx, st.RunID)
	if err != nil {
		return nil, err
	}
	verbs := p.Ledger.SessionVerbs(st.RunID, "triage", gen)
	if _, err := verbs.Artifact(ctx, ref.Path, "intake-record", "Stage-0 intake record (S06.2)", ref.SHA256); err != nil {
		return nil, err
	}
	// No triage DECISION line here: at Start the honest one would read
	// "high/generic/default", which states nothing. The decision is recorded
	// where the facts become facts — in the classify step (S06.1 Stage 0).
	if err := p.recordTaxonomyFallback(ctx, st, fallback); err != nil {
		return nil, err
	}
	return st, nil
}

// writeRecord persists one version of the Stage-0 intake record (S06.1 Stage 0)
// and returns its ref. Both versions carry the same key set: family_source
// rides it too, so "generic" on the v1 baseline is attributable rather than
// indistinguishable from a registry-declared generic (P3-RW-11 R5).
func (p *Pipeline) writeRecord(st *State, version int) (ArtifactRef, error) {
	record := map[string]any{
		"family": st.Family, "family_source": st.FamilySource,
		"stakes_tier": st.Tier, "floor_reasons": st.FloorReasons,
		"size_cost_guess": st.Guess, "data_bearing": st.DataBearing,
		"registry_slice_ref": registryRef(st.Registry), "band": st.Band,
		"taxonomy": st.TaxonomyID + "@" + st.TaxonomyVersion, "ts": p.nowRFC3339(),
	}
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return ArtifactRef{}, fmt.Errorf("intake: marshal intake record: %w", err)
	}
	return p.store().write(p.store().recordPath(st.TaskID, version), append(raw, '\n'), nil, version)
}

// classifyStep is the model half of Stage-0 triage (Spec S06.2 rules 1/3/4 and
// the S06.4 band), run ONCE at the top of the first advance.
//
// WHY HERE. The duty behind this seam must write a $0 D7 row on its consuming
// run (CONVENTIONS §26 R18), and that row is only writable while the run exists
// and is checkpointable. `Pipeline.Start` has neither: the intake run is being
// born there, and admission is the scheduler's (S10, the ErrNotRunning
// doctrine). The top of an advance is the one site where the pipeline holds a
// RUNNING consuming run — `st.RunID`, which after an AdvanceDispatched rebind is
// the fork — and it still precedes every paid seam in the drive, so S06.2's
// "classification before any paid model runs" holds by code order.
//
// The marker clears in EVERY branch: a failed classify is a completed triage
// attempt, not a per-advance retry loop. A crash between the duty call and the
// state append below leaves the marker pending, so a redispatch repeats one $0
// call and re-applies the same deterministic guards — accepted, because the
// alternative (clearing first) loses a crashed classification with no retry.
func (p *Pipeline) classifyStep(ctx context.Context, st *State) error {
	if !st.TriagePending {
		return nil
	}
	st.TriagePending = false

	classified := false
	if p.Classifier != nil {
		prop, err := p.Classifier.Classify(ctx, TriageInput{
			RunID: st.RunID, Request: st.Req, Registry: st.Registry,
		})
		if err == nil {
			classified = true
			// PRECEDENCE registry > classifier > requester-asked (P3-RW-11 OQ3):
			// the classifier adopts a family only over the DEFAULT baseline. A
			// registered project's family is its owner's standing declaration and
			// a per-request guess never overrules it; and by first-advance time a
			// requester-answered family can exist too (a re-entrant state), which
			// this must never overwrite either.
			//
			// VALIDATED HERE TOO (P3-RW-11 drain D3): the adapter maps an
			// out-of-vocabulary label to "" already, but this is the pipeline's own
			// boundary and a second Classifier implementation is a seam away. An
			// unresolved family leaves the question to the requester rather than
			// falling back to generic.
			if ValidFamily(prop.Family) && st.FamilySource == FamilySourceDefault {
				st.Family, st.FamilySource = prop.Family, FamilySourceClassifier
			}
			if ValidTier(prop.Tier) {
				st.Tier = prop.Tier
			}
			st.Guess = prop.Est
			st.addHits(prop.DataHits) // add-only
			st.addFloors(validFloors(prop.Floors))
			// Zero-interaction band (S06.4): all four conditions,
			// evaluated conservatively; the cost bound is per-user ⚙.
			if st.FloorTier == "" && prop.Tier == TierTrivial && prop.ReadOnly && !prop.NewNeeds && len(st.openDataBearing()) == 0 {
				if capUSD, err := p.Settings.FloatFor(keyZeroInteractionCost, st.Owner); err == nil && prop.Est.Known && prop.Est.USD < capUSD {
					st.Band = true
				}
			}
			if prop.Tier == TierTrivial && !st.Band {
				// Trivial IS the band; a trivial proposal failing the band
				// conditions lands at low (S06.4).
				st.Tier = maxTier(TierLow, st.FloorTier)
			}
		}
	}
	if !classified {
		// nil seam, seam error, abstain, invalid label: high stakes, family left
		// unresolved for the S06.5 question (S06.2).
		st.Tier = TierHigh
	}

	// The family may have moved (generic → software), so the question set is
	// re-keyed and the registry's pre-answered slots re-applied against THAT set
	// — otherwise answers the platform already holds would be asked for again.
	fallback := p.applyFamilyTaxonomy(st)

	// Record v2 carries the classified values: a NEW immutable file beside v1,
	// never an overwrite (S06.6). File first, then the single state append, then
	// the ledger — the ledger never nests inside an intake WriteTx (the Start
	// precedent; single-connection pool).
	ref, err := p.writeRecord(st, 2)
	if err != nil {
		return err
	}
	st.RecordRef = &ref
	if err := p.appendState(ctx, st); err != nil {
		return err
	}

	gen, err := p.currentGen(ctx, st.RunID)
	if err != nil {
		return err
	}
	verbs := p.Ledger.SessionVerbs(st.RunID, "triage", gen)
	if _, err := verbs.Artifact(ctx, ref.Path, "intake-record",
		"Stage-0 intake record after classification (S06.2)", ref.SHA256); err != nil {
		return err
	}
	if _, err := p.Ledger.RecordDecision(ctx, st.RunID, ledger.AuthorPlatform, run.ActorPlatform, "triage",
		fmt.Sprintf("triage: family=%s (%s) tier=%s band=%v data-bearing=%d clearance=%.0f%%",
			st.Family, st.FamilySource, st.Tier, st.Band, len(st.DataBearing), st.Clearance),
		"Stage-0 triage classification (S06.2)", 0); err != nil {
		return err
	}
	return p.recordTaxonomyFallback(ctx, st, fallback)
}

// applyFamilyTaxonomy keys the state to its family's question set, re-applies
// the registry's pre-answered slots against THAT set, and recomputes clearance.
//
// It is called at Start AND after a family swap, because both are the same act:
// the registry may have pre-answered slots that only exist in one family's
// taxonomy, so a swap that did not re-apply them would throw away answers the
// platform already held and ask the requester for them again.
//
// It returns the DISCLOSURE line for a family whose question set is not seeded
// yet — the generic set stands in while `st.Family` keeps the true family — or
// "" when the family's own set was used. The fallback is stated, never
// performed silently (P3-RW-11 R6): the state's TaxonomyID/Family divergence is
// the durable witness and this line is the readable one.
func (p *Pipeline) applyFamilyTaxonomy(st *State) string {
	tax := p.taxonomyFor(st.Family)
	st.TaxonomyID, st.TaxonomyVersion = tax.ID, tax.Version
	if st.Registry != nil {
		for slotID, val := range st.Registry.ResolvedSlots {
			if tax.Slot(slotID) != nil {
				st.resolveSlot(SlotResolution{SlotID: slotID, How: ResolvedRegistry, Value: val})
			}
		}
	}
	st.Clearance = tax.Clearance(st.resolvedSet())
	if _, seeded := p.taxonomies()[st.Family]; seeded {
		return ""
	}
	return fmt.Sprintf("no question set is seeded for the %s family yet: the interview uses the generic set, "+
		"while the task's family stays %s for planning and routing (S06.5)", st.Family, st.Family)
}

// recordTaxonomyFallback writes the disclosure as a platform decision. A no-op
// for the empty line, so callers need no condition of their own.
func (p *Pipeline) recordTaxonomyFallback(ctx context.Context, st *State, line string) error {
	if line == "" {
		return nil
	}
	_, err := p.Ledger.RecordDecision(ctx, st.RunID, ledger.AuthorPlatform, run.ActorPlatform, "triage",
		line, "question-set fallback disclosed rather than performed silently (S06.5)", 0)
	return err
}

func validFloors(in []FloorReason) []FloorReason {
	valid := map[string]bool{
		FloorOutwardEffect: true, FloorNewSpend: true, FloorCredentialTouch: true,
		FloorSharedAssetWrite: true, FloorRegulatedDomain: true,
	}
	var out []FloorReason
	for _, f := range in {
		if valid[f.Class] {
			out = append(out, f)
		}
	}
	return out
}

// projectOf returns the matched registry project id, or "" when no registry
// entry matched (the fingerprint probe then supplies no repo HEAD — never a
// faked one, R33).
func projectOf(r *RegistrySlice) string {
	if r != nil {
		return r.Project
	}
	return ""
}

func registryRef(r *RegistrySlice) string {
	if r == nil {
		return ""
	}
	return r.Ref
}

// ---- Advance: the stage driver ----

// Advance drives the pipeline until it blocks on a gate or reaches a
// terminal phase, and returns the current state. Stage work requires the
// intake run to be running (ErrNotRunning otherwise).
func (p *Pipeline) Advance(ctx context.Context, taskID string) (*State, error) {
	st, err := p.LoadState(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return p.advanceLoaded(ctx, st)
}

// AdvanceDispatched is the entry the DISPATCH leg uses: it names the run the
// scheduler just claimed and drove to running, and the pipeline REBINDS its
// state onto that run before advancing (Spec S02.5 step 2 — the fork
// "supersedes" its parent; S02.3 supersession).
//
// This exists because the pipeline's state carries the run id it was born with
// (`<task>.intake`, composed once at birth). A recovery fork is a DIFFERENT run
// row, so a dispatch that drove the pipeline by task id alone left the pipeline
// checking, holding the lease of, and completing the CRASHED PARENT — the
// advance died with ErrNotRunning, the ladder re-forked, and the lineage burned
// to a tombstone with everything needed to heal already durable (the live-world
// t-80eff shape). The rebind is the whole of the fix; everything downstream
// already reads `st.RunID`.
//
// The rebind happens BEFORE any advance decision, including the terminal-phase
// and open-gate early returns — a fork dispatched over an already-approved
// state must complete ITSELF, and a fork dispatched over an open gate must park
// on that gate rather than sit running-idle for the ladder to reap again.
func (p *Pipeline) AdvanceDispatched(ctx context.Context, taskID, runID string) (*State, error) {
	st, err := p.LoadState(ctx, taskID)
	if err != nil {
		return nil, err
	}
	rebound, err := p.rebind(ctx, st, runID)
	if err != nil {
		return nil, err
	}
	if rebound && st.OpenAskID != "" {
		return st, nil // the rebind parked it on the gate it inherited
	}
	return p.advanceLoaded(ctx, st)
}

// rebind moves the pipeline state's identity onto the dispatched run when that
// run supersedes the state's current one, and reports whether it did.
//
// The guard is the S02.5 step-3 LINEAGE, not the id's shape: walking the
// dispatched run's `parent_run_id` chain (migration 0002) must reach the
// state's current run, and both must belong to the same task. A same-id
// dispatch is a no-op (the ordinary, non-forked path performs no write at all —
// R13). Anything else is refused loudly with ErrLineage: a rebind onto an
// unrelated run would hand one task's interview to another run's identity, and
// the dispatch leg's crash path is the right place for that to surface.
//
// The multi-hop case is real: if a fork crashed before its own rebind
// committed, the state still names an ANCESTOR of the run being dispatched.
func (p *Pipeline) rebind(ctx context.Context, st *State, runID string) (bool, error) {
	if runID == "" || runID == st.RunID {
		return false, nil
	}
	r, err := p.Runs.Get(ctx, runID)
	if err != nil {
		return false, err
	}
	if r.TaskID != st.TaskID {
		return false, fmt.Errorf("%w: run %q belongs to task %q, not %q", ErrLineage, runID, r.TaskID, st.TaskID)
	}
	if err := p.lineageReaches(ctx, r, st.RunID); err != nil {
		return false, err
	}
	from := st.RunID
	st.RunID = runID
	// ONE transaction, because these are ONE fact — the fork takes over: the
	// rebind state event lands on the FORK at the fork's current generation
	// (appendStateTx reads it in-tx — CONVENTIONS §13: control-plane acts append
	// at the run's current generation); an open ask row re-points to the fork in
	// the same commit so `asks.run_id` never names a superseded run (recovery's
	// step-5 ask join excludes asks on terminal runs); and, when a gate IS open,
	// the fork parks on it in the same commit.
	//
	// The park is S06.1's "gates wait" applied to a run that inherited a gate:
	// the dispatch leg already took this run to running, and a running run
	// nobody drives is exactly the corpse the next sweep reaps. NOT a kill — the
	// run is ours, it is idle by construction, and parked is the ratified wait
	// state (NO-AUTO-KILL, S14.4 / G1 D1.3).
	err = p.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		if st.OpenAskID != "" {
			if _, err := tx.ExecContext(ctx,
				`UPDATE asks SET run_id = ? WHERE ask_id = ? AND status = 'open'`,
				st.RunID, st.OpenAskID); err != nil {
				return fmt.Errorf("intake: re-point ask %q: %w", st.OpenAskID, err)
			}
		}
		if err := p.appendStateTx(ctx, tx, st); err != nil {
			return err
		}
		if st.OpenAskID == "" {
			return nil
		}
		_, err := p.Runs.TransitionTx(ctx, tx, st.RunID, run.StateParked, run.TransitionOptions{
			Reason: fmt.Sprintf("intake gate open on the superseding run: %s — gates wait (S06.1)", st.OpenAskKind),
			Actor:  run.ActorPlatform,
		})
		return err
	})
	if err != nil {
		st.RunID = from // the in-memory state never outruns the durable record
		return false, err
	}
	return true, nil
}

// lineageReaches walks the dispatched run's parent chain until it reaches
// target (Spec S02.5 step 3: a successor's `parent_run_id` names the run it
// supersedes). Each hop must be a real row; the walk is bounded by
// ⚙ recovery.max_attempts' worth of generations several times over, so a cyclic
// or absurd chain refuses rather than spins.
func (p *Pipeline) lineageReaches(ctx context.Context, from run.Run, target string) error {
	const maxHops = 64
	cur := from
	for hop := 0; hop < maxHops; hop++ {
		if cur.ParentRunID == "" {
			return fmt.Errorf("%w: %q has no lineage reaching %q", ErrLineage, from.ID, target)
		}
		if cur.ParentRunID == target {
			return nil
		}
		next, err := p.Runs.Get(ctx, cur.ParentRunID)
		if err != nil {
			return fmt.Errorf("%w: %q lineage hop %q: %v", ErrLineage, from.ID, cur.ParentRunID, err)
		}
		cur = next
	}
	return fmt.Errorf("%w: %q lineage exceeds %d hops", ErrLineage, from.ID, maxHops)
}

func (p *Pipeline) advanceLoaded(ctx context.Context, st *State) (*State, error) {
	if st.Phase == PhaseApproved || st.Phase == PhaseCancelled {
		return st, nil
	}
	if st.OpenAskID != "" {
		return st, nil // a gate is open — gates wait (S06.1)
	}
	r, err := p.Runs.Get(ctx, st.RunID)
	if err != nil {
		return nil, err
	}
	if r.State != run.StateRunning {
		return nil, fmt.Errorf("%w: run %s is %s", ErrNotRunning, st.RunID, r.State)
	}
	// The post-answer advance is the platform driving this run in-process, so
	// the pipeline HOLDS its lease for the whole drive (Spec S02.2 at ⚙
	// recovery.heartbeat; P3-RW-5 R3/R5). The hold sits after the resume
	// transition that got us here, so it beats at the resumed generation, and
	// it beats immediately — the un-park instant is the moment the incident
	// killed a live interview. It ends when the advance does: the next gate
	// parks the run, and nobody drives a parked run.
	defer p.Leases.Hold(ctx, st.RunID, leaseHolderIntake)()

	// Stage-0's classifier half, on the running run and before any paid seam
	// (S06.2 ordering). Run-once: the marker is durable.
	if err := p.classifyStep(ctx, st); err != nil {
		return nil, err
	}

	var pair *Pair
	for guard := 0; guard < 64; guard++ {
		var blocked bool
		var err error
		switch st.Phase {
		case PhaseInterview:
			blocked, pair, err = p.phaseInterview(ctx, st, pair)
		case PhaseSpine:
			blocked, pair, err = p.phaseSpine(ctx, st, pair)
		case PhaseCritique:
			blocked, pair, err = p.phaseCritique(ctx, st, pair)
		case PhaseApproval:
			blocked, pair, err = p.phaseApproval(ctx, st, pair)
		default:
			return st, nil
		}
		if err != nil {
			return nil, err
		}
		if blocked || st.Phase == PhaseApproved || st.Phase == PhaseCancelled {
			return st, nil
		}
	}
	return nil, fmt.Errorf("intake: pipeline did not settle (programming error)")
}

// phaseInterview runs the Stage-1 interview loop and drafting (Spec
// S06.5–S06.6).
func (p *Pipeline) phaseInterview(ctx context.Context, st *State, pair *Pair) (bool, *Pair, error) {
	// Before any taxonomy question: if nothing resolved the family, ASK
	// (P3-RW-11 R4). It has to come first, because a question card drawn from
	// the wrong question set is the defect this packet exists to remove — and
	// asking is cheaper than a whole interview aimed at the wrong thing.
	if blocked, err := p.familyGate(ctx, st); blocked || err != nil {
		return blocked, pair, err
	}

	tax := p.taxonomyFor(st.Family)
	resolved := st.resolvedSet()
	st.Clearance = tax.Clearance(resolved)

	// Explicit Re-interview (S06.9): one full card of the top-weight
	// slots, answered values overwrite.
	if st.ReinterviewRequested {
		st.ReinterviewRequested = false
		qs := make([]Question, 0, maxQuestionsPerCard)
		for _, s := range tax.Slots {
			if len(qs) == maxQuestionsPerCard {
				break
			}
			qs = append(qs, Question{ID: s.ID, Text: s.Question, Options: s.Options, Weight: s.Weight})
		}
		return true, pair, p.issueCard(ctx, st, p.buildInterviewCard(ctx, st, tax, qs))
	}

	// The interview: continues while Clearance is below the tier floor and
	// unresolved slots remain; no fixed question caps (G1 P8). The band
	// never interviews (S06.4); force-proceed stops it.
	if !st.Band && !st.ForceProceeded {
		floor, err := p.clearanceFloor(st.Tier)
		if err != nil {
			return false, pair, err
		}
		unresolved := tax.Unresolved(resolved)
		if st.Clearance < floor && len(unresolved) > 0 {
			qs := make([]Question, 0, maxQuestionsPerCard)
			for _, s := range unresolved {
				if len(qs) == maxQuestionsPerCard {
					break
				}
				qs = append(qs, Question{ID: s.ID, Text: s.Question, Options: s.Options, Weight: s.Weight})
			}
			return true, pair, p.issueCard(ctx, st, p.buildInterviewCard(ctx, st, tax, qs))
		}
	}

	if !st.NeedsDraft {
		// Pair exists (TIER-UP re-entry or re-interview): floor satisfied
		// again — re-verify through the spine.
		st.Phase = PhaseSpine
		return false, pair, p.appendState(ctx, st)
	}

	// Unresolved slots convert to explicit assumptions where the spec says
	// so: the band auto-lists all of them; force-proceed converts all of
	// them; a floor reached leaves the low-weight tail to the planner's
	// own listed assumptions (S06.4, S06.5).
	if st.Band || st.ForceProceeded {
		origin := "band"
		if st.ForceProceeded {
			origin = "force_proceed"
		}
		for _, s := range tax.Unresolved(resolved) {
			st.resolveSlot(SlotResolution{
				SlotID: s.ID, How: ResolvedAssumption,
				Assumption: fmt.Sprintf("(%s) %s — assumed: proceeding without an answer", origin, s.Name),
			})
		}
		st.Clearance = tax.Clearance(st.resolvedSet())
	}

	// Draft (Stage-1 emission).
	in := DraftInput{
		// The CONSUMING run, read from the state rather than composed from the
		// task id: after a recovery-fork rebind it is the fork (R7).
		RunID:   st.RunID,
		Request: st.Req, Family: st.Family, Tier: st.Tier,
		Registry: st.Registry, Taxonomy: tax,
		Resolutions: st.Resolutions, DataHits: st.openDataBearing(),
		Supplied: st.Supplied, Escalations: st.Escalations,
		SpecVersion: st.SpecVersion + 1, PlanVersion: st.PlanVersion + 1,
	}
	if st.SpecRef != nil {
		prior, err := p.loadPair(st)
		if err != nil {
			return false, pair, err
		}
		in.Prior = prior
	}
	next, err := p.Planner.Draft(ctx, in)
	if blocked, herr := p.handleEscalation(ctx, st, err, "draft"); blocked || herr != nil {
		return blocked, pair, herr
	}
	newPair, err := p.acceptEmission(ctx, st, in.Prior, &next)
	if err != nil {
		return false, pair, err
	}
	st.NeedsDraft = false
	st.Phase = PhaseSpine
	return false, newPair, p.appendState(ctx, st)
}

// ---- The S06.5 phrase-and-summarize seat (P3-RW-12) ----

// understoodBlock composes the deterministic "here is what I understood so
// far" items from the state's own record (P3-RW-12 R8/R9). It reads
// State.Resolutions and nothing else: the block can never claim more than the
// platform actually holds, and it needs no new state — the resolutions ride
// every state event already, and the rendered block rides the ask snapshot.
//
// Items come in TAXONOMY order rather than answer order, so a requester
// reading the same list across rounds sees it grow in place instead of
// reshuffling. A resolution whose slot the ACTIVE set does not carry still
// appears: a family swap keeps the answers already given (applyFamilyAnswer),
// and silently dropping them from the summary would misreport the record.
//
// Nil when nothing is resolved yet — there is nothing honest to show.
func understoodBlock(st *State, tax *Taxonomy) *UnderstoodBlock {
	if len(st.Resolutions) == 0 {
		return nil
	}
	items := make([]UnderstoodItem, 0, len(st.Resolutions))
	add := func(r SlotResolution, name string) {
		items = append(items, UnderstoodItem{
			SlotID: r.SlotID, Name: name, How: r.How,
			Value: r.Value, Assumption: r.Assumption,
		})
	}
	for _, s := range tax.Slots {
		for _, r := range st.Resolutions {
			if r.SlotID == s.ID {
				add(r, s.Name)
				break
			}
		}
	}
	for _, r := range st.Resolutions {
		if tax.Slot(r.SlotID) == nil {
			add(r, r.SlotID) // carried over from another family's set
		}
	}
	return &UnderstoodBlock{Items: items}
}

// understoodRecap is the approval card's FULL block (P3-RW-12 R9): every
// resolution, plus every answered 1.7 escalation.
//
// The escalations belong here and nowhere earlier. An escalation is a
// consequential ambiguity raised mid-planning and answered by the requester —
// it changed what gets built exactly as much as an interview answer did, so a
// recap that omitted it would under-report what the approval is approving.
// They carry their own origin label rather than a Resolved* kind, because they
// resolve no must-know slot and Clearance must keep ignoring them.
func understoodRecap(st *State, tax *Taxonomy) *UnderstoodBlock {
	block := understoodBlock(st, tax)
	if len(st.Escalations) == 0 {
		return block
	}
	if block == nil {
		block = &UnderstoodBlock{}
	}
	for i, e := range st.Escalations {
		block.Items = append(block.Items, UnderstoodItem{
			SlotID: fmt.Sprintf("escalation-%d", i+1),
			Name:   e.Question,
			How:    UnderstoodEscalation,
			Value:  e.Answer,
		})
	}
	return block
}

// buildInterviewCard turns an ALREADY-MADE slot selection into the card the
// requester sees (Spec S06.5; P3-RW-12 R6).
//
// The order is the whole point. The taxonomy decides what must be asked and
// the pipeline decides which of those slots fit on this card — both before
// the model is involved. Only then is the finished selection offered to the
// utility seat, and only the WORDING of it comes back. The result is folded
// by slot id here, in platform code: an id that was not asked is dropped, an
// id the seat skipped keeps the taxonomy's words, and the question count,
// order, ids and options are untouchable by construction. That is why the
// containment is not a sentence in a prompt — a prompt-level lockout is
// something a model can be talked out of (R03 §2.1), a fold cannot be.
//
// A nil or erroring seat is not an error: the card ships with the taxonomy's
// own words and no added click (R12).
func (p *Pipeline) buildInterviewCard(ctx context.Context, st *State, tax *Taxonomy, qs []Question) *Card {
	card := &Card{Kind: CardInterview, Questions: qs, Understood: understoodBlock(st, tax)}
	if p.Phraser == nil || len(qs) == 0 {
		return card
	}
	in := PhraseInput{
		// The CONSUMING run, read from the state: after a recovery-fork
		// rebind this is the fork, so the seat's ONE $0 D7 row meters on the
		// run the pipeline is actually driving (§26; the DraftInput.RunID
		// precedent).
		RunID:   st.RunID,
		Request: st.Req, Family: st.Family, Tier: st.Tier,
		Questions: make([]PhraseQuestion, 0, len(qs)),
	}
	for _, q := range qs {
		in.Questions = append(in.Questions, PhraseQuestion{ID: q.ID, Text: q.Text})
	}
	if card.Understood != nil {
		in.Understood = card.Understood.Items
	}
	res, err := p.Phraser.PhraseAndSummarize(ctx, in)
	if err != nil {
		return card
	}
	for i := range card.Questions {
		if text, ok := res.Phrasings[card.Questions[i].ID]; ok && strings.TrimSpace(text) != "" {
			card.Questions[i].Phrased = text
		}
	}
	if summary := strings.TrimSpace(res.Summary); summary != "" {
		if card.Understood == nil {
			card.Understood = &UnderstoodBlock{}
		}
		card.Understood.Text = summary
	}
	return card
}

// familyGate issues the S06.5 family question when Stage 0 left the family
// unresolved, and reports whether the pipeline is now parked on it.
//
// It fires on exactly one condition — `FamilySource == "default"`, the marker
// Start writes when neither a registered project nor the classifier resolved a
// family — so it cannot fire twice (the answer records "requester"), and a
// state event written before this packet, which carries no source at all, is
// left exactly as it was: an interview already in flight in an old world is
// never interrupted by a new question (R10).
//
// THE BAND IS NEVER ASKED. S06.4 makes the zero-interaction band the class of
// task that never requires a click, and a task worth under the ⚙ cost cap is
// not worth a question about its shape — it keeps generic with its auto-listed
// assumptions, and the source on the record says which.
func (p *Pipeline) familyGate(ctx context.Context, st *State) (bool, error) {
	if st.Band || st.FamilySource != FamilySourceDefault {
		return false, nil
	}
	return true, p.issueCard(ctx, st, &Card{Kind: CardFamily, Decision: &DecisionBody{
		Summary: "What kind of task is this?",
		Detail: []string{
			"Nothing here says what kind of work this is: it is not filed under a project that declares one, and the platform could not tell from the request itself.",
			"The answer decides which questions get asked next — the platform asks about different things when it is changing software than when it is looking something up.",
		},
		Choices: FamilyChoices(),
		Help: HelpBlock{
			What: "Picking one tells the platform which set of questions to ask about this task. It changes nothing else: not what gets built, " +
				"not what it costs, and not what you approve before anything runs.",
			Wrong: "Pick the closest one — a rough match still asks better questions than none. If none of them fits, choose \"Something else\" " +
				"and the platform asks its general questions instead.",
			Recommend: "Answer from the result you want, not from how the work would be done: \"Build or change software\" if you want something " +
				"working at the end, \"Find something out\" if you want an answer, \"Write or create content\" if you want a piece of writing or a design.",
		},
	}})
}

// phaseSpine applies pending revisions and runs the Stage-2 deterministic
// spine (Spec S06.7).
func (p *Pipeline) phaseSpine(ctx context.Context, st *State, pair *Pair) (bool, *Pair, error) {
	pair, err := p.ensurePair(st, pair)
	if err != nil {
		return false, pair, err
	}

	if st.PendingRevise != nil {
		req := *st.PendingRevise
		in := ReviseInput{
			RunID: st.RunID, // the consuming run (R7)
			Pair:  *pair, Reason: req.Reason, Findings: req.Findings,
			Resolutions: req.Resolutions, Escalations: st.Escalations,
			SpecVersion: st.SpecVersion + 1, PlanVersion: st.PlanVersion + 1,
		}
		next, err := p.Planner.Revise(ctx, in)
		if blocked, herr := p.handleEscalation(ctx, st, err, "revise"); blocked || herr != nil {
			return blocked, pair, herr
		}
		pair, err = p.acceptEmission(ctx, st, pair, &next)
		if err != nil {
			return false, pair, err
		}
		st.PendingRevise = nil
		if err := p.appendState(ctx, st); err != nil {
			return false, pair, err
		}
	}

	// Advisory semantic spot-check (S06.7(a)) — advisory-only.
	var advisory []string
	if p.SpotCheck != nil {
		if notes, err := p.SpotCheck.Check(ctx, *pair); err == nil {
			advisory = notes
		} else {
			advisory = []string{"spot-check unavailable: " + err.Error()}
		}
	}

	res, err := p.runSpine(ctx, st, pair, advisory)
	if err != nil {
		return false, pair, err
	}
	st.Spine = res
	if _, err := p.Ledger.RecordDecision(ctx, st.RunID, ledger.AuthorPlatform, run.ActorPlatform, "spine",
		"spine: "+res.summary(), "Stage-2 deterministic spine (S06.7)", 0); err != nil {
		return false, pair, err
	}

	// (a) Coverage: bounded auto-fix, then a decision card — an agreed
	// criterion never disappears silently.
	if len(res.Uncovered) > 0 {
		rounds, err := p.Settings.Int(keyCoverageAutofixRounds)
		if err != nil {
			return false, pair, fmt.Errorf("intake: read ⚙ %s: %w", keyCoverageAutofixRounds, err)
		}
		if int64(st.CoverageRounds) < rounds {
			st.CoverageRounds++
			st.PendingRevise = &ReviseReq{Reason: ReviseCoverage, Findings: res.Uncovered}
			return false, pair, p.appendState(ctx, st)
		}
		card := &Card{Kind: CardCoverage, Decision: &DecisionBody{
			Summary: fmt.Sprintf("The plan does not cover: %s. Auto-fix rounds are exhausted (⚙ %s).", joinKeys(res.Uncovered), keyCoverageAutofixRounds),
			Detail:  res.Uncovered,
			Choices: []Option{
				{Label: "Re-plan once more", Value: ChoiceReplan},
				{Label: "Drop the criterion (recorded, visible)", Value: ChoiceDropCriterion},
				{Label: "Proceed with the gap listed on the approval card", Value: ChoiceProceedUncovered},
			},
			Help: HelpBlock{
				What:      "An agreed acceptance criterion has no plan step delivering it.",
				Wrong:     "Proceeding with the gap means that criterion will not be worked on or verified.",
				Recommend: "Re-plan once more; drop the criterion only if you no longer want it.",
			},
		}}
		return true, pair, p.issueCard(ctx, st, card)
	}

	// (d) Research-node presence: bounce once, then card (S06.7(d)).
	if len(res.MissingResearch) > 0 {
		if !st.ResearchBounced {
			st.ResearchBounced = true
			st.PendingRevise = &ReviseReq{Reason: ReviseResearch, Findings: res.MissingResearch}
			return false, pair, p.appendState(ctx, st)
		}
		card := &Card{Kind: CardResearch, Decision: &DecisionBody{
			Summary: "The task depends on facts in the world, but the plan still lacks required research steps for: " + joinKeys(res.MissingResearch),
			Detail:  res.MissingResearch,
			Choices: []Option{
				{Label: "I'll supply the fact myself", Value: ChoiceSupplyFact},
				{Label: "Re-plan to add the research step", Value: ChoiceReplan},
			},
			Help: HelpBlock{
				What:      "Live research is required by policy for data-bearing tasks (1.9).",
				Wrong:     "Without it the result may rest on stale or invented facts.",
				Recommend: "Let the plan research it — or supply the fact if you already own it.",
			},
		}}
		return true, pair, p.issueCard(ctx, st, card)
	}

	// Open NEEDS-CLARIFICATION markers cannot reach approval (S06.6).
	if len(res.OpenMarkers) > 0 {
		qs := make([]Question, 0, maxQuestionsPerCard)
		for i, m := range res.OpenMarkers {
			if len(qs) == maxQuestionsPerCard {
				break
			}
			qs = append(qs, Question{ID: fmt.Sprintf("marker-%d", i+1), Text: m})
		}
		// A Stage-1 ask is a Stage-1 ask: the clarification card carries the
		// same understanding block (P3-RW-12 OQ5). It is NOT phrased — these
		// questions are the planner's own marker text, not taxonomy wording,
		// so there is nothing here for the phrasing seat to reword.
		return true, pair, p.issueCard(ctx, st, &Card{
			Kind: CardClarification, Questions: qs,
			Understood: understoodBlock(st, p.taxonomyFor(st.Family)),
		})
	}

	if tierRank[st.Tier] >= tierRank[TierStandard] && !st.CritiqueDone {
		st.Phase = PhaseCritique
	} else {
		st.Phase = PhaseApproval
	}
	return false, pair, p.appendState(ctx, st)
}

// phaseCritique is Stage 3 (Spec S06.8): fresh-context self-attack,
// artifact-only input, mandatory from the standard tier up.
func (p *Pipeline) phaseCritique(ctx context.Context, st *State, pair *Pair) (bool, *Pair, error) {
	if tierRank[st.Tier] < tierRank[TierStandard] || st.CritiqueDone {
		st.Phase = PhaseApproval
		return false, pair, p.appendState(ctx, st)
	}
	if p.Critic == nil {
		return false, pair, fmt.Errorf("%w: critique is mandatory at %s tier (S06.4) and no Critic is configured", ErrSeamMissing, st.Tier)
	}
	pair, err := p.ensurePair(st, pair)
	if err != nil {
		return false, pair, err
	}

	var v Verdict
	if len(st.OpenFindings) > 0 {
		// Re-critique checks only the named findings (S06.8 REVISE).
		v, err = p.Critic.Recheck(ctx, *pair, st.OpenFindings)
	} else {
		v, err = p.Critic.Critique(ctx, *pair)
	}
	if blocked, herr := p.handleEscalation(ctx, st, err, "critique"); blocked || herr != nil {
		return blocked, pair, herr
	}

	round := len(st.Verdicts) + 1
	st.Verdicts = append(st.Verdicts, VerdictRecord{
		Round: round, Kind: v.Kind, Findings: v.Findings, Doubt: v.Doubt,
		Proposed: v.ProposedTier, TS: p.nowRFC3339(),
	})
	// The verdict is written to the ledger; the findings artifact joins §5
	// (S06.1 Stage 3: decisions + artifacts).
	md := renderCritiqueMD(st.TaskID, st.PlanVersion, round, v)
	ref, err := p.store().write(p.store().critiquePath(st.TaskID, st.PlanVersion, round), md, nil, round)
	if err != nil {
		return false, pair, err
	}
	gen, err := p.currentGen(ctx, st.RunID)
	if err != nil {
		return false, pair, err
	}
	verbs := p.Ledger.SessionVerbs(st.RunID, "critique", gen)
	if _, err := verbs.Artifact(ctx, ref.Path, "critique", fmt.Sprintf("Stage-3 critique round %d (S06.8)", round), ref.SHA256); err != nil {
		return false, pair, err
	}
	if _, err := verbs.Decide(ctx,
		fmt.Sprintf("critique verdict: %s (round %d, %d findings)", v.Kind, round, len(v.Findings)),
		"Stage-3 plan self-attack (S06.8)", 0); err != nil {
		return false, pair, err
	}

	switch v.Kind {
	case VerdictPass:
		st.CritiqueDone = true
		st.OpenFindings = nil
		st.Phase = PhaseApproval

	case VerdictRevise:
		rounds, err := p.Settings.Int(keyCritiqueReviseRounds)
		if err != nil {
			return false, pair, fmt.Errorf("intake: read ⚙ %s: %w", keyCritiqueReviseRounds, err)
		}
		if int64(st.CritiqueRounds) < rounds {
			st.CritiqueRounds++
			st.OpenFindings = v.Findings
			st.PendingRevise = &ReviseReq{Reason: ReviseCritique, Findings: v.Findings}
			st.Phase = PhaseSpine // the spine runs after every revision (S06.7)
		} else {
			// One pass, never loops: proceed with the critique attached —
			// the open findings ride the approval card (S06.8).
			st.CritiqueDone = true
			st.OpenFindings = v.Findings
			st.Phase = PhaseApproval
		}

	case VerdictSpecDoubt:
		// Mandatory decision card — never absorbed (S06.8, P45).
		card := &Card{Kind: CardSpecDoubt, Decision: &DecisionBody{
			Summary: "The critique concludes the specification itself may be a bad idea: " + v.Doubt,
			Choices: []Option{
				{Label: "Proceed anyway", Value: ChoiceProceedAnyway},
				{Label: "Adjust the spec", Value: ChoiceAdjustSpec},
				{Label: "Rethink (cancel this intake)", Value: ChoiceRethink},
			},
			Help: HelpBlock{
				What:      "A fresh reviewer doubts the goal as specified, not just the plan.",
				Wrong:     "Proceeding anyway may execute a well-built plan for the wrong goal.",
				Recommend: "Read the doubt; adjust the spec if it rings true.",
			},
		}}
		return true, pair, p.issueCard(ctx, st, card)

	case VerdictTierUp:
		raised := ValidTier(v.ProposedTier) && tierRank[v.ProposedTier] > tierRank[st.Tier]
		if raised {
			st.Tier = maxTier(st.Tier, v.ProposedTier)
			st.exitBand("TIER-UP (S06.8)")
			st.CritiqueDone = false
			st.OpenFindings = nil
			st.Phase = PhaseInterview // re-enter at the new tier's requirements
		} else {
			// A TIER-UP that cannot raise (already at the proposed tier or
			// higher) degenerates to proceed-with-verdict-attached; the
			// requester sees it on the card.
			st.CritiqueDone = true
			st.Phase = PhaseApproval
		}

	default:
		return false, pair, fmt.Errorf("intake: unknown verdict %q", v.Kind)
	}
	return false, pair, p.appendState(ctx, st)
}

// phaseApproval is Stage 4 (Spec S06.9).
func (p *Pipeline) phaseApproval(ctx context.Context, st *State, pair *Pair) (bool, *Pair, error) {
	pair, err := p.ensurePair(st, pair)
	if err != nil {
		return false, pair, err
	}
	if len(pair.Spec.Clarifications) > 0 {
		return false, pair, ErrMarkersOpen
	}
	// S08.8 selection runs BEFORE the card is built so the selected worker
	// and its plain-language reason are visible pre-execution (the no-fit
	// two-stage offer rides the same card; a pinned selection survives
	// re-planning recomputes).
	if err := p.computeRouting(ctx, st, pair); err != nil {
		return false, pair, err
	}
	if st.Band {
		// Zero-interaction band: the pipeline still ran — SPEC and
		// degenerate PLAN exist in the ledger, ungated; no blocking gate,
		// completion card only (delivered at completion, S06.4).
		if err := p.approve(ctx, st, pair, run.ActorPlatform, true, nil); err != nil {
			return false, pair, err
		}
		return false, pair, nil
	}
	card, err := p.buildApprovalCard(ctx, st, pair)
	if err != nil {
		return false, pair, err
	}
	if p.Fingerprint != nil {
		if fp, err := p.Fingerprint(ctx, projectOf(st.Registry)); err == nil {
			fp.SpecPlanVersion = pair.Plan.SpecPlanVersion()
			st.StoredFingerprint = &fp
		}
	}
	// Cited project-truth entries (Spec S09.6 "records those entry versions in
	// its freshness fingerprint", R31): resolve the plan's citations to their
	// current versions and record them in the stored fingerprint so a later
	// supersession/removal/new-version is drift. A citation that cannot resolve
	// at approval (resolver error, or a cited key that is not an ACTIVE
	// project-truth entry) is a LOUD capture failure surfaced through the
	// approval flow — never silently stored-as-nothing, which would escape every
	// future drift check (F10). The resolver-absent test posture leaves
	// citations uncaptured (the §14 absent-seam degradation); production always
	// wires it.
	if len(pair.Plan.CitedEntries) > 0 && p.CitedEntryVersions != nil {
		versions, err := p.CitedEntryVersions(ctx, pair.Plan.CitedEntries)
		if err != nil {
			return false, pair, fmt.Errorf("%w: %v", ErrCitationUnresolved, err)
		}
		for _, k := range pair.Plan.CitedEntries {
			if _, ok := versions[k]; !ok {
				return false, pair, fmt.Errorf("%w: cited entry %q is not an active project-truth entry at approval", ErrCitationUnresolved, k)
			}
		}
		if st.StoredFingerprint == nil {
			st.StoredFingerprint = &run.Fingerprint{SpecPlanVersion: pair.Plan.SpecPlanVersion()}
		}
		st.StoredFingerprint.CitedEntryVersions = versions
	}
	return true, pair, p.issueCard(ctx, st, card)
}

// ---- Emission acceptance (draft/revise) ----

// acceptEmission validates a Stage-1 emission, assigns versions (a spec
// version moves only when spec content changed — stable keys, no drift),
// guarantees converted-slot assumptions are listed, persists the artifact
// files, and appends the ledger §5 refs via the stage session verbs.
func (p *Pipeline) acceptEmission(ctx context.Context, st *State, prior *Pair, next *Pair) (*Pair, error) {
	// The platform guarantees requester-supplied inputs are recorded on
	// the SPEC (S06.3), regardless of what the model listed.
	for _, f := range st.Supplied {
		found := false
		for _, have := range next.Spec.Supplied {
			if have.RuleID == f.RuleID && have.Fact == f.Fact {
				found = true
				break
			}
		}
		if !found {
			next.Spec.Supplied = append(next.Spec.Supplied, f)
		}
	}

	// The platform guarantees visibility of converted slots regardless of
	// what the model listed (S06.5 force-proceed contract).
	tax := p.taxonomyFor(st.Family)
	for _, r := range st.Resolutions {
		if r.How != ResolvedAssumption {
			continue
		}
		origin := "slot:" + r.SlotID
		found := false
		for _, a := range next.Spec.Assumptions {
			if a.Origin == origin {
				found = true
				break
			}
		}
		if !found {
			text := r.Assumption
			if s := tax.Slot(r.SlotID); s != nil && text == "" {
				text = s.Name + ": assumed"
			}
			next.Spec.Assumptions = append(next.Spec.Assumptions, Assumption{Text: text, Origin: origin})
		}
	}

	// Version assignment: plan bumps on every emission; spec bumps only on
	// content change.
	specV, planV := st.SpecVersion+1, st.PlanVersion+1
	if prior != nil && specContentEqual(&prior.Spec, &next.Spec) {
		specV = prior.Spec.Version
	}
	next.Spec.TaskID, next.Spec.Owner = st.TaskID, st.Owner
	next.Plan.TaskID, next.Plan.Owner = st.TaskID, st.Owner
	next.Spec.Version, next.Spec.Status, next.Spec.Tier = specV, StatusDraft, st.Tier
	next.Plan.Version, next.Plan.SpecVersion, next.Plan.Status, next.Plan.Tier = planV, specV, StatusDraft, st.Tier
	if err := next.Validate(); err != nil {
		return nil, err
	}

	store := p.store()
	specRef, err := store.write(store.specPath(st.TaskID, specV), renderSpecMD(&next.Spec), &next.Spec, specV)
	if err != nil {
		return nil, err
	}
	planRef, err := store.write(store.planPath(st.TaskID, planV), renderPlanMD(&next.Plan), &next.Plan, planV)
	if err != nil {
		return nil, err
	}
	st.SpecVersion, st.PlanVersion = specV, planV
	st.SpecRef, st.PlanRef = &specRef, &planRef

	// §5 artifact refs land through the plan stage session (S05.1 writers).
	gen, err := p.currentGen(ctx, st.RunID)
	if err != nil {
		return nil, err
	}
	verbs := p.Ledger.SessionVerbs(st.RunID, "plan", gen)
	if _, err := verbs.Artifact(ctx, specRef.Path, "spec", fmt.Sprintf("SPEC %s (S06.6)", next.Spec.VersionKey()), specRef.SHA256); err != nil {
		return nil, err
	}
	if _, err := verbs.Artifact(ctx, planRef.Path, "plan", fmt.Sprintf("PLAN %s (S06.6)", next.Plan.VersionKey()), planRef.SHA256); err != nil {
		return nil, err
	}
	return next, nil
}

// specContentEqual compares spec content modulo version/status.
func specContentEqual(a, b *Spec) bool {
	ca, cb := *a, *b
	ca.Version, cb.Version = 0, 0
	ca.Status, cb.Status = "", ""
	ca.Tier, cb.Tier = "", ""
	ja, _ := json.Marshal(ca)
	jb, _ := json.Marshal(cb)
	return string(ja) == string(jb)
}

// handleEscalation routes a seam call's Escalation into a single-question
// card (1.7: the run blocks-not-fails and resumes on answer). Returns
// (true, nil) when a card was issued; (false, err) on a real error;
// (false, nil) when there was no error at all.
func (p *Pipeline) handleEscalation(ctx context.Context, st *State, err error, from string) (bool, error) {
	if err == nil {
		return false, nil
	}
	var esc *Escalation
	if !errors.As(err, &esc) {
		return false, err
	}
	st.PendingEscalation = from
	card := &Card{Kind: CardEscalation, Questions: []Question{{ID: "escalation", Text: esc.Question}}}
	if ierr := p.issueCard(ctx, st, card); ierr != nil {
		return false, ierr
	}
	return true, nil
}

// ---- Cards & gates ----

// issueCard persists the ask row, parks the run, and appends the state —
// one transaction. Gates wait; answering resumes the pipeline in place
// (Spec S06.1).
func (p *Pipeline) issueCard(ctx context.Context, st *State, card *Card) error {
	st.CardVersion++
	askID := fmt.Sprintf("intake:%s:%d", st.TaskID, st.CardVersion)
	card.TaskID, card.RunID = st.TaskID, st.RunID
	card.Version = st.CardVersion
	card.IssuedTS = p.nowRFC3339()
	card.Clearance = st.Clearance
	card.Tier = st.Tier
	st.OpenAskID, st.OpenAskKind = askID, card.Kind
	st.CardIssuedTS = card.IssuedTS
	st.StaleFlag, st.StaleReasons = false, nil
	return p.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		if err := p.insertAskTx(ctx, tx, askID, st, card); err != nil {
			return err
		}
		if _, err := p.Runs.TransitionTx(ctx, tx, st.RunID, run.StateParked, run.TransitionOptions{
			Reason: fmt.Sprintf("intake gate open: %s — gates wait (S06.1)", card.Kind),
			Actor:  run.ActorPlatform,
		}); err != nil {
			return err
		}
		return p.appendStateTx(ctx, tx, st)
	})
}

// buildApprovalCard renders the S06.9 card content contract from the
// artifacts.
func (p *Pipeline) buildApprovalCard(ctx context.Context, st *State, pair *Pair) (*Card, error) {
	help := defaultHelp(pair)
	if p.Utility != nil {
		if h, err := p.Utility.Help(ctx, *pair); err == nil {
			help = h
		}
	}
	steps := make([]string, 0, len(pair.Plan.Steps))
	for _, s := range pair.Plan.Steps {
		steps = append(steps, s.Title)
	}
	costTime := "UNPRICED (no price table; measured usage will appear on the receipt)"
	if pair.Plan.Est.Known {
		costTime = fmt.Sprintf("~%.2f USD (API-equivalent, D5)", pair.Plan.Est.USD)
	}
	l1 := ApprovalLayer1{
		Restatement: pair.Spec.Restatement,
		Deliverable: pair.Spec.Outcome,
		Steps:       steps,
		WillNotDo:   pair.Spec.OutOfScope,
		Assumptions: pair.Spec.Assumptions, // the centerpiece (R03 §2.4)
		Risks:       pair.Plan.Risks,
		CostTime:    costTime,
		Clearance:   st.Clearance,
		SizeClass:   pair.Plan.Est.SizeClass,
		Help:        help,
		Uncovered:   st.AcceptedUncovered,
		OpenFinds:   st.OpenFindings,
		// The full record beside the planner's prose restatement (P3-RW-12
		// R9): every resolution — including the band's and force-proceed's
		// conversions — plus every answered escalation, each labelled with
		// how it got there. It complements the restatement (one is what the
		// planner understood, the other is what the platform recorded) and it
		// adds no card and no click, because approval IS the confirmation
		// (S06.1).
		Understood: understoodRecap(st, p.taxonomyFor(st.Family)),
	}
	if st.Spine != nil && st.Spine.SizeFinding != "" {
		l1.SizeNote = st.Spine.SizeDetail // stakes-gated display: non-trivial shows it (2.5)
	}
	l2 := ApprovalLayer2{
		ACs: pair.Spec.ACs, Steps: pair.Plan.Steps, Coverage: pair.Plan.Coverage,
		Verdicts: st.Verdicts, ResearchNodes: pair.Plan.ResearchNodes,
		Estimate: pair.Plan.Est, SpecRef: st.SpecRef, PlanRef: st.PlanRef,
	}
	actions := []string{ActionApprove, ActionRePlan, ActionReInterview, ActionCancel}
	if st.Routing != nil && st.Routing.ComposeEarned && st.Compose == nil {
		// The no-fit stage-2 compose offer is an ANSWERABLE verb exactly
		// when composition is earned (Spec S08.6 compose-when-earned) and
		// no composition ran for this task yet.
		actions = append(actions, ActionCompose)
	}
	return &Card{Kind: CardApproval, Approval: &ApprovalBody{
		Layer1: l1,
		Layer2: l2,
		// The S08.8 selection block: worker + plain-language reason, the
		// re-route candidates, and the no-fit two-stage offer — visible and
		// overridable pre-execution.
		Routing: st.Routing,
		Actions: actions,
	}}, nil
}

// ---- Shared helpers ----

func (p *Pipeline) clearanceFloor(t Tier) (float64, error) {
	var key string
	switch t {
	case TierLow:
		key = keyClearanceFloorLow
	case TierStandard:
		key = keyClearanceFloorStd
	case TierHigh:
		key = keyClearanceFloorHigh
	default:
		return 0, nil // trivial: no interview (S06.4)
	}
	v, err := p.Settings.Int(key)
	if err != nil {
		return 0, fmt.Errorf("intake: read ⚙ %s: %w", key, err)
	}
	return float64(v), nil
}

// ensurePair loads the current pair from the durable artifacts when the
// caller does not hold it (resume-from-artifact, Spec S06.1).
func (p *Pipeline) ensurePair(st *State, pair *Pair) (*Pair, error) {
	if pair != nil {
		return pair, nil
	}
	return p.loadPair(st)
}

func (p *Pipeline) loadPair(st *State) (*Pair, error) {
	spec, err := p.store().loadSpec(st.SpecRef)
	if err != nil {
		return nil, err
	}
	plan, err := p.store().loadPlan(st.PlanRef)
	if err != nil {
		return nil, err
	}
	return &Pair{Spec: spec, Plan: plan}, nil
}

// CurrentPair loads a task's CURRENT drafted artifact pair regardless of
// approval — the read seam for consumers of the specification content
// itself (the S08.6 composer's task-spec policy input reads it while the
// approval card is still open). Nothing EXECUTES from this read (D10:
// execution still demands ApprovedPair); the artifact loads carry the same
// sha256 + re-render integrity checks.
func (p *Pipeline) CurrentPair(ctx context.Context, taskID string) (*Pair, error) {
	st, err := p.LoadState(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return p.loadPair(st)
}

// ApprovedPair loads a task's APPROVED artifact pair plus its pipeline
// state — the read seam execution and verification build from (B2-4
// walking skeleton): work proceeds from the durable approved artifacts,
// never from conversation (D7; Spec S06.1), and the load re-verifies
// sha256 + byte-identical re-render (resume-from-artifact integrity, Spec
// S06.6). ErrPhase when no approved plan exists — D10: nothing executes
// unapproved.
func (p *Pipeline) ApprovedPair(ctx context.Context, taskID string) (Pair, *State, error) {
	st, err := p.LoadState(ctx, taskID)
	if err != nil {
		return Pair{}, nil, err
	}
	if st.Phase != PhaseApproved {
		return Pair{}, nil, fmt.Errorf("%w: task %s is %s, not approved (D10)", ErrPhase, taskID, st.Phase)
	}
	pair, err := p.loadPair(st)
	if err != nil {
		return Pair{}, nil, err
	}
	return *pair, st, nil
}

func (p *Pipeline) currentGen(ctx context.Context, runID string) (int64, error) {
	r, err := p.Runs.Get(ctx, runID)
	if err != nil {
		return 0, err
	}
	return r.Generation, nil
}

func joinKeys(keys []string) string {
	out := ""
	for i, k := range keys {
		if i > 0 {
			out += ", "
		}
		out += k
	}
	return out
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failure is unrecoverable
	}
	return hex.EncodeToString(b)
}
