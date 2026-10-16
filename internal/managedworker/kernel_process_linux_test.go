//go:build linux

package managedworker

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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
		Mode      string `json:"mode"`
		Run       string `json:"run"`
		State     string `json:"state"`
		Root      string `json:"root"`
		Bootstrap bool   `json:"bootstrap"`
		WorkerUID uint32 `json:"worker_uid"`
		WorkerGID uint32 `json:"worker_gid"`
		CallerUID uint32 `json:"caller_uid"`
		CallerGID uint32 `json:"caller_gid"`
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
		rootMode(t, input.Root, input.State, input.Bootstrap)
	default:
		t.Fatal("unknown probe mode")
	}
}

func serve(t *testing.T, layout Layout, subject, caller Subject) {
	t.Helper()
	if err := SetNoNewPrivs(); err != nil {
		t.Fatal("no-new-privs unavailable", err)
	}
	if err := RequireSubject(subject); err != nil {
		t.Fatal("subject refused", err)
	}
	if err := RequireLayout(layout, subject); err != nil {
		t.Fatal("layout refused", err)
	}
	lease, err := AcquireLease(layout.Run, subject)
	if err != nil {
		t.Fatal("lease refused", err)
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
		if err != nil {
			outcomes = append(outcomes, "unauthorized")
			continue
		}
		_ = conn.Close()
		outcomes = append(outcomes, "authorized")
	}
	if outcomes[0] != "unauthorized" || outcomes[1] != "authorized" {
		t.Fatal("peer authorization order wrong", outcomes)
	}
	emit(map[string]any{"state": "PASS", "category": "peer_authorization", "outcomes": outcomes})
}

// rootMode proves the worker can own a managed root under the dedicated
// identity, and that an incomplete root is refused rather than adopted.
//
// The persisted proof lives in the state directory, never inside the managed
// root: an extra entry inside the root would itself be a recovery condition.
func rootMode(t *testing.T, root, state string, bootstrap bool) {
	t.Helper()
	ctx := context.Background()
	proofPath := filepath.Join(state, "root-proof.json")
	if bootstrap {
		proof, err := managedfs.Initialize(ctx, root)
		if err != nil {
			t.Fatal("bootstrap refused", err)
		}
		raw, err := json.Marshal(proof)
		if err != nil {
			t.Fatal("proof encoding failed", err)
		}
		if err := os.WriteFile(proofPath, raw, 0o600); err != nil {
			t.Fatal("proof persistence failed", err)
		}
		if err := os.Chmod(proofPath, 0o600); err != nil {
			t.Fatal("proof ownership failed", err)
		}
		emit(map[string]any{"state": "PASS", "category": "bootstrap", "root": proof})
		return
	}
	raw, err := os.ReadFile(proofPath)
	if err != nil {
		t.Fatal("persisted proof unavailable", err)
	}
	var proof managedfs.RootProof
	if json.Unmarshal(raw, &proof) != nil {
		t.Fatal("persisted proof malformed")
	}
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

func emit(value map[string]any) {
	_ = json.NewEncoder(os.Stdout).Encode(value)
}
