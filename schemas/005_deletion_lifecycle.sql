-- 005: M7 分级删除生命周期
--
-- 源目录执行器仍只允许隔离。永久删除只能作用于 quarantine_items 中
-- 已到保留期、未处于 HOLD、经过独立二次审批的受管隔离项。

CREATE TABLE IF NOT EXISTS quarantine_items (
  id TEXT PRIMARY KEY,
  plan_id TEXT NOT NULL REFERENCES operation_plans(id),
  action_index INTEGER NOT NULL,
  source_path TEXT NOT NULL,
  quarantine_path TEXT NOT NULL UNIQUE,
  content_sha256 TEXT NOT NULL,
  file_size INTEGER NOT NULL,
  quarantined_at TEXT NOT NULL,
  retain_until TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'QUARANTINED',
  hold_reason TEXT,
  restored_at TEXT,
  purged_at TEXT,
  UNIQUE(plan_id, action_index)
);
CREATE INDEX IF NOT EXISTS idx_quarantine_status_retention
  ON quarantine_items(status, retain_until);

CREATE TABLE IF NOT EXISTS restore_plans (
  id TEXT PRIMARY KEY,
  item_id TEXT NOT NULL REFERENCES quarantine_items(id),
  state TEXT NOT NULL DEFAULT 'DRAFT',
  quarantine_path TEXT NOT NULL,
  restore_path TEXT NOT NULL,
  expected_sha256 TEXT NOT NULL,
  expected_size INTEGER NOT NULL,
  approval_digest TEXT NOT NULL,
  created_at TEXT NOT NULL,
  approved_at TEXT,
  restored_at TEXT
);

CREATE TABLE IF NOT EXISTS restore_journal (
  id INTEGER PRIMARY KEY,
  plan_id TEXT NOT NULL UNIQUE REFERENCES restore_plans(id),
  item_id TEXT NOT NULL REFERENCES quarantine_items(id),
  quarantine_path TEXT NOT NULL,
  restore_path TEXT NOT NULL,
  content_sha256 TEXT NOT NULL,
  file_size INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  started_at TEXT NOT NULL,
  completed_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_restore_journal_recovery
  ON restore_journal(status)
  WHERE status = 'pending';

CREATE TABLE IF NOT EXISTS purge_plans (
  id TEXT PRIMARY KEY,
  item_id TEXT NOT NULL REFERENCES quarantine_items(id),
  state TEXT NOT NULL DEFAULT 'DRAFT',
  expected_path TEXT NOT NULL,
  expected_sha256 TEXT NOT NULL,
  expected_size INTEGER NOT NULL,
  retain_until TEXT NOT NULL,
  approval_digest TEXT NOT NULL,
  created_at TEXT NOT NULL,
  approved_at TEXT,
  purged_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_purge_plans_state ON purge_plans(state);

CREATE TABLE IF NOT EXISTS purge_journal (
  id INTEGER PRIMARY KEY,
  plan_id TEXT NOT NULL UNIQUE REFERENCES purge_plans(id),
  item_id TEXT NOT NULL REFERENCES quarantine_items(id),
  quarantine_path TEXT NOT NULL,
  staging_path TEXT NOT NULL,
  content_sha256 TEXT NOT NULL,
  file_size INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  started_at TEXT NOT NULL,
  completed_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_purge_journal_recovery
  ON purge_journal(status)
  WHERE status IN ('pending', 'staged', 'commit_pending');
