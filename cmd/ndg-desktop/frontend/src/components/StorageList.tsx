import { wails } from "../wailsjs/go/models";

export interface StorageListProps {
  storages: wails.StorageInfo[];
  storagesError: string | null;
}

export default function StorageList({ storages, storagesError }: StorageListProps) {
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
  );
}
