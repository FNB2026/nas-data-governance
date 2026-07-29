import { useState } from "react";
import { wails, formatdiag, governancediag, merge } from "../wailsjs/go/models";
import {
  DiagnoseFormats,
  DiagnoseGovernance,
  DiagnoseMerges,
} from "../wailsjs/go/wails/API";
import { formatBytes, formatDateTime, hasWailsRuntime, errorText } from "../lib/utils";

type DiagTab = "formats" | "governance" | "merges";

const TAB_LABELS: Record<DiagTab, string> = {
  formats: "格式诊断",
  governance: "治理诊断",
  merges: "合并诊断",
};

export interface DiagnosticPanelProps {
  storages: wails.StorageInfo[];
}

export default function DiagnosticPanel({ storages }: DiagnosticPanelProps) {
  const [activeTab, setActiveTab] = useState<DiagTab>("formats");
  const [storageId, setStorageId] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [formatReport, setFormatReport] = useState<formatdiag.Report | null>(null);
  const [govReport, setGovReport] = useState<governancediag.Report | null>(null);
  const [mergeReport, setMergeReport] = useState<merge.DiagnosticReport | null>(null);

  const runDiagnostic = async () => {
    if (!hasWailsRuntime()) return;
    setLoading(true);
    setError(null);
    try {
      if (activeTab === "formats") {
        const report = await DiagnoseFormats({
          storage_id: storageId,
        } as wails.DiagnoseFormatsRequest);
        setFormatReport(report);
      } else if (activeTab === "governance") {
        const report = await DiagnoseGovernance({
          storage_id: storageId,
        } as wails.DiagnoseGovernanceRequest);
        setGovReport(report);
      } else {
        const report = await DiagnoseMerges({
          storage_id: storageId,
        } as wails.DiagnoseMergesRequest);
        setMergeReport(report);
      }
    } catch (e: unknown) {
      setError(errorText(e));
    } finally {
      setLoading(false);
    }
  };

  const switchTab = (tab: DiagTab) => {
    setActiveTab(tab);
    setError(null);
  };

  return (
    <section className="card card--full diag-panel">
      <div className="card-header-row">
        <h2>诊断报告</h2>
        <span className="muted diag-read-only-hint">只读 · 仅供人工审阅</span>
      </div>

      {/* Tab bar */}
      <div className="diag-tabs" role="tablist">
        {(Object.keys(TAB_LABELS) as DiagTab[]).map((tab) => (
          <button
            key={tab}
            role="tab"
            aria-selected={activeTab === tab}
            className={`diag-tab ${activeTab === tab ? "diag-tab--active" : ""}`}
            onClick={() => switchTab(tab)}
          >
            {TAB_LABELS[tab]}
          </button>
        ))}
      </div>

      {/* Controls */}
      <div className="filter-row" aria-label="诊断参数">
        <label>
          存储
          <select
            value={storageId}
            onChange={(e) => setStorageId(e.target.value)}
          >
            <option value="">全部存储</option>
            {storages.map((s) => (
              <option key={s.id} value={s.id}>{s.id}</option>
            ))}
          </select>
        </label>
        <button
          className="btn-sm"
          disabled={loading}
          onClick={runDiagnostic}
        >
          {loading ? "诊断中…" : "运行诊断"}
        </button>
      </div>

      {error && <p className="error" role="alert">{error}</p>}

      {/* Report content */}
      {activeTab === "formats" && formatReport && (
        <FormatReportView report={formatReport} />
      )}
      {activeTab === "governance" && govReport && (
        <GovernanceReportView report={govReport} />
      )}
      {activeTab === "merges" && mergeReport && (
        <MergeReportView report={mergeReport} />
      )}

      {/* Empty state */}
      {!error && !loading && !formatReport && activeTab === "formats" && (
        <p className="muted">点击「运行诊断」生成格式审查报告</p>
      )}
      {!error && !loading && !govReport && activeTab === "governance" && (
        <p className="muted">点击「运行诊断」生成治理审查报告</p>
      )}
      {!error && !loading && !mergeReport && activeTab === "merges" && (
        <p className="muted">点击「运行诊断」生成合并建议报告</p>
      )}
    </section>
  );
}

// ---- Summary stat grid ----

function StatGrid({ stats }: { stats: { label: string; value: string | number }[] }) {
  return (
    <div className="diag-stat-grid">
      {stats.map((s, i) => (
        <div key={i} className="diag-stat-item">
          <span className="diag-stat-label">{s.label}</span>
          <span className="diag-stat-value">{s.value}</span>
        </div>
      ))}
    </div>
  );
}

// ---- Safety notes ----

function SafetyNotes({ notes }: { notes: string[] }) {
  if (!notes || notes.length === 0) return null;
  return (
    <div className="diag-safety-notes">
      <h4>安全提示</h4>
      <ul>
        {notes.map((n, i) => (
          <li key={i}>{n}</li>
        ))}
      </ul>
    </div>
  );
}

// ---- JSON detail table ----

