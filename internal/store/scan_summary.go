package store

import (
	"context"
	"database/sql"
)

// ScanSummary is a path-free read model for channel adapters.
type ScanSummary struct{ Active, Missing, Unavailable, Quick, Full int }

// CheckpointSummary exposes only progress and status; it is not a Resume selector.
type CheckpointSummary struct {
	Status  string
	Scanned int
}

// LatestCheckpointSummary includes terminal checkpoints without changing
// LastCheckpoint's existing resumable-only semantics.
func (s *SQLiteStore) LatestCheckpointSummary(ctx context.Context, storageID string) (CheckpointSummary, error) {
	var v CheckpointSummary
	err := s.db.QueryRowContext(ctx, `SELECT status, scanned_count FROM scan_checkpoints WHERE storage_id=? ORDER BY id DESC LIMIT 1`, storageID).Scan(&v.Status, &v.Scanned)
	if err == sql.ErrNoRows {
		return v, ErrNotFound
	}
	return v, err
}

func (s *SQLiteStore) ScanSummary(ctx context.Context, storageID string) (ScanSummary, error) {
	var v ScanSummary
	e := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(file_status='active'),0), COALESCE(SUM(file_status='missing'),0), COALESCE(SUM(file_status='unavailable'),0), COALESCE(SUM(file_status='active' AND quick_hash IS NOT NULL AND quick_hash<>''),0), COALESCE(SUM(file_status='active' AND content_sha256 IS NOT NULL AND content_sha256<>''),0) FROM file_instances WHERE storage_id=?`, storageID).Scan(&v.Active, &v.Missing, &v.Unavailable, &v.Quick, &v.Full)
	return v, e
}
