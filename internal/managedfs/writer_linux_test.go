//go:build linux

package managedfs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

var testContext = context.Background()

func privateParent(t *testing.T) string {
	t.Helper()
	// Go's numbered TempDir child uses 0777 filtered by the process umask;
	// that is not the explicit private parent required by Initialize.
	parent := filepath.Join(t.TempDir(), "worker")
	if os.Mkdir(parent, 0700) != nil {
		t.Fatal("private parent fixture failed")
	}
	return parent
}

// One probe decides the whole test: every subtest shares this platform.
func requireBootstrapKernel(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 || os.Getegid() == 0 {
		t.Skip("non-root worker required")
	}
	if _, err := Initialize(testContext, filepath.Join(privateParent(t), "probe")); errors.Is(err, ErrUnsupported) && os.Getenv("NDG_REQUIRE_MANAGED_READER_KERNEL") != "1" {
		t.Skip("unsupported execution filesystem/kernel")
	}
}

func initialized(t *testing.T) (string, RootProof) {
	t.Helper()
	if os.Geteuid() == 0 || os.Getegid() == 0 {
		t.Skip("non-root worker required")
	}
	path := filepath.Join(privateParent(t), "execution")
	proof, err := Initialize(testContext, path)
	if errors.Is(err, ErrUnsupported) && os.Getenv("NDG_REQUIRE_MANAGED_READER_KERNEL") != "1" {
		t.Skip("unsupported execution filesystem/kernel")
	}
	if err != nil {
		t.Fatal("private bootstrap failed", err)
	}
	return path, proof
}

func writable(t *testing.T, path string, root RootProof) *writer {
	t.Helper()
	w, err := OpenWriter(testContext, path, root)
	if err != nil {
		t.Fatal("private writer open failed", err)
	}
	t.Cleanup(func() {
		if w.Close() != nil {
			t.Error("private writer close failed")
		}
	})
	return w.(*writer)
}

func copySpec(raw []byte) CopySpec {
	h := sha256.Sum256(raw)
	return CopySpec{int64(len(raw)), hex.EncodeToString(h[:]), strings.Repeat("e", 64)}
}

func sealed(t *testing.T, w *writer, raw []byte) ObjectProof {
	t.Helper()
	proof, err := w.SealCopy(testContext, bytes.NewReader(raw), copySpec(raw))
	if err != nil {
		t.Fatal("seal failed", err)
	}
	return proof
}

func TestWriterSealPurgeReopenAndKeeper(t *testing.T) {
	path, root := initialized(t)
	w := writable(t, path, root)
	raw := []byte("synthetic immutable source and keeper")
	keeper := append([]byte{}, raw...)
	proof := sealed(t, w, raw)
	if w.Inspect(testContext, proof) != nil {
		t.Fatal("registered object did not match")
	}
	if w.Close() != nil {
		t.Fatal("first close failed")
	}
	w = writable(t, path, root)
	if w.Inspect(testContext, proof) != nil || w.Purge(testContext, proof, strings.Repeat("f", 64)) != nil {
		t.Fatal("reopened managed purge failed")
	}
	if _, err := os.Lstat(filepath.Join(path, "objects", proof.ObjectID)); !os.IsNotExist(err) {
		t.Fatal("purged name retained")
	}
	if !bytes.Equal(raw, keeper) {
		t.Fatal("input or keeper changed")
	}
	if !errors.Is(w.Purge(testContext, proof, strings.Repeat("f", 64)), ErrRejected) {
		t.Fatal("replayed purge did not refuse")
	}
	if w.Close() != nil {
		t.Fatal("second close failed")
	}
	w = writable(t, path, root)
	if w.control.Epoch != 2 || w.catalogReady(testContext) != nil {
		t.Fatal("durable completed epoch missing")
	}
	firstClose, secondClose := w.Close(), w.Close()
	if firstClose != nil || secondClose != nil || !errors.Is(w.Inspect(testContext, proof), ErrClosed) {
		t.Fatal("closed lifetime mismatch")
	}
}

func TestInitializeRejectsNonPrivateParent(t *testing.T) {
	if os.Geteuid() == 0 || os.Getegid() == 0 {
		t.Skip("non-root worker required")
	}
	parent := filepath.Join(t.TempDir(), "public-parent")
	if os.Mkdir(parent, 0700) != nil || os.Chmod(parent, 0755) != nil {
		t.Fatal("non-private fixture failed")
	}
	path := filepath.Join(parent, "execution")
	if proof, err := Initialize(testContext, path); proof != (RootProof{}) || !errors.Is(err, ErrRejected) {
		t.Fatal("non-private parent accepted")
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("non-private parent refusal wrote a root")
	}
}

