package app

import (
	"context"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/query"
	"github.com/FNB2026/nas-data-governance/internal/report"
)

// DuplicateService provides duplicate group analysis. It supports two modes:
//   - In-memory: groups a file slice using report.DuplicateGroups (legacy
//     JSONL path, used by the `duplicates` and `plan` CLI commands).
//   - Database-backed: paginated queries via query.Reader (used by the
//     future desktop UI for virtual scrolling).
type DuplicateService struct {
	reader query.Reader // nil when only in-memory mode is used
}

// NewDuplicateService creates a service for in-memory duplicate analysis.
// Use NewDuplicateServiceWithReader to also support database-backed queries.
func NewDuplicateService() *DuplicateService {
	return &DuplicateService{}
}

// NewDuplicateServiceWithReader creates a service backed by a query.Reader
// (typically *store.SQLiteStore) for paginated group listing and detail.
func NewDuplicateServiceWithReader(r query.Reader) *DuplicateService {
	return &DuplicateService{reader: r}
}

// DuplicateGroups groups files by ContentSHA256 and computes physical
// identity statistics (hardlink-aware reclaimable capacity). This is the
// in-memory path used when the caller has already loaded file instances
// (e.g., from a JSONL index).
func (s *DuplicateService) DuplicateGroups(ctx context.Context, files []domain.FileInstance) []domain.DuplicateGroup {
	return report.DuplicateGroups(files)
}

// ListGroups returns a page of duplicate group summaries from the database.
// Requires a query.Reader; panics if constructed without one.
func (s *DuplicateService) ListGroups(ctx context.Context, q query.GroupQuery) (query.GroupPage, error) {
	if s.reader == nil {
		panic("app: ListGroups requires a query.Reader; use NewDuplicateServiceWithReader")
	}
	return s.reader.ListDuplicateGroups(ctx, q)
}

// GroupDetail loads the full detail (all file members) for a single
// duplicate group. Requires a query.Reader.
func (s *DuplicateService) GroupDetail(ctx context.Context, storageID, sha256 string) (query.GroupDetail, error) {
	if s.reader == nil {
		panic("app: GroupDetail requires a query.Reader; use NewDuplicateServiceWithReader")
	}
	return s.reader.GetGroupDetail(ctx, storageID, sha256)
}
