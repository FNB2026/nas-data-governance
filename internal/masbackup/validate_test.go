package masbackup

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FNB2026/nas-data-governance/internal/store"
)

func makeBackup(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "ndg-masbackup-test-")
	if err != nil {
		t.Fatal(err)
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	dbPath := filepath.Join(dir, databaseName)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := createFrozenSchema11(context.Background(), db); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dbPath, 0600); err != nil {
		t.Fatal(err)
	}
	// This fixture was created and closed by this test. Remove only its own
	// SQLite sidecars so the test models the documented closed-backup contract.
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if err := os.Remove(dbPath + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
	}
	writeManifest(t, dir, SupportedSourceID, SupportedSchemaID)
	return dir
}

func validateFixture(t *testing.T, dir string) (Result, error) {
	t.Helper()
	stage := privateStage(t)
	return ValidateClosedBackup(context.Background(), dir, stage)
}

func privateStage(t *testing.T) string {
	t.Helper()
	stage, err := os.MkdirTemp(t.TempDir(), "private-stage-")
	if err != nil {
		t.Fatal(err)
	}
	stage, err = filepath.EvalSymlinks(stage)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(stage, 0700); err != nil {
		t.Fatal(err)
	}
	return stage
}

func writeManifest(t *testing.T, dir, sourceID, schemaID string) {
	t.Helper()
	database, err := os.ReadFile(filepath.Join(dir, databaseName))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(database)
	fingerprint := schemaFingerprintForFixture(t, filepath.Join(dir, databaseName))
	manifest := Manifest{
		FormatVersion: FormatVersion, SourceBundleID: sourceID,
		DatabaseSchemaID: schemaID, DatabaseSchemaFingerprint: fingerprint, DatabaseSHA256: hex.EncodeToString(sum[:]),
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, manifestName), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func schemaFingerprintForFixture(t *testing.T, path string) string {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro&immutable=1")
	if err != nil {
		return strings.Repeat("0", 64)
	}
	defer db.Close()
	fingerprint, err := fingerprintSchema(context.Background(), db)
	if err != nil {
		return strings.Repeat("0", 64)
	}
	return fingerprint
}

func TestValidateClosedBackupAcceptsCurrentDisposableSQLiteSnapshot(t *testing.T) {
	dir := makeBackup(t)
	before, err := os.ReadFile(filepath.Join(dir, databaseName))
	if err != nil {
		t.Fatal(err)
	}
	result, err := validateFixture(t, dir)
	if err != nil {
		t.Fatalf("ValidateClosedBackup: %v", err)
	}
	if result.SourceBundleID != SupportedSourceID || result.DatabaseSchemaID != SupportedSchemaID || result.IntegrityCheck != "ok" {
		t.Fatalf("unexpected validation result: %#v", result)
	}
	if result.DatabaseSchemaFingerprint != supportedSchemaFingerprint {
		t.Fatalf("unexpected schema fingerprint: %s", result.DatabaseSchemaFingerprint)
	}
	after, err := os.ReadFile(filepath.Join(dir, databaseName))
	if err != nil || string(before) != string(after) {
		t.Fatal("validation modified the supplied database")
	}
}

func TestCurrentSchemaFingerprintForFixture(t *testing.T) {
	dir := makeBackup(t)
	if got := schemaFingerprintForFixture(t, filepath.Join(dir, databaseName)); got != supportedSchemaFingerprint {
		t.Fatalf("current schema fingerprint changed: %s", got)
	}
}

func TestValidateClosedBackupRejectsUnverifiedIdentityAndSchema(t *testing.T) {
	for _, tc := range []struct {
		name, sourceID, schemaID string
	}{
		{name: "wrong source app", sourceID: "com.other.app", schemaID: SupportedSchemaID},
		{name: "unknown schema", sourceID: SupportedSourceID, schemaID: "future-schema"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := makeBackup(t)
			writeManifest(t, dir, tc.sourceID, tc.schemaID)
			if _, err := validateFixture(t, dir); !errors.Is(err, ErrInvalidBackup) {
				t.Fatalf("expected fail-closed rejection, got %v", err)
			}
		})
	}
}