func TestWriterRefusesExistingAndPartialBootstrap(t *testing.T) {
	path, root := initialized(t)
	before, err := os.ReadFile(filepath.Join(path, "control.json"))
	if err != nil {
		t.Fatal("control fixture failed")
	}
	if _, err := Initialize(testContext, path); !errors.Is(err, ErrRejected) {
		t.Fatal("existing root was adopted")
	}
	after, _ := os.ReadFile(filepath.Join(path, "control.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("existing root changed")
	}
	if os.Rename(filepath.Join(path, "control.json"), filepath.Join(path, "control-incomplete")) != nil {
		t.Fatal("partial bootstrap fixture failed")
	}
	if w, err := OpenWriter(testContext, path, root); w != nil || !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal("partial bootstrap accepted")
	}
}

func TestWriterLeaseAndStoredChildIdentities(t *testing.T) {
	path, root := initialized(t)
	w := writable(t, path, root)
	if second, err := OpenWriter(testContext, path, root); second != nil || !errors.Is(err, ErrBusy) {
		t.Fatal("second writer acquired lease")
	}
	if w.Close() != nil {
		t.Fatal("lease close failed")
	}
	for _, name := range []string{"objects", "staging", "writer.lock", "catalog.sqlite"} {
		t.Run(name, func(t *testing.T) {
			path, root := initialized(t)
			original := filepath.Join(path, name)
			moved := filepath.Join(filepath.Dir(path), "saved-"+name)
			if os.Rename(original, moved) != nil {
				t.Fatal("replace fixture move failed")
			}
			if name == "objects" || name == "staging" {
				if os.Mkdir(original, 0700) != nil {
					t.Fatal("replace directory fixture failed")
				}
			} else if os.WriteFile(original, nil, 0600) != nil {
				t.Fatal("replace file fixture failed")
			}
			if second, err := OpenWriter(testContext, path, root); second != nil || err == nil {
				t.Fatal("new child inode adopted")
			}
		})
	}
}

func TestWriterMissingCatalogDoesNotCreate(t *testing.T) {
	path, root := initialized(t)
	file := filepath.Join(path, "catalog.sqlite")
	if os.Rename(file, filepath.Join(filepath.Dir(path), "saved-catalog")) != nil {
		t.Fatal("missing fixture failed")
	}
	if second, err := OpenWriter(testContext, path, root); second != nil || err == nil {
		t.Fatal("missing database accepted")
	}
	if _, err := os.Lstat(file); !os.IsNotExist(err) {
		t.Fatal("opener created a database")
	}
}

func TestWriterRejectsCatalogRollbackWithSameInode(t *testing.T) {
	path, root := initialized(t)
	w := writable(t, path, root)
	proof := sealed(t, w, []byte("synthetic rollback object"))
	if w.Close() != nil {
		t.Fatal("snapshot close failed")
	}
	snapshot := map[string][]byte{}
	for _, name := range []string{"catalog.sqlite", "catalog.sqlite-wal", "catalog.sqlite-shm"} {
		raw, err := os.ReadFile(filepath.Join(path, name))
		if err != nil {
			t.Fatal("cold snapshot failed")
		}
		snapshot[name] = raw
	}
	w = writable(t, path, root)
	if w.Purge(testContext, proof, strings.Repeat("f", 64)) != nil || w.Close() != nil {
		t.Fatal("newer epoch fixture failed")
	}
	// Artificial rollback keeps the catalog inode and UUID. Epoch mismatch
	// must refuse even though this is not caught by the main-file inode gate.
	for name, raw := range snapshot {
		if os.WriteFile(filepath.Join(path, name), raw, 0600) != nil {
			t.Fatal("artificial catalog rollback failed")
		}
	}
	if second, err := OpenWriter(testContext, path, root); second != nil || !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal("stale catalog epoch accepted")
	}
}

