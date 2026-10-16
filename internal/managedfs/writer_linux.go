//go:build linux

package managedfs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

type writer struct {
	gate            sync.Mutex
	closed, blocked bool
	r               *reader
	staging, lock   int
	stageID         directoryIdentity
	lockID          fileIdentity
	control         control
	db              *catalog
	// Test-only, unexported. No environment/CLI/API fault or recovery switch.
	fault func(string) error
}

func names(fd int) ([]string, error) {
	n, err := rooted(fd, ".", unix.O_RDONLY|unix.O_DIRECTORY)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(n), "managed private inventory")
	defer f.Close()
	list, err := f.Readdirnames(-1)
	if err != nil {
		return nil, ErrRejected
	}
	return list, nil
}

func checkRegularAt(parent int, name string) (fileIdentity, error) {
	fd, err := rooted(parent, name, unix.O_RDONLY)
	if err != nil {
		return fileIdentity{}, err
	}
	defer unix.Close(fd)
	st, err := regular(fd)
	if err != nil {
		return fileIdentity{}, err
	}
	return identityOf(st), nil
}

func (w *writer) bound() error {
	if w.r.bound() != nil {
		return ErrRecoveryRequired
	}
	stage, err := rooted(w.r.root, "staging", unix.O_RDONLY|unix.O_DIRECTORY)
	if err != nil {
		return ErrRecoveryRequired
	}
	id, check := directory(stage, true)
	_ = unix.Close(stage)
	if check != nil || id != w.stageID {
		return ErrRecoveryRequired
	}
	if id, err := checkRegularAt(w.r.root, "writer.lock"); err != nil || id != w.lockID {
		return ErrRecoveryRequired
	}
	if id, err := checkRegularAt(w.r.root, "catalog.sqlite"); err != nil || id != w.control.Catalog {
		return ErrRecoveryRequired
	}
	for _, name := range []string{"catalog.sqlite-wal", "catalog.sqlite-shm"} {
		var st unix.Stat_t
		err := unix.Fstatat(w.r.root, name, &st, unix.AT_SYMLINK_NOFOLLOW)
		if errors.Is(err, unix.ENOENT) {
			continue
		}
		if err != nil {
			return ErrRecoveryRequired
		}
		if _, err := checkRegularAt(w.r.root, name); err != nil {
			return ErrRecoveryRequired
		}
	}
	return nil
}

func (w *writer) cleanRoot() error {
	list, err := names(w.r.root)
	if err != nil {
		return ErrRecoveryRequired
	}
	allowed := map[string]bool{"root.json": true, "control.json": true, "objects": true, "staging": true, "writer.lock": true, "catalog.sqlite": true, "catalog.sqlite-wal": true, "catalog.sqlite-shm": true}
	for _, name := range list {
		if !allowed[name] {
			return ErrRecoveryRequired
		}
	}
	list, err = names(w.staging)
	if err != nil || len(list) != 0 {
		return ErrRecoveryRequired
	}
	return nil
}

func (w *writer) cleanInventory(ctx context.Context) error {
	if w.cleanRoot() != nil {
		return ErrRecoveryRequired
	}
	list, err := names(w.r.objects)
	if err != nil {
		return ErrRecoveryRequired
	}
	rows, err := w.db.QueryContext(ctx, "SELECT id FROM objects WHERE state='SEALED'")
	if err != nil {
		return ErrRecoveryRequired
	}
	expected := map[string]bool{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil || !opaque(id) {
			_ = rows.Close()
			return ErrRecoveryRequired
		}
		expected[id] = true
	}
	rowErr := rows.Err()
	closeErr := rows.Close()
	if rowErr != nil || closeErr != nil || len(list) != len(expected) {
		return ErrRecoveryRequired
	}
	for _, name := range list {
		if !expected[name] {
			return ErrRecoveryRequired
		}
	}
	return nil
}

func (w *writer) ready(ctx context.Context) error {
	if w.closed {
		return ErrClosed
	}
	if w.blocked {
		return ErrRecoveryRequired
	}
	if ctx.Err() != nil {
		return ErrCancelled
	}
	c, err := readControl(w.r.root, w.r.proof)
	if err != nil || c != w.control || w.bound() != nil || w.catalogReady(ctx) != nil || w.cleanInventory(ctx) != nil {
		w.blocked = true
		return ErrRecoveryRequired
	}
	return nil
}

