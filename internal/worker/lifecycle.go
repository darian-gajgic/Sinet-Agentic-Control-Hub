package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/worker/automation"
)

// lifecycle.go — the store's template lifecycle verbs (Spec S08.4):
// versions are immutable rows + file commits; active_version is the only
// mutable pointer; rollback = repoint; approval is the one path from
// requested to granted (Spec S08.2); revalidation flags are S08.10's.

// Provenance is the version provenance block (Spec S08.1).
type Provenance struct {
	AuthorKind  string // "human" | "composer"
	Composer    string // model+version when composer-drafted
	PlaybookVer string // composer playbook version when composer-drafted
	EvidenceRef string // gap record / spec refs
	Origin      Origin
	OriginRef   string
}

func (p Provenance) validate() error {
	switch p.AuthorKind {
	case "human", "composer":
	default:
		return fmt.Errorf("%w: author kind %q", ErrInvalid, p.AuthorKind)
	}
	switch p.Origin {
	case OriginComposed, OriginHumanWritten, OriginImported, OriginAdoptedFrom:
	default:
		return fmt.Errorf("%w: origin %q", ErrInvalid, p.Origin)
	}
	return nil
}

// parseDraft parses+canonicalizes a draft source, rejecting guardrail
// fields at entry (Spec S08.2: the structural reject — a guardrail-class
// field never even lands as a draft file).
func parseDraft(src string) (Definition, string, []Finding, error) {
	def, findings, err := ParseTemplate(src)
	if err != nil {
		return def, "", nil, err
	}
	for _, f := range findings {
		if f.Code == FindingGuardrail {
			return def, "", findings, fmt.Errorf("%w: %s", ErrGuardrailField, f.Message)
		}
	}
	return def, RenderTemplate(def), findings, nil
}

// CreateDraft creates a template with its v1 draft version from a template
// document plus its requested grants (control-plane draft, never in the
// file). Human-gated; the owner is the actor. Imports use this same path
// with Origin=OriginImported — no auto-import from any registry, EVER
// (Spec S08.10, P-T16-4): reaching this verb requires a human actor.
func (s *Store) CreateDraft(ctx context.Context, actor string, src string, req RequestedGrants, prov Provenance) (Template, Version, error) {
	if _, err := s.person(ctx, actor); err != nil {
		return Template{}, Version{}, err
	}
	if err := prov.validate(); err != nil {
		return Template{}, Version{}, err
	}
	def, canonical, _, err := parseDraft(src)
	if err != nil {
		return Template{}, Version{}, err
	}
	if def.Name == "" || def.Domain == "" || def.Kind == "" {
		return Template{}, Version{}, fmt.Errorf("%w: a draft needs name, domain, and kind (full schema lint is station 1's)", ErrInvalid)
	}
	if _, err := s.Domain(ctx, def.Domain); err != nil {
		return Template{}, Version{}, err
	}

	now := rfc3339(s.now())
	t := Template{
		ID: newID("wt"), Owner: actor, Name: def.Name, Scope: ScopePersonal,
		Kind: def.Kind, Domain: def.Domain, Status: StatusDraft,
		CreatedTS: now, UpdatedTS: now,
	}
	v, err := s.insertVersion(ctx, t, 1, "", canonical, req, prov, actor, now, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO worker_templates (template_id, user_id, name, scope, kind, domain, status, created_ts, updated_ts)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			t.ID, t.Owner, t.Name, string(t.Scope), string(t.Kind), t.Domain, string(t.Status), now, now)
		if err != nil {
			return fmt.Errorf("worker: insert template: %w", err)
		}
		_, err = s.log.AppendTx(ctx, tx, s.event(actor, evTemplateCreated, map[string]any{
			"template": t.ID, "name": t.Name, "kind": t.Kind, "domain": t.Domain, "origin": prov.Origin,
		}))
		return err
	})
	if err != nil {
		return Template{}, Version{}, err
	}
	return t, v, nil
}

