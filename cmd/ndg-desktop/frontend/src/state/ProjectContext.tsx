// Global project context: holds cross-page state extracted from App.tsx.
// Scan polling lives here so it survives page navigation.
// Page-local state (scan form, group filters, selected items) stays in pages.

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { wails } from "../wailsjs/go/models";
import { TERMINAL_STATES, hasWailsRuntime } from "../lib/utils";
import type { ToastItem, ToastType } from "../components/Toast";
import { deriveCapabilities, type AppCapabilities } from "../app/capability";
import { loadSettings, saveSettings, maskPath } from "./settings";
import { isNetworkError, computeBackoffDelay } from "../lib/retry";
import { api } from "../api/client";

// ---- Context value type ----

interface ScanStartParams {
  root: string;
  fullScan: boolean;
  workers?: number;
}

interface ProjectContextValue {
  // Version
  version: wails.VersionInfo | null;

  // Project
  project: wails.ProjectInfo | null;
  projectPath: string;
  isReadWrite: boolean;
  busy: boolean;
  error: string | null;

  // Recent projects (start-card entry; available before a project opens)
  recentProjects: wails.RecentProjectEntry[];
  // Pending scan root: set when a project is created from the start card
  // so the scan page can prefill the root, then cleared once consumed.
  pendingScanRoot: string;
  clearPendingScanRoot: () => void;

  // Storages (shared across pages)
  storages: wails.StorageInfo[];
  storagesError: string | null;

  // Scan status (global — polling must survive navigation)
  activeJobId: string | null;
  scanProgress: wails.ScanJobProgress | null;
  cancelling: boolean;
  canRetryScan: boolean;

  // Connection status (network drive tolerance)
  connectionStatus: "connected" | "reconnecting" | "disconnected";

  // Jobs (shared)
  jobs: wails.JobSummary[];
  jobsError: string | null;
  hasMoreJobs: boolean;
  loadMoreJobs: () => Promise<void>;

  // Scan filter persistence (survives page navigation)
  scanFilterState: string;
  scanFilterType: string;
  setScanFilterState: (v: string) => void;
  setScanFilterType: (v: string) => void;

  // Toast notifications
  toasts: ToastItem[];
  pushToast: (type: ToastType, title: string, message?: string) => number;
  dismissToast: (id: number) => void;

  // Data refresh signal — incremented when scan completes or project refreshes.
  // Pages watch this to reload their data.
  dataRevision: number;

  // Capabilities
  capabilities: AppCapabilities;

  // Settings (global — reactive across all pages)
  pathPrivacyMode: boolean;
  togglePathPrivacy: () => void;
  displayPath: (path: string) => string;

  // Scan defaults (global — reactive across pages)
  defaultFullScan: boolean;
  defaultWorkers: string;
  setDefaultFullScan: (v: boolean) => void;
  setDefaultWorkers: (v: string) => void;

  // Actions
  setProjectPath: (path: string) => void;
  openProject: (readWrite: boolean) => Promise<void>;
  openExisting: (path: string, readWrite: boolean) => Promise<void>;
  createNewProject: (name: string, scanRoot: string) => Promise<void>;
  closeProject: () => Promise<void>;
  refreshProject: () => Promise<void>;
  startScan: (params: ScanStartParams) => Promise<void>;
  retryLastScan: () => Promise<void>;
  cancelScan: () => Promise<void>;
  loadJobs: () => Promise<void>;
  refreshRecoveryLock: () => Promise<wails.RecoveryStatusDTO | null>;
}

const ProjectContext = createContext<ProjectContextValue | null>(null);

// ---- Provider ----

