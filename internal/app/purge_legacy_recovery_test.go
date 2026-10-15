package app

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/executor"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// A legacy journal cannot distinguish completed unlink from an external move,
// hidden/replaced mount, or recreated path. Real temporary files and reopened
// SQLite state must remain unresolved for every observable path combination.
func TestLegacyPurgeRecoveryRetainsLockAcrossReopen(t *testing.T) {
	for _, status := range []string{"pending", "staged", "commit_pending"} {
		for _, shape := range []string{"original_only", "stage_only", "matching_both", "missing_both", "different_both", "stage_symlink", "stage_directory", "root_replaced"} {
			t.Run(status+"/"+shape, func(t *testing.T) {
				ctx := context.Background()
				tmp := t.TempDir()
				source := filepath.Join(tmp, "source")
				root := filepath.Join(tmp, "quarantine")
				for _, p := range []string{source, root} {
					if err := os.Mkdir(p, 0700); err != nil {
						t.Fatal(err)
					}
				}
				db := filepath.Join(t.TempDir(), "state.db")
				st, err := store.Open(ctx, db)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = st.Close() })
				now := time.Now().UTC()
				item := seedQuarantineItem(t, st, source, root, "legacy", now.Add(-2*time.Hour), now.Add(-time.Hour))
				fixture := newPurgeLifecycleFixture(st)
				plans, err := fixture.CreatePlans(ctx)
				if err != nil || len(plans) != 1 {
					t.Fatalf("seed plans: %v", err)
				}
				plan := plans[0]
				if err := fixture.ApprovePlan(ctx, plan.ID, plan.ApprovalDigest); err != nil {
					t.Fatal(err)
				}
				plan, err = st.GetPurgePlan(ctx, plan.ID)
				if err != nil {
					t.Fatal(err)
				}
				// Candidate enumeration persists eligibility; capture its actual
				// durable state before testing recovery, not the older seed value.
				item, err = st.GetQuarantineItem(ctx, item.ID)
				if err != nil {
					t.Fatal(err)
				}
				stage := filepath.Join(root, ".purge-staging", plan.ID+".stage")
				if err := os.Mkdir(filepath.Dir(stage), 0700); err != nil {
					t.Fatal(err)
				}
				original, err := os.ReadFile(item.QuarantinePath)
				if err != nil {
					t.Fatal(err)
				}
				if err := st.BeginPurge(ctx, plan, item, stage, now); err != nil {
					t.Fatal(err)
				}
				if status != "pending" {
					if err := st.MarkPurgeStaged(ctx, plan.ID); err != nil {
						t.Fatal(err)
					}
				}
				if status == "commit_pending" {
					if err := st.MarkPurgeCommitPending(ctx, plan.ID); err != nil {
						t.Fatal(err)
					}
				}
				write := func(p string, b []byte) {
					t.Helper()
					if err := os.WriteFile(p, b, 0600); err != nil {
						t.Fatal(err)
					}
				}
				removeOriginal := func() {
					t.Helper()
					if err := os.Remove(item.QuarantinePath); err != nil {
						t.Fatal(err)
					}
				}
				switch shape {
				case "original_only":
				case "stage_only":
					if err := os.Rename(item.QuarantinePath, stage); err != nil {
						t.Fatal(err)
					}
				case "matching_both":
					write(stage, original)
				case "missing_both":
					removeOriginal()
				case "different_both":
					write(stage, []byte("unknown staged bytes"))
					write(item.QuarantinePath, []byte("recreated original bytes"))
				case "stage_symlink":
					if err := os.Symlink(item.QuarantinePath, stage); err != nil {
						t.Fatal(err)
					}
				case "stage_directory":
					if err := os.Mkdir(stage, 0700); err != nil {
						t.Fatal(err)
					}
					write(filepath.Join(stage, "unknown"), []byte("preserve"))
				case "root_replaced":
					if err := os.Rename(root, root+"-preserved"); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(root, 0700); err != nil {
						t.Fatal(err)
					}
					write(filepath.Join(root, "unrelated"), []byte("replacement root"))
				}
				beforeFiles := purgeRecoveryFiles(t, tmp)
				beforeEntries, err := st.ListRecoverablePurges(ctx)
				if err != nil || len(beforeEntries) != 1 {
					t.Fatal("seed journal missing")
				}
				for range 2 {
					if err := st.Close(); err != nil {
						t.Fatal(err)
					}
					st, err = store.Open(ctx, db)
					if err != nil {
						t.Fatal(err)
					}
					results, err := NewPurgeService(st).RecoverPurges(ctx, root)
					if err != nil || len(results) != 1 {
						t.Fatalf("recovery failed: %v", err)
					}
					r := results[0]
					if r.PlanID != plan.ID || r.Status != executor.StepFailed || r.Err == nil || r.ErrorType != "manual_reconciliation_required" || r.FinalState != "" {
						t.Fatalf("unproven finalization: %+v", r)
					}
					if requireNoRecovery(ctx, st) == nil {
						t.Fatal("recovery lock cleared")
					}
					entries, err := st.ListRecoverablePurges(ctx)
					if err != nil || !reflect.DeepEqual(entries, beforeEntries) {
						t.Fatal("journal changed")
					}
					gotPlan, err := st.GetPurgePlan(ctx, plan.ID)
					if err != nil || !reflect.DeepEqual(gotPlan, plan) {
						t.Fatal("plan changed")
					}
					gotItem, err := st.GetQuarantineItem(ctx, item.ID)
					if err != nil || !reflect.DeepEqual(gotItem, item) {
						t.Fatal("item changed")
					}
					if !reflect.DeepEqual(purgeRecoveryFiles(t, tmp), beforeFiles) {
						t.Fatal("legacy recovery changed filesystem evidence")
					}
					if PurgeReleaseAllowed() {
						t.Fatal("release policy unexpectedly enabled")
					}
				}
			})
		}
	}
}

type purgeRecoveryFile struct {
	Mode  os.FileMode
	Bytes string
	Link  string
}

// WalkDir does not follow symlinks. Content is read only for fixture regular
// files; this helper never dereferences a journal's untrusted target.
func purgeRecoveryFiles(t *testing.T, root string) map[string]purgeRecoveryFile {
	t.Helper()
	out := map[string]purgeRecoveryFile{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		s, err := d.Info()
		if err != nil {
			return err
		}
		value := purgeRecoveryFile{Mode: s.Mode()}
		if s.Mode().IsRegular() {
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			value.Bytes = string(b)
		}
		if s.Mode()&os.ModeSymlink != 0 {
			v, err := os.Readlink(p)
			if err != nil {
				return err
			}
			value.Link = v
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		out[rel] = value
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
