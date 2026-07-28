import { useCallback, useEffect, useState } from "react";
import {
  CloseProject,
  GetGroupDetail,
  GetProjectInfo,
  GetVersion,
  ListDuplicateGroups,
  ListStorages,
  OpenProject,
} from "./wailsjs/go/wails/API";
import { wails } from "./wailsjs/go/models";

function hasWailsRuntime(): boolean {
  return typeof window !== "undefined" && "go" in window && "runtime" in window;
}

function errorText(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function formatBytes(bytes: number): string {
  if (bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
}

function shortHash(hash: string): string {
  if (hash.length <= 12) return hash;
  return `${hash.slice(0, 8)}…${hash.slice(-4)}`;
}

export default function App() {
  const [version, setVersion] = useState<wails.VersionInfo | null>(null);
  const [project, setProject] = useState<wails.ProjectInfo | null>(null);
  const [projectPath, setProjectPath] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Storage list state
  const [storages, setStorages] = useState<wails.StorageInfo[]>([]);
  const [storagesError, setStoragesError] = useState<string | null>(null);

  // Duplicate groups state (keyset pagination)
  const [groups, setGroups] = useState<wails.GroupSummary[]>([]);
  const [nextCursor, setNextCursor] = useState("");
  const [totalCount, setTotalCount] = useState(0);
  const [groupsLoading, setGroupsLoading] = useState(false);
  const [groupsError, setGroupsError] = useState<string | null>(null);

  // Group detail state
  const [selectedGroup, setSelectedGroup] = useState<wails.GroupDetailResponse | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState<string | null>(null);

  useEffect(() => {
    if (!hasWailsRuntime()) {
      setVersion({ version: "dev", commit: "n/a", build_time: "n/a" } as wails.VersionInfo);
      return;
    }
    GetVersion()
      .then(setVersion)
      .catch((e: unknown) => setError(errorText(e)));
  }, []);

  const loadStorages = useCallback(async () => {
    if (!hasWailsRuntime()) return;
    try {
      const list = await ListStorages();
      setStorages(list || []);
      setStoragesError(null);
    } catch (e: unknown) {
      setStoragesError(errorText(e));
    }
  }, []);

  const loadGroups = useCallback(async (cursor: string) => {
    if (!hasWailsRuntime()) return;
    setGroupsLoading(true);
    try {
      const resp = await ListDuplicateGroups({
        page_size: 20,
        cursor,
      } as wails.ListGroupsRequest);
      if (cursor) {
        setGroups((prev) => [...prev, ...(resp.groups || [])]);
      } else {
        setGroups(resp.groups || []);
      }
      setNextCursor(resp.next_cursor || "");
      setTotalCount(resp.total_count || 0);
      setGroupsError(null);
    } catch (e: unknown) {
      setGroupsError(errorText(e));
    } finally {
      setGroupsLoading(false);
    }
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
      setSelectedGroup(null);
      // Auto-load storages and first page of groups
      await loadStorages();
      await loadGroups("");
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
      setStorages([]);
      setGroups([]);
      setNextCursor("");
      setTotalCount(0);
      setSelectedGroup(null);
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
      await loadStorages();
      await loadGroups("");
      setSelectedGroup(null);
    } catch (e: unknown) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  };

  const handleLoadMore = () => {
    if (nextCursor && !groupsLoading) {
      loadGroups(nextCursor);
    }
  };

  const handleSelectGroup = async (storageId: string, sha256: string) => {
    if (!hasWailsRuntime()) return;
    setDetailLoading(true);
    setDetailError(null);
    try {
      const detail = await GetGroupDetail(storageId, sha256);
      setSelectedGroup(detail);
    } catch (e: unknown) {
      setDetailError(errorText(e));
      setSelectedGroup(null);
    } finally {
      setDetailLoading(false);
    }
  };

  const handleCloseDetail = () => {
    setSelectedGroup(null);
    setDetailError(null);
  };

  const projectOpen = project !== null;

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

      <main className={projectOpen ? "app-main app-main--dashboard" : "app-main"}>
        {/* Project panel */}
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

        {projectOpen && (
          <>
            {/* Storage list */}
            <section className="card card--full">
              <h2>存储列表</h2>
              {storagesError ? (
                <p className="error" role="alert">{storagesError}</p>
              ) : storages.length === 0 ? (
                <p className="muted">暂无已注册的存储</p>
              ) : (
                <div className="table-wrap">
                  <table className="data-table">
                    <thead>
                      <tr>
                        <th>ID</th>
                        <th>根路径</th>
                        <th>类型</th>
                        <th>注册时间</th>
                      </tr>
                    </thead>
                    <tbody>
                      {storages.map((s) => (
                        <tr key={s.id}>
                          <td className="mono">{s.id}</td>
                          <td className="path-cell">{s.root_path}</td>
                          <td>{s.kind}</td>
                          <td className="muted">{s.created_at || "—"}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </section>

            {/* Duplicate groups */}
            <section className="card card--full">
              <div className="card-header-row">
                <h2>重复文件组</h2>
                {totalCount > 0 && (
                  <span className="count-badge">共 {totalCount} 组</span>
                )}
              </div>
              {groupsError ? (
                <p className="error" role="alert">{groupsError}</p>
              ) : groups.length === 0 && !groupsLoading ? (
                <p className="muted">未检测到重复文件，或尚未扫描</p>
              ) : (
                <>
                  <div className="table-wrap">
                    <table className="data-table">
                      <thead>
                        <tr>
                          <th>SHA-256</th>
                          <th>存储</th>
                          <th className="num">文件大小</th>
                          <th className="num">路径数</th>
                          <th className="num">物理副本</th>
                          <th className="num">硬链接别名</th>
                          <th className="num">可回收空间</th>
                          <th>操作</th>
                        </tr>
                      </thead>
                      <tbody>
                        {groups.map((g) => {
                          const key = `${g.storage_id}/${g.sha256}`;
                          const isSelected =
                            selectedGroup &&
                            selectedGroup.sha256 === g.sha256 &&
                            selectedGroup.storage_id === g.storage_id;
                          return (
                            <tr
                              key={key}
                              className={isSelected ? "row-selected" : ""}
                            >
                              <td className="mono" title={g.sha256}>{shortHash(g.sha256)}</td>
                              <td className="mono">{g.storage_id}</td>
                              <td className="num">{formatBytes(g.size)}</td>
                              <td className="num">{g.path_count}</td>
                              <td className="num">{g.physical_copy_count}</td>
                              <td className="num">{g.hardlink_alias_count}</td>
                              <td className="num">{formatBytes(g.physical_reclaimable_bytes)}</td>
                              <td>
                                <button
                                  className="btn-sm"
                                  disabled={detailLoading}
                                  onClick={() => handleSelectGroup(g.storage_id, g.sha256)}
                                >
                                  详情
                                </button>
                              </td>
                            </tr>
                          );
                        })}
                      </tbody>
                    </table>
                  </div>
                  {nextCursor && (
                    <div className="load-more">
                      <button
                        disabled={groupsLoading}
                        onClick={handleLoadMore}
                      >
                        {groupsLoading ? "加载中…" : "加载更多"}
                      </button>
                    </div>
                  )}
                </>
              )}
            </section>

            {/* Group detail */}
            {(selectedGroup || detailLoading || detailError) && (
              <section className="card card--full group-detail">
                <div className="card-header-row">
                  <h2>组详情</h2>
                  <button className="btn-sm secondary" onClick={handleCloseDetail}>关闭</button>
                </div>
                {detailLoading ? (
                  <p className="muted">加载中…</p>
                ) : detailError ? (
                  <p className="error" role="alert">{detailError}</p>
                ) : selectedGroup ? (
                  <>
                    <div className="detail-summary">
                      <span><strong>SHA-256：</strong><span className="mono">{selectedGroup.sha256}</span></span>
                      <span><strong>存储：</strong><span className="mono">{selectedGroup.storage_id}</span></span>
                      <span><strong>文件大小：</strong>{formatBytes(selectedGroup.size)}</span>
                      <span><strong>路径数：</strong>{selectedGroup.path_count}</span>
                      <span><strong>物理副本：</strong>{selectedGroup.physical_copy_count}</span>
                      <span><strong>硬链接别名：</strong>{selectedGroup.hardlink_alias_count}</span>
                      <span><strong>可回收：</strong>{formatBytes(selectedGroup.physical_reclaimable_bytes)}</span>
                    </div>
                    <div className="table-wrap">
                      <table className="data-table">
                        <thead>
                          <tr>
                            <th>路径</th>
                            <th>文件名</th>
                            <th className="num">大小</th>
                            <th>修改时间</th>
                            <th>物理可靠</th>
                            <th>格式</th>
                          </tr>
                        </thead>
                        <tbody>
                          {(selectedGroup.files || []).map((f, i) => (
                            <tr key={`${f.path}-${i}`}>
                              <td className="path-cell" title={f.path}>{f.path}</td>
                              <td>{f.name}</td>
                              <td className="num">{formatBytes(f.size)}</td>
                              <td className="muted">{f.modified_at || "—"}</td>
                              <td>{f.physical_reliable ? "是" : "否"}</td>
                              <td className="muted">{f.format_kind || "—"}{f.format_mime ? ` / ${f.format_mime}` : ""}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </>
                ) : null}
              </section>
            )}

            {/* Scan placeholder */}
            <section className="card">
              <h2>扫描</h2>
              <p className="muted">扫描功能将在后续 PR 中实现</p>
            </section>
          </>
        )}
      </main>

      <footer className="app-footer">
        <p>NDG — NAS Data Governance · 只读 Alpha</p>
      </footer>
    </div>
  );
}
