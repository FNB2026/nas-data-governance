package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"nas-data-governance/internal/domain"
	"nas-data-governance/internal/formatdiag"
	"nas-data-governance/internal/store"
)

func TestDiagnoseFormatsWritesPrivateOfflineReport(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "governance.db")
	ctx := context.Background()
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.RegisterStorage(ctx, domain.Storage{ID: "s", RootPath: "/offline", Kind: "test", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	files := []domain.FileInstance{{StorageID: "s", Path: "/offline/large.wav", Name: "large.wav", Size: 200 << 20, ModifiedAt: time.Now(), DiscoveredAt: time.Now()}}
	ids, err := st.UpsertFiles(ctx, files)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveFormat(ctx, ids[0], domain.FormatInfo{Format: "unknown", Category: domain.CategoryUnknown}); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(tmp, "private", "review.json")
	if err := runDiagnoseFormats([]string{"--db", dbPath, "--out", out}); err != nil {
		t.Fatalf("diagnose offline database: %v", err)
	}
	stat, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	if stat.Mode().Perm() != privateFileMode {
		t.Fatalf("mode=%o want=%o", stat.Mode().Perm(), privateFileMode)
	}
	var report formatdiag.Report
	readJSONFile(t, out, &report)
	if report.Summary.LargeUnknown != 1 || len(report.LargeUnknown) != 1 {
		t.Fatalf("unexpected report: %#v", report)
	}
}
