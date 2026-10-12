// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import SourceAuthorizationPanel from "./SourceAuthorizationPanel";

afterEach(cleanup);

describe("SourceAuthorizationPanel", () => {
  it("requires an explicit user selection and does not imply scanning", () => {
    const onAuthorize = vi.fn();
    render(<SourceAuthorizationPanel busy={false} isReadWrite onAuthorize={onAuthorize} />);

    expect(screen.getByText(/不会自动扫描、Resume 或解除恢复锁/)).toBeInTheDocument();
    expect(screen.getByText(/不会自动迁移旧检查点/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "选择并授权扫描目录" }));
    expect(onAuthorize).toHaveBeenCalledTimes(1);
  });

  it("disables authorization for read-only projects or while busy", () => {
    const onAuthorize = vi.fn();
    const { rerender } = render(
      <SourceAuthorizationPanel busy={false} isReadWrite={false} onAuthorize={onAuthorize} />,
    );
    expect(screen.getByRole("button", { name: "选择并授权扫描目录" })).toBeDisabled();

    rerender(<SourceAuthorizationPanel busy onAuthorize={onAuthorize} isReadWrite />);
    expect(screen.getByRole("button", { name: "正在登记授权…" })).toBeDisabled();
    expect(onAuthorize).not.toHaveBeenCalled();
  });
});
