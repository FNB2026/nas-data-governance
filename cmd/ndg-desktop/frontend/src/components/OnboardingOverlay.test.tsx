// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import OnboardingOverlay from "./OnboardingOverlay";

afterEach(cleanup);

function setup(overrides: Partial<Parameters<typeof OnboardingOverlay>[0]> = {}) {
  const props = {
    busy: false,
    onPickDirectory: vi.fn(async () => "/Volumes/Archive/Photos"),
    onCreateProject: vi.fn(),
    onUseExisting: vi.fn(),
    onDismiss: vi.fn(),
    error: null,
    ...overrides,
  };
  render(<OnboardingOverlay {...props} />);
  return props;
}

describe("OnboardingOverlay first-run content", () => {
  it("explains exactly the three first-run questions", () => {
    setup();

    expect(screen.getByText("NDG 是什么")).toBeInTheDocument();
    expect(screen.getByText("扫描不会修改源文件")).toBeInTheDocument();
    expect(screen.getByText("建议第一次选择较小的目录")).toBeInTheDocument();
  });

  it("exposes the primary and secondary actions", () => {
    setup();

    expect(screen.getByRole("button", { name: "选择数据目录" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "打开已有项目" })).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /了解 NDG 的安全机制/ }),
    ).toBeInTheDocument();
  });

  it("keeps implementation vocabulary out of the first-run copy", () => {
    setup();

    const text = document.body.textContent || "";
    for (const term of ["SHA-256", "inode", "Journal", "stale", "hardlink"]) {
      expect(text).not.toContain(term);
    }
  });

  it("hides the safety explanation until the user asks for it", () => {
    setup();

    expect(screen.queryByText("扫描只读")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /了解 NDG 的安全机制/ }));

    expect(screen.getByText("扫描只读")).toBeInTheDocument();
    expect(screen.getByText("隔离而非删除")).toBeInTheDocument();
    expect(screen.getByText("恢复锁")).toBeInTheDocument();
    expect(screen.getByText("本地优先")).toBeInTheDocument();
    expect(screen.getByText("由你决定")).toBeInTheDocument();
  });
});

describe("OnboardingOverlay actions", () => {
  it("creates the project from the picked directory", async () => {
    const props = setup();

    fireEvent.click(screen.getByRole("button", { name: "选择数据目录" }));

    // The async handler resolves on the next microtask.
    await vi.waitFor(() => {
      expect(props.onCreateProject).toHaveBeenCalledWith("/Volumes/Archive/Photos");
    });
    expect(props.onPickDirectory).toHaveBeenCalledWith();
    expect(props.onDismiss).not.toHaveBeenCalled();
  });

  it("hands control back to the start card when picking is cancelled", async () => {
    const props = setup({ onPickDirectory: vi.fn(async () => "") });

    fireEvent.click(screen.getByRole("button", { name: "选择数据目录" }));

    await vi.waitFor(() => {
      expect(props.onDismiss).toHaveBeenCalledTimes(1);
    });
    expect(props.onCreateProject).not.toHaveBeenCalled();
  });

  it("leaves onboarding for an existing project", () => {
    const props = setup();

    fireEvent.click(screen.getByRole("button", { name: "打开已有项目" }));

    expect(props.onUseExisting).toHaveBeenCalledTimes(1);
    expect(props.onCreateProject).not.toHaveBeenCalled();
  });

  it("can be dismissed from the close button", () => {
    const props = setup();

    fireEvent.click(screen.getByRole("button", { name: "关闭引导" }));

    expect(props.onDismiss).toHaveBeenCalledTimes(1);
  });

  it("can be dismissed with Escape", () => {
    const props = setup();

    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });

    expect(props.onDismiss).toHaveBeenCalledTimes(1);
  });

  it("surfaces a create failure instead of silently continuing", () => {
    setup({ error: "创建项目失败：目录不可写" });

    expect(screen.getByRole("alert")).toHaveTextContent("目录不可写");
  });

  it("blocks the actions while a create is in flight", () => {
    setup({ busy: true });

    expect(screen.getByRole("button", { name: "选择数据目录" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "打开已有项目" })).toBeDisabled();
  });
});
