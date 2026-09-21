// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, within } from "@testing-library/react";

const { contextMock, openExternalMock } = vi.hoisted(() => ({
  contextMock: {
    version: null as null | {
      version: string;
      commit: string;
      build_time: string;
      channel: string;
    },
    error: null as string | null,
    capabilities: {
      project_open: false,
      project_mode: "closed",
      can_scan: false,
      can_view_results: false,
      can_edit_reviews: false,
      can_approve_plans: false,
      can_execute_quarantine: false,
      can_execute_purge: false,
      recovery_lock_active: false,
      disabled_reasons: {},
    } as Record<string, unknown>,
    pathPrivacyMode: false,
    togglePathPrivacy: vi.fn(),
    defaultFullScan: true,
    defaultWorkers: "6",
    setDefaultFullScan: vi.fn(),
    setDefaultWorkers: vi.fn(),
    recentProjects: [] as Array<{ name: string; path: string }>,
    displayPath: (path: string) => path,
    restartOnboarding: vi.fn(),
  },
  openExternalMock: vi.fn(),
}));

vi.mock("../state/ProjectContext", () => ({ useProject: () => contextMock }));
vi.mock("../lib/external", () => ({ openExternal: openExternalMock }));

import SettingsPage from "./SettingsPage";
import { PRODUCT_LINKS } from "../app/productInfo";
import { deriveCapabilities } from "../app/capability";

/** Capabilities for an open, read-write project with no recovery lock. */
function readWriteCapabilities() {
  return deriveCapabilities({ projectOpen: true, isReadWrite: true }) as unknown as Record<
    string,
    unknown
  >;
}

beforeEach(() => {
  contextMock.version = null;
  contextMock.error = null;
  contextMock.capabilities = deriveCapabilities({
    projectOpen: false,
    isReadWrite: false,
  }) as unknown as Record<string, unknown>;
  contextMock.recentProjects = [];
  openExternalMock.mockClear();
  contextMock.restartOnboarding.mockClear();
});

afterEach(cleanup);

describe("SettingsPage information architecture", () => {
  it("renders the five groups in the documented order", () => {
    render(<SettingsPage />);

    const headings = screen
      .getAllByRole("heading", { level: 3 })
      .map((node) => node.textContent);

    expect(headings).toEqual(["常规", "扫描", "隐私", "安全", "关于"]);
  });

  it("keeps scan defaults in the scan group", () => {
    render(<SettingsPage />);

    const scan = document.getElementById("settings-scan") as HTMLElement;
    expect(within(scan).getByLabelText(/默认完整扫描/)).toBeChecked();
    expect(within(scan).getByLabelText(/默认并发工作线程数/)).toHaveValue(6);
  });
});

describe("SettingsPage privacy group", () => {
  it("shows path masking state and the three closed network switches", () => {
    render(<SettingsPage />);

    const privacy = document.getElementById("settings-privacy") as HTMLElement;
    expect(within(privacy).getByLabelText(/路径脱敏模式/)).not.toBeChecked();
    expect(within(privacy).getAllByText("已关闭")).toHaveLength(3);
    expect(within(privacy).getByText("外部 AI")).toBeInTheDocument();
    expect(within(privacy).getByText("遥测")).toBeInTheDocument();
    expect(within(privacy).getByText("云上传")).toBeInTheDocument();
  });

  it("reflects the masked preview when privacy mode is on", () => {
    contextMock.pathPrivacyMode = true;
    render(<SettingsPage />);

    const privacy = document.getElementById("settings-privacy") as HTMLElement;
    expect(privacy.querySelector(".settings-preview-path")?.textContent).toBe(
      "…/[hidden]/r***t.pdf",
    );
    expect(within(privacy).getByText("已开启")).toBeInTheDocument();
    contextMock.pathPrivacyMode = false;
  });

  it("shows the unmasked preview when privacy mode is off", () => {
    render(<SettingsPage />);

    const privacy = document.getElementById("settings-privacy") as HTMLElement;
    expect(privacy.querySelector(".settings-preview-path")?.textContent).toBe(
      "/data/archive/Documents/Work/Projects/report.pdf",
    );
  });
});

