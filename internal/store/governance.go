package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"modernc.org/sqlite"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/report"
	"github.com/FNB2026/nas-data-governance/internal/version"
)

var ErrGovernanceRejected = errors.New("governance request rejected")

func init() {
	sqlite.MustRegisterDeterministicScalarFunction("ndg_group_id_v1", 2, func(_ *sqlite.FunctionContext, a []driver.Value) (driver.Value, error) {
		s, sok := a[0].(string)
		h, hok := a[1].(string)
		if !sok || !hok {
			return nil, ErrGovernanceRejected
		}
		return report.StableGroupID(s, h), nil
	})
}

type governanceReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func opaqueID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", ErrGovernanceRejected
	}
	return hex.EncodeToString(b), nil
}
func requestHash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (s *SQLiteStore) SetGovernanceReadOnly() { s.governanceReadOnly = true }
func (s *SQLiteStore) governanceAllowed() bool {
	return s.secureOnClose && s.ownerRelease != nil && !s.governanceReadOnly && !strings.HasPrefix(version.Channel, "web") && !strings.HasPrefix(version.Channel, "fnos")
}

func governanceGuard(ctx context.Context, r governanceReader) (recovery, scan bool, err error) {
	var n int
	q := `SELECT (SELECT COUNT(*) FROM operation_plans WHERE state IN ('STALE_CHECKED','EXECUTING') OR (state='APPROVED' AND EXISTS(SELECT 1 FROM execution_journal WHERE plan_id=operation_plans.id)))+(SELECT COUNT(*) FROM restore_journal WHERE status='pending')+(SELECT COUNT(*) FROM purge_journal WHERE status IN ('pending','staged','commit_pending'))`
	if err = r.QueryRowContext(ctx, q).Scan(&n); err != nil {
		return false, false, ErrGovernanceRejected
	}
	recovery = n > 0
	if err = r.QueryRowContext(ctx, `SELECT COUNT(*) FROM job_runs WHERE state IN ('QUEUED','RUNNING','CANCEL_REQUESTED')`).Scan(&n); err != nil {
		return false, false, ErrGovernanceRejected
	}
	return recovery, n > 0, nil
}
func (s *SQLiteStore) GovernanceCapabilities(ctx context.Context) (domain.GovernanceCapabilities, error) {
	c := domain.GovernanceCapabilities{MaxBatchSize: 100, ReasonCodes: []string{}}
	var migrated int
	if s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='governance_project'`).Scan(&migrated) != nil {
		return c, ErrGovernanceRejected
	}
	if migrated == 0 {
		c.ProjectScope = "unmigrated"
		c.ReasonCodes = append(c.ReasonCodes, "SCHEMA_UPGRADE_REQUIRED")
	} else {
		if err := s.db.QueryRowContext(ctx, `SELECT scope FROM governance_project WHERE id=1`).Scan(&c.ProjectScope); err != nil {
			return c, ErrGovernanceRejected
		}
	}
	if s.scopeBinding == "" {
		return c, ErrGovernanceRejected
	}
	c.ProjectScope += "-" + s.scopeBinding
	r, a, e := governanceGuard(ctx, s.db)
	if e != nil {
		return c, e
	}
	c.RecoveryLockActive = r
	c.ScanActive = a
	c.CanPreview = true
	if !s.governanceAllowed() {
		c.ReasonCodes = append(c.ReasonCodes, "READ_ONLY_CHANNEL")
	}
	if r {
		c.ReasonCodes = append(c.ReasonCodes, "RECOVERY_LOCK")
	}
	if a {
		c.ReasonCodes = append(c.ReasonCodes, "SCAN_ACTIVE")
	}
	c.CanRecordDecisions = len(c.ReasonCodes) == 0
	return c, nil
}
func supportedDecision(d domain.ReviewDecisionType) bool {
	switch d {
	case domain.DecisionKeepAll, domain.DecisionDraftAction, domain.DecisionDeferred, domain.DecisionRejectedSuggestion, domain.DecisionCrossArchive, domain.DecisionBackupRelation:
		return true
	}
	return false
}

// Subject fingerprints cover indexed members, contexts, protection, rule
// versions and related plans. They are stale checks for recording intent, never
// evidence that a source is unchanged enough for a file execution.
func governanceSubject(ctx context.Context, r governanceReader, ref domain.GovernanceSubject) (domain.GovernanceSubject, string, error) {
	if len(ref.StorageID) > 256 || len(ref.SHA256) != 64 || ref.GroupID != report.StableGroupID(ref.StorageID, ref.SHA256) {
		return ref, "", ErrGovernanceRejected
	}
	if _, e := hex.DecodeString(ref.SHA256); e != nil {
		return ref, "", ErrGovernanceRejected
	}
	queries := []string{
		`SELECT f.id,f.path,f.name,f.size,f.mode,f.mtime,f.device,f.inode,f.physical_reliable,f.link_count,f.quick_hash,f.content_sha256,f.file_status,COALESCE(d.context_json,''),COALESCE(d.rule_version,'') FROM file_instances f LEFT JOIN directory_contexts d ON d.file_id=f.id WHERE f.storage_id=? AND f.content_sha256=? AND f.file_status='active' ORDER BY f.id`,
		`SELECT id,path_prefix,reason FROM protected_paths WHERE storage_id=? ORDER BY id`,
		`SELECT id,version,priority,enabled,definition_yaml,source,status FROM rules ORDER BY id`,
		`SELECT id,state,risk_level,evidence_json,source_path,target_path FROM operation_plans WHERE source_path IN (SELECT path FROM file_instances WHERE storage_id=? AND content_sha256=? AND file_status='active') ORDER BY id`,
	}
	h := sha256.New()
	// A preview from different product/rule code cannot silently carry forward.
	h.Write([]byte("governance-intent-v1\x00" + version.Version + "\x00" + version.Commit + "\x00" + version.Channel + "\x00"))
	memberCount := 0
	for i, q := range queries {
		args := []any{ref.StorageID, ref.SHA256}
		if i == 1 {
			args = args[:1]
		}
		if i == 2 {
			args = nil
		}
		rows, e := r.QueryContext(ctx, q, args...)
		if e != nil {
			return ref, "", ErrGovernanceRejected
		}
		cols, e := rows.Columns()
		if e != nil {
			rows.Close()
			return ref, "", ErrGovernanceRejected
		}
		for rows.Next() {
			values := make([]any, len(cols))
			ptr := make([]any, len(cols))
			for j := range values {
				ptr[j] = &values[j]
			}
			if rows.Scan(ptr...) != nil {
				rows.Close()
				return ref, "", ErrGovernanceRejected
			}
			b, _ := json.Marshal(values)
			h.Write(b)
			h.Write([]byte{0})
			if i == 0 {
				memberCount++
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return ref, "", ErrGovernanceRejected
		}
		h.Write([]byte{byte(i)})
	}
	if memberCount < 2 {
		return ref, "", ErrGovernanceRejected
	}
	ref.SubjectVersion = hex.EncodeToString(h.Sum(nil))
	ref.DecisionRevision = 0
	decision := ""
	expr, e := decisionRevisionExpression(ctx, r)
	if e != nil {
		return ref, "", e
	}
	e = r.QueryRowContext(ctx, `SELECT decision_type,`+expr+` FROM group_decisions WHERE group_id=? ORDER BY updated_at DESC,id DESC LIMIT 1`, ref.GroupID).Scan(&decision, &ref.DecisionRevision)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return ref, "", ErrGovernanceRejected
	}
	return ref, decision, nil
}

func decisionRevisionExpression(ctx context.Context, r governanceReader) (string, error) {
	var n int
	if r.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('group_decisions') WHERE name='revision'`).Scan(&n) != nil {
		return "", ErrGovernanceRejected
	}
	if n == 0 {
		return "1", nil
	}
	return "revision", nil
}

