// Seven-domain navigation identifiers for NDG desktop.
// Stable top-level routes — pages are organized by user task, not by Go package.

export type AppRoute =
  | "sources"
  | "scan-jobs"
  | "duplicate-results"
  | "governance-review"
  | "execution-center"
  | "audit-recovery"
  | "settings";

export interface NavItem {
  id: AppRoute;
  label: string;
  description: string;
}

export const NAV_ITEMS: NavItem[] = [
  { id: "sources", label: "数据源", description: "项目、存储与扫描准备" },
  { id: "scan-jobs", label: "扫描任务", description: "新建扫描、进度与历史" },
  { id: "duplicate-results", label: "重复结果", description: "重复文件组与目录语境" },
  { id: "governance-review", label: "治理复核", description: "治理决策与计划草案" },
  { id: "execution-center", label: "执行中心", description: "隔离、清理与执行安全" },
  { id: "audit-recovery", label: "审计与恢复", description: "操作审计、Journal与恢复" },
  { id: "settings", label: "设置", description: "应用配置与开发者信息" },
];

export const DEFAULT_ROUTE: AppRoute = "sources";
