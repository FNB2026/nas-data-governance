// Audit & recovery page: operation logs, execution journal, and recovery status.
// V8 wiring — connects audit log queries and journal entry queries.

import { useCallback, useEffect, useState } from "react";
import { useProject } from "../state/ProjectContext";
import { hasWailsRuntime, errorText, formatBytes, shortHash, formatDateTime } from "../lib/utils";
import {
  CheckRecoveryLock,
  ListJournalEntries,
  ListOperationLogs,
} from "../wailsjs/go/wails/API";
import { wails } from "../wailsjs/go/models";

// ---- Constants ----

const JOURNAL_STATUS_LABELS: Record<string, string> = {
  pending: "待执行",
  done: "已完成",
  failed: "失败",
};

const LOG_EVENT_LABELS: Record<string, string> = {
  plan_started: "计划开始",
  plan_completed: "计划完成",
  plan_failed: "计划失败",
  action_started: "动作开始",
  action_completed: "动作完成",
  action_failed: "动作失败",
  rollback_started: "回滚开始",
  rollback_completed: "回滚完成",
  rollback_failed: "回滚失败",
  stale_check: "过期检查",
  quarantine_registered: "隔离注册",
};

// ---- Component ----

export default function AuditRecoveryPage() {
  const { capabilities, dataRevision } = useProject();

  // Plan filter
  const [planFilter, setPlanFilter] = useState<string>("");

  // Audit logs
  const [logs, setLogs] = useState<wails.OperationLogDTO[]>([]);
  const [logsLoading, setLogsLoading] = useState(false);
  const [logsError, setLogsError] = useState<string | null>(null);

  // Journal entries
  const [journal, setJournal] = useState<wails.JournalEntryDTO[]>([]);
  const [journalLoading, setJournalLoading] = useState(false);
  const [journalError, setJournalError] = useState<string | null>(null);

  // Recovery status
  const [recoveryStatus, setRecoveryStatus] = useState<wails.RecoveryStatusDTO | null>(null);

  // ---- Data loading ----

  const loadLogs = useCallback(async () => {
    if (!hasWailsRuntime()) return;
    setLogsLoading(true);
    setLogsError(null);
    try {
      const list = await ListOperationLogs(planFilter);
      setLogs(list || []);
    } catch (e: unknown) {
      setLogsError(errorText(e));
      setLogs([]);
    } finally {
      setLogsLoading(false);
    }
  }, [planFilter]);

  const loadJournal = useCallback(async () => {
    if (!hasWailsRuntime()) return;
    setJournalLoading(true);
    setJournalError(null);
    try {
      const list = await ListJournalEntries(planFilter);
      setJournal(list || []);
    } catch (e: unknown) {
      setJournalError(errorText(e));
      setJournal([]);
    } finally {
      setJournalLoading(false);
    }
  }, [planFilter]);

  const loadRecoveryStatus = useCallback(async () => {
    if (!hasWailsRuntime()) return;
    try {
      const status = await CheckRecoveryLock();
      setRecoveryStatus(status);
    } catch {
      // Non-fatal
    }
  }, []);

  const loadAll = useCallback(async () => {
    await Promise.all([loadLogs(), loadJournal(), loadRecoveryStatus()]);
  }, [loadLogs, loadJournal, loadRecoveryStatus]);

  useEffect(() => {
    if (capabilities.project_open) {
      void loadAll();
    }
  }, [capabilities.project_open, dataRevision, loadAll]);

  // Unique plan IDs from logs and journal for the filter dropdown
  const knownPlanIds = Array.from(new Set([
    ...logs.map((l) => l.plan_id),
    ...journal.map((j) => j.plan_id),
  ])).sort();

  // ---- Render ----

  if (!capabilities.project_open) {
    return (
      <div className="page page--audit-recovery">
        <div className="page-header">
          <h2>审计与恢复</h2>
          <p className="muted">请先打开项目</p>
        </div>
      </div>
    );
  }

  return (
    <div className="page page--audit-recovery">
      <div className="page-header">
        <h2>审计与恢复</h2>
        <p className="muted">操作审计日志、执行 Journal 与恢复状态</p>
      </div>

      {/* Recovery lock banner */}
      {recoveryStatus?.lock_active && (
        <div className="exec-lock-banner" role="alert">
          <strong>恢复锁激活</strong> — {recoveryStatus.executing_count} 个计划卡在执行状态，请前往「执行中心」处理
        </div>
      )}

      {/* Filter toolbar */}
      <div className="audit-toolbar">
        <select value={planFilter} onChange={(e) => setPlanFilter(e.target.value)}>
          <option value="">全部计划 ({logs.length + journal.length})</option>
          {knownPlanIds.map((id) => (
            <option key={id} value={id}>{id}</option>
          ))}
        </select>
        <button className="btn-sm secondary" onClick={() => void loadAll()}>
          刷新
        </button>
        {recoveryStatus && (
          <span className={`audit-recovery-badge ${recoveryStatus.lock_active ? "audit-recovery-badge--active" : ""}`}>
            {recoveryStatus.lock_active ? `恢复锁 (${recoveryStatus.executing_count})` : "无恢复锁"}
          </span>
        )}
      </div>

      {/* Two-panel layout */}
      <div className="audit-layout">
        {/* Audit logs panel */}
        <div className="audit-panel">
          <h3>操作审计 ({logs.length})</h3>
          {logsError && <p className="error" role="alert">{logsError}</p>}
          {logsLoading ? (
            <p className="muted">加载中…</p>
          ) : logs.length === 0 ? (
            <p className="muted">暂无审计日志</p>
          ) : (
            <div className="table-wrap">
              <table className="data-table">
                <thead>
                  <tr>
                    <th>时间</th>
                    <th>计划</th>
                    <th>事件</th>
                  </tr>
                </thead>
                <tbody>
                  {logs.slice(0, 100).map((log) => (
                    <tr key={log.id}>
                      <td className="mono">{formatDateTime(log.created_at)}</td>
                      <td className="mono">{log.plan_id}</td>
                      <td>
                        <span className="audit-event-tag">
                          {LOG_EVENT_LABELS[log.event_type] || log.event_type}
                        </span>
                        {log.detail && Object.keys(log.detail).length > 0 && (
                          <details className="audit-detail">
                            <summary>详情</summary>
                            <pre>{JSON.stringify(log.detail, null, 2)}</pre>
                          </details>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {logs.length > 100 && (
                <p className="muted audit-truncated">仅显示前 100 条（共 {logs.length} 条）</p>
              )}
            </div>
          )}
        </div>

        {/* Journal entries panel */}
        <div className="audit-panel">
          <h3>执行 Journal ({journal.length})</h3>
          {journalError && <p className="error" role="alert">{journalError}</p>}
          {journalLoading ? (
            <p className="muted">加载中…</p>
          ) : journal.length === 0 ? (
            <p className="muted">暂无 Journal 记录</p>
          ) : (
            <div className="table-wrap">
              <table className="data-table">
                <thead>
                  <tr>
                    <th>#</th>
                    <th>类型</th>
                    <th>状态</th>
                    <th>回滚</th>
                    <th>大小</th>
                    <th>SHA-256</th>
                    <th>开始</th>
                    <th>完成</th>
                  </tr>
                </thead>
                <tbody>
                  {journal.slice(0, 100).map((entry, i) => (
                    <tr key={i}>
                      <td className="num">{entry.action_index}</td>
                      <td className="mono">{entry.action_type}</td>
                      <td>
                        <span className={`journal-status journal-status--${entry.status}`}>
                          {JOURNAL_STATUS_LABELS[entry.status] || entry.status}
                        </span>
                      </td>
                      <td>
                        {entry.rollback_status && (
                          <span className={`journal-status journal-status--${entry.rollback_status}`}>
                            {JOURNAL_STATUS_LABELS[entry.rollback_status] || entry.rollback_status}
                          </span>
                        )}
                      </td>
                      <td className="num">{formatBytes(entry.file_size)}</td>
                      <td className="mono">{shortHash(entry.content_sha256)}</td>
                      <td className="mono">{entry.started_at ? formatDateTime(entry.started_at) : "—"}</td>
                      <td className="mono">{entry.completed_at ? formatDateTime(entry.completed_at) : "—"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {journal.length > 100 && (
                <p className="muted audit-truncated">仅显示前 100 条（共 {journal.length} 条）</p>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
