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
        <h3>任务历史</h3>
        {jobsError ? (
          <p className="error" role="alert">{jobsError}</p>
        ) : jobs.length === 0 ? (
          <p className="muted">暂无任务记录</p>
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
                {jobs.map((j) => (
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