// NewVersion creates the next immutable version of a template from an
// edited document (Spec S08.4: every edit is a new version row plus a new
// file commit; history never rewritten). Owner edits own templates; a
// household-shared template's edits are operator-gated new versions.
func (s *Store) NewVersion(ctx context.Context, actor, templateID, src string, req RequestedGrants, prov Provenance) (Version, error) {
	role, err := s.person(ctx, actor)
	if err != nil {
		return Version{}, err
	}
	t, err := s.Template(ctx, templateID)
	if err != nil {
		return Version{}, err
	}
	if t.Scope == ScopeHousehold && role != "operator" {
		return Version{}, fmt.Errorf("%w: edits to a shared template are operator-gated new versions (S08.4)", ErrOperatorRequired)
	}
	if t.Scope == ScopePersonal && t.Owner != actor && role != "operator" {
		return Version{}, ErrNotOwner
	}
	if err := prov.validate(); err != nil {
		return Version{}, err
	}
	def, canonical, _, err := parseDraft(src)
	if err != nil {
		return Version{}, err
	}
	if def.Kind != t.Kind {
		return Version{}, fmt.Errorf("%w: kind is fixed at creation (%s)", ErrKindMismatch, t.Kind)
	}
	if def.Domain != t.Domain {
		return Version{}, fmt.Errorf("%w: domain is fixed at creation — a new domain is a new template (S08.1)", ErrInvalid)
	}
	prev, err := s.latestVersion(ctx, templateID)
	if err != nil {
		return Version{}, err
	}
	now := rfc3339(s.now())
	return s.insertVersion(ctx, t, prev.Version+1, prev.ID, canonical, req, prov, actor, now, nil)
}

// insertVersion writes the file then the row(s) in one tx; the file is
// removed if the tx fails.
func (s *Store) insertVersion(ctx context.Context, t Template, num int64, supersedes, canonical string,
	req RequestedGrants, prov Provenance, actor, now string, extra func(tx *sql.Tx) error) (Version, error) {

	grantsJSON, err := json.Marshal(req)
	if err != nil {
		return Version{}, fmt.Errorf("worker: marshal requested grants: %w", err)
	}
	relPath, sha, err := s.writeTemplateFile(t.ID, num, canonical)
	if err != nil {
		return Version{}, err
	}
	v := Version{
		ID: newID("wtv"), TemplateID: t.ID, Version: num, Supersedes: supersedes,
		FilePath: relPath, FileSHA256: sha, Requested: req,
		AuthorKind: prov.AuthorKind, Composer: prov.Composer, PlaybookVer: prov.PlaybookVer,
		EvidenceRef: prov.EvidenceRef, Origin: prov.Origin, OriginRef: prov.OriginRef,
		CreatedBy: actor, CreatedTS: now,
	}
	err = s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if extra != nil {
			if err := extra(tx); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO worker_template_versions (version_id, template_id, version, supersedes_id,
			   file_path, file_sha256, requested_grants, author_kind, composer_model,
			   composer_playbook_version, evidence_ref, origin, origin_ref, created_by, created_ts)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			v.ID, v.TemplateID, v.Version, nullable(v.Supersedes), v.FilePath, v.FileSHA256,
			string(grantsJSON), v.AuthorKind, v.Composer, v.PlaybookVer, v.EvidenceRef,
			string(v.Origin), v.OriginRef, v.CreatedBy, v.CreatedTS)
		if err != nil {
			return fmt.Errorf("worker: insert version: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE worker_templates SET updated_ts = ? WHERE template_id = ?`, now, t.ID); err != nil {
			return fmt.Errorf("worker: touch template: %w", err)
		}
		_, err = s.log.AppendTx(ctx, tx, s.event(actor, evVersionCreated, map[string]any{
			"template": t.ID, "version": v.ID, "n": num, "file_sha256": sha, "origin": prov.Origin,
		}))
		return err
	})
	if err != nil {
		_ = os.Remove(filepath.Join(s.root, relPath))
		return Version{}, err
	}
	return v, nil
}

// ApproveOpts parameterizes station-4 approval.
type ApproveOpts struct {
	// AckFlagged lists the above-ceiling audit line items the approver
	// explicitly acknowledges (Spec S08.6 station 2: flagged lines on the
	// approval card). Every flagged item must be listed or approval
	// refuses (ErrAboveCeiling).
	AckFlagged []string
}

