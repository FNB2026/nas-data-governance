-- 004: 扫描检查点与文件状态标记（增量扫描/断点续扫基础）
--
-- scan_checkpoints 记录每次扫描的进度，支持中断后从断点恢复。
-- file_instances 新增 status 列标记文件是否仍存在（active/missing/unavailable），
-- 用于增量扫描时检测已删除文件。

CREATE TABLE IF NOT EXISTS scan_checkpoints (
  id INTEGER PRIMARY KEY,
  storage_id TEXT NOT NULL REFERENCES storages(id),
  last_scanned_path TEXT NOT NULL DEFAULT '',
  scanned_count INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'running',  -- running | completed | aborted
  started_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_checkpoint_storage ON scan_checkpoints(storage_id, status);
