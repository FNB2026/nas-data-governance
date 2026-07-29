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
import {
  CancelScan,
  CheckRecoveryLock,
  CloseProject,
  GetProjectInfo,
  GetScanProgress,
  GetVersion,
  ListRecentJobs,
  ListStorages,
  OpenProject,
  OpenProjectReadWrite,
  StartScan,
} from "../wailsjs/go/wails/API";
import { wails } from "../wailsjs/go/models";
import { TERMINAL_STATES, hasWailsRuntime, friendlyError } from "../lib/utils";
import type { ToastItem, ToastType } from "../components/Toast";
import { deriveCapabilities, type AppCapabilities } from "../app/capability";
import { loadSettings, saveSettings, maskPath } from "./settings";
import { isNetworkError, computeBackoffDelay, retryWithBackoff } from "../lib/retry";

// ---- Context value type ----

interface ScanStartParams {
  root: string;
  storageId: string;
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
  pushToast: (type: ToastType, title: string, message?: string) => void;
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

  // Actions
  setProjectPath: (path: string) => void;
  openProject: (readWrite: boolean) => Promise<void>;
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

  const togglePathPrivacy = useCallback(() => {
    setPathPrivacyMode((prev) => {
      const next = !prev;
      saveSettings({ pathPrivacyMode: next });
      return next;
    });
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
  const MAX_POLL_RETRIES = 5;

  // ---- Toast helpers ----

  const pushToast = useCallback((type: ToastType, title: string, message?: string) => {
    const id = ++toastIdRef.current;
    setToasts((prev) => [...prev, { id, type, title, message }]);
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
    try {
      const list = await retryWithBackoff(() => ListStorages(), {
        maxRetries: 3,
        onRetry: (attempt) => {
          if (attempt === 1) setConnectionStatus("reconnecting");
        },
      });
      setStorages(list || []);
      setStoragesError(null);
      setConnectionStatus("connected");
    } catch (e: unknown) {
      setStoragesError(friendlyError(e));
      if (isNetworkError(e)) setConnectionStatus("disconnected");
    }
  }, []);

  const loadJobs = useCallback(async () => {
    if (!hasWailsRuntime()) return;
    try {
      const limit = jobsLimitRef.current;
      const list = await retryWithBackoff(() => ListRecentJobs(limit), { maxRetries: 3 });
      setJobs(list || []);
      setHasMoreJobs((list || []).length >= limit);
      setJobsError(null);
    } catch (e: unknown) {
      setJobsError(friendlyError(e));
    }
  }, []);

  const loadMoreJobs = useCallback(async () => {
    if (!hasWailsRuntime()) return;
    const nextLimit = jobsLimitRef.current + 20;
    jobsLimitRef.current = nextLimit;
    try {
      const list = await ListRecentJobs(nextLimit);
      setJobs(list || []);
      setHasMoreJobs((list || []).length >= nextLimit);
      setJobsError(null);
    } catch (e: unknown) {
      setJobsError(friendlyError(e));
    }
  }, []);

  // ---- Version (load on mount) ----

  useEffect(() => {
    if (!hasWailsRuntime()) {
      setVersion({ version: "dev", commit: "n/a", build_time: "n/a" } as wails.VersionInfo);
      return;
    }
    GetVersion()
      .then(setVersion)
      .catch((e: unknown) => setError(friendlyError(e)));
  }, []);

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
        const p = await GetScanProgress(activeJobId);

        if (projectRevRef.current !== pollRev) return;

        setScanProgress(p);

        // Reset retry counter on success
        pollRetryCountRef.current = 0;
        setConnectionStatus("connected");

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
            pushToast("warning", "连接中断", `正在尝试重新连接… (${attempt}/${MAX_POLL_RETRIES})`);
          }

          scheduleNextPoll(delay);
        } else {
          // Non-network error, or max retries exceeded — give up
          setConnectionStatus("disconnected");
          setActiveJobId(null);
          setCancelling(false);
          pollRetryCountRef.current = 0;

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
  }, [activeJobId, loadJobs, loadStorages, notifyDataChanged, pushToast]);

  // ---- Project actions ----

  const refreshRecoveryLock = useCallback(async () => {
    if (!hasWailsRuntime()) return null;
    try {
      const status = await CheckRecoveryLock();
      setRecoveryLockActive(status.lock_active);
      return status;
    } catch {
      return null;
    }
  }, []);

  const openProject = useCallback(async (readWrite: boolean) => {
    if (!projectPath.trim()) {
      setError("请先选择或输入项目数据库路径");
      return;
    }
    setBusy(true);
    try {
      const info = readWrite
        ? await OpenProjectReadWrite(projectPath.trim())
        : await OpenProject(projectPath.trim());
      // Bump project revision to invalidate any in-flight poll from a
      // previous project session.
      projectRevRef.current++;
      setProject(info);
      setIsReadWrite(readWrite);
      setLastScanParams(null);
      setError(null);

      // Check recovery lock after opening
      try {
        const status = await CheckRecoveryLock();
        setRecoveryLockActive(status.lock_active);
        if (status.lock_active) {
          pushToast("warning", "恢复锁激活", `检测到 ${status.executing_count} 个未完成执行计划`);
        }
      } catch {
        setRecoveryLockActive(false);
      }

      const loads: Promise<void>[] = [loadStorages()];
      if (readWrite) {
        loads.push(loadJobs());
      }
      await Promise.all(loads);
      notifyDataChanged();
    } catch (e: unknown) {
      setError(friendlyError(e));
    } finally {
      setBusy(false);
    }
  }, [projectPath, loadStorages, loadJobs, notifyDataChanged]);

  const closeProject = useCallback(async () => {
    setBusy(true);
    try {
      await CloseProject();
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
      setError(null);
    } catch (e: unknown) {
      setError(friendlyError(e));
    } finally {
      setBusy(false);
    }
  }, []);

  const refreshProject = useCallback(async () => {
    setBusy(true);
    try {
      const info = await GetProjectInfo();
      setProject(info);
      setError(null);
      await Promise.all([
        loadStorages(),
        ...(isReadWrite ? [loadJobs()] : []),
      ]);
      notifyDataChanged();
    } catch (e: unknown) {
      setError(friendlyError(e));
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
        storageId: params.storageId.trim(),
      };
      const resp = await StartScan({
        root: normalizedParams.root,
        storage_id: normalizedParams.storageId,
        full_scan: normalizedParams.fullScan,
        workers: normalizedParams.workers,
      } as wails.StartScanRequest);
      setLastScanParams(normalizedParams);
      setActiveJobId(resp.job_id);
      setScanProgress(null);
    } catch (e: unknown) {
      pushToast("error", "启动扫描失败", friendlyError(e));
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
      await CancelScan(activeJobId);
    } catch (e: unknown) {
      pushToast("error", "取消扫描失败", friendlyError(e));
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
    setProjectPath,
    openProject,
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