export function ProjectProvider({ children }: { children: ReactNode }) {
  const [version, setVersion] = useState<wails.VersionInfo | null>(null);
  const [project, setProject] = useState<wails.ProjectInfo | null>(null);
  const [projectPath, setProjectPath] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [isReadWrite, setIsReadWrite] = useState(false);

  // Storages
  const [storages, setStorages] = useState<wails.StorageInfo[]>([]);
  const [storagesError, setStoragesError] = useState<string | null>(null);

  // Recent projects + pending scan root (start-card flow)
  const [recentProjects, setRecentProjects] = useState<wails.RecentProjectEntry[]>([]);
  const [pendingScanRoot, setPendingScanRoot] = useState<string>("");

  // Scan status
  const [activeJobId, setActiveJobId] = useState<string | null>(null);
  const [scanProgress, setScanProgress] = useState<wails.ScanJobProgress | null>(null);
  const [cancelling, setCancelling] = useState(false);
  const [lastScanParams, setLastScanParams] = useState<ScanStartParams | null>(null);

  // Jobs
  const [jobs, setJobs] = useState<wails.JobSummary[]>([]);
  const [jobsError, setJobsError] = useState<string | null>(null);
  const jobsLimitRef = useRef(20);
  const [hasMoreJobs, setHasMoreJobs] = useState(false);

  // Scan filter persistence (survives page navigation)
  const [scanFilterState, setScanFilterState] = useState("");
  const [scanFilterType, setScanFilterType] = useState("");

  // Toast
  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const toastIdRef = useRef(0);

  // Data revision
  const [dataRevision, setDataRevision] = useState(0);

  // Recovery lock (checked after project open)
  const [recoveryLockActive, setRecoveryLockActive] = useState(false);

  // Connection status (network drive tolerance)
  const [connectionStatus, setConnectionStatus] = useState<"connected" | "reconnecting" | "disconnected">("connected");

  // Settings (global — reactive across all pages)
  const [pathPrivacyMode, setPathPrivacyMode] = useState<boolean>(() => loadSettings().pathPrivacyMode);
  const [defaultFullScan, setDefaultFullScanState] = useState<boolean>(() => loadSettings().defaultFullScan);
  const [defaultWorkers, setDefaultWorkersState] = useState<string>(() => loadSettings().defaultWorkers);

  const togglePathPrivacy = useCallback(() => {
    setPathPrivacyMode((prev) => {
      const next = !prev;
      saveSettings({ ...loadSettings(), pathPrivacyMode: next });
      return next;
    });
  }, []);

  const setDefaultFullScan = useCallback((v: boolean) => {
    setDefaultFullScanState(v);
    saveSettings({ ...loadSettings(), defaultFullScan: v });
  }, []);

  const setDefaultWorkers = useCallback((v: string) => {
    setDefaultWorkersState(v);
    saveSettings({ ...loadSettings(), defaultWorkers: v });
  }, []);

  const displayPath = useCallback(
    (path: string): string => {
      if (!pathPrivacyMode) return path;
      return maskPath(path);
    },
    [pathPrivacyMode],
  );

  // Project revision: incremented on every open/close/switch. The polling
  // effect captures this value at start and checks it before writing back
  // state, preventing stale responses from a previous project session.
  const projectRevRef = useRef(0);
  // Poll-in-flight guard: prevents overlapping GetScanProgress requests
  // when the backend takes longer than the 1-second interval.
  const pollInFlightRef = useRef(false);
  // Retry tracking for network drive disconnection tolerance
  const pollRetryCountRef = useRef(0);
  const pollTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reconnectToastIdRef = useRef<number | null>(null);
  const MAX_POLL_RETRIES = 5;

  // ---- Toast helpers ----

  const pushToast = useCallback((type: ToastType, title: string, message?: string) => {
    const id = ++toastIdRef.current;
    setToasts((prev) => [...prev, { id, type, title, message }]);
    return id;
  }, []);

  const dismissToast = useCallback((id: number) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  const notifyDataChanged = useCallback(() => {
    setDataRevision((r) => r + 1);
  }, []);

  // ---- Data loaders ----

  const loadStorages = useCallback(async () => {
    if (!hasWailsRuntime()) return;
    const loadRev = projectRevRef.current;
    try {
      const list = await api.storages.list();
      if (projectRevRef.current !== loadRev) return;
      setStorages(list || []);
      setStoragesError(null);
    } catch (e: unknown) {
      if (projectRevRef.current !== loadRev) return;
      setStoragesError((e as Error).message);
    }
  }, []);

  // Best-effort recent-projects refresh for the start card. Failures are
  // swallowed so an unreadable manifest never blocks opening a project.
  const loadRecentProjects = useCallback(async () => {
    if (!hasWailsRuntime()) return;
    try {
      const list = await api.project.listRecent();
      setRecentProjects(list || []);
    } catch {
      // non-fatal
    }
  }, []);

  const loadJobs = useCallback(async () => {
    if (!hasWailsRuntime()) return;
    const loadRev = projectRevRef.current;
    try {
      const limit = jobsLimitRef.current;
      const list = await api.scan.listJobs(limit);
      if (projectRevRef.current !== loadRev) return;
      setJobs(list || []);
      setHasMoreJobs((list || []).length >= limit);
      setJobsError(null);
    } catch (e: unknown) {
      if (projectRevRef.current !== loadRev) return;
      setJobsError((e as Error).message);
    }
  }, []);

  const loadMoreJobs = useCallback(async () => {
    if (!hasWailsRuntime()) return;
    const loadRev = projectRevRef.current;
    const nextLimit = jobsLimitRef.current + 20;
    try {
      const list = await api.scan.listJobs(nextLimit);
      if (projectRevRef.current !== loadRev) return;
      jobsLimitRef.current = nextLimit;
      setJobs(list || []);
      setHasMoreJobs((list || []).length >= nextLimit);
      setJobsError(null);
    } catch (e: unknown) {
      if (projectRevRef.current !== loadRev) return;
      setJobsError((e as Error).message);
    }
  }, []);

  // ---- Version (load on mount) ----

  useEffect(() => {
    if (!hasWailsRuntime()) {
      setVersion({ version: "dev", commit: "n/a", build_time: "n/a" } as wails.VersionInfo);
      return;
    }
    api.project.version()
      .then(setVersion)
      .catch((e: unknown) => setError((e as Error).message));
    // Load recent projects for the start card (available before any
    // project is open).
    void loadRecentProjects();
  }, [loadRecentProjects]);

  // ---- Scan polling (global — survives page navigation) ----
  // Uses recursive setTimeout with exponential backoff for network drive tolerance.

  useEffect(() => {
    if (!activeJobId) return;

    const pollRev = projectRevRef.current;

    const poll = async () => {
      // Discard if project changed since poll started.
      if (projectRevRef.current !== pollRev) return;

      // Guard against overlapping requests.
      if (pollInFlightRef.current) {
        scheduleNextPoll(1000);
        return;
      }
      pollInFlightRef.current = true;

      try {
        const p = await api.scan.getProgress(activeJobId);

        if (projectRevRef.current !== pollRev) return;

        setScanProgress(p);

        // Reset retry counter on success
        pollRetryCountRef.current = 0;
        setConnectionStatus("connected");
        if (reconnectToastIdRef.current !== null) {
          dismissToast(reconnectToastIdRef.current);
          reconnectToastIdRef.current = null;
        }

        if (TERMINAL_STATES.has(p.state)) {
          setActiveJobId(null);
          setCancelling(false);
          void loadJobs();

          if (p.state === "COMPLETED") {
            void loadStorages();
            notifyDataChanged();
            pushToast(
              "success",
              "扫描完成",
              `已发现 ${p.discovered.toLocaleString()} 个文件，处理 ${p.processed.toLocaleString()} 个`,
            );
          } else if (p.state === "FAILED") {
            pushToast(
              "error",
              "扫描失败",
              p.error_code ? `错误码：${p.error_code}` : "请查看任务详情",
            );
          } else if (p.state === "CANCELLED") {
            pushToast("warning", "扫描已取消");
          }
          return; // Don't schedule next poll for terminal states
        }

        // Normal: schedule next poll in 1 second
        scheduleNextPoll(1000);
      } catch (error) {
        if (projectRevRef.current !== pollRev) return;

        if (isNetworkError(error) && pollRetryCountRef.current < MAX_POLL_RETRIES) {
          // Network error — retry with exponential backoff
          pollRetryCountRef.current++;
          const attempt = pollRetryCountRef.current;
          const delay = computeBackoffDelay(attempt - 1);
          setConnectionStatus("reconnecting");

          if (attempt === 1) {
            reconnectToastIdRef.current = pushToast(
              "warning",
              "连接中断",
              `正在尝试重新连接… (${attempt}/${MAX_POLL_RETRIES})`,
            );
          }

          scheduleNextPoll(delay);
        } else {
          // Non-network error, or max retries exceeded — give up
          setConnectionStatus(isNetworkError(error) ? "disconnected" : "connected");
          setActiveJobId(null);
          setCancelling(false);
          pollRetryCountRef.current = 0;
          if (reconnectToastIdRef.current !== null) {
            dismissToast(reconnectToastIdRef.current);
            reconnectToastIdRef.current = null;
          }

          if (isNetworkError(error)) {
            pushToast("error", "扫描连接中断", `重试 ${MAX_POLL_RETRIES} 次后仍无法连接，请检查网络盘状态后重试`);
          } else {
            pushToast("error", "扫描连接中断", "无法获取扫描进度，请检查后重试");
          }
        }
      } finally {
        pollInFlightRef.current = false;
      }
    };

    const scheduleNextPoll = (delayMs: number) => {
      if (pollTimeoutRef.current) clearTimeout(pollTimeoutRef.current);
      pollTimeoutRef.current = setTimeout(() => void poll(), delayMs);
    };

    // Start polling
    scheduleNextPoll(1000);

    return () => {
      if (pollTimeoutRef.current) clearTimeout(pollTimeoutRef.current);
      pollRetryCountRef.current = 0;
    };
  }, [activeJobId, dismissToast, loadJobs, loadStorages, notifyDataChanged, pushToast]);

  // ---- Project actions ----

  const refreshRecoveryLock = useCallback(async () => {
    if (!hasWailsRuntime()) return null;
    try {
      const status = await api.recovery.checkLock();
      setRecoveryLockActive(status.lock_active);
      return status;
    } catch {
      return null;
    }
  }, []);

  // Shared post-open wiring: bump rev, set project state, check recovery
  // lock, and reload storages / jobs / recent. Used by openProject,
  // openRecent and createNewProject so all entry points stay consistent.
  const wireOpenProject = useCallback(async (
    info: wails.ProjectInfo,
    readWrite: boolean,
  ) => {
    // Bump project revision to invalidate any in-flight poll from a
    // previous project session.
    projectRevRef.current++;
    setProject(info);
    setProjectPath(info.path);
    setIsReadWrite(readWrite);
    setLastScanParams(null);
    setError(null);

    // Check recovery lock after opening
    try {
      const status = await api.recovery.checkLock();
      setRecoveryLockActive(status.lock_active);
      if (status.lock_active) {
        pushToast("warning", "恢复锁激活", `检测到 ${status.executing_count} 个未完成执行计划`);
      }
    } catch {
      setRecoveryLockActive(false);
    }

    const loads: Promise<void>[] = [loadStorages(), loadRecentProjects()];
    if (readWrite) {
      loads.push(loadJobs());
    }
    await Promise.all(loads);
    setConnectionStatus("connected");
    notifyDataChanged();
  }, [loadStorages, loadJobs, loadRecentProjects, notifyDataChanged, pushToast]);

  const openProject = useCallback(async (readWrite: boolean) => {
    if (!projectPath.trim()) {
      setError("请先选择或输入项目数据库路径");
      return;
    }
    setBusy(true);
    try {
      const info = readWrite
        ? await api.project.openReadWrite(projectPath.trim())
        : await api.project.open(projectPath.trim());
      await wireOpenProject(info, readWrite);
    } catch (e: unknown) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }, [projectPath, wireOpenProject]);

  // Open an existing project database at an explicit path. Used by the
  // start card's "recent project" (read-write) and "advanced: open
  // existing database" (read-write or read-only) entries. Decoupled from
  // projectPath state so the start card never has to sync local input.
  const openExisting = useCallback(async (path: string, readWrite: boolean) => {
    if (!path.trim()) return;
    setBusy(true);
    try {
      const info = readWrite
        ? await api.project.openReadWrite(path.trim())
        : await api.project.open(path.trim());
      await wireOpenProject(info, readWrite);
    } catch (e: unknown) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }, [wireOpenProject]);

  // V9 first-launch: create a fresh project from a scan source in a
  // single atomic backend call. The backend validates the source, creates
  // the DB under the OS app-support dir, registers the source as a
  // storage with a backend-generated ID, and writes project.json — all
  // with rollback on failure. The recent-projects manifest is recorded
  // separately after the atomic transaction succeeds; its failure is
  // non-fatal and does not trigger rollback. The frontend only sets
  // pendingScanRoot so the scan page can prefill the root. The user never
  // types a .db path and never picks a storage ID.
  const createNewProject = useCallback(async (name: string, scanRoot: string) => {
    setBusy(true);
    try {
      const info = await api.project.createFromSource({
        name: name,
        source_path: scanRoot,
      });
      await wireOpenProject(info, true);
      setPendingScanRoot(scanRoot);
      pushToast("success", "项目已创建", "已在本机创建项目数据库并登记数据源，可开始扫描");
    } catch (e: unknown) {
      setError((e as Error).message);
      pushToast("error", "创建项目失败", (e as Error).message);
    } finally {
      setBusy(false);
    }
  }, [wireOpenProject, pushToast]);

  const clearPendingScanRoot = useCallback(() => setPendingScanRoot(""), []);

  const closeProject = useCallback(async () => {
    setBusy(true);
    try {
      await api.project.close();
      // Bump project revision to invalidate in-flight polls.
      projectRevRef.current++;
      setProject(null);
      setIsReadWrite(false);
      setStorages([]);
      setStoragesError(null);
      setActiveJobId(null);
      setScanProgress(null);
      setCancelling(false);
      setLastScanParams(null);
      setJobs([]);
      setJobsError(null);
      jobsLimitRef.current = 20;
      setHasMoreJobs(false);
      setScanFilterState("");
      setScanFilterType("");
      setRecoveryLockActive(false);
      setConnectionStatus("connected");
      setPendingScanRoot("");
      if (reconnectToastIdRef.current !== null) {
        const reconnectToastId = reconnectToastIdRef.current;
        setToasts((prev) => prev.filter((toast) => toast.id !== reconnectToastId));
        reconnectToastIdRef.current = null;
      }
      setError(null);
      // Refresh the recent list so the just-closed project appears in the
      // start card (the backend records it on open/create).
      void loadRecentProjects();
    } catch (e: unknown) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }, [loadRecentProjects]);

  const refreshProject = useCallback(async () => {
    setBusy(true);
    try {
      const info = await api.project.info();
      setProject(info);
      setError(null);
      await Promise.all([
        loadStorages(),
        ...(isReadWrite ? [loadJobs()] : []),
      ]);
      setConnectionStatus("connected");
      notifyDataChanged();
    } catch (e: unknown) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }, [isReadWrite, loadStorages, loadJobs, notifyDataChanged]);

  // ---- Scan actions ----

  const startScan = useCallback(async (params: ScanStartParams) => {
    if (!params.root.trim()) return;
    try {
      const normalizedParams = {
        ...params,
        root: params.root.trim(),
      };
      const resp = await api.scan.start({
        root: normalizedParams.root,
        full_scan: normalizedParams.fullScan,
        workers: normalizedParams.workers,
      } as wails.StartScanRequest);
      setLastScanParams(normalizedParams);
      setActiveJobId(resp.job_id);
      setScanProgress(null);
      setConnectionStatus("connected");
    } catch (e: unknown) {
      pushToast("error", "启动扫描失败", (e as Error).message);
    }
  }, [pushToast]);

  const retryLastScan = useCallback(async () => {
    if (!lastScanParams) {
      pushToast("warning", "无法重试扫描", "最近一次扫描参数不可用，请重新填写扫描参数");
      return;
    }
    await startScan(lastScanParams);
  }, [lastScanParams, pushToast, startScan]);

  const cancelScan = useCallback(async () => {
    if (!activeJobId) return;
    setCancelling(true);
    try {
      await api.scan.cancel(activeJobId);
    } catch (e: unknown) {
      pushToast("error", "取消扫描失败", (e as Error).message);
      setCancelling(false);
    }
    // Polling will detect CANCELLED state and reset cancelling.
  }, [activeJobId, pushToast]);

  // ---- Derived values ----

  const projectOpen = project !== null;
  const capabilities = deriveCapabilities({ projectOpen, isReadWrite, recoveryLockActive });

  // Scan progress percent (kept as-is; Phase 2 will fix DISCOVERING phase)
  // Exported via context for pages that need it.

  const value: ProjectContextValue = {
    version,
    project,
    projectPath,
    isReadWrite,
    busy,
    error,
    recentProjects,
    pendingScanRoot,
    clearPendingScanRoot,
    storages,
    storagesError,
    activeJobId,
    scanProgress,
    cancelling,
    canRetryScan: lastScanParams !== null,
    connectionStatus,
    jobs,
    jobsError,
    hasMoreJobs,
    loadMoreJobs,
    scanFilterState,
    scanFilterType,
    setScanFilterState,
    setScanFilterType,
    toasts,
    pushToast,
    dismissToast,
    dataRevision,
    capabilities,
    pathPrivacyMode,
    togglePathPrivacy,
    displayPath,
    defaultFullScan,
    defaultWorkers,
    setDefaultFullScan,
    setDefaultWorkers,
    setProjectPath,
    openProject,
    openExisting,
    createNewProject,
    closeProject,
    refreshProject,
    startScan,
    retryLastScan,
    cancelScan,
    loadJobs,
    refreshRecoveryLock,
  };

  return <ProjectContext.Provider value={value}>{children}</ProjectContext.Provider>;
}

// ---- Hook ----

export function useProject(): ProjectContextValue {
  const ctx = useContext(ProjectContext);
  if (!ctx) {
    throw new Error("useProject must be used within ProjectProvider");
  }
  return ctx;
}
