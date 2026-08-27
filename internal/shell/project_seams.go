package shell

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// deliverableTaskID derives a task id from THE task deliverable id
// (stage.TaskDeliverableID = "dlv-<task>") so the base-content seam can resolve
// the deliverable's own pipeline (F16c). Any id that is not that shape
// (composer definitions "dlv-<task>-def", other prefixes) yields a task id that
// resolves no registered project → the empty base.
func deliverableTaskID(deliverableID string) string {
	if t, ok := strings.CutPrefix(deliverableID, "dlv-"); ok {
		return t
	}
	return ""
}

// projectSeams is the composition-root adapter from internal/project to the
// narrow func-field seams stage and intake consume (Spec S13.5/S13.7). The git
// side is wired HERE — stage/intake/review never import internal/project
// (brief R35; CONVENTIONS §23); the walls stay clean. A run resolves to its
// task's registered project by the DURABLE intake-time match recorded on the
// intake state (st.Registry.Project, owner-scoped at S06.2 triage) — never by
// re-matching titles, so a cross-user name collision can never supply another
// user's project (F3). A task's runs share ONE run-branch worktree keyed by the
// task id (S02.10 "an isolated workspace" per task).
type projectSeams struct {
	proj *project.Store
	runs *run.Store
	db   *storage.DB
	// pipe is late-bound after stage.New (the pipeline is built inside it);
	// projectForTask reads the durable intake match through it.
	pipe *intake.Pipeline
	// prices supplies the S02.6 fingerprint's price-table member (B6-3A). It is
	// the narrowest possible view of the composed table — a version string —
	// because that is all a fingerprint needs and nothing here should be able
	// to price anything.
	prices interface{ Version() string }
	// review resolves a deliverable revision's content pin (its snapshot
	// commit) for the R6 verification-workspace seam. Late-bound: the review
	// store is composed after these seams are.
	review *review.Store
	// scratch is the platform-owned root the R6 materializations are built
	// under (<state-dir>/verify-workspaces) — never system temp, the §25
	// preview-clones precedent.
	scratch string
}

// registrySeam is the production intake.Registry (S06.2 step 2): match a
// request to an ACTIVE registry entry, projecting its captured conventions,
// commands (with source hashes) and danger zones into the intake consuming
// shape.
type registrySeam struct{ proj *project.Store }

func (r registrySeam) Match(ctx context.Context, req intake.Request) (intake.RegistrySlice, bool, error) {
	// A SUBMITTED pin resolves the entry directly and never consults the text
	// (P3-RW-1; the Projects-tab door hands the registry id). Its validation —
	// the requester owns or belongs to it, and it is ACTIVE — is the project
	// store's own, so the visibility predicate stays in one package (S15.2:
	// authorization is enforced server-side, once).
	//
	// The field is taken VERBATIM (drain r1 F3): non-empty as submitted IS a
	// pin attempt, and a padded or blank value simply names no project. The
	// durable record marshals the same bytes, so what was recorded and what
	// was resolved can never diverge.
	if pin := req.Project; pin != "" {
		e, err := r.proj.PinForIntake(ctx, pin, req.UserID)
		if err != nil {
			return intake.RegistrySlice{}, false, pinRefusal(pin, err)
		}
		return toRegistrySlice(e), true, nil
	}
	e, ok, err := r.proj.MatchForIntake(ctx, project.MatchHint{UserID: req.UserID, Title: req.Title, Text: req.Text})
	if err != nil || !ok {
		return intake.RegistrySlice{}, false, err
	}
	return toRegistrySlice(e), true, nil
}

// pinRefusal translates the store's refusal into the intake sentinel the
// pipeline maps to its 4xx. A store error that is not a refusal (a database
// failure) stays itself; on a pinned request Start propagates it loudly all
// the same — only the unpinned scan path degrades on seam errors.
func pinRefusal(pin string, err error) error {
	switch {
	case errors.Is(err, project.ErrNotActive):
		return fmt.Errorf("%w: %q", intake.ErrPinNotActive, pin)
	case errors.Is(err, project.ErrNotFound):
		return fmt.Errorf("%w: %q", intake.ErrPinUnknown, pin)
	default:
		return err
	}
}

