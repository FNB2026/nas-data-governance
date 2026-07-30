// Utility functions for duplicate group evidence analysis.
// Derives directory context and physical identity from existing data fields.

import type { wails } from "../wailsjs/go/models";

/**
 * Split a file path into segments for directory context display.
 * Returns up to 4 levels of parent directories + filename.
 */
export function pathSegments(path: string): string[] {
  if (!path) return [];
  // Normalize and split
  const parts = path.split("/").filter((s) => s.length > 0);
  return parts;
}

/**
 * Get parent directory path (up to N levels).
 */
export function parentDir(path: string, levels = 1): string {
  const segs = pathSegments(path);
  if (segs.length <= levels) return "/";
  return "/" + segs.slice(0, segs.length - levels).join("/");
}

/**
 * Get the last N path segments as a compact context string.
 */
export function compactPath(path: string, segments = 3): string {
  const segs = pathSegments(path);
  if (segs.length <= segments) return "/" + segs.join("/");
  return "…/" + segs.slice(-segments).join("/");
}

/**
 * Extract a representative file name from a path.
 */
export function fileName(path: string): string {
  const segs = pathSegments(path);
  return segs.length > 0 ? segs[segs.length - 1] : path;
}

/**
 * Determine if a file is a hardlink alias (link count > 1).
 */
export function isHardlinkAlias(file: wails.FileItem): boolean {
  return (file.physical_link_count ?? 0) > 1;
}

/**
 * Check if two files share the same physical identity (same device + inode).
 */
export function samePhysicalIdentity(
  a: wails.FileItem,
  b: wails.FileItem,
): boolean {
  return (
    a.physical_device === b.physical_device &&
    a.physical_inode === b.physical_inode &&
    a.physical_inode !== undefined &&
    a.physical_inode !== 0
  );
}

/**
 * Group files by physical identity (device:inode).
 * Returns groups of files that are hardlinks of each other.
 */
export function groupByPhysicalIdentity(
  files: wails.FileItem[],
): wails.FileItem[][] {
  const groups: Map<string, wails.FileItem[]> = new Map();
  for (const f of files) {
    if (!f.physical_reliable || f.physical_inode === undefined || f.physical_inode === 0) {
      // Unreliable identity — each gets its own group
      groups.set(`unreliable-${f.path}`, [f]);
      continue;
    }
    const key = `${f.physical_device}:${f.physical_inode}`;
    const existing = groups.get(key);
    if (existing) {
      existing.push(f);
    } else {
      groups.set(key, [f]);
    }
  }
  return Array.from(groups.values());
}

/**
 * Compute capacity breakdown for a duplicate group.
 * - totalLogical: size * path_count (sum of all copies)
 * - physicalReclaimable: bytes that can be freed by removing redundant physical copies
 * - hardlinkReclaimable: always 0 (hardlinks share the same blocks)
 */
export interface CapacityBreakdown {
  totalLogical: number;
  physicalReclaimable: number;
  hardlinkAliasBytes: number;
  physicalCopyCount: number;
  hardlinkAliasCount: number;
}

export function computeCapacity(
  size: number,
  pathCount: number,
  physicalCopyCount: number,
  hardlinkAliasCount: number,
  physicalReclaimableBytes: number,
): CapacityBreakdown {
  return {
    totalLogical: size * pathCount,
    physicalReclaimable: physicalReclaimableBytes,
    hardlinkAliasBytes: hardlinkAliasCount > 0 ? size * hardlinkAliasCount : 0,
    physicalCopyCount,
    hardlinkAliasCount,
  };
}

/**
 * Derive a risk indicator from group properties.
 * Until the backend provides a risk DTO with directory role, business
 * anchor, and protection rules, we return "UNASSESSED" to avoid
 * misleading the user with incomplete heuristics.
 */
export function deriveRiskLevel(
  _physicalCopyCount: number,
  _hardlinkAliasCount: number,
  _reclaimableBytes: number,
): "UNASSESSED" {
  return "UNASSESSED";
}

export function riskLabel(risk: "LOW" | "MEDIUM" | "HIGH" | "UNASSESSED"): string {
  return { LOW: "低", MEDIUM: "中", HIGH: "高", UNASSESSED: "未评估" }[risk];
}

export function riskBadgeClass(risk: "LOW" | "MEDIUM" | "HIGH" | "UNASSESSED"): string {
  return `risk-badge risk-badge--${risk.toLowerCase()}`;
}
