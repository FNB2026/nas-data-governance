package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/query"
	"github.com/FNB2026/nas-data-governance/internal/report"
)

// Compile-time assertion: SQLiteStore must implement query.Reader.
var _ query.Reader = (*SQLiteStore)(nil)

// ListDuplicateGroups implements query.Reader via keyset pagination on
// the file_instances table. It uses a CTE to compute group summaries,
// applies the keyset cursor, then joins with file_instances to load
// members for physical stats computation.
//
// The sort key is (reclaimable_estimate DESC, content_sha256 ASC,
// storage_id ASC) where reclaimable_estimate = (path_count - 1) * size.
// This is an upper bound on PhysicalReclaimableBytes (which may be less
// due to hardlinks); physical stats are computed accurately in Go after
// fetching the page.
func (s *SQLiteStore) ListDuplicateGroups(ctx context.Context, q query.GroupQuery) (query.GroupPage, error) {
	pageSize := q.PageSize
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}

	// Phase 1: total count for UI display.
	totalCount, err := s.countDuplicateGroups(ctx, q.StorageID, q.MinReclaimableBytes)
	if err != nil {
		return query.GroupPage{}, fmt.Errorf("store: count duplicate groups: %w", err)
	}

	// Phase 2: build and execute the paginated query.
	var sb strings.Builder
	var args []any

	sb.WriteString(`
WITH dup_groups AS (
    SELECT
        storage_id,
        content_sha256,
        COUNT(*) AS path_count,
        MIN(size) AS group_size,
        MIN(path) AS sample_path,
        (COUNT(*) - 1) * MIN(size) AS reclaimable_estimate
    FROM file_instances
    WHERE file_status = 'active'
      AND content_sha256 IS NOT NULL
      AND content_sha256 <> ''`)
	if q.StorageID != "" {
		sb.WriteString("\n      AND storage_id = ?")
		args = append(args, q.StorageID)
	}
	sb.WriteString(`
    GROUP BY storage_id, content_sha256
    HAVING COUNT(*) > 1`)
	if q.MinReclaimableBytes > 0 {
		sb.WriteString(" AND (COUNT(*) - 1) * MIN(size) >= ?")
		args = append(args, q.MinReclaimableBytes)
	}
	sb.WriteString(`
),
page_groups AS (
    SELECT * FROM dup_groups`)
	if q.Cursor != nil {
		sb.WriteString(`
    WHERE reclaimable_estimate < ?
       OR (reclaimable_estimate = ? AND content_sha256 > ?)
       OR (reclaimable_estimate = ? AND content_sha256 = ? AND storage_id > ?)`)
		c := q.Cursor
		args = append(args,
			c.ReclaimableEstimate,
			c.ReclaimableEstimate, c.SHA256,
			c.ReclaimableEstimate, c.SHA256, c.StorageID,
		)
	}
	sb.WriteString(`
    ORDER BY reclaimable_estimate DESC, content_sha256 ASC, storage_id ASC
    LIMIT ?
)`)
	args = append(args, pageSize+1) // +1 to detect next page

	sb.WriteString(`
SELECT
    pg.storage_id, pg.content_sha256, pg.path_count, pg.group_size,
    pg.sample_path, pg.reclaimable_estimate,
    fi.path, fi.name, fi.size, fi.mode, fi.mtime, fi.device, fi.inode,
    fi.quick_hash, fi.discovered_at
FROM page_groups pg
JOIN file_instances fi
    ON fi.content_sha256 = pg.content_sha256
   AND fi.storage_id = pg.storage_id
   AND fi.file_status = 'active'
ORDER BY pg.reclaimable_estimate DESC, pg.content_sha256 ASC,
         pg.storage_id ASC, fi.path`)

	rows, err := s.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return query.GroupPage{}, fmt.Errorf("store: query duplicate groups: %w", err)
	}
	defer rows.Close()

	// Phase 3: parse rows, grouping by (storage_id, content_sha256).
	type groupKey struct {
		storageID string
		sha256    string
	}
	type groupAccum struct {
		storageID           string
		sha256              string
		pathCount           int
		groupSize           int64
		samplePath          string
		reclaimableEstimate int64
		files               []domain.FileInstance
	}

	groups := map[groupKey]*groupAccum{}
	order := []groupKey{}

	for rows.Next() {
		var (
			storageID, contentSHA256, samplePath string
			pathCount                            int
			groupSize, reclaimableEstimate       int64
			path, name, mtime, discoveredAt      string
			size                                 int64
			mode                                 uint32
			device, inode                        int64
			quickHash                            string
		)

		if err := rows.Scan(
			&storageID, &contentSHA256, &pathCount, &groupSize,
			&samplePath, &reclaimableEstimate,
			&path, &name, &size, &mode, &mtime, &device, &inode,
			&quickHash, &discoveredAt,
		); err != nil {
			return query.GroupPage{}, fmt.Errorf("store: scan duplicate group row: %w", err)
		}

		key := groupKey{storageID, contentSHA256}
		acc, exists := groups[key]
		if !exists {
			acc = &groupAccum{
				storageID:           storageID,
				sha256:              contentSHA256,
				pathCount:           pathCount,
				groupSize:           groupSize,
				samplePath:          samplePath,
				reclaimableEstimate: reclaimableEstimate,
			}
			groups[key] = acc
			order = append(order, key)
		}

		f := domain.FileInstance{
			StorageID:     storageID,
			Path:          path,
			Name:          name,
			Size:          size,
			Mode:          mode,
			Device:        uint64(device),
			Inode:         uint64(inode),
			QuickHash:     quickHash,
			ContentSHA256: contentSHA256,
		}
		// Reconstruct PhysicalIdentity from device/inode columns.
		// The database does not persist the Reliable flag; we infer it
		// from non-zero device and inode values so that hardlink dedup
		// works correctly for query results read back from SQLite.
		if device != 0 && inode != 0 {
			f.Physical = domain.PhysicalIdentity{
				Device:   uint64(device),
				Inode:    uint64(inode),
				Reliable: true,
			}
		}
		f.ModifiedAt, _ = time.Parse(time.RFC3339Nano, mtime)
		f.DiscoveredAt, _ = time.Parse(time.RFC3339Nano, discoveredAt)
		acc.files = append(acc.files, f)
	}
	if err := rows.Err(); err != nil {
		return query.GroupPage{}, fmt.Errorf("store: iterate duplicate groups: %w", err)
	}

	// Phase 4: detect next page and trim to pageSize.
	hasNext := len(order) > pageSize
	if hasNext {
		order = order[:pageSize]
	}

	// Phase 5: build summaries with physical stats and group IDs.
	summaries := make([]query.DuplicateGroupSummary, 0, len(order))
	groupIDs := make([]string, 0, len(order))
	for _, key := range order {
		acc := groups[key]
		stats := report.ComputePhysicalStats(acc.files, acc.groupSize)
		groupID := report.StableGroupID(acc.storageID, acc.sha256)
		summaries = append(summaries, query.DuplicateGroupSummary{
			GroupID:                  groupID,
			SHA256:                   acc.sha256,
			Size:                     acc.groupSize,
			StorageID:                acc.storageID,
			PathCount:                acc.pathCount,
			PhysicalCopyCount:        stats.PhysicalCopyCount,
			HardlinkAliasCount:       stats.HardlinkAliasCount,
			PhysicalReclaimableBytes: stats.PhysicalReclaimableBytes,
			SamplePath:               acc.samplePath,
		})
		groupIDs = append(groupIDs, groupID)
	}

	// Phase 6: batch-load review decisions.
	if len(groupIDs) > 0 {
		decisions, err := s.batchGroupDecisions(ctx, groupIDs)
		if err != nil {
			return query.GroupPage{}, fmt.Errorf("store: batch group decisions: %w", err)
		}
		for i := range summaries {
			if dt, ok := decisions[summaries[i].GroupID]; ok {
				summaries[i].DecisionType = dt
			}
		}
	}

	// Phase 7: build next cursor from the last item on this page.
	var nextCursor *query.GroupCursor
	if hasNext && len(order) > 0 {
		lastKey := order[len(order)-1]
		lastAcc := groups[lastKey]
		nextCursor = &query.GroupCursor{
			ReclaimableEstimate: lastAcc.reclaimableEstimate,
			SHA256:              lastAcc.sha256,
			StorageID:           lastAcc.storageID,
		}
	}

	return query.GroupPage{
		Groups:     summaries,
		NextCursor: nextCursor,
		TotalCount: totalCount,
	}, nil
}

