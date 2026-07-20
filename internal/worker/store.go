package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// store.go — the worker store over the 0005 tables plus the file root
// (Spec S08.1: rows + git-versioned template files; the row is the index,
// the file is the content). The control plane is the sole writer; every
// creating/approving/disposing verb is human-gated (D10) via the users
// table, the same capability posture as the S09 gate. Git commit-on-write
// is the S13 committer seam (B4) — file_commit stays NULL until it lands,
// the 0005 trigger permitting exactly the one NULL→hash fill.

// Settings is the store's view of the settings registry (the 4 ⚙
// workers.* keys, read by dotted key — S01.10).
type Settings interface {
	Int(key string) (int64, error)
	Float(key string) (float64, error)
}

// ⚙ keys (Spec S08 settings table; declared in internal/settings).
const (
	keyFirstN           = "workers.first_n"
	keyGapProposalCount = "workers.gap_proposal_count"
	keyDryrunCostCap    = "workers.dryrun_cost_cap_usd"
)

// Event types (platform-scope, owner-attributed; names provisional pending
// the S14 event contract at B5 — the standing CONVENTIONS §7/§8 note).
const (
	evTemplateCreated = "worker.template_created"
	evVersionCreated  = "worker.version_created"
	evValidated       = "worker.validated"
	evApproved        = "worker.approved"
	evPromoted        = "worker.promoted"
	evRepointed       = "worker.repointed"
	evFlagged         = "worker.flagged"
	evRetired         = "worker.retired"
	evReview          = "worker.review"
	evGraduated       = "worker.graduated"
	evGap             = "worker.gap"
	evDomain          = "worker.domain_maturity"
	evAutomationRun   = "worker.automation_run"
)

const workerEventSchemaVersion = 1

// Config configures a Store.
type Config struct {
	DB       *storage.DB
	Log      *eventlog.Log
	Settings Settings
	// Root is the worker file root: templates/<id>/v<N>.md and
	// skills/<name>/ live under it.
	Root string
	// Overlays is the S09-machinery overlay seam (compile.go); nil is the
	// dormant v0 default (empty slice).
	Overlays OverlaySource
	// Now defaults to time.Now; tests inject a clock.
	Now func() time.Time
}

// Store is the worker store.
type Store struct {
	db       *storage.DB
	log      *eventlog.Log
	settings Settings
	root     string
	overlays OverlaySource
	now      func() time.Time
}

// NewStore opens the store and materializes the file root.
func NewStore(cfg Config) (*Store, error) {
	if cfg.DB == nil || cfg.Log == nil {
		return nil, fmt.Errorf("worker: store needs DB and event log")
	}
	if cfg.Settings == nil {
		return nil, fmt.Errorf("worker: store needs the settings registry (⚙ discipline, S01.10)")
	}
	if cfg.Root == "" {
		return nil, fmt.Errorf("worker: store needs a file root")
	}
	for _, d := range []string{cfg.Root, filepath.Join(cfg.Root, "templates"), filepath.Join(cfg.Root, "skills")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return nil, fmt.Errorf("worker: create %s: %w", d, err)
		}
	}
	s := &Store{db: cfg.DB, log: cfg.Log, settings: cfg.Settings, root: cfg.Root,
		overlays: cfg.Overlays, now: cfg.Now}
	if s.now == nil {
		s.now = time.Now
	}
	return s, nil
}

// Root returns the file root.
func (s *Store) Root() string { return s.root }

// person resolves the acting human's row; reserved ids and the dev
// identity have no person row and are structurally refused (the S09.1
// capability posture applied to the worker gate verbs; Spec S01.9).
func (s *Store) person(ctx context.Context, actor string) (role string, err error) {
	if actor == "" {
		return "", fmt.Errorf("%w: empty actor", ErrNotHuman)
	}
	err = s.db.QueryRowContext(ctx, `SELECT role FROM users WHERE user_id = ?`, actor).Scan(&role)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("%w: %q", ErrNotHuman, actor)
	}
	if err != nil {
		return "", fmt.Errorf("worker: resolve actor: %w", err)
	}
	return role, nil
}

func (s *Store) event(userID, typ string, payload map[string]any) eventlog.Append {
	raw, err := json.Marshal(payload)
	if err != nil {
		raw = []byte(`{}`)
	}
	return eventlog.Append{UserID: userID, Type: typ, SchemaVersion: workerEventSchemaVersion, Payload: raw}
}

// ── Row reads ──

const templateColumns = `template_id, user_id, name, scope, kind, domain, status,
	coalesce(active_version, ''), created_ts, updated_ts`

