//go:build !darwin && !linux

package executor

import "errors"

func probeRestorePath(string, string, string, int64) (bool, bool, error) {
	return false, false, errors.New("restore executor: boundary support unavailable")
}
