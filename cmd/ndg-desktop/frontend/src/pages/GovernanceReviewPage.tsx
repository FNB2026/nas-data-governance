// Governance review page: plan listing, detail inspection, decision recording.
// V6 wiring — connects PlanService and ReviewService via Wails bindings.

import { useCallback, useEffect, useState } from "react";
import { useProject } from "../state/ProjectContext";
import { hasWailsRuntime, formatBytes, shortHash } from "../lib/utils";
import CopyButton from "../components/CopyButton";
import ErrorState from "../components/ErrorState";
import EmptyState from "../components/EmptyState";
import DisabledNotice from "../components/DisabledNotice";
import { api } from "../api/client";
import { wails } from "../wailsjs/go/models";
import { type AppRoute } from "../app/routes";

// ---- Constants ----

const PLAN_STATE_LABELS: Record<string, string> = {
  DRAFT: "草案",
  APPROVED: "已批准",
  STALE_CHECKED: "已校验",
  EXECUTING: "执行中",
  VERIFIED: "已验证",
  ROLLED_BACK: "已回滚",
};

const RISK_LABELS: Record<string, string> = {
  low: "低",
  medium: "中",
  high: "高",
  critical: "严重",
};

const DECISION_LABELS: Record<string, string> = {
  KEEP_ALL: "全部保留",
  DRAFT_ACTION: "起草动作",
  DEFERRED: "暂缓",
  REJECTED_SUGGESTION: "拒绝建议",
  CROSS_ARCHIVE: "交叉归档",
  BACKUP_RELATION: "备份关系",
  PRIMARY_RETENTION: "指定保留",
};

const DECISION_OPTIONS = [
  { value: "KEEP_ALL", label: "全部保留" },
  { value: "DRAFT_ACTION", label: "起草动作" },
  { value: "DEFERRED", label: "暂缓" },
  { value: "REJECTED_SUGGESTION", label: "拒绝建议" },
  { value: "CROSS_ARCHIVE", label: "交叉归档" },
  { value: "BACKUP_RELATION", label: "备份关系" },
];

const ACTION_LABELS: Record<string, string> = {
  KEEP: "保留",
  MOVE: "移动",
  COPY: "复制",
  RENAME: "重命名",
  DELETE: "删除",
  QUARANTINE: "隔离",
  SKIP: "跳过",
  REVIEW: "复核",
};

// ---- Helpers ----

function planStateLabel(state: string): string {
  return PLAN_STATE_LABELS[state] || state;
}

function planStateBadgeClass(state: string): string {
  return `plan-state-badge plan-state-badge--${state.toLowerCase().replace(/_/g, "-")}`;
}

function riskLabel(risk: string): string {
  return RISK_LABELS[risk] || risk;
}

function riskBadgeClass(risk: string): string {
  return `risk-badge risk-badge--${risk || "unassessed"}`;
}

function decisionLabel(dt: string): string {
  return DECISION_LABELS[dt] || dt;
}

function actionLabel(action: string): string {
  return ACTION_LABELS[action] || action;
}

// ---- Component ----

interface GovernanceReviewPageProps {
  onNavigate?: (route: AppRoute) => void;
}

