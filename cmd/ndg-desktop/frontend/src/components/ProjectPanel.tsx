import { wails } from "../wailsjs/go/models";

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
  const projectOpen = project !== null;

  return (
    <section className="card project-card">
      <h2>项目</h2>
      {projectOpen ? (
        <div className="project-info">
          <p>
            <strong>数据库：</strong>
            <span className="project-path">{project!.path}</span>
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
          </div>
          <label className="mode-toggle">
            <input
              type="checkbox"
              checked={readWriteMode}
              onChange={(event) => onReadWriteModeChange(event.target.checked)}
            />
            读写模式（可扫描）
          </label>
          <button disabled={busy || !projectPath.trim()} onClick={onOpenProject}>
            {readWriteMode ? "读写打开" : "只读打开"}
          </button>
        </div>
      )}
      {error && <p className="error" role="alert">{error}</p>}
    </section>
  );
}
