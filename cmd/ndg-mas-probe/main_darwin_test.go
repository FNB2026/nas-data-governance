//go:build darwin && masprobe

package main

import (
	"context"
	"testing"
	"time"
)

func TestInvalidBookmarkFailsClosed(t *testing.T) {
	for _, raw := range []string{"", "not a base64 bookmark", "bm90LWEtYm9va21hcms="} {
		g, err := resolveBookmark(raw)
		if err != nil || g.Error != "authorization_required" || g.Path != "" || g.Bookmark != "" {
			t.Fatal("invalid authority must not grant a source")
		}
	}
}
func TestProjectIdentitySeparated(t *testing.T) {
	if storageID("/synthetic/a") == storageID("/synthetic/b") {
		t.Fatal("roots must not share project identity")
	}
	if storageID("/synthetic/a") != storageID("/synthetic/a") {
		t.Fatal("root identity must survive restart")
	}
}

func TestShutdownCancelsBeforeWaitingAndRejectsNewScan(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := &Probe{ctx: ctx, cancel: cancel}
	p.mu.Lock() // An active scan holds its scoped resource.
	done := make(chan struct{})
	go func() { p.shutdown(); close(done) }()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel scan")
	}
	select {
	case <-done:
		t.Fatal("scope released before operation returned")
	default:
	}
	p.mu.Unlock()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not finish")
	}
	if v := p.Scan(); v.Error != "shutting_down" || !v.Locked {
		t.Fatal("new scan accepted during shutdown")
	}
}

func TestNativeCloseBeforePickerNeverEntersModal(t *testing.T) {
	if got := nativePickAfterClose(); got != `{"error":"shutting_down"}` {
		t.Fatal("late picker must reject close latch")
	}
}