// All filters apply to the entire indexed dataset. Filename search is literal
// and intentionally does not support private path search in this increment.
func (s *SQLiteStore) ListGovernanceGroups(ctx context.Context, q domain.GovernanceQuery) (domain.GovernancePage, error) {
	p := domain.GovernancePage{Items: []domain.GovernanceGroup{}, Page: q.Page, PageSize: q.PageSize, UnavailableFilters: []string{"risk", "role", "protected", "executable", "time", "path_search", "filtered_selection"}}
	if p.Page <= 0 {
		p.Page = 1
	}
	if p.Page > 100000 || q.MinSize < 0 || q.MaxSize < 0 || len(q.Search) > 256 || len(q.StorageID) > 256 || len(q.DecisionType) > 64 {
		return p, ErrGovernanceRejected
	}
	if p.PageSize <= 0 || p.PageSize > 200 {
		p.PageSize = 20
	}
	sortColumn := map[string]string{"": "size", "size": "size", "path_count": "path_count", "group_id": "group_id"}[q.Sort]
	if sortColumn == "" {
		return p, ErrGovernanceRejected
	}
	direction := "ASC"
	if q.Descending {
		direction = "DESC"
	}
	cte := `WITH groups AS (SELECT storage_id,content_sha256,ndg_group_id_v1(storage_id,content_sha256) AS group_id,MIN(size) AS size,COUNT(*) AS path_count,MIN(path) AS sample_path FROM file_instances WHERE file_status='active' AND content_sha256 IS NOT NULL AND content_sha256<>'' GROUP BY storage_id,content_sha256 HAVING COUNT(*)>1), reviewed AS (SELECT groups.*,COALESCE((SELECT decision_type FROM group_decisions d WHERE d.group_id=groups.group_id ORDER BY updated_at DESC,id DESC LIMIT 1),'') AS decision_type FROM groups) `
	where := " WHERE 1=1"
	args := []any{}
	if q.StorageID != "" {
		where += " AND storage_id=?"
		args = append(args, q.StorageID)
	}
	if q.DecisionType != "" {
		where += " AND decision_type=?"
		d := q.DecisionType
		if d == "UNREVIEWED" {
			d = ""
		}
		args = append(args, d)
	}
	if q.MinSize > 0 {
		where += " AND size>=?"
		args = append(args, q.MinSize)
	}
	if q.MaxSize > 0 {
		where += " AND size<=?"
		args = append(args, q.MaxSize)
	}
	if q.Search != "" {
		where += ` AND EXISTS(SELECT 1 FROM file_instances f WHERE f.storage_id=reviewed.storage_id AND f.content_sha256=reviewed.content_sha256 AND f.file_status='active' AND instr(lower(f.name),lower(?))>0)`
		args = append(args, q.Search)
	}
	tx, e := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return p, ErrGovernanceRejected
	}
	defer tx.Rollback()
	if tx.QueryRowContext(ctx, cte+"SELECT COUNT(*) FROM reviewed"+where, args...).Scan(&p.TotalCount) != nil {
		return p, ErrGovernanceRejected
	}
	args = append(args, p.PageSize, (p.Page-1)*p.PageSize)
	rows, e := tx.QueryContext(ctx, cte+"SELECT group_id,storage_id,content_sha256,sample_path,size,path_count,decision_type FROM reviewed"+where+" ORDER BY "+sortColumn+" "+direction+",group_id ASC LIMIT ? OFFSET ?", args...)
	if e != nil {
		return p, ErrGovernanceRejected
	}
	for rows.Next() {
		g := domain.GovernanceGroup{Risk: "UNKNOWN", Role: "unknown", SafetyFlags: []string{"RISK_UNAVAILABLE", "ROLE_UNAVAILABLE"}}
		if rows.Scan(&g.GroupID, &g.StorageID, &g.SHA256, &g.SamplePath, &g.Size, &g.PathCount, &g.DecisionType) != nil {
			rows.Close()
			return p, ErrGovernanceRejected
		}
		p.Items = append(p.Items, g)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return p, ErrGovernanceRejected
	}
	for i := range p.Items {
		ref, _, e := governanceSubject(ctx, tx, p.Items[i].GovernanceSubject)
		if e != nil {
			return p, e
		}
		p.Items[i].GovernanceSubject = ref
	}
	if tx.Commit() != nil {
		return p, ErrGovernanceRejected
	}
	return p, nil
}