func scanTemplate(r interface{ Scan(...any) error }) (Template, error) {
	var t Template
	err := r.Scan(&t.ID, &t.Owner, &t.Name, (*string)(&t.Scope), (*string)(&t.Kind),
		&t.Domain, (*string)(&t.Status), &t.ActiveVersion, &t.CreatedTS, &t.UpdatedTS)
	return t, err
}

// Template loads one template row.
func (s *Store) Template(ctx context.Context, id string) (Template, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+templateColumns+` FROM worker_templates WHERE template_id = ?`, id)
	t, err := scanTemplate(row)
	if err == sql.ErrNoRows {
		return t, fmt.Errorf("%w: template %q", ErrNotFound, id)
	}
	if err != nil {
		return t, fmt.Errorf("worker: load template: %w", err)
	}
	return t, nil
}

const versionColumns = `version_id, template_id, version, coalesce(supersedes_id, ''),
	file_path, file_sha256, coalesce(file_commit, ''), requested_grants, author_kind,
	composer_model, composer_playbook_version, evidence_ref, origin, origin_ref,
	created_by, created_ts, approved_by, coalesce(approved_ts, ''), coalesce(graduated_ts, '')`

func scanVersion(r interface{ Scan(...any) error }) (Version, error) {
	var v Version
	var grants string
	err := r.Scan(&v.ID, &v.TemplateID, &v.Version, &v.Supersedes, &v.FilePath,
		&v.FileSHA256, &v.FileCommit, &grants, &v.AuthorKind, &v.Composer,
		&v.PlaybookVer, &v.EvidenceRef, (*string)(&v.Origin), &v.OriginRef,
		&v.CreatedBy, &v.CreatedTS, &v.ApprovedBy, &v.ApprovedTS, &v.GraduatedTS)
	if err != nil {
		return v, err
	}
	if err := json.Unmarshal([]byte(grants), &v.Requested); err != nil {
		return v, fmt.Errorf("worker: decode requested grants: %w", err)
	}
	return v, nil
}

// VersionByID loads one version row.
func (s *Store) VersionByID(ctx context.Context, id string) (Version, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+versionColumns+` FROM worker_template_versions WHERE version_id = ?`, id)
	v, err := scanVersion(row)
	if err == sql.ErrNoRows {
		return v, fmt.Errorf("%w: version %q", ErrNotFound, id)
	}
	if err != nil {
		return v, fmt.Errorf("worker: load version: %w", err)
	}
	return v, nil
}

// latestVersion loads the highest version row of a template.
func (s *Store) latestVersion(ctx context.Context, templateID string) (Version, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+versionColumns+` FROM worker_template_versions
		 WHERE template_id = ? ORDER BY version DESC LIMIT 1`, templateID)
	v, err := scanVersion(row)
	if err == sql.ErrNoRows {
		return v, fmt.Errorf("%w: template %q has no versions", ErrNotFound, templateID)
	}
	if err != nil {
		return v, fmt.Errorf("worker: load latest version: %w", err)
	}
	return v, nil
}

// Guardrails loads the enforcement row of a version (Spec S08.2).
func (s *Store) Guardrails(ctx context.Context, versionID string) (Guardrails, error) {
	var g Guardrails
	var granted, permMap, hosts string
	var attach int64
	row := s.db.QueryRowContext(ctx,
		`SELECT version_id, granted_tools, permission_map, confinement_class, egress_class,
		        egress_hosts, budget_ceiling_usd, budget_ceiling_steps, gate_policy,
		        first_n_remaining, schedule_attachable, created_ts, updated_ts
		 FROM worker_guardrails WHERE version_id = ?`, versionID)
	err := row.Scan(&g.VersionID, &granted, &permMap, &g.Class, (*string)(&g.Egress),
		&hosts, &g.BudgetUSD, &g.BudgetSteps, &g.GatePolicy,
		&g.FirstNRemaining, &attach, &g.CreatedTS, &g.UpdatedTS)
	if err == sql.ErrNoRows {
		return g, fmt.Errorf("%w: guardrails for version %q", ErrNotFound, versionID)
	}
	if err != nil {
		return g, fmt.Errorf("worker: load guardrails: %w", err)
	}
	g.ScheduleAttachable = attach == 1
	if err := json.Unmarshal([]byte(granted), &g.GrantedTools); err != nil {
		return g, fmt.Errorf("worker: decode granted tools: %w", err)
	}
	var pm struct {
		GatedTools     []string `json:"gated_tools"`
		PermissionMode string   `json:"permission_mode"`
	}
	if err := json.Unmarshal([]byte(permMap), &pm); err != nil {
		return g, fmt.Errorf("worker: decode permission map: %w", err)
	}
	g.GatedTools, g.PermissionMode = pm.GatedTools, pm.PermissionMode
	if err := json.Unmarshal([]byte(hosts), &g.EgressHosts); err != nil {
		return g, fmt.Errorf("worker: decode egress hosts: %w", err)
	}
	return g, nil
}

