-- 006: 桌面任务运行与结构化事件（V2 应用服务与任务系统）
--
-- job_runs 记录每次后台任务（扫描、格式分析、关系分析、计划生成等）的
-- 状态、阶段、进度与时间，供桌面端任务历史、重启恢复和进度展示使用。
-- job_events 只保存脱敏、低频、结构化里程碑，不保存每个文件事件。
--
-- 现有 internal/runner 继续作为 semaphore 并发执行器，不改造成持久队列。
-- JobManager 位于 runner 上层，负责持久化任务状态与重启恢复。
--
-- 隐私约束：progress_json 与 job_events 的 payload 不得包含完整路径、
-- 文件名、错误中的原始路径、数据库路径或隔离区路径。

CREATE TABLE IF NOT EXISTS job_runs (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  job_type TEXT NOT NULL,              -- scan | analyze | relations | plan | execute | restore | purge | learn
  state TEXT NOT NULL DEFAULT 'QUEUED', -- QUEUED | RUNNING | CANCEL_REQUESTED | CANCELLED | COMPLETED | FAILED
  stage TEXT NOT NULL DEFAULT 'DISCOVERING',
  -- DISCOVERING | METADATA_INDEXING | QUICK_HASHING | FULL_HASHING |
  -- CONTEXT_CLASSIFYING | FORMAT_ANALYZING | GROUPING | PLANNING | FINALIZING
  created_at TEXT NOT NULL,
  started_at TEXT,
  completed_at TEXT,
  cancel_requested INTEGER NOT NULL DEFAULT 0,
  progress_json TEXT NOT NULL DEFAULT '{}',
  warning_count INTEGER NOT NULL DEFAULT 0,
  error_code TEXT
);

CREATE INDEX IF NOT EXISTS idx_job_runs_project_state
  ON job_runs(project_id, state);

CREATE INDEX IF NOT EXISTS idx_job_runs_state
  ON job_runs(state)
  WHERE state IN ('RUNNING', 'CANCEL_REQUESTED');

-- 重启恢复查询：找出所有未终态任务
CREATE INDEX IF NOT EXISTS idx_job_runs_recovery
  ON job_runs(state)
  WHERE state IN ('QUEUED', 'RUNNING', 'CANCEL_REQUESTED');

CREATE TABLE IF NOT EXISTS job_events (
  id INTEGER PRIMARY KEY,
  job_id TEXT NOT NULL REFERENCES job_runs(id),
  sequence INTEGER NOT NULL,
  event_type TEXT NOT NULL,            -- job:created | job:stage | job:progress | job:warning | job:completed | job:failed | job:cancelled
  stage TEXT NOT NULL,
  state TEXT NOT NULL,
  payload_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  UNIQUE(job_id, sequence)
);

CREATE INDEX IF NOT EXISTS idx_job_events_job_sequence
  ON job_events(job_id, sequence);
