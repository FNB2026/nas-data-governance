// Duplicate results page: three-zone split layout.
// Top: capacity summary bar + governance entry
// Left: group list with filters (scrollable)
// Right: evidence inspector (scrollable)

import { useCallback, useEffect, useRef, useState, useMemo } from "react";
import DuplicateGroups from "../components/DuplicateGroups";
import GroupDetail from "../components/GroupDetail";
import { useProject } from "../state/ProjectContext";
import { hasWailsRuntime, errorText, formatBytes } from "../lib/utils";
import { ListDuplicateGroups, GetGroupDetail } from "../wailsjs/go/wails/API";
import { wails } from "../wailsjs/go/models";
import {
  computeCapacity,
} from "../lib/evidence";

export default function DuplicateResultsPage() {
  const { storages, dataRevision } = useProject();

  // Groups state (page-local)
  const [groups, setGroups] = useState<wails.GroupSummary[]>([]);
  const [nextCursor, setNextCursor] = useState("");
  const [totalCount, setTotalCount] = useState(0);
  const [groupsLoading, setGroupsLoading] = useState(false);
  const [groupsError, setGroupsError] = useState<string | null>(null);
  const [storageFilter, setStorageFilter] = useState("");
  const [minReclaimableMiB, setMinReclaimableMiB] = useState("");
  const [appliedStorageFilter, setAppliedStorageFilter] = useState("");
  const [appliedMinimumBytes, setAppliedMinimumBytes] = useState(0);
  const groupsRequestInFlight = useRef(false);

  // Group detail (page-local)
  const [selectedGroup, setSelectedGroup] = useState<wails.GroupDetailResponse | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState<string | null>(null);

  const loadGroups = useCallback(async (
    cursor: string,
    storageId: string,
    minReclaimableBytes: number,
  ) => {
    if (!hasWailsRuntime() || groupsRequestInFlight.current) return;
    groupsRequestInFlight.current = true;
    setGroupsLoading(true);
    try {
      const resp = await ListDuplicateGroups({
        storage_id: storageId,
        page_size: 20,
        cursor,
        min_reclaimable_bytes: minReclaimableBytes,
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
      groupsRequestInFlight.current = false;
      setGroupsLoading(false);
    }
  }, []);

  // Initial load + reload on dataRevision change
  useEffect(() => {
    if (hasWailsRuntime()) {
      setSelectedGroup(null);
      void loadGroups("", appliedStorageFilter, appliedMinimumBytes);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dataRevision]);

  const handleLoadMore = () => {
    if (nextCursor && !groupsLoading) {
      loadGroups(nextCursor, appliedStorageFilter, appliedMinimumBytes);
    }
  };

  const parsedMinimumBytes = (): number | null => {
    if (!minReclaimableMiB.trim()) return 0;
    const value = Number(minReclaimableMiB);
    if (!Number.isFinite(value) || value < 0) return null;
    return Math.floor(value * 1024 * 1024);
  };

  const handleApplyFilters = () => {
    const minimum = parsedMinimumBytes();
    if (minimum === null) {
      setGroupsError("最小可回收空间必须是非负数");
      return;
    }
    setSelectedGroup(null);
    setAppliedStorageFilter(storageFilter);
    setAppliedMinimumBytes(minimum);
    void loadGroups("", storageFilter, minimum);
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

  // Aggregate capacity across all loaded groups (page-local, not total)
  const aggregateCapacity = useMemo(() => {
    let totalLogical = 0;
    let totalReclaimable = 0;
    let totalHardlinkAlias = 0;
    for (const g of groups) {
      const cap = computeCapacity(
        g.size,
        g.path_count,
        g.physical_copy_count,
        g.hardlink_alias_count,
        g.physical_reclaimable_bytes,
      );
      totalLogical += cap.totalLogical;
      totalReclaimable += cap.physicalReclaimable;
      totalHardlinkAlias += cap.hardlinkAliasBytes;
    }
    return { totalLogical, totalReclaimable, totalHardlinkAlias };
  }, [groups]);

  return (
    <div className="page page--duplicate-results dup-page">
      <div className="page-header">
        <h2>重复结果</h2>
        <p className="muted">重复文件组与目录语境</p>
      </div>

      {/* Capacity summary bar */}
      {groups.length > 0 && (
        <div className="dup-summary-bar">
          <div className="dup-summary-stats">
            <div className="dup-summary-stat">
              <span className="dup-summary-label">重复组（全量）</span>
              <span className="dup-summary-value">{totalCount}</span>
            </div>
            <div className="dup-summary-stat">
              <span className="dup-summary-label">逻辑总量（已加载 {groups.length} 组）</span>
              <span className="dup-summary-value">{formatBytes(aggregateCapacity.totalLogical)}</span>
            </div>
            <div className="dup-summary-stat dup-summary-stat--reclaimable">
              <span className="dup-summary-label">可回收物理空间（已加载）</span>
              <span className="dup-summary-value">{formatBytes(aggregateCapacity.totalReclaimable)}</span>
            </div>
            <div className="dup-summary-stat">
              <span className="dup-summary-label">硬链接别名字节（已加载）</span>
              <span className="dup-summary-value">{formatBytes(aggregateCapacity.totalHardlinkAlias)}</span>
            </div>
          </div>
        </div>
      )}

      {/* Three-zone split: left list + right evidence */}
      <div className="dup-split">
        <div className="dup-split-left">
          <DuplicateGroups
            groups={groups}
            totalCount={totalCount}
            groupsLoading={groupsLoading}
            groupsError={groupsError}
            nextCursor={nextCursor}
            storages={storages}
            storageFilter={storageFilter}
            minReclaimableMiB={minReclaimableMiB}
            detailLoading={detailLoading}
            selectedGroup={selectedGroup}
            onStorageFilterChange={setStorageFilter}
            onMinReclaimableMiBChange={setMinReclaimableMiB}
            onApplyFilters={handleApplyFilters}
            onLoadMore={handleLoadMore}
            onSelectGroup={handleSelectGroup}
          />
        </div>
        <div className="dup-split-right">
          <GroupDetail
            selectedGroup={selectedGroup}
            detailLoading={detailLoading}
            detailError={detailError}
            onClose={handleCloseDetail}
          />
        </div>
      </div>
    </div>
  );
}
