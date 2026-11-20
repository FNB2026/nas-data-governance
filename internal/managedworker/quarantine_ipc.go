package managedworker

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/managedfs"
)

type quarantineResponse struct {
	Plan   QuarantinePurgeResult `json:"plan"`
	Reason string                `json:"reason,omitempty"`
}

// ServeQuarantinePurge is the explicit quarantine service for the Unix worker.
// It accepts only a connection admitted by Listener.Accept.
func (c *Conn) ServeQuarantinePurge(ctx context.Context, service *QuarantinePurge) error {
	if c == nil || c.conn == nil || c.peer.UID == 0 || c.peer.GID == 0 || service == nil {
		return ErrUnauthorized
	}
	defer c.Close()
	if err := c.conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return ErrProtocol
	}
	request, err := c.ReadRequest()
	response := quarantineResponse{}
	if err == nil {
		response.Plan, err = service.dispatch(ctx, request)
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
	raw, err := json.Marshal(response)
	if err != nil {
		return ErrProtocol
	}
	if err := c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return ErrProtocol
	}
	return c.WriteResponse(raw)
}
