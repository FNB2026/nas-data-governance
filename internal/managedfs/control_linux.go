//go:build linux

package managedfs

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
	"modernc.org/sqlite"
)

type fileIdentity struct {
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
}

type intent struct {
	ID            string      `json:"id"`
	Kind          string      `json:"kind"`
	ObjectID      string      `json:"object_id"`
	Size          int64       `json:"size"`
	SHA256        string      `json:"sha256"`
	Authorization string      `json:"authorization_sha256"`
	Proof         ObjectProof `json:"proof"`
}

type control struct {
	Version int          `json:"version"`
	Root    RootProof    `json:"root"`
	Catalog fileIdentity `json:"catalog"`
	Lock    fileIdentity `json:"lock"`
	Objects fileIdentity `json:"objects"`
	Staging fileIdentity `json:"staging"`
	Epoch   int64        `json:"epoch"`
	Pending *intent      `json:"pending"`
}

func randomID() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", ErrRejected
	}
	return hex.EncodeToString(raw[:]), nil
}

func encoded(value any) []byte {
	raw, _ := json.Marshal(value) // All callers use fixed structs without fallible fields.
	return append(raw, '\n')
}

func identityOf(st unix.Statx_t) fileIdentity {
	return fileIdentity{unix.Mkdev(st.Dev_major, st.Dev_minor), st.Ino}
}

func readControl(root int, proof RootProof) (control, error) {
	var c control
	fd, err := rooted(root, "control.json", unix.O_RDONLY)
	if err != nil {
		return c, ErrRecoveryRequired
	}
	f := os.NewFile(uintptr(fd), "managed control")
	defer f.Close()
	st, err := regular(fd)
	if err != nil || st.Size > 8192 {
		return c, ErrRecoveryRequired
	}
	raw, err := io.ReadAll(io.LimitReader(f, 8193))
	if err != nil || json.Unmarshal(raw, &c) != nil || !bytes.Equal(raw, encoded(c)) || c.Version != 1 || c.Root != proof || c.Catalog.Inode == 0 || c.Lock.Inode == 0 || c.Objects.Inode == 0 || c.Staging.Inode == 0 || c.Epoch < 0 || c.Pending != nil {
		return c, ErrRecoveryRequired
	}
	return c, nil
}

func createAt(parent int, name string) (int, error) {
	fd, err := unix.Openat2(parent, name, &unix.OpenHow{
		Flags: unix.O_RDWR | unix.O_CREAT | unix.O_EXCL | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_NONBLOCK,
		Mode:  0600, Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV,
	})
	if err != nil {
		return -1, ErrRejected
	}
	if _, err = regular(fd); err != nil {
		_ = unix.Close(fd)
		return -1, err
	}
	return fd, nil
}

// openCatalog uses SQLite's ordinary Unix VFS only in the exclusive private
// namespace contract. It does not pretend /proc/self/fd survives SQLite's
// pathname canonicalization, nor claim atomic protection against privileged
// concurrent namespace mutation. Kernel admission happens before any SQLite open.
type catalog struct {
	*sql.Conn // Pinned for the entire session; never call the pool for operations.
	pool      *sql.DB
}

func (c *catalog) Close() error {
	err := c.Conn.Close()
	poolErr := c.pool.Close()
	if err != nil || poolErr != nil {
		return ErrRecoveryRequired
	}
	return nil
}

