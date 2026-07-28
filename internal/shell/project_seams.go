package shell

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
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
}

// registrySeam is the production intake.Registry (S06.2 step 2): match a
// request to an ACTIVE registry entry, projecting its captured conventions,
// commands (with source hashes) and danger zones into the intake consuming
// shape.
type registrySeam struct{ proj *project.Store }

func (r registrySeam) Match(ctx context.Context, req intake.Request) (intake.RegistrySlice, bool, error) {
	e, ok, err := r.proj.MatchForIntake(ctx, project.MatchHint{UserID: req.UserID, Title: req.Title, Text: req.Text})
	if err != nil || !ok {
		return intake.RegistrySlice{}, false, err
	}
	return toRegistrySlice(e), true, nil
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
		Project:     e.ProjectID,
		Ref:         fmt.Sprintf("%s@capture-v%d", e.ProjectID, e.CaptureVersion),
		Conventions: e.Capture.Conventions,
		Commands:    commands,
		DangerZones: zones,
	}
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
	if !strings.HasSuffix(runID, stage.RunSuffixExecute) {
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
