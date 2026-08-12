package project

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
)

// The project/repository registry (Spec S13.7). Control-plane verbs over
// repo_registry (+ its immutable capture history), each appending a
// platform-scope, owner-attributed audit event (refs/hashes, never blobs).

// RegisterInput registers a new project (Spec S13.7 onboarding step 1).
type RegisterInput struct {
	ProjectID     string
	Owner         string
	Name          string
	StorePath     string
	RemoteURL     string
	DefaultBranch string
	Members       []string
}

// Register creates a PENDING registry entry (Spec S13.7: register → … → owner
// approves → active). Protected refs default to the project's default branch
// ("main only via accepts", Spec S13.6). The entry feeds nothing until active.
func (s *Store) Register(ctx context.Context, in RegisterInput) (Entry, error) {
	if in.ProjectID == "" || in.Owner == "" || in.Name == "" || in.StorePath == "" {
		return Entry{}, fmt.Errorf("%w: register needs project id, owner, name and store path", ErrBadInput)
	}
	branch := in.DefaultBranch
	if branch == "" {
		branch = "main"
	}
	if _, err := s.Get(ctx, in.ProjectID); err == nil {
		return Entry{}, fmt.Errorf("%w: %q", ErrAlreadyRegistered, in.ProjectID)
	} else if !errors.Is(err, ErrNotFound) {
		return Entry{}, err
	}
	members := marshalStrings(in.Members)
	protected := marshalStrings([]string{branch})
	now := s.nowRFC3339()
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO repo_registry (project_id, user_id, name, store_path, remote_url,
			                           default_branch, members, protected_refs, state,
			                           capture_version, created_ts, updated_ts)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending', 0, ?, ?)`,
			in.ProjectID, in.Owner, in.Name, in.StorePath, in.RemoteURL,
			branch, members, protected, now, now); err != nil {
			return fmt.Errorf("project: insert registry entry: %w", err)
		}
		payload, err := json.Marshal(struct {
			ProjectID     string `json:"project_id"`
			Name          string `json:"name"`
			DefaultBranch string `json:"default_branch"`
			HasRemote     bool   `json:"has_remote"`
		}{in.ProjectID, in.Name, branch, in.RemoteURL != ""})
		if err != nil {
			return err
		}
		return s.appendTx(ctx, tx, in.Owner, EventRegistered, payload)
	})
	if err != nil {
		return Entry{}, err
	}
	return s.Get(ctx, in.ProjectID)
}

// Get reads one entry with its current captured content (Spec S13.7).
func (s *Store) Get(ctx context.Context, projectID string) (Entry, error) {
	var (
		e         Entry
		members   string
		protected string
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT project_id, user_id, name, store_path, remote_url, default_branch,
		       members, protected_refs, state, capture_version, created_ts, updated_ts
		FROM repo_registry WHERE project_id = ?`, projectID).
		Scan(&e.ProjectID, &e.Owner, &e.Name, &e.StorePath, &e.RemoteURL, &e.DefaultBranch,
			&members, &protected, &e.State, &e.CaptureVersion, &e.CreatedTS, &e.UpdatedTS)
	if errors.Is(err, sql.ErrNoRows) {
		return Entry{}, fmt.Errorf("%w: %q", ErrNotFound, projectID)
	}
	if err != nil {
		return Entry{}, fmt.Errorf("project: read entry %q: %w", projectID, err)
	}
	e.Members = unmarshalStrings(members)
	e.ProtectedRefs = unmarshalStrings(protected)
	if e.CaptureVersion > 0 {
		cap, err := s.captureAt(ctx, projectID, e.CaptureVersion)
		if err != nil {
			return Entry{}, err
		}
		e.Capture = cap
	}
	return e, nil
}

// captureAt reads one immutable capture version.
func (s *Store) captureAt(ctx context.Context, projectID string, version int) (Capture, error) {
	var (
		c           Capture
		conventions string
		commands    string
		zones       string
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT version, conventions, commands, danger_zones, scan_hash, family, captured_by, captured_ts
		FROM repo_registry_captures WHERE project_id = ? AND version = ?`, projectID, version).
		Scan(&c.Version, &conventions, &commands, &zones, &c.ScanHash, &c.Family, &c.CapturedBy, &c.CapturedTS)
	if errors.Is(err, sql.ErrNoRows) {
		return Capture{}, fmt.Errorf("%w: %q capture v%d", ErrNotFound, projectID, version)
	}
	if err != nil {
		return Capture{}, fmt.Errorf("project: read capture: %w", err)
	}
	c.Conventions = unmarshalStrings(conventions)
	if err := json.Unmarshal([]byte(commands), &c.Commands); err != nil {
		return Capture{}, fmt.Errorf("project: decode commands: %w", err)
	}
	if err := json.Unmarshal([]byte(zones), &c.DangerZones); err != nil {
		return Capture{}, fmt.Errorf("project: decode danger zones: %w", err)
	}
	return c, nil
}

