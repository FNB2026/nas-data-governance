import { wails } from "../wailsjs/go/models";
import { formatBytes } from "../lib/utils";

export interface GroupDetailProps {
  selectedGroup: wails.GroupDetailResponse | null;
  detailLoading: boolean;
  detailError: string | null;
  onClose: () => void;
}

export default function GroupDetail({
  selectedGroup,
  detailLoading,
  detailError,
  onClose,
}: GroupDetailProps) {
  if (!selectedGroup && !detailLoading && !detailError) return null;

  return (
    <section className="card card--full group-detail">
      <div className="card-header-row">
        <h2>组详情</h2>
        <button className="btn-sm secondary" onClick={onClose}>关闭</button>
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
  );
}
