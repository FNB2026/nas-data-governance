package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	idx "github.com/FNB2026/nas-data-governance/internal/index"
	"github.com/FNB2026/nas-data-governance/internal/merge"
)

func TestDiagnoseMergesWritesPrivateNonExecutableReport(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "index.jsonl")
	files := []domain.FileInstance{
		{Path: "/data/project/a", Name: "a", ModifiedAt: time.Now(), DiscoveredAt: time.Now()},
		{Path: "/data/project_backup/a", Name: "a", ModifiedAt: time.Now(), DiscoveredAt: time.Now()},
	}
	if err := idx.Write(indexPath, files); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "private.json")
	if err := runDiagnoseMerges([]string{"--index", indexPath, "--out", out}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(out)
	if err != nil || info.Mode().Perm() != privateFileMode {
		t.Fatalf("private report mode: info=%v err=%v", info, err)
	}
	var report merge.DiagnosticReport
	readJSONFile(t, out, &report)
	if report.ExecutionAuthorized || report.Summary.Suggestions != 1 {
		t.Fatalf("unexpected report: %#v", report.Summary)
	}
}

func TestDiagnoseMergesRejectsIndexSymlink(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "index.jsonl")
	if err := idx.Write(indexPath, []domain.FileInstance{{Path: "/data/a", Name: "a", ModifiedAt: time.Now(), DiscoveredAt: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(dir, "linked-index.jsonl")
	if err := os.Symlink(indexPath, linkPath); err != nil {
		t.Fatal(err)
	}
	if err := runDiagnoseMerges([]string{"--index", linkPath, "--out", filepath.Join(dir, "out.json")}); err == nil {
		t.Fatal("expected index symlink to be rejected")
	}
}
