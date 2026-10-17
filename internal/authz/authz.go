// Package authz defines the real business-authorization contract between the
// governance plan/approval services and the private Worker. It is the single
// internal component that decides whether a dangerous operation (SEAL / PURGE /
// INSPECT / RESTORE) is eligible to execute, based on durable authorization
// truth rather than transient connection properties.
//
// Design invariants (enforced by Verifier.Check):
//   - A kernel-attested peer identity (SO_PEERCRED) is NEVER sufficient.
//   - A complete set of IPC protocol fields is NEVER sufficient.
//   - A well-formed SHA-256 digest is necessary but NEVER sufficient.
//   - Every identity dimension asserted by the facts is verified. Missing or
//     zero-valued request fields are refused, not skipped; zero-byte objects
//     are first-class (size 0 matches only size 0).
//   - The request plan ID must equal the fact plan ID, and the fact source's
//     operation must equal the requested operation: one plan kind can never
//     authorize a different dangerous operation.
//   - Authorization time comes only from the verifier's trusted execution
//     environment clock; requests cannot supply it.
//   - Approval corroboration counts independent durable AUTHORIZATION RECORDS
//     (approval + dry-run). It is never presented as a count of retainable
//     file copies; copy sufficiency remains a business/executor concern.
//
// The component is read-only and does not dispatch, approve, or write. It
// merely answers "is this request authorized by the durable facts?". Enabling
// real SEAL/PURGE dispatch remains a separate, later, reviewed step.
package authz

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

// OpKind enumerates the dangerous operations whose execution requires a real
// business-authorization contract.
type OpKind string

const (
	OpSeal    OpKind = "SEAL"
	OpPurge   OpKind = "PURGE"
	OpInspect OpKind = "INSPECT"
	OpRestore OpKind = "RESTORE"
)

// stateApproved is the only plan state that confers execution eligibility.
// Both OperationPlan and PurgePlan use the literal "APPROVED" for this state.
const stateApproved = "APPROVED"

// minCorroboration is the floor on independent durable approval records for
// PURGE that no fact source may weaken: the approval record plus the dry-run
// verification record. It is never a count of retainable file copies.
const minCorroboration = 2

// minRestoreCorroboration is RESTORE's floor. Restore has one durable
// authorization record (the approved restore plan): it carries no dry-run
// record, it is the reversible direction, and its safety comes from
// plan↔item↔original-destination binding here plus the executor's live
// destination-absence and snapshot checks.
const minRestoreCorroboration = 1

// Reason is a typed rejection code produced by Verifier.Check. It is stable
// and suitable for audit logs and operator messaging.
type Reason string

const (
	ReasonEligible     Reason = "eligible"
	ReasonExpired      Reason = "expired"
	ReasonRevoked      Reason = "revoked"
	ReasonTampered     Reason = "tampered"
	ReasonWrongObject  Reason = "wrong_object"
	ReasonWrongPlan    Reason = "wrong_plan"
	ReasonRecoveryLock Reason = "recovery_lock"
)

// Request is what a caller (e.g. a Worker envelope) presents for a dangerous
// operation. SourcePeer and IPCFields are included on purpose: the verifier
// must prove they are never sufficient on their own. Check ignores them as a
// gate and only binds the durable Facts.
//
// Identity fields use pointer types where omission must be distinguishable
// from a legitimate zero value: a nil Size is an incomplete request and is
// refused, while a Size of 0 is a legitimate zero-byte object. Authorization
// time is NEVER taken from the request.
type Request struct {
	Operation OpKind
	PlanID    string

	// PresentedDigest is the authorization_sha256 carried by the envelope. A
	// well-formed SHA-256 is necessary but NOT sufficient on its own.
	PresentedDigest string

	// Object identity claims taken from the envelope. Empty strings are
	// refused (an empty object identity is never legitimate).
	ObjectID      string
	ContentSHA256 string
	// Size must be non-nil; 0 is a legitimate zero-byte object.
	Size *int64
	// Device and Inode must be carried by PURGE envelopes (presence is
	// enforced for PURGE). Their truth is bound live against the filesystem
	// by the executor, not by this durable-fact layer.
	Device *uint64
	Inode  *uint64

	// SourcePeer is the kernel-attested SO_PEERCRED identity. Informational
	// only; never a sufficient condition for eligibility.
	SourcePeer string
	// IPCFields are the full IPC protocol fields. Informational only.
	IPCFields map[string]string
}

