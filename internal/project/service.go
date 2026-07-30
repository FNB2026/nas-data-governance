// Package project owns the desktop project lifecycle independently of Wails.
// It can be reused by a CLI, another desktop shell, or automation without
// duplicating source validation, metadata, recent-project, or rollback rules.
package project

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/privatefs"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

const (
	AppSupportDirName   = "NDG"
	ProjectsSubDir      = "projects"
	DatabaseFileName    = "governance.db"
	ProjectMetaFileName = "project.json"
	RecentFileName      = "recent.json"
	recentMaxEntries    = 20
	projectMetaSchema   = 1
)

type SupportBaseFunc func() (string, error)
type MetaWriter func(string, ProjectMeta) error

type Options struct {
	SupportBase SupportBaseFunc
	WriteMeta   MetaWriter
}

type Service struct {
	supportBase SupportBaseFunc
	writeMeta   MetaWriter
}

type CreateInput struct {
	Name       string
	SourcePath string
}

type CreatedProject struct {
	Store        *store.SQLiteStore
	ProjectID    string
	Name         string
	DatabasePath string
	ProjectDir   string
	ProjectsRoot string
	Storage      domain.Storage
}

type ProjectMeta struct {
	SchemaVersion int       `json:"schemaVersion"`
	ProjectID     string    `json:"projectId"`
	Name          string    `json:"name"`
	Database      string    `json:"database"`
	CreatedAt     time.Time `json:"createdAt"`
}

type RecentEntry struct {
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	OpenedAt time.Time `json:"opened_at"`
}

func NewService(opts Options) *Service {
	base := opts.SupportBase
	if base == nil {
		base = DefaultSupportBase
	}
	svc := &Service{supportBase: base}
	if opts.WriteMeta != nil {
		svc.writeMeta = opts.WriteMeta
	} else {
		svc.writeMeta = svc.writeProjectMeta
	}
	return svc
}

func DefaultSupportBase() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("project: resolve app support dir: %w", err)
	}
	return filepath.Join(base, AppSupportDirName), nil
}

func (s *Service) SupportBase() (string, error) { return s.supportBase() }

// CreateFromSource performs the complete storage-facing project creation
// transaction. It claims the project directory atomically, opens/migrates the
// database, registers the source, and writes project.json. Any failure removes
// only the directory claimed by this call.
func (s *Service) CreateFromSource(ctx context.Context, input CreateInput) (*CreatedProject, error) {
	sourceRoot, err := s.ValidateScanSource(input.SourcePath, nil, "")
	if err != nil {
		return nil, err
	}
	base, err := s.supportBase()
	if err != nil {
		return nil, err
	}
	if err := privatefs.SecureDirectory(base); err != nil {
		return nil, fmt.Errorf("project: create app support dir: %w", err)
	}
	projectsRoot := filepath.Join(base, ProjectsSubDir)
	if err := privatefs.SecureDirectory(projectsRoot); err != nil {
		return nil, fmt.Errorf("project: create projects dir: %w", err)
	}

	displayName := strings.TrimSpace(input.Name)
	if displayName == "" {
		displayName = filepath.Base(filepath.Clean(sourceRoot))
		if displayName == "." || displayName == string(filepath.Separator) {
			displayName = ""
		}
	}
	id, projectDir, err := claimProjectDirectory(projectsRoot, displayName)
	if err != nil {
		return nil, err
	}
	dbPath := filepath.Join(projectDir, DatabaseFileName)
	rollback := func() { _ = removeClaimedProjectDir(projectDir, projectsRoot) }

	st, err := store.Open(ctx, dbPath)
	if err != nil {
		rollback()
		return nil, fmt.Errorf("project: create project database: %w", err)
	}
	fail := func(cause error) (*CreatedProject, error) {
		_ = st.Close()
		rollback()
		return nil, cause
	}

	storage := domain.Storage{
		ID:        GenerateStorageID(sourceRoot),
		RootPath:  sourceRoot,
		Kind:      "filesystem",
		CreatedAt: time.Now().UTC(),
	}
	if err := st.RegisterStorage(ctx, storage); err != nil {
		return fail(fmt.Errorf("project: register scan source: %w", err))
	}
	if displayName == "" {
		displayName = id
	}
	meta := ProjectMeta{
		SchemaVersion: projectMetaSchema,
		ProjectID:     id,
		Name:          displayName,
		Database:      DatabaseFileName,
		CreatedAt:     time.Now().UTC(),
	}
	if err := s.writeMeta(projectDir, meta); err != nil {
		return fail(err)
	}
	return &CreatedProject{
		Store:        st,
		ProjectID:    id,
		Name:         displayName,
		DatabasePath: dbPath,
		ProjectDir:   projectDir,
		ProjectsRoot: projectsRoot,
		Storage:      storage,
	}, nil
}

