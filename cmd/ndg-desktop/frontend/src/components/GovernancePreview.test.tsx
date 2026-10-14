// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import GovernancePreview from "./GovernancePreview";
import type { ReviewPreview, SelectedReview } from "../lib/governance";

afterEach(cleanup);

const subject = { group_id: "synthetic-group", storage_id: "synthetic-storage", sha256: "a".repeat(64), subject_version: "synthetic-version", decision_revision: 0 };
const selection = new Map<string, SelectedReview>([[subject.group_id, { group: { ...subject, sample_path: "/synthetic/private-name", size: 20, path_count: 2, decision_type: "", risk: "UNKNOWN", role: "unknown", safety_flags: [] }, filterScope: "synthetic", dataRevision: 0, page: 1 }]]);
function preview(sourceFreshness: string, eligibility: "ELIGIBLE" | "BLOCKED", reasonCode: string): ReviewPreview {
  return { preview_token: eligibility === "ELIGIBLE" ? "synthetic-token" : "", expires_at: "2026-10-14T00:00:00Z", project_scope: "synthetic-project", records_only: true,
    items: [{ ...subject, eligibility, reason_code: reasonCode, source_freshness: sourceFreshness, source_checked_at: "2026-10-14T00:00:00Z" }] };
}

it("shows point-in-time source match without implying later execution authority", () => {
  render(<GovernancePreview preview={preview("MATCH", "ELIGIBLE", "RECORD_ONLY")} selection={selection} kind="KEEP_ALL" canRecord busy={false} ambiguous={false} onClose={vi.fn()} onCommit={vi.fn()} canQueryReceipt={false} onQueryReceipt={vi.fn()} note={null} />);
  expect(screen.getByText(/预览时所选文件 SHA-256 与索引一致；保存时会再次核对/)).toBeVisible();
  expect(screen.queryByText(/删除|移动或修改源文件/)).toBeVisible();
  expect(document.body.textContent).not.toContain("private-name");
});

it("shows a safe changed-source reason and disables commit", () => {
  render(<GovernancePreview preview={preview("SOURCE_CHANGED", "BLOCKED", "SOURCE_CHANGED")} selection={selection} kind="KEEP_ALL" canRecord busy={false} ambiguous={false} onClose={vi.fn()} onCommit={vi.fn()} canQueryReceipt={false} onQueryReceipt={vi.fn()} note={null} />);
  expect(screen.getByText("所选组中的源文件已变化，请重新扫描并核对结果。未记录决定。")).toBeVisible();
  expect(screen.getByRole("button", { name: "保存决定（未批准、未执行）" })).toBeDisabled();
});
