// Display vocabulary only. Review decisions never grant execution approval.
export const DECISIONS = [
  { value: "KEEP_ALL", label: "保留所有文件", explanation: "仅记录保留意图，不移动文件。" },
  { value: "DRAFT_ACTION", label: "制定处理方案", explanation: "仅记录制定方案的意图；仍须另行创建、复核和批准具体方案。" },
  { value: "DEFERRED", label: "稍后再处理", explanation: "暂缓本次复核，不代表已经处理完成。" },
  { value: "REJECTED_SUGGESTION", label: "不采纳此建议", explanation: "记录对本次建议的意见，不保证停止未来建议。" },
  { value: "CROSS_ARCHIVE", label: "标记不同用途的归档副本", explanation: "记录用途关系，不归档、移动或删除文件。" },
  { value: "BACKUP_RELATION", label: "标记为备份关系", explanation: "记录关系意图，不证明备份完整性。" },
] as const;

export type DecisionKind = typeof DECISIONS[number]["value"];
export function decisionLabel(kind: string): string {
  if (!kind) return "待决定";
  if (kind === "PRIMARY_RETENTION") return "历史决定：指定主要保留项";
  return DECISIONS.find((item) => item.value === kind)?.label ?? "未知历史决定（需要核对）";
}

export const REASON_LABELS: Record<string, string> = {
  PROJECT_CLOSED: "请先打开项目。",
  SCHEMA_UPGRADE_REQUIRED: "此数据库需要先完成受支持的结构升级；当前仅可查看。",
  READ_ONLY_CHANNEL: "当前渠道或项目仅可只读查看与预览。",
  RECORDING_UNAVAILABLE: "当前不能记录决定；请核对平台、扫描与恢复锁状态。",
  SAFETY_STATE_CHANGED: "安全状态已变化，未记录此项；请核对恢复锁和扫描状态。",
  GROUP_UNAVAILABLE: "此组当前不可核对，请重新载入。",
  NOT_COMMITTED: "此项尚未持久化；批次未结束时只能核对原请求。",
  ALREADY_RECORDED: "决定和备注未变化。",
  RECORD_ONLY: "仅记录治理意图。",
  RECORDED_ONLY: "已记录意图；未批准、未执行。",
  READ_ONLY: "当前项目只读，不能保存决定。",
  SANDBOX_READ_ONLY: "商店预验证仅允许只读查看。",
  CHANNEL_READ_ONLY: "此平台暂不支持记录决定。",
  RECOVERY_LOCK: "检测到未完成操作，已暂停新的写入；请进入审计与恢复。",
  SCAN_ACTIVE: "扫描正在更新结果，请等待扫描结束后重新预览。",
  SUBJECT_CHANGED: "组成员或建议已经变化，请重新载入并核对。",
  DECISION_CHANGED: "另一窗口已修改决定，请重新载入。",
  PREVIEW_EXPIRED: "预览已过期，请重新预览。",
  PROJECT_CHANGED: "项目已变化，请在当前项目重新选择。",
  IDEMPOTENCY_CONFLICT: "重试标识与请求不一致，不能自动重放。",
  UNSUPPORTED_DECISION: "此决定需要单独核对，当前批量接口不支持。",
  GROUP_NOT_FOUND: "此组已不在当前结果中，请重新载入。",
  SOURCE_CHANGED: "所选组中的源文件已变化，请重新扫描并核对结果。未记录决定。",
  SOURCE_UNAVAILABLE: "所选组中的源文件当前不可读取；未标记为缺失，也未记录决定。",
  AUTHORIZATION_REQUIRED: "源目录授权检查未通过，具体授权原因尚未确认；请核对授权并重新选择，未记录决定。",
  NETWORK_PAUSED: "已登记的网络源当前不可访问，不能据此确认服务器断开；请核对连接和源身份后重新预览。未记录决定。",
  SOURCE_LIMIT_EXCEEDED: "所选文件超过本次核验预算；请缩小选择。未记录决定。",
  SOURCE_UNVERIFIED: "无法确认所选源文件当前内容；未记录决定。",
  SOURCE_PLATFORM_PROOF_UNAVAILABLE: "此版本尚未实现当前平台所需的安全挂载边界核验；未读取源文件，不能记录决定。重试不会解除此限制。",
  SOURCE_VERIFICATION_CANCELLED: "本次源核验已取消，尚无完整结果；需要时重新预览，未记录决定。",
  SOURCE_VERIFICATION_TIMEOUT: "本次源核验超时，未完成内容确认；不能据此判定断网或文件缺失。请核对源状态后重新预览。",
  SOURCE_CATALOG_UNAVAILABLE: "无法读取项目中的源登记或成员记录；请核对项目状态，未记录决定。",
  SOURCE_AUTHORIZATION_MISMATCH: "授权返回的源目录与登记不一致；请核对身份并重新选择授权，未记录决定。",
  SOURCE_PERMISSION_DENIED: "系统拒绝读取所选源文件；请检查权限后重新预览，未记录决定。",
  SOURCE_MOUNT_BOUNDARY_REJECTED: "读取被挂载边界检查拒绝；请核对源根目录及嵌套挂载，不会自动绕过。",
  SOURCE_SYMLINK_REJECTED: "读取被符号链接检查拒绝；请核对源目录与文件绑定，不会跟随链接。",
  SOURCE_VERIFICATION_API_UNAVAILABLE: "系统不支持本次核验所需的文件访问接口；不能使用较弱检查代替，未记录决定。",
  SOURCE_READ_FAILED: "源读取未完成，具体原因尚未确认；请人工核对，不自动判定断网或文件缺失。",
  SOURCE_FILE_UNAVAILABLE: "登记的源文件当前不可访问；请核对路径绑定，未标记 missing，也未记录决定。",
  SOURCE_REGISTRATION_UNAVAILABLE: "源登记缺失或根目录无效；请核对项目的数据源登记，未记录决定。",
  SOURCE_CONTENT_CHANGED: "源文件 SHA-256 与索引不同；请重新扫描并核对，未记录决定。",
  SOURCE_IDENTITY_OR_METADATA_CHANGED: "源文件身份、大小、修改时间或路径绑定已变化；当前证据无法再细分，请重新扫描并核对。",
  UNAVAILABLE: "安全接口暂不可用；可以查看结果，不能提交决定。",
};

