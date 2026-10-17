package managedworker

// The business-authorization front end for the dangerous operations. It wires
// the R1/R2 authz contract (internal/authz) into the worker's controlled
// internal path, with one deliberate constraint: the verification source is
// the durable SQLiteStore — the project's single authorization truth — never
// IPC fields and never caller-supplied Facts. The concrete store parameter
// type enforces that at compile time.
//
// Admission layering stays intact: kernel peer credentials (SO_PEERCRED) admit
// the connection, the protocol validator admits the frame, and this gate admits
// the business action. Each layer is necessary; none is sufficient alone.
//
// R3 scope: an eligible request forms an internal verification record and
// nothing else. No SEAL, PURGE or RESTORE executes on its strength, and the
// public Web/API surface gains no dangerous endpoint from this file.

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/FNB2026/nas-data-governance/internal/authz"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// AuthorizationGate decides dangerous operations against durable
// authorization truth. It holds no mutable state and is safe for concurrent
// dispatches.
type AuthorizationGate struct {
	verifier *authz.Verifier
	facts    authz.FactSource
}

// NewStoreAuthorizationGate builds the business-authorization gate on the
// durable SQLiteStore. The concrete parameter type is deliberate: a gate that
// could be constructed from anything less durable would reintroduce the
// caller-supplied-facts hole the contract closed.
func NewStoreAuthorizationGate(st *store.SQLiteStore) *AuthorizationGate {
	return &AuthorizationGate{
		verifier: authz.NewVerifier(),
		facts:    authz.NewStoreFacts(st),
	}
}

// AuthorizationDenied is the typed business rejection. It unwraps to
// ErrAuthorization, so callers that only know the coarser pre-R3 contract
// still observe the same sentinel, while audit code can extract the stable
// reason code with errors.As.
type AuthorizationDenied struct {
	Op     string
	PlanID string
	Reason authz.Reason
	Detail string
}

func (e *AuthorizationDenied) Error() string {
	return fmt.Sprintf("managed worker operation %s denied by durable authorization: %s (%s)", e.Op, e.Reason, e.Detail)
}

// Unwrap ties the typed denial to the protocol-level authorization sentinel.
func (e *AuthorizationDenied) Unwrap() error { return ErrAuthorization }

// Verdict is the internal verification result of an eligible request. R3
// deliberately stops here: eligibility forms this record and nothing else —
// no SEAL, no PURGE, no RESTORE is executed on its strength.
type Verdict struct {
	Op       string `json:"op"`
	PlanID   string `json:"plan_id"`
	ObjectID string `json:"object_id"`
	Eligible bool   `json:"eligible"`
	Reason   string `json:"reason"`
	Peer     string `json:"peer"`
}

// DispatchAuthorized is the gate-wired dispatch path. PING and STATUS keep
// their existing behavior. A dangerous op without a gate is refused exactly
// as the ungated Dispatch refuses it — wiring the business gate is the only
// way past that line, and a nil gate can never widen it. A dangerous op with
// a gate is answered either with a typed durable-authorization denial or,
// when eligible, with an internal Verdict that performs no operation.
func DispatchAuthorized(ctx context.Context, request Request, peer Subject, gate *AuthorizationGate) (any, error) {
	if ctx.Err() != nil {
		return nil, ErrProtocol
	}
	switch request.Op {
	case "PING":
		return map[string]any{"ok": true}, nil
	case "STATUS":
		return Status{Ready: true, PurgeReleaseAllowed: false, Executable: len(Executable())}, nil
	case "SEAL", "PURGE", "INSPECT", "RESTORE":
		if gate == nil {
			// Fail-closed, identical to the ungated Dispatch.
			return nil, ErrAuthorization
		}
		return gate.authorize(ctx, request, peer)
	}
	return nil, ErrProtocol
}