// Approve is station 4 (Spec S08.6, D10): the owner approves personal
// workers (the operator may approve any). It requires a green validation
// record for the version, copies requested → granted into the guardrails
// row (Spec S08.2: the ONLY writer of enforcement state, on this human
// approval), seeds the first-N counter (⚙ workers.first_n; carried from
// the predecessor when the diff touches neither body nor equipment — G3
// D3.4), repoints active_version, and stamps approval on the version and
// the validation record.
func (s *Store) Approve(ctx context.Context, actor, versionID string, opts ApproveOpts) (Guardrails, error) {
	role, err := s.person(ctx, actor)
	if err != nil {
		return Guardrails{}, err
	}
	v, err := s.VersionByID(ctx, versionID)
	if err != nil {
		return Guardrails{}, err
	}
	t, err := s.Template(ctx, v.TemplateID)
	if err != nil {
		return Guardrails{}, err
	}
	if t.Owner != actor && role != "operator" {
		return Guardrails{}, fmt.Errorf("%w: the owner approves personal workers (D10)", ErrNotOwner)
	}
	if t.Status == StatusRetired {
		return Guardrails{}, fmt.Errorf("%w: template is retired", ErrNotActive)
	}
	rec, err := s.LatestValidation(ctx, versionID)
	if err != nil {
		return Guardrails{}, err
	}
	if !rec.Green {
		return Guardrails{}, fmt.Errorf("%w: latest battery pass is red (S08.6)", ErrNotValidated)
	}
	acked := map[string]bool{}
	for _, id := range opts.AckFlagged {
		acked[id] = true
	}
	for _, item := range rec.Audit.FlaggedItems() {
		if !acked[item] {
			return Guardrails{}, fmt.Errorf("%w: %s (flagged on the approval card, S08.6 station 2)", ErrAboveCeiling, item)
		}
	}

	// First-N seed: reset on a body/equipment diff, carry otherwise (G3
	// D3.4; Spec S08.4).
	firstN, err := s.settings.Int(keyFirstN)
	if err != nil {
		return Guardrails{}, fmt.Errorf("worker: read ⚙ %s: %w", keyFirstN, err)
	}
	remaining := firstN
	if v.Supersedes != "" {
		carried, ok, err := s.carriedFirstN(ctx, v)
		if err != nil {
			return Guardrails{}, err
		}
		if ok {
			remaining = carried
		}
	}

	// The version becoming active feeds the S08.8 selection index; parse
	// the hash-verified file now so the index refresh rides the same tx
	// (search.go; migration 0006).
	src, err := s.readTemplateFile(v)
	if err != nil {
		return Guardrails{}, err
	}
	def, _, err := ParseTemplate(src)
	if err != nil {
		return Guardrails{}, err
	}

	pm, err := json.Marshal(map[string]any{
		"gated_tools": v.Requested.GatedTools, "permission_mode": v.Requested.PermissionMode,
	})
	if err != nil {
		return Guardrails{}, err
	}
	granted, err := json.Marshal(orEmpty(v.Requested.Tools))
	if err != nil {
		return Guardrails{}, err
	}
	hosts, err := json.Marshal(orEmpty(v.Requested.EgressHosts))
	if err != nil {
		return Guardrails{}, err
	}
	now := rfc3339(s.now())
	g := Guardrails{
		VersionID: v.ID, GrantedTools: v.Requested.Tools, GatedTools: v.Requested.GatedTools,
		PermissionMode: v.Requested.PermissionMode, Class: v.Requested.Class,
		Egress: v.Requested.Egress, EgressHosts: v.Requested.EgressHosts,
		BudgetUSD: v.Requested.BudgetUSD, BudgetSteps: v.Requested.BudgetSteps,
		GatePolicy: "{}", FirstNRemaining: remaining,
		ScheduleAttachable: v.Requested.ScheduleAttachable,
		CreatedTS:          now, UpdatedTS: now,
	}
	err = s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO worker_guardrails (version_id, granted_tools, permission_map, confinement_class,
			   egress_class, egress_hosts, budget_ceiling_usd, budget_ceiling_steps, gate_policy,
			   first_n_remaining, schedule_attachable, created_ts, updated_ts)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, '{}', ?, ?, ?, ?)`,
			g.VersionID, string(granted), string(pm), g.Class, string(g.Egress), string(hosts),
			g.BudgetUSD, g.BudgetSteps, g.FirstNRemaining, boolInt(g.ScheduleAttachable), now, now); err != nil {
			return fmt.Errorf("worker: write guardrails: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE worker_template_versions SET approved_by = ?, approved_ts = ? WHERE version_id = ?`,
			actor, now, v.ID); err != nil {
			return fmt.Errorf("worker: stamp version approval: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE validation_records SET approver = ?, approved_ts = ? WHERE record_id = ? AND approver = ''`,
			actor, now, rec.ID); err != nil {
			return fmt.Errorf("worker: stamp validation approval: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE worker_templates SET status = ?, active_version = ?, updated_ts = ? WHERE template_id = ?`,
			string(StatusActive), v.ID, now, t.ID); err != nil {
			return fmt.Errorf("worker: activate version: %w", err)
		}
		if err := refreshSearchTx(ctx, tx, t.ID, def); err != nil {
			return err
		}
		_, err := s.log.AppendTx(ctx, tx, s.event(actor, evApproved, map[string]any{
			"template": t.ID, "version": v.ID, "approver": actor,
			"first_n": remaining, "acked_flags": opts.AckFlagged,
		}))
		return err
	})
	if err != nil {
		return Guardrails{}, err
	}
	return g, nil
}

