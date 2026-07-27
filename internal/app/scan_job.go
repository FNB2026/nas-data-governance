package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/jobs"
)

// ScanJobRunner wraps ScanService with JobManager to provide persistent
// task tracking, structured events, and cancellation for scan operations.
// It bridges the ScanService's in-memory progress counters to the
// JobManager's persisted progress and event log.
//
// Per ADR-0006 §2, this sits in the application service layer, above
// ScanService and below the CLI/desktop presentation layer. The CLI can
// call ScanService.Scan directly for simple use cases, or use
// ScanJobRunner for job-tracked execution.
type ScanJobRunner struct {
	scan *ScanService
	jobs *jobs.JobManager
}

// NewScanJobRunner creates a scan job runner.
func NewScanJobRunner(scan *ScanService, mgr *jobs.JobManager) *ScanJobRunner {
	return &ScanJobRunner{scan: scan, jobs: mgr}
}

// scanStageMap maps ScanService internal stage strings to JobStage values.
var scanStageMap = map[string]jobs.JobStage{
	"traversal":  jobs.StageDiscovering,
	"quick_hash": jobs.StageQuickHashing,
	"persisting": jobs.StageFinalizing,
	"completed":  jobs.StageFinalizing,
}

// RunScanAsJob creates a scan job, starts it, and executes the scan
// pipeline with progress tracking and cancellation support.
//
// The method blocks until the scan completes, fails, or is cancelled.
// The returned jobID can be used with JobManager to query state and
// event history.
//
// Progress is polled from ScanService at progressInterval (1 second by
// default) and reported to the JobManager as structured events. Stage
// transitions are detected and reported automatically.
func (r *ScanJobRunner) RunScanAsJob(ctx context.Context, projectID string, in ScanInput) (string, *ScanResult, error) {
	jobID, err := r.jobs.Create(ctx, projectID, jobs.JobScan)
	if err != nil {
		return "", nil, fmt.Errorf("app: scan job: create: %w", err)
	}

	var result *ScanResult
	var scanErr error

	err = r.jobs.Run(ctx, jobID, func(jobCtx context.Context, reporter *jobs.Reporter) error {
		// Start a progress poller goroutine that reads ScanService.Progress()
		// and reports to the JobManager at intervals.
		progressDone := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			defer wg.Done()
			r.pollScanProgress(jobCtx, reporter, progressDone)
		}()

		// Run the scan. The jobCtx will be cancelled if the user requests
		// cancellation, causing the scan to stop.
		result, scanErr = r.scan.Scan(jobCtx, in)

		// Stop the progress poller.
		close(progressDone)
		wg.Wait()

		if scanErr != nil {
			if errors.Is(scanErr, context.Canceled) || errors.Is(scanErr, context.DeadlineExceeded) {
				return jobs.ErrCancellationRequested
			}
			return scanErr
		}

		// Emit a final progress update with the scan result counts.
		_ = reporter.SetProgress(jobCtx, jobs.ProgressPayload{
			Discovered: r.scan.discovered.Load(),
			Processed:  r.scan.processed.Load(),
			Failed:     r.scan.failed.Load(),
		})

		return nil
	})

	return jobID, result, err
}

// pollScanProgress reads ScanService.Progress() at regular intervals and
// reports stage transitions and progress snapshots to the JobManager.
// It exits when progressDone is closed or jobCtx is cancelled.
func (r *ScanJobRunner) pollScanProgress(ctx context.Context, reporter *jobs.Reporter, progressDone <-chan struct{}) {
	const progressInterval = 1 * time.Second
	lastStage := ""

	ticker := time.NewTicker(progressInterval)
	defer ticker.Stop()

	for {
		select {
		case <-progressDone:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			p := r.scan.Progress()

			// Detect and report stage transitions.
			if p.Stage != lastStage {
				if jobStage, ok := scanStageMap[p.Stage]; ok {
					_ = reporter.SetStage(ctx, jobStage)
				}
				lastStage = p.Stage
			}

			// Report progress snapshot (privacy-safe: counters only).
			_ = reporter.SetProgress(ctx, jobs.ProgressPayload{
				Discovered: p.Discovered,
				Processed:  p.Processed,
				Failed:     p.Failed,
			})
		}
	}
}
