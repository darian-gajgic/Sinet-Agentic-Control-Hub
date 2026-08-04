package ledger

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
)

// Fresh-context-per-stage assembly (Spec S05.3/S05.4): every pipeline stage
// starts a fresh engine session from a stage brief the control plane
// assembles by PURE LOOKUP over platform-owned facts — the task record, the
// stage from the approved plan, and registry-keyed sources. No agent-
// supplied identifier ever selects context: free text or engine output
// never keys a lookup (G2 D2.1); the firewall sits here, at the injection
// layer. Crash recovery and normal stage start are the SAME code path
// (Spec S05.2): recovery pins LedgerVersion to the checkpointed revision.

// EventContextManifest is the trace-manifest event type (Spec S05.4: every
// injected item is logged to the run trace; one manifest per assembly plus
// entries for any mid-stage injection). Name provisional pending the S14
// event contract (B5), extending the CONVENTIONS §7/§8 naming note.
const EventContextManifest = "knowledge.injected"

// contextManifestSchemaVersion versions the context.manifest payload.
const contextManifestSchemaVersion = 1

// Precedence labels the authority of an injected block. 8.9's precedence
// (task spec > project > personal > house) is expressed by these explicit
// labels on each block, never by ordering (Spec S05.4); the label enum also
// names the stability-sort buckets of the assembly frame.
type Precedence string

const (
	PrecedenceHouse   Precedence = "house"
	PrecedenceProject Precedence = "project"
	PrecedenceUser    Precedence = "user"
	PrecedenceTask    Precedence = "task"
	PrecedenceStage   Precedence = "stage"
)

// precedenceOrder is the stability-sorted assembly order of Spec S05.4:
// house/static → project → user overlay → task ledger → stage brief —
// serving cache-prefix stability and a deterministic frame.
var precedenceOrder = map[Precedence]int{
	PrecedenceHouse: 0, PrecedenceProject: 1, PrecedenceUser: 2,
	PrecedenceTask: 3, PrecedenceStage: 4,
}

// DispositionOverBudgetDropped marks an item a source selected but the
// per-scope injection budget excluded (Spec S09.8): the item is recorded
// in the trace manifest — silent truncation is banned — and injected
// nowhere.
const DispositionOverBudgetDropped = "over_budget_dropped"

// Item is one injectable context item. Sources return items with their
// selection provenance filled; the assembly computes the content hash.
type Item struct {
	ItemID       string
	SourcePath   string
	Content      string
	Version      string
	SelectorRule string
	Precedence   Precedence

	// Disposition non-empty makes the item manifest-only: it appears in
	// the trace manifest under that marker and never becomes a brief block
	// (Spec S09.8 over_budget_dropped).
	Disposition string

	// ConflictsWith lists item ids in the same frame this item holds an
	// open conflict edge with (Spec S09.7: a known-conflicting pair
	// entering one frame is flagged in the trace manifest).
	ConflictsWith []string
}

// Block is one assembled brief block: the item plus its pinned marker
// (pinned blocks are the post-compaction re-injection set, Spec S05.7).
type Block struct {
	Item
	Pinned bool
}

// ManifestEntry is the S05.4 trace-manifest schema, Sinet-owned:
// {item_id, source_path, content_hash, version, selector_rule,
// precedence_label} — extended with the S09.8 disposition marker and the
// S09.7 conflict-pair flag (both empty on plain injected entries).
type ManifestEntry struct {
	ItemID          string `json:"item_id"`
	SourcePath      string `json:"source_path"`
	ContentHash     string `json:"content_hash"`
	Version         string `json:"version"`
	SelectorRule    string `json:"selector_rule"`
	PrecedenceLabel string `json:"precedence_label"`

	// Disposition records why a manifested item was not injected
	// (over_budget_dropped, Spec S09.8).
	Disposition string `json:"disposition,omitempty"`
	// ConflictsWith flags a known-conflicting pair entering one frame
	// (Spec S09.7).
	ConflictsWith []string `json:"conflicts_with,omitempty"`
}

// FindingKind classifies assembly findings.
type FindingKind string

