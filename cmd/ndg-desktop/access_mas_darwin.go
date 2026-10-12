//go:build darwin && mas

package main

import (
	"github.com/FNB2026/nas-data-governance/internal/adapters/wails"
	"github.com/FNB2026/nas-data-governance/internal/fileaccess"
	"github.com/FNB2026/nas-data-governance/internal/macaccess"
	"path/filepath"
)

const sandboxDesktop = true

func newDesktopAPI() *wails.API {
	base, e := macaccess.SupportBase()
	if e != nil {
		panic("MAS_CONTAINER_REQUIRED")
	}
	access, e := fileaccess.New(filepath.Join(base, "capabilities"), macaccess.Native{})
	if e != nil {
		panic("AUTHORIZATION_REQUIRED")
	}
	return wails.NewSandboxAPI(access, base)
}
