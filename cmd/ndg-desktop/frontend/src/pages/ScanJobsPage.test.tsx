// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

const { contextMock, startScanMock } = vi.hoisted(() => {
  const startScan = vi.fn();
  return {
    startScanMock: startScan,
    contextMock: {
      activeJobId: null,
      scanProgress: null,
      cancelling: false,
      canRetryScan: false,
      jobs: [],
      jobsError: null,
      hasMoreJobs: false,
      loadMoreJobs: vi.fn(),
      scanFilterState: "",
      scanFilterType: "",
      setScanFilterState: vi.fn(),
      setScanFilterType: vi.fn(),
      startScan,
      retryLastScan: vi.fn(),
      cancelScan: vi.fn(),
      capabilities: { can_scan: true },
      defaultFullScan: true,
      defaultWorkers: "6",
      pathPrivacyMode: false,
    },
  };
});

vi.mock("../state/ProjectContext", () => ({ useProject: () => contextMock }));

import ScanJobsPage from "./ScanJobsPage";

beforeEach(() => {
  startScanMock.mockReset().mockResolvedValue(undefined);
  contextMock.defaultFullScan = true;
  contextMock.defaultWorkers = "6";
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("ScanJobsPage scan defaults", () => {
  it("initializes advanced controls from persisted defaults", () => {
    render(<ScanJobsPage />);
    fireEvent.click(screen.getByRole("button", { name: "高级选项" }));

    expect(screen.getByLabelText("并发数（可选）")).toHaveValue(6);
    expect(screen.getByLabelText("全量扫描")).toBeChecked();
  });

  it("sends normalized defaults in the scan request", async () => {
    render(<ScanJobsPage />);
    fireEvent.change(screen.getByLabelText("根目录"), { target: { value: "  /source/root  " } });
    fireEvent.click(screen.getByRole("button", { name: "高级选项" }));
    fireEvent.change(screen.getByLabelText("存储 ID（可选）"), { target: { value: "  archive  " } });
    fireEvent.click(screen.getByRole("button", { name: "开始扫描" }));

    await waitFor(() => {
      expect(startScanMock).toHaveBeenCalledWith({
        root: "/source/root",
        storageId: "archive",
        fullScan: true,
        workers: 6,
      });
    });
  });

  it("blocks an empty root before calling the backend", () => {
    render(<ScanJobsPage />);
    fireEvent.click(screen.getByRole("button", { name: "开始扫描" }));

    expect(screen.getByRole("alert")).toHaveTextContent("请输入要扫描的根目录路径");
    expect(startScanMock).not.toHaveBeenCalled();
  });
});