// RollbackProjectCreation is for adapter setup failures after CreateFromSource
// returned successfully. It closes the store and removes only the atomically
// claimed project directory.
func (s *Service) RollbackProjectCreation(created *CreatedProject) error {
	if created == nil {
		return nil
	}
	if created.Store != nil {
		_ = created.Store.Close()
	}
	return removeClaimedProjectDir(created.ProjectDir, created.ProjectsRoot)
}

func (s *Service) RegisterScanSource(ctx context.Context, st *store.SQLiteStore, root, projectDBPath string) (domain.Storage, error) {
	if st == nil {
		return domain.Storage{}, errors.New("project: store is required")
	}
	existing, err := st.ListStorages(ctx)
	if err != nil {
		return domain.Storage{}, fmt.Errorf("project: list existing storages: %w", err)
	}
	resolved, err := s.ValidateScanSource(root, existing, projectDBPath)
	if err != nil {
		return domain.Storage{}, err
	}
	storage := domain.Storage{ID: GenerateStorageID(resolved), RootPath: resolved, Kind: "filesystem", CreatedAt: time.Now().UTC()}
	if err := st.RegisterStorage(ctx, storage); err != nil {
		return domain.Storage{}, fmt.Errorf("project: register scan source: %w", err)
	}
	return storage, nil
}

func (s *Service) ValidateScanSource(root string, existing []domain.Storage, projectDBPath string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", errors.New("project: scan source root is required")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("project: resolve scan source: %w", err)
	}
	info, err := os.Lstat(absRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("project: scan source does not exist: %s", absRoot)
		}
		return "", fmt.Errorf("project: inspect scan source: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("project: scan source must not be a symbolic link: %s", absRoot)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project: scan source must be a directory: %s", absRoot)
	}
	if info.Mode().Perm()&0o444 == 0 || info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("project: scan source is not readable and searchable: %s", absRoot)
	}
	real, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", fmt.Errorf("project: resolve scan source symlinks: %w", err)
	}
	f, err := os.Open(real)
	if err != nil {
		return "", fmt.Errorf("project: scan source is not readable: %w", err)
	}
	_ = f.Close()

	if ndgRoot, baseErr := s.supportBase(); baseErr == nil {
		if ndgReal, evalErr := filepath.EvalSymlinks(ndgRoot); evalErr == nil {
			ndgRoot = ndgReal
		}
		if IsWithin(real, ndgRoot) {
			return "", errors.New("project: scan source must not be inside the NDG application support directory")
		}
	}
	if err := RejectScanRootContainingProjectFiles(real, projectDBPath); err != nil {
		return "", err
	}
	for _, item := range existing {
		existingReal := item.RootPath
		if r, evalErr := filepath.EvalSymlinks(item.RootPath); evalErr == nil {
			existingReal = r
		}
		switch {
		case existingReal == real:
			return "", fmt.Errorf("project: scan source already registered: %s", item.RootPath)
		case IsWithin(real, existingReal):
			return "", fmt.Errorf("project: scan source is inside an already registered source: %s", item.RootPath)
		case IsWithin(existingReal, real):
			return "", fmt.Errorf("project: scan source contains an already registered source: %s", item.RootPath)
		}
	}
	return real, nil
}

