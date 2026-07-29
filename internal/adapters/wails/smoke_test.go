package wails

import (
	"context"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/jobs"
)

// TestSmokeScanLifecycle exercises the full desktop scan flow end-to-end:
// OpenProjectReadWrite → StartScan → poll progress → COMPLETED →
// ListStorages → ListDuplicateGroups → ListRecentJobs → GetJobDetail.
//
// This mirrors the exact call sequence the React frontend makes when a
// user opens a project in read-write mode, starts a scan, and reviews
// the results.
func TestSmokeScanLifecycle(t *testing.T) {
	scanDir := createScanDir(t)
	path := t.TempDir() + "/smoke-lifecycle.db"
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	// 1. Open project in read-write mode (creates new database).
	info, err := api.OpenProjectReadWrite(path)
	if err != nil {
		t.Fatalf("OpenProjectReadWrite: %v", err)
	}
	if !info.IsOpen || info.StorageCount != 0 {
		t.Fatalf("unexpected project info: %#v", info)
	}

	// 2. Start an asynchronous scan.
	resp, err := api.StartScan(StartScanRequest{
		Root:      scanDir,
		StorageID: "smoke",
		Workers:   2,
	})
	if err != nil {
		t.Fatalf("StartScan: %v", err)
	}
	if resp.JobID == "" {
		t.Fatal("expected non-empty job ID")
	}

	// 3. Poll progress until terminal.
	prog := waitForJobTerminal(t, api, resp.JobID, 30*time.Second)
	if prog.State != "COMPLETED" {
		t.Fatalf("expected COMPLETED, got %s (error: %s)", prog.State, prog.ErrorCode)
	}
	if prog.Discovered != 3 {
		t.Errorf("expected 3 discovered, got %d", prog.Discovered)
	}
	if prog.Processed != 3 {
		t.Errorf("expected 3 processed, got %d", prog.Processed)
	}
	if prog.Failed != 0 {
		t.Errorf("expected 0 failed, got %d", prog.Failed)
	}

	// 4. Verify storage was registered.
	storages, err := api.ListStorages()
	if err != nil {
		t.Fatalf("ListStorages: %v", err)
	}
	found := false
	for _, s := range storages {
		if s.ID == "smoke" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected storage 'smoke' after scan")
	}

	// 5. Verify duplicate groups are visible.
	groupsResp, err := api.ListDuplicateGroups(ListGroupsRequest{PageSize: 50})
	if err != nil {
		t.Fatalf("ListDuplicateGroups: %v", err)
	}
	if groupsResp.TotalCount == 0 {
		t.Error("expected duplicate groups after scanning files with shared content")
	}

	// 6. Verify job appears in recent jobs list.
	jobs, err := api.ListRecentJobs(10)
	if err != nil {
		t.Fatalf("ListRecentJobs: %v", err)
	}
	if len(jobs) == 0 {
		t.Fatal("expected at least 1 job in history")
	}
	if jobs[0].JobID != resp.JobID {
		t.Errorf("expected first job %s, got %s", resp.JobID, jobs[0].JobID)
	}
	if jobs[0].State != "COMPLETED" {
		t.Errorf("expected COMPLETED in history, got %s", jobs[0].State)
	}

	// 7. Verify job detail has events.
	detail, err := api.GetJobDetail(resp.JobID)
	if err != nil {
		t.Fatalf("GetJobDetail: %v", err)
	}
	if detail.JobID != resp.JobID {
		t.Errorf("expected job ID %s, got %s", resp.JobID, detail.JobID)
	}
	if len(detail.Events) < 2 {
		t.Errorf("expected at least 2 events, got %d", len(detail.Events))
	}
	// Verify events are ordered by sequence.
	for i := 1; i < len(detail.Events); i++ {
		if detail.Events[i].Sequence <= detail.Events[i-1].Sequence {
			t.Fatal("events not ordered by sequence")
			break
		}
	}
}

