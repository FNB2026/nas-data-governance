// Audit & recovery page: stub — RecoveryService and JobManager exist, desktop wiring pending.

import { useProject } from "../state/ProjectContext";

export default function AuditRecoveryPage() {
  const { capabilities } = useProject();

  return (
    <div className="page page--audit-recovery">
      <div className="page-header">
        <h2>审计与恢复</h2>
        <p className="muted">操作审计、Journal与恢复</p>
      </div>

      <div className="stub-card">
        <h3>能力建设中</h3>
        <p className="muted">
          后端已具备以下服务能力，桌面端正在接线：
        </p>
        <ul className="stub-list">
          <li>RecoveryService 崩溃恢复（未完成任务检测与恢复）</li>
          <li>JobManager 持久化任务与取消处理</li>
          <li>Journal 事务日志（执行步骤、校验结果、回滚记录）</li>
          <li>执行审计记录</li>
        </ul>
        <div className="stub-status">
          {capabilities.recovery_lock_active ? (
            <span className="state-badge state-badge--failed">
              恢复锁激活 — 请处理未完成执行
            </span>
          ) : (
            <span className="state-badge state-badge--cancelled">
              恢复检测待 Binding 接入
            </span>
          )}
        </div>
      </div>
    </div>
  );
}