func governanceAudit(ctx context.Context, tx *sql.Tx, batchID, groupID, old, new, subject string, revision int64, reason, source, status, code, at string) error {
	_, e := tx.ExecContext(ctx, `INSERT INTO governance_decision_audit(batch_id,group_id,old_decision,new_decision,subject_version,decision_revision,reason,source,result,reason_code,actor,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, batchID, groupID, old, new, subject, revision, reason, source, status, code, "local_user", at)
	if e != nil {
		return ErrGovernanceRejected
	}
	return nil
}

func decisionAuditSnapshot(ctx context.Context, tx *sql.Tx, groupID string) (string, string, error) {
	var id, kind, reason, rule, created, updated string
	var retained sql.NullInt64
	var revision int64
	e := tx.QueryRowContext(ctx, `SELECT id,decision_type,retained_file_id,COALESCE(reason,''),COALESCE(rule_id,''),created_at,updated_at,revision FROM group_decisions WHERE group_id=? ORDER BY updated_at DESC,id DESC LIMIT 1`, groupID).Scan(&id, &kind, &retained, &reason, &rule, &created, &updated, &revision)
	if errors.Is(e, sql.ErrNoRows) {
		return "{}", "", nil
	}
	if e != nil {
		return "", "", ErrGovernanceRejected
	}
	value := map[string]any{"id": id, "decision_type": kind, "reason": reason, "rule_id": rule, "created_at": created, "updated_at": updated, "revision": revision}
	if retained.Valid {
		value["retained_file_id"] = retained.Int64
	}
	b, _ := json.Marshal(value)
	return string(b), created, nil
}

func guardedGovernanceWrite(ctx context.Context, s *SQLiteStore, tx *sql.Tx) error {
	if !s.governanceAllowed() {
		return ErrGovernanceRejected
	}
	r, a, e := governanceGuard(ctx, tx)
	if e != nil || r || a {
		return ErrGovernanceRejected
	}
	return nil
}

// CheckDecisionWrite is also used by legacy composition roots before saving.
func (s *SQLiteStore) CheckDecisionWrite(ctx context.Context) error {
	c, e := s.GovernanceCapabilities(ctx)
	if e != nil || !c.CanRecordDecisions {
		return ErrGovernanceRejected
	}
	return nil
}

func validKey(s string) bool {
	if len(s) < 16 || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}
func (s *SQLiteStore) FindGovernanceSubject(ctx context.Context, groupID string) (domain.GovernanceSubject, error) {
	var ref domain.GovernanceSubject
	ref.GroupID = groupID
	if len(groupID) > 128 || s.db.QueryRowContext(ctx, `SELECT storage_id,content_sha256 FROM file_instances WHERE file_status='active' AND ndg_group_id_v1(storage_id,content_sha256)=? GROUP BY storage_id,content_sha256 HAVING COUNT(*)>1 LIMIT 1`, groupID).Scan(&ref.StorageID, &ref.SHA256) != nil {
		return ref, ErrGovernanceRejected
	}
	ref, _, e := governanceSubject(ctx, s.db, ref)
	return ref, e
}
