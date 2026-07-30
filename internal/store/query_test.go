package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/query"
	"github.com/FNB2026/nas-data-governance/internal/report"
)

// seedFileWithPhysical inserts a file instance with explicit Device/Inode
// for hardlink testing. The existing seedFile helper does not set these.
func seedFileWithPhysical(t *testing.T, st *SQLiteStore, storageID, path string, size int64, hash string, device, inode uint64) {
	t.Helper()
	files := []domain.FileInstance{{
		StorageID: storageID, Path: path, Name: filepath.Base(path),
		Size: size, Mode: 0o644, ModifiedAt: time.Now().UTC(),
		Device: device, Inode: inode,
		QuickHash: hash, ContentSHA256: hash, DiscoveredAt: time.Now().UTC(),
		Physical: domain.PhysicalIdentity{
			Device: device, Inode: inode, Reliable: device != 0 && inode != 0,
		},
	}}
	if _, err := st.UpsertFiles(context.Background(), files); err != nil {
		t.Fatalf("upsert file: %v", err)
	}
}

// seedGroupDecision inserts a row into group_decisions for testing.
func seedGroupDecision(t *testing.T, st *SQLiteStore, groupID, decisionType, reason string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := st.db.ExecContext(context.Background(),
		`INSERT INTO group_decisions(id, group_id, decision_type, reason, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?)`,
		groupID+"-"+decisionType, groupID, decisionType, reason, now, now)
	if err != nil {
		t.Fatalf("seed group decision: %v", err)
	}
}

func TestListDuplicateGroupsBasic(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedStorage(t, st, "s1")

	// Two files with the same hash → one duplicate group.
	seedFile(t, st, "s1", "/vol/a.txt", 100, "hashA")
	seedFile(t, st, "s1", "/vol/b.txt", 100, "hashA")
	// A unique file (no duplicate) should not appear.
	seedFile(t, st, "s1", "/vol/c.txt", 200, "hashB")

	page, err := st.ListDuplicateGroups(ctx, query.GroupQuery{PageSize: 20})
	if err != nil {
		t.Fatalf("ListDuplicateGroups: %v", err)
	}
	if len(page.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(page.Groups))
	}
	g := page.Groups[0]
	if g.SHA256 != "hashA" {
		t.Errorf("SHA256: expected hashA, got %s", g.SHA256)
	}
	if g.Size != 100 {
		t.Errorf("Size: expected 100, got %d", g.Size)
	}
	if g.PathCount != 2 {
		t.Errorf("PathCount: expected 2, got %d", g.PathCount)
	}
	if g.PhysicalCopyCount != 2 {
		t.Errorf("PhysicalCopyCount: expected 2, got %d", g.PhysicalCopyCount)
	}
	if g.PhysicalReclaimableBytes != 100 {
		t.Errorf("PhysicalReclaimableBytes: expected 100, got %d", g.PhysicalReclaimableBytes)
	}
	if g.GroupID == "" {
		t.Error("GroupID should not be empty")
	}
	if g.SamplePath == "" {
		t.Error("SamplePath should not be empty")
	}
	if page.TotalCount != 1 {
		t.Errorf("TotalCount: expected 1, got %d", page.TotalCount)
	}
	if page.NextCursor != nil {
		t.Error("NextCursor should be nil for a single-page result")
	}
}

func TestListDuplicateGroupsEmptyResult(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedStorage(t, st, "s1")

	// Only unique files, no duplicates.
	seedFile(t, st, "s1", "/vol/a.txt", 100, "hashA")
	seedFile(t, st, "s1", "/vol/b.txt", 200, "hashB")

	page, err := st.ListDuplicateGroups(ctx, query.GroupQuery{PageSize: 20})
	if err != nil {
		t.Fatalf("ListDuplicateGroups: %v", err)
	}
	if len(page.Groups) != 0 {
		t.Fatalf("expected 0 groups, got %d", len(page.Groups))
	}
	if page.TotalCount != 0 {
		t.Errorf("TotalCount: expected 0, got %d", page.TotalCount)
	}
	if page.NextCursor != nil {
		t.Error("NextCursor should be nil")
	}
}

