package app

import (
	"context"
	"fmt"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/formatdiag"
	"github.com/FNB2026/nas-data-governance/internal/governancediag"
	"github.com/FNB2026/nas-data-governance/internal/merge"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// DiagnosticStore is the store subset needed by DiagnosticService.
// store.SQLiteStore satisfies this interface structurally.
type DiagnosticStore interface {
	ListFiles(ctx context.Context, storageID string) ([]domain.FileInstance, error)
	ListFormats(ctx context.Context, storageID string) ([]store.FormatRecord, error)
}

// DiagnosticService provides read-only diagnostic reports from the
// project database. It never opens source files and never grants
// cleanup authority; all reports are for human review only.
//
// The service does NOT:
//   - Parse command-line flags (CLI's job)
//   - Write JSON report files (CLI's job)
//   - Print to stdout/stderr (CLI's job)
//   - Access the filesystem (all data comes from the store)
type DiagnosticService struct {
	store DiagnosticStore
}

// NewDiagnosticService creates a new diagnostic service backed by the
// given store. The store may be a read-only connection.
func NewDiagnosticService(st DiagnosticStore) *DiagnosticService {
	return &DiagnosticService{store: st}
}

// DiagnoseFormatsInput defines parameters for a format diagnostic run.
type DiagnoseFormatsInput struct {
	// StorageID filters files by storage ID (empty = all storages).
	StorageID string
	// LargeUnknownMinimum is the size threshold for flagging unknown
	// formats. Defaults to 100 MiB; must be at least 1 MiB.
	LargeUnknownMinimum int64
}

// DiagnoseFormats builds a format review report from the project
// database. It joins file instances with persisted format records to
// identify large unknown files, extension mismatches, and metadata gaps.
func (s *DiagnosticService) DiagnoseFormats(ctx context.Context, in DiagnoseFormatsInput) (*formatdiag.Report, error) {
	if in.LargeUnknownMinimum < 1<<20 {
		in.LargeUnknownMinimum = 100 << 20 // default 100 MiB
	}
	files, err := s.store.ListFiles(ctx, in.StorageID)
	if err != nil {
		return nil, fmt.Errorf("app: diagnose formats: list files: %w", err)
	}
	formats, err := s.store.ListFormats(ctx, in.StorageID)
	if err != nil {
		return nil, fmt.Errorf("app: diagnose formats: list formats: %w", err)
	}
	report := formatdiag.Build(files, formats, in.LargeUnknownMinimum, time.Now())
	return &report, nil
}

// DiagnoseGovernanceInput defines parameters for a governance diagnostic run.
type DiagnoseGovernanceInput struct {
	// StorageID filters files by storage ID (empty = all storages).
	StorageID string
	// LargeMediaMinimum is the size threshold for detailed media review.
	// Defaults to 100 MiB; must be at least 1 MiB.
	LargeMediaMinimum int64
}

// DiagnoseGovernance builds a governance review report from the
// project database. It merges file instances with format records,
// then analyzes duplicate groups, zero-byte files, and large media.
//
// All generated plans are DRAFT; if any non-draft plan is detected the
// service refuses the result and returns an error (safety guard).
func (s *DiagnosticService) DiagnoseGovernance(ctx context.Context, in DiagnoseGovernanceInput) (*governancediag.Report, error) {
	if in.LargeMediaMinimum < 1<<20 {
		in.LargeMediaMinimum = 100 << 20 // default 100 MiB
	}
	files, err := s.store.ListFiles(ctx, in.StorageID)
	if err != nil {
		return nil, fmt.Errorf("app: diagnose governance: list files: %w", err)
	}
	formats, err := s.store.ListFormats(ctx, in.StorageID)
	if err != nil {
		return nil, fmt.Errorf("app: diagnose governance: list formats: %w", err)
	}
	// Merge format info into file instances (same as CLI).
	byKey := make(map[string]int, len(files))
	for i := range files {
		byKey[files[i].StorageID+"\x00"+files[i].Path] = i
	}
	for _, record := range formats {
		if i, ok := byKey[record.StorageID+"\x00"+record.Path]; ok {
			files[i].Format = record.Info
		}
	}
	report := governancediag.Build(files, in.LargeMediaMinimum, time.Now())
	if report.Summary.NonDraftPlans != 0 || report.ExecutionAuthorized {
		return nil, fmt.Errorf("app: governance diagnostic refused unsafe non-draft result")
	}
	return &report, nil
}

// DiagnoseMergesInput defines parameters for a merge diagnostic run.
type DiagnoseMergesInput struct {
	// StorageID filters files by storage ID (empty = all storages).
	StorageID string
}

// DiagnoseMerges builds a merge gate review report from the project
// database. It identifies sibling directories with similar names and
// evaluates filename overlap using the Jaccard similarity.
func (s *DiagnosticService) DiagnoseMerges(ctx context.Context, in DiagnoseMergesInput) (*merge.DiagnosticReport, error) {
	files, err := s.store.ListFiles(ctx, in.StorageID)
	if err != nil {
		return nil, fmt.Errorf("app: diagnose merges: list files: %w", err)
	}
	report := merge.Diagnose(files, time.Now())
	return &report, nil
}
