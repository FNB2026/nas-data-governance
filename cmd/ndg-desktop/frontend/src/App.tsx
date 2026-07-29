import { useCallback, useEffect, useRef, useState } from "react";
import {
  CancelScan,
  CloseProject,
  GetGroupDetail,
  GetJobDetail,
  GetProjectInfo,
  GetScanProgress,
  GetVersion,
  ListDuplicateGroups,
  ListRecentJobs,
  ListStorages,
  OpenProject,
  OpenProjectReadWrite,
  StartScan,
} from "./wailsjs/go/wails/API";
import { wails } from "./wailsjs/go/models";
import { TERMINAL_STATES, hasWailsRuntime, errorText } from "./lib/utils";

import ProjectPanel from "./components/ProjectPanel";
import ScanPanel from "./components/ScanPanel";
import StorageList from "./components/StorageList";
import DuplicateGroups from "./components/DuplicateGroups";
import GroupDetail from "./components/GroupDetail";
import JobDetail from "./components/JobDetail";
import DiagnosticPanel from "./components/DiagnosticPanel";
import ToastContainer, { type ToastItem, type ToastType } from "./components/Toast";

export default function App() {
  const [version, setVersion] = useState<wails.VersionInfo | null>(null);
  const [project, setProject] = useState<wails.ProjectInfo | null>(null);
  const [projectPath, setProjectPath] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Read-write mode
  const [readWriteMode, setReadWriteMode] = useState(false);
  const [isReadWrite, setIsReadWrite] = useState(false);

  // Storage list state
  const [storages, setStorages] = useState<wails.StorageInfo[]>([]);
  const [storagesError, setStoragesError] = useState<string | null>(null);

  // Duplicate groups state (keyset pagination)
  const [groups, setGroups] = useState<wails.GroupSummary[]>([]);
  const [nextCursor, setNextCursor] = useState("");
  const [totalCount, setTotalCount] = useState(0);
  const [groupsLoading, setGroupsLoading] = useState(false);
  const [groupsError, setGroupsError] = useState<string | null>(null);
  const [storageFilter, setStorageFilter] = useState("");
  const [minReclaimableMiB, setMinReclaimableMiB] = useState("");
  const [appliedStorageFilter, setAppliedStorageFilter] = useState("");
  const [appliedMinimumBytes, setAppliedMinimumBytes] = useState(0);
  const groupsRequestInFlight = useRef(false);

  // Group detail state
  const [selectedGroup, setSelectedGroup] = useState<wails.GroupDetailResponse | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState<string | null>(null);

  // Scan form state
  const [scanRoot, setScanRoot] = useState("");
  const [scanStorageId, setScanStorageId] = useState("");
  const [scanFullScan, setScanFullScan] = useState(false);
  const [scanWorkers, setScanWorkers] = useState("");
  const [scanError, setScanError] = useState<string | null>(null);
  const [scanStarting, setScanStarting] = useState(false);

  // Active scan job
  const [activeJobId, setActiveJobId] = useState<string | null>(null);
  const [scanProgress, setScanProgress] = useState<wails.ScanJobProgress | null>(null);
  const [cancelling, setCancelling] = useState(false);

  // Job history
  const [jobs, setJobs] = useState<wails.JobSummary[]>([]);
  const [jobsError, setJobsError] = useState<string | null>(null);

  // Job detail
  const [selectedJob, setSelectedJob] = useState<wails.JobDetailResponse | null>(null);
  const [jobDetailLoading, setJobDetailLoading] = useState(false);
  const [jobDetailError, setJobDetailError] = useState<string | null>(null);

  // Toast notifications
  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const toastIdRef = useRef(0);

  const pushToast = useCallback((type: ToastType, title: string, message?: string) => {
    const id = ++toastIdRef.current;
    setToasts((prev) => [...prev, { id, type, title, message }]);
  }, []);

  const dismissToast = useCallback((id: number) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  // ---- effects ----

  useEffect(() => {
    if (!hasWailsRuntime()) {
      setVersion({ version: "dev", commit: "n/a", build_time: "n/a" } as wails.VersionInfo);
      return;
    }
    GetVersion()
      .then(setVersion)
      .catch((e: unknown) => setError(errorText(e)));
  }, []);

  // Progress polling: when activeJobId is set, poll GetScanProgress every 1s.
  // Stops automatically when the job reaches a terminal state.
  useEffect(() => {
    if (!activeJobId) return;

    const intervalId = setInterval(async () => {
      try {
        const p = await GetScanProgress(activeJobId);
        setScanProgress(p);
        if (TERMINAL_STATES.has(p.state)) {
          clearInterval(intervalId);
          setActiveJobId(null);
          setCancelling(false);
          void loadJobs();

          // Scan completion notification
          if (p.state === "COMPLETED") {
            void loadStorages();
            void loadGroups("", appliedStorageFilter, appliedMinimumBytes);
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
        }
      } catch {
        clearInterval(intervalId);
        setActiveJobId(null);
        setCancelling(false);
        pushToast("error", "扫描连接中断", "无法获取扫描进度，请检查后重试");
      }
    }, 1000);

    return () => clearInterval(intervalId);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeJobId, appliedStorageFilter, appliedMinimumBytes]);

  // ---- data loaders ----

  const loadStorages = useCallback(async () => {
    if (!hasWailsRuntime()) return;
    try {
      const list = await ListStorages();
      setStorages(list || []);
      setStoragesError(null);
    } catch (e: unknown) {
      setStoragesError(errorText(e));
    }
  }, []);

  const loadGroups = useCallback(async (
    cursor: string,
    storageId: string,
    minReclaimableBytes: number,
  ) => {
    if (!hasWailsRuntime() || groupsRequestInFlight.current) return;
    groupsRequestInFlight.current = true;
    setGroupsLoading(true);
    try {
      const resp = await ListDuplicateGroups({
        storage_id: storageId,
        page_size: 20,
        cursor,
        min_reclaimable_bytes: minReclaimableBytes,
      } as wails.ListGroupsRequest);
      if (cursor) {
        setGroups((prev) => [...prev, ...(resp.groups || [])]);
      } else {
        setGroups(resp.groups || []);
      }
      setNextCursor(resp.next_cursor || "");
      setTotalCount(resp.total_count || 0);
      setGroupsError(null);
    } catch (e: unknown) {
      setGroupsError(errorText(e));
    } finally {
      groupsRequestInFlight.current = false;
      setGroupsLoading(false);
    }
  }, []);

  const loadJobs = useCallback(async () => {
    if (!hasWailsRuntime()) return;
    try {
      const list = await ListRecentJobs(20);
      setJobs(list || []);
      setJobsError(null);
    } catch (e: unknown) {
      setJobsError(errorText(e));
    }
  }, []);

  // ---- project handlers ----

  const resetScanState = () => {
    setActiveJobId(null);
    setScanProgress(null);
    setCancelling(false);
    setScanError(null);
    setScanRoot("");
    setScanStorageId("");
    setScanFullScan(false);
    setScanWorkers("");
    setSelectedJob(null);
    setJobDetailError(null);
    setJobs([]);
    setJobsError(null);
  };

  const handleOpenProject = async () => {
    if (!projectPath.trim()) {
      setError("请先选择或输入项目数据库路径");
      return;
    }
    setBusy(true);
    try {
      const info = readWriteMode
        ? await OpenProjectReadWrite(projectPath.trim())
        : await OpenProject(projectPath.trim());
      setProject(info);
      setIsReadWrite(readWriteMode);
      setError(null);
      setSelectedGroup(null);
      setStorageFilter("");
      setMinReclaimableMiB("");
      setAppliedStorageFilter("");
      setAppliedMinimumBytes(0);
      resetScanState();
      const loads: Promise<void>[] = [loadStorages(), loadGroups("", "", 0)];
      if (readWriteMode) {
        loads.push(loadJobs());
      }
      await Promise.all(loads);
    } catch (e: unknown) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  };

  const handleCloseProject = async () => {
    setBusy(true);
    try {
      await CloseProject();
      setProject(null);
      setIsReadWrite(false);
      setStorages([]);
      setGroups([]);
      setNextCursor("");
      setTotalCount(0);
      setSelectedGroup(null);
      setStorageFilter("");
      setMinReclaimableMiB("");
      setAppliedStorageFilter("");
      setAppliedMinimumBytes(0);
      setError(null);
      resetScanState();
    } catch (e: unknown) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  };

  const handleRefreshProject = async () => {
    setBusy(true);
    try {
      const info = await GetProjectInfo();
      setProject(info);
      setError(null);
      await Promise.all([
        loadStorages(),
        loadGroups("", appliedStorageFilter, appliedMinimumBytes),
        ...(isReadWrite ? [loadJobs()] : []),
      ]);
      setSelectedGroup(null);
    } catch (e: unknown) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  };

  // ---- duplicate group handlers ----

  const handleLoadMore = () => {
    if (nextCursor && !groupsLoading) {
      loadGroups(nextCursor, appliedStorageFilter, appliedMinimumBytes);
    }
  };

  const parsedMinimumBytes = (): number | null => {
    if (!minReclaimableMiB.trim()) return 0;
    const value = Number(minReclaimableMiB);
    if (!Number.isFinite(value) || value < 0) return null;
    return Math.floor(value * 1024 * 1024);
  };

  const handleApplyFilters = () => {
    const minimum = parsedMinimumBytes();
    if (minimum === null) {
      setGroupsError("最小可回收空间必须是非负数");
      return;
    }
    setSelectedGroup(null);
    setAppliedStorageFilter(storageFilter);
    setAppliedMinimumBytes(minimum);
    void loadGroups("", storageFilter, minimum);
  };

  const handleSelectGroup = async (storageId: string, sha256: string) => {
    if (!hasWailsRuntime()) return;
    setDetailLoading(true);
    setDetailError(null);
    try {
      const detail = await GetGroupDetail(storageId, sha256);
      setSelectedGroup(detail);
    } catch (e: unknown) {
      setDetailError(errorText(e));
      setSelectedGroup(null);
    } finally {
      setDetailLoading(false);
    }
  };

  const handleCloseDetail = () => {
    setSelectedGroup(null);
    setDetailError(null);
  };

  // ---- scan handlers ----

  const handleStartScan = async () => {
    if (!scanRoot.trim()) {
      setScanError("请输入要扫描的根目录路径");
      return;
    }
    setScanStarting(true);
    setScanError(null);
    try {
      const workersNum = scanWorkers.trim() ? parseInt(scanWorkers, 10) : undefined;
      const resp = await StartScan({
        root: scanRoot.trim(),
        storage_id: scanStorageId.trim(),
        full_scan: scanFullScan,
        workers: Number.isFinite(workersNum) && workersNum! > 0 ? workersNum : undefined,
      } as wails.StartScanRequest);
      setActiveJobId(resp.job_id);
      setScanProgress(null);
    } catch (e: unknown) {
      setScanError(errorText(e));
    } finally {
      setScanStarting(false);
    }
  };

  const handleCancelScan = async () => {
    if (!activeJobId) return;
    setCancelling(true);
    try {
      await CancelScan(activeJobId);
    } catch (e: unknown) {
      setScanError(errorText(e));
      setCancelling(false);
    }
    // Polling will detect CANCELLED state and reset cancelling.
  };

  // ---- job detail handlers ----

  const handleSelectJob = async (jobId: string) => {
    if (!hasWailsRuntime()) return;
    setJobDetailLoading(true);
    setJobDetailError(null);
    try {
      const detail = await GetJobDetail(jobId);
      setSelectedJob(detail);
    } catch (e: unknown) {
      setJobDetailError(errorText(e));
      setSelectedJob(null);
    } finally {
      setJobDetailLoading(false);
    }
  };

  const handleCloseJobDetail = () => {
    setSelectedJob(null);
    setJobDetailError(null);
  };

  // ---- derived ----

  const projectOpen = project !== null;
  const scanActive = activeJobId !== null;
  const progressPercent =
    scanProgress && scanProgress.discovered > 0
      ? Math.min(100, Math.round((scanProgress.processed / scanProgress.discovered) * 100))
      : 0;

  // ---- render ----

  return (
    <div className="app">
      <header className="app-header">
        <h1>NDG 数据治理工作台</h1>
        {version && (
          <span className="version-badge">
            v{version.version} ({version.commit})
          </span>
        )}
      </header>

      <main className={projectOpen ? "app-main app-main--dashboard" : "app-main"}>
        <ProjectPanel
          project={project}
          projectPath={projectPath}
          busy={busy}
          error={error}
          readWriteMode={readWriteMode}
          isReadWrite={isReadWrite}
          onProjectPathChange={setProjectPath}
          onReadWriteModeChange={setReadWriteMode}
          onOpenProject={handleOpenProject}
          onCloseProject={handleCloseProject}
          onRefreshProject={handleRefreshProject}
        />

        {projectOpen && (
          <>
            {isReadWrite && (
              <ScanPanel
                scanRoot={scanRoot}
                scanStorageId={scanStorageId}
                scanFullScan={scanFullScan}
                scanWorkers={scanWorkers}
                scanStarting={scanStarting}
                scanError={scanError}
                scanActive={scanActive}
                scanProgress={scanProgress}
                cancelling={cancelling}
                progressPercent={progressPercent}
                jobs={jobs}
                jobsError={jobsError}
                jobDetailLoading={jobDetailLoading}
                onScanRootChange={setScanRoot}
                onScanStorageIdChange={setScanStorageId}
                onScanFullScanChange={setScanFullScan}
                onScanWorkersChange={setScanWorkers}
                onStartScan={handleStartScan}
                onCancelScan={handleCancelScan}
                onSelectJob={handleSelectJob}
              />
            )}

            <StorageList storages={storages} storagesError={storagesError} />

            <DuplicateGroups
              groups={groups}
              totalCount={totalCount}
              groupsLoading={groupsLoading}
              groupsError={groupsError}
              nextCursor={nextCursor}
              storages={storages}
              storageFilter={storageFilter}
              minReclaimableMiB={minReclaimableMiB}
              detailLoading={detailLoading}
              selectedGroup={selectedGroup}
              onStorageFilterChange={setStorageFilter}
              onMinReclaimableMiBChange={setMinReclaimableMiB}
              onApplyFilters={handleApplyFilters}
              onLoadMore={handleLoadMore}
              onSelectGroup={handleSelectGroup}
            />

            <GroupDetail
              selectedGroup={selectedGroup}
              detailLoading={detailLoading}
              detailError={detailError}
              onClose={handleCloseDetail}
            />

            <JobDetail
              selectedJob={selectedJob}
              jobDetailLoading={jobDetailLoading}
              jobDetailError={jobDetailError}
              onClose={handleCloseJobDetail}
            />

            <DiagnosticPanel storages={storages} />
          </>
        )}
      </main>

      <footer className="app-footer">
        <p>NDG — NAS Data Governance · {isReadWrite ? "读写模式" : "只读 Alpha"}</p>
      </footer>

      <ToastContainer toasts={toasts} onDismiss={dismissToast} />
    </div>
  );
}