func TestValidateClosedBackupRejectsUnrecognizedDatabaseSchemaEvenWhenManifestMatches(t *testing.T) {
	dir := makeBackup(t)
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(dir, databaseName))+"?_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE future_extension (id INTEGER PRIMARY KEY, value TEXT)`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if err := os.Remove(filepath.Join(dir, databaseName+suffix)); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
	}
	// The fixture manifest is recomputed over the modified file and its schema;
	// the binary's pinned allowlist must still reject this future layout.
	writeManifest(t, dir, SupportedSourceID, SupportedSchemaID)
	if _, err := validateFixture(t, dir); !errors.Is(err, ErrInvalidBackup) {
		t.Fatalf("expected pinned schema fingerprint rejection, got %v", err)
	}
}

func TestValidateClosedBackupRejectsManifestSchemaFingerprintMismatch(t *testing.T) {
	dir := makeBackup(t)
	manifestPath := filepath.Join(dir, manifestName)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.DatabaseSchemaFingerprint = strings.Repeat("0", 64)
	data, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := validateFixture(t, dir); !errors.Is(err, ErrInvalidBackup) {
		t.Fatalf("expected manifest schema mismatch rejection, got %v", err)
	}
}

func TestValidateClosedBackupRejectsSQLiteSidecars(t *testing.T) {
	dir := makeBackup(t)
	if err := os.WriteFile(filepath.Join(dir, databaseName+"-wal"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := validateFixture(t, dir); !errors.Is(err, ErrInvalidBackup) {
		t.Fatalf("expected sidecar rejection, got %v", err)
	}
}

func TestValidateClosedBackupRejectsDigestMismatchAndCorruption(t *testing.T) {
	t.Run("digest mismatch", func(t *testing.T) {
		dir := makeBackup(t)
		if err := os.WriteFile(filepath.Join(dir, databaseName), []byte("changed"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := validateFixture(t, dir); !errors.Is(err, ErrInvalidBackup) {
			t.Fatalf("expected digest rejection, got %v", err)
		}
	})
	t.Run("valid digest over corrupt database", func(t *testing.T) {
		dir := makeBackup(t)
		if err := os.WriteFile(filepath.Join(dir, databaseName), []byte(strings.Repeat("x", 256)), 0600); err != nil {
			t.Fatal(err)
		}
		writeManifest(t, dir, SupportedSourceID, SupportedSchemaID)
		if _, err := validateFixture(t, dir); !errors.Is(err, ErrInvalidBackup) {
			t.Fatalf("expected integrity rejection, got %v", err)
		}
	})
}

func TestValidateClosedBackupRejectsUnknownManifestFieldsAndSymlinks(t *testing.T) {
	t.Run("unknown field", func(t *testing.T) {
		dir := makeBackup(t)
		data, err := os.ReadFile(filepath.Join(dir, manifestName))
		if err != nil {
			t.Fatal(err)
		}
		data = []byte(strings.TrimSuffix(string(data), "}") + `,"bookmark":"must-not-be-imported"}`)
		if err := os.WriteFile(filepath.Join(dir, manifestName), data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := validateFixture(t, dir); !errors.Is(err, ErrInvalidBackup) {
			t.Fatalf("expected strict manifest rejection, got %v", err)
		}
	})
	t.Run("manifest symlink", func(t *testing.T) {
		dir := makeBackup(t)
		manifestPath := filepath.Join(dir, manifestName)
		real := filepath.Join(dir, "real-manifest.json")
		if err := os.Rename(manifestPath, real); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(real, manifestPath); err != nil {
			t.Fatal(err)
		}
		if _, err := validateFixture(t, dir); !errors.Is(err, ErrInvalidBackup) {
			t.Fatalf("expected manifest symlink rejection, got %v", err)
		}
	})
	t.Run("unexpected directory entry", func(t *testing.T) {
		dir := makeBackup(t)
		if err := os.Mkdir(filepath.Join(dir, "extra"), 0700); err != nil {
			t.Fatal(err)
		}
		if _, err := validateFixture(t, dir); !errors.Is(err, ErrInvalidBackup) {
			t.Fatalf("expected extra entry rejection, got %v", err)
		}
	})
	t.Run("database symlink", func(t *testing.T) {
		dir := makeBackup(t)
		dbPath := filepath.Join(dir, databaseName)
		real := filepath.Join(dir, "real.db")
		if err := os.Rename(dbPath, real); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(real, dbPath); err != nil {
			t.Fatal(err)
		}
		if _, err := validateFixture(t, dir); !errors.Is(err, ErrInvalidBackup) {
			t.Fatalf("expected symlink rejection, got %v", err)
		}
	})
}

func TestValidateClosedBackupRejectsNonPrivateOrSymlinkStagingRoot(t *testing.T) {
	dir := makeBackup(t)
	t.Run("group accessible", func(t *testing.T) {
		stage := t.TempDir()
		if err := os.Chmod(stage, 0755); err != nil {
			t.Fatal(err)
		}
		if _, err := ValidateClosedBackup(context.Background(), dir, stage); !errors.Is(err, ErrInvalidBackup) {
			t.Fatalf("expected non-private staging rejection, got %v", err)
		}
	})
	t.Run("symlink", func(t *testing.T) {
		real := t.TempDir()
		if err := os.Chmod(real, 0700); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(t.TempDir(), "stage-link")
		if err := os.Symlink(real, link); err != nil {
			t.Fatal(err)
		}
		if _, err := ValidateClosedBackup(context.Background(), dir, link); !errors.Is(err, ErrInvalidBackup) {
			t.Fatalf("expected staging symlink rejection, got %v", err)
		}
	})
}

func TestCopyContextAndHashHonorsCancellationAndSizeLimit(t *testing.T) {
	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var dst strings.Builder
		var hash strings.Builder
		if _, err := copyContextAndHash(ctx, &dst, strings.NewReader("fixture"), &hash, 100); !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation, got %v", err)
		}
		if dst.Len() != 0 || hash.Len() != 0 {
			t.Fatal("cancelled copy wrote bytes")
		}
	})
	t.Run("limit", func(t *testing.T) {
		var dst strings.Builder
		var hash strings.Builder
		if _, err := copyContextAndHash(context.Background(), &dst, strings.NewReader("four"), &hash, 3); err == nil {
			t.Fatal("expected size-limit rejection")
		}
		if dst.Len() != 0 || hash.Len() != 0 {
			t.Fatal("over-limit chunk was written")
		}
	})
}

func TestValidateClosedBackupCancellationCleansPrivateStage(t *testing.T) {
	dir := makeBackup(t)
	stage := privateStage(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ValidateClosedBackup(ctx, dir, stage); !errors.Is(err, ErrInvalidBackup) {
		t.Fatalf("expected cancelled validation rejection, got %v", err)
	}
	entries, err := os.ReadDir(stage)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("cancelled validation left %d staging entries", len(entries))
	}
}

func TestValidateClosedBackupCleanupFailureRejectsAndClearsResult(t *testing.T) {
	dir := makeBackup(t)
	stage := privateStage(t)
	cleanupErr := errors.New("injected cleanup failure")
	result, err := validateClosedBackup(context.Background(), dir, stage, func(path string) error {
		if removeErr := os.RemoveAll(path); removeErr != nil {
			return removeErr
		}
		return cleanupErr
	})
	if !errors.Is(err, ErrInvalidBackup) {
		t.Fatalf("expected cleanup failure rejection, got %v", err)
	}
	if result != (Result{}) {
		t.Fatal("cleanup failure returned validation success data")
	}
	entries, err := os.ReadDir(stage)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("cleanup test left %d staging entries", len(entries))
	}
}

func TestGovernanceSchema12NotAutomaticallyAcceptedByBackupPrototype(t *testing.T) {
	dir := makeBackup(t)
	source := filepath.Join(t.TempDir(), "new-layout.db")
	st, err := store.Open(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.Close(); err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	// Copy a clean closed snapshot. Never remove the owner's coordination file
	// from a live database to make an export or acceptance appear successful.
	if err = os.WriteFile(filepath.Join(dir, databaseName), bytes, 0600); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, dir, SupportedSourceID, SupportedSchemaID)
	if _, err = validateFixture(t, dir); err == nil {
		t.Fatal("new governance layout auto-approved by frozen backup prototype")
	}
	after, err := os.ReadFile(filepath.Join(dir, databaseName))
	if err != nil || string(bytes) != string(after) {
		t.Fatal("refused snapshot changed")
	}
}
