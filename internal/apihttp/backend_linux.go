//go:build linux

package apihttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/FNB2026/nas-data-governance/internal/app"
	"github.com/FNB2026/nas-data-governance/internal/fingerprint"
	"github.com/FNB2026/nas-data-governance/internal/jobs"
	"github.com/FNB2026/nas-data-governance/internal/query"
	"github.com/FNB2026/nas-data-governance/internal/scanner"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

type binding struct {
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
}
type source struct {
	alias, path  string
	fd           *os.File
	info         os.FileInfo
	st           *store.SQLiteStore
	scan         *app.ScanService
	manager      *jobs.JobManager
	runner       *app.ScanJobRunner
	job          string
	running      bool
	observations sync.Map
}
type LocalBackend struct {
	mu      sync.Mutex
	sources map[string]*source
	lock    *os.File
	closing bool
	wg      sync.WaitGroup
}

// openAbsolute rejects symlinks in every configured root component. Mounts at
// the configured root are allowed, but child reads use openat2 NO_XDEV.
func openAbsolute(path string) (*os.File, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == "/" {
		return nil, ErrDenied
	}
	fd, e := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, ErrDenied
	}
	for _, part := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		next, e := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		_ = unix.Close(fd)
		if e != nil {
			return nil, ErrDenied
		}
		fd = next
	}
	return os.NewFile(uintptr(fd), "approved-root"), nil
}
func overlaps(a, b string) bool {
	r, e := filepath.Rel(a, b)
	return e == nil && (r == "." || r != ".." && !strings.HasPrefix(r, "../"))
}
func OpenLocal(state string, roots map[string]string) (*LocalBackend, error) {
	if len(roots) == 0 || len(roots) > 16 {
		return nil, ErrDenied
	}
	if !filepath.IsAbs(state) || filepath.Clean(state) != state || state == "/" {
		return nil, ErrDenied
	}
	dir, e := openAbsolute(state)
	if e != nil {
		return nil, e
	}
	defer dir.Close()
	info, e := dir.Stat()
	if e != nil || info.Mode().Perm() != 0700 || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Geteuid()) {
		return nil, ErrDenied
	}
	var fsinfo unix.Statfs_t
	if unix.Fstatfs(int(dir.Fd()), &fsinfo) != nil {
		return nil, ErrDenied
	}
	switch uint64(fsinfo.Type) {
	case 0xef53, 0x58465342, 0x9123683e, 0x01021994, 0x794c7630, 0x2fc12fc1:
	default:
		return nil, ErrDenied
	}
	fd, e := unix.Openat(int(dir.Fd()), "server.lock", unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if e != nil {
		return nil, ErrDenied
	}
	lock := os.NewFile(uintptr(fd), "state-lock")
	li, e := lock.Stat()
	if e != nil || !li.Mode().IsRegular() || li.Mode().Perm() != 0600 || li.Sys().(*syscall.Stat_t).Uid != uint32(os.Geteuid()) || li.Sys().(*syscall.Stat_t).Nlink != 1 || unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB) != nil {
		lock.Close()
		return nil, ErrDenied
	}
	b := &LocalBackend{sources: map[string]*source{}, lock: lock}
	fail := func() (*LocalBackend, error) { _ = b.Close(); return nil, ErrDenied }
	for alias, path := range roots {
		if !validAlias(alias) || overlaps(path, state) || overlaps(state, path) {
			return fail()
		}
		root, e := openAbsolute(path)
		if e != nil {
			return fail()
		}
		ri, e := root.Stat()
		if e != nil {
			root.Close()
			return fail()
		}
		s := &source{alias: alias, path: path, fd: root, info: ri}
		profile, probeErr := scanner.ProbeSource(context.Background(), path)
		if probeErr != nil || profile.Network || profile.FilesystemType == "unknown" {
			root.Close()
			return fail()
		}
		b.sources[alias] = s
		// Refuse duplicate/overlapping roots, including alternate inode bindings.
		for other, x := range b.sources {
			if other != alias && (overlaps(path, x.path) || overlaps(x.path, path) || os.SameFile(ri, x.info)) {
				return fail()
			}
		}
		// Require Linux openat2 support before any scanning/database mutation.
		test, e := s.open(".", unix.O_DIRECTORY)
		if e != nil {
			return fail()
		}
		test.Close()
		sub := filepath.Join(state, alias)
		if e = unix.Mkdirat(int(dir.Fd()), alias, 0700); e != nil && !errors.Is(e, unix.EEXIST) {
			return fail()
		}
		subdir, e := openAbsolute(sub)
		if e != nil {
			return fail()
		}
		si, e := subdir.Stat()
		subdir.Close()
		if e != nil || si.Mode().Perm() != 0700 || si.Sys().(*syscall.Stat_t).Uid != uint32(os.Geteuid()) {
			return fail()
		}
		sys := ri.Sys().(*syscall.Stat_t)
		ident := binding{uint64(sys.Dev), uint64(sys.Ino)}
		bindpath := filepath.Join(sub, "source-binding.json")
		if e = checkPrivateFile(bindpath, true); e != nil {
			return fail()
		}
		data, e := os.ReadFile(bindpath)
		if e == nil {
			var saved binding
			if json.Unmarshal(data, &saved) != nil || saved != ident {
				return fail()
			}
		} else if errors.Is(e, os.ErrNotExist) {
			data, _ = json.Marshal(ident)
			f, e := os.OpenFile(bindpath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if e != nil {
				return fail()
			}
			_, e = f.Write(data)
			if e == nil {
				e = f.Sync()
			}
			ce := f.Close()
			if e != nil || ce != nil {
				return fail()
			}
		} else {
			return fail()
		}
		for _, member := range []string{"governance.db", "governance.db-wal", "governance.db-shm"} {
			if checkPrivateFile(filepath.Join(sub, member), true) != nil {
				return fail()
			}
		}
		s.st, e = store.Open(context.Background(), filepath.Join(sub, "governance.db"))
		if e != nil {
			return fail()
		}
		existing, e := s.st.ListStorages(context.Background())
		if e != nil {
			return fail()
		}
		for _, x := range existing {
			if x.ID != alias || x.RootPath != path {
				return fail()
			}
		}
		s.scan = app.NewScanServiceWithHashFunc(s.st, s.hash(false), s.hash(true))
		s.manager = jobs.New(s.st)
		if _, e = s.manager.Recover(context.Background()); e != nil {
			return fail()
		}
		s.runner = app.NewScanJobRunner(s.scan, s.manager)
	}
	return b, nil
}
func (s *source) check(context.Context) error {
	current, e := openAbsolute(s.path)
	if e != nil {
		return ErrDenied
	}
	defer current.Close()
	info, e := current.Stat()
	if e != nil || !os.SameFile(s.info, info) {
		return ErrDenied
	}
	return nil
}
func (s *source) open(rel string, flags int) (*os.File, error) {
	fd, e := unix.Openat2(int(s.fd.Fd()), rel, &unix.OpenHow{Flags: uint64(unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NONBLOCK | flags), Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV})
	if e != nil {
		return nil, e
	}
	return os.NewFile(uintptr(fd), "approved-source-member"), nil
}
func (s *source) rel(path string) (string, error) {
	rel, e := filepath.Rel(s.path, path)
	if e != nil || !filepath.IsLocal(rel) {
		return "", ErrDenied
	}
	return rel, nil
}
func (s *source) readDir(path string) ([]os.DirEntry, error) {
	if e := s.check(context.Background()); e != nil {
		return nil, e
	}
	rel, e := s.rel(path)
	if e != nil {
		return nil, e
	}
	f, e := s.open(rel, unix.O_DIRECTORY)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	infos, e := f.Readdir(-1)
	if e != nil {
		return nil, e
	}
	entries := make([]os.DirEntry, len(infos))
	for i, info := range infos {
		entries[i] = fs.FileInfoToDirEntry(info)
		s.observations.Store(filepath.Join(path, info.Name()), info)
	}
	return entries, nil
}
func (s *source) hash(full bool) app.HashFunc {
	return func(path string, size int64) (string, error) {
		if e := s.check(context.Background()); e != nil {
			return "", e
		}
		rel, e := s.rel(path)
		if e != nil {
			return "", e
		}
		f, e := s.open(rel, 0)
		if e != nil {
			return "", e
		}
		defer f.Close()
		before, e := f.Stat()
		expected, ok := s.observations.Load(path)
		if e != nil || !ok || !before.Mode().IsRegular() || before.Size() != size || !os.SameFile(expected.(os.FileInfo), before) || !expected.(os.FileInfo).ModTime().Equal(before.ModTime()) {
			return "", ErrDenied
		}
		var h string
		if full {
			h, e = fingerprint.FullReader(f)
		} else {
			h, e = fingerprint.QuickReader(f, size)
		}
		if e != nil {
			return "", e
		}
		after, e := f.Stat()
		if e != nil || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
			return "", ErrDenied
		}
		current, e := s.open(rel, 0)
		if e != nil {
			return "", e
		}
		defer current.Close()
		ci, e := current.Stat()
		if e != nil || !os.SameFile(before, ci) || ci.Size() != before.Size() || !ci.ModTime().Equal(before.ModTime()) || s.check(context.Background()) != nil {
			return "", ErrDenied
		}
		return h, nil
	}
}
func (b *LocalBackend) Sources() []Source {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := []Source{}
	for a := range b.sources {
		out = append(out, Source{a})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Alias < out[j].Alias })
	return out
}
func (b *LocalBackend) Start(alias string, resume bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.sources[alias]
	if b.closing || s == nil {
		return ErrDenied
	}
	for _, x := range b.sources {
		if x.running {
			return ErrDenied
		}
	}
	ctx := context.Background()
	if s.check(ctx) != nil {
		return ErrDenied
	}
	plans, e := s.st.ListExecutingPlans(ctx)
	if e != nil || len(plans) > 0 {
		return ErrDenied
	}
	restores, e := s.st.ListPendingRestores(ctx)
	if e != nil || len(restores) > 0 {
		return ErrDenied
	}
	purges, e := s.st.ListRecoverablePurges(ctx)
	if e != nil || len(purges) > 0 {
		return ErrDenied
	}
	if resume {
		cp, e := s.st.LastCheckpoint(ctx, alias)
		if e != nil || cp.Status == "completed" {
			return ErrDenied
		}
	}
	profile, e := scanner.ProbeSource(ctx, s.path)
	if e != nil {
		return ErrDenied
	}
	s.running = true
	b.wg.Add(1)
	job, e := s.runner.StartScanJobWithCleanup(ctx, alias, app.ScanInput{Root: s.path, StorageID: alias, Resume: resume, Workers: 1, HashAttempts: 1, NetworkSource: profile.Network, AccessCheck: s.check, ReadDirectory: s.readDir}, func() { b.mu.Lock(); s.running = false; b.mu.Unlock(); b.wg.Done() })
	if e != nil {
		s.running = false
		b.wg.Done()
		return ErrDenied
	}
	s.job = job
	return nil
}
func (b *LocalBackend) Cancel(alias string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.sources[alias]
	if s == nil || !s.running {
		return ErrDenied
	}
	if e := s.manager.RequestCancel(context.Background(), s.job); e != nil {
		return ErrDenied
	}
	return nil
}
func (b *LocalBackend) Snapshot(alias string) (Snapshot, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.sources[alias]
	if s == nil || b.closing {
		return Snapshot{}, ErrDenied
	}
	ctx := context.Background()
	v := Snapshot{State: "IDLE", Groups: []Group{}}
	runs, e := s.st.ListJobsByProject(ctx, alias, 1)
	if e != nil {
		return v, ErrDenied
	}
	if len(runs) > 0 {
		j := runs[0]
		v.State = string(j.State)
		v.Stage = string(j.Stage)
		v.Discovered = j.Progress.Discovered
		v.Processed = j.Progress.Processed
		v.Failed = j.Progress.Failed
	}
	if s.running {
		p := s.scan.Progress()
		v.Discovered = p.Discovered
		v.Processed = p.Processed
		v.Failed = p.Failed
	}
	if cp, e := s.st.LastCheckpoint(ctx, alias); e == nil {
		v.Checkpoint = cp.Status
		v.Scanned = cp.ScannedCount
	}
	summary, e := s.st.ScanSummary(ctx, alias)
	if e != nil {
		return v, ErrDenied
	}
	v.Active = summary.Active
	v.Missing = summary.Missing
	v.Unavailable = summary.Unavailable
	v.Quick = summary.Quick
	v.Full = summary.Full
	page, e := app.NewDuplicateServiceWithReader(s.st).ListGroups(ctx, query.GroupQuery{StorageID: alias, PageSize: 200})
	if e != nil {
		return v, ErrDenied
	}
	for i, g := range page.Groups {
		v.Groups = append(v.Groups, Group{fmt.Sprintf("group_%d", i+1), g.PathCount, g.Size})
	}
	return v, nil
}
func (b *LocalBackend) Close() error {
	b.mu.Lock()
	if b.closing {
		b.mu.Unlock()
		return ErrDenied
	}
	b.closing = true
	for _, s := range b.sources {
		if s.running {
			_ = s.manager.RequestCancel(context.Background(), s.job)
		}
	}
	b.mu.Unlock()
	b.wg.Wait()
	var result error
	for _, s := range b.sources {
		if s.st != nil {
			if e := s.st.Close(); e != nil {
				result = ErrDenied
			}
		}
		_ = s.fd.Close()
	}
	if b.lock != nil {
		_ = b.lock.Close()
	}
	return result
}

// Retain conservative native I/O stop semantics: Close waits for outstanding
// source reads; callers must not release ownership while a read remains active.

func checkPrivateFile(path string, missing bool) error {
	info, e := os.Lstat(path)
	if missing && errors.Is(e, os.ErrNotExist) {
		return nil
	}
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return ErrDenied
	}
	s, ok := info.Sys().(*syscall.Stat_t)
	if !ok || s.Uid != uint32(os.Geteuid()) || s.Nlink != 1 {
		return ErrDenied
	}
	return nil
}
