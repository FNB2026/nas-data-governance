package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/executor"
)

// Test-only crash-state fixtures in fresh SQLite databases. These are not
// signed-App acceptance and never read or modify the historical crash DB.
func TestRestoreRecoveryCrashContract(t *testing.T) {
	for _, mode := range []string{"quarantine_only", "restore_only", "both", "neither", "partial_destination"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			source, quarantine := t.TempDir(), t.TempDir()
			st := openTestStore(t)
			item := seedQuarantineItem(t, st, source, quarantine, "disposable.dat", time.Now(), time.Now().Add(time.Hour))
			original, err := os.ReadFile(item.QuarantinePath)
			if err != nil {
				t.Fatal(err)
			}
			backup := filepath.Join(t.TempDir(), "backup")
			if err := os.WriteFile(backup, original, 0600); err != nil {
				t.Fatal(err)
			}
			svc := NewQuarantineService(st)
			plan, err := svc.CreateRestorePlan(ctx, item.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := svc.ApproveRestorePlan(ctx, plan.ID, plan.ApprovalDigest); err != nil {
				t.Fatal(err)
			}
			approved, err := st.GetRestorePlan(ctx, plan.ID)
			if err != nil {
				t.Fatal(err)
			}
			// Injection boundary: BeginRestore committed; process interrupted before
			// MarkRestoreCompleted. Only disposable test paths are mutated below.
			if err := st.BeginRestore(ctx, approved, item, time.Now()); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "restore_only":
				if err := executor.MoveFile(item.QuarantinePath, item.SourcePath, item.ContentSHA256); err != nil {
					t.Fatal(err)
				}
			case "both":
				if err := os.WriteFile(item.SourcePath, original, 0600); err != nil {
					t.Fatal(err)
				}
			case "neither":
				if err := os.WriteFile(item.QuarantinePath, []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			case "partial_destination":
				if err := os.WriteFile(item.SourcePath, []byte("partial"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			beforeSource, _ := os.ReadFile(item.SourcePath)
			beforeQ, _ := os.ReadFile(item.QuarantinePath)
			results, err := svc.RecoverRestores(ctx, RecoverRestoresInput{QuarantineRoot: quarantine, SourceRoots: []string{source}})
			if err != nil || len(results) != 1 {
				t.Fatalf("recovery result count/error: %d %v", len(results), err)
			}
			actual, err := st.GetRestorePlan(ctx, plan.ID)
			if err != nil {
				t.Fatal(err)
			}
			pending, err := st.ListPendingRestores(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "both" || mode == "neither" {
				if results[0].Status != executor.StepFailed || results[0].ErrorType == "" || actual.State != domain.RestoreApproved || len(pending) != 1 {
					t.Fatal("uncertain restore released lock or reported success")
				}
				afterSource, _ := os.ReadFile(item.SourcePath)
				afterQ, _ := os.ReadFile(item.QuarantinePath)
				if string(afterSource) != string(beforeSource) || string(afterQ) != string(beforeQ) {
					t.Fatal("ambiguous bytes were changed")
				}
			} else {
				if results[0].Status != executor.StepOK || actual.State != domain.RestoreRolledBack || len(pending) != 0 {
					t.Fatal("confirmed rollback not durable")
				}
				got, err := executor.Snapshot(item.QuarantinePath, true)
				if err != nil || got.Hash != item.ContentSHA256 || got.Size != item.FileSize {
					t.Fatal("quarantine rollback lost bytes")
				}
				if mode == "partial_destination" {
					got, _ := os.ReadFile(item.SourcePath)
					if string(got) != "partial" {
						t.Fatal("partial destination deleted without authority")
					}
				} else if _, err := os.Stat(item.SourcePath); !os.IsNotExist(err) {
					t.Fatal("unexpected restored duplicate")
				}
			}
			// No recovery attempt changed the independent backup or item lifecycle.
			b, _ := os.ReadFile(backup)
			if string(b) != string(original) {
				t.Fatal("backup changed")
			}
			managed, err := st.GetQuarantineItem(ctx, item.ID)
			if err != nil || managed.Status != domain.QuarantineActive {
				t.Fatal("item lifecycle diverged")
			}
		})
	}
}
