package shell

import (
	"context"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/accept"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/broker"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// The S13.6 accept orchestration + the S13.9 follow-up verb, constructed at the
// composition root from the PRODUCTION stores (F1) so they are compiled into the
// binary and reachable. The HTTP/answer invocation surface (with the S01.9 PIN
// step-up) is the named B6 seam — NOT built here; the api layer holds these for
// it. Both shell.Run and the composition test call buildAcceptSurface (the
// B4-2 F9 real-seams precedent — no test-local mirror of the wiring).

// acceptSurface bundles the two S13 operator verbs.
type acceptSurface struct {
	Accepter *accept.Accepter
	FollowUp *intake.FollowUp
}

// acceptDeps are the production stores the accept surface composes over.
type acceptDeps struct {
	Proj     *project.Store
	Journal  *gates.Journal
	Review   *review.Store
	Runs     *run.Store
	DB       *storage.DB
	Log      *eventlog.Log
	Settings run.FreshnessSettings
	// BrokerSocket is the per-user broker UDS the push/sign/posture seams dial
	// per call (the broker is a separate process — no persistent connection,
	// the CredInject precedent). v0 single-operator uses the one operator
	// broker; a per-accepting-user socket is a post-v0 concern.
	BrokerSocket string
	// Pipeline is the intake pipeline: its Start enters a follow-up successor
	// into normal intake (S13.9).
	Pipeline *intake.Pipeline
	// ProjectForTask resolves a run's task to its project (the durable intake
	// match) for the sibling-accept producer's active-run scan.
	ProjectForTask func(ctx context.Context, taskID string) (string, error)
}

// dialPusher dials the broker per push and CAS-pushes through it (S13.6/S11.5).
type dialPusher struct{ socket string }

func (p dialPusher) Push(req broker.Request) (broker.PushResult, error) {
	c, err := broker.Dial(p.socket)
	if err != nil {
		return broker.PushResult{}, err
	}
	defer c.Close()
	return c.Push(req)
}

// buildAcceptSurface constructs the accept orchestration + follow-up verb from
// the production stores. The broker seams (push, SSH sign, signing-posture
// presence check) dial the broker per call; the signing posture is derived from
// the user's git-ssh-key presence (F3, structural per-user); the sibling-accept
// producer scans active project runs.
func buildAcceptSurface(d acceptDeps) (*acceptSurface, error) {
	signer := func(profile, namespace string, data []byte) ([]byte, error) {
		c, err := broker.Dial(d.BrokerSocket)
		if err != nil {
			return nil, err
		}
		defer c.Close()
		return c.SignData(profile, namespace, data)
	}
	posture := func(_ context.Context, user string) (bool, string, error) {
		profile := user + "-git"
		c, err := broker.Dial(d.BrokerSocket)
		if err != nil {
			return false, "", err
		}
		defer c.Close()
		kind, has, err := c.HasKey(profile)
		return has && kind == broker.KindGitSSHKey, profile, err
	}
	activeRuns := func(ctx context.Context, projectID string) ([]run.SiblingAcceptRun, error) {
		active, err := d.Runs.InStates(ctx,
			run.StateNew, run.StateQueued, run.StateClaimed, run.StateRunning, run.StateParked, run.StateDraining)
		if err != nil {
			return nil, err
		}
		var out []run.SiblingAcceptRun
		for _, r := range active {
			pid, err := d.ProjectForTask(ctx, r.TaskID)
			if err != nil {
				return nil, err
			}
			if pid != "" && pid == projectID {
				out = append(out, run.SiblingAcceptRun{RunID: r.ID})
			}
		}
		return out, nil
	}
	accepter, err := accept.New(accept.Config{
		Project: d.Proj, Journal: d.Journal, Push: dialPusher{socket: d.BrokerSocket}, Review: d.Review,
		Signer: signer, SigningPosture: posture, ActiveRuns: activeRuns, Freshness: d.Settings,
	})
	if err != nil {
		return nil, err
	}
	start := func(ctx context.Context, req intake.Request) error {
		_, e := d.Pipeline.Start(ctx, req)
		return e
	}
	return &acceptSurface{
		Accepter: accepter,
		FollowUp: &intake.FollowUp{DB: d.DB, Log: d.Log, Start: start},
	}, nil
}

// acceptAccepter / acceptFollowUp are nil-safe accessors (the accept surface is
// unbuilt in the opts.Admission-override test path).
func acceptAccepter(s *acceptSurface) *accept.Accepter {
	if s == nil {
		return nil
	}
	return s.Accepter
}

func acceptFollowUp(s *acceptSurface) *intake.FollowUp {
	if s == nil {
		return nil
	}
	return s.FollowUp
}
