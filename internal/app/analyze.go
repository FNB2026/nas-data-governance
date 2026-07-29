package app

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/format"
	"github.com/FNB2026/nas-data-governance/internal/runner"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// AnalyzeStore is the store subset needed by AnalyzeService for resume
// and batch persistence of format records.
// store.SQLiteStore satisfies this interface structurally.
type AnalyzeStore interface {
	ListFormats(ctx context.Context, storageID string) ([]store.FormatRecord, error)
	SaveFormatsByPath(ctx context.Context, records []store.FormatRecord) (int, int, error)
}

// AnalyzeFunc analyzes a file's format by reading its headers (K-006).
type AnalyzeFunc func(path string) (domain.FormatInfo, error)

// FormatReportEntry is one row in the analyze output.
type FormatReportEntry struct {
	Path      string            `json:"path"`
	StorageID string            `json:"storage_id"`
	Format    domain.FormatInfo `json:"format"`
	Error     string            `json:"error,omitempty"`
}

// AnalyzeInput defines parameters for a format analysis run.
type AnalyzeInput struct {
	// Files is the list of file instances to analyze (required).
	Files []domain.FileInstance
	// StorageID filters files by storage ID (empty = no filter).
	StorageID string
	// Limit caps the number of files to analyze (0 = all).
	Limit int
	// Workers is the number of concurrent analysis workers (1 = serial).
	Workers int
	// Resume reuses completed format rows from the store and skips their reads.
	Resume bool
	// RefreshUnknown, with Resume, re-analyzes previously unknown records.
	RefreshUnknown bool
	// RefreshMetadata, with Resume, re-analyzes supported media records with missing metadata.
	RefreshMetadata bool
	// BatchSize is the number of format records per SQLite transaction.
	BatchSize int
}

// AnalyzeProgress is a point-in-time snapshot of analysis progress.
// Safe to read from a separate goroutine while Analyze is running.
type AnalyzeProgress struct {
	Stage     string `json:"stage"`
	Processed int64  `json:"processed"`
	Total     int64  `json:"total"`
	Failed    int64  `json:"failed"`
	Reused    int64  `json:"reused"`
}

// AnalyzeResult holds the outcome of a format analysis run. The CLI
// layer is responsible for writing the JSON report from Entries and
// printing summary lines from the counters.
type AnalyzeResult struct {
	// Entries is the complete set of format report entries (one per file).
	Entries []FormatReportEntry
	// Analyzed is the count of recognized formats.
	Analyzed int64
	// Unrecognized is the count of unknown formats.
	Unrecognized int64
	// Failed is the count of files that failed analysis.
	Failed int64
	// Reused is the count of format records reused from the DB (resume mode).
	Reused int64
	// Persisted is the count of new format records saved to the DB.
	Persisted int64
	// LookupFailed is the count of records that had no matching DB row.
	LookupFailed int64
	// PersistErr is the error from the persistence goroutine, if any.
	PersistErr error
}

// AnalyzeService handles header-only format analysis of files with
// optional DB persistence and resume capability. It is read-only with
// respect to user data: it only reads file headers and writes its own
// report/database.
//
// The service does NOT:
//   - Parse command-line flags (CLI's job)
//   - Write JSON report files (CLI's job)
//   - Write progress files (CLI's job — it polls Progress() via a ticker)
//   - Print to stdout/stderr (CLI's job)
type AnalyzeService struct {
	store   AnalyzeStore // nil for JSON-only mode
	analyze AnalyzeFunc

	// Internal progress counters. Read by Progress() from any goroutine.
	stage     atomic.Value // string
	processed atomic.Int64
	total     atomic.Int64
	failed    atomic.Int64
	reused    atomic.Int64
}

// NewAnalyzeService creates a new analyze service with the default
// format.Analyze function. The store may be nil for analysis without
// DB persistence.
func NewAnalyzeService(st AnalyzeStore) *AnalyzeService {
	s := &AnalyzeService{store: st, analyze: format.Analyze}
	s.stage.Store("idle")
	return s
}