// carriedFirstN loads the predecessor's counter when the diff touches
// neither body nor equipment (G3 D3.4 reset rule).
func (s *Store) carriedFirstN(ctx context.Context, v Version) (int64, bool, error) {
	prev, err := s.VersionByID(ctx, v.Supersedes)
	if err != nil {
		return 0, false, err
	}
	prevG, err := s.Guardrails(ctx, prev.ID)
	if errors.Is(err, ErrNotFound) {
		// Predecessor never approved: nothing to carry.
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	prevSrc, err := s.readTemplateFile(prev)
	if err != nil {
		return 0, false, err
	}
	newSrc, err := s.readTemplateFile(v)
	if err != nil {
		return 0, false, err
	}
	prevDef, _, err := ParseTemplate(prevSrc)
	if err != nil {
		return 0, false, err
	}
	newDef, _, err := ParseTemplate(newSrc)
	if err != nil {
		return 0, false, err
	}
	if prevDef.Body == newDef.Body && equipmentEqual(prevDef.Equipment, newDef.Equipment) {
		return prevG.FirstNRemaining, true, nil
	}
	return 0, false, nil
}

// Promote flips a template personal → household (D10, 12.5): an operator
// approval carrying the personal-data scan — the template must not embed
// user-scope content; overlays hold the personal part by construction
// (Spec S08.4). Shared templates are read-only references; edits become
// operator-gated new versions (NewVersion enforces).
func (s *Store) Promote(ctx context.Context, actor, templateID string) error {
	role, err := s.person(ctx, actor)
	if err != nil {
		return err
	}
	if role != "operator" {
		return fmt.Errorf("%w: household promotion is the operator's approval (D10)", ErrOperatorRequired)
	}
	t, err := s.Template(ctx, templateID)
	if err != nil {
		return err
	}
	if t.Status != StatusActive || t.ActiveVersion == "" {
		return fmt.Errorf("%w: only an active, validated template promotes (S08.4)", ErrNotActive)
	}
	v, err := s.VersionByID(ctx, t.ActiveVersion)
	if err != nil {
		return err
	}
	src, err := s.readTemplateFile(v)
	if err != nil {
		return err
	}
	// Personal-data scan (deterministic lint rule): the definition must
	// not embed its owner's identity — the overlay is where personal
	// content lives (Spec S08.4).
	if strings.Contains(src, t.Owner) {
		return fmt.Errorf("%w: template embeds owner-scope content (%q) — personal content belongs in overlays (S08.4)", ErrInvalid, t.Owner)
	}
	now := rfc3339(s.now())
	return s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`UPDATE worker_templates SET scope = ?, updated_ts = ? WHERE template_id = ?`,
			string(ScopeHousehold), now, templateID); err != nil {
			return fmt.Errorf("worker: promote: %w", err)
		}
		_, err := s.log.AppendTx(ctx, tx, s.event(actor, evPromoted, map[string]any{
			"template": templateID, "version": t.ActiveVersion, "approver": actor,
		}))
		return err
	})
}

