//go:build linux

package managedworker

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/app"
	"github.com/FNB2026/nas-data-governance/internal/format"
	"github.com/FNB2026/nas-data-governance/internal/managedfs"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

const crashStorage = "crash-fixture"

func TestSourcePurgePersistsAcrossDatabaseProcessKill(t *testing.T) {
	if os.Geteuid() == 0 || os.Getegid() == 0 {
		t.Skip("worker runs as a non-root account")
	}
	for _, cut := range []string{"before_claim", "after_claim", "after_filesystem_commit", "after_database_commit"} {
		t.Run(cut, func(t *testing.T) {
			ctx := context.Background()
			base := t.TempDir()
			root := filepath.Join(base, "temp")
			target, keeper := filepath.Join(root, "target.pdf"), filepath.Join(root, "keeper.pdf")
			sentinel := filepath.Join(base, "outside")
			if err := os.MkdirAll(filepath.Join(root, ".ndg-purge-stage"), 0700); err != nil {
				t.Fatal(err)
			}
			const contents = "%PDF-1.7\nSynthetic process crash fixture\n"
			for _, path := range []string{target, keeper, sentinel} {
				if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
					t.Fatal(err)
				}
			}
			database := filepath.Join(base, "test.db")
			st, err := store.Open(ctx, database)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = st.Close() })
			scan, err := app.NewScanService(st).Scan(ctx, app.ScanInput{
				Root: root, StorageID: crashStorage, Workers: 1, HashAttempts: 1,
			})
			if err != nil || !scan.FullTraversal || len(scan.Files) != 2 {
				t.Fatal("could not scan process crash fixture", err)
			}
			ids := make(map[string]int64)
			for _, path := range []string{target, keeper} {
				id, err := st.FileID(ctx, crashStorage, path)
				if err != nil {
					t.Fatal(err)
				}
				info, err := format.Detect(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := st.SaveFormat(ctx, id, info); err != nil {
					t.Fatal(err)
				}
				ids[path] = id
			}
			service, err := NewSourcePurge(st, root)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := service.Plan(ctx, crashStorage, ids[target], ids[keeper])
			if err != nil {
				t.Fatal(err)
			}
			for _, action := range []string{"APPROVE", "DRY_RUN"} {
				if _, err := service.Advance(ctx, plan.ID, plan.Digest, action, ""); err != nil {
					t.Fatal(err)
				}
			}
			if err := st.Close(); err != nil {
				t.Fatal(err)
			}

			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(exe, "-test.run=^TestSourcePurgeDatabaseKillChild$")
			cmd.Env = append(os.Environ(), "NDG_TEST_SOURCE_CUT="+cut, "NDG_TEST_SOURCE_DB="+database,
				"NDG_TEST_SOURCE_PLAN="+plan.ID, "NDG_TEST_SOURCE_DIGEST="+plan.Digest)
			out, err := cmd.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
				t.Fatalf("expected real SIGKILL, got %v %s", err, out)
			}

			reopened, err := store.Open(ctx, database)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			committed := cut == "after_database_commit"
			deleted := committed || cut == "after_filesystem_commit"
			wantState := "EXECUTING"
			if cut == "before_claim" {
				wantState = "DRY_RUN"
			} else if committed {
				wantState = "PURGED"
			}
			verifySourceCrashState(t, ctx, reopened, root, plan, wantState, deleted)
			for _, path := range []string{keeper, sentinel} {
				if got, err := os.ReadFile(path); err != nil || string(got) != contents {
					t.Fatal("keeper or outside sentinel changed", err)
				}
			}
		})
	}
}

