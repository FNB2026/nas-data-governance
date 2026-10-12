package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

func TestLatestCheckpointSummaryTerminalRestartAndErrors(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "governance.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RegisterStorage(ctx, domain.Storage{ID: "synthetic", RootPath: "/synthetic", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.LatestCheckpointSummary(ctx, "synthetic"); !errors.Is(err, ErrNotFound) {
		t.Fatal("missing checkpoint semantics", err)
	}
	id, err := s.StartCheckpoint(ctx, "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.UpdateCheckpoint(ctx, id, "/synthetic/private_marker", 7); err != nil {
		t.Fatal(err)
	}
	v, err := s.LatestCheckpointSummary(ctx, "synthetic")
	if err != nil || v.Status != "running" || v.Scanned != 7 {
		t.Fatal(v, err)
	}
	if err = s.CompleteCheckpoint(ctx, id, "completed"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.LastCheckpoint(ctx, "synthetic"); !errors.Is(err, ErrNotFound) {
		t.Fatal("terminal checkpoint offered for Resume", err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.LatestCheckpointSummary(ctx, "synthetic")
	if err != nil || v.Status != "completed" || v.Scanned != 7 {
		t.Fatal(v, err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = s.LatestCheckpointSummary(ctx, "synthetic"); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatal("database failure hidden as absent checkpoint", err)
	}
}

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