// onboardDoor is the api-facing S13.7 onboarding seam (api.OnboardSurface,
// P3-RW-2): the HTTP create door reaches the onboarding task through it.
//
// It is composed HERE, at the root that already holds both organs, rather than
// on *stage.Surface, because the door records one field the landed stage seam
// has no parameter for: the entry's REMOTE URL, which S13.7 stores as data and
// which nothing ever dials (§23). Widening Config.OnboardStart /
// Skeleton.StartOnboarding to carry it would change landed signatures other
// compositions consume, for a value the run substrate does not use — and this
// file is already where the registry↔consumer translation lives (pinRefusal).
//
// THE TWO CALLS ARE ONE ONBOARDING. project.OnboardStart returns the pending
// entry's EXISTING draft rather than re-cloning when the entry is already
// registered — the same idempotent re-dispatch path the onboarding run itself
// takes (S13.7/F1) — so register → init → scan → draft happens exactly once and
// the skeleton's own call is a read.
type onboardDoor struct {
	proj *project.Store
	sk   *stage.Skeleton
}

var _ api.OnboardSurface = onboardDoor{}

// OnboardRefs names the onboarding of a project without performing anything, so
// a retried create can answer with the references of the one already running.
func (d onboardDoor) OnboardRefs(projectID string) api.OnboardRefs {
	return api.OnboardRefs{TaskID: stage.OnboardTaskID(projectID), AskRef: stage.OnboardAskID(projectID)}
}

// StartOnboarding registers and drafts the entry (with its stored remote), then
// launches the onboarding task whose durable ask carries the draft to its owner.
func (d onboardDoor) StartOnboarding(ctx context.Context, owner, projectID, name, remoteURL string) (api.OnboardRefs, error) {
	return d.StartOnboardingWithFamily(ctx, owner, projectID, name, remoteURL, "")
}

// StartOnboardingWithFamily is the same onboarding with the owner's declared
// task family (P3-RW-11 R2/R10). It is a SECOND method rather than a widened
// StartOnboarding because api.OnboardSurface is a landed interface with other
// implementors, and S15.2's additive rule is what keeps them compiling
// unchanged: a door that predates the family keeps working and simply carries
// none. The family is a door-only field — it lands on project.OnboardInput
// here, at the root, exactly as RemoteURL does, and no stage seam widens (§23).
func (d onboardDoor) StartOnboardingWithFamily(ctx context.Context, owner, projectID, name, remoteURL, family string) (api.OnboardRefs, error) {
	if _, err := d.proj.OnboardStart(ctx, project.OnboardInput{
		ProjectID: projectID, Owner: owner, Name: name, RemoteURL: remoteURL, Family: family,
	}); err != nil {
		return api.OnboardRefs{}, onboardRefusal(err)
	}
	// Source stays EMPTY: over HTTP a clone source is a host-filesystem read
	// primitive, so the door cannot express one (P3-RW-2 OQ5) and every
	// onboarding started here initializes a fresh store.
	taskID, err := d.sk.StartOnboarding(ctx, owner, projectID, name, "")
	if err != nil {
		return api.OnboardRefs{}, err
	}
	return api.OnboardRefs{TaskID: taskID, AskRef: stage.OnboardAskID(projectID)}, nil
}

// onboardRefusal translates the project store's refusals into transport errors
// on the SENTINEL, never on the message text (§38's ban) — pinRefusal's
// discipline one seam over. Anything unmarked stays itself and answers 500 with
// the cause in the ops log: a platform fault is not the caller's fault.
func onboardRefusal(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, project.ErrAlreadyRegistered):
		return &api.SurfaceError{Status: http.StatusConflict, Code: "already_registered", Msg: err.Error()}
	case errors.Is(err, project.ErrBadInput):
		return &api.SurfaceError{Status: http.StatusBadRequest, Code: "bad_request", Msg: err.Error()}
	default:
		return err
	}
}

