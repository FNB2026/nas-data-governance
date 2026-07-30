package app

import (
	"context"

	"github.com/FNB2026/nas-data-governance/internal/executor"
)

// RecoveryStore is the store subset needed for crash recovery. It matches
// executor.RecoveryStore (Journal + GetPlan + UpdatePlanState).
// store.SQLiteStore satisfies this interface structurally.
type RecoveryStore interface {
	executor.RecoveryStore
}

// RecoveryService handles crash recovery for plans left in EXECUTING state.
// It wraps executor.Recover with a clean service boundary so the CLI and
// desktop can both call it without knowing executor internals.
type RecoveryService struct {
	store RecoveryStore
}

// NewRecoveryService creates a recovery service backed by the given store.
func NewRecoveryService(st RecoveryStore) *RecoveryService {
	return &RecoveryService{store: st}
}

// Recover scans for plans in EXECUTING state and brings them to a safe,
// deterministic state:
//   - If any filesystem action was done → undo all in reverse, mark ROLLED_BACK.
//   - If no action was done → reset to APPROVED for re-execution.
//
// Returns one RecoveryResult per inspected plan.
func (s *RecoveryService) Recover(ctx context.Context) ([]executor.RecoveryResult, error) {
	exec := executor.NewForRecovery()
	return exec.Recover(ctx, s.store), nil
}
