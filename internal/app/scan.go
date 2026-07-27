package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/dircontext"
	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/fingerprint"
	"github.com/FNB2026/nas-data-governance/internal/runner"
	"github.com/FNB2026/nas-data-governance/internal/scanner"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// ScanStore is the store subset needed by ScanService for incremental
// hash reuse, checkpoint management, and DB persistence.
// store.SQLiteStore satisfies this interface structurally.
type ScanStore interface {
	RegisterStorage(ctx context.Context, st domain.Storage) error
	ListFileMetadata(ctx context.Context, storageID string) ([]store.FileMeta, error)
	StartCheckpoint(ctx context.Context, storageID string) (int64, error)
	LastCheckpoint(ctx context.Context, storageID string) (store.Checkpoint, error)
	UpdateCheckpoint(ctx context.Context, checkpointID int64, lastPath string, scannedCount int) error
	CompleteCheckpoint(ctx context.Context, checkpointID int64, status string) error
	UpsertFiles(ctx context.Context, files []domain.FileInstance) ([]int64, error)
	SaveContext(ctx context.Context, fileID int64, c domain.DirectoryContext, ruleVersion string) error
	MarkFilesMissing(ctx context.Context, storageID string, paths []string) (int64, error)
	MarkFilesUnavailable(ctx context.Context, storageID string, paths []string) (int64, error)
}

// HashFunc computes a fingerprint for a file given its path and size.
type HashFunc func(path string, size int64) (string, error)

// HashFailure records a file whose hash could not be computed. The CLI
// layer writes these to a private manifest; the service returns them
// as part of ScanResult.
type HashFailure struct {
	ID           string    `json:"id"`
	Stage        string    `json:"stage"`
	StorageID    string    `json:"storage_id"`
	Path         string    `json:"path"`
	Size         int64     `json:"size"`
	ModifiedAt   time.Time `json:"modified_at"`
	Device       uint64    `json:"device"`
	Inode        uint64    `json:"inode"`
	Attempts     int       `json:"attempts"`
	Status       string    `json:"status"`
	DiscoveredAt time.Time `json:"discovered_at"`
}

// NewHashFailure creates a HashFailure from a file instance. The ID is
// a deterministic SHA-256 of (storageID, path) so retries can correlate
// entries across manifests without embedding sensitive data in logs.
func NewHashFailure(file domain.FileInstance, stage string, attempts int, status string) HashFailure {
	sum := sha256.Sum256([]byte(file.StorageID + "\x00" + file.Path))
	return HashFailure{
		ID:           hex.EncodeToString(sum[:]),
		Stage:        stage,
		StorageID:    file.StorageID,
		Path:         file.Path,
		Size:         file.Size,
		ModifiedAt:   file.ModifiedAt,
		Device:       file.Device,
		Inode:        file.Inode,
		Attempts:     attempts,
		Status:       status,
		DiscoveredAt: file.DiscoveredAt,
	}
}

// ScanInput defines parameters for a scan run.
type ScanInput struct {
	// Root is the root directory to scan (required).
	Root string
	// StorageID is the storage identifier for DB persistence.
	StorageID string
	// FullScan forces a full scan, ignoring checkpoint and recompute all hashes.
	FullScan bool
	// Resume resumes from the last checkpoint if available.
	Resume bool
	// Workers is the number of concurrent hash workers (1 = serial).
	Workers int
	// HashAttempts is the maximum read attempts per hash (1-10).
	HashAttempts int
	// HashRetryDelay is the delay between hash attempts.
	HashRetryDelay time.Duration
}

// ScanProgress is a point-in-time snapshot of scan progress. Safe to
// read from a separate goroutine while Scan is running.
type ScanProgress struct {
	Stage      string `json:"stage"`
	Discovered int64  `json:"discovered"`
	Processed  int64  `json:"processed"`
	Failed     int64  `json:"failed"`
}

