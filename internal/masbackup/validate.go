// Package masbackup validates a closed GitHub-edition project backup before a
// future MAS import. It never migrates bookmarks, opens a project for writing,
// or removes/rewrites the supplied backup.
package masbackup

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

const (
	FormatVersion     = 1
	SupportedSourceID = "com.fnb.ndg"
	SupportedSchemaID = "ndg-project-layout-2026-10-v1"
	manifestName      = "backup.json"
	databaseName      = "governance.db"
	maxManifestSize   = 64 << 10
	maxDatabaseSize   = 8 << 30
	// Exact sqlite_master fingerprint for the supported current migration layout.
	supportedSchemaFingerprint = "e177d2a1ec16d33cd96eed49a70082c8b25a10c5b6b72ca7e4c7e5138f811af5"
)

// Manifest is deliberately path-free. Bookmarks, roots, project names, and
// user-specific metadata are never part of the interchange header.
type Manifest struct {
	FormatVersion             int    `json:"formatVersion"`
	SourceBundleID            string `json:"sourceBundleId"`
	DatabaseSchemaID          string `json:"databaseSchemaId"`
	DatabaseSchemaFingerprint string `json:"databaseSchemaFingerprint"`
	DatabaseSHA256            string `json:"databaseSha256"`
}

// Result contains only non-sensitive validation facts; it never returns a
// project path, source path, file name, or database contents.
type Result struct {
	FormatVersion             int
	SourceBundleID            string
	DatabaseSchemaID          string
	DatabaseSchemaFingerprint string
	DatabaseSHA256            string
	IntegrityCheck            string
}

var ErrInvalidBackup = errors.New("masbackup: backup rejected")

// ValidateClosedBackup validates a two-file backup directory. The caller must
// hold the native read lease for dir. The trusted caller must separately prove
// stagingRoot's owner, ACL, and MAS container membership, and keep the staging
// root and its ancestry stable for the entire call. A 0700 mode is not proof of
// those conditions. The mode/path checks here are only prototype preflight;
// staging creation uses a path, so this function is not a security boundary
// against concurrent staging-root replacement. Production integration remains
// blocked until rooted staging creation and runtime containment are verified.
// Input is opened relative to an os.Root,
// copied to a private temporary file, then checked read-only. The supplied
// backup is never modified. The caller remains responsible for ensuring the
// backup was produced from a cleanly closed project while holding its project
// ownership lock; this preflight cannot prove that contract.
func ValidateClosedBackup(ctx context.Context, dir, stagingRoot string) (result Result, retErr error) {
	return validateClosedBackup(ctx, dir, stagingRoot, os.RemoveAll)
}