// Domain loads one domains row (Spec S08.7).
func (s *Store) Domain(ctx context.Context, name string) (Domain, error) {
	var d Domain
	row := s.db.QueryRowContext(ctx,
		`SELECT domain, verification_maturity, rubric_ref, updated_ts FROM domains WHERE domain = ?`, name)
	err := row.Scan(&d.Name, (*string)(&d.Maturity), &d.RubricRef, &d.UpdatedTS)
	if err == sql.ErrNoRows {
		return d, fmt.Errorf("%w: domain %q", ErrNotFound, name)
	}
	if err != nil {
		return d, fmt.Errorf("worker: load domain: %w", err)
	}
	return d, nil
}

// CreateDomain adds a domains row, operator-gated (D10). A new domain is
// born DEGRADED — 2.1 maturity honesty: graduation to full is a separate
// SetDomainMaturity flip asserting a real quality check exists (Spec
// S08.7; the S08.1 day-one rows are the 0005 migration's).
func (s *Store) CreateDomain(ctx context.Context, actor, name string) error {
	role, err := s.person(ctx, actor)
	if err != nil {
		return err
	}
	if role != "operator" {
		return fmt.Errorf("%w: domain rows are operator acts (D10, S08.7)", ErrOperatorRequired)
	}
	if !nameRe.MatchString(name) {
		return fmt.Errorf("%w: domain %q must be lowercase dash-separated", ErrInvalid, name)
	}
	now := rfc3339(s.now())
	return s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO domains (domain, verification_maturity, rubric_ref, updated_ts) VALUES (?, ?, '', ?)`,
			name, string(MaturityDegraded), now); err != nil {
			return fmt.Errorf("worker: create domain: %w", err)
		}
		_, err := s.log.AppendTx(ctx, tx, s.event(actor, evDomain,
			map[string]any{"domain": name, "maturity": MaturityDegraded, "actor": actor, "created": true}))
		return err
	})
}

// SetDomainMaturity flips a domains row through D10: the operator's act
// (Spec S08.7 domain graduation; a degraded→full flip asserts a real
// quality check exists and passed its falsifiability floor — S07/S14).
func (s *Store) SetDomainMaturity(ctx context.Context, actor, domain string, m Maturity, rubricRef string) error {
	role, err := s.person(ctx, actor)
	if err != nil {
		return err
	}
	if role != "operator" {
		return fmt.Errorf("%w: domain maturity is a D10 operator flip (S08.7)", ErrOperatorRequired)
	}
	switch m {
	case MaturityFull, MaturityDegraded:
	default:
		return fmt.Errorf("%w: maturity %q", ErrInvalid, m)
	}
	if m == MaturityFull && strings.TrimSpace(rubricRef) == "" {
		return fmt.Errorf("%w: graduation to full requires the rubric/quality-check ref (S08.7)", ErrInvalid)
	}
	now := rfc3339(s.now())
	return s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE domains SET verification_maturity = ?, rubric_ref = ?, updated_ts = ? WHERE domain = ?`,
			string(m), rubricRef, now, domain)
		if err != nil {
			return fmt.Errorf("worker: set domain maturity: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("%w: domain %q", ErrNotFound, domain)
		}
		_, err = s.log.AppendTx(ctx, tx, s.event(actor, evDomain,
			map[string]any{"domain": domain, "maturity": m, "rubric_ref": rubricRef, "actor": actor}))
		return err
	})
}

// DeliveryPolicy is the S08.7 structural enforcement read: whether this
// worker's outputs may deliver without requester review, and why not. The
// scheduler/gate layers consume it; degraded mode and first-N are
// enforced here as data, not advisory prose.
type DeliveryPolicy struct {
	Deliverable    bool     `json:"deliverable"`
	RequiresReview bool     `json:"requires_review"`
	Reasons        []string `json:"reasons,omitempty"`
}

