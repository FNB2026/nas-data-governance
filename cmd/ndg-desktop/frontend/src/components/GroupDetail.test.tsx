// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { wails } from "../wailsjs/go/models";

const projectState = vi.hoisted(() => ({ pathPrivacyMode: false }));

vi.mock("../state/ProjectContext", () => ({
  useProject: () => ({
    displayPath: (path: string) => path,
    pathPrivacyMode: projectState.pathPrivacyMode,
  }),
}));

import GroupDetail from "./GroupDetail";

afterEach(() => {
  projectState.pathPrivacyMode = false;
  cleanup();
});

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

describe("GroupDetail directory context & retention", () => {
  it("renders per-copy role, protection, anchor, score and reasons", () => {
    render(
      <GroupDetail
        selectedGroup={detailWithDirContext()}
        detailLoading={false}
        detailError={null}
        onClose={vi.fn()}
      />,
    );

    expect(screen.getByText("目录语境与保留理由")).toBeInTheDocument();
    expect(screen.getAllByText("formal_archive")).toHaveLength(1);
    expect(screen.getAllByText("temporary")).toHaveLength(1);
    expect(screen.getByText("受保护")).toBeInTheDocument();
    expect(screen.getByText("保留项")).toBeInTheDocument();
    expect(screen.getByText("135")).toBeInTheDocument();
    expect(screen.getByText("87")).toBeInTheDocument();
    expect(screen.getByText("PRJ-2024-001")).toBeInTheDocument();
    expect(screen.getByText("PRJ-2024-002")).toBeInTheDocument();
    expect(screen.getByText("同组保留项：保留评分最高")).toBeInTheDocument();
    expect(screen.getByText(/目录权威等级 90/)).toBeInTheDocument();
  });

  it("hides business-anchor details in path privacy mode", () => {
    projectState.pathPrivacyMode = true;

    render(
      <GroupDetail
        selectedGroup={detailWithDirContext()}
        detailLoading={false}
        detailError={null}
        onClose={vi.fn()}
      />,
    );

    expect(screen.getAllByText("已隐藏")).toHaveLength(2);
    expect(screen.queryByText("PRJ-2024-001")).not.toBeInTheDocument();
    expect(screen.queryByText("PRJ-2024-002")).not.toBeInTheDocument();
  });
});

function detailWithDirContext(): wails.GroupDetailResponse {
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
    files: [firstPath, secondPath].map((path, idx) => ({
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
      is_retain_selected: idx === 0,
      retain_reason:
        idx === 0 ? "同组保留项：保留评分最高" : "同一低权威目录角色内的完全重复副本；待审批后隔离",
      retain_score: {
        total: idx === 0 ? 135 : 87,
        authority: 90,
        stability: 20,
        path_depth: 8,
        role_bonus: 20,
        reasons:
          idx === 0
            ? ["目录权威等级 90（+90）", "角色 formal_archive：权威来源奖励 +20"]
            : ["目录权威等级 10（+10）"],
      },
      dir_context: {
        role: idx === 0 ? "formal_archive" : "temporary",
        authority_level: idx === 0 ? 90 : 10,
        privacy_level: "normal",
        protected: idx === 0,
        branch_point: "",
        business_anchor: idx === 0 ? "PRJ-2024-001" : "PRJ-2024-002",
      },
    })),
  });
}