func validateClosedBackup(ctx context.Context, dir, stagingRoot string, removeAll func(string) error) (result Result, retErr error) {
	if strings.TrimSpace(dir) == "" {
		return result, reject("empty backup root")
	}
	stageAbs, err := validatePrivateStagingRoot(stagingRoot)
	if err != nil {
		return result, reject("private validation staging unavailable")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return result, reject("invalid backup root")
	}
	rootInfo, err := os.Lstat(abs)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return result, reject("backup root must be a real directory")
	}
	for parent := filepath.Dir(abs); ; parent = filepath.Dir(parent) {
		info, statErr := os.Lstat(parent)
		if statErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return result, reject("backup root ancestry is not a real directory")
		}
		if filepath.Dir(parent) == parent {
			break
		}
	}
	root, err := os.OpenRoot(abs)
	if err != nil {
		return result, reject("backup root unavailable")
	}
	defer root.Close()
	openedRootInfo, err := root.Stat(".")
	if err != nil || !os.SameFile(rootInfo, openedRootInfo) {
		return result, reject("backup root identity changed")
	}

	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil || len(entries) != 2 {
		return result, reject("backup must contain exactly manifest and database")
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		seen[entry.Name()] = true
		if entry.Type()&fs.ModeSymlink != 0 || entry.IsDir() {
			return result, reject("backup entries must be regular files")
		}
		info, statErr := root.Lstat(entry.Name())
		if statErr != nil || !info.Mode().IsRegular() || !sameDevice(rootInfo, info) {
			return result, reject("backup entries must be regular files on the selected volume")
		}
	}
	if !seen[manifestName] || !seen[databaseName] {
		return result, reject("required backup entry missing")
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, err := root.Lstat(databaseName + suffix); err == nil || !errors.Is(err, fs.ErrNotExist) {
			return result, reject("sqlite sidecar present; backup is not closed")
		}
	}

	manifestInfo, err := root.Lstat(manifestName)
	if err != nil || !manifestInfo.Mode().IsRegular() || manifestInfo.Size() <= 0 || manifestInfo.Size() > maxManifestSize {
		return result, reject("invalid manifest file")
	}
	manifestFile, err := root.Open(manifestName)
	if err != nil {
		return result, reject("manifest unavailable")
	}
	manifestOpenedInfo, statErr := manifestFile.Stat()
	manifestBytes, readErr := io.ReadAll(io.LimitReader(manifestFile, maxManifestSize+1))
	closeErr := manifestFile.Close()
	if statErr != nil || !os.SameFile(manifestInfo, manifestOpenedInfo) || readErr != nil || closeErr != nil || int64(len(manifestBytes)) != manifestInfo.Size() {
		return result, reject("manifest changed or unreadable")
	}
	var manifest Manifest
	decoder := json.NewDecoder(strings.NewReader(string(manifestBytes)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return result, reject("manifest format invalid")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return result, reject("manifest has trailing data")
	}
	if manifest.FormatVersion != FormatVersion || manifest.SourceBundleID != SupportedSourceID || manifest.DatabaseSchemaID != SupportedSchemaID || !validSHA256(manifest.DatabaseSchemaFingerprint) || !validSHA256(manifest.DatabaseSHA256) {
		return result, reject("backup identity or schema unsupported")
	}

	dbInfo, err := root.Lstat(databaseName)
	if err != nil || !dbInfo.Mode().IsRegular() || dbInfo.Size() < 100 || dbInfo.Size() > maxDatabaseSize || !sameDevice(rootInfo, dbInfo) {
		return result, reject("invalid database file")
	}
	dbFile, err := root.Open(databaseName)
	if err != nil {
		return result, reject("database unavailable")
	}
	defer dbFile.Close()
	openedInfo, err := dbFile.Stat()
	if err != nil || !os.SameFile(dbInfo, openedInfo) || dbInfo.Size() != openedInfo.Size() || !dbInfo.ModTime().Equal(openedInfo.ModTime()) {
		return result, reject("database identity changed")
	}

	tmpDir, err := os.MkdirTemp(stageAbs, "ndg-mas-backup-")
	if err != nil {
		return result, reject("private validation staging unavailable")
	}
	defer func() {
		if cleanupErr := removeAll(tmpDir); cleanupErr != nil {
			result = Result{}
			retErr = errors.Join(ErrInvalidBackup, errors.New("validation staging cleanup failed"))
		}
	}()
	tmpPath := filepath.Join(tmpDir, databaseName)
	tmp, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return result, reject("private validation staging unavailable")
	}
	hash := sha256.New()
	n, copyErr := copyContextAndHash(ctx, tmp, dbFile, hash, maxDatabaseSize)
	if copyErr == nil {
		copyErr = tmp.Sync()
	}
	if closeErr := tmp.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil || n != dbInfo.Size() || n > maxDatabaseSize {
		return result, reject("database copy incomplete or changed")
	}
	finalInfo, err := dbFile.Stat()
	if err != nil || !os.SameFile(openedInfo, finalInfo) || finalInfo.Size() != openedInfo.Size() || !finalInfo.ModTime().Equal(openedInfo.ModTime()) {
		return result, reject("database changed while copying")
	}
	actualHash := hex.EncodeToString(hash.Sum(nil))
	if actualHash != strings.ToLower(manifest.DatabaseSHA256) {
		return result, reject("database digest mismatch")
	}
	// Re-check sidecars after the copy; a concurrent writer invalidates the
	// closed-backup contract even if the main database digest happens to match.
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, err := root.Lstat(databaseName + suffix); err == nil || !errors.Is(err, fs.ErrNotExist) {
			return result, reject("sqlite sidecar appeared during validation")
		}
	}

	actualSchemaFingerprint, err := validateDatabase(ctx, tmpPath)
	if err != nil || actualSchemaFingerprint != supportedSchemaFingerprint || actualSchemaFingerprint != strings.ToLower(manifest.DatabaseSchemaFingerprint) {
		return result, reject("database integrity or schema check failed")
	}
	result = Result{
		FormatVersion: manifest.FormatVersion, SourceBundleID: manifest.SourceBundleID,
		DatabaseSchemaID: manifest.DatabaseSchemaID, DatabaseSchemaFingerprint: actualSchemaFingerprint, DatabaseSHA256: actualHash,
		IntegrityCheck: "ok",
	}
	return result, nil
}