// toRegistrySlice projects an active entry's current capture into the intake
// shape. Danger zones carry their source hashes (the ledger §2 pin, S05.1);
// commands flatten to the intake []string surface with their slot names.
func toRegistrySlice(e project.Entry) intake.RegistrySlice {
	zones := make([]intake.DangerZone, 0, len(e.Capture.DangerZones))
	for _, z := range e.Capture.DangerZones {
		rule := z.Rule
		if z.Action != "" {
			rule = z.Action + ": " + z.Rule
		}
		zones = append(zones, intake.DangerZone{Path: z.Path, Rule: rule, SourceHash: z.SourceHash})
	}
	var commands []string
	for _, c := range []struct{ name, val string }{
		{"build", e.Capture.Commands.Build}, {"test", e.Capture.Commands.Test},
		{"lint", e.Capture.Commands.Lint}, {"run", e.Capture.Commands.Run},
		{"preview", e.Capture.Commands.Preview},
	} {
		if c.val != "" {
			commands = append(commands, c.name+": "+c.val)
		}
	}
	return intake.RegistrySlice{
		Project: e.ProjectID,
		Ref:     fmt.Sprintf("%s@capture-v%d", e.ProjectID, e.CaptureVersion),
		// The captured task family, which is what makes the pipeline's landed
		// `if slice.Family != "" { st.Family = slice.Family }` fire at all
		// (P3-RW-11 R1): a project whose owner declared "software" opens the
		// software question set instead of the generic one. An undeclared family
		// stays empty — the slice carries none and the interview ASKS, rather
		// than the platform assuming on the project's behalf.
		Family:      intake.Family(e.Capture.Family),
		Conventions: e.Capture.Conventions,
		Commands:    commands,
		DangerZones: zones,
	}
}

// CheckPackFor builds the V1 check pack for one (domain, task) from the S13.7
// registry capture (P3-RW-14 R5) — the wire that was missing when the whole
// software family crashed at verify: `stage.Config.CheckPacks` shipped empty,
// so every launch-domain deliverable hit ErrNoCheckPack, and the ladder forked
// that deterministic refusal to a tombstone.
//
// "The registry feeds … preview commands" (S13.7) and S07.3's ladder rungs are
// lint/typecheck/build → tests → smoke, so the captured commands ARE the pack:
// lint and build are the static rung, test the unit rung. `run` and `preview`
// are deliberately NOT checks — they start something and wait, which is a
// preview (S13.8), not a verdict.
//
// The pack carries the capture's own timestamp as its S07.3 rule-7 verified-on
// stamp (P-T06-1): a suite is exactly as fresh as the scan it came from, and
// ⚙ verification.check_audit_interval_days flags the verdict stale from there.
//
// Four honest answers, and the drain depends on the difference:
//
//   - (nil, nil) — this domain has no pack machinery (non-launch domains keep
//     the ratified degraded mode, Spec S07.8);
//   - (pack, nil) — run it;
//   - (bootstrap pack, nil) — the project is registered and holds no
//     build/test/lint command, so there is no rung to run YET. Spec S07.8's
//     bootstrap posture (A14, 2026-08-27) defines that landing: the drain runs
//     and records every rung UNVERIFIABLE-HERE rather than refusing;
//   - an ErrNoCheckPack error NAMING WHAT IS MISSING — a genuine integrity
//     case, which after A14 means the task is attached to no registered
//     project at all: there is no capture to compute a posture from, and its
//     door (register the project, attach the task) exists today. The verify leg
//     turns that into the operator's decision card and parks (R4). Nothing is
//     invented on the project's behalf and no launch domain is ever silently
//     degraded (OQ3(i)).
func (s *projectSeams) CheckPackFor(ctx context.Context, domain, taskID string) (*verify.CheckPack, error) {
	if !verify.LaunchDomain(domain) {
		return nil, nil
	}
	projectID, err := s.projectForTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if projectID == "" {
		return nil, fmt.Errorf("%w: this software task is not attached to a registered project, so the platform has no build or test commands to check it with — register the project (Projects tab), attach the task to it, then retry",
			verify.ErrNoCheckPack)
	}
	e, err := s.proj.Get(ctx, projectID)
	if err != nil {
		return nil, err // a registry read failure is mechanical, never a card
	}
	return packFromCapture(domain, e)
}

