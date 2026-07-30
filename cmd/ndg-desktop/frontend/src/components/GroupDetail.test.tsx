// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { wails } from "../wailsjs/go/models";

vi.mock("../state/ProjectContext", () => ({
  useProject: () => ({ displayPath: (path: string) => path }),
}));

import GroupDetail from "./GroupDetail";

afterEach(cleanup);

const firstPath = "/Volumes/archive/F.产业资料库/场地类/场景资料库（待整理）/公园、绿景/IMG_20181216_154426.jpg";
const secondPath = "/Volumes/archive/F.产业资料库/场地类/场景资料库（待整理）/景/IMG_20181216_154426.jpg";

function detailWithUnreliableNasIdentity(): wails.GroupDetailResponse {
  return new wails.GroupDetailResponse({
    group_id: "group-1",
    sha256: "53ea638600000000000000000000000000000000000000000000000000007a62",
    size: 4_600_000,
    storage_id: "nas-f-industry-library",
    path_count: 2,
    physical_copy_count: 2,
    hardlink_alias_count: 0,
    physical_reclaimable_bytes: 4_600_000,
    sample_path: firstPath,
    files: [firstPath, secondPath].map((path) => ({
      storage_id: "nas-f-industry-library",
      path,
      name: "IMG_20181216_154426.jpg",
      size: 4_600_000,
      modified_at: "2023-04-05T00:08:15Z",
      is_symlink: false,
      physical_device: 0,
      physical_inode: 0,
      physical_link_count: 0,
      physical_reliable: false,
    })),
  });
}

describe("GroupDetail physical evidence", () => {
  it("shows complete paths and explains conservative NAS identity estimates", () => {
    render(
      <GroupDetail
        selectedGroup={detailWithUnreliableNasIdentity()}
        detailLoading={false}
        detailError={null}
        onClose={vi.fn()}
      />,
    );

    expect(screen.getByText("物理副本与硬链接关系")).toBeInTheDocument();
    expect(screen.getByText(firstPath)).toBeInTheDocument();
    expect(screen.getByText(secondPath)).toBeInTheDocument();
    expect(screen.getAllByText("物理身份待确认")).toHaveLength(2);
    expect(screen.getAllByText("按独立副本保守估算")).toHaveLength(2);
    expect(screen.getByText(/物理副本（估算）：/)).toBeInTheDocument();
    expect(screen.queryByText("独立物理副本")).not.toBeInTheDocument();
  });
});
