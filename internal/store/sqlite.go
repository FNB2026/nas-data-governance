package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/privatefs"

	_ "modernc.org/sqlite"
)

// SQLiteStore implements Store on top of a SQLite database file.
type SQLiteStore struct {
	db            *sql.DB
	path          string
	secureOnClose bool
}

// Open creates (or opens) a SQLite database at path and applies migrations.
// The database is project-owned; it never reads or writes user files.
func Open(ctx context.Context, path string) (*SQLiteStore, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("store: resolve path: %w", err)
	}
	if strings.ContainsAny(absPath, "?#") {
		return nil, fmt.Errorf("store: path contains unsupported URI delimiter")
	}
	dbDir := filepath.Dir(absPath)
	if filepath.Base(dbDir) == "var" {
		err = privatefs.SecureDirectory(dbDir)
	} else {
		err = privatefs.EnsureDirectory(dbDir)
	}
	if err != nil {
		return nil, fmt.Errorf("store: secure directory: %w", err)
	}
	if _, err := os.Lstat(absPath); err == nil {
		if err := privatefs.SecureFile(absPath); err != nil {
			return nil, fmt.Errorf("store: secure database: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("store: inspect database: %w", err)
	}
	// _txlock=immediate makes write transactions acquire the write lock up
	// front, avoiding "database is locked" mid-transaction. foreign_keys &
	// busy_timeout make the behavior safer under concurrent readers.
	dsn := fmt.Sprintf("file:%s?_txlock=immediate&_busy_timeout=5000", filepath.ToSlash(absPath))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite serializes writes; one conn avoids lock storms.
	s := &SQLiteStore{db: db, path: absPath, secureOnClose: true}
	if err := s.Init(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := privatefs.SecureSQLiteFiles(absPath); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: secure sqlite files: %w", err)
	}
	return s, nil
}

// OpenReadOnly opens an existing project database without applying migrations
// or allowing writes. It is intended for diagnostic/reporting commands whose
// database boundary must remain strictly read-only.
func OpenReadOnly(ctx context.Context, path string) (*SQLiteStore, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("store: resolve read-only path: %w", err)
	}
	// modernc/sqlite passes file: DSNs to SQLite's URI parser. Reject URI
	// delimiters instead of risking mode=ro being parsed as part of a filename.
	if strings.ContainsAny(absPath, "?#") {
		return nil, fmt.Errorf("store: read-only path contains unsupported URI delimiter")
	}
	info, err := os.Lstat(absPath)
	if err != nil {
		return nil, fmt.Errorf("store: inspect read-only database: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("store: read-only database must not be a symbolic link")
	}
	dsn := fmt.Sprintf("file:%s?mode=ro&_busy_timeout=5000", filepath.ToSlash(absPath))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open read-only: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: open read-only: %w", err)
	}
	return &SQLiteStore{db: db, path: absPath}, nil
}

func (s *SQLiteStore) Close() error {
	if err := s.db.Close(); err != nil {
		return err
	}
	if s.secureOnClose {
		return privatefs.SecureSQLiteFiles(s.path)
	}
	return nil
}

// ---------------- storages ----------------