// NewAnalyzeServiceWithFunc creates a new analyze service with a custom
// analyze function, primarily for testing.
func NewAnalyzeServiceWithFunc(st AnalyzeStore, fn AnalyzeFunc) *AnalyzeService {
	s := &AnalyzeService{store: st, analyze: fn}
	s.stage.Store("idle")
	return s
}

// Progress returns a snapshot of the current analysis progress. Safe
// to call from a separate goroutine while Analyze is running. The CLI
// layer can poll this on a ticker to write progress files.
func (s *AnalyzeService) Progress() AnalyzeProgress {
	return AnalyzeProgress{
		Stage:     s.stage.Load().(string),
		Processed: s.processed.Load(),
		Total:     s.total.Load(),
		Failed:    s.failed.Load(),
		Reused:    s.reused.Load(),
	}
}

// Analyze runs header-only format analysis on the input files.
//
// The pipeline:
//  1. Filter by StorageID and apply Limit
//  2. Resume: load existing format rows from DB, separate reused vs pending
//  3. Concurrent analysis via worker pool (each worker reads file headers)
//  4. Separate persistence goroutine batches SQLite writes
//  5. Return structured result for CLI to write report and print summary
func (s *AnalyzeService) Analyze(ctx context.Context, in AnalyzeInput) (*AnalyzeResult, error) {
	if in.Workers < 1 || in.Workers > 64 {
		return nil, fmt.Errorf("app: Analyze: workers must be between 1 and 64")
	}
	if in.BatchSize < 1 || in.BatchSize > 10000 {
		return nil, fmt.Errorf("app: Analyze: batch-size must be between 1 and 10000")
	}
	if in.Resume && s.store == nil {
		return nil, fmt.Errorf("app: Analyze: --resume requires a store")
	}
	if in.RefreshUnknown && !in.Resume {
		return nil, fmt.Errorf("app: Analyze: --refresh-unknown requires --resume")
	}
	if in.RefreshMetadata && !in.Resume {
		return nil, fmt.Errorf("app: Analyze: --refresh-metadata requires --resume")
	}

	// Reset progress counters.
	s.processed.Store(0)
	s.total.Store(0)
	s.failed.Store(0)
	s.reused.Store(0)
	s.stage.Store("format_analysis")

	// Filter by StorageID.
	files := in.Files
	if in.StorageID != "" {
		filtered := make([]domain.FileInstance, 0, len(files))
		for _, f := range files {
			if f.StorageID == in.StorageID {
				filtered = append(filtered, f)
			}
		}
		files = filtered
	}

	// Apply limit.
	if in.Limit > 0 && len(files) > in.Limit {
		files = files[:in.Limit]
	}

	s.total.Store(int64(len(files)))

	result := &AnalyzeResult{
		Entries: make([]FormatReportEntry, len(files)),
	}

	// Resume: load existing format rows from DB. Resume is based on durable
	// format rows, not a fragile path cursor. This safely handles worker
	// completion order and makes repeated runs idempotent.
	existing := map[string]domain.FormatInfo{}
	if in.Resume {
		records, err := s.store.ListFormats(ctx, in.StorageID)
		if err != nil {
			return nil, fmt.Errorf("app: load existing formats: %w", err)
		}
		for _, record := range records {
			existing[formatRecordKey(record.StorageID, record.Path)] = record.Info
		}
	}

	// Separate reused vs pending files.
	type pendingFile struct {
		index int
		file  domain.FileInstance
	}
	pending := make([]pendingFile, 0, len(files))
	for i, file := range files {
		if info, ok := existing[formatRecordKey(file.StorageID, file.Path)]; ok &&
			!(in.RefreshUnknown && IsUnknownFormat(info)) &&
			!(in.RefreshMetadata && NeedsMetadataRefresh(info)) {
			result.Entries[i] = FormatReportEntry{Path: file.Path, StorageID: file.StorageID, Format: info}
			atomic.AddInt64(&result.Reused, 1)
			if IsUnknownFormat(info) {
				atomic.AddInt64(&result.Unrecognized, 1)
			} else {
				atomic.AddInt64(&result.Analyzed, 1)
			}
			continue
		}
		pending = append(pending, pendingFile{index: i, file: file})
	}
	s.reused.Store(atomic.LoadInt64(&result.Reused))

	// Persistence goroutine: batches SQLite writes. Never writes to the NAS.
	var persistCh chan store.FormatRecord
	var persistWG sync.WaitGroup
	var persistMu sync.Mutex
	if s.store != nil {
		persistCh = make(chan store.FormatRecord, in.BatchSize*2)
		persistWG.Add(1)
		go func() {
			defer persistWG.Done()
			batch := make([]store.FormatRecord, 0, in.BatchSize)
			flush := func() {
				if len(batch) == 0 {
					return
				}
				persistMu.Lock()
				failedAlready := result.PersistErr != nil
				persistMu.Unlock()
				if !failedAlready {
					saved, missing, err := s.store.SaveFormatsByPath(context.Background(), batch)
					persistMu.Lock()
					result.Persisted += int64(saved)
					result.LookupFailed += int64(missing)
					if err != nil && result.PersistErr == nil {
						result.PersistErr = err
					}
					persistMu.Unlock()
				}
				batch = batch[:0]
			}
			for record := range persistCh {
				batch = append(batch, record)
				if len(batch) >= in.BatchSize {
					flush()
				}
			}
			flush()
		}()
	}

	// Concurrent format analysis via worker pool.
	var entriesMu sync.Mutex
	var processedNew int64
	ar := runner.New(in.Workers)
	var submitErr error
	for _, item := range pending {
		idx := item.index
		file := item.file
		if err := ar.Submit(ctx, func() error {
			info, analyzeErr := s.analyze(file.Path)
			entry := FormatReportEntry{Path: file.Path, StorageID: file.StorageID, Format: info}
			if analyzeErr != nil {
				entry.Error = analyzeErr.Error()
				atomic.AddInt64(&result.Failed, 1)
			} else if IsUnknownFormat(info) {
				atomic.AddInt64(&result.Unrecognized, 1)
			} else {
				atomic.AddInt64(&result.Analyzed, 1)
			}
			if persistCh != nil && analyzeErr == nil && info.Format != "" {
				persistCh <- store.FormatRecord{StorageID: file.StorageID, Path: file.Path, Info: info}
			}
			entriesMu.Lock()
			result.Entries[idx] = entry
			entriesMu.Unlock()
			done := atomic.LoadInt64(&result.Reused) + atomic.AddInt64(&processedNew, 1)
			s.processed.Store(done)
			return nil
		}); err != nil {
			submitErr = err
			break
		}
	}
	ar.Wait()

	// Flush persistence.
	s.stage.Store("persisting")
	if persistCh != nil {
		close(persistCh)
		persistWG.Wait()
	}

	// Check completeness.
	completed := atomic.LoadInt64(&result.Reused) + atomic.LoadInt64(&processedNew)
	if completed != int64(len(files)) {
		s.stage.Store("interrupted")
		if submitErr != nil {
			return nil, submitErr
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("app: analysis incomplete: %d/%d", completed, len(files))
	}

	s.failed.Store(atomic.LoadInt64(&result.Failed))
	s.stage.Store("completed")
	return result, nil
}

// ---- helper functions (business logic, shared with CLI) ----

// formatRecordKey builds a deterministic key for a format record.
func formatRecordKey(storageID, path string) string {
	return storageID + "\x00" + path
}

// IsUnknownFormat returns true if the format info represents an
// unrecognized or empty format.
func IsUnknownFormat(info domain.FormatInfo) bool {
	return info.Format == "" || info.Format == "unknown" || info.Category == domain.CategoryUnknown
}

// NeedsMetadataRefresh is deliberately capability-scoped. Formats whose
// parser cannot currently fill a missing field are not retried forever.
func NeedsMetadataRefresh(info domain.FormatInfo) bool {
	switch info.Format {
	case "wav", "aiff", "flac", "m4a":
		return info.Duration <= 0
	case "mp4", "mov", "m4v", "avi":
		return info.Duration <= 0 || info.Width <= 0 || info.Height <= 0
	case "mpeg":
		return info.Width <= 0 || info.Height <= 0
	default:
		return false
	}
}