export function reasonLabel(code: string): string {
  return Object.prototype.hasOwnProperty.call(REASON_LABELS, code)
    ? REASON_LABELS[code]
    : "未完成安全校验，请重新载入并人工核对。";
}

export const OUTCOME_LABELS: Record<string, string> = {
  SAVED: "已记录（未批准、未执行）",
  UNCHANGED: "决定未变化",
  CONFLICT: "数据或决定发生冲突",
  BLOCKED: "安全限制阻断",
  FAILED: "未能记录",
};

export interface ReviewGroup {
  group_id: string;
  storage_id: string;
  sha256: string;
  sample_path: string;
  size: number;
  path_count: number;
  decision_type: string;
  decision_revision: number;
  subject_version: string;
  risk: string;
  role: string;
  safety_flags: string[];
}

export interface GovernanceQuery {
  storage_id: string;
  search: string;
  decision_type: string;
  min_size: number;
  max_size: number;
  sort: "size" | "path_count" | "group_id";
  descending: boolean;
  page: number;
  page_size: number;
}

export interface ReviewPage {
  items: ReviewGroup[];
  total_count: number;
  page: number;
  page_size: number;
  unavailable_filters: string[];
}

export interface GovernanceCapabilities {
  can_record_decisions: boolean;
  can_preview: boolean;
  recovery_lock_active: boolean;
  scan_active: boolean;
  max_batch_size: number;
  reason_codes: string[];
  project_scope: string;
}

export type ReviewSubject = Pick<ReviewGroup, "group_id" | "storage_id" | "sha256" | "subject_version" | "decision_revision">;
export interface PreviewRequest { items: ReviewSubject[]; decision_type: DecisionKind; reason: string }
export interface PreviewItem extends ReviewSubject {
  eligibility: "ELIGIBLE" | "BLOCKED" | "CONFLICT";
  reason_code: string;
  source_freshness?: string;
  source_checked_at?: string;
}
export interface ReviewPreview {
  preview_token: string;
  expires_at: string;
  project_scope: string;
  items: PreviewItem[];
  records_only: boolean;
}