// Repoint moves active_version to a previously approved version — Spec
// S08.4 "rollback = repoint". The target must carry its own approval and a
// green validation record (its records stand; no re-run).
func (s *Store) Repoint(ctx context.Context, actor, templateID, versionID string) error {
	role, err := s.person(ctx, actor)
	if err != nil {
		return err
	}
	t, err := s.Template(ctx, templateID)
	if err != nil {
		return err
	}
	if t.Owner != actor && role != "operator" {
		return ErrNotOwner
	}
	v, err := s.VersionByID(ctx, versionID)
	if err != nil {
		return err
	}
	if v.TemplateID != templateID {
		return fmt.Errorf("%w: version %q belongs to another template", ErrInvalid, versionID)
	}
	if v.ApprovedTS == "" {
		return fmt.Errorf("%w: rollback targets a previously approved version (S08.4)", ErrNotValidated)
	}
	rec, err := s.LatestValidation(ctx, versionID)
	if err != nil {
		return err
	}
	if !rec.Green {
		return fmt.Errorf("%w: target version's latest battery pass is red", ErrNotValidated)
	}
	// The repoint target's definition feeds the S08.8 selection index
	// (search.go; the same-tx refresh discipline as Approve).
	src, err := s.readTemplateFile(v)
	if err != nil {
		return err
	}
	def, _, err := ParseTemplate(src)
	if err != nil {
		return err
	}
	now := rfc3339(s.now())
	return s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`UPDATE worker_templates SET active_version = ?, status = ?, updated_ts = ? WHERE template_id = ?`,
			versionID, string(StatusActive), now, templateID); err != nil {
			return fmt.Errorf("worker: repoint: %w", err)
		}
		if err := refreshSearchTx(ctx, tx, templateID, def); err != nil {
			return err
		}
		_, err := s.log.AppendTx(ctx, tx, s.event(actor, evRepointed, map[string]any{
			"template": templateID, "version": versionID, "actor": actor,
		}))
		return err
	})
}

// Retire flips a template to retired (Spec S08.9: rollback = alias
// repoint; retirement = status flip + schedule detachment + cards —
// schedule surfaces are v1; attachment already requires status=active).
func (s *Store) Retire(ctx context.Context, actor, templateID string) error {
	role, err := s.person(ctx, actor)
	if err != nil {
		return err
	}
	t, err := s.Template(ctx, templateID)
	if err != nil {
		return err
	}
	if t.Owner != actor && role != "operator" {
		return ErrNotOwner
	}
	now := rfc3339(s.now())
	return s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`UPDATE worker_templates SET status = ?, updated_ts = ? WHERE template_id = ?`,
			string(StatusRetired), now, templateID); err != nil {
			return fmt.Errorf("worker: retire: %w", err)
		}
		_, err := s.log.AppendTx(ctx, tx, s.event(actor, evRetired,
			map[string]any{"template": templateID, "actor": actor}))
		return err
	})
}

// FlagByModel flags every template whose ACTIVE version references the
// model — validation-record model slot or the definition's recorded model
// pin (Spec S08.10a: model change/deprecation → flagged, MUST revalidate
// before further unsupervised use; flagged workers may run supervised).
func (s *Store) FlagByModel(ctx context.Context, model, reason string) ([]string, error) {
	return s.flagWhere(ctx, model, reason, func(ctx context.Context, t Template, v Version) (bool, error) {
		var n int
		err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM validation_records WHERE version_id = ? AND model = ?`,
			v.ID, model).Scan(&n)
		if err != nil {
			return false, err
		}
		if n > 0 {
			return true, nil
		}
		src, err := s.readTemplateFile(v)
		if err != nil {
			return false, err
		}
		def, _, err := ParseTemplate(src)
		if err != nil {
			return false, err
		}
		return def.Profile.ModelPin == model, nil
	})
}

// FlagByEnginePin flags every template whose ACTIVE version was validated
// against the pin — an engine-pin bump is a mass revalidation event
// (P-T14-1, Spec S08.10b); a dialect-version bump carries the same
// semantics for automations (Spec S08.9).
func (s *Store) FlagByEnginePin(ctx context.Context, pin, reason string) ([]string, error) {
	return s.flagWhere(ctx, pin, reason, func(ctx context.Context, t Template, v Version) (bool, error) {
		var n int
		err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM validation_records WHERE version_id = ? AND engine_pin = ?`,
			v.ID, pin).Scan(&n)
		return n > 0, err
	})
}