func TestWriterStaleProofAndForeignFilesStayUnchanged(t *testing.T) {
	for _, kind := range []string{"same-bytes-replacement", "content", "hardlink", "symlink", "size", "hash", "authorization"} {
		t.Run(kind, func(t *testing.T) {
			path, root := initialized(t)
			w := writable(t, path, root)
			raw := []byte("synthetic keeper for stale proof")
			proof := sealed(t, w, raw)
			file := filepath.Join(path, "objects", proof.ObjectID)
			keeper := filepath.Join(filepath.Dir(path), "keeper")
			if os.WriteFile(keeper, raw, 0600) != nil {
				t.Fatal("keeper fixture failed")
			}
			auth := strings.Repeat("f", 64)
			switch kind {
			case "same-bytes-replacement":
				if os.Rename(file, filepath.Join(filepath.Dir(path), "saved-object")) != nil || os.WriteFile(file, raw, 0600) != nil {
					t.Fatal("replacement failed")
				}
			case "content":
				if os.WriteFile(file, []byte("changed synthetic bytes"), 0600) != nil {
					t.Fatal("content change failed")
				}
			case "hardlink":
				if os.Link(file, filepath.Join(filepath.Dir(path), "hardlink")) != nil {
					t.Fatal("hardlink failed")
				}
			case "symlink":
				if os.Rename(file, filepath.Join(filepath.Dir(path), "saved-object")) != nil || os.Symlink(keeper, file) != nil {
					t.Fatal("symlink failed")
				}
			case "size":
				proof.Size++
			case "hash":
				proof.SHA256 = strings.Repeat("0", 64)
			case "authorization":
				auth = "not-an-authorization"
			}
			if err := w.Purge(testContext, proof, auth); !errors.Is(err, ErrRejected) {
				t.Fatal("stale/invalid target accepted", err)
			}
			after, _ := os.ReadFile(keeper)
			if !bytes.Equal(after, raw) || w.control.Epoch != 1 || w.blocked {
				t.Fatal("pre-intent refusal modified keeper or epoch")
			}
		})
	}
}

// These are protocol fault/acknowledgement cases, not a claim of physical
// power-loss behavior. The kernel driver separately kills actual processes.
func TestWriterEveryPersistenceBoundaryBlocksUncertainSession(t *testing.T) {
	common := []string{"root.pending.create", "root.pending.write", "root.pending.sync", "root.pending.close", "root.pending.rename", "root.pending.dirsync", "catalog.begin", "catalog.staged", "catalog.complete", "root.ready.create", "root.ready.write", "root.ready.sync", "root.ready.close", "root.ready.rename", "root.ready.dirsync"}
	common = append(common, "catalog.begin.commit", "catalog.staged.commit", "catalog.complete.commit")
	for _, kind := range []string{"SEAL", "PURGE"} {
		labels := append([]string{}, common...)
		if kind == "SEAL" {
			labels = append(labels, "object.create", "object.copy", "object.sync", "object.close", "stage.copy.dirsync", "seal.rename", "objects.seal.dirsync", "stage.seal.dirsync")
		} else {
			labels = append(labels, "purge.rename", "objects.purge.dirsync", "stage.purge.dirsync", "catalog.irreversible", "catalog.irreversible.commit", "purge.unlink", "stage.unlink.dirsync")
		}
		for _, label := range labels {
			for _, edge := range []string{"before", "after"} {
				t.Run(kind+"/"+label+"."+edge, func(t *testing.T) {
					path, root := initialized(t)
					w := writable(t, path, root)
					raw := []byte("synthetic boundary object")
					var proof ObjectProof
					if kind == "PURGE" {
						proof = sealed(t, w, raw)
					}
					reached := false
					w.fault = func(at string) error {
						if at == label+"."+edge {
							reached = true
							return errors.New("synthetic fault")
						}
						return nil
					}
					var err error
					if kind == "SEAL" {
						_, err = w.SealCopy(testContext, bytes.NewReader(raw), copySpec(raw))
					} else {
						err = w.Purge(testContext, proof, strings.Repeat("f", 64))
					}
					if !reached || !errors.Is(err, ErrRecoveryRequired) || !w.blocked {
						t.Fatal("uncertain attempt did not block")
					}
					if !errors.Is(w.Inspect(testContext, proof), ErrRecoveryRequired) || !errors.Is(w.Purge(testContext, proof, strings.Repeat("f", 64)), ErrRecoveryRequired) {
						t.Fatal("blocked writer admitted another operation")
					}
					if _, err := w.SealCopy(testContext, bytes.NewReader(raw), copySpec(raw)); !errors.Is(err, ErrRecoveryRequired) {
						t.Fatal("blocked writer admitted a copy")
					}
					if w.Close() != nil {
						t.Fatal("blocked close failed")
					}
					second, err := OpenWriter(testContext, path, root)
					mayBeComplete := label == "root.pending.create" && edge == "before" || label == "root.ready.rename" && edge == "after" || label == "root.ready.dirsync"
					if second != nil {
						defer second.Close()
						if !mayBeComplete || err != nil {
							t.Fatal("incomplete operation silently reopened")
						}
					} else if err == nil || mayBeComplete {
						t.Fatal("unexpected reopen state", err)
					}
					if !bytes.Equal(raw, []byte("synthetic boundary object")) {
						t.Fatal("source bytes changed")
					}
				})
			}
		}
	}
}