func openCatalog(ctx context.Context, path string, initialize bool) (*catalog, error) {
	if strings.ContainsAny(path, "?#%") {
		return nil, ErrRejected
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"/catalog.sqlite?mode=rw&_txlock=immediate")
	if err != nil {
		return nil, ErrRejected
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	good := false
	defer func() {
		if !good {
			_ = db.Close()
		}
	}()
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, ErrRejected
	}
	defer func() {
		if !good {
			_ = conn.Close()
		}
	}()
	// Persistence prevents Close from unlinking sidecars by a pathname which
	// might no longer name the pinned root. It never clears pending intents.
	if conn.Raw(func(value any) error {
		fc, ok := value.(sqlite.FileControl)
		if !ok {
			return ErrUnsupported
		}
		mode, err := fc.FileControlPersistWAL("main", 1)
		if err != nil || mode != 1 {
			return ErrUnsupported
		}
		mode, err = fc.FileControlPersistWAL("main", -1)
		if err != nil || mode != 1 {
			return ErrUnsupported
		}
		return nil
	}) != nil {
		return nil, ErrUnsupported
	}
	var mode string
	pragma := "PRAGMA journal_mode"
	if initialize {
		pragma += "=WAL"
	}
	if conn.QueryRowContext(ctx, pragma).Scan(&mode) != nil || mode != "wal" {
		return nil, ErrRejected
	}
	for _, p := range []struct {
		key   string
		value int
	}{{"synchronous", 2}, {"foreign_keys", 1}, {"trusted_schema", 0}, {"temp_store", 2}, {"busy_timeout", 0}, {"wal_autocheckpoint", 0}} {
		var got int
		if _, err := conn.ExecContext(ctx, "PRAGMA "+p.key+"="+strconv.Itoa(p.value)); err != nil || conn.QueryRowContext(ctx, "PRAGMA "+p.key).Scan(&got) != nil || got != p.value {
			return nil, ErrRejected
		}
	}
	good = true
	return &catalog{Conn: conn, pool: db}, nil
}

const bootstrapPartialName = "ndg-bootstrap-partial"

// Test-only, unexported. No environment, CLI or API fault or recovery switch.
var bootstrapFault func(string) error

// Kernel and descriptor failures are fixed categories, never raw errno. The
// unsupported-platform/kernel refusal and the uncertain-catalog refusal must
// survive so callers can still skip or require recovery.
func classify(err error) error {
	if errors.Is(err, ErrUnsupported) || errors.Is(err, ErrCancelled) || errors.Is(err, ErrRecoveryRequired) {
		return err
	}
	return ErrRejected
}

func bootstrapStep(ctx context.Context, label string, action func() error) error {
	if ctx.Err() != nil {
		return ErrCancelled
	}
	if bootstrapFault != nil && bootstrapFault(label+".before") != nil {
		return ErrRejected
	}
	if err := action(); err != nil {
		return classify(err)
	}
	if bootstrapFault != nil && bootstrapFault(label+".after") != nil {
		return ErrRejected
	}
	return nil
}

// Every descriptor is released exactly once, including when the injected fault
// fires on the edge that would otherwise have released it.
func bootstrapFile(ctx context.Context, parent int, label, name string, content []byte) error {
	fd := -1
	released := false
	defer func() {
		if !released && fd >= 0 {
			_ = unix.Close(fd)
		}
	}()
	if err := bootstrapStep(ctx, label+".create", func() error {
		var e error
		fd, e = createAt(parent, name)
		return e
	}); err != nil {
		return err
	}
	if len(content) > 0 {
		if err := bootstrapStep(ctx, label+".write", func() error {
			n, e := unix.Write(fd, content)
			if e != nil || n != len(content) {
				return ErrRejected
			}
			return nil
		}); err != nil {
			return err
		}
	}
	if err := bootstrapStep(ctx, label+".sync", func() error { return unix.Fsync(fd) }); err != nil {
		return err
	}
	return bootstrapStep(ctx, label+".close", func() error {
		released = true
		return unix.Close(fd)
	})
}

func bootstrapDirectory(ctx context.Context, parent int, name string) error {
	fd := -1
	released := false
	defer func() {
		if !released && fd >= 0 {
			_ = unix.Close(fd)
		}
	}()
	if err := bootstrapStep(ctx, name+".mkdir", func() error {
		return unix.Mkdirat(parent, name, 0700)
	}); err != nil {
		return err
	}
	if err := bootstrapStep(ctx, name+".open", func() error {
		var e error
		fd, e = rooted(parent, name, unix.O_RDONLY|unix.O_DIRECTORY)
		return e
	}); err != nil {
		return err
	}
	if err := bootstrapStep(ctx, name+".private", func() error {
		_, e := directory(fd, true)
		return e
	}); err != nil {
		return err
	}
	if err := bootstrapStep(ctx, name+".sync", func() error { return unix.Fsync(fd) }); err != nil {
		return err
	}
	return bootstrapStep(ctx, name+".close", func() error {
		released = true
		return unix.Close(fd)
	})
}

