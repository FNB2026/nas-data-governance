package app

import (
	"context"
	"os"
	"path/filepath"
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
		if event.EventType == events.EventStage {
			stages = append(stages, event.Stage)
		}
	}
	assertStageOrder(t, stages, string(jobs.StageQuickHashing), string(jobs.StageFullHashing), string(jobs.StageFinalizing))
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
