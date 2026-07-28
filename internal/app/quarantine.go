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

// QuarantineService manages the quarantine lifecycle: listing items,
// creating/approving/executing restore plans, and crash recovery.
//
// Per AGENTS.md rule 2, plan creation, approval, and execution are
// separate steps that must not be merged. The service preserves this
// separation: each method is an independent entry point.
//
// The service does NOT:
//   - Parse command-line flags (CLI's job)
//   - Write JSON audit/report files (CLI's job)
//   - Print to stdout/stderr (CLI's job)
type QuarantineService struct {
	store *store.SQLiteStore
}

// NewQuarantineService creates a quarantine service. The store is required
// for all operations.
func NewQuarantineService(st *store.SQLiteStore) *QuarantineService {
	return &QuarantineService{store: st}
}

// ListItems returns quarantine items, optionally filtered by lifecycle status.
func (s *QuarantineService) ListItems(ctx context.Context, status domain.QuarantineStatus) ([]domain.QuarantineItem, error) {
	return s.store.ListQuarantineItems(ctx, status)
}

// CreateRestorePlan builds a DRAFT restore plan for a single quarantine item.
// This is a read-only operation: no filesystem writes occur. The plan is
// persisted to the database and returned for CLI to write to a private file.
func (s *QuarantineService) CreateRestorePlan(ctx context.Context, itemID string) (*domain.RestorePlan, error) {
	if itemID == "" {
		return nil, fmt.Errorf("app: CreateRestorePlan: item-id is required")
	}
	item, err := s.store.GetQuarantineItem(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("app: get quarantine item: %w", err)
	}
	plan, err := purge.BuildRestorePlan(item, time.Now().UTC())
	if err != nil {
		return nil, fmt.Errorf("app: build restore plan: %w", err)
	}
	if err := s.store.SaveRestorePlan(ctx, plan); err != nil {
		return nil, fmt.Errorf("app: save restore plan: %w", err)
	}
	return &plan, nil
}

// ApproveRestorePlan transitions a DRAFT restore plan to APPROVED using
// the digest from the private DRAFT plan file. This is a database-only
// operation: no filesystem writes occur.
func (s *QuarantineService) ApproveRestorePlan(ctx context.Context, planID, digest string) error {
	if planID == "" || digest == "" {
		return fmt.Errorf("app: ApproveRestorePlan: plan-id and digest are required")
	}
	if err := s.store.ApproveRestorePlan(ctx, planID, digest, time.Now().UTC()); err != nil {
		return fmt.Errorf("app: approve restore plan: %w", err)
	}
	return nil
}

// RestoreExecuteInput defines parameters for executing a restore plan.
type RestoreExecuteInput struct {
	PlanID         string
	Digest         string
	QuarantineRoot string
	SourceRoots    []string
	DryRun         bool
}

// RestoreExecuteResult holds the outcome of a restore execution.
type RestoreExecuteResult struct {
	Result executor.RestoreResult
}

// ExecuteRestore runs a restore plan through the safe-operation pipeline.
// In dry-run mode, the plan is validated without filesystem writes. In real
// mode, the executor performs stale checks, isolation, verification, audit,
// and rollback per AGENTS.md rule 7.
//
// The digest is verified again at execution time to prevent replay attacks.
func (s *QuarantineService) ExecuteRestore(ctx context.Context, in RestoreExecuteInput) (*RestoreExecuteResult, error) {
	if in.PlanID == "" || in.Digest == "" || in.QuarantineRoot == "" || len(in.SourceRoots) == 0 {
		return nil, fmt.Errorf("app: ExecuteRestore: plan-id, digest, quarantine, and source-root are required")
	}
	plan, err := s.store.GetRestorePlan(ctx, in.PlanID)
	if err != nil {
		return nil, fmt.Errorf("app: get restore plan: %w", err)
	}
	if plan.ApprovalDigest != in.Digest {
		return nil, fmt.Errorf("app: restore execution digest rejected")
	}
	item, err := s.store.GetQuarantineItem(ctx, plan.ItemID)
	if err != nil {
		return nil, fmt.Errorf("app: get quarantine item: %w", err)
	}
	exec, err := executor.NewRestoreExecutor(in.QuarantineRoot, in.SourceRoots, s.store)
	if err != nil {
		return nil, fmt.Errorf("app: create restore executor: %w", err)
	}
	var result executor.RestoreResult
	if in.DryRun {
		result = exec.ValidateRestore(ctx, plan, item)
	} else {
		result = exec.ExecuteRestore(ctx, &plan, &item)
	}
	if result.Err != nil {
		return &RestoreExecuteResult{Result: result}, fmt.Errorf("restore failed: %s", result.ErrorType)
	}
	return &RestoreExecuteResult{Result: result}, nil
}

// RecoverRestoresInput defines parameters for crash recovery.
type RecoverRestoresInput struct {
	QuarantineRoot string
	SourceRoots    []string
}

// RecoverRestores reconciles non-terminal restore operations after a crash.
// Returns all results; the CLI is responsible for counting failures and
// deciding whether to require manual review.
func (s *QuarantineService) RecoverRestores(ctx context.Context, in RecoverRestoresInput) ([]executor.RestoreResult, error) {
	if in.QuarantineRoot == "" || len(in.SourceRoots) == 0 {
		return nil, fmt.Errorf("app: RecoverRestores: quarantine and source-root are required")
	}
	exec, err := executor.NewRestoreExecutor(in.QuarantineRoot, in.SourceRoots, s.store)
	if err != nil {
		return nil, fmt.Errorf("app: create restore executor: %w", err)
	}
	return exec.RecoverRestores(ctx), nil
}