// FindingShimDrift is the S05.5 shim-drift check result: the project's
// CLAUDE.md diverged from the canonical one-line import shim. A platform-
// detected defect (P-T04-2) — surfaced on the result and in the manifest
// event, never a silent condition; carding is later machinery (S14/S15).
const FindingShimDrift FindingKind = "shim_drift"

// Finding is one non-fatal assembly finding.
type Finding struct {
	Kind   FindingKind `json:"kind"`
	ItemID string      `json:"item_id"`
	Detail string      `json:"detail"`
}

// CanonicalShim is the canonical CLAUDE.md content: a one-line import shim
// for the Claude lane, AGENTS.md being the canonical convention file per
// registered repo (Spec S05.5, G2 D2.2). Compared trimmed (hash over the
// line, tolerant of trailing newline).
const CanonicalShim = "@AGENTS.md"

// SliceQuery carries the platform-owned facts a Source may select on —
// registry keys and stage identity only, per the S05.4 deterministic-
// selection rule. RunID is the assembling run's identity, resolved by
// Assemble itself (never caller/agent-supplied): sources use it for their
// own bookkeeping acts (e.g. the S09.8 curation card), not as a selection
// key.
type SliceQuery struct {
	RunID  string
	TaskID string
	Owner  string
	Stage  string
	Clean  bool
}

// Source supplies context items for a stage brief. Implementations are the
// owning sections' machinery; each item must carry its selector rule so the
// manifest stays audit-complete.
type Source interface {
	Items(ctx context.Context, q SliceQuery) ([]Item, error)
}

// Sources are the named assembly seams. All are optional (nil contributes
// nothing); each is owned by an out-of-scope section and stubbed until its
// packet lands.
type Sources struct {
	// Knowledge is Spec S09's (B3): knowledge slices per scope within
	// per-scope injection budgets (G2 Def.7).
	Knowledge Source
	// Conventions is the S05.5 registry path: AGENTS.md + the CLAUDE.md
	// shim + repo conventions from the registered project (registry is
	// S1.6/S06 machinery).
	Conventions Source
	// Worker is Spec S08's (B3): compiled worker equipment
	// (template → overlay → instance).
	Worker Source
	// Plan is Spec S06's (B2-2): stage instructions from the approved plan.
	Plan Source
}

// AssembleInput names one stage-brief assembly.
type AssembleInput struct {
	RunID string
	Stage string

	// Clean applies the S05.4 clean-context exception (verification
	// stages): the ledger projection reduces to objective_ac, and
	// user-overlay items and learned_this_task are structurally excluded —
	// the verifier receives the acceptance criteria and the deliverable
	// diff, never the executor's overlay, history, or learned notes. What
	// else a verification stage receives is Spec S07's (B2-3), passed via
	// Extra.
	Clean bool

	// LedgerVersion pins the projection to an exact revision — the recovery
	// path (Spec S05.2: fork from the checkpointed ledger uses this same
	// assembly). 0 = current.
	LedgerVersion int64

	Sources Sources

	// Extra items are caller-injected stage inputs (e.g. the S07
	// deliverable diff), subject to the same manifest and clean-mode rules.
	Extra []Item
}

// Brief is one assembled stage brief: the deterministic, stability-sorted
// block frame a fresh engine session starts from, with its trace manifest.
type Brief struct {
	RunID         string
	TaskID        string
	Stage         string
	Clean         bool
	LedgerVersion int64
	Blocks        []Block
	Manifest      []ManifestEntry
	// ManifestEventSeq is the run_events seq of this assembly's manifest
	// event.
	ManifestEventSeq int64
	Findings         []Finding
}

// manifestPayload is the context.manifest event payload.
type manifestPayload struct {
	Kind          string          `json:"kind"` // assembly | reinjection | recitation
	Stage         string          `json:"stage"`
	TaskID        string          `json:"task_id"`
	LedgerVersion int64           `json:"ledger_version"`
	Clean         bool            `json:"clean,omitempty"`
	Source        string          `json:"source,omitempty"`      // reinjection: SessionStart source
	SessionID     string          `json:"session_id,omitempty"`  // reinjection/recitation: engine session
	ToolUseID     string          `json:"tool_use_id,omitempty"` // recitation: the delivering tool boundary
	Items         []ManifestEntry `json:"items"`
	Findings      []Finding       `json:"findings,omitempty"`
}

