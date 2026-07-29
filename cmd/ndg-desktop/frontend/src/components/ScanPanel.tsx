import { useState, useMemo } from "react";
import { wails } from "../wailsjs/go/models";
import {
  formatDateTime,
  shortHash,
  stateBadgeClass,
  stateLabel,
  stageLabel,
} from "../lib/utils";

export interface ScanPanelProps {
  // form state
  scanRoot: string;
  scanStorageId: string;
  scanFullScan: boolean;
  scanWorkers: string;
  scanStarting: boolean;
  scanError: string | null;
  // active scan
  scanActive: boolean;
  scanProgress: wails.ScanJobProgress | null;
  cancelling: boolean;
  progressPercent: number;
  // job history
  jobs: wails.JobSummary[];
  jobsError: string | null;
  jobDetailLoading: boolean;
  // handlers
  onScanRootChange: (v: string) => void;
  onScanStorageIdChange: (v: string) => void;
  onScanFullScanChange: (v: boolean) => void;
  onScanWorkersChange: (v: string) => void;
  onStartScan: () => void;
  onCancelScan: () => void;
  onSelectJob: (jobId: string) => void;
}

const STATE_FILTER_OPTIONS = [
  { value: "", label: "全部状态" },
  { value: "QUEUED", label: "排队中" },
  { value: "RUNNING", label: "运行中" },
  { value: "CANCEL_REQUESTED", label: "取消中" },
  { value: "COMPLETED", label: "已完成" },
  { value: "FAILED", label: "已失败" },
  { value: "CANCELLED", label: "已取消" },
];

