//go:build linux

package managedfs

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/scanner"
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

func TestRootedPurgeRejectsReplacedParentAndRoot(t *testing.T) {
	for _, replace := range []string{"parent", "root"} {
		t.Run(replace, func(t *testing.T) {
			ctx, plan, _ := purgeFixture(t)
			p := plan.RootPath
			if replace == "parent" {
				p = filepath.Join(p, "folder")
			}
			if err := os.Rename(p, p+"-original"); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(p, 0700); err != nil {
				t.Fatal(err)
			}
			if replace == "root" {
				for _, name := range []string{purgeStage, "folder"} {
					if err := os.Mkdir(filepath.Join(p, name), 0700); err != nil {
						t.Fatal(err)
					}
				}
			}
			substitution := filepath.Join(plan.RootPath, plan.Target.Relative)
			if err := os.WriteFile(substitution, []byte("synthetic duplicate"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := ExecutePurgeTransaction(ctx, plan); !errors.Is(err, ErrRejected) {
				t.Fatal("replacement with identical bytes admitted", err)
			}
			if _, err := os.Lstat(substitution); err != nil {
				t.Fatal("replacement was modified", err)
			}
		})
	}
}

func TestRootedPurgeRefusesRealSameDeviceBind(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "true" || os.Getenv("RUNNER_ENVIRONMENT") != "github-hosted" {
		t.Skip("privileged synthetic mount test runs only on a disposable GitHub runner")
	}
	dir, err := os.MkdirTemp("", "ndg-purge-bind-")
	if err != nil {
		t.Fatal(err)
	}
	root, outside := filepath.Join(dir, "root"), filepath.Join(dir, "outside")
	mount := filepath.Join(root, "nested")
	for _, p := range []string{root, mount, outside} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	sentinel := filepath.Join(outside, "synthetic")
	if err := os.WriteFile(sentinel, []byte("outside sentinel"), 0600); err != nil {
		t.Fatal(err)
	}
	attached := false
	defer func() {
		if attached {
			if err := exec.Command("sudo", "-n", "umount", "--", mount).Run(); err != nil {
				t.Error("synthetic bind failed to detach; retaining fixture", err)
				return
			}
		}
		if err := os.RemoveAll(dir); err != nil {
			t.Error("synthetic fixture cleanup failed", err)
		}
	}()
	if err := exec.Command("sudo", "-n", "mount", "--bind", "--", outside, mount).Run(); err != nil {
		t.Fatal("synthetic bind failed", err)
	}
	attached = true
	var outer, inner unix.Stat_t
	if unix.Stat(root, &outer) != nil || unix.Stat(mount, &inner) != nil || outer.Dev != inner.Dev {
		t.Fatal("fixture did not establish same-device bind")
	}
	if _, err := ObservePurgeTarget(context.Background(), root, "nested/synthetic"); !errors.Is(err, ErrRejected) {
		t.Fatal("nested mount admitted", err)
	}
	safe := filepath.Join(root, "safe")
	if err := os.WriteFile(safe, []byte("safe fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	var indexed []string
	_, err = scanner.Scan(context.Background(), scanner.Options{Root: root}, func(f domain.FileInstance) error {
		indexed = append(indexed, f.Path)
		return nil
	})
	if err != nil || len(indexed) != 1 || indexed[0] != safe {
		t.Fatal("scanner did not exclude the nested mount", err, indexed)
	}
	boundary, err := scanner.OpenBoundary(root)
	if err != nil {
		t.Fatal(err)
	}
	defer boundary.Close()
	if err := boundary.ReadFile(domain.FileInstance{Path: filepath.Join(mount, "synthetic")}, func(*os.File) error {
		t.Fatal("confined reader received a descriptor outside the mount")
		return nil
	}); !errors.Is(err, scanner.ErrMountBoundary) {
		t.Fatal("confined read did not reject the nested mount", err)
	}
	data, err := os.ReadFile(sentinel)
	if err != nil || string(data) != "outside sentinel" {
		t.Fatal("outside sentinel changed")
	}
}

func TestRootedPurgeRejectsSharedWritableNamespace(t *testing.T) {
	for _, relative := range []string{".", "folder", "folder/remove", "folder/keeper"} {
		t.Run(relative, func(t *testing.T) {
			ctx, plan, stage := purgeFixture(t)
			changed := filepath.Join(plan.RootPath, relative)
			original, err := os.Stat(changed)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(changed, original.Mode().Perm()|0020); err != nil {
				t.Fatal(err)
			}
			if _, err := ExecutePurgeTransaction(ctx, plan); !errors.Is(err, ErrRejected) {
				t.Fatalf("expected shared-writable component to be rejected, got %v", err)
			}
			if _, err := os.Stat(filepath.Join(stage, "pending")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("expected rejection before journal creation, got %v", err)
			}
			if _, err := os.Stat(filepath.Join(plan.RootPath, plan.Target.Relative)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRootedPurgeProcessKill(t *testing.T) {
	for _, cut := range []string{"pending", "renamed", "parent_synced", "staged", "commit_pending",
		"unlinked_unsynced", "unlinked", "receipt_synced", "pending_removed"} {
		t.Run(cut, func(t *testing.T) {
			ctx, plan, stage := purgeFixture(t)
			raw, err := json.Marshal(plan)
			if err != nil {
				t.Fatal(err)
			}
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(exe, "-test.run=^TestRootedPurgeKillChild$")
			cmd.Env = append(os.Environ(), "NDG_PURGE_TEST_CUT="+cut, "NDG_PURGE_TEST_PLAN="+string(raw))
			output, err := cmd.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
				t.Fatalf("expected real SIGKILL, got %v %s", err, output)
			}
			if purgeEntryMissing(t, stage, "pending") != (cut == "pending_removed") {
				t.Fatal("journal does not match the interrupted phase")
			}
			keeper, err := ObservePurgeTarget(ctx, plan.RootPath, plan.Keeper.Relative)
			if err != nil || keeper != *plan.Keeper {
				t.Fatal("interrupted purge changed its keeper", err)
			}
			result, err := RecoverPurgeTransaction(ctx, plan)
			switch cut {
			case "pending", "renamed", "parent_synced", "staged":
				if err != nil || result.State != "ROLLED_BACK" ||
					purgeEntryMissing(t, plan.RootPath, plan.Target.Relative) || !purgeEntryMissing(t, stage, plan.ID) {
					t.Fatal("did not safely restore the synthetic target", err)
				}
			default:
				if !errors.Is(err, ErrRecoveryRequired) || !purgeEntryMissing(t, plan.RootPath, plan.Target.Relative) {
					t.Fatal("accepted ambiguous post-commit outcome", err)
				}
			}
		})
	}
}

func TestRootedPurgeKillChild(t *testing.T) {
	cut := os.Getenv("NDG_PURGE_TEST_CUT")
	if cut == "" {
		return
	}
	var plan PurgeTransaction
	if json.Unmarshal([]byte(os.Getenv("NDG_PURGE_TEST_PLAN")), &plan) != nil ||
		!strings.HasPrefix(plan.RootPath, filepath.Join(os.TempDir(), "TestRootedPurgeProcessKill")) ||
		plan.Target.Relative != "folder/remove" {
		t.Fatal("subprocess requires its dedicated synthetic fixture")
	}
	_, err := executePurgeTransaction(context.Background(), plan, func(phase string) error {
		if phase == cut {
			return unix.Kill(os.Getpid(), unix.SIGKILL)
		}
		return nil
	})
	t.Fatal("subprocess did not reach kill point", err)
}

func TestRootedPurgeRollbackProcessKill(t *testing.T) {
	for _, tc := range []struct {
		name, cut string
		damage    func(*testing.T, PurgeTransaction, string)
	}{
		{name: "intent_synced", cut: "rollback_intent_synced"},
		{name: "renamed", cut: "rollback_renamed"},
		{name: "before_receipt", cut: "rollback_before_receipt"},
		{name: "receipt_synced", cut: "rollback_receipt_synced"},
		{name: "replaced_after_rename", cut: "rollback_renamed", damage: func(t *testing.T, plan PurgeTransaction, _ string) {
			name := filepath.Join(plan.RootPath, plan.Target.Relative)
			raw, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(name, name+".saved"); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(name, raw, 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "corrupt_receipt", cut: "rollback_receipt_synced", damage: func(t *testing.T, plan PurgeTransaction, stage string) {
			if err := os.WriteFile(filepath.Join(stage, "receipt-"+plan.ID), []byte("{"), 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "corrupt_intent", cut: "rollback_renamed", damage: func(t *testing.T, plan PurgeTransaction, stage string) {
			if err := os.WriteFile(filepath.Join(stage, "rollback-"+plan.ID), []byte("{"), 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "root_replaced", cut: "rollback_renamed", damage: func(t *testing.T, plan PurgeTransaction, _ string) {
			if err := os.Rename(plan.RootPath, plan.RootPath+".saved"); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(plan.RootPath, 0700); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "unlink_without_receipt", cut: "unlinked"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, plan, stage := purgeFixture(t)
			if tc.cut != "unlinked" {
				_, err := executePurgeTransaction(ctx, plan, func(phase string) error {
					if phase == "staged" {
						return ErrRecoveryRequired
					}
					return nil
				})
				if !errors.Is(err, ErrRecoveryRequired) || purgeEntryMissing(t, stage, plan.ID) {
					t.Fatal("fixture did not reach staged state", err)
				}
			}
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(plan)
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(exe, "-test.run=^TestRootedPurgeRollbackKillChild$")
			cmd.Env = append(os.Environ(), "NDG_TEST_ROLLBACK_CUT="+tc.cut, "NDG_TEST_ROLLBACK_PLAN="+string(raw))
			out, err := cmd.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
				t.Fatalf("expected actual SIGKILL, got %v %s", err, out)
			}
			if tc.damage != nil {
				tc.damage(t, plan, stage)
			}
			blocked := tc.damage != nil || tc.cut == "unlinked"
			for i := 0; i < 3; i++ {
				r, err := ReconcilePurgeTransaction(ctx, plan)
				if blocked {
					if !errors.Is(err, ErrRecoveryRequired) || r != (PurgeReceipt{}) {
						t.Fatal("accepted incomplete or changed recovery evidence", r, err)
					}
				} else if err != nil || r.ID != plan.ID || r.State != "ROLLED_BACK" {
					t.Fatal("did not recover identical terminal state", r, err)
				}
			}
			if tc.name != "root_replaced" && purgeEntryMissing(t, stage, "pending") == blocked {
				t.Fatal("pending state does not reflect recovery outcome")
			}
			if !blocked {
				target, err := ObservePurgeTarget(ctx, plan.RootPath, plan.Target.Relative)
				if err != nil || !samePurgeExceptCTime(target, plan.Target) {
					t.Fatal("recovery did not preserve the original file", err)
				}
			}
		})
	}
}

func TestRootedPurgeRollbackKillChild(t *testing.T) {
	cut := os.Getenv("NDG_TEST_ROLLBACK_CUT")
	if cut == "" {
		return
	}
	var plan PurgeTransaction
	if json.Unmarshal([]byte(os.Getenv("NDG_TEST_ROLLBACK_PLAN")), &plan) != nil ||
		!strings.HasPrefix(plan.RootPath, filepath.Join(os.TempDir(), "TestRootedPurgeRollbackProcessKill")) ||
		plan.Target.Relative != "folder/remove" {
		t.Fatal("subprocess requires its dedicated synthetic fixture")
	}
	stop := func(phase string) error {
		if phase == cut {
			return unix.Kill(os.Getpid(), unix.SIGKILL)
		}
		return nil
	}
	if cut == "unlinked" {
		_, err := executePurgeTransaction(context.Background(), plan, stop)
		t.Fatal("did not reach synthetic unlink checkpoint", err)
	}
	_, err := reconcilePurgeWithFault(context.Background(), plan, false, stop)
	t.Fatal("did not reach synthetic rollback checkpoint", err)
}
