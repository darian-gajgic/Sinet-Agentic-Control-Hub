package stage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
)

// The platform-side reciter (Spec S05.3; Research/18 §7-C1, operator
// standing directive 2026-07-20): in long stages the platform re-reads the
// ledger's state/next_actions every ⚙ context.recitation_interval_turns
// and delivers the re-read into the running session at the next tool
// boundary. The split of duties is the §7-C1 shape exactly:
//
//   - The PLATFORM (this file) computes turns-dueness from run events —
//     one turn per persisted per-call usage event, the same OnEvent seam
//     the budget watcher rides (observers fire only after the Driver's
//     durable write, so every counted turn is a run_events fact) — and
//     AUTHORS the recitation: ledger state/next_actions rendered by pure
//     lookup at a pinned ledger_version (ledger.RecitationText), written
//     to the ctl-dir pending file via temp-file + atomic rename.
//   - The engine-side hook (claudecli.RunPostToolUseHook, compiled by the
//     lowering when CompiledWorker.Recitation is set) is a delivery valve
//     ONLY: it read-and-consumes the pending file at a tool boundary and
//     holds no cadence state. A turn without tool calls simply delivers at
//     the next boundary; a re-authoring before delivery atomically
//     replaces the pending content (newest platform truth wins).
//   - Every delivery is manifested + journaled: the valve's fires log is
//     read back after the session (the CONVENTIONS §13 fires→manifest
//     pattern, mirrored shapes — stage never imports the substrate
//     package) and each fire becomes a context.manifest run event
//     (ledger.AppendRecitationManifest), hash-verified against the
//     platform re-rendering at the pinned revision.
//
// Recitation applies to ledger-backed working sessions only: an
// artifact-only session (critique, rework — input contracts exclude the
// ledger projection) and a clean-context session (verification, Spec
// S05.4 exception) have no business receiving the executor's state; a
// tool-less session has no delivery boundary. ⚙ 0 turns recitation off.

// reciteSubdir/recitePending/reciteFiresFile mirror the adapter's ctl-dir
// recitation layout (the airlock file contract; byte-level equivalence is
// pinned by the stage↔claudecli contract test).
const (
	reciteSubdir    = "recite"
	recitePending   = "pending.json"
	reciteFiresFile = "recite.fires.log"
)

// pendingRecitation mirrors claudecli.PendingRecitation (the pending-file
// payload the delivery valve consumes).
type pendingRecitation struct {
	LedgerVersion int64  `json:"ledger_version"`
	Content       string `json:"content"`
	ContentSHA256 string `json:"content_sha256"`
	WrittenAt     string `json:"written_at"`
}

// reciteFire mirrors claudecli.ReciteFire (one recorded delivery).
type reciteFire struct {
	TS            string `json:"ts"`
	SessionID     string `json:"session_id"`
	ToolUseID     string `json:"tool_use_id"`
	LedgerVersion int64  `json:"ledger_version"`
	ContentSHA256 string `json:"content_sha256"`
}

// reciter authors due recitations for one stage session.
type reciter struct {
	s        *Skeleton
	ctx      context.Context
	runID    string
	taskID   string
	stage    string
	ctlDir   string
	interval int64
	turns    int64 // turns observed since the last authoring
}

// newReciter decides whether this session recites and builds its reciter.
// Nil (recitation off) when: the ⚙ interval is 0; the session is not a
// ledger-backed working session (no assembled brief, or clean mode); or
// the compiled toolset is empty (no tool boundary can ever deliver). A ⚙
// read failure logs loudly and disables — the budget-watcher misread
// posture: never invent a number (CONVENTIONS §2).
func (s *Skeleton) newReciter(ctx context.Context, in SessionInput, brief ledger.Brief, workDir string, toolAllowlist []string) *reciter {
	if !in.Assemble || in.Clean || brief.TaskID == "" || len(toolAllowlist) == 0 {
		return nil
	}
	interval, err := s.cfg.Settings.Int(keyRecitationInterval)
	if err != nil {
		s.logger().Error("stage: read ⚙ "+keyRecitationInterval, "err", err)
		return nil
	}
	if interval <= 0 {
		return nil // ⚙ 0 = recitation off (S18 clamp: 0, or 5..50)
	}
	return &reciter{
		s: s, ctx: ctx, runID: in.RunID, taskID: brief.TaskID, stage: in.Stage,
		ctlDir: filepath.Join(workDir, "gate-ctl"), interval: interval,
	}
}