describe("SettingsPage security group", () => {
  it("reports a closed project as restricted", () => {
    render(<SettingsPage />);

    const security = document.getElementById("settings-security") as HTMLElement;
    expect(within(security).getByText("当前项目模式")).toBeInTheDocument();
    expect(within(security).getByText("未打开")).toBeInTheDocument();
    expect(within(security).getByText("Recovery Lock")).toBeInTheDocument();
    expect(within(security).getAllByText("受限")).toHaveLength(3);
  });

  it("unlocks governance and purge for a read-write project", () => {
    contextMock.capabilities = readWriteCapabilities();
    render(<SettingsPage />);

    const security = document.getElementById("settings-security") as HTMLElement;
    expect(within(security).getByText("读写")).toBeInTheDocument();
    expect(within(security).getAllByText("可用")).toHaveLength(3);
    expect(within(security).getByText("无")).toBeInTheDocument();
  });

  it("surfaces an active recovery lock", () => {
    contextMock.capabilities = deriveCapabilities({
      projectOpen: true,
      isReadWrite: true,
      recoveryLockActive: true,
    }) as unknown as Record<string, unknown>;
    render(<SettingsPage />);

    const security = document.getElementById("settings-security") as HTMLElement;
    expect(within(security).getByText("激活")).toBeInTheDocument();
    expect(within(security).getAllByText("受限")).toHaveLength(3);
  });
});

describe("SettingsPage general group", () => {
  it("shows an empty state when there are no recent projects", () => {
    render(<SettingsPage />);

    const general = document.getElementById("settings-general") as HTMLElement;
    expect(within(general).getByText("暂无最近项目")).toBeInTheDocument();
  });

  it("lists recent projects and masks their paths when privacy is on", () => {
    contextMock.recentProjects = [{ name: "产业资料库", path: "/data/ndg/db" }];
    contextMock.displayPath = () => "…/[hidden]/d***b";
    render(<SettingsPage />);

    const general = document.getElementById("settings-general") as HTMLElement;
    expect(within(general).getByText("产业资料库")).toBeInTheDocument();
    expect(within(general).getByText("…/[hidden]/d***b")).toBeInTheDocument();
    contextMock.displayPath = (path: string) => path;
  });

  it("restarts the first-run guide from the general group", () => {
    render(<SettingsPage />);

    const general = document.getElementById("settings-general") as HTMLElement;
    within(general).getByRole("button", { name: "重新显示首次启动引导" }).click();
    expect(contextMock.restartOnboarding).toHaveBeenCalledTimes(1);
  });
});

describe("SettingsPage about group", () => {
  it("shows a loading state while version is missing", () => {
    render(<SettingsPage />);
    const about = document.getElementById("settings-about") as HTMLElement;
    const status = within(about).getByRole("status");
    expect(status).toHaveAttribute("aria-busy", "true");
    expect(within(about).getByText("正在加载版本信息…")).toBeInTheDocument();
  });

  it("shows an error state when version failed to load", () => {
    contextMock.error = "版本获取失败";
    render(<SettingsPage />);
    const about = document.getElementById("settings-about") as HTMLElement;
    expect(within(about).getByRole("alert")).toHaveTextContent("版本获取失败");
  });

  it("renders the product identity and version table", () => {
    contextMock.version = {
      version: "0.5.0-beta.1",
      commit: "abc123",
      build_time: "2026-09-01",
      channel: "beta",
    };
    render(<SettingsPage />);

    const about = document.getElementById("settings-about") as HTMLElement;
    expect(within(about).getByText("NDG — NAS Data Governance")).toBeInTheDocument();
    expect(within(about).getByText("0.5.0-beta.1")).toBeInTheDocument();
    expect(within(about).getByText("abc123")).toBeInTheDocument();
    expect(within(about).getByText("2026-09-01")).toBeInTheDocument();
    expect(within(about).getByText("beta")).toBeInTheDocument();
  });

  it("exposes every product link and opens it only on click", () => {
    render(<SettingsPage />);

    const about = document.getElementById("settings-about") as HTMLElement;
    // No link is opened just by rendering the page.
    expect(openExternalMock).not.toHaveBeenCalled();

    for (const link of PRODUCT_LINKS) {
      const button = within(about).getByRole("button", { name: new RegExp(link.label) });
      button.click();
    }

    expect(openExternalMock).toHaveBeenCalledTimes(PRODUCT_LINKS.length);
    const openedUrls = openExternalMock.mock.calls.map((call) => call[0]);
    expect(openedUrls).toEqual(PRODUCT_LINKS.map((link) => link.url));
  });

  it("states the Beta offline boundary in the About group", () => {
    render(<SettingsPage />);

    const about = document.getElementById("settings-about") as HTMLElement;
    expect(
      within(about).getByText(/不提供在线更新检测、账号登录或自动联网/),
    ).toBeInTheDocument();
  });
});
