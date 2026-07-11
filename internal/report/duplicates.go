package report

import "nas-data-governance/internal/domain"

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
		groups = append(groups, domain.DuplicateGroup{SHA256: hash, Size: members[0].Size, Files: members})
	}
	return groups
}
