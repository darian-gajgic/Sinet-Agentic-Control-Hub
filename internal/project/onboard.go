package project

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Onboarding a repository is itself a task the platform performs (Spec
// S13.7): register → clone into the project store → scan → draft conventions,
// commands and danger zones → the owner approves the draft (D10) → the entry
// goes active. This file owns the deterministic platform steps — clone/init,
// the scan/draft heuristics, and the owner-gated approval. The scan/draft is
// DETERMINISTIC platform code at v0 (S13.7 names no model duty; none is
// invented — S06.2 deterministic-first posture); the owner edits/extends at
// the approval card. The run-substrate realization of the draft as a durable
// `asks` row answered by the owner (the run-role/park-resume half of "a task
// the platform performs") is the composition's wiring (CONVENTIONS §16); the
// pending entry with its drafted capture is that approval's durable record.

// Draft is the scan's proposed capture, presented for owner approval.
type Draft struct {
	Conventions []string     `json:"conventions,omitempty"`
	Commands    Commands     `json:"commands"`
	DangerZones []DangerZone `json:"danger_zones,omitempty"`
	// ScanHash fingerprints the scanned source (the drift-check baseline).
	ScanHash string `json:"scan_hash,omitempty"`
	// Family is the owner-declared task family (P3-RW-11 R2). It rides the
	// draft so the owner SEES it on the card they approve and can edit it
	// there, exactly as they edit conventions. omitempty keeps a family-less
	// draft byte-identical to a pre-RW-11 one.
	Family string `json:"family,omitempty"`
}

// OnboardInput drives register → clone/init → scan → draft (Spec S13.7).
type OnboardInput struct {
	ProjectID string
	Owner     string
	Name      string
	// Source is a LOCAL fixture repo path to clone (dev-mode: no network
	// remotes anywhere — the packet constraint). Empty creates a fresh empty
	// repo in the project store (Spec S13.7: "a v0 project without a repo gets
	// one created/registered by the onboarding task" — documents ride the
	// identical machinery).
	Source string
	// RemoteURL is stored data, never dialed (Spec S13.7).
	RemoteURL string
	// DefaultBranch overrides the created/detected default branch (default
	// "main").
	DefaultBranch string
	// Family is the owner-declared task family (P3-RW-11 R2), validated against
	// the six-value vocabulary or "". It reaches here from the create door at
	// the composition root — a door-only field, never a widened stage seam (the
	// RemoteURL precedent, §23).
	Family string
}

// Onboard performs the platform's onboarding task up to the owner-approval
// point: it creates the project store (clone from the local source, or init a
// fresh repo for the no-repo case), registers a PENDING entry, scans the
// store, and records the drafted conventions/commands/danger-zones as the
// entry's first capture. The returned entry is pending; the owner approves via
// Approve (D10), which activates it.
func (s *Store) Onboard(ctx context.Context, in OnboardInput) (Entry, Draft, error) {
	if in.ProjectID == "" || in.Owner == "" || in.Name == "" {
		return Entry{}, Draft{}, fmt.Errorf("%w: onboard needs project id, owner and name", ErrBadInput)
	}
	branch := in.DefaultBranch
	if branch == "" {
		branch = "main"
	}
	store := filepath.Join(s.root, "stores", pathComponent(in.ProjectID))
	if _, err := os.Stat(store); err == nil {
		// A store directory without a registry entry is a CONFLICT on the id,
		// not a malformed request: it is what a half-finished or overlapping
		// onboarding of the same id leaves behind, and the caller resolves it
		// by choosing another id. The refusal names the id it was given and
		// never the host path — a filesystem path has no caller use, and a
		// refusal is a wire surface like any other (Spec S11 host hygiene).
		return Entry{}, Draft{}, fmt.Errorf("%w: a store for project id %q already exists", ErrAlreadyRegistered, in.ProjectID)
	}
	if err := os.MkdirAll(filepath.Dir(store), 0o700); err != nil {
		return Entry{}, Draft{}, fmt.Errorf("project: stores dir: %w", err)
	}

	if in.Source != "" {
		// Clone from the LOCAL fixture repo (file transport only). --no-local
		// is deliberately NOT used: a local clone is what dev-mode onboarding
		// does; --no-hardlinks keeps the copy independent of the source's
		// object store.
		if _, err := s.git(ctx, "", identity{}, "clone", "-q", "--no-hardlinks", in.Source, store); err != nil {
			return Entry{}, Draft{}, fmt.Errorf("project: clone %q: %w", in.Source, err)
		}
		if b, err := s.detectDefaultBranch(ctx, store); err == nil && b != "" {
			branch = b
		}
	} else {
		if err := s.initRepo(ctx, store, branch); err != nil {
			return Entry{}, Draft{}, err
		}
		// A fresh repo needs one commit so the default branch has a HEAD for
		// run-branch worktrees to base on (this is repo initialization, not a
		// snapshot — the never-allow-empty rule is a SNAPSHOT rule, S13.5).
		if _, err := s.git(ctx, store, platformIdentity, "commit", "--allow-empty", "--no-verify", "--no-gpg-sign",
			"-m", "initialize project store (Spec S13.7)"); err != nil {
			return Entry{}, Draft{}, err
		}
	}

	e, err := s.Register(ctx, RegisterInput{
		ProjectID: in.ProjectID, Owner: in.Owner, Name: in.Name,
		StorePath: store, RemoteURL: in.RemoteURL, DefaultBranch: branch,
	})
	if err != nil {
		return Entry{}, Draft{}, err
	}
	draft, err := s.Scan(store, branch)
	if err != nil {
		return Entry{}, Draft{}, err
	}
	if _, err := s.Capture(ctx, CaptureInput{
		ProjectID: in.ProjectID, By: in.Owner,
		Conventions: draft.Conventions, Commands: draft.Commands,
		DangerZones: draft.DangerZones, ScanHash: draft.ScanHash,
	}); err != nil {
		return Entry{}, Draft{}, err
	}
	e, err = s.Get(ctx, in.ProjectID)
	if err != nil {
		return Entry{}, Draft{}, err
	}
	return e, draft, nil
}

// Approve activates a pending entry on owner approval, optionally re-capturing
// the owner's edited draft first (Spec S13.7: the owner edits/extends at the
// approval card). D10: only the owner may approve; a non-owner is refused
// (ErrNotOwner).
func (s *Store) Approve(ctx context.Context, projectID, approver string, edited *Draft) (Entry, error) {
	e, err := s.Get(ctx, projectID)
	if err != nil {
		return Entry{}, err
	}
	if approver != e.Owner {
		return Entry{}, fmt.Errorf("%w: entry %q is owned by %q", ErrNotOwner, projectID, e.Owner)
	}
	if edited != nil {
		if _, err := s.Capture(ctx, CaptureInput{
			ProjectID: projectID, By: approver,
			Conventions: edited.Conventions, Commands: edited.Commands,
			DangerZones: edited.DangerZones, ScanHash: edited.ScanHash,
		}); err != nil {
			return Entry{}, err
		}
	}
	return s.Activate(ctx, projectID, approver)
}

// OnboardStart runs the platform onboarding task's deterministic steps
// (register → clone/init → scan → draft) and returns the drafted content as
// JSON to surface on the durable owner-approval ask (Spec S13.7, R5/F1). It is
// idempotent under re-dispatch: if the entry already exists pending, it returns
// its current drafted capture rather than re-cloning. The run-substrate
// realization (the .onboard run role + the ask/park) is the stage layer's; the
// git side stays here (stage never imports internal/project, brief R35).
func (s *Store) OnboardStart(ctx context.Context, in OnboardInput) ([]byte, error) {
	if e, err := s.Get(ctx, in.ProjectID); err == nil {
		// Already onboarded (re-dispatch after a crash): return the current
		// drafted capture; a rescan is the owner's explicit re-scan verb.
		if e.CaptureVersion >= 1 {
			return json.Marshal(draftOf(e.Capture))
		}
		return json.Marshal(Draft{})
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	_, draft, err := s.Onboard(ctx, in)
	if err != nil {
		return nil, err
	}
	return json.Marshal(draft)
}

// OnboardApprove activates a pending entry on owner approval, applying an
// optional edited draft (Spec S13.7 → D10). The owner check is the stage ask's
// (the ask row's owner answers); this asserts it too (defence in depth).
func (s *Store) OnboardApprove(ctx context.Context, projectID, approver string, editedDraftJSON []byte) error {
	var edited *Draft
	if len(editedDraftJSON) > 0 && string(editedDraftJSON) != "null" {
		var d Draft
		if err := json.Unmarshal(editedDraftJSON, &d); err != nil {
			return fmt.Errorf("%w: onboard edited draft: %v", ErrBadInput, err)
		}
		// Recompute the drift baseline over the owner's edited content.
		d.ScanHash = scanHash(d)
		edited = &d
	}
	_, err := s.Approve(ctx, projectID, approver, edited)
	return err
}

// draftOf projects a capture back into a Draft (the onboarding card payload).
func draftOf(c Capture) Draft {
	return Draft{Conventions: c.Conventions, Commands: c.Commands, DangerZones: c.DangerZones, ScanHash: c.ScanHash}
}

// Rescan re-scans an entry's store and records a NEW capture version (Spec
// S13.7: re-scan on demand). Never overwrites the prior capture — re-capture
// is a new immutable version.
func (s *Store) Rescan(ctx context.Context, projectID, by string) (Capture, error) {
	e, err := s.Get(ctx, projectID)
	if err != nil {
		return Capture{}, err
	}
	draft, err := s.Scan(e.StorePath, e.DefaultBranch)
	if err != nil {
		return Capture{}, err
	}
	return s.Capture(ctx, CaptureInput{
		ProjectID: projectID, By: by,
		Conventions: draft.Conventions, Commands: draft.Commands,
		DangerZones: draft.DangerZones, ScanHash: draft.ScanHash,
	})
}

// DriftReport reports whether an entry's captured content has drifted from a
// live re-scan (Spec S13.7 re-scan-when-drift-detected; S05.6 row 2: captured
// source hashes vs live content). The captured content is NEVER silently
// overwritten — drift is surfaced (a registry.drift event + the flag); the
// operator re-scans (Rescan) and re-approves when they choose.
type DriftReport struct {
	Drifted bool     `json:"drifted"`
	Reasons []string `json:"reasons,omitempty"`
}

// DriftCheck compares the current capture's scan fingerprint against a live
// re-scan and, on mismatch, records a registry.drift event and returns the
// flag with human reasons.
func (s *Store) DriftCheck(ctx context.Context, projectID string) (DriftReport, error) {
	e, err := s.Get(ctx, projectID)
	if err != nil {
		return DriftReport{}, err
	}
	if e.CaptureVersion < 1 {
		return DriftReport{}, fmt.Errorf("%w: entry %q has no capture to compare", ErrBadInput, projectID)
	}
	live, err := s.Scan(e.StorePath, e.DefaultBranch)
	if err != nil {
		return DriftReport{}, err
	}
	var reasons []string
	if live.ScanHash != e.Capture.ScanHash {
		reasons = append(reasons, "scanned source changed since capture")
	}
	reasons = append(reasons, dangerZoneDrift(e.Capture.DangerZones, live.DangerZones)...)
	rep := DriftReport{Drifted: len(reasons) > 0, Reasons: reasons}
	if !rep.Drifted {
		return rep, nil
	}
	payload, err := json.Marshal(struct {
		ProjectID string   `json:"project_id"`
		Version   int      `json:"capture_version"`
		Reasons   []string `json:"reasons"`
	}{projectID, e.CaptureVersion, reasons})
	if err != nil {
		return DriftReport{}, err
	}
	err = s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		return s.appendTx(ctx, tx, e.Owner, EventDrift, payload)
	})
	if err != nil {
		return DriftReport{}, err
	}
	return rep, nil
}

