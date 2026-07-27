package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/formatdiag"
	"github.com/FNB2026/nas-data-governance/internal/governancediag"
)

func TestBuildCriticalHoldRegisterOnlyFreezesCritical(t *testing.T) {
	plans := []domain.OperationPlan{
		{ID: "critical", State: domain.PlanDraft, Risk: domain.RiskCritical, Size: 10},
		{ID: "high", State: domain.PlanDraft, Risk: domain.RiskHigh, Size: 20},
	}
	got := buildCriticalHoldRegister(plans, time.Unix(0, 0))
	if got.Count != 1 || got.Bytes != 10 || got.Items[0].Status != "HOLD" || got.ExecutionAuthorized {
		t.Fatalf("unexpected register: %#v", got)
	}
}

func TestBuildColdCopyPlansCreatesBoundedDraftCopies(t *testing.T) {
	sourceRoot := t.TempDir()
	targetRoot := filepath.Join(t.TempDir(), "future-stage")
	source := filepath.Join(sourceRoot, "candidate.mp4")
	if err := os.WriteFile(source, []byte("cold-review-candidate"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	tiering := mediaTieringPlan{Items: []mediaTierItem{{
		StorageID: "test", Path: source, Size: info.Size(), Format: "mp4",
		Tier: coldReviewTier, Context: domain.DirectoryContext{Role: domain.RoleUnorganized},
	}}}
	files := []domain.FileInstance{{StorageID: "test", Path: source, Size: info.Size()}}
	plans, bytes, err := buildColdCopyPlans(tiering, files, sourceRoot, targetRoot, "wave-test", 1, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 || bytes != info.Size() {
		t.Fatalf("unexpected wave: plans=%d bytes=%d", len(plans), bytes)
	}
	plan := plans[0]
	if plan.State != domain.PlanDraft || plan.Risk != domain.RiskHigh || plan.Actions[0].Action != domain.OperationCopy {
		t.Fatalf("unexpected plan: %#v", plan)
	}
	if plan.Actions[0].File.ContentSHA256 == "" || filepath.Dir(plan.Actions[0].TargetPath) != targetRoot {
		t.Fatalf("missing integrity snapshot or target: %#v", plan.Actions[0])
	}
	if _, err := os.Stat(targetRoot); !os.IsNotExist(err) {
		t.Fatal("DRAFT preparation must not create the target root")
	}
}

func TestBuildColdCopyPlansRejectsProtectedCandidate(t *testing.T) {
	sourceRoot := t.TempDir()
	source := filepath.Join(sourceRoot, "protected.mov")
	if err := os.WriteFile(source, []byte("protected"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(source)
	tiering := mediaTieringPlan{Items: []mediaTierItem{{
		StorageID: "test", Path: source, Size: info.Size(), Format: "mov", Tier: coldReviewTier,
		Context: domain.DirectoryContext{Role: domain.RoleRaw, Protected: true},
	}}}
	files := []domain.FileInstance{{StorageID: "test", Path: source, Size: info.Size()}}
	if _, _, err := buildColdCopyPlans(tiering, files, sourceRoot, filepath.Join(t.TempDir(), "stage"), "wave-test", 1, 1<<20); err == nil {
		t.Fatal("protected candidate must be rejected")
	}
}

func TestBuildExtensionRenamePlansCreatesVerifiedDraft(t *testing.T) {
	sourceRoot := t.TempDir()
	source := filepath.Join(sourceRoot, "scan.pdf")
	if err := os.WriteFile(source, []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00\x01\x02\x03\x04\x05\x06\x07"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	mismatches := []formatdiag.ExtensionMismatch{{StorageID: "test", Path: source, Size: info.Size(), Extension: ".pdf", Detected: "jpeg"}}
	files := []domain.FileInstance{{StorageID: "test", Path: source, Size: info.Size()}}
	plans, bytes, err := buildExtensionRenamePlans(mismatches, files, sourceRoot, "wave-extension", "jpeg", 1, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 || bytes != info.Size() || plans[0].State != domain.PlanDraft || plans[0].Actions[0].Action != domain.OperationRename {
		t.Fatalf("unexpected plans: %#v", plans)
	}
	if filepath.Ext(plans[0].Actions[0].TargetPath) != ".jpg" || plans[0].Actions[0].File.ContentSHA256 == "" {
		t.Fatalf("unexpected action: %#v", plans[0].Actions[0])
	}
}

func TestBuildMediaTieringPlanKeepsProtectionAheadOfColdReview(t *testing.T) {
	review := governancediag.Report{LargeMediaReviews: []governancediag.LargeMediaReview{
		{Path: "/root/raw.mov", Size: 2 << 30, Format: domain.FormatInfo{Format: "mov", Category: domain.CategoryVideo}, Context: domain.DirectoryContext{Role: domain.RoleRaw, Protected: true}},
		{Path: "/root/unassigned.mov", Size: 2 << 30, Format: domain.FormatInfo{Format: "mov", Category: domain.CategoryVideo}},
	}}
	got := buildMediaTieringPlan(review, time.Unix(0, 0))
	if got.Summary["ONLINE_PROTECTED"] != 1 || got.Summary["COLD_STORAGE_REVIEW"] != 1 || got.ExecutionAuthorized {
		t.Fatalf("unexpected media plan: %#v", got)
	}
	if !got.Items[0].ProxyRecommended || !got.Items[1].ProxyRecommended {
		t.Fatal("large video proxy recommendation missing")
	}
}
