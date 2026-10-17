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
// Every store failure is returned as an error. A query that cannot prove the
// recovery lock is free must refuse — a silent "no recovery pending" would let
// an execution proceed with unknown recovery state.
//
// The recovery-lock check mirrors requireNoRecovery (pending executing plans,
// pending restores, recoverable purge journals) and additionally probes the
// execution lock file, which reports three states: confirmed free, confirmed
// held, and unknown (an error, and therefore a refusal).
type storeFacts struct {
	store *store.SQLiteStore
	// ownsExecutionLock reports that the calling component already holds the
	// execution lock — the re-verification phase of the chained contract.
	// When true, a "held" probe result reflects our own reservation, which
	// flock exclusivity guarantees no competing writer shares. Journal-based
	// recovery evidence still blocks independently.
	ownsExecutionLock func() bool
}

// NewStoreFacts builds a FactSource from the durable store.
func NewStoreFacts(st *store.SQLiteStore) FactSource {
	return &storeFacts{store: st}
}

// NewStoreFactsOwningExecutionLock builds a FactSource for the re-verification
// phase of the chained contract, where the caller already holds the execution
// lock. The owns callback must report exactly the span during which the caller
// holds the lock.
func NewStoreFactsOwningExecutionLock(st *store.SQLiteStore, owns func() bool) FactSource {
	return &storeFacts{store: st, ownsExecutionLock: owns}
}

func (f *storeFacts) ReadOnlyFacts(ctx context.Context, op OpKind, planID string) (Facts, error) {
	// Only PURGE has a durable approval contract today. A purge approval is
	// not a SEAL/INSPECT/RESTORE approval: binding the operation type here
	// prevents one plan kind from authorizing a different dangerous operation.
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
	item, itemErr := f.store.GetQuarantineItem(ctx, plan.ItemID)
	if itemErr != nil && !errors.Is(itemErr, store.ErrNotFound) {
		return Facts{}, itemErr
	}

	planSize := plan.ExpectedSize
	facts := Facts{
		Operation:             OpPurge,
		PlanID:                plan.ID,
		PlanState:             string(plan.State),
		PlanRevoked:           plan.State == domain.PurgeRolledBack || plan.State == domain.PurgeFailed,
		ApprovedAt:            plan.ApprovedAt,
		ExpectedDigest:        plan.ApprovalDigest,
		DryRunDigest:          plan.DryRunDigest,
		DryRunVerifiedAt:      plan.DryRunVerifiedAt,
		RetainUntil:           plan.RetainUntil,
		ExpectedObjectID:      plan.ItemID,
		ExpectedSHA256:        plan.ExpectedSHA256,
		ExpectedSize:          &planSize,
		ExpectedPath:          plan.ExpectedPath,
		RequiredCorroboration: 2, // approval record + dry-run record
	}
	if itemErr == nil {
		facts.ItemStatus = string(item.Status)
		facts.ItemHoldReason = item.HoldReason
		facts.ItemSHA256 = item.ContentSHA256
		facts.ItemSize = item.FileSize
		facts.ItemPath = item.QuarantinePath
		facts.ItemRetainUntil = item.RetainUntil
		facts.SourcePath = item.SourcePath
	}

	// Approval corroboration counts durable authorization records: the
	// approval record plus the dry-run verification record. Their digest
	// consistency and time ordering are verified by Verifier.Check; this is
	// never a count of retainable file copies.
	corroboration := 1
	if plan.DryRunVerifiedAt != nil && plan.DryRunDigest != "" {
		corroboration = 2
	}
	facts.ApprovalCorroboration = corroboration

	// Execution lock: the probe distinguishes free / held / unknown, and an
	// unknown state is an error. Our own reservation is not a competing
	// writer's.
	held, err := f.store.ExecutionLockHeld()
	if err != nil {
		return Facts{}, err
	}
	if held && !(f.ownsExecutionLock != nil && f.ownsExecutionLock()) {
		facts.RecoveryLockHeld = true
	}

	// Durable recovery evidence, mirroring requireNoRecovery. Every query
	// failure is an error — never a silent "no recovery pending".
	ids, err := f.store.ListExecutingPlans(ctx)
	if err != nil {
		return Facts{}, err
	}
	if len(ids) > 0 {
		facts.RecoveryLockHeld = true
	}
	rs, err := f.store.ListPendingRestores(ctx)
	if err != nil {
		return Facts{}, err
	}
	if len(rs) > 0 {
		facts.RecoveryLockHeld = true
	}
	ps, err := f.store.ListRecoverablePurges(ctx)
	if err != nil {
		return Facts{}, err
	}
	if len(ps) > 0 {
		facts.RecoveryLockHeld = true
	}
	return facts, nil
}
