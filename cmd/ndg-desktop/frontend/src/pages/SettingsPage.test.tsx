// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";

const { contextMock } = vi.hoisted(() => ({
  contextMock: {
    version: null as null | {
      version: string;
      commit: string;
      build_time: string;
      channel: string;
    },
    error: null as string | null,
    capabilities: {} as Record<string, never>,
    pathPrivacyMode: false,
    togglePathPrivacy: vi.fn(),
    defaultFullScan: true,
    defaultWorkers: "6",
    setDefaultFullScan: vi.fn(),
    setDefaultWorkers: vi.fn(),
  },
}));

vi.mock("../state/ProjectContext", () => ({ useProject: () => contextMock }));

import SettingsPage from "./SettingsPage";

beforeEach(() => {
  contextMock.version = null;
  contextMock.error = null;
});

afterEach(cleanup);

describe("SettingsPage version states", () => {
  it("shows a loading state while version is missing", () => {
    render(<SettingsPage />);
    const status = screen.getByRole("status");
    expect(status).toHaveAttribute("aria-busy", "true");
    expect(screen.getByText("正在加载版本信息…")).toBeInTheDocument();
  });

  it("shows an error state when version failed to load", () => {
    contextMock.error = "版本获取失败";
    render(<SettingsPage />);
    expect(screen.getByRole("alert")).toHaveTextContent("版本获取失败");
  });

  it("renders the version table when available", () => {
    contextMock.version = {
      version: "0.5.0-beta.1",
      commit: "abc123",
      build_time: "2026-09-01",
      channel: "beta",
    };
    render(<SettingsPage />);
    expect(screen.getByText("0.5.0-beta.1")).toBeInTheDocument();
    expect(screen.getByText("abc123")).toBeInTheDocument();
  });
});