// Int64Field returns a pointer to v, for building Requests where a legitimate
// zero value must be distinguishable from omission.
func Int64Field(v int64) *int64 { return &v }

// Uint64Field returns a pointer to v, for building Requests.
func Uint64Field(v uint64) *uint64 { return &v }

// Facts is the durable authorization truth for one plan. It is the single
// source of authorization truth: the verifier never trusts caller claims about
// these values and binds them exclusively from Facts.
type Facts struct {
	// Operation is the operation this fact set authorizes. It must equal the
	// request's operation: a purge approval is not a seal approval.
	Operation OpKind

	PlanID      string
	PlanState   string
	PlanRevoked bool
	// ApprovedAt is the durable approval time. A nil ApprovedAt on an
	// approved plan is an inconsistent record and is refused.
	ApprovedAt *time.Time

	ExpectedDigest   string
	DryRunDigest     string
	DryRunVerifiedAt *time.Time

	RetainUntil time.Time

	// Plan-side object expectations: what the approval was granted against.
	// All four must be asserted (ExpectedSize non-nil; 0 is valid) or the
	// fact set cannot authorize anything.
	ExpectedObjectID string
	ExpectedSHA256   string
	ExpectedSize     *int64
	ExpectedPath     string

	// ExpectedRestorePath is the original source location a RESTORE plan is
	// bound to. For RESTORE facts it must equal the item's durable source
	// path; PURGE facts leave it empty. It exists so a restore approval can
	// never silently retarget a different destination.
	ExpectedRestorePath string

	// Item-side durable state: the object the plan points at. The verifier
	// binds plan expectations against this record, so a plan whose snapshot
	// drifted from its item is refused here, not only at execution time.
	ItemStatus      string
	ItemHoldReason  string
	ItemSHA256      string
	ItemSize        int64
	ItemPath        string
	ItemRetainUntil time.Time
	SourcePath      string

	// ApprovalCorroboration is the number of independent durable
	// authorization records corroborating this approval (the approval record
	// plus the dry-run verification record). It is NOT a count of retainable
	// file copies.
	ApprovalCorroboration int
	RequiredCorroboration int

	RecoveryLockHeld bool
}

// Eligibility is the verifier's decision.
type Eligibility struct {
	Eligible bool
	Reason   Reason
	Detail   string
}

// FactSource supplies the durable authorization truth for (op, planID). All
// implementations must be read-only, and every internal failure must be
// returned as an error — a fact source that cannot prove a lock is free has
// not proven anything.
type FactSource interface {
	ReadOnlyFacts(ctx context.Context, op OpKind, planID string) (Facts, error)
}

var sha256Re = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

func wellFormedSHA256(s string) bool { return sha256Re.MatchString(s) }

// Verifier binds plan identity, approval status, approval digest, source and
// object identity, retention period, approval corroboration and operation type
// to a single execution-eligibility decision. Its clock is part of the
// trusted execution environment: requests cannot influence it.
type Verifier struct {
	now func() time.Time
}

// NewVerifier returns a read-only authorization verifier using the local
// trusted clock.
func NewVerifier() *Verifier {
	return &Verifier{now: func() time.Time { return time.Now().UTC() }}
}

// NewVerifierWithClock pins the trusted authorization clock (tests, replay).
func NewVerifierWithClock(now func() time.Time) *Verifier { return &Verifier{now: now} }

