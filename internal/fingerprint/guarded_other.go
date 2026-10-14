//go:build !darwin && !linux

package fingerprint

import (
	"context"
	"errors"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

// Guarded fails closed on platforms without descriptor-based boundary support.
func Guarded(string, domain.FileInstance, bool) (string, error) {
	return "", errors.New("recovered fingerprint boundary support unavailable")
}

// GuardedContext fails closed on platforms without descriptor-based boundary
// support. It preserves cancellation without attempting to open a path.
func GuardedContext(ctx context.Context, _ string, _ domain.FileInstance) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return "", errors.New("recovered fingerprint boundary support unavailable")
}
