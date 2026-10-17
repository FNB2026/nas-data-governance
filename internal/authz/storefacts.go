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
//
// Lock ownership (the re-verification phase of the chained contract) is
// claimed only through an ExecutionLockScope acquired from this same store.
// There is deliberately no constructor that accepts a caller-supplied
// owns-callback: a callback can keep returning true after the lock is gone,
// which is the A6 window this design closes.
type storeFacts struct {
	store *store.SQLiteStore
	// scope is the real execution-lock reservation held by the calling
	// component, or nil when this fact source runs before the lock is
	// reserved. Ownership is evaluated freshly on every facts read, so a
	// scope released mid-chain stops being ownership for later reads.
	scope *ExecutionLockScope
}

// NewStoreFacts builds a FactSource from the durable store, with no execution
// lock ownership claim. This is the initial authorization gate.
func NewStoreFacts(st *store.SQLiteStore) FactSource {
	return &storeFacts{store: st}
}

// NewStoreFactsWithScope builds a FactSource for the re-verification phase of
// the chained contract, where the caller holds the execution lock through
// scope. The scope must have been acquired from the same store; a scope from
// another database, a zero-value scope, or a scope that has since been
// released is not ownership, and the plain lock probe then governs.
func NewStoreFactsWithScope(st *store.SQLiteStore, scope *ExecutionLockScope) FactSource {
	return &storeFacts{store: st, scope: scope}
}

// ownsExecutionLock reports that the calling component really holds the
// execution lock of exactly this store right now. A held probe result that
// this predicate covers reflects our own reservation, which flock exclusivity
// guarantees no competing writer shares. Journal-based recovery evidence
// still blocks independently.
func (f *storeFacts) ownsExecutionLock() bool {
	return f.scope.Covers(f.store)
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
	// unknown state is an error. Our own reservation — proven by a scope
	// acquired from this same store and not yet released — is not a
	// competing writer's.
	held, err := f.store.ExecutionLockHeld()
	if err != nil {
		return Facts{}, err
	}
	if held && !f.ownsExecutionLock() {
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
