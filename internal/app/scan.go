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
	// NetworkSource enables remote-disconnect pause semantics. It is derived
	// from the read-only source preflight rather than from user input.
	NetworkSource bool
	// Workers is the number of concurrent hash workers (1 = serial).
	Workers int
	// HashAttempts is the maximum read attempts per hash (1-10).
	HashAttempts int
	// HashRetryDelay is the delay between hash attempts.
	HashRetryDelay time.Duration
	// onStageChanged is set by ScanJobRunner so persistent jobs receive
	// stage transitions immediately instead of relying on a polling window.
	// It is intentionally private: callers cannot forge job state.
	onStageChanged func(string)
}

// ErrNetworkSourceUnavailable indicates that a remote filesystem became
// unavailable after the durable scan prefix had been saved. Callers should
// offer resume rather than treating this as data loss or a completed scan.
var ErrNetworkSourceUnavailable = errors.New("app: network source unavailable; scan paused")

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
	// CoverageState makes the reconciliation decision explicit for UI and
	// audit consumers. Partial coverage must never be interpreted as deletion.
	CoverageState string
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
	// Production recovered reads use root-anchored, no-follow descriptors.
	guardedRecovery bool

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
		store:           st,
		guardedRecovery: true,
		quickHash:       fingerprint.Quick,
		fullHash:        func(path string, _ int64) (string, error) { return fingerprint.Full(path) },
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

