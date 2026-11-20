//go:build linux

package managedfs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func purgeFixture(t *testing.T) (context.Context, PurgeTransaction, string) {
	t.Helper()
	if os.Geteuid() == 0 || os.Getegid() == 0 {
		t.Skip("worker runs under a non-root account")
	}
	ctx := context.Background()
	sandbox := t.TempDir()
	root := filepath.Join(sandbox, "source")
	for _, dir := range []string{root, filepath.Join(root, purgeStage), filepath.Join(root, "folder")} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	sentinel := filepath.Join(sandbox, "outside-sentinel")
	for _, name := range []string{"folder/remove", "folder/keeper"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("synthetic duplicate"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(sentinel, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	target, err := ObservePurgeTarget(ctx, root, "folder/remove")
	if errors.Is(err, ErrUnsupported) && os.Getenv("NDG_REQUIRE_MANAGED_READER_KERNEL") == "" {
		t.Skip("kernel mount identity unavailable")
	}
	if err != nil {
		t.Fatal("cannot bind synthetic target", err)
	}
	keeper, err := ObservePurgeTarget(ctx, root, "folder/keeper")
	if err != nil {
		t.Fatal("cannot bind synthetic keeper", err)
	}
	t.Cleanup(func() {
		raw, err := os.ReadFile(sentinel)
		if err != nil || string(raw) != "outside" {
			t.Error("outside sentinel changed")
		}
	})
	return ctx, PurgeTransaction{
		ID: strings.Repeat("1", 64), ApprovalSHA256: strings.Repeat("2", 64),
		RootPath: root, Target: target, Keeper: &keeper,
	}, filepath.Join(root, purgeStage)
}

func TestRootedPurgeCommitsExactlyOneFile(t *testing.T) {
	ctx, plan, stage := purgeFixture(t)
	result, err := ExecutePurgeTransaction(ctx, plan)
	if err != nil || result.State != "PURGED" {
		t.Fatalf("synthetic deletion did not commit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(plan.RootPath, plan.Target.Relative)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("selected synthetic file survived")
	}
	keeper, err := ObservePurgeTarget(ctx, plan.RootPath, plan.Keeper.Relative)
	if err != nil || keeper != *plan.Keeper {
		t.Fatal("keeper changed", err)
	}
	if _, err := os.Stat(filepath.Join(stage, "receipt-"+plan.ID)); err != nil {
		t.Fatal("missing durable receipt")
	}
	if _, err := ExecutePurgeTransaction(ctx, plan); !errors.Is(err, ErrRejected) {
		t.Fatal("replay was not refused", err)
	}
}

func TestRootedPurgeRejectsStaleTargetAndAliases(t *testing.T) {
	ctx, plan, stage := purgeFixture(t)
	target := filepath.Join(plan.RootPath, plan.Target.Relative)
	alias := filepath.Join(plan.RootPath, "alias")
	if err := os.Link(target, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := ObservePurgeTarget(ctx, plan.RootPath, plan.Target.Relative); !errors.Is(err, ErrRejected) {
		t.Fatal("hardlink admitted", err)
	}
	if err := os.Symlink(target, filepath.Join(plan.RootPath, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := ObservePurgeTarget(ctx, plan.RootPath, "link"); !errors.Is(err, ErrRejected) {
		t.Fatal("symlink admitted", err)
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ExecutePurgeTransaction(ctx, plan); !errors.Is(err, ErrRejected) {
		t.Fatal("stale target admitted", err)
	}
	if !purgeEntryMissing(t, stage, "pending") {
		t.Fatal("stale refusal started mutation")
	}
}

func purgeEntryMissing(t *testing.T, base, relative string) bool {
	t.Helper()
	_, err := os.Lstat(filepath.Join(base, relative))
	return errors.Is(err, os.ErrNotExist)
}

func TestRootedPurgeRetainsEvidenceAcrossCommitBoundaries(t *testing.T) {
	for _, cut := range []string{"pending", "staged", "commit_pending", "unlinked"} {
		t.Run(cut, func(t *testing.T) {
			ctx, plan, stage := purgeFixture(t)
			_, err := executePurgeTransaction(ctx, plan, func(phase string) error {
				if phase == cut {
					return errors.New("synthetic fault")
				}
				return nil
			})
			if !errors.Is(err, ErrRecoveryRequired) || purgeEntryMissing(t, stage, "pending") {
				t.Fatalf("fault did not retain recovery evidence: %v", err)
			}
			if got := purgeEntryMissing(t, plan.RootPath, plan.Target.Relative); got != (cut != "pending") {
				t.Fatal("unexpected source state")
			}
			if got := purgeEntryMissing(t, stage, plan.ID); got != (cut == "pending" || cut == "unlinked") {
				t.Fatal("unexpected staging state")
			}
			if !purgeEntryMissing(t, stage, "receipt-"+plan.ID) {
				t.Fatal("ambiguous state reported a successful purge")
			}
			if _, err := ExecutePurgeTransaction(ctx, plan); !errors.Is(err, ErrRecoveryRequired) {
				t.Fatal("unresolved state did not block retry", err)
			}
		})
	}
}

func TestRootedPurgeRecoversOnlyBeforeCommit(t *testing.T) {
	for _, cut := range []string{"pending", "staged", "commit_pending"} {
		t.Run(cut, func(t *testing.T) {
			ctx, plan, stage := purgeFixture(t)
			_, err := executePurgeTransaction(ctx, plan, func(phase string) error {
				if phase == cut {
					return errors.New("synthetic fault")
				}
				return nil
			})
			if !errors.Is(err, ErrRecoveryRequired) {
				t.Fatal("fixture did not interrupt", err)
			}
			result, err := RecoverPurgeTransaction(ctx, plan)
			if cut == "commit_pending" {
				if !errors.Is(err, ErrRecoveryRequired) || purgeEntryMissing(t, stage, plan.ID) {
					t.Fatal("post-commit staging was not preserved", err)
				}
				return
			}
			if err != nil || result.State != "ROLLED_BACK" || !purgeEntryMissing(t, stage, "pending") {
				t.Fatal("pre-commit recovery failed", err)
			}
			restored, err := ObservePurgeTarget(ctx, plan.RootPath, plan.Target.Relative)
			if err != nil || restored.File != plan.Target.File || restored.SHA256 != plan.Target.SHA256 {
				t.Fatal("recovery failed to restore exact object", err)
			}
		})
	}
}

func TestRootedPurgeConcurrentExecutionRefused(t *testing.T) {
	ctx, plan, _ := purgeFixture(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		_, err := executePurgeTransaction(ctx, plan, func(phase string) error {
			if phase == "pending" {
				close(entered)
				<-release
			}
			return nil
		})
		finished <- err
	}()
	select {
	case <-entered:
	case err := <-finished:
		t.Fatal("first execution failed before holding the lease", err)
	}
	_, concurrentErr := ExecutePurgeTransaction(ctx, plan)
	close(release)
	executionErr := <-finished
	if !errors.Is(concurrentErr, ErrBusy) || executionErr != nil {
		t.Fatalf("unexpected concurrent outcome: %v, %v", concurrentErr, executionErr)
	}
}
