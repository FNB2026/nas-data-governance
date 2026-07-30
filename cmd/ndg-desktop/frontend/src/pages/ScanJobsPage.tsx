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
    resumeScan,
    cancelScan,
    capabilities,
    defaultFullScan,
    defaultWorkers,
    pendingScanRoot,
    clearPendingScanRoot,
  } = useProject();

  // Form state (page-local) — initialized from global scan defaults
  const [scanRoot, setScanRoot] = useState("");
  const [scanFullScan, setScanFullScan] = useState(defaultFullScan);
  const [scanWorkers, setScanWorkers] = useState(defaultWorkers);
  const [scanStarting, setScanStarting] = useState(false);
  const [scanError, setScanError] = useState<string | null>(null);

  // Checkpoint info: checked when scanRoot changes or a scan terminates.
  // Drives the visibility of the "继续扫描" (Continue Scan) button.
  const [checkpointInfo, setCheckpointInfo] = useState<wails.ScanCheckpointDTO | null>(null);

  // Derived: whether a scan is currently active (polled globally in context).
  const scanActive = activeJobId !== null;

  // Consume a pending scan root set by the start card's "new project"
  // flow: prefill the root, then clear it so it only applies once.
  useEffect(() => {
    if (!pendingScanRoot) return;
    setScanRoot(pendingScanRoot);
    setScanError(null);
    clearPendingScanRoot();
  }, [pendingScanRoot, clearPendingScanRoot]);

  // Check for a resumable checkpoint whenever the scan root changes or
  // a scan terminates (scanActive goes false). Skips the check while a
  // scan is active — the resume button is hidden then anyway.
  useEffect(() => {
    if (!hasWailsRuntime() || !scanRoot.trim() || scanActive) {
      setCheckpointInfo(null);
      return;
    }
    let cancelled = false;
    api.scan.getCheckpoint(scanRoot.trim())
      .then((info) => {
        if (!cancelled) setCheckpointInfo(info);
      })
      .catch(() => {
        if (!cancelled) setCheckpointInfo(null);
      });
    return () => { cancelled = true; };
  }, [scanRoot, scanActive]);

  // Job detail (page-local)
  const [selectedJob, setSelectedJob] = useState<wails.JobDetailResponse | null>(null);
  const [jobDetailLoading, setJobDetailLoading] = useState(false);
  const [jobDetailError, setJobDetailError] = useState<string | null>(null);

  const handleRegisteredRootSelect = (storage: wails.StorageInfo) => {
    setScanRoot(storage.root_path);
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

  const handleResumeScan = async () => {
    if (!scanRoot.trim()) {
      setScanError("请输入要扫描的根目录路径");
      return;
    }
    setScanStarting(true);
    setScanError(null);
    try {
      const workersNum = scanWorkers.trim() ? parseInt(scanWorkers, 10) : undefined;
      await resumeScan(
        scanRoot.trim(),
        Number.isFinite(workersNum) && workersNum! > 0 ? workersNum : undefined,
      );
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
        canResumeScan={checkpointInfo?.available ?? false}
        resumeCheckpointCount={checkpointInfo?.scanned_count ?? 0}
        stateFilter={scanFilterState}
        typeFilter={scanFilterType}
        onScanRootChange={setScanRoot}
        onRegisteredRootSelect={handleRegisteredRootSelect}
        onScanFullScanChange={setScanFullScan}
        onScanWorkersChange={setScanWorkers}
        onStartScan={() => void handleStartScan()}
        onCancelScan={() => void cancelScan()}
        onSelectJob={(jobId) => void handleSelectJob(jobId)}
        onStateFilterChange={setScanFilterState}
        onTypeFilterChange={setScanFilterType}
        onLoadMoreJobs={() => void loadMoreJobs()}
        onRetryScan={() => void handleRetryScan()}
        onResumeScan={() => void handleResumeScan()}
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
