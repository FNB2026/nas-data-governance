// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

const { apiMock, contextMock, pushToastMock } = vi.hoisted(() => ({
  pushToastMock: vi.fn(),
  contextMock: {
    capabilities: {
      project_open: true,
      can_execute_quarantine: true,
      recovery_lock_active: false,
    },
    isReadWrite: true,
    dataRevision: 0,
    pushToast: vi.fn(),
  },
  apiMock: {
    governance: { listAll: vi.fn() },
    execution: {
      executePlans: vi.fn(),
      listQuarantine: vi.fn(),
      listRestores: vi.fn(),
      listPurges: vi.fn(),
      createRestorePlan: vi.fn(),
      approveRestore: vi.fn(),
      executeRestore: vi.fn(),
      createPurgePlans: vi.fn(),
      approvePurge: vi.fn(),
      executePurge: vi.fn(),
    },
    recovery: {
      checkLock: vi.fn(),
      recoverSource: vi.fn(),
      recoverRestores: vi.fn(),
      recoverPurges: vi.fn(),
    },
  },
}));

contextMock.pushToast = pushToastMock;

vi.mock("../state/ProjectContext", () => ({ useProject: () => contextMock }));
vi.mock("../api/client", () => ({ api: apiMock }));

import ExecutionCenterPage from "./ExecutionCenterPage";

const approvedPlan = {
  id: "plan-approved",
  group_id: "group-1",
  state: "APPROVED",
  risk: "low",
  size: 2048,
  content_sha256: "abcdef0123456789",
  actions: [],
};

const successfulResult = {
  results: [{ plan_id: "plan-approved", final_state: "VERIFIED", steps: [] }],
  executed: 1,
  skipped: 0,
  failed: 0,
};

beforeEach(() => {
  Object.defineProperty(window, "go", { value: {}, configurable: true });
  Object.defineProperty(window, "runtime", { value: {}, configurable: true });
  contextMock.capabilities.can_execute_quarantine = true;
  contextMock.capabilities.recovery_lock_active = false;
  apiMock.governance.listAll.mockReset().mockResolvedValue([approvedPlan]);
  apiMock.execution.listQuarantine.mockReset().mockResolvedValue([]);
  apiMock.execution.listRestores.mockReset().mockResolvedValue([]);
  apiMock.execution.listPurges.mockReset().mockResolvedValue([]);
  apiMock.execution.executePlans.mockReset().mockResolvedValue(successfulResult);
  apiMock.recovery.checkLock.mockReset().mockResolvedValue({ lock_active: false, executing_count: 0 });
  pushToastMock.mockReset();
});

afterEach(() => {
  cleanup();
  Reflect.deleteProperty(window, "go");
  Reflect.deleteProperty(window, "runtime");
});

async function selectPlanAndFillRoots() {
  expect(await screen.findByText("plan-approved")).toBeVisible();
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.change(screen.getByPlaceholderText("隔离根目录"), { target: { value: "  /quarantine  " } });
  fireEvent.change(screen.getByPlaceholderText("源根目录（每行一个）"), {
    target: { value: " /source-a\n\n/source-b " },
  });
}

describe("ExecutionCenterPage plan execution", () => {
  it("requires a successful dry-run before real execution", async () => {
    render(<ExecutionCenterPage />);
    await selectPlanAndFillRoots();

    const executeButton = screen.getByRole("button", { name: "执行选中计划" });
    expect(executeButton).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "试运行" }));

    await waitFor(() => {
      expect(apiMock.execution.executePlans).toHaveBeenNthCalledWith(1, {
        plan_ids: ["plan-approved"],
        quarantine_root: "/quarantine",
        source_roots: ["/source-a", "/source-b"],
        dry_run: true,
        retention_hours: 720,
      });
    });
    expect(await screen.findByText("试运行已通过")).toBeVisible();
    expect(executeButton).toBeEnabled();

    fireEvent.click(executeButton);
    await waitFor(() => {
      expect(apiMock.execution.executePlans).toHaveBeenNthCalledWith(2, expect.objectContaining({
        plan_ids: ["plan-approved"],
        dry_run: false,
      }));
    });
  });

  it("rejects missing roots without calling the backend", async () => {
    render(<ExecutionCenterPage />);
    expect(await screen.findByText("plan-approved")).toBeVisible();
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(screen.getByRole("button", { name: "试运行" }));

    expect(pushToastMock).toHaveBeenCalledWith("error", "缺少隔离根目录", "请填写隔离根目录");
    expect(apiMock.execution.executePlans).not.toHaveBeenCalled();
  });

  it("keeps new plan execution disabled while the recovery lock is active", async () => {
    contextMock.capabilities.can_execute_quarantine = false;
    contextMock.capabilities.recovery_lock_active = true;
    apiMock.recovery.checkLock.mockResolvedValue({ lock_active: true, executing_count: 1 });
    render(<ExecutionCenterPage />);

    expect(await screen.findByRole("alert")).toHaveTextContent("恢复锁激活");
    fireEvent.click(screen.getByRole("checkbox"));
    expect(screen.getByRole("button", { name: "试运行" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "执行选中计划" })).toBeDisabled();
  });
});
