package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// composer.go — the S08.6 composer: ONE-SHOT generation behind an engine
// seam, never archive/search evolution (search-based meta-agents are
// uneconomical at household n, archive-learning measures harmful, and
// self-referential optimizers game their validators — Spec S08.6). The
// generation session is ceremony on the requester's planning-model duty
// seat (the engine seam's production implementation dispatches it; Spec
// S08.6 "frontier-class flat-rate lane per the requester's duty map"),
// billed and itemized to the requester.
//
// Inputs, injected BY POLICY (Spec S08.6, exhaustively): the task spec +
// gap record; the composer playbook — the versioned S09.10 house knowledge
// object, current approved version; the palette (S08.5 — options, never
// defaults; EMPTY at v0: no palette raw material has been imported); the
// template schema, lint rules, and ceiling table. Nothing else enters.
//
// Output: a template file draft + requested grants + a proposed dry-run
// sample task + a draft golden-case seed — feeding INTO the four-station
// battery (Spec S08.6): the draft lands via CreateDraft (origin=composed,
// full provenance) and the battery still gates adoption; the composer
// NEVER iterates a draft against battery results — that would be the
// banned generate-test-refine loop.

// Playbook is the composer playbook input — the current approved version
// of the S09.10 house object ("the composer reads the current approved
// version"; storage and governance are S09's, read through a seam at the
// composition root — this package never imports the memory store).
type Playbook struct {
	EntryID string
	Version int64
	Content string
}

// PaletteBundle is one S08.5 palette entry: harvested raw material re-cut
// as a tool+context bundle, persona stripped — an OPTION for the composer,
// never a default and never a standing formation (14.4). The v0 palette is
// empty; bundles arrive only through a deliberate harvest import.
type PaletteBundle struct {
	Name    string   `json:"name"`
	Tools   []string `json:"tools,omitempty"`
	Context string   `json:"context,omitempty"`
}

// ComposeInput is one composition request.
type ComposeInput struct {
	// TaskID is the requesting task (provenance origin_ref; billing rides
	// the caller's ceremony run).
	TaskID string
	// TaskSpec is the task specification input (the drafted SPEC content —
	// restatement, acceptance criteria, constraints).
	TaskSpec string
	// GapSignature names the gap record earning this composition; the
	// record itself is loaded from the store (a policy input — every no-fit
	// wrote one, Spec S08.8).
	GapSignature string
	// Playbook is the current approved composer playbook (required by
	// policy).
	Playbook Playbook
	// Palette lists the available palette bundles (empty at v0).
	Palette []PaletteBundle
}

// ComposeRequest is the assembled one-shot request an engine seam receives:
// the policy inputs plus the platform-rendered reference (template schema,
// lint rules, ceiling table, registered domains).
type ComposeRequest struct {
	Requester string
	TaskID    string
	TaskSpec  string
	Gap       GapRecord
	Playbook  Playbook
	Palette   []PaletteBundle
	Reference string
}

// ComposeOutput is the S08.6 composer output contract.
type ComposeOutput struct {
	// TemplateDraft is the full template file draft (markdown + YAML
	// frontmatter, the S08.1 format).
	TemplateDraft string `json:"template"`
	// Requested are the requested grants — control-plane draft rows, never
	// file content (Spec S08.2); approval copies requested → granted only
	// after the station-2 permission audit.
	Requested RequestedGrants `json:"grants"`
	// SampleTask is the proposed station-3 dry-run sample; the requester
	// curates it on the approval card (edit/replace = a fresh battery pass,
	// Spec S08.6 sample authorship).
	SampleTask string `json:"sample_task"`
	// GoldenNote is the draft golden-case seed note (the dry-run pair seeds
	// the golden set labeled unverified, R15-OQ4 flow).
	GoldenNote string `json:"golden_note,omitempty"`
	// Model is the engine-reported model that drafted (provenance block
	// "composer(model+version, playbook version)"); set by the engine seam,
	// never by the model output.
	Model string `json:"-"`
}

// ComposeEngine is the one-shot generation seam. ComposeOnce runs EXACTLY
// one generation ceremony for the request — implementations never loop,
// sample, or search over candidates (the one-shot rule, Spec S08.6); the
// production implementation is an engine session on the requester's
// planning-model duty seat, tests use fakes (zero paid calls).
type ComposeEngine interface {
	ComposeOnce(ctx context.Context, req ComposeRequest) (ComposeOutput, error)
}

