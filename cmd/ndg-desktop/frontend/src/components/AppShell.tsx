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
    project,
    isReadWrite,
    capabilities,
    activeJobId,
    scanProgress,
    connectionStatus,
    displayPath,
    refreshProject,
  } = useProject();

  const modeLabel = !project
    ? "未打开"
    : isReadWrite
      ? "读写"
      : "只读";

  const scanActive = activeJobId !== null;
  const projectName = project
    ? displayPath(project.path)?.split("/").pop() || "项目"
    : "尚未打开项目";

  return (
    <div className="app-shell">
      <header className="app-header">
        <div className="app-header-left">
          <div className="app-brand" aria-label="NDG 数据治理">
            <span className="app-brand-mark" aria-hidden="true">N</span>
            <span className="app-brand-name">NDG</span>
          </div>
          <span className="header-project-name" title={project ? displayPath(project.path) : undefined}>
            {projectName}
          </span>
          <span className={`mode-badge mode-badge--${capabilities.project_mode}`}>
            {modeLabel}
          </span>
          {scanActive && scanProgress && (
            <span className="header-scan-indicator" role="status">
              扫描中 · 已处理 {scanProgress.processed.toLocaleString()} /{" "}
              {scanProgress.discovered.toLocaleString()}
            </span>
          )}
          {capabilities.recovery_lock_active && (
            <span className="header-recovery-indicator" role="status">
              需要恢复处理
            </span>
          )}
          {connectionStatus === "reconnecting" && (
            <span className="header-conn-indicator header-conn-indicator--reconnecting" role="status">
              正在重新连接 NAS…
            </span>
          )}
          {connectionStatus === "disconnected" && (
            <div className="header-connection-alert" role="status">
              <span>NAS 连接中断</span>
              <button type="button" className="header-status-action" onClick={() => void refreshProject()}>
                重新检查
              </button>
            </div>
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
                aria-current={active ? "page" : undefined}
                aria-label={`${item.label}：${item.description}${!enabled && reason ? `（${reason}）` : ""}`}
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
