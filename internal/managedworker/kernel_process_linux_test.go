//go:build linux

package managedworker

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/FNB2026/nas-data-governance/internal/managedfs"
)

// Driven only by the disposable namespace harness, under a dedicated untrusted
// UID, with artificial private directories. It never touches a product source,
// a service, a network setting or a real deployment.
func TestManagedWorkerKernelProcess(t *testing.T) {
	path := os.Getenv("NDG_MANAGED_WORKER_PROBE")
	if path == "" {
		t.Skip("disposable kernel driver required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("private probe input unavailable")
	}
	var input struct {
		Mode      string   `json:"mode"`
		Run       string   `json:"run"`
		State     string   `json:"state"`
		Root      string   `json:"root"`
		Socket    string   `json:"socket"`
		Ops       []string `json:"ops"`
		Auth      string   `json:"auth"`
		Bootstrap bool     `json:"bootstrap"`
		WorkerUID uint32   `json:"worker_uid"`
		WorkerGID uint32   `json:"worker_gid"`
		CallerUID uint32   `json:"caller_uid"`
		CallerGID uint32   `json:"caller_gid"`
	}
	if json.Unmarshal(raw, &input) != nil {
		t.Fatal("private probe input malformed")
	}
	subject := Subject{UID: input.WorkerUID, GID: input.WorkerGID}
	caller := Subject{UID: input.CallerUID, GID: input.CallerGID}
	layout := Layout{Run: input.Run, State: input.State}

	switch input.Mode {
	case "serve":
		serve(t, layout, subject, caller)
	case "duplicate":
		// A second worker must be refused outright rather than queued.
		if lease, err := AcquireLease(layout.Run, subject); lease != nil || !errors.Is(err, ErrBusy) {
			if lease != nil {
				_ = lease.Release()
			}
			t.Fatal("duplicate worker acquired the lease", err)
		}
		emit(map[string]any{"state": "PASS", "category": "duplicate_refused"})
	case "no_new_privs":
		// The launcher is responsible for setting it; without it the worker
		// refuses, so a setuid helper cannot widen an apparently low worker.
		if err := RequireNoNewPrivs(); !errors.Is(err, ErrIsolation) {
			t.Fatal("worker accepted a process that can regain privileges", err)
		}
		emit(map[string]any{"state": "PASS", "category": "no_new_privs_required"})
	case "dial":
		// The driver runs this under a specific UID, because a peer identity
		// is only meaningful when the connecting process actually has it.
		conn, err := net.Dial("unix", input.Socket)
		if err != nil {
			t.Fatal("dial failed", err)
		}
		_ = conn.Close()
		emit(map[string]any{"state": "PASS", "category": "dialed", "uid": os.Geteuid()})
	case "send":
		// The authorized peer sends the operations itself, so a refusal is
		// witnessed by the caller that would have performed it.
		sendMode(t, input.Socket, input.Ops)
	case "dispatch":
		dispatchMode(t, layout, subject, caller, input.Ops)
	case "seal_refused":
		sealRefusedMode(t, layout, subject, input.Root, input.State)
	case "seal":
		sealMode(t, layout, subject, input.Root, input.State)
	case "reader":
		readerMode(t, layout, subject, input.Root, input.State)
	case "reader_refused":
		readerRefusedMode(t, layout, subject, input.Root, input.State)
	case "pending":
		// Begins a real SealCopy, announces readiness from inside the copy,
		// then waits: the driver SIGKILLs this process at that exact point so
		// the next worker meets a root with a durable pending intent.
		pendingMode(t, layout, subject, input.Root, input.State, input.Bootstrap, input.Auth)
	case "hold":
		// Holds the lease while the driver tries to start a second worker.
		if err := RequireSubject(subject); err != nil {
			t.Fatal("subject refused", err)
		}
		lease, err := AcquireLease(layout.Run, subject)
		if err != nil {
			t.Fatal("first lease refused", err)
		}
		if err := lease.Record(); err != nil {
			t.Fatal("lease record failed", err)
		}
		emit(map[string]any{"state": "READY", "pid": os.Getpid()})
		bufio.NewReader(os.Stdin).ReadString('\n')
		if err := lease.Release(); err != nil {
			t.Fatal("lease release failed", err)
		}
		emit(map[string]any{"state": "PASS", "category": "lease_released"})
	case "root":
		rootMode(t, layout, subject, input.Root, input.State, input.Bootstrap)
	default:
		t.Fatal("unknown probe mode")
	}
}

// workerGates runs the privilege gates in the order the worker establishes
// them: no-new-privs first, because it has to be irreversible before anything
// else runs, then the dedicated subject, the private layout and the
// single-instance lease. It returns the lease, or the category of the gate that
// refused. Every path that touches the managed root goes through it, so a root
// can never be created, opened or recovered while a gate is still open, and the
// refusal is observable instead of silent.
//
// No-new-privs is required, not set. Setting it from inside the process only
// covers the calling thread, and a Go worker already has other threads by the
// time any test code runs, so a worker cannot repair a launcher that left it
// out. The launcher has to establish it before the process starts.
func workerGates(t *testing.T, layout Layout, subject Subject) (*Lease, string) {
	t.Helper()
	if err := RequireNoNewPrivs(); err != nil {
		if !errors.Is(err, ErrIsolation) {
			t.Fatal("no-new-privs check unavailable", err)
		}
		return nil, "no_new_privs_required"
	}
	if err := RequireSubject(subject); err != nil {
		if !errors.Is(err, ErrIsolation) {
			t.Fatal("subject check unavailable", err)
		}
		return nil, "subject_refused"
	}
	if err := RequireLayout(layout, subject); err != nil {
		if !errors.Is(err, ErrIsolation) {
			t.Fatal("layout check unavailable", err)
		}
		return nil, "layout_refused"
	}
	lease, err := AcquireLease(layout.Run, subject)
	if err != nil {
		if !errors.Is(err, ErrBusy) {
			t.Fatal("lease unavailable", err)
		}
		return nil, "duplicate_refused"
	}
	return lease, ""
}

func serve(t *testing.T, layout Layout, subject, caller Subject) {
	t.Helper()
	lease, refused := workerGates(t, layout, subject)
	if refused != "" {
		t.Fatal("serve gate refused", refused)
	}
	defer func() { _ = lease.Release() }()
	if err := lease.Record(); err != nil {
		t.Fatal("lease record failed", err)
	}
	listener, err := Listen(layout, subject, []Subject{caller})
	if err != nil {
		t.Fatal("listen refused", err)
	}
	defer listener.Close()
	emit(map[string]any{"state": "READY", "socket": listener.Addr(), "pid": os.Getpid()})
	// Fixed order: the unauthorized peer is attempted first by the driver.
	outcomes := []string{}
	for i := 0; i < 2; i++ {
		conn, err := listener.Accept()
		// Only a kernel-attested refusal counts as "unauthorized". An accept
		// error means the peer never arrived, or the listener stopped working,
		// and recording that as a refusal would let this case pass without any
		// peer credential ever having been examined.
		if errors.Is(err, ErrUnauthorized) {
			outcomes = append(outcomes, "unauthorized")
			continue
		}
		if err != nil {
			t.Fatal("accept failed", err)
		}
		_ = conn.Close()
		outcomes = append(outcomes, "authorized")
	}
	if outcomes[0] != "unauthorized" || outcomes[1] != "authorized" {
		t.Fatal("peer authorization order wrong", outcomes)
	}
	emit(map[string]any{"state": "PASS", "category": "peer_authorization", "outcomes": outcomes})
}

// persistProof writes an identity record as the durable hand-off between
// processes, exactly the way a real bootstrap or copy-in layer would persist a
// proof for a later verifier to read. It lives in the state directory, never
// inside the managed root: an extra entry inside the root would itself be a
// recovery condition.
func persistProof(t *testing.T, state, name string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal("proof encoding failed", err)
	}
	path := filepath.Join(state, name)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal("proof persistence failed", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal("proof ownership failed", err)
	}
}

