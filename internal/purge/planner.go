// Package purge builds permanent-deletion proposals from expired managed
// quarantine items. It is advisory-only and never touches the filesystem.
package purge

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

// BuildPlans returns one DRAFT plan per expired, unprotected quarantine item.
// HOLD and non-active lifecycle items are omitted. PURGE_ELIGIBLE is accepted
// again only after a prior plan has rolled back.
func BuildPlans(items []domain.QuarantineItem, now time.Time) []domain.PurgePlan {
	plans := make([]domain.PurgePlan, 0)
	for _, item := range items {
		if (item.Status != domain.QuarantineActive && item.Status != domain.QuarantinePurgeEligible) ||
			item.HoldReason != "" || item.RetainUntil.After(now) {
			continue
		}
		id := purgePlanID(item, now)
		plan := domain.PurgePlan{
			ID: id, ItemID: item.ID, State: domain.PurgeDraft,
			ExpectedPath: item.QuarantinePath, ExpectedSHA256: item.ContentSHA256,
			ExpectedSize: item.FileSize, RetainUntil: item.RetainUntil.UTC(),
			CreatedAt: now.UTC(),
		}
		plan.ApprovalDigest = approvalDigest(plan)
		plans = append(plans, plan)
	}
	return plans
}

func purgePlanID(item domain.QuarantineItem, now time.Time) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf(
		"%s\x00%s\x00%d\x00%s\x00%s", item.ID, item.ContentSHA256, item.FileSize,
		item.RetainUntil.UTC().Format(time.RFC3339Nano),
		now.UTC().Format(time.RFC3339Nano))))
	return fmt.Sprintf("purge-%x", sum[:12])
}

func approvalDigest(plan domain.PurgePlan) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf(
		"%s\x00%s\x00%s\x00%d\x00%s\x00%s",
		plan.ID, plan.ItemID, plan.ExpectedSHA256, plan.ExpectedSize,
		plan.RetainUntil.UTC().Format(time.RFC3339Nano), plan.ExpectedPath)))
	return fmt.Sprintf("%x", sum[:])
}
