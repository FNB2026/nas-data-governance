// Scan jobs page: new scan form, active progress, job history, job detail.
// Scan form state is page-local; scan status and polling are global (context).

import { useState } from "react";
import ScanPanel from "../components/ScanPanel";
import JobDetail from "../components/JobDetail";
import { useProject } from "../state/ProjectContext";
import { hasWailsRuntime, errorText } from "../lib/utils";
import { GetJobDetail } from "../wailsjs/go/wails/API";
import { wails } from "../wailsjs/go/models";

export default function ScanJobsPage() {
  const {
    activeJobId,
    scanProgress,
    cancelling,
    jobs,
    jobsError,
    startScan,
    cancelScan,
    capabilities,
  } = useProject();

  // Form state (page-local)
  const [scanRoot, setScanRoot] = useState("");
  const [scanStorageId, setScanStorageId] = useState("");
  const [scanFullScan, setScanFullScan] = useState(false);
  const [scanWorkers, setScanWorkers] = useState("");
  const [scanStarting, setScanStarting] = useState(false);
  const [scanError, setScanError] = useState<string | null>(null);

  // Job detail (page-local)
  const [selectedJob, setSelectedJob] = useState<wails.JobDetailResponse | null>(null);
  const [jobDetailLoading, setJobDetailLoading] = useState(false);
  const [jobDetailError, setJobDetailError] = useState<string | null>(null);

  const scanActive = activeJobId !== null;

  const handleStartScan = async () => {
    if (!scanRoot.trim()) {
      setScanError("请输入要扫描的根目录路径");
      return;
    }
    setScanStarting(true);
    setScanError(null);
    try {
      const workersNum = scanWorkers.trim() ? parseInt(scanWorkers, 10) : undefined;
      await startScan({
        root: scanRoot.trim(),
        storageId: scanStorageId.trim(),
        fullScan: scanFullScan,
        workers: Number.isFinite(workersNum) && workersNum! > 0 ? workersNum : undefined,
      });
    } catch (e: unknown) {
      setScanError(errorText(e));
    } finally {
      setScanStarting(false);
    }
  };

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
        jobsError={jobsError}
        jobDetailLoading={jobDetailLoading}
        canScan={capabilities.can_scan}
        onScanRootChange={setScanRoot}
        onScanStorageIdChange={setScanStorageId}
        onScanFullScanChange={setScanFullScan}
        onScanWorkersChange={setScanWorkers}
        onStartScan={() => void handleStartScan()}
        onCancelScan={() => void cancelScan()}
        onSelectJob={(jobId) => void handleSelectJob(jobId)}
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
