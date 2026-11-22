// Seven-domain navigation identifiers for NDG desktop.
// Stable top-level routes — pages are organized by user task, not by Go package.

export type AppRoute =
  | "sources"
  | "scan-jobs"
  | "governance-workbench"
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

export const MAS_CHANNEL = import.meta.env.VITE_NDG_CHANNEL === "mas";

const desktopNav: NavItem[] = [
  { id: "sources", label: "数据源", description: "项目、存储与扫描准备" },
  { id: "scan-jobs", label: "扫描任务", description: "新建扫描、进度与历史" },
  { id: "governance-workbench", label: "重复治理", description: "查看证据、记录决定与逐项回执" },
  { id: "execution-center", label: "任务中心", description: "独立审批后的隔离与恢复" },
  { id: "audit-recovery", label: "审计与恢复", description: "操作审计、Journal与恢复" },
  { id: "settings", label: "设置", description: "应用配置、隐私与安全状态" },
];

export const NAV_ITEMS: NavItem[] = MAS_CHANNEL
  ? [
      ...desktopNav.filter(({ id }) => id === "sources" || id === "scan-jobs"),
      { id: "duplicate-results", label: "重复结果", description: "重复文件、目录语境与保留理由" },
      ...desktopNav.filter(({ id }) => id === "settings"),
    ]
  : desktopNav;

export const DEFAULT_ROUTE: AppRoute = "sources";

// Preserve existing keyboard/deep-link route identifiers. Old duplicate-results
// maps to the unified workbench; the old plan-review page remains accessible.
export const SHORTCUT_ROUTES: AppRoute[] = MAS_CHANNEL
  ? NAV_ITEMS.map(({ id }) => id)
  : ["sources", "scan-jobs", "duplicate-results", "governance-review", "execution-center", "audit-recovery", "settings"];