// observe consumes the session's persisted contract events (the
// StartRequest.OnEvent seam): each per-call usage event is one turn — one
// paid assistant message, the run-event granularity D7 checkpoints ride —
// and every ⚙ interval-th turn makes a recitation due. Authoring failures
// are loud and non-fatal: recitation is an in-stage aid and must never
// kill a paid session, but a silent skip is banned (S14.3 posture).
func (r *reciter) observe(ev adapters.Event) {
	if ev.Kind != adapters.KindUsage || ev.Usage == nil || ev.Usage.Total {
		return
	}
	r.turns++
	if r.turns < r.interval {
		return
	}
	r.turns = 0
	if err := r.author(); err != nil {
		r.s.logger().Error("stage: author recitation", "run", r.runID, "stage", r.stage, "err", err)
	}
}

// author writes the due recitation: the run's task ledger CURRENT document
// (pure lookup — the recitation IS a re-read of the evolving state), the
// rendering pinned to that document's ledger_version, into the ctl-dir
// pending file by temp-file + atomic rename (single-filesystem, so a
// concurrent valve claim sees either the old or the new pending, never a
// torn read; an undelivered older pending is atomically superseded).
func (r *reciter) author() error {
	doc, found, err := r.s.cfg.Ledger.Current(r.ctx, r.taskID)
	if err != nil {
		return err
	}
	if !found {
		return ledger.ErrNoLedger
	}
	content := ledger.RecitationText(doc)
	sum := sha256.Sum256([]byte(content))
	raw, err := json.Marshal(pendingRecitation{
		LedgerVersion: doc.LedgerVersion,
		Content:       content,
		ContentSHA256: hex.EncodeToString(sum[:]),
		WrittenAt:     r.s.now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return err
	}
	dir := filepath.Join(r.ctlDir, reciteSubdir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".pending-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, recitePending))
}

// recordRecitations turns the delivery valve's fires into recitation
// manifest events (Spec S05.4 mid-stage entries; §7-C1 condition 3 —
// every delivery manifested and journaled, for EVERY session disposition:
// a delivery into a session that later failed still happened). Fires are
// read from the ctl-dir exchange the hook appended; absence is the normal
// quiet path. The platform manifests deliveries from FIRES only — a
// crash window between the valve's claim and its fire append leaves
// on-disk delivered-* evidence but no manifest claim, and the next
// dueness re-authors (self-healing, never silent-lying).
func (s *Skeleton) recordRecitations(ctx context.Context, in SessionInput) {
	workDir, _, err := s.runDirs(in.RunID, in.Stage)
	if err != nil {
		return
	}
	fires, err := readReciteFires(filepath.Join(workDir, "gate-ctl"))
	if err != nil || len(fires) == 0 {
		return
	}
	for _, f := range fires {
		if _, err := s.cfg.Ledger.AppendRecitationManifest(ctx, in.RunID, in.Stage,
			f.LedgerVersion, f.ContentSHA256, f.SessionID, f.ToolUseID); err != nil {
			s.logger().Warn("stage: recitation manifest", "run", in.RunID, "err", err)
		}
	}
}

// readReciteFires reads the valve's fires log (mirrored line shape; the
// log is a platform-side artifact, the runner.go sessionStartFire
// precedent).
func readReciteFires(ctlDir string) ([]reciteFire, error) {
	raw, err := os.ReadFile(filepath.Join(ctlDir, reciteFiresFile))
	if err != nil {
		return nil, err
	}
	var out []reciteFire
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var f reciteFire
		if err := json.Unmarshal([]byte(line), &f); err != nil {
			continue
		}
		out = append(out, f)
	}
	return out, nil
}
