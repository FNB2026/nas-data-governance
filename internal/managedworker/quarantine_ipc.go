package managedworker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/managedfs"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

type purgeResponse[T any] struct {
	Plan   T      `json:"plan"`
	Reason string `json:"reason,omitempty"`
}

// ServeQuarantinePurge is the explicit quarantine service for the Unix worker.
// It accepts only a connection admitted by Listener.Accept.
func (c *Conn) ServeQuarantinePurge(ctx context.Context, service *QuarantinePurge, source ...*SourcePurge) error {
	if c == nil || c.conn == nil || c.peer.UID == 0 || c.peer.GID == 0 || service == nil {
		return ErrUnauthorized
	}
	defer c.Close()
	if err := c.conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return ErrProtocol
	}
	request, err := c.ReadRequest()
	response := purgeResponse[any]{}
	project := service.project
	operation := "PURGE_INVALID_REQUEST"
	if sourcePurgeOp(request.Op) || quarantinePurgeOp(request.Op) {
		operation = request.Op
	}
	if err == nil {
		if strings.HasPrefix(request.Op, "SOURCE_DIRECT_PURGE_") && len(source) == 1 && source[0] != nil {
			project = source[0].project
			response.Plan, err = source[0].dispatch(ctx, request)
		} else {
			response.Plan, err = service.dispatch(ctx, request)
		}
	}
	switch {
	case err == nil:
	case errors.Is(err, managedfs.ErrRecoveryRequired):
		response.Reason = "RECOVERY_REQUIRED"
	case errors.Is(err, ErrBusy), errors.Is(err, managedfs.ErrBusy):
		response.Reason = "BUSY"
	default:
		response.Reason = "REFUSED"
	}
	if err != nil {
		if auditErr := auditPurgeRefusal(ctx, project, request, operation, response.Reason); auditErr != nil {
			return auditErr
		}
	}
	raw, err := json.Marshal(response)
	if err != nil {
		return ErrProtocol
	}
	if err := c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return ErrProtocol
	}
	return c.WriteResponse(raw)
}

// Discard caller-controlled fields. Only a plan ID already present in this
// project may be used to correlate a refusal with a durable plan.
func auditPurgeRefusal(ctx context.Context, project *store.SQLiteStore, request Request, operation, reason string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	audit := store.AuthorizationDecision{Operation: operation, Reason: reason}
	if id, err := request.OpaqueID("plan_id"); err == nil {
		if plan, err := project.ReadDualPurgePlan(ctx, id); err == nil &&
			strings.HasPrefix(operation, plan.Operation+"_") {
			audit.PlanID = plan.ID
		}
	}
	return project.AppendAuthorizationDecision(ctx, audit)
}

func quarantinePurgeOp(op string) bool {
	switch op {
	case "QUARANTINE_PURGE_PLAN", "QUARANTINE_PURGE_APPROVE", "QUARANTINE_PURGE_DRY_RUN",
		"QUARANTINE_PURGE_EXECUTE", "QUARANTINE_PURGE_CANCEL", "QUARANTINE_PURGE_RECOVER", "QUARANTINE_PURGE_RECEIPT":
		return true
	}
	return false
}
