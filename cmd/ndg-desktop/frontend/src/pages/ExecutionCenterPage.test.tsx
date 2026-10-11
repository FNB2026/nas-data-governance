// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

const { apiMock, contextMock, pushToastMock } = vi.hoisted(() => ({
  pushToastMock: vi.fn(),
  contextMock: {
    capabilities: {
      project_open: true,
      can_execute_quarantine: true,
      can_execute_purge: true,
      recovery_lock_active: false,
      disabled_reasons: {},
    },
    isReadWrite: true,
    dataRevision: 0,
    pushToast: vi.fn(),
    refreshRecoveryLock: vi.fn(),
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
  contextMock.dataRevision = 0;
  contextMock.isReadWrite = true;
  apiMock.execution.createRestorePlan.mockReset();
  apiMock.execution.approveRestore.mockReset().mockResolvedValue(undefined);
  apiMock.execution.executeRestore.mockReset().mockResolvedValue({ status: "ok", final_state: "APPROVED" });
  contextMock.capabilities.can_execute_quarantine = true;
  contextMock.capabilities.can_execute_purge = true;
  contextMock.capabilities.recovery_lock_active = false;
  apiMock.governance.listAll.mockReset().mockResolvedValue([approvedPlan]);
  apiMock.execution.listQuarantine.mockReset().mockResolvedValue([]);
  apiMock.execution.listRestores.mockReset().mockResolvedValue([]);
  apiMock.execution.listPurges.mockReset().mockResolvedValue([]);
  apiMock.execution.executePlans.mockReset().mockResolvedValue(successfulResult);
  apiMock.recovery.checkLock.mockReset().mockResolvedValue({ lock_active: false, executing_count: 0 });
  pushToastMock.mockReset();
  contextMock.refreshRecoveryLock.mockReset().mockResolvedValue({ lock_active: false, executing_count: 0 });
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

    const executeButton = screen.getByRole("button", { name: "执行隔离" });
    expect(executeButton).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "执行前试运行" }));

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
    fireEvent.click(screen.getByRole("button", { name: "执行前试运行" }));

    expect(pushToastMock).toHaveBeenCalledWith("error", "缺少隔离根目录", "请填写隔离根目录");
    expect(apiMock.execution.executePlans).not.toHaveBeenCalled();
  });

  it("keeps new plan execution disabled while the recovery lock is active", async () => {
    contextMock.capabilities.can_execute_quarantine = false;
    contextMock.capabilities.can_execute_purge = false;
    contextMock.capabilities.recovery_lock_active = true;
    apiMock.recovery.checkLock.mockResolvedValue({ lock_active: true, executing_count: 1 });
    render(<ExecutionCenterPage />);

    expect(await screen.findByRole("alert")).toHaveTextContent("恢复锁激活");
    fireEvent.click(screen.getByRole("checkbox"));
    expect(screen.getByRole("button", { name: "执行前试运行" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "执行隔离" })).toBeDisabled();
  });

  it("keeps purge approval disabled while the recovery lock is active", async () => {
    contextMock.capabilities.can_execute_purge = false;
    contextMock.capabilities.recovery_lock_active = true;
    apiMock.execution.listPurges.mockResolvedValue([{
      id: "purge-draft",
      state: "DRAFT",
      expected_size: 1024,
      expected_sha256: "abcdef0123456789",
      retain_until: "2026-01-01T00:00:00Z",
      approval_digest: "digest",
    }]);

    render(<ExecutionCenterPage />);
    fireEvent.click(await screen.findByRole("button", { name: "清理 (1)" }));

    expect(await screen.findByRole("button", { name: "批准" })).toBeDisabled();
    expect(screen.getByText("永久清理危险区")).toBeVisible();
  });
});


describe("ExecutionCenter recovery feedback", () => {
  it.each([
    ["recoverSource", "恢复源目录执行", [{ action: "skipped", errors: ["PRIVATE_CANARY /private/source"] }]],
    ["recoverRestores", "恢复隔离还原", [{ status: "failed", error: "PRIVATE_CANARY /private/source" }]],
    ["recoverPurges", "恢复清理操作", [{ status: "failed", error_type: "PRIVATE_CANARY" }]],
  ] as const)("classifies %s and refreshes the global lock once", async (method, label, results) => {
    apiMock.recovery[method].mockResolvedValue(results);
    contextMock.refreshRecoveryLock.mockResolvedValue({ lock_active: true, executing_count: 1 });
    render(<ExecutionCenterPage />);
    fireEvent.click(screen.getByRole("button", { name: /^恢复(?: ⚠)?$/ }));
    const recoveryButton = await screen.findByRole("button", { name: label });
    await waitFor(() => expect(recoveryButton).toBeEnabled());
    fireEvent.click(recoveryButton);
    await waitFor(() => expect(pushToastMock).toHaveBeenCalled());
    expect(contextMock.refreshRecoveryLock).toHaveBeenCalledTimes(1);
    expect(apiMock.recovery[method]).toHaveBeenCalledTimes(1);
    expect(pushToastMock.mock.calls.some(([tone]) => tone === "success")).toBe(false);
    expect(document.body.textContent).toContain("人工核对");
    expect(document.body.textContent).not.toContain("PRIVATE_CANARY");
  });
});

