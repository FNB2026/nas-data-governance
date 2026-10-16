//go:build linux

package managedworker

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func deadline() time.Time { return time.Now().Add(10 * time.Second) }

func dialUnix(path string) (*net.UnixConn, error) {
	addr, err := net.ResolveUnixAddr("unix", path)
	if err != nil {
		return nil, err
	}
	return net.DialUnix("unix", nil, addr)
}

func workerSubject(t *testing.T) Subject {
	t.Helper()
	if os.Geteuid() == 0 || os.Getegid() == 0 {
		t.Skip("non-root worker required")
	}
	return Subject{UID: uint32(os.Geteuid()), GID: uint32(os.Getegid())}
}

func privateDir(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal("private fixture failed", err)
	}
	return path
}

func TestRequireSubjectRefusesRootAndMismatch(t *testing.T) {
	want := workerSubject(t)
	if got := RequireSubject(Subject{UID: 0, GID: want.GID}); !errors.Is(got, ErrIsolation) {
		t.Fatal("root UID accepted", got)
	}
	if got := RequireSubject(Subject{UID: want.UID, GID: 0}); !errors.Is(got, ErrIsolation) {
		t.Fatal("root GID accepted", got)
	}
	if got := RequireSubject(Subject{UID: want.UID + 1000, GID: want.GID}); !errors.Is(got, ErrIsolation) {
		t.Fatal("foreign UID accepted", got)
	}
	if got := RequireSubject(want); got != nil {
		t.Fatal("matching subject refused", got)
	}
}

// The subject check must not be satisfiable by a process that merely looks
// unprivileged: capabilities are read from the kernel, not from the uid.
func TestRequireSubjectUsesKernelCapabilityState(t *testing.T) {
	want := workerSubject(t)
	caps, err := kernelStatus()
	if err != nil {
		t.Fatal("kernel status unavailable", err)
	}
	effective, err := parseHexField(caps, "CapEff:")
	if err != nil {
		t.Fatal("CapEff unavailable", err)
	}
	if effective != 0 {
		t.Skip("runner holds capabilities; not an unprivileged worker")
	}
	if got := RequireSubject(want); got != nil {
		t.Fatal("unprivileged subject refused", got)
	}
}

func TestRequireLayoutRefusesSharedOrMissingDirectories(t *testing.T) {
	want := workerSubject(t)
	good := privateDir(t)
	other := privateDir(t)
	if got := RequireLayout(Layout{Run: good, State: other}, want); got != nil {
		t.Fatal("valid layout refused", got)
	}
	if got := RequireLayout(Layout{Run: good, State: good}, want); !errors.Is(got, ErrIsolation) {
		t.Fatal("shared run/state accepted", got)
	}
	if got := RequireLayout(Layout{Run: good, State: ""}, want); !errors.Is(got, ErrIsolation) {
		t.Fatal("empty state accepted", got)
	}
	if got := RequireLayout(Layout{Run: "", State: other}, want); !errors.Is(got, ErrIsolation) {
		t.Fatal("empty run accepted", got)
	}
	missing := filepath.Join(good, "absent")
	if got := RequireLayout(Layout{Run: good, State: missing}, want); !errors.Is(got, ErrIsolation) {
		t.Fatal("missing directory accepted", got)
	}
}

// A directory another account could traverse is not a private worker layout,
// even though this process owns it.
func TestRequireLayoutRefusesGroupOrWorldAccessibleDirectory(t *testing.T) {
	want := workerSubject(t)
	path := privateDir(t)
	if err := os.Chmod(path, 0o750); err != nil {
		t.Fatal("chmod failed", err)
	}
	if got := RequireLayout(Layout{Run: path, State: privateDir(t)}, want); !errors.Is(got, ErrIsolation) {
		t.Fatal("group-readable directory accepted", got)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal("chmod restore failed", err)
	}
	if got := RequireLayout(Layout{Run: path, State: privateDir(t)}, want); got != nil {
		t.Fatal("restored directory refused", got)
	}
}

