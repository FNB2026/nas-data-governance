package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/authz"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// AuthorizationGate is the app-side assembly point of the business
// authorization contract. It closes residual risk A7: before this gate,
// each internal caller assembled its own verifier/facts pair, so a future
// web/API/frontend call site could plausibly construct Facts from untrusted
// input or skip the Worker's authorization contract entirely.
//
// Lifecycle invariants:
//
//   - Single owner. The gate is constructed exactly once per store binding
//     via NewAuthorizationGate and holds the ONLY authorization FactSource:
//     authz.NewStoreFacts on the same *store.SQLiteStore (concrete type, no
//     injection). No method of this gate accepts caller-built Facts, and no
//     caller can reach the facts source. Web/API/frontend layers can state
//     what they want (plan id + presented digest) but can never state what
//     is true.
//   - Dangerous operations default-deny. The gate exposes only typed entry
//     points (AuthorizeRestore / AuthorizePurge); every other operation has
//     no entry point at all. A request whose plan kind does not match the
//     operation is structurally refused (each fact source consults only its
//     own plan tables), as is a plan that does not exist.
//   - Read-only authorization state. Decisions are derived exclusively from
//     read-only fact reads; the gate never writes, approves, or mutates any
//     plan or item. The single write it performs is its own audit record.
//   - Audit before action. Every decision — eligible or refused, including
//     probes with unknown plan ids — is appended to authorization_audit.
//     The caller contract is fail-closed: proceed only when
//     err == nil && el.Eligible. An audit-write failure therefore surfaces
//     as a non-nil error even for an eligible verdict, so an unaudited
//     approval can never be acted on.
//   - Trusted clock. The gate uses the verifier's real clock; there is no
//     constructor that accepts an injectable time source.
//
// Chained-contract shape (verify twice, under different trust):
//
//	AuthorizeRestore(ctx, ...)                          // pre-lock: plain lock probe governs
//	scope, _ := authz.AcquireExecutionLockScope(store)  // real flock reservation
//	AuthorizeRestoreWithScope(ctx, ..., scope)          // re-verification: ownership proven, not claimed
//
// Purge requests additionally carry the caller's live device/inode
// observation of the object, mirroring the Worker envelope contract.
//
// The with-scope phase is the A6-corrected re-verification: ownership is
// carried by the same release handle the lock was acquired with, and a scope
// bound to another database is never ownership evidence for this store.
type AuthorizationGate struct {
	store    *store.SQLiteStore
	verifier *authz.Verifier
}

// NewAuthorizationGate assembles the single authorization gate for one
// store. The concrete *store.SQLiteStore parameter is deliberate: the gate
// must be bound to the durable truth, not to an interface a caller could
// satisfy with something else.
func NewAuthorizationGate(st *store.SQLiteStore) *AuthorizationGate {
	return &AuthorizationGate{
		store:    st,
		verifier: authz.NewVerifier(),
	}
}

// AuthorizationRefusal is the typed error the gate returns when a request is
// durably refused. Reason and Detail mirror the verifier's stable codes so
// callers (CLI, desktop UI, future API) can surface the same classification
// the Worker protocol uses over the wire.
type AuthorizationRefusal struct {
	Operation string
	Reason    string
	Detail    string
}

func (e *AuthorizationRefusal) Error() string {
	return fmt.Sprintf("app: %s authorization refused: %s", e.Operation, e.Reason)
}

// durableClaims loads the identity claims of one durable plan. The claims
// are read by the gate itself — the caller names the plan, nothing else.
// An unknown plan yields no claims; the verifier then refuses on empty
// facts (its plan-state step fires before any identity step), which keeps
// the typed wrong_plan classification for forged probes.
type durableClaims struct {
	itemID      string
	contentSHA  string
	size        int64
	hasIdentity bool
}

func (g *AuthorizationGate) claimsFor(ctx context.Context, op authz.OpKind, planID string) (durableClaims, error) {
	switch op {
	case authz.OpRestore:
		plan, err := g.store.GetRestorePlan(ctx, planID)
		if errors.Is(err, store.ErrNotFound) {
			return durableClaims{}, nil
		}
		if err != nil {
			return durableClaims{}, err
		}
		return durableClaims{itemID: plan.ItemID, contentSHA: plan.ExpectedSHA256, size: plan.ExpectedSize, hasIdentity: true}, nil
	case authz.OpPurge:
		plan, err := g.store.GetPurgePlan(ctx, planID)
		if errors.Is(err, store.ErrNotFound) {
			return durableClaims{}, nil
		}
		if err != nil {
			return durableClaims{}, err
		}
		return durableClaims{itemID: plan.ItemID, contentSHA: plan.ExpectedSHA256, size: plan.ExpectedSize, hasIdentity: true}, nil
	default:
		// No other operation has a durable approval contract; the gate has
		// no entry point for them, so this branch is unreachable today and
		// default-deny if it ever is.
		return durableClaims{}, nil
	}
}

