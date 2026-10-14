import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import { canCommitGovernancePreview, decisionLabel, reasonLabel, type DecisionKind, type ReviewPreview, type SelectedReview } from "../lib/governance";

export default function GovernancePreview({ preview, selection, kind, canRecord, busy, ambiguous, onClose, onCommit, canQueryReceipt, onQueryReceipt, note }: {
  preview: ReviewPreview; selection: Map<string, SelectedReview>; kind: DecisionKind;
  note: string | null; canRecord: boolean; busy: boolean; ambiguous: boolean; canQueryReceipt: boolean; onQueryReceipt: () => void; onClose: () => void; onCommit: () => void;
}) {
  const container = useRef<HTMLDivElement>(null);
  const title = useRef<HTMLHeadingElement>(null);
  const [now, setNow] = useState(Date.now);
  const expiry = Date.parse(preview.expires_at);
  useEffect(() => {
    if (!Number.isFinite(expiry) || expiry <= Date.now()) return;
    const timer = setTimeout(() => setNow(Date.now()), Math.min(expiry - Date.now() + 1, 86400000));
    return () => clearTimeout(timer);
  }, [expiry]);
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    title.current?.focus();
    return () => { if (previous?.isConnected) previous.focus(); };
  }, []);
  const keyboard = (event: KeyboardEvent) => {
    if (event.key === "Escape" && !busy && !ambiguous) { event.preventDefault(); onClose(); }
    if (event.key !== "Tab") return;
    const elements = container.current?.querySelectorAll<HTMLElement>('button:not(:disabled), [tabindex="0"]');
    if (!elements?.length) { event.preventDefault(); title.current?.focus(); return; }
    const first = elements[0], last = elements[elements.length - 1];
    if (event.shiftKey && (document.activeElement === first || document.activeElement === title.current)) { event.preventDefault(); last.focus(); }
    else if (!event.shiftKey && (document.activeElement === last || document.activeElement === title.current)) { event.preventDefault(); first.focus(); }
  };
  const eligible = preview.items.filter((item) => item.eligibility === "ELIGIBLE" && item.source_freshness === "MATCH").length;
  const expired = !!preview.preview_token && (!Number.isFinite(expiry) || expiry <= now);
  const mayRetryAmbiguous = ambiguous && !!preview.preview_token;
  const mayCommit = canRecord && !busy && !!preview.preview_token && !expired && (mayRetryAmbiguous || canCommitGovernancePreview(preview));
  const commit = () => { if (mayCommit) onCommit(); };
  return <div className="gov-modal-backdrop"><div ref={container} className="gov-modal card" role="dialog" aria-modal="true" aria-labelledby="gov-preview-title" onKeyDown={keyboard}>
    <h3 id="gov-preview-title" ref={title} tabIndex={-1}>确认要记录的决定</h3>
    <p><strong>{decisionLabel(kind)}</strong> · 显式选择 {preview.items.length} 组；可记录 {eligible} 组，其余需核对。</p>
    {note === null ? <p className="muted">隐私模式隐藏备注；不会在此展示私人内容。</p> : note && <p className="gov-path">将记录的备注：{note}</p>}
    <p className="gov-records-notice">仅更新治理数据库与审计：不会删除、移动或修改源文件。保存后仍未批准、未执行。</p>
    <p className="muted">保护、恢复锁和平台限制不会因这个预览被解除。部分冲突会逐项反馈。</p>
    <ol className="gov-preview-items">{preview.items.map((item, index) => <li key={item.group_id}>
      <strong>组 {index + 1} · {selection.get(item.group_id)?.group.path_count ?? "未知"} 份文件</strong>
      <span>{item.eligibility === "ELIGIBLE"
        ? item.source_freshness === "MATCH"
          ? `预览时所选文件 SHA-256 与索引一致；保存时会再次核对${item.source_checked_at ? `（${item.source_checked_at}）` : ""}。`
          : "未完成源文件核验，不能记录决定。"
        : reasonLabel(item.reason_code)}</span>
    </li>)}</ol>
    {!canRecord && <p role="status">当前仅可只读预览，不能保存决定。</p>}
    {expired && !ambiguous && <p role="status">预览已过期，请关闭并重新预览。</p>}
    {ambiguous && <p role="status">前次响应未确认。只能重试同一请求；不要新建批次。已记录的项不会自动撤销。</p>}
    <div className="gov-actions">
      <button type="button" className="secondary" disabled={busy || ambiguous} onClick={onClose}>返回核对</button>
      {canQueryReceipt && <button type="button" className="secondary" disabled={busy} onClick={onQueryReceipt}>查询已提交批次回执</button>}
      <button type="button" disabled={!mayCommit} onClick={commit}>
        {busy ? "正在记录…（关闭不会回滚）" : ambiguous ? "重试相同请求" : "保存决定（未批准、未执行）"}
      </button>
    </div>
  </div></div>;
}
