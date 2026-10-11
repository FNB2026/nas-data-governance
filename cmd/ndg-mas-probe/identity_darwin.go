//go:build darwin && masprobe

package main

import (
	"crypto/sha256"
	"encoding/hex"
)

func storageID(path string) string {
	h := sha256.Sum256([]byte(path))
	return "probe_" + hex.EncodeToString(h[:8])
}
