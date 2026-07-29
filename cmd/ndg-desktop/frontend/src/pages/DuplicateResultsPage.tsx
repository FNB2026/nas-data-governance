// Duplicate results page: group list, filters, group detail.
// Groups state is page-local; watches dataRevision for cross-page refresh.

import { useCallback, useEffect, useRef, useState } from "react";
import DuplicateGroups from "../components/DuplicateGroups";
import GroupDetail from "../components/GroupDetail";
import { useProject } from "../state/ProjectContext";
import { hasWailsRuntime, errorText } from "../lib/utils";
import { ListDuplicateGroups, GetGroupDetail } from "../wailsjs/go/wails/API";
import { wails } from "../wailsjs/go/models";

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

  // Initial load + reload on dataRevision change (e.g. after scan completion)
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

  return (
    <div className="page page--duplicate-results">
      <div className="page-header">
        <h2>重复结果</h2>
        <p className="muted">重复文件组与目录语境</p>
      </div>

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

      <GroupDetail
        selectedGroup={selectedGroup}
        detailLoading={detailLoading}
        detailError={detailError}
        onClose={handleCloseDetail}
      />
    </div>
  );
}