export default function ScanPanel({
  scanRoot,
  scanStorageId,
  scanFullScan,
  scanWorkers,
  scanStarting,
  scanError,
  scanActive,
  scanProgress,
  cancelling,
  progressPercent,
  jobs,
  jobsError,
  jobDetailLoading,
  onScanRootChange,
  onScanStorageIdChange,
  onScanFullScanChange,
  onScanWorkersChange,
  onStartScan,
  onCancelScan,
  onSelectJob,
}: ScanPanelProps) {
  // Job history filters (internal state — purely presentational)
  const [stateFilter, setStateFilter] = useState("");
  const [typeFilter, setTypeFilter] = useState("");

  // Derive unique job types from the data for the filter dropdown
  const jobTypes = useMemo(() => {
    const types = new Set<string>();
    for (const j of jobs) {
      if (j.job_type) types.add(j.job_type);
    }
    return Array.from(types).sort();
  }, [jobs]);

  // Apply filters client-side
  const filteredJobs = useMemo(() => {
    if (!stateFilter && !typeFilter) return jobs;
    return jobs.filter((j) => {
      if (stateFilter && j.state !== stateFilter) return false;
      if (typeFilter && j.job_type !== typeFilter) return false;
      return true;
    });
  }, [jobs, stateFilter, typeFilter]);

  const hasActiveFilter = stateFilter !== "" || typeFilter !== "";

  const clearFilters = () => {
    setStateFilter("");
    setTypeFilter("");
  };

  return (
    <section className="card card--full scan-panel">
      <div className="card-header-row">
        <h2>扫描</h2>
        {scanActive && scanProgress && (
          <span className={stateBadgeClass(scanProgress.state)}>
            {stateLabel(scanProgress.state)}
          </span>
        )}
      </div>

      {/* Scan form */}
      <div className="scan-form" aria-label="扫描参数">
        <label>
          根目录
          <input
            type="text"
            value={scanRoot}
            onChange={(e) => onScanRootChange(e.target.value)}
            placeholder="/path/to/scan"
            disabled={scanActive}
          />
        </label>
        <label>
          存储 ID（可选）
          <input
            type="text"
            value={scanStorageId}
            onChange={(e) => onScanStorageIdChange(e.target.value)}
            placeholder="default"
            disabled={scanActive}
          />
        </label>
        <label>
          并发数（可选）
          <input
            type="number"
            min="1"
            max="32"
            value={scanWorkers}
            onChange={(e) => onScanWorkersChange(e.target.value)}
            placeholder="4"
            disabled={scanActive}
          />
        </label>
        <label className="checkbox-label">
          <input
            type="checkbox"
            checked={scanFullScan}
            onChange={(e) => onScanFullScanChange(e.target.checked)}
            disabled={scanActive}
          />
          全量扫描
        </label>
        <button
          className="btn-sm"
          disabled={scanActive || scanStarting}
          onClick={onStartScan}
        >
          {scanStarting ? "启动中…" : "开始扫描"}
        </button>
      </div>
      {scanError && <p className="error" role="alert">{scanError}</p>}

      {/* Active scan progress */}
      {scanProgress && (
        <div className="scan-progress">
          <div className="progress-bar">
            <div className="progress-bar-fill" style={{ width: `${progressPercent}%` }} />
          </div>
          <div className="progress-stats">
            <span><strong>阶段：</strong>{stageLabel(scanProgress.stage)}</span>
            <span><strong>已发现：</strong>{scanProgress.discovered.toLocaleString()}</span>
            <span><strong>已处理：</strong>{scanProgress.processed.toLocaleString()}</span>
            <span><strong>失败：</strong>{scanProgress.failed.toLocaleString()}</span>
            {scanProgress.warning_count > 0 && (
              <span className="warn"><strong>警告：</strong>{scanProgress.warning_count}</span>
            )}
            {scanProgress.error_code && (
              <span className="error-text"><strong>错误：</strong>{scanProgress.error_code}</span>
            )}
          </div>
          {scanActive && (
            <button
              className="btn-sm secondary"
              disabled={cancelling}
              onClick={onCancelScan}
            >
              {cancelling ? "取消中…" : "取消扫描"}
            </button>
          )}
        </div>
      )}

      {/* Job history */}
      <div className="job-history">
        <div className="card-header-row">
          <h3>任务历史</h3>
          {jobs.length > 0 && (
            <span className="count-badge">
              {hasActiveFilter
                ? `${filteredJobs.length} / ${jobs.length} 条`
                : `共 ${jobs.length} 条`}
            </span>
          )}
        </div>

        {/* Job filters */}
        {jobs.length > 0 && (
          <div className="filter-row job-filter-row" aria-label="任务筛选">
            <label>
              状态
              <select
                value={stateFilter}
                onChange={(e) => setStateFilter(e.target.value)}
              >
                {STATE_FILTER_OPTIONS.map((opt) => (
                  <option key={opt.value} value={opt.value}>{opt.label}</option>
                ))}
              </select>
            </label>
            <label>
              类型
              <select
                value={typeFilter}
                onChange={(e) => setTypeFilter(e.target.value)}
              >
                <option value="">全部类型</option>
                {jobTypes.map((t) => (
                  <option key={t} value={t}>{t}</option>
                ))}
              </select>
            </label>
            {hasActiveFilter && (
              <button className="btn-sm secondary" onClick={clearFilters}>
                清除筛选
              </button>
            )}
          </div>
        )}

        {jobsError ? (
          <p className="error" role="alert">{jobsError}</p>
        ) : jobs.length === 0 ? (
          <p className="muted">暂无任务记录</p>
        ) : filteredJobs.length === 0 ? (
          <p className="muted">没有匹配筛选条件的任务</p>
        ) : (
          <div className="table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th>任务 ID</th>
                  <th>类型</th>
                  <th>状态</th>
                  <th>阶段</th>
                  <th className="num">已发现</th>
                  <th className="num">已处理</th>
                  <th className="num">失败</th>
                  <th>创建时间</th>
                  <th>完成时间</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {filteredJobs.map((j) => (
                  <tr key={j.job_id}>
                    <td className="mono" title={j.job_id}>{shortHash(j.job_id)}</td>
                    <td>{j.job_type}</td>
                    <td>
                      <span className={stateBadgeClass(j.state)}>
                        {stateLabel(j.state)}
                      </span>
                    </td>
                    <td>{stageLabel(j.stage)}</td>
                    <td className="num">{j.discovered ?? 0}</td>
                    <td className="num">{j.processed ?? 0}</td>
                    <td className="num">{j.failed ?? 0}</td>
                    <td className="muted">{formatDateTime(j.created_at)}</td>
                    <td className="muted">{formatDateTime(j.completed_at || "")}</td>
                    <td>
                      <button
                        className="btn-sm"
                        disabled={jobDetailLoading}
                        onClick={() => onSelectJob(j.job_id)}
                      >
                        详情
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </section>
  );
}