// ErrComposeRejected marks a composition whose one shot produced an
// unusable draft (unparseable template, guardrail-class field, missing
// identity fields). The shot is NOT retried — regenerating against the
// rejection would be the banned refine loop; the gap record keeps its
// standing disposition and a human may author by hand (the S08.8 no-fit
// default) or request composition again with changed inputs.
var ErrComposeRejected = errors.New("worker: composer draft rejected — the one shot is not retried (S08.6)")

// Compose runs the S08.6 composition: assemble the inputs by policy, take
// the ONE shot through the engine seam, land the draft via CreateDraft with
// composer provenance, and mark the gap composed. The battery is NOT run
// here — the caller drives RunBattery with the proposed sample task, so
// generation and validation stay structurally separate (the composer can
// never see battery results).
func (s *Store) Compose(ctx context.Context, actor string, in ComposeInput, eng ComposeEngine) (Template, Version, ComposeOutput, error) {
	if _, err := s.person(ctx, actor); err != nil {
		return Template{}, Version{}, ComposeOutput{}, err
	}
	if eng == nil {
		return Template{}, Version{}, ComposeOutput{}, fmt.Errorf("%w: composition needs a ComposeEngine (S08.6)", ErrInvalid)
	}
	if strings.TrimSpace(in.Playbook.Content) == "" {
		return Template{}, Version{}, ComposeOutput{}, fmt.Errorf(
			"%w: the composer playbook is a policy input — no current approved S09.10 house object was supplied (S08.6)", ErrInvalid)
	}
	gap, err := s.Gap(ctx, in.GapSignature)
	if err != nil {
		return Template{}, Version{}, ComposeOutput{}, err
	}
	reference, err := s.composeReference(ctx)
	if err != nil {
		return Template{}, Version{}, ComposeOutput{}, err
	}

	out, err := eng.ComposeOnce(ctx, ComposeRequest{
		Requester: actor,
		TaskID:    in.TaskID,
		TaskSpec:  in.TaskSpec,
		Gap:       gap,
		Playbook:  in.Playbook,
		Palette:   in.Palette,
		Reference: reference,
	})
	if err != nil {
		return Template{}, Version{}, ComposeOutput{}, fmt.Errorf("worker: compose generation: %w", err)
	}
	if strings.TrimSpace(out.TemplateDraft) == "" {
		return Template{}, Version{}, out, fmt.Errorf("%w: empty template draft", ErrComposeRejected)
	}
	if out.Model == "" {
		return Template{}, Version{}, out, fmt.Errorf("%w: the engine seam must report the drafting model (S08.1 provenance)", ErrInvalid)
	}

	prov := Provenance{
		AuthorKind:  "composer",
		Composer:    out.Model,
		PlaybookVer: fmt.Sprintf("%s@v%d", in.Playbook.EntryID, in.Playbook.Version),
		EvidenceRef: "gap:" + in.GapSignature,
		Origin:      OriginComposed,
		OriginRef:   in.TaskID,
	}
	t, v, err := s.CreateDraft(ctx, actor, out.TemplateDraft, out.Requested, prov)
	if err != nil {
		// The draft did not survive entry validation (parse failure,
		// guardrail-class field — the S08.2 structural reject fires at
		// CreateDraft): the composition is rejected, not retried.
		return Template{}, Version{}, out, fmt.Errorf("%w: %v", ErrComposeRejected, err)
	}
	if err := s.SetGapDisposition(ctx, actor, in.GapSignature, GapComposed); err != nil {
		return t, v, out, err
	}
	return t, v, out, nil
}

// Gap reads one gap record by signature.
func (s *Store) Gap(ctx context.Context, signature string) (GapRecord, error) {
	var refsRaw, family, disp, lastSeen string
	var count int64
	err := s.db.QueryRowContext(ctx,
		`SELECT task_refs, family, occurrence_count, disposition, last_seen_ts FROM gap_records WHERE signature = ?`,
		signature).Scan(&refsRaw, &family, &count, &disp, &lastSeen)
	if err == sql.ErrNoRows {
		return GapRecord{}, fmt.Errorf("%w: gap record %q", ErrNotFound, signature)
	}
	if err != nil {
		return GapRecord{}, fmt.Errorf("worker: read gap record: %w", err)
	}
	var refs []string
	if err := json.Unmarshal([]byte(refsRaw), &refs); err != nil {
		return GapRecord{}, fmt.Errorf("worker: decode gap task refs: %w", err)
	}
	return GapRecord{Signature: signature, Family: family, TaskRefs: refs,
		Occurrences: count, LastSeenTS: lastSeen, Disposition: GapDisposition(disp)}, nil
}