// List returns every registry entry, active first, then by project id.
func (s *Store) List(ctx context.Context) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT project_id FROM repo_registry ORDER BY state, project_id`)
	if err != nil {
		return nil, fmt.Errorf("project: list entries: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(ids))
	for _, id := range ids {
		e, err := s.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// The owner-declared task family of a registered project (P3-RW-11 R2; Spec
// S13.7). The six values are internal/intake's `Family` constants duplicated BY
// VALUE, because the §23 import wall bars this package from importing
// internal/intake — the two lists are pinned byte-equal at the composition
// root, which is the one place both are legitimately visible.
const (
	FamilySoftware = "software"
	FamilyResearch = "research"
	FamilyContent  = "content"
	FamilyData     = "data"
	FamilyChore    = "chore"
	FamilyGeneric  = "generic"
)

// Families is the vocabulary in its ratified order — the ONE list this package
// validates against and the root pins.
func Families() []string {
	return []string{FamilySoftware, FamilyResearch, FamilyContent, FamilyData, FamilyChore, FamilyGeneric}
}

// validFamily accepts the six values plus "" — honest absence: a project whose
// owner declared no family simply has none, and the interview asks (R4).
func validFamily(f string) bool {
	if f == "" {
		return true
	}
	for _, known := range Families() {
		if f == known {
			return true
		}
	}
	return false
}

// badFamily is the loud refusal a value outside the vocabulary earns. It names
// what is accepted, so a caller can fix the request without reading source.
func badFamily(f string) error {
	return fmt.Errorf("%w: task family %q is not one of %s (or empty for none)",
		ErrBadInput, f, strings.Join(Families(), ", "))
}

// CaptureInput records a captured-content version (Spec S13.7). Every call is
// a NEW immutable version; the entry's pointer advances. The captured content
// is never overwritten in place.
type CaptureInput struct {
	ProjectID   string
	By          string
	Conventions []string
	Commands    Commands
	DangerZones []DangerZone
	ScanHash    string
	// Family is the owner-declared task family (P3-RW-11 R2). "" = none.
	Family string
}

// Capture writes a new captured-content version and advances the entry's
// pointer (Spec S13.7: captured content is versioned, re-capture bumps
// traceably). Returns the new capture.
func (s *Store) Capture(ctx context.Context, in CaptureInput) (Capture, error) {
	if in.ProjectID == "" || in.By == "" {
		return Capture{}, fmt.Errorf("%w: capture needs a project and an actor", ErrBadInput)
	}
	if !validFamily(in.Family) {
		return Capture{}, badFamily(in.Family)
	}
	e, err := s.Get(ctx, in.ProjectID)
	if err != nil {
		return Capture{}, err
	}
	version := e.CaptureVersion + 1
	conventions := marshalStrings(in.Conventions)
	commands, err := json.Marshal(in.Commands)
	if err != nil {
		return Capture{}, fmt.Errorf("project: marshal commands: %w", err)
	}
	zones, err := json.Marshal(dangerZonesOrEmpty(in.DangerZones))
	if err != nil {
		return Capture{}, fmt.Errorf("project: marshal danger zones: %w", err)
	}
	now := s.nowRFC3339()
	err = s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO repo_registry_captures (project_id, version, conventions, commands,
			                                    danger_zones, scan_hash, family, captured_by, captured_ts)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			in.ProjectID, version, conventions, string(commands), string(zones),
			in.ScanHash, in.Family, in.By, now); err != nil {
			return fmt.Errorf("project: insert capture: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE repo_registry SET capture_version = ?, updated_ts = ? WHERE project_id = ?`,
			version, now, in.ProjectID); err != nil {
			return fmt.Errorf("project: advance capture pointer: %w", err)
		}
		payload, err := json.Marshal(struct {
			ProjectID   string `json:"project_id"`
			Version     int    `json:"version"`
			ScanHash    string `json:"scan_hash,omitempty"`
			Family      string `json:"family,omitempty"`
			Conventions int    `json:"conventions"`
			DangerZones int    `json:"danger_zones"`
		}{in.ProjectID, version, in.ScanHash, in.Family, len(in.Conventions), len(in.DangerZones)})
		if err != nil {
			return err
		}
		return s.appendTx(ctx, tx, in.By, EventCaptured, payload)
	})
	if err != nil {
		return Capture{}, err
	}
	return s.captureAt(ctx, in.ProjectID, version)
}

