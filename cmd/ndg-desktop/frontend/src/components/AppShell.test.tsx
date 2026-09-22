// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

const { projectState } = vi.hoisted(() => ({
  projectState: {
    project: null as { path: string } | null,
    isReadWrite: false,
    capabilities: {
      project_open: false,
      project_mode: "closed" as "closed" | "read_only" | "read_write",
      can_scan: false,
      can_view_results: false,
      can_edit_reviews: false,
      can_approve_plans: false,
      can_execute_quarantine: false,
      can_execute_purge: false,
      recovery_lock_active: false,
      disabled_reasons: {
        "scan-jobs": "请先打开项目",
        "duplicate-results": "请先打开项目",
        "governance-review": "请先打开项目",
        "execution-center": "请先打开项目",
        "audit-recovery": "请先打开项目",
      },
    },
    activeJobId: null as string | null,
    scanProgress: null as { processed: number; discovered: number } | null,
    connectionStatus: "connected" as "connected" | "reconnecting" | "disconnected",
    displayPath: (path: string) => path,
    refreshProject: vi.fn(),
  },
}));

vi.mock("../state/ProjectContext", () => ({
  useProject: () => projectState,
}));

import AppShell from "./AppShell";

afterEach(() => cleanup());

beforeEach(() => {
  projectState.project = null;
  projectState.isReadWrite = false;
  projectState.activeJobId = null;
  projectState.scanProgress = null;
  projectState.connectionStatus = "connected";
  projectState.capabilities.project_open = false;
  projectState.capabilities.project_mode = "closed";
  projectState.capabilities.recovery_lock_active = false;
  projectState.refreshProject.mockReset();
});

describe("AppShell", () => {
  it("keeps version detail out of the header and explains unavailable navigation", () => {
    render(
      <AppShell activeRoute="sources" onRouteChange={vi.fn()}>
        <div>数据源内容</div>
      </AppShell>,
    );

    expect(screen.getByLabelText("NDG 数据治理")).toBeInTheDocument();
    expect(screen.getByText("尚未打开项目")).toBeInTheDocument();
    expect(screen.queryByText(/v0\.5/)).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /扫描任务：新建扫描、进度与历史/ })).toBeDisabled();
    expect(screen.getByRole("button", { name: /数据源：项目、存储与扫描准备/ })).toHaveAttribute("aria-current", "page");
  });

  it("shows a user-actionable NAS connection status", () => {
    projectState.project = { path: "/Volumes/NAS/NDG.ndg" };
    projectState.isReadWrite = true;
    projectState.capabilities.project_open = true;
    projectState.capabilities.project_mode = "read_write";
    projectState.connectionStatus = "disconnected";

    render(
      <AppShell activeRoute="sources" onRouteChange={vi.fn()}>
        <div>数据源内容</div>
      </AppShell>,
    );

    expect(screen.getByText("NAS 连接中断")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "重新检查" }));
    expect(projectState.refreshProject).toHaveBeenCalledOnce();
  });

  it("prioritizes active scan progress in the header", () => {
    projectState.project = { path: "/Volumes/NAS/NDG.ndg" };
    projectState.isReadWrite = true;
    projectState.capabilities.project_open = true;
    projectState.capabilities.project_mode = "read_write";
    projectState.activeJobId = "job-1";
    projectState.scanProgress = { processed: 42813, discovered: 103211 };

    render(
      <AppShell activeRoute="sources" onRouteChange={vi.fn()}>
        <div>数据源内容</div>
      </AppShell>,
    );

    expect(screen.getByText("扫描中 · 已处理 42,813 / 103,211")).toBeInTheDocument();
  });
});

describe("AppShell accessibility contract (UI-P8-B)", () => {
  function renderShell() {
    return render(
      <AppShell activeRoute="sources" onRouteChange={vi.fn()}>
        <div>数据源内容</div>
      </AppShell>,
    );
  }

  it("offers a skip link that targets the main region", () => {
    const { container } = renderShell();

    const skipLink = screen.getByRole("link", { name: "跳到主要内容" });
    expect(skipLink).toHaveAttribute("href", "#main-content");

    const main = container.querySelector("main");
    expect(main).toHaveAttribute("id", "main-content");
    // Programmatic focus target for the skip link.
    expect(main).toHaveAttribute("tabindex", "-1");
  });

  it("labels the primary navigation landmark", () => {
    renderShell();

    expect(screen.getByRole("navigation", { name: "主导航" })).toBeInTheDocument();
  });

  it("never expresses a disabled destination by colour alone", () => {
    renderShell();

    const blocked = screen.getByRole("button", { name: /扫描任务：新建扫描、进度与历史/ });
    // Native disabled attribute (removed from the tab order) …
    expect(blocked).toBeDisabled();
    // … plus the reason carried in the accessible name, not only in a tooltip.
    expect(blocked).toHaveAccessibleName(/请先打开项目/);
  });

  it("marks the current page for assistive tech, not only visually", () => {
    renderShell();

    const current = screen.getByRole("button", { name: /数据源：项目、存储与扫描准备/ });
    expect(current).toHaveAttribute("aria-current", "page");
    expect(current).not.toBeDisabled();
  });
});
