package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"nas-data-governance/internal/domain"

	_ "modernc.org/sqlite"
)

// SQLiteStore implements Store on top of a SQLite database file.
type SQLiteStore struct {
	db *sql.DB
}

// Open creates (or opens) a SQLite database at path and applies migrations.
// The database is project-owned; it never reads or writes user files.
func Open(ctx context.Context, path string) (*SQLiteStore, error) {
	// _txlock=immediate makes write transactions acquire the write lock up
	// front, avoiding "database is locked" mid-transaction. foreign_keys &
	// busy_timeout make the behavior safer under concurrent readers.
	dsn := fmt.Sprintf("file:%s?_txlock=immediate&_busy_timeout=5000", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite serializes writes; one conn avoids lock storms.
	s := &SQLiteStore{db: db}
	if err := s.Init(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLiteStore) Close() error { return s.db.Close() }

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
	for _, f := range files {
		var id int64
		err := prep.QueryRowContext(ctx,
			f.StorageID, f.Path, f.Name, f.Size, f.Mode,
			f.ModifiedAt.UTC().Format(time.RFC3339Nano),
			f.Device, f.Inode, f.QuickHash, f.ContentSHA256,
			f.DiscoveredAt.UTC().Format(time.RFC3339Nano),
		).Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("store: upsert file %q: %w", f.Path, err)
		}
		ids = append(ids, id)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: commit upsert: %w", err)
	}
	return ids, nil
}

func (s *SQLiteStore) ListFiles(ctx context.Context, storageID string) ([]domain.FileInstance, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT storage_id, path, name, size, mode, mtime, device, inode, quick_hash, content_sha256, discovered_at
		FROM file_instances WHERE storage_id = ? ORDER BY path`, storageID)
	if err != nil {
		return nil, fmt.Errorf("store: list files: %w", err)
	}
	defer rows.Close()
	out := make([]domain.FileInstance, 0)
	for rows.Next() {
		var f domain.FileInstance
		var mtime, discoveredAt string
		if err := rows.Scan(&f.StorageID, &f.Path, &f.Name, &f.Size, &f.Mode, &mtime,
			&f.Device, &f.Inode, &f.QuickHash, &f.ContentSHA256, &discoveredAt); err != nil {
			return nil, err
		}
		f.ModifiedAt, _ = time.Parse(time.RFC3339Nano, mtime)
		f.DiscoveredAt, _ = time.Parse(time.RFC3339Nano, discoveredAt)
		out = append(out, f)
	}
	return out, rows.Err()
}

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
