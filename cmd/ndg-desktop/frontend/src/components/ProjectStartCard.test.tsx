// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

const { pickDirectoryMock } = vi.hoisted(() => ({
  pickDirectoryMock: vi.fn(),
}));

vi.mock("../state/ProjectContext", () => ({
  useProject: () => ({ displayPath: (path: string) => path }),
}));
vi.mock("../lib/utils", () => ({ hasWailsRuntime: () => true }));
vi.mock("../api/client", () => ({
  api: { project: { pickDirectory: pickDirectoryMock } },
}));

import ProjectStartCard from "./ProjectStartCard";

beforeEach(() => {
  pickDirectoryMock.mockReset().mockResolvedValue("/Volumes/NAS/产业资料库");
});

afterEach(() => cleanup());

describe("ProjectStartCard first-launch flow", () => {
  it("keeps the database path behind the advanced entry", () => {
    render(
      <ProjectStartCard
        busy={false}
        error={null}
        recentProjects={[]}
        onCreate={vi.fn()}
        onOpenRecent={vi.fn()}
        onOpenExisting={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: "新建扫描项目" })).toBeInTheDocument();
    expect(screen.queryByLabelText("项目数据库路径")).not.toBeInTheDocument();
  });

  it("creates from the picked directory without requiring a name", async () => {
    const onCreate = vi.fn();
    render(
      <ProjectStartCard
        busy={false}
        error={null}
        recentProjects={[]}
        onCreate={onCreate}
        onOpenRecent={vi.fn()}
        onOpenExisting={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "新建扫描项目" }));
    fireEvent.click(screen.getByRole("button", { name: "选择目录" }));
    await waitFor(() => expect(screen.getByLabelText("待扫描目录")).toHaveValue("/Volumes/NAS/产业资料库"));

    fireEvent.click(screen.getByRole("button", { name: "创建项目并前往扫描" }));
    expect(onCreate).toHaveBeenCalledWith("", "/Volumes/NAS/产业资料库");
  });
});
