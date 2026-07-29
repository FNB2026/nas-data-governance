// Command ndg-desktop is the Wails v2 desktop application entry point.
// It provides a React + TypeScript frontend backed by the Go core engine.
//
// Per ADR-0006, this is the ONLY cmd/ package that imports Wails. The
// binding layer lives in internal/adapters/wails/ and exposes only
// high-level use cases to the frontend.
package main

import (
	"context"
	"embed"

	wailsapp "github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/FNB2026/nas-data-governance/internal/adapters/wails"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	api := wails.NewAPI()

	err := wailsapp.Run(&options.App{
		Title:     "NDG — 数据治理工作台",
		Width:     1280,
		Height:    800,
		MinWidth:  960,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: options.NewRGBA(255, 255, 255, 255),
		OnStartup: func(ctx context.Context) {
			// Center window on the primary screen to avoid
			// off-screen placement on multi-monitor setups.
			runtime.WindowCenter(ctx)
		},
		OnShutdown: func(ctx context.Context) {
			// Ensure the project database is closed on exit.
			_ = api.CloseProject()
		},
		Mac: &mac.Options{
			TitleBar: mac.TitleBarHiddenInset(),
		},
		Bind: []any{api},
	})
	if err != nil {
		panic(err)
	}
}
