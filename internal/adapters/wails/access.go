package wails

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/FNB2026/nas-data-governance/internal/fileaccess"
	projectsvc "github.com/FNB2026/nas-data-governance/internal/project"
	"github.com/FNB2026/nas-data-governance/internal/scanner"
)

// NewSandboxAPI is a composition-root constructor, not a bound API method.
// MAS builds also deny access in NewAPI until this repository is supplied.
func NewSandboxAPI(access *fileaccess.Manager, support string) *API {
	a := NewAPI()
	a.sandbox = true
	a.access = access
	a.support = support
	q := filepath.Join(support, "quarantine")
	if e := os.MkdirAll(q, 0700); e != nil || os.Chmod(q, 0700) != nil || !a.safeDatabaseDirectory(q) {
		panic("QUARANTINE_UNAVAILABLE")
	}
	a.defaultQuarantine = q
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
	return a.sourceLeaseContext(context.Background(), root)
}
func (a *API) sourceLeaseContext(ctx context.Context, root string) (*fileaccess.Lease, error) {
	if !a.sandbox {
		return &fileaccess.Lease{Root: root}, nil
	}
	if a.access == nil {
		return nil, fileaccess.ErrAuthorizationRequired
	}
	return a.access.Acquire(ctx, root)
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
	if a.sandbox && !sandboxWrites {
		return fileaccess.ErrWriteDisabled
	}
	return nil
}

// writeAccess holds security-scoped leases for the whole multi-step file
// operation. A lease is never dropped between copy, verify and source removal.
type writeAccess struct {
	roots      []string
	quarantine string
	leases     []*fileaccess.Lease
}

func (w *writeAccess) Release() {
	if w == nil {
		return
	}
	for i := len(w.leases) - 1; i >= 0; i-- {
		w.leases[i].Release()
	}
	w.leases = nil
}

func (a *API) withinSupport(path string) bool {
	if a.support == "" {
		return false
	}
	abs, e := filepath.Abs(path)
	if e != nil {
		return false
	}
	rel, e := filepath.Rel(a.support, abs)
	return e == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))))
}

func (a *API) acquireOneWriteRoot(root string) (string, *fileaccess.Lease, error) {
	if !a.sandbox {
		return root, nil, nil
	}
	if a.withinSupport(root) {
		// Container-owned roots still need every component checked for
		// symlinks: resolving a symlinked parent must not turn into an
		// unauthorized write outside the container.
		if !a.safeDatabaseDirectory(root) {
			return "", nil, fileaccess.ErrAuthorizationRequired
		}
		real, e := filepath.EvalSymlinks(root)
		if e != nil || !a.withinSupport(real) {
			return "", nil, fileaccess.ErrAuthorizationRequired
		}
		return real, nil, nil
	}
	lease, e := a.access.Acquire(context.Background(), root)
	if e != nil {
		return "", nil, e
	}
	return lease.Root, lease, nil
}

// acquireWriteAccess resolves every source root and the quarantine root before
// the executor is called. The sandbox cannot write through a path alone: each
// external root must still have a live read-write security-scoped lease.
func (a *API) acquireWriteAccess(sourceRoots []string, quarantineRoot string) (*writeAccess, error) {
	if !a.sandbox {
		return &writeAccess{roots: sourceRoots, quarantine: quarantineRoot}, nil
	}
	a.mu.RLock()
	access, closing, fallback := a.access, a.closing, a.defaultQuarantine
	a.mu.RUnlock()
	if access == nil || closing || fallback == "" {
		return nil, fileaccess.ErrAuthorizationRequired
	}
	q := strings.TrimSpace(quarantineRoot)
	if q == "" {
		q = fallback
	}
	if !a.withinSupport(q) && !filepath.IsAbs(q) {
		return nil, fileaccess.ErrAuthorizationRequired
	}
	// Never create the quarantine directory from an API-supplied path. The
	// container default is created by NewSandboxAPI; anything else must
	// already exist and pass the same symlink checks as a source root.
	wa := &writeAccess{quarantine: q}
	seen := make(map[string]bool, len(sourceRoots)+1)
	for _, root := range sourceRoots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		resolved, lease, e := a.acquireOneWriteRoot(root)
		if e != nil {
			wa.Release()
			return nil, e
		}
		if lease != nil {
			wa.leases = append(wa.leases, lease)
		}
		if !seen[resolved] {
			seen[resolved] = true
			wa.roots = append(wa.roots, resolved)
		}
	}
	if len(wa.roots) == 0 {
		wa.Release()
		return nil, fileaccess.ErrAuthorizationRequired
	}
	resolvedQ, qLease, e := a.acquireOneWriteRoot(q)
	if e != nil {
		wa.Release()
		return nil, e
	}
	if qLease != nil {
		wa.leases = append(wa.leases, qLease)
	}
	wa.quarantine = resolvedQ
	return wa, nil
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
