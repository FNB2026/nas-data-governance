// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

const { apiMock, contextMock, pushToastMock } = vi.hoisted(() => ({
  pushToastMock: vi.fn(),
  contextMock: {
    capabilities: {
      project_open: true,
      can_edit_reviews: true,
      can_approve_plans: true,
      recovery_lock_active: false,
      disabled_reasons: {},
    },
    isReadWrite: true,
    dataRevision: 0,
    pushToast: vi.fn(),
    displayPath: (path: string) => path,
  },
  apiMock: {
    governance: {
      listAll: vi.fn(),
      listDecisions: vi.fn(),
      buildDrafts: vi.fn(),
      saveDrafts: vi.fn(),
      saveDecision: vi.fn(),
      approve: vi.fn(),
    },
  },
}));

contextMock.pushToast = pushToastMock;

vi.mock("../state/ProjectContext", () => ({ useProject: () => contextMock }));
vi.mock("../api/client", () => ({ api: apiMock }));

import GovernanceReviewPage from "./GovernanceReviewPage";

const draftPlan = {
  id: "plan-1",
  group_id: "group-1",
  state: "DRAFT",
  risk: "low",
  size: 1024,
  content_sha256: "abcdef0123456789",
  retain_path: "/source/keep.txt",
  evidence: ["内容哈希一致"],
  actions: [],
};

beforeEach(() => {
  Object.defineProperty(window, "go", { value: {}, configurable: true });
  Object.defineProperty(window, "runtime", { value: {}, configurable: true });
  apiMock.governance.listAll.mockReset().mockResolvedValue([]);
  apiMock.governance.listDecisions.mockReset().mockResolvedValue([]);
  apiMock.governance.buildDrafts.mockReset().mockResolvedValue([draftPlan]);
  apiMock.governance.saveDrafts.mockReset().mockResolvedValue([draftPlan]);
  apiMock.governance.saveDecision.mockReset().mockResolvedValue({
    group_id: "group-1",
    decision_type: "DEFERRED",
    reason: "等待业务确认",
  });
  apiMock.governance.approve.mockReset().mockResolvedValue({ approved: [draftPlan] });
  pushToastMock.mockReset();
});

afterEach(() => {
  cleanup();
  Reflect.deleteProperty(window, "go");
  Reflect.deleteProperty(window, "runtime");
});

describe("GovernanceReviewPage workflow", () => {
  it("previews, persists, and approves a generated draft", async () => {
    render(<GovernanceReviewPage />);

    fireEvent.click(screen.getByRole("button", { name: "生成系统建议" }));
    expect(await screen.findByText("系统建议预览 — 共 1 条；尚未保存、批准或执行")).toBeVisible();
    expect(apiMock.governance.buildDrafts).toHaveBeenCalledWith("");

    fireEvent.click(screen.getByRole("button", { name: "保存系统建议" }));
    await waitFor(() => expect(apiMock.governance.saveDrafts).toHaveBeenCalledWith(""));

    fireEvent.click(screen.getByText("plan-1"));
    const approveButton = await screen.findByRole("button", { name: "批准计划" });
    expect(approveButton).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "保存用户决定" }));
    await waitFor(() => expect(apiMock.governance.saveDecision).toHaveBeenCalled());
    expect(approveButton).toBeEnabled();
    fireEvent.click(approveButton);

    await waitFor(() => {
      expect(apiMock.governance.approve).toHaveBeenCalledWith({ plan_ids: ["plan-1"] });
    });
    expect(pushToastMock).toHaveBeenCalledWith("success", "计划已批准", "plan-1");
  });

  it("persists the selected review decision and trimmed reason", async () => {
    apiMock.governance.listAll.mockResolvedValue([draftPlan]);
    render(<GovernanceReviewPage />);

    fireEvent.click(await screen.findByText("plan-1"));
    const selects = screen.getAllByRole("combobox");
    fireEvent.change(selects[1], { target: { value: "DEFERRED" } });
    fireEvent.change(screen.getByPlaceholderText("说明决策原因…"), {
      target: { value: "  等待业务确认  " },
    });
    fireEvent.click(screen.getByRole("button", { name: "保存用户决定" }));

    await waitFor(() => {
      expect(apiMock.governance.saveDecision).toHaveBeenCalledWith({
        group_id: "group-1",
        decision_type: "DEFERRED",
        reason: "等待业务确认",
      });
    });
  });

  it("shows an approved, user-decided plan as ready for the execution center", async () => {
    const onNavigate = vi.fn();
    apiMock.governance.listAll.mockResolvedValue([{ ...draftPlan, state: "APPROVED" }]);
    apiMock.governance.listDecisions.mockResolvedValue([{
      group_id: "group-1",
      decision_type: "KEEP_ALL",
      reason: "保留业务副本",
    }]);

    render(<GovernanceReviewPage onNavigate={onNavigate} />);

    fireEvent.click(await screen.findByText("plan-1"));
    expect(await screen.findByText("已记录的用户决定")).toBeVisible();
    expect(screen.getByText("已批准。进入执行中心后仍需完成执行前校验。")).toBeVisible();

    fireEvent.click(screen.getByRole("button", { name: "前往执行中心" }));
    expect(onNavigate).toHaveBeenCalledWith("execution-center");
  });
});
