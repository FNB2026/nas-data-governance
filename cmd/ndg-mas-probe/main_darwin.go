//go:build darwin && masprobe

// Standalone read-only capability probe. No executor, Wails production API or
// release code is linked. Only explicitly selected disposable sources are read.
package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework UniformTypeIdentifiers
#include <stdlib.h>
#include "bridge_darwin.h"
*/
import "C"

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	core "github.com/FNB2026/nas-data-governance/internal/app"
	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/store"
	wails "github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

type grant struct {
	Path     string `json:"path"`
	Bookmark string `json:"bookmark"`
	Network  bool   `json:"network"`
	Stale    bool   `json:"stale"`
	Error    string `json:"error"`
}
type View struct {
	GoConfigInContainer   bool   `json:"go_config_in_container"`
	FoundationInContainer bool   `json:"foundation_in_container"`
	State                 string `json:"state"`
	Locked                bool   `json:"locked"`
	Error                 string `json:"error"`
	Files                 int    `json:"files"`
	Quick                 int    `json:"quick"`
	Full                  int    `json:"full"`
	Groups                int    `json:"groups"`
	Missing               int64  `json:"missing"`
	Unavailable           int64  `json:"unavailable"`
	DBReopened            bool   `json:"db_reopened"`
	Stale                 bool   `json:"stale"`
	Network               bool   `json:"network"`
}
type Probe struct {
	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
	stopping atomic.Bool
	dir      string
	current  grant
	view     View
}

func native(p *C.char) string { defer C.free(unsafe.Pointer(p)); return C.GoString(p) }
func resolveBookmark(bookmark string) (grant, error) {
	raw := C.CString(bookmark)
	defer C.free(unsafe.Pointer(raw))
	var g grant
	err := json.Unmarshal([]byte(native(C.ndg_resolve(raw))), &g)
	return g, err
}

func (p *Probe) save() error {
	b, err := json.Marshal(p.current)
	if err != nil {
		return err
	}
	tmp := filepath.Join(p.dir, "grant.tmp")
	if err = os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(p.dir, "grant.json"))
}
func (p *Probe) block(kind string) View {
	p.view = View{State: "PAUSED_AUTHORIZATION", Locked: true, Error: kind}
	return p.view
}
func (p *Probe) Select() View {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopping.Load() {
		return p.block("shutting_down")
	}
	defer C.ndg_stop()
	// Test-only initial panel location; never grants authority without Open.
	initial := C.CString(os.Getenv("NDG_PROBE_PICKER_START"))
	defer C.free(unsafe.Pointer(initial))
	var g grant
	if json.Unmarshal([]byte(native(C.ndg_pick(initial))), &g) != nil {
		return p.block("native_result_invalid")
	}
	if g.Error != "" {
		return p.block(g.Error)
	}
	canonical, canonicalErr := filepath.EvalSymlinks(g.Path)
	if canonicalErr != nil {
		C.ndg_stop()
		return p.block("source_unavailable")
	}
	g.Path = canonical
	// Each new selection owns an independent DB; prior projects remain intact.
	p.current = g
	if err := p.save(); err != nil {
		C.ndg_stop()
		return p.block("bookmark_persist_failed")
	}
	p.view = View{State: "AUTHORIZED", Network: g.Network}
	return p.view
}
func (p *Probe) Scan() View {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopping.Load() {
		return p.block("shutting_down")
	}
	if p.current.Bookmark == "" {
		return p.block("authorization_required")
	}
	g, resolveErr := resolveBookmark(p.current.Bookmark)
	if resolveErr != nil {
		return p.block("native_result_invalid")
	}
	if g.Error != "" {
		return p.block(g.Error)
	}
	canonical, canonicalErr := filepath.EvalSymlinks(g.Path)
	if canonicalErr != nil {
		C.ndg_stop()
		return p.block("source_unavailable")
	}
	g.Path = canonical
	defer C.ndg_stop()
	// Refuse a resolved root relocation; a fresh picker grant is required.
	savedRoot, rootErr := filepath.EvalSymlinks(p.current.Path)
	if rootErr != nil || g.Path != savedRoot {
		return p.block("root_changed_reselect")
	}
	p.current = g
	if p.save() != nil {
		return p.block("bookmark_persist_failed")
	}
	ctx := p.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	// Root paths exist only in private container metadata, never GUI/log errors.
	id := storageID(g.Path)
	st, err := store.Open(ctx, filepath.Join(p.dir, id+".db"))
	if err != nil {
		return p.block("database_open_failed")
	}
	if err = st.RegisterStorage(ctx, domain.Storage{ID: id, RootPath: g.Path, Kind: "local", CreatedAt: time.Now()}); err != nil {
		_ = st.Close()
		return p.block("database_register_failed")
	}
	result, scanErr := core.NewScanService(st).Scan(ctx, core.ScanInput{Root: g.Path, StorageID: id, Resume: true, NetworkSource: g.Network, Workers: 1, HashAttempts: 1})
	if scanErr != nil {
		_ = st.Close()
		if errors.Is(scanErr, core.ErrNetworkSourceUnavailable) {
			return p.block("network_source_unavailable")
		}
		return p.block("scan_failed")
	}
	files, err := st.ListFiles(ctx, id)
	if err != nil {
		_ = st.Close()
		return p.block("database_read_failed")
	}
	v := View{State: "COMPLETED", Files: len(files), Network: g.Network, Stale: g.Stale, Missing: result.Missing, Unavailable: result.Unavailable}
	hashes := map[string]int{}
	for _, f := range files {
		if f.QuickHash != "" {
			v.Quick++
		}
		if f.ContentSHA256 != "" {
			v.Full++
			hashes[f.ContentSHA256]++
		}
	}
	for _, n := range hashes {
		if n > 1 {
			v.Groups++
		}
	}
	if !result.FullTraversal || len(result.HashFailures) > 0 {
		_ = st.Close()
		return p.block("partial_coverage")
	}
	if st.Close() != nil {
		return p.block("database_close_failed")
	}
	reopened, err := store.Open(ctx, filepath.Join(p.dir, id+".db"))
	if err != nil {
		return p.block("database_reopen_failed")
	}
	again, err := reopened.ListFiles(ctx, id)
	closeErr := reopened.Close()
	if err != nil || closeErr != nil || len(again) != v.Files {
		return p.block("database_reopen_mismatch")
	}
	goConfig, _ := os.UserConfigDir()
	v.GoConfigInContainer = strings.Contains(goConfig, "/Containers/com.fnb.ndg.mas.phase0/Data/")
	v.FoundationInContainer = true
	v.DBReopened = true
	p.view = v
	b, _ := json.Marshal(v)
	_ = os.WriteFile(filepath.Join(p.dir, "result.json"), b, 0600)
	return v
}

