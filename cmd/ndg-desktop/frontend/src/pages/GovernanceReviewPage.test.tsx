// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

const { apiMock, contextMock, pushToastMock } = vi.hoisted(() => ({
  pushToastMock: vi.fn(),
  contextMock: {
    capabilities: { project_open: true },
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

    fireEvent.click(screen.getByRole("button", { name: "生成草案" }));
    expect(await screen.findByText("草案预览 — 共 1 条（点击「保存到数据库」持久化）")).toBeVisible();
    expect(apiMock.governance.buildDrafts).toHaveBeenCalledWith("");

    fireEvent.click(screen.getByRole("button", { name: "保存到数据库" }));
    await waitFor(() => expect(apiMock.governance.saveDrafts).toHaveBeenCalledWith(""));

    fireEvent.click(screen.getByText("plan-1"));
    fireEvent.click(await screen.findByRole("button", { name: "批准计划" }));

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
    fireEvent.click(screen.getByRole("button", { name: "保存决策" }));

    await waitFor(() => {
      expect(apiMock.governance.saveDecision).toHaveBeenCalledWith({
        group_id: "group-1",
        decision_type: "DEFERRED",
        reason: "等待业务确认",
      });
    });
  });
});
