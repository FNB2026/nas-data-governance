import { describe, expect, it } from "vitest";
import { recoveryFeedback, type RecoveryKind } from "./recoveryFeedback";
import type { wails } from "../wailsjs/go/models";
const clear = { lock_active: false, executing_count: 0 } as wails.RecoveryStatusDTO;
const locked = { lock_active: true, executing_count: 1 } as wails.RecoveryStatusDTO;

describe("recovery feedback terminal contract", () => {
  it.each([
    ["source", { action: "rolled_back" }],
    ["restore", { status: "ok", final_state: "ROLLED_BACK" }],
    ["purge", { status: "ok", final_state: "ROLLED_BACK" }],
    ["purge", { status: "ok", final_state: "PURGED" }],
  ] as const)("confirms %s only with a fresh clear lock", (kind, result) => {
    expect(recoveryFeedback(kind, [result], clear).tone).toBe("success");
    for (const lock of [locked, null]) {
      const feedback = recoveryFeedback(kind, [result], lock);
      expect(feedback.tone).toBe("warning");
      expect(feedback.summary).toContain("禁止新的文件写入");
    }
  });
  it("requires fresh review for a draft reset instead of claiming rollback", () => {
    const f = recoveryFeedback("source", [{ action: "reset_to_draft" }], clear);
    expect(f.tone).toBe("warning");
    expect(f.summary).toContain("旧审批不可复用");
    expect(f.summary).toContain("确认回滚 0 条");
  });
  it.each(["source", "restore", "purge"] as RecoveryKind[])("does not count empty %s results as work", (kind) => {
    expect(recoveryFeedback(kind, [], clear).tone).toBe("info");
    expect(recoveryFeedback(kind, [], clear).summary).toContain("无需恢复");
    expect(recoveryFeedback(kind, [], locked).tone).toBe("warning");
  });
  it.each([
    ["source", { action: "skipped" }],
    ["source", { action: "reset_to_approved" }],
    ["source", { action: "rolled_back", errors: ["PRIVATE_CANARY /private/secret.dat"] }],
    ["restore", { status: "ok" }],
    ["restore", { status: "ok", final_state: "RESTORED" }],
    ["restore", { status: "ok", final_state: "ROLLED_BACK", error: "rollback PRIVATE_CANARY" }],
    ["purge", { status: "ok", final_state: "PURGED", error_type: "PRIVATE_CANARY" }],
    ["purge", { status: "failed", final_state: "ROLLED_BACK" }],
  ] as const)("blocks unknown or conflicting %s results without raw errors", (kind, result) => {
    const f = recoveryFeedback(kind, [result], clear);
    expect(f.tone).toBe("warning");
    expect(f.summary).toContain("未完成 1 条");
    expect(f.summary).toContain("人工核对");
    expect(JSON.stringify(f)).not.toMatch(/PRIVATE_CANARY|secret.dat/);
  });
  it("keeps mixed counts separate", () => {
    const f = recoveryFeedback("source", [{ action: "rolled_back" }, { action: "skipped" }, { action: "reset_to_draft" }], locked);
    expect(f.summary).toContain("检查 3 条，确认回滚 1 条，退回草案 1 条，确认提交 0 条，未完成 1 条");
    expect(f.tone).toBe("warning");
  });
  it("does not trust an inconsistent clear lock", () => {
    const inconsistent = { ...clear, restore_pending_count: 1 } as wails.RecoveryStatusDTO;
    expect(recoveryFeedback("source", [{ action: "rolled_back" }], inconsistent).tone).toBe("warning");
  });
});
