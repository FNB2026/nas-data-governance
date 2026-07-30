package wails

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// createProjectDBWithDiagnostics creates a project database with files
// and format records suitable for diagnostic testing:
//   - s1: 3 duplicate files (hash "aaa"), 1 zero-byte file, 1 large video
//   - Format records for the video file and one duplicate
func createProjectDBWithDiagnostics(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "diagnostics.db")
	st, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("create project database: %v", err)
	}
	ctx := context.Background()
	if err := st.RegisterStorage(ctx, domain.Storage{
		ID: "s1", RootPath: "/source1", Kind: "local", CreatedAt: time.Now().UTC(),
	}); err != nil {
		_ = st.Close()
		t.Fatalf("seed storage: %v", err)
	}
	now := time.Now().UTC()
	files := []domain.FileInstance{
		{StorageID: "s1", Path: "/source1/a.txt", Name: "a.txt", Size: 1000, ModifiedAt: now, ContentSHA256: "aaa", DiscoveredAt: now},
		{StorageID: "s1", Path: "/source1/b.txt", Name: "b.txt", Size: 1000, ModifiedAt: now, ContentSHA256: "aaa", DiscoveredAt: now},
		{StorageID: "s1", Path: "/source1/zero.dat", Name: "zero.dat", Size: 0, ModifiedAt: now, ContentSHA256: "zero", DiscoveredAt: now},
		{StorageID: "s1", Path: "/source1/video.mp4", Name: "video.mp4", Size: 200 << 20, ModifiedAt: now, ContentSHA256: "vid", DiscoveredAt: now},
	}
	fileIDs, err := st.UpsertFiles(ctx, files)
	if err != nil {
		_ = st.Close()
		t.Fatalf("seed files: %v", err)
	}
	// Save format for the video file (fileIDs[3]).
	if err := st.SaveFormat(ctx, fileIDs[3], domain.FormatInfo{
		Format: "mp4", Category: domain.CategoryVideo, MIME: "video/mp4",
		Width: 1920, Height: 1080, Duration: 120,
	}); err != nil {
		_ = st.Close()
		t.Fatalf("seed format: %v", err)
	}
	// Save format for the first duplicate (fileIDs[0]).
	if err := st.SaveFormat(ctx, fileIDs[0], domain.FormatInfo{
		Format: "txt", Category: domain.CategoryDocument, MIME: "text/plain",
	}); err != nil {
		_ = st.Close()
		t.Fatalf("seed format: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close project database: %v", err)
	}
	return path
}

// ---- DiagnoseFormats tests ----

func TestDiagnoseFormatsReadOnly(t *testing.T) {
	path := createProjectDBWithDiagnostics(t)
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProject(path); err != nil {
		t.Fatalf("OpenProject (read-only): %v", err)
	}

	report, err := api.DiagnoseFormats(DiagnoseFormatsRequest{})
	if err != nil {
		t.Fatalf("DiagnoseFormats: %v", err)
	}
	if report.Summary.Files != 4 {
		t.Errorf("expected 4 files, got %d", report.Summary.Files)
	}
	if report.Summary.FormatRows != 2 {
		t.Errorf("expected 2 format rows, got %d", report.Summary.FormatRows)
	}
	if report.Summary.MissingFormatRows != 2 {
		t.Errorf("expected 2 missing format rows, got %d", report.Summary.MissingFormatRows)
	}
	// The zero-byte file (0 bytes) is below the large-unknown threshold.
	// The video file has a known format, so it should not appear in LargeUnknown.
	if report.Summary.LargeUnknown != 0 {
		t.Errorf("expected 0 large unknown, got %d", report.Summary.LargeUnknown)
	}
	// Safety notes must be present.
	if len(report.SafetyNotes) == 0 {
		t.Error("expected non-empty safety notes")
	}
}

func TestDiagnoseFormatsReadWrite(t *testing.T) {
	path := createProjectDBWithDiagnostics(t)
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProjectReadWrite(path); err != nil {
		t.Fatalf("OpenProjectReadWrite: %v", err)
	}

	report, err := api.DiagnoseFormats(DiagnoseFormatsRequest{StorageID: "s1"})
	if err != nil {
		t.Fatalf("DiagnoseFormats: %v", err)
	}
	if report.Summary.Files != 4 {
		t.Errorf("expected 4 files, got %d", report.Summary.Files)
	}
}

