// Execution center page: quarantine lifecycle, purge management, and crash recovery.
// V7 wiring — connects QuarantineService, PurgeService, and RecoveryService.

import { useCallback, useEffect, useState } from "react";
import { useProject } from "../state/ProjectContext";
import { hasWailsRuntime, errorText, formatBytes, shortHash, formatDateTime } from "../lib/utils";
import CopyButton from "../components/CopyButton";
import {
  ApprovePurgePlan,
  ApproveRestorePlan,
  CheckRecoveryLock,
  CreatePurgePlans,
  CreateRestorePlan,
  ExecutePurge,
  ExecuteRestore,
  ListPurgePlans,
  ListQuarantineItems,
  ListRestorePlans,
  RecoverPurges,
  RecoverRestores,
  RecoverSourcePlans,
} from "../wailsjs/go/wails/API";
import { wails } from "../wailsjs/go/models";

// ---- Constants ----

const QUARANTINE_STATUS_LABELS: Record<string, string> = {
  QUARANTINED: "已隔离",
  HOLD: "保留中",
  PURGE_ELIGIBLE: "可清理",
  RESTORED: "已恢复",
  PURGED: "已清理",
  ROLLED_BACK: "已回滚",
};

const RESTORE_STATE_LABELS: Record<string, string> = {
  DRAFT: "草案",
  APPROVED: "已批准",
  RESTORED: "已恢复",
  ROLLED_BACK: "已回滚",
};

const PURGE_STATE_LABELS: Record<string, string> = {
  DRAFT: "草案",
  APPROVED: "已批准",
  STAGED: "已暂存",
  PURGED: "已清理",
  ROLLED_BACK: "已回滚",
  FAILED: "失败",
};

type ExecTab = "quarantine" | "purge" | "recovery";

// ---- Helpers ----

function quarantineStatusBadgeClass(status: string): string {
  return `q-status-badge q-status-badge--${status.toLowerCase().replace(/_/g, "-")}`;
}

function planStateBadgeClass(state: string, prefix: string): string {
  return `${prefix}-state-badge ${prefix}-state-badge--${state.toLowerCase().replace(/_/g, "-")}`;
}

// ---- Component ----

