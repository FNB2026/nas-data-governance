// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { maskPath } from "../state/settings";

const { apiMock, contextMock, projectState } = vi.hoisted(() => ({
  projectState: { pathPrivacyMode: false },
  contextMock: {
    capabilities: { project_open: true },
    dataRevision: 0,
    isReadWrite: true,
    pushToast: vi.fn(),
    refreshRecoveryLock: vi.fn(),
    displayPath: (path: string) => path,
  },
  apiMock: {
    audit: { listLogs: vi.fn(), listJournal: vi.fn() },
    recovery: { checkLock: vi.fn() },
  },
}));

vi.mock("../state/ProjectContext", () => ({ useProject: () => contextMock }));
vi.mock("../api/client", () => ({ api: apiMock }));

import AuditRecoveryPage, { computeSummary, recoverySteps } from "./AuditRecoveryPage";

// displayPath is the page's single privacy choke point; mirror it here.
contextMock.displayPath = (path: string) =>
  projectState.pathPrivacyMode ? maskPath(path) : path;

beforeEach(() => {
  Object.defineProperty(window, "go", { value: {}, configurable: true });
  Object.defineProperty(window, "runtime", { value: {}, configurable: true });
  projectState.pathPrivacyMode = false;
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

describe("AuditRecoveryPage readable log detail", () => {
  const detailLog = (detail: Record<string, unknown>) => ({
    id: 1,
    plan_id: "p1",
    event_type: "action_completed",
    created_at: "2026-09-01T08:00:00Z",
    detail,
  });

  it("renders structured detail rows with labels and badges", async () => {
    apiMock.audit.listLogs.mockResolvedValue([
      detailLog({ status: "failed", final_state: "ROLLED_BACK" }),
    ]);
    render(<AuditRecoveryPage />);

    expect(await screen.findByText("状态")).toBeInTheDocument();
    expect(screen.getByText("最终状态")).toBeInTheDocument();
    expect(screen.getByText("ROLLED_BACK")).toBeInTheDocument();
    expect(screen.getByText("failed").className).toContain("state-badge--failed");
    expect(screen.getByText(/详情（2 项）/)).toBeInTheDocument();
  });

  it("maps known error_type codes and leaves unknown short codes as-is", async () => {
    apiMock.audit.listLogs.mockResolvedValue([
      detailLog({ error_type: "journal_begin_failed" }),
      detailLog({ error_type: "some_unknown_code" }),
    ]);
    render(<AuditRecoveryPage />);

    expect(await screen.findByText("执行日志登记失败")).toBeInTheDocument();
    expect(screen.getByText("some_unknown_code")).toBeInTheDocument();
  });

  it("renders unknown detail keys and keeps them in the collapsed fallback", async () => {
    apiMock.audit.listLogs.mockResolvedValue([
      detailLog({ weird_key: "x", ok: true }),
    ]);
    render(<AuditRecoveryPage />);

    expect(await screen.findByText("weird_key")).toBeInTheDocument();
    expect(screen.getByText("x")).toBeInTheDocument();
    expect(screen.getByText("true")).toBeInTheDocument();
    expect(screen.getByText(/详情（2 项）/)).toBeInTheDocument();
  });
});

describe("AuditRecoveryPage privacy masking", () => {
  const journalEntry = (overrides: Record<string, unknown> = {}) => ({
    plan_id: "p1",
    task_id: "t1",
    action_index: 1,
    action_type: "QUARANTINE",
    source_path: "/data/private/archive/report.pdf",
    target_path: "",
    content_sha256: "abc123",
    file_size: 1024,
    status: "failed",
    rollback_status: "done",
    started_at: "2026-09-01T08:00:00Z",
    completed_at: "2026-09-01T08:00:01Z",
    ...overrides,
  });

  it("masks journal paths in privacy mode", async () => {
    projectState.pathPrivacyMode = true;
    apiMock.audit.listJournal.mockResolvedValue([journalEntry()]);
    render(<AuditRecoveryPage />);

    expect(await screen.findByText("…/[hidden]/r***t.pdf")).toBeInTheDocument();
    expect(screen.queryByText("/data/private/archive/report.pdf")).not.toBeInTheDocument();
  });

  it("shows raw journal paths outside privacy mode", async () => {
    apiMock.audit.listJournal.mockResolvedValue([journalEntry()]);
    render(<AuditRecoveryPage />);

    expect(await screen.findByText("/data/private/archive/report.pdf")).toBeInTheDocument();
  });

  it("masks deeply nested paths inside the collapsible detail", async () => {
    projectState.pathPrivacyMode = true;
    apiMock.audit.listLogs.mockResolvedValue([
      { id: 1, plan_id: "p1", event_type: "action_completed", created_at: "2026-09-01T08:00:00Z", detail: { items: [{ source_path: "/data/private/a/b.txt" }] } },
    ]);
    render(<AuditRecoveryPage />);

    fireEvent.click(await screen.findByText(/详情（1 项）/));
    const pre = document.querySelector(".audit-detail pre");
    expect(pre?.textContent ?? "").not.toContain("/data/private/a/b.txt");
    expect(pre?.textContent ?? "").toContain("[hidden]");
  });
});

describe("computeSummary & recoverySteps", () => {
  const log = (n: number) =>
    ({ id: n, plan_id: "p", event_type: "x", created_at: "" } as unknown as Parameters<typeof computeSummary>[0][number]);

  const entry = (overrides: Record<string, unknown>) => ({
    plan_id: "p",
    task_id: "t",
    action_index: 1,
    action_type: "MOVE",
    source_path: "/a",
    target_path: "",
    content_sha256: "c",
    file_size: 1,
    status: "done",
    rollback_status: "",
    started_at: "",
    completed_at: "",
    ...overrides,
  } as unknown as Parameters<typeof computeSummary>[1][number]);

  const status = {
    lock_active: true,
    executing_count: 2,
    source_executing_count: 1,
    restore_pending_count: 3,
    purge_recoverable_count: 2,
  } as unknown as Parameters<typeof computeSummary>[2];

  it("counts failures and rollbacks from the journal only", () => {
    const summary = computeSummary(
      [log(1), log(2), log(3)],
      [
        entry({ status: "failed", rollback_status: "done" }),
        entry({ status: "failed" }),
        entry({ status: "done", rollback_status: "done" }),
        entry({ status: "done" }),
      ],
      status,
    );
    expect(summary.logCount).toBe(3);
    expect(summary.journalCount).toBe(4);
    expect(summary.failedCount).toBe(2);
    expect(summary.rollbackCount).toBe(2);
    expect(summary.restorePending).toBe(3);
    expect(summary.purgeRecoverable).toBe(2);
  });

  it("derives step readiness from counts and filled roots", () => {
    const steps1 = recoverySteps(status, "", "");
    expect(steps1[0].ready).toBe(true);
    expect(steps1[1].ready).toBe(false);
    expect(steps1[2].ready).toBe(false);

    const steps2 = recoverySteps(status, "/q", "/s");
    expect(steps2[1].ready).toBe(true);
    expect(steps2[2].ready).toBe(true);

    const steps3 = recoverySteps(status, "/q", "");
    expect(steps3[1].ready).toBe(false);
    expect(steps3[2].ready).toBe(true);

    expect(recoverySteps(null, "/q", "/s")).toEqual([]);
  });
});

describe("AuditRecoveryPage summary & guidance", () => {
  const journalEntry = (overrides: Record<string, unknown> = {}) => ({
    plan_id: "p1",
    task_id: "t1",
    action_index: 1,
    action_type: "MOVE",
    source_path: "/data/source/a.pdf",
    target_path: "",
    content_sha256: "c",
    file_size: 1,
    status: "done",
    rollback_status: "",
    started_at: "",
    completed_at: "",
    ...overrides,
  });

  it("renders summary cards and recovery guidance when the lock is active", async () => {
    apiMock.audit.listLogs.mockResolvedValue(
      [1, 2, 3, 4, 5, 6].map((id) => ({
        id,
        plan_id: `p${id}`,
        event_type: "action_completed",
        created_at: "2026-09-01T08:00:00Z",
        detail: { status: "done" },
      })),
    );
    apiMock.audit.listJournal.mockResolvedValue([
      journalEntry({ status: "failed", rollback_status: "done" }),
      journalEntry({ status: "failed", rollback_status: "done" }),
      journalEntry({ status: "done", rollback_status: "done" }),
      journalEntry({ status: "done", rollback_status: "" }),
    ]);
    apiMock.recovery.checkLock.mockResolvedValue({
      lock_active: true,
      executing_count: 2,
      source_executing_count: 1,
      restore_pending_count: 0,
      purge_recoverable_count: 5,
    });
    const { container } = render(<AuditRecoveryPage />);

    const grid = await screen.findByLabelText("审计与恢复概览");
    expect(grid).toHaveTextContent("6");
    expect(grid).toHaveTextContent("4");
    expect(grid).toHaveTextContent("2");
    expect(grid).toHaveTextContent("3");
    expect(grid).toHaveTextContent("0");
    expect(grid).toHaveTextContent("5");
    expect(container.querySelector(".ek-summary-card--danger")).not.toBeNull();

    expect(screen.getByLabelText("恢复指引")).toBeInTheDocument();
    const guidance = within(screen.getByLabelText("恢复指引"));
    guidance.getByText("恢复普通执行");
    guidance.getByText("恢复隔离还原");
    guidance.getByText("恢复永久清理");
    expect(guidance.getByText("1 步待处理")).toBeInTheDocument();
  });

  it("hides recovery guidance when the lock is inactive", async () => {
    render(<AuditRecoveryPage />);

    expect(await screen.findByLabelText("审计与恢复概览")).toBeInTheDocument();
    expect(screen.queryByLabelText("恢复指引")).not.toBeInTheDocument();
  });
});