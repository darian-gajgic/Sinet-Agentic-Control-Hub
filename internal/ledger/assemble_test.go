package ledger_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
)

// Fresh-context-per-stage acceptance: the stage brief materializes from
// the ledger by pure lookup (S05.4), stability-sorted with explicit
// precedence labels, every injected item trace-manifested; recovery
// (fork-from-checkpoint) rides the same assembly at a pinned revision
// (S05.2); the clean-context exception holds structurally.

type fakeSource struct {
	items []ledger.Item
	got   []ledger.SliceQuery
}

func (s *fakeSource) Items(_ context.Context, q ledger.SliceQuery) ([]ledger.Item, error) {
	s.got = append(s.got, q)
	return s.items, nil
}

func item(id, path, content, version, rule string, p ledger.Precedence) ledger.Item {
	return ledger.Item{ItemID: id, SourcePath: path, Content: content, Version: version, SelectorRule: rule, Precedence: p}
}

func fullSources() ledger.Sources {
	return ledger.Sources{
		Knowledge: &fakeSource{items: []ledger.Item{
			item("house/style", "house/style.md", "house style rules\n", "1", "static house set", ledger.PrecedenceHouse),
		}},
		Conventions: &fakeSource{items: []ledger.Item{
			item("proj/AGENTS.md", "repo/AGENTS.md", "build: go build ./...\n", "sha-1", "registry: project key", ledger.PrecedenceProject),
			item("proj/CLAUDE.md", "repo/CLAUDE.md", "@AGENTS.md\n", "sha-2", "registry: project key", ledger.PrecedenceProject),
		}},
		Worker: &fakeSource{items: []ledger.Item{
			item("worker/overlay", "workers/w1/overlay.md", "personal overlay\n", "3", "assigned worker template+overlay", ledger.PrecedenceUser),
		}},
		Plan: &fakeSource{items: []ledger.Item{
			item("plan/stage", "plan/p1#execute-1", "stage instructions\n", "plan-v1", "approved plan: stage row", ledger.PrecedenceStage),
		}},
	}
}

func manifestEvents(t *testing.T, f *fix) []eventlog.Event {
	t.Helper()
	events, err := f.log.After(context.Background(), 0, 1000)
	if err != nil {
		t.Fatalf("After: %v", err)
	}
	var out []eventlog.Event
	for _, e := range events {
		if e.Type == ledger.EventContextManifest {
			out = append(out, e)
		}
	}
	return out
}

