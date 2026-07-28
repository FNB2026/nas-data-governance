package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// ---- AnalyzeService ----

func TestAnalyzeService_JSONOnlyMode(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "data")
	os.MkdirAll(root, 0o755)
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0o644)
	os.WriteFile(filepath.Join(root, "b.txt"), []byte("world"), 0o644)

	files := []domain.FileInstance{
		{StorageID: "test", Path: filepath.Join(root, "a.txt"), Name: "a.txt"},
		{StorageID: "test", Path: filepath.Join(root, "b.txt"), Name: "b.txt"},
	}

	svc := NewAnalyzeService(nil) // no DB
	result, err := svc.Analyze(context.Background(), AnalyzeInput{
		Files:     files,
		Workers:   1,
		BatchSize: 500,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result.Entries))
	}
	if result.Failed != 0 {
		t.Fatalf("expected 0 failed, got %d", result.Failed)
	}
	if result.Reused != 0 {
		t.Fatalf("expected 0 reused, got %d", result.Reused)
	}
	if result.Persisted != 0 {
		t.Fatalf("JSON-only mode should not persist, got %d", result.Persisted)
	}
}

func TestAnalyzeService_WithMockAnalyzeFunc(t *testing.T) {
	files := []domain.FileInstance{
		{StorageID: "s1", Path: "/data/a.txt", Name: "a.txt"},
		{StorageID: "s1", Path: "/data/b.txt", Name: "b.txt"},
		{StorageID: "s1", Path: "/data/c.txt", Name: "c.txt"},
	}

	// Mock analyze function: returns known formats for .txt, unknown for others.
	mockFn := func(path string) (domain.FormatInfo, error) {
		return domain.FormatInfo{Format: "txt", Category: domain.CategoryDocument}, nil
	}

	svc := NewAnalyzeServiceWithFunc(nil, mockFn)
	result, err := svc.Analyze(context.Background(), AnalyzeInput{
		Files:     files,
		Workers:   2,
		BatchSize: 500,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if result.Analyzed != 3 {
		t.Fatalf("expected 3 analyzed, got %d", result.Analyzed)
	}
	if result.Unrecognized != 0 {
		t.Fatalf("expected 0 unrecognized, got %d", result.Unrecognized)
	}
	if result.Failed != 0 {
		t.Fatalf("expected 0 failed, got %d", result.Failed)
	}
	for i, entry := range result.Entries {
		if entry.Format.Format != "txt" {
			t.Fatalf("entry %d: expected format txt, got %s", i, entry.Format.Format)
		}
		if entry.Error != "" {
			t.Fatalf("entry %d: unexpected error %s", i, entry.Error)
		}
	}
}

func TestAnalyzeService_MixedResults(t *testing.T) {
	files := []domain.FileInstance{
		{StorageID: "s1", Path: "/data/a.txt", Name: "a.txt"},
		{StorageID: "s1", Path: "/data/b.txt", Name: "b.txt"},
		{StorageID: "s1", Path: "/data/c.txt", Name: "c.txt"},
	}

	callCount := 0
	mockFn := func(path string) (domain.FormatInfo, error) {
		callCount++
		if path == "/data/b.txt" {
			return domain.FormatInfo{}, os.ErrNotExist // simulate failure
		}
		if path == "/data/c.txt" {
			return domain.FormatInfo{Format: "unknown", Category: domain.CategoryUnknown}, nil
		}
		return domain.FormatInfo{Format: "txt", Category: domain.CategoryDocument}, nil
	}

	svc := NewAnalyzeServiceWithFunc(nil, mockFn)
	result, err := svc.Analyze(context.Background(), AnalyzeInput{
		Files:     files,
		Workers:   1,
		BatchSize: 500,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if result.Analyzed != 1 {
		t.Fatalf("expected 1 analyzed, got %d", result.Analyzed)
	}
	if result.Unrecognized != 1 {
		t.Fatalf("expected 1 unrecognized, got %d", result.Unrecognized)
	}
	if result.Failed != 1 {
		t.Fatalf("expected 1 failed, got %d", result.Failed)
	}
	if callCount != 3 {
		t.Fatalf("expected 3 analyze calls, got %d", callCount)
	}
}

func TestAnalyzeService_StorageIDFilter(t *testing.T) {
	files := []domain.FileInstance{
		{StorageID: "s1", Path: "/data/a.txt", Name: "a.txt"},
		{StorageID: "s2", Path: "/data/b.txt", Name: "b.txt"},
		{StorageID: "s1", Path: "/data/c.txt", Name: "c.txt"},
	}

	mockFn := func(path string) (domain.FormatInfo, error) {
		return domain.FormatInfo{Format: "txt", Category: domain.CategoryDocument}, nil
	}

	svc := NewAnalyzeServiceWithFunc(nil, mockFn)
	result, err := svc.Analyze(context.Background(), AnalyzeInput{
		Files:     files,
		StorageID: "s1",
		Workers:   1,
		BatchSize: 500,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("expected 2 entries after storage filter, got %d", len(result.Entries))
	}
	if result.Analyzed != 2 {
		t.Fatalf("expected 2 analyzed, got %d", result.Analyzed)
	}
}

func TestAnalyzeService_Limit(t *testing.T) {
	files := make([]domain.FileInstance, 10)
	for i := range files {
		files[i] = domain.FileInstance{
			StorageID: "s1",
			Path:      "/data/file" + string(rune('a'+i)) + ".txt",
			Name:      "file" + string(rune('a'+i)) + ".txt",
		}
	}

	mockFn := func(path string) (domain.FormatInfo, error) {
		return domain.FormatInfo{Format: "txt", Category: domain.CategoryDocument}, nil
	}

	svc := NewAnalyzeServiceWithFunc(nil, mockFn)
	result, err := svc.Analyze(context.Background(), AnalyzeInput{
		Files:     files,
		Limit:     3,
		Workers:   1,
		BatchSize: 500,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(result.Entries) != 3 {
		t.Fatalf("expected 3 entries after limit, got %d", len(result.Entries))
	}
	if result.Analyzed != 3 {
		t.Fatalf("expected 3 analyzed, got %d", result.Analyzed)
	}
}

func TestAnalyzeService_DBPersistence(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "data")
	os.MkdirAll(root, 0o755)
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("content"), 0o644)

	dbPath := filepath.Join(tmp, "governance.db")
	st, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// First: run a scan to populate file_instances in the DB.
	scanSvc := NewScanService(st)
	scanResult, err := scanSvc.Scan(context.Background(), ScanInput{
		Root:           root,
		StorageID:      "test",
		Workers:        1,
		HashAttempts:   3,
		HashRetryDelay: 0,
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(scanResult.Files) != 1 {
		t.Fatalf("expected 1 scanned file, got %d", len(scanResult.Files))
	}

	// Run analyze with DB persistence.
	mockFn := func(path string) (domain.FormatInfo, error) {
		return domain.FormatInfo{Format: "txt", Category: domain.CategoryDocument}, nil
	}
	analyzeSvc := NewAnalyzeServiceWithFunc(st, mockFn)
	result, err := analyzeSvc.Analyze(context.Background(), AnalyzeInput{
		Files:     scanResult.Files,
		Workers:   1,
		BatchSize: 500,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if result.Persisted != 1 {
		t.Fatalf("expected 1 persisted, got %d", result.Persisted)
	}
	if result.LookupFailed != 0 {
		t.Fatalf("expected 0 lookup failures, got %d", result.LookupFailed)
	}

	// Verify format was persisted to DB.
	records, err := st.ListFormats(context.Background(), "test")
	if err != nil {
		t.Fatalf("ListFormats: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 format record in DB, got %d", len(records))
	}
	if records[0].Info.Format != "txt" {
		t.Fatalf("expected format txt, got %s", records[0].Info.Format)
	}
}

func TestAnalyzeService_ResumeReusesDBRecords(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "data")
	os.MkdirAll(root, 0o755)
	srcFile := filepath.Join(root, "sample.txt")
	os.WriteFile(srcFile, []byte("content"), 0o644)

	dbPath := filepath.Join(tmp, "governance.db")
	st, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// Scan to populate file_instances.
	scanSvc := NewScanService(st)
	scanResult, err := scanSvc.Scan(context.Background(), ScanInput{
		Root:           root,
		StorageID:      "test",
		Workers:        1,
		HashAttempts:   3,
		HashRetryDelay: 0,
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	// First analyze: persist format records.
	callCount := 0
	mockFn := func(path string) (domain.FormatInfo, error) {
		callCount++
		return domain.FormatInfo{Format: "txt", Category: domain.CategoryDocument}, nil
	}
	analyzeSvc := NewAnalyzeServiceWithFunc(st, mockFn)
	_, err = analyzeSvc.Analyze(context.Background(), AnalyzeInput{
		Files:     scanResult.Files,
		Workers:   1,
		BatchSize: 500,
	})
	if err != nil {
		t.Fatalf("first analyze: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 analyze call, got %d", callCount)
	}

	// Remove the source file to prove resume doesn't read it.
	os.Remove(srcFile)

	// Second analyze with resume: should reuse DB records, not read files.
	callCount = 0
	result, err := analyzeSvc.Analyze(context.Background(), AnalyzeInput{
		Files:     scanResult.Files,
		Resume:    true,
		Workers:   1,
		BatchSize: 500,
	})
	if err != nil {
		t.Fatalf("resume analyze: %v", err)
	}
	if callCount != 0 {
		t.Fatalf("resume should not call analyze function, got %d calls", callCount)
	}
	if result.Reused != 1 {
		t.Fatalf("expected 1 reused, got %d", result.Reused)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.Entries))
	}
	if result.Entries[0].Format.Format != "txt" {
		t.Fatalf("expected format txt from DB, got %s", result.Entries[0].Format.Format)
	}
}

func TestAnalyzeService_RefreshMetadataReanalyzesGaps(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "data")
	os.MkdirAll(root, 0o755)
	srcFile := filepath.Join(root, "sample.wav")
	// Minimal WAV header (44 bytes).
	wav := make([]byte, 44)
	copy(wav[:4], "RIFF")
	copy(wav[8:12], "WAVE")
	copy(wav[12:16], "fmt ")
	os.WriteFile(srcFile, wav, 0o644)

	dbPath := filepath.Join(tmp, "governance.db")
	st, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// Scan to populate file_instances.
	scanSvc := NewScanService(st)
	scanResult, err := scanSvc.Scan(context.Background(), ScanInput{
		Root:           root,
		StorageID:      "test",
		Workers:        1,
		HashAttempts:   3,
		HashRetryDelay: 0,
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	// First analyze: persist a WAV record with missing duration.
	callCount := 0
	mockFn := func(path string) (domain.FormatInfo, error) {
		callCount++
		return domain.FormatInfo{Format: "wav", Category: domain.CategoryAudio, Duration: 0}, nil
	}
	analyzeSvc := NewAnalyzeServiceWithFunc(st, mockFn)
	_, err = analyzeSvc.Analyze(context.Background(), AnalyzeInput{
		Files:     scanResult.Files,
		Workers:   1,
		BatchSize: 500,
	})
	if err != nil {
		t.Fatalf("first analyze: %v", err)
	}

	// Second analyze with resume + refresh-metadata: should re-analyze the WAV.
	callCount = 0
	updatedFn := func(path string) (domain.FormatInfo, error) {
		callCount++
		return domain.FormatInfo{Format: "wav", Category: domain.CategoryAudio, Duration: 2, Codec: "pcm"}, nil
	}
	analyzeSvc2 := NewAnalyzeServiceWithFunc(st, updatedFn)
	result, err := analyzeSvc2.Analyze(context.Background(), AnalyzeInput{
		Files:           scanResult.Files,
		Resume:          true,
		RefreshMetadata: true,
		Workers:         1,
		BatchSize:       500,
	})
	if err != nil {
		t.Fatalf("refresh analyze: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("refresh-metadata should re-analyze 1 file, got %d calls", callCount)
	}
	if result.Reused != 0 {
		t.Fatalf("expected 0 reused (all refreshed), got %d", result.Reused)
	}

	// Verify updated record was persisted.
	records, err := st.ListFormats(context.Background(), "test")
	if err != nil {
		t.Fatalf("ListFormats: %v", err)
	}
	if len(records) != 1 || records[0].Info.Duration != 2 {
		t.Fatalf("expected duration 2 in DB, got %#v", records)
	}
}

func TestAnalyzeService_ValidationErrors(t *testing.T) {
	svc := NewAnalyzeService(nil)

	// Workers out of range.
	_, err := svc.Analyze(context.Background(), AnalyzeInput{
		Files:     nil,
		Workers:   0,
		BatchSize: 500,
	})
	if err == nil {
		t.Fatal("expected error for workers=0")
	}

	// BatchSize out of range.
	_, err = svc.Analyze(context.Background(), AnalyzeInput{
		Files:     nil,
		Workers:   1,
		BatchSize: 0,
	})
	if err == nil {
		t.Fatal("expected error for batch-size=0")
	}

	// Resume without store.
	_, err = svc.Analyze(context.Background(), AnalyzeInput{
		Files:     nil,
		Workers:   1,
		BatchSize: 500,
		Resume:    true,
	})
	if err == nil {
		t.Fatal("expected error for resume without store")
	}

	// RefreshUnknown without resume.
	_, err = svc.Analyze(context.Background(), AnalyzeInput{
		Files:          nil,
		Workers:        1,
		BatchSize:      500,
		RefreshUnknown: true,
	})
	if err == nil {
		t.Fatal("expected error for refresh-unknown without resume")
	}

	// RefreshMetadata without resume.
	_, err = svc.Analyze(context.Background(), AnalyzeInput{
		Files:           nil,
		Workers:         1,
		BatchSize:       500,
		RefreshMetadata: true,
	})
	if err == nil {
		t.Fatal("expected error for refresh-metadata without resume")
	}
}

func TestAnalyzeService_ProgressTracking(t *testing.T) {
	files := make([]domain.FileInstance, 5)
	for i := range files {
		files[i] = domain.FileInstance{
			StorageID: "s1",
			Path:      "/data/file" + string(rune('a'+i)) + ".txt",
			Name:      "file" + string(rune('a'+i)) + ".txt",
		}
	}

	mockFn := func(path string) (domain.FormatInfo, error) {
		return domain.FormatInfo{Format: "txt", Category: domain.CategoryDocument}, nil
	}

	svc := NewAnalyzeServiceWithFunc(nil, mockFn)

	// Before analyze, stage should be idle.
	p := svc.Progress()
	if p.Stage != "idle" {
		t.Fatalf("expected idle stage, got %s", p.Stage)
	}

	result, err := svc.Analyze(context.Background(), AnalyzeInput{
		Files:     files,
		Workers:   1,
		BatchSize: 500,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	// After analyze, stage should be completed.
	p = svc.Progress()
	if p.Stage != "completed" {
		t.Fatalf("expected completed stage, got %s", p.Stage)
	}
	if p.Total != 5 {
		t.Fatalf("expected total 5, got %d", p.Total)
	}
	if p.Processed != 5 {
		t.Fatalf("expected processed 5, got %d", p.Processed)
	}

	_ = result // result already validated above
}

// ---- helper function unit tests ----

func TestIsUnknownFormat(t *testing.T) {
	tests := []struct {
		name string
		info domain.FormatInfo
		want bool
	}{
		{"empty format", domain.FormatInfo{}, true},
		{"unknown format", domain.FormatInfo{Format: "unknown", Category: domain.CategoryUnknown}, true},
		{"unknown category", domain.FormatInfo{Format: "txt", Category: domain.CategoryUnknown}, true},
		{"recognized", domain.FormatInfo{Format: "txt", Category: domain.CategoryDocument}, false},
		{"recognized audio", domain.FormatInfo{Format: "wav", Category: domain.CategoryAudio}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsUnknownFormat(tt.info); got != tt.want {
				t.Errorf("IsUnknownFormat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNeedsMetadataRefresh(t *testing.T) {
	tests := []struct {
		name string
		info domain.FormatInfo
		want bool
	}{
		{"wav missing duration", domain.FormatInfo{Format: "wav", Category: domain.CategoryAudio}, true},
		{"wav with duration", domain.FormatInfo{Format: "wav", Category: domain.CategoryAudio, Duration: 2}, false},
		{"mp4 missing dimensions", domain.FormatInfo{Format: "mp4", Category: domain.CategoryVideo, Duration: 2}, true},
		{"mp4 complete", domain.FormatInfo{Format: "mp4", Category: domain.CategoryVideo, Duration: 2, Width: 1920, Height: 1080}, false},
		{"mpeg missing dimensions", domain.FormatInfo{Format: "mpeg", Category: domain.CategoryVideo}, true},
		{"mpeg with dimensions", domain.FormatInfo{Format: "mpeg", Category: domain.CategoryVideo, Width: 1920, Height: 1080}, false},
		{"flv unsupported", domain.FormatInfo{Format: "flv", Category: domain.CategoryVideo}, false},
		{"txt not applicable", domain.FormatInfo{Format: "txt", Category: domain.CategoryDocument}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NeedsMetadataRefresh(tt.info); got != tt.want {
				t.Errorf("NeedsMetadataRefresh() = %v, want %v", got, tt.want)
			}
		})
	}
}