func TestWriterRealSQLiteReadOnlyFailureAndCancellation(t *testing.T) {
	for _, kind := range []string{"sqlite-readonly", "cancel-after-begin"} {
		t.Run(kind, func(t *testing.T) {
			path, root := initialized(t)
			w := writable(t, path, root)
			ctx, cancel := context.WithCancel(testContext)
			defer cancel()
			if kind == "sqlite-readonly" {
				if _, err := w.db.ExecContext(ctx, "PRAGMA query_only=ON"); err != nil {
					t.Fatal("readonly fixture failed")
				}
			} else {
				w.fault = func(at string) error {
					if at == "catalog.begin.after" {
						cancel()
					}
					return nil
				}
			}
			raw := []byte("synthetic cancelled source")
			if _, err := w.SealCopy(ctx, bytes.NewReader(raw), copySpec(raw)); !errors.Is(err, ErrRecoveryRequired) {
				t.Fatal("uncertain store failure accepted")
			}
			if w.Close() != nil {
				t.Fatal("uncertain store close failed")
			}
			if second, err := OpenWriter(testContext, path, root); second != nil || !errors.Is(err, ErrRecoveryRequired) {
				t.Fatal("pending root admitted fresh writer")
			}
		})
	}
}

func TestWriterReentrantCloseDoesNotReleaseOrReuseDescriptors(t *testing.T) {
	path, root := initialized(t)
	w := writable(t, path, root)
	var alias Writer = w
	checked := false
	w.fault = func(at string) error {
		if at != "object.copy.before" {
			return nil
		}
		checked = true
		if !errors.Is(alias.Close(), ErrBusy) || !errors.Is(alias.Inspect(testContext, ObjectProof{}), ErrBusy) {
			t.Fatal("callback reentered session")
		}
		if second, err := OpenWriter(testContext, path, root); second != nil || !errors.Is(err, ErrRecoveryRequired) {
			t.Fatal("pending root/lease escaped callback")
		}
		for n := 0; n < 32; n++ {
			fd, err := unix.Open("/dev/null", unix.O_RDONLY|unix.O_CLOEXEC, 0)
			if err != nil {
				t.Fatal("descriptor pressure failed")
			}
			if fd == w.lock || fd == w.staging || fd == w.r.root || fd == w.r.objects {
				t.Fatal("busy Close released retained FD")
			}
			_ = unix.Close(fd)
		}
		return nil
	}
	proof := sealed(t, w, []byte("synthetic callback object"))
	if !checked || alias.Inspect(testContext, proof) != nil {
		t.Fatal("non-reentrant callback lost session")
	}
}

type pausedCopy struct {
	entered, release chan struct{}
	finished         bool
}

func (p *pausedCopy) Read(buffer []byte) (int, error) {
	if p.finished {
		return 0, io.EOF
	}
	close(p.entered)
	<-p.release
	p.finished = true
	return copy(buffer, "synthetic concurrent input"), nil
}

// Ordered exactly as Initialize executes them. This list is the coverage
// contract: a new bootstrap step that is missing here has no fault evidence.
var bootstrapSteps = []string{
	"parent.resolve", "parent.private", "parent.filesystem",
	"root.mkdir", "root.open",
	"partial.create", "partial.sync", "partial.close",
	"root.private", "root.filesystem",
	"objects.mkdir", "objects.open", "objects.private", "objects.sync", "objects.close",
	"staging.mkdir", "staging.open", "staging.private", "staging.sync", "staging.close",
	"rootjson.create", "rootjson.write", "rootjson.sync", "rootjson.close",
	"lockfile.create", "lockfile.sync", "lockfile.close",
	"dbfile.create", "dbfile.sync", "dbfile.close",
	"lease.open", "lease.flock",
	"root.predirsync", "parent.predirsync",
	"catalog.open", "catalog.schema", "catalog.identity", "catalog.close",
	"catalog.sidecars", "catalog.stat", "root.presync",
	"control.lock", "control.children",
	"control.create", "control.write", "control.sync", "control.close",
	"root.dirsync", "parent.dirsync",
	"partial.unlink", "partial.dirsync",
}

// Without this, a newly added bootstrap step silently has no fault evidence.
func TestBootstrapStepListMatchesExecutedProtocol(t *testing.T) {
	requireBootstrapKernel(t)
	seen := []string{}
	bootstrapFault = func(at string) error {
		seen = append(seen, at)
		return nil
	}
	defer func() { bootstrapFault = nil }()
	if _, err := Initialize(testContext, filepath.Join(privateParent(t), "execution")); err != nil {
		t.Fatal("clean bootstrap failed", err)
	}
	expected := []string{}
	for _, label := range bootstrapSteps {
		expected = append(expected, label+".before", label+".after")
	}
	if !reflect.DeepEqual(seen, expected) {
		t.Fatalf("bootstrap step list is stale: executed %d, declared %d\n%v", len(seen), len(expected), seen)
	}
}

