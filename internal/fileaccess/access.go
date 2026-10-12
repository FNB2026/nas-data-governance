// Package fileaccess owns read-only OS grants. A persisted path is an identity,
// never an authority. Raw bookmark bytes stay in a private capability repository.
package fileaccess

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

var (
	ErrAuthorizationRequired = errors.New("AUTHORIZATION_REQUIRED")
	ErrSourceUnavailable     = errors.New("SOURCE_UNAVAILABLE")
	ErrNetworkPaused         = errors.New("NETWORK_PAUSED")
	ErrWriteDisabled         = errors.New("MAS_WRITE_DISABLED")
)

type Grant struct {
	Root     string
	Bookmark string
	Identity string
	Network  bool
}
type Native interface {
	Pick(context.Context) (Grant, func(), error)
	Resolve(context.Context, Grant) (Grant, func(), error)
}
type Manager struct {
	mu     sync.Mutex
	dir    string
	native Native
	closed bool
	wg     sync.WaitGroup
}
type Lease struct {
	Root    string
	Network bool
	once    sync.Once
	release func()
}

func (l *Lease) Release() {
	if l != nil {
		if l.release != nil {
			l.once.Do(l.release)
		}
	}
}
func New(dir string, n Native) (*Manager, error) {
	if n == nil {
		return nil, ErrAuthorizationRequired
	}
	abs, e := filepath.Abs(dir)
	if e != nil {
		return nil, ErrAuthorizationRequired
	}
	if e = os.MkdirAll(abs, 0700); e != nil {
		return nil, ErrAuthorizationRequired
	}
	if !safeDirectory(abs) {
		return nil, ErrAuthorizationRequired
	}
	if e = os.Chmod(abs, 0700); e != nil {
		return nil, ErrAuthorizationRequired
	}
	return &Manager{dir: abs, native: n}, nil
}
func safeDirectory(path string) bool {
	for p := path; ; p = filepath.Dir(p) {
		s, e := os.Lstat(p)
		if e != nil || !s.IsDir() || s.Mode()&os.ModeSymlink != 0 {
			return false
		}
		if filepath.Dir(p) == p {
			return true
		}
	}
}
func canonical(root string) (string, error) {
	abs, e := filepath.Abs(root)
	if e != nil {
		return "", ErrAuthorizationRequired
	}
	info, e := os.Lstat(abs)
	if e != nil {
		if os.IsPermission(e) {
			return "", ErrAuthorizationRequired
		}
		return "", ErrSourceUnavailable
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", ErrAuthorizationRequired
	}
	real, e := filepath.EvalSymlinks(abs)
	if e != nil {
		return "", ErrAuthorizationRequired
	}
	return real, nil
}
func (m *Manager) recordPath(root string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(root)))
	return filepath.Join(m.dir, hex.EncodeToString(sum[:])+".json")
}
func (m *Manager) Select(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || ctx.Err() != nil {
		return "", ErrAuthorizationRequired
	}
	g, stop, e := m.native.Pick(ctx)
	if stop != nil {
		defer stop()
	}
	if e != nil {
		return "", classify(e, false)
	}
	root, e := canonical(g.Root)
	if e != nil {
		return "", e
	}
	g.Root = root
	if g.Bookmark == "" || g.Identity == "" || stop == nil {
		return "", ErrAuthorizationRequired
	}
	if e = m.save(g); e != nil {
		return "", ErrAuthorizationRequired
	}
	return root, nil
}
func (m *Manager) save(g Grant) error {
	if !safeDirectory(m.dir) {
		return ErrAuthorizationRequired
	}
	data, e := json.Marshal(g)
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(m.dir, ".grant-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(data)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	if e = os.Rename(name, m.recordPath(g.Root)); e != nil {
		return e
	}
	d, e := os.Open(m.dir)
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func (m *Manager) load(root string) (Grant, error) {
	var g Grant
	if !safeDirectory(m.dir) {
		return g, ErrAuthorizationRequired
	}
	path := m.recordPath(root)
	info, e := os.Lstat(path)
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return g, ErrAuthorizationRequired
	}
	r, e := os.OpenRoot(m.dir)
	if e != nil {
		return g, ErrAuthorizationRequired
	}
	defer r.Close()
	f, e := r.Open(filepath.Base(path))
	if e != nil {
		return g, ErrAuthorizationRequired
	}
	defer f.Close()
	actual, e := f.Stat()
	if e != nil || !os.SameFile(info, actual) || actual.Size() > 1024*1024 {
		return g, ErrAuthorizationRequired
	}
	if json.NewDecoder(f).Decode(&g) != nil || g.Root != root || g.Bookmark == "" || g.Identity == "" {
		return Grant{}, ErrAuthorizationRequired
	}
	return g, nil
}
func classify(e error, network bool) error {
	if errors.Is(e, ErrSourceUnavailable) {
		if network {
			return ErrNetworkPaused
		}
		return ErrSourceUnavailable
	}
	if errors.Is(e, ErrNetworkPaused) {
		return ErrNetworkPaused
	}
	return ErrAuthorizationRequired
}
func (m *Manager) Acquire(ctx context.Context, root string) (*Lease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || ctx.Err() != nil {
		return nil, ErrAuthorizationRequired
	}
	// Lookup before touching the source: an arbitrary path must never probe it.
	key, e := filepath.Abs(root)
	if e != nil {
		return nil, ErrAuthorizationRequired
	}
	key = filepath.Clean(key)
	saved, e := m.load(key)
	if e != nil {
		return nil, ErrAuthorizationRequired
	}
	g, stop, e := m.native.Resolve(ctx, saved)
	if e != nil {
		if stop != nil {
			stop()
		}
		return nil, classify(e, saved.Network)
	}
	if stop == nil {
		return nil, ErrAuthorizationRequired
	}
	resolved, e := canonical(g.Root)
	if e != nil {
		stop()
		return nil, classify(e, saved.Network)
	}
	if resolved != saved.Root || g.Identity != saved.Identity || g.Network != saved.Network || g.Bookmark == "" {
		stop()
		return nil, ErrAuthorizationRequired
	}
	g.Root = resolved
	if e = m.save(g); e != nil {
		stop()
		return nil, ErrAuthorizationRequired
	}
	m.wg.Add(1)
	return &Lease{Root: resolved, Network: g.Network, release: func() { m.mu.Lock(); stop(); m.mu.Unlock(); m.wg.Done() }}, nil
}

// Close rejects new work, cancels a native panel without holding mu, and waits
// for all borrowers to release. It never tears authority away from active IO.
func (m *Manager) Close() {
	if c, ok := m.native.(interface{ CancelPicker() }); ok {
		c.CancelPicker()
	}
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	m.wg.Wait()
}
