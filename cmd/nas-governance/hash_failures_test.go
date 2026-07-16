package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	idx "github.com/FNB2026/nas-data-governance/internal/index"
)

func TestHashWithRetryRecoversAfterTransientFailures(t *testing.T) {
	calls := 0
	value, attempts, err := hashWithRetry(context.Background(), "/redacted", 1, 3, 0, func(string, int64) (string, error) {
		calls++
		if calls < 3 {
			return "", errors.New("transient")
		}
		return "hash", nil
	})
	if err != nil || value != "hash" || attempts != 3 {
		t.Fatalf("value=%q attempts=%d err=%v", value, attempts, err)
	}
}

func TestScanRetainsUnfingerprintedRecordAndWritesPrivateManifest(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "root")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sensitive-name.txt"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(tmp, "index.jsonl")
	failuresPath := filepath.Join(tmp, "private", "failures.jsonl")

	original := quickHash
	quickHash = func(string, int64) (string, error) { return "", errors.New("unavailable") }
	defer func() { quickHash = original }()

	if err := runScan([]string{
		"--root", root, "--out", indexPath, "--storage", "test",
		"--hash-attempts", "2", "--hash-retry-delay", "0",
		"--hash-failures-out", failuresPath,
	}); err != nil {
		t.Fatalf("scan: %v", err)
	}
	files, err := idx.Read(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].QuickHash != "" {
		t.Fatalf("failed file must remain in index without quick hash: %+v", files)
	}
	failures, err := readHashFailures(failuresPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 1 || failures[0].Attempts != 2 || failures[0].Stage != "quick" {
		t.Fatalf("unexpected failures: %+v", failures)
	}
	info, err := os.Stat(failuresPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != privateFileMode {
		t.Fatalf("manifest mode=%o, want %o", info.Mode().Perm(), privateFileMode)
	}
}

func TestRetryHashesRecoversToNewIndex(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "root")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "file.bin")
	if err := os.WriteFile(path, []byte("recoverable"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	device, inode := fileIdentity(info)
	file := domain.FileInstance{
		StorageID: "test", Path: path, Name: info.Name(), Size: info.Size(),
		Mode: uint32(info.Mode()), ModifiedAt: info.ModTime(), Device: device,
		Inode: inode, DiscoveredAt: time.Now().UTC(),
	}
	indexPath := filepath.Join(tmp, "index.jsonl")
	outPath := filepath.Join(tmp, "recovered.jsonl")
	failuresPath := filepath.Join(tmp, "failures.jsonl")
	remainingPath := filepath.Join(tmp, "remaining.jsonl")
	if err := idx.Write(indexPath, []domain.FileInstance{file}); err != nil {
		t.Fatal(err)
	}
	if err := writeHashFailures(failuresPath, []hashFailure{newHashFailure(file, "quick", 3, "hash_failed")}); err != nil {
		t.Fatal(err)
	}
	if err := runRetryHashes([]string{
		"--root", root, "--failures", failuresPath, "--index", indexPath,
		"--out", outPath, "--remaining-out", remainingPath, "--retry-delay", "0",
	}); err != nil {
		t.Fatalf("retry: %v", err)
	}
	files, err := idx.Read(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].QuickHash == "" {
		t.Fatalf("recovered hash missing: %+v", files)
	}
	remaining, err := readHashFailures(remainingPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("unexpected unresolved entries: %+v", remaining)
	}
	if _, err := os.Stat(indexPath); err != nil {
		t.Fatalf("source index changed or removed: %v", err)
	}
}

func TestRetryHashesCompletesFullHashForRecoveredCandidate(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "root")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("same duplicate content")
	paths := []string{filepath.Join(root, "one.bin"), filepath.Join(root, "two.bin")}
	files := make([]domain.FileInstance, 2)
	for i, path := range paths {
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		device, inode := fileIdentity(info)
		files[i] = domain.FileInstance{
			StorageID: "test", Path: path, Name: info.Name(), Size: info.Size(),
			Mode: uint32(info.Mode()), ModifiedAt: info.ModTime(), Device: device,
			Inode: inode, DiscoveredAt: time.Now().UTC(),
		}
	}
	quick, err := quickHash(paths[1], files[1].Size)
	if err != nil {
		t.Fatal(err)
	}
	full, err := fullHash(paths[1], files[1].Size)
	if err != nil {
		t.Fatal(err)
	}
	files[1].QuickHash = quick
	files[1].ContentSHA256 = full

	indexPath := filepath.Join(tmp, "index.jsonl")
	outPath := filepath.Join(tmp, "recovered.jsonl")
	failuresPath := filepath.Join(tmp, "failures.jsonl")
	if err := idx.Write(indexPath, files); err != nil {
		t.Fatal(err)
	}
	if err := writeHashFailures(failuresPath, []hashFailure{newHashFailure(files[0], "quick", 3, "hash_failed")}); err != nil {
		t.Fatal(err)
	}
	if err := runRetryHashes([]string{
		"--root", root, "--failures", failuresPath, "--index", indexPath,
		"--out", outPath, "--retry-delay", "0",
	}); err != nil {
		t.Fatal(err)
	}
	recovered, err := idx.Read(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if recovered[0].QuickHash != quick || recovered[0].ContentSHA256 != full {
		t.Fatalf("progressive hashes not completed: %+v", recovered[0])
	}
}

func TestIncrementalScanRetriesCachedRecordWithoutQuickHash(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "root")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "file.bin")
	if err := os.WriteFile(path, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	device, inode := fileIdentity(info)
	file := domain.FileInstance{
		StorageID: "test", Path: path, Name: info.Name(), Size: info.Size(),
		Mode: uint32(info.Mode()), ModifiedAt: info.ModTime(), Device: device,
		Inode: inode, DiscoveredAt: time.Now().UTC(),
	}
	seedIndex := filepath.Join(tmp, "seed.jsonl")
	dbPath := filepath.Join(tmp, "governance.db")
	if err := idx.Write(seedIndex, []domain.FileInstance{file}); err != nil {
		t.Fatal(err)
	}
	if err := runImportIndex([]string{"--index", seedIndex, "--db", dbPath}); err != nil {
		t.Fatal(err)
	}

	original := quickHash
	calls := 0
	quickHash = func(path string, size int64) (string, error) {
		calls++
		return original(path, size)
	}
	defer func() { quickHash = original }()
	if err := runScan([]string{
		"--root", root, "--out", filepath.Join(tmp, "rescanned.jsonl"),
		"--storage", "test", "--db", dbPath, "--hash-retry-delay", "0",
	}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("quick hash calls=%d, want 1", calls)
	}
}

