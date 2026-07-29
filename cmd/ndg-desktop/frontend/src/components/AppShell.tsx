// AppShell: sidebar + header + content area.
// Renders seven-domain navigation with capability-based enable/disable.

import { type ReactNode } from "react";
import { NAV_ITEMS, type AppRoute } from "../app/routes";
import { isRouteEnabled } from "../app/capability";
import { useProject } from "../state/ProjectContext";

interface AppShellProps {
  activeRoute: AppRoute;
  onRouteChange: (route: AppRoute) => void;
  children: ReactNode;
}

export default function AppShell({
  activeRoute,
  onRouteChange,
  children,
}: AppShellProps) {
  const {
    version,
    project,
    isReadWrite,
    capabilities,
    activeJobId,
    scanProgress,
  } = useProject();

  const modeLabel = !project
    ? "未打开"
    : isReadWrite
      ? "读写"
      : "只读";

  const scanActive = activeJobId !== null;

  return (
    <div className="app-shell">
      <header className="app-header" key="header">
        <div className="app-header-left">
          <h1>NDG 数据治理</h1>
          {project && (
            <span className="header-project-name">
              {project.path?.split("/").pop() || "项目"}
            </span>
          )}
          <span className={`mode-badge mode-badge--${capabilities.project_mode}`}>
            {modeLabel}
          </span>
          {scanActive && scanProgress && (
            <span className="header-scan-indicator">
              扫描中 {scanProgress.processed.toLocaleString()} /{" "}
              {scanProgress.discovered.toLocaleString()}
            </span>
          )}
        </div>
        <div className="app-header-right">
          {version && (
            <span className="version-badge">
              v{version.version} ({version.commit})
            </span>
          )}
        </div>
      </header>

      <div className="app-body">
        <nav className="app-sidebar">
          {NAV_ITEMS.map((item) => {
            const enabled = isRouteEnabled(item.id, capabilities);
            const reason = capabilities.disabled_reasons[item.id];
            const active = activeRoute === item.id;
            return (
              <button
                key={item.id}
                className={`nav-item ${active ? "nav-item--active" : ""} ${
                  !enabled ? "nav-item--disabled" : ""
                }`}
                disabled={!enabled}
                title={!enabled ? reason : undefined}
                onClick={() => enabled && onRouteChange(item.id)}
              >
                <span className="nav-item-label">{item.label}</span>
                <span className="nav-item-desc">{item.description}</span>
              </button>
            );
          })}
        </nav>

        <main className="app-content">{children}</main>
      </div>

      <footer className="app-footer">
        <span>NDG — NAS Data Governance</span>
        <span className="footer-separator">·</span>
        <span>{modeLabel}模式</span>
        {capabilities.recovery_lock_active && (
          <>
            <span className="footer-separator">·</span>
            <span className="footer-recovery">恢复锁激活</span>
          </>
        )}
      </footer>
    </div>
  );
}