func (s *ScanService) setStage(in ScanInput, stage string) {
	s.stage.Store(stage)
	if in.onStageChanged != nil {
		in.onStageChanged(stage)
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
	if in.Resume && !in.FullScan {
		s.setStage(in, "preparing_resume")
	} else {
		s.setStage(in, "traversal")
	}

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
	// Running, aborted, and network-paused checkpoints are resumable. A
	// 'completed' checkpoint means the
	// previous scan finished — resuming from it would skip already-
	// fully-scanned files, so we start a fresh checkpoint instead.
	if s.store != nil {
		if in.Resume && !in.FullScan {
			cp, err := s.store.LastCheckpoint(ctx, in.StorageID)
			if err == nil && (cp.Status == "running" || cp.Status == "aborted" || cp.Status == "paused_network") {
				result.ResumedFrom = cp.LastScannedPath
				result.CheckpointID = cp.ID
				result.ResumedCount = cp.ScannedCount
			} else if err != nil && !errors.Is(err, store.ErrNotFound) {
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

	if result.ResumedFrom != "" {
		s.setStage(in, "seeking_resume")
	} else {
		s.setStage(in, "traversal")
	}

	// Scan with incremental hash reuse.
	var files []domain.FileInstance
	var filesMu sync.Mutex
	var hashFailures []HashFailure
	var failuresMu sync.Mutex
	var networkUnavailable atomic.Bool
	persistedCount := 0
	lastCheckpointedSessionCount := 0

	addFile := func(f domain.FileInstance, discovered bool) {
		filesMu.Lock()
		files = append(files, f)
		filesMu.Unlock()
		if discovered {
			s.processed.Add(1)
		}
	}
	addFailure := func(f HashFailure) {
		failuresMu.Lock()
		hashFailures = append(hashFailures, f)
		failuresMu.Unlock()
		s.failed.Add(1)
	}

	hashRunner := runner.New(in.Workers)
	persistPending := func(persistCtx context.Context) error {
		if s.store == nil {
			return nil
		}
		filesMu.Lock()
		if persistedCount >= len(files) {
			filesMu.Unlock()
			return nil
		}
		batch := append([]domain.FileInstance(nil), files[persistedCount:]...)
		filesMu.Unlock()

		ids, err := s.store.UpsertFiles(persistCtx, batch)
		if err != nil {
			return err
		}
		for i, id := range ids {
			if err := s.store.SaveContext(persistCtx, id, dircontext.Classify(batch[i].Path), dircontext.RuleVersion()); err != nil {
				return err
			}
		}
		persistedCount += len(batch)
		return nil
	}
	scanOpts := scanner.Options{
		Root:          in.Root,
		StorageID:     in.StorageID,
		ExcludedNames: scanner.DefaultExclusions(),
		ResumePath:    result.ResumedFrom,
		NetworkSource: in.NetworkSource,
		OnDirectoryCompleted: func(cp scanner.DirectoryCheckpoint) error {
			if s.store == nil || result.CheckpointID == 0 || cp.FilesScanned <= lastCheckpointedSessionCount {
				return nil
			}
			// A resume boundary becomes durable only after all hashing submitted
			// for this completed directory prefix has finished and its metadata
			// has been committed to the project database.
			hashRunner.Wait()
			if networkUnavailable.Load() {
				// Do not advance past files whose content was not readable after
				// the network mount disappeared. Resume must revisit them.
				return ErrNetworkSourceUnavailable
			}
			if err := persistPending(ctx); err != nil {
				return errors.New("persist directory scan checkpoint failed")
			}
			totalScanned := result.ResumedCount + cp.FilesScanned
			if err := s.store.UpdateCheckpoint(ctx, result.CheckpointID, cp.LastScannedPath, totalScanned); err != nil {
				return errors.New("update directory scan checkpoint failed")
			}
			lastCheckpointedSessionCount = cp.FilesScanned
			return nil
		},
	}
	visitFile := func(file domain.FileInstance, discovered bool) error {
		// Once content access is lost, further traversal cannot advance the
		// durable checkpoint. Stop queuing work and finalize a resumable pause.
		if networkUnavailable.Load() {
			return ErrNetworkSourceUnavailable
		}
		if discovered && s.discovered.Add(1) == 1 && result.ResumedFrom != "" {
			s.setStage(in, "traversal")
		}
		// Incremental reuse is allowed only when both the previous and current
		// scan report a reliable physical identity. SMB/NFS/WebDAV/FUSE may
		// expose synthetic or unstable inode values; those values must never
		// authorize reuse of a cached content hash.
		if cached, ok := cache[file.Path]; ok {
			if canReuseCachedHash(cached, file) {
				file.QuickHash = cached.QuickHash
				file.ContentSHA256 = cached.ContentSHA256
				addFile(file, discovered)
				return nil
			}
		}
		// File is new or changed: compute quick hash (possibly concurrent).
		return hashRunner.Submit(ctx, func() error {
			hash := s.quickHash
			if !discovered {
				hash = func(path string, size int64) (string, error) {
					if s.guardedRecovery {
						return fingerprint.Guarded(in.Root, file, false)
					}
					if err := scanner.ValidateFile(in.Root, file); err != nil {
						return "", err
					}
					q, err := s.quickHash(path, size)
					if err != nil {
						return "", err
					}
					if err := scanner.ValidateFile(in.Root, file); err != nil {
						return "", err
					}
					return q, nil
				}
			}
			q, used, qerr := hashWithRetry(ctx, file.Path, file.Size, in.HashAttempts, in.HashRetryDelay, hash)
			if qerr != nil {
				if in.NetworkSource && scanner.IsNetworkUnavailableError(qerr) {
					networkUnavailable.Store(true)
				}
				addFile(file, discovered)
				addFailure(NewHashFailure(file, "quick", used, "hash_failed"))
				return errors.New("quick fingerprint failed; path omitted")
			}
			file.QuickHash = q
			addFile(file, discovered)
			return nil
		})
	}
	stats, err := scanner.Scan(ctx, scanOpts, func(file domain.FileInstance) error {
		return visitFile(file, true)
	})

	s.setStage(in, "quick_hash")
	hashErrs := hashRunner.Wait()
	_ = hashErrs // already recorded as hashFailures
	if networkUnavailable.Load() {
		stats.SourceUnavailable = true
		if errors.Is(err, ErrNetworkSourceUnavailable) {
			// This is a controlled network pause, not a traversal failure.
			err = nil
		}
	}
	// Cancellation may leave an incomplete directory prefix in memory. Do not
	// force that batch into the project database or advance its checkpoint:
	// retain the last already-durable directory boundary and mark it aborted.
	if ctx.Err() != nil {
		if result.CheckpointID != 0 && s.store != nil {
			checkpointCtx, cancelCheckpoint := context.WithTimeout(context.Background(), 5*time.Second)
			_ = s.store.CompleteCheckpoint(checkpointCtx, result.CheckpointID, "aborted")
			cancelCheckpoint()
		}
		return nil, ctx.Err()
	}
	if persistErr := persistPending(ctx); persistErr != nil {
		if result.CheckpointID != 0 && s.store != nil {
			_ = s.store.CompleteCheckpoint(context.Background(), result.CheckpointID, "aborted")
		}
		return nil, fmt.Errorf("app: persist scan prefix: %w", persistErr)
	}

	if err != nil {
		if result.CheckpointID != 0 && s.store != nil {
			_ = s.store.CompleteCheckpoint(ctx, result.CheckpointID, "aborted")
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, errors.New("scan traversal failed; source paths omitted")
	}

	// Traversal durability does not imply that its duplicate candidates have
	// completed their full hashes. Revisit only pending prefix groups and their
	// peers, including groups spanning the checkpoint. Use the scanner's normal
	// boundary checks and fresh metadata; never trust a remote cached inode.
	if result.ResumedFrom != "" && !stats.SourceUnavailable {
		visited := map[string]bool{}
		for {
			selected := resumeHashCandidates(cache, result.ResumedFrom, files, visited)
			if len(selected) == 0 {
				break
			}
			s.setStage(in, "quick_hash")
			observed := map[string]bool{}
			selectedStats, selectedErr := scanner.Scan(ctx, scanner.Options{
				Root: in.Root, StorageID: in.StorageID, NetworkSource: in.NetworkSource,
				ExcludedNames: scanner.DefaultExclusions(), SelectedPaths: selected,
			}, func(file domain.FileInstance) error {
				observed[file.Path] = true
				visited[file.Path] = true
				return visitFile(file, false)
			})
			hashRunner.Wait()
			if networkUnavailable.Load() || selectedStats.SourceUnavailable || errors.Is(selectedErr, ErrNetworkSourceUnavailable) {
				stats.SourceUnavailable = true
				break
			}
			// A replaced symlink, excluded path, mount boundary or unreadable
			// candidate is not a successful recovery. Keep the checkpoint resumable
			// rather than silently certifying its cached hashes as current.
			if selectedErr != nil || len(selectedStats.Errors) != 0 || len(observed) != len(selected) {
				_ = s.store.CompleteCheckpoint(context.Background(), result.CheckpointID, "aborted")
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				return nil, errors.New("resume candidate validation failed; source paths omitted")
			}
		}
	}

	// Second-stage hashing: only for files that share size+quick_hash
	// with another file AND don't already have content_sha256 cached.
	s.setStage(in, "full_hash")
	bySizeQuick := map[string][]int{}
	for i, f := range files {
		if f.QuickHash != "" {
			key := fmt.Sprintf("%d:%s", f.Size, f.QuickHash)
			bySizeQuick[key] = append(bySizeQuick[key], i)
		}
	}
	fullRunner := runner.New(in.Workers)
	// Each candidate starts pending and must reach a recorded success or
	// failure. Cancellation/network pause may leave pending work; those runs
	// cannot certify completion. Different workers own different indexes.
	fullOutcomes := make([]string, len(files))
	for _, indexes := range bySizeQuick {
		if len(indexes) >= 2 {
			for _, i := range indexes {
				if files[i].ContentSHA256 == "" {
					fullOutcomes[i] = "pending"
				}
			}
		}
	}
fullHashSubmission:
	for _, indexes := range bySizeQuick {
		if len(indexes) < 2 {
			continue
		}
		for _, i := range indexes {
			if files[i].ContentSHA256 != "" {
				continue
			}
			// A remote source disappearing is a scan-level interruption. Stop
			// queuing additional full hashes as soon as a worker confirms it,
			// then let the already-running bounded set drain before persisting
			// the resumable paused checkpoint below.
			if networkUnavailable.Load() {
				break fullHashSubmission
			}
			idx := i // capture for closure
			if submitErr := fullRunner.Submit(ctx, func() error {
				hash := s.fullHash
				if result.ResumedFrom != "" && files[idx].Path <= result.ResumedFrom {
					// Recovered prefix work has been absent from this traversal.
					// Check its bounds/metadata on both sides of every read attempt.
					hash = func(path string, size int64) (string, error) {
						if s.guardedRecovery {
							return fingerprint.Guarded(in.Root, files[idx], true)
						}
						if err := scanner.ValidateFile(in.Root, files[idx]); err != nil {
							return "", err
						}
						h, err := s.fullHash(path, size)
						if err != nil {
							return "", err
						}
						if err := scanner.ValidateFile(in.Root, files[idx]); err != nil {
							return "", err
						}
						return h, nil
					}
				}
				h, used, ferr := hashWithRetry(ctx, files[idx].Path, files[idx].Size, in.HashAttempts, in.HashRetryDelay, hash)
				if ferr == nil && h == "" {
					ferr = errors.New("empty full fingerprint")
				}
				if ferr != nil {
					if in.NetworkSource && scanner.IsNetworkUnavailableError(ferr) {
						networkUnavailable.Store(true)
					}
					addFailure(NewHashFailure(files[idx], "full", used, "hash_failed"))
					fullOutcomes[idx] = "failed"
					return errors.New("full fingerprint failed; path omitted")
				}
				filesMu.Lock()
				files[idx].ContentSHA256 = h
				fullOutcomes[idx] = "success"
				filesMu.Unlock()
				return nil
			}); submitErr != nil {
				break fullHashSubmission
			}
		}
	}
	_ = fullRunner.Wait()
	// Cancellation can leave candidates unsubmitted. They have no terminal
	// hash outcome, so even a complete traversal must remain resumable.
	if ctx.Err() != nil {
		if s.store != nil && result.CheckpointID != 0 {
			checkpointCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = s.store.CompleteCheckpoint(checkpointCtx, result.CheckpointID, "aborted")
			cancel()
		}
		return nil, ctx.Err()
	}
	if networkUnavailable.Load() {
		stats.SourceUnavailable = true
	}
	if !stats.SourceUnavailable {
		for _, outcome := range fullOutcomes {
			if outcome == "pending" {
				return nil, errors.New("full fingerprint work incomplete; scan remains resumable")
			}
		}
	}
	s.setStage(in, "persisting")

	result.Files = files
	result.HashFailures = hashFailures
	result.ScanErrors = len(stats.Errors)

	// Persist to DB and mark missing files.
	if s.store != nil {
		// Full hashes are computed after traversal checkpoints are persisted.
		// Upsert the final in-memory records once more so those stronger hashes
		// replace the quick-hash-only checkpoint rows.
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
		seenPaths := make([]string, 0, len(files)+len(cache))
		// A resumed scan intentionally skips the already-persisted prefix.
		// Include that durable prefix in reconciliation so a successful resume
		// cannot incorrectly mark its own checkpointed files as missing.
		if result.ResumedFrom != "" {
			for path := range cache {
				if path <= result.ResumedFrom {
					seenPaths = append(seenPaths, path)
				}
			}
		}
		for _, f := range files {
			seenPaths = append(seenPaths, f.Path)
		}
		if len(stats.Errors) == 0 && !stats.SourceUnavailable {
			result.Missing, err = s.store.MarkFilesMissing(ctx, in.StorageID, seenPaths)
			result.FullTraversal = true
			result.CoverageState = "complete"
		} else {
			result.Unavailable, err = s.store.MarkFilesUnavailable(ctx, in.StorageID, seenPaths)
			result.CoverageState = "partial"
		}
		if err != nil {
			return nil, fmt.Errorf("app: reconcile scan coverage: %w", err)
		}
		// Complete or pause the checkpoint. A network pause remains resumable.
		if result.CheckpointID != 0 {
			status := "completed"
			if stats.SourceUnavailable {
				status = "paused_network"
			}
			if err := s.store.CompleteCheckpoint(ctx, result.CheckpointID, status); err != nil {
				return nil, errors.New("persist scan checkpoint outcome failed")
			}
		}
	}
	if s.store == nil {
		if len(stats.Errors) == 0 && !stats.SourceUnavailable {
			result.FullTraversal = true
			result.CoverageState = "complete"
		} else {
			result.CoverageState = "partial"
		}
	}
	if stats.SourceUnavailable {
		return result, ErrNetworkSourceUnavailable
	}

	s.setStage(in, "completed")
	return result, nil
}

func canReuseCachedHash(cached store.FileMeta, file domain.FileInstance) bool {
	return cached.Size == file.Size &&
		cached.ModifiedAt.Equal(file.ModifiedAt) &&
		cached.PhysicalReliable && file.Physical.Reliable &&
		cached.Device == file.Device &&
		cached.Inode == file.Inode && cached.QuickHash != ""
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
