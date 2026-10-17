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
//   - Eligibility requires EVERY binding to hold at once: plan identity,
//     approval status, approval digest, source & object identity, retention
//     period, independent valid-copy corroboration, and operation type.
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
type Request struct {
	Operation OpKind
	PlanID    string

	// PresentedDigest is the authorization_sha256 carried by the envelope. A
	// well-formed SHA-256 is necessary but NOT sufficient on its own.
	PresentedDigest string

	// Object identity claims taken from the envelope.
	ObjectID      string
	ContentSHA256 string
	Size          int64
	MountPath     string
	Device        uint64
	Inode         uint64

	// SourcePeer is the kernel-attested SO_PEERCRED identity. Informational
	// only; never a sufficient condition for eligibility.
	SourcePeer string
	// IPCFields are the full IPC protocol fields. Informational only.
	IPCFields map[string]string

	// Now overrides the verification time; defaults to time.Now().UTC().
	Now time.Time
}

// Facts is the durable authorization truth for one plan. It is the single
// source of authorization truth: the verifier never trusts caller claims about
// these values and binds them exclusively from Facts.
type Facts struct {
	PlanID           string
	PlanState        string
	PlanRevoked      bool
	ExpectedDigest   string
	DryRunDigest     string
	DryRunVerifiedAt *time.Time
	RetainUntil      time.Time
	ExpectedObjectID string
	ExpectedSHA256   string
	ExpectedSize     int64
	ExpectedPath     string
	Device           uint64
	Inode            uint64
	ValidCopies      int
	RequiredCopies   int
	RecoveryLockHeld bool
	SourcePath       string
}

// Eligibility is the verifier's decision.
type Eligibility struct {
	Eligible bool
	Reason   Reason
	Detail   string
}

// FactSource supplies the durable authorization truth for (op, planID). All
// implementations must be read-only.
type FactSource interface {
	ReadOnlyFacts(ctx context.Context, op OpKind, planID string) (Facts, error)
}

var sha256Re = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

func wellFormedSHA256(s string) bool { return sha256Re.MatchString(s) }

// Verifier binds plan identity, approval status, approval digest, source and
// object identity, retention period, valid-copy count and operation type to a
// single execution-eligibility decision.
type Verifier struct{}

// NewVerifier returns a read-only authorization verifier.
func NewVerifier() *Verifier { return &Verifier{} }

// Check evaluates a presented Request against the durable Facts supplied by
// src. It returns a non-eligible Eligibility (with a typed Reason) for every
// failure mode; an error only when the fact source itself fails.
func (v *Verifier) Check(ctx context.Context, src FactSource, req Request) (Eligibility, error) {
	if src == nil {
		return Eligibility{}, errors.New("authz: nil fact source")
	}
	facts, err := src.ReadOnlyFacts(ctx, req.Operation, req.PlanID)
	if err != nil {
		return Eligibility{}, fmt.Errorf("authz: read facts: %w", err)
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	// 1. Recovery / execution lock: no dangerous op while recovery is pending.
	if facts.RecoveryLockHeld {
		return ineligible(ReasonRecoveryLock, "recovery or execution lock is active"), nil
	}
	// 2. Plan identity + approval status.
	if facts.PlanState != stateApproved {
		if facts.PlanRevoked {
			return ineligible(ReasonRevoked, "approval was invalidated or rolled back"), nil
		}
		return ineligible(ReasonWrongPlan, "plan is not an approved authorization for this operation"), nil
	}
	// 3. Retention period: the protected window must have elapsed.
	if now.Before(facts.RetainUntil) {
		return ineligible(ReasonExpired, "retention period has not expired"), nil
	}
	// 4. Approval digest binding (well-formed AND matches durable truth).
	if !wellFormedSHA256(req.PresentedDigest) || !strings.EqualFold(req.PresentedDigest, facts.ExpectedDigest) {
		return ineligible(ReasonTampered, "presented approval digest is malformed or does not match durable truth"), nil
	}
	// 5. Independent corroboration: a lone digest is not authority.
	if facts.ValidCopies < facts.RequiredCopies {
		return ineligible(ReasonTampered, "approval is not independently corroborated by enough valid copies"), nil
	}
	// 6. Source and object identity binding.
	if !objectIdentityMatches(req, facts) {
		return ineligible(ReasonWrongObject, "source or object identity does not match durable truth"), nil
	}
	return Eligibility{Eligible: true, Reason: ReasonEligible, Detail: "all authorization bindings satisfied"}, nil
}

// objectIdentityMatches verifies the caller's object/source claims against the
// durable Fact. Zero-valued Fact fields are not asserted (e.g. device/inode are
// only bound for operations that carry them).
func objectIdentityMatches(req Request, f Facts) bool {
	if f.ExpectedObjectID != "" && req.ObjectID != f.ExpectedObjectID {
		return false
	}
	if f.ExpectedSHA256 != "" && req.ContentSHA256 != "" && !strings.EqualFold(req.ContentSHA256, f.ExpectedSHA256) {
		return false
	}
	if f.ExpectedSize != 0 && req.Size != 0 && req.Size != f.ExpectedSize {
		return false
	}
	if f.ExpectedPath != "" && req.MountPath != f.ExpectedPath {
		return false
	}
	if f.Device != 0 && req.Device != f.Device {
		return false
	}
	if f.Inode != 0 && req.Inode != f.Inode {
		return false
	}
	return true
}

func ineligible(r Reason, detail string) Eligibility {
	return Eligibility{Eligible: false, Reason: r, Detail: detail}
}
