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
			// Force window onto the primary screen's visible area.
			// WindowCenter can fail on multi-monitor setups with a
			// disconnected external display, so we use an explicit
			// position that is always on the primary screen.
			runtime.WindowSetPosition(ctx, 100, 100)
			// Inject the native directory picker. The adapter layer
			// must not import the Wails runtime (ADR-0006), so the
			// dialog is wired here and exposed to the frontend via
			// API.PickDirectory.
			api.SetDirectoryPicker(func(title string) (string, error) {
				return runtime.OpenDirectoryDialog(ctx, runtime.OpenDialogOptions{
					Title:           title,
					ShowHiddenFiles: true,
				})
			})
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