export default function ExecutionCenterPage() {
  const { capabilities, isReadWrite, dataRevision, pushToast } = useProject();

  const [activeTab, setActiveTab] = useState<ExecTab>("quarantine");

  // Quarantine data
  const [quarantineItems, setQuarantineItems] = useState<wails.QuarantineItemDTO[]>([]);
  const [quarantineLoading, setQuarantineLoading] = useState(false);
  const [quarantineError, setQuarantineError] = useState<string | null>(null);
  const [statusFilter, setStatusFilter] = useState<string>("");

  // Restore workflow
  const [restorePlans, setRestorePlans] = useState<wails.RestorePlanDTO[]>([]);
  const [restoreRoot, setRestoreRoot] = useState("");
  const [restoreSourceRoots, setRestoreSourceRoots] = useState("");
  const [restoring, setRestoring] = useState(false);

  // Purge data
  const [purgePlans, setPurgePlans] = useState<wails.PurgePlanDTO[]>([]);
  const [purgeError, setPurgeError] = useState<string | null>(null);
  const [purgeRoot, setPurgeRoot] = useState("");
  const [purging, setPurging] = useState(false);
  const [purgeConfirmations, setPurgeConfirmations] = useState<Record<string, string>>({});

  // Recovery
  const [recoveryStatus, setRecoveryStatus] = useState<wails.RecoveryStatusDTO | null>(null);
  const [recoveryLoading, setRecoveryLoading] = useState(false);
  const [recoveryResults, setRecoveryResults] = useState<string | null>(null);

  // ---- Data loading ----

  const loadQuarantine = useCallback(async () => {
    if (!hasWailsRuntime()) return;
    setQuarantineLoading(true);
    setQuarantineError(null);
    try {
      const list = await ListQuarantineItems(statusFilter);
      setQuarantineItems(list || []);
    } catch (e: unknown) {
      setQuarantineError(errorText(e));
      setQuarantineItems([]);
    } finally {
      setQuarantineLoading(false);
    }
  }, [statusFilter]);

  const loadRecoveryStatus = useCallback(async () => {
    if (!hasWailsRuntime()) return;
    setRecoveryLoading(true);
    try {
      const status = await CheckRecoveryLock();
      setRecoveryStatus(status);
    } catch {
      // Non-fatal
    } finally {
      setRecoveryLoading(false);
    }
  }, []);

  const loadLifecyclePlans = useCallback(async () => {
    if (!hasWailsRuntime()) return;
    try {
      const [restores, purges] = await Promise.all([ListRestorePlans(), ListPurgePlans()]);
      setRestorePlans(restores || []);
      setPurgePlans(purges || []);
    } catch (e: unknown) {
      setPurgeError(errorText(e));
    }
  }, []);

  useEffect(() => {
    if (capabilities.project_open) {
      void loadQuarantine();
      void loadRecoveryStatus();
      void loadLifecyclePlans();
    }
  }, [capabilities.project_open, dataRevision, loadLifecyclePlans, loadQuarantine, loadRecoveryStatus]);

  // ---- Quarantine actions ----

  const handleCreateRestorePlan = async (itemId: string) => {
    setRestoring(true);
    try {
      const plan = await CreateRestorePlan(itemId);
      setRestorePlans((prev) => {
        const next = prev.filter((p) => p.item_id !== itemId);
        return [...next, plan];
      });
      pushToast("success", "恢复草案已创建", `计划 ${plan.id}`);
    } catch (e: unknown) {
      pushToast("error", "创建恢复计划失败", errorText(e));
    } finally {
      setRestoring(false);
    }
  };

  const handleApproveRestore = async (planId: string, digest: string) => {
    try {
      await ApproveRestorePlan(planId, digest);
      setRestorePlans((prev) =>
        prev.map((p) => (p.id === planId ? wails.RestorePlanDTO.createFrom({ ...p, state: "APPROVED" }) : p)),
      );
      pushToast("success", "恢复计划已批准", planId);
    } catch (e: unknown) {
      pushToast("error", "批准失败", errorText(e));
    }
  };

  const handleExecuteRestore = async (planId: string, digest: string, dryRun: boolean) => {
    setRestoring(true);
    try {
      const result = await ExecuteRestore({
        plan_id: planId,
        digest,
        quarantine_root: restoreRoot.trim(),
        source_roots: restoreSourceRoots.split("\n").map((s) => s.trim()).filter(Boolean),
        dry_run: dryRun,
      } as wails.ExecuteRestoreRequest);
      if (result.status === "ok") {
        pushToast("success", dryRun ? "校验通过" : "恢复执行成功", `${planId}: ${result.final_state}`);
      } else {
        pushToast("error", dryRun ? "校验失败" : "恢复执行失败", result.error || result.error_type || planId);
      }
      void loadQuarantine();
    } catch (e: unknown) {
      pushToast("error", "执行恢复失败", errorText(e));
    } finally {
      setRestoring(false);
    }
  };

  // ---- Purge actions ----

  const handleCreatePurgePlans = async () => {
    setPurging(true);
    setPurgeError(null);
    try {
      const plans = await CreatePurgePlans();
      setPurgePlans(plans || []);
      pushToast("success", "清理草案已生成", `共 ${plans?.length || 0} 条`);
    } catch (e: unknown) {
      setPurgeError(errorText(e));
      pushToast("error", "生成清理计划失败", errorText(e));
    } finally {
      setPurging(false);
    }
  };

  const handleApprovePurge = async (planId: string, digest: string) => {
    try {
      await ApprovePurgePlan(planId, digest);
      setPurgePlans((prev) =>
        prev.map((p) => (p.id === planId ? wails.PurgePlanDTO.createFrom({ ...p, state: "APPROVED" }) : p)),
      );
      pushToast("success", "清理计划已批准", planId);
    } catch (e: unknown) {
      pushToast("error", "批准失败", errorText(e));
    }
  };

  const handleExecutePurge = async (planId: string, digest: string, dryRun: boolean, confirmation = "") => {
    setPurging(true);
    try {
      const result = await ExecutePurge({
        plan_id: planId,
        digest,
        quarantine_root: purgeRoot.trim(),
        dry_run: dryRun,
        confirmation,
      } as wails.ExecutePurgeRequest);
      if (result.status === "ok") {
        pushToast("success", dryRun ? "校验通过" : "清理执行成功", `${planId}: ${result.final_state}`);
      } else {
        pushToast("error", dryRun ? "校验失败" : "清理执行失败", result.error || result.error_type || planId);
      }
      void loadQuarantine();
    } catch (e: unknown) {
      pushToast("error", "执行清理失败", errorText(e));
    } finally {
      setPurging(false);
    }
  };

  // ---- Recovery actions ----

  const handleRecoverSource = async () => {
    setRecoveryLoading(true);
    try {
      const results = await RecoverSourcePlans();
      if (results.length === 0) {
        pushToast("success", "无需恢复", "没有检测到卡住的执行计划");
        setRecoveryResults("无需恢复 — 没有卡住的执行计划");
      } else {
        const summary = results.map((r) => `${r.plan_id}: ${r.action} (回滚 ${r.rolled_back} 步)`).join("\n");
        setRecoveryResults(summary);
        pushToast("success", "恢复完成", `处理了 ${results.length} 个计划`);
      }
      void loadRecoveryStatus();
      void loadQuarantine();
    } catch (e: unknown) {
      pushToast("error", "恢复失败", errorText(e));
    } finally {
      setRecoveryLoading(false);
    }
  };

  const handleRecoverRestores = async () => {
    setRecoveryLoading(true);
    try {
      const results = await RecoverRestores({
        quarantine_root: restoreRoot.trim(),
        source_roots: restoreSourceRoots.split("\n").map((s) => s.trim()).filter(Boolean),
      } as wails.RecoverRestoresRequest);
      if (results.length === 0) {
        setRecoveryResults("无需恢复 — 没有卡住的恢复操作");
      } else {
        setRecoveryResults(results.map((r) => `${r.plan_id || "未知"}: ${r.status}`).join("\n"));
      }
      pushToast("success", "恢复操作完成", `处理了 ${results.length} 条`);
    } catch (e: unknown) {
      pushToast("error", "恢复失败", errorText(e));
    } finally {
      setRecoveryLoading(false);
    }
  };

  const handleRecoverPurges = async () => {
    setRecoveryLoading(true);
    try {
      const results = await RecoverPurges(purgeRoot.trim());
      if (results.length === 0) {
        setRecoveryResults("无需恢复 — 没有卡住的清理操作");
      } else {
        setRecoveryResults(results.map((r) => `${r.plan_id || "未知"}: ${r.status}`).join("\n"));
      }
      pushToast("success", "清理恢复完成", `处理了 ${results.length} 条`);
    } catch (e: unknown) {
      pushToast("error", "恢复失败", errorText(e));
    } finally {
      setRecoveryLoading(false);
    }
  };

  // ---- Filtered items ----

  const filteredItems = quarantineItems;

  // ---- Render ----

  if (!capabilities.project_open) {
    return (
      <div className="page page--execution-center">
        <div className="page-header">
          <h2>执行中心</h2>
          <p className="muted">请先打开项目</p>
        </div>
      </div>
    );
  }

  return (
    <div className="page page--execution-center">
      <div className="page-header">
        <h2>执行中心</h2>
        <p className="muted">隔离生命周期、清理审批与崩溃恢复</p>
      </div>

      {/* Recovery lock banner */}
      {recoveryStatus?.lock_active && (
        <div className="exec-lock-banner" role="alert">
          <strong>恢复锁激活</strong> — 检测到 {recoveryStatus.executing_count} 个未完成执行计划，请先在「恢复」标签页处理
        </div>
      )}

      {/* Tab navigation */}
      <div className="exec-tabs">
        <button
          className={`exec-tab ${activeTab === "quarantine" ? "exec-tab--active" : ""}`}
          onClick={() => setActiveTab("quarantine")}
        >
          隔离与恢复 ({filteredItems.length})
        </button>
        <button
          className={`exec-tab ${activeTab === "purge" ? "exec-tab--active" : ""}`}
          onClick={() => setActiveTab("purge")}
        >
          清理 ({purgePlans.length})
        </button>
        <button
          className={`exec-tab ${activeTab === "recovery" ? "exec-tab--active" : ""}`}
          onClick={() => setActiveTab("recovery")}
        >
          恢复 {recoveryStatus?.lock_active ? "⚠" : ""}
        </button>
      </div>

      {/* ---- Quarantine tab ---- */}
      {activeTab === "quarantine" && (
        <div className="exec-tab-content">
          <div className="exec-toolbar">
            <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)}>
              <option value="">全部状态</option>
              {Object.entries(QUARANTINE_STATUS_LABELS).map(([s, label]) => (
                <option key={s} value={s}>{label}</option>
              ))}
            </select>
            <button className="btn-sm secondary" onClick={() => void loadQuarantine()} disabled={quarantineLoading}>
              {quarantineLoading ? "加载中…" : "刷新"}
            </button>
            {isReadWrite && (
              <div className="exec-root-inputs">
                <input
                  type="text"
                  placeholder="隔离根目录"
                  value={restoreRoot}
                  onChange={(e) => setRestoreRoot(e.target.value)}
                  className="exec-root-input"
                />
                <input
                  type="text"
                  placeholder="源根目录（逗号分隔）"
                  value={restoreSourceRoots}
                  onChange={(e) => setRestoreSourceRoots(e.target.value)}
                  className="exec-root-input"
                />
              </div>
            )}
          </div>

          {quarantineError && <p className="error" role="alert">{quarantineError}</p>}

          {filteredItems.length === 0 ? (
            <div className="empty-state">
              <p className="muted">{quarantineLoading ? "加载中…" : "暂无隔离项"}</p>
            </div>
          ) : (
            <div className="table-wrap">
              <table className="data-table">
                <thead>
                  <tr>
                    <th>ID</th>
                    <th>状态</th>
                    <th>大小</th>
                    <th>SHA-256</th>
                    <th>隔离时间</th>
                    <th>保留至</th>
                    {isReadWrite && <th>操作</th>}
                  </tr>
                </thead>
                <tbody>
                  {filteredItems.map((item) => {
                    const restorePlan = restorePlans.find((p) => p.item_id === item.id);
                    return (
                      <tr key={item.id}>
                        <td className="mono">
                          {item.id}
                          <CopyButton text={item.id} />
                        </td>
                        <td>
                          <span className={quarantineStatusBadgeClass(item.status)}>
                            {QUARANTINE_STATUS_LABELS[item.status] || item.status}
                          </span>
                        </td>
                        <td className="num">{formatBytes(item.file_size)}</td>
                        <td className="mono">
                          {shortHash(item.content_sha256)}
                          <CopyButton text={item.content_sha256} />
                        </td>
                        <td>{formatDateTime(item.quarantined_at)}</td>
                        <td>{formatDateTime(item.retain_until)}</td>
                        {isReadWrite && (
                          <td>
                            {item.status === "QUARANTINED" && !restorePlan && (
                              <button
                                className="btn-sm"
                                onClick={() => void handleCreateRestorePlan(item.id)}
                                disabled={restoring}
                              >
                                创建恢复
                              </button>
                            )}
                            {restorePlan && (
                              <div className="exec-plan-actions">
                                <span className={`exec-state-badge exec-state-badge--${restorePlan.state.toLowerCase()}`}>
                                  {RESTORE_STATE_LABELS[restorePlan.state] || restorePlan.state}
                                </span>
                                {restorePlan.state === "DRAFT" && (
                                  <button
                                    className="btn-sm"
                                    onClick={() => void handleApproveRestore(restorePlan.id, restorePlan.approval_digest)}
                                  >
                                    批准
                                  </button>
                                )}
                                {restorePlan.state === "APPROVED" && (
                                  <>
                                    <button
                                      className="btn-sm secondary"
                                      onClick={() => void handleExecuteRestore(restorePlan.id, restorePlan.approval_digest, true)}
                                      disabled={restoring}
                                    >
                                      试运行
                                    </button>
                                    <button
                                      className="btn-sm"
                                      onClick={() => void handleExecuteRestore(restorePlan.id, restorePlan.approval_digest, false)}
                                      disabled={restoring}
                                    >
                                      执行
                                    </button>
                                  </>
                                )}
                              </div>
                            )}
                          </td>
                        )}
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {/* ---- Purge tab ---- */}
      {activeTab === "purge" && (
        <div className="exec-tab-content">
          <div className="exec-toolbar">
            <button
              className="btn-sm"
              onClick={() => void handleCreatePurgePlans()}
              disabled={purging || !isReadWrite}
            >
              {purging ? "生成中…" : "生成清理草案"}
            </button>
            {isReadWrite && (
              <input
                type="text"
                placeholder="隔离根目录"
                value={purgeRoot}
                onChange={(e) => setPurgeRoot(e.target.value)}
                className="exec-root-input"
              />
            )}
          </div>

          {purgeError && <p className="error" role="alert">{purgeError}</p>}

          {purgePlans.length === 0 ? (
            <div className="empty-state">
              <p className="muted">点击「生成清理草案」为到期隔离项创建清理计划</p>
            </div>
          ) : (
            <div className="table-wrap">
              <table className="data-table">
                <thead>
                  <tr>
                    <th>计划 ID</th>
                    <th>状态</th>
                    <th>大小</th>
                    <th>SHA-256</th>
                    <th>保留至</th>
                    <th>操作</th>
                  </tr>
                </thead>
                <tbody>
                  {purgePlans.map((plan) => (
                    <tr key={plan.id}>
                      <td className="mono">
                        {plan.id}
                        <CopyButton text={plan.id} />
                      </td>
                      <td>
                        <span className={planStateBadgeClass(plan.state, "purge")}>
                          {PURGE_STATE_LABELS[plan.state] || plan.state}
                        </span>
                      </td>
                      <td className="num">{formatBytes(plan.expected_size)}</td>
                      <td className="mono">
                        {shortHash(plan.expected_sha256)}
                        <CopyButton text={plan.expected_sha256} />
                      </td>
                      <td>{formatDateTime(plan.retain_until)}</td>
                      <td>
                        <div className="exec-plan-actions">
                          {plan.state === "DRAFT" && (
                            <button
                              className="btn-sm"
                              onClick={() => void handleApprovePurge(plan.id, plan.approval_digest)}
                            >
                              批准
                            </button>
                          )}
                          {plan.state === "APPROVED" && (
                            <>
                              <button
                                className="btn-sm secondary"
                                onClick={() => void handleExecutePurge(plan.id, plan.approval_digest, true)}
                                disabled={purging}
                              >
                                试运行
                              </button>
                              <div className="exec-confirmation">
                                <label htmlFor={`purge-confirm-${plan.id}`}>逐字输入确认语句</label>
                                <code>
                                  {plan.confirmation_text}
                                  <CopyButton text={plan.confirmation_text} />
                                </code>
                                <input
                                  id={`purge-confirm-${plan.id}`}
                                  type="text"
                                  value={purgeConfirmations[plan.id] || ""}
                                  onChange={(e) => setPurgeConfirmations((prev) => ({ ...prev, [plan.id]: e.target.value }))}
                                  placeholder="输入上方确认语句"
                                  className={purgeConfirmations[plan.id] === plan.confirmation_text ? "exec-confirm-match" : ""}
                                />
                              </div>
                              <button
                                className="btn-sm"
                                onClick={() => void handleExecutePurge(
                                  plan.id,
                                  plan.approval_digest,
                                  false,
                                  purgeConfirmations[plan.id] || "",
                                )}
                                disabled={
                                  purging ||
                                  !plan.dry_run_verified_at ||
                                  purgeConfirmations[plan.id] !== plan.confirmation_text
                                }
                              >
                                执行
                              </button>
                              {!plan.dry_run_verified_at && <span className="muted">请先完成试运行</span>}
                            </>
                          )}
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {/* ---- Recovery tab ---- */}
      {activeTab === "recovery" && (
        <div className="exec-tab-content">
          <div className="exec-recovery-panel">
            <div className="exec-recovery-status">
              <h3>恢复状态</h3>
              {recoveryStatus ? (
                <div className="exec-recovery-info">
                  <div>
                    <span className="muted">恢复锁</span>{" "}
                    {recoveryStatus.lock_active ? (
                      <span className="state-badge state-badge--failed">激活</span>
                    ) : (
                      <span className="state-badge state-badge--completed">未激活</span>
                    )}
                  </div>
                  <div>
                    <span className="muted">卡住的执行计划</span> {recoveryStatus.executing_count}
                  </div>
                </div>
              ) : (
                <p className="muted">{recoveryLoading ? "检查中…" : "状态未知"}</p>
              )}
            </div>

            <div className="exec-recovery-actions">
              <h3>恢复操作</h3>
              <p className="muted">以下操作会将卡住的计划恢复至安全终态（回滚或重置）</p>
              <div className="exec-recovery-buttons">
                <button
                  className="btn-sm"
                  onClick={() => void handleRecoverSource()}
                  disabled={recoveryLoading || !isReadWrite}
                >
                  恢复源目录执行
                </button>
                <button
                  className="btn-sm"
                  onClick={() => void handleRecoverRestores()}
                  disabled={recoveryLoading || !isReadWrite}
                >
                  恢复隔离还原
                </button>
                <button
                  className="btn-sm"
                  onClick={() => void handleRecoverPurges()}
                  disabled={recoveryLoading || !isReadWrite}
                >
                  恢复清理操作
                </button>
              </div>
            </div>

            {recoveryResults && (
              <div className="exec-recovery-results">
                <h3>恢复结果</h3>
                <pre className="exec-recovery-log">{recoveryResults}</pre>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
