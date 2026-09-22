// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import ErrorState from "./ErrorState";

afterEach(cleanup);

describe("ErrorState", () => {
  it("renders message with alert role", () => {
    render(<ErrorState message="加载失败" />);
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("加载失败");
  });

  it("fires retry when onRetry is provided", () => {
    const onRetry = vi.fn();
    render(<ErrorState message="加载失败" onRetry={onRetry} />);
    fireEvent.click(screen.getByRole("button", { name: "重试" }));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it("renders technical detail when provided", () => {
    render(<ErrorState message="加载失败" detail="connection refused" />);
    expect(screen.getByText("技术详情")).toBeInTheDocument();
    expect(screen.getByText("connection refused")).toBeInTheDocument();
  });
});