// BoundaryProbe tests a synthetic sibling canary, not source data or writes.
func (p *Probe) BoundaryProbe() View {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current.Path == "" {
		return p.block("authorization_required")
	}
	f, err := os.Open(filepath.Join(filepath.Dir(p.current.Path), "unselected-test.bin"))
	if f != nil {
		_ = f.Close()
	}
	if errors.Is(err, os.ErrPermission) {
		p.view = View{State: "BOUNDARY_READ_DENIED", Locked: true}
	} else {
		p.view = View{State: "BOUNDARY_INCONCLUSIVE", Locked: true, Error: "unselected_read_not_denied"}
	}
	return p.view
}

func (p *Probe) Status() View { p.mu.Lock(); defer p.mu.Unlock(); return p.view }

// Shutdown cancels before waiting for the operation mutex. The scope remains
// held until scanner workers and SQLite accesses have returned.
func (p *Probe) shutdown() {
	p.stopping.Store(true)
	if p.cancel != nil {
		p.cancel()
	}
	C.ndg_cancel_picker()
	p.mu.Lock()
	defer p.mu.Unlock()
	C.ndg_stop()
}
func main() {
	dir := native(C.ndg_support())
	// Abort if Foundation failed to choose the sandbox container.
	if !strings.Contains(dir, "/Containers/com.fnb.ndg.mas.phase0/Data/") {
		panic("sandbox container unavailable")
	}
	if os.MkdirAll(dir, 0700) != nil {
		panic("private container unavailable")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := &Probe{ctx: ctx, cancel: cancel, dir: dir, view: View{State: "NO_GRANT", Locked: true}}
	if b, e := os.ReadFile(filepath.Join(dir, "grant.json")); e == nil {
		if json.Unmarshal(b, &p.current) == nil {
			p.view.State = "BOOKMARK_SAVED"
		}
	}
	if err := wails.Run(&options.App{Title: "NDG MAS Phase 0 — disposable read-only", Width: 900, Height: 600, AssetServer: &assetserver.Options{Assets: assets}, Bind: []any{p}, OnBeforeClose: func(context.Context) bool { p.shutdown(); return false }, OnShutdown: func(context.Context) { cancel() }}); err != nil {
		panic("prototype runtime failed")
	}
}
