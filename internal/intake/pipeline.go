package intake

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
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

	// PinnableLanes is the S08.8 lane-pin seam (S00.9 A13): which lanes a
	// request may name in Request.PinnedLane, each carrying the SELECTION
	// layer's own verdict. The composition root fills it beside the coverage
	// view the router reads, so the pinnable set has one spelling (§65 D4).
	//
	// NIL IS FAIL-CLOSED, deliberately: with no seam wired nothing is
	// pinnable and every pin refuses. The hazard this packet exists to close
	// is a pin silently dropped, so an unwired seam may not answer "allow
	// everything" — the only default is consent (§12), and a mutation that
	// stops the composition root filling this turns the accepted case red
	// rather than leaving it quietly permissive.
	//
	// An EMPTY PinnedLane never consults it, so an unpinned request on an
	// unwired pipeline behaves exactly as it did before this seam existed.
	PinnableLanes []LanePinOption

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

	// Logger records the pipeline's DEGRADES — the places an optional seam
	// failed and the requester silently got the lesser card (PH-1 F3;
	// CONVENTIONS §71 slog/stderr). Nil = slog.Default(); the event log stays
	// the audit truth, this is the operator's search surface.
	Logger *slog.Logger
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

// logger is the nil-safe degrade logger (an unwired pipeline logs to the
// process default, never to nothing).
func (p *Pipeline) logger() *slog.Logger {
	if p.Logger != nil {
		return p.Logger
	}
	return slog.Default()
}

