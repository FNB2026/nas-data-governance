package store

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWritableStoreOwnerRejectsSecondHandle(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "governance.db")
	first, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := Open(ctx, path)
	if err == nil {
		second.Close()
		t.Fatal("second project writer accepted")
	}
	ro, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal("independent read-only handle rejected", err)
	}
	ro.Close()
}

func TestStoreOwnerAcrossProcessAndCrashExit(t *testing.T) {
	if path := os.Getenv("NDG_SYNTHETIC_OWNER_DB"); path != "" {
		s, e := Open(context.Background(), path)
		deny := os.Getenv("NDG_SYNTHETIC_OWNER_DENY") == "1"
		if deny && e != nil {
			os.Exit(0)
		}
		if !deny && e == nil {
			_ = s
			os.Exit(0)
		}
		os.Exit(2)
	}
	path := filepath.Join(t.TempDir(), "governance.db")
	s, e := Open(context.Background(), path)
	if e != nil {
		t.Fatal(e)
	}
	child := func(deny bool) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestStoreOwnerAcrossProcessAndCrashExit$")
		cmd.Env = append(os.Environ(), "NDG_SYNTHETIC_OWNER_DB="+path)
		if deny {
			cmd.Env = append(cmd.Env, "NDG_SYNTHETIC_OWNER_DENY=1")
		}
		if cmd.Run() != nil {
			t.Fatal("cross-process owner invariant failed")
		}
	}
	child(true)
	if s.Close() != nil {
		t.Fatal("close")
	}
	child(false) // exits without explicit Close
	s, e = Open(context.Background(), path)
	if e != nil {
		t.Fatal("OS did not release crashed owner", e)
	}
	s.Close()
}

func TestOwnerRejectsLinkAndUnsafeModesWithoutMutation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "governance.db")
	s, e := Open(ctx, path)
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
	data, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	link := filepath.Join(filepath.Dir(path), "hardlink.db")
	if e = os.Link(path, link); e != nil {
		t.Fatal(e)
	}
	if _, e = Open(ctx, path); e == nil || strings.Contains(e.Error(), path) {
		t.Fatal("unsafe database identity accepted or disclosed")
	}
	after, _ := os.ReadFile(path)
	if string(data) != string(after) {
		t.Fatal("rejected database changed")
	}
	other := filepath.Join(t.TempDir(), "fresh.db")
	target := filepath.Join(t.TempDir(), "synthetic_anchor")
	os.WriteFile(target, []byte("preserve"), 0600)
	if e = os.Symlink(target, other+".owner.lock"); e != nil {
		t.Fatal(e)
	}
	if _, e = Open(ctx, other); e == nil {
		t.Fatal("owner symlink accepted")
	}
	after, _ = os.ReadFile(target)
	if string(after) != "preserve" {
		t.Fatal("external sidecar changed")
	}
}

func TestStoreOwnerDoesNotInterpretPercentEscapesAsAnotherFilename(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "synthetic%39.db")
	st, err := Open(context.Background(), path)
	if err != nil {
		entries, readErr := os.ReadDir(root)
		if readErr != nil || len(entries) != 0 {
			t.Fatal("URI refusal changed the directory")
		}
		return
	} // Explicit fail-closed refusal is acceptable.
	if err = st.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) < 16 || string(data[:16]) != "SQLite format 3\x00" {
		t.Fatal("SQLite filename does not match the acquired owner lease")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "synthetic%39.db" && entry.Name() != "synthetic%39.db.owner.lock" {
			t.Fatal("escaped filename opened a different database")
		}
	}
}
