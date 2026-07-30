// Sources page: project open/close, storage list, scan readiness,
// and diagnostic reports (format / governance / merge).

import { useState } from "react";
import ProjectPanel from "../components/ProjectPanel";
import StorageList from "../components/StorageList";
import OnboardingGuide from "../components/OnboardingGuide";
import DiagnosticPanel from "../components/DiagnosticPanel";
import { useProject } from "../state/ProjectContext";

export default function SourcesPage() {
  const {
    project,
    projectPath,
    busy,
    error,
    isReadWrite,
    storages,
    storagesError,
    setProjectPath,
    openProject,
    closeProject,
    refreshProject,
  } = useProject();

  const [readWriteMode, setReadWriteMode] = useState(false);

  return (
    <div className="page page--sources">
      <div className="page-header">
        <h2>数据源</h2>
        <p className="muted">项目、存储、扫描准备与诊断报告</p>
      </div>

      <ProjectPanel
        project={project}
        projectPath={projectPath}
        busy={busy}
        error={error}
        readWriteMode={readWriteMode}
        isReadWrite={isReadWrite}
        onProjectPathChange={setProjectPath}
        onReadWriteModeChange={setReadWriteMode}
        onOpenProject={() => void openProject(readWriteMode)}
        onCloseProject={() => void closeProject()}
        onRefreshProject={() => void refreshProject()}
      />

      {!project && <OnboardingGuide />}

      {project && (
        <>
          <StorageList storages={storages} storagesError={storagesError} />
          <DiagnosticPanel storages={storages} />
        </>
      )}
    </div>
  );
}
