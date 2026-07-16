package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/governancediag"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

func TestDiagnoseGovernanceWritesPrivateDraftOnlyReport(t *testing.T) {
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
	hash := strings.Repeat("c", 64)
	files := []domain.FileInstance{
		{StorageID: "s", Path: "/offline/cache/a.wav", Name: "a.wav", Size: 200, ContentSHA256: hash, ModifiedAt: time.Now(), DiscoveredAt: time.Now()},
		{StorageID: "s", Path: "/offline/cache/b.wav", Name: "b.wav", Size: 200, ContentSHA256: hash, ModifiedAt: time.Now(), DiscoveredAt: time.Now()},
	}
	ids, err := st.UpsertFiles(ctx, files)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if err := st.SaveFormat(ctx, id, domain.FormatInfo{Format: "wav", Category: domain.CategoryAudio, Duration: 2, Codec: "pcm"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runDiagnoseGovernance([]string{"--db", dbPath, "--out", dbPath}); err == nil {
		t.Fatal("diagnostic must refuse to overwrite its input database")
	}
	readOnly, err := store.OpenReadOnly(ctx, dbPath)
	if err != nil {
		t.Fatalf("database damaged after refused overwrite: %v", err)
	}
	_ = readOnly.Close()
	out := filepath.Join(tmp, "private", "p2.json")
	if err := runDiagnoseGovernance([]string{"--db", dbPath, "--out", out, "--large-media-min", "100"}); err == nil {
		t.Fatal("minimum below 1 MiB should be rejected")
	}
	if err := runDiagnoseGovernance([]string{"--db", dbPath, "--out", out, "--large-media-min", "1048576"}); err != nil {
		t.Fatal(err)
	}
	stat, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	if stat.Mode().Perm() != privateFileMode {
		t.Fatalf("mode=%v", stat.Mode().Perm())
	}
	var review governancediag.Report
	readJSONFile(t, out, &review)
	if review.ExecutionAuthorized || review.Summary.NonDraftPlans != 0 || review.Summary.DraftPlans != 1 {
		t.Fatalf("unsafe report: %#v", review.Summary)
	}
}
