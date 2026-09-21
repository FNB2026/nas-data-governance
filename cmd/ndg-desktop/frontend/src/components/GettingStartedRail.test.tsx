// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";

const { contextMock } = vi.hoisted(() => ({
  contextMock: {
    jobs: [] as Array<{ id: string }>,
    scanProgress: null as null | { processed: number; discovered: number },
    capabilities: {} as Record<string, unknown>,
    dismissGettingStarted: vi.fn(),
  },
}));

vi.mock("../state/ProjectContext", () => ({ useProject: () => contextMock }));

import GettingStartedRail from "./GettingStartedRail";
import { deriveCapabilities } from "../app/capability";

function setCapabilities(opts: { projectOpen: boolean; isReadWrite: boolean }) {
  contextMock.capabilities = deriveCapabilities(opts) as unknown as Record<string, unknown>;
}

beforeEach(() => {
  contextMock.jobs = [];
  contextMock.scanProgress = null;
  contextMock.dismissGettingStarted.mockClear();
  setCapabilities({ projectOpen: true, isReadWrite: true });
});

afterEach(cleanup);

describe("GettingStartedRail", () => {
  it("lists the five workflow steps in order", () => {
    render(<GettingStartedRail onNavigate={vi.fn()} />);

    const labels = screen
      .getAllByText(/^(数据源|扫描|查看重复结果|治理复核|安全隔离)$/)
      .map((node) => node.textContent);
    expect(labels).toEqual(["数据源", "扫描", "查看重复结果", "治理复核", "安全隔离"]);
  });

  it("marks a step done only when there is a real signal", () => {
    render(<GettingStartedRail onNavigate={vi.fn()} />);

    const steps = screen.getAllByRole("listitem");
    // Data source is done (project open); scan is not (no job yet).
    expect(steps[0]).toHaveClass("getting-started-step--done");
    expect(steps[1]).not.toHaveClass("getting-started-step--done");
  });

  it("marks the scan step done once a job exists", () => {
    contextMock.jobs = [{ id: "job-1" }];
    render(<GettingStartedRail onNavigate={vi.fn()} />);

    const steps = screen.getAllByRole("listitem");
    expect(steps[1]).toHaveClass("getting-started-step--done");
  });

  it("navigates to the requested route", () => {
    const onNavigate = vi.fn();
    render(<GettingStartedRail onNavigate={onNavigate} />);

    const buttons = screen.getAllByRole("button", { name: "前往" });
    buttons[3].click();

    expect(onNavigate).toHaveBeenCalledWith("governance-review");
  });

  it("disables steps the capability model blocks", () => {
    setCapabilities({ projectOpen: true, isReadWrite: false });
    render(<GettingStartedRail onNavigate={vi.fn()} />);

    const buttons = screen.getAllByRole("button", { name: "前往" });
    // Execution center requires read-write mode.
    expect(buttons[4]).toBeDisabled();
    expect(buttons[4]).toHaveAttribute("title", "只读模式，无法执行写操作");
  });

  it("can be dismissed", () => {
    render(<GettingStartedRail onNavigate={vi.fn()} />);

    screen.getByRole("button", { name: "知道了" }).click();

    expect(contextMock.dismissGettingStarted).toHaveBeenCalledTimes(1);
  });
});