// VerificationWorkspace materializes the revision under review for the V1
// checks, with answer-bearing VCS history STRIPPED (Spec S07.3 rule 1,
// P-T06-2; P3-RW-14 R6). Substrate is the ratified §25 pair: a LOCKED utility
// checkout at the revision's own snapshot pin, then a `.git`-less copy of it.
//
// Why not hand the checks the worktree, or the checkout itself: with history
// present, a check (or an engine reading the evidence) can look up the answer
// instead of deriving it, which is the measured failure mode P-T06-2 names —
// the pass would score retrieval, not work. And a locked checkout is platform
// state; the checks get a throwaway copy so nothing they do can dirty it.
//
// An empty dir with a nil error is the honest absence the caller falls back
// on: no project, or no snapshot pin (a non-repo-backed deliverable) means
// there is no revision tree to materialize — not that verification failed.
func (s *projectSeams) VerificationWorkspace(ctx context.Context, taskID string, revision int) (string, func(), error) {
	if s.review == nil {
		return "", nil, nil
	}
	projectID, err := s.projectForTask(ctx, taskID)
	if err != nil || projectID == "" {
		return "", nil, err
	}
	rev, err := s.review.RevisionAt(ctx, stage.TaskDeliverableID(taskID), revision)
	if err != nil {
		return "", nil, err
	}
	if rev.SnapshotSHA == "" {
		return "", nil, nil // content-pin lane: nothing repo-backed to check out
	}

	checkout, err := s.proj.AddUtilityCheckout(ctx, projectID, rev.SnapshotSHA)
	if err != nil {
		return "", nil, err
	}
	release := func() {
		if rerr := s.proj.ReleaseUtilityCheckout(ctx, projectID, checkout.Path); rerr != nil {
			// A leaked checkout is a defect worth seeing, never a reason to
			// fail a verification that has already run.
			log.Printf("shell: release verification checkout %s: %v", checkout.Path, rerr)
		}
	}
	dir, err := strippedCopy(s.scratch, checkout.Path)
	if err != nil {
		release()
		return "", nil, err
	}
	return dir, func() {
		if rerr := os.RemoveAll(dir); rerr != nil {
			log.Printf("shell: remove verification workspace %s: %v", dir, rerr)
		}
		release()
	}, nil
}

// strippedCopy copies src into a fresh dir under scratchRoot WITHOUT its VCS
// history — the §25 copyTree precedent, kept local rather than shared because
// the two callers answer to different specs (S13.8 zero-mutation previews vs
// S07.3 rule 1 verification) and a shared helper would couple them.
//
// A worktree's `.git` is a FILE pointer, not a directory, so both forms are
// skipped. Symlinks and devices are dropped: the checks read plain source, and
// a symlink out of the workspace is exactly the escape this strip exists to
// prevent.
func strippedCopy(scratchRoot, src string) (string, error) {
	if scratchRoot == "" {
		return "", fmt.Errorf("shell: no scratch root for the verification workspace")
	}
	if err := os.MkdirAll(scratchRoot, 0o700); err != nil {
		return "", fmt.Errorf("shell: verification scratch root: %w", err)
	}
	dst, err := os.MkdirTemp(scratchRoot, "verify-")
	if err != nil {
		return "", fmt.Errorf("shell: verification workspace dir: %w", err)
	}
	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if d.Name() == ".git" {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		// Preserve the executable bit: a check that runs ./script needs it.
		mode := os.FileMode(0o600)
		if info.Mode()&0o100 != 0 {
			mode = 0o700
		}
		return os.WriteFile(target, data, mode)
	})
	if err != nil {
		_ = os.RemoveAll(dst)
		return "", err
	}
	return dst, nil
}

// RepoFacts reports the durable repo-backed platform facts for a task's
// execute leg: the snapshot commit its work landed on, and the attempt's
// recorded base ref. Both feed the S07.2 wrote-nothing gate (P3-RW-14 R7),
// which is why this reads and never writes — see project.SnapshotAndBase.
//
// Empty strings are honest absences (no project, no worktree, no recorded
// base), and the gate declines to fire on any of them.
func (s *projectSeams) RepoFacts(ctx context.Context, taskID string) (snapshot, base string, err error) {
	projectID, err := s.projectForTask(ctx, taskID)
	if err != nil || projectID == "" {
		return "", "", err
	}
	return s.proj.SnapshotAndBase(ctx, projectID, taskID)
}

