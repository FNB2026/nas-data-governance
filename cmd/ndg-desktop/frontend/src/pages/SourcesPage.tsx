// Sources page: project start card / open-project summary, storage list,
// scan readiness, and diagnostic reports (format / governance / merge).

import { useState, useEffect, useCallback } from "react";
import ProjectPanel from "../components/ProjectPanel";
import ProjectStartCard from "../components/ProjectStartCard";
import StorageList from "../components/StorageList";
import SourceAuthorizationPanel from "../components/SourceAuthorizationPanel";
import DiagnosticPanel from "../components/DiagnosticPanel";
import GettingStartedRail from "../components/GettingStartedRail";
import { useProject } from "../state/ProjectContext";
import { hasWailsRuntime } from "../lib/utils";
import { wails } from "../wailsjs/go/models";
import { api } from "../api/client";
import type { AppRoute } from "../app/routes";

export interface SourcesPageProps {
  onNavigate: (route: AppRoute) => void;
}

export default function SourcesPage({ onNavigate }: SourcesPageProps) {
  const {
    project,
    busy,
    error,
    isReadWrite,
    storages,
    storagesError,
    recentProjects,
    dataRevision,
    gettingStartedDone,
    createNewProject,
    openExisting,
    closeProject,
    refreshProject,
  } = useProject();

  const [readiness, setReadiness] = useState<wails.ProjectReadinessDTO | null>(null);
  const [sourceProfiles, setSourceProfiles] = useState<Record<string, wails.SourcePreflightDTO | null>>({});
  const [canReauthorizeSources, setCanReauthorizeSources] = useState(false);
  const [authorizingSource, setAuthorizingSource] = useState(false);
  const [sourceAuthorizationError, setSourceAuthorizationError] = useState<string | null>(null);
  const [sourceAuthorizationNotice, setSourceAuthorizationNotice] = useState<string | null>(null);

  const handleAuthorizeSource = useCallback(async () => {
    if (!project || !isReadWrite || authorizingSource || busy || !hasWailsRuntime()) return;
    setAuthorizingSource(true);
    setSourceAuthorizationError(null);
    setSourceAuthorizationNotice(null);
    try {
      const root = await api.project.pickDirectory("选择或重新授权扫描目录");
      if (!root) {
        setSourceAuthorizationNotice("未选择目录；项目和扫描状态未更改。");
        return;
      }
      await api.project.registerScanSource(root);
      await refreshProject();
      setSourceAuthorizationNotice("授权已登记。没有启动扫描或 Resume，也没有解除恢复锁；请核对存储身份后手动继续。");
    } catch (e: unknown) {
      const message = (e as Error).message || "";
      // Avoid reflecting backend paths or other source details in the UI.
      setSourceAuthorizationError(
        /AUTHORIZATION_REQUIRED|授权|取消/i.test(message)
          ? "目录授权未完成；没有启动扫描或 Resume。请重新选择经确认的目录。"
          : "目录登记失败；没有启动扫描或 Resume。请刷新项目并人工复核。",
      );
    } finally {
      setAuthorizingSource(false);
    }
  }, [project, isReadWrite, authorizingSource, busy, refreshProject]);

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
    let cancelled = false;
    setCanReauthorizeSources(false);
    if (!project || !hasWailsRuntime()) return () => { cancelled = true; };
    void api.capabilities.get().then((capabilities) => {
      if (!cancelled) setCanReauthorizeSources(capabilities.can_reauthorize_sources === true);
    }).catch(() => {
      // Fail closed: a distribution identity error must not show MAS-only copy.
      if (!cancelled) setCanReauthorizeSources(false);
    });
    return () => { cancelled = true; };
  }, [project]);

  useEffect(() => {
    if (project) {
      void loadReadiness();
    } else {
      setReadiness(null);
    }
  }, [project, dataRevision, loadReadiness]);

  useEffect(() => {
    let cancelled = false;
    if (!project || !hasWailsRuntime()) {
      setSourceProfiles({});
      return () => { cancelled = true; };
    }
    setSourceProfiles({});
    void Promise.all(storages.map(async (storage) => {
      try {
        const profile = await api.scan.preflight(storage.root_path);
        return [storage.id, profile] as const;
      } catch {
        return [storage.id, null] as const;
      }
    })).then((entries) => {
      if (!cancelled) setSourceProfiles(Object.fromEntries(entries));
    });
    return () => { cancelled = true; };
  }, [project, storages, dataRevision]);

  return (
    <div className="page page--sources">
      <div className="page-header">
        <h2>数据源</h2>
        <p className="muted">项目、存储、扫描准备与诊断报告</p>
      </div>

      {project && !gettingStartedDone && <GettingStartedRail onNavigate={onNavigate} />}

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
          <SourceAuthorizationPanel
            enabled={canReauthorizeSources}
            busy={busy || authorizingSource}
            isReadWrite={isReadWrite}
            onAuthorize={() => void handleAuthorizeSource()}
            error={sourceAuthorizationError}
            notice={sourceAuthorizationNotice}
          />
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

          <StorageList storages={storages} storagesError={storagesError} sourceProfiles={sourceProfiles} />
          <DiagnosticPanel storages={storages} />
        </>
      )}
    </div>
  );
}
