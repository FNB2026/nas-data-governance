import type { ToastType } from "../components/Toast";
import type { wails } from "../wailsjs/go/models";

export type RecoveryKind = "source" | "restore" | "purge";
type Result = { action?: string; errors?: readonly string[]; status?: string; final_state?: string; error?: string; error_type?: string };
const LABELS = { source: "普通执行", restore: "隔离还原", purge: "永久清理" };
export const RECOVERY_REQUEST_FAILED = "恢复请求未获确认，请人工核对；不要重复文件写入或清除现场。";

export function isRecoveryLockClear(lock: wails.RecoveryStatusDTO | null): boolean {
  return lock?.lock_active === false && lock.executing_count === 0 &&
    (lock.source_executing_count ?? 0) === 0 && (lock.restore_pending_count ?? 0) === 0 && (lock.purge_recoverable_count ?? 0) === 0;
}

/** Only established terminal outcomes are trusted. Raw errors and IDs may
 * contain paths or business markers; never interpolate them into feedback. */
export function recoveryFeedback(kind: RecoveryKind, results: readonly Result[], lock: wails.RecoveryStatusDTO | null) {
  let rolledBack = 0;
  let review = 0;
  let committed = 0;
  let unresolved = 0;
  const lines = results.map((r, i) => {
    let detail = "安全阻断：结果未确认，请人工核对；保留现场。";
    const errors = !!r.errors?.length || !!r.error || !!r.error_type;
    if (!errors && kind === "source" && r.action === "reset_to_draft") {
      review++;
      detail = "已退回草案：旧审批不可复用，需重新审查和审批。";
    } else if (!errors && (kind === "source" ? r.action === "rolled_back" : r.status === "ok" && r.final_state === "ROLLED_BACK")) {
      rolledBack++;
      detail = "已确认回滚。";
    } else if (!errors && kind === "purge" && r.status === "ok" && r.final_state === "PURGED") {
      committed++;
      detail = "已确认既有清理提交终态。";
    } else {
      unresolved++;
    }
    return `第 ${i + 1} 条：${detail}`;
  });
  const lockClear = isRecoveryLockClear(lock);
  const lockText = lockClear ? "恢复锁已解除。" : lock?.lock_active
    ? "恢复锁仍激活：仍有未解决状态，禁止新的文件写入，请人工核对。"
    : "无法确认恢复锁已解除：禁止新的文件写入，请人工核对。";
  const counts = `检查 ${results.length} 条，确认回滚 ${rolledBack} 条，退回草案 ${review} 条，确认提交 ${committed} 条，未完成 ${unresolved} 条。`;
  const summary = [results.length ? counts : `${LABELS[kind]}：无需恢复（本类型无恢复任务）。`, ...lines, lockText].join("\n");
  let tone: ToastType = "warning";
  let title = `${LABELS[kind]}安全阻断`;
  if (lockClear && unresolved === 0) {
    if (results.length === 0) { tone = "info"; title = "无需恢复"; }
    else if (review > 0) { title = "需要重新审查"; }
    else { tone = "success"; title = `${LABELS[kind]}恢复已确认`; }
  }
  return { tone, title, summary };
}
