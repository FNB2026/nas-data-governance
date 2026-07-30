-- 008: 治理复核决策表（V1 正确性与查询基座）
--
-- 将 ReviewDecision 与 PlanState 彻底分离。
-- group_decisions 记录用户对重复组的复核决策，独立于操作计划状态机。
--
-- 复核状态（decision_type）：
--   KEEP_ALL              保留全部并关闭本组复核
--   DRAFT_ACTION          生成治理草案（交由 plan 流程）
--   DEFERRED              暂缓处理
--   REJECTED_SUGGESTION   驳回系统建议
--   CROSS_ARCHIVE         标记为交叉归档
--   BACKUP_RELATION       标记为备份关系
--   PRIMARY_RETENTION     选择主要保留项
--
-- 对应 ADR-0006 第 6 项迁移编号与校正指南 V1 第 6 项
--「ReviewDecision 与 PlanState 分离」。

CREATE TABLE IF NOT EXISTS group_decisions (
  id TEXT PRIMARY KEY,
  group_id TEXT NOT NULL,              -- 稳定 group_id = SHA256(domain + storage + content_sha256)
  decision_type TEXT NOT NULL,         -- KEEP_ALL | DRAFT_ACTION | DEFERRED | REJECTED_SUGGESTION | CROSS_ARCHIVE | BACKUP_RELATION | PRIMARY_RETENTION
  retained_file_id INTEGER,            -- PRIMARY_RETENTION 时指向保留的 file_instances.id
  reason TEXT,
  rule_id TEXT,                        -- 触发该决策的学习规则（如有）
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

-- 按组查询最新决策
CREATE INDEX IF NOT EXISTS idx_group_decisions_group
  ON group_decisions(group_id, updated_at);

-- 按决策类型筛选
CREATE INDEX IF NOT EXISTS idx_group_decisions_type
  ON group_decisions(decision_type);