// Assemble builds the stage brief for one run's next stage and appends its
// trace-manifest event. The ledger projection — pinned sections verbatim
// and whole, current decisions/state/artifacts view, learned_this_task for
// this task only — always comes from this run's task ledger, which is what
// makes the §6 never-into-another-task firewall structural.
//
// Source items are collected BEFORE the write transaction: the pool is one
// connection (Spec S02.1), so foreign code doing its own DB reads (e.g.
// the S06 Plan source) inside an open WriteTx would self-deadlock — the
// inverse of the never-nest rule intake already observes toward this
// package (CONVENTIONS §14). The authoritative doc read, the projection,
// and the manifest append stay one atomic transaction; the manifest hashes
// exactly the content injected.
func (s *Store) Assemble(ctx context.Context, in AssembleInput) (Brief, error) {
	if in.Stage == "" {
		return Brief{}, fmt.Errorf("%w: assembly without stage", ErrInvalidWrite)
	}
	// Pre-transaction: resolve the query facts and collect source items
	// (pure lookup over platform-owned facts, Spec S05.4).
	pre, err := readRun(ctx, s.db, in.RunID)
	if err != nil {
		return Brief{}, err
	}
	preDoc, _, found, err := currentDoc(ctx, s.db, pre.taskID)
	if err != nil {
		return Brief{}, err
	}
	if !found {
		return Brief{}, fmt.Errorf("%w: task %q", ErrNoLedger, pre.taskID)
	}
	sourceItems, err := collectSourceItems(ctx, in, SliceQuery{
		RunID: in.RunID, TaskID: preDoc.TaskID, Owner: preDoc.Owner,
		Stage: in.Stage, Clean: in.Clean,
	})
	if err != nil {
		return Brief{}, err
	}

	var brief Brief
	err = s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		r, err := readRun(ctx, tx, in.RunID)
		if err != nil {
			return err
		}
		doc, _, found, err := currentDoc(ctx, tx, r.taskID)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("%w: task %q", ErrNoLedger, r.taskID)
		}
		if in.LedgerVersion != 0 && in.LedgerVersion != doc.LedgerVersion {
			doc, err = s.atVersionTx(ctx, tx, r.taskID, in.LedgerVersion)
			if err != nil {
				return err
			}
		}

		blocks, manifestOnly, err := assembleBlocks(sourceItems, in, doc)
		if err != nil {
			return err
		}
		findings := checkShimDrift(blocks)

		// Stability sort: bucket order only; within a bucket the collection
		// order is already deterministic (ledger section order, then source
		// order, then Extra).
		sort.SliceStable(blocks, func(i, j int) bool {
			return precedenceOrder[blocks[i].Precedence] < precedenceOrder[blocks[j].Precedence]
		})

		// Injected entries first, then manifest-only entries (dropped items,
		// Spec S09.8) in collection order — recorded, never injected.
		manifest := make([]ManifestEntry, 0, len(blocks)+len(manifestOnly))
		for _, b := range blocks {
			manifest = append(manifest, entryFor(b.Item))
		}
		for _, it := range manifestOnly {
			manifest = append(manifest, entryFor(it))
		}
		payload, err := json.Marshal(manifestPayload{
			Kind: "assembly", Stage: in.Stage, TaskID: doc.TaskID,
			LedgerVersion: doc.LedgerVersion, Clean: in.Clean,
			Items: manifest, Findings: findings,
		})
		if err != nil {
			return fmt.Errorf("ledger: marshal manifest: %w", err)
		}
		seq, err := s.log.AppendTx(ctx, tx, eventlog.Append{
			RunID:         in.RunID,
			Generation:    r.generation,
			UserID:        r.userID,
			Type:          EventContextManifest,
			SchemaVersion: contextManifestSchemaVersion,
			Payload:       payload,
		})
		if err != nil {
			return err
		}
		brief = Brief{
			RunID: in.RunID, TaskID: doc.TaskID, Stage: in.Stage, Clean: in.Clean,
			LedgerVersion: doc.LedgerVersion, Blocks: blocks,
			Manifest: manifest, ManifestEventSeq: seq, Findings: findings,
		}
		return nil
	})
	if err != nil {
		return Brief{}, err
	}
	return brief, nil
}

