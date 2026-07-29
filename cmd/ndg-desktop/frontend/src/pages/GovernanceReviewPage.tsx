// Governance review page: stub — backend services exist, desktop wiring pending.
// Shows honest status without fake buttons.

import { useProject } from "../state/ProjectContext";

export default function GovernanceReviewPage() {
  const { capabilities, isReadWrite } = useProject();

  return (
    <div className="page page--governance-review">
      <div className="page-header">
        <h2>治理复核</h2>
        <p className="muted">治理决策与计划草案</p>
      </div>

      <div className="stub-card">
        <h3>能力建设中</h3>
        <p className="muted">
          后端已具备以下服务能力，桌面端正在接线：
        </p>
        <ul className="stub-list">
          <li>目录角色、权威性、业务锚点识别</li>
          <li>重复文件规划与保留评分（PlanService）</li>
          <li>计划 / 审批 / 执行三步分离</li>
          <li>治理诊断与合并诊断</li>
          <li>复核决定持久化（ReviewService）</li>
        </ul>
        <div className="stub-status">
          <span className={`state-badge ${isReadWrite ? "state-badge--queued" : "state-badge--cancelled"}`}>
            {isReadWrite ? "读写模式 — 接线后可编辑草案" : "只读模式 — 接线后可查看"}
          </span>
          {!capabilities.can_approve_plans && (
            <span className="muted">审批能力待 Binding 接入</span>
          )}
        </div>
      </div>
    </div>
  );
}
