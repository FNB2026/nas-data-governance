import { wails } from "../wailsjs/go/models";
import { useProject } from "../state/ProjectContext";

export interface StorageListProps {
  storages: wails.StorageInfo[];
  storagesError: string | null;
  sourceProfiles: Record<string, wails.SourcePreflightDTO | null>;
}

export default function StorageList({ storages, storagesError, sourceProfiles }: StorageListProps) {
  const { displayPath } = useProject();

  return (
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
                <th>连接与扫描建议</th>
                <th>注册时间</th>
              </tr>
            </thead>
            <tbody>
              {storages.map((s) => (
                <tr key={s.id}>
                  <td className="mono">{s.id}</td>
                  <td className="path-cell" title={displayPath(s.root_path)}>{displayPath(s.root_path)}</td>
                  <td>{s.kind}</td>
                  <td>{renderSourceProfile(sourceProfiles, s.id)}</td>
                  <td className="muted">{s.created_at || "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

function renderSourceProfile(
  profiles: Record<string, wails.SourcePreflightDTO | null>,
  storageID: string,
) {
  if (!(storageID in profiles)) {
    return <span className="muted">检测中…</span>;
  }
  const profile = profiles[storageID];
  if (!profile || profile.status !== "online") {
    return <span className="state-badge state-badge--failed">不可用</span>;
  }
  const sourceKind = profile.network ? "网络存储" : "本地存储";
  return (
    <div>
      <span className="state-badge state-badge--completed">在线</span>
      <div className="muted">
        {sourceKind} · {profile.filesystem_type || "unknown"} · {profile.latency_ms} ms
      </div>
      <div className="muted">建议扫描并发 {profile.recommended_workers}</div>
      {profile.network && profile.latency_ms >= 250 && (
        <div className="warn">高延迟链路：建议保持单并发，并检查 VPN/Tailscale 是否正在中继</div>
      )}
      {!profile.physical_identity_reliable && (
        <div className="muted">物理身份按网络存储保守处理</div>
      )}
    </div>
  );
}