func TestAssembleFullBrief(t *testing.T) {
	f := newFix(t)
	f.task(t, "t1", "u1")
	r := f.run(t, "r1", "t1")
	ctx := context.Background()
	seedLedger(t, f, r.ID, r.Generation)

	brief, err := f.store.Assemble(ctx, ledger.AssembleInput{
		RunID: r.ID, Stage: "execute-1", Sources: fullSources(),
	})
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	if brief.TaskID != "t1" || brief.LedgerVersion != 10 {
		t.Fatalf("brief header = %q v%d", brief.TaskID, brief.LedgerVersion)
	}

	// Stability-sorted frame: house → project → user → task ledger → stage
	// (S05.4), ledger sections in S05.1 order inside the task bucket.
	var ids []string
	for _, b := range brief.Blocks {
		ids = append(ids, b.ItemID)
	}
	want := []string{
		"house/style",
		"proj/AGENTS.md", "proj/CLAUDE.md",
		"worker/overlay",
		"ledger/objective_ac", "ledger/constraints_danger_zones",
		"ledger/decisions", "ledger/state", "ledger/artifacts", "ledger/learned_this_task",
		"plan/stage",
	}
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Fatalf("block order:\n got %v\nwant %v", ids, want)
	}
	if !brief.Blocks[4].Pinned || !brief.Blocks[5].Pinned || brief.Blocks[6].Pinned {
		t.Fatalf("pinned markers wrong: %+v", brief.Blocks[4:7])
	}

	// One manifest per assembly, every injected item entried with hash +
	// selector + precedence label (S05.4).
	if len(brief.Manifest) != len(brief.Blocks) {
		t.Fatalf("manifest %d entries for %d blocks", len(brief.Manifest), len(brief.Blocks))
	}
	for i, e := range brief.Manifest {
		sum := sha256.Sum256([]byte(brief.Blocks[i].Content))
		if e.ContentHash != hex.EncodeToString(sum[:]) {
			t.Fatalf("entry %s hash mismatch", e.ItemID)
		}
		if e.SelectorRule == "" || e.PrecedenceLabel == "" || e.SourcePath == "" {
			t.Fatalf("incomplete manifest entry: %+v", e)
		}
	}
	if brief.Manifest[4].PrecedenceLabel != "task" || brief.Manifest[10].PrecedenceLabel != "stage" {
		t.Fatalf("precedence labels = %+v", brief.Manifest)
	}

	// The manifest landed in the run trace.
	evs := manifestEvents(t, f)
	if len(evs) != 1 || evs[0].Seq != brief.ManifestEventSeq || evs[0].RunID != r.ID {
		t.Fatalf("manifest events = %+v (want one at seq %d)", evs, brief.ManifestEventSeq)
	}
	var payload struct {
		Kind          string                 `json:"kind"`
		Stage         string                 `json:"stage"`
		LedgerVersion int64                  `json:"ledger_version"`
		Items         []ledger.ManifestEntry `json:"items"`
	}
	if err := json.Unmarshal(evs[0].Payload, &payload); err != nil {
		t.Fatalf("manifest payload: %v", err)
	}
	if payload.Kind != "assembly" || payload.Stage != "execute-1" || payload.LedgerVersion != 10 || len(payload.Items) != len(brief.Blocks) {
		t.Fatalf("manifest payload = %+v", payload)
	}

	// Deterministic rendering with explicit precedence labels (8.9 by
	// labels): same brief, same bytes.
	text := ledger.BriefText(brief)
	if text != ledger.BriefText(brief) {
		t.Fatalf("BriefText not deterministic")
	}
	if !strings.Contains(text, "=== [task] ledger/objective_ac v10 ===") ||
		!strings.Contains(text, "=== [house] house/style v1 ===") ||
		!strings.Contains(text, "=== [stage] plan/stage vplan-v1 ===") {
		t.Fatalf("labels missing from rendering:\n%s", text)
	}
	if !strings.Contains(text, "Ship the demo") || !strings.Contains(text, "never push to main") {
		t.Fatalf("pinned content missing from rendering")
	}
	if len(brief.Findings) != 0 {
		t.Fatalf("unexpected findings: %+v", brief.Findings)
	}

	// The selection firewall: sources were queried with platform-owned
	// facts only (S05.4 — no agent-supplied identifiers; RunID is the
	// assembly's own resolution, for source-side bookkeeping acts).
	src := fullSources()
	if _, err := f.store.Assemble(ctx, ledger.AssembleInput{RunID: r.ID, Stage: "execute-2", Sources: src}); err != nil {
		t.Fatalf("Assemble 2: %v", err)
	}
	ks := src.Knowledge.(*fakeSource)
	wantQuery := ledger.SliceQuery{RunID: r.ID, TaskID: "t1", Owner: "u1", Stage: "execute-2"}
	if len(ks.got) != 1 || ks.got[0] != wantQuery {
		t.Fatalf("source query = %+v, want %+v", ks.got, wantQuery)
	}
}

