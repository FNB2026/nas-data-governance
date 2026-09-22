// @vitest-environment jsdom
//
// App-level first-run gate (UI-P7-C / UI-P7-D scenario 1 & 3).
// Verifies that the onboarding overlay is shown exactly once — on a
// first launch with no project — and never over an open project.

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";

const { contextMock } = vi.hoisted(() => ({
  contextMock: {
    project: null as null | Record<string, unknown>,
    onboardingDone: false,
    gettingStartedDone: false,
  } as Record<string, unknown>,
}));

vi.mock("./state/ProjectContext", () => ({
  ProjectProvider: ({ children }: { children: ReactNode }) => <>{children}</>,
  useProject: () => contextMock,
}));

import App from "./App";
import { deriveCapabilities } from "./app/capability";

function resetContext(overrides: Record<string, unknown> = {}) {
  const caps = deriveCapabilities({ projectOpen: false, isReadWrite: false });
  Object.assign(contextMock, {
    version: { version: "0.5.0-beta.1", commit: "abc", build_time: "t", channel: "beta" },
    project: null,
    projectPath: "",
    isReadWrite: false,
    busy: false,
    error: null,
    recentProjects: [],
    pendingScanRoot: "",
    clearPendingScanRoot: vi.fn(),
    storages: [],
    storagesError: null,
    activeJobId: null,
    scanProgress: null,
    cancelling: false,
    canRetryScan: false,
    connectionStatus: "connected",
    jobs: [],
    jobsError: null,
    hasMoreJobs: false,
    loadMoreJobs: vi.fn(),
    scanFilterState: "",
    scanFilterType: "",
    setScanFilterState: vi.fn(),
    setScanFilterType: vi.fn(),
    toasts: [],
    pushToast: vi.fn(),
    dismissToast: vi.fn(),
    dataRevision: 0,
    capabilities: caps,
    pathPrivacyMode: false,
    togglePathPrivacy: vi.fn(),
    displayPath: (path: string) => path,
    defaultFullScan: false,
    defaultWorkers: "",
    setDefaultFullScan: vi.fn(),
    setDefaultWorkers: vi.fn(),
    onboardingDone: false,
    dismissOnboarding: vi.fn(),
    restartOnboarding: vi.fn(),
    gettingStartedDone: false,
    dismissGettingStarted: vi.fn(),
    setProjectPath: vi.fn(),
    openProject: vi.fn(async () => undefined),
    openExisting: vi.fn(async () => undefined),
    createNewProject: vi.fn(async () => undefined),
    closeProject: vi.fn(async () => undefined),
    refreshProject: vi.fn(async () => undefined),
    startScan: vi.fn(async () => undefined),
    retryLastScan: vi.fn(async () => undefined),
    resumeScan: vi.fn(async () => undefined),
    cancelScan: vi.fn(async () => undefined),
    loadJobs: vi.fn(async () => undefined),
    refreshRecoveryLock: vi.fn(async () => null),
    ...overrides,
  });
}

beforeEach(() => resetContext());
afterEach(cleanup);

describe("first-run onboarding gate", () => {
  it("shows the overlay on a first launch with no project", () => {
    render(<App />);

    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("开始使用 NDG")).toBeInTheDocument();
  });

  it("stays hidden on later launches once dismissed", () => {
    resetContext({ onboardingDone: true });
    render(<App />);

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    // The start card remains the entry point.
    expect(screen.getByRole("heading", { name: "开始" })).toBeInTheDocument();
  });

  it("never covers an open project", () => {
    resetContext({
      project: { path: "/data/ndg/project.json", name: "产业资料库" },
      isReadWrite: true,
      capabilities: deriveCapabilities({ projectOpen: true, isReadWrite: true }),
    });
    render(<App />);

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("dismisses the overlay from the close button", () => {
    render(<App />);

    screen.getByRole("button", { name: "关闭引导" }).click();

    expect(contextMock.dismissOnboarding).toHaveBeenCalledTimes(1);
  });

  it("dismisses the overlay when the user opens an existing project", () => {
    render(<App />);

    screen.getByRole("button", { name: "打开已有项目" }).click();

    expect(contextMock.dismissOnboarding).toHaveBeenCalledTimes(1);
  });

  it("shows the getting-started rail on an open project until dismissed", () => {
    resetContext({
      project: { path: "/data/ndg/project.json", name: "产业资料库" },
      isReadWrite: true,
      capabilities: deriveCapabilities({ projectOpen: true, isReadWrite: true }),
      onboardingDone: true,
    });
    render(<App />);

    expect(screen.getByLabelText("入门步骤")).toBeInTheDocument();
  });

  it("hides the getting-started rail once dismissed", () => {
    resetContext({
      project: { path: "/data/ndg/project.json", name: "产业资料库" },
      isReadWrite: true,
      capabilities: deriveCapabilities({ projectOpen: true, isReadWrite: true }),
      onboardingDone: true,
      gettingStartedDone: true,
    });
    render(<App />);

    expect(screen.queryByLabelText("入门步骤")).not.toBeInTheDocument();
  });
});
