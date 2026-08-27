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
	"unicode/utf8"

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
	// Origin names what produced this version, for the audit trail alone
	// (P3-GF5 R6): "" for a scan-derived capture — every caller that predates
	// this field, so their event bytes are unchanged — and OriginEdit for an
	// owner's direct command edit. It distinguishes a person's act from the
	// scan's draft on a row that otherwise looks identical; nothing branches
	// on it.
	Origin string
}

// OriginEdit marks a capture an owner typed rather than one a scan derived
// (Spec S13.7 rows are owner-attributed; P3-GF5 R6).
const OriginEdit = "edit"

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
	if _, err := s.Get(ctx, in.ProjectID); err != nil {
		return Capture{}, err
	}
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
	// The version is allocated INSIDE the write transaction (P3-GF5 R7). Read
	// before it, two writers racing the same entry both computed the same
	// version+1 and the loser died on the (project_id, version) primary key —
	// a raw constraint abort with no meaning for the person who pressed the
	// button. BEGIN IMMEDIATE serializes the readers with the writers, so the
	// number the insert uses is the number the pointer actually holds.
	var version int
	err = s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		var current int
		if err := tx.QueryRowContext(ctx,
			`SELECT capture_version FROM repo_registry WHERE project_id = ?`, in.ProjectID).Scan(&current); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: %q", ErrNotFound, in.ProjectID)
			}
			return fmt.Errorf("project: read capture pointer: %w", err)
		}
		version = current + 1
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
			// Origin is additive and omitempty (P3-GF5 R6): every caller that
			// predates it leaves it empty, so their event bytes are unchanged,
			// and an owner's direct edit is distinguishable from a scan draft
			// on a row that otherwise reads identically.
			Origin string `json:"origin,omitempty"`
		}{in.ProjectID, version, in.ScanHash, in.Family, len(in.Conventions), len(in.DangerZones), in.Origin})
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

// commandMaxRunes bounds one captured command.
//
// A structural constant with its reason, not a ⚙ key (the cancelReasonMaxRunes
// / sseBatchSize pattern; S13.7 and S18 ratify nothing here): a captured
// command is ONE shell line, and anything longer than this is a script, which
// belongs in the repository the command calls. Interim under the standing
// settings-tab directive.
//
// The bound counts RUNES, because it describes what a person typed and a byte
// count would refuse a shorter command written in a language with wider
// characters.
const commandMaxRunes = 500

// commandSlots names each command slot with its accessor, so validation and
// carry-forward run over ALL FIVE rather than over whichever ones somebody
// remembered (Spec S13.7: build/test/lint/run/preview).
var commandSlots = []struct {
	name string
	get  func(Commands) string
	set  func(*Commands, string)
}{
	{"build", func(c Commands) string { return c.Build }, func(c *Commands, v string) { c.Build = v }},
	{"test", func(c Commands) string { return c.Test }, func(c *Commands, v string) { c.Test = v }},
	{"lint", func(c Commands) string { return c.Lint }, func(c *Commands, v string) { c.Lint = v }},
	{"run", func(c Commands) string { return c.Run }, func(c *Commands, v string) { c.Run = v }},
	{"preview", func(c Commands) string { return c.Preview }, func(c *Commands, v string) { c.Preview = v }},
}

// validCommands trims each slot and refuses anything the verification sandbox
// could not run as one shell line (P3-GF5 R4).
//
// WHAT IS CHECKED AND WHAT DELIBERATELY IS NOT. A captured command is executed
// only later, inside the network-off C2 verification sandbox, as
// `/bin/sh -lc <cmd>`, and the platform reads the exit status (Spec S07.3 rule
// 1/3; S11). So the shape has to survive that: one line (a multi-line value
// smuggles a script body past the one-rung-one-line contract), no NUL (it
// cannot cross an argv), valid UTF-8 (text the platform can vouch for), and a
// bound.
//
// There is NO token allowlist and no shell-safety filter, deliberately. The
// owner already owns arbitrary code execution inside their own repository —
// its Makefile is theirs — so a filter here would refuse honest commands while
// stopping nothing, which is theater posing as a boundary. NOTHING is executed,
// resolved or dialed at capture time on any path through this function.
//
// An all-empty set is ACCEPTED: it returns the project to Spec S07.8's
// bootstrap posture, which is the honest recompute, not an error.
func validCommands(c Commands) (Commands, error) {
	var out Commands
	for _, slot := range commandSlots {
		v := strings.TrimSpace(slot.get(c))
		switch {
		case strings.ContainsAny(v, "\n\r"):
			return Commands{}, fmt.Errorf("%w: the %s command must be a single line — a check rung runs as one shell line, and a multi-line value would be a script the platform cannot report a rung for", ErrBadInput, slot.name)
		case strings.ContainsRune(v, 0):
			return Commands{}, fmt.Errorf("%w: the %s command contains a NUL byte, which cannot be passed to a program", ErrBadInput, slot.name)
		case !utf8.ValidString(v):
			return Commands{}, fmt.Errorf("%w: the %s command is not valid UTF-8", ErrBadInput, slot.name)
		case utf8.RuneCountInString(v) > commandMaxRunes:
			return Commands{}, fmt.Errorf("%w: the %s command is %d characters; the limit is %d — a command this long is a script, which belongs in the project it calls",
				ErrBadInput, slot.name, utf8.RuneCountInString(v), commandMaxRunes)
		}
		slot.set(&out, v)
	}
	return out, nil
}

