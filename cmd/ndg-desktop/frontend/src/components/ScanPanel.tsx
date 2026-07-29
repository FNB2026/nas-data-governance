import { useState, useMemo, useEffect } from "react";
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
  // job history
  jobs: wails.JobSummary[];
  jobsError: string | null;
  jobDetailLoading: boolean;
  // capability
  canScan: boolean;
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

// Stages where processed/discovered is a valid progress ratio.
// DISCOVERING is excluded — the denominator is still changing.
const DETERMINATE_STAGES = new Set(["QUICK_HASHING", "FULL_HASHING"]);

// Stages where we show an indeterminate bar (activity without percentage).
const INDETERMINATE_STAGES = new Set([
  "DISCOVERING",
  "METADATA_INDEXING",
  "CONTEXT_CLASSIFYING",
  "FORMAT_ANALYZING",
  "GROUPING",
  "PLANNING",
  "FINALIZING",
]);

interface ProgressDisplay {
  mode: "determinate" | "indeterminate" | "terminal";
  percent: number;
}

function computeProgress(stage: string, state: string, progress: wails.ScanJobProgress): ProgressDisplay {
  if (state === "COMPLETED") return { mode: "terminal", percent: 100 };
  if (state === "FAILED" || state === "CANCELLED") return { mode: "terminal", percent: 0 };

  if (DETERMINATE_STAGES.has(stage) && progress.discovered > 0) {
    return {
      mode: "determinate",
      percent: Math.min(100, Math.round((progress.processed / progress.discovered) * 100)),
    };
  }

  if (INDETERMINATE_STAGES.has(stage)) {
    return { mode: "indeterminate", percent: 0 };
  }

  return { mode: "indeterminate", percent: 0 };
}

function formatDuration(startedAt?: string): string {
  if (!startedAt) return "—";
  const start = new Date(startedAt);
  if (isNaN(start.getTime())) return "—";
  const elapsed = Date.now() - start.getTime();
  if (elapsed < 0) return "—";
  const totalSec = Math.floor(elapsed / 1000);
  const h = Math.floor(totalSec / 3600);
  const m = Math.floor((totalSec % 3600) / 60);
  const s = totalSec % 60;
  if (h > 0) return `${h}:${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
  return `${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
}

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
  jobs,
  jobsError,
  jobDetailLoading,
  canScan,
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

  // Tick state to refresh duration display every second
  const [, setTick] = useState(0);
  useEffect(() => {
    if (!scanActive) return;
    const id = setInterval(() => setTick((t) => t + 1), 1000);
    return () => clearInterval(id);
  }, [scanActive]);

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

  const progressDisplay = scanProgress
    ? computeProgress(scanProgress.stage, scanProgress.state, scanProgress)
    : null;

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

      {/* Scan form — hidden in read-only mode */}
      {canScan ? (
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
      ) : (
        <p className="muted">只读模式：可查看任务历史，无法新建扫描。切换到读写模式以创建新扫描。</p>
      )}
      {scanError && <p className="error" role="alert">{scanError}</p>}

      {/* Active scan progress — stage-aware display */}
      {scanProgress && progressDisplay && (
        <div className="scan-progress">
          {/* Progress bar: determinate or indeterminate */}
          {progressDisplay.mode === "determinate" ? (
            <div className="progress-bar">
              <div
                className="progress-bar-fill"
                style={{ width: `${progressDisplay.percent}%` }}
              />
            </div>
          ) : progressDisplay.mode === "indeterminate" ? (
            <div className="progress-bar progress-bar--indeterminate">
              <div className="progress-bar-fill progress-bar-fill--indeterminate" />
            </div>
          ) : null}

          {/* Stats: always show stage + counts */}
          <div className="progress-stats">
            <span><strong>阶段：</strong>{stageLabel(scanProgress.stage)}</span>

            {/* DISCOVERING: show discovered count without percentage */}
            {scanProgress.stage === "DISCOVERING" && (
              <span><strong>正在发现文件：</strong>{scanProgress.discovered.toLocaleString()} 项</span>
            )}

            {/* Determinate stages: show percentage + ratio */}
            {progressDisplay.mode === "determinate" && (
              <span>
                <strong>进度：</strong>{progressDisplay.percent}%
                （{scanProgress.processed.toLocaleString()} / {scanProgress.discovered.toLocaleString()}）
              </span>
            )}

            {/* Non-DISCOVERING indeterminate: show processed count */}
            {progressDisplay.mode === "indeterminate" && scanProgress.stage !== "DISCOVERING" && (
              <span><strong>已处理：</strong>{scanProgress.processed.toLocaleString()}</span>
            )}

            <span><strong>已发现：</strong>{scanProgress.discovered.toLocaleString()}</span>
            <span><strong>失败：</strong>{scanProgress.failed.toLocaleString()}</span>

            {/* Duration */}
            {scanActive && (
              <span><strong>持续：</strong>{formatDuration(scanProgress.started_at)}</span>
            )}

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
          <div className="empty-state">
            <p className="muted">暂无任务记录</p>
            <p className="muted">填写上方扫描参数并点击"开始扫描"以创建第一个任务。</p>
          </div>
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