func TestListDuplicateGroupsPagination(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedStorage(t, st, "s1")

	// Create 5 duplicate groups with different sizes so reclaimable
	// estimates are distinct and ordering is deterministic.
	// Sizes: 500, 400, 300, 200, 100 → reclaimable: 500, 400, 300, 200, 100
	for i, size := range []int64{500, 400, 300, 200, 100} {
		hash := "hash" + string(rune('A'+i))
		seedFile(t, st, "s1", "/vol/"+string(rune('a'+i))+"1.txt", size, hash)
		seedFile(t, st, "s1", "/vol/"+string(rune('a'+i))+"2.txt", size, hash)
	}

	// Page 1: 2 items
	page1, err := st.ListDuplicateGroups(ctx, query.GroupQuery{PageSize: 2})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if len(page1.Groups) != 2 {
		t.Fatalf("page 1: expected 2 groups, got %d", len(page1.Groups))
	}
	if page1.TotalCount != 5 {
		t.Errorf("TotalCount: expected 5, got %d", page1.TotalCount)
	}
	// First page should have the two largest reclaimable groups.
	if page1.Groups[0].Size != 500 {
		t.Errorf("page 1[0].Size: expected 500, got %d", page1.Groups[0].Size)
	}
	if page1.Groups[1].Size != 400 {
		t.Errorf("page 1[1].Size: expected 400, got %d", page1.Groups[1].Size)
	}
	if page1.NextCursor == nil {
		t.Fatal("NextCursor should not be nil when there are more pages")
	}

	// Page 2: 2 items
	page2, err := st.ListDuplicateGroups(ctx, query.GroupQuery{
		PageSize: 2,
		Cursor:   page1.NextCursor,
	})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(page2.Groups) != 2 {
		t.Fatalf("page 2: expected 2 groups, got %d", len(page2.Groups))
	}
	if page2.Groups[0].Size != 300 {
		t.Errorf("page 2[0].Size: expected 300, got %d", page2.Groups[0].Size)
	}
	if page2.Groups[1].Size != 200 {
		t.Errorf("page 2[1].Size: expected 200, got %d", page2.Groups[1].Size)
	}
	if page2.NextCursor == nil {
		t.Fatal("NextCursor should not be nil")
	}

	// Page 3: 1 item (last page)
	page3, err := st.ListDuplicateGroups(ctx, query.GroupQuery{
		PageSize: 2,
		Cursor:   page2.NextCursor,
	})
	if err != nil {
		t.Fatalf("page 3: %v", err)
	}
	if len(page3.Groups) != 1 {
		t.Fatalf("page 3: expected 1 group, got %d", len(page3.Groups))
	}
	if page3.Groups[0].Size != 100 {
		t.Errorf("page 3[0].Size: expected 100, got %d", page3.Groups[0].Size)
	}
	if page3.NextCursor != nil {
		t.Error("NextCursor should be nil on last page")
	}
}

func TestListDuplicateGroupsFilterByStorage(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedStorage(t, st, "s1")
	seedStorage(t, st, "s2")

	// Duplicates in s1
	seedFile(t, st, "s1", "/vol/a.txt", 100, "hashA")
	seedFile(t, st, "s1", "/vol/b.txt", 100, "hashA")
	// Duplicates in s2 (same hash but different storage = different group)
	seedFile(t, st, "s2", "/vol/c.txt", 100, "hashA")
	seedFile(t, st, "s2", "/vol/d.txt", 100, "hashA")

	// Filter by s1 only
	page, err := st.ListDuplicateGroups(ctx, query.GroupQuery{
		StorageID: "s1",
		PageSize:  20,
	})
	if err != nil {
		t.Fatalf("ListDuplicateGroups: %v", err)
	}
	if len(page.Groups) != 1 {
		t.Fatalf("expected 1 group for s1, got %d", len(page.Groups))
	}
	if page.Groups[0].StorageID != "s1" {
		t.Errorf("StorageID: expected s1, got %s", page.Groups[0].StorageID)
	}
	if page.TotalCount != 1 {
		t.Errorf("TotalCount: expected 1, got %d", page.TotalCount)
	}

	// No filter → both storages
	pageAll, err := st.ListDuplicateGroups(ctx, query.GroupQuery{PageSize: 20})
	if err != nil {
		t.Fatalf("ListDuplicateGroups all: %v", err)
	}
	if len(pageAll.Groups) != 2 {
		t.Fatalf("expected 2 groups across storages, got %d", len(pageAll.Groups))
	}
	if pageAll.TotalCount != 2 {
		t.Errorf("TotalCount: expected 2, got %d", pageAll.TotalCount)
	}
}

