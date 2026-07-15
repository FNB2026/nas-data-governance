package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiagnosePathsWritesPrivateNonExecutableReport(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "不存在.wav")
	legacy := filepath.Join(t.TempDir(), "legacy.log")
	line := "warning: hash error: open " + missing + ": no such file or directory\n"
	if err := os.WriteFile(legacy, []byte(line), privateFileMode); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "path-review.json")
	if err := runDiagnosePaths([]string{"--root", root, "--legacy-log", legacy, "--out", out}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(out)
	if err != nil || info.Mode().Perm() != privateFileMode {
		t.Fatalf("private report mode: info=%v err=%v", info, err)
	}
}

func TestReadLegacyHashErrorPathsRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.log")
	if err := os.WriteFile(real, []byte("x"), privateFileMode); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.log")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readLegacyHashErrorPaths(link); err == nil {
		t.Fatal("expected symlink input to be rejected")
	}
}

func TestDiagnosePathsRequiresLegacyOrManifest(t *testing.T) {
	root := t.TempDir()
	if err := runDiagnosePaths([]string{"--root", root}); err == nil {
		t.Fatal("expected error when neither --legacy-log nor --failures-manifest is given")
	}
}

func TestDiagnosePathsRejectsBothLegacyAndManifest(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(t.TempDir(), "legacy.log")
	if err := os.WriteFile(legacy, []byte("x"), privateFileMode); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(t.TempDir(), "failures.jsonl")
	if err := os.WriteFile(manifest, []byte("{}"), privateFileMode); err != nil {
		t.Fatal(err)
	}
	if err := runDiagnosePaths([]string{
		"--root", root, "--legacy-log", legacy, "--failures-manifest", manifest,
	}); err == nil {
		t.Fatal("expected error when both --legacy-log and --failures-manifest are given")
	}
}

func TestReadHashFailureManifestPathsParsesJSONL(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "failures.jsonl")
	// Two unique path entries, one duplicate, and one without a path.
	content := `{"id":"a","stage":"quick","path":"/tmp/a.txt","size":10}
{"id":"b","stage":"quick","path":"/tmp/b.txt","size":20}
{"id":"a-again","stage":"quick","path":"/tmp/a.txt","size":10}
{"id":"c","stage":"quick","size":30}
`
	if err := os.WriteFile(manifest, []byte(content), privateFileMode); err != nil {
		t.Fatal(err)
	}
	paths, err := readHashFailureManifestPaths(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths (entry without path skipped), got %d", len(paths))
	}
}

func TestReadHashFailureManifestPathsRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.jsonl")
	if err := os.WriteFile(real, []byte(`{"path":"/x"}`), privateFileMode); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.jsonl")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readHashFailureManifestPaths(link); err == nil {
		t.Fatal("expected symlink input to be rejected")
	}
}

func TestReadHashFailureManifestPathsRejectsEmpty(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "empty.jsonl")
	// Entries exist but none carry a path.
	if err := os.WriteFile(manifest, []byte(`{"id":"x","size":1}`+"\n"), privateFileMode); err != nil {
		t.Fatal(err)
	}
	if _, err := readHashFailureManifestPaths(manifest); err == nil {
		t.Fatal("expected error when no path entries are present")
	}
}

func TestDiagnosePathsUsesFailuresManifest(t *testing.T) {
	root := t.TempDir()
	// Create a missing-file scenario so the report has one candidate.
	missing := filepath.Join(root, "missing.txt")
	manifest := filepath.Join(t.TempDir(), "failures.jsonl")
	content := `{"id":"a","stage":"quick","path":"` + missing + `","size":10}` + "\n"
	if err := os.WriteFile(manifest, []byte(content), privateFileMode); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "path-review.json")
	if err := runDiagnosePaths([]string{
		"--root", root, "--failures-manifest", manifest, "--out", out,
	}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(out)
	if err != nil || info.Mode().Perm() != privateFileMode {
		t.Fatalf("private report mode: info=%v err=%v", info, err)
	}
}
