// Centralized API client: wraps all Wails backend bindings with
// unified retry logic (network drive tolerance) and friendly error
// messages. Pages import from here instead of raw Wails bindings.
//
// Usage:
//   import { api } from "../api/client";
//   const plans = await api.governance.listAll();
//   // errors are already ApiError with friendly messages
//   } catch (e) { setError((e as Error).message); }

import {
  ApprovePlans,
  ApprovePurgePlan,
  ApproveRestorePlan,
  BuildDraftPlans,
  CancelScan,
  CheckRecoveryLock,
  CloseProject,
  CreatePurgePlans,
  CreateRestorePlan,
  DiagnoseFormats,
  DiagnoseGovernance,
  DiagnoseMerges,
  ExecutePlans,
  ExecutePurge,
  ExecuteRestore,
  GetAppCapabilities,
  GetGroupDecision,
  GetGroupDetail,
  GetJobDetail,
  GetProjectInfo,
  GetProjectReadiness,
  GetScanProgress,
  GetVersion,
  ListAllPlans,
  ListDuplicateGroups,
  ListGroupDecisions,
  ListJournalEntries,
  ListOperationLogs,
  ListPurgePlans,
  ListQuarantineItems,
  ListRecentJobs,
  ListRestorePlans,
  ListReviewPlans,
  ListStorages,
  OpenProject,
  OpenProjectReadWrite,
  RecoverPurges,
  RecoverRestores,
  RecoverSourcePlans,
  SaveDraftPlans,
  SaveGroupDecision,
  StartScan,
  ValidateProjectPath,
} from "../wailsjs/go/wails/API";
import { wails, formatdiag, governancediag, merge } from "../wailsjs/go/models";
import { friendlyError } from "../lib/utils";
import { retryWithBackoff } from "../lib/retry";

// ---- ApiError ----

/**
 * Error thrown by the API client. The `message` is already a user-friendly
 * string (via friendlyError), so catch blocks can use it directly without
 * calling friendlyError again.
 */
export class ApiError extends Error {
  readonly raw: unknown;

  constructor(raw: unknown) {
    super(friendlyError(raw));
    this.name = "ApiError";
    this.raw = raw;
  }
}

// ---- Internal helpers ----

interface CallOptions {
  /** Max retry attempts for network errors. Set to 0 to disable retry. */
  retries?: number;
}

/**
 * Wraps a Wails API call with retry and friendly error conversion.
 * Default: 3 retries with exponential backoff (network errors only).
 */
async function call<T>(fn: () => Promise<T>, opts: CallOptions = {}): Promise<T> {
  const retries = opts.retries ?? 3;
  try {
    if (retries > 0) {
      return await retryWithBackoff(fn, { maxRetries: retries });
    }
    return await fn();
  } catch (e) {
    throw new ApiError(e);
  }
}

/**
 * Wraps a Wails API call WITHOUT retry. Use for frequently-polled
 * operations (e.g., GetScanProgress, CheckRecoveryLock) where the
 * caller manages its own retry/timing logic.
 */
async function callOnce<T>(fn: () => Promise<T>): Promise<T> {
  try {
    return await fn();
  } catch (e) {
    throw new ApiError(e);
  }
}

// ---- API client ----

