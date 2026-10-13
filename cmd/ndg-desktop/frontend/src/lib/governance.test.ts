import { describe, expect, it } from "vitest";
import { DECISIONS, decisionLabel, reasonLabel, validBatchResult, type BatchResult } from "./governance";

function receipt(): BatchResult {
  return { batch_id: "synthetic-batch", project_scope: "synthetic-project", status: "COMPLETE", requested_count: 2,
    counts: { total: 2, saved: 1, unchanged: 0, conflict: 1, blocked: 0, failed: 0 }, items: [
      { group_id: "g1", status: "SAVED", reason_code: "", decision_revision: 1 },
      { group_id: "g2", status: "CONFLICT", reason_code: "SUBJECT_CHANGED", decision_revision: 2 },
    ] };
}

describe("review vocabulary and outcome proof", () => {
  it("preserves all six enums and explains the historical seventh without reinterpreting it", () => {
    expect(DECISIONS.map((entry) => entry.value)).toEqual(["KEEP_ALL", "DRAFT_ACTION", "DEFERRED", "REJECTED_SUGGESTION", "CROSS_ARCHIVE", "BACKUP_RELATION"]);
    expect(decisionLabel("PRIMARY_RETENTION")).toContain("历史决定");
    expect(decisionLabel("unexpected_private_marker")).not.toContain("private_marker");
    expect(reasonLabel("/synthetic/private-marker/path")).not.toContain("private-marker");
  });
  it("checks one outcome per requested group and exact accounting", () => {
    expect(validBatchResult(receipt(), ["g1", "g2"])).toBe(true);
    const duplicate = receipt(); duplicate.items[1].group_id = "g1";
    expect(validBatchResult(duplicate, ["g1", "g2"])).toBe(false);
    const missing = receipt(); missing.items.pop();
    expect(validBatchResult(missing, ["g1", "g2"])).toBe(false);
    const incorrect = receipt(); incorrect.counts.saved = 2;
    expect(validBatchResult(incorrect, ["g1", "g2"])).toBe(false);
    const foreign = receipt(); foreign.items[0].group_id = "foreign";
    expect(validBatchResult(foreign, ["g1", "g2"])).toBe(false);
  });
});

it("unknown reason codes cannot resolve inherited object properties", () => {
  for (const code of ["constructor", "toString", "__proto__", "hasOwnProperty"]) {
    expect(reasonLabel(code)).toBe("未完成安全校验，请重新载入并人工核对。");
  }
});
