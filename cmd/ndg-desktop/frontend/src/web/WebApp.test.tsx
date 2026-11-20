// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { WebApp } from "./WebApp";
import { HttpAdapter, WebRequestError, type Snapshot } from "./http";
afterEach(() => { cleanup(); vi.restoreAllMocks(); vi.useRealTimers(); });
function snapshot(n: number): Snapshot { return { state: "COMPLETED", stage: "", discovered: 12, processed: 12, failed: 0, checkpoint: "", scanned: 12, active: 12, missing: 0, unavailable: 0, quick: 12, full: 12, groups: Array.from({ length: n }, (_, i) => ({ id: `group_${i}`, copies: 2, bytes: 100 })) }; }
function api() { const a = new HttpAdapter(); vi.spyOn(a, "session").mockResolvedValue(); vi.spyOn(a, "sources").mockResolvedValue([{ alias: "source_a" }, { alias: "source_b" }]); return a; }
it("discards an obsolete source response after a selection change", async () => {
 const a = api(); let resolveA!: (v: Snapshot) => void;
 vi.spyOn(a, "status").mockImplementation(alias => alias === "source_a" ? new Promise(resolve => { resolveA = resolve; }) : Promise.resolve(snapshot(2)));
 render(<WebApp api={a} />); const select = await screen.findByLabelText("已授权来源");
 await waitFor(() => expect(a.status).toHaveBeenCalledWith("source_a", expect.any(AbortSignal)));
 fireEvent.change(select, { target: { value: "source_b" } }); await screen.findByText("重复组 · 2 组");
 await act(async () => resolveA(snapshot(9)));
 expect(screen.queryByText("重复组 · 9 组")).not.toBeInTheDocument(); expect(screen.getByText("重复组 · 2 组")).toBeInTheDocument();
});
it("clears prior results and returns to login on session expiry", async () => {
 const a = api(); vi.spyOn(a, "status").mockResolvedValueOnce(snapshot(1)).mockRejectedValue(new WebRequestError(401));
 render(<WebApp api={a} />); await screen.findByText("重复组 · 1 组"); fireEvent.click(screen.getByRole("button", { name: "刷新状态" }));
 await screen.findByRole("heading", { name: "登录 NDG" }); expect(screen.queryByText("重复组 · 1 组")).not.toBeInTheDocument(); expect(screen.getByRole("alert")).toHaveTextContent("会话已失效");
});
it("does not keep an idle session alive by polling completed scans", async () => {
 vi.useFakeTimers(); const a = api(); vi.spyOn(a, "status").mockResolvedValue(snapshot(0));
 await act(async () => { render(<WebApp api={a} />); }); expect(a.status).toHaveBeenCalledTimes(1);
 await act(async () => { vi.advanceTimersByTime(60_000); }); expect(a.status).toHaveBeenCalledTimes(1);
});
it("does not replay a refused scan or present it as accepted", async () => {
 const a = api(); vi.spyOn(a, "status").mockResolvedValue(snapshot(0)); vi.spyOn(a, "scan").mockRejectedValue(new WebRequestError(409));
 render(<WebApp api={a} />); await screen.findByText("扫描完成"); fireEvent.click(screen.getByRole("button", { name: "开始扫描" }));
 await screen.findByRole("alert"); expect(a.scan).toHaveBeenCalledTimes(1); expect(screen.queryByText("扫描请求已接受，请等待实际扫描结果。")).not.toBeInTheDocument();
});
// 未开放的能力必须"看得见的未启用"，不能给一个貌似能执行的入口。
// 这条断言读的是真实渲染出的控件集合，而不是常量：
// 任何人新增一个治理写操作按钮（"隔离文件""恢复""永久清理"…），
// 都会落到 allowlist 之外或命中 forbidden 列表，从而失败。
it("offers only read-only controls and states the unopened capabilities", async () => {
 const a = api(); vi.spyOn(a, "status").mockResolvedValue(snapshot(2));
 render(<WebApp api={a} />); await screen.findByText("重复组 · 2 组");
 expect(screen.getByText(/隔离、恢复与永久删除尚未开放/)).toBeInTheDocument();
 const sorted = (values: string[]) => [...values].sort();
 const allowed = ["从断点继续", "停止扫描", "刷新状态", "开始扫描", "退出登录"];
 const rendered = screen.getAllByRole("button").map(b => (b.textContent ?? "").trim());
 expect(sorted(rendered)).toEqual(sorted(allowed));
 for (const forbidden of [/隔离/, /恢复/, /永久/, /删除/, /清理/, /批准/, /执行/, /移动/]) {
  expect(screen.queryByRole("button", { name: forbidden })).not.toBeInTheDocument();
 }
});
