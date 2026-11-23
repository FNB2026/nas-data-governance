//go:build mas

package wails

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

func hashFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestMASSandboxReadWriteQuarantineRestoreLifecycle(t *testing.T) {
	a, _, source := sandboxTestAPI(t)
	cache := filepath.Join(source, "cache")
	if err := os.MkdirAll(cache, 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"cache/pair-a-1.bin": "same payload",
		"cache/pair-a-2.bin": "same payload",
		"cache/pair-b-1.bin": "other payload",
		"cache/pair-b-2.bin": "other payload",
		"single.bin":         "unique payload",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(source, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.PickDirectory(""); err != nil {
		t.Fatalf("pick source: %v", err)
	}
	if _, err := a.CreateProjectFromSource(CreateProjectInput{Name: "mas-rw", SourcePath: source}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	job, err := a.StartScan(StartScanRequest{Root: source})
	if err != nil {
		t.Fatalf("start scan: %v", err)
	}
	waitSandboxJob(t, a, job.JobID)

	caps, err := a.GetAppCapabilities()
	if err != nil {
		t.Fatal(err)
	}
	if !caps.CanScan || !caps.CanApprovePlans || !caps.CanExecuteQuarantine || caps.CanExecutePurge {
		t.Fatalf("unexpected MAS capabilities: %+v", caps)
	}
	if caps.DefaultQuarantineRoot == "" {
		t.Fatal("MAS quarantine root was not exposed")
	}

	drafts, err := a.SaveDraftPlans("")
	if err != nil {
		t.Fatalf("save draft plans: %v", err)
	}
	if len(drafts) == 0 {
		t.Fatal("no governance plans generated")
	}
	ids := make([]string, len(drafts))
	for i, p := range drafts {
		ids[i] = p.ID
	}
	if _, err := a.ApprovePlans(ApprovePlansRequest{PlanIDs: ids}); err != nil {
		t.Fatalf("approve plans: %v", err)
	}

	dry, err := a.ExecutePlans(ExecutePlansRequest{
		PlanIDs: ids, SourceRoots: []string{source}, DryRun: true, RetentionHours: 720,
	})
	if err != nil {
		t.Fatalf("dry-run execute: %v", err)
	}
	if dry.Failed != 0 {
		t.Fatalf("dry-run failed: %+v", dry)
	}
	for name := range files {
		if _, err := os.Stat(filepath.Join(source, name)); err != nil {
			t.Fatalf("dry-run changed %s: %v", name, err)
		}
	}
	if entries, err := os.ReadDir(caps.DefaultQuarantineRoot); err != nil || len(entries) != 0 {
		t.Fatalf("dry-run wrote quarantine entries=%d err=%v", len(entries), err)
	}

	realRun, err := a.ExecutePlans(ExecutePlansRequest{
		PlanIDs: ids, SourceRoots: []string{source}, RetentionHours: 720,
	})
	if err != nil {
		t.Fatalf("real execute: %v", err)
	}
	if realRun.Failed != 0 || realRun.Executed == 0 {
		t.Fatalf("real execute result: %+v", realRun)
	}

	items, err := a.ListQuarantineItems("")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Fatal("no quarantine items registered")
	}
	var item QuarantineItemDTO
	for _, candidate := range items {
		if _, err := os.Stat(candidate.SourcePath); os.IsNotExist(err) {
			item = candidate
			break
		}
	}
	if item.ID == "" {
		t.Fatal("no quarantined source disappeared")
	}
	if _, err := os.Stat(item.QuarantinePath); err != nil {
		t.Fatalf("quarantine file missing: %v", err)
	}
	if got, want := hashFile(t, item.QuarantinePath), item.ContentSHA256; got != want {
		t.Fatalf("quarantine hash=%s want=%s", got, want)
	}

	restorePlan, err := a.CreateRestorePlan(item.ID)
	if err != nil {
		t.Fatalf("create restore plan: %v", err)
	}
	if err := a.ApproveRestorePlan(restorePlan.ID, restorePlan.ApprovalDigest); err != nil {
		t.Fatalf("approve restore: %v", err)
	}
	restoreRun, err := a.ExecuteRestore(ExecuteRestoreRequest{
		PlanID: restorePlan.ID, Digest: restorePlan.ApprovalDigest, SourceRoots: []string{source},
	})
	if err != nil {
		t.Fatalf("execute restore: %v", err)
	}
	if restoreRun.Status != "ok" {
		t.Fatalf("restore result: %+v", restoreRun)
	}
	if _, err := os.Stat(item.SourcePath); err != nil {
		t.Fatalf("restored source missing: %v", err)
	}
	if got := hashFile(t, item.SourcePath); got != item.ContentSHA256 {
		t.Fatalf("restored hash=%s want=%s", got, item.ContentSHA256)
	}
}

func TestMASSandboxRecoveryAcquiresSourceLease(t *testing.T) {
	a, _, source := sandboxTestAPI(t)
	if err := os.WriteFile(filepath.Join(source, "pending.bin"), []byte("pending"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.PickDirectory(""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateProjectFromSource(CreateProjectInput{Name: "mas-recovery", SourcePath: source}); err != nil {
		t.Fatal(err)
	}
	if err := a.store.CreateTask(t.Context(), domain.OperationTask{ID: "reservation", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := a.store.SavePlans(t.Context(), "reservation", []domain.OperationPlan{{
		ID: "reservation-plan", TaskID: "reservation", State: domain.PlanExecuting,
	}}); err != nil {
		t.Fatal(err)
	}
	out, err := a.RecoverSourcePlans()
	if err != nil {
		t.Fatalf("recover source plans: %v", err)
	}
	if len(out) != 1 || out[0].Action != "reset_to_draft" {
		t.Fatalf("unexpected recovery: %+v", out)
	}
}
