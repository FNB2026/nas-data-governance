import { wails } from "../wailsjs/go/models";
import { formatBytes, shortHash } from "../lib/utils";
import {
  computeCapacity,
  deriveRiskLevel,
  riskLabel,
  riskBadgeClass,
  fileName,
} from "../lib/evidence";

export interface DuplicateGroupsProps {
  groups: wails.GroupSummary[];
  totalCount: number;
  groupsLoading: boolean;
  groupsError: string | null;
  nextCursor: string;
  storages: wails.StorageInfo[];
  storageFilter: string;
  minReclaimableMiB: string;
  detailLoading: boolean;
  selectedGroup: wails.GroupDetailResponse | null;
  onStorageFilterChange: (value: string) => void;
  onMinReclaimableMiBChange: (value: string) => void;
  onApplyFilters: () => void;
  onLoadMore: () => void;
  onSelectGroup: (storageId: string, sha256: string) => void;
}

export default function DuplicateGroups({
  groups,
  totalCount,
  groupsLoading,
  groupsError,
  nextCursor,
  storages,
  storageFilter,
  minReclaimableMiB,
  detailLoading,
  selectedGroup,
  onStorageFilterChange,
  onMinReclaimableMiBChange,
  onApplyFilters,
  onLoadMore,
  onSelectGroup,
}: DuplicateGroupsProps) {
  return (
    <div className="dup-groups-panel">
      {/* Filter bar */}
      <div className="filter-row dup-filter-row" aria-label="重复组筛选">
        <label>
          存储
          <select
            value={storageFilter}
            onChange={(event) => onStorageFilterChange(event.target.value)}
          >
            <option value="">全部存储</option>
            {storages.map((storage) => (
              <option key={storage.id} value={storage.id}>{storage.id}</option>
            ))}
          </select>
        </label>
        <label>
          最小可回收（MiB）
          <input
            type="number"
            min="0"
            step="1"
            value={minReclaimableMiB}
            onChange={(event) => onMinReclaimableMiBChange(event.target.value)}
            placeholder="0"
          />
        </label>
        <button
          className="btn-sm"
          disabled={groupsLoading}
          onClick={onApplyFilters}
        >
          应用筛选
        </button>
      </div>

      {/* Group list */}
      {groupsError ? (
        <p className="error" role="alert">{groupsError}</p>
      ) : groups.length === 0 && !groupsLoading ? (
        <div className="empty-state">
          <p className="muted">未检测到重复文件，或尚未扫描</p>
          <p className="muted">完成扫描后，内容相同的文件将在此显示，附带目录语境与物理身份证据。</p>
        </div>
      ) : (
        <>
          <div className="dup-list-wrap">
            <table className="data-table dup-table">
              <thead>
                <tr>
                  <th>风险</th>
                  <th>代表文件</th>
                  <th className="num">大小</th>
                  <th className="num">路径</th>
                  <th className="num">物理副本</th>
                  <th className="num">硬链接</th>
                  <th className="num">可回收</th>
                </tr>
              </thead>
              <tbody>
                {groups.map((g) => {
                  const key = `${g.storage_id}/${g.sha256}`;
                  const isSelected =
                    selectedGroup &&
                    selectedGroup.sha256 === g.sha256 &&
                    selectedGroup.storage_id === g.storage_id;
                  const cap = computeCapacity(
                    g.size,
                    g.path_count,
                    g.physical_copy_count,
                    g.hardlink_alias_count,
                    g.physical_reclaimable_bytes,
                  );
                  const risk = deriveRiskLevel(
                    g.physical_copy_count,
                    g.hardlink_alias_count,
                    g.physical_reclaimable_bytes,
                  );
                  const repName = g.sample_path ? fileName(g.sample_path) : shortHash(g.sha256);

                  return (
                    <tr
                      key={key}
                      className={`dup-row ${isSelected ? "row-selected" : ""} ${groupsLoading ? "dup-row--loading" : ""}`}
                      onClick={() => !detailLoading && onSelectGroup(g.storage_id, g.sha256)}
                      style={{ cursor: detailLoading ? "wait" : "pointer" }}
                    >
                      <td>
                        <span className={riskBadgeClass(risk)}>{riskLabel(risk)}</span>
                      </td>
                      <td className="path-cell" title={g.sample_path || g.sha256}>
                        {repName}
                      </td>
                      <td className="num">{formatBytes(g.size)}</td>
                      <td className="num">{g.path_count}</td>
                      <td className="num">
                        {cap.physicalCopyCount}
                        {cap.physicalCopyCount <= 1 && cap.hardlinkAliasCount === 0 && (
                          <span className="muted" title="仅一个物理副本，无可回收空间"> ⚠</span>
                        )}
                      </td>
                      <td className="num">
                        {cap.hardlinkAliasCount > 0 ? (
                          <span className="hardlink-flag" title="存在硬链接别名，共享存储块">🔗 {cap.hardlinkAliasCount}</span>
                        ) : (
                          <span className="muted">—</span>
                        )}
                      </td>
                      <td className="num">
                        {cap.physicalReclaimable > 0 ? (
                          <strong>{formatBytes(cap.physicalReclaimable)}</strong>
                        ) : (
                          <span className="muted" title="硬链接组无可回收物理空间">0 B</span>
                        )}
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
                onClick={onLoadMore}
              >
                {groupsLoading ? "加载中…" : "加载更多"}
              </button>
            </div>
          )}
          {totalCount > 0 && (
            <div className="dup-count">
              <span className="muted">共 {totalCount} 组</span>
            </div>
          )}
        </>
      )}
    </div>
  );
}