func TestListDuplicateGroupsMinReclaimableFilter(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedStorage(t, st, "s1")

	// Group with reclaimable estimate = 100 (2 copies × 100 bytes)
	seedFile(t, st, "s1", "/vol/a1.txt", 100, "hashA")
	seedFile(t, st, "s1", "/vol/a2.txt", 100, "hashA")
	// Group with reclaimable estimate = 10000 (2 copies × 10000 bytes)
	seedFile(t, st, "s1", "/vol/b1.txt", 10000, "hashB")
	seedFile(t, st, "s1", "/vol/b2.txt", 10000, "hashB")

	// Filter: min reclaimable 500 → only the 10000-byte group
	page, err := st.ListDuplicateGroups(ctx, query.GroupQuery{
		PageSize:            20,
		MinReclaimableBytes: 500,
	})
	if err != nil {
		t.Fatalf("ListDuplicateGroups: %v", err)
	}
	if len(page.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(page.Groups))
	}
	if page.Groups[0].Size != 10000 {
		t.Errorf("Size: expected 10000, got %d", page.Groups[0].Size)
	}
	if page.TotalCount != 1 {
		t.Errorf("TotalCount: expected 1, got %d", page.TotalCount)
	}
}

func TestListDuplicateGroupsSortOrder(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedStorage(t, st, "s1")

	// Two groups with the same reclaimable estimate (same size, same path count).
	// Sort should fall back to SHA256 ASC.
	seedFile(t, st, "s1", "/vol/a1.txt", 100, "zzz")
	seedFile(t, st, "s1", "/vol/a2.txt", 100, "zzz")
	seedFile(t, st, "s1", "/vol/b1.txt", 100, "aaa")
	seedFile(t, st, "s1", "/vol/b2.txt", 100, "aaa")

	page, err := st.ListDuplicateGroups(ctx, query.GroupQuery{PageSize: 20})
	if err != nil {
		t.Fatalf("ListDuplicateGroups: %v", err)
	}
	if len(page.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(page.Groups))
	}
	// "aaa" should come before "zzz" (ASC on SHA256)
	if page.Groups[0].SHA256 != "aaa" {
		t.Errorf("expected aaa first, got %s", page.Groups[0].SHA256)
	}
	if page.Groups[1].SHA256 != "zzz" {
		t.Errorf("expected zzz second, got %s", page.Groups[1].SHA256)
	}
}

func TestListDuplicateGroupsExcludesMissingFiles(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedStorage(t, st, "s1")

	seedFile(t, st, "s1", "/vol/a.txt", 100, "hashA")
	seedFile(t, st, "s1", "/vol/b.txt", 100, "hashA")

	// Mark /vol/b.txt as missing by keeping only /vol/a.txt in the "seen" set.
	// MarkFilesMissing marks all paths NOT in the seen list as missing.
	if _, err := st.MarkFilesMissing(ctx, "s1", []string{"/vol/a.txt"}); err != nil {
		t.Fatalf("mark missing: %v", err)
	}

	page, err := st.ListDuplicateGroups(ctx, query.GroupQuery{PageSize: 20})
	if err != nil {
		t.Fatalf("ListDuplicateGroups: %v", err)
	}
	if len(page.Groups) != 0 {
		t.Fatalf("expected 0 groups after marking file missing, got %d", len(page.Groups))
	}
}

func TestListDuplicateGroupsHardlinkStats(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedStorage(t, st, "s1")

	// 3 paths, all hardlinks (same Device+Inode) = 1 physical copy.
	// Reclaimable = (1-1) * 500 = 0
	seedFileWithPhysical(t, st, "s1", "/vol/a.txt", 500, "hashX", 1, 99)
	seedFileWithPhysical(t, st, "s1", "/vol/b.txt", 500, "hashX", 1, 99)
	seedFileWithPhysical(t, st, "s1", "/vol/c.txt", 500, "hashX", 1, 99)

	page, err := st.ListDuplicateGroups(ctx, query.GroupQuery{PageSize: 20})
	if err != nil {
		t.Fatalf("ListDuplicateGroups: %v", err)
	}
	if len(page.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(page.Groups))
	}
	g := page.Groups[0]
	if g.PathCount != 3 {
		t.Errorf("PathCount: expected 3, got %d", g.PathCount)
	}
	if g.PhysicalCopyCount != 1 {
		t.Errorf("PhysicalCopyCount: expected 1 (all hardlinks), got %d", g.PhysicalCopyCount)
	}
	if g.HardlinkAliasCount != 2 {
		t.Errorf("HardlinkAliasCount: expected 2, got %d", g.HardlinkAliasCount)
	}
	if g.PhysicalReclaimableBytes != 0 {
		t.Errorf("PhysicalReclaimableBytes: expected 0, got %d", g.PhysicalReclaimableBytes)
	}
}

