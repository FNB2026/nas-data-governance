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

// ---- constants ----

const TERMINAL_STATES = new Set(["COMPLETED", "FAILED", "CANCELLED"]);

const STAGE_LABELS: Record<string, string> = {
  DISCOVERING: "发现文件",
  METADATA_INDEXING: "索引元数据",
  QUICK_HASHING: "快速哈希",
  FULL_HASHING: "完整哈希",
  CONTEXT_CLASSIFYING: "上下文分类",
  FORMAT_ANALYZING: "格式分析",
  GROUPING: "分组",
  PLANNING: "规划",
  FINALIZING: "收尾",
};

const STATE_LABELS: Record<string, string> = {
  QUEUED: "排队中",
  RUNNING: "运行中",
  CANCEL_REQUESTED: "取消中",
  COMPLETED: "已完成",
  FAILED: "已失败",
  CANCELLED: "已取消",
};

const EVENT_LABELS: Record<string, string> = {
  "job:created": "创建",
  "job:stage": "阶段切换",
  "job:progress": "进度更新",
  "job:warning": "警告",
  "job:completed": "完成",
  "job:failed": "失败",
  "job:cancelled": "取消",
};

// ---- helpers ----

function hasWailsRuntime(): boolean {
  return typeof window !== "undefined" && "go" in window && "runtime" in window;
}

