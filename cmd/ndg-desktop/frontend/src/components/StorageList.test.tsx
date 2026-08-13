// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { wails } from "../wailsjs/go/models";

vi.mock("../state/ProjectContext", () => ({
  useProject: () => ({ displayPath: (path: string) => path }),
}));

import StorageList from "./StorageList";

afterEach(cleanup);

describe("StorageList source preflight", () => {
  it("shows conservative network storage guidance", () => {
    const storage = new wails.StorageInfo({
      id: "source-1",
      root_path: "/Volumes/synthetic-nas",
      kind: "filesystem",
      created_at: "2026-08-13T00:00:00Z",
    });
    const profile = new wails.SourcePreflightDTO({
      status: "online",
      filesystem_type: "smbfs",
      network: true,
      physical_identity_reliable: false,
      latency_ms: 413,
      recommended_workers: 1,
    });

    render(
      <StorageList
        storages={[storage]}
        storagesError={null}
        sourceProfiles={{ "source-1": profile }}
      />,
    );

    expect(screen.getByText("在线")).toBeInTheDocument();
    expect(screen.getByText("网络存储 · smbfs · 413 ms")).toBeInTheDocument();
    expect(screen.getByText("建议扫描并发 1")).toBeInTheDocument();
    expect(screen.getByText(/高延迟链路.*VPN\/Tailscale.*中继/)).toBeInTheDocument();
    expect(screen.getByText("物理身份按网络存储保守处理")).toBeInTheDocument();
  });

  it("does not expose backend error details for an unavailable source", () => {
    const storage = new wails.StorageInfo({
      id: "source-2",
      root_path: "/Volumes/private-share",
      kind: "filesystem",
    });
    render(
      <StorageList
        storages={[storage]}
        storagesError={null}
        sourceProfiles={{ "source-2": null }}
      />,
    );
    expect(screen.getByText("不可用")).toBeInTheDocument();
    expect(screen.queryByText(/private-share/)).toBeInTheDocument();
    expect(screen.queryByText(/timeout|permission|denied/i)).not.toBeInTheDocument();
  });
});
