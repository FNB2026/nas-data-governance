package purge

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

// BuildRestorePlan creates one advisory restore plan. HOLD affects permanent
// deletion only; restoring a protected item is allowed and preferred.
func BuildRestorePlan(item domain.QuarantineItem, now time.Time) (domain.RestorePlan, error) {
	switch item.Status {
	case domain.QuarantineActive, domain.QuarantineHold, domain.QuarantinePurgeEligible:
	default:
		return domain.RestorePlan{}, fmt.Errorf("restore planner: item is not restorable")
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf(
		"%s\x00%s\x00%s\x00%d\x00%s", item.ID, item.QuarantinePath,
		item.SourcePath, item.FileSize, now.UTC().Format(time.RFC3339Nano))))
	plan := domain.RestorePlan{
		ID: fmt.Sprintf("restore-%x", sum[:12]), ItemID: item.ID,
		State: domain.RestoreDraft, QuarantinePath: item.QuarantinePath,
		RestorePath: item.SourcePath, ExpectedSHA256: item.ContentSHA256,
		ExpectedSize: item.FileSize, CreatedAt: now.UTC(),
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf(
		"%s\x00%s\x00%s\x00%s\x00%d", plan.ID, plan.ItemID,
		plan.QuarantinePath, plan.ExpectedSHA256, plan.ExpectedSize)))
	plan.ApprovalDigest = fmt.Sprintf("%x", digest[:])
	return plan, nil
}