func OpenWriter(ctx context.Context, path string, proof RootProof) (Writer, error) {
	if ctx.Err() != nil {
		return nil, ErrCancelled
	}
	r, err := OpenReader(path, proof)
	if err != nil {
		return nil, err
	}
	w := &writer{r: r.(*reader), staging: -1, lock: -1}
	keep := false
	defer func() {
		if !keep {
			_ = w.Close()
		}
	}()
	w.control, err = readControl(w.r.root, proof)
	if err != nil {
		return nil, err
	}
	w.lock, err = rooted(w.r.root, "writer.lock", unix.O_RDWR)
	if err != nil {
		return nil, err
	}
	st, err := regular(w.lock)
	if err != nil || st.Size != 0 || identityOf(st) != w.control.Lock || (fileIdentity{w.r.objectsID.device, w.r.objectsID.inode}) != w.control.Objects {
		return nil, ErrRejected
	}
	w.lockID = identityOf(st)
	if unix.Flock(w.lock, unix.LOCK_EX|unix.LOCK_NB) != nil {
		return nil, ErrBusy
	}
	w.staging, err = rooted(w.r.root, "staging", unix.O_RDONLY|unix.O_DIRECTORY)
	if err != nil {
		return nil, err
	}
	w.stageID, err = directory(w.staging, true)
	if err != nil || (fileIdentity{w.stageID.device, w.stageID.inode}) != w.control.Staging || w.bound() != nil || w.cleanRoot() != nil {
		return nil, ErrRecoveryRequired
	}
	// A Pending root never opens SQLite. Missing/replaced DBs and mounted or
	// unsafe pre-existing sidecars have already refused before this point.
	w.db, err = openCatalog(ctx, path, false)
	if err != nil {
		return nil, err
	}
	var check string
	if w.db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&check) != nil || check != "ok" || w.ready(ctx) != nil {
		return nil, ErrRecoveryRequired
	}
	keep = true
	return w, nil
}

func (w *writer) step(ctx context.Context, label string, action func() error) error {
	if ctx.Err() != nil {
		return ErrCancelled
	}
	if w.fault != nil && w.fault(label+".before") != nil {
		return ErrRecoveryRequired
	}
	if err := action(); err != nil {
		return ErrRecoveryRequired
	}
	if w.fault != nil && w.fault(label+".after") != nil {
		return ErrRecoveryRequired
	}
	return nil
}

// Each fresh control file is retained on failure. A clean Ready reopen rejects
// any unknown root entry, so a failed pre-rename write cannot hide an orphan.
func (w *writer) persistControl(ctx context.Context, c control, label string) error {
	id, err := randomID()
	if err != nil {
		return err
	}
	name := "control-" + id
	fd := -1
	if err = w.step(ctx, label+".create", func() error { var e error; fd, e = createAt(w.r.root, name); return e }); err != nil {
		if fd >= 0 {
			_ = unix.Close(fd)
		}
		return err
	}
	f := os.NewFile(uintptr(fd), "private durable control")
	defer f.Close()
	raw := encoded(c)
	if w.step(ctx, label+".write", func() error {
		n, e := f.Write(raw)
		if e != nil || n != len(raw) {
			return ErrRejected
		}
		return nil
	}) != nil ||
		w.step(ctx, label+".sync", f.Sync) != nil ||
		w.step(ctx, label+".close", f.Close) != nil ||
		w.step(ctx, label+".rename", func() error {
			if w.bound() != nil {
				return ErrRecoveryRequired
			}
			return unix.Renameat2(w.r.root, name, w.r.root, "control.json", 0)
		}) != nil ||
		w.step(ctx, label+".dirsync", func() error { return unix.Fsync(w.r.root) }) != nil {
		return ErrRecoveryRequired
	}
	return nil
}

func (w *writer) begin(ctx context.Context, op intent) error {
	// Do not clear this flag on any uncertain attempt, including a syscall
	// failing before its first write. Only the completed protocol clears it.
	w.blocked = true
	old := w.control.Epoch
	if old == math.MaxInt64 {
		return ErrRecoveryRequired
	}
	w.control.Epoch++
	w.control.Pending = &op
	if w.persistControl(ctx, w.control, "root.pending") != nil || w.beginCatalog(ctx, op, old) != nil {
		return ErrRecoveryRequired
	}
	return nil
}

