package store

import "context"

// ScanSummary is a path-free read model for channel adapters.
type ScanSummary struct{ Active, Missing, Unavailable, Quick, Full int }

func (s *SQLiteStore) ScanSummary(ctx context.Context, storageID string) (ScanSummary, error) {
	var v ScanSummary
	e := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(file_status='active'),0), COALESCE(SUM(file_status='missing'),0), COALESCE(SUM(file_status='unavailable'),0), COALESCE(SUM(file_status='active' AND quick_hash IS NOT NULL AND quick_hash<>''),0), COALESCE(SUM(file_status='active' AND content_sha256 IS NOT NULL AND content_sha256<>''),0) FROM file_instances WHERE storage_id=?`, storageID).Scan(&v.Active, &v.Missing, &v.Unavailable, &v.Quick, &v.Full)
	return v, e
}
