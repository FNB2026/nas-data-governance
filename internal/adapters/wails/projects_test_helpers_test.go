package wails

import projectsvc "github.com/FNB2026/nas-data-governance/internal/project"

type ProjectMeta = projectsvc.ProjectMeta

const projectsSubDir = projectsvc.ProjectsSubDir

func appSupportBase() (string, error)      { return appSupportBaseFn() }
func normalizeNameID(name string) string   { return projectsvc.NormalizeNameID(name) }
func generateStorageID(root string) string { return projectsvc.GenerateStorageID(root) }
func isWithin(path, parent string) bool    { return projectsvc.IsWithin(path, parent) }
func readProjectMeta(projectDir string) (ProjectMeta, bool) {
	return newProjectService().ReadMeta(projectDir)
}
func projectDisplayName(path string) string { return newProjectService().DisplayName(path) }
func recentPath() (string, error)           { return newProjectService().RecentPath() }
func readRecentManifest() ([]RecentProjectEntry, error) {
	entries, err := newProjectService().ListRecent()
	if err != nil {
		return nil, err
	}
	out := make([]RecentProjectEntry, len(entries))
	for i, entry := range entries {
		out[i] = RecentProjectEntry{Name: entry.Name, Path: entry.Path, OpenedAt: entry.OpenedAt}
	}
	return out, nil
}
func writeRecentManifest(entries []RecentProjectEntry) error {
	svc := newProjectService()
	mapped := make([]projectsvc.RecentEntry, len(entries))
	for i, entry := range entries {
		mapped[i] = projectsvc.RecentEntry{Name: entry.Name, Path: entry.Path, OpenedAt: entry.OpenedAt}
	}
	return svc.ReplaceRecent(mapped)
}
