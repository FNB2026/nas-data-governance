import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import { governanceApi } from "../api/governance";
import { useProject } from "../state/ProjectContext";
import { wails } from "../wailsjs/go/models";
import { filterScope, subject, validBatchResult, type BatchResult, type DecisionKind, type GovernanceCapabilities,
  type GovernanceQuery, type ReviewGroup, type ReviewPage, type ReviewPreview, type SelectedReview } from "../lib/governance";

export const INITIAL_QUERY: GovernanceQuery = {
  storage_id: "", search: "", decision_type: "", min_size: 0, max_size: 0,
  sort: "size", descending: true, page: 1, page_size: 20,
};
const CLOSED_CAPS: GovernanceCapabilities = {
  can_record_decisions: false, can_preview: false, recovery_lock_active: true,
  scan_active: false, max_batch_size: 100, reason_codes: ["UNAVAILABLE"], project_scope: "",
};

type Submission = { token: string; key: string; ids: string[]; scope: string; batchId?: string };
// Session-only reconciliation survives route unmounts. No paths, notes or
// selection payloads enter localStorage, URLs or ordinary logs.
const inFlightMutations = new Set<string>();
const unresolved = new Map<string, { sent: Submission; preview: ReviewPreview; selection: Map<string, SelectedReview> }>();

