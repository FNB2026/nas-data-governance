import { wails } from "../wailsjs/go/models";
import { useProject } from "../state/ProjectContext";
import { formatBytes, formatDateTime } from "../lib/utils";
import CopyButton from "./CopyButton";
import LoadingState from "./LoadingState";

export default function GovernanceEvidence({ detail, loading, onClose }: {
  detail: wails.GroupDetailResponse | null; loading: boolean; onClose: () => void;
}) {
  const { displayPath, pathPrivacyMode } = useProject();
  if (loading) return <section className="card" aria-label="文件证据"><LoadingState /></section>;
  if (!detail) return <section className="card gov-evidence-empty"><h3>先核对文件与目录用途</h3><p className="muted">选择一组的“查看证据”。内容相同不等于可以删除；记录决定也不等于执行。</p></section>;
  return <section className="card gov-evidence" aria-label="文件证据">
    <div className="card-header-row"><h3>文件证据与目录用途</h3><button type="button" className="secondary btn-sm" onClick={onClose}>关闭证据</button></div>
    <p>索引中完整 SHA-256 相同的 {detail.path_count} 个活动文件；当前源文件是否变化仍需执行前校验。哈希一致不证明目录用途相同。</p>
    <p className="muted">风险尚需领域复核。下列保留评分是系统建议，不是审批或执行记录；物理身份不可靠时，不认证可回收物理空间。</p>
    <dl className="gov-facts"><dt>单文件逻辑大小</dt><dd>{formatBytes(detail.size)}</dd><dt>硬链接别名</dt><dd>{detail.hardlink_alias_count}</dd><dt>存储源</dt><dd>{pathPrivacyMode ? "已隐藏" : detail.storage_id}</dd></dl>
    {detail.files.map((file, index) => <article className="gov-member" key={file.path}>
      <h4>文件 {index + 1}{file.dir_context?.protected ? " · 受到保护" : ""}</h4>
      <div className="gov-path"><span className="mono">{displayPath(file.path)}</span><CopyButton text={displayPath(file.path)} label="复制显示路径" /></div>
      <dl className="gov-facts">
        <dt>大小 / 修改时间</dt><dd>{formatBytes(file.size)} / {Number.isFinite(Date.parse(file.modified_at)) ? formatDateTime(file.modified_at) : "尚未验证"}</dd>
        <dt>格式证据</dt><dd>{pathPrivacyMode ? "隐私模式下隐藏详细格式" : file.format_kind || file.format_mime || "尚未验证"}</dd>
        <dt>目录职责</dt><dd>{pathPrivacyMode ? "隐私模式下隐藏详细角色" : file.dir_context?.role || "尚未验证"}</dd>
        <dt>保护状态</dt><dd>{file.dir_context ? file.dir_context.protected ? "受保护；记录决定不能解除保护" : "目录语境未标记保护（不构成执行许可）" : "尚未验证"}</dd>
        <dt>物理身份</dt><dd>{file.physical_reliable ? "扫描时已记录；不保证当前仍相同" : "不可靠或尚未验证"}</dd>
        <dt>保留建议</dt><dd>{file.is_retain_selected ? "建议保留此副本" : "需要人工核对用途"}</dd>
        <dt>建议理由</dt><dd>{pathPrivacyMode ? "隐私模式下隐藏详细理由" : file.retain_reason || "尚未验证"}</dd>
      </dl>
    </article>)}
    <details><summary>技术标识</summary><p className="mono">{pathPrivacyMode ? "隐私模式下隐藏内容哈希与组标识" : detail.group_id}</p></details>
  </section>;
}
