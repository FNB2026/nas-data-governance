// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { wails } from "../wailsjs/go/models";

vi.mock("../state/ProjectContext", () => ({
  useProject: () => ({ displayPath: (path: string) => path }),
}));

import DuplicateGroups from "./DuplicateGroups";

afterEach(cleanup);

const handlers = {
  onStorageFilterChange: () => {},
  onMinReclaimableMiBChange: () => {},
  onApplyFilters: () => {},
  onLoadMore: () => {},
  onSelectGroup: () => {},
};

const baseProps = {
  groups: [] as wails.GroupSummary[],
  totalCount: 0,
  groupsError: null as string | null,
  groupsLoading: false,
  nextCursor: "",
  storages: [] as wails.StorageInfo[],
  storageFilter: "",
  minReclaimableMiB: "",
  detailLoading: false,
  selectedGroup: null as wails.GroupDetailResponse | null,
  ...handlers,
};

describe("DuplicateGroups states", () => {
  it("shows a loading state on first load", () => {
    render(<DuplicateGroups {...baseProps} groupsLoading={true} />);
    expect(screen.getByRole("status")).toHaveAttribute("aria-busy", "true");
  });

  it("shows an empty state when no groups", () => {
    render(<DuplicateGroups {...baseProps} />);
    expect(screen.getByText("暂无数据")).toBeInTheDocument();
    expect(screen.getByText(/未检测到重复文件/)).toBeInTheDocument();
  });

  it("shows an error state when loading fails", () => {
    render(<DuplicateGroups {...baseProps} groupsError="后端不可达" />);
    expect(screen.getByRole("alert")).toHaveTextContent("后端不可达");
  });
});