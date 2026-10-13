import { useEffect, useRef, useState } from "react";
import { governanceApi } from "../api/governance";
import { OUTCOME_LABELS, reasonLabel, validBatchHistory, validBatchResult, type BatchHistoryPage, type BatchResult, type BatchSummary } from "../lib/governance";
import { useProject } from "../state/ProjectContext";

// Durable discovery is separate from active submission. A historical lookup
// never commits or clears an unresolved request in the workbench.
export default function GovernanceBatchHistory({ refreshSignal, expectedScope }: { refreshSignal: string; expectedScope: string }) {
  const context = useProject();
  const identity = (context.project?.project_id ?? "") + ":" + (context.project?.path ?? "") + ":" + expectedScope;
  const supported = governanceApi.historyAvailable();
  const available = !!context.project && supported && !!expectedScope;
  const [expanded, setExpanded] = useState(false);
  const [page, setPage] = useState<BatchHistoryPage | null>(null);
  const [detail, setDetail] = useState<BatchResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const generation = useRef(0);
  const currentIdentity = useRef(identity);
  currentIdentity.current = identity;
  const load = async (number: number) => {
    const ticket = ++generation.current;
    setLoading(true); setError(""); setDetail(null);
    try {
      const value = await governanceApi.history(number);
      if (ticket !== generation.current || identity !== currentIdentity.current) return;
      if (!validBatchHistory(value, number) || value.project_scope !== expectedScope) throw new Error("UNAVAILABLE");
      setPage(value);
    } catch { if (ticket === generation.current) { setPage(null); setError("无法核对批次历史；请稍后重新查询。不会重新提交决定。"); } }
    finally { if (ticket === generation.current) setLoading(false); }
  };
  useEffect(() => {
    generation.current++; setPage(null); setDetail(null); setError(""); setLoading(false);
    if (expanded && available) void load(1);
    return () => { generation.current++; };
  }, [identity, expanded, available, refreshSignal]);
  const lookup = async (item: BatchSummary) => {
    if (!page || loading) return;
    const ticket = ++generation.current, scope = page.project_scope;
    setLoading(true); setDetail(null); setError("");
    try {
      const value = await governanceApi.result(item.batch_id);
      if (ticket !== generation.current || identity !== currentIdentity.current) return;
      if (value.batch_id !== item.batch_id || value.project_scope !== scope || value.requested_count !== item.requested_count ||
          !Array.isArray(value.items) || !validBatchResult(value, value.items.map((entry) => entry.group_id))) throw new Error("UNAVAILABLE");
      setDetail(value);
    } catch { if (ticket === generation.current) setError("尚未取得可信的历史回执；请刷新核对，不会自动重试或重新提交。"); }
    finally { if (ticket === generation.current) setLoading(false); }
  };
  return <section className="card" aria-label="批次历史">
    <h3>已提交批次与重启后核对</h3>
    <p>这里只查询持久记录；不重试提交、不批准、不执行，也不解除恢复锁。</p>
    {!supported ? <p>当前版本未提供批次发现接口。已知请求回执仍可单独核对。</p> : !available ? <p>尚未核对当前项目作用域，暂不能查询历史。</p> :
      <button type="button" className="secondary" onClick={() => setExpanded(!expanded)}>{expanded ? "收起批次历史" : "查看已提交批次"}</button>}
    {expanded && available && <>
      <button type="button" className="secondary" disabled={loading} onClick={() => void load(page?.page ?? 1)}>刷新批次历史</button>
      {loading && <p role="status">正在只读查询批次记录…</p>}
      {error && <p role="alert">{error}</p>}
      {page && <>
        <p>历史共 {page.total_count} 个批次；当前第 {page.page} 页。</p>
        {!page.items.length && <p>当前项目没有此页批次记录；这不证明源文件已处理。</p>}
        <ol>{page.items.map((item, index) => <li key={item.batch_id}>
          <time dateTime={item.created_at}>{new Date(item.created_at).toLocaleString()}</time>：
          {item.status === "COMPLETE" ? "持久批次终态已记录" : item.status === "CANCELLED" ? "已取消；已记录项不撤销" : "尚未结案，需核对"}；
          请求 {item.requested_count} 组，持久回执 {item.recorded_count} 组。
          <button type="button" className="secondary btn-sm" disabled={loading} onClick={() => void lookup(item)}>核对本页批次 {index + 1}</button>
        </li>)}</ol>
        <button type="button" className="secondary" disabled={loading || page.page <= 1} onClick={() => void load(page.page - 1)}>历史上一页</button>
        <button type="button" className="secondary" disabled={loading || page.page * page.page_size >= page.total_count} onClick={() => void load(page.page + 1)}>历史下一页</button>
      </>}
      {detail && <section aria-label="历史批次回执" aria-live="polite">
        <h4>{detail.status === "IN_PROGRESS" ? "历史批次尚未结束" : "历史持久回执"}</h4>
        <p>已记录 {detail.counts.saved} · 未变化 {detail.counts.unchanged} · 冲突 {detail.counts.conflict} · 阻断 {detail.counts.blocked} · 未能记录 {detail.counts.failed}；共 {detail.requested_count} 组。未批准、未执行。</p>
        <ol>{detail.items.map((item, index) => <li key={item.group_id}>组 {index + 1}：{OUTCOME_LABELS[item.status]}；{reasonLabel(item.reason_code)}</li>)}</ol>
        {detail.status === "IN_PROGRESS" && <p role="status">持久批次仍未结案。请人工核对，不能把尚未持久化的项当成功；这里不会生成新 key 或重新提交。</p>}
      </section>}
    </>}
  </section>;
}
