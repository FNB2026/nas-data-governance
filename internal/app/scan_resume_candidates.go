package app

import (
	"fmt"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// resumeHashCandidates reconstructs work from duplicate groups, not from a
// missing SHA alone: unique files deliberately have no full hash. Fresh session
// metadata overrides the cache by path. Peers with completed hashes are included
// for revalidation, and a changed quick hash can expose another prefix peer in a
// subsequent pass. visited bounds those passes to at most one visit per path.
func resumeHashCandidates(cache map[string]store.FileMeta, boundary string, files []domain.FileInstance, visited map[string]bool) map[string]bool {
	type candidate struct {
		path, quick, full string
		size              int64
	}
	byPath := make(map[string]candidate, len(cache)+len(files))
	for path, m := range cache {
		if path <= boundary {
			byPath[path] = candidate{path, m.QuickHash, m.ContentSHA256, m.Size}
		}
	}
	for _, f := range files {
		byPath[f.Path] = candidate{f.Path, f.QuickHash, f.ContentSHA256, f.Size}
	}
	groups := map[string][]candidate{}
	for _, c := range byPath {
		if c.quick != "" {
			key := fmt.Sprintf("%d:%s", c.size, c.quick)
			groups[key] = append(groups[key], c)
		}
	}
	selected := map[string]bool{}
	for _, group := range groups {
		pending := false
		for _, c := range group {
			pending = pending || c.full == ""
		}
		if len(group) < 2 || !pending {
			continue
		}
		for _, c := range group {
			if c.path <= boundary && !visited[c.path] {
				selected[c.path] = true
			}
		}
	}
	return selected
}
