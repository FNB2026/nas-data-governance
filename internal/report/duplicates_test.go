package report

import (
	"testing"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

func TestDuplicateGroupsOnlyIncludesRepeatedFullHashes(t *testing.T) {
	files := []domain.FileInstance{
		{Path: "a", Size: 3, ContentSHA256: "x"},
		{Path: "b", Size: 3, ContentSHA256: "x"},
		{Path: "c", Size: 4, ContentSHA256: "y"},
		{Path: "d"},
	}
	groups := DuplicateGroups(files)
	if len(groups) != 1 || len(groups[0].Files) != 2 {
		t.Fatalf("unexpected groups: %#v", groups)
	}
}

func TestDuplicateGroupsCountsPathAndPhysicalCopies(t *testing.T) {
	// Two paths, two distinct physical objects (no hardlinks).
	files := []domain.FileInstance{
		{StorageID: "s1", Path: "a", Size: 100, ContentSHA256: "hash1",
			Physical: domain.PhysicalIdentity{Device: 1, Inode: 10, Reliable: true}},
		{StorageID: "s1", Path: "b", Size: 100, ContentSHA256: "hash1",
			Physical: domain.PhysicalIdentity{Device: 1, Inode: 20, Reliable: true}},
	}
	groups := DuplicateGroups(files)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	g := groups[0]
	if g.PathCount != 2 {
		t.Errorf("PathCount: expected 2, got %d", g.PathCount)
	}
	if g.PhysicalCopyCount != 2 {
		t.Errorf("PhysicalCopyCount: expected 2, got %d", g.PhysicalCopyCount)
	}
	if g.HardlinkAliasCount != 0 {
		t.Errorf("HardlinkAliasCount: expected 0, got %d", g.HardlinkAliasCount)
	}
	// 2 physical copies - 1 retained = 1 reclaimable * 100 bytes
	if g.PhysicalReclaimableBytes != 100 {
		t.Errorf("PhysicalReclaimableBytes: expected 100, got %d", g.PhysicalReclaimableBytes)
	}
}

func TestDuplicateGroupsHardlinksCountAsOnePhysicalCopy(t *testing.T) {
	// Three paths, but all share the same Device+Inode = 1 physical object.
	// Reclaimable should be 0 because deleting 2 of 3 hardlinks frees nothing.
	files := []domain.FileInstance{
		{StorageID: "s1", Path: "a", Size: 500, ContentSHA256: "hash2",
			Physical: domain.PhysicalIdentity{Device: 2, Inode: 99, Reliable: true, LinkCount: 3}},
		{StorageID: "s1", Path: "b", Size: 500, ContentSHA256: "hash2",
			Physical: domain.PhysicalIdentity{Device: 2, Inode: 99, Reliable: true, LinkCount: 3}},
		{StorageID: "s1", Path: "c", Size: 500, ContentSHA256: "hash2",
			Physical: domain.PhysicalIdentity{Device: 2, Inode: 99, Reliable: true, LinkCount: 3}},
	}
	groups := DuplicateGroups(files)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	g := groups[0]
	if g.PathCount != 3 {
		t.Errorf("PathCount: expected 3, got %d", g.PathCount)
	}
	if g.PhysicalCopyCount != 1 {
		t.Errorf("PhysicalCopyCount: expected 1 (all hardlinks), got %d", g.PhysicalCopyCount)
	}
	if g.HardlinkAliasCount != 2 {
		t.Errorf("HardlinkAliasCount: expected 2, got %d", g.HardlinkAliasCount)
	}
	// 1 physical copy - 1 retained = 0 reclaimable
	if g.PhysicalReclaimableBytes != 0 {
		t.Errorf("PhysicalReclaimableBytes: expected 0 (hardlinks), got %d", g.PhysicalReclaimableBytes)
	}
}

func TestDuplicateGroupsMixedHardlinksAndRealCopies(t *testing.T) {
	// 4 paths: 2 hardlinks (same inode) + 2 independent copies.
	// Physical objects: 3 (one hardlink pair + two independents).
	// Reclaimable: (3-1) * 200 = 400
	files := []domain.FileInstance{
		{StorageID: "s1", Path: "a", Size: 200, ContentSHA256: "hash3",
			Physical: domain.PhysicalIdentity{Device: 1, Inode: 50, Reliable: true}},
		{StorageID: "s1", Path: "b", Size: 200, ContentSHA256: "hash3",
			Physical: domain.PhysicalIdentity{Device: 1, Inode: 50, Reliable: true}},
		{StorageID: "s1", Path: "c", Size: 200, ContentSHA256: "hash3",
			Physical: domain.PhysicalIdentity{Device: 1, Inode: 60, Reliable: true}},
		{StorageID: "s1", Path: "d", Size: 200, ContentSHA256: "hash3",
			Physical: domain.PhysicalIdentity{Device: 1, Inode: 70, Reliable: true}},
	}
	groups := DuplicateGroups(files)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	g := groups[0]
	if g.PathCount != 4 {
		t.Errorf("PathCount: expected 4, got %d", g.PathCount)
	}
	if g.PhysicalCopyCount != 3 {
		t.Errorf("PhysicalCopyCount: expected 3, got %d", g.PhysicalCopyCount)
	}
	if g.HardlinkAliasCount != 1 {
		t.Errorf("HardlinkAliasCount: expected 1, got %d", g.HardlinkAliasCount)
	}
	if g.PhysicalReclaimableBytes != 400 {
		t.Errorf("PhysicalReclaimableBytes: expected 400, got %d", g.PhysicalReclaimableBytes)
	}
}

func TestDuplicateGroupsUnreliableIdentityIsConservative(t *testing.T) {
	// No PhysicalIdentity set (zero value) = unreliable.
	// Should treat each path as a separate physical copy.
	files := []domain.FileInstance{
		{StorageID: "s1", Path: "a", Size: 1000, ContentSHA256: "hash4"},
		{StorageID: "s1", Path: "b", Size: 1000, ContentSHA256: "hash4"},
		{StorageID: "s1", Path: "c", Size: 1000, ContentSHA256: "hash4"},
	}
	groups := DuplicateGroups(files)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	g := groups[0]
	if g.PhysicalCopyCount != 3 {
		t.Errorf("PhysicalCopyCount: expected 3 (conservative), got %d", g.PhysicalCopyCount)
	}
	if g.HardlinkAliasCount != 0 {
		t.Errorf("HardlinkAliasCount: expected 0 (conservative), got %d", g.HardlinkAliasCount)
	}
	if g.PhysicalReclaimableBytes != 2000 {
		t.Errorf("PhysicalReclaimableBytes: expected 2000, got %d", g.PhysicalReclaimableBytes)
	}
}

func TestDuplicateGroupsMixedReliableAndUnreliableIdentityIsConservative(t *testing.T) {
	// Two hardlink aliases plus two members whose identity is unknown.
	// Unknown identities must each count as an independent physical copy.
	files := []domain.FileInstance{
		{StorageID: "s1", Path: "a", Size: 200, ContentSHA256: "mixed",
			Physical: domain.PhysicalIdentity{Device: 1, Inode: 50, Reliable: true}},
		{StorageID: "s1", Path: "b", Size: 200, ContentSHA256: "mixed",
			Physical: domain.PhysicalIdentity{Device: 1, Inode: 50, Reliable: true}},
		{StorageID: "s1", Path: "c", Size: 200, ContentSHA256: "mixed"},
		{StorageID: "s1", Path: "d", Size: 200, ContentSHA256: "mixed"},
	}

	groups := DuplicateGroups(files)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	g := groups[0]
	if g.PhysicalCopyCount != 3 {
		t.Errorf("PhysicalCopyCount: expected 3, got %d", g.PhysicalCopyCount)
	}
	if g.HardlinkAliasCount != 1 {
		t.Errorf("HardlinkAliasCount: expected 1, got %d", g.HardlinkAliasCount)
	}
	if g.PhysicalReclaimableBytes != 400 {
		t.Errorf("PhysicalReclaimableBytes: expected 400, got %d", g.PhysicalReclaimableBytes)
	}
}

func TestDuplicateGroupsGroupIDIsStableAndNonEmpty(t *testing.T) {
	files := []domain.FileInstance{
		{StorageID: "s1", Path: "a", Size: 10, ContentSHA256: "stable_hash"},
		{StorageID: "s1", Path: "b", Size: 10, ContentSHA256: "stable_hash"},
	}
	groups := DuplicateGroups(files)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].GroupID == "" {
		t.Error("GroupID should not be empty")
	}
	// Same input should produce same GroupID.
	groups2 := DuplicateGroups(files)
	if groups[0].GroupID != groups2[0].GroupID {
		t.Errorf("GroupID not stable: %s vs %s", groups[0].GroupID, groups2[0].GroupID)
	}
	// Different storage should produce different GroupID.
	filesDiffStorage := []domain.FileInstance{
		{StorageID: "s2", Path: "a", Size: 10, ContentSHA256: "stable_hash"},
		{StorageID: "s2", Path: "b", Size: 10, ContentSHA256: "stable_hash"},
	}
	groups3 := DuplicateGroups(filesDiffStorage)
	if groups[0].GroupID == groups3[0].GroupID {
		t.Error("GroupID should differ across storages")
	}
}