// GetGroupDetail implements query.Reader by loading all file members for
// a single duplicate group identified by (storage_id, content_sha256).
func (s *SQLiteStore) GetGroupDetail(ctx context.Context, storageID, sha256 string) (query.GroupDetail, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT storage_id, path, name, size, mode, mtime, device, inode,
       quick_hash, content_sha256, discovered_at
FROM file_instances
WHERE file_status = 'active' AND storage_id = ? AND content_sha256 = ?
ORDER BY path`, storageID, sha256)
	if err != nil {
		return query.GroupDetail{}, fmt.Errorf("store: get group detail: %w", err)
	}
	defer rows.Close()

	files := make([]domain.FileInstance, 0)
	for rows.Next() {
		var f domain.FileInstance
		var mtime, discoveredAt string
		var device, inode int64
		if err := rows.Scan(&f.StorageID, &f.Path, &f.Name, &f.Size, &f.Mode, &mtime,
			&device, &inode, &f.QuickHash, &f.ContentSHA256, &discoveredAt); err != nil {
			return query.GroupDetail{}, fmt.Errorf("store: scan group detail row: %w", err)
		}
		f.Device = uint64(device)
		f.Inode = uint64(inode)
		// Reconstruct PhysicalIdentity from device/inode columns.
		if device != 0 && inode != 0 {
			f.Physical = domain.PhysicalIdentity{
				Device:   uint64(device),
				Inode:    uint64(inode),
				Reliable: true,
			}
		}
		f.ModifiedAt, _ = time.Parse(time.RFC3339Nano, mtime)
		f.DiscoveredAt, _ = time.Parse(time.RFC3339Nano, discoveredAt)
		files = append(files, f)
	}
	if err := rows.Err(); err != nil {
		return query.GroupDetail{}, fmt.Errorf("store: iterate group detail: %w", err)
	}
	if len(files) == 0 {
		return query.GroupDetail{}, query.ErrNotFound
	}

	size := files[0].Size
	stats := report.ComputePhysicalStats(files, size)
	groupID := report.StableGroupID(storageID, sha256)

	// Load latest decision for this group.
	decisionType := ""
	decisions, err := s.batchGroupDecisions(ctx, []string{groupID})
	if err != nil {
		return query.GroupDetail{}, fmt.Errorf("store: get group decision: %w", err)
	}
	if dt, ok := decisions[groupID]; ok {
		decisionType = dt
	}

	return query.GroupDetail{
		DuplicateGroupSummary: query.DuplicateGroupSummary{
			GroupID:                  groupID,
			SHA256:                   sha256,
			Size:                     size,
			StorageID:                storageID,
			PathCount:                len(files),
			PhysicalCopyCount:        stats.PhysicalCopyCount,
			HardlinkAliasCount:       stats.HardlinkAliasCount,
			PhysicalReclaimableBytes: stats.PhysicalReclaimableBytes,
			SamplePath:               files[0].Path,
			DecisionType:             decisionType,
		},
		Files: files,
	}, nil
}

// countDuplicateGroups returns the total number of duplicate groups
// matching the filter, ignoring pagination.
func (s *SQLiteStore) countDuplicateGroups(ctx context.Context, storageID string, minReclaimable int64) (int, error) {
	var sb strings.Builder
	var args []any

	sb.WriteString(`
SELECT COUNT(*) FROM (
    SELECT 1 FROM file_instances
    WHERE file_status = 'active' AND content_sha256 IS NOT NULL AND content_sha256 <> ''`)
	if storageID != "" {
		sb.WriteString(" AND storage_id = ?")
		args = append(args, storageID)
	}
	sb.WriteString(`
    GROUP BY storage_id, content_sha256
    HAVING COUNT(*) > 1`)
	if minReclaimable > 0 {
		sb.WriteString(" AND (COUNT(*) - 1) * MIN(size) >= ?")
		args = append(args, minReclaimable)
	}
	sb.WriteString("\n) AS cnt")

	var count int
	if err := s.db.QueryRowContext(ctx, sb.String(), args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("store: count duplicate groups: %w", err)
	}
	return count, nil
}

// batchGroupDecisions loads the latest decision type for each group_id.
// Returns a map of group_id -> decision_type. When multiple decisions
// exist for the same group, the one with the most recent updated_at wins.
func (s *SQLiteStore) batchGroupDecisions(ctx context.Context, groupIDs []string) (map[string]string, error) {
	if len(groupIDs) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(groupIDs))
	args := make([]any, len(groupIDs))
	for i, id := range groupIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	q := fmt.Sprintf(`
SELECT group_id, decision_type FROM group_decisions
WHERE group_id IN (%s)
ORDER BY group_id, updated_at DESC`, strings.Join(placeholders, ","))

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: batch group decisions: %w", err)
	}
	defer rows.Close()

	result := map[string]string{}
	for rows.Next() {
		var groupID, decisionType string
		if err := rows.Scan(&groupID, &decisionType); err != nil {
			return nil, fmt.Errorf("store: scan group decision: %w", err)
		}
		// Keep only the first (latest due to ORDER BY) per group_id.
		if _, exists := result[groupID]; !exists {
			result[groupID] = decisionType
		}
	}
	return result, rows.Err()
}