func TestListDuplicateGroupsMinReclaimableExcludesHardlinkOnlyGroup(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedStorage(t, st, "s1")

	// Three paths reference one physical object, so physical reclaimable is 0.
	seedFileWithPhysical(t, st, "s1", "/vol/a.txt", 500, "hashX", 1, 99)
	seedFileWithPhysical(t, st, "s1", "/vol/b.txt", 500, "hashX", 1, 99)
	seedFileWithPhysical(t, st, "s1", "/vol/c.txt", 500, "hashX", 1, 99)

	page, err := st.ListDuplicateGroups(ctx, query.GroupQuery{
		PageSize: 20, MinReclaimableBytes: 1,
	})
	if err != nil {
		t.Fatalf("ListDuplicateGroups: %v", err)
	}
	if len(page.Groups) != 0 || page.TotalCount != 0 {
		t.Fatalf("hardlink-only group should be filtered out: %#v", page)
	}
}

func TestListDuplicateGroupsSortsByPhysicalReclaimable(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedStorage(t, st, "s1")

	// A large path-level group is one physical object and frees no space.
	seedFileWithPhysical(t, st, "s1", "/vol/a1", 10000, "aaa", 1, 10)
	seedFileWithPhysical(t, st, "s1", "/vol/a2", 10000, "aaa", 1, 10)
	// A smaller group contains two physical copies and frees 100 bytes.
	seedFileWithPhysical(t, st, "s1", "/vol/b1", 100, "bbb", 1, 20)
	seedFileWithPhysical(t, st, "s1", "/vol/b2", 100, "bbb", 1, 21)

	page, err := st.ListDuplicateGroups(ctx, query.GroupQuery{PageSize: 20})
	if err != nil {
		t.Fatalf("ListDuplicateGroups: %v", err)
	}
	if len(page.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(page.Groups))
	}
	if page.Groups[0].SHA256 != "bbb" {
		t.Fatalf("expected physically reclaimable group first, got %s", page.Groups[0].SHA256)
	}
}

func TestListDuplicateGroupsWithDecision(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedStorage(t, st, "s1")

	seedFile(t, st, "s1", "/vol/a.txt", 100, "hashA")
	seedFile(t, st, "s1", "/vol/b.txt", 100, "hashA")

	// First page to get the group_id
	page, err := st.ListDuplicateGroups(ctx, query.GroupQuery{PageSize: 20})
	if err != nil {
		t.Fatalf("ListDuplicateGroups: %v", err)
	}
	if len(page.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(page.Groups))
	}
	groupID := page.Groups[0].GroupID

	// Insert a decision
	seedGroupDecision(t, st, groupID, "KEEP_ALL", "user reviewed")

	// Query again — decision should be populated
	page2, err := st.ListDuplicateGroups(ctx, query.GroupQuery{PageSize: 20})
	if err != nil {
		t.Fatalf("ListDuplicateGroups: %v", err)
	}
	if page2.Groups[0].DecisionType != "KEEP_ALL" {
		t.Errorf("DecisionType: expected KEEP_ALL, got %s", page2.Groups[0].DecisionType)
	}
}

func TestGetGroupDetailBasic(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedStorage(t, st, "s1")

	seedFile(t, st, "s1", "/vol/a.txt", 100, "hashA")
	seedFile(t, st, "s1", "/vol/b.txt", 100, "hashA")
	seedFile(t, st, "s1", "/vol/c.txt", 100, "hashA")

	detail, err := st.GetGroupDetail(ctx, "s1", "hashA")
	if err != nil {
		t.Fatalf("GetGroupDetail: %v", err)
	}
	if detail.SHA256 != "hashA" {
		t.Errorf("SHA256: expected hashA, got %s", detail.SHA256)
	}
	if detail.StorageID != "s1" {
		t.Errorf("StorageID: expected s1, got %s", detail.StorageID)
	}
	if detail.PathCount != 3 {
		t.Errorf("PathCount: expected 3, got %d", detail.PathCount)
	}
	if len(detail.Files) != 3 {
		t.Errorf("Files: expected 3, got %d", len(detail.Files))
	}
	// Files should be sorted by path
	if detail.Files[0].Path != "/vol/a.txt" {
		t.Errorf("Files[0].Path: expected /vol/a.txt, got %s", detail.Files[0].Path)
	}
	if detail.Files[1].Path != "/vol/b.txt" {
		t.Errorf("Files[1].Path: expected /vol/b.txt, got %s", detail.Files[1].Path)
	}
	if detail.Files[2].Path != "/vol/c.txt" {
		t.Errorf("Files[2].Path: expected /vol/c.txt, got %s", detail.Files[2].Path)
	}
	if detail.PhysicalReclaimableBytes != 200 {
		t.Errorf("PhysicalReclaimableBytes: expected 200, got %d", detail.PhysicalReclaimableBytes)
	}
}