// Delivery computes the policy for a template's active version.
func (s *Store) Delivery(ctx context.Context, templateID string) (DeliveryPolicy, error) {
	t, err := s.Template(ctx, templateID)
	if err != nil {
		return DeliveryPolicy{}, err
	}
	var p DeliveryPolicy
	switch t.Status {
	case StatusActive:
		p.Deliverable = true
	case StatusFlagged:
		// Flagged workers may run supervised, never unsupervised (Spec
		// S08.10a).
		p.Deliverable = true
		p.RequiresReview = true
		p.Reasons = append(p.Reasons, "status flagged — revalidation pending (S08.10)")
	default:
		p.Reasons = append(p.Reasons, fmt.Sprintf("status %s — not deliverable", t.Status))
		return p, nil
	}
	d, err := s.Domain(ctx, t.Domain)
	if err != nil {
		return DeliveryPolicy{}, err
	}
	if d.Maturity == MaturityDegraded {
		// Structural, not advisory (Spec S08.7): cannot deliver without
		// requester review while the domain lacks a real quality check.
		p.RequiresReview = true
		p.Reasons = append(p.Reasons, fmt.Sprintf("domain %s is degraded (S08.7)", d.Name))
	}
	if t.ActiveVersion != "" {
		g, err := s.Guardrails(ctx, t.ActiveVersion)
		if err != nil {
			return DeliveryPolicy{}, err
		}
		if g.FirstNRemaining > 0 {
			p.RequiresReview = true
			p.Reasons = append(p.Reasons, fmt.Sprintf("supervised first-N: %d reviews remaining (S08.6)", g.FirstNRemaining))
		}
	}
	return p, nil
}

// ── Gap records (Spec S08.6 compose-when-earned; S08.8 writes them at
//    no-fit, B3-3) ──

// RecordGap upserts a gap record for a no-fit outcome and reports whether
// composition is now earned: at ⚙ workers.gap_proposal_count occurrences
// of the family a composition proposal card surfaces (14.4
// operationalized; the card surface rides the S08.8 no-fit flow, B3-3).
func (s *Store) RecordGap(ctx context.Context, signature, family, taskRef string) (GapRecord, bool, error) {
	if strings.TrimSpace(signature) == "" || strings.TrimSpace(family) == "" {
		return GapRecord{}, false, fmt.Errorf("%w: gap record needs signature and family", ErrInvalid)
	}
	threshold, err := s.settings.Int(keyGapProposalCount)
	if err != nil {
		return GapRecord{}, false, fmt.Errorf("worker: read ⚙ %s: %w", keyGapProposalCount, err)
	}
	now := rfc3339(s.now())
	var rec GapRecord
	var due bool
	err = s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		var refsRaw string
		var count int64
		var disp string
		err := tx.QueryRowContext(ctx,
			`SELECT task_refs, occurrence_count, disposition FROM gap_records WHERE signature = ?`,
			signature).Scan(&refsRaw, &count, &disp)
		switch {
		case err == sql.ErrNoRows:
			refsRaw, count, disp = "[]", 0, string(GapOpen)
		case err != nil:
			return fmt.Errorf("worker: load gap record: %w", err)
		}
		var refs []string
		if err := json.Unmarshal([]byte(refsRaw), &refs); err != nil {
			return fmt.Errorf("worker: decode gap task refs: %w", err)
		}
		if taskRef != "" {
			refs = append(refs, taskRef)
		}
		count++
		due = count >= threshold && GapDisposition(disp) == GapOpen
		if due {
			disp = string(GapProposed)
		}
		refsJSON, err := json.Marshal(refs)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO gap_records (signature, family, task_refs, occurrence_count, last_seen_ts, disposition)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT (signature) DO UPDATE SET
			   task_refs = excluded.task_refs, occurrence_count = excluded.occurrence_count,
			   last_seen_ts = excluded.last_seen_ts, disposition = excluded.disposition`,
			signature, family, string(refsJSON), count, now, disp); err != nil {
			return fmt.Errorf("worker: upsert gap record: %w", err)
		}
		rec = GapRecord{Signature: signature, Family: family, TaskRefs: refs,
			Occurrences: count, LastSeenTS: now, Disposition: GapDisposition(disp)}
		_, err = s.log.AppendTx(ctx, tx, s.event("platform", evGap, map[string]any{
			"signature": signature, "family": family, "occurrences": count,
			"disposition": disp, "proposal_due": due,
		}))
		return err
	})
	return rec, due, err
}

// SetGapDisposition records a gap outcome (composed / dismissed / re-open).
func (s *Store) SetGapDisposition(ctx context.Context, actor, signature string, d GapDisposition) error {
	if _, err := s.person(ctx, actor); err != nil {
		return err
	}
	switch d {
	case GapOpen, GapProposed, GapComposed, GapDismissed:
	default:
		return fmt.Errorf("%w: disposition %q", ErrInvalid, d)
	}
	now := rfc3339(s.now())
	return s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE gap_records SET disposition = ?, last_seen_ts = ? WHERE signature = ?`,
			string(d), now, signature)
		if err != nil {
			return fmt.Errorf("worker: set gap disposition: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("%w: gap record %q", ErrNotFound, signature)
		}
		_, err = s.log.AppendTx(ctx, tx, s.event(actor, evGap,
			map[string]any{"signature": signature, "disposition": d, "actor": actor}))
		return err
	})
}

