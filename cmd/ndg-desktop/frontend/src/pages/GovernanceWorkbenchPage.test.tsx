// @vitest-environment jsdom
// Synthetic component contract only, not native Wails/filesystem acceptance.
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { BatchResult, GovernanceQuery, PreviewRequest, ReviewGroup } from "../lib/governance";

const { bridge, context, oldApi } = vi.hoisted(() => ({
  bridge: { available: vi.fn(), capabilities: vi.fn(), list: vi.fn(), preview: vi.fn(), commit: vi.fn(), result: vi.fn() },
  context: {} as Record<string, unknown>,
  oldApi: { duplicates: { getDetail: vi.fn(), listGroups: vi.fn() } },
}));
vi.mock("../api/governance", () => ({ governanceApi: bridge }));
vi.mock("../api/client", () => ({ api: oldApi }));
vi.mock("../state/ProjectContext", () => ({ useProject: () => context }));
import GovernanceWorkbenchPage from "./GovernanceWorkbenchPage";
import { maskPath } from "../state/settings";

const groups: ReviewGroup[] = [1, 2, 3].map((n) => ({ group_id: "g" + n, storage_id: "synthetic-source", sha256: "synthetic-hash-" + n,
  sample_path: "/synthetic-fixture/marker_" + n + ".txt", size: n * 1024, path_count: 2,
  decision_type: "", subject_version: "subject-v" + n, decision_revision: 0, risk: "UNKNOWN", role: "unknown", safety_flags: [] }));
function result(ids: string[], conflict = ""): BatchResult {
  return { batch_id: "synthetic-batch", project_scope: "project-a", status: "COMPLETE", requested_count: ids.length,
    counts: { total: ids.length, saved: ids.length - (conflict ? 1 : 0), unchanged: 0, conflict: conflict ? 1 : 0, blocked: 0, failed: 0 },
    items: ids.map((id) => ({ group_id: id, status: id === conflict ? "CONFLICT" : "SAVED", reason_code: id === conflict ? "SUBJECT_CHANGED" : "", decision_revision: 1 })) };
}
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>((yes) => { resolve = yes; }); return { promise, resolve }; }

let projectOrdinal = 0;
beforeEach(() => {
  Object.assign(context, { project: { path: "/synthetic-ui/project-" + (++projectOrdinal) + ".json", name: "合成验收项目" }, isReadWrite: true,
    capabilities: { project_open: true, can_edit_reviews: true, can_execute_quarantine: true, recovery_lock_active: false },
    dataRevision: 0, pathPrivacyMode: false, displayPath: (path: string) => context.pathPrivacyMode ? maskPath(path) : path,
    storages: [{ id: "synthetic-source" }], activeJobId: null, connectionStatus: "connected" });
  bridge.available.mockReset().mockReturnValue(true);
  bridge.capabilities.mockReset().mockResolvedValue({ can_record_decisions: true, can_preview: true, recovery_lock_active: false,
    scan_active: false, max_batch_size: 100, reason_codes: [], project_scope: "project-a" });
  bridge.list.mockReset().mockImplementation(async (query: GovernanceQuery) => ({ items: query.page === 1 ? groups.slice(0, 2) : groups.slice(2), total_count: 3, page: query.page, page_size: 2, unavailable_filters: ["risk"] }));
  bridge.preview.mockReset().mockImplementation(async (request: PreviewRequest) => ({ preview_token: "synthetic-token", expires_at: new Date(Date.now() + 60000).toISOString(), project_scope: "project-a", records_only: true,
    items: request.items.map((item) => ({ ...item, eligibility: "ELIGIBLE", reason_code: "" })) }));
  bridge.commit.mockReset().mockImplementation(async () => result(bridge.preview.mock.calls[bridge.preview.mock.calls.length - 1]?.[0].items.map((item: ReviewGroup) => item.group_id) || []));
  bridge.result.mockReset();
  oldApi.duplicates.listGroups.mockReset().mockResolvedValue({ groups, total_count: 3, next_cursor: "" });
  oldApi.duplicates.getDetail.mockReset().mockResolvedValue({ group_id: "g1", sha256: "synthetic-hash-1", path_count: 2, size: 1024, hardlink_alias_count: 0, storage_id: "synthetic-source", files: [] });
});
afterEach(cleanup);
async function ready() { await screen.findByText("代表文件：/synthetic-fixture/marker_1.txt"); }
async function chooseFirst() { await ready(); fireEvent.click(screen.getByRole("checkbox", { name: "选择本页第 1 组" })); }
async function preview() { fireEvent.click(screen.getByRole("button", { name: /预览这一组决定|预览批量决定/ })); await screen.findByRole("dialog"); }