func TestRetryHashesRejectsStaleAndUnsafeTargets(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "root")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "changed.bin")
	if err := os.WriteFile(path, []byte("new content"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	device, inode := fileIdentity(info)
	failure := hashFailure{
		ID: "id", Stage: "quick", StorageID: "test", Path: path,
		Size: info.Size() + 1, ModifiedAt: info.ModTime(), Device: device, Inode: inode,
	}
	if _, status := validateRetryTarget(root, device, failure); status != "stale" {
		t.Fatalf("status=%q, want stale", status)
	}

	outside := failure
	outside.Path = filepath.Join(tmp, "outside.bin")
	if _, status := validateRetryTarget(root, device, outside); status != "outside_root" {
		t.Fatalf("status=%q, want outside_root", status)
	}

	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	symlinkFailure := failure
	symlinkFailure.Path = link
	if _, status := validateRetryTarget(root, device, symlinkFailure); status != "symlink_rejected" {
		t.Fatalf("status=%q, want symlink_rejected", status)
	}
}

func TestRetryHashesRequiresSeparateOutput(t *testing.T) {
	err := runRetryHashes([]string{
		"--root", t.TempDir(), "--failures", "f", "--index", "same", "--out", "same",
	})
	if err == nil {
		t.Fatal("expected source overwrite rejection")
	}
}
