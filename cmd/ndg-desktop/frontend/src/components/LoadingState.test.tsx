// @vitest-environment jsdom

import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import LoadingState from "./LoadingState";

afterEach(cleanup);

describe("LoadingState", () => {
  it("renders default label with status role and aria-busy", () => {
    render(<LoadingState />);
    const status = screen.getByRole("status");
    expect(status).toBeInTheDocument();
    expect(status).toHaveAttribute("aria-busy", "true");
    expect(screen.getByText("加载中…")).toBeInTheDocument();
  });

  it("renders a custom label", () => {
    render(<LoadingState label="正在生成报告…" />);
    expect(screen.getByText("正在生成报告…")).toBeInTheDocument();
  });
});