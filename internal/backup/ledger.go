package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// The snapshot_ledger access (Spec S13.10, P-T12-4). The integrity triple
// (sha256 + size + created-at) is the ONLY defense against a compromised remote
// swapping a blob — age proves who can read, never who wrote. The ledger row is
// recorded in platform.db (and, because the table is in the durable set, inside
// every SUBSEQUENT archive) after a successful push. The rows are append-only
// (migration 0009 triggers); retention prunes the repo BLOBS, never the ledger.

// LedgerRow is one snapshot_ledger row.
type LedgerRow struct {
	Seq        int64
	SnapshotID string
	SHA256     string
	SizeBytes  int64
	CreatedTS  string
	BlobName   string
	RepoCommit string
}

// recordLedger inserts one snapshot_ledger row (platform-scope, 15.6) after a
// successful push. user_id is 'platform' — the whole durable set is platform
// state.
func (s *Snapshotter) recordLedger(ctx context.Context, row LedgerRow) (LedgerRow, error) {
	err := s.cfg.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO snapshot_ledger (user_id, snapshot_id, sha256, size_bytes, created_ts, blob_name, repo_commit)
			 VALUES ('platform', ?, ?, ?, ?, ?, ?)`,
			row.SnapshotID, row.SHA256, row.SizeBytes, row.CreatedTS, row.BlobName, row.RepoCommit)
		if err != nil {
			return fmt.Errorf("backup: insert snapshot_ledger: %w", err)
		}
		row.Seq, err = res.LastInsertId()
		return err
	})
	if err != nil {
		return LedgerRow{}, err
	}
	return row, nil
}

// NewestLedgerRow returns the most recent snapshot_ledger row — what the
// restore drill verifies the fetched newest blob against (Spec S13.10). It errs
// when the ledger is empty (there is nothing to drill).
func NewestLedgerRow(ctx context.Context, db LedgerDB) (LedgerRow, error) {
	var r LedgerRow
	err := db.QueryRowContext(ctx,
		`SELECT seq, snapshot_id, sha256, size_bytes, created_ts, blob_name, repo_commit
		 FROM snapshot_ledger ORDER BY seq DESC LIMIT 1`).
		Scan(&r.Seq, &r.SnapshotID, &r.SHA256, &r.SizeBytes, &r.CreatedTS, &r.BlobName, &r.RepoCommit)
	if errors.Is(err, sql.ErrNoRows) {
		return LedgerRow{}, ErrNoSnapshots
	}
	if err != nil {
		return LedgerRow{}, fmt.Errorf("backup: read newest ledger row: %w", err)
	}
	return r, nil
}

// dropForKeep returns the blob bases to prune so that, after this snapshot, at
// most `keep` snapshots are retained (Spec S13.10 keep-N). Priors are ordered
// oldest-first by the ledger seq; the oldest (total+1-keep) are dropped.
func (s *Snapshotter) dropForKeep(ctx context.Context, keep int) ([]string, error) {
	if keep < 1 {
		keep = 1
	}
	rows, err := s.cfg.DB.QueryContext(ctx,
		`SELECT blob_name FROM snapshot_ledger ORDER BY seq`)
	if err != nil {
		return nil, fmt.Errorf("backup: read ledger bases: %w", err)
	}
	defer rows.Close()
	var priors []string
	for rows.Next() {
		var b string
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		priors = append(priors, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	toDrop := len(priors) + 1 - keep // +1 for the snapshot about to be written
	if toDrop <= 0 {
		return nil, nil
	}
	return priors[:toDrop], nil
}

// LedgerDB is the read surface the drill needs (satisfied by *storage.DB).
type LedgerDB interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// ErrNoSnapshots signals an empty snapshot_ledger (nothing to drill).
var ErrNoSnapshots = errors.New("backup: snapshot ledger is empty")
