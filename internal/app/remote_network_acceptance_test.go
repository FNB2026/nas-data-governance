package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/fingerprint"
	"github.com/FNB2026/nas-data-governance/internal/jobs"
	"github.com/FNB2026/nas-data-governance/internal/scanner"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// TestMountedNetworkPauseResumeAcceptance is an opt-in, operator-driven test
// for a real mounted network source. It never writes the source. While it is
// running, the operator unmounts and later remounts the source after observing
// the phase markers in verbose test output. All project state is local.
func TestMountedNetworkPauseResumeAcceptance(t *testing.T) {
	root := os.Getenv("NDG_TEST_NETWORK_SOURCE")
	if root == "" {
		t.Skip("NDG_TEST_NETWORK_SOURCE is not set")
	}

	ctx := context.Background()
	probeCtx, cancelProbe := context.WithTimeout(ctx, 10*time.Second)
	profile, err := scanner.ProbeSource(probeCtx, root)
	cancelProbe()
	if err != nil {
		t.Fatalf("initial source preflight failed (path omitted): %v", err)
	}
	if !profile.Network {
		t.Fatal("acceptance source must be a mounted network filesystem")
	}

	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "acceptance.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// Keep a small acceptance scan active long enough for the operator to
	// unmount it, while still performing real read-only fingerprints.
	quick := func(path string, size int64) (string, error) {
		time.Sleep(1500 * time.Millisecond)
		return fingerprint.Quick(path, size)
	}
	full := func(path string, _ int64) (string, error) {
		time.Sleep(1500 * time.Millisecond)
		return fingerprint.Full(path)
	}
	scanSvc := NewScanServiceWithHashFunc(st, quick, full)
	mgr := jobs.New(st)
	runner := NewScanJobRunner(scanSvc, mgr)
	input := ScanInput{
		Root:           root,
		StorageID:      "remote-pause-acceptance",
		NetworkSource:  true,
		Workers:        1,
		HashAttempts:   1,
		HashRetryDelay: 0,
	}

	t.Log("REMOTE_PAUSE_READY: unmount the selected SMB volume now")
	jobID, err := runner.StartScanJob(ctx, "remote-acceptance", input)
	if err != nil {
		t.Fatal(err)
	}
	run := waitForAcceptanceTerminal(t, ctx, mgr, jobID, 90*time.Second)
	if run.State != jobs.StatePausedNetwork {
		t.Fatalf("first job state = %s, want PAUSED_NETWORK", run.State)
	}
	cp, err := st.LastCheckpoint(ctx, input.StorageID)
	if err != nil {
		t.Fatal(err)
	}
	if cp.Status != "paused_network" {
		t.Fatalf("checkpoint status = %q, want paused_network", cp.Status)
	}
	t.Logf("REMOTE_PAUSE_CONFIRMED: checkpoint_count=%d; remount the SMB volume now", cp.ScannedCount)

	waitForMountedSource(t, root, 120*time.Second)
	input.Resume = true
	resumeID, err := runner.StartScanJob(ctx, "remote-acceptance", input)
	if err != nil {
		t.Fatal(err)
	}
	resumed := waitForAcceptanceTerminal(t, ctx, mgr, resumeID, 5*time.Minute)
	if resumed.State != jobs.StateCompleted {
		t.Fatalf("resumed job state = %s, want COMPLETED", resumed.State)
	}
	if _, err := st.LastCheckpoint(ctx, input.StorageID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("completed resume checkpoint lookup = %v, want ErrNotFound", err)
	}
	t.Logf("REMOTE_RESUME_COMPLETED: discovered=%d processed=%d", resumed.Progress.Discovered, resumed.Progress.Processed)
}

func waitForAcceptanceTerminal(t *testing.T, ctx context.Context, mgr *jobs.JobManager, jobID string, timeout time.Duration) jobs.JobRun {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		run, err := mgr.Get(ctx, jobID)
		if err != nil {
			t.Fatal(err)
		}
		if run.State.IsTerminal() {
			return run
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("timed out waiting for acceptance job terminal state")
	return jobs.JobRun{}
}

func waitForMountedSource(t *testing.T, root string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		profile, err := scanner.ProbeSource(ctx, root)
		cancel()
		if err == nil && profile.Status == scanner.SourceOnline {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatal("timed out waiting for source remount (path omitted)")
}
