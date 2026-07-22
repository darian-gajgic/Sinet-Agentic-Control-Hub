package shell

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// deliverableTaskID derives a task id from THE task deliverable id
// (stage.TaskDeliverableID = "dlv-<task>"); "" for a shape that is not the
// task deliverable (composer-definition and other ids stay non-repo-backed).
func deliverableTaskID(deliverableID string) string {
	if t, ok := strings.CutPrefix(deliverableID, "dlv-"); ok {
		return t
	}
	return ""
}

// projectSeams is the composition-root adapter from internal/project to the
// narrow func-field seams stage and intake consume (Spec S13.5/S13.7). The git
// side is wired HERE — stage/intake/review never import internal/project
// (CONVENTIONS §35); the walls stay clean. A run resolves to its task's
// registered project by the deterministic S06.2 registry match on the task
// title; a task's runs share ONE run-branch worktree keyed by the task id
// (S02.10 "an isolated workspace" per task).
type projectSeams struct {
	proj *project.Store
	runs *run.Store
	db   *storage.DB
}

// registry is the production intake.Registry (S06.2 step 2): match a request
// to an ACTIVE registry entry, projecting its captured conventions, commands
// (with source hashes) and danger zones into the intake consuming shape.
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

// taskTitle reads a task's title (the deterministic registry-match input at
// execute/verify time; the intake request text is not persisted on the task).
func (s projectSeams) taskTitle(ctx context.Context, taskID string) (string, error) {
	var title string
	err := s.db.QueryRowContext(ctx, `SELECT title FROM tasks WHERE task_id = ?`, taskID).Scan(&title)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return title, err
}

// matchRun resolves a run to its task's active registered project (ok=false =
// no registered project → the plain run dir / content-pin lane).
func (s projectSeams) matchRun(ctx context.Context, runID string) (project.Entry, bool, error) {
	r, err := s.runs.Get(ctx, runID)
	if err != nil {
		return project.Entry{}, false, err
	}
	if r.TaskID == "" {
		return project.Entry{}, false, nil
	}
	title, err := s.taskTitle(ctx, r.TaskID)
	if err != nil || title == "" {
		return project.Entry{}, false, err
	}
	return s.proj.MatchForIntake(ctx, project.MatchHint{UserID: r.UserID, Title: title})
}

// workspacePath resolves (creating on first sight) the task's run-branch
// worktree for a run of a matched-project task.
func (s projectSeams) workspacePath(ctx context.Context, runID string) (string, bool, error) {
	e, ok, err := s.matchRun(ctx, runID)
	if err != nil || !ok {
		return "", false, err
	}
	r, err := s.runs.Get(ctx, runID)
	if err != nil {
		return "", false, err
	}
	ws, err := s.proj.EnsureWorkspace(ctx, e.ProjectID, r.TaskID)
	if err != nil {
		return "", false, err
	}
	return ws.Path, true, nil
}

// WorkspaceCwd is the stage runner's cwd seam (R34).
func (s projectSeams) WorkspaceCwd(ctx context.Context, runID string) (string, bool, error) {
	return s.workspacePath(ctx, runID)
}

// Snapshot is the Driver.Snapshot / round-boundary snapshot seam (R17/R19):
// the run's workspace snapshot commit sha, or "" for a workspace-less run.
func (s projectSeams) Snapshot(ctx context.Context, runID string) (string, error) {
	path, ok, err := s.workspacePath(ctx, runID)
	if err != nil || !ok {
		return "", err
	}
	return s.proj.Snapshot(ctx, path)
}

// CreateRevisionRef creates the minted-revision platform ref in the run's
// project store (R20); a workspace-less run has no project to ref.
func (s projectSeams) CreateRevisionRef(ctx context.Context, runID, ref, snapshotSHA string) error {
	e, ok, err := s.matchRun(ctx, runID)
	if err != nil || !ok {
		return err
	}
	return s.proj.CreateRevisionRef(ctx, e.ProjectID, ref, snapshotSHA)
}

// Fingerprint supplies the matched project's real repo HEAD for the S02.6/
// S09.6 approval-staleness fingerprint (R33); unobservable members stay empty.
func (s projectSeams) Fingerprint(ctx context.Context, projectID string) (run.Fingerprint, error) {
	if projectID == "" {
		return run.Fingerprint{}, nil
	}
	head, err := s.proj.RepoHead(ctx, projectID)
	if err != nil {
		// A missing/pending project is not an error at the fingerprint probe —
		// it simply supplies no repo HEAD (never faked, R33).
		return run.Fingerprint{}, nil
	}
	return run.Fingerprint{RepoHead: head}, nil
}

// BaseContent resolves a repo-backed deliverable's pre-task base content for
// Compare's old-side 0 (R25). The deliverable id derives its task
// (dlv-<task>), whose run-branch base is refs/sinet/base/<task>. Not repo-
// backed (no matching active project) → ok=false, the empty base.
func (s projectSeams) BaseContent(ctx context.Context, deliverableID string) (map[string]string, bool, error) {
	taskID := deliverableTaskID(deliverableID)
	if taskID == "" {
		return nil, false, nil
	}
	title, err := s.taskTitle(ctx, taskID)
	if err != nil || title == "" {
		return nil, false, err
	}
	// The base is owner-agnostic here; match visibly-active projects by title.
	e, ok, err := s.proj.MatchForIntakeAny(ctx, project.MatchHint{Title: title})
	if err != nil || !ok {
		return nil, false, err
	}
	files, err := s.proj.BaseContent(ctx, e.ProjectID, taskID)
	if err != nil {
		return nil, false, err
	}
	if len(files) == 0 {
		return nil, false, nil
	}
	return files, true, nil
}
