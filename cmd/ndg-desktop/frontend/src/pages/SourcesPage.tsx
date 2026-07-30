// Sources page: project start card / open-project summary, storage list,
// scan readiness, and diagnostic reports (format / governance / merge).

import { useState, useEffect, useCallback } from "react";
import ProjectPanel from "../components/ProjectPanel";
import ProjectStartCard from "../components/ProjectStartCard";
import StorageList from "../components/StorageList";
import DiagnosticPanel from "../components/DiagnosticPanel";
import { useProject } from "../state/ProjectContext";
import { hasWailsRuntime } from "../lib/utils";
import { wails } from "../wailsjs/go/models";
import { api } from "../api/client";

export default function SourcesPage() {
  const {
    project,
    busy,
    error,
    isReadWrite,
    storages,
    storagesError,
    recentProjects,
    dataRevision,
    createNewProject,
    openExisting,
    closeProject,
    refreshProject,
  } = useProject();

  const [readiness, setReadiness] = useState<wails.ProjectReadinessDTO | null>(null);

  const loadReadiness = useCallback(async () => {
    if (!hasWailsRuntime()) return;
    try {
      const result = await api.capabilities.readiness();
      setReadiness(result);
    } catch {
      // Non-fatal: readiness is informational
      setReadiness(null);
    }
  }, []);

  useEffect(() => {
    if (project) {
      void loadReadiness();
    } else {
      setReadiness(null);
    }
  }, [project, dataRevision, loadReadiness]);

  return (
    <div className="page page--sources">
      <div className="page-header">
        <h2>数据源</h2>
        <p className="muted">项目、存储、扫描准备与诊断报告</p>
      </div>

      {project ? (
        <ProjectPanel
          project={project}
          busy={busy}
          error={error}
          isReadWrite={isReadWrite}
          onCloseProject={() => void closeProject()}
          onRefreshProject={() => void refreshProject()}
        />
      ) : (
        <ProjectStartCard
          busy={busy}
          error={error}
          recentProjects={recentProjects}
          onCreate={(name, scanRoot) => void createNewProject(name, scanRoot)}
          onOpenRecent={(path) => void openExisting(path, true)}
          onOpenExisting={(path, readWrite) => void openExisting(path, readWrite)}
        />
      )}

      {project && (
        <>
          {/* Readiness checklist */}
          {readiness && (
            <section className="card card--full readiness-panel">
              <div className="card-header-row">
                <h3>扫描就绪状态</h3>
                <span className={`readiness-badge ${readiness.ready ? "readiness-badge--ready" : "readiness-badge--not-ready"}`}>
                  {readiness.ready ? "✓ 就绪" : "未就绪"}
                </span>
              </div>
              <div className="readiness-checks">
                {readiness.checks.map((check) => (
                  <div key={check.key} className={`readiness-check ${check.passed ? "readiness-check--passed" : "readiness-check--failed"}`}>
                    <span className="readiness-check-icon">
                      {check.passed ? "✓" : "○"}
                    </span>
                    <div className="readiness-check-content">
                      <span className="readiness-check-label">{check.label}</span>
                      {check.hint && (
                        <span className="readiness-check-hint muted">{check.hint}</span>
                      )}
                      {!check.passed && check.reason && (
                        <span className="readiness-check-reason muted">{check.reason}</span>
                      )}
                    </div>
                  </div>
                ))}
              </div>
              <div className="readiness-stats">
                <span className="muted">存储 {readiness.storage_count}</span>
                <span className="muted">文件 {readiness.file_count}</span>
                <span className="muted">计划 {readiness.plan_count}</span>
              </div>
            </section>
          )}

          <StorageList storages={storages} storagesError={storagesError} />
          <DiagnosticPanel storages={storages} />
        </>
      )}
    </div>
  );
}
