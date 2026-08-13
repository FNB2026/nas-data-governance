// Shared constants and utility functions for the NDG desktop frontend.

export const TERMINAL_STATES = new Set(["PAUSED_NETWORK", "COMPLETED", "FAILED", "CANCELLED"]);

export const STAGE_LABELS: Record<string, string> = {
  DISCOVERING: "发现文件",
  METADATA_INDEXING: "索引元数据",
  QUICK_HASHING: "快速哈希",
  FULL_HASHING: "完整哈希",
  CONTEXT_CLASSIFYING: "上下文分类",
  FORMAT_ANALYZING: "格式分析",
  GROUPING: "分组",
  PLANNING: "规划",
  FINALIZING: "保存索引与收尾",
};

export const STATE_LABELS: Record<string, string> = {
  QUEUED: "排队中",
  RUNNING: "运行中",
  CANCEL_REQUESTED: "取消中",
  PAUSED_NETWORK: "网络中断，已暂停",
  COMPLETED: "已完成",
  FAILED: "已失败",
  CANCELLED: "已取消",
};

export const EVENT_LABELS: Record<string, string> = {
  "job:created": "创建",
  "job:stage": "阶段切换",
  "job:progress": "进度更新",
  "job:warning": "警告",
  "job:paused_network": "网络中断暂停",
  "job:completed": "完成",
  "job:failed": "失败",
  "job:cancelled": "取消",
};

export function hasWailsRuntime(): boolean {
  return typeof window !== "undefined" && "go" in window && "runtime" in window;
}

export function errorText(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

/**
 * Maps raw backend errors to user-friendly messages with actionable hints.
 * Returns the original message if no pattern matches.
 */
export function friendlyError(error: unknown): string {
  const raw = errorText(error).toLowerCase();

  // Project / database errors
  if (raw.includes("no such table") || raw.includes("schema mismatch")) {
    return "数据库结构不兼容。请以读写模式重新打开项目以执行自动迁移。";
  }
  if (raw.includes("database is locked") || raw.includes("database locked")) {
    return "数据库被占用，可能是其他程序正在访问。请关闭其他 NDG 实例后重试。";
  }
  if (raw.includes("unable to open database") || (raw.includes("cannot open") && raw.includes("database"))) {
    return "无法打开数据库文件。请检查路径是否正确、文件是否存在、以及是否有读取权限。";
  }
  if ((raw.includes("read-only") || raw.includes("readonly")) && raw.includes("write")) {
    return "当前为只读模式，无法执行写操作。请在数据源页面以读写模式重新打开项目。";
  }
  if (raw.includes("symbolic link") || raw.includes("symlink")) {
    return "路径包含符号链接，已拒绝访问以防止越权。请直接使用实际物理路径。";
  }

  // Scan errors
  if (raw.includes("permission denied") || raw.includes("access denied")) {
    return "权限不足，无法读取该目录。请检查目录权限或使用具有访问权限的账户。";
  }
  if (
    raw.includes("no such file or directory") ||
    (raw.includes("not found") && (raw.includes("path") || raw.includes("directory")))
  ) {
    return "路径不存在。请检查输入的路径是否正确，包括拼写和大小写。";
  }
  if (raw.includes("root") && raw.includes("empty")) {
    return "扫描根目录不能为空。请输入要扫描的目录路径。";
  }
  if (raw.includes("worker") && raw.includes("panic")) {
    return "扫描工作线程异常崩溃。请减少并发数后重试，并查看任务日志确认原因。";
  }

  // Execution errors
  if (raw.includes("purge confirmation rejected")) {
    return "清理确认语句不匹配。请逐字输入确认语句（注意全角/半角差异）。";
  }
  if (raw.includes("dry-run is required") || raw.includes("dry_run")) {
    return "永久清理前必须先完成试运行校验。请先点击「试运行」按钮。";
  }
  if (raw.includes("digest") && raw.includes("reject")) {
    return "审批摘要校验失败。计划可能已被修改，请刷新后重新审批。";
  }
  if (raw.includes("stale")) {
    return "计划已过期（文件自审批后发生了变化）。请重新生成计划并审批。";
  }
  if (raw.includes("recovery lock") || raw.includes("executing_count")) {
    return "恢复锁已激活，存在未完成的执行计划。请先在审计与恢复页面处理。";
  }
  if (raw.includes("rollback")) {
    return "操作已回滚。请检查日志了解失败原因，修正后重试。";
  }

  // Network / connection
  if (raw.includes("connection refused") || raw.includes("timeout")) {
    return "连接超时或被拒绝。如果是网络盘，请检查连接状态后重试。";
  }

  return errorText(error);
}

export function formatBytes(bytes: number): string {
  if (bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB", "PB", "EB"];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
}

export function shortHash(hash: string): string {
  if (hash.length <= 12) return hash;
  return `${hash.slice(0, 8)}…${hash.slice(-4)}`;
}

export function formatDateTime(iso: string): string {
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

export function stateLabel(state: string): string {
  return STATE_LABELS[state] || state;
}

export function stageLabel(stage: string): string {
  return STAGE_LABELS[stage] || stage;
}

export function eventLabel(eventType: string): string {
  return EVENT_LABELS[eventType] || eventType;
}

export function stateBadgeClass(state: string): string {
  return `state-badge state-badge--${state.toLowerCase().replace("_", "-")}`;
}

export async function copyToClipboard(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch {
    // Fallback below
  }
  try {
    const textarea = document.createElement("textarea");
    textarea.value = text;
    textarea.style.position = "fixed";
    textarea.style.opacity = "0";
    document.body.appendChild(textarea);
    textarea.select();
    const ok = document.execCommand("copy");
    document.body.removeChild(textarea);
    return ok;
  } catch {
    return false;
  }
}