// loadRootProof reads the persisted root identity. A fresh observation must
// never replace a stored, approved identity, so every later process reads the
// record instead of re-deriving one.
func loadRootProof(t *testing.T, state string) managedfs.RootProof {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(state, "root-proof.json"))
	if err != nil {
		t.Fatal("persisted proof unavailable")
	}
	var proof managedfs.RootProof
	if json.Unmarshal(raw, &proof) != nil {
		t.Fatal("persisted proof malformed")
	}
	return proof
}

// loadObjectProof reads the persisted object identity: the record a real
// copy-in layer would hand a verifier. Verification is therefore exercised
// against stored evidence, never against a value the verifier just computed
// itself.
func loadObjectProof(t *testing.T, state string) managedfs.ObjectProof {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(state, "object-proof.json"))
	if err != nil {
		t.Fatal("persisted object proof unavailable")
	}
	var proof managedfs.ObjectProof
	if json.Unmarshal(raw, &proof) != nil {
		t.Fatal("persisted object proof malformed")
	}
	return proof
}

// rootMode proves the worker can own a managed root under the dedicated
// identity, and that an incomplete root is refused rather than adopted.
//
// The persisted proof lives in the state directory, never inside the managed
// root: an extra entry inside the root would itself be a recovery condition.
//
// The gates run before anything else, so a worker that is not the dedicated
// subject, or a second worker while one holds the lease, is refused before the
// root is created or opened.
func rootMode(t *testing.T, layout Layout, subject Subject, root, state string, bootstrap bool) {
	t.Helper()
	lease, refused := workerGates(t, layout, subject)
	if refused != "" {
		emit(map[string]any{"state": "PASS", "category": refused})
		return
	}
	defer func() { _ = lease.Release() }()
	ctx := context.Background()
	if bootstrap {
		proof, err := managedfs.Initialize(ctx, root)
		if err != nil {
			t.Fatal("bootstrap refused", err)
		}
		persistProof(t, state, "root-proof.json", proof)
		emit(map[string]any{"state": "PASS", "category": "bootstrap", "root": proof})
		return
	}
	proof := loadRootProof(t, state)
	reader, err := managedfs.OpenReader(root, proof)
	if err != nil {
		// A partial root is refused before any object operation is attempted.
		emit(map[string]any{"state": "PASS", "category": "incomplete_refused", "stage": "reader"})
		return
	}
	if err := reader.Close(); err != nil {
		t.Fatal("reader close failed", err)
	}
	w, err := managedfs.OpenWriter(ctx, root, proof)
	if err != nil {
		// A pending root, a broken catalog or a stray root entry lands here.
		emit(map[string]any{"state": "PASS", "category": "incomplete_refused", "stage": "writer"})
		return
	}
	if err := w.Close(); err != nil {
		t.Fatal("writer close failed", err)
	}
	emit(map[string]any{"state": "PASS", "category": "ready_root"})
}

