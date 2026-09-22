// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import EmptyState from "./EmptyState";

afterEach(cleanup);

describe("EmptyState", () => {
  it("renders title and optional hint", () => {
    render(<EmptyState title="暂无数据" hint="请先运行一次扫描" />);
    expect(screen.getByText("暂无数据")).toBeInTheDocument();
    expect(screen.getByText("请先运行一次扫描")).toBeInTheDocument();
  });

  it("fires action when actionLabel and onAction are provided", () => {
    const onAction = vi.fn();
    render(<EmptyState title="暂无数据" actionLabel="运行诊断" onAction={onAction} />);
    fireEvent.click(screen.getByRole("button", { name: "运行诊断" }));
    expect(onAction).toHaveBeenCalledTimes(1);
  });
});