// packFromCapture is the capture→pack projection, pure over one registry entry.
func packFromCapture(domain string, e project.Entry) (*verify.CheckPack, error) {
	checks := packChecks(e.Capture.Commands)
	if len(checks) == 0 {
		// The fresh-scaffold case: a REGISTERED project holding no build, test
		// or lint command has no executable rung, and Spec S07.8's bootstrap
		// posture (A14, 2026-08-27) is its landing. The drain runs, records
		// every rung UNVERIFIABLE-HERE and marks its verdict advisory;
		// capturing the project's commands restores the full ladder on the
		// next revision.
		//
		// The capture date is deliberately not required here: it stamps a
		// suite's freshness (rule 7) and there is no suite to stamp.
		return verify.BootstrapPack(domain, e.Capture.Version), nil
	}
	capturedAt, err := time.Parse(time.RFC3339Nano, e.Capture.CapturedTS)
	if err != nil {
		return nil, fmt.Errorf("%w: project %q has commands but no readable capture date, so the platform cannot say how fresh its checks are — re-scan the project, then retry",
			verify.ErrBadPack, e.Name)
	}
	pack := &verify.CheckPack{
		Domain:     domain,
		Version:    e.Capture.Version,
		VerifiedOn: capturedAt,
		Checks:     checks,
	}
	if err := pack.Validate(); err != nil {
		return nil, err // ErrBadPack — the card names it; nothing runs half-checked
	}
	return pack, nil
}

// packChecks maps the captured commands onto the S07.3 ladder rungs. Each
// check runs as one shell line inside the network-off verification sandbox
// (the SandboxCheckRunner, class C2 — P-T06-2), and the verdict is the exit
// status read platform-side, never anything the command says about itself
// (S07.3 rule 3).
//
// None of them is an ACCEPTANCE check: they originate from the project's own
// conventions, not from the frozen ACs, so they carry no AC key and claim no
// separate-context provenance (S07.3 rule 4 — a doer-written test never passes
// by construction here, because these are not passed off as AC evidence).
func packChecks(c project.Commands) []verify.Check {
	var checks []verify.Check
	for _, r := range []struct {
		id    string
		stage verify.LadderStage
		cmd   string
	}{
		{"lint", verify.StageStatic, c.Lint},
		{"build", verify.StageStatic, c.Build},
		{"test", verify.StageUnit, c.Test},
	} {
		if strings.TrimSpace(r.cmd) == "" {
			continue
		}
		checks = append(checks, verify.Check{
			ID:    r.id,
			Stage: r.stage,
			Argv:  []string{"/bin/sh", "-lc", r.cmd},
			// A failing project check says the work does not meet the bar the
			// project itself set: an AC-blocker's route (rework round, then a
			// requester decision card at cap — Spec S07.7).
			FindingCategory: verify.CatACBlocker,
		})
	}
	return checks
}

// projectForTask resolves a task's registered project via the durable intake-
// time match (st.Registry.Project). "" = no project (no intake state, or no
// registry entry matched at triage). Owner-scoping is inherited: the intake
// match at S06.2 already filtered to entries the requester could see.
func (s *projectSeams) projectForTask(ctx context.Context, taskID string) (string, error) {
	if s.pipe == nil || taskID == "" {
		return "", nil
	}
	st, err := s.pipe.LoadState(ctx, taskID)
	if err != nil {
		if errors.Is(err, intake.ErrNoState) {
			return "", nil
		}
		return "", err
	}
	if st.Registry == nil {
		return "", nil
	}
	return st.Registry.Project, nil
}

// WorkspaceCwd is the stage runner's cwd seam (R34), SCOPED to execute legs
// (F4): intake/verify/compose/helper ceremony sessions get no worktree, no
// branch, no snapshot. It creates the task's run-branch worktree on first call
// and records runs.workspace_ref (R14).
func (s *projectSeams) WorkspaceCwd(ctx context.Context, runID string) (string, bool, error) {
	// The role gate is FORK-AWARE through the platform's one shared suffix
	// matcher (P3-RW-6 R12/OQ5): a recovery fork `<task>.execute.g1` is the same
	// execution under a new identity (Spec S02.5 step 3), and the raw suffix test
	// refused it — the resumed execution silently lost the worktree and snapshot
	// lane its parent had, writing into the plain run dir instead. The lane
	// itself is per (project, task), so the fork resolves to the same worktree.
	if !strings.HasSuffix(metering.StripForkSuffix(runID), stage.RunSuffixExecute) {
		return "", false, nil
	}
	r, err := s.runs.Get(ctx, runID)
	if err != nil {
		return "", false, err
	}
	projectID, err := s.projectForTask(ctx, r.TaskID)
	if err != nil || projectID == "" {
		return "", false, err
	}
	ws, err := s.proj.EnsureWorkspace(ctx, projectID, r.TaskID)
	if err != nil {
		return "", false, err
	}
	if r.WorkspaceRef != ws.Path {
		if err := s.runs.SetWorkspaceRef(ctx, runID, ws.Path); err != nil {
			return "", false, err
		}
	}
	return ws.Path, true, nil
}