func TestAssembleCleanContextException(t *testing.T) {
	f := newFix(t)
	f.task(t, "t1", "u1")
	r := f.run(t, "r1", "t1")
	ctx := context.Background()
	seedLedger(t, f, r.ID, r.Generation)

	diff := item("verify/diff", "workspace/diff.patch", "--- a\n+++ b\n", "1", "deliverable diff (S07)", ledger.PrecedenceStage)
	brief, err := f.store.Assemble(ctx, ledger.AssembleInput{
		RunID: r.ID, Stage: "verify", Clean: true, Sources: fullSources(), Extra: []ledger.Item{diff},
	})
	if err != nil {
		t.Fatalf("Assemble clean: %v", err)
	}
	var ids []string
	for _, b := range brief.Blocks {
		ids = append(ids, b.ItemID)
	}
	joined := strings.Join(ids, ",")
	// The verifier receives the acceptance criteria and the deliverable
	// diff — never the executor's overlay, history, or learned_this_task
	// (S05.4 clean-context exception).
	if !strings.Contains(joined, "ledger/objective_ac") || !strings.Contains(joined, "verify/diff") {
		t.Fatalf("clean brief missing AC/diff: %v", ids)
	}
	for _, banned := range []string{"worker/overlay", "ledger/decisions", "ledger/state", "ledger/artifacts", "ledger/learned_this_task", "ledger/constraints_danger_zones"} {
		if strings.Contains(joined, banned) {
			t.Fatalf("clean brief leaked %s: %v", banned, ids)
		}
	}
}

func TestAssembleRecoveryIsSameCodePath(t *testing.T) {
	f := newFix(t)
	f.task(t, "t1", "u1")
	r := f.run(t, "r1", "t1")
	ctx := context.Background()
	seedLedger(t, f, r.ID, r.Generation)

	// The checkpoint records the revision the stage was built from…
	ref, err := f.store.CheckpointRef(ctx, r.ID)
	if err != nil {
		t.Fatalf("CheckpointRef: %v", err)
	}
	parsed, _, err := ledger.ParseRevisionRef(ref)
	if err != nil {
		t.Fatalf("ParseRevisionRef: %v", err)
	}
	normal, err := f.store.Assemble(ctx, ledger.AssembleInput{RunID: r.ID, Stage: "execute-1", Sources: fullSources()})
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	// …the task moves on…
	if _, err := f.store.SessionVerbs(r.ID, "execute-1", r.Generation).Note(ctx, "later work"); err != nil {
		t.Fatalf("Note: %v", err)
	}

	// …and recovery forks a fresh session from the checkpointed ledger
	// through the SAME assembly (S05.2): identical brief at the pinned
	// revision, differing from a current-revision brief.
	recovered, err := f.store.Assemble(ctx, ledger.AssembleInput{
		RunID: r.ID, Stage: "execute-1", LedgerVersion: parsed.LedgerVersion, Sources: fullSources(),
	})
	if err != nil {
		t.Fatalf("Assemble at checkpoint revision: %v", err)
	}
	if ledger.BriefText(recovered) != ledger.BriefText(normal) {
		t.Fatalf("recovery brief diverges from the checkpointed stage brief")
	}
	current, err := f.store.Assemble(ctx, ledger.AssembleInput{RunID: r.ID, Stage: "execute-1", Sources: fullSources()})
	if err != nil {
		t.Fatalf("Assemble current: %v", err)
	}
	if ledger.BriefText(current) == ledger.BriefText(normal) {
		t.Fatalf("current brief should include the later revision")
	}
}

