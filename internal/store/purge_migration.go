package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"

	"github.com/FNB2026/nas-data-governance/schemas"
)

// OpenPurgeCandidate is the private Worker entry point for Schema014.
// Ordinary store opens continue to use the old-compatible layout.
func OpenPurgeCandidate(ctx context.Context, path string) (*SQLiteStore, error) {
	st, err := Open(ctx, path)
	if err != nil {
		return nil, err
	}
	if err := st.enablePurgeSchema(ctx); err != nil {
		_ = st.Close()
		return nil, err
	}
	return st, nil
}

func (s *SQLiteStore) enablePurgeSchema(ctx context.Context) error {
	layout, err := schemaLayoutFingerprint(ctx, s.db)
	if err != nil {
		return err
	}
	if layout == dualPurge14Layout {
		return nil
	}
	if layout != authorizationAudit13Layout {
		return ErrGovernanceRejected
	}
	recovery, scan, err := governanceGuard(ctx, s.db)
	if err != nil || recovery || scan {
		return ErrGovernanceRejected
	}
	// Open holds the project owner flock. VACUUM INTO captures committed
	// WAL content while the old schema is still supported by old binaries.
	if err := s.backupBeforePurgeSchema(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.ExecContext(ctx, schemas.Schema014); err != nil {
		return err
	}
	layout, err = schemaLayoutFingerprint(ctx, tx)
	if err != nil || layout != dualPurge14Layout {
		return ErrGovernanceRejected
	}
	return tx.Commit()
}

func (s *SQLiteStore) backupBeforePurgeSchema(ctx context.Context) error {
	dir := filepath.Dir(s.path)
	stage, err := os.MkdirTemp(dir, ".pre-schema014-")
	if err != nil {
		return err
	}
	snapshot := filepath.Join(stage, "snapshot.db")
	defer os.RemoveAll(stage)
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, snapshot); err != nil {
		return err
	}
	if err := os.Chmod(snapshot, 0600); err != nil {
		return err
	}
	if err := verifyPurgeBackup(ctx, snapshot); err != nil {
		return err
	}
	file, err := os.OpenFile(snapshot, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	if err := errors.Join(file.Sync(), file.Close()); err != nil {
		return err
	}
	// A previous backup is deliberately preserved. Before retrying a failed
	// migration the operator can retain it under a different name.
	if err := os.Link(snapshot, s.path+".before-schema014"); err != nil {
		return err
	}
	parent, err := os.Open(dir)
	if err != nil {
		return err
	}
	return errors.Join(parent.Sync(), parent.Close())
}

func verifyPurgeBackup(ctx context.Context, snapshot string) error {
	// Immutable is safe for this newly closed, private VACUUM destination.
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(snapshot)+"?mode=ro&immutable=1")
	if err != nil {
		return err
	}
	defer db.Close()
	layout, err := schemaLayoutFingerprint(ctx, db)
	var check string
	if err != nil || layout != authorizationAudit13Layout ||
		db.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&check) != nil || check != "ok" {
		return ErrGovernanceRejected
	}
	return nil
}
