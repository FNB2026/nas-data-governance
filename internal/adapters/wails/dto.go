package wails

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/query"
)

// StorageInfo is the DTO for a storage entry returned by ListStorages.
type StorageInfo struct {
	ID        string `json:"id"`
	RootPath  string `json:"root_path"`
	Kind      string `json:"kind"`
	CreatedAt string `json:"created_at"` // RFC3339
}

// ListGroupsRequest is the input DTO for ListDuplicateGroups.
// Cursor is an opaque base64 token; empty string means first page.
type ListGroupsRequest struct {
	StorageID           string `json:"storage_id,omitempty"`
	PageSize            int    `json:"page_size,omitempty"`
	Cursor              string `json:"cursor,omitempty"`
	MinReclaimableBytes int64  `json:"min_reclaimable_bytes,omitempty"`
}

// GroupSummary is the list-view DTO for a duplicate group.
type GroupSummary struct {
	GroupID                  string `json:"group_id"`
	SHA256                   string `json:"sha256"`
	Size                     int64  `json:"size"`
	StorageID                string `json:"storage_id"`
	PathCount                int    `json:"path_count"`
	PhysicalCopyCount        int    `json:"physical_copy_count"`
	HardlinkAliasCount       int    `json:"hardlink_alias_count"`
	PhysicalReclaimableBytes int64  `json:"physical_reclaimable_bytes"`
	SamplePath               string `json:"sample_path"`
	DecisionType             string `json:"decision_type,omitempty"`
}

// ListGroupsResponse is the paginated output DTO for ListDuplicateGroups.
// NextCursor is empty when there are no more pages.
type ListGroupsResponse struct {
	Groups     []GroupSummary `json:"groups"`
	NextCursor string         `json:"next_cursor,omitempty"`
	TotalCount int            `json:"total_count"`
}

// FileItem is the DTO for a file instance in the group detail view.
type FileItem struct {
	StorageID     string `json:"storage_id"`
	Path          string `json:"path"`
	Name          string `json:"name"`
	Size          int64  `json:"size"`
	ModifiedAt    string `json:"modified_at"`
	IsSymlink     bool   `json:"is_symlink"`
	QuickHash     string `json:"quick_hash,omitempty"`
	ContentSHA256 string `json:"content_sha256,omitempty"`
	// Physical identity (hardlink awareness)
	PhysicalDevice    uint64 `json:"physical_device,omitempty"`
	PhysicalInode     uint64 `json:"physical_inode,omitempty"`
	PhysicalLinkCount uint64 `json:"physical_link_count,omitempty"`
	PhysicalReliable  bool   `json:"physical_reliable"`
	// Format info
	FormatKind string `json:"format_kind,omitempty"`
	FormatMIME string `json:"format_mime,omitempty"`
}

// GroupDetailResponse is the output DTO for GetGroupDetail.
// It embeds GroupSummary and adds the full file member list.
type GroupDetailResponse struct {
	GroupSummary
	Files []FileItem `json:"files"`
}

// ---- mapping helpers ----

func mapStorages(storages []domain.Storage) []StorageInfo {
	out := make([]StorageInfo, len(storages))
	for i, s := range storages {
		createdAt := ""
		if !s.CreatedAt.IsZero() {
			createdAt = s.CreatedAt.UTC().Format(time.RFC3339)
		}
		out[i] = StorageInfo{
			ID:        s.ID,
			RootPath:  s.RootPath,
			Kind:      s.Kind,
			CreatedAt: createdAt,
		}
	}
	return out
}

func mapGroupSummary(g query.DuplicateGroupSummary) GroupSummary {
	return GroupSummary{
		GroupID:                  g.GroupID,
		SHA256:                   g.SHA256,
		Size:                     g.Size,
		StorageID:                g.StorageID,
		PathCount:                g.PathCount,
		PhysicalCopyCount:        g.PhysicalCopyCount,
		HardlinkAliasCount:       g.HardlinkAliasCount,
		PhysicalReclaimableBytes: g.PhysicalReclaimableBytes,
		SamplePath:               g.SamplePath,
		DecisionType:             g.DecisionType,
	}
}

func mapGroupPage(page query.GroupPage) ListGroupsResponse {
	groups := make([]GroupSummary, len(page.Groups))
	for i, g := range page.Groups {
		groups[i] = mapGroupSummary(g)
	}
	return ListGroupsResponse{
		Groups:     groups,
		NextCursor: encodeCursor(page.NextCursor),
		TotalCount: page.TotalCount,
	}
}

func mapFileItem(f domain.FileInstance) FileItem {
	modifiedAt := ""
	if !f.ModifiedAt.IsZero() {
		modifiedAt = f.ModifiedAt.UTC().Format(time.RFC3339)
	}
	return FileItem{
		StorageID:         f.StorageID,
		Path:              f.Path,
		Name:              f.Name,
		Size:              f.Size,
		ModifiedAt:        modifiedAt,
		IsSymlink:         f.IsSymlink,
		QuickHash:         f.QuickHash,
		ContentSHA256:     f.ContentSHA256,
		PhysicalDevice:    f.Physical.Device,
		PhysicalInode:     f.Physical.Inode,
		PhysicalLinkCount: f.Physical.LinkCount,
		PhysicalReliable:  f.Physical.Reliable,
		FormatKind:        f.Format.Format,
		FormatMIME:        f.Format.MIME,
	}
}

func mapGroupDetail(d query.GroupDetail) GroupDetailResponse {
	files := make([]FileItem, len(d.Files))
	for i, f := range d.Files {
		files[i] = mapFileItem(f)
	}
	return GroupDetailResponse{
		GroupSummary: mapGroupSummary(d.DuplicateGroupSummary),
		Files:        files,
	}
}

// ---- cursor encoding ----

// encodeCursor serializes a GroupCursor into an opaque base64 token.
// Returns empty string for nil cursor (no more pages).
func encodeCursor(c *query.GroupCursor) string {
	if c == nil {
		return ""
	}
	data, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(data)
}

// decodeCursor parses an opaque base64 token back into a GroupCursor.
// Returns nil (no error) for empty string (first page).
func decodeCursor(s string) (*query.GroupCursor, error) {
	if s == "" {
		return nil, nil
	}
	data, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("wails: invalid cursor encoding: %w", err)
	}
	var c query.GroupCursor
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("wails: decode cursor: %w", err)
	}
	return &c, nil
}

// toQuery converts a ListGroupsRequest into a query.GroupQuery.
func (req ListGroupsRequest) toQuery() (query.GroupQuery, error) {
	if req.PageSize < 0 || req.PageSize > 200 {
		return query.GroupQuery{}, fmt.Errorf("wails: page_size must be between 1 and 200, or 0 for the default")
	}
	if req.MinReclaimableBytes < 0 {
		return query.GroupQuery{}, fmt.Errorf("wails: min_reclaimable_bytes must not be negative")
	}
	cursor, err := decodeCursor(req.Cursor)
	if err != nil {
		return query.GroupQuery{}, err
	}
	if cursor != nil && (cursor.SHA256 == "" || cursor.StorageID == "" || cursor.ReclaimableEstimate < 0) {
		return query.GroupQuery{}, fmt.Errorf("wails: cursor is incomplete or invalid")
	}
	return query.GroupQuery{
		StorageID:           req.StorageID,
		PageSize:            req.PageSize,
		Cursor:              cursor,
		MinReclaimableBytes: req.MinReclaimableBytes,
	}, nil
}
