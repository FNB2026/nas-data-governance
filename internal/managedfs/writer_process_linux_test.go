//go:build linux

package managedfs

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

type writerProbeConfig struct {
	Area     string      `json:"area"`
	Mode     string      `json:"mode"`
	Root     RootProof   `json:"root"`
	Target   ObjectProof `json:"target"`
	Keeper   ObjectProof `json:"keeper"`
	StopAt   string      `json:"stop_at"`
	Expected string      `json:"expected"`
}

// Only the disposable controller supplies this test configuration. There is
// no corresponding environment switch in a product constructor or binary.
func TestManagedWriterKernelProcess(t *testing.T) {
	input := os.Getenv("NDG_MANAGED_WRITER_PROBE")
	if input == "" {
		t.Skip("disposable native controller only")
	}
	if os.Geteuid() == 0 || os.Getegid() == 0 {
		t.Fatal("non-root child required")
	}
	raw, err := os.ReadFile(input)
	var cfg writerProbeConfig
	if err != nil || json.Unmarshal(raw, &cfg) != nil {
		t.Fatal("probe input rejected")
	}
	ctx := context.Background()
	rootPath := filepath.Join(cfg.Area, "root")
	emit := func(v any) { b, _ := json.Marshal(v); fmt.Println(string(b)) }
	if cfg.Mode == "prepare" {
		root, err := Initialize(ctx, rootPath)
		if err != nil {
			t.Fatal("probe initialize failed", err)
		}
		w, err := OpenWriter(ctx, rootPath, root)
		if err != nil {
			t.Fatal("probe writer failed", err)
		}
		target, err := w.SealCopy(ctx, strings.NewReader("synthetic target object"), copySpec([]byte("synthetic target object")))
		if err != nil {
			t.Fatal("probe target seal failed", err)
		}
		keeper, err := w.SealCopy(ctx, strings.NewReader("synthetic keeper object"), copySpec([]byte("synthetic keeper object")))
		if err != nil || w.Close() != nil {
			t.Fatal("probe keeper/close failed", err)
		}
		emit(map[string]any{"state": "PASS", "root": root, "target": target, "keeper": keeper})
		return
	}
	opened, err := OpenWriter(ctx, rootPath, cfg.Root)
	if cfg.Mode == "reopen" || cfg.Mode == "lease" {
		if cfg.Expected == "ready" {
			if err != nil || opened == nil || opened.Inspect(ctx, cfg.Keeper) != nil || opened.Close() != nil {
				t.Fatal("ready reopen failed", err)
			}
		} else {
			if opened != nil {
				_ = opened.Close()
				t.Fatal("expected constructor refusal")
			}
			if cfg.Expected == "busy" && !errors.Is(err, ErrBusy) || cfg.Expected == "recovery" && !errors.Is(err, ErrRecoveryRequired) {
				t.Fatal("wrong refusal category", err)
			}
		}
		emit(map[string]string{"state": "PASS", "category": cfg.Expected})
		return
	}
	if cfg.Expected == "constructor_refused" {
		if opened != nil {
			_ = opened.Close()
			t.Fatal("mounted constructor accepted")
		}
		if err == nil {
			t.Fatal("constructor refusal missing")
		}
		emit(map[string]string{"state": "PASS", "category": cfg.Expected})
		return
	}
	if err != nil {
		t.Fatal("probe open failed", err)
	}
	w := opened.(*writer)
	defer w.Close()
	lines := bufio.NewReader(os.Stdin)
	w.fault = func(at string) error {
		if at == cfg.StopAt {
			emit(map[string]string{"state": "BOUNDARY", "at": at})
			if line, err := lines.ReadString('\n'); err != nil || line != "continue\n" {
				return ErrRecoveryRequired
			}
		}
		return nil
	}
	emit(map[string]string{"state": "READY"})
	if line, err := lines.ReadString('\n'); err != nil || line != "execute\n" {
		t.Fatal("probe handshake failed")
	}
	if cfg.Mode == "seal" {
		// Artificial source is opened read-only; the primitive receives only
		// the caller-owned reader. It never receives a source unlink path.
		fd, e := unix.Open(filepath.Join(cfg.Area, "source"), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if e != nil {
			t.Fatal("probe source failed")
		}
		f := os.NewFile(uintptr(fd), "synthetic source")
		_, err = w.SealCopy(ctx, f, copySpec([]byte("synthetic copy input")))
		_ = f.Close()
	} else {
		err = w.Purge(ctx, cfg.Target, strings.Repeat("f", 64))
	}
	if cfg.Expected == "success" && err != nil || cfg.Expected == "refused" && err == nil {
		t.Fatal("wrong execution result", err)
	}
	if w.Close() != nil {
		t.Fatal("probe close failed")
	}
	emit(map[string]string{"state": "PASS", "category": cfg.Expected})
}