func RejectScanRootContainingProjectFiles(root, projectDBPath string) error {
	if strings.TrimSpace(root) == "" || strings.TrimSpace(projectDBPath) == "" {
		return nil
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		realRoot, err = filepath.Abs(root)
		if err != nil {
			return fmt.Errorf("project: resolve scan root: %w", err)
		}
	}
	realDB, err := filepath.EvalSymlinks(projectDBPath)
	if err != nil {
		realDB, err = filepath.Abs(projectDBPath)
		if err != nil {
			return fmt.Errorf("project: resolve project database: %w", err)
		}
	}
	if IsWithin(realDB, realRoot) {
		return errors.New("project: scan source must not contain the current project database or its WAL/SHM files")
	}
	return nil
}

func IsWithin(path, parent string) bool {
	p, par := filepath.Clean(path), filepath.Clean(parent)
	return p == par || strings.HasPrefix(p, par+string(filepath.Separator))
}

func GenerateStorageID(root string) string {
	h := sha256.Sum256([]byte(filepath.Clean(root)))
	return "src_" + hex.EncodeToString(h[:6])
}

func (s *Service) ResolveIdentity(dbPath string) (projectID, name, jobScope string) {
	name = s.DisplayName(dbPath)
	if meta, ok := s.ReadMeta(filepath.Dir(dbPath)); ok && strings.TrimSpace(meta.ProjectID) != "" {
		return meta.ProjectID, name, meta.ProjectID
	}
	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		absPath = filepath.Clean(dbPath)
	}
	h := sha256.Sum256([]byte(absPath))
	return "legacy_" + hex.EncodeToString(h[:8]), name, absPath
}

func (s *Service) DisplayName(dbPath string) string {
	if meta, ok := s.ReadMeta(filepath.Dir(dbPath)); ok && strings.TrimSpace(meta.Name) != "" {
		return meta.Name
	}
	parent := filepath.Base(filepath.Dir(dbPath))
	if parent != "" && parent != "." && parent != string(filepath.Separator) {
		return parent
	}
	return strings.TrimSuffix(filepath.Base(dbPath), filepath.Ext(dbPath))
}

func (s *Service) ReadMeta(projectDir string) (ProjectMeta, bool) {
	data, err := os.ReadFile(filepath.Join(projectDir, ProjectMetaFileName))
	if err != nil {
		return ProjectMeta{}, false
	}
	var meta ProjectMeta
	if json.Unmarshal(data, &meta) != nil {
		return ProjectMeta{}, false
	}
	return meta, true
}

func (s *Service) writeProjectMeta(projectDir string, meta ProjectMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("project: marshal project meta: %w", err)
	}
	return atomicWriteFile(filepath.Join(projectDir, ProjectMetaFileName), data)
}

func (s *Service) RecentPath() (string, error) {
	base, err := s.supportBase()
	if err != nil {
		return "", err
	}
	if err := privatefs.SecureDirectory(base); err != nil {
		return "", fmt.Errorf("project: secure app support dir: %w", err)
	}
	return filepath.Join(base, RecentFileName), nil
}

func (s *Service) ListRecent() ([]RecentEntry, error) {
	p, err := s.RecentPath()
	if err != nil {
		return nil, err
	}
	unlock, err := lockFile(p + ".lock")
	if err != nil {
		return nil, err
	}
	defer unlock()
	return readRecentManifest(p)
}

func (s *Service) RecordRecent(name, dbPath string) error {
	p, err := s.RecentPath()
	if err != nil {
		return err
	}
	unlock, err := lockFile(p + ".lock")
	if err != nil {
		return err
	}
	defer unlock()
	entries, err := readRecentManifest(p)
	if err != nil {
		return err
	}
	out := make([]RecentEntry, 0, len(entries)+1)
	out = append(out, RecentEntry{Name: name, Path: dbPath, OpenedAt: time.Now().UTC()})
	for _, entry := range entries {
		if entry.Path != dbPath {
			out = append(out, entry)
		}
	}
	if len(out) > recentMaxEntries {
		out = out[:recentMaxEntries]
	}
	return writeRecentManifest(p, out)
}

