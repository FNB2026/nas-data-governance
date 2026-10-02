package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FNB2026/nas-data-governance/internal/events"
	"github.com/FNB2026/nas-data-governance/internal/jobs"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

func TestScanJobRecordsFullHashAndFinalizingStages(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tmp := t.TempDir()
	root := filepath.Join(tmp, "source")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("duplicate content for scan stage regression")
	for _, name := range []string{"copy-a.txt", "copy-b.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), content, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	st, err := store.Open(ctx, filepath.Join(tmp, "project.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	mgr := jobs.New(st)
	runner := NewScanJobRunner(NewScanService(st), mgr)
	jobID, result, err := runner.RunScanAsJob(ctx, "project", ScanInput{
		Root: root, StorageID: "stage-test", Workers: 1,
		HashAttempts: 1, HashRetryDelay: 0,
	})
	if err != nil {
		t.Fatalf("run scan job: %v", err)
	}
	if result == nil || len(result.Files) != 2 {
		t.Fatalf("unexpected scan result: %#v", result)
	}

	run, err := mgr.Get(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if run.State != jobs.StateCompleted || run.Stage != jobs.StageFinalizing {
		t.Fatalf("final job state=%s stage=%s", run.State, run.Stage)
	}

	jobEvents, err := mgr.ListEvents(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	var stages []string
	for _, event := range jobEvents {
		if event.EventType == events.EventWarning {
			t.Fatal("clean scan must not emit a failure summary")
		}
		if event.EventType == events.EventStage {
			stages = append(stages, event.Stage)
		}
	}
	assertStageOrder(t, stages, string(jobs.StageQuickHashing), string(jobs.StageFullHashing), string(jobs.StageFinalizing))
}

func TestScanJobPersistsFailureSummaryWithoutPrivatePaths(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	root := filepath.Join(tmp, "private-source")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	privateName := "private-source-file.txt"
	path := filepath.Join(root, privateName)
	if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(ctx, filepath.Join(tmp, "private-project.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	failHash := func(string, int64) (string, error) {
		return "", &os.PathError{Op: "open", Path: path, Err: os.ErrPermission}
	}
	mgr := jobs.New(st)
	runner := NewScanJobRunner(NewScanServiceWithHashFunc(st, failHash, failHash), mgr)
	jobID, _, err := runner.RunScanAsJob(ctx, filepath.Join(tmp, "private-project.db"), ScanInput{
		Root: root, StorageID: "test", Workers: 1, HashAttempts: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := mgr.Get(ctx, jobID)
	if err != nil || run.State != jobs.StateCompleted || run.WarningCount != 1 {
		t.Fatal("nonfatal hash failure should complete with one summary warning")
	}
	jobEvents, err := mgr.ListEvents(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	var summary map[string]any
	for _, event := range jobEvents {
		payload, err := json.Marshal(event.Payload)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(payload), tmp) || strings.Contains(string(payload), privateName) {
			t.Fatal("persisted job event exposed private source or project data")
		}
		if event.EventType == events.EventWarning {
			summary = event.Payload
		}
	}
	if summary["category"] != "scan_summary" || summary["quick_hash_failures"] != float64(1) ||
		summary["full_hash_failures"] != float64(0) || summary["scan_errors"] != float64(0) ||
		summary["coverage_state"] != "complete" {
		t.Fatalf("unexpected failure summary: %v", summary)
	}
}

func assertStageOrder(t *testing.T, stages []string, want ...string) {
	t.Helper()
	next := 0
	for _, stage := range stages {
		if next < len(want) && stage == want[next] {
			next++
		}
	}
	if next != len(want) {
		t.Fatalf("stage order %v does not contain %v", stages, want)
	}
}
