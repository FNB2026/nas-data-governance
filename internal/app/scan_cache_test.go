package app

import (
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

func TestCanReuseCachedHashRequiresReliablePhysicalIdentity(t *testing.T) {
	mtime := time.Now().UTC().Truncate(time.Second)
	cached := store.FileMeta{
		Path: "/synthetic/file", Size: 10, ModifiedAt: mtime,
		Device: 7, Inode: 9, PhysicalReliable: false, QuickHash: "cached",
	}
	file := domain.FileInstance{
		Path: "/synthetic/file", Size: 10, ModifiedAt: mtime, Device: 7, Inode: 9,
		Physical: domain.PhysicalIdentity{Device: 7, Inode: 9, Reliable: false},
	}
	if canReuseCachedHash(cached, file) {
		t.Fatal("unreliable network identity authorized cached hash reuse")
	}

	cached.PhysicalReliable = true
	file.Physical.Reliable = true
	if !canReuseCachedHash(cached, file) {
		t.Fatal("reliable unchanged physical identity did not reuse cached hash")
	}
}

func TestCanReuseCachedHashRequiresMatchingDevice(t *testing.T) {
	mtime := time.Now().UTC()
	cached := store.FileMeta{
		Size: 10, ModifiedAt: mtime, Device: 7, Inode: 9,
		PhysicalReliable: true, QuickHash: "cached",
	}
	file := domain.FileInstance{
		Size: 10, ModifiedAt: mtime, Device: 8, Inode: 9,
		Physical: domain.PhysicalIdentity{Device: 8, Inode: 9, Reliable: true},
	}
	if canReuseCachedHash(cached, file) {
		t.Fatal("device mismatch authorized cached hash reuse")
	}
}