function errorText(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function formatBytes(bytes: number): string {
  if (bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB", "PB", "EB"];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
}

function shortHash(hash: string): string {
  if (hash.length <= 12) return hash;
  return `${hash.slice(0, 8)}…${hash.slice(-4)}`;
}

function formatDateTime(iso: string): string {
  if (!iso) return "—";
  try {
    const d = new Date(iso);
    if (isNaN(d.getTime())) return iso;
    return d.toLocaleString("zh-CN", {
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    });
  } catch {
    return iso;
  }
}

function stateLabel(state: string): string {
  return STATE_LABELS[state] || state;
}

function stageLabel(stage: string): string {
  return STAGE_LABELS[stage] || stage;
}

function eventLabel(eventType: string): string {
  return EVENT_LABELS[eventType] || eventType;
}

function stateBadgeClass(state: string): string {
  return `state-badge state-badge--${state.toLowerCase().replace("_", "-")}`;
}

// ---- component ----

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
          if (p.state === "COMPLETED") {
            void loadStorages();
            void loadGroups("", appliedStorageFilter, appliedMinimumBytes);
          }
        }
      } catch {
        clearInterval(intervalId);
        setActiveJobId(null);
        setCancelling(false);
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
        {/* Project panel */}
        <section className="card project-card">
          <h2>项目</h2>
          {projectOpen ? (
            <div className="project-info">
              <p>
                <strong>数据库：</strong>
                <span className="project-path">{project!.path}</span>
              </p>
              <p>
                <strong>存储数量：</strong>
                {project!.storage_count}
                {isReadWrite && <span className="mode-indicator">读写模式</span>}
              </p>
              <div className="button-row">
                <button disabled={busy} onClick={handleRefreshProject}>刷新</button>
                <button className="secondary" disabled={busy} onClick={handleCloseProject}>关闭项目</button>
              </div>
            </div>
          ) : (
            <div className="project-open">
              <p className="muted">
                {readWriteMode
                  ? "读写模式：可创建新数据库、执行扫描；首次打开会自动建表迁移。"
                  : "只读模式：不会创建或迁移数据库，仅查询已有数据。"}
              </p>
              <div className="path-row">
                <input
                  aria-label="项目数据库路径"
                  value={projectPath}
                  onChange={(event) => setProjectPath(event.target.value)}
                  placeholder="/path/to/project.db"
                />
              </div>
              <label className="mode-toggle">
                <input
                  type="checkbox"
                  checked={readWriteMode}
                  onChange={(event) => setReadWriteMode(event.target.checked)}
                />
                读写模式（可扫描）
              </label>
              <button disabled={busy || !projectPath.trim()} onClick={handleOpenProject}>
                {readWriteMode ? "读写打开" : "只读打开"}
              </button>
            </div>
          )}
          {error && <p className="error" role="alert">{error}</p>}
        </section>

        {projectOpen && (
          <>
            {/* Scan panel — only in read-write mode */}
            {isReadWrite && (
              <section className="card card--full scan-panel">
                <div className="card-header-row">
                  <h2>扫描</h2>
                  {scanActive && scanProgress && (
                    <span className={stateBadgeClass(scanProgress.state)}>
                      {stateLabel(scanProgress.state)}
                    </span>
                  )}
                </div>

                {/* Scan form */}
                <div className="scan-form" aria-label="扫描参数">
                  <label>
                    根目录
                    <input
                      type="text"
                      value={scanRoot}
                      onChange={(e) => setScanRoot(e.target.value)}
                      placeholder="/path/to/scan"
                      disabled={scanActive}
                    />
                  </label>
                  <label>
                    存储 ID（可选）
                    <input
                      type="text"
                      value={scanStorageId}
                      onChange={(e) => setScanStorageId(e.target.value)}
                      placeholder="default"
                      disabled={scanActive}
                    />
                  </label>
                  <label>
                    并发数（可选）
                    <input
                      type="number"
                      min="1"
                      max="32"
                      value={scanWorkers}
                      onChange={(e) => setScanWorkers(e.target.value)}
                      placeholder="4"
                      disabled={scanActive}
                    />
                  </label>
                  <label className="checkbox-label">
                    <input
                      type="checkbox"
                      checked={scanFullScan}
                      onChange={(e) => setScanFullScan(e.target.checked)}
                      disabled={scanActive}
                    />
                    全量扫描
                  </label>
                  <button
                    className="btn-sm"
                    disabled={scanActive || scanStarting}
                    onClick={handleStartScan}
                  >
                    {scanStarting ? "启动中…" : "开始扫描"}
                  </button>
                </div>
                {scanError && <p className="error" role="alert">{scanError}</p>}

                {/* Active scan progress */}
                {scanProgress && (
                  <div className="scan-progress">
                    <div className="progress-bar">
                      <div className="progress-bar-fill" style={{ width: `${progressPercent}%` }} />
                    </div>
                    <div className="progress-stats">
                      <span><strong>阶段：</strong>{stageLabel(scanProgress.stage)}</span>
                      <span><strong>已发现：</strong>{scanProgress.discovered.toLocaleString()}</span>
                      <span><strong>已处理：</strong>{scanProgress.processed.toLocaleString()}</span>
                      <span><strong>失败：</strong>{scanProgress.failed.toLocaleString()}</span>
                      {scanProgress.warning_count > 0 && (
                        <span className="warn"><strong>警告：</strong>{scanProgress.warning_count}</span>
                      )}
                      {scanProgress.error_code && (
                        <span className="error-text"><strong>错误：</strong>{scanProgress.error_code}</span>
                      )}
                    </div>
                    {scanActive && (
                      <button
                        className="btn-sm secondary"
                        disabled={cancelling}
                        onClick={handleCancelScan}
                      >
                        {cancelling ? "取消中…" : "取消扫描"}
                      </button>
                    )}
                  </div>
                )}

                {/* Job history */}
                <div className="job-history">
                  <h3>任务历史</h3>
                  {jobsError ? (
                    <p className="error" role="alert">{jobsError}</p>
                  ) : jobs.length === 0 ? (
                    <p className="muted">暂无任务记录</p>
                  ) : (
                    <div className="table-wrap">
                      <table className="data-table">
                        <thead>
                          <tr>
                            <th>任务 ID</th>
                            <th>类型</th>
                            <th>状态</th>
                            <th>阶段</th>
                            <th className="num">已发现</th>
                            <th className="num">已处理</th>
                            <th className="num">失败</th>
                            <th>创建时间</th>
                            <th>完成时间</th>
                            <th>操作</th>
                          </tr>
                        </thead>
                        <tbody>
                          {jobs.map((j) => (
                            <tr key={j.job_id}>
                              <td className="mono" title={j.job_id}>{shortHash(j.job_id)}</td>
                              <td>{j.job_type}</td>
                              <td>
                                <span className={stateBadgeClass(j.state)}>
                                  {stateLabel(j.state)}
                                </span>
                              </td>
                              <td>{stageLabel(j.stage)}</td>
                              <td className="num">{j.discovered ?? 0}</td>
                              <td className="num">{j.processed ?? 0}</td>
                              <td className="num">{j.failed ?? 0}</td>
                              <td className="muted">{formatDateTime(j.created_at)}</td>
                              <td className="muted">{formatDateTime(j.completed_at || "")}</td>
                              <td>
                                <button
                                  className="btn-sm"
                                  disabled={jobDetailLoading}
                                  onClick={() => handleSelectJob(j.job_id)}
                                >
                                  详情
                                </button>
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  )}
                </div>
              </section>
            )}

            {/* Storage list */}
            <section className="card card--full">
              <h2>存储列表</h2>
              {storagesError ? (
                <p className="error" role="alert">{storagesError}</p>
              ) : storages.length === 0 ? (
                <p className="muted">暂无已注册的存储</p>
              ) : (
                <div className="table-wrap">
                  <table className="data-table">
                    <thead>
                      <tr>
                        <th>ID</th>
                        <th>根路径</th>
                        <th>类型</th>
                        <th>注册时间</th>
                      </tr>
                    </thead>
                    <tbody>
                      {storages.map((s) => (
                        <tr key={s.id}>
                          <td className="mono">{s.id}</td>
                          <td className="path-cell">{s.root_path}</td>
                          <td>{s.kind}</td>
                          <td className="muted">{s.created_at || "—"}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </section>

            {/* Duplicate groups */}
            <section className="card card--full">
              <div className="card-header-row">
                <h2>重复文件组</h2>
                {totalCount > 0 && (
                  <span className="count-badge">共 {totalCount} 组</span>
                )}
              </div>
              <div className="filter-row" aria-label="重复组筛选">
                <label>
                  存储
                  <select
                    value={storageFilter}
                    onChange={(event) => setStorageFilter(event.target.value)}
                  >
                    <option value="">全部存储</option>
                    {storages.map((storage) => (
                      <option key={storage.id} value={storage.id}>{storage.id}</option>
                    ))}
                  </select>
                </label>
                <label>
                  最小可回收（MiB）
                  <input
                    type="number"
                    min="0"
                    step="1"
                    value={minReclaimableMiB}
                    onChange={(event) => setMinReclaimableMiB(event.target.value)}
                    placeholder="0"
                  />
                </label>
                <button
                  className="btn-sm"
                  disabled={groupsLoading}
                  onClick={handleApplyFilters}
                >
                  应用筛选
                </button>
              </div>
              {groupsError ? (
                <p className="error" role="alert">{groupsError}</p>
              ) : groups.length === 0 && !groupsLoading ? (
                <p className="muted">未检测到重复文件，或尚未扫描</p>
              ) : (
                <>
                  <div className="table-wrap">
                    <table className="data-table">
                      <thead>
                        <tr>
                          <th>SHA-256</th>
                          <th>存储</th>
                          <th className="num">文件大小</th>
                          <th className="num">路径数</th>
                          <th className="num">物理副本</th>
                          <th className="num">硬链接别名</th>
                          <th className="num">可回收空间</th>
                          <th>操作</th>
                        </tr>
                      </thead>
                      <tbody>
                        {groups.map((g) => {
                          const key = `${g.storage_id}/${g.sha256}`;
                          const isSelected =
                            selectedGroup &&
                            selectedGroup.sha256 === g.sha256 &&
                            selectedGroup.storage_id === g.storage_id;
                          return (
                            <tr
                              key={key}
                              className={isSelected ? "row-selected" : ""}
                            >
                              <td className="mono" title={g.sha256}>{shortHash(g.sha256)}</td>
                              <td className="mono">{g.storage_id}</td>
                              <td className="num">{formatBytes(g.size)}</td>
                              <td className="num">{g.path_count}</td>
                              <td className="num">{g.physical_copy_count}</td>
                              <td className="num">{g.hardlink_alias_count}</td>
                              <td className="num">{formatBytes(g.physical_reclaimable_bytes)}</td>
                              <td>
                                <button
                                  className="btn-sm"
                                  disabled={detailLoading}
                                  onClick={() => handleSelectGroup(g.storage_id, g.sha256)}
                                >
                                  详情
                                </button>
                              </td>
                            </tr>
                          );
                        })}
                      </tbody>
                    </table>
                  </div>
                  {nextCursor && (
                    <div className="load-more">
                      <button
                        disabled={groupsLoading}
                        onClick={handleLoadMore}
                      >
                        {groupsLoading ? "加载中…" : "加载更多"}
                      </button>
                    </div>
                  )}
                </>
              )}
            </section>

            {/* Group detail */}
            {(selectedGroup || detailLoading || detailError) && (
              <section className="card card--full group-detail">
                <div className="card-header-row">
                  <h2>组详情</h2>
                  <button className="btn-sm secondary" onClick={handleCloseDetail}>关闭</button>
                </div>
                {detailLoading ? (
                  <p className="muted">加载中…</p>
                ) : detailError ? (
                  <p className="error" role="alert">{detailError}</p>
                ) : selectedGroup ? (
                  <>
                    <div className="detail-summary">
                      <span><strong>SHA-256：</strong><span className="mono">{selectedGroup.sha256}</span></span>
                      <span><strong>存储：</strong><span className="mono">{selectedGroup.storage_id}</span></span>
                      <span><strong>文件大小：</strong>{formatBytes(selectedGroup.size)}</span>
                      <span><strong>路径数：</strong>{selectedGroup.path_count}</span>
                      <span><strong>物理副本：</strong>{selectedGroup.physical_copy_count}</span>
                      <span><strong>硬链接别名：</strong>{selectedGroup.hardlink_alias_count}</span>
                      <span><strong>可回收：</strong>{formatBytes(selectedGroup.physical_reclaimable_bytes)}</span>
                    </div>
                    <div className="table-wrap">
                      <table className="data-table">
                        <thead>
                          <tr>
                            <th>路径</th>
                            <th>文件名</th>
                            <th className="num">大小</th>
                            <th>修改时间</th>
                            <th>物理可靠</th>
                            <th>格式</th>
                          </tr>
                        </thead>
                        <tbody>
                          {(selectedGroup.files || []).map((f, i) => (
                            <tr key={`${f.path}-${i}`}>
                              <td className="path-cell" title={f.path}>{f.path}</td>
                              <td>{f.name}</td>
                              <td className="num">{formatBytes(f.size)}</td>
                              <td className="muted">{f.modified_at || "—"}</td>
                              <td>{f.physical_reliable ? "是" : "否"}</td>
                              <td className="muted">{f.format_kind || "—"}{f.format_mime ? ` / ${f.format_mime}` : ""}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </>
                ) : null}
              </section>
            )}

            {/* Job detail */}
            {(selectedJob || jobDetailLoading || jobDetailError) && (
              <section className="card card--full job-detail">
                <div className="card-header-row">
                  <h2>任务详情</h2>
                  <button className="btn-sm secondary" onClick={handleCloseJobDetail}>关闭</button>
                </div>
                {jobDetailLoading ? (
                  <p className="muted">加载中…</p>
                ) : jobDetailError ? (
                  <p className="error" role="alert">{jobDetailError}</p>
                ) : selectedJob ? (
                  <>
                    <div className="detail-summary">
                      <span><strong>任务 ID：</strong><span className="mono">{selectedJob.job_id}</span></span>
                      <span>
                        <strong>状态：</strong>
                        <span className={stateBadgeClass(selectedJob.state)}>
                          {stateLabel(selectedJob.state)}
                        </span>
                      </span>
                      <span><strong>阶段：</strong>{stageLabel(selectedJob.stage)}</span>
                      <span><strong>已发现：</strong>{selectedJob.discovered.toLocaleString()}</span>
                      <span><strong>已处理：</strong>{selectedJob.processed.toLocaleString()}</span>
                      <span><strong>失败：</strong>{selectedJob.failed.toLocaleString()}</span>
                      {selectedJob.warning_count > 0 && (
                        <span className="warn"><strong>警告：</strong>{selectedJob.warning_count}</span>
                      )}
                      {selectedJob.error_code && (
                        <span className="error-text"><strong>错误码：</strong>{selectedJob.error_code}</span>
                      )}
                      <span><strong>创建：</strong>{formatDateTime(selectedJob.created_at)}</span>
                      {selectedJob.started_at && (
                        <span><strong>开始：</strong>{formatDateTime(selectedJob.started_at)}</span>
                      )}
                      {selectedJob.completed_at && (
                        <span><strong>完成：</strong>{formatDateTime(selectedJob.completed_at)}</span>
                      )}
                    </div>
                    <h3>事件时间线</h3>
                    <div className="table-wrap">
                      <table className="data-table">
                        <thead>
                          <tr>
                            <th className="num">#</th>
                            <th>事件</th>
                            <th>阶段</th>
                            <th>状态</th>
                            <th>时间</th>
                            <th>载荷</th>
                          </tr>
                        </thead>
                        <tbody>
                          {(selectedJob.events || []).map((ev) => (
                            <tr key={ev.sequence}>
                              <td className="num">{ev.sequence}</td>
                              <td>{eventLabel(ev.event_type)}</td>
                              <td>{stageLabel(ev.stage)}</td>
                              <td>
                                <span className={stateBadgeClass(ev.state)}>
                                  {stateLabel(ev.state)}
                                </span>
                              </td>
                              <td className="muted">{formatDateTime(ev.created_at)}</td>
                              <td className="payload-cell">
                                {ev.payload && Object.keys(ev.payload).length > 0 ? (
                                  <pre className="payload-json">{JSON.stringify(ev.payload)}</pre>
                                ) : (
                                  <span className="muted">—</span>
                                )}
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </>
                ) : null}
              </section>
            )}
          </>
        )}
      </main>

      <footer className="app-footer">
        <p>NDG — NAS Data Governance · {isReadWrite ? "读写模式" : "只读 Alpha"}</p>
      </footer>
    </div>
  );
}
