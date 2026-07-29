// Execution center page: stub — Quarantine/Purge services exist, desktop wiring pending.

import { useProject } from "../state/ProjectContext";

export default function ExecutionCenterPage() {
  const { capabilities } = useProject();

  return (
    <div className="page page--execution-center">
      <div className="page-header">
        <h2>执行中心</h2>
        <p className="muted">隔离、清理与执行安全</p>
      </div>

      <div className="stub-card">
        <h3>能力建设中</h3>
        <p className="muted">
          后端已具备以下服务能力，桌面端正在接线：
        </p>
        <ul className="stub-list">
          <li>Quarantine 隔离服务（含隔离生命周期管理）</li>
          <li>Purge 清理服务（限制在隔离区内）</li>
          <li>Restore 恢复服务（隔离文件还原）</li>
          <li>stale 检查、SHA-256 校验、执行审计</li>
          <li>Journal 事务日志与崩溃恢复</li>
          <li>26 个隔离集成测试覆盖</li>
        </ul>
        <div className="stub-status">
          <span className="state-badge state-badge--cancelled">
            执行能力待 Binding 接入
          </span>
          {!capabilities.can_execute_quarantine && (
            <span className="muted">隔离执行未接线</span>
          )}
          {!capabilities.can_execute_purge && (
            <span className="muted">清理执行未接线</span>
          )}
        </div>
      </div>
    </div>
  );
}
