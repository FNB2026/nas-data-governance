import { useState, useEffect, useRef } from "react";
import { wails } from "../wailsjs/go/models";
import { ValidateProjectPath } from "../wailsjs/go/wails/API";
import { hasWailsRuntime, friendlyError } from "../lib/utils";
import { useProject } from "../state/ProjectContext";

export interface ProjectPanelProps {
  project: wails.ProjectInfo | null;
  projectPath: string;
  busy: boolean;
  error: string | null;
  readWriteMode: boolean;
  isReadWrite: boolean;
  onProjectPathChange: (value: string) => void;
  onReadWriteModeChange: (checked: boolean) => void;
  onOpenProject: () => void;
  onCloseProject: () => void;
  onRefreshProject: () => void;
}

type PathStatus = "empty" | "checking" | "valid" | "invalid";

export default function ProjectPanel({
  project,
  projectPath,
  busy,
  error,
  readWriteMode,
  isReadWrite,
  onProjectPathChange,
  onReadWriteModeChange,
  onOpenProject,
  onCloseProject,
  onRefreshProject,
}: ProjectPanelProps) {
  const { displayPath } = useProject();
  const projectOpen = project !== null;

  // Path pre-validation (debounced)
  const [pathStatus, setPathStatus] = useState<PathStatus>("empty");
  const [pathHint, setPathHint] = useState<string>("");
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (projectOpen) return;
    const trimmed = projectPath.trim();

    if (!trimmed) {
      setPathStatus("empty");
      setPathHint("");
      return;
    }

    if (!hasWailsRuntime()) {
      setPathStatus("valid");
      setPathHint("");
      return;
    }

    setPathStatus("checking");
    setPathHint("");

    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(async () => {
      // In read-write mode, the database file may not exist yet (new
      // project). The backend ValidateProjectPath requires the file to
      // already exist, so for RW mode we do client-side validation only:
      // check the extension and that the path has a parent directory.
      if (readWriteMode) {
        const ext = trimmed.toLowerCase().match(/\.(db|sqlite|sqlite3)$/);
        if (!ext) {
          setPathStatus("invalid");
          setPathHint("路径需要 .db、.sqlite 或 .sqlite3 扩展名");
          return;
        }
        const lastSlash = Math.max(trimmed.lastIndexOf("/"), trimmed.lastIndexOf("\\"));
        if (lastSlash <= 0) {
          setPathStatus("invalid");
          setPathHint("请输入完整路径，包含父目录");
          return;
        }
        setPathStatus("valid");
        setPathHint("新数据库将在读写打开时自动创建");
        return;
      }

      // Read-only mode: the database file must already exist.
      try {
        await ValidateProjectPath(trimmed);
        setPathStatus("valid");
        setPathHint("");
      } catch (e: unknown) {
        setPathStatus("invalid");
        setPathHint(friendlyError(e));
      }
    }, 300);

    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
  }, [projectPath, projectOpen, readWriteMode]);

  return (
    <section className="card project-card">
      <h2>项目</h2>
      {projectOpen ? (
        <div className="project-info">
          <p>
            <strong>数据库：</strong>
            <span className="project-path">{displayPath(project!.path)}</span>
          </p>
          <p>
            <strong>存储数量：</strong>
            {project!.storage_count}
            {isReadWrite && <span className="mode-indicator">读写模式</span>}
          </p>
          <div className="button-row">
            <button disabled={busy} onClick={onRefreshProject}>刷新</button>
            <button className="secondary" disabled={busy} onClick={onCloseProject}>关闭项目</button>
          </div>
        </div>
      ) : (
        <div className="project-open">
          <p className="muted">
            {readWriteMode
              ? "读写模式：可创建新数据库、执行扫描；首次打开会自动建表迁移。"
              : "只读模式：不会创建或迁移数据库，仅查询已有数据。"}
          </p>
          <div className="path-row">
            <input
              aria-label="项目数据库路径"
              value={projectPath}
              onChange={(event) => onProjectPathChange(event.target.value)}
              placeholder="/path/to/project.db"
            />
            {pathStatus === "checking" && (
              <span className="path-status path-status--checking">校验中…</span>
            )}
            {pathStatus === "valid" && (
              <span className="path-status path-status--valid">✓ 路径可用</span>
            )}
            {pathStatus === "invalid" && (
              <span className="path-status path-status--invalid" title={pathHint}>
                ✕ {pathHint || "路径无效"}
              </span>
            )}
          </div>
          {pathStatus === "invalid" && pathHint && (
            <p className="error path-error-detail">{pathHint}</p>
          )}
          <label className="mode-toggle">
            <input
              type="checkbox"
              checked={readWriteMode}
              onChange={(event) => onReadWriteModeChange(event.target.checked)}
            />
            读写模式（可扫描）
          </label>
          <button disabled={busy || !projectPath.trim() || pathStatus === "invalid"} onClick={onOpenProject}>
            {readWriteMode ? "读写打开" : "只读打开"}
          </button>
        </div>
      )}
      {error && <p className="error" role="alert">{error}</p>}
    </section>
  );
}
