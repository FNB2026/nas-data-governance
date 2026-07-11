PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;

CREATE TABLE IF NOT EXISTS storages (id TEXT PRIMARY KEY, root_path TEXT NOT NULL, kind TEXT NOT NULL, created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS file_instances (
  id INTEGER PRIMARY KEY, storage_id TEXT NOT NULL REFERENCES storages(id), path TEXT NOT NULL,
  name TEXT NOT NULL, size INTEGER NOT NULL, mode INTEGER NOT NULL, mtime TEXT NOT NULL,
  device INTEGER, inode INTEGER, quick_hash TEXT, content_sha256 TEXT, discovered_at TEXT NOT NULL,
  verified_at TEXT, UNIQUE(storage_id, path)
);
CREATE INDEX IF NOT EXISTS idx_files_size_quick ON file_instances(size, quick_hash);
CREATE INDEX IF NOT EXISTS idx_files_content ON file_instances(content_sha256);
CREATE TABLE IF NOT EXISTS directory_contexts (id INTEGER PRIMARY KEY, file_id INTEGER NOT NULL UNIQUE REFERENCES file_instances(id), context_json TEXT NOT NULL, rule_version TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS file_formats (id INTEGER PRIMARY KEY, file_id INTEGER NOT NULL UNIQUE REFERENCES file_instances(id), format_name TEXT NOT NULL, category TEXT NOT NULL, format_json TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS protected_paths (id INTEGER PRIMARY KEY, storage_id TEXT NOT NULL REFERENCES storages(id), path_prefix TEXT NOT NULL, reason TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS rules (id TEXT PRIMARY KEY, version TEXT NOT NULL, priority INTEGER NOT NULL, enabled INTEGER NOT NULL DEFAULT 1, definition_yaml TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS operation_tasks (id TEXT PRIMARY KEY, root_path TEXT NOT NULL, state TEXT NOT NULL, created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS operation_plans (id TEXT PRIMARY KEY, task_id TEXT NOT NULL REFERENCES operation_tasks(id), operation_type TEXT NOT NULL, source_path TEXT NOT NULL, target_path TEXT, reason TEXT NOT NULL, risk_level TEXT NOT NULL, evidence_json TEXT NOT NULL, state TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS operation_logs (id INTEGER PRIMARY KEY, plan_id TEXT NOT NULL REFERENCES operation_plans(id), event_type TEXT NOT NULL, detail_json TEXT NOT NULL, created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);