// logSeatDegrade records an OPTIONAL model seat that failed and the lesser card
// that shipped because of it (PH-1 F3).
//
// Platform-authored fields only: which seat, which run, which card, how much
// of it was affected, and the seam's own error — never the requester's text or
// anything a model wrote (S01.11). WARN, because nothing is broken enough to
// stop the run and everything here is worth an operator's attention: this is
// the line whose absence let a 0%-success seat run a whole cold walk unnoticed.
func (p *Pipeline) logSeatDegrade(seat, runID string, card CardKind, questions int, cause error) {
	p.logger().Warn("intake: "+seat+" seat degraded — the card ships the platform's own wording, and the requester is told so",
		"seat", seat, "run", runID, "card", string(card), "questions", questions, "cause", cause)
}

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
	// The LANE PIN is refused HERE — before the task id is even minted, and
	// far before the task/run/event birth transaction below (S00.9 A13). It
	// is a question about this request's input, answered at the boundary that
	// admits it (§30: an unhonorable pin is a bad REQUEST, never a platform
	// defect), and answering it late would leave a task born believing it got
	// a lane it did not get — the Request.Project precedent exactly.
	if err := p.refuseLanePin(req.PinnedLane); err != nil {
		return nil, err
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
		TierSource: TierSourceFailClosed,
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
			Family: settledFamily(st),
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
				st.Tier, st.TierSource = prop.Tier, TierSourceClassifier
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
		// unresolved for the S06.5 question (S06.2). The POSTURE is recorded as
		// such: high because the platform could not read the request, not
		// because it judged the task risky — which is what lets it settle once
		// the requester supplies the fact whose absence caused the abstain.
		st.Tier, st.TierSource = TierHigh, TierSourceFailClosed
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

// settledFamily is the family the platform can stand behind when it asks the
// classification duty to read a task: a registered project's standing
// declaration, or the requester's own answer. A classifier's own earlier guess
// is never fed back to it, and an unresolved family is precisely what the duty
// is being asked about (precedence registry > classifier > requester-asked,
// P3-RW-11 OQ3).
func settledFamily(st *State) Family {
	switch st.FamilySource {
	case FamilySourceRegistry, FamilySourceRequester:
		return st.Family
	default:
		return ""
	}
}

// settleAbstainedTier completes Stage 0 when the requester's family answer
// removes the cause of a classifier abstain (Spec S06.2; P3-GF14 R4.1).
//
// The fail-closed HIGH is a POSTURE held while classification is unavailable —
// S06.2's own word is that the task is TREATED as high-stakes — not a
// classification. Completing the classification once its cause is gone is
// therefore Stage 0 finishing, not one of S06.4's automatic adjustments (which
// govern a tier that WAS classified): the proposal ASSIGNS the tier exactly as
// the first classification does. Floors still clamp upward regardless, the
// zero-interaction band is never re-entered, and one shot is structural — the
// family card is issued only while nothing has resolved the family, so it is
// answered at most once per task.
//
// A classifier that still abstains leaves the HIGH standing, and the card says
// why it is high (the stakes block's fail-closed origin).
func (p *Pipeline) settleAbstainedTier(ctx context.Context, st *State) error {
	if st.TierSource != TierSourceFailClosed || p.Classifier == nil {
		return nil
	}
	prop, err := p.Classifier.Classify(ctx, TriageInput{
		RunID: st.RunID, Request: st.Req, Registry: st.Registry, Family: st.Family,
	})
	if err != nil || !ValidTier(prop.Tier) {
		return nil
	}
	st.Tier, st.TierSource = prop.Tier, TierSourceClassifier
	st.Guess = prop.Est
	st.addHits(prop.DataHits)              // add-only (S06.2/S06.3)
	st.addFloors(validFloors(prop.Floors)) // overrides upward, and then owns the tier
	if prop.Tier == TierTrivial {
		// Trivial IS the band, and the band is decided once at the first pass
		// and never re-entered (S06.4): a trivial proposal arriving here lands
		// at low, the same landing pipeline.go's first classification gives it.
		st.Tier = maxTier(TierLow, st.FloorTier)
	}
	_, err = p.Ledger.RecordDecision(ctx, st.RunID, ledger.AuthorPlatform, run.ActorPlatform, "triage",
		"how careful to be with this task settled as "+plainTier(st.Tier)+
			" once you said what kind of task it is — until then the platform could not read the request and was treating it as high-stakes",
		"Stage-0 classification completed after the family answer removed the abstain's cause (S06.2)", 0)
	return err
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
	// The never-asked slots settle HERE, at the same moment the question set is
	// keyed — which is what makes the Clearance meter count them from the very
	// first card rather than discovering them at the end (P3-GF7 R3; harvest H10).
	// They resolve as explicit assumptions, S06.5's third arm, so acceptEmission's
	// existing guarantee mints each one onto the SPEC where it can be contested.
	//
	// Skip-if-already-resolved is what makes it idempotent across re-entry AND
	// across a family switch: a settlement whose text the planner later made
	// concrete (the back-fill in acceptEmission) is not overwritten with the
	// generic sentence on the next pass.
	held := st.resolvedSet()
	for i := range tax.Slots {
		s := &tax.Slots[i]
		if !s.neverAsked() || held[s.ID] {
			continue
		}
		st.resolveSlot(SlotResolution{
			SlotID: s.ID, How: ResolvedAssumption, Via: ViaSystem,
			Assumption: systemAssumption(s),
		})
	}
	st.Clearance = tax.Clearance(st.resolvedSet())
	if _, seeded := p.taxonomies()[st.Family]; seeded {
		return ""
	}
	// S06.5: the fallback is disclosed, never performed silently.
	return fmt.Sprintf("no question set has been written for %s work yet, so the interview asks the general "+
		"questions; the task is still treated as %s when the plan is made and the work is assigned", st.Family, st.Family)
}

// systemAssumption writes the placeholder a never-asked slot carries until the
// planner states its own concrete resolution, in words for the PERSON who will
// read it on the approval card (CONVENTIONS §57 drafting rule 1, §59).
//
// It says its own reason, because this arm's reason is genuinely its own: the
// band was never asked, the force-proceed was asked and declined the lot, the
// skip was asked and waved through — and this one was never a question at all.
// So it must not borrow the skip's sentence: a person who was never asked did
// not skip anything.
func systemAssumption(s *Slot) string {
	name := s.ID
	if s.Name != "" {
		name = s.Name
	}
	return fmt.Sprintf("%s: I settle this one myself rather than asking — what I picked is on the plan below, and you can change it there.", name)
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
			// S06.1: an open gate parks the run until it is answered.
			Reason: fmt.Sprintf("the newer run for this task is waiting on your answer to the %s — nothing moves until that card is answered", plainCardKind(CardKind(st.OpenAskKind))),
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
	// THE DRIVE'S LIFETIME IS THE RUN'S, NOT THE REQUEST'S (P3-GF14 R1). Once a
	// beat has resumed the run, the drive runs to its own end — the next gate,
	// or a terminal phase — whatever became of the viewer's connection. A page
	// reload used to cancel the request context mid-draft, which killed the
	// beat, crashed the run for the recovery ladder and re-paid the whole plan
	// ceremony on the fork: a ~12-minute re-billed heal for the exact act the
	// card invites ("you can leave; nothing is lost"). The caller's death may
	// cost the RESPONSE, never the WORK.
	//
	// The detach lives HERE, at the one seam every drive passes through (the
	// §56 in-seam doctrine: `crash` detaches inside so no call site can forget),
	// and it changes nothing before the resume commit — a caller that dies while
	// its answer is still being written still fails that write, and the standing
	// card is still the remedy. Values are kept; only cancellation is dropped.
	// Runaway containment stays where it is ratified: the lease keeper beating
	// under the drive (§54), the recovery ladder, and the watchdog.
	ctx = context.WithoutCancel(ctx)
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

	// Explicit Re-interview (S06.9): the REVIEW card — every slot of the
	// active set, each carrying what it currently says, answered values
	// overwrite (P3-GF3-BE1 R8).
	if st.ReinterviewRequested {
		st.ReinterviewRequested = false
		return true, pair, p.issueCard(ctx, st, p.buildInterviewCard(ctx, st, tax, reviewQuestions(st, tax)))
	}

	// The interview: continues while Clearance is below the tier floor and
	// unresolved slots remain; no fixed question caps (G1 P8). The band
	// never interviews (S06.4); force-proceed stops it.
	if !st.Band && !st.ForceProceeded {
		floor, err := p.clearanceFloor(st.Tier)
		if err != nil {
			return false, pair, err
		}
		// ASKABLE-unresolved on both halves: the slots that go on the card and
		// the condition that decides whether a card ships at all read the same
		// list, so an interview below its floor whose only unresolved slots are
		// never-asked ones stops instead of issuing an empty card (P3-GF7 R2).
		unresolved := tax.UnresolvedAskable(resolved)
		if st.Clearance < floor && len(unresolved) > 0 {
			qs := make([]Question, 0, maxQuestionsPerCard)
			for _, s := range unresolved {
				if len(qs) == maxQuestionsPerCard {
					break
				}
				qs = append(qs, Question{
					ID: s.ID, Text: s.Question, Why: s.Why,
					Options: s.Options, Weight: s.Weight, Recommended: s.Recommended,
				})
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
		// The prose is for the PERSON reading the approval card, so it says
		// what happened in words, not in the platform's own vocabulary: the
		// structural origin already rides the artifact's Assumption.Origin
		// field, and printing an internal token twice told the requester
		// nothing while making the sentence unreadable (S06.5 disclosed
		// assumptions; CONVENTIONS §57 drafting rule 1). Each arm names the
		// reason nobody was asked — the two are genuinely different reasons.
		because := "this request was small enough to run without an interview, so I assumed a sensible default"
		if st.ForceProceeded {
			because = "you asked me to go ahead without answering, so I assumed a sensible default"
		}
		for _, s := range tax.Unresolved(resolved) {
			st.resolveSlot(SlotResolution{
				SlotID: s.ID, How: ResolvedAssumption,
				Assumption: fmt.Sprintf("%s — %s.", s.Name, because),
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
		// Everything the requester has already settled travels with the ask
		// (R5): the contract's settled-facts rule binds against this block.
		SettledMarkers: st.SettledMarkers,
		SpecVersion:    st.SpecVersion + 1, PlanVersion: st.PlanVersion + 1,
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
		if herr == nil {
			return true, pair, nil // the 1.7 escalation card is open
		}
		// A contract-invalid emission lands on a card, never in the ladder (R3).
		faulted, ferr := p.handleEmissionFault(ctx, st, herr, EmissionOpDraft)
		return faulted, pair, ferr
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
	add := func(r SlotResolution, name string, s *Slot) {
		items = append(items, UnderstoodItem{
			SlotID: r.SlotID, Name: name, How: r.How,
			Value: r.Value, Label: optionLabel(s, r.Value), Assumption: r.Assumption,
		})
	}
	for i := range tax.Slots {
		s := &tax.Slots[i]
		for _, r := range st.Resolutions {
			if r.SlotID == s.ID {
				add(r, s.Name, s)
				break
			}
		}
	}
	for _, r := range st.Resolutions {
		if tax.Slot(r.SlotID) == nil {
			// Carried over from another family's set: no options here to
			// match, so no label — honest absence, never a guess.
			add(r, r.SlotID, nil)
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

// reviewQuestions composes the S06.9 Re-interview review card: every slot of
// the active set, in taxonomy declaration order, each resolved one carrying
// what the record currently says about it (P3-GF3-BE1 R8; design note §2.D).
//
// It is the answer to a requester who wants to CHANGE something, and the shape
// follows from that: re-asking four blind questions makes a person who already
// answered eight of them hunt for the one they wanted to correct. The whole set
// with its current answers attached is the surface they asked for.
//
// S06.5's "up to 4 questions per card" governs interview DELIVERY — fresh
// asking, highest-weight-first, below the floor — and that loop is untouched
// (maxQuestionsPerCard still bounds it). This is the S06.9 verb's own review
// surface, a card the requester explicitly asked for, not a batch of unresolved
// questions (§11 OQ-3, the CardFamily precedent).
//
// The resolutions are composed by platform code from State.Resolutions under
// the understoodBlock discipline: the card can never claim more than the record
// holds, and no model contributes to it.
func reviewQuestions(st *State, tax *Taxonomy) []Question {
	res := make(map[string]SlotResolution, len(st.Resolutions))
	for _, r := range st.Resolutions {
		res[r.SlotID] = r
	}
	qs := make([]Question, 0, len(tax.Slots))
	for _, s := range tax.Slots {
		if s.neverAsked() {
			// A re-interview is still a question card, so it must not present
			// the guess-my-misread question either (P3-GF7 R2, ratified OQ5).
			// These slots' current resolutions ride the understood block this
			// card already carries, and correction stays available through
			// Assume, free text, and the approval card's Re-plan contest.
			continue
		}
		q := Question{
			ID: s.ID, Text: s.Question, Why: s.Why,
			Options: s.Options, Weight: s.Weight, Recommended: s.Recommended,
		}
		if r, ok := res[s.ID]; ok {
			q.Resolution = &UnderstoodItem{
				SlotID: r.SlotID, Name: s.Name, How: r.How,
				Value: r.Value, Label: optionLabel(&s, r.Value), Assumption: r.Assumption,
			}
		}
		qs = append(qs, q)
	}
	return qs
}

// researchGapSummary writes the research card's headline: what the plan still
// has to go and look up, named by SUBJECT rather than by the P47 rule id the
// spine collected (P3-GF13 R1 — the card used to read "…research steps for:
// P47-5"). The ids stay in Detail, which is a machine member.
//
// A rule the seed table does not carry has no plain subject to name, so the
// sentence says the general thing rather than inventing a specific one.
func researchGapSummary(ruleIDs []string) string {
	const lead = "This task depends on facts about the world that can change, and the plan has no step that goes and checks them"
	subjects := make([]string, 0, len(ruleIDs))
	seen := make(map[string]bool, len(ruleIDs))
	for _, id := range ruleIDs {
		name := strings.ToLower(ResearchSubject(id))
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		subjects = append(subjects, name)
	}
	if len(subjects) == 0 {
		return lead + "."
	}
	return lead + ": " + strings.Join(subjects, ", ") + "."
}

// pinnedSuffix says, in words, what the routing block's `pinned` boolean means
// for the requester — the record used to render it as a Go bool (`pin=true`).
func pinnedSuffix(pinned bool) string {
	if pinned {
		return " The choice is pinned, so a re-plan keeps it."
	}
	return ""
}

// optionLabel resolves the plain wording the requester clicked from the machine
// value the record stores. The machine value is left exactly where it is: it is
// the key the answer fold and the FE's edit box round-trip through, and
// replacing it would turn a re-submitted CHOICE into free text (P3-GF13 R9).
//
// "" is the honest answer in both absence cases — a slot with no options to
// match against, and an answer the requester typed in their own words, where
// the value already IS the plain wording.
func optionLabel(s *Slot, value string) string {
	if s == nil || value == "" {
		return ""
	}
	for _, o := range s.Options {
		if o.Value == value {
			return o.Label
		}
	}
	return ""
}

// hasOptionValue reports whether v names one of this question's own option
// values — the containment check the suggested-option fold runs.
func hasOptionValue(opts []Option, v string) bool {
	for _, o := range opts {
		if o.Value == v {
			return true
		}
	}
	return false
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
		// The options ride along so the seat can name one in its suggestion
		// (R3). They are context, never something to rewrite: the labels are
		// authored content and the values are canonical vocabulary, and the fold
		// below takes only a value that already belongs to this question.
		in.Questions = append(in.Questions, PhraseQuestion{ID: q.ID, Text: q.Text, Options: q.Options})
	}
	if card.Understood != nil {
		in.Understood = card.Understood.Items
	}
	res, err := p.Phraser.PhraseAndSummarize(ctx, in)
	if err != nil {
		// The card still ships the taxonomy's own words with no added click —
		// that posture is right and is unchanged. What was wrong was that it
		// happened in SILENCE: the requester was told on screen that the wording
		// is stock, and the operator had nothing to search for. A degradation the
		// user can see must never be invisible to the person who can fix it
		// (PH-1 F3).
		p.logSeatDegrade("phrase", st.RunID, CardInterview, len(qs), err)
		return card
	}
	for i := range card.Questions {
		q := &card.Questions[i]
		if text, ok := res.Phrasings[q.ID]; ok && strings.TrimSpace(text) != "" {
			q.Phrased = text
		}
		// The suggestion folds under the SAME containment as the phrasing: by
		// asked slot id, in platform code. An id nobody asked reaches nothing,
		// an empty suggestion decorates nothing, and a suggested option that
		// names no option of THIS question is dropped while its text survives —
		// a wrong pointer must not become a click that answers the question
		// wrongly (R5).
		if text, ok := res.Suggestions[q.ID]; ok && strings.TrimSpace(text) != "" {
			q.Suggested = text
		}
		if value, ok := res.SuggestedOptions[q.ID]; ok && hasOptionValue(q.Options, value) {
			q.SuggestedOption = value
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
			// The answered-marker record rides the revise leg too — this is the
			// leg the witnessed confirm-loop actually lived on (R5).
			SettledMarkers: st.SettledMarkers,
			SpecVersion:    st.SpecVersion + 1, PlanVersion: st.PlanVersion + 1,
		}
		next, err := p.Planner.Revise(ctx, in)
		if blocked, herr := p.handleEscalation(ctx, st, err, "revise"); blocked || herr != nil {
			if herr == nil {
				return true, pair, nil // the 1.7 escalation card is open
			}
			// PendingRevise is deliberately still set here — it is cleared only
			// after acceptEmission succeeds — so the granted round re-drives this
			// revision with the requester's findings intact (R3).
			faulted, ferr := p.handleEmissionFault(ctx, st, herr, EmissionOpRevise)
			return faulted, pair, ferr
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
			// The SENTENCE names the plain SUBJECT of each missing lookup;
			// Detail keeps the raw rule ids, which are machine members.
			Summary: researchGapSummary(res.MissingResearch),
			Detail:  res.MissingResearch,
			Choices: []Option{
				{Label: "I'll supply the fact myself", Value: ChoiceSupplyFact},
				{Label: "Re-plan to add the research step", Value: ChoiceReplan},
			},
			Help: HelpBlock{
				// 1.9: live research is required, never model memory.
				What:      "When a result rests on facts that can change — prices, dates, versions, what is available — the platform looks them up rather than trusting what a model happens to remember.",
				Wrong:     "Without it the result may rest on stale or invented facts.",
				Recommend: "Let the plan research it — or supply the fact if you already own it.",
			},
		}}
		return true, pair, p.issueCard(ctx, st, card)
	}

	// Open NEEDS-CLARIFICATION markers cannot reach approval (S06.6). The loop
	// is BOUNDED (R7): what is still open at the bound was converted to a listed
	// assumption by acceptEmission before the spine ever saw it, so reaching
	// here past the bound would mean a marker arrived on a pair nobody accepted.
	if len(res.OpenMarkers) > 0 && st.ClarificationRounds < clarificationRoundLimit {
		st.ClarificationRounds++
		qs := make([]Question, 0, maxQuestionsPerCard)
		for i, m := range res.OpenMarkers {
			if len(qs) == maxQuestionsPerCard {
				break
			}
			// A marker the PLATFORM authored carries its own finite choice set
			// and its own reason line; a planner's marker carries neither
			// (P3-GF14 R3). Free text stays available on every question.
			qs = append(qs, Question{
				ID: fmt.Sprintf("marker-%d", i+1), Text: m,
				Options: markerOptions(m), Why: markerWhy(m),
			})
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
			// The platform's own reading of the plan raised the tier, so the
			// platform owns the standing tier from here (P3-GF14 R4.4); a floor
			// re-check later takes it back if one trips.
			st.Tier, st.TierSource = maxTier(st.Tier, v.ProposedTier), TierSourceClassifier
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

	// A slot the platform settled without asking carries a placeholder sentence
	// until the planner says what it actually settled on. Where the planner DID
	// state it — "prices are shown as 12.34 euro" rather than "I settle this one
	// myself" — that is what the requester should read everywhere the resolution
	// appears, so the record takes its text from the accepted artifact (P3-GF7 R3).
	//
	// Platform code COPYING from the artifact, never prose invented pipeline-side:
	// the loop above has already guaranteed an entry exists for every one of
	// these origins, so this only ever replaces the placeholder with the
	// planner's own words, and only for the slots the platform settled itself.
	for i := range st.Resolutions {
		r := &st.Resolutions[i]
		if r.Via != ViaSystem {
			continue
		}
		for _, a := range next.Spec.Assumptions {
			if a.Origin == "slot:"+r.SlotID && strings.TrimSpace(a.Text) != "" {
				r.Assumption = a.Text
				break
			}
		}
	}

	// The NEEDS-CLARIFICATION markers, against the platform's own record
	// (P3-GF12 R6/R7). Both arms are S06.6's own sentence — "each marker is
	// either asked (S06.5) or converted to a listed assumption" — applied to the
	// two cases the landed pipeline had no answer for: a marker that WAS asked
	// and answered and came back anyway, and a marker still open when the rounds
	// ran out. Neither arm is model-output repair: this function already
	// normalizes the SPEC's platform-guaranteed content above, and a marker is
	// not dropped here, it is RESOLVED — with the resolution visible.
	// The unstated-unit backstop (P3-GF14 R3), raised BEFORE the marker arms
	// run, so it is bound by exactly the same contract as a planner's own
	// marker: asked once, settled from the record if it was already answered,
	// and converted to a listed assumption when the rounds run out.
	raisedCurrency := p.currencyGapStands(st, next) && !hasClarification(next.Spec.Clarifications, currencyMarker)
	if raisedCurrency {
		next.Spec.Clarifications = append(next.Spec.Clarifications, currencyMarker)
	}
	disclosures := settleMarkers(st, next)
	if raisedCurrency && st.settledMarker(currencyMarker) == nil {
		disclosures = append(disclosures, markerDisclosure{
			line: "the plan showed prices as bare numbers with no currency named anywhere, so the platform raised it as an open point",
			why:  "S06.5 ask-don't-assume — an unstated unit on priced criteria would change what the result must be; S06.6's marker contract carries it (P3-GF14 R3)",
		})
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
	// Nothing the marker guard did is silent (§60's spirit; S06.7(a)'s "never
	// disappears silently"): each settle and each conversion says on the ledger
	// exactly what was taken off the open list and what stands in its place.
	for _, d := range disclosures {
		if _, err := p.Ledger.RecordDecision(ctx, st.RunID, ledger.AuthorPlatform, run.ActorPlatform, "plan",
			d.line, d.why, 0); err != nil {
			return nil, err
		}
	}

	// The emission was ACCEPTED, so any refused-emission fault this task was
	// parked on is spent (P3-GF12 R3): the next refusal starts its own count.
	st.EmissionFault = nil
	return next, nil
}

// markerDisclosure is one ledger line the marker guard owes.
type markerDisclosure struct{ line, why string }

// settleMarkers applies the two S06.6 marker arms to an emission's open
// NEEDS-CLARIFICATION list and returns what must be disclosed (P3-GF12 R6/R7).
//
// (1) SETTLED FROM THE RECORD. A marker whose text the requester already
// answered is not an open question — it was asked (S06.5) and it was answered,
// and the record proves it. Re-carding it is the witnessed confirm-loop: four
// rounds of a person re-confirming their own shop details, the last ask
// byte-identical to the one they had just answered. So it comes off the open
// list and the answer lands on the SPEC where the requester can see it, rather
// than evaporating with the one-shot ReviseReq that used to carry it.
//
// (2) CONVERTED AT THE BOUND. A marker still open when the clarification rounds
// are spent becomes a LISTED assumption — S06.6's own second arm, verbatim. It
// is never swallowed: it lands on the approval card's centerpiece, in the
// planner's own words, on the very card the requester was going to answer
// anyway, and Re-plan contests it. That is the difference between a bound and a
// gag.
//
// The disposition in both arms is a listed assumption (OQ-4) rather than a
// silent fact, because an arbitrary marker answer is not necessarily a P47
// supplied fact — and where the answer IS already recorded as one, the supplied
// entry stands alone and no duplicate is minted.
func settleMarkers(st *State, next *Pair) []markerDisclosure {
	if len(next.Spec.Clarifications) == 0 {
		return nil
	}
	var disclosures []markerDisclosure
	open := make([]string, 0, len(next.Spec.Clarifications))
	list := func(text string) {
		for _, a := range next.Spec.Assumptions {
			if a.Text == text {
				return
			}
		}
		next.Spec.Assumptions = append(next.Spec.Assumptions, Assumption{Text: text, Origin: AssumptionOriginMarker})
	}
	for _, m := range next.Spec.Clarifications {
		marker := strings.TrimSpace(m)
		if rec := st.settledMarker(marker); rec != nil {
			answer := strings.TrimSpace(rec.Answer)
			if !suppliedSays(next.Spec.Supplied, answer) {
				list(fmt.Sprintf("You were already asked this and you answered it — %s You said: %s", plainQuestion(marker), answer))
			}
			disclosures = append(disclosures, markerDisclosure{
				line: fmt.Sprintf("settled from the record: the plan raised an open point the requester had already answered (%q → %q); it is listed on the SPEC instead of being asked again", marker, answer),
				why:  "S06.6 first arm — the marker was asked (S06.5) and answered; the answered-marker record is the proof (P3-GF12 R6)",
			})
			continue
		}
		if st.ClarificationRounds >= clarificationRoundLimit {
			list(fmt.Sprintf("Still open when the questions ran out, so I am going ahead on my own best reading of it — change it here if I have read it wrong: %s", plainQuestion(marker)))
			disclosures = append(disclosures, markerDisclosure{
				line: fmt.Sprintf("converted to a listed assumption after %d clarification rounds: %q", clarificationRoundLimit, marker),
				why:  "S06.6 second arm — each marker is either asked or converted to a listed assumption; the assumption is contestable on the approval card (P3-GF12 R7)",
			})
			continue
		}
		open = append(open, m)
	}
	if len(open) == 0 {
		next.Spec.Clarifications = nil
	} else {
		next.Spec.Clarifications = open
	}
	return disclosures
}

// plainQuestion renders a marker as one sentence inside an assumption line: the
// planner's own words, punctuated so the assumption reads as prose rather than
// as two sentences colliding.
func plainQuestion(marker string) string {
	switch {
	case marker == "",
		strings.HasSuffix(marker, "."),
		strings.HasSuffix(marker, "?"),
		strings.HasSuffix(marker, "!"):
		return marker
	}
	return marker + "."
}

// suppliedSays reports whether a requester-supplied fact already carries this
// answer, in which case the supplied entry IS the record and a second listing
// of the same words would only make the card longer (OQ-4).
func suppliedSays(supplied []SuppliedFact, answer string) bool {
	for _, f := range supplied {
		if strings.TrimSpace(f.Fact) == answer {
			return true
		}
	}
	return false
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

// clarificationRoundLimit bounds the S06.6 marker loop: at most this many
// NEEDS-CLARIFICATION cards per intake, after which what is still open converts
// to a listed assumption (S06.6's own second arm).
//
// A plain structural constant, not a ⚙ — S18 declares no key for it, and it is
// the same class as maxQuestionsPerCard: it bounds CEREMONY, not a decision the
// platform makes (flagged to the operator gate under the standing settings-tab
// directive). It follows the shape its two siblings already have: coverage has
// ⚙ intake.coverage_autofix_rounds, research has its one bounce, and the marker
// loop had nothing at all — which is how a live intake reached a fourth round
// with a person re-confirming facts they had supplied in round one.
//
// G1 P8's "no fixed question caps" governs the S06.5 interview taxonomy, where
// the questions come from a weighted must-know set and stopping early costs
// understanding. This bounds a different thing: a model's own re-raised doubts,
// which do not run out on their own, and whose leftovers are LISTED rather than
// dropped — so nothing is lost, it just stops costing a human round.
const clarificationRoundLimit = 2

// handleEmissionFault lands a contract-invalid Stage-1 emission on a served
// decision card instead of crashing the run (P3-GF12 R3). It reports whether the
// pipeline is now parked on that card; any other error passes straight through
// unchanged.
//
// WHY A CARD. The seam has already spent its bounded re-emission with the
// refusal fed back verbatim, so what remains is a deterministic refusal — and
// the recovery ladder "can never fix a deterministic failure". The witnessed
// alternative was exactly that: the drive errored, mapDriveErr crashed the run,
// and the ladder forked the same Draft with the same prompt and ZERO new
// information until ⚙ recovery.max_attempts tombstoned the lineage — twice, on
// one task, with the second round opened by the tombstone card's own retry. The
// crash doctrine is right for infrastructure death (§56) and stays untouched;
// this class simply stops entering it.
//
// A BAND TASK CARDS TOO. S06.4's zero-interaction band bounds intake CEREMONY —
// questions, approval — and this is not ceremony: it is the platform saying it
// has nothing to give. The alternative is a dead run nobody was told about, and
// "no blocking gate" never meant "die silently".
func (p *Pipeline) handleEmissionFault(ctx context.Context, st *State, err error, op string) (bool, error) {
	if err == nil || !errors.Is(err, ErrBadArtifact) {
		return false, err
	}
	rounds := 1
	if st.EmissionFault != nil && st.EmissionFault.Op == op {
		rounds = st.EmissionFault.Rounds + 1
	}
	refusal := strings.TrimSpace(err.Error())
	st.EmissionFault = &EmissionFault{Op: op, Refusal: refusal, Rounds: rounds, TS: p.nowRFC3339()}
	if _, err := p.Ledger.RecordDecision(ctx, st.RunID, ledger.AuthorPlatform, run.ActorPlatform, "plan",
		"planner emission refused by the artifact contract and NOT accepted: "+refusal,
		"the seam's bounded re-emission is spent; the refusal goes to the requester as a decision card rather than to the recovery ladder (S06.6 [A15]; P3-GF12 R3)", 0); err != nil {
		return false, err
	}
	return true, p.issueCard(ctx, st, emissionCard(st.EmissionFault))
}

// emissionCard is the honest landing: what the platform tried, why it stopped,
// and the requester's real choices — in words for the person, not in the
// platform's vocabulary (CONVENTIONS §57 drafting rule 1, §59).
func emissionCard(fault *EmissionFault) *Card {
	what := "the plan"
	if fault.Op == EmissionOpRevise {
		what = "the revised plan"
	}
	detail := []string{
		"This is the platform's own check refusing what came back — not something that went wrong inside your task. Nothing was saved, nothing about your request changed, and no work has run.",
		"The planning model gets a bounded number of corrections, with this exact refusal handed back to it each time, before any of this reaches you. Those are spent. Nothing was shortened or rewritten on your behalf: what a model writes, the platform re-asks or refuses — it never edits it.",
		fmt.Sprintf("Planning rounds spent on %s and refused so far: %d.", what, fault.Rounds),
	}
	return &Card{Kind: CardEmission, Decision: &DecisionBody{
		Summary: fmt.Sprintf("I could not get %s into a shape this platform will accept: %s", what, fault.Refusal),
		Detail:  detail,
		Choices: []Option{
			{Label: "Try once more", Value: ChoiceReplan},
			{Label: "Stop here (cancel this task)", Value: ChoiceRethink},
		},
		Help: HelpBlock{
			What:      "Trying once more starts one more planning round, and that round costs what a planning round costs. Stopping here cancels this task; nothing has run either way, so nothing has to be undone.",
			Wrong:     "A fresh round can come back over the same limit again — the planning model writes the plan, and the platform will refuse it again rather than quietly cut it down to size.",
			Recommend: "Try once more first: the model is told the exact limit it overran. If it fails again, stop here and start over with a smaller or more specific request — a narrower job is easier to describe inside the limits.",
		},
	}}
}

// ---- Cards & gates ----

// issueCard persists the ask row, parks the run, and appends the state —
// one transaction. Gates wait; answering resumes the pipeline in place
// (Spec S06.1).
func (p *Pipeline) issueCard(ctx context.Context, st *State, card *Card) error {
	// The tier's ⚙ floor rides every card next to the computed Clearance, from
	// the one site that issues them all (R11): a meter that says how far along
	// the questions are, without saying where they stop, is a meter nobody can
	// read. Served, never derived — the floor VALUE and every use of it are
	// exactly as landed, and the trivial tier's 0 omits the field.
	floor, err := p.clearanceFloor(st.Tier)
	if err != nil {
		return err
	}
	st.CardVersion++
	askID := fmt.Sprintf("intake:%s:%d", st.TaskID, st.CardVersion)
	card.TaskID, card.RunID = st.TaskID, st.RunID
	card.Version = st.CardVersion
	card.IssuedTS = p.nowRFC3339()
	card.Clearance = st.Clearance
	card.ClearanceFloor = floor
	card.Tier = st.Tier
	switch card.Kind {
	case CardInterview, CardClarification, CardApproval:
		// The whole stakes truth travels with the cards a person decides at,
		// from the same single site (P3-GF14 R4.3): the tier, what set it, why
		// in plain words, any pending downward proposal, and whether the
		// requester's one downward move is legal. A surface reading this can
		// never disagree with the platform about the stakes.
		card.Stakes = stakesBlock(st)
	}
	if card.Kind != CardFamily {
		// The family guess travels on every card that HAS one, from the single
		// site that issues them all (P3-GF7 R9): a surface can say "I am
		// treating this as software work — my guess" before a question renders,
		// instead of leaving the requester to discover a misclassification
		// through four wrong questions (harvest H18). The family card omits it,
		// because family is exactly what is unresolved there.
		card.Family, card.FamilySource = st.Family, st.FamilySource
	}
	st.OpenAskID, st.OpenAskKind = askID, card.Kind
	st.CardIssuedTS = card.IssuedTS
	st.StaleFlag, st.StaleReasons = false, nil
	return p.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		if err := p.insertAskTx(ctx, tx, askID, st, card); err != nil {
			return err
		}
		if _, err := p.Runs.TransitionTx(ctx, tx, st.RunID, run.StateParked, run.TransitionOptions{
			// S06.1: an open gate parks the run until it is answered.
			Reason: fmt.Sprintf("waiting for your answer on the %s — nothing else happens until that card is answered", plainCardKind(card.Kind)),
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
		h, err := p.Utility.Help(ctx, *pair)
		if err != nil {
			// Same seat, same alias, same silence — the S06.9 Help block fell
			// back byte-identically to defaultHelp() on cold walk 1 and left no
			// trace either (PH-1 F3).
			p.logSeatDegrade("help", st.RunID, CardApproval, 0, err)
		} else {
			help = h
		}
	}
	steps := make([]string, 0, len(pair.Plan.Steps))
	for _, s := range pair.Plan.Steps {
		steps = append(steps, s.Title)
	}
	costTime := "UNPRICED (no price table; measured usage will appear on the receipt)"
	if pair.Plan.Est.Known {
		// D5: the figure is API-equivalent, never a bill — said the way the
		// receipt's own UNPRICED copy says it.
		costTime = fmt.Sprintf("about %.2f USD if these calls were bought one at a time — a size guide, not a bill: they ride a subscription and cost nothing extra", pair.Plan.Est.USD)
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
	// Layer 2 serves the WHOLE derived understanding, per field (P3-GF8 R4;
	// operator record r5 §B.1 + §C rule 7). The constraints and the
	// requester-supplied inputs were the two understanding fields the drafted-
	// plan surface did not serve, so a requester could not see — let alone
	// contest — what the platform believed it had been told to work within.
	// Everything else in the §B.1 list was already here or on Layer 1: goal
	// restatement → Layer1.Restatement; acceptance criteria → ACs; users and
	// data/integrations → the family slots in the Understood recap plus
	// Supplied and ResearchNodes; out-of-scope → Layer1.WillNotDo; risks →
	// Layer1.Risks; assumptions and decisions → Layer1.Assumptions plus the
	// recap. No SPEC member is invented: S06.6's enumeration is untouched, and
	// Layer 1 stays one phone screen [A15 "What does NOT change"].
	l2 := ApprovalLayer2{
		ACs: pair.Spec.ACs, Steps: pair.Plan.Steps, Coverage: pair.Plan.Coverage,
		Verdicts: st.Verdicts, ResearchNodes: pair.Plan.ResearchNodes,
		Estimate: pair.Plan.Est, SpecRef: st.SpecRef, PlanRef: st.PlanRef,
		Constraints: pair.Spec.Constraints, Supplied: pair.Spec.Supplied,
	}
	// The S06.9 action vocabulary is unchanged. The lowering door is OFFERED on
	// the stakes block instead — `Stakes.CanLower`, computed from the very rule
	// Pipeline.LowerTier enforces (§71: one rule, two readers) — because it is
	// a statement about the stakes and belongs where the stakes are served,
	// beside the tier it would move and the reason it stands where it does.
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
