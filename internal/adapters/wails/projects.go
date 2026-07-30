package wails

// This file is intentionally a thin Wails adapter. Project lifecycle,
// validation, metadata, recent-project, rollback, and concurrency rules live
// in internal/project.Service so other entry points can reuse them.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	projectsvc "github.com/FNB2026/nas-data-governance/internal/project"
)

type DirectoryPicker func(title string) (string, error)

type RecentProjectEntry struct {
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	OpenedAt time.Time `json:"opened_at"`
}

type CreateProjectInput struct {
	Name       string `json:"name"`
	SourcePath string `json:"source_path"`
}

var (
	appSupportBaseFn   = projectsvc.DefaultSupportBase
	writeProjectMetaFn projectsvc.MetaWriter
)

func newProjectService() *projectsvc.Service {
	return projectsvc.NewService(projectsvc.Options{
		SupportBase: func() (string, error) { return appSupportBaseFn() },
		WriteMeta:   writeProjectMetaFn,
	})
}

func (a *API) projectService() *projectsvc.Service {
	if a.projectSvc == nil {
		a.projectSvc = newProjectService()
	}
	return a.projectSvc
}

func (a *API) SetDirectoryPicker(picker DirectoryPicker) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.dirPicker = picker
}

func (a *API) PickDirectory(title string) (string, error) {
	a.mu.RLock()
	picker := a.dirPicker
	a.mu.RUnlock()
	if picker == nil {
		return "", errors.New("wails: directory picker is not configured")
	}
	if strings.TrimSpace(title) == "" {
		title = "选择目录"
	}
	return picker(title)
}

func (a *API) CreateProjectFromSource(input CreateProjectInput) (ProjectInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.store != nil {
		return ProjectInfo{}, ErrProjectAlreadyOpen
	}
	created, err := a.projectService().CreateFromSource(context.Background(), projectsvc.CreateInput{
		Name: input.Name, SourcePath: input.SourcePath,
	})
	if err != nil {
		return ProjectInfo{}, fmt.Errorf("wails: create project from source: %w", err)
	}
	a.initReadWriteServicesLocked(created.Store)
	a.path = created.DatabasePath
	a.projectID = created.ProjectID
	a.jobScope = created.ProjectID
	info, err := a.projectInfoLocked(context.Background())
	if err != nil {
		a.resetProjectStateLocked()
		_ = a.projectService().RollbackProjectCreation(created)
		return ProjectInfo{}, err
	}
	_ = a.projectService().RecordRecent(created.Name, created.DatabasePath)
	return info, nil
}

func (a *API) RegisterScanSource(root string) (StorageInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.store == nil {
		return StorageInfo{}, ErrNoProjectOpen
	}
	storage, err := a.projectService().RegisterScanSource(context.Background(), a.store, root, a.path)
	if err != nil {
		return StorageInfo{}, fmt.Errorf("wails: register scan source: %w", err)
	}
	return mapStorages([]domain.Storage{storage})[0], nil
}

func (a *API) ListRecentProjects() ([]RecentProjectEntry, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	entries, err := a.projectService().ListRecent()
	if err != nil {
		return nil, fmt.Errorf("wails: list recent projects: %w", err)
	}
	out := make([]RecentProjectEntry, len(entries))
	for i, entry := range entries {
		out[i] = RecentProjectEntry{Name: entry.Name, Path: entry.Path, OpenedAt: entry.OpenedAt}
	}
	return out, nil
}

func (a *API) recordRecentLocked(name, path string) error {
	return a.projectService().RecordRecent(name, path)
}
func (a *API) resolveProjectIdentity(path string) (string, string, string) {
	return a.projectService().ResolveIdentity(path)
}
func (a *API) projectDisplayName(path string) string { return a.projectService().DisplayName(path) }
func (a *API) rejectScanRootContainingProjectFiles(root, dbPath string) error {
	return projectsvc.RejectScanRootContainingProjectFiles(root, dbPath)
}
