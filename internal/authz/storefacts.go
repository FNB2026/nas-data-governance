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
// Operation binding is structural: a facts read for PURGE consults only the
// purge plan tables, a facts read for RESTORE consults only the restore plan
// tables. One plan kind can never authorize the other operation because the
// other operation's fact source simply does not know the plan. SEAL and
// INSPECT have no durable approval contract yet and resolve to empty facts
// (wrong_plan).
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
	switch op {
	case OpPurge:
		return f.purgeFacts(ctx, planID)
	case OpRestore:
		return f.restoreFacts(ctx, planID)
	default:
		// Only PURGE and RESTORE have durable approval contracts today. An
		// approval of one kind is not an approval of another: binding the
		// operation here prevents one plan kind from authorizing a different
		// dangerous operation.
		return Facts{}, nil
	}
}

// readItem loads the quarantine item a plan points at. A missing item yields
// itemErr == store.ErrNotFound with no error: the verifier refuses on the
// empty item state. Any other read failure is an error — a refusal.
func (f *storeFacts) readItem(ctx context.Context, itemID string) (domain.QuarantineItem, error) {
	item, err := f.store.GetQuarantineItem(ctx, itemID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return domain.QuarantineItem{}, err
	}
	return item, nil
}

// recoveryEvidence mirrors requireNoRecovery and fills RecoveryLockHeld from
// the execution-lock probe plus the three durable journal queries. Every
// query failure is an error — never a silent "no recovery pending".
func (f *storeFacts) recoveryEvidence(ctx context.Context, facts *Facts) error {
	held, err := f.store.ExecutionLockHeld()
	if err != nil {
		return err
	}
	if held && !f.ownsExecutionLock() {
		facts.RecoveryLockHeld = true
	}
	ids, err := f.store.ListExecutingPlans(ctx)
	if err != nil {
		return err
	}
	if len(ids) > 0 {
		facts.RecoveryLockHeld = true
	}
	rs, err := f.store.ListPendingRestores(ctx)
	if err != nil {
		return err
	}
	if len(rs) > 0 {
		facts.RecoveryLockHeld = true
	}
	ps, err := f.store.ListRecoverablePurges(ctx)
	if err != nil {
		return err
	}
	if len(ps) > 0 {
		facts.RecoveryLockHeld = true
	}
	return nil
}

// purgeFacts builds the durable facts of a purge plan. A purge approval is
// not a restore approval: this source consults only purge tables.
func (f *storeFacts) purgeFacts(ctx context.Context, planID string) (Facts, error) {
	plan, err := f.store.GetPurgePlan(ctx, planID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// Unknown plan: not an approved authorization.
			return Facts{}, nil
		}
		return Facts{}, err
	}
	item, itemErr := f.readItem(ctx, plan.ItemID)
	if itemErr != nil {
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

	if err := f.recoveryEvidence(ctx, &facts); err != nil {
		return Facts{}, err
	}
	return facts, nil
}

// restoreFacts builds the durable facts of a restore plan. A restore approval
// is not a purge approval: this source consults only restore tables, and the
// facts carry no purge retention semantics.
func (f *storeFacts) restoreFacts(ctx context.Context, planID string) (Facts, error) {
	plan, err := f.store.GetRestorePlan(ctx, planID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return Facts{}, nil
		}
		return Facts{}, err
	}
	item, itemErr := f.readItem(ctx, plan.ItemID)
	if itemErr != nil {
		return Facts{}, itemErr
	}

	planSize := plan.ExpectedSize
	facts := Facts{
		Operation:             OpRestore,
		PlanID:                plan.ID,
		PlanState:             string(plan.State),
		PlanRevoked:           plan.State == domain.RestoreRolledBack,
		ApprovedAt:            plan.ApprovedAt,
		ExpectedDigest:        plan.ApprovalDigest,
		ExpectedObjectID:      plan.ItemID,
		ExpectedSHA256:        plan.ExpectedSHA256,
		ExpectedSize:          &planSize,
		ExpectedPath:          plan.QuarantinePath,
		ExpectedRestorePath:   plan.RestorePath,
		RequiredCorroboration: minRestoreCorroboration, // the approved plan record itself
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

	// The durable authorization record set for restore is the approved plan
	// itself: one record, present when the plan carries a real approval time
	// and digest.
	corroboration := 0
	if plan.ApprovedAt != nil && plan.ApprovalDigest != "" {
		corroboration = 1
	}
	facts.ApprovalCorroboration = corroboration

	if err := f.recoveryEvidence(ctx, &facts); err != nil {
		return Facts{}, err
	}
	return facts, nil
}
