// Evidence inspector: displays group detail with path segmentation,
// physical identity grouping, and hardlink detection evidence.

import { wails } from "../wailsjs/go/models";
import { formatBytes, shortHash, formatDateTime } from "../lib/utils";
import {
  computeCapacity,
  deriveRiskLevel,
  riskLabel,
  riskBadgeClass,
  groupByPhysicalIdentity,
  pathSegments,
  compactPath,
  isHardlinkAlias,
} from "../lib/evidence";

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

  const cap = selectedGroup
    ? computeCapacity(
        selectedGroup.size,
        selectedGroup.path_count,
        selectedGroup.physical_copy_count,
        selectedGroup.hardlink_alias_count,
        selectedGroup.physical_reclaimable_bytes,
      )
    : null;

  const risk = selectedGroup
    ? deriveRiskLevel(
        selectedGroup.physical_copy_count,
        selectedGroup.hardlink_alias_count,
        selectedGroup.physical_reclaimable_bytes,
      )
    : null;

  const physGroups = selectedGroup?.files
    ? groupByPhysicalIdentity(selectedGroup.files)
    : [];

  return (
    <section className="card card--full group-detail evidence-inspector">
      <div className="card-header-row">
        <h2>证据检查器</h2>
        <button className="btn-sm secondary" onClick={onClose}>关闭</button>
      </div>
      {detailLoading ? (
        <p className="muted">加载中…</p>
      ) : detailError ? (
        <p className="error" role="alert">{detailError}</p>
      ) : selectedGroup && cap && risk ? (
        <>
          {/* Summary bar */}
          <div className="evidence-summary">
            <div className="evidence-summary-row">
              <span className={riskBadgeClass(risk)}>{riskLabel(risk)}</span>
              <span className="muted">SHA-256：<span className="mono">{shortHash(selectedGroup.sha256)}</span></span>
              <span className="muted">存储：<span className="mono">{selectedGroup.storage_id}</span></span>
            </div>
            <div className="evidence-summary-row">
              <span><strong>文件大小：</strong>{formatBytes(selectedGroup.size)}</span>
              <span><strong>路径数：</strong>{selectedGroup.path_count}</span>
              <span><strong>物理副本：</strong>{cap.physicalCopyCount}</span>
              <span><strong>硬链接别名：</strong>{cap.hardlinkAliasCount}</span>
            </div>
            <div className="capacity-bar">
              <div className="capacity-item">
                <span className="capacity-label">逻辑总量</span>
                <span className="capacity-value">{formatBytes(cap.totalLogical)}</span>
              </div>
              <div className="capacity-item capacity-item--reclaimable">
                <span className="capacity-label">可回收物理空间</span>
                <span className="capacity-value">{formatBytes(cap.physicalReclaimable)}</span>
              </div>
              <div className="capacity-item capacity-item--hardlink">
                <span className="capacity-label">硬链接别名字节</span>
                <span className="capacity-value">{formatBytes(cap.hardlinkAliasBytes)}</span>
              </div>
            </div>
          </div>

          {/* Physical identity groups */}
          <div className="evidence-section">
            <h3 className="evidence-section-title">
              物理身份分组
              <span className="muted evidence-section-count">{physGroups.length} 组</span>
            </h3>
            <p className="evidence-hint muted">
              同一物理身份（设备:Inode）的文件互为硬链接，共享存储块，不可回收空间。
            </p>
            <div className="table-wrap">
              <table className="data-table evidence-table">
                <thead>
                  <tr>
                    <th>路径分段</th>
                    <th>文件名</th>
                    <th className="num">大小</th>
                    <th>修改时间</th>
                    <th>物理身份</th>
                    <th>硬链接</th>
                    <th>格式</th>
                  </tr>
                </thead>
                <tbody>
                  {selectedGroup.files.map((f, i) => {
                    const segments = pathSegments(f.path);
                    const ctx = compactPath(f.path, 3);
                    const hardlink = isHardlinkAlias(f);
                    const physId = f.physical_reliable && f.physical_inode
                      ? `${f.physical_device}:${f.physical_inode}`
                      : "不可靠";
                    return (
                      <tr key={`${f.path}-${i}`} className={hardlink ? "row-hardlink" : ""}>
                        <td className="path-cell" title={f.path}>
                          <span className="path-segments" title={f.path}>
                            {segments.slice(-4, -1).map((seg, idx) => (
                              <span key={idx} className="path-seg">{seg}</span>
                            ))}
                            {segments.length > 4 && <span className="path-seg path-seg--ellipsis">…</span>}
                          </span>
                          <span className="path-compact muted">{ctx}</span>
                        </td>
                        <td>{f.name}</td>
                        <td className="num">{formatBytes(f.size)}</td>
                        <td className="muted">{formatDateTime(f.modified_at)}</td>
                        <td className="mono phys-id">{physId}</td>
                        <td>
                          {hardlink ? (
                            <span className="hardlink-flag" title={`链接数 ${f.physical_link_count}`}>
                              🔗 {f.physical_link_count}
                            </span>
                          ) : f.physical_reliable ? (
                            <span className="muted">—</span>
                          ) : (
                            <span className="muted" title="SMB/NFS/FUSE 等文件系统不支持 inode 信任">⚠</span>
                          )}
                        </td>
                        <td className="muted">{f.format_kind || "—"}{f.format_mime ? ` / ${f.format_mime}` : ""}</td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </div>

          {/* Physical identity grouping summary */}
          {physGroups.length > 1 && (
            <div className="evidence-section">
              <h3 className="evidence-section-title">硬链接关系图</h3>
              <div className="hardlink-groups">
                {physGroups.map((pg, idx) => {
                  const isHardlinkGroup = pg.length > 1;
                  return (
                    <div key={idx} className={`hardlink-group ${isHardlinkGroup ? "hardlink-group--linked" : ""}`}>
                      <div className="hardlink-group-header">
                        {isHardlinkGroup ? (
                          <span className="hardlink-flag">🔗 硬链接组 ({pg.length} 路径)</span>
                        ) : (
                          <span className="muted">独立物理副本</span>
                        )}
                        {pg[0].physical_reliable && pg[0].physical_inode && (
                          <span className="mono muted phys-id">
                            {pg[0].physical_device}:{pg[0].physical_inode}
                          </span>
                        )}
                      </div>
                      <ul className="hardlink-paths">
                        {pg.map((f, fi) => (
                          <li key={fi} className="hardlink-path" title={f.path}>
                            {f.path}
                          </li>
                        ))}
                      </ul>
                    </div>
                  );
                })}
              </div>
            </div>
          )}
        </>
      ) : null}
    </section>
  );
}
