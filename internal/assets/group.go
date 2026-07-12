// Package assets clusters files into asset groups by business anchor or
// path proximity. It is read-only: it never touches the filesystem, only
// interprets path strings (K-001/K-002). Clustering is conservative — a
// false negative just leaves files ungrouped, while a false positive could
// wrongly collapse distinct business matters.
package assets

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"nas-data-governance/internal/dircontext"
	"nas-data-governance/internal/domain"
)

// pathSegmentDepth is how many path segments (from the root) are used as a
// fallback cluster key when no business anchor is detected. Three levels
// balances granularity: deep enough to separate distinct projects, shallow
// enough to group a project's scattered files.
const pathSegmentDepth = 3

// minMembers is the smallest group size worth reporting. A single file is
// not an asset group.
const minMembers = 2

// Group clusters files into asset groups. Files sharing a business anchor
// (project code, year folder) are grouped by that anchor. Files without a
// detectable anchor are grouped by their first pathSegmentDepth directory
// segments. Groups with fewer than minMembers members are discarded.
func Group(files []domain.FileInstance) []domain.AssetGroup {
	buckets := map[string][]domain.FileInstance{} // cluster key -> files
	evidence := map[string][]string{}             // cluster key -> reasons
	anchors := map[string]string{}                // cluster key -> anchor (if any)

	for _, f := range files {
		ctx := dircontext.Classify(f.Path)
		key, ev, anchor := clusterKey(f.Path, ctx)
		buckets[key] = append(buckets[key], f)
		evidence[key] = append(evidence[key], ev)
		anchors[key] = anchor
	}

	groups := make([]domain.AssetGroup, 0, len(buckets))
	for key, members := range buckets {
		if len(members) < minMembers {
			continue
		}
		root := commonDirPrefix(members)
		g := domain.AssetGroup{
			ID:       groupID(key),
			Anchor:   anchors[key],
			RootPath: root,
			Members:  sortedMembers(members),
			Evidence: dedupeEvidence(evidence[key]),
		}
		if g.Anchor != "" {
			g.Evidence = append(g.Evidence, fmt.Sprintf("业务锚点：%s", g.Anchor))
		} else {
			g.Evidence = append(g.Evidence, fmt.Sprintf("路径前 %d 段聚类：%s", pathSegmentDepth, key))
		}
		groups = append(groups, g)
	}
	// Stable output order for deterministic reports/tests.
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].ID < groups[j].ID
	})
	return groups
}

// clusterKey returns (key, evidence, anchor). When the directory context
// carries a business anchor, the anchor is the key. Otherwise the first
// pathSegmentDepth segments of the path form the key.
func clusterKey(path string, ctx domain.DirectoryContext) (string, string, string) {
	if ctx.BusinessAnchor != "" {
		return "anchor:" + ctx.BusinessAnchor,
			fmt.Sprintf("路径 %s 命中业务锚点 %s", path, ctx.BusinessAnchor),
			ctx.BusinessAnchor
	}
	segs := splitSegments(filepath.ToSlash(filepath.Dir(path)))
	if len(segs) > pathSegmentDepth {
		segs = segs[:pathSegmentDepth]
	}
	key := "path:" + strings.Join(segs, "/")
	return key, fmt.Sprintf("路径 %s 无业务锚点，按路径前缀聚类", path), ""
}

func splitSegments(dir string) []string {
	// Trim leading slash so the first segment isn't empty on absolute paths.
	dir = strings.TrimPrefix(dir, "/")
	if dir == "" {
		return nil
	}
	return strings.Split(dir, "/")
}

// commonDirPrefix returns the deepest directory that is a prefix of every
// member's parent directory. Falls back to the first member's dir when no
// common prefix exists.
func commonDirPrefix(files []domain.FileInstance) string {
	if len(files) == 0 {
		return ""
	}
	dirs := make([]string, len(files))
	for i, f := range files {
		dirs[i] = filepath.ToSlash(filepath.Dir(f.Path))
	}
	prefix := dirs[0]
	for _, d := range dirs[1:] {
		for !strings.HasPrefix(d+"/", prefix+"/") && prefix != "" {
			idx := strings.LastIndex(prefix, "/")
			if idx <= 0 {
				prefix = ""
			} else {
				prefix = prefix[:idx]
			}
		}
	}
	if prefix == "" {
		return "/"
	}
	return prefix
}

func sortedMembers(files []domain.FileInstance) []domain.FileInstance {
	out := make([]domain.FileInstance, len(files))
	copy(out, files)
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].ModifiedAt.Equal(out[j].ModifiedAt) {
			return out[i].ModifiedAt.Before(out[j].ModifiedAt)
		}
		return strings.Compare(out[i].Path, out[j].Path) < 0
	})
	return out
}

func dedupeEvidence(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func groupID(key string) string {
	sum := sha1.Sum([]byte(key))
	return "grp-" + hex.EncodeToString(sum[:6])
}
