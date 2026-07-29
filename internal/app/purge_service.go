package app

import (
	"context"
	"fmt"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/executor"
	"github.com/FNB2026/nas-data-governance/internal/purge"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// PurgeService manages the purge lifecycle: creating/approving/executing
// purge plans for managed quarantine items that have reached their purge-
// eligibility date, and crash recovery.
//
// Per AGENTS.md rule 2, plan creation, approval, and execution are
// separate steps that must not be merged. Per the user's directive,
// purge is restricted to item-by-item permanent deletion within the
// managed quarantine area.
//
// The service does NOT:
//   - Parse command-line flags (CLI's job)
//   - Write JSON audit/report files (CLI's job)
//   - Print to stdout/stderr (CLI's job)
type PurgeService struct {
	store *store.SQLiteStore
}

// NewPurgeService creates a purge service. The store is required.
func NewPurgeService(st *store.SQLiteStore) *PurgeService {
	return &PurgeService{store: st}
}

// ListPlans returns all durable purge plans.
func (s *PurgeService) ListPlans(ctx context.Context) ([]domain.PurgePlan, error) {
	return s.store.ListPurgePlans(ctx)
}

// PurgeConfirmationText is the exact high-risk statement required for one
// permanent-deletion plan. The backend recomputes it at execution time.
func PurgeConfirmationText(plan domain.PurgePlan) string {
	return fmt.Sprintf("确认永久清理隔离区中的 1 个文件，共 %d 字节", plan.ExpectedSize)
}

// CreatePlans builds DRAFT purge plans for all purge-eligible quarantine
// items. This is a read-only operation: no filesystem writes occur. Plans
// are persisted to the database and returned for CLI to write to a file.
func (s *PurgeService) CreatePlans(ctx context.Context) ([]domain.PurgePlan, error) {
	now := time.Now().UTC()
	items, err := s.store.ListPurgeCandidates(ctx, now)
	if err != nil {
		return nil, fmt.Errorf("app: list purge candidates: %w", err)
	}
	plans := purge.BuildPlans(items, now)
	if len(plans) > 0 {
		if err := s.store.SavePurgePlans(ctx, plans); err != nil {
			return nil, fmt.Errorf("app: save purge plans: %w", err)
		}
	}
	return plans, nil
}

// ApprovePlan transitions a DRAFT purge plan to APPROVED using the digest
// from the private DRAFT plan file. This is a database-only operation.
func (s *PurgeService) ApprovePlan(ctx context.Context, planID, digest string) error {
	if planID == "" || digest == "" {
		return fmt.Errorf("app: ApprovePlan: plan-id and digest are required")
	}
	if err := s.store.ApprovePurgePlan(ctx, planID, digest, time.Now().UTC()); err != nil {
		return fmt.Errorf("app: approve purge plan: %w", err)
	}
	return nil
}

// PurgeExecuteInput defines parameters for executing a purge plan.
type PurgeExecuteInput struct {
	PlanID         string
	Digest         string
	QuarantineRoot string
	DryRun         bool
	Confirmation   string
}

// PurgeExecuteResult holds the outcome of a purge execution.
type PurgeExecuteResult struct {
	Result executor.PurgeResult
}

// ExecutePurge runs a purge plan through the safe-operation pipeline.
// In dry-run mode, the plan is validated without filesystem writes. In real
// mode, the executor performs stale checks, verification, audit, and
// rollback per AGENTS.md rule 7.
//
// The digest is verified again at execution time to prevent replay attacks.
// Purge is restricted to item-by-item permanent deletion within the managed
// quarantine area.
func (s *PurgeService) ExecutePurge(ctx context.Context, in PurgeExecuteInput) (*PurgeExecuteResult, error) {
	if in.PlanID == "" || in.Digest == "" || in.QuarantineRoot == "" {
		return nil, fmt.Errorf("app: ExecutePurge: plan-id, digest, and quarantine are required")
	}
	plan, err := s.store.GetPurgePlan(ctx, in.PlanID)
	if err != nil {
		return nil, fmt.Errorf("app: get purge plan: %w", err)
	}
	if plan.ApprovalDigest != in.Digest {
		return nil, fmt.Errorf("app: purge execution digest rejected")
	}
	if !in.DryRun {
		if plan.DryRunVerifiedAt == nil || plan.DryRunDigest != in.Digest {
			return nil, fmt.Errorf("app: successful dry-run is required before purge execution")
		}
		if in.Confirmation != PurgeConfirmationText(plan) {
			return nil, fmt.Errorf("app: purge confirmation rejected")
		}
	}
	item, err := s.store.GetQuarantineItem(ctx, plan.ItemID)
	if err != nil {
		return nil, fmt.Errorf("app: get quarantine item: %w", err)
	}
	exec, err := executor.NewPurgeExecutor(in.QuarantineRoot, s.store)
	if err != nil {
		return nil, fmt.Errorf("app: create purge executor: %w", err)
	}
	var result executor.PurgeResult
	if in.DryRun {
		result = exec.ValidatePurge(ctx, plan, item)
	} else {
		result = exec.ExecutePurge(ctx, &plan, &item)
	}
	if result.Err != nil {
		return &PurgeExecuteResult{Result: result}, fmt.Errorf("purge failed: %s", result.ErrorType)
	}
	if in.DryRun {
		if err := s.store.MarkPurgeDryRunVerified(ctx, plan.ID, in.Digest, time.Now().UTC()); err != nil {
			return nil, fmt.Errorf("app: persist purge dry-run: %w", err)
		}
	}
	return &PurgeExecuteResult{Result: result}, nil
}

// RecoverPurges reconciles non-terminal purge operations after a crash.
// Returns all results; the CLI is responsible for counting failures and
// deciding whether to require manual review.
func (s *PurgeService) RecoverPurges(ctx context.Context, quarantineRoot string) ([]executor.PurgeResult, error) {
	if quarantineRoot == "" {
		return nil, fmt.Errorf("app: RecoverPurges: quarantine is required")
	}
	exec, err := executor.NewPurgeExecutor(quarantineRoot, s.store)
	if err != nil {
		return nil, fmt.Errorf("app: create purge executor: %w", err)
	}
	return exec.RecoverPurges(ctx), nil
}