function JsonDetailTable({ title, rows }: { title: string; rows: any[] }) {
  if (!rows || rows.length === 0) return null;
  return (
    <div className="diag-detail-section">
      <h4>{title} <span className="count-badge">{rows.length}</span></h4>
      <div className="table-wrap">
        <pre className="payload-json diag-json-block">
          {JSON.stringify(rows, null, 2)}
        </pre>
      </div>
    </div>
  );
}

// ---- Format report view ----

function FormatReportView({ report }: { report: formatdiag.Report }) {
  const s = report.summary;
  const generatedAt = typeof report.generated_at === "string"
    ? report.generated_at
    : String(report.generated_at || "");
  return (
    <div className="diag-report">
      <p className="muted diag-generated-at">
        生成时间：{formatDateTime(generatedAt)} · 大文件阈值：{formatBytes(report.large_unknown_minimum)}
      </p>
      <StatGrid stats={[
        { label: "文件总数", value: s.files },
        { label: "格式记录", value: s.format_rows },
        { label: "缺失格式", value: s.missing_format_rows },
        { label: "大未知文件", value: s.large_unknown },
        { label: "扩展名不匹配", value: s.extension_mismatches },
        { label: "元数据缺口", value: s.formats_with_metadata_gap },
      ]} />
      <JsonDetailTable title="大未知文件" rows={report.large_unknown} />
      <JsonDetailTable title="扩展名不匹配" rows={report.extension_mismatches} />
      <JsonDetailTable title="元数据缺口" rows={report.metadata_gaps} />
      <SafetyNotes notes={report.safety_notes} />
    </div>
  );
}

// ---- Governance report view ----

function GovernanceReportView({ report }: { report: governancediag.Report }) {
  const s = report.summary;
  const generatedAt = typeof report.generated_at === "string"
    ? report.generated_at
    : String(report.generated_at || "");
  return (
    <div className="diag-report">
      <p className="muted diag-generated-at">
        生成时间：{formatDateTime(generatedAt)} · 大媒体阈值：{formatBytes(report.large_media_minimum)}
        {report.execution_authorized && <span className="warn"> 执行已授权</span>}
      </p>
      <StatGrid stats={[
        { label: "文件总数", value: s.files },
        { label: "格式记录", value: s.format_rows },
        { label: "缺失格式", value: s.missing_format_rows },
        { label: "重复组", value: s.duplicate_groups },
        { label: "重复文件", value: s.duplicate_files },
        { label: "理论冗余", value: formatBytes(s.theoretical_redundant_bytes) },
        { label: "草稿计划", value: s.draft_plans },
        { label: "非草稿计划", value: s.non_draft_plans },
        { label: "关键计划", value: s.critical_plans },
        { label: "审查动作", value: s.review_actions },
        { label: "隔离候选", value: s.quarantine_candidate_actions },
        { label: "零字节文件", value: s.zero_byte_files },
        { label: "大媒体文件", value: s.large_media_files },
        { label: "大媒体总量", value: formatBytes(s.large_media_bytes) },
        { label: "大媒体含关系", value: s.large_media_with_relations },
        { label: "大媒体含业务锚点", value: s.large_media_with_business_anchor },
        { label: "大媒体项目工作", value: s.large_media_project_work },
        { label: "大媒体受保护", value: s.large_media_protected },
        { label: "缺编解码器", value: s.large_media_missing_codec },
        { label: "缺时长", value: s.large_media_missing_duration },
        { label: "媒体关系", value: s.media_relations },
      ]} />
      <JsonDetailTable title="重复审查" rows={report.duplicate_reviews} />
      <JsonDetailTable title="零字节审查" rows={report.zero_byte_reviews} />
      <JsonDetailTable title="媒体聚合" rows={report.media_aggregates} />
      <JsonDetailTable title="大媒体审查" rows={report.large_media_reviews} />
      <JsonDetailTable title="媒体关系" rows={report.media_relations} />
      <SafetyNotes notes={report.safety_notes} />
    </div>
  );
}

// ---- Merge report view ----

function MergeReportView({ report }: { report: merge.DiagnosticReport }) {
  const s = report.summary;
  const generatedAt = typeof report.generated_at === "string"
    ? report.generated_at
    : String(report.generated_at || "");
  return (
    <div className="diag-report">
      <p className="muted diag-generated-at">
        生成时间：{formatDateTime(generatedAt)} · 建议阈值：{report.suggestion_threshold}
        {report.execution_authorized && <span className="warn"> 执行已授权</span>}
      </p>
      <StatGrid stats={[
        { label: "文件总数", value: s.files },
        { label: "目录数", value: s.directories },
        { label: "同级父目录", value: s.sibling_parents },
        { label: "同级对", value: s.sibling_pairs },
        { label: "名称相似对", value: s.name_similar_pairs },
        { label: "正重叠对", value: s.positive_overlap_pairs },
        { label: "重叠≥0.10", value: s.overlap_at_least_0_10 },
        { label: "重叠≥0.25", value: s.overlap_at_least_0_25 },
        { label: "重叠≥0.50", value: s.overlap_at_least_0_50 },
        { label: "合并建议", value: s.suggestions },
      ]} />
      <JsonDetailTable title="名称相似审查" rows={report.name_similar_reviews} />
      <SafetyNotes notes={report.safety_notes} />
    </div>
  );
}