func (s *SQLiteStore) RegisterStorage(ctx context.Context, st domain.Storage) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO storages(id, root_path, kind, created_at) VALUES(?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET root_path=excluded.root_path, kind=excluded.kind`,
		st.ID, st.RootPath, st.Kind, st.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: register storage: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListStorages(ctx context.Context) ([]domain.Storage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, root_path, kind, created_at FROM storages ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("store: list storages: %w", err)
	}
	defer rows.Close()
	out := make([]domain.Storage, 0)
	for rows.Next() {
		var st domain.Storage
		var createdAt string
		if err := rows.Scan(&st.ID, &st.RootPath, &st.Kind, &createdAt); err != nil {
			return nil, err
		}
		st.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		out = append(out, st)
	}
	return out, rows.Err()
}

// ---------------- file_instances ----------------

func (s *SQLiteStore) UpsertFiles(ctx context.Context, files []domain.FileInstance) ([]int64, error) {
	if len(files) == 0 {
		return nil, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store: begin upsert: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // commit wins; rollback is a no-op then.

	ids := make([]int64, 0, len(files))
	const stmt = `INSERT INTO file_instances
		(storage_id, path, name, size, mode, mtime, device, inode, quick_hash, content_sha256, discovered_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(storage_id, path) DO UPDATE SET
			name=excluded.name, size=excluded.size, mode=excluded.mode, mtime=excluded.mtime,
			device=excluded.device, inode=excluded.inode, quick_hash=excluded.quick_hash,
			content_sha256=excluded.content_sha256, discovered_at=excluded.discovered_at
		RETURNING id`
	prep, err := tx.PrepareContext(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("store: prepare upsert: %w", err)
	}
	defer prep.Close()
	for row, f := range files {
		var id int64
		err := prep.QueryRowContext(ctx,
			f.StorageID, f.Path, f.Name, f.Size, f.Mode,
			f.ModifiedAt.UTC().Format(time.RFC3339Nano),
			sqliteUint64(f.Device), sqliteUint64(f.Inode), f.QuickHash, f.ContentSHA256,
			f.DiscoveredAt.UTC().Format(time.RFC3339Nano),
		).Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("store: upsert file row %d: %w", row+1, err)
		}
		ids = append(ids, id)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: commit upsert: %w", err)
	}
	return ids, nil
}

func (s *SQLiteStore) ListFiles(ctx context.Context, storageID string) ([]domain.FileInstance, error) {
	query := `
		SELECT storage_id, path, name, size, mode, mtime, device, inode, quick_hash, content_sha256, discovered_at
		FROM file_instances
		WHERE file_status = 'active'`
	var rows *sql.Rows
	var err error
	if storageID == "" {
		rows, err = s.db.QueryContext(ctx, query+` ORDER BY storage_id, path`)
	} else {
		rows, err = s.db.QueryContext(ctx, query+` AND storage_id = ? ORDER BY path`, storageID)
	}
	if err != nil {
		return nil, fmt.Errorf("store: list files: %w", err)
	}
	defer rows.Close()
	out := make([]domain.FileInstance, 0)
	for rows.Next() {
		var f domain.FileInstance
		var mtime, discoveredAt string
		var device, inode int64
		if err := rows.Scan(&f.StorageID, &f.Path, &f.Name, &f.Size, &f.Mode, &mtime,
			&device, &inode, &f.QuickHash, &f.ContentSHA256, &discoveredAt); err != nil {
			return nil, err
		}
		f.Device, f.Inode = uint64(device), uint64(inode)
		f.ModifiedAt, _ = time.Parse(time.RFC3339Nano, mtime)
		f.DiscoveredAt, _ = time.Parse(time.RFC3339Nano, discoveredAt)
		out = append(out, f)
	}
	return out, rows.Err()
}

// SQLite INTEGER is signed, while Unix device and inode identifiers are
// uint64. SMB clients may synthesize identifiers with the high bit set. A
// bit-preserving cast stores those values as negative INTEGERs and converts
// them back on read without losing identity information.
func sqliteUint64(v uint64) int64 { return int64(v) }

func (s *SQLiteStore) FileID(ctx context.Context, storageID, path string) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM file_instances WHERE storage_id = ? AND path = ?`, storageID, path).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("store: file id: %w", err)
	}
	return id, nil
}

// ---------------- directory_contexts ----------------

