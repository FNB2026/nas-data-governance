package store

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

func TestSourcePurgeFormatBindingRejectsReplacedRescannedAndUnknown(t *testing.T) {
	base := SourcePurgeFile{SHA256: strings.Repeat("a", 64), MTime: "mtime", Discovered: "scan1", Device: 3, Inode: 4, Size: 17}
	binding := FormatBinding{base.SHA256, base.MTime, base.Discovered, base.Device, base.Inode, base.Size}
	for _, tc := range []struct {
		name   string
		change func(*SourcePurgeFile, *persistedFormat)
		valid  bool
	}{
		{"fresh", func(*SourcePurgeFile, *persistedFormat) {}, true},
		{"rescan", func(f *SourcePurgeFile, _ *persistedFormat) { f.Discovered = "scan2" }, false},
		{"replacement", func(f *SourcePurgeFile, _ *persistedFormat) { f.Inode++ }, false},
		{"format changed", func(f *SourcePurgeFile, _ *persistedFormat) { f.SHA256 = strings.Repeat("b", 64) }, false},
		{"unknown", func(_ *SourcePurgeFile, p *persistedFormat) { p.Format = "unknown" }, false},
		{"legacy", func(_ *SourcePurgeFile, p *persistedFormat) { p.Binding = nil }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := base
			meta := persistedFormat{FormatInfo: domain.FormatInfo{Format: "pdf", Category: domain.CategoryDocument}, Binding: &binding}
			tc.change(&file, &meta)
			data, err := json.Marshal(meta)
			if err != nil {
				t.Fatal(err)
			}
			file.Format = string(data)
			if _, valid := sourceFormat(file); valid != tc.valid {
				t.Fatal("unexpected proof verdict", valid)
			}
		})
	}
}

func TestPurgeCandidateMigrationRollback(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "project.db")
	old, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := old.RegisterStorage(ctx, domain.Storage{ID: "test", RootPath: dir, Kind: "filesystem", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if layout, err := schemaLayoutFingerprint(ctx, old.db); err != nil || layout != authorizationAudit13Layout {
		t.Fatal("ordinary open migrated purge schema", err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := OpenPurgeCandidate(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if layout, err := schemaLayoutFingerprint(ctx, upgraded.db); err != nil || layout != dualPurge14Layout {
		t.Fatal("explicit upgrade failed", err)
	}
	if err := upgraded.Close(); err != nil {
		t.Fatal(err)
	}
	backup := path + ".before-schema014"
	if err := verifyPurgeBackup(ctx, backup); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(dir, "restored.db")
	if err := os.WriteFile(restored, data, 0600); err != nil {
		t.Fatal(err)
	}
	if baseline := os.Getenv("NDG_R1_BASE_CLI"); baseline != "" {
		if output, err := exec.Command(baseline, "quarantine", "list", "--db", path).CombinedOutput(); err == nil {
			t.Fatal("baseline accepted unsupported migrated database", string(output))
		}
		if output, err := exec.Command(baseline, "quarantine", "list", "--db", restored).CombinedOutput(); err != nil {
			t.Fatal("baseline refused restored backup", err, string(output))
		}
	}
	reopened, err := Open(ctx, restored)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var count int
	if err := reopened.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM storages WHERE id='test'").Scan(&count); err != nil || count != 1 {
		t.Fatal("backup lost project row", err)
	}
}