// Check evaluates a presented Request against the durable Facts supplied by
// src. It returns a non-eligible Eligibility (with a typed Reason) for every
// failure mode; an error only when the fact source itself fails — an error is
// always a refusal, never a silent pass.
func (v *Verifier) Check(ctx context.Context, src FactSource, req Request) (Eligibility, error) {
	if src == nil {
		return Eligibility{}, errors.New("authz: nil fact source")
	}
	facts, err := src.ReadOnlyFacts(ctx, req.Operation, req.PlanID)
	if err != nil {
		return Eligibility{}, fmt.Errorf("authz: read facts: %w", err)
	}
	now := v.now()

	// 1. Recovery / execution lock: no dangerous op while recovery is pending
	//    or the lock state is unknown.
	if facts.RecoveryLockHeld {
		return ineligible(ReasonRecoveryLock, "recovery or execution lock is active"), nil
	}

	// 2. Plan identity and operation binding: the facts must be the facts of
	//    THIS plan and THIS operation.
	if facts.PlanID == "" || facts.PlanID != req.PlanID {
		return ineligible(ReasonWrongPlan, "fact plan identity does not match the request"), nil
	}
	if facts.Operation != req.Operation {
		return ineligible(ReasonWrongPlan, "facts authorize a different operation"), nil
	}

	// 3. Approval status.
	if facts.PlanState != stateApproved {
		if facts.PlanRevoked {
			return ineligible(ReasonRevoked, "approval was invalidated or rolled back"), nil
		}
		return ineligible(ReasonWrongPlan, "plan is not an approved authorization for this operation"), nil
	}

	// 4. Approval record integrity and timing (durable state, trusted clock).
	if facts.ApprovedAt == nil {
		return ineligible(ReasonTampered, "approved plan lacks a durable approval time"), nil
	}
	if facts.ApprovedAt.After(now) {
		return ineligible(ReasonTampered, "approval time is in the future"), nil
	}

	// 5. Retention period (PURGE only): the protected window must have
	//    elapsed. RESTORE deliberately does not inherit this precondition —
	//    a restore moves an object back to its owner, the opposite direction
	//    of a destructive purge, and its eligibility is governed by the
	//    restore business rules (plan approval + item restorability), not by
	//    the purge retention clock.
	if req.Operation == OpPurge && now.Before(facts.RetainUntil) {
		return ineligible(ReasonExpired, "retention period has not expired"), nil
	}

	// 6. The facts must bind a complete object identity. A plan that cannot
	//    name what it authorizes against authorizes nothing.
	if facts.ExpectedObjectID == "" || facts.ExpectedSHA256 == "" ||
		facts.ExpectedPath == "" || facts.ExpectedSize == nil {
		return ineligible(ReasonWrongObject, "facts do not bind a complete object identity"), nil
	}

	// 7. Item durable state and plan↔item binding, per operation:
	//    - PURGE: the plan's snapshot must still match the object it points
	//      at, and the object must be in the one state a purge may act on.
	//    - RESTORE: the restore plan's snapshot must still match its item,
	//      including the original source destination, and the item must be
	//      in a state a restore may act on (the same set the restore
	//      executor accepts: active, held, or purge eligible).
	switch req.Operation {
	case OpPurge:
		if facts.ItemStatus != string(domain.QuarantinePurgeEligible) || facts.ItemHoldReason != "" {
			return ineligible(ReasonWrongObject, "quarantine item is not purge eligible: "+facts.ItemStatus), nil
		}
		if facts.SourcePath == "" {
			return ineligible(ReasonWrongObject, "quarantine item lacks a source identity"), nil
		}
		if !strings.EqualFold(facts.ItemSHA256, facts.ExpectedSHA256) ||
			facts.ItemSize != *facts.ExpectedSize ||
			facts.ItemPath != facts.ExpectedPath ||
			!facts.ItemRetainUntil.Equal(facts.RetainUntil) {
			return ineligible(ReasonWrongObject, "plan snapshot no longer matches its quarantine item"), nil
		}
	case OpRestore:
		switch domain.QuarantineStatus(facts.ItemStatus) {
		case domain.QuarantineActive, domain.QuarantineHold, domain.QuarantinePurgeEligible:
		default:
			return ineligible(ReasonWrongObject, "quarantine item is not restorable: "+facts.ItemStatus), nil
		}
		if facts.ExpectedRestorePath == "" {
			return ineligible(ReasonWrongObject, "restore facts lack the original destination binding"), nil
		}
		if !strings.EqualFold(facts.ItemSHA256, facts.ExpectedSHA256) ||
			facts.ItemSize != *facts.ExpectedSize ||
			facts.ItemPath != facts.ExpectedPath ||
			facts.SourcePath != facts.ExpectedRestorePath {
			return ineligible(ReasonWrongObject, "restore plan snapshot no longer matches its quarantine item"), nil
		}
	}

	// 8. Request completeness: every dimension the protocol requires must be
	//    present. Omission is refused, never skipped; zero is only legal
	//    where zero is a legitimate value.
	if !wellFormedSHA256(req.PresentedDigest) {
		return ineligible(ReasonTampered, "presented approval digest is malformed"), nil
	}
	if !strings.EqualFold(req.PresentedDigest, facts.ExpectedDigest) {
		return ineligible(ReasonTampered, "presented approval digest does not match durable truth"), nil
	}
	if req.ObjectID == "" || req.ContentSHA256 == "" || req.Size == nil {
		return ineligible(ReasonWrongObject, "request is missing required object identity fields"), nil
	}
	if req.Operation == OpPurge && (req.Device == nil || req.Inode == nil) {
		return ineligible(ReasonWrongObject, "purge request is missing device or inode"), nil
	}
	// Explicit input-domain constraints, not equality substitutes: an object
	// size is a non-negative identity dimension.
	if *req.Size < 0 {
		return ineligible(ReasonWrongObject, "object size must be a non-negative identity dimension"), nil
	}

	// 9. Request↔facts identity binding, including zero-byte objects.
	if req.ObjectID != facts.ExpectedObjectID {
		return ineligible(ReasonWrongObject, "object id does not match durable truth"), nil
	}
	if !strings.EqualFold(req.ContentSHA256, facts.ExpectedSHA256) {
		return ineligible(ReasonWrongObject, "content digest does not match durable truth"), nil
	}
	if *req.Size != *facts.ExpectedSize {
		return ineligible(ReasonWrongObject, "object size does not match durable truth"), nil
	}

	// 10. Approval corroboration: independent durable records, digest-bound
	//     and time-ordered. A lone digest is not authority. The floor is per
	//     operation: PURGE requires the approval record plus the dry-run
	//     verification record (minCorroboration); RESTORE's durable record
	//     set is the approved plan itself — restore is the reversible
	//     direction, carries no dry-run record, and its identity is instead
	//     bound plan↔item↔original-destination and re-proven live by the
	//     executor (destination absence + snapshot). The fact source cannot
	//     set either floor below its operation's minimum.
	var floor int
	if req.Operation == OpPurge {
		floor = minCorroboration
	} else {
		floor = minRestoreCorroboration
	}
	required := facts.RequiredCorroboration
	if required < floor {
		required = floor
	}
	if facts.ApprovalCorroboration < required {
		return ineligible(ReasonTampered, "approval is not independently corroborated"), nil
	}
	if req.Operation == OpPurge {
		if facts.DryRunDigest == "" || facts.DryRunVerifiedAt == nil {
			return ineligible(ReasonTampered, "dry-run verification record is missing"), nil
		}
		if !wellFormedSHA256(facts.DryRunDigest) || !strings.EqualFold(facts.DryRunDigest, facts.ExpectedDigest) {
			return ineligible(ReasonTampered, "dry-run validated a different digest than the approval"), nil
		}
		if facts.DryRunVerifiedAt.Before(*facts.ApprovedAt) {
			return ineligible(ReasonTampered, "dry-run verification precedes the approval"), nil
		}
		if facts.DryRunVerifiedAt.After(now) {
			return ineligible(ReasonTampered, "dry-run verification time is in the future"), nil
		}
	}

	return Eligibility{Eligible: true, Reason: ReasonEligible, Detail: "all authorization bindings satisfied"}, nil
}

func ineligible(r Reason, detail string) Eligibility {
	return Eligibility{Eligible: false, Reason: r, Detail: detail}
}