func (s *SQLiteStore) SaveContext(ctx context.Context, fileID int64, c domain.DirectoryContext, ruleVersion string) error {
	blob, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("store: marshal context: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO directory_contexts(file_id, context_json, rule_version) VALUES(?, ?, ?)
		 ON CONFLICT(file_id) DO UPDATE SET context_json=excluded.context_json, rule_version=excluded.rule_version`,
		fileID, string(blob), ruleVersion)
	if err != nil {
		return fmt.Errorf("store: save context: %w", err)
	}
	return nil
}

// SaveContexts persists a batch in one transaction. It is used by index
// import so large indexes do not create one SQLite transaction per file.
func (s *SQLiteStore) SaveContexts(ctx context.Context, fileIDs []int64, contexts []domain.DirectoryContext, ruleVersion string) error {
	if len(fileIDs) != len(contexts) {
		return fmt.Errorf("store: context batch length mismatch")
	}
	if len(fileIDs) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin context batch: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO directory_contexts(file_id, context_json, rule_version) VALUES(?, ?, ?)
		ON CONFLICT(file_id) DO UPDATE SET context_json=excluded.context_json, rule_version=excluded.rule_version`)
	if err != nil {
		return fmt.Errorf("store: prepare context batch: %w", err)
	}
	defer stmt.Close()
	for i, c := range contexts {
		blob, err := json.Marshal(c)
		if err != nil {
			return fmt.Errorf("store: marshal context: %w", err)
		}
		if _, err := stmt.ExecContext(ctx, fileIDs[i], string(blob), ruleVersion); err != nil {
			return fmt.Errorf("store: save context batch: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit context batch: %w", err)
	}
	return nil
}

// ---------------- file_formats ----------------

func (s *SQLiteStore) SaveFormat(ctx context.Context, fileID int64, info domain.FormatInfo) error {
	blob, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("store: marshal format: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO file_formats(file_id, format_name, category, format_json) VALUES(?, ?, ?, ?)
		 ON CONFLICT(file_id) DO UPDATE SET format_name=excluded.format_name, category=excluded.category, format_json=excluded.format_json`,
		fileID, info.Format, string(info.Category), string(blob))
	if err != nil {
		return fmt.Errorf("store: save format: %w", err)
	}
	return nil
}

func (s *SQLiteStore) SaveFormatsByPath(ctx context.Context, records []FormatRecord) (int, int, error) {
	if len(records) == 0 {
		return 0, 0, nil
	}
	blobs := make([][]byte, len(records))
	for i, record := range records {
		if record.StorageID == "" || record.Path == "" || record.Info.Format == "" {
			return 0, 0, fmt.Errorf("store: incomplete format batch record %d", i)
		}
		blob, err := json.Marshal(record.Info)
		if err != nil {
			return 0, 0, fmt.Errorf("store: marshal format batch: %w", err)
		}
		blobs[i] = blob
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("store: begin format batch: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	lookup, err := tx.PrepareContext(ctx, `SELECT id FROM file_instances WHERE storage_id = ? AND path = ?`)
	if err != nil {
		return 0, 0, fmt.Errorf("store: prepare format lookup: %w", err)
	}
	defer lookup.Close()
	upsert, err := tx.PrepareContext(ctx, `INSERT INTO file_formats(file_id, format_name, category, format_json) VALUES(?, ?, ?, ?)
		ON CONFLICT(file_id) DO UPDATE SET format_name=excluded.format_name, category=excluded.category, format_json=excluded.format_json`)
	if err != nil {
		return 0, 0, fmt.Errorf("store: prepare format batch: %w", err)
	}
	defer upsert.Close()
	saved, missing := 0, 0
	for i, record := range records {
		var fileID int64
		if err := lookup.QueryRowContext(ctx, record.StorageID, record.Path).Scan(&fileID); err != nil {
			if err == sql.ErrNoRows {
				missing++
				continue
			}
			return 0, 0, fmt.Errorf("store: lookup format batch row: %w", err)
		}
		if _, err := upsert.ExecContext(ctx, fileID, record.Info.Format, string(record.Info.Category), string(blobs[i])); err != nil {
			return 0, 0, fmt.Errorf("store: save format batch: %w", err)
		}
		saved++
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("store: commit format batch: %w", err)
	}
	return saved, missing, nil
}

func (s *SQLiteStore) ListFormats(ctx context.Context, storageID string) ([]FormatRecord, error) {
	query := `SELECT f.storage_id, f.path, x.format_json
		FROM file_formats x JOIN file_instances f ON f.id=x.file_id
		WHERE f.file_status = 'active'`
	var args []any
	if storageID != "" {
		query += ` AND f.storage_id = ?`
		args = append(args, storageID)
	}
	query += ` ORDER BY f.storage_id, f.path`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list formats: %w", err)
	}
	defer rows.Close()
	var out []FormatRecord
	for rows.Next() {
		var record FormatRecord
		var blob string
		if err := rows.Scan(&record.StorageID, &record.Path, &blob); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(blob), &record.Info); err != nil {
			return nil, fmt.Errorf("store: decode persisted format: %w", err)
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

// ---------------- rules (L1 infrastructure) ----------------

func (s *SQLiteStore) SaveRule(ctx context.Context, r domain.Rule) error {
	enabled := 0
	if r.Enabled {
		enabled = 1
	}
	var approvedAt, expiresAt any
	if r.ApprovedAt != nil {
		approvedAt = r.ApprovedAt.UTC().Format(time.RFC3339Nano)
	}
	if r.ExpiresAt != nil {
		expiresAt = r.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO rules(id, version, priority, enabled, definition_yaml, source, batch_id, confidence, status, approved_at, expires_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET version=excluded.version, priority=excluded.priority, enabled=excluded.enabled,
		   definition_yaml=excluded.definition_yaml, source=excluded.source, batch_id=excluded.batch_id,
		   confidence=excluded.confidence, status=excluded.status, approved_at=excluded.approved_at, expires_at=excluded.expires_at`,
		r.ID, r.Version, r.Priority, enabled, r.Definition,
		string(r.Source), nullIfEmpty(r.BatchID), r.Confidence, string(r.Status), approvedAt, expiresAt)
	if err != nil {
		return fmt.Errorf("store: save rule: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListRules(ctx context.Context, source domain.RuleSource, status domain.RuleStatus) ([]domain.Rule, error) {
	q := `SELECT id, version, priority, enabled, definition_yaml, source, batch_id, confidence, status, approved_at, expires_at FROM rules`
	args := []any{}
	conds := []string{}
	if source != "" {
		conds = append(conds, "source = ?")
		args = append(args, string(source))
	}
	if status != "" {
		conds = append(conds, "status = ?")
		args = append(args, string(status))
	}
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += " ORDER BY priority DESC, id"
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list rules: %w", err)
	}
	defer rows.Close()
	out := make([]domain.Rule, 0)
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) UpdateRuleStatus(ctx context.Context, ruleID string, status domain.RuleStatus, approvedAt *time.Time) error {
	var current string
	if err := s.db.QueryRowContext(ctx, `SELECT status FROM rules WHERE id = ?`, ruleID).Scan(&current); err != nil {
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		return fmt.Errorf("store: read rule status: %w", err)
	}
	if !legalRuleTransition(domain.RuleStatus(current), status) {
		return fmt.Errorf("store: illegal rule status transition %s -> %s", current, status)
	}
	var at any
	if approvedAt != nil {
		at = approvedAt.UTC().Format(time.RFC3339Nano)
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE rules SET status = ?, approved_at = ? WHERE id = ?`,
		string(status), at, ruleID)
	if err != nil {
		return fmt.Errorf("store: update rule status: %w", err)
	}
	return nil
}

func legalRuleTransition(from, to domain.RuleStatus) bool {
	switch from {
	case domain.RuleDraft:
		return to == domain.RuleProbation || to == domain.RuleRejected || to == domain.RuleDisabled
	case domain.RuleProbation:
		return to == domain.RuleApproved || to == domain.RuleRejected || to == domain.RuleDisabled
	case domain.RuleApproved:
		return to == domain.RuleDisabled
	default:
		return false
	}
}

func (s *SQLiteStore) DisableBatch(ctx context.Context, batchID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE rules SET status = ?, enabled = 0 WHERE batch_id = ?`,
		string(domain.RuleDisabled), batchID)
	if err != nil {
		return fmt.Errorf("store: disable batch: %w", err)
	}
	return nil
}

func (s *SQLiteStore) SaveLearningBatch(ctx context.Context, b LearningBatch) error {
	var completedAt any
	if b.CompletedAt != nil {
		completedAt = b.CompletedAt.UTC().Format(time.RFC3339Nano)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO learning_batches(id, source, started_at, completed_at, rule_count, status)
		 VALUES(?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET completed_at=excluded.completed_at, rule_count=excluded.rule_count, status=excluded.status`,
		b.ID, b.Source, b.StartedAt.UTC().Format(time.RFC3339Nano), completedAt, b.RuleCount, b.Status)
	if err != nil {
		return fmt.Errorf("store: save learning batch: %w", err)
	}
	return nil
}

// scanRule reads a rule row. Uses sql.Scanner via rows.Scan.
func scanRule(rows *sql.Rows) (domain.Rule, error) {
	var r domain.Rule
	var enabled int
	var source, status, batchID, approvedAt, expiresAt sql.NullString
	err := rows.Scan(&r.ID, &r.Version, &r.Priority, &enabled, &r.Definition,
		&source, &batchID, &r.Confidence, &status, &approvedAt, &expiresAt)
	if err != nil {
		return r, fmt.Errorf("store: scan rule: %w", err)
	}
	r.Enabled = enabled != 0
	r.Source = domain.RuleSource(source.String)
	r.BatchID = batchID.String
	r.Status = domain.RuleStatus(status.String)
	if approvedAt.Valid && approvedAt.String != "" {
		t, err := time.Parse(time.RFC3339Nano, approvedAt.String)
		if err == nil {
			r.ApprovedAt = &t
		}
	}
	if expiresAt.Valid && expiresAt.String != "" {
		t, err := time.Parse(time.RFC3339Nano, expiresAt.String)
		if err == nil {
			r.ExpiresAt = &t
		}
	}
	return r, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *SQLiteStore) CreateTask(ctx context.Context, task domain.OperationTask) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO operation_tasks(id, root_path, state, created_at) VALUES(?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET root_path=excluded.root_path, state=excluded.state`,
		task.ID, task.RootPath, task.State, task.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: create task: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetTask(ctx context.Context, id string) (domain.OperationTask, error) {
	var t domain.OperationTask
	var createdAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, root_path, state, created_at FROM operation_tasks WHERE id = ?`, id).
		Scan(&t.ID, &t.RootPath, &t.State, &createdAt)
	if err == sql.ErrNoRows {
		return t, ErrNotFound
	}
	if err != nil {
		return t, fmt.Errorf("store: get task: %w", err)
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return t, nil
}

func (s *SQLiteStore) ListTasks(ctx context.Context) ([]domain.OperationTask, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, root_path, state, created_at FROM operation_tasks ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("store: list tasks: %w", err)
	}
	defer rows.Close()
	out := make([]domain.OperationTask, 0)
	for rows.Next() {
		var t domain.OperationTask
		var createdAt string
		if err := rows.Scan(&t.ID, &t.RootPath, &t.State, &createdAt); err != nil {
			return nil, err
		}
		t.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) UpdateTaskState(ctx context.Context, taskID string, state domain.TaskState) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE operation_tasks SET state = ? WHERE id = ?`,
		string(state), taskID)
	if err != nil {
		return fmt.Errorf("store: update task state: %w", err)
	}
	return nil
}

// ---------------- operation_plans ----------------

// SavePlans replaces all plans for a task atomically. Each plan becomes one
// row; the actions list and group-level metadata (content_sha256, retain_path,
// retain_score, evidence) live inside evidence_json so the existing schema
// does not need a schema migration to express a composite plan.
func (s *SQLiteStore) SavePlans(ctx context.Context, taskID string, plans []domain.OperationPlan) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin save plans: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, `DELETE FROM operation_plans WHERE task_id = ?`, taskID); err != nil {
		return fmt.Errorf("store: clear plans: %w", err)
	}
	prep, err := tx.PrepareContext(ctx, `
		INSERT INTO operation_plans(id, task_id, operation_type, source_path, target_path, reason, risk_level, evidence_json, state)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("store: prepare plan insert: %w", err)
	}
	defer prep.Close()
	for _, p := range plans {
		primary := derivePrimaryAction(p)
		blob, err := json.Marshal(planEnvelope{Plan: p})
		if err != nil {
			return fmt.Errorf("store: marshal plan %s: %w", p.ID, err)
		}
		if _, err := prep.ExecContext(ctx,
			p.ID, taskID, string(primary.Action), primary.SourcePath, primary.TargetPath,
			primary.Reason, string(p.Risk), string(blob), string(p.State)); err != nil {
			return fmt.Errorf("store: insert plan %s: %w", p.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit plans: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListPlans(ctx context.Context, taskID string) ([]domain.OperationPlan, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT evidence_json FROM operation_plans WHERE task_id = ? ORDER BY id`, taskID)
	if err != nil {
		return nil, fmt.Errorf("store: list plans: %w", err)
	}
	defer rows.Close()
	out := make([]domain.OperationPlan, 0)
	for rows.Next() {
		var blob string
		if err := rows.Scan(&blob); err != nil {
			return nil, err
		}
		var env planEnvelope
		if err := json.Unmarshal([]byte(blob), &env); err != nil {
			return nil, fmt.Errorf("store: unmarshal plan: %w", err)
		}
		out = append(out, env.Plan)
	}
	return out, rows.Err()
}

// ListAllPlans returns plans across all tasks, ordered by task creation
// time then plan id. Used by feedback learning (L4).
func (s *SQLiteStore) ListAllPlans(ctx context.Context) ([]domain.OperationPlan, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT p.evidence_json
		 FROM operation_plans p
		 JOIN operation_tasks t ON t.id = p.task_id
		 ORDER BY t.created_at, p.id`)
	if err != nil {
		return nil, fmt.Errorf("store: list all plans: %w", err)
	}
	defer rows.Close()
	out := make([]domain.OperationPlan, 0)
	for rows.Next() {
		var blob string
		if err := rows.Scan(&blob); err != nil {
			return nil, err
		}
		var env planEnvelope
		if err := json.Unmarshal([]byte(blob), &env); err != nil {
			return nil, fmt.Errorf("store: unmarshal plan: %w", err)
		}
		out = append(out, env.Plan)
	}
	return out, rows.Err()
}

// GetPlan loads a single plan by its ID. Used by crash recovery (P0-1).
// The denormalized state column is authoritative (it reflects
// UpdatePlanState transitions); the State inside evidence_json may be
// stale from the original SavePlans call.
func (s *SQLiteStore) GetPlan(ctx context.Context, planID string) (domain.OperationPlan, error) {
	var p domain.OperationPlan
	var blob, state string
	err := s.db.QueryRowContext(ctx,
		`SELECT evidence_json, state FROM operation_plans WHERE id = ?`, planID).Scan(&blob, &state)
	if err == sql.ErrNoRows {
		return p, ErrNotFound
	}
	if err != nil {
		return p, fmt.Errorf("store: get plan: %w", err)
	}
	var env planEnvelope
	if err := json.Unmarshal([]byte(blob), &env); err != nil {
		return p, fmt.Errorf("store: unmarshal plan: %w", err)
	}
	p = env.Plan
	p.State = domain.PlanState(state) // state column is authoritative
	return p, nil
}

// UpdatePlanState persists a plan's state transition. Used by crash
// recovery to mark a plan as ROLLED_BACK or reset to APPROVED.
func (s *SQLiteStore) UpdatePlanState(ctx context.Context, planID string, state domain.PlanState) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE operation_plans SET state = ? WHERE id = ?`, string(state), planID)
	if err != nil {
		return fmt.Errorf("store: update plan state: %w", err)
	}
	return nil
}

// planEnvelope wraps an OperationPlan for JSON storage. The envelope key
// future-proofs the blob against later embedding task-level metadata.
type planEnvelope struct {
	Plan domain.OperationPlan `json:"plan"`
}

// primaryActionSummary is the denormalized view (operation_type,
// source_path, target_path, reason) stored alongside evidence_json. We use
// the retain action when present so the row points at the kept copy;
// otherwise the first action. This is informational — the canonical data
// lives in evidence_json.
type primaryActionSummary struct {
	Action     domain.OperationType
	SourcePath string
	TargetPath string
	Reason     string
}

func derivePrimaryAction(p domain.OperationPlan) primaryActionSummary {
	if len(p.Actions) == 0 {
		return primaryActionSummary{Reason: firstOr(p.Evidence, "")}
	}
	if p.RetainPath != "" {
		for _, a := range p.Actions {
			if a.Path == p.RetainPath {
				return primaryActionSummary{Action: a.Action, SourcePath: a.Path, Reason: a.Reason}
			}
		}
	}
	a := p.Actions[0]
	return primaryActionSummary{Action: a.Action, SourcePath: a.Path, Reason: a.Reason}
}

func firstOr(s []string, def string) string {
	if len(s) == 0 {
		return def
	}
	return s[0]
}

// ---------------- operation_logs ----------------

func (s *SQLiteStore) AppendLog(ctx context.Context, planID, eventType string, detail map[string]any) error {
	blob, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("store: marshal log detail: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO operation_logs(plan_id, event_type, detail_json, created_at) VALUES(?, ?, ?, ?)`,
		planID, eventType, string(blob), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: append log: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListLogs(ctx context.Context, planID string) ([]domain.OperationLog, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, plan_id, event_type, detail_json, created_at FROM operation_logs WHERE plan_id = ? ORDER BY id`, planID)
	if err != nil {
		return nil, fmt.Errorf("store: list logs: %w", err)
	}
	defer rows.Close()
	out := make([]domain.OperationLog, 0)
	for rows.Next() {
		var l domain.OperationLog
		var detailBlob, createdAt string
		if err := rows.Scan(&l.ID, &l.PlanID, &l.EventType, &detailBlob, &createdAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(detailBlob), &l.Detail)
		l.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		out = append(out, l)
	}
	return out, rows.Err()
}

// ---------------- execution journal (P0-1 崩溃恢复) ----------------

// journalTouchesFilesystem mirrors executor.touchesFilesystem. Kept local
// to avoid a cross-package dependency; the semantics are stable (KEEP/SKIP/
// REVIEW are advisory-only and never touch files).
func journalTouchesFilesystem(action domain.OperationType) bool {
	switch action {
	case domain.OperationKeep, domain.OperationSkip, domain.OperationReview:
		return false
	default:
		return true
	}
}

// BeginJournal 为 plan 的所有 filesystem action 写入 pending 记录。
// 已存在的记录不重复写入（INSERT OR IGNORE），保证幂等。
func (s *SQLiteStore) BeginJournal(ctx context.Context, taskID, planID string, actions []domain.PlannedAction) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i, action := range actions {
		if !journalTouchesFilesystem(action.Action) {
			continue
		}
		_, err := s.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO execution_journal
			  (plan_id, task_id, action_index, action_type, source_path, target_path, content_sha256, file_size, status, started_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?)`,
			planID, taskID, i, string(action.Action),
			action.Path, nullIfEmpty(action.TargetPath),
			action.File.ContentSHA256, action.File.Size, now)
		if err != nil {
			return fmt.Errorf("store: begin journal: %w", err)
		}
	}
	return nil
}

// MarkJournalDone 标记 action 执行完成，并记录实际目标路径。
// actualTargetPath 对 MOVE/COPY/RENAME 等于 plan 中的 TargetPath；对
// QUARANTINE/DELETE 是运行时解析出的隔离路径，回滚时需要此路径。
func (s *SQLiteStore) MarkJournalDone(ctx context.Context, planID string, actionIndex int, actualTargetPath string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE execution_journal SET status = 'done', target_path = ?, completed_at = ? WHERE plan_id = ? AND action_index = ?`,
		nullIfEmpty(actualTargetPath), time.Now().UTC().Format(time.RFC3339Nano), planID, actionIndex)
	if err != nil {
		return fmt.Errorf("store: mark journal done: %w", err)
	}
	return nil
}

// MarkJournalFailed 标记 action 执行失败。
func (s *SQLiteStore) MarkJournalFailed(ctx context.Context, planID string, actionIndex int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE execution_journal SET status = 'failed', completed_at = ? WHERE plan_id = ? AND action_index = ?`,
		time.Now().UTC().Format(time.RFC3339Nano), planID, actionIndex)
	if err != nil {
		return fmt.Errorf("store: mark journal failed: %w", err)
	}
	return nil
}

