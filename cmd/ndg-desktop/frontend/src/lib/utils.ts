// Shared constants and utility functions for the NDG desktop frontend.

export const TERMINAL_STATES = new Set(["COMPLETED", "FAILED", "CANCELLED"]);

export const STAGE_LABELS: Record<string, string> = {
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

export const STATE_LABELS: Record<string, string> = {
  QUEUED: "排队中",
  RUNNING: "运行中",
  CANCEL_REQUESTED: "取消中",
  COMPLETED: "已完成",
  FAILED: "已失败",
  CANCELLED: "已取消",
};

export const EVENT_LABELS: Record<string, string> = {
  "job:created": "创建",
  "job:stage": "阶段切换",
  "job:progress": "进度更新",
  "job:warning": "警告",
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
