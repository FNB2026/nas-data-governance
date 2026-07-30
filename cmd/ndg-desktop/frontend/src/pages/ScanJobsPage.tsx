// Scan jobs page: new scan form, active progress, job history, job detail.
// Scan form state is page-local; scan status and polling are global (context).

import { useState, useEffect } from "react";
import ScanPanel from "../components/ScanPanel";
import JobDetail from "../components/JobDetail";
import { useProject } from "../state/ProjectContext";
import { hasWailsRuntime } from "../lib/utils";
import { wails } from "../wailsjs/go/models";
import { api } from "../api/client";

export default function ScanJobsPage() {
  const {
    activeJobId,
    scanProgress,
    cancelling,
    canRetryScan,
    jobs,
    storages,
    jobsError,
    hasMoreJobs,
    loadMoreJobs,
    scanFilterState,
    scanFilterType,
    setScanFilterState,
    setScanFilterType,
    startScan,
    retryLastScan,
    cancelScan,
    capabilities,
    defaultFullScan,
    defaultWorkers,
    pendingScanRoot,
    clearPendingScanRoot,
  } = useProject();

  // Form state (page-local) — initialized from global scan defaults
  const [scanRoot, setScanRoot] = useState("");
  const [scanStorageId, setScanStorageId] = useState("");
  const [scanFullScan, setScanFullScan] = useState(defaultFullScan);
  const [scanWorkers, setScanWorkers] = useState(defaultWorkers);
  const [scanStarting, setScanStarting] = useState(false);
  const [scanError, setScanError] = useState<string | null>(null);

  // Consume a pending scan root set by the start card's "new project"
  // flow: prefill the root and the default storage id, then clear it so
  // it only applies once. Also auto-select the registered storage when a
  // pending root matches a known storage.
  useEffect(() => {
    if (!pendingScanRoot) return;
    setScanRoot(pendingScanRoot);
    setScanError(null);
    clearPendingScanRoot();
  }, [pendingScanRoot, clearPendingScanRoot]);

  // Auto-select the storage whose root matches the prefilled scan root,
  // so the user can hit "开始扫描" without picking a storage manually.
  useEffect(() => {
    if (!scanRoot) return;
    const match = storages.find((s) => s.root_path === scanRoot);
    if (match) {
      setScanStorageId(match.id);
    }
  }, [scanRoot, storages]);

  // Job detail (page-local)
  const [selectedJob, setSelectedJob] = useState<wails.JobDetailResponse | null>(null);
  const [jobDetailLoading, setJobDetailLoading] = useState(false);
  const [jobDetailError, setJobDetailError] = useState<string | null>(null);

  const scanActive = activeJobId !== null;

  const handleRegisteredRootSelect = (storage: wails.StorageInfo) => {
    setScanRoot(storage.root_path);
    setScanStorageId(storage.id);
    setScanError(null);
  };

  const handleStartScan = async () => {
    if (!scanRoot.trim()) {
      setScanError("请输入要扫描的根目录路径");
      return;
    }
    setScanStarting(true);
    setScanError(null);
    try {
      const workersNum = scanWorkers.trim() ? parseInt(scanWorkers, 10) : undefined;
      const params = {
        root: scanRoot.trim(),
        storageId: scanStorageId.trim(),
        fullScan: scanFullScan,
        workers: Number.isFinite(workersNum) && workersNum! > 0 ? workersNum : undefined,
      };
      await startScan(params);
    } catch (e: unknown) {
      setScanError((e as Error).message);
    } finally {
      setScanStarting(false);
    }
  };

  const handleRetryScan = async () => {
    setScanStarting(true);
    setScanError(null);
    try {
      await retryLastScan();
    } catch (e: unknown) {
      setScanError((e as Error).message);
    } finally {
      setScanStarting(false);
    }
  };

  const handleSelectJob = async (jobId: string) => {
    if (!hasWailsRuntime()) return;
    setJobDetailLoading(true);
    setJobDetailError(null);
    try {
      const detail = await api.scan.getJobDetail(jobId);
      setSelectedJob(detail);
    } catch (e: unknown) {
      setJobDetailError((e as Error).message);
      setSelectedJob(null);
    } finally {
      setJobDetailLoading(false);
    }
  };

  const handleCloseJobDetail = () => {
    setSelectedJob(null);
    setJobDetailError(null);
  };

  return (
    <div className="page page--scan-jobs">
      <div className="page-header">
        <h2>扫描任务</h2>
        <p className="muted">新建扫描、进度与历史</p>
      </div>

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
        jobs={jobs}
        storages={storages}
        jobsError={jobsError}
        jobDetailLoading={jobDetailLoading}
        hasMoreJobs={hasMoreJobs}
        canScan={capabilities.can_scan}
        canRetryScan={canRetryScan}
        stateFilter={scanFilterState}
        typeFilter={scanFilterType}
        onScanRootChange={setScanRoot}
        onRegisteredRootSelect={handleRegisteredRootSelect}
        onScanStorageIdChange={setScanStorageId}
        onScanFullScanChange={setScanFullScan}
        onScanWorkersChange={setScanWorkers}
        onStartScan={() => void handleStartScan()}
        onCancelScan={() => void cancelScan()}
        onSelectJob={(jobId) => void handleSelectJob(jobId)}
        onStateFilterChange={setScanFilterState}
        onTypeFilterChange={setScanFilterType}
        onLoadMoreJobs={() => void loadMoreJobs()}
        onRetryScan={() => void handleRetryScan()}
      />

      <JobDetail
        selectedJob={selectedJob}
        jobDetailLoading={jobDetailLoading}
        jobDetailError={jobDetailError}
        onClose={handleCloseJobDetail}
      />
    </div>
  );
}
