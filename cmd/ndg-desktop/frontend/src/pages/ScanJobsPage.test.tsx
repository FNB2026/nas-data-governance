// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

const { contextMock, startScanMock } = vi.hoisted(() => {
  const startScan = vi.fn();
  return {
    startScanMock: startScan,
    contextMock: {
      activeJobId: null as string | null,
      scanProgress: null as null | {
        job_id: string;
        state: string;
        stage: string;
        discovered: number;
        processed: number;
        failed: number;
        warning_count: number;
        error_code: string;
        created_at: string;
        started_at: string;
        completed_at: string;
      },
      cancelling: false,
      canRetryScan: false,
      jobs: [],
      storages: [] as Array<{ id: string; root_path: string; kind: string; created_at: string }>,
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
  contextMock.activeJobId = null;
  contextMock.scanProgress = null;
  contextMock.storages = [];
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

  it("fills the root and storage ID from the registered-directory dropdown", () => {
    contextMock.storages = [{
      id: "nas-industry",
      root_path: "/Volumes/archive/F.industry",
      kind: "filesystem",
      created_at: "2026-07-30T08:00:00Z",
    }];
    render(<ScanJobsPage />);

    fireEvent.change(screen.getByLabelText("选择已登记目录"), {
      target: { value: "/Volumes/archive/F.industry" },
    });

    expect(screen.getByLabelText("根目录")).toHaveValue("/Volumes/archive/F.industry");
    fireEvent.click(screen.getByRole("button", { name: "高级选项" }));
    expect(screen.getByLabelText("存储 ID（可选）")).toHaveValue("nas-industry");
  });

  it("shows full hashing as indeterminate candidate verification", () => {
    contextMock.activeJobId = "job-1";
    contextMock.scanProgress = {
      job_id: "job-1",
      state: "RUNNING",
      stage: "FULL_HASHING",
      discovered: 735,
      processed: 735,
      failed: 0,
      warning_count: 0,
      error_code: "",
      created_at: "2026-07-30T08:00:00Z",
      started_at: "2026-07-30T08:00:00Z",
      completed_at: "",
    };
    render(<ScanJobsPage />);

    expect(screen.getByText("完整哈希")).toBeInTheDocument();
    expect(screen.getByText("正在校验重复候选的完整内容")).toBeInTheDocument();
    expect(screen.queryByText("100%")).not.toBeInTheDocument();
  });

  it("labels finalization as saving the index", () => {
    contextMock.activeJobId = "job-1";
    contextMock.scanProgress = {
      job_id: "job-1",
      state: "RUNNING",
      stage: "FINALIZING",
      discovered: 735,
      processed: 735,
      failed: 0,
      warning_count: 0,
      error_code: "",
      created_at: "2026-07-30T08:00:00Z",
      started_at: "2026-07-30T08:00:00Z",
      completed_at: "",
    };
    render(<ScanJobsPage />);

    expect(screen.getByText("保存索引与收尾")).toBeInTheDocument();
  });
});