func TestAcquireLeaseIsSingleInstance(t *testing.T) {
	want := workerSubject(t)
	dir := privateDir(t)
	first, err := AcquireLease(dir, want)
	if err != nil {
		t.Fatal("first lease refused", err)
	}
	if err := first.Record(); err != nil {
		t.Fatal("lease record failed", err)
	}
	pid, alive, err := LeaseHolder(dir)
	if err != nil {
		t.Fatal("lease holder unreadable", err)
	}
	if pid != os.Getpid() || !alive {
		t.Fatal("lease holder mismatch", pid, alive)
	}
	// A second worker must be refused outright, never queued or adopted.
	if second, err := AcquireLease(dir, want); second != nil || !errors.Is(err, ErrBusy) {
		if second != nil {
			_ = second.Release()
		}
		t.Fatal("second lease acquired", err)
	}
	if err := first.Release(); err != nil {
		t.Fatal("lease release failed", err)
	}
	// After release the lease is acquirable again, but the file identity and
	// ownership are deliberately preserved rather than recreated.
	third, err := AcquireLease(dir, want)
	if err != nil {
		t.Fatal("lease not reusable after release", err)
	}
	if third.Path() != first.Path() {
		t.Fatal("lease path changed across restarts")
	}
	if err := third.Release(); err != nil {
		t.Fatal("second release failed", err)
	}
}

func TestAcquireLeaseRefusesForeignDirectory(t *testing.T) {
	want := workerSubject(t)
	dir := privateDir(t)
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal("chmod failed", err)
	}
	if lease, err := AcquireLease(dir, want); lease != nil || !errors.Is(err, ErrIsolation) {
		if lease != nil {
			_ = lease.Release()
		}
		t.Fatal("lease taken in a shared directory", err)
	}
}

// The socket must never replace an existing entry, so a socket planted by
// another account cannot be adopted or shadowed by this worker.
func TestListenRefusesExistingSocket(t *testing.T) {
	want := workerSubject(t)
	layout := Layout{Run: privateDir(t), State: privateDir(t)}
	socket := filepath.Join(layout.Run, SocketName)
	if err := os.WriteFile(socket, []byte("planted"), 0o600); err != nil {
		t.Fatal("socket fixture failed", err)
	}
	if ln, err := Listen(layout, want, []Subject{want}); ln != nil || !errors.Is(err, ErrIsolation) {
		if ln != nil {
			_ = ln.Close()
		}
		t.Fatal("existing socket adopted", err)
	}
	raw, err := os.ReadFile(socket)
	if err != nil || string(raw) != "planted" {
		t.Fatal("planted socket was replaced", err)
	}
}

func TestListenRefusesRootCallerAndRequiresLayout(t *testing.T) {
	want := workerSubject(t)
	layout := Layout{Run: privateDir(t), State: privateDir(t)}
	if ln, err := Listen(layout, want, []Subject{{UID: 0, GID: want.GID}}); ln != nil || !errors.Is(err, ErrIsolation) {
		if ln != nil {
			_ = ln.Close()
		}
		t.Fatal("root caller authorized", err)
	}
	if ln, err := Listen(layout, want, nil); ln != nil || !errors.Is(err, ErrIsolation) {
		if ln != nil {
			_ = ln.Close()
		}
		t.Fatal("empty caller list accepted", err)
	}
	shared := Layout{Run: layout.Run, State: layout.Run}
	if ln, err := Listen(shared, want, []Subject{want}); ln != nil || !errors.Is(err, ErrIsolation) {
		if ln != nil {
			_ = ln.Close()
		}
		t.Fatal("shared layout accepted", err)
	}
}

// Authorized round trip: the kernel-attested peer must match exactly what the
// listener was configured to admit.
func TestListenAcceptsAuthorizedPeerAndReportsIdentity(t *testing.T) {
	want := workerSubject(t)
	layout := Layout{Run: privateDir(t), State: privateDir(t)}
	ln, err := Listen(layout, want, []Subject{want})
	if err != nil {
		t.Fatal("listen failed", err)
	}
	defer ln.Close()
	if ln.Addr() != filepath.Join(layout.Run, SocketName) {
		t.Fatal("socket path mismatch", ln.Addr())
	}
	info, err := os.Lstat(ln.Addr())
	if err != nil || info.Mode()&0o7777 != 0o600 {
		t.Fatal("socket is not owner-only", err)
	}
	done := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		if conn.Peer() != want {
			done <- errors.New("peer identity mismatch")
			return
		}
		if err := conn.SetDeadline(deadline()); err != nil {
			done <- err
			return
		}
		request, err := conn.ReadRequest()
		if err != nil || request.Op != "PING" {
			done <- err
			return
		}
		done <- conn.WriteResponse([]byte(`{"ok":true}`))
	}()
	client, err := dialUnix(ln.Addr())
	if err != nil {
		t.Fatal("dial failed", err)
	}
	defer client.Close()
	body := []byte(`{"op":"PING"}`)
	raw, err := frame(body)
	if err != nil {
		t.Fatal("frame failed", err)
	}
	if _, err := client.Write(raw); err != nil {
		t.Fatal("write failed", err)
	}
	if err := <-done; err != nil {
		t.Fatal("authorized round trip failed", err)
	}
}