func TestShimDriftIsNeverSilent(t *testing.T) {
	f := newFix(t)
	f.task(t, "t1", "u1")
	r := f.run(t, "r1", "t1")
	ctx := context.Background()
	seedLedger(t, f, r.ID, r.Generation)

	src := ledger.Sources{Conventions: &fakeSource{items: []ledger.Item{
		item("proj/CLAUDE.md", "repo/CLAUDE.md", "# a whole config file\n", "sha-9", "registry: project key", ledger.PrecedenceProject),
	}}}
	brief, err := f.store.Assemble(ctx, ledger.AssembleInput{RunID: r.ID, Stage: "execute-1", Sources: src})
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	if len(brief.Findings) != 1 || brief.Findings[0].Kind != ledger.FindingShimDrift || brief.Findings[0].ItemID != "proj/CLAUDE.md" {
		t.Fatalf("findings = %+v, want one shim_drift", brief.Findings)
	}
	// The finding rides the manifest event — detected, recorded, never a
	// silent condition (S05.5, P-T04-2).
	evs := manifestEvents(t, f)
	var payload struct {
		Findings []ledger.Finding `json:"findings"`
	}
	if err := json.Unmarshal(evs[len(evs)-1].Payload, &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if len(payload.Findings) != 1 || payload.Findings[0].Kind != ledger.FindingShimDrift {
		t.Fatalf("event findings = %+v", payload.Findings)
	}
}

func TestPinnedPlacementIsVerbatim(t *testing.T) {
	f := newFix(t)
	f.task(t, "t1", "u1")
	r := f.run(t, "r1", "t1")
	ctx := context.Background()
	seedLedger(t, f, r.ID, r.Generation)

	brief, err := f.store.Assemble(ctx, ledger.AssembleInput{RunID: r.ID, Stage: "execute-1", Sources: fullSources()})
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	pinned := ledger.PinnedText(brief)
	if !strings.Contains(ledger.BriefText(brief), pinned) {
		t.Fatalf("pinned re-injection body is not byte-verbatim inside the brief")
	}
	dir := t.TempDir()
	path, hash, err := ledger.PlacePinned(dir, brief)
	if err != nil {
		t.Fatalf("PlacePinned: %v", err)
	}
	placed, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read placed: %v", err)
	}
	if string(placed) != pinned {
		t.Fatalf("placed file diverges from PinnedText")
	}
	sum := sha256.Sum256(placed)
	if hash != hex.EncodeToString(sum[:]) {
		t.Fatalf("placement hash mismatch")
	}

	// Mid-stage re-injection lands manifest entries for exactly the pinned
	// items, recomputed from the same revision (S05.4/S05.7 step 3).
	seq, err := f.store.AppendReinjectionManifest(ctx, r.ID, "execute-1", brief.LedgerVersion, false, "compact", "sess-1")
	if err != nil {
		t.Fatalf("AppendReinjectionManifest: %v", err)
	}
	evs := manifestEvents(t, f)
	last := evs[len(evs)-1]
	if last.Seq != seq {
		t.Fatalf("reinjection seq = %d, want %d", last.Seq, seq)
	}
	var payload struct {
		Kind      string                 `json:"kind"`
		Source    string                 `json:"source"`
		SessionID string                 `json:"session_id"`
		Items     []ledger.ManifestEntry `json:"items"`
	}
	if err := json.Unmarshal(last.Payload, &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if payload.Kind != "reinjection" || payload.Source != "compact" || payload.SessionID != "sess-1" {
		t.Fatalf("reinjection payload = %+v", payload)
	}
	if len(payload.Items) != 2 ||
		payload.Items[0].ContentHash != brief.Manifest[4].ContentHash ||
		payload.Items[1].ContentHash != brief.Manifest[5].ContentHash {
		t.Fatalf("reinjection entries diverge from the assembly's pinned entries: %+v", payload.Items)
	}
}

func TestAssembleValidation(t *testing.T) {
	f := newFix(t)
	f.task(t, "t1", "u1")
	r := f.run(t, "r1", "t1")
	ctx := context.Background()

	// No ledger yet: a stage brief has nothing to build from.
	if _, err := f.store.Assemble(ctx, ledger.AssembleInput{RunID: r.ID, Stage: "s"}); !errors.Is(err, ledger.ErrNoLedger) {
		t.Fatalf("no-ledger err = %v", err)
	}
	seedLedger(t, f, r.ID, r.Generation)
	// Manifest completeness is mandatory: an item without selector
	// provenance cannot be injected.
	bad := ledger.Sources{Plan: &fakeSource{items: []ledger.Item{{ItemID: "x", Content: "y", Precedence: ledger.PrecedenceStage}}}}
	if _, err := f.store.Assemble(ctx, ledger.AssembleInput{RunID: r.ID, Stage: "s", Sources: bad}); !errors.Is(err, ledger.ErrInvalidWrite) {
		t.Fatalf("selectorless item err = %v", err)
	}
	if _, err := f.store.Assemble(ctx, ledger.AssembleInput{RunID: r.ID, Stage: "s", LedgerVersion: 77}); !errors.Is(err, ledger.ErrVersionNotFound) {
		t.Fatalf("pinned missing revision err = %v", err)
	}
}