func TestDuplicateGroupsDeterministicSortOrder(t *testing.T) {
	// Two groups with different reclaimable bytes.
	// The group with more reclaimable bytes should come first.
	files := []domain.FileInstance{
		{StorageID: "s1", Path: "a", Size: 100, ContentSHA256: "small"},
		{StorageID: "s1", Path: "b", Size: 100, ContentSHA256: "small"},
		{StorageID: "s1", Path: "c", Size: 9999, ContentSHA256: "large"},
		{StorageID: "s1", Path: "d", Size: 9999, ContentSHA256: "large"},
	}
	groups := DuplicateGroups(files)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	// "large" group has more reclaimable bytes, should be first.
	if groups[0].PhysicalReclaimableBytes < groups[1].PhysicalReclaimableBytes {
		t.Errorf("expected DESC sort by reclaimable bytes; got %d before %d",
			groups[0].PhysicalReclaimableBytes, groups[1].PhysicalReclaimableBytes)
	}
}

func TestPhysicalKeyUnreliableReturnsEmpty(t *testing.T) {
	p := domain.PhysicalIdentity{Device: 1, Inode: 2, Reliable: false}
	if key := p.PhysicalKey("s1"); key != "" {
		t.Errorf("unreliable identity should return empty key, got %s", key)
	}
}

func TestPhysicalKeyZeroDeviceOrInodeReturnsEmpty(t *testing.T) {
	p := domain.PhysicalIdentity{Device: 0, Inode: 2, Reliable: true}
	if key := p.PhysicalKey("s1"); key != "" {
		t.Errorf("zero device should return empty key, got %s", key)
	}
	p = domain.PhysicalIdentity{Device: 1, Inode: 0, Reliable: true}
	if key := p.PhysicalKey("s1"); key != "" {
		t.Errorf("zero inode should return empty key, got %s", key)
	}
}

func TestPhysicalKeyReliableReturnsStableKey(t *testing.T) {
	p := domain.PhysicalIdentity{Device: 42, Inode: 7, Reliable: true}
	key := p.PhysicalKey("s1")
	if key == "" {
		t.Fatal("reliable identity should return non-empty key")
	}
	// Same input should produce same key.
	if key2 := p.PhysicalKey("s1"); key != key2 {
		t.Errorf("key not stable: %s vs %s", key, key2)
	}
	// Different storage should produce different key.
	if key3 := p.PhysicalKey("s2"); key == key3 {
		t.Error("key should differ across storages")
	}
}