// ── Skills (static-only, Spec S08.1) ──

// ResolveSkill implements SkillResolver over the store's skill root.
func (s *Store) ResolveSkill(name string) (Skill, error) {
	if !nameRe.MatchString(name) {
		return Skill{}, fmt.Errorf("%w: skill name %q", ErrInvalid, name)
	}
	return LoadSkill(filepath.Join(s.root, "skills", name))
}

// InstallSkill writes a skill directory after static validation — a
// human-gated act (imports/authoring both; Spec S08.10 imports pass the
// full battery when a template referencing the skill is validated).
func (s *Store) InstallSkill(ctx context.Context, actor, name string, files map[string][]byte) (Skill, error) {
	if _, err := s.person(ctx, actor); err != nil {
		return Skill{}, err
	}
	if !nameRe.MatchString(name) {
		return Skill{}, fmt.Errorf("%w: skill name %q must be lowercase dash-separated", ErrInvalid, name)
	}
	if _, ok := files["SKILL.md"]; !ok {
		return Skill{}, fmt.Errorf("%w: a skill needs SKILL.md", ErrInvalid)
	}
	dir := filepath.Join(s.root, "skills", name)
	if _, err := os.Stat(dir); err == nil {
		return Skill{}, fmt.Errorf("%w: skill %q already installed (skills are versioned with the templates that reference them)", ErrInvalid, name)
	}
	for rel, content := range files {
		clean := filepath.Clean(rel)
		if clean != rel || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			return Skill{}, fmt.Errorf("%w: skill file path %q", ErrInvalid, rel)
		}
		abs := filepath.Join(dir, clean)
		if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
			return Skill{}, fmt.Errorf("worker: create skill dir: %w", err)
		}
		if err := os.WriteFile(abs, content, 0o600); err != nil {
			return Skill{}, fmt.Errorf("worker: write skill file: %w", err)
		}
	}
	sk, err := LoadSkill(dir)
	if err == nil && len(sk.Findings) > 0 {
		err = fmt.Errorf("%w: %s", ErrLintReject, sk.Findings[0].Message)
	}
	if err != nil {
		_ = os.RemoveAll(dir)
		return Skill{}, err
	}
	return sk, nil
}

// skillRefFor hashes a resolved skill dir into its compile ref (compile.go
// pins skill content inside the config hash — Spec S08.3 one-unit rule).
func (s *Store) skillRefFor(name string) (CompiledSkillRef, error) {
	sk, err := s.ResolveSkill(name)
	if err != nil {
		return CompiledSkillRef{}, err
	}
	paths := append([]string{"SKILL.md"}, sk.Files...)
	sort.Strings(paths)
	var all []byte
	for _, rel := range paths {
		b, err := os.ReadFile(filepath.Join(sk.Dir, rel))
		if err != nil {
			return CompiledSkillRef{}, fmt.Errorf("worker: hash skill %s: %w", name, err)
		}
		all = append(all, []byte(rel)...)
		all = append(all, 0)
		all = append(all, b...)
		all = append(all, 0)
	}
	return CompiledSkillRef{Name: name, Dir: sk.Dir, SHA256: sha256Hex(all)}, nil
}

// readTemplateFile loads and hash-verifies a version's file (Spec S08.3:
// the hash check catches the tamper; a stale/tampered file never
// compiles).
func (s *Store) readTemplateFile(v Version) (string, error) {
	raw, err := os.ReadFile(filepath.Join(s.root, v.FilePath))
	if err != nil {
		return "", fmt.Errorf("worker: read template file: %w", err)
	}
	if sha256Hex(raw) != v.FileSHA256 {
		return "", fmt.Errorf("%w: %s", ErrTamperedFile, v.FilePath)
	}
	return string(raw), nil
}

// writeTemplateFile writes the canonical bytes for a new version.
func (s *Store) writeTemplateFile(templateID string, version int64, canonical string) (relPath, sha string, err error) {
	relPath = filepath.Join("templates", templateID, fmt.Sprintf("v%d.md", version))
	abs := filepath.Join(s.root, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return "", "", fmt.Errorf("worker: create template dir: %w", err)
	}
	if err := os.WriteFile(abs, []byte(canonical), 0o600); err != nil {
		return "", "", fmt.Errorf("worker: write template file: %w", err)
	}
	return relPath, sha256Hex([]byte(canonical)), nil
}