func (w *writer) finish(ctx context.Context, op intent, proof ObjectProof) error {
	if w.completeCatalog(ctx, op, proof) != nil {
		return ErrRecoveryRequired
	}
	ready := w.control
	ready.Pending = nil
	if w.persistControl(ctx, ready, "root.ready") != nil {
		return ErrRecoveryRequired
	}
	w.control = ready
	if w.bound() != nil || w.catalogReady(ctx) != nil || w.cleanInventory(ctx) != nil {
		return ErrRecoveryRequired
	}
	w.blocked = false
	return nil
}

func proofAt(parent int, name string, root RootProof, objectID, hash string) (ObjectProof, error) {
	fd, err := rooted(parent, name, unix.O_RDONLY)
	if err != nil {
		return ObjectProof{}, err
	}
	defer unix.Close(fd)
	st, err := regular(fd)
	if err != nil || st.Size > uint64(maxObjectBytes) {
		return ObjectProof{}, ErrRejected
	}
	return ObjectProof{root, objectID, unix.Mkdev(st.Dev_major, st.Dev_minor), st.Ino, int64(st.Size), st.Mtime.Sec, st.Mtime.Nsec, st.Ctime.Sec, st.Ctime.Nsec, hash}, nil
}

func (w *writer) Inspect(ctx context.Context, proof ObjectProof) error {
	if !w.gate.TryLock() {
		return ErrBusy
	}
	defer w.gate.Unlock()
	if err := w.ready(ctx); err != nil {
		return err
	}
	if w.catalogObject(ctx, proof) != nil {
		return ErrRejected
	}
	return w.r.Inspect(ctx, proof)
}

func (w *writer) SealCopy(ctx context.Context, source io.Reader, spec CopySpec) (ObjectProof, error) {
	var empty ObjectProof
	if !w.gate.TryLock() {
		return empty, ErrBusy
	}
	defer w.gate.Unlock()
	if err := w.ready(ctx); err != nil {
		return empty, err
	}
	if source == nil || spec.Size < 0 || spec.Size > maxObjectBytes || !opaque(spec.SHA256) || !opaque(spec.AuthorizationSHA256) {
		return empty, ErrRejected
	}
	id, err := randomID()
	if err != nil {
		return empty, err
	}
	objectID, err := randomID()
	if err != nil {
		return empty, err
	}
	op := intent{ID: id, Kind: "SEAL", ObjectID: objectID, Size: spec.Size, SHA256: spec.SHA256, Authorization: spec.AuthorizationSHA256}
	if w.begin(ctx, op) != nil {
		return empty, ErrRecoveryRequired
	}
	fd := -1
	if w.step(ctx, "object.create", func() error {
		if w.bound() != nil {
			return ErrRecoveryRequired
		}
		var e error
		fd, e = createAt(w.staging, id)
		return e
	}) != nil {
		if fd >= 0 {
			_ = unix.Close(fd)
		}
		return empty, ErrRecoveryRequired
	}
	f := os.NewFile(uintptr(fd), "private copied object")
	defer f.Close()
	if w.step(ctx, "object.copy", func() error {
		h := sha256.New()
		buffer := make([]byte, 64*1024)
		var total int64
		for {
			if ctx.Err() != nil {
				return ErrCancelled
			}
			n, e := source.Read(buffer)
			if n < 0 || n > len(buffer) {
				return ErrRejected
			}
			total += int64(n)
			if total > spec.Size {
				return ErrRejected
			}
			if count, err := f.Write(buffer[:n]); err != nil || count != n {
				return ErrRejected
			}
			_, _ = h.Write(buffer[:n])
			if e == io.EOF {
				break
			}
			if e != nil || n == 0 {
				return ErrRejected
			}
		}
		if total != spec.Size || hex.EncodeToString(h.Sum(nil)) != spec.SHA256 {
			return ErrRejected
		}
		return nil
	}) != nil || w.step(ctx, "object.sync", f.Sync) != nil || w.step(ctx, "object.close", f.Close) != nil ||
		w.step(ctx, "stage.copy.dirsync", func() error { return unix.Fsync(w.staging) }) != nil {
		return empty, ErrRecoveryRequired
	}
	staged, err := proofAt(w.staging, id, w.r.proof, objectID, spec.SHA256)
	if err != nil || staged.Size != spec.Size || inspectFile(ctx, w.staging, id, staged) != nil || w.stageCatalog(ctx, op, staged) != nil {
		return empty, ErrRecoveryRequired
	}
	if w.step(ctx, "seal.rename", func() error {
		if w.bound() != nil || inspectFile(ctx, w.staging, id, staged) != nil {
			return ErrRecoveryRequired
		}
		return unix.Renameat2(w.staging, id, w.r.objects, objectID, unix.RENAME_NOREPLACE)
	}) != nil ||
		w.step(ctx, "objects.seal.dirsync", func() error { return unix.Fsync(w.r.objects) }) != nil ||
		w.step(ctx, "stage.seal.dirsync", func() error { return unix.Fsync(w.staging) }) != nil {
		return empty, ErrRecoveryRequired
	}
	proof, err := proofAt(w.r.objects, objectID, w.r.proof, objectID, spec.SHA256)
	if err != nil || proof.Device != staged.Device || proof.Inode != staged.Inode || proof.Size != staged.Size || proof.MTimeSec != staged.MTimeSec || proof.MTimeNsec != staged.MTimeNsec || w.r.Inspect(ctx, proof) != nil || w.finish(ctx, op, proof) != nil {
		return empty, ErrRecoveryRequired
	}
	return proof, nil
}

