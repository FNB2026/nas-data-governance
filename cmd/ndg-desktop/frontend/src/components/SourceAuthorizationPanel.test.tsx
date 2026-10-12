// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import SourceAuthorizationPanel from "./SourceAuthorizationPanel";

afterEach(cleanup);

describe("SourceAuthorizationPanel", () => {
  it("requires an explicit user selection and does not imply scanning", () => {
    const onAuthorize = vi.fn();
    render(<SourceAuthorizationPanel enabled busy={false} isReadWrite onAuthorize={onAuthorize} />);

    expect(screen.getByText(/Mac App Store 沙盒版只会/)).toBeInTheDocument();
    expect(screen.getByText(/不会自动扫描、Resume 或解除恢复锁/)).toBeInTheDocument();
    expect(screen.getByText(/不会自动迁移旧检查点/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "选择并授权扫描目录" }));
    expect(onAuthorize).toHaveBeenCalledTimes(1);
  });

  it("disables authorization for read-only projects or while busy", () => {
    const onAuthorize = vi.fn();
    const { rerender } = render(
      <SourceAuthorizationPanel enabled busy={false} isReadWrite={false} onAuthorize={onAuthorize} />,
    );
    expect(screen.getByRole("button", { name: "选择并授权扫描目录" })).toBeDisabled();

    rerender(<SourceAuthorizationPanel enabled busy onAuthorize={onAuthorize} isReadWrite />);
    expect(screen.getByRole("button", { name: "正在登记授权…" })).toBeDisabled();
    expect(onAuthorize).not.toHaveBeenCalled();
  });

  it("hides MAS authorization copy when the backend capability is absent", () => {
    render(<SourceAuthorizationPanel enabled={false} busy={false} isReadWrite onAuthorize={vi.fn()} />);
    expect(screen.queryByText("源目录授权")).not.toBeInTheDocument();
    expect(screen.queryByText(/Mac App Store 沙盒版/)).not.toBeInTheDocument();
  });
});