// existingWorkspacePath resolves a run's task worktree WITHOUT creating one
// (the snapshot/ref-creation resolution for the verify-leg mint — the worktree
// was created by the execute leg). "" when the task has no registered project
// or no worktree yet (intake before execute → workspace-less, F4).
func (s *projectSeams) existingWorkspacePath(ctx context.Context, runID string) (string, error) {
	r, err := s.runs.Get(ctx, runID)
	if err != nil {
		return "", err
	}
	projectID, err := s.projectForTask(ctx, r.TaskID)
	if err != nil || projectID == "" {
		return "", err
	}
	path, ok, err := s.proj.ExistingWorkspace(ctx, projectID, r.TaskID)
	if err != nil || !ok {
		return "", err
	}
	return path, nil
}

// Snapshot is the Driver.Snapshot / round-boundary snapshot seam (R17/R19):
// the run's task-workspace snapshot commit sha, or "" for a workspace-less run
// (the honest content-pin lane). Never creates a worktree — an intake-leg
// checkpoint leaves the artifact block honestly empty (F4).
func (s *projectSeams) Snapshot(ctx context.Context, runID string) (string, error) {
	path, err := s.existingWorkspacePath(ctx, runID)
	if err != nil || path == "" {
		return "", err
	}
	return s.proj.Snapshot(ctx, path)
}

// CreateRevisionRef creates the minted-revision platform ref in the run's
// project store (R20); a workspace-less run has no project to ref.
func (s *projectSeams) CreateRevisionRef(ctx context.Context, runID, ref, snapshotSHA string) error {
	r, err := s.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	projectID, err := s.projectForTask(ctx, r.TaskID)
	if err != nil || projectID == "" {
		return err
	}
	return s.proj.CreateRevisionRef(ctx, projectID, ref, snapshotSHA)
}

// Fingerprint supplies the observable members of the S02.6/S09.6 approval-
// staleness fingerprint (R33): the matched project's real repo HEAD, and — from
// B6-3A — the price table's version. Members nothing can observe stay empty.
//
// The price-table member is OBSERVABLE now, which it was not before: until the
// table had a durable home (migration 0019) its version was a fixed label, so
// filling this in would have been fabricating a constant. It is now the digest
// of the stored row set, so it MOVES exactly when a price moves — which is the
// whole point of the member. S10.3 makes price drift a first-class staleness
// cause ("it re-prices the remaining plan, so any change fires the freshness
// re-validation trigger", G1 Def.5), and this is the wire that carries it.
//
// It rides even when no project matched: a price is not a property of a
// project, and a plan parked against a table that has since changed is stale
// whether or not the platform could resolve its repo.
func (s *projectSeams) Fingerprint(ctx context.Context, projectID string) (run.Fingerprint, error) {
	fp := run.Fingerprint{}
	if s.prices != nil {
		fp.PriceTableVersion = s.prices.Version()
	}
	if projectID == "" {
		return fp, nil
	}
	head, err := s.proj.RepoHead(ctx, projectID)
	if err != nil {
		// A missing/pending project is not an error at the fingerprint probe —
		// it simply supplies no repo HEAD (never faked, R33).
		return fp, nil
	}
	fp.RepoHead = head
	return fp, nil
}

// BaseContent resolves a repo-backed deliverable's pre-task base content for
// Compare's old-side 0 (R25), OWNER-SCOPED through the deliverable's own
// pipeline: dlv-<task> → the task's durable intake project → base ref. Not
// repo-backed (no registered project for that task) → ok=false, the empty
// base. No name-based cross-user resolution is possible (F3).
func (s *projectSeams) BaseContent(ctx context.Context, deliverableID string) (map[string]string, bool, error) {
	taskID := deliverableTaskID(deliverableID)
	projectID, err := s.projectForTask(ctx, taskID)
	if err != nil || projectID == "" {
		return nil, false, err
	}
	files, err := s.proj.BaseContent(ctx, projectID, taskID)
	if err != nil {
		return nil, false, err
	}
	if len(files) == 0 {
		return nil, false, nil
	}
	return files, true, nil
}
