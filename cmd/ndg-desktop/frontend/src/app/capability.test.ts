import { describe, expect, it } from "vitest";
import { deriveCapabilities, isRouteEnabled } from "./capability";
import { NAV_ITEMS } from "./routes";

// ---- deriveCapabilities ----

describe("deriveCapabilities", () => {
  it("returns closed mode when project is not open", () => {
    const caps = deriveCapabilities({ projectOpen: false, isReadWrite: false });
    expect(caps.project_mode).toBe("closed");
    expect(caps.project_open).toBe(false);
    expect(caps.can_scan).toBe(false);
    expect(caps.can_view_results).toBe(false);
  });

  it("returns read_only mode when project is open but not read-write", () => {
    const caps = deriveCapabilities({ projectOpen: true, isReadWrite: false });
    expect(caps.project_mode).toBe("read_only");
    expect(caps.project_open).toBe(true);
    expect(caps.can_view_results).toBe(true);
    expect(caps.can_scan).toBe(false);
    expect(caps.can_edit_reviews).toBe(false);
  });

  it("returns read_write mode with full capabilities", () => {
    const caps = deriveCapabilities({ projectOpen: true, isReadWrite: true });
    expect(caps.project_mode).toBe("read_write");
    expect(caps.can_scan).toBe(true);
    expect(caps.can_edit_reviews).toBe(true);
    expect(caps.can_approve_plans).toBe(true);
    expect(caps.can_execute_quarantine).toBe(true);
    expect(caps.can_execute_purge).toBe(true);
  });

  it("disables scan and execution when recovery lock is active", () => {
    const caps = deriveCapabilities({
      projectOpen: true,
      isReadWrite: true,
      recoveryLockActive: true,
    });
    expect(caps.recovery_lock_active).toBe(true);
    expect(caps.can_scan).toBe(false);
    expect(caps.can_execute_quarantine).toBe(false);
    expect(caps.can_execute_purge).toBe(false);
    expect(caps.can_edit_reviews).toBe(false);
  });

  it("disables all task routes when project is closed", () => {
    const caps = deriveCapabilities({ projectOpen: false, isReadWrite: false });
    expect(caps.disabled_reasons["scan-jobs"]).toBeTruthy();
    expect(caps.disabled_reasons["duplicate-results"]).toBeTruthy();
    expect(caps.disabled_reasons["governance-review"]).toBeTruthy();
    expect(caps.disabled_reasons["execution-center"]).toBeTruthy();
    expect(caps.disabled_reasons["audit-recovery"]).toBeTruthy();
  });

  it("disables execution-center in read-only mode", () => {
    const caps = deriveCapabilities({ projectOpen: true, isReadWrite: false });
    expect(caps.disabled_reasons["execution-center"]).toContain("只读模式");
    // Other routes should still be enabled
    expect(caps.disabled_reasons["scan-jobs"]).toBeUndefined();
    expect(caps.disabled_reasons["duplicate-results"]).toBeUndefined();
  });

  it("overrides scan-jobs and execution-center with recovery lock message", () => {
    const caps = deriveCapabilities({
      projectOpen: true,
      isReadWrite: true,
      recoveryLockActive: true,
    });
    expect(caps.disabled_reasons["scan-jobs"]).toContain("恢复锁");
    expect(caps.disabled_reasons["execution-center"]).toContain("恢复锁");
  });

  it("defaults recoveryLockActive to false when omitted", () => {
    const caps = deriveCapabilities({ projectOpen: true, isReadWrite: true });
    expect(caps.recovery_lock_active).toBe(false);
  });
});

// ---- isRouteEnabled ----

describe("isRouteEnabled", () => {
  it("always enables sources and settings", () => {
    const caps = deriveCapabilities({ projectOpen: false, isReadWrite: false });
    expect(isRouteEnabled("sources", caps)).toBe(true);
    expect(isRouteEnabled("settings", caps)).toBe(true);
  });

  it("disables task routes when project is closed", () => {
    const caps = deriveCapabilities({ projectOpen: false, isReadWrite: false });
    expect(isRouteEnabled("scan-jobs", caps)).toBe(false);
    expect(isRouteEnabled("duplicate-results", caps)).toBe(false);
    expect(isRouteEnabled("governance-review", caps)).toBe(false);
    expect(isRouteEnabled("execution-center", caps)).toBe(false);
    expect(isRouteEnabled("audit-recovery", caps)).toBe(false);
  });

  it("enables view routes in read-only mode", () => {
    const caps = deriveCapabilities({ projectOpen: true, isReadWrite: false });
    expect(isRouteEnabled("scan-jobs", caps)).toBe(true);
    expect(isRouteEnabled("duplicate-results", caps)).toBe(true);
    expect(isRouteEnabled("governance-review", caps)).toBe(true);
    expect(isRouteEnabled("audit-recovery", caps)).toBe(true);
    // Execution center requires read_write
    expect(isRouteEnabled("execution-center", caps)).toBe(false);
  });

  it("enables all routes in read-write mode", () => {
    const caps = deriveCapabilities({ projectOpen: true, isReadWrite: true });
    for (const item of NAV_ITEMS) {
      expect(isRouteEnabled(item.id, caps)).toBe(true);
    }
  });

  it("disables scan-jobs when recovery lock is active", () => {
    const caps = deriveCapabilities({
      projectOpen: true,
      isReadWrite: true,
      recoveryLockActive: true,
    });
    // scan-jobs is in disabled_reasons, so isRouteEnabled returns false
    expect(isRouteEnabled("scan-jobs", caps)).toBe(false);
    expect(isRouteEnabled("execution-center", caps)).toBe(false);
    // Other view routes remain enabled
    expect(isRouteEnabled("duplicate-results", caps)).toBe(true);
    expect(isRouteEnabled("audit-recovery", caps)).toBe(true);
  });
});