describe("governance workbench selection and records-only flow", () => {
  it("records a single intent on the same page without an execution API", async () => {
    render(<GovernanceWorkbenchPage />); await chooseFirst(); await preview();
    expect(screen.getByText(/不会删除、移动或修改源文件/)).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "保存决定（未批准、未执行）" }));
    await screen.findByRole("region", { name: "批次回执" });
    expect(bridge.commit).toHaveBeenCalledTimes(1);
    expect(screen.getByText(/已选 0 组/)).toBeVisible();
  });
  it("preserves explicit groups across pages and never submits hidden unselected IDs", async () => {
    render(<GovernanceWorkbenchPage />); await chooseFirst();
    fireEvent.click(screen.getByRole("button", { name: "下一页" }));
    await screen.findByText("代表文件：/synthetic-fixture/marker_3.txt");
    fireEvent.click(screen.getByRole("checkbox", { name: "选择本页第 1 组" }));
    expect(screen.getByText(/已选 2 组（跨 2 页）/)).toBeVisible();
    await preview();
    expect(bridge.preview.mock.calls[0][0].items.map((item: ReviewGroup) => item.group_id)).toEqual(["g1", "g3"]);
    expect(screen.getByRole("button", { name: "选择全部筛选结果（尚不支持）" })).toBeDisabled();
  });
  it("selects only the current loaded page, retaining choices on sort and warning on filter changes", async () => {
    render(<GovernanceWorkbenchPage />); await ready();
    fireEvent.click(screen.getByRole("button", { name: "选择本页 2 组" }));
    fireEvent.change(screen.getByLabelText("文件名搜索"), { target: { value: "marker" } });
    fireEvent.click(screen.getByRole("button", { name: "应用筛选" }));
    await waitFor(() => expect(bridge.list.mock.calls[bridge.list.mock.calls.length - 1]?.[0].search).toBe("marker"));
    expect(screen.getByRole("alert")).toHaveTextContent("2 个选择来自不同筛选范围");
    expect(screen.getByText(/已选 2 组/)).toBeVisible();
  });
  it("keeps unsuccessful selections and reports per-item outcomes instead of total success", async () => {
    bridge.commit.mockResolvedValue(result(["g1", "g2"], "g2"));
    render(<GovernanceWorkbenchPage />); await ready(); fireEvent.click(screen.getByRole("button", { name: "选择本页 2 组" })); await preview();
    fireEvent.click(screen.getByRole("button", { name: "保存决定（未批准、未执行）" }));
    await screen.findByRole("region", { name: "批次回执" });
    expect(screen.getByRole("checkbox", { name: "选择本页第 1 组" })).not.toBeChecked();
    expect(screen.getByRole("checkbox", { name: "选择本页第 2 组" })).toBeChecked();
    expect(screen.getByText(/组 2：数据或决定发生冲突/)).toBeVisible();
    expect(screen.getByText(/已选 1 组/)).toBeVisible();
  });
  it("retries an ambiguous transport with exactly the same token and idempotency key", async () => {
    bridge.commit.mockRejectedValueOnce(new Error("/synthetic-private-marker/credential"));
    render(<GovernanceWorkbenchPage />); await chooseFirst(); await preview();
    fireEvent.click(screen.getByRole("button", { name: "保存决定（未批准、未执行）" }));
    const retry = await screen.findByRole("button", { name: "重试相同请求" });
    expect(screen.getByRole("button", { name: "清空选择" })).toBeDisabled();
    expect(document.body.textContent).not.toContain("synthetic-private-marker");
    fireEvent.click(retry); await screen.findByRole("region", { name: "批次回执" });
    expect(bridge.commit.mock.calls[1]).toEqual(bridge.commit.mock.calls[0]);
  });
  it("queries a lost response by the retained key without resubmitting, even under a later recovery lock", async () => {
    bridge.commit.mockRejectedValueOnce(new Error("ambiguous transport"));
    bridge.result.mockResolvedValue(result(["g1"]));
    const view = render(<GovernanceWorkbenchPage />); await chooseFirst(); await preview();
    fireEvent.click(screen.getByRole("button", { name: "保存决定（未批准、未执行）" }));
    await screen.findByRole("button", { name: "查询已提交批次回执" });
    context.capabilities = { project_open: true, can_edit_reviews: false, recovery_lock_active: true };
    view.rerender(<GovernanceWorkbenchPage />);
    expect(screen.getByRole("button", { name: "重试相同请求" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "查询已提交批次回执" }));
    await screen.findByRole("region", { name: "批次回执" });
    expect(bridge.result).toHaveBeenCalledWith("key:" + bridge.commit.mock.calls[0][1]);
    expect(bridge.commit).toHaveBeenCalledTimes(1);
    expect(screen.getByText(/已选 0 组/)).toBeVisible();
  });
  it("keeps IN_PROGRESS locked and resolves it through a read-only batch receipt", async () => {
    bridge.commit.mockResolvedValue({ ...result(["g1"]), status: "IN_PROGRESS" });
    bridge.result.mockResolvedValue(result(["g1"]));
    render(<GovernanceWorkbenchPage />); await chooseFirst(); await preview();
    fireEvent.click(screen.getByRole("button", { name: "保存决定（未批准、未执行）" }));
    await screen.findByText("逐组回执：尚未确认结束");
    expect(screen.getByRole("button", { name: "清空选择" })).toBeDisabled();
    expect(screen.getByText(/已选 1 组/)).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "查询已提交批次回执" }));
    await screen.findByText("逐组回执：已核对终态");
    expect(bridge.result).toHaveBeenCalledWith("synthetic-batch");
    expect(bridge.commit).toHaveBeenCalledTimes(1);
  });
  it("rejects a receipt from another batch and retains the original request", async () => {
    bridge.commit.mockResolvedValue({ ...result(["g1"]), status: "IN_PROGRESS" });
    bridge.result.mockResolvedValue({ ...result(["g1"]), batch_id: "different-batch" });
    render(<GovernanceWorkbenchPage />); await chooseFirst(); await preview();
    fireEvent.click(screen.getByRole("button", { name: "保存决定（未批准、未执行）" }));
    await screen.findByText("逐组回执：尚未确认结束");
    fireEvent.click(screen.getByRole("button", { name: "查询已提交批次回执" }));
    await screen.findByText(/尚未取得可信回执/);
    expect(screen.queryByText("逐组回执：已核对终态")).toBeNull();
    expect(screen.getByRole("button", { name: "清空选择" })).toBeDisabled();
  });
  it("preserves an unresolved request across leaving and returning to the route", async () => {
    bridge.commit.mockRejectedValueOnce(new Error("ambiguous response"));
    bridge.result.mockResolvedValue(result(["g1"]));
    const first = render(<GovernanceWorkbenchPage />); await chooseFirst(); await preview();
    fireEvent.click(screen.getByRole("button", { name: "保存决定（未批准、未执行）" }));
    await screen.findByRole("button", { name: "查询已提交批次回执" });
    first.unmount();
    render(<GovernanceWorkbenchPage />);
    fireEvent.click(await screen.findByRole("button", { name: "查询已提交批次回执" }));
    await screen.findByText("逐组回执：已核对终态");
    expect(bridge.commit).toHaveBeenCalledTimes(1);
    expect(bridge.result).toHaveBeenCalledWith("key:" + bridge.commit.mock.calls[0][1]);
  });
  it("does not duplicate an in-flight mutation after route remount", async () => {
    const pending = deferred<BatchResult>(); bridge.commit.mockReturnValue(pending.promise);
    const first = render(<GovernanceWorkbenchPage />); await chooseFirst(); await preview();
    fireEvent.click(screen.getByRole("button", { name: "保存决定（未批准、未执行）" }));
    first.unmount(); render(<GovernanceWorkbenchPage />);
    const retry = await screen.findByRole("button", { name: "重试相同请求" });
    expect(retry).toBeDisabled(); fireEvent.click(retry);
    expect(bridge.commit).toHaveBeenCalledTimes(1);
    pending.resolve(result(["g1"]));
    bridge.result.mockResolvedValue(result(["g1"]));
    fireEvent.click(screen.getByRole("button", { name: "查询已提交批次回执" }));
    await screen.findByText("逐组回执：已核对终态");
    expect(bridge.commit).toHaveBeenCalledTimes(1);
  });
  it("shows the exact escaped note in confirmation before committing", async () => {
    render(<GovernanceWorkbenchPage />); await chooseFirst();
    fireEvent.change(screen.getByLabelText("备注（可选，不写入普通日志）"), { target: { value: "核对 <synthetic> 用途" } });
    await preview();
    expect(screen.getByRole("dialog")).toHaveTextContent("将记录的备注：核对 <synthetic> 用途");
    expect(document.querySelector("synthetic")).toBeNull();
  });
  it("prevents double clicks from submitting two mutations", async () => {
    const pending = deferred<BatchResult>(); bridge.commit.mockReturnValue(pending.promise);
    render(<GovernanceWorkbenchPage />); await chooseFirst(); await preview();
    const button = screen.getByRole("button", { name: "保存决定（未批准、未执行）" });
    fireEvent.click(button); fireEvent.click(button);
    expect(bridge.commit).toHaveBeenCalledTimes(1);
    pending.resolve(result(["g1"])); await screen.findByRole("region", { name: "批次回执" });
  });
  it("does not declare success on a malformed or foreign outcome receipt", async () => {
    const invalid = result(["g1"]); invalid.items[0].group_id = "foreign"; bridge.commit.mockResolvedValue(invalid);
    render(<GovernanceWorkbenchPage />); await chooseFirst(); await preview();
    fireEvent.click(screen.getByRole("button", { name: "保存决定（未批准、未执行）" }));
    await screen.findByRole("button", { name: "重试相同请求" });
    expect(screen.queryByRole("region", { name: "批次回执" })).not.toBeInTheDocument();
    expect(screen.getByText(/已选 1 组/)).toBeVisible();
  });
  it("does not reinterpret a confirmed receipt as ambiguous when list refresh fails", async () => {
    render(<GovernanceWorkbenchPage />); await chooseFirst(); await preview();
    bridge.list.mockRejectedValue(new Error("private-refresh-marker"));
    fireEvent.click(screen.getByRole("button", { name: "保存决定（未批准、未执行）" }));
    await screen.findByRole("region", { name: "批次回执" });
    expect(await screen.findByText(/决定回执已确认，但列表更新失败/)).toBeVisible();
    expect(screen.queryByRole("button", { name: "重试相同请求" })).not.toBeInTheDocument();
  });
  it("retains old selected versions after a scan revision changes", async () => {
    const page = render(<GovernanceWorkbenchPage />); await chooseFirst();
    context.dataRevision = 1; page.rerender(<GovernanceWorkbenchPage />); await ready();
    expect(screen.getByRole("alert")).toHaveTextContent("1 个选择来自旧扫描结果");
    await preview(); expect(bridge.preview.mock.calls[0][0].items[0].subject_version).toBe("subject-v1");
  });
  it("ignores a preview that resolves after a data revision and releases the UI busy state", async () => {
    const pending = deferred<unknown>(); bridge.preview.mockReturnValue(pending.promise);
    const page = render(<GovernanceWorkbenchPage />); await chooseFirst();
    fireEvent.click(screen.getByRole("button", { name: "预览这一组决定" }));
    context.dataRevision = 1; page.rerender(<GovernanceWorkbenchPage />); await ready();
    pending.resolve({});
    await waitFor(() => expect(screen.getByRole("button", { name: "预览这一组决定" })).toBeEnabled());
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
  it("keeps read-only and recovery-lock recording disabled despite permissive stale UI data", async () => {
    context.isReadWrite = false; context.capabilities = { project_open: true, can_edit_reviews: false, recovery_lock_active: true };
    render(<GovernanceWorkbenchPage />); await chooseFirst(); await preview();
    expect(screen.getByRole("button", { name: "保存决定（未批准、未执行）" })).toBeDisabled();
    expect(bridge.commit).not.toHaveBeenCalled();
  });
  it("uses read-only fallback when the safe high-level contract is absent", async () => {
    bridge.available.mockReturnValue(false); render(<GovernanceWorkbenchPage />); await ready();
    fireEvent.click(screen.getByRole("checkbox", { name: "选择本页第 1 组" }));
    expect(screen.getByRole("button", { name: "预览这一组决定" })).toBeDisabled();
    expect(bridge.preview).not.toHaveBeenCalled(); expect(bridge.commit).not.toHaveBeenCalled();
  });
  it("does not leak marker names, paths or previously entered search/reason in privacy mode", async () => {
    const page = render(<GovernanceWorkbenchPage />); await chooseFirst();
    fireEvent.change(screen.getByLabelText("文件名搜索"), { target: { value: "PRIVACY_BUSINESS_MARKER" } });
    fireEvent.change(screen.getByLabelText("备注（可选，不写入普通日志）"), { target: { value: "PRIVACY_BUSINESS_MARKER" } });
    context.pathPrivacyMode = true; page.rerender(<GovernanceWorkbenchPage />);
    expect(document.body.innerHTML).not.toContain("PRIVACY_BUSINESS_MARKER");
    expect(document.body.innerHTML).not.toContain("/synthetic-fixture");
    expect(screen.getByRole("button", { name: "预览这一组决定" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "清除隐藏备注" }));
    expect(screen.getByRole("button", { name: "预览这一组决定" })).toBeEnabled();
  });
  it("does not submit invalid file-size filters", async () => {
    render(<GovernanceWorkbenchPage />); await ready(); const calls = bridge.list.mock.calls.length;
    fireEvent.change(screen.getByLabelText("最小单文件（MiB）"), { target: { value: "20" } });
    fireEvent.change(screen.getByLabelText("最大单文件（MiB）"), { target: { value: "10" } });
    fireEvent.click(screen.getByRole("button", { name: "应用筛选" }));
    expect(screen.getByText(/最大值不能小于最小值/)).toBeVisible(); expect(bridge.list).toHaveBeenCalledTimes(calls);
  });
});