describe("ExecutionCenter recovery request and lock uncertainty", () => {
  it.each(["rejected", "unknown", "confirmed"])("handles %s source recovery without a second write request", async (mode) => {
    apiMock.recovery.recoverSource.mockReset().mockResolvedValue([{ action: "rolled_back" }]);
    contextMock.refreshRecoveryLock.mockResolvedValue(mode === "confirmed" ? { lock_active: false, executing_count: 0 } : null);
    if (mode === "rejected") apiMock.recovery.recoverSource.mockRejectedValue(new Error("PRIVATE_CANARY /private/path"));
    render(<ExecutionCenterPage />);
    fireEvent.click(screen.getByRole("button", { name: /^恢复(?: ⚠)?$/ }));
    const button = await screen.findByRole("button", { name: "恢复源目录执行" });
    await waitFor(() => expect(button).toBeEnabled());
    fireEvent.click(button);
    await waitFor(() => expect(pushToastMock).toHaveBeenCalled());
    expect(apiMock.recovery.recoverSource).toHaveBeenCalledTimes(1);
    expect(contextMock.refreshRecoveryLock).toHaveBeenCalledTimes(1);
    expect(pushToastMock.mock.calls.some(([tone]) => tone === "success")).toBe(mode === "confirmed");
    expect(document.body.textContent).not.toContain("PRIVATE_CANARY");
  });
});


describe("Restore after verified rollback", () => {
  it("offers a new draft rather than reusing a rolled-back approval", async () => {
    apiMock.execution.listQuarantine.mockResolvedValue([{
      id: "item-disposable", status: "QUARANTINED", file_size: 1024,
      content_sha256: "abcdef0123456789", quarantined_at: "2026-01-01T00:00:00Z", retain_until: "2026-02-01T00:00:00Z",
    }]);
    apiMock.execution.listRestores.mockResolvedValue([{
      id: "restore-old", item_id: "item-disposable", state: "ROLLED_BACK", approval_digest: "",
    }]);
    apiMock.execution.createRestorePlan.mockImplementation(async () => {
      const plan = { id: "restore-new", item_id: "item-disposable", state: "DRAFT", approval_digest: "new-digest" };
      apiMock.execution.listRestores.mockResolvedValue([plan]);
      return plan;
    });
    apiMock.execution.approveRestore.mockResolvedValue(undefined);
    render(<ExecutionCenterPage />);
    fireEvent.click(await screen.findByRole("button", { name: /隔离与恢复/ }));
    expect(await screen.findByText("item-disposable")).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "创建恢复草案" }));
    fireEvent.click(await screen.findByRole("button", { name: "批准" }));
    await waitFor(() => expect(apiMock.execution.approveRestore).toHaveBeenCalledWith("restore-new", "new-digest"));
  });
});


const restoreItem = { id: "item-safe", status: "QUARANTINED", file_size: 1024, content_sha256: "abcdef", quarantined_at: "2026-01-01T00:00:00Z", retain_until: "2026-02-01T00:00:00Z" };
const rolledRestore = { id: "old-restore", item_id: restoreItem.id, state: "ROLLED_BACK", approval_digest: "" };
const activeRestore = { id: "fresh-restore", item_id: restoreItem.id, state: "DRAFT", approval_digest: "fresh-digest" };
async function renderRestores(plans: object[], status = "QUARANTINED") {
  apiMock.execution.listQuarantine.mockResolvedValue([{ ...restoreItem, status }]);
  apiMock.execution.listRestores.mockResolvedValue(plans);
  const view = render(<ExecutionCenterPage />);
  fireEvent.click(await screen.findByRole("button", { name: /隔离与恢复/ }));
  expect(await screen.findByText(restoreItem.id)).toBeVisible();
  return view;
}