// ScanResult holds the outcome of a scan. The CLI layer is responsible
// for writing the JSONL index and the hash-failure manifest from this data.
type ScanResult struct {
	// Files is the complete set of scanned file instances with hashes.
	Files []domain.FileInstance
	// HashFailures records files whose hashes could not be computed.
	HashFailures []HashFailure
	// ScanErrors is the count of non-fatal filesystem errors during traversal.
	ScanErrors int
	// Missing is the count of files marked missing (only when store is used
	// and traversal was complete).
	Missing int64
	// Unavailable is the count of files marked unavailable (partial traversal).
	Unavailable int64
	// CheckpointID is the checkpoint ID used (0 when no store).
	CheckpointID int64
	// ResumedFrom is the checkpoint resume path (empty when not resuming).
	ResumedFrom string
	// ResumedCount is the file count from the resumed checkpoint (0 when not resuming).
	ResumedCount int
	// FullTraversal is true when the scan completed without errors,
	// meaning MarkFilesMissing was used instead of MarkFilesUnavailable.
	FullTraversal bool
}

// ScanService handles filesystem scanning with incremental hash reuse,
// two-stage progressive fingerprinting, and DB persistence. It is the
// most complex application service because it orchestrates traversal,
// concurrent hashing, cache management, and reconciliation.
//
// The service does NOT:
//   - Parse command-line flags (CLI's job)
//   - Write JSONL index files (CLI's job)
//   - Write hash-failure manifests (CLI's job)
//   - Write progress files (CLI's job — it polls Progress() via a ticker)
//   - Print to stdout/stderr (CLI's job)
type ScanService struct {
	store     ScanStore // nil for JSONL-only mode
	quickHash HashFunc
	fullHash  HashFunc

	// Internal progress counters. Read by Progress() from any goroutine.
	stage      atomic.Value // string
	discovered atomic.Int64
	processed  atomic.Int64
	failed     atomic.Int64
}

// NewScanService creates a scan service with default hash functions
// (fingerprint.Quick and fingerprint.Full). The store may be nil for
// JSONL-only scans without DB persistence.
func NewScanService(st ScanStore) *ScanService {
	s := &ScanService{
		store:     st,
		quickHash: fingerprint.Quick,
		fullHash:  func(path string, _ int64) (string, error) { return fingerprint.Full(path) },
	}
	s.stage.Store("idle")
	return s
}

// NewScanServiceWithHashFunc creates a scan service with custom hash
// functions, primarily for testing.
func NewScanServiceWithHashFunc(st ScanStore, quick, full HashFunc) *ScanService {
	s := &ScanService{
		store:     st,
		quickHash: quick,
		fullHash:  full,
	}
	s.stage.Store("idle")
	return s
}

// Progress returns a snapshot of the current scan progress. Safe to
// call from a separate goroutine while Scan is running. The CLI layer
// can poll this on a ticker to write progress files.
func (s *ScanService) Progress() ScanProgress {
	return ScanProgress{
		Stage:      s.stage.Load().(string),
		Discovered: s.discovered.Load(),
		Processed:  s.processed.Load(),
		Failed:     s.failed.Load(),
	}
}

