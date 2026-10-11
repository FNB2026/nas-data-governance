package wails

import (
	"context"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

func TestPurgeAPIDefaultDenialAndCapabilities(t *testing.T) {
	for _, mode := range []string{"closed", "read_only", "read_write", "locked"} {
		t.Run(mode, func(t *testing.T) {
			api := NewAPI()
			defer api.CloseProject()
			if mode != "closed" {
				path := createProjectDB(t)
				var err error
				if mode == "read_only" {
					_, err = api.OpenProject(path)
				} else {
					_, err = api.OpenProjectReadWrite(path)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			if mode == "locked" {
				ctx := context.Background()
				task := domain.OperationTask{ID: "locked-task", RootPath: t.TempDir(), State: "executing", CreatedAt: time.Now()}
				if err := api.store.CreateTask(ctx, task); err != nil {
					t.Fatal(err)
				}
				if err := api.store.SavePlans(ctx, task.ID, []domain.OperationPlan{{ID: "locked-plan", TaskID: task.ID, State: domain.PlanExecuting}}); err != nil {
					t.Fatal(err)
				}
			}
			_, err := api.CreatePurgePlans()
			if err == nil || err.Error() != "purge disabled by release policy" {
				t.Errorf("create: %v", err)
			}
			err = api.ApprovePurgePlan("private-canary", "digest")
			if err == nil || err.Error() != "purge disabled by release policy" {
				t.Errorf("approve: %v", err)
			}
			for _, dry := range []bool{true, false} {
				_, err = api.ExecutePurge(ExecutePurgeRequest{PlanID: "private-canary", Digest: "digest", QuarantineRoot: "private-canary", DryRun: dry})
				if err == nil || err.Error() != "purge disabled by release policy" {
					t.Errorf("execute dry=%v: %v", dry, err)
				}
			}
			caps, err := api.GetAppCapabilities()
			if err != nil || caps.CanExecutePurge {
				t.Errorf("purge capability: %+v %v", caps, err)
			}
			if mode == "locked" && (!caps.RecoveryLockActive || caps.CanExecuteQuarantine || caps.CanScan) {
				t.Fatal("recovery lock bypassed")
			}
			if mode == "read_write" && (!caps.CanExecuteQuarantine || !caps.CanScan) {
				t.Fatal("unrelated capabilities disabled")
			}
		})
	}
}