// decision is the shared body of every entry point: load durable claims,
// verify against the gate's own fact source, audit, and translate refusals
// into the typed error.
func (g *AuthorizationGate) decision(ctx context.Context, op authz.OpKind, planID, presentedDigest string, scope *authz.ExecutionLockScope, live *liveIdentity) (authz.Eligibility, error) {
	facts := authz.NewStoreFacts(g.store)
	if scope != nil {
		facts = authz.NewStoreFactsWithScope(g.store, scope)
	}
	claims, err := g.claimsFor(ctx, op, planID)
	if err != nil {
		// The plan record itself could not be read: the decision cannot be
		// proven, which is a refusal. Audit the failed attempt and fail.
		el := authz.Eligibility{Reason: authz.ReasonWrongPlan, Detail: "durable plan unreadable"}
		_ = g.audit(ctx, op, planID, "", el, g.ownedBy(scope), err)
		return el, fmt.Errorf("app: %s authorization could not read the durable plan: %w", op, err)
	}
	req := authz.Request{
		Operation:       op,
		PlanID:          planID,
		PresentedDigest: presentedDigest,
		ObjectID:        claims.itemID,
		ContentSHA256:   claims.contentSHA,
	}
	if claims.hasIdentity {
		req.Size = authz.Int64Field(claims.size)
	}
	if live != nil {
		req.Device = authz.Uint64Field(live.device)
		req.Inode = authz.Uint64Field(live.inode)
	}
	el, verr := g.verifier.Check(ctx, facts, req)
	if verr != nil {
		// A fact-read or verification error is a refusal (fail-closed) and
		// is audited as such.
		failed := authz.Eligibility{Reason: authz.ReasonWrongPlan, Detail: "authorization facts unavailable"}
		_ = g.audit(ctx, op, planID, claims.itemID, failed, g.ownedBy(scope), verr)
		return el, fmt.Errorf("app: %s authorization failed: %w", op, verr)
	}
	if auditErr := g.audit(ctx, op, planID, claims.itemID, el, g.ownedBy(scope), nil); auditErr != nil {
		// Audit before action: an unaudited approval must not be
		// actionable, so the audit failure fails the call even when the
		// verdict was eligible. The refusal verdict itself is unchanged.
		return el, fmt.Errorf("app: authorization audit failed: %w", auditErr)
	}
	if !el.Eligible {
		return el, &AuthorizationRefusal{
			Operation: string(op),
			Reason:    string(el.Reason),
			Detail:    el.Detail,
		}
	}
	return el, nil
}

// ownedBy reports that scope is truthful ownership of THIS gate's store.
// A scope owned by another database is real ownership — of that database —
// and must never be recorded as ownership here.
func (g *AuthorizationGate) ownedBy(scope *authz.ExecutionLockScope) bool {
	return scope.Covers(g.store)
}

// audit persists one decision record. Content discipline (AGENTS rule 6):
// opaque identifiers and stable reason codes only — no file paths, no
// content hashes, no sizes.
func (g *AuthorizationGate) audit(ctx context.Context, op authz.OpKind, planID, itemID string, el authz.Eligibility, ownership bool, cause error) error {
	reason := string(el.Reason)
	detail := el.Detail
	if cause != nil {
		// The verdict could not be proven (facts unavailable). Record the
		// refusal cause class without leaking error internals.
		reason = string(authz.ReasonWrongPlan)
		detail = "authorization facts unavailable"
	}
	return g.store.AppendAuthorizationDecision(ctx, store.AuthorizationDecision{
		Operation:         string(op),
		PlanID:            planID,
		ItemID:            itemID,
		Eligible:          el.Eligible && cause == nil,
		Reason:            reason,
		Detail:            detail,
		OwnershipVerified: ownership,
		DecidedAt:         time.Now().UTC(),
	})
}

// AuthorizeRestore verifies business authorization for executing the restore
// plan planID with the presented digest. This is the pre-lock phase: the
// plain execution-lock probe governs the recovery evidence.
func (g *AuthorizationGate) AuthorizeRestore(ctx context.Context, planID, presentedDigest string) (authz.Eligibility, error) {
	return g.decision(ctx, authz.OpRestore, planID, presentedDigest, nil, nil)
}

// AuthorizeRestoreWithScope is the re-verification phase, run while the
// caller holds the execution lock through scope. A nil scope, a released
// scope, or a scope bound to another database is not ownership; the plain
// lock probe then governs exactly as in the pre-lock phase.
func (g *AuthorizationGate) AuthorizeRestoreWithScope(ctx context.Context, planID, presentedDigest string, scope *authz.ExecutionLockScope) (authz.Eligibility, error) {
	return g.decision(ctx, authz.OpRestore, planID, presentedDigest, scope, nil)
}

// AuthorizePurge verifies business authorization for a purge plan. device
// and inode are the caller's live identity observation of the quarantine
// object (the same dimension a Worker envelope must carry); the executor
// re-proves them against the mount root before any write. The read-only
// authorization path is productized here; the execution side remains
// unreleased (PurgeReleaseAllowed=false) — an eligible purge verdict
// authorizes nothing until the release gate opens.
func (g *AuthorizationGate) AuthorizePurge(ctx context.Context, planID, presentedDigest string, device, inode uint64) (authz.Eligibility, error) {
	return g.decision(ctx, authz.OpPurge, planID, presentedDigest, nil, &liveIdentity{device: device, inode: inode})
}

// AuthorizePurgeWithScope is the re-verification phase for purge, run under
// the caller's execution-lock scope.
func (g *AuthorizationGate) AuthorizePurgeWithScope(ctx context.Context, planID, presentedDigest string, scope *authz.ExecutionLockScope, device, inode uint64) (authz.Eligibility, error) {
	return g.decision(ctx, authz.OpPurge, planID, presentedDigest, scope, &liveIdentity{device: device, inode: inode})
}

// liveIdentity carries the caller's live object observation into the
// request. It is never persisted and never becomes authorization truth on
// its own: durable facts still bind the plan, and the executor re-proves
// the live dimension.
type liveIdentity struct {
	device uint64
	inode  uint64
}
