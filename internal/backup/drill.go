package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// The S13.10 restore drill — scheduled and TESTED: a backup that is not
// restore-tested does not exist. It fetches the newest blob from the REMOTE
// (not the local clone — it proves recovery from the actual remote), verifies
// it against the snapshot_ledger FAIL-CLOSED (a mismatch aborts and raises a
// High flag — the P-T12-4 defense against a swapped blob), decrypts with the
// ESCROW identity (proving the recovery material, not merely a live key),
// unpacks, rebuilds the DB into a SCRATCH location (the live DB is never
// touched), and asserts integrity + the S02.9 invariants. The result is an
// event-log record either way.

// DrillResult is the outcome of a restore drill.
type DrillResult struct {
	OK             bool
	SnapshotID     string
	LedgerVerified bool
	Check          storage.RestoreCheck
	Reason         string // failure reason when !OK
}

// RestoreDrill runs one scheduled restore drill (Spec S13.10). A ledger
// mismatch (a swapped/corrupt blob) fails CLOSED: the drill aborts, raises a
// High flag, records the failure, and returns an error — it never proceeds to
// decrypt an unverified blob. An empty ledger returns ErrNoSnapshots (nothing
// to drill).
func (s *Snapshotter) RestoreDrill(ctx context.Context) (DrillResult, error) {
	row, err := NewestLedgerRow(ctx, s.cfg.DB)
	if err != nil {
		return DrillResult{}, err
	}
	now := s.now()

	work, err := os.MkdirTemp("", "sinet-drill-")
	if err != nil {
		return DrillResult{}, fmt.Errorf("backup: drill work dir: %w", err)
	}
	defer os.RemoveAll(work)

	// Fetch: a FRESH clone of the remote (recovery from the actual remote, not
	// the platform's local snapshot clone).
	clone := filepath.Join(work, "remote-clone")
	if _, err := s.git(ctx, "", "clone", "--branch", s.cfg.Repo.Branch, s.cfg.Repo.Remote, clone); err != nil {
		return DrillResult{}, fmt.Errorf("backup: drill fetch: %w", err)
	}
	blob, err := readBlobChunks(clone, row.BlobName)
	if err != nil {
		return s.failDrill(ctx, row, now, fmt.Sprintf("read blob: %v", err))
	}

	// Verify against the ledger FAIL-CLOSED (P-T12-4).
	sum := sha256.Sum256(blob)
	if hex.EncodeToString(sum[:]) != row.SHA256 || int64(len(blob)) != row.SizeBytes {
		return s.failDrill(ctx, row, now,
			fmt.Sprintf("ledger mismatch: fetched %d bytes sha %s, ledger %d bytes sha %s",
				len(blob), hex.EncodeToString(sum[:])[:12], row.SizeBytes, row.SHA256[:12]))
	}

	// Decrypt with the ESCROW identity (proving the recovery material).
	r, err := decryptWith(bytes.NewReader(blob), s.cfg.Crypto.Identity)
	if err != nil {
		return s.failDrill(ctx, row, now, fmt.Sprintf("decrypt: %v", err))
	}
	archiveBytes, err := io.ReadAll(r)
	if err != nil {
		return s.failDrill(ctx, row, now, fmt.Sprintf("read decrypted: %v", err))
	}
	un, err := unpackArchive(archiveBytes)
	if err != nil {
		return s.failDrill(ctx, row, now, fmt.Sprintf("unpack: %v", err))
	}

	// Rebuild into a scratch DB (the live DB is never touched) and assert the
	// S02.9 invariants.
	rebuilt := filepath.Join(work, "rebuilt.db")
	if err := storage.RebuildFromDump(ctx, rebuilt, un.DumpSQL, un.UserVersion); err != nil {
		return s.failDrill(ctx, row, now, fmt.Sprintf("rebuild: %v", err))
	}
	check, err := storage.CheckRestored(ctx, rebuilt)
	if err != nil {
		return s.failDrill(ctx, row, now, fmt.Sprintf("invariants: %v", err))
	}
	if !check.AllOK() {
		return s.failDrill(ctx, row, now,
			fmt.Sprintf("S02.9 invariants failed: integrity=%v fk=%v gapfree=%v", check.IntegrityOK, check.ForeignKeysOK, check.EventSeqGapFree))
	}

	res := DrillResult{OK: true, SnapshotID: row.SnapshotID, LedgerVerified: true, Check: check}
	if err := s.recordDrill(ctx, now, res); err != nil {
		return DrillResult{}, err
	}
	return res, nil
}

// failDrill records a failed drill (event + High flag on a ledger mismatch) and
// returns the fail-closed error.
func (s *Snapshotter) failDrill(ctx context.Context, row LedgerRow, now time.Time, reason string) (DrillResult, error) {
	res := DrillResult{OK: false, SnapshotID: row.SnapshotID, Reason: reason}
	// A verification failure raises a High flag (the S14 seam): the remote may
	// be compromised, or the recovery material has drifted.
	flag, _ := json.Marshal(map[string]any{"snapshot_id": row.SnapshotID, "reason": reason, "severity": "high"})
	if _, err := s.cfg.Log.Append(ctx, eventlog.Append{
		UserID: "platform", Type: EventDrillFlag, SchemaVersion: eventSchemaVersion, Payload: flag, Time: now,
	}); err != nil {
		return DrillResult{}, err
	}
	if err := s.recordDrill(ctx, now, res); err != nil {
		return DrillResult{}, err
	}
	return res, fmt.Errorf("backup: restore drill failed CLOSED: %s", reason)
}

// recordDrill appends the drill result event either way (Spec S13.10).
func (s *Snapshotter) recordDrill(ctx context.Context, now time.Time, res DrillResult) error {
	payload, _ := json.Marshal(map[string]any{
		"snapshot_id": res.SnapshotID, "ok": res.OK, "ledger_verified": res.LedgerVerified,
		"integrity_ok": res.Check.IntegrityOK, "foreign_keys_ok": res.Check.ForeignKeysOK,
		"event_seq_gap_free": res.Check.EventSeqGapFree, "run_event_count": res.Check.RunEventCount,
		"reason": res.Reason,
	})
	_, err := s.cfg.Log.Append(ctx, eventlog.Append{
		UserID: "platform", Type: EventDrill, SchemaVersion: eventSchemaVersion, Payload: payload, Time: now,
	})
	return err
}

// readBlobChunks reads and concatenates a snapshot's chunk files (<base>.NNN,
// in order) from a checked-out snapshot-repo tree.
func readBlobChunks(dir, base string) ([]byte, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var chunks []string
	for _, e := range entries {
		if !e.IsDir() && chunkBase(e.Name()) == base && strings.HasPrefix(e.Name(), base+".") {
			chunks = append(chunks, e.Name())
		}
	}
	if len(chunks) == 0 {
		return nil, fmt.Errorf("no chunk files for snapshot %q in the remote tree", base)
	}
	sort.Strings(chunks)
	var buf bytes.Buffer
	for _, c := range chunks {
		body, err := os.ReadFile(filepath.Join(dir, c))
		if err != nil {
			return nil, err
		}
		buf.Write(body)
	}
	return buf.Bytes(), nil
}