func (s *Store) flagWhere(ctx context.Context, subject, reason string,
	match func(context.Context, Template, Version) (bool, error)) ([]string, error) {

	rows, err := s.db.QueryContext(ctx,
		`SELECT `+templateColumns+` FROM worker_templates WHERE status IN ('active', 'validated')`)
	if err != nil {
		return nil, fmt.Errorf("worker: scan templates: %w", err)
	}
	var candidates []Template
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		candidates = append(candidates, t)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	var flagged []string
	now := rfc3339(s.now())
	for _, t := range candidates {
		if t.ActiveVersion == "" {
			continue
		}
		v, err := s.VersionByID(ctx, t.ActiveVersion)
		if err != nil {
			return flagged, err
		}
		hit, err := match(ctx, t, v)
		if err != nil {
			return flagged, err
		}
		if !hit {
			continue
		}
		err = s.db.WriteTx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx,
				`UPDATE worker_templates SET status = ?, updated_ts = ? WHERE template_id = ?`,
				string(StatusFlagged), now, t.ID); err != nil {
				return err
			}
			_, err := s.log.AppendTx(ctx, tx, s.event(t.Owner, evFlagged, map[string]any{
				"template": t.ID, "subject": subject, "reason": reason,
			}))
			return err
		})
		if err != nil {
			return flagged, fmt.Errorf("worker: flag %s: %w", t.ID, err)
		}
		flagged = append(flagged, t.ID)
	}
	return flagged, nil
}

// RecordSupervisedReview records one requester review of a supervised
// first-N output (Spec S08.6: count-based, requester review regardless of
// oversight settings). Reaching zero records the graduation event on the
// version (worker graduation additionally requires domain=full — Spec
// S08.7; Delivery reads both).
func (s *Store) RecordSupervisedReview(ctx context.Context, actor, versionID string) (int64, error) {
	if _, err := s.person(ctx, actor); err != nil {
		return 0, err
	}
	g, err := s.Guardrails(ctx, versionID)
	if err != nil {
		return 0, err
	}
	if g.FirstNRemaining == 0 {
		return 0, nil
	}
	remaining := g.FirstNRemaining - 1
	now := rfc3339(s.now())
	err = s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`UPDATE worker_guardrails SET first_n_remaining = ?, updated_ts = ? WHERE version_id = ?`,
			remaining, now, versionID); err != nil {
			return fmt.Errorf("worker: decrement first-N: %w", err)
		}
		if _, err := s.log.AppendTx(ctx, tx, s.event(actor, evReview,
			map[string]any{"version": versionID, "remaining": remaining, "reviewer": actor})); err != nil {
			return err
		}
		if remaining == 0 {
			if _, err := tx.ExecContext(ctx,
				`UPDATE worker_template_versions SET graduated_ts = ? WHERE version_id = ? AND graduated_ts IS NULL`,
				now, versionID); err != nil {
				return fmt.Errorf("worker: stamp graduation: %w", err)
			}
			if _, err := s.log.AppendTx(ctx, tx, s.event(actor, evGraduated,
				map[string]any{"version": versionID})); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return remaining, nil
}

