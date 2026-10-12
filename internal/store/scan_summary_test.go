package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

func TestScanSummaryIncludesReconciliationAndLayeredHashes(t *testing.T) {
	ctx := context.Background()
	s, e := Open(ctx, filepath.Join(t.TempDir(), "governance.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	s.RegisterStorage(ctx, domain.Storage{ID: "synthetic", RootPath: "/synthetic", CreatedAt: time.Now()})
	files := []domain.FileInstance{{StorageID: "synthetic", Path: "/synthetic/a", Name: "a", QuickHash: "quick", ContentSHA256: "full", ModifiedAt: time.Now(), DiscoveredAt: time.Now()}, {StorageID: "synthetic", Path: "/synthetic/b", Name: "b", QuickHash: "quick", ModifiedAt: time.Now(), DiscoveredAt: time.Now()}, {StorageID: "synthetic", Path: "/synthetic/c", Name: "c", ModifiedAt: time.Now(), DiscoveredAt: time.Now()}}
	if _, e = s.UpsertFiles(ctx, files); e != nil {
		t.Fatal(e)
	}
	v, e := s.ScanSummary(ctx, "synthetic")
	if e != nil || v.Active != 3 || v.Quick != 2 || v.Full != 1 {
		t.Fatal(v, e)
	}
	if _, e = s.db.ExecContext(ctx, `UPDATE file_instances SET file_status='missing' WHERE path='/synthetic/b'`); e != nil {
		t.Fatal(e)
	}
	if _, e = s.db.ExecContext(ctx, `UPDATE file_instances SET file_status='unavailable' WHERE path='/synthetic/c'`); e != nil {
		t.Fatal(e)
	}
	v, e = s.ScanSummary(ctx, "synthetic")
	if e != nil || v.Active != 1 || v.Missing != 1 || v.Unavailable != 1 || v.Quick != 1 || v.Full != 1 {
		t.Fatal(v, e)
	}
}