// MarkJournalRolledBack 标记 action 已回滚。rollbackErr 非 nil 时记为 failed。
func (s *SQLiteStore) MarkJournalRolledBack(ctx context.Context, planID string, actionIndex int, rollbackErr error) error {
	status := "done"
	if rollbackErr != nil {
		status = "failed"
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE execution_journal SET rollback_status = ? WHERE plan_id = ? AND action_index = ?`,
		status, planID, actionIndex)
	if err != nil {
		return fmt.Errorf("store: mark journal rolled back: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListJournalDone(ctx context.Context, planID string) ([]JournalEntry, error) {
	return s.listJournalByStatus(ctx, planID, "done")
}

func (s *SQLiteStore) ListJournalPending(ctx context.Context, planID string) ([]JournalEntry, error) {
	return s.listJournalByStatus(ctx, planID, "pending")
}

func (s *SQLiteStore) ListJournalAll(ctx context.Context, planID string) ([]JournalEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT plan_id, task_id, action_index, action_type, source_path, target_path,
		        content_sha256, file_size, status, rollback_status, started_at, completed_at
		 FROM execution_journal WHERE plan_id = ? ORDER BY action_index`, planID)
	if err != nil {
		return nil, fmt.Errorf("store: list journal all: %w", err)
	}
	return scanJournalEntries(rows)
}

