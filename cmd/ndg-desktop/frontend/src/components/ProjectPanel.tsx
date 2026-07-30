// Project panel: compact summary shown when a project is open.
// The closed (no-project) state is handled by ProjectStartCard, which
// owns the three first-launch entry points. ProjectPanel only renders
// the open-project view (db path, storage count, refresh / close).

import { wails } from "../wailsjs/go/models";
import { useProject } from "../state/ProjectContext";

export interface ProjectPanelProps {
  project: wails.ProjectInfo;
  busy: boolean;
  error: string | null;
  isReadWrite: boolean;
  onCloseProject: () => void;
  onRefreshProject: () => void;
}

export default function ProjectPanel({
  project,
  busy,
  error,
  isReadWrite,
  onCloseProject,
  onRefreshProject,
}: ProjectPanelProps) {
  const { displayPath } = useProject();

  return (
    <section className="card project-card">
      <h2>项目</h2>
      <div className="project-info">
        <p>
          <strong>名称：</strong>
          {project.name}
        </p>
        <p>
          <strong>数据库：</strong>
          <span className="project-path">{displayPath(project.path)}</span>
        </p>
        <p>
          <strong>存储数量：</strong>
          {project.storage_count}
          {isReadWrite && <span className="mode-indicator">读写模式</span>}
        </p>
        <div className="button-row">
          <button disabled={busy} onClick={onRefreshProject}>刷新</button>
          <button className="secondary" disabled={busy} onClick={onCloseProject}>关闭项目</button>
        </div>
      </div>
      {error && <p className="error" role="alert">{error}</p>}
    </section>
  );
}
