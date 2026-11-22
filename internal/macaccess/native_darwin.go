//go:build darwin && mas

// Package macaccess bridges OS-selected read-write grants into Go. It has
// no Wails binding and returns no bookmark bytes through frontend DTOs.
package macaccess

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework UniformTypeIdentifiers
#include <stdlib.h>
#include "bridge_darwin.h"
*/
import "C"
import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"github.com/FNB2026/nas-data-governance/internal/fileaccess"
)

type Native struct{}

func decode(raw *C.char, scope unsafe.Pointer) (fileaccess.Grant, func(), error) {
	if raw == nil {
		if scope != nil {
			C.ndg_access_release(scope)
		}
		return fileaccess.Grant{}, nil, fileaccess.ErrAuthorizationRequired
	}
	defer C.free(unsafe.Pointer(raw))
	var r struct {
		fileaccess.Grant
		Error string
	}
	if json.Unmarshal([]byte(C.GoString(raw)), &r) != nil || r.Error != "" {
		if scope != nil {
			C.ndg_access_release(scope)
		}
		if r.Error == "SOURCE_UNAVAILABLE" {
			return fileaccess.Grant{}, nil, fileaccess.ErrSourceUnavailable
		}
		return fileaccess.Grant{}, nil, fileaccess.ErrAuthorizationRequired
	}
	if scope == nil {
		return fileaccess.Grant{}, nil, fileaccess.ErrAuthorizationRequired
	}
	var once sync.Once
	return r.Grant, func() { once.Do(func() { C.ndg_access_release(scope) }) }, nil
}
func (Native) Pick(ctx context.Context) (fileaccess.Grant, func(), error) {
	if ctx.Err() != nil {
		return fileaccess.Grant{}, nil, fileaccess.ErrAuthorizationRequired
	}
	var scope unsafe.Pointer
	raw := C.ndg_access_pick(&scope)
	return decode(raw, scope)
}
func (Native) Resolve(ctx context.Context, saved fileaccess.Grant) (fileaccess.Grant, func(), error) {
	if ctx.Err() != nil {
		return fileaccess.Grant{}, nil, fileaccess.ErrAuthorizationRequired
	}
	raw := C.CString(saved.Bookmark)
	defer C.free(unsafe.Pointer(raw))
	root := C.CString(saved.Root)
	defer C.free(unsafe.Pointer(root))
	var scope unsafe.Pointer
	var network C.int
	if saved.Network {
		network = 1
	}
	response := C.ndg_access_resolve(raw, root, network, &scope)
	return decode(response, scope)
}
func (Native) CancelPicker() { C.ndg_access_cancel_picker() }
func SupportBase() (string, error) {
	raw := C.ndg_access_support()
	if raw == nil {
		return "", fileaccess.ErrAuthorizationRequired
	}
	defer C.free(unsafe.Pointer(raw))
	bundle := C.ndg_access_bundle_id()
	if bundle == nil {
		return "", errors.New("MAS_CONTAINER_REQUIRED")
	}
	defer C.free(unsafe.Pointer(bundle))
	dir := C.GoString(raw)
	if !isSupportContainer(dir, C.GoString(bundle)) {
		return "", errors.New("MAS_CONTAINER_REQUIRED")
	}
	return dir, nil
}

func isSupportContainer(dir, bundle string) bool {
	// Bind to the running bundle identifier instead of naming experimental
	// builds. Release com.fnb.ndg.mas and its QA sub-identifiers all resolve
	// through the same container rule; unrelated bundles are rejected.
	if bundle != "com.fnb.ndg.mas" && !strings.HasPrefix(bundle, "com.fnb.ndg.mas.") {
		return false
	}
	suffix := filepath.Join("Library", "Containers", bundle, "Data", "Library", "Application Support", "NDG")
	return filepath.IsAbs(dir) && filepath.Clean(dir) == dir && strings.HasSuffix(dir, "/"+suffix)
}
