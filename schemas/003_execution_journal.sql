-- 003: 执行事务日志（崩溃恢复基础）
-- 记录每个 plan 每个 action 的执行状态，使进程崩溃后可以恢复或回滚。
-- 与 operation_plans（已存 source_path）一致，这是受控审计/恢复表，
-- 需要路径才能回滚，非普通日志（K-009 的"普通日志"指对外输出）。

CREATE TABLE IF NOT EXISTS execution_journal (
  id INTEGER PRIMARY KEY,
  plan_id TEXT NOT NULL REFERENCES operation_plans(id),
  task_id TEXT NOT NULL REFERENCES operation_tasks(id),
  action_index INTEGER NOT NULL,              -- action 在 plan.Actions 中的序号
  action_type TEXT NOT NULL,                  -- quarantine/move/copy/delete/rename
  source_path TEXT NOT NULL,                  -- 原始路径（回滚需要）
  target_path TEXT,                           -- 目标路径（move/copy/rename，其他为空）
  content_sha256 TEXT,                        -- 校验哈希
  file_size INTEGER NOT NULL DEFAULT 0,       -- 字节数（审计用）
  status TEXT NOT NULL DEFAULT 'pending',     -- pending/done/failed
  rollback_status TEXT,                       -- pending/done/failed（null=未尝试回滚）
  started_at TEXT,
  completed_at TEXT,
  UNIQUE(plan_id, action_index)
);

-- 查找未完成执行（崩溃恢复入口）：plan 处于 EXECUTING 且有 pending/done 记录
CREATE INDEX IF NOT EXISTS idx_journal_pending ON execution_journal(status) WHERE status IN ('pending', 'done');
CREATE INDEX IF NOT EXISTS idx_journal_plan ON execution_journal(plan_id);