describe("Restore plan selection safety", () => {
  it.each([false, true])("approves only the new draft regardless of list order (%s)", async (reverse) => {
    await renderRestores(reverse ? [activeRestore, rolledRestore] : [rolledRestore, activeRestore]);
    expect(screen.queryByRole("button", { name: "创建恢复草案" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "批准" }));
    await waitFor(() => expect(apiMock.execution.approveRestore).toHaveBeenCalledWith("fresh-restore", "fresh-digest"));
  });

  it.each([false, true])("uses the fresh approval across dry run and reload (%s)", async (reverse) => {
    const approved = { ...activeRestore, state: "APPROVED" };
    await renderRestores(reverse ? [approved, rolledRestore] : [rolledRestore, approved]);
    fireEvent.change(screen.getByPlaceholderText("隔离根目录"), { target: { value: "/quarantine" } });
    fireEvent.change(screen.getByPlaceholderText("源根目录（逗号分隔）"), { target: { value: "/source" } });
    fireEvent.click(screen.getByRole("button", { name: "试运行" }));
    await waitFor(() => expect(apiMock.execution.executeRestore).toHaveBeenCalledWith(expect.objectContaining({ plan_id: "fresh-restore", digest: "fresh-digest", dry_run: true })));
    await waitFor(() => expect(screen.getByRole("button", { name: "执行" })).toBeEnabled());
    fireEvent.click(screen.getByRole("button", { name: "执行" }));
    await waitFor(() => expect(apiMock.execution.executeRestore).toHaveBeenLastCalledWith(expect.objectContaining({ plan_id: "fresh-restore", digest: "fresh-digest", dry_run: false })));
  });

  it.each([
    { name: "conflicting active plans", plans: [activeRestore, { ...activeRestore, id: "other-active", state: "APPROVED" }] },
    { name: "unknown unresolved state", plans: [rolledRestore, { ...activeRestore, state: "UNKNOWN" }] },
  ])("blocks $name", async ({ plans }) => {
    await renderRestores(plans);
    for (const name of ["创建恢复草案", "批准", "试运行", "执行"]) expect(screen.queryByRole("button", { name })).not.toBeInTheDocument();
  });

  it.each(["HOLD", "RESTORED", "PURGED"])("does not create a new plan for %s", async (status) => {
    await renderRestores([rolledRestore], status);
    expect(screen.queryByRole("button", { name: "创建恢复草案" })).not.toBeInTheDocument();
  });

  it("keeps creation disabled under a recovery lock", async () => {
    contextMock.capabilities.can_execute_quarantine = false;
    contextMock.capabilities.recovery_lock_active = true;
    apiMock.recovery.checkLock.mockResolvedValue({ lock_active: true, executing_count: 1 });
    await renderRestores([rolledRestore]);
    expect(screen.getByRole("button", { name: "创建恢复草案" })).toBeDisabled();
  });

  it("does not expose a write action in read-only mode", async () => {
    contextMock.isReadWrite = false;
    await renderRestores([rolledRestore]);
    expect(screen.queryByRole("button", { name: "创建恢复草案" })).not.toBeInTheDocument();
  });

  it("disables repeated creation while a new draft is pending", async () => {
    apiMock.execution.createRestorePlan.mockReturnValue(new Promise(() => {}));
    await renderRestores([rolledRestore]);
    const create = screen.getByRole("button", { name: "创建恢复草案" });
    fireEvent.click(create);
    expect(screen.queryByRole("button", { name: "创建恢复草案" })).not.toBeInTheDocument();
    expect(apiMock.execution.createRestorePlan).toHaveBeenCalledOnce();
  });
});


describe("Restore list trust", () => {
  it("does not treat a pending read as an empty plan list", async () => {
    apiMock.execution.listRestores.mockReturnValue(new Promise(() => {}));
    apiMock.execution.listQuarantine.mockResolvedValue([restoreItem]);
    render(<ExecutionCenterPage />);
    fireEvent.click(await screen.findByRole("button", { name: /隔离与恢复/ }));
    expect(await screen.findByText(restoreItem.id)).toBeVisible();
    const create = screen.queryByRole("button", { name: "创建恢复草案" });
    if (create) expect(create).toBeDisabled();
    expect(apiMock.execution.createRestorePlan).not.toHaveBeenCalled();
  });

  it("blocks writes after failed refresh instead of trusting a rolled-back cache", async () => {
    await renderRestores([rolledRestore]);
    expect(screen.getByRole("button", { name: "创建恢复草案" })).toBeEnabled();
    apiMock.execution.listRestores.mockRejectedValue(new Error("private-test-anchor"));
    fireEvent.click(screen.getByRole("button", { name: "刷新" }));
    await waitFor(() => {
      expect(apiMock.execution.listRestores).toHaveBeenCalledTimes(2);
      const create = screen.queryByRole("button", { name: "创建恢复草案" });
      if (create) expect(create).toBeDisabled();
    });
    expect(screen.queryByText("private-test-anchor")).not.toBeInTheDocument();
    expect(apiMock.execution.createRestorePlan).not.toHaveBeenCalled();
  });
});


