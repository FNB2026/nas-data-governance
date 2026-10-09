//go:build !darwin && !linux

package fingerprint

import (
	"errors"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

// Guarded fails closed on platforms without descriptor-based boundary support.
func Guarded(string, domain.FileInstance, bool) (string, error) {
	return "", errors.New("recovered fingerprint boundary support unavailable")
}
