package govweb

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/FNB2026/nas-data-governance/internal/managedworker"
)

// QuarantinePurgeClient submits selections and approvals to the private worker.
type QuarantinePurgeClient interface {
	Call(context.Context, string, string, string, string, string) (managedworker.QuarantinePurgeResult, string, error)
}

type purgeFields struct {
	Op           string `json:"op"`
	ItemID       string `json:"item_id,omitempty"`
	PlanID       string `json:"plan_id,omitempty"`
	Digest       string `json:"digest,omitempty"`
	Confirmation string `json:"confirmation,omitempty"`
}

func (s *Server) handleQuarantinePurge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.TLS == nil || s.quarantinePurge == nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"reason": "QUARANTINE_PURGE_UNAVAILABLE"})
		return
	}
	s.guarded(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"reason": "INVALID_REQUEST"})
			return
		}
		// The Worker and the Web gateway use the same strict field policy.
		request, err := managedworker.DecodeRequest(raw)
		if err != nil || !validQuarantineOp(request.Op) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"reason": "INVALID_REQUEST"})
			return
		}
		var fields purgeFields
		d := json.NewDecoder(bytes.NewReader(raw))
		d.DisallowUnknownFields()
		if d.Decode(&fields) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"reason": "INVALID_REQUEST"})
			return
		}
		plan, reason, err := s.quarantinePurge.Call(r.Context(), fields.Op, fields.ItemID, fields.PlanID, fields.Digest, fields.Confirmation)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"reason": "WORKER_RESULT_UNAVAILABLE"})
			return
		}
		if reason != "" {
			writeJSON(w, http.StatusConflict, map[string]any{"plan": plan, "reason": reason})
			return
		}
		writeJSON(w, http.StatusOK, plan)
	})(w, r)
}

func validQuarantineOp(op string) bool {
	switch op {
	case "QUARANTINE_PURGE_PLAN", "QUARANTINE_PURGE_APPROVE", "QUARANTINE_PURGE_DRY_RUN",
		"QUARANTINE_PURGE_EXECUTE", "QUARANTINE_PURGE_RECOVER", "QUARANTINE_PURGE_RECEIPT", "QUARANTINE_PURGE_CANCEL":
		return true
	}
	return false
}

type SourcePurgeClient interface {
	CallSource(context.Context, managedworker.Request) (managedworker.SourcePurgeResult, string, error)
}

func (s *Server) handleSourcePurge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.TLS == nil || s.sourcePurge == nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"reason": "SOURCE_DIRECT_PURGE_UNAVAILABLE"})
		return
	}
	s.guarded(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"reason": "INVALID_REQUEST"})
			return
		}
		request, err := managedworker.DecodeRequest(raw)
		if err != nil || !strings.HasPrefix(request.Op, "SOURCE_DIRECT_PURGE_") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"reason": "INVALID_REQUEST"})
			return
		}
		plan, reason, err := s.sourcePurge.CallSource(r.Context(), request)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"reason": "WORKER_RESULT_UNAVAILABLE"})
			return
		}
		if reason != "" {
			writeJSON(w, http.StatusConflict, map[string]any{"plan": plan, "reason": reason})
			return
		}
		writeJSON(w, http.StatusOK, plan)
	})(w, r)
}
