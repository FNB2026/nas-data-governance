-- 002: 规则学习基础设施
-- 新增规则冲突记录、规则命中统计、学习批次记录表。
-- rules 表的扩展字段通过 Go 代码幂等添加（SQLite 的 ALTER TABLE ADD COLUMN
-- 不支持 IF NOT EXISTS，重复执行会报 duplicate column name）。

-- 规则冲突记录：当学习规则与 builtin 规则匹配同一目录但角色不同时记录
-- path_hash 存路径哈希，不存原路径（K-009 隐私保护）
CREATE TABLE IF NOT EXISTS rule_conflicts (
  id INTEGER PRIMARY KEY,
  rule_id_a TEXT NOT NULL REFERENCES rules(id),
  rule_id_b TEXT NOT NULL REFERENCES rules(id),
  path_hash TEXT NOT NULL,
  resolved INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_rule_conflicts_unresolved ON rule_conflicts(resolved) WHERE resolved = 0;

-- 规则命中统计：跟踪每条规则的命中次数，供评估规则有效性
CREATE TABLE IF NOT EXISTS rule_hits (
  id INTEGER PRIMARY KEY,
  rule_id TEXT NOT NULL UNIQUE REFERENCES rules(id),
  hit_count INTEGER NOT NULL DEFAULT 0,
  last_hit_at TEXT
);

-- 学习批次记录：每次学习生成一个批次，支持整批回滚
CREATE TABLE IF NOT EXISTS learning_batches (
  id TEXT PRIMARY KEY,
  source TEXT NOT NULL,
  started_at TEXT NOT NULL,
  completed_at TEXT,
  rule_count INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'running'
);
