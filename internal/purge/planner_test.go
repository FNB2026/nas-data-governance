package purge

import (
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

func TestBuildPlansOnlyExpiredUnprotectedItems(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	base := domain.QuarantineItem{
		ID: "q-1", QuarantinePath: "/q/a", ContentSHA256: "abc", FileSize: 3,
		Status: domain.QuarantineActive, RetainUntil: now.Add(-time.Hour),
	}
	hold := base
	hold.ID, hold.HoldReason = "q-hold", "protected_context"
	future := base
	future.ID, future.RetainUntil = "q-future", now.Add(time.Hour)
	purged := base
	purged.ID, purged.Status = "q-purged", domain.QuarantinePurged
	retry := base
	retry.ID, retry.Status = "q-retry", domain.QuarantinePurgeEligible

	plans := BuildPlans([]domain.QuarantineItem{base, hold, future, purged, retry}, now)
	if len(plans) != 2 {
		t.Fatalf("expected two plans, got %d", len(plans))
	}
	if plans[0].ItemID != base.ID || plans[0].State != domain.PurgeDraft {
		t.Fatalf("unexpected plan: %#v", plans[0])
	}
	if len(plans[0].ApprovalDigest) != 64 {
		t.Fatalf("expected full approval digest, got %q", plans[0].ApprovalDigest)
	}
}

func TestBuildPlansDeterministic(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	item := domain.QuarantineItem{
		ID: "q-1", QuarantinePath: "/q/a", ContentSHA256: "abc", FileSize: 3,
		Status: domain.QuarantineActive, RetainUntil: now.Add(-time.Hour),
	}
	a := BuildPlans([]domain.QuarantineItem{item}, now)
	b := BuildPlans([]domain.QuarantineItem{item}, now)
	if a[0].ID != b[0].ID || a[0].ApprovalDigest != b[0].ApprovalDigest {
		t.Fatalf("plans are not deterministic: %#v %#v", a[0], b[0])
	}
}