export default function GovernanceReviewPage({ onNavigate }: GovernanceReviewPageProps) {
  const { capabilities, isReadWrite, dataRevision, pushToast, displayPath } = useProject();

  // Write affordances (generate drafts / record decisions / approve) are
  // gated by capability, not just the read/write mode flag: the recovery
  // lock also blocks new writes while keeping the read surface usable.
  const canWriteReviews = capabilities.can_edit_reviews && capabilities.can_approve_plans;
  const reviewDisabledReason =
    capabilities.disabled_reasons?.["governance-review"] ??
    (capabilities.recovery_lock_active
      ? "恢复锁激活中，请先处理未完成执行"
      : !isReadWrite
        ? "只读模式，无法执行写操作"
        : null);

  // Plan data
  const [plans, setPlans] = useState<wails.PlanDTO[]>([]);
  const [plansLoading, setPlansLoading] = useState(false);
  const [plansError, setPlansError] = useState<string | null>(null);

  // Decisions map (group_id → decision)
  const [decisionsMap, setDecisionsMap] = useState<Map<string, wails.GroupDecisionDTO>>(new Map());

  // Selected plan
  const [selectedPlanId, setSelectedPlanId] = useState<string | null>(null);

  // Decision form
  const [decisionType, setDecisionType] = useState("KEEP_ALL");
  const [decisionReason, setDecisionReason] = useState("");
  const [savingDecision, setSavingDecision] = useState(false);

  // Draft plans (in-memory preview)
  const [draftPlans, setDraftPlans] = useState<wails.PlanDTO[] | null>(null);
  const [buildingDrafts, setBuildingDrafts] = useState(false);
  const [savingDrafts, setSavingDrafts] = useState(false);

  // Approval
  const [approving, setApproving] = useState(false);

  // Filters
  const [stateFilter, setStateFilter] = useState<string>("");

  // ---- Data loading ----

  const loadPlans = useCallback(async () => {
    if (!hasWailsRuntime()) return;
    setPlansLoading(true);
    setPlansError(null);
    try {
      const list = await api.governance.listAll();
      setPlans(list || []);
    } catch (e: unknown) {
      setPlansError((e as Error).message);
      setPlans([]);
    } finally {
      setPlansLoading(false);
    }
  }, []);

  const loadDecisions = useCallback(async () => {
    if (!hasWailsRuntime()) return;
    try {
      const list = await api.governance.listDecisions("");
      const map = new Map<string, wails.GroupDecisionDTO>();
      for (const d of list || []) {
        map.set(d.group_id, d);
      }
      setDecisionsMap(map);
    } catch {
      // Non-fatal: decisions are optional
    }
  }, []);

  const loadAll = useCallback(async () => {
    await Promise.all([loadPlans(), loadDecisions()]);
  }, [loadPlans, loadDecisions]);

  // Load on mount and when dataRevision changes (e.g., after scan completes)
  useEffect(() => {
    if (capabilities.project_open) {
      void loadAll();
    }
  }, [capabilities.project_open, dataRevision, loadAll]);

  // ---- Actions ----

  const handleBuildDrafts = async () => {
    setBuildingDrafts(true);
    try {
      const drafts = await api.governance.buildDrafts("");
      setDraftPlans(drafts || []);
      pushToast("success", "草案生成完成", `共 ${drafts?.length || 0} 条规划`);
    } catch (e: unknown) {
      pushToast("error", "生成草案失败", (e as Error).message);
    } finally {
      setBuildingDrafts(false);
    }
  };

  const handleSaveDrafts = async () => {
    setSavingDrafts(true);
    try {
      const saved = await api.governance.saveDrafts("");
      setDraftPlans(null);
      setPlans(saved || []);
      pushToast("success", "草案已落库", `共保存 ${saved?.length || 0} 条计划`);
    } catch (e: unknown) {
      pushToast("error", "保存草案失败", (e as Error).message);
    } finally {
      setSavingDrafts(false);
    }
  };

  const handleSaveDecision = async () => {
    if (!selectedPlan) return;
    setSavingDecision(true);
    try {
      const result = await api.governance.saveDecision({
        group_id: selectedPlan.group_id,
        decision_type: decisionType,
        reason: decisionReason.trim(),
      } as wails.SaveDecisionRequest);
      setDecisionsMap((prev) => {
        const next = new Map(prev);
        next.set(selectedPlan.group_id, result);
        return next;
      });
      pushToast("success", "决策已保存", `${selectedPlan.group_id}: ${decisionLabel(decisionType)}`);
    } catch (e: unknown) {
      pushToast("error", "保存决策失败", (e as Error).message);
    } finally {
      setSavingDecision(false);
    }
  };

  const handleApprove = async (planId: string) => {
    setApproving(true);
    try {
      await api.governance.approve({ plan_ids: [planId] } as wails.ApprovePlansRequest);
      // Update local state
      setPlans((prev) =>
        prev.map((p) =>
          p.id === planId
            ? wails.PlanDTO.createFrom({ ...p, state: "APPROVED" })
            : p,
        ),
      );
      pushToast("success", "计划已批准", planId);
    } catch (e: unknown) {
      pushToast("error", "批准失败", (e as Error).message);
    } finally {
      setApproving(false);
    }
  };

  // Load existing decision when selecting a plan
  const handleSelectPlan = async (planId: string) => {
    setSelectedPlanId(planId);
    // Pre-fill form from existing decision if available
    const plan = displayPlans.find((item) => item.id === planId);
    const existing = plan ? decisionsMap.get(plan.group_id) : undefined;
    if (existing) {
      setDecisionType(existing.decision_type);
      setDecisionReason(existing.reason || "");
    } else {
      setDecisionType("KEEP_ALL");
      setDecisionReason("");
    }
  };

  // ---- Filtered plans ----

  const displayPlans = draftPlans ?? plans;
  const selectedPlan = displayPlans.find((p) => p.id === selectedPlanId) || null;
  const filteredPlans = stateFilter
    ? displayPlans.filter((p) => p.state === stateFilter)
    : displayPlans;

  // State distribution
  const stateCounts = displayPlans.reduce((acc, p) => {
    acc[p.state] = (acc[p.state] || 0) + 1;
    return acc;
  }, {} as Record<string, number>);
  const selectedDecision = selectedPlan ? decisionsMap.get(selectedPlan.group_id) : undefined;
  const hasRecordedDecision = selectedDecision !== undefined;
  const canApproveSelected =
    canWriteReviews &&
    selectedPlan?.state === "DRAFT" &&
    selectedPlan.risk !== "critical" &&
    !draftPlans &&
    hasRecordedDecision;

  // ---- Render ----

  if (!capabilities.project_open) {
    return (
      <div className="page page--governance-review">
        <div className="page-header">
          <h2>治理复核</h2>
          <p className="muted">请先打开项目</p>
        </div>
      </div>
    );
  }

  return (
    <div className="page page--governance-review">
      <div className="page-header">
        <h2>治理复核</h2>
        <p className="muted">计划草案、复核决定与审批</p>
      </div>

      {/* Toolbar */}
      <div className="gov-toolbar">
        <div className="gov-toolbar-left">
          <button
            className="btn-sm secondary"
            onClick={() => void loadAll()}
            disabled={plansLoading}
          >
            {plansLoading ? "加载中…" : "刷新"}
          </button>
          {canWriteReviews && (
            <button
              className="btn-sm"
              onClick={() => void handleBuildDrafts()}
              disabled={buildingDrafts}
            >
              {buildingDrafts ? "生成中…" : "生成系统建议"}
            </button>
          )}
          {draftPlans && (
            <>
              <button
                className="btn-sm"
                onClick={() => void handleSaveDrafts()}
                disabled={savingDrafts || draftPlans.length === 0}
              >
                {savingDrafts ? "保存中…" : "保存系统建议"}
              </button>
              <button
                className="btn-sm secondary"
                onClick={() => setDraftPlans(null)}
              >
                返回已保存建议
              </button>
            </>
          )}
        </div>
        <div className="gov-toolbar-right">
          <select
            className="gov-filter-select"
            value={stateFilter}
            onChange={(e) => setStateFilter(e.target.value)}
          >
            <option value="">全部状态 ({displayPlans.length})</option>
            {Object.entries(PLAN_STATE_LABELS).map(([state, label]) => (
              <option key={state} value={state}>
                {label} ({stateCounts[state] || 0})
              </option>
            ))}
          </select>
          <span className={`mode-badge ${isReadWrite ? "mode-badge--rw" : "mode-badge--ro"}`}>
            {isReadWrite ? "读写" : "只读"}
          </span>
        </div>
      </div>

      {plansError && <ErrorState message={plansError} />}

      {!canWriteReviews && (
        <DisabledNotice
          reason={reviewDisabledReason ?? "只读模式，无法执行写操作"}
          hint="当前模式无法生成草案、记录决策或批准计划；计划列表仍可查看。"
        />
      )}

      <section className="gov-workflow" aria-label="治理复核流程">
        <div className="gov-workflow-step gov-workflow-step--system">
          <span className="gov-workflow-number">1</span>
          <div>
            <strong>系统建议</strong>
            <span>{selectedPlan ? "仅供复核，不会自动执行" : "生成后选择一份建议查看依据"}</span>
          </div>
        </div>
        <div className={`gov-workflow-step ${hasRecordedDecision ? "gov-workflow-step--complete" : ""}`}>
          <span className="gov-workflow-number">2</span>
          <div>
            <strong>用户决定</strong>
            <span>{hasRecordedDecision ? decisionLabel(selectedDecision.decision_type) : "记录后才能批准计划"}</span>
          </div>
        </div>
        <div className={`gov-workflow-step ${selectedPlan?.state === "APPROVED" ? "gov-workflow-step--complete" : ""}`}>
          <span className="gov-workflow-number">3</span>
          <div>
            <strong>批准状态</strong>
            <span>{selectedPlan ? planStateLabel(selectedPlan.state) : "选择计划后显示"}</span>
          </div>
        </div>
        <div className={`gov-workflow-step ${selectedPlan?.state === "APPROVED" ? "gov-workflow-step--ready" : ""}`}>
          <span className="gov-workflow-number">4</span>
          <div>
            <strong>执行中心</strong>
            <span>{selectedPlan?.state === "APPROVED" ? "可进入执行前校验" : "批准后才可进入"}</span>
          </div>
        </div>
      </section>

      {/* Two-panel layout */}
      <div className="gov-layout">
        {/* Plan list */}
        <div className="gov-plan-list">
          {filteredPlans.length === 0 ? (
            draftPlans ? (
              <EmptyState title="暂无数据" hint="草案生成完成，但无重复组可规划" />
            ) : plans.length === 0 ? (
              <EmptyState title="暂无数据" hint="暂无已保存建议。点击「生成系统建议」从已扫描文件生成建议草案。" />
            ) : (
              <EmptyState title="无匹配结果" hint="当前筛选条件下无计划" />
            )
          ) : (
            <>
              {draftPlans && (
                <div className="gov-draft-notice">
                  系统建议预览 — 共 {draftPlans.length} 条；尚未保存、批准或执行
                </div>
              )}
              {filteredPlans.map((plan) => {
                const decision = decisionsMap.get(plan.group_id);
                return (
                  <div
                    key={plan.id}
                    className={`gov-plan-item ${selectedPlanId === plan.id ? "gov-plan-item--selected" : ""}`}
                    onClick={() => void handleSelectPlan(plan.id)}
                  >
                    <div className="gov-plan-item-header">
                      <span className={planStateBadgeClass(plan.state)}>
                        {planStateLabel(plan.state)}
                      </span>
                      <span className={riskBadgeClass(plan.risk)}>
                        {riskLabel(plan.risk)}
                      </span>
                      {decision && (
                        <span className="gov-decision-tag">
                          {decisionLabel(decision.decision_type)}
                        </span>
                      )}
                    </div>
                    <div className="gov-plan-item-id">{plan.id}</div>
                    <div className="gov-plan-item-meta">
                      <span>{formatBytes(plan.size)}</span>
                      <span>{plan.actions.length} 动作</span>
                      <span className="muted">{shortHash(plan.content_sha256)}</span>
                    </div>
                  </div>
                );
              })}
            </>
          )}
        </div>

        {/* Plan detail */}
        <div className="gov-plan-detail">
          {!selectedPlan ? (
            <div className="gov-empty">
              选择左侧计划查看详情
            </div>
          ) : (
            <>
              {/* Detail header */}
              <div className="gov-detail-header">
                <div className="gov-detail-title">
                  <h3>{selectedPlan.id}</h3>
                  <div className="gov-detail-badges">
                    <span className={planStateBadgeClass(selectedPlan.state)}>
                      {planStateLabel(selectedPlan.state)}
                    </span>
                    <span className={riskBadgeClass(selectedPlan.risk)}>
                      {riskLabel(selectedPlan.risk)}
                    </span>
                  </div>
                </div>
                <div className="gov-detail-meta">
                  <div><span className="muted">容量</span> {formatBytes(selectedPlan.size)}</div>
                  <div><span className="muted">哈希</span> {shortHash(selectedPlan.content_sha256)}<CopyButton text={selectedPlan.content_sha256} label="复制治理计划 SHA-256" /></div>
                  {selectedPlan.retain_path && (
                    <div className="gov-retain-path">
                      <span className="muted">保留路径</span>
                      <code>{displayPath(selectedPlan.retain_path)}</code>
                    </div>
                  )}
                </div>
              </div>

              {/* System evidence */}
              {selectedPlan.evidence.length > 0 && (
                <div className="gov-section">
                  <h4>系统建议依据</h4>
                  <ul className="gov-evidence-list">
                    {selectedPlan.evidence.map((ev, i) => (
                      <li key={i}>{ev}</li>
                    ))}
                  </ul>
                </div>
              )}

              {/* Suggested actions */}
              <div className="gov-section">
                <h4>系统建议动作 ({selectedPlan.actions.length})</h4>
                <p className="gov-section-hint">建议不会修改源数据；请依据下方证据记录你的决定。</p>
                <div className="gov-actions-list">
                  {selectedPlan.actions.map((action, i) => (
                    <div key={i} className="gov-action-item">
                      <div className="gov-action-row">
                        <span className={`gov-action-badge gov-action-badge--${action.action.toLowerCase()}`}>
                          {actionLabel(action.action)}
                        </span>
                        <span className="gov-action-path" title={displayPath(action.path)}>
                          {displayPath(action.path)}
                        </span>
                        {action.context_role && (
                          <span className="gov-role-tag">{action.context_role}</span>
                        )}
                      </div>
                      <div className="gov-action-reason">{action.reason}</div>
                      {action.target_path && (
                        <div className="gov-action-target">
                          <span className="muted">目标：</span>
                          <code>{displayPath(action.target_path)}</code>
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              </div>

              {/* Existing user decision */}
              {selectedDecision && (
                <div className="gov-section">
                  <h4>已记录的用户决定</h4>
                  <div className="gov-existing-decision">
                    <span className="gov-decision-tag gov-decision-tag--large">
                      {decisionLabel(selectedDecision.decision_type)}
                    </span>
                    {selectedDecision.reason && (
                      <p className="muted">{selectedDecision.reason}</p>
                    )}
                  </div>
                </div>
              )}

              {/* Decision form (write capability only) */}
              {canWriteReviews && (
                <div className="gov-section gov-decision-form">
                  <h4>用户决定</h4>
                  <p className="gov-section-hint">保存决定不会批准计划，也不会触发隔离、移动或删除。</p>
                  <div className="gov-form-row">
                    <label className="gov-form-label">决策类型</label>
                    <select
                      className="gov-form-select"
                      value={decisionType}
                      onChange={(e) => setDecisionType(e.target.value)}
                    >
                      {DECISION_OPTIONS.map((opt) => (
                        <option key={opt.value} value={opt.value}>
                          {opt.label}
                        </option>
                      ))}
                    </select>
                  </div>
                  <div className="gov-form-row">
                    <label className="gov-form-label">理由（可选）</label>
                    <textarea
                      className="gov-form-textarea"
                      value={decisionReason}
                      onChange={(e) => setDecisionReason(e.target.value)}
                      rows={2}
                      placeholder="说明决策原因…"
                    />
                  </div>
                  <div className="gov-form-actions">
                    <button
                      className="btn-sm secondary"
                      onClick={() => void handleSaveDecision()}
                      disabled={savingDecision}
                    >
                      {savingDecision ? "保存中…" : "保存用户决定"}
                    </button>
                  </div>
                </div>
              )}

              <div className="gov-section gov-approval-section">
                <h4>批准状态</h4>
                {selectedPlan.state === "DRAFT" && selectedPlan.risk !== "critical" && !draftPlans && (
                  <>
                    <p className="gov-section-hint">
                      {hasRecordedDecision
                        ? "批准是独立的人工作用；批准后仍需在执行中心完成执行前校验。"
                        : "请先记录用户决定。系统建议本身不能作为批准。"}
                    </p>
                    {canWriteReviews && (
                      <button
                        className="btn-sm"
                        onClick={() => void handleApprove(selectedPlan.id)}
                        disabled={approving || !canApproveSelected}
                      >
                        {approving ? "批准中…" : "批准计划"}
                      </button>
                    )}
                  </>
                )}
                {selectedPlan.risk === "critical" && (
                  <span className="gov-hold-notice">
                    严重风险计划处于 HOLD 状态，需独立释放
                  </span>
                )}
                {selectedPlan.state === "APPROVED" && (
                  <div className="gov-execution-ready">
                    <span>已批准。进入执行中心后仍需完成执行前校验。</span>
                    {onNavigate && (
                      <button className="btn-sm secondary" onClick={() => onNavigate("execution-center")}>
                        前往执行中心
                      </button>
                    )}
                  </div>
                )}
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
