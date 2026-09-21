// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

const { apiMock } = vi.hoisted(() => ({
  apiMock: {
    diagnostics: { formats: vi.fn(), governance: vi.fn(), merges: vi.fn() },
  },
}));

vi.mock("../api/client", () => ({ api: apiMock }));

import DiagnosticPanel from "./DiagnosticPanel";

beforeEach(() => {
  Object.defineProperty(window, "go", { value: {}, configurable: true });
  Object.defineProperty(window, "runtime", { value: {}, configurable: true });
  apiMock.diagnostics.formats.mockReset();
  apiMock.diagnostics.governance.mockReset();
  apiMock.diagnostics.merges.mockReset();
});

afterEach(() => {
  cleanup();
  Reflect.deleteProperty(window, "go");
  Reflect.deleteProperty(window, "runtime");
});

const emptyReport = {
  summary: {},
  large_unknown: [],
  extension_mismatches: [],
  metadata_gaps: [],
  safety_notes: [],
};

describe("DiagnosticPanel report states", () => {
  it("shows an empty state before any report is generated", () => {
    render(<DiagnosticPanel storages={[]} />);
    expect(screen.getByText("暂无诊断报告")).toBeInTheDocument();
    expect(screen.getByText("点击「运行诊断」生成格式审查报告")).toBeInTheDocument();
  });

  it("shows a loading state while generating a report", async () => {
    apiMock.diagnostics.formats.mockReturnValue(new Promise(() => {}));
    render(<DiagnosticPanel storages={[]} />);

    fireEvent.click(screen.getByRole("button", { name: "运行诊断" }));

    expect(screen.getByText("正在生成报告…")).toBeInTheDocument();
    expect(screen.queryByText("暂无诊断报告")).not.toBeInTheDocument();
  });

  it("shows an error state when generation fails", async () => {
    apiMock.diagnostics.formats.mockRejectedValue(new Error("诊断服务不可达"));
    render(<DiagnosticPanel storages={[]} />);

    fireEvent.click(screen.getByRole("button", { name: "运行诊断" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("诊断服务不可达");
  });

  it("renders the report when generation succeeds", async () => {
    apiMock.diagnostics.formats.mockResolvedValue(emptyReport);
    render(<DiagnosticPanel storages={[]} />);

    fireEvent.click(screen.getByRole("button", { name: "运行诊断" }));

    expect(await screen.findByText("文件总数")).toBeInTheDocument();
  });
});