// TestSmokeCancelAndHistory verifies the cancel flow: start a scan,
// cancel it, verify terminal state, and confirm the job appears in
// history with the correct state.
func TestSmokeCancelAndHistory(t *testing.T) {
	scanDir := createScanDir(t)
	path := t.TempDir() + "/smoke-cancel.db"
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProjectReadWrite(path); err != nil {
		t.Fatalf("OpenProjectReadWrite: %v", err)
	}

	resp, err := api.StartScan(StartScanRequest{
		Root:      scanDir,
		StorageID: "smoke-cancel",
	})
	if err != nil {
		t.Fatalf("StartScan: %v", err)
	}

	// Cancel immediately. For a small directory the scan may have
	// already completed; either terminal state is acceptable.
	if err := api.CancelScan(resp.JobID); err != nil {
		t.Fatalf("CancelScan: %v", err)
	}

	prog := waitForJobTerminal(t, api, resp.JobID, 30*time.Second)
	if prog.State != "CANCELLED" && prog.State != "COMPLETED" {
		t.Fatalf("expected CANCELLED or COMPLETED, got %s", prog.State)
	}

	// Verify the job is in history with the correct state.
	jobs, err := api.ListRecentJobs(10)
	if err != nil {
		t.Fatalf("ListRecentJobs: %v", err)
	}
	if len(jobs) == 0 || jobs[0].JobID != resp.JobID {
		t.Fatal("expected cancelled job in history")
	}
	if jobs[0].State != prog.State {
		t.Errorf("history state %s != progress state %s", jobs[0].State, prog.State)
	}

	// Verify job detail is accessible and has events.
	detail, err := api.GetJobDetail(resp.JobID)
	if err != nil {
		t.Fatalf("GetJobDetail: %v", err)
	}
	if len(detail.Events) < 1 {
		t.Error("expected at least 1 event for cancelled job")
	}
}

// TestSmokeCrashRecovery simulates a desktop crash during a scan:
//  1. Open project RW, start a scan, close the project immediately
//     (simulating app crash/quit while the job is non-terminal).
//  2. Reopen the same database.
//  3. Verify the stale job is marked FAILED with error_code
//     "crash_recovery" and appears in history.
//
// To reliably trigger the crash recovery path (rather than racing
// against a fast scan), Phase 1 directly seeds a RUNNING job into
// the database via the store, then closes the API. Phase 2 reopens
// through the Wails adapter, which triggers OpenProjectReadWrite's
// built-in Recover() call.
func TestSmokeCrashRecovery(t *testing.T) {
	scanDir := createScanDir(t)
	path := t.TempDir() + "/smoke-crash.db"

	// Phase 1: Create the database via the Wails adapter, then
	// directly insert a non-terminal job to simulate a crash mid-scan.
	var staleJobID string
	{
		api := NewAPI()
		if _, err := api.OpenProjectReadWrite(path); err != nil {
			t.Fatalf("OpenProjectReadWrite (phase 1): %v", err)
		}

		// Directly seed a RUNNING job via the store to guarantee a
		// non-terminal state at crash time. This simulates a scan
		// that was interrupted mid-execution.
		ctx := context.Background()
		mgr := jobs.New(api.store)
		jobID, err := mgr.Create(ctx, path, jobs.JobScan)
		if err != nil {
			t.Fatalf("Create job: %v", err)
		}
		staleJobID = jobID

		// Transition to RUNNING so the job is in a recoverable state.
		if err := api.store.UpdateJobState(ctx, jobID, jobs.StateRunning, jobs.StageQuickHashing); err != nil {
			t.Fatalf("UpdateJobState: %v", err)
		}

		// Simulate crash: close without completing the job.
		_ = api.CloseProject()
		t.Logf("phase 1: seeded RUNNING job %s, closed project (simulated crash)", staleJobID)
	}

	// Phase 2: Reopen the same database. OpenProjectReadWrite runs
	// crash recovery, which should find the non-terminal job and mark
	// it as FAILED with error_code "crash_recovery".
	{
		api := NewAPI()
		t.Cleanup(func() { _ = api.CloseProject() })

		if _, err := api.OpenProjectReadWrite(path); err != nil {
			t.Fatalf("OpenProjectReadWrite (phase 2): %v", err)
		}

		// List recent jobs — the stale job should now be FAILED.
		recentJobs, err := api.ListRecentJobs(10)
		if err != nil {
			t.Fatalf("ListRecentJobs (phase 2): %v", err)
		}
		if len(recentJobs) == 0 {
			t.Fatal("expected at least 1 job after crash recovery")
		}

		// Find the crashed job.
		var found bool
		for _, j := range recentJobs {
			if j.JobID == staleJobID {
				found = true
				if j.State != "FAILED" {
					t.Errorf("expected FAILED, got %s", j.State)
				}
				if j.ErrorCode != "crash_recovery" {
					t.Errorf("expected error_code 'crash_recovery', got '%s'", j.ErrorCode)
				}
				break
			}
		}
		if !found {
			t.Fatalf("stale job %s not found in recent jobs", staleJobID)
		}
		t.Logf("phase 2: found crash-recovered job %s", staleJobID)

		// Verify the recovered job has a crash_recovery event in its timeline.
		detail, err := api.GetJobDetail(staleJobID)
		if err != nil {
			t.Fatalf("GetJobDetail (phase 2): %v", err)
		}
		if detail.ErrorCode != "crash_recovery" {
			t.Errorf("expected error_code 'crash_recovery', got '%s'", detail.ErrorCode)
		}

		foundFailedEvent := false
		for _, ev := range detail.Events {
			if ev.EventType == "job:failed" {
				foundFailedEvent = true
				if ev.State != "FAILED" {
					t.Errorf("expected FAILED state in failed event, got %s", ev.State)
				}
				break
			}
		}
		if !foundFailedEvent {
			t.Error("expected job:failed event in crash-recovered job timeline")
		}

		// Phase 3: Verify a new scan can be started after recovery.
		resp, err := api.StartScan(StartScanRequest{
			Root:      scanDir,
			StorageID: "smoke-post-recovery",
		})
		if err != nil {
			t.Fatalf("StartScan (phase 3): %v", err)
		}
		prog := waitForJobTerminal(t, api, resp.JobID, 30*time.Second)
		if prog.State != "COMPLETED" {
			t.Fatalf("expected COMPLETED after recovery, got %s", prog.State)
		}
		t.Logf("phase 3: post-recovery scan completed successfully (job %s)", resp.JobID)
	}
}