// ListDomains lists the registered domains rows (Spec S08.1).
func (s *Store) ListDomains(ctx context.Context) ([]Domain, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT domain, verification_maturity, coalesce(rubric_ref, ''), updated_ts FROM domains ORDER BY domain`)
	if err != nil {
		return nil, fmt.Errorf("worker: list domains: %w", err)
	}
	defer rows.Close()
	var out []Domain
	for rows.Next() {
		var d Domain
		var m string
		if err := rows.Scan(&d.Name, &m, &d.RubricRef, &d.UpdatedTS); err != nil {
			return nil, fmt.Errorf("worker: scan domain: %w", err)
		}
		d.Maturity = Maturity(m)
		out = append(out, d)
	}
	return out, rows.Err()
}

// composeReference renders the platform-owned reference inputs (Spec S08.6:
// "the template schema, lint rules, and ceiling table") from the SAME
// authorities the battery enforces — schema from template.go, lint rules
// from lint.go's stations, ceilings from ceiling.go's v0 table — plus the
// registered domains a draft may name. Deterministic; no drift between what
// the composer is told and what validation checks.
func (s *Store) composeReference(ctx context.Context) (string, error) {
	personaMax, err := s.settings.Int(keyPersonaLinesMax)
	if err != nil {
		return "", fmt.Errorf("worker: read ⚙ %s: %w", keyPersonaLinesMax, err)
	}
	domains, err := s.ListDomains(ctx)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("TEMPLATE SCHEMA (markdown + YAML frontmatter; closed key set — unknown keys are findings):\n")
	tops := make([]string, 0, len(templateSchema))
	for k := range templateSchema {
		tops = append(tops, k)
	}
	sort.Strings(tops)
	for _, k := range tops {
		sub := templateSchema[k]
		if sub == nil {
			b.WriteString("  " + k + "\n")
			continue
		}
		subs := make([]string, 0, len(sub))
		for sk := range sub {
			subs = append(subs, sk)
		}
		sort.Strings(subs)
		b.WriteString("  " + k + ": {" + strings.Join(subs, ", ") + "}\n")
	}
	fmt.Fprintf(&b, "Registered domains (a draft names exactly one): %s\n", domainNames(domains))
	b.WriteString("Body: the prompt body — objective template, method, boundaries, output contract, escalation triggers.\n")

	b.WriteString("\nLINT RULES (station 1, deterministic):\n")
	b.WriteString("  - name, description, kind, domain are required; selectors are deterministic rule inputs.\n")
	b.WriteString("  - GUARDRAIL-CLASS FIELDS (tools granted, permissions, confinement, egress, budgets, gates, schedules) are a STRUCTURAL REJECT in any file — enforcement lives in control-plane rows only; the file may only REQUEST equipment (equipment: block).\n")
	fmt.Fprintf(&b, "  - persona: at most %d tone/behavior lines (⚙ %s) — persona is a tone lever only; specialization comes from curated tools, 2–3 concise skills, deterministically injected knowledge, and an output contract, in that order.\n", personaMax, keyPersonaLinesMax)
	b.WriteString("  - tool/skill references must resolve; unresolved knowledge topic keys warn.\n")
	b.WriteString("  - the body is screened for instruction-injection patterns (definitions are untrusted content).\n")
	b.WriteString("  - templates stay THIN in strong-pretraining domains: more template is not a better worker.\n")

	b.WriteString("\nTASK-CLASS CEILING TABLE (station 2 — requested grants above a ceiling become flagged approval lines):\n")
	for _, row := range V0CeilingTable {
		tools := strings.Join(row.Tools, ", ")
		if row.ConnectorVerbs {
			tools = "fixed connector verbs on the one named service"
		}
		fmt.Fprintf(&b, "  %s · %s: max tools [%s], max confinement %s, max egress %s\n",
			row.Domain, row.Family, tools, row.MaxClass, row.Egress)
	}
	fmt.Fprintf(&b, "  generic fallback (unmatched family): max tools [%s], max confinement %s, max egress %s\n",
		strings.Join(genericFallbackRow.Tools, ", "), genericFallbackRow.MaxClass, genericFallbackRow.Egress)
	return b.String(), nil
}

func domainNames(domains []Domain) string {
	names := make([]string, 0, len(domains))
	for _, d := range domains {
		names = append(names, fmt.Sprintf("%s (%s)", d.Name, d.Maturity))
	}
	if len(names) == 0 {
		return "(none registered)"
	}
	return strings.Join(names, ", ")
}