export const api = {
  // ---- Project lifecycle ----
  project: {
    open: (path: string): Promise<wails.ProjectInfo> =>
      call(() => OpenProject(path)),
    openReadWrite: (path: string): Promise<wails.ProjectInfo> =>
      call(() => OpenProjectReadWrite(path)),
    close: (): Promise<void> =>
      call(() => CloseProject()),
    info: (): Promise<wails.ProjectInfo> =>
      call(() => GetProjectInfo()),
    validatePath: (path: string): Promise<void> =>
      call(() => ValidateProjectPath(path)),
    version: (): Promise<wails.VersionInfo> =>
      call(() => GetVersion()),
  },

  // ---- Scan operations ----
  scan: {
    start: (req: wails.StartScanRequest): Promise<wails.StartScanResponse> =>
      call(() => StartScan(req)),
    cancel: (jobId: string): Promise<void> =>
      call(() => CancelScan(jobId)),
    /** No retry — polling callers manage their own retry. */
    getProgress: (jobId: string): Promise<wails.ScanJobProgress> =>
      callOnce(() => GetScanProgress(jobId)),
    listJobs: (limit: number): Promise<wails.JobSummary[]> =>
      call(() => ListRecentJobs(limit), { retries: 3 }),
    getJobDetail: (jobId: string): Promise<wails.JobDetailResponse> =>
      call(() => GetJobDetail(jobId)),
  },

  // ---- Storage ----
  storages: {
    list: (): Promise<wails.StorageInfo[]> =>
      call(() => ListStorages(), { retries: 3 }),
  },

  // ---- Duplicate results ----
  duplicates: {
    listGroups: (req: wails.ListGroupsRequest): Promise<wails.ListGroupsResponse> =>
      call(() => ListDuplicateGroups(req)),
    getDetail: (storageId: string, sha256: string): Promise<wails.GroupDetailResponse> =>
      call(() => GetGroupDetail(storageId, sha256)),
  },

  // ---- Governance review ----
  governance: {
    buildDrafts: (storageId: string): Promise<wails.PlanDTO[]> =>
      call(() => BuildDraftPlans(storageId)),
    saveDrafts: (storageId: string): Promise<wails.PlanDTO[]> =>
      call(() => SaveDraftPlans(storageId)),
    listAll: (): Promise<wails.PlanDTO[]> =>
      call(() => ListAllPlans()),
    listReview: (): Promise<wails.PlanDTO[]> =>
      call(() => ListReviewPlans()),
    approve: (req: wails.ApprovePlansRequest): Promise<wails.ApprovePlansResponse> =>
      call(() => ApprovePlans(req)),
    listDecisions: (groupId: string): Promise<wails.GroupDecisionDTO[]> =>
      call(() => ListGroupDecisions(groupId)),
    getDecision: (groupId: string): Promise<wails.GroupDecisionDTO> =>
      call(() => GetGroupDecision(groupId)),
    saveDecision: (req: wails.SaveDecisionRequest): Promise<wails.GroupDecisionDTO> =>
      call(() => SaveGroupDecision(req)),
  },

  // ---- Execution center ----
  execution: {
    executePlans: (req: wails.ExecutePlansRequest): Promise<wails.ExecutePlansResponse> =>
      call(() => ExecutePlans(req)),
    listQuarantine: (statusFilter: string): Promise<wails.QuarantineItemDTO[]> =>
      call(() => ListQuarantineItems(statusFilter)),
    createRestorePlan: (itemId: string): Promise<wails.RestorePlanDTO> =>
      call(() => CreateRestorePlan(itemId)),
    approveRestore: (planId: string, digest: string): Promise<void> =>
      call(() => ApproveRestorePlan(planId, digest)),
    executeRestore: (req: wails.ExecuteRestoreRequest): Promise<wails.ExecuteRestoreResponse> =>
      call(() => ExecuteRestore(req)),
    listRestores: (): Promise<wails.RestorePlanDTO[]> =>
      call(() => ListRestorePlans()),
    createPurgePlans: (): Promise<wails.PurgePlanDTO[]> =>
      call(() => CreatePurgePlans()),
    approvePurge: (planId: string, digest: string): Promise<void> =>
      call(() => ApprovePurgePlan(planId, digest)),
    executePurge: (req: wails.ExecutePurgeRequest): Promise<wails.ExecutePurgeResponse> =>
      call(() => ExecutePurge(req)),
    listPurges: (): Promise<wails.PurgePlanDTO[]> =>
      call(() => ListPurgePlans()),
  },

  // ---- Recovery ----
  recovery: {
    /** No retry — polled frequently, caller manages retry. */
    checkLock: (): Promise<wails.RecoveryStatusDTO> =>
      callOnce(() => CheckRecoveryLock()),
    recoverSource: (): Promise<wails.RecoveryResultDTO[]> =>
      call(() => RecoverSourcePlans()),
    recoverRestores: (req: wails.RecoverRestoresRequest): Promise<wails.RestoreRecoveryResultDTO[]> =>
      call(() => RecoverRestores(req)),
    recoverPurges: (quarantineRoot: string): Promise<wails.PurgeRecoveryResultDTO[]> =>
      call(() => RecoverPurges(quarantineRoot)),
  },

  // ---- Audit ----
  audit: {
    listLogs: (planFilter: string): Promise<wails.OperationLogDTO[]> =>
      call(() => ListOperationLogs(planFilter)),
    listJournal: (planFilter: string): Promise<wails.JournalEntryDTO[]> =>
      call(() => ListJournalEntries(planFilter)),
  },

  // ---- Diagnostics ----
  diagnostics: {
    formats: (req: wails.DiagnoseFormatsRequest): Promise<formatdiag.Report> =>
      call(() => DiagnoseFormats(req)),
    governance: (req: wails.DiagnoseGovernanceRequest): Promise<governancediag.Report> =>
      call(() => DiagnoseGovernance(req)),
    merges: (req: wails.DiagnoseMergesRequest): Promise<merge.DiagnosticReport> =>
      call(() => DiagnoseMerges(req)),
  },

  // ---- Capabilities & readiness ----
  capabilities: {
    get: (): Promise<wails.AppCapabilitiesDTO> =>
      call(() => GetAppCapabilities()),
    readiness: (): Promise<wails.ProjectReadinessDTO> =>
      call(() => GetProjectReadiness()),
  },
} as const;

// ---- Type re-exports for convenience ----

export type { wails, formatdiag, governancediag, merge };