func (s *SQLiteStore) listJournalByStatus(ctx context.Context, planID, status string) ([]JournalEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT plan_id, task_id, action_index, action_type, source_path, target_path,
		        content_sha256, file_size, status, rollback_status, started_at, completed_at
		 FROM execution_journal WHERE plan_id = ? AND status = ? ORDER BY action_index`,
		planID, status)
	if err != nil {
		return nil, fmt.Errorf("store: list journal %s: %w", status, err)
	}
	return scanJournalEntries(rows)
}

func scanJournalEntries(rows *sql.Rows) ([]JournalEntry, error) {
	defer rows.Close()
	out := make([]JournalEntry, 0)
	for rows.Next() {
		var e JournalEntry
		var targetPath, rollbackStatus, startedAt, completedAt sql.NullString
		if err := rows.Scan(
			&e.PlanID, &e.TaskID, &e.ActionIndex, &e.ActionType,
			&e.SourcePath, &targetPath, &e.ContentSHA256, &e.FileSize,
			&e.Status, &rollbackStatus, &startedAt, &completedAt,
		); err != nil {
			return nil, err
		}
		e.TargetPath = targetPath.String
		e.RollbackStatus = rollbackStatus.String
		if startedAt.Valid {
			t, _ := time.Parse(time.RFC3339Nano, startedAt.String)
			e.StartedAt = &t
		}
		if completedAt.Valid {
			t, _ := time.Parse(time.RFC3339Nano, completedAt.String)
			e.CompletedAt = &t
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListExecutingPlans 列出 state=EXECUTING 的 plan_id。
func (s *SQLiteStore) ListExecutingPlans(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM operation_plans WHERE state = 'EXECUTING'`)
	if err != nil {
		return nil, fmt.Errorf("store: list executing plans: %w", err)
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ---------------- incremental scan (P0-2) ----------------

// ListFileMetadata returns metadata for all active files in a storage.
// Used by the incremental scanner to detect unchanged files and skip
// hash recomputation.
func (s *SQLiteStore) ListFileMetadata(ctx context.Context, storageID string) ([]FileMeta, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT path, size, mtime, device, inode, quick_hash, content_sha256
		 FROM file_instances
		 WHERE storage_id = ? AND file_status = 'active'
		 ORDER BY path`, storageID)
	if err != nil {
		return nil, fmt.Errorf("store: list file metadata: %w", err)
	}
	defer rows.Close()
	out := make([]FileMeta, 0)
	for rows.Next() {
		var m FileMeta
		var mtime string
		var device, inode sql.NullInt64
		var quickHash, contentSHA256 sql.NullString
		if err := rows.Scan(&m.Path, &m.Size, &mtime, &device, &inode, &quickHash, &contentSHA256); err != nil {
			return nil, err
		}
		m.ModifiedAt, _ = time.Parse(time.RFC3339Nano, mtime)
		if device.Valid {
			m.Device = uint64(device.Int64)
		}
		if inode.Valid {
			m.Inode = uint64(inode.Int64)
		}
		m.QuickHash = quickHash.String
		m.ContentSHA256 = contentSHA256.String
		out = append(out, m)
	}
	return out, rows.Err()
}

// MarkFilesMissing sets file_status='missing' for paths not seen in the
// current scan. Uses a parameterized IN clause built safely.
func (s *SQLiteStore) MarkFilesMissing(ctx context.Context, storageID string, paths []string) (int64, error) {
	return s.reconcileUnseen(ctx, storageID, paths, "missing")
}

// MarkFilesUnavailable is used after a partial traversal. Unseen rows remain
// auditable but cannot participate in current-snapshot diagnostics or plans.
func (s *SQLiteStore) MarkFilesUnavailable(ctx context.Context, storageID string, paths []string) (int64, error) {
	return s.reconcileUnseen(ctx, storageID, paths, "unavailable")
}

// reconcileUnseen uses a temporary table instead of a giant NOT IN clause.
// Real NAS snapshots commonly exceed SQLite's host-parameter limit.
func (s *SQLiteStore) reconcileUnseen(ctx context.Context, storageID string, paths []string, status string) (int64, error) {
	if status != "missing" && status != "unavailable" {
		return 0, fmt.Errorf("store: invalid reconciliation status")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("store: begin file reconciliation: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.ExecContext(ctx, `CREATE TEMP TABLE IF NOT EXISTS scan_seen_paths(path TEXT PRIMARY KEY) WITHOUT ROWID`); err != nil {
		return 0, fmt.Errorf("store: create seen-path table: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM scan_seen_paths`); err != nil {
		return 0, fmt.Errorf("store: reset seen-path table: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO scan_seen_paths(path) VALUES(?)`)
	if err != nil {
		return 0, fmt.Errorf("store: prepare seen path: %w", err)
	}
	for _, path := range paths {
		if _, err := stmt.ExecContext(ctx, path); err != nil {
			_ = stmt.Close()
			return 0, fmt.Errorf("store: insert seen path: %w", err)
		}
	}
	if err := stmt.Close(); err != nil {
		return 0, err
	}
	eligible := "file_status = 'active'"
	if status == "missing" {
		eligible = "file_status IN ('active','unavailable')"
	}
	query := fmt.Sprintf(`UPDATE file_instances SET file_status = ?
		WHERE storage_id = ? AND %s
		AND NOT EXISTS (SELECT 1 FROM scan_seen_paths s WHERE s.path = file_instances.path)`, eligible)
	res, err := tx.ExecContext(ctx, query, status, storageID)
	if err != nil {
		return 0, fmt.Errorf("store: reconcile unseen files: %w", err)
	}
	count, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("store: commit file reconciliation: %w", err)
	}
	return count, nil
}

// MarkFileActive sets file_status='active' for a single path.
func (s *SQLiteStore) MarkFileActive(ctx context.Context, storageID, path string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE file_instances SET file_status = 'active'
		 WHERE storage_id = ? AND path = ?`, storageID, path)
	if err != nil {
		return fmt.Errorf("store: mark file active: %w", err)
	}
	return nil
}

// ---------------- scan checkpoints (P0-2) ----------------

// StartCheckpoint creates a new scan checkpoint in 'running' state.
func (s *SQLiteStore) StartCheckpoint(ctx context.Context, storageID string) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO scan_checkpoints(storage_id, last_scanned_path, scanned_count, status, started_at, updated_at)
		 VALUES (?, '', 0, 'running', ?, ?)`,
		storageID, now, now)
	if err != nil {
		return 0, fmt.Errorf("store: start checkpoint: %w", err)
	}
	return res.LastInsertId()
}