func TestDiagnoseFormatsNoProjectOpen(t *testing.T) {
	api := NewAPI()
	_, err := api.DiagnoseFormats(DiagnoseFormatsRequest{})
	if err != ErrNoProjectOpen {
		t.Errorf("expected ErrNoProjectOpen, got %v", err)
	}
}

// ---- DiagnoseGovernance tests ----

func TestDiagnoseGovernanceReadOnly(t *testing.T) {
	path := createProjectDBWithDiagnostics(t)
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProject(path); err != nil {
		t.Fatalf("OpenProject (read-only): %v", err)
	}

	report, err := api.DiagnoseGovernance(DiagnoseGovernanceRequest{})
	if err != nil {
		t.Fatalf("DiagnoseGovernance: %v", err)
	}
	if report.Summary.Files != 4 {
		t.Errorf("expected 4 files, got %d", report.Summary.Files)
	}
	// 2 files share hash "aaa" → 1 duplicate group.
	if report.Summary.DuplicateGroups != 1 {
		t.Errorf("expected 1 duplicate group, got %d", report.Summary.DuplicateGroups)
	}
	// 1 zero-byte file.
	if report.Summary.ZeroByteFiles != 1 {
		t.Errorf("expected 1 zero-byte file, got %d", report.Summary.ZeroByteFiles)
	}
	// 1 large media file (video.mp4 at 200 MiB).
	if report.Summary.LargeMediaFiles != 1 {
		t.Errorf("expected 1 large media file, got %d", report.Summary.LargeMediaFiles)
	}
	// All plans must be DRAFT.
	if report.Summary.NonDraftPlans != 0 {
		t.Errorf("expected 0 non-draft plans, got %d", report.Summary.NonDraftPlans)
	}
	if report.ExecutionAuthorized {
		t.Error("execution must not be authorized")
	}
}

func TestDiagnoseGovernanceNoProjectOpen(t *testing.T) {
	api := NewAPI()
	_, err := api.DiagnoseGovernance(DiagnoseGovernanceRequest{})
	if err != ErrNoProjectOpen {
		t.Errorf("expected ErrNoProjectOpen, got %v", err)
	}
}

// ---- DiagnoseMerges tests ----

func TestDiagnoseMergesReadOnly(t *testing.T) {
	path := createProjectDBWithDiagnostics(t)
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProject(path); err != nil {
		t.Fatalf("OpenProject (read-only): %v", err)
	}

	report, err := api.DiagnoseMerges(DiagnoseMergesRequest{})
	if err != nil {
		t.Fatalf("DiagnoseMerges: %v", err)
	}
	if report.Summary.Files != 4 {
		t.Errorf("expected 4 files, got %d", report.Summary.Files)
	}
	// All files are in /source1, so there's 1 directory.
	if report.Summary.Directories != 1 {
		t.Errorf("expected 1 directory, got %d", report.Summary.Directories)
	}
	if report.ExecutionAuthorized {
		t.Error("execution must not be authorized")
	}
}

func TestDiagnoseMergesNoProjectOpen(t *testing.T) {
	api := NewAPI()
	_, err := api.DiagnoseMerges(DiagnoseMergesRequest{})
	if err != ErrNoProjectOpen {
		t.Errorf("expected ErrNoProjectOpen, got %v", err)
	}
}

// ---- V5 no-project-open tests (batch) ----

func TestV5DiagnosticMethodsNoProjectOpen(t *testing.T) {
	api := NewAPI()
	if _, err := api.DiagnoseFormats(DiagnoseFormatsRequest{}); err != ErrNoProjectOpen {
		t.Errorf("DiagnoseFormats: expected ErrNoProjectOpen, got %v", err)
	}
	if _, err := api.DiagnoseGovernance(DiagnoseGovernanceRequest{}); err != ErrNoProjectOpen {
		t.Errorf("DiagnoseGovernance: expected ErrNoProjectOpen, got %v", err)
	}
	if _, err := api.DiagnoseMerges(DiagnoseMergesRequest{}); err != ErrNoProjectOpen {
		t.Errorf("DiagnoseMerges: expected ErrNoProjectOpen, got %v", err)
	}
}
