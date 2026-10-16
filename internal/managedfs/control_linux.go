//go:build linux

package managedfs

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
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

// Initialize is the only bootstrap operation. It creates exactly one NEW root
// under a private existing parent. Existing or partial roots are never adopted,
// chmodded, migrated, repaired or deleted. No source paths are accepted.
func Initialize(ctx context.Context, path string) (RootProof, error) {
	var empty RootProof
	if ctx.Err() != nil {
		return empty, ErrCancelled
	}
	if os.Geteuid() == 0 || os.Getegid() == 0 || !filepath.IsAbs(path) || filepath.Clean(path) != path || path == "/" || strings.ContainsAny(path, "?#%") {
		return empty, ErrRejected
	}
	parent, err := absolute(filepath.Dir(path))
	if err != nil {
		return empty, err
	}
	defer unix.Close(parent)
	if _, err = directory(parent, true); err != nil {
		return empty, err
	}
	rootID, err := randomID()
	if err != nil {
		return empty, err
	}
	storeID, err := randomID()
	if err != nil || unix.Mkdirat(parent, filepath.Base(path), 0700) != nil {
		return empty, ErrRejected
	}
	fd, err := rooted(parent, filepath.Base(path), unix.O_RDONLY|unix.O_DIRECTORY)
	if err != nil {
		return empty, err
	}
	defer unix.Close(fd)
	id, err := directory(fd, true)
	if err != nil {
		return empty, err
	}
	var fs unix.Statfs_t
	if unix.Fstatfs(fd, &fs) != nil || !(uint64(fs.Type) == 0xef53 || uint64(fs.Type) == 0x9123683e || uint64(fs.Type) == 0x58465342) {
		return empty, ErrUnsupported
	}
	proof := RootProof{rootID, storeID, id.device, id.inode}
	for _, name := range []string{"objects", "staging"} {
		if unix.Mkdirat(fd, name, 0700) != nil {
			return empty, ErrRejected
		}
		child, err := rooted(fd, name, unix.O_RDONLY|unix.O_DIRECTORY)
		if err != nil {
			return empty, err
		}
		_, check := directory(child, true)
		syncErr := unix.Fsync(child)
		_ = unix.Close(child)
		if check != nil || syncErr != nil {
			return empty, ErrRejected
		}
	}
	for _, name := range []string{"root.json", "writer.lock", "catalog.sqlite"} {
		n, err := createAt(fd, name)
		if err != nil {
			return empty, err
		}
		f := os.NewFile(uintptr(n), "bootstrap private file")
		if name == "root.json" {
			raw := encoded(struct {
				Version int    `json:"version"`
				RootID  string `json:"root_id"`
				StoreID string `json:"store_id"`
			}{1, rootID, storeID})
			if count, err := f.Write(raw); err != nil || count != len(raw) {
				_ = f.Close()
				return empty, ErrRejected
			}
		}
		syncErr := f.Sync()
		closeErr := f.Close()
		if syncErr != nil || closeErr != nil {
			return empty, ErrRejected
		}
	}
	lock, err := rooted(fd, "writer.lock", unix.O_RDWR)
	if err != nil {
		return empty, err
	}
	defer unix.Close(lock)
	if unix.Flock(lock, unix.LOCK_EX|unix.LOCK_NB) != nil || unix.Fsync(fd) != nil || unix.Fsync(parent) != nil {
		return empty, ErrRejected
	}
	db, err := openCatalog(ctx, path, true)
	if err != nil {
		return empty, err
	}
	closed := false
	defer func() {
		if !closed {
			_ = db.Close()
		}
	}()
	if _, err = db.ExecContext(ctx, catalogSchema); err != nil {
		return empty, ErrRejected
	}
	if _, err = db.ExecContext(ctx, "INSERT INTO identity(id,root_id,store_id,epoch,pending) VALUES(1,?,?,0,'')", proof.RootID, proof.StoreID); err != nil {
		return empty, ErrRejected
	}
	closeDBErr := db.Close()
	closed = true
	if closeDBErr != nil {
		return empty, ErrRecoveryRequired
	}
	for _, name := range []string{"catalog.sqlite", "catalog.sqlite-wal", "catalog.sqlite-shm"} {
		if _, err := checkRegularAt(fd, name); err != nil {
			return empty, ErrRejected
		}
	}
	n, err := rooted(fd, "catalog.sqlite", unix.O_RDONLY)
	if err != nil {
		return empty, err
	}
	st, check := regular(n)
	_ = unix.Close(n)
	if check != nil || unix.Fsync(fd) != nil {
		return empty, ErrRejected
	}
	c := control{Version: 1, Root: proof, Catalog: identityOf(st)}
	c.Lock, err = checkRegularAt(fd, "writer.lock")
	if err != nil {
		return empty, err
	}
	for name, target := range map[string]*fileIdentity{"objects": &c.Objects, "staging": &c.Staging} {
		child, e := rooted(fd, name, unix.O_RDONLY|unix.O_DIRECTORY)
		if e != nil {
			return empty, e
		}
		identity, check := directory(child, true)
		_ = unix.Close(child)
		if check != nil {
			return empty, check
		}
		*target = fileIdentity{identity.device, identity.inode}
	}
	n, err = createAt(fd, "control.json")
	if err != nil {
		return empty, err
	}
	f := os.NewFile(uintptr(n), "bootstrap private control")
	raw := encoded(c)
	count, writeErr := f.Write(raw)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil || count != len(raw) || syncErr != nil || closeErr != nil || unix.Fsync(fd) != nil || unix.Fsync(parent) != nil {
		return empty, ErrRejected
	}
	return proof, nil
}