describe("Restore refresh races", () => {
  it("does not let an older successful read reopen writes after the newest read fails", async () => {
    let resolveOld!: (plans: object[]) => void;
    apiMock.execution.listRestores.mockReturnValueOnce(new Promise((resolve) => { resolveOld = resolve; }));
    apiMock.execution.listQuarantine.mockResolvedValue([restoreItem]);
    render(<ExecutionCenterPage />);
    fireEvent.click(await screen.findByRole("button", { name: /隔离与恢复/ }));
    expect(await screen.findByText(restoreItem.id)).toBeVisible();
    apiMock.execution.listRestores.mockRejectedValueOnce(new Error("private-read-anchor"));
    fireEvent.click(screen.getByRole("button", { name: "刷新" }));
    await waitFor(() => expect(apiMock.execution.listRestores).toHaveBeenCalledTimes(2));
    await act(async () => resolveOld([rolledRestore]));
    expect(screen.queryByRole("button", { name: "创建恢复草案" })).not.toBeInTheDocument();
    expect(screen.getByText("恢复状态尚未确认，请刷新后人工核对")).toBeVisible();
    expect(screen.queryByText("private-read-anchor")).not.toBeInTheDocument();
  });

  it("keeps an existing approval disabled when its state read fails", async () => {
    await renderRestores([{ ...activeRestore, state: "APPROVED" }]);
    apiMock.execution.listRestores.mockRejectedValueOnce(new Error("private-read-anchor"));
    fireEvent.click(screen.getByRole("button", { name: "刷新" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "执行" })).toBeDisabled());
    expect(screen.getByRole("button", { name: "试运行" })).toBeDisabled();
    expect(apiMock.execution.executeRestore).not.toHaveBeenCalled();
  });
});


describe("Restore read and mutation interleaving", () => {
  it("invalidates a read that straddles draft creation and reloads canonical plans", async () => {
    let finishCreate!: (plan: object) => void;
    let finishOldPurges!: (plans: object[]) => void;
    apiMock.execution.createRestorePlan.mockImplementation(() => new Promise((resolve) => { finishCreate = resolve; }));
    const view = await renderRestores([rolledRestore]);
    fireEvent.click(screen.getByRole("button", { name: "创建恢复草案" }));
    expect(screen.getByRole("button", { name: "刷新" })).toBeDisabled();
    apiMock.execution.listPurges.mockReturnValueOnce(new Promise((resolve) => { finishOldPurges = resolve; }));
    contextMock.dataRevision++;
    view.rerender(<ExecutionCenterPage />);
    await waitFor(() => expect(apiMock.execution.listRestores).toHaveBeenCalledTimes(2));
    apiMock.execution.listRestores.mockResolvedValue([rolledRestore, activeRestore]);
    await act(async () => finishCreate(activeRestore));
    await waitFor(() => expect(apiMock.execution.listRestores).toHaveBeenCalledTimes(3));
    await act(async () => finishOldPurges([]));
    expect(await screen.findByRole("button", { name: "批准" })).toBeEnabled();
    expect(screen.queryByRole("button", { name: "创建恢复草案" })).not.toBeInTheDocument();
  });

  it("never displays raw lifecycle read errors on the purge tab", async () => {
    await renderRestores([rolledRestore]);
    apiMock.execution.listRestores.mockRejectedValueOnce(new Error("PRIVATE_READ_LEAK_MARKER"));
    fireEvent.click(screen.getByRole("button", { name: "刷新" }));
    await waitFor(() => expect(apiMock.execution.listRestores).toHaveBeenCalledTimes(2));
    fireEvent.click(screen.getByRole("button", { name: /清理 \(0\)/ }));
    expect(await screen.findByText("恢复与清理计划读取失败，请刷新后人工核对")).toBeVisible();
    expect(screen.queryByText("PRIVATE_READ_LEAK_MARKER")).not.toBeInTheDocument();
  });
});