// dangerZoneDrift reports danger zones whose guarded source hash changed or
// vanished — the concrete "captured source-hash vs live content" comparison.
func dangerZoneDrift(captured, live []DangerZone) []string {
	liveByPath := map[string]string{}
	for _, z := range live {
		liveByPath[z.Path] = z.SourceHash
	}
	var reasons []string
	for _, z := range captured {
		if z.SourceHash == "" {
			continue
		}
		cur, ok := liveByPath[z.Path]
		if !ok {
			reasons = append(reasons, fmt.Sprintf("danger zone %q source no longer present", z.Path))
			continue
		}
		if cur != z.SourceHash {
			reasons = append(reasons, fmt.Sprintf("danger zone %q source content changed", z.Path))
		}
	}
	return reasons
}

// detectDefaultBranch reads the store's current branch (the clone's HEAD).
func (s *Store) detectDefaultBranch(ctx context.Context, store string) (string, error) {
	return s.git(ctx, store, identity{}, "rev-parse", "--abbrev-ref", "HEAD")
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		default:
			return '_'
		}
	}, s)
}

// pathComponent maps an id to a COLLISION-FREE filesystem path component: an
// already path-safe id maps to itself; any id sanitize would alter carries a
// short content-hash suffix, so two distinct ids (e.g. "a/b" and "a_b") can
// never share a worktree/store path (F16e — a collision would be silent data
// corruption across pipelines).
func pathComponent(id string) string {
	clean := sanitize(id)
	if clean == id && id != "" {
		return clean
	}
	return clean + "-" + hashContent([]byte(id))[:12]
}