// atVersionTx is AtVersion inside the assembly transaction.
func (s *Store) atVersionTx(ctx context.Context, tx *sql.Tx, taskID string, version int64) (Document, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT e.payload FROM run_events e
		  JOIN runs r ON e.run_id = r.run_id
		 WHERE r.task_id = ? AND e.type = '`+EventLedgerUpdate+`'
		 ORDER BY e.event_seq`, taskID)
	if err != nil {
		return Document{}, fmt.Errorf("ledger: read revisions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return Document{}, fmt.Errorf("ledger: scan revision: %w", err)
		}
		var up updatePayload
		if err := json.Unmarshal([]byte(payload), &up); err != nil {
			return Document{}, fmt.Errorf("ledger: decode ledger_update payload: %w", err)
		}
		if up.Ledger.LedgerVersion == version {
			return up.Ledger, nil
		}
	}
	if err := rows.Err(); err != nil {
		return Document{}, err
	}
	return Document{}, fmt.Errorf("%w: task %q version %d", ErrVersionNotFound, taskID, version)
}

// collectSourceItems gathers the source items in deterministic collection
// order (Knowledge, Conventions, Worker, Plan). Runs OUTSIDE the assembly
// transaction — sources may do their own DB reads (see Assemble).
func collectSourceItems(ctx context.Context, in AssembleInput, q SliceQuery) ([]Item, error) {
	var items []Item
	for _, src := range []struct {
		name string
		s    Source
	}{
		{"knowledge", in.Sources.Knowledge},
		{"conventions", in.Sources.Conventions},
		{"worker", in.Sources.Worker},
		{"plan", in.Sources.Plan},
	} {
		if src.s == nil {
			continue
		}
		got, err := src.s.Items(ctx, q)
		if err != nil {
			return nil, fmt.Errorf("ledger: %s source: %w", src.name, err)
		}
		items = append(items, got...)
	}
	return items, nil
}

// assembleBlocks builds the frame from the pre-collected source items, the
// in-transaction ledger projection (task bucket), then Extra. The second
// return carries the manifest-only items (non-empty Disposition): recorded
// in the trace manifest, injected nowhere (Spec S09.8).
func assembleBlocks(sourceItems []Item, in AssembleInput, doc Document) ([]Block, []Item, error) {
	var blocks []Block
	var manifestOnly []Item
	add := func(items []Item) error {
		for _, it := range items {
			b, err := blockFor(it, in.Clean)
			if err != nil {
				return err
			}
			switch {
			case b == nil && it.Disposition != "" && !(in.Clean && it.Precedence == PrecedenceUser):
				manifestOnly = append(manifestOnly, it)
			case b != nil:
				blocks = append(blocks, *b)
			}
		}
		return nil
	}
	if err := add(sourceItems); err != nil {
		return nil, nil, err
	}
	blocks = append(blocks, projectLedger(doc, in.Clean)...)
	if err := add(in.Extra); err != nil {
		return nil, nil, err
	}
	return blocks, manifestOnly, nil
}

// blockFor validates one source item and applies the clean-mode firewall:
// user-overlay items never reach a verification brief (Spec S05.4
// clean-context exception). A nil, nil return means the item yields no
// block (clean-mode drop, or manifest-only disposition).
func blockFor(it Item, clean bool) (*Block, error) {
	if it.ItemID == "" || it.SelectorRule == "" {
		return nil, fmt.Errorf("%w: injected item requires item_id and selector_rule (S05.4 manifest)", ErrInvalidWrite)
	}
	if _, ok := precedenceOrder[it.Precedence]; !ok {
		return nil, fmt.Errorf("%w: item %q precedence %q", ErrInvalidWrite, it.ItemID, it.Precedence)
	}
	if clean && it.Precedence == PrecedenceUser {
		return nil, nil
	}
	if it.Disposition != "" {
		return nil, nil
	}
	return &Block{Item: it}, nil
}

// Ledger projection identities (stable across assemblies; the revision
// rides the manifest entry's version field).
const (
	itemObjectiveAC = "ledger/objective_ac"
	itemConstraints = "ledger/constraints_danger_zones"
	itemDecisions   = "ledger/decisions"
	itemState       = "ledger/state"
	itemArtifacts   = "ledger/artifacts"
	itemLearned     = "ledger/learned_this_task"

	selectorLedger = "run.task_id -> ledger projection (S05.4 pure lookup)"
	selectorPinned = "run.task_id -> pinned section, verbatim and whole (S05.1)"
)

// projectLedger renders the ledger projection blocks (Spec S05.4): pinned
// sections verbatim and whole ALWAYS; the current decisions/state/artifacts
// view and learned_this_task when non-empty; clean mode reduces to
// objective_ac.
func projectLedger(doc Document, clean bool) []Block {
	ledgerItem := func(id, selector string, content string, pinned bool) Block {
		return Block{
			Item: Item{
				ItemID:       id,
				SourcePath:   "platform.db:run_events/" + EventLedgerUpdate + "/" + doc.TaskID,
				Content:      content,
				Version:      strconv.FormatInt(doc.LedgerVersion, 10),
				SelectorRule: selector,
				Precedence:   PrecedenceTask,
			},
			Pinned: pinned,
		}
	}
	blocks := []Block{
		ledgerItem(itemObjectiveAC, selectorPinned, renderSection(doc.ObjectiveAC), true),
	}
	if clean {
		return blocks
	}
	blocks = append(blocks, ledgerItem(itemConstraints, selectorPinned, renderSection(doc.Constraints), true))
	if len(doc.Decisions) > 0 {
		blocks = append(blocks, ledgerItem(itemDecisions, selectorLedger, renderSection(doc.Decisions), false))
	}
	blocks = append(blocks, ledgerItem(itemState, selectorLedger, renderSection(doc.State), false))
	if len(doc.Artifacts) > 0 {
		blocks = append(blocks, ledgerItem(itemArtifacts, selectorLedger, renderSection(doc.Artifacts), false))
	}
	if len(doc.LearnedThisTask) > 0 {
		blocks = append(blocks, ledgerItem(itemLearned, selectorLedger, renderSection(doc.LearnedThisTask), false))
	}
	return blocks
}

// renderSection serializes one ledger section for injection: the stored
// content whole, in a stable machine-faithful form — nothing is summarized
// or paraphrased (Spec S05.1 pinned rule; the same renderer serves pinned
// and non-pinned blocks so re-injection is byte-verbatim).
func renderSection(v any) string {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		// Sections are plain structs; marshal cannot fail on them.
		return fmt.Sprintf("<render error: %v>", err)
	}
	return string(raw) + "\n"
}

func entryFor(it Item) ManifestEntry {
	sum := sha256.Sum256([]byte(it.Content))
	return ManifestEntry{
		ItemID:          it.ItemID,
		SourcePath:      it.SourcePath,
		ContentHash:     hex.EncodeToString(sum[:]),
		Version:         it.Version,
		SelectorRule:    it.SelectorRule,
		PrecedenceLabel: string(it.Precedence),
		Disposition:     it.Disposition,
		ConflictsWith:   it.ConflictsWith,
	}
}

// checkShimDrift runs the S05.5 shim-drift check at every assembly: any
// injected CLAUDE.md must be the canonical one-line import shim.
func checkShimDrift(blocks []Block) []Finding {
	var findings []Finding
	for _, b := range blocks {
		if filepath.Base(b.SourcePath) != "CLAUDE.md" {
			continue
		}
		if strings.TrimSpace(b.Content) == CanonicalShim {
			continue
		}
		findings = append(findings, Finding{
			Kind:   FindingShimDrift,
			ItemID: b.ItemID,
			Detail: "CLAUDE.md diverged from the canonical one-line shim " + CanonicalShim + " (S05.5, P-T04-2)",
		})
	}
	return findings
}

// ---- Injection materialization (Spec S05.4 channels; B1-4 spike) ----

// The Claude-lane per-stage channel, resolved by the B1-4 spike
// (P3/measurements/2026-07-20-precompact-injection-mechanics.md, M4):
// stage-brief body via prompt assembly, pinned-section survival across
// compaction via a SessionStart hook (matched on source
// startup|resume|compact) emitting additionalContext — wired through the
// lowered settings exactly like the S03.4 gate hook. BriefText and
// PlacePinned produce those two materializations; the hook runner and
// settings compilation are the adapter's (internal/adapters/claudecli).

// PinnedContextFile is the platform-placed file name the SessionStart
// re-injection hook reads (placed in the run's platform-owned WorkDir,
// read-only under confinement).
const PinnedContextFile = "pinned-context.md"

// BriefText renders the brief for prompt assembly: the stability-sorted
// block frame, each block headed by its explicit precedence label and item
// identity (8.9 by labels, S05.4). Deterministic: same brief, same bytes.
func BriefText(b Brief) string {
	var sb strings.Builder
	for _, blk := range b.Blocks {
		writeBlock(&sb, blk)
	}
	return sb.String()
}

// PinnedText renders exactly the pinned blocks of a brief — the verbatim
// post-compaction re-injection body (Spec S05.7 step 3). The bytes equal
// the same blocks inside BriefText; verbatim is testable, not asserted.
func PinnedText(b Brief) string {
	var sb strings.Builder
	for _, blk := range b.Blocks {
		if blk.Pinned {
			writeBlock(&sb, blk)
		}
	}
	return sb.String()
}

func writeBlock(sb *strings.Builder, blk Block) {
	fmt.Fprintf(sb, "=== [%s] %s v%s ===\n", blk.Precedence, blk.ItemID, blk.Version)
	sb.WriteString(blk.Content)
	if !strings.HasSuffix(blk.Content, "\n") {
		sb.WriteByte('\n')
	}
	sb.WriteByte('\n')
}

// PlacePinned writes the brief's pinned re-injection body to
// dir/pinned-context.md and returns its path and content hash. The
// placement is a platform act over already-manifested content; the
// SessionStart hook re-emits the file verbatim.
func PlacePinned(dir string, b Brief) (path, sha256hex string, err error) {
	content := PinnedText(b)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", fmt.Errorf("ledger: place pinned context: %w", err)
	}
	path = filepath.Join(dir, PinnedContextFile)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", "", fmt.Errorf("ledger: place pinned context: %w", err)
	}
	sum := sha256.Sum256([]byte(content))
	return path, hex.EncodeToString(sum[:]), nil
}

// AppendReinjectionManifest records a mid-stage pinned re-injection in the
// run trace (Spec S05.4 "plus entries for any mid-stage injection,
// including post-compaction re-injection"; S05.7 step 3). source is the
// SessionStart source that fired (startup|resume|compact), sessionID the
// engine session; clean mirrors the stage brief's mode (a clean stage's
// pinned set is objective_ac only). The entries are recomputed from the
// ledger revision the stage was assembled from, so the record is
// self-contained.
func (s *Store) AppendReinjectionManifest(ctx context.Context, runID, stage string, ledgerVersion int64, clean bool, source, sessionID string) (int64, error) {
	var seq int64
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		r, err := readRun(ctx, tx, runID)
		if err != nil {
			return err
		}
		doc, _, found, err := currentDoc(ctx, tx, r.taskID)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("%w: task %q", ErrNoLedger, r.taskID)
		}
		if ledgerVersion != 0 && ledgerVersion != doc.LedgerVersion {
			doc, err = s.atVersionTx(ctx, tx, r.taskID, ledgerVersion)
			if err != nil {
				return err
			}
		}
		var items []ManifestEntry
		for _, b := range projectLedger(doc, clean) {
			if b.Pinned {
				items = append(items, entryFor(b.Item))
			}
		}
		payload, err := json.Marshal(manifestPayload{
			Kind: "reinjection", Stage: stage, TaskID: doc.TaskID,
			LedgerVersion: doc.LedgerVersion, Source: source, SessionID: sessionID,
			Items: items,
		})
		if err != nil {
			return fmt.Errorf("ledger: marshal reinjection manifest: %w", err)
		}
		seq, err = s.log.AppendTx(ctx, tx, eventlog.Append{
			RunID:         runID,
			Generation:    r.generation,
			UserID:        r.userID,
			Type:          EventContextManifest,
			SchemaVersion: contextManifestSchemaVersion,
			Payload:       payload,
		})
		return err
	})
	if err != nil {
		return 0, err
	}
	return seq, nil
}
