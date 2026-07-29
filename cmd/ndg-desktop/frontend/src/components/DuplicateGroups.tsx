import { wails } from "../wailsjs/go/models";
import { formatBytes, shortHash } from "../lib/utils";

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
    <section className="card card--full">
      <div className="card-header-row">
        <h2>重复文件组</h2>
        {totalCount > 0 && (
          <span className="count-badge">共 {totalCount} 组</span>
        )}
      </div>
      <div className="filter-row" aria-label="重复组筛选">
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
                          onClick={() => onSelectGroup(g.storage_id, g.sha256)}
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
                onClick={onLoadMore}
              >
                {groupsLoading ? "加载中…" : "加载更多"}
              </button>
            </div>
          )}
        </>
      )}
    </section>
  );
}
