// Capability model: derives what the user can do from project state.
// Frontend navigation and buttons read this object instead of guessing.

import type { AppRoute } from "./routes";

export type ProjectMode = "closed" | "read_only" | "read_write";

export interface AppCapabilities {
  project_open: boolean;
  project_mode: ProjectMode;
  can_scan: boolean;
  can_view_results: boolean;
  can_edit_reviews: boolean;
  can_approve_plans: boolean;
  can_execute_quarantine: boolean;
  can_execute_purge: boolean;
  recovery_lock_active: boolean;
  /** Disabled route → human-readable reason shown in tooltip / sidebar. */
  disabled_reasons: Partial<Record<AppRoute, string>>;
}

export function deriveCapabilities(opts: {
  projectOpen: boolean;
  isReadWrite: boolean;
  recoveryLockActive?: boolean;
}): AppCapabilities {
  const { projectOpen, isReadWrite } = opts;
  const recoveryLock = opts.recoveryLockActive ?? false;
  const mode: ProjectMode = !projectOpen
    ? "closed"
    : isReadWrite
      ? "read_write"
      : "read_only";

  const disabled_reasons: Partial<Record<AppRoute, string>> = {};

  if (!projectOpen) {
    disabled_reasons["scan-jobs"] = "请先打开项目";
    disabled_reasons["duplicate-results"] = "请先打开项目";
    disabled_reasons["governance-review"] = "请先打开项目";
    disabled_reasons["execution-center"] = "请先打开项目";
    disabled_reasons["audit-recovery"] = "请先打开项目";
  } else if (!isReadWrite) {
    // Read-only mode: scan history is viewable, but new scans and
    // execution are disabled. The scan-jobs page itself remains
    // accessible; the new-scan form checks can_scan.
    disabled_reasons["execution-center"] = "只读模式，无法执行写操作";
  }

  if (recoveryLock) {
    disabled_reasons["scan-jobs"] = "恢复锁激活中，请先处理未完成执行";
    disabled_reasons["execution-center"] = "恢复锁激活中，请先处理未完成执行";
  }

  return {
    project_open: projectOpen,
    project_mode: mode,
    can_scan: isReadWrite && !recoveryLock,
    can_view_results: projectOpen,
    can_edit_reviews: isReadWrite && !recoveryLock,
    can_approve_plans: isReadWrite && !recoveryLock,
    can_execute_quarantine: isReadWrite && !recoveryLock,
    can_execute_purge: isReadWrite && !recoveryLock,
    recovery_lock_active: recoveryLock,
    disabled_reasons,
  };
}

export function isRouteEnabled(
  route: AppRoute,
  caps: AppCapabilities,
): boolean {
  if (route in caps.disabled_reasons) return false;
  switch (route) {
    case "sources":
    case "settings":
      return true;
    case "scan-jobs":
    case "duplicate-results":
    case "governance-review":
    case "audit-recovery":
      return caps.project_open;
    case "execution-center":
      return caps.project_open && caps.project_mode === "read_write";
    default:
      return false;
  }
}
