// @vitest-environment jsdom

import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { apiMock } = vi.hoisted(() => ({
  apiMock: {
    project: {
      version: vi.fn(),
      open: vi.fn(),
      openReadWrite: vi.fn(),
      close: vi.fn(),
      info: vi.fn(),
      listRecent: vi.fn(),
    },
    storages: { list: vi.fn() },
    scan: {
      start: vi.fn(),
      cancel: vi.fn(),
      getProgress: vi.fn(),
      listJobs: vi.fn(),
    },
    recovery: { checkLock: vi.fn() },
  },
}));

vi.mock("../api/client", () => ({ api: apiMock }));

import { ProjectProvider, useProject } from "./ProjectContext";

const settingsValues = new Map<string, string>();
const storageMock = {
  getItem: (key: string) => settingsValues.get(key) ?? null,
  setItem: (key: string, value: string) => settingsValues.set(key, value),
  removeItem: (key: string) => settingsValues.delete(key),
  clear: () => settingsValues.clear(),
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function ContextHarness() {
  const ctx = useProject();
  return (
    <div>
      <input
        aria-label="project path"
        value={ctx.projectPath}
        onChange={(event) => ctx.setProjectPath(event.target.value)}
      />
      <button onClick={() => void ctx.openProject(true)}>open rw</button>
      <button onClick={() => void ctx.closeProject()}>close</button>
      <button onClick={() => void ctx.startScan({
        root: "  /source/root  ",
        storageId: "  archive  ",
        fullScan: true,
        workers: 8,
      })}>start scan</button>
      <span data-testid="project">{ctx.project?.path ?? "closed"}</span>
      <span data-testid="storages">{ctx.storages.map((storage) => storage.id).join(",")}</span>
      <span data-testid="active-job">{ctx.activeJobId ?? "none"}</span>
      <span data-testid="can-execute">{String(ctx.capabilities.can_execute_quarantine)}</span>
      <span data-testid="toasts">{ctx.toasts.map((toast) => toast.title).join(",")}</span>
    </div>
  );
}

function renderContext() {
  return render(
    <ProjectProvider>
      <ContextHarness />
    </ProjectProvider>,
  );
}

async function openReadWriteProject() {
  fireEvent.change(screen.getByLabelText("project path"), { target: { value: "/tmp/project.db" } });
  fireEvent.click(screen.getByRole("button", { name: "open rw" }));
  await waitFor(() => expect(screen.getByTestId("project")).toHaveTextContent("/tmp/project.db"));
}

beforeEach(() => {
  Object.defineProperty(window, "go", { value: {}, configurable: true });
  Object.defineProperty(window, "runtime", { value: {}, configurable: true });
  Object.defineProperty(window, "localStorage", { value: storageMock, configurable: true });
  vi.stubGlobal("localStorage", storageMock);
  settingsValues.clear();
  apiMock.project.version.mockReset().mockResolvedValue({ version: "test", commit: "test", build_time: "now" });
  apiMock.project.openReadWrite.mockReset().mockResolvedValue({ path: "/tmp/project.db", is_open: true });
  apiMock.project.open.mockReset().mockResolvedValue({ path: "/tmp/project.db", is_open: true });
  apiMock.project.close.mockReset().mockResolvedValue(undefined);
  apiMock.project.info.mockReset().mockResolvedValue({ path: "/tmp/project.db", is_open: true });
  apiMock.project.listRecent.mockReset().mockResolvedValue([]);
  apiMock.storages.list.mockReset().mockResolvedValue([]);
  apiMock.scan.listJobs.mockReset().mockResolvedValue([]);
  apiMock.scan.start.mockReset().mockResolvedValue({ job_id: "job-1" });
  apiMock.scan.cancel.mockReset().mockResolvedValue(undefined);
  apiMock.scan.getProgress.mockReset();
  apiMock.recovery.checkLock.mockReset().mockResolvedValue({ lock_active: false, executing_count: 0 });
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  Reflect.deleteProperty(window, "go");
  Reflect.deleteProperty(window, "runtime");
});

describe("ProjectContext safety and scan wiring", () => {
  it("maps normalized scan parameters to the backend request", async () => {
    renderContext();
    await openReadWriteProject();
    fireEvent.click(screen.getByRole("button", { name: "start scan" }));

    await waitFor(() => {
      expect(apiMock.scan.start).toHaveBeenCalledWith({
        root: "/source/root",
        storage_id: "archive",
        full_scan: true,
        workers: 8,
      });
    });
    expect(screen.getByTestId("active-job")).toHaveTextContent("job-1");
  });

  it("derives a blocked execution capability from the recovery lock", async () => {
    apiMock.recovery.checkLock.mockResolvedValue({ lock_active: true, executing_count: 2 });
    renderContext();
    await openReadWriteProject();

    await waitFor(() => expect(screen.getByTestId("can-execute")).toHaveTextContent("false"));
    expect(screen.getByTestId("toasts")).toHaveTextContent("恢复锁激活");
  });

  it("discards a storage response that resolves after the project closes", async () => {
    const pendingStorages = deferred<Array<{ id: string }>>();
    apiMock.storages.list.mockReturnValue(pendingStorages.promise);
    renderContext();
    await openReadWriteProject();

    fireEvent.click(screen.getByRole("button", { name: "close" }));
    await waitFor(() => expect(screen.getByTestId("project")).toHaveTextContent("closed"));

    await act(async () => {
      pendingStorages.resolve([{ id: "stale-storage" }]);
      await pendingStorages.promise;
    });

    expect(screen.getByTestId("storages")).toHaveTextContent("");
    expect(screen.getByTestId("storages")).not.toHaveTextContent("stale-storage");
  });

  it("drops a completed scan response after the project closes", async () => {
    vi.useFakeTimers();
    const pendingProgress = deferred<{
      state: string;
      stage: string;
      discovered: number;
      processed: number;
      failed: number;
      warning_count: number;
    }>();
    apiMock.scan.getProgress.mockReturnValue(pendingProgress.promise);
    renderContext();

    fireEvent.change(screen.getByLabelText("project path"), { target: { value: "/tmp/project.db" } });
    fireEvent.click(screen.getByRole("button", { name: "open rw" }));
    await act(async () => {});
    fireEvent.click(screen.getByRole("button", { name: "start scan" }));
    await act(async () => {});
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });
    expect(apiMock.scan.getProgress).toHaveBeenCalledWith("job-1");

    fireEvent.click(screen.getByRole("button", { name: "close" }));
    await act(async () => {});
    await act(async () => {
      pendingProgress.resolve({
        state: "COMPLETED",
        stage: "FINALIZING",
        discovered: 10,
        processed: 10,
        failed: 0,
        warning_count: 0,
      });
      await pendingProgress.promise;
    });

    expect(screen.getByTestId("active-job")).toHaveTextContent("none");
    expect(screen.getByTestId("toasts")).not.toHaveTextContent("扫描完成");
  });
});