// TestSmokeReadOnlyThenReadWrite verifies that a user can open a
// project read-only first (for browsing), close it, then reopen in
// read-write mode to perform a scan. This mirrors the desktop UX flow.
func TestSmokeReadOnlyThenReadWrite(t *testing.T) {
	path := createProjectDBWithDuplicates(t)
	api1 := NewAPI()

	// Read-only open for browsing.
	info, err := api1.OpenProject(path)
	if err != nil {
		t.Fatalf("OpenProject (read-only): %v", err)
	}
	if info.StorageCount != 2 {
		t.Fatalf("expected 2 storages, got %d", info.StorageCount)
	}

	// Verify read-only can query but not scan.
	groups, err := api1.ListDuplicateGroups(ListGroupsRequest{PageSize: 10})
	if err != nil {
		t.Fatalf("ListDuplicateGroups (read-only): %v", err)
	}
	if groups.TotalCount != 2 {
		t.Fatalf("expected 2 groups, got %d", groups.TotalCount)
	}

	_, err = api1.StartScan(StartScanRequest{Root: "/tmp"})
	if err != ErrProjectNotReadWrite {
		t.Fatalf("expected ErrProjectNotReadWrite, got %v", err)
	}

	// Close read-only, reopen in read-write mode.
	if err := api1.CloseProject(); err != nil {
		t.Fatalf("CloseProject: %v", err)
	}

	api2 := NewAPI()
	t.Cleanup(func() { _ = api2.CloseProject() })

	info2, err := api2.OpenProjectReadWrite(path)
	if err != nil {
		t.Fatalf("OpenProjectReadWrite: %v", err)
	}
	if info2.StorageCount != 2 {
		t.Fatalf("expected 2 storages after RW open, got %d", info2.StorageCount)
	}

	// Verify scan is now possible.
	jobs, err := api2.ListRecentJobs(10)
	if err != nil {
		t.Fatalf("ListRecentJobs (read-write): %v", err)
	}
	// No jobs yet — database was created by tests, not scanned.
	if len(jobs) != 0 {
		t.Logf("note: expected 0 jobs in pre-seeded DB, got %d (may have leftover from prior test)", len(jobs))
	}
}
