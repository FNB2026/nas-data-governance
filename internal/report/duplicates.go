package report

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

// DuplicateGroups groups files by ContentSHA256 and computes physical
// identity statistics for each group. Hardlinks (same Device+Inode) are
// counted as one physical copy so that reclaimable capacity is not
// overstated.
//
// Per ADR-0006 / V1 correctness:
//   - Groups are sorted by PhysicalReclaimableBytes DESC, then SHA256 ASC
//     for deterministic output.
//   - GroupID is a stable hash of governance_domain_id + storage_id +
//     content_sha256, independent of array order or pagination.
//   - When physical identity is unreliable (e.g., SMB without stable
//     inodes), each path is treated as a separate physical copy.
func DuplicateGroups(files []domain.FileInstance) []domain.DuplicateGroup {
	byHash := map[string][]domain.FileInstance{}
	for _, f := range files {
		if f.ContentSHA256 != "" {
			byHash[f.ContentSHA256] = append(byHash[f.ContentSHA256], f)
		}
	}
	groups := make([]domain.DuplicateGroup, 0)
	for hash, members := range byHash {
		if len(members) < 2 {
			continue
		}
		groups = append(groups, buildGroup(hash, members))
	}
	// Deterministic sort: reclaimable bytes DESC, then SHA256 ASC.
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].PhysicalReclaimableBytes != groups[j].PhysicalReclaimableBytes {
			return groups[i].PhysicalReclaimableBytes > groups[j].PhysicalReclaimableBytes
		}
		return groups[i].SHA256 < groups[j].SHA256
	})
	return groups
}

// PhysicalStats holds the computed physical identity statistics for a
// duplicate group. These values are derived from Device+Inode analysis
// and are used by both the in-memory report builder and the database-
// backed query read model.
type PhysicalStats struct {
	PhysicalCopyCount        int
	HardlinkAliasCount       int
	PhysicalReclaimableBytes int64
}

// ComputePhysicalStats computes physical identity statistics for a set of
// file instances that share the same content hash and storage. Hardlinks
// (same Device+Inode) are counted as one physical copy so that reclaimable
// capacity is not overstated.
//
// When physical identity is unreliable (e.g., SMB without stable inodes),
// each path is treated as a separate physical copy (conservative).
func ComputePhysicalStats(members []domain.FileInstance, size int64) PhysicalStats {
	pathCount := len(members)

	// Count distinct reliable physical objects via PhysicalKey. Every
	// unreliable member is counted separately: an empty key means "unknown",
	// never "the same object as another unknown member".
	physicalSet := map[string]struct{}{}
	unreliableCount := 0
	for _, f := range members {
		key := f.Physical.PhysicalKey(f.StorageID)
		if key == "" {
			unreliableCount++
			continue
		}
		physicalSet[key] = struct{}{}
	}
	physicalCopyCount := len(physicalSet) + unreliableCount

	hardlinkAliasCount := pathCount - physicalCopyCount
	if hardlinkAliasCount < 0 {
		hardlinkAliasCount = 0
	}

	// Reclaimable = (physical_copies - 1) * size.
	var reclaimable int64
	if physicalCopyCount > 1 {
		reclaimable = int64(physicalCopyCount-1) * size
	}

	return PhysicalStats{
		PhysicalCopyCount:        physicalCopyCount,
		HardlinkAliasCount:       hardlinkAliasCount,
		PhysicalReclaimableBytes: reclaimable,
	}
}

// buildGroup constructs a DuplicateGroup with physical identity statistics.
func buildGroup(hash string, members []domain.FileInstance) domain.DuplicateGroup {
	stats := ComputePhysicalStats(members, members[0].Size)
	return domain.DuplicateGroup{
		SHA256:                   hash,
		Size:                     members[0].Size,
		Files:                    members,
		PathCount:                len(members),
		GroupID:                  StableGroupID(members[0].StorageID, hash),
		PhysicalCopyCount:        stats.PhysicalCopyCount,
		HardlinkAliasCount:       stats.HardlinkAliasCount,
		PhysicalReclaimableBytes: stats.PhysicalReclaimableBytes,
	}
}

// StableGroupID computes a deterministic group identifier from
// storage_id and content SHA256. This is independent of array order and
// pagination cursors.
//
// Format: hex(SHA256(storage_id + ":" + content_sha256)) truncated to
// 32 hex chars (16 bytes), sufficient for UI use.
//
// Note: governance_domain_id is not yet a separate concept in the schema;
// for now storage_id serves as the domain boundary (per V1: same storage =
// same governance domain; cross-storage = "related copies" only).
func StableGroupID(storageID, contentSHA256 string) string {
	h := sha256.Sum256([]byte(storageID + ":" + contentSHA256))
	return hex.EncodeToString(h[:16]) // 32 hex chars, sufficient for UI use
}