// UpdateCheckpoint updates the checkpoint's progress fields.
func (s *SQLiteStore) UpdateCheckpoint(ctx context.Context, checkpointID int64, lastPath string, scannedCount int) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx,
		`UPDATE scan_checkpoints SET last_scanned_path = ?, scanned_count = ?, updated_at = ?
		 WHERE id = ?`,
		lastPath, scannedCount, now, checkpointID)
	if err != nil {
		return fmt.Errorf("store: update checkpoint: %w", err)
	}
	return nil
}

// CompleteCheckpoint marks a checkpoint as 'completed' or 'aborted'.
func (s *SQLiteStore) CompleteCheckpoint(ctx context.Context, checkpointID int64, status string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx,
		`UPDATE scan_checkpoints SET status = ?, updated_at = ? WHERE id = ?`,
		status, now, checkpointID)
	if err != nil {
		return fmt.Errorf("store: complete checkpoint: %w", err)
	}
	return nil
}

// LastCheckpoint returns the most recent incomplete checkpoint for a storage.
func (s *SQLiteStore) LastCheckpoint(ctx context.Context, storageID string) (Checkpoint, error) {
	var cp Checkpoint
	var startedAt, updatedAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, storage_id, last_scanned_path, scanned_count, status, started_at, updated_at
		 FROM scan_checkpoints
		 WHERE storage_id = ? AND status = 'running'
		 ORDER BY started_at DESC LIMIT 1`, storageID).
		Scan(&cp.ID, &cp.StorageID, &cp.LastScannedPath, &cp.ScannedCount, &cp.Status, &startedAt, &updatedAt)
	if err == sql.ErrNoRows {
		return cp, ErrNotFound
	}
	if err != nil {
		return cp, fmt.Errorf("store: last checkpoint: %w", err)
	}
	cp.StartedAt, _ = time.Parse(time.RFC3339Nano, startedAt)
	cp.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return cp, nil
}
