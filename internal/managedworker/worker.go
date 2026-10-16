// Package managedworker isolates the private managed-object primitives behind a
// dedicated low-privilege subject and a fixed local IPC surface.
//
// It exists so that the browser, the Web process and the scanner never hold
// managed-object capability and never supply a filesystem path. A caller may
// only name an already registered object by its opaque identifier; every other
// key is refused before any operation is considered.
//
// This is an unintegrated prototype. It is not imported by the app, the Web API
// or the legacy executor, it exposes no dangerous product capability, and it
// never authorizes source removal, restore, retention or purge approval.
// PurgeReleaseAllowed stays false.
package managedworker

import (
	"errors"
	"net"
	"os"
	"strings"
	"time"
)

var (
	// ErrUnauthorized is a fixed category: the peer identity is never echoed.
	ErrUnauthorized = errors.New("managed worker caller not authorized")
	// ErrIsolation covers every unmet deployment requirement, including a
	// privileged process, a shared directory or a subject that is root.
	ErrIsolation = errors.New("managed worker isolation requirement unmet")
	// ErrUnsupported is returned on platforms without these kernel primitives.
	ErrUnsupported = errors.New("managed worker unavailable on this platform")
	// ErrBusy is the single-instance and in-use lease refusal.
	ErrBusy = errors.New("managed worker already running")
	// ErrProtocol covers malformed, oversized or unauthorized request shapes.
	ErrProtocol = errors.New("managed worker protocol violation")
)

// Subject is a fixed process identity. UID and GID must both be non-zero: the
// worker is never root, in whole or in part.
type Subject struct {
	UID uint32
	GID uint32
}

// Layout is the fixed private directory contract. Both directories must exist,
// must be owned by Subject and must be inaccessible to every other account.
// Run holds the socket and the lease; State is the parent of the managed root.
// They are separate so that a socket is never created inside the managed root,
// where an unknown entry would be a bootstrap recovery condition.
type Layout struct {
	Run   string
	State string
}

// maxRequestBytes bounds a single frame. There is no streaming or chunked mode.
const maxRequestBytes = 64 << 10

// The protocol is a fixed op plus a fixed key set per op. Any other key is
// refused, so a path, a command, an environment variable or an execution hint
// cannot be smuggled into a request even if a caller is authorized.
var allowedKeys = map[string][]string{
	"PING":    {},
	"STATUS":  {},
	"SEAL":    {"object_id", "size", "sha256", "authorization_sha256"},
	"PURGE":   {"object_id", "authorization_sha256", "size", "sha256", "device", "inode"},
	"INSPECT": {"object_id", "size", "sha256", "device", "inode"},
}

// Ops returns the fixed set of operations this prototype accepts.
func Ops() []string {
	names := make([]string, 0, len(allowedKeys))
	for name := range allowedKeys {
		names = append(names, name)
	}
	return names
}

func opaqueID(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'f' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

// pathLike refuses a value that could name a location. The protocol has no path
// field at all, so this is a second, independent guard rather than the primary
// one: it also catches an operator-visible identifier that was pasted in error.
func pathLike(value string) bool {
	return strings.ContainsAny(value, "/\\") || strings.Contains(value, "..")
}

// Listener is the fixed local IPC endpoint. Its concrete listener is never
// exported, so a caller cannot move, rebind or widen it.
type Listener struct {
	ln      *net.UnixListener
	path    string
	callers map[Subject]struct{}
}

// Close releases the listener. It does not delete an existing socket path, so
// a socket created by another account is never replaced by this process.
func (l *Listener) Close() error {
	if l == nil || l.ln == nil {
		return nil
	}
	return l.ln.Close()
}

// Addr reports the bound socket path for auditing.
func (l *Listener) Addr() string {
	if l == nil || l.ln == nil {
		return ""
	}
	return l.path
}

// Conn is one authorized connection. Peer is the kernel-attested identity, not
// anything the caller asserted about itself.
type Conn struct {
	conn *net.UnixConn
	peer Subject
}

// Peer reports the kernel-attested caller identity for auditing.
func (c *Conn) Peer() Subject { return c.peer }

func (c *Conn) Close() error { return c.conn.Close() }

// SetDeadline bounds one connection so a stalled caller cannot hold the
// single-instance worker indefinitely.
func (c *Conn) SetDeadline(at time.Time) error { return c.conn.SetDeadline(at) }

// Lease is the single-instance lock. It is never transferred and never
// reclaimed from a holder that disappeared.
type Lease struct {
	f    *os.File
	path string
}

// Path reports the lease file for auditing.
func (l *Lease) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}
