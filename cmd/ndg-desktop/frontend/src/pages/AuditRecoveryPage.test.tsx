// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";

const { apiMock, contextMock } = vi.hoisted(() => ({
  contextMock: {
    capabilities: { project_open: true },
    dataRevision: 0,
    isReadWrite: true,
    pushToast: vi.fn(),
    refreshRecoveryLock: vi.fn(),
  },
  apiMock: {
    audit: { listLogs: vi.fn(), listJournal: vi.fn() },
    recovery: { checkLock: vi.fn() },
  },
}));

vi.mock("../state/ProjectContext", () => ({ useProject: () => contextMock }));
vi.mock("../api/client", () => ({ api: apiMock }));

import AuditRecoveryPage from "./AuditRecoveryPage";

beforeEach(() => {
  Object.defineProperty(window, "go", { value: {}, configurable: true });
  Object.defineProperty(window, "runtime", { value: {}, configurable: true });
  apiMock.audit.listLogs.mockReset().mockResolvedValue([]);
  apiMock.audit.listJournal.mockReset().mockResolvedValue([]);
  apiMock.recovery.checkLock.mockReset().mockResolvedValue({ lock_active: false, executing_count: 0 });
});

afterEach(() => {
  cleanup();
  Reflect.deleteProperty(window, "go");
  Reflect.deleteProperty(window, "runtime");
});

describe("AuditRecoveryPage log & journal states", () => {
  it("shows a loading state while logs and journal are fetching", async () => {
    apiMock.audit.listLogs.mockReturnValue(new Promise(() => {}));
    apiMock.audit.listJournal.mockReturnValue(new Promise(() => {}));
    render(<AuditRecoveryPage />);

    expect(screen.getAllByText("加载中…").length).toBe(2);
  });

  it("shows an error state when log loading fails", async () => {
    apiMock.audit.listLogs.mockRejectedValue(new Error("日志服务不可达"));
    render(<AuditRecoveryPage />);

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("日志服务不可达");
  });

  it("shows empty states when there are no logs or journal entries", async () => {
    render(<AuditRecoveryPage />);

    expect(await screen.findByText("暂无审计日志")).toBeInTheDocument();
    expect(await screen.findByText("暂无 Journal 记录")).toBeInTheDocument();
  });
});