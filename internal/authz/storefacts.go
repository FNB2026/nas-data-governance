package authz

import (
	"context"
	"errors"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// storeFacts is the production FactSource backed by the project's single source
// of authorization truth: the durable SQLiteStore. It is read-only; it never
// writes, approves, or dispatches.
//
// The recovery-lock check mirrors requireNoRecovery (pending executing plans,
// pending restores, recoverable purge journals) and additionally probes the
// execution lock file, so the verifier refuses while any recovery or execution
// writer is active.
type storeFacts struct {
	store *store.SQLiteStore
}

// NewStoreFacts builds a FactSource from the durable store.
func NewStoreFacts(st *store.SQLiteStore) FactSource {
	return &storeFacts{store: st}
}

func (f *storeFacts) ReadOnlyFacts(ctx context.Context, op OpKind, planID string) (Facts, error) {
	// Only PURGE has a durable approval contract today. A purge approval is not
	// a SEAL/INSPECT/RESTORE approval: binding the operation type here prevents
	// one plan kind from authorizing a different dangerous operation.
	if op != OpPurge {
		return Facts{}, nil
	}
	plan, err := f.store.GetPurgePlan(ctx, planID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// Unknown plan: not an approved authorization.
			return Facts{}, nil
		}
		return Facts{}, err
	}
	facts := Facts{
		PlanID:           plan.ID,
		PlanState:        string(plan.State),
		ExpectedDigest:   plan.ApprovalDigest,
		DryRunDigest:     plan.DryRunDigest,
		DryRunVerifiedAt: plan.DryRunVerifiedAt,
		RetainUntil:      plan.RetainUntil,
		ExpectedObjectID: plan.ItemID,
		ExpectedSHA256:   plan.ExpectedSHA256,
		ExpectedSize:     plan.ExpectedSize,
		ExpectedPath:     plan.ExpectedPath,
		PlanRevoked:      plan.State == domain.PurgeRolledBack || plan.State == domain.PurgeFailed,
		RequiredCopies:   2, // approval digest + dry-run digest
	}
	valid := 1
	if plan.DryRunVerifiedAt != nil && plan.DryRunDigest != "" {
		valid = 2
	}
	facts.ValidCopies = valid

	// Recovery lock: mirror requireNoRecovery plus the execution lock probe.
	if held := f.store.ExecutionLockHeld(); held {
		facts.RecoveryLockHeld = true
	}
	if !facts.RecoveryLockHeld {
		if ids, e := f.store.ListExecutingPlans(ctx); e == nil && len(ids) > 0 {
			facts.RecoveryLockHeld = true
		}
	}
	if !facts.RecoveryLockHeld {
		if rs, e := f.store.ListPendingRestores(ctx); e == nil && len(rs) > 0 {
			facts.RecoveryLockHeld = true
		}
	}
	if !facts.RecoveryLockHeld {
		if ps, e := f.store.ListRecoverablePurges(ctx); e == nil && len(ps) > 0 {
			facts.RecoveryLockHeld = true
		}
	}
	return facts, nil
}
