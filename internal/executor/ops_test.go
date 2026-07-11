package executor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyFileCreatesIdenticalDestination(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src.txt")
	dst := filepath.Join(t.TempDir(), "sub", "dst.txt")
	if err := os.WriteFile(src, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := CopyFile(src, dst)
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	if n != 11 {
		t.Fatalf("expected 11 bytes, got %d", n)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello world" {
		t.Fatalf("content mismatch: %q", got)
	}
}

func TestCopyFilePreservesPermissions(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	dst := filepath.Join(t.TempDir(), "dst")
	if err := os.WriteFile(src, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CopyFile(src, dst); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600, got %o", info.Mode().Perm())
	}
}

func TestCopyFileRefusesToOverwrite(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	dst := filepath.Join(t.TempDir(), "dst")
	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := CopyFile(src, dst)
	if err != ErrDestinationExists {
		t.Fatalf("expected ErrDestinationExists, got %v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "old" {
		t.Fatalf("destination should be unchanged, got %q", got)
	}
}

func TestCopyFileRefusesSymlink(t *testing.T) {
	src := filepath.Join(t.TempDir(), "real")
	link := filepath.Join(t.TempDir(), "link")
	dst := filepath.Join(t.TempDir(), "dst")
	if err := os.WriteFile(src, []byte("real"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(src, link); err != nil {
		t.Fatal(err)
	}
	_, err := CopyFile(link, dst)
	if err != ErrSymlinkRefused {
		t.Fatalf("expected ErrSymlinkRefused, got %v", err)
	}
}

func TestVerifyFileMatchesExpectedHash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	snap, err := Snapshot(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyFile(path, snap.Hash); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestVerifyFileDetectsMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyFile(path, "deadbeef"); err != ErrHashMismatch {
		t.Fatalf("expected ErrHashMismatch, got %v", err)
	}
}

func TestMoveFileCopiesAndDeletesSource(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	dst := filepath.Join(t.TempDir(), "dst")
	if err := os.WriteFile(src, []byte("move me"), 0o644); err != nil {
		t.Fatal(err)
	}
	snap, err := Snapshot(src, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := MoveFile(src, dst, snap.Hash); err != nil {
		t.Fatalf("move: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source should be removed, got err=%v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "move me" {
		t.Fatalf("destination content mismatch: %q", got)
	}
}

func TestMoveFileLeavesSourceUntouchedOnHashMismatch(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	dst := filepath.Join(t.TempDir(), "dst")
	if err := os.WriteFile(src, []byte("real content"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Pass a wrong hash → verify fails → source must survive, dst removed.
	err := MoveFile(src, dst, "wronghash")
	if err == nil {
		t.Fatal("expected error from MoveFile")
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source should still exist, got err=%v", err)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatalf("destination should be removed after verify failure, got err=%v", err)
	}
}

func TestMoveFileLeavesBothOnDeleteFailure(t *testing.T) {
	// We cannot easily make os.Remove fail on a real file, so this test
	// verifies the error path indirectly: when CopyFile succeeds and
	// VerifyFile succeeds but the source is somehow removed mid-flight,
	// MoveFile returns an error. We simulate by pointing src at a path
	// that exists for copy+verify but is removed before delete.
	//
	// Since we can't intercept os.Remove, this test instead documents the
	// contract: MoveFile returns "delete source:" wrapped error when
	// removal fails. A full simulation requires a filesystem mock, which
	// is out of scope for M3b's real-filesystem tests.
	t.Skip("requires filesystem mock; covered by executor integration test")
}

func TestSafeRemoveDeletesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SafeRemove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected file removed, got err=%v", err)
	}
}

func TestSafeRemoveRefusesSymlink(t *testing.T) {
	real := filepath.Join(t.TempDir(), "real")
	link := filepath.Join(t.TempDir(), "link")
	if err := os.WriteFile(real, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if err := SafeRemove(link); err != ErrSymlinkRefused {
		t.Fatalf("expected ErrSymlinkRefused, got %v", err)
	}
	if _, err := os.Stat(real); err != nil {
		t.Fatalf("real file should still exist, got err=%v", err)
	}
}