// CompileForRun is the per-invocation compile entry (Spec S08.3): active
// template → hash-verified file → parsed definition + granted guardrails +
// the requester's overlay slice (through the S09 machinery; empty at v0 by
// dormancy) + instance refs → the compiled unit. The B3-3 dispatcher calls
// this per spawn and records ConfigHash on the run.
func (s *Store) CompileForRun(ctx context.Context, templateID, requester string, inst InstanceRefs) (Compiled, error) {
	t, err := s.Template(ctx, templateID)
	if err != nil {
		return Compiled{}, err
	}
	if t.Status != StatusActive && t.Status != StatusFlagged {
		return Compiled{}, fmt.Errorf("%w: status %s", ErrNotActive, t.Status)
	}
	if t.ActiveVersion == "" {
		return Compiled{}, fmt.Errorf("%w: no active version", ErrNotActive)
	}
	v, err := s.VersionByID(ctx, t.ActiveVersion)
	if err != nil {
		return Compiled{}, err
	}
	g, err := s.Guardrails(ctx, v.ID)
	if err != nil {
		return Compiled{}, err
	}
	src, err := s.readTemplateFile(v)
	if err != nil {
		return Compiled{}, err
	}
	def, findings, err := ParseTemplate(src)
	if err != nil {
		return Compiled{}, err
	}
	for _, f := range findings {
		if f.Code == FindingGuardrail {
			return Compiled{}, fmt.Errorf("%w: %s", ErrGuardrailField, f.Message)
		}
	}
	var overlay []OverlayItem
	if s.overlays != nil {
		overlay, err = s.overlays.OverlaySlice(ctx, requester, templateID)
		if err != nil {
			return Compiled{}, fmt.Errorf("worker: overlay slice: %w", err)
		}
	}
	var skills []CompiledSkillRef
	for _, name := range def.Equipment.Skills {
		ref, err := s.skillRefFor(name)
		if err != nil {
			return Compiled{}, err
		}
		skills = append(skills, ref)
	}
	return CompileInvocation(CompileInput{
		TemplateID: t.ID, VersionID: v.ID, Def: def, FileSHA256: v.FileSHA256,
		Guardrails: g, Overlay: overlay, Instance: inst, Skills: skills,
	})
}

// RunAutomation executes a kind=automation template on demand (Spec S08.9:
// v0 surface — no schedules): active version, hash-verified body, parsed
// in the pinned dialect, executed with no model in the loop; outward steps
// journal gated proposals. The invoker is billed/attributed (3.4).
func (s *Store) RunAutomation(ctx context.Context, actor, templateID string, payload json.RawMessage, verbs automation.Verbs, journal *gates.Journal) (automation.Report, error) {
	if _, err := s.person(ctx, actor); err != nil {
		return automation.Report{}, err
	}
	t, err := s.Template(ctx, templateID)
	if err != nil {
		return automation.Report{}, err
	}
	if t.Kind != KindAutomation {
		return automation.Report{}, fmt.Errorf("%w: %s is not an automation", ErrKindMismatch, templateID)
	}
	if t.Status != StatusActive {
		return automation.Report{}, fmt.Errorf("%w: status %s", ErrNotActive, t.Status)
	}
	v, err := s.VersionByID(ctx, t.ActiveVersion)
	if err != nil {
		return automation.Report{}, err
	}
	src, err := s.readTemplateFile(v)
	if err != nil {
		return automation.Report{}, err
	}
	def, _, err := ParseTemplate(src)
	if err != nil {
		return automation.Report{}, err
	}
	wf, err := automation.Parse(def.Body)
	if err != nil {
		return automation.Report{}, err
	}
	rep, execErr := automation.Execute(ctx, automation.ExecInput{
		Workflow: wf, Payload: payload, Verbs: verbs, Journal: journal, UserID: actor,
	})
	payloadSHA := ""
	if len(payload) > 0 {
		payloadSHA = sha256Hex(payload)
	}
	ev := map[string]any{
		"template": t.ID, "version": v.ID, "dialect": automation.DialectVersion,
		"invoker": actor, "payload_sha256": payloadSHA, "steps": len(rep.Steps),
	}
	if execErr != nil {
		ev["error"] = execErr.Error()
	}
	if _, logErr := s.log.Append(ctx, s.event(actor, evAutomationRun, ev)); logErr != nil && execErr == nil {
		return rep, logErr
	}
	return rep, execErr
}

func nullable(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func orEmpty(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}