// Activate makes a pending entry active on owner approval (Spec S13.7 → D10:
// the owner approves their own object). A draft (at least one capture) must
// exist first: an entry with no captured content has nothing to approve.
func (s *Store) Activate(ctx context.Context, projectID, approver string) (Entry, error) {
	e, err := s.Get(ctx, projectID)
	if err != nil {
		return Entry{}, err
	}
	if approver != e.Owner {
		return Entry{}, fmt.Errorf("%w: entry %q is owned by %q", ErrNotOwner, projectID, e.Owner)
	}
	if e.CaptureVersion < 1 {
		return Entry{}, fmt.Errorf("%w: entry %q has no drafted content to approve", ErrBadInput, projectID)
	}
	if e.State == StateActive {
		return e, nil
	}
	now := s.nowRFC3339()
	err = s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`UPDATE repo_registry SET state = 'active', updated_ts = ? WHERE project_id = ? AND state = 'pending'`,
			now, projectID); err != nil {
			return fmt.Errorf("project: activate entry: %w", err)
		}
		payload, err := json.Marshal(struct {
			ProjectID string `json:"project_id"`
			Version   int    `json:"capture_version"`
		}{projectID, e.CaptureVersion})
		if err != nil {
			return err
		}
		return s.appendTx(ctx, tx, approver, EventActivated, payload)
	})
	if err != nil {
		return Entry{}, err
	}
	return s.Get(ctx, projectID)
}

