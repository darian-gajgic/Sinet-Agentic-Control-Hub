// Package backup is the Spec S13.10 platform-state snapshot pipeline and the
// restore drill. The pipeline takes the S02.9 durable set (VACUUM INTO -> text
// `.dump`) plus the platform's git-versioned file stores, wraps them as
// tar | zstd | age into ONE encrypted blob per snapshot, commits it to the
// operator-owned private snapshot repo (keep-N on one branch), and has the
// broker CAS-push it. Every snapshot's SHA-256 + size + created-at land in the
// snapshot_ledger (platform.db AND, via the durable set, every subsequent
// archive) — the load-bearing P-T12-4 integrity record, since age has no sender
// authentication. The restore drill fetches the newest blob, verifies it
// against the ledger FAIL-CLOSED, decrypts with the ESCROW identity, rebuilds
// the DB from the dump into a scratch location, and asserts integrity + the
// S02.9 invariants — proving the backup is restorable (a backup that is not
// restore-tested does not exist).
//
// Client-side encryption is UNCONDITIONAL (nothing leaves the host
// unencrypted); SECRETS are NEVER in the payload (per-person credential stores
// and broker secrets are a broker duty, deliberately decoupled — the pipeline
// reads platform.db + the file stores only, never the broker store).
//
// Imports: internal/storage (durable set) + internal/eventlog + internal/broker
// (client) + the adopted age/zstd modules + stdlib. It does NOT import
// internal/{memory,adapters,stage,intake} — the capability walls hold
// (CONVENTIONS §29, AST-pinned in conformance_test.go).
package backup

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"filippo.io/age"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/broker"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// Size discipline (Spec S13.10): GitHub enforces a 100 MB file limit, so the
// blob is chunked at ~90 MB — each part stays safely under. Household snapshots
// sit far below both, so the single-chunk path is the norm; the chunk path is
// the "if ever needed" escape hatch (R19).
const (
	chunkThresholdBytes  = 90 * 1024 * 1024
	gitHubFileLimitBytes = 100 * 1024 * 1024 // documented; chunks are always < this
	snapshotPrefix       = "snapshot-"
)

// keyBackupKeep is ⚙ backup.keep — the keep-N retention count (Spec S13.10),
// read by dotted key, never a constant.
const keyBackupKeep = "backup.keep"

// Provisional event types (pending the S14 contract, B5), extending the
// standing naming note.
const (
	// EventSnapshot records a completed snapshot (platform-scope).
	EventSnapshot = "platform.snapshot"
	// EventDrill records a restore-drill result either way (Spec S13.10).
	EventDrill = "platform.restore_drill"
	// EventDrillFlag is the High flag a fail-closed drill raises (the S14
	// seam): a ledger-verify mismatch means the remote may be compromised.
	EventDrillFlag = "platform.restore_drill_flag"

	eventSchemaVersion = 1
)

// Settings is the backup package's view of the settings registry (⚙ by dotted
// key through a narrow per-package interface, CONVENTIONS §2/§6).
type Settings interface {
	Int(key string) (int64, error)
}

// Pusher is the broker CAS-push seam (*broker.Client satisfies it). The snapshot
// push is the one credentialed, networked op — the broker holds the key and
// enforces P-T12-1 (CAS-always; a protected-ref push needs accept-authorization,
// which the snapshot path carries).
type Pusher interface {
	Push(req broker.Request) (broker.PushResult, error)
}

// RepoConfig is the snapshot repo's local clone + remote + protected ref.
type RepoConfig struct {
	LocalDir     string // persistent local working clone (platform is sole writer)
	Remote       string // remote URL (file:// dev fixtures, ssh:// live — stored data)
	Branch       string // the snapshot branch (default resolved to "main")
	ProtectedRef string // the snapshot repo's own protected ref (refs/heads/<branch>)
}