// blockedCopy announces entry from inside SealCopy's read loop and then never
// returns. Entry happens after the root Pending marker and the catalog intent
// are durable, so a kill at that point genuinely leaves the root uncertain.
type blockedCopy struct {
	entered chan struct{}
	once    sync.Once
	block   chan struct{}
}

func (b *blockedCopy) Read([]byte) (int, error) {
	b.once.Do(func() { close(b.entered) })
	<-b.block
	return 0, io.EOF
}

func pendingMode(t *testing.T, layout Layout, subject Subject, root, state string, bootstrap bool, auth string) {
	t.Helper()
	lease, refused := workerGates(t, layout, subject)
	if refused != "" {
		emit(map[string]any{"state": "PASS", "category": refused})
		return
	}
	defer func() { _ = lease.Release() }()
	ctx := context.Background()
	if bootstrap {
		proof, err := managedfs.Initialize(ctx, root)
		if err != nil {
			t.Fatal("bootstrap refused", err)
		}
		persistProof(t, state, "root-proof.json", proof)
	}
	proof := loadRootProof(t, state)
	w, err := managedfs.OpenWriter(ctx, root, proof)
	if err != nil {
		t.Fatal("writer refused", err)
	}
	source := &blockedCopy{entered: make(chan struct{}), block: make(chan struct{})}
	if auth == "" {
		auth = strings.Repeat("b", 64)
	}
	digest := sha256.Sum256([]byte("synthetic pending input"))
	spec := managedfs.CopySpec{
		Size:                int64(len("synthetic pending input")),
		SHA256:              hex.EncodeToString(digest[:]),
		AuthorizationSHA256: auth,
	}
	go func() {
		_, _ = w.SealCopy(ctx, source, spec)
	}()
	<-source.entered
	emit(map[string]any{"state": "READY", "pid": os.Getpid()})
	// Stay alive with the lease and the pending root until the driver kills us.
	<-source.block
}

