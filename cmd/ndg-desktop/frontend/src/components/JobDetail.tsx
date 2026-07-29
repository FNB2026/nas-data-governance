import { wails } from "../wailsjs/go/models";
import {
  formatDateTime,
  stateBadgeClass,
  stateLabel,
  stageLabel,
  eventLabel,
} from "../lib/utils";

export interface JobDetailProps {
  selectedJob: wails.JobDetailResponse | null;
  jobDetailLoading: boolean;
  jobDetailError: string | null;
  onClose: () => void;
}

export default function JobDetail({
  selectedJob,
  jobDetailLoading,
  jobDetailError,
  onClose,
}: JobDetailProps) {
  if (!selectedJob && !jobDetailLoading && !jobDetailError) return null;

  return (
    <section className="card card--full job-detail">
      <div className="card-header-row">
        <h2>任务详情</h2>
        <button className="btn-sm secondary" onClick={onClose}>关闭</button>
      </div>
      {jobDetailLoading ? (
        <p className="muted">加载中…</p>
      ) : jobDetailError ? (
        <p className="error" role="alert">{jobDetailError}</p>
      ) : selectedJob ? (
        <>
          <div className="detail-summary">
            <span><strong>任务 ID：</strong><span className="mono">{selectedJob.job_id}</span></span>
            <span>
              <strong>状态：</strong>
              <span className={stateBadgeClass(selectedJob.state)}>
                {stateLabel(selectedJob.state)}
              </span>
            </span>
            <span><strong>阶段：</strong>{stageLabel(selectedJob.stage)}</span>
            <span><strong>已发现：</strong>{selectedJob.discovered.toLocaleString()}</span>
            <span><strong>已处理：</strong>{selectedJob.processed.toLocaleString()}</span>
            <span><strong>失败：</strong>{selectedJob.failed.toLocaleString()}</span>
            {selectedJob.warning_count > 0 && (
              <span className="warn"><strong>警告：</strong>{selectedJob.warning_count}</span>
            )}
            {selectedJob.error_code && (
              <span className="error-text"><strong>错误码：</strong>{selectedJob.error_code}</span>
            )}
            <span><strong>创建：</strong>{formatDateTime(selectedJob.created_at)}</span>
            {selectedJob.started_at && (
              <span><strong>开始：</strong>{formatDateTime(selectedJob.started_at)}</span>
            )}
            {selectedJob.completed_at && (
              <span><strong>完成：</strong>{formatDateTime(selectedJob.completed_at)}</span>
            )}
          </div>
          <h3>事件时间线</h3>
          <div className="table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th className="num">#</th>
                  <th>事件</th>
                  <th>阶段</th>
                  <th>状态</th>
                  <th>时间</th>
                  <th>载荷</th>
                </tr>
              </thead>
              <tbody>
                {(selectedJob.events || []).map((ev) => (
                  <tr key={ev.sequence}>
                    <td className="num">{ev.sequence}</td>
                    <td>{eventLabel(ev.event_type)}</td>
                    <td>{stageLabel(ev.stage)}</td>
                    <td>
                      <span className={stateBadgeClass(ev.state)}>
                        {stateLabel(ev.state)}
                      </span>
                    </td>
                    <td className="muted">{formatDateTime(ev.created_at)}</td>
                    <td className="payload-cell">
                      {ev.payload && Object.keys(ev.payload).length > 0 ? (
                        <pre className="payload-json">{JSON.stringify(ev.payload)}</pre>
                      ) : (
                        <span className="muted">—</span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      ) : null}
    </section>
  );
}
