// Package query defines the read-model DTOs and ports for the desktop
// visualization layer. It provides paginated, filtered access to duplicate
// groups without loading the entire dataset into memory.
//
// Per ADR-0006 / V1 Query Read Model:
//   - ListDuplicateGroups uses keyset pagination on (reclaimable_estimate
//     DESC, content_sha256 ASC, storage_id ASC) to avoid OFFSET's O(n) scan.
//   - GetGroupDetail loads file members on demand, not in the list view.
//   - Pagination/filtering use a hardlink-aware physical reclaimable value;
//     final physical stats are computed in Go after fetching rows.
package query

import (
	"context"
	"errors"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

// ErrNotFound is returned when a single-group lookup has no match.
var ErrNotFound = errors.New("query: group not found")

// DuplicateGroupSummary is the list-view DTO for a duplicate group.
// It excludes individual file members for efficient pagination. Physical
// stats are computed accurately (hardlink-aware) even in the summary.
type DuplicateGroupSummary struct {
	GroupID                  string `json:"group_id"`
	SHA256                   string `json:"sha256"`
	Size                     int64  `json:"size"`
	StorageID                string `json:"storage_id"`
	PathCount                int    `json:"path_count"`
	PhysicalCopyCount        int    `json:"physical_copy_count"`
	HardlinkAliasCount       int    `json:"hardlink_alias_count"`
	PhysicalReclaimableBytes int64  `json:"physical_reclaimable_bytes"`
	// SamplePath is one representative path for quick display.
	SamplePath string `json:"sample_path"`
	// DecisionType is the latest review decision for this group, empty
	// if no decision has been recorded.
	DecisionType string `json:"decision_type,omitempty"`
}

// GroupDetail is the full detail-view DTO with all file members loaded.
type GroupDetail struct {
	DuplicateGroupSummary
	Files []domain.FileInstance `json:"files"`
}

// GroupCursor is the keyset pagination cursor. It captures the sort
// position of the last item on the current page so the next page can
// resume without scanning prior rows.
//
// Sort order: ReclaimableEstimate DESC, SHA256 ASC, StorageID ASC.
type GroupCursor struct {
	ReclaimableEstimate int64  `json:"reclaimable_estimate"`
	SHA256              string `json:"sha256"`
	StorageID           string `json:"storage_id"`
}

// GroupQuery defines parameters for listing duplicate groups.
type GroupQuery struct {
	// StorageID filters by storage; empty means all storages.
	StorageID string
	// PageSize is items per page. Values <= 0 or > 200 default to 20.
	PageSize int
	// Cursor is the keyset position; nil means first page.
	Cursor *GroupCursor
	// MinReclaimableBytes filters out groups whose hardlink-aware physical
	// reclaimable bytes are below this threshold. 0 = no minimum.
	MinReclaimableBytes int64
}

// GroupPage is one page of duplicate group results.
type GroupPage struct {
	Groups     []DuplicateGroupSummary `json:"groups"`
	NextCursor *GroupCursor            `json:"next_cursor,omitempty"`
	// TotalCount is the estimated total number of matching groups
	// (ignoring pagination), for UI display.
	TotalCount int `json:"total_count"`
}

// Reader is the read-model port for duplicate group queries. The store
// package provides a concrete implementation backed by SQLite.
type Reader interface {
	// ListDuplicateGroups returns a page of duplicate group summaries,
	// sorted by reclaimable estimate descending then SHA256 ascending.
	ListDuplicateGroups(ctx context.Context, q GroupQuery) (GroupPage, error)

	// GetGroupDetail loads the full detail (all file members) for a
	// single duplicate group identified by storage_id + content_sha256.
	// Returns ErrNotFound when no active files share the given hash.
	GetGroupDetail(ctx context.Context, storageID, sha256 string) (GroupDetail, error)
}