func (w *writer) Purge(ctx context.Context, proof ObjectProof, authorizationSHA256 string) error {
	if !w.gate.TryLock() {
		return ErrBusy
	}
	defer w.gate.Unlock()
	if err := w.ready(ctx); err != nil {
		return err
	}
	if !opaque(authorizationSHA256) || w.catalogObject(ctx, proof) != nil || w.r.Inspect(ctx, proof) != nil {
		return ErrRejected
	}
	id, err := randomID()
	if err != nil {
		return err
	}
	op := intent{ID: id, Kind: "PURGE", ObjectID: proof.ObjectID, Size: proof.Size, SHA256: proof.SHA256, Authorization: authorizationSHA256, Proof: proof}
	if w.begin(ctx, op) != nil {
		return ErrRecoveryRequired
	}
	if w.step(ctx, "purge.rename", func() error {
		if w.bound() != nil || w.r.Inspect(ctx, proof) != nil {
			return ErrRecoveryRequired
		}
		return unix.Renameat2(w.r.objects, proof.ObjectID, w.staging, id, unix.RENAME_NOREPLACE)
	}) != nil ||
		w.step(ctx, "objects.purge.dirsync", func() error { return unix.Fsync(w.r.objects) }) != nil ||
		w.step(ctx, "stage.purge.dirsync", func() error { return unix.Fsync(w.staging) }) != nil {
		return ErrRecoveryRequired
	}
	staged, err := proofAt(w.staging, id, w.r.proof, proof.ObjectID, proof.SHA256)
	if err != nil || staged.Device != proof.Device || staged.Inode != proof.Inode || staged.Size != proof.Size || staged.MTimeSec != proof.MTimeSec || staged.MTimeNsec != proof.MTimeNsec || inspectFile(ctx, w.staging, id, staged) != nil || w.stageCatalog(ctx, op, staged) != nil || w.irreversibleCatalog(ctx, op) != nil {
		return ErrRecoveryRequired
	}
	if w.step(ctx, "purge.unlink", func() error {
		if w.bound() != nil || inspectFile(ctx, w.staging, id, staged) != nil {
			return ErrRecoveryRequired
		}
		return unix.Unlinkat(w.staging, id, 0)
	}) != nil ||
		w.step(ctx, "stage.unlink.dirsync", func() error { return unix.Fsync(w.staging) }) != nil || w.finish(ctx, op, proof) != nil {
		return ErrRecoveryRequired
	}
	return nil
}

func (w *writer) Close() error {
	if !w.gate.TryLock() {
		return ErrBusy
	}
	defer w.gate.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	var result error
	// SQLite may checkpoint/truncate retained WAL on Close, but PERSIST_WAL
	// prevents namespace deletion. Keep root, sidecar context and flock alive
	// until this same pinned physical connection and its pool have closed.
	if w.db != nil && w.db.Close() != nil {
		result = ErrRecoveryRequired
	}
	if w.r != nil && w.r.Close() != nil {
		result = ErrRecoveryRequired
	}
	for _, fd := range []int{w.staging, w.lock} {
		if fd >= 0 && unix.Close(fd) != nil {
			result = ErrRecoveryRequired
		}
	}
	w.staging, w.lock = -1, -1
	return result
}
