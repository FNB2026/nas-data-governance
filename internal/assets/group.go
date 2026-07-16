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

	"github.com/FNB2026/nas-data-governance/internal/dircontext"
	"github.com/FNB2026/nas-data-governance/internal/domain"
)

// pathSegmentDepth is how many path segments (from the root) are used as a
// fallback cluster key when no business anchor is detected. Three levels
// balances granularity: deep enough to separate distinct projects, shallow
// enough to group a project's scattered files.
const pathSegmentDepth = 3

// minMembers is the smallest group size worth reporting. A single file is
// not an asset group.
const minMembers = 2

// maxGroupMembers is a safety ceiling, not a claim that 10,000 files form
// one business asset. Buckets above it are split by progressively deeper
// relative paths and finally deterministic shards. False negatives are safer
// than silently collapsing unrelated business matters.
const maxGroupMembers = 10000

// Group clusters files into asset groups. Files sharing a business anchor
// (project code, year folder) are grouped by that anchor. Files without a
// detectable anchor are grouped by their first pathSegmentDepth directory
// segments. Groups with fewer than minMembers members are discarded.
func Group(files []domain.FileInstance) []domain.AssetGroup {
	buckets := map[string][]domain.FileInstance{} // cluster key -> files
	evidence := map[string][]string{}             // cluster key -> reasons
	anchors := map[string]string{}                // cluster key -> anchor (if any)

	root := commonDirPrefix(files)
	for _, f := range files {
		ctx := dircontext.Classify(f.Path)
		key, ev, anchor := clusterKey(f.Path, ctx, root)
		buckets[key] = append(buckets[key], f)
		evidence[key] = append(evidence[key], ev)
		anchors[key] = anchor
	}

	type candidate struct {
		key, anchor string
		members     []domain.FileInstance
		evidence    []string
		review      bool
	}
	candidates := make([]candidate, 0, len(buckets))
	for key, members := range buckets {
		if len(members) < minMembers {
			continue
		}
		parts := splitOversized(key, members, root)
		for _, part := range parts {
			if len(part.members) < minMembers {
				continue
			}
			candidates = append(candidates, candidate{
				key: part.key, anchor: anchors[key], members: part.members,
				evidence: evidence[key], review: part.split,
			})
		}
	}

	groups := make([]domain.AssetGroup, 0, len(candidates))
	for _, c := range candidates {
		key, members := c.key, c.members
		root := commonDirPrefix(members)
		g := domain.AssetGroup{
			ID:       groupID(key),
			Anchor:   c.anchor,
			RootPath: root,
			Members:  sortedMembers(members),
			Evidence: dedupeEvidence(c.evidence),
		}
		if c.review {
			g.ReviewRequired = true
			g.ReviewReason = "超大候选组已按更深路径安全拆分，边界必须人工复核"
			g.Evidence = append(g.Evidence, "超大候选组安全拆分：保护规则优先，禁止跨分片自动合并")
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

type groupPart struct {
	key     string
	members []domain.FileInstance
	split   bool
}

func splitOversized(key string, members []domain.FileInstance, root string) []groupPart {
	if len(members) <= maxGroupMembers {
		return []groupPart{{key: key, members: members}}
	}
	parts := []groupPart{{key: key, members: members, split: true}}
	for depth := 2; depth <= 8; depth++ {
		next := make([]groupPart, 0, len(parts))
		changed := false
		for _, part := range parts {
			if len(part.members) <= maxGroupMembers {
				next = append(next, part)
				continue
			}
			buckets := map[string][]domain.FileInstance{}
			for _, file := range part.members {
				rel, err := filepath.Rel(root, filepath.Dir(file.Path))
				if err != nil {
					rel = filepath.Dir(file.Path)
				}
				segs := splitSegments(filepath.ToSlash(rel))
				if len(segs) > depth {
					segs = segs[:depth]
				}
				prefix := strings.Join(segs, "/")
				buckets[prefix] = append(buckets[prefix], file)
			}
			if len(buckets) == 1 {
				next = append(next, part)
				continue
			}
			changed = true
			keys := make([]string, 0, len(buckets))
			for prefix := range buckets {
				keys = append(keys, prefix)
			}
			sort.Strings(keys)
			for _, prefix := range keys {
				next = append(next, groupPart{key: key + ":path:" + prefix, members: buckets[prefix], split: true})
			}
		}
		parts = next
		if !changed {
			break
		}
	}

	final := make([]groupPart, 0, len(parts))
	for _, part := range parts {
		if len(part.members) <= maxGroupMembers {
			final = append(final, part)
			continue
		}
		sorted := sortedMembers(part.members)
		for start, shard := 0, 0; start < len(sorted); start, shard = start+maxGroupMembers, shard+1 {
			end := start + maxGroupMembers
			if end > len(sorted) {
				end = len(sorted)
			}
			final = append(final, groupPart{
				key:     fmt.Sprintf("%s:shard:%04d", part.key, shard),
				members: sorted[start:end], split: true,
			})
		}
	}
	return final
}

// clusterKey returns (key, evidence, anchor). When the directory context
// carries a business anchor, the anchor is the key. Otherwise the first
// pathSegmentDepth segments of the path form the key.
func clusterKey(path string, ctx domain.DirectoryContext, root string) (string, string, string) {
	relDir := filepath.ToSlash(filepath.Dir(path))
	if rel, err := filepath.Rel(root, filepath.Dir(path)); err == nil {
		relDir = filepath.ToSlash(rel)
	}
	relSegs := splitSegments(relDir)
	scope := ""
	if len(relSegs) > 0 {
		scope = relSegs[0]
	}
	if ctx.BusinessAnchor != "" {
		return "anchor:" + scope + ":" + ctx.BusinessAnchor,
			"目录语境检测到业务锚点；具体来源仅保留在成员记录中",
			ctx.BusinessAnchor
	}
	segs := relSegs
	if len(segs) > pathSegmentDepth {
		segs = segs[:pathSegmentDepth]
	}
	key := "path:" + strings.Join(segs, "/")
	return key, "未检测到业务锚点，按相对路径前缀聚类；具体来源仅保留在成员记录中", ""
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