func (g *AuthorizationGate) authorize(ctx context.Context, request Request, peer Subject) (any, error) {
	req, err := authzRequestFromEnvelope(request, peer)
	if err != nil {
		return nil, err // protocol-level shape violation
	}
	el, err := g.verifier.Check(ctx, g.facts, req)
	if err != nil {
		// A fact-source failure is always a refusal, never a pass. It is not
		// a typed business reason: the durable truth could not be read.
		return nil, fmt.Errorf("%w: durable authorization facts unavailable: %v", ErrAuthorization, err)
	}
	if !el.Eligible {
		return nil, &AuthorizationDenied{
			Op:     request.Op,
			PlanID: req.PlanID,
			Reason: el.Reason,
			Detail: el.Detail,
		}
	}
	return Verdict{
		Op:       request.Op,
		PlanID:   req.PlanID,
		ObjectID: req.ObjectID,
		Eligible: true,
		Reason:   string(el.Reason),
		Peer:     fmt.Sprintf("uid=%d gid=%d", peer.UID, peer.GID),
	}, nil
}

// authzRequestFromEnvelope maps the protocol frame onto the business request.
// Every identity field is optional at this layer: omission is meaningful and
// is refused by the verifier with a typed reason (an envelope without plan_id
// is a wrong_plan, not a protocol error), while a present field with a
// malformed value is a protocol violation. Authorization time is never taken
// from the request; the verifier's trusted clock supplies it.
func authzRequestFromEnvelope(request Request, peer Subject) (authz.Request, error) {
	req := authz.Request{
		Operation:  authz.OpKind(request.Op),
		SourcePeer: fmt.Sprintf("uid=%d gid=%d", peer.UID, peer.GID),
		IPCFields:  map[string]string{},
	}
	if value, present, err := optionalString(request, "plan_id"); err != nil {
		return authz.Request{}, err
	} else if present {
		req.PlanID = value
		req.IPCFields["plan_id"] = value
	}
	if value, present, err := optionalString(request, "object_id"); err != nil {
		return authz.Request{}, err
	} else if present {
		req.ObjectID = value
		req.IPCFields["object_id"] = value
	}
	if value, present, err := optionalString(request, "sha256"); err != nil {
		return authz.Request{}, err
	} else if present {
		req.ContentSHA256 = value
		req.IPCFields["sha256"] = value
	}
	if value, present, err := optionalString(request, "authorization_sha256"); err != nil {
		return authz.Request{}, err
	} else if present {
		req.PresentedDigest = value
		req.IPCFields["authorization_sha256"] = value
	}
	if value, present, err := optionalInt64(request, "size"); err != nil {
		return authz.Request{}, err
	} else if present {
		// Explicit input-domain constraint, not an equality substitute: an
		// object size is a non-negative identity dimension.
		if value < 0 {
			return authz.Request{}, ErrProtocol
		}
		req.Size = authz.Int64Field(value)
		req.IPCFields["size"] = strconv.FormatInt(value, 10)
	}
	if value, present, err := optionalInt64(request, "device"); err != nil {
		return authz.Request{}, err
	} else if present {
		if value < 0 {
			return authz.Request{}, ErrProtocol
		}
		req.Device = authz.Uint64Field(uint64(value))
		req.IPCFields["device"] = strconv.FormatInt(value, 10)
	}
	if value, present, err := optionalInt64(request, "inode"); err != nil {
		return authz.Request{}, err
	} else if present {
		if value < 0 {
			return authz.Request{}, ErrProtocol
		}
		req.Inode = authz.Uint64Field(uint64(value))
		req.IPCFields["inode"] = strconv.FormatInt(value, 10)
	}
	return req, nil
}

// optionalString reads an optional string field with exactly the validation
// Request.String applies, except that an absent key is a meaningful omission
// rather than an error.
func optionalString(r Request, key string) (string, bool, error) {
	raw, ok := r.Fields[key]
	if !ok {
		return "", false, nil
	}
	var value string
	if json.Unmarshal(raw, &value) != nil || pathLike(value) {
		return "", true, ErrProtocol
	}
	return value, true, nil
}

// optionalInt64 reads an optional integer field with exactly the validation
// Request.Int64 applies, except that an absent key is a meaningful omission.
func optionalInt64(r Request, key string) (int64, bool, error) {
	raw, ok := r.Fields[key]
	if !ok {
		return 0, false, nil
	}
	var value int64
	if json.Unmarshal(raw, &value) != nil || strconv.FormatInt(value, 10) != strings.TrimSpace(string(raw)) {
		return 0, true, ErrProtocol
	}
	return value, true, nil
}