// sealRefusedMode proves a malformed approval digest is refused before anything
// is written. The digest is the caller-supplied decision record, so a request
// that cannot even name one must not reach the copy. After the refusals the root
// must still open as Ready: a refusal that had left a pending intent would turn
// an audit failure into a recovery condition, and the next worker would have to
// refuse a root that was never actually operated on.
func sealRefusedMode(t *testing.T, layout Layout, subject Subject, root, state string) {
	t.Helper()
	lease, refused := workerGates(t, layout, subject)
	if refused != "" {
		emit(map[string]any{"state": "PASS", "category": refused})
		return
	}
	defer func() { _ = lease.Release() }()
	ctx := context.Background()
	proof := loadRootProof(t, state)
	w, err := managedfs.OpenWriter(ctx, root, proof)
	if err != nil {
		t.Fatal("writer refused", err)
	}
	payload := "synthetic refused approval input"
	digest := sha256.Sum256([]byte(payload))
	for _, auth := range []string{"", "not-a-digest", strings.Repeat("z", 64), strings.Repeat("a", 63)} {
		spec := managedfs.CopySpec{
			Size:                int64(len(payload)),
			SHA256:              hex.EncodeToString(digest[:]),
			AuthorizationSHA256: auth,
		}
		if _, err := w.SealCopy(ctx, strings.NewReader(payload), spec); !errors.Is(err, managedfs.ErrRejected) {
			t.Fatal("malformed approval digest accepted", auth, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal("writer close failed", err)
	}
	// Nothing durable happened, so the root is still Ready rather than pending.
	fresh, err := managedfs.OpenWriter(ctx, root, proof)
	if err != nil {
		t.Fatal("refused copies left the root unready", err)
	}
	if err := fresh.Close(); err != nil {
		t.Fatal("second writer close failed", err)
	}
	emit(map[string]any{"state": "PASS", "category": "malformed_authorization_refused"})
}

// sealMode seals one synthetic object under a well-formed approval digest and
// persists the returned proof as the durable object-identity record, the same
// hand-off a real copy-in layer would make to a verifier. The digest is well
// formed on purpose: this mode exists to produce a real object with a stored
// identity, not to test the digest boundary, which sealRefusedMode covers.
func sealMode(t *testing.T, layout Layout, subject Subject, root, state string) {
	t.Helper()
	lease, refused := workerGates(t, layout, subject)
	if refused != "" {
		emit(map[string]any{"state": "PASS", "category": refused})
		return
	}
	defer func() { _ = lease.Release() }()
	ctx := context.Background()
	proof := loadRootProof(t, state)
	w, err := managedfs.OpenWriter(ctx, root, proof)
	if err != nil {
		t.Fatal("writer refused", err)
	}
	payload := "synthetic sealed object for identity verification"
	digest := sha256.Sum256([]byte(payload))
	object, err := w.SealCopy(ctx, strings.NewReader(payload), managedfs.CopySpec{
		Size:                int64(len(payload)),
		SHA256:              hex.EncodeToString(digest[:]),
		AuthorizationSHA256: strings.Repeat("c", 64),
	})
	if err != nil {
		t.Fatal("synthetic seal refused", err)
	}
	if err := w.Close(); err != nil {
		t.Fatal("writer close failed", err)
	}
	persistProof(t, state, "object-proof.json", object)
	emit(map[string]any{"state": "PASS", "category": "sealed",
		"object_id": object.ObjectID, "sha256": object.SHA256, "size": object.Size})
}

// readerMode is the positive control for object identity: the stored proof is
// accepted. It has to exist on its own, because a verifier that refused
// everything would otherwise let every refusal case below pass while proving
// nothing.
func readerMode(t *testing.T, layout Layout, subject Subject, root, state string) {
	t.Helper()
	lease, refused := workerGates(t, layout, subject)
	if refused != "" {
		emit(map[string]any{"state": "PASS", "category": refused})
		return
	}
	defer func() { _ = lease.Release() }()
	ctx := context.Background()
	proof := loadRootProof(t, state)
	object := loadObjectProof(t, state)
	r, err := managedfs.OpenReader(root, proof)
	if err != nil {
		t.Fatal("reader refused a ready root", err)
	}
	if err := r.Inspect(ctx, object); err != nil {
		t.Fatal("stored object proof rejected", err)
	}
	if err := r.Close(); err != nil {
		t.Fatal("reader close failed", err)
	}
	emit(map[string]any{"state": "PASS", "category": "object_identity_accepted",
		"outcomes": []string{"accepted"}})
}

// readerRefusedMode tampers with one field of the stored proof at a time. Each
// variant keeps the shape the verifier accepts -- the digest stays opaque, the
// size stays in range, the object ID stays a well-formed name -- so what
// refuses them is the identity comparison, not request validation. The
// untampered proof is inspected before and after the tampering: before, so the
// refusals cannot pass by refusing everything; after, so a refusal that had
// modified the object would be caught by the verifier's own read-only
// guarantee.
func readerRefusedMode(t *testing.T, layout Layout, subject Subject, root, state string) {
	t.Helper()
	lease, refused := workerGates(t, layout, subject)
	if refused != "" {
		emit(map[string]any{"state": "PASS", "category": refused})
		return
	}
	defer func() { _ = lease.Release() }()
	ctx := context.Background()
	proof := loadRootProof(t, state)
	stored := loadObjectProof(t, state)
	r, err := managedfs.OpenReader(root, proof)
	if err != nil {
		t.Fatal("reader refused a ready root", err)
	}
	outcomes := []string{}
	if err := r.Inspect(ctx, stored); err != nil {
		t.Fatal("stored object proof rejected before tampering", err)
	}
	outcomes = append(outcomes, "accepted")
	for _, variant := range []struct {
		name   string
		mutate func(*managedfs.ObjectProof)
	}{
		{"digest", func(p *managedfs.ObjectProof) { p.SHA256 = strings.Repeat("0", 64) }},
		{"size", func(p *managedfs.ObjectProof) { p.Size++ }},
		{"inode", func(p *managedfs.ObjectProof) { p.Inode++ }},
		{"root", func(p *managedfs.ObjectProof) { p.Root.StoreID = strings.Repeat("d", 64) }},
		{"object_id", func(p *managedfs.ObjectProof) { p.ObjectID = strings.Repeat("e", 64) }},
	} {
		tampered := stored
		variant.mutate(&tampered)
		if err := r.Inspect(ctx, tampered); !errors.Is(err, managedfs.ErrRejected) {
			t.Fatal("tampered object proof accepted", variant.name, err)
		}
		outcomes = append(outcomes, variant.name+"_refused")
	}
	if err := r.Inspect(ctx, stored); err != nil {
		t.Fatal("refusals modified the object or its identity", err)
	}
	outcomes = append(outcomes, "accepted_again")
	if err := r.Close(); err != nil {
		t.Fatal("reader close failed", err)
	}
	emit(map[string]any{"state": "PASS", "category": "object_identity_refused",
		"outcomes": outcomes})
}

func emit(value map[string]any) {
	_ = json.NewEncoder(os.Stdout).Encode(value)
}

// dispatchMode proves that an authorized caller still cannot reach SEAL, PURGE
// or INSPECT. The peer identity was already established by Accept, so this
// connection is as privileged as a connection can get, and each request is
// complete and well formed: what refuses them is authorization, not validation.
// Reaching the socket is therefore never the same permission as deleting an
// object.
func dispatchMode(t *testing.T, layout Layout, subject, caller Subject, ops []string) {
	t.Helper()
	if len(ops) == 0 {
		t.Fatal("no operations requested")
	}
	lease, refused := workerGates(t, layout, subject)
	if refused != "" {
		t.Fatal("dispatch gate refused", refused)
	}
	defer func() { _ = lease.Release() }()
	listener, err := Listen(layout, subject, []Subject{caller})
	if err != nil {
		t.Fatal("listen refused", err)
	}
	defer listener.Close()
	emit(map[string]any{"state": "READY", "socket": listener.Addr(), "pid": os.Getpid()})
	conn, err := listener.Accept()
	if err != nil {
		t.Fatal("accept refused", err)
	}
	defer conn.Close()
	outcomes := []string{}
	for range ops {
		request, err := conn.ReadRequest()
		if err != nil {
			t.Fatal("request refused", err)
		}
		response, err := Dispatch(context.Background(), request)
		if err != nil {
			outcomes = append(outcomes, request.Op+":refused")
		} else if response == nil {
			outcomes = append(outcomes, request.Op+":empty")
		} else {
			outcomes = append(outcomes, request.Op+":executed")
		}
		raw, err := json.Marshal(map[string]any{"ok": err == nil, "op": request.Op})
		if err != nil {
			t.Fatal("response encoding failed", err)
		}
		if err := conn.WriteResponse(raw); err != nil {
			t.Fatal("response write failed", err)
		}
	}
	if Executable()[0] != "PING" || len(Executable()) != 2 {
		t.Fatal("executable operations changed", Executable())
	}
	emit(map[string]any{"state": "PASS", "category": "dispatch", "outcomes": outcomes,
		"executable": Executable()})
}

// sendMode runs inside the authorized peer identity and sends each operation as
// a real framed request, reading the answer back with the same bound the
// product enforces so the client cannot be the thing that accepts an oversized
// response.
func sendMode(t *testing.T, socket string, ops []string) {
	t.Helper()
	conn, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatal("dial failed", err)
	}
	defer conn.Close()
	client := &Conn{conn: conn.(*net.UnixConn)}
	outcomes := []string{}
	for _, op := range ops {
		body, err := json.Marshal(map[string]string{"op": op})
		if err != nil {
			t.Fatal("request encoding failed", err)
		}
		raw, err := frame(body)
		if err != nil {
			t.Fatal("frame failed", err)
		}
		if _, err := client.conn.Write(raw); err != nil {
			t.Fatal("write failed", err)
		}
		var header [4]byte
		if _, err := io.ReadFull(client.conn, header[:]); err != nil {
			t.Fatal("response read failed", err)
		}
		size := binary.BigEndian.Uint32(header[:])
		if size == 0 || size > maxRequestBytes {
			t.Fatal("response bound", size)
		}
		answer := make([]byte, size)
		if _, err := io.ReadFull(client.conn, answer); err != nil {
			t.Fatal("response read failed", err)
		}
		outcomes = append(outcomes, op+":"+string(answer))
	}
	emit(map[string]any{"state": "PASS", "category": "sent", "outcomes": outcomes})
}