// Scan runs the full scan pipeline:
//
//  1. Load incremental cache from DB (if store && !fullScan)
//  2. Resume from checkpoint (if resume && !fullScan)
//  3. Traverse filesystem, computing quick hashes (concurrent)
//  4. Second-stage: compute full SHA-256 for duplicate candidates
//  5. Persist to DB: UpsertFiles, SaveContext, MarkMissing/Unavailable
//  6. Complete checkpoint
//
// The returned ScanResult contains all data the CLI needs to write
// JSONL, hash-failure manifests, and print summary lines.
func (s *ScanService) Scan(ctx context.Context, in ScanInput) (*ScanResult, error) {
	if in.Root == "" {
		return nil, fmt.Errorf("app: Scan: --root is required")
	}
	if in.HashAttempts < 1 || in.HashAttempts > 10 {
		return nil, fmt.Errorf("app: Scan: --hash-attempts must be between 1 and 10")
	}
	if in.HashRetryDelay < 0 || in.HashRetryDelay > 30*time.Second {
		return nil, fmt.Errorf("app: Scan: --hash-retry-delay must be between 0 and 30s")
	}

	// Reset progress counters.
	s.discovered.Store(0)
	s.processed.Store(0)
	s.failed.Store(0)
	s.stage.Store("traversal")

	rootPath, err := filepath.Abs(in.Root)
	if err != nil {
		return nil, err
	}

	result := &ScanResult{}

	// Register storage in DB if store is available.
	if s.store != nil {
		if err := s.store.RegisterStorage(ctx, domain.Storage{
			ID: in.StorageID, RootPath: rootPath, Kind: "filesystem", CreatedAt: time.Now().UTC(),
		}); err != nil {
			return nil, err
		}
	}

	// Load existing file metadata for incremental hash reuse.
	cache := map[string]store.FileMeta{}
	if s.store != nil && !in.FullScan {
		existing, err := s.store.ListFileMetadata(ctx, in.StorageID)
		if err != nil {
			return nil, fmt.Errorf("app: load file metadata: %w", err)
		}
		for _, m := range existing {
			cache[m.Path] = m
		}
	}

	// Checkpoint: resume from the last incomplete scan if --resume.
	if s.store != nil {
		if in.Resume && !in.FullScan {
			cp, err := s.store.LastCheckpoint(ctx, in.StorageID)
			if err == nil {
				result.ResumedFrom = cp.LastScannedPath
				result.CheckpointID = cp.ID
				result.ResumedCount = cp.ScannedCount
			} else if !errors.Is(err, store.ErrNotFound) {
				return nil, fmt.Errorf("app: load checkpoint: %w", err)
			}
		}
		if result.CheckpointID == 0 {
			result.CheckpointID, err = s.store.StartCheckpoint(ctx, in.StorageID)
			if err != nil {
				return nil, err
			}
		}
	}

	// Scan with incremental hash reuse.
	var files []domain.FileInstance
	var filesMu sync.Mutex
	var hashFailures []HashFailure
	var failuresMu sync.Mutex

	addFile := func(f domain.FileInstance) {
		filesMu.Lock()
		files = append(files, f)
		filesMu.Unlock()
		s.processed.Add(1)
	}
	addFailure := func(f HashFailure) {
		failuresMu.Lock()
		hashFailures = append(hashFailures, f)
		failuresMu.Unlock()
		s.failed.Add(1)
	}

	hashRunner := runner.New(in.Workers)
	scanOpts := scanner.Options{
		Root:          in.Root,
		StorageID:     in.StorageID,
		ExcludedNames: scanner.DefaultExclusions(),
		ResumePath:    result.ResumedFrom,
	}
	stats, err := scanner.Scan(ctx, scanOpts, func(file domain.FileInstance) error {
		count := s.discovered.Add(1)
		if s.store != nil && result.CheckpointID != 0 && count%1000 == 0 {
			if err := s.store.UpdateCheckpoint(ctx, result.CheckpointID, "", int(count)); err != nil {
				return errors.New("update aggregate scan checkpoint failed")
			}
		}
		// Incremental check: if size + mtime + inode are unchanged,
		// reuse cached hashes instead of recomputing.
		if cached, ok := cache[file.Path]; ok {
			if cached.Size == file.Size &&
				cached.ModifiedAt.Equal(file.ModifiedAt) &&
				cached.Inode == file.Inode && cached.QuickHash != "" {
				file.QuickHash = cached.QuickHash
				file.ContentSHA256 = cached.ContentSHA256
				addFile(file)
				return nil
			}
		}
		// File is new or changed: compute quick hash (possibly concurrent).
		return hashRunner.Submit(ctx, func() error {
			q, used, qerr := hashWithRetry(ctx, file.Path, file.Size, in.HashAttempts, in.HashRetryDelay, s.quickHash)
			if qerr != nil {
				addFile(file)
				addFailure(NewHashFailure(file, "quick", used, "hash_failed"))
				return errors.New("quick fingerprint failed; path omitted")
			}
			file.QuickHash = q
			addFile(file)
			return nil
		})
	})

	s.stage.Store("quick_hash")
	hashErrs := hashRunner.Wait()
	_ = hashErrs // already recorded as hashFailures

	if err != nil {
		if result.CheckpointID != 0 && s.store != nil {
			_ = s.store.CompleteCheckpoint(ctx, result.CheckpointID, "aborted")
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, errors.New("scan traversal failed; source paths omitted")
	}

	// Second-stage hashing: only for files that share size+quick_hash
	// with another file AND don't already have content_sha256 cached.
	bySizeQuick := map[string][]int{}
	for i, f := range files {
		if f.ContentSHA256 == "" && f.QuickHash != "" {
			key := fmt.Sprintf("%d:%s", f.Size, f.QuickHash)
			bySizeQuick[key] = append(bySizeQuick[key], i)
		}
	}
	fullRunner := runner.New(in.Workers)
	for _, indexes := range bySizeQuick {
		if len(indexes) < 2 {
			continue
		}
		for _, i := range indexes {
			idx := i // capture for closure
			fullRunner.Submit(ctx, func() error {
				h, used, ferr := hashWithRetry(ctx, files[idx].Path, files[idx].Size, in.HashAttempts, in.HashRetryDelay, s.fullHash)
				if ferr != nil {
					addFailure(NewHashFailure(files[idx], "full", used, "hash_failed"))
					return errors.New("full fingerprint failed; path omitted")
				}
				filesMu.Lock()
				files[idx].ContentSHA256 = h
				filesMu.Unlock()
				return nil
			})
		}
	}
	_ = fullRunner.Wait()
	s.stage.Store("persisting")

	result.Files = files
	result.HashFailures = hashFailures
	result.ScanErrors = len(stats.Errors)

	// Persist to DB and mark missing files.
	if s.store != nil {
		ids, err := s.store.UpsertFiles(ctx, files)
		if err != nil {
			return nil, err
		}
		for i, id := range ids {
			if err := s.store.SaveContext(ctx, id, dircontext.Classify(files[i].Path), dircontext.RuleVersion()); err != nil {
				return nil, err
			}
		}
		// Mark files not seen in this scan as missing.
		seenPaths := make([]string, len(files))
		for i, f := range files {
			seenPaths[i] = f.Path
		}
		if len(stats.Errors) == 0 {
			result.Missing, err = s.store.MarkFilesMissing(ctx, in.StorageID, seenPaths)
			result.FullTraversal = true
		} else {
			result.Unavailable, err = s.store.MarkFilesUnavailable(ctx, in.StorageID, seenPaths)
		}
		if err != nil {
			return nil, fmt.Errorf("app: reconcile scan coverage: %w", err)
		}
		// Complete checkpoint.
		if result.CheckpointID != 0 {
			_ = s.store.CompleteCheckpoint(ctx, result.CheckpointID, "completed")
		}
	}

	s.stage.Store("completed")
	return result, nil
}

// hashWithRetry calls hash with up to `attempts` retries, sleeping
// `delay` between attempts. Returns the hash, the number of attempts
// used, and the last error (nil on success). If ctx is cancelled during
// a retry delay, returns ctx.Err() immediately.
func hashWithRetry(ctx context.Context, path string, size int64, attempts int, delay time.Duration, hash HashFunc) (string, int, error) {
	if attempts < 1 {
		attempts = 1
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		value, err := hash(path, size)
		if err == nil {
			return value, attempt, nil
		}
		if attempt == attempts {
			return "", attempt, err
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", attempt, ctx.Err()
		case <-timer.C:
		}
	}
	return "", attempts, errors.New("hash attempts exhausted")
}