// Config configures a Snapshotter.
type Config struct {
	DB       *storage.DB
	Log      *eventlog.Log
	Settings Settings
	Crypto   Crypto // recipients (snapshot) + escrow identity (drill)
	Pusher   Pusher // broker CAS push
	Repo     RepoConfig
	// FileStores are the git-versioned file-store roots exported text-first.
	FileStores []NamedDir
	// BrokerGitProfile is the broker git-ssh-key profile for the live ssh push
	// (empty in dev = file:// transport, no key). The live leg is pure-config.
	BrokerGitProfile string
	// StripPayloadBeforeSeq elides run_events trace payload bodies at or below
	// this event_seq from the dump (Spec S02.9). 0 at v0 — nothing is compacted
	// yet, so nothing is past the 11.1 horizon; 11.1 compaction sets it.
	StripPayloadBeforeSeq int64
	// ChunkThresholdBytes overrides the ~90 MB blob-chunk threshold (Spec
	// S13.10). 0 uses the default; the household path is single-chunk.
	ChunkThresholdBytes int
	Now                 func() time.Time
}

// Snapshotter runs the S13.10 pipeline and the restore drill.
type Snapshotter struct {
	cfg Config
}

// New builds a Snapshotter, defaulting the branch and clock.
func New(cfg Config) (*Snapshotter, error) {
	if cfg.DB == nil || cfg.Log == nil || cfg.Settings == nil {
		return nil, fmt.Errorf("backup: config needs DB, Log and Settings")
	}
	if cfg.Repo.Branch == "" {
		cfg.Repo.Branch = "main"
	}
	if cfg.Repo.ProtectedRef == "" {
		cfg.Repo.ProtectedRef = "refs/heads/" + cfg.Repo.Branch
	}
	if cfg.ChunkThresholdBytes <= 0 {
		cfg.ChunkThresholdBytes = chunkThresholdBytes
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Snapshotter{cfg: cfg}, nil
}

func (s *Snapshotter) now() time.Time { return s.cfg.Now() }

// Snapshot runs one scheduled snapshot (Spec S13.10): durable set -> VACUUM
// INTO -> text dump + file-store exports -> tar|zstd|age -> one encrypted blob
// -> committed to the snapshot repo (keep-N) -> broker CAS push -> a
// snapshot_ledger row. Returns the recorded ledger row. Every git leg is local
// (commit) or the broker's (push); no network op runs in this process.
func (s *Snapshotter) Snapshot(ctx context.Context) (LedgerRow, error) {
	if len(s.cfg.Crypto.Recipients) == 0 {
		return LedgerRow{}, fmt.Errorf("backup: no age recipients — client-side encryption is unconditional (S13.10)")
	}
	keep, err := s.cfg.Settings.Int(keyBackupKeep)
	if err != nil {
		return LedgerRow{}, fmt.Errorf("backup: read ⚙ %s: %w", keyBackupKeep, err)
	}
	now := s.now()

	work, err := os.MkdirTemp("", "sinet-snapshot-")
	if err != nil {
		return LedgerRow{}, fmt.Errorf("backup: work dir: %w", err)
	}
	defer os.RemoveAll(work)

	// Durable set -> consistent copy -> text dump (traces past the horizon
	// excluded; secrets never here — platform.db + file stores only).
	vac := filepath.Join(work, "vac.db")
	if err := s.cfg.DB.VacuumInto(ctx, vac); err != nil {
		return LedgerRow{}, err
	}
	dumpSQL, userVersion, err := storage.DumpFrom(ctx, vac, s.cfg.StripPayloadBeforeSeq)
	if err != nil {
		return LedgerRow{}, err
	}
	archive, err := buildArchive(dumpSQL, userVersion, s.cfg.FileStores, now)
	if err != nil {
		return LedgerRow{}, err
	}
	blob, err := encryptBlob(archive, s.cfg.Crypto.Recipients)
	if err != nil {
		return LedgerRow{}, err
	}
	sum := sha256.Sum256(blob)
	shaHex := hex.EncodeToString(sum[:])

	base, err := snapshotBase(now)
	if err != nil {
		return LedgerRow{}, err
	}
	// keep-N: retain the newest `keep` snapshots (this one + the newest keep-1
	// priors), computed from the ledger's authoritative seq order.
	dropBases, err := s.dropForKeep(ctx, int(keep))
	if err != nil {
		return LedgerRow{}, err
	}
	commit, err := s.writeSnapshotCommit(ctx, blob, base, dropBases)
	if err != nil {
		return LedgerRow{}, err
	}

	res, err := s.cfg.Pusher.Push(broker.Request{
		RepoDir: s.cfg.Repo.LocalDir,
		Remote:  s.cfg.Repo.Remote,
		Refs: []broker.RefUpdate{{
			Ref: s.cfg.Repo.ProtectedRef, ExpectSHA: commit.ExpectSHA, SrcSHA: commit.NewSHA,
		}},
		Protected:  []string{s.cfg.Repo.ProtectedRef},
		Authorized: true, // the snapshot path is an authorized protected-ref writer (§8 reading 3)
		Profile:    s.cfg.BrokerGitProfile,
	})
	if err != nil {
		return LedgerRow{}, fmt.Errorf("backup: snapshot push: %w", err)
	}
	if res.Rejected {
		// The platform is the SOLE writer of the snapshot repo, so a stale
		// lease means the remote diverged (corruption/rotation) — loud, never a
		// blind retry against a new tip.
		return LedgerRow{}, fmt.Errorf("backup: snapshot push rejected — snapshot repo tip diverged")
	}

	row, err := s.recordLedger(ctx, LedgerRow{
		SnapshotID: base, SHA256: shaHex, SizeBytes: int64(len(blob)),
		CreatedTS: now.UTC().Format(time.RFC3339Nano), BlobName: base, RepoCommit: commit.NewSHA,
	})
	if err != nil {
		return LedgerRow{}, err
	}
	payload, _ := json.Marshal(map[string]any{
		"snapshot_id": base, "sha256": shaHex, "size_bytes": len(blob),
		"repo_commit": commit.NewSHA, "chunks": len(splitBlob(blob, s.cfg.ChunkThresholdBytes)),
	})
	if _, err := s.cfg.Log.Append(ctx, eventlog.Append{
		UserID: "platform", Type: EventSnapshot, SchemaVersion: eventSchemaVersion, Payload: payload, Time: now,
	}); err != nil {
		return LedgerRow{}, err
	}
	return row, nil
}

// encryptBlob wraps the archive as age -r <recipients> (Spec S13.10). The whole
// blob is what the snapshot_ledger hashes.
func encryptBlob(archive []byte, recipients []age.Recipient) ([]byte, error) {
	var buf bytes.Buffer
	w, err := encryptTo(&buf, recipients)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(archive); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("backup: close age writer: %w", err)
	}
	return buf.Bytes(), nil
}

// snapshotBase is a timestamp-sortable, collision-safe base name for a
// snapshot's blob (its chunk files are <base>.NNN).
func snapshotBase(now time.Time) (string, error) {
	var r [4]byte
	if _, err := rand.Read(r[:]); err != nil {
		return "", fmt.Errorf("backup: base name: %w", err)
	}
	return fmt.Sprintf("%s%s-%x", snapshotPrefix, now.UTC().Format("20060102T150405Z"), r), nil
}

// splitBlob splits data into chunks no larger than threshold (Spec S13.10 blob
// discipline). One chunk for household-sized blobs; the multi-chunk path keeps
// every file under GitHub's 100 MB limit.
func splitBlob(data []byte, threshold int) [][]byte {
	if threshold <= 0 || len(data) <= threshold {
		return [][]byte{data}
	}
	var out [][]byte
	for off := 0; off < len(data); off += threshold {
		end := min(off+threshold, len(data))
		out = append(out, data[off:end])
	}
	return out
}

// chunkBase strips a chunk file's trailing .NNN suffix to recover the base name.
func chunkBase(name string) string {
	if i := lastDot(name); i >= 0 {
		return name[:i]
	}
	return name
}

func lastDot(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '.' {
			return i
		}
	}
	return -1
}