// ProtectedRefs returns the broker protected-ref policy data for an ACTIVE
// entry (Spec S13.6/S13.7; the broker CONSUMER is B4-3's — this is the read
// surface). Default at onboarding = [default branch].
func (s *Store) ProtectedRefs(ctx context.Context, projectID string) ([]string, error) {
	e, err := s.Get(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if !e.Active() {
		return nil, fmt.Errorf("%w: %q", ErrNotActive, projectID)
	}
	return e.ProtectedRefs, nil
}

// MatchHint is the intake-side match input (Spec S06.2 step 2). The full
// request text is scanned for a registered project's name/alias so the
// interview never asks what the platform already knows ("in the shop backend"
// needs no path-explaining).
type MatchHint struct {
	UserID string
	Title  string
	Text   string
}

// MatchForIntake returns the ACTIVE registry entry a request most specifically
// names, matched deterministically by the project name/alias appearing as a
// token in the request (Spec S13.7 "the registry feeds intake resolution").
// Only entries the user owns or is a member of are visible. ok=false when
// nothing matches.
func (s *Store) MatchForIntake(ctx context.Context, h MatchHint) (Entry, bool, error) {
	entries, err := s.List(ctx)
	if err != nil {
		return Entry{}, false, err
	}
	hay := " " + strings.ToLower(h.Title+" "+h.Text) + " "
	var best Entry
	bestLen := 0
	for _, e := range entries {
		if !e.Active() || !visibleTo(e, h.UserID) {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(e.Name))
		if name == "" {
			continue
		}
		if tokenPresent(hay, name) && len(name) > bestLen {
			best, bestLen = e, len(name)
		}
	}
	if bestLen == 0 {
		return Entry{}, false, nil
	}
	return best, true, nil
}

// PinForIntake resolves a request's SUBMITTED project pin: the registry id a
// requester chose explicitly (the Projects-tab door, P3-RW-1) rather than named
// in the request text. It is MatchForIntake's sibling and shares its edge — the
// same visibility predicate over the same lifecycle rule — but resolves by id
// and refuses distinguishably, so a pin the requester cannot have is a refusal
// rather than the scan's ordinary "nothing matched" (Spec S13.7 "the registry
// feeds intake resolution"; S15.2 server-side authority).
//
// An entry that does not exist and one the requester cannot see return the SAME
// ErrNotFound with the same text: telling them apart would leak the existence of
// another person's project. A VISIBLE entry that is not yet active returns
// ErrNotActive — the requester may know that honestly.
func (s *Store) PinForIntake(ctx context.Context, projectID, userID string) (Entry, error) {
	e, err := s.Get(ctx, projectID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Entry{}, noSuchPin(projectID)
		}
		return Entry{}, err
	}
	if !visibleTo(e, userID) {
		return Entry{}, noSuchPin(projectID)
	}
	if !e.Active() {
		return Entry{}, fmt.Errorf("%w: %q", ErrNotActive, projectID)
	}
	return e, nil
}

// noSuchPin is the ONE refusal an unknown id and an invisible entry share.
func noSuchPin(projectID string) error {
	return fmt.Errorf("%w: %q", ErrNotFound, projectID)
}

// visibleTo reports whether user owns or is an invited member of the entry.
func visibleTo(e Entry, user string) bool {
	if e.Owner == user {
		return true
	}
	for _, m := range e.Members {
		if m == user {
			return true
		}
	}
	return false
}

// tokenPresent reports whether name appears in hay (already space-padded and
// lowercased) at word boundaries — a deterministic, closed-world match.
func tokenPresent(hay, name string) bool {
	for i := 0; ; {
		j := strings.Index(hay[i:], name)
		if j < 0 {
			return false
		}
		start := i + j
		end := start + len(name)
		if !isWordChar(hay[start-1]) && !isWordChar(hay[end]) {
			return true
		}
		i = start + 1
	}
}

func isWordChar(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
}

// appendTx appends a platform-scope, owner-attributed registry audit event
// (run_id NULL per Spec S02.2; 15.6). Names are provisional pending S14
// (CONVENTIONS §7/§8 note).
func (s *Store) appendTx(ctx context.Context, tx *sql.Tx, userID, typ string, payload json.RawMessage) error {
	_, err := s.log.AppendTx(ctx, tx, eventlog.Append{
		UserID: userID, Type: typ, SchemaVersion: eventSchemaVersion, Payload: payload, Time: s.clock(),
	})
	return err
}

func marshalStrings(v []string) string {
	if len(v) == 0 {
		return "[]"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func unmarshalStrings(s string) []string {
	if s == "" || s == "[]" {
		return nil
	}
	var v []string
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil
	}
	return v
}

func dangerZonesOrEmpty(z []DangerZone) []DangerZone {
	if z == nil {
		return []DangerZone{}
	}
	sorted := append([]DangerZone(nil), z...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	return sorted
}

// hashContent is the source-hash primitive for danger zones and scan
// fingerprints (Spec S13.7 drift check; S05.6 row 2).
func hashContent(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