export function useGovernanceWorkbench() {
  const context = useProject();
  const { project, dataRevision } = context;
  const projectKey = project?.path ?? "";
  const [query, setQuery] = useState<GovernanceQuery>(INITIAL_QUERY);
  const [page, setPage] = useState<ReviewPage>({ items: [], total_count: 0, page: 1, page_size: 20, unavailable_filters: [] });
  const [caps, setCaps] = useState(CLOSED_CAPS);
  const [selection, setSelection] = useState<Map<string, SelectedReview>>(() => new Map());
  const [preview, setPreview] = useState<ReviewPreview | null>(null);
  const [result, setResult] = useState<BatchResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [operation, setOperation] = useState<"preview" | "commit" | "receipt" | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [detail, setDetail] = useState<wails.GroupDetailResponse | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [backendReady, setBackendReady] = useState(false);
  const generation = useRef(0);
  const projectEpoch = useRef(0);
  const detailGeneration = useRef(0);
  const boundProject = useRef(projectKey);
  const legacyCursors = useRef(new Map<number, string>([[1, ""]]));
  // Kept only in memory. Retry the identical request; never mint a fresh key
  // after an ambiguous IPC response or replay a file execution method.
  const submission = useRef<Submission | null>(unresolved.get(projectKey)?.sent ?? null);
  const activeOperation = useRef(false);

  const load = useCallback(async (nextQuery: GovernanceQuery) => {
    const ticket = ++generation.current;
    detailGeneration.current++;
    setPreview(null); setDetail(null); setDetailLoading(false); setError(null); setLoading(true);
    if (!projectKey) { setLoading(false); setPage({ items: [], total_count: 0, page: 1, page_size: 20, unavailable_filters: [] }); return; }
    const ready = governanceApi.available();
    setBackendReady(ready);
    try {
      if (ready) {
        const [list, actualCaps] = await Promise.all([governanceApi.list(nextQuery), governanceApi.capabilities()]);
        if (ticket !== generation.current || boundProject.current !== projectKey) return;
        setPage(list); setCaps(actualCaps);
      } else {
        // Compatibility view only. This fallback has no decision mutation.
        const cursor = legacyCursors.current.get(nextQuery.page);
        if (cursor === undefined) throw new Error("UNAVAILABLE");
        const list = await api.duplicates.listGroups({ storage_id: nextQuery.storage_id, page_size: 20, cursor, min_reclaimable_bytes: 0 } as wails.ListGroupsRequest);
        if (ticket !== generation.current || boundProject.current !== projectKey) return;
        if (list.next_cursor) legacyCursors.current.set(nextQuery.page + 1, list.next_cursor);
        setPage({ items: (list.groups ?? []).map((g) => ({ ...g, decision_type: g.decision_type ?? "", subject_version: "", decision_revision: "", risk: "UNKNOWN", role: "unknown", safety_flags: [] })),
          page: nextQuery.page, page_size: 20, total_count: list.total_count, unavailable_filters: ["search", "decision", "size", "sort", "risk", "role", "protected", "time"] });
        setCaps(CLOSED_CAPS);
      }
    } catch {
      if (ticket === generation.current) { setError("无法读取当前结果，请重新载入；未提交任何新的决定。"); setCaps(CLOSED_CAPS); }
    } finally { if (ticket === generation.current) {
      const pending = unresolved.get(projectKey);
      if (pending) { submission.current = pending.sent; setPreview(pending.preview); setSelection(new Map(pending.selection)); }
      setLoading(false);
    } }
  }, [projectKey]);

  useEffect(() => {
    const changed = boundProject.current !== projectKey;
    boundProject.current = projectKey;
    if (changed) {
      projectEpoch.current++;
      submission.current = null; activeOperation.current = false;
      setSelection(new Map()); setResult(null); setCaps(CLOSED_CAPS); setOperation(null);
      legacyCursors.current = new Map([[1, ""]]);
      setQuery(INITIAL_QUERY);
    }
    void load(changed ? INITIAL_QUERY : query);
    return () => { generation.current++; detailGeneration.current++; };
    // Queries are applied by explicit actions; data changes preserve selections
    // as stale snapshots rather than silently selecting new group versions.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectKey, dataRevision, load]);

  const applyQuery = (next: GovernanceQuery) => {
    if (activeOperation.current || submission.current) return;
    if (filterScope(next) !== filterScope(query)) legacyCursors.current = new Map([[1, ""]]);
    setQuery(next); setResult(null); submission.current = null;
    void load(next);
  };

  const toggle = (group: ReviewGroup) => {
    if (activeOperation.current || submission.current) return;
    setPreview(null); submission.current = null; setResult(null);
    setSelection((previous) => {
      const next = new Map(previous);
      if (next.has(group.group_id)) next.delete(group.group_id);
      else if (next.size < Math.min(100, caps.max_batch_size || 100)) next.set(group.group_id, { group, filterScope: filterScope(query), dataRevision, page: query.page });
      return next;
    });
  };

  const selectPage = () => {
    if (activeOperation.current || submission.current) return;
    setPreview(null); submission.current = null; setResult(null);
    setSelection((previous) => {
      const next = new Map(previous);
      for (const group of page.items) {
        if (next.size >= Math.min(100, caps.max_batch_size || 100) && !next.has(group.group_id)) break;
        next.set(group.group_id, { group, filterScope: filterScope(query), dataRevision, page: query.page });
      }
      return next;
    });
  };

  const clear = () => {
    if (activeOperation.current || submission.current) return;
    setSelection(new Map()); setPreview(null); setResult(null); submission.current = null;
  };

  const showDetail = async (group: ReviewGroup) => {
    const ticket = ++detailGeneration.current;
    setDetail(null); setDetailLoading(true);
    try {
      const value = await api.duplicates.getDetail(group.storage_id, group.sha256);
      if (ticket === detailGeneration.current && boundProject.current === projectKey) setDetail(value);
    } catch { if (ticket === detailGeneration.current) setError("无法读取这组文件的证据，请重新载入。"); }
    finally { if (ticket === detailGeneration.current) setDetailLoading(false); }
  };

  const makePreview = async (kind: DecisionKind, reason: string) => {
    if (!selection.size || activeOperation.current || submission.current || !caps.can_preview || boundProject.current !== projectKey) return;
    activeOperation.current = true; setOperation("preview"); setError(null); setResult(null); setPreview(null); submission.current = null;
    const ticket = generation.current;
    const epoch = projectEpoch.current;
    try {
      const value = await governanceApi.preview({ items: Array.from(selection.values(), (item) => subject(item.group)), decision_type: kind, reason });
      if (ticket !== generation.current || boundProject.current !== projectKey) return;
      const ids = new Set(selection.keys());
      if (!value.records_only || value.project_scope !== caps.project_scope || value.items.length !== ids.size ||
          new Set(value.items.map((item) => item.group_id)).size !== ids.size || value.items.some((item) => !ids.has(item.group_id))) throw new Error("UNAVAILABLE");
      setPreview(value);
    } catch { if (ticket === generation.current) setError("无法完成安全预览，请重新载入；未批准或执行文件操作。"); }
    finally { if (epoch === projectEpoch.current) { activeOperation.current = false; setOperation(null); } }
  };

  const canRecord = !inFlightMutations.has(projectKey) && boundProject.current === projectKey && context.isReadWrite && context.capabilities.can_edit_reviews &&
    !context.capabilities.recovery_lock_active && !context.activeJobId && caps.can_record_decisions && !caps.recovery_lock_active && !caps.scan_active;

  const acceptReceipt = async (value: BatchResult, sent: NonNullable<typeof submission.current>, ticket: number, epoch: number) => {
    if (epoch !== projectEpoch.current || boundProject.current !== projectKey) return;
    if (value.project_scope !== sent.scope || !validBatchResult(value, sent.ids) || !value.batch_id ||
        (sent.batchId && value.batch_id !== sent.batchId)) throw new Error("UNAVAILABLE");
      setResult(value);
      sent.batchId = value.batch_id;
      if (value.status === "IN_PROGRESS") { setError("批次仍未确认结束，请查询回执；不要创建新的请求重放。"); return; }
      const succeeded = new Set(value.items.filter((item) => item.status === "SAVED" || item.status === "UNCHANGED").map((item) => item.group_id));
      setSelection((previous) => new Map(Array.from(previous).filter(([id]) => !succeeded.has(id))));
      setPreview(null); submission.current = null; unresolved.delete(projectKey);
      // Reload read-only state without losing failed/blocked selections.
      try {
        const [nextPage, actualCaps] = await Promise.all([governanceApi.list(query), governanceApi.capabilities()]);
        if (ticket === generation.current) { setPage(nextPage); setCaps(actualCaps); }
      } catch { if (epoch === projectEpoch.current) setError("决定回执已确认，但列表更新失败；请重新载入。不要重放已成功的条目。"); }
  };

  const commit = async () => {
    if (!canRecord || !preview?.preview_token || activeOperation.current || inFlightMutations.has(projectKey)) return;
    if (!submission.current) {
      const bytes = crypto.getRandomValues(new Uint8Array(16));
      const key = Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("");
      submission.current = { token: preview.preview_token, key, ids: preview.items.map((item) => item.group_id), scope: preview.project_scope };
      unresolved.set(projectKey, { sent: submission.current, preview, selection: new Map(selection) });
    }
    const sent = submission.current;
    const ticket = generation.current;
    const epoch = projectEpoch.current;
    activeOperation.current = true; inFlightMutations.add(projectKey); setOperation("commit"); setError(null);
    try {
      const value = await governanceApi.commit(sent.token, sent.key);
      if (epoch !== projectEpoch.current || boundProject.current !== projectKey) return;
      if (value.project_scope !== sent.scope || !validBatchResult(value, sent.ids)) throw new Error("UNAVAILABLE");
      await acceptReceipt(value, sent, ticket, epoch);
    } catch { if (epoch === projectEpoch.current) setError("响应未能确认。已提交的决定可能已持久化；请使用“重试相同请求”核对，不要重复创建批次。"); }
    finally { inFlightMutations.delete(projectKey); if (epoch === projectEpoch.current) { activeOperation.current = false; setOperation(null); } }
  };

  const queryReceipt = async () => {
    const sent = submission.current;
    if (!sent || activeOperation.current || boundProject.current !== projectKey) return;
    const ticket = generation.current, epoch = projectEpoch.current;
    activeOperation.current = true; setOperation("receipt"); setError(null);
    try { await acceptReceipt(await governanceApi.result(sent.batchId || "key:" + sent.key), sent, ticket, epoch); }
    catch { if (epoch === projectEpoch.current) setError("尚未取得可信回执；原请求仍保留，请稍后重新查询。查询不会再次提交决定。"); }
    finally { if (epoch === projectEpoch.current) { activeOperation.current = false; setOperation(null); } }
  };

  return { context, query, page, caps, selection, preview, result, loading, operation, error, detail, detailLoading, backendReady,
    canRecord, hasAmbiguousSubmission: submission.current !== null, applyQuery, toggle, selectPage, clear, showDetail, makePreview, commit, queryReceipt, canQueryReceipt: submission.current !== null,
    closeDetail: () => { detailGeneration.current++; setDetail(null); setDetailLoading(false); },
    closePreview: () => { if (!activeOperation.current && !submission.current) setPreview(null); },
    invalidatePreview: () => { if (!activeOperation.current && !submission.current) setPreview(null); },
    refresh: () => { if (!activeOperation.current && !submission.current) void load(query); },
  };
}