// Only source-verified eligible groups may start a new records-only commit.
// A blocked group may remain in the same preview while a MATCH group is
// recorded; an eligible group with missing or non-MATCH freshness fails the
// whole new submission closed. Existing ambiguous submissions are reconciled
// separately with their original idempotency key.
export function canCommitGovernancePreview(preview: ReviewPreview | null): boolean {
  if (!preview?.preview_token) return false;
  const eligible = preview.items.filter((item) => item.eligibility === "ELIGIBLE");
  return eligible.length > 0 && eligible.every((item) => item.source_freshness === "MATCH");
}
export type Outcome = "SAVED" | "UNCHANGED" | "CONFLICT" | "BLOCKED" | "FAILED";
export interface BatchResult {
  batch_id: string;
  project_scope: string;
  status: "COMPLETE" | "IN_PROGRESS" | "CANCELLED";
  requested_count: number;
  counts: { total: number; saved: number; unchanged: number; conflict: number; blocked: number; failed: number };
  items: { group_id: string; status: Outcome; reason_code: string; decision_revision: number }[];
}

export function subject(group: ReviewGroup): ReviewSubject {
  return { group_id: group.group_id, storage_id: group.storage_id, sha256: group.sha256,
    subject_version: group.subject_version, decision_revision: group.decision_revision };
}

export interface BatchSummary { batch_id: string; status: BatchResult["status"]; requested_count: number; recorded_count: number; created_at: string }
export interface BatchHistoryPage { project_scope: string; items: BatchSummary[]; total_count: number; page: number; page_size: number }

export function validBatchHistory(value: BatchHistoryPage, expectedPage: number): boolean {
  if (!value || typeof value.project_scope !== "string" || !value.project_scope || !Array.isArray(value.items) ||
      !Number.isSafeInteger(value.total_count) || value.total_count < 0 || value.page !== expectedPage || value.page_size !== 10 ||
      value.items.length > value.page_size || value.items.length !== Math.max(0, Math.min(value.page_size, value.total_count - (value.page - 1) * value.page_size))) return false;
  if (value.items.some((item) => !item || typeof item !== "object")) return false;
  return new Set(value.items.map((item) => item.batch_id)).size === value.items.length && value.items.every((item) =>
    typeof item.batch_id === "string" && /^[a-zA-Z0-9_-]{16,128}$/.test(item.batch_id) &&
    ["COMPLETE", "IN_PROGRESS", "CANCELLED"].includes(item.status) && Number.isSafeInteger(item.requested_count) &&
    item.requested_count > 0 && item.requested_count <= 100 && Number.isSafeInteger(item.recorded_count) &&
    item.recorded_count >= 0 && item.recorded_count <= item.requested_count &&
    (item.status !== "COMPLETE" || item.recorded_count === item.requested_count) &&
    typeof item.created_at === "string" && Number.isFinite(Date.parse(item.created_at)));
}

// Validate accounting before reporting success; transport success is insufficient.
export function validBatchResult(result: BatchResult, expectedIds: string[]): boolean {
  if (!result || !["COMPLETE", "IN_PROGRESS", "CANCELLED"].includes(result.status) || !Array.isArray(result.items) || !result.counts) return false;
  const expected = new Set(expectedIds);
  const received = new Set(result.items.map((item) => item.group_id));
  if (expected.size !== expectedIds.length || received.size !== result.items.length ||
      result.requested_count !== expected.size || result.items.length !== expected.size ||
      result.counts.total !== expected.size || result.items.some((item) => !expected.has(item.group_id) || !Number.isSafeInteger(item.decision_revision) || item.decision_revision < 0)) return false;
  const counts = { saved: 0, unchanged: 0, conflict: 0, blocked: 0, failed: 0 };
  for (const item of result.items) {
    const key = item.status.toLowerCase() as keyof typeof counts;
    if (!Object.prototype.hasOwnProperty.call(counts, key)) return false;
    counts[key]++;
  }
  return Object.entries(counts).every(([key, value]) => result.counts[key as keyof typeof counts] === value);
}

// Explicit selection survives pagination; it never becomes an implicit query set.
export interface SelectedReview { group: ReviewGroup; filterScope: string; dataRevision: number; page: number }
export function filterScope(query: GovernanceQuery): string {
  return JSON.stringify([query.storage_id, query.search, query.decision_type, query.min_size, query.max_size]);
}