func TestGetGroupDetailNotFound(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedStorage(t, st, "s1")

	seedFile(t, st, "s1", "/vol/a.txt", 100, "hashA")

	_, err := st.GetGroupDetail(ctx, "s1", "nonexistent")
	if err != query.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetGroupDetailExcludesMissingFiles(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedStorage(t, st, "s1")

	seedFile(t, st, "s1", "/vol/a.txt", 100, "hashA")
	seedFile(t, st, "s1", "/vol/b.txt", 100, "hashA")
	seedFile(t, st, "s1", "/vol/c.txt", 100, "hashA")

	// Mark /vol/c.txt as missing by passing the "seen" paths (a and b).
	// MarkFilesMissing marks all paths NOT in the seen list as missing.
	if _, err := st.MarkFilesMissing(ctx, "s1", []string{"/vol/a.txt", "/vol/b.txt"}); err != nil {
		t.Fatalf("mark missing: %v", err)
	}

	detail, err := st.GetGroupDetail(ctx, "s1", "hashA")
	if err != nil {
		t.Fatalf("GetGroupDetail: %v", err)
	}
	if detail.PathCount != 2 {
		t.Errorf("PathCount: expected 2 (missing excluded), got %d", detail.PathCount)
	}
	if len(detail.Files) != 2 {
		t.Errorf("Files: expected 2, got %d", len(detail.Files))
	}
}

func TestGetGroupDetailWithDecision(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedStorage(t, st, "s1")

	seedFile(t, st, "s1", "/vol/a.txt", 100, "hashA")
	seedFile(t, st, "s1", "/vol/b.txt", 100, "hashA")

	// Compute the group_id using the same algorithm as the production code
	groupID := report.StableGroupID("s1", "hashA")
	seedGroupDecision(t, st, groupID, "DRAFT_ACTION", "needs cleanup")

	detail, err := st.GetGroupDetail(ctx, "s1", "hashA")
	if err != nil {
		t.Fatalf("GetGroupDetail: %v", err)
	}
	if detail.DecisionType != "DRAFT_ACTION" {
		t.Errorf("DecisionType: expected DRAFT_ACTION, got %s", detail.DecisionType)
	}
}

func TestListDuplicateGroupsDefaultPageSize(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedStorage(t, st, "s1")

	// Create 25 duplicate groups
	for i := 0; i < 25; i++ {
		hash := "hash" + string(rune('A'+i))
		seedFile(t, st, "s1", "/vol/"+string(rune('a'+i))+"1.txt", int64(100+i), hash)
		seedFile(t, st, "s1", "/vol/"+string(rune('a'+i))+"2.txt", int64(100+i), hash)
	}

	// PageSize = 0 should default to 20
	page, err := st.ListDuplicateGroups(ctx, query.GroupQuery{PageSize: 0})
	if err != nil {
		t.Fatalf("ListDuplicateGroups: %v", err)
	}
	if len(page.Groups) != 20 {
		t.Fatalf("expected 20 groups (default page size), got %d", len(page.Groups))
	}
	if page.TotalCount != 25 {
		t.Errorf("TotalCount: expected 25, got %d", page.TotalCount)
	}
	if page.NextCursor == nil {
		t.Error("NextCursor should not be nil (more pages exist)")
	}
}

func TestListDuplicateGroupsPageSizeOver200Capped(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedStorage(t, st, "s1")

	// Create 2 duplicate groups
	seedFile(t, st, "s1", "/vol/a1.txt", 100, "hashA")
	seedFile(t, st, "s1", "/vol/a2.txt", 100, "hashA")

	// PageSize = 999 should be capped to 200, but still return all results
	page, err := st.ListDuplicateGroups(ctx, query.GroupQuery{PageSize: 999})
	if err != nil {
		t.Fatalf("ListDuplicateGroups: %v", err)
	}
	if len(page.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(page.Groups))
	}
}
