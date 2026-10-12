//go:build !mas

package main

import "github.com/FNB2026/nas-data-governance/internal/adapters/wails"

const sandboxDesktop = false

func newDesktopAPI() *wails.API { return wails.NewAPI() }
