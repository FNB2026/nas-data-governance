package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/report"
)

// SourcePurgeFacts are obtained from one project snapshot. This is index
// evidence only. The Worker independently establishes live filesystem facts.
type SourcePurgeFacts struct {
	StorageID, RootPath, GenerationTime, GroupID, PolicyDigest string
	Generation                                                 int64
	Target, Keeper                                             SourcePurgeFile
}

type SourcePurgeFile struct {
	ID, Size                                   int64
	Device, Inode                              uint64
	Mode                                       uint32
	Path, SHA256, MTime, Discovered, Directory string
	Format, RuleVersion                        string
}

func readSourcePurgeFacts(ctx context.Context, r governanceReader, storageID string, targetID, keeperID int64) (SourcePurgeFacts, error) {
	facts := SourcePurgeFacts{StorageID: storageID}
	if storageID == "" || len(storageID) > 128 || targetID <= 0 || keeperID <= 0 || targetID == keeperID {
		return facts, ErrGovernanceRejected
	}
	var started, status string
	err := r.QueryRowContext(ctx, `SELECT s.root_path, c.id, c.started_at, c.updated_at, c.status
		FROM storages s JOIN scan_checkpoints c ON c.storage_id = s.id
		WHERE s.id = ? AND s.kind = 'local' ORDER BY c.id DESC LIMIT 1`, storageID).
		Scan(&facts.RootPath, &facts.Generation, &started, &facts.GenerationTime, &status)
	if err != nil || status != "completed" {
		return facts, ErrGovernanceRejected
	}
	minTime, err := time.Parse(time.RFC3339Nano, started)
	maxTime, timeErr := time.Parse(time.RFC3339Nano, facts.GenerationTime)
	if err != nil || timeErr != nil || maxTime.Before(minTime) {
		return facts, ErrGovernanceRejected
	}
	for _, row := range []struct {
		id   int64
		file *SourcePurgeFile
	}{{targetID, &facts.Target}, {keeperID, &facts.Keeper}} {
		f := row.file
		err := r.QueryRowContext(ctx, `SELECT f.id,f.path,f.size,f.mode,f.mtime,f.device,f.inode,f.content_sha256,
			f.discovered_at,d.context_json,d.rule_version,m.format_json
			FROM file_instances f JOIN directory_contexts d ON d.file_id=f.id JOIN file_formats m ON m.file_id=f.id
			WHERE f.id=? AND f.storage_id=? AND f.file_status='active' AND f.physical_reliable=1 AND f.link_count=1`,
			row.id, storageID).Scan(&f.ID, &f.Path, &f.Size, &f.Mode, &f.MTime, &f.Device, &f.Inode, &f.SHA256,
			&f.Discovered, &f.Directory, &f.RuleVersion, &f.Format)
		if err != nil || f.Device == 0 || f.Inode == 0 || f.Size < 0 {
			return facts, ErrGovernanceRejected
		}
		discovered, err := time.Parse(time.RFC3339Nano, f.Discovered)
		if err != nil || discovered.Before(minTime) || discovered.After(maxTime) {
			return facts, ErrGovernanceRejected
		}
	}
	if facts.Target.Size != facts.Keeper.Size || facts.Target.SHA256 != facts.Keeper.SHA256 ||
		(facts.Target.Device == facts.Keeper.Device && facts.Target.Inode == facts.Keeper.Inode) {
		return facts, ErrGovernanceRejected
	}
	facts.GroupID = report.StableGroupID(storageID, facts.Target.SHA256)
	group, _, err := governanceSubject(ctx, r, domain.GovernanceSubject{StorageID: storageID, GroupID: facts.GroupID, SHA256: facts.Target.SHA256})
	if err != nil {
		return facts, ErrGovernanceRejected
	}
	// A storage with manually protected paths needs individually audited
	// policy evaluation before source deletion can be admitted.
	var protected bool
	if err := r.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM protected_paths WHERE storage_id=?
		UNION ALL SELECT 1 FROM quarantine_items WHERE source_path IN (?,?) AND COALESCE(hold_reason,'') <> '')`,
		storageID, facts.Target.Path, facts.Keeper.Path).Scan(&protected); err != nil || protected {
		return facts, ErrGovernanceRejected
	}
	var decisionID string
	var decision domain.ReviewDecisionType
	var retained sql.NullInt64
	err = r.QueryRowContext(ctx, `SELECT id,decision_type,retained_file_id FROM group_decisions
		WHERE group_id=? ORDER BY `+decisionTimestampOrderFunction+`(updated_at) DESC,id DESC LIMIT 1`, facts.GroupID).Scan(&decisionID, &decision, &retained)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return facts, ErrGovernanceRejected
	}
	switch decision {
	case domain.DecisionUnreviewed, domain.DecisionDraftAction:
	case domain.DecisionPrimaryRetention:
		if !retained.Valid || retained.Int64 != keeperID {
			return facts, ErrGovernanceRejected
		}
	default:
		return facts, ErrGovernanceRejected
	}
	facts.PolicyDigest = group.SubjectVersion + ":" + decisionID + ":" + string(decision)
	return facts, nil
}

func (s *SQLiteStore) SourcePurgeSnapshot(ctx context.Context, storageID string, targetID, keeperID int64) (SourcePurgeFacts, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SourcePurgeFacts{}, err
	}
	defer tx.Rollback() //nolint:errcheck
	return readSourcePurgeFacts(ctx, tx, storageID, targetID, keeperID)
}

func (s *SQLiteStore) AdvanceSourcePurge(ctx context.Context, plan DualPurgePlan, proof SourcePurgeFacts, next string, now time.Time) error {
	ok := plan.State == "DRAFT" && next == "APPROVED" || plan.State == "APPROVED" && next == "DRY_RUN" ||
		plan.State == "DRY_RUN" && next == "EXECUTING" ||
		(plan.State == "DRAFT" || plan.State == "APPROVED" || plan.State == "DRY_RUN") && next == "CANCELLED"
	if !ok || plan.Operation != SourceDirectPurge || !now.Before(plan.ExpiresAt) {
		return ErrGovernanceRejected
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if next != "CANCELLED" {
		recovery, scan, err := governanceGuard(ctx, tx)
		if err != nil || recovery || scan {
			return ErrGovernanceRejected
		}
		current, err := readSourcePurgeFacts(ctx, tx, proof.StorageID, proof.Target.ID, proof.Keeper.ID)
		if err != nil || current != proof {
			return ErrGovernanceRejected
		}
	}
	if err := updateSourcePurge(ctx, tx, plan, next, now); err != nil {
		return err
	}
	return tx.Commit()
}

func updateSourcePurge(ctx context.Context, tx *sql.Tx, plan DualPurgePlan, next string, now time.Time) error {
	result, err := tx.ExecContext(ctx, `UPDATE dual_purge_plans SET state=?, updated_at=? WHERE id=? AND
		operation=? AND subject_id=? AND digest=? AND evidence_json=? AND state=? AND expires_at=?`,
		next, formatTime(now), plan.ID, plan.Operation, plan.SubjectID, plan.Digest, plan.EvidenceJSON, plan.State, formatTime(plan.ExpiresAt))
	if err != nil {
		return err
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return ErrGovernanceRejected
	}
	return nil
}

func (s *SQLiteStore) FinishSourcePurge(ctx context.Context, plan DualPurgePlan, proof SourcePurgeFacts, next string, now time.Time) error {
	if plan.Operation != SourceDirectPurge || plan.State != "EXECUTING" || (next != "PURGED" && next != "ROLLED_BACK") {
		return ErrGovernanceRejected
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if err := updateSourcePurge(ctx, tx, plan, next, now); err != nil {
		return err
	}
	if next == "PURGED" {
		current, err := readSourcePurgeFacts(ctx, tx, proof.StorageID, proof.Target.ID, proof.Keeper.ID)
		if err != nil || current != proof {
			return ErrGovernanceRejected
		}
		if _, err := tx.ExecContext(ctx, `UPDATE file_instances SET file_status='missing' WHERE id=?`, proof.Target.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SourcePurgeDecision checks recorded directory semantics and format
// protection before a filesystem mutation can be proposed.
func (f SourcePurgeFacts) SourcePurgeDecision(ruleVersion string) (domain.DirectoryRole, error) {
	var contexts [2]domain.DirectoryContext
	for i, file := range []SourcePurgeFile{f.Target, f.Keeper} {
		var format domain.FormatInfo
		if json.Unmarshal([]byte(file.Directory), &contexts[i]) != nil ||
			json.Unmarshal([]byte(file.Format), &format) != nil || format.Protected || format.Format == "" ||
			format.Role == domain.FormatRoleMetadataSidecar || format.Role == domain.FormatRoleProjectSource ||
			file.RuleVersion != ruleVersion || contexts[i].Protected || contexts[i].PrivacyLevel != "normal" ||
			contexts[i].BranchPoint != "" {
			return "", ErrGovernanceRejected
		}
		switch contexts[i].Role {
		case domain.RoleTemporary, domain.RoleCache, domain.RoleUnorganized:
		default:
			return "", ErrGovernanceRejected
		}
		for _, ancestor := range contexts[i].ParentChain {
			switch ancestor.Role {
			case domain.RoleRaw, domain.RoleSensitive, domain.RoleBackup, domain.RoleSystem, domain.RoleFormalArchive:
				return "", ErrGovernanceRejected
			}
		}
	}
	if contexts[0].Role != contexts[1].Role || contexts[0].BusinessAnchor != contexts[1].BusinessAnchor {
		return "", ErrGovernanceRejected
	}
	return contexts[0].Role, nil
}

// SourcePurgeCandidates is a bounded display view. Every execution plan
// independently validates the selected target and keeper against a new snapshot.
func (s *SQLiteStore) SourcePurgeCandidates(ctx context.Context, root string) ([]SourcePurgeChoice, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT s.id,f.id,f.path,f.size,f.content_sha256,
		COALESCE(d.role,''),COALESCE(d.protected,1),COALESCE(d.privacy_level,'')
		FROM storages s JOIN file_instances f ON f.storage_id=s.id
		LEFT JOIN directory_contexts d ON d.file_id=f.id
		WHERE s.root_path=? AND s.kind='local' AND f.file_status='active'
		AND f.physical_reliable=1 AND f.link_count=1 AND length(f.content_sha256)=64
		AND EXISTS(SELECT 1 FROM file_instances k WHERE k.storage_id=f.storage_id
		AND k.file_status='active' AND k.content_sha256=f.content_sha256 AND k.size=f.size
		AND k.physical_reliable=1 AND k.link_count=1 AND (k.device<>f.device OR k.inode<>f.inode))
		ORDER BY f.content_sha256,f.id LIMIT 50`, root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	choices := make([]SourcePurgeChoice, 0)
	for rows.Next() {
		var c SourcePurgeChoice
		var privacy string
		var protected bool
		if err := rows.Scan(&c.StorageID, &c.FileID, &c.Path, &c.Size, &c.SHA256, &c.Role, &protected, &privacy); err != nil {
			return nil, err
		}
		c.Protected = protected || privacy != "normal"
		if c.Protected {
			// A sensitive path should never appear in the purge interface.
			continue
		}
		choices = append(choices, c)
	}
	return choices, rows.Err()
}

type SourcePurgeChoice struct {
	StorageID string               `json:"storage_id"`
	FileID    int64                `json:"file_id,string"`
	Path      string               `json:"path"`
	Size      int64                `json:"size"`
	SHA256    string               `json:"sha256"`
	Role      domain.DirectoryRole `json:"role"`
	Protected bool                 `json:"protected"`
}