// EditCommands replaces an ACTIVE entry's captured command set and returns the
// capture that now stands, plus whether a new version was minted (Spec S13.7:
// the registry holds a project's build/test/lint/run/preview commands, rows are
// owner-attributed, captured content is versioned; migration 0008's own comment
// names this writer — "the version bumps on every re-capture/re-scan/EDIT").
//
// FULL REPLACEMENT, CARRY-FORWARD FOR EVERYTHING ELSE. The submitted set
// becomes the capture's whole `commands` member — an omitted slot is CLEARED,
// because a capture nobody can empty is a capture whose wrong command can only
// be replaced, never withdrawn. Conventions, danger zones, the scan hash and
// the family are carried forward byte-equal, which is the Rescan discipline for
// the same reason: dropping the scan hash would leave DriftCheck comparing
// against nothing, and dropping owner-approved conventions or zones would unset
// them with no event and no card behind it.
//
// RETRY-SAFE (Spec S15.2: "a repeated answer returns the already-resolved
// state — a phone retry can never double-fire"). A submission whose validated
// set equals what is already captured mints no version and appends no event; it
// returns the capture that stands with minted=false, and the caller says so.
//
// OWNER-ONLY AND ACTIVE-ONLY. D10 is authority over one's own object, and the
// captured commands decide what the verification sandbox executes for every
// member's runs. A PENDING entry is refused with ErrNotActive rather than
// edited: its draft's door is the onboarding approval card, where the owner
// edits the whole draft and approving activates the entry — a second write path
// onto a pending draft would be one act with two audit stories.
func (s *Store) EditCommands(ctx context.Context, projectID, by string, cmds Commands) (Capture, bool, error) {
	if projectID == "" || by == "" {
		return Capture{}, false, fmt.Errorf("%w: editing commands needs a project and an actor", ErrBadInput)
	}
	wanted, err := validCommands(cmds)
	if err != nil {
		return Capture{}, false, err
	}
	e, err := s.Get(ctx, projectID)
	if err != nil {
		return Capture{}, false, err
	}
	if by != e.Owner {
		return Capture{}, false, fmt.Errorf("%w: entry %q is owned by %q", ErrNotOwner, projectID, e.Owner)
	}
	if !e.Active() {
		return Capture{}, false, fmt.Errorf("%w: %q", ErrNotActive, projectID)
	}
	if e.CaptureVersion > 0 && e.Capture.Commands == wanted {
		return e.Capture, false, nil
	}
	c, err := s.Capture(ctx, CaptureInput{
		ProjectID:   projectID,
		By:          by,
		Conventions: e.Capture.Conventions,
		Commands:    wanted,
		DangerZones: e.Capture.DangerZones,
		ScanHash:    e.Capture.ScanHash,
		Family:      e.Capture.Family,
		Origin:      OriginEdit,
	})
	if err != nil {
		return Capture{}, false, err
	}
	return c, true, nil
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

// dangerZonesOrEmpty normalizes the stored zone list: sorted by path, and the
// EMPTY list stored as `[]` rather than `null`.
//
// The length test rather than a nil test is the point (found by P3-GF5's
// carry-forward property). `append([]DangerZone(nil))` over an empty-but-
// non-nil slice returns NIL, which marshals to `null` — so a capture that
// carried an empty zone list forward stored `null` where the capture it copied
// stored `[]`, and the next carry-forward flipped it back. The decoded value was
// identical either way, so nothing noticed; the stored BYTES were not, which is
// exactly what "carried forward byte-equal" is supposed to mean, and what a
// committed golden fixture reads.
func dangerZonesOrEmpty(z []DangerZone) []DangerZone {
	if len(z) == 0 {
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
