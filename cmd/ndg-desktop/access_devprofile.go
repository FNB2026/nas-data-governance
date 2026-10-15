//go:build ndg_devprofile && !mas

package main

import (
	"os"

	"github.com/FNB2026/nas-data-governance/internal/adapters/wails"
)

const sandboxDesktop = false

// Acceptance-only composition. Ordinary and MAS builds cannot select it.
func newDesktopAPI() *wails.API {
	a, err := wails.NewIsolatedDevAPI(os.Getenv("NDG_DEV_PROFILE"))
	if err != nil {
		panic("NDG_DEV_PROFILE_REQUIRED")
	}
	return a
}
