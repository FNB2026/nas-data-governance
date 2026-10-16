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
