//go:build linux

package managedworker

import (
	"net"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// SocketName is the fixed socket name. There is no configurable endpoint and
// no abstract-namespace socket: the path is always inside the private run
// directory, which is the only place authorized to hold it.
const SocketName = "worker.sock"

// Listen binds the fixed socket. It refuses to replace an existing socket, so
// a socket created by another account can never be adopted or shadowed by this
// process. The caller must already satisfy RequireSubject and RequireLayout.
func Listen(layout Layout, want Subject, callers []Subject) (*Listener, error) {
	if err := RequireSubject(want); err != nil {
		return nil, err
	}
	if err := RequireLayout(layout, want); err != nil {
		return nil, err
	}
	if len(callers) == 0 {
		return nil, ErrIsolation
	}
	authorized := map[Subject]struct{}{}
	for _, caller := range callers {
		if caller.UID == 0 || caller.GID == 0 {
			return nil, ErrIsolation
		}
		authorized[caller] = struct{}{}
	}
	path := filepath.Join(layout.Run, SocketName)
	if _, err := os.Lstat(path); err == nil {
		return nil, ErrIsolation
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, ErrIsolation
	}
	// Any local process may reach the socket; admission is decided only by the
	// kernel-attested peer credentials. A restrictive mode here would stop an
	// outsider at the filesystem and make the credential check untestable.
	if err := os.Chmod(path, 0o666); err != nil {
		_ = ln.Close()
		return nil, ErrIsolation
	}
	return &Listener{ln: ln.(*net.UnixListener), path: path, callers: authorized}, nil
}

// Accept admits a connection only when the kernel attests the peer identity.
// Credentials come from SO_PEERCRED, never from the request body, so a caller
// cannot claim to be an authorized subject.
func (l *Listener) Accept() (*Conn, error) {
	raw, err := l.ln.Accept()
	if err != nil {
		return nil, err
	}
	conn, ok := raw.(*net.UnixConn)
	if !ok {
		_ = raw.Close()
		return nil, ErrProtocol
	}
	peer, err := peerSubject(conn)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if _, ok := l.callers[peer]; !ok {
		_ = conn.Close()
		return nil, ErrUnauthorized
	}
	return &Conn{conn: conn, peer: peer}, nil
}

func peerSubject(conn *net.UnixConn) (Subject, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return Subject{}, ErrUnsupported
	}
	var (
		cred *unix.Ucred
		e    error
	)
	if err := raw.Control(func(fd uintptr) {
		cred, e = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return Subject{}, ErrUnsupported
	}
	if e != nil || cred == nil {
		return Subject{}, ErrUnsupported
	}
	return Subject{UID: cred.Uid, GID: cred.Gid}, nil
}