// Initialize is the only bootstrap operation. It creates exactly one NEW root
// under a private existing parent. Existing or partial roots are never adopted,
// chmodded, migrated, repaired or deleted. No source paths are accepted.
//
// The partial marker names every incomplete root before any other root content
// exists. It is removed only after the whole protocol is durable, so a crash
// can leave a usable root or a marked one, never a silent partial that the
// Reader would otherwise accept. Nothing clears it on failure: a failed
// bootstrap never becomes adoptable, and it never self-heals.
func Initialize(ctx context.Context, path string) (RootProof, error) {
	var empty RootProof
	if ctx.Err() != nil {
		return empty, ErrCancelled
	}
	if os.Geteuid() == 0 || os.Getegid() == 0 || !filepath.IsAbs(path) || filepath.Clean(path) != path || path == "/" || strings.ContainsAny(path, "?#%") {
		return empty, ErrRejected
	}
	parent := -1
	if err := bootstrapStep(ctx, "parent.resolve", func() error {
		var e error
		parent, e = absolute(filepath.Dir(path))
		return e
	}); err != nil {
		return empty, err
	}
	defer unix.Close(parent)
	if err := bootstrapStep(ctx, "parent.private", func() error {
		_, e := directory(parent, true)
		return e
	}); err != nil {
		return empty, err
	}
	if err := bootstrapStep(ctx, "parent.filesystem", func() error {
		_, e := writerFilesystem(parent)
		return e
	}); err != nil {
		return empty, err
	}
	rootID, err := randomID()
	if err != nil {
		return empty, err
	}
	storeID, err := randomID()
	if err != nil {
		return empty, err
	}
	if err := bootstrapStep(ctx, "root.mkdir", func() error {
		return unix.Mkdirat(parent, filepath.Base(path), 0700)
	}); err != nil {
		return empty, err
	}
	fd := -1
	if err := bootstrapStep(ctx, "root.open", func() error {
		var e error
		fd, e = rooted(parent, filepath.Base(path), unix.O_RDONLY|unix.O_DIRECTORY)
		return e
	}); err != nil {
		if fd >= 0 {
			_ = unix.Close(fd)
		}
		return empty, err
	}
	defer unix.Close(fd)
	if err := bootstrapFile(ctx, fd, "partial", bootstrapPartialName, nil); err != nil {
		return empty, err
	}
	id := directoryIdentity{}
	if err := bootstrapStep(ctx, "root.private", func() error {
		var e error
		id, e = directory(fd, true)
		return e
	}); err != nil {
		return empty, err
	}
	if err := bootstrapStep(ctx, "root.filesystem", func() error {
		_, e := writerFilesystem(fd)
		return e
	}); err != nil {
		return empty, err
	}
	proof := RootProof{rootID, storeID, id.device, id.inode}
	for _, name := range []string{"objects", "staging"} {
		if err := bootstrapDirectory(ctx, fd, name); err != nil {
			return empty, err
		}
	}
	raw := encoded(struct {
		Version int    `json:"version"`
		RootID  string `json:"root_id"`
		StoreID string `json:"store_id"`
	}{1, rootID, storeID})
	if err := bootstrapFile(ctx, fd, "rootjson", "root.json", raw); err != nil {
		return empty, err
	}
	if err := bootstrapFile(ctx, fd, "lockfile", "writer.lock", nil); err != nil {
		return empty, err
	}
	if err := bootstrapFile(ctx, fd, "dbfile", "catalog.sqlite", nil); err != nil {
		return empty, err
	}
	lock := -1
	if err := bootstrapStep(ctx, "lease.open", func() error {
		var e error
		lock, e = rooted(fd, "writer.lock", unix.O_RDWR)
		return e
	}); err != nil {
		if lock >= 0 {
			_ = unix.Close(lock)
		}
		return empty, err
	}
	defer unix.Close(lock)
	if err := bootstrapStep(ctx, "lease.flock", func() error {
		return unix.Flock(lock, unix.LOCK_EX|unix.LOCK_NB)
	}); err != nil {
		return empty, err
	}
	if err := bootstrapStep(ctx, "root.predirsync", func() error { return unix.Fsync(fd) }); err != nil {
		return empty, err
	}
	if err := bootstrapStep(ctx, "parent.predirsync", func() error { return unix.Fsync(parent) }); err != nil {
		return empty, err
	}
	db := (*catalog)(nil)
	dbOpen := false
	if err := bootstrapStep(ctx, "catalog.open", func() error {
		var e error
		db, e = openCatalog(ctx, path, true)
		if e == nil {
			dbOpen = true
		}
		return e
	}); err != nil {
		return empty, err
	}
	defer func() {
		if dbOpen {
			_ = db.Close()
			dbOpen = false
		}
	}()
	if err := bootstrapStep(ctx, "catalog.schema", func() error {
		_, e := db.ExecContext(ctx, catalogSchema)
		if e != nil {
			return ErrRejected
		}
		return nil
	}); err != nil {
		return empty, err
	}
	if err := bootstrapStep(ctx, "catalog.identity", func() error {
		_, e := db.ExecContext(ctx, "INSERT INTO identity(id,root_id,store_id,epoch,pending) VALUES(1,?,?,0,'')", proof.RootID, proof.StoreID)
		if e != nil {
			return ErrRejected
		}
		return nil
	}); err != nil {
		return empty, err
	}
	if err := bootstrapStep(ctx, "catalog.close", func() error {
		dbOpen = false
		return db.Close()
	}); err != nil {
		return empty, err
	}
	if err := bootstrapStep(ctx, "catalog.sidecars", func() error {
		for _, name := range []string{"catalog.sqlite", "catalog.sqlite-wal", "catalog.sqlite-shm"} {
			if _, e := checkRegularAt(fd, name); e != nil {
				return e
			}
		}
		return nil
	}); err != nil {
		return empty, err
	}
	var st unix.Statx_t
	if err := bootstrapStep(ctx, "catalog.stat", func() error {
		n, e := rooted(fd, "catalog.sqlite", unix.O_RDONLY)
		if e != nil {
			return e
		}
		st, e = regular(n)
		_ = unix.Close(n)
		return e
	}); err != nil {
		return empty, err
	}
	if err := bootstrapStep(ctx, "root.presync", func() error { return unix.Fsync(fd) }); err != nil {
		return empty, err
	}
	c := control{Version: 1, Root: proof, Catalog: identityOf(st)}
	if err := bootstrapStep(ctx, "control.lock", func() error {
		var e error
		c.Lock, e = checkRegularAt(fd, "writer.lock")
		return e
	}); err != nil {
		return empty, err
	}
	if err := bootstrapStep(ctx, "control.children", func() error {
		for _, name := range []string{"objects", "staging"} {
			child, e := rooted(fd, name, unix.O_RDONLY|unix.O_DIRECTORY)
			if e != nil {
				return e
			}
			identity, e := directory(child, true)
			_ = unix.Close(child)
			if e != nil {
				return e
			}
			target := &c.Objects
			if name == "staging" {
				target = &c.Staging
			}
			*target = fileIdentity{identity.device, identity.inode}
		}
		return nil
	}); err != nil {
		return empty, err
	}
	if err := bootstrapFile(ctx, fd, "control", "control.json", encoded(c)); err != nil {
		return empty, err
	}
	if err := bootstrapStep(ctx, "root.dirsync", func() error { return unix.Fsync(fd) }); err != nil {
		return empty, err
	}
	if err := bootstrapStep(ctx, "parent.dirsync", func() error { return unix.Fsync(parent) }); err != nil {
		return empty, err
	}
	if err := bootstrapStep(ctx, "partial.unlink", func() error {
		return unix.Unlinkat(fd, bootstrapPartialName, 0)
	}); err != nil {
		return empty, err
	}
	if err := bootstrapStep(ctx, "partial.dirsync", func() error { return unix.Fsync(fd) }); err != nil {
		return empty, err
	}
	return proof, nil
}
