package wails

import (
	"context"
	"errors"
	"github.com/FNB2026/nas-data-governance/internal/fileaccess"
	projectsvc "github.com/FNB2026/nas-data-governance/internal/project"
	"github.com/FNB2026/nas-data-governance/internal/scanner"
	"os"
	"path/filepath"
	"strings"
)

// NewSandboxAPI is a composition-root constructor, not a bound API method.
// MAS builds also deny access in NewAPI until this repository is supplied.
func NewSandboxAPI(access *fileaccess.Manager, support string) *API {
	a := NewAPI()
	a.sandbox = true
	a.access = access
	a.support = support
	a.scanCtx, a.scanCancel = context.WithCancel(context.Background())
	a.projectSvc = projectsvc.NewService(projectsvc.Options{SupportBase: func() (string, error) {
		if !a.safeDatabaseDirectory(support) {
			return "", fileaccess.ErrAuthorizationRequired
		}
		return support, nil
	}})
	return a
}
func (a *API) sourceLease(root string) (*fileaccess.Lease, error) {
	if !a.sandbox {
		return &fileaccess.Lease{Root: root}, nil
	}
	if a.access == nil {
		return nil, fileaccess.ErrAuthorizationRequired
	}
	return a.access.Acquire(context.Background(), root)
}
func (a *API) safeDatabaseDirectory(path string) bool {
	abs, e := filepath.Abs(path)
	if e != nil || a.support == "" {
		return false
	}
	rel, e := filepath.Rel(a.support, abs)
	if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	for p := abs; ; p = filepath.Dir(p) {
		s, e := os.Lstat(p)
		if e != nil || !s.IsDir() || s.Mode()&os.ModeSymlink != 0 {
			return false
		}
		if p == a.support {
			return true
		}
		if filepath.Dir(p) == p {
			return false
		}
	}
}
func (a *API) authorizeDatabase(path string) error {
	if !a.sandbox {
		return nil
	}
	if filepath.Base(path) != "governance.db" || !strings.HasPrefix(filepath.Clean(path), filepath.Join(a.support, "projects")+string(filepath.Separator)) || !a.safeDatabaseDirectory(filepath.Dir(path)) {
		return fileaccess.ErrAuthorizationRequired
	}
	s, e := os.Lstat(path)
	if e != nil || !s.Mode().IsRegular() {
		return fileaccess.ErrAuthorizationRequired
	}
	return nil
}
func (a *API) denySourceWrites() error {
	if a.sandbox {
		return fileaccess.ErrWriteDisabled
	}
	return nil
}

// ShutdownSandboxAPI must run in OnBeforeClose while the main event loop is active.
// Cancel jobs, drain their cleanup, then close the repository and DB. Uncertain
// durable Journal states are never changed by regaining a source grant.
func ShutdownSandboxAPI(a *API) { a.shutdownAccess() }
func (a *API) shutdownAccess() {
	a.mu.Lock()
	a.closing = true
	if a.scanCancel != nil {
		a.scanCancel()
	}
	access := a.access
	a.mu.Unlock()
	if access != nil { // Cancel a panel before waiting for API calls to return.
		// Manager.Close waits for all existing leases without releasing them early.
		access.Close()
	}
	a.scanWG.Wait()
	_ = a.CloseProject()
}
func safeAccessError(e error) error {
	if errors.Is(e, fileaccess.ErrSourceUnavailable) || errors.Is(e, fileaccess.ErrNetworkPaused) || errors.Is(e, fileaccess.ErrAuthorizationRequired) {
		return e
	}
	return fileaccess.ErrAuthorizationRequired
}

func (a *API) probeSource(ctx context.Context, root string) (scanner.SourceProfile, error) {
	if !a.sandbox {
		return scanner.ProbeSource(ctx, root)
	}
	lease, e := a.sourceLease(root)
	if e != nil {
		return scanner.SourceProfile{}, e
	}
	p, e := scanner.ProbeSourceWithCleanup(ctx, root, lease.Release)
	if e != nil {
		var pe *scanner.ProbeError
		if errors.As(e, &pe) && pe.Code == "permission_denied" {
			return p, fileaccess.ErrAuthorizationRequired
		}
		if lease.Network {
			return p, fileaccess.ErrNetworkPaused
		}
		return p, fileaccess.ErrSourceUnavailable
	}
	return p, nil
}