func verifySourceCrashState(t *testing.T, ctx context.Context, st *store.SQLiteStore, root string, plan SourcePurgeResult, wantState string, deleted bool) {
	t.Helper()
	svc, err := NewSourcePurge(st, root)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := svc.Advance(ctx, plan.ID, plan.Digest, "RECEIPT", "")
	if err != nil || receipt.State != wantState {
		t.Fatal("unexpected persisted receipt", receipt.State, err)
	}
	caps, err := st.GovernanceCapabilities(ctx)
	if err != nil || caps.RecoveryLockActive != (wantState == "EXECUTING") {
		t.Fatal("unexpected recovery lock", err)
	}
	files, err := st.ListFiles(ctx, crashStorage)
	wantFiles := 2
	if wantState == "PURGED" {
		wantFiles = 1
	}
	if err != nil || len(files) != wantFiles {
		t.Fatalf("unexpected active index after interruption: %d, %v", len(files), err)
	}
	decision, err := st.ListAuthorizationDecisions(ctx, 1)
	if err != nil || len(decision) != 1 || decision[0].PlanID != plan.ID || decision[0].Detail != wantState {
		t.Fatal("unexpected persisted authorization transition", err)
	}
	for path, absent := range map[string]bool{
		filepath.Join(root, "target.pdf"):                           deleted,
		filepath.Join(root, ".ndg-purge-stage", "receipt-"+plan.ID): !deleted,
	} {
		_, err := os.Lstat(path)
		if (absent && !errors.Is(err, os.ErrNotExist)) || (!absent && err != nil) {
			t.Fatal("unexpected filesystem state after interruption", err)
		}
	}
	if wantState == "EXECUTING" {
		if _, err := svc.Advance(ctx, plan.ID, plan.Digest, "RECOVER", ""); !errors.Is(err, managedfs.ErrRecoveryRequired) {
			t.Fatal("ambiguous crash state must remain locked", err)
		}
	}
}

func TestSourcePurgeDatabaseKillChild(t *testing.T) {
	cut := os.Getenv("NDG_TEST_SOURCE_CUT")
	if cut == "" {
		return
	}
	database := os.Getenv("NDG_TEST_SOURCE_DB")
	if !strings.HasPrefix(database, filepath.Join(os.TempDir(), "TestSourcePurgePersistsAcrossDatabaseProcessKill")) ||
		filepath.Base(database) != "test.db" {
		t.Fatal("subprocess requires its dedicated synthetic fixture")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc, err := NewSourcePurge(st, filepath.Join(filepath.Dir(database), "temp"))
	if err != nil {
		t.Fatal(err)
	}
	p, proof, err := svc.read(ctx, os.Getenv("NDG_TEST_SOURCE_PLAN"), os.Getenv("NDG_TEST_SOURCE_DIGEST"))
	if err != nil || proof.Target.Relative != "target.pdf" || proof.Keeper.Relative != "keeper.pdf" {
		t.Fatal("subprocess requires approved fixture files", err)
	}
	stop := func(phase string) {
		if phase == cut {
			if err := syscall.Kill(os.Getpid(), syscall.SIGKILL); err != nil {
				t.Fatal(err)
			}
		}
	}
	release, err := st.AcquireExecutionLock()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	txn := managedfs.PurgeTransaction{ID: p.ID, ApprovalSHA256: p.Digest, RootPath: svc.root, Target: proof.Target, Keeper: &proof.Keeper}
	if err := managedfs.DryRunPurgeTransaction(ctx, txn); err != nil {
		t.Fatal(err)
	}
	stop("before_claim")
	if err := st.AdvanceSourcePurge(ctx, p, proof.Facts, "EXECUTING", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	stop("after_claim")
	p.State = "EXECUTING"
	result, err := managedfs.ExecutePurgeTransaction(ctx, txn)
	if err != nil || result.State != "PURGED" {
		t.Fatal("fixture filesystem commit failed", err)
	}
	stop("after_filesystem_commit")
	if _, err := svc.finish(ctx, p, proof, result.State); err != nil {
		t.Fatal(err)
	}
	stop("after_database_commit")
	t.Fatal("subprocess did not reach kill point")
}
