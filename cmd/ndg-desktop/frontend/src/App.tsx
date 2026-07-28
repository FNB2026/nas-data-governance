import { useEffect, useState } from "react";
import {
  CloseProject,
  GetProjectInfo,
  GetVersion,
  OpenProject,
} from "./wailsjs/go/wails/API";

interface VersionInfo {
  version: string;
  commit: string;
  build_time: string;
}

interface ProjectInfo {
  path: string;
  is_open: boolean;
  storage_count: number;
}

function hasWailsRuntime(): boolean {
  return typeof window !== "undefined" && "go" in window && "runtime" in window;
}

function errorText(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

export default function App() {
  const [version, setVersion] = useState<VersionInfo | null>(null);
  const [project, setProject] = useState<ProjectInfo | null>(null);
  const [projectPath, setProjectPath] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!hasWailsRuntime()) {
      setVersion({ version: "dev", commit: "n/a", build_time: "n/a" });
      return;
    }
    GetVersion()
      .then(setVersion)
      .catch((e: unknown) => setError(errorText(e)));
  }, []);

  const handleOpenProject = async () => {
    if (!projectPath.trim()) {
      setError("请先选择或输入项目数据库路径");
      return;
    }
    setBusy(true);
    try {
      const info = await OpenProject(projectPath.trim());
      setProject(info);
      setError(null);
    } catch (e: unknown) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  };

  const handleCloseProject = async () => {
    setBusy(true);
    try {
      await CloseProject();
      setProject(null);
      setError(null);
    } catch (e: unknown) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  };

  const handleRefreshProject = async () => {
    setBusy(true);
    try {
      const info = await GetProjectInfo();
      setProject(info);
      setError(null);
    } catch (e: unknown) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="app">
      <header className="app-header">
        <h1>NDG 数据治理工作台</h1>
        {version && (
          <span className="version-badge">
            v{version.version} ({version.commit})
          </span>
        )}
      </header>

      <main className="app-main">
        <section className="card project-card">
          <h2>项目</h2>
          {project ? (
            <div className="project-info">
              <p>
                <strong>数据库：</strong>
                <span className="project-path">{project.path}</span>
              </p>
              <p>
                <strong>存储数量：</strong>
                {project.storage_count}
              </p>
              <div className="button-row">
                <button disabled={busy} onClick={handleRefreshProject}>刷新</button>
                <button className="secondary" disabled={busy} onClick={handleCloseProject}>关闭项目</button>
              </div>
            </div>
          ) : (
            <div className="project-open">
              <p className="muted">输入已有项目数据库路径；只读 Alpha 不会创建或迁移数据库。</p>
              <div className="path-row">
                <input
                  aria-label="项目数据库路径"
                  value={projectPath}
                  onChange={(event) => setProjectPath(event.target.value)}
                  placeholder="/path/to/project.db"
                />
              </div>
              <button disabled={busy || !projectPath.trim()} onClick={handleOpenProject}>只读打开</button>
            </div>
          )}
          {error && <p className="error" role="alert">{error}</p>}
        </section>

        <section className="card">
          <h2>扫描</h2>
          <p className="muted">扫描功能将在后续 PR 中实现</p>
        </section>

        <section className="card">
          <h2>重复组</h2>
          <p className="muted">重复文件检测将在后续 PR 中实现</p>
        </section>
      </main>

      <footer className="app-footer">
        <p>NDG — NAS Data Governance · 只读 Alpha</p>
      </footer>
    </div>
  );
}