// These are protocol fault cases, not a claim of physical power-loss behavior.
// Nothing adopts, repairs or deletes the residual partial root.
func TestInitializeEveryStepFailsClosed(t *testing.T) {
	requireBootstrapKernel(t)
	for _, label := range bootstrapSteps {
		for _, edge := range []string{"before", "after"} {
			t.Run(label+"."+edge, func(t *testing.T) {
				// Only the removal of the marker and its final sync are allowed
				// to leave a structurally complete root behind.
				complete := label == "partial.unlink" && edge == "after" || label == "partial.dirsync"
				parent := privateParent(t)
				path := filepath.Join(parent, "execution")
				reached := false
				bootstrapFault = func(at string) error {
					if at == label+"."+edge {
						reached = true
						return errors.New("synthetic fault")
					}
					return nil
				}
				defer func() { bootstrapFault = nil }()
				proof, err := Initialize(testContext, path)
				if !reached {
					t.Fatal("fault point was never reached")
				}
				if err == nil || proof != (RootProof{}) {
					t.Fatal("incomplete bootstrap succeeded")
				}
				bootstrapFault = nil
				if _, err := os.Lstat(path); err != nil {
					// A clean pre-operation state must not retain anything.
					entries, e := os.ReadDir(parent)
					if e != nil || len(entries) != 0 {
						t.Fatal("bootstrap wrote outside the private root")
					}
					if _, e := Initialize(testContext, path); e != nil {
						t.Fatal("clean pre-operation state was not reusable", e)
					}
					return
				}
				// A partial root is never adopted, migrated or clobbered.
				if _, err := Initialize(testContext, path); err == nil {
					t.Fatal("partial root was adopted or clobbered")
				}
				if complete {
					return
				}
				entries, err := os.ReadDir(path)
				if err != nil {
					t.Fatal("residual root unreadable", err)
				}
				if len(entries) == 0 {
					return // An empty private root carries no bootstrap state.
				}
				// Any root that received content must name itself incomplete.
				if _, err := os.Lstat(filepath.Join(path, bootstrapPartialName)); err != nil {
					t.Fatal("incomplete root is not self-identifying")
				}
			})
		}
	}
}

// Uses the real stored proof, so this cannot pass on a proof mismatch: only
// the partial marker can make these refusals fire.
func TestBootstrapPartialMarkerRefusesReaderAndWriter(t *testing.T) {
	path, root := initialized(t)
	marker := filepath.Join(path, bootstrapPartialName)
	if os.WriteFile(marker, nil, 0600) != nil {
		t.Fatal("partial marker fixture failed")
	}
	if r, err := OpenReader(path, root); r != nil || err == nil {
		t.Fatal("partial root was readable")
	}
	if w, err := OpenWriter(testContext, path, root); w != nil || err == nil {
		t.Fatal("partial root was writable")
	}
	// Positive control: the same root without the marker is complete.
	if os.Remove(marker) != nil {
		t.Fatal("positive control removal failed")
	}
	w, err := OpenWriter(testContext, path, root)
	if err != nil {
		t.Fatal("complete root was refused", err)
	}
	if w.Close() != nil {
		t.Fatal("complete root close failed")
	}
}

func TestWriterConcurrentCloseKeepsLeaseAndDescriptors(t *testing.T) {
	path, root := initialized(t)
	w := writable(t, path, root)
	input := &pausedCopy{entered: make(chan struct{}), release: make(chan struct{})}
	finished := make(chan error, 1)
	go func() {
		_, err := w.SealCopy(testContext, input, copySpec([]byte("synthetic concurrent input")))
		finished <- err
	}()
	select {
	case <-input.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent operation did not enter")
	}
	busyClose := w.Close()
	busyInspect := w.Inspect(testContext, ObjectProof{})
	busyPurge := w.Purge(testContext, ObjectProof{}, strings.Repeat("f", 64))
	close(input.release)
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal("concurrent operation lost descriptors", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent operation did not finish")
	}
	if !errors.Is(busyClose, ErrBusy) || !errors.Is(busyInspect, ErrBusy) || !errors.Is(busyPurge, ErrBusy) {
		t.Fatal("busy lifetime admitted concurrent operation")
	}
	if w.Close() != nil {
		t.Fatal("concurrent writer close failed")
	}
}