// ReplaceRecent is primarily useful for migration and deterministic tests. It
// shares the same inter-process lock and atomic writer as RecordRecent.
func (s *Service) ReplaceRecent(entries []RecentEntry) error {
	p, err := s.RecentPath()
	if err != nil {
		return err
	}
	unlock, err := lockFile(p + ".lock")
	if err != nil {
		return err
	}
	defer unlock()
	return writeRecentManifest(p, entries)
}

func readRecentManifest(path string) ([]RecentEntry, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("project: read recent manifest: %w", err)
	}
	var entries []RecentEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		_ = os.Rename(path, fmt.Sprintf("%s.corrupt-%d", path, time.Now().UnixNano()))
		return nil, nil
	}
	return entries, nil
}

func writeRecentManifest(path string, entries []RecentEntry) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("project: marshal recent manifest: %w", err)
	}
	return atomicWriteFile(path, data)
}

// atomicWriteFile uses a unique same-directory temp file, eliminating the
// fixed .tmp collision between processes while preserving atomic rename.
func atomicWriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := privatefs.EnsureDirectory(dir); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("project: create temp file: %w", err)
	}
	tmp := f.Name()
	committed := false
	defer func() {
		_ = f.Close()
		if !committed {
			_ = os.Remove(tmp)
		}
	}()
	if err := f.Chmod(privatefs.FileMode); err != nil {
		return fmt.Errorf("project: secure temp file: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("project: write temp file: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("project: sync temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("project: close temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("project: rename temp file: %w", err)
	}
	committed = true
	if err := privatefs.SecureFile(path); err != nil {
		return err
	}
	dirHandle, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("project: open parent directory for sync: %w", err)
	}
	defer dirHandle.Close()
	if err := dirHandle.Sync(); err != nil {
		return fmt.Errorf("project: sync parent directory: %w", err)
	}
	return nil
}

var (
	nameSep      = regexp.MustCompile(`[\s_]+`)
	nameStrip    = regexp.MustCompile(`[^\pL\pN-]+`)
	nameCollapse = regexp.MustCompile(`-{2,}`)
)

func NormalizeNameID(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = nameSep.ReplaceAllString(s, "-")
	s = nameStrip.ReplaceAllString(s, "")
	s = nameCollapse.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// claimProjectDirectory combines name selection with atomic mkdir. Concurrent
// instances can observe collisions, but only one can claim each candidate.
func claimProjectDirectory(projectsRoot, name string) (string, string, error) {
	base := NormalizeNameID(name)
	if base == "" {
		base = "project-" + time.Now().Format("20060102-150405")
	}
	for i := 1; i <= 1000; i++ {
		id := base
		if i > 1 {
			id = fmt.Sprintf("%s-%d", base, i)
		}
		dir := filepath.Join(projectsRoot, id)
		err := os.Mkdir(dir, privatefs.DirectoryMode)
		if err == nil {
			return id, dir, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", "", fmt.Errorf("project: claim project directory: %w", err)
		}
	}
	return "", "", errors.New("project: unable to claim a unique project directory")
}

func removeClaimedProjectDir(projectDir, projectsRoot string) error {
	absDir, err := filepath.Abs(projectDir)
	if err != nil {
		return err
	}
	absRoot, err := filepath.Abs(projectsRoot)
	if err != nil {
		return err
	}
	if absDir == absRoot || !IsWithin(absDir, absRoot) {
		return fmt.Errorf("project: refuse to remove dir outside projects root: %s", absDir)
	}
	info, err := os.Lstat(absDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("project: refuse to remove non-directory project path: %s", absDir)
	}
	return os.RemoveAll(absDir)
}
