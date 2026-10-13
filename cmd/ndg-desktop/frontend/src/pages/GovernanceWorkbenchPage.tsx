import { useEffect, useState } from "react";
import { useGovernanceWorkbench } from "../hooks/useGovernanceWorkbench";
import { DECISIONS, OUTCOME_LABELS, decisionLabel, filterScope, reasonLabel, type DecisionKind } from "../lib/governance";
import { formatBytes } from "../lib/utils";
import { type AppRoute } from "../app/routes";
import DisabledNotice from "../components/DisabledNotice";
import ErrorState from "../components/ErrorState";
import EmptyState from "../components/EmptyState";
import LoadingState from "../components/LoadingState";
import GovernanceEvidence from "../components/GovernanceEvidence";
import GovernancePreview from "../components/GovernancePreview";
import "../governance.css";

export default function GovernanceWorkbenchPage({ onNavigate }: { onNavigate?: (route: AppRoute) => void }) {
  const flow = useGovernanceWorkbench();
  const { context, query, page, selection, preview, result, caps } = flow;
  const [search, setSearch] = useState("");
  const [storage, setStorage] = useState("");
  const [decisionFilter, setDecisionFilter] = useState("");
  const [minimum, setMinimum] = useState("");
  const [maximum, setMaximum] = useState("");
  const [sort, setSort] = useState<"size" | "path_count" | "group_id">("size");
  const [descending, setDescending] = useState(true);
  const [kind, setKind] = useState<DecisionKind>("KEEP_ALL");
  const [reason, setReason] = useState("");
  const [filterError, setFilterError] = useState("");
  useEffect(() => {
    setSearch(""); setStorage(""); setDecisionFilter(""); setMinimum(""); setMaximum("");
    setSort("size"); setDescending(true); setKind("KEEP_ALL"); setReason(""); setFilterError("");
  }, [context.project?.path]);
  const busy = flow.operation !== null;
  const locked = busy || flow.hasAmbiguousSubmission;
  const scope = filterScope(query);
  const stale = Array.from(selection.values()).filter((item) => item.dataRevision !== context.dataRevision).length;
  const otherFilters = Array.from(selection.values()).filter((item) => item.filterScope !== scope).length;
  const pages = new Set(Array.from(selection.values(), (item) => item.page)).size;
  const totalPages = Math.max(1, Math.ceil(page.total_count / page.page_size));
  const apply = () => {
    const min = minimum ? Number(minimum) * 1024 * 1024 : 0;
    const max = maximum ? Number(maximum) * 1024 * 1024 : 0;
    if (!Number.isSafeInteger(min) || !Number.isSafeInteger(max) || min < 0 || max < 0 || (max > 0 && max < min)) { setFilterError("大小范围必须是有效的非负数，最大值不能小于最小值。"); return; }
    setFilterError("");
    flow.applyQuery({ ...query, search: context.pathPrivacyMode ? "" : search.trim(), storage_id: storage, decision_type: decisionFilter, min_size: min, max_size: max, sort, descending, page: 1 });
  };
  const exportSummary = () => {
    if (!result) return;
    const safe = { records_only: true, status: result.status, counts: result.counts,
      items: result.items.map((item, index) => ({ ordinal: index + 1, status: item.status, explanation: reasonLabel(item.reason_code) })) };
    const url = URL.createObjectURL(new Blob([JSON.stringify(safe, null, 2)], { type: "application/json" }));
    const link = document.createElement("a"); link.href = url; link.download = "ndg-decision-summary.json"; link.click(); URL.revokeObjectURL(url);
  };

  if (!context.capabilities.project_open) return <div className="page"><h2>重复治理</h2><p>请先打开项目。</p></div>;
  return <div className="page gov-workbench">
    <header className="page-header gov-header"><div><h2>重复治理</h2><p className="muted">查看并决定 → 确认并处理；源文件操作仍有独立审批与执行门禁。</p></div><button type="button" className="secondary" disabled={locked || flow.loading} onClick={flow.refresh}>重新载入</button></header>
    <div className="gov-topline"><strong>{context.pathPrivacyMode ? "当前项目" : context.project?.name || "当前项目"}</strong><span>{flow.canRecord ? "可记录决定（非执行许可）" : "只读查看与预览"}</span></div>
    {!flow.canRecord && <DisabledNotice reason={caps.reason_codes.map(reasonLabel).join(" ") || "当前不能记录决定。"} hint="历史记录、文件证据与审计仍可查看。" actionLabel={caps.recovery_lock_active && onNavigate ? "前往审计与恢复" : undefined} onAction={caps.recovery_lock_active && onNavigate ? () => onNavigate("audit-recovery") : undefined} />}
    {context.activeJobId && <DisabledNotice reason="扫描正在更新结果；选择可能过期，提交前必须重新核对。" />}
    {context.connectionStatus !== "connected" && <DisabledNotice reason="连接尚未恢复，当前结果不是源文件仍可达的证明。" />}
    <div className="gov-stat-grid"><div className="card"><span>当前筛选总组数</span><strong>{flow.loading ? "核对中…" : page.total_count}</strong></div><div className="card"><span>当前页</span><strong>{page.page} / {totalPages}</strong></div><div className="card"><span>显式选择</span><strong>{selection.size} 组 · 跨 {pages} 页</strong></div></div>
    <p className="muted">尚不认证可回收物理空间；列表风险和保护统计未完成领域认证，不能以未知状态推断安全。</p>
    <form className="gov-filters card" onSubmit={(event) => { event.preventDefault(); apply(); }} aria-label="治理组筛选">
      <label>文件名搜索<input value={context.pathPrivacyMode ? "" : search} onChange={(event) => setSearch(event.target.value)} disabled={locked || !flow.backendReady || context.pathPrivacyMode} placeholder={context.pathPrivacyMode ? "隐私模式下停用搜索" : "搜索全部匹配组的文件名"} /></label>
      <label>存储源<select value={storage} onChange={(event) => setStorage(event.target.value)} disabled={locked}><option value="">全部存储源</option>{context.storages.map((item, index) => <option key={item.id} value={item.id}>{context.pathPrivacyMode ? "存储源 " + (index + 1) : item.id}</option>)}</select></label>
      <label>决定状态<select value={decisionFilter} onChange={(event) => setDecisionFilter(event.target.value)} disabled={locked || !flow.backendReady}><option value="">全部状态</option><option value="UNREVIEWED">待决定</option>{DECISIONS.map((item) => <option value={item.value} key={item.value}>{item.label}</option>)}</select></label>
      <label>最小单文件（MiB）<input type="number" min="0" step="any" value={minimum} onChange={(event) => setMinimum(event.target.value)} disabled={locked || !flow.backendReady} /></label>
      <label>最大单文件（MiB）<input type="number" min="0" step="any" value={maximum} onChange={(event) => setMaximum(event.target.value)} disabled={locked || !flow.backendReady} /></label>
      <label>排序<select value={sort} onChange={(event) => setSort(event.target.value as typeof sort)} disabled={locked || !flow.backendReady}><option value="size">单文件大小</option><option value="path_count">重复份数</option><option value="group_id">稳定组顺序</option></select></label>
      <label className="gov-check"><input type="checkbox" checked={descending} onChange={(event) => setDescending(event.target.checked)} disabled={locked || !flow.backendReady} />降序</label>
      <button type="submit" disabled={locked || flow.loading}>应用筛选</button>
      {context.pathPrivacyMode && query.search && <p className="gov-filter-note" role="status">当前结果仍按之前的搜索筛选，关键词已隐藏；应用筛选将清除搜索。</p>}
      <p className="muted gov-filter-note">路径全文、风险、目录角色、保护、可执行性和时间筛选尚未认证，当前不可用；不是仅在当前页筛选。</p>
    </form>
    {(flow.error || filterError) && <ErrorState message={filterError || flow.error || ""} />}
    <section className="card gov-selection" aria-label="跨页选择">
      <div className="gov-actions"><button type="button" className="secondary" disabled={locked || !page.items.length || flow.loading} onClick={flow.selectPage}>选择本页 {page.items.length} 组</button><button type="button" className="secondary" disabled={locked || !selection.size} onClick={flow.clear}>清空选择</button><button type="button" className="secondary" disabled title="缺少服务端稳定选集快照">选择全部筛选结果（尚不支持）</button></div>
      <p role="status">已选 {selection.size} 组（跨 {pages} 页），上限 {Math.min(100, caps.max_batch_size || 100)} 组。选择单位是重复组，不是要删除的文件。</p>
      {(stale > 0 || otherFilters > 0) && <p role="alert">{stale} 个选择来自旧扫描结果；{otherFilters} 个选择来自不同筛选范围。预览仍按这些显式选择核对，不会自动重选。</p>}
    </section>
    <div className="gov-layout"><section className="card gov-groups" aria-label="重复组列表">
      {flow.loading ? <LoadingState /> : !page.items.length ? <EmptyState title="没有匹配的重复组" hint="也可能尚未完成扫描；没有结果不等于已经清理文件。" /> : <ul className="gov-group-list" tabIndex={0} aria-label="本页重复组，可滚动查看">{page.items.map((group, index) => <li key={group.group_id} className={selection.has(group.group_id) ? "gov-group-selected" : ""}>
        <label className="gov-group-check"><input type="checkbox" aria-label={"选择本页第 " + (index + 1) + " 组"} checked={selection.has(group.group_id)} disabled={locked || (!selection.has(group.group_id) && selection.size >= Math.min(100, caps.max_batch_size || 100))} onChange={() => flow.toggle(group)} /><strong>内容相同的 {group.path_count} 份文件</strong></label>
        <p className="gov-path muted">{context.pathPrivacyMode ? "安全别名 · 本页组 " + (index + 1) : "代表文件：" + context.displayPath(group.sample_path)}</p>
        <p>{formatBytes(group.size)} / 份 · {decisionLabel(group.decision_type)}</p><p className="muted">系统建议：核对目录用途。风险尚未评估，不能推断可执行。</p>
        <button type="button" className="secondary btn-sm" onClick={() => void flow.showDetail(group)}>查看证据</button>
      </li>)}</ul>}
      <div className="gov-pagination"><button type="button" className="secondary" disabled={locked || flow.loading || query.page <= 1} onClick={() => flow.applyQuery({ ...query, page: query.page - 1 })}>上一页</button><span>第 {page.page} 页 / 共 {totalPages} 页</span><button type="button" className="secondary" disabled={locked || flow.loading || query.page >= totalPages} onClick={() => flow.applyQuery({ ...query, page: query.page + 1 })}>下一页</button></div>
    </section><GovernanceEvidence detail={flow.detail} loading={flow.detailLoading} onClose={flow.closeDetail} /></div>
    <section className="card gov-decision-form" aria-label="记录决定">
      <h3>为显式选择记录决定</h3><label>主要决定<select value={kind} disabled={locked} onChange={(event) => { setKind(event.target.value as DecisionKind); flow.invalidatePreview(); }}>{DECISIONS.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}</select></label>
      <p className="muted">{DECISIONS.find((item) => item.value === kind)?.explanation}</p><label>备注（可选，不写入普通日志）<textarea value={context.pathPrivacyMode ? "" : reason} maxLength={1000} disabled={locked || context.pathPrivacyMode} onChange={(event) => { setReason(event.target.value); flow.invalidatePreview(); }} /></label>
      {context.pathPrivacyMode && reason.trim() && <p role="status">隐私模式隐藏了备注。请关闭隐私模式核对，或<button type="button" className="secondary btn-sm" disabled={locked} onClick={() => { setReason(""); flow.invalidatePreview(); }}>清除隐藏备注</button>后再预览。</p>}
      <div className="gov-actions"><button type="button" disabled={locked || !selection.size || !caps.can_preview || flow.loading || (context.pathPrivacyMode && !!reason.trim())} onClick={() => void flow.makePreview(kind, reason.trim())}>{selection.size === 1 ? "预览这一组决定" : "预览批量决定"}</button><span className="muted">无选择或安全接口不可用时不能提交。</span></div>
      <p className="muted">关系标记继续兼容旧互斥枚举；不会自动归档，也不认证真实备份。历史指定保留记录仍可查看，批量不覆盖该类型。</p>
    </section>
    {result && <section className="card gov-outcomes" aria-label="批次回执" aria-live="polite"><h3>逐组回执：{result.status === "COMPLETE" ? "已核对终态" : result.status === "CANCELLED" ? "已取消（已记录项不会撤销）" : "尚未确认结束"}</h3><p>已记录 {result.counts.saved} · 未变化 {result.counts.unchanged} · 冲突 {result.counts.conflict} · 阻断 {result.counts.blocked} · 失败 {result.counts.failed}；合计 {result.counts.total} 组。未批准、未执行。</p><ol>{result.items.map((item, index) => <li key={item.group_id}>组 {index + 1}：{OUTCOME_LABELS[item.status] || "未知状态，需要核对"}{item.reason_code ? "；" + reasonLabel(item.reason_code) : ""}</li>)}</ol><p>{result.status === "IN_PROGRESS" ? "批次未结束，全部选择和原请求仍保留。请查询回执，不要创建新批次。" : "成功项已从选择中移除；未成功项保留。请重新载入、重新选择变化项后再预览，不自动重放整批。"}</p><button type="button" className="secondary" onClick={exportSummary}>导出脱敏摘要</button></section>}
    <section className="card gov-followup"><h3>需要具体文件操作时</h3><p>记录“制定处理方案”仅保存意图。创建草案、人工复核、审批、Dry Run 和执行仍分别处理；当前没有批量批准或批量执行快捷入口。</p><div className="gov-actions"><button type="button" className="secondary" disabled={!onNavigate} onClick={() => onNavigate?.("governance-review")}>查看单计划复核（兼容入口）</button><button type="button" className="secondary" disabled={!onNavigate} onClick={() => onNavigate?.("audit-recovery")}>审计与恢复</button><button type="button" className="secondary" disabled={!onNavigate || !context.capabilities.can_execute_quarantine} onClick={() => onNavigate?.("execution-center")}>任务中心（仍需单独确认）</button></div><p className="muted">当前版本不支持永久清理；切换页面不提供审批或解除恢复锁的授权。</p></section>
    {preview && <GovernancePreview note={context.pathPrivacyMode ? null : flow.previewIntent?.note ?? ""} preview={preview} selection={selection} kind={flow.previewIntent?.kind ?? kind} canRecord={flow.canCommit} busy={flow.operation === "commit" || flow.operation === "receipt"} canQueryReceipt={flow.canQueryReceipt} onQueryReceipt={() => void flow.queryReceipt()} ambiguous={flow.hasAmbiguousSubmission} onClose={flow.closePreview} onCommit={() => void flow.commit()} />}
  </div>;
}
