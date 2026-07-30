// Start card: the only thing a new user sees when no project is open.
// Three entry points (per docs/desktop-frontend-architecture.md §5.3):
//   1. 新建扫描项目 (primary)  — pick a scan dir, app creates the db
//   2. 打开最近项目 (secondary) — one click on a recent project
//   3. 打开已有数据库 (advanced) — type/select an existing .db path
//
// New users never type a .db path: the primary flow picks a scan
// directory via the native dialog and the backend creates the project
// database under the OS app-support dir (read-write, owner-only).

import { useState } from "react";
import { wails } from "../wailsjs/go/models";
import { hasWailsRuntime } from "../lib/utils";
import { useProject } from "../state/ProjectContext";
import { api } from "../api/client";

export interface ProjectStartCardProps {
  busy: boolean;
  error: string | null;
  recentProjects: wails.RecentProjectEntry[];
  onCreate: (name: string, scanRoot: string) => void;
  onOpenRecent: (path: string) => void;
  onOpenExisting: (path: string, readWrite: boolean) => void;
}

type Mode = "idle" | "creating" | "advanced";

export default function ProjectStartCard({
  busy,
  error,
  recentProjects,
  onCreate,
  onOpenRecent,
  onOpenExisting,
}: ProjectStartCardProps) {
  const { displayPath } = useProject();
  const [mode, setMode] = useState<Mode>("idle");

  // Create-flow form state
  const [scanRoot, setScanRoot] = useState("");
  const [projectName, setProjectName] = useState("");
  const [pickerBusy, setPickerBusy] = useState(false);

  // Advanced (open existing) state
  const [existingPath, setExistingPath] = useState("");

  const handlePickDirectory = async () => {
    if (!hasWailsRuntime()) return;
    setPickerBusy(true);
    try {
      const picked = await api.project.pickDirectory("选择待扫描目录");
      // Empty string with no error means the user cancelled.
      if (picked) setScanRoot(picked);
    } catch {
      // Non-fatal: user can still type a path manually in advanced mode.
    } finally {
      setPickerBusy(false);
    }
  };

  const handleCreate = () => {
    if (!scanRoot.trim()) return;
    onCreate(projectName.trim(), scanRoot.trim());
  };

  const resetCreateForm = () => {
    setScanRoot("");
    setProjectName("");
    setMode("idle");
  };

  return (
    <section className="card project-start-card">
      <h2>开始</h2>
      <p className="muted">
        选择待扫描的目录，NDG 会在本机自动创建项目数据库；扫描源保持只读，不会被写入。
      </p>

      {/* Primary entry: new scan project */}
      {mode === "creating" ? (
        <div className="start-create-form">
          <div className="path-row">
            <input
              aria-label="待扫描目录"
              type="text"
              value={scanRoot}
              onChange={(e) => setScanRoot(e.target.value)}
              placeholder="点击右侧按钮选择待扫描目录"
              readOnly
            />
            <button
              type="button"
              disabled={busy || pickerBusy}
              onClick={() => void handlePickDirectory()}
            >
              {pickerBusy ? "选择中…" : "选择目录"}
            </button>
          </div>
          <label className="start-name-field">
            <span className="muted">项目名称（可选）</span>
            <input
              aria-label="项目名称"
              type="text"
              value={projectName}
              onChange={(e) => setProjectName(e.target.value)}
              placeholder="例如：产业资料库"
              maxLength={64}
            />
          </label>
          <div className="button-row">
            <button
              type="button"
              disabled={busy || !scanRoot.trim()}
              onClick={handleCreate}
            >
              {busy ? "创建中…" : "创建项目并前往扫描"}
            </button>
            <button
              type="button"
              className="secondary"
              disabled={busy}
              onClick={resetCreateForm}
            >
              取消
            </button>
          </div>
        </div>
      ) : (
        <button
          type="button"
          className="start-primary"
          disabled={busy}
          onClick={() => setMode("creating")}
        >
          新建扫描项目
        </button>
      )}

      {/* Secondary entry: recent projects */}
      {recentProjects.length > 0 && (
        <div className="start-recent">
          <h3>最近项目</h3>
          <ul className="start-recent-list">
            {recentProjects.map((entry) => (
              <li key={entry.path}>
                <button
                  type="button"
                  className="start-recent-item"
                  disabled={busy}
                  title={displayPath(entry.path)}
                  onClick={() => onOpenRecent(entry.path)}
                >
                  <span className="start-recent-name">{entry.name}</span>
                  <span className="start-recent-path muted">
                    {displayPath(entry.path)}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </div>
      )}

      {/* Advanced entry: open an existing database */}
      <div className="start-advanced">
        <button
          type="button"
          className="link-button"
          disabled={busy}
          onClick={() => setMode(mode === "advanced" ? "idle" : "advanced")}
        >
          {mode === "advanced" ? "▾" : "▸"} 高级：打开已有数据库
        </button>
        {mode === "advanced" && (
          <div className="start-advanced-form">
            <p className="muted">
              适用于已在本机或其他位置存在项目数据库的情况。新建项目请使用上方主入口。
            </p>
            <div className="path-row">
              <input
                aria-label="项目数据库路径"
                type="text"
                value={existingPath}
                onChange={(e) => setExistingPath(e.target.value)}
                placeholder="/path/to/governance.db"
              />
            </div>
            <div className="button-row">
              <button
                type="button"
                disabled={busy || !existingPath.trim()}
                onClick={() => onOpenExisting(existingPath.trim(), true)}
              >
                读写打开
              </button>
              <button
                type="button"
                className="secondary"
                disabled={busy || !existingPath.trim()}
                onClick={() => onOpenExisting(existingPath.trim(), false)}
              >
                只读打开
              </button>
            </div>
          </div>
        )}
      </div>

      {error && <p className="error" role="alert">{error}</p>}
    </section>
  );
}