func validatePrivateStagingRoot(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("empty staging root")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(abs)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return "", errors.New("staging root must be a private real directory")
	}
	for parent := filepath.Dir(abs); ; parent = filepath.Dir(parent) {
		parentInfo, statErr := os.Lstat(parent)
		if statErr != nil || !parentInfo.IsDir() || parentInfo.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("staging root ancestry is not a real directory")
		}
		if filepath.Dir(parent) == parent {
			break
		}
	}
	return abs, nil
}

func copyContextAndHash(ctx context.Context, dst io.Writer, src io.Reader, hash io.Writer, limit int64) (int64, error) {
	buf := make([]byte, 128*1024)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		readSize := len(buf)
		if remaining := limit + 1 - total; int64(readSize) > remaining {
			readSize = int(remaining)
		}
		n, readErr := src.Read(buf[:readSize])
		if n > 0 {
			if int64(n) > limit-total {
				return total + int64(n), errors.New("database exceeds validation size limit")
			}
			if _, err := dst.Write(buf[:n]); err != nil {
				return total, err
			}
			if _, err := hash.Write(buf[:n]); err != nil {
				return total, err
			}
			total += int64(n)
		}
		if errors.Is(readErr, io.EOF) {
			return total, nil
		}
		if readErr != nil {
			return total, readErr
		}
	}
}

func validateDatabase(ctx context.Context, path string) (string, error) {
	if strings.ContainsAny(path, "?#") {
		return "", errors.New("unsupported uri character")
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro&immutable=1")
	if err != nil {
		return "", err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		return "", err
	}
	rows, err := db.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return "", err
	}
	count := 0
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil || value != "ok" {
			_ = rows.Close()
			return "", errors.New("integrity check failed")
		}
		count++
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return "", err
	}
	if err := rows.Close(); err != nil || count != 1 {
		return "", errors.New("integrity result invalid")
	}
	rows, err = db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return "", err
	}
	if rows.Next() {
		_ = rows.Close()
		return "", errors.New("foreign key check failed")
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return "", err
	}
	if err := rows.Close(); err != nil {
		return "", err
	}
	if err := requireCurrentTables(ctx, db); err != nil {
		return "", err
	}
	return fingerprintSchema(ctx, db)
}

func fingerprintSchema(ctx context.Context, db *sql.DB) (string, error) {
	rows, err := db.QueryContext(ctx, `SELECT type, name, tbl_name, COALESCE(sql, '') FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' ORDER BY type, name`)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	for rows.Next() {
		var objectType, name, tableName, statement string
		if err := rows.Scan(&objectType, &name, &tableName, &statement); err != nil {
			_ = rows.Close()
			return "", err
		}
		for _, part := range []string{objectType, name, tableName, strings.TrimSpace(statement)} {
			_, _ = io.WriteString(hash, part)
			_, _ = hash.Write([]byte{0})
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return "", err
	}
	if err := rows.Close(); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func requireCurrentTables(ctx context.Context, db *sql.DB) error {
	for table, required := range map[string][]string{
		"storages":          {"id", "root_path", "kind", "created_at"},
		"file_instances":    {"storage_id", "path", "name", "quick_hash", "content_sha256", "file_status"},
		"scan_checkpoints":  {"storage_id", "last_scanned_path", "scanned_count", "status", "updated_at"},
		"execution_journal": {"plan_id", "action_index", "status", "rollback_status"},
		"restore_journal":   {"plan_id", "item_id", "status", "completed_at"},
		"purge_journal":     {"plan_id", "item_id", "status", "completed_at"},
	} {
		columns, err := tableColumns(ctx, db, table)
		if err != nil {
			return err
		}
		for _, column := range required {
			if !columns[column] {
				return errors.New("required database column missing")
			}
		}
	}
	return nil
}

func tableColumns(ctx context.Context, db *sql.DB, table string) (map[string]bool, error) {
	// Table names are fixed in this package, never derived from user input.
	rows, err := db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := make(map[string]bool)
	for rows.Next() {
		var cid, notnull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notnull, &defaultValue, &primaryKey); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	return columns, rows.Err()
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func reject(reason string) error {
	// Details are fixed classifications, never user-controlled paths or names.
	return errors.Join(ErrInvalidBackup, errors.New(reason